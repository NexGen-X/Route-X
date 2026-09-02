-- Migrasi 0006 — log request, timeline percobaan, dan payload.
--
-- Tabel terbesar di sistem ini, dan yang paling sering di-query dashboard. Tiga
-- keputusan yang membentuknya:
--
-- 1. DIPARTISI BULANAN. Log request tumbuh tanpa batas, sementara hampir semua query
--    menyangkut rentang waktu terbatas. Partisi rentang membuat perencana melewati
--    seluruh bulan yang tidak diminta, dan menghapus data lama menjadi DROP TABLE
--    yang instan, bukan DELETE besar yang membuat tabel membengkak.
--
-- 2. TANPA FOREIGN KEY, tetapi menyimpan cuplikan nama. Log adalah catatan fakta yang
--    sudah terjadi. Kalau ada FK ke providers, menghapus provider akan menggagalkan
--    penghapusan atau menghapus riwayatnya; dan pada jalur tulis terpanas, memeriksa
--    FK menambah biaya pada setiap request. Karena itu ID tetap disimpan untuk
--    join opsional, sementara nama disimpan sebagai cuplikan agar log tetap terbaca
--    setelah entitasnya hilang.
--
-- 3. PAYLOAD DIPISAH ke tabelnya sendiri. Body request dan respons jauh lebih besar
--    daripada metrik, dan retensinya lebih pendek. Memisahkannya menjaga baris
--    requests tetap ramping sehingga agregasi dashboard tidak membaca megabyte JSON.

-- Helper pembuat partisi bulanan. Dipakai migrasi ini dan worker pemeliharaan
-- partisi, sehingga aturan penamaan dan rentangnya hanya ada di satu tempat.
create or replace function create_monthly_partition(parent text, month date)
returns text as $$
declare
    start_at date := date_trunc('month', month)::date;
    end_at   date := (date_trunc('month', month) + interval '1 month')::date;
    part     text := format('%s_%s', parent, to_char(start_at, 'YYYYMM'));
begin
    execute format(
        'create table if not exists %I partition of %I for values from (%L) to (%L)',
        part, parent, start_at, end_at
    );
    return part;
end;
$$ language plpgsql;

comment on function create_monthly_partition(text, date) is
    'Membuat partisi bulanan bernama <parent>_YYYYMM bila belum ada. Dipakai migrasi dan worker pemeliharaan.';

-- ---------------------------------------------------------------------------
-- requests
-- ---------------------------------------------------------------------------
create table if not exists requests (
    id         uuid        not null default gen_random_uuid(),
    created_at timestamptz not null default now(),
    -- Nilai X-Request-Id, untuk menautkan baris ini dengan log aplikasi.
    request_id text        not null,

    -- Pemilik lalu lintas. Tanpa FK, lihat catatan 2 di atas.
    api_key_id   uuid,
    api_key_name text,
    user_id      uuid,

    method   text not null,
    endpoint text not null,

    -- Apa yang diminta klien (bisa berupa alias) dan hasil resolusinya.
    requested_model text not null,
    model_id        uuid,
    model_name      text,
    provider_id     uuid,
    provider_name   text,
    -- Nama model di sisi upstream yang benar-benar dipanggil.
    upstream_model  text,

    status_code int  not null,
    error_type  text,
    error_code  text,
    -- Pesan yang SUDAH disaring. Jangan pernah menulis error mentah upstream ke sini.
    error_message text,

    stream boolean not null default false,

    input_tokens        int not null default 0,
    output_tokens       int not null default 0,
    cached_input_tokens int not null default 0,
    reasoning_tokens    int not null default 0,
    total_tokens        int not null default 0,

    -- Presisi 8 desimal: satu request murah bisa berbiaya di bawah satu mikro-dolar,
    -- dan pembulatan pada tingkat baris akan hilang seluruhnya saat diagregasi.
    cost_usd numeric(16, 8) not null default 0,

    -- Waktu dari gateway menerima request sampai respons selesai.
    latency_ms int not null,
    -- Time to first token, hanya terisi pada respons streaming.
    ttft_ms int,
    -- Waktu yang dihabiskan menunggu upstream, tanpa overhead gateway.
    upstream_latency_ms int,

    retry_count    int not null default 0,
    failover_count int not null default 0,

    routing_strategy text,
    -- Ringkasan keputusan routing: kandidat yang dipertimbangkan dan alasan
    -- pemilihannya. Inilah yang ditampilkan tab "Routing" di request inspector.
    routing_decision jsonb not null default '{}'::jsonb,

    client_ip  inet,
    user_agent text,

    primary key (id, created_at),

    constraint requests_status_range   check (status_code between 100 and 599),
    constraint requests_tokens_nonneg  check (
        input_tokens >= 0 and output_tokens >= 0 and cached_input_tokens >= 0 and
        reasoning_tokens >= 0 and total_tokens >= 0
    ),
    constraint requests_cost_nonneg    check (cost_usd >= 0),
    constraint requests_latency_nonneg check (latency_ms >= 0 and (ttft_ms is null or ttft_ms >= 0)),
    constraint requests_retry_nonneg   check (retry_count >= 0 and failover_count >= 0)
) partition by range (created_at);

-- Kunci utama memakai urutan (id, created_at) supaya pencarian satu request lewat id
-- tetap bisa memakai awalan indeks; kunci partisi wajib ikut serta dalam PK.

-- Indeks berikut dibuat pada tabel induk dan otomatis menurun ke semua partisi,
-- termasuk partisi yang dibuat worker di masa depan. Urutannya mencerminkan pola
-- filter dashboard: selalu terbaru dulu, disaring per satu dimensi.
create index if not exists requests_created_idx    on requests (created_at desc);
create index if not exists requests_api_key_idx    on requests (api_key_id, created_at desc);
create index if not exists requests_provider_idx   on requests (provider_id, created_at desc);
create index if not exists requests_model_idx      on requests (model_id, created_at desc);
create index if not exists requests_status_idx     on requests (status_code, created_at desc);
create index if not exists requests_request_id_idx on requests (request_id);
-- Panel "Errors" hanya melihat kegagalan; indeks partial jauh lebih kecil daripada
-- indeks penuh atas status_code dan langsung terurut waktu.
create index if not exists requests_errors_idx on requests (created_at desc)
    where status_code >= 400;

comment on table requests is 'Log request gateway, dipartisi bulanan. Tanpa foreign key: catatan fakta yang harus tetap utuh setelah entitasnya dihapus.';
comment on column requests.routing_decision is 'Kandidat provider yang dipertimbangkan dan alasan pemilihan, untuk tab Routing di request inspector.';
comment on column requests.cost_usd is 'Biaya per request dengan presisi 8 desimal; pembulatan lebih awal akan hilang saat diagregasi.';

-- ---------------------------------------------------------------------------
-- request_events — timeline percobaan
-- ---------------------------------------------------------------------------
create table if not exists request_events (
    id         bigserial   not null,
    request_pk uuid        not null,
    created_at timestamptz not null default now(),
    -- Nomor urut percobaan dalam satu request, mulai dari 1.
    seq        int         not null,
    kind       text        not null,

    provider_id    uuid,
    provider_name  text,
    upstream_model text,

    status_code int,
    latency_ms  int,
    -- Kategori kegagalan yang stabil untuk diagregasi, mis. "timeout", "rate_limit".
    error_kind    text,
    -- Sudah disaring.
    error_message text,

    detail jsonb not null default '{}'::jsonb,

    primary key (id, created_at),

    constraint request_events_seq_positive check (seq >= 1),
    constraint request_events_kind_valid check (kind in (
        'attempt_started', 'attempt_succeeded', 'attempt_failed',
        'retry_scheduled', 'failover', 'circuit_open', 'rate_limited',
        'budget_exceeded', 'content_blocked', 'stream_started', 'stream_completed'
    ))
) partition by range (created_at);

create index if not exists request_events_request_idx on request_events (request_pk, seq);
create index if not exists request_events_created_idx on request_events (created_at desc);
create index if not exists request_events_kind_idx    on request_events (kind, created_at desc);

comment on table request_events is 'Timeline per percobaan dalam satu request: mulai, gagal, retry, failover, circuit terbuka.';

-- ---------------------------------------------------------------------------
-- request_payloads — body request dan respons
-- ---------------------------------------------------------------------------
create table if not exists request_payloads (
    request_pk uuid        not null,
    created_at timestamptz not null default now(),

    -- Header sudah disaring sebelum ditulis: Authorization, X-Api-Key, dan Cookie
    -- tidak boleh pernah sampai ke tabel ini.
    request_headers  jsonb,
    request_body     jsonb,
    response_headers jsonb,
    response_body    jsonb,

    -- Ditandai true bila body dipotong karena melewati batas ukuran simpan.
    truncated  boolean not null default false,
    size_bytes int     not null default 0,

    primary key (request_pk, created_at),

    constraint request_payloads_size_nonneg check (size_bytes >= 0)
) partition by range (created_at);

create index if not exists request_payloads_created_idx on request_payloads (created_at desc);

comment on table request_payloads is 'Body request dan respons, retensi lebih pendek daripada requests. Header wajib sudah disaring.';

-- ---------------------------------------------------------------------------
-- Partisi awal
--
-- Bulan ini dan dua bulan ke depan dibuat sekarang; worker pemeliharaan menambah
-- bulan berikutnya jauh sebelum dibutuhkan.
--
-- Setiap tabel juga mendapat partisi DEFAULT sebagai jaring pengaman: kalau worker
-- gagal atau tertinggal, baris tetap tertulis alih-alih request gagal karena tidak
-- ada partisi yang cocok. Konsekuensinya, memasang partisi baru untuk rentang yang
-- sudah terisi di DEFAULT menuntut pemindahan baris — jadi worker pemeliharaan
-- memperingatkan bila partisi DEFAULT tidak kosong.
-- ---------------------------------------------------------------------------
do $$
declare
    parent text;
    m      int;
begin
    foreach parent in array array['requests', 'request_events', 'request_payloads'] loop
        for m in 0..2 loop
            perform create_monthly_partition(parent, (current_date + (m || ' month')::interval)::date);
        end loop;
        execute format('create table if not exists %I partition of %I default', parent || '_default', parent);
    end loop;
end $$;
