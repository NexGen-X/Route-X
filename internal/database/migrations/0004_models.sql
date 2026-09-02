-- Migrasi 0004 — registry model, alias, pemetaan model↔provider, dan riwayat harga.
--
-- Pemisahan tabelnya disengaja:
--   models          katalog kanonik: apa yang boleh diminta klien
--   model_aliases   nama lain yang menunjuk ke model kanonik
--   provider_models provider mana yang bisa melayani model itu, dan dengan nama apa
--                   di sisi upstream
--   model_pricing   riwayat harga per pasangan model-provider
--
-- Menggabungkan harga ke dalam pemetaan routing akan memaksa penggandaan baris setiap
-- kali harga berubah, dan membuat perhitungan biaya lampau tidak mungkin akurat.

-- ---------------------------------------------------------------------------
-- models
-- ---------------------------------------------------------------------------
create table if not exists models (
    id       uuid primary key default gen_random_uuid(),
    -- Pengenal yang diminta klien, mis. "gpt-5". Inilah nilai pada field "model"
    -- di body request OpenAI.
    model_id text not null unique,

    display_name text not null,
    family       text,

    context_window    int,
    max_output_tokens int,

    -- Kemampuan model, dipakai strategi routing capability-based.
    capabilities text[] not null default '{}',

    enabled          boolean not null default true,
    routing_priority int     not null default 100,
    -- Strategi bawaan untuk model ini. NULL berarti memakai setelan global; aturan
    -- di routing_rules bisa menimpanya lagi untuk kondisi yang lebih spesifik.
    routing_strategy text,

    deprecated_at timestamptz,
    metadata      jsonb       not null default '{}'::jsonb,
    created_at    timestamptz not null default now(),
    updated_at    timestamptz not null default now(),

    constraint models_model_id_not_empty check (length(btrim(model_id)) > 0),
    constraint models_context_positive   check (context_window is null or context_window > 0),
    constraint models_output_positive    check (max_output_tokens is null or max_output_tokens > 0),
    constraint models_strategy_valid     check (routing_strategy is null or routing_strategy in
        ('priority', 'round_robin', 'weighted', 'lowest_latency', 'lowest_cost', 'capability')),
    -- Kemampuan dibatasi daftar yang dikenali mesin routing. Tanpa ini, salah tulis
    -- satu huruf akan membuat model diam-diam tidak pernah terpilih.
    constraint models_capabilities_known check (
        capabilities <@ array['text', 'vision', 'reasoning', 'tools', 'embeddings']::text[]
    )
);

-- Routing capability-based menyaring per elemen array, jadi butuh indeks GIN.
create index if not exists models_capabilities_idx on models using gin (capabilities);
create index if not exists models_enabled_idx      on models (routing_priority) where enabled;
create index if not exists models_family_idx       on models (family);

drop trigger if exists models_set_updated_at on models;
create trigger models_set_updated_at before update on models
    for each row execute function set_updated_at();

comment on table models is 'Katalog model kanonik yang boleh diminta klien.';
comment on column models.model_id is 'Nilai yang dikirim klien pada field "model", mis. gpt-5.';
comment on column models.capabilities is 'Dibatasi: text, vision, reasoning, tools, embeddings. Dipakai routing capability-based.';

-- ---------------------------------------------------------------------------
-- model_aliases
-- ---------------------------------------------------------------------------
create table if not exists model_aliases (
    alias      text        primary key,
    model_id   uuid        not null references models (id) on delete cascade,
    created_at timestamptz not null default now(),
    created_by uuid references users (id) on delete set null,

    constraint model_aliases_alias_not_empty check (length(btrim(alias)) > 0)
);

create index if not exists model_aliases_model_idx on model_aliases (model_id);

-- Alias tidak boleh sama dengan model_id yang sudah ada, dan sebaliknya. Resolusi
-- selalu mencoba model kanonik lebih dulu, jadi alias yang menabrak nama kanonik
-- tidak akan pernah terpakai — kegagalan yang membingungkan karena tidak ada pesan
-- error, model hanya "tidak berperilaku seperti yang dikonfigurasi". Dua trigger
-- berikut menutup kedua arah tabrakan itu.
create or replace function check_alias_not_canonical() returns trigger as $$
begin
    if exists (select 1 from models where model_id = new.alias) then
        raise exception 'alias % bertabrakan dengan model_id kanonik yang sudah ada', new.alias
            using errcode = 'unique_violation';
    end if;
    return new;
end;
$$ language plpgsql;

drop trigger if exists model_aliases_not_canonical on model_aliases;
create trigger model_aliases_not_canonical before insert or update on model_aliases
    for each row execute function check_alias_not_canonical();

create or replace function check_model_id_not_alias() returns trigger as $$
begin
    if exists (select 1 from model_aliases where alias = new.model_id) then
        raise exception 'model_id % bertabrakan dengan alias yang sudah ada', new.model_id
            using errcode = 'unique_violation';
    end if;
    return new;
end;
$$ language plpgsql;

drop trigger if exists models_id_not_alias on models;
create trigger models_id_not_alias before insert or update of model_id on models
    for each row execute function check_model_id_not_alias();

comment on table model_aliases is 'Nama alternatif yang menunjuk model kanonik. Tidak boleh menabrak models.model_id.';

-- ---------------------------------------------------------------------------
-- provider_models — provider mana melayani model mana
-- ---------------------------------------------------------------------------
create table if not exists provider_models (
    id          uuid not null primary key default gen_random_uuid(),
    model_id    uuid not null references models (id)    on delete cascade,
    provider_id uuid not null references providers (id) on delete cascade,

    -- Nama model di sisi upstream, yang sering berbeda dari nama kanonik.
    -- Contoh: kanonik "gpt-4o" bisa menjadi "gpt-4o-2024-08-06" di upstream.
    upstream_model_name text not null,

    enabled  boolean not null default true,
    -- Menimpa prioritas dan bobot provider khusus untuk model ini. NULL berarti
    -- memakai nilai dari tabel providers.
    priority int,
    weight   int,

    -- Provider bisa membatasi jendela konteks lebih kecil dari kemampuan model.
    max_context_window int,
    supports_streaming boolean not null default true,
    supports_tools     boolean not null default true,

    created_at timestamptz not null default now(),
    updated_at timestamptz not null default now(),

    constraint provider_models_upstream_not_empty check (length(btrim(upstream_model_name)) > 0),
    constraint provider_models_priority_range     check (priority is null or priority between 0 and 100000),
    constraint provider_models_weight_positive    check (weight is null or weight > 0),
    unique (model_id, provider_id)
);

-- Inti pemilihan rute: "provider aktif mana yang bisa melayani model ini, berurut
-- prioritas". Indeks ini yang dipakai jalur tersebut.
create index if not exists provider_models_routing_idx on provider_models (model_id, priority nulls last, weight desc nulls last)
    where enabled;
create index if not exists provider_models_provider_idx on provider_models (provider_id);

drop trigger if exists provider_models_set_updated_at on provider_models;
create trigger provider_models_set_updated_at before update on provider_models
    for each row execute function set_updated_at();

comment on table provider_models is 'Pemetaan model kanonik ke provider yang bisa melayaninya, beserta nama upstream-nya.';

-- ---------------------------------------------------------------------------
-- model_pricing — riwayat harga
-- ---------------------------------------------------------------------------
create table if not exists model_pricing (
    id                uuid not null primary key default gen_random_uuid(),
    provider_model_id uuid not null references provider_models (id) on delete cascade,

    -- Harga per SATU JUTA token, mengikuti konvensi yang dipakai semua penyedia.
    -- Tipe numeric, bukan floating point: pembulatan biner pada nilai uang akan
    -- menghasilkan selisih yang menumpuk di seluruh tagihan.
    input_usd_per_mtok        numeric(14, 6) not null,
    output_usd_per_mtok       numeric(14, 6) not null,
    -- Harga token input yang terlayani prompt cache, biasanya jauh lebih murah.
    cached_input_usd_per_mtok numeric(14, 6),
    -- Beberapa model menagih token penalaran terpisah dari token output.
    reasoning_usd_per_mtok    numeric(14, 6),
    currency                  text not null default 'USD',

    -- Rentang berlaku. Biaya request lampau dihitung dengan harga yang berlaku saat
    -- request itu terjadi, sehingga laporan bulan lalu tidak berubah ketika harga naik.
    effective_from timestamptz not null default now(),
    effective_to   timestamptz,

    -- Asal angka ini, mis. "official" atau "manual", untuk jejak audit.
    source     text        not null default 'manual',
    created_at timestamptz not null default now(),
    created_by uuid references users (id) on delete set null,

    constraint model_pricing_input_nonneg    check (input_usd_per_mtok >= 0),
    constraint model_pricing_output_nonneg   check (output_usd_per_mtok >= 0),
    constraint model_pricing_cached_nonneg   check (cached_input_usd_per_mtok is null or cached_input_usd_per_mtok >= 0),
    constraint model_pricing_reason_nonneg   check (reasoning_usd_per_mtok is null or reasoning_usd_per_mtok >= 0),
    constraint model_pricing_range_ordered   check (effective_to is null or effective_to > effective_from),
    constraint model_pricing_currency_format check (currency ~ '^[A-Z]{3}$'),
    unique (provider_model_id, effective_from)
);

-- Hanya boleh ada SATU harga yang sedang berlaku per pasangan model-provider.
-- Tanpa penjagaan ini, dua baris terbuka akan membuat perhitungan biaya bergantung
-- pada urutan baris yang kebetulan terbaca.
create unique index if not exists model_pricing_one_current_idx
    on model_pricing (provider_model_id) where effective_to is null;

-- Pencarian harga yang berlaku pada satu titik waktu, dipakai saat menghitung biaya.
create index if not exists model_pricing_lookup_idx
    on model_pricing (provider_model_id, effective_from desc);

comment on table model_pricing is 'Riwayat harga per pasangan model-provider. Harga per satu juta token, tipe numeric agar akurat.';
comment on column model_pricing.effective_to is 'NULL berarti harga yang sedang berlaku. Hanya satu baris terbuka per pasangan.';
