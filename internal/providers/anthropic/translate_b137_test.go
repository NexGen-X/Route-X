package anthropic

import (
	"context"
	"encoding/json"
	"testing"

	"github.com/NexGen-X/Route-X/internal/providers"
)

// Test regresi B-137 (adapter Anthropic): sub-field thinking tak dikenal
// yang dipertahankan decode di ThinkingParam.Extra diteruskan SETIA ke
// body upstream Anthropic — dibuktikan dari body yang ditangkap server
// tiruan, bukan dari struktur internal.

func TestThinkingExtraDiteruskanKeUpstreamAnthropic(t *testing.T) {
	p, rec := newScriptedServer(t, textResponse)
	req := simpleRequest()
	req.Thinking = &providers.ThinkingParam{
		Type:         "enabled",
		BudgetTokens: 2048,
		Extra: map[string]json.RawMessage{
			"display":     json.RawMessage(`"summarized"`),
			"bidang_baru": json.RawMessage(`"nilai-x"`),
		},
	}
	req.MaxTokens = ptrInt(8192)
	if _, err := p.ChatCompletion(context.Background(), req); err != nil {
		t.Fatalf("ChatCompletion: %v", err)
	}
	th := rec.at(t, 0).Thinking
	if th["type"] != "enabled" || th["budget_tokens"] != float64(2048) {
		t.Errorf("thinking = %v, mau enabled/2048", th)
	}
	if th["display"] != "summarized" {
		t.Errorf("thinking.display = %v, mau summarized diteruskan setia (B-137)", th["display"])
	}
	if th["bidang_baru"] != "nilai-x" {
		t.Errorf("thinking.bidang_baru = %v, mau nilai-x diteruskan setia (B-137)", th["bidang_baru"])
	}
}

func TestThinkingAdaptiveExtraDiteruskanKeUpstreamAnthropic(t *testing.T) {
	p, rec := newScriptedServer(t, textResponse)
	req := simpleRequest()
	req.Thinking = &providers.ThinkingParam{
		Type: "adaptive",
		Extra: map[string]json.RawMessage{
			"display": json.RawMessage(`"summarized"`),
		},
	}
	if _, err := p.ChatCompletion(context.Background(), req); err != nil {
		t.Fatalf("ChatCompletion: %v", err)
	}
	th := rec.at(t, 0).Thinking
	if th["type"] != "adaptive" {
		t.Errorf("thinking = %v, mau type adaptive", th)
	}
	if th["display"] != "summarized" {
		t.Errorf("thinking.display = %v, mau summarized diteruskan setia (B-137)", th["display"])
	}
	if _, ada := th["budget_tokens"]; ada {
		t.Errorf("thinking memuat budget_tokens karangan untuk adaptive: %v", th)
	}
}

func TestThinkingTanpaExtraTidakBerubah(t *testing.T) {
	p, rec := newScriptedServer(t, textResponse)
	req := simpleRequest()
	req.Thinking = &providers.ThinkingParam{Type: "enabled", BudgetTokens: 2048}
	req.MaxTokens = ptrInt(8192)
	if _, err := p.ChatCompletion(context.Background(), req); err != nil {
		t.Fatalf("ChatCompletion: %v", err)
	}
	th := rec.at(t, 0).Thinking
	if len(th) != 2 {
		t.Errorf("thinking = %v, mau tepat {type, budget_tokens} tanpa Extra", th)
	}
}
