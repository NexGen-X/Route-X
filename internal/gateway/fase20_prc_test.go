package gateway

// fase20_prc_test.go — pengunci Fase 20 PR-C (sisi gateway).
//
//	B-461 hasil probe basi lintas episode tidak memutuskan
//	      episode half_open yang lebih baru (token episode).

import (
	"context"
	"testing"
	"time"
)

// TestB461ProbeBasiLintasEpisodeDiabaikan menjalankan skenario
// probe putaran bersih #7 (W1-K4) persis: probe A diberi pada
// episode 1 dan berjalan lama; episode disetel ulang untuk probe
// B; B gagal dan membuka lagi; probe C diberi pada episode
// berikutnya; hasil BASI A (sukses) tiba — pada kode lama ia
// MENUTUP breaker di tengah episode C. Kini token episodenya tidak
// cocok dan hasilnya diabaikan.
func TestB461ProbeBasiLintasEpisodeDiabaikan(t *testing.T) {
	env := newBreakerEnv(t, cfgUji())
	ctx := context.Background()
	b := env.breaker

	// Buka breaker: 4 kegagalan berturut (MinSamples 4, ambang 0,5).
	for i := 0; i < 4; i++ {
		if err := b.Record(ctx, breakerProviderA, breakerModelA, false); err != nil {
			t.Fatalf("Record pembuka: %v", err)
		}
	}
	if izin, state := env.allow(t, breakerProviderA, breakerModelA); izin || state != StateOpen {
		t.Fatalf("sesudah 4 gagal: izin=%v state=%v, mau tertolak open", izin, state)
	}

	izinkanEpisode := func() (bool, State, int64) {
		t.Helper()
		izin, state, token, err := b.IzinkanEpisode(ctx, breakerProviderA, breakerModelA)
		if err != nil {
			t.Fatalf("IzinkanEpisode: %v", err)
		}
		return izin, state, token
	}

	// Episode 1: probe A diberi, token T1.
	env.advance(2 * time.Second) // lewati OpenDuration → half_open
	izin, state, tokenA := izinkanEpisode()
	if !izin || state != StateHalfOpen || tokenA == 0 {
		t.Fatalf("probe A: izin=%v state=%v token=%d, mau probe half_open bertoken", izin, state, tokenA)
	}

	// Episode disetel ulang (probeWait = OpenDuration lewat selagi
	// A masih berjalan): probe B diberi dengan token BERBEDA.
	env.advance(2 * time.Second)
	izin, state, tokenB := izinkanEpisode()
	if !izin || state != StateHalfOpen || tokenB == 0 || tokenB == tokenA {
		t.Fatalf("probe B: izin=%v state=%v token=%d (A=%d), mau episode baru bertoken beda", izin, state, tokenB, tokenA)
	}

	// B gagal → breaker membuka lagi.
	if err := b.RecordHasilEpisode(ctx, breakerProviderA, breakerModelA, false, tokenB); err != nil {
		t.Fatalf("RecordHasilEpisode B: %v", err)
	}
	if izin, state := env.allow(t, breakerProviderA, breakerModelA); izin || state != StateOpen {
		t.Fatalf("sesudah B gagal: izin=%v state=%v, mau open", izin, state)
	}

	// Episode berikutnya: probe C diberi.
	env.advance(2 * time.Second)
	izin, state, tokenC := izinkanEpisode()
	if !izin || state != StateHalfOpen || tokenC == 0 || tokenC == tokenB {
		t.Fatalf("probe C: izin=%v state=%v token=%d, mau probe episode baru", izin, state, tokenC)
	}

	// Hasil basi A tiba (sukses, token episode 1): WAJIB diabaikan.
	if err := b.RecordHasilEpisode(ctx, breakerProviderA, breakerModelA, true, tokenA); err != nil {
		t.Fatalf("RecordHasilEpisode A basi: %v", err)
	}
	izin, state, tokenLagi := izinkanEpisode()
	if izin || state != StateHalfOpen {
		t.Fatalf("sesudah hasil basi A: izin=%v state=%v — breaker tertutup/diizinkan oleh bukti basi (token lagi=%d)", izin, state, tokenLagi)
	}

	// Hasil C yang sesungguhnya (sukses, token cocok) menutup.
	if err := b.RecordHasilEpisode(ctx, breakerProviderA, breakerModelA, true, tokenC); err != nil {
		t.Fatalf("RecordHasilEpisode C: %v", err)
	}
	if izin, state := env.allow(t, breakerProviderA, breakerModelA); !izin || state != StateClosed {
		t.Fatalf("sesudah C sukses: izin=%v state=%v, mau closed", izin, state)
	}
}
