# Runbook deploy Route-X ke gateway.example.com

Arsitektur: Cloudflare DNS -> Caddy :443 (TLS otomatis Let's Encrypt)
-> reverse_proxy 127.0.0.1:8080 (systemd routex.service).
Dashboard tersemat di biner via web/embed.go, jadi satu biner cukup.

## 1. Prasyarat sekali saja

  - DNS A gateway.example.com ke IP publik server (saat deploy: 203.0.113.100).
  - Caddy terinstall dari repo resmi (v2.11.4 saat deploy).
  - DB routex_prod (owner routex), direktori /opt/routex dan /etc/routex.

## 2. Env produksi (/etc/routex/routex.env, chmod 600)

  APP_ENV=production
  PORT=8080
  PUBLIC_URL=https://gateway.example.com
  DATABASE_URL=postgres://routex:***@127.0.0.1:5432/routex_prod?sslmode=require
  REDIS_URL=redis://127.0.0.1:6379/0
  SESSION_SECRET=<64 hex>
  ENCRYPTION_KEY=<base64 32 byte>
  API_KEY_PEPPER=<64 hex>
  METRICS_TOKEN=<64 hex>
  METRICS_ALLOW_LOOPBACK=false
  RATE_LIMIT_FAIL_CLOSED=true
  TRUSTED_PROXIES=127.0.0.1/32
  UPSTREAM_ALLOW_HTTP=false
  INITIAL_ADMIN_EMAIL=admin@routex.local
  INITIAL_ADMIN_PASSWORD=<GANTI_PASSWORD_MIN_16_KARAKTER>

Validasi config menolak boot bila PUBLIC_URL bukan https,
DATABASE_URL pakai sslmode=disable, atau METRICS_TOKEN < 32 char.

## 3. Rilis versi baru

  cd /root/Route-X && go build -o /opt/routex/ai-gateway ./cmd/ai-gateway
  cd web && npm run build   # dist tersemat saat build Go berikutnya
  set -a; . /etc/routex/routex.env; set +a
  /opt/routex/ai-gateway -migrate
  systemctl restart routex
  curl https://gateway.example.com/healthz && curl https://gateway.example.com/readyz

Catatan: build web DULU baru build Go, karena dist tersemat via go:embed
saat kompilasi. Urutan terbalik = dashboard basi.

## 4. Caddy (/etc/caddy/Caddyfile)

  gateway.example.com {
      encode zstd gzip

      reverse_proxy 127.0.0.1:8080 {
          header_up X-Real-Ip {remote_host}

          transport http {
              keepalive 300s
              keepalive_idle_conns 250
          }
      }
  }

  caddy validate --config /etc/caddy/Caddyfile --adapter caddyfile
  systemctl reload caddy

TRUSTED_PROXIES=127.0.0.1/32 membuat gateway percaya X-Forwarded-For
dari Caddy untuk rate-limit IP yang benar (lihat httpx RealIP).
Tanpa ini, semua pengunjung terlihat sebagai 127.0.0.1 dan satu
penyerang bisa menghabiskan jatah seluruh pengguna.

## 5. Setelah deploy pertama

  1. Login ke konsol (admin@routex.local atau default onboarding admin@routex.local / RouteX#Initial2026!), ganti password saat diminta.
  2. Hapus INITIAL_ADMIN_PASSWORD dari /etc/routex/routex.env jika diisi, restart.
  3. Verifikasi: landing 200 + judul Route-X, /healthz ok, /readyz 200.

## 6. Operasional harian

  systemctl status routex caddy prometheus
  journalctl -u routex -n 50
  Backup: pg_dump routex_prod -Fc -f /var/backups/routex-TANGGAL.dump

## 7. Observability (Prometheus lokal, DIMATIKAN 2026-09-09)

  Prometheus dinonaktifkan (systemctl disable --now) karena pemakaian
  pribadi tidak butuh alerting. Grafik dashboard TIDAK terpengaruh:
  ia membaca API internal + DB, bukan Prometheus.
  Nyalakan lagi bila perlu tren: systemctl enable --now prometheus.
  Config utuh di /etc/prometheus/prometheus.yml + rules/routex.yml
  (4 alert), token di /etc/prometheus/routex_token.txt.

## 8. Backup otomatis

  Cron /etc/cron.d/routex-backup: tiap 02:00 jalan sebagai root,
  /usr/local/bin/routex-backup.sh dump via su postgres ke
  /var/backups/routex/routex-TANGGAL.dump, retensi 14 hari.
  Verifikasi 2026-09-09: dump 255 KB, restore ke DB uji users=1,
  provider_models=3, schema_migrations=9.

  PENTING: password role postgres dibagi semua DB. Jangan ALTER USER
  routex untuk keperluan test/lokal tanpa mengembalikannya ke nilai
  di /etc/routex/routex.env — worker langsung 28P01 dan backup cron
  ikut gagal auth.
