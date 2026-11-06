-- Migrasi 0026 — fungsi cast inet toleran untuk ban warisan (B-305).
--
-- Lapis kedua B-269 menutup baris ban ip_range ber-bit-host, tetapi
-- baris ban warisan yang subjeknya BUKAN alamat sama sekali (teks
-- sampah dari impor/penanaman lama, mis. 'bukan-alamat-ip' atau
-- '999.1.1.1/24') tetap melempar 22P02 dari cast b.subject::inet di
-- Authenticate — dan karena cast itu dievaluasi untuk baris berjenis
-- cocok pada SETIAP autentikasi, satu baris racun menjatuhkan
-- autentikasi SEMUA API key (500), bukan hanya key yang diban.
--
-- routex_inet_aman membungkus cast itu dalam blok exception plpgsql:
-- nilai yang tidak terurai menjadi inet menghasilkan NULL, dan
-- perbandingan terhadap NULL tidak pernah benar — baris racun tidak
-- bisa cocok dengan IP mana pun, setara tidak menegakkan apa pun,
-- tanpa menghapus datanya (riwayat ban tetap utuh untuk audit).
-- Nilai yang sah — termasuk yang ber-bit-host — terurai persis
-- seperti cast langsung, jadi semantik B-269 (<<= membandingkan
-- jaringan termasked) tidak berubah.

create or replace function routex_inet_aman(t text) returns inet
language plpgsql
stable
as $$
begin
	return t::inet;
exception when others then
	return null;
end
$$;
