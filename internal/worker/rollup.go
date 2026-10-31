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

// RollupRepo mendefinisikan operasi agregasi usage. Dipenuhi oleh *traffic.Repo.
type RollupRepo interface {
	RollupRange(ctx context.Context, from, to time.Time) (jam, hari int64, err error)
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

// Run mengeksekusi perhitungan rollup pemakaian untuk jendela waktu ke belakang.
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

	// Pemulihan downtime (audit F6): jendela tetap 3 jam berarti setelah
	// gateway mati lebih dari 3 jam, jam-jam lama TIDAK PERNAH ter-rollup
	// lagi — dan penjaga retensi (yang kini fail-closed + memeriksa
	// kelengkapan) akan menolak membuang partisinya selamanya. Cari jam
	// tak-lengkap tertua dalam 45 hari terakhir dan mulai dari sana bila
	// lebih tua dari jendela biasa. Kegagalan pencarian tidak fatal:
	// jendela biasa tetap berjalan dan penjaga retensi tetap aman.
	if tertua, err := w.jamTakLengkapTertua(ctx, now.AddDate(0, 0, -45), from); err != nil {
		w.logger.WarnContext(ctx, "pencarian jam rollup tak lengkap gagal, memakai jendela biasa", "error", err)
	} else if tertua != nil && tertua.Before(from) {
		w.logger.InfoContext(ctx, "rollup memulihkan jam tak lengkap akibat downtime",
			"dari", tertua.Format(time.RFC3339), "jendela_biasa", from.Format(time.RFC3339))
		from = *tertua
	}

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
	return nil
}
