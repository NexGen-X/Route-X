package gateway

import (
	"strings"
	"testing"
)

// TestB359BlockSystemNonTeksDitolak adalah pengunci B-359: jalur
// Anthropic dulu membuang senyap block system yang well-typed
// tetapi non-teks — field system berbentuk array hanya
// mengumpulkan type=="text", dan pesan berperan system di
// messages hanya diambil teksnya lewat Text(). Klien menerima
// jawaban atas permintaan yang berbeda dari yang ia kirim. Kini
// keduanya error 400 yang menyebut tipe block-nya (pasangan arah
// well-typed dari B-337 yang menutup bentuk salah-tipe).
func TestB359BlockSystemNonTeksDitolak(t *testing.T) {
	gambar := `{"type":"image","source":{"type":"base64","media_type":"image/png","data":"aGVsbG8="}}`

	t.Run("field system berisi gambar", func(t *testing.T) {
		body := `{"model":"claude-sonnet-4-5","max_tokens":64,` +
			`"system":[{"type":"text","text":"Kamu asisten."},` + gambar + `],` +
			`"messages":[{"role":"user","content":"halo"}]}`
		_, err := DecodeAnthropicMessagesRequest([]byte(body))
		if err == nil {
			t.Fatal("decode system bergambar lolos, mau error")
		}
		if !strings.Contains(err.Error(), "image") {
			t.Errorf("error = %q, mau menyebut tipe block image", err)
		}
	})

	t.Run("pesan system berisi gambar", func(t *testing.T) {
		body := `{"model":"claude-sonnet-4-5","max_tokens":64,` +
			`"messages":[{"role":"system","content":[` + gambar + `]},` +
			`{"role":"user","content":"halo"}]}`
		_, err := DecodeAnthropicMessagesRequest([]byte(body))
		if err == nil {
			t.Fatal("decode pesan system bergambar lolos, mau error")
		}
		if !strings.Contains(err.Error(), "image_url") {
			t.Errorf("error = %q, mau menyebut tipe bagian image_url", err)
		}
	})

	t.Run("kontrol teks tetap lolos", func(t *testing.T) {
		body := `{"model":"claude-sonnet-4-5","max_tokens":64,` +
			`"system":[{"type":"text","text":"Kamu asisten."}],` +
			`"messages":[{"role":"system","content":"Selalu ringkas."},` +
			`{"role":"user","content":"halo"}]}`
		req, err := DecodeAnthropicMessagesRequest([]byte(body))
		if err != nil {
			t.Fatalf("decode kontrol teks: %v", err)
		}
		if len(req.Messages) == 0 || req.Messages[0].Role != "system" {
			t.Fatalf("pesan pertama = %+v, mau pesan system gabungan", req.Messages)
		}
		gabung := req.Messages[0].Text()
		if !strings.Contains(gabung, "Kamu asisten.") || !strings.Contains(gabung, "Selalu ringkas.") {
			t.Errorf("teks system gabungan = %q, mau memuat kedua sumber teks system", gabung)
		}
	})
}
