-- Migrasi 0001 — bootstrap.
--
-- Tabel settings menyimpan konfigurasi runtime yang boleh diubah operator dari
-- dashboard, tanpa restart dan tanpa mengedit environment. Isinya hanya kebijakan
-- yang aman diganti saat aplikasi hidup: batas laju bawaan, daftar model yang
-- ditampilkan, sakelar fitur, dan sejenisnya.
--
-- Rahasia TIDAK boleh masuk sini. Kredensial provider dan kunci enkripsi tetap
-- datang dari environment atau tabel terenkripsi tersendiri, karena nilai di tabel
-- ini terbaca siapa pun yang punya akses baca database maupun panel admin.
--
-- Nilainya jsonb, bukan text, supaya satu kunci bisa memuat struktur (mis. objek
-- kebijakan atau daftar) dan tetap bisa di-query per bagian oleh PostgreSQL tanpa
-- mengurai apa pun di sisi aplikasi.
--
-- Memakai "if not exists" supaya migrasi ini tetap aman dijalankan pada database
-- yang sudah dibuat manual sebelum runner migrasi ada.

create table if not exists settings (
    -- Kunci hierarkis dipisah titik, mis. "ratelimit.default_rpm".
    key         text        primary key,
    -- Nilai apa pun yang sah sebagai JSON; skalar pun disimpan sebagai jsonb.
    value       jsonb       not null,
    -- Keterangan singkat yang ditampilkan di sebelah kolom nilai di dashboard.
    description text,
    -- Diperbarui aplikasi setiap kali nilai berubah; dipakai untuk membatalkan cache.
    updated_at  timestamptz not null default now(),
    -- Pengguna yang terakhir mengubah. Null untuk nilai yang ditanam sistem, dan
    -- sengaja belum berforeign key karena tabel users baru dibuat di fase berikutnya.
    updated_by  uuid,

    -- Kunci kosong hampir pasti bug pemanggil, bukan konfigurasi yang disengaja.
    constraint settings_key_not_empty check (length(btrim(key)) > 0)
);

comment on table settings is
    'Konfigurasi runtime yang bisa diubah dari dashboard tanpa restart. Bukan tempat menyimpan rahasia.';
comment on column settings.key is
    'Kunci hierarkis dipisah titik, mis. ratelimit.default_rpm.';
comment on column settings.value is
    'Nilai dalam bentuk JSON; skalar pun disimpan sebagai jsonb.';
comment on column settings.description is
    'Keterangan singkat untuk ditampilkan di dashboard.';
comment on column settings.updated_at is
    'Waktu perubahan terakhir, dipakai untuk membatalkan cache di sisi aplikasi.';
comment on column settings.updated_by is
    'Pengguna terakhir yang mengubah nilai; null untuk nilai bawaan sistem.';
