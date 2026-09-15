package google

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
// pernah bocor ke pesan error maupun ke query string.
const testKey = "AIzaSyRAHASIA-JANGAN-BOCOR-0123456789"

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

// requestLog merekam permintaan yang diterima server tiruan.
type requestLog struct {
	mu       sync.Mutex
	method   string
	path     string
	rawQuery string
	rawURI   string
	header   http.Header
	bodies   [][]byte
}

func (l *requestLog) record(r *http.Request) {
	body, _ := io.ReadAll(r.Body)
	l.mu.Lock()
	defer l.mu.Unlock()
	l.method = r.Method
	l.path = r.URL.Path
	l.rawQuery = r.URL.RawQuery
	l.rawURI = r.RequestURI
	l.header = r.Header.Clone()
	l.bodies = append(l.bodies, body)
}

func (l *requestLog) Method() string { l.mu.Lock(); defer l.mu.Unlock(); return l.method }
func (l *requestLog) Path() string   { l.mu.Lock(); defer l.mu.Unlock(); return l.path }
func (l *requestLog) Query() string  { l.mu.Lock(); defer l.mu.Unlock(); return l.rawQuery }
func (l *requestLog) URI() string    { l.mu.Lock(); defer l.mu.Unlock(); return l.rawURI }
func (l *requestLog) Header(name string) string {
	l.mu.Lock()
	defer l.mu.Unlock()
	return l.header.Get(name)
}

func (l *requestLog) count() int { l.mu.Lock(); defer l.mu.Unlock(); return len(l.bodies) }

func (l *requestLog) raw(t *testing.T, i int) []byte {
	t.Helper()
	l.mu.Lock()
	defer l.mu.Unlock()
	if i >= len(l.bodies) {
		t.Fatalf("permintaan ke-%d belum pernah masuk (baru %d permintaan)", i, len(l.bodies))
	}
	return l.bodies[i]
}

// newServer membuat server tiruan berdialek Gemini beserta provider yang menunjuk ke sana.
func newServer(t *testing.T, handler func(w http.ResponseWriter, r *http.Request)) (*Provider, *requestLog) {
	t.Helper()
	log := &requestLog{}
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		log.record(r)
		handler(w, r)
	}))
	t.Cleanup(srv.Close)

	p, err := New(Config{
		Name:       "google-uji",
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

// newScriptedServer membalas urutan body JSON sesuai urutan permintaan yang masuk.
func newScriptedServer(t *testing.T, responses ...string) (*Provider, *requestLog) {
	t.Helper()
	var mu sync.Mutex
	next := 0
	return newServer(t, func(w http.ResponseWriter, _ *http.Request) {
		mu.Lock()
		idx := next
		next++
		mu.Unlock()
		if idx >= len(responses) {
			idx = len(responses) - 1
		}
		w.Header().Set("Content-Type", "application/json")
		_, _ = io.WriteString(w, responses[idx])
	})
}

func jsonHandler(status int, body string) func(http.ResponseWriter, *http.Request) {
	return func(w http.ResponseWriter, _ *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(status)
		_, _ = io.WriteString(w, body)
	}
}

func simpleRequest() *providers.ChatRequest {
	return &providers.ChatRequest{
		Model:    "gemini-2.5-flash",
		Messages: []providers.Message{{Role: providers.RoleUser, Content: "halo"}},
	}
}

const textResponse = `{
  "candidates": [{
    "content": {"role": "model", "parts": [{"text": "halo juga"}]},
    "finishReason": "STOP",
    "index": 0
  }],
  "usageMetadata": {"promptTokenCount": 10, "candidatesTokenCount": 5, "totalTokenCount": 15},
  "modelVersion": "gemini-2.5-flash-001",
  "responseId": "resp-1"
}`

func ptrFloat(v float64) *float64 { return &v }
func ptrInt(v int) *int           { return &v }

func TestNewMenerapkanNilaiBawaan(t *testing.T) {
	p, err := New(Config{Credential: security.Secret(testKey)})
	if err != nil {
		t.Fatalf("New() error: %v", err)
	}
	if p.Kind() != providers.KindGoogle || p.Name() != providers.KindGoogle {
		t.Errorf("Kind()/Name() = %q/%q", p.Kind(), p.Name())
	}
	if p.baseURL != DefaultBaseURL {
		t.Errorf("baseURL = %q, mau %q", p.baseURL, DefaultBaseURL)
	}
}

func TestNewMenghormatiNilaiEksplisit(t *testing.T) {
	p, err := New(Config{
		Name:         "byok-gemini",
		Kind:         providers.KindCustom,
		BaseURL:      "https://contoh.test/v1beta/",
		Credential:   security.Secret(testKey),
		ExtraHeaders: map[string]string{"": "diabaikan", "x-uji": "ya"},
	})
	if err != nil {
		t.Fatalf("New() error: %v", err)
	}
	if p.Name() != "byok-gemini" || p.Kind() != providers.KindCustom {
		t.Errorf("Name()/Kind() = %q/%q", p.Name(), p.Kind())
	}
	if _, ada := p.headers[""]; ada {
		t.Error("header dengan nama kosong seharusnya dibuang")
	}
	// Base URL yang sudah memuat /v1beta tidak boleh menghasilkan /v1beta/v1beta.
	if got := p.endpoint("/v1beta/models"); got != "https://contoh.test/v1beta/models" {
		t.Errorf("endpoint = %q", got)
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

// Kredensial Gemini WAJIB lewat header: query string tercatat di log akses, dan pada
// gateway BYOK itu berarti key milik pengguna tersimpan di berkas log operator.
func TestKredensialLewatHeaderBukanQueryString(t *testing.T) {
	p, log := newScriptedServer(t, textResponse)
	if _, err := p.ChatCompletion(context.Background(), simpleRequest()); err != nil {
		t.Fatalf("ChatCompletion() error: %v", err)
	}

	if got := log.Header(headerAPIKey); got != testKey {
		t.Errorf("header %s = %q, mau kredensial terkirim utuh", headerAPIKey, got)
	}
	if strings.Contains(log.URI(), testKey) {
		t.Errorf("kredensial muncul di URL: %q", log.URI())
	}
	if strings.Contains(log.Query(), "key=") {
		t.Errorf("query string memuat parameter key: %q", log.Query())
	}
	if got := log.Header("Authorization"); got != "" {
		t.Errorf("Authorization terkirim (%q); Gemini memakai x-goog-api-key", got)
	}
	if got := log.Header("Content-Type"); got != "application/json" {
		t.Errorf("Content-Type = %q", got)
	}
}

func TestKredensialOAuthBearerGoogle(t *testing.T) {
	log := &requestLog{}
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		log.record(r)
		jsonHandler(http.StatusOK, textResponse)(w, r)
	}))
	defer srv.Close()

	oauthToken := "ya29.a0AfH6SMA-sample-token-12345"
	p, err := New(Config{
		Name:       "gemini-oauth",
		Kind:       providers.KindGoogle,
		BaseURL:    srv.URL,
		Credential: security.Secret(oauthToken),
		SSRFPolicy: testPolicy(),
	})
	if err != nil {
		t.Fatalf("New: %v", err)
	}

	if _, err := p.ChatCompletion(context.Background(), simpleRequest()); err != nil {
		t.Fatalf("ChatCompletion: %v", err)
	}

	if got := log.Header("Authorization"); got != "Bearer "+oauthToken {
		t.Errorf("Authorization = %q, mau 'Bearer %s'", got, oauthToken)
	}
	if got := log.Header(headerAPIKey); got != "" {
		t.Errorf("x-goog-api-key tidak boleh terkirim untuk token OAuth, dapat: %q", got)
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
		BaseURL:      srv.URL,
		Credential:   security.Secret(testKey),
		SSRFPolicy:   testPolicy(),
		ExtraHeaders: map[string]string{headerAPIKey: "kunci-palsu", "x-uji": "ya"},
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
	if got := log.Header("x-uji"); got != "ya" {
		t.Errorf("header tambahan hilang: %q", got)
	}
}

// Nama model masuk ke PATH URL, jadi bentuk dan escape-nya harus benar.
func TestNamaModelMasukKePathDenganEscapeBenar(t *testing.T) {
	tests := []struct {
		nama    string
		model   string
		mauPath string
	}{
		{"nama pendek", "gemini-2.5-flash", "/v1beta/models/gemini-2.5-flash:generateContent"},
		{"nama resource lengkap", "models/gemini-2.5-pro", "/v1beta/models/gemini-2.5-pro:generateContent"},
		{"model hasil tuning", "tunedModels/kode-abc123", "/v1beta/tunedModels/kode-abc123:generateContent"},
		{"garis miring di awal dibersihkan", "/models/gemini-2.5-flash/", "/v1beta/models/gemini-2.5-flash:generateContent"},
		{"spasi di-escape", "gemini 2.5", "/v1beta/models/gemini 2.5:generateContent"},
	}
	for _, tc := range tests {
		t.Run(tc.nama, func(t *testing.T) {
			p, log := newScriptedServer(t, textResponse)
			req := simpleRequest()
			req.Model = tc.model
			if _, err := p.ChatCompletion(context.Background(), req); err != nil {
				t.Fatalf("ChatCompletion() error: %v", err)
			}
			if got := log.Path(); got != tc.mauPath {
				t.Errorf("path = %q, mau %q", got, tc.mauPath)
			}
			// Nama model tidak boleh menyelundup ke body: Gemini menolak field asing.
			if strings.Contains(string(log.raw(t, 0)), `"model"`) {
				t.Errorf("nama model ikut di body: %s", log.raw(t, 0))
			}
		})
	}

	t.Run("spasi terkirim sebagai persen-encoding", func(t *testing.T) {
		p, log := newScriptedServer(t, textResponse)
		req := simpleRequest()
		req.Model = "gemini 2.5"
		if _, err := p.ChatCompletion(context.Background(), req); err != nil {
			t.Fatalf("ChatCompletion() error: %v", err)
		}
		if !strings.Contains(log.URI(), "gemini%202.5:generateContent") {
			t.Errorf("URI = %q, mau spasi ter-escape", log.URI())
		}
	})
}

func TestNamaModelTidakSahDitolak(t *testing.T) {
	tests := []struct {
		nama  string
		model string
	}{
		{"kosong", ""},
		{"hanya garis miring", "///"},
		{"memuat titik dua", "gemini:streamGenerateContent"},
		{"memuat tanda tanya", "gemini?alt=sse"},
		{"memuat pagar", "gemini#x"},
		{"segmen kosong", "models//gemini"},
		{"segmen naik direktori", "models/../models/gemini"},
	}
	for _, tc := range tests {
		t.Run(tc.nama, func(t *testing.T) {
			p, log := newScriptedServer(t, textResponse)
			req := simpleRequest()
			req.Model = tc.model
			_, err := p.ChatCompletion(context.Background(), req)
			e := providers.AsError(err)
			if e == nil {
				t.Fatalf("error = %v, mau *providers.Error", err)
			}
			if e.Kind != providers.ErrKindInvalidRequest {
				t.Errorf("Kind = %q, mau %q", e.Kind, providers.ErrKindInvalidRequest)
			}
			if log.count() != 0 {
				t.Error("permintaan dengan nama model cacat tetap dikirim")
			}
		})
	}
}

func TestModelResourceName(t *testing.T) {
	tests := map[string]string{
		"gemini-2.5-flash":        "models/gemini-2.5-flash",
		"models/gemini-2.5-flash": "models/gemini-2.5-flash",
		"tunedModels/abc":         "tunedModels/abc",
		"/models/x/":              "models/x",
	}
	for in, mau := range tests {
		if got := modelResourceName(in); got != mau {
			t.Errorf("modelResourceName(%q) = %q, mau %q", in, got, mau)
		}
	}
}

func TestEmbeddingsSatuMasukan(t *testing.T) {
	body := `{"embedding":{"values":[0.1,0.2,0.3]}}`
	p, log := newScriptedServer(t, body)

	resp, err := p.Embeddings(context.Background(), &providers.EmbeddingsRequest{
		Model:      "text-embedding-004",
		Input:      []string{"halo dunia"},
		Dimensions: ptrInt(3),
	})
	if err != nil {
		t.Fatalf("Embeddings() error: %v", err)
	}
	if log.Path() != "/v1beta/models/text-embedding-004:embedContent" {
		t.Errorf("path = %q", log.Path())
	}

	var sent struct {
		Content struct {
			Parts []struct {
				Text string `json:"text"`
			} `json:"parts"`
		} `json:"content"`
		OutputDimensionality *int `json:"outputDimensionality"`
	}
	if err := json.Unmarshal(log.raw(t, 0), &sent); err != nil {
		t.Fatalf("body tidak bisa diurai: %v", err)
	}
	if len(sent.Content.Parts) != 1 || sent.Content.Parts[0].Text != "halo dunia" {
		t.Errorf("content = %+v", sent.Content)
	}
	if sent.OutputDimensionality == nil || *sent.OutputDimensionality != 3 {
		t.Errorf("outputDimensionality = %v", sent.OutputDimensionality)
	}

	if len(resp.Data) != 1 || len(resp.Data[0].Vector) != 3 || resp.Data[0].Vector[0] != 0.1 {
		t.Errorf("data = %+v", resp.Data)
	}
	if resp.Model != "text-embedding-004" {
		t.Errorf("Model = %q", resp.Model)
	}

	var raw struct {
		Object string `json:"object"`
		Data   []struct {
			Object    string    `json:"object"`
			Index     int       `json:"index"`
			Embedding []float32 `json:"embedding"`
		} `json:"data"`
		Model string `json:"model"`
	}
	if err := json.Unmarshal(resp.Raw, &raw); err != nil {
		t.Fatalf("Raw bukan dialek OpenAI: %v", err)
	}
	if raw.Object != "list" || len(raw.Data) != 1 || raw.Data[0].Object != "embedding" {
		t.Errorf("Raw = %s", resp.Raw)
	}
	if len(raw.Data[0].Embedding) != 3 {
		t.Errorf("vektor di Raw = %v", raw.Data[0].Embedding)
	}
}

func TestEmbeddingsBanyakMasukanMemakaiBatch(t *testing.T) {
	body := `{"embeddings":[{"values":[1,2]},{"values":[3,4]}]}`
	p, log := newScriptedServer(t, body)

	resp, err := p.Embeddings(context.Background(), &providers.EmbeddingsRequest{
		Model: "text-embedding-004",
		Input: []string{"satu", "dua"},
	})
	if err != nil {
		t.Fatalf("Embeddings() error: %v", err)
	}
	if log.Path() != "/v1beta/models/text-embedding-004:batchEmbedContents" {
		t.Errorf("path = %q, mau :batchEmbedContents", log.Path())
	}

	var sent struct {
		Requests []struct {
			Model   string `json:"model"`
			Content struct {
				Parts []struct {
					Text string `json:"text"`
				} `json:"parts"`
			} `json:"content"`
		} `json:"requests"`
	}
	if err := json.Unmarshal(log.raw(t, 0), &sent); err != nil {
		t.Fatalf("body tidak bisa diurai: %v", err)
	}
	if len(sent.Requests) != 2 {
		t.Fatalf("jumlah entri = %d, mau 2", len(sent.Requests))
	}
	// batchEmbedContents menuntut setiap entri menyebut nama resource model-nya sendiri.
	if sent.Requests[0].Model != "models/text-embedding-004" {
		t.Errorf("requests[0].model = %q", sent.Requests[0].Model)
	}
	if sent.Requests[1].Content.Parts[0].Text != "dua" {
		t.Errorf("requests[1] = %+v", sent.Requests[1])
	}

	if len(resp.Data) != 2 || resp.Data[1].Index != 1 || resp.Data[1].Vector[1] != 4 {
		t.Errorf("data = %+v", resp.Data)
	}
}

func TestEmbeddingsTanpaMasukanDitolak(t *testing.T) {
	p, _ := newScriptedServer(t, `{}`)
	for _, req := range []*providers.EmbeddingsRequest{nil, {Model: "text-embedding-004"}} {
		_, err := p.Embeddings(context.Background(), req)
		e := providers.AsError(err)
		if e == nil || e.Kind != providers.ErrKindInvalidRequest {
			t.Errorf("error = %v, mau invalid_request", err)
		}
	}
}

func TestEmbeddingsMelaporkanKegagalanUpstream(t *testing.T) {
	p, _ := newServer(t, jsonHandler(http.StatusForbidden,
		`{"error":{"code":403,"message":"tidak berhak","status":"PERMISSION_DENIED"}}`))

	for _, input := range [][]string{{"satu"}, {"satu", "dua"}} {
		_, err := p.Embeddings(context.Background(), &providers.EmbeddingsRequest{
			Model: "text-embedding-004", Input: input,
		})
		e := providers.AsError(err)
		if e == nil || e.Kind != providers.ErrKindPermission {
			t.Errorf("error = %v, mau permission", err)
		}
	}
}

func TestEmbeddingsResponsRusak(t *testing.T) {
	p, _ := newScriptedServer(t, `{"embedding": bukan-json`)
	_, err := p.Embeddings(context.Background(), &providers.EmbeddingsRequest{
		Model: "text-embedding-004", Input: []string{"satu"},
	})
	e := providers.AsError(err)
	if e == nil || e.Kind != providers.ErrKindServer {
		t.Errorf("error = %v, mau server", err)
	}
}

func TestModelsMemetakanDaftarModel(t *testing.T) {
	body := `{"models":[
	  {"name":"models/gemini-2.5-flash","displayName":"Gemini 2.5 Flash"},
	  {"name":"tunedModels/abc"},
	  {"name":""}
	]}`
	p, log := newScriptedServer(t, body)

	models, err := p.Models(context.Background())
	if err != nil {
		t.Fatalf("Models() error: %v", err)
	}
	if log.Method() != http.MethodGet || log.Path() != "/v1beta/models" {
		t.Errorf("permintaan = %s %s", log.Method(), log.Path())
	}
	if log.Query() != "pageSize=1000" {
		t.Errorf("query = %q, mau pageSize=1000", log.Query())
	}
	if len(models) != 2 {
		t.Fatalf("jumlah model = %d, mau 2 (entri tanpa nama dibuang)", len(models))
	}
	// Awalan models/ dilepas supaya id yang dilihat klien bisa dikirim kembali apa adanya.
	if models[0].ID != "gemini-2.5-flash" || models[0].OwnedBy != providers.KindGoogle {
		t.Errorf("model[0] = %+v", models[0])
	}
	if models[1].ID != "tunedModels/abc" {
		t.Errorf("model[1].ID = %q, mau nama resource utuh untuk model tuning", models[1].ID)
	}
}

func TestModelsMelaporkanBodyRusak(t *testing.T) {
	p, _ := newScriptedServer(t, `{"models": bukan-json`)
	if _, err := p.Models(context.Background()); err == nil {
		t.Fatal("Models() berhasil, mau error")
	}
}

func TestHealthCheckMemakaiDaftarModel(t *testing.T) {
	p, log := newScriptedServer(t, `{"models":[]}`)

	res := p.HealthCheck(context.Background())
	if !res.Healthy || res.StatusCode != http.StatusOK {
		t.Fatalf("hasil = %+v", res)
	}
	if res.Latency <= 0 {
		t.Error("Latency = 0, mau terukur")
	}
	if log.Path() != "/v1beta/models" || log.Query() != "pageSize=1" {
		t.Errorf("probe = %s?%s", log.Path(), log.Query())
	}
}

func TestHealthCheckMelaporkanKegagalan(t *testing.T) {
	p, _ := newServer(t, jsonHandler(http.StatusForbidden,
		`{"error":{"code":403,"message":"key ditolak","status":"PERMISSION_DENIED"}}`))

	res := p.HealthCheck(context.Background())
	if res.Healthy {
		t.Fatal("Healthy = true, mau false")
	}
	if res.ErrorKind != providers.ErrKindPermission {
		t.Errorf("ErrorKind = %q, mau %q", res.ErrorKind, providers.ErrKindPermission)
	}
	if res.ErrorMessage == "" {
		t.Error("ErrorMessage kosong")
	}
}

func TestHealthCheckMelaporkanKegagalanTransport(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(http.ResponseWriter, *http.Request) {}))
	base := srv.URL
	srv.Close()

	p, err := New(Config{BaseURL: base, Credential: security.Secret(testKey), SSRFPolicy: testPolicy()})
	if err != nil {
		t.Fatalf("New() error: %v", err)
	}
	res := p.HealthCheck(context.Background())
	if res.Healthy || res.ErrorKind != providers.ErrKindNetwork {
		t.Errorf("hasil = %+v", res)
	}
}

func TestPemetaanStatusError(t *testing.T) {
	tests := []struct {
		nama   string
		status int
		body   string
		kind   providers.ErrorKind
	}{
		{"auth", http.StatusUnauthorized, `{"error":{"code":401,"message":"tidak sah","status":"UNAUTHENTICATED"}}`, providers.ErrKindAuth},
		{"permission", http.StatusForbidden, `{"error":{"code":403,"message":"ditolak","status":"PERMISSION_DENIED"}}`, providers.ErrKindPermission},
		{"model tidak ada", http.StatusNotFound, `{"error":{"code":404,"message":"model tidak dikenal","status":"NOT_FOUND"}}`, providers.ErrKindModelNotFound},
		{"permintaan cacat", http.StatusBadRequest, `{"error":{"code":400,"message":"contents wajib","status":"INVALID_ARGUMENT"}}`, providers.ErrKindInvalidRequest},
		{"rate limit", http.StatusTooManyRequests, `{"error":{"code":429,"message":"kuota per menit","status":"RESOURCE_EXHAUSTED"}}`, providers.ErrKindRateLimit},
		{"overloaded", http.StatusServiceUnavailable, `{"error":{"code":503,"message":"model overloaded","status":"UNAVAILABLE"}}`, providers.ErrKindOverloaded},
		{"server", http.StatusInternalServerError, `{"error":{"code":500,"message":"gagal internal","status":"INTERNAL"}}`, providers.ErrKindServer},
		{"body tanpa bentuk error", http.StatusInternalServerError, `bukan json`, providers.ErrKindServer},
		// Gemini melaporkan key tidak sah sebagai 400 INVALID_ARGUMENT, bukan 401.
		{"key tidak sah", http.StatusBadRequest,
			`{"error":{"code":400,"message":"API key not valid. Please pass a valid API key.","status":"INVALID_ARGUMENT"}}`,
			providers.ErrKindAuth},
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
				t.Error("Message kosong")
			}
		})
	}
}

func TestRetryAfterDibaca(t *testing.T) {
	p, _ := newServer(t, func(w http.ResponseWriter, _ *http.Request) {
		w.Header().Set("Retry-After", "12")
		w.WriteHeader(http.StatusTooManyRequests)
		_, _ = io.WriteString(w, `{"error":{"code":429,"message":"tunggu","status":"RESOURCE_EXHAUSTED"}}`)
	})
	_, err := p.ChatCompletion(context.Background(), simpleRequest())
	e := providers.AsError(err)
	if e == nil {
		t.Fatalf("error = %v", err)
	}
	if e.RetryAfter != 12*time.Second {
		t.Errorf("RetryAfter = %v, mau 12s", e.RetryAfter)
	}
}

func TestResponsRusakDilaporkanSebagaiKegagalanServer(t *testing.T) {
	p, _ := newScriptedServer(t, `{"candidates": bukan-json`)
	_, err := p.ChatCompletion(context.Background(), simpleRequest())
	e := providers.AsError(err)
	if e == nil || e.Kind != providers.ErrKindServer {
		t.Fatalf("error = %v, mau server", err)
	}
}

func TestResponsTanpaKandidatDitolak(t *testing.T) {
	p, _ := newScriptedServer(t, `{"candidates":[],"usageMetadata":{"promptTokenCount":3}}`)
	_, err := p.ChatCompletion(context.Background(), simpleRequest())
	e := providers.AsError(err)
	if e == nil || e.Kind != providers.ErrKindServer {
		t.Fatalf("error = %v, mau server", err)
	}
}

func TestPembatalanContextDiklasifikasiTepat(t *testing.T) {
	p, _ := newScriptedServer(t, textResponse)
	ctx, cancel := context.WithCancel(context.Background())
	cancel()

	_, err := p.ChatCompletion(ctx, simpleRequest())
	e := providers.AsError(err)
	if e == nil {
		t.Fatalf("error = %v, mau *providers.Error", err)
	}
	if e.Kind != providers.ErrKindCanceled || e.Retryable() {
		t.Errorf("error = %+v", e)
	}
}

func TestPermintaanNilDitolak(t *testing.T) {
	p, _ := newScriptedServer(t, textResponse)
	if _, err := p.ChatCompletion(context.Background(), nil); providers.AsError(err) == nil {
		t.Fatalf("error = %v, mau *providers.Error", err)
	}
	if _, err := p.ChatCompletionStream(context.Background(), nil); providers.AsError(err) == nil {
		t.Fatalf("error = %v, mau *providers.Error", err)
	}
}

// Kredensial tidak boleh muncul di pesan error mana pun, termasuk ketika upstream
// memantulkannya kembali di badan errornya.
func TestKredensialTidakPernahBocorKePesanError(t *testing.T) {
	echo := `{"error":{"code":400,"message":"API key not valid: ` + testKey + `","status":"INVALID_ARGUMENT"}}`

	t.Run("chat completion", func(t *testing.T) {
		p, _ := newServer(t, jsonHandler(http.StatusBadRequest, echo))
		_, err := p.ChatCompletion(context.Background(), simpleRequest())
		if err == nil {
			t.Fatal("mau error")
		}
		assertTanpaKredensial(t, err.Error())
		e := providers.AsError(err)
		if e == nil {
			t.Fatal("mau *providers.Error")
		}
		assertTanpaKredensial(t, e.Message)
		if !strings.Contains(e.Message, redactedMark) {
			t.Errorf("kredensial tidak diganti penanda tersamar: %q", e.Message)
		}
	})

	t.Run("health check", func(t *testing.T) {
		p, _ := newServer(t, jsonHandler(http.StatusBadRequest, echo))
		assertTanpaKredensial(t, p.HealthCheck(context.Background()).ErrorMessage)
	})

	t.Run("models", func(t *testing.T) {
		p, _ := newServer(t, jsonHandler(http.StatusForbidden, echo))
		_, err := p.Models(context.Background())
		if err == nil {
			t.Fatal("mau error")
		}
		assertTanpaKredensial(t, err.Error())
	})

	t.Run("embeddings", func(t *testing.T) {
		p, _ := newServer(t, jsonHandler(http.StatusBadRequest, echo))
		_, err := p.Embeddings(context.Background(), &providers.EmbeddingsRequest{
			Model: "text-embedding-004", Input: []string{"x"},
		})
		if err == nil {
			t.Fatal("mau error")
		}
		assertTanpaKredensial(t, err.Error())
	})

	t.Run("aliran", func(t *testing.T) {
		p, _ := newServer(t, sseHandler(
			`data: {"error":{"code":403,"message":"key `+testKey+` ditolak","status":"PERMISSION_DENIED"}}`+"\n\n",
		))
		st, err := p.ChatCompletionStream(context.Background(), simpleRequest())
		if err != nil {
			t.Fatalf("ChatCompletionStream() error: %v", err)
		}
		defer st.Close()
		_, err = st.Recv()
		if err == nil {
			t.Fatal("mau error")
		}
		assertTanpaKredensial(t, err.Error())
	})

	t.Run("secret tidak tercetak", func(t *testing.T) {
		assertTanpaKredensial(t, security.Secret(testKey).String())
	})
}

func assertTanpaKredensial(t *testing.T, s string) {
	t.Helper()
	if strings.Contains(s, testKey) {
		t.Fatalf("kredensial bocor: %q", s)
	}
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
	if iface.Kind() != providers.KindGoogle {
		t.Errorf("Kind() = %q", iface.Kind())
	}
}
