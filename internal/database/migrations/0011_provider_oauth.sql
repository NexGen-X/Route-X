-- ==============================================================================
-- Route-X Migration 0011: Provider OAuth Sessions & Multi-Account Rotation Strategy
-- ==============================================================================
-- Menambahkan dukungan sesi OAuth 2.0 multi-account untuk upstream provider
-- (seperti Google Antigravity) dan strategi rotasi kredensial universal
-- (round_robin vs priority) untuk seluruh provider.
-- ==============================================================================

-- 1. Tambah kolom credential_strategy ke tabel providers
alter table providers
    add column if not exists credential_strategy text not null default 'round_robin';

alter table providers
    drop constraint if exists providers_credential_strategy_valid;
alter table providers
    add constraint providers_credential_strategy_valid check (credential_strategy in ('round_robin', 'priority'));

-- 2. Tambah kolom priority ke tabel provider_credentials
alter table provider_credentials
    add column if not exists priority int not null default 100;

-- 3. Tabel provider_oauth_sessions
create table if not exists provider_oauth_sessions (
    id                      uuid        primary key default gen_random_uuid(),
    provider_id             uuid        not null references providers (id) on delete cascade,
    credential_id           uuid        not null references provider_credentials (id) on delete cascade,
    account_email           text        not null,
    account_name            text,
    client_id               text        not null default '1071006060591-tmhssin2h21lcre235vtolojh4g403ep.apps.googleusercontent.com',
    encryption_key_id       text        not null,
    refresh_token_encrypted text        not null,
    token_uri               text        not null default 'https://oauth2.googleapis.com/token',
    redirect_uri            text        not null default 'http://localhost:4567',
    scopes                  jsonb       not null default '[]'::jsonb,
    last_refreshed_at       timestamptz,
    last_refresh_error      text,
    enabled                 boolean     not null default true,
    created_at              timestamptz not null default now(),
    updated_at              timestamptz not null default now(),

    constraint provider_oauth_sessions_email_unique unique (provider_id, account_email)
);

create index if not exists provider_oauth_sessions_provider_idx on provider_oauth_sessions (provider_id);
create index if not exists provider_oauth_sessions_cred_idx     on provider_oauth_sessions (credential_id);
create index if not exists provider_oauth_sessions_enabled_idx  on provider_oauth_sessions (enabled) where enabled;

drop trigger if exists provider_oauth_sessions_set_updated_at on provider_oauth_sessions;
create trigger provider_oauth_sessions_set_updated_at before update on provider_oauth_sessions
    for each row execute function set_updated_at();

comment on table provider_oauth_sessions is 'Sesi otorisasi OAuth 2.0 multi-account untuk upstream provider dengan refresh token terenkripsi.';
comment on column providers.credential_strategy is 'Strategi pemilihan kredensial/akun di dalam satu provider: round_robin atau priority.';
comment on column provider_credentials.priority is 'Tingkat prioritas kredensial saat provider memakai strategi priority (makin kecil makin utama).';
