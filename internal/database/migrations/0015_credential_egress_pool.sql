-- ==============================================================================
-- Route-X Migration 0015: Per-Account Egress Pool Isolation
-- ==============================================================================
-- Menambahkan relasi opsional egress_pool_id pada tabel provider_credentials.
-- Bila terisi, akun/kredensial ini akan menggunakan jalur keluar (proxy) khusus
-- yang meng-override konfigurasi egress_pool_id milik provider induk.
-- ==============================================================================

-- 1. Tambah kolom egress_pool_id ke provider_credentials
alter table provider_credentials
    add column if not exists egress_pool_id uuid references egress_pool (id) on delete set null;

-- 2. Buat index parsial untuk performa query relasi egress pool
create index if not exists provider_credentials_egress_pool_idx
    on provider_credentials (egress_pool_id)
    where egress_pool_id is not null;

comment on column provider_credentials.egress_pool_id is
    'Proxy keluar opsional khusus kredensial/akun ini. Meng-override egress_pool_id milik provider upstream. NULL berarti mewarisi provider default atau langsung.';
