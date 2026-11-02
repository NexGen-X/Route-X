package worker

import (
	"context"
	"errors"
	"fmt"
	"strings"
	"testing"
	"time"

	"github.com/jackc/pgx/v5/pgxpool"

	"github.com/NexGen-X/Route-X/internal/database/repo/traffic"
	"github.com/NexGen-X/Route-X/internal/database/repo/upstream"
	"github.com/NexGen-X/Route-X/internal/providers"
)

// --- Health checker paralel (V-DT-11) ---------------------------------------

// providerLambat adalah provider semu yang HealthCheck-nya benar-benar
// MEMBLOKIR selama tunda, meniru provider blackhole yang membakar timeout.
type providerLambat struct {
	dummyProvider
	tunda time.Duration
}

func (p *providerLambat) HealthCheck(ctx context.Context) providers.HealthResult {
	select {
	case <-time.After(p.tunda):
	case <-ctx.Done():
	}
	return providers.HealthResult{Healthy: true, Latency: p.tunda}
}

type pabrikLambat struct{ tunda time.Duration }

func (f *pabrikLambat) ProviderFor(context.Context, *upstream.Provider) (providers.Provider, error) {
	return &providerLambat{tunda: f.tunda}, nil
}

// TestHealthCheckerBerjalanParalel mereproduksi audit3 V-DT-11: versi
// sekuensial membuat satu putaran berdurasi jumlah seluruh timeout — dua
// belas provider lambat @400ms = 4,8 detik. Dengan paralel terbatas,
// putaran yang sama selesai dalam waktu ±dua gelombang.
func TestHealthCheckerBerjalanParalel(t *testing.T) {
	ctx := context.Background()

	const n = 12
	provs := make([]*upstream.Provider, n)
	for i := range provs {
		provs[i] = &upstream.Provider{
			ID: fmt.Sprintf("p-%d", i), Name: fmt.Sprintf("penyedia-%d", i),
			Enabled: true, TimeoutMS: 5000,
		}
	}
	src := &dummyProviderSource{providers: provs}
	job := NewHealthCheckerJob(nil, src, &pabrikLambat{tunda: 400 * time.Millisecond}, &dummyEnqueuer{}, nil, nil)

	mulai := time.Now()
	if err := job.Run(ctx); err != nil {
		t.Fatalf("Run: %v", err)
	}
	elapsed := time.Since(mulai)

	if elapsed > 2400*time.Millisecond {
		t.Errorf("satu putaran memakan %v — probe tampak sekuensial (12 × 400ms = 4,8 detik)", elapsed)
	}
	src.mu.Lock()
	jumlah := len(src.reports)
	src.mu.Unlock()
	if jumlah != n {
		t.Errorf("laporan kesehatan tercatat = %d, mau %d (semua provider tetap diperiksa)", jumlah, n)
	}
}

// --- Status jujur RetentionWorker (V-DT-09) ---------------------------------

type rollupRepoGagal struct{ dummyRollupRepo }

func (*rollupRepoGagal) RollupRange(context.Context, time.Time, time.Time) (int64, int64, error) {
	return 0, 0, errors.New("rollup meledak")
}

// TestRetentionWorkerMengembalikanErrorSaatLangkahGagal mereproduksi
// audit3 V-DT-09: versi lama menelan kegagalan setiap langkah dan
// mengembalikan nil, jadi supervisor mencatat putaran gagal sebagai sukses.
func TestRetentionWorkerMengembalikanErrorSaatLangkahGagal(t *testing.T) {
	pool := testPool(t)
	ctx := context.Background()

	w := NewRetentionWorker(pool, &dummyRetentionCleaner{}, &rollupRepoGagal{}, 30, 7, 365, nil)
	err := w.Run(ctx)
	if err == nil {
		t.Fatal("Run mengembalikan nil padahal rollup pra-retensi gagal — status sukses palsu")
	}
	if !strings.Contains(err.Error(), "rollup pra-retensi") {
		t.Errorf("error = %v, mau menyebut langkah yang gagal", err)
	}
}

// --- Perkakas partisi untuk test retensi ------------------------------------

func awalBulan(t time.Time) time.Time {
	return time.Date(t.Year(), t.Month(), 1, 0, 0, 0, 0, time.UTC)
}

func namaPartisi(parent string, bulan time.Time) string {
	return fmt.Sprintf("%s_%04d%02d", parent, bulan.Year(), int(bulan.Month()))
}

func buatPartisi(t *testing.T, ctx context.Context, pool *pgxpool.Pool, parent string, bulan time.Time) {
	t.Helper()
	if _, err := pool.Exec(ctx, `select create_monthly_partition($1, $2::date)`, parent, bulan); err != nil {
		t.Fatalf("membuat partisi %s %s: %v", parent, bulan.Format("2006-01"), err)
	}
}

func partisiAda(t *testing.T, ctx context.Context, pool *pgxpool.Pool, nama string) bool {
	t.Helper()
	var ada bool
	if err := pool.QueryRow(ctx, `select to_regclass($1) is not null`, nama).Scan(&ada); err != nil {
		t.Fatalf("memeriksa partisi %s: %v", nama, err)
	}
	return ada
}

func sisipRequestMentah(t *testing.T, ctx context.Context, pool *pgxpool.Pool, id string, at time.Time) {
	t.Helper()
	_, err := pool.Exec(ctx, `
		insert into requests (
			request_id, method, endpoint, requested_model,
			api_key_id, api_key_name, model_id, model_name, provider_id, provider_name,
			status_code, error_type, stream,
			input_tokens, output_tokens, cached_input_tokens, reasoning_tokens, total_tokens,
			cost_usd, latency_ms, ttft_ms, retry_count, failover_count, created_at
		) values (
			$1, 'POST', '/v1/chat/completions', 'gpt-5',
			null, null, null, 'gpt-5', null, 'smoke',
			200, null, false,
			10, 5, 0, 0, 15,
			0, 100, null, 0, 0, $2
		)`, id, at)
	if err != nil {
		t.Fatalf("menyisipkan request %s: %v", id, err)
	}
}

// TestRetentionMembuangPartisiPayloadsDanAuditLogs mereproduksi separuh
// audit3 V-DT-07 (partisi request_payloads tidak pernah di-DROP, hanya
// barisnya yang di-DELETE) dan V-DT-20 (audit_logs tanpa retensi).
func TestRetentionMembuangPartisiPayloadsDanAuditLogs(t *testing.T) {
	pool := testPool(t)
	ctx := context.Background()
	now := time.Now().UTC()

	// Partisi payload 40 hari lalu: melewati retensi body 7 hari.
	bulanPayload := awalBulan(now.AddDate(0, 0, -40))
	buatPartisi(t, ctx, pool, "request_payloads", bulanPayload)
	if _, err := pool.Exec(ctx, `
		insert into request_payloads (request_pk, created_at, request_body)
		values (gen_random_uuid(), $1, '{"uji":true}'::jsonb)`, bulanPayload.AddDate(0, 0, 9)); err != nil {
		t.Fatalf("menyisipkan payload: %v", err)
	}

	// Baris audit 400 hari lalu (dihapus; retensi 365) dan 10 hari lalu (disimpan).
	for _, usia := range []int{400, 10} {
		if _, err := pool.Exec(ctx, `
			insert into audit_logs (action, resource_type, occurred_at)
			values ('uji.aksi', 'uji', $1)`, now.AddDate(0, 0, -usia)); err != nil {
			t.Fatalf("menyisipkan audit_logs: %v", err)
		}
	}

	w := NewRetentionWorker(pool, &dummyRetentionCleaner{}, &dummyRollupRepo{}, 30, 7, 365, nil)
	if err := w.Run(ctx); err != nil {
		t.Fatalf("Run retensi: %v", err)
	}

	if partisiAda(t, ctx, pool, namaPartisi("request_payloads", bulanPayload)) {
		t.Error("partisi request_payloads kedaluwarsa masih ada — partisinya tidak pernah dibuang")
	}
	var sisaAudit int
	if err := pool.QueryRow(ctx, `select count(*) from audit_logs where action = 'uji.aksi'`).Scan(&sisaAudit); err != nil {
		t.Fatalf("menghitung audit_logs: %v", err)
	}
	if sisaAudit != 1 {
		t.Errorf("baris audit_logs tersisa = %d, mau 1 (yang 400 hari terhapus, yang 10 hari disimpan)", sisaAudit)
	}
}

// TestRetentionMenolakDropHourlySaatHarianTakLengkap mereproduksi
// audit3 V-DT-04: usage_hourly dibuang murni berdasarkan umur padahal
// agregat hariannya belum ada — bahan perbaikan hari itu terkubur permanen.
func TestRetentionMenolakDropHourlySaatHarianTakLengkap(t *testing.T) {
	pool := testPool(t)
	ctx := context.Background()
	now := time.Now().UTC()

	bulan := awalBulan(now.AddDate(0, 0, -70))
	hari := bulan.AddDate(0, 0, 9) // tanggal 10 bulan itu, tengah malam UTC
	buatPartisi(t, ctx, pool, "usage_hourly", bulan)
	if _, err := pool.Exec(ctx, `
		insert into usage_hourly (bucket, request_count, success_count)
		values ($1, 5, 5)`, hari.Add(10*time.Hour)); err != nil {
		t.Fatalf("menyisipkan usage_hourly: %v", err)
	}

	w := NewRetentionWorker(pool, &dummyRetentionCleaner{}, &dummyRollupRepo{}, 30, 7, 365, nil)
	// Putaran pertama WAJIB berstatus gagal: penjaga menolak drop, dan sejak
	// V-DT-09 penolakan itu adalah kegagalan putaran, bukan sukses semu.
	if err := w.Run(ctx); err == nil {
		t.Fatal("Run retensi pertama mengembalikan nil padahal penjaga harian menolak drop")
	}
	if !partisiAda(t, ctx, pool, namaPartisi("usage_hourly", bulan)) {
		t.Fatal("partisi usage_hourly dibuang padahal usage_daily hari itu kosong — penjaga harian tidak bekerja")
	}

	// Lengkapi hariannya: kini drop aman dan harus terjadi.
	if _, err := pool.Exec(ctx, `
		insert into usage_daily (bucket, request_count, success_count)
		values ($1, 5, 5)`, hari); err != nil {
		t.Fatalf("menyisipkan usage_daily: %v", err)
	}
	if err := w.Run(ctx); err != nil {
		t.Fatalf("Run retensi kedua: %v", err)
	}
	if partisiAda(t, ctx, pool, namaPartisi("usage_hourly", bulan)) {
		t.Error("partisi usage_hourly tetap ada padahal hariannya sudah lengkap")
	}
}

// TestRetentionEventsMengikutiRequests mereproduksi audit3 V-DT-05:
// request_events dibuang independen dari requests, sehingga jejak per
// request bisa hilang sementara request-nya sendiri masih disimpan.
func TestRetentionEventsMengikutiRequests(t *testing.T) {
	pool := testPool(t)
	ctx := context.Background()
	now := time.Now().UTC()

	bulan := awalBulan(now.AddDate(0, 0, -70))
	saat := bulan.AddDate(0, 0, 4).Add(10 * time.Hour) // tanggal 5, jam 10:00
	buatPartisi(t, ctx, pool, "requests", bulan)
	buatPartisi(t, ctx, pool, "request_events", bulan)
	buatPartisi(t, ctx, pool, "usage_hourly", bulan)
	sisipRequestMentah(t, ctx, pool, "uji-events-1", saat)

	w := NewRetentionWorker(pool, &dummyRetentionCleaner{}, &dummyRollupRepo{}, 30, 7, 365, nil)
	// Putaran pertama berstatus gagal (penjaga requests menolak drop).
	if err := w.Run(ctx); err == nil {
		t.Fatal("Run retensi pertama mengembalikan nil padahal penjaga requests menolak drop")
	}
	// Request belum ter-rollup: partisinya ditolak penjaga, dan events
	// bulan yang sama WAJIB ikut tertahan.
	if !partisiAda(t, ctx, pool, namaPartisi("requests", bulan)) {
		t.Fatal("partisi requests dibuang padahal jamnya belum ter-rollup")
	}
	if !partisiAda(t, ctx, pool, namaPartisi("request_events", bulan)) {
		t.Error("partisi request_events dibuang sementara partisi requests bulan yang sama masih ada")
	}

	// Rollup jam itu secara manual (per jam + hariannya), lalu putaran
	// berikutnya membuang requests dan events (requests lebih dulu, events
	// mengikuti di putaran yang sama).
	jam := time.Date(saat.Year(), saat.Month(), saat.Day(), saat.Hour(), 0, 0, 0, time.UTC)
	if _, err := pool.Exec(ctx, `
		insert into usage_hourly (bucket, request_count, success_count)
		values ($1, 1, 1)`, jam); err != nil {
		t.Fatalf("menyisipkan usage_hourly: %v", err)
	}
	if _, err := pool.Exec(ctx, `
		insert into usage_daily (bucket, request_count, success_count)
		values ($1, 1, 1)`, awalBulan(saat).AddDate(0, 0, saat.Day()-1)); err != nil {
		t.Fatalf("menyisipkan usage_daily: %v", err)
	}
	if err := w.Run(ctx); err != nil {
		t.Fatalf("Run retensi kedua: %v", err)
	}
	if partisiAda(t, ctx, pool, namaPartisi("requests", bulan)) {
		t.Error("partisi requests tetap ada padahal jamnya sudah ter-rollup")
	}
	if partisiAda(t, ctx, pool, namaPartisi("request_events", bulan)) {
		t.Error("partisi request_events tetap ada padahal partisi requests pendampingnya sudah dibuang")
	}
}

// TestRollupWorkerMengejarKetertinggalan mereproduksi audit3 V-DT-07/V-DT-08
// (jam tak-ter-rollup lebih tua dari jendela tetap 45 hari tidak pernah
// pulih) dan V-DT-04 (hari tak-lengkap tidak terdeteksi di mana pun).
func TestRollupWorkerMengejarKetertinggalan(t *testing.T) {
	pool := testPool(t)
	ctx := context.Background()
	now := time.Now().UTC()

	// (a) Request 50 hari lalu yang tidak pernah ter-rollup.
	bulanLama := awalBulan(now.AddDate(0, 0, -50))
	saat := bulanLama.AddDate(0, 0, 4).Add(10*time.Hour + 15*time.Minute)
	buatPartisi(t, ctx, pool, "requests", bulanLama)
	sisipRequestMentah(t, ctx, pool, "uji-rollup-1", saat)
	sisipRequestMentah(t, ctx, pool, "uji-rollup-2", saat.Add(time.Minute))
	sisipRequestMentah(t, ctx, pool, "uji-rollup-3", saat.Add(2*time.Minute))

	// (b) Hari 10 hari lalu: usage_hourly ada, usage_daily kosong.
	hariBasi := awalBulan(now).AddDate(0, 0, -10)
	buatPartisi(t, ctx, pool, "usage_hourly", awalBulan(hariBasi))
	if _, err := pool.Exec(ctx, `
		insert into usage_hourly (bucket, request_count, success_count)
		values ($1, 4, 4)`, hariBasi.Add(9*time.Hour)); err != nil {
		t.Fatalf("menyisipkan usage_hourly: %v", err)
	}

	w := NewRollupWorker(pool, traffic.New(pool), nil)
	if err := w.Run(ctx); err != nil {
		t.Fatalf("Run rollup: %v", err)
	}

	jam := time.Date(saat.Year(), saat.Month(), saat.Day(), saat.Hour(), 0, 0, 0, time.UTC)
	var hitungan int64
	if err := pool.QueryRow(ctx, `
		select coalesce(sum(request_count), 0) from usage_hourly where bucket = $1`, jam).Scan(&hitungan); err != nil {
		t.Fatalf("membaca usage_hourly hasil pemulihan: %v", err)
	}
	if hitungan != 3 {
		t.Errorf("usage_hourly jam 50 hari lalu = %d, mau 3 — pemulihan tidak menjangkau lebih dari 45 hari", hitungan)
	}

	var harian int64
	if err := pool.QueryRow(ctx, `
		select coalesce(sum(request_count), 0) from usage_daily where bucket = $1`, hariBasi).Scan(&harian); err != nil {
		t.Fatalf("membaca usage_daily hasil perbaikan: %v", err)
	}
	if harian != 4 {
		t.Errorf("usage_daily hari basi = %d, mau 4 — hari tak-lengkap tidak terdeteksi/diperbaiki", harian)
	}
}

// TestRetentionRelokasiBarisDefault mereproduksi audit3 V-DT-06 lewat jalur
// worker: baris yang mengendap di partisi DEFAULT harus dipindahkan ke
// partisi bulannya oleh retensi lalu dibuang lewat jalur terpenjaga —
// bukan mengendap selamanya.
func TestRetentionRelokasiBarisDefault(t *testing.T) {
	pool := testPool(t)
	ctx := context.Background()
	now := time.Now().UTC()

	bulan := awalBulan(now.AddDate(0, 0, -70))
	saat := bulan.AddDate(0, 0, 4).Add(11 * time.Hour)
	// Baris payload mendarat di DEFAULT karena partisi bulannya belum ada.
	if _, err := pool.Exec(ctx, `
		insert into request_payloads (request_pk, created_at, request_body)
		values (gen_random_uuid(), $1, '{"nyasar":true}'::jsonb)`, saat); err != nil {
		t.Fatalf("menyisipkan payload nyasar: %v", err)
	}
	var diDefault int
	if err := pool.QueryRow(ctx, `select count(*) from request_payloads_default`).Scan(&diDefault); err != nil {
		t.Fatalf("menghitung baris default: %v", err)
	}
	if diDefault != 1 {
		t.Fatalf("baris di partisi DEFAULT = %d, mau 1 (prasyarat uji)", diDefault)
	}

	w := NewRetentionWorker(pool, &dummyRetentionCleaner{}, &dummyRollupRepo{}, 30, 7, 365, nil)
	if err := w.Run(ctx); err != nil {
		t.Fatalf("Run retensi: %v", err)
	}

	if err := pool.QueryRow(ctx, `select count(*) from request_payloads_default`).Scan(&diDefault); err != nil {
		t.Fatalf("menghitung ulang baris default: %v", err)
	}
	if diDefault != 0 {
		t.Errorf("baris di partisi DEFAULT sesudah retensi = %d, mau 0", diDefault)
	}
	if partisiAda(t, ctx, pool, namaPartisi("request_payloads", bulan)) {
		t.Error("partisi hasil relokasi masih ada padahal seluruh rentangnya melewati retensi body")
	}
	var total int
	if err := pool.QueryRow(ctx, `select count(*) from request_payloads`).Scan(&total); err != nil {
		t.Fatalf("menghitung payload: %v", err)
	}
	if total != 0 {
		t.Errorf("total baris payload = %d, mau 0", total)
	}
}
