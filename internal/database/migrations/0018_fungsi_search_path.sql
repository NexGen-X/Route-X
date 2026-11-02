-- Migrasi 0018 — fungsi histogram memakai search_path tetap (perbaikan B-045).
--
-- Cadangan database Route-X tidak pernah bisa dipulihkan sampai migrasi ini
-- ada. Lima fungsi di migrasi 0007 memanggil fungsi saudaranya TANPA
-- kualifikasi skema (mis. usage_histogram_valid memanggil
-- usage_latency_bounds()). Nama tak berkualifikasi di badan fungsi SQL dan
-- plpgsql diselesaikan pada saat fungsi DIPAKAI, memakai search_path sesi
-- pemanggil. pg_dump memulihkan cadangan dengan search_path kosong, sehingga
-- validasi check constraint saat ATTACH PARTITION partisi usage_hourly gagal
-- ("function usage_latency_bounds() does not exist") dan restore berhenti di
-- tengah — pada jalur lama: sesudah --clean menjatuhkan objek, menyisakan
-- skema hibrida setengah pulih.
--
-- Memakukan search_path pada definisi fungsi membuat resolusi nama di badan
-- fungsi tidak lagi bergantung pada sesi pemanggil: cadangan bisa dipulihkan
-- oleh psql/pg_restore dalam kondisi baku, dan pemanggil dengan search_path
-- apa pun mendapat hasil yang sama.
--
-- Nilai yang dipaku adalah skema tempat fungsi dibuat (current_schema() saat
-- migrasi berjalan — "public" di produksi, skema sekali pakai di test),
-- BUKAN kata "public" yang ditulis mati: fungsi-fungsi ini hidup di skema
-- yang sama dengan tabelnya di setiap lingkungan.
--
-- Aturan ke depan: fungsi yang merujuk objek lain tanpa kualifikasi skema
-- WAJIB memakukan search_path seperti ini, atau merujuk dengan nama
-- berkualifikasi penuh.

do $$
begin
    execute format('alter function %I.usage_latency_slot(int) set search_path = %I',
                   current_schema(), current_schema());
    execute format('alter function %I.usage_histogram_valid(bigint[]) set search_path = %I',
                   current_schema(), current_schema());
    execute format('alter function %I.usage_histogram_empty(bigint[]) set search_path = %I',
                   current_schema(), current_schema());
    execute format('alter function %I.usage_histogram_accum(bigint[], int) set search_path = %I',
                   current_schema(), current_schema());
    execute format('alter function %I.usage_histogram_percentile(bigint[], numeric) set search_path = %I',
                   current_schema(), current_schema());
end $$;
