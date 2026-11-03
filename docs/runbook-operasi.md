# Runbook operasi Route-X (produksi)

## 1. Backup + restore database

Dua jalur resmi — rincian lengkap di `docs/DISASTER_RECOVERY.md`:

  - Backup terjadwal di server: `scripts/ops/backup-db.sh`
    (dump SQL gzip ke /var/backups/routex, pola nama
    routex_prod_YYYYMMDD_HHMMSS.sql.gz, retensi 7 hari,
    log /var/log/routex/backup.log). Penjadwalan cron dipasang
    operator sendiri; repo tidak memasangnya otomatis.
  - Ekspor/restore dari aplikasi: Pengaturan → Cadangan & Pemulihan
    (POST /api/admin/system/backup/export dan .../restore).
    Restore produksi WAJIB lewat jalur ini — ia memakai skratch-swap:
    dump dipindai, di-restore ke database skratch, dicek versi
    skemanya, lalu nama ditukar dan database lama dipertahankan
    sebagai <db>_pre_restore. Jangan memulihkan dump langsung ke
    database produksi.

Verifikasi berkas cadangan di database buang:

  createdb routex_verifikasi
  gunzip -c <berkas>.sql.gz | psql -d routex_verifikasi -v ON_ERROR_STOP=1
  psql -d routex_verifikasi -tAc "SELECT count(*) FROM schema_migrations;"
  # harus: 20 (migrasi 0001 sampai 0020)
  dropdb routex_verifikasi

## 2. Alert yang disarankan

Ambang dari hasil beban 200 VU (p95 13 ms, error ~0):

  healthz           != 200 selama 2 menit    -> page
  readyz            != 200 selama 2 menit    -> page (dependensi kritis: PostgreSQL/skema;
                                                 Redis non-kritis — gagalnya membuat status
                                                 "degraded" tetapi tetap HTTP 200)
  p95 chat          > 2000 ms selama 5 menit -> warning
  5xx / menit       > 10 selama 5 menit      -> warning
  antrian worker    macet > 10 menit         -> warning
  disk partisi requests > 80 persen          -> warning

Ambang p95 2000 ms sama dengan thresholds k6-berat.js.

## 3. Retensi data

Tiga knob env di internal/config/config.go (bawaan dalam kurung):
REQUEST_LOG_RETENTION_DAYS (30) untuk baris requests,
REQUEST_BODY_RETENTION_DAYS (7) untuk payload request, dan
AUDIT_LOG_RETENTION_DAYS (365) untuk audit_logs. Worker retensi
menjalankan penghapusan berpartisi; kegagalannya tercatat jujur di
status worker, bukan ditelan.
Cek baris tua sebelum mengubah:

  psql -f scripts/ops/retention-check.sql

Menaikkan retensi hanya bila compliance meminta + cek disk dulu
(rujukan historis uji beban 2026-09: partisi requests tumbuh
~385 MB per 294 ribu baris).

## 4. Rotasi secret

Tidak semua secret setara — memperlakukan keempatnya dengan prosedur yang
sama pernah tertulis di sini dan itu berbahaya. Yang benar per variabel:

### API_KEY_PEPPER — JANGAN dirotasi

Pepper adalah bahan hash satu arah untuk API key. Menggantinya membuat
SEMUA API key yang sudah diterbitkan gagal autentikasi (401) secara
permanen dan serentak: tidak ada re-hash yang mungkin karena nilai mentah
key tidak disimpan. Pemulihan hanya dua: kembalikan nilai pepper lama
persis, atau buat ulang seluruh API key. Pepper hanya boleh berubah pada
instalasi baru yang belum menerbitkan key.

### SESSION_SECRET — hanya membatalkan token CSRF, BUKAN sesi

Secret ini adalah kunci HMAC token CSRF. Keabsahan sesi login ditentukan
baris tabel `sessions` di database, jadi merotasi SESSION_SECRET TIDAK
mengeluarkan siapa pun — sesi lama tetap sah sampai kedaluwarsa/dicabut.
Untuk memaksa semua perangkat masuk ulang (respons insiden), pakai saklar
khususnya:

  POST /api/admin/access/sessions/revoke-all   (sesi admin)

atau cabut sesi per pengguna lewat halaman Akses. Rotasi SESSION_SECRET
sendiri aman kapan pun (restart; token CSRF lama gugur, sesi tetap hidup).

### ENCRYPTION_KEY — lewat masa tenggang, bukan sekali tebas

Ciphertext kredensial provider, secret webhook, URL egress, dan refresh
token OAuth dienkripsi dengan kunci ini. Prosedur yang aman:

  1. Siapkan kunci baru (base64 32 byte) di secret manager.
  2. Setel ENCRYPTION_KEY=<kunci baru> DAN
     ENCRYPTION_KEY_PREVIOUS=<kunci lama>, restart, verifikasi
     healthz+readyz 200 dan kredensial provider terbaca. Pada keadaan ini
     gateway membaca ciphertext lama maupun baru, dan menulis dengan
     kunci baru.
  3. Jalankan routex-rotate untuk menulis ulang ciphertext lama:
     OLD_ENCRYPTION_KEY=<lama> NEW_ENCRYPTION_KEY=<baru> routex-rotate
     (kunci hanya dari env — argumen baris perintah terlihat di daftar
     proses). Rotasi yang menemukan baris berkunci asing (bukan lama/bukan
     baru) akan DIBATALKAN keras, bukan dilewati.
  4. Verifikasi lagi, lalu HAPUS ENCRYPTION_KEY_PREVIOUS dan restart.
     Selama env itu terpasang, kunci lama masih dipercaya untuk dekripsi.

Urutan lama "rotasi dulu dengan gateway mati, ganti env, nyalakan" tidak
lagi wajib — masa tenggang di langkah 2 menghapus jendela di mana gateway
tidak bisa membaca datanya sendiri.

### METRICS_TOKEN — aman dirotasi

Ganti nilainya (min 32 char), samakan konfigurasi scraper Prometheus,
restart. Tidak ada data persisten yang bergantung padanya.

## 5. Dependensi

  go mod verify            # harus: all modules verified
  npm audit --omit=dev     # harus: 0 vuln prod

Hasil terakhir terverifikasi 2026-09-09: go mod verify bersih, npm prod 0 vuln.
2 vuln dev-only (esbuild via vite, GHSA dev-server) DITERIMA:
hanya memengaruhi `vite dev`, tidak ikut build produksi.
Catatan itu historis untuk pohon Vite 5. Sejak upgrade terkoordinasi
Vite 8 + @vitejs/plugin-react 6 (Langkah 5a), vite tidak lagi membawa
esbuild (esbuild yang tersisa di pohon hanya salinan bersarang milik
vitest 2) dan larangan lama "jangan sampai naik ke vite 8" gugur —
vite 8 kini versi yang dipakai. Angka audit terkini wajib diambil dari
lari audit baru, bukan dari catatan ini.
Jalankan ulang kedua perintah di atas untuk angka terkini — hasil
bertanggal ini adalah catatan historis, bukan status kini.

## 6. Zona waktu periode anggaran

Batas kalender periode anggaran weekly/monthly (Senin 00:00 dan tanggal 1
00:00) berpatokan pada env `BUDGET_TIMEZONE` (nama IANA, mis.
`Asia/Makassar`). Bawaan `UTC` — persis perilaku sebelum knob ini ada.
Periode daily tidak terpengaruh: ia 24 jam bergulir, bukan batas kalender.

Mengubahnya tidak menulis ulang jendela anggaran yang sedang berjalan:
anggaran yang sudah punya `period_end` mempertahankannya sampai reset
berikutnya, dan mulai periode sesudahnya seluruh pembuatan, reset worker,
dan reset manual memakai zona baru secara konsisten (satu sumber
konfigurasi yang sama). Nama zona yang salah ketik membuat gateway
menolak boot — sengaja, supaya pergeseran reset tidak terjadi diam-diam.
