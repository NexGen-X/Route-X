package openai

import (
	"context"
	"testing"

	"github.com/NexGen-X/Route-X/internal/providers"
)

// TestHealthCheckB214MenolakBodyBukanDaftarModel membuktikan B-214:
// status 200 saja tidak lagi cukup untuk menyatakan sehat — body probe
// harus berbentuk daftar model dialek ini. Di kode lama kedua kasus
// pertama di bawah ini dinyatakan sehat karena body dibuang tanpa dibaca.
func TestHealthCheckB214MenolakBodyBukanDaftarModel(t *testing.T) {
	kasus := []struct {
		nama  string
		body  string
		sehat bool
	}{
		{"halaman HTML proksi", `<html><head><title>Selamat datang</title></head><body>ok</body></html>`, false},
		{"JSON bentuk lain", `{"status":"ok","version":3}`, false},
		{"data bukan larik", `{"object":"list","data":{"id":"gpt-4o"}}`, false},
		{"bukan JSON sama sekali", `daftar model versi teks`, false},
		{"katalog kosong tetap sehat", `{"object":"list","data":[]}`, true},
		{"daftar model sah", `{"object":"list","data":[{"id":"gpt-4o-mini"}]}`, true},
	}
	for _, k := range kasus {
		t.Run(k.nama, func(t *testing.T) {
			u := newUpstream(t, jsonHandler(200, k.body))
			p := testProvider(t, u.url(), nil)

			result := p.HealthCheck(context.Background())
			if result.Healthy != k.sehat {
				t.Errorf("Healthy = %v untuk body %q, ingin %v (hasil: %+v)", result.Healthy, k.nama, k.sehat, result)
			}
			if !k.sehat && result.ErrorKind != providers.ErrKindServer {
				t.Errorf("ErrorKind = %q untuk %q, ingin %q", result.ErrorKind, k.nama, providers.ErrKindServer)
			}
		})
	}
}
