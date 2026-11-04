# Runbook deploy Route-X (native + systemd + Caddy)

Arsitektur: DNS publik → Caddy :443 (TLS otomatis Let's Encrypt; untuk
host berupa alamat IP, Caddy memakai CA internalnya) → reverse_proxy
127.0.0.1:8080 → systemd `routex.service`.
Dashboard tersemat di biner via `//go:embed`, jadi satu biner cukup.

Jalur yang disarankan untuk server Linux baru adalah installer resmi
`install-native.sh` (mengunduh biner rilis + verifikasi checksum,
menyiapkan PostgreSQL/Redis/Caddy/service, dan hanya mengklaim berhasil
sesudah probe `/healthz`). Runbook ini adalah padanan manualnya dan
acuan memahami apa yang installer lakukan.

## 1. Prasyarat sekali saja

  - DNS A untuk domain Anda (contoh di dokumen ini: `gateway.example.com`)
    menunjuk ke IP publik server — atau pakai alamat IP langsung sebagai
    host (sertifikat menjadi CA internal Caddy, tidak tepercaya publik).
  - Bila domain dipasang BELAKANGAN pada instalasi yang awalnya memakai
    alamat IP, jalankan ulang installer dengan `DOMAIN=<domain>`: tanpa
    itu Caddy tetap melayani sertifikat internal untuk IP dan domain
    gagal (di Cloudflare tampak sebagai error 525).
  - Caddy terpasang dari repo resmi; PostgreSQL dan Redis berjalan.
  - Database produksi (pola installer: `routex_prod`, owner `routex`),
    direktori /opt/routex dan /etc/routex, pengguna sistem `routex`.

## 2. Env produksi (/etc/routex/routex.env, chmod 600)

  APP_ENV=production
  PORT=8080
  PUBLIC_URL=https://gateway.example.com
  DATABASE_URL=postgres://routex:***@127.0.0.1:5432/routex_prod?sslmode=require
  REDIS_URL=redis://127.0.0.1:6379/0
  SESSION_SECRET=<hasilkan via scripts/ops/generate-secrets.sh>
  ENCRYPTION_KEY=<base64 tepat 32 byte — generate-secrets.sh>
  API_KEY_PEPPER=<base64 min 32 byte — generate-secrets.sh>
  METRICS_TOKEN=<min 32 karakter>
  METRICS_ALLOW_LOOPBACK=false
  RATE_LIMIT_FAIL_CLOSED=true
  TRUSTED_PROXIES=127.0.0.1,::1
  UPSTREAM_ALLOW_HTTP=false
  INITIAL_ADMIN_EMAIL=admin@routex.local
  INITIAL_ADMIN_PASSWORD=<GANTI_DENGAN_PASSWORD_KUAT_ANDA>

Hasilkan secret dengan `bash scripts/ops/generate-secrets.sh` — formatnya
sudah benar untuk validasi config (ENCRYPTION_KEY dan API_KEY_PEPPER
base64; nilai heksadesimal 64 karakter didekode menjadi 48 byte dan
membuat gateway MENOLAK boot).
Daftar env lengkap dan bawaannya: `.env.example` dan
`internal/config/config.go` — tidak ada `.env` contoh lain yang lebih
otoritatif dari kedua sumber itu.

Validasi config menolak boot bila PUBLIC_URL bukan https saat
APP_ENV=production, DATABASE_URL memakai sslmode=disable di produksi,
atau METRICS_TOKEN < 32 char di produksi.

## 3. Rilis versi baru

Jalur installer: jalankan ulang `install-native.sh`, atau
`scripts/ops/update-live.sh` untuk pembaruan terpantau (mencatat SHA
yang terpasang dan mem-probe /healthz sesudah restart).

Jalur manual dari sumber:

  cd <direktori repo>
  cd web && npm run build && cd ..   # dist tersemat saat build Go
  go build -o /opt/routex/ai-gateway ./cmd/ai-gateway
  set -a; . /etc/routex/routex.env; set +a
  /opt/routex/ai-gateway -migrate    # migrasi juga berjalan otomatis saat boot
  systemctl restart routex
  curl https://gateway.example.com/healthz && curl https://gateway.example.com/readyz

Catatan: build web DULU baru build Go, karena dist tersemat via go:embed
saat kompilasi. Urutan terbalik = dashboard basi.
Respons /healthz kini hanya `{"status":"ok"}` dan /readyz hanya
`status` + `checks` — tidak ada lagi version/commit/uptime di endpoint
publik; versi berjalan terlihat di log startup dan panel admin.

## 4. Caddy (/etc/caddy/Caddyfile)

Installer menulis blok setara berikut (dan mencadangkan Caddyfile lama
ke `Caddyfile.bak-<timestamp>` sebelum menimpa blok Route-X):

  gateway.example.com {
      encode zstd gzip {
          minimum_length 512
      }

      header {
          Strict-Transport-Security "max-age=31536000; includeSubDomains; preload"
      }

      reverse_proxy 127.0.0.1:8080 {
          header_up X-Real-Ip {remote_host}

          transport http {
              dial_timeout 5s
              keepalive 90s
              keepalive_idle_conns 250
          }
      }
  }

  caddy validate --config /etc/caddy/Caddyfile --adapter caddyfile
  systemctl reload caddy

TRUSTED_PROXIES=127.0.0.1,::1 (pola installer native: Caddy di host yang
sama) membuat gateway percaya X-Forwarded-For dari Caddy untuk
rate-limit IP yang benar. Tanpa ini, semua pengunjung terlihat sebagai
127.0.0.1 dan satu penyerang bisa menghabiskan jatah seluruh pengguna.
Untuk stack Docker Compose, bawaannya adalah subnet compose terpaku
172.30.0.0/16 (lihat deploy/caddy/Caddyfile: keepalive 90s +
flush_interval -1 + header keamanan).

## 5. Setelah deploy pertama

  1. Login ke konsol dengan email/password dari `INITIAL_ADMIN_*` yang
     Anda setel, lalu ganti password saat diminta (wajib pada login
     pertama).
  2. INITIAL_ADMIN_PASSWORD boleh dibiarkan di routex.env (izin 600):
     seed hanya membuat admin saat tabel pengguna kosong, jadi nilainya
     tidak dipakai lagi sesudah admin pertama ada. Menghapusnya juga
     aman.
  3. Verifikasi: halaman login tampil, /healthz `{"status":"ok"}`,
     /readyz 200 dengan semua checks ok.

## 6. Operasional harian

  systemctl status routex caddy
  journalctl -u routex -n 50
  Backup: lihat bagian 8 dan docs/DISASTER_RECOVERY.md.

Unit systemd resmi ada di deploy/systemd/routex.service (berjalan
sebagai pengguna `routex`, EnvironmentFile=/etc/routex/routex.env,
Restart=on-failure).

## 7. Observability (Prometheus, opsional)

Endpoint /metrics dilindungi METRICS_TOKEN (dan kebijakan loopback
METRICS_ALLOW_LOOPBACK). Prometheus adalah pelengkap opsional untuk
pemakaian pribadi — grafik dashboard TIDAK bergantung padanya: dashboard
membaca API internal + database. Alert yang disarankan ada di
docs/runbook-operasi.md §2.

## 8. Backup otomatis

Skrip resmi: `scripts/ops/backup-db.sh` — membaca DATABASE_URL dari
/etc/routex/routex.env, menulis dump SQL gzip ke /var/backups/routex/
(pola routex_prod_YYYYMMDD_HHMMSS.sql.gz), retensi 7 hari, log di
/var/log/routex/backup.log. Pasang sebagai /usr/local/bin/routex-backup.sh
lalu jadwalkan sendiri, contoh cron harian 02:00 UTC:

  0 2 * * * root /usr/local/bin/routex-backup.sh

Verifikasi berkala: pulihkan dump terbaru ke database buang dan pastikan
`SELECT count(*) FROM schema_migrations;` harus sama dengan jumlah berkas migrasi di internal/database/migrations/ pada versi biner yang dipakai (sumber kebenaran hitungan migrasi; jangan menulis angka mati di sini) (prosedur lengkap di
docs/DISASTER_RECOVERY.md §4). Restore produksi hanya lewat panel admin
(skratch-swap), bukan psql langsung.

PENTING: password role postgres dibagi semua database di server yang
sama. Jangan ALTER USER routex untuk keperluan test/lokal tanpa
mengembalikannya ke nilai di /etc/routex/routex.env — worker langsung
gagal auth (28P01) dan backup ikut gagal.
