package worker

import (
	"context"
	"log/slog"
	"time"

	"github.com/jackc/pgx/v5/pgxpool"

	"github.com/NexGen-X/Route-X/internal/database/repo"
	"github.com/NexGen-X/Route-X/internal/database/repo/policy"
	"github.com/NexGen-X/Route-X/internal/database/repo/upstream"
)

// BudgetResetRepo mendefinisikan operasi untuk reset anggaran yang expired.
// Dipenuhi oleh *policy.Repo.
type BudgetResetRepo interface {
	ExpiredBudgets(ctx context.Context, asOf time.Time) ([]*policy.Budget, error)
	CalculateSpend(ctx context.Context, scope, scopeID string, from time.Time, to *time.Time) (upstream.USD, error)
	ResetPeriodWithSpend(ctx context.Context, id string, start time.Time, end *time.Time, spent upstream.USD) (*policy.Budget, error)
}

// BudgetResetterJob memajukan periode anggaran yang sudah berakhir dan menghitung ulang pemakaiannya.
//
// Aturan 11:
//   - Memakai query baru ExpiredBudgets karena ActiveBudgets mengecualikan baris yang period_end-nya lewat.
//   - Menghitung ulang spent_usd dari requests.cost_usd (skala 8) karena kolom budgets.spent_usd
//     berskala 6 sehingga akumulasi AddSpend PostgreSQL mengalami pembulatan.
//   - Melepas penanda peringatan (alerted_at = null) agar periode baru dapat memicu peringatan kembali.
type BudgetResetterJob struct {
	pool   *pgxpool.Pool
	repo   BudgetResetRepo
	logger *slog.Logger
}

// NewBudgetResetterJob membuat instance baru BudgetResetterJob.
func NewBudgetResetterJob(pool *pgxpool.Pool, repo BudgetResetRepo, logger *slog.Logger) *BudgetResetterJob {
	if logger == nil {
		logger = slog.Default()
	}
	return &BudgetResetterJob{
		pool:   pool,
		repo:   repo,
		logger: logger,
	}
}

func (b *BudgetResetterJob) Name() string { return "budget_period_resetter" }

// Run mengeksekusi pemeriksaan dan pembaruan periode anggaran kedaluwarsa.
func (b *BudgetResetterJob) Run(ctx context.Context) error {
	unlock, ok, err := TryAdvisoryLock(ctx, b.pool, LockBudget)
	if err != nil {
		return repo.Err("advisory lock budget reset", err)
	}
	if !ok {
		return ErrJobSkipped
	}
	defer unlock()

	now := time.Now().UTC()
	expired, err := b.repo.ExpiredBudgets(ctx, now)
	if err != nil {
		return repo.Err("mengambil anggaran kedaluwarsa", err)
	}

	for _, bg := range expired {
		if err := b.resetOne(ctx, bg, now); err != nil {
			b.logger.WarnContext(ctx, "gagal memajukan periode anggaran",
				"budget_id", bg.ID, "budget_name", bg.Name, "error", err)
		}
	}

	return nil
}

// resetOne memajukan periode satu anggaran dan menghitung pemakaian riilnya.
func (b *BudgetResetterJob) resetOne(ctx context.Context, bg *policy.Budget, now time.Time) error {
	if bg.PeriodEnd == nil {
		return nil
	}

	newStart := *bg.PeriodEnd
	var newEnd time.Time

	// Majukan rentang periode hingga mencakup waktu sekarang
	for {
		switch bg.Period {
		case policy.PeriodDaily:
			newEnd = newStart.Add(24 * time.Hour)
		case policy.PeriodWeekly:
			newEnd = newStart.Add(7 * 24 * time.Hour)
		case policy.PeriodMonthly:
			newEnd = newStart.AddDate(0, 1, 0)
		default:
			// Periode 'total' atau tidak dikenal tidak direset otomatis
			return nil
		}

		if newEnd.After(now) {
			break
		}
		newStart = newEnd
	}

	// Hitung ulang pemakaian riil dari requests.cost_usd (skala 8 desimal)
	spend, err := b.repo.CalculateSpend(ctx, bg.Scope, bg.ScopeID, newStart, &newEnd)
	if err != nil {
		return repo.Err("menghitung ulang pemakaian", err)
	}

	updated, err := b.repo.ResetPeriodWithSpend(ctx, bg.ID, newStart, &newEnd, spend)
	if err != nil {
		return repo.Err("mereset periode di database", err)
	}

	b.logger.InfoContext(ctx, "periode anggaran berhasil dimajukan",
		"budget_id", updated.ID,
		"budget_name", updated.Name,
		"periode_baru_mulai", newStart.Format(time.RFC3339),
		"periode_baru_akhir", newEnd.Format(time.RFC3339),
		"pemakaian_terhitung_usd", spend.String(),
	)
	return nil
}
