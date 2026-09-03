package worker

import (
	"context"
	"fmt"
	"log/slog"
	"time"

	"github.com/jackc/pgx/v5/pgxpool"
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

// Run mengeksekusi perhitungan rollup pemakaian untuk jendela waktu ke belakang.
func (w *RollupWorker) Run(ctx context.Context) error {
	unlock, ok, err := TryAdvisoryLock(ctx, w.pool, LockRollup)
	if err != nil {
		return fmt.Errorf("advisory lock rollup: %w", err)
	}
	if !ok {
		return nil
	}
	defer unlock()

	now := time.Now().UTC()
	// Jendela belakang: dari 3 jam lalu sampai akhir jam berjalan
	from := now.Add(-w.backlog).Truncate(time.Hour)
	to := now.Add(time.Hour).Truncate(time.Hour)

	jam, hari, err := w.repo.RollupRange(ctx, from, to)
	if err != nil {
		return fmt.Errorf("eksekusi RollupRange [%s..%s]: %w", from.Format(time.RFC3339), to.Format(time.RFC3339), err)
	}

	w.logger.DebugContext(ctx, "rollup pemakaian berhasil dieksekusi",
		"dari", from.Format(time.RFC3339),
		"sampai", to.Format(time.RFC3339),
		"baris_jam", jam,
		"baris_hari", hari,
	)
	return nil
}
