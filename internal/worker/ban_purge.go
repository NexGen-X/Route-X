package worker

import (
	"context"
	"log/slog"
	"time"

	"github.com/jackc/pgx/v5/pgxpool"

	"github.com/NexGen-X/Route-X/internal/database/repo"
)

// BanPurgeRepo mendefinisikan operasi pembersihan ban kedaluwarsa.
// Dipenuhi oleh *policy.Repo.
type BanPurgeRepo interface {
	PurgeExpired(ctx context.Context, olderThan time.Duration) (int64, error)
}

// BanExpiredPurgeJob membersihkan baris bans yang sudah kedaluwarsa.
//
// PurgeExpired sebelumnya TIDAK punya satu pun pemanggil produksi
// (B-587): baris ban mati menumpuk tanpa pernah dibersihkan —
// sebelum perbaikan CreateBan, baris itu bahkan memblokir ban baru
// untuk subjek yang sama. Job ini memanggilnya berkala dengan
// tenggang (olderThan) supaya ban yang baru kedaluwarsa masih punya
// jejak sebelum dibersihkan; yang dicabut manual tidak pernah
// tersentuh (PurgeExpired hanya menghapus yang lifted_at-nya null).
type BanExpiredPurgeJob struct {
	pool      *pgxpool.Pool
	repo      BanPurgeRepo
	olderThan time.Duration
	logger    *slog.Logger
}

// NewBanExpiredPurgeJob membuat instance baru BanExpiredPurgeJob.
// olderThan <= 0 berarti bawaan 24 jam (pola yang sama dengan
// session_cleanup di main.go).
func NewBanExpiredPurgeJob(pool *pgxpool.Pool, repo BanPurgeRepo, olderThan time.Duration, logger *slog.Logger) *BanExpiredPurgeJob {
	if logger == nil {
		logger = slog.Default()
	}
	if olderThan <= 0 {
		olderThan = 24 * time.Hour
	}
	return &BanExpiredPurgeJob{
		pool:      pool,
		repo:      repo,
		olderThan: olderThan,
		logger:    logger,
	}
}

func (j *BanExpiredPurgeJob) Name() string { return "ban_expired_purge" }

// Run menjalankan satu putaran pembersihan ban kedaluwarsa.
func (j *BanExpiredPurgeJob) Run(ctx context.Context) error {
	unlock, ok, err := TryAdvisoryLock(ctx, j.pool, LockBanPurge)
	if err != nil {
		return repo.Err("advisory lock purge ban", err)
	}
	if !ok {
		return ErrJobSkipped
	}
	defer unlock()

	n, err := j.repo.PurgeExpired(ctx, j.olderThan)
	if err != nil {
		return repo.Err("membersihkan ban kedaluwarsa", err)
	}
	if n > 0 {
		j.logger.InfoContext(ctx, "ban kedaluwarsa dibersihkan", "jumlah", n)
	}
	return nil
}
