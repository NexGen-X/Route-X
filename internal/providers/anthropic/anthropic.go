// Package anthropic menerjemahkan bentuk kanonik gateway ke Anthropic Messages API.
//
// Dialek Anthropic berbeda dari bentuk kanonik (yang bergaya OpenAI) pada hal-hal yang
// tidak bisa dipetakan satu-satu, dan setiap perbedaan ditangani eksplisit di paket ini:
//
//   - system bukan elemen messages, melainkan field tingkat atas.
//   - peran wajib bergantian user/assistant, dan pesan pertama wajib user.
//   - tool memakai input_schema, panggilannya berupa content block tool_use, dan
//     hasilnya kembali sebagai content block tool_result di pesan berperan user.
//   - max_tokens wajib ada.
//   - usage memisahkan token cache dari input, sehingga total harus dijumlahkan sendiri.
//   - streaming memakai peristiwa bernama tanpa penanda [DONE].
//
// Karena permukaan API gateway ini berdialek OpenAI, field Raw pada ChatResponse dan
// StreamEvent diisi hasil terjemahan KE dialek OpenAI — bukan body Anthropic apa adanya.
// Itulah muatan yang diteruskan ke klien.
package anthropic

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"log/slog"
	"net/http"
	"strings"
	"time"

	"github.com/NexGen-X/Route-X/internal/providers"
	"github.com/NexGen-X/Route-X/internal/security"
)

// Nilai bawaan dan nama header dialek Anthropic.
const (
	// DefaultBaseURL adalah endpoint publik Anthropic.
	DefaultBaseURL = "https://api.anthropic.com"

	// DefaultAPIVersion adalah nilai header anthropic-version.
	//
	// "2023-06-01" dipilih karena itu SATU-SATUNYA versi GA Messages API: Anthropic
	// menambah kemampuan baru secara aditif di bawah versi yang sama (fitur eksperimen
	// masuk lewat header anthropic-beta, bukan lewat versi baru). Jadi menyematkannya
	// justru pilihan paling stabil — tidak ada versi lebih baru yang tertinggal — dan
	// bila Anthropic kelak menerbitkan versi bertanggal lain, operator bisa menimpanya
	// lewat Config.APIVersion tanpa menunggu gateway diperbarui.
	DefaultAPIVersion = "2023-06-01"

	// DefaultMaxTokens adalah nilai max_tokens bila permintaan kanonik tidak menyebutkannya.
	//
	// Anthropic MEWAJIBKAN max_tokens sementara bentuk kanonik membolehkannya kosong,
	// jadi harus ada bawaan. 4096 dipilih karena itu batas keluaran terendah di seluruh
	// keluarga Claude yang masih dilayani (Claude 3 membatasi 4096) — nilai yang lebih
	// besar akan ditolak model-model itu dengan 400, dan kegagalan seperti itu tidak
	// boleh muncul hanya karena klien tidak mengisi satu field opsional. Sekaligus cukup
	// panjang untuk jawaban chat biasa. Operator yang memakai model bergenerasi baru
	// bisa meninggikannya lewat Config.DefaultMaxTokens.
	DefaultMaxTokens = 4096

	headerAPIKey  = "x-api-key"
	headerVersion = "anthropic-version"

	// maxResponseBytes membatasi body respons non-streaming yang dibaca. Tanpa batas,
	// satu provider yang bermasalah bisa menghabiskan memori proses.
	maxResponseBytes = 32 << 20

	// redactedMark menggantikan kredensial yang terlanjur dipantulkan upstream.
	redactedMark = "[REDACTED]"

	// healthCheckTimeout adalah batas satu health check. Sengaja jauh lebih
	// pendek dari timeout request: probe berjalan terjadwal untuk SETIAP
	// provider, dan satu upstream yang menggantung akan menahan slot probe.
	healthCheckTimeout = 10 * time.Second
)

// Config mengatur satu instance provider Anthropic.
type Config struct {
	// Name adalah nama provider seperti tercatat di database; dipakai untuk log dan metrik.
	Name string
	// Kind biasanya providers.KindAnthropic. Kosong berarti nilai itu.
	Kind string
	// BaseURL kosong berarti DefaultBaseURL. Boleh sudah memuat "/v1".
	BaseURL string
	// Credential adalah API key Anthropic. Disimpan sebagai security.Secret dan hanya
	// dibuka tepat saat menyusun header.
	Credential security.Secret
	// APIVersion menimpa DefaultAPIVersion bila diisi.
	APIVersion string
	// Timeout membatasi permintaan non-streaming. Streaming dibatasi lewat context.
	Timeout time.Duration
	// SSRFPolicy menjaga alamat yang boleh dihubungi.
	SSRFPolicy security.SSRFPolicy
	// ProxyURL adalah egress proxy opsional.
	ProxyURL security.Secret
	// ExtraHeaders adalah header tambahan dari operator, mis. "anthropic-beta".
	ExtraHeaders map[string]string
	// DefaultMaxTokens menimpa DefaultMaxTokens bila diisi.
	DefaultMaxTokens int
}

// Provider adalah adapter Anthropic Messages API.
type Provider struct {
	name       string
	kind       string
	baseURL    string
	apiVersion string
	credential security.Secret
	maxTokens  int
	headers    map[string]string

	client *http.Client
	// streamClient tidak memasang http.Client.Timeout: batas itu mencakup pembacaan
	// body, jadi memasangnya berarti setiap aliran mati pada detik yang sama.
	streamClient *http.Client
	// healthTimeout membatasi satu health check; nol berarti healthCheckTimeout.
	healthTimeout time.Duration
}

var _ providers.Provider = (*Provider)(nil)

// New membuat provider Anthropic dari konfigurasi.
//
// Base URL divalidasi ulang di sini meski pemanggil seharusnya sudah melakukannya:
// konfigurasi provider bisa datang dari baris perintah, berkas, maupun basis data, dan
// satu jalur yang lupa memvalidasi cukup untuk membuka SSRF.
func New(cfg Config) (*Provider, error) {
	base := strings.TrimSpace(cfg.BaseURL)
	if base == "" {
		base = DefaultBaseURL
	}
	if err := security.ValidateBaseURL(base, cfg.SSRFPolicy); err != nil {
		return nil, fmt.Errorf("base URL provider Anthropic tidak sah: %w", err)
	}
	if cfg.Credential.IsZero() {
		return nil, errors.New("kredensial provider Anthropic kosong")
	}

	name := strings.TrimSpace(cfg.Name)
	if name == "" {
		name = providers.KindAnthropic
	}
	kind := cfg.Kind
	if kind == "" {
		kind = providers.KindAnthropic
	}
	version := strings.TrimSpace(cfg.APIVersion)
	if version == "" {
		version = DefaultAPIVersion
	}
	maxTokens := cfg.DefaultMaxTokens
	if maxTokens <= 0 {
		maxTokens = DefaultMaxTokens
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
		apiVersion:    version,
		credential:    cfg.Credential,
		maxTokens:     maxTokens,
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
	return fmt.Sprintf("anthropic.Provider{name:%q kind:%q}", p.name, p.kind)
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

// endpoint menyusun URL absolut satu endpoint Anthropic.
func (p *Provider) endpoint(path string) string { return providers.JoinURL(p.baseURL, path) }

// newRequest menyusun permintaan HTTP lengkap dengan header autentikasi.
//
// Header wajib dipasang SETELAH ExtraHeaders supaya header dari operator tidak bisa
// menimpa kredensial atau versi API — salah tulis di konfigurasi seharusnya tidak
// berujung pada permintaan tanpa autentikasi.
func (p *Provider) newRequest(ctx context.Context, method, url string, body []byte, accept string) (*http.Request, error) {
	var reader io.Reader
	if body != nil {
		reader = bytes.NewReader(body)
	}
	req, err := http.NewRequestWithContext(ctx, method, url, reader)
	if err != nil {
		// Pesan dari net/http memuat URL; yang keluar hanya kategorinya.
		return nil, providers.Newf(providers.ErrKindInvalidRequest, p.name, "URL endpoint Anthropic tidak sah")
	}
	for k, v := range p.headers {
		req.Header.Set(k, v)
	}
	req.Header.Set(headerAPIKey, p.credential.Reveal())
	req.Header.Set(headerVersion, p.apiVersion)
	req.Header.Set("Accept", accept)
	if body != nil {
		req.Header.Set("Content-Type", "application/json")
	}
	return req, nil
}

// do mengirim permintaan dan mengembalikan respons dengan body yang BELUM dibaca.
// Pada status non-2xx body sudah ditutup dan yang kembali adalah *providers.Error.
func (p *Provider) do(ctx context.Context, client *http.Client, method, url string, body []byte, accept string) (*http.Response, error) {
	req, err := p.newRequest(ctx, method, url, body, accept)
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

// readAll membaca body respons dengan batas ukuran.
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

	// Anthropic melaporkan saldo habis sebagai invalid_request_error dengan status 400,
	// tanpa kode kuota tersendiri. Dibiarkan begitu, kegagalan yang seharusnya memicu
	// failover justru dianggap permintaan cacat dan tidak dialihkan ke mana pun.
	if kind == providers.ErrKindInvalidRequest && strings.Contains(strings.ToLower(message), "credit balance") {
		kind = providers.ErrKindQuota
	}
	if message == "" {
		message = fmt.Sprintf("Anthropic menjawab dengan status %d", resp.StatusCode)
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
// Kredensial diganti penanda tersamar karena upstream kadang memantulkan kembali key
// yang dikirim (mis. "invalid x-api-key: sk-ant-..."), dan pesan itu berakhir di log
// operator serta respons ke klien. Pada gateway BYOK, key tersebut milik pengguna lain.
func (p *Provider) safeMessage(msg string) string {
	if msg == "" {
		return ""
	}
	if cred := p.credential.Reveal(); cred != "" {
		msg = strings.ReplaceAll(msg, cred, redactedMark)
	}
	return msg
}

// ChatCompletion mengirim satu permintaan completion non-streaming.
func (p *Provider) ChatCompletion(ctx context.Context, req *providers.ChatRequest) (*providers.ChatResponse, error) {
	body, err := p.buildMessagesRequest(req, false)
	if err != nil {
		return nil, err
	}
	resp, err := p.do(ctx, p.client, http.MethodPost, p.endpoint("/v1/messages"), body, "application/json")
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()

	raw, err := p.readAll(resp)
	if err != nil {
		return nil, err
	}
	var upstream messagesResponse
	if err := json.Unmarshal(raw, &upstream); err != nil {
		return nil, providers.Newf(providers.ErrKindServer, p.name, "respons Anthropic tidak bisa diurai")
	}
	return p.toCanonicalResponse(&upstream), nil
}

// Embeddings selalu gagal: Anthropic tidak menyediakan API embeddings.
//
// Dikembalikan sebagai ErrKindInvalidRequest, bukan kegagalan generik, supaya mesin
// routing TIDAK mengalihkannya ke provider lain sebagai kegagalan sementara — masalahnya
// ada pada pemetaan model di konfigurasi, dan mencoba provider lain hanya menyembunyikan
// salah konfigurasi itu dari operator.
func (p *Provider) Embeddings(_ context.Context, _ *providers.EmbeddingsRequest) (*providers.EmbeddingsResponse, error) {
	return nil, providers.Newf(providers.ErrKindInvalidRequest, p.name,
		"Anthropic tidak menyediakan API embeddings; arahkan model embedding ke provider yang mendukungnya (mis. OpenAI atau Google)")
}

// Models mengambil daftar model dari GET /v1/models.
func (p *Provider) Models(ctx context.Context) ([]providers.ModelInfo, error) {
	// limit=1000 adalah batas maksimum Anthropic. Bawaannya hanya 20, dan daftar yang
	// terpotong akan tampak seperti model yang hilang bagi operator.
	resp, err := p.do(ctx, p.client, http.MethodGet, p.endpoint("/v1/models")+"?limit=1000", nil, "application/json")
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()

	raw, err := p.readAll(resp)
	if err != nil {
		return nil, err
	}
	var payload struct {
		Data []struct {
			ID        string `json:"id"`
			CreatedAt string `json:"created_at"`
		} `json:"data"`
	}
	if err := json.Unmarshal(raw, &payload); err != nil {
		return nil, providers.Newf(providers.ErrKindServer, p.name, "daftar model Anthropic tidak bisa diurai")
	}

	out := make([]providers.ModelInfo, 0, len(payload.Data))
	for _, m := range payload.Data {
		if m.ID == "" {
			continue
		}
		info := providers.ModelInfo{ID: m.ID, OwnedBy: providers.KindAnthropic}
		if t, err := time.Parse(time.RFC3339, m.CreatedAt); err == nil {
			info.Created = t.Unix()
		}
		out = append(out, info)
	}
	return out, nil
}

// HealthCheck memeriksa provider lewat GET /v1/models, bukan lewat completion.
//
// Endpoint daftar model tidak menagih token dan tidak menyentuh kuota model, tetapi tetap
// melewati jalur yang sama pentingnya: DNS, TLS, proxy egress, dan autentikasi key.
// Health check yang memanggil completion akan menagih pengguna untuk setiap probe.
func (p *Provider) HealthCheck(ctx context.Context) providers.HealthResult {
	timeout := p.healthTimeout
	if timeout <= 0 {
		timeout = healthCheckTimeout
	}
	ctx, cancel := context.WithTimeout(ctx, timeout)
	defer cancel()

	start := time.Now()
	resp, err := p.do(ctx, p.client, http.MethodGet, p.endpoint("/v1/models")+"?limit=1", nil, "application/json")
	latency := time.Since(start)

	if err != nil {
		res := providers.HealthResult{Healthy: false, Latency: latency, ErrorKind: providers.ErrKindUnknown}
		if e := providers.AsError(err); e != nil {
			res.ErrorKind = e.Kind
			res.StatusCode = e.StatusCode
			res.ErrorMessage = e.Message
		} else {
			res.ErrorMessage = "gagal menghubungi Anthropic"
		}
		return res
	}
	defer resp.Body.Close()
	// Body dibuang sampai batas kecil supaya koneksi bisa dipakai ulang.
	_, _ = io.Copy(io.Discard, io.LimitReader(resp.Body, 4096))

	return providers.HealthResult{Healthy: true, Latency: time.Since(start), StatusCode: resp.StatusCode}
}
