package usage

import (
	"context"
	"encoding/json"
	"errors"
	"net/http/httptest"
	"net/netip"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/NexGen-X/Route-X/internal/database/repo/policy"
	"github.com/NexGen-X/Route-X/internal/database/repo/traffic"
	"github.com/NexGen-X/Route-X/internal/database/repo/upstream"
	"github.com/NexGen-X/Route-X/internal/observability"
)

// penyimpanan adalah Store palsu yang mencatat apa yang ditulis.
type penyimpanan struct {
	mu        sync.Mutex
	baris     []traffic.Record
	timeline  [][]traffic.Event
	errInsert error
	errEvents error
	// tahan menahan setiap Insert sampai ditutup, untuk menguji antrean penuh.
	tahan chan struct{}
}

func (p *penyimpanan) Insert(_ context.Context, rec traffic.Record) (string, time.Time, error) {
	if p.tahan != nil {
		<-p.tahan
	}
	p.mu.Lock()
	defer p.mu.Unlock()
	if p.errInsert != nil {
		return "", time.Time{}, p.errInsert
	}
	p.baris = append(p.baris, rec)
	return "pk-1", time.Unix(1_760_000_000, 0).UTC(), nil
}

func (p *penyimpanan) AppendEvents(_ context.Context, _ string, _ time.Time, ev []traffic.Event) error {
	p.mu.Lock()
	defer p.mu.Unlock()
	if p.errEvents != nil {
		return p.errEvents
	}
	p.timeline = append(p.timeline, ev)
	return nil
}

func (p *penyimpanan) semua() []traffic.Record {
	p.mu.Lock()
	defer p.mu.Unlock()
	return append([]traffic.Record(nil), p.baris...)
}

// pemakai adalah Spender palsu.
type pemakai struct {
	mu      sync.Mutex
	jumlah  []upstream.USD
	targets [][]policy.Target
	err     error
}

func (p *pemakai) AddSpend(_ context.Context, targets []policy.Target, amount upstream.USD) (int64, error) {
	p.mu.Lock()
	defer p.mu.Unlock()
	if p.err != nil {
		return 0, p.err
	}
	p.jumlah = append(p.jumlah, amount)
	p.targets = append(p.targets, targets)
	return int64(len(targets)), nil
}

func (p *pemakai) total() []upstream.USD {
	p.mu.Lock()
	defer p.mu.Unlock()
	return append([]upstream.USD(nil), p.jumlah...)
}

// scrape mengambil keluaran /metrics sebagai teks.
func scrape(t *testing.T, m *observability.Metrics) string {
	t.Helper()
	rec := httptest.NewRecorder()
	m.Handler().ServeHTTP(rec, httptest.NewRequest("GET", "/metrics", nil))
	return rec.Body.String()
}

// pencatatUji merakit pencatat lengkap beserta harga untuk satu pemetaan.
func pencatatUji(t *testing.T, opts ...Option) (*Recorder, *penyimpanan, *pemakai, *observability.Metrics) {
	t.Helper()
	cached := usd(t, "0.30")
	src := &sumberHarga{rows: []*upstream.Price{{
		ProviderModelID: "pm-1",
		Input:           usd(t, "3.00"),
		Output:          usd(t, "15.00"),
		CachedInput:     &cached,
	}}}
	prices := NewPricer(src, testLogger())
	if err := prices.Warm(context.Background()); err != nil {
		t.Fatalf("Warm: %v", err)
	}

	store := &penyimpanan{}
	spend := &pemakai{}
	metrics := observability.NewMetrics()
	r := New(Deps{Store: store, Spend: spend, Prices: prices, Metrics: metrics, Logger: testLogger()}, opts...)
	t.Cleanup(func() {
		ctx, batal := context.WithTimeout(context.Background(), 5*time.Second)
		defer batal()
		_ = r.Close(ctx)
	})
	return r, store, spend, metrics
}

// tutup menutup pencatat dan menunggu antreannya habis.
func tutup(t *testing.T, r *Recorder) {
	t.Helper()
	ctx, batal := context.WithTimeout(context.Background(), 5*time.Second)
	defer batal()
	if err := r.Close(ctx); err != nil {
		t.Fatalf("Close: %v", err)
	}
}

func eventLengkap() Event {
	return Event{
		RequestID:      "req-1",
		Method:         "POST",
		Endpoint:       "/v1/chat/completions",
		APIKeyID:       "00000000-0000-0000-0000-0000000000a1",
		APIKeyName:     "kunci produksi",
		UserID:         "00000000-0000-0000-0000-0000000000b1",
		RequestedModel: "gpt-5-mini",
		ModelID:        "00000000-0000-0000-0000-0000000000c1",
		ModelName:      "gpt-5",
		ProviderID:     "00000000-0000-0000-0000-0000000000d1",
		ProviderName:   "openai",

		ProviderModelID: "pm-1",
		UpstreamModel:   "gpt-5-2026-08-01",

		StatusCode: 200,

		InputTokens: 1000, CachedInputTokens: 400,
		OutputTokens: 500, ReasoningTokens: 200, TotalTokens: 1500,

		Latency:         1500 * time.Millisecond,
		UpstreamLatency: 1400 * time.Millisecond,

		RoutingStrategy: "lowest_cost",
		Targets: []policy.Target{
			{Scope: policy.ScopeAPIKey, ID: "00000000-0000-0000-0000-0000000000a1"},
			{Scope: policy.ScopeGlobal},
		},
		ClientIP:  netip.MustParseAddr("203.0.113.9"),
		UserAgent: "openai-python/1.2.3",
	}
}

func TestRecorderMenulisBarisDanMenaikkanAnggaran(t *testing.T) {
	r, store, spend, metrics := pencatatUji(t)

	r.Record(context.Background(), eventLengkap())
	tutup(t, r)

	baris := store.semua()
	if len(baris) != 1 {
		t.Fatalf("baris tertulis = %d, mau 1", len(baris))
	}
	got := baris[0]

	// 0,00942 USD, angka yang sama dengan TestBiayaMemakaiBagianYangSudahDipisah.
	if got.CostUSD != 942_000 {
		t.Errorf("cost_usd = %d satuan, mau 942000", got.CostUSD)
	}
	// Kolom token memakai konvensi OpenAI: input SUDAH memuat cached, output SUDAH memuat
	// reasoning. Kalau ini berubah, angka di log request tidak lagi sama dengan angka di
	// body respons yang diterima klien.
	if got.InputTokens != 1000 || got.OutputTokens != 500 || got.TotalTokens != 1500 {
		t.Errorf("token = %d/%d/%d, mau 1000/500/1500", got.InputTokens, got.OutputTokens, got.TotalTokens)
	}
	if got.LatencyMS != 1500 || got.TTFTMS != nil {
		t.Errorf("latensi = %d, ttft = %v; mau 1500 dan nil untuk non-streaming", got.LatencyMS, got.TTFTMS)
	}
	if got.UpstreamLatencyMS == nil || *got.UpstreamLatencyMS != 1400 {
		t.Errorf("upstream_latency_ms = %v, mau 1400", got.UpstreamLatencyMS)
	}
	if got.ErrorType != nil {
		t.Errorf("error_type = %v pada permintaan berhasil, mau NULL", *got.ErrorType)
	}

	if total := spend.total(); len(total) != 1 || total[0] != 942_000 {
		t.Errorf("AddSpend = %v, mau satu panggilan berisi 942000 satuan", total)
	}

	body := scrape(t, metrics)
	for _, mau := range []string{
		`routex_usage_records_total{outcome="written"} 1`,
		`routex_gateway_requests_total{model="gpt-5",provider="openai",status="200",stream="false"} 1`,
		`routex_gateway_tokens_total{direction="input",model="gpt-5",provider="openai"} 1000`,
	} {
		if !strings.Contains(body, mau) {
			t.Errorf("metrik %q tidak ada di /metrics", mau)
		}
	}
}

// Nama model yang dipakai sebagai label metrik WAJIB nama kanonik. RequestedModel datang
// dari klien, jadi memakainya berarti satu klien yang mengirim nama acak bisa meledakkan
// jumlah time series.
func TestRecorderTidakMemakaiNamaModelDariKlienSebagaiLabel(t *testing.T) {
	r, _, _, metrics := pencatatUji(t)

	ev := eventLengkap()
	ev.RequestedModel = "model-karangan-klien-9f2a"
	r.Record(context.Background(), ev)
	tutup(t, r)

	if body := scrape(t, metrics); strings.Contains(body, "model-karangan-klien-9f2a") {
		t.Fatal("nama model dari klien muncul sebagai label metrik")
	}
}

func TestRecorderMencatatKegagalanPenulisanTanpaMenggagalkanApaPun(t *testing.T) {
	r, store, spend, metrics := pencatatUji(t)
	store.mu.Lock()
	store.errInsert = errors.New("relation \"requests\" is not available")
	store.mu.Unlock()

	r.Record(context.Background(), eventLengkap())
	tutup(t, r)

	if body := scrape(t, metrics); !strings.Contains(body, `routex_usage_records_total{outcome="failed"} 1`) {
		t.Error("kegagalan penulisan tidak terlihat di metrik")
	}
	// Pemakaian anggaran TETAP dinaikkan: biayanya sudah nyata dikeluarkan, dan kegagalan
	// menulis log tidak boleh ikut membatalkan penegakan anggaran.
	if total := spend.total(); len(total) != 1 {
		t.Errorf("AddSpend dipanggil %d kali saat penulisan gagal, mau 1", len(total))
	}
}

func TestRecorderMelaporkanKegagalanAnggaranTerpisah(t *testing.T) {
	r, _, spend, metrics := pencatatUji(t)
	spend.mu.Lock()
	spend.err = errors.New("deadlock detected")
	spend.mu.Unlock()

	r.Record(context.Background(), eventLengkap())
	tutup(t, r)

	body := scrape(t, metrics)
	if !strings.Contains(body, `routex_usage_records_total{outcome="written"} 1`) {
		t.Error("baris log seharusnya tetap tertulis")
	}
	if !strings.Contains(body, `routex_usage_records_total{outcome="spend_failed"} 1`) {
		t.Error("kegagalan menaikkan pemakaian anggaran tidak dihitung terpisah")
	}
}

func TestRecorderMembuangCatatanSaatAntreanPenuh(t *testing.T) {
	tahan := make(chan struct{})
	r, store, _, metrics := pencatatUji(t, WithQueueSize(1), WithWorkers(1))
	store.mu.Lock()
	store.tahan = tahan
	store.mu.Unlock()

	// Satu catatan diambil penulis dan tertahan, satu mengisi antrean, sisanya dibuang.
	for range 12 {
		r.Record(context.Background(), eventLengkap())
	}
	close(tahan)
	tutup(t, r)

	body := scrape(t, metrics)
	if !strings.Contains(body, `routex_usage_records_total{outcome="dropped"}`) {
		t.Fatal("catatan yang dibuang tidak dihitung; batas antrean menjadi tidak bisa dipertanggungjawabkan")
	}
	// Metrik permintaan diambil SEBELUM antrean, jadi keduabelasnya harus tetap terhitung
	// walaupun sebagian barisnya tidak pernah tertulis.
	if !strings.Contains(body, `status="200",stream="false"} 12`) {
		t.Error("metrik permintaan hilang untuk catatan yang dibuang")
	}
}

func TestRecordSetelahCloseTidakPanik(t *testing.T) {
	r, _, _, metrics := pencatatUji(t)
	tutup(t, r)

	// Tidak boleh panik walaupun antreannya sudah tidak dilayani siapa pun.
	r.Record(context.Background(), eventLengkap())

	if body := scrape(t, metrics); !strings.Contains(body, `routex_usage_records_total{outcome="dropped"} 1`) {
		t.Error("catatan setelah Close tidak dihitung sebagai dibuang")
	}
}

func TestRecorderMenulisTimelineDanKeputusanRouting(t *testing.T) {
	r, store, _, _ := pencatatUji(t)

	ev := eventLengkap()
	ev.Routing = Routing{
		Strategy: "lowest_cost",
		RuleID:   "00000000-0000-0000-0000-0000000000e1",
		RuleName: "murah dulu",
		Candidates: []RoutingCandidate{
			{Position: 1, ProviderName: "mati", ProviderModelID: "pm-0", Priority: 1},
			{Position: 2, ProviderName: "openai", ProviderModelID: "pm-1", Priority: 10, Chosen: true},
		},
	}
	ev.Attempts = []Attempt{
		{ProviderID: "p0", ProviderName: "mati", Nth: 1, Duration: 20 * time.Millisecond,
			ErrorKind: "network", ErrorMessage: "koneksi ditolak", BreakerState: "closed"},
		{ProviderID: "p1", ProviderName: "openai", Nth: 1, Duration: 1200 * time.Millisecond,
			BreakerState: "closed"},
	}
	ev.Notes = []Note{{Kind: traffic.EventContentBlocked, Message: "jawaban ditolak penyaring"}}
	r.Record(context.Background(), ev)
	tutup(t, r)

	store.mu.Lock()
	timeline := store.timeline
	store.mu.Unlock()
	if len(timeline) != 1 || len(timeline[0]) != 3 {
		t.Fatalf("timeline = %v, mau satu penulisan berisi 3 peristiwa", timeline)
	}
	kinds := []string{timeline[0][0].Kind, timeline[0][1].Kind, timeline[0][2].Kind}
	mau := []string{traffic.EventAttemptFailed, traffic.EventAttemptSucceeded, traffic.EventContentBlocked}
	for i := range mau {
		if kinds[i] != mau[i] {
			t.Errorf("timeline[%d].Kind = %q, mau %q", i, kinds[i], mau[i])
		}
		if timeline[0][i].Seq != i+1 {
			t.Errorf("timeline[%d].Seq = %d, mau %d", i, timeline[0][i].Seq, i+1)
		}
	}

	var rt Routing
	if err := json.Unmarshal(store.semua()[0].RoutingDecision, &rt); err != nil {
		t.Fatalf("routing_decision bukan JSON yang bisa dibaca: %v", err)
	}
	if rt.RuleName != "murah dulu" || len(rt.Candidates) != 2 || !rt.Candidates[1].Chosen {
		t.Errorf("routing_decision = %+v", rt)
	}
}

// Status di luar rentang yang bisa disimpan berarti ada jalur keluar yang lupa melaporkan
// statusnya. Barisnya tetap harus tertulis, karena baris berstatus salah masih jauh lebih
// berguna daripada tidak ada baris sama sekali.
func TestRecorderMenjepitStatusYangTidakSah(t *testing.T) {
	r, store, _, _ := pencatatUji(t)

	ev := eventLengkap()
	ev.StatusCode = 0
	r.Record(context.Background(), ev)
	tutup(t, r)

	if got := store.semua()[0].StatusCode; got != 500 {
		t.Fatalf("status_code = %d, mau 500", got)
	}
}

func TestRecorderTanpaHargaMencatatBiayaNolDanTidakMenyentuhAnggaran(t *testing.T) {
	store, spend := &penyimpanan{}, &pemakai{}
	r := New(Deps{Store: store, Spend: spend, Prices: NewPricer(nil, testLogger()), Logger: testLogger()})

	r.Record(context.Background(), eventLengkap())
	tutup(t, r)

	if got := store.semua()[0].CostUSD; got != 0 {
		t.Errorf("cost_usd = %d, mau 0 saat harga belum diisi", got)
	}
	// Nol tidak boleh menghasilkan panggilan AddSpend: UPDATE yang menambah nol hanya
	// menyentuh baris tanpa mengubah apa pun.
	if total := spend.total(); len(total) != 0 {
		t.Errorf("AddSpend dipanggil %d kali untuk biaya nol", len(total))
	}
}

func TestRecorderMencatatKegagalanUpstreamPerPercobaan(t *testing.T) {
	r, _, _, metrics := pencatatUji(t)

	ev := eventLengkap()
	ev.StatusCode = 504
	ev.ErrorType = "timeout"
	ev.Attempts = []Attempt{
		{ProviderID: "p0", ProviderName: "lambat", Nth: 1, ErrorKind: "timeout"},
		{ProviderID: "p0", ProviderName: "lambat", Nth: 2, ErrorKind: "timeout"},
		{ProviderID: "p1", ProviderName: "openai", Nth: 1, ErrorKind: "server"},
	}
	r.Record(context.Background(), ev)
	tutup(t, r)

	body := scrape(t, metrics)
	for _, mau := range []string{
		`routex_gateway_upstream_failures_total{kind="timeout",provider="lambat"} 2`,
		`routex_gateway_upstream_failures_total{kind="server",provider="openai"} 1`,
		`routex_gateway_retries_total{provider="lambat",reason="timeout"} 1`,
		`routex_gateway_failovers_total{from_provider="lambat",reason="timeout",to_provider="openai"} 1`,
	} {
		if !strings.Contains(body, mau) {
			t.Errorf("metrik %q tidak ada di /metrics", mau)
		}
	}
}
