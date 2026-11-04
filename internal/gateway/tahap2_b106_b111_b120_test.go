package gateway

import (
	"context"
	"net/http"
	"testing"

	"github.com/NexGen-X/Route-X/internal/database/repo/upstream"
	"github.com/NexGen-X/Route-X/internal/providers"
	"github.com/NexGen-X/Route-X/internal/router"
)

// --- B-106: estimasi biaya memakai cuplikan harga terkini -------------------

// penaksirDuaJalur memisahkan kedua jalur harga secara ekstrem: Cost
// (cuplikan beku) tidak tahu harga apa pun, CostCurrent (cuplikan terkini)
// tahu. Estimasi reservasi WAJIB memakai jalur terkini.
type penaksirDuaJalur struct {
	statis   upstream.USD
	segar    upstream.USD
	statisOK bool
	segarOK  bool
}

func (p *penaksirDuaJalur) Cost(_ string, _ router.TokenEstimate) (upstream.USD, bool) {
	return p.statis, p.statisOK
}

func (p *penaksirDuaJalur) CostCurrent(_ context.Context, _ string, _ router.TokenEstimate) (upstream.USD, bool) {
	return p.segar, p.segarOK
}

// penaksirStatisSaja hanya punya Cost (kontrak router.CostSource lama).
type penaksirStatisSaja struct {
	nilai upstream.USD
	ok    bool
}

func (p *penaksirStatisSaja) Cost(_ string, _ router.TokenEstimate) (upstream.USD, bool) {
	return p.nilai, p.ok
}

// modelDanPemetaan adalah ModelResolver yang juga bisa mendaftar pemetaan
// provider-model, seperti *upstream.ModelRepo di produksi.
type modelDanPemetaan struct {
	model    *upstream.Model
	pemetaan []*upstream.ProviderModel
}

func (m *modelDanPemetaan) Resolve(_ context.Context, _ string) (*upstream.Model, error) {
	return m.model, nil
}

func (m *modelDanPemetaan) ListProviderModels(_ context.Context, _ upstream.ProviderModelFilter) ([]*upstream.ProviderModel, error) {
	return m.pemetaan, nil
}

func TestEstimasiBiayaMemakaiHargaTerkini(t *testing.T) {
	model := modelUji()
	h := &Handlers{
		models: &modelDanPemetaan{
			model:    model,
			pemetaan: []*upstream.ProviderModel{{ID: "pm-1", ModelID: model.ID}},
		},
		penaksir: &penaksirDuaJalur{
			statisOK: false, // cuplikan beku sejak boot: tidak tahu harga
			segar:    upstream.MustParseUSD("0.25"), segarOK: true,
		},
		logger: loggerSenyap(),
	}
	req := &providers.ChatRequest{Model: model.ModelID, Messages: []providers.Message{{Role: "user", Content: "halo"}}}
	if got := h.estimasiBiaya(context.Background(), model, req); got != upstream.MustParseUSD("0.25") {
		t.Errorf("estimasiBiaya = %s, mau 0.25 dari jalur harga terkini (B-106)", got)
	}
}

func TestEstimasiBiayaJatuhKeCostBiasa(t *testing.T) {
	model := modelUji()
	h := &Handlers{
		models: &modelDanPemetaan{
			model:    model,
			pemetaan: []*upstream.ProviderModel{{ID: "pm-1", ModelID: model.ID}},
		},
		penaksir: &penaksirStatisSaja{nilai: upstream.MustParseUSD("0.5"), ok: true},
		logger:   loggerSenyap(),
	}
	req := &providers.ChatRequest{Model: model.ModelID, Messages: []providers.Message{{Role: "user", Content: "halo"}}}
	if got := h.estimasiBiaya(context.Background(), model, req); got != upstream.MustParseUSD("0.5") {
		t.Errorf("estimasiBiaya = %s, mau 0.5 dari Cost biasa", got)
	}
}

// --- B-120: kandidat kosong karena kesehatan, bukan konfigurasi ------------

// sumberKandidatKesehatan menjawab kosong untuk query biasa, tetapi
// menghasilkan kandidat bila IncludeUnhealthy — persis keadaan provider
// yang disisihkan cuplikan kesehatan segar.
type sumberKandidatKesehatan struct {
	semua []*upstream.RouteCandidate
}

func (s *sumberKandidatKesehatan) RouteCandidates(_ context.Context, q upstream.RouteQuery) ([]*upstream.RouteCandidate, error) {
	if q.IncludeUnhealthy {
		return s.semua, nil
	}
	return nil, nil
}

func TestSemuaProviderTakSehatJadi503ProviderUnhealthy(t *testing.T) {
	s := susun(t, providerSukses(), func(_ *susunan, d *HandlersDeps) {
		d.Candidates = &sumberKandidatKesehatan{
			semua: []*upstream.RouteCandidate{kandidatRute("utama", "gpt-4o-mini-2024")},
		}
	})
	w := s.panggil(t, http.MethodPost, "/chat/completions", bodyChatMinimal, nil)
	if w.Code != http.StatusServiceUnavailable {
		t.Fatalf("status = %d, mau 503 (body: %s)", w.Code, w.Body.String())
	}
	if got := kodeGalat(galat(t, w)); got != "provider_unhealthy" {
		t.Errorf("code = %q, mau provider_unhealthy — bukan no_provider_for_model (B-120)", got)
	}
}

// --- B-111: tersisih aturan rute bukan context_length_exceeded --------------

func TestKandidatTersisihAturanRuteJadi503BukanKonteks(t *testing.T) {
	model := modelUji()
	rule := &upstream.RoutingRule{
		ID: "aturan-lain", Name: "hanya-provider-lain", Enabled: true,
		Strategy:     string(router.StrategyPriority),
		MatchModelID: &model.ID,
		Providers: []upstream.RuleProvider{
			{ProviderID: "prov-yang-lain", Position: 1},
		},
	}
	engine := router.NewEngine(&sumberAturanTiruan{rules: []*upstream.RoutingRule{rule}}, nil, loggerSenyap())

	s := susun(t, providerSukses(), func(_ *susunan, d *HandlersDeps) {
		d.Engine = engine
	})
	w := s.panggil(t, http.MethodPost, "/chat/completions", bodyChatMinimal, nil)
	if w.Code != http.StatusServiceUnavailable {
		t.Fatalf("status = %d, mau 503 (body: %s)", w.Code, w.Body.String())
	}
	if got := kodeGalat(galat(t, w)); got != "no_available_provider" {
		t.Errorf("code = %q, mau no_available_provider — bukan context_length_exceeded (B-111)", got)
	}
}
