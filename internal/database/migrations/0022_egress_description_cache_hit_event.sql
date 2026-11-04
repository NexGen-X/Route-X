-- Migrasi 0022 — deskripsi egress pool + jenis event cache_hit (B-116).
--
-- 1. egress_pool.description: handler admin MENERIMA field description saat
--    membuat/memperbarui pool tetapi tidak ada kolomnya, jadi nilai itu hilang
--    diam-diam dan DTO tidak pernah bisa mengembalikannya (B-116 butir 1).
--    Kolom teks biasa, bawaan '' — bukan rahasia, aman ditampilkan di DTO.
--
-- 2. request_events: jenis 'cache_hit' supaya request yang terjawab dari
--    response cache punya baris timeline sendiri (B-116 butir 7). Tanpa ini
--    tab Events inspektor kosong untuk HIT — bukan karena datanya hilang,
--    melainkan karena tidak ada percobaan upstream yang bisa dicatat dan
--    tidak ada jenis event untuk keadaan itu. Constraint diganti dengan
--    daftar lama + satu nilai baru; tidak ada baris lama yang ditulis ulang.

alter table egress_pool
    add column if not exists description text not null default '';

comment on column egress_pool.description is
    'Deskripsi bebas operator untuk pool ini. Diterima API admin dan dikembalikan apa adanya di DTO (B-116).';

alter table request_events
    drop constraint if exists request_events_kind_valid;

alter table request_events
    add constraint request_events_kind_valid check (kind in (
        'attempt_started', 'attempt_succeeded', 'attempt_failed',
        'retry_scheduled', 'failover', 'circuit_open', 'rate_limited',
        'budget_exceeded', 'content_blocked', 'stream_started', 'stream_completed',
        'cache_hit'
    ));
