package anthropic

import (
	"context"
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync"
	"testing"

	"github.com/NexGen-X/Route-X/internal/providers"
	"github.com/NexGen-X/Route-X/internal/security"
)

// bodyRecorder menyimpan seluruh body permintaan yang masuk.
type bodyRecorder struct {
	mu     sync.Mutex
	bodies [][]byte
}

func (b *bodyRecorder) add(raw []byte) {
	b.mu.Lock()
	defer b.mu.Unlock()
	b.bodies = append(b.bodies, raw)
}

func (b *bodyRecorder) at(t *testing.T, i int) wireRequest {
	t.Helper()
	b.mu.Lock()
	defer b.mu.Unlock()
	if i >= len(b.bodies) {
		t.Fatalf("permintaan ke-%d belum pernah masuk (baru %d permintaan)", i, len(b.bodies))
	}
	return decodeRequest(t, b.bodies[i])
}

func (b *bodyRecorder) count() int {
	b.mu.Lock()
	defer b.mu.Unlock()
	return len(b.bodies)
}

// newScriptedServer membalas urutan body JSON sesuai urutan permintaan yang masuk.
func newScriptedServer(t *testing.T, responses ...string) (*Provider, *bodyRecorder) {
	t.Helper()
	rec := &bodyRecorder{}
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		raw, _ := io.ReadAll(r.Body)
		rec.add(raw)

		idx := rec.count() - 1
		if idx >= len(responses) {
			idx = len(responses) - 1
		}
		w.Header().Set("Content-Type", "application/json")
		_, _ = io.WriteString(w, responses[idx])
	}))
	t.Cleanup(srv.Close)

	p, err := New(Config{
		Name:       "anthropic-uji",
		BaseURL:    srv.URL,
		Credential: security.Secret(testKey),
		SSRFPolicy: testPolicy(),
	})
	if err != nil {
		t.Fatalf("New() error: %v", err)
	}
	return p, rec
}

func ptrFloat(v float64) *float64 { return &v }
func ptrInt(v int) *int           { return &v }
func ptrBool(v bool) *bool        { return &v }

func TestPermintaanKanonikDiterjemahkanFieldPerField(t *testing.T) {
	p, rec := newScriptedServer(t, textResponse)

	req := &providers.ChatRequest{
		Model: "claude-sonnet-4-5",
		Messages: []providers.Message{
			{Role: providers.RoleSystem, Content: "jadilah singkat"},
			{Role: providers.RoleUser, Content: "berapa 2+2?"},
			{Role: providers.RoleAssistant, Content: "4"},
			{Role: providers.RoleUser, Content: "yakin?"},
		},
		MaxTokens:   ptrInt(256),
		Temperature: ptrFloat(0.25),
		TopP:        ptrFloat(0.9),
		Stop:        []string{"STOP"},
		User:        "pengguna-42",
		// Field bergaya OpenAI tanpa padanan di Anthropic tidak boleh diteruskan.
		Seed:           ptrInt(7),
		ResponseFormat: json.RawMessage(`{"type":"json_object"}`),
		Extra:          map[string]json.RawMessage{"frequency_penalty": json.RawMessage(`0.5`)},
	}
	if _, err := p.ChatCompletion(context.Background(), req); err != nil {
		t.Fatalf("ChatCompletion() error: %v", err)
	}

	body := rec.at(t, 0)
	if body.Model != "claude-sonnet-4-5" {
		t.Errorf("model = %q", body.Model)
	}
	if body.MaxTokens != 256 {
		t.Errorf("max_tokens = %d, mau 256", body.MaxTokens)
	}
	if body.System != "jadilah singkat" {
		t.Errorf("system = %q", body.System)
	}
	if body.Temperature == nil || *body.Temperature != 0.25 {
		t.Errorf("temperature = %v", body.Temperature)
	}
	if body.TopP == nil || *body.TopP != 0.9 {
		t.Errorf("top_p = %v", body.TopP)
	}
	if len(body.StopSequences) != 1 || body.StopSequences[0] != "STOP" {
		t.Errorf("stop_sequences = %v (Anthropic memakai stop_sequences, bukan stop)", body.StopSequences)
	}
	if body.Stream {
		t.Error("stream = true pada permintaan non-streaming")
	}
	if body.Metadata["user_id"] != "pengguna-42" {
		t.Errorf("metadata = %v", body.Metadata)
	}
	if len(body.Extra) != 0 {
		t.Errorf("field bergaya OpenAI ikut terkirim: seed=%s", body.Extra)
	}
	if len(body.Messages) != 3 {
		t.Fatalf("jumlah pesan = %d, mau 3", len(body.Messages))
	}
	mau := []string{"user", "assistant", "user"}
	for i, m := range body.Messages {
		if m.Role != mau[i] {
			t.Errorf("pesan[%d].role = %q, mau %q", i, m.Role, mau[i])
		}
		if len(m.Content) != 1 || m.Content[0].Type != blockText {
			t.Errorf("pesan[%d].content = %+v, mau satu block text", i, m.Content)
		}
	}
	if body.Messages[0].Content[0].Text != "berapa 2+2?" {
		t.Errorf("isi pesan pertama = %q", body.Messages[0].Content[0].Text)
	}
}

func TestPesanSystemTidakPernahTertinggalDiArrayMessages(t *testing.T) {
	tests := []struct {
		nama      string
		messages  []providers.Message
		mauSystem string
	}{
		{
			nama: "tanpa system",
			messages: []providers.Message{
				{Role: providers.RoleUser, Content: "halo"},
			},
			mauSystem: "",
		},
		{
			nama: "satu system",
			messages: []providers.Message{
				{Role: providers.RoleSystem, Content: "kamu penerjemah"},
				{Role: providers.RoleUser, Content: "halo"},
			},
			mauSystem: "kamu penerjemah",
		},
		{
			nama: "beberapa system dan developer, termasuk di tengah percakapan",
			messages: []providers.Message{
				{Role: providers.RoleSystem, Content: "aturan satu"},
				{Role: providers.RoleDeveloper, Content: "aturan dua"},
				{Role: providers.RoleUser, Content: "halo"},
				{Role: providers.RoleSystem, Content: "aturan tiga"},
			},
			mauSystem: "aturan satu\n\naturan dua\n\naturan tiga",
		},
		{
			nama: "system multimodal",
			messages: []providers.Message{
				{Role: providers.RoleSystem, Parts: []providers.ContentPart{
					{Type: providers.PartTypeText, Text: "baris satu"},
					{Type: providers.PartTypeText, Text: "baris dua"},
				}},
				{Role: providers.RoleUser, Content: "halo"},
			},
			mauSystem: "baris satu\nbaris dua",
		},
		{
			nama: "system kosong dibuang",
			messages: []providers.Message{
				{Role: providers.RoleSystem, Content: ""},
				{Role: providers.RoleUser, Content: "halo"},
			},
			mauSystem: "",
		},
	}

	for _, tc := range tests {
		t.Run(tc.nama, func(t *testing.T) {
			p, rec := newScriptedServer(t, textResponse)
			req := &providers.ChatRequest{Model: "claude-sonnet-4-5", Messages: tc.messages}
			if _, err := p.ChatCompletion(context.Background(), req); err != nil {
				t.Fatalf("ChatCompletion() error: %v", err)
			}

			body := rec.at(t, 0)
			if body.System != tc.mauSystem {
				t.Errorf("system = %q, mau %q", body.System, tc.mauSystem)
			}
			for i, m := range body.Messages {
				if m.Role != roleUser && m.Role != roleAssistant {
					t.Errorf("pesan[%d].role = %q; Anthropic menolak peran selain user/assistant", i, m.Role)
				}
			}
		})
	}
}

func TestPeranBerurutanDigabungMenjadiSatuPesan(t *testing.T) {
	p, rec := newScriptedServer(t, textResponse)

	req := &providers.ChatRequest{
		Model: "claude-sonnet-4-5",
		Messages: []providers.Message{
			{Role: providers.RoleUser, Content: "bagian satu"},
			{Role: providers.RoleUser, Content: "bagian dua"},
			{Role: providers.RoleAssistant, Content: "jawab satu"},
			{Role: providers.RoleAssistant, Content: "jawab dua"},
			{Role: providers.RoleUser, Content: "bagian tiga"},
		},
	}
	if _, err := p.ChatCompletion(context.Background(), req); err != nil {
		t.Fatalf("ChatCompletion() error: %v", err)
	}

	body := rec.at(t, 0)
	if len(body.Messages) != 3 {
		t.Fatalf("jumlah pesan = %d, mau 3 setelah penggabungan: %+v", len(body.Messages), body.Messages)
	}
	if len(body.Messages[0].Content) != 2 {
		t.Fatalf("pesan user pertama memuat %d block, mau 2", len(body.Messages[0].Content))
	}
	if body.Messages[0].Content[0].Text != "bagian satu" || body.Messages[0].Content[1].Text != "bagian dua" {
		t.Errorf("urutan block berubah: %+v", body.Messages[0].Content)
	}
	if body.Messages[1].Role != roleAssistant || len(body.Messages[1].Content) != 2 {
		t.Errorf("pesan assistant tidak tergabung: %+v", body.Messages[1])
	}
}

func TestPercakapanTidakSahDitolakSebelumDikirim(t *testing.T) {
	tests := []struct {
		nama     string
		req      *providers.ChatRequest
		mauPesan string
	}{
		{"permintaan nil", nil, "kosong"},
		{"model kosong", &providers.ChatRequest{Messages: []providers.Message{{Role: providers.RoleUser, Content: "x"}}}, "model"},
		{"tanpa pesan", &providers.ChatRequest{Model: "claude-sonnet-4-5"}, "percakapan"},
		{
			"hanya system",
			&providers.ChatRequest{Model: "claude-sonnet-4-5", Messages: []providers.Message{
				{Role: providers.RoleSystem, Content: "aturan"},
			}},
			"percakapan",
		},
		{
			"dibuka assistant",
			&providers.ChatRequest{Model: "claude-sonnet-4-5", Messages: []providers.Message{
				{Role: providers.RoleAssistant, Content: "aku mulai"},
				{Role: providers.RoleUser, Content: "halo"},
			}},
			"user",
		},
		{
			"peran tidak dikenal",
			&providers.ChatRequest{Model: "claude-sonnet-4-5", Messages: []providers.Message{
				{Role: providers.Role("penonton"), Content: "halo"},
			}},
			"peran",
		},
		{
			"hasil tool tanpa id",
			&providers.ChatRequest{Model: "claude-sonnet-4-5", Messages: []providers.Message{
				{Role: providers.RoleUser, Content: "cuaca?"},
				{Role: providers.RoleTool, Content: "cerah"},
			}},
			"tool_call_id",
		},
		{
			"argumen tool bukan objek",
			&providers.ChatRequest{Model: "claude-sonnet-4-5", Messages: []providers.Message{
				{Role: providers.RoleUser, Content: "cuaca?"},
				{Role: providers.RoleAssistant, ToolCalls: []providers.ToolCall{{
					ID: "toolu_1", Type: "function",
					Function: providers.FunctionCall{Name: "cuaca", Arguments: "bukan-json"},
				}}},
			}},
			"argumen tool",
		},
		{
			"tool tanpa nama",
			&providers.ChatRequest{Model: "claude-sonnet-4-5",
				Messages: []providers.Message{{Role: providers.RoleUser, Content: "halo"}},
				Tools:    []providers.Tool{{Type: "function"}},
			},
			"nama",
		},
		{
			"masukan audio",
			&providers.ChatRequest{Model: "claude-sonnet-4-5", Messages: []providers.Message{
				{Role: providers.RoleUser, Parts: []providers.ContentPart{
					{Type: providers.PartTypeAudio, Audio: &providers.Audio{Data: "AAA", Format: "wav"}},
				}},
			}},
			"audio",
		},
	}

	for _, tc := range tests {
		t.Run(tc.nama, func(t *testing.T) {
			p, rec := newScriptedServer(t, textResponse)
			_, err := p.ChatCompletion(context.Background(), tc.req)
			e := providers.AsError(err)
			if e == nil {
				t.Fatalf("error = %v, mau *providers.Error", err)
			}
			if e.Kind != providers.ErrKindInvalidRequest {
				t.Errorf("Kind = %q, mau %q", e.Kind, providers.ErrKindInvalidRequest)
			}
			if !strings.Contains(strings.ToLower(e.Message), strings.ToLower(tc.mauPesan)) {
				t.Errorf("pesan %q tidak menyebut %q", e.Message, tc.mauPesan)
			}
			if rec.count() != 0 {
				t.Error("permintaan cacat tetap dikirim ke upstream")
			}
			if e.Failoverable() {
				t.Error("permintaan cacat tidak boleh dialihkan ke provider lain")
			}
		})
	}
}

func TestMaxTokensSelaluTerisi(t *testing.T) {
	t.Run("bawaan dipakai bila kanonik kosong", func(t *testing.T) {
		p, rec := newScriptedServer(t, textResponse)
		if _, err := p.ChatCompletion(context.Background(), simpleRequest()); err != nil {
			t.Fatalf("ChatCompletion() error: %v", err)
		}
		if got := rec.at(t, 0).MaxTokens; got != DefaultMaxTokens {
			t.Errorf("max_tokens = %d, mau %d", got, DefaultMaxTokens)
		}
	})

	t.Run("bawaan dari config", func(t *testing.T) {
		rec := &bodyRecorder{}
		srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			raw, _ := io.ReadAll(r.Body)
			rec.add(raw)
			_, _ = io.WriteString(w, textResponse)
		}))
		t.Cleanup(srv.Close)

		p, err := New(Config{
			BaseURL: srv.URL, Credential: security.Secret(testKey),
			SSRFPolicy: testPolicy(), DefaultMaxTokens: 777,
		})
		if err != nil {
			t.Fatalf("New() error: %v", err)
		}
		if _, err := p.ChatCompletion(context.Background(), simpleRequest()); err != nil {
			t.Fatalf("ChatCompletion() error: %v", err)
		}
		if got := rec.at(t, 0).MaxTokens; got != 777 {
			t.Errorf("max_tokens = %d, mau 777", got)
		}
	})

	t.Run("nilai nol diabaikan", func(t *testing.T) {
		p, rec := newScriptedServer(t, textResponse)
		req := simpleRequest()
		req.MaxTokens = ptrInt(0)
		if _, err := p.ChatCompletion(context.Background(), req); err != nil {
			t.Fatalf("ChatCompletion() error: %v", err)
		}
		if got := rec.at(t, 0).MaxTokens; got != DefaultMaxTokens {
			t.Errorf("max_tokens = %d, mau %d (0 bukan nilai sah bagi Anthropic)", got, DefaultMaxTokens)
		}
	})
}

func TestReasoningEffortMenjadiThinking(t *testing.T) {
	p, rec := newScriptedServer(t, textResponse)

	req := simpleRequest()
	req.ReasoningEffort = "high"
	req.Temperature = ptrFloat(0.7)
	req.TopP = ptrFloat(0.5)
	req.MaxTokens = ptrInt(512) // lebih kecil dari budget, harus dinaikkan
	if _, err := p.ChatCompletion(context.Background(), req); err != nil {
		t.Fatalf("ChatCompletion() error: %v", err)
	}

	body := rec.at(t, 0)
	if body.Thinking == nil {
		t.Fatal("thinking tidak terkirim")
	}
	if body.Thinking["type"] != "enabled" {
		t.Errorf("thinking.type = %v", body.Thinking["type"])
	}
	budget, ok := body.Thinking["budget_tokens"].(float64)
	if !ok || int(budget) != 8192 {
		t.Errorf("budget_tokens = %v, mau 8192", body.Thinking["budget_tokens"])
	}
	if body.MaxTokens <= int(budget) {
		t.Errorf("max_tokens = %d, mau lebih besar dari budget %d", body.MaxTokens, int(budget))
	}
	if body.Temperature != nil || body.TopP != nil {
		t.Error("temperature/top_p tetap terkirim; Anthropic menolaknya saat thinking menyala")
	}

	if got := thinkingBudget("tidak-dikenal"); got != 0 {
		t.Errorf("thinkingBudget(tidak dikenal) = %d, mau 0", got)
	}
	for effort, mau := range map[string]int{"minimal": 1024, "low": 1024, "medium": 4096, "high": 8192, "": 0} {
		if got := thinkingBudget(effort); got != mau {
			t.Errorf("thinkingBudget(%q) = %d, mau %d", effort, got, mau)
		}
	}
}

func TestToolDiterjemahkanKeInputSchema(t *testing.T) {
	p, rec := newScriptedServer(t, textResponse)

	req := simpleRequest()
	req.Tools = []providers.Tool{
		{Type: "function", Function: providers.FunctionDef{
			Name:        "cuaca",
			Description: "cari cuaca kota",
			Parameters:  json.RawMessage(`{"type":"object","properties":{"kota":{"type":"string"}},"required":["kota"]}`),
		}},
		{Type: "function", Function: providers.FunctionDef{Name: "tanpa_argumen"}},
		// Jenis tool yang tidak punya padanan dilewati, bukan menggagalkan permintaan.
		{Type: "code_interpreter", Function: providers.FunctionDef{Name: "abaikan-aku"}},
	}
	if _, err := p.ChatCompletion(context.Background(), req); err != nil {
		t.Fatalf("ChatCompletion() error: %v", err)
	}

	body := rec.at(t, 0)
	if len(body.Tools) != 2 {
		t.Fatalf("jumlah tool = %d, mau 2: %+v", len(body.Tools), body.Tools)
	}
	if body.Tools[0].Name != "cuaca" || body.Tools[0].Description != "cari cuaca kota" {
		t.Errorf("tool[0] = %+v", body.Tools[0])
	}
	if body.Tools[0].InputSchema["type"] != "object" {
		t.Errorf("input_schema tidak diteruskan: %v", body.Tools[0].InputSchema)
	}
	if _, ada := body.Tools[0].InputSchema["properties"]; !ada {
		t.Errorf("input_schema kehilangan properties: %v", body.Tools[0].InputSchema)
	}
	// input_schema WAJIB ada di Anthropic walau tool tidak punya argumen.
	if body.Tools[1].InputSchema["type"] != "object" {
		t.Errorf("tool tanpa parameter kehilangan input_schema: %+v", body.Tools[1])
	}
}

func TestToolChoiceDiterjemahkan(t *testing.T) {
	tests := []struct {
		nama     string
		raw      string
		parallel *bool
		mau      map[string]any
	}{
		{"auto", `"auto"`, nil, map[string]any{"type": "auto"}},
		{"none", `"none"`, nil, map[string]any{"type": "none"}},
		{"required menjadi any", `"required"`, nil, map[string]any{"type": "any"}},
		{"fungsi tertentu", `{"type":"function","function":{"name":"cuaca"}}`, nil,
			map[string]any{"type": "tool", "name": "cuaca"}},
		{"bentuk anthropic diteruskan", `{"type":"any"}`, nil, map[string]any{"type": "any"}},
		{"tidak dikenal diabaikan", `"entah"`, nil, nil},
		{"objek tidak dikenal diabaikan", `{"type":"entah"}`, nil, nil},
		{"bukan json diabaikan", `[1,2]`, nil, nil},
		{"parallel false tanpa pilihan tool", ``, ptrBool(false),
			map[string]any{"type": "auto", "disable_parallel_tool_use": true}},
		{"parallel false dengan pilihan tool", `"required"`, ptrBool(false),
			map[string]any{"type": "any", "disable_parallel_tool_use": true}},
		{"parallel true tidak menambah apa pun", `"auto"`, ptrBool(true), map[string]any{"type": "auto"}},
	}

	for _, tc := range tests {
		t.Run(tc.nama, func(t *testing.T) {
			p, rec := newScriptedServer(t, textResponse)
			req := simpleRequest()
			if tc.raw != "" {
				req.ToolChoice = json.RawMessage(tc.raw)
			}
			req.ParallelToolCalls = tc.parallel
			if _, err := p.ChatCompletion(context.Background(), req); err != nil {
				t.Fatalf("ChatCompletion() error: %v", err)
			}

			got := rec.at(t, 0).ToolChoice
			if tc.mau == nil {
				if got != nil {
					t.Fatalf("tool_choice = %v, mau tidak terkirim", got)
				}
				return
			}
			if got == nil {
				t.Fatalf("tool_choice tidak terkirim, mau %v", tc.mau)
			}
			for k, v := range tc.mau {
				if got[k] != v {
					t.Errorf("tool_choice[%q] = %v, mau %v", k, got[k], v)
				}
			}
			if len(got) != len(tc.mau) {
				t.Errorf("tool_choice = %v, mau tepat %v", got, tc.mau)
			}
		})
	}
}

// Alur tool bolak-balik penuh: model meminta tool, hasilnya dikirim balik, model menjawab.
// Inilah pemetaan yang paling mudah salah — hasil tool di Anthropic bukan peran tersendiri,
// melainkan block tool_result di dalam pesan user.
func TestAlurToolBolakBalikPenuh(t *testing.T) {
	toolResponse := `{
	  "id": "msg_tool",
	  "type": "message",
	  "role": "assistant",
	  "model": "claude-sonnet-4-5",
	  "content": [
	    {"type": "text", "text": "sebentar"},
	    {"type": "tool_use", "id": "toolu_A", "name": "cuaca", "input": {"kota": "Bandung"}},
	    {"type": "tool_use", "id": "toolu_B", "name": "waktu", "input": {}}
	  ],
	  "stop_reason": "tool_use",
	  "usage": {"input_tokens": 40, "output_tokens": 12}
	}`
	finalResponse := `{
	  "id": "msg_akhir",
	  "type": "message",
	  "role": "assistant",
	  "model": "claude-sonnet-4-5",
	  "content": [{"type": "text", "text": "Bandung 24 derajat pukul 09:00"}],
	  "stop_reason": "end_turn",
	  "usage": {"input_tokens": 60, "output_tokens": 20}
	}`
	p, rec := newScriptedServer(t, toolResponse, finalResponse)

	tools := []providers.Tool{
		{Type: "function", Function: providers.FunctionDef{
			Name:       "cuaca",
			Parameters: json.RawMessage(`{"type":"object","properties":{"kota":{"type":"string"}}}`),
		}},
		{Type: "function", Function: providers.FunctionDef{Name: "waktu"}},
	}

	// Putaran satu: model meminta dua tool sekaligus.
	first, err := p.ChatCompletion(context.Background(), &providers.ChatRequest{
		Model:    "claude-sonnet-4-5",
		Messages: []providers.Message{{Role: providers.RoleUser, Content: "cuaca dan waktu di Bandung?"}},
		Tools:    tools,
	})
	if err != nil {
		t.Fatalf("putaran 1 error: %v", err)
	}
	if len(first.Choices) != 1 {
		t.Fatalf("jumlah choice = %d", len(first.Choices))
	}
	choice := first.Choices[0]
	if choice.FinishReason != providers.FinishToolCalls {
		t.Errorf("finish reason = %q, mau %q", choice.FinishReason, providers.FinishToolCalls)
	}
	if choice.Message.Content != "sebentar" {
		t.Errorf("teks = %q", choice.Message.Content)
	}
	if len(choice.Message.ToolCalls) != 2 {
		t.Fatalf("jumlah tool call = %d, mau 2", len(choice.Message.ToolCalls))
	}
	call := choice.Message.ToolCalls[0]
	if call.ID != "toolu_A" || call.Function.Name != "cuaca" || call.Type != "function" {
		t.Errorf("tool call[0] = %+v", call)
	}
	if call.Index != 0 || choice.Message.ToolCalls[1].Index != 1 {
		t.Errorf("index tool call salah: %d dan %d", call.Index, choice.Message.ToolCalls[1].Index)
	}
	if !strings.Contains(call.Function.Arguments, `"kota"`) {
		t.Errorf("argumen tool = %q, mau JSON dari field input", call.Function.Arguments)
	}
	if choice.Message.ToolCalls[1].Function.Arguments != "{}" {
		t.Errorf("input kosong seharusnya menjadi {}, dapat %q", choice.Message.ToolCalls[1].Function.Arguments)
	}

	// Putaran dua: hasil kedua tool dikirim balik seperti yang dilakukan klien OpenAI.
	second, err := p.ChatCompletion(context.Background(), &providers.ChatRequest{
		Model: "claude-sonnet-4-5",
		Messages: []providers.Message{
			{Role: providers.RoleUser, Content: "cuaca dan waktu di Bandung?"},
			{Role: providers.RoleAssistant, Content: "sebentar", ToolCalls: choice.Message.ToolCalls},
			{Role: providers.RoleTool, ToolCallID: "toolu_A", Content: `{"suhu":24}`},
			{Role: providers.RoleTool, ToolCallID: "toolu_B", Content: "09:00"},
		},
		Tools: tools,
	})
	if err != nil {
		t.Fatalf("putaran 2 error: %v", err)
	}
	if second.Choices[0].Message.Content != "Bandung 24 derajat pukul 09:00" {
		t.Errorf("jawaban akhir = %q", second.Choices[0].Message.Content)
	}

	body := rec.at(t, 1)
	if len(body.Messages) != 3 {
		t.Fatalf("jumlah pesan = %d, mau 3 (user, assistant, user berisi kedua tool_result): %+v",
			len(body.Messages), body.Messages)
	}

	assistant := body.Messages[1]
	if assistant.Role != roleAssistant {
		t.Fatalf("pesan[1].role = %q", assistant.Role)
	}
	if len(assistant.Content) != 3 {
		t.Fatalf("pesan assistant memuat %d block, mau teks + dua tool_use", len(assistant.Content))
	}
	if assistant.Content[1].Type != blockToolUse || assistant.Content[1].ID != "toolu_A" ||
		assistant.Content[1].Name != "cuaca" {
		t.Errorf("block tool_use = %+v", assistant.Content[1])
	}
	if string(assistant.Content[1].Input) != `{"kota":"Bandung"}` {
		t.Errorf("input tool_use = %s, mau objek utuh", assistant.Content[1].Input)
	}

	results := body.Messages[2]
	if results.Role != roleUser {
		t.Errorf("hasil tool berperan %q, mau user — Anthropic tidak punya peran tool", results.Role)
	}
	if len(results.Content) != 2 {
		t.Fatalf("hasil tool memuat %d block, mau 2 dalam SATU pesan", len(results.Content))
	}
	for i, mauID := range []string{"toolu_A", "toolu_B"} {
		b := results.Content[i]
		if b.Type != blockToolResult {
			t.Errorf("block[%d].type = %q, mau %q", i, b.Type, blockToolResult)
		}
		if b.ToolUseID != mauID {
			t.Errorf("block[%d].tool_use_id = %q, mau %q", i, b.ToolUseID, mauID)
		}
		if len(b.Content) != 1 || b.Content[0].Type != blockText {
			t.Errorf("block[%d].content = %+v, mau satu block text", i, b.Content)
		}
	}
	if results.Content[0].Content[0].Text != `{"suhu":24}` {
		t.Errorf("isi tool_result = %q", results.Content[0].Content[0].Text)
	}
}

func TestHasilToolKosongTetapTerkirim(t *testing.T) {
	p, rec := newScriptedServer(t, textResponse)
	_, err := p.ChatCompletion(context.Background(), &providers.ChatRequest{
		Model: "claude-sonnet-4-5",
		Messages: []providers.Message{
			{Role: providers.RoleUser, Content: "jalankan"},
			{Role: providers.RoleAssistant, ToolCalls: []providers.ToolCall{{
				ID: "toolu_1", Type: "function", Function: providers.FunctionCall{Name: "kosong"},
			}}},
			{Role: providers.RoleTool, ToolCallID: "toolu_1", Content: ""},
		},
	})
	if err != nil {
		t.Fatalf("ChatCompletion() error: %v", err)
	}
	body := rec.at(t, 0)
	last := body.Messages[len(body.Messages)-1]
	if last.Content[0].Type != blockToolResult || last.Content[0].ToolUseID != "toolu_1" {
		t.Errorf("block tool_result hilang: %+v", last)
	}
	if len(last.Content[0].Content) != 0 {
		t.Errorf("hasil kosong seharusnya tanpa block isi: %+v", last.Content[0].Content)
	}
	// Argumen tool kosong tetap menjadi objek kosong, bukan string kosong.
	if got := string(body.Messages[1].Content[0].Input); got != "{}" {
		t.Errorf("input tool_use = %q, mau {}", got)
	}
}

func TestUsageMenjumlahkanTokenCacheSendiri(t *testing.T) {
	body := `{
	  "id": "msg_usage",
	  "type": "message",
	  "role": "assistant",
	  "model": "claude-sonnet-4-5",
	  "content": [{"type": "text", "text": "ya"}],
	  "stop_reason": "end_turn",
	  "usage": {
	    "input_tokens": 10,
	    "output_tokens": 20,
	    "cache_read_input_tokens": 5,
	    "cache_creation_input_tokens": 3
	  }
	}`
	p, _ := newScriptedServer(t, body)

	resp, err := p.ChatCompletion(context.Background(), simpleRequest())
	if err != nil {
		t.Fatalf("ChatCompletion() error: %v", err)
	}

	// input_tokens Anthropic TIDAK memuat token cache, jadi InputTokens kanonik harus
	// menjumlahkan ketiganya — kalau tidak, biaya yang mengurangi token cache dari input
	// bisa negatif pada percakapan dengan cache besar.
	if resp.Usage.InputTokens != 18 {
		t.Errorf("InputTokens = %d, mau 18 (10 + 5 cache read + 3 cache creation)", resp.Usage.InputTokens)
	}
	if resp.Usage.CachedInputTokens != 5 {
		t.Errorf("CachedInputTokens = %d, mau 5", resp.Usage.CachedInputTokens)
	}
	if resp.Usage.OutputTokens != 20 {
		t.Errorf("OutputTokens = %d, mau 20", resp.Usage.OutputTokens)
	}
	if resp.Usage.TotalTokens != 38 {
		t.Errorf("TotalTokens = %d, mau 38", resp.Usage.TotalTokens)
	}
	// Bukti penjumlahan sendiri: menyalin input_tokens + output_tokens saja menghasilkan 30.
	if resp.Usage.TotalTokens == 30 {
		t.Error("TotalTokens mengabaikan token cache")
	}
}

func TestPemetaanStopReason(t *testing.T) {
	tests := map[string]string{
		"end_turn":      providers.FinishStop,
		"stop_sequence": providers.FinishStop,
		"pause_turn":    providers.FinishStop,
		"max_tokens":    providers.FinishLength,
		"tool_use":      providers.FinishToolCalls,
		"refusal":       providers.FinishContentFilter,
		"":              "",
		"alasan_baru":   providers.FinishStop,
	}
	for upstream, mau := range tests {
		if got := mapStopReason(upstream); got != mau {
			t.Errorf("mapStopReason(%q) = %q, mau %q", upstream, got, mau)
		}
	}

	p, _ := newScriptedServer(t, `{
	  "id":"msg_len","model":"claude-sonnet-4-5",
	  "content":[{"type":"text","text":"terpotong"}],
	  "stop_reason":"max_tokens",
	  "usage":{"input_tokens":1,"output_tokens":2}
	}`)
	resp, err := p.ChatCompletion(context.Background(), simpleRequest())
	if err != nil {
		t.Fatalf("ChatCompletion() error: %v", err)
	}
	if resp.Choices[0].FinishReason != providers.FinishLength {
		t.Errorf("FinishReason = %q, mau %q", resp.Choices[0].FinishReason, providers.FinishLength)
	}
}

func TestBlockPenalaranDipisahDariJawaban(t *testing.T) {
	p, rec := newScriptedServer(t, `{
	  "id":"msg_think","model":"claude-sonnet-4-5",
	  "content":[
	    {"type":"thinking","thinking":"pikir dulu","signature":"tanda-tangan"},
	    {"type":"redacted_thinking","data":"terenkripsi"},
	    {"type":"text","text":"jawaban"}
	  ],
	  "stop_reason":"end_turn",
	  "usage":{"input_tokens":1,"output_tokens":2}
	}`, textResponse)

	resp, err := p.ChatCompletion(context.Background(), simpleRequest())
	if err != nil {
		t.Fatalf("ChatCompletion() error: %v", err)
	}
	msg := resp.Choices[0].Message
	if msg.Content != "jawaban" {
		t.Errorf("Content = %q, mau hanya jawaban", msg.Content)
	}
	if msg.ReasoningContent != "pikir dulu" {
		t.Errorf("ReasoningContent = %q", msg.ReasoningContent)
	}

	// Jejak penalaran TIDAK dikirim ulang: Anthropic menolak block thinking tanpa signature.
	if _, err := p.ChatCompletion(context.Background(), &providers.ChatRequest{
		Model: "claude-sonnet-4-5",
		Messages: []providers.Message{
			{Role: providers.RoleUser, Content: "halo"},
			{Role: providers.RoleAssistant, Content: "jawaban", ReasoningContent: "pikir dulu"},
			{Role: providers.RoleUser, Content: "lanjut"},
		},
	}); err != nil {
		t.Fatalf("ChatCompletion() error: %v", err)
	}
	for _, m := range rec.at(t, 1).Messages {
		for _, b := range m.Content {
			if b.Type == blockThinking || b.Thinking != "" {
				t.Errorf("block thinking terkirim ulang: %+v", b)
			}
		}
	}
}

func TestMultimodalGambarDiterjemahkan(t *testing.T) {
	p, rec := newScriptedServer(t, textResponse)

	req := &providers.ChatRequest{
		Model: "claude-sonnet-4-5",
		Messages: []providers.Message{{Role: providers.RoleUser, Parts: []providers.ContentPart{
			{Type: providers.PartTypeText, Text: "apa ini?"},
			{Type: providers.PartTypeText, Text: ""},
			{Type: providers.PartTypeImageURL, ImageURL: &providers.ImageURL{
				URL: "data:image/png;charset=utf-8;base64,AAAB", Detail: "high",
			}},
			{Type: providers.PartTypeImageURL, ImageURL: &providers.ImageURL{URL: "https://contoh.test/a.png"}},
			{Type: providers.PartTypeImageURL, ImageURL: &providers.ImageURL{URL: ""}},
			{Type: providers.PartTypeImageURL},
			{Type: "jenis_baru", Text: "diabaikan"},
		}}},
	}
	if _, err := p.ChatCompletion(context.Background(), req); err != nil {
		t.Fatalf("ChatCompletion() error: %v", err)
	}

	blocks := rec.at(t, 0).Messages[0].Content
	if len(blocks) != 3 {
		t.Fatalf("jumlah block = %d, mau 3 (teks + dua gambar): %+v", len(blocks), blocks)
	}
	if blocks[1].Source["type"] != "base64" || blocks[1].Source["media_type"] != "image/png" ||
		blocks[1].Source["data"] != "AAAB" {
		t.Errorf("data URI tidak terurai: %v", blocks[1].Source)
	}
	if blocks[2].Source["type"] != "url" || blocks[2].Source["url"] != "https://contoh.test/a.png" {
		t.Errorf("URL gambar tidak diteruskan: %v", blocks[2].Source)
	}
}

func TestParseDataURI(t *testing.T) {
	tests := []struct {
		raw         string
		mauType     string
		mauData     string
		mauBerhasil bool
	}{
		{"data:image/jpeg;base64,QUJD", "image/jpeg", "QUJD", true},
		{"data:;base64,QUJD", "application/octet-stream", "QUJD", true},
		{"data:image/png,QUJD", "", "", false},
		{"data:image/png;base64,", "", "", false},
		{"https://contoh.test/a.png", "", "", false},
		{"data:tanpa-koma", "", "", false},
	}
	for _, tc := range tests {
		gotType, gotData, ok := parseDataURI(tc.raw)
		if ok != tc.mauBerhasil || gotType != tc.mauType || gotData != tc.mauData {
			t.Errorf("parseDataURI(%q) = (%q, %q, %v), mau (%q, %q, %v)",
				tc.raw, gotType, gotData, ok, tc.mauType, tc.mauData, tc.mauBerhasil)
		}
	}
}

func TestUserIDDipangkas(t *testing.T) {
	p, rec := newScriptedServer(t, textResponse)
	req := simpleRequest()
	req.User = strings.Repeat("u", maxMetadataUserIDLen+50)
	if _, err := p.ChatCompletion(context.Background(), req); err != nil {
		t.Fatalf("ChatCompletion() error: %v", err)
	}
	got, _ := rec.at(t, 0).Metadata["user_id"].(string)
	if len(got) != maxMetadataUserIDLen {
		t.Errorf("panjang user_id = %d, mau %d", len(got), maxMetadataUserIDLen)
	}
}

// Raw wajib berisi bentuk dialek OpenAI hasil terjemahan, karena itulah yang diteruskan ke
// klien gateway — bukan body Anthropic apa adanya.
func TestRawBerisiDialekOpenAI(t *testing.T) {
	upstream := `{
	  "id": "msg_raw",
	  "type": "message",
	  "role": "assistant",
	  "model": "claude-sonnet-4-5",
	  "content": [
	    {"type":"thinking","thinking":"pikir"},
	    {"type": "tool_use", "id": "toolu_R", "name": "cuaca", "input": {"kota":"Solo"}}
	  ],
	  "stop_reason": "tool_use",
	  "usage": {"input_tokens": 7, "output_tokens": 9, "cache_read_input_tokens": 2}
	}`
	p, _ := newScriptedServer(t, upstream)

	resp, err := p.ChatCompletion(context.Background(), simpleRequest())
	if err != nil {
		t.Fatalf("ChatCompletion() error: %v", err)
	}

	var raw struct {
		ID      string `json:"id"`
		Object  string `json:"object"`
		Created int64  `json:"created"`
		Model   string `json:"model"`
		Choices []struct {
			Index   int `json:"index"`
			Message struct {
				Role             string          `json:"role"`
				Content          json.RawMessage `json:"content"`
				ReasoningContent string          `json:"reasoning_content"`
				ToolCalls        []struct {
					ID       string `json:"id"`
					Type     string `json:"type"`
					Index    *int   `json:"index"`
					Function struct {
						Name      string `json:"name"`
						Arguments string `json:"arguments"`
					} `json:"function"`
				} `json:"tool_calls"`
			} `json:"message"`
			FinishReason *string `json:"finish_reason"`
		} `json:"choices"`
		Usage struct {
			PromptTokens        int `json:"prompt_tokens"`
			CompletionTokens    int `json:"completion_tokens"`
			TotalTokens         int `json:"total_tokens"`
			PromptTokensDetails *struct {
				CachedTokens int `json:"cached_tokens"`
			} `json:"prompt_tokens_details"`
		} `json:"usage"`
	}
	if err := json.Unmarshal(resp.Raw, &raw); err != nil {
		t.Fatalf("Raw bukan JSON dialek OpenAI: %v\nraw: %s", err, resp.Raw)
	}
	if raw.Object != "chat.completion" {
		t.Errorf("object = %q, mau chat.completion", raw.Object)
	}
	if raw.ID != "msg_raw" || raw.Model != "claude-sonnet-4-5" || raw.Created == 0 {
		t.Errorf("identitas respons salah: %+v", raw)
	}
	if len(raw.Choices) != 1 {
		t.Fatalf("jumlah choice = %d", len(raw.Choices))
	}
	choice := raw.Choices[0]
	if choice.Message.Role != "assistant" {
		t.Errorf("role = %q", choice.Message.Role)
	}
	// Jawaban tanpa teks memakai content null, sama seperti OpenAI.
	if string(choice.Message.Content) != "null" {
		t.Errorf("content = %s, mau null saat hanya ada tool call", choice.Message.Content)
	}
	if choice.Message.ReasoningContent != "pikir" {
		t.Errorf("reasoning_content = %q", choice.Message.ReasoningContent)
	}
	if choice.FinishReason == nil || *choice.FinishReason != providers.FinishToolCalls {
		t.Errorf("finish_reason = %v, mau tool_calls", choice.FinishReason)
	}
	if len(choice.Message.ToolCalls) != 1 {
		t.Fatalf("jumlah tool call = %d", len(choice.Message.ToolCalls))
	}
	tc := choice.Message.ToolCalls[0]
	if tc.ID != "toolu_R" || tc.Type != "function" || tc.Function.Name != "cuaca" {
		t.Errorf("tool call = %+v", tc)
	}
	if tc.Function.Arguments != `{"kota":"Solo"}` {
		t.Errorf("arguments = %q, mau string JSON", tc.Function.Arguments)
	}
	if tc.Index != nil {
		t.Error("index tool call hanya boleh muncul pada aliran")
	}
	if raw.Usage.PromptTokens != 9 || raw.Usage.CompletionTokens != 9 || raw.Usage.TotalTokens != 18 {
		t.Errorf("usage = %+v, mau prompt 9 (7+2 cache), completion 9, total 18", raw.Usage)
	}
	if raw.Usage.PromptTokensDetails == nil || raw.Usage.PromptTokensDetails.CachedTokens != 2 {
		t.Errorf("cached_tokens tidak dilaporkan: %+v", raw.Usage.PromptTokensDetails)
	}
}

func TestTeksBeberapaBlockDisambungTanpaPemisahBaru(t *testing.T) {
	p, _ := newScriptedServer(t, `{
	  "id":"msg_multi","model":"claude-sonnet-4-5",
	  "content":[{"type":"text","text":"satu "},{"type":"text","text":"dua"}],
	  "stop_reason":"end_turn",
	  "usage":{"input_tokens":1,"output_tokens":1}
	}`)
	resp, err := p.ChatCompletion(context.Background(), simpleRequest())
	if err != nil {
		t.Fatalf("ChatCompletion() error: %v", err)
	}
	if got := resp.Choices[0].Message.Content; got != "satu dua" {
		t.Errorf("Content = %q, mau %q tanpa karakter tambahan", got, "satu dua")
	}
}
