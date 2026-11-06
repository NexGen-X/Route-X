package policy

import (
	"errors"
	"testing"

	"github.com/NexGen-X/Route-X/internal/database/repo/upstream"
)

// TestB354UpdateTanpaPerubahanMenolak adalah pengunci B-354 sisi
// repo: UpdateBudget dan UpdateRateLimit dulu menjawab params
// kosong dengan baris yang sudah ada (200 idempoten), sementara
// repo keys/upstream menjawab ErrNoChanges yang dipetakan handler
// ke 400 no_changes (B-306). Satu permukaan admin, dua kontrak:
// klien tidak bisa membedakan "permintaanmu kosong" dari "berhasil
// diubah". Kini keduanya menolak dengan ErrNoChanges paket ini.
func TestB354UpdateTanpaPerubahanMenolak(t *testing.T) {
	ctx, r := newRepo(t)

	b, err := r.CreateBudget(ctx, CreateBudgetParams{
		Name: "anggaran-b354", Scope: ScopeGlobal, Period: PeriodMonthly,
		LimitUSD: upstream.MustParseUSD("10"), ActionOnExceed: ActionBlock, AlertThresholdPct: 80,
	})
	if err != nil {
		t.Fatalf("CreateBudget: %v", err)
	}
	if _, err := r.UpdateBudget(ctx, b.ID, UpdateBudgetParams{}); !errors.Is(err, ErrNoChanges) {
		t.Errorf("UpdateBudget(params kosong) = %v, mau ErrNoChanges", err)
	}
	baru := "anggaran-b354-baru"
	if _, err := r.UpdateBudget(ctx, b.ID, UpdateBudgetParams{Name: &baru}); err != nil {
		t.Errorf("UpdateBudget(dengan nama) = %v, mau sukses (kontrol)", err)
	}

	rl, err := r.CreateRateLimit(ctx, CreateRateLimitParams{
		Scope: ScopeGlobal, RequestsPerMinute: pointer(60),
	})
	if err != nil {
		t.Fatalf("CreateRateLimit: %v", err)
	}
	if _, err := r.UpdateRateLimit(ctx, rl.ID, UpdateRateLimitParams{}); !errors.Is(err, ErrNoChanges) {
		t.Errorf("UpdateRateLimit(params kosong) = %v, mau ErrNoChanges", err)
	}
	rpm := 120
	if _, err := r.UpdateRateLimit(ctx, rl.ID, UpdateRateLimitParams{RequestsPerMinute: &rpm}); err != nil {
		t.Errorf("UpdateRateLimit(dengan rpm) = %v, mau sukses (kontrol)", err)
	}
}
