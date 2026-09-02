-- Migrasi 0002 — identitas, RBAC, sesi, dan audit.
--
-- Fondasi autentikasi admin. Empat peran bawaan (Super Admin, Admin, Operator,
-- Viewer) dimodelkan sebagai data, bukan konstanta di kode, supaya dashboard bisa
-- menampilkan kewenangan tiap peran dan supaya penambahan izin baru tidak menuntut
-- perubahan skema.

-- Fungsi bersama untuk memelihara updated_at. Diletakkan di migrasi ini karena
-- 0001 sudah diterapkan dan checksum-nya diverifikasi — file migrasi yang sudah
-- pernah jalan tidak boleh diubah lagi.
create or replace function set_updated_at() returns trigger as $$
begin
    new.updated_at = now();
    return new;
end;
$$ language plpgsql;

-- ---------------------------------------------------------------------------
-- users
-- ---------------------------------------------------------------------------
create table if not exists users (
    id            uuid        primary key default gen_random_uuid(),
    email         text        not null,
    -- Hash argon2id berformat PHC. Parameter biayanya tersimpan di dalam string,
    -- sehingga biaya bisa dinaikkan tanpa membuat hash lama gagal diverifikasi.
    password_hash text        not null,
    display_name  text,
    status        text        not null default 'active',

    -- Perlindungan brute force. Penguncian disimpan di database, bukan hanya di
    -- Redis, supaya kunci tidak hilang saat cache dikosongkan atau restart.
    failed_login_attempts int  not null default 0,
    locked_until          timestamptz,

    last_login_at timestamptz,
    last_login_ip inet,
    -- Dipaksa mengganti password saat login berikutnya; dipakai untuk admin pertama
    -- yang password awalnya berasal dari environment.
    must_change_password boolean not null default false,

    created_at    timestamptz not null default now(),
    updated_at    timestamptz not null default now(),

    constraint users_email_not_empty check (length(btrim(email)) > 0),
    constraint users_status_valid    check (status in ('active', 'disabled', 'locked'))
);

-- Email dibandingkan tanpa memperhatikan huruf besar-kecil: "Admin@x.com" dan
-- "admin@x.com" harus menjadi akun yang sama, bukan dua akun berbeda. Indeks
-- fungsional dipakai daripada extension citext agar tidak menambah dependensi.
create unique index if not exists users_email_lower_key on users (lower(email));
create index if not exists users_status_idx on users (status) where status <> 'active';

drop trigger if exists users_set_updated_at on users;
create trigger users_set_updated_at before update on users
    for each row execute function set_updated_at();

comment on table users is 'Akun admin yang bisa masuk ke dashboard. Bukan pengguna API — klien API memakai api_keys.';
comment on column users.must_change_password is 'Memaksa penggantian password saat login berikutnya, dipakai untuk admin pertama dari environment.';

-- ---------------------------------------------------------------------------
-- roles & permissions
-- ---------------------------------------------------------------------------
create table if not exists roles (
    id          uuid        primary key default gen_random_uuid(),
    name        text        not null unique,
    description text,
    -- Peran sistem tidak boleh dihapus atau diubah namanya dari dashboard, karena
    -- kode dan seed bergantung padanya.
    is_system   boolean     not null default false,
    -- Urutan tampilan dan pembanding kewenangan; makin kecil makin berkuasa.
    rank        int         not null,
    created_at  timestamptz not null default now(),

    constraint roles_name_not_empty check (length(btrim(name)) > 0)
);

create table if not exists permissions (
    id          uuid        primary key default gen_random_uuid(),
    -- Kunci berbentuk "sumber_daya:aksi", mis. "providers:write".
    key         text        not null unique,
    description text,
    created_at  timestamptz not null default now(),

    constraint permissions_key_format check (key ~ '^[a-z_]+:[a-z_]+$')
);

create table if not exists role_permissions (
    role_id       uuid not null references roles (id)       on delete cascade,
    permission_id uuid not null references permissions (id) on delete cascade,
    primary key (role_id, permission_id)
);

create table if not exists user_roles (
    user_id    uuid        not null references users (id) on delete cascade,
    role_id    uuid        not null references roles (id) on delete restrict,
    granted_at timestamptz not null default now(),
    -- Pemberi peran tidak di-cascade: menghapus admin tidak boleh menghapus catatan
    -- siapa yang pernah memberi kewenangan.
    granted_by uuid references users (id) on delete set null,
    primary key (user_id, role_id)
);

create index if not exists user_roles_role_idx on user_roles (role_id);

comment on table roles is 'Peran RBAC. Peran sistem (is_system) tidak bisa dihapus dari dashboard.';
comment on column roles.rank is 'Tingkat kewenangan; makin kecil makin berkuasa. Dipakai agar admin tidak bisa menaikkan peran di atas dirinya.';
comment on table permissions is 'Izin bergranularitas halus, dirujuk peran lewat role_permissions.';

-- ---------------------------------------------------------------------------
-- sessions
-- ---------------------------------------------------------------------------
create table if not exists sessions (
    id         uuid        primary key default gen_random_uuid(),
    user_id    uuid        not null references users (id) on delete cascade,
    -- Hanya hash SHA-256 dari token yang disimpan. Dump database karena itu tidak
    -- bisa dipakai untuk membajak sesi yang sedang berjalan.
    token_hash text        not null unique,
    ip         inet,
    user_agent text,
    created_at   timestamptz not null default now(),
    last_seen_at timestamptz not null default now(),
    expires_at   timestamptz not null,
    revoked_at   timestamptz,

    constraint sessions_expires_after_creation check (expires_at > created_at)
);

-- Pencarian sesi terjadi pada setiap request dashboard, jadi jalur ini harus lewat
-- indeks: token_hash sudah unique, dan indeks berikut melayani daftar sesi aktif
-- per pengguna serta pembersihan sesi kedaluwarsa oleh worker.
create index if not exists sessions_user_active_idx on sessions (user_id, expires_at desc)
    where revoked_at is null;
create index if not exists sessions_expires_idx on sessions (expires_at)
    where revoked_at is null;

comment on table sessions is 'Sesi dashboard. Menyimpan hash token, bukan tokennya, sehingga dump database tidak bisa membajak sesi.';

-- ---------------------------------------------------------------------------
-- audit_logs
-- ---------------------------------------------------------------------------
create table if not exists audit_logs (
    id           bigserial   primary key,
    occurred_at  timestamptz not null default now(),

    -- Referensi ke pelaku diputus (set null) bila akunnya dihapus, tetapi email dan
    -- namanya disimpan sebagai cuplikan. Catatan audit harus tetap menjawab "siapa
    -- yang melakukan ini" walaupun akunnya sudah lama tidak ada.
    actor_user_id uuid references users (id) on delete set null,
    actor_email   text,
    actor_role    text,

    action        text not null,
    resource_type text not null,
    resource_id   text,

    ip         inet,
    user_agent text,
    -- ID request yang memicu aksi, untuk menautkan audit dengan log aplikasi.
    request_id text,
    -- Detail tambahan. Nilai sensitif WAJIB sudah tersamar sebelum masuk sini.
    metadata   jsonb not null default '{}'::jsonb,

    constraint audit_logs_action_not_empty check (length(btrim(action)) > 0)
);

-- Audit log hampir selalu dibaca terbaru-dulu, dan sering disaring per pelaku atau
-- per sumber daya.
create index if not exists audit_logs_occurred_idx on audit_logs (occurred_at desc);
create index if not exists audit_logs_actor_idx    on audit_logs (actor_user_id, occurred_at desc);
create index if not exists audit_logs_resource_idx on audit_logs (resource_type, resource_id, occurred_at desc);
create index if not exists audit_logs_action_idx   on audit_logs (action, occurred_at desc);

comment on table audit_logs is 'Catatan aksi administratif. Email pelaku disimpan sebagai cuplikan agar catatan tetap bermakna setelah akunnya dihapus.';
comment on column audit_logs.metadata is 'Detail tambahan dalam JSON. Nilai sensitif harus sudah tersamar sebelum ditulis.';
