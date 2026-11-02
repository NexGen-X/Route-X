-- ==============================================================================
-- Route-X Migration 0019: Indeks Ganda, Constraint Pipeline, Partisi DEFAULT
-- (audit3 grup 11 — B-049 & B-048: V-DT-14, V-DT-17, V-DT-06)
-- ==============================================================================
-- Tiga utang skema dilunasi di sini:
--
-- 1. INDEKS GANDA DIHAPUS (V-DT-14). requests_errors_idx dari 0006
--    (created_at desc WHERE status_code >= 400) tersubsumsi penuh oleh
--    requests_errors_all_idx dari 0016 (kolom & arah sama, predikatnya
--    superset: status_code >= 400 OR error_type IS NOT NULL). Jalur tulis
--    terpanas di sistem memelihara dua indeks untuk pertanyaan yang sama.
--
-- 2. CONSTRAINT PIPELINE 0014 DITULIS ULANG (V-DT-17). Versi lama memakai
--    (pipeline->>'attempts')::int di dalam CHECK: attempts non-numerik
--    melempar 22P02 (invalid_text_representation) — error yang lolos dari
--    pemetaan repo sebagai 500 buram — alih-alih check_violation yang
--    semestinya. Elemen array models juga tidak diperiksa tipenya sama
--    sekali: [1,2] lolos skema lalu gagal jauh di hilir saat router membaca
--    pipeline menjadi []string.
--    Versi baru:
--      - attempts diperiksa dengan CASE: regex digit menjaga cast, sehingga
--        cast hanya dievaluasi atas teks yang pasti angka pendek — urutan
--        evaluasi CASE dijamin PostgreSQL, pola "regex AND cast" biasa tidak.
--      - setiap elemen models wajib bertipe string, diperiksa dengan
--        jsonb_path_exists (fungsi biasa — CHECK melarang subquery).
--      - seluruh predikat dibungkus coalesce(..., false) agar komponen yang
--        NULL (field hilang) berarti DITOLAK, bukan "lolos karena NULL".
--    Bila di database ini sudah ada baris yang melanggar aturan baru (hanya
--    mungkin lahir dari SQL manual — jalur API mem-parse pipeline menjadi
--    []string), constraint dipasang NOT VALID plus peringatan, supaya
--    upgrade tidak pernah macet oleh data warisan; tulis baru tetap dijaga.
--
-- 3. create_monthly_partition MENYELAMATKAN BARIS DEFAULT (V-DT-06). Versi
--    0006 gagal total bila partisi DEFAULT memuat baris dalam rentang bulan
--    yang diminta ("updated partition constraint for default partition
--    would be violated") — dan tidak ada jalan lain memindahkan baris itu,
--    sehingga satu baris nyasar (downtime >2 bulan, restore cadangan lama)
--    meracuni pembuatan partisi bulan itu SELAMANYA. Versi baru memindahkan
--    baris DEFAULT dalam rentang ke tabel penampung sementara, membuat
--    partisinya, lalu memasukkan baris kembali lewat parent (insert ke
--    parent mengarahkan baris ke partisi barunya). Nama kolom kunci partisi
--    dibaca dari katalog, bukan ditulis mati, agar fungsi tetap generik
--    untuk keempat tabel terpartisi (created_at / bucket).
--    Sesuai aturan migrasi 0018, search_path fungsi ini DIPAKU ke skema
--    tempat ia dibuat: badan fungsi merujuk tabel secara dinamis dan tidak
--    boleh bergantung pada search_path sesi pemanggil.
-- ==============================================================================

-- 1. Indeks ganda (V-DT-14).
drop index if exists requests_errors_idx;

-- 2. Constraint pipeline (V-DT-17).
do $$
declare
    pelanggar bigint;
    predikat constant text := $pred$
        jsonb_typeof(pipeline) = 'object'
        and jsonb_typeof(pipeline->'strategy') = 'string'
        and (pipeline->>'strategy') in ('priority', 'round_robin', 'lowest_latency', 'lowest_cost')
        and jsonb_typeof(pipeline->'models') = 'array'
        and jsonb_array_length(pipeline->'models') between 1 and 8
        and not jsonb_path_exists(pipeline, '$.models[*] ? (@.type() != "string")')
        and (case when (pipeline->>'attempts') ~ '^[0-9]{1,9}$'
                  then (pipeline->>'attempts')::int between 1 and 20
                  else false end)
    $pred$;
begin
    execute format(
        'select count(*) from routing_rules where pipeline is not null and not coalesce((%s), false)',
        predikat)
        into pelanggar;

    alter table routing_rules drop constraint if exists routing_rules_pipeline_strategy_valid;

    if pelanggar > 0 then
        raise warning 'migrasi 0019: % baris routing_rules melanggar constraint pipeline baru; constraint dipasang NOT VALID. Perbaiki baris tersebut, lalu jalankan: alter table routing_rules validate constraint routing_rules_pipeline_strategy_valid;', pelanggar;
        execute format(
            'alter table routing_rules add constraint routing_rules_pipeline_strategy_valid check (pipeline is null or coalesce((%s), false)) not valid',
            predikat);
    else
        execute format(
            'alter table routing_rules add constraint routing_rules_pipeline_strategy_valid check (pipeline is null or coalesce((%s), false))',
            predikat);
    end if;
end $$;

-- 3. Fungsi partisi yang menyelamatkan baris DEFAULT (V-DT-06).
create or replace function create_monthly_partition(parent text, month date)
returns text as $$
declare
    start_at  date := date_trunc('month', month)::date;
    end_at    date := (date_trunc('month', month) + interval '1 month')::date;
    part      text := format('%s_%s', parent, to_char(start_at, 'YYYYMM'));
    defpart   text := format('%s_default', parent);
    keycol    name;
    stash     text;
    ada_baris boolean := false;
begin
    if to_regclass(part) is not null then
        return part;
    end if;

    -- Pindahkan baris DEFAULT yang masuk rentang bulan ini keluar dulu.
    -- Tabel penampung bersifat sementara per-sesi dan dibuang eksplisit di
    -- akhir (bukan hanya ON COMMIT DROP) karena fungsi ini dipanggil
    -- berulang dalam satu transaksi oleh loop migrasi & maintainer.
    if to_regclass(defpart) is not null then
        select a.attname into keycol
        from pg_partitioned_table pt
        join pg_class c on c.oid = pt.partrelid
        join pg_attribute a on a.attrelid = c.oid and a.attnum = pt.partattrs[0]
        where c.relname = parent;
        if keycol is not null then
            stash := 'routex_pindah_' || part;
            execute format('drop table if exists %I', stash);
            execute format(
                'create temp table %I on commit drop as select * from %I where %I >= $1 and %I < $2',
                stash, defpart, keycol, keycol)
                using start_at, end_at;
            execute format('select exists (select 1 from %I)', stash) into ada_baris;
            if ada_baris then
                execute format('delete from %I where %I >= $1 and %I < $2', defpart, keycol, keycol)
                    using start_at, end_at;
            end if;
        end if;
    end if;

    execute format(
        'create table if not exists %I partition of %I for values from (%L) to (%L)',
        part, parent, start_at, end_at
    );

    if ada_baris then
        execute format('insert into %I select * from %I', parent, stash);
        execute format('drop table %I', stash);
    end if;
    return part;
end;
$$ language plpgsql;

comment on function create_monthly_partition(text, date) is
    'Membuat partisi bulanan bernama <parent>_YYYYMM bila belum ada; baris partisi DEFAULT dalam rentang dipindahkan ke partisi baru, bukan menggagalkan pembuatan. Dipakai migrasi dan worker pemeliharaan.';

-- Pakukan search_path fungsi ke skema tempat ia dibuat (pola migrasi 0018:
-- skema dibaca dinamis dari current_schema(), bukan literal).
do $$
begin
    execute format('alter function %I.create_monthly_partition(text, date) set search_path = %I',
        current_schema(), current_schema());
end $$;
