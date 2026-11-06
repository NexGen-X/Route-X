package gateway

import (
	"encoding/json"
	"strings"
	"testing"

	"github.com/NexGen-X/Route-X/internal/providers"
)

// TestAnthropicNonStreamRefusal membuktikan B-213 di permukaan
// /v1/messages non-streaming: pesan kanonik ber-Refusal dengan
// FinishContentFilter menjadi block teks berisi penjelasan penolakan DAN
// stop_reason "refusal" — bukan end_turn kosong seperti sebelumnya, saat
// field refusal bahkan tidak ada di bentuk kanonik.
func TestAnthropicNonStreamRefusal(t *testing.T) {
	resp := panggilMessagesSekali(t, &providers.ChatResponse{
		ID: "msg_b213",
		Choices: []providers.Choice{{
			Message: providers.Message{
				Role:    providers.RoleAssistant,
				Refusal: "Saya tidak bisa membantu permintaan itu.",
			},
			FinishReason: providers.FinishContentFilter,
		}},
		Usage: providers.Usage{InputTokens: 4, OutputTokens: 6, TotalTokens: 10},
	})

	if resp["stop_reason"] != "refusal" {
		t.Errorf("stop_reason = %v, ingin refusal", resp["stop_reason"])
	}
	konten, ok := resp["content"].([]any)
	if !ok || len(konten) != 1 {
		t.Fatalf("content = %v, ingin tepat 1 block teks penolakan", resp["content"])
	}
	blok, _ := konten[0].(map[string]any)
	if blok["type"] != "text" || blok["text"] != "Saya tidak bisa membantu permintaan itu." {
		t.Errorf("block = %v, ingin teks berisi penjelasan penolakan", blok)
	}
}

// TestAnthropicStreamingRefusalDelta membuktikan potongan RefusalDelta
// mengalir sebagai teks block di permukaan streaming Anthropic dan
// peristiwa penutup FinishContentFilter menjadi stop_reason refusal.
func TestAnthropicStreamingRefusalDelta(t *testing.T) {
	aliran := &aliranTiruan{ev: []*providers.StreamEvent{
		{RefusalDelta: "Saya tidak "},
		{RefusalDelta: "bisa."},
		{
			FinishReason: providers.FinishContentFilter,
			Usage:        &providers.Usage{InputTokens: 4, OutputTokens: 6, TotalTokens: 10},
		},
	}}
	ps := panggilStreamMessages(t, aliran)

	teks := ""
	for _, p := range ps {
		if p.nama == "content_block_delta" {
			teks += deltaBlok(t, p)["text"].(string)
		}
		if p.nama == "message_delta" {
			sr := muatJSON(t, p)["delta"].(map[string]any)["stop_reason"]
			if sr != "refusal" {
				t.Errorf("stop_reason aliran = %v, ingin refusal", sr)
			}
		}
	}
	if teks != "Saya tidak bisa." {
		t.Errorf("teks penolakan tergabung = %q, ingin \"Saya tidak bisa.\"", teks)
	}
}

// TestResponsesRefusalPart membuktikan encoder /v1/responses: pesan
// ber-Refusal menjadi part {"type":"refusal"} pada item message, dan
// status keseluruhan incomplete/content_filter mengikuti B-173.
func TestResponsesRefusalPart(t *testing.T) {
	raw := kodekanResponsResponses(&providers.ChatResponse{
		ID:    "chatcmpl-b213",
		Model: "m",
		Choices: []providers.Choice{{
			Message: providers.Message{
				Role:    providers.RoleAssistant,
				Refusal: "Tidak bisa.",
			},
			FinishReason: providers.FinishContentFilter,
		}},
		Usage: providers.Usage{InputTokens: 2, OutputTokens: 3, TotalTokens: 5},
	}, "m", false)

	var obj map[string]any
	if err := json.Unmarshal(raw, &obj); err != nil {
		t.Fatalf("objek responses bukan JSON: %v", err)
	}
	if obj["status"] != "incomplete" {
		t.Errorf("status = %v, ingin incomplete untuk penolakan", obj["status"])
	}
	out, _ := obj["output"].([]any)
	if len(out) != 1 {
		t.Fatalf("output = %v, ingin 1 item message", obj["output"])
	}
	item, _ := out[0].(map[string]any)
	isi, _ := item["content"].([]any)
	if len(isi) != 1 {
		t.Fatalf("content item = %v, ingin 1 part refusal", item["content"])
	}
	part, _ := isi[0].(map[string]any)
	if part["type"] != "refusal" || part["refusal"] != "Tidak bisa." {
		t.Errorf("part = %v, ingin {type:refusal, refusal:\"Tidak bisa.\"}", part)
	}
	if strings.Contains(string(raw), `"annotations"`) && len(isi) == 1 && part["type"] == "refusal" {
		// Part refusal tidak boleh membawa field output_text karangan.
		if _, ada := part["annotations"]; ada {
			t.Errorf("part refusal membawa annotations: %v", part)
		}
		if _, ada := part["text"]; ada {
			t.Errorf("part refusal membawa text kosong: %v", part)
		}
	}
}
