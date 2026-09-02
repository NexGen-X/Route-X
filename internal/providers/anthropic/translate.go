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
	Text      string `json:"text,omitempty"`
	Thinking  string `json:"thinking,omitempty"`
	Signature string `json:"signature,omitempty"`

	// image
	Source *imageSource `json:"source,omitempty"`

	// tool_use
	ID    string          `json:"id,omitempty"`
	Name  string          `json:"name,omitempty"`
	Input json.RawMessage `json:"input,omitempty"`

	// tool_result
	ToolUseID string  `json:"tool_use_id,omitempty"`
	Content   []block `json:"content,omitempty"`
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
}

// messagesResponse adalah respons non-streaming /v1/messages.
type messagesResponse struct {
	ID         string        `json:"id"`
	Type       string        `json:"type"`
	Role       string        `json:"role"`
	Model      string        `json:"model"`
	Content    []block       `json:"content"`
	StopReason string        `json:"stop_reason"`
	Usage      upstreamUsage `json:"usage"`
}

// buildMessagesRequest menerjemahkan permintaan kanonik menjadi body Anthropic.
func (p *Provider) buildMessagesRequest(req *providers.ChatRequest, stream bool) ([]byte, error) {
	if req == nil {
		return nil, providers.Newf(providers.ErrKindInvalidRequest, p.name, "permintaan kosong")
	}
	if strings.TrimSpace(req.Model) == "" {
		return nil, providers.Newf(providers.ErrKindInvalidRequest, p.name, "nama model wajib diisi")
	}

	system, conversation := splitSystem(req.Messages)
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
	}

	body.MaxTokens = p.maxTokens
	if req.MaxTokens != nil && *req.MaxTokens > 0 {
		body.MaxTokens = *req.MaxTokens
	}

	// Penalaran diaktifkan hanya bila klien memintanya lewat ReasoningEffort. Anthropic
	// menolak temperature maupun top_p saat extended thinking menyala, dan menuntut
	// max_tokens lebih besar dari budget — keduanya diperbaiki di sini agar permintaan
	// yang sah di bentuk kanonik tidak berubah menjadi 400 di upstream.
	if budget := thinkingBudget(req.ReasoningEffort); budget > 0 {
		body.Thinking = &thinkingConfig{Type: "enabled", BudgetTokens: budget}
		if body.MaxTokens <= budget {
			body.MaxTokens = budget + p.maxTokens
		}
		body.Temperature = nil
		body.TopP = nil
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
func splitSystem(messages []providers.Message) (string, []providers.Message) {
	var parts []string
	rest := make([]providers.Message, 0, len(messages))
	for i := range messages {
		m := &messages[i]
		if m.Role != providers.RoleSystem && m.Role != providers.RoleDeveloper {
			rest = append(rest, *m)
			continue
		}
		if text := m.Text(); text != "" {
			parts = append(parts, text)
		}
	}
	return strings.Join(parts, "\n\n"), rest
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
		// ReasoningContent sengaja TIDAK dikirim ulang. Anthropic hanya menerima block
		// thinking bila disertai signature kriptografisnya, dan bentuk kanonik tidak
		// membawa signature itu — mengirimkannya tanpa signature membuat seluruh
		// permintaan ditolak.
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
		result := block{Type: blockToolResult, ToolUseID: m.ToolCallID}
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
	input := u.InputTokens + u.CacheReadInputTokens + u.CacheCreationInputTokens
	return providers.Usage{
		InputTokens:       input,
		CachedInputTokens: u.CacheReadInputTokens,
		OutputTokens:      u.OutputTokens,
		TotalTokens:       input + u.OutputTokens,
	}
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
	default:
		// end_turn, stop_sequence, pause_turn, dan alasan baru apa pun: dari sisi klien
		// artinya sama — model berhenti dengan sendirinya.
		return providers.FinishStop
	}
}

// toCanonicalResponse menerjemahkan respons Anthropic ke bentuk kanonik.
func (p *Provider) toCanonicalResponse(u *messagesResponse) *providers.ChatResponse {
	var text, reasoning strings.Builder
	var calls []providers.ToolCall

	for i := range u.Content {
		b := &u.Content[i]
		switch b.Type {
		case blockText:
			// Disambung tanpa pemisah: Anthropic memecah teks menjadi beberapa block di
			// sekitar tool_use, dan menambahkan baris baru berarti mengarang karakter
			// yang tidak pernah dihasilkan model.
			text.WriteString(b.Text)
		case blockThinking:
			reasoning.WriteString(b.Thinking)
		case blockRedactedThink:
			// Muatannya terenkripsi dan hanya berguna bila dikirim balik ke Anthropic;
			// tidak ada yang bisa ditampilkan ke klien.
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

	resp := &providers.ChatResponse{
		ID:    u.ID,
		Model: u.Model,
		// Anthropic tidak mengirim timestamp pembuatan, sedangkan bentuk kanonik dan
		// klien berdialek OpenAI mengharapkannya. Waktu penerimaan adalah perkiraan
		// terdekat yang tersedia.
		Created: time.Now().Unix(),
		Choices: []providers.Choice{{
			Index: 0,
			Message: providers.Message{
				Role:             providers.RoleAssistant,
				Content:          text.String(),
				ToolCalls:        calls,
				ReasoningContent: reasoning.String(),
			},
			FinishReason: mapStopReason(u.StopReason),
		}},
		Usage: toCanonicalUsage(u.Usage),
	}
	resp.Raw = encodeCompletion(resp)
	return resp
}
