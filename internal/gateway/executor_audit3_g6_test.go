package gateway

import (
	"context"
	"io"
	"sync"
	"testing"

	"github.com/NexGen-X/Route-X/internal/database/repo/upstream"
	"github.com/NexGen-X/Route-X/internal/providers"
)

// Test regresi audit3 grup 6 (B-025, B-026, B-027) untuk executor: satu
// izin probe = satu hit upstream, slot tidak menghanguskan probe, slot
// dipegang selama stream hidup, dan anggaran combo tidak habis oleh
// percobaan lewatan.

// penjagaHalfOpen adalah CircuitGuard yang mensimulasikan satu episode
// half_open: izin pertama adalah probe (kuota 1), dan probe yang gagal
// membuka breaker untuk sisa test.
type penjagaHalfOpen struct {
	mu           sync.Mutex
	probeDipakai bool
	terbuka      bool
	izinDiminta  int
	catatanProbe []bool
}

func (p *penjagaHalfOpen) Allow(_ context.Context, _, _ string) (bool, State, error) {
	p.mu.Lock()
	defer p.mu.Unlock()
	p.izinDiminta++
	if p.terbuka {
		return false, StateOpen, nil
	}
	if p.probeDipakai {
		return false, StateHalfOpen, nil
	}
	p.probeDipakai = true
	return true, StateHalfOpen, nil
}

func (p *penjagaHalfOpen) Record(_ context.Context, _, _ string, sukses bool) error {
	return p.RecordHasil(context.Background(), "", "", sukses, false)
}

func (p *penjagaHalfOpen) RecordHasil(_ context.Context, _, _ string, sukses, probe bool) error {
	p.mu.Lock()
	defer p.mu.Unlock()
	if probe {
		p.catatanProbe = append(p.catatanProbe, sukses)
		if !sukses {
			p.terbuka = true
		}
	}
	return nil
}

func (p *penjagaHalfOpen) RecordHasilWithConfig(ctx context.Context, providerID, model string, sukses, probe bool, _ BreakerConfig) error {
	return p.RecordHasil(ctx, providerID, model, sukses, probe)
}

// TestExecutorProbeHanyaSatuHitUpstream memverifikasi V-BR-02: pada fase
// half_open, satu izin probe menghasilkan TEPAT satu permintaan upstream
// walau MaxAttempts 3 — probe yang gagal membuka lagi breaker, dan
// percobaan berikutnya dalam permintaan yang sama ditolak izinnya, bukan
// menembak upstream dari izin yang sama.
func TestExecutorProbeHanyaSatuHitUpstream(t *testing.T) {
	g := &penjagaHalfOpen{}
	e, _ := executorUji(g)

	hit := 0
	out := Execute(context.Background(), e, rencana("m", 3, 0, kandidat("a")),
		func(_ context.Context, _ *upstream.RouteCandidate) (string, error) {
			hit++
			return "", providers.Newf(providers.ErrKindServer, "a", "upstream masih mati")
		})

	if hit != 1 {
		t.Errorf("hit upstream = %d, mau tepat 1 (satu izin probe, satu percobaan)", hit)
	}
	if out.Err == nil {
		t.Fatal("Err nil, mau kegagalan")
	}
	if len(g.catatanProbe) != 1 || g.catatanProbe[0] {
		t.Errorf("catatan probe = %v, mau tepat [false]", g.catatanProbe)
	}
	// Percobaan kedua tercatat sebagai lewatan pemutus arus, bukan hit.
	if got := out.Percobaan(); got != 1 {
		t.Errorf("Percobaan() = %d, mau 1", got)
	}
}

// aliranTahan adalah Stream uji yang mengirim satu peristiwa lalu menahan
// aliran tetap terbuka sampai Close dipanggil.
type aliranTahan struct {
	terkirim bool
	tutup    chan struct{}
	sekali   sync.Once
}

func aliranTahanBaru() *aliranTahan { return &aliranTahan{tutup: make(chan struct{})} }

func (a *aliranTahan) Recv() (*providers.StreamEvent, error) {
	if !a.terkirim {
		a.terkirim = true
		return &providers.StreamEvent{Delta: "x"}, nil
	}
	<-a.tutup
	return nil, io.EOF
}

func (a *aliranTahan) Close() error {
	a.sekali.Do(func() { close(a.tutup) })
	return nil
}

// TestExecutorSlotTidakMenghanguskanProbe memverifikasi V-BR-07: slot
// penuh membuat kandidat dilewati TANPA meminta izin pemutus arus — izin
// probe tidak terbuang oleh kandidat yang bahkan tidak dihubungi.
func TestExecutorSlotTidakMenghanguskanProbe(t *testing.T) {
	g := &penjagaHalfOpen{}
	e, _ := executorUji(g)

	satu := int(1)
	a := kandidat("a", func(c *upstream.RouteCandidate) { c.MaxConcurrent = &satu })

	// Stream pertama memegang satu-satunya slot a.
	aliran := aliranTahanBaru()
	outStream := ExecuteStream(context.Background(), e, rencana("m", 1, 0, a),
		func(_ context.Context, _ *upstream.RouteCandidate) (providers.Stream, error) {
			return aliran, nil
		})
	if outStream.Err != nil {
		t.Fatalf("ExecuteStream: %v", outStream.Err)
	}
	defer func() { _ = outStream.Value.Close() }()
	if g.izinDiminta != 1 {
		t.Fatalf("izin diminta = %d sesudah stream dibuka, mau 1", g.izinDiminta)
	}

	// Permintaan kedua ke a: slot penuh → dilewati, dan izin TIDAK diminta.
	hit := 0
	out := Execute(context.Background(), e, rencana("m", 1, 0, a),
		func(_ context.Context, _ *upstream.RouteCandidate) (string, error) {
			hit++
			return "tidak boleh sampai", nil
		})
	if hit != 0 {
		t.Errorf("op dipanggil %d kali pada kandidat berslot penuh, mau 0", hit)
	}
	if g.izinDiminta != 1 {
		t.Errorf("izin diminta = %d, mau tetap 1 — slot penuh menghanguskan izin probe", g.izinDiminta)
	}
	if len(out.Attempts) != 1 || out.Attempts[0].SkipReason == "" {
		t.Errorf("Attempts = %+v, mau satu percobaan lewatan", out.Attempts)
	}
}

// TestExecutorSlotDipegangSelamaStreamHidup memverifikasi V-BR-06
// (B-026): selama stream pertama masih hidup, stream kedua ke provider
// ber-max_concurrent 1 dialihkan ke kandidat lain; sesudah stream pertama
// ditutup, provider itu bisa dipakai lagi.
func TestExecutorSlotDipegangSelamaStreamHidup(t *testing.T) {
	g := penjaga()
	e, _ := executorUji(g)

	satu := int(1)
	a := kandidat("a", func(c *upstream.RouteCandidate) { c.MaxConcurrent = &satu })
	b := kandidat("b")

	bukaA := 0
	buka := func(_ context.Context, c *upstream.RouteCandidate) (providers.Stream, error) {
		if c.ProviderName == "a" {
			bukaA++
			return aliranTahanBaru(), nil
		}
		return &aliranHasil{hasil: []error{nil}}, nil
	}

	pertama := ExecuteStream(context.Background(), e, rencana("m", 1, 0, a, b), buka)
	if pertama.Err != nil || pertama.Candidate == nil || pertama.Candidate.ProviderName != "a" {
		t.Fatalf("stream pertama = (%v, %v), mau berhasil via a", pertama.Candidate, pertama.Err)
	}

	// Stream kedua selagi yang pertama hidup: a dilewati (slot), b melayani.
	kedua := ExecuteStream(context.Background(), e, rencana("m", 1, 0, a, b), buka)
	if kedua.Err != nil || kedua.Candidate == nil || kedua.Candidate.ProviderName != "b" {
		t.Fatalf("stream kedua = (%v, %v), mau berhasil via b karena slot a masih dipegang", kedua.Candidate, kedua.Err)
	}
	if bukaA != 1 {
		t.Errorf("a dibuka %d kali, mau 1 — slot dilepas saat stream pertama dibuka", bukaA)
	}
	_ = kedua.Value.Close()

	// Sesudah stream pertama ditutup, slot a bebas lagi.
	if err := pertama.Value.Close(); err != nil {
		t.Fatalf("menutup stream pertama: %v", err)
	}
	ketiga := ExecuteStream(context.Background(), e, rencana("m", 1, 0, a, b), buka)
	if ketiga.Err != nil || ketiga.Candidate == nil || ketiga.Candidate.ProviderName != "a" {
		t.Fatalf("stream ketiga = (%v, %v), mau berhasil via a sesudah slot dilepas", ketiga.Candidate, ketiga.Err)
	}
	_ = ketiga.Value.Close()
}

// TestExecutorAnggaranComboTidakHabisOlehLewatan memverifikasi V-BR-05:
// kandidat yang dilewati (slot penuh atau pemutus arus) tidak mengurangi
// anggaran combo, sehingga kandidat sehat berikutnya tetap dicoba.
func TestExecutorAnggaranComboTidakHabisOlehLewatan(t *testing.T) {
	t.Run("lewatan slot", func(t *testing.T) {
		g := penjaga()
		e, _ := executorUji(g)

		satu := int(1)
		a := kandidat("a", func(c *upstream.RouteCandidate) { c.MaxConcurrent = &satu })
		b := kandidat("b")

		// Pegang slot a dengan stream yang masih hidup.
		pegang := ExecuteStream(context.Background(), e, rencana("m", 1, 0, a),
			func(_ context.Context, _ *upstream.RouteCandidate) (providers.Stream, error) {
				return aliranTahanBaru(), nil
			})
		if pegang.Err != nil {
			t.Fatalf("ExecuteStream pemegang slot: %v", pegang.Err)
		}
		defer func() { _ = pegang.Value.Close() }()

		plan := rencana("m", 1, 0, a, b)
		plan.Budget = 1 // anggaran combo: tepat satu percobaan nyata
		out := Execute(context.Background(), e, plan,
			func(_ context.Context, c *upstream.RouteCandidate) (string, error) {
				if c.ProviderName == "a" {
					t.Error("kandidat a dipanggil padahal slotnya penuh")
				}
				return "dari-b", nil
			})
		if out.Err != nil || out.Value != "dari-b" {
			t.Fatalf("hasil = (%q, %v), mau (dari-b, nil) — anggaran habis oleh lewatan slot", out.Value, out.Err)
		}
	})

	t.Run("lewatan pemutus arus", func(t *testing.T) {
		g := penjaga()
		g.tolak["prov-a"] = true
		e, _ := executorUji(g)

		plan := rencana("m", 1, 0, kandidat("a"), kandidat("b"))
		plan.Budget = 1
		out := Execute(context.Background(), e, plan,
			func(_ context.Context, c *upstream.RouteCandidate) (string, error) {
				if c.ProviderName == "a" {
					t.Error("kandidat a dipanggil padahal pemutus arusnya terbuka")
				}
				return "dari-b", nil
			})
		if out.Err != nil || out.Value != "dari-b" {
			t.Fatalf("hasil = (%q, %v), mau (dari-b, nil) — anggaran habis oleh lewatan breaker", out.Value, out.Err)
		}
	})
}
