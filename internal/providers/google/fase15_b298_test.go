package google

import (
	"context"
	"net/http"
	"testing"

	"github.com/NexGen-X/Route-X/internal/providers"
)

// TestB298KandidatKosongDitolak adalah pengunci B-298 sisi Google:
// kandidat STOP tanpa konten yang bisa diterjemahkan sebelumnya lolos
// sebagai sukses kosong (penjaga kekosongan kanonik B-261 hanya ada
// di adapter Anthropic). Finish filter-konten dikecualikan —
// kekosongannya sah.
func TestB298KandidatKosongDitolak(t *testing.T) {
	kasus := []struct {
		nama     string
		body     string
		mauGagal bool
	}{
		{"STOP tanpa part",
			`{"candidates":[{"index":0,"content":{"role":"model","parts":[]},"finishReason":"STOP"}],"usageMetadata":{"promptTokenCount":3,"candidatesTokenCount":0,"totalTokenCount":3}}`,
			true},
		{"STOP hanya inlineData video",
			`{"candidates":[{"index":0,"content":{"role":"model","parts":[{"inlineData":{"mimeType":"video/mp4","data":"AAAA"}}]},"finishReason":"STOP"}],"usageMetadata":{"promptTokenCount":3,"candidatesTokenCount":1,"totalTokenCount":4}}`,
			true},
		{"SAFETY tanpa part adalah filter konten yang sah",
			`{"candidates":[{"index":0,"content":{"role":"model","parts":[]},"finishReason":"SAFETY"}]}`,
			false},
		{"STOP dengan teks normal",
			`{"candidates":[{"index":0,"content":{"role":"model","parts":[{"text":"halo"}]},"finishReason":"STOP"}],"usageMetadata":{"promptTokenCount":3,"candidatesTokenCount":1,"totalTokenCount":4}}`,
			false},
		{"STOP dengan gambar inline adalah konten nyata",
			`{"candidates":[{"index":0,"content":{"role":"model","parts":[{"inlineData":{"mimeType":"image/png","data":"aVBvcg=="}}]},"finishReason":"STOP"}],"usageMetadata":{"promptTokenCount":3,"candidatesTokenCount":1,"totalTokenCount":4}}`,
			false},
	}
	for _, k := range kasus {
		t.Run(k.nama, func(t *testing.T) {
			p, _ := newServer(t, jsonHandler(http.StatusOK, k.body))
			resp, err := p.ChatCompletion(context.Background(), simpleRequest())
			if k.mauGagal {
				if err == nil {
					t.Fatalf("ChatCompletion berhasil untuk kandidat kosong — sukses kosong lolos")
				}
				if e := providers.AsError(err); e == nil || e.Kind != providers.ErrKindServer {
					t.Errorf("error = %v, mau *providers.Error berkelas server", err)
				}
				return
			}
			if err != nil {
				t.Fatalf("ChatCompletion gagal untuk kasus kontrol: %v", err)
			}
			if len(resp.Choices) != 1 {
				t.Fatalf("choices = %d, mau 1", len(resp.Choices))
			}
		})
	}
}
