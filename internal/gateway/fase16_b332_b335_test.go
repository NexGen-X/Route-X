package gateway

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/NexGen-X/Route-X/internal/database/repo/upstream"
	"github.com/NexGen-X/Route-X/internal/providers"
)

// TestB332GambarMunculDiResponses adalah pengunci B-332: encoder
// non-streaming /v1/responses menyusun isi pesan HANYA dari
// PartTypeText — respons model penghasil gambar (yang oleh adapter
// dinyatakan sebagai part image_url pada pesan kanonik) tiba di
// klien Responses sebagai jawaban teks saja atau bahkan pesan
// kosong. Dekoder permukaan ini sudah membaca input_image; arah
// keluar kini simetris: part gambar menjadi
// {"type":"output_image","image_url":...}.
func TestB332GambarMunculDiResponses(t *testing.T) {
	resp := &providers.ChatResponse{
		ID:      "chatcmpl-b332",
		Model:   "model-gambar",
		Created: 1700000000,
		Choices: []providers.Choice{{
			Index: 0,
			Message: providers.Message{
				Role:    providers.RoleAssistant,
				Content: "ini gambarnya",
				Parts: []providers.ContentPart{
					{Type: providers.PartTypeText, Text: "ini gambarnya"},
					{Type: providers.PartTypeImageURL, ImageURL: &providers.ImageURL{URL: "data:image/png;base64,aGVsbG8="}},
				},
			},
			FinishReason: providers.FinishStop,
		}},
	}
	raw := kodekanResponsResponses(resp, "model-gambar", false)
	var obj struct {
		Output []struct {
			Type    string `json:"type"`
			Content []struct {
				Type     string `json:"type"`
				Text     string `json:"text"`
				ImageURL string `json:"image_url"`
			} `json:"content"`
		} `json:"output"`
	}
	if err := json.Unmarshal(raw, &obj); err != nil {
		t.Fatalf("respons bukan JSON sah: %v (%s)", err, raw)
	}
	var teks, gambar string
	for _, item := range obj.Output {
		if item.Type != "message" {
			continue
		}
		for _, p := range item.Content {
			switch p.Type {
			case "output_text":
				teks = p.Text
			case "output_image":
				gambar = p.ImageURL
			}
		}
	}
	if teks != "ini gambarnya" {
		t.Errorf("output_text = %q, mau %q", teks, "ini gambarnya")
	}
	if gambar != "data:image/png;base64,aGVsbG8=" {
		t.Errorf("output_image = %q, mau data URI gambar — part gambar hilang dari respons Responses", gambar)
	}
}

// TestB335AdapterNilBukanPanic adalah pengunci B-335: cobaKandidat
// mengembalikan nilai adapter dengan ok=true setiap kali error-nya
// nil TANPA memeriksa nil — adapter (atau tiruan uji di masa depan)
// yang mengembalikan (nil, nil) menghasilkan Outcome sukses
// ber-Value nil, dan handler men-dereference-nya: panic 500 pada
// chat maupun embeddings. Invariant executor sendiri ("Outcome
// tanpa Err dan tanpa Value TIDAK boleh keluar") kini ditegakkan
// juga pada jalur sukses: hasil nil tanpa error diperlakukan
// sebagai kegagalan percobaan berkind server.
func TestB335AdapterNilBukanPanic(t *testing.T) {
	t.Run("chat", func(t *testing.T) {
		prov := &providerTiruan{
			nama: "utama", kind: providers.KindOpenAI,
			chat: func(_ context.Context, _ *providers.ChatRequest) (*providers.ChatResponse, error) {
				return nil, nil
			},
		}
		s := susun(t, prov, nil)
		w := panggilTanpaPanic(t, s, "/chat/completions", bodyChat)
		if w.Code < 500 {
			t.Errorf("status = %d, mau 5xx terkendali (body: %s)", w.Code, w.Body.String())
		}
		if !strings.Contains(w.Body.String(), `"error"`) {
			t.Errorf("body = %q, mau envelope error", w.Body.String())
		}
	})

	t.Run("embeddings", func(t *testing.T) {
		prov := &providerTiruan{
			nama: "utama", kind: providers.KindOpenAI,
			embed: func(_ context.Context, _ *providers.EmbeddingsRequest) (*providers.EmbeddingsResponse, error) {
				return nil, nil
			},
		}
		// Model uji harness harus menyatakan kemampuan embeddings,
		// kalau tidak permintaan ditolak 400 sebelum mencapai
		// provider (pola yang sama seperti test embeddings lain).
		s := susun(t, prov, func(s *susunan, _ *HandlersDeps) {
			s.models.model.Capabilities = []string{upstream.CapEmbeddings}
		})
		w := panggilTanpaPanic(t, s, "/embeddings", `{"model":"gpt-4o-mini","input":"halo"}`)
		if w.Code < 500 {
			t.Errorf("status = %d, mau 5xx terkendali (body: %s)", w.Code, w.Body.String())
		}
		if !strings.Contains(w.Body.String(), `"error"`) {
			t.Errorf("body = %q, mau envelope error", w.Body.String())
		}
	})
}

// panggilTanpaPanic memanggil handler dengan recover: panic pada
// kode pra-perbaikan ditangkap di sini dan dilaporkan sebagai
// kegagalan test yang terbaca, bukan crash seluruh paket.
func panggilTanpaPanic(t *testing.T, s *susunan, jalur, body string) *httptest.ResponseRecorder {
	t.Helper()
	defer func() {
		if r := recover(); r != nil {
			t.Fatalf("handler PANIC untuk adapter (nil, nil): %v", r)
		}
	}()
	return s.panggil(t, http.MethodPost, jalur, body, nil)
}
