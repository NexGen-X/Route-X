package anthropic

import (
	"context"
	"net/http"
	"testing"
)

// B-263 (sisi Anthropic): signature NUMERIK pada blok thinking tidak
// lagi menggagalkan unmarshal seluruh respons — ia dinormalkan menjadi
// teks literalnya dan sampai ke ReasoningSignature kanonik.
func TestB263SignatureNumerikTidakMenggagalkanRespons(t *testing.T) {
	p, _ := newServer(t, jsonHandler(http.StatusOK,
		`{"id":"msg_1","type":"message","role":"assistant","model":"claude-sonnet-4-5","content":[{"type":"thinking","thinking":"sedang berpikir","signature":987},{"type":"text","text":"halo"}],"stop_reason":"end_turn","usage":{"input_tokens":3,"output_tokens":2}}`))

	resp, err := p.ChatCompletion(context.Background(), simpleRequest())
	if err != nil {
		t.Fatalf("ChatCompletion gagal oleh signature numerik: %v", err)
	}
	if len(resp.Choices) != 1 {
		t.Fatalf("choices = %d, mau 1", len(resp.Choices))
	}
	msg := resp.Choices[0].Message
	if msg.ReasoningSignature != "987" {
		t.Errorf("ReasoningSignature = %q, mau %q (literal angka)", msg.ReasoningSignature, "987")
	}
	if msg.Content != "halo" {
		t.Errorf("Content = %q, mau %q", msg.Content, "halo")
	}
}
