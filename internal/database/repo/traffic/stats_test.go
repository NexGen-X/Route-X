package traffic

import (
	"testing"
	"time"
)

func TestIntegrationSummaryDanSeries(t *testing.T) {
	ctx, pool, r := testEnv(t)
	jam := jamUji(t)
	ttft := 25

	sisipRequest(t, ctx, pool, sisip{
		prefix: "a", jumlah: 6, at: jam.Add(5 * time.Minute),
		providerID: "11111111-1111-1111-1111-111111111111", providerName: "smoke",
		modelID: "22222222-2222-2222-2222-222222222222", modelName: "gpt-5",
		status: 200, stream: true, input: 100, output: 20, total: 120,
		costUnits: 2_000, latencyMS: 100, ttftMS: &ttft,
	})
	sisipRequest(t, ctx, pool, sisip{
		prefix: "b", jumlah: 4, at: jam.Add(65 * time.Minute),
		providerID: "11111111-1111-1111-1111-111111111111", providerName: "smoke",
		modelID: "22222222-2222-2222-2222-222222222222", modelName: "gpt-5",
		status: 429, errorType: "rate_limit", latencyMS: 5,
	})

	q := Query{From: jam, To: jam.Add(3 * time.Hour), Source: SourceRequests}
	s, err := r.Summary(ctx, q)
	if err != nil {
		t.Fatalf("Summary: %v", err)
	}
	if s.Source != SourceRequests {
		t.Errorf("Source = %q, mau %q", s.Source, SourceRequests)
	}
	if s.Requests != 10 || s.Success != 6 || s.Errors != 4 {
		t.Errorf("request/sukses/gagal = %d/%d/%d, mau 10/6/4", s.Requests, s.Success, s.Errors)
	}
	if s.CostUSD.Units() != 12_000 {
		t.Errorf("biaya = %s (%d satuan), mau 12000 satuan", s.CostUSD, s.CostUSD.Units())
	}
	if s.TTFT.Count != 6 {
		t.Errorf("cacah TTFT = %d, mau 6", s.TTFT.Count)
	}
	if rasio, ada := s.Availability(); !ada || rasio != 0.6 {
		t.Errorf("Availability = %v, %v; mau 0.6", rasio, ada)
	}
	if s.Latency.P95 == nil {
		t.Fatal("p95 latensi nil padahal ada sampel")
	}
	if got := s.Latency.AvgMS(); got != 62 {
		t.Errorf("rata-rata latensi = %v, mau 62", got)
	}

	// Granularitas jam: dua titik, karena barisnya jatuh di dua jam berbeda.
	q.Granularity = GranularityHour
	titik, err := r.Series(ctx, q)
	if err != nil {
		t.Fatalf("Series jam: %v", err)
	}
	if len(titik) != 2 {
		t.Fatalf("titik = %d, mau 2", len(titik))
	}
	if !titik[0].Bucket.Equal(jam) || titik[0].Requests != 6 {
		t.Errorf("titik pertama = %s / %d", titik[0].Bucket.UTC(), titik[0].Requests)
	}
	if titik[1].Errors != 4 {
		t.Errorf("titik kedua gagal = %d, mau 4", titik[1].Errors)
	}

	// Granularitas hari: satu titik yang menggabungkan keduanya.
	q.Granularity = GranularityDay
	titik, err = r.Series(ctx, q)
	if err != nil {
		t.Fatalf("Series hari: %v", err)
	}
	if len(titik) != 1 || titik[0].Requests != 10 {
		t.Fatalf("titik harian = %v", titik)
	}
}

func TestIntegrationSummaryRentangKosongMengembalikanNolBukanError(t *testing.T) {
	ctx, _, r := testEnv(t)

	s, err := r.Summary(ctx, Query{
		From:   time.Date(2020, 1, 1, 0, 0, 0, 0, time.UTC),
		To:     time.Date(2020, 1, 2, 0, 0, 0, 0, time.UTC),
		Source: SourceRequests,
	})
	if err != nil {
		t.Fatalf("Summary: %v", err)
	}
	if s.Requests != 0 {
		t.Errorf("request = %d, mau 0", s.Requests)
	}
	// Persentil nil, bukan nol: "tidak ada data" dan "nol milidetik" adalah dua hal berbeda,
	// dan grafik yang menggambar nol untuk yang pertama menyesatkan pembacanya.
	if s.Latency.P95 != nil || s.TTFT.P50 != nil {
		t.Errorf("persentil terisi pada rentang kosong: %v / %v", s.Latency.P95, s.TTFT.P50)
	}
	if _, ada := s.Availability(); ada {
		t.Error("Availability melaporkan ada data pada rentang kosong")
	}
}

// INI aturan yang paling menentukan di seluruh jalur agregasi: persentil rentang gabungan
// wajib lewat histogram, bukan rata-rata persentil per bucket. Test ini menyusun data yang
// bentuknya biasa untuk gateway — banyak permintaan cepat, beberapa yang menggantung — lalu
// menunjukkan ketiga angkanya sekaligus.
func TestIntegrationPersentilGabunganBukanRataRataPersentil(t *testing.T) {
	ctx, pool, r := testEnv(t)
	jam := jamUji(t)

	sisipRequest(t, ctx, pool, sisip{
		prefix: "cepat", jumlah: 1000, at: jam.Add(10 * time.Minute),
		providerName: "smoke", status: 200, total: 10, latencyMS: 80,
	})
	sisipRequest(t, ctx, pool, sisip{
		prefix: "lambat", jumlah: 10, at: jam.Add(70 * time.Minute),
		providerName: "smoke", status: 200, total: 10, latencyMS: 60000,
	})

	if _, _, err := r.RollupRange(ctx, jam, jam.Add(2*time.Hour)); err != nil {
		t.Fatalf("RollupRange: %v", err)
	}

	q := Query{From: jam, To: jam.Add(3 * time.Hour)}

	q.Source = SourceRequests
	mentah, err := r.Summary(ctx, q)
	if err != nil {
		t.Fatalf("Summary requests: %v", err)
	}
	if mentah.Latency.P95 == nil || *mentah.Latency.P95 != 80 {
		t.Fatalf("p95 persis dari requests = %v, mau 80", mentah.Latency.P95)
	}

	q.Source = SourceHourly
	q.Granularity = GranularityHour
	gabungan, err := r.Summary(ctx, q)
	if err != nil {
		t.Fatalf("Summary hourly: %v", err)
	}
	if gabungan.Latency.P95 == nil {
		t.Fatal("p95 gabungan nil")
	}
	// Histogram menempatkan 80 ms di slot [60, 90), jadi hasil interpolasinya harus tetap di
	// dalam slot itu — beberapa milidetik dari angka sebenarnya, bukan beberapa detik.
	if *gabungan.Latency.P95 < 60 || *gabungan.Latency.P95 >= 90 {
		t.Errorf("p95 histogram gabungan = %v ms, mau di dalam slot [60, 90)", *gabungan.Latency.P95)
	}

	// Angka yang akan muncul kalau persentil per bucket dirata-ratakan. Dibaca langsung dari
	// kolomnya supaya besarnya kesalahan terlihat di keluaran test, bukan hanya diklaim.
	var rataP95 float64
	if err := pool.QueryRow(ctx, `select avg(latency_p95_ms)::float8 from usage_hourly`).Scan(&rataP95); err != nil {
		t.Fatalf("membaca rata-rata p95: %v", err)
	}
	if rataP95 < 10000 {
		t.Fatalf("rata-rata p95 per bucket = %v ms; test ini kehilangan gunanya kalau kesalahannya tidak lagi besar", rataP95)
	}
	t.Logf("p95 persis %v ms, histogram gabungan %v ms, rata-rata p95 per bucket %v ms",
		*mentah.Latency.P95, *gabungan.Latency.P95, rataP95)
}

func TestIntegrationBreakdownPerProvider(t *testing.T) {
	ctx, pool, r := testEnv(t)
	jam := jamUji(t)

	sisipRequest(t, ctx, pool, sisip{
		prefix: "besar", jumlah: 7, at: jam.Add(time.Minute),
		providerID: "11111111-1111-1111-1111-111111111111", providerName: "openai",
		status: 200, total: 100, latencyMS: 50, costUnits: 1_000,
	})
	sisipRequest(t, ctx, pool, sisip{
		prefix: "kecil", jumlah: 3, at: jam.Add(2 * time.Minute),
		providerID: "33333333-3333-3333-3333-333333333333", providerName: "anthropic",
		status: 500, errorType: "server", latencyMS: 10,
	})
	// Permintaan yang ditolak sebelum satu pun provider terpilih.
	sisipRequest(t, ctx, pool, sisip{
		prefix: "tolak", jumlah: 2, at: jam.Add(3 * time.Minute),
		status: 400, errorType: "content_filter", latencyMS: 1,
	})

	potongan, err := r.Breakdown(ctx, Query{From: jam, To: jam.Add(time.Hour), Source: SourceRequests}, DimProvider, 10)
	if err != nil {
		t.Fatalf("Breakdown: %v", err)
	}
	if len(potongan) != 3 {
		t.Fatalf("potongan = %d, mau 3", len(potongan))
	}
	if potongan[0].Name != "openai" || potongan[0].Requests != 7 {
		t.Errorf("potongan pertama = %+v, mau openai dengan 7 request", potongan[0])
	}
	// Potongan tanpa provider dilaporkan apa adanya: request yang ditolak lebih awal tetap
	// request yang terjadi, dan menghilangkannya membuat jumlah potongan tidak lagi sama
	// dengan total.
	var adaTanpaProvider bool
	for _, p := range potongan {
		if p.ID == "" && p.Requests == 2 {
			adaTanpaProvider = true
		}
	}
	if !adaTanpaProvider {
		t.Error("potongan tanpa provider hilang dari hasil")
	}

	if _, err := r.Breakdown(ctx, Query{Source: SourceRequests}, Dimension("prompt"), 10); err == nil {
		t.Error("dimensi tak dikenal diterima; nama kolom tidak boleh datang dari pemanggil")
	}
}

func TestPemilihanSumberDanGranularitasOtomatis(t *testing.T) {
	akhir := time.Date(2026, 9, 3, 12, 0, 0, 0, time.UTC)

	tests := []struct {
		nama     string
		q        Query
		source   Source
		gran     Granularity
		errorkan bool
	}{
		{
			nama:   "rentang pendek memakai baris mentah dan bucket jam",
			q:      Query{From: akhir.Add(-6 * time.Hour), To: akhir},
			source: SourceRequests, gran: GranularityHour,
		},
		{
			nama:   "rentang beberapa hari masih mentah tetapi bucket harian",
			q:      Query{From: akhir.Add(-5 * 24 * time.Hour), To: akhir},
			source: SourceRequests, gran: GranularityDay,
		},
		{
			nama:   "rentang panjang pindah ke ringkasan harian",
			q:      Query{From: akhir.Add(-40 * 24 * time.Hour), To: akhir},
			source: SourceDaily, gran: GranularityDay,
		},
		{
			nama:     "granularitas jam pada ringkasan harian ditolak",
			q:        Query{From: akhir.Add(-2 * time.Hour), To: akhir, Source: SourceDaily, Granularity: GranularityHour},
			errorkan: true,
		},
		{
			nama:     "sumber tak dikenal ditolak",
			q:        Query{From: akhir.Add(-time.Hour), To: akhir, Source: Source("materialized")},
			errorkan: true,
		},
		{
			nama:     "rentang terbalik ditolak",
			q:        Query{From: akhir, To: akhir.Add(-time.Hour)},
			errorkan: true,
		},
	}

	for _, tc := range tests {
		t.Run(tc.nama, func(t *testing.T) {
			p, err := rencanakan("uji", tc.q)
			if tc.errorkan {
				if err == nil {
					t.Fatalf("mau error, dapat rencana %+v", p)
				}
				return
			}
			if err != nil {
				t.Fatalf("rencanakan: %v", err)
			}
			if p.source != tc.source {
				t.Errorf("sumber = %q, mau %q", p.source, tc.source)
			}
			if p.gran != tc.gran {
				t.Errorf("granularitas = %q, mau %q", p.gran, tc.gran)
			}
		})
	}
}

func TestIntegrationLatencyByProviderModel(t *testing.T) {
	ctx, pool, r := testEnv(t)

	var providerID, modelID, pmID string
	if err := pool.QueryRow(ctx, `
		insert into providers (name, display_name, kind, base_url)
		values ('smoke', 'Smoke', 'openai_compatible', 'http://127.0.0.1:8123') returning id::text`).
		Scan(&providerID); err != nil {
		t.Fatalf("menyisipkan provider: %v", err)
	}
	if err := pool.QueryRow(ctx, `
		insert into models (model_id, display_name) values ('gpt-5', 'GPT-5') returning id::text`).
		Scan(&modelID); err != nil {
		t.Fatalf("menyisipkan model: %v", err)
	}
	if err := pool.QueryRow(ctx, `
		insert into provider_models (model_id, provider_id, upstream_model_name)
		values ($1, $2, 'gpt-5-2026') returning id::text`, modelID, providerID).Scan(&pmID); err != nil {
		t.Fatalf("menyisipkan pemetaan: %v", err)
	}

	baru := time.Now().Add(-5 * time.Minute)
	ttft := 50
	sisipRequest(t, ctx, pool, sisip{
		prefix: "ok", jumlah: 20, at: baru,
		providerID: providerID, providerName: "smoke", modelID: modelID, modelName: "gpt-5",
		status: 200, stream: true, total: 10, latencyMS: 30000, ttftMS: &ttft,
	})
	// Kegagalan cepat: kalau ikut dihitung, provider yang menolak setiap permintaan dalam
	// 5 ms menjadi yang tercepat menurut angka — dan seluruh lalu lintas dikirim ke sana.
	sisipRequest(t, ctx, pool, sisip{
		prefix: "gagal", jumlah: 40, at: baru,
		providerID: providerID, providerName: "smoke", modelID: modelID, modelName: "gpt-5",
		status: 502, errorType: "server", latencyMS: 5,
	})

	got, err := r.LatencyByProviderModel(ctx, time.Hour, 0.95, 10)
	if err != nil {
		t.Fatalf("LatencyByProviderModel: %v", err)
	}
	// Yang diukur coalesce(ttft_ms, latency_ms): pada respons streaming yang ditunggu
	// pengguna adalah token pertama, bukan panjang jawabannya.
	if d, ok := got[pmID]; !ok || d != 50*time.Millisecond {
		t.Fatalf("latensi pemetaan = %v (%v), mau 50ms", d, ok)
	}

	// Sampel minimum yang belum terpenuhi berarti pemetaan itu TIDAK muncul: p95 dari
	// beberapa sampel bukan pengukuran, dan pemanggil harus mengurutkannya paling belakang.
	got, err = r.LatencyByProviderModel(ctx, time.Hour, 0.95, 25)
	if err != nil {
		t.Fatalf("LatencyByProviderModel minSamples besar: %v", err)
	}
	if _, ok := got[pmID]; ok {
		t.Error("pemetaan dengan sampel di bawah minimum tetap dilaporkan terukur")
	}

	if _, err := r.LatencyByProviderModel(ctx, 0, 0.95, 10); err == nil {
		t.Error("jendela nol diterima")
	}
	if _, err := r.LatencyByProviderModel(ctx, time.Hour, 1.5, 10); err == nil {
		t.Error("persentil di luar (0,1] diterima")
	}
}
