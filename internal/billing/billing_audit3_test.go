package billing

import (
	"context"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"github.com/NexGen-X/Route-X/internal/database/repo/policy"
	"github.com/NexGen-X/Route-X/internal/database/repo/upstream"
)

// Test regresi audit3 V-AB-16: pemeriksaan anggaran pra-permintaan atas
// akuntansi pasca-bayar terlampaui sebesar konkurensi bila tanpa reservasi
// (bukti live: 10 request paralel menembus pagu 6,3×).

func TestTahanKonkurensiTidakMelampauiPagu(t *testing.T) {
	src := &sumber{budgets: []*policy.Budget{
		anggaran("g", policy.ScopeGlobal, "", "1.00", "0", nil),
	}}
	e := NewEnforcer(src, nil)

	const n = 20
	estimasi := upstream.MustParseUSD("0.10")
	var lolos atomic.Int64
	var wg sync.WaitGroup
	for i := 0; i < n; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			if v := e.Tahan(context.Background(), nil, estimasi); !v.Blocked {
				lolos.Add(1)
			}
		}()
	}
	wg.Wait()

	if lolos.Load() != 10 {
		t.Errorf("yang lolos = %d, mau tepat 10 (pagu $1.00 / estimasi $0.10)", lolos.Load())
	}
}

func TestTahanKedaluwarsaMembebaskanRuang(t *testing.T) {
	now := time.Date(2026, 11, 2, 10, 0, 0, 0, time.UTC)
	src := &sumber{budgets: []*policy.Budget{
		anggaran("g", policy.ScopeGlobal, "", "1.00", "0", nil),
	}}
	e := NewEnforcer(src, nil,
		WithClock(func() time.Time { return now }),
		WithHoldTTL(time.Minute))

	if v := e.Tahan(context.Background(), nil, upstream.MustParseUSD("1.00")); v.Blocked {
		t.Fatal("tahanan pertama ditolak padahal anggaran kosong")
	}
	if v := e.Tahan(context.Background(), nil, upstream.MustParseUSD("1.00")); !v.Blocked {
		t.Fatal("tahanan kedua lolos padahal seluruh pagu sedang ditahan")
	}

	now = now.Add(2 * time.Minute)
	if v := e.Tahan(context.Background(), nil, upstream.MustParseUSD("1.00")); v.Blocked {
		t.Error("tahanan sesudah kedaluwarsa tetap ditolak — ruang tidak dibebaskan")
	}
}

func TestTahanMenghitungSpentYangSudahTercatat(t *testing.T) {
	src := &sumber{budgets: []*policy.Budget{
		anggaran("g", policy.ScopeGlobal, "", "1.00", "0.95", nil),
	}}
	e := NewEnforcer(src, nil)

	if v := e.Tahan(context.Background(), nil, upstream.MustParseUSD("0.10")); !v.Blocked {
		t.Error("estimasi $0.10 di atas sisa $0.05 lolos")
	}
	if v := e.Tahan(context.Background(), nil, upstream.MustParseUSD("0.05")); v.Blocked {
		t.Error("estimasi tepat sebesar sisa ditolak")
	}
}

func TestTahanNolSamaDenganPemeriksaanBiasa(t *testing.T) {
	src := &sumber{budgets: []*policy.Budget{
		anggaran("g", policy.ScopeGlobal, "", "1.00", "1.00", nil),
	}}
	e := NewEnforcer(src, nil)

	if v := e.Tahan(context.Background(), nil, 0); !v.Blocked {
		t.Error("Tahan(0) pada anggaran habis harus memblokir, sama seperti Check")
	}
}
