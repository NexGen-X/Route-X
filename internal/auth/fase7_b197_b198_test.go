package auth

import (
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/NexGen-X/Route-X/internal/database/seed"
	"github.com/NexGen-X/Route-X/internal/security"
)

// Test B-197: jalur sesi pada RequireSessionOrBearer (dipakai /metrics)
// wajib mensyaratkan admin. Sesi pengguna biasa yang sah tidak lagi cukup
// untuk men-scrape metrik Prometheus penuh; jalur Bearer-token scraper dan
// penolakan tanpa kredensial tidak berubah.
func TestFase7B197MetricsSesiWajibAdmin(t *testing.T) {
	e := newEnv(t)
	e.makeUser(t, "admin-metrics@example.test", seed.RoleAdmin)
	e.makeUser(t, "viewer-metrics@example.test", seed.RoleViewer)
	adminRes := e.login(t, "admin-metrics@example.test")
	viewerRes := e.login(t, "viewer-metrics@example.test")

	mw := e.svc.RequireSessionOrBearer("token-skraper-uji", false)

	panggil := func(t *testing.T, siapkan func(r *http.Request)) (*httptest.ResponseRecorder, *probe) {
		t.Helper()
		target := &probe{}
		r := httptest.NewRequest(http.MethodGet, "/metrics", nil)
		if siapkan != nil {
			siapkan(r)
		}
		rec := httptest.NewRecorder()
		mw(target.handler()).ServeHTTP(rec, r)
		return rec, target
	}
	denganCookie := func(token security.Secret) func(r *http.Request) {
		return func(r *http.Request) {
			r.AddCookie(&http.Cookie{Name: e.svc.Cookies().SessionName(), Value: token.Reveal()})
		}
	}
	denganHeader := func(token security.Secret) func(r *http.Request) {
		return func(r *http.Request) {
			r.Header.Set("X-Session-Token", token.Reveal())
		}
	}

	t.Run("sesi admin lewat cookie lolos", func(t *testing.T) {
		rec, target := panggil(t, denganCookie(adminRes.Token))
		if rec.Code != http.StatusNoContent || !target.reached {
			t.Fatalf("status = %d, tercapai = %v", rec.Code, target.reached)
		}
	})

	t.Run("sesi pengguna biasa lewat cookie ditolak", func(t *testing.T) {
		rec, target := panggil(t, denganCookie(viewerRes.Token))
		if rec.Code != http.StatusUnauthorized || target.reached {
			t.Fatalf("status = %d, tercapai = %v; mau 401 dan handler tidak tercapai", rec.Code, target.reached)
		}
	})

	t.Run("sesi pengguna biasa lewat header ditolak", func(t *testing.T) {
		rec, target := panggil(t, denganHeader(viewerRes.Token))
		if rec.Code != http.StatusUnauthorized || target.reached {
			t.Fatalf("status = %d, tercapai = %v; mau 401 dan handler tidak tercapai", rec.Code, target.reached)
		}
	})

	t.Run("sesi admin lewat header lolos", func(t *testing.T) {
		rec, target := panggil(t, denganHeader(adminRes.Token))
		if rec.Code != http.StatusNoContent || !target.reached {
			t.Fatalf("status = %d, tercapai = %v", rec.Code, target.reached)
		}
	})

	t.Run("bearer token scraper tetap lolos tanpa sesi", func(t *testing.T) {
		rec, target := panggil(t, func(r *http.Request) {
			r.Header.Set("Authorization", "Bearer token-skraper-uji")
		})
		if rec.Code != http.StatusNoContent || !target.reached {
			t.Fatalf("status = %d, tercapai = %v", rec.Code, target.reached)
		}
	})

	t.Run("tanpa kredensial tetap ditolak", func(t *testing.T) {
		rec, target := panggil(t, nil)
		if rec.Code != http.StatusUnauthorized || target.reached {
			t.Fatalf("status = %d, tercapai = %v", rec.Code, target.reached)
		}
	})
}

// Test B-198: saat touch menggeser kedaluwarsa sesi di database, cookie sesi
// diterbitkan ulang dengan sisa umur yang baru — browser tidak lagi
// membuang cookie pada TTL awal sementara sesi di server masih hidup.
func TestFase7B198CookieSesiDiterbitkanUlangSaatTouch(t *testing.T) {
	e := newEnv(t)
	e.makeUser(t, "geser@example.test", seed.RoleAdmin)
	res := e.login(t, "geser@example.test")

	panggil := func(t *testing.T, siapkan func(r *http.Request)) (*httptest.ResponseRecorder, *probe) {
		t.Helper()
		target := &probe{}
		r := httptest.NewRequest(http.MethodGet, "/dashboard", nil)
		siapkan(r)
		rec := httptest.NewRecorder()
		e.svc.RequireSession()(target.handler()).ServeHTTP(rec, r)
		return rec, target
	}
	cookieSesi := func(rec *httptest.ResponseRecorder) *http.Cookie {
		for _, ck := range rec.Result().Cookies() {
			if ck.Name == e.svc.Cookies().SessionName() {
				return ck
			}
		}
		return nil
	}
	denganCookie := func(r *http.Request) {
		r.AddCookie(&http.Cookie{Name: e.svc.Cookies().SessionName(), Value: res.Token.Reveal()})
	}

	t.Run("jejak segar: cookie tidak diterbitkan ulang", func(t *testing.T) {
		rec, target := panggil(t, denganCookie)
		if rec.Code != http.StatusNoContent || !target.reached {
			t.Fatalf("status = %d, tercapai = %v", rec.Code, target.reached)
		}
		if ck := cookieSesi(rec); ck != nil {
			t.Fatalf("cookie sesi diterbitkan ulang padahal touch belum menembak: %+v", ck)
		}
	})

	// Paksa jejak sesi cukup tua agar touch berikutnya benar-benar menembak.
	e.exec(t, `update sessions set last_seen_at = now() - interval '10 minutes' where id = $1`,
		res.Principal.SessionID)

	t.Run("sesudah touch: cookie diterbitkan ulang dengan sisa umur penuh", func(t *testing.T) {
		rec, target := panggil(t, denganCookie)
		if rec.Code != http.StatusNoContent || !target.reached {
			t.Fatalf("status = %d, tercapai = %v", rec.Code, target.reached)
		}
		ck := cookieSesi(rec)
		if ck == nil {
			t.Fatal("cookie sesi tidak diterbitkan ulang sesudah touch menggeser kedaluwarsa")
		}
		if ck.Value != res.Token.Reveal() {
			t.Error("nilai cookie terbitan ulang bukan token sesi yang sama")
		}
		// Sisa umur harus mendekati TTL sesi penuh (12 jam bawaan), bukan
		// sisa TTL awal yang hampir habis.
		if ck.MaxAge < 3600 {
			t.Errorf("MaxAge cookie = %d detik, mau mendekati TTL penuh sesudah digeser", ck.MaxAge)
		}
		if !ck.HttpOnly {
			t.Error("cookie terbitan ulang kehilangan HttpOnly")
		}
	})

	t.Run("jalur header tidak memasang cookie", func(t *testing.T) {
		e.exec(t, `update sessions set last_seen_at = now() - interval '10 minutes' where id = $1`,
			res.Principal.SessionID)
		rec, target := panggil(t, func(r *http.Request) {
			r.Header.Set("X-Session-Token", res.Token.Reveal())
		})
		if rec.Code != http.StatusNoContent || !target.reached {
			t.Fatalf("status = %d, tercapai = %v", rec.Code, target.reached)
		}
		if ck := cookieSesi(rec); ck != nil {
			t.Fatalf("jalur header memasang cookie sesi: %+v", ck)
		}
	})
}
