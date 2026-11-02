package usage

import (
	"context"
	"math"
	"strings"
	"testing"
	"time"

	"github.com/NexGen-X/Route-X/internal/database/repo/upstream"
	"github.com/NexGen-X/Route-X/internal/observability"
)

// Test regresi sisa audit3 B-068 (V-EM-25 bagian b): event yang DIBUANG
// pencatat — antrean penuh atau pencatat sudah ditutup — tetap dihitung
// biayanya dan dinaikkan ke anggaran, persis sekali, tanpa baris requests.

// rakitPencatatHargaCurrent merakit pencatat dengan harga cuplikan yang
// sama seperti pencatatUji tetapi mengembalikan kendali atas Store supaya
// test bisa menahan Insert.
func rakitPencatat(t *testing.T, store *penyimpanan, opts ...Option) (*Recorder, *pemakai, *observability.Metrics) {
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
	spend := &pemakai{}
	metrics := observability.NewMetrics()
	r := New(Deps{Store: store, Spend: spend, Prices: prices, Metrics: metrics, Logger: testLogger()}, opts...)
	t.Cleanup(func() {
		ctx, batal := context.WithTimeout(context.Background(), 5*time.Second)
		defer batal()
		_ = r.Close(ctx)
	})
	return r, spend, metrics
}

// eventDenganInput menyalin eventLengkap dengan identitas dan jumlah token
// input berbeda, supaya biaya tiap event bisa dibedakan persis:
// input 1000 → 942000 satuan, 2000 → 1242000, 3000 → 1542000 pada harga
// cuplikan rakitPencatat (lihat TestRecorderMenulisBarisDanMenaikkanAnggaran).
func eventDenganInput(id string, input int) Event {
	ev := eventLengkap()
	ev.RequestID = id
	ev.InputTokens = input
	ev.TotalTokens = input + ev.OutputTokens
	return ev
}

func TestRecorderEventDibuangAntreanPenuhTetapDitagih(t *testing.T) {
	tahan := make(chan struct{})
	store := &penyimpanan{}
	r, spend, metrics := rakitPencatat(t, store, WithQueueSize(1), WithWorkers(1))
	store.mu.Lock()
	store.tahan = tahan
	store.mu.Unlock()

	// ev1 diambil satu-satunya penulis dan tertahan di Insert; ev2 mengisi
	// antrean; ev3 DIBUANG — dan penyelesaian biayanya berjalan sinkron di
	// dalam Record, jadi harus sudah terlihat sebelum antrean dikuras.
	r.Record(context.Background(), eventDenganInput("req-1", 1000))
	// Tunggu sampai penulis benar-benar mengambil ev1 (Depth kembali 0)
	// supaya ev2 pasti mengisi antrean, bukan ikut dibuang.
	batasWaktu := time.Now().Add(5 * time.Second)
	for r.Depth() != 0 && time.Now().Before(batasWaktu) {
		time.Sleep(time.Millisecond)
	}
	if r.Depth() != 0 {
		t.Fatal("penulis tidak mengambil ev1 dari antrean")
	}
	r.Record(context.Background(), eventDenganInput("req-2", 2000))
	r.Record(context.Background(), eventDenganInput("req-3", 3000))

	if total := spend.total(); len(total) != 1 || total[0] != upstream.USD(1542000) {
		t.Fatalf("AddSpend sesudah ev3 dibuang = %v, mau tepat satu panggilan 1542000 (biaya ev3)", total)
	}
	if baris := store.semua(); len(baris) != 0 {
		t.Fatalf("baris tertulis sebelum antrean dikuras = %d, mau 0", len(baris))
	}

	close(tahan)
	tutup(t, r)

	// Sesudah kuras: ev1 dan ev2 tertulis dan tertagih lewat jalur normal;
	// ev3 tertagih TEPAT sekali (tidak ganda) dan tidak punya baris.
	baris := store.semua()
	if len(baris) != 2 {
		t.Fatalf("baris tertulis = %d, mau 2 (ev1 dan ev2 saja)", len(baris))
	}
	for _, b := range baris {
		if b.RequestID == "req-3" {
			t.Fatal("ev3 yang dibuang tidak boleh punya baris requests")
		}
	}
	total := spend.total()
	if len(total) != 3 {
		t.Fatalf("AddSpend total = %d panggilan, mau 3 (satu per event)", len(total))
	}
	jumlah := map[upstream.USD]int{}
	for _, v := range total {
		jumlah[v]++
	}
	for _, mau := range []upstream.USD{942000, 1242000, 1542000} {
		if jumlah[mau] != 1 {
			t.Errorf("biaya %d tertagih %d kali, mau tepat 1", mau, jumlah[mau])
		}
	}

	teks := scrape(t, metrics)
	if v := nilaiMetrik(t, teks, "routex_gateway_cost_usd_total{"); math.Abs(v-0.03726) > 1e-9 {
		t.Errorf("routex_gateway_cost_usd_total = %v, mau 0.03726 (ketiga event, termasuk yang dibuang)\n%s", v, teks)
	}
	if !strings.Contains(teks, `routex_usage_records_total{outcome="dropped"} 1`) {
		t.Errorf("outcome dropped harus tetap terhitung jujur\n%s", teks)
	}
	if !strings.Contains(teks, `routex_usage_records_total{outcome="written"} 2`) {
		t.Errorf("outcome written harus 2\n%s", teks)
	}
}

func TestRecorderEventSesudahCloseTetapDitagih(t *testing.T) {
	store := &penyimpanan{}
	r, spend, metrics := rakitPencatat(t, store)
	tutup(t, r)

	// Pencatat sudah ditutup: catatan dibuang, tetapi biayanya nyata dan
	// anggaran tetap wajib bergerak — cabang "ditutup" dari Record.
	r.Record(context.Background(), eventDenganInput("req-terlambat", 1000))

	if total := spend.total(); len(total) != 1 || total[0] != upstream.USD(942000) {
		t.Fatalf("AddSpend = %v, mau tepat satu panggilan 942000", total)
	}
	if baris := store.semua(); len(baris) != 0 {
		t.Fatalf("baris tertulis = %d, mau 0", len(baris))
	}
	if teks := scrape(t, metrics); !strings.Contains(teks, `routex_usage_records_total{outcome="dropped"} 1`) {
		t.Errorf("outcome dropped harus 1\n%s", teks)
	}
}

func TestRecorderEventDibuangTanpaHargaTidakMenyentuhAnggaran(t *testing.T) {
	store := &penyimpanan{}
	r, spend, metrics := rakitPencatat(t, store)
	tutup(t, r)

	// Pemetaan tanpa harga: biaya nol adalah jawaban yang benar dari
	// kalkulator yang sama — tidak ada AddSpend, tetapi ketiadaan harganya
	// tetap terhitung di metrik unpriced seperti jalur normal.
	ev := eventDenganInput("req-tanpa-harga", 1000)
	ev.ProviderModelID = "pm-tanpa-harga"
	r.Record(context.Background(), ev)

	if total := spend.total(); len(total) != 0 {
		t.Fatalf("AddSpend = %v, mau tidak ada untuk biaya nol", total)
	}
	if teks := scrape(t, metrics); nilaiMetrik(t, teks, "routex_usage_unpriced_total{") != 1 {
		t.Errorf("routex_usage_unpriced_total harus 1 untuk event dibuang tanpa harga\n%s", teks)
	}
}
