package google

import (
	"context"
	"net/http"
	"testing"
)

// B-262: signature pada part THOUGHT (bukan hanya functionCall) kini
// sampai ke Message.ReasoningSignature kanonik. Sebelumnya case thought
// hanya menyalin teksnya — untuk model thinking murni, signature yang
// menempel pada part thought hilang dan kelanjutan penalaran lintas
// giliran terputus.
func TestB262SignaturePartThoughtSampaiKanonik(t *testing.T) {
	body := `{
	  "candidates": [{
	    "index": 0,
	    "content": {"role": "model", "parts": [
	      {"thought": true, "text": "mempertimbangkan pilihan", "thoughtSignature": "sig-pikiran-262"},
	      {"text": "jawaban akhir"}
	    ]},
	    "finishReason": "STOP"
	  }],
	  "usageMetadata": {"promptTokenCount": 10, "candidatesTokenCount": 5, "totalTokenCount": 15}
	}`
	p, _ := newServer(t, jsonHandler(http.StatusOK, body))
	resp, err := p.ChatCompletion(context.Background(), simpleRequest())
	if err != nil {
		t.Fatalf("ChatCompletion: %v", err)
	}
	if len(resp.Choices) != 1 {
		t.Fatalf("choices = %d, mau 1", len(resp.Choices))
	}
	msg := resp.Choices[0].Message
	if msg.ReasoningContent != "mempertimbangkan pilihan" {
		t.Errorf("ReasoningContent = %q, mau %q", msg.ReasoningContent, "mempertimbangkan pilihan")
	}
	if msg.ReasoningSignature != "sig-pikiran-262" {
		t.Errorf("ReasoningSignature = %q, mau %q — signature part thought hilang", msg.ReasoningSignature, "sig-pikiran-262")
	}
	if msg.Content != "jawaban akhir" {
		t.Errorf("Content = %q, mau %q", msg.Content, "jawaban akhir")
	}
}

// B-263 (sisi Google): thoughtSignature NUMERIK dari upstream/proksi
// tidak lagi menggagalkan unmarshal seluruh respons — ia dinormalkan
// menjadi teks literalnya, pada part thought maupun functionCall.
func TestB263SignatureNumerikTidakMenggagalkanRespons(t *testing.T) {
	body := `{
	  "candidates": [{
	    "index": 0,
	    "content": {"role": "model", "parts": [
	      {"thought": true, "text": "berpikir", "thoughtSignature": 987654},
	      {"functionCall": {"name": "baca_berkas", "args": {"path": "a.txt"}}, "thoughtSignature": 12345}
	    ]},
	    "finishReason": "STOP"
	  }],
	  "usageMetadata": {"promptTokenCount": 10, "candidatesTokenCount": 5, "totalTokenCount": 15}
	}`
	p, _ := newServer(t, jsonHandler(http.StatusOK, body))
	resp, err := p.ChatCompletion(context.Background(), simpleRequest())
	if err != nil {
		t.Fatalf("ChatCompletion gagal oleh signature numerik: %v", err)
	}
	if len(resp.Choices) != 1 {
		t.Fatalf("choices = %d, mau 1", len(resp.Choices))
	}
	msg := resp.Choices[0].Message
	if msg.ReasoningSignature != "987654" {
		t.Errorf("ReasoningSignature = %q, mau %q (literal angka)", msg.ReasoningSignature, "987654")
	}
	if len(msg.ToolCalls) != 1 || msg.ToolCalls[0].ThoughtSignature != "12345" {
		t.Errorf("ToolCalls = %+v, mau satu panggilan dengan ThoughtSignature %q", msg.ToolCalls, "12345")
	}
}
