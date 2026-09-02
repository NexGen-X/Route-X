package auth

import (
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/NexGen-X/Route-X/internal/database/repo/identity"
	"github.com/NexGen-X/Route-X/internal/database/seed"
	"github.com/NexGen-X/Route-X/internal/security"
)

// probe adalah handler penanda: ia mencatat apakah middleware meloloskan permintaan, dan
// principal apa yang diterimanya.
type probe struct {
	reached   bool
	principal *Principal
}

func (p *probe) handler() http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		p.reached = true
		p.principal, _ = PrincipalFrom(r.Context())
		w.WriteHeader(http.StatusNoContent)
	})
}

// callWithPrincipal menjalankan middleware dengan principal tertentu di context.
// principal nil berarti request belum melewati RequireSession.
func callWithPrincipal(mw func(http.Handler) http.Handler, p *Principal) (*httptest.ResponseRecorder, *probe) {
	target := &probe{}
	r := httptest.NewRequest(http.MethodGet, "/sumber-daya", nil)
	if p != nil {
		r = r.WithContext(WithPrincipal(r.Context(), p))
	}
	rec := httptest.NewRecorder()
	mw(target.handler()).ServeHTTP(rec, r)
	return rec, target
}

// principalWith merakit principal untuk test otorisasi, tanpa menyentuh database.
func principalWith(permissions ...string) *Principal {
	return &Principal{
		User:        identity.User{ID: "11111111-1111-4111-8111-111111111111", Email: "uji@example.test"},
		SessionID:   "3f3e0f6a-0000-4000-8000-000000000001",
		Permissions: permissions,
		Roles:       []string{seed.RoleOperator},
	}
}

func TestRequirePermission(t *testing.T) {
	t.Run("izin ada", func(t *testing.T) {
		rec, target := callWithPrincipal(RequirePermission(seed.PermProvidersWrite),
			principalWith(seed.PermProvidersRead, seed.PermProvidersWrite))
		if rec.Code != http.StatusNoContent || !target.reached {
			t.Fatalf("status = %d, tercapai = %v", rec.Code, target.reached)
		}
		if target.principal == nil {
			t.Error("principal tidak diteruskan ke handler")
		}
	})

	t.Run("izin tidak ada", func(t *testing.T) {
		rec, target := callWithPrincipal(RequirePermission(seed.PermUsersWrite),
			principalWith(seed.PermProvidersRead))
		if rec.Code != http.StatusForbidden {
			t.Errorf("status = %d, mau 403", rec.Code)
		}
		if target.reached {
			t.Error("handler tercapai padahal izin tidak cukup")
		}
		body := rec.Body.String()
		// Nama izin yang kurang tidak boleh ikut ke respons: bagi pemegang sesi berperan
		// rendah, itu adalah peta kewenangan sistem.
		if strings.Contains(body, seed.PermUsersWrite) {
			t.Errorf("respons membocorkan nama izin yang kurang: %s", body)
		}
		if !strings.Contains(body, "insufficient_permissions") {
			t.Errorf("body = %s, mau memuat kode insufficient_permissions", body)
		}
	})

	// Tanpa principal berarti rute tidak berada di belakang RequireSession: yang kurang
	// adalah autentikasinya, jadi 401 — bukan 403.
	t.Run("tanpa principal", func(t *testing.T) {
		rec, target := callWithPrincipal(RequirePermission(seed.PermUsersRead), nil)
		if rec.Code != http.StatusUnauthorized {
			t.Errorf("status = %d, mau 401", rec.Code)
		}
		if target.reached {
			t.Error("handler tercapai tanpa principal")
		}
		if !strings.Contains(rec.Body.String(), CodeSessionRequired) {
			t.Errorf("body = %s, mau memuat kode %q", rec.Body.String(), CodeSessionRequired)
		}
	})
}

func TestRequireAnyPermission(t *testing.T) {
	cases := []struct {
		name      string
		principal *Principal
		wanted    []string
		wantCode  int
	}{
		{"salah satu ada", principalWith(seed.PermUsageRead), []string{seed.PermRequestsRead, seed.PermUsageRead}, http.StatusNoContent},
		{"tidak ada satu pun", principalWith(seed.PermHealthRead), []string{seed.PermRequestsRead, seed.PermUsageRead}, http.StatusForbidden},
		{"daftar kosong selalu menolak", principalWith(seed.PermUsersWrite), nil, http.StatusForbidden},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			rec, _ := callWithPrincipal(RequireAnyPermission(tc.wanted...), tc.principal)
			if rec.Code != tc.wantCode {
				t.Errorf("status = %d, mau %d", rec.Code, tc.wantCode)
			}
		})
	}

	t.Run("tanpa principal", func(t *testing.T) {
		rec, _ := callWithPrincipal(RequireAnyPermission(seed.PermUsersRead), nil)
		if rec.Code != http.StatusUnauthorized {
			t.Errorf("status = %d, mau 401", rec.Code)
		}
	})
}

// Principal yang wajib mengganti password tetap sah; yang membatasinya adalah middleware
// terpisah, supaya rute ganti password sendiri tetap bisa dipakai.
func TestRequirePasswordChanged(t *testing.T) {
	pending := principalWith(seed.PermHealthRead)
	pending.User.MustChangePassword = true

	rec, target := callWithPrincipal(RequirePasswordChanged(), pending)
	if rec.Code != http.StatusForbidden {
		t.Errorf("status = %d, mau 403", rec.Code)
	}
	if target.reached {
		t.Error("handler tercapai padahal password wajib diganti")
	}
	if !strings.Contains(rec.Body.String(), CodePasswordChangeRequired) {
		t.Errorf("body = %s, mau memuat kode %q", rec.Body.String(), CodePasswordChangeRequired)
	}

	// Izinnya sendiri tidak terpengaruh: principal tetap membawa kewenangan penuh.
	if !pending.Can(seed.PermHealthRead) {
		t.Error("izin principal ikut hilang")
	}

	settled := principalWith(seed.PermHealthRead)
	rec, target = callWithPrincipal(RequirePasswordChanged(), settled)
	if rec.Code != http.StatusNoContent || !target.reached {
		t.Errorf("status = %d, tercapai = %v", rec.Code, target.reached)
	}

	rec, _ = callWithPrincipal(RequirePasswordChanged(), nil)
	if rec.Code != http.StatusUnauthorized {
		t.Errorf("tanpa principal: status = %d, mau 401", rec.Code)
	}
}

func TestPrincipalHelpers(t *testing.T) {
	var nilPrincipal *Principal
	if nilPrincipal.Can(seed.PermUsersRead) || nilPrincipal.CanAny(seed.PermUsersRead) {
		t.Error("principal nil seharusnya tidak punya izin")
	}
	if nilPrincipal.MustChangePassword() || nilPrincipal.TopRole() != "" {
		t.Error("principal nil seharusnya kosong")
	}
	if nilPrincipal.Actor() != (identity.Actor{}) {
		t.Error("Actor() dari principal nil seharusnya kosong")
	}

	p := principalWith(seed.PermUsersRead)
	if p.Can("") {
		t.Error("izin kosong seharusnya tidak pernah cocok")
	}
	actor := p.Actor()
	if actor.UserID != p.User.ID || actor.Email != p.User.Email || actor.Role != seed.RoleOperator {
		t.Errorf("Actor() = %+v", actor)
	}

	// PrincipalFrom pada context tanpa principal.
	if _, ok := PrincipalFrom(httptest.NewRequest(http.MethodGet, "/", nil).Context()); ok {
		t.Error("PrincipalFrom melaporkan ada principal pada context kosong")
	}
	// Principal nil yang ditaruh dengan sengaja tetap dilaporkan tidak ada, supaya tidak
	// ada handler yang menerima penunjuk nil dan menganggapnya sudah terautentikasi.
	ctx := WithPrincipal(httptest.NewRequest(http.MethodGet, "/", nil).Context(), nil)
	if _, ok := PrincipalFrom(ctx); ok {
		t.Error("principal nil dilaporkan ada")
	}
}

// --- RequireSession (butuh database) -----------------------------------------

func TestRequireSession(t *testing.T) {
	e := newEnv(t)
	e.makeUser(t, "sesi@example.test", seed.RoleAdmin)
	res := e.login(t, "sesi@example.test")

	call := func(t *testing.T, token security.Secret) (*httptest.ResponseRecorder, *probe) {
		t.Helper()
		target := &probe{}
		r := httptest.NewRequest(http.MethodGet, "/dashboard", nil)
		if !token.IsZero() {
			r.AddCookie(&http.Cookie{Name: e.svc.Cookies().SessionName(), Value: token.Reveal()})
		}
		rec := httptest.NewRecorder()
		e.svc.RequireSession()(target.handler()).ServeHTTP(rec, r)
		return rec, target
	}

	t.Run("cookie sah", func(t *testing.T) {
		rec, target := call(t, res.Token)
		if rec.Code != http.StatusNoContent || !target.reached {
			t.Fatalf("status = %d, tercapai = %v, body = %s", rec.Code, target.reached, rec.Body)
		}
		if target.principal == nil || target.principal.SessionID != res.Principal.SessionID {
			t.Errorf("principal di handler = %+v", target.principal)
		}
		if !target.principal.Can(seed.PermProvidersWrite) {
			t.Error("izin tidak ikut ke principal di handler")
		}
	})

	// Pesan dan kode untuk semua kegagalan sesi harus sama: tidak ada cookie, token asing,
	// dan sesi yang dicabut tidak boleh bisa dibedakan.
	t.Run("kegagalan sesi tidak dibedakan", func(t *testing.T) {
		revoked := e.login(t, "sesi@example.test")
		if err := e.svc.Logout(e.ctx, revoked.Principal.SessionID, revoked.Principal.Actor(), "", ""); err != nil {
			t.Fatalf("Logout: %v", err)
		}

		bodies := map[string]struct{}{}
		for name, token := range map[string]security.Secret{
			"tanpa cookie":       "",
			"token asing":        security.Secret("token-yang-tidak-pernah-ada"),
			"sesi sudah dicabut": revoked.Token,
		} {
			rec, target := call(t, token)
			if rec.Code != http.StatusUnauthorized {
				t.Errorf("%s: status = %d, mau 401", name, rec.Code)
			}
			if target.reached {
				t.Errorf("%s: handler tercapai", name)
			}
			bodies[rec.Body.String()] = struct{}{}
		}
		if len(bodies) != 1 {
			t.Errorf("kegagalan sesi menghasilkan %d body berbeda, mau tepat 1: %v", len(bodies), bodies)
		}
	})

	// Cookie yang sudah tidak mewakili sesi apa pun dihapus, supaya browser berhenti
	// mengirim token mati pada setiap request berikutnya.
	t.Run("cookie mati dihapus", func(t *testing.T) {
		rec, _ := call(t, security.Secret("token-yang-tidak-pernah-ada"))
		cookies := cookiesOf(t, rec)
		for _, name := range []string{e.svc.Cookies().SessionName(), e.svc.Cookies().CSRFName()} {
			ck := cookies[name]
			if ck == nil {
				t.Fatalf("cookie %q tidak dihapus", name)
			}
			if ck.MaxAge >= 0 || ck.Value != "" {
				t.Errorf("%s: MaxAge = %d, nilai = %q", name, ck.MaxAge, ck.Value)
			}
		}
	})

	// Pengguna yang wajib mengganti password tetap lolos RequireSession — kalau tidak,
	// tidak ada jalan menuju form ganti password itu sendiri.
	t.Run("wajib ganti password tetap lolos", func(t *testing.T) {
		user := e.makeUser(t, "wajib-ganti@example.test", seed.RoleViewer)
		e.exec(t, `update users set must_change_password = true where id = $1`, user.ID)
		pending := e.login(t, "wajib-ganti@example.test")

		rec, target := call(t, pending.Token)
		if rec.Code != http.StatusNoContent || !target.reached {
			t.Fatalf("status = %d, tercapai = %v", rec.Code, target.reached)
		}
		if !target.principal.MustChangePassword() {
			t.Error("principal tidak menandai kewajiban ganti password")
		}
	})
}
