package gateway

import (
	"context"
	"net/http"
	"strings"
	"testing"

	"github.com/NexGen-X/Route-X/internal/providers"
)

// TestB301AliranChatTanpaKontenGagal adalah pengunci B-301 sisi chat:
// aliran yang berakhir dengan SATU peristiwa finish saja (tanpa satu
// byte konten pun) sebelumnya ditutup sebagai sukses bersih — klien
// menerima 200 dengan body hanya "data: [DONE]", request tercatat
// sukses, webhook request.completed terkirim. Kini ia kegagalan
// upstream di dalam aliran. Finish content_filter dikecualikan:
// kekosongannya sah, sejajar pengecualian B-261/B-298.
func TestB301AliranChatTanpaKontenGagal(t *testing.T) {
	susunAliran := func(ev ...*providers.StreamEvent) *susunan {
		prov := &providerTiruan{
			nama: "utama", kind: providers.KindOpenAI,
			alir: func(_ context.Context, _ *providers.ChatRequest) (providers.Stream, error) {
				return &aliranTiruan{ev: ev}, nil
			},
		}
		return susun(t, prov, nil)
	}

	t.Run("finish saja tanpa konten adalah kegagalan", func(t *testing.T) {
		s := susunAliran(&providers.StreamEvent{FinishReason: providers.FinishStop})
		w := s.panggil(t, http.MethodPost, "/chat/completions", bodyChatStreaming, nil)
		if w.Code != http.StatusOK {
			t.Fatalf("status = %d, mau 200 (header SSE sudah terkirim)", w.Code)
		}
		body := w.Body.String()
		if !strings.Contains(body, `"error"`) {
			t.Errorf("body = %q, mau memuat peristiwa error — aliran tanpa konten ditutup sebagai sukses bersih", body)
		}
		if !strings.HasSuffix(body, "data: [DONE]\n\n") {
			t.Errorf("body = %q, mau tetap diakhiri penanda [DONE]", body)
		}
	})

	t.Run("finish content_filter tanpa konten tetap sukses bersih", func(t *testing.T) {
		s := susunAliran(&providers.StreamEvent{FinishReason: providers.FinishContentFilter})
		w := s.panggil(t, http.MethodPost, "/chat/completions", bodyChatStreaming, nil)
		body := w.Body.String()
		if strings.Contains(body, `"error"`) {
			t.Errorf("body = %q, mau tanpa peristiwa error untuk filter konten", body)
		}
		if body != "data: [DONE]\n\n" {
			t.Errorf("body = %q, mau hanya penanda [DONE]", body)
		}
	})

	t.Run("aliran berkonten tetap sukses bersih", func(t *testing.T) {
		s := susunAliran(peristiwa("hai"), &providers.StreamEvent{FinishReason: providers.FinishStop})
		w := s.panggil(t, http.MethodPost, "/chat/completions", bodyChatStreaming, nil)
		body := w.Body.String()
		if strings.Contains(body, `"error"`) {
			t.Errorf("body = %q, mau tanpa peristiwa error untuk aliran berkonten", body)
		}
	})
}

// TestB301AliranMessagesTanpaKontenGagal adalah pengunci B-301 sisi
// Messages: aliran finish-saja sebelumnya ditutup dengan message_delta
// end_turn dan output_tokens 1 HASIL KLEM (upstream melaporkan 0) —
// sukses karangan lengkap dengan angka karangan. Kini permukaan
// mengirim peristiwa error Anthropic dan jejak ditandai gagal; tidak
// ada message_delta end_turn. Refusal (filter konten) tetap ditutup
// normal dengan stop_reason refusal.
func TestB301AliranMessagesTanpaKontenGagal(t *testing.T) {
	t.Run("finish saja tanpa konten adalah kegagalan", func(t *testing.T) {
		aliran := &aliranTiruan{ev: []*providers.StreamEvent{
			{FinishReason: providers.FinishStop, Usage: &providers.Usage{InputTokens: 5, OutputTokens: 0, TotalTokens: 5}},
		}}
		ps := panggilStreamMessages(t, aliran)
		var adaError, adaDelta bool
		for _, p := range ps {
			if p.nama == "error" {
				adaError = true
			}
			if p.nama == "message_delta" {
				adaDelta = true
			}
		}
		if !adaError {
			t.Errorf("peristiwa = %v, mau memuat peristiwa error", namaPeristiwa(ps))
		}
		if adaDelta {
			t.Errorf("peristiwa = %v, mau TANPA message_delta — end_turn karangan untuk aliran tanpa konten", namaPeristiwa(ps))
		}
	})

	t.Run("refusal tanpa konten tetap ditutup normal", func(t *testing.T) {
		aliran := &aliranTiruan{ev: []*providers.StreamEvent{
			{FinishReason: providers.FinishContentFilter, Usage: &providers.Usage{InputTokens: 5, OutputTokens: 0, TotalTokens: 5}},
		}}
		ps := panggilStreamMessages(t, aliran)
		var delta *ssePeristiwa
		for i := range ps {
			if ps[i].nama == "error" {
				t.Fatalf("peristiwa error untuk refusal yang sah: %v", namaPeristiwa(ps))
			}
			if ps[i].nama == "message_delta" {
				delta = &ps[i]
			}
		}
		if delta == nil {
			t.Fatalf("peristiwa = %v, mau message_delta untuk refusal", namaPeristiwa(ps))
		}
		if d := deltaBlok(t, *delta); d["stop_reason"] != "refusal" {
			t.Errorf("stop_reason = %v, mau refusal", d["stop_reason"])
		}
	})
}
