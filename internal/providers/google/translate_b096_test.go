package google

import (
	"context"
	"testing"

	"github.com/NexGen-X/Route-X/internal/providers"
)

// Test regresi B-096 untuk adapter Google: thinking.type "adaptive" tidak
// punya padanan di Gemini. Ia tidak boleh ditolak dan tidak boleh menjadi
// thinkingConfig yang dikarang — tanpa ReasoningEffort, generationConfig
// tidak membawa thinkingConfig sama sekali (bawaan Gemini berlaku, persis
// makna "model memutuskan sendiri").

func TestThinkingAdaptiveTidakMenjadiThinkingConfig(t *testing.T) {
	p, log := newScriptedServer(t, textResponse)
	req := simpleRequest()
	req.Thinking = &providers.ThinkingParam{Type: "adaptive"}
	if _, err := p.ChatCompletion(context.Background(), req); err != nil {
		t.Fatalf("ChatCompletion(adaptive): %v — B-096: adaptive tidak boleh ditolak adapter Google", err)
	}
	cfg := decodeRequest(t, log.raw(t, 0)).GenerationConfig
	if thinking, ada := cfg["thinkingConfig"]; ada {
		t.Errorf("thinkingConfig = %v, mau tidak terkirim untuk adaptive tanpa effort", thinking)
	}
}

// ReasoningEffort eksplisit tetap menjadi thinkingConfig seperti sebelumnya —
// B-096 tidak menyentuh jalur effort.
func TestThinkingAdaptiveDenganEffortTetapMemakaiEffort(t *testing.T) {
	p, log := newScriptedServer(t, textResponse)
	req := simpleRequest()
	req.Thinking = &providers.ThinkingParam{Type: "adaptive"}
	req.ReasoningEffort = "low"
	if _, err := p.ChatCompletion(context.Background(), req); err != nil {
		t.Fatalf("ChatCompletion: %v", err)
	}
	cfg := decodeRequest(t, log.raw(t, 0)).GenerationConfig
	thinking, ada := cfg["thinkingConfig"].(map[string]any)
	if !ada {
		t.Fatalf("thinkingConfig tidak terkirim padahal effort=low: %v", cfg)
	}
	if thinking["thinkingBudget"] != float64(1024) {
		t.Errorf("thinkingBudget = %v, mau 1024 dari effort low (perilaku lama)", thinking["thinkingBudget"])
	}
}
