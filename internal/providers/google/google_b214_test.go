package google

import (
	"context"
	"net/http"
	"testing"

	"github.com/NexGen-X/Route-X/internal/providers"
)

// TestHealthCheckB214MenolakBodyBukanDaftarModel membuktikan B-214 di
// mode Gemini standar: body probe 200 yang bukan daftar model (HTML
// proksi, JSON bentuk lain, field models bertipe salah) kini tidak
// sehat. Di kode lama body dibuang tanpa dibaca dan semua 200 sehat.
func TestHealthCheckB214MenolakBodyBukanDaftarModel(t *testing.T) {
	kasus := []struct {
		nama  string
		body  string
		sehat bool
	}{
		{"halaman HTML proksi", `<html><body>selamat datang</body></html>`, false},
		{"JSON bentuk lain", `{"status":"ok"}`, false},
		{"models objek bukan larik", `{"models":{"gemini-2.5-flash":{}}}`, false},
		{"katalog kosong tetap sehat", `{"models":[]}`, true},
		{"daftar model sah", `{"models":[{"name":"models/gemini-2.5-flash"}]}`, true},
	}
	for _, k := range kasus {
		t.Run(k.nama, func(t *testing.T) {
			p, _ := newScriptedServer(t, k.body)
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

// TestHealthCheckB214Antigravity membuktikan B-214 di mode Antigravity:
// katalog fetchAvailableModels berbentuk OBJEK peta model — bentuk itulah
// yang divalidasi, dan body HTML pada status 200 kini tidak sehat.
func TestHealthCheckB214Antigravity(t *testing.T) {
	t.Run("HTML tidak sehat", func(t *testing.T) {
		p, _ := newAntigravityServer(t, "proj-b214", func(w http.ResponseWriter, _ *http.Request) {
			w.Header().Set("Content-Type", "text/html")
			w.WriteHeader(http.StatusOK)
			_, _ = w.Write([]byte(`<html><body>proksi</body></html>`))
		})
		res := p.HealthCheck(context.Background())
		if res.Healthy {
			t.Errorf("Healthy = true untuk body HTML, ingin false (hasil: %+v)", res)
		}
	})

	t.Run("katalog objek sah tetap sehat", func(t *testing.T) {
		p, _ := newAntigravityServer(t, "proj-b214", jsonHandler(http.StatusOK, daftarModelAntigravity))
		res := p.HealthCheck(context.Background())
		if !res.Healthy {
			t.Errorf("Healthy = false untuk katalog objek sah (hasil: %+v)", res)
		}
	})

	t.Run("models larik bukan bentuk Antigravity", func(t *testing.T) {
		p, _ := newAntigravityServer(t, "proj-b214", jsonHandler(http.StatusOK, `{"models":[{"name":"models/x"}]}`))
		res := p.HealthCheck(context.Background())
		if res.Healthy {
			t.Errorf("Healthy = true untuk models berbentuk larik di mode Antigravity, ingin false (hasil: %+v)", res)
		}
	})
}
