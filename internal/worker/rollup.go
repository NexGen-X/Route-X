package worker

import (
	"context"
	"errors"
	"fmt"
	"log/slog"
	"time"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"

	"github.com/NexGen-X/Route-X/internal/database/repo"
)

// batasPulihJamPerPutaran membatasi berapa jam tertunggak yang dikejar dalam
// satu putaran pemulihan. Tanpa batas, mengejar tunggakan berminggu-minggu
// dalam satu Run mengunci worker sangat lama; dengan batas, tunggakan dicicil
// putaran demi putaran, tertua dulu, dan jendela biasa selalu didahulukan.
const batasPulihJamPerPutaran = 96

// batasPulihHariPerPutaran membatasi hari tak-lengkap yang diperbaiki dalam
// satu putaran (satu panggilan RollupDaily per hari).
const batasPulihHariPerPutaran = 31

// RollupRepo mendefinisikan operasi agregasi usage. Dipenuhi oleh *traffic.Repo.
type RollupRepo interface {
	RollupRange(ctx context.Context, from, to time.Time) (jam, hari int64, err error)
	RollupDaily(ctx context.Context, day time.Time) (int64, error)
}

// RollupWorker menjalankan agregasi berkala pemakaian token dan biaya.
//
// Sesuai aturan 4 (ROLLUP DULU, RETENSI KEMUDIAN):
//   - Menghitung ulang jam-jam pada jendela belakang (misal 3 jam terakhir) untuk menangkap
//     lalu lintas yang mendarat lambat atau agregasi yang terlewat saat restart.
//   - Menimpa hasil agregasi secara idempoten (ON CONFLICT DO UPDATE).
//   - Memakai pg_advisory_lock LockRollup agar hanya satu instance yang melakukan kalkulasi.
type RollupWorker struct {
	pool    *pgxpool.Pool
	repo    RollupRepo
	logger  *slog.Logger
	backlog time.Duration
}

// NewRollupWorker membuat instance baru RollupWorker.
func NewRollupWorker(pool *pgxpool.Pool, repo RollupRepo, logger *slog.Logger) *RollupWorker {
	if logger == nil {
		logger = slog.Default()
	}
	return &RollupWorker{
		pool:    pool,
		repo:    repo,
		logger:  logger,
		backlog: 3 * time.Hour,
	}
}

func (w *RollupWorker) Name() string { return "usage_rollup_scheduler" }

// jamTakLengkapTertua mengembalikan awal jam tertua pada [sejak, sampai)
// yang jumlah request mentahnya tidak sama dengan SUM(request_count)
// usage_hourly — definisi kelengkapan yang sama dengan penjaga drop
// partisi di RetentionWorker. Nil berarti semua jam pada rentang lengkap.
func (w *RollupWorker) jamTakLengkapTertua(ctx context.Context, sejak, sampai time.Time) (*time.Time, error) {
	var bucket *time.Time
	err := w.pool.QueryRow(ctx, `
		select per_jam.b
		from (
			select date_trunc('hour', created_at, 'UTC') as b, count(*) as c
			from requests
			where created_at >= $1 and created_at < $2
			group by 1
		) per_jam
		left join (
			select bucket as b, sum(request_count) as s
			from usage_hourly
			where bucket >= $1 and bucket < $2
			group by 1
		) agg on agg.b = per_jam.b
		where coalesce(agg.s, 0) <> per_jam.c
		order by per_jam.b asc
		limit 1`, sejak, sampai).Scan(&bucket)
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return nil, nil
		}
		return nil, err
	}
	return bucket, nil
}

// lantaiData mengembalikan batas bawah pemindaian pemulihan: jam terawal yang
// datanya MASIH ADA, dari requests mentah maupun usage_hourly. Nil berarti
// belum ada data sama sekali.
//
// Versi lama memindai dari jendela tetap "45 hari yang lalu" (audit3 V-DT-08):
// jam tak-ter-rollup yang lebih tua dari 45 hari tidak pernah ditemukan lagi
// (audit3 V-DT-07), sementara datanya sendiri sudah lama dibuang retensi atau
// justru masih ada — tak satu pun dibuktikan. Memindai dari data tertua yang
// benar-benar ada membuat lantai itu sebuah watermark yang jujur: yang sudah
// dibuang retensi memang tidak bisa (dan tidak perlu) dipulihkan, yang masih
// ada pasti terjangkau.
func (w *RollupWorker) lantaiData(ctx context.Context) (*time.Time, error) {
	var lantai *time.Time
	err := w.pool.QueryRow(ctx, `
		select least(
			(select date_trunc('hour', min(created_at), 'UTC') from requests),
			(select date_trunc('hour', min(bucket), 'UTC') from usage_hourly)
		)`).Scan(&lantai)
	if err != nil {
		return nil, err
	}
	return lantai, nil
}

// hariTakLengkap mengembalikan awal hari-hari lampau (sebelum awalHariIni) pada
// [sejak, awalHariIni) yang total usage_daily-nya tidak sama dengan total
// usage_hourly — tertua dulu, maksimal batas hari.
//
// Kelengkapan HARIAN tidak pernah diperiksa di mana pun sebelum ini
// (audit3 V-DT-04): RollupRange menulis ulang hari-hari yang jamnya ia sentuh,
// tetapi hari yang jamnya sudah lengkap dari pemulihan masa lalu (atau yang
// baris hariannya lahir dari versi lama) tidak pernah dibandingkan ulang —
// statistik rentang panjang menyimpang dari statistik per jam secara permanen
// dan diam-diam.
func (w *RollupWorker) hariTakLengkap(ctx context.Context, sejak, awalHariIni time.Time, batas int) ([]time.Time, error) {
	rows, err := w.pool.Query(ctx, `
		with per_jam as (
			select date_trunc('day', bucket, 'UTC') as harinya, sum(request_count) as n
			from usage_hourly
			where bucket >= $1 and bucket < $2
			group by 1
		), per_hari as (
			select bucket as harinya, sum(request_count) as n
			from usage_daily
			where bucket >= $1 and bucket < $2
			group by 1
		)
		select coalesce(per_jam.harinya, per_hari.harinya)
		from per_jam
		full join per_hari on per_hari.harinya = per_jam.harinya
		where coalesce(per_jam.n, 0) <> coalesce(per_hari.n, 0)
		order by 1 asc
		limit $3`, sejak, awalHariIni, batas)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var out []time.Time
	for rows.Next() {
		var h time.Time
		if err := rows.Scan(&h); err != nil {
			return nil, err
		}
		out = append(out, h)
	}
	return out, rows.Err()
}

// Run mengeksekusi perhitungan rollup pemakaian untuk jendela waktu ke belakang.
//
// Tiga tahap, urutannya penting:
//  1. Jendela biasa (3 jam terakhir) — selalu didahulukan agar dasbor dekat
//     tidak pernah menunggu tunggakan lama selesai.
//  2. Pemulihan jam: jam tak-lengkap tertua sejak lantai data dikejar,
//     dicicil maksimal batasPulihJamPerPutaran jam per putaran.
//  3. Perbaikan hari: hari lampau yang total hariannya menyimpang dari total
//     jamnya ditulis ulang, maksimal batasPulihHariPerPutaran hari.
//
// Kegagalan tahap 2/3 tidak membatalkan tahap lain, tetapi membuat Run
// mengembalikan error gabungan: putaran yang sebagian gagal tidak boleh
// tercatat sukses.
func (w *RollupWorker) Run(ctx context.Context) error {
	unlock, ok, err := TryAdvisoryLock(ctx, w.pool, LockRollup)
	if err != nil {
		return repo.Err("advisory lock rollup", err)
	}
	if !ok {
		return ErrJobSkipped
	}
	defer unlock()

	now := time.Now().UTC()
	// Jendela belakang: dari 3 jam lalu sampai akhir jam berjalan.
	from := now.Add(-w.backlog).Truncate(time.Hour)
	to := now.Add(time.Hour).Truncate(time.Hour)

	jam, hari, err := w.repo.RollupRange(ctx, from, to)
	if err != nil {
		return repo.Err(fmt.Sprintf("eksekusi RollupRange [%s..%s]", from.Format(time.RFC3339), to.Format(time.RFC3339)), err)
	}
	w.logger.DebugContext(ctx, "rollup pemakaian berhasil dieksekusi",
		"dari", from.Format(time.RFC3339),
		"sampai", to.Format(time.RFC3339),
		"baris_jam", jam,
		"baris_hari", hari,
	)

	lantai, err := w.lantaiData(ctx)
	if err != nil {
		return repo.Err("menentukan lantai data rollup", err)
	}
	if lantai == nil {
		return nil // belum ada data sama sekali; tidak ada yang dipulihkan
	}

	var errs []error

	// Tahap 2: kejar jam tak-lengkap yang lebih tua dari jendela biasa.
	if tertua, err := w.jamTakLengkapTertua(ctx, *lantai, from); err != nil {
		errs = append(errs, repo.Err("pencarian jam rollup tak lengkap", err))
	} else if tertua != nil {
		sampai := tertua.Add(time.Duration(batasPulihJamPerPutaran) * time.Hour)
		if sampai.After(from) {
			sampai = from
		}
		w.logger.InfoContext(ctx, "rollup memulihkan jam tak lengkap akibat downtime",
			"dari", tertua.Format(time.RFC3339), "sampai", sampai.Format(time.RFC3339))
		if _, _, err := w.repo.RollupRange(ctx, *tertua, sampai); err != nil {
			errs = append(errs, repo.Err(fmt.Sprintf("pemulihan rollup jam [%s..%s]", tertua.Format(time.RFC3339), sampai.Format(time.RFC3339)), err))
		}
	}

	// Tahap 3: perbaiki hari lampau yang hariannya menyimpang dari jamnya.
	awalHariIni := time.Date(now.Year(), now.Month(), now.Day(), 0, 0, 0, 0, time.UTC)
	if hariBermasalah, err := w.hariTakLengkap(ctx, *lantai, awalHariIni, batasPulihHariPerPutaran); err != nil {
		errs = append(errs, repo.Err("pencarian hari rollup tak lengkap", err))
	} else {
		for _, h := range hariBermasalah {
			w.logger.InfoContext(ctx, "rollup memperbaiki hari tak lengkap", "hari", h.Format("2006-01-02"))
			if _, err := w.repo.RollupDaily(ctx, h); err != nil {
				errs = append(errs, repo.Err(fmt.Sprintf("perbaikan rollup harian %s", h.Format("2006-01-02")), err))
			}
		}
	}

	return errors.Join(errs...)
}
