package billing

import (
	"context"
	"errors"
	"sync"
	"testing"
	"time"

	"github.com/NexGen-X/Route-X/internal/database/repo/policy"
	"github.com/NexGen-X/Route-X/internal/database/repo/upstream"
)

// sumber adalah Source tiruan.
type sumber struct {
	mu        sync.Mutex
	budgets   []*policy.Budget
	err       error
	dibaca    int
	ditandai  []string
	tandaiOK  bool
	tandaiErr error
}

func (s *sumber) ActiveBudgets(context.Context) ([]*policy.Budget, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.dibaca++
	if s.err != nil {
		return nil, s.err
	}
	return s.budgets, nil
}

func (s *sumber) MarkAlerted(_ context.Context, id string) (bool, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.ditandai = append(s.ditandai, id)
	return s.tandaiOK, s.tandaiErr
}

func (s *sumber) jumlahDibaca() int {
	s.mu.Lock()
	defer s.mu.Unlock()
	return s.dibaca
}

func (s *sumber) penandaan() []string {
	s.mu.Lock()
	defer s.mu.Unlock()
	return append([]string(nil), s.ditandai...)
}

// anggaran menyusun satu anggaran secukupnya untuk test.
func anggaran(id, scope, scopeID, limit, spent string, ubah func(*policy.Budget)) *policy.Budget {
	b := &policy.Budget{
		ID: id, Name: id, Scope: scope, ScopeID: scopeID, Period: policy.PeriodMonthly,
		LimitUSD: upstream.MustParseUSD(limit), SpentUSD: upstream.MustParseUSD(spent),
		ActionOnExceed: policy.ActionBlock, AlertThresholdPct: 80, Enabled: true,
	}
	if ubah != nil {
		ubah(b)
	}
	return b
}

func TestCheckMemblokirSaatAnggaranHabis(t *testing.T) {
	src := &sumber{budgets: []*policy.Budget{
		anggaran("global", policy.ScopeGlobal, "", "100", "100", nil),
	}}
	e := NewEnforcer(src, nil)

	v := e.Check(context.Background(), nil)
	if !v.Blocked {
		t.Fatal("anggaran habis tidak memblokir")
	}
	if v.Budget == nil || v.Budget.ID != "global" {
		t.Errorf("anggaran pemblokir = %v", v.Budget)
	}
}

// TestCheckMelaporkanYangPalingMengikat: kalau dua anggaran sama-sama habis, yang perlu
// diketahui pelanggan adalah yang paling mengikat — melonggarkan yang lain tidak menolong.
func TestCheckMelaporkanYangPalingMengikat(t *testing.T) {
	src := &sumber{budgets: []*policy.Budget{
		anggaran("global-longgar", policy.ScopeGlobal, "", "1000", "1000", nil),
		anggaran("key-ketat", policy.ScopeAPIKey, "key-1", "10", "50", nil),
	}}
	e := NewEnforcer(src, nil)

	v := e.Check(context.Background(), []policy.Target{{Scope: policy.ScopeAPIKey, ID: "key-1"}})
	if !v.Blocked {
		t.Fatal("tidak memblokir")
	}
	if v.Budget.ID != "key-ketat" {
		t.Errorf("anggaran pemblokir = %s, mau key-ketat (sisanya paling sedikit)", v.Budget.ID)
	}
}

// TestCheckHanyaTargetSendiri menjaga batas penyewa: anggaran milik key lain tidak boleh
// memblokir permintaan key ini.
func TestCheckHanyaTargetSendiri(t *testing.T) {
	src := &sumber{budgets: []*policy.Budget{
		anggaran("key-lain", policy.ScopeAPIKey, "key-lain", "10", "999", nil),
	}}
	e := NewEnforcer(src, nil)

	v := e.Check(context.Background(), []policy.Target{{Scope: policy.ScopeAPIKey, ID: "key-1"}})
	if v.Blocked {
		t.Errorf("diblokir anggaran milik key lain: %v", v.Budget)
	}
}

func TestCheckActionWarnTidakMemblokir(t *testing.T) {
	src := &sumber{budgets: []*policy.Budget{
		anggaran("peringatan", policy.ScopeGlobal, "", "10", "999", func(b *policy.Budget) {
			b.ActionOnExceed = policy.ActionWarn
		}),
	}}
	e := NewEnforcer(src, nil)

	v := e.Check(context.Background(), nil)
	if v.Blocked {
		t.Error("anggaran berjenis warn memblokir; operator yang memilih warn justru tidak mau lalu lintasnya mati")
	}
	// Terlampaui berjenis warn tetap melewati ambang, jadi tetap perlu diberitahukan.
	if len(v.Alerts) != 1 {
		t.Errorf("jumlah peringatan = %d, mau 1", len(v.Alerts))
	}
}

func TestCheckMengumpulkanPeringatanSebelumHabis(t *testing.T) {
	src := &sumber{budgets: []*policy.Budget{
		anggaran("hampir", policy.ScopeGlobal, "", "100", "85", nil),
		anggaran("masih-aman", policy.ScopeAPIKey, "key-1", "100", "10", nil),
	}}
	e := NewEnforcer(src, nil)

	v := e.Check(context.Background(), []policy.Target{{Scope: policy.ScopeAPIKey, ID: "key-1"}})
	if v.Blocked {
		t.Fatal("memblokir padahal belum habis")
	}
	if len(v.Alerts) != 1 || v.Alerts[0].ID != "hampir" {
		t.Errorf("peringatan = %v, mau hanya anggaran yang melewati ambang", v.Alerts)
	}
}

func TestAnnounceMenandaiSekaliLaluMembatalkanSalinan(t *testing.T) {
	src := &sumber{
		budgets:  []*policy.Budget{anggaran("hampir", policy.ScopeGlobal, "", "100", "85", nil)},
		tandaiOK: true,
	}
	e := NewEnforcer(src, nil)

	v := e.Check(context.Background(), nil)
	if len(v.Alerts) != 1 {
		t.Fatalf("jumlah peringatan = %d, mau 1", len(v.Alerts))
	}
	e.Announce(context.Background(), v.Alerts)

	if got := src.penandaan(); len(got) != 1 || got[0] != "hampir" {
		t.Errorf("penandaan = %v", got)
	}
	// Salinan dibatalkan supaya pemeriksaan berikutnya membaca alerted_at yang sudah terisi
	// alih-alih mengumpulkan peringatan yang sama sampai TTL habis.
	sebelum := src.jumlahDibaca()
	e.Check(context.Background(), nil)
	if src.jumlahDibaca() != sebelum+1 {
		t.Error("salinan tidak dibatalkan setelah Announce")
	}
}

func TestRemaining(t *testing.T) {
	src := &sumber{budgets: []*policy.Budget{
		anggaran("global", policy.ScopeGlobal, "", "100", "10", nil),
		anggaran("key", policy.ScopeAPIKey, "key-1", "20", "17.5", nil),
	}}
	e := NewEnforcer(src, nil)

	sisa, ada := e.Remaining(context.Background(), []policy.Target{{Scope: policy.ScopeAPIKey, ID: "key-1"}})
	if !ada {
		t.Fatal("tidak ada anggaran yang dilaporkan")
	}
	if sisa != upstream.MustParseUSD("2.5") {
		t.Errorf("sisa = %s, mau 2.5 (anggaran paling mengikat)", sisa)
	}

	kosong := NewEnforcer(nil, nil)
	if _, ada := kosong.Remaining(context.Background(), nil); ada {
		t.Error("tanpa sumber melaporkan ada anggaran")
	}
}

func TestPembacaanGagalMempertahankanSalinanLama(t *testing.T) {
	src := &sumber{budgets: []*policy.Budget{
		anggaran("global", policy.ScopeGlobal, "", "10", "10", nil),
	}}
	sekarang := time.Now()
	e := NewEnforcer(src, nil, WithTTL(5*time.Second), WithClock(func() time.Time { return sekarang }))

	if v := e.Check(context.Background(), nil); !v.Blocked {
		t.Fatal("pembacaan pertama tidak memblokir")
	}
	src.mu.Lock()
	src.err = errors.New("database sedang tidak bisa dibaca")
	src.mu.Unlock()
	sekarang = sekarang.Add(time.Minute)

	// Anggaran yang sudah habis harus TETAP memblokir walau tabelnya sedang tidak terbaca:
	// kehilangan salinan berarti biaya kembali mengalir tanpa batas.
	if v := e.Check(context.Background(), nil); !v.Blocked {
		t.Error("setelah pembacaan gagal, anggaran yang habis berhenti memblokir")
	}
}

func TestTanpaSumberTidakMemblokirApaPun(t *testing.T) {
	e := NewEnforcer(nil, nil)
	if v := e.Check(context.Background(), []policy.Target{{Scope: policy.ScopeAPIKey, ID: "key-1"}}); v.Blocked {
		t.Error("tanpa sumber anggaran malah memblokir")
	}
}

func TestAmanDipakaiBersamaan(t *testing.T) {
	src := &sumber{budgets: []*policy.Budget{
		anggaran("global", policy.ScopeGlobal, "", "100", "50", nil),
		anggaran("key", policy.ScopeAPIKey, "key-1", "10", "1", nil),
	}, tandaiOK: true}
	e := NewEnforcer(src, nil, WithTTL(time.Nanosecond))
	target := []policy.Target{{Scope: policy.ScopeAPIKey, ID: "key-1"}}

	var wg sync.WaitGroup
	for i := 0; i < 16; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			for j := 0; j < 50; j++ {
				v := e.Check(context.Background(), target)
				e.Remaining(context.Background(), target)
				if len(v.Alerts) > 0 {
					e.Announce(context.Background(), v.Alerts)
				}
			}
		}()
	}
	wg.Wait()
}

type antreanWebhookUji struct {
	mu     sync.Mutex
	events []string
}

func (a *antreanWebhookUji) Enqueue(ctx context.Context, event string, payload any) (int, error) {
	a.mu.Lock()
	defer a.mu.Unlock()
	a.events = append(a.events, event)
	return 1, nil
}

// TestAnnounceEnqueueWebhookEvents memastikan event budget.threshold dan budget.exceeded
// dimasukkan ke antrean webhook saat ambang peringatan dan batas terlewati.
func TestAnnounceEnqueueWebhookEvents(t *testing.T) {
	src := &sumber{
		budgets: []*policy.Budget{
			anggaran("budget-threshold", policy.ScopeGlobal, "", "100", "85", nil),
			anggaran("budget-exceeded", policy.ScopeGlobal, "", "50", "55", func(b *policy.Budget) {
				b.AlertThresholdPct = 0
			}),
		},
		tandaiOK: true,
	}

	queue := &antreanWebhookUji{}
	e := NewEnforcer(src, nil, WithWebhookEnqueuer(queue))

	v := e.Check(context.Background(), nil)
	if len(v.Alerts) != 2 {
		t.Fatalf("Alerts count = %d, mau 2", len(v.Alerts))
	}

	e.Announce(context.Background(), v.Alerts)

	queue.mu.Lock()
	defer queue.mu.Unlock()
	if len(queue.events) != 2 {
		t.Fatalf("jumlah event diantrekan = %d, mau 2", len(queue.events))
	}
	if queue.events[0] != "budget.threshold" {
		t.Errorf("event 0 = %s, mau budget.threshold", queue.events[0])
	}
	if queue.events[1] != "budget.exceeded" {
		t.Errorf("event 1 = %s, mau budget.exceeded", queue.events[1])
	}
}
