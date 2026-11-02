package upstream

import (
	"context"
	"testing"
	"time"
)

// Test regresi audit3 B-011 untuk repo harga.

// TestPriceCostCacheCreation: token pembuatan cache ditagih 1,25× tarif input
// (tarif tulis cache 5 menit Anthropic), bukan dilipat ke input pada 1,0×
// (audit3 V-AB-11). Bukti live sebelum perbaikan: 1000 token creation
// tertagih $0.00115 padahal tarif resminya $0.0014.
func TestPriceCostCacheCreation(t *testing.T) {
	p := priceOf("1.00", "5.00", nil, nil)

	b, err := p.Cost(TokenUsage{CacheCreation: 1_000_000})
	if err != nil {
		t.Fatalf("Cost: %v", err)
	}
	if b.CacheCreation != 125_000_000 {
		t.Errorf("biaya creation 1M token = %d satuan, mau 125000000 ($1.25)", b.CacheCreation)
	}
	if b.Total != 125_000_000 {
		t.Errorf("total = %d, mau 125000000", b.Total)
	}

	campur, err := p.Cost(TokenUsage{Input: 1_000_000, CacheCreation: 1_000_000})
	if err != nil {
		t.Fatalf("Cost campuran: %v", err)
	}
	if campur.Input != 100_000_000 || campur.CacheCreation != 125_000_000 || campur.Total != 225_000_000 {
		t.Errorf("campuran = %+v, mau input 100000000 + creation 125000000 = 225000000", campur)
	}

	if nol, err := p.Cost(TokenUsage{}); err != nil || nol.Total != 0 || nol.CacheCreation != 0 {
		t.Errorf("tanpa token = %+v, mau nol", nol)
	}
	// Pembulatan setengah ke atas pada tarif berskala: 1 token creation pada
	// harga $1/1M = 125 satuan mentah / 100... tepatnya 1×100×5/4 = 125.
	if satu, err := p.Cost(TokenUsage{CacheCreation: 1}); err != nil || satu.CacheCreation != 125 {
		t.Errorf("biaya 1 token creation = %d satuan, mau 125", satu.CacheCreation)
	}
}

// TestPricingCurrentMengabaikanHargaMasaDepan membuktikan V-AB-08: harga
// terjadwal (effective_from di masa depan) TIDAK berlaku dini. Sebelum
// perbaikan, Current/CurrentAll hanya menyaring effective_to is null sehingga
// harga masa depan menimpa harga yang sedang berlaku di seluruh penagihan.
func TestPricingCurrentMengabaikanHargaMasaDepan(t *testing.T) {
	ctx := context.Background()
	pool := newTestPool(ctx, t)
	pricing := NewPricingRepo(pool)
	admin := seedUser(ctx, t, pool)

	model := seedModel(ctx, t, pool, "gpt-jadwal", CapText)
	provider := seedProvider(ctx, t, pool, "openai")
	pm := seedAttachment(ctx, t, pool, model.ID, provider.ID, nil)

	now := time.Now().UTC().Truncate(time.Microsecond)
	if _, err := SetPrice(ctx, pool, SetPriceParams{
		ProviderModelID: pm.ID,
		Input:           MustParseUSD("1.00"),
		Output:          MustParseUSD("2.00"),
		EffectiveFrom:   now.Add(-time.Hour),
		CreatedBy:       &admin,
	}); err != nil {
		t.Fatalf("SetPrice berlaku: %v", err)
	}
	if _, err := SetPrice(ctx, pool, SetPriceParams{
		ProviderModelID: pm.ID,
		Input:           MustParseUSD("99.00"),
		Output:          MustParseUSD("99.00"),
		EffectiveFrom:   now.Add(48 * time.Hour),
		CreatedBy:       &admin,
	}); err != nil {
		t.Fatalf("SetPrice terjadwal: %v", err)
	}

	cur, err := pricing.Current(ctx, pm.ID)
	if err != nil {
		t.Fatalf("Current: %v", err)
	}
	if cur.Input != MustParseUSD("1.00") {
		t.Errorf("Current input = %s, mau 1.00 — harga masa depan berlaku dini", cur.Input)
	}

	all, err := pricing.CurrentAll(ctx)
	if err != nil {
		t.Fatalf("CurrentAll: %v", err)
	}
	var untukPM []*Price
	for _, p := range all {
		if p.ProviderModelID == pm.ID {
			untukPM = append(untukPM, p)
		}
	}
	if len(untukPM) != 1 || untukPM[0].Input != MustParseUSD("1.00") {
		t.Errorf("CurrentAll untuk pemetaan = %+v, mau tepat satu harga 1.00", untukPM)
	}

	// Harga terjadwal tetap utuh untuk waktunya: At di masa depan menemukannya.
	nanti, err := pricing.At(ctx, pm.ID, now.Add(72*time.Hour))
	if err != nil {
		t.Fatalf("At masa depan: %v", err)
	}
	if nanti.Input != MustParseUSD("99.00") {
		t.Errorf("At(+72j) input = %s, mau 99.00", nanti.Input)
	}
}
