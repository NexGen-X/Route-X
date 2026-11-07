package anthropic

import (
	"context"
	"errors"
	"io"
	"net/http"
	"strings"
	"testing"

	"github.com/NexGen-X/Route-X/internal/providers"
)

// TestB388SystemMultimodalDitolak adalah pengunci B-388 sisi
// anthropic: pesan system yang membawa bagian gambar sebelumnya
// lolos ke field system sebagai teks saja — bagian gambarnya
// dibuang senyap oleh Text(). Kini adapter menolaknya dengan
// error permintaan-tidak-sah yang menyebut jenis bagiannya,
// sejajar penolakan permukaan Messages (B-359).
func TestB388SystemMultimodalDitolak(t *testing.T) {
	p, _ := newServer(t, jsonHandler(http.StatusOK, textResponse))
	req := &providers.ChatRequest{
		Model: "claude-sonnet-4-5",
		Messages: []providers.Message{
			{Role: providers.RoleSystem, Parts: []providers.ContentPart{
				{Type: providers.PartTypeText, Text: "Kamu asisten."},
				{Type: providers.PartTypeImageURL, ImageURL: &providers.ImageURL{URL: "data:image/png;base64,aVBvcg=="}},
			}},
			{Role: providers.RoleUser, Content: "halo"},
		},
	}
	_, err := p.ChatCompletion(context.Background(), req)
	if err == nil {
		t.Fatal("ChatCompletion menerima pesan system bergambar — bagian gambar akan hilang senyap")
	}
	var pe *providers.Error
	if !errors.As(err, &pe) || pe.Kind != providers.ErrKindInvalidRequest {
		t.Fatalf("error = %v, mau *providers.Error berkind invalid request", err)
	}
	if !strings.Contains(pe.Message, "image_url") {
		t.Errorf("pesan error = %q, mau menyebut jenis bagian image_url", pe.Message)
	}

	// Kontrol: system teks murni tetap diterima dan menjadi field system.
	req.Messages[0].Parts = []providers.ContentPart{{Type: providers.PartTypeText, Text: "Kamu asisten."}}
	if _, err := p.ChatCompletion(context.Background(), req); err != nil {
		t.Errorf("system teks murni ditolak: %v", err)
	}
}

// TestB389CompletionMembawaParts adalah pengunci B-389:
// encodeCompletion sebelumnya hanya menyalin Message.Content,
// jadi gambar keluaran yang sudah diterjemahkan toCanonicalResponse
// ke Parts (B-299) hilang dari Raw non-streaming (content:"").
// Kini Parts dirender sebagai larik content dialek OpenAI, sama
// seperti adapter google.
func TestB389CompletionMembawaParts(t *testing.T) {
	t.Run("gambar plus teks menjadi larik content", func(t *testing.T) {
		body := `{"id":"msg_389","type":"message","role":"assistant","model":"claude-sonnet-4-5","content":[{"type":"image","source":{"type":"base64","media_type":"image/png","data":"aVBvcg=="}},{"type":"text","text":"ini gambarnya"}],"stop_reason":"end_turn","usage":{"input_tokens":3,"output_tokens":4}}`
		p, _ := newServer(t, jsonHandler(http.StatusOK, body))
		resp, err := p.ChatCompletion(context.Background(), simpleRequest())
		if err != nil {
			t.Fatalf("ChatCompletion: %v", err)
		}
		raw := string(resp.Raw)
		if !strings.Contains(raw, `"content":[{"type":"image_url","image_url":{"url":"data:image/png;base64,aVBvcg=="}},{"type":"text","text":"ini gambarnya"}]`) {
			t.Errorf("Raw = %s, mau content larik image_url + text dari Parts kanonik", raw)
		}
	})

	t.Run("teks murni tetap string", func(t *testing.T) {
		body := `{"id":"msg_389","type":"message","role":"assistant","model":"claude-sonnet-4-5","content":[{"type":"text","text":"halo"}],"stop_reason":"end_turn","usage":{"input_tokens":3,"output_tokens":1}}`
		p, _ := newServer(t, jsonHandler(http.StatusOK, body))
		resp, err := p.ChatCompletion(context.Background(), simpleRequest())
		if err != nil {
			t.Fatalf("ChatCompletion: %v", err)
		}
		if !strings.Contains(string(resp.Raw), `"content":"halo"`) {
			t.Errorf("Raw = %s, mau content string untuk teks murni", resp.Raw)
		}
	})

	t.Run("tool saja tetap null", func(t *testing.T) {
		body := `{"id":"msg_389","type":"message","role":"assistant","model":"claude-sonnet-4-5","content":[{"type":"tool_use","id":"toolu_1","name":"cuaca","input":{"kota":"Makassar"}}],"stop_reason":"tool_use","usage":{"input_tokens":3,"output_tokens":1}}`
		p, _ := newServer(t, jsonHandler(http.StatusOK, body))
		resp, err := p.ChatCompletion(context.Background(), simpleRequest())
		if err != nil {
			t.Fatalf("ChatCompletion: %v", err)
		}
		if !strings.Contains(string(resp.Raw), `"content":null`) {
			t.Errorf("Raw = %s, mau content null untuk jawaban tool-saja", resp.Raw)
		}
	})
}

// TestB416RedactedThinkingDibuangTercatat adalah pengunci
// keputusan B-416: block redacted_thinking dibuang DENGAN SADAR
// (muatan terenkripsi tanpa slot kanonik; lihat komentar keputusan
// di translate.go) — yang dikunci di sini adalah dua sisinya:
// data terenkripsi tidak muncul di bentuk kanonik maupun Raw,
// DAN teks pendampingnya tidak ikut hilang.
func TestB416RedactedThinkingDibuangTercatat(t *testing.T) {
	const muatanRahasia = "TUNGGUL-ENKRIPSI-B416"

	t.Run("non-streaming", func(t *testing.T) {
		body := `{"id":"msg_416","type":"message","role":"assistant","model":"claude-sonnet-4-5","content":[{"type":"redacted_thinking","data":"` + muatanRahasia + `"},{"type":"text","text":"jawaban terlihat"}],"stop_reason":"end_turn","usage":{"input_tokens":3,"output_tokens":2}}`
		p, _ := newServer(t, jsonHandler(http.StatusOK, body))
		resp, err := p.ChatCompletion(context.Background(), simpleRequest())
		if err != nil {
			t.Fatalf("ChatCompletion: %v", err)
		}
		msg := resp.Choices[0].Message
		if strings.Contains(string(resp.Raw), muatanRahasia) || strings.Contains(msg.Content, muatanRahasia) {
			t.Errorf("muatan terenkripsi bocor ke keluaran (Raw: %s)", resp.Raw)
		}
		for _, part := range msg.Parts {
			if strings.Contains(part.Text, muatanRahasia) {
				t.Errorf("muatan terenkripsi bocor ke Parts: %+v", part)
			}
		}
		if msg.Content != "jawaban terlihat" {
			t.Errorf("Content = %q, mau teks pendamping tetap utuh", msg.Content)
		}
	})

	t.Run("streaming", func(t *testing.T) {
		p, _ := newServer(t, sseHandler(
			"event: message_start\ndata: {\"type\":\"message_start\",\"message\":{\"id\":\"msg_416\",\"model\":\"claude-sonnet-4-5-20250929\",\"usage\":{\"input_tokens\":10,\"output_tokens\":1}}}\n\n",
			"event: content_block_start\ndata: {\"type\":\"content_block_start\",\"index\":0,\"content_block\":{\"type\":\"redacted_thinking\",\"data\":\""+muatanRahasia+"\"}}\n\n",
			"event: content_block_stop\ndata: {\"type\":\"content_block_stop\",\"index\":0}\n\n",
			"event: content_block_start\ndata: {\"type\":\"content_block_start\",\"index\":1,\"content_block\":{\"type\":\"text\",\"text\":\"\"}}\n\n",
			"event: content_block_delta\ndata: {\"type\":\"content_block_delta\",\"index\":1,\"delta\":{\"type\":\"text_delta\",\"text\":\"jawaban terlihat\"}}\n\n",
			"event: content_block_stop\ndata: {\"type\":\"content_block_stop\",\"index\":1}\n\n",
			"event: message_delta\ndata: {\"type\":\"message_delta\",\"delta\":{\"stop_reason\":\"end_turn\"},\"usage\":{\"output_tokens\":5}}\n\n",
			"event: message_stop\ndata: {\"type\":\"message_stop\"}\n\n",
		))
		st, err := p.ChatCompletionStream(context.Background(), simpleRequest())
		if err != nil {
			t.Fatalf("ChatCompletionStream: %v", err)
		}
		defer st.Close()
		var semuaRaw, semuaDelta strings.Builder
		for {
			ev, err := st.Recv()
			if err == io.EOF {
				break
			}
			if err != nil {
				t.Fatalf("Recv: %v", err)
			}
			semuaRaw.Write(ev.Raw)
			semuaDelta.WriteString(ev.Delta)
		}
		if strings.Contains(semuaRaw.String(), muatanRahasia) {
			t.Errorf("muatan terenkripsi bocor ke Raw streaming: %s", semuaRaw.String())
		}
		if !strings.Contains(semuaDelta.String(), "jawaban terlihat") {
			t.Errorf("teks pendamping hilang di streaming, delta terkumpul = %q", semuaDelta.String())
		}
	})
}
