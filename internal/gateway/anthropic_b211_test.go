package gateway

import (
	"testing"

	"github.com/NexGen-X/Route-X/internal/providers"
)

// TestAnthropicStreamingBlokGambar membuktikan B-211 di permukaan
// /v1/messages streaming: StreamEvent ber-Parts gambar dari adapter
// (mis. generasi gambar Gemini) menjadi blok image Anthropic yang utuh
// (content_block_start + stop dengan source base64), TANPA menggandakan
// teks — teks yang sama di dalam Parts sudah mengalir lewat Delta.
func TestAnthropicStreamingBlokGambar(t *testing.T) {
	aliran := &aliranTiruan{ev: []*providers.StreamEvent{
		{Delta: "Ini gambarnya:"},
		{Parts: []providers.ContentPart{
			{Type: providers.PartTypeText, Text: "Ini gambarnya:"},
			{Type: providers.PartTypeImageURL, ImageURL: &providers.ImageURL{URL: "data:image/png;base64,aGVsbG8="}},
		}},
		{
			FinishReason: providers.FinishStop,
			Usage:        &providers.Usage{InputTokens: 5, OutputTokens: 7, TotalTokens: 12},
		},
	}}
	ps := panggilStreamMessages(t, aliran)

	mau := []string{
		"message_start",
		"content_block_start", "content_block_delta", "content_block_stop",
		"content_block_start", "content_block_stop",
		"message_delta", "message_stop",
	}
	if got := namaPeristiwa(ps); !samaIrisan(got, mau) {
		t.Fatalf("urutan peristiwa salah:\ndapat %v\nmau   %v", got, mau)
	}

	// Blok pertama text di index 0, blok kedua image di index 1.
	if i := indexBlok(t, ps, "content_block_start", 0); contentBlock(t, ps[i])["type"] != "text" {
		t.Errorf("blok pertama bukan text: %s", ps[i].muat)
	}
	i := indexBlok(t, ps, "content_block_start", 1)
	cb := contentBlock(t, ps[i])
	if cb["type"] != "image" {
		t.Fatalf("blok kedua bukan image: %s", ps[i].muat)
	}
	sumber, ok := cb["source"].(map[string]any)
	if !ok {
		t.Fatalf("blok image tanpa source: %s", ps[i].muat)
	}
	if sumber["type"] != "base64" || sumber["media_type"] != "image/png" || sumber["data"] != "aGVsbG8=" {
		t.Errorf("source blok image = %v, ingin base64 image/png aGVsbG8=", sumber)
	}

	// Teks tidak boleh ganda: tepat satu text_delta berisi teks peristiwa
	// pertama; salinan teks di dalam Parts tidak dirender ulang.
	jumlahDelta := 0
	for _, p := range ps {
		if p.nama == "content_block_delta" {
			jumlahDelta++
			if d := deltaBlok(t, p); d["text"] != "Ini gambarnya:" {
				t.Errorf("text_delta = %v, ingin teks peristiwa pertama", d)
			}
		}
	}
	if jumlahDelta != 1 {
		t.Errorf("jumlah content_block_delta = %d, ingin 1 (teks tidak digandakan)", jumlahDelta)
	}
}

// TestAnthropicStreamingPartAudioDilewati membuktikan part audio di
// permukaan streaming Anthropic dilewati secara eksplisit (protokolnya
// tidak punya blok keluaran audio) tanpa merusak aliran teks di
// sekelilingnya.
func TestAnthropicStreamingPartAudioDilewati(t *testing.T) {
	aliran := &aliranTiruan{ev: []*providers.StreamEvent{
		{Delta: "dengar: "},
		{Parts: []providers.ContentPart{
			{Type: providers.PartTypeAudio, Audio: &providers.Audio{Data: "QUJD", Format: "mp3"}},
		}},
		{Delta: "selesai"},
		{
			FinishReason: providers.FinishStop,
			Usage:        &providers.Usage{InputTokens: 5, OutputTokens: 7, TotalTokens: 12},
		},
	}}
	ps := panggilStreamMessages(t, aliran)

	for _, p := range ps {
		if p.nama == "content_block_start" && contentBlock(t, p)["type"] != "text" {
			t.Errorf("blok non-text terkirim untuk part audio: %s", p.muat)
		}
	}
	teks := ""
	for _, p := range ps {
		if p.nama == "content_block_delta" {
			teks += deltaBlok(t, p)["text"].(string)
		}
	}
	if teks != "dengar: selesai" {
		t.Errorf("teks tergabung = %q, ingin \"dengar: selesai\"", teks)
	}
}
