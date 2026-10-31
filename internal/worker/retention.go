package worker

import (
	"context"
	"errors"
	"fmt"
	"log/slog"
	"strconv"
	"strings"
	"time"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"

	"github.com/NexGen-X/Route-X/internal/database/repo"
)

// WebhookRetentionCleaner mendefinisikan pembersihan retensi webhook.
type WebhookRetentionCleaner interface {
	DeleteRetention(ctx context.Context, cutoff time.Time, batchSize int) (int64, error)
}

// RetentionWorker membersihkan data historis yang sudah melewati batas retensi.
//
// Aturan 5:
// a. requests, request_events, usage_hourly dibersihkan dengan MEMBUANG PARTISI (hanya jika
//
//	seluruh rentang partisi lebih tua dari cutoff). Mencegah bloat tabel raksasa.
//
// b. request_payloads dan webhook_deliveries dibersihkan dengan PENGHAPUSAN BERBATCH.
//
//	Diberi jeda 50ms antar batch agar tidak memonopoli I/O disk database.
type RetentionWorker struct {
	pool        *pgxpool.Pool
	webhookRepo WebhookRetentionCleaner
	rollupRepo  RollupRepo
	logDays     int
	payloadDays int
	batchSize   int
	batchPause  time.Duration
	logger      *slog.Logger
}

// NewRetentionWorker membuat instance baru RetentionWorker.
func NewRetentionWorker(
	pool *pgxpool.Pool,
	webhookRepo WebhookRetentionCleaner,
	rollupRepo RollupRepo,
	logDays int,
	payloadDays int,
	logger *slog.Logger,
) *RetentionWorker {
	if logDays <= 0 {
		logDays = 30
	}
	if payloadDays <= 0 {
		payloadDays = 7
	}
	if logger == nil {
		logger = slog.Default()
	}
	return &RetentionWorker{
		pool:        pool,
		webhookRepo: webhookRepo,
		rollupRepo:  rollupRepo,
		logDays:     logDays,
		payloadDays: payloadDays,
		batchSize:   500,
		batchPause:  50 * time.Millisecond,
		logger:      logger,
	}
}

func (w *RetentionWorker) Name() string { return "retention_cleaner" }

// Run mengeksekusi siklus pembersihan retensi.
func (w *RetentionWorker) Run(ctx context.Context) error {
	unlock, ok, err := TryAdvisoryLock(ctx, w.pool, LockRetention)
	if err != nil {
		return repo.Err("advisory lock retention", err)
	}
	if !ok {
		return ErrJobSkipped
	}
	defer unlock()

	now := time.Now().UTC()

	// Aturan 4: ROLLUP DULU, RETENSI KEMUDIAN.
	// Sebelum membuang baris mentah, pastikan rollup jam-jam lampau sudah tuntas dihitung
	if w.rollupRepo != nil {
		from := now.Add(-3 * time.Hour).Truncate(time.Hour)
		to := now.Add(time.Hour).Truncate(time.Hour)
		if _, _, err := w.rollupRepo.RollupRange(ctx, from, to); err != nil {
			w.logger.WarnContext(ctx, "rollup pra-retensi gagal, retensi tetap berhati-hati", "error", err)
		}
	}

	// 1. Pembersihan partisi kedaluwarsa untuk requests, request_events, dan usage_hourly
	cutoffLog := now.AddDate(0, 0, -w.logDays)
	for _, table := range []string{"requests", "request_events", "usage_hourly"} {
		if err := w.dropOldPartitions(ctx, table, cutoffLog); err != nil {
			w.logger.WarnContext(ctx, "gagal membuang partisi kedaluwarsa", "table", table, "error", err)
		}
	}

	// 2. Penghapusan berbatch untuk request_payloads (retensi body lebih singkat, default 7 hari)
	cutoffPayload := now.AddDate(0, 0, -w.payloadDays)
	if err := w.deletePayloadsBatch(ctx, cutoffPayload); err != nil {
		w.logger.WarnContext(ctx, "gagal menghapus batch request_payloads", "error", err)
	}

	// 3. Penghapusan berbatch untuk webhook_deliveries (delivered/abandoned lewat batas)
	if w.webhookRepo != nil {
		if err := w.deleteWebhookDeliveriesBatch(ctx, cutoffPayload); err != nil {
			w.logger.WarnContext(ctx, "gagal membersihkan batch webhook_deliveries", "error", err)
		}
	}

	// 4. Retensi health_checks (audit F6): tabel ini bertambah 1 baris per
	// provider per interval health check SELAMANYA — sebelumnya tidak ada
	// DELETE untuknya di seluruh repo. Riwayat kesehatan 7 hari cukup
	// untuk diagnosis; sisanya hanya membebani indeks.
	if err := w.deleteHealthChecksBatch(ctx, now.AddDate(0, 0, -7)); err != nil {
		w.logger.WarnContext(ctx, "gagal membersihkan batch health_checks", "error", err)
	}

	return nil
}

// deleteHealthChecksBatch menghapus baris health_checks yang lebih tua dari
// cutoff secara berbatch agar kunci baris tidak menahan penulis health
// check yang berjalan tiap interval.
func (w *RetentionWorker) deleteHealthChecksBatch(ctx context.Context, cutoff time.Time) error {
	for i := 0; i < 50; i++ {
		tag, err := w.pool.Exec(ctx, `
			delete from health_checks
			where id in (
				select id from health_checks
				where checked_at < $1
				order by id
				limit 5000
			)`, cutoff)
		if err != nil {
			return repo.Err("menghapus health_checks kedaluwarsa", err)
		}
		if tag.RowsAffected() == 0 {
			return nil
		}
		select {
		case <-ctx.Done():
			return ctx.Err()
		case <-time.After(w.batchPause):
		}
	}
	return nil
}

// dropOldPartitions mencari partisi yang seluruh rentangnya lebih tua dari cutoff lalu membuangnya.
func (w *RetentionWorker) dropOldPartitions(ctx context.Context, parentTable string, cutoff time.Time) error {
	rows, err := w.pool.Query(ctx, `
		select c.relname
		from pg_inherits i
		join pg_class c on c.oid = i.inhrelid
		join pg_class p on p.oid = i.inhparent
		where p.relname = $1 and c.relname not like '%_default'
		order by c.relname`,
		parentTable,
	)
	if err != nil {
		return repo.Err("query partisi "+parentTable, err)
	}
	defer rows.Close()

	var toDrop []string
	var unrolledPartitions []string
	for rows.Next() {
		var partName string
		if err := rows.Scan(&partName); err != nil {
			return err
		}

		// Format nama partisi: <parent>_YYYYMM
		prefix := parentTable + "_"
		if !strings.HasPrefix(partName, prefix) {
			continue
		}
		dateStr := strings.TrimPrefix(partName, prefix)
		if len(dateStr) != 6 {
			continue
		}
		year, errY := strconv.Atoi(dateStr[:4])
		month, errM := strconv.Atoi(dateStr[4:])
		if errY != nil || errM != nil || month < 1 || month > 12 {
			continue
		}

		startDate := time.Date(year, time.Month(month), 1, 0, 0, 0, 0, time.UTC)
		endDate := time.Date(year, time.Month(month+1), 1, 0, 0, 0, 0, time.UTC)

		// HANYA pertimbangkan drop jika SELURUH rentang partisi lebih tua dari cutoff
		if !endDate.Before(cutoff) {
			continue
		}

		// Aturan 5.6: Sinkronisasi Rollup & Retensi.
		// Retensi WAJIB menolak membuang partisi requests yang jam-jamnya belum
		// ter-rollup PENUH ke usage_hourly, untuk mencegah lubang permanen
		// pada data statistik lalu lintas.
		//
		// Pemeriksaan ini FAIL-CLOSED dan berbasis KELENGKAPAN (audit F5/F6):
		// versi lama (a) hanya membuktikan ada >=1 baris usage_hourly per jam
		// — bukan bahwa seluruh request jam itu teragregasi, dan (b) error
		// query apa pun selain ErrNoRows diperlakukan sebagai "sudah
		// ter-rollup" sehingga partisi ter-DROP saat pemeriksaan justru
		// gagal. Kini jumlah request per jam dibandingkan dengan
		// SUM(request_count) usage_hourly per bucket; selisih ATAU error
		// pemeriksaan = drop ditolak.
		if parentTable == "requests" {
			var unrolled int
			checkQuery := fmt.Sprintf(`
				select 1 from (
					select date_trunc('hour', r.created_at, 'UTC') as b, count(*) as c
					from %s r
					group by 1
				) per_jam
				left join (
					select bucket as b, sum(request_count) as s
					from usage_hourly
					group by 1
				) agg on agg.b = per_jam.b
				where coalesce(agg.s, 0) <> per_jam.c
				limit 1`, partName)
			errCheck := w.pool.QueryRow(ctx, checkQuery).Scan(&unrolled)
			if errCheck == nil {
				w.logger.ErrorContext(ctx, "PENOLAKAN DROP PARTISI: terdapat jam request yang belum ter-rollup penuh ke usage_hourly",
					"table", parentTable, "partition", partName,
					"start", startDate.Format("2006-01-02"), "end", endDate.Format("2006-01-02"))
				unrolledPartitions = append(unrolledPartitions, partName)
				continue
			}
			if !errors.Is(errCheck, pgx.ErrNoRows) {
				w.logger.ErrorContext(ctx, "PENOLAKAN DROP PARTISI: pemeriksaan rollup gagal, drop dibatalkan demi keamanan data",
					"table", parentTable, "partition", partName, "error", errCheck)
				unrolledPartitions = append(unrolledPartitions, partName)
				continue
			}
		}

		toDrop = append(toDrop, partName)
	}
	if err := rows.Err(); err != nil {
		return err
	}

	for _, part := range toDrop {
		w.logger.InfoContext(ctx, "membuang partisi kedaluwarsa melewati masa retensi",
			"parent", parentTable, "partition", part, "cutoff", cutoff.Format(time.RFC3339))
		// Detach dan drop
		query := fmt.Sprintf("drop table if exists %s", part)
		if _, err := w.pool.Exec(ctx, query); err != nil {
			return repo.Err("drop table "+part, err)
		}
	}

	if len(unrolledPartitions) > 0 {
		return fmt.Errorf("partisi %v ditolak untuk dibuang karena masih memuat data yang belum ter-rollup ke usage_hourly", unrolledPartitions)
	}

	return nil
}

// deletePayloadsBatch menghapus isi request_payloads bertahap dengan jeda kecil antar batch.
func (w *RetentionWorker) deletePayloadsBatch(ctx context.Context, cutoff time.Time) error {
	for {
		select {
		case <-ctx.Done():
			return ctx.Err()
		default:
		}

		tag, err := w.pool.Exec(ctx, `
			delete from request_payloads
			where (request_pk, created_at) in (
				select request_pk, created_at
				from request_payloads
				where created_at < $1
				limit $2
			)`,
			cutoff, w.batchSize,
		)
		if err != nil {
			return repo.Err("delete batch request_payloads", err)
		}

		if tag.RowsAffected() == 0 {
			break
		}

		// Jeda antar batch untuk memberi napas pada disk I/O dan autovacuum
		time.Sleep(w.batchPause)
	}
	return nil
}

// deleteWebhookDeliveriesBatch menghapus delivery lama (delivered/abandoned) bertahap.
func (w *RetentionWorker) deleteWebhookDeliveriesBatch(ctx context.Context, cutoff time.Time) error {
	for {
		select {
		case <-ctx.Done():
			return ctx.Err()
		default:
		}

		n, err := w.webhookRepo.DeleteRetention(ctx, cutoff, w.batchSize)
		if err != nil {
			return repo.Err("delete retention webhook deliveries", err)
		}

		if n == 0 {
			break
		}

		time.Sleep(w.batchPause)
	}
	return nil
}
