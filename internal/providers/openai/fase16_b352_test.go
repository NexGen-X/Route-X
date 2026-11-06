package openai

import (
	"math"
	"testing"
)

// TestB352UsageRaksasaTidakMembungkus adalah pengunci B-352 sisi
// OpenAI: bila upstream tidak mengirim total_tokens, adapter
// menjumlahkan input + output sendiri. Kini lewat TambahToken
// yang menjenuhkan; jumlah polosnya membungkus negatif melewati
// MaxInt.
func TestB352UsageRaksasaTidakMembungkus(t *testing.T) {
	u := &wireUsage{PromptTokens: math.MaxInt - 1, CompletionTokens: 10, TotalTokens: 0}
	out := u.toCanonical()
	if out.TotalTokens != math.MaxInt {
		t.Errorf("TotalTokens = %d, mau jenuh di MaxInt (%d)", out.TotalTokens, math.MaxInt)
	}

	// Kontrol: total bawaan upstream tetap dipakai apa adanya.
	u = &wireUsage{PromptTokens: 100, CompletionTokens: 30, TotalTokens: 130}
	out = u.toCanonical()
	if out.TotalTokens != 130 {
		t.Errorf("TotalTokens wajar = %d, mau 130", out.TotalTokens)
	}
}
