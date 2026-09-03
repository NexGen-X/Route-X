package traffic

import (
	"context"
	"fmt"
	"strings"
	"time"

	"github.com/NexGen-X/Route-X/internal/database/repo"
	"github.com/NexGen-X/Route-X/internal/database/repo/upstream"
)

// Berkas ini adalah sisi BACA agregasi pemakaian: angka yang dipakai dashboard, API
// admin, dan pemantauan.
//
// # Persentil rentang gabungan tidak boleh dirata-ratakan
//
// Aturan yang membentuk seluruh berkas ini. Persentil bukan besaran aditif: merata-ratakan
// p95 dari 30 baris harian bukan salah sedikit, melainkan salah sama sekali. Terukur pada
// data yang bentuknya biasa untuk gateway — 1.000 sampel cepat dan 10 sampel lambat —
// p95 sebenarnya 88 ms, rata-rata p95 per bucket 4.548 ms, histogram gabungan 88,38 ms.
//
// Karena itu setiap pembacaan rollup di sini melewati usage_histogram_sum(hist) lalu
// usage_histogram_percentile, tanpa pengecualian, dan kolom latency_p95_ms milik baris
// rollup TIDAK PERNAH dibaca lintas baris.
//
// # Dua sumber, dan pilihannya eksplisit
//
// SourceRequests membaca baris mentah: persentilnya PERSIS (percentile_disc), dan itu yang
// diinginkan saat menyelidiki gangguan yang baru terjadi. SourceHourly dan SourceDaily
// membaca ringkasan: persentilnya hasil interpolasi di dalam slot histogram, dan itu yang
// membuat rentang 90 hari bisa dijawab tanpa membaca puluhan juta baris.
//
// Keduanya TIDAK PERNAH dicampur dalam satu jawaban. Mencampurnya berarti separuh grafik
// memakai definisi yang berbeda dari separuh lainnya, dan tidak ada cara membaca angkanya
// tanpa mengetahui di mana batasnya. Harga dari pilihan itu: berpindah sumber bisa
// menggeser p95 sebesar lebar satu slot histogram, dan Stats.Source menyebutkan sumber
// mana yang dipakai supaya pergeseran itu bisa dijelaskan alih-alih ditebak.
//
// Rollup diisi RollupHourly/RollupDaily. Sampai worker Fase 10 menjadwalkannya, kedua
// tabel ringkasan bisa kosong — dan rentang panjang yang dijawab dari tabel kosong
// mengembalikan nol, bukan error. Itulah alasan Stats.Source ada di hasil.

// Source memilih tabel yang dibaca.
type Source string

const (
	// SourceAuto memilih sendiri: requests untuk rentang pendek, usage_daily untuk yang
	// panjang. Batasnya autoSourceLimit.
	SourceAuto Source = ""
	// SourceRequests membaca baris mentah. Persentilnya persis.
	SourceRequests Source = "requests"
	// SourceHourly membaca usage_hourly.
	SourceHourly Source = "hourly"
	// SourceDaily membaca usage_daily.
	SourceDaily Source = "daily"
)

// Granularity adalah lebar satu titik pada Series.
type Granularity string

const (
	// GranularityAuto memilih jam untuk rentang pendek dan hari untuk yang panjang.
	GranularityAuto Granularity = ""
	GranularityHour Granularity = "hour"
	GranularityDay  Granularity = "day"
)

// Batas pemilihan otomatis.
//
// Tujuh hari untuk sumber: itu rentang terpanjang yang masih nyaman dibaca dari baris
// mentah pada volume yang wajar, dan sekaligus rentang terpendek yang sudah pasti punya
// baris harian bila rollup berjalan. Dua hari untuk granularitas: 48 titik per jam masih
// terbaca sebagai grafik, sementara 720 titik untuk 30 hari tidak.
const (
	autoSourceLimit      = 7 * 24 * time.Hour
	autoGranularityLimit = 48 * time.Hour
)

// Dimension adalah pemecah pada Breakdown.
type Dimension string

const (
	DimProvider Dimension = "provider"
	DimModel    Dimension = "model"
	DimAPIKey   Dimension = "api_key"
)

// Query membatasi pembacaan agregat.
type Query struct {
	// From dan To membatasi rentang. Nol berarti DefaultWindow terakhir; batas ini selalu
	// ada karena requests dipartisi waktu dan query tanpa batas menyentuh semua partisi.
	From time.Time
	To   time.Time

	APIKeyID   string
	ProviderID string
	ModelID    string

	// Source dan Granularity kosong berarti dipilih otomatis dari lebar rentang.
	Source      Source
	Granularity Granularity
}

// Distribution adalah sebaran satu besaran waktu dalam milidetik.
//
// Sum dan Count dipisah, bukan disimpan sebagai rata-rata, karena rata-rata dari
// rata-rata memberi bobot sama pada bucket berisi 3 request dan bucket berisi 300 ribu.
// Persentil bertipe pointer: nil berarti TIDAK ADA SAMPEL, yang berbeda dari nol
// milidetik — dan grafik yang menggambar nol untuk "tidak ada data" membuat operator
// menyimpulkan sistemnya sangat cepat justru ketika ia tidak melayani apa pun.
type Distribution struct {
	Count int64
	SumMS int64
	MaxMS *int

	P50 *float64
	P90 *float64
	P95 *float64
	P99 *float64
}

// AvgMS mengembalikan rata-rata milidetik, 0 bila tanpa sampel.
func (d Distribution) AvgMS() float64 {
	if d.Count == 0 {
		return 0
	}
	return float64(d.SumMS) / float64(d.Count)
}

// Totals adalah besaran agregat satu himpunan request.
//
// Token mengikuti konvensi OpenAI seperti kolom requests: InputTokens SUDAH memuat
// CachedInputTokens, dan OutputTokens SUDAH memuat ReasoningTokens. Jadi
// TotalTokens = Input + Output, dan menjumlahkan keempatnya menghitung token cache serta
// token penalaran dua kali.
type Totals struct {
	Requests  int64
	Success   int64
	Errors    int64
	Timeouts  int64
	Retries   int64
	Failovers int64

	InputTokens       int64
	OutputTokens      int64
	CachedInputTokens int64
	ReasoningTokens   int64
	TotalTokens       int64

	CostUSD upstream.USD

	Latency Distribution
	TTFT    Distribution
}

// Availability mengembalikan rasio permintaan yang berhasil, dan apakah ada permintaan
// sama sekali.
//
// Nilai kedua bukan kenyamanan: rasio 0 pada nol permintaan tidak bisa dibedakan dari
// rasio 0 pada seribu permintaan yang semuanya gagal, dan keduanya menuntut tindakan yang
// sangat berbeda dari operator.
func (t Totals) Availability() (float64, bool) {
	if t.Requests == 0 {
		return 0, false
	}
	return float64(t.Success) / float64(t.Requests), true
}

// ErrorRate mengembalikan rasio permintaan yang gagal.
func (t Totals) ErrorRate() (float64, bool) {
	if t.Requests == 0 {
		return 0, false
	}
	return float64(t.Errors) / float64(t.Requests), true
}

// Stats adalah ringkasan satu rentang.
type Stats struct {
	Totals
	From   time.Time
	To     time.Time
	Source Source
}

// Point adalah satu titik pada grafik.
type Point struct {
	Totals
	Bucket time.Time
}

// Slice adalah satu potong Breakdown.
type Slice struct {
	Totals
	// ID kosong berarti dimensinya tidak terisi pada baris-baris itu, mis. request yang
	// ditolak sebelum provider terpilih. Dilaporkan apa adanya, bukan dibuang: request
	// tanpa provider tetap request yang terjadi.
	ID   string
	Name string
}

// agregatMentah adalah ekspresi agregat atas tabel requests.
//
// Persentilnya percentile_disc: baris mentahnya ada di tangan, jadi tidak ada alasan
// kehilangan presisi — dan percentile_disc mengembalikan nilai yang BENAR-BENAR terukur,
// bukan interpolasi antara dua pengamatan yang tidak pernah terjadi.
//
// Sukses dan gagal ditentukan status DAN error_type. Dua sebabnya, dan keduanya nyata:
// timeout upstream sampai ke klien sebagai 504 sementara koneksi terputus menjadi 502, jadi
// kode status tidak bisa lagi membedakan kategori kegagalan; dan aliran yang terputus
// setelah sebagian terkirim berstatus 200, jadi menghitung kegagalan dari status saja
// melewatkan seluruh kelas kegagalan streaming. Kedua penyaring itu komplemen persis,
// sehingga sukses + gagal selalu sama dengan jumlah request.
const agregatMentah = `
	count(*),
	count(*) filter (where status_code < 400 and error_type is null),
	count(*) filter (where status_code >= 400 or error_type is not null),
	count(*) filter (where error_type = 'timeout'),
	coalesce(sum(retry_count), 0),
	coalesce(sum(failover_count), 0),
	coalesce(sum(input_tokens), 0),
	coalesce(sum(output_tokens), 0),
	coalesce(sum(cached_input_tokens), 0),
	coalesce(sum(reasoning_tokens), 0),
	coalesce(sum(total_tokens), 0),
	(coalesce(sum(cost_usd), 0) * 100000000)::bigint,
	count(latency_ms),
	coalesce(sum(latency_ms), 0)::bigint,
	max(latency_ms),
	(percentile_disc(0.50) within group (order by latency_ms))::float8,
	(percentile_disc(0.90) within group (order by latency_ms))::float8,
	(percentile_disc(0.95) within group (order by latency_ms))::float8,
	(percentile_disc(0.99) within group (order by latency_ms))::float8,
	count(ttft_ms),
	coalesce(sum(ttft_ms), 0)::bigint,
	max(ttft_ms),
	(percentile_disc(0.50) within group (order by ttft_ms))::float8,
	(percentile_disc(0.90) within group (order by ttft_ms))::float8,
	(percentile_disc(0.95) within group (order by ttft_ms))::float8,
	(percentile_disc(0.99) within group (order by ttft_ms))::float8`

// agregatRollup adalah ekspresi agregat atas usage_hourly maupun usage_daily.
//
// Bentuk kolom kedua tabel itu sengaja identik, jadi satu ekspresi melayani keduanya.
//
// Persentilnya WAJIB lewat usage_histogram_sum: kolom latency_p95_ms milik satu baris
// hanya sah untuk baris itu sendiri. Keempat pemanggilan memakai ekspresi agregat yang
// sama persis, sehingga PostgreSQL menghitung histogram gabungannya sekali lalu memakainya
// berulang — bukan empat kali.
const agregatRollup = `
	coalesce(sum(request_count), 0),
	coalesce(sum(success_count), 0),
	coalesce(sum(error_count), 0),
	coalesce(sum(timeout_count), 0),
	coalesce(sum(retry_count), 0),
	coalesce(sum(failover_count), 0),
	coalesce(sum(input_tokens), 0),
	coalesce(sum(output_tokens), 0),
	coalesce(sum(cached_input_tokens), 0),
	coalesce(sum(reasoning_tokens), 0),
	coalesce(sum(total_tokens), 0),
	(coalesce(sum(cost_usd), 0) * 100000000)::bigint,
	coalesce(sum(latency_count), 0),
	coalesce(sum(latency_sum_ms), 0),
	max(latency_max_ms),
	usage_histogram_percentile(usage_histogram_sum(latency_hist), 0.50)::float8,
	usage_histogram_percentile(usage_histogram_sum(latency_hist), 0.90)::float8,
	usage_histogram_percentile(usage_histogram_sum(latency_hist), 0.95)::float8,
	usage_histogram_percentile(usage_histogram_sum(latency_hist), 0.99)::float8,
	coalesce(sum(ttft_count), 0),
	coalesce(sum(ttft_sum_ms), 0),
	max(ttft_max_ms),
	usage_histogram_percentile(usage_histogram_sum(ttft_hist), 0.50)::float8,
	usage_histogram_percentile(usage_histogram_sum(ttft_hist), 0.90)::float8,
	usage_histogram_percentile(usage_histogram_sum(ttft_hist), 0.95)::float8,
	usage_histogram_percentile(usage_histogram_sum(ttft_hist), 0.99)::float8`

// scanTotals membaca satu baris hasil agregat, mengikuti urutan agregatMentah dan
// agregatRollup — keduanya WAJIB tetap sama bentuknya.
//
// dest tambahan di depan dipakai pemanggil yang ikut memilih kolom bucket atau dimensi,
// sehingga hanya ada satu tempat yang tahu urutan 26 kolom agregat itu.
func scanTotals(s interface{ Scan(...any) error }, depan ...any) (Totals, error) {
	var t Totals
	var biaya int64
	dest := append(depan,
		&t.Requests, &t.Success, &t.Errors, &t.Timeouts, &t.Retries, &t.Failovers,
		&t.InputTokens, &t.OutputTokens, &t.CachedInputTokens, &t.ReasoningTokens, &t.TotalTokens,
		&biaya,
		&t.Latency.Count, &t.Latency.SumMS, &t.Latency.MaxMS,
		&t.Latency.P50, &t.Latency.P90, &t.Latency.P95, &t.Latency.P99,
		&t.TTFT.Count, &t.TTFT.SumMS, &t.TTFT.MaxMS,
		&t.TTFT.P50, &t.TTFT.P90, &t.TTFT.P95, &t.TTFT.P99,
	)
	if err := s.Scan(dest...); err != nil {
		return Totals{}, err
	}
	t.CostUSD = upstream.USD(biaya)
	return t, nil
}

// rencana adalah bentuk query yang sudah diselesaikan: tabel, kolom waktu, ekspresi
// agregat, syarat, dan argumennya.
//
// Dirakit sekali lalu dipakai Summary, Series, dan Breakdown. Tanpa perantara ini, ketiga
// method itu harus mengulang pemilihan sumber dan perakitan syarat — dan pengulangan itu
// tepat tempat satu method kehilangan satu filter tanpa satu pun test menyadarinya, karena
// hasilnya tetap berupa angka yang wajar.
type rencana struct {
	source Source
	tabel  string
	waktu  string
	agg    string
	conds  []string
	args   []any
	from   time.Time
	to     time.Time
	gran   Granularity
}

// rencanakan menyelesaikan Query menjadi rencana.
func rencanakan(op string, q Query) (rencana, error) {
	from, to := q.From, q.To
	if to.IsZero() {
		to = time.Now()
	}
	if from.IsZero() {
		from = to.Add(-DefaultWindow)
	}
	if !to.After(from) {
		return rencana{}, fmt.Errorf("%s: rentang kosong (%s..%s)", op,
			from.UTC().Format(time.RFC3339), to.UTC().Format(time.RFC3339))
	}

	src := q.Source
	if src == SourceAuto {
		if to.Sub(from) <= autoSourceLimit {
			src = SourceRequests
		} else {
			src = SourceDaily
		}
	}

	p := rencana{source: src, from: from, to: to}
	switch src {
	case SourceRequests:
		p.tabel, p.waktu, p.agg = "requests", "created_at", agregatMentah
	case SourceHourly:
		p.tabel, p.waktu, p.agg = "usage_hourly", "bucket", agregatRollup
	case SourceDaily:
		p.tabel, p.waktu, p.agg = "usage_daily", "bucket", agregatRollup
	default:
		return rencana{}, fmt.Errorf("%s: sumber %q tidak dikenal", op, q.Source)
	}

	gran := q.Granularity
	if gran == GranularityAuto {
		gran = GranularityDay
		if to.Sub(from) <= autoGranularityLimit {
			gran = GranularityHour
		}
	}
	switch gran {
	case GranularityHour:
		// usage_daily tidak menyimpan jam, dan menaikkannya menjadi hari secara diam-diam
		// akan menghasilkan grafik dengan sumbu x yang bukan diminta pemanggil.
		if src == SourceDaily {
			return rencana{}, fmt.Errorf("%s: granularitas jam tidak bisa dibaca dari usage_daily", op)
		}
	case GranularityDay:
	default:
		return rencana{}, fmt.Errorf("%s: granularitas %q tidak dikenal", op, q.Granularity)
	}
	p.gran = gran

	p.args = []any{from, to}
	p.conds = []string{p.waktu + " >= $1", p.waktu + " < $2"}
	tambah := func(kolom string, nilai any) {
		p.args = append(p.args, nilai)
		p.conds = append(p.conds, fmt.Sprintf("%s = $%d", kolom, len(p.args)))
	}
	if q.APIKeyID != "" {
		tambah("api_key_id", q.APIKeyID)
	}
	if q.ProviderID != "" {
		tambah("provider_id", q.ProviderID)
	}
	if q.ModelID != "" {
		tambah("model_id", q.ModelID)
	}
	return p, nil
}

// where mengembalikan klausa WHERE lengkap.
func (p rencana) where() string { return " where " + strings.Join(p.conds, " and ") }

// bucketExpr mengembalikan ekspresi perataan bucket.
//
// date_trunc versi tiga argumen dipakai supaya perataannya di UTC dan tidak bergantung
// pada TimeZone sesi — kalau bergantung, dua instance dengan setelan berbeda akan
// mengembalikan grafik dengan batas bucket yang berbeda untuk data yang sama.
func (p rencana) bucketExpr() string {
	return fmt.Sprintf("date_trunc('%s', %s, 'UTC')", string(p.gran), p.waktu)
}

// Summary mengembalikan ringkasan satu rentang.
//
// Selalu mengembalikan satu Stats, termasuk ketika tidak ada satu pun request: nol
// permintaan adalah jawaban yang sah dan berbeda dari kegagalan. Stats.Source menyebut
// tabel yang benar-benar dibaca, karena persentil dari requests dan dari rollup punya
// definisi yang berbeda.
func (r *Repo) Summary(ctx context.Context, q Query) (Stats, error) {
	const op = "meringkas pemakaian"

	p, err := rencanakan(op, q)
	if err != nil {
		return Stats{}, err
	}

	t, err := scanTotals(r.q.QueryRow(ctx, `select `+p.agg+` from `+p.tabel+p.where(), p.args...))
	if err != nil {
		return Stats{}, repo.Err(op, err)
	}
	return Stats{Totals: t, From: p.from, To: p.to, Source: p.source}, nil
}

// Series mengembalikan satu titik per bucket, terurut naik.
//
// Bucket yang tidak punya request TIDAK dikembalikan sebagai titik nol. Pengisian lubang
// adalah keputusan tampilan — grafik garis ingin nol, tabel ingin baris yang hilang tetap
// hilang — dan lapisan ini tidak boleh memilihkannya.
func (r *Repo) Series(ctx context.Context, q Query) ([]Point, error) {
	const op = "mengambil seri pemakaian"

	p, err := rencanakan(op, q)
	if err != nil {
		return nil, err
	}

	bucket := p.bucketExpr()
	rows, err := r.q.Query(ctx, `select `+bucket+`, `+p.agg+` from `+p.tabel+p.where()+
		` group by 1 order by 1`, p.args...)
	if err != nil {
		return nil, repo.Err(op, err)
	}
	defer rows.Close()

	var out []Point
	for rows.Next() {
		var pt Point
		t, err := scanTotals(rows, &pt.Bucket)
		if err != nil {
			return nil, repo.Err(op, err)
		}
		pt.Totals = t
		out = append(out, pt)
	}
	if err := rows.Err(); err != nil {
		return nil, repo.Err(op, err)
	}
	return out, nil
}

// kolomDimensi memetakan dimensi ke pasangan kolom id dan nama.
//
// Daftar putih, bukan nama kolom dari pemanggil: nama kolom masuk ke SQL sebagai
// identifier dan tidak bisa diparameterkan.
var kolomDimensi = map[Dimension][2]string{
	DimProvider: {"provider_id", "provider_name"},
	DimModel:    {"model_id", "model_name"},
	DimAPIKey:   {"api_key_id", "api_key_name"},
}

// Breakdown memecah satu rentang menurut satu dimensi, terurut dari yang paling banyak
// request.
//
// limit <= 0 memakai repo.DefaultPageLimit. Tidak ada paginasi keyset di sini dengan
// sengaja: jumlah provider, model, dan API key aktif terbatas, dan pemecahan yang
// berhalaman menuntut pemanggil menjumlahkan sendiri lintas halaman untuk mendapat
// totalnya — pertanyaan yang sudah dijawab Summary.
func (r *Repo) Breakdown(ctx context.Context, q Query, dim Dimension, limit int) ([]Slice, error) {
	const op = "memecah pemakaian per dimensi"

	kolom, ok := kolomDimensi[dim]
	if !ok {
		return nil, fmt.Errorf("%s: dimensi %q tidak dikenal", op, dim)
	}
	p, err := rencanakan(op, q)
	if err != nil {
		return nil, err
	}
	if limit <= 0 {
		limit = repo.DefaultPageLimit
	}

	idKolom, namaKolom := kolom[0], kolom[1]
	// Nama diambil dengan max() karena ia cuplikan: satu entitas yang berganti nama di
	// tengah rentang punya lebih dari satu nama, dan yang berwenang tetap kolom id.
	p.args = append(p.args, limit)
	rows, err := r.q.Query(ctx, `select coalesce(`+idKolom+`::text, ''), coalesce(max(`+namaKolom+`), ''), `+
		p.agg+` from `+p.tabel+p.where()+
		` group by 1 order by 3 desc, 1 limit $`+fmt.Sprint(len(p.args)), p.args...)
	if err != nil {
		return nil, repo.Err(op, err)
	}
	defer rows.Close()

	var out []Slice
	for rows.Next() {
		var s Slice
		t, err := scanTotals(rows, &s.ID, &s.Name)
		if err != nil {
			return nil, repo.Err(op, err)
		}
		s.Totals = t
		out = append(out, s)
	}
	if err := rows.Err(); err != nil {
		return nil, repo.Err(op, err)
	}
	return out, nil
}

// LatencyByProviderModel mengembalikan persentil latensi per pemetaan model-provider,
// dibaca dari baris requests pada jendela terakhir.
//
// Inilah sumber angka untuk strategi routing lowest_latency, dan tiga keputusan di dalamnya
// menentukan apakah strategi itu benar:
//
//   - Yang diukur coalesce(ttft_ms, latency_ms), bukan latency_ms saja. Untuk respons
//     streaming, yang ditunggu pengguna adalah token pertama; latensi penuhnya adalah
//     panjang jawaban, bukan kecepatan provider. Memakai latency_ms apa adanya membuat
//     provider yang melayani jawaban panjang selalu terlihat paling lambat. Sisa biasnya
//     diakui: pada satu pemetaan yang lalu lintasnya campur, dua definisi ikut tercampur.
//   - Hanya request BERHASIL. Provider yang menolak setiap permintaan dalam 5 ms adalah
//     yang tercepat menurut angka mentah, dan mengurutkannya paling depan berarti
//     mengirimkan seluruh lalu lintas ke provider yang sedang rusak.
//   - minSamples menyaring pemetaan yang sampelnya terlalu sedikit, dan pemetaan yang
//     tersaring TIDAK muncul di hasil sama sekali. Pemanggil memperlakukan yang tidak ada
//     sebagai "belum terukur" lalu mengurutkannya paling belakang — p95 dari dua sampel
//     bukan pengukuran, dan memperlakukannya seperti pengukuran akan mengalihkan seluruh
//     lalu lintas ke provider yang baru saja melayani satu permintaan cepat.
//
// Dikembalikan dalam milidetik. Kuncinya provider_models.id, yaitu kunci yang sama dengan
// yang dipakai tabel harga dan router.
func (r *Repo) LatencyByProviderModel(
	ctx context.Context, window time.Duration, percentile float64, minSamples int,
) (map[string]time.Duration, error) {
	const op = "mengambil latensi per pemetaan model-provider"

	if window <= 0 {
		return nil, fmt.Errorf("%s: jendela pengukuran harus positif", op)
	}
	if percentile <= 0 || percentile > 1 {
		return nil, fmt.Errorf("%s: persentil harus pecahan dalam (0,1], bukan %v", op, percentile)
	}
	if minSamples < 1 {
		minSamples = 1
	}

	rows, err := r.q.Query(ctx, `
		select pm.id::text,
		       (percentile_disc($2) within group (order by coalesce(r.ttft_ms, r.latency_ms)))::float8
		from requests r
		join provider_models pm
		  on pm.provider_id = r.provider_id and pm.model_id = r.model_id
		where r.created_at >= $1 and r.status_code < 400
		group by pm.id
		having count(*) >= $3`,
		time.Now().Add(-window), percentile, minSamples)
	if err != nil {
		return nil, repo.Err(op, err)
	}
	defer rows.Close()

	out := make(map[string]time.Duration)
	for rows.Next() {
		var (
			id string
			ms *float64
		)
		if err := rows.Scan(&id, &ms); err != nil {
			return nil, repo.Err(op, err)
		}
		if ms == nil || *ms < 0 {
			continue
		}
		out[id] = time.Duration(*ms * float64(time.Millisecond))
	}
	if err := rows.Err(); err != nil {
		return nil, repo.Err(op, err)
	}
	return out, nil
}
