# Runbook uji beban + e2e Route-X (staging lokal)

Hasil terakhir: 2026-09-09. k6 20 VU 5 menit: 17.880 request, 0 gagal,
p95 6,16 ms (ambang 800 ms). Playwright 3 alur: lolos 3,1 detik.
Rute: gateway -> echo OpenAI-compatible lokal (scripts/load/echo-upstream.py).

## 1. Siapkan database staging

  sudo -u postgres psql -c "CREATE DATABASE routex_staging OWNER routex"

Jangan pakai database routex asli: uji beban menulis belasan ribu baris requests.

## 2. Siapkan kredensial sekali pakai

  SESS=$(openssl rand -base64 48 | tr -d '\n')
  ENC=$(openssl rand -base64 32)
  PEP=$(openssl rand -base64 32)
  MET=$(openssl rand -base64 32 | tr -d '\n')
  DBPW=$(openssl rand -base64 24 | tr -d '/+=' | cut -c1-24)
  sudo -u postgres psql -c "ALTER ROLE routex WITH PASSWORD '$DBPW'"

Simpan di /tmp/staging.env (chmod 600, JANGAN commit):

  APP_ENV=staging
  PORT=18080
  DATABASE_URL=postgres://routex:${DBPW}@127.0.0.1:5432/routex_staging?sslmode=disable
  REDIS_URL=redis://127.0.0.1:6379/1
  SESSION_SECRET=${SESS}
  ENCRYPTION_KEY=${ENC}
  API_KEY_PEPPER=${PEP}
  METRICS_TOKEN=${MET}
  INITIAL_ADMIN_EMAIL=admin-loadtest@local
  INITIAL_ADMIN_PASSWORD=<password-kuat-16+-karakter>
  UPSTREAM_ALLOW_HTTP=true
  UPSTREAM_ALLOWED_PRIVATE_ADDRS=127.0.0.1/32

Dua baris UPSTREAM_* wajib: echo lokal pakai http ke 127.0.0.1 dan
penjaga SSRF menolak keduanya tanpa izin eksplisit operator.
Hanya untuk staging lokal, jangan tiru di produksi.

## 3. Build, migrasi, nyalakan

  go build -o /tmp/ai-gateway-staging ./cmd/ai-gateway
  set -a; . /tmp/staging.env; set +a
  /tmp/ai-gateway-staging -migrate   # harus: applied=9, seed 25 permissions
  /tmp/ai-gateway-staging            # di latar; healthz di :18080

## 4. Nyalakan echo upstream

  python3 scripts/load/echo-upstream.py 19091

Echo menjawab GET /models + /v1/models dan POST /chat/completions +
/v1/chat/completions (non-stream + SSE format OpenAI, latency 5-15 ms).

## 5. Daftarkan provider + model + kredensial (via curl + cookie admin)

Login dulu (POST /api/auth/login), ganti password bila diminta
(POST /api/auth/change-password, wajib header Origin + X-CSRF-Token
diambil dari cookie routex_csrf -- lihat riwayat sesi 2026-09-09 bila lupa),
lalu:

  POST /api/admin/upstreams/providers
    {"name":"echo-loadtest","kind":"openai_compatible",
     "base_url":"http://127.0.0.1:19091","enabled":true,"priority":1}
  POST /api/admin/upstreams/providers/<id>/models {"name":"gpt-5"}
  POST /api/admin/upstreams/providers/<id>/credentials
    {"label":"echo-dummy","api_key":"<bebas, echo tidak memeriksa>"}

## 6. Buat API key beban (RPM 6000, bukan 600)

Key 600 RPM kena 429 massal pada 20 VU (60 req/detik vs jatah 10/detik).
Itu bukti limiter bekerja, bukan bug -- tapi untuk uji beban murni
butuhkan jatah longgar:

  POST /api/admin/access/api-keys
    {"name":"loadtest-key","owner_user_id":"<id-admin>","rate_limit_rpm":6000}

## 7. Jalan k6

  k6 run -e BASE_URL=http://127.0.0.1:18080 -e API_KEY=<key> scripts/load/k6-smoke.js

Skenario: 20 VU 5 menit campuran healthz + chat non-stream + chat SSE
(separuh SSE pakai timeout 2 detik untuk mensimulasikan abort klien).
Ambang: p95 < 800 ms, error < 1%.

## 8. Jalan Playwright (butuh user Viewer khusus e2e)

  POST /api/admin/access/users
    {"email":"e2e-tester@local","name":"E2E Tester",
     "password":"<kuat>","must_change_password":false}
  POST /api/admin/access/users/<id>/roles {"role_id":"<id-peran-Viewer>"}

  cd web
  BASE_URL=http://127.0.0.1:18080 E2E_EMAIL=e2e-tester@local \
    E2E_PASSWORD=<password> npx playwright test e2e/alur-inti.spec.ts

Jangan jalankan Playwright bersamaan dengan k6 bila mengukur latency:
tiga browser ikut memakai server yang sama dan mencemari angka p95.

## 9. Verifikasi kuota pasca-abort

  select bucket, request_count, success_count, error_count,
         input_tokens, output_tokens from usage_hourly order by bucket desc;

Abort SSE harus tetap tercatat (lantai token dari byte terkirim),
bukan hilang tanpa jejak.

## 10. Bersih-bersih

  pkill -f ai-gateway-staging; pkill -f echo-upstream.py
  sudo -u postgres psql -c "DROP DATABASE routex_staging"
  shred -u /tmp/staging.env /tmp/staging.cookies /tmp/loadtest_key /tmp/e2e_pw
  sudo -u postgres psql -c "ALTER ROLE routex WITH PASSWORD '<password-semula>'"
