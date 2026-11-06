-- ==============================================================================
-- Route-X Migration 0024: Penanda Request Belum Berharga (UNPRICED)
-- ==============================================================================
-- Menambahkan kolom unpriced ke requests serta penghitung unpriced_count ke
-- usage_hourly dan usage_daily (B-190).
--
-- Latar: lapisan akuntansi tahu persis kapan sebuah request tidak bisa
-- dihargai — pemetaan modelnya tidak punya baris harga yang berlaku — dan
-- sudah melaporkannya sebagai metrik, tetapi baris requests-nya sendiri
-- ditulis dengan cost_usd = 0 tanpa penanda apa pun. Akibatnya dasbor
-- menampilkan "$0" untuk pemakaian yang sebenarnya tidak ternilai, dan
-- total biaya tampak lengkap padahal sebagian request tidak terhitung.
-- Penanda ini membuat keadaan itu jujur di sumbernya: barisnya tahu ia
-- belum berharga, agregat menghitung berapa banyak, dan UI menampilkannya
-- sebagai UNPRICED alih-alih $0.
--
-- Sifat kolom: unpriced HANYA benar bila token terpakai > 0 dan harga
-- tidak ditemukan. Request tanpa token, cache HIT (keputusan B-188: biaya
-- nol secara sah), dan penolakan sebelum upstream tetap false — ketiganya
-- memang tidak mempunyai biaya, bukan belum berharga.
-- ==============================================================================

alter table requests
    add column if not exists unpriced boolean not null default false;

comment on column requests.unpriced is 'TRUE bila request memakai token tetapi tidak ada harga yang berlaku untuk pemetaan modelnya, sehingga cost_usd = 0 berarti "belum berharga", bukan gratis (B-190).';

alter table usage_hourly
    add column if not exists unpriced_count bigint not null default 0;

comment on column usage_hourly.unpriced_count is 'Jumlah request belum berharga (requests.unpriced) dalam bucket ini (B-190).';

alter table usage_daily
    add column if not exists unpriced_count bigint not null default 0;

comment on column usage_daily.unpriced_count is 'Jumlah request belum berharga (requests.unpriced) dalam bucket ini (B-190).';
