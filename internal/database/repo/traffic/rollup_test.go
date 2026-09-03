package traffic

import (
	"context"
	"testing"
	"time"

	"github.com/jackc/pgx/v5/pgxpool"
)

// jamUji adalah bucket jam yang dipakai seluruh test di berkas ini.
//
// Waktunya tetap dan rata di UTC supaya perbandingan bucket tidak bergantung pada saat test
// dijalankan. Bulan yang dipilih ada di dalam partisi yang dibuat migrasi.
func jamUji(t *testing.T) time.Time {
	t.Helper()
	n := time.Now().UTC()
	return time.Date(n.Year(), n.Month(), n.Day(), 10, 0, 0, 0, time.UTC)
}

// sisipRequest menyisipkan sejumlah baris requests dengan bentuk yang seragam.
//
// Lewat SQL langsung, bukan lewat Repo.Insert: yang diuji di berkas ini adalah agregasinya,
// dan menyisipkan seribu baris satu per satu lewat Insert menghabiskan waktu test untuk hal
// yang sudah diuji di tempat lain.
func sisipRequest(t *testing.T, ctx context.Context, pool *pgxpool.Pool, args sisip) {
	t.Helper()
	_, err := pool.Exec(ctx, `
		insert into requests (
			request_id, method, endpoint, requested_model,
			api_key_id, api_key_name, model_id, model_name, provider_id, provider_name,
			status_code, error_type, stream,
			input_tokens, output_tokens, cached_input_tokens, reasoning_tokens, total_tokens,
			cost_usd, latency_ms, ttft_ms, retry_count, failover_count, created_at
		)
		select
			$1 || '-' || g, 'POST', '/v1/chat/completions', 'gpt-5',
			$2::uuid, $3, $4::uuid, $5, $6::uuid, $7,
			$8, $9, $10,
			$11, $12, $13, $14, $15,
			($16::bigint / 100000000.0)::numeric(16,8), $17, $18, $19, $20, $21
		from generate_series(1, $22) g`,
		args.prefix,
		nilUUID(args.apiKeyID), nilTeks(args.apiKeyName),
		nilUUID(args.modelID), nilTeks(args.modelName),
		nilUUID(args.providerID), nilTeks(args.providerName),
		args.status, nilTeks(args.errorType), args.stream,
		args.input, args.output, args.cachedInput, args.reasoning, args.total,
		args.costUnits, args.latencyMS, args.ttftMS, args.retry, args.failover, args.at,
		args.jumlah)
	if err != nil {
		t.Fatalf("menyisipkan baris uji: %v", err)
	}
}

type sisip struct {
	prefix string
	jumlah int
	at     time.Time

	apiKeyID, apiKeyName     string
	modelID, modelName       string
	providerID, providerName string

	status    int
	errorType string
	stream    bool

	input, output, cachedInput, reasoning, total int
	costUnits                                    int64
	latencyMS                                    int
	ttftMS                                       *int
	retry, failover                              int
}

func nilTeks(s string) any {
	if s == "" {
		return nil
	}
	return s
}

func nilUUID(s string) any {
	if s == "" {
		return nil
	}
	return s
}

func TestIntegrationRollupHourlyDariRequests(t *testing.T) {
	ctx, pool, r := testEnv(t)
	jam := jamUji(t)
	ttft := 30

	sisipRequest(t, ctx, pool, sisip{
		prefix: "ok", jumlah: 10, at: jam.Add(15 * time.Minute),
		providerID: "11111111-1111-1111-1111-111111111111", providerName: "smoke",
		modelID: "22222222-2222-2222-2222-222222222222", modelName: "gpt-5",
		status: 200, stream: true,
		input: 100, output: 50, cachedInput: 40, reasoning: 20, total: 150,
		costUnits: 1_000, latencyMS: 80, ttftMS: &ttft,
	})
	// Satu baris yang GAGAL karena timeout, dan satu aliran yang terputus setelah sebagian
	// terkirim: status 200, tetapi error_type terisi. Yang kedua inilah alasan sukses dan
	// gagal tidak boleh dihitung dari status saja.
	sisipRequest(t, ctx, pool, sisip{
		prefix: "timeout", jumlah: 2, at: jam.Add(20 * time.Minute),
		providerID: "11111111-1111-1111-1111-111111111111", providerName: "smoke",
		modelID: "22222222-2222-2222-2222-222222222222", modelName: "gpt-5",
		status: 504, errorType: "timeout",
		latencyMS: 60000, retry: 2, failover: 1, total: 0,
	})
	sisipRequest(t, ctx, pool, sisip{
		prefix: "putus", jumlah: 1, at: jam.Add(25 * time.Minute),
		providerID: "11111111-1111-1111-1111-111111111111", providerName: "smoke",
		modelID: "22222222-2222-2222-2222-222222222222", modelName: "gpt-5",
		status: 200, errorType: "stream_aborted", stream: true,
		input: 10, output: 5, total: 15, latencyMS: 900, ttftMS: &ttft,
	})

	n, err := r.RollupHourly(ctx, jam.Add(37*time.Minute))
	if err != nil {
		t.Fatalf("RollupHourly: %v", err)
	}
	if n != 1 {
		t.Fatalf("baris rollup = %d, mau 1", n)
	}

	var (
		bucket                                  time.Time
		req, sukses, gagal, timeout             int64
		retry, failover                         int64
		input, output, cached, reasoning, total int64
		biaya                                   int64
		latCount, latSum                        int64
		latMax, latP95                          *int
		ttftCount                               int64
		histTotal                               int64
	)
	err = pool.QueryRow(ctx, `
		select bucket, request_count, success_count, error_count, timeout_count,
		       retry_count, failover_count,
		       input_tokens, output_tokens, cached_input_tokens, reasoning_tokens, total_tokens,
		       (cost_usd * 100000000)::bigint,
		       latency_count, latency_sum_ms, latency_max_ms, latency_p95_ms,
		       ttft_count, usage_histogram_total(latency_hist)
		from usage_hourly`).Scan(&bucket, &req, &sukses, &gagal, &timeout,
		&retry, &failover, &input, &output, &cached, &reasoning, &total, &biaya,
		&latCount, &latSum, &latMax, &latP95, &ttftCount, &histTotal)
	if err != nil {
		t.Fatalf("membaca usage_hourly: %v", err)
	}

	if !bucket.Equal(jam) {
		t.Errorf("bucket = %s, mau %s", bucket.UTC(), jam)
	}
	if req != 13 || sukses != 10 || gagal != 3 || timeout != 2 {
		t.Errorf("request/sukses/gagal/timeout = %d/%d/%d/%d, mau 13/10/3/2", req, sukses, gagal, timeout)
	}
	// Aliran yang terputus berstatus 200 tetapi tetap harus dihitung gagal — kalau tidak,
	// seluruh kelas kegagalan streaming tidak pernah muncul di dashboard.
	if sukses+gagal != req {
		t.Errorf("sukses + gagal = %d, mau sama dengan request_count %d", sukses+gagal, req)
	}
	if retry != 4 || failover != 2 {
		t.Errorf("retry/failover = %d/%d, mau 4/2", retry, failover)
	}
	if input != 1010 || output != 505 || cached != 400 || reasoning != 200 || total != 1515 {
		t.Errorf("token = %d/%d/%d/%d/%d", input, output, cached, reasoning, total)
	}
	if biaya != 10_000 {
		t.Errorf("cost_usd = %d satuan, mau 10000", biaya)
	}
	if latCount != 13 || histTotal != 13 {
		t.Errorf("latency_count = %d, total histogram = %d; keduanya mau 13", latCount, histTotal)
	}
	if latSum != 10*80+2*60000+900 {
		t.Errorf("latency_sum_ms = %d, mau %d", latSum, 10*80+2*60000+900)
	}
	if latMax == nil || *latMax != 60000 {
		t.Errorf("latency_max_ms = %v, mau 60000", latMax)
	}
	// 13 sampel, p95 jatuh pada nilai terbesar.
	if latP95 == nil || *latP95 != 60000 {
		t.Errorf("latency_p95_ms = %v, mau 60000", latP95)
	}
	// TTFT hanya terisi untuk respons streaming, jadi cacahnya lebih kecil dari request.
	if ttftCount != 11 {
		t.Errorf("ttft_count = %d, mau 11", ttftCount)
	}
}

func TestIntegrationRollupIdempoten(t *testing.T) {
	ctx, pool, r := testEnv(t)
	jam := jamUji(t)

	sisipRequest(t, ctx, pool, sisip{
		prefix: "a", jumlah: 5, at: jam.Add(time.Minute),
		providerName: "smoke", status: 200, total: 100, input: 60, output: 40,
		costUnits: 500, latencyMS: 40,
	})

	for i := range 3 {
		if _, err := r.RollupHourly(ctx, jam); err != nil {
			t.Fatalf("RollupHourly putaran %d: %v", i, err)
		}
		if _, err := r.RollupDaily(ctx, jam); err != nil {
			t.Fatalf("RollupDaily putaran %d: %v", i, err)
		}
	}

	var barisJam, barisHari, totalJam, totalHari int64
	if err := pool.QueryRow(ctx, `
		select (select count(*) from usage_hourly), (select count(*) from usage_daily),
		       (select coalesce(sum(request_count), 0) from usage_hourly),
		       (select coalesce(sum(request_count), 0) from usage_daily)`).
		Scan(&barisJam, &barisHari, &totalJam, &totalHari); err != nil {
		t.Fatalf("membaca hasil: %v", err)
	}

	// Kalau bentuk upsert-nya menambah alih-alih menimpa, angkanya menjadi 15 setelah tiga
	// putaran — dan tidak ada satu pun error yang muncul.
	if barisJam != 1 || barisHari != 1 || totalJam != 5 || totalHari != 5 {
		t.Fatalf("baris jam/hari = %d/%d, total jam/hari = %d/%d; mau 1/1 dan 5/5",
			barisJam, barisHari, totalJam, totalHari)
	}
}

func TestIntegrationRollupRangeMenutupSeluruhJamDanHari(t *testing.T) {
	ctx, pool, r := testEnv(t)
	hari := jamUji(t).Truncate(24 * time.Hour)

	for _, jam := range []int{1, 5, 23} {
		sisipRequest(t, ctx, pool, sisip{
			prefix: "j", jumlah: 2, at: hari.Add(time.Duration(jam) * time.Hour),
			providerName: "smoke", status: 200, total: 10, latencyMS: 20,
		})
	}

	// Batasnya sengaja tidak rata: separuh jam bukan bucket yang bisa disimpan, jadi jam yang
	// memuat batas harus ikut dihitung seluruhnya.
	nJam, nHari, err := r.RollupRange(ctx, hari.Add(30*time.Minute), hari.Add(24*time.Hour))
	if err != nil {
		t.Fatalf("RollupRange: %v", err)
	}
	if nJam != 3 {
		t.Errorf("baris jam = %d, mau 3", nJam)
	}
	if nHari != 1 {
		t.Errorf("baris hari = %d, mau 1", nHari)
	}

	var total int64
	if err := pool.QueryRow(ctx, `select sum(request_count) from usage_daily`).Scan(&total); err != nil {
		t.Fatalf("membaca usage_daily: %v", err)
	}
	if total != 6 {
		t.Errorf("request_count harian = %d, mau 6", total)
	}
}

func TestRollupRangeMenolakRentangKosong(t *testing.T) {
	r := New(nil)
	saat := time.Date(2026, 9, 3, 10, 0, 0, 0, time.UTC)

	// Rentang kosong ditolak SEBELUM satu pernyataan pun dijalankan — Repo di sini bahkan
	// tidak punya koneksi, jadi test ini gagal dengan panic kalau pemeriksaannya hilang.
	if _, _, err := r.RollupRange(context.Background(), saat, saat); err == nil {
		t.Error("rentang kosong diterima")
	}
	if _, _, err := r.RollupRange(context.Background(), saat, saat.Add(-time.Hour)); err == nil {
		t.Error("rentang terbalik diterima")
	}
}
