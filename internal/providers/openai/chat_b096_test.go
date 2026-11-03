package openai

import (
	"encoding/json"
	"strings"
	"testing"

	"github.com/NexGen-X/Route-X/internal/providers"
)

// Test regresi B-096 untuk adapter OpenAI(-compatible): thinking.type
// "adaptive" dari Claude Code TIDAK boleh ditolak dan TIDAK boleh terkirim
// ke upstream yang tidak mengenalnya (kasus live: intern-s2 lewat intern-ai).
// Makna "adaptive" — model memutuskan sendiri — diwakili dengan tidak
// mengirim parameter thinking/reasoning apa pun: bawaan upstream berlaku.

func TestPayloadThinkingAdaptiveDibuangTanpaDitolak(t *testing.T) {
	p := &Provider{}
	req := &providers.ChatRequest{
		Model:    "intern-s2",
		Messages: []providers.Message{{Role: providers.RoleUser, Content: "halo"}},
		Thinking: &providers.ThinkingParam{Type: "adaptive"},
	}
	payload, err := p.chatPayload(req, false)
	if err != nil {
		t.Fatalf("chatPayload(adaptive): %v — B-096: adaptive tidak boleh ditolak adapter non-Anthropic", err)
	}
	if strings.Contains(string(payload), "adaptive") || strings.Contains(string(payload), "thinking") {
		t.Errorf("payload memuat thinking tak dikenal: %s", payload)
	}
	var body map[string]any
	if err := json.Unmarshal(payload, &body); err != nil {
		t.Fatalf("payload bukan JSON sah: %v", err)
	}
	if _, ada := body["reasoning_effort"]; ada {
		t.Errorf("reasoning_effort dikarang dari adaptive: %v — adaptive bukan effort", body["reasoning_effort"])
	}
}

// (c) Regresi: "enabled" dengan budget untuk non-Anthropic mempertahankan
// perilaku yang sudah ada — ia memang selalu diabaikan adapter ini (yang
// berlaku hanya ReasoningEffort eksplisit), dan B-096 tidak mengubahnya.
func TestPayloadThinkingEnabledTetapDiabaikanSepertiSebelumnya(t *testing.T) {
	p := &Provider{}
	req := &providers.ChatRequest{
		Model:    "intern-s2",
		Messages: []providers.Message{{Role: providers.RoleUser, Content: "halo"}},
		Thinking: &providers.ThinkingParam{Type: "enabled", BudgetTokens: 2048},
	}
	payload, err := p.chatPayload(req, false)
	if err != nil {
		t.Fatalf("chatPayload(enabled): %v", err)
	}
	var body map[string]any
	if err := json.Unmarshal(payload, &body); err != nil {
		t.Fatalf("payload bukan JSON sah: %v", err)
	}
	if _, ada := body["thinking"]; ada {
		t.Errorf("thinking terkirim ke upstream OpenAI-compatible: %s", payload)
	}
	if _, ada := body["reasoning_effort"]; ada {
		t.Errorf("reasoning_effort dikarang dari thinking enabled: %s", payload)
	}

	// ReasoningEffort eksplisit tetap diterjemahkan seperti sebelumnya.
	req.ReasoningEffort = "high"
	payload, err = p.chatPayload(req, false)
	if err != nil {
		t.Fatalf("chatPayload(effort): %v", err)
	}
	if err := json.Unmarshal(payload, &body); err != nil {
		t.Fatalf("payload bukan JSON sah: %v", err)
	}
	if body["reasoning_effort"] != "high" {
		t.Errorf("reasoning_effort = %v, mau high (perilaku lama utuh)", body["reasoning_effort"])
	}
}
