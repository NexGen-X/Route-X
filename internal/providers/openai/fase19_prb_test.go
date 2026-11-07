package openai

import (
	"context"
	"encoding/json"
	"io"
	"net/http"
	"strings"
	"testing"
	"time"

	"github.com/NexGen-X/Route-X/internal/providers"
)

// TestB432SignatureNumerikToolCall adalah pengunci B-432:
// thought_signature NUMERIK pada tool call dulu menggagalkan
// SELURUH respons (wireToolCall memakai string biasa), persis
// pola B-263 yang adapter anthropic/google sudah kebal lewat
// SignatureToleran. Kini adapter ini toleran juga, di jalur
// non-streaming maupun streaming.
func TestB432SignatureNumerikToolCall(t *testing.T) {
	u := newUpstream(t, jsonHandler(200,
		`{"id":"chatcmpl-b432","object":"chat.completion","created":1730000000,"model":"gemini-2.5-pro","choices":[{"index":0,"message":{"role":"assistant","content":null,"tool_calls":[{"id":"call_b432","type":"function","function":{"name":"cari","arguments":"{\"q\":\"x\"}"},"thought_signature":12345}]},"finish_reason":"tool_calls"}],"usage":{"prompt_tokens":10,"completion_tokens":5,"total_tokens":15}}`))
	p := testProvider(t, u.url(), nil)

	resp, err := p.ChatCompletion(context.Background(), chatRequest())
	if err != nil {
		t.Fatalf("ChatCompletion gagal untuk signature numerik: %v", err)
	}
	if len(resp.Choices) != 1 || len(resp.Choices[0].Message.ToolCalls) != 1 {
		t.Fatalf("choices/toolcalls = %+v, mau 1/1", resp.Choices)
	}
	if got := resp.Choices[0].Message.ToolCalls[0].ThoughtSignature; got != "12345" {
		t.Errorf("ThoughtSignature = %q, mau \"12345\" (teks literal angka)", got)
	}

	// Jalur streaming memakai struct yang sama di parseChunk.
	ev, err := parseChunk([]byte(
		`{"choices":[{"index":0,"delta":{"tool_calls":[{"index":0,"id":"call_s","type":"function","function":{"name":"cari","arguments":"{}"},"thought_signature":999}]},"finish_reason":null}]}`))
	if err != nil {
		t.Fatalf("parseChunk gagal untuk signature numerik: %v", err)
	}
	if len(ev.ToolCalls) != 1 || ev.ToolCalls[0].ThoughtSignature != "999" {
		t.Errorf("ToolCalls streaming = %+v, mau satu call ber-signature \"999\"", ev.ToolCalls)
	}
}

// TestB425ThoughtSignatureTingkatPesan adalah pengunci B-425:
// thought_signature TINGKAT PESAN hilang di adapter ini pada
// tiga arah — respons non-streaming, delta streaming, dan
// riwayat masuk. Kini ia berjalan sebagai ReasoningSignature
// kanonik, sejajar adapter anthropic/google (B-390).
func TestB425ThoughtSignatureTingkatPesan(t *testing.T) {
	t.Run("respons non-streaming", func(t *testing.T) {
		u := newUpstream(t, jsonHandler(200,
			`{"id":"chatcmpl-b425","object":"chat.completion","created":1730000000,"model":"m","choices":[{"index":0,"message":{"role":"assistant","content":"jawaban","thought_signature":"sig-pesan-b425"},"finish_reason":"stop"}],"usage":{"prompt_tokens":3,"completion_tokens":2,"total_tokens":5}}`))
		p := testProvider(t, u.url(), nil)
		resp, err := p.ChatCompletion(context.Background(), chatRequest())
		if err != nil {
			t.Fatalf("ChatCompletion: %v", err)
		}
		if got := resp.Choices[0].Message.ReasoningSignature; got != "sig-pesan-b425" {
			t.Errorf("ReasoningSignature = %q, mau sig-pesan-b425", got)
		}
	})

	t.Run("delta streaming", func(t *testing.T) {
		ev, err := parseChunk([]byte(
			`{"choices":[{"index":0,"delta":{"content":"halo","thought_signature":"sig-delta-b425"},"finish_reason":null}]}`))
		if err != nil {
			t.Fatalf("parseChunk: %v", err)
		}
		if ev.ReasoningSignature != "sig-delta-b425" {
			t.Errorf("ReasoningSignature delta = %q, mau sig-delta-b425", ev.ReasoningSignature)
		}
	})

	t.Run("arah masuk riwayat", func(t *testing.T) {
		u := newUpstream(t, jsonHandler(200,
			`{"id":"chatcmpl-b425b","object":"chat.completion","created":1730000000,"model":"m","choices":[{"index":0,"message":{"role":"assistant","content":"ok"},"finish_reason":"stop"}],"usage":{"prompt_tokens":3,"completion_tokens":1,"total_tokens":4}}`))
		p := testProvider(t, u.url(), nil)
		req := chatRequest()
		req.Messages = append(req.Messages,
			providers.Message{Role: providers.RoleAssistant, Content: "jawaban lama", ReasoningSignature: "sig-riwayat-b425"},
			providers.Message{Role: providers.RoleUser, Content: "lanjutkan"},
		)
		if _, err := p.ChatCompletion(context.Background(), req); err != nil {
			t.Fatalf("ChatCompletion riwayat: %v", err)
		}
		body := u.lastBody()
		msgs, _ := body["messages"].([]any)
		ketemu := false
		for _, m := range msgs {
			msg, _ := m.(map[string]any)
			if msg["thought_signature"] == "sig-riwayat-b425" {
				ketemu = true
			}
		}
		if !ketemu {
			b, _ := json.Marshal(body["messages"])
			t.Errorf("body keluar tidak memuat thought_signature tingkat pesan: %s", b)
		}
	})
}

// TestB431RetryAfterBody200OpenAI adalah pengunci B-431 (sisi
// OpenAI): respons 200 ber-body error rate-limit dengan header
// Retry-After: 17 dulu menghasilkan RetryAfter 0 — header dibuang
// di jalur errorFromSuccessBody. Kini header terbaca.
func TestB431RetryAfterBody200OpenAI(t *testing.T) {
	u := newUpstream(t, func(w http.ResponseWriter, _ *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		w.Header().Set("Retry-After", "17")
		w.WriteHeader(200)
		_, _ = io.WriteString(w, `{"error":{"message":"pelan-pelan","type":"rate_limit_error","code":"rate_limit_exceeded"}}`)
	})
	p := testProvider(t, u.url(), nil)
	_, err := p.ChatCompletion(context.Background(), chatRequest())
	pe := providers.AsError(err)
	if pe == nil {
		t.Fatalf("err = %v, mau *providers.Error", err)
	}
	if pe.Kind != providers.ErrKindRateLimit {
		t.Errorf("kind = %q, mau rate_limit", pe.Kind)
	}
	if pe.RetryAfter != 17*time.Second {
		t.Errorf("RetryAfter = %v, mau 17s — header body-200 dibuang", pe.RetryAfter)
	}
	if !strings.Contains(pe.Error(), "rate_limit") {
		t.Errorf("error = %q", pe.Error())
	}
}
