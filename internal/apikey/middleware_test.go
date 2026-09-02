package apikey

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"net/netip"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/NexGen-X/Route-X/internal/database/repo"
	"github.com/NexGen-X/Route-X/internal/database/repo/keys"
	"github.com/NexGen-X/Route-X/internal/httpx"
	"github.com/NexGen-X/Route-X/internal/observability"
	"github.com/NexGen-X/Route-X/internal/security"
)

// --- Perkakas uji ------------------------------------------------------------

// safeBuffer adalah penampung log yang aman dibaca sementara goroutine latar (pencatat
// pemakaian) masih mungkin menulis ke dalamnya.
type safeBuffer struct {
	mu  sync.Mutex
	buf bytes.Buffer
}

func (b *safeBuffer) Write(p []byte) (int, error) {
	b.mu.Lock()
	defer b.mu.Unlock()
	return b.buf.Write(p)
}

func (b *safeBuffer) String() string {
	b.mu.Lock()
	defer b.mu.Unlock()
	return b.buf.String()
}

// touchRecord adalah satu pemanggilan TouchUsage.
type touchRecord struct {
	id string
	ip netip.Addr
}

// fakeRepo adalah KeyAuthenticator tiruan.
//
// Tidak ada method di sini yang memakai *testing.T: TouchUsage dipanggil dari goroutine
// latar yang bisa hidup lebih lama dari test-nya, dan menyentuh T dari sana akan menjadi
// panik yang menyesatkan alih-alih kegagalan yang jelas. Yang dilakukan hanyalah mencatat
// ke channel berbuffer.
type fakeRepo struct {
	mu     sync.Mutex
	gotRaw []string
	gotIP  []netip.Addr

	key      *keys.Key
	reason   string
	err      error
	touchErr error

	touched chan touchRecord
}

func newFakeRepo(key *keys.Key) *fakeRepo {
	return &fakeRepo{key: key, touched: make(chan touchRecord, 8)}
}

func (f *fakeRepo) Authenticate(_ context.Context, raw string, clientIP netip.Addr) (*keys.Key, error) {
	f.mu.Lock()
	f.gotRaw = append(f.gotRaw, raw)
	f.gotIP = append(f.gotIP, clientIP)
	f.mu.Unlock()

	switch {
	case f.err != nil:
		return nil, f.err
	case f.reason != "":
		return nil, &keys.Rejection{Reason: f.reason}
	default:
		return f.key, nil
	}
}

func (f *fakeRepo) TouchUsage(_ context.Context, id string, ip netip.Addr) error {
	select {
	case f.touched <- touchRecord{id: id, ip: ip}:
	default:
	}
	return f.touchErr
}

func (f *fakeRepo) lastCall() (string, netip.Addr) {
	f.mu.Lock()
	defer f.mu.Unlock()
	if len(f.gotRaw) == 0 {
		return "", netip.Addr{}
	}
	return f.gotRaw[len(f.gotRaw)-1], f.gotIP[len(f.gotIP)-1]
}

// testKey membuat baris api_keys tiruan yang bentuknya sama dengan hasil repo asli.
func testKey() *keys.Key {
	return &keys.Key{
		ID:          "8c6f0a0e-0f2a-4a1c-9f0e-2d0b8a7c1f11",
		Name:        "kunci uji",
		Prefix:      security.KeyPrefixLive,
		Last4:       "DEFG",
		OwnerUserID: "1f2e3d4c-5b6a-4798-8899-aabbccddeeff",
		Status:      keys.StatusActive,
		Scopes:      []string{keys.ScopeInference, keys.ScopeModelsRead},
	}
}

func discardLogger() *slog.Logger { return slog.New(slog.DiscardHandler) }

// bearerRequest menyusun request /v1 dengan kredensial di header Authorization.
func bearerRequest() *http.Request {
	r := httptest.NewRequest("POST", "/v1/chat/completions", nil)
	r.Header.Set("Authorization", "Bearer "+testRawKey)
	return r
}

// okHandler membalas 200 dan mencatat principal yang diterimanya.
func okHandler(seen **Principal) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if p, ok := PrincipalFrom(r.Context()); ok && seen != nil {
			*seen = p
		}
		w.WriteHeader(http.StatusOK)
	})
}

// serve menjalankan satu request melalui RealIP lalu middleware autentikasi.
//
// RealIP ikut dipasang karena itulah satu-satunya sumber IP klien yang dipercaya
// httpx.ClientIPFrom, dan IP itu bagian dari kontrak repo.Authenticate.
func serve(a *Authenticator, next http.Handler, r *http.Request) *httptest.ResponseRecorder {
	rec := httptest.NewRecorder()
	httpx.RealIP(nil)(a.Authenticate()(next)).ServeHTTP(rec, r)
	return rec
}

// --- Autentikasi -------------------------------------------------------------

func TestAuthenticateSuccess(t *testing.T) {
	key := testKey()
	fake := newFakeRepo(key)
	a := NewAuthenticator(fake, nil, discardLogger())

	var seen *Principal
	rec := serve(a, okHandler(&seen), bearerRequest())

	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d, mau 200 (body: %s)", rec.Code, rec.Body.String())
	}
	if seen == nil {
		t.Fatal("principal tidak ada di context handler")
	}
	if seen.ID() != key.ID {
		t.Errorf("principal.ID() = %q, mau %q", seen.ID(), key.ID)
	}
	if seen.OwnerUserID() != key.OwnerUserID {
		t.Errorf("principal.OwnerUserID() = %q", seen.OwnerUserID())
	}
	if !seen.Can(keys.ScopeInference) {
		t.Error("principal kehilangan cakupan inference")
	}

	raw, ip := fake.lastCall()
	if raw != testRawKey {
		t.Errorf("repo menerima raw = %q", raw)
	}
	// httptest.NewRequest memakai RemoteAddr 192.0.2.1:1234.
	if ip.String() != "192.0.2.1" {
		t.Errorf("repo menerima ip = %q, mau 192.0.2.1", ip)
	}
}

func TestAuthenticateAcceptsAPIKeyHeader(t *testing.T) {
	fake := newFakeRepo(testKey())
	a := NewAuthenticator(fake, nil, discardLogger())

	r := httptest.NewRequest("POST", "/v1/chat/completions", nil)
	r.Header.Set("X-Api-Key", testRawKey)

	if rec := serve(a, okHandler(nil), r); rec.Code != http.StatusOK {
		t.Fatalf("status = %d, mau 200", rec.Code)
	}
	if raw, _ := fake.lastCall(); raw != testRawKey {
		t.Errorf("repo menerima raw = %q", raw)
	}
}

// Seluruh kegagalan autentikasi wajib menghasilkan 401 dengan body yang identik byte per
// byte. Ini inti kebijakan paket ini, jadi yang dibandingkan benar-benar byte-nya, bukan
// hanya status atau kode error.
func TestAuthenticateFailuresShareIdenticalBody(t *testing.T) {
	cases := []struct {
		name    string
		reason  string
		prepare func(r *http.Request)
	}{
		{name: "tanpa kredensial", prepare: func(*http.Request) {}},
		{
			name: "skema salah",
			prepare: func(r *http.Request) {
				r.Header.Set("Authorization", "Basic "+testRawKey)
			},
		},
		{
			name: "kredensial bertentangan",
			prepare: func(r *http.Request) {
				r.Header.Set("Authorization", "Bearer "+testRawKey)
				r.Header.Set("X-Api-Key", "sk_live_ZYXWVUTSRQPONMLKJIHGFEDCBA9876543210zyxwvut")
			},
		},
		{name: "not_found", reason: keys.ReasonNotFound},
		{name: "disabled", reason: keys.ReasonDisabled},
		{name: "revoked", reason: keys.ReasonRevoked},
		{name: "expired", reason: keys.ReasonExpired},
		{name: "ip_not_allowed", reason: keys.ReasonIPNotAllowed},
		{name: "banned", reason: keys.ReasonBanned},
	}

	var (
		first     []byte
		firstName string
	)
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			fake := newFakeRepo(testKey())
			fake.reason = tc.reason
			a := NewAuthenticator(fake, nil, discardLogger())

			r := httptest.NewRequest("POST", "/v1/chat/completions", nil)
			if tc.prepare != nil {
				tc.prepare(r)
			} else {
				r.Header.Set("Authorization", "Bearer "+testRawKey)
			}

			next := http.HandlerFunc(func(http.ResponseWriter, *http.Request) {
				t.Error("handler dijalankan padahal autentikasi gagal")
			})
			rec := serve(a, next, r)

			if rec.Code != http.StatusUnauthorized {
				t.Fatalf("status = %d, mau 401 (body: %s)", rec.Code, rec.Body.String())
			}
			if got := rec.Header().Get("Content-Type"); got != "application/json" {
				t.Errorf("Content-Type = %q", got)
			}

			body := rec.Body.Bytes()
			if first == nil {
				first, firstName = bytes.Clone(body), tc.name
				return
			}
			if !bytes.Equal(first, body) {
				t.Errorf("body berbeda dari kasus %q:\n %q\n %q", firstName, first, body)
			}
		})
	}

	// Envelope-nya diperiksa sekali, di sini, supaya kalau bentuknya salah tidak semua
	// kasus di atas ikut gagal dan menutupi sebabnya.
	var envelope httpx.ErrorResponse
	if err := json.Unmarshal(first, &envelope); err != nil {
		t.Fatalf("body bukan JSON: %v (%s)", err, first)
	}
	if envelope.Error.Type != httpx.ErrTypeAuthentication {
		t.Errorf("error.type = %q, mau %q", envelope.Error.Type, httpx.ErrTypeAuthentication)
	}
	if envelope.Error.Code == nil || *envelope.Error.Code != httpx.CodeInvalidAPIKey {
		t.Errorf("error.code = %v, mau %q", envelope.Error.Code, httpx.CodeInvalidAPIKey)
	}
	if envelope.Error.Message != MessageInvalidKey {
		t.Errorf("error.message = %q, mau %q", envelope.Error.Message, MessageInvalidKey)
	}
}

// Kegagalan infrastruktur bukan kegagalan kredensial: 500, bukan 401. Membalas 401 akan
// menyuruh pelanggan memutar API key-nya untuk masalah yang bukan miliknya.
func TestAuthenticateInfrastructureFailureIs500(t *testing.T) {
	fake := newFakeRepo(testKey())
	fake.err = &repo.Error{Op: "memverifikasi API key", Code: "57014"}
	logs := &safeBuffer{}
	a := NewAuthenticator(fake, nil, slog.New(slog.NewJSONHandler(logs, nil)))

	rec := serve(a, okHandler(nil), bearerRequest())
	if rec.Code != http.StatusInternalServerError {
		t.Fatalf("status = %d, mau 500 (body: %s)", rec.Code, rec.Body.String())
	}

	var envelope httpx.ErrorResponse
	if err := json.Unmarshal(rec.Body.Bytes(), &envelope); err != nil {
		t.Fatalf("body bukan JSON: %v", err)
	}
	if envelope.Error.Type != httpx.ErrTypeAPI {
		t.Errorf("error.type = %q, mau %q", envelope.Error.Type, httpx.ErrTypeAPI)
	}
	if logs.String() == "" {
		t.Error("kegagalan infrastruktur tidak dicatat")
	}
}

// Sentinel keys.ErrKeyNotUsable harus tetap terbaca lewat Rejection, karena middleware
// membedakan penolakan dari kegagalan infrastruktur berdasarkan tipe itu.
func TestAuthenticateRejectionUnwrapsToSentinel(t *testing.T) {
	fake := newFakeRepo(nil)
	fake.err = errors.Join(&keys.Rejection{Reason: keys.ReasonRevoked})
	a := NewAuthenticator(fake, nil, discardLogger())

	if rec := serve(a, okHandler(nil), bearerRequest()); rec.Code != http.StatusUnauthorized {
		t.Errorf("status = %d, mau 401", rec.Code)
	}
}

// Nilai API key mentah tidak boleh muncul di log maupun di respons, pada jalur berhasil
// maupun gagal. Key yang benar-benar berbentuk sah dikirim di kedua header yang diterima,
// karena itulah bentuk yang paling mungkin lolos tanpa sengaja.
func TestNoRawKeyInLogsOrResponse(t *testing.T) {
	scenarios := []struct {
		name      string
		reason    string
		wantTouch bool
		prepare   func(r *http.Request)
	}{
		{
			name:      "berhasil lewat authorization",
			wantTouch: true,
			prepare: func(r *http.Request) {
				r.Header.Set("Authorization", "Bearer "+testRawKey)
			},
		},
		{
			name:      "berhasil lewat x-api-key",
			wantTouch: true,
			prepare:   func(r *http.Request) { r.Header.Set("X-Api-Key", testRawKey) },
		},
		{
			name:   "ditolak karena dicabut",
			reason: keys.ReasonRevoked,
			prepare: func(r *http.Request) {
				r.Header.Set("Authorization", "Bearer "+testRawKey)
			},
		},
		{
			name: "ditolak karena bentuk header salah",
			prepare: func(r *http.Request) {
				r.Header.Set("Authorization", "Basic "+testRawKey)
			},
		},
		{
			name: "ditolak karena bertentangan",
			prepare: func(r *http.Request) {
				r.Header.Set("Authorization", "Bearer "+testRawKey)
				r.Header.Set("X-Api-Key", testRawKey+"X")
			},
		},
	}

	for _, tc := range scenarios {
		t.Run(tc.name, func(t *testing.T) {
			logs := &safeBuffer{}
			fake := newFakeRepo(testKey())
			fake.reason = tc.reason
			// Ambang nol supaya pencatatan pemakaian ikut berjalan dan lognya juga
			// terperiksa.
			a := NewAuthenticator(fake, nil, slog.New(slog.NewJSONHandler(logs, nil)),
				WithTouchInterval(time.Nanosecond))

			r := httptest.NewRequest("POST", "/v1/chat/completions", nil)
			tc.prepare(r)
			rec := serve(a, okHandler(nil), r)

			if containsRawKey(rec.Body.String()) {
				t.Errorf("respons memuat nilai key mentah: %s", rec.Body.String())
			}
			for name, values := range rec.Header() {
				for _, v := range values {
					if containsRawKey(v) {
						t.Errorf("header respons %s memuat nilai key mentah", name)
					}
				}
			}
			// Menunggu goroutine pencatat pemakaian selesai lebih dulu, supaya lognya
			// ikut terbaca.
			if tc.wantTouch {
				select {
				case <-fake.touched:
				case <-time.After(2 * time.Second):
					t.Fatal("pencatatan pemakaian tidak pernah terjadi")
				}
			}
			if got := logs.String(); containsRawKey(got) {
				t.Errorf("log memuat nilai key mentah: %s", got)
			}
		})
	}
}

// --- Cakupan -----------------------------------------------------------------

func TestRequireScope(t *testing.T) {
	key := testKey() // inference + models:read

	tests := []struct {
		name       string
		scope      string
		principal  *Principal
		wantStatus int
	}{
		{name: "cakupan ada", scope: keys.ScopeInference, principal: &Principal{Key: key}, wantStatus: http.StatusOK},
		{name: "cakupan lain ada", scope: keys.ScopeModelsRead, principal: &Principal{Key: key}, wantStatus: http.StatusOK},
		{name: "cakupan tidak ada", scope: keys.ScopeAdminWrite, principal: &Principal{Key: key}, wantStatus: http.StatusForbidden},
		{name: "tanpa principal", scope: keys.ScopeInference, wantStatus: http.StatusUnauthorized},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			next := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				w.WriteHeader(http.StatusOK)
			})

			r := httptest.NewRequest("POST", "/v1/chat/completions", nil)
			// RequireScope tidak punya penerima, jadi loggernya diambil dari context.
			r = r.WithContext(observability.WithLogger(r.Context(), discardLogger()))
			if tc.principal != nil {
				r = r.WithContext(WithPrincipal(r.Context(), tc.principal))
			}
			rec := httptest.NewRecorder()
			RequireScope(tc.scope)(next).ServeHTTP(rec, r)

			if rec.Code != tc.wantStatus {
				t.Fatalf("status = %d, mau %d (body: %s)", rec.Code, tc.wantStatus, rec.Body.String())
			}
			if tc.wantStatus == http.StatusOK {
				return
			}

			var envelope httpx.ErrorResponse
			if err := json.Unmarshal(rec.Body.Bytes(), &envelope); err != nil {
				t.Fatalf("body bukan JSON: %v", err)
			}
			if tc.wantStatus == http.StatusForbidden {
				if envelope.Error.Type != httpx.ErrTypePermission {
					t.Errorf("error.type = %q, mau %q", envelope.Error.Type, httpx.ErrTypePermission)
				}
				// Cakupan yang kurang memang disebutkan: pemegang key sudah terbukti
				// sah, jadi tidak ada yang bocor, dan ia perlu tahu apa yang harus
				// diperbaiki.
				if !strings.Contains(envelope.Error.Message, tc.scope) {
					t.Errorf("pesan %q tidak menyebut cakupan %q", envelope.Error.Message, tc.scope)
				}
			}
			if tc.wantStatus == http.StatusUnauthorized && envelope.Error.Type != httpx.ErrTypeAuthentication {
				t.Errorf("error.type = %q, mau %q", envelope.Error.Type, httpx.ErrTypeAuthentication)
			}
		})
	}
}

// --- Pencatatan pemakaian ----------------------------------------------------

func TestTouchUsageOnlyWhenStale(t *testing.T) {
	recent := time.Now().Add(-time.Minute)
	old := time.Now().Add(-time.Hour)

	tests := []struct {
		name       string
		lastUsedAt *time.Time
		interval   time.Duration
		wantTouch  bool
	}{
		{name: "belum pernah dipakai", lastUsedAt: nil, interval: DefaultTouchInterval, wantTouch: true},
		{name: "jejak sudah tua", lastUsedAt: &old, interval: DefaultTouchInterval, wantTouch: true},
		{name: "jejak masih baru", lastUsedAt: &recent, interval: DefaultTouchInterval, wantTouch: false},
		{name: "pencatatan dimatikan", lastUsedAt: nil, interval: 0, wantTouch: false},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			key := testKey()
			key.LastUsedAt = tc.lastUsedAt
			fake := newFakeRepo(key)
			a := NewAuthenticator(fake, nil, discardLogger(), WithTouchInterval(tc.interval))

			if rec := serve(a, okHandler(nil), bearerRequest()); rec.Code != http.StatusOK {
				t.Fatalf("status = %d, mau 200", rec.Code)
			}

			// Menunggu lama hanya masuk akal untuk kasus yang memang menunggu sesuatu;
			// kasus sebaliknya cukup diberi jeda singkat untuk membuktikan tidak ada
			// yang datang.
			timeout := 300 * time.Millisecond
			if tc.wantTouch {
				timeout = 2 * time.Second
			}
			select {
			case got := <-fake.touched:
				if !tc.wantTouch {
					t.Fatalf("pemakaian dicatat padahal tidak seharusnya (%+v)", got)
				}
				if got.id != key.ID {
					t.Errorf("TouchUsage id = %q, mau %q", got.id, key.ID)
				}
				if got.ip.String() != "192.0.2.1" {
					t.Errorf("TouchUsage ip = %q", got.ip)
				}
			case <-time.After(timeout):
				if tc.wantTouch {
					t.Fatal("pemakaian tidak pernah dicatat")
				}
			}
		})
	}
}

// Kegagalan pencatatan pemakaian tidak boleh menggagalkan request.
func TestTouchUsageFailureDoesNotFailRequest(t *testing.T) {
	fake := newFakeRepo(testKey())
	fake.touchErr = errors.New("koneksi database terputus")
	logs := &safeBuffer{}
	a := NewAuthenticator(fake, nil, slog.New(slog.NewJSONHandler(logs, nil)))

	if rec := serve(a, okHandler(nil), bearerRequest()); rec.Code != http.StatusOK {
		t.Fatalf("status = %d, mau 200", rec.Code)
	}
	select {
	case <-fake.touched:
	case <-time.After(2 * time.Second):
		t.Fatal("TouchUsage tidak pernah dipanggil")
	}

	// Log peringatannya ditulis dari goroutine latar; beri kesempatan sampai muncul.
	deadline := time.Now().Add(2 * time.Second)
	for !strings.Contains(logs.String(), "gagal mencatat pemakaian API key") {
		if time.Now().After(deadline) {
			t.Fatalf("kegagalan pencatatan tidak dilaporkan: %s", logs.String())
		}
		time.Sleep(10 * time.Millisecond)
	}
}

// --- IP klien ----------------------------------------------------------------

// IP yang diteruskan ke repo harus hasil resolusi RealIP, bukan RemoteAddr mentah:
// itulah alamat yang dipakai menegakkan ip_allowlist dan memeriksa blokir.
func TestAuthenticateUsesResolvedClientIP(t *testing.T) {
	fake := newFakeRepo(testKey())
	a := NewAuthenticator(fake, nil, discardLogger())

	r := bearerRequest()
	r.RemoteAddr = "127.0.0.1:41234"
	r.Header.Set("X-Forwarded-For", "203.0.113.7")

	rec := httptest.NewRecorder()
	httpx.RealIP(httpx.PrivateProxyPrefixes())(a.Authenticate()(okHandler(nil))).ServeHTTP(rec, r)

	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d, mau 200", rec.Code)
	}
	if _, ip := fake.lastCall(); ip.String() != "203.0.113.7" {
		t.Errorf("repo menerima ip = %q, mau 203.0.113.7", ip)
	}
}

// Tanpa RealIP tidak ada IP di context. Repo harus menerima alamat kosong, bukan nilai
// karangan — repo memperlakukannya sebagai "tidak cocok dengan daftar putih mana pun".
func TestAuthenticateWithoutRealIP(t *testing.T) {
	fake := newFakeRepo(testKey())
	a := NewAuthenticator(fake, nil, discardLogger())

	rec := httptest.NewRecorder()
	a.Authenticate()(okHandler(nil)).ServeHTTP(rec, bearerRequest())

	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d, mau 200", rec.Code)
	}
	if _, ip := fake.lastCall(); ip.IsValid() {
		t.Errorf("repo menerima ip = %q, mau alamat kosong", ip)
	}
}

// --- Metrik dan log ----------------------------------------------------------

// Dua Authenticator di atas registry yang sama harus memakai penghitung yang sama, bukan
// panik. Pendaftaran ganda yang tidak ditangani akan mematikan proses saat start justru
// pada deployment yang merakit lebih dari satu jalur autentikasi.
func TestAuthMetricsSharedAcrossInstances(t *testing.T) {
	metrics := observability.NewMetrics()

	first := NewAuthenticator(newFakeRepo(testKey()), metrics, discardLogger())
	second := NewAuthenticator(newFakeRepo(testKey()), metrics, discardLogger())
	if first.rejected != second.rejected {
		t.Fatal("Authenticator kedua tidak memakai penghitung yang sudah terdaftar")
	}
	// Logger nil jatuh ke logger default proses, bukan menjadi panic saat dipakai.
	if NewAuthenticator(newFakeRepo(nil), nil, nil).logger == nil {
		t.Error("logger nil tidak diganti logger default")
	}

	for _, a := range []*Authenticator{first, second} {
		rec := serve(a, okHandler(nil), httptest.NewRequest("POST", "/v1/chat/completions", nil))
		if rec.Code != http.StatusUnauthorized {
			t.Fatalf("status = %d, mau 401", rec.Code)
		}
	}

	families, err := metrics.Registry().Gather()
	if err != nil {
		t.Fatalf("mengumpulkan metrik: %v", err)
	}
	var got float64
	for _, family := range families {
		if family.GetName() != "routex_apikey_auth_rejected_total" {
			continue
		}
		for _, metric := range family.GetMetric() {
			for _, pair := range metric.GetLabel() {
				if pair.GetName() == "reason" && pair.GetValue() == ReasonMissingCredential {
					got = metric.GetCounter().GetValue()
				}
			}
		}
	}
	if got != 2 {
		t.Errorf("penghitung penolakan = %v, mau 2", got)
	}
}

// Log dari jalur ini harus bisa dikorelasikan dengan request_id, karena itulah pegangan
// yang dipakai dukungan saat pelanggan melaporkan 401 yang tidak ia mengerti.
func TestRejectionLogCarriesRequestID(t *testing.T) {
	const requestID = "req-uji-0123456789"

	logs := &safeBuffer{}
	fake := newFakeRepo(testKey())
	fake.reason = keys.ReasonRevoked
	a := NewAuthenticator(fake, nil, slog.New(slog.NewJSONHandler(logs, nil)))

	r := bearerRequest()
	r = r.WithContext(observability.WithRequestID(r.Context(), requestID))
	if rec := serve(a, okHandler(nil), r); rec.Code != http.StatusUnauthorized {
		t.Fatalf("status = %d, mau 401", rec.Code)
	}

	out := logs.String()
	if !strings.Contains(out, requestID) {
		t.Errorf("log tidak memuat request_id: %s", out)
	}
	if !strings.Contains(out, keys.ReasonRevoked) {
		t.Errorf("log tidak memuat alasan penolakan: %s", out)
	}
}
