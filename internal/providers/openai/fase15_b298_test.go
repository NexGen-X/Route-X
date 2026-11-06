package openai

import (
	"context"
	"testing"

	"github.com/NexGen-X/Route-X/internal/providers"
)

// TestB298ChoiceKosongDitolak adalah pengunci B-298 sisi OpenAI:
// choice yang HADIR tetapi pesannya kosong total sebelumnya lolos
// sebagai sukses kosong karena satu-satunya gerbang memeriksa jumlah
// choice. Finish content_filter dikecualikan — kekosongannya sah.
func TestB298ChoiceKosongDitolak(t *testing.T) {
	req := &providers.ChatRequest{
		Model:    "model-uji",
		Messages: []providers.Message{{Role: "user", Content: "halo"}},
	}
	kasus := []struct {
		nama     string
		body     string
		mauGagal bool
	}{
		{"content kosong finish stop",
			`{"id":"c1","model":"m","choices":[{"index":0,"message":{"role":"assistant","content":""},"finish_reason":"stop"}],"usage":{"prompt_tokens":3,"completion_tokens":0,"total_tokens":3}}`,
			true},
		{"content kosong finish content_filter sah",
			`{"id":"c1","model":"m","choices":[{"index":0,"message":{"role":"assistant","content":""},"finish_reason":"content_filter"}]}`,
			false},
		{"content normal",
			`{"id":"c1","model":"m","choices":[{"index":0,"message":{"role":"assistant","content":"halo"},"finish_reason":"stop"}]}`,
			false},
		{"refusal adalah konten",
			`{"id":"c1","model":"m","choices":[{"index":0,"message":{"role":"assistant","content":"","refusal":"tidak bisa"},"finish_reason":"stop"}]}`,
			false},
	}
	for _, k := range kasus {
		t.Run(k.nama, func(t *testing.T) {
			u := newUpstream(t, jsonHandler(200, k.body))
			p := testProvider(t, u.url(), nil)
			_, err := p.ChatCompletion(context.Background(), req)
			if k.mauGagal && err == nil {
				t.Fatal("ChatCompletion berhasil untuk choice kosong — sukses kosong lolos")
			}
			if !k.mauGagal && err != nil {
				t.Fatalf("ChatCompletion gagal untuk kasus kontrol: %v", err)
			}
		})
	}
}
