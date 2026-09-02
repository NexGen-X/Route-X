-- Migrasi 0003 — provider upstream, kredensial terenkripsi, egress pool, health check.
--
-- Provider adalah satu tujuan upstream yang bisa dihubungi gateway: OpenAI, Anthropic,
-- Google, layanan apa pun yang bicara protokol OpenAI, atau HTTP custom. BYOK
-- dimodelkan sebagai provider biasa yang dimiliki seorang pengguna, bukan tipe
-- terpisah, sehingga seluruh mesin routing, health check, dan pencatatan pemakaian
-- berlaku sama tanpa cabang khusus.

-- ---------------------------------------------------------------------------
-- egress_pool — jalur keluar untuk permintaan upstream
-- ---------------------------------------------------------------------------
create table if not exists egress_pool (
    id          uuid        primary key default gen_random_uuid(),
    name        text        not null unique,
    kind        text        not null,
    -- URL proxy hampir selalu memuat kredensial, jadi disimpan terenkripsi dengan
    -- format yang sama seperti kredensial provider: v1.<key_id>.<payload>.
    proxy_url_encrypted text not null,
    encryption_key_id   text not null,
    -- Bentuk tersamar untuk ditampilkan di dashboard, mis. "http://user:****@host:3128".
    masked_hint text        not null,

    enabled     boolean     not null default true,
    weight      int         not null default 1,
    region      text,

    last_health_status  text,
    last_health_at      timestamptz,
    last_latency_ms     int,

    created_at  timestamptz not null default now(),
    updated_at  timestamptz not null default now(),
    created_by  uuid references users (id) on delete set null,

    constraint egress_pool_name_not_empty check (length(btrim(name)) > 0),
    constraint egress_pool_kind_valid     check (kind in ('http', 'https', 'socks5')),
    constraint egress_pool_weight_positive check (weight > 0),
    constraint egress_pool_health_valid    check (last_health_status is null
                                                 or last_health_status in ('healthy', 'degraded', 'unhealthy'))
);

create index if not exists egress_pool_enabled_idx on egress_pool (enabled) where enabled;

drop trigger if exists egress_pool_set_updated_at on egress_pool;
create trigger egress_pool_set_updated_at before update on egress_pool
    for each row execute function set_updated_at();

comment on table egress_pool is 'Proxy keluar opsional untuk permintaan upstream. URL-nya terenkripsi karena biasanya memuat kredensial.';

-- ---------------------------------------------------------------------------
-- providers
-- ---------------------------------------------------------------------------
create table if not exists providers (
    id           uuid        primary key default gen_random_uuid(),
    -- Nama pendek untuk dipakai di kode, log, label metrik, dan URL.
    name         text        not null unique,
    display_name text        not null,
    kind         text        not null,
    base_url     text        not null,

    enabled  boolean not null default true,
    -- Makin kecil makin diutamakan pada strategi routing "priority".
    priority int     not null default 100,
    -- Bobot untuk strategi "weighted"; hanya bermakna di antara provider sederajat.
    weight   int     not null default 1,

    timeout_ms  int not null default 120000,
    max_retries int not null default 2,

    -- Batas laju yang dipaksakan gateway SEBELUM memanggil upstream, agar kuota
    -- pihak ketiga tidak terlanggar. NULL berarti tanpa batas dari sisi kami.
    rate_limit_rpm int,
    rate_limit_tpm int,
    -- Batas permintaan bersamaan ke provider ini.
    max_concurrent int,

    egress_pool_id uuid references egress_pool (id) on delete set null,

    -- BYOK: provider yang kredensialnya dibawa pengguna sendiri.
    is_byok       boolean not null default false,
    owner_user_id uuid references users (id) on delete cascade,

    -- Cuplikan kesehatan terkini. Riwayat lengkapnya ada di health_checks; kolom ini
    -- ada supaya dashboard dan pemilihan rute tidak perlu mengagregasi tabel riwayat
    -- pada setiap request.
    last_health_status   text,
    last_health_at       timestamptz,
    last_latency_ms      int,
    consecutive_failures int not null default 0,

    metadata   jsonb       not null default '{}'::jsonb,
    created_at timestamptz not null default now(),
    updated_at timestamptz not null default now(),
    created_by uuid references users (id) on delete set null,

    constraint providers_name_format     check (name ~ '^[a-z0-9][a-z0-9_-]*$'),
    constraint providers_kind_valid      check (kind in ('openai', 'anthropic', 'google', 'openai_compatible', 'custom')),
    constraint providers_base_url_scheme check (base_url ~* '^https?://'),
    constraint providers_priority_range  check (priority between 0 and 100000),
    constraint providers_weight_positive check (weight > 0),
    constraint providers_timeout_range   check (timeout_ms between 1000 and 900000),
    constraint providers_retries_range   check (max_retries between 0 and 10),
    constraint providers_health_valid    check (last_health_status is null
                                                or last_health_status in ('healthy', 'degraded', 'unhealthy')),
    -- Provider BYOK wajib punya pemilik, dan provider bersama tidak boleh punya.
    -- Tanpa aturan ini, kredensial milik satu pengguna bisa terpakai lalu lintas
    -- pengguna lain.
    constraint providers_byok_has_owner  check ((is_byok and owner_user_id is not null)
                                                or (not is_byok and owner_user_id is null))
);

-- Pemilihan kandidat rute membaca provider aktif berurut prioritas pada setiap
-- request, jadi jalur itu dilayani indeks partial.
create index if not exists providers_routing_idx on providers (priority, weight desc)
    where enabled and not is_byok;
create index if not exists providers_byok_owner_idx on providers (owner_user_id) where is_byok;
create index if not exists providers_kind_idx       on providers (kind);
create index if not exists providers_health_idx     on providers (last_health_status)
    where enabled;

drop trigger if exists providers_set_updated_at on providers;
create trigger providers_set_updated_at before update on providers
    for each row execute function set_updated_at();

comment on table providers is 'Tujuan upstream. BYOK adalah provider biasa yang punya owner_user_id, bukan tipe terpisah.';
comment on column providers.priority is 'Makin kecil makin diutamakan pada strategi routing priority.';
comment on column providers.last_health_status is 'Cuplikan hasil health check terakhir; riwayatnya ada di health_checks.';

-- ---------------------------------------------------------------------------
-- provider_credentials
-- ---------------------------------------------------------------------------
create table if not exists provider_credentials (
    id          uuid        primary key default gen_random_uuid(),
    provider_id uuid        not null references providers (id) on delete cascade,
    label       text        not null default 'primary',

    -- Ciphertext AES-256-GCM berformat v1.<key_id>.<base64url(nonce||ciphertext)>.
    -- TIDAK ADA kolom plaintext di tabel ini, dan tidak boleh ditambahkan.
    ciphertext        text not null,
    -- Kunci mana yang mengenkripsi baris ini, supaya rotasi ENCRYPTION_KEY bisa
    -- dilakukan bertahap tanpa mendekripsi seluruh tabel sekaligus.
    encryption_key_id text not null,
    -- Bentuk tersamar untuk dashboard, mis. "sk-****9a21". Bukan rahasia.
    masked_hint       text not null,

    enabled      boolean not null default true,
    last_used_at timestamptz,
    expires_at   timestamptz,
    -- Dinaikkan saat upstream menolak kredensial ini, agar rotasi bisa diingatkan.
    auth_failures int not null default 0,

    created_at timestamptz not null default now(),
    updated_at timestamptz not null default now(),
    created_by uuid references users (id) on delete set null,

    constraint provider_credentials_label_not_empty check (length(btrim(label)) > 0),
    -- Menjaga format ciphertext tetap seperti yang dihasilkan internal/security.
    constraint provider_credentials_ciphertext_format check (ciphertext ~ '^v[0-9]+\.[0-9a-f]+\.[A-Za-z0-9_-]+$'),
    unique (provider_id, label)
);

create index if not exists provider_credentials_active_idx on provider_credentials (provider_id)
    where enabled;
create index if not exists provider_credentials_key_id_idx on provider_credentials (encryption_key_id);

drop trigger if exists provider_credentials_set_updated_at on provider_credentials;
create trigger provider_credentials_set_updated_at before update on provider_credentials
    for each row execute function set_updated_at();

comment on table provider_credentials is 'Kredensial upstream terenkripsi. Tidak ada kolom plaintext dan tidak boleh ditambahkan.';
comment on column provider_credentials.encryption_key_id is 'Pengenal kunci enkripsi yang dipakai baris ini, untuk rotasi bertahap.';

-- ---------------------------------------------------------------------------
-- health_checks — riwayat
-- ---------------------------------------------------------------------------
create table if not exists health_checks (
    id          bigserial   primary key,
    provider_id uuid        not null references providers (id) on delete cascade,
    checked_at  timestamptz not null default now(),

    status      text not null,
    latency_ms  int,
    status_code int,
    -- Kategori kegagalan yang stabil (mis. "timeout", "dns", "auth", "http_5xx")
    -- supaya bisa diagregasi; pesan bebas ada di kolom berikutnya.
    error_kind    text,
    -- Pesan yang SUDAH disaring. Jangan pernah menulis error mentah dari klien HTTP
    -- ke sini: pesan itu bisa memuat URL berkredensial.
    error_message text,

    constraint health_checks_status_valid check (status in ('healthy', 'degraded', 'unhealthy'))
);

create index if not exists health_checks_provider_idx on health_checks (provider_id, checked_at desc);
create index if not exists health_checks_checked_idx  on health_checks (checked_at desc);

comment on table health_checks is 'Riwayat health check provider. error_message wajib sudah disaring sebelum ditulis.';
