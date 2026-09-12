package gateway

import (
	"context"
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/go-chi/chi/v5"

	"github.com/NexGen-X/Route-X/internal/apikey"
	"github.com/NexGen-X/Route-X/internal/database/repo"
	"github.com/NexGen-X/Route-X/internal/database/repo/keys"
	"github.com/NexGen-X/Route-X/internal/database/repo/upstream"
	"github.com/NexGen-X/Route-X/internal/httpx"
	"github.com/NexGen-X/Route-X/internal/providers"
	"github.com/NexGen-X/Route-X/internal/router"
)

// --- Tiruan dependensi -------------------------------------------------------

type pencariModel struct {
	model *upstream.Model
	err   error
}

func (p *pencariModel) Resolve(_ context.Context, _ string) (*upstream.Model, error) {
	if p.err != nil {
		return nil, p.err
	}
	return p.model, nil
}

type pendaftarModel struct {
	models []*upstream.Model
	err    error
}

func (p *pendaftarModel) List(_ context.Context, _ upstream.ModelFilter, _ repo.Page) ([]*upstream.Model, string, error) {
	if p.err != nil {
		return nil, "", p.err
	}
	return p.models, "", nil
}

type sumberKandidat struct {
	cands []*upstream.RouteCandidate
	err   error
}

func (s *sumberKandidat) RouteCandidates(_ context.Context, _ upstream.RouteQuery) ([]*upstream.RouteCandidate, error) {
	if s.err != nil {
		return nil, s.err
	}
	return s.cands, nil
}

type pembatasKey struct {
	model    bool
	provider bool
	err      error
}

func (p *pembatasKey) AllowsModel(_ context.Context, _, _ string) (bool, error) {
	return p.model, p.err
}

func (p *pembatasKey) AllowsProvider(_ context.Context, _, _ string) (bool, error) {
	return p.provider, p.err
}

// pabrikTiruan menyerahkan provider per nama provider, sehingga satu test bisa membuat
// kandidat pertama gagal dan kandidat kedua berhasil.
type pabrikTiruan struct {
	mu        sync.Mutex
	perNama   map[string]providers.Provider
	bawaan    providers.Provider
	err       error
	dipanggil []string
}

func (f *pabrikTiruan) Provider(_ context.Context, c *upstream.RouteCandidate) (providers.Provider, error) {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.dipanggil = append(f.dipanggil, c.ProviderName)
	if f.err != nil {
		return nil, f.err
	}
	if p, ok := f.perNama[c.ProviderName]; ok {
		return p, nil
	}
	return f.bawaan, nil
}

func (f *pabrikTiruan) urutan() []string {
	f.mu.Lock()
	defer f.mu.Unlock()
	return append([]string(nil), f.dipanggil...)
}

// providerTiruan adalah providers.Provider yang perilakunya disuntik per test.
type providerTiruan struct {
	nama  string
	kind  string
	chat  func(ctx context.Context, req *providers.ChatRequest) (*providers.ChatResponse, error)
	alir  func(ctx context.Context, req *providers.ChatRequest) (providers.Stream, error)
	embed func(ctx context.Context, req *providers.EmbeddingsRequest) (*providers.EmbeddingsResponse, error)

	mu       sync.Mutex
	diterima []*providers.ChatRequest
}

var _ providers.Provider = (*providerTiruan)(nil)

func (p *providerTiruan) Kind() string { return p.kind }
func (p *providerTiruan) Name() string { return p.nama }

// Hook yang tidak dipasang mengembalikan kegagalan, bukan panic: satu test yang menyentuh
// jalur yang tidak ia siapkan harus gagal dengan pesan yang menyebut jalurnya, bukan dengan
// nil-pointer di tengah stack trace executor.
func (p *providerTiruan) ChatCompletion(ctx context.Context, req *providers.ChatRequest) (*providers.ChatResponse, error) {
	p.catat(req)
	if p.chat == nil {
		return nil, belumDisiapkan("ChatCompletion")
	}
	return p.chat(ctx, req)
}

func (p *providerTiruan) ChatCompletionStream(ctx context.Context, req *providers.ChatRequest) (providers.Stream, error) {
	p.catat(req)
	if p.alir == nil {
		return nil, belumDisiapkan("ChatCompletionStream")
	}
	return p.alir(ctx, req)
}

func (p *providerTiruan) Embeddings(ctx context.Context, req *providers.EmbeddingsRequest) (*providers.EmbeddingsResponse, error) {
	if p.embed == nil {
		return nil, belumDisiapkan("Embeddings")
	}
	return p.embed(ctx, req)
}

// belumDisiapkan menandai hook provider tiruan yang tidak dipasang test ini.
func belumDisiapkan(metode string) *providers.Error {
	return &providers.Error{
		Kind:     providers.ErrKindInvalidRequest,
		Provider: "tiruan",
		Message:  "provider tiruan tidak menyiapkan " + metode,
	}
}

func (p *providerTiruan) Models(_ context.Context) ([]providers.ModelInfo, error) { return nil, nil }

func (p *providerTiruan) HealthCheck(_ context.Context) providers.HealthResult {
	return providers.HealthResult{Healthy: true}
}

func (p *providerTiruan) catat(req *providers.ChatRequest) {
	p.mu.Lock()
	defer p.mu.Unlock()
	salinan := *req
	p.diterima = append(p.diterima, &salinan)
}

func (p *providerTiruan) terakhir(t *testing.T) *providers.ChatRequest {
	t.Helper()
	p.mu.Lock()
	defer p.mu.Unlock()
	if len(p.diterima) == 0 {
		t.Fatal("provider tiruan tidak menerima satu pun permintaan")
	}
	return p.diterima[len(p.diterima)-1]
}

// aliranTiruan adalah providers.Stream di atas daftar peristiwa yang sudah disiapkan.
type aliranTiruan struct {
	ev      []*providers.StreamEvent
	i       int
	akhir   error // dikembalikan setelah peristiwa habis; nil berarti io.EOF
	ditutup bool
}

func (s *aliranTiruan) Recv() (*providers.StreamEvent, error) {
	if s.i < len(s.ev) {
		s.i++
		return s.ev[s.i-1], nil
	}
	if s.akhir != nil {
		return nil, s.akhir
	}
	return nil, io.EOF
}

func (s *aliranTiruan) Close() error {
	s.ditutup = true
	return nil
}

// peristiwa menyusun StreamEvent bermuatan chunk berdialek OpenAI.
func peristiwa(delta string) *providers.StreamEvent {
	return &providers.StreamEvent{
		Delta: delta,
		Raw:   []byte(`{"id":"c1","object":"chat.completion.chunk","choices":[{"index":0,"delta":{"content":"` + delta + `"}}]}`),
	}
}

// --- Perkakas ----------------------------------------------------------------

func modelUji() *upstream.Model {
	keluarga := "gpt"
	return &upstream.Model{
		ID:          "8f1f0b6e-0000-4000-8000-00000000m001",
		ModelID:     "gpt-4o-mini",
		DisplayName: "GPT-4o mini",
		Family:      &keluarga,
		Enabled:     true,
		CreatedAt:   time.Unix(1700000000, 0),
	}
}

// kandidatRute membuat kandidat dengan nama upstream yang BERBEDA dari nama kanonik, supaya
// test bisa membuktikan nama mana yang benar-benar dikirim ke adapter.
func kandidatRute(nama, upstreamModel string) *upstream.RouteCandidate {
	return &upstream.RouteCandidate{
		ProviderModelID:   "pm-" + nama,
		UpstreamModelName: upstreamModel,
		SupportsStreaming: true,
		SupportsTools:     true,
		Priority:          10,
		Weight:            1,
		ProviderID:        "prov-" + nama,
		ProviderName:      nama,
		Kind:              providers.KindOpenAI,
		BaseURL:           "https://contoh.invalid",
		TimeoutMS:         2000,
	}
}

// susunan adalah kumpulan dependensi Handlers untuk satu test.
type susunan struct {
	models  *pencariModel
	lister  *pendaftarModel
	cands   *sumberKandidat
	factory *pabrikTiruan
	batas   *pembatasKey
	h       *Handlers
}

// susun membuat Handlers dengan tiruan, tanpa Redis dan tanpa jaringan.
//
// Executor dibuat dengan CircuitGuard nil (diizinkan: lihat komentar CircuitGuard) dan tanpa
// jeda backoff, supaya test failover tidak menghabiskan waktu nyata menunggu.
func susun(t *testing.T, prov providers.Provider, ubah func(*susunan, *HandlersDeps)) *susunan {
	t.Helper()
	s := &susunan{
		models:  &pencariModel{model: modelUji()},
		lister:  &pendaftarModel{},
		cands:   &sumberKandidat{cands: []*upstream.RouteCandidate{kandidatRute("utama", "gpt-4o-mini-2024")}},
		factory: &pabrikTiruan{bawaan: prov},
	}
	deps := HandlersDeps{
		Models:     s.models,
		Lister:     s.lister,
		Candidates: s.cands,
		Factory:    s.factory,
		Executor:   NewExecutor(nil, loggerSenyap(), WithSleepFunc(func(context.Context, time.Duration) error { return nil })),
		Logger:     loggerSenyap(),
	}
	if ubah != nil {
		ubah(s, &deps)
	}
	h, err := NewHandlers(deps)
	if err != nil {
		t.Fatalf("NewHandlers: %v", err)
	}
	s.h = h
	return s
}

// panggil mengirim satu permintaan melalui router /v1 yang sungguhan.
func (s *susunan) panggil(t *testing.T, metode, jalur, body string, ctx context.Context) *httptest.ResponseRecorder {
	t.Helper()
	var pembaca io.Reader
	if body != "" {
		pembaca = strings.NewReader(body)
	}
	r := httptest.NewRequest(metode, jalur, pembaca)
	if ctx != nil {
		r = r.WithContext(ctx)
	}
	w := httptest.NewRecorder()
	s.h.Routes().ServeHTTP(w, r)
	return w
}

// galat mengurai envelope error dari respons.
func galat(t *testing.T, w *httptest.ResponseRecorder) httpx.ErrorDetail {
	t.Helper()
	var env httpx.ErrorResponse
	if err := json.Unmarshal(w.Body.Bytes(), &env); err != nil {
		t.Fatalf("body bukan envelope error: %v (body: %s)", err, w.Body.String())
	}
	return env.Error
}

// kodeGalat mengembalikan kode error, atau string kosong bila null.
func kodeGalat(d httpx.ErrorDetail) string {
	if d.Code == nil {
		return ""
	}
	return *d.Code
}

// ctxDenganKey menaruh principal API key ke context.
func ctxDenganKey(id, pemilik string) context.Context {
	return apikey.WithPrincipal(context.Background(), &apikey.Principal{
		Key: &keys.Key{ID: id, OwnerUserID: pemilik, Prefix: "sk_live_", Last4: "9a21"},
	})
}

// --- Chat completions non-streaming ------------------------------------------

const bodyChatMinimal = `{"model":"gpt-4o-mini","messages":[{"role":"user","content":"halo"}]}`

// respons upstream yang sengaja memuat field yang belum dikenal gateway. Field itu harus
// tetap ada di respons ke klien — itulah gunanya meneruskan Raw apa adanya.
const rawUpstream = `{"id":"chatcmpl-1","object":"chat.completion","model":"gpt-4o-mini-2024",` +
	`"choices":[{"index":0,"message":{"role":"assistant","content":"hai"},"finish_reason":"stop"}],` +
	`"usage":{"prompt_tokens":3,"completion_tokens":2,"total_tokens":5},` +
	`"field_masa_depan":{"apa pun":[1,2,3]}}`

func providerSukses() *providerTiruan {
	return &providerTiruan{
		nama: "utama", kind: providers.KindOpenAI,
		chat: func(_ context.Context, _ *providers.ChatRequest) (*providers.ChatResponse, error) {
			// Choices ikut diisi, bukan hanya Raw: adapter sungguhan mengisi keduanya, dan
			// bagian yang terurai itulah yang dibaca penyaring konten fase jawaban.
			return &providers.ChatResponse{
				ID:  "chatcmpl-1",
				Raw: json.RawMessage(rawUpstream),
				Choices: []providers.Choice{{
					Index:        0,
					Message:      providers.Message{Role: providers.RoleAssistant, Content: "hai"},
					FinishReason: providers.FinishStop,
				}},
			}, nil
		},
		alir: func(_ context.Context, _ *providers.ChatRequest) (providers.Stream, error) {
			return &aliranTiruan{ev: []*providers.StreamEvent{peristiwa("hai")}}, nil
		},
		embed: func(_ context.Context, _ *providers.EmbeddingsRequest) (*providers.EmbeddingsResponse, error) {
			return &providers.EmbeddingsResponse{Raw: json.RawMessage(`{"object":"list","data":[]}`)}, nil
		},
	}
}

func TestChatCompletionsMeneruskanBodyUpstreamApaAdanya(t *testing.T) {
	s := susun(t, providerSukses(), nil)
	w := s.panggil(t, http.MethodPost, "/chat/completions", bodyChatMinimal, nil)

	if w.Code != http.StatusOK {
		t.Fatalf("status = %d, mau 200 (body: %s)", w.Code, w.Body.String())
	}
	if got := w.Body.String(); got != rawUpstream {
		t.Errorf("body respons bukan body upstream apa adanya:\ndapat %s\nmau   %s", got, rawUpstream)
	}
	if got := w.Header().Get("Content-Type"); got != "application/json" {
		t.Errorf("Content-Type = %q", got)
	}
	if got := w.Header().Get("Cache-Control"); got != "no-store" {
		t.Errorf("Cache-Control = %q, mau no-store", got)
	}
}

// TestChatCompletionsMemakaiNamaModelUpstream menjaga penerjemahan nama model.
//
// Satu model kanonik dipetakan ke nama berbeda di setiap provider. Mengirim nama kanonik ke
// upstream berakhir sebagai "model tidak ditemukan" di provider yang sebenarnya melayaninya.
func TestChatCompletionsMemakaiNamaModelUpstream(t *testing.T) {
	prov := providerSukses()
	s := susun(t, prov, nil)
	w := s.panggil(t, http.MethodPost, "/chat/completions", bodyChatMinimal, nil)
	if w.Code != http.StatusOK {
		t.Fatalf("status = %d, mau 200", w.Code)
	}
	if got := prov.terakhir(t).Model; got != "gpt-4o-mini-2024" {
		t.Errorf("model yang dikirim ke adapter = %q, mau nama upstream %q", got, "gpt-4o-mini-2024")
	}
}

// TestResponsesDilayaniHandlerYangSama memastikan nama baru OpenAI ikut dilayani.
func TestResponsesDilayaniHandlerYangSama(t *testing.T) {
	s := susun(t, providerSukses(), nil)
	if w := s.panggil(t, http.MethodPost, "/responses", bodyChatMinimal, nil); w.Code != http.StatusOK {
		t.Errorf("status = %d, mau 200 (body: %s)", w.Code, w.Body.String())
	}
}

func TestChatCompletionsBodyTidakSah(t *testing.T) {
	for _, tc := range []struct {
		nama string
		body string
	}{
		{"kosong", ""},
		{"bukan JSON", `{`},
		{"tanpa model", `{"messages":[{"role":"user","content":"a"}]}`},
		{"tanpa messages", `{"model":"gpt-4o-mini"}`},
	} {
		t.Run(tc.nama, func(t *testing.T) {
			s := susun(t, providerSukses(), nil)
			w := s.panggil(t, http.MethodPost, "/chat/completions", tc.body, nil)
			if w.Code != http.StatusBadRequest {
				t.Fatalf("status = %d, mau 400 (body: %s)", w.Code, w.Body.String())
			}
			d := galat(t, w)
			if d.Type != httpx.ErrTypeInvalidRequest {
				t.Errorf("type = %q, mau %q", d.Type, httpx.ErrTypeInvalidRequest)
			}
			if d.Message == "" {
				t.Error("pesan error kosong")
			}
		})
	}
}

func TestChatCompletionsModelTidakDikenal(t *testing.T) {
	s := susun(t, providerSukses(), func(s *susunan, _ *HandlersDeps) {
		s.models.model, s.models.err = nil, repo.ErrNotFound
	})
	w := s.panggil(t, http.MethodPost, "/chat/completions", bodyChatMinimal, nil)
	if w.Code != http.StatusNotFound {
		t.Fatalf("status = %d, mau 404", w.Code)
	}
	if got := kodeGalat(galat(t, w)); got != "model_not_found" {
		t.Errorf("code = %q, mau model_not_found", got)
	}
}

// TestChatCompletionsModelDimatikan menjaga pembedaan yang menentukan tindak lanjut
// pengguna: nama yang salah menuntut nama lain, model yang dimatikan menuntut menghubungi
// operator. Keduanya 404, tetapi kodenya berbeda.
func TestChatCompletionsModelDimatikan(t *testing.T) {
	s := susun(t, providerSukses(), func(s *susunan, _ *HandlersDeps) {
		s.models.model.Enabled = false
	})
	w := s.panggil(t, http.MethodPost, "/chat/completions", bodyChatMinimal, nil)
	if w.Code != http.StatusNotFound {
		t.Fatalf("status = %d, mau 404", w.Code)
	}
	if got := kodeGalat(galat(t, w)); got != "model_disabled" {
		t.Errorf("code = %q, mau model_disabled", got)
	}
}

// TestChatCompletionsModelUsangTetapDilayani: menandai model usang adalah pemberitahuan,
// bukan pemutusan. Menolaknya akan mematikan lalu lintas pelanggan tanpa masa peralihan.
func TestChatCompletionsModelUsangTetapDilayani(t *testing.T) {
	s := susun(t, providerSukses(), func(s *susunan, _ *HandlersDeps) {
		usang := time.Unix(1700000000, 0)
		s.models.model.DeprecatedAt = &usang
	})
	if w := s.panggil(t, http.MethodPost, "/chat/completions", bodyChatMinimal, nil); w.Code != http.StatusOK {
		t.Errorf("status = %d, mau 200 untuk model usang", w.Code)
	}
}

// --- Tanpa kandidat ----------------------------------------------------------

func TestTanpaPemetaanProviderJadi503(t *testing.T) {
	s := susun(t, providerSukses(), func(s *susunan, _ *HandlersDeps) { s.cands.cands = nil })
	w := s.panggil(t, http.MethodPost, "/chat/completions", bodyChatMinimal, nil)
	if w.Code != http.StatusServiceUnavailable {
		t.Fatalf("status = %d, mau 503", w.Code)
	}
	if got := kodeGalat(galat(t, w)); got != "no_provider_for_model" {
		t.Errorf("code = %q, mau no_provider_for_model", got)
	}
}

// TestSebabTanpaKandidatDibedakan menjaga pesan yang bisa ditindaklanjuti. Pesan tunggal
// "tidak ada provider" tidak memberi tahu operator apakah harus menambah provider,
// menyalakan streaming, atau melonggarkan pembatasan.
func TestSebabTanpaKandidatDibedakan(t *testing.T) {
	for _, tc := range []struct {
		nama   string
		body   string
		ubah   func(c *upstream.RouteCandidate)
		status int
		kode   string
	}{
		{
			"streaming tidak didukung",
			`{"model":"gpt-4o-mini","messages":[{"role":"user","content":"a"}],"stream":true}`,
			func(c *upstream.RouteCandidate) { c.SupportsStreaming = false },
			http.StatusBadRequest, "streaming_not_supported",
		},
		{
			"tool tidak didukung",
			`{"model":"gpt-4o-mini","messages":[{"role":"user","content":"a"}],` +
				`"tools":[{"type":"function","function":{"name":"f"}}]}`,
			func(c *upstream.RouteCandidate) { c.SupportsTools = false },
			http.StatusBadRequest, "tools_not_supported",
		},
		{
			"jendela konteks tidak cukup",
			bodyChatMinimal,
			func(c *upstream.RouteCandidate) { kecil := 1; c.MaxContextWindow = &kecil },
			http.StatusBadRequest, "context_length_exceeded",
		},
	} {
		t.Run(tc.nama, func(t *testing.T) {
			s := susun(t, providerSukses(), func(s *susunan, _ *HandlersDeps) { tc.ubah(s.cands.cands[0]) })
			w := s.panggil(t, http.MethodPost, "/chat/completions", tc.body, nil)
			if w.Code != tc.status {
				t.Fatalf("status = %d, mau %d (body: %s)", w.Code, tc.status, w.Body.String())
			}
			if got := kodeGalat(galat(t, w)); got != tc.kode {
				t.Errorf("code = %q, mau %q", got, tc.kode)
			}
		})
	}
}

// --- Pemetaan kegagalan upstream ---------------------------------------------

func TestKegagalanUpstreamDipetakanKeStatusYangBenar(t *testing.T) {
	for _, tc := range []struct {
		kind   providers.ErrorKind
		status int
		kode   string
	}{
		// Kegagalan milik klien diteruskan apa adanya.
		{providers.ErrKindInvalidRequest, http.StatusBadRequest, "invalid_request_error"},
		{providers.ErrKindContentFilter, http.StatusBadRequest, "content_filter"},
		// Kredensial OPERATOR yang ditolak tidak boleh tampak seperti API key klien yang
		// salah: klien yang patuh akan memutar kunci yang sehat lalu membuka tiket.
		{providers.ErrKindAuth, http.StatusBadGateway, "upstream_auth_failed"},
		{providers.ErrKindQuota, http.StatusBadGateway, "upstream_quota_exhausted"},
		{providers.ErrKindTimeout, http.StatusGatewayTimeout, "upstream_timeout"},
		{providers.ErrKindOverloaded, http.StatusServiceUnavailable, "upstream_overloaded"},
		{providers.ErrKindServer, http.StatusBadGateway, "upstream_error"},
	} {
		t.Run(string(tc.kind), func(t *testing.T) {
			prov := &providerTiruan{
				nama: "utama", kind: providers.KindOpenAI,
				chat: func(_ context.Context, _ *providers.ChatRequest) (*providers.ChatResponse, error) {
					return nil, &providers.Error{Kind: tc.kind, Provider: "utama", Message: "gagal"}
				},
			}
			s := susun(t, prov, nil)
			w := s.panggil(t, http.MethodPost, "/chat/completions", bodyChatMinimal, nil)
			if w.Code != tc.status {
				t.Fatalf("status = %d, mau %d (body: %s)", w.Code, tc.status, w.Body.String())
			}
			if got := kodeGalat(galat(t, w)); got != tc.kode {
				t.Errorf("code = %q, mau %q", got, tc.kode)
			}
		})
	}
}

// TestRateLimitMengirimRetryAfter menjaga satu-satunya kegagalan hulu yang benar-benar
// mereda dengan menunggu, sekaligus pembulatan ke ATAS: Retry-After: 0 mengajak klien
// mengulang seketika, yaitu kebalikan dari maksud header itu.
func TestRateLimitMengirimRetryAfter(t *testing.T) {
	prov := &providerTiruan{
		nama: "utama", kind: providers.KindOpenAI,
		chat: func(_ context.Context, _ *providers.ChatRequest) (*providers.ChatResponse, error) {
			return nil, &providers.Error{
				Kind: providers.ErrKindRateLimit, Provider: "utama",
				Message: "dibatasi", RetryAfter: 1500 * time.Millisecond,
			}
		},
	}
	s := susun(t, prov, nil)
	w := s.panggil(t, http.MethodPost, "/chat/completions", bodyChatMinimal, nil)
	if w.Code != http.StatusTooManyRequests {
		t.Fatalf("status = %d, mau 429", w.Code)
	}
	if got := w.Header().Get("Retry-After"); got != "2" {
		t.Errorf("Retry-After = %q, mau 2 (1,5 detik dibulatkan ke atas)", got)
	}
}

// TestFailoverKeKandidatBerikutnya membuktikan janji utama gateway ini pada jalur HTTP yang
// sungguhan: kandidat pertama gagal dengan kegagalan yang aman dialihkan, dan klien tetap
// menerima jawaban.
func TestFailoverKeKandidatBerikutnya(t *testing.T) {
	rusak := &providerTiruan{
		nama: "rusak", kind: providers.KindOpenAI,
		chat: func(_ context.Context, _ *providers.ChatRequest) (*providers.ChatResponse, error) {
			return nil, &providers.Error{Kind: providers.ErrKindServer, Provider: "rusak", Message: "500"}
		},
	}
	sehat := providerSukses()
	sehat.nama = "sehat"

	s := susun(t, nil, func(s *susunan, _ *HandlersDeps) {
		s.cands.cands = []*upstream.RouteCandidate{
			kandidatRute("rusak", "gpt-4o-mini-2024"),
			kandidatRute("sehat", "gpt-4o-mini-2024"),
		}
		s.factory.perNama = map[string]providers.Provider{"rusak": rusak, "sehat": sehat}
	})

	w := s.panggil(t, http.MethodPost, "/chat/completions", bodyChatMinimal, nil)
	if w.Code != http.StatusOK {
		t.Fatalf("status = %d, mau 200 (body: %s)", w.Code, w.Body.String())
	}
	if got := w.Body.String(); got != rawUpstream {
		t.Errorf("body bukan dari provider yang sehat: %s", got)
	}
	urut := s.factory.urutan()
	if len(urut) < 2 || urut[0] != "rusak" || urut[len(urut)-1] != "sehat" {
		t.Errorf("urutan percobaan = %v, mau berawal di rusak dan berakhir di sehat", urut)
	}
}

// --- Chat completions streaming ----------------------------------------------

const bodyChatStreaming = `{"model":"gpt-4o-mini","messages":[{"role":"user","content":"halo"}],"stream":true}`

func TestChatCompletionsStreamingFramingSSE(t *testing.T) {
	aliran := &aliranTiruan{ev: []*providers.StreamEvent{
		peristiwa("ha"),
		// Peristiwa tanpa muatan — mis. peristiwa pembuka Anthropic yang hanya membawa
		// metadata — TIDAK boleh diteruskan: sebagai "data: \n\n" ia diurai klien sebagai
		// chunk kosong yang tidak sah.
		{Delta: ""},
		peristiwa("i"),
	}}
	prov := &providerTiruan{
		nama: "utama", kind: providers.KindOpenAI,
		alir: func(_ context.Context, _ *providers.ChatRequest) (providers.Stream, error) {
			return aliran, nil
		},
	}
	s := susun(t, prov, nil)
	w := s.panggil(t, http.MethodPost, "/chat/completions", bodyChatStreaming, nil)

	if w.Code != http.StatusOK {
		t.Fatalf("status = %d, mau 200 (body: %s)", w.Code, w.Body.String())
	}
	if got := w.Header().Get("Content-Type"); got != "text/event-stream" {
		t.Errorf("Content-Type = %q, mau text/event-stream", got)
	}
	if got := w.Header().Get("X-Accel-Buffering"); got != "no" {
		t.Errorf("X-Accel-Buffering = %q, mau no", got)
	}

	mau := "data: " + string(peristiwa("ha").Raw) + "\n\n" +
		"data: " + string(peristiwa("i").Raw) + "\n\n" +
		"data: [DONE]\n\n"
	if got := w.Body.String(); got != mau {
		t.Errorf("aliran tidak sesuai framing SSE:\ndapat %q\nmau   %q", got, mau)
	}
	// Tanpa Close, koneksi upstream menggantung sampai tenggat permintaan — satu koneksi
	// tersangkut untuk setiap permintaan streaming.
	if !aliran.ditutup {
		t.Error("aliran upstream tidak ditutup setelah selesai")
	}
}

// TestStreamingGagalSebelumByteTerkirimTetap4xx menjaga batas yang tidak boleh dilewati:
// aliran upstream dibuka LEBIH DULU, dan PrepareSSE baru dipanggil setelah pembukaan itu
// berhasil. Kalau dibalik, penolakan upstream sampai ke klien sebagai 200 disusul peristiwa
// error di dalam stream, dan klien OpenAI mana pun menganggap itu percakapan yang berhenti
// tanpa sebab.
func TestStreamingGagalSebelumByteTerkirimTetapStatusHTTP(t *testing.T) {
	prov := &providerTiruan{
		nama: "utama", kind: providers.KindOpenAI,
		alir: func(_ context.Context, _ *providers.ChatRequest) (providers.Stream, error) {
			return nil, &providers.Error{
				Kind: providers.ErrKindInvalidRequest, Provider: "utama",
				Message: "parameter tidak dikenal",
			}
		},
	}
	s := susun(t, prov, nil)
	w := s.panggil(t, http.MethodPost, "/chat/completions", bodyChatStreaming, nil)

	if w.Code != http.StatusBadRequest {
		t.Fatalf("status = %d, mau 400 (body: %s)", w.Code, w.Body.String())
	}
	if ct := w.Header().Get("Content-Type"); strings.Contains(ct, "event-stream") {
		t.Errorf("Content-Type = %q; header SSE terkirim padahal aliran gagal dibuka", ct)
	}
	if got := kodeGalat(galat(t, w)); got != "invalid_request_error" {
		t.Errorf("code = %q", got)
	}
}

// TestStreamingGagalSetelahSebagianTerkirim menjaga satu-satunya jalur pelaporan yang tersisa
// setelah status terkunci: peristiwa error di dalam stream, disusul penanda penutup supaya
// klien tidak menunggu sampai timeout-nya sendiri.
func TestStreamingGagalSetelahSebagianTerkirim(t *testing.T) {
	aliran := &aliranTiruan{
		ev: []*providers.StreamEvent{peristiwa("ha")},
		akhir: &providers.Error{
			Kind: providers.ErrKindStreamAborted, Provider: "utama",
			Message: "aliran terputus", StreamedBytes: 10,
		},
	}
	prov := &providerTiruan{
		nama: "utama", kind: providers.KindOpenAI,
		alir: func(_ context.Context, _ *providers.ChatRequest) (providers.Stream, error) {
			return aliran, nil
		},
	}
	s := susun(t, prov, nil)
	w := s.panggil(t, http.MethodPost, "/chat/completions", bodyChatStreaming, nil)

	// Status sudah terkunci 200 sejak chunk pertama terkirim.
	if w.Code != http.StatusOK {
		t.Fatalf("status = %d, mau 200", w.Code)
	}
	body := w.Body.String()
	if !strings.HasPrefix(body, "data: "+string(peristiwa("ha").Raw)) {
		t.Errorf("chunk pertama tidak terkirim: %q", body)
	}
	if !strings.HasSuffix(body, "data: [DONE]\n\n") {
		t.Errorf("penanda penutup tidak dikirim setelah kegagalan: %q", body)
	}

	// Peristiwa error memakai envelope yang SAMA dengan jalur non-streaming, supaya klien
	// tidak perlu mempelajari bentuk error kedua.
	var envelope httpx.ErrorResponse
	ditemukan := false
	for _, blok := range strings.Split(body, "\n\n") {
		muatan := strings.TrimPrefix(blok, "data: ")
		if muatan == blok || muatan == "[DONE]" {
			continue
		}
		if json.Unmarshal([]byte(muatan), &envelope) == nil && envelope.Error.Message != "" {
			ditemukan = true
		}
	}
	if !ditemukan {
		t.Errorf("tidak ada peristiwa berenvelope error di dalam aliran: %q", body)
	}
	if envelope.Error.Type != httpx.ErrTypeAPI {
		t.Errorf("type peristiwa error = %q, mau %q", envelope.Error.Type, httpx.ErrTypeAPI)
	}
}

// --- Embeddings --------------------------------------------------------------

func TestEmbeddingsMeneruskanBodyUpstreamApaAdanya(t *testing.T) {
	const raw = `{"object":"list","model":"text-embedding-3-small",` +
		`"data":[{"object":"embedding","index":0,"embedding":[0.1,0.2]}],` +
		`"usage":{"prompt_tokens":2,"total_tokens":2},"field_masa_depan":1}`
	prov := &providerTiruan{
		nama: "utama", kind: providers.KindOpenAI,
		embed: func(_ context.Context, req *providers.EmbeddingsRequest) (*providers.EmbeddingsResponse, error) {
			if req.Model != "gpt-4o-mini-2024" {
				t.Errorf("model yang dikirim ke adapter = %q, mau nama upstream", req.Model)
			}
			return &providers.EmbeddingsResponse{Raw: json.RawMessage(raw)}, nil
		},
	}
	s := susun(t, prov, nil)
	w := s.panggil(t, http.MethodPost, "/embeddings", `{"model":"gpt-4o-mini","input":"halo"}`, nil)

	if w.Code != http.StatusOK {
		t.Fatalf("status = %d, mau 200 (body: %s)", w.Code, w.Body.String())
	}
	if got := w.Body.String(); got != raw {
		t.Errorf("body respons bukan body upstream apa adanya: %s", got)
	}
}

func TestEmbeddingsBodyTidakSah(t *testing.T) {
	s := susun(t, providerSukses(), nil)
	w := s.panggil(t, http.MethodPost, "/embeddings", `{"model":"gpt-4o-mini"}`, nil)
	if w.Code != http.StatusBadRequest {
		t.Fatalf("status = %d, mau 400", w.Code)
	}
	if !strings.Contains(galat(t, w).Message, "input") {
		t.Errorf("pesan error tidak menyebut field input: %s", galat(t, w).Message)
	}
}

// --- GET /v1/models ----------------------------------------------------------

func TestModelsMendaftarRegistryGateway(t *testing.T) {
	tanpaKeluarga := &upstream.Model{
		ID: "id-2", ModelID: "model-tanpa-keluarga", Enabled: true, CreatedAt: time.Unix(1700000001, 0),
	}
	s := susun(t, providerSukses(), func(s *susunan, _ *HandlersDeps) {
		s.lister.models = []*upstream.Model{modelUji(), tanpaKeluarga}
	})
	w := s.panggil(t, http.MethodGet, "/models", "", nil)

	if w.Code != http.StatusOK {
		t.Fatalf("status = %d, mau 200 (body: %s)", w.Code, w.Body.String())
	}
	var daftar struct {
		Object string `json:"object"`
		Data   []struct {
			ID      string `json:"id"`
			Object  string `json:"object"`
			Created int64  `json:"created"`
			OwnedBy string `json:"owned_by"`
		} `json:"data"`
	}
	if err := json.Unmarshal(w.Body.Bytes(), &daftar); err != nil {
		t.Fatalf("body bukan daftar model: %v", err)
	}
	if daftar.Object != "list" || len(daftar.Data) != 2 {
		t.Fatalf("daftar = %+v", daftar)
	}
	if daftar.Data[0].ID != "gpt-4o-mini" || daftar.Data[0].Object != "model" {
		t.Errorf("entri pertama = %+v", daftar.Data[0])
	}
	// Detik, bukan milidetik: klien yang mengubahnya menjadi tanggal mengharapkan detik.
	if daftar.Data[0].Created != 1700000000 {
		t.Errorf("created = %d, mau 1700000000", daftar.Data[0].Created)
	}
	if daftar.Data[0].OwnedBy != "gpt" {
		t.Errorf("owned_by = %q, mau keluarga model", daftar.Data[0].OwnedBy)
	}
	// Menebak nama vendor dari nama model akan salah tepat pada penyedia yang jarang.
	if daftar.Data[1].OwnedBy != "routex" {
		t.Errorf("owned_by tanpa keluarga = %q, mau routex", daftar.Data[1].OwnedBy)
	}
}

func TestModelsTanpaListerJadi500(t *testing.T) {
	s := susun(t, providerSukses(), func(_ *susunan, d *HandlersDeps) { d.Lister = nil })
	// Daftar KOSONG akan membuat klien menyimpulkan gateway ini tidak melayani model apa pun.
	if w := s.panggil(t, http.MethodGet, "/models", "", nil); w.Code != http.StatusInternalServerError {
		t.Errorf("status = %d, mau 500", w.Code)
	}
}

// --- Pembatasan API key ------------------------------------------------------

func TestPembatasanModelPadaKey(t *testing.T) {
	s := susun(t, providerSukses(), func(s *susunan, d *HandlersDeps) {
		s.batas = &pembatasKey{model: false, provider: true}
		d.Restrict = s.batas
	})
	w := s.panggil(t, http.MethodPost, "/chat/completions", bodyChatMinimal, ctxDenganKey("k-1", "u-1"))
	if w.Code != http.StatusForbidden {
		t.Fatalf("status = %d, mau 403 (body: %s)", w.Code, w.Body.String())
	}
	if got := kodeGalat(galat(t, w)); got != "model_not_allowed" {
		t.Errorf("code = %q, mau model_not_allowed", got)
	}
}

// TestPemeriksaanKewenanganGagalTertutup: pembatasan model adalah kontrol KEWENANGAN.
// Melewatkannya saat database tidak bisa dibaca berarti key yang dibatasi mendapat akses ke
// model yang justru dilarang untuknya.
func TestPemeriksaanKewenanganGagalTertutup(t *testing.T) {
	s := susun(t, providerSukses(), func(s *susunan, d *HandlersDeps) {
		s.batas = &pembatasKey{err: repo.ErrNotFound}
		d.Restrict = s.batas
	})
	if w := s.panggil(t, http.MethodPost, "/chat/completions", bodyChatMinimal, ctxDenganKey("k-1", "u-1")); w.Code != http.StatusInternalServerError {
		t.Errorf("status = %d, mau 500", w.Code)
	}
}

func TestPembatasanProviderMenyaringKandidat(t *testing.T) {
	s := susun(t, providerSukses(), func(s *susunan, d *HandlersDeps) {
		s.batas = &pembatasKey{model: true, provider: false}
		d.Restrict = s.batas
	})
	w := s.panggil(t, http.MethodPost, "/chat/completions", bodyChatMinimal, ctxDenganKey("k-1", "u-1"))
	if w.Code != http.StatusServiceUnavailable {
		t.Fatalf("status = %d, mau 503 (body: %s)", w.Code, w.Body.String())
	}
	if got := kodeGalat(galat(t, w)); got != "no_provider_for_model" {
		t.Errorf("code = %q", got)
	}
}

// TestModelsMenghormatiPembatasanKey: daftar yang memuat model yang akan ditolak 403 saat
// dipakai adalah daftar yang menyesatkan — klien memilih dari daftar itu.
func TestModelsMenghormatiPembatasanKey(t *testing.T) {
	s := susun(t, providerSukses(), func(s *susunan, d *HandlersDeps) {
		s.lister.models = []*upstream.Model{modelUji()}
		s.batas = &pembatasKey{model: false, provider: true}
		d.Restrict = s.batas
	})
	w := s.panggil(t, http.MethodGet, "/models", "", ctxDenganKey("k-1", "u-1"))
	if w.Code != http.StatusOK {
		t.Fatalf("status = %d, mau 200", w.Code)
	}
	if !strings.Contains(w.Body.String(), `"data":[]`) {
		t.Errorf("daftar masih memuat model yang dilarang: %s", w.Body.String())
	}
}

// --- Susunan -----------------------------------------------------------------

// TestNewHandlersMenolakDependensiKosong menjaga agar kegagalan susunan muncul saat start,
// bukan sebagai nil-pointer pada permintaan pertama pengguna.
func TestNewHandlersMenolakDependensiKosong(t *testing.T) {
	lengkap := func() HandlersDeps {
		return HandlersDeps{
			Models:     &pencariModel{model: modelUji()},
			Candidates: &sumberKandidat{},
			Factory:    &pabrikTiruan{},
			Executor:   NewExecutor(nil, loggerSenyap()),
		}
	}
	for nama, kosongkan := range map[string]func(*HandlersDeps){
		"Models":     func(d *HandlersDeps) { d.Models = nil },
		"Candidates": func(d *HandlersDeps) { d.Candidates = nil },
		"Factory":    func(d *HandlersDeps) { d.Factory = nil },
		"Executor":   func(d *HandlersDeps) { d.Executor = nil },
	} {
		t.Run(nama, func(t *testing.T) {
			d := lengkap()
			kosongkan(&d)
			if _, err := NewHandlers(d); err == nil {
				t.Errorf("%s kosong diterima, mau ditolak", nama)
			}
		})
	}
	if _, err := NewHandlers(lengkap()); err != nil {
		t.Errorf("dependensi lengkap ditolak: %v", err)
	}
}

// TestRoutesBekerjaSaatDipasangDiBawahPrefiks menjaga komposisi yang dipakai biner.
//
// Routes() dikembalikan sebagai sub-router dan dipasang di "/v1" oleh cmd/ai-gateway. chi
// tidak menulis ulang r.URL.Path saat Mount — pemotongan prefiks hidup di
// RouteContext.RoutePath — sehingga sub-router yang dirakit dengan cara yang salah tetap
// lulus semua test yang memanggil Routes() langsung, lalu menjawab 404 di produksi.
func TestRoutesBekerjaSaatDipasangDiBawahPrefiks(t *testing.T) {
	s := susun(t, providerSukses(), func(s *susunan, _ *HandlersDeps) {
		s.lister.models = []*upstream.Model{modelUji()}
	})

	induk := chi.NewRouter()
	induk.Mount("/v1", s.h.Routes())

	for _, tc := range []struct {
		metode, jalur, body string
	}{
		{http.MethodPost, "/v1/chat/completions", bodyChatMinimal},
		{http.MethodPost, "/v1/responses", bodyChatMinimal},
		{http.MethodPost, "/v1/embeddings", `{"model":"gpt-4o-mini","input":"a"}`},
		{http.MethodGet, "/v1/models", ""},
	} {
		t.Run(tc.jalur, func(t *testing.T) {
			var pembaca io.Reader
			if tc.body != "" {
				pembaca = strings.NewReader(tc.body)
			}
			w := httptest.NewRecorder()
			induk.ServeHTTP(w, httptest.NewRequest(tc.metode, tc.jalur, pembaca))
			if w.Code == http.StatusNotFound {
				t.Fatalf("status = 404; rute tidak cocok saat dipasang di bawah /v1")
			}
		})
	}
}

// --- Test Combo Pipeline, Provider Filter, dan Smart Context Bypass ---------

type petaModel struct {
	models map[string]*upstream.Model
}

func (p *petaModel) Resolve(_ context.Context, requested string) (*upstream.Model, error) {
	if m, ok := p.models[requested]; ok {
		return m, nil
	}
	return nil, repo.ErrNotFound
}

type petaKandidat struct {
	cands map[string][]*upstream.RouteCandidate
}

func (p *petaKandidat) RouteCandidates(_ context.Context, q upstream.RouteQuery) ([]*upstream.RouteCandidate, error) {
	return p.cands[q.ModelID], nil
}

type sumberAturanTiruan struct {
	rules []*upstream.RoutingRule
}

func (s *sumberAturanTiruan) ActiveRules(_ context.Context) ([]*upstream.RoutingRule, error) {
	return s.rules, nil
}

func ptrString(s string) *string { return &s }
func ptrInt(i int) *int          { return &i }

func TestEkstrakComboPipeline(t *testing.T) {
	// 1. Nil rule
	if p := ekstrakComboPipeline(nil); p != nil {
		t.Fatalf("ekstrakComboPipeline(nil) = %v; ingin nil", p)
	}

	// 2. Format lama [combo:tier2=...]
	legacyRule := &router.Rule{
		Description: "Fallback otomatis [combo:tier2=gpt-4o-mini]",
	}
	pLegacy := ekstrakComboPipeline(legacyRule)
	if len(pLegacy) != 1 || pLegacy[0].Tier != 2 || pLegacy[0].Model != "gpt-4o-mini" {
		t.Fatalf("pLegacy salah: %+v", pLegacy)
	}

	// 3. Format baru pipeline JSON
	newRule := &router.Rule{
		Description: `Pipeline gateway [combo:alias=super-gateway] [combo:pipeline=[{"tier":2,"model":"llama-3","providers":["prov-a","prov-b"]},{"tier":3,"model":"gemini-1.5"}]]`,
	}
	pNew := ekstrakComboPipeline(newRule)
	if len(pNew) != 2 {
		t.Fatalf("len(pNew) = %d; ingin 2: %+v", len(pNew), pNew)
	}
	if pNew[0].Tier != 2 || pNew[0].Model != "llama-3" || len(pNew[0].ProviderIDs) != 2 || pNew[0].ProviderIDs[0] != "prov-a" {
		t.Errorf("pNew[0] salah: %+v", pNew[0])
	}
	if pNew[1].Tier != 3 || pNew[1].Model != "gemini-1.5" {
		t.Errorf("pNew[1] salah: %+v", pNew[1])
	}

	// 4. Alias extraction
	alias := ekstrakComboAlias(newRule)
	if alias != "super-gateway" {
		t.Errorf("ekstrakComboAlias = %q; ingin 'super-gateway'", alias)
	}
}

func TestSemuaKandidatKekecilan(t *testing.T) {
	// Kandidat tanpa batas context
	c1 := &upstream.RouteCandidate{ProviderName: "p1"}
	if semuaKandidatKekecilan([]*upstream.RouteCandidate{c1}, 5000) {
		t.Errorf("kandidat tanpa batas konteks tidak boleh dianggap kekecilan")
	}

	// Kandidat dengan context cukup
	c2 := &upstream.RouteCandidate{ProviderName: "p2", MaxContextWindow: ptrInt(10000)}
	if semuaKandidatKekecilan([]*upstream.RouteCandidate{c2}, 5000) {
		t.Errorf("kandidat dengan context 10000 tidak boleh kekecilan untuk 5000 token")
	}

	// Seluruh kandidat kekecilan
	c3 := &upstream.RouteCandidate{ProviderName: "p3", MaxContextWindow: ptrInt(1000)}
	c4 := &upstream.RouteCandidate{ProviderName: "p4", MaxContextWindow: ptrInt(2000)}
	if !semuaKandidatKekecilan([]*upstream.RouteCandidate{c3, c4}, 3000) {
		t.Errorf("kandidat dengan max 1000 & 2000 harus dilaporkan kekecilan untuk 3000 token")
	}
}

func TestComboRoutingCascadeNTier(t *testing.T) {
	m1 := &upstream.Model{ID: "m-t1", ModelID: "tier1-model", Enabled: true}
	m2 := &upstream.Model{ID: "m-t2", ModelID: "tier2-model", Enabled: true}
	m3 := &upstream.Model{ID: "m-t3", ModelID: "tier3-model", Enabled: true}

	cand1 := kandidatRute("prov-t1", "upstream-t1")
	cand2 := kandidatRute("prov-t2", "upstream-t2")
	cand3 := kandidatRute("prov-t3", "upstream-t3")

	models := &petaModel{models: map[string]*upstream.Model{
		"tier1-model": m1,
		"tier2-model": m2,
		"tier3-model": m3,
	}}
	cands := &petaKandidat{cands: map[string][]*upstream.RouteCandidate{
		"m-t1": {cand1},
		"m-t2": {cand2},
		"m-t3": {cand3},
	}}

	rule := &upstream.RoutingRule{
		ID:           "r-combo-3tier",
		Name:         "Aturan 3-Tier",
		MatchModelID: ptrString("m-t1"),
		Strategy:     "priority",
		MaxAttempts:  1,
		Enabled:      true,
		Description:  ptrString(`[combo:pipeline=[{"tier":2,"model":"tier2-model"},{"tier":3,"model":"tier3-model"}]]`),
	}
	engine := router.NewEngine(&sumberAturanTiruan{rules: []*upstream.RoutingRule{rule}}, nil, loggerSenyap())

	p1 := &providerTiruan{
		nama: "prov-t1", kind: providers.KindOpenAI,
		chat: func(_ context.Context, _ *providers.ChatRequest) (*providers.ChatResponse, error) {
			return nil, providers.Newf(providers.ErrKindServer, "prov-t1", "error 500 dari tier 1")
		},
	}
	p2 := &providerTiruan{
		nama: "prov-t2", kind: providers.KindOpenAI,
		chat: func(_ context.Context, _ *providers.ChatRequest) (*providers.ChatResponse, error) {
			return nil, providers.Newf(providers.ErrKindQuota, "prov-t2", "rate limit 429 dari tier 2")
		},
	}
	p3 := &providerTiruan{
		nama: "prov-t3", kind: providers.KindOpenAI,
		chat: func(_ context.Context, _ *providers.ChatRequest) (*providers.ChatResponse, error) {
			return &providers.ChatResponse{
				ID:  "chatcmpl-t3",
				Raw: json.RawMessage(`{"id":"chatcmpl-t3","choices":[{"message":{"content":"jawaban sukses dari tier 3"}}]}`),
			}, nil
		},
	}

	factory := &pabrikTiruan{
		perNama: map[string]providers.Provider{
			"prov-t1": p1,
			"prov-t2": p2,
			"prov-t3": p3,
		},
	}

	deps := HandlersDeps{
		Models:     models,
		Lister:     &pendaftarModel{},
		Candidates: cands,
		Factory:    factory,
		Engine:     engine,
		Executor:   NewExecutor(nil, loggerSenyap(), WithSleepFunc(func(context.Context, time.Duration) error { return nil })),
		Logger:     loggerSenyap(),
	}
	h, err := NewHandlers(deps)
	if err != nil {
		t.Fatalf("NewHandlers: %v", err)
	}

	w := httptest.NewRecorder()
	r := httptest.NewRequest(http.MethodPost, "/chat/completions", strings.NewReader(`{"model":"tier1-model","messages":[{"role":"user","content":"halo"}]}`))
	h.Routes().ServeHTTP(w, r)

	if w.Code != http.StatusOK {
		t.Fatalf("status = %d; ingin 200 (body: %s)", w.Code, w.Body.String())
	}
	if !strings.Contains(w.Body.String(), "jawaban sukses dari tier 3") {
		t.Errorf("respons tidak memuat jawaban dari tier 3: %s", w.Body.String())
	}

	urutan := factory.urutan()
	if len(urutan) != 3 || urutan[0] != "prov-t1" || urutan[1] != "prov-t2" || urutan[2] != "prov-t3" {
		t.Errorf("urutan eksekusi salah: %v; ingin [prov-t1 prov-t2 prov-t3]", urutan)
	}
}

func TestComboRoutingProviderFilterPerTier(t *testing.T) {
	m1 := &upstream.Model{ID: "m-t1", ModelID: "tier1-model", Enabled: true}
	m2 := &upstream.Model{ID: "m-t2", ModelID: "tier2-model", Enabled: true}

	cand1 := kandidatRute("prov-t1", "upstream-t1")
	cand2Bad := kandidatRute("prov-t2-bad", "upstream-t2-bad")
	cand2Good := kandidatRute("prov-t2-good", "upstream-t2-good")

	models := &petaModel{models: map[string]*upstream.Model{
		"tier1-model": m1,
		"tier2-model": m2,
	}}
	cands := &petaKandidat{cands: map[string][]*upstream.RouteCandidate{
		"m-t1": {cand1},
		"m-t2": {cand2Bad, cand2Good},
	}}

	// Rule menyaring Tier 2 hanya boleh memanggil cand2Good (ProviderID "prov-prov-t2-good")
	rule := &upstream.RoutingRule{
		ID:           "r-combo-filter",
		Name:         "Aturan Filter Provider",
		MatchModelID: ptrString("m-t1"),
		Strategy:     "priority",
		Enabled:      true,
		Description:  ptrString(`[combo:pipeline=[{"tier":2,"model":"tier2-model","providers":["prov-prov-t2-good"]}]]`),
	}
	engine := router.NewEngine(&sumberAturanTiruan{rules: []*upstream.RoutingRule{rule}}, nil, loggerSenyap())

	p1 := &providerTiruan{
		nama: "prov-t1", kind: providers.KindOpenAI,
		chat: func(_ context.Context, _ *providers.ChatRequest) (*providers.ChatResponse, error) {
			return nil, providers.Newf(providers.ErrKindServer, "prov-t1", "error 500")
		},
	}
	p2Bad := &providerTiruan{
		nama: "prov-t2-bad", kind: providers.KindOpenAI,
		chat: func(_ context.Context, _ *providers.ChatRequest) (*providers.ChatResponse, error) {
			t.Fatal("prov-t2-bad tidak boleh dipanggil karena disaring oleh filter provider tier!")
			return nil, nil
		},
	}
	p2Good := &providerTiruan{
		nama: "prov-t2-good", kind: providers.KindOpenAI,
		chat: func(_ context.Context, _ *providers.ChatRequest) (*providers.ChatResponse, error) {
			return &providers.ChatResponse{
				ID:  "chatcmpl-t2-good",
				Raw: json.RawMessage(`{"id":"chatcmpl-t2-good","choices":[{"message":{"content":"sukses prov good"}}]}`),
			}, nil
		},
	}

	factory := &pabrikTiruan{
		perNama: map[string]providers.Provider{
			"prov-t1":      p1,
			"prov-t2-bad":  p2Bad,
			"prov-t2-good": p2Good,
		},
	}

	deps := HandlersDeps{
		Models:     models,
		Lister:     &pendaftarModel{},
		Candidates: cands,
		Factory:    factory,
		Engine:     engine,
		Executor:   NewExecutor(nil, loggerSenyap(), WithSleepFunc(func(context.Context, time.Duration) error { return nil })),
		Logger:     loggerSenyap(),
	}
	h, err := NewHandlers(deps)
	if err != nil {
		t.Fatalf("NewHandlers: %v", err)
	}

	w := httptest.NewRecorder()
	r := httptest.NewRequest(http.MethodPost, "/chat/completions", strings.NewReader(`{"model":"tier1-model","messages":[{"role":"user","content":"halo"}]}`))
	h.Routes().ServeHTTP(w, r)

	if w.Code != http.StatusOK {
		t.Fatalf("status = %d; ingin 200 (body: %s)", w.Code, w.Body.String())
	}
	if !strings.Contains(w.Body.String(), "sukses prov good") {
		t.Errorf("respons tidak memuat jawaban yang diharapkan: %s", w.Body.String())
	}
}

func TestComboRoutingSmartContextBypass(t *testing.T) {
	m1 := &upstream.Model{ID: "m-t1", ModelID: "tier1-small-ctx", Enabled: true}
	m2 := &upstream.Model{ID: "m-t2", ModelID: "tier2-large-ctx", Enabled: true}

	// Tier 1 hanya punya context window 100 token
	cand1 := kandidatRute("prov-t1", "upstream-t1")
	cand1.MaxContextWindow = ptrInt(100)

	// Tier 2 punya context window 100000 token
	cand2 := kandidatRute("prov-t2", "upstream-t2")
	cand2.MaxContextWindow = ptrInt(100000)

	models := &petaModel{models: map[string]*upstream.Model{
		"tier1-small-ctx": m1,
		"tier2-large-ctx": m2,
	}}
	cands := &petaKandidat{cands: map[string][]*upstream.RouteCandidate{
		"m-t1": {cand1},
		"m-t2": {cand2},
	}}

	rule := &upstream.RoutingRule{
		ID:           "r-combo-bypass",
		Name:         "Aturan Bypass Context",
		MatchModelID: ptrString("m-t1"),
		Strategy:     "priority",
		Enabled:      true,
		Description:  ptrString(`[combo:pipeline=[{"tier":2,"model":"tier2-large-ctx"}]]`),
	}
	engine := router.NewEngine(&sumberAturanTiruan{rules: []*upstream.RoutingRule{rule}}, nil, loggerSenyap())

	p1 := &providerTiruan{
		nama: "prov-t1", kind: providers.KindOpenAI,
		chat: func(_ context.Context, _ *providers.ChatRequest) (*providers.ChatResponse, error) {
			t.Fatal("prov-t1 tidak boleh dipanggil karena prompt melebihi context window-nya!")
			return nil, nil
		},
	}
	p2 := &providerTiruan{
		nama: "prov-t2", kind: providers.KindOpenAI,
		chat: func(_ context.Context, req *providers.ChatRequest) (*providers.ChatResponse, error) {
			return &providers.ChatResponse{
				ID:  "chatcmpl-t2-bypass",
				Raw: json.RawMessage(`{"id":"chatcmpl-t2-bypass","choices":[{"message":{"content":"sukses via context bypass tier 2"}}]}`),
			}, nil
		},
	}

	factory := &pabrikTiruan{
		perNama: map[string]providers.Provider{
			"prov-t1": p1,
			"prov-t2": p2,
		},
	}

	deps := HandlersDeps{
		Models:     models,
		Lister:     &pendaftarModel{},
		Candidates: cands,
		Factory:    factory,
		Engine:     engine,
		Executor:   NewExecutor(nil, loggerSenyap(), WithSleepFunc(func(context.Context, time.Duration) error { return nil })),
		Logger:     loggerSenyap(),
	}
	h, err := NewHandlers(deps)
	if err != nil {
		t.Fatalf("NewHandlers: %v", err)
	}

	// Prompt panjang: ~100 kata (~130 token), melebihi batas 100 token milik Tier 1
	longPrompt := strings.Repeat("kata kata kata kata ", 25)
	body := `{"model":"tier1-small-ctx","messages":[{"role":"user","content":"` + longPrompt + `"}]}`

	w := httptest.NewRecorder()
	r := httptest.NewRequest(http.MethodPost, "/chat/completions", strings.NewReader(body))
	h.Routes().ServeHTTP(w, r)

	if w.Code != http.StatusOK {
		t.Fatalf("status = %d; ingin 200 via smart context bypass (body: %s)", w.Code, w.Body.String())
	}
	if !strings.Contains(w.Body.String(), "sukses via context bypass tier 2") {
		t.Errorf("respons tidak memuat jawaban tier 2: %s", w.Body.String())
	}

	urutan := factory.urutan()
	if len(urutan) != 1 || urutan[0] != "prov-t2" {
		t.Errorf("prov-t1 harus dilewati langsung; urutan: %v", urutan)
	}
}

