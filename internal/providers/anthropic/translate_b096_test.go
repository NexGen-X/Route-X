package anthropic

import (
	"context"
	"testing"

	"github.com/NexGen-X/Route-X/internal/providers"
)

// Test regresi B-096 untuk adapter Anthropic: upstream Anthropic asli adalah
// satu-satunya yang mengenal thinking.type "adaptive", jadi di sinilah ia
// diteruskan apa adanya.

// (b) "adaptive" terkirim utuh sebagai thinking.type "adaptive", TANPA
// budget_tokens yang dikarang, dan menang atas ReasoningEffort — sama
// seperti thinking eksplisit lain (V-GW-17).
func TestThinkingAdaptiveDiteruskanApaAdanya(t *testing.T) {
	p, rec := newScriptedServer(t, textResponse)
	req := simpleRequest()
	req.Thinking = &providers.ThinkingParam{Type: "adaptive"}
	req.ReasoningEffort = "high" // tidak boleh mengubah adaptive menjadi enabled/8192
	if _, err := p.ChatCompletion(context.Background(), req); err != nil {
		t.Fatalf("ChatCompletion: %v", err)
	}
	th := rec.at(t, 0).Thinking
	if th["type"] != "adaptive" {
		t.Errorf("thinking = %v, mau type adaptive persis seperti diminta klien", th)
	}
	if _, ada := th["budget_tokens"]; ada {
		t.Errorf("thinking = %v, mau tanpa budget_tokens (adaptive tidak punya anggaran)", th)
	}
}

// "adaptive" adalah thinking yang menyala: temperature/top_p distrip persis
// seperti jalur enabled, karena Anthropic menolak keduanya saat thinking
// menyala — dan kombinasi keduanya tidak boleh menjadi penolakan juga.
func TestThinkingAdaptiveStripTemperatureTopP(t *testing.T) {
	p, rec := newScriptedServer(t, textResponse)
	req := simpleRequest()
	req.Temperature = ptrFloat(0.5)
	req.TopP = ptrFloat(0.9)
	req.Thinking = &providers.ThinkingParam{Type: "adaptive"}
	if _, err := p.ChatCompletion(context.Background(), req); err != nil {
		t.Fatalf("ChatCompletion: %v", err)
	}
	body := rec.at(t, 0)
	if body.Temperature != nil || body.TopP != nil {
		t.Errorf("temperature/top_p tetap terkirim saat adaptive menyala: %v/%v", body.Temperature, body.TopP)
	}
	if len(body.Thinking) == 0 {
		t.Error("thinking adaptive tidak terkirim")
	}
}
