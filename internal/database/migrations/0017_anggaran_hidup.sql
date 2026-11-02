-- ==============================================================================
-- Route-X Migration 0017: Anggaran Hidup (Audit3 B-009 & B-010)
-- ==============================================================================
-- Tiga perbaikan data untuk temuan audit3 klaster Anggaran & Billing:
-- 1. budgets.period_end NULL pada anggaran berkala diisi dari period_start, supaya
--    BudgetResetterJob bisa memprosesnya (V-AB-01: period_end tak pernah diisi saat
--    pembuatan membuat anggaran daily/weekly/monthly ditegakkan seumur hidup).
-- 2. budgets.scope_id & rate_limits.scope_id cakupan 'model' yang masih berisi NAMA
--    atau ALIAS model diubah menjadi UUID model kanonik (V-AB-02, V-AB-03: jalur
--    request selalu memakai UUID, jadi baris bernama tidak pernah bertemu target).
-- 3. budgets.limit_usd & spent_usd diperlebar numeric(14,6) -> numeric(20,10)
--    (V-AB-07: pada skala 6, penambahan biaya berskala 8 di bawah 5e-7 USD
--    dibulatkan menjadi nol dan hilang permanen dari akumulasi).
-- ==============================================================================

-- 1. Pulihkan batas periode anggaran berkala warisan. Nilai ini hanya jembatan:
--    begitu period_end terisi dan sudah lewat, worker memajukannya dengan batas
--    kalender baru (policy.AkhirPeriode) dan menghitung ulang pemakaian riil dari
--    tabel requests untuk jendela yang sedang berjalan.
update budgets
set period_end = period_start + case period
        when 'daily'   then interval '1 day'
        when 'weekly'  then interval '7 days'
        when 'monthly' then interval '1 month'
    end
where period_end is null and period in ('daily', 'weekly', 'monthly');

-- 2a. Sembuhkan scope_id model pada budgets: nama kanonik -> UUID.
--     Penjaga NOT EXISTS menghindari tabrakan indeks unik (scope, scope_id, period)
--     bila baris UUID-nya sudah ada; baris yang bertabrakan dibiarkan apa adanya.
update budgets b
set scope_id = m.id::text
from models m
where b.scope = 'model' and b.scope_id = m.model_id
  and not exists (
      select 1 from budgets lain
      where lain.scope = 'model' and lain.scope_id = m.id::text
        and lain.period = b.period and lain.id <> b.id
  );

-- 2b. Sembuhkan scope_id model pada budgets: alias -> UUID.
update budgets b
set scope_id = m.id::text
from model_aliases a
join models m on m.id = a.model_id
where b.scope = 'model' and b.scope_id = a.alias
  and not exists (
      select 1 from budgets lain
      where lain.scope = 'model' and lain.scope_id = m.id::text
        and lain.period = b.period and lain.id <> b.id
  );

-- 2c. Sembuhkan scope_id model pada rate_limits: nama kanonik -> UUID.
--     Indeks uniknya (scope, scope_id), tanpa periode.
update rate_limits r
set scope_id = m.id::text
from models m
where r.scope = 'model' and r.scope_id = m.model_id
  and not exists (
      select 1 from rate_limits lain
      where lain.scope = 'model' and lain.scope_id = m.id::text
        and lain.id <> r.id
  );

-- 2d. Sembuhkan scope_id model pada rate_limits: alias -> UUID.
update rate_limits r
set scope_id = m.id::text
from model_aliases a
join models m on m.id = a.model_id
where r.scope = 'model' and r.scope_id = a.alias
  and not exists (
      select 1 from rate_limits lain
      where lain.scope = 'model' and lain.scope_id = m.id::text
        and lain.id <> r.id
  );

-- 3. Perlebar presisi uang anggaran. Nilai berskala 8 (USD internal) kini tersimpan
--    eksak; konversi dari skala 6 tidak mengubah nilai yang sudah tersimpan.
alter table budgets
    alter column limit_usd type numeric(20, 10),
    alter column spent_usd type numeric(20, 10);
