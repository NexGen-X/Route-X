package upstream

import (
	"errors"
	"math"
	"testing"
)

// TestB303TotalTerjagaMenolakLuapan adalah pengunci B-303:
// TokenUsage.Total() dulu penjumlahan int64 polos — komponen yang
// jumlahnya melewati MaxInt64 membungkus menjadi negatif, dan gerbang
// biaya di internal/usage membacanya sebagai "tidak ada token"
// sehingga biaya menjadi 0 secara senyap. TotalTerjaga memakai
// penjumlahan berpenjaga yang sama dengan jumlahUSD (B-259).
func TestB303TotalTerjagaMenolakLuapan(t *testing.T) {
	luapan := TokenUsage{Input: math.MaxInt64, Output: 1}
	if _, err := luapan.TotalTerjaga(); !errors.Is(err, ErrInvalidAmount) {
		t.Errorf("TotalTerjaga komponen luapan = err %v, mau ErrInvalidAmount", err)
	}
	// Kontrol: jumlah normal tidak berubah dari Total() lama.
	normal := TokenUsage{Input: 10, CachedInput: 2, CacheCreation: 3, Output: 5, Reasoning: 1}
	got, err := normal.TotalTerjaga()
	if err != nil {
		t.Fatalf("TotalTerjaga normal: %v", err)
	}
	if got != 21 || got != normal.Total() {
		t.Errorf("TotalTerjaga normal = %d, mau 21 (sama dengan Total lama)", got)
	}
	// Kontrol batas: tepat MaxInt64 masih sah.
	batas := TokenUsage{Input: math.MaxInt64 - 5, Output: 5}
	if got, err := batas.TotalTerjaga(); err != nil || got != math.MaxInt64 {
		t.Errorf("TotalTerjaga tepat-batas = %d, %v; mau MaxInt64 tanpa error", got, err)
	}
}
