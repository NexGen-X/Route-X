package gateway

import (
	"strings"
	"testing"

	"github.com/NexGen-X/Route-X/internal/database/repo/upstream"
	"github.com/NexGen-X/Route-X/internal/providers"
)

// Test regresi B-096: Claude Code modern mengirim thinking.type "adaptive"
// pada POST /v1/messages. Sebelum perbaikan, decode menolaknya 400 untuk
// SEMUA rute — termasuk model non-Claude (intern-s2 lewat intern-ai) yang
// justru menjadi korban laporannya — karena penolakan terjadi sebelum
// routing tahu upstream mana yang melayani.

// (a) Decode menerima "adaptive" dan membawanya ke bentuk kanonik apa adanya.
func TestDecodeAnthropicThinkingAdaptiveDiterima(t *testing.T) {
	body := []byte(`{
		"model": "intern-s2",
		"max_tokens": 100,
		"messages": [{"role": "user", "content": "halo"}],
		"thinking": {"type": "adaptive"}
	}`)
	req, err := DecodeAnthropicMessagesRequest(body)
	if err != nil {
		t.Fatalf("DecodeAnthropicMessagesRequest(adaptive): %v — B-096: adaptive tidak boleh lagi ditolak saat decode", err)
	}
	if req.Thinking == nil || req.Thinking.Type != "adaptive" {
		t.Errorf("Thinking = %+v, mau {adaptive}", req.Thinking)
	}
}

// Nilai yang memang tidak dikenal tetap ditolak — penerimaan adaptive bukan
// penerimaan semua nilai.
func TestDecodeAnthropicThinkingTidakDikenalTetapDitolak(t *testing.T) {
	body := []byte(`{
		"model": "m",
		"max_tokens": 100,
		"messages": [{"role": "user", "content": "halo"}],
		"thinking": {"type": "entah"}
	}`)
	_, err := DecodeAnthropicMessagesRequest(body)
	if err == nil || !strings.Contains(err.Error(), `"entah"`) {
		t.Errorf("err = %v, mau penolakan yang menyebut nilai tak dikenalnya", err)
	}
}

// (a) Per kandidat: "adaptive" distrip untuk kandidat non-Anthropic — salinan
// untuk upstream OpenAI-compatible tidak membawa parameter thinking apa pun —
// dan permintaan asal TIDAK berubah (kandidat Anthropic pada percobaan
// failover berikutnya masih bisa menerima nilai aslinya).
func TestUntukKandidatAdaptiveDistripNonAnthropic(t *testing.T) {
	req := &providers.ChatRequest{
		Model:    "intern-s2",
		Thinking: &providers.ThinkingParam{Type: "adaptive"},
	}
	for _, kind := range []string{providers.KindOpenAI, providers.KindOpenAICompatible, providers.KindGoogle, providers.KindCustom} {
		c := &upstream.RouteCandidate{Kind: kind, UpstreamModelName: "intern-s2"}
		salinan := untukKandidat(req, c)
		if salinan.Thinking != nil {
			t.Errorf("kind %s: Thinking = %+v, mau nil (adaptive distrip, bawaan upstream berlaku)", kind, salinan.Thinking)
		}
		if salinan.Model != "intern-s2" {
			t.Errorf("kind %s: Model = %q, mau nama upstream kandidat", kind, salinan.Model)
		}
	}
	if req.Thinking == nil || req.Thinking.Type != "adaptive" {
		t.Errorf("permintaan asal berubah: Thinking = %+v, mau tetap {adaptive}", req.Thinking)
	}
}

// (b) Per kandidat: "adaptive" untuk kandidat Anthropic dibawa apa adanya.
func TestUntukKandidatAdaptiveUtuhUntukAnthropic(t *testing.T) {
	req := &providers.ChatRequest{
		Model:    "model-x",
		Thinking: &providers.ThinkingParam{Type: "adaptive"},
	}
	c := &upstream.RouteCandidate{Kind: providers.KindAnthropic, UpstreamModelName: "claude-sonnet-4-5"}
	salinan := untukKandidat(req, c)
	if salinan.Thinking == nil || salinan.Thinking.Type != "adaptive" {
		t.Errorf("Thinking = %+v, mau {adaptive} utuh untuk kandidat Anthropic", salinan.Thinking)
	}
}

// (c) Regresi: "enabled"/"disabled" untuk kandidat non-Anthropic TIDAK
// diubah untukKandidat — perilaku lama dipertahankan persis (adapter
// non-Anthropic memang sudah mengabaikannya sejak sebelum B-096).
func TestUntukKandidatEnabledDisabledNonAnthropicTidakBerubah(t *testing.T) {
	for _, tipe := range []string{"enabled", "disabled"} {
		req := &providers.ChatRequest{
			Model:    "intern-s2",
			Thinking: &providers.ThinkingParam{Type: tipe, BudgetTokens: 2048},
		}
		c := &upstream.RouteCandidate{Kind: providers.KindOpenAICompatible, UpstreamModelName: "intern-s2"}
		salinan := untukKandidat(req, c)
		if salinan.Thinking == nil || salinan.Thinking.Type != tipe || salinan.Thinking.BudgetTokens != 2048 {
			t.Errorf("tipe %s: Thinking = %+v, mau utuh seperti perilaku sebelum B-096", tipe, salinan.Thinking)
		}
	}
}
