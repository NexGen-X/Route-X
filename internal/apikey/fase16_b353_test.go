package apikey

import (
	"context"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/NexGen-X/Route-X/internal/database/repo/keys"
)

// TestB353RefundBerjangkarKeJendelaKenaikan adalah pengunci B-353:
// RefundScopes dulu menyusun jendela dari jam SAAT REFUND, bukan jam
// saat kenaikan — refund yang menyeberangi batas jendela menurunkan
// ember BARU (yang bisa memakan hitungan sah di sana) sementara ember
// lama menyimpan hitungan hantu sampai kedaluwarsa. Kini Limit()
// menanam jangkar waktu kenaikannya pada context permintaan, dan
// RefundRequest memakainya: ember yang diturunkan selalu ember yang
// dinaikkan.
func TestB353RefundBerjangkarKeJendelaKenaikan(t *testing.T) {
	env := newLimiterEnv(t)
	rpm := 100
	p := &Principal{Key: &keys.Key{ID: testKeyID, RateLimitRPM: &rpm}}

	// Jalankan middleware Limit() dan tangkap context yang diteruskan
	// ke handler — itulah context yang kelak dipakai KembalikanKuota.
	var ctxTurunan context.Context
	next := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		ctxTurunan = r.Context()
	})
	req := httptest.NewRequest(http.MethodPost, "/v1/chat/completions", nil)
	req = req.WithContext(WithPrincipal(req.Context(), p))
	rec := httptest.NewRecorder()
	env.limiter.Limit()(next).ServeHTTP(rec, req)
	if rec.Code != http.StatusOK {
		t.Fatalf("Limit() menjawab %d, mau 200", rec.Code)
	}
	if ctxTurunan == nil {
		t.Fatal("handler di belakang Limit() tidak terpanggil")
	}

	rpmKeys := kunciJendela(t, env, "rpm-")
	if len(rpmKeys) == 0 {
		t.Fatal("kunci rpm tidak ditemukan sesudah Limit()")
	}
	for _, k := range rpmKeys {
		if v, _ := env.mr.Get(k); v != "1" {
			t.Fatalf("counter rpm %q = %q sesudah Limit(), mau \"1\"", k, v)
		}
	}

	// Seberangi batas jendela menit, lalu refund memakai context
	// permintaan: yang turun harus ember LAMA.
	env.advance(61 * time.Second)
	env.limiter.RefundRequest(ctxTurunan, p, testIP)
	for _, k := range rpmKeys {
		if v, _ := env.mr.Get(k); v != "0" {
			t.Errorf("counter rpm ember kenaikan %q = %q sesudah refund menyeberang jendela, mau \"0\" — refund menurunkan ember yang salah", k, v)
		}
	}
	// Dan tidak boleh ada ember baru yang tercipta/berkurang karenanya:
	// seluruh kunci rpm yang ada hanyalah ember lama yang kini nol.
	for _, k := range kunciJendela(t, env, "rpm-") {
		if v, _ := env.mr.Get(k); v != "0" {
			t.Errorf("kunci rpm %q bernilai %q, mau semua \"0\"", k, v)
		}
	}
}
