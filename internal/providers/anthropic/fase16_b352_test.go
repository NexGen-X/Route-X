package anthropic

import (
	"math"
	"testing"
)

// TestB352UsageRaksasaTidakMembungkus adalah pengunci B-352 sisi
// Anthropic: toCanonicalUsage menjumlahkan input + cache read +
// cache creation lalu + output dari field JSON upstream yang tidak
// dibatasi dekode. Jumlah polosnya membungkus negatif melewati
// MaxInt; kini ketiganya lewat TambahToken yang menjenuhkan, jadi
// hitungan raksasa tetap "sangat besar" — gerbang biaya hilir
// (TotalTerjaga, B-303) yang memutuskan perlakuannya, bukan
// kebungkusan senyap di adapter.
func TestB352UsageRaksasaTidakMembungkus(t *testing.T) {
	out := toCanonicalUsage(upstreamUsage{
		InputTokens:              math.MaxInt - 1,
		CacheReadInputTokens:     10,
		CacheCreationInputTokens: 10,
		OutputTokens:             10,
	})
	if out.InputTokens != math.MaxInt {
		t.Errorf("InputTokens = %d, mau jenuh di MaxInt (%d)", out.InputTokens, math.MaxInt)
	}
	if out.TotalTokens != math.MaxInt {
		t.Errorf("TotalTokens = %d, mau jenuh di MaxInt (%d) — membungkus negatif berarti bocor", out.TotalTokens, math.MaxInt)
	}

	// Kontrol: angka wajar dijumlahkan persis seperti sebelumnya.
	out = toCanonicalUsage(upstreamUsage{
		InputTokens: 100, CacheReadInputTokens: 40, CacheCreationInputTokens: 5, OutputTokens: 30,
	})
	if out.InputTokens != 145 || out.TotalTokens != 175 {
		t.Errorf("usage wajar = input %d total %d, mau 145/175", out.InputTokens, out.TotalTokens)
	}
}
