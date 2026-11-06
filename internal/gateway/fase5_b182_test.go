package gateway

import (
	"testing"

	"github.com/NexGen-X/Route-X/internal/database/repo/upstream"
	"github.com/NexGen-X/Route-X/internal/router"
)

// Test pengunci B-182 (Fase 5 kampanye tuntas-logika): sidik rute
// response cache buta terhadap isi combo.
//
// sidikRuteCache sebelumnya hanya meng-hash ID + Strategy aturan dan
// decision.Candidates. Untuk aturan combo, decision.Candidates
// hanyalah kandidat jangkar model yang diminta — himpunan yang
// benar-benar melayani adalah gabungan lintas model di
// plan.Candidates, dan definisi Pipeline (strategi, attempts, daftar
// model berurutan) tidak tertangkap sama sekali. Akibatnya mengedit
// pipeline combo tidak mengubah kunci cache: jawaban tersimpan di
// bawah pipeline lama tetap tersaji sampai TTL.

func persiapanComboB182(models []string, attempts int, strategi router.Strategy, layan []*upstream.RouteCandidate) *persiapan {
	return &persiapan{
		decision: router.Decision{
			Rule: &router.Rule{
				ID:       "rule-combo-1",
				Strategy: router.StrategyPriority,
				Pipeline: &router.ComboPipeline{
					Strategy: strategi,
					Attempts: attempts,
					Models:   models,
				},
			},
			// Jangkar sengaja selalu sama: satu-satunya pembeda antar
			// kasus uji adalah pipeline dan himpunan layannya.
			Candidates: []*upstream.RouteCandidate{kandidat("a")},
		},
		plan: Plan{Candidates: layan},
	}
}

func TestB182SidikRutePekaTerhadapIsiCombo(t *testing.T) {
	layanA := []*upstream.RouteCandidate{
		kandidat("a"),
		kandidat("b"),
	}
	dasar := persiapanComboB182([]string{"model-a", "model-b"}, 4, router.StrategyPriority, layanA)
	sidikDasar := sidikRuteCache(dasar)

	t.Run("stabil untuk masukan identik", func(t *testing.T) {
		sama := persiapanComboB182([]string{"model-a", "model-b"}, 4, router.StrategyPriority, layanA)
		if sidikRuteCache(sama) != sidikDasar {
			t.Error("sidik berubah untuk masukan identik — cache tidak akan pernah hit secara stabil")
		}
	})

	kasus := []struct {
		nama string
		pr   *persiapan
	}{
		{"model pipeline diganti", persiapanComboB182(
			[]string{"model-a", "model-c"}, 4, router.StrategyPriority, layanA)},
		{"urutan model pipeline dibalik", persiapanComboB182(
			[]string{"model-b", "model-a"}, 4, router.StrategyPriority, layanA)},
		{"attempts pipeline berubah", persiapanComboB182(
			[]string{"model-a", "model-b"}, 2, router.StrategyPriority, layanA)},
		{"strategi pipeline berubah", persiapanComboB182(
			[]string{"model-a", "model-b"}, 4, router.StrategyWeighted, layanA)},
		{"himpunan layan plan berubah", persiapanComboB182(
			[]string{"model-a", "model-b"}, 4, router.StrategyPriority,
			[]*upstream.RouteCandidate{kandidat("a")})},
	}
	for _, k := range kasus {
		t.Run(k.nama, func(t *testing.T) {
			if sidikRuteCache(k.pr) == sidikDasar {
				t.Errorf("B-182: sidik tidak berubah saat %s — jawaban cache dari susunan lama akan tetap tersaji", k.nama)
			}
		})
	}
}

// Kontrol: aturan non-combo tidak terpengaruh plan — plan-nya
// deterministik dari decision, dan sidiknya tetap ditentukan ID,
// strategi, dan kandidat keputusan seperti sebelumnya.
func TestB182SidikNonComboTidakBerubahOlehPlan(t *testing.T) {
	susun := func(layan []*upstream.RouteCandidate) *persiapan {
		return &persiapan{
			decision: router.Decision{
				Rule:       &router.Rule{ID: "rule-biasa-1", Strategy: router.StrategyPriority},
				Candidates: []*upstream.RouteCandidate{kandidat("a")},
			},
			plan: Plan{Candidates: layan},
		}
	}
	a := sidikRuteCache(susun([]*upstream.RouteCandidate{kandidat("a")}))
	b := sidikRuteCache(susun(nil))
	if a != b || a == "" {
		t.Errorf("sidik non-combo berubah oleh plan (%q vs %q) — cakupan B-182 hanya combo", a, b)
	}
}
