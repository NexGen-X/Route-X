package httpx

import (
	"encoding/json"
	"errors"
	"io"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"net/netip"
	"strconv"
	"strings"
	"testing"

	"github.com/NexGen-X/Route-X/internal/config"
	"github.com/NexGen-X/Route-X/internal/observability"
)

// --- RequestID --------------------------------------------------------------

func TestRequestIDMembuatIDBaru(t *testing.T) {
	var fromContext string
	handler := RequestID()(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		fromContext = observability.RequestIDFrom(r.Context())
	}))

	rec := httptest.NewRecorder()
	handler.ServeHTTP(rec, httptest.NewRequest(http.MethodGet, "/v1/models", nil))

	if fromContext == "" {
		t.Fatal("request_id tidak tersedia di context")
	}
	if got := rec.Header().Get(HeaderRequestID); got != fromContext {
		t.Fatalf("header respons %q tidak sama dengan ID di context %q", got, fromContext)
	}
	if len(fromContext) != 36 {
		t.Fatalf("ID yang dihasilkan bukan UUID 36 karakter: %q", fromContext)
	}
	if fromContext[14] != '4' {
		t.Fatalf("ID bukan UUID versi 4: %q", fromContext)
	}
	if !strings.ContainsRune("89ab", rune(fromContext[19])) {
		t.Fatalf("varian UUID tidak sesuai RFC 4122: %q", fromContext)
	}

	// ID harus berbeda antar request, kalau tidak korelasi log tidak ada gunanya.
	pertama := fromContext
	rec2 := httptest.NewRecorder()
	handler.ServeHTTP(rec2, httptest.NewRequest(http.MethodGet, "/v1/models", nil))
	if rec2.Header().Get(HeaderRequestID) == pertama {
		t.Fatal("dua request berbeda mendapat request_id yang sama")
	}
}

func TestRequestIDMenghormatiHeaderKlienYangSah(t *testing.T) {
	valid := []string{
		"018f3a9c-1d2e-4f56-8a9b-0c1d2e3f4a5b",
		"01HX7Q9J8K5N2P4R6T8V0W1Y3Z",
		"00f067aa0ba902b7",
		"trace:abc-123_v.2",
	}
	for _, want := range valid {
		t.Run(want, func(t *testing.T) {
			var got string
			handler := RequestID()(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				got = observability.RequestIDFrom(r.Context())
			}))
			req := httptest.NewRequest(http.MethodGet, "/v1/models", nil)
			req.Header.Set(HeaderRequestID, want)

			rec := httptest.NewRecorder()
			handler.ServeHTTP(rec, req)

			if got != want {
				t.Fatalf("ID klien tidak dipakai: got %q, want %q", got, want)
			}
			if h := rec.Header().Get(HeaderRequestID); h != want {
				t.Fatalf("header respons %q, want %q", h, want)
			}
		})
	}
}

// TestRequestIDMenolakNilaiBerbahaya adalah uji anti header injection: nilai dari
// klien dipantulkan ke header respons dan masuk ke log, jadi tidak boleh ada
// karakter yang bisa memecah header atau baris log.
func TestRequestIDMenolakNilaiBerbahaya(t *testing.T) {
	tests := map[string]string{
		"terlalu panjang":     strings.Repeat("a", maxRequestIDLen+1),
		"crlf":                "abc\r\nX-Injected: jahat",
		"lf saja":             "abc\ndef",
		"cr saja":             "abc\rdef",
		"tab":                 "abc\tdef",
		"nul":                 "abc\x00def",
		"spasi":               "abc def",
		"karakter kontrol":    "abc\x1b[31mdef",
		"tanda kutip":         `abc"def`,
		"pemisah header":      "abc; def=ghi",
		"non-ascii":           "abc-é-def",
		"kurung sudut":        "<script>alert(1)</script>",
		"kosong":              "",
		"hanya spasi":         "   ",
		"percobaan traversal": "../../etc/passwd",
	}

	for name, jahat := range tests {
		t.Run(name, func(t *testing.T) {
			var got string
			handler := RequestID()(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				got = observability.RequestIDFrom(r.Context())
			}))
			req := httptest.NewRequest(http.MethodGet, "/v1/models", nil)
			// Set lewat map langsung: Header.Set tidak menolak nilai aneh, dan justru
			// nilai seperti inilah yang harus disaring middleware.
			req.Header[HeaderRequestID] = []string{jahat}

			rec := httptest.NewRecorder()
			handler.ServeHTTP(rec, req)

			if got == jahat {
				t.Fatalf("nilai berbahaya dipakai apa adanya: %q", jahat)
			}
			if len(got) != 36 {
				t.Fatalf("nilai berbahaya tidak diganti UUID baru, dapat %q", got)
			}
			echoed := rec.Header().Get(HeaderRequestID)
			if echoed != got {
				t.Fatalf("header respons %q tidak sama dengan ID pengganti %q", echoed, got)
			}
			if strings.ContainsAny(echoed, "\r\n\x00\t ") {
				t.Fatalf("header respons masih memuat karakter berbahaya: %q", echoed)
			}
			if strings.Contains(echoed, "X-Injected") {
				t.Fatalf("header injection lolos: %q", echoed)
			}
		})
	}
}

func TestRequestIDMenautkanLoggerKeContext(t *testing.T) {
	logger, logs := newTestLogger()

	handler := RequestID()(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		observability.LoggerFrom(r.Context()).Info("dari dalam handler")
	}))

	req := httptest.NewRequest(http.MethodGet, "/v1/models", nil)
	req = req.WithContext(observability.WithLogger(req.Context(), logger))

	rec := httptest.NewRecorder()
	handler.ServeHTTP(rec, req)

	id := rec.Header().Get(HeaderRequestID)
	out := logs.String()
	if !strings.Contains(out, `"request_id":"`+id+`"`) {
		t.Fatalf("logger context tidak membawa request_id %q, log: %s", id, out)
	}
}

// --- Recover ----------------------------------------------------------------

func TestRecoverMembalas500TanpaMembocorkanPanic(t *testing.T) {
	logger, logs := newTestLogger()
	const detailInternal = "pq: relation \"api_keys\" does not exist"

	handler := Recover(logger)(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		panic(detailInternal)
	}))

	rec := httptest.NewRecorder()
	handler.ServeHTTP(rec, httptest.NewRequest(http.MethodPost, "/v1/chat/completions", nil))

	if rec.Code != http.StatusInternalServerError {
		t.Fatalf("status %d, want 500", rec.Code)
	}

	body := rec.Body.String()
	if strings.Contains(body, detailInternal) {
		t.Fatalf("pesan panic bocor ke klien: %s", body)
	}
	if strings.Contains(body, "goroutine") || strings.Contains(body, ".go:") {
		t.Fatalf("stack trace bocor ke klien: %s", body)
	}

	var resp ErrorResponse
	if err := json.Unmarshal(rec.Body.Bytes(), &resp); err != nil {
		t.Fatalf("body bukan envelope JSON yang sah: %v (%s)", err, body)
	}
	if resp.Error.Type != ErrTypeAPI {
		t.Fatalf("type %q, want %q", resp.Error.Type, ErrTypeAPI)
	}
	if resp.Error.Code == nil || *resp.Error.Code != CodeInternalError {
		t.Fatalf("code %v, want %q", resp.Error.Code, CodeInternalError)
	}
	if resp.Error.Message != msgInternalError {
		t.Fatalf("message %q, want pesan generik %q", resp.Error.Message, msgInternalError)
	}

	// Detail yang tidak dikirim ke klien harus tetap tercatat untuk operator.
	out := logs.String()
	if !strings.Contains(out, "api_keys") {
		t.Fatalf("pesan panic tidak tercatat di log: %s", out)
	}
	if !strings.Contains(out, "stack") {
		t.Fatalf("stack trace tidak tercatat di log: %s", out)
	}
}

func TestRecoverTidakMenulisHeaderKeduaSaatStreaming(t *testing.T) {
	logger, logs := newTestLogger()
	const terkirim = "data: {\"delta\":\"halo\"}\n\n"

	handler := Recover(logger)(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "text/event-stream")
		w.WriteHeader(http.StatusOK)
		if _, err := io.WriteString(w, terkirim); err != nil {
			t.Errorf("menulis event pertama: %v", err)
		}
		if err := http.NewResponseController(w).Flush(); err != nil {
			t.Errorf("flush event pertama: %v", err)
		}
		panic("penyedia upstream memutus koneksi")
	}))

	rec := httptest.NewRecorder()
	handler.ServeHTTP(rec, httptest.NewRequest(http.MethodPost, "/v1/chat/completions", nil))

	if rec.Code != http.StatusOK {
		t.Fatalf("status %d, want 200 karena header sudah terkirim sebelum panic", rec.Code)
	}
	if got := rec.Body.String(); got != terkirim {
		t.Fatalf("body %q; envelope error tidak boleh ditempel di belakang stream", got)
	}
	if !strings.Contains(logs.String(), "upstream memutus koneksi") {
		t.Fatalf("panic di tengah stream tidak tercatat: %s", logs.String())
	}
}

func TestRecoverMeneruskanErrAbortHandler(t *testing.T) {
	logger, logs := newTestLogger()
	handler := Recover(logger)(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		panic(http.ErrAbortHandler)
	}))

	defer func() {
		recovered := recover()
		if recovered == nil {
			t.Fatal("http.ErrAbortHandler harus diteruskan ke net/http, bukan ditelan")
		}
		if recovered != http.ErrAbortHandler {
			t.Fatalf("panic yang diteruskan %v, want http.ErrAbortHandler", recovered)
		}
		if strings.Contains(logs.String(), "panic saat menangani request") {
			t.Fatal("ErrAbortHandler tidak boleh dicatat sebagai panic")
		}
	}()

	handler.ServeHTTP(httptest.NewRecorder(), httptest.NewRequest(http.MethodGet, "/", nil))
}

// --- SecurityHeaders --------------------------------------------------------

func TestSecurityHeaders(t *testing.T) {
	wajib := map[string]string{
		"X-Content-Type-Options":     "nosniff",
		"X-Frame-Options":            "DENY",
		"Referrer-Policy":            "strict-origin-when-cross-origin",
		"Cross-Origin-Opener-Policy": "same-origin",
		"Permissions-Policy":         permissionsPolicy,
		"Content-Security-Policy":    cspSPA,
	}

	for _, tc := range []struct {
		env      config.Env
		wantHSTS bool
	}{
		{config.EnvDevelopment, false},
		{config.EnvStaging, false},
		{config.EnvProduction, true},
	} {
		t.Run(string(tc.env), func(t *testing.T) {
			cfg := testConfig(tc.env)
			handler := SecurityHeaders(cfg)(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				w.WriteHeader(http.StatusNoContent)
			}))

			rec := httptest.NewRecorder()
			handler.ServeHTTP(rec, httptest.NewRequest(http.MethodGet, "/", nil))

			for name, want := range wajib {
				if got := rec.Header().Get(name); got != want {
					t.Errorf("header %s = %q, want %q", name, got, want)
				}
			}
			for _, direktif := range []string{"default-src 'self'", "frame-ancestors 'none'", "object-src 'none'"} {
				if !strings.Contains(rec.Header().Get("Content-Security-Policy"), direktif) {
					t.Errorf("CSP tidak memuat %q", direktif)
				}
			}

			hsts := rec.Header().Get("Strict-Transport-Security")
			if tc.wantHSTS && hsts == "" {
				t.Error("HSTS harus dipasang di produksi")
			}
			if !tc.wantHSTS && hsts != "" {
				t.Errorf("HSTS tidak boleh dipasang di %s, dapat %q", tc.env, hsts)
			}
		})
	}
}

func TestSecurityHeadersMenerimaConfigNil(t *testing.T) {
	handler := SecurityHeaders(nil)(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {}))
	rec := httptest.NewRecorder()
	handler.ServeHTTP(rec, httptest.NewRequest(http.MethodGet, "/", nil))

	if rec.Header().Get("X-Content-Type-Options") != "nosniff" {
		t.Error("header dasar hilang saat cfg nil")
	}
	if rec.Header().Get("Strict-Transport-Security") != "" {
		t.Error("cfg nil harus diperlakukan sebagai non-produksi")
	}
}

// --- MaxBytes ---------------------------------------------------------------

func assertPayloadTooLarge(t *testing.T, rec *httptest.ResponseRecorder, limit int64) {
	t.Helper()
	if rec.Code != http.StatusRequestEntityTooLarge {
		t.Fatalf("status %d, want 413; body: %s", rec.Code, rec.Body.String())
	}
	var resp ErrorResponse
	if err := json.Unmarshal(rec.Body.Bytes(), &resp); err != nil {
		t.Fatalf("body bukan envelope JSON: %v (%s)", err, rec.Body.String())
	}
	if resp.Error.Type != ErrTypeInvalidRequest {
		t.Errorf("type %q, want %q", resp.Error.Type, ErrTypeInvalidRequest)
	}
	if resp.Error.Code == nil || *resp.Error.Code != CodePayloadTooLarge {
		t.Errorf("code %v, want %q", resp.Error.Code, CodePayloadTooLarge)
	}
	if !strings.Contains(resp.Error.Message, "melebihi batas") {
		t.Errorf("pesan tidak menjelaskan sebab: %q", resp.Error.Message)
	}
	if !strings.Contains(resp.Error.Message, strconv.FormatInt(limit, 10)) {
		t.Errorf("pesan tidak menyebut batas %d: %q", limit, resp.Error.Message)
	}
}

func TestMaxBytesMenolakBodyDenganContentLengthBesar(t *testing.T) {
	const limit int64 = 16
	dipanggil := false
	handler := MaxBytes(limit)(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		dipanggil = true
	}))

	req := httptest.NewRequest(http.MethodPost, "/v1/chat/completions", strings.NewReader(strings.Repeat("x", 200)))
	rec := httptest.NewRecorder()
	handler.ServeHTTP(rec, req)

	assertPayloadTooLarge(t, rec, limit)
	if dipanggil {
		t.Error("handler tidak boleh dijalankan saat Content-Length sudah melewati batas")
	}
}

func TestMaxBytesMenolakBodyChunkedYangMelewatiBatas(t *testing.T) {
	const limit int64 = 16
	var readErr error
	handler := MaxBytes(limit)(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		// Handler sengaja tidak menulis apa pun saat gagal, meniru handler yang
		// menyerahkan pelaporan error ke middleware.
		_, readErr = io.ReadAll(r.Body)
	}))

	req := httptest.NewRequest(http.MethodPost, "/v1/chat/completions", newUnknownLengthBody(strings.Repeat("x", 200)))
	if req.ContentLength >= 0 {
		t.Fatalf("prasyarat test gagal: ContentLength %d, seharusnya tidak diketahui", req.ContentLength)
	}
	rec := httptest.NewRecorder()
	handler.ServeHTTP(rec, req)

	if readErr == nil {
		t.Fatal("pembacaan body seharusnya gagal karena melewati batas")
	}
	assertPayloadTooLarge(t, rec, limit)
}

func TestMaxBytesMelewatkanBodyDiBawahBatas(t *testing.T) {
	const payload = `{"model":"gpt-4o"}`
	var terbaca string
	handler := MaxBytes(1024)(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		b, err := io.ReadAll(r.Body)
		if err != nil {
			t.Errorf("membaca body: %v", err)
		}
		terbaca = string(b)
		w.WriteHeader(http.StatusAccepted)
	}))

	rec := httptest.NewRecorder()
	handler.ServeHTTP(rec, httptest.NewRequest(http.MethodPost, "/v1/chat/completions", strings.NewReader(payload)))

	if rec.Code != http.StatusAccepted {
		t.Fatalf("status %d, want 202", rec.Code)
	}
	if terbaca != payload {
		t.Fatalf("body diterima %q, want %q", terbaca, payload)
	}
}

func TestMaxBytesNolBerartiTanpaBatas(t *testing.T) {
	handler := MaxBytes(0)(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		b, err := io.ReadAll(r.Body)
		if err != nil {
			t.Errorf("membaca body: %v", err)
		}
		if len(b) != 100000 {
			t.Errorf("body terpotong: %d byte", len(b))
		}
		w.WriteHeader(http.StatusOK)
	}))

	rec := httptest.NewRecorder()
	handler.ServeHTTP(rec, httptest.NewRequest(http.MethodPost, "/", strings.NewReader(strings.Repeat("x", 100000))))

	if rec.Code != http.StatusOK {
		t.Fatalf("status %d, want 200", rec.Code)
	}
}

// --- AccessLog --------------------------------------------------------------

func TestAccessLogMencatatMetadataRequest(t *testing.T) {
	logger, logs := newTestLogger()

	handler := Chain(RequestID(), AccessLog(logger))(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusCreated)
		if _, err := io.WriteString(w, "selesai"); err != nil {
			t.Errorf("menulis body: %v", err)
		}
	}))

	req := httptest.NewRequest(http.MethodPost, "/v1/chat/completions", nil)
	req.RemoteAddr = "203.0.113.9:51234"
	rec := httptest.NewRecorder()
	handler.ServeHTTP(rec, req)

	out := logs.String()
	for _, want := range []string{
		`"status":201`,
		`"method":"POST"`,
		`"path":"/v1/chat/completions"`,
		`"bytes":7`,
		"duration_ms",
		`"remote_ip":"203.0.113.9"`,
		`"request_id":"` + rec.Header().Get(HeaderRequestID) + `"`,
	} {
		if !strings.Contains(out, want) {
			t.Errorf("log tidak memuat %s\nlog: %s", want, out)
		}
	}
}

func TestAccessLogMenaikkanLevelUntukKegagalan(t *testing.T) {
	logger, logs := newTestLogger()
	handler := AccessLog(logger)(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusBadGateway)
	}))

	handler.ServeHTTP(httptest.NewRecorder(), httptest.NewRequest(http.MethodGet, "/v1/models", nil))

	if !strings.Contains(logs.String(), `"level":"ERROR"`) {
		t.Fatalf("status 5xx harus dicatat pada level ERROR: %s", logs.String())
	}
}

// TestAccessLogTidakMembocorkanKredensial adalah pagar utama paket ini: kredensial
// pelanggan tidak boleh pernah mendarat di sistem log.
func TestAccessLogTidakMembocorkanKredensial(t *testing.T) {
	logger, logs := newTestLogger()

	handler := Chain(RequestID(), AccessLog(logger))(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
	}))

	req := httptest.NewRequest(http.MethodPost, "/v1/chat/completions?api_key=sk_live_rahasia123&model=gpt-4o", nil)
	req.Header.Set("Authorization", "Bearer sk_live_rahasia123")
	req.Header.Set("X-Api-Key", "sk_live_rahasia123")
	req.Header.Set("Cookie", "session=rahasia123")
	req.Header.Set("Proxy-Authorization", "Basic cmFoYXNpYTEyMw==")

	handler.ServeHTTP(httptest.NewRecorder(), req)

	out := logs.String()
	for _, terlarang := range []string{
		"rahasia123",
		"sk_live",
		"Bearer",
		"Authorization",
		"Cookie",
		"session=",
		"cmFoYXNpYTEyMw",
	} {
		if strings.Contains(out, terlarang) {
			t.Fatalf("log membocorkan %q\nlog: %s", terlarang, out)
		}
	}

	// Metadata yang tidak sensitif tetap harus ada, kalau tidak log jadi tak berguna.
	if !strings.Contains(out, `"path":"/v1/chat/completions"`) {
		t.Fatalf("path tidak tercatat: %s", out)
	}
	// Nama parameter query berguna untuk debug rute; nilainya yang harus hilang.
	if !strings.Contains(out, "api_key=") {
		t.Fatalf("nama parameter query hilang dari log: %s", out)
	}
	if !strings.Contains(out, "gpt-4o") {
		t.Fatalf("parameter query non-sensitif hilang dari log: %s", out)
	}
}

func TestRedactQuery(t *testing.T) {
	tests := []struct {
		raw  string
		want string
	}{
		{"", ""},
		{"model=gpt-4o", "model=gpt-4o"},
		{"api_key=sk_live_123", "api_key=%5BREDACTED%5D"},
		{"token=abc&model=gpt-4o", "model=gpt-4o&token=%5BREDACTED%5D"},
		{"password=p&secret=s", "password=%5BREDACTED%5D&secret=%5BREDACTED%5D"},
		{"%zz", observability.RedactedValue},
	}
	for _, tc := range tests {
		if got := redactQuery(tc.raw); got != tc.want {
			t.Errorf("redactQuery(%q) = %q, want %q", tc.raw, got, tc.want)
		}
	}
}

// --- RealIP -----------------------------------------------------------------

func TestRealIP(t *testing.T) {
	trusted, err := ParseTrustedProxies([]string{"10.0.0.0/8", "192.0.2.1", "::1"})
	if err != nil {
		t.Fatalf("menyiapkan daftar proxy: %v", err)
	}

	tests := []struct {
		name         string
		trusted      []netip.Prefix
		remoteAddr   string
		forwardedFor string
		realIP       string
		want         string
	}{
		{
			name:         "daftar kosong mengabaikan header sepenuhnya",
			trusted:      nil,
			remoteAddr:   "203.0.113.9:51234",
			forwardedFor: "198.51.100.7",
			realIP:       "198.51.100.8",
			want:         "203.0.113.9",
		},
		{
			name:         "peer tidak terpercaya tidak boleh memalsukan IP",
			trusted:      trusted,
			remoteAddr:   "203.0.113.9:51234",
			forwardedFor: "198.51.100.7",
			want:         "203.0.113.9",
		},
		{
			name:         "peer terpercaya membawa IP klien",
			trusted:      trusted,
			remoteAddr:   "10.0.0.1:51234",
			forwardedFor: "198.51.100.7",
			want:         "198.51.100.7",
		},
		{
			name:         "hop terpercaya di kanan dilewati",
			trusted:      trusted,
			remoteAddr:   "10.0.0.1:51234",
			forwardedFor: "198.51.100.7, 10.0.0.5, 10.0.0.1",
			want:         "198.51.100.7",
		},
		{
			name:         "klien tidak bisa menyisipkan hop di kiri",
			trusted:      trusted,
			remoteAddr:   "10.0.0.1:51234",
			forwardedFor: "1.1.1.1, 198.51.100.7",
			want:         "198.51.100.7",
		},
		{
			name:         "semua hop terpercaya memakai entri paling kiri",
			trusted:      trusted,
			remoteAddr:   "10.0.0.1:51234",
			forwardedFor: "10.1.2.3, 10.0.0.5",
			want:         "10.1.2.3",
		},
		{
			name:         "entri rusak menghentikan penelusuran",
			trusted:      trusted,
			remoteAddr:   "10.0.0.1:51234",
			forwardedFor: "198.51.100.7, bukan-ip",
			want:         "10.0.0.1",
		},
		{
			name:       "x-real-ip dipakai bila tidak ada x-forwarded-for",
			trusted:    trusted,
			remoteAddr: "10.0.0.1:51234",
			realIP:     "198.51.100.7",
			want:       "198.51.100.7",
		},
		{
			name:       "x-real-ip dari peer tidak terpercaya diabaikan",
			trusted:    trusted,
			remoteAddr: "203.0.113.9:51234",
			realIP:     "198.51.100.7",
			want:       "203.0.113.9",
		},
		{
			name:         "ipv6 terpercaya",
			trusted:      trusted,
			remoteAddr:   "[::1]:51234",
			forwardedFor: "2001:db8::1234",
			want:         "2001:db8::1234",
		},
		{
			name:       "alamat ipv4-mapped dinormalkan",
			trusted:    nil,
			remoteAddr: "[::ffff:203.0.113.9]:51234",
			want:       "203.0.113.9",
		},
		{
			name:         "proxy tunggal dalam daftar",
			trusted:      trusted,
			remoteAddr:   "192.0.2.1:443",
			forwardedFor: "198.51.100.7",
			want:         "198.51.100.7",
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			var got netip.Addr
			var ok bool
			handler := RealIP(tc.trusted)(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				got, ok = ClientIPFrom(r.Context())
			}))

			req := httptest.NewRequest(http.MethodGet, "/v1/models", nil)
			req.RemoteAddr = tc.remoteAddr
			if tc.forwardedFor != "" {
				req.Header.Set(headerForwardedFor, tc.forwardedFor)
			}
			if tc.realIP != "" {
				req.Header.Set(headerRealIP, tc.realIP)
			}

			handler.ServeHTTP(httptest.NewRecorder(), req)

			if !ok {
				t.Fatalf("IP klien tidak tersedia di context")
			}
			if got.String() != tc.want {
				t.Fatalf("IP klien %s, want %s", got, tc.want)
			}
		})
	}
}

func TestRealIPTidakMenimpaRemoteAddr(t *testing.T) {
	trusted, err := ParseTrustedProxies([]string{"10.0.0.0/8"})
	if err != nil {
		t.Fatalf("menyiapkan daftar proxy: %v", err)
	}

	var remoteAddr string
	handler := RealIP(trusted)(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		remoteAddr = r.RemoteAddr
	}))

	req := httptest.NewRequest(http.MethodGet, "/", nil)
	req.RemoteAddr = "10.0.0.1:51234"
	req.Header.Set(headerForwardedFor, "198.51.100.7")
	handler.ServeHTTP(httptest.NewRecorder(), req)

	if remoteAddr != "10.0.0.1:51234" {
		t.Fatalf("RemoteAddr berubah menjadi %q; peer asli harus tetap bisa dilihat", remoteAddr)
	}
}

func TestClientIPFromKosongTanpaRealIP(t *testing.T) {
	req := httptest.NewRequest(http.MethodGet, "/", nil)
	if _, ok := ClientIPFrom(req.Context()); ok {
		t.Fatal("ClientIPFrom harus melaporkan false bila RealIP tidak dipasang")
	}
}

func TestParseTrustedProxies(t *testing.T) {
	got, err := ParseTrustedProxies([]string{" 10.0.0.0/8 ", "", "192.0.2.7", "2001:db8::/32"})
	if err != nil {
		t.Fatalf("error tak terduga: %v", err)
	}
	if len(got) != 3 {
		t.Fatalf("jumlah prefix %d, want 3: %v", len(got), got)
	}
	if !got[1].Contains(netip.MustParseAddr("192.0.2.7")) {
		t.Errorf("IP tunggal tidak menjadi prefix /32: %v", got[1])
	}
	if got[1].Contains(netip.MustParseAddr("192.0.2.8")) {
		t.Errorf("prefix untuk IP tunggal terlalu lebar: %v", got[1])
	}

	if _, err := ParseTrustedProxies([]string{"bukan-cidr"}); err == nil {
		t.Fatal("nilai tidak sah harus menghasilkan error")
	}
}

// --- Wrapper ResponseWriter -------------------------------------------------

func TestResponseWriterMeneruskanFlushDanUnwrap(t *testing.T) {
	rec := httptest.NewRecorder()
	rw := wrapResponseWriter(rec)

	if _, ok := any(rw).(http.Flusher); !ok {
		t.Fatal("wrapper harus memenuhi http.Flusher")
	}
	if rw.Unwrap() != http.ResponseWriter(rec) {
		t.Fatal("Unwrap harus mengembalikan writer asli")
	}
	if wrapResponseWriter(rw) != rw {
		t.Fatal("wrapper yang sudah ada harus dipakai ulang, bukan dibungkus lagi")
	}

	if _, err := io.WriteString(rw, "abc"); err != nil {
		t.Fatalf("menulis: %v", err)
	}
	// Jalur inilah yang dipakai handler SSE; kalau Unwrap tidak ada, ini error.
	if err := http.NewResponseController(rw).Flush(); err != nil {
		t.Fatalf("http.ResponseController(w).Flush() gagal: %v", err)
	}
	if !rec.Flushed {
		t.Fatal("Flush tidak diteruskan ke writer di bawah")
	}
	if rw.status != http.StatusOK || rw.bytes != 3 {
		t.Fatalf("status %d bytes %d, want 200/3", rw.status, rw.bytes)
	}
}

func TestResponseWriterStatusDanHeaderInformasional(t *testing.T) {
	fake := newRecordingWriter()
	rw := wrapResponseWriter(fake)

	// 1xx boleh dikirim berulang dan bukan penutup header.
	rw.WriteHeader(http.StatusEarlyHints)
	if rw.wroteHeader {
		t.Fatal("respons 1xx tidak boleh menandai respons sudah dimulai")
	}

	rw.WriteHeader(http.StatusTeapot)
	rw.WriteHeader(http.StatusOK) // harus diabaikan
	if rw.status != http.StatusTeapot {
		t.Fatalf("status %d, want %d", rw.status, http.StatusTeapot)
	}
	if len(fake.codes) != 2 || fake.codes[0] != http.StatusEarlyHints || fake.codes[1] != http.StatusTeapot {
		t.Fatalf("WriteHeader diteruskan sebagai %v, want [103 418]", fake.codes)
	}
}

func TestChainMenjalankanMiddlewareBerurutan(t *testing.T) {
	var urutan []string
	tandai := func(name string) func(http.Handler) http.Handler {
		return func(next http.Handler) http.Handler {
			return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				urutan = append(urutan, name)
				next.ServeHTTP(w, r)
			})
		}
	}

	handler := Chain(tandai("luar"), nil, tandai("dalam"))(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		urutan = append(urutan, "handler")
	}))
	handler.ServeHTTP(httptest.NewRecorder(), httptest.NewRequest(http.MethodGet, "/", nil))

	if strings.Join(urutan, ",") != "luar,dalam,handler" {
		t.Fatalf("urutan eksekusi %v, want [luar dalam handler]", urutan)
	}
}

func TestResponseWriterMemenuhiHttpFlusherLangsung(t *testing.T) {
	rec := httptest.NewRecorder()
	// Pustaka lama memakai type assertion, bukan http.ResponseController; jalur itu
	// harus tetap bekerja.
	var w http.ResponseWriter = wrapResponseWriter(rec)
	flusher, ok := w.(http.Flusher)
	if !ok {
		t.Fatal("wrapper tidak memenuhi http.Flusher")
	}

	if _, err := io.WriteString(w, "data: satu\n\n"); err != nil {
		t.Fatalf("menulis: %v", err)
	}
	flusher.Flush()

	if !rec.Flushed {
		t.Fatal("Flush lewat type assertion tidak diteruskan")
	}
}

func TestResponseWriterReadFromMenghitungByte(t *testing.T) {
	rec := httptest.NewRecorder()
	rw := wrapResponseWriter(rec)
	const isi = "aset statis yang cukup panjang"

	// Sumber sengaja tanpa WriteTo supaya io.Copy benar-benar memakai ReadFrom milik
	// wrapper — jalur cepat yang dipakai saat menyajikan file statis.
	n, err := io.Copy(rw, newUnknownLengthBody(isi))
	if err != nil {
		t.Fatalf("io.Copy: %v", err)
	}
	if n != int64(len(isi)) || rw.bytes != int64(len(isi)) {
		t.Fatalf("io.Copy %d byte, tercatat %d, want %d", n, rw.bytes, len(isi))
	}
	if rec.Body.String() != isi {
		t.Fatalf("body %q, want %q", rec.Body.String(), isi)
	}
	if rw.status != http.StatusOK {
		t.Fatalf("status %d, want 200", rw.status)
	}
}

func TestResponseWriterHijack(t *testing.T) {
	t.Run("diteruskan ke koneksi nyata", func(t *testing.T) {
		logger, _ := newTestLogger()
		srv := httptest.NewServer(AccessLog(logger)(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			hj, ok := w.(http.Hijacker)
			if !ok {
				t.Error("wrapper tidak memenuhi http.Hijacker di server nyata")
				return
			}
			conn, buf, err := hj.Hijack()
			if err != nil {
				t.Errorf("Hijack: %v", err)
				return
			}
			defer conn.Close()
			if _, err := buf.WriteString("HTTP/1.1 200 OK\r\nContent-Length: 2\r\nConnection: close\r\n\r\nhi"); err != nil {
				t.Errorf("menulis respons mentah: %v", err)
				return
			}
			if err := buf.Flush(); err != nil {
				t.Errorf("flush respons mentah: %v", err)
			}
		})))
		defer srv.Close()

		resp, err := srv.Client().Get(srv.URL + "/ws")
		if err != nil {
			t.Fatalf("request: %v", err)
		}
		defer resp.Body.Close()
		body, err := io.ReadAll(resp.Body)
		if err != nil {
			t.Fatalf("membaca body: %v", err)
		}
		if string(body) != "hi" {
			t.Fatalf("body %q, want \"hi\"", body)
		}
	})

	t.Run("melaporkan tidak didukung", func(t *testing.T) {
		rw := wrapResponseWriter(httptest.NewRecorder())
		if _, _, err := rw.Hijack(); !errors.Is(err, http.ErrNotSupported) {
			t.Fatalf("error %v, want membungkus http.ErrNotSupported", err)
		}
	})
}

func TestLevelForStatus(t *testing.T) {
	tests := map[int]slog.Level{
		200: slog.LevelInfo,
		304: slog.LevelInfo,
		400: slog.LevelWarn,
		429: slog.LevelWarn,
		500: slog.LevelError,
		503: slog.LevelError,
	}
	for status, want := range tests {
		if got := levelForStatus(status); got != want {
			t.Errorf("levelForStatus(%d) = %s, want %s", status, got, want)
		}
	}
}

func TestRealIPRemoteAddrTidakBisaDiurai(t *testing.T) {
	logger, logs := newTestLogger()

	var ok bool
	handler := Chain(RealIP(PrivateProxyPrefixes()), AccessLog(logger))(
		http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			_, ok = ClientIPFrom(r.Context())
		}))

	req := httptest.NewRequest(http.MethodGet, "/v1/models", nil)
	// Bentuk yang muncul saat server mendengarkan di unix socket.
	req.RemoteAddr = "@"
	handler.ServeHTTP(httptest.NewRecorder(), req)

	if ok {
		t.Fatal("IP tidak boleh ditaruh di context bila RemoteAddr tidak bisa diurai")
	}
	if !strings.Contains(logs.String(), `"remote_ip":"@"`) {
		t.Fatalf("access log tidak memakai RemoteAddr apa adanya: %s", logs.String())
	}
}
