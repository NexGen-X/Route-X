package traffic

import (
	"context"
	"fmt"
	"time"

	"github.com/NexGen-X/Route-X/internal/database/repo"
)

// Berkas ini mengisi usage_hourly dan usage_daily dari data yang sudah ada.
//
// # Satu bucket dihitung ulang lalu ditimpa, bukan ditambahkan
//
// Kedua fungsi di sini memakai "insert ... on conflict do update set x = excluded.x",
// jadi menjalankannya dua kali untuk bucket yang sama menghasilkan angka yang sama.
// Bentuk "set x = tabel.x + excluded.x" akan menggandakan angka pada setiap pengulangan
// dan baru benar bila ada watermark per bucket — kerumitan yang tidak dibutuhkan, karena
// satu bucket selalu murah dihitung ulang. Sifat idempoten inilah yang membuat pemanggil
// boleh menghitung ulang jam yang sedang berjalan sesering ia mau, dan boleh mengulang
// jam lampau setelah request yang datang terlambat masuk.
//
// # Kenapa harian dibangun dari jam, bukan dari requests
//
// Satu hari untuk satu kombinasi dimensi adalah 24 baris usage_hourly, sementara dari
// requests bisa jutaan baris. Penggabungannya tetap sah karena seluruh besaran di kedua
// tabel bersifat aditif KECUALI persentil — dan persentil harian diambil dari histogram
// gabungan lewat usage_histogram_sum, bukan dari merata-ratakan persentil per jam.
//
// Konsekuensi yang harus diketahui pembaca kolomnya: persentil pada usage_hourly PERSIS
// (dihitung percentile_disc atas baris requests), sedangkan persentil pada usage_daily
// adalah hasil interpolasi di dalam slot histogram. Galatnya terbatas lebar slot, dan
// itu jauh lebih baik daripada rata-rata persentil — yang bukan salah sedikit, melainkan
// salah sama sekali. Untuk angka yang harus persis, sumbernya tetap requests.
//
// # Nama entitas adalah cuplikan
//
// Nama api key, provider, dan model diambil dengan max() di dalam grup. Kalau satu
// entitas berganti nama di tengah bucket, salah satu namanya yang tercatat — dan itu
// diterima: kolom nama ada supaya baris agregat tetap terbaca setelah entitasnya dihapus,
// sementara yang berwenang tetap kolom id.
//
// # Token mengikuti konvensi OpenAI, seperti requests
//
// input_tokens SUDAH memuat cached_input_tokens, dan output_tokens SUDAH memuat
// reasoning_tokens — lihat catatan di usage.Event. Jadi total = input + output, dan
// menjumlahkan keempat kolom akan menghitung token cache dan token penalaran dua kali.

// RollupHourly menghitung ulang satu bucket jam dari tabel requests.
//
// at boleh titik waktu mana pun di dalam jam yang dimaksud; perataannya dilakukan di sini
// supaya pemanggil tidak bisa mengirim bucket yang tidak rata — check constraint
// usage_hourly_bucket_aligned akan menolaknya, tetapi menolak di sini memberi pesan yang
// menyebut sebabnya.
//
// Mengembalikan jumlah baris agregat yang ditulis atau diperbarui. Nol berarti tidak ada
// satu pun request pada jam itu, yang merupakan keadaan sah — bukan kegagalan.
//
// Perhatian: fungsi ini hanya MENIMPA. Menghitung ulang jam yang baris mentahnya sudah
// dibuang retensi tidak menolkan baris agregatnya, ia membiarkan nilai lama apa adanya.
// Itu disengaja: retensi membuang detail, bukan ringkasan.
func (r *Repo) RollupHourly(ctx context.Context, at time.Time) (int64, error) {
	const op = "menghitung rollup pemakaian per jam"

	from := awalJam(at)
	tag, err := r.q.Exec(ctx, rollupHourlySQL, from, from.Add(time.Hour))
	if err != nil {
		return 0, repo.Err(op, err)
	}
	return tag.RowsAffected(), nil
}

// RollupDaily menghitung ulang satu bucket hari dari usage_hourly.
//
// Menuntut jam-jam di dalam hari itu sudah di-rollup lebih dulu: fungsi ini tidak
// menyentuh requests sama sekali. Jam yang belum dihitung berarti harinya kurang, dan
// menghitung ulang setelah jam itu masuk memperbaikinya karena hasilnya ditimpa.
func (r *Repo) RollupDaily(ctx context.Context, at time.Time) (int64, error) {
	const op = "menghitung rollup pemakaian per hari"

	from := awalHari(at)
	tag, err := r.q.Exec(ctx, rollupDailySQL, from, from.AddDate(0, 0, 1))
	if err != nil {
		return 0, repo.Err(op, err)
	}
	return tag.RowsAffected(), nil
}

// RollupRange menghitung ulang seluruh jam pada satu rentang, lalu hari yang memuatnya.
//
// Dipakai backfill dan worker pemeliharaan: satu pemanggilan yang menutup rentang penuh
// jauh lebih sulit disalahgunakan daripada dua loop di sisi pemanggil, yang gampang
// melewatkan hari terakhir ketika rentangnya berakhir di tengah hari.
//
// Batasnya diratakan ke jam, sehingga rentang [09:30, 10:15) tetap menghitung jam 09 dan
// jam 10 seluruhnya — separuh jam bukan bucket yang bisa disimpan tabel ini.
func (r *Repo) RollupRange(ctx context.Context, from, to time.Time) (jam, hari int64, err error) {
	if !to.After(from) {
		return 0, 0, fmt.Errorf("menghitung rollup pemakaian: rentang kosong (%s..%s)",
			from.UTC().Format(time.RFC3339), to.UTC().Format(time.RFC3339))
	}

	for t := awalJam(from); t.Before(to); t = t.Add(time.Hour) {
		n, err := r.RollupHourly(ctx, t)
		if err != nil {
			return jam, hari, err
		}
		jam += n
	}
	for d := awalHari(from); d.Before(to); d = d.AddDate(0, 0, 1) {
		n, err := r.RollupDaily(ctx, d)
		if err != nil {
			return jam, hari, err
		}
		hari += n
	}
	return jam, hari, nil
}

// awalJam meratakan waktu ke awal jam di UTC.
//
// Dirakit dengan time.Date, bukan Truncate: Truncate bekerja atas durasi sejak waktu nol
// dan kebetulan benar untuk satu jam, tetapi kebetulan itu tidak berlaku untuk satuan lain
// dan tidak terbaca sebagai maksud. Zona UTC dipaksa karena bucket-nya rata di UTC —
// perataan mengikuti zona sesi membuat satu bucket bisa terisi dua kali dengan batas
// berbeda.
func awalJam(t time.Time) time.Time {
	u := t.UTC()
	return time.Date(u.Year(), u.Month(), u.Day(), u.Hour(), 0, 0, 0, time.UTC)
}

// awalHari meratakan waktu ke awal hari di UTC.
func awalHari(t time.Time) time.Time {
	u := t.UTC()
	return time.Date(u.Year(), u.Month(), u.Day(), 0, 0, 0, 0, time.UTC)
}

// rollupHourlySQL menghitung satu jam dari requests.
//
// Bucket-nya diambil dari parameter, bukan dari date_trunc per baris: rentangnya sudah
// tepat satu jam, jadi date_trunc hanya akan menghitung ulang nilai yang sudah kami
// ketahui — dan nilai dari parameter itu pasti rata, sehingga check constraint
// usage_hourly_bucket_aligned tidak bisa dilanggar oleh pembulatan yang berbeda.
//
// Sukses dan gagal ditentukan status DAN error_type, bukan status saja. Aliran yang
// terputus setelah sebagian terkirim sampai ke klien sebagai 200 — status itu benar, karena
// klien memang menerima jawaban sebagian — sehingga menghitung kegagalan dari status saja
// akan melewatkan seluruh kelas kegagalan streaming. Kedua penyaring itu komplemen persis,
// jadi success_count + error_count selalu sama dengan request_count dan constraint
// usage_hourly_outcome_consistent terpenuhi.
//
// timeout dihitung tanpa syarat status, dengan alasan yang sama: timeout di tengah aliran
// juga berstatus 200.
//
// Persentil dipakai percentile_disc, bukan percentile_cont: kolomnya bertipe int dan
// percentile_disc mengembalikan salah satu nilai yang BENAR-BENAR terukur, jadi tidak ada
// keputusan pembulatan yang diambil diam-diam. Angka ini persis untuk jam ini sendiri, dan
// karena itu TIDAK boleh dijumlahkan atau dirata-ratakan antar baris — untuk rentang
// gabungan, histogram di kolom terakhirlah yang dipakai.
const rollupHourlySQL = `
insert into usage_hourly (
	bucket, api_key_id, api_key_name, provider_id, provider_name, model_id, model_name,
	request_count, success_count, error_count, timeout_count, retry_count, failover_count,
	input_tokens, output_tokens, cached_input_tokens, reasoning_tokens, total_tokens,
	cost_usd,
	latency_sum_ms, latency_count, latency_max_ms,
	latency_p50_ms, latency_p90_ms, latency_p95_ms, latency_p99_ms, latency_hist,
	ttft_sum_ms, ttft_count, ttft_max_ms,
	ttft_p50_ms, ttft_p90_ms, ttft_p95_ms, ttft_p99_ms, ttft_hist
)
select
	$1::timestamptz,
	api_key_id, max(api_key_name), provider_id, max(provider_name), model_id, max(model_name),
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
	coalesce(sum(cost_usd), 0),
	coalesce(sum(latency_ms), 0),
	count(latency_ms),
	max(latency_ms),
	percentile_disc(0.50) within group (order by latency_ms),
	percentile_disc(0.90) within group (order by latency_ms),
	percentile_disc(0.95) within group (order by latency_ms),
	percentile_disc(0.99) within group (order by latency_ms),
	usage_histogram_of(latency_ms),
	coalesce(sum(ttft_ms), 0),
	count(ttft_ms),
	max(ttft_ms),
	percentile_disc(0.50) within group (order by ttft_ms),
	percentile_disc(0.90) within group (order by ttft_ms),
	percentile_disc(0.95) within group (order by ttft_ms),
	percentile_disc(0.99) within group (order by ttft_ms),
	usage_histogram_of(ttft_ms)
from requests
where created_at >= $1 and created_at < $2
group by api_key_id, provider_id, model_id
on conflict (bucket, api_key_id, provider_id, model_id) do update set
	api_key_name   = excluded.api_key_name,
	provider_name  = excluded.provider_name,
	model_name     = excluded.model_name,
	request_count  = excluded.request_count,
	success_count  = excluded.success_count,
	error_count    = excluded.error_count,
	timeout_count  = excluded.timeout_count,
	retry_count    = excluded.retry_count,
	failover_count = excluded.failover_count,
	input_tokens        = excluded.input_tokens,
	output_tokens       = excluded.output_tokens,
	cached_input_tokens = excluded.cached_input_tokens,
	reasoning_tokens    = excluded.reasoning_tokens,
	total_tokens        = excluded.total_tokens,
	cost_usd            = excluded.cost_usd,
	latency_sum_ms = excluded.latency_sum_ms,
	latency_count  = excluded.latency_count,
	latency_max_ms = excluded.latency_max_ms,
	latency_p50_ms = excluded.latency_p50_ms,
	latency_p90_ms = excluded.latency_p90_ms,
	latency_p95_ms = excluded.latency_p95_ms,
	latency_p99_ms = excluded.latency_p99_ms,
	latency_hist   = excluded.latency_hist,
	ttft_sum_ms = excluded.ttft_sum_ms,
	ttft_count  = excluded.ttft_count,
	ttft_max_ms = excluded.ttft_max_ms,
	ttft_p50_ms = excluded.ttft_p50_ms,
	ttft_p90_ms = excluded.ttft_p90_ms,
	ttft_p95_ms = excluded.ttft_p95_ms,
	ttft_p99_ms = excluded.ttft_p99_ms,
	ttft_hist   = excluded.ttft_hist`

// rollupDailySQL menghitung satu hari dari usage_hourly.
//
// Histogram digabung lebih dulu di CTE, lalu persentilnya dibaca dari hasil gabungan itu.
// Dua alasan bentuknya begitu, dan keduanya bukan gaya:
//
//   - INI satu-satunya cara yang benar. usage_histogram_sum menjumlahkan cacah slot demi
//     slot, sehingga hasilnya persis histogram seluruh hari; persentilnya lalu dicari dari
//     distribusi gabungan. Merata-ratakan latency_p95_ms milik 24 baris jam bukan salah
//     sedikit — dengan 1.000 sampel cepat dan 10 sampel lambat, p95 sebenarnya 88 ms
//     sementara rata-rata p95 per bucket menghasilkan 4.548 ms.
//   - Tanpa CTE, usage_histogram_sum harus ditulis di dalam empat pemanggilan
//     usage_histogram_percentile dan pembaca berikutnya tidak punya cara melihat bahwa
//     keempatnya memang berasal dari satu histogram yang sama.
//
// Cast ke int pada persentil membulatkan pecahan hasil interpolasi. Itu satu-satunya
// pembulatan di jalur ini, dan besarnya jauh di bawah lebar slot histogram.
const rollupDailySQL = `
with gabungan as (
	select
		api_key_id,
		max(api_key_name)  as api_key_name,
		provider_id,
		max(provider_name) as provider_name,
		model_id,
		max(model_name)    as model_name,
		coalesce(sum(request_count),  0) as request_count,
		coalesce(sum(success_count),  0) as success_count,
		coalesce(sum(error_count),    0) as error_count,
		coalesce(sum(timeout_count),  0) as timeout_count,
		coalesce(sum(retry_count),    0) as retry_count,
		coalesce(sum(failover_count), 0) as failover_count,
		coalesce(sum(input_tokens),        0) as input_tokens,
		coalesce(sum(output_tokens),       0) as output_tokens,
		coalesce(sum(cached_input_tokens), 0) as cached_input_tokens,
		coalesce(sum(reasoning_tokens),    0) as reasoning_tokens,
		coalesce(sum(total_tokens),        0) as total_tokens,
		coalesce(sum(cost_usd), 0) as cost_usd,
		coalesce(sum(latency_sum_ms), 0) as latency_sum_ms,
		coalesce(sum(latency_count),  0) as latency_count,
		max(latency_max_ms) as latency_max_ms,
		usage_histogram_sum(latency_hist) as latency_hist,
		coalesce(sum(ttft_sum_ms), 0) as ttft_sum_ms,
		coalesce(sum(ttft_count),  0) as ttft_count,
		max(ttft_max_ms) as ttft_max_ms,
		usage_histogram_sum(ttft_hist) as ttft_hist
	from usage_hourly
	where bucket >= $1 and bucket < $2
	group by api_key_id, provider_id, model_id
)
insert into usage_daily (
	bucket, api_key_id, api_key_name, provider_id, provider_name, model_id, model_name,
	request_count, success_count, error_count, timeout_count, retry_count, failover_count,
	input_tokens, output_tokens, cached_input_tokens, reasoning_tokens, total_tokens,
	cost_usd,
	latency_sum_ms, latency_count, latency_max_ms,
	latency_p50_ms, latency_p90_ms, latency_p95_ms, latency_p99_ms, latency_hist,
	ttft_sum_ms, ttft_count, ttft_max_ms,
	ttft_p50_ms, ttft_p90_ms, ttft_p95_ms, ttft_p99_ms, ttft_hist
)
select
	$1::timestamptz,
	api_key_id, api_key_name, provider_id, provider_name, model_id, model_name,
	request_count, success_count, error_count, timeout_count, retry_count, failover_count,
	input_tokens, output_tokens, cached_input_tokens, reasoning_tokens, total_tokens,
	cost_usd,
	latency_sum_ms, latency_count, latency_max_ms,
	usage_histogram_percentile(latency_hist, 0.50)::int,
	usage_histogram_percentile(latency_hist, 0.90)::int,
	usage_histogram_percentile(latency_hist, 0.95)::int,
	usage_histogram_percentile(latency_hist, 0.99)::int,
	latency_hist,
	ttft_sum_ms, ttft_count, ttft_max_ms,
	usage_histogram_percentile(ttft_hist, 0.50)::int,
	usage_histogram_percentile(ttft_hist, 0.90)::int,
	usage_histogram_percentile(ttft_hist, 0.95)::int,
	usage_histogram_percentile(ttft_hist, 0.99)::int,
	ttft_hist
from gabungan
on conflict (bucket, api_key_id, provider_id, model_id) do update set
	api_key_name   = excluded.api_key_name,
	provider_name  = excluded.provider_name,
	model_name     = excluded.model_name,
	request_count  = excluded.request_count,
	success_count  = excluded.success_count,
	error_count    = excluded.error_count,
	timeout_count  = excluded.timeout_count,
	retry_count    = excluded.retry_count,
	failover_count = excluded.failover_count,
	input_tokens        = excluded.input_tokens,
	output_tokens       = excluded.output_tokens,
	cached_input_tokens = excluded.cached_input_tokens,
	reasoning_tokens    = excluded.reasoning_tokens,
	total_tokens        = excluded.total_tokens,
	cost_usd            = excluded.cost_usd,
	latency_sum_ms = excluded.latency_sum_ms,
	latency_count  = excluded.latency_count,
	latency_max_ms = excluded.latency_max_ms,
	latency_p50_ms = excluded.latency_p50_ms,
	latency_p90_ms = excluded.latency_p90_ms,
	latency_p95_ms = excluded.latency_p95_ms,
	latency_p99_ms = excluded.latency_p99_ms,
	latency_hist   = excluded.latency_hist,
	ttft_sum_ms = excluded.ttft_sum_ms,
	ttft_count  = excluded.ttft_count,
	ttft_max_ms = excluded.ttft_max_ms,
	ttft_p50_ms = excluded.ttft_p50_ms,
	ttft_p90_ms = excluded.ttft_p90_ms,
	ttft_p95_ms = excluded.ttft_p95_ms,
	ttft_p99_ms = excluded.ttft_p99_ms,
	ttft_hist   = excluded.ttft_hist`
