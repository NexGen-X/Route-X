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
// a. requests, request_events, usage_hourly, dan request_payloads dibersihkan
// dengan MEMBUANG PARTISI (hanya jika seluruh rentang partisi lebih tua dari
// cutoff DAN penjaga kelengkapan masing-masing lolos). Mencegah bloat tabel
// raksasa.
//
// b. Sisa baris request_payloads (partisi DEFAULT, partisi bulan berjalan
// retensi), webhook_deliveries, health_checks, dan audit_logs dibersihkan
// dengan PENGHAPUSAN BERBATCH berbatas iterasi. Diberi jeda 50ms antar batch
// agar tidak memonopoli I/O disk database.
//
// c. Baris yang mengendap di partisi DEFAULT direlokasi dulu ke partisi
// bulannya (migrasi 0019) agar ikut jalur drop terpenjaga; tanpa ini baris
// DEFAULT tidak pernah terhapus oleh mekanisme apa pun (audit3 V-DT-06).
type RetentionWorker struct {
	pool        *pgxpool.Pool
	webhookRepo WebhookRetentionCleaner
	rollupRepo  RollupRepo
	logDays     int
	payloadDays int
	auditDays   int
	batchSize   int
	batchPause  time.Duration
	logger      *slog.Logger
}

// NewRetentionWorker membuat instance baru RetentionWorker.
//
// auditDays adalah masa retensi audit_logs — tabel jejak audit yang
// sebelumnya tidak dibersihkan oleh siapa pun dan tumbuh tanpa batas
// (audit3 V-DT-20).
func NewRetentionWorker(
	pool *pgxpool.Pool,
	webhookRepo WebhookRetentionCleaner,
	rollupRepo RollupRepo,
	logDays int,
	payloadDays int,
	auditDays int,
	logger *slog.Logger,
) *RetentionWorker {
	if logDays <= 0 {
		logDays = 30
	}
	if payloadDays <= 0 {
		payloadDays = 7
	}
	if auditDays <= 0 {
		auditDays = 365
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
		auditDays:   auditDays,
		batchSize:   500,
		batchPause:  50 * time.Millisecond,
		logger:      logger,
	}
}

func (w *RetentionWorker) Name() string { return "retention_cleaner" }

// Run mengeksekusi siklus pembersihan retensi.
//
// Setiap langkah tetap dicoba walau langkah lain gagal, tetapi kegagalan
// TIDAK lagi ditelan: versi lama mencatat log lalu mengembalikan nil,
// sehingga supervisor mencatat putaran yang langkah intinya gagal — bahkan
// putaran yang DITOLAK penjaga kelengkapan — sebagai "success" (audit3
// V-DT-09). Kini semua kegagalan digabung dan dikembalikan; hanya
// keberhasilan penuh yang tercatat sukses.
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
	var errs []error

	// Aturan 4: ROLLUP DULU, RETENSI KEMUDIAN.
	// Sebelum membuang baris mentah, pastikan rollup jam-jam lampau sudah tuntas dihitung.
	// Kegagalannya dicatat sebagai kegagalan putaran (langkah lain tetap jalan —
	// penjaga kelengkapan di dropOldPartitions adalah garis pertahanan datanya).
	if w.rollupRepo != nil {
		from := now.Add(-3 * time.Hour).Truncate(time.Hour)
		to := now.Add(time.Hour).Truncate(time.Hour)
		if _, _, err := w.rollupRepo.RollupRange(ctx, from, to); err != nil {
			errs = append(errs, repo.Err("rollup pra-retensi", err))
		}
	}

	cutoffLog := now.AddDate(0, 0, -w.logDays)
	cutoffPayload := now.AddDate(0, 0, -w.payloadDays)

	// 1. Relokasi baris partisi DEFAULT yang sudah melewati masa retensinya
	// (audit3 V-DT-06): pindahkan ke partisi bulannya lewat fungsi partisi
	// migrasi 0019, supaya langkah drop di bawah menilai mereka dengan
	// penjaga kelengkapan yang sama seperti baris partisi biasa. Urutannya
	// wajib sebelum drop; kolom kunci tiap tabel mengikuti definisi partisinya.
	for _, reloc := range []struct {
		parent, kolomKunci string
		cutoff             time.Time
	}{
		{"requests", "created_at", cutoffLog},
		{"request_events", "created_at", cutoffLog},
		{"usage_hourly", "bucket", cutoffLog},
		{"request_payloads", "created_at", cutoffPayload},
	} {
		if err := w.relokasiBarisDefault(ctx, reloc.parent, reloc.kolomKunci, reloc.cutoff); err != nil {
			errs = append(errs, err)
		}
	}

	// 2. Pembersihan partisi kedaluwarsa. requests/request_events/usage_hourly
	// memakai cutoff log; request_payloads memakai cutoff body yang lebih
	// singkat — partisinya sebelumnya TIDAK PERNAH dibuang (audit3 V-DT-07),
	// hanya barisnya yang di-DELETE berbatch, jadi file partisinya menumpuk
	// permanen walau isinya sudah kosong.
	for _, drop := range []struct {
		table  string
		cutoff time.Time
	}{
		{"requests", cutoffLog},
		{"request_events", cutoffLog},
		{"usage_hourly", cutoffLog},
		{"request_payloads", cutoffPayload},
	} {
		if err := w.dropOldPartitions(ctx, drop.table, drop.cutoff); err != nil {
			errs = append(errs, repo.Err("membuang partisi kedaluwarsa "+drop.table, err))
		}
	}

	// 3. Penghapusan berbatch untuk sisa request_payloads (partisi DEFAULT dan
	// partisi yang rentangnya belum seluruhnya melewati cutoff).
	if err := w.deletePayloadsBatch(ctx, cutoffPayload); err != nil {
		errs = append(errs, err)
	}

	// 4. Penghapusan berbatch untuk webhook_deliveries (delivered/abandoned lewat batas)
	if w.webhookRepo != nil {
		if err := w.deleteWebhookDeliveriesBatch(ctx, cutoffPayload); err != nil {
			errs = append(errs, err)
		}
	}

	// 5. Retensi health_checks (audit F6): tabel ini bertambah 1 baris per
	// provider per interval health check SELAMANYA — sebelumnya tidak ada
	// DELETE untuknya di seluruh repo. Riwayat kesehatan 7 hari cukup
	// untuk diagnosis; sisanya hanya membebani indeks.
	if err := w.deleteHealthChecksBatch(ctx, now.AddDate(0, 0, -7)); err != nil {
		errs = append(errs, err)
	}

	// 6. Retensi audit_logs (audit3 V-DT-20): jejak audit tidak lagi disimpan
	// selamanya; masa simpannya diatur AUDIT_LOG_RETENTION_DAYS.
	if err := w.deleteAuditLogsBatch(ctx, now.AddDate(0, 0, -w.auditDays)); err != nil {
		errs = append(errs, err)
	}

	return errors.Join(errs...)
}

// batasRelokasiPerPutaran membatasi bulan DEFAULT yang direlokasi per tabel
// per putaran, agar putaran pertama sesudah downtime panjang tidak berubah
// menjadi migrasi data raksasa; sisanya dicicil putaran berikutnya.
const batasRelokasiPerPutaran = 3

// relokasiBarisDefault memindahkan baris partisi DEFAULT yang bulannya sudah
// melewati cutoff ke partisi bulanan mereka, dengan memanggil fungsi partisi
// migrasi 0019 (yang sejak 0019 menyelamatkan baris DEFAULT, bukan gagal).
// Bulan yang direntangkan cutoff ikut dibuat partisinya; keputusan membuang
// tetap milik dropOldPartitions beserta penjaganya.
func (w *RetentionWorker) relokasiBarisDefault(ctx context.Context, parentTable, kolomKunci string, cutoff time.Time) error {
	defTable := parentTable + "_default"
	var ada bool
	if err := w.pool.QueryRow(ctx, "select to_regclass($1) is not null", defTable).Scan(&ada); err != nil {
		return repo.Err("memeriksa partisi default "+parentTable, err)
	}
	if !ada {
		return nil
	}

	// Nama tabel/kolom berasal dari daftar tetap di Run — bukan masukan
	// pengguna — jadi aman dirangkai ke SQL.
	rows, err := w.pool.Query(ctx, fmt.Sprintf(
		`select distinct date_trunc('month', %s)::date from %s where %s < $1 order by 1 asc limit $2`,
		kolomKunci, defTable, kolomKunci), cutoff, batasRelokasiPerPutaran)
	if err != nil {
		return repo.Err("membaca bulan partisi default "+parentTable, err)
	}
	var bulan []time.Time
	for rows.Next() {
		var b time.Time
		if err := rows.Scan(&b); err != nil {
			rows.Close()
			return err
		}
		bulan = append(bulan, b)
	}
	rows.Close()
	if err := rows.Err(); err != nil {
		return repo.Err("membaca bulan partisi default "+parentTable, err)
	}

	for _, b := range bulan {
		var dibuat string
		if err := w.pool.QueryRow(ctx, "select create_monthly_partition($1, $2::date)", parentTable, b).Scan(&dibuat); err != nil {
			return repo.Err(fmt.Sprintf("relokasi baris default %s bulan %s", parentTable, b.Format("2006-01")), err)
		}
		w.logger.InfoContext(ctx, "baris partisi DEFAULT direlokasi ke partisi bulannya",
			"parent", parentTable, "partisi", dibuat)
	}
	return nil
}

// hariRollupLengkap melaporkan apakah usage_daily untuk satu hari kalender
// UTC sudah sama totalnya dengan usage_hourly hari itu — penjaga sebelum
// partisi usage_hourly dibuang (audit3 V-DT-04): retensi versi lama membuang
// usage_hourly murni berdasarkan umur, padahal baris per jam itulah SATU-
// SATUNYA bahan untuk memperbaiki hari yang agregat hariannya hilang/basi.
// Selisih sekecil apa pun berarti harian belum mencerminkan jamnya.
func (w *RetentionWorker) hariRollupLengkap(ctx context.Context, hari time.Time) (bool, error) {
	var lengkap bool
	err := w.pool.QueryRow(ctx, `
		select (select coalesce(sum(request_count), 0) from usage_hourly
		        where bucket >= $1 and bucket < $2)
		     = (select coalesce(sum(request_count), 0) from usage_daily
		        where bucket >= $1 and bucket < $2)`,
		hari, hari.AddDate(0, 0, 1)).Scan(&lengkap)
	if err != nil {
		return false, err
	}
	return lengkap, nil
}

// deleteAuditLogsBatch menghapus baris audit_logs yang lebih tua dari cutoff
// secara berbatch terbatas, meniru pola deleteHealthChecksBatch.
func (w *RetentionWorker) deleteAuditLogsBatch(ctx context.Context, cutoff time.Time) error {
	for i := 0; i < 50; i++ {
		tag, err := w.pool.Exec(ctx, `
			delete from audit_logs
			where ctid in (
				select ctid from audit_logs
				where occurred_at < $1
				limit 5000
			)`, cutoff)
		if err != nil {
			return repo.Err("menghapus audit_logs kedaluwarsa", err)
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
	w.logger.WarnContext(ctx, "penghapusan audit_logs mencapai batas iterasi; sisa dilanjutkan putaran berikutnya",
		"cutoff", cutoff.Format(time.RFC3339))
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

		// Penjaga kelengkapan HARIAN untuk usage_hourly (audit3 V-DT-04):
		// partisi per jam hanya boleh dibuang bila SETIAP hari di rentangnya
		// sudah tercermin penuh di usage_daily. Tanpa penjaga ini retensi
		// membuang bahan baku perbaikan harian murni berdasarkan umur —
		// padahal kelengkapan harian sebelumnya tidak dideteksi di mana pun,
		// jadi hari yang basi/hilang terkubur permanen bersama partisinya.
		// Fail-closed: selisih ATAU error pemeriksaan = drop ditolak.
		if parentTable == "usage_hourly" {
			ditolak := false
			for hari := startDate; hari.Before(endDate); hari = hari.AddDate(0, 0, 1) {
				lengkap, errHari := w.hariRollupLengkap(ctx, hari)
				if errHari != nil {
					w.logger.ErrorContext(ctx, "PENOLAKAN DROP PARTISI: pemeriksaan kelengkapan harian gagal, drop dibatalkan demi keamanan data",
						"table", parentTable, "partition", partName, "error", errHari)
					ditolak = true
					break
				}
				if !lengkap {
					w.logger.ErrorContext(ctx, "PENOLAKAN DROP PARTISI: ada hari yang belum tercermin penuh di usage_daily",
						"table", parentTable, "partition", partName, "hari", hari.Format("2006-01-02"))
					ditolak = true
					break
				}
			}
			if ditolak {
				unrolledPartitions = append(unrolledPartitions, partName)
				continue
			}
		}

		// Sinkronisasi events dengan requests (audit3 V-DT-05): kedua tabel
		// sebelumnya dibuang independen, jadi partisi events sebulan bisa
		// hilang sementara request-nya masih ada (tertolak penjaganya sendiri)
		// — jejak audit per request lenyap dari data yang masih disimpan.
		// Partisi events hanya dibuang bila partisi requests bulan yang sama
		// sudah tidak ada. Urutan pemrosesan Run (requests sebelum events)
		// membuat partisi yang lolos penjaga terbuang lebih dulu di putaran
		// yang sama.
		if parentTable == "request_events" {
			var requestsMasihAda bool
			if err := w.pool.QueryRow(ctx, "select to_regclass($1) is not null", "requests_"+dateStr).Scan(&requestsMasihAda); err != nil {
				w.logger.ErrorContext(ctx, "PENOLAKAN DROP PARTISI: pemeriksaan partisi requests pendamping gagal, drop dibatalkan demi keamanan data",
					"table", parentTable, "partition", partName, "error", err)
				unrolledPartitions = append(unrolledPartitions, partName)
				continue
			}
			if requestsMasihAda {
				w.logger.InfoContext(ctx, "drop partisi events ditunda: partisi requests bulan yang sama masih ada",
					"table", parentTable, "partition", partName)
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
		return fmt.Errorf("partisi %v ditolak untuk dibuang karena penjaga kelengkapan rollup belum lolos", unrolledPartitions)
	}

	return nil
}

// batasBatchPayloads membatasi iterasi penghapusan berbatch per putaran
// (audit3 V-DT-12): versi lama berputar `for {}` tanpa batas, jadi tunggakan
// raksasa dari downtime panjang membuat satu putaran retensi berjalan
// berjam-jam dan mengunci worker. Dengan batas ini satu putaran menghapus
// maksimal batasBatchPayloads × batchSize baris; sisa dicicil putaran
// berikutnya dan cap yang tercapai dicatat sebagai peringatan.
const batasBatchPayloads = 400

// batasBatchWebhook sama seperti batasBatchPayloads, untuk delivery webhook.
const batasBatchWebhook = 200

// deletePayloadsBatch menghapus isi request_payloads bertahap dengan jeda kecil antar batch.
func (w *RetentionWorker) deletePayloadsBatch(ctx context.Context, cutoff time.Time) error {
	for i := 0; i < batasBatchPayloads; i++ {
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
			return nil
		}

		// Jeda antar batch untuk memberi napas pada disk I/O dan autovacuum
		time.Sleep(w.batchPause)
	}
	w.logger.WarnContext(ctx, "penghapusan request_payloads mencapai batas iterasi; sisa dilanjutkan putaran berikutnya",
		"cutoff", cutoff.Format(time.RFC3339), "batas_iterasi", batasBatchPayloads)
	return nil
}

// deleteWebhookDeliveriesBatch menghapus delivery lama (delivered/abandoned) bertahap.
func (w *RetentionWorker) deleteWebhookDeliveriesBatch(ctx context.Context, cutoff time.Time) error {
	for i := 0; i < batasBatchWebhook; i++ {
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
			return nil
		}

		time.Sleep(w.batchPause)
	}
	w.logger.WarnContext(ctx, "penghapusan webhook_deliveries mencapai batas iterasi; sisa dilanjutkan putaran berikutnya",
		"cutoff", cutoff.Format(time.RFC3339), "batas_iterasi", batasBatchWebhook)
	return nil
}
