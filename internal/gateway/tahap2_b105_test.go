package gateway

// Test kontrak permukaan Responses API (B-105): terjemahan dialek masuk,
// bentuk objek response keluar, dan penolakan eksplisit di luar subset.

import (
	"encoding/json"
	"strings"
	"testing"

	"github.com/NexGen-X/Route-X/internal/providers"
)

func TestUraiResponsesInputStringDanInstruksi(t *testing.T) {
	req, err := uraiPermintaanResponses([]byte(`{
		"model": "m1", "input": "halo dunia", "instructions": "Jawab singkat.",
		"max_output_tokens": 42, "temperature": 0.5
	}`))
	if err != nil {
		t.Fatal(err)
	}
	if len(req.Messages) != 2 {
		t.Fatalf("pesan = %d, mau 2 (system+user)", len(req.Messages))
	}
	if req.Messages[0].Role != providers.RoleSystem || req.Messages[0].Content != "Jawab singkat." {
		t.Fatalf("pesan system salah: %+v", req.Messages[0])
	}
	if req.Messages[1].Role != providers.RoleUser || req.Messages[1].Content != "halo dunia" {
		t.Fatalf("pesan user salah: %+v", req.Messages[1])
	}
	if req.MaxTokens == nil || *req.MaxTokens != 42 {
		t.Fatalf("max_output_tokens tidak diterjemahkan: %+v", req.MaxTokens)
	}
}

func TestUraiResponsesItemCampuran(t *testing.T) {
	req, err := uraiPermintaanResponses([]byte(`{
		"model": "m1",
		"input": [
			{"type": "message", "role": "user", "content": [{"type": "input_text", "text": "cuaca?"}]},
			{"type": "function_call", "call_id": "call_1", "name": "cuaca", "arguments": "{\"kota\":\"Jakarta\"}"},
			{"type": "function_call", "call_id": "call_2", "name": "waktu", "arguments": "{}"},
			{"type": "function_call_output", "call_id": "call_1", "output": "cerah"},
			{"type": "reasoning", "summary": []}
		],
		"tools": [{"type": "function", "name": "cuaca", "description": "d", "parameters": {"type": "object"}, "strict": true}],
		"tool_choice": {"type": "function", "name": "cuaca"}
	}`))
	if err != nil {
		t.Fatal(err)
	}
	// user, assistant(2 panggilan tergabung), tool = 3 pesan.
	if len(req.Messages) != 3 {
		t.Fatalf("pesan = %d, mau 3: %+v", len(req.Messages), req.Messages)
	}
	asst := req.Messages[1]
	if asst.Role != providers.RoleAssistant || len(asst.ToolCalls) != 2 {
		t.Fatalf("function_call berurutan tidak tergabung: %+v", asst)
	}
	if asst.ToolCalls[0].ID != "call_1" || asst.ToolCalls[0].Function.Name != "cuaca" {
		t.Fatalf("tool call pertama salah: %+v", asst.ToolCalls[0])
	}
	tool := req.Messages[2]
	if tool.Role != providers.RoleTool || tool.ToolCallID != "call_1" || tool.Content != "cerah" {
		t.Fatalf("function_call_output salah: %+v", tool)
	}
	if len(req.Tools) != 1 || req.Tools[0].Function.Name != "cuaca" ||
		req.Tools[0].Function.Strict == nil || !*req.Tools[0].Function.Strict {
		t.Fatalf("tools bentuk datar tidak diterjemahkan: %+v", req.Tools)
	}
	if !strings.Contains(string(req.ToolChoice), `"function":{"name":"cuaca"}`) {
		t.Fatalf("tool_choice tidak diterjemahkan ke bentuk chat: %s", req.ToolChoice)
	}
}

func TestUraiResponsesPenolakanEksplisit(t *testing.T) {
	kasus := []struct {
		nama string
		body string
	}{
		{"stream", `{"model":"m","input":"x","stream":true}`},
		{"previous_response_id", `{"model":"m","input":"x","previous_response_id":"resp_1"}`},
		{"item tak dikenal", `{"model":"m","input":[{"type":"web_search_call","id":"ws_1"}]}`},
		{"input_file", `{"model":"m","input":[{"type":"message","role":"user","content":[{"type":"input_file","file_id":"f1"}]}]}`},
		{"item_reference", `{"model":"m","input":[{"type":"item_reference","id":"i1"}]}`},
		{"tanpa model", `{"input":"x"}`},
		{"tanpa input", `{"model":"m"}`},
	}
	for _, k := range kasus {
		t.Run(k.nama, func(t *testing.T) {
			if _, err := uraiPermintaanResponses([]byte(k.body)); err == nil {
				t.Fatal("diharapkan penolakan eksplisit, malah lolos")
			}
		})
	}
	// Kode khusus streaming harus bisa dibedakan pemanggil.
	_, err := uraiPermintaanResponses([]byte(`{"model":"m","input":"x","stream":true}`))
	ke, ok := err.(*kekeliruanResponses)
	if !ok || ke.kode != "streaming_not_supported" {
		t.Fatalf("kode streaming salah: %v", err)
	}
}

func TestUraiResponsesFormatTeksJSONSchema(t *testing.T) {
	req, err := uraiPermintaanResponses([]byte(`{
		"model": "m", "input": "x",
		"text": {"format": {"type": "json_schema", "name": "orang", "schema": {"type": "object"}, "strict": true}}
	}`))
	if err != nil {
		t.Fatal(err)
	}
	s := string(req.ResponseFormat)
	if !strings.Contains(s, `"json_schema":{"name":"orang"`) || !strings.Contains(s, `"strict":true`) {
		t.Fatalf("format json_schema tidak diterjemahkan ke bentuk bersarang chat: %s", s)
	}
}

func TestKodekanResponsResponsesBentukBenar(t *testing.T) {
	kanonik := &providers.ChatResponse{
		ID: "chatcmpl-abc", Model: "upstream-1", Created: 1700000000,
		Choices: []providers.Choice{{
			FinishReason: "tool_calls",
			Message: providers.Message{
				Role:    providers.RoleAssistant,
				Content: "sebentar",
				ToolCalls: []providers.ToolCall{{
					ID: "call_9", Type: "function",
					Function: providers.FunctionCall{Name: "cuaca", Arguments: "{}"},
				}},
			},
		}},
		Usage: providers.Usage{InputTokens: 5, OutputTokens: 7, TotalTokens: 12},
	}
	var obj map[string]any
	if err := json.Unmarshal(kodekanResponsResponses(kanonik, "m-publik", false), &obj); err != nil {
		t.Fatal(err)
	}
	if obj["object"] != "response" || obj["status"] != "completed" {
		t.Fatalf("kerangka objek salah: %v / %v", obj["object"], obj["status"])
	}
	if id, _ := obj["id"].(string); !strings.HasPrefix(id, "resp_") {
		t.Fatalf("id tidak berawalan resp_: %v", obj["id"])
	}
	if obj["model"] != "upstream-1" || obj["created_at"].(float64) != 1700000000 {
		t.Fatalf("model/created_at salah: %v %v", obj["model"], obj["created_at"])
	}
	out, _ := obj["output"].([]any)
	if len(out) != 2 {
		t.Fatalf("output = %d item, mau 2 (message+function_call)", len(out))
	}
	pesan, _ := out[0].(map[string]any)
	if pesan["type"] != "message" || pesan["role"] != "assistant" {
		t.Fatalf("item pertama bukan message assistant: %v", pesan)
	}
	isi, _ := pesan["content"].([]any)
	if len(isi) != 1 || isi[0].(map[string]any)["type"] != "output_text" || isi[0].(map[string]any)["text"] != "sebentar" {
		t.Fatalf("content message salah: %v", isi)
	}
	fc, _ := out[1].(map[string]any)
	if fc["type"] != "function_call" || fc["call_id"] != "call_9" || fc["name"] != "cuaca" || fc["status"] != "completed" {
		t.Fatalf("item function_call salah: %v", fc)
	}
	u, _ := obj["usage"].(map[string]any)
	if u["input_tokens"].(float64) != 5 || u["output_tokens"].(float64) != 7 || u["total_tokens"].(float64) != 12 {
		t.Fatalf("usage salah: %v", u)
	}
}

func TestKodekanResponsResponsesTidakTuntasSaatLength(t *testing.T) {
	kanonik := &providers.ChatResponse{
		ID: "chatcmpl-x", Model: "u",
		Choices: []providers.Choice{{
			FinishReason: "length",
			Message:      providers.Message{Role: providers.RoleAssistant, Content: "terpotong"},
		}},
	}
	var obj map[string]any
	if err := json.Unmarshal(kodekanResponsResponses(kanonik, "m", false), &obj); err != nil {
		t.Fatal(err)
	}
	if obj["status"] != "incomplete" {
		t.Fatalf("status = %v, mau incomplete untuk finish length", obj["status"])
	}
	inc, _ := obj["incomplete_details"].(map[string]any)
	if inc["reason"] != "max_output_tokens" {
		t.Fatalf("incomplete_details salah: %v", inc)
	}
}
