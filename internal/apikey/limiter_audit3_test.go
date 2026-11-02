package apikey

import (
	"context"
	"net/netip"
	"strconv"
	"strings"
	"testing"

	"github.com/NexGen-X/Route-X/internal/database/repo/keys"
)

// Test regresi audit3 B-013 untuk pembatas laju.

// kunciJendela mencari kunci Redis satu jendela (mis. "rpm-") untuk key uji.
func kunciJendela(t *testing.T, env *limiterEnv, jendela string) []string {
	t.Helper()
	var hasil []string
	for _, k := range env.mr.Keys() {
		if strings.Contains(k, jendela) && strings.Contains(k, testKeyID) {
			hasil = append(hasil, k)
		}
	}
	return hasil
}

// TestLimiterKunciRacunTidakMenghentikanPenegakan membuktikan V-AB-18: satu
// nilai counter non-integer ("abc" — sisa alat eksternal atau korupsi) tidak
// lagi membunuh skrip Lua di tengah Fase 3. Sebelum perbaikan, INCRBY pada
// kunci racun menggagalkan seluruh skrip SETELAH sebagian kunci naik:
// kenaikan parsial plus gerbang gagal-terbuka, dan counter harian membeku
// sementara counter token terus naik (bukti live: daily tertahan di 1,
// daily_tokens 12→36).
func TestLimiterKunciRacunTidakMenghentikanPenegakan(t *testing.T) {
	env := newLimiterEnv(t)
	ctx := context.Background()
	limits := Limits{RPM: 100, Daily: 2}

	if d := env.limiter.Allow(ctx, testKeyID, limits, testIP); !d.Allowed {
		t.Fatalf("request pertama ditolak: %+v", d)
	}

	rpmKeys := kunciJendela(t, env, "rpm-")
	if len(rpmKeys) == 0 {
		t.Fatal("kunci rpm tidak ditemukan sesudah request pertama")
	}
	for _, k := range rpmKeys {
		env.mr.Set(k, "abc")
	}

	// Request kedua: kunci racun dibuang dan dibangun ulang; penegakan jendela
	// lain (harian) tetap berjalan.
	if d := env.limiter.Allow(ctx, testKeyID, limits, testIP); !d.Allowed {
		t.Fatalf("request kedua ditolak oleh kunci racun: %+v", d)
	}
	// Request ketiga WAJIB ditolak pagu harian 2 — inilah bukti penegakan tidak
	// gagal-terbuka. Tanpa perbaikan, skrip mati di rpm dan harian tidak pernah
	// mencapai 2, jadi request ini lolos.
	if d := env.limiter.Allow(ctx, testKeyID, limits, testIP); d.Allowed {
		t.Fatal("request ketiga lolos padahal pagu harian 2 tercapai — penegakan mati oleh kunci racun")
	}
	// Counter rpm pulih menjadi angka (dibangun ulang dari request kedua).
	for _, k := range rpmKeys {
		v, _ := env.mr.Get(k)
		if _, err := strconv.Atoi(v); err != nil {
			t.Errorf("counter rpm %q sesudah pemulihan = %q, mau bilangan bulat", k, v)
		}
	}
}

// TestLimiterRefundMengembalikanKuota membuktikan mekanisme pengembalian
// V-AB-17: RefundRequest menurunkan counter request yang sudah terlanjur
// dinaikkan Limit(), dijepit di nol, dan tidak menciptakan kunci baru.
func TestLimiterRefundMengembalikanKuota(t *testing.T) {
	env := newLimiterEnv(t)
	ctx := context.Background()
	rpm := 100
	p := &Principal{Key: &keys.Key{ID: testKeyID, RateLimitRPM: &rpm}}
	limits := Limits{RPM: rpm}

	for i := 0; i < 3; i++ {
		if d := env.limiter.Allow(ctx, testKeyID, limits, testIP); !d.Allowed {
			t.Fatalf("request %d ditolak: %+v", i+1, d)
		}
	}
	rpmKeys := kunciJendela(t, env, "rpm-")
	if len(rpmKeys) == 0 {
		t.Fatal("kunci rpm tidak ditemukan")
	}
	for _, k := range rpmKeys {
		if v, _ := env.mr.Get(k); v != "3" {
			t.Fatalf("counter rpm %q = %q, mau \"3\" sebelum refund", k, v)
		}
	}

	env.limiter.RefundRequest(ctx, p, testIP)
	for _, k := range rpmKeys {
		if v, _ := env.mr.Get(k); v != "2" {
			t.Errorf("counter rpm %q = %q sesudah satu refund, mau \"2\"", k, v)
		}
	}

	// Refund berlebih dijepit di nol, tidak pernah negatif.
	for i := 0; i < 5; i++ {
		env.limiter.RefundRequest(ctx, p, testIP)
	}
	for _, k := range rpmKeys {
		if v, _ := env.mr.Get(k); v != "0" {
			t.Errorf("counter rpm %q = %q sesudah refund berlebih, mau \"0\"", k, v)
		}
	}

	// Refund untuk key yang belum pernah memakai kuota tidak menciptakan kunci.
	asing := &Principal{Key: &keys.Key{ID: "8c6f0a0e-0000-4000-8000-000000000099", RateLimitRPM: &rpm}}
	env.limiter.RefundRequest(ctx, asing, netip.Addr{})
	for _, k := range env.mr.Keys() {
		if strings.Contains(k, "000000000099") {
			t.Errorf("refund menciptakan kunci untuk key asing: %q", k)
		}
	}
}
