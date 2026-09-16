package auth

import (
	"bytes"
	"encoding/json"
	"github.com/NexGen-X/Route-X/internal/database/seed"
	"github.com/NexGen-X/Route-X/internal/httpx"
	"github.com/NexGen-X/Route-X/internal/observability"
	"github.com/NexGen-X/Route-X/internal/security"
	"io"
	"net/http"
	"net/http/cookiejar"
	"net/http/httptest"
	"net/url"
	"strings"
	"testing"
)

// testRequestID dipasang di setiap request test supaya jalur korelasi audit ↔ log ikut
// terlatih, sama seperti yang dilakukan httpx.RequestID di produksi.
const testRequestID = "req-uji-0001"

// client adalah klien HTTP untuk router autentikasi, lengkap dengan penyimpan cookie
// sehingga alur login → aksi → logout berjalan seperti di browser.
type client struct {
	t    *testing.T
	base string
	http *http.Client
	// csrf adalah token terakhir yang diterbitkan server, disalin ke header pada setiap
	// permintaan pengubah state — persis yang dilakukan skrip dashboard.
	csrf string
}

// newClient menyalakan server test berisi Handlers.Routes() dan klien untuk memanggilnya.
//
// Logger dan request ID ditautkan ke context lewat middleware kecil, meniru apa yang
// dilakukan httpx.RequestID: tanpa itu, log dari handler dan middleware jatuh ke logger
// default proses dan tidak bisa diperiksa test kebocoran rahasia.
func (e *env) newClient(t *testing.T) *client {
	t.Helper()

	logger := observability.NewLogger(e.logs, observability.LoggerOptions{})
	routes := NewHandlers(e.svc).Routes()
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		ctx := observability.WithRequestID(r.Context(), testRequestID)
		ctx = observability.WithLogger(ctx, logger)
		routes.ServeHTTP(w, r.WithContext(ctx))
	}))
	t.Cleanup(srv.Close)

	jar, err := cookiejar.New(nil)
	if err != nil {
		t.Fatalf("cookiejar: %v", err)
	}
	return &client{t: t, base: srv.URL, http: &http.Client{Jar: jar}}
}

// do mengirim satu permintaan. body nil berarti tanpa body dan tanpa Content-Type.
func (c *client) do(method, path string, body any, tweak ...func(*http.Request)) *http.Response {
	c.t.Helper()

	var reader io.Reader
	if body != nil {
		raw, err := json.Marshal(body)
		if err != nil {
			c.t.Fatalf("menyusun body: %v", err)
		}
		reader = bytes.NewReader(raw)
	}

	req, err := http.NewRequest(method, c.base+path, reader)
	if err != nil {
		c.t.Fatalf("menyusun request: %v", err)
	}
	if body != nil {
		req.Header.Set("Content-Type", "application/json")
	}
	// Origin diisi dengan origin server itu sendiri: PUBLIC_URL kosong di test, jadi
	// pemeriksaan CSRF membandingkannya dengan host request.
	req.Header.Set("Origin", c.base)
	if c.csrf != "" {
		req.Header.Set(HeaderCSRFToken, c.csrf)
	}
	for _, fn := range tweak {
		fn(req)
	}

	res, err := c.http.Do(req)
	if err != nil {
		c.t.Fatalf("%s %s: %v", method, path, err)
	}
	return res
}

// decode membaca body JSON dan menutup respons.
func decode[T any](t *testing.T, res *http.Response) T {
	t.Helper()
	defer res.Body.Close()
	var out T
	raw, err := io.ReadAll(res.Body)
	if err != nil {
		t.Fatalf("membaca body: %v", err)
	}
	if err := json.Unmarshal(raw, &out); err != nil {
		t.Fatalf("membaca body %q: %v", raw, err)
	}
	return out
}

// bodyOf membaca body sebagai teks dan menutup respons.
func bodyOf(t *testing.T, res *http.Response) string {
	t.Helper()
	defer res.Body.Close()
	raw, err := io.ReadAll(res.Body)
	if err != nil {
		t.Fatalf("membaca body: %v", err)
	}
	return string(raw)
}

// login memanggil POST /login dan menyimpan token CSRF-nya.
func (c *client) login(email, password string) *http.Response {
	c.t.Helper()
	res := c.do(http.MethodPost, "/login", map[string]string{"email": email, "password": password})
	return res
}

// errorEnvelope adalah bentuk balasan gagal dari httpx.
type errorEnvelope struct {
	Error struct {
		Message   string  `json:"message"`
		Type      string  `json:"type"`
		Code      *string `json:"code"`
		RequestID string  `json:"request_id"`
	} `json:"error"`
}

// Alur lengkap: login memasang cookie, /me mengembalikan identitas yang sama,
// /change-password bekerja dengan token CSRF, dan /logout membersihkan cookie.
func TestHandlersFullFlow(t *testing.T) {
	e := newEnv(t)
	user := e.makeUser(t, "admin@example.test", seed.RoleSuperAdmin)
	c := e.newClient(t)

	res := c.login("admin@example.test", testPassword)
	if res.StatusCode != http.StatusOK {
		t.Fatalf("POST /login = %d: %s", res.StatusCode, bodyOf(t, res))
	}
	// Cookie sesi harus HttpOnly, cookie CSRF harus terbaca skrip.
	var sawSession, sawCSRF bool
	for _, ck := range res.Cookies() {
		switch ck.Name {
		case SessionCookieName:
			sawSession = true
			if !ck.HttpOnly {
				t.Error("cookie sesi tidak HttpOnly")
			}
		case CSRFCookieName:
			sawCSRF = true
			if ck.HttpOnly {
				t.Error("cookie CSRF tidak boleh HttpOnly")
			}
		}
	}
	if !sawSession || !sawCSRF {
		t.Fatalf("cookie sesi terpasang = %v, cookie CSRF terpasang = %v", sawSession, sawCSRF)
	}

	body := decode[PrincipalResponse](t, res)
	if body.User.ID != user.ID || body.User.Email != "admin@example.test" {
		t.Errorf("pengguna di balasan = %+v", body.User)
	}
	if body.CSRFToken == "" {
		t.Fatal("csrf_token kosong di balasan login")
	}
	if body.Session.ID == "" || body.Session.ExpiresAt.IsZero() {
		t.Errorf("sesi di balasan = %+v", body.Session)
	}
	if len(body.Roles) != 1 || body.Roles[0] != "Admin" {
		t.Errorf("peran = %v", body.Roles)
	}
	c.csrf = body.CSRFToken

	// GET /me mengembalikan bentuk yang sama.
	me := decode[PrincipalResponse](t, c.do(http.MethodGet, "/me", nil))
	if me.User.ID != user.ID || me.Session.ID != body.Session.ID {
		t.Errorf("GET /me = %+v", me)
	}
	if me.CSRFToken != body.CSRFToken {
		t.Error("token CSRF berubah antara login dan /me padahal sesinya sama")
	}

	// Ganti password: sesi ini dipertahankan, jadi permintaan berikutnya masih berhasil.
	changed := c.do(http.MethodPost, "/change-password", map[string]string{
		"current_password": testPassword,
		"new_password":     testNewPassword,
	})
	if changed.StatusCode != http.StatusOK {
		t.Fatalf("POST /change-password = %d: %s", changed.StatusCode, bodyOf(t, changed))
	}
	status := decode[StatusResponse](t, changed)
	if status.Status != StatusPasswordChanged {
		t.Errorf("status = %q", status.Status)
	}
	if res := c.do(http.MethodGet, "/me", nil); res.StatusCode != http.StatusOK {
		t.Errorf("GET /me setelah ganti password = %d: %s", res.StatusCode, bodyOf(t, res))
	}

	// Logout mencabut sesi dan menghapus cookie.
	out := c.do(http.MethodPost, "/logout", nil)
	if out.StatusCode != http.StatusOK {
		t.Fatalf("POST /logout = %d: %s", out.StatusCode, bodyOf(t, out))
	}
	if got := decode[StatusResponse](t, out).Status; got != StatusLoggedOut {
		t.Errorf("status = %q", got)
	}
	if res := c.do(http.MethodGet, "/me", nil); res.StatusCode != http.StatusUnauthorized {
		t.Errorf("GET /me setelah logout = %d, mau 401", res.StatusCode)
	}

	// request_id dari context ikut ke catatan audit, sehingga satu keluhan bisa dipetakan
	// dari log ke audit dan sebaliknya.
	var requestID string
	err := e.pool.QueryRow(e.ctx, `select coalesce(request_id, '') from audit_logs
		where action = 'auth.login' order by id limit 1`).Scan(&requestID)
	if err != nil {
		t.Fatalf("membaca request_id audit: %v", err)
	}
	if requestID != testRequestID {
		t.Errorf("request_id di audit = %q, mau %q", requestID, testRequestID)
	}
}

// Ketiga kegagalan login harus menghasilkan respons yang identik byte per byte — status,
// kode, dan pesan — supaya tidak ada yang bisa dipakai membedakan akun yang ada.
func TestHandlersLoginFailuresIdentical(t *testing.T) {
	e := newEnv(t)
	e.makeUser(t, "ada@example.test", seed.RoleViewer)
	e.makeUser(t, "terkunci@example.test", seed.RoleViewer)
	e.exec(t, `update users set locked_until = now() + interval '10 minutes',
		failed_login_attempts = 5 where email = 'terkunci@example.test'`)
	e.makeUser(t, "mati@example.test", seed.RoleViewer)
	e.exec(t, `update users set status = 'disabled' where email = 'mati@example.test'`)

	c := e.newClient(t)
	attempts := map[string][2]string{
		"email tidak ada": {"hantu@example.test", testPassword},
		"password salah":  {"ada@example.test", "Password-Yang-Salah-2026"},
		"akun terkunci":   {"terkunci@example.test", testPassword},
		"akun dimatikan":  {"mati@example.test", testPassword},
	}

	bodies := map[string]struct{}{}
	for name, attempt := range attempts {
		res := c.login(attempt[0], attempt[1])
		if res.StatusCode != http.StatusUnauthorized {
			t.Errorf("%s: status = %d, mau 401", name, res.StatusCode)
		}
		if len(res.Cookies()) > 0 {
			t.Errorf("%s: kegagalan login memasang cookie: %v", name, res.Cookies())
		}
		env := decode[errorEnvelope](t, res)
		if env.Error.Message != MessageLoginFailed {
			t.Errorf("%s: pesan = %q, mau %q", name, env.Error.Message, MessageLoginFailed)
		}
		if env.Error.Code == nil || *env.Error.Code != CodeLoginFailed {
			t.Errorf("%s: kode = %v, mau %q", name, env.Error.Code, CodeLoginFailed)
		}
		bodies[env.Error.Message+"|"+*env.Error.Code] = struct{}{}
	}
	if len(bodies) != 1 {
		t.Errorf("kegagalan login menghasilkan %d respons berbeda, mau tepat 1: %v", len(bodies), bodies)
	}
}

func TestHandlersRequestValidation(t *testing.T) {
	e := newEnv(t)
	e.makeUser(t, "ada@example.test", seed.RoleViewer)
	c := e.newClient(t)

	t.Run("bukan JSON", func(t *testing.T) {
		res := c.do(http.MethodPost, "/login", map[string]string{"email": "a@b.test", "password": "x"},
			func(r *http.Request) { r.Header.Set("Content-Type", "application/x-www-form-urlencoded") })
		if res.StatusCode != http.StatusUnsupportedMediaType {
			t.Errorf("status = %d, mau 415", res.StatusCode)
		}
		if !strings.Contains(bodyOf(t, res), CodeUnsupportedMediaType) {
			t.Error("kode error tidak menyebut media type")
		}
	})

	t.Run("tanpa Content-Type", func(t *testing.T) {
		req, _ := http.NewRequest(http.MethodPost, c.base+"/login", strings.NewReader(`{}`))
		res, err := c.http.Do(req)
		if err != nil {
			t.Fatalf("request: %v", err)
		}
		if res.StatusCode != http.StatusUnsupportedMediaType {
			t.Errorf("status = %d, mau 415", res.StatusCode)
		}
		res.Body.Close()
	})

	t.Run("JSON rusak", func(t *testing.T) {
		req, _ := http.NewRequest(http.MethodPost, c.base+"/login", strings.NewReader(`{"email":`))
		req.Header.Set("Content-Type", "application/json")
		res, err := c.http.Do(req)
		if err != nil {
			t.Fatalf("request: %v", err)
		}
		if res.StatusCode != http.StatusBadRequest {
			t.Errorf("status = %d, mau 400", res.StatusCode)
		}
		res.Body.Close()
	})

	t.Run("field wajib kosong", func(t *testing.T) {
		for _, body := range []map[string]string{
			{"email": "", "password": testPassword},
			{"email": "ada@example.test", "password": ""},
			{},
		} {
			res := c.do(http.MethodPost, "/login", body)
			if res.StatusCode != http.StatusBadRequest {
				t.Errorf("body %v: status = %d, mau 400", body, res.StatusCode)
			}
			res.Body.Close()
		}
	})
}

// Rute pengubah state di belakang cookie wajib membawa token CSRF; rute baca tidak.
func TestHandlersCSRFEnforcement(t *testing.T) {
	e := newEnv(t)
	e.makeUser(t, "admin@example.test", seed.RoleAdmin)
	c := e.newClient(t)

	body := decode[PrincipalResponse](t, c.login("admin@example.test", testPassword))

	// Tanpa header token: c.csrf masih kosong, jadi header tidak dipasang.
	res := c.do(http.MethodPost, "/logout", nil)
	if res.StatusCode != http.StatusForbidden {
		t.Errorf("logout tanpa token CSRF = %d, mau 403", res.StatusCode)
	}
	if !strings.Contains(bodyOf(t, res), CodeCSRFInvalid) {
		t.Error("kode error tidak menyebut CSRF")
	}

	// GET tetap boleh tanpa token.
	if res := c.do(http.MethodGet, "/me", nil); res.StatusCode != http.StatusOK {
		t.Errorf("GET /me tanpa token CSRF = %d, mau 200", res.StatusCode)
	}

	// Origin asing ditolak walau token benar.
	c.csrf = body.CSRFToken
	res = c.do(http.MethodPost, "/logout", nil, func(r *http.Request) {
		r.Header.Set("Origin", "https://evil.example.test")
	})
	if res.StatusCode != http.StatusForbidden {
		t.Errorf("logout dengan Origin asing = %d, mau 403", res.StatusCode)
	}
	res.Body.Close()

	// Token dari sesi lain ditolak.
	other := e.login(t, "admin@example.test")
	res = c.do(http.MethodPost, "/logout", nil, func(r *http.Request) {
		r.Header.Set(HeaderCSRFToken, e.svc.CSRF().Issue(other.Principal.SessionID))
	})
	if res.StatusCode != http.StatusForbidden {
		t.Errorf("logout dengan token sesi lain = %d, mau 403", res.StatusCode)
	}
	res.Body.Close()

	// Token yang benar lolos.
	if res := c.do(http.MethodPost, "/logout", nil); res.StatusCode != http.StatusOK {
		t.Errorf("logout dengan token yang benar = %d: %s", res.StatusCode, bodyOf(t, res))
	}
}

func TestHandlersChangePasswordResponses(t *testing.T) {
	e := newEnv(t)
	e.makeUser(t, "ganti@example.test", seed.RoleViewer)
	c := e.newClient(t)
	c.csrf = decode[PrincipalResponse](t, c.login("ganti@example.test", testPassword)).CSRFToken

	cases := []struct {
		name     string
		body     map[string]string
		wantCode int
		wantErr  string
	}{
		{
			name:     "password lama salah",
			body:     map[string]string{"current_password": "Bukan-Punya-Saya-2026", "new_password": testNewPassword},
			wantCode: http.StatusForbidden, wantErr: CodeCurrentPasswordInvalid,
		},
		{
			name:     "password baru lemah",
			body:     map[string]string{"current_password": testPassword, "new_password": "pendek"},
			wantCode: http.StatusBadRequest, wantErr: CodeWeakPassword,
		},
		{
			name:     "password baru sama",
			body:     map[string]string{"current_password": testPassword, "new_password": testPassword},
			wantCode: http.StatusBadRequest, wantErr: CodePasswordUnchanged,
		},
		{
			name:     "field kosong",
			body:     map[string]string{"current_password": testPassword},
			wantCode: http.StatusBadRequest, wantErr: "invalid_request",
		},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			res := c.do(http.MethodPost, "/change-password", tc.body)
			if res.StatusCode != tc.wantCode {
				t.Errorf("status = %d, mau %d", res.StatusCode, tc.wantCode)
			}
			body := bodyOf(t, res)
			if !strings.Contains(body, tc.wantErr) {
				t.Errorf("body = %s, mau memuat %q", body, tc.wantErr)
			}
		})
	}

	// Password lama yang salah TIDAK boleh dibalas 401: dashboard akan menafsirkannya
	// sebagai sesi kedaluwarsa dan mengeluarkan pengguna dari form yang sedang diisinya.
	res := c.do(http.MethodPost, "/change-password", map[string]string{
		"current_password": "Bukan-Punya-Saya-2026", "new_password": testNewPassword,
	})
	if res.StatusCode == http.StatusUnauthorized {
		t.Error("password lama salah dibalas 401; seharusnya 403 agar sesi tidak dianggap habis")
	}
	res.Body.Close()

	// Pesan kekuatan password menyebut aturan yang dilanggar — di sini itu aman dan
	// diperlukan, karena pemanggilnya sudah terautentikasi.
	res = c.do(http.MethodPost, "/change-password", map[string]string{
		"current_password": testPassword, "new_password": "Pendek1",
	})
	env := decode[errorEnvelope](t, res)
	if !strings.Contains(env.Error.Message, "minimal") {
		t.Errorf("pesan = %q, mau menyebut aturan panjang minimum", env.Error.Message)
	}
}

// Batas laju dibalas 429 dengan Retry-After, bukan 401: dashboard harus menyuruh menunggu,
// bukan menyuruh mencoba password lain.
func TestHandlersRateLimitResponse(t *testing.T) {
	limiter, _ := newLimiter(t, WithLoginRateLimits(1, 0))
	e := newEnv(t, WithLoginLimiter(limiter))
	e.makeUser(t, "ada@example.test", seed.RoleViewer)
	c := e.newClient(t)

	if res := c.login("ada@example.test", "Salah-2026-Sekali"); res.StatusCode != http.StatusUnauthorized {
		t.Fatalf("percobaan pertama = %d", res.StatusCode)
	}

	res := c.login("ada@example.test", testPassword)
	if res.StatusCode != http.StatusTooManyRequests {
		t.Fatalf("status = %d, mau 429: %s", res.StatusCode, bodyOf(t, res))
	}
	if res.Header.Get("Retry-After") == "" {
		t.Error("header Retry-After tidak dipasang")
	}
	env := decode[errorEnvelope](t, res)
	if env.Error.Message != MessageLoginRateLimited {
		t.Errorf("pesan = %q, mau %q", env.Error.Message, MessageLoginRateLimited)
	}
}

// Tidak ada password, hash password, maupun token sesi yang boleh muncul di log, di pesan
// error, atau di body respons — pada satu pun jalur.
func TestNoSecretsLeak(t *testing.T) {
	e := newEnv(t)
	user := e.makeUser(t, "rahasia@example.test", seed.RoleSuperAdmin)
	storedHash := e.passwordHash(t, user.ID)

	c := e.newClient(t)
	var bodies strings.Builder
	record := func(res *http.Response) {
		bodies.WriteString(bodyOf(t, res))
		bodies.WriteString("\n")
	}

	// Jalur berhasil.
	loginRes := c.login("rahasia@example.test", testPassword)
	loginBody := bodyOf(t, loginRes)
	bodies.WriteString(loginBody)
	var principal PrincipalResponse
	if err := json.Unmarshal([]byte(loginBody), &principal); err != nil {
		t.Fatalf("membaca balasan login: %v", err)
	}
	c.csrf = principal.CSRFToken

	// Token sesi mentah hanya boleh ada di cookie, tidak di body.
	jarURL, _ := url.Parse(c.base)
	var rawToken string
	for _, ck := range c.http.Jar.Cookies(jarURL) {
		if ck.Name == SessionCookieName {
			rawToken = ck.Value
		}
	}
	if rawToken == "" {
		t.Fatal("cookie sesi tidak ditemukan di penyimpan cookie")
	}
	if strings.Contains(loginBody, rawToken) {
		t.Error("body balasan login memuat token sesi mentah")
	}

	// Jalur gagal dan jalur pengubah state.
	record(c.do(http.MethodGet, "/me", nil))
	record(c.login("rahasia@example.test", "Password-Yang-Salah-2026"))
	record(c.login("hantu@example.test", testPassword))
	record(c.do(http.MethodPost, "/change-password", map[string]string{
		"current_password": "Password-Yang-Salah-2026", "new_password": testNewPassword,
	}))
	record(c.do(http.MethodPost, "/change-password", map[string]string{
		"current_password": testPassword, "new_password": "pendek",
	}))
	record(c.do(http.MethodPost, "/change-password", map[string]string{
		"current_password": testPassword, "new_password": testNewPassword,
	}))
	record(c.do(http.MethodPost, "/logout", nil))

	// Pesan error dari pemanggilan langsung layanan.
	var errorTexts strings.Builder
	collect := func(err error) {
		if err != nil {
			errorTexts.WriteString(err.Error())
			errorTexts.WriteString("\n")
		}
	}
	_, err := e.svc.Login(e.ctx, "rahasia@example.test", security.Secret("Password-Yang-Salah-2026"), "", "")
	collect(err)
	_, err = e.svc.Login(e.ctx, "hantu@example.test", security.Secret(testPassword), "", "")
	collect(err)
	_, err = e.svc.ChangePassword(e.ctx, user.ID, security.Secret("Password-Yang-Salah-2026"),
		security.Secret(testNewPassword), "", "", "")
	collect(err)
	_, err = e.svc.ChangePassword(e.ctx, user.ID, security.Secret(testNewPassword),
		security.Secret("pendek"), "", "", "")
	collect(err)
	_, err = e.svc.Authenticate(e.ctx, security.Secret(rawToken))
	collect(err)

	secrets := map[string]string{
		"password uji":   testPassword,
		"password baru":  testNewPassword,
		"password salah": "Password-Yang-Salah-2026",
		"hash password":  storedHash,
		"potongan hash":  storedHash[len(storedHash)-24:],
		"token sesi":     rawToken,
		"potongan token": rawToken[:20],
	}
	haystacks := map[string]string{
		"log":         e.logs.String(),
		"body":        bodies.String(),
		"pesan error": errorTexts.String(),
	}

	for place, haystack := range haystacks {
		if strings.TrimSpace(haystack) == "" {
			t.Errorf("%s kosong — test kebocoran tidak memeriksa apa pun", place)
		}
		for name, secret := range secrets {
			if strings.Contains(haystack, secret) {
				t.Errorf("%s memuat %s", place, name)
			}
		}
	}

	// Hash yang tersimpan juga tidak boleh ikut terserialisasi ke JSON pengguna, walaupun
	// seluruh struct identity.User dikirim ke klien.
	if strings.Contains(bodies.String(), "password_hash") {
		t.Error("body respons memuat field password_hash")
	}
}

// Jalur bertahan: handler dipanggil langsung tanpa RequireSession di depannya. Lewat
// Routes() ini tidak mungkin terjadi, tetapi handler-nya diekspor dan bisa dipasang di
// router lain — jadi ia harus gagal tertutup alih-alih menyentuh principal nil.
func TestHandlersWithoutPrincipal(t *testing.T) {
	e := newEnv(t)
	h := NewHandlers(e.svc)

	cases := map[string]http.HandlerFunc{
		"/me":              h.Me,
		"/logout":          h.Logout,
		"/change-password": h.ChangePassword,
	}
	for path, handler := range cases {
		t.Run(path, func(t *testing.T) {
			r := httptest.NewRequest(http.MethodPost, path, strings.NewReader(`{}`))
			r.Header.Set("Content-Type", "application/json")
			rec := httptest.NewRecorder()
			handler(rec, r)

			if rec.Code != http.StatusUnauthorized {
				t.Errorf("status = %d, mau 401", rec.Code)
			}
			if !strings.Contains(rec.Body.String(), CodeSessionRequired) {
				t.Errorf("body = %s, mau memuat %q", rec.Body.String(), CodeSessionRequired)
			}
		})
	}
}

// IP yang dicatat di sesi dan audit harus berasal dari resolusi httpx.RealIP, bukan dari
// header apa adanya: IP dipakai untuk pembatasan laju dan penelusuran insiden, jadi ia hanya
// boleh dipercaya bila datang dari proxy yang memang dipercaya.
func TestHandlersRecordsResolvedClientIP(t *testing.T) {
	e := newEnv(t)
	e.makeUser(t, "ip@example.test", seed.RoleViewer)

	// RealIP dipasang di depan router, seperti di produksi. Peer koneksi test adalah
	// loopback, yang termasuk daftar proxy privat, jadi X-Forwarded-For dipercaya.
	routes := NewHandlers(e.svc).Routes()
	srv := httptest.NewServer(httpx.RealIP(httpx.PrivateProxyPrefixes())(routes))
	t.Cleanup(srv.Close)

	body, err := json.Marshal(map[string]string{"email": "ip@example.test", "password": testPassword})
	if err != nil {
		t.Fatalf("menyusun body: %v", err)
	}
	req, err := http.NewRequest(http.MethodPost, srv.URL+"/login", bytes.NewReader(body))
	if err != nil {
		t.Fatalf("menyusun request: %v", err)
	}
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("X-Forwarded-For", "203.0.113.77")

	res, err := http.DefaultClient.Do(req)
	if err != nil {
		t.Fatalf("POST /login: %v", err)
	}
	if res.StatusCode != http.StatusOK {
		t.Fatalf("POST /login = %d: %s", res.StatusCode, bodyOf(t, res))
	}
	res.Body.Close()

	var sessionIP, auditIP string
	if err := e.pool.QueryRow(e.ctx, `select coalesce(host(ip), '') from sessions limit 1`).Scan(&sessionIP); err != nil {
		t.Fatalf("membaca ip sesi: %v", err)
	}
	if err := e.pool.QueryRow(e.ctx, `select coalesce(host(ip), '') from audit_logs
		where action = 'auth.login' limit 1`).Scan(&auditIP); err != nil {
		t.Fatalf("membaca ip audit: %v", err)
	}
	if sessionIP != "203.0.113.77" || auditIP != "203.0.113.77" {
		t.Errorf("ip sesi = %q, ip audit = %q; mau 203.0.113.77", sessionIP, auditIP)
	}
}

// Kegagalan infrastruktur dibalas 500 dengan pesan generik: nama tabel, teks driver, dan
// alamat internal tidak boleh pernah sampai ke klien.
func TestHandlersInternalErrorIsGeneric(t *testing.T) {
	e := newEnv(t)
	e.makeUser(t, "ada@example.test", seed.RoleViewer)
	c := e.newClient(t)

	// Pool ditutup untuk memaksa kegagalan di lapisan data. Pembersihan schema memakai
	// koneksi admin yang berbeda, jadi ia tidak terpengaruh.
	e.pool.Close()

	res := c.login("ada@example.test", testPassword)
	if res.StatusCode != http.StatusInternalServerError {
		t.Fatalf("status = %d, mau 500: %s", res.StatusCode, bodyOf(t, res))
	}
	env := decode[errorEnvelope](t, res)
	if env.Error.Type != "api_error" {
		t.Errorf("tipe = %q, mau api_error", env.Error.Type)
	}
	for _, forbidden := range []string{"users", "select", "pgx", "closed pool", "password_hash"} {
		if strings.Contains(strings.ToLower(env.Error.Message), forbidden) {
			t.Errorf("pesan 500 memuat detail internal %q: %q", forbidden, env.Error.Message)
		}
	}
	if env.Error.RequestID != testRequestID {
		t.Errorf("request_id = %q, mau %q", env.Error.RequestID, testRequestID)
	}
}

func TestSetupHintEndpoint(t *testing.T) {
	e := newEnv(t)
	c := e.newClient(t)

	// Database baru yang telah di-seed memiliki admin default dengan MustChangePassword = true
	res := c.do(http.MethodGet, "/setup-hint", nil)
	if res.StatusCode != http.StatusOK {
		t.Fatalf("status = %d, mau 200", res.StatusCode)
	}
	var hint SetupHintResponse
	if err := json.NewDecoder(res.Body).Decode(&hint); err != nil {
		t.Fatalf("decode JSON: %v", err)
	}
	if !hint.HasDefaultAdmin {
		t.Error("HasDefaultAdmin = false padahal admin default aktif dan must_change_password")
	}
	if hint.DefaultEmail != "admin@routex.local" {
		t.Errorf("DefaultEmail = %q", hint.DefaultEmail)
	}
	if hint.DefaultPassword != "RouteX#Initial2026!" {
		t.Errorf("DefaultPassword = %q", hint.DefaultPassword)
	}

	u, err := e.users.GetByEmail(e.ctx, "admin@routex.local")
	if err != nil {
		t.Fatalf("GetByEmail admin: %v", err)
	}

	// Ganti password
	if err := e.users.ChangePassword(e.ctx, u.ID, security.Secret("sandi-baru-yang-kuat-1234"), false); err != nil {
		t.Fatalf("ChangePassword: %v", err)
	}

	res = c.do(http.MethodGet, "/setup-hint", nil)
	if res.StatusCode != http.StatusOK {
		t.Fatalf("status = %d, mau 200", res.StatusCode)
	}
	if err := json.NewDecoder(res.Body).Decode(&hint); err != nil {
		t.Fatalf("decode JSON: %v", err)
	}
	if hint.HasDefaultAdmin {
		t.Error("HasDefaultAdmin = true padahal password sudah diubah")
	}
}
