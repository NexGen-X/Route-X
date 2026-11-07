package anthropic

import (
	"context"
	"io"
	"net/http"
	"strings"
	"testing"
	"time"

	"github.com/NexGen-X/Route-X/internal/providers"
)

// TestB426RefusalKosongContentFilter adalah pengunci B-426:
// stop_reason refusal TANPA konten dulu tertangkap penjaga
// kekosongan B-261 sebagai kind server (retryable+failoverable).
// Kini refusal — kosong maupun berteks — adalah respons sah
// ber-finish content_filter, sejajar penjaga B-298 google/openai.
func TestB426RefusalKosongContentFilter(t *testing.T) {
	t.Run("refusal kosong", func(t *testing.T) {
		p, _ := newServer(t, jsonHandler(200, `{
			"id": "msg_b426", "type": "message", "role": "assistant",
			"model": "claude-sonnet-4-5",
			"content": [],
			"stop_reason": "refusal",
			"usage": {"input_tokens": 10, "output_tokens": 0}
		}`))
		resp, err := p.ChatCompletion(context.Background(), simpleRequest())
		if err != nil {
			t.Fatalf("ChatCompletion refusal kosong: %v — mau sukses content_filter", err)
		}
		if len(resp.Choices) != 1 || resp.Choices[0].FinishReason != providers.FinishContentFilter {
			t.Fatalf("choices = %+v, mau satu choice finish content_filter", resp.Choices)
		}
	})

	t.Run("refusal berteks tetap content_filter", func(t *testing.T) {
		p, _ := newServer(t, jsonHandler(200, `{
			"id": "msg_b426b", "type": "message", "role": "assistant",
			"model": "claude-sonnet-4-5",
			"content": [{"type": "text", "text": "tidak bisa membantu itu"}],
			"stop_reason": "refusal",
			"usage": {"input_tokens": 10, "output_tokens": 4}
		}`))
		resp, err := p.ChatCompletion(context.Background(), simpleRequest())
		if err != nil {
			t.Fatalf("ChatCompletion refusal berteks: %v", err)
		}
		if resp.Choices[0].FinishReason != providers.FinishContentFilter {
			t.Errorf("finish = %q, mau content_filter", resp.Choices[0].FinishReason)
		}
	})

	t.Run("kosong biasa tetap server", func(t *testing.T) {
		p, _ := newServer(t, jsonHandler(200, `{
			"id": "msg_b426c", "type": "message", "role": "assistant",
			"model": "claude-sonnet-4-5",
			"content": [],
			"stop_reason": "end_turn",
			"usage": {"input_tokens": 10, "output_tokens": 0}
		}`))
		_, err := p.ChatCompletion(context.Background(), simpleRequest())
		pe := providers.AsError(err)
		if pe == nil || pe.Kind != providers.ErrKindServer {
			t.Errorf("err = %v, mau *providers.Error berkind server untuk kekosongan biasa", err)
		}
	})
}

// TestB429GambarHasilToolAnthropic adalah pengunci B-429 (sisi
// Anthropic): gambar pada pesan hasil tool dulu hilang — cabang
// RoleTool hanya memanen Text(). Kini ia menjadi block image di
// dalam konten tool_result pada body upstream.
func TestB429GambarHasilToolAnthropic(t *testing.T) {
	p, log := newServer(t, jsonHandler(200, textResponse))
	req := &providers.ChatRequest{
		Model: "claude-sonnet-4-5",
		Messages: []providers.Message{
			{Role: providers.RoleUser, Content: "pakai tool"},
			{Role: providers.RoleAssistant, ToolCalls: []providers.ToolCall{{
				ID: "toolu_b429", Type: "function",
				Function: providers.FunctionCall{Name: "tangkap", Arguments: "{}"},
			}}},
			{Role: providers.RoleTool, ToolCallID: "toolu_b429", Parts: []providers.ContentPart{
				{Type: providers.PartTypeText, Text: "hasil tangkapan:"},
				{Type: providers.PartTypeImageURL, ImageURL: &providers.ImageURL{URL: "data:image/png;base64,aVBvcg=="}},
			}},
		},
	}
	if _, err := p.ChatCompletion(context.Background(), req); err != nil {
		t.Fatalf("ChatCompletion: %v", err)
	}
	body := string(log.Body())
	if !strings.Contains(body, "aVBvcg==") {
		t.Errorf("data gambar tidak muncul di body upstream: %s", body)
	}
	if !strings.Contains(body, `"image"`) {
		t.Errorf("block image tidak ada di body upstream: %s", body)
	}
}

// TestB431RetryAfterBody200Anthropic adalah pengunci B-431 (sisi
// Anthropic): body bertipe error pada status 200 dengan header
// Retry-After: 17 dulu menghasilkan RetryAfter 0.
func TestB431RetryAfterBody200Anthropic(t *testing.T) {
	p, _ := newServer(t, func(w http.ResponseWriter, _ *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		w.Header().Set("Retry-After", "17")
		w.WriteHeader(200)
		_, _ = io.WriteString(w, `{"type":"error","error":{"type":"rate_limit_error","message":"pelan-pelan"}}`)
	})
	_, err := p.ChatCompletion(context.Background(), simpleRequest())
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
}
