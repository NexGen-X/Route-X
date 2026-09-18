// Package google menerjemahkan bentuk kanonik gateway ke Gemini generateContent API.
//
// Dialek Gemini adalah yang paling jauh dari bentuk kanonik (yang bergaya OpenAI), dan
// setiap perbedaannya ditangani eksplisit di paket ini:
//
//   - nama field berbeda seluruhnya: contents/parts, bukan messages/content.
//   - peran hanya "user" dan "model"; tidak ada "assistant" maupun "tool".
//   - systemInstruction terpisah dari percakapan.
//   - parameter generasi berkumpul di generationConfig.
//   - nama model masuk ke PATH URL, bukan ke body.
//   - tool memakai functionDeclarations bergaya OpenAPI, panggilannya part functionCall,
//     hasilnya part functionResponse, dan TIDAK ada id panggilan — id-nya disintesis di
//     sini (lihat synthesizeCallID).
//   - promptFeedback.blockReason berarti permintaan diblokir kebijakan konten.
//
// Field Raw pada ChatResponse dan StreamEvent diisi hasil terjemahan KE dialek OpenAI,
// karena itulah muatan yang diteruskan ke klien gateway.
package google

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"log/slog"
	"net/http"
	"net/url"
	"strings"
	"time"

	"github.com/NexGen-X/Route-X/internal/providers"
	"github.com/NexGen-X/Route-X/internal/security"
)

// Nilai bawaan dan nama header dialek Gemini.
const (
	// DefaultBaseURL adalah endpoint publik Gemini API.
	DefaultBaseURL = "https://generativelanguage.googleapis.com"

	// AntigravityBaseURL adalah endpoint resmi Antigravity Cloud Code upstream.
	AntigravityBaseURL = "https://daily-cloudcode-pa.googleapis.com"

	// APIVersion adalah versi jalur Gemini API yang dipakai adapter ini. v1beta adalah
	// jalur yang memuat seluruh kemampuan yang diterjemahkan di sini (tool, thinking,
	// respons berskema); v1 masih tertinggal beberapa di antaranya.
	APIVersion = "v1beta"

	// headerAPIKey adalah satu-satunya tempat kredensial dikirim.
	//
	// Gemini juga menerima key sebagai query parameter ?key=..., dan itu sengaja TIDAK
	// dipakai: query string tercatat di log akses, log proxy, dan jejak error di
	// sepanjang jalur — pada gateway BYOK berarti key milik pengguna tersimpan di berkas
	// log yang dibaca operator.
	headerAPIKey = "x-goog-api-key"

	// maxResponseBytes membatasi body respons non-streaming yang dibaca.
	maxResponseBytes = 32 << 20

	// redactedMark menggantikan kredensial yang terlanjur dipantulkan upstream.
	redactedMark = "[REDACTED]"

	// healthCheckTimeout adalah batas satu health check. Sengaja jauh lebih
	// pendek dari timeout request: probe berjalan terjadwal untuk SETIAP
	// provider, dan satu upstream yang menggantung akan menahan slot probe.
	healthCheckTimeout = 10 * time.Second
)

// Config mengatur satu instance provider Google.
type Config struct {
	// Name adalah nama provider seperti tercatat di database; dipakai untuk log dan metrik.
	Name string
	// Kind biasanya providers.KindGoogle. Kosong berarti nilai itu.
	Kind string
	// BaseURL kosong berarti DefaultBaseURL. Boleh sudah memuat "/v1beta".
	BaseURL string
	// Credential adalah API key Gemini, hanya dibuka saat menyusun header.
	Credential security.Secret
	// Timeout membatasi permintaan non-streaming. Streaming dibatasi lewat context.
	Timeout time.Duration
	// SSRFPolicy menjaga alamat yang boleh dihubungi.
	SSRFPolicy security.SSRFPolicy
	// ProxyURL adalah egress proxy opsional.
	ProxyURL security.Secret
	// ExtraHeaders adalah header tambahan dari operator.
	ExtraHeaders map[string]string
}

// Provider adalah adapter Gemini generateContent API.
type Provider struct {
	name       string
	kind       string
	baseURL    string
	credential security.Secret
	headers    map[string]string

	client *http.Client
	// streamClient tidak memasang http.Client.Timeout; batas itu mencakup pembacaan body.
	streamClient *http.Client
	// healthTimeout membatasi satu health check; nol berarti healthCheckTimeout.
	healthTimeout time.Duration
}

var _ providers.Provider = (*Provider)(nil)

// New membuat provider Google dari konfigurasi.
func New(cfg Config) (*Provider, error) {
	base := strings.TrimSpace(cfg.BaseURL)
	if strings.Contains(strings.ToLower(cfg.Name), "antigravity") && (base == "" || base == DefaultBaseURL) {
		base = AntigravityBaseURL
	}
	if base == "" {
		base = DefaultBaseURL
	}
	if err := security.ValidateBaseURL(base, cfg.SSRFPolicy); err != nil {
		return nil, fmt.Errorf("base URL provider Google tidak sah: %w", err)
	}
	if cfg.Credential.IsZero() {
		return nil, errors.New("kredensial provider Google kosong")
	}

	name := strings.TrimSpace(cfg.Name)
	if name == "" {
		name = providers.KindGoogle
	}
	kind := cfg.Kind
	if kind == "" {
		kind = providers.KindGoogle
	}

	clientCfg := providers.ClientConfig{
		BaseURL:    base,
		Timeout:    cfg.Timeout,
		SSRFPolicy: cfg.SSRFPolicy,
		ProxyURL:   cfg.ProxyURL,
	}
	client, err := providers.NewHTTPClient(clientCfg, false)
	if err != nil {
		return nil, err
	}
	streamClient, err := providers.NewHTTPClient(clientCfg, true)
	if err != nil {
		return nil, err
	}

	headers := make(map[string]string, len(cfg.ExtraHeaders))
	for k, v := range cfg.ExtraHeaders {
		if strings.TrimSpace(k) == "" {
			continue
		}
		headers[k] = v
	}

	return &Provider{
		name:          name,
		kind:          kind,
		baseURL:       strings.TrimRight(base, "/"),
		credential:    cfg.Credential,
		headers:       headers,
		client:        client,
		streamClient:  streamClient,
		healthTimeout: healthTimeoutFor(cfg.Timeout),
	}, nil
}

// healthTimeoutFor menurunkan batas health check: bawaan healthCheckTimeout,
// dipersempit bila Timeout konfigurasi lebih kecil dan positif.
func healthTimeoutFor(configTimeout time.Duration) time.Duration {
	if configTimeout > 0 && configTimeout < healthCheckTimeout {
		return configTimeout
	}
	return healthCheckTimeout
}

// Kind mengembalikan jenis provider.
func (p *Provider) Kind() string { return p.kind }

// Name mengembalikan nama provider.
func (p *Provider) Name() string { return p.name }

// isAntigravity memeriksa apakah provider ini menargetkan backend Google Antigravity Cloud Code.
func (p *Provider) isAntigravity() bool {
	return strings.Contains(p.baseURL, "cloudcode-pa.googleapis.com") ||
		strings.Contains(strings.ToLower(p.name), "antigravity")
}

// String menyamarkan seluruh isi Provider.
//
// security.Secret pada field credential TIDAK cukup di sini. fmt tidak boleh memanggil
// metode pada field yang tidak diekspor (reflect.Value.CanInterface bernilai false untuk
// nilai seperti itu), jadi tanpa metode di tingkat struct "%v" pada Provider mencetak isi
// mentah Secret alih-alih "[REDACTED]". Sudah diuji langsung, dan itulah cara paling mudah
// sebuah API key masuk ke log tanpa ada satu pun kode yang tampak mencetaknya.
//
// Receiver-nya nilai, bukan pointer, supaya "%v" pada Provider maupun pada *Provider
// sama-sama lewat sini — dengan receiver pointer, mencetak struct-nya langsung tetap bocor.
func (p Provider) String() string {
	return fmt.Sprintf("google.Provider{name:%q kind:%q}", p.name, p.kind)
}

// GoString menutup jalur "%#v".
func (p Provider) GoString() string { return p.String() }

// LogValue menutup jalur slog.
func (p Provider) LogValue() slog.Value {
	return slog.GroupValue(slog.String("provider", p.name), slog.String("kind", p.kind))
}

// Verifikasi bahwa jalur keluaran umum benar-benar tertutup.
var (
	_ fmt.Stringer   = Provider{}
	_ fmt.GoStringer = Provider{}
	_ slog.LogValuer = Provider{}
)

// endpoint menyusun URL absolut satu endpoint Gemini.
func (p *Provider) endpoint(path string) string { return providers.JoinURL(p.baseURL, path) }

// methodURL menyusun URL satu method model, mis. ":generateContent".
//
// Nama model Gemini masuk ke PATH, bukan ke body — satu-satunya provider di gateway ini
// yang berperilaku begitu, dan sumber kesalahan yang tidak terlihat sampai upstream
// menjawab 404.
func (p *Provider) methodURL(model, method, query string) (string, error) {
	resource, err := p.modelResourcePath(model)
	if err != nil {
		return "", err
	}
	u := p.endpoint("/" + APIVersion + "/" + resource + ":" + method)
	if query != "" {
		u += "?" + query
	}
	return u, nil
}

// modelResourcePath menyusun potongan path resource model yang sudah di-escape.
func (p *Provider) modelResourcePath(model string) (string, error) {
	name := strings.Trim(strings.TrimSpace(model), "/")
	if name == "" {
		return "", providers.Newf(providers.ErrKindInvalidRequest, p.name, "nama model wajib diisi")
	}
	// ":" memisahkan resource dari method di URL Gemini (".../models/x:generateContent").
	// Nama model yang memuatnya akan menghasilkan path dengan dua method — dan url.
	// PathEscape membiarkan ":" apa adanya, jadi penjagaannya harus di sini. "?" dan "#"
	// ditolak dengan alasan yang sama: keduanya memotong path.
	if strings.ContainsAny(name, ":?#") {
		return "", providers.Newf(providers.ErrKindInvalidRequest, p.name,
			"nama model %q memuat karakter yang tidak diizinkan di path Gemini", model)
	}

	segments := strings.Split(name, "/")
	// Nama pendek ("gemini-2.5-flash") adalah bentuk yang dikirim klien berdialek OpenAI,
	// sedangkan Gemini menuntut nama resource lengkap. Nama yang sudah memuat "/"
	// (mis. "models/gemini-2.5-flash" atau "tunedModels/abc") dibiarkan utuh — menambah
	// awalan di depannya akan menghasilkan "models/models/..." yang selalu 404.
	if len(segments) == 1 {
		segments = []string{"models", segments[0]}
	}
	for i, seg := range segments {
		if seg == "" || seg == "." || seg == ".." {
			return "", providers.Newf(providers.ErrKindInvalidRequest, p.name,
				"nama model %q memuat segmen path yang tidak sah", model)
		}
		// Di-escape per segmen, bukan sekaligus: url.PathEscape juga meng-escape "/"
		// menjadi %2F, jadi meng-escape seluruh nama akan merusak pemisah segmennya.
		segments[i] = url.PathEscape(seg)
	}
	return strings.Join(segments, "/"), nil
}

// modelResourceName mengembalikan nama resource model tanpa escape, untuk dipakai di body.
func modelResourceName(model string) string {
	name := strings.Trim(strings.TrimSpace(model), "/")
	if !strings.Contains(name, "/") {
		return "models/" + name
	}
	return name
}

// newRequest menyusun permintaan HTTP lengkap dengan header autentikasi.
//
// Header wajib dipasang SETELAH ExtraHeaders supaya header dari operator tidak bisa
// menimpa kredensial.
func (p *Provider) newRequest(ctx context.Context, method, endpoint string, body []byte, accept string) (*http.Request, error) {
	var reader io.Reader
	if body != nil {
		reader = bytes.NewReader(body)
	}
	req, err := http.NewRequestWithContext(ctx, method, endpoint, reader)
	if err != nil {
		return nil, providers.Newf(providers.ErrKindInvalidRequest, p.name, "URL endpoint Gemini tidak sah")
	}
	for k, v := range p.headers {
		req.Header.Set(k, v)
	}
	cred := p.credential.Reveal()
	if strings.HasPrefix(cred, "ya29.") || strings.HasPrefix(cred, "Bearer ") {
		req.Header.Set("Authorization", "Bearer "+strings.TrimPrefix(cred, "Bearer "))
	} else if cred != "" {
		req.Header.Set(headerAPIKey, cred)
	}
	if p.isAntigravity() && req.Header.Get("User-Agent") == "" {
		req.Header.Set("User-Agent", "antigravity/1.0.0")
	}
	req.Header.Set("Accept", accept)
	if body != nil {
		req.Header.Set("Content-Type", "application/json")
	}
	return req, nil
}

// do mengirim permintaan dan mengembalikan respons dengan body yang BELUM dibaca.
func (p *Provider) do(ctx context.Context, client *http.Client, method, endpoint string, body []byte, accept string) (*http.Response, error) {
	req, err := p.newRequest(ctx, method, endpoint, body, accept)
	if err != nil {
		return nil, err
	}
	resp, err := client.Do(req)
	if err != nil {
		return nil, providers.FromTransport(p.name, err)
	}
	if resp.StatusCode < 200 || resp.StatusCode > 299 {
		defer resp.Body.Close()
		return nil, p.errorFromResponse(resp)
	}
	return resp, nil
}

func (p *Provider) readAll(resp *http.Response) ([]byte, error) {
	raw, err := io.ReadAll(io.LimitReader(resp.Body, maxResponseBytes+1))
	if err != nil {
		return nil, providers.FromTransport(p.name, err)
	}
	if len(raw) > maxResponseBytes {
		return nil, providers.Newf(providers.ErrKindServer, p.name,
			"respons upstream melebihi batas %d byte", maxResponseBytes)
	}
	return raw, nil
}

// errorFromResponse menerjemahkan respons gagal menjadi error terklasifikasi.
func (p *Provider) errorFromResponse(resp *http.Response) *providers.Error {
	code, message := providers.ReadErrorBody(resp.Body)
	kind := providers.Classify(resp.StatusCode, code)

	// Gemini melaporkan API key yang tidak sah sebagai 400 INVALID_ARGUMENT, bukan 401.
	// Dibiarkan begitu, kredensial salah akan tampak sebagai permintaan cacat: tidak
	// dialihkan ke provider lain, dan operator tidak pernah melihat bahwa yang bermasalah
	// adalah key-nya.
	if lower := strings.ToLower(message); resp.StatusCode == http.StatusBadRequest &&
		(strings.Contains(lower, "api key not valid") || strings.Contains(lower, "api_key_invalid")) {
		kind = providers.ErrKindAuth
	}
	if message == "" {
		message = fmt.Sprintf("Gemini menjawab dengan status %d", resp.StatusCode)
	}
	return &providers.Error{
		Kind:         kind,
		Provider:     p.name,
		StatusCode:   resp.StatusCode,
		Message:      p.safeMessage(message),
		UpstreamCode: code,
		RetryAfter:   providers.ParseRetryAfter(resp.Header),
	}
}

// safeMessage menyaring pesan asal upstream sebelum ikut ke providers.Error.
//
// Kredensial diganti penanda tersamar karena upstream kadang memantulkan kembali key yang
// dikirim, dan pesan itu berakhir di log operator serta respons ke klien.
func (p *Provider) safeMessage(msg string) string {
	if msg == "" {
		return ""
	}
	if cred := p.credential.Reveal(); cred != "" {
		msg = strings.ReplaceAll(msg, cred, redactedMark)
	}
	return msg
}

// ChatCompletion mengirim satu permintaan generateContent non-streaming.
func (p *Provider) ChatCompletion(ctx context.Context, req *providers.ChatRequest) (*providers.ChatResponse, error) {
	if req == nil {
		return nil, providers.Newf(providers.ErrKindInvalidRequest, p.name, "permintaan kosong")
	}
	body, err := p.buildGenerateRequest(req)
	if err != nil {
		return nil, err
	}
	if p.isAntigravity() {
		var reqObj any
		if err := json.Unmarshal(body, &reqObj); err != nil {
			return nil, providers.Newf(providers.ErrKindInvalidRequest, p.name, "gagal memproses payload Antigravity")
		}
		modelName := strings.TrimPrefix(req.Model, "models/")
		envelope := map[string]any{
			"model":   modelName,
			"project": "",
			"request": reqObj,
		}
		envelopeBytes, err := json.Marshal(envelope)
		if err != nil {
			return nil, providers.Newf(providers.ErrKindInvalidRequest, p.name, "gagal membungkus permintaan Antigravity")
		}

		endpoint := p.endpoint("/v1internal:generateContent")
		resp, err := p.do(ctx, p.client, http.MethodPost, endpoint, envelopeBytes, "application/json")
		if err != nil {
			return nil, err
		}
		defer resp.Body.Close()

		raw, err := p.readAll(resp)
		if err != nil {
			return nil, err
		}

		var upstream generateResponse
		var env struct {
			Response generateResponse `json:"response"`
		}
		if err := json.Unmarshal(raw, &env); err == nil && (len(env.Response.Candidates) > 0 || env.Response.PromptFeedback != nil) {
			upstream = env.Response
		} else {
			if err := json.Unmarshal(raw, &upstream); err != nil {
				return nil, providers.Newf(providers.ErrKindServer, p.name, "respons Antigravity tidak bisa diurai")
			}
		}
		return p.toCanonicalResponse(req.Model, &upstream)
	}

	endpoint, err := p.methodURL(req.Model, "generateContent", "")
	if err != nil {
		return nil, err
	}

	resp, err := p.do(ctx, p.client, http.MethodPost, endpoint, body, "application/json")
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()

	raw, err := p.readAll(resp)
	if err != nil {
		return nil, err
	}
	var upstream generateResponse
	if err := json.Unmarshal(raw, &upstream); err != nil {
		return nil, providers.Newf(providers.ErrKindServer, p.name, "respons Gemini tidak bisa diurai")
	}
	return p.toCanonicalResponse(req.Model, &upstream)
}

// Embeddings memanggil :embedContent untuk satu masukan, atau :batchEmbedContents untuk
// beberapa masukan sekaligus.
func (p *Provider) Embeddings(ctx context.Context, req *providers.EmbeddingsRequest) (*providers.EmbeddingsResponse, error) {
	if req == nil || len(req.Input) == 0 {
		return nil, providers.Newf(providers.ErrKindInvalidRequest, p.name, "permintaan embeddings tanpa masukan")
	}
	if len(req.Input) == 1 {
		return p.embedSingle(ctx, req)
	}
	return p.embedBatch(ctx, req)
}

// EmbeddingsRequest.Extra sengaja tidak diteruskan di sini, alasannya sama dengan
// ChatRequest.Extra di translate.go: isinya field berdialek OpenAI, dan body :embedContent
// bentuknya berbeda seluruhnya — meneruskannya apa adanya hanya menghasilkan 400 dari
// Gemini untuk field yang tidak dikenalnya.
func (p *Provider) embedSingle(ctx context.Context, req *providers.EmbeddingsRequest) (*providers.EmbeddingsResponse, error) {
	body, err := json.Marshal(embedRequest{
		Content:              textContent(req.Input[0]),
		OutputDimensionality: req.Dimensions,
	})
	if err != nil {
		return nil, providers.Newf(providers.ErrKindInvalidRequest, p.name, "permintaan embeddings tidak bisa diserialisasi")
	}
	endpoint, err := p.methodURL(req.Model, "embedContent", "")
	if err != nil {
		return nil, err
	}
	var payload struct {
		Embedding embeddingValues `json:"embedding"`
	}
	if err := p.postJSON(ctx, endpoint, body, &payload); err != nil {
		return nil, err
	}
	return p.embeddingsResponse(req.Model, []embeddingValues{payload.Embedding}), nil
}

func (p *Provider) embedBatch(ctx context.Context, req *providers.EmbeddingsRequest) (*providers.EmbeddingsResponse, error) {
	// batchEmbedContents menuntut setiap entri menyebut model-nya sendiri, meski model
	// yang sama sudah ada di path.
	resource := modelResourceName(req.Model)
	batch := batchEmbedRequest{Requests: make([]embedRequest, 0, len(req.Input))}
	for _, in := range req.Input {
		batch.Requests = append(batch.Requests, embedRequest{
			Model:                resource,
			Content:              textContent(in),
			OutputDimensionality: req.Dimensions,
		})
	}
	body, err := json.Marshal(batch)
	if err != nil {
		return nil, providers.Newf(providers.ErrKindInvalidRequest, p.name, "permintaan embeddings tidak bisa diserialisasi")
	}
	endpoint, err := p.methodURL(req.Model, "batchEmbedContents", "")
	if err != nil {
		return nil, err
	}
	var payload struct {
		Embeddings []embeddingValues `json:"embeddings"`
	}
	if err := p.postJSON(ctx, endpoint, body, &payload); err != nil {
		return nil, err
	}
	return p.embeddingsResponse(req.Model, payload.Embeddings), nil
}

// postJSON mengirim body JSON dan mengurai respons ke out.
func (p *Provider) postJSON(ctx context.Context, endpoint string, body []byte, out any) error {
	resp, err := p.do(ctx, p.client, http.MethodPost, endpoint, body, "application/json")
	if err != nil {
		return err
	}
	defer resp.Body.Close()

	raw, err := p.readAll(resp)
	if err != nil {
		return err
	}
	if err := json.Unmarshal(raw, out); err != nil {
		return providers.Newf(providers.ErrKindServer, p.name, "respons Gemini tidak bisa diurai")
	}
	return nil
}

// Models mengambil daftar model dari provider.
func (p *Provider) Models(ctx context.Context) ([]providers.ModelInfo, error) {
	if p.isAntigravity() {
		endpoint := p.endpoint("/v1internal:fetchAvailableModels")
		resp, err := p.do(ctx, p.client, http.MethodPost, endpoint, []byte("{}"), "application/json")
		if err != nil {
			return nil, err
		}
		defer resp.Body.Close()

		raw, err := p.readAll(resp)
		if err != nil {
			return nil, err
		}

		var payload struct {
			Models          map[string]any `json:"models"`
			AgentModelSorts []struct {
				Groups []struct {
					ModelIDs []string `json:"modelIds"`
				} `json:"groups"`
			} `json:"agentModelSorts"`
			CommandModelIds []string            `json:"commandModelIds"`
			TieredModelIds  map[string][]string `json:"tieredModelIds"`
		}
		if err := json.Unmarshal(raw, &payload); err != nil {
			return nil, providers.Newf(providers.ErrKindServer, p.name, "daftar model Antigravity tidak bisa diurai")
		}

		seen := make(map[string]bool)
		var out []providers.ModelInfo

		addModel := func(id string) {
			id = strings.TrimSpace(id)
			if id == "" || seen[id] {
				return
			}
			seen[id] = true
			out = append(out, providers.ModelInfo{
				ID:      id,
				OwnedBy: providers.KindGoogle,
				Created: 0,
			})
		}

		for id := range payload.Models {
			addModel(id)
		}
		for _, sort := range payload.AgentModelSorts {
			for _, g := range sort.Groups {
				for _, id := range g.ModelIDs {
					addModel(id)
				}
			}
		}
		for _, id := range payload.CommandModelIds {
			addModel(id)
		}
		for _, list := range payload.TieredModelIds {
			for _, id := range list {
				addModel(id)
			}
		}

		return out, nil
	}

	// pageSize=1000 adalah batas maksimum Gemini; bawaannya 50, dan daftar yang terpotong
	// akan tampak seperti model yang hilang bagi operator.
	resp, err := p.do(ctx, p.client, http.MethodGet, p.endpoint("/"+APIVersion+"/models")+"?pageSize=1000", nil, "application/json")
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()

	raw, err := p.readAll(resp)
	if err != nil {
		return nil, err
	}
	var payload struct {
		Models []struct {
			Name string `json:"name"`
		} `json:"models"`
	}
	if err := json.Unmarshal(raw, &payload); err != nil {
		return nil, providers.Newf(providers.ErrKindServer, p.name, "daftar model Gemini tidak bisa diurai")
	}

	out := make([]providers.ModelInfo, 0, len(payload.Models))
	for _, m := range payload.Models {
		if m.Name == "" {
			continue
		}
		// Awalan "models/" dilepas supaya id yang dilihat klien sama dengan yang boleh
		// dikirimnya kembali; modelResourcePath memasangnya lagi saat menyusun URL.
		out = append(out, providers.ModelInfo{
			ID:      strings.TrimPrefix(m.Name, "models/"),
			OwnedBy: providers.KindGoogle,
			// Gemini tidak melaporkan waktu pembuatan model.
			Created: 0,
		})
	}
	return out, nil
}

// HealthCheck memeriksa provider.
// Untuk Antigravity memanggil POST /v1internal:fetchAvailableModels.
// Untuk Gemini standar memanggil GET /v1beta/models?pageSize=1.
func (p *Provider) HealthCheck(ctx context.Context) providers.HealthResult {
	timeout := p.healthTimeout
	if timeout <= 0 {
		timeout = healthCheckTimeout
	}
	ctx, cancel := context.WithTimeout(ctx, timeout)
	defer cancel()

	start := time.Now()
	if p.isAntigravity() {
		endpoint := p.endpoint("/v1internal:fetchAvailableModels")
		resp, err := p.do(ctx, p.client, http.MethodPost, endpoint, []byte("{}"), "application/json")
		latency := time.Since(start)
		if err != nil {
			res := providers.HealthResult{Healthy: false, Latency: latency, ErrorKind: providers.ErrKindUnknown}
			if e := providers.AsError(err); e != nil {
				res.ErrorKind = e.Kind
				res.StatusCode = e.StatusCode
				res.ErrorMessage = e.Message
			} else {
				res.ErrorMessage = "gagal menghubungi Google Antigravity"
			}
			return res
		}
		defer resp.Body.Close()
		_, _ = io.Copy(io.Discard, io.LimitReader(resp.Body, 4096))
		return providers.HealthResult{Healthy: true, Latency: latency, StatusCode: resp.StatusCode}
	}

	resp, err := p.do(ctx, p.client, http.MethodGet, p.endpoint("/"+APIVersion+"/models")+"?pageSize=1", nil, "application/json")
	latency := time.Since(start)

	if err != nil {
		res := providers.HealthResult{Healthy: false, Latency: latency, ErrorKind: providers.ErrKindUnknown}
		if e := providers.AsError(err); e != nil {
			res.ErrorKind = e.Kind
			res.StatusCode = e.StatusCode
			res.ErrorMessage = e.Message
		} else {
			res.ErrorMessage = "gagal menghubungi Gemini"
		}
		return res
	}
	defer resp.Body.Close()
	_, _ = io.Copy(io.Discard, io.LimitReader(resp.Body, 4096))

	return providers.HealthResult{Healthy: true, Latency: latency, StatusCode: resp.StatusCode}
}
