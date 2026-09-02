-- Migrasi 0007 — agregasi pemakaian: rollup per jam dan per hari.
--
-- Tabel requests di 0006 tetap sumber kebenaran per request, tetapi dashboard tidak
-- boleh mengagregasi puluhan juta baris setiap kali halaman dibuka. Dua tabel di sini
-- adalah ringkasan yang diisi worker: usage_hourly untuk grafik 24 jam dan 7 hari,
-- usage_daily untuk rentang 30 hari dan rentang kustom yang lebih panjang.
--
-- Lima keputusan yang membentuk kedua tabel:
--
-- 1. KUNCI ALAMI, TANPA ID SINTETIS. Satu baris adalah satu kombinasi
--    (bucket, api_key_id, provider_id, model_id). Itulah yang membuat worker cukup
--    memakai "insert ... on conflict do update" dan tetap benar saat dijalankan ulang.
--    Dimensinya boleh NULL — request yang ditolak sebelum provider terpilih memang
--    tidak punya provider_id — sehingga kunci ini tidak bisa dijadikan primary key,
--    karena primary key melarang NULL. Dipakai UNIQUE NULLS NOT DISTINCT (butuh
--    PostgreSQL 15+): pada indeks unique biasa NULL dianggap berbeda dari NULL, jadi
--    baris berdimensi NULL akan BERTAMBAH setiap kali worker jalan alih-alih
--    diperbarui — idempotensi hilang tanpa satu pun pesan error.
--
-- 2. KONTRAK WORKER: HITUNG ULANG SATU BUCKET LALU TIMPA, BUKAN MENAMBAH SELISIH.
--    Bentuk "do update set x = excluded.x" idempoten dengan sendirinya. Bentuk
--    "set x = usage_hourly.x + excluded.x" menggandakan angka pada setiap pengulangan
--    dan baru benar bila worker memelihara watermark per bucket — kerumitan yang tidak
--    dibutuhkan, karena satu bucket selalu murah dihitung ulang dari requests lewat
--    indeks requests_created_idx.
--
-- 3. BUCKET SELALU RATA DI UTC, dijaga check constraint. Kalau perataan mengikuti zona
--    waktu sesi, satu bucket bisa terisi dua kali dengan batas berbeda saat sesi
--    berganti zona, dan hari di zona berpindah DST akan berisi 23 atau 25 jam. Zona
--    waktu adalah urusan tampilan; konversi dilakukan saat membaca.
--
-- 4. CUPLIKAN NAMA IKUT DISIMPAN, alasannya sama seperti requests di 0006: baris
--    agregat harus tetap terbaca setelah provider, model, atau API key-nya dihapus.
--    ID tetap ada untuk join opsional, tetapi tidak ada foreign key ke tabel entitas —
--    ini catatan fakta yang sudah terjadi, bukan konfigurasi.
--
-- 5. BENTUK KOLOM KEDUA TABEL SENGAJA IDENTIK, hanya granularitas bucket-nya berbeda,
--    supaya lapisan repository memakai satu tipe baris dan satu query yang berbeda
--    hanya pada nama tabel.
--
-- ---------------------------------------------------------------------------
-- LATENSI DAN PERSENTIL — keputusan yang paling menentukan di file ini
-- ---------------------------------------------------------------------------
-- Rata-rata saja tidak cukup: dashboard menampilkan P50/P90/P95/P99 dan TTFT. Yang
-- disimpan karena itu ada tiga lapis, masing-masing punya alasan tersendiri:
--
-- a. latency_sum_ms + latency_count (dan pasangan ttft_*). Rata-rata harus dihitung
--    dari jumlah dan cacah, bukan disimpan sebagai angka rata-rata: merata-ratakan
--    rata-rata per bucket memberi bobot sama pada bucket yang isinya 3 request dan
--    bucket yang isinya 300 ribu. latency_max_ms ikut disimpan karena maksimum bisa
--    digabung persis dengan greatest().
--
-- b. Kolom latency_p50_ms..p99 dan ttft_p50_ms..p99. Angka ini dihitung worker dari
--    baris requests mentah, jadi PERSIS untuk bucket-nya sendiri, dan dipakai jalur
--    cepat dashboard: grafik 24 jam menampilkan satu titik per bucket, dan setiap
--    titik hanya butuh satu baris. Kolom-kolom ini TIDAK BOLEH dijumlahkan atau
--    dirata-ratakan antar baris — persentil bukan besaran aditif.
--
-- c. latency_hist dan ttft_hist: histogram batas tetap, satu bigint per slot. Inilah
--    satu-satunya struktur di tabel ini yang boleh digabung antar bucket, karena
--    penggabungannya adalah penjumlahan cacah per slot, bukan perata-rataan hasil.
--
-- Jadi kasus "P95 untuk rentang 30 hari" — yang salah secara matematis kalau dihitung
-- dengan merata-ratakan P95 per hari — dijawab lewat (c):
--
--     select usage_histogram_percentile(usage_histogram_sum(latency_hist), 0.95)
--     from usage_daily
--     where bucket >= :from and bucket < :to;
--
-- usage_histogram_sum menjumlahkan slot demi slot sehingga hasilnya persis histogram
-- gabungan seluruh rentang, lalu persentilnya dicari dari distribusi gabungan itu.
-- Aturan praktisnya untuk lapisan repository: satu baris → pakai kolom (b); lebih dari
-- satu baris → WAJIB lewat (c).
--
-- Efek samping yang berguna: usage_daily bisa dibangun dengan menggabungkan 24 baris
-- usage_hourly tanpa membaca requests lagi, dan persentil hariannya tetap sah.
--
-- Mengapa histogram batas tetap, bukan sketsa t-digest atau DDSketch: sketsa itu jauh
-- lebih akurat di ekor distribusi, tetapi di PostgreSQL keduanya datang dari extension
-- yang belum tentu tersedia di database terkelola, dan tipe datanya tidak bisa dibaca
-- ulang tanpa extension yang sama terpasang. Histogram di sini hanya array bigint:
-- penggabungannya aritmetika biasa, bisa dibaca alat apa pun, dan galatnya terbatas
-- pada lebar slot — rasio antar batas dijaga di sekitar 1,4-1,5 pada rentang yang
-- penting, jadi persentil hasil interpolasi menyimpang jauh di bawah selisih yang
-- berarti bagi operator. Untuk angka penagihan yang harus persis, sumbernya tetap
-- requests, bukan tabel ini.

-- ---------------------------------------------------------------------------
-- Perkakas histogram latensi
--
-- Batas slot didefinisikan di satu fungsi supaya penulis (worker), pembaca
-- (dashboard), nilai default kolom, dan check constraint memakai angka yang sama.
-- Kalau batasnya ditulis ulang di beberapa tempat, satu kali beda urutan sudah cukup
-- untuk membuat persentil salah tanpa ada yang gagal.
--
-- Batas dinyatakan dalam milidetik dan menaik. Slot ke-i berisi nilai pada rentang
-- [batas[i-1], batas[i]) — slot 1 dimulai dari 0, dan slot terakhir adalah luapan
-- [batas terakhir, tak hingga). Jumlah slot karena itu selalu cardinality(batas) + 1.
-- Rentangnya dipilih untuk lalu lintas LLM: dari beberapa milidetik (jawaban dari
-- cache) sampai sepuluh menit (generasi panjang yang mendekati timeout provider).
-- ---------------------------------------------------------------------------
create or replace function usage_latency_bounds() returns bigint[] as $$
    select array[
             5,     10,     15,     25,     40,     60,     90,    130,    190,    280,
           400,    600,    850,   1200,   1700,   2500,   3500,   5000,   7000,  10000,
         14000,  20000,  30000,  45000,  60000,  90000, 120000, 180000, 300000, 600000
    ]::bigint[];
$$ language sql immutable parallel safe;

comment on function usage_latency_bounds() is
    'Batas atas slot histogram latensi dalam milidetik, menaik. Satu-satunya sumber kebenaran bentuk histogram.';

-- Memetakan satu nilai latensi ke nomor slot. Dipakai worker saat mengisi histogram,
-- dan berguna langsung di SQL: "group by usage_latency_slot(latency_ms)" adalah cara
-- tercepat membangun histogram untuk backfill besar.
create or replace function usage_latency_slot(latency_ms int) returns int as $$
    select case
        when latency_ms is null then null
        -- Nilai negatif tidak semestinya ada; dimasukkan ke slot pertama daripada
        -- membuat seluruh rollup gagal karena satu baris rusak.
        when latency_ms < 0 then 1
        else width_bucket(latency_ms::bigint, usage_latency_bounds()) + 1
    end;
$$ language sql immutable parallel safe;

-- Total sampel di dalam sebuah histogram. Dipakai check constraint dan penghitung
-- persentil, jadi keduanya tidak mungkin memakai definisi yang berbeda.
create or replace function usage_histogram_total(counts bigint[]) returns bigint as $$
    select coalesce(sum(c), 0)::bigint from unnest(coalesce(counts, '{}'::bigint[])) as t(c);
$$ language sql immutable parallel safe;

-- Penjaga bentuk histogram, dipakai dari check constraint kedua tabel rollup:
-- panjang array harus tepat, satu dimensi, tanpa NULL, dan tanpa cacah negatif.
create or replace function usage_histogram_valid(counts bigint[]) returns boolean as $$
    select counts is not null
       and array_ndims(counts) = 1
       and array_length(counts, 1) = cardinality(usage_latency_bounds()) + 1
       and (select bool_and(c is not null and c >= 0) from unnest(counts) as t(c));
$$ language sql immutable parallel safe;

-- Mengembalikan histogram kosong bila state agregasi masih NULL, supaya agregat di
-- bawah selalu menghasilkan array yang sah dan kolomnya bisa NOT NULL.
create or replace function usage_histogram_empty(state bigint[]) returns bigint[] as $$
    select coalesce(state, array_fill(0::bigint, array[cardinality(usage_latency_bounds()) + 1]));
$$ language sql immutable parallel safe;

-- Penjumlahan slot demi slot: INI penggabungan yang membuat persentil rentang gabungan
-- benar. Panjang yang berbeda ditolak keras, karena histogram dengan batas berbeda
-- kalau dijumlahkan akan menghasilkan angka yang kelihatan wajar tapi salah.
create or replace function usage_histogram_add(a bigint[], b bigint[]) returns bigint[] as $$
begin
    if a is null then return b; end if;
    if b is null then return a; end if;
    if array_length(a, 1) <> array_length(b, 1) then
        raise exception 'histogram tidak sepanjang: % vs % slot', array_length(a, 1), array_length(b, 1)
            using errcode = 'data_exception';
    end if;
    return (
        select array_agg(coalesce(a[i], 0) + coalesce(b[i], 0) order by i)
        from generate_series(1, array_length(a, 1)) as g(i)
    );
end;
$$ language plpgsql immutable parallel safe;

-- Menambahkan satu pengamatan latensi ke state histogram; sfunc untuk usage_histogram_of.
create or replace function usage_histogram_accum(state bigint[], latency_ms int) returns bigint[] as $$
declare
    slot int;
begin
    state := usage_histogram_empty(state);
    if latency_ms is null then
        return state;
    end if;
    slot := usage_latency_slot(latency_ms);
    state[slot] := coalesce(state[slot], 0) + 1;
    return state;
end;
$$ language plpgsql immutable parallel safe;

-- Persentil dari sebuah histogram, termasuk histogram hasil penggabungan.
--
-- p adalah pecahan (0,1]; 0.95 berarti P95. Nilainya dicari dengan menelusuri cacah
-- kumulatif sampai melewati p * total, lalu diinterpolasi linear di dalam slot yang
-- memuat titik itu — galatnya karena itu tidak pernah lebih besar dari lebar slot.
-- Histogram tanpa sampel mengembalikan NULL, bukan 0: "tidak ada data" dan "nol
-- milidetik" adalah dua hal yang berbeda di dashboard.
create or replace function usage_histogram_percentile(counts bigint[], p numeric) returns numeric as $$
declare
    bounds  bigint[] := usage_latency_bounds();
    slots   int      := cardinality(bounds) + 1;
    total   bigint;
    target  numeric;
    running bigint := 0;
    before  bigint;
    c       bigint;
    lo      numeric;
    hi      numeric;
    i       int;
begin
    if p is null or p <= 0 or p > 1 then
        raise exception 'persentil harus pecahan dalam (0,1], bukan %', p
            using errcode = 'invalid_parameter_value';
    end if;
    if counts is null then
        return null;
    end if;
    if array_length(counts, 1) <> slots then
        raise exception 'histogram harus punya % slot, bukan %', slots, array_length(counts, 1)
            using errcode = 'data_exception';
    end if;

    total := usage_histogram_total(counts);
    if total = 0 then
        return null;
    end if;

    target := p * total;
    for i in 1 .. slots loop
        c       := coalesce(counts[i], 0);
        before  := running;
        running := running + c;
        if running >= target then
            if i = slots then
                -- Slot luapan tidak punya batas atas, jadi tidak ada yang bisa
                -- diinterpolasi. Yang dilaporkan batas terakhir apa adanya: hasilnya
                -- "setidaknya sebesar ini", dan itu lebih jujur daripada ekstrapolasi.
                return bounds[slots - 1]::numeric;
            end if;
            lo := case when i = 1 then 0::numeric else bounds[i - 1]::numeric end;
            hi := bounds[i]::numeric;
            return round(lo + (hi - lo) * ((target - before)::numeric / c::numeric), 2);
        end if;
    end loop;
    return null;
end;
$$ language plpgsql immutable parallel safe;

comment on function usage_histogram_percentile(bigint[], numeric) is
    'Persentil (p sebagai pecahan, mis. 0.95) dari histogram, termasuk histogram gabungan hasil usage_histogram_sum.';

-- Dua agregat. Keduanya dibungkus DO block karena PostgreSQL tidak punya
-- "create aggregate if not exists" dan migrasi di proyek ini harus aman dijalankan ulang.
--
--   usage_histogram_of(latency_ms)  membangun histogram dari baris requests mentah
--   usage_histogram_sum(hist)       menggabungkan histogram antar bucket
--
-- combinefunc-nya sama-sama usage_histogram_add sehingga keduanya boleh dipakai
-- perencana paralel: penggabungan parsial per worker adalah operasi yang sama.
do $$
begin
    if not exists (
        select 1 from pg_proc p join pg_namespace n on n.oid = p.pronamespace
        where p.proname = 'usage_histogram_of' and p.prokind = 'a' and n.nspname = current_schema()
    ) then
        create aggregate usage_histogram_of (int) (
            sfunc       = usage_histogram_accum,
            stype       = bigint[],
            finalfunc   = usage_histogram_empty,
            combinefunc = usage_histogram_add,
            parallel    = safe
        );
    end if;

    if not exists (
        select 1 from pg_proc p join pg_namespace n on n.oid = p.pronamespace
        where p.proname = 'usage_histogram_sum' and p.prokind = 'a' and n.nspname = current_schema()
    ) then
        create aggregate usage_histogram_sum (bigint[]) (
            sfunc       = usage_histogram_add,
            stype       = bigint[],
            finalfunc   = usage_histogram_empty,
            combinefunc = usage_histogram_add,
            parallel    = safe
        );
    end if;
end $$;

-- ---------------------------------------------------------------------------
-- usage_hourly — granularitas jam, untuk grafik 24 jam dan 7 hari
--
-- DIPARTISI BULANAN. Volumenya jauh di bawah requests, tetapi tumbuh terus dan
-- perkalian dimensinya nyata: satu jam bisa menghasilkan sebanyak
-- (api key aktif x provider x model) baris, dan setiap baris membawa dua array
-- histogram sehingga ukurannya ratusan byte — tabel ini bertambah besar lebih cepat
-- dalam byte daripada dalam jumlah baris. Tiga alasan konkretnya:
--
--   * Retensi. Tabel inilah yang datanya dibuang lebih dulu ketika ringkasan harian
--     sudah cukup. Dengan partisi, membuang setahun data adalah DROP TABLE yang
--     seketika; tanpa partisi, DELETE besar meninggalkan tabel membengkak yang harus
--     di-VACUUM FULL.
--   * Setiap query dashboard menyaring rentang bucket, jadi perencana bisa melewati
--     seluruh bulan yang tidak diminta.
--   * Biaya tambahannya nol: worker pemeliharaan partisi untuk requests di 0006 sudah
--     ada, dan tabel ini cukup ikut daftar induknya.
--
-- Upsert tetap bekerja di tabel terpartisi karena kunci unique-nya memuat kunci
-- partisi (bucket) sebagai kolom pertama — syarat wajib indeks unique pada tabel
-- terpartisi, dan sekaligus urutan kolom yang dibutuhkan pemindaian rentang bucket.
-- ---------------------------------------------------------------------------
create table if not exists usage_hourly (
    -- Awal jam, selalu rata di UTC.
    bucket timestamptz not null,

    -- Dimensi. NULL berarti "tidak ada / belum terpilih", bukan "semua".
    api_key_id    uuid,
    api_key_name  text,
    provider_id   uuid,
    provider_name text,
    model_id      uuid,
    model_name    text,

    -- Ukuran lalu lintas. timeout_count adalah bagian dari error_count, bukan
    -- tambahannya; retry dan failover dihitung per kejadian, jadi bisa melebihi
    -- request_count.
    request_count  bigint not null default 0,
    success_count  bigint not null default 0,
    error_count    bigint not null default 0,
    timeout_count  bigint not null default 0,
    retry_count    bigint not null default 0,
    failover_count bigint not null default 0,

    input_tokens        bigint not null default 0,
    output_tokens       bigint not null default 0,
    cached_input_tokens bigint not null default 0,
    reasoning_tokens    bigint not null default 0,
    total_tokens        bigint not null default 0,

    -- Uang selalu numeric. Skalanya 8 desimal seperti requests.cost_usd supaya
    -- penjumlahan tidak membulatkan lebih dulu; digit bulatnya lebih banyak karena
    -- nilai di sini adalah hasil penjumlahan banyak request.
    cost_usd numeric(20, 8) not null default 0,

    -- Latensi lapis (a): rata-rata dihitung sebagai latency_sum_ms / latency_count.
    latency_sum_ms bigint not null default 0,
    latency_count  bigint not null default 0,
    latency_max_ms int,
    -- Lapis (b): persis untuk baris ini sendiri, tidak boleh digabung antar baris.
    latency_p50_ms int,
    latency_p90_ms int,
    latency_p95_ms int,
    latency_p99_ms int,
    -- Lapis (c): satu-satunya kolom yang boleh digabung antar bucket.
    latency_hist bigint[] not null default array_fill(0::bigint, array[cardinality(usage_latency_bounds()) + 1]),

    -- TTFT hanya terisi untuk respons streaming, jadi ttft_count biasanya lebih kecil
    -- dari request_count. Bentuknya sengaja sama persis dengan latensi di atas.
    ttft_sum_ms bigint not null default 0,
    ttft_count  bigint not null default 0,
    ttft_max_ms int,
    ttft_p50_ms int,
    ttft_p90_ms int,
    ttft_p95_ms int,
    ttft_p99_ms int,
    ttft_hist bigint[] not null default array_fill(0::bigint, array[cardinality(usage_latency_bounds()) + 1]),

    -- Kapan baris ini terakhir dihitung ulang. Berguna saat menelusuri request yang
    -- datang terlambat: bucket lama yang updated_at-nya baru berarti sempat direvisi.
    updated_at timestamptz not null default now(),

    -- date_trunc 3 argumen dipakai karena versi itu immutable — versi 2 argumen
    -- bergantung pada TimeZone sesi sehingga tidak boleh dipakai di check constraint.
    constraint usage_hourly_bucket_aligned check (bucket = date_trunc('hour', bucket, 'UTC')),

    constraint usage_hourly_counts_nonneg check (
        request_count >= 0 and success_count >= 0 and error_count >= 0 and
        timeout_count >= 0 and retry_count   >= 0 and failover_count >= 0
    ),
    -- Satu request dihitung sekali: sukses atau gagal. Kalau jumlahnya melebihi
    -- request_count berarti worker menghitung ganda.
    constraint usage_hourly_outcome_consistent check (success_count + error_count <= request_count),
    constraint usage_hourly_timeout_subset    check (timeout_count <= error_count),

    constraint usage_hourly_tokens_nonneg check (
        input_tokens >= 0 and output_tokens >= 0 and cached_input_tokens >= 0 and
        reasoning_tokens >= 0 and total_tokens >= 0
    ),
    constraint usage_hourly_cost_nonneg check (cost_usd >= 0),

    constraint usage_hourly_latency_nonneg check (
        latency_sum_ms >= 0 and latency_count >= 0 and
        (latency_max_ms is null or latency_max_ms >= 0) and
        (latency_p50_ms is null or latency_p50_ms >= 0) and
        (latency_p90_ms is null or latency_p90_ms >= 0) and
        (latency_p95_ms is null or latency_p95_ms >= 0) and
        (latency_p99_ms is null or latency_p99_ms >= 0)
    ),
    constraint usage_hourly_ttft_nonneg check (
        ttft_sum_ms >= 0 and ttft_count >= 0 and
        (ttft_max_ms is null or ttft_max_ms >= 0) and
        (ttft_p50_ms is null or ttft_p50_ms >= 0) and
        (ttft_p90_ms is null or ttft_p90_ms >= 0) and
        (ttft_p95_ms is null or ttft_p95_ms >= 0) and
        (ttft_p99_ms is null or ttft_p99_ms >= 0)
    ),
    -- Tidak mungkin ada lebih banyak pengukuran daripada request.
    constraint usage_hourly_sample_bounded check (latency_count <= request_count and ttft_count <= request_count),

    constraint usage_hourly_latency_hist_shape check (usage_histogram_valid(latency_hist)),
    constraint usage_hourly_ttft_hist_shape    check (usage_histogram_valid(ttft_hist)),
    -- latency_count sengaja redundan dengan total histogram supaya rata-rata bisa
    -- dihitung tanpa membuka array. Dua constraint ini yang menjamin keduanya tidak
    -- pernah berbeda — histogram yang tidak terisi akan membuat P95 rentang gabungan
    -- salah tanpa gejala apa pun.
    constraint usage_hourly_latency_hist_total check (usage_histogram_total(latency_hist) = latency_count),
    constraint usage_hourly_ttft_hist_total    check (usage_histogram_total(ttft_hist)    = ttft_count),

    -- NULLS NOT DISTINCT: lihat keputusan 1 di kepala file.
    constraint usage_hourly_bucket_dims_key unique nulls not distinct (bucket, api_key_id, provider_id, model_id)
) partition by range (bucket);

-- Pola query dashboard selalu "rentang bucket, disaring satu dimensi, terurut bucket",
-- jadi setiap dimensi mendapat indeks yang diawali dimensi itu dan diikuti bucket.
--
-- Tidak ada indeks khusus untuk bucket saja: kunci unique di atas sudah diawali bucket
-- dan melayani pemindaian rentang tanpa filter dimensi. Menambah indeks bucket-only
-- hanya akan menggandakan biaya tulis worker tanpa menambah jalur akses baru.
create index if not exists usage_hourly_api_key_idx  on usage_hourly (api_key_id,  bucket desc);
create index if not exists usage_hourly_provider_idx on usage_hourly (provider_id, bucket desc);
create index if not exists usage_hourly_model_idx    on usage_hourly (model_id,    bucket desc);

drop trigger if exists usage_hourly_set_updated_at on usage_hourly;
create trigger usage_hourly_set_updated_at before update on usage_hourly
    for each row execute function set_updated_at();

comment on table usage_hourly is
    'Rollup pemakaian per jam per (api key, provider, model), dipartisi bulanan. Diisi worker dengan insert ... on conflict do update yang menghitung ulang satu bucket.';
comment on column usage_hourly.bucket is
    'Awal jam, selalu rata di UTC. Konversi ke zona waktu operator dilakukan saat membaca.';
comment on column usage_hourly.provider_name is
    'Cuplikan nama saat rollup dihitung, agar baris tetap terbaca setelah providernya dihapus.';
comment on column usage_hourly.timeout_count is
    'Bagian dari error_count, bukan tambahannya.';
comment on column usage_hourly.latency_p95_ms is
    'Persis untuk bucket ini sendiri. Untuk rentang lebih dari satu baris pakai usage_histogram_percentile(usage_histogram_sum(latency_hist), 0.95).';
comment on column usage_hourly.latency_hist is
    'Histogram latensi batas tetap; satu-satunya kolom latensi yang boleh digabung antar bucket. Batas slotnya dari usage_latency_bounds().';
comment on column usage_hourly.ttft_count is
    'Jumlah request yang punya TTFT (hanya streaming), karena itu biasanya lebih kecil dari request_count.';

-- ---------------------------------------------------------------------------
-- usage_daily — granularitas hari, untuk rentang 30 hari dan rentang kustom
--
-- SENGAJA TIDAK DIPARTISI. Barisnya 24 kali lebih sedikit daripada usage_hourly untuk
-- kombinasi dimensi yang sama, dan inilah tabel yang justru dibaca melintasi rentang
-- panjang — "biaya tahun lalu" tetap terjawab dari sini setelah partisi usage_hourly
-- bulan-bulan itu dibuang. Mempartisinya berarti partisi bulanan berisi sekitar 30
-- baris per kombinasi dimensi: perencana harus mempertimbangkan puluhan partisi untuk
-- satu query rentang panjang, worker pemeliharaan dapat satu induk lagi untuk diurus,
-- dan tidak ada satu pun manfaat retensi karena tabel ini memang disimpan selamanya.
--
-- Bentuk kolomnya identik dengan usage_hourly, hanya bucket-nya rata di hari, sehingga
-- repository dan worker bisa memakai kode yang sama untuk keduanya.
-- ---------------------------------------------------------------------------
create table if not exists usage_daily (
    -- Awal hari, selalu rata di UTC.
    bucket timestamptz not null,

    api_key_id    uuid,
    api_key_name  text,
    provider_id   uuid,
    provider_name text,
    model_id      uuid,
    model_name    text,

    request_count  bigint not null default 0,
    success_count  bigint not null default 0,
    error_count    bigint not null default 0,
    timeout_count  bigint not null default 0,
    retry_count    bigint not null default 0,
    failover_count bigint not null default 0,

    input_tokens        bigint not null default 0,
    output_tokens       bigint not null default 0,
    cached_input_tokens bigint not null default 0,
    reasoning_tokens    bigint not null default 0,
    total_tokens        bigint not null default 0,

    cost_usd numeric(20, 8) not null default 0,

    latency_sum_ms bigint not null default 0,
    latency_count  bigint not null default 0,
    latency_max_ms int,
    latency_p50_ms int,
    latency_p90_ms int,
    latency_p95_ms int,
    latency_p99_ms int,
    latency_hist bigint[] not null default array_fill(0::bigint, array[cardinality(usage_latency_bounds()) + 1]),

    ttft_sum_ms bigint not null default 0,
    ttft_count  bigint not null default 0,
    ttft_max_ms int,
    ttft_p50_ms int,
    ttft_p90_ms int,
    ttft_p95_ms int,
    ttft_p99_ms int,
    ttft_hist bigint[] not null default array_fill(0::bigint, array[cardinality(usage_latency_bounds()) + 1]),

    updated_at timestamptz not null default now(),

    constraint usage_daily_bucket_aligned check (bucket = date_trunc('day', bucket, 'UTC')),

    constraint usage_daily_counts_nonneg check (
        request_count >= 0 and success_count >= 0 and error_count >= 0 and
        timeout_count >= 0 and retry_count   >= 0 and failover_count >= 0
    ),
    constraint usage_daily_outcome_consistent check (success_count + error_count <= request_count),
    constraint usage_daily_timeout_subset    check (timeout_count <= error_count),

    constraint usage_daily_tokens_nonneg check (
        input_tokens >= 0 and output_tokens >= 0 and cached_input_tokens >= 0 and
        reasoning_tokens >= 0 and total_tokens >= 0
    ),
    constraint usage_daily_cost_nonneg check (cost_usd >= 0),

    constraint usage_daily_latency_nonneg check (
        latency_sum_ms >= 0 and latency_count >= 0 and
        (latency_max_ms is null or latency_max_ms >= 0) and
        (latency_p50_ms is null or latency_p50_ms >= 0) and
        (latency_p90_ms is null or latency_p90_ms >= 0) and
        (latency_p95_ms is null or latency_p95_ms >= 0) and
        (latency_p99_ms is null or latency_p99_ms >= 0)
    ),
    constraint usage_daily_ttft_nonneg check (
        ttft_sum_ms >= 0 and ttft_count >= 0 and
        (ttft_max_ms is null or ttft_max_ms >= 0) and
        (ttft_p50_ms is null or ttft_p50_ms >= 0) and
        (ttft_p90_ms is null or ttft_p90_ms >= 0) and
        (ttft_p95_ms is null or ttft_p95_ms >= 0) and
        (ttft_p99_ms is null or ttft_p99_ms >= 0)
    ),
    constraint usage_daily_sample_bounded check (latency_count <= request_count and ttft_count <= request_count),

    constraint usage_daily_latency_hist_shape check (usage_histogram_valid(latency_hist)),
    constraint usage_daily_ttft_hist_shape    check (usage_histogram_valid(ttft_hist)),
    constraint usage_daily_latency_hist_total check (usage_histogram_total(latency_hist) = latency_count),
    constraint usage_daily_ttft_hist_total    check (usage_histogram_total(ttft_hist)    = ttft_count),

    constraint usage_daily_bucket_dims_key unique nulls not distinct (bucket, api_key_id, provider_id, model_id)
);

create index if not exists usage_daily_api_key_idx  on usage_daily (api_key_id,  bucket desc);
create index if not exists usage_daily_provider_idx on usage_daily (provider_id, bucket desc);
create index if not exists usage_daily_model_idx    on usage_daily (model_id,    bucket desc);

drop trigger if exists usage_daily_set_updated_at on usage_daily;
create trigger usage_daily_set_updated_at before update on usage_daily
    for each row execute function set_updated_at();

comment on table usage_daily is
    'Rollup pemakaian per hari. Tidak dipartisi: dibaca melintasi rentang panjang dan disimpan selamanya, termasuk setelah partisi usage_hourly lama dibuang.';
comment on column usage_daily.bucket is
    'Awal hari, selalu rata di UTC. Hari kalender operator dihitung saat membaca, bukan disimpan.';
comment on column usage_daily.latency_hist is
    'Boleh dibangun dengan menggabungkan 24 baris usage_hourly lewat usage_histogram_sum, tanpa membaca requests lagi.';

-- ---------------------------------------------------------------------------
-- Partisi awal usage_hourly
--
-- Bulan ini dan dua bulan ke depan, plus partisi DEFAULT sebagai jaring pengaman,
-- mengikuti pola dan alasan yang sama seperti akhir migrasi 0006: kalau worker
-- pemeliharaan tertinggal, rollup tetap tertulis alih-alih gagal karena tidak ada
-- partisi yang cocok.
-- ---------------------------------------------------------------------------
do $$
declare
    m int;
begin
    for m in 0..2 loop
        perform create_monthly_partition('usage_hourly', (current_date + (m || ' month')::interval)::date);
    end loop;
    execute 'create table if not exists usage_hourly_default partition of usage_hourly default';
end $$;
