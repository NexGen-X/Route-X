package anthropic

import (
	"context"
	"io"
	"strings"
	"testing"
)

// TestB295SignatureDeltaSampaiPeristiwa adalah pengunci B-295 sisi
// Anthropic: signature_delta pada aliran sebelumnya dibuang ("tidak
// punya wujud di sisi klien") karena StreamEvent tidak punya field
// signature. Kini ia tiba sebagai StreamEvent.ReasoningSignature.
//
// Catatan B-324 (Fase 16): asersi Raw semula menuntut Raw KOSONG
// ("dialek OpenAI tak punya padanan chunk untuknya"). Keputusan itu
// digantikan B-324 — dialek kini punya padanannya (delta
// thought_signature), karena Raw kosong berarti gateway melewati
// peristiwa itu dan signature tak pernah sampai ke byte klien.
// Asersi di bawah kini menuntut Raw MEMUAT signature-nya; cakupan
// kanonik B-295 (ReasoningSignature tiba) tidak berubah.
func TestB295SignatureDeltaSampaiPeristiwa(t *testing.T) {
	p, _ := newServer(t, sseHandler(
		"event: message_start\ndata: {\"type\":\"message_start\",\"message\":{\"id\":\"msg_295\",\"model\":\"claude-sonnet-4-5-20250929\",\"usage\":{\"input_tokens\":10,\"output_tokens\":1}}}\n\n",
		"event: content_block_start\ndata: {\"type\":\"content_block_start\",\"index\":0,\"content_block\":{\"type\":\"thinking\",\"thinking\":\"\"}}\n\n",
		"event: content_block_delta\ndata: {\"type\":\"content_block_delta\",\"index\":0,\"delta\":{\"type\":\"thinking_delta\",\"thinking\":\"pikir\"}}\n\n",
		"event: content_block_delta\ndata: {\"type\":\"content_block_delta\",\"index\":0,\"delta\":{\"type\":\"signature_delta\",\"signature\":\"sig-ant-295\"}}\n\n",
		"event: content_block_stop\ndata: {\"type\":\"content_block_stop\",\"index\":0}\n\n",
		"event: message_delta\ndata: {\"type\":\"message_delta\",\"delta\":{\"stop_reason\":\"end_turn\"},\"usage\":{\"output_tokens\":5}}\n\n",
		"event: message_stop\ndata: {\"type\":\"message_stop\"}\n\n",
	))
	st, err := p.ChatCompletionStream(context.Background(), simpleRequest())
	if err != nil {
		t.Fatalf("ChatCompletionStream: %v", err)
	}
	defer st.Close()
	var sig string
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
			// B-324: Raw peristiwa ini wajib memuat signature-nya
			// sebagai delta thought_signature; Raw kosong berarti
			// signature berhenti di kontrak kanonik.
			if !strings.Contains(string(ev.Raw), `"thought_signature":"sig-ant-295"`) {
				t.Errorf("Raw peristiwa signature = %q, mau memuat thought_signature sig-ant-295", ev.Raw)
			}
		}
	}
	if sig != "sig-ant-295" {
		t.Errorf("ReasoningSignature = %q, mau %q — signature_delta terbuang di jalur streaming", sig, "sig-ant-295")
	}
}
