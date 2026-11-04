package gateway

// Test regresi B-110: panel/API circuit breaker selalu menampilkan
// failure_count=0 pada breaker TERBUKA, karena pencatat menghapus hash
// breaker (termasuk ember kegagalan f*) saat transisi membuka dan hanya
// menyisakan open_until — handler admin lalu menjumlah field yang sudah
// tidak ada. Pencatat kini menulis open_failures pada transisi membuka;
// test ini mengunci kontrak penyimpanannya di tingkat Redis.

import (
	"testing"
	"time"

	"github.com/NexGen-X/Route-X/internal/cache"
)

// TestBreakerOpenFailuresTersimpanSaatMembukaB110 membuktikan jumlah
// kegagalan pemicu tetap terbaca dari hash sesudah breaker membuka, dan
// hilang lagi begitu breaker menutup (tidak ada angka sisa yang basi).
func TestBreakerOpenFailuresTersimpanSaatMembukaB110(t *testing.T) {
	e := newBreakerEnv(t, cfgUji())
	key := cache.CircuitBreakerKey(breakerProviderA, breakerModelA)

	// cfgUji: MinSamples 4, ambang 0,5 — empat kegagalan membuka breaker.
	e.bukaBreaker(t, breakerProviderA, breakerModelA)

	if got := e.mr.HGet(key, "open_failures"); got != "4" {
		t.Errorf("open_failures sesudah membuka = %q, mau %q (jumlah kegagalan pemicu)", got, "4")
	}
	// Ember f* memang terhapus oleh desain (jendela bersih) — open_failures
	// adalah satu-satunya angka kegagalan yang tersisa, dan itu disengaja.
	if ember, err := e.mr.HKeys(key); err != nil || len(ember) != 2 {
		t.Errorf("field hash breaker terbuka = %v (err %v), mau tepat [open_until open_failures]", ember, err)
	}

	// Probe half-open yang GAGAL membuka lagi: open_failures bertambah
	// satu (probe yang gagal itu sendiri), bukan kembali ke angka basi
	// dan bukan hilang.
	e.advance(cfgUji().OpenDuration + 100*time.Millisecond)
	if izin, state := e.allow(t, breakerProviderA, breakerModelA); !izin || state != StateHalfOpen {
		t.Fatalf("Allow half-open = (%v, %s), mau (true, half_open)", izin, state)
	}
	if err := e.breaker.RecordHasil(t.Context(), breakerProviderA, breakerModelA, false, true); err != nil {
		t.Fatalf("RecordHasil probe gagal: %v", err)
	}
	if state := e.state(t, breakerProviderA, breakerModelA); state != StateOpen {
		t.Fatalf("state sesudah probe gagal = %s, mau open", state)
	}
	if got := e.mr.HGet(key, "open_failures"); got != "5" {
		t.Errorf("open_failures sesudah probe gagal = %q, mau %q", got, "5")
	}

	// Probe half-open yang BERHASIL menutup breaker: hash dibersihkan,
	// open_failures hilang — breaker tertutup tidak melaporkan sisa.
	e.advance(cfgUji().OpenDuration + 100*time.Millisecond)
	if izin, state := e.allow(t, breakerProviderA, breakerModelA); !izin || state != StateHalfOpen {
		t.Fatalf("Allow half-open kedua = (%v, %s), mau (true, half_open)", izin, state)
	}
	if err := e.breaker.RecordHasil(t.Context(), breakerProviderA, breakerModelA, true, true); err != nil {
		t.Fatalf("RecordHasil probe berhasil: %v", err)
	}
	if state := e.state(t, breakerProviderA, breakerModelA); state != StateClosed {
		t.Fatalf("state sesudah probe berhasil = %s, mau closed", state)
	}
	if got := e.mr.HGet(key, "open_failures"); got != "" {
		t.Errorf("open_failures sesudah menutup = %q, mau kosong", got)
	}
}
