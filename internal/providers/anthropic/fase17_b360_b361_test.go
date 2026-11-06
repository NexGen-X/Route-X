package anthropic

import (
	"context"
	"encoding/json"
	"io"
	"net/http"
	"strings"
	"testing"

	"github.com/NexGen-X/Route-X/internal/providers"
)

// TestB360NonStreamingMembawaThoughtSignature adalah pengunci
// B-360 sisi Anthropic: kanonis mengisi ReasoningSignature dari
// block thinking bertanda tangan dan dialek streaming sudah
// membawanya sejak B-324, tetapi oaMessage non-streaming tidak
// punya field-nya — Raw yang diteruskan permukaan Chat (kontrak
// B-013) membuang signature, jadi giliran lanjutan multi-turn
// lewat jalur non-streaming kehilangannya.
func TestB360NonStreamingMembawaThoughtSignature(t *testing.T) {
	p, _ := newServer(t, jsonHandler(http.StatusOK,
		`{"id":"msg_360","type":"message","role":"assistant","model":"claude-sonnet-4-5","content":[{"type":"thinking","thinking":"sedang berpikir","signature":"sig-ant-360"},{"type":"text","text":"halo"}],"stop_reason":"end_turn","usage":{"input_tokens":3,"output_tokens":2}}`))

	resp, err := p.ChatCompletion(context.Background(), simpleRequest())
	if err != nil {
		t.Fatalf("ChatCompletion: %v", err)
	}
	if len(resp.Choices) != 1 {
		t.Fatalf("choices = %d, mau 1", len(resp.Choices))
	}
	if got := resp.Choices[0].Message.ReasoningSignature; got != "sig-ant-360" {
		t.Fatalf("ReasoningSignature kanonik = %q, mau sig-ant-360 (kontrol terjemahan)", got)
	}
	if !strings.Contains(string(resp.Raw), `"thought_signature":"sig-ant-360"`) {
		t.Errorf("Raw non-streaming = %s, mau memuat thought_signature sig-ant-360 — jalur non-streaming membuangnya", resp.Raw)
	}
}

// TestB361GambarStreamingMembawaRawContent adalah pengunci B-361:
// peristiwa gambar streaming B-341 mengisi Parts kanonik tetapi
// Raw chunk-nya "diam" (delta kosong dari &oaDelta{}), sedangkan
// permukaan Chat gateway meneruskan Raw apa adanya — gambar
// hilang khusus di Chat walau utuh di Messages. Kini delta
// membawa larik content image_url pada Raw-nya, meniru dialek
// Google (B-299/B-211).
func TestB361GambarStreamingMembawaRawContent(t *testing.T) {
	p, _ := newServer(t, sseHandler(
		"event: message_start\ndata: {\"type\":\"message_start\",\"message\":{\"id\":\"msg_361\",\"model\":\"claude-sonnet-4-5-20250929\",\"usage\":{\"input_tokens\":10,\"output_tokens\":1}}}\n\n",
		"event: content_block_start\ndata: {\"type\":\"content_block_start\",\"index\":0,\"content_block\":{\"type\":\"image\",\"source\":{\"type\":\"base64\",\"media_type\":\"image/png\",\"data\":\"aGVsbG8=\"}}}\n\n",
		"event: content_block_stop\ndata: {\"type\":\"content_block_stop\",\"index\":0}\n\n",
		"event: message_delta\ndata: {\"type\":\"message_delta\",\"delta\":{\"stop_reason\":\"end_turn\"},\"usage\":{\"output_tokens\":5}}\n\n",
		"event: message_stop\ndata: {\"type\":\"message_stop\"}\n\n",
	))
	st, err := p.ChatCompletionStream(context.Background(), simpleRequest())
	if err != nil {
		t.Fatalf("ChatCompletionStream: %v", err)
	}
	defer st.Close()

	var rawGambar string
	var partKanonik string
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
				partKanonik = part.ImageURL.URL
			}
		}
		if strings.Contains(string(ev.Raw), `"image_url"`) {
			rawGambar = string(ev.Raw)
		}
	}
	if partKanonik != "data:image/png;base64,aGVsbG8=" {
		t.Fatalf("part gambar kanonik = %q, mau data URI (kontrol B-341)", partKanonik)
	}
	if rawGambar == "" {
		t.Fatal("tidak ada peristiwa streaming yang Raw-nya memuat image_url — gambar hilang di permukaan Chat")
	}
	var chunk struct {
		Choices []struct {
			Delta struct {
				Content []struct {
					Type     string `json:"type"`
					ImageURL struct {
						URL string `json:"url"`
					} `json:"image_url"`
				} `json:"content"`
			} `json:"delta"`
		} `json:"choices"`
	}
	if err := json.Unmarshal([]byte(rawGambar), &chunk); err != nil {
		t.Fatalf("urai Raw gambar: %v (raw: %s)", err, rawGambar)
	}
	if len(chunk.Choices) != 1 || len(chunk.Choices[0].Delta.Content) != 1 {
		t.Fatalf("chunk gambar = %s, mau satu choice dengan satu part content", rawGambar)
	}
	part := chunk.Choices[0].Delta.Content[0]
	if part.Type != "image_url" || part.ImageURL.URL != "data:image/png;base64,aGVsbG8=" {
		t.Errorf("part Raw = %+v, mau image_url berisi data URI yang sama dengan kanonik", part)
	}
}
