package usage

import (
	"context"
	"testing"

	"github.com/NexGen-X/Route-X/internal/database/repo/upstream"
	"github.com/NexGen-X/Route-X/internal/router"
)

// TestCostCurrentMelihatHargaSesudahBoot membuktikan akar B-106: cuplikan
// harga hanya dimuat Warm (boot) dan Price, sedangkan jalur pencatatan
// memakai PriceAt (query langsung) — jadi harga yang dibuat SESUDAH boot
// tidak pernah masuk cuplikan dan Cost menjawab "tidak diketahui"
// selamanya. Estimasi reservasi anggaran membaca Cost, sehingga
// reservasinya selalu nol dan penegakan jatuh ke pasca-bayar.
// CostCurrent menyegarkan cuplikan yang kedaluwarsa sebelum menghitung.
func TestCostCurrentMelihatHargaSesudahBoot(t *testing.T) {
	src := &sumberHarga{}
	p := NewPricer(src, testLogger())
	ctx := context.Background()

	// Boot tanpa harga apa pun: cuplikan termuat kosong.
	if err := p.Warm(ctx); err != nil {
		t.Fatalf("Warm: %v", err)
	}
	est := router.TokenEstimate{InputTokens: 1_000_000, OutputTokens: 0}
	if _, ok := p.Cost("pm-1", est); ok {
		t.Fatal("Cost sebelum ada harga harus tidak diketahui")
	}

	// Operator memasang harga sesudah boot (jalur admin setPricing, yang
	// juga membatalkan cuplikan lewat Invalidate).
	src.rows = []*upstream.Price{{
		ProviderModelID: "pm-1",
		Input:           usd(t, "1.5"),
	}}
	p.Invalidate()

	// Cost biasa tetap membaca cuplikan beku — itulah kontraknya untuk
	// router (tanpa context, tanpa database).
	if _, ok := p.Cost("pm-1", est); ok {
		t.Fatal("Cost tidak boleh menyegarkan cuplikan; kontrak router akan berubah")
	}

	// CostCurrent menyegarkan lalu menghitung: 1 juta token masuk × $1,5.
	got, ok := p.CostCurrent(ctx, "pm-1", est)
	if !ok {
		t.Fatal("CostCurrent harus melihat harga yang dibuat sesudah boot")
	}
	if want := usd(t, "1.5"); got != want {
		t.Errorf("CostCurrent = %s, mau %s", got, want)
	}
	if src.baca.Load() < 2 {
		t.Errorf("CurrentAll terbaca %d kali, mau >= 2 (Warm + penyegaran CostCurrent)", src.baca.Load())
	}
}
