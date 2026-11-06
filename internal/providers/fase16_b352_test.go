package providers

import (
	"math"
	"testing"
)

// TestB352TambahTokenMenjenuh adalah pengunci B-352 untuk penjaga
// bersama adapter: penjumlahan hitungan token yang melewati batas
// int harus JENUH di batasnya, bukan membungkus ke negatif — angka
// negatif terbaca hilir sebagai "tidak ada token" dan lolos dari
// gerbang biaya yang sudah terjaga (TotalTerjaga, B-303).
func TestB352TambahTokenMenjenuh(t *testing.T) {
	kasus := []struct {
		a, b, mau int
	}{
		{0, 0, 0},
		{3, 4, 7},
		{math.MaxInt - 1, 1, math.MaxInt},
		{math.MaxInt - 1, 5, math.MaxInt},
		{math.MaxInt, math.MaxInt, math.MaxInt},
		{math.MinInt + 1, -5, math.MinInt},
		{-3, -4, -7},
		{math.MaxInt, -1, math.MaxInt - 1},
		{5, 0, 5},
		{0, -5, -5},
	}
	for _, k := range kasus {
		if got := TambahToken(k.a, k.b); got != k.mau {
			t.Errorf("TambahToken(%d, %d) = %d, mau %d", k.a, k.b, got, k.mau)
		}
	}
}
