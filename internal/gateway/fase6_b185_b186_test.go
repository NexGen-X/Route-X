package gateway

import (
	"context"
	"testing"

	"github.com/NexGen-X/Route-X/internal/database/repo/upstream"
	"github.com/NexGen-X/Route-X/internal/providers"
	"github.com/NexGen-X/Route-X/internal/router"
)

// Test pengunci B-185 dan B-186 (Fase 6 kampanye tuntas-logika).
//
// B-185: strategi yang tercatat pada jejak untuk aturan combo harus
// strategi PIPELINE — yang benar-benar mengatur urutan gabungan — bukan
// strategi aturan yang hanya memilih kandidat jangkar.
// B-186: error lokal milik percobaan lewatan slot-penuh (RateLimit,
// Local) tidak boleh menimpa kegagalan upstream nyata kandidat lain;
// ia hanya menjadi kegagalan akhir bila tak ada kegagalan nyata sama
// sekali (kasus kandidat tunggal slot-penuh).

func TestB185StrategiTercatatComboAdalahStrategiPipeline(t *testing.T) {
	kasus := []struct {
		nama string
		rule *router.Rule
		mau  string
	}{
		{
			nama: "combo memakai strategi pipeline",
			rule: &router.Rule{
				ID: "r-combo", Name: "combo", Strategy: router.StrategyPriority,
				Pipeline: &router.ComboPipeline{
					Strategy: router.StrategyRoundRobin,
					Models:   []string{"model-a", "model-b"},
				},
			},
			mau: "round_robin",
		},
		{
			nama: "aturan biasa memakai strategi aturan",
			rule: &router.Rule{ID: "r-biasa", Name: "biasa", Strategy: router.StrategyWeighted},
			mau:  "weighted",
		},
		{
			nama: "pipeline satu model bukan combo, strategi aturan berlaku",
			rule: &router.Rule{
				ID: "r-tunggal", Name: "tunggal", Strategy: router.StrategyPriority,
				Pipeline: &router.ComboPipeline{
					Strategy: router.StrategyRoundRobin,
					Models:   []string{"model-a"},
				},
			},
			mau: "priority",
		},
		{
			nama: "tanpa aturan memakai bawaan prioritas",
			rule: nil,
			mau:  "priority",
		},
	}
	for _, k := range kasus {
		t.Run(k.nama, func(t *testing.T) {
			j := &jejak{}
			j.pasangKeputusan(router.Decision{Rule: k.rule}, false)
			if j.strategi != k.mau {
				t.Errorf("strategi tercatat = %q, mau %q", j.strategi, k.mau)
			}
		})
	}
}

// kandidatSlotPenuh membuat kandidat yang slot konkurensinya sudah
// dipesan penuh oleh test, sehingga executor melewatinya tanpa pernah
// menghubungi upstream.
func kandidatSlotPenuh(t *testing.T, e *Executor, nama string) *upstream.RouteCandidate {
	t.Helper()
	satu := 1
	c := kandidat(nama, func(c *upstream.RouteCandidate) {
		c.MaxConcurrent = &satu
		c.MaxRetries = 0
	})
	lepas, ok := e.ambilSlot(c)
	if !ok {
		t.Fatalf("pemesanan slot awal untuk %s gagal", nama)
	}
	t.Cleanup(lepas)
	return c
}

func rencanaUjiB186(kandidat ...*upstream.RouteCandidate) Plan {
	return Plan{
		Model:       "model-a",
		MaxAttempts: 3,
		BackoffBase: 0,
		Candidates:  kandidat,
	}
}

func TestB186ErrorSlotPenuhTidakMenimpaErrorUpstreamNyata(t *testing.T) {
	e, _ := executorUji(penjagaModelBaru())

	kandA := kandidat("pa", func(c *upstream.RouteCandidate) { c.MaxRetries = 0 })
	kandB := kandidatSlotPenuh(t, e, "pb")

	plan := rencanaUjiB186(kandA, kandB)
	out := Execute(context.Background(), e, plan,
		func(_ context.Context, c *upstream.RouteCandidate) (string, error) {
			return "", providers.Newf(providers.ErrKindServer, c.ProviderName, "upstream 500")
		})
	if out.Err == nil {
		t.Fatal("Err = nil, mau kegagalan server kandidat A")
	}
	if out.Err.Kind != providers.ErrKindServer {
		t.Errorf("Err.Kind = %q (%q), mau %q — error lokal slot-penuh kandidat terakhir menimpa kegagalan upstream nyata",
			out.Err.Kind, out.Err.Message, providers.ErrKindServer)
	}
}

func TestB186KandidatTunggalSlotPenuhMemakaiErrorLokal(t *testing.T) {
	e, _ := executorUji(penjagaModelBaru())

	kandB := kandidatSlotPenuh(t, e, "pb")

	plan := rencanaUjiB186(kandB)
	out := Execute(context.Background(), e, plan,
		func(_ context.Context, c *upstream.RouteCandidate) (string, error) {
			t.Error("jalankan tidak boleh dipanggil untuk kandidat slot-penuh")
			return "ok", nil
		})
	if out.Err == nil {
		t.Fatal("Err = nil, mau error lokal slot-penuh")
	}
	if out.Err.Kind != providers.ErrKindRateLimit || !out.Err.Local {
		t.Errorf("Err = {%q, Local:%t}, mau {%q, Local:true} — tanpa kegagalan nyata, error lokal slot-penuh adalah jawaban yang benar",
			out.Err.Kind, out.Err.Local, providers.ErrKindRateLimit)
	}
}

func TestB186ErrorNyataSesudahSlotPenuhTetapDilaporkan(t *testing.T) {
	e, _ := executorUji(penjagaModelBaru())

	kandB := kandidatSlotPenuh(t, e, "pb")
	kandA := kandidat("pa", func(c *upstream.RouteCandidate) { c.MaxRetries = 0 })

	plan := rencanaUjiB186(kandB, kandA)
	out := Execute(context.Background(), e, plan,
		func(_ context.Context, c *upstream.RouteCandidate) (string, error) {
			return "", providers.Newf(providers.ErrKindServer, c.ProviderName, "upstream 500")
		})
	if out.Err == nil || out.Err.Kind != providers.ErrKindServer {
		t.Errorf("Err = %v, mau kegagalan server kandidat A", out.Err)
	}
}
