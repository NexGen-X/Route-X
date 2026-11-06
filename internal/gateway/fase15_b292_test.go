package gateway

import (
	"strings"
	"testing"
)

// TestB292DecodeAnthropicBersarangTidakBocor adalah pengunci B-292:
// kegagalan decode pada bentuk BERSARANG permukaan Anthropic (isi pesan
// dan tool_choice) sebelumnya diteruskan apa adanya lewat %w sehingga
// nama struct Go internal dan frasa stdlib "cannot unmarshal" bocor ke
// klien. Pesan kini harus menyebut field JSON-nya (atau generik per
// kasus) tanpa jeroan Go — sejajar sanitasi tingkat-atas B-253.
func TestB292DecodeAnthropicBersarangTidakBocor(t *testing.T) {
	kasus := []struct {
		nama string
		body string
	}{
		{"konten angka", `{"model":"m","max_tokens":10,"messages":[{"role":"user","content":123}]}`},
		{"konten objek", `{"model":"m","max_tokens":10,"messages":[{"role":"user","content":{"x":1}}]}`},
		{"teks blok angka", `{"model":"m","max_tokens":10,"messages":[{"role":"user","content":[{"type":"text","text":123}]}]}`},
		{"tool_choice tipe angka", `{"model":"m","max_tokens":10,"messages":[{"role":"user","content":"halo"}],"tool_choice":{"type":123}}`},
		{"tool_choice bool teks", `{"model":"m","max_tokens":10,"messages":[{"role":"user","content":"halo"}],"tool_choice":{"type":"auto","disable_parallel_tool_use":"ya"}}`},
	}
	terlarang := []string{"cannot unmarshal", "Go struct", "Go value", "anthropicBlock", "gateway."}
	for _, k := range kasus {
		t.Run(k.nama, func(t *testing.T) {
			_, err := DecodeAnthropicMessagesRequest([]byte(k.body))
			if err == nil {
				t.Fatalf("body %s: mau error decode, dapat nil", k.nama)
			}
			for _, s := range terlarang {
				if strings.Contains(err.Error(), s) {
					t.Errorf("pesan error memuat jeroan %q: %v", s, err)
				}
			}
		})
	}
}
