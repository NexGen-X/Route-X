package google

import (
	"context"
	"encoding/json"
	"errors"
	"io"
	"strings"
	"testing"

	"github.com/NexGen-X/Route-X/internal/providers"
)

// TestB211PartNonTeksNonStream membuktikan B-211 di jalur non-streaming:
// part keluaran Gemini di luar teks — gambar hasil generasi (inlineData),
// kode eksekusi, dan hasil eksekusi — tidak lagi dibuang diam-diam.
// Gambar menjadi part image_url ber-data-URI di Message.Parts (dengan
// urutan part persis seperti upstream), kode + hasil eksekusi menjadi
// teks jawaban, dan Raw dialek OpenAI merender Parts sebagai larik
// content. Di kode lama test ini gagal: Parts selalu kosong dan Raw
// content hanya memuat teks pembuka.
func TestB211PartNonTeksNonStream(t *testing.T) {
	body := `{
	  "candidates": [{
	    "content": {"role": "model", "parts": [
	      {"text": "Ini gambarnya: "},
	      {"inlineData": {"mimeType": "image/png", "data": "aGVsbG8="}},
	      {"executableCode": {"language": "PYTHON", "code": "print(42)"}},
	      {"codeExecutionResult": {"outcome": "OUTCOME_OK", "output": "42"}}
	    ]},
	    "finishReason": "STOP",
	    "index": 0
	  }],
	  "usageMetadata": {"promptTokenCount": 10, "candidatesTokenCount": 20, "totalTokenCount": 30},
	  "modelVersion": "gemini-2.5-flash-001",
	  "responseId": "resp-b211"
	}`
	p, _ := newServer(t, jsonHandler(200, body))

	resp, err := p.ChatCompletion(context.Background(), simpleRequest())
	if err != nil {
		t.Fatalf("ChatCompletion: %v", err)
	}
	msg := resp.Choices[0].Message

	// Urutan Parts persis urutan upstream: teks, gambar, kode, hasil.
	if len(msg.Parts) != 4 {
		t.Fatalf("jumlah Parts = %d, ingin 4 (%+v)", len(msg.Parts), msg.Parts)
	}
	if msg.Parts[0].Type != providers.PartTypeText || msg.Parts[0].Text != "Ini gambarnya: " {
		t.Errorf("Parts[0] = %+v, ingin teks pembuka", msg.Parts[0])
	}
	if msg.Parts[1].Type != providers.PartTypeImageURL || msg.Parts[1].ImageURL == nil ||
		msg.Parts[1].ImageURL.URL != "data:image/png;base64,aGVsbG8=" {
		t.Errorf("Parts[1] = %+v, ingin image_url data:image/png;base64,aGVsbG8=", msg.Parts[1])
	}
	if msg.Parts[2].Type != providers.PartTypeText || msg.Parts[2].Text != "print(42)" {
		t.Errorf("Parts[2] = %+v, ingin teks kode print(42)", msg.Parts[2])
	}
	if msg.Parts[3].Type != providers.PartTypeText || msg.Parts[3].Text != "42" {
		t.Errorf("Parts[3] = %+v, ingin teks hasil 42", msg.Parts[3])
	}
	// Kode + hasil eksekusi adalah teks jawaban: Content memuat semuanya.
	if msg.Content != "Ini gambarnya: print(42)42" {
		t.Errorf("Content = %q, ingin gabungan teks + kode + hasil", msg.Content)
	}
	// Text() membaca Parts bila terisi dan menggabungkan segmen teksnya
	// dengan newline (kontrak Text() untuk pesan multimodal): ketiga
	// segmen — pembuka, kode, hasil — harus hadir di sana.
	teks := msg.Text()
	for _, segmen := range []string{"Ini gambarnya: ", "print(42)", "42"} {
		if !strings.Contains(teks, segmen) {
			t.Errorf("Text() = %q, kehilangan segmen %q", teks, segmen)
		}
	}

	// Raw dialek OpenAI: content adalah LARIK dengan elemen image_url.
	var raw struct {
		Choices []struct {
			Message struct {
				Content []struct {
					Type     string `json:"type"`
					Text     string `json:"text"`
					ImageURL *struct {
						URL string `json:"url"`
					} `json:"image_url"`
				} `json:"content"`
			} `json:"message"`
		} `json:"choices"`
	}
	if err := json.Unmarshal(resp.Raw, &raw); err != nil {
		t.Fatalf("Raw bukan larik content yang sah: %v (%s)", err, resp.Raw)
	}
	elemen := raw.Choices[0].Message.Content
	if len(elemen) != 4 || elemen[1].Type != "image_url" || elemen[1].ImageURL == nil ||
		elemen[1].ImageURL.URL != "data:image/png;base64,aGVsbG8=" {
		t.Errorf("Raw content = %+v, ingin larik 4 elemen dengan image_url di posisi 1", elemen)
	}
}

// TestB211AudioInlineMenjadiPartAudio membuktikan part audio hasil
// generasi menjadi part input_audio kanonik berisi data + format yang sama.
func TestB211AudioInlineMenjadiPartAudio(t *testing.T) {
	body := `{
	  "candidates": [{
	    "content": {"role": "model", "parts": [
	      {"inlineData": {"mimeType": "audio/mp3", "data": "QUJD"}}
	    ]},
	    "finishReason": "STOP",
	    "index": 0
	  }],
	  "modelVersion": "gemini-2.5-flash-001"
	}`
	p, _ := newServer(t, jsonHandler(200, body))

	resp, err := p.ChatCompletion(context.Background(), simpleRequest())
	if err != nil {
		t.Fatalf("ChatCompletion: %v", err)
	}
	msg := resp.Choices[0].Message
	if len(msg.Parts) != 1 || msg.Parts[0].Type != providers.PartTypeAudio || msg.Parts[0].Audio == nil {
		t.Fatalf("Parts = %+v, ingin satu part audio", msg.Parts)
	}
	if msg.Parts[0].Audio.Data != "QUJD" || msg.Parts[0].Audio.Format != "mp3" {
		t.Errorf("Audio = %+v, ingin data QUJD format mp3", msg.Parts[0].Audio)
	}
	if !strings.Contains(string(resp.Raw), `"input_audio"`) || !strings.Contains(string(resp.Raw), `"format":"mp3"`) {
		t.Errorf("Raw tidak merender part audio: %s", resp.Raw)
	}
}

// TestB211PartNonTeksStream membuktikan B-211 di jalur streaming: peristiwa
// berpart gambar membawa StreamEvent.Parts dan Raw-nya merender content
// sebagai larik, sementara peristiwa teks murni mempertahankan bentuk
// string persis seperti sebelumnya.
func TestB211PartNonTeksStream(t *testing.T) {
	p, _ := newServer(t, sseHandler(
		`data: {"candidates":[{"content":{"role":"model","parts":[{"text":"halo "}]},"index":0}]}`+"\n\n",
		`data: {"candidates":[{"content":{"role":"model","parts":[{"text":"gambar: "},{"inlineData":{"mimeType":"image/png","data":"aGVsbG8="}}]},"index":0}]}`+"\n\n",
		`data: {"candidates":[{"content":{"role":"model","parts":[]},"finishReason":"STOP","index":0}],"usageMetadata":{"promptTokenCount":10,"candidatesTokenCount":5,"totalTokenCount":15}}`+"\n\n",
	))

	st, err := p.ChatCompletionStream(context.Background(), simpleRequest())
	if err != nil {
		t.Fatalf("ChatCompletionStream: %v", err)
	}
	defer st.Close()

	var peristiwa []*providers.StreamEvent
	for {
		ev, err := st.Recv()
		if errors.Is(err, io.EOF) {
			break
		}
		if err != nil {
			t.Fatalf("Recv: %v", err)
		}
		peristiwa = append(peristiwa, ev)
	}

	var denganParts *providers.StreamEvent
	teks := ""
	for _, ev := range peristiwa {
		teks += ev.Delta
		if len(ev.Parts) > 0 {
			denganParts = ev
		}
	}
	if teks != "halo gambar: " {
		t.Errorf("teks tergabung = %q, ingin \"halo gambar: \"", teks)
	}
	if denganParts == nil {
		t.Fatal("tidak ada peristiwa ber-Parts; part gambar hilang di aliran")
	}
	if len(denganParts.Parts) != 2 || denganParts.Parts[1].Type != providers.PartTypeImageURL ||
		denganParts.Parts[1].ImageURL == nil || denganParts.Parts[1].ImageURL.URL != "data:image/png;base64,aGVsbG8=" {
		t.Errorf("Parts peristiwa = %+v, ingin [teks, image_url data URI]", denganParts.Parts)
	}
	if !strings.Contains(string(denganParts.Raw), `"content":[{`) ||
		!strings.Contains(string(denganParts.Raw), `"image_url":{"url":"data:image/png;base64,aGVsbG8="}`) {
		t.Errorf("Raw peristiwa berpart bukan larik content bergambar: %s", denganParts.Raw)
	}
	// Peristiwa teks murni: content tetap string.
	if len(peristiwa) > 0 && !strings.Contains(string(peristiwa[0].Raw), `"content":"halo "`) {
		t.Errorf("Raw peristiwa teks murni berubah bentuk: %s", peristiwa[0].Raw)
	}
}

// TestB211TeksMurniTanpaParts adalah penjaga regresi: respons teks murni
// tidak memperoleh Parts dan Raw content-nya tetap string.
func TestB211TeksMurniTanpaParts(t *testing.T) {
	body := `{
	  "candidates": [{
	    "content": {"role": "model", "parts": [{"text": "jawaban biasa"}]},
	    "finishReason": "STOP",
	    "index": 0
	  }],
	  "modelVersion": "gemini-2.5-flash-001"
	}`
	p, _ := newServer(t, jsonHandler(200, body))

	resp, err := p.ChatCompletion(context.Background(), simpleRequest())
	if err != nil {
		t.Fatalf("ChatCompletion: %v", err)
	}
	msg := resp.Choices[0].Message
	if len(msg.Parts) != 0 {
		t.Errorf("Parts = %+v untuk respons teks murni, ingin kosong", msg.Parts)
	}
	if !strings.Contains(string(resp.Raw), `"content":"jawaban biasa"`) {
		t.Errorf("Raw content bukan string polos: %s", resp.Raw)
	}
}
