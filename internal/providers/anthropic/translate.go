package anthropic

import (
	"bytes"
	"encoding/json"
	"strings"
	"time"

	"github.com/NexGen-X/Route-X/internal/providers"
)

// Jenis content block Anthropic yang dipakai adapter ini.
const (
	blockText            = "text"
	blockImage           = "image"
	blockDocument        = "document"
	blockToolUse         = "tool_use"
	blockToolResult      = "tool_result"
	blockThinking        = "thinking"
	blockRedactedThink   = "redacted_thinking"
	roleUser             = "user"
	roleAssistant        = "assistant"
	maxMetadataUserIDLen = 256
)

// messagesRequest adalah body POST /v1/messages.
type messagesRequest struct {
	Model         string            `json:"model"`
	MaxTokens     int               `json:"max_tokens"`
	Messages      []upstreamMessage `json:"messages"`
	System        string            `json:"system,omitempty"`
	Temperature   *float64          `json:"temperature,omitempty"`
	TopP          *float64          `json:"top_p,omitempty"`
	TopK          *int              `json:"top_k,omitempty"`
	StopSequences []string          `json:"stop_sequences,omitempty"`
	Stream        bool              `json:"stream,omitempty"`
	Tools         []upstreamTool    `json:"tools,omitempty"`
	ToolChoice    *upstreamChoice   `json:"tool_choice,omitempty"`
	Thinking      *thinkingConfig   `json:"thinking,omitempty"`
	Metadata      *upstreamMetadata `json:"metadata,omitempty"`
}

type upstreamMessage struct {
	Role    string  `json:"role"`
	Content []block `json:"content"`
}

// block adalah satu content block. Satu struct dipakai untuk kedua arah karena bentuk
// block pada request dan respons memang sama; field yang tidak relevan dikosongkan.
type block struct {
	Type string `json:"type"`

	// text dan thinking
	Text      string                     `json:"text,omitempty"`
	Thinking  string                     `json:"thinking,omitempty"`
	Signature providers.SignatureToleran `json:"signature,omitempty"`

	// image
	Source *imageSource `json:"source,omitempty"`

	// tool_use
	ID    string          `json:"id,omitempty"`
	Name  string          `json:"name,omitempty"`
	Input json.RawMessage `json:"input,omitempty"`

	// tool_result
	ToolUseID string  `json:"tool_use_id,omitempty"`
	Content   []block `json:"content,omitempty"`
	IsError   bool    `json:"is_error,omitempty"`
}

type imageSource struct {
	Type      string `json:"type"`
	MediaType string `json:"media_type,omitempty"`
	Data      string `json:"data,omitempty"`
	URL       string `json:"url,omitempty"`
}

type upstreamTool struct {
	Name        string          `json:"name"`
	Description string          `json:"description,omitempty"`
	InputSchema json.RawMessage `json:"input_schema"`
}

type upstreamChoice struct {
	Type string `json:"type"`
	Name string `json:"name,omitempty"`
	// DisableParallelToolUse adalah kebalikan dari parallel_tool_calls milik OpenAI.
	DisableParallelToolUse *bool `json:"disable_parallel_tool_use,omitempty"`
}

type thinkingConfig struct {
	Type         string `json:"type"`
	BudgetTokens int    `json:"budget_tokens,omitempty"`
	// Extra adalah sub-field thinking tak dikenal dari klien (B-137) yang
	// diteruskan setia ke upstream Anthropic; kunci type/budget_tokens
	// tidak boleh ada di sini (decode gateway sudah memisahkannya).
	Extra map[string]json.RawMessage `json:"-"`
}

// MarshalJSON menulis type/budget_tokens lalu menggabungkan Extra apa
// adanya, agar sub-field klien modern (mis. "display") sampai ke upstream.
func (c thinkingConfig) MarshalJSON() ([]byte, error) {
	type alias thinkingConfig
	dasar, err := json.Marshal(alias(c))
	if err != nil {
		return nil, err
	}
	if len(c.Extra) == 0 {
		return dasar, nil
	}
	gabungan := make(map[string]json.RawMessage, len(c.Extra)+2)
	if err := json.Unmarshal(dasar, &gabungan); err != nil {
		return nil, err
	}
	for k, v := range c.Extra {
		if k == "type" || k == "budget_tokens" {
			continue // field dikenal selalu menang dari salinan mentahnya
		}
		gabungan[k] = v
	}
	return json.Marshal(gabungan)
}

type upstreamMetadata struct {
	UserID string `json:"user_id,omitempty"`
}

// upstreamUsage adalah blok usage Anthropic.
//
// input_tokens di sini TIDAK memuat token cache — keduanya dilaporkan terpisah. Itulah
// alasan total tidak boleh diambil dari satu field mana pun; lihat toCanonicalUsage.
type upstreamUsage struct {
	InputTokens              int `json:"input_tokens"`
	OutputTokens             int `json:"output_tokens"`
	CacheReadInputTokens     int `json:"cache_read_input_tokens"`
	CacheCreationInputTokens int `json:"cache_creation_input_tokens"`
	// CacheCreation adalah rincian TTL token tulis cache. Objek ini tidak
	// selalu hadir (respons tanpa tulis cache, atau upstream lama), dan
	// ketidakhadirannya BUKAN nol yang pasti — penagihan membedakannya:
	// tanpa rincian, seluruh creation ditagih tarif 5 menit.
	CacheCreation *rincianCacheCreation `json:"cache_creation"`
}

// rincianCacheCreation adalah objek cache_creation pada usage Anthropic:
// pecahan cache_creation_input_tokens menurut TTL tulisnya. Pada respons
// Anthropic yang sah, ephemeral_1h + ephemeral_5m = cache_creation_input_tokens.
type rincianCacheCreation struct {
	Ephemeral1hInputTokens int `json:"ephemeral_1h_input_tokens"`
	Ephemeral5mInputTokens int `json:"ephemeral_5m_input_tokens"`
}

// upstreamErrorBody adalah objek "error" pada body Anthropic — bentuk yang
// sama dipakai peristiwa error streaming dan body bertipe "error".
type upstreamErrorBody struct {
	Type    string `json:"type"`
	Message string `json:"message"`
}

// messagesResponse adalah respons non-streaming /v1/messages.
type messagesResponse struct {
	ID           string        `json:"id"`
	Type         string        `json:"type"`
	Role         string        `json:"role"`
	Model        string        `json:"model"`
	Content      []block       `json:"content"`
	StopReason   string        `json:"stop_reason"`
	StopSequence string        `json:"stop_sequence"`
	Usage        upstreamUsage `json:"usage"`
	// Error terisi bila upstream membungkus kegagalan sebagai body
	// bertipe "error" (B-212) — termasuk pada respons berstatus 200 dari
	// sebagian upstream kompatibel/proksi. Tanpa field ini body seperti
	// itu terurai menjadi respons kosong yang tampak sukses.
	Error *upstreamErrorBody `json:"error,omitempty"`
}

// buildMessagesRequest menerjemahkan permintaan kanonik menjadi body Anthropic.
func (p *Provider) buildMessagesRequest(req *providers.ChatRequest, stream bool) ([]byte, error) {
	if req == nil {
		return nil, providers.Newf(providers.ErrKindInvalidRequest, p.name, "permintaan kosong")
	}
	if strings.TrimSpace(req.Model) == "" {
		return nil, providers.Newf(providers.ErrKindInvalidRequest, p.name, "nama model wajib diisi")
	}

	system, conversation, err := splitSystem(p.name, req.Messages)
	if err != nil {
		return nil, err
	}
	msgs, err := p.buildConversation(conversation)
	if err != nil {
		return nil, err
	}
	if len(msgs) == 0 {
		return nil, providers.Newf(providers.ErrKindInvalidRequest, p.name,
			"percakapan tidak memuat satu pun pesan user atau assistant yang berisi")
	}
	// Anthropic menuntut pesan pertama berperan user. Diperiksa di sini alih-alih
	// dibiarkan gagal di upstream: hasilnya sama-sama 400, tetapi pesan ini menyebut
	// masalah sebenarnya, dan satu perjalanan jaringan tidak terbuang.
	if msgs[0].Role != roleUser {
		return nil, providers.Newf(providers.ErrKindInvalidRequest, p.name,
			"pesan pertama harus berperan user; Anthropic menolak percakapan yang dibuka peran %s", msgs[0].Role)
	}

	body := messagesRequest{
		Model:         req.Model,
		Messages:      msgs,
		System:        system,
		StopSequences: req.Stop,
		Stream:        stream,
		Temperature:   req.Temperature,
		TopP:          req.TopP,
		TopK:          req.TopK,
	}

	body.MaxTokens = p.maxTokens
	if req.MaxTokens != nil && *req.MaxTokens > 0 {
		body.MaxTokens = *req.MaxTokens
	}

	// Extended thinking diaktifkan oleh permintaan eksplisit klien
	// (ChatRequest.Thinking, dari field thinking permukaan messages) atau oleh
	// ReasoningEffort kanonik. Yang eksplisit menang: ia membawa budget token
	// persis, dan menerjemahkannya dari effort akan mengubah angka itu.
	// Anthropic menolak temperature maupun top_p saat thinking menyala, dan
	// menuntut max_tokens lebih besar dari budget — keduanya diperbaiki di
	// sini agar permintaan yang sah di bentuk kanonik tidak berubah menjadi
	// 400 di upstream.
	budget := 0
	adaptive := false
	switch {
	case req.Thinking != nil && req.Thinking.Type == "enabled":
		budget = req.Thinking.BudgetTokens
		if budget < 1024 {
			// 1024 adalah budget minimum yang diterima Anthropic; enabled
			// tanpa budget (atau di bawahnya) dinaikkan ke minimum itu.
			budget = 1024
		}
	case req.Thinking != nil && req.Thinking.Type == "adaptive":
		// "adaptive" diteruskan APA ADANYA (B-096): maknanya model
		// memutuskan sendiri, jadi tidak ada budget yang boleh dikarang
		// dan tidak ada penyesuaian max_tokens berbasis budget. Ia tetap
		// thinking yang MENYALA, jadi perlakuan temperature/top_p di
		// bawah sama seperti jalur budget.
		adaptive = true
	case req.Thinking != nil && req.Thinking.Type == "disabled":
		// Mati secara eksplisit: ReasoningEffort tidak boleh menyalakannya
		// kembali di belakang keputusan klien.
		budget = 0
	default:
		budget = thinkingBudget(req.ReasoningEffort)
	}
	// Sub-field tak dikenal dari thinking eksplisit klien diteruskan setia
	// (B-137). Jalur ReasoningEffort dan "disabled" tidak membawa Extra:
	// yang pertama memang tidak punya sumbernya, yang kedua tidak mengirim
	// blok thinking sama sekali ke upstream.
	var thinkingExtra map[string]json.RawMessage
	if req.Thinking != nil {
		thinkingExtra = req.Thinking.Extra
	}
	if adaptive {
		body.Thinking = &thinkingConfig{Type: "adaptive", Extra: thinkingExtra}
		body.Temperature = nil
		body.TopP = nil
	} else if budget > 0 {
		body.Thinking = &thinkingConfig{Type: "enabled", BudgetTokens: budget, Extra: thinkingExtra}
		if body.MaxTokens <= budget {
			body.MaxTokens = budget + p.maxTokens
		}
		body.Temperature = nil
		body.TopP = nil
	} else if req.Temperature != nil && req.TopP != nil {
		// Anthropic menolak temperature dan top_p yang ditentukan BERSAMAAN.
		// Meneruskan keduanya apa adanya hanya memindahkan penolakan itu ke
		// upstream, dan membuang salah satunya diam-diam berarti menjalankan
		// parameter yang bukan parameter klien — jadi konfliknya ditolak
		// eksplisit di sini, sebelum jaringan disentuh (audit3 V-GW-31). Saat
		// thinking menyala cabang ini tidak dicapai: keduanya sudah distrip
		// di atas, persis seperti yang dituntut Anthropic.
		return nil, providers.Newf(providers.ErrKindInvalidRequest, p.name,
			"temperature dan top_p tidak boleh ditentukan bersamaan untuk Anthropic; pilih salah satu")
	}

	tools, err := p.buildTools(req.Tools)
	if err != nil {
		return nil, err
	}
	body.Tools = tools
	body.ToolChoice = toolChoiceFor(req.ToolChoice, req.ParallelToolCalls)

	if req.User != "" {
		// Anthropic membatasi user_id 256 karakter. Nilainya berasal dari klien, jadi
		// dipangkas alih-alih dibiarkan menggagalkan seluruh permintaan.
		body.Metadata = &upstreamMetadata{UserID: truncate(req.User, maxMetadataUserIDLen)}
	}

	// Field yang tidak punya padanan di Anthropic sengaja diabaikan, bukan diteruskan:
	// Anthropic menolak body dengan field asing, jadi meneruskan sisa parameter bergaya
	// OpenAI (ChatRequest.Extra, Seed, ResponseFormat) akan mengubah parameter yang
	// tidak berbahaya menjadi kegagalan 400.
	raw, err := json.Marshal(body)
	if err != nil {
		return nil, providers.Newf(providers.ErrKindInvalidRequest, p.name, "permintaan tidak bisa diserialisasi")
	}
	return raw, nil
}

// splitSystem memindahkan seluruh pesan system dan developer ke field system.
//
// Anthropic menolak peran system di dalam array messages, jadi tidak boleh ada satu pun
// yang tertinggal. Beberapa pesan digabung dengan baris kosong sebagai pemisah: masing-
// masing adalah blok instruksi tersendiri, dan menyambungnya tanpa jarak membuat kalimat
// terakhir satu blok melebur ke kalimat pertama blok berikutnya.
//
// Pesan system yang membawa bagian NON-TEKS ditolak dengan error
// (B-388): field system Anthropic hanya menerima teks, jadi bagian
// gambar/audio pada pesan system tidak punya padanan — membuangnya
// senyap (perilaku lama: Text() hanya memanen bagian teks)
// menghilangkan sebagian instruksi yang diminta klien tanpa jejak.
// Permukaan Messages sudah menolak bentuk ini di pintunya (B-359);
// jalur Chat sampai di sini tanpa pemeriksaan itu, jadi adapter
// adalah pertahanan terakhirnya.
func splitSystem(nama string, messages []providers.Message) (string, []providers.Message, error) {
	var parts []string
	rest := make([]providers.Message, 0, len(messages))
	for i := range messages {
		m := &messages[i]
		if m.Role != providers.RoleSystem && m.Role != providers.RoleDeveloper {
			rest = append(rest, *m)
			continue
		}
		for _, part := range m.Parts {
			if part.Type != providers.PartTypeText {
				return "", nil, providers.Newf(providers.ErrKindInvalidRequest, nama,
					"pesan %s memuat bagian %q yang tidak dapat diterjemahkan ke instruksi system teks", m.Role, part.Type)
			}
		}
		if text := m.Text(); text != "" {
			parts = append(parts, text)
		}
	}
	return strings.Join(parts, "\n\n"), rest, nil
}

// buildConversation menerjemahkan pesan kanonik menjadi pesan Anthropic yang berperan
// bergantian.
//
// Pesan berperan sama yang berurutan DIGABUNG, bukan ditolak. Dua alasan:
//
//  1. Penggabungan tidak menghilangkan apa pun. Isi satu pesan Anthropic memang berupa
//     daftar content block, jadi menyatukan dua pesan user berarti menyambung daftar
//     block-nya dengan urutan utuh — tidak ada teks yang dibuang atau ditulis ulang.
//     Menolak permintaan justru menghukum klien untuk sesuatu yang sah di bentuk kanonik
//     dan bisa diterjemahkan dengan setia.
//  2. Alur tool MEWAJIBKANNYA. Bentuk kanonik mengirim satu pesan berperan tool per
//     hasil, sedangkan Anthropic menuntut seluruh block tool_result untuk satu putaran
//     tool_use berada di SATU pesan user berikutnya. Tanpa penggabungan, percakapan
//     dengan dua tool paralel selalu ditolak upstream.
func (p *Provider) buildConversation(messages []providers.Message) ([]upstreamMessage, error) {
	out := make([]upstreamMessage, 0, len(messages))
	for i := range messages {
		m := &messages[i]

		role, blocks, err := p.blocksFor(m)
		if err != nil {
			return nil, err
		}
		if len(blocks) == 0 {
			// Anthropic menolak pesan tanpa isi dan block teks kosong, jadi pesan yang
			// tidak menyisakan apa pun setelah terjemahan dibuang di sini.
			continue
		}
		if n := len(out); n > 0 && out[n-1].Role == role {
			out[n-1].Content = append(out[n-1].Content, blocks...)
			continue
		}
		out = append(out, upstreamMessage{Role: role, Content: blocks})
	}
	return out, nil
}

// blocksFor menerjemahkan satu pesan kanonik menjadi peran Anthropic dan content block.
func (p *Provider) blocksFor(m *providers.Message) (string, []block, error) {
	switch m.Role {
	case providers.RoleUser:
		blocks, err := p.contentBlocks(m)
		return roleUser, blocks, err

	case providers.RoleAssistant:
		blocks, err := p.contentBlocks(m)
		if err != nil {
			return "", nil, err
		}
		// Block thinking riwayat dikirim ulang HANYA bila tanda tangannya ikut
		// terbawa (ReasoningSignature): Anthropic mengesahkan isi thinking dari
		// signature itu, dan mengirim thinking telanjang membuat seluruh
		// permintaan ditolak. Tanpa signature, jejak penalarannya dibuang —
		// tetapi isinya tidak hilang dari percakapan secara makna, karena
		// jawaban teks assistant tetap terkirim (audit3 V-GW-23).
		if m.ReasoningContent != "" && m.ReasoningSignature != "" {
			blocks = append([]block{{
				Type:      blockThinking,
				Thinking:  m.ReasoningContent,
				Signature: providers.SignatureToleran(m.ReasoningSignature),
			}}, blocks...)
		}
		for i := range m.ToolCalls {
			tc := &m.ToolCalls[i]
			input, err := p.toolInput(tc)
			if err != nil {
				return "", nil, err
			}
			blocks = append(blocks, block{
				Type:  blockToolUse,
				ID:    tc.ID,
				Name:  tc.Function.Name,
				Input: input,
			})
		}
		return roleAssistant, blocks, nil

	case providers.RoleTool:
		// Hasil tool bukan peran tersendiri di Anthropic: ia block tool_result di dalam
		// pesan user. Tanpa tool_use_id, Anthropic tidak bisa mengaitkannya ke panggilan
		// mana pun dan menolak permintaan.
		if m.ToolCallID == "" {
			return "", nil, providers.Newf(providers.ErrKindInvalidRequest, p.name,
				"pesan berperan tool tidak menyertakan tool_call_id, sehingga hasilnya tidak bisa dikaitkan ke panggilan tool")
		}
		result := block{Type: blockToolResult, ToolUseID: m.ToolCallID, IsError: m.IsError}
		if text := m.Text(); text != "" {
			result.Content = []block{{Type: blockText, Text: text}}
		}
		return roleUser, []block{result}, nil

	default:
		return "", nil, providers.Newf(providers.ErrKindInvalidRequest, p.name,
			"peran pesan %q tidak dikenal", string(m.Role))
	}
}

// contentBlocks menerjemahkan isi pesan, termasuk bentuk multimodal.
func (p *Provider) contentBlocks(m *providers.Message) ([]block, error) {
	if !m.IsMultimodal() {
		if m.Content == "" {
			return nil, nil
		}
		return []block{{Type: blockText, Text: m.Content}}, nil
	}

	blocks := make([]block, 0, len(m.Parts))
	for i := range m.Parts {
		part := &m.Parts[i]
		switch part.Type {
		case providers.PartTypeText:
			if part.Text == "" {
				continue
			}
			blocks = append(blocks, block{Type: blockText, Text: part.Text})

		case providers.PartTypeImageURL:
			if part.ImageURL == nil || part.ImageURL.URL == "" {
				continue
			}
			// Detail ("low"/"high") tidak punya padanan di Anthropic dan diabaikan.
			if mediaType, data, ok := parseDataURI(part.ImageURL.URL); ok {
				blocks = append(blocks, block{Type: blockImage, Source: &imageSource{
					Type: "base64", MediaType: mediaType, Data: data,
				}})
				continue
			}
			blocks = append(blocks, block{Type: blockImage, Source: &imageSource{
				Type: "url", URL: part.ImageURL.URL,
			}})

		case providers.PartTypeDocument:
			if part.Document == nil || part.Document.Data == "" {
				continue
			}
			// Dokumen diteruskan sebagai block document native — bentuk yang
			// sama persis dengan yang dikirim klien di permukaan Anthropic.
			// Inilah satu-satunya adapter yang bisa melakukannya; gateway
			// menyaring kandidat supaya bagian ini tidak pernah tiba di
			// adapter lain (Batch C2).
			blocks = append(blocks, block{Type: blockDocument, Source: &imageSource{
				Type: "base64", MediaType: part.Document.MediaType, Data: part.Document.Data,
			}})

		case providers.PartTypeAudio:
			// Anthropic tidak menerima masukan audio. Membuangnya diam-diam berarti model
			// menjawab pertanyaan tentang audio yang tidak pernah ia terima, jadi lebih
			// baik gagal dengan sebab yang jelas.
			return nil, providers.Newf(providers.ErrKindInvalidRequest, p.name,
				"Anthropic tidak menerima masukan audio; gunakan provider yang mendukung audio")

		default:
			// Jenis bagian baru diabaikan, sesuai kontrak: adapter tidak boleh menolak
			// permintaan hanya karena ada field yang belum dikenalnya.
			continue
		}
	}
	return blocks, nil
}

// toolInput menyiapkan field input block tool_use.
//
// Anthropic menuntut input berupa objek JSON. Argumen kosong menjadi objek kosong, tetapi
// argumen yang ada dan bukan objek TIDAK diperbaiki diam-diam: menggantinya dengan objek
// kosong membuat model membaca panggilan tool tanpa argumen, dan jawabannya akan salah
// tanpa jejak apa pun tentang penyebabnya.
func (p *Provider) toolInput(tc *providers.ToolCall) (json.RawMessage, error) {
	args := strings.TrimSpace(tc.Function.Arguments)
	if args == "" {
		return json.RawMessage(`{}`), nil
	}
	var probe map[string]json.RawMessage
	if err := json.Unmarshal([]byte(args), &probe); err != nil {
		return nil, providers.Newf(providers.ErrKindInvalidRequest, p.name,
			"argumen tool %q bukan objek JSON yang sah", tc.Function.Name)
	}
	return json.RawMessage(args), nil
}

// buildTools menerjemahkan tool kanonik menjadi bentuk Anthropic.
func (p *Provider) buildTools(tools []providers.Tool) ([]upstreamTool, error) {
	if len(tools) == 0 {
		return nil, nil
	}
	out := make([]upstreamTool, 0, len(tools))
	for i := range tools {
		t := &tools[i]
		// Jenis tool selain function (mis. code_interpreter milik OpenAI) tidak punya
		// padanan dan dilewati, bukan ditolak.
		if t.Type != "" && t.Type != "function" {
			continue
		}
		if t.Function.Name == "" {
			return nil, providers.Newf(providers.ErrKindInvalidRequest, p.name, "definisi tool tanpa nama")
		}
		// input_schema WAJIB ada di Anthropic, sementara parameters boleh kosong di
		// bentuk kanonik. Skema objek kosong berarti "tool tanpa argumen".
		schema := json.RawMessage(`{"type":"object","properties":{}}`)
		if len(bytes.TrimSpace(t.Function.Parameters)) > 0 {
			schema = t.Function.Parameters
		}
		out = append(out, upstreamTool{
			Name:        t.Function.Name,
			Description: t.Function.Description,
			InputSchema: schema,
		})
	}
	return out, nil
}

// toolChoiceFor menerjemahkan tool_choice bergaya OpenAI ke bentuk Anthropic.
func toolChoiceFor(raw json.RawMessage, parallel *bool) *upstreamChoice {
	choice := translateToolChoice(raw)

	// parallel_tool_calls=false hanya bisa disampaikan lewat objek tool_choice, jadi
	// objeknya dibuat dengan mode bawaan bila klien tidak menentukan pilihan tool.
	if parallel != nil && !*parallel {
		if choice == nil {
			choice = &upstreamChoice{Type: "auto"}
		}
		disable := true
		choice.DisableParallelToolUse = &disable
	}
	return choice
}

func translateToolChoice(raw json.RawMessage) *upstreamChoice {
	if len(bytes.TrimSpace(raw)) == 0 {
		return nil
	}
	var name string
	if err := json.Unmarshal(raw, &name); err == nil {
		switch name {
		case "auto":
			return &upstreamChoice{Type: "auto"}
		case "none":
			return &upstreamChoice{Type: "none"}
		case "required", "any":
			return &upstreamChoice{Type: "any"}
		default:
			return nil
		}
	}

	var obj struct {
		Type     string `json:"type"`
		Name     string `json:"name"`
		Function struct {
			Name string `json:"name"`
		} `json:"function"`
	}
	if err := json.Unmarshal(raw, &obj); err != nil {
		return nil
	}
	switch obj.Type {
	case "function", "tool":
		target := obj.Function.Name
		if target == "" {
			target = obj.Name
		}
		if target == "" {
			return nil
		}
		return &upstreamChoice{Type: "tool", Name: target}
	case "auto":
		return &upstreamChoice{Type: "auto"}
	case "none":
		return &upstreamChoice{Type: "none"}
	case "any", "required":
		return &upstreamChoice{Type: "any"}
	default:
		return nil
	}
}

// thinkingBudget menerjemahkan ReasoningEffort kanonik menjadi budget_tokens.
//
// Nol berarti extended thinking tidak diaktifkan. 1024 adalah batas minimum yang diterima
// Anthropic, jadi effort terendah pun tidak boleh di bawah itu.
func thinkingBudget(effort string) int {
	switch strings.ToLower(strings.TrimSpace(effort)) {
	case "minimal", "low":
		return 1024
	case "medium":
		return 4096
	case "high":
		return 8192
	default:
		return 0
	}
}

// parseDataURI memisahkan media type dan muatan base64 dari sebuah data URI.
func parseDataURI(raw string) (mediaType, data string, ok bool) {
	if !strings.HasPrefix(raw, "data:") {
		return "", "", false
	}
	meta, payload, found := strings.Cut(raw[len("data:"):], ",")
	if !found || payload == "" {
		return "", "", false
	}
	if !strings.HasSuffix(meta, ";base64") {
		// Data URI tanpa base64 (persen-encoded) tidak dipakai klien mana pun untuk
		// gambar dan tidak diterima Anthropic.
		return "", "", false
	}
	meta = strings.TrimSuffix(meta, ";base64")
	// Parameter tambahan seperti ";charset=utf-8" dibuang; hanya media type yang dipakai.
	if i := strings.IndexByte(meta, ';'); i >= 0 {
		meta = meta[:i]
	}
	if meta == "" {
		meta = "application/octet-stream"
	}
	return meta, payload, true
}

func truncate(s string, n int) string {
	if len(s) <= n {
		return s
	}
	return s[:n]
}

// toCanonicalUsage menerjemahkan usage Anthropic ke bentuk kanonik.
//
// Dua keputusan penting:
//
//   - InputTokens dijadikan input_tokens + cache_read + cache_creation. Anthropic
//     melaporkan ketiganya terpisah, sedangkan bentuk kanonik mengikuti OpenAI, yang
//     prompt_tokens-nya SUDAH memuat token cache dan menyebut cached_tokens sebagai
//     bagiannya. Kalau cache dibiarkan di luar InputTokens, perhitungan biaya yang
//     mengurangi CachedInputTokens dari InputTokens bisa menghasilkan angka negatif —
//     token cache read rutin melebihi input segar pada percakapan panjang.
//   - TotalTokens dijumlahkan sendiri. Anthropic tidak punya field total, dan menyalin
//     input_tokens + output_tokens saja akan melewatkan seluruh token cache.
func toCanonicalUsage(u upstreamUsage) providers.Usage {
	// B-352: penjumlahan memakai TambahToken yang menjenuhkan —
	// field-field ini datang dari JSON upstream tanpa batas dekode,
	// dan jumlah polosnya bisa membungkus negatif melewati MaxInt.
	input := providers.TambahToken(providers.TambahToken(u.InputTokens, u.CacheReadInputTokens), u.CacheCreationInputTokens)
	out := providers.Usage{
		InputTokens:       input,
		CachedInputTokens: u.CacheReadInputTokens,
		// Ikut dibawa terpisah supaya penagihan bisa mengenakan premi tulis
		// cache 1,25×; melipatnya ke InputTokens saja menagihnya sebagai input
		// biasa 1,0× (audit3 V-AB-11).
		CacheCreationInputTokens: u.CacheCreationInputTokens,
		OutputTokens:             u.OutputTokens,
		TotalTokens:              providers.TambahToken(input, u.OutputTokens),
	}
	// Rincian TTL tulis cache ikut dibawa bila dilaporkan: penagihan
	// membedakan tulis 1 jam (2×) dari 5 menit (1,25×). Tanpa objek
	// cache_creation kedua field tetap nol dan seluruh creation jatuh ke
	// tarif 5 menit (sisa B-011).
	if u.CacheCreation != nil {
		out.CacheCreation1hInputTokens = u.CacheCreation.Ephemeral1hInputTokens
		out.CacheCreation5mInputTokens = u.CacheCreation.Ephemeral5mInputTokens
	}
	return out
}

// mapStopReason menerjemahkan stop_reason Anthropic ke bentuk kanonik.
func mapStopReason(reason string) string {
	switch reason {
	case "":
		return ""
	case "max_tokens":
		return providers.FinishLength
	case "tool_use":
		return providers.FinishToolCalls
	case "refusal":
		return providers.FinishContentFilter
	case "stop_sequence":
		// TIDAK dilipat ke FinishStop: permukaan Anthropic membedakannya, dan
		// nilai urutannya sendiri dibawa Choice.StopSequence (audit3 V-GW-28).
		return providers.FinishStopSequence
	case "pause_turn":
		return providers.FinishPauseTurn
	default:
		// end_turn dan alasan baru apa pun: dari sisi klien artinya sama —
		// model berhenti dengan sendirinya.
		return providers.FinishStop
	}
}

// pesanKanonikKosong melaporkan apakah pesan kanonik tidak memuat apa
// pun yang bisa diteruskan ke klien — tanpa teks, tanpa bagian, tanpa
// tool call, tanpa penalaran, tanpa penolakan, tanpa transkrip audio
// (B-261). Signature penalaran sendirian BUKAN konten: ia hanya sah
// menemani teks penalarannya.
func pesanKanonikKosong(m providers.Message) bool {
	return m.Content == "" && len(m.Parts) == 0 && len(m.ToolCalls) == 0 &&
		m.ReasoningContent == "" && m.Refusal == "" && m.AudioTranscript == ""
}

// toCanonicalResponse menerjemahkan respons Anthropic ke bentuk kanonik.
func (p *Provider) toCanonicalResponse(u *messagesResponse) *providers.ChatResponse {
	var text, reasoning strings.Builder
	var calls []providers.ToolCall
	var signature string
	jumlahThinking := 0

	// parts/adaGambar sejajar pola B-211 di adapter Google: urutan block
	// dipertahankan utuh (termasuk segmen teksnya) dan Parts hanya
	// terpasang bila ada block gambar (B-299).
	var parts []providers.ContentPart
	adaGambar := false

	for i := range u.Content {
		b := &u.Content[i]
		switch b.Type {
		case blockText:
			// Disambung tanpa pemisah: Anthropic memecah teks menjadi beberapa block di
			// sekitar tool_use, dan menambahkan baris baru berarti mengarang karakter
			// yang tidak pernah dihasilkan model.
			text.WriteString(b.Text)
			parts = append(parts, providers.ContentPart{Type: providers.PartTypeText, Text: b.Text})
		case blockImage:
			// B-299: block image sebelumnya jatuh menembus switch ini
			// tanpa case — hilang senyap sebelum pemetaan kanonik,
			// sehingga respons berisi gambar saja tertangkap penjaga
			// kekosongan B-261 dengan alasan yang salah ("kosong",
			// padahal isinya gambar). Kini diterjemahkan ke bagian
			// gambar kanonik: sumber base64 menjadi data URI, sumber
			// url diteruskan apa adanya — kebalikan persis arah
			// permintaan di toBlocks.
			if b.Source == nil {
				continue
			}
			var url string
			switch {
			case b.Source.Type == "url" && b.Source.URL != "":
				url = b.Source.URL
			case b.Source.Data != "":
				url = "data:" + b.Source.MediaType + ";base64," + b.Source.Data
			default:
				continue
			}
			parts = append(parts, providers.ContentPart{
				Type:     providers.PartTypeImageURL,
				ImageURL: &providers.ImageURL{URL: url},
			})
			adaGambar = true
		case blockThinking:
			reasoning.WriteString(b.Thinking)
			jumlahThinking++
			signature = string(b.Signature)
		case blockRedactedThink:
			// KEPUTUSAN TERCATAT (B-416, dipertahankan sesudah
			// diperiksa ulang di Putaran Bersih #5): block ini
			// dibuang dengan sadar. Muatannya terenkripsi dan
			// hanya berguna bila dikirim balik ke Anthropic apa
			// adanya; bentuk kanonik tidak punya slot untuk
			// muatan terenkripsi per-block, dan tidak ada yang
			// bisa ditampilkan ke klien. Meneruskannya menuntut
			// slot kanonik baru beserta aturan kirim-baliknya di
			// semua adapter — perubahan kontrak, bukan perbaikan
			// kebocoran. Yang WAJIB tetap utuh adalah block teks
			// pendampingnya; penguncinya TestB416. Jalur streaming
			// mengambil keputusan yang sama (stream.go, blockStart
			// tanpa cabang block ini).
			continue
		case blockToolUse:
			args := "{}"
			if len(bytes.TrimSpace(b.Input)) > 0 {
				args = string(b.Input)
			}
			calls = append(calls, providers.ToolCall{
				Index:    len(calls),
				ID:       b.ID,
				Type:     "function",
				Function: providers.FunctionCall{Name: b.Name, Arguments: args},
			})
		}
	}
	if jumlahThinking != 1 {
		// Tanda tangan hanya sah untuk block-nya sendiri; gabungan beberapa
		// block tidak bisa dipasangi salah satu tanda tangan mereka.
		signature = ""
	}

	pesan := providers.Message{
		Role:               providers.RoleAssistant,
		Content:            text.String(),
		ToolCalls:          calls,
		ReasoningContent:   reasoning.String(),
		ReasoningSignature: signature,
	}
	if adaGambar {
		// Hanya terpasang bila ada gambar (B-299): respons teks murni
		// mempertahankan bentuk persis seperti sebelumnya, dan
		// Message.Text() membaca gabungan segmen dari Parts bila
		// terisi — isinya identik dengan Content di atas.
		pesan.Parts = parts
	}

	resp := &providers.ChatResponse{
		ID:    u.ID,
		Model: u.Model,
		// Anthropic tidak mengirim timestamp pembuatan, sedangkan bentuk kanonik dan
		// klien berdialek OpenAI mengharapkannya. Waktu penerimaan adalah perkiraan
		// terdekat yang tersedia.
		Created: time.Now().Unix(),
		Choices: []providers.Choice{{
			Index:        0,
			Message:      pesan,
			FinishReason: mapStopReason(u.StopReason),
			StopSequence: u.StopSequence,
		}},
		Usage: toCanonicalUsage(u.Usage),
	}
	resp.Raw = encodeCompletion(resp)
	return resp
}
