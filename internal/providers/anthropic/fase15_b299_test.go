package anthropic

import (
	"context"
	"net/http"
	"testing"

	"github.com/NexGen-X/Route-X/internal/providers"
)

// TestB299BlockImageDiterjemahkan adalah pengunci B-299: block image
// pada respons Anthropic sebelumnya hilang senyap di toCanonicalResponse
// (tidak ada case untuknya) — respons berisi gambar saja bahkan
// tertangkap penjaga kekosongan B-261 dengan alasan yang salah. Kini
// block image menjadi bagian gambar kanonik, urutan dipertahankan.
func TestB299BlockImageDiterjemahkan(t *testing.T) {
	t.Run("gambar base64 plus teks", func(t *testing.T) {
		body := `{"id":"msg_1","type":"message","role":"assistant","model":"claude-sonnet-4-5","content":[{"type":"image","source":{"type":"base64","media_type":"image/png","data":"aVBvcg=="}},{"type":"text","text":"ini gambarnya"}],"stop_reason":"end_turn","usage":{"input_tokens":3,"output_tokens":4}}`
		p, _ := newServer(t, jsonHandler(http.StatusOK, body))
		resp, err := p.ChatCompletion(context.Background(), simpleRequest())
		if err != nil {
			t.Fatalf("ChatCompletion: %v", err)
		}
		msg := resp.Choices[0].Message
		if len(msg.Parts) != 2 {
			t.Fatalf("Parts = %d, mau 2 (gambar + teks, urutan dipertahankan)", len(msg.Parts))
		}
		if msg.Parts[0].Type != providers.PartTypeImageURL || msg.Parts[0].ImageURL == nil ||
			msg.Parts[0].ImageURL.URL != "data:image/png;base64,aVBvcg==" {
			t.Errorf("Parts[0] = %+v, mau bagian gambar data URI", msg.Parts[0])
		}
		if msg.Parts[1].Type != providers.PartTypeText || msg.Parts[1].Text != "ini gambarnya" {
			t.Errorf("Parts[1] = %+v, mau bagian teks", msg.Parts[1])
		}
		if msg.Content != "ini gambarnya" {
			t.Errorf("Content = %q, mau teks tetap terisi", msg.Content)
		}
	})

	t.Run("gambar url saja adalah konten nyata", func(t *testing.T) {
		body := `{"id":"msg_1","type":"message","role":"assistant","model":"claude-sonnet-4-5","content":[{"type":"image","source":{"type":"url","url":"https://contoh.invalid/g.png"}}],"stop_reason":"end_turn","usage":{"input_tokens":3,"output_tokens":4}}`
		p, _ := newServer(t, jsonHandler(http.StatusOK, body))
		resp, err := p.ChatCompletion(context.Background(), simpleRequest())
		if err != nil {
			t.Fatalf("ChatCompletion menolak respons gambar-saja sebagai kosong: %v", err)
		}
		msg := resp.Choices[0].Message
		if len(msg.Parts) != 1 || msg.Parts[0].ImageURL == nil || msg.Parts[0].ImageURL.URL != "https://contoh.invalid/g.png" {
			t.Errorf("Parts = %+v, mau satu bagian gambar url", msg.Parts)
		}
	})

	t.Run("teks murni tidak berubah bentuk", func(t *testing.T) {
		body := `{"id":"msg_1","type":"message","role":"assistant","model":"claude-sonnet-4-5","content":[{"type":"text","text":"halo"}],"stop_reason":"end_turn","usage":{"input_tokens":3,"output_tokens":1}}`
		p, _ := newServer(t, jsonHandler(http.StatusOK, body))
		resp, err := p.ChatCompletion(context.Background(), simpleRequest())
		if err != nil {
			t.Fatalf("ChatCompletion: %v", err)
		}
		if len(resp.Choices[0].Message.Parts) != 0 {
			t.Errorf("Parts terisi untuk teks murni — bentuk lama harus dipertahankan")
		}
	})
}
