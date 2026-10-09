package auth

import (
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/NexGen-X/Route-X/internal/config"
	"github.com/NexGen-X/Route-X/internal/security"
)

// cookiesOf mengambil cookie yang dipasang sebuah respons, diindeks nama.
func cookiesOf(t *testing.T, rec *httptest.ResponseRecorder) map[string]*http.Cookie {
	t.Helper()
	out := map[string]*http.Cookie{}
	for _, ck := range (&http.Response{Header: rec.Header()}).Cookies() {
		out[ck.Name] = ck
	}
	return out
}

// Atribut cookie sesi harus mengikuti environment dan skema PublicURL, dan namanya ikut
// berubah karena prefiks "__Host-" tidak sah tanpa Secure.
func TestSessionCookieAttributes(t *testing.T) {
	cases := []struct {
		name       string
		env        config.Env
		publicURL  string
		wantName   string
		wantSecure bool
	}{
		{"development", config.EnvDevelopment, "", SessionCookieName, false},
		{"staging", config.EnvStaging, "", SessionCookieName, false},
		{"staging_http", config.EnvStaging, "http://staging.internal", SessionCookieName, false},
		{"staging_https", config.EnvStaging, "https://staging.internal", SessionCookieNameHost, true},
		{"development_https", config.EnvDevelopment, "https://dev.internal", SessionCookieNameHost, true},
		{"production", config.EnvProduction, "", SessionCookieNameHost, true},
		{"production_https", config.EnvProduction, "https://api.routex.ai", SessionCookieNameHost, true},
	}

	const ttl = 3 * time.Hour
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			cookies := NewCookies(&config.Config{AppEnv: tc.env, PublicURL: tc.publicURL}, ttl)

			rec := httptest.NewRecorder()
			cookies.SetSession(rec, security.Secret("token-uji"))
			ck := cookiesOf(t, rec)[tc.wantName]
			if ck == nil {
				t.Fatalf("cookie %q tidak dipasang; header = %q", tc.wantName, rec.Header().Values("Set-Cookie"))
			}

			if ck.Value != "token-uji" {
				t.Errorf("nilai = %q", ck.Value)
			}
			if !ck.HttpOnly {
				t.Error("cookie sesi harus HttpOnly agar skrip halaman tidak bisa mengeluarkan tokennya")
			}
			if ck.Secure != tc.wantSecure {
				t.Errorf("Secure = %v, mau %v", ck.Secure, tc.wantSecure)
			}
			if ck.SameSite != http.SameSiteLaxMode {
				t.Errorf("SameSite = %v, mau Lax", ck.SameSite)
			}
			if ck.Path != "/" {
				t.Errorf("Path = %q, mau /", ck.Path)
			}
			if ck.Domain != "" {
				t.Errorf("Domain = %q, harus kosong (syarat prefiks __Host-)", ck.Domain)
			}
			if ck.MaxAge != int(ttl.Seconds()) {
				t.Errorf("MaxAge = %d, mau %d", ck.MaxAge, int(ttl.Seconds()))
			}
		})
	}
}

// Prefiks "__Host-" hanya boleh muncul di produksi atau saat PublicURL berskema HTTPS:
// di http://localhost browser menolak cookie berprefiks itu secara diam-diam, sehingga
// login tidak akan pernah berhasil.
func TestHostPrefixOnlyInProduction(t *testing.T) {
	dev := NewCookies(&config.Config{AppEnv: config.EnvDevelopment}, time.Hour)
	prod := NewCookies(&config.Config{AppEnv: config.EnvProduction}, time.Hour)
	stagingHTTPS := NewCookies(&config.Config{AppEnv: config.EnvStaging, PublicURL: "https://staging.internal"}, time.Hour)

	for _, name := range []string{dev.SessionName(), dev.CSRFName()} {
		if len(name) >= len(hostPrefix) && name[:len(hostPrefix)] == hostPrefix {
			t.Errorf("nama cookie development %q memakai prefiks %s", name, hostPrefix)
		}
	}
	if prod.SessionName() != SessionCookieNameHost || prod.CSRFName() != CSRFCookieNameHost {
		t.Errorf("nama cookie produksi = %q dan %q", prod.SessionName(), prod.CSRFName())
	}
	if stagingHTTPS.SessionName() != SessionCookieNameHost || stagingHTTPS.CSRFName() != CSRFCookieNameHost {
		t.Errorf("nama cookie staging HTTPS = %q dan %q", stagingHTTPS.SessionName(), stagingHTTPS.CSRFName())
	}

	// cfg nil diperlakukan sebagai non-produksi.
	if NewCookies(nil, time.Hour).SessionName() != SessionCookieName {
		t.Error("cfg nil seharusnya diperlakukan sebagai non-produksi")
	}
}

// Bila PublicURL berskema HTTPS, cookie sesi dan CSRF wajib berprefiks __Host-
// dan beratribut Secure: true bahkan di lingkungan staging atau development.
func TestCookiesWithHTTPSPublicURL(t *testing.T) {
	cases := []struct {
		name       string
		env        config.Env
		publicURL  string
		wantSecure bool
		wantHost   bool
	}{
		{"staging_https", config.EnvStaging, "https://staging.internal", true, true},
		{"development_https", config.EnvDevelopment, "https://dev.internal", true, true},
		{"staging_http", config.EnvStaging, "http://staging.internal", false, false},
		{"development_http", config.EnvDevelopment, "http://localhost:8080", false, false},
		{"production_empty_url", config.EnvProduction, "", true, true},
		{"production_https", config.EnvProduction, "https://api.routex.ai", true, true},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			cookies := NewCookies(&config.Config{AppEnv: tc.env, PublicURL: tc.publicURL}, time.Hour)
			if tc.wantHost {
				if cookies.SessionName() != SessionCookieNameHost {
					t.Errorf("SessionName = %q, mau %q", cookies.SessionName(), SessionCookieNameHost)
				}
				if cookies.CSRFName() != CSRFCookieNameHost {
					t.Errorf("CSRFName = %q, mau %q", cookies.CSRFName(), CSRFCookieNameHost)
				}
			} else {
				if cookies.SessionName() != SessionCookieName {
					t.Errorf("SessionName = %q, mau %q", cookies.SessionName(), SessionCookieName)
				}
				if cookies.CSRFName() != CSRFCookieName {
					t.Errorf("CSRFName = %q, mau %q", cookies.CSRFName(), CSRFCookieName)
				}
			}

			rec := httptest.NewRecorder()
			cookies.SetSession(rec, security.Secret("s-token"))
			cookies.SetCSRF(rec, "c-token")

			cookiesMap := cookiesOf(t, rec)
			sCk := cookiesMap[cookies.SessionName()]
			if sCk == nil {
				t.Fatalf("cookie sesi %q tidak ditemukan", cookies.SessionName())
			}
			if sCk.Secure != tc.wantSecure {
				t.Errorf("sesi Secure = %v, mau %v", sCk.Secure, tc.wantSecure)
			}

			cCk := cookiesMap[cookies.CSRFName()]
			if cCk == nil {
				t.Fatalf("cookie CSRF %q tidak ditemukan", cookies.CSRFName())
			}
			if cCk.Secure != tc.wantSecure {
				t.Errorf("CSRF Secure = %v, mau %v", cCk.Secure, tc.wantSecure)
			}
		})
	}
}

// Cookie CSRF harus bisa dibaca skrip halaman — itu inti pola double-submit — sementara
// cookie sesi tidak boleh.
func TestCSRFCookieIsReadableByScripts(t *testing.T) {
	cookies := NewCookies(&config.Config{AppEnv: config.EnvProduction}, time.Hour)

	rec := httptest.NewRecorder()
	cookies.SetCSRF(rec, "token-csrf")
	ck := cookiesOf(t, rec)[CSRFCookieNameHost]
	if ck == nil {
		t.Fatal("cookie CSRF tidak dipasang")
	}
	if ck.HttpOnly {
		t.Error("cookie CSRF tidak boleh HttpOnly: skrip dashboard harus bisa menyalinnya ke header")
	}
	if !ck.Secure || ck.SameSite != http.SameSiteLaxMode || ck.Path != "/" {
		t.Errorf("atribut cookie CSRF: Secure=%v SameSite=%v Path=%q", ck.Secure, ck.SameSite, ck.Path)
	}
}

// Penghapusan harus mengosongkan nilai DAN memasang Max-Age negatif.
func TestClearCookies(t *testing.T) {
	cookies := NewCookies(&config.Config{AppEnv: config.EnvDevelopment}, time.Hour)

	rec := httptest.NewRecorder()
	cookies.ClearSession(rec)
	cookies.ClearCSRF(rec)

	got := cookiesOf(t, rec)
	for _, name := range []string{SessionCookieName, CSRFCookieName} {
		ck := got[name]
		if ck == nil {
			t.Fatalf("cookie %q tidak dihapus", name)
		}
		if ck.Value != "" {
			t.Errorf("%s: nilai = %q, mau kosong", name, ck.Value)
		}
		if ck.MaxAge >= 0 {
			t.Errorf("%s: MaxAge = %d, mau negatif", name, ck.MaxAge)
		}
	}
}

// Hanya nama yang sesuai environment yang dibaca. Menerima keduanya akan membuat cookie
// non-Secure sisa masa development tetap diterima di produksi.
func TestSessionTokenReadsOnlyMatchingName(t *testing.T) {
	prod := NewCookies(&config.Config{AppEnv: config.EnvProduction}, time.Hour)

	r := httptest.NewRequest(http.MethodGet, "/", nil)
	r.AddCookie(&http.Cookie{Name: SessionCookieName, Value: "token-dev"})
	if got := prod.SessionToken(r); !got.IsZero() {
		t.Errorf("cookie tanpa prefiks diterima di produksi: %q", got.Reveal())
	}

	r.AddCookie(&http.Cookie{Name: SessionCookieNameHost, Value: "token-prod"})
	if got := prod.SessionToken(r); got.Reveal() != "token-prod" {
		t.Errorf("SessionToken = %q, mau %q", got.Reveal(), "token-prod")
	}

	// Cookie kosong sama dengan tidak ada cookie.
	empty := httptest.NewRequest(http.MethodGet, "/", nil)
	empty.AddCookie(&http.Cookie{Name: SessionCookieNameHost, Value: ""})
	if got := prod.SessionToken(empty); !got.IsZero() {
		t.Errorf("cookie kosong dibaca sebagai token %q", got.Reveal())
	}

	// Tanpa cookie sama sekali.
	if got := prod.SessionToken(httptest.NewRequest(http.MethodGet, "/", nil)); !got.IsZero() {
		t.Errorf("tanpa cookie menghasilkan token %q", got.Reveal())
	}
	if got := prod.CSRFToken(httptest.NewRequest(http.MethodGet, "/", nil)); got != "" {
		t.Errorf("tanpa cookie CSRF menghasilkan %q", got)
	}
}

// TTL cookie mengikuti masa berlaku sesi, dan nilai tak wajar jatuh ke bawaan.
func TestCookieTTLFallback(t *testing.T) {
	if got := NewCookies(nil, 0).TTL(); got != DefaultSessionTTL {
		t.Errorf("TTL(0) = %v, mau %v", got, DefaultSessionTTL)
	}
	if got := NewCookies(nil, -time.Hour).TTL(); got != DefaultSessionTTL {
		t.Errorf("TTL(negatif) = %v, mau %v", got, DefaultSessionTTL)
	}
	if got := NewCookies(nil, 90*time.Minute).TTL(); got != 90*time.Minute {
		t.Errorf("TTL = %v", got)
	}
}
