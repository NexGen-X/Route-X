package auth

import (
	"errors"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/NexGen-X/Route-X/internal/config"
	"github.com/NexGen-X/Route-X/internal/database/seed"
	"github.com/NexGen-X/Route-X/internal/security"
)

// --- B-041 / V-AU-03: header sampah bukan jalan pintas CSRF ------------------

// Kehadiran header Authorization/X-Session-Token — bernilai apa pun — tidak
// boleh melewatkan pemeriksaan CSRF untuk request yang terautentikasi cookie.
// Inilah persis muatan request pada bukti live audit3: cookie sesi sah +
// "Authorization: Basic eHl6" + tanpa token CSRF.
func TestCSRFHeaderSampahTidakMelewati(t *testing.T) {
	const sessionID = "3f3e0f6a-0000-4000-8000-0000000000aa"
	h := newCSRFHarness(t, "http://dashboard.example.test")
	token := h.csrf.Issue(sessionID)

	for name, extra := range map[string]map[string]string{
		"Authorization Basic sampah":  {"Authorization": "Basic eHl6"},
		"Authorization Bearer sampah": {"Authorization": "Bearer token-yang-tidak-pernah-ada"},
		"X-Session-Token sampah":      {"X-Session-Token": "asal-isi"},
	} {
		t.Run(name+" tanpa token CSRF ditolak", func(t *testing.T) {
			rec := h.do(t, csrfRequest{
				sessionID:    sessionID,
				origin:       "http://dashboard.example.test",
				extraHeaders: extra,
			})
			if rec.Code != http.StatusForbidden {
				t.Errorf("status = %d, mau 403", rec.Code)
			}
			if h.reached {
				t.Error("handler tercapai: header sampah melewatkan CSRF")
			}
		})
	}

	// Header sampah tidak merusak jalur sah: request cookie dengan token CSRF
	// yang benar tetap lolos seluruh pemeriksaan.
	t.Run("token CSRF sah tetap lolos meski ada header sampah", func(t *testing.T) {
		rec := h.do(t, csrfRequest{
			sessionID:    sessionID,
			origin:       "http://dashboard.example.test",
			header:       token,
			cookie:       token,
			extraHeaders: map[string]string{"Authorization": "Basic eHl6"},
		})
		if rec.Code != http.StatusNoContent || !h.reached {
			t.Errorf("status = %d, tercapai = %v, mau 204 dan tercapai", rec.Code, h.reached)
		}
	})
}

// Sebaliknya, principal yang BENAR-BENAR terautentikasi lewat token header
// (ViaHeader dari RequireSession, bukan sekadar header hadir) memang kebal
// CSRF dan tidak dituntut token/origin apa pun.
func TestCSRFTokenHeaderTervalidasiDilewati(t *testing.T) {
	h := newCSRFHarness(t, "http://dashboard.example.test")
	rec := h.do(t, csrfRequest{viaHeader: true})
	if rec.Code != http.StatusNoContent || !h.reached {
		t.Errorf("status = %d, tercapai = %v, mau 204 dan tercapai", rec.Code, h.reached)
	}
}

// --- B-041 / V-AU-04: header sampah tidak mengalahkan cookie sah -------------

func TestRequireSessionKredensialHeaderDanCookie(t *testing.T) {
	e := newEnv(t)
	e.makeUser(t, "cadangan@example.test", seed.RoleAdmin)
	res := e.login(t, "cadangan@example.test")

	call := func(t *testing.T, mutate func(r *http.Request)) (*httptest.ResponseRecorder, *probe) {
		t.Helper()
		target := &probe{}
		r := httptest.NewRequest(http.MethodGet, "/dashboard", nil)
		mutate(r)
		rec := httptest.NewRecorder()
		e.svc.RequireSession()(target.handler()).ServeHTTP(rec, r)
		return rec, target
	}
	withCookie := func(r *http.Request) {
		r.AddCookie(&http.Cookie{Name: e.svc.Cookies().SessionName(), Value: res.Token.Reveal()})
	}

	// Header sampah + cookie sah: cookie yang menang, sesi TIDAK dibunuh
	// (tidak ada cookie penghapusan di respons) — versi lama menjawab 401 dan
	// menghapus cookie, yaitu logout paksa oleh header yang tidak sah.
	for name, setHeader := range map[string]func(r *http.Request){
		"X-Session-Token sampah": func(r *http.Request) { r.Header.Set("X-Session-Token", "asal-isi") },
		"Bearer sampah":          func(r *http.Request) { r.Header.Set("Authorization", "Bearer token-asing") },
	} {
		t.Run(name+" kalah dari cookie sah", func(t *testing.T) {
			rec, target := call(t, func(r *http.Request) {
				withCookie(r)
				setHeader(r)
			})
			if rec.Code != http.StatusNoContent || !target.reached {
				t.Fatalf("status = %d, tercapai = %v, body = %s", rec.Code, target.reached, rec.Body)
			}
			if target.principal == nil || target.principal.SessionID != res.Principal.SessionID {
				t.Errorf("principal = %+v, mau sesi dari cookie", target.principal)
			}
			if target.principal.ViaHeader {
				t.Error("principal ditandai ViaHeader padahal yang mengautentikasi cookie")
			}
			if ck := cookiesOf(t, rec)[e.svc.Cookies().SessionName()]; ck != nil && ck.MaxAge < 0 {
				t.Error("cookie sesi ikut dihapus padahal autentikasi berhasil")
			}
		})
	}

	// Token header yang sah mengautentikasi sendiri dan menandai ViaHeader —
	// inilah satu-satunya dasar pengecualian CSRF untuk klien non-browser.
	t.Run("token header sah", func(t *testing.T) {
		rec, target := call(t, func(r *http.Request) {
			r.Header.Set("X-Session-Token", res.Token.Reveal())
		})
		if rec.Code != http.StatusNoContent || !target.reached {
			t.Fatalf("status = %d, tercapai = %v", rec.Code, target.reached)
		}
		if target.principal == nil || !target.principal.ViaHeader {
			t.Errorf("principal = %+v, mau ViaHeader=true", target.principal)
		}
	})

	// Header sampah TANPA cookie tetap ditolak seperti sebelumnya.
	t.Run("header sampah tanpa cookie ditolak", func(t *testing.T) {
		rec, target := call(t, func(r *http.Request) {
			r.Header.Set("X-Session-Token", "asal-isi")
		})
		if rec.Code != http.StatusUnauthorized || target.reached {
			t.Errorf("status = %d, tercapai = %v, mau 401 dan tidak tercapai", rec.Code, target.reached)
		}
	})
}

// --- B-040 / V-AU-02: ganti password berbagi lockout dengan login ------------

func TestChangePasswordBerbagiPenghitungKegagalanLogin(t *testing.T) {
	e := newEnv(t)
	user := e.makeUser(t, "lockout@example.test", seed.RoleAdmin)
	res := e.login(t, "lockout@example.test")

	salah := security.Secret("Password-Yang-Salah-2026")
	baru := security.Secret(testNewPassword)

	// Ambang login adalah 5: lima kegagalan verifikasi password lama harus
	// mengunci akun, bukan hanya menumpuk catatan audit seperti sebelumnya.
	for i := 1; i <= 5; i++ {
		_, err := e.svc.ChangePassword(e.ctx, user.ID, salah, baru,
			res.Principal.SessionID, "203.0.113.10", "uji/1.0")
		if !errors.Is(err, ErrWrongPassword) {
			t.Fatalf("percobaan %d: err = %v, mau ErrWrongPassword", i, err)
		}
	}

	fresh, err := e.users.GetByID(e.ctx, user.ID)
	if err != nil {
		t.Fatalf("GetByID: %v", err)
	}
	if fresh.FailedLoginAttempts != 5 {
		t.Errorf("failed_login_attempts = %d, mau 5", fresh.FailedLoginAttempts)
	}
	if fresh.LockedUntil == nil || !fresh.LockedUntil.After(time.Now()) {
		t.Errorf("locked_until = %v, mau di masa depan", fresh.LockedUntil)
	}

	// Selama terkunci, password lama yang BENAR pun ditolak dengan 429 —
	// verifikasi tidak boleh terus ditebak dari sesi yang sudah masuk.
	_, err = e.svc.ChangePassword(e.ctx, user.ID, security.Secret(testPassword), baru,
		res.Principal.SessionID, "203.0.113.10", "uji/1.0")
	if !errors.Is(err, ErrPasswordChangeThrottled) {
		t.Errorf("saat terkunci: err = %v, mau ErrPasswordChangeThrottled", err)
	}

	// Penghitungnya SATU: login pun melihat akun ini terkunci.
	_, err = e.svc.Login(e.ctx, "lockout@example.test", security.Secret(testPassword), "203.0.113.10", "uji/1.0")
	var loginErr *LoginError
	if !errors.As(err, &loginErr) || loginErr.Reason != ReasonLocked {
		t.Errorf("Login saat terkunci: err = %v, mau LoginError terkunci", err)
	}

	// Sesi yang sudah berjalan TIDAK ikut mati oleh penguncian (kebijakan
	// yang sama dengan Authenticate pada jalur login).
	if _, err := e.svc.Authenticate(e.ctx, res.Token); err != nil {
		t.Errorf("sesi berjalan ikut mati oleh penguncian: %v", err)
	}
}

func TestChangePasswordBerhasilMenolkanPenghitung(t *testing.T) {
	e := newEnv(t)
	user := e.makeUser(t, "nolkan@example.test", seed.RoleViewer)
	res := e.login(t, "nolkan@example.test")

	for i := 0; i < 3; i++ {
		if _, err := e.svc.ChangePassword(e.ctx, user.ID,
			security.Secret("Password-Yang-Salah-2026"), security.Secret(testNewPassword),
			res.Principal.SessionID, "", ""); !errors.Is(err, ErrWrongPassword) {
			t.Fatalf("kegagalan %d: err = %v", i, err)
		}
	}
	if _, err := e.svc.ChangePassword(e.ctx, user.ID,
		security.Secret(testPassword), security.Secret(testNewPassword),
		res.Principal.SessionID, "", ""); err != nil {
		t.Fatalf("ChangePassword sah: %v", err)
	}

	fresh, err := e.users.GetByID(e.ctx, user.ID)
	if err != nil {
		t.Fatalf("GetByID: %v", err)
	}
	if fresh.FailedLoginAttempts != 0 || fresh.LockedUntil != nil {
		t.Errorf("sesudah berhasil: attempts = %d, locked_until = %v, mau 0 dan kosong",
			fresh.FailedLoginAttempts, fresh.LockedUntil)
	}
	// Jejak login tidak boleh berubah oleh ganti password.
	if fresh.LastLoginAt == nil {
		t.Error("last_login_at hilang setelah ganti password")
	}
}

// --- B-042 / V-AU-05: batas sesi bersamaan -----------------------------------

func TestLoginMembatasiSesiBersamaan(t *testing.T) {
	e := newEnv(t, WithMaxSessionsPerUser(3))
	user := e.makeUser(t, "batas-sesi@example.test", seed.RoleAdmin)

	var tokens []security.Secret
	for i := 0; i < 5; i++ {
		tokens = append(tokens, e.login(t, "batas-sesi@example.test").Token)
	}

	active, err := e.sessions.ListActive(e.ctx, user.ID)
	if err != nil {
		t.Fatalf("ListActive: %v", err)
	}
	if len(active) != 3 {
		t.Fatalf("sesi aktif = %d, mau 3 (batas)", len(active))
	}

	// Dua sesi tertua terusir; tiga terbaru tetap sah.
	for i, tok := range tokens[:2] {
		if _, err := e.svc.Authenticate(e.ctx, tok); !errors.Is(err, ErrNoSession) {
			t.Errorf("sesi lama #%d: err = %v, mau ErrNoSession", i, err)
		}
	}
	for i, tok := range tokens[2:] {
		if _, err := e.svc.Authenticate(e.ctx, tok); err != nil {
			t.Errorf("sesi baru #%d seharusnya masih sah: %v", i, err)
		}
	}
}

// --- B-041 / V-AU-07: skema PUBLIC_URL tidak peka huruf ----------------------

func TestCookieSecureSkemaTidakPekaHuruf(t *testing.T) {
	for _, publicURL := range []string{"HTTPS://contoh.test", "Https://contoh.test"} {
		cfg := &config.Config{AppEnv: config.EnvDevelopment, PublicURL: publicURL}
		cookies := NewCookies(cfg, time.Hour)
		if got := cookies.SessionName(); got != SessionCookieNameHost {
			t.Errorf("PUBLIC_URL %q: nama cookie sesi = %q, mau %q (Secure/__Host-)",
				publicURL, got, SessionCookieNameHost)
		}
	}
}
