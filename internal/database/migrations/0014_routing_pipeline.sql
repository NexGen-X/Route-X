-- ==============================================================================
-- Route-X Migration 0014: Routing Pipeline & Virtual Alias
-- ==============================================================================
-- Aturan routing sekarang punya dua "mode" konseptual:
--
--   1. Model Only (jalur lama, tetap didukung apa adanya): kolom pipeline NULL.
--      Aturan terdiri dari match_model_id + strategy + daftar kandidat provider,
--      persis seperti sebelum migrasi ini. Tidak ada satu pun baris lama yang diubah
--      oleh migrasi ini — data lama tetap dibaca lewat tag [combo:...] di description.
--
--   2. Combo Model (jalur baru): kolom pipeline berisi resep failover multi-model
--      berbentuk jsonb:
--
--        {"strategy":"round_robin","attempts":3,"models":["a","b","c"]}
--
--      attempts adalah ANGGARAN TOTAL percobaan lintas seluruh model, bukan per-kandidat
--      seperti max_attempts. Eksekusinya (urutan, pemotongan anggaran) ada di gateway;
--      migrasi ini hanya menyiapkan tempat penyimpanannya.
--
-- Constraint pipeline sengaja hanya menerima 4 strategi yang ditampilkan UI
-- (priority, round_robin, lowest_latency, lowest_cost). Enum strategi di level Go
-- mengenal 6 nilai (bertambah weighted dan capability), dan kedua nilai itu sah di
-- kolom strategy lama — tetapi pipeline adalah fitur baru yang hanya bisa diisi dari
-- UI, dan UI hanya menyuguhkan 4 tombol. Menerima 6 di sini akan mengizinkan keadaan
-- yang tidak bisa dibuat operator mana pun, jadi constraint tetap 4. Ini sengaja,
-- bukan kelalaian yang perlu dirapikan.
--
-- virtual_alias menggantikan tag [combo:alias=...] di description: bila terisi, klien
-- bisa memanggil aturan ini seolah ia model tersendiri. Unik karena alias adalah
-- pengenal publik — dua aturan memakai alias yang sama berarti permintaan tidak bisa
-- dipastikan melayani aturan mana.
-- ==============================================================================

-- 1. Dua kolom baru, kedu nullable: pipeline NULL = Model Only (jalur lama).
alter table routing_rules
    add column if not exists pipeline jsonb,
    add column if not exists virtual_alias text;

-- 2. Validasi bentuk pipeline. Hanya diperiksa bila pipeline tidak NULL, sehingga
--    seluruh aturan lama lolos tanpa diubah. Pemeriksaan tipe jsonb_typeof sebelum
--    membaca fieldnya karena pipeline yang bukan objek (mis. array atau string) akan
--    membuat operator ->> mengembalikan NULL dan menyembunyikan pelanggaran sebenarnya
--    di balik NULL BETWEEN.
alter table routing_rules
    drop constraint if exists routing_rules_pipeline_strategy_valid;
alter table routing_rules
    add constraint routing_rules_pipeline_strategy_valid check (pipeline is null or (
        jsonb_typeof(pipeline) = 'object'
        and jsonb_typeof(pipeline->'strategy') = 'string'
        and (pipeline->>'strategy') in ('priority', 'round_robin', 'lowest_latency', 'lowest_cost')
        and jsonb_typeof(pipeline->'models') = 'array'
        and jsonb_array_length(pipeline->'models') between 1 and 8
        and (pipeline->>'attempts')::int between 1 and 20
    ));

-- 3. Alias virtual unik. Partial unique (hanya yang tidak NULL) karena NULL bukan
--    kependekan dari "alias kosong" — itu artinya "tidak punya alias", dan beberapa
--    aturan boleh tidak punya alias sekaligus.
--
--    Bentuknya INDEX, bukan CONSTRAINT: PostgreSQL tidak mengizinkan klausa WHERE pada
--    ADD CONSTRAINT UNIQUE, sementara index unik parsial adalah satu-satunya cara
--    menegakkan keunikan hanya pada baris yang aliasnya terisi. Namanya mengikuti
--    konvensi index tabel ini (lihat routing_rules_eval_idx di 0008), dan fungsinya
--    persis seperti constraint unik parsial: pelanggaran tetap dilaporkan sebagai
--    duplikat kunci.
create unique index if not exists routing_rules_virtual_alias_idx
    on routing_rules (virtual_alias)
    where virtual_alias is not null;

comment on column routing_rules.pipeline is
    'Resep failover multi-model (combo). NULL berarti Model Only: satu model, satu daftar kandidat provider. attempts adalah anggaran TOTAL percobaan lintas seluruh model.';
comment on column routing_rules.virtual_alias is
    'Nama model virtual yang memanggil aturan ini. NULL berarti aturan hanya bisa cocok lewat kondisi pencocokannya.';
