package gateway

import (
	"context"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/NexGen-X/Route-X/internal/database/repo/policy"
	"github.com/NexGen-X/Route-X/internal/database/repo/upstream"
	"github.com/NexGen-X/Route-X/internal/providers"
	"github.com/NexGen-X/Route-X/internal/router"
)

// B-294: refund B-254 tidak menjangkau jalur tier fallback combo.
// siapkanTierFallback menaikkan penghitung cakupan model TIER lewat
// PeriksaBatasModel, tetapi tidak pernah menandainya — penolakan
// gerbang SESUDAHNYA (anggaran/token) atau tier yang dilewati
// membakar slot model tier secara permanen sampai jendela berganti.
//
// Pola: model tier-2 ber-RPM 1 + anggaran cakupan model tier-2 habis.
// Permintaan pertama: Execute utama gagal → persiapan tier menaikkan
// batas model tier (1/1) lalu ditolak anggaran (402). Permintaan kedua
// harus ditolak ANGGARAN lagi (402); pra-fix ia tertahan batas model
// tier (429).
func TestB294RefundMenjangkauTierFallback(t *testing.T) {
	m1 := &upstream.Model{ID: "m-t1", ModelID: "tier1-model", Enabled: true}
	m2 := &upstream.Model{ID: "m-t2", ModelID: "tier2-model", Enabled: true}
	models := &petaModel{models: map[string]*upstream.Model{
		"tier1-model": m1, "tier2-model": m2,
	}}
	cands := &petaKandidat{cands: map[string][]*upstream.RouteCandidate{
		"m-t1": {kandidatRute("utama", "upstream-t1")},
		"m-t2": {kandidatRute("utama", "upstream-t2")},
	}}
	rule := &upstream.RoutingRule{
		ID:           "r-fase15-b294",
		Name:         "Aturan Tier B-294",
		MatchModelID: ptrString("m-t1"),
		Strategy:     "priority",
		MaxAttempts:  1,
		Enabled:      true,
		Description:  ptrString(`[combo:pipeline=[{"tier":2,"model":"tier2-model"}]]`),
	}
	engine := router.NewEngine(&sumberAturanTiruan{rules: []*upstream.RoutingRule{rule}}, nil, loggerSenyap())

	tiruan := &providerTiruan{
		nama: "utama", kind: providers.KindOpenAI,
		chat: func(_ context.Context, req *providers.ChatRequest) (*providers.ChatResponse, error) {
			if req.Model == "upstream-t1" {
				return nil, providers.Newf(providers.ErrKindServer, "utama", "error 500 dari tier 1")
			}
			return chatSukses("jawaban sukses dari tier 2")(context.Background(), req)
		},
	}
	factory := &pabrikTiruan{perNama: map[string]providers.Provider{"utama": tiruan}}

	src := &sumberKebijakan{
		limits: []*policy.RateLimit{{
			Scope: policy.ScopeModel, ScopeID: "m-t2", RequestsPerMinute: pointerG(1), Enabled: true,
		}},
		budgets: []*policy.Budget{{
			ID: "b294", Name: "anggaran model tier-2", Scope: policy.ScopeModel, ScopeID: "m-t2",
			Period:   policy.PeriodMonthly,
			LimitUSD: upstream.MustParseUSD("10"), SpentUSD: upstream.MustParseUSD("10"),
			ActionOnExceed: policy.ActionBlock, AlertThresholdPct: 80, Enabled: true,
		}},
	}
	guard := guardUji(t, src)

	h, err := NewHandlers(HandlersDeps{
		Models:     models,
		Lister:     &pendaftarModel{},
		Candidates: cands,
		Factory:    factory,
		Engine:     engine,
		Guard:      guard,
		Executor:   NewExecutor(nil, loggerSenyap(), WithSleepFunc(func(context.Context, time.Duration) error { return nil })),
		Logger:     loggerSenyap(),
	})
	if err != nil {
		t.Fatalf("NewHandlers: %v", err)
	}

	for i := 1; i <= 2; i++ {
		r := httptest.NewRequest(http.MethodPost, "/chat/completions", strings.NewReader(bodyComboChat))
		r = r.WithContext(ctxDenganKey("k-b294", "u-b294"))
		w := httptest.NewRecorder()
		h.Routes().ServeHTTP(w, r)
		if w.Code != http.StatusPaymentRequired {
			t.Fatalf("permintaan %d status = %d, mau 402 — slot model tier dari persiapan yang ditolak anggaran harus dikembalikan (body: %s)",
				i, w.Code, w.Body.String())
		}
	}
}
