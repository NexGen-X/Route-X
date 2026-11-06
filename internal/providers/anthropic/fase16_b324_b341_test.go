package anthropic

import (
	"context"
	"encoding/json"
	"io"
	"testing"

	"github.com/NexGen-X/Route-X/internal/providers"
)

// TestB341BlockImageStreamingSampaiKanonik adalah pengunci B-341:
// blockStart aliran Anthropic dulu hanya menangani tool_use/text —
// block image pada content_block_start hilang senyap, asimetri dari
// B-299 yang menutup jalur non-streaming. Kini block image menjadi
// part gambar kanonik (base64 → data URI, url → apa adanya), persis
// aturan jalur non-streaming.
func TestB341BlockImageStreamingSampaiKanonik(t *testing.T) {
	p, _ := newServer(t, sseHandler(
		"event: message_start\ndata: {\"type\":\"message_start\",\"message\":{\"id\":\"msg_341\",\"model\":\"claude-sonnet-4-5-20250929\",\"usage\":{\"input_tokens\":10,\"output_tokens\":1}}}\n\n",
		"event: content_block_start\ndata: {\"type\":\"content_block_start\",\"index\":0,\"content_block\":{\"type\":\"image\",\"source\":{\"type\":\"base64\",\"media_type\":\"image/png\",\"data\":\"aGVsbG8=\"}}}\n\n",
		"event: content_block_stop\ndata: {\"type\":\"content_block_stop\",\"index\":0}\n\n",
		"event: content_block_start\ndata: {\"type\":\"content_block_start\",\"index\":1,\"content_block\":{\"type\":\"image\",\"source\":{\"type\":\"url\",\"url\":\"https://contoh.invalid/gambar.jpg\"}}}\n\n",
		"event: content_block_stop\ndata: {\"type\":\"content_block_stop\",\"index\":1}\n\n",
		"event: message_delta\ndata: {\"type\":\"message_delta\",\"delta\":{\"stop_reason\":\"end_turn\"},\"usage\":{\"output_tokens\":5}}\n\n",
		"event: message_stop\ndata: {\"type\":\"message_stop\"}\n\n",
	))
	st, err := p.ChatCompletionStream(context.Background(), simpleRequest())
	if err != nil {
		t.Fatalf("ChatCompletionStream: %v", err)
	}
	defer st.Close()
	var urls []string
	for {
		ev, err := st.Recv()
		if err == io.EOF {
			break
		}
		if err != nil {
			t.Fatalf("Recv: %v", err)
		}
		for _, part := range ev.Parts {
			if part.Type == providers.PartTypeImageURL && part.ImageURL != nil {
				urls = append(urls, part.ImageURL.URL)
			}
		}
	}
	mau := []string{"data:image/png;base64,aGVsbG8=", "https://contoh.invalid/gambar.jpg"}
	if len(urls) != len(mau) {
		t.Fatalf("part gambar kanonik = %v, mau %v — block image streaming terbuang", urls, mau)
	}
	for i := range mau {
		if urls[i] != mau[i] {
			t.Errorf("part gambar[%d] = %q, mau %q", i, urls[i], mau[i])
		}
	}
}

// TestB324SignatureDeltaMembawaChunk adalah pengunci sisi Anthropic
// dari B-324: signature_delta sebelumnya tiba sebagai peristiwa
// kanonik ber-Raw KOSONG, dan gateway melewati peristiwa ber-Raw
// kosong — signature berhenti di kontrak internal dan tak pernah
// menjadi byte SSE ke klien. Kini peristiwa itu membawa chunk
// ber-field thought_signature pada delta-nya.
func TestB324SignatureDeltaMembawaChunk(t *testing.T) {
	p, _ := newServer(t, sseHandler(
		"event: message_start\ndata: {\"type\":\"message_start\",\"message\":{\"id\":\"msg_324\",\"model\":\"claude-sonnet-4-5-20250929\",\"usage\":{\"input_tokens\":10,\"output_tokens\":1}}}\n\n",
		"event: content_block_start\ndata: {\"type\":\"content_block_start\",\"index\":0,\"content_block\":{\"type\":\"thinking\",\"thinking\":\"\"}}\n\n",
		"event: content_block_delta\ndata: {\"type\":\"content_block_delta\",\"index\":0,\"delta\":{\"type\":\"signature_delta\",\"signature\":\"sig-ant-324\"}}\n\n",
		"event: content_block_stop\ndata: {\"type\":\"content_block_stop\",\"index\":0}\n\n",
		"event: message_delta\ndata: {\"type\":\"message_delta\",\"delta\":{\"stop_reason\":\"end_turn\"},\"usage\":{\"output_tokens\":5}}\n\n",
		"event: message_stop\ndata: {\"type\":\"message_stop\"}\n\n",
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
		if ev.ReasoningSignature != "sig-ant-324" {
			continue
		}
		ketemu = true
		if len(ev.Raw) == 0 {
			t.Fatalf("peristiwa signature ber-Raw kosong — signature berhenti di kontrak kanonik (B-324)")
		}
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
		if len(chunk.Choices) != 1 || chunk.Choices[0].Delta.ThoughtSignature != "sig-ant-324" {
			t.Errorf("delta.thought_signature pada Raw = %+v, mau %q", chunk.Choices, "sig-ant-324")
		}
	}
	if !ketemu {
		t.Fatal("peristiwa ReasoningSignature tidak ditemukan sama sekali")
	}
}
