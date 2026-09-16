package auth

import (
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/NexGen-X/Route-X/internal/config"
	"github.com/NexGen-X/Route-X/internal/database/repo/identity"
)

// csrfHarness adalah CSRF beserta handler terlindungi yang siap diuji tanpa database.
type csrfHarness struct {
	csrf    *CSRF
	cookies *Cookies
	handler http.Handler
	// reached menandai handler di belakang middleware benar-benar terpanggil.
	reached bool
}

func newCSRFHarness(t *testing.T, publicURL string) *csrfHarness {
	t.Helper()
	cfg := &config.Config{
		AppEnv:        config.EnvDevelopment,
		PublicURL:     publicURL,
		SessionSecret: testSessionSecret,
	}
	cookies := NewCookies(cfg, time.Hour)
	h := &csrfHarness{csrf: NewCSRF(cfg, cookies, nil), cookies: cookies}
	h.handler = h.csrf.Protect()(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		h.reached = true
		w.WriteHeader(http.StatusNoContent)
	}))
	return h
}

// csrfRequest merakit permintaan yang sudah melewati RequireSession.
type csrfRequest struct {
	method    string
	sessionID string
	origin    string
	referer   string
	header    string
	cookie    string
	// noPrincipal mensimulasikan pemasangan middleware tanpa RequireSession di depannya.
	noPrincipal bool
}

func (h *csrfHarness) do(t *testing.T, in csrfRequest) *httptest.ResponseRecorder {
	t.Helper()
	h.reached = false

	method := in.method
	if method == "" {
		method = http.MethodPost
	}
	r := httptest.NewRequest(method, "http://dashboard.example.test/aksi", nil)
	if in.origin != "" {
		r.Header.Set("Origin", in.origin)
	}
	if in.referer != "" {
		r.Header.Set("Referer", in.referer)
	}
	if in.header != "" {
		r.Header.Set(HeaderCSRFToken, in.header)
	}
	if in.cookie != "" {
		r.AddCookie(&http.Cookie{Name: h.cookies.CSRFName(), Value: in.cookie})
	}
	if !in.noPrincipal {
		sessionID := in.sessionID
		if sessionID == "" {
			sessionID = "3f3e0f6a-0000-4000-8000-000000000001"
		}
		principal := &Principal{
			User:      identity.User{ID: "11111111-1111-4111-8111-111111111111"},
			SessionID: sessionID,
		}
		r = r.WithContext(WithPrincipal(r.Context(), principal))
	}

	rec := httptest.NewRecorder()
	h.handler.ServeHTTP(rec, r)
	return rec
}

func TestCSRFAcceptsMatchingToken(t *testing.T) {
	h := newCSRFHarness(t, "http://dashboard.example.test")
	const sessionID = "3f3e0f6a-0000-4000-8000-0000000000aa"
	token := h.csrf.Issue(sessionID)

	rec := h.do(t, csrfRequest{
		sessionID: sessionID,
		origin:    "http://dashboard.example.test",
		header:    token,
		cookie:    token,
	})
	if rec.Code != http.StatusNoContent || !h.reached {
		t.Fatalf("status = %d, handler tercapai = %v, body = %s", rec.Code, h.reached, rec.Body)
	}
}

func TestCSRFRejections(t *testing.T) {
	const sessionID = "3f3e0f6a-0000-4000-8000-0000000000aa"
	const otherSessionID = "3f3e0f6a-0000-4000-8000-0000000000bb"

	h := newCSRFHarness(t, "http://dashboard.example.test")
	token := h.csrf.Issue(sessionID)
	foreignToken := h.csrf.Issue(otherSessionID)

	cases := map[string]csrfRequest{
		"tanpa token sama sekali": {
			sessionID: sessionID, origin: "http://dashboard.example.test",
		},
		"hanya header, tanpa cookie": {
			sessionID: sessionID, origin: "http://dashboard.example.test", header: token,
		},
		"hanya cookie, tanpa header": {
			sessionID: sessionID, origin: "http://dashboard.example.test", cookie: token,
		},
		"cookie dan header berbeda": {
			sessionID: sessionID, origin: "http://dashboard.example.test",
			header: token, cookie: foreignToken,
		},
		"token milik sesi lain": {
			sessionID: sessionID, origin: "http://dashboard.example.test",
			header: foreignToken, cookie: foreignToken,
		},
		"token karangan": {
			sessionID: sessionID, origin: "http://dashboard.example.test",
			header: "token-karangan", cookie: "token-karangan",
		},
		"origin asing": {
			sessionID: sessionID, origin: "https://evil.example.test",
			header: token, cookie: token,
		},
		"referer asing": {
			sessionID: sessionID, referer: "https://evil.example.test/halaman",
			header: token, cookie: token,
		},
		"tanpa origin maupun referer": {
			sessionID: sessionID, header: token, cookie: token,
		},
		"origin null": {
			sessionID: sessionID, origin: "null", header: token, cookie: token,
		},
		"tanpa principal": {
			noPrincipal: true, origin: "http://dashboard.example.test",
			header: token, cookie: token,
		},
	}

	for name, req := range cases {
		t.Run(name, func(t *testing.T) {
			rec := h.do(t, req)
			if rec.Code != http.StatusForbidden {
				t.Errorf("status = %d, mau 403", rec.Code)
			}
			if h.reached {
				t.Error("handler di belakang middleware tercapai padahal permintaan harus ditolak")
			}
			body := rec.Body.String()
			if !strings.Contains(body, CodeCSRFInvalid) {
				t.Errorf("body = %s, mau memuat kode %q", body, CodeCSRFInvalid)
			}
			// Alasan penolakan tidak boleh sampai ke klien: bagi penyerang ia menunjukkan
			// bagian mana yang sudah berhasil dilewati.
			if !strings.Contains(body, MessageCSRFInvalid) {
				t.Errorf("body = %s, mau memuat pesan seragam %q", body, MessageCSRFInvalid)
			}
		})
	}
}

// Metode yang tidak mengubah state tidak diperiksa: tanpa itu setiap pemuatan halaman
// dashboard akan menuntut token yang justru baru diambil lewat pemuatan itu.
func TestCSRFSkipsSafeMethods(t *testing.T) {
	h := newCSRFHarness(t, "http://dashboard.example.test")

	for _, method := range []string{http.MethodGet, http.MethodHead, http.MethodOptions, http.MethodTrace} {
		t.Run(method, func(t *testing.T) {
			rec := h.do(t, csrfRequest{method: method})
			if rec.Code != http.StatusNoContent || !h.reached {
				t.Errorf("status = %d, handler tercapai = %v", rec.Code, h.reached)
			}
		})
	}

	// Metode pengubah state — termasuk yang jarang dipakai — tetap diperiksa. Daftarnya
	// positif, jadi metode yang tidak dikenal jatuh ke sisi yang diperiksa.
	for _, method := range []string{http.MethodPost, http.MethodPut, http.MethodPatch, http.MethodDelete, "PURGE"} {
		t.Run(method, func(t *testing.T) {
			rec := h.do(t, csrfRequest{method: method})
			if rec.Code != http.StatusForbidden {
				t.Errorf("status = %d, mau 403", rec.Code)
			}
		})
	}
}

// Tanpa PUBLIC_URL, origin dibandingkan dengan host request itu sendiri.
func TestCSRFOriginFallsBackToRequestHost(t *testing.T) {
	h := newCSRFHarness(t, "")
	const sessionID = "3f3e0f6a-0000-4000-8000-0000000000cc"
	token := h.csrf.Issue(sessionID)

	// Host request pada httptest.NewRequest di atas adalah dashboard.example.test.
	rec := h.do(t, csrfRequest{
		sessionID: sessionID, origin: "http://dashboard.example.test",
		header: token, cookie: token,
	})
	if rec.Code != http.StatusNoContent {
		t.Errorf("origin sama dengan host request ditolak: status %d", rec.Code)
	}

	// Skema berbeda tetap lolos ketika PUBLIC_URL kosong: hanya host yang dibandingkan,
	// karena menebak skema request menuntut mempercayai X-Forwarded-Proto.
	rec = h.do(t, csrfRequest{
		sessionID: sessionID, origin: "https://dashboard.example.test",
		header: token, cookie: token,
	})
	if rec.Code != http.StatusNoContent {
		t.Errorf("origin dengan skema berbeda ditolak: status %d", rec.Code)
	}

	rec = h.do(t, csrfRequest{
		sessionID: sessionID, origin: "http://lain.example.test",
		header: token, cookie: token,
	})
	if rec.Code != http.StatusForbidden {
		t.Errorf("origin host lain diterima: status %d", rec.Code)
	}
}

// Referer dipakai sebagai cadangan bila Origin tidak ada.
func TestCSRFRefererFallback(t *testing.T) {
	h := newCSRFHarness(t, "http://dashboard.example.test")
	const sessionID = "3f3e0f6a-0000-4000-8000-0000000000dd"
	token := h.csrf.Issue(sessionID)

	rec := h.do(t, csrfRequest{
		sessionID: sessionID, referer: "http://dashboard.example.test/pengaturan?tab=umum",
		header: token, cookie: token,
	})
	if rec.Code != http.StatusNoContent {
		t.Errorf("Referer dari origin sendiri ditolak: status %d, body %s", rec.Code, rec.Body)
	}
}

// Token harus terikat sesi, bisa dihitung ulang, dan tidak bisa dikarang tanpa kunci.
func TestCSRFTokenBinding(t *testing.T) {
	h := newCSRFHarness(t, "")
	const a = "3f3e0f6a-0000-4000-8000-00000000000a"
	const b = "3f3e0f6a-0000-4000-8000-00000000000b"

	tokenA, tokenB := h.csrf.Issue(a), h.csrf.Issue(b)
	if tokenA == "" || tokenB == "" {
		t.Fatal("Issue mengembalikan token kosong")
	}
	if tokenA == tokenB {
		t.Error("dua sesi berbeda menghasilkan token yang sama")
	}
	if h.csrf.Issue(a) != tokenA {
		t.Error("Issue tidak deterministik untuk sesi yang sama")
	}
	if !h.csrf.Verify(a, tokenA) {
		t.Error("token sendiri tidak terverifikasi")
	}
	if h.csrf.Verify(b, tokenA) {
		t.Error("token sesi lain terverifikasi")
	}
	if h.csrf.Issue("") != "" {
		t.Error("sesi kosong seharusnya tidak menghasilkan token")
	}
	for _, token := range []string{"", "x", tokenA + "x", tokenA[:len(tokenA)-1]} {
		if h.csrf.Verify(a, token) {
			t.Errorf("token %q terverifikasi", token)
		}
	}

	// Kunci yang berbeda harus menghasilkan token yang berbeda: tanpa itu, token dari satu
	// deployment akan berlaku di deployment lain.
	other := NewCSRF(&config.Config{SessionSecret: "kunci-lain-yang-panjangnya-lebih-dari-32-karakter"},
		h.cookies, nil)
	if other.Issue(a) == tokenA {
		t.Error("kunci berbeda menghasilkan token yang sama")
	}
	if other.Verify(a, tokenA) {
		t.Error("token dari kunci lain terverifikasi")
	}
}

// Perbandingan token harus waktu-konstan: perbandingan biasa berhenti pada byte pertama
// yang berbeda, sehingga lamanya jawaban menunjukkan berapa byte awal yang sudah benar dan
// token yang sah bisa disusun sebyte demi sebyte alih-alih ditebak seluruhnya.
//
// Pengukurannya memakai nilai MINIMUM dari beberapa gelombang, karena gangguan hanya bisa
// menambah waktu. Sebelum menyimpulkan apa pun, sebuah pembanding kontrol yang MEMANG peka
// posisi diukur dengan cara yang sama: kalau instrumen ini tidak bisa membedakan keduanya
// pada mesin yang sedang dipakai, test dilewati alih-alih melaporkan hasil yang tidak
// bermakna.
func TestConstantTimeComparison(t *testing.T) {
	const size = 1 << 18
	base := strings.Repeat("a", size)
	early := "b" + base[1:]
	late := base[:size-1] + "b"

	// Kebenaran lebih dulu: perbedaan di posisi mana pun harus tertangkap.
	if !constantTimeEqual(base, base) {
		t.Fatal("nilai identik dilaporkan berbeda")
	}
	for name, other := range map[string]string{"byte pertama": early, "byte terakhir": late} {
		if constantTimeEqual(base, other) {
			t.Fatalf("perbedaan di %s tidak tertangkap", name)
		}
	}

	measure := func(equal func(string, string) bool, other string) time.Duration {
		const (
			batches = 5
			rounds  = 20
		)
		best := time.Hour
		for b := 0; b < batches; b++ {
			start := time.Now()
			for i := 0; i < rounds; i++ {
				if equal(base, other) {
					t.Fatal("pembanding melaporkan dua nilai berbeda sebagai sama")
				}
			}
			if elapsed := time.Since(start); elapsed < best {
				best = elapsed
			}
		}
		return best
	}

	// Kontrol: perbandingan string biasa berhenti di byte pertama yang berbeda.
	naive := func(a, b string) bool { return a == b }
	controlEarly := measure(naive, early)
	controlLate := measure(naive, late)
	sensitivity := float64(controlLate) / float64(controlEarly)
	t.Logf("kontrol: byte pertama %v, byte terakhir %v (rasio %.2f)", controlEarly, controlLate, sensitivity)
	if sensitivity < 2.0 {
		t.Skipf("pengukuran waktu di mesin ini tidak cukup peka (rasio kontrol %.2f)", sensitivity)
	}

	gotEarly := measure(constantTimeEqual, early)
	gotLate := measure(constantTimeEqual, late)
	ratio := float64(gotLate) / float64(gotEarly)
	if ratio < 1 {
		ratio = 1 / ratio
	}
	t.Logf("constantTimeEqual: byte pertama %v, byte terakhir %v (rasio %.2f)", gotEarly, gotLate, ratio)

	// Pengukuran wall-clock pada host bersama tetap bising meski pembandingnya
	// crypto/subtle. Kontrol di atas memastikan instrumen mampu melihat early-exit;
	// toleransi ini hanya mencegah gate palsu akibat scheduling/cache host.
	const tolerance = 2.0
	if ratio > tolerance {
		t.Errorf("lamanya perbandingan bergantung pada posisi perbedaan (rasio %.2f > %.1f)", ratio, tolerance)
	}
}

// Tanpa SESSION_SECRET, kunci dibuat acak per proses. Itu tetap aman secara kriptografis,
// jadi yang diuji di sini adalah tidak ada jalur yang panik dan token tetap terikat sesi.
func TestCSRFWithoutSessionSecret(t *testing.T) {
	cookies := NewCookies(nil, time.Hour)
	c := NewCSRF(&config.Config{}, cookies, nil)

	const sessionID = "3f3e0f6a-0000-4000-8000-0000000000ee"
	token := c.Issue(sessionID)
	if token == "" {
		t.Fatal("Issue mengembalikan token kosong")
	}
	if !c.Verify(sessionID, token) {
		t.Error("token sendiri tidak terverifikasi")
	}
	// cfg nil juga harus aman.
	if NewCSRF(nil, cookies, nil).Issue(sessionID) == "" {
		t.Error("cfg nil menghasilkan token kosong")
	}
}

func TestOriginOf(t *testing.T) {
	cases := map[string]string{
		"https://admin.example.test":            "https://admin.example.test",
		"https://admin.example.test/":           "https://admin.example.test",
		"https://admin.example.test:8443/masuk": "https://admin.example.test:8443",
		"  http://localhost:8080  ":             "http://localhost:8080",
		"":                                      "",
		"bukan-url":                             "",
		"/hanya/path":                           "",
		"https://":                              "",
		"::bukan-url-sama-sekali":               "",
	}
	for in, want := range cases {
		if got := originOf(in); got != want {
			t.Errorf("originOf(%q) = %q, mau %q", in, got, want)
		}
	}
}

func TestCSRFOriginMatchesRequestHostEvenWhenPublicURLSet(t *testing.T) {
	h := newCSRFHarness(t, "http://127.0.0.1:8080")
	const sessionID = "3f3e0f6a-0000-4000-8000-0000000000aa"
	token := h.csrf.Issue(sessionID)

	r := httptest.NewRequest(http.MethodPost, "http://54.179.116.100/api/auth/change-password", nil)
	r.Header.Set("Origin", "http://54.179.116.100")
	r.Header.Set(HeaderCSRFToken, token)
	r.AddCookie(&http.Cookie{Name: h.cookies.CSRFName(), Value: token})
	principal := &Principal{
		User:      identity.User{ID: "11111111-1111-4111-8111-111111111111"},
		SessionID: sessionID,
	}
	r = r.WithContext(WithPrincipal(r.Context(), principal))
	rec := httptest.NewRecorder()
	h.handler.ServeHTTP(rec, r)

	if rec.Code != http.StatusNoContent {
		t.Fatalf("status = %d, mau %d, body = %s", rec.Code, http.StatusNoContent, rec.Body)
	}
}
