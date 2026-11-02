package upstream

import "testing"

// Test regresi sisa B-011: tarif cache-creation 1 jam (2×) diurai dari
// tarif 5 menit (1,25×). Sebelumnya adaptor tidak memecah keduanya dan
// seluruh token creation ditagih 1,25× — tulis cache 1 jam tertagih
// kurang dari tarif resminya.

// TestPriceCostCacheCreationPecahan1h: pecahan TTL penuh (1h + 5m =
// total creation) menghasilkan biaya campuran yang tepat.
func TestPriceCostCacheCreationPecahan1h(t *testing.T) {
	p := priceOf("1.00", "5.00", nil, nil)

	// 400 ribu token 1 jam × 2× + 600 ribu token 5 menit × 1,25× pada
	// harga input $1/1M = $0.80 + $0.75 = $1.55.
	b, err := p.Cost(TokenUsage{CacheCreation: 1_000_000, CacheCreation1h: 400_000, CacheCreation5m: 600_000})
	if err != nil {
		t.Fatalf("Cost: %v", err)
	}
	if b.CacheCreation != 155_000_000 {
		t.Errorf("biaya creation campuran = %d satuan, mau 155000000 ($1.55)", b.CacheCreation)
	}
	if b.Total != 155_000_000 {
		t.Errorf("total = %d, mau 155000000", b.Total)
	}

	// Seluruhnya 1 jam: $2.00.
	b, err = p.Cost(TokenUsage{CacheCreation: 1_000_000, CacheCreation1h: 1_000_000})
	if err != nil {
		t.Fatalf("Cost semua 1 jam: %v", err)
	}
	if b.CacheCreation != 200_000_000 {
		t.Errorf("biaya creation semua 1 jam = %d, mau 200000000 ($2.00)", b.CacheCreation)
	}

	// Seluruhnya 5 menit eksplisit: identik dengan tanpa pecahan, $1.25.
	b, err = p.Cost(TokenUsage{CacheCreation: 1_000_000, CacheCreation5m: 1_000_000})
	if err != nil {
		t.Fatalf("Cost semua 5 menit: %v", err)
	}
	if b.CacheCreation != 125_000_000 {
		t.Errorf("biaya creation semua 5 menit = %d, mau 125000000 ($1.25)", b.CacheCreation)
	}
}

// TestPriceCostCacheCreationPecahanSebagian: pecahan yang jumlahnya
// kurang dari total creation — sisa tanpa atribusi TTL ditagih tarif
// 5 menit, bukan hilang dan bukan ikut tarif 1 jam.
func TestPriceCostCacheCreationPecahanSebagian(t *testing.T) {
	p := priceOf("1.00", "5.00", nil, nil)

	// 200 ribu 1 jam ($0.40) + 300 ribu 5 menit ($0.375) + 500 ribu sisa
	// tanpa atribusi pada tarif 5 menit ($0.625) = $1.40.
	b, err := p.Cost(TokenUsage{CacheCreation: 1_000_000, CacheCreation1h: 200_000, CacheCreation5m: 300_000})
	if err != nil {
		t.Fatalf("Cost: %v", err)
	}
	if b.CacheCreation != 140_000_000 {
		t.Errorf("biaya creation pecahan sebagian = %d, mau 140000000 ($1.40)", b.CacheCreation)
	}
}

// TestPriceCostCacheCreationPecahanTidakKonsisten: laporan provider yang
// pecahannya melewati totalnya dijepit — creation tidak pernah ditagih
// melebihi total yang dilaporkan — dan pecahan negatif dianggap nol.
func TestPriceCostCacheCreationPecahanTidakKonsisten(t *testing.T) {
	p := priceOf("1.00", "5.00", nil, nil)

	// 1h diklaim 300 dari total 100: dijepit ke 100, semuanya tarif 1 jam
	// = 100 × $1/1M × 2 = 20000 satuan.
	b, err := p.Cost(TokenUsage{CacheCreation: 100, CacheCreation1h: 300, CacheCreation5m: 300})
	if err != nil {
		t.Fatalf("Cost: %v", err)
	}
	if b.CacheCreation != 20_000 {
		t.Errorf("biaya creation terjepit = %d, mau 20000", b.CacheCreation)
	}

	// Pecahan negatif: seluruh creation jatuh ke tarif 5 menit.
	b, err = p.Cost(TokenUsage{CacheCreation: 1000, CacheCreation1h: -50, CacheCreation5m: -50})
	if err != nil {
		t.Fatalf("Cost pecahan negatif: %v", err)
	}
	if b.CacheCreation != 125_000 {
		t.Errorf("biaya creation pecahan negatif = %d, mau 125000 (tarif 5 menit)", b.CacheCreation)
	}
}

// TestTokenUsageTotalDenganPecahan: pecahan TTL adalah bagian dari
// CacheCreation, bukan tambahan — Total tidak berubah oleh kehadirannya.
func TestTokenUsageTotalDenganPecahan(t *testing.T) {
	u := TokenUsage{Input: 10, CacheCreation: 100, CacheCreation1h: 40, CacheCreation5m: 60, Output: 5}
	if u.Total() != 115 {
		t.Errorf("Total = %d, mau 115", u.Total())
	}
}
