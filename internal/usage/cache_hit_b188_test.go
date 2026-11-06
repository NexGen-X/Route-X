package usage

import (
	"context"
	"testing"
)

// TestRecorderCacheHitBiayaNolB188 adalah pengunci B-188: event cache HIT
// yang membawa ProviderModelID berharga PENUH dan token utuh (kombinasi
// nyata sejak audit3 V-GW-09 memasang pemenang pada jalur HIT) wajib
// tercatat dengan biaya NOL dan tanpa AddSpend — sesuai desain response
// cache: HIT memakan kuota token, bukan biaya. Token pada baris requests
// tetap utuh supaya kuota dan inspektor tidak kehilangan angka, dan HIT
// tidak boleh terhitung sebagai pemetaan tanpa harga.
func TestRecorderCacheHitBiayaNolB188(t *testing.T) {
	r, store, spend, metrics := pencatatUji(t)

	ev := eventLengkap()
	ev.CacheHit = true
	r.Record(context.Background(), ev)
	tutup(t, r)

	baris := store.semua()
	if len(baris) != 1 {
		t.Fatalf("baris tertulis = %d, mau 1", len(baris))
	}
	got := baris[0]
	if got.CostUSD != 0 {
		t.Errorf("cost_usd HIT = %d satuan, mau 0 (tanpa panggilan upstream)", got.CostUSD)
	}
	if got.InputTokens != 1000 || got.OutputTokens != 500 || got.TotalTokens != 1500 {
		t.Errorf("token HIT = %d/%d/%d, mau tetap utuh 1000/500/1500 untuk kuota",
			got.InputTokens, got.OutputTokens, got.TotalTokens)
	}
	if total := spend.total(); len(total) != 0 {
		t.Errorf("AddSpend terpanggil %v untuk HIT, mau tidak sama sekali", total)
	}
	if teks := scrape(t, metrics); nilaiMetrik(t, teks, "routex_usage_unpriced_total{") != 0 {
		t.Errorf("routex_usage_unpriced_total naik untuk cache HIT, mau 0:\n%s", teks)
	}
}
