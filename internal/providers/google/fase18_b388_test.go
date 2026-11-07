package google

import (
	"context"
	"errors"
	"net/http"
	"strings"
	"testing"

	"github.com/NexGen-X/Route-X/internal/providers"
)

// TestB388SystemMultimodalDitolak adalah pengunci B-388 sisi
// google: pesan system yang membawa bagian gambar sebelumnya
// lolos ke systemInstruction sebagai teks saja — bagian gambarnya
// dibuang senyap oleh Text(). Kini adapter menolaknya dengan
// error permintaan-tidak-sah yang menyebut jenis bagiannya,
// sejajar adapter anthropic dan permukaan Messages (B-359).
func TestB388SystemMultimodalDitolak(t *testing.T) {
	p, _ := newServer(t, jsonHandler(http.StatusOK, textResponse))
	req := &providers.ChatRequest{
		Model: "gemini-2.5-flash",
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

	// Kontrol: system teks murni tetap diterima.
	req.Messages[0].Parts = []providers.ContentPart{{Type: providers.PartTypeText, Text: "Kamu asisten."}}
	if _, err := p.ChatCompletion(context.Background(), req); err != nil {
		t.Errorf("system teks murni ditolak: %v", err)
	}
}
