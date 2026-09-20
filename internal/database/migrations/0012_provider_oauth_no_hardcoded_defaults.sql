-- ==============================================================================
-- Route-X Migration 0012: Hapus Default Hardcoded OAuth Client ID Vendor
-- ==============================================================================
-- Kerentanan H3: migrasi 0011 menanam OAuth Client ID publik Google Antigravity
-- ('1071006060591-...apps.googleusercontent.com') beserta redirect URI localhost
-- sebagai DEFAULT kolom di skema database. Akibatnya setiap insert yang lupa
-- menyatakan client_id diam-diam memakai identitas vendor, dan skema sendiri
-- yang menyembunyikan rahasia publik tersebut — tanpa jejak di kode aplikasi.
--
-- Perbaikan: DEFAULT kolom client_id dan redirect_uri dihapus. Kedua kolom tetap
-- NOT NULL, sehingga insert yang tidak menyatakan nilai sekarang DITOLAK database
-- alih-alih diam-diam mengambil nilai vendor. Fallback nilai pindah ke layer
-- aplikasi (internal/database/repo/upstream/oauth.go) yang merujuk konstanta
-- eksplisit di internal/oauth/google.go: mudah diaudit, jelas terbaca sebagai
-- konfigurasi kode, dan tidak lagi disembunyikan di balik DEFAULT skema.
--
-- token_uri sengaja DIPERTAHANKAN default-nya: endpoint token Google adalah
-- konstanta vendor yang stabil dan netral (bukan identitas maupun rahasia), tidak
-- seperti redirect_uri yang merupakan konfigurasi deployment per-instalasi.
--
-- Migrasi murni DDL: tidak mengubah, memindahkan, atau menghapus baris yang ada.
-- Sesi operator yang sudah tersimpan tetap utuh dengan nilainya masing-masing.
-- ==============================================================================

-- Penghapusan DEFAULT dibungkus pengecekan pg_attrdef supaya idempoten: 'alter
-- column ... drop default' hanya dijalankan ketika default-nya memang ada, sehingga
-- migrasi ini aman dijalankan ulang pada database yang sudah pernah menerimanya.
-- to_regclass() mengikuti search_path, jadi blok ini bekerja di schema apa pun.
do $$
declare
    rel oid := to_regclass('provider_oauth_sessions');
begin
    if rel is null then
        return;
    end if;

    if exists (
        select 1
        from pg_attrdef a
        join pg_attribute b on b.attrelid = a.adrelid and b.attnum = a.adnum
        where a.adrelid = rel and b.attname = 'client_id'
    ) then
        alter table provider_oauth_sessions alter column client_id drop default;
    end if;

    if exists (
        select 1
        from pg_attrdef a
        join pg_attribute b on b.attrelid = a.adrelid and b.attnum = a.adnum
        where a.adrelid = rel and b.attname = 'redirect_uri'
    ) then
        alter table provider_oauth_sessions alter column redirect_uri drop default;
    end if;
end
$$;

comment on column provider_oauth_sessions.client_id  is 'OAuth Client ID provider. WAJIB dinyatakan saat insert: skema tidak lagi menyimpan default vendor (migrasi 0012); fallback ada di layer aplikasi.';
comment on column provider_oauth_sessions.redirect_uri is 'URI redirect OAuth yang dipakai saat otorisasi. WAJIB dinyatakan saat insert: konfigurasi deployment per-instalasi, bukan konstanta skema.';
