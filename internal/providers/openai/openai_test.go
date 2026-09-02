package openai

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"log/slog"
	"net"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync"
	"testing"
	"testing/iotest"
	"time"

	"github.com/NexGen-X/Route-X/internal/providers"
	"github.com/NexGen-X/Route-X/internal/security"
)

// Adapter ini wajib memenuhi kontrak paket induk seluruhnya; pemeriksaannya dilakukan pada
// waktu kompilasi, di sisi produksi (openai.go) maupun di sisi test.
var _ providers.Provider = (*Provider)(nil)

// testKey sengaja dibuat menyerupai kunci sungguhan supaya pencarian kebocoran di pesan
// error dan di keluaran fmt benar-benar bermakna.
const testKey = "sk-live-RAHASIA-jangan-sampai-bocor-4f2a9c"

// upstream adalah test double untuk provider pihak ketiga berdialek OpenAI.
//
// Yang dipalsukan hanya server di ujung kabel. Klien HTTP, penyusunan body, pembacaan SSE,
// dan klasifikasi error yang diuji di bawah adalah kode yang sama persis dengan produksi.
type upstream struct {
	t   *testing.T
	srv *httptest.Server

	mu   sync.Mutex
	reqs []capturedRequest
}

// capturedRequest adalah permintaan yang diterima test double.
type capturedRequest struct {
	method string
	path   string
	query  string
	header http.Header
	body   []byte
}

func newUpstream(t *testing.T, handler http.HandlerFunc) *upstream {
	t.Helper()
	u := &upstream{t: t}
	u.srv = httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		body, _ := io.ReadAll(r.Body)
		u.mu.Lock()
		u.reqs = append(u.reqs, capturedRequest{
			method: r.Method,
			path:   r.URL.Path,
			query:  r.URL.RawQuery,
			header: r.Header.Clone(),
			body:   body,
		})
		u.mu.Unlock()
		handler(w, r)
	}))
	t.Cleanup(func() {
		// Koneksi klien ditutup lebih dulu: sebagian handler di bawah sengaja menahan
		// respons sampai kliennya pergi, dan Close sendirian akan menunggu selamanya.
		u.srv.CloseClientConnections()
		u.srv.Close()
	})
	return u
}

// url adalah base URL test double.
func (u *upstream) url() string { return u.srv.URL }

// last mengembalikan permintaan terakhir yang diterima.
func (u *upstream) last() capturedRequest {
	u.t.Helper()
	u.mu.Lock()
	defer u.mu.Unlock()
	if len(u.reqs) == 0 {
		u.t.Fatal("upstream tidak menerima satu pun permintaan")
	}
	return u.reqs[len(u.reqs)-1]
}

// lastBody mengurai body JSON permintaan terakhir.
func (u *upstream) lastBody() map[string]any {
	u.t.Helper()
	var decoded map[string]any
	if err := json.Unmarshal(u.last().body, &decoded); err != nil {
		u.t.Fatalf("body permintaan bukan JSON: %v", err)
	}
	return decoded
}

// testProvider membuat Provider yang menunjuk ke test double.
func testProvider(t *testing.T, baseURL string, tweak func(*Config)) *Provider {
	t.Helper()
	cfg := Config{
		Name:       "uji",
		Kind:       providers.KindOpenAI,
		BaseURL:    baseURL,
		Credential: security.Secret(testKey),
		Timeout:    3 * time.Second,
		// httptest mendengarkan di 127.0.0.1, tepat di rentang yang dijaga penjaga SSRF.
		// Melepas alamat privat adalah satu-satunya cara test bicara dengan test double-nya
		// sendiri — dan itu memang pilihan yang tersedia bagi operator yang menjalankan
		// model lokal.
		SSRFPolicy: security.SSRFPolicy{AllowHTTP: true, AllowPrivate: true},
	}
	if tweak != nil {
		tweak(&cfg)
	}
	p, err := New(cfg)
	if err != nil {
		t.Fatalf("New: %v", err)
	}
	return p
}

// chatRequest adalah permintaan chat paling sederhana yang sah.
func chatRequest() *providers.ChatRequest {
	return &providers.ChatRequest{
		Model:    "gpt-4o-mini",
		Messages: []providers.Message{{Role: providers.RoleUser, Content: "halo"}},
	}
}

// jsonHandler menjawab satu body JSON dengan status tertentu.
func jsonHandler(status int, body string) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(status)
		_, _ = io.WriteString(w, body)
	}
}

func TestNewValidation(t *testing.T) {
	base := "https://api.openai.com/v1"
	tests := []struct {
		name    string
		cfg     Config
		wantErr bool
	}{
		{"openai lengkap", Config{Name: "a", Kind: providers.KindOpenAI, BaseURL: base, Credential: testKey}, false},
		{"compatible tanpa kredensial", Config{Name: "a", Kind: providers.KindOpenAICompatible, BaseURL: base}, false},
		{"custom tanpa kredensial", Config{Name: "a", Kind: providers.KindCustom, BaseURL: base}, false},
		{"kind kosong", Config{Name: "a", BaseURL: base, Credential: testKey}, true},
		// Dialek lain punya adapternya sendiri.
		{"kind anthropic", Config{Name: "a", Kind: providers.KindAnthropic, BaseURL: base, Credential: testKey}, true},
		{"base URL kosong", Config{Name: "a", Kind: providers.KindOpenAI, Credential: testKey}, true},
		{"base URL hanya spasi", Config{Name: "a", Kind: providers.KindOpenAI, BaseURL: "   ", Credential: testKey}, true},
		// api.openai.com selalu menuntut kredensial, jadi konfigurasi tanpa kunci pasti salah.
		{"openai tanpa kredensial", Config{Name: "a", Kind: providers.KindOpenAI, BaseURL: base}, true},
		{"proxy tidak didukung", Config{Name: "a", Kind: providers.KindOpenAI, BaseURL: base, Credential: testKey,
			ProxyURL: "ftp://proxy.internal:21"}, true},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			p, err := New(tc.cfg)
			if tc.wantErr {
				if err == nil {
					t.Fatal("New berhasil padahal konfigurasinya tidak sah")
				}
				if strings.Contains(err.Error(), testKey) {
					t.Errorf("pesan New membocorkan kredensial: %v", err)
				}
				return
			}
			if err != nil {
				t.Fatalf("New: %v", err)
			}
			if p.Kind() != tc.cfg.Kind {
				t.Errorf("Kind() = %q, mau %q", p.Kind(), tc.cfg.Kind)
			}
		})
	}
}

// Nama kosong tidak boleh menghasilkan label metrik kosong.
func TestNewFallsBackToKindAsName(t *testing.T) {
	p, err := New(Config{Kind: providers.KindCustom, BaseURL: "https://x.test/v1"})
	if err != nil {
		t.Fatalf("New: %v", err)
	}
	if p.Name() != providers.KindCustom {
		t.Errorf("Name() = %q, mau %q", p.Name(), providers.KindCustom)
	}
}

// ExtraHeaders disalin: map milik pemanggil yang berubah setelah New tidak boleh mengubah
// header yang terkirim, dan tidak boleh menjadi data race.
func TestNewCopiesExtraHeaders(t *testing.T) {
	u := newUpstream(t, jsonHandler(200, `{"data":[]}`))
	headers := map[string]string{"X-Tenant": "awal"}
	p := testProvider(t, u.url(), func(c *Config) { c.ExtraHeaders = headers })

	headers["X-Tenant"] = "diubah setelah New"
	if _, err := p.Models(context.Background()); err != nil {
		t.Fatalf("Models: %v", err)
	}
	if got := u.last().header.Get("X-Tenant"); got != "awal" {
		t.Errorf("X-Tenant = %q, mau %q", got, "awal")
	}
}

// Kredensial harus benar-benar terkirim sebagai bearer token, dan header khusus OpenAI
// hanya ikut bila diisi.
func TestRequestHeaders(t *testing.T) {
	u := newUpstream(t, jsonHandler(200, `{"data":[]}`))
	p := testProvider(t, u.url(), func(c *Config) {
		c.Organization = "org-123"
		c.Project = "proj-456"
		c.ExtraHeaders = map[string]string{"X-Tenant": "acme", "X-Trace": "abc"}
	})

	if _, err := p.Models(context.Background()); err != nil {
		t.Fatalf("Models: %v", err)
	}

	h := u.last().header
	if got, want := h.Get("Authorization"), "Bearer "+testKey; got != want {
		t.Errorf("Authorization = %q, mau %q", got, want)
	}
	if got := h.Get("OpenAI-Organization"); got != "org-123" {
		t.Errorf("OpenAI-Organization = %q", got)
	}
	if got := h.Get("OpenAI-Project"); got != "proj-456" {
		t.Errorf("OpenAI-Project = %q", got)
	}
	if got := h.Get("X-Tenant"); got != "acme" {
		t.Errorf("X-Tenant = %q", got)
	}
	if got := h.Get("X-Trace"); got != "abc" {
		t.Errorf("X-Trace = %q", got)
	}
}

// Organization, Project, dan ExtraHeaders yang kosong tidak boleh dikirim sebagai header
// kosong: sebagian provider menolak header yang dikenalnya tapi tanpa nilai.
func TestHeadersOmittedWhenEmpty(t *testing.T) {
	u := newUpstream(t, jsonHandler(200, `{"data":[]}`))
	p := testProvider(t, u.url(), nil)

	if _, err := p.Models(context.Background()); err != nil {
		t.Fatalf("Models: %v", err)
	}
	h := u.last().header
	for _, name := range []string{"OpenAI-Organization", "OpenAI-Project"} {
		if _, ada := h[name]; ada {
			t.Errorf("header %s terkirim padahal tidak dikonfigurasi", name)
		}
	}
}

// ExtraHeaders ada untuk MENAMBAH header, bukan mengganti kredensial.
func TestExtraHeadersCannotOverrideGatewayHeaders(t *testing.T) {
	u := newUpstream(t, jsonHandler(200, `{"data":[]}`))
	p := testProvider(t, u.url(), func(c *Config) {
		c.ExtraHeaders = map[string]string{
			"Authorization":       "Bearer kunci-milik-orang-lain",
			"OpenAI-Organization": "org-penyerang",
			"Content-Type":        "text/plain",
		}
		c.Organization = "org-sah"
	})

	if _, err := p.Models(context.Background()); err != nil {
		t.Fatalf("Models: %v", err)
	}
	h := u.last().header
	if got, want := h.Get("Authorization"), "Bearer "+testKey; got != want {
		t.Errorf("Authorization = %q, mau %q", got, want)
	}
	if got := h.Get("OpenAI-Organization"); got != "org-sah" {
		t.Errorf("OpenAI-Organization = %q, mau org-sah", got)
	}
}

// Provider kind compatible boleh tanpa kredensial (model lokal), dan saat itu header
// Authorization tidak boleh dikirim sama sekali.
func TestNoAuthorizationHeaderWithoutCredential(t *testing.T) {
	u := newUpstream(t, jsonHandler(200, `{"data":[]}`))
	p := testProvider(t, u.url(), func(c *Config) {
		c.Kind = providers.KindOpenAICompatible
		c.Credential = ""
	})

	if _, err := p.Models(context.Background()); err != nil {
		t.Fatalf("Models: %v", err)
	}
	if _, ada := u.last().header["Authorization"]; ada {
		t.Error("header Authorization terkirim padahal tidak ada kredensial")
	}
}

// Klasifikasi status inilah yang menentukan apakah mesin routing mengulang, mengalihkan,
// atau berhenti. Satu baris di tabel ini berubah berarti perilaku produksi berubah.
func TestErrorClassificationPerStatus(t *testing.T) {
	tests := []struct {
		name     string
		status   int
		body     string
		wantKind providers.ErrorKind
		wantCode string
	}{
		{"401 auth", 401, `{"error":{"message":"Incorrect API key","type":"invalid_request_error","code":"invalid_api_key"}}`,
			providers.ErrKindAuth, "invalid_api_key"},
		{"403 permission", 403, `{"error":{"message":"tidak berhak"}}`, providers.ErrKindPermission, ""},
		{"404 model tidak ada", 404, `{"error":{"message":"model tidak dikenal"}}`, providers.ErrKindModelNotFound, ""},
		{"429 rate limit", 429, `{"error":{"message":"terlalu cepat","code":"rate_limit_exceeded"}}`,
			providers.ErrKindRateLimit, "rate_limit_exceeded"},
		// 429 dipakai untuk dua hal yang menuntut keputusan berbeda: menunggu menolong yang
		// pertama, hanya provider lain yang menolong yang kedua.
		{"429 kuota habis", 429, `{"error":{"message":"saldo habis","code":"insufficient_quota","type":"insufficient_quota"}}`,
			providers.ErrKindQuota, "insufficient_quota"},
		{"500 server", 500, `{"error":{"message":"galat internal"}}`, providers.ErrKindServer, ""},
		{"503 penuh", 503, `{"error":{"message":"sedang penuh"}}`, providers.ErrKindOverloaded, ""},
		{"400 bentuk salah", 400, `{"error":{"message":"terlalu panjang","code":"context_length_exceeded"}}`,
			providers.ErrKindInvalidRequest, "context_length_exceeded"},
		// Kebijakan konten tidak boleh dialihkan: provider lain kemungkinan besar menolak
		// dengan alasan sama, dan mencoba semuanya berarti mengirim muatan itu ke banyak pihak.
		{"400 kebijakan konten", 400, `{"error":{"message":"ditolak","code":"content_policy_violation"}}`,
			providers.ErrKindContentFilter, "content_policy_violation"},
		{"408 timeout upstream", 408, `{"error":{"message":"terlalu lama"}}`, providers.ErrKindTimeout, ""},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			u := newUpstream(t, jsonHandler(tc.status, tc.body))
			p := testProvider(t, u.url(), nil)

			_, err := p.ChatCompletion(context.Background(), chatRequest())
			if err == nil {
				t.Fatal("status gagal tidak menghasilkan error")
			}
			e := providers.AsError(err)
			if e == nil {
				t.Fatalf("error bukan *providers.Error: %v", err)
			}
			if e.Kind != tc.wantKind {
				t.Errorf("Kind = %s, mau %s", e.Kind, tc.wantKind)
			}
			if e.StatusCode != tc.status {
				t.Errorf("StatusCode = %d, mau %d", e.StatusCode, tc.status)
			}
			if e.UpstreamCode != tc.wantCode {
				t.Errorf("UpstreamCode = %q, mau %q", e.UpstreamCode, tc.wantCode)
			}
			if e.Provider != "uji" {
				t.Errorf("Provider = %q, mau uji", e.Provider)
			}
		})
	}
}

// Retry-After menentukan berapa lama mesin routing menunggu sebelum mencoba lagi.
func TestRetryAfterParsedFromHeader(t *testing.T) {
	u := newUpstream(t, func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Retry-After", "7")
		w.WriteHeader(429)
		_, _ = io.WriteString(w, `{"error":{"message":"tunggu","code":"rate_limit_exceeded"}}`)
	})
	p := testProvider(t, u.url(), nil)

	_, err := p.ChatCompletion(context.Background(), chatRequest())
	e := providers.AsError(err)
	if e == nil {
		t.Fatalf("error bukan *providers.Error: %v", err)
	}
	if e.RetryAfter != 7*time.Second {
		t.Errorf("RetryAfter = %v, mau 7s", e.RetryAfter)
	}
	if !e.Retryable() {
		t.Error("rate limit seharusnya boleh diulang")
	}
}

// Pesan error upstream sering memuat kembali potongan prompt dan kadang kredensialnya.
// Tidak satu pun boleh ikut ke *providers.Error, yang berakhir di log operator, tabel
// pemakaian, dan respons ke klien.
func TestErrorMessageOmitsUpstreamEchoAndCredential(t *testing.T) {
	const rahasia = "nomor kartu kredit pengguna 4111 1111 1111 1111"
	body := fmt.Sprintf(`{"error":{"message":"Invalid prompt: %s (key %s)","code":"invalid_prompt"}}`, rahasia, testKey)

	u := newUpstream(t, jsonHandler(400, body))
	p := testProvider(t, u.url(), nil)

	_, err := p.ChatCompletion(context.Background(), chatRequest())
	if err == nil {
		t.Fatal("status 400 tidak menghasilkan error")
	}
	got := err.Error()
	if strings.Contains(got, rahasia) {
		t.Errorf("pesan error memuat isi permintaan pengguna: %q", got)
	}
	if strings.Contains(got, testKey) {
		t.Errorf("pesan error memuat kredensial: %q", got)
	}
	if strings.Contains(got, "Invalid prompt") {
		t.Errorf("pesan error meneruskan pesan upstream apa adanya: %q", got)
	}
	// Kode upstream tetap ikut: itu token pendek yang menjelaskan penyebabnya tanpa
	// memindahkan data pengguna ke tempat baru.
	if !strings.Contains(got, "invalid_prompt") {
		t.Errorf("pesan error tidak menyebut kode upstream: %q", got)
	}
}

// Kode error upstream ikut ke pesan, log, dan label metrik, jadi panjang dan karakter
// kontrolnya harus dibatasi di satu tempat.
func TestSanitizeCode(t *testing.T) {
	if got := sanitizeCode("  insufficient_quota\n "); got != "insufficient_quota" {
		t.Errorf("sanitizeCode = %q", got)
	}
	// Baris baru di tengah kode akan memecah satu baris log menjadi dua.
	if got := sanitizeCode("kode\nsuntikan\tlog"); strings.ContainsAny(got, "\n\t") {
		t.Errorf("karakter kontrol lolos: %q", got)
	}
	if got := sanitizeCode(strings.Repeat("a", 500)); len([]rune(got)) != maxUpstreamCodeRunes {
		t.Errorf("panjang kode = %d rune, mau %d", len([]rune(got)), maxUpstreamCodeRunes)
	}
	if got := sanitizeCode("   "); got != "" {
		t.Errorf("kode kosong = %q", got)
	}
}

// Kredensial tidak boleh muncul di keluaran fmt mana pun.
//
// security.Secret sendiri tidak cukup: fmt tidak boleh memanggil metode pada field yang
// tidak diekspor, jadi tanpa String/GoString di tingkat struct, "%v" pada Provider akan
// mencetak isi mentah kunci. Ini test yang membuktikan lubang itu tertutup.
func TestProviderFormattingNeverRevealsCredential(t *testing.T) {
	p := testProvider(t, "https://api.openai.com/v1", func(c *Config) {
		c.ProxyURL = "http://user:sandi@proxy.internal:3128"
	})

	for _, format := range []string{"%v", "%+v", "%#v", "%s", "%q"} {
		for _, arg := range []any{p, *p} {
			got := fmt.Sprintf(format, arg)
			if strings.Contains(got, testKey) {
				t.Errorf("format %s membocorkan kredensial: %s", format, got)
			}
			if strings.Contains(got, "sandi") {
				t.Errorf("format %s membocorkan kredensial proxy: %s", format, got)
			}
		}
	}
	// Nama dan kind memang harus tetap terbaca, kalau tidak keluaran log jadi tak berguna.
	if got := fmt.Sprintf("%v", p); !strings.Contains(got, "uji") {
		t.Errorf("keluaran fmt tidak menyebut nama provider: %s", got)
	}
}

// Jalur slog juga harus tertutup: satu logger.Info dengan Provider sebagai atribut cukup
// untuk memindahkan kunci ke berkas log.
func TestProviderLogValueRedactsCredential(t *testing.T) {
	var buf strings.Builder
	logger := slog.New(slog.NewJSONHandler(&buf, nil))
	p := testProvider(t, "https://api.openai.com/v1", nil)

	logger.Info("provider siap", "provider", p)
	if strings.Contains(buf.String(), testKey) {
		t.Errorf("slog membocorkan kredensial: %s", buf.String())
	}
	if !strings.Contains(buf.String(), "uji") {
		t.Errorf("slog tidak mencatat nama provider: %s", buf.String())
	}
}

// Kredensial juga tidak boleh bocor lewat jalur error apa pun.
func TestCredentialNeverAppearsInAnyError(t *testing.T) {
	// Upstream yang mengembalikan kunci di badan errornya — perilaku yang benar-benar ada
	// pada pesan "Incorrect API key provided: sk-...".
	u := newUpstream(t, jsonHandler(401, fmt.Sprintf(`{"error":{"message":"Incorrect API key provided: %s","code":"invalid_api_key"}}`, testKey)))
	p := testProvider(t, u.url(), nil)
	ctx := context.Background()

	var errs []error
	if _, err := p.ChatCompletion(ctx, chatRequest()); err != nil {
		errs = append(errs, err)
	}
	if _, err := p.ChatCompletionStream(ctx, chatRequest()); err != nil {
		errs = append(errs, err)
	}
	if _, err := p.Embeddings(ctx, &providers.EmbeddingsRequest{Model: "e", Input: []string{"a"}}); err != nil {
		errs = append(errs, err)
	}
	if _, err := p.Models(ctx); err != nil {
		errs = append(errs, err)
	}
	if len(errs) != 4 {
		t.Fatalf("hanya %d dari 4 jalur yang gagal; test ini butuh keempatnya", len(errs))
	}

	health := p.HealthCheck(ctx)
	for _, err := range append(errs, fmt.Errorf("%s", health.ErrorMessage)) {
		if strings.Contains(err.Error(), testKey) {
			t.Errorf("kredensial bocor lewat error: %v", err)
		}
	}
}

// Batas waktu harus terklasifikasi sebagai timeout, dan pesannya tidak boleh memuat alamat
// upstream: pada provider BYOK itu alamat milik pengguna lain.
func TestTimeoutClassifiedWithoutLeakingAddress(t *testing.T) {
	u := newUpstream(t, func(w http.ResponseWriter, r *http.Request) {
		select {
		case <-r.Context().Done():
		case <-time.After(2 * time.Second):
		}
	})
	p := testProvider(t, u.url(), func(c *Config) { c.Timeout = 100 * time.Millisecond })

	_, err := p.ChatCompletion(context.Background(), chatRequest())
	e := providers.AsError(err)
	if e == nil {
		t.Fatalf("error bukan *providers.Error: %v", err)
	}
	if e.Kind != providers.ErrKindTimeout {
		t.Errorf("Kind = %s, mau %s", e.Kind, providers.ErrKindTimeout)
	}
	assertNoAddress(t, e.Error(), u.url())
}

// Koneksi yang ditolak adalah kegagalan jaringan, dan alamatnya juga tidak boleh ikut.
func TestConnectionRefusedClassifiedWithoutLeakingAddress(t *testing.T) {
	// Port yang baru saja dilepas: satu-satunya cara yang bisa diandalkan untuk
	// mendapatkan alamat yang pasti menolak koneksi.
	ln, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatalf("listen: %v", err)
	}
	addr := ln.Addr().String()
	if err := ln.Close(); err != nil {
		t.Fatalf("close listener: %v", err)
	}

	p := testProvider(t, "http://"+addr, nil)
	_, err = p.ChatCompletion(context.Background(), chatRequest())
	e := providers.AsError(err)
	if e == nil {
		t.Fatalf("error bukan *providers.Error: %v", err)
	}
	if e.Kind != providers.ErrKindNetwork {
		t.Errorf("Kind = %s, mau %s", e.Kind, providers.ErrKindNetwork)
	}
	if !e.Retryable() || !e.Failoverable() {
		t.Error("kegagalan jaringan sebelum ada byte terkirim seharusnya boleh diulang dan dialihkan")
	}
	assertNoAddress(t, e.Error(), "http://"+addr)
}

// Pembatalan dari klien bukan kegagalan provider dan tidak boleh diulang.
func TestCanceledContextClassifiedAsCanceled(t *testing.T) {
	u := newUpstream(t, func(w http.ResponseWriter, r *http.Request) {
		<-r.Context().Done()
	})
	p := testProvider(t, u.url(), nil)

	ctx, cancel := context.WithCancel(context.Background())
	go func() {
		time.Sleep(50 * time.Millisecond)
		cancel()
	}()

	_, err := p.ChatCompletion(ctx, chatRequest())
	e := providers.AsError(err)
	if e == nil {
		t.Fatalf("error bukan *providers.Error: %v", err)
	}
	if e.Kind != providers.ErrKindCanceled {
		t.Errorf("Kind = %s, mau %s", e.Kind, providers.ErrKindCanceled)
	}
	if e.Retryable() || e.Failoverable() {
		t.Error("permintaan yang dibatalkan klien tidak boleh diulang maupun dialihkan")
	}
}

// assertNoAddress memastikan host maupun port tidak muncul di pesan.
func assertNoAddress(t *testing.T, message, rawURL string) {
	t.Helper()
	trimmed := strings.TrimPrefix(rawURL, "http://")
	host, port, err := net.SplitHostPort(trimmed)
	if err != nil {
		t.Fatalf("SplitHostPort(%q): %v", trimmed, err)
	}
	if strings.Contains(message, host) {
		t.Errorf("pesan memuat host upstream: %q", message)
	}
	if strings.Contains(message, port) {
		t.Errorf("pesan memuat port upstream: %q", message)
	}
}

// Setiap kategori kegagalan harus punya pesan sendiri yang menyebut status: pesan itulah
// satu-satunya keterangan yang dilihat operator, karena pesan upstream tidak diteruskan.
func TestDescribeKindCoversEveryKind(t *testing.T) {
	kinds := []providers.ErrorKind{
		providers.ErrKindAuth, providers.ErrKindPermission, providers.ErrKindModelNotFound,
		providers.ErrKindRateLimit, providers.ErrKindQuota, providers.ErrKindContentFilter,
		providers.ErrKindInvalidRequest, providers.ErrKindOverloaded, providers.ErrKindServer,
		providers.ErrKindTimeout, providers.ErrKindNetwork, providers.ErrKindStreamAborted,
		providers.ErrKindCanceled, providers.ErrKindUnknown,
	}
	seen := map[string]providers.ErrorKind{}
	for _, kind := range kinds {
		msg := describeKind(kind, 503)
		if msg == "" {
			t.Errorf("%s: pesan kosong", kind)
			continue
		}
		if !strings.Contains(msg, "503") {
			t.Errorf("%s: pesan tidak menyebut status: %q", kind, msg)
		}
		// Kategori yang punya konsekuensi routing berbeda tidak boleh berbagi pesan yang
		// sama; kalau tidak, operator tidak bisa membedakannya dari log.
		if kind != providers.ErrKindNetwork && kind != providers.ErrKindStreamAborted &&
			kind != providers.ErrKindCanceled && kind != providers.ErrKindUnknown {
			if before, ada := seen[msg]; ada {
				t.Errorf("%s memakai pesan yang sama dengan %s: %q", kind, before, msg)
			}
			seen[msg] = kind
		}
	}
}

// readBody menolak body raksasa: upstream menentukan berapa memori yang dipakai gateway per
// request, dan satu provider bermasalah tidak boleh bisa menghabiskannya.
func TestReadBodyRejectsOversizedResponse(t *testing.T) {
	_, err := readBody("uji", endlessReader{})
	if err == nil {
		t.Fatal("body raksasa diterima")
	}
	if err.Kind != providers.ErrKindServer {
		t.Errorf("Kind = %s, mau %s", err.Kind, providers.ErrKindServer)
	}
}

// Koneksi yang mati saat body dibaca adalah kegagalan transport, bukan respons salah bentuk:
// bedanya menentukan apakah mesin routing boleh mengulang.
func TestReadBodyClassifiesReadFailureAsTransport(t *testing.T) {
	_, err := readBody("uji", iotest.TimeoutReader(strings.NewReader("sebagian")))
	if err == nil {
		t.Fatal("kegagalan pembacaan tidak dilaporkan")
	}
	if err.Kind != providers.ErrKindTimeout && err.Kind != providers.ErrKindNetwork {
		t.Errorf("Kind = %s, mau timeout atau network", err.Kind)
	}
}

// endlessReader selalu mengembalikan data, tanpa pernah selesai.
type endlessReader struct{}

func (endlessReader) Read(p []byte) (int, error) {
	for i := range p {
		p[i] = 'a'
	}
	return len(p), nil
}

// Galat berstatus sukses dengan kode yang tidak dikenali: ErrKindServer adalah dugaan
// terbaik, karena hanya kategori itu yang aman diulang sekaligus dialihkan.
func TestErrorWithSuccessStatusAndUnknownCodeBecomesServerError(t *testing.T) {
	u := newUpstream(t, jsonHandler(200, `{"error":{"message":"ada yang aneh","code":"kode_tak_dikenal"}}`))
	p := testProvider(t, u.url(), nil)

	_, err := p.ChatCompletion(context.Background(), chatRequest())
	e := providers.AsError(err)
	if e == nil {
		t.Fatalf("error bukan *providers.Error: %v", err)
	}
	if e.Kind != providers.ErrKindServer {
		t.Errorf("Kind = %s, mau %s", e.Kind, providers.ErrKindServer)
	}
	if e.UpstreamCode != "kode_tak_dikenal" {
		t.Errorf("UpstreamCode = %q", e.UpstreamCode)
	}
	if !e.Retryable() || !e.Failoverable() {
		t.Error("galat server seharusnya boleh diulang dan dialihkan")
	}
	if strings.Contains(e.Message, "ada yang aneh") {
		t.Errorf("pesan upstream diteruskan apa adanya: %q", e.Message)
	}
}

// Base URL yang tidak bisa dijadikan permintaan HTTP adalah salah konfigurasi, bukan
// kegagalan provider — dan pesannya tidak boleh memuat URL-nya, yang bisa berisi kredensial
// di query string.
func TestUnusableBaseURLReportedWithoutLeakingIt(t *testing.T) {
	p := testProvider(t, "http://x.test/awalan\x7f", nil)

	_, err := p.Models(context.Background())
	e := providers.AsError(err)
	if e == nil {
		t.Fatalf("error bukan *providers.Error: %v", err)
	}
	if e.Kind != providers.ErrKindInvalidRequest {
		t.Errorf("Kind = %s, mau %s", e.Kind, providers.ErrKindInvalidRequest)
	}
	if strings.Contains(e.Message, "x.test") {
		t.Errorf("pesan memuat base URL: %q", e.Message)
	}
}
