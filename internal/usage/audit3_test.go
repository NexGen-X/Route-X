package usage

import (
	"context"
	"errors"
	"strconv"
	"strings"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"github.com/NexGen-X/Route-X/internal/database/repo"
	"github.com/NexGen-X/Route-X/internal/database/repo/upstream"
	"github.com/NexGen-X/Route-X/internal/observability"
	"github.com/NexGen-X/Route-X/internal/providers"
)

// Test regresi audit3 klaster penagihan (B-011, B-014).

// sumberHargaWaktu adalah PriceSource yang juga menjawab harga historis lewat
// At, dengan harga yang SENGAJA berbeda dari cuplikan current supaya test bisa
// membuktikan harga mana yang dipakai penagihan.
type sumberHargaWaktu struct {
	sumberHarga
	lama    *upstream.Price
	errAt   error
	dipakai atomic.Int64
}

func (s *sumberHargaWaktu) At(_ context.Context, _ string, _ time.Time) (*upstream.Price, error) {
	s.dipakai.Add(1)
	if s.errAt != nil {
		return nil, s.errAt
	}
	return s.lama, nil
}

// nilaiMetrik membaca satu counter dari keluaran scrape Prometheus.
func nilaiMetrik(t *testing.T, teks, awalan string) float64 {
	t.Helper()
	for _, baris := range strings.Split(teks, "\n") {
		if strings.HasPrefix(baris, awalan) {
			bidang := strings.Fields(baris)
			v, err := strconv.ParseFloat(bidang[len(bidang)-1], 64)
			if err != nil {
				t.Fatalf("urai nilai metrik %q: %v", baris, err)
			}
			return v
		}
	}
	return 0
}

func TestTokensForMemisahkanCacheCreation(t *testing.T) {
	// Konvensi Anthropic yang dibawa adaptor: InputTokens SUDAH memuat token
	// baca cache dan pembuatan cache. Keduanya harus dipisah — pembuatan cache
	// ditagih di atas tarif input, bukan dilipat ke dalamnya (audit3 V-AB-11).
	got := TokensFor(providers.Usage{
		InputTokens:              1600,
		CachedInputTokens:        500,
		CacheCreationInputTokens: 1000,
		OutputTokens:             100,
		TotalTokens:              1700,
	})
	want := upstream.TokenUsage{Input: 100, CachedInput: 500, CacheCreation: 1000, Output: 100}
	if got != want {
		t.Fatalf("TokensFor = %+v, mau %+v", got, want)
	}
	if total := got.Total(); total != 1700 {
		t.Errorf("total token setelah dipisah = %d, mau 1700", total)
	}
}

func TestPricerPriceAtMemakaiHargaPadaWaktunya(t *testing.T) {
	cachedLama := usd(t, "0.10")
	lama := &upstream.Price{
		ProviderModelID: "pm-1",
		Input:           usd(t, "1.00"),
		Output:          usd(t, "5.00"),
		CachedInput:     &cachedLama,
	}
	src := &sumberHargaWaktu{
		sumberHarga: sumberHarga{rows: []*upstream.Price{{
			ProviderModelID: "pm-1",
			Input:           usd(t, "3.00"),
			Output:          usd(t, "15.00"),
		}}},
		lama: lama,
	}
	p := NewPricer(src, testLogger())
	if err := p.Warm(context.Background()); err != nil {
		t.Fatalf("Warm: %v", err)
	}

	got, ok := p.PriceAt(context.Background(), "pm-1", time.Now().Add(-24*time.Hour))
	if !ok {
		t.Fatal("PriceAt: harga tidak ditemukan")
	}
	if got.Input != lama.Input {
		t.Errorf("PriceAt input = %s, mau harga lampau %s (bukan cuplikan current)", got.Input, lama.Input)
	}
	if src.dipakai.Load() == 0 {
		t.Error("At tidak pernah dipanggil — PriceAt jatuh ke cuplikan secara diam-diam")
	}
}

func TestPricerPriceAtJatuhKeCuplikanTanpaSumberHistoris(t *testing.T) {
	src := &sumberHarga{rows: []*upstream.Price{{
		ProviderModelID: "pm-1",
		Input:           usd(t, "3.00"),
		Output:          usd(t, "15.00"),
	}}}
	p := NewPricer(src, testLogger())
	if err := p.Warm(context.Background()); err != nil {
		t.Fatalf("Warm: %v", err)
	}
	got, ok := p.PriceAt(context.Background(), "pm-1", time.Now().Add(-24*time.Hour))
	if !ok || got.Input != usd(t, "3.00") {
		t.Errorf("PriceAt tanpa At = %+v, %v; mau harga cuplikan", got, ok)
	}
}

func TestPricerPriceAtTidakDikenal(t *testing.T) {
	src := &sumberHargaWaktu{errAt: repo.ErrNotFound}
	p := NewPricer(src, testLogger())
	if _, ok := p.PriceAt(context.Background(), "pm-x", time.Now()); ok {
		t.Error("PriceAt untuk pemetaan tanpa harga historis harus ok=false")
	}
}

// TestRecorderMenagihHargaWaktuPermintaan membuktikan V-AB-09: catatan yang
// antre melewati perubahan harga ditagih dengan harga yang berlaku saat
// permintaan terjadi (TerjadiPada), bukan harga current saat pekerja memproses.
func TestRecorderMenagihHargaWaktuPermintaan(t *testing.T) {
	cachedLama := usd(t, "0.10")
	src := &sumberHargaWaktu{
		sumberHarga: sumberHarga{rows: []*upstream.Price{{
			ProviderModelID: "pm-1",
			Input:           usd(t, "3.00"),
			Output:          usd(t, "15.00"),
		}}},
		lama: &upstream.Price{
			ProviderModelID: "pm-1",
			Input:           usd(t, "1.00"),
			Output:          usd(t, "5.00"),
			CachedInput:     &cachedLama,
		},
	}
	prices := NewPricer(src, testLogger())
	if err := prices.Warm(context.Background()); err != nil {
		t.Fatalf("Warm: %v", err)
	}
	store := &penyimpanan{}
	spend := &pemakai{}
	r := New(Deps{Store: store, Spend: spend, Prices: prices, Metrics: observability.NewMetrics(), Logger: testLogger()})

	ev := eventLengkap()
	ev.TerjadiPada = time.Now().Add(-time.Hour)
	r.Record(context.Background(), ev)
	tutup(t, r)

	// Harga lampau: segar 600×$1 + cache 400×$0.10 + output 300×$5 + reasoning
	// 200×$5 = $0.00314 = 314000 satuan. Harga current akan memberi 942000.
	jumlah := spend.total()
	if len(jumlah) != 1 {
		t.Fatalf("AddSpend terpanggil %d kali, mau 1", len(jumlah))
	}
	if jumlah[0] != upstream.USD(314000) {
		t.Errorf("biaya tertagih = %d satuan, mau 314000 (harga waktu permintaan)", jumlah[0])
	}
}

// TestRecorderTanpaHargaTerhitungDiMetrik membuktikan V-AB-10: pemetaan tanpa
// harga tidak lagi senyap — selain baris biaya nol, ada counter yang bisa
// dialarmkan operator.
func TestRecorderTanpaHargaTerhitungDiMetrik(t *testing.T) {
	r, store, spend, metrics := pencatatUji(t)

	ev := eventLengkap()
	ev.ProviderModelID = "pm-tanpa-harga"
	r.Record(context.Background(), ev)
	tutup(t, r)

	if baris := store.semua(); len(baris) != 1 || baris[0].CostUSD != 0 {
		t.Fatalf("baris tercatat = %+v, mau satu baris biaya nol", baris)
	}
	if total := spend.total(); len(total) != 0 {
		t.Errorf("AddSpend terpanggil untuk biaya nol: %v", total)
	}
	teks := scrape(t, metrics)
	if v := nilaiMetrik(t, teks, "routex_usage_unpriced_total{"); v != 1 {
		t.Errorf("routex_usage_unpriced_total = %v, mau 1\n%s", v, teks)
	}
}

// TestRecorderKegagalanTimelineTerhitungDiMetrik membuktikan V-AB-23:
// kegagalan AppendEvents menaikkan counter, bukan hanya baris log Warn.
func TestRecorderKegagalanTimelineTerhitungDiMetrik(t *testing.T) {
	r, store, _, metrics := pencatatUji(t)
	store.errEvents = errors.New("timeline putus")

	// Timeline hanya ditulis bila ada percobaan/keputusan routing, jadi event
	// uji membawa satu percobaan.
	ev := eventLengkap()
	ev.Attempts = []Attempt{
		{ProviderID: "p1", ProviderName: "openai", Nth: 1, Duration: 100 * time.Millisecond, BreakerState: "closed"},
	}
	r.Record(context.Background(), ev)
	tutup(t, r)

	teks := scrape(t, metrics)
	if v := nilaiMetrik(t, teks, "routex_usage_event_failures_total "); v != 1 {
		t.Errorf("routex_usage_event_failures_total = %v, mau 1\n%s", v, teks)
	}
}

// TestRecorderRecordBersamaanDenganClose membuktikan V-LN-17: Record yang
// berlomba dengan Close tidak panik (kirim ke antrean yang sudah ditutup) dan
// tidak hilang senyap — setiap catatan berakhir tertulis atau terhitung
// terbuang. Paling bermakna di bawah -race.
func TestRecorderRecordBersamaanDenganClose(t *testing.T) {
	src := &sumberHarga{rows: []*upstream.Price{{
		ProviderModelID: "pm-1",
		Input:           usd(t, "3.00"),
		Output:          usd(t, "15.00"),
	}}}
	prices := NewPricer(src, testLogger())
	if err := prices.Warm(context.Background()); err != nil {
		t.Fatalf("Warm: %v", err)
	}
	store := &penyimpanan{}
	metrics := observability.NewMetrics()
	r := New(Deps{Store: store, Spend: &pemakai{}, Prices: prices, Metrics: metrics, Logger: testLogger()})

	const pekerja = 8
	const perPekerja = 250
	const total = pekerja * perPekerja

	var wg sync.WaitGroup
	for i := 0; i < pekerja; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			for j := 0; j < perPekerja; j++ {
				r.Record(context.Background(), eventLengkap())
			}
		}()
	}
	// Tutup di tengah perlombaan, dengan tenggat longgar supaya antrean yang
	// sah sempat terkuras.
	ctx, batal := context.WithTimeout(context.Background(), 30*time.Second)
	defer batal()
	_ = r.Close(ctx)
	wg.Wait()

	tertulis := float64(len(store.semua()))
	teks := scrape(t, metrics)
	terbuang := nilaiMetrik(t, teks, `routex_usage_records_total{outcome="dropped"}`)
	gagal := nilaiMetrik(t, teks, `routex_usage_records_total{outcome="failed"}`)
	if tertulis+terbuang+gagal != total {
		t.Errorf("tertulis %v + terbuang %v + gagal %v = %v, mau %d — ada catatan hilang senyap",
			tertulis, terbuang, gagal, tertulis+terbuang+gagal, total)
	}
}
