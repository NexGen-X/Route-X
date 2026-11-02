package gateway

import (
	"context"
	"net/http"
	"testing"

	"github.com/NexGen-X/Route-X/internal/contentfilter"
	"github.com/NexGen-X/Route-X/internal/database/repo/policy"
	"github.com/NexGen-X/Route-X/internal/ratelimit"
)

// sinkWebhookPalsu merekam panggilan Enqueue dari produser event gateway.
type sinkWebhookPalsu struct {
	events   []string
	payloads []map[string]any
}

func (s *sinkWebhookPalsu) Enqueue(_ context.Context, event string, payload any) (int, error) {
	s.events = append(s.events, event)
	if m, ok := payload.(map[string]any); ok {
		s.payloads = append(s.payloads, m)
	}
	return 1, nil
}

func (s *sinkWebhookPalsu) cari(event string) map[string]any {
	for i, e := range s.events {
		if e == event {
			return s.payloads[i]
		}
	}
	return nil
}

// TestGuardContentBlockedMemancarkanEvent membuktikan produser content.blocked
// terpasang (audit3 V-WH-08): penolakan penyaring konten memancarkan event
// dengan identitas aturan, tanpa isi permintaan.
func TestGuardContentBlockedMemancarkanEvent(t *testing.T) {
	src := &sumberKebijakan{filters: []*policy.ContentFilter{
		filterBaris("f1", "tanpa rahasia", policy.FilterBlockedPattern, func(f *policy.ContentFilter) {
			f.Pattern = "rahasia"
			f.PatternType = policy.PatternSubstring
		}),
	}}
	sink := &sinkWebhookPalsu{}
	g := NewGuard(GuardDeps{
		Filters:  contentfilter.NewEngine(src, loggerSenyap()),
		Webhooks: sink,
		Logger:   loggerSenyap(),
	})

	rj := g.SaringPermintaan(context.Background(), contentfilter.Subject{
		Text: "tolong jaga rahasia ini", ModelID: "model-uji",
	})
	if rj == nil || rj.Code != CodeContentFilter {
		t.Fatalf("penolakan = %+v, mau Rejection berkode %s", rj, CodeContentFilter)
	}
	p := sink.cari("content.blocked")
	if p == nil {
		t.Fatalf("event content.blocked tidak dipancarkan; events=%v", sink.events)
	}
	if p["filter"] != "tanpa rahasia" {
		t.Errorf("payload filter = %v, mau nama aturan", p["filter"])
	}

	// Permintaan bersih tidak memancarkan event.
	if rj := g.SaringPermintaan(context.Background(), contentfilter.Subject{
		Text: "halo biasa saja", ModelID: "model-uji",
	}); rj != nil {
		t.Fatalf("permintaan bersih ditolak: %+v", rj)
	}
	if len(sink.events) != 1 {
		t.Errorf("events sesudah permintaan bersih = %v, mau tetap 1 (content.blocked saja)", sink.events)
	}
}

// TestGuardRatelimitExceededMemancarkanEvent membuktikan produser
// ratelimit.exceeded terpasang (audit3 V-WH-08).
func TestGuardRatelimitExceededMemancarkanEvent(t *testing.T) {
	src := &sumberKebijakan{limits: []*policy.RateLimit{{
		Scope: policy.ScopeGlobal, DailyTokenLimit: pointerG(int64(10)), Enabled: true,
	}}}
	sink := &sinkWebhookPalsu{}
	g := NewGuard(GuardDeps{
		Filters:  contentfilter.NewEngine(src, loggerSenyap()),
		Rates:    ratelimit.NewEngine(src, loggerSenyap()),
		Limiter:  limiterUji(t),
		Webhooks: sink,
		Logger:   loggerSenyap(),
	})
	targets := []policy.Target{{Scope: policy.ScopeGlobal}}

	// Habiskan anggaran token, lalu gerbang harus menolak + memancarkan event.
	g.CatatToken(context.Background(), nil, targets, 50)
	rj := g.GerbangToken(context.Background(), nil, targets)
	if rj == nil || rj.Code != CodeRateLimited {
		t.Fatalf("penolakan = %+v, mau Rejection berkode %s", rj, CodeRateLimited)
	}
	if p := sink.cari("ratelimit.exceeded"); p == nil {
		t.Fatalf("event ratelimit.exceeded tidak dipancarkan; events=%v", sink.events)
	}
}

// TestJejakMemancarkanEventPermintaan membuktikan produser request.completed
// dan request.failed terpasang di finalisasi jejak (audit3 V-WH-08).
func TestJejakMemancarkanEventPermintaan(t *testing.T) {
	sink := &sinkWebhookPalsu{}
	s, _ := susunDenganPencatat(t, providerSukses(), func(_ *susunan, d *HandlersDeps) {
		d.Webhooks = sink
	})

	if w := s.panggil(t, http.MethodPost, "/chat/completions", bodyChatMinimal, nil); w.Code != http.StatusOK {
		t.Fatalf("chat sukses status = %d, mau 200 (body: %s)", w.Code, w.Body.String())
	}
	p := sink.cari("request.completed")
	if p == nil {
		t.Fatalf("event request.completed tidak dipancarkan; events=%v", sink.events)
	}
	if p["status_code"] != http.StatusOK {
		t.Errorf("payload status_code = %v, mau 200", p["status_code"])
	}

	// Permintaan gagal (body JSON rusak → 400) memancarkan request.failed.
	if w := s.panggil(t, http.MethodPost, "/chat/completions", "{bukan json", nil); w.Code != http.StatusBadRequest {
		t.Fatalf("chat rusak status = %d, mau 400", w.Code)
	}
	pf := sink.cari("request.failed")
	if pf == nil {
		t.Fatalf("event request.failed tidak dipancarkan; events=%v", sink.events)
	}
	if pf["status_code"] != http.StatusBadRequest {
		t.Errorf("payload status_code gagal = %v, mau 400", pf["status_code"])
	}
}
