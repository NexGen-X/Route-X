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
	// PartTypeDocument adalah lampiran dokumen biner (PDF) yang hanya bisa
	// dilayani upstream Anthropic. Ia TIDAK diratakan menjadi teks: PDF yang
	// dibuang atau diubah diam-diam membuat model menjawab tanpa lampirannya.
	// Gateway menyaring kandidat untuk permintaan yang memuat bagian ini —
	// lihat gateway/anthropic.go (Batch C2).
	PartTypeDocument = "document"
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

// Document adalah lampiran dokumen biner berkode base64, padanan block
// `document` bersumber base64 pada Messages API Anthropic.
//
// MediaType saat ini hanya "application/pdf": dokumen teks TIDAK memakai
// bentuk ini, karena teks diratakan menjadi bagian teks biasa saat decode —
// ia setia sebagai teks di semua dialek. PDF tidak punya padanan teks, jadi
// ia berjalan sebagai bagiannya sendiri sampai adapter Anthropic, satu-
// satunya adapter yang bisa meneruskannya utuh.
type Document struct {
	MediaType string `json:"media_type"`
	// Data adalah isi dokumen berkode base64 (bentuk standar, tanpa awalan
	// data URI).
	Data string `json:"data"`
}

// ContentPart adalah satu bagian pesan multimodal.
type ContentPart struct {
	Type     string    `json:"type"`
	Text     string    `json:"text,omitempty"`
	ImageURL *ImageURL `json:"image_url,omitempty"`
	Audio    *Audio    `json:"input_audio,omitempty"`
	Document *Document `json:"document,omitempty"`
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
	// IsError menandai hasil tool yang GAGAL. Hanya bermakna pada pesan
	// berperan tool; padanannya di dialek Anthropic adalah is_error pada
	// block tool_result. Tanpa field ini penanda itu hilang di terjemahan
	// dan model membaca kegagalan sebagai keberhasilan (audit3 V-GW-24).
	IsError bool
	// ReasoningContent adalah jejak penalaran yang dikembalikan sebagian model.
	// Disimpan terpisah dari Content karena penagihannya juga terpisah.
	ReasoningContent string
	// ReasoningSignature adalah tanda tangan kriptografis atas ReasoningContent
	// yang diterbitkan Anthropic bersama block thinking. Anthropic hanya
	// menerima block thinking yang dikirim balik BILA tanda tangannya utuh,
	// jadi keduanya harus berjalan bersama: adapter yang tidak bisa menyertakan
	// tanda tangan ini wajib membuang jejak penalarannya juga, bukan
	// mengirimkannya telanjang (audit3 V-GW-23).
	ReasoningSignature string
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

// ThinkingParam adalah parameter extended thinking bergaya Anthropic.
//
// Type "enabled" menyalakan penalaran dengan anggaran BudgetTokens; "disabled"
// mematikannya secara eksplisit; "adaptive" (B-096) berarti model memutuskan
// sendiri kapan dan seberapa banyak berpikir — tanpa anggaran, dan itulah
// bawaan Claude Code modern. "adaptive" hanya bermakna bagi upstream
// Anthropic: adapter Anthropic meneruskannya apa adanya, sedangkan adapter
// lain membuangnya (gateway juga menstripnya per kandidat di untukKandidat)
// karena upstream non-Anthropic tidak punya konsep itu — representasi paling
// jujurnya di sana adalah membiarkan bawaan upstream, bukan mengarang
// anggaran. Nilai lain tidak diteruskan — permukaan yang mengurai permintaan
// menolaknya sebelum sampai ke sini.
type ThinkingParam struct {
	Type         string
	BudgetTokens int
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
	// TopK membatasi penarikan sampel ke K token teratas. Parameter ini milik
	// dialek Anthropic (dan sebagian penyedia lain); adapter yang tidak
	// mengenalnya mengabaikannya sesuai kontrak Extra di bawah.
	TopK   *int
	Stop   []string
	Stream bool

	// Thinking adalah parameter extended thinking bergaya Anthropic yang
	// diminta klien secara eksplisit. Ia berbeda dari ReasoningEffort:
	// ReasoningEffort adalah intensitas abstrak yang diterjemahkan tiap
	// adapter, sedangkan Thinking membawa budget token persis seperti yang
	// ditulis klien — menerjemahkannya ke effort akan mengubah angka yang
	// diminta. Adapter Anthropic memakainya apa adanya dan ia menang atas
	// terjemahan ReasoningEffort (audit3 V-GW-17).
	Thinking *ThinkingParam

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
	// CacheCreationInputTokens adalah token yang DITULIS ke prompt cache. Ia
	// BAGIAN dari InputTokens (konvensi bersarang yang sama seperti CachedInput),
	// dan dipisahkan karena tarifnya berbeda dari input biasa: Anthropic
	// menagihnya 1,25× harga input. Hanya adaptor Anthropic yang mengisinya.
	CacheCreationInputTokens int
	// CacheCreation1hInputTokens dan CacheCreation5mInputTokens adalah
	// PECAHAN CacheCreationInputTokens menurut TTL tulisnya, dari objek
	// cache_creation pada usage Anthropic: tulis 1 jam ditagih 2× harga
	// input, tulis 5 menit 1,25×. Keduanya BAGIAN dari
	// CacheCreationInputTokens (bukan tambahan di atasnya) dan nol bila
	// upstream tidak melaporkan pecahan — penagihan lalu memperlakukan
	// seluruh creation sebagai tarif 5 menit, persis perilaku sebelum
	// pecahan ini diurai (sisa B-011).
	CacheCreation1hInputTokens int
	CacheCreation5mInputTokens int
	OutputTokens               int
	ReasoningTokens            int
	TotalTokens                int
}

// Choice adalah satu kandidat jawaban.
type Choice struct {
	Index        int
	Message      Message
	FinishReason string
	// StopSequence adalah urutan berhenti yang benar-benar memicu penghentian,
	// terisi hanya bila FinishReason adalah FinishStopSequence. Dialek OpenAI
	// tidak melaporkannya, dialek Anthropic mewajibkannya utuh (audit3 V-GW-28).
	StopSequence string
}

// Alasan berhenti dalam bentuk kanonik (mengikuti OpenAI).
const (
	FinishStop          = "stop"
	FinishLength        = "length"
	FinishToolCalls     = "tool_calls"
	FinishContentFilter = "content_filter"
	// FinishStopSequence dan FinishPauseTurn adalah alasan berhenti khas
	// Anthropic yang TIDAK dilipat ke FinishStop: klien Anthropic membedakan
	// ketiganya, dan melipatkannya berarti memberi tahu klien hal yang salah
	// tentang mengapa model berhenti (audit3 V-GW-28). Permukaan berdialek
	// OpenAI memetakan keduanya kembali ke "stop" saat menulis finish_reason,
	// karena kosakata OpenAI memang tidak punya padanannya.
	FinishStopSequence = "stop_sequence"
	FinishPauseTurn    = "pause_turn"
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
	// StopSequence menyertai FinishReason FinishStopSequence pada peristiwa
	// penutup, sejajar dengan Choice.StopSequence pada jalur non-streaming.
	StopSequence string
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
	Model string
	Input []string
	// TokenInput adalah masukan berbentuk token yang sudah dikodekan klien
	// (bentuk ketiga kontrak OpenAI). Bila terisi, Input kosong dan salah satu
	// dari keduanya selalu terisi. Token TIDAK diterjemahkan gateway menjadi
	// teks — ia hanya bermakna bagi tokenizer model tujuan — jadi adapter
	// berdialek OpenAI meneruskannya apa adanya dan adapter dialek lain
	// menolaknya dengan jelas (audit3 V-GW-40).
	TokenInput [][]int
	Dimensions *int
	User       string
	// EncodingFormat: "float" atau "base64". Kosong berarti float.
	EncodingFormat string

	// Extra memuat field yang tidak dikenal dari body klien, dengan aturan yang sama
	// seperti ChatRequest.Extra: diteruskan apa adanya oleh adapter berdialek OpenAI dan
	// diabaikan oleh adapter dialek lain yang tidak punya padanan untuknya.
	//
	// Ada di sini karena permukaan embeddings juga bertumbuh — dimensions dan
	// encoding_format keduanya tambahan yang belum lama. Tanpa tempat ini, satu-satunya
	// pilihan yang tersisa bagi lapisan HTTP adalah membuang field yang belum dikenal,
	// yaitu permintaan yang diterima lalu dijalankan dengan parameter yang bukan
	// parameter pengirimnya.
	Extra map[string]json.RawMessage
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
