package anthropic

import (
	"context"
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"net/netip"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/NexGen-X/Route-X/internal/providers"
	"github.com/NexGen-X/Route-X/internal/security"
)

// testKey adalah API key sungguhan-mirip yang dipakai untuk membuktikan kredensial tidak
// pernah bocor ke pesan error mana pun.
const testKey = "sk-ant-api03-RAHASIA-JANGAN-BOCOR-0123456789"

// testPolicy melonggarkan penjaga SSRF supaya test bisa menghubungi httptest.Server yang
// selalu berada di 127.0.0.1 lewat http.
//
// Yang dipakai adalah pengecualian PER ALAMAT, bukan AllowPrivate. Bedanya bukan kosmetik:
// AllowPrivate melepas seluruh penjagaan rentang, sehingga test tidak akan menyadari kalau
// adapter ini mulai menghubungi alamat internal yang lain — misalnya endpoint metadata
// cloud lewat base URL yang dibelokkan. Dengan hanya 127.0.0.1 yang dikecualikan, test
// tetap bicara dengan test double-nya sendiri sementara sisa penjagaannya masih aktif.
//
// Sebelum lapisan dial ikut menghormati pengecualian ini, cara ini tidak bisa dipakai dan
// test terpaksa memakai AllowPrivate.
func testPolicy() security.SSRFPolicy {
	return security.SSRFPolicy{AllowHTTP: true, AllowedPrivateAddrs: loopbackSaja()}
}

// loopbackSaja mengembalikan pengecualian untuk kedua bentuk alamat loopback.
//
// Keduanya diperlukan karena httptest.Server bisa mendengarkan di 127.0.0.1 maupun [::1]
// tergantung tumpukan jaringan mesin yang menjalankan test.
func loopbackSaja() []netip.Prefix {
	addrs, err := security.ParsePrivateAddrs([]string{"127.0.0.1", "::1"})
	if err != nil {
		panic(err)
	}
	return addrs
}

// requestLog merekam permintaan yang diterima server tiruan. Dilindungi mutex karena
// handler berjalan di goroutine lain daripada test.
type requestLog struct {
	mu       sync.Mutex
	method   string
	path     string
	rawQuery string
	header   http.Header
	body     []byte
}

func (l *requestLog) record(r *http.Request) {
	body, _ := io.ReadAll(r.Body)
	l.mu.Lock()
	defer l.mu.Unlock()
	l.method = r.Method
	l.path = r.URL.Path
	l.rawQuery = r.URL.RawQuery
	l.header = r.Header.Clone()
	l.body = body
}

func (l *requestLog) Method() string { l.mu.Lock(); defer l.mu.Unlock(); return l.method }
func (l *requestLog) Path() string   { l.mu.Lock(); defer l.mu.Unlock(); return l.path }
func (l *requestLog) Query() string  { l.mu.Lock(); defer l.mu.Unlock(); return l.rawQuery }
func (l *requestLog) Header(name string) string {
	l.mu.Lock()
	defer l.mu.Unlock()
	return l.header.Get(name)
}
func (l *requestLog) Body() []byte { l.mu.Lock(); defer l.mu.Unlock(); return l.body }

// newServer membuat server tiruan berdialek Anthropic beserta provider yang menunjuk ke sana.
func newServer(t *testing.T, handler func(w http.ResponseWriter, r *http.Request)) (*Provider, *requestLog) {
	t.Helper()
	log := &requestLog{}
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		log.record(r)
		handler(w, r)
	}))
	t.Cleanup(srv.Close)

	p, err := New(Config{
		Name:       "anthropic-uji",
		BaseURL:    srv.URL,
		Credential: security.Secret(testKey),
		SSRFPolicy: testPolicy(),
		Timeout:    5 * time.Second,
	})
	if err != nil {
		t.Fatalf("New() error: %v", err)
	}
	return p, log
}

// jsonHandler membalas satu body JSON tetap.
func jsonHandler(status int, body string) func(http.ResponseWriter, *http.Request) {
	return func(w http.ResponseWriter, _ *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(status)
		_, _ = io.WriteString(w, body)
	}
}

// wire* adalah bentuk kawat Anthropic yang ditulis ulang di test, bukan struct produksi:
// dengan begitu tag JSON yang salah ikut tertangkap.
type wireRequest struct {
	Model         string          `json:"model"`
	MaxTokens     int             `json:"max_tokens"`
	System        string          `json:"system"`
	Stream        bool            `json:"stream"`
	Temperature   *float64        `json:"temperature"`
	TopP          *float64        `json:"top_p"`
	StopSequences []string        `json:"stop_sequences"`
	Messages      []wireMessage   `json:"messages"`
	Tools         []wireTool      `json:"tools"`
	ToolChoice    map[string]any  `json:"tool_choice"`
	Thinking      map[string]any  `json:"thinking"`
	Metadata      map[string]any  `json:"metadata"`
	Extra         json.RawMessage `json:"seed"`
}

type wireMessage struct {
	Role    string      `json:"role"`
	Content []wireBlock `json:"content"`
}

type wireBlock struct {
	Type      string            `json:"type"`
	Text      string            `json:"text"`
	ID        string            `json:"id"`
	Name      string            `json:"name"`
	Input     json.RawMessage   `json:"input"`
	ToolUseID string            `json:"tool_use_id"`
	Content   []wireBlock       `json:"content"`
	Source    map[string]string `json:"source"`
	Thinking  string            `json:"thinking"`
}

type wireTool struct {
	Name        string         `json:"name"`
	Description string         `json:"description"`
	InputSchema map[string]any `json:"input_schema"`
}

func decodeRequest(t *testing.T, raw []byte) wireRequest {
	t.Helper()
	var out wireRequest
	if err := json.Unmarshal(raw, &out); err != nil {
		t.Fatalf("body permintaan tidak bisa diurai: %v\nbody: %s", err, raw)
	}
	return out
}

func simpleRequest() *providers.ChatRequest {
	return &providers.ChatRequest{
		Model:    "claude-sonnet-4-5",
		Messages: []providers.Message{{Role: providers.RoleUser, Content: "halo"}},
	}
}

const textResponse = `{
  "id": "msg_01",
  "type": "message",
  "role": "assistant",
  "model": "claude-sonnet-4-5-20250929",
  "content": [{"type": "text", "text": "halo juga"}],
  "stop_reason": "end_turn",
  "usage": {"input_tokens": 10, "output_tokens": 5}
}`

func TestNewMenerapkanNilaiBawaan(t *testing.T) {
	p, err := New(Config{Credential: security.Secret(testKey)})
	if err != nil {
		t.Fatalf("New() error: %v", err)
	}
	if p.Kind() != providers.KindAnthropic {
		t.Errorf("Kind() = %q, mau %q", p.Kind(), providers.KindAnthropic)
	}
	if p.Name() != providers.KindAnthropic {
		t.Errorf("Name() = %q, mau %q", p.Name(), providers.KindAnthropic)
	}
	if p.baseURL != DefaultBaseURL {
		t.Errorf("baseURL = %q, mau %q", p.baseURL, DefaultBaseURL)
	}
	if p.apiVersion != DefaultAPIVersion {
		t.Errorf("apiVersion = %q, mau %q", p.apiVersion, DefaultAPIVersion)
	}
	if p.maxTokens != DefaultMaxTokens {
		t.Errorf("maxTokens = %d, mau %d", p.maxTokens, DefaultMaxTokens)
	}
}

func TestNewMenghormatiNilaiEksplisit(t *testing.T) {
	p, err := New(Config{
		Name:             "byok-1",
		Kind:             providers.KindCustom,
		BaseURL:          "https://contoh.test/v1/",
		Credential:       security.Secret(testKey),
		APIVersion:       "2099-01-01",
		DefaultMaxTokens: 128,
		ExtraHeaders:     map[string]string{"": "diabaikan", "anthropic-beta": "fitur-uji"},
	})
	if err != nil {
		t.Fatalf("New() error: %v", err)
	}
	if p.Name() != "byok-1" || p.Kind() != providers.KindCustom {
		t.Errorf("Name()/Kind() = %q/%q", p.Name(), p.Kind())
	}
	if p.apiVersion != "2099-01-01" || p.maxTokens != 128 {
		t.Errorf("apiVersion/maxTokens = %q/%d", p.apiVersion, p.maxTokens)
	}
	if _, ada := p.headers[""]; ada {
		t.Error("header dengan nama kosong seharusnya dibuang")
	}
	if p.headers["anthropic-beta"] != "fitur-uji" {
		t.Errorf("ExtraHeaders tidak tersimpan: %v", p.headers)
	}
}

func TestNewMenolakKonfigurasiTidakSah(t *testing.T) {
	tests := []struct {
		nama string
		cfg  Config
	}{
		{"kredensial kosong", Config{}},
		{"skema http tanpa izin", Config{BaseURL: "http://contoh.test", Credential: security.Secret(testKey)}},
		{"skema tidak didukung", Config{BaseURL: "ftp://contoh.test", Credential: security.Secret(testKey)}},
		{"proxy tidak didukung", Config{Credential: security.Secret(testKey), ProxyURL: security.Secret("ftp://proxy.test")}},
	}
	for _, tc := range tests {
		t.Run(tc.nama, func(t *testing.T) {
			if _, err := New(tc.cfg); err == nil {
				t.Fatal("New() berhasil, mau error")
			}
		})
	}
}

func TestHeaderAutentikasiTerkirimDenganNamaTepat(t *testing.T) {
	p, log := newServer(t, jsonHandler(http.StatusOK, textResponse))
	if _, err := p.ChatCompletion(context.Background(), simpleRequest()); err != nil {
		t.Fatalf("ChatCompletion() error: %v", err)
	}

	if got := log.Header(headerAPIKey); got != testKey {
		t.Errorf("header %s = %q, mau kredensial terkirim utuh", headerAPIKey, got)
	}
	if got := log.Header(headerVersion); got != DefaultAPIVersion {
		t.Errorf("header %s = %q, mau %q", headerVersion, got, DefaultAPIVersion)
	}
	if got := log.Header("Content-Type"); got != "application/json" {
		t.Errorf("Content-Type = %q", got)
	}
	// Anthropic memakai x-api-key, bukan Authorization: Bearer.
	if got := log.Header("Authorization"); got != "" {
		t.Errorf("Authorization terkirim (%q); Anthropic hanya memakai x-api-key", got)
	}
	if log.Path() != "/v1/messages" {
		t.Errorf("path = %q, mau /v1/messages", log.Path())
	}
}

func TestExtraHeadersTidakBisaMenimpaAutentikasi(t *testing.T) {
	log := &requestLog{}
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		log.record(r)
		jsonHandler(http.StatusOK, textResponse)(w, r)
	}))
	t.Cleanup(srv.Close)

	p, err := New(Config{
		BaseURL:    srv.URL,
		Credential: security.Secret(testKey),
		SSRFPolicy: testPolicy(),
		ExtraHeaders: map[string]string{
			headerAPIKey:     "kunci-palsu",
			headerVersion:    "1999-01-01",
			"anthropic-beta": "fitur-uji",
		},
	})
	if err != nil {
		t.Fatalf("New() error: %v", err)
	}
	if _, err := p.ChatCompletion(context.Background(), simpleRequest()); err != nil {
		t.Fatalf("ChatCompletion() error: %v", err)
	}

	if got := log.Header(headerAPIKey); got != testKey {
		t.Errorf("header %s = %q; ExtraHeaders tidak boleh menimpa kredensial", headerAPIKey, got)
	}
	if got := log.Header(headerVersion); got != DefaultAPIVersion {
		t.Errorf("header %s = %q; ExtraHeaders tidak boleh menimpa versi API", headerVersion, got)
	}
	if got := log.Header("anthropic-beta"); got != "fitur-uji" {
		t.Errorf("header tambahan hilang: %q", got)
	}
}

func TestEmbeddingsDitolakDenganPesanJelas(t *testing.T) {
	p, err := New(Config{Credential: security.Secret(testKey)})
	if err != nil {
		t.Fatalf("New() error: %v", err)
	}
	resp, err := p.Embeddings(context.Background(), &providers.EmbeddingsRequest{
		Model: "claude-sonnet-4-5", Input: []string{"halo"},
	})
	if resp != nil {
		t.Error("respons tidak nil; embeddings seharusnya gagal, bukan mengembalikan nol")
	}
	e := providers.AsError(err)
	if e == nil {
		t.Fatalf("error = %v, mau *providers.Error", err)
	}
	if e.Kind != providers.ErrKindInvalidRequest {
		t.Errorf("Kind = %q, mau %q", e.Kind, providers.ErrKindInvalidRequest)
	}
	if !strings.Contains(e.Message, "embeddings") {
		t.Errorf("pesan tidak menyebut embeddings: %q", e.Message)
	}
	if e.Failoverable() {
		t.Error("Failoverable() = true; salah pemetaan model tidak boleh disembunyikan lewat failover")
	}
}

func TestModelsMemetakanDaftarModel(t *testing.T) {
	body := `{"data":[
	  {"type":"model","id":"claude-sonnet-4-5","created_at":"2025-09-29T00:00:00Z"},
	  {"type":"model","id":"claude-opus-4-1","created_at":"bukan-tanggal"},
	  {"type":"model","id":""}
	],"has_more":false}`
	p, log := newServer(t, jsonHandler(http.StatusOK, body))

	models, err := p.Models(context.Background())
	if err != nil {
		t.Fatalf("Models() error: %v", err)
	}
	if log.Method() != http.MethodGet || log.Path() != "/v1/models" {
		t.Errorf("permintaan = %s %s, mau GET /v1/models", log.Method(), log.Path())
	}
	if log.Query() != "limit=1000" {
		t.Errorf("query = %q, mau limit=1000", log.Query())
	}
	if len(models) != 2 {
		t.Fatalf("jumlah model = %d, mau 2 (entri tanpa id dibuang)", len(models))
	}
	if models[0].ID != "claude-sonnet-4-5" || models[0].OwnedBy != providers.KindAnthropic {
		t.Errorf("model[0] = %+v", models[0])
	}
	if models[0].Created == 0 {
		t.Error("created_at RFC3339 tidak terurai")
	}
	if models[1].Created != 0 {
		t.Errorf("created_at tidak sah seharusnya menjadi 0, dapat %d", models[1].Created)
	}
}

func TestModelsMelaporkanBodyRusak(t *testing.T) {
	p, _ := newServer(t, jsonHandler(http.StatusOK, `{"data": bukan-json`))
	if _, err := p.Models(context.Background()); err == nil {
		t.Fatal("Models() berhasil, mau error")
	}
}

func TestHealthCheckMemakaiDaftarModel(t *testing.T) {
	p, log := newServer(t, jsonHandler(http.StatusOK, `{"data":[]}`))

	res := p.HealthCheck(context.Background())
	if !res.Healthy {
		t.Fatalf("Healthy = false: %+v", res)
	}
	if res.StatusCode != http.StatusOK {
		t.Errorf("StatusCode = %d", res.StatusCode)
	}
	if res.Latency <= 0 {
		t.Error("Latency = 0, mau terukur")
	}
	if log.Path() != "/v1/models" || log.Query() != "limit=1" {
		t.Errorf("probe = %s?%s, mau /v1/models?limit=1", log.Path(), log.Query())
	}
}

func TestHealthCheckMelaporkanKegagalan(t *testing.T) {
	p, _ := newServer(t, jsonHandler(http.StatusUnauthorized,
		`{"type":"error","error":{"type":"authentication_error","message":"invalid x-api-key"}}`))

	res := p.HealthCheck(context.Background())
	if res.Healthy {
		t.Fatal("Healthy = true, mau false")
	}
	if res.ErrorKind != providers.ErrKindAuth {
		t.Errorf("ErrorKind = %q, mau %q", res.ErrorKind, providers.ErrKindAuth)
	}
	if res.StatusCode != http.StatusUnauthorized {
		t.Errorf("StatusCode = %d", res.StatusCode)
	}
	if res.ErrorMessage == "" {
		t.Error("ErrorMessage kosong")
	}
}

func TestHealthCheckMelaporkanKegagalanTransport(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(http.ResponseWriter, *http.Request) {}))
	base := srv.URL
	srv.Close() // tidak ada lagi yang mendengarkan di alamat ini

	p, err := New(Config{BaseURL: base, Credential: security.Secret(testKey), SSRFPolicy: testPolicy()})
	if err != nil {
		t.Fatalf("New() error: %v", err)
	}
	res := p.HealthCheck(context.Background())
	if res.Healthy {
		t.Fatal("Healthy = true, mau false")
	}
	if res.ErrorKind != providers.ErrKindNetwork {
		t.Errorf("ErrorKind = %q, mau %q", res.ErrorKind, providers.ErrKindNetwork)
	}
}

func TestPemetaanStatusError(t *testing.T) {
	tests := []struct {
		nama   string
		status int
		body   string
		kind   providers.ErrorKind
	}{
		{"auth", http.StatusUnauthorized, `{"error":{"type":"authentication_error","message":"key ditolak"}}`, providers.ErrKindAuth},
		{"permission", http.StatusForbidden, `{"error":{"type":"permission_error","message":"tidak berhak"}}`, providers.ErrKindPermission},
		{"model tidak ada", http.StatusNotFound, `{"error":{"type":"not_found_error","message":"model tidak dikenal"}}`, providers.ErrKindModelNotFound},
		{"permintaan cacat", http.StatusBadRequest, `{"error":{"type":"invalid_request_error","message":"messages: field wajib"}}`, providers.ErrKindInvalidRequest},
		{"rate limit", http.StatusTooManyRequests, `{"error":{"type":"rate_limit_error","message":"terlalu cepat"}}`, providers.ErrKindRateLimit},
		{"overloaded", 529, `{"error":{"type":"overloaded_error","message":"sedang penuh"}}`, providers.ErrKindOverloaded},
		{"server", http.StatusInternalServerError, `{"error":{"type":"api_error","message":"gagal internal"}}`, providers.ErrKindServer},
		{"body tanpa bentuk error", http.StatusInternalServerError, `bukan json`, providers.ErrKindServer},
		// Saldo habis dilaporkan Anthropic sebagai 400 invalid_request_error; tanpa
		// pengecualian ini kegagalan yang seharusnya failover tidak dialihkan ke mana pun.
		{"saldo habis", http.StatusBadRequest,
			`{"error":{"type":"invalid_request_error","message":"Your credit balance is too low to access the API"}}`,
			providers.ErrKindQuota},
	}

	for _, tc := range tests {
		t.Run(tc.nama, func(t *testing.T) {
			p, _ := newServer(t, jsonHandler(tc.status, tc.body))
			_, err := p.ChatCompletion(context.Background(), simpleRequest())
			e := providers.AsError(err)
			if e == nil {
				t.Fatalf("error = %v, mau *providers.Error", err)
			}
			if e.Kind != tc.kind {
				t.Errorf("Kind = %q, mau %q", e.Kind, tc.kind)
			}
			if e.StatusCode != tc.status {
				t.Errorf("StatusCode = %d, mau %d", e.StatusCode, tc.status)
			}
			if e.Message == "" {
				t.Error("Message kosong; pemanggil butuh keterangan yang bisa dicatat")
			}
		})
	}
}

func TestRetryAfterDibaca(t *testing.T) {
	p, _ := newServer(t, func(w http.ResponseWriter, _ *http.Request) {
		w.Header().Set("Retry-After", "7")
		w.WriteHeader(http.StatusTooManyRequests)
		_, _ = io.WriteString(w, `{"error":{"type":"rate_limit_error","message":"tunggu"}}`)
	})
	_, err := p.ChatCompletion(context.Background(), simpleRequest())
	e := providers.AsError(err)
	if e == nil {
		t.Fatalf("error = %v", err)
	}
	if e.RetryAfter != 7*time.Second {
		t.Errorf("RetryAfter = %v, mau 7s", e.RetryAfter)
	}
	if !e.Retryable() || !e.Failoverable() {
		t.Error("rate limit seharusnya boleh diulang dan dialihkan")
	}
}

func TestResponsRusakDilaporkanSebagaiKegagalanServer(t *testing.T) {
	p, _ := newServer(t, jsonHandler(http.StatusOK, `{"id": bukan-json`))
	_, err := p.ChatCompletion(context.Background(), simpleRequest())
	e := providers.AsError(err)
	if e == nil {
		t.Fatalf("error = %v", err)
	}
	if e.Kind != providers.ErrKindServer {
		t.Errorf("Kind = %q, mau %q", e.Kind, providers.ErrKindServer)
	}
}

func TestPembatalanContextDiklasifikasiTepat(t *testing.T) {
	p, _ := newServer(t, jsonHandler(http.StatusOK, textResponse))
	ctx, cancel := context.WithCancel(context.Background())
	cancel()

	_, err := p.ChatCompletion(ctx, simpleRequest())
	e := providers.AsError(err)
	if e == nil {
		t.Fatalf("error = %v, mau *providers.Error", err)
	}
	if e.Kind != providers.ErrKindCanceled {
		t.Errorf("Kind = %q, mau %q", e.Kind, providers.ErrKindCanceled)
	}
	if e.Retryable() {
		t.Error("permintaan yang dibatalkan klien tidak boleh diulang")
	}
}

// Kredensial tidak boleh muncul di pesan error mana pun, termasuk ketika upstream sendiri
// memantulkannya kembali di badan errornya.
func TestKredensialTidakPernahBocorKePesanError(t *testing.T) {
	echo := `{"error":{"type":"authentication_error","message":"invalid x-api-key: ` + testKey + ` (periksa dashboard)"}}`

	t.Run("chat completion", func(t *testing.T) {
		p, _ := newServer(t, jsonHandler(http.StatusUnauthorized, echo))
		_, err := p.ChatCompletion(context.Background(), simpleRequest())
		if err == nil {
			t.Fatal("mau error")
		}
		assertTanpaKredensial(t, err.Error())
		if e := providers.AsError(err); e != nil {
			assertTanpaKredensial(t, e.Message)
			if !strings.Contains(e.Message, redactedMark) {
				t.Errorf("kredensial tidak diganti penanda tersamar: %q", e.Message)
			}
		}
	})

	t.Run("health check", func(t *testing.T) {
		p, _ := newServer(t, jsonHandler(http.StatusUnauthorized, echo))
		res := p.HealthCheck(context.Background())
		assertTanpaKredensial(t, res.ErrorMessage)
	})

	t.Run("models", func(t *testing.T) {
		p, _ := newServer(t, jsonHandler(http.StatusForbidden, echo))
		_, err := p.Models(context.Background())
		if err == nil {
			t.Fatal("mau error")
		}
		assertTanpaKredensial(t, err.Error())
	})

	t.Run("aliran", func(t *testing.T) {
		p, _ := newServer(t, sseHandler(
			"event: message_start\ndata: {\"type\":\"message_start\",\"message\":{\"id\":\"msg_1\",\"usage\":{\"input_tokens\":3}}}\n\n",
			"event: error\ndata: {\"type\":\"error\",\"error\":{\"type\":\"authentication_error\",\"message\":\"key "+testKey+" ditolak\"}}\n\n",
		))
		st, err := p.ChatCompletionStream(context.Background(), simpleRequest())
		if err != nil {
			t.Fatalf("ChatCompletionStream() error: %v", err)
		}
		defer st.Close()
		var last error
		for {
			_, err := st.Recv()
			if err != nil {
				last = err
				break
			}
		}
		if last == nil {
			t.Fatal("mau error dari aliran")
		}
		assertTanpaKredensial(t, last.Error())
	})

	t.Run("secret tidak tercetak", func(t *testing.T) {
		s := security.Secret(testKey)
		assertTanpaKredensial(t, s.String())
	})
}

func assertTanpaKredensial(t *testing.T, s string) {
	t.Helper()
	if strings.Contains(s, testKey) {
		t.Fatalf("kredensial bocor: %q", s)
	}
	// Potongan panjang key pun tidak boleh muncul.
	if strings.Contains(s, "RAHASIA-JANGAN-BOCOR") {
		t.Fatalf("potongan kredensial bocor: %q", s)
	}
}

func TestProviderMemenuhiKontrak(t *testing.T) {
	var _ providers.Provider = (*Provider)(nil)

	p, err := New(Config{Credential: security.Secret(testKey)})
	if err != nil {
		t.Fatalf("New() error: %v", err)
	}
	var iface providers.Provider = p
	if iface.Kind() != providers.KindAnthropic {
		t.Errorf("Kind() = %q", iface.Kind())
	}
}
