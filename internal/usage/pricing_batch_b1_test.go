package usage

import (
	"testing"

	"github.com/NexGen-X/Route-X/internal/database/repo/upstream"
	"github.com/NexGen-X/Route-X/internal/providers"
)

// Test regresi sisa B-011 di rantai pencatatan: pecahan TTL cache
// creation mengalir Event → TokensFor → Cost tanpa hilang dan tanpa
// mengubah perilaku bila upstream tidak melaporkannya.

// TestTokensForPecahanCacheCreation: pecahan diteruskan ke TokenUsage
// dengan penjepitan yang sama seperti creation itu sendiri.
func TestTokensForPecahanCacheCreation(t *testing.T) {
	got := TokensFor(providers.Usage{
		InputTokens:                2000,
		CachedInputTokens:          500,
		CacheCreationInputTokens:   1000,
		CacheCreation1hInputTokens: 400,
		CacheCreation5mInputTokens: 600,
		OutputTokens:               300,
	})
	want := upstream.TokenUsage{
		Input: 500, CachedInput: 500,
		CacheCreation: 1000, CacheCreation1h: 400, CacheCreation5m: 600,
		Output: 300,
	}
	if got != want {
		t.Errorf("TokensFor = %+v, mau %+v", got, want)
	}

	// Jumlah pecahan melewati creation: 1 jam dijepit lebih dulu,
	// 5 menit mendapat sisanya.
	got = TokensFor(providers.Usage{
		InputTokens:                1000,
		CacheCreationInputTokens:   1000,
		CacheCreation1hInputTokens: 800,
		CacheCreation5mInputTokens: 800,
	})
	if got.CacheCreation1h != 800 || got.CacheCreation5m != 200 {
		t.Errorf("pecahan terjepit = %d/%d, mau 800/200",
			got.CacheCreation1h, got.CacheCreation5m)
	}
}

// TestEventBiayaCacheCreation1h: ujung ke ujung di dalam paket — Event
// berincian 1h/5m menghasilkan biaya creation campuran yang tepat, dan
// Event tanpa rincian identik dengan tarif lama 1,25×.
func TestEventBiayaCacheCreation1h(t *testing.T) {
	price := &upstream.Price{
		Input:  upstream.MustParseUSD("1.00"),
		Output: upstream.MustParseUSD("5.00"),
	}

	ev := &Event{
		InputTokens:                1_000_000,
		CacheCreationInputTokens:   1_000_000,
		CacheCreation1hInputTokens: 400_000,
		CacheCreation5mInputTokens: 600_000,
	}
	b, err := price.Cost(ev.tokenUsage())
	if err != nil {
		t.Fatalf("Cost: %v", err)
	}
	if b.CacheCreation != 155_000_000 || b.Total != 155_000_000 {
		t.Errorf("biaya = %+v, mau creation/total 155000000 ($1.55)", b)
	}

	evTanpa := &Event{
		InputTokens:              1_000_000,
		CacheCreationInputTokens: 1_000_000,
	}
	b, err = price.Cost(evTanpa.tokenUsage())
	if err != nil {
		t.Fatalf("Cost tanpa rincian: %v", err)
	}
	if b.CacheCreation != 125_000_000 {
		t.Errorf("biaya tanpa rincian = %d, mau 125000000 ($1.25, perilaku lama)", b.CacheCreation)
	}
}
