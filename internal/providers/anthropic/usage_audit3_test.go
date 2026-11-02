package anthropic

import "testing"

// TestToCanonicalUsageMembawaCacheCreation membuktikan separuh adaptor dari
// V-AB-11: token pembuatan cache Anthropic dibawa sebagai komponen terpisah
// ke pemakaian kanonik, bukan hanya dilipat ke InputTokens (yang akan
// menagihnya 1,0× di hilir).
func TestToCanonicalUsageMembawaCacheCreation(t *testing.T) {
	got := toCanonicalUsage(upstreamUsage{
		InputTokens:              100,
		OutputTokens:             50,
		CacheReadInputTokens:     500,
		CacheCreationInputTokens: 1000,
	})

	if got.CacheCreationInputTokens != 1000 {
		t.Errorf("CacheCreationInputTokens = %d, mau 1000", got.CacheCreationInputTokens)
	}
	// Konvensi kanonik tetap: InputTokens memuat read + creation (bersarang),
	// supaya konsumen lama yang hanya membaca InputTokens tidak kekurangan.
	if got.InputTokens != 1600 {
		t.Errorf("InputTokens = %d, mau 1600 (100 segar + 500 baca + 1000 buat)", got.InputTokens)
	}
	if got.CachedInputTokens != 500 {
		t.Errorf("CachedInputTokens = %d, mau 500", got.CachedInputTokens)
	}
	if got.TotalTokens != 1650 {
		t.Errorf("TotalTokens = %d, mau 1650", got.TotalTokens)
	}
}
