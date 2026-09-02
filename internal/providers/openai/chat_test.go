package openai

import (
	"context"
	"encoding/json"
	"net/http"
	"strings"
	"testing"

	"github.com/NexGen-X/Route-X/internal/providers"
)

// chatResponseBody adalah respons upstream yang sengaja memuat field yang belum dikenal
// gateway (system_fingerprint, logprobs, field_masa_depan). Field-field itu harus tetap
// utuh di Raw, karena Raw-lah yang diteruskan ke klien.
const chatResponseBody = `{
  "id": "chatcmpl-123",
  "object": "chat.completion",
  "created": 1712345678,
  "model": "gpt-4o-mini-2024-07-18",
  "system_fingerprint": "fp_belum_dikenal",
  "choices": [
    {
      "index": 0,
      "message": {"role": "assistant", "content": "Halo!", "refusal": null},
      "logprobs": null,
      "finish_reason": "stop"
    }
  ],
  "usage": {
    "prompt_tokens": 30,
    "completion_tokens": 12,
    "total_tokens": 42,
    "prompt_tokens_details": {"cached_tokens": 24, "audio_tokens": 0},
    "completion_tokens_details": {"reasoning_tokens": 8, "audio_tokens": 0}
  },
  "field_masa_depan": {"apa pun": [1, 2, 3]}
}`

// requestPayload adalah bentuk body permintaan yang diperiksa test.
type requestPayload struct {
	Model    string `json:"model"`
	Stream   *bool  `json:"stream"`
	Messages []struct {
		Role       string          `json:"role"`
		Content    json.RawMessage `json:"content"`
		Name       string          `json:"name"`
		ToolCallID string          `json:"tool_call_id"`
		ToolCalls  []struct {
			Index    *int   `json:"index"`
			ID       string `json:"id"`
			Type     string `json:"type"`
			Function struct {
				Name      string `json:"name"`
				Arguments string `json:"arguments"`
			} `json:"function"`
		} `json:"tool_calls"`
		ReasoningContent string `json:"reasoning_content"`
	} `json:"messages"`
	MaxTokens   *int     `json:"max_tokens"`
	Temperature *float64 `json:"temperature"`
	TopP        *float64 `json:"top_p"`
	Stop        []string `json:"stop"`
	Seed        *int     `json:"seed"`
	User        string   `json:"user"`
	Tools       []struct {
		Type     string `json:"type"`
		Function struct {
			Name       string          `json:"name"`
			Parameters json.RawMessage `json:"parameters"`
		} `json:"function"`
	} `json:"tools"`
	ToolChoice        json.RawMessage `json:"tool_choice"`
	ResponseFormat    json.RawMessage `json:"response_format"`
	ParallelToolCalls *bool           `json:"parallel_tool_calls"`
	ReasoningEffort   string          `json:"reasoning_effort"`
}

// lastPayload mengurai body permintaan terakhir ke bentuk bertipe.
func (u *upstream) lastPayload() requestPayload {
	u.t.Helper()
	var out requestPayload
	if err := json.Unmarshal(u.last().body, &out); err != nil {
		u.t.Fatalf("body permintaan tidak bisa diurai: %v", err)
	}
	return out
}

func ptr[T any](v T) *T { return &v }

// Setiap parameter yang dipahami gateway harus benar-benar sampai ke upstream.
func TestChatCompletionSendsRequestParameters(t *testing.T) {
	u := newUpstream(t, jsonHandler(200, chatResponseBody))
	p := testProvider(t, u.url(), nil)

	req := &providers.ChatRequest{
		Model: "gpt-4o-mini",
		Messages: []providers.Message{
			{Role: providers.RoleSystem, Content: "jadilah ringkas"},
			{Role: providers.RoleUser, Content: "cuaca di Jakarta?", Name: "budi"},
		},
		MaxTokens:         ptr(256),
		Temperature:       ptr(0.4),
		TopP:              ptr(0.9),
		Stop:              []string{"###"},
		Seed:              ptr(7),
		User:              "user-abc",
		ReasoningEffort:   "medium",
		ParallelToolCalls: ptr(false),
		ToolChoice:        json.RawMessage(`"auto"`),
		ResponseFormat:    json.RawMessage(`{"type":"json_object"}`),
		Tools: []providers.Tool{{
			Function: providers.FunctionDef{
				Name:       "get_weather",
				Parameters: json.RawMessage(`{"type":"object","properties":{"kota":{"type":"string"}}}`),
			},
		}},
	}

	if _, err := p.ChatCompletion(context.Background(), req); err != nil {
		t.Fatalf("ChatCompletion: %v", err)
	}

	got := u.last()
	if got.method != http.MethodPost {
		t.Errorf("method = %s, mau POST", got.method)
	}
	if got.path != "/v1/chat/completions" {
		t.Errorf("path = %s", got.path)
	}
	if ct := got.header.Get("Content-Type"); ct != "application/json" {
		t.Errorf("Content-Type = %q", ct)
	}

	body := u.lastPayload()
	if body.Model != "gpt-4o-mini" {
		t.Errorf("model = %q", body.Model)
	}
	if len(body.Messages) != 2 || body.Messages[1].Name != "budi" {
		t.Fatalf("messages tidak diteruskan utuh: %+v", body.Messages)
	}
	if string(body.Messages[0].Content) != `"jadilah ringkas"` {
		t.Errorf("pesan teks biasa tidak dikirim sebagai string: %s", body.Messages[0].Content)
	}
	if body.MaxTokens == nil || *body.MaxTokens != 256 {
		t.Errorf("max_tokens = %v", body.MaxTokens)
	}
	if body.Temperature == nil || *body.Temperature != 0.4 {
		t.Errorf("temperature = %v", body.Temperature)
	}
	if body.TopP == nil || *body.TopP != 0.9 {
		t.Errorf("top_p = %v", body.TopP)
	}
	if len(body.Stop) != 1 || body.Stop[0] != "###" {
		t.Errorf("stop = %v", body.Stop)
	}
	if body.Seed == nil || *body.Seed != 7 {
		t.Errorf("seed = %v", body.Seed)
	}
	if body.User != "user-abc" {
		t.Errorf("user = %q", body.User)
	}
	if body.ReasoningEffort != "medium" {
		t.Errorf("reasoning_effort = %q", body.ReasoningEffort)
	}
	if body.ParallelToolCalls == nil || *body.ParallelToolCalls {
		t.Errorf("parallel_tool_calls = %v", body.ParallelToolCalls)
	}
	if string(body.ToolChoice) != `"auto"` {
		t.Errorf("tool_choice = %s", body.ToolChoice)
	}
	if string(body.ResponseFormat) != `{"type":"json_object"}` {
		t.Errorf("response_format = %s", body.ResponseFormat)
	}
	if len(body.Tools) != 1 || body.Tools[0].Function.Name != "get_weather" {
		t.Fatalf("tools tidak diteruskan: %+v", body.Tools)
	}
	// Type yang kosong diisi "function" supaya upstream tidak menolak dengan 400.
	if body.Tools[0].Type != "function" {
		t.Errorf("tools[0].type = %q, mau function", body.Tools[0].Type)
	}
	// stream selalu ditulis eksplisit, termasuk pada jalur non-streaming.
	if body.Stream == nil || *body.Stream {
		t.Errorf("stream = %v, mau false eksplisit", body.Stream)
	}
}

// Respons harus terurai lengkap, termasuk token yang harganya berbeda.
func TestChatCompletionParsesResponse(t *testing.T) {
	u := newUpstream(t, jsonHandler(200, chatResponseBody))
	p := testProvider(t, u.url(), nil)

	resp, err := p.ChatCompletion(context.Background(), chatRequest())
	if err != nil {
		t.Fatalf("ChatCompletion: %v", err)
	}

	if resp.ID != "chatcmpl-123" {
		t.Errorf("ID = %q", resp.ID)
	}
	if resp.Model != "gpt-4o-mini-2024-07-18" {
		t.Errorf("Model = %q", resp.Model)
	}
	if resp.Created != 1712345678 {
		t.Errorf("Created = %d", resp.Created)
	}
	if len(resp.Choices) != 1 {
		t.Fatalf("jumlah choice = %d", len(resp.Choices))
	}
	c := resp.Choices[0]
	if c.Message.Content != "Halo!" {
		t.Errorf("Content = %q", c.Message.Content)
	}
	if c.Message.Role != providers.RoleAssistant {
		t.Errorf("Role = %q", c.Message.Role)
	}
	if c.FinishReason != providers.FinishStop {
		t.Errorf("FinishReason = %q", c.FinishReason)
	}

	want := providers.Usage{
		InputTokens:       30,
		CachedInputTokens: 24,
		OutputTokens:      12,
		ReasoningTokens:   8,
		TotalTokens:       42,
	}
	if resp.Usage != want {
		t.Errorf("Usage = %+v, mau %+v", resp.Usage, want)
	}
}

// Raw adalah body upstream apa adanya: field yang belum dikenal gateway tidak boleh hilang,
// karena Raw inilah yang diteruskan ke klien.
func TestChatCompletionRawIsUpstreamBodyVerbatim(t *testing.T) {
	u := newUpstream(t, jsonHandler(200, chatResponseBody))
	p := testProvider(t, u.url(), nil)

	resp, err := p.ChatCompletion(context.Background(), chatRequest())
	if err != nil {
		t.Fatalf("ChatCompletion: %v", err)
	}
	if string(resp.Raw) != chatResponseBody {
		t.Errorf("Raw bukan body upstream apa adanya:\n%s", resp.Raw)
	}
	for _, field := range []string{"system_fingerprint", "field_masa_depan", "logprobs", "refusal"} {
		if !strings.Contains(string(resp.Raw), field) {
			t.Errorf("field %q hilang dari Raw", field)
		}
	}
}

// Extra adalah jalan bagi parameter provider yang belum dikenal gateway.
func TestChatCompletionForwardsExtraFields(t *testing.T) {
	u := newUpstream(t, jsonHandler(200, chatResponseBody))
	p := testProvider(t, u.url(), nil)

	req := chatRequest()
	req.Extra = map[string]json.RawMessage{
		"logit_bias":             json.RawMessage(`{"1234":-100}`),
		"prediction":             json.RawMessage(`{"type":"content","content":"tebakan"}`),
		"parameter_masa_depan":   json.RawMessage(`true`),
		"":                       json.RawMessage(`"kunci kosong diabaikan"`),
		"nilai_kosong_diabaikan": json.RawMessage(``),
		"max_completion_tokens":  json.RawMessage(`512`),
	}

	if _, err := p.ChatCompletion(context.Background(), req); err != nil {
		t.Fatalf("ChatCompletion: %v", err)
	}

	body := u.lastBody()
	if got, ok := body["parameter_masa_depan"]; !ok || got != true {
		t.Errorf("parameter_masa_depan = %v (ada: %v)", got, ok)
	}
	if got, ok := body["max_completion_tokens"]; !ok || got != float64(512) {
		t.Errorf("max_completion_tokens = %v", got)
	}
	bias, ok := body["logit_bias"].(map[string]any)
	if !ok || bias["1234"] != float64(-100) {
		t.Errorf("logit_bias = %v", body["logit_bias"])
	}
	if _, ok := body[""]; ok {
		t.Error("kunci kosong diteruskan ke upstream")
	}
	if _, ok := body["nilai_kosong_diabaikan"]; ok {
		t.Error("nilai kosong diteruskan ke upstream")
	}
}

// Extra berasal dari body klien. Kalau ia bisa menimpa field gateway, klien bisa menukar
// model yang sudah dipilih mesin routing, mengganti messages yang sudah lewat content
// filter, atau membalik "stream" sehingga adapter menguraikan bentuk respons yang salah.
func TestExtraCannotOverrideGatewayFields(t *testing.T) {
	u := newUpstream(t, jsonHandler(200, chatResponseBody))
	p := testProvider(t, u.url(), nil)

	req := chatRequest()
	req.MaxTokens = ptr(64)
	req.Temperature = ptr(0.1)
	req.Extra = map[string]json.RawMessage{
		"model":       json.RawMessage(`"model-pilihan-klien"`),
		"messages":    json.RawMessage(`[{"role":"user","content":"pesan sisipan"}]`),
		"stream":      json.RawMessage(`true`),
		"max_tokens":  json.RawMessage(`999999`),
		"temperature": json.RawMessage(`2`),
	}

	if _, err := p.ChatCompletion(context.Background(), req); err != nil {
		t.Fatalf("ChatCompletion: %v", err)
	}

	body := u.lastPayload()
	if body.Model != "gpt-4o-mini" {
		t.Errorf("model = %q, mau gpt-4o-mini", body.Model)
	}
	if len(body.Messages) != 1 || string(body.Messages[0].Content) != `"halo"` {
		t.Errorf("messages ditimpa Extra: %s", u.last().body)
	}
	if body.Stream == nil || *body.Stream {
		t.Errorf("stream ditimpa Extra: %v", body.Stream)
	}
	if body.MaxTokens == nil || *body.MaxTokens != 64 {
		t.Errorf("max_tokens = %v, mau 64", body.MaxTokens)
	}
	if body.Temperature == nil || *body.Temperature != 0.1 {
		t.Errorf("temperature = %v, mau 0.1", body.Temperature)
	}
}

// Extra yang bukan JSON sah harus ditolak sebagai permintaan cacat, bukan dikirim.
func TestChatCompletionRejectsInvalidExtra(t *testing.T) {
	u := newUpstream(t, jsonHandler(200, chatResponseBody))
	p := testProvider(t, u.url(), nil)

	req := chatRequest()
	req.Extra = map[string]json.RawMessage{"rusak": json.RawMessage(`{tidak sah`)}

	_, err := p.ChatCompletion(context.Background(), req)
	e := providers.AsError(err)
	if e == nil {
		t.Fatalf("error bukan *providers.Error: %v", err)
	}
	if e.Kind != providers.ErrKindInvalidRequest {
		t.Errorf("Kind = %s, mau %s", e.Kind, providers.ErrKindInvalidRequest)
	}
	if e.Failoverable() {
		t.Error("permintaan cacat tidak boleh dialihkan ke provider lain")
	}
	// Isi body klien tidak boleh ikut ke pesan error.
	if strings.Contains(e.Message, "tidak sah") {
		t.Errorf("pesan memuat potongan body klien: %q", e.Message)
	}
}

// Bentuk content dipilih dari Parts, bukan ditebak dari isi string.
func TestChatCompletionMultimodalContent(t *testing.T) {
	u := newUpstream(t, jsonHandler(200, chatResponseBody))
	p := testProvider(t, u.url(), nil)

	req := &providers.ChatRequest{
		Model: "gpt-4o",
		Messages: []providers.Message{
			{Role: providers.RoleSystem, Content: "teks biasa tetap string"},
			{Role: providers.RoleUser, Parts: []providers.ContentPart{
				{Type: providers.PartTypeText, Text: "apa ini?"},
				{Type: providers.PartTypeImageURL, ImageURL: &providers.ImageURL{
					URL: "https://x.test/a.png", Detail: "high",
				}},
				{Type: providers.PartTypeAudio, Audio: &providers.Audio{Data: "QUJD", Format: "wav"}},
			}},
			// Multimodal dengan satu bagian teks pendek: tetap harus jadi array, dan inilah
			// yang salah kalau bentuknya ditebak dari panjang Content.
			{Role: providers.RoleUser, Parts: []providers.ContentPart{{Type: providers.PartTypeText, Text: "ya"}}},
		},
	}

	if _, err := p.ChatCompletion(context.Background(), req); err != nil {
		t.Fatalf("ChatCompletion: %v", err)
	}

	body := u.lastPayload()
	if len(body.Messages) != 3 {
		t.Fatalf("jumlah pesan = %d", len(body.Messages))
	}
	if got := string(body.Messages[0].Content); got != `"teks biasa tetap string"` {
		t.Errorf("pesan teks biasa = %s, mau string tunggal", got)
	}
	if got := string(body.Messages[1].Content); !strings.HasPrefix(got, "[") {
		t.Fatalf("pesan multimodal bukan array: %s", got)
	}
	if got := string(body.Messages[2].Content); !strings.HasPrefix(got, "[") {
		t.Errorf("multimodal berisi satu bagian pendek bukan array: %s", got)
	}

	var parts []providers.ContentPart
	if err := json.Unmarshal(body.Messages[1].Content, &parts); err != nil {
		t.Fatalf("array content tidak bisa diurai: %v", err)
	}
	if len(parts) != 3 {
		t.Fatalf("jumlah bagian = %d", len(parts))
	}
	if parts[1].ImageURL == nil || parts[1].ImageURL.URL != "https://x.test/a.png" || parts[1].ImageURL.Detail != "high" {
		t.Errorf("bagian gambar tidak utuh: %+v", parts[1])
	}
	if parts[2].Audio == nil || parts[2].Audio.Format != "wav" {
		t.Errorf("bagian audio tidak utuh: %+v", parts[2])
	}
}

// Riwayat percakapan yang dikirim ulang: tool call ikut, dan jejak penalaran tidak.
func TestChatCompletionSerializesConversationHistory(t *testing.T) {
	u := newUpstream(t, jsonHandler(200, chatResponseBody))
	p := testProvider(t, u.url(), nil)

	req := &providers.ChatRequest{
		Model: "gpt-4o-mini",
		Messages: []providers.Message{
			{Role: providers.RoleUser, Content: "cuaca?"},
			{
				Role: providers.RoleAssistant,
				// Pesan asisten yang hanya memuat tool call: content tidak boleh dikirim
				// sebagai string kosong.
				ToolCalls: []providers.ToolCall{{
					Index: 3, ID: "call_1",
					Function: providers.FunctionCall{Name: "get_weather", Arguments: `{"kota":"Jakarta"}`},
				}},
				// Jejak penalaran adalah field keluaran; mengirimnya balik ditolak sebagian
				// provider penalaran.
				ReasoningContent: "pikiran panjang yang tidak boleh dikirim balik",
			},
			{Role: providers.RoleTool, ToolCallID: "call_1", Content: `{"suhu":31}`},
		},
	}

	if _, err := p.ChatCompletion(context.Background(), req); err != nil {
		t.Fatalf("ChatCompletion: %v", err)
	}

	raw := string(u.last().body)
	if strings.Contains(raw, "reasoning_content") || strings.Contains(raw, "pikiran panjang") {
		t.Errorf("jejak penalaran dikirim balik ke upstream: %s", raw)
	}

	body := u.lastPayload()
	assistant := body.Messages[1]
	if len(assistant.Content) != 0 && string(assistant.Content) != "null" {
		t.Errorf("content pesan tool call = %s, mau tidak dikirim", assistant.Content)
	}
	if len(assistant.ToolCalls) != 1 {
		t.Fatalf("tool_calls = %+v", assistant.ToolCalls)
	}
	tc := assistant.ToolCalls[0]
	if tc.ID != "call_1" || tc.Type != "function" || tc.Function.Arguments != `{"kota":"Jakarta"}` {
		t.Errorf("tool call tidak utuh: %+v", tc)
	}
	// index hanya bermakna pada aliran; ikut terkirim justru bisa ditolak upstream.
	if tc.Index != nil {
		t.Errorf("index ikut terkirim: %v", *tc.Index)
	}
	if body.Messages[2].ToolCallID != "call_1" {
		t.Errorf("tool_call_id = %q", body.Messages[2].ToolCallID)
	}
}

// Tool call pada respons non-streaming: index diisi dari urutan array supaya pemanggil bisa
// memasangkan hasilnya, walau upstream tidak mengirim index.
func TestChatCompletionParsesToolCallsAndReasoning(t *testing.T) {
	const body = `{
  "id": "chatcmpl-9",
  "model": "gpt-4o",
  "choices": [{
    "index": 0,
    "message": {
      "role": "assistant",
      "content": null,
      "reasoning_content": "menimbang dua kota",
      "tool_calls": [
        {"id": "call_a", "type": "function", "function": {"name": "get_weather", "arguments": "{\"kota\":\"Jakarta\"}"}},
        {"id": "call_b", "type": "function", "function": {"name": "get_weather", "arguments": "{\"kota\":\"Bandung\"}"}}
      ]
    },
    "finish_reason": "tool_calls"
  }]
}`
	u := newUpstream(t, jsonHandler(200, body))
	p := testProvider(t, u.url(), nil)

	resp, err := p.ChatCompletion(context.Background(), chatRequest())
	if err != nil {
		t.Fatalf("ChatCompletion: %v", err)
	}
	msg := resp.Choices[0].Message
	if msg.Content != "" {
		t.Errorf("Content = %q, mau kosong untuk content null", msg.Content)
	}
	if msg.ReasoningContent != "menimbang dua kota" {
		t.Errorf("ReasoningContent = %q", msg.ReasoningContent)
	}
	if len(msg.ToolCalls) != 2 {
		t.Fatalf("jumlah tool call = %d", len(msg.ToolCalls))
	}
	if msg.ToolCalls[0].Index != 0 || msg.ToolCalls[1].Index != 1 {
		t.Errorf("index tool call = %d dan %d, mau 0 dan 1", msg.ToolCalls[0].Index, msg.ToolCalls[1].Index)
	}
	if msg.ToolCalls[1].Function.Arguments != `{"kota":"Bandung"}` {
		t.Errorf("arguments = %q", msg.ToolCalls[1].Function.Arguments)
	}
	if resp.Choices[0].FinishReason != providers.FinishToolCalls {
		t.Errorf("FinishReason = %q", resp.Choices[0].FinishReason)
	}
	// Usage yang tidak dikirim upstream tetap nol, bukan mengarang angka.
	if resp.Usage.InputTokens != 0 || resp.Usage.TotalTokens != 0 {
		t.Errorf("Usage = %+v, mau nol", resp.Usage)
	}
}

// Sebagian provider compatible menjawab content sebagai array bagian. Menolaknya berarti
// seluruh respons gagal diurai padahal isinya lengkap.
func TestChatCompletionAcceptsContentAsPartsArray(t *testing.T) {
	const body = `{"id":"x","model":"m","choices":[{"index":0,"message":{"role":"assistant","content":[{"type":"text","text":"jawaban"}]},"finish_reason":"stop"}]}`
	u := newUpstream(t, jsonHandler(200, body))
	p := testProvider(t, u.url(), nil)

	resp, err := p.ChatCompletion(context.Background(), chatRequest())
	if err != nil {
		t.Fatalf("ChatCompletion: %v", err)
	}
	msg := resp.Choices[0].Message
	if !msg.IsMultimodal() || msg.Text() != "jawaban" {
		t.Errorf("content array tidak terurai: %+v", msg)
	}
}

// Nama model dipakai pencatatan pemakaian; provider yang tidak mengembalikannya tidak boleh
// menghasilkan baris tanpa model.
func TestChatCompletionFallsBackToRequestedModel(t *testing.T) {
	const body = `{"id":"x","choices":[{"index":0,"message":{"role":"assistant","content":"ya"},"finish_reason":"stop"},
	{"index":1,"message":{"role":"assistant","content":"juga"},"finish_reason":"length"}],
	"usage":{"prompt_tokens":5,"completion_tokens":3}}`
	u := newUpstream(t, jsonHandler(200, body))
	p := testProvider(t, u.url(), nil)

	resp, err := p.ChatCompletion(context.Background(), chatRequest())
	if err != nil {
		t.Fatalf("ChatCompletion: %v", err)
	}
	if resp.Model != "gpt-4o-mini" {
		t.Errorf("Model = %q, mau model yang diminta", resp.Model)
	}
	if len(resp.Choices) != 2 || resp.Choices[1].Index != 1 {
		t.Errorf("choice kedua hilang: %+v", resp.Choices)
	}
	// total_tokens yang tidak dikirim dihitung dari input dan output.
	if resp.Usage.TotalTokens != 8 {
		t.Errorf("TotalTokens = %d, mau 8", resp.Usage.TotalTokens)
	}
}

// Sebagian gateway berdialek OpenAI mengirim galat DENGAN status 200. Kalau dibiarkan
// lewat, mesin routing menganggap request berhasil dan tidak pernah failover.
func TestChatCompletionDetectsErrorWithSuccessStatus(t *testing.T) {
	const body = `{"error":{"message":"model sedang penuh","code":"insufficient_quota"}}`
	u := newUpstream(t, jsonHandler(200, body))
	p := testProvider(t, u.url(), nil)

	_, err := p.ChatCompletion(context.Background(), chatRequest())
	e := providers.AsError(err)
	if e == nil {
		t.Fatalf("error bukan *providers.Error: %v", err)
	}
	if e.Kind != providers.ErrKindQuota {
		t.Errorf("Kind = %s, mau %s", e.Kind, providers.ErrKindQuota)
	}
	if e.UpstreamCode != "insufficient_quota" {
		t.Errorf("UpstreamCode = %q", e.UpstreamCode)
	}
	if !e.Failoverable() {
		t.Error("kuota habis seharusnya boleh dialihkan ke provider lain")
	}
}

// Respons 200 tanpa choice dan tanpa bentuk error tetap harus menjadi kegagalan.
func TestChatCompletionRejectsResponseWithoutChoices(t *testing.T) {
	u := newUpstream(t, jsonHandler(200, `{"id":"x","model":"m","choices":[]}`))
	p := testProvider(t, u.url(), nil)

	_, err := p.ChatCompletion(context.Background(), chatRequest())
	e := providers.AsError(err)
	if e == nil {
		t.Fatalf("error bukan *providers.Error: %v", err)
	}
	if e.Kind != providers.ErrKindServer {
		t.Errorf("Kind = %s, mau %s", e.Kind, providers.ErrKindServer)
	}
}

// Body 2xx yang bukan JSON adalah kegagalan provider, bukan permintaan cacat.
func TestChatCompletionRejectsNonJSONResponse(t *testing.T) {
	u := newUpstream(t, func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(200)
		_, _ = w.Write([]byte("<html>gateway pihak ketiga sedang bermasalah</html>"))
	})
	p := testProvider(t, u.url(), nil)

	_, err := p.ChatCompletion(context.Background(), chatRequest())
	e := providers.AsError(err)
	if e == nil {
		t.Fatalf("error bukan *providers.Error: %v", err)
	}
	if e.Kind != providers.ErrKindServer {
		t.Errorf("Kind = %s, mau %s", e.Kind, providers.ErrKindServer)
	}
	if strings.Contains(e.Message, "html") {
		t.Errorf("pesan meneruskan body upstream: %q", e.Message)
	}
}

// Permintaan tanpa pesan tidak perlu menyentuh jaringan.
func TestChatCompletionRejectsEmptyRequest(t *testing.T) {
	u := newUpstream(t, jsonHandler(200, chatResponseBody))
	p := testProvider(t, u.url(), nil)
	ctx := context.Background()

	for _, req := range []*providers.ChatRequest{nil, {Model: "m"}} {
		if _, err := p.ChatCompletion(ctx, req); err == nil {
			t.Errorf("permintaan %+v diterima", req)
		}
		if _, err := p.ChatCompletionStream(ctx, req); err == nil {
			t.Errorf("permintaan streaming %+v diterima", req)
		}
	}
	u.mu.Lock()
	defer u.mu.Unlock()
	if len(u.reqs) != 0 {
		t.Errorf("%d permintaan tetap dikirim ke upstream", len(u.reqs))
	}
}

// Jejak penalaran punya dua nama di lapangan: DeepSeek memakai reasoning_content, sebagian
// agregator memakai reasoning. Keduanya harus terbaca, dan bentuk yang bukan string
// diabaikan alih-alih menggagalkan seluruh respons.
func TestChatCompletionReadsReasoningFromEitherField(t *testing.T) {
	tests := []struct {
		name    string
		message string
		want    string
	}{
		{"reasoning_content", `{"role":"assistant","content":"ya","reasoning_content":"jalur A"}`, "jalur A"},
		{"reasoning string", `{"role":"assistant","content":"ya","reasoning":"jalur B"}`, "jalur B"},
		{"reasoning_content menang", `{"role":"assistant","content":"ya","reasoning_content":"jalur A","reasoning":"jalur B"}`, "jalur A"},
		{"reasoning objek diabaikan", `{"role":"assistant","content":"ya","reasoning":{"ringkasan":"jalur C"}}`, ""},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			body := `{"id":"x","model":"m","choices":[{"index":0,"message":` + tc.message + `,"finish_reason":"stop"}]}`
			u := newUpstream(t, jsonHandler(200, body))
			p := testProvider(t, u.url(), nil)

			resp, err := p.ChatCompletion(context.Background(), chatRequest())
			if err != nil {
				t.Fatalf("ChatCompletion: %v", err)
			}
			if got := resp.Choices[0].Message.ReasoningContent; got != tc.want {
				t.Errorf("ReasoningContent = %q, mau %q", got, tc.want)
			}
		})
	}
}
