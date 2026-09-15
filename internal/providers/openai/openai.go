// Package openai adalah adapter untuk provider yang bicara dialek OpenAI.
//
// Satu paket ini melayani tiga Kind sekaligus — KindOpenAI, KindOpenAICompatible, dan
// KindCustom — karena wire format-nya sama persis: /v1/chat/completions dengan body
// bergaya OpenAI dan aliran SSE yang diakhiri "[DONE]". Memecahnya menjadi tiga paket
// hanya menghasilkan tiga salinan kode yang sama, yang berarti setiap perbaikan harus
// ditulis tiga kali dan dua di antaranya akan terlupa.
//
// Yang benar-benar membedakan ketiga Kind itu hanya seberapa jauh adapter boleh
// berasumsi tentang upstream:
//
//   - KindOpenAI: kami tahu pasti field mana yang didukung, jadi gateway boleh
//     menambahkan field yang dibutuhkannya sendiri — lihat stream_options di chat.go.
//   - KindOpenAICompatible dan KindCustom: upstream-nya tidak dikenal, dan sebagian
//     implementasi menolak dengan 400 setiap field yang tidak ada di dokumentasinya.
//     Untuk keduanya adapter mengirim sesedikit mungkin di luar apa yang diminta
//     pemanggil, dan menguraikan respons dengan longgar.
package openai

import (
	"bytes"
	"context"
	"errors"
	"fmt"
	"io"
	"log/slog"
	"maps"
	"net/http"
	"strings"
	"time"

	"github.com/NexGen-X/Route-X/internal/providers"
	"github.com/NexGen-X/Route-X/internal/security"
)

// Endpoint dialek OpenAI. JoinURL yang menangani base URL yang sudah memuat "/v1".
const (
	pathChatCompletions = "/v1/chat/completions"
	pathEmbeddings      = "/v1/embeddings"
	pathModels          = "/v1/models"
)

const (
	// maxResponseBytes membatasi body respons non-streaming yang dibaca ke memori.
	//
	// Body harus utuh di memori karena ChatResponse.Raw adalah body upstream apa adanya,
	// dan itu berarti upstream menentukan berapa banyak memori yang dipakai gateway per
	// request. Batasnya dibuat longgar agar batch embedding besar tetap lewat, tetapi
	// tetap ada supaya satu provider yang bermasalah tidak bisa menghabiskan memori
	// proses.
	maxResponseBytes = 32 << 20
	// maxHealthDrainBytes adalah jumlah byte yang dibuang saat health check. Isinya tidak
	// dipakai, tetapi body perlu dibaca agar koneksi bisa dipakai ulang probe berikutnya.
	maxHealthDrainBytes = 8 << 10
	// maxUpstreamCodeRunes membatasi panjang kode error upstream yang disimpan.
	maxUpstreamCodeRunes = 64
	// healthCheckTimeout adalah batas satu health check.
	//
	// Sengaja jauh lebih pendek dari timeout request: probe berjalan terjadwal untuk
	// SETIAP provider, dan satu upstream yang menggantung dua menit akan menahan slot
	// probe selama itu sambil menunda pemeriksaan provider lain.
	healthCheckTimeout = 10 * time.Second
)

// Config adalah konfigurasi satu provider berdialek OpenAI.
type Config struct {
	Name       string          // nama provider di database, untuk log & metrik
	Kind       string          // providers.KindOpenAI / KindOpenAICompatible / KindCustom
	BaseURL    string          // sudah divalidasi pemanggil
	Credential security.Secret // API key upstream
	Timeout    time.Duration
	SSRFPolicy security.SSRFPolicy
	ProxyURL   security.Secret // egress opsional
	// Organization dan Project untuk header khusus OpenAI; kosong = tidak dikirim.
	Organization string
	Project      string
	// ExtraHeaders untuk provider compatible yang butuh header tambahan.
	ExtraHeaders map[string]string
}

// Provider adalah satu upstream berdialek OpenAI.
//
// Aman dipakai bersamaan oleh banyak goroutine: setelah New tidak ada field yang berubah,
// dan http.Client memang dirancang untuk dipakai bersama.
type Provider struct {
	name         string
	kind         string
	baseURL      string
	credential   security.Secret
	organization string
	project      string
	extraHeaders map[string]string

	client        *http.Client
	streamClient  *http.Client
	healthTimeout time.Duration
}

// Provider wajib memenuhi kontrak paket induk seluruhnya.
var _ providers.Provider = (*Provider)(nil)

// New membuat adapter dari konfigurasi.
func New(cfg Config) (*Provider, error) {
	switch cfg.Kind {
	case providers.KindOpenAI, providers.KindOpenAICompatible, providers.KindCustom:
	case "":
		return nil, errors.New("kind provider wajib diisi")
	default:
		// Dialek lain punya adapternya sendiri; menerima Kind di luar tiga ini berarti
		// mengirim body bergaya OpenAI ke upstream yang tidak memahaminya.
		return nil, fmt.Errorf("kind %q tidak dilayani adapter openai", cfg.Kind)
	}
	if strings.TrimSpace(cfg.BaseURL) == "" {
		return nil, errors.New("base URL provider wajib diisi")
	}
	// Base URL divalidasi di sini meski pemanggil seharusnya sudah melakukannya:
	// konfigurasi provider bisa datang dari baris perintah, berkas, maupun basis
	// data, dan satu jalur yang lupa memvalidasi cukup untuk membuka SSRF.
	if err := security.ValidateBaseURL(cfg.BaseURL, cfg.SSRFPolicy); err != nil {
		return nil, fmt.Errorf("base URL provider openai tidak sah: %w", err)
	}
	// Kredensial wajib hanya untuk KindOpenAI. api.openai.com selalu menuntutnya,
	// sedangkan server berdialek OpenAI yang dijalankan sendiri (Ollama, vLLM, LM Studio)
	// umumnya berjalan tanpa autentikasi — memaksakan kredensial di sana hanya membuat
	// operator mengisi nilai palsu supaya lolos validasi.
	if cfg.Kind == providers.KindOpenAI && cfg.Credential.IsZero() {
		return nil, errors.New("kredensial upstream wajib diisi untuk provider kind openai")
	}

	name := strings.TrimSpace(cfg.Name)
	if name == "" {
		// Nama hanya dipakai sebagai label log dan metrik. Kind masih bisa dibaca
		// manusia, jauh lebih baik daripada label kosong di dasbor.
		name = cfg.Kind
	}

	clientCfg := providers.ClientConfig{
		BaseURL:    cfg.BaseURL,
		Timeout:    cfg.Timeout,
		SSRFPolicy: cfg.SSRFPolicy,
		ProxyURL:   cfg.ProxyURL,
	}
	// Dua klien, bukan satu klien yang timeout-nya diubah per request: http.Client.Timeout
	// tidak bisa diatur per permintaan, dan forStreaming adalah parameter konstruktor
	// paket induk — menghormatinya berarti memanggilnya dua kali, sehingga penyesuaian
	// khusus streaming yang mungkin ditambahkan di sana ikut terpakai tanpa perubahan di
	// adapter ini.
	client, err := providers.NewHTTPClient(clientCfg, false)
	if err != nil {
		return nil, err
	}
	streamClient, err := providers.NewHTTPClient(clientCfg, true)
	if err != nil {
		return nil, err
	}

	health := healthCheckTimeout
	if cfg.Timeout > 0 && cfg.Timeout < health {
		health = cfg.Timeout
	}

	return &Provider{
		name:         name,
		kind:         cfg.Kind,
		baseURL:      cfg.BaseURL,
		credential:   cfg.Credential,
		organization: strings.TrimSpace(cfg.Organization),
		project:      strings.TrimSpace(cfg.Project),
		// Disalin supaya map milik pemanggil yang berubah setelah New tidak mengubah
		// header yang terkirim — dan tidak menjadi data race, karena satu Provider
		// dipakai bersamaan oleh banyak request.
		extraHeaders:  maps.Clone(cfg.ExtraHeaders),
		client:        client,
		streamClient:  streamClient,
		healthTimeout: health,
	}, nil
}

// Kind mengembalikan jenis provider.
func (p *Provider) Kind() string { return p.kind }

// Name mengembalikan nama provider seperti tercatat di database.
func (p *Provider) Name() string { return p.name }

// String menyamarkan seluruh isi Provider.
//
// security.Secret sendiri TIDAK cukup di sini. fmt tidak boleh memanggil metode pada
// field yang tidak diekspor (reflect.Value.CanInterface bernilai false), jadi tanpa
// metode di tingkat struct, "%v" pada Provider akan mencetak isi mentah Secret alih-alih
// "[REDACTED]" — sudah diuji, dan itulah cara paling mudah sebuah API key masuk ke log
// tanpa ada satu pun kode yang tampak mencetaknya.
//
// Base URL juga tidak dicetak: sebagian provider menempelkan kunci di query string, dan
// nama serta kind sudah cukup untuk mengenali provider mana yang dimaksud.
//
// Receiver-nya nilai, bukan pointer, supaya "%v" pada Provider maupun pada *Provider
// sama-sama lewat sini — dengan receiver pointer, mencetak struct-nya langsung tetap bocor.
func (p Provider) String() string {
	return fmt.Sprintf("openai.Provider{name:%q kind:%q}", p.name, p.kind)
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

// setHeaders menyusun header satu permintaan.
//
// ExtraHeaders dipasang PALING DULU, lalu header milik gateway menimpanya. Urutan itu
// disengaja dan sejalan dengan aturan ChatRequest.Extra pada body: ExtraHeaders ada untuk
// MENAMBAH header yang dibutuhkan upstream tertentu, bukan untuk mengganti Authorization.
// Kalau ia menang, satu baris konfigurasi bisa mengirim kredensial yang salah — atau
// menyalin kredensial provider ini ke header yang diteruskan ke tempat lain.
func (p *Provider) setHeaders(h http.Header, hasBody, streaming bool) {
	for k, v := range p.extraHeaders {
		h.Set(k, v)
	}

	if hasBody {
		h.Set("Content-Type", "application/json")
	}
	if streaming {
		h.Set("Accept", "text/event-stream")
	} else {
		h.Set("Accept", "application/json")
	}

	// Reveal dipanggil tepat di titik pemakaian dan hasilnya tidak disimpan ke variabel
	// mana pun: nilai aslinya hanya hidup selama pemanggilan ini.
	if !p.credential.IsZero() {
		token := strings.TrimPrefix(p.credential.Reveal(), "Bearer ")
		h.Set("Authorization", "Bearer "+token)
	}
	if p.organization != "" {
		h.Set("OpenAI-Organization", p.organization)
	}
	if p.project != "" {
		h.Set("OpenAI-Project", p.project)
	}
}

// do mengirim satu permintaan dan hanya mengembalikan respons yang statusnya 2xx.
//
// Pada status gagal body ditutup di sini dan yang keluar hanya *providers.Error. Pada
// status berhasil body diserahkan ke pemanggil, yang WAJIB menutupnya.
func (p *Provider) do(ctx context.Context, method, path string, payload []byte, streaming bool) (*http.Response, *providers.Error) {
	var body io.Reader
	if payload != nil {
		body = bytes.NewReader(payload)
	}

	req, err := http.NewRequestWithContext(ctx, method, providers.JoinURL(p.baseURL, path), body)
	if err != nil {
		// Pesan http.NewRequest memuat URL lengkap, dan base URL bisa memuat kredensial
		// di query string, jadi yang keluar hanya kategorinya.
		return nil, providers.Newf(providers.ErrKindInvalidRequest, p.name, "URL endpoint upstream tidak sah")
	}
	p.setHeaders(req.Header, payload != nil, streaming)

	client := p.client
	if streaming {
		client = p.streamClient
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

// readBody membaca seluruh body respons dengan batas ukuran.
func readBody(provider string, r io.Reader) ([]byte, *providers.Error) {
	raw, err := io.ReadAll(io.LimitReader(r, maxResponseBytes+1))
	if err != nil {
		// Koneksi terputus saat membaca body: kegagalan transport, bukan respons yang
		// salah bentuk.
		return nil, providers.FromTransport(provider, err)
	}
	if len(raw) > maxResponseBytes {
		return nil, providers.Newf(providers.ErrKindServer, provider,
			"respons upstream melebihi batas %d byte", maxResponseBytes)
	}
	return raw, nil
}

// errorFromResponse mengubah respons non-2xx menjadi *providers.Error.
//
// Pesan dari upstream TIDAK ikut. providers.Error.Message berakhir di log operator, di
// tabel pemakaian, dan di respons ke klien, sedangkan pesan error provider sering memuat
// kembali potongan prompt yang menyebabkannya — pada gateway itu berarti data satu
// pengguna berpindah ke tempat yang dibaca orang lain. Yang diambil hanya kode error,
// token pendek seperti "insufficient_quota", plus status HTTP; itu sudah cukup untuk
// mendiagnosis.
func (p *Provider) errorFromResponse(resp *http.Response) *providers.Error {
	code, _ := providers.ReadErrorBody(resp.Body)
	code = sanitizeCode(code)
	kind := providers.Classify(resp.StatusCode, code)

	return &providers.Error{
		Kind:         kind,
		Provider:     p.name,
		StatusCode:   resp.StatusCode,
		Message:      withCode(describeKind(kind, resp.StatusCode), code),
		UpstreamCode: code,
		RetryAfter:   providers.ParseRetryAfter(resp.Header),
	}
}

// errorFromSuccessBody menangkap galat yang dikirim upstream DENGAN status sukses.
//
// Sebagian gateway berdialek OpenAI menjawab 200 dengan body {"error":{...}}. Kalau
// dibiarkan lewat, klien menerima 200 tanpa jawaban, mesin routing menganggap request
// berhasil sehingga tidak pernah failover, dan pencatatan pemakaian menyimpan request
// sukses tanpa token.
func (p *Provider) errorFromSuccessBody(raw []byte, status int, fallback string) *providers.Error {
	code, msg := providers.ReadErrorBody(bytes.NewReader(raw))
	if msg == "" {
		return providers.Newf(providers.ErrKindServer, p.name, "%s", fallback)
	}

	code = sanitizeCode(code)
	kind := providers.Classify(status, code)
	if kind == providers.ErrKindUnknown {
		// Status 2xx tidak memberi petunjuk apa pun, dan kode upstream tidak dikenali.
		// ErrKindServer adalah dugaan terbaik: aman diulang dan aman dialihkan.
		kind = providers.ErrKindServer
	}
	return &providers.Error{
		Kind:         kind,
		Provider:     p.name,
		StatusCode:   status,
		Message:      withCode(fmt.Sprintf("upstream melaporkan galat pada respons berstatus %d", status), code),
		UpstreamCode: code,
	}
}

// describeKind menghasilkan pesan yang aman untuk satu kategori kegagalan.
func describeKind(kind providers.ErrorKind, status int) string {
	switch kind {
	case providers.ErrKindAuth:
		return fmt.Sprintf("kredensial upstream ditolak (status %d)", status)
	case providers.ErrKindPermission:
		return fmt.Sprintf("kredensial tidak berhak atas model atau fitur ini (status %d)", status)
	case providers.ErrKindModelNotFound:
		return fmt.Sprintf("model tidak tersedia di provider ini (status %d)", status)
	case providers.ErrKindRateLimit:
		return fmt.Sprintf("permintaan dibatasi laju oleh provider (status %d)", status)
	case providers.ErrKindQuota:
		return fmt.Sprintf("kuota atau saldo provider habis (status %d)", status)
	case providers.ErrKindContentFilter:
		return fmt.Sprintf("permintaan ditolak kebijakan konten provider (status %d)", status)
	case providers.ErrKindInvalidRequest:
		return fmt.Sprintf("provider menolak bentuk permintaan (status %d)", status)
	case providers.ErrKindOverloaded:
		return fmt.Sprintf("provider sedang penuh (status %d)", status)
	case providers.ErrKindServer:
		return fmt.Sprintf("provider mengembalikan galat internal (status %d)", status)
	case providers.ErrKindTimeout:
		return fmt.Sprintf("provider melaporkan batas waktu terlampaui (status %d)", status)
	default:
		return fmt.Sprintf("respons tidak terduga dari provider (status %d)", status)
	}
}

// withCode menempelkan kode upstream ke pesan bila ada.
func withCode(message, code string) string {
	if code == "" {
		return message
	}
	return message + " (kode upstream: " + code + ")"
}

// sanitizeCode merapikan kode error upstream sebelum ikut ke pesan, log, dan label metrik.
//
// Nilainya datang dari pihak ketiga: panjangnya tidak terbatas dan bisa memuat baris baru
// yang memecah satu baris log menjadi dua. Dibersihkan di satu tempat supaya seluruh
// jalur error tidak perlu mengingatnya, dan dilakukan SEBELUM Classify agar kode yang
// hanya kotor karena spasi tetap terklasifikasi benar.
func sanitizeCode(code string) string {
	code = strings.TrimSpace(code)
	if code == "" {
		return ""
	}
	code = strings.Map(func(r rune) rune {
		if r < 0x20 || r == 0x7f {
			return -1
		}
		return r
	}, code)

	if r := []rune(code); len(r) > maxUpstreamCodeRunes {
		code = string(r[:maxUpstreamCodeRunes])
	}
	return code
}

// wireUsage adalah bentuk usage di dialek OpenAI. Dipakai chat maupun embeddings.
type wireUsage struct {
	PromptTokens     int `json:"prompt_tokens"`
	CompletionTokens int `json:"completion_tokens"`
	TotalTokens      int `json:"total_tokens"`
	// Rincian token yang harganya berbeda. Keduanya opsional: model lama dan sebagian
	// provider compatible tidak mengirimkannya.
	PromptTokensDetails *struct {
		CachedTokens int `json:"cached_tokens"`
	} `json:"prompt_tokens_details"`
	CompletionTokensDetails *struct {
		ReasoningTokens int `json:"reasoning_tokens"`
	} `json:"completion_tokens_details"`
}

// toCanonical menerjemahkan usage ke bentuk kanonik.
func (u *wireUsage) toCanonical() providers.Usage {
	out := providers.Usage{
		InputTokens:  u.PromptTokens,
		OutputTokens: u.CompletionTokens,
		TotalTokens:  u.TotalTokens,
	}
	if u.PromptTokensDetails != nil {
		out.CachedInputTokens = u.PromptTokensDetails.CachedTokens
	}
	if u.CompletionTokensDetails != nil {
		out.ReasoningTokens = u.CompletionTokensDetails.ReasoningTokens
	}
	// Sebagian provider berdialek OpenAI tidak mengirim total_tokens. Biaya dihitung dari
	// input dan output, jadi tidak ada yang hilang, tetapi total nol membuat baris
	// pemakaian terlihat seperti request tanpa token.
	if out.TotalTokens == 0 {
		out.TotalTokens = out.InputTokens + out.OutputTokens
	}
	return out
}

// firstNonEmpty mengembalikan nilai pertama yang tidak kosong.
func firstNonEmpty(values ...string) string {
	for _, v := range values {
		if v != "" {
			return v
		}
	}
	return ""
}
