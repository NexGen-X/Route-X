package openai

import (
	"context"
	"encoding/json"
	"testing"

	"github.com/NexGen-X/Route-X/internal/providers"
)

// TestB371ThoughtSignatureToolCallDuaArah adalah pengunci B-371:
// codec gateway membawa thought_signature bolak-balik pada
// riwayat sebagai pengecualian bernama (B-162) dan adapter Google
// menyalinnya, tetapi wireToolCall adapter ini membuangnya di
// kedua arah — respons upstream bergaya Gemini kehilangan
// signature saat diurai, dan riwayat yang dikirim ulang ke
// upstream kehilangannya lagi, padahal Gemini menolak riwayat
// function call tanpa signature pada giliran lanjutan.
func TestB371ThoughtSignatureToolCallDuaArah(t *testing.T) {
	u := newUpstream(t, jsonHandler(200,
		`{"id":"chatcmpl-371","object":"chat.completion","created":1730000000,"model":"gemini-2.5-pro","choices":[{"index":0,"message":{"role":"assistant","content":null,"tool_calls":[{"id":"call_371","type":"function","function":{"name":"cari","arguments":"{\"q\":\"x\"}"},"thought_signature":"sig-oai-371"}]},"finish_reason":"tool_calls"}],"usage":{"prompt_tokens":10,"completion_tokens":5,"total_tokens":15}}`))
	p := testProvider(t, u.url(), nil)

	resp, err := p.ChatCompletion(context.Background(), chatRequest())
	if err != nil {
		t.Fatalf("ChatCompletion: %v", err)
	}
	if len(resp.Choices) != 1 || len(resp.Choices[0].Message.ToolCalls) != 1 {
		t.Fatalf("choices/toolcalls = %+v, mau 1/1", resp.Choices)
	}
	tc := resp.Choices[0].Message.ToolCalls[0]
	if tc.ThoughtSignature != "sig-oai-371" {
		t.Fatalf("ThoughtSignature kanonik = %q, mau sig-oai-371 — terbuang saat penguraian respons", tc.ThoughtSignature)
	}

	// Arah keluar: riwayat yang memuat tool call itu dikirim ulang.
	riwayat := chatRequest()
	riwayat.Messages = append(riwayat.Messages,
		providers.Message{Role: providers.RoleAssistant, ToolCalls: []providers.ToolCall{tc}},
		providers.Message{Role: providers.RoleUser, Content: "lanjutkan"},
	)
	if _, err := p.ChatCompletion(context.Background(), riwayat); err != nil {
		t.Fatalf("ChatCompletion riwayat: %v", err)
	}
	body := u.lastBody()
	pesan, _ := body["messages"].([]any)
	var ketemu bool
	for _, m := range pesan {
		msg, _ := m.(map[string]any)
		calls, _ := msg["tool_calls"].([]any)
		for _, c := range calls {
			call, _ := c.(map[string]any)
			if call["id"] == "call_371" {
				ketemu = true
				if call["thought_signature"] != "sig-oai-371" {
					b, _ := json.Marshal(call)
					t.Errorf("tool call keluar = %s, mau memuat thought_signature sig-oai-371", b)
				}
			}
		}
	}
	if !ketemu {
		t.Fatalf("tool call call_371 tidak ditemukan pada body keluar: %v", body["messages"])
	}
}
