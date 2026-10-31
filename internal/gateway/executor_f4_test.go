package gateway

import (
	"context"
	"errors"
	"io"
	"sync"
	"testing"
	"time"

	"github.com/NexGen-X/Route-X/internal/database/repo/upstream"
	"github.com/NexGen-X/Route-X/internal/providers"
	"github.com/NexGen-X/Route-X/internal/router"
)

// TestPlanFromRuleMembawaKonfigurasiBreaker memverifikasi kolom
// routing_rules (failure_threshold/open_duration/half_open_probes) kini
// benar-benar sampai ke Plan sebagai BreakerCfg (audit F4) — sebelumnya
// kolom-kolom itu tidak pernah dibaca jalur request.
func TestPlanFromRuleMembawaKonfigurasiBreaker(t *testing.T) {
	rule := &router.Rule{
		FailureThreshold: 9,
		OpenDuration:     45 * time.Second,
		HalfOpenProbes:   3,
	}
	p := PlanFromRule(rule, "m", []*upstream.RouteCandidate{kandidat("a")})
	if p.BreakerCfg.MinSamples != 9 {
		t.Errorf("MinSamples = %d, mau 9 (dari failure_threshold)", p.BreakerCfg.MinSamples)
	}
	if p.BreakerCfg.OpenDuration != 45*time.Second {
		t.Errorf("OpenDuration = %v, mau 45s", p.BreakerCfg.OpenDuration)
	}
	if p.BreakerCfg.HalfOpenProbes != 3 {
		t.Errorf("HalfOpenProbes = %d, mau 3", p.BreakerCfg.HalfOpenProbes)
	}

	pNihil := PlanFromRule(nil, "m", []*upstream.RouteCandidate{kandidat("a")})
	if pNihil.BreakerCfg != (BreakerConfig{}) {
		t.Errorf("BreakerCfg tanpa aturan = %+v, mau nol (bawaan global)", pNihil.BreakerCfg)
	}
}

// penjagaTerkonfigurasi mencatat panggilan jalur WithConfig secara terpisah.
type penjagaTerkonfigurasi struct {
	penjagaUji
	muCfg     sync.Mutex
	cfgAllow  []BreakerConfig
	cfgRecord []string
}

func (p *penjagaTerkonfigurasi) AllowWithConfig(_ context.Context, providerID, _ string, cfg BreakerConfig) (bool, State, error) {
	p.muCfg.Lock()
	defer p.muCfg.Unlock()
	p.cfgAllow = append(p.cfgAllow, cfg)
	return true, StateClosed, nil
}

func (p *penjagaTerkonfigurasi) RecordWithConfig(_ context.Context, providerID, _ string, sukses bool, cfg BreakerConfig) error {
	p.muCfg.Lock()
	defer p.muCfg.Unlock()
	p.cfgRecord = append(p.cfgRecord, providerID)
	return nil
}

// TestExecuteMemakaiKonfigurasiBreakerAturan memverifikasi executor
// memakai jalur terkonfigurasi bila Plan membawa BreakerCfg.
func TestExecuteMemakaiKonfigurasiBreakerAturan(t *testing.T) {
	g := &penjagaTerkonfigurasi{penjagaUji: penjagaUji{tolak: map[string]bool{}}}
	e, _ := executorUji(g)

	plan := rencana("m", 1, 0, kandidat("a"))
	plan.BreakerCfg = BreakerConfig{MinSamples: 7, OpenDuration: 10 * time.Second, HalfOpenProbes: 2}

	out := Execute(context.Background(), e, plan,
		func(_ context.Context, _ *upstream.RouteCandidate) (string, error) { return "ok", nil })
	if out.Err != nil {
		t.Fatalf("Err = %v", out.Err)
	}
	if len(g.cfgAllow) != 1 || g.cfgAllow[0].MinSamples != 7 {
		t.Errorf("AllowWithConfig terpanggil %v kali dengan cfg %v, mau 1x MinSamples=7", len(g.cfgAllow), g.cfgAllow)
	}
	if len(g.cfgRecord) != 1 {
		t.Errorf("RecordWithConfig terpanggil %d kali, mau 1", len(g.cfgRecord))
	}
	if len(g.catatan()) != 0 {
		t.Errorf("jalur Record biasa ikut terpanggil: %v", g.catatan())
	}
}

// TestExecuteTidakMencatatPenolakanLokalKeBreaker memverifikasi error
// Local (keputusan limiter gateway sendiri) tidak mencemari pemutus arus
// (audit F4).
func TestExecuteTidakMencatatPenolakanLokalKeBreaker(t *testing.T) {
	g := penjaga()
	e, _ := executorUji(g)

	out := Execute(context.Background(), e, rencana("m", 1, 0, kandidat("a")),
		func(_ context.Context, c *upstream.RouteCandidate) (string, error) {
			return "", &providers.Error{
				Kind:  providers.ErrKindRateLimit,
				Local: true,
			}
		})
	if out.Err == nil {
		t.Fatal("Err nil, mau kegagalan batas lokal")
	}
	if len(g.catatan()) != 0 {
		t.Errorf("penolakan lokal tercatat ke breaker: %v, mau kosong", g.catatan())
	}
}

// TestExecuteMaksRetriesProviderMembatasiPercobaan memverifikasi kolom
// providers.max_retries kini membatasi percobaan per kandidat (audit F4).
func TestExecuteMaksRetriesProviderMembatasiPercobaan(t *testing.T) {
	g := penjaga()
	e, _ := executorUji(g)

	c := kandidat("a", func(c *upstream.RouteCandidate) { c.MaxRetries = 0 })
	panggilan := 0
	out := Execute(context.Background(), e, rencana("m", 3, 0, c),
		func(_ context.Context, _ *upstream.RouteCandidate) (string, error) {
			panggilan++
			return "", providers.Newf(providers.ErrKindOverloaded, "a", "penuh")
		})
	if out.Err == nil {
		t.Fatal("Err nil, mau kegagalan")
	}
	if panggilan != 1 {
		t.Errorf("op dipanggil %d kali, mau 1 (max_retries=0 walau MaxAttempts=3)", panggilan)
	}
}

// TestExecuteBatasKonkurensiProviderMelewatiKandidat memverifikasi slot
// max_concurrent yang penuh membuat kandidat dilewati dengan error Local
// dan kandidat berikutnya dicoba (audit F4).
func TestExecuteBatasKonkurensiProviderMelewatiKandidat(t *testing.T) {
	g := penjaga()
	e, _ := executorUji(g)

	satu := int(1)
	a := kandidat("a", func(c *upstream.RouteCandidate) { c.MaxConcurrent = &satu })
	b := kandidat("b")

	masuk := make(chan struct{})
	lepas := make(chan struct{})
	var sekali sync.Once
	go func() {
		Execute(context.Background(), e, rencana("m", 1, 0, a),
			func(_ context.Context, _ *upstream.RouteCandidate) (string, error) {
				sekali.Do(func() { close(masuk) })
				<-lepas
				return "ok", nil
			})
	}()
	<-masuk // slot a terisi

	out := Execute(context.Background(), e, rencana("m", 1, 0, a, b),
		func(_ context.Context, c *upstream.RouteCandidate) (string, error) {
			if c.ProviderName == "a" {
				t.Error("kandidat a dipanggil padahal slotnya penuh")
			}
			return "dari-b", nil
		})
	close(lepas)

	if out.Err != nil {
		t.Fatalf("Err = %v", out.Err)
	}
	if out.Value != "dari-b" {
		t.Errorf("Value = %v, mau dari-b", out.Value)
	}
	// Yang tercatat hanya keberhasilan b; kandidat a yang dilewati slot
	// penuh tidak boleh menghasilkan catatan apa pun (penolakan lokal).
	cat := g.catatan()
	if len(cat) != 1 || cat[0] != "prov-b:true" {
		t.Errorf("catatan breaker = %v, mau tepat [prov-b:true]", cat)
	}
}

// aliranHasil adalah Stream uji dengan urutan hasil Recv yang ditentukan.
type aliranHasil struct {
	hasil []error
	idx   int
}

func (a *aliranHasil) Recv() (*providers.StreamEvent, error) {
	if a.idx >= len(a.hasil) {
		return nil, io.EOF
	}
	err := a.hasil[a.idx]
	a.idx++
	if err != nil {
		return nil, err
	}
	return &providers.StreamEvent{Delta: "x"}, nil
}

func (a *aliranHasil) Close() error { return nil }

// TestAliranPencatatMelaporkanHasilAkhir memverifikasi pembungkus aliran
// streaming melaporkan EOF sebagai sukses dan error tengah-aliran sebagai
// gagal — tepat satu kali (audit F4).
func TestAliranPencatatMelaporkanHasilAkhir(t *testing.T) {
	t.Run("eof sukses", func(t *testing.T) {
		var hasil []bool
		a := &aliranPencatat{
			Stream: &aliranHasil{hasil: []error{nil}},
			ctx:    context.Background(),
			catat:  func(s bool) { hasil = append(hasil, s) },
		}
		_, _ = a.Recv()
		_, err := a.Recv()
		if !errors.Is(err, io.EOF) {
			t.Fatalf("Recv kedua = %v, mau EOF", err)
		}
		_ = a.Close()
		if len(hasil) != 1 || !hasil[0] {
			t.Errorf("catatan = %v, mau [true]", hasil)
		}
	})

	t.Run("putus di tengah gagal", func(t *testing.T) {
		var hasil []bool
		a := &aliranPencatat{
			Stream: &aliranHasil{hasil: []error{nil, errors.New("putus")}},
			ctx:    context.Background(),
			catat:  func(s bool) { hasil = append(hasil, s) },
		}
		_, _ = a.Recv()
		_, _ = a.Recv()
		_ = a.Close()
		if len(hasil) != 1 || hasil[0] {
			t.Errorf("catatan = %v, mau [false]", hasil)
		}
	})
}
