package google

import (
	"context"
	"io"
	"net/http"
	"testing"
)

// TestB295SignatureThoughtStreamingSampaiPeristiwa adalah pengunci
// B-295 (B-262 sisi streaming): pada jalur streaming, signature part
// thought sebelumnya terbuang struktural — StreamEvent tidak punya
// field untuknya. Kini signature yang menempel pada part thought
// harus tiba di StreamEvent.ReasoningSignature, sejajar
// Message.ReasoningSignature pada jalur non-streaming.
func TestB295SignatureThoughtStreamingSampaiPeristiwa(t *testing.T) {
	p, _ := newServer(t, sseHandler(
		`data: {"candidates":[{"index":0,"content":{"role":"model","parts":[{"thought":true,"text":"mempertimbangkan","thoughtSignature":"sig-aliran-295"}]}}]}`+"\n\n"+
			`data: {"candidates":[{"index":0,"content":{"role":"model","parts":[{"text":"jawaban"}]},"finishReason":"STOP"}],"usageMetadata":{"promptTokenCount":10,"candidatesTokenCount":5,"totalTokenCount":15}}`+"\n\n",
	))
	st, err := p.ChatCompletionStream(context.Background(), simpleRequest())
	if err != nil {
		t.Fatalf("ChatCompletionStream: %v", err)
	}
	defer st.Close()
	var sig string
	var nalar string
	for {
		ev, err := st.Recv()
		if err == io.EOF {
			break
		}
		if err != nil {
			t.Fatalf("Recv: %v", err)
		}
		if ev.ReasoningSignature != "" {
			sig = ev.ReasoningSignature
		}
		nalar += ev.ReasoningDelta
	}
	if nalar != "mempertimbangkan" {
		t.Errorf("ReasoningDelta terkumpul = %q, mau %q", nalar, "mempertimbangkan")
	}
	if sig != "sig-aliran-295" {
		t.Errorf("ReasoningSignature di peristiwa aliran = %q, mau %q — signature part thought terbuang di jalur streaming", sig, "sig-aliran-295")
	}
	_ = http.StatusOK
}
