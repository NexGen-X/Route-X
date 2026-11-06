package google

import (
	"math"
	"testing"
)

// TestB352UsageRaksasaTidakMembungkus adalah pengunci B-352 sisi
// Google: OutputTokens adalah candidates + thoughts dan
// TotalTokens cadangan adalah input + output — keduanya dari
// usageMetadata upstream. Kini lewat TambahToken yang
// menjenuhkan; jumlah polosnya membungkus negatif melewati
// MaxInt.
func TestB352UsageRaksasaTidakMembungkus(t *testing.T) {
	out := toCanonicalUsage(&usageMetadata{
		PromptTokenCount:     10,
		CandidatesTokenCount: math.MaxInt - 1,
		ThoughtsTokenCount:   10,
		TotalTokenCount:      0, // kosong → jalur total cadangan yang dijumlahkan
	})
	if out.OutputTokens != math.MaxInt {
		t.Errorf("OutputTokens = %d, mau jenuh di MaxInt (%d)", out.OutputTokens, math.MaxInt)
	}
	if out.TotalTokens != math.MaxInt {
		t.Errorf("TotalTokens = %d, mau jenuh di MaxInt (%d)", out.TotalTokens, math.MaxInt)
	}

	// Kontrol: total bawaan upstream tetap dipakai apa adanya.
	out = toCanonicalUsage(&usageMetadata{
		PromptTokenCount: 100, CandidatesTokenCount: 30, ThoughtsTokenCount: 5, TotalTokenCount: 999,
	})
	if out.OutputTokens != 35 || out.TotalTokens != 999 {
		t.Errorf("usage wajar = output %d total %d, mau 35/999", out.OutputTokens, out.TotalTokens)
	}
}
