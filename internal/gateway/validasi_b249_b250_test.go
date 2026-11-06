package gateway

import (
	"net/http"
	"strings"
	"testing"
)

// TestB249AnthropicMenolakSamplingDiLuarRentang adalah pengunci B-249:
// decoder permukaan Anthropic sebelumnya hanya memvalidasi max_tokens,
// sehingga temperature/top_p/top_k mustahil lolos ke adapter persis
// (terbukti: temperature 99, top_p -1, top_k -3 → tanpa error). Kini
// rentang kontrak dialek Anthropic ditegakkan: temperature 0..1,
// top_p 0..1, top_k > 0 — pasangan B-171 di permukaan lain.
func TestB249AnthropicMenolakSamplingDiLuarRentang(t *testing.T) {
	pesan := `"messages":[{"role":"user","content":"halo"}]`
	kasus := []struct {
		nama  string
		body  string
		field string
	}{
		{"temperature di atas 1", `{"model":"m","max_tokens":10,"temperature":99,` + pesan + `}`, "temperature"},
		{"temperature 1.5 (sah di OpenAI, tidak di Anthropic)", `{"model":"m","max_tokens":10,"temperature":1.5,` + pesan + `}`, "temperature"},
		{"temperature negatif", `{"model":"m","max_tokens":10,"temperature":-0.1,` + pesan + `}`, "temperature"},
		{"top_p negatif", `{"model":"m","max_tokens":10,"top_p":-1,` + pesan + `}`, "top_p"},
		{"top_p di atas 1", `{"model":"m","max_tokens":10,"top_p":1.5,` + pesan + `}`, "top_p"},
		{"top_k nol", `{"model":"m","max_tokens":10,"top_k":0,` + pesan + `}`, "top_k"},
		{"top_k negatif", `{"model":"m","max_tokens":10,"top_k":-3,` + pesan + `}`, "top_k"},
	}
	for _, k := range kasus {
		t.Run(k.nama, func(t *testing.T) {
			_, err := decodeAnthropicMessagesRequest([]byte(k.body), true)
			if err == nil {
				t.Fatalf("body diterima, mau ditolak: %s", k.body)
			}
			if !strings.Contains(err.Error(), k.field) {
				t.Errorf("pesan error = %q, mau menyebut field %s", err.Error(), k.field)
			}
		})
	}

	// Batas yang sah tetap diterima.
	sah := `{"model":"m","max_tokens":10,"temperature":1,"top_p":1,"top_k":1,` + pesan + `}`
	if _, err := decodeAnthropicMessagesRequest([]byte(sah), true); err != nil {
		t.Errorf("nilai batas sah ditolak: %v", err)
	}

	// Level handler: penolakan tampil sebagai 400 ber-envelope Anthropic.
	s := susun(t, providerSukses(), nil)
	w := s.panggil(t, http.MethodPost, "/messages",
		`{"model":"gpt-4o-mini","max_tokens":10,"temperature":99,"messages":[{"role":"user","content":"halo"}]}`, nil)
	if w.Code != http.StatusBadRequest {
		t.Errorf("handler temperature 99: status = %d, mau 400: %s", w.Code, w.Body.String())
	}
}

// TestB250ValidasiRentangFieldExtraDanDimensions adalah pengunci B-250:
// max_completion_tokens (yang hidup di Extra chat) dan dimensions
// embeddings sebelumnya lolos tanpa pemeriksaan nilai — -5 untuk
// keduanya diterima decoder. Kini keduanya divalidasi > 0 di decoder,
// sekelas B-171/B-173.
func TestB250ValidasiRentangFieldExtraDanDimensions(t *testing.T) {
	pesanChat := `"messages":[{"role":"user","content":"halo"}]`

	for _, nilai := range []string{"-5", "0", `"banyak"`, "1.5"} {
		body := `{"model":"m","max_completion_tokens":` + nilai + `,` + pesanChat + `}`
		if _, err := DecodeChatRequest([]byte(body)); err == nil {
			t.Errorf("max_completion_tokens=%s diterima, mau ditolak", nilai)
		} else if !strings.Contains(err.Error(), "max_completion_tokens") {
			t.Errorf("max_completion_tokens=%s: pesan = %q, mau menyebut field", nilai, err.Error())
		}
	}
	// Nilai sah tetap diteruskan lewat Extra apa adanya.
	out, err := DecodeChatRequest([]byte(`{"model":"m","max_completion_tokens":100,` + pesanChat + `}`))
	if err != nil {
		t.Fatalf("max_completion_tokens sah ditolak: %v", err)
	}
	if n, ok := angkaExtra(out.Extra, "max_completion_tokens"); !ok || n != 100 {
		t.Errorf("Extra max_completion_tokens = %v/%v, mau 100 tetap terbawa", n, ok)
	}

	for _, nilai := range []string{"-5", "0"} {
		body := `{"model":"m","input":"halo","dimensions":` + nilai + `}`
		if _, err := DecodeEmbeddingsRequest([]byte(body)); err == nil {
			t.Errorf("dimensions=%s diterima, mau ditolak", nilai)
		} else if !strings.Contains(err.Error(), "dimensions") {
			t.Errorf("dimensions=%s: pesan = %q, mau menyebut field", nilai, err.Error())
		}
	}
	if _, err := DecodeEmbeddingsRequest([]byte(`{"model":"m","input":"halo","dimensions":128}`)); err != nil {
		t.Errorf("dimensions sah ditolak: %v", err)
	}
}

// TestTurunanB253PesanDecodeAnthropicTidakBocor adalah pengunci turunan
// B-253 di decoder Anthropic: kegagalan unmarshal sebelumnya dibungkus
// apa adanya ("body JSON Anthropic tidak sah: json: cannot unmarshal
// string into Go struct field anthropicRequest.max_tokens of type
// int"), membocorkan nama struct Go internal ke klien. Kini kesalahan
// tipe menyebut field JSON-nya dan kesalahan sintaks generik.
func TestTurunanB253PesanDecodeAnthropicTidakBocor(t *testing.T) {
	_, err := decodeAnthropicMessagesRequest([]byte(
		`{"model":"m","max_tokens":"banyak","messages":[{"role":"user","content":"halo"}]}`), true)
	if err == nil {
		t.Fatal("max_tokens string diterima, mau ditolak")
	}
	if strings.Contains(err.Error(), "anthropicRequest") || strings.Contains(err.Error(), "cannot unmarshal") {
		t.Errorf("pesan masih membocorkan detail internal: %q", err.Error())
	}
	if !strings.Contains(err.Error(), "max_tokens") {
		t.Errorf("pesan = %q, mau menyebut field max_tokens", err.Error())
	}

	_, err = decodeAnthropicMessagesRequest([]byte(`{rusak`), true)
	if err == nil {
		t.Fatal("JSON rusak diterima, mau ditolak")
	}
	if err.Error() != "body JSON Anthropic tidak sah" {
		t.Errorf("pesan sintaks = %q, mau generik tanpa detail stdlib", err.Error())
	}
}
