package gateway

import (
	"context"
	"encoding/json"
	"net/http"
	"testing"

	"github.com/NexGen-X/Route-X/internal/providers"
)

// TestB293ResponsChoiceKosongDitolakSemuaPermukaan adalah pengunci
// B-293 (residu B-247): adapter yang "berhasil" dengan satu Choice
// tetapi pesan KOSONG total dan Raw kosong sebelumnya dikarang sukses
// 200 oleh Responses & Messages, padahal permukaan chat menjawab 500
// untuk keadaan yang sama. Kriteria penolakan kini memakai penjaga
// kekosongan bersama: choice ada tetapi tak satu pun memuat konten
// yang bisa diteruskan = pelanggaran kontrak adapter, bukan jawaban.
func TestB293ResponsChoiceKosongDitolakSemuaPermukaan(t *testing.T) {
	prov := providerSukses()
	prov.chat = func(_ context.Context, _ *providers.ChatRequest) (*providers.ChatResponse, error) {
		return &providers.ChatResponse{
			ID: "chatcmpl-kosong",
			Choices: []providers.Choice{{
				Index:        0,
				Message:      providers.Message{Role: providers.RoleAssistant},
				FinishReason: "stop",
			}},
		}, nil
	}
	s := susun(t, prov, nil)

	wr := s.panggil(t, http.MethodPost, "/responses", `{"model":"gpt-4o-mini","input":"halo"}`, nil)
	if wr.Code != http.StatusInternalServerError {
		t.Errorf("responses choice kosong: status = %d, mau 500 (body: %s)", wr.Code, wr.Body.String())
	}

	wa := s.panggil(t, http.MethodPost, "/messages",
		`{"model":"gpt-4o-mini","max_tokens":10,"messages":[{"role":"user","content":"halo"}]}`, nil)
	if wa.Code != http.StatusInternalServerError {
		t.Errorf("messages choice kosong: status = %d, mau 500 (body: %s)", wa.Code, wa.Body.String())
	}

	wc := s.panggil(t, http.MethodPost, "/chat/completions", bodyChatMinimal, nil)
	if wc.Code != http.StatusInternalServerError {
		t.Errorf("chat choice kosong: status = %d, mau 500 (konsistensi)", wc.Code)
	}

	// Kontrol: choice dengan penalaran SAJA (tanpa teks) adalah jawaban
	// sah — penjaga tidak boleh menolaknya.
	prov2 := providerSukses()
	prov2.chat = func(_ context.Context, _ *providers.ChatRequest) (*providers.ChatResponse, error) {
		return &providers.ChatResponse{
			ID: "chatcmpl-nalar",
			Choices: []providers.Choice{{
				Index:        0,
				Message:      providers.Message{Role: providers.RoleAssistant, ReasoningContent: "berpikir keras"},
				FinishReason: "stop",
			}},
			Raw: json.RawMessage(`{"id":"chatcmpl-nalar"}`),
		}, nil
	}
	s2 := susun(t, prov2, nil)
	wr2 := s2.panggil(t, http.MethodPost, "/responses", `{"model":"gpt-4o-mini","input":"halo"}`, nil)
	if wr2.Code != http.StatusOK {
		t.Errorf("responses reasoning-saja: status = %d, mau 200 (body: %s)", wr2.Code, wr2.Body.String())
	}
}
