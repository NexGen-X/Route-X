-- Skrip cek pra-retensi Route-X: jalankan SEBELUM mengubah nilai retensi.
-- Tujuan: memastikan sisa disk cukup dan melihat kandidat partisi yang akan dibuang.
-- Cara pakai: psql "$DATABASE_URL" -f scripts/ops/retention-check.sql
-- Catatan: hanya SELECT, aman dijalankan kapan saja.

-- 1. Ukuran total database dan tiap tabel induk besar.
select
  pg_size_pretty(pg_database_size(current_database())) as ukuran_database,
  pg_size_pretty(pg_total_relation_size('requests')) as requests_total,
  pg_size_pretty(pg_total_relation_size('request_events')) as request_events_total,
  pg_size_pretty(pg_total_relation_size('request_payloads')) as request_payloads_total,
  pg_size_pretty(pg_total_relation_size('usage_hourly')) as usage_hourly_total,
  pg_size_pretty(pg_total_relation_size('webhook_deliveries')) as webhook_deliveries_total;

-- 2. Daftar partisi bulanan beserta ukurannya, tertua dulu.
-- Format nama partisi: <induk>_YYYYMM (lihat internal/worker/retention.go dropOldPartitions).
select
  p.relname as partisi,
  pg_size_pretty(pg_total_relation_size(c.oid)) as ukuran
from pg_inherits i
join pg_class c on c.oid = i.inhrelid
join pg_class p on p.oid = i.inhparent
where p.relname in ('requests', 'request_events', 'usage_hourly')
  and c.relname not like '%_default'
order by c.relname;

-- 3. Jumlah baris request_payloads yang LEBIH TUA dari retensi body default (7 hari).
-- Ganti interval bila REQUEST_BODY_RETENTION_DAYS diubah.
select count(*) as payload_lebih_tua_dari_7_hari
from request_payloads
where created_at < now() - interval '7 days';

-- 4. Jumlah baris requests yang LEBIH TUA dari retensi log default (30 hari).
select count(*) as requests_lebih_tua_dari_30_hari
from requests
where created_at < now() - interval '30 days';

-- 5. Partisi requests yang BELUM ter-rollup ke usage_hourly (tidak akan dibuang worker).
-- Bila baris muncul di sini, perbaiki rollup dulu sebelum berharap disk menyusut.
select r.created_at::date as tanggal, count(*) as baris_belum_rollup
from requests r
where not exists (
  select 1 from usage_hourly u
  where u.bucket = date_trunc('hour', r.created_at, 'UTC')
)
group by 1 order by 1 limit 20;
