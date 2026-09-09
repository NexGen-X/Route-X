# Runbook deploy Route-X ke id-tech.cloud

Arsitektur: Cloudflare DNS -> Caddy :443 (TLS otomatis Let's Encrypt)
-> reverse_proxy 127.0.0.1:8080 (systemd routex.service).
Dashboard tersemat di biner via web/embed.go, jadi satu biner cukup.

## 1. Prasyarat sekali saja

  - DNS A id-tech.cloud ke IP publik server (saat deploy: 54.179.116.100).
  - Caddy terinstall dari repo resmi (v2.11.4 saat deploy).
  - DB routex_prod (owner routex), direktori /opt/routex dan /etc/routex.

## 2. Env produksi (/etc/routex/routex.env, chmod 600)

  APP_ENV=production
  PORT=8080
  PUBLIC_URL=https://id-tech.cloud
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
  INITIAL_ADMIN_EMAIL=admin@id-tech.cloud
  INITIAL_ADMIN_PASSWORD=<GANTI_PASSWORD_MIN_16_KARAKTER>

Validasi config menolak boot bila PUBLIC_URL bukan https,
DATABASE_URL pakai sslmode=disable, atau METRICS_TOKEN < 32 char.

## 3. Rilis versi baru

  cd /root/Route-X && go build -o /opt/routex/ai-gateway ./cmd/ai-gateway
  cd web && npm run build   # dist tersemat saat build Go berikutnya
  set -a; . /etc/routex/routex.env; set +a
  /opt/routex/ai-gateway -migrate
  systemctl restart routex
  curl https://id-tech.cloud/healthz && curl https://id-tech.cloud/readyz

Catatan: build web DULU baru build Go, karena dist tersemat via go:embed
saat kompilasi. Urutan terbalik = dashboard basi.

## 4. Caddy (/etc/caddy/Caddyfile)

  id-tech.cloud {
      reverse_proxy 127.0.0.1:8080 {
          header_up X-Forwarded-For {remote_host}
          header_up X-Forwarded-Proto {scheme}
          header_up X-Real-Ip {remote_host}
      }
      encode gzip
  }

  caddy validate --config /etc/caddy/Caddyfile --adapter caddyfile
  systemctl reload caddy

TRUSTED_PROXIES=127.0.0.1/32 membuat gateway percaya X-Forwarded-For
dari Caddy untuk rate-limit IP yang benar (lihat httpx RealIP).
Tanpa ini, semua pengunjung terlihat sebagai 127.0.0.1 dan satu
penyerang bisa menghabiskan jatah seluruh pengguna.

## 5. Setelah deploy pertama

  1. Login admin@id-tech.cloud, ganti password saat diminta.
  2. Hapus INITIAL_ADMIN_PASSWORD dari /etc/routex/routex.env, restart.
  3. Verifikasi: landing 200 + judul Route-X, /healthz ok, /readyz 200.

## 6. Operasional harian

  systemctl status routex caddy prometheus
  journalctl -u routex -n 50
  Backup: pg_dump routex_prod -Fc -f /var/backups/routex-TANGGAL.dump

## 7. Observability (Prometheus lokal)

  Prometheus :9090 (paket debian), scrape 127.0.0.1:8080/metrics 30 detik
  dengan bearer token dari /etc/prometheus/routex_token.txt (isi =
  METRICS_TOKEN produksi, chmod 600, owner prometheus).
  Config /etc/prometheus/prometheus.yml + rules
  /etc/prometheus/rules/routex.yml (4 alert: RoutexDown 2 mnt,
  RoutexP95Tinggi >2 dtk 5 mnt, RoutexErrorBanyak >10/mnt 5 mnt,
  RoutexProviderMati 5 mnt). Verifikasi 2026-09-09: up routex=1,
  justworker=1, 4 rules aktif.

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
