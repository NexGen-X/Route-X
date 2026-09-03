package worker

import (
	"context"
	"fmt"
	"log/slog"
	"strconv"
	"strings"
	"time"

	"github.com/jackc/pgx/v5/pgxpool"
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
		return fmt.Errorf("advisory lock retention: %w", err)
	}
	if !ok {
		return nil
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
		return fmt.Errorf("query partisi %s: %w", parentTable, err)
	}
	defer rows.Close()

	var toDrop []string
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

		// Akhir rentang partisi bulanan adalah tanggal 1 bulan berikutnya
		endDate := time.Date(year, time.Month(month+1), 1, 0, 0, 0, 0, time.UTC)

		// HANYA drop jika SELURUH rentang partisi lebih tua dari cutoff
		if endDate.Before(cutoff) {
			toDrop = append(toDrop, partName)
		}
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
			return fmt.Errorf("drop table %s: %w", part, err)
		}
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
			return fmt.Errorf("delete batch request_payloads: %w", err)
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
			return fmt.Errorf("delete retention webhook deliveries: %w", err)
		}

		if n == 0 {
			break
		}

		time.Sleep(w.batchPause)
	}
	return nil
}
