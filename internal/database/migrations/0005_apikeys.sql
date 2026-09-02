-- Migrasi 0005 — API key, batas laju, anggaran, dan blokir.
--
-- API key adalah kredensial yang dipakai klien terhadap gateway ini, berbeda dari
-- kredensial provider di 0003 yang dipakai gateway terhadap upstream.

-- ---------------------------------------------------------------------------
-- api_keys
-- ---------------------------------------------------------------------------
create table if not exists api_keys (
    id   uuid primary key default gen_random_uuid(),
    name text not null,

    -- HMAC-SHA256 hex dari key, memakai pepper server. Deterministik supaya bisa
    -- dicari lewat indeks pada setiap request; nilai mentahnya tidak pernah disimpan.
    key_hash   text not null unique,
    -- Prefix dan empat karakter terakhir cukup untuk mengenali key di dashboard
    -- tanpa menyimpan apa pun yang bisa dipakai untuk autentikasi.
    key_prefix text not null,
    last4      text not null,

    owner_user_id uuid not null references users (id) on delete cascade,
    status        text not null default 'active',

    -- Cakupan izin key. Kosong berarti hanya inferensi, tanpa akses admin.
    scopes text[] not null default '{}',

    -- Batas laju khusus key ini. NULL berarti memakai batas dari tabel rate_limits
    -- pada cakupan yang lebih luas.
    rate_limit_rps int,
    rate_limit_rpm int,
    rate_limit_tpm int,
    daily_request_limit   bigint,
    monthly_request_limit bigint,
    daily_token_limit     bigint,
    monthly_token_limit   bigint,

    -- Pembatasan asal permintaan. Kosong berarti dari mana saja. Tipe cidr, bukan
    -- text, supaya pencocokan subnet dilakukan PostgreSQL dan format yang salah
    -- ditolak saat penyimpanan, bukan saat penegakan.
    ip_allowlist cidr[] not null default '{}',

    expires_at   timestamptz,
    last_used_at timestamptz,
    last_used_ip inet,

    revoked_at    timestamptz,
    revoked_by    uuid references users (id) on delete set null,
    revoke_reason text,

    -- Jejak rotasi: key baru menunjuk ke key yang digantikannya.
    rotated_from uuid references api_keys (id) on delete set null,

    metadata   jsonb       not null default '{}'::jsonb,
    created_at timestamptz not null default now(),
    updated_at timestamptz not null default now(),
    created_by uuid references users (id) on delete set null,

    constraint api_keys_name_not_empty  check (length(btrim(name)) > 0),
    constraint api_keys_status_valid    check (status in ('active', 'disabled', 'revoked')),
    constraint api_keys_prefix_valid    check (key_prefix in ('sk_live_', 'sk_test_')),
    constraint api_keys_last4_len       check (length(last4) = 4),
    constraint api_keys_hash_format     check (key_hash ~ '^[0-9a-f]{64}$'),
    constraint api_keys_scopes_known    check (
        scopes <@ array['inference', 'models:read', 'usage:read', 'admin:read', 'admin:write']::text[]
    ),
    constraint api_keys_limits_positive check (
        (rate_limit_rps        is null or rate_limit_rps        > 0) and
        (rate_limit_rpm        is null or rate_limit_rpm        > 0) and
        (rate_limit_tpm        is null or rate_limit_tpm        > 0) and
        (daily_request_limit   is null or daily_request_limit   > 0) and
        (monthly_request_limit is null or monthly_request_limit > 0) and
        (daily_token_limit     is null or daily_token_limit     > 0) and
        (monthly_token_limit   is null or monthly_token_limit   > 0)
    ),
    -- Key yang dicabut wajib punya waktu pencabutan, dan sebaliknya. Tanpa ini,
    -- status dan timestamp bisa saling bertentangan.
    constraint api_keys_revoked_consistent check (
        (status = 'revoked' and revoked_at is not null) or
        (status <> 'revoked' and revoked_at is null)
    ),
    -- Key tidak boleh menjadi rotasi dari dirinya sendiri.
    constraint api_keys_rotation_not_self check (rotated_from is null or rotated_from <> id)
);

-- Autentikasi setiap request inferensi adalah pencarian tepat pada key_hash; sudah
-- dilayani indeks unique. Indeks berikut melayani dashboard dan worker.
create index if not exists api_keys_owner_idx   on api_keys (owner_user_id, created_at desc);
create index if not exists api_keys_status_idx  on api_keys (status) where status = 'active';
create index if not exists api_keys_expires_idx on api_keys (expires_at)
    where expires_at is not null and status = 'active';

drop trigger if exists api_keys_set_updated_at on api_keys;
create trigger api_keys_set_updated_at before update on api_keys
    for each row execute function set_updated_at();

comment on table api_keys is 'Kredensial klien terhadap gateway. Hanya hash HMAC yang disimpan, bukan key aslinya.';
comment on column api_keys.ip_allowlist is 'Tipe cidr agar pencocokan subnet dikerjakan PostgreSQL dan format salah ditolak lebih awal.';

-- ---------------------------------------------------------------------------
-- Pembatasan model dan provider per key
--
-- Dibuat sebagai tabel penghubung, bukan array teks, supaya menghapus model atau
-- provider otomatis membersihkan pembatasan yang menyebutkannya. Dengan array teks,
-- nama yang sudah tidak ada akan tertinggal dan diam-diam mempersempit akses key.
-- ---------------------------------------------------------------------------
create table if not exists api_key_allowed_models (
    api_key_id uuid not null references api_keys (id) on delete cascade,
    model_id   uuid not null references models (id)   on delete cascade,
    primary key (api_key_id, model_id)
);

create table if not exists api_key_allowed_providers (
    api_key_id  uuid not null references api_keys (id)   on delete cascade,
    provider_id uuid not null references providers (id)  on delete cascade,
    primary key (api_key_id, provider_id)
);

create index if not exists api_key_allowed_models_model_idx       on api_key_allowed_models (model_id);
create index if not exists api_key_allowed_providers_provider_idx on api_key_allowed_providers (provider_id);

comment on table api_key_allowed_models is 'Daftar putih model per key. Tidak ada baris berarti semua model yang aktif diizinkan.';

-- ---------------------------------------------------------------------------
-- rate_limits — batas pada cakupan selain key
-- ---------------------------------------------------------------------------
create table if not exists rate_limits (
    id    uuid primary key default gen_random_uuid(),
    scope text not null,
    -- Pengenal entitas yang dibatasi. NULL hanya untuk scope 'global'.
    scope_id text,

    requests_per_second   int,
    requests_per_minute   int,
    tokens_per_minute     int,
    daily_request_limit   bigint,
    monthly_request_limit bigint,
    daily_token_limit     bigint,
    monthly_token_limit   bigint,

    enabled    boolean     not null default true,
    created_at timestamptz not null default now(),
    updated_at timestamptz not null default now(),
    created_by uuid references users (id) on delete set null,

    constraint rate_limits_scope_valid check (scope in ('global', 'api_key', 'user', 'provider', 'model', 'ip')),
    constraint rate_limits_scope_id_presence check (
        (scope = 'global' and scope_id is null) or (scope <> 'global' and scope_id is not null)
    ),
    -- Baris tanpa satu pun batas terisi tidak melakukan apa pun tetapi tetap
    -- dievaluasi pada jalur request; lebih baik ditolak saat dibuat.
    constraint rate_limits_at_least_one check (
        coalesce(requests_per_second, requests_per_minute, tokens_per_minute) is not null or
        coalesce(daily_request_limit, monthly_request_limit, daily_token_limit, monthly_token_limit) is not null
    )
);

-- Satu baris per cakupan. 'global' memakai indeks terpisah karena scope_id-nya NULL
-- dan NULL tidak dianggap sama dalam indeks unique biasa.
create unique index if not exists rate_limits_scope_key on rate_limits (scope, scope_id)
    where scope_id is not null;
create unique index if not exists rate_limits_global_key on rate_limits ((true))
    where scope = 'global';
create index if not exists rate_limits_enabled_idx on rate_limits (scope) where enabled;

drop trigger if exists rate_limits_set_updated_at on rate_limits;
create trigger rate_limits_set_updated_at before update on rate_limits
    for each row execute function set_updated_at();

comment on table rate_limits is 'Batas laju pada cakupan global, user, provider, model, atau IP. Batas per key ada di kolom api_keys.';

-- ---------------------------------------------------------------------------
-- budgets
-- ---------------------------------------------------------------------------
create table if not exists budgets (
    id       uuid primary key default gen_random_uuid(),
    name     text not null,
    scope    text not null,
    scope_id text,
    period   text not null,

    -- Nilai uang selalu numeric, tidak pernah floating point.
    limit_usd numeric(14, 6) not null,
    -- Akumulasi pemakaian pada periode berjalan. Direset worker saat periode berganti.
    spent_usd numeric(14, 6) not null default 0,

    period_start timestamptz not null default now(),
    period_end   timestamptz,

    -- Tindakan saat anggaran terlampaui. 'warn' hanya memicu notifikasi; 'block'
    -- menolak request baru.
    action_on_exceed    text not null default 'block',
    alert_threshold_pct int  not null default 80,
    alerted_at          timestamptz,

    enabled    boolean     not null default true,
    created_at timestamptz not null default now(),
    updated_at timestamptz not null default now(),
    created_by uuid references users (id) on delete set null,

    constraint budgets_name_not_empty  check (length(btrim(name)) > 0),
    constraint budgets_scope_valid     check (scope in ('global', 'api_key', 'user', 'provider', 'model')),
    constraint budgets_scope_id_presence check (
        (scope = 'global' and scope_id is null) or (scope <> 'global' and scope_id is not null)
    ),
    constraint budgets_period_valid    check (period in ('daily', 'weekly', 'monthly', 'total')),
    constraint budgets_limit_positive  check (limit_usd > 0),
    constraint budgets_spent_nonneg    check (spent_usd >= 0),
    constraint budgets_action_valid    check (action_on_exceed in ('block', 'warn')),
    constraint budgets_threshold_range check (alert_threshold_pct between 1 and 100)
);

create unique index if not exists budgets_scope_period_key on budgets (scope, scope_id, period)
    where scope_id is not null;
create unique index if not exists budgets_global_period_key on budgets (period)
    where scope = 'global';
create index if not exists budgets_enabled_idx on budgets (scope, scope_id) where enabled;

drop trigger if exists budgets_set_updated_at on budgets;
create trigger budgets_set_updated_at before update on budgets
    for each row execute function set_updated_at();

comment on table budgets is 'Batas biaya per periode. spent_usd adalah akumulasi periode berjalan, direset worker.';

-- ---------------------------------------------------------------------------
-- bans
-- ---------------------------------------------------------------------------
create table if not exists bans (
    id           uuid primary key default gen_random_uuid(),
    subject_kind text not null,
    -- IP tunggal, rentang CIDR, atau UUID entitas, tergantung subject_kind.
    subject      text not null,

    reason     text        not null,
    -- NULL berarti permanen sampai dicabut manual.
    expires_at timestamptz,
    lifted_at  timestamptz,
    lifted_by  uuid references users (id) on delete set null,

    created_at timestamptz not null default now(),
    created_by uuid references users (id) on delete set null,

    constraint bans_subject_kind_valid check (subject_kind in ('ip', 'ip_range', 'api_key', 'user')),
    constraint bans_subject_not_empty  check (length(btrim(subject)) > 0),
    constraint bans_reason_not_empty   check (length(btrim(reason)) > 0)
);

-- Hanya satu blokir aktif per subjek. Blokir yang sudah dicabut tetap disimpan
-- sebagai riwayat, jadi indeks unique-nya partial.
create unique index if not exists bans_active_subject_key on bans (subject_kind, subject)
    where lifted_at is null;
create index if not exists bans_expires_idx on bans (expires_at)
    where lifted_at is null and expires_at is not null;

comment on table bans is 'Blokir IP, rentang IP, API key, atau pengguna. Baris yang dicabut disimpan sebagai riwayat.';
