package router

import (
	"context"
	"errors"
	"slices"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"github.com/NexGen-X/Route-X/internal/database/repo/upstream"
)

// sumberAturan adalah RuleSource untuk test.
type sumberAturan struct {
	mu      sync.Mutex
	rows    []*upstream.RoutingRule
	err     error
	panggil atomic.Int64
}

func (s *sumberAturan) ActiveRules(context.Context) ([]*upstream.RoutingRule, error) {
	s.panggil.Add(1)
	s.mu.Lock()
	defer s.mu.Unlock()
	if s.err != nil {
		return nil, s.err
	}
	return slices.Clone(s.rows), nil
}

func (s *sumberAturan) ganti(rows []*upstream.RoutingRule, err error) {
	s.mu.Lock()
	s.rows, s.err = rows, err
	s.mu.Unlock()
}

func baris(nama string, prioritas int, strategi Strategy, ubah ...func(*upstream.RoutingRule)) *upstream.RoutingRule {
	r := &upstream.RoutingRule{
		ID: "id-" + nama, Name: nama, Priority: prioritas,
		Strategy: string(strategi), MaxAttempts: 3, BackoffMS: 250,
		FailureThreshold: 5, OpenDurationMS: 30_000, HalfOpenProbes: 1, Enabled: true,
	}
	for _, f := range ubah {
		f(r)
	}
	return r
}

func TestEngineRouteMemakaiAturanYangCocok(t *testing.T) {
	model := "m1"
	src := &sumberAturan{rows: []*upstream.RoutingRule{
		baris("khusus", 10, StrategyLowestLatency, func(r *upstream.RoutingRule) { r.MatchModelID = &model }),
		baris("bawaan", 100, StrategyPriority),
	}}
	e := NewEngine(src, NewSelector(WithLatencySource(sumberLatensi{
		"pm-cepat": 10 * time.Millisecond, "pm-lambat": 900 * time.Millisecond,
	})), nil)

	cands := []*upstream.RouteCandidate{kandidat("lambat", 1, 1), kandidat("cepat", 99, 1)}
	got := e.Route(context.Background(), Request{ModelID: "m1"}, cands)

	if got.Rule == nil || got.Rule.Name != "khusus" {
		t.Fatalf("Rule = %v, mau khusus", got.Rule)
	}
	// Strategi aturan yang menang benar-benar dipakai: kalau tidak, urutannya akan
	// mengikuti prioritas dan "lambat" ada di depan.
	if mau := []string{"cepat", "lambat"}; !slices.Equal(nama(got.Candidates), mau) {
		t.Errorf("kandidat = %v, mau %v", nama(got.Candidates), mau)
	}
}

// Tidak ada aturan yang cocok BUKAN kegagalan: permintaan dirutekan dengan strategi bawaan.
func TestEngineRouteTanpaAturanYangCocok(t *testing.T) {
	lain := "m-lain"
	src := &sumberAturan{rows: []*upstream.RoutingRule{
		baris("khusus", 10, StrategyLowestCost, func(r *upstream.RoutingRule) { r.MatchModelID = &lain }),
	}}
	e := NewEngine(src, NewSelector(), nil)

	cands := []*upstream.RouteCandidate{kandidat("b", 9, 1), kandidat("a", 1, 1)}
	got := e.Route(context.Background(), Request{ModelID: "m1"}, cands)

	if got.Rule != nil {
		t.Errorf("Rule = %v, mau nil", got.Rule)
	}
	if mau := []string{"a", "b"}; !slices.Equal(nama(got.Candidates), mau) {
		t.Errorf("kandidat = %v, mau %v (urutan prioritas)", nama(got.Candidates), mau)
	}
}

// Tanpa RuleSource sama sekali, routing tetap bekerja. Itu perilaku yang benar untuk
// pemasangan baru yang belum punya satu pun aturan.
func TestEngineTanpaSumberAturan(t *testing.T) {
	e := NewEngine(nil, nil, nil)
	cands := []*upstream.RouteCandidate{kandidat("b", 9, 1), kandidat("a", 1, 1)}
	got := e.Route(context.Background(), Request{}, cands)
	if got.Rule != nil {
		t.Errorf("Rule = %v, mau nil", got.Rule)
	}
	if mau := []string{"a", "b"}; !slices.Equal(nama(got.Candidates), mau) {
		t.Errorf("kandidat = %v, mau %v", nama(got.Candidates), mau)
	}
}

func TestEngineCacheAturan(t *testing.T) {
	src := &sumberAturan{rows: []*upstream.RoutingRule{baris("bawaan", 100, StrategyPriority)}}
	jam := waktuUji{t: time.Now()}
	e := NewEngine(src, NewSelector(), nil, WithRuleTTL(5*time.Second), WithClock(jam.now))

	cands := []*upstream.RouteCandidate{kandidat("a", 1, 1)}
	for i := 0; i < 10; i++ {
		e.Route(context.Background(), Request{}, cands)
	}
	if got := src.panggil.Load(); got != 1 {
		t.Errorf("sumber dibaca %d kali, mau 1 — cache tidak bekerja", got)
	}

	// Setelah TTL lewat, dibaca ulang sekali.
	jam.maju(6 * time.Second)
	for i := 0; i < 10; i++ {
		e.Route(context.Background(), Request{}, cands)
	}
	if got := src.panggil.Load(); got != 2 {
		t.Errorf("sumber dibaca %d kali, mau 2", got)
	}
}

func TestEngineInvalidateMemaksaMuatUlang(t *testing.T) {
	src := &sumberAturan{rows: []*upstream.RoutingRule{baris("bawaan", 100, StrategyPriority)}}
	jam := waktuUji{t: time.Now()}
	e := NewEngine(src, NewSelector(), nil, WithRuleTTL(time.Hour), WithClock(jam.now))

	cands := []*upstream.RouteCandidate{kandidat("a", 1, 1)}
	e.Route(context.Background(), Request{}, cands)

	// Aturan baru: strategi berubah, tetapi TTL masih satu jam.
	src.ganti([]*upstream.RoutingRule{baris("baru", 100, StrategyLowestCost)}, nil)
	if got := e.Route(context.Background(), Request{}, cands); got.Rule.Name != "bawaan" {
		t.Errorf("Rule = %q, mau bawaan — cache seharusnya masih berlaku", got.Rule.Name)
	}

	e.Invalidate()
	if got := e.Route(context.Background(), Request{}, cands); got.Rule.Name != "baru" {
		t.Errorf("Rule = %q, mau baru setelah Invalidate", got.Rule.Name)
	}
}

// Kegagalan membaca aturan tidak boleh menggagalkan permintaan: satu tabel konfigurasi yang
// tidak terbaca akan mematikan seluruh lalu lintas inference, padahal daftar kandidat sudah
// ada di tangan dan urutan bawaan benar-benar bisa dipakai.
func TestEngineGagalBacaAturanTetapMerutekan(t *testing.T) {
	src := &sumberAturan{err: errors.New("postgres tidak menjawab")}
	e := NewEngine(src, NewSelector(), nil)

	cands := []*upstream.RouteCandidate{kandidat("b", 9, 1), kandidat("a", 1, 1)}
	got := e.Route(context.Background(), Request{}, cands)

	if got.Rule != nil {
		t.Errorf("Rule = %v, mau nil", got.Rule)
	}
	if mau := []string{"a", "b"}; !slices.Equal(nama(got.Candidates), mau) {
		t.Errorf("kandidat = %v, mau %v", nama(got.Candidates), mau)
	}
}

// Kegagalan pemuatan ULANG mempertahankan aturan lama. Membuang cache saat sumbernya sedang
// tidak bisa dibaca berarti gangguan sementara pada database mengubah cara seluruh lalu
// lintas dirutekan.
func TestEngineGagalMuatUlangMempertahankanAturanLama(t *testing.T) {
	model := "m1"
	src := &sumberAturan{rows: []*upstream.RoutingRule{
		baris("khusus", 10, StrategyLowestCost, func(r *upstream.RoutingRule) { r.MatchModelID = &model }),
	}}
	jam := waktuUji{t: time.Now()}
	e := NewEngine(src, NewSelector(), nil, WithRuleTTL(time.Second), WithClock(jam.now))

	cands := []*upstream.RouteCandidate{kandidat("a", 1, 1)}
	if got := e.Route(context.Background(), Request{ModelID: "m1"}, cands); got.Rule == nil {
		t.Fatal("aturan tidak terbaca pada pemuatan pertama")
	}

	src.ganti(nil, errors.New("postgres tidak menjawab"))
	jam.maju(2 * time.Second)

	got := e.Route(context.Background(), Request{ModelID: "m1"}, cands)
	if got.Rule == nil || got.Rule.Name != "khusus" {
		t.Errorf("Rule = %v, mau tetap khusus dari cache lama", got.Rule)
	}
}

// Satu baris aturan yang cacat dilewati, sisanya tetap berlaku. Kalau tidak, satu baris
// dengan strategi tak dikenal akan mematikan routing seluruhnya.
func TestEngineMelewatiAturanCacatTanpaMenjatuhkanSisanya(t *testing.T) {
	model := "m1"
	src := &sumberAturan{rows: []*upstream.RoutingRule{
		baris("cacat", 10, Strategy("entah_apa")),
		baris("sehat", 20, StrategyLowestCost, func(r *upstream.RoutingRule) { r.MatchModelID = &model }),
	}}
	e := NewEngine(src, NewSelector(), nil)

	cands := []*upstream.RouteCandidate{kandidat("a", 1, 1)}
	got := e.Route(context.Background(), Request{ModelID: "m1"}, cands)
	if got.Rule == nil || got.Rule.Name != "sehat" {
		t.Fatalf("Rule = %v, mau sehat", got.Rule)
	}
	if len(got.Candidates) != 1 {
		t.Errorf("kandidat = %v, mau satu", nama(got.Candidates))
	}
}

// Pemuatan ulang yang bersamaan disatukan menjadi satu query: cache yang kedaluwarsa di
// bawah beban tidak boleh menghasilkan satu query per permintaan.
func TestEngineMenyatukanPemuatanUlangBersamaan(t *testing.T) {
	src := &sumberAturan{rows: []*upstream.RoutingRule{baris("bawaan", 100, StrategyPriority)}}
	e := NewEngine(src, NewSelector(), nil, WithRuleTTL(time.Hour))

	cands := []*upstream.RouteCandidate{kandidat("a", 1, 1)}
	var wg sync.WaitGroup
	for i := 0; i < 64; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			e.Route(context.Background(), Request{}, cands)
		}()
	}
	wg.Wait()

	if got := src.panggil.Load(); got != 1 {
		t.Errorf("sumber dibaca %d kali, mau 1", got)
	}
}

// waktuUji adalah jam yang bisa dimajukan test.
type waktuUji struct {
	mu sync.Mutex
	t  time.Time
}

func (w *waktuUji) now() time.Time {
	w.mu.Lock()
	defer w.mu.Unlock()
	return w.t
}

func (w *waktuUji) maju(d time.Duration) {
	w.mu.Lock()
	w.t = w.t.Add(d)
	w.mu.Unlock()
}

// OrderCandidates mengurutkan kandidat dari BEBERAPA model menurut strategi resep combo,
// bukan strategi aturan. Bedanya nyata: strategi aturan priority akan menempatkan
// kandidat prioritas-kecil di depan, tapi resep lowest_latency bisa membalik urutan itu
// berdasarkan latensi — strategi yang dipakai harus strategi resep.
func TestOrderCandidatesMemakaiStrategiResep(t *testing.T) {
	e := NewEngine(nil, NewSelector(WithLatencySource(sumberLatensi{
		"pm-lambat": 900 * time.Millisecond, "pm-cepat": 10 * time.Millisecond,
	})), nil)

	// Priority naik: "lambat" (1) lebih dulu daripada "cepat" (99) bila diurutkan
	// berdasarkan strategi priority.
	cands := []*upstream.RouteCandidate{
		kandidat("lambat", 1, 1),
		kandidat("cepat", 99, 1),
	}

	// Strategi resep lowest_latency membalik urutan: "cepat" di depan.
	got := e.OrderCandidates(Request{}, "r-combo", StrategyLowestLatency, cands)
	mau := []string{"cepat", "lambat"}
	if !slices.Equal(nama(got), mau) {
		t.Errorf("urutan = %v, mau %v — strategi resep harus mengalahkan prioritas", nama(got), mau)
	}

	// Strategi resep priority mengembalikan urutan prioritas.
	got = e.OrderCandidates(Request{}, "r-combo", StrategyPriority, cands)
	mau = []string{"lambat", "cepat"}
	if !slices.Equal(nama(got), mau) {
		t.Errorf("urutan = %v, mau %v", nama(got), mau)
	}
}

// Kandidat yang tidak sanggup melayani permintaan dibuang, bukan ditunda: combo pipeline
// harus membuang kandidat tanpa kemampuan yang diminta sekuensial, sama seperti Order biasa.
func TestOrderCandidatesMembuangYangTidakSanggup(t *testing.T) {
	e := NewEngine(nil, NewSelector(), nil)

	cands := []*upstream.RouteCandidate{
		kandidat("alat-tanpa-tools", 1, 1, func(c *upstream.RouteCandidate) { c.SupportsTools = false }),
		kandidat("alat-lengkap", 2, 1),
	}
	got := e.OrderCandidates(Request{Capabilities: []string{upstream.CapTools}}, "r-combo", StrategyPriority, cands)
	mau := []string{"alat-lengkap"}
	if !slices.Equal(nama(got), mau) {
		t.Errorf("urutan = %v, mau %v — kandidat tanpa tools harus dibuang", nama(got), mau)
	}
}
