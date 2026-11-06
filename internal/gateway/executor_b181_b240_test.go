package gateway

import (
	"context"
	"fmt"
	"sync"
	"testing"

	"github.com/NexGen-X/Route-X/internal/database/repo/upstream"
	"github.com/NexGen-X/Route-X/internal/providers"
)

// Test pengunci B-181 dan B-240 (Fase 4 kampanye tuntas-logika).
//
// B-181: kunci pemutus arus untuk plan combo harus memakai model
// kanonik PER KANDIDAT, bukan model jangkar plan untuk semua anggota.
// B-240: penolakan filter konten (ErrKindContentFilter) adalah 4xx
// sisi klien seperti ErrKindInvalidRequest — ia tidak boleh tercatat
// sebagai kegagalan pemutus arus.

// penjagaModel adalah CircuitGuard uji yang merekam kunci
// (provider, model) persis seperti yang diterima dari executor.
type penjagaModel struct {
	mu         sync.Mutex
	tolakKunci map[string]bool
	izin       []string
	catatan    []string
}

func penjagaModelBaru() *penjagaModel {
	return &penjagaModel{tolakKunci: map[string]bool{}}
}

func (p *penjagaModel) Allow(_ context.Context, providerID, model string) (bool, State, error) {
	p.mu.Lock()
	defer p.mu.Unlock()
	kunci := providerID + "|" + model
	p.izin = append(p.izin, kunci)
	if p.tolakKunci[kunci] {
		return false, StateOpen, nil
	}
	return true, StateClosed, nil
}

func (p *penjagaModel) Record(_ context.Context, providerID, model string, sukses bool) error {
	p.mu.Lock()
	defer p.mu.Unlock()
	p.catatan = append(p.catatan, fmt.Sprintf("%s|%s|%t", providerID, model, sukses))
	return nil
}

func rencanaCombo() Plan {
	return Plan{
		Model:       "model-a",
		ModelNames:  map[string]string{"uuid-a": "model-a", "uuid-b": "model-b"},
		MaxAttempts: 3,
		BackoffBase: 0,
	}
}

// kandidatSekali membuat kandidat yang jatahnya tepat satu percobaan
// (MaxRetries 0), supaya catatan breaker per kandidat dalam test
// combo berisi tepat satu entri per kandidat.
func kandidatSekali(nama, modelID string) *upstream.RouteCandidate {
	return kandidat(nama, func(c *upstream.RouteCandidate) {
		c.ModelID = modelID
		c.MaxRetries = 0
	})
}

func TestB181BreakerMemakaiModelPerKandidat(t *testing.T) {
	g := penjagaModelBaru()
	e, _ := executorUji(g)

	kandB := kandidatSekali("pb", "uuid-b")
	kandA := kandidatSekali("pa", "uuid-a")

	plan := rencanaCombo()
	plan.Candidates = []*upstream.RouteCandidate{kandB, kandA}
	out := Execute(context.Background(), e, plan,
		func(_ context.Context, c *upstream.RouteCandidate) (string, error) {
			if c == kandB {
				return "", providers.Newf(providers.ErrKindServer, c.ProviderName, "upstream 500")
			}
			return "ok", nil
		})
	if out.Err != nil {
		t.Fatalf("Err = %v, mau sukses lewat kandidat model-a", out.Err)
	}
	mau := []string{"prov-pb|model-b|false", "prov-pa|model-a|true"}
	if len(g.catatan) != len(mau) {
		t.Fatalf("catatan breaker = %v, mau %v", g.catatan, mau)
	}
	for i := range mau {
		if g.catatan[i] != mau[i] {
			t.Errorf("catatan[%d] = %q, mau %q — kegagalan anggota model-b wajib tercatat ke model-b, bukan jangkar model-a", i, g.catatan[i], mau[i])
		}
	}
}

func TestB181IzinBreakerMenolakModelAnggotaSendiri(t *testing.T) {
	g := penjagaModelBaru()
	// Breaker (prov-pb, model-b) terbuka; (prov-pb, model-a) tidak
	// pernah ada. Pra-fix executor menanyakan kunci jangkar untuk
	// kandidat B sehingga ia tetap dicoba.
	g.tolakKunci["prov-pb|model-b"] = true
	e, _ := executorUji(g)

	kandB := kandidat("pb", func(c *upstream.RouteCandidate) { c.ModelID = "uuid-b" })
	kandA := kandidat("pa", func(c *upstream.RouteCandidate) { c.ModelID = "uuid-a" })

	plan := rencanaCombo()
	plan.Candidates = []*upstream.RouteCandidate{kandB, kandA}
	dipanggilB := false
	out := Execute(context.Background(), e, plan,
		func(_ context.Context, c *upstream.RouteCandidate) (string, error) {
			if c == kandB {
				dipanggilB = true
			}
			return "ok", nil
		})
	if out.Err != nil {
		t.Fatalf("Err = %v, mau sukses lewat kandidat model-a", out.Err)
	}
	if dipanggilB {
		t.Errorf("kandidat model-b tetap dicoba padahal breaker (provider, model-b) terbuka — izin ditanyakan dengan kunci jangkar")
	}
}

func TestB181KandidatTanpaPemetaanJatuhKeModelPlan(t *testing.T) {
	g := penjagaModelBaru()
	e, _ := executorUji(g)

	asing := kandidat("px", func(c *upstream.RouteCandidate) { c.ModelID = "uuid-tak-dikenal" })
	plan := rencanaCombo()
	plan.Candidates = []*upstream.RouteCandidate{asing}
	out := Execute(context.Background(), e, plan,
		func(_ context.Context, c *upstream.RouteCandidate) (string, error) {
			return "ok", nil
		})
	if out.Err != nil {
		t.Fatalf("Err = %v, mau sukses", out.Err)
	}
	if len(g.catatan) != 1 || g.catatan[0] != "prov-px|model-a|true" {
		t.Errorf("catatan = %v, mau kunci jangkar model-a untuk kandidat tanpa pemetaan nama", g.catatan)
	}
}

func TestB240ContentFilterTidakMenggerusBreaker(t *testing.T) {
	g := penjagaModelBaru()
	e, _ := executorUji(g)

	out := Execute(context.Background(), e,
		rencana("model-a", 3, 0, kandidat("pa")),
		func(_ context.Context, c *upstream.RouteCandidate) (string, error) {
			return "", providers.Newf(providers.ErrKindContentFilter, c.ProviderName, "ditolak kebijakan konten upstream")
		})
	if out.Err == nil || out.Err.Kind != providers.ErrKindContentFilter {
		t.Fatalf("Err = %v, mau content_filter", out.Err)
	}
	if len(g.catatan) != 0 {
		t.Errorf("B-240: penolakan filter konten tercatat ke breaker %v, mau tanpa catatan kegagalan", g.catatan)
	}
}

func TestB240KegagalanServerTetapTercatat(t *testing.T) {
	g := penjagaModelBaru()
	e, _ := executorUji(g)

	out := Execute(context.Background(), e,
		rencana("model-a", 1, 0, kandidat("pa")),
		func(_ context.Context, c *upstream.RouteCandidate) (string, error) {
			return "", providers.Newf(providers.ErrKindServer, c.ProviderName, "upstream 500")
		})
	if out.Err == nil {
		t.Fatal("Err = nil, mau kegagalan server")
	}
	if len(g.catatan) != 1 || g.catatan[0] != "prov-pa|model-a|false" {
		t.Errorf("catatan = %v, mau satu kegagalan server tercatat (pengecualian B-240 wajib spesifik per jenis)", g.catatan)
	}
}
