# Runbook operasi Route-X (produksi)

## 1. Backup + restore database

Backup harian dengan pg_dump format custom:

  pg_dump $DATABASE_URL -Fc -f /var/backups/routex-$(date +%F).dump

Restore ke database kosong (verifikasi berkala):

  createdb routex_restore
  pg_restore -d routex_restore /var/backups/routex-TANGGAL.dump
  psql -d routex_restore -tAc "SELECT count(*) FROM schema_migrations;"
  # harus: 15 (migrasi 0001 sampai 0015)

Hasil uji: dump ~254 KB, restore ke DB baru
users/models sama dengan sumber, schema_migrations=15.

## 2. Alert yang disarankan

Ambang dari hasil beban 200 VU (p95 13 ms, error ~0):

  healthz           != 200 selama 2 menit    -> page
  readyz            != 200 selama 2 menit    -> page (cek Redis/DB)
  p95 chat          > 2000 ms selama 5 menit -> warning
  5xx / menit       > 10 selama 5 menit      -> warning
  antrian worker    macet > 10 menit         -> warning
  disk partisi requests > 80 persen          -> warning

Ambang p95 2000 ms sama dengan thresholds k6-berat.js.

## 3. Retensi data (30/7)

Konfigurasi di internal/config/config.go: retensi requests 30 hari,
usage_hourly 7 hari (nilai default, bisa dioverride env).
Cek baris tua sebelum mengubah:

  psql -f scripts/ops/retention-check.sql

Menaikkan ke 90/14 hanya bila compliance meminta + cek disk dulu
(partisi requests tumbuh ~385 MB per 294 ribu baris pada uji berat).

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

  POST /api/admin/access/sessions/revoke-all   (izin users:write)

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

Hasil 2026-09-09: go mod verify bersih, npm prod 0 vuln.
2 vuln dev-only (esbuild via vite, GHSA dev-server) DITERIMA:
hanya memengaruhi `vite dev`, tidak ikut build produksi.
Jangan `npm audit fix --force` (naik ke vite 8, breaking change).

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
