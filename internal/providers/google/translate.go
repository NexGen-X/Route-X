package google

import (
	"bytes"
	"encoding/json"
	"fmt"
	"strings"
	"time"

	"github.com/NexGen-X/Route-X/internal/providers"
)

// Peran yang dikenal Gemini. Tidak ada "assistant" maupun "tool": jawaban model berperan
// "model", dan hasil tool dikirim sebagai part functionResponse di dalam peran "user".
const (
	roleUser  = "user"
	roleModel = "model"
)

// generateRequest adalah body POST :generateContent.
type generateRequest struct {
	Contents          []geminiContent `json:"contents"`
	SystemInstruction *geminiContent  `json:"systemInstruction,omitempty"`
	GenerationConfig  *genConfig      `json:"generationConfig,omitempty"`
	Tools             []geminiTool    `json:"tools,omitempty"`
	ToolConfig        *toolConfig     `json:"toolConfig,omitempty"`
}

type geminiContent struct {
	Role  string       `json:"role,omitempty"`
	Parts []geminiPart `json:"parts"`
}

type geminiPart struct {
	Text string `json:"text,omitempty"`
	// Thought menandai part berisi ringkasan penalaran, bukan jawaban.
	Thought          bool                `json:"thought,omitempty"`
	InlineData       *geminiBlob         `json:"inlineData,omitempty"`
	FileData         *geminiFileData     `json:"fileData,omitempty"`
	FunctionCall     *geminiFunctionCall `json:"functionCall,omitempty"`
	FunctionResponse *geminiFunctionResp `json:"functionResponse,omitempty"`
}

type geminiBlob struct {
	MimeType string `json:"mimeType"`
	Data     string `json:"data"`
}

type geminiFileData struct {
	MimeType string `json:"mimeType,omitempty"`
	FileURI  string `json:"fileUri"`
}

type geminiFunctionCall struct {
	Name string          `json:"name"`
	Args json.RawMessage `json:"args,omitempty"`
}

type geminiFunctionResp struct {
	Name     string          `json:"name"`
	Response json.RawMessage `json:"response"`
}

type genConfig struct {
	MaxOutputTokens  *int            `json:"maxOutputTokens,omitempty"`
	Temperature      *float64        `json:"temperature,omitempty"`
	TopP             *float64        `json:"topP,omitempty"`
	StopSequences    []string        `json:"stopSequences,omitempty"`
	Seed             *int            `json:"seed,omitempty"`
	ResponseMimeType string          `json:"responseMimeType,omitempty"`
	ResponseSchema   json.RawMessage `json:"responseSchema,omitempty"`
	ThinkingConfig   *thinkingConfig `json:"thinkingConfig,omitempty"`
}

type thinkingConfig struct {
	IncludeThoughts bool `json:"includeThoughts,omitempty"`
	ThinkingBudget  *int `json:"thinkingBudget,omitempty"`
}

type geminiTool struct {
	FunctionDeclarations []functionDeclaration `json:"functionDeclarations,omitempty"`
}

type functionDeclaration struct {
	Name        string          `json:"name"`
	Description string          `json:"description,omitempty"`
	Parameters  json.RawMessage `json:"parameters,omitempty"`
}

type toolConfig struct {
	FunctionCallingConfig *functionCallingConfig `json:"functionCallingConfig,omitempty"`
}

type functionCallingConfig struct {
	Mode                 string   `json:"mode,omitempty"`
	AllowedFunctionNames []string `json:"allowedFunctionNames,omitempty"`
}

// generateResponse adalah respons :generateContent, dan juga bentuk setiap peristiwa pada
// aliran alt=sse — Gemini mengirim objek respons UTUH per peristiwa, bukan delta.
type generateResponse struct {
	Candidates     []geminiCandidate `json:"candidates"`
	PromptFeedback *promptFeedback   `json:"promptFeedback,omitempty"`
	UsageMetadata  *usageMetadata    `json:"usageMetadata,omitempty"`
	ModelVersion   string            `json:"modelVersion,omitempty"`
	ResponseID     string            `json:"responseId,omitempty"`
	// Error muncul bila Gemini menyisipkan kegagalan ke tengah aliran.
	Error *struct {
		Code    int    `json:"code"`
		Message string `json:"message"`
		Status  string `json:"status"`
	} `json:"error,omitempty"`
}

type geminiCandidate struct {
	Content      geminiContent `json:"content"`
	FinishReason string        `json:"finishReason,omitempty"`
	Index        int           `json:"index"`
}

type promptFeedback struct {
	BlockReason string `json:"blockReason,omitempty"`
}

type usageMetadata struct {
	PromptTokenCount        int `json:"promptTokenCount"`
	CandidatesTokenCount    int `json:"candidatesTokenCount"`
	CachedContentTokenCount int `json:"cachedContentTokenCount"`
	ThoughtsTokenCount      int `json:"thoughtsTokenCount"`
	TotalTokenCount         int `json:"totalTokenCount"`
}

type embedRequest struct {
	Model                string        `json:"model,omitempty"`
	Content              geminiContent `json:"content"`
	OutputDimensionality *int          `json:"outputDimensionality,omitempty"`
}

type batchEmbedRequest struct {
	Requests []embedRequest `json:"requests"`
}

type embeddingValues struct {
	Values []float32 `json:"values"`
}

// buildGenerateRequest menerjemahkan permintaan kanonik menjadi body Gemini.
func (p *Provider) buildGenerateRequest(req *providers.ChatRequest) ([]byte, error) {
	system, conversation := splitSystem(req.Messages)
	contents, err := p.buildContents(conversation)
	if err != nil {
		return nil, err
	}
	if len(contents) == 0 {
		return nil, providers.Newf(providers.ErrKindInvalidRequest, p.name,
			"percakapan tidak memuat satu pun pesan berisi selain system")
	}

	body := generateRequest{Contents: contents}
	if system != "" {
		// systemInstruction adalah field tingkat atas: peran system TIDAK boleh tertinggal
		// di contents, karena Gemini hanya mengenal "user" dan "model" di sana.
		body.SystemInstruction = &geminiContent{Parts: []geminiPart{{Text: system}}}
	}
	body.GenerationConfig = generationConfigFor(req)

	tools, err := p.buildTools(req.Tools)
	if err != nil {
		return nil, err
	}
	body.Tools = tools
	body.ToolConfig = toolConfigFor(req.ToolChoice)

	// Field kanonik tanpa padanan (User, ParallelToolCalls, ChatRequest.Extra) diabaikan,
	// bukan diteruskan: Gemini menolak body dengan field asing, jadi meneruskan sisa
	// parameter bergaya OpenAI akan mengubah parameter tak berbahaya menjadi 400.
	raw, err := json.Marshal(body)
	if err != nil {
		return nil, providers.Newf(providers.ErrKindInvalidRequest, p.name, "permintaan tidak bisa diserialisasi")
	}
	return raw, nil
}

// splitSystem memindahkan pesan system dan developer ke systemInstruction.
//
// Beberapa pesan digabung dengan baris kosong sebagai pemisah: masing-masing adalah blok
// instruksi tersendiri, dan menyambungnya tanpa jarak melebur kalimat antar blok.
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

// buildContents menerjemahkan pesan kanonik menjadi contents Gemini.
//
// Content berperan sama yang berurutan digabung. Gemini lebih longgar daripada Anthropic
// soal ini, tetapi penggabungan tetap perlu untuk alur tool: bentuk kanonik mengirim satu
// pesan per hasil tool, sedangkan seluruh functionResponse dari satu putaran seharusnya
// berada dalam satu content di belakang functionCall-nya.
func (p *Provider) buildContents(messages []providers.Message) ([]geminiContent, error) {
	// Nama fungsi diambil dari pesan assistant yang memuat panggilannya, karena
	// functionResponse Gemini dikaitkan lewat NAMA — Gemini tidak punya id panggilan.
	names := toolNameIndex(messages)

	out := make([]geminiContent, 0, len(messages))
	for i := range messages {
		m := &messages[i]
		role, parts, err := p.partsFor(m, names)
		if err != nil {
			return nil, err
		}
		if len(parts) == 0 {
			continue
		}
		if n := len(out); n > 0 && out[n-1].Role == role {
			out[n-1].Parts = append(out[n-1].Parts, parts...)
			continue
		}
		out = append(out, geminiContent{Role: role, Parts: parts})
	}
	return out, nil
}

// toolNameIndex memetakan id panggilan tool ke nama fungsinya.
func toolNameIndex(messages []providers.Message) map[string]string {
	names := make(map[string]string)
	for i := range messages {
		for j := range messages[i].ToolCalls {
			tc := &messages[i].ToolCalls[j]
			if tc.ID != "" && tc.Function.Name != "" {
				names[tc.ID] = tc.Function.Name
			}
		}
	}
	return names
}

// partsFor menerjemahkan satu pesan kanonik menjadi peran Gemini dan daftar part.
func (p *Provider) partsFor(m *providers.Message, names map[string]string) (string, []geminiPart, error) {
	switch m.Role {
	case providers.RoleUser:
		parts, err := p.contentParts(m)
		return roleUser, parts, err

	case providers.RoleAssistant:
		parts, err := p.contentParts(m)
		if err != nil {
			return "", nil, err
		}
		// ReasoningContent tidak dikirim ulang: part ber-thought hanya bermakna sebagai
		// keluaran model, dan Gemini tidak menerimanya sebagai riwayat.
		for i := range m.ToolCalls {
			tc := &m.ToolCalls[i]
			args, err := p.functionArgs(tc)
			if err != nil {
				return "", nil, err
			}
			parts = append(parts, geminiPart{FunctionCall: &geminiFunctionCall{
				Name: tc.Function.Name,
				Args: args,
			}})
		}
		return roleModel, parts, nil

	case providers.RoleTool:
		// Inilah tempat ketiadaan id panggilan di Gemini harus dijembatani: bentuk kanonik
		// mengikat hasil ke panggilan lewat tool_call_id, Gemini lewat nama fungsi.
		name := names[m.ToolCallID]
		if name == "" {
			// Cadangan: id yang kami sintesis sendiri memuat nama fungsinya, jadi masih
			// bisa dipulihkan walau klien tidak mengirim ulang pesan assistant-nya.
			name = functionNameFromCallID(m.ToolCallID)
		}
		if name == "" {
			return "", nil, providers.Newf(providers.ErrKindInvalidRequest, p.name,
				"hasil tool dengan tool_call_id %q tidak bisa dikaitkan ke nama fungsi mana pun; sertakan pesan assistant yang memuat panggilan tool tersebut", m.ToolCallID)
		}
		return roleUser, []geminiPart{{FunctionResponse: &geminiFunctionResp{
			Name:     name,
			Response: functionResponsePayload(m.Text()),
		}}}, nil

	default:
		return "", nil, providers.Newf(providers.ErrKindInvalidRequest, p.name,
			"peran pesan %q tidak dikenal", string(m.Role))
	}
}

// contentParts menerjemahkan isi pesan, termasuk bentuk multimodal.
func (p *Provider) contentParts(m *providers.Message) ([]geminiPart, error) {
	if !m.IsMultimodal() {
		if m.Content == "" {
			return nil, nil
		}
		return []geminiPart{{Text: m.Content}}, nil
	}

	parts := make([]geminiPart, 0, len(m.Parts))
	for i := range m.Parts {
		part := &m.Parts[i]
		switch part.Type {
		case providers.PartTypeText:
			if part.Text == "" {
				continue
			}
			parts = append(parts, geminiPart{Text: part.Text})

		case providers.PartTypeImageURL:
			if part.ImageURL == nil || part.ImageURL.URL == "" {
				continue
			}
			if mimeType, data, ok := parseDataURI(part.ImageURL.URL); ok {
				parts = append(parts, geminiPart{InlineData: &geminiBlob{MimeType: mimeType, Data: data}})
				continue
			}
			// URL biasa diteruskan sebagai fileData. Gemini hanya menerima URI Files API
			// atau gs://, jadi tautan https sembarang akan ditolak upstream — dan itu
			// disengaja: mengunduhnya sendiri akan menjadikan gateway pengambil URL
			// pilihan klien, yakni tepat SSRF yang dijaga di seluruh repositori ini.
			parts = append(parts, geminiPart{FileData: &geminiFileData{FileURI: part.ImageURL.URL}})

		case providers.PartTypeAudio:
			if part.Audio == nil || part.Audio.Data == "" {
				continue
			}
			// Berbeda dari Anthropic, Gemini menerima audio sebagai inlineData.
			parts = append(parts, geminiPart{InlineData: &geminiBlob{
				MimeType: "audio/" + strings.TrimPrefix(part.Audio.Format, "audio/"),
				Data:     part.Audio.Data,
			}})

		default:
			// Jenis bagian baru diabaikan: adapter tidak boleh menolak permintaan hanya
			// karena ada field yang belum dikenalnya.
			continue
		}
	}
	return parts, nil
}

// functionArgs menyiapkan field args part functionCall.
//
// Gemini menuntut objek. Argumen kosong menjadi objek kosong, tetapi argumen yang ada dan
// bukan objek tidak diperbaiki diam-diam: model akan membaca panggilan tanpa argumen dan
// menjawab salah tanpa jejak penyebabnya.
func (p *Provider) functionArgs(tc *providers.ToolCall) (json.RawMessage, error) {
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

// functionResponsePayload membungkus hasil tool menjadi objek JSON.
//
// Hasil tool di bentuk kanonik berupa teks bebas, sedangkan functionResponse.response
// Gemini harus objek. Teks yang sudah berupa objek JSON dipakai apa adanya; sisanya
// dibungkus di bawah "output" agar isinya tetap sampai utuh ke model.
func functionResponsePayload(text string) json.RawMessage {
	trimmed := strings.TrimSpace(text)
	if trimmed != "" {
		var probe map[string]json.RawMessage
		if json.Unmarshal([]byte(trimmed), &probe) == nil {
			return json.RawMessage(trimmed)
		}
	}
	wrapped, err := json.Marshal(map[string]string{"output": text})
	if err != nil {
		return json.RawMessage(`{}`)
	}
	return wrapped
}

// synthesizeCallID membuat id panggilan tool yang tidak dimiliki Gemini.
//
// Bentuk kanonik dan klien berdialek OpenAI mengikat hasil tool ke panggilannya lewat id,
// sedangkan Gemini hanya mengirim nama fungsi. Tanpa id buatan, klien tidak punya apa pun
// untuk dikirim balik sebagai tool_call_id, dan dua panggilan ke fungsi yang sama dalam
// satu putaran tidak bisa dibedakan.
//
// Formatnya "call_<urutan>_<nama>". Urutan panggilan membuatnya unik walau namanya sama,
// dan nama fungsi ikut tertanam sehingga masih bisa dipulihkan dari id saja — jalur
// cadangan bila klien mengirim hasil tanpa menyertakan ulang pesan assistant-nya.
func synthesizeCallID(seq int, name string) string {
	return fmt.Sprintf("call_%d_%s", seq, name)
}

// functionNameFromCallID memulihkan nama fungsi dari id buatan synthesizeCallID.
// Mengembalikan string kosong bila id tidak berbentuk demikian.
func functionNameFromCallID(id string) string {
	rest, ok := strings.CutPrefix(id, "call_")
	if !ok {
		return ""
	}
	seq, name, ok := strings.Cut(rest, "_")
	if !ok || seq == "" || name == "" {
		return ""
	}
	for _, r := range seq {
		if r < '0' || r > '9' {
			return ""
		}
	}
	// Nama fungsi boleh memuat "_" — hanya urutannya yang dipotong di pemisah pertama.
	return name
}

// generationConfigFor mengumpulkan parameter generasi kanonik ke generationConfig.
func generationConfigFor(req *providers.ChatRequest) *genConfig {
	cfg := &genConfig{
		MaxOutputTokens: req.MaxTokens,
		Temperature:     req.Temperature,
		TopP:            req.TopP,
		StopSequences:   req.Stop,
		Seed:            req.Seed,
		ThinkingConfig:  thinkingFor(req.ReasoningEffort),
	}
	cfg.ResponseMimeType, cfg.ResponseSchema = responseFormatFor(req.ResponseFormat)

	if cfg.MaxOutputTokens == nil && cfg.Temperature == nil && cfg.TopP == nil &&
		len(cfg.StopSequences) == 0 && cfg.Seed == nil && cfg.ResponseMimeType == "" &&
		cfg.ThinkingConfig == nil {
		return nil
	}
	return cfg
}

// responseFormatFor menerjemahkan response_format bergaya OpenAI ke bentuk Gemini.
func responseFormatFor(raw json.RawMessage) (string, json.RawMessage) {
	if len(bytes.TrimSpace(raw)) == 0 {
		return "", nil
	}
	var format struct {
		Type       string `json:"type"`
		JSONSchema struct {
			Schema json.RawMessage `json:"schema"`
		} `json:"json_schema"`
	}
	if err := json.Unmarshal(raw, &format); err != nil {
		return "", nil
	}
	switch format.Type {
	case "json_object":
		return "application/json", nil
	case "json_schema":
		if len(bytes.TrimSpace(format.JSONSchema.Schema)) == 0 {
			return "application/json", nil
		}
		return "application/json", sanitizeSchema(format.JSONSchema.Schema)
	default:
		// "text" dan bentuk baru lainnya: biarkan bawaan Gemini.
		return "", nil
	}
}

// thinkingFor menerjemahkan ReasoningEffort kanonik menjadi thinkingConfig.
//
// Hanya diaktifkan bila klien memintanya secara eksplisit. Bila model yang dituju tidak
// mendukung thinking, Gemini akan menolak dengan sebab yang jelas — itu jawaban yang benar
// untuk permintaan yang memang meminta penalaran.
func thinkingFor(effort string) *thinkingConfig {
	budget := 0
	switch strings.ToLower(strings.TrimSpace(effort)) {
	case "minimal":
		// 0 mematikan thinking; itulah arti terdekat dari "minimal".
		budget = 0
	case "low":
		budget = 1024
	case "medium":
		budget = 8192
	case "high":
		budget = 24576
	default:
		return nil
	}
	return &thinkingConfig{IncludeThoughts: budget > 0, ThinkingBudget: &budget}
}

// buildTools menerjemahkan tool kanonik menjadi functionDeclarations.
func (p *Provider) buildTools(tools []providers.Tool) ([]geminiTool, error) {
	if len(tools) == 0 {
		return nil, nil
	}
	decls := make([]functionDeclaration, 0, len(tools))
	for i := range tools {
		t := &tools[i]
		// Jenis tool selain function tidak punya padanan dan dilewati, bukan ditolak.
		if t.Type != "" && t.Type != "function" {
			continue
		}
		if t.Function.Name == "" {
			return nil, providers.Newf(providers.ErrKindInvalidRequest, p.name, "definisi tool tanpa nama")
		}
		decls = append(decls, functionDeclaration{
			Name:        t.Function.Name,
			Description: t.Function.Description,
			// Gemini memakai "parameters" bergaya OpenAPI, bukan "function.parameters"
			// bergaya OpenAI, dan menolak beberapa kata kunci JSON Schema.
			Parameters: sanitizeSchema(t.Function.Parameters),
		})
	}
	if len(decls) == 0 {
		return nil, nil
	}
	// Seluruh deklarasi dikumpulkan dalam SATU entri tools: Gemini memperlakukan tiap
	// entri sebagai satu jenis tool, bukan satu fungsi.
	return []geminiTool{{FunctionDeclarations: decls}}, nil
}

// toolConfigFor menerjemahkan tool_choice bergaya OpenAI ke functionCallingConfig.
func toolConfigFor(raw json.RawMessage) *toolConfig {
	if len(bytes.TrimSpace(raw)) == 0 {
		return nil
	}
	mode, allowed := "", []string(nil)

	var name string
	if err := json.Unmarshal(raw, &name); err == nil {
		switch name {
		case "auto":
			mode = "AUTO"
		case "none":
			mode = "NONE"
		case "required", "any":
			mode = "ANY"
		default:
			return nil
		}
	} else {
		var obj struct {
			Type     string `json:"type"`
			Function struct {
				Name string `json:"name"`
			} `json:"function"`
		}
		if err := json.Unmarshal(raw, &obj); err != nil {
			return nil
		}
		if obj.Type != "function" || obj.Function.Name == "" {
			return nil
		}
		// Memaksa satu fungsi tertentu: mode ANY dengan daftar yang hanya memuat fungsi itu.
		mode, allowed = "ANY", []string{obj.Function.Name}
	}
	return &toolConfig{FunctionCallingConfig: &functionCallingConfig{Mode: mode, AllowedFunctionNames: allowed}}
}

// unsupportedSchemaKeys adalah kata kunci JSON Schema yang ditolak Gemini.
//
// Gemini hanya menerima subset OpenAPI 3.0, dan klien yang menulis skema untuk OpenAI
// hampir selalu menyertakan "additionalProperties" serta "$schema" — dua field yang
// membuat SELURUH permintaan gagal di Gemini. Keduanya tidak berpengaruh pada makna skema
// di sana (Gemini menerapkan skema secara ketat), jadi dibuang alih-alih menggagalkan
// permintaan. Kata kunci lain diteruskan apa adanya: skema yang memakai fitur di luar
// subset OpenAPI Gemini memang harus ditolak upstream dengan sebabnya sendiri.
var unsupportedSchemaKeys = map[string]bool{
	"$schema":              true,
	"$defs":                true,
	"definitions":          true,
	"additionalProperties": true,
	"patternProperties":    true,
	"strict":               true,
}

// sanitizeSchema membuang kata kunci JSON Schema yang ditolak Gemini, secara rekursif.
func sanitizeSchema(raw json.RawMessage) json.RawMessage {
	if len(bytes.TrimSpace(raw)) == 0 {
		return nil
	}
	var doc any
	if err := json.Unmarshal(raw, &doc); err != nil {
		// Bukan JSON sah: diteruskan apa adanya supaya upstream yang melaporkan sebabnya.
		return raw
	}
	cleaned, err := json.Marshal(pruneSchema(doc))
	if err != nil {
		return raw
	}
	return cleaned
}

func pruneSchema(node any) any {
	switch v := node.(type) {
	case map[string]any:
		out := make(map[string]any, len(v))
		for key, val := range v {
			if unsupportedSchemaKeys[key] {
				continue
			}
			out[key] = pruneSchema(val)
		}
		return out
	case []any:
		out := make([]any, 0, len(v))
		for _, item := range v {
			out = append(out, pruneSchema(item))
		}
		return out
	default:
		return node
	}
}

// parseDataURI memisahkan mime type dan muatan base64 dari sebuah data URI.
func parseDataURI(raw string) (mimeType, data string, ok bool) {
	if !strings.HasPrefix(raw, "data:") {
		return "", "", false
	}
	meta, payload, found := strings.Cut(raw[len("data:"):], ",")
	if !found || payload == "" {
		return "", "", false
	}
	if !strings.HasSuffix(meta, ";base64") {
		return "", "", false
	}
	meta = strings.TrimSuffix(meta, ";base64")
	if i := strings.IndexByte(meta, ';'); i >= 0 {
		meta = meta[:i]
	}
	if meta == "" {
		meta = "application/octet-stream"
	}
	return meta, payload, true
}

func textContent(text string) geminiContent {
	return geminiContent{Parts: []geminiPart{{Text: text}}}
}

// toCanonicalUsage menerjemahkan usageMetadata ke bentuk kanonik.
//
// Dua hal yang mudah salah:
//
//   - promptTokenCount Gemini SUDAH memuat cachedContentTokenCount, jadi keduanya tidak
//     boleh dijumlahkan lagi — berbeda dari Anthropic, yang melaporkannya terpisah.
//   - thoughtsTokenCount ditagih sebagai keluaran, sementara candidatesTokenCount tidak
//     memuatnya. Bentuk kanonik mengikuti OpenAI, yang completion_tokens-nya memuat token
//     penalaran dan menyebut reasoning_tokens sebagai bagiannya, jadi keduanya dijumlahkan
//     ke OutputTokens dan thoughts tetap dilaporkan terpisah di ReasoningTokens.
func toCanonicalUsage(u *usageMetadata) providers.Usage {
	if u == nil {
		return providers.Usage{}
	}
	out := providers.Usage{
		InputTokens:       u.PromptTokenCount,
		CachedInputTokens: u.CachedContentTokenCount,
		OutputTokens:      u.CandidatesTokenCount + u.ThoughtsTokenCount,
		ReasoningTokens:   u.ThoughtsTokenCount,
		// totalTokenCount dipakai bila ada: itu angka Gemini sendiri dan bisa memuat token
		// yang tidak terwakili field lain (mis. pemakaian tool bawaan).
		TotalTokens: u.TotalTokenCount,
	}
	if out.TotalTokens == 0 {
		out.TotalTokens = out.InputTokens + out.OutputTokens
	}
	return out
}

// mapFinishReason menerjemahkan finishReason Gemini ke bentuk kanonik.
//
// SAFETY dan RECITATION menjadi content_filter, dan itu keputusan dengan konsekuensi:
// content_filter sengaja TIDAK di-failover, karena provider lain kemungkinan besar menolak
// muatan yang sama dan mencoba semuanya berarti mengirim muatan itu ke banyak pihak.
func mapFinishReason(reason string) string {
	switch strings.ToUpper(strings.TrimSpace(reason)) {
	case "":
		return ""
	case "MAX_TOKENS":
		return providers.FinishLength
	case "SAFETY", "RECITATION", "BLOCKLIST", "PROHIBITED_CONTENT", "SPII", "IMAGE_SAFETY":
		return providers.FinishContentFilter
	default:
		// STOP, OTHER, MALFORMED_FUNCTION_CALL, dan alasan baru apa pun: dari sisi klien
		// artinya model berhenti.
		return providers.FinishStop
	}
}

// toCanonicalResponse menerjemahkan respons Gemini ke bentuk kanonik.
func (p *Provider) toCanonicalResponse(model string, u *generateResponse) (*providers.ChatResponse, error) {
	if err := p.checkBlocked(u); err != nil {
		return nil, err
	}
	if len(u.Candidates) == 0 {
		return nil, providers.Newf(providers.ErrKindServer, p.name, "respons Gemini tidak memuat kandidat jawaban")
	}

	resp := &providers.ChatResponse{
		ID:    u.ResponseID,
		Model: firstNonEmpty(u.ModelVersion, model),
		// Gemini tidak mengirim waktu pembuatan, sedangkan klien berdialek OpenAI
		// mengharapkannya; waktu penerimaan adalah perkiraan terdekat yang tersedia.
		Created: time.Now().Unix(),
		Choices: make([]providers.Choice, 0, len(u.Candidates)),
		Usage:   toCanonicalUsage(u.UsageMetadata),
	}
	if resp.ID == "" {
		resp.ID = fmt.Sprintf("gemini-%d", resp.Created)
	}

	// Penomoran panggilan tool berjalan lintas kandidat supaya id yang disintesis tetap
	// unik dalam satu respons.
	seq := 0
	for i := range u.Candidates {
		cand := &u.Candidates[i]
		var text, reasoning strings.Builder
		var calls []providers.ToolCall

		for j := range cand.Content.Parts {
			part := &cand.Content.Parts[j]
			switch {
			case part.FunctionCall != nil:
				args := "{}"
				if len(bytes.TrimSpace(part.FunctionCall.Args)) > 0 {
					args = string(part.FunctionCall.Args)
				}
				calls = append(calls, providers.ToolCall{
					Index:    len(calls),
					ID:       synthesizeCallID(seq, part.FunctionCall.Name),
					Type:     "function",
					Function: providers.FunctionCall{Name: part.FunctionCall.Name, Arguments: args},
				})
				seq++
			case part.Thought:
				reasoning.WriteString(part.Text)
			case part.Text != "":
				text.WriteString(part.Text)
			}
		}

		finish := mapFinishReason(cand.FinishReason)
		// Gemini menjawab STOP walau yang dihasilkannya adalah panggilan tool; klien
		// berdialek OpenAI menunggu tool_calls untuk tahu bahwa putarannya belum selesai.
		if len(calls) > 0 && (finish == providers.FinishStop || finish == "") {
			finish = providers.FinishToolCalls
		}
		resp.Choices = append(resp.Choices, providers.Choice{
			Index: cand.Index,
			Message: providers.Message{
				Role:             providers.RoleAssistant,
				Content:          text.String(),
				ToolCalls:        calls,
				ReasoningContent: reasoning.String(),
			},
			FinishReason: finish,
		})
	}

	resp.Raw = encodeCompletion(resp)
	return resp, nil
}

// checkBlocked menerjemahkan penolakan kebijakan konten dan error yang menumpang di body.
func (p *Provider) checkBlocked(u *generateResponse) error {
	if u.PromptFeedback != nil && u.PromptFeedback.BlockReason != "" {
		// blockReason berarti PERMINTAAN yang diblokir kebijakan konten, bukan kegagalan
		// generik. Klasifikasinya penting: content_filter tidak di-failover, sehingga
		// muatan yang sama tidak dikirim berkeliling ke provider lain yang akan menolaknya
		// dengan alasan serupa.
		return providers.Newf(providers.ErrKindContentFilter, p.name,
			"Gemini memblokir permintaan karena kebijakan konten (%s)", u.PromptFeedback.BlockReason)
	}
	if u.Error != nil {
		code, message := u.Error.Status, u.Error.Message
		if message == "" {
			message = "Gemini melaporkan kegagalan"
		}
		return &providers.Error{
			Kind:         providers.Classify(u.Error.Code, code),
			Provider:     p.name,
			StatusCode:   u.Error.Code,
			Message:      p.safeMessage(message),
			UpstreamCode: code,
		}
	}
	return nil
}

// embeddingsResponse menyusun respons embeddings kanonik.
func (p *Provider) embeddingsResponse(model string, values []embeddingValues) *providers.EmbeddingsResponse {
	out := &providers.EmbeddingsResponse{
		Model: model,
		Data:  make([]providers.Embedding, 0, len(values)),
	}
	for i, v := range values {
		out.Data = append(out.Data, providers.Embedding{Index: i, Vector: v.Values})
	}
	// Gemini tidak melaporkan token pada endpoint embeddings, jadi Usage tetap nol.
	out.Raw = encodeEmbeddings(out)
	return out
}

func firstNonEmpty(values ...string) string {
	for _, v := range values {
		if v != "" {
			return v
		}
	}
	return ""
}
