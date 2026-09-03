// Package providers memuat kontrak satu upstream AI beserta representasi kanonik
// request dan respons.
//
// Representasi kanoniknya bergaya OpenAI, karena itulah dialek yang dipakai permukaan
// API gateway ini — klien yang sudah ada cukup menukar base URL. Provider yang bicara
// dialek lain (Anthropic Messages, Gemini generateContent) diterjemahkan di adapter
// masing-masing, sehingga mesin routing, pencatatan pemakaian, dan perhitungan biaya
// tidak pernah perlu tahu dialek mana yang sedang dipakai.
//
// Menambah provider baru berarti menulis satu paket yang memenuhi Provider. Tidak ada
// bagian gateway di luar paket itu yang perlu diubah.
package providers

import (
	"context"
	"encoding/json"
	"time"
)

// Kind provider. Harus sama dengan constraint providers_kind_valid di migrasi 0003.
const (
	KindOpenAI           = "openai"
	KindAnthropic        = "anthropic"
	KindGoogle           = "google"
	KindOpenAICompatible = "openai_compatible"
	KindCustom           = "custom"
)

// Role pengirim pesan.
type Role string

const (
	RoleSystem    Role = "system"
	RoleUser      Role = "user"
	RoleAssistant Role = "assistant"
	RoleTool      Role = "tool"
	// RoleDeveloper dipakai model penalaran OpenAI sebagai pengganti system.
	RoleDeveloper Role = "developer"
)

// Jenis bagian konten multimodal.
const (
	PartTypeText     = "text"
	PartTypeImageURL = "image_url"
	PartTypeAudio    = "input_audio"
)

// ImageURL adalah rujukan gambar, berupa URL atau data URI base64.
type ImageURL struct {
	URL string `json:"url"`
	// Detail: "low", "high", atau "auto". Kosong berarti biarkan provider memutuskan.
	Detail string `json:"detail,omitempty"`
}

// Audio adalah masukan audio berkode base64.
type Audio struct {
	Data   string `json:"data"`
	Format string `json:"format"`
}

// ContentPart adalah satu bagian pesan multimodal.
type ContentPart struct {
	Type     string    `json:"type"`
	Text     string    `json:"text,omitempty"`
	ImageURL *ImageURL `json:"image_url,omitempty"`
	Audio    *Audio    `json:"input_audio,omitempty"`
}

// Message adalah satu pesan dalam percakapan.
//
// Konten punya dua bentuk karena API OpenAI sendiri menerima keduanya: string tunggal
// untuk pesan teks biasa, dan array bagian untuk multimodal. Parts menang bila terisi;
// adapter tidak boleh menebak dari panjang string.
type Message struct {
	Role    Role   `json:"role"`
	Content string `json:"-"`
	Parts   []ContentPart

	// Name adalah nama peserta opsional.
	Name string
	// ToolCalls berisi permintaan pemanggilan tool dari asisten.
	ToolCalls []ToolCall
	// ToolCallID mengikat pesan berperan tool ke permintaan yang dijawabnya.
	ToolCallID string
	// ReasoningContent adalah jejak penalaran yang dikembalikan sebagian model.
	// Disimpan terpisah dari Content karena penagihannya juga terpisah.
	ReasoningContent string
}

// IsMultimodal melaporkan apakah pesan ini memakai bentuk array bagian.
func (m *Message) IsMultimodal() bool { return len(m.Parts) > 0 }

// Text mengembalikan seluruh teks pesan, menggabungkan bagian bila multimodal. Dipakai
// content filter dan penghitung token perkiraan.
func (m *Message) Text() string {
	if !m.IsMultimodal() {
		return m.Content
	}
	var b []byte
	for _, p := range m.Parts {
		if p.Type == PartTypeText && p.Text != "" {
			if len(b) > 0 {
				b = append(b, '\n')
			}
			b = append(b, p.Text...)
		}
	}
	return string(b)
}

// FunctionDef mendeskripsikan satu fungsi yang boleh dipanggil model.
type FunctionDef struct {
	Name        string          `json:"name"`
	Description string          `json:"description,omitempty"`
	Parameters  json.RawMessage `json:"parameters,omitempty"`
	Strict      *bool           `json:"strict,omitempty"`
}

// Tool adalah satu alat yang tersedia bagi model.
type Tool struct {
	Type     string      `json:"type"`
	Function FunctionDef `json:"function"`
}

// FunctionCall adalah pemanggilan fungsi yang diminta model.
//
// Arguments berupa string JSON, bukan objek terurai — itu memang bentuk yang dikirim
// OpenAI, dan model kadang menghasilkan JSON tidak sah. Menguraikannya di sini berarti
// gateway menolak respons yang sebenarnya harus diteruskan apa adanya ke klien, yang
// lebih siap menangani kasus itu.
type FunctionCall struct {
	Name      string `json:"name"`
	Arguments string `json:"arguments"`
}

// ToolCall adalah satu permintaan pemanggilan tool.
type ToolCall struct {
	// Index dipakai pada respons streaming untuk menyusun ulang potongan.
	Index    int          `json:"index,omitempty"`
	ID       string       `json:"id,omitempty"`
	Type     string       `json:"type,omitempty"`
	Function FunctionCall `json:"function"`
}

// ChatRequest adalah permintaan completion dalam bentuk kanonik.
//
// Model sudah berisi nama di sisi upstream — resolusi alias dan pemilihan provider
// terjadi sebelum request mencapai adapter.
type ChatRequest struct {
	Model    string
	Messages []Message

	MaxTokens   *int
	Temperature *float64
	TopP        *float64
	Stop        []string
	Stream      bool

	Tools      []Tool
	ToolChoice json.RawMessage
	// ParallelToolCalls nil berarti biarkan bawaan provider.
	ParallelToolCalls *bool

	ResponseFormat json.RawMessage
	Seed           *int
	// ReasoningEffort: "minimal", "low", "medium", "high". Kosong berarti bawaan.
	ReasoningEffort string
	// User adalah pengenal pengguna akhir yang diteruskan ke provider untuk
	// penyalahgunaan. Nilainya berasal dari klien; jangan diisi dengan data internal.
	User string

	// Extra memuat field yang tidak dikenal dari body klien.
	//
	// Diteruskan apa adanya ke provider dengan dialek OpenAI. Ini yang membuat
	// parameter baru dari provider bisa dipakai tanpa menunggu gateway diperbarui —
	// dan alasan adapter WAJIB mengabaikan field yang tidak dipahaminya, bukan menolak
	// request.
	Extra map[string]json.RawMessage
}

// Usage adalah jumlah token satu permintaan.
//
// CachedInputTokens dan ReasoningTokens dipisah karena harganya berbeda dari token
// input dan output biasa. Keduanya nol pada provider yang tidak melaporkannya.
type Usage struct {
	InputTokens       int
	CachedInputTokens int
	OutputTokens      int
	ReasoningTokens   int
	TotalTokens       int
}

// Choice adalah satu kandidat jawaban.
type Choice struct {
	Index        int
	Message      Message
	FinishReason string
}

// Alasan berhenti dalam bentuk kanonik (mengikuti OpenAI).
const (
	FinishStop          = "stop"
	FinishLength        = "length"
	FinishToolCalls     = "tool_calls"
	FinishContentFilter = "content_filter"
)

// ChatResponse adalah respons completion non-streaming.
type ChatResponse struct {
	ID      string
	Model   string
	Created int64
	Choices []Choice
	Usage   Usage

	// Raw adalah body upstream apa adanya.
	//
	// Untuk provider berdialek OpenAI, inilah yang diteruskan ke klien — menyusun ulang
	// respons dari struct di atas akan menghapus field yang belum dikenal gateway,
	// padahal klien mungkin memakainya. Untuk provider berdialek lain, adapter mengisi
	// Raw dengan hasil terjemahannya ke bentuk OpenAI.
	Raw json.RawMessage
}

// StreamEvent adalah satu peristiwa pada respons streaming.
type StreamEvent struct {
	// Delta adalah potongan teks baru, kosong pada peristiwa non-teks.
	Delta string
	// ReasoningDelta adalah potongan jejak penalaran.
	ReasoningDelta string
	// ToolCalls adalah potongan pemanggilan tool; Index menandai tool mana.
	ToolCalls []ToolCall
	// FinishReason terisi pada peristiwa terakhir satu choice.
	FinishReason string
	// Usage terisi pada peristiwa penutup bila provider melaporkannya.
	Usage *Usage
	// Raw adalah MUATAN satu peristiwa SSE dalam dialek OpenAI — objek
	// "chat.completion.chunk" berbentuk JSON, dan HANYA itu.
	//
	// Bukan blok SSE lengkap. Framing-nya (awalan "data: ", baris kosong pemisah, dan
	// penanda penutup "[DONE]") dibuat lapisan HTTP gateway, bukan adapter. Pembagian ini
	// harus tegas karena kalau tidak, salah satu dari dua kegagalan pasti terjadi: adapter
	// yang ikut memasang framing membuat gateway mengirim "data: data: {...}" pada provider
	// tertentu saja, atau gateway yang mengandalkan adapter memasangnya membuat aliran dari
	// provider lain tidak pernah punya penutup. Keduanya hanya muncul di provider tertentu,
	// yaitu kelas cacat yang paling sulit terlihat di test.
	//
	// Adapter berdialek OpenAI meneruskan muatan upstream apa adanya di sini — menyusunnya
	// ulang dari field di atas akan menghapus field yang belum dikenal gateway. Adapter
	// dialek lain mengisinya dengan hasil terjemahannya ke bentuk chunk OpenAI.
	//
	// Kosong berarti peristiwa ini tidak punya padanan yang perlu diteruskan ke klien
	// (mis. peristiwa pembuka milik Anthropic yang hanya membawa metadata).
	Raw []byte
}

// Stream adalah aliran peristiwa respons.
//
// Recv mengembalikan io.EOF ketika aliran selesai dengan normal. Close WAJIB dipanggil
// dan aman dipanggil berulang; tanpa itu koneksi upstream menggantung sampai timeout.
type Stream interface {
	Recv() (*StreamEvent, error)
	Close() error
}

// EmbeddingsRequest adalah permintaan embedding.
type EmbeddingsRequest struct {
	Model      string
	Input      []string
	Dimensions *int
	User       string
	// EncodingFormat: "float" atau "base64". Kosong berarti float.
	EncodingFormat string
}

// Embedding adalah satu vektor hasil.
type Embedding struct {
	Index  int
	Vector []float32
}

// EmbeddingsResponse adalah respons embedding.
type EmbeddingsResponse struct {
	Model string
	Data  []Embedding
	Usage Usage
	Raw   json.RawMessage
}

// ModelInfo adalah satu entri daftar model dari provider.
type ModelInfo struct {
	ID      string
	OwnedBy string
	Created int64
}

// HealthResult adalah hasil satu health check.
type HealthResult struct {
	Healthy    bool
	Latency    time.Duration
	StatusCode int
	// ErrorKind adalah kategori stabil untuk diagregasi, mis. "timeout" atau "auth".
	ErrorKind ErrorKind
	// ErrorMessage sudah disaring dan aman disimpan maupun ditampilkan.
	ErrorMessage string
}

// Provider adalah kontrak satu upstream.
//
// Semua method menghormati pembatalan context.
//
// # Kewajiban redaksi
//
// Implementasi TIDAK boleh menyimpan kredensial dalam bentuk plaintext yang bisa
// tercetak — pakai security.Secret. Tetapi security.Secret SENDIRI TIDAK CUKUP bila
// disimpan sebagai field TAK DIEKSPOR: fmt tidak boleh memanggil metode pada field
// seperti itu (reflect.Value.CanInterface bernilai false), sehingga "%v" pada struct
// pemuatnya mencetak isi Secret apa adanya, bukan "[REDACTED]".
//
// Karena itu setiap tipe yang memuat security.Secret di field tak diekspor WAJIB
// menyediakan sendiri:
//
//	func (p T) String() string    // fmt: %v, %s
//	func (p T) GoString() string  // fmt: %#v
//	func (p T) LogValue() slog.Value
//
// Receiver-nya WAJIB nilai, bukan pointer. Dengan receiver pointer, mencetak struct
// nilainya langsung ("%v" pada T, bukan *T) tetap melewati metode dan tetap bocor.
//
// Kewajiban ini dijaga TestRedaksiFieldTakDiekspor di internal/security, yang membaca
// seluruh sumber repo dan menggagalkan build test bila ada tipe baru yang melewatkannya.
type Provider interface {
	// Kind mengembalikan salah satu konstanta Kind di atas.
	Kind() string
	// Name adalah nama provider seperti tercatat di database, dipakai untuk log dan label metrik.
	Name() string

	ChatCompletion(ctx context.Context, req *ChatRequest) (*ChatResponse, error)
	ChatCompletionStream(ctx context.Context, req *ChatRequest) (Stream, error)
	Embeddings(ctx context.Context, req *EmbeddingsRequest) (*EmbeddingsResponse, error)
	Models(ctx context.Context) ([]ModelInfo, error)

	// HealthCheck tidak mengembalikan error: kegagalan ADALAH hasilnya, dan
	// pemanggilnya selalu ingin mencatat hasil itu, bukan menanganinya sebagai galat.
	HealthCheck(ctx context.Context) HealthResult
}
