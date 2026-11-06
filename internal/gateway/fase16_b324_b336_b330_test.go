package gateway

import (
	"testing"

	"github.com/NexGen-X/Route-X/internal/providers"
)

// TestB324BlokThinkingDirenderStreaming adalah pengunci sisi
// permukaan dari B-324: anthropicChatMengalir dulu SENGAJA tidak
// merender ReasoningDelta/ReasoningSignature sama sekali ("tidak
// dirender ... menghindari blok yang tidak dapat diverifikasi
// klien"), sehingga aliran model reasoning tiba di klien Anthropic
// tanpa satu blok thinking pun — dan tanda tangan yang dibutuhkan
// untuk meneruskan blok itu pada giliran berikut tak pernah keluar.
// Perender non-streaming pada handler yang sama sudah menerbitkan
// blok thinking + signature untuk SEMUA provider; jalur streaming
// kini setara: blok thinking dibuka, thinking_delta dan
// signature_delta diterbitkan di dalamnya, lalu blok teks menyusul
// pada indeks berikutnya.
func TestB324BlokThinkingDirenderStreaming(t *testing.T) {
	aliran := &aliranTiruan{ev: []*providers.StreamEvent{
		{ReasoningDelta: "mempertimbangkan"},
		{ReasoningSignature: "sig-324"},
		{Delta: "jawaban"},
		{FinishReason: providers.FinishStop, Usage: &providers.Usage{InputTokens: 5, OutputTokens: 7, TotalTokens: 12}},
	}}
	ps := panggilStreamMessages(t, aliran)

	var idxThinking, idxTeks = -1, -1
	var thinkingDelta, signatureDelta, teksDelta string
	var urutan []string
	for i, p := range ps {
		m := muatJSON(t, p)
		switch p.nama {
		case "content_block_start":
			cb := contentBlock(t, p)
			switch cb["type"] {
			case "thinking":
				idxThinking = i
			case "text":
				idxTeks = i
			}
		case "content_block_delta":
			d := deltaBlok(t, p)
			switch d["type"] {
			case "thinking_delta":
				thinkingDelta, _ = d["thinking"].(string)
			case "signature_delta":
				signatureDelta, _ = d["signature"].(string)
			case "text_delta":
				teksDelta, _ = d["text"].(string)
			}
		}
		_ = m
		urutan = append(urutan, p.nama)
	}
	if idxThinking < 0 {
		t.Fatalf("peristiwa = %v, mau memuat content_block_start thinking — blok thinking tak dirender", urutan)
	}
	if thinkingDelta != "mempertimbangkan" {
		t.Errorf("thinking_delta = %q, mau %q", thinkingDelta, "mempertimbangkan")
	}
	if signatureDelta != "sig-324" {
		t.Errorf("signature_delta = %q, mau %q — signature tak sampai klien Messages", signatureDelta, "sig-324")
	}
	if idxTeks < 0 || idxTeks < idxThinking {
		t.Errorf("blok teks (peristiwa ke-%d) harus menyusul blok thinking (ke-%d): %v", idxTeks, idxThinking, urutan)
	}
	if teksDelta != "jawaban" {
		t.Errorf("text_delta = %q, mau %q", teksDelta, "jawaban")
	}
}

// TestB336AliranHanyaPenalaranDitutupSukses adalah pengunci B-336:
// penalaran tak pernah dihitung sebagai konten oleh gerbang B-301
// (byteKonten hanya dari Delta teks), jadi aliran yang isinya murni
// penalaran selalu ditutup sebagai kegagalan "respons kosong" walau
// sehat. Begitu blok thinking dirender (B-324), blok yang terbit
// sendiri menandai aliran berkonten — subkasus ini kini sukses
// normal.
func TestB336AliranHanyaPenalaranDitutupSukses(t *testing.T) {
	aliran := &aliranTiruan{ev: []*providers.StreamEvent{
		{ReasoningDelta: "hanya nalar"},
		{ReasoningSignature: "sig-336"},
		{FinishReason: providers.FinishStop, Usage: &providers.Usage{InputTokens: 5, OutputTokens: 3, TotalTokens: 8}},
	}}
	ps := panggilStreamMessages(t, aliran)
	var adaError bool
	var delta *ssePeristiwa
	for i := range ps {
		if ps[i].nama == "error" {
			adaError = true
		}
		if ps[i].nama == "message_delta" {
			delta = &ps[i]
		}
	}
	if adaError {
		t.Errorf("peristiwa = %v, mau tanpa error — aliran reasoning-only sehat ditutup sebagai kegagalan kosong", namaPeristiwa(ps))
	}
	if delta == nil {
		t.Fatalf("peristiwa = %v, mau message_delta penutup", namaPeristiwa(ps))
	}
	if d := deltaBlok(t, *delta); d["stop_reason"] != "end_turn" {
		t.Errorf("stop_reason = %v, mau end_turn", d["stop_reason"])
	}
}

// TestB330MessageStartOutputTokensNol adalah pengunci B-330:
// message_start dulu menyatakan "output_tokens": 1 sebelum satu
// token pun diterbitkan — angka awal hasil karangan (komentar di
// titik itu bahkan menyebut nilai lain). Jalur non-streaming
// melaporkan usage sebenarnya; message_delta penutup tetap membawa
// angka sebenarnya. Kini message_start jujur nol.
func TestB330MessageStartOutputTokensNol(t *testing.T) {
	aliran := &aliranTiruan{ev: []*providers.StreamEvent{
		{Delta: "halo"},
		{FinishReason: providers.FinishStop, Usage: &providers.Usage{InputTokens: 5, OutputTokens: 2, TotalTokens: 7}},
	}}
	ps := panggilStreamMessages(t, aliran)
	for _, p := range ps {
		if p.nama != "message_start" {
			continue
		}
		msg, ok := muatJSON(t, p)["message"].(map[string]any)
		if !ok {
			t.Fatalf("message_start tanpa objek message: %s", p.muat)
		}
		usage, ok := msg["usage"].(map[string]any)
		if !ok {
			t.Fatalf("message_start tanpa usage: %s", p.muat)
		}
		if got := usage["output_tokens"]; got != float64(0) {
			t.Errorf("message_start usage.output_tokens = %v, mau 0 (belum ada token saat start)", got)
		}
		return
	}
	t.Fatal("peristiwa message_start tidak ditemukan")
}
