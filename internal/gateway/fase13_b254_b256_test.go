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

// B-254: refund penolakan pra-upstream sebelumnya hanya memulihkan cakupan
// resolver (key/pengguna/IP/global) — cakupan MODEL yang dinaikkan
// PeriksaBatasModel di kebijakan tidak pernah dibatalkan. Pola W2-D3:
// batas model RPM 1 + anggaran global habis. Permintaan pertama lolos
// batas model (penghitung naik 1/1) lalu ditolak anggaran (402); permintaan
// kedua harus ditolak ANGGARAN lagi (402). Pra-fix ia tertahan batas
// model (429) — kuota model permintaan yang tidak pernah dilayani bocor
// permanen sampai jendela berganti.
func TestRefundJugaMemulihkanKuotaModelB254(t *testing.T) {
	src := &sumberKebijakan{
		limits: []*policy.RateLimit{{
			Scope: policy.ScopeModel, ScopeID: modelUji().ID, RequestsPerMinute: pointerG(1), Enabled: true,
		}},
		budgets: []*policy.Budget{{
			ID: "b1", Name: "anggaran bulanan", Scope: policy.ScopeGlobal, Period: policy.PeriodMonthly,
			LimitUSD: upstream.MustParseUSD("10"), SpentUSD: upstream.MustParseUSD("10"),
			ActionOnExceed: policy.ActionBlock, AlertThresholdPct: 80, Enabled: true,
		}},
	}
	s := susun(t, providerSukses(), func(_ *susunan, d *HandlersDeps) { d.Guard = guardUji(t, src) })

	for i := 1; i <= 2; i++ {
		w := s.panggil(t, http.MethodPost, "/chat/completions", bodyChatMinimal, ctxDenganKey("k-b254", "u-b254"))
		if w.Code != http.StatusPaymentRequired {
			t.Fatalf("permintaan %d status = %d, mau 402 — kuota model permintaan yang ditolak anggaran harus dikembalikan (body: %s)",
				i, w.Code, w.Body.String())
		}
	}
}

// B-256: memo keputusan batas sebelumnya ditanam per Execute, bukan per
// permintaan — Execute kedua dalam permintaan yang sama (fallback tier
// combo) memeriksa cakupan provider dari awal dan MEMAKAN kuotanya lagi.
// Di sini SATU provider melayani model utama dan model tier-2 dengan
// kebijakan provider RPM 1: Execute utama menaikkan penghitung (1/1)
// lalu providernya gagal; Execute tier-2 atas provider yang sama hanya
// boleh lolos lewat keputusan yang dimemoisasi. Pra-fix pemeriksaan
// segar menemukan penghitung penuh, kandidat dilewati sebagai
// rate-limit, dan permintaan gagal walau model tier-2 sehat.
func TestMemoBatasSatuUntukSeluruhPermintaanB256(t *testing.T) {
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
		ID:           "r-fase13-b256",
		Name:         "Aturan Tier B-256",
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

	src := &sumberKebijakan{limits: []*policy.RateLimit{{
		Scope: policy.ScopeProvider, ScopeID: "prov-utama", RequestsPerMinute: pointerG(1), Enabled: true,
	}}}
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

	r := httptest.NewRequest(http.MethodPost, "/chat/completions", strings.NewReader(bodyComboChat))
	r = r.WithContext(ctxDenganKey("k-b256", "u-b256"))
	w := httptest.NewRecorder()
	h.Routes().ServeHTTP(w, r)

	if w.Code != http.StatusOK {
		t.Fatalf("status = %d; ingin 200 — Execute tier-2 tidak boleh memakan kuota provider yang sama lagi (urutan: %v; body: %s)",
			w.Code, factory.urutan(), w.Body.String())
	}
	if !strings.Contains(w.Body.String(), "jawaban sukses dari tier 2") {
		t.Errorf("respons tidak memuat jawaban tier 2: %s", w.Body.String())
	}
}
