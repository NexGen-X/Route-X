# Runbook operasi Route-X (produksi)

## 1. Backup + restore database

Backup harian dengan pg_dump format custom:

  pg_dump $DATABASE_URL -Fc -f /var/backups/routex-$(date +%F).dump

Restore ke database kosong (verifikasi berkala):

  createdb routex_restore
  pg_restore -d routex_restore /var/backups/routex-TANGGAL.dump
  psql -d routex_restore -tAc "SELECT count(*) FROM schema_migrations;"
  # harus: 10 (migrasi 0001 sampai 0010)

Hasil uji: dump ~254 KB, restore ke DB baru
users/models sama dengan sumber, schema_migrations=10.

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

Variabel yang wajib diganti berkala: SESSION_SECRET, ENCRYPTION_KEY,
API_KEY_PEPPER, METRICS_TOKEN (min 32 char). Prosedur:

  1. Generate nilai baru, simpan di secret manager.
  2. Rolling restart satu instance, verifikasi healthz+readyz 200.
  3. Lanjutkan ke instance berikutnya.
  4. Sesi lama invalid setelah SESSION_SECRET diganti:
     umumkan maintenance window ke pengguna.

## 5. Dependensi

  go mod verify            # harus: all modules verified
  npm audit --omit=dev     # harus: 0 vuln prod

Hasil 2026-09-09: go mod verify bersih, npm prod 0 vuln.
2 vuln dev-only (esbuild via vite, GHSA dev-server) DITERIMA:
hanya memengaruhi `vite dev`, tidak ikut build produksi.
Jangan `npm audit fix --force` (naik ke vite 8, breaking change).
