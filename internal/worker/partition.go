package worker

import (
	"context"
	"fmt"
	"log/slog"
	"time"

	"github.com/jackc/pgx/v5/pgxpool"
)

// PartitionMaintainerJob memastikan partisi bulanan masa depan selalu tersedia dan
// memeriksa bahwa partisi default tidak menampung baris nyasar.
//
// Aturan 5: Pemeliharaan partisi WAJIB memperingatkan bila partisi DEFAULT tidak kosong.
// Baris yang mendarat di DEFAULT berarti partisi bulanan terlambat dibuat dan pemangkasan
// retensi masa depan tidak bisa membuangnya lewat drop table.
type PartitionMaintainerJob struct {
	pool   *pgxpool.Pool
	logger *slog.Logger
}

// NewPartitionMaintainerJob membuat instance baru PartitionMaintainerJob.
func NewPartitionMaintainerJob(pool *pgxpool.Pool, logger *slog.Logger) *PartitionMaintainerJob {
	if logger == nil {
		logger = slog.Default()
	}
	return &PartitionMaintainerJob{
		pool:   pool,
		logger: logger,
	}
}

func (p *PartitionMaintainerJob) Name() string { return "partition_maintainer" }

// Run membuat partisi untuk 1 dan 2 bulan ke depan, lalu memverifikasi partisi default.
func (p *PartitionMaintainerJob) Run(ctx context.Context) error {
	unlock, ok, err := TryAdvisoryLock(ctx, p.pool, LockPartition)
	if err != nil {
		return fmt.Errorf("advisory lock partition: %w", err)
	}
	if !ok {
		return nil
	}
	defer unlock()

	tables := []string{"requests", "request_events", "request_payloads", "usage_hourly"}
	now := time.Now().UTC()

	// 1. Buat partisi untuk bulan ini, bulan depan, dan 2 bulan ke depan
	for m := 0; m <= 2; m++ {
		targetDate := now.AddDate(0, m, 0)
		for _, table := range tables {
			var createdPart string
			err := p.pool.QueryRow(ctx, "select create_monthly_partition($1, $2::date)", table, targetDate).Scan(&createdPart)
			if err != nil {
				return fmt.Errorf("membuat partisi %s untuk %s: %w", table, targetDate.Format("2006-01"), err)
			}
			p.logger.DebugContext(ctx, "partisi bulanan siap", "tabel", table, "partisi", createdPart)
		}
	}

	// 2. Periksa partisi DEFAULT: tidak boleh ada baris di sana
	for _, table := range tables {
		defaultTable := table + "_default"
		var count int64
		err := p.pool.QueryRow(ctx, fmt.Sprintf("select count(*) from %s", defaultTable)).Scan(&count)
		if err != nil {
			p.logger.WarnContext(ctx, "gagal memeriksa jumlah baris partisi default", "tabel", defaultTable, "error", err)
			continue
		}
		if count > 0 {
			// Peringatan keras: ada data yang salah masuk ke partisi default!
			p.logger.WarnContext(ctx, "PERINGATAN KRITIS: partisi DEFAULT tidak kosong! Data mendarat di luar rentang partisi bulanan",
				"tabel", defaultTable, "jumlah_baris", count)
		}
	}

	return nil
}
