package google

import (
	"context"
	"net/http"
	"strings"
	"testing"
)

// TestB360NonStreamingMembawaThoughtSignature adalah pengunci
// B-360 sisi Google: kanonis mengisi ReasoningSignature dari part
// thought bertanda tangan dan dialek streaming sudah membawanya
// sejak B-324, tetapi oaMessage non-streaming tidak punya
// field-nya — Raw yang diteruskan permukaan Chat (kontrak B-013)
// membuang signature, jadi giliran lanjutan multi-turn lewat
// jalur non-streaming kehilangannya.
func TestB360NonStreamingMembawaThoughtSignature(t *testing.T) {
	p, _ := newServer(t, jsonHandler(http.StatusOK,
		`{"candidates":[{"index":0,"content":{"role":"model","parts":[{"thought":true,"text":"mempertimbangkan","thoughtSignature":"sig-gem-360"},{"text":"jawaban"}]},"finishReason":"STOP"}],"usageMetadata":{"promptTokenCount":10,"candidatesTokenCount":5,"totalTokenCount":15}}`))

	resp, err := p.ChatCompletion(context.Background(), simpleRequest())
	if err != nil {
		t.Fatalf("ChatCompletion: %v", err)
	}
	if len(resp.Choices) != 1 {
		t.Fatalf("choices = %d, mau 1", len(resp.Choices))
	}
	if got := resp.Choices[0].Message.ReasoningSignature; got != "sig-gem-360" {
		t.Fatalf("ReasoningSignature kanonik = %q, mau sig-gem-360 (kontrol terjemahan)", got)
	}
	if !strings.Contains(string(resp.Raw), `"thought_signature":"sig-gem-360"`) {
		t.Errorf("Raw non-streaming = %s, mau memuat thought_signature sig-gem-360 — jalur non-streaming membuangnya", resp.Raw)
	}
}
