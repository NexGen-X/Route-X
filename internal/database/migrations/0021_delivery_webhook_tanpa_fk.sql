-- =============================================================================
-- Route-X Migration 0021: Riwayat delivery webhook bertahan sesudah webhook
-- dihapus (B-115)
-- =============================================================================
-- Migrasi 0008 memasang webhook_deliveries.webhook_id sebagai foreign key
-- ON DELETE CASCADE: menghapus satu webhook memusnahkan SELURUH riwayat
-- pengirimannya — bukti pengiriman yang justru dibutuhkan saat menyelisik
-- sengketa/debugging integrasi hilang bersama objeknya, dan GET delivery
-- per id menjawab 404 untuk riwayat yang barusan ada.
--
-- Kebijakan yang disamakan di sini adalah kebijakan yang sudah berlaku di
-- tabel requests: requests.api_key_id SENGAJA tidak ber-foreign key supaya
-- riwayat tidak bergantung pada baris induk yang masih ada (lihat komentar
-- keys.Repo.Delete). webhook_id kini diperlakukan sama: kolomnya tetap
-- NOT NULL bertipe uuid dan tetap terindeks
-- (webhook_deliveries_webhook_idx), tetapi nilainya adalah PENGENAL
-- SNAPSHOT webhook asal — bukan referensi yang harus selalu resolvable.
--
-- Akibat yang sudah disiapkan kode dan kini benar-benar terjangkau:
--   - Baris delivery selesai (delivered/failed/abandoned) tetap terbaca
--     admin lewat GET /deliveries/{id} dan daftar per webhook_id.
--   - Baris yang masih antre (pending/failed) untuk webhook yang terhapus
--     diambil worker seperti biasa, lalu dispatcher menandainya abandoned
--     dengan alasan "webhook induk tidak ditemukan" (jalur V-WH-01 di
--     internal/webhooks/dispatcher.go) — tidak dikirim, tidak dijadwalkan
--     ulang, tidak hilang diam-diam.
--   - Retensi tetap berlaku: baris delivered/abandoned lama tetap
--     dibersihkan worker retensi menurut umurnya.
--
-- Data lama tidak berubah sama sekali: migrasi ini hanya melepas satu
-- constraint; tidak ada baris yang ditulis ulang.
-- =============================================================================

alter table webhook_deliveries
    drop constraint if exists webhook_deliveries_webhook_id_fkey;

comment on column webhook_deliveries.webhook_id is
    'Pengenal snapshot webhook asal pengiriman. SENGAJA tanpa foreign key sejak migrasi 0021 (B-115): riwayat delivery harus bertahan sesudah webhook dihapus, sama seperti requests.api_key_id. Baris antre milik webhook yang sudah tidak ada ditandai abandoned oleh dispatcher.';
