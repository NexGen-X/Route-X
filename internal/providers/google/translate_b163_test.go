package google

import (
	"context"
	"encoding/json"
	"net/http"
	"sync"
	"testing"

	"github.com/NexGen-X/Route-X/internal/providers"
)

// TestB163IDToolCallCodeAssist adalah test pengunci sisi kode B-163: id panggilan
// tool dari upstream Code Assist (tool_use.id Vertex) dipertahankan ke bentuk
// kanonik dan dipasang kembali pada part functionCall riwayat KHUSUS mode
// Antigravity. Jalur Gemini murni tidak berubah satu byte pun (gerbang probe wire
// untuk permukaan Gemini tetap terbuka — lihat laporan PR).
func TestB163IDToolCallCodeAssist(t *testing.T) {
	toolResponse := `{"response": {
	  "candidates": [{
	    "content": {"role": "model", "parts": [
	      {"functionCall": {"id": "toolu_01B163", "name": "tulis_berkas", "args": {"path": "hasil.txt"}}}
	    ]},
	    "finishReason": "STOP",
	    "index": 0
	  }],
	  "usageMetadata": {"promptTokenCount": 40, "candidatesTokenCount": 12, "totalTokenCount": 52}
	}}`
	finalResponse := `{"response": {
	  "candidates": [{
	    "content": {"role": "model", "parts": [{"text": "berkas tertulis"}]},
	    "finishReason": "STOP"
	  }],
	  "usageMetadata": {"promptTokenCount": 60, "candidatesTokenCount": 20, "totalTokenCount": 80}
	}}`

	var mu sync.Mutex
	next := 0
	responses := []string{toolResponse, finalResponse}
	p, log := newAntigravityServer(t, "proj-uji", func(w http.ResponseWriter, _ *http.Request) {
		mu.Lock()
		idx := next
		next++
		mu.Unlock()
		if idx >= len(responses) {
			idx = len(responses) - 1
		}
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(responses[idx]))
	})

	tools := []providers.Tool{{Type: "function", Function: providers.FunctionDef{
		Name:       "tulis_berkas",
		Parameters: json.RawMessage(`{"type":"object","properties":{"path":{"type":"string"}}}`),
	}}}

	first, err := p.ChatCompletion(context.Background(), &providers.ChatRequest{
		Model:    "claude-sonnet-5-5-medium",
		Messages: []providers.Message{{Role: providers.RoleUser, Content: "tulis hasil.txt"}},
		Tools:    tools,
	})
	if err != nil {
		t.Fatalf("putaran 1 error: %v", err)
	}
	calls := first.Choices[0].Message.ToolCalls
	if len(calls) != 1 {
		t.Fatalf("jumlah tool call = %d, mau 1", len(calls))
	}
	if calls[0].ID != "toolu_01B163" {
		t.Errorf("ToolCall.ID = %q, mau %q (id upstream harus dipertahankan, bukan disintesis)", calls[0].ID, "toolu_01B163")
	}

	// Putaran dua: riwayat yang sama dikirim balik; part functionCall di dalam
	// amplop Code Assist harus membawa id yang sama.
	_, err = p.ChatCompletion(context.Background(), &providers.ChatRequest{
		Model: "claude-sonnet-5-5-medium",
		Messages: []providers.Message{
			{Role: providers.RoleUser, Content: "tulis hasil.txt"},
			{Role: providers.RoleAssistant, ToolCalls: calls},
			{Role: providers.RoleTool, ToolCallID: calls[0].ID, Content: `{"ok":true}`},
		},
		Tools: tools,
	})
	if err != nil {
		t.Fatalf("putaran 2 error: %v", err)
	}
	var envelope struct {
		Request wireRequest `json:"request"`
	}
	if err := json.Unmarshal(log.raw(t, 1), &envelope); err != nil {
		t.Fatalf("amplop permintaan bukan JSON: %v", err)
	}
	model := envelope.Request.Contents[1]
	if len(model.Parts) != 1 || model.Parts[0].FunctionCall == nil {
		t.Fatalf("contents model = %+v, mau satu functionCall", model.Parts)
	}
	if model.Parts[0].FunctionCall.ID != "toolu_01B163" {
		t.Errorf("functionCall.id pada riwayat = %q, mau %q", model.Parts[0].FunctionCall.ID, "toolu_01B163")
	}
}

// TestB163JalurGeminiMurniTidakBerubah membuktikan sisi aman gerbang probe: pada
// provider Gemini murni, part functionCall riwayat TIDAK memuat field id sama
// sekali — perilaku sebelum B-163 persis.
func TestB163JalurGeminiMurniTidakBerubah(t *testing.T) {
	p, log := newScriptedServer(t, textResponse)
	_, err := p.ChatCompletion(context.Background(), &providers.ChatRequest{
		Model: "gemini-2.5-flash",
		Messages: []providers.Message{
			{Role: providers.RoleUser, Content: "cuaca?"},
			{Role: providers.RoleAssistant, ToolCalls: []providers.ToolCall{{
				ID:       "call_0_cuaca",
				Type:     "function",
				Function: providers.FunctionCall{Name: "cuaca", Arguments: `{"kota":"Bandung"}`},
			}}},
			{Role: providers.RoleTool, ToolCallID: "call_0_cuaca", Content: `{"suhu":24}`},
		},
	})
	if err != nil {
		t.Fatalf("ChatCompletion error: %v", err)
	}
	body := decodeRequest(t, log.raw(t, 0))
	model := body.Contents[1]
	if len(model.Parts) != 1 || model.Parts[0].FunctionCall == nil {
		t.Fatalf("contents model = %+v, mau satu functionCall", model.Parts)
	}
	if model.Parts[0].FunctionCall.ID != "" {
		t.Errorf("functionCall.id = %q, mau kosong pada jalur Gemini murni", model.Parts[0].FunctionCall.ID)
	}
}
