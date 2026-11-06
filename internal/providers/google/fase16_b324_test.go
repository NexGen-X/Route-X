package google

import (
	"context"
	"encoding/json"
	"io"
	"testing"
)

// TestB324SignatureThoughtSampaiByteKlien adalah pengunci sisi Google
// dari B-324: B-295 membawa signature part thought sampai kontrak
// kanonik (StreamEvent.ReasoningSignature), tetapi encodeChunk
// menyusun Raw dari oaDelta yang tidak punya field signature — byte
// SSE ke klien tak pernah memuatnya, dan rantai multi-turn lewat
// klien eksternal tetap kehilangan signature di mil terakhir. Kini
// delta chunk membawa thought_signature pada peristiwa yang sama.
func TestB324SignatureThoughtSampaiByteKlien(t *testing.T) {
	p, _ := newServer(t, sseHandler(
		`data: {"candidates":[{"index":0,"content":{"role":"model","parts":[{"thought":true,"text":"mempertimbangkan","thoughtSignature":"sig-aliran-324"}]}}]}`+"\n\n"+
			`data: {"candidates":[{"index":0,"content":{"role":"model","parts":[{"text":"jawaban"}]},"finishReason":"STOP"}],"usageMetadata":{"promptTokenCount":10,"candidatesTokenCount":5,"totalTokenCount":15}}`+"\n\n",
	))
	st, err := p.ChatCompletionStream(context.Background(), simpleRequest())
	if err != nil {
		t.Fatalf("ChatCompletionStream: %v", err)
	}
	defer st.Close()
	var ketemu bool
	for {
		ev, err := st.Recv()
		if err == io.EOF {
			break
		}
		if err != nil {
			t.Fatalf("Recv: %v", err)
		}
		if ev.ReasoningSignature != "sig-aliran-324" {
			continue
		}
		ketemu = true
		var chunk struct {
			Choices []struct {
				Delta struct {
					ThoughtSignature string `json:"thought_signature"`
				} `json:"delta"`
			} `json:"choices"`
		}
		if err := json.Unmarshal(ev.Raw, &chunk); err != nil {
			t.Fatalf("Raw peristiwa signature bukan chunk sah: %v", err)
		}
		if len(chunk.Choices) != 1 || chunk.Choices[0].Delta.ThoughtSignature != "sig-aliran-324" {
			t.Errorf("delta.thought_signature pada Raw = %+v, mau %q — signature tak sampai byte klien", chunk.Choices, "sig-aliran-324")
		}
	}
	if !ketemu {
		t.Fatal("peristiwa ReasoningSignature tidak ditemukan sama sekali")
	}
}
