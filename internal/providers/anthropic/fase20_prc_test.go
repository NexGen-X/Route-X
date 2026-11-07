package anthropic

// fase20_prc_test.go — pengunci Fase 20 PR-C (sisi Anthropic).
//
//	B-458 error tengah-aliran tidak lagi meneruskan teks upstream.
//	B-460 error tengah-aliran membawa Retry-After header pembuka.
//	B-464 dokumen hasil tool menjadi block document; audio hasil
//	      tool ditolak eksplisit (sejajar audio pesan user).
//	B-465 Models() berhenti bernama pada has_more + last_id kosong.
//	B-466 sitasi dibuang sebagai KEPUTUSAN TERCATAT; teks utuh.

import (
	"context"
	"net/http"
	"strings"
	"testing"
	"time"

	"github.com/NexGen-X/Route-X/internal/providers"
)

// TestB458ErrorTengahAliranGenerik membuktikan teks bebas upstream
// pada peristiwa error streaming TIDAK mencapai providers.Error —
// yang dibawa hanya klasifikasi + kode upstream.
func TestB458ErrorTengahAliranGenerik(t *testing.T) {
	p, _ := newServer(t, jsonHandler(http.StatusOK, textResponse))
	s := &stream{provider: p}
	env := &streamEnvelope{Error: &struct {
		Type    string `json:"type"`
		Message string `json:"message"`
	}{Type: "invalid_request_error", Message: "PENANDA-BOCOR-B458: prompt rahasia pengguna ditolak karena xyz"}}

	err := s.upstreamError(env)
	if err.Kind != providers.ErrKindInvalidRequest {
		t.Errorf("Kind = %q, mau invalid_request", err.Kind)
	}
	if strings.Contains(err.Message, "PENANDA-BOCOR-B458") || strings.Contains(err.Message, "prompt rahasia") {
		t.Errorf("pesan error masih memuat teks upstream: %q — kebocoran B-458", err.Message)
	}
	if err.UpstreamCode != "invalid_request_error" {
		t.Errorf("UpstreamCode = %q, mau invalid_request_error", err.UpstreamCode)
	}
	if !strings.Contains(err.Message, "invalid_request_error") {
		t.Errorf("pesan generik tidak menyebut kode upstream: %q", err.Message)
	}
}

// TestB460ErrorTengahAliranRetryAfter membuktikan jeda dari header
// respons pembuka aliran menempel pada error tengah-aliran.
func TestB460ErrorTengahAliranRetryAfter(t *testing.T) {
	p, _ := newServer(t, jsonHandler(http.StatusOK, textResponse))
	s := &stream{provider: p, resp: &http.Response{Header: http.Header{
		"Retry-After": []string{"17"},
	}}}
	env := &streamEnvelope{Error: &struct {
		Type    string `json:"type"`
		Message string `json:"message"`
	}{Type: "rate_limit_error", Message: "slow down"}}

	err := s.upstreamError(env)
	if err.Kind != providers.ErrKindRateLimit {
		t.Errorf("Kind = %q, mau rate_limit", err.Kind)
	}
	if err.RetryAfter != 17*time.Second {
		t.Errorf("RetryAfter = %v, mau 17s dari header pembuka", err.RetryAfter)
	}
}

// TestB465ModelsKursorKosongBerhentiBernama membuktikan kombinasi
// has_more=true + last_id="" adalah error server eksplisit, bukan
// katalog sebagian yang dilaporkan sukses.
func TestB465ModelsKursorKosongBerhentiBernama(t *testing.T) {
	body := `{"data":[{"type":"model","id":"claude-sonnet-4-5","created_at":"2025-09-29T00:00:00Z"}],"has_more":true,"last_id":""}`
	p, _ := newServer(t, jsonHandler(http.StatusOK, body))

	_, err := p.Models(context.Background())
	pe := providers.AsError(err)
	if pe == nil {
		t.Fatalf("Models() = sukses, mau error bernama untuk kursor kosong (err=%v)", err)
	}
	if pe.Kind != providers.ErrKindServer {
		t.Errorf("Kind = %q, mau server", pe.Kind)
	}
	if !strings.Contains(pe.Message, "last_id") {
		t.Errorf("pesan tidak menamai sebab berhenti: %q", pe.Message)
	}
}

// TestB464DokumenDanAudioHasilTool membuktikan dua kaki B-464 sisi
// Anthropic: dokumen hasil tool menjadi block document native di
// dalam tool_result; audio hasil tool ditolak eksplisit.
func TestB464DokumenDanAudioHasilTool(t *testing.T) {
	p, log := newServer(t, jsonHandler(http.StatusOK, textResponse))
	req := &providers.ChatRequest{
		Model: "claude-sonnet-4-5",
		Messages: []providers.Message{
			{Role: providers.RoleUser, Content: "pakai tool"},
			{Role: providers.RoleAssistant, ToolCalls: []providers.ToolCall{{
				ID: "toolu_b464", Type: "function",
				Function: providers.FunctionCall{Name: "baca", Arguments: "{}"},
			}}},
			{Role: providers.RoleTool, ToolCallID: "toolu_b464", Parts: []providers.ContentPart{
				{Type: providers.PartTypeText, Text: "berkasnya:"},
				{Type: providers.PartTypeDocument, Document: &providers.Document{
					MediaType: "application/pdf", Data: "JVBERi0xLjQ=",
				}},
			}},
		},
	}
	if _, err := p.ChatCompletion(context.Background(), req); err != nil {
		t.Fatalf("ChatCompletion dokumen hasil tool: %v", err)
	}
	body := string(log.Body())
	if !strings.Contains(body, `"document"`) || !strings.Contains(body, "JVBERi0xLjQ=") {
		t.Errorf("block document hasil tool tidak ada di body upstream: %s", body)
	}

	req.Messages[2].Parts = []providers.ContentPart{
		{Type: providers.PartTypeText, Text: "rekamannya:"},
		{Type: providers.PartTypeAudio, Audio: &providers.Audio{Data: "UklGRg==", Format: "wav"}},
	}
	_, err := p.ChatCompletion(context.Background(), req)
	pe := providers.AsError(err)
	if pe == nil || pe.Kind != providers.ErrKindInvalidRequest {
		t.Fatalf("audio hasil tool: err = %v, mau invalid_request eksplisit", err)
	}
	if !strings.Contains(pe.Message, "audio") {
		t.Errorf("pesan penolakan tidak menyebut audio: %q", pe.Message)
	}
}

// TestB466SitasiAnthropicDibuangTercatat mengunci keputusan B-466:
// respons ber-sitasi diterjemahkan dengan teks UTUH dan tanpa
// atribusi yang dikarang — sitasi gugur sesuai keputusan tercatat
// di translate.go, bukan karena kecelakaan bentuk.
func TestB466SitasiAnthropicDibuangTercatat(t *testing.T) {
	body := `{
		"id": "msg_b466", "type": "message", "role": "assistant",
		"model": "claude-sonnet-4-5",
		"content": [{"type": "text",
			"text": "Jakarta adalah ibu kota Indonesia.",
			"citations": [{"type": "web_search_result_location",
				"url": "https://contoh.test/sumber-b466",
				"title": "Sumber B466", "encrypted_index": "abc"}]}],
		"stop_reason": "end_turn",
		"usage": {"input_tokens": 10, "output_tokens": 5}
	}`
	p, _ := newServer(t, jsonHandler(http.StatusOK, body))

	resp, err := p.ChatCompletion(context.Background(), simpleRequest())
	if err != nil {
		t.Fatalf("ChatCompletion ber-sitasi: %v", err)
	}
	if len(resp.Choices) == 0 || resp.Choices[0].Message.Content != "Jakarta adalah ibu kota Indonesia." {
		t.Fatalf("teks jawaban tidak utuh: %+v", resp.Choices)
	}
	if strings.Contains(string(resp.Raw), "sumber-b466") {
		t.Errorf("sitasi muncul di Raw — keputusan B-466 berubah tanpa catatan: %s", resp.Raw)
	}
}
