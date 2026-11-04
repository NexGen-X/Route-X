package openai

import (
	"encoding/json"
	"testing"

	"github.com/NexGen-X/Route-X/internal/providers"
)

// Test regresi B-137 (adapter non-Anthropic): sub-field thinking tak
// dikenal yang dipertahankan decode untuk upstream Anthropic DIBUANG di
// sini bersama seluruh parameter thinking — konsisten dengan B-096 dan
// kini terdokumentasi di providers.ThinkingParam: upstream
// OpenAI-compatible tidak punya konsep thinking bergaya Anthropic, jadi
// tidak ada padanan setia yang bisa dikirim.
func TestPayloadThinkingExtraDibuangUntukNonAnthropic(t *testing.T) {
	p := &Provider{}
	req := &providers.ChatRequest{
		Model:    "intern-s2",
		Messages: []providers.Message{{Role: providers.RoleUser, Content: "halo"}},
		Thinking: &providers.ThinkingParam{
			Type:         "enabled",
			BudgetTokens: 2048,
			Extra: map[string]json.RawMessage{
				"display": json.RawMessage(`"summarized"`),
			},
		},
	}
	payload, err := p.chatPayload(req, false)
	if err != nil {
		t.Fatalf("chatPayload: %v", err)
	}
	var body map[string]any
	if err := json.Unmarshal(payload, &body); err != nil {
		t.Fatalf("payload bukan JSON sah: %v", err)
	}
	if _, ada := body["thinking"]; ada {
		t.Errorf("thinking terkirim ke upstream non-Anthropic: %s", payload)
	}
	if _, ada := body["display"]; ada {
		t.Errorf("sub-field display bocor ke payload non-Anthropic: %s", payload)
	}
}
