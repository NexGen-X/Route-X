package gateway

import (
	"context"
	"sync"
	"testing"
	"time"
)

// Test regresi audit3 grup 6 (B-024, B-025, B-029) untuk circuit breaker:
// dimensi konfigurasi pada kunci, hasil basi pada episode half_open,
// pasangan event opened/closed, dan reset yang membersihkan varian
// konfigurasi.

// recordProbe mencatat hasil sebuah probe half_open.
func (e *breakerEnv) recordProbe(t *testing.T, provider, model string, sukses bool) {
	t.Helper()
	if err := e.breaker.RecordHasil(context.Background(), provider, model, sukses, true); err != nil {
		t.Fatalf("RecordHasil(%s, %s, sukses=%v, probe): %v", provider, model, sukses, err)
	}
}

// cfgKetatUji adalah konfigurasi yang membuka pada kegagalan pertama dan
// memakai durasi open yang khas (5 detik), supaya campur-aduk dengan
// konfigurasi lain terlihat dari angkanya.
func cfgKetatUji() BreakerConfig {
	return BreakerConfig{
		FailureThreshold: 0.5,
		MinSamples:       1,
		Window:           10 * time.Second,
		OpenDuration:     5 * time.Second,
		HalfOpenProbes:   1,
	}
}

// cfgLonggarUji adalah konfigurasi yang praktis tidak pernah membuka pada
// volume uji (sampel minimum 100).
func cfgLonggarUji() BreakerConfig {
	return BreakerConfig{
		FailureThreshold: 0.5,
		MinSamples:       100,
		Window:           10 * time.Second,
		OpenDuration:     2 * time.Minute,
		HalfOpenProbes:   1,
	}
}

// TestBreakerKonfigurasiBerbedaTidakBerbagiState memverifikasi B-024
// (V-BR-01): dua konfigurasi pada pasangan (provider, model) yang sama
// menilai state masing-masing — aturan ketat yang membuka breaker-nya
// tidak menolak aturan longgar, dan durasi open satu episode adalah milik
// konfigurasi yang membukanya.
func TestBreakerKonfigurasiBerbedaTidakBerbagiState(t *testing.T) {
	e := newBreakerEnv(t, cfgUji())
	ctx := context.Background()
	ketat, longgar := cfgKetatUji(), cfgLonggarUji()

	// Satu kegagalan membuka breaker konfigurasi ketat (sampel minimum 1).
	if err := e.breaker.RecordHasilWithConfig(ctx, breakerProviderA, breakerModelA, false, false, ketat); err != nil {
		t.Fatalf("RecordHasilWithConfig ketat: %v", err)
	}
	izin, state, err := e.breaker.AllowWithConfig(ctx, breakerProviderA, breakerModelA, ketat)
	if err != nil {
		t.Fatalf("AllowWithConfig ketat: %v", err)
	}
	if izin || state != StateOpen {
		t.Fatalf("ketat: izin=%v state=%s, mau (false, open)", izin, state)
	}

	// Konfigurasi longgar pada pasangan yang sama TIDAK ikut tertolak:
	// embernya sendiri belum punya satu pun sampel.
	izin, _, err = e.breaker.AllowWithConfig(ctx, breakerProviderA, breakerModelA, longgar)
	if err != nil {
		t.Fatalf("AllowWithConfig longgar: %v", err)
	}
	if !izin {
		t.Fatal("longgar ikut tertolak oleh breaker milik konfigurasi ketat")
	}

	// Konfigurasi global (jalur Allow biasa) juga tidak ikut tertolak.
	izin, _, err = e.breaker.Allow(ctx, breakerProviderA, breakerModelA)
	if err != nil {
		t.Fatalf("Allow global: %v", err)
	}
	if !izin {
		t.Fatal("konfigurasi global ikut tertolak oleh breaker milik konfigurasi ketat")
	}

	// Kedua breaker hidup di kunci yang berbeda.
	kunciKetat := e.breaker.kunciUntuk(breakerProviderA, breakerModelA, ketat.normalized())
	kunciGlobal := e.breaker.kunciUntuk(breakerProviderA, breakerModelA, e.breaker.cfg)
	if kunciKetat == kunciGlobal {
		t.Fatalf("kunci konfigurasi berbeda sama: %q", kunciKetat)
	}
	if !e.mr.Exists(kunciKetat) {
		t.Errorf("kunci konfigurasi ketat %q tidak ada di Redis", kunciKetat)
	}

	// Episode ketat memakai durasi open milik ketat (5 detik): sesudah 5
	// detik ia boleh di-probe, bukan sesudah durasi konfigurasi lain.
	e.advance(5 * time.Second)
	izin, state, err = e.breaker.AllowWithConfig(ctx, breakerProviderA, breakerModelA, ketat)
	if err != nil {
		t.Fatalf("AllowWithConfig ketat (probe): %v", err)
	}
	if !izin || state != StateHalfOpen {
		t.Fatalf("ketat sesudah 5 dtk: izin=%v state=%s, mau (true, half_open)", izin, state)
	}
}

// TestBreakerRecordBasiTidakMenutupHalfOpen memverifikasi B-025
// (V-BR-03): hasil permintaan lama yang diizinkan saat closed dan baru
// selesai sesudah open_until lewat TIDAK memutuskan episode half_open —
// tidak menutup breaker (sukses basi) dan tidak membukanya lagi (gagal
// basi). Hanya hasil probe yang memutuskan.
func TestBreakerRecordBasiTidakMenutupHalfOpen(t *testing.T) {
	e := newBreakerEnv(t, cfgUji())

	e.bukaBreaker(t, breakerProviderA, breakerModelA)
	e.advance(cfgUji().OpenDuration)
	if state := e.state(t, breakerProviderA, breakerModelA); state != StateHalfOpen {
		t.Fatalf("state = %s, mau half_open sebelum hasil basi tiba", state)
	}

	// Hasil basi sukses: breaker tidak boleh menutup.
	e.record(t, breakerProviderA, breakerModelA, true)
	if state := e.state(t, breakerProviderA, breakerModelA); state != StateHalfOpen {
		t.Fatalf("hasil basi sukses mengubah state menjadi %s, mau tetap half_open", state)
	}

	// Hasil basi gagal: breaker tidak boleh membuka lagi dengan timer baru.
	e.record(t, breakerProviderA, breakerModelA, false)
	if state := e.state(t, breakerProviderA, breakerModelA); state != StateHalfOpen {
		t.Fatalf("hasil basi gagal mengubah state menjadi %s, mau tetap half_open", state)
	}

	// Probe yang sesungguhnya tetap memutuskan episode.
	izin, state := e.allow(t, breakerProviderA, breakerModelA)
	if !izin || state != StateHalfOpen {
		t.Fatalf("izin probe = (%v, %s), mau (true, half_open)", izin, state)
	}
	e.recordProbe(t, breakerProviderA, breakerModelA, true)
	if state := e.state(t, breakerProviderA, breakerModelA); state != StateClosed {
		t.Fatalf("probe berhasil tidak menutup breaker: state = %s", state)
	}
}

// pengumpulTransisi mengumpulkan state yang disampaikan listener, berurutan.
type pengumpulTransisi struct {
	mu     sync.Mutex
	events []State
}

func (p *pengumpulTransisi) dengar(_ context.Context, _, _ string, state State, _, _ int64) {
	p.mu.Lock()
	defer p.mu.Unlock()
	p.events = append(p.events, state)
}

func (p *pengumpulTransisi) ambil() []State {
	p.mu.Lock()
	defer p.mu.Unlock()
	return append([]State(nil), p.events...)
}

// tungguTransisi menunggu sampai pengumpul memuat tepat mau event, atau
// tenggat lewat. Penyampaian listener berjalan di goroutine, jadi test
// tidak boleh mengasumsikannya sinkron.
func tungguTransisi(t *testing.T, p *pengumpulTransisi, mau int) []State {
	t.Helper()
	tenggat := time.Now().Add(5 * time.Second)
	for time.Now().Before(tenggat) {
		if got := p.ambil(); len(got) >= mau {
			return got
		}
		time.Sleep(5 * time.Millisecond)
	}
	return p.ambil()
}

// TestBreakerListenerBerpasanganDanResetMenerbitkanClosed memverifikasi
// B-029 (V-BR-11): satu episode open menghasilkan tepat satu
// circuit.opened; probe yang gagal membuka lagi breaker TIDAK
// menghasilkan opened kedua; pemulihan menghasilkan circuit.closed; dan
// reset manual operator ikut menerbitkan circuit.closed.
func TestBreakerListenerBerpasanganDanResetMenerbitkanClosed(t *testing.T) {
	e := newBreakerEnv(t, cfgUji())
	ctx := context.Background()
	p := &pengumpulTransisi{}
	e.breaker.SetStateChangeListener(p.dengar)
	t.Cleanup(func() { _ = e.breaker.Close(ctx) })

	// Episode pertama: opened tepat sekali.
	e.bukaBreaker(t, breakerProviderA, breakerModelA)
	got := tungguTransisi(t, p, 1)
	if len(got) != 1 || got[0] != StateOpen {
		t.Fatalf("event = %v, mau tepat [open]", got)
	}

	// Probe gagal → breaker membuka lagi, tetapi listener TIDAK mendengar
	// opened kedua: bagi pendengar, episode-nya belum pernah berakhir.
	e.advance(cfgUji().OpenDuration)
	if izin, _ := e.allow(t, breakerProviderA, breakerModelA); !izin {
		t.Fatal("probe tidak diizinkan")
	}
	e.recordProbe(t, breakerProviderA, breakerModelA, false)
	time.Sleep(250 * time.Millisecond)
	if got := p.ambil(); len(got) != 1 {
		t.Fatalf("event sesudah probe gagal = %v, mau tetap tepat 1 (open)", got)
	}

	// Probe berhasil → closed, berpasangan dengan opened di atas.
	e.advance(cfgUji().OpenDuration)
	if izin, _ := e.allow(t, breakerProviderA, breakerModelA); !izin {
		t.Fatal("probe kedua tidak diizinkan")
	}
	e.recordProbe(t, breakerProviderA, breakerModelA, true)
	got = tungguTransisi(t, p, 2)
	if len(got) != 2 || got[1] != StateClosed {
		t.Fatalf("event = %v, mau [open closed]", got)
	}

	// Episode baru sesudah pemulihan: opened lagi (pasangan baru sah).
	e.bukaBreaker(t, breakerProviderA, breakerModelA)
	got = tungguTransisi(t, p, 3)
	if len(got) != 3 || got[2] != StateOpen {
		t.Fatalf("event = %v, mau [open closed open]", got)
	}

	// Reset manual menerbitkan closed — pemulihan oleh operator tidak
	// lagi senyap.
	if err := e.breaker.Reset(ctx, breakerProviderA, breakerModelA); err != nil {
		t.Fatalf("Reset: %v", err)
	}
	got = tungguTransisi(t, p, 4)
	if len(got) != 4 || got[3] != StateClosed {
		t.Fatalf("event = %v, mau [open closed open closed]", got)
	}
}

// TestBreakerResetMenghapusVarianKonfigurasi memverifikasi B-024 dari
// sisi operator: reset atas satu pasangan ikut menutup breaker varian
// konfigurasi per-aturan milik pasangan itu.
func TestBreakerResetMenghapusVarianKonfigurasi(t *testing.T) {
	e := newBreakerEnv(t, cfgUji())
	ctx := context.Background()
	ketat := cfgKetatUji()

	if err := e.breaker.RecordHasilWithConfig(ctx, breakerProviderA, breakerModelA, false, false, ketat); err != nil {
		t.Fatalf("RecordHasilWithConfig: %v", err)
	}
	izin, _, err := e.breaker.AllowWithConfig(ctx, breakerProviderA, breakerModelA, ketat)
	if err != nil {
		t.Fatalf("AllowWithConfig: %v", err)
	}
	if izin {
		t.Fatal("breaker varian belum terbuka, test tidak menyiapkan keadaan yang benar")
	}

	if err := e.breaker.Reset(ctx, breakerProviderA, breakerModelA); err != nil {
		t.Fatalf("Reset: %v", err)
	}

	izin, _, err = e.breaker.AllowWithConfig(ctx, breakerProviderA, breakerModelA, ketat)
	if err != nil {
		t.Fatalf("AllowWithConfig sesudah reset: %v", err)
	}
	if !izin {
		t.Fatal("varian konfigurasi masih menolak sesudah reset melaporkan berhasil")
	}
}
