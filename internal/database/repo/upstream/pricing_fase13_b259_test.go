package upstream

import (
	"errors"
	"testing"
)

// B-259: penjumlahan Total antar-komponen di Price.Cost sebelumnya int64
// polos — dua komponen yang masing-masing sah dapat menjumlah melewati
// MaxInt64 dan membungkus menjadi Total NEGATIF tanpa error, melanggar
// kontrak costOf sendiri. Total negatif yang lolos berarti Recorder
// mencatat biaya negatif dan AddSpend negatif mengikis anggaran.
func TestPriceCostTotalMeluberDitolakB259(t *testing.T) {
	// Harga per juta token sebesar 5e18 satuan: dengan tepat 1 juta token,
	// biaya per komponen = harga itu sendiri (5e18 < MaxInt64, sah per
	// komponen), tetapi input + output = 1e19 > MaxInt64.
	p := Price{
		Input:    USD(5_000_000_000_000_000_000),
		Output:   USD(5_000_000_000_000_000_000),
		Currency: DefaultCurrency,
	}
	_, err := p.Cost(TokenUsage{Input: 1_000_000, Output: 1_000_000})
	if err == nil {
		t.Fatal("Cost menerima penjumlahan Total yang meluber; ingin error")
	}
	if !errors.Is(err, ErrInvalidAmount) {
		t.Fatalf("error = %v; ingin membungkus ErrInvalidAmount", err)
	}
}

// Kontrol: penjumlahan besar yang masih di dalam rentang int64 tetap
// diterima persis — penjaga luapan tidak boleh menolak angka sah.
func TestPriceCostTotalBesarSahB259(t *testing.T) {
	p := Price{
		Input:    USD(4_000_000_000_000_000_000),
		Output:   USD(4_000_000_000_000_000_000),
		Currency: DefaultCurrency,
	}
	out, err := p.Cost(TokenUsage{Input: 1_000_000, Output: 1_000_000})
	if err != nil {
		t.Fatalf("Cost menolak total sah: %v", err)
	}
	if out.Total != USD(8_000_000_000_000_000_000) {
		t.Fatalf("Total = %d; ingin 8000000000000000000", int64(out.Total))
	}
}
