package gateway

import (
	"encoding/json"
	"testing"
)

// Test regresi B-137: sub-field thinking yang tidak dikenal gateway
// (mis. "display" dari klien Claude Code modern) sebelumnya terbuang
// diam-diam saat decode permukaan Anthropic — terverifikasi pra-perbaikan:
// body ber-thinking {type:enabled, budget_tokens:2048, display:summarized,
// bidang_baru:nilai-x} ter-decode menjadi hanya {Type, BudgetTokens}.
// Kini sub-field itu dipertahankan mentah di ThinkingParam.Extra agar
// adapter Anthropic meneruskannya setia ke upstream Anthropic.

func TestDecodeAnthropicThinkingSubFieldTakDikenalDipertahankan(t *testing.T) {
	body := []byte(`{
		"model": "claude-test",
		"max_tokens": 4096,
		"messages": [{"role": "user", "content": "halo"}],
		"thinking": {"type": "enabled", "budget_tokens": 2048, "display": "summarized", "bidang_baru": "nilai-x"}
	}`)
	req, err := DecodeAnthropicMessagesRequest(body)
	if err != nil {
		t.Fatalf("DecodeAnthropicMessagesRequest: %v", err)
	}
	if req.Thinking == nil {
		t.Fatal("Thinking nil")
	}
	if req.Thinking.Type != "enabled" || req.Thinking.BudgetTokens != 2048 {
		t.Errorf("Thinking = %+v, mau enabled/2048", req.Thinking)
	}
	if got := string(req.Thinking.Extra["display"]); got != `"summarized"` {
		t.Errorf("Extra[display] = %s, mau \"summarized\" utuh", got)
	}
	if got := string(req.Thinking.Extra["bidang_baru"]); got != `"nilai-x"` {
		t.Errorf("Extra[bidang_baru] = %s, mau \"nilai-x\" utuh", got)
	}
	if _, ada := req.Thinking.Extra["type"]; ada {
		t.Error("Extra memuat type — field dikenal tidak boleh ganda di Extra")
	}
	if _, ada := req.Thinking.Extra["budget_tokens"]; ada {
		t.Error("Extra memuat budget_tokens — field dikenal tidak boleh ganda di Extra")
	}
}

func TestDecodeAnthropicThinkingAdaptiveDenganDisplay(t *testing.T) {
	body := []byte(`{
		"model": "claude-test",
		"max_tokens": 4096,
		"messages": [{"role": "user", "content": "halo"}],
		"thinking": {"type": "adaptive", "display": "summarized"}
	}`)
	req, err := DecodeAnthropicMessagesRequest(body)
	if err != nil {
		t.Fatalf("DecodeAnthropicMessagesRequest: %v", err)
	}
	if req.Thinking == nil || req.Thinking.Type != "adaptive" {
		t.Fatalf("Thinking = %+v, mau adaptive", req.Thinking)
	}
	if got := string(req.Thinking.Extra["display"]); got != `"summarized"` {
		t.Errorf("Extra[display] = %s, mau \"summarized\" utuh", got)
	}
	// Marshal ulang Extra menghasilkan JSON yang sama dengan kiriman klien.
	b, err := json.Marshal(req.Thinking.Extra)
	if err != nil {
		t.Fatalf("marshal Extra: %v", err)
	}
	if string(b) != `{"display":"summarized"}` {
		t.Errorf("Extra termarshal = %s, mau {\"display\":\"summarized\"}", b)
	}
}

func TestDecodeAnthropicThinkingTanpaSubFieldEkstra(t *testing.T) {
	body := []byte(`{
		"model": "claude-test",
		"max_tokens": 4096,
		"messages": [{"role": "user", "content": "halo"}],
		"thinking": {"type": "enabled", "budget_tokens": 1024}
	}`)
	req, err := DecodeAnthropicMessagesRequest(body)
	if err != nil {
		t.Fatalf("DecodeAnthropicMessagesRequest: %v", err)
	}
	if req.Thinking == nil || len(req.Thinking.Extra) != 0 {
		t.Errorf("Thinking = %+v, mau tanpa Extra", req.Thinking)
	}
}
