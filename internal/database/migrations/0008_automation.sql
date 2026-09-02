-- Migrasi 0008 — aturan routing, penyaring konten, webhook, dan integrasi.
--
-- Semua tabel di file ini adalah KONFIGURASI, bukan log. Bedanya menentukan dua hal
-- yang berlawanan dengan pilihan di 0006:
--
--   * Referensi memakai foreign key sungguhan. Aturan routing yang menyebut model atau
--     provider yang sudah tidak ada bukan catatan sejarah, melainkan konfigurasi rusak
--     yang diam-diam tidak pernah cocok. Integritas referensial memang diinginkan di
--     sini, dan volumenya kecil sehingga biaya pemeriksaannya tidak berarti.
--   * Daftar kandidat provider disimpan sebagai tabel penghubung, bukan uuid[]. Array
--     tidak bisa punya foreign key, jadi menghapus provider akan meninggalkan ID
--     menggantung di dalam array — persis kegagalan yang dihindari 0005 pada
--     pembatasan model per API key.
--
-- Urutan bagian di file ini: routing lebih dulu, lalu integrations, baru content_filters.
-- integrations naik ke depan karena penyaring konten bertipe moderasi menunjuk ke
-- baris integrations lewat foreign key, dan targetnya harus sudah ada saat tabel dibuat.

-- ---------------------------------------------------------------------------
-- routing_rules — pemilihan provider
--
-- Satu aturan menjawab: "untuk lalu lintas yang cocok kondisi ini, pilih provider
-- dengan strategi ini, dan kalau gagal lakukan ini". Evaluasinya berurut priority naik
-- dan ATURAN PERTAMA YANG COCOK MENANG, jadi aturan yang lebih spesifik diberi
-- priority lebih kecil. Kondisi yang kosong berarti "tidak membatasi", sehingga aturan
-- tanpa kondisi apa pun adalah aturan bawaan yang cocok untuk semua lalu lintas.
--
-- Kondisi pencocokan sengaja berupa kolom terpisah, bukan satu jsonb: kondisinya
-- dievaluasi pada jalur request terpanas, dan model serta API key harus bisa dijaga
-- foreign key. Kalau nanti perlu kondisi yang lebih eksotis, kolom baru lebih mudah
-- dipahami daripada skema jsonb yang tumbuh diam-diam.
--
-- Pengaturan retry dan circuit breaker ada di aturan, bukan hanya di provider, karena
-- kesabaran yang pantas berbeda per jenis lalu lintas: permintaan dari dashboard boleh
-- menunggu lama, permintaan interaktif tidak.
--
-- Kandidat provider ada di routing_rule_providers. Aturan TANPA kandidat sah dan
-- bermakna: "pakai strategi ini atas semua provider yang bisa melayani model yang
-- diminta", mengikuti provider_models. Karena itu tidak ada constraint yang memaksa
-- aturan punya kandidat.
-- ---------------------------------------------------------------------------
create table if not exists routing_rules (
    id          uuid primary key default gen_random_uuid(),
    name        text not null,
    description text,

    -- Urutan evaluasi; makin kecil makin dulu diperiksa.
    priority int not null default 100,

    -- Kondisi pencocokan, digabung dengan AND. NULL atau array kosong = tidak dibatasi.
    --
    -- Keduanya cascade saat entitasnya dihapus: aturan yang mensyaratkan model atau
    -- API key tertentu tidak akan pernah bisa cocok lagi setelah entitas itu hilang,
    -- jadi menyimpannya hanya menyisakan konfigurasi mati di dashboard.
    match_model_id   uuid references models (id)   on delete cascade,
    match_api_key_id uuid references api_keys (id) on delete cascade,
    -- Kapabilitas yang diminta request. Dibatasi daftar yang sama seperti
    -- models.capabilities di 0004; salah tulis satu huruf berarti aturan tidak pernah
    -- cocok, dan kegagalan seperti itu tidak memunculkan pesan apa pun saat runtime.
    match_capabilities text[] not null default '{}',

    strategy text not null default 'priority',

    -- Retry di dalam satu percobaan routing.
    max_attempts int not null default 3,
    -- Jeda dasar backoff eksponensial; jitter ditambahkan aplikasi, bukan disimpan.
    backoff_ms int not null default 250,

    -- Circuit breaker per provider dalam konteks aturan ini.
    failure_threshold int not null default 5,
    open_duration_ms  int not null default 30000,
    half_open_probes  int not null default 1,

    enabled boolean not null default true,

    created_at timestamptz not null default now(),
    updated_at timestamptz not null default now(),
    created_by uuid references users (id) on delete set null,

    constraint routing_rules_name_not_empty  check (length(btrim(name)) > 0),
    constraint routing_rules_priority_range  check (priority between 0 and 100000),
    constraint routing_rules_strategy_valid  check (strategy in
        ('priority', 'round_robin', 'weighted', 'lowest_latency', 'lowest_cost', 'capability')),
    constraint routing_rules_capabilities_known check (
        match_capabilities <@ array['text', 'vision', 'reasoning', 'tools', 'embeddings']::text[]
    ),
    constraint routing_rules_attempts_range     check (max_attempts between 1 and 10),
    constraint routing_rules_backoff_range      check (backoff_ms between 0 and 60000),
    constraint routing_rules_failure_threshold  check (failure_threshold between 1 and 1000),
    constraint routing_rules_open_duration      check (open_duration_ms between 1000 and 3600000),
    constraint routing_rules_half_open_range    check (half_open_probes between 1 and 100),
    constraint routing_rules_name_key unique (name)
);

-- Jalur evaluasi: aturan aktif berurut priority. created_at ikut menjadi kolom kedua
-- supaya dua aturan dengan priority sama tetap dievaluasi dalam urutan yang pasti —
-- "aturan pertama yang cocok menang" tidak bermakna kalau urutannya bisa berubah
-- antar query. priority tidak dibuat unique karena operator perlu bisa mengelompokkan
-- aturan sederajat tanpa menomori ulang semuanya.
create index if not exists routing_rules_eval_idx on routing_rules (priority, created_at)
    where enabled;
-- Dua indeks berikut melayani pertanyaan dashboard "aturan mana yang menyebut model /
-- API key ini", sekaligus membuat cascade penghapusan tidak perlu memindai tabel.
create index if not exists routing_rules_model_idx on routing_rules (match_model_id)
    where match_model_id is not null;
create index if not exists routing_rules_api_key_idx on routing_rules (match_api_key_id)
    where match_api_key_id is not null;

drop trigger if exists routing_rules_set_updated_at on routing_rules;
create trigger routing_rules_set_updated_at before update on routing_rules
    for each row execute function set_updated_at();

comment on table routing_rules is
    'Aturan pemilihan provider. Dievaluasi berurut priority naik; aturan pertama yang cocok menang.';
comment on column routing_rules.priority is
    'Makin kecil makin dulu dievaluasi. Priority sama ditengahi created_at agar urutannya pasti.';
comment on column routing_rules.match_capabilities is
    'Kapabilitas yang harus diminta request agar aturan cocok. Kosong berarti tidak dibatasi.';
comment on column routing_rules.backoff_ms is
    'Jeda dasar backoff eksponensial dalam milidetik; jitter ditambahkan aplikasi.';
comment on column routing_rules.half_open_probes is
    'Jumlah permintaan percobaan yang diizinkan lewat saat circuit beralih dari terbuka ke setengah terbuka.';

-- ---------------------------------------------------------------------------
-- routing_rule_providers — kandidat provider dan urutan fallback
--
-- position menentukan urutan fallback di dalam satu aturan dan wajib unik per aturan:
-- dua provider pada urutan yang sama membuat fallback bergantung pada urutan baris
-- yang kebetulan terbaca. weight hanya bermakna untuk strategi weighted dan menimpa
-- providers.weight khusus dalam aturan ini.
-- ---------------------------------------------------------------------------
create table if not exists routing_rule_providers (
    rule_id     uuid not null references routing_rules (id) on delete cascade,
    provider_id uuid not null references providers (id)     on delete cascade,
    -- 1 dicoba lebih dulu.
    position   int not null,
    weight     int,
    created_at timestamptz not null default now(),

    primary key (rule_id, provider_id),

    constraint routing_rule_providers_position_positive check (position >= 1),
    constraint routing_rule_providers_weight_positive  check (weight is null or weight > 0)
);

create unique index if not exists routing_rule_providers_order_key
    on routing_rule_providers (rule_id, position);
create index if not exists routing_rule_providers_provider_idx
    on routing_rule_providers (provider_id);

comment on table routing_rule_providers is
    'Kandidat provider per aturan routing beserta urutan fallback. Tabel penghubung, bukan array, agar provider yang dihapus tidak meninggalkan ID menggantung.';
comment on column routing_rule_providers.weight is
    'Bobot khusus aturan ini untuk strategi weighted; NULL memakai providers.weight.';

-- ---------------------------------------------------------------------------
-- integrations — konfigurasi layanan pihak ketiga
--
-- Satu baris adalah satu sambungan ke layanan luar: Cloudflare untuk WAF dan DNS,
-- penyedia moderasi konten, tujuan notifikasi, dan sejenisnya. Kredensialnya mengikuti
-- pola provider_credentials di 0003 — ciphertext + encryption_key_id + masked_hint,
-- TANPA kolom plaintext dan tidak boleh ditambahkan.
--
-- kind hanya dijaga bentuknya, bukan didaftar dalam check constraint. Ini berbeda dari
-- pilihan pada models.capabilities atau webhooks.events, dan sengaja: daftar integrasi
-- bertambah setiap rilis, sementara mengubah check constraint menuntut migrasi baru
-- untuk sesuatu yang sudah divalidasi di tempat yang lebih tahu — aplikasi hanya punya
-- handler untuk kind yang memang didukungnya, dan menandai baris dengan kind tak
-- dikenal sebagai tidak didukung di dashboard. Contoh nilai: 'cloudflare',
-- 'openai_moderation', 'azure_content_safety', 'slack'.
--
-- Kredensial boleh kosong seluruhnya, karena tidak semua integrasi punya rahasia.
-- Yang tidak boleh adalah terisi sebagian: ciphertext tanpa encryption_key_id tidak
-- bisa didekripsi lagi setelah kunci dirotasi.
--
-- config menampung pengaturan yang BUKAN rahasia (zone id, nama akun, ambang batas).
-- Alasannya sama seperti settings di 0001: apa pun di kolom ini terbaca siapa saja
-- yang bisa membaca database atau membuka panel admin.
-- ---------------------------------------------------------------------------
create table if not exists integrations (
    id   uuid primary key default gen_random_uuid(),
    kind text not null,
    -- Beberapa sambungan untuk kind yang sama dibedakan namanya, mis. dua akun Cloudflare.
    name         text not null default 'default',
    display_name text,

    enabled boolean not null default true,

    -- Ciphertext AES-256-GCM berformat v1.<key_id>.<base64url(nonce||ciphertext)>.
    credential_ciphertext text,
    encryption_key_id     text,
    -- Bentuk tersamar untuk dashboard, mis. "cf_****3f9a". Bukan rahasia.
    masked_hint text,

    config jsonb not null default '{}'::jsonb,

    -- Hasil uji koneksi terakhir. Memakai kosakata status yang sama dengan
    -- providers.last_health_status supaya dashboard bisa memakai komponen yang sama.
    last_check_status text,
    last_check_at     timestamptz,
    -- Sudah disaring: pesan mentah dari klien HTTP bisa memuat URL berkredensial.
    last_check_error text,

    created_at timestamptz not null default now(),
    updated_at timestamptz not null default now(),
    created_by uuid references users (id) on delete set null,

    constraint integrations_kind_format     check (kind ~ '^[a-z0-9][a-z0-9_]*$'),
    constraint integrations_name_not_empty  check (length(btrim(name)) > 0),
    constraint integrations_credential_format check (
        credential_ciphertext is null or credential_ciphertext ~ '^v[0-9]+\.[0-9a-f]+\.[A-Za-z0-9_-]+$'
    ),
    constraint integrations_credential_complete check (
        (credential_ciphertext is     null and encryption_key_id is     null and masked_hint is     null) or
        (credential_ciphertext is not null and encryption_key_id is not null and masked_hint is not null)
    ),
    constraint integrations_status_valid check (
        last_check_status is null or last_check_status in ('healthy', 'degraded', 'unhealthy')
    ),
    constraint integrations_kind_name_key unique (kind, name)
);

create index if not exists integrations_enabled_idx  on integrations (kind) where enabled;
-- Rotasi ENCRYPTION_KEY memerlukan daftar baris per kunci, sama seperti pada
-- provider_credentials di 0003.
create index if not exists integrations_key_id_idx on integrations (encryption_key_id)
    where encryption_key_id is not null;

drop trigger if exists integrations_set_updated_at on integrations;
create trigger integrations_set_updated_at before update on integrations
    for each row execute function set_updated_at();

comment on table integrations is
    'Sambungan ke layanan pihak ketiga. Kredensial terenkripsi mengikuti pola provider_credentials; tidak ada kolom plaintext.';
comment on column integrations.kind is
    'Jenis integrasi, mis. cloudflare atau openai_moderation. Hanya bentuknya dijaga: daftar yang didukung ada di aplikasi.';
comment on column integrations.config is
    'Pengaturan non-rahasia (zone id, nama akun, ambang batas). Rahasia harus masuk credential_ciphertext.';
comment on column integrations.last_check_error is
    'Pesan hasil uji koneksi yang sudah disaring; jangan menulis error mentah klien HTTP ke sini.';

-- ---------------------------------------------------------------------------
-- content_filters — penyaring permintaan dan respons
--
-- Satu baris adalah satu aturan, dan kind menentukan kolom mana yang bermakna:
--
--   blocked_pattern      tolak bila pola cocok
--   allowed_pattern      tolak bila pola TIDAK cocok (daftar putih, logikanya terbalik)
--   model_restriction    tolak permintaan ke model tertentu
--   provider_restriction tolak perutean ke provider tertentu
--   request_size         tolak permintaan yang melebihi batas ukuran
--   moderation           serahkan penilaian ke penyedia moderasi di integrations
--
-- 'moderation' SUDAH ADA di daftar kind sejak sekarang meskipun implementasinya belum,
-- dan itu inti dari kesiapan integrasi penyedia moderasi: kalau kind-nya ditambahkan
-- nanti, yang berubah adalah check constraint — dan mengubah constraint di migrasi yang
-- sudah diterapkan tidak mungkin, jadi harus lewat migrasi baru. Kolom moderation_*
-- ada karena alasan yang sama. Kredensial penyedia TIDAK disimpan di sini: baris ini
-- hanya menunjuk ke integrations, sehingga hanya ada satu tempat rahasia disimpan dan
-- rotasi kunci tetap bekerja. Foreign key-nya on delete restrict, bukan cascade atau
-- set null: menghapus integrasi yang masih dipakai penyaring aktif akan membuat
-- penyaring itu berhenti menilai apa pun tanpa ada yang tahu.
--
-- POLA DAN ReDoS. pattern_type menyatakan pola dibaca sebagai substring atau regex.
-- Substring adalah default yang dianjurkan justru karena regex dari input operator
-- bisa dibuat berperilaku eksponensial terhadap panjang input (ReDoS) — dan input di
-- sini adalah prompt pengguna, yang panjangnya tidak dikendalikan operator. Database
-- tidak bisa mencegah itu: pola disimpan sebagai teks dan tidak pernah dieksekusi oleh
-- PostgreSQL di sini. Tanggung jawabnya ada di lapisan aplikasi, yang WAJIB
--   1. memvalidasi dan mengompilasi pola saat disimpan, bukan saat request masuk, dan
--      menolak pola yang tidak bisa dikompilasi;
--   2. menjalankan pencocokan dengan batas waktu, dan menghitung pola yang melewatinya.
-- max_eval_ms adalah batas waktu itu — disimpan per aturan supaya operator bisa memberi
-- pola yang memang mahal anggaran lebih besar tanpa melonggarkan yang lain.
-- eval_timeout_count dan last_timeout_at adalah umpan balik dari penegakan itu, dan
-- itulah yang membuat pola bermasalah kelihatan di dashboard alih-alih hanya
-- memperlambat setiap request diam-diam.
-- ---------------------------------------------------------------------------
create table if not exists content_filters (
    id          uuid primary key default gen_random_uuid(),
    name        text not null,
    description text,

    kind text not null,
    -- Urutan evaluasi; makin kecil makin dulu. Penyaring yang menolak lebih murah
    -- sebaiknya didahulukan agar permintaan buruk berhenti sebelum pola yang mahal.
    priority int not null default 100,
    -- Bagian mana yang diperiksa.
    applies_to text not null default 'request',
    -- Tindakan saat aturan terpicu. 'warn' hanya mencatat dan memicu notifikasi.
    action text not null default 'block',

    -- kind = blocked_pattern / allowed_pattern
    pattern        text,
    pattern_type   text,
    case_sensitive boolean not null default false,
    -- Anggaran waktu pencocokan per aturan, dipaksakan aplikasi (lihat catatan ReDoS).
    max_eval_ms int not null default 50,
    -- Umpan balik penegakan batas waktu di atas.
    eval_timeout_count bigint not null default 0,
    last_timeout_at    timestamptz,

    -- kind = model_restriction / provider_restriction
    model_id    uuid references models (id)    on delete cascade,
    provider_id uuid references providers (id) on delete cascade,

    -- kind = request_size
    max_request_bytes bigint,

    -- kind = moderation
    moderation_integration_id uuid references integrations (id) on delete restrict,
    -- Kategori yang dianggap pelanggaran. Taksonomi kategorinya milik penyedia, jadi
    -- sengaja tidak dibatasi daftar di sini.
    moderation_categories text[] not null default '{}',
    -- Ambang skor 0..1; di atas ambang berarti terpicu.
    moderation_threshold numeric(5, 4),
    moderation_config    jsonb not null default '{}'::jsonb,

    enabled boolean not null default true,

    created_at timestamptz not null default now(),
    updated_at timestamptz not null default now(),
    created_by uuid references users (id) on delete set null,

    constraint content_filters_name_not_empty check (length(btrim(name)) > 0),
    constraint content_filters_kind_valid check (kind in (
        'blocked_pattern', 'allowed_pattern', 'model_restriction',
        'provider_restriction', 'request_size', 'moderation'
    )),
    constraint content_filters_priority_range check (priority between 0 and 100000),
    constraint content_filters_applies_to_valid check (applies_to in ('request', 'response', 'both')),
    constraint content_filters_action_valid     check (action in ('block', 'warn')),
    constraint content_filters_pattern_type_valid check (
        pattern_type is null or pattern_type in ('substring', 'regex')
    ),
    constraint content_filters_pattern_not_empty check (pattern is null or length(pattern) > 0),
    constraint content_filters_eval_budget_range check (max_eval_ms between 1 and 5000),
    constraint content_filters_timeout_count_nonneg check (eval_timeout_count >= 0),
    constraint content_filters_size_positive check (max_request_bytes is null or max_request_bytes > 0),
    constraint content_filters_threshold_range check (
        moderation_threshold is null or moderation_threshold between 0 and 1
    ),
    -- Aturan yang kolom pentingnya kosong tidak melakukan apa pun tetapi tetap
    -- dievaluasi pada setiap request, dan lebih buruk lagi: operator menyangka
    -- perlindungannya aktif. Karena itu bentuknya dijaga sesuai kind.
    constraint content_filters_shape_valid check (
        case kind
            when 'blocked_pattern'      then pattern is not null and pattern_type is not null
            when 'allowed_pattern'      then pattern is not null and pattern_type is not null
            when 'model_restriction'    then model_id is not null
            when 'provider_restriction' then provider_id is not null
            when 'request_size'         then max_request_bytes is not null
            when 'moderation'           then moderation_integration_id is not null
            else false
        end
    ),
    constraint content_filters_name_key unique (name)
);

-- Jalur evaluasi: penyaring aktif berurut priority, dengan created_at sebagai penengah
-- agar urutannya pasti seperti pada routing_rules.
create index if not exists content_filters_eval_idx on content_filters (priority, created_at)
    where enabled;
create index if not exists content_filters_kind_idx on content_filters (kind);
-- Melayani pertanyaan "penyaring mana yang menyebut entitas ini" dan menghindari
-- pemindaian tabel saat entitasnya dihapus.
create index if not exists content_filters_model_idx on content_filters (model_id)
    where model_id is not null;
create index if not exists content_filters_provider_idx on content_filters (provider_id)
    where provider_id is not null;
create index if not exists content_filters_moderation_idx on content_filters (moderation_integration_id)
    where moderation_integration_id is not null;

drop trigger if exists content_filters_set_updated_at on content_filters;
create trigger content_filters_set_updated_at before update on content_filters
    for each row execute function set_updated_at();

comment on table content_filters is
    'Aturan penyaring permintaan dan respons. kind menentukan kolom mana yang bermakna; bentuknya dijaga content_filters_shape_valid.';
comment on column content_filters.kind is
    'blocked_pattern, allowed_pattern, model_restriction, provider_restriction, request_size, atau moderation.';
comment on column content_filters.pattern_type is
    'substring atau regex. Substring dianjurkan karena regex dari input operator bisa berperilaku eksponensial (ReDoS).';
comment on column content_filters.max_eval_ms is
    'Batas waktu pencocokan pola yang dipaksakan aplikasi, bukan database. PostgreSQL tidak pernah mengeksekusi pola ini.';
comment on column content_filters.eval_timeout_count is
    'Berapa kali pencocokan pola ini melewati max_eval_ms; dipakai dashboard untuk menandai pola bermasalah.';
comment on column content_filters.moderation_integration_id is
    'Penyedia moderasi di tabel integrations. Kredensialnya tidak pernah disimpan di baris ini.';

-- ---------------------------------------------------------------------------
-- webhooks — pendaftaran endpoint
--
-- Secret HMAC-nya terenkripsi mengikuti pola provider_credentials di 0003, dengan
-- format ciphertext yang sama dan tanpa kolom plaintext. Berbeda dari integrations,
-- secret di sini WAJIB ada: payload webhook selalu ditandatangani, karena penerima
-- tidak punya cara lain memastikan permintaan itu benar dari gateway ini.
--
-- events dibatasi daftar yang dikenali, tidak seperti integrations.kind. Alasannya
-- terbalik: nama event di sini diketik operator, dan nama yang salah tulis membuat
-- webhook tidak pernah terpicu tanpa satu pun pesan error. Nilai '*' berarti berlangganan
-- semua event, termasuk event yang ditambahkan rilis berikutnya. Konsekuensi yang
-- diterima: menambah event baru menuntut migrasi baru.
--
-- Tiga kolom last_* adalah cuplikan hasil pengiriman terakhir, ada supaya daftar
-- webhook di dashboard tidak perlu mengagregasi webhook_deliveries pada setiap
-- pembukaan halaman. Riwayat lengkapnya tetap di tabel itu.
-- ---------------------------------------------------------------------------
create table if not exists webhooks (
    id   uuid primary key default gen_random_uuid(),
    name text not null,
    url  text not null,

    events text[] not null,

    -- Ciphertext AES-256-GCM berformat v1.<key_id>.<base64url(nonce||ciphertext)>.
    -- Tidak ada kolom plaintext di tabel ini, dan tidak boleh ditambahkan.
    secret_ciphertext text not null,
    encryption_key_id text not null,
    -- Bentuk tersamar untuk dashboard, mis. "whsec_****1b7e". Bukan rahasia.
    masked_hint text not null,

    enabled boolean not null default true,
    -- Jumlah percobaan ulang setelah percobaan pertama gagal; 0 berarti sekali kirim saja.
    max_retries int not null default 5,
    timeout_ms  int not null default 10000,

    last_delivery_at     timestamptz,
    last_delivery_status text,
    -- Dipakai dashboard untuk menandai endpoint yang mati, dan aplikasi untuk
    -- menonaktifkan otomatis bila perlu.
    consecutive_failures int not null default 0,

    created_at timestamptz not null default now(),
    updated_at timestamptz not null default now(),
    created_by uuid references users (id) on delete set null,

    constraint webhooks_name_not_empty check (length(btrim(name)) > 0),
    constraint webhooks_url_scheme     check (url ~* '^https?://'),
    constraint webhooks_events_not_empty check (cardinality(events) > 0),
    constraint webhooks_events_known check (events <@ array[
        '*',
        'request.completed', 'request.failed',
        'provider.unhealthy', 'provider.recovered',
        'circuit.opened', 'circuit.closed',
        'budget.threshold', 'budget.exceeded',
        'ratelimit.exceeded', 'content.blocked',
        'api_key.created', 'api_key.revoked', 'ban.created'
    ]::text[]),
    constraint webhooks_secret_format check (secret_ciphertext ~ '^v[0-9]+\.[0-9a-f]+\.[A-Za-z0-9_-]+$'),
    constraint webhooks_retries_range check (max_retries between 0 and 20),
    constraint webhooks_timeout_range check (timeout_ms between 1000 and 120000),
    constraint webhooks_failures_nonneg check (consecutive_failures >= 0),
    constraint webhooks_last_status_valid check (
        last_delivery_status is null or last_delivery_status in ('delivered', 'failed', 'abandoned')
    ),
    constraint webhooks_name_key unique (name)
);

create index if not exists webhooks_enabled_idx on webhooks (enabled) where enabled;
-- Pencarian pelanggan satu event: "webhook aktif mana yang berlangganan event ini".
-- Array dicari per elemen, jadi indeksnya GIN, sama seperti models.capabilities di 0004.
create index if not exists webhooks_events_idx on webhooks using gin (events);
create index if not exists webhooks_key_id_idx on webhooks (encryption_key_id);

drop trigger if exists webhooks_set_updated_at on webhooks;
create trigger webhooks_set_updated_at before update on webhooks
    for each row execute function set_updated_at();

comment on table webhooks is
    'Pendaftaran endpoint webhook. Secret HMAC-nya terenkripsi dan wajib ada: payload selalu ditandatangani.';
comment on column webhooks.events is
    'Event yang dilangganan. Nilai * berarti semua event, termasuk yang ditambahkan rilis berikutnya.';
comment on column webhooks.max_retries is
    'Percobaan ulang setelah percobaan pertama gagal. Pengiriman melewati batas ini berstatus abandoned.';
comment on column webhooks.consecutive_failures is
    'Kegagalan berurutan sejak keberhasilan terakhir; dipakai menandai atau menonaktifkan endpoint mati.';

-- ---------------------------------------------------------------------------
-- webhook_deliveries — antrean kerja sekaligus riwayat pengiriman
--
-- Tabel ini bukan log: worker MENGAMBIL baris dari sini, jadi bentuknya ditentukan pola
-- pengambilan, bukan pola pembacaan dashboard. Query pengambilannya:
--
--     select * from webhook_deliveries
--     where status in ('pending', 'failed') and next_attempt_at <= now()
--     order by next_attempt_at
--     limit $1
--     for update skip locked;
--
-- SKIP LOCKED-lah yang membuat beberapa instance worker tidak saling mengambil baris
-- yang sama: baris yang sudah dikunci transaksi lain dilewati, bukan ditunggu, sehingga
-- tidak ada worker yang menganggur menunggu kunci dan tidak ada baris terkirim dua kali
-- dalam satu putaran. Indeks webhook_deliveries_due_idx dibuat persis untuk query itu:
-- partial atas status yang masih perlu dikerjakan, diurut next_attempt_at, sehingga
-- pengambilan tidak pernah menyentuh — bahkan tidak mengindeks — riwayat yang sudah
-- selesai. Itu penting karena riwayat akan jauh lebih besar daripada antreannya.
--
-- Status 'delivering' plus sewa locked_at/locked_by ada untuk kasus worker mati di
-- tengah pengiriman. SKIP LOCKED sendiri sudah cukup selama transaksi hidup, tetapi
-- begitu proses hilang, kunci lepas sementara statusnya tertinggal di 'delivering'
-- selamanya. Worker pemulih mengembalikannya:
--
--     update webhook_deliveries
--     set status = 'pending', locked_at = null, locked_by = null
--     where status = 'delivering' and locked_at < now() - interval '5 minutes';
--
-- webhook_deliveries_stuck_idx melayani query itu. Constraint
-- webhook_deliveries_lease_consistent menjaga sewa dan status tidak pernah berselisih:
-- keluar dari 'delivering' berarti sewanya wajib dibersihkan dalam pernyataan yang sama.
--
-- Tabel ini TIDAK dipartisi walau tumbuh terus: barisnya diperbarui di tempat berkali-
-- kali (status, attempt_count, next_attempt_at), yang tidak cocok dengan tabel
-- terpartisi bila pembaruan sampai memindahkan baris antar partisi, dan volumenya
-- beberapa tingkat di bawah requests. Pembersihannya jadi tugas worker retensi: hapus
-- baris delivered dan abandoned yang sudah lewat masa simpan.
-- ---------------------------------------------------------------------------
create table if not exists webhook_deliveries (
    id         bigserial primary key,
    webhook_id uuid not null references webhooks (id) on delete cascade,
    -- Event konkret; bentuknya saja yang dijaga, tidak didaftar seperti webhooks.events,
    -- karena nilai di sini dihasilkan kode, bukan diketik operator.
    event text not null,
    -- Badan yang dikirim apa adanya. Nilai sensitif WAJIB sudah tersamar sebelum masuk.
    payload jsonb not null,

    status        text not null default 'pending',
    attempt_count int  not null default 0,
    -- Kapan percobaan berikutnya boleh dijalankan. Backoff dihitung aplikasi lalu
    -- disimpan di sini, sehingga antrean tetap benar walau worker restart.
    next_attempt_at timestamptz not null default now(),

    -- Sewa worker; lihat catatan pemulihan di atas.
    locked_at timestamptz,
    locked_by text,

    response_status_code int,
    -- Sudah disaring. Jangan pernah menulis error mentah klien HTTP ke sini: URL
    -- webhook bisa memuat token di query string.
    error_message text,

    created_at   timestamptz not null default now(),
    delivered_at timestamptz,

    constraint webhook_deliveries_event_format check (event ~ '^[a-z][a-z0-9_]*\.[a-z][a-z0-9_]*$'),
    constraint webhook_deliveries_status_valid check (
        status in ('pending', 'delivering', 'delivered', 'failed', 'abandoned')
    ),
    constraint webhook_deliveries_attempts_nonneg check (attempt_count >= 0),
    constraint webhook_deliveries_response_range check (
        response_status_code is null or response_status_code between 100 and 599
    ),
    -- Status dan timestamp tidak boleh saling bertentangan, sama seperti
    -- api_keys_revoked_consistent di 0005.
    constraint webhook_deliveries_delivered_consistent check ((status = 'delivered') = (delivered_at is not null)),
    constraint webhook_deliveries_lease_consistent    check ((status = 'delivering') = (locked_at is not null))
);

-- Antrean: hanya baris yang masih perlu dikerjakan, diurut waktu kelayakannya.
create index if not exists webhook_deliveries_due_idx on webhook_deliveries (next_attempt_at)
    where status in ('pending', 'failed');
-- Pemulihan baris yang ditinggalkan worker yang mati.
create index if not exists webhook_deliveries_stuck_idx on webhook_deliveries (locked_at)
    where status = 'delivering';
-- Riwayat per webhook di dashboard, terbaru dulu.
create index if not exists webhook_deliveries_webhook_idx on webhook_deliveries (webhook_id, created_at desc);
-- Pembersihan oleh worker retensi.
create index if not exists webhook_deliveries_created_idx on webhook_deliveries (created_at)
    where status in ('delivered', 'abandoned');

comment on table webhook_deliveries is
    'Antrean dan riwayat pengiriman webhook. Diambil worker dengan for update skip locked; webhook_deliveries_due_idx melayani query itu.';
comment on column webhook_deliveries.next_attempt_at is
    'Waktu percobaan berikutnya boleh dijalankan. Backoff dihitung aplikasi dan disimpan di sini agar tahan restart.';
comment on column webhook_deliveries.locked_at is
    'Sewa worker. Wajib null di luar status delivering, dijaga webhook_deliveries_lease_consistent.';
comment on column webhook_deliveries.locked_by is
    'Pengenal instance worker pemegang sewa, untuk menelusuri pengiriman yang tersangkut.';
comment on column webhook_deliveries.status is
    'pending, delivering, delivered, failed (akan dicoba lagi), atau abandoned (melewati max_retries).';
comment on column webhook_deliveries.payload is
    'Badan yang dikirim apa adanya; nilai sensitif harus sudah tersamar sebelum ditulis.';
