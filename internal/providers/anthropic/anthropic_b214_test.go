package anthropic

import (
	"context"
	"net/http"
	"testing"

	"github.com/NexGen-X/Route-X/internal/providers"
)

// TestHealthCheckB214MenolakBodyBukanDaftarModel membuktikan B-214 di
// adapter Anthropic: body probe 200 yang bukan daftar model (HTML proksi,
// JSON bentuk lain) kini tidak sehat. Di kode lama body dibuang tanpa
// dibaca dan semua 200 dinyatakan sehat.
func TestHealthCheckB214MenolakBodyBukanDaftarModel(t *testing.T) {
	kasus := []struct {
		nama  string
		body  string
		sehat bool
	}{
		{"halaman HTML proksi", `<html><body>selamat datang di proksi</body></html>`, false},
		{"JSON bentuk lain", `{"status":"ok"}`, false},
		{"data bukan larik", `{"data":{"id":"claude-sonnet-4-5"}}`, false},
		{"daftar model sah", `{"data":[{"id":"claude-sonnet-4-5","type":"model"}]}`, true},
	}
	for _, k := range kasus {
		t.Run(k.nama, func(t *testing.T) {
			p, _ := newServer(t, jsonHandler(http.StatusOK, k.body))
			res := p.HealthCheck(context.Background())
			if res.Healthy != k.sehat {
				t.Errorf("Healthy = %v untuk %q, ingin %v (hasil: %+v)", res.Healthy, k.nama, k.sehat, res)
			}
			if !k.sehat && res.ErrorKind != providers.ErrKindServer {
				t.Errorf("ErrorKind = %q untuk %q, ingin %q", res.ErrorKind, k.nama, providers.ErrKindServer)
			}
		})
	}
}
