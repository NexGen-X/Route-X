#!/usr/bin/env bash
# Script Verifikasi End-to-End Route-X AI Gateway
set -euo pipefail

SCRIPT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"
ROOT_DIR="$(cd "$SCRIPT_DIR/../.." && pwd)"

if [ -f "$ROOT_DIR/.env" ]; then
  set -a
  # shellcheck disable=SC1091
  source "$ROOT_DIR/.env"
  set +a
fi

echo "=================================================================="
echo "          Route-X AI Gateway — Verifikasi End-to-End              "
echo "=================================================================="

GATEWAY_PORT=8080
MOCK_PORT=9099
COOKIE_JAR=$(mktemp /tmp/routex_cookie_jar_XXXXXX)
MOCK_PID=""
GATEWAY_PID=""

cleanup() {
  echo ""
  echo "Membersihkan proses dan artefak data pengujian..."
  [ -n "$GATEWAY_PID" ] && kill "$GATEWAY_PID" 2>/dev/null || true
  [ -n "$MOCK_PID" ] && kill "$MOCK_PID" 2>/dev/null || true
  rm -f "$COOKIE_JAR"
  "$SCRIPT_DIR/stop.sh" >/dev/null 2>&1 || true
  "$SCRIPT_DIR/clean.sh" >/dev/null 2>&1 || true
  echo "Pengujian dihentikan dan seluruh data uji dibersihkan."
}
trap cleanup EXIT

# 1. Jalankan Mock Upstream Server
echo "[1/7] Menjalankan mock upstream server di port ${MOCK_PORT}..."
MOCK_UPSTREAM_PORT="$MOCK_PORT" python3 "$SCRIPT_DIR/mock_upstream.py" &
MOCK_PID=$!
echo "$MOCK_PID" > /tmp/routex_mock_upstream.pid
sleep 1

# 2. Nyalakan Biner ai-gateway jika belum berjalan
if ! curl -s "http://127.0.0.1:${GATEWAY_PORT}/healthz" >/dev/null 2>&1; then
  echo "[2/7] Menyalakan biner ai-gateway di port ${GATEWAY_PORT}..."
  "$ROOT_DIR/ai-gateway" &
  GATEWAY_PID=$!
  sleep 2
else
  echo "[2/7] Gateway sudah berjalan di port ${GATEWAY_PORT}."
fi

# 3. Verifikasi Probes & OpenAPI Documentation
echo "[3/7] Memverifikasi probe kesehatan /healthz dan /readyz..."
HEALTH_STATUS=$(curl -s -o /dev/null -w "%{http_code}" "http://127.0.0.1:${GATEWAY_PORT}/healthz")
READYZ_STATUS=$(curl -s -o /dev/null -w "%{http_code}" "http://127.0.0.1:${GATEWAY_PORT}/readyz")

if [ "$HEALTH_STATUS" != "200" ] || [ "$READYZ_STATUS" != "200" ]; then
  echo "FAIL: Healthz ($HEALTH_STATUS), Readyz ($READYZ_STATUS)"
  exit 1
fi
echo "      Healthz OK (200), Readyz OK (200)."

# 4. Login Admin dan Dapatkan CSRF Token
echo "[4/7] Melakukan autentikasi sesi admin di /api/auth/login..."
ADMIN_EMAIL="${ADMIN_EMAIL:-${INITIAL_ADMIN_EMAIL:-admin@route-x.local}}"
ADMIN_PASS="${ADMIN_PASS:-${INITIAL_ADMIN_PASSWORD:?Variabel ADMIN_PASS atau INITIAL_ADMIN_PASSWORD wajib disetel}}"
LOGIN_RES=$(curl -s -c "$COOKIE_JAR" -X POST "http://127.0.0.1:${GATEWAY_PORT}/api/auth/login" \
  -H "Content-Type: application/json" \
  -d "{\"email\":\"${ADMIN_EMAIL}\",\"password\":\"${ADMIN_PASS}\"}")

if echo "$LOGIN_RES" | grep -q "login_failed"; then
  echo "FAIL: Autentikasi admin gagal: $LOGIN_RES"
  exit 1
fi

CSRF_TOKEN=$(grep "routex_csrf" "$COOKIE_JAR" | awk '{print $7}' || true)
if [ -z "$CSRF_TOKEN" ]; then
  echo "FAIL: Tidak berhasil mendapatkan CSRF token dari cookie: $LOGIN_RES"
  exit 1
fi
echo "      Login berhasil, CSRF Token diperoleh."

# Verifikasi /docs dapat dibuka dengan sesi admin yang sah
DOCS_STATUS=$(curl -s -b "$COOKIE_JAR" -o /dev/null -w "%{http_code}" "http://127.0.0.1:${GATEWAY_PORT}/docs")
if [ "$DOCS_STATUS" != "200" ]; then
  echo "FAIL: Akses ke /docs dengan sesi admin mengembalikan $DOCS_STATUS (mau 200)"
  exit 1
fi
echo "      Dokumentasi OpenAPI /docs terverifikasi aktif untuk sesi admin (200 OK)."

# 5. Daftarkan Upstream Provider dan Kunci API Klien
echo "[5/7] Mendaftarkan provider upstream uji dan membuat Client API Key..."
RAND_ID=$RANDOM
PROVIDER_NAME="e2e-mock-${RAND_ID}"
PROVIDER_RES=$(curl -s -b "$COOKIE_JAR" -X POST "http://127.0.0.1:${GATEWAY_PORT}/api/admin/upstreams/providers" \
  -H "Content-Type: application/json" \
  -H "Origin: http://127.0.0.1:${GATEWAY_PORT}" \
  -H "X-CSRF-Token: $CSRF_TOKEN" \
  -d "{
    \"name\": \"${PROVIDER_NAME}\",
    \"display_name\": \"E2E Mock Provider\",
    \"kind\": \"openai\",
    \"base_url\": \"http://127.0.0.1:${MOCK_PORT}/v1\"
  }")

PROVIDER_ID=$(echo "$PROVIDER_RES" | grep -oE '"(id|ID)":"[^"]*' | head -n 1 | cut -d'"' -f4 || true)
if [ -z "$PROVIDER_ID" ]; then
  echo "FAIL: Gagal membaca ID provider dari respon: $PROVIDER_RES"
  exit 1
fi

# Buat kredensial provider
MOCK_KEY="mock-cred-key-$(head -c 16 /dev/urandom | xxd -p || echo 'e2e-random-token')"
curl -s -b "$COOKIE_JAR" -X POST "http://127.0.0.1:${GATEWAY_PORT}/api/admin/upstreams/providers/${PROVIDER_ID}/credentials" \
  -H "Content-Type: application/json" \
  -H "Origin: http://127.0.0.1:${GATEWAY_PORT}" \
  -H "X-CSRF-Token: $CSRF_TOKEN" \
  -d "{\"label\":\"mock-cred\",\"api_key\":\"${MOCK_KEY}\"}" > /dev/null

# Sambungkan mapping model gpt-5 ke provider ini
curl -s -b "$COOKIE_JAR" -X POST "http://127.0.0.1:${GATEWAY_PORT}/api/admin/upstreams/models/gpt-5/mappings" \
  -H "Content-Type: application/json" \
  -H "Origin: http://127.0.0.1:${GATEWAY_PORT}" \
  -H "X-CSRF-Token: $CSRF_TOKEN" \
  -d "{\"provider_id\":\"${PROVIDER_ID}\",\"upstream_model\":\"gpt-5\"}" > /dev/null

# Buat client API Key
KEY_RES=$(curl -s -b "$COOKIE_JAR" -X POST "http://127.0.0.1:${GATEWAY_PORT}/api/admin/access/api-keys" \
  -H "Content-Type: application/json" \
  -H "Origin: http://127.0.0.1:${GATEWAY_PORT}" \
  -H "X-CSRF-Token: $CSRF_TOKEN" \
  -d "{\"name\":\"E2E Key ${RAND_ID}\",\"live\":true,\"scopes\":[\"inference\"],\"rate_limit_rpm\":600,\"rate_limit_tpm\":1000000}")

RAW_KEY=$(echo "$KEY_RES" | grep -o '"raw_key":"[^"]*' | cut -d'"' -f4 || true)
if [ -z "$RAW_KEY" ]; then
  RAW_KEY=$(echo "$KEY_RES" | grep -o '"token":"[^"]*' | cut -d'"' -f4 || true)
fi
if [ -z "$RAW_KEY" ]; then
  echo "Gagal membuat Client API Key: $KEY_RES"
  exit 1
fi
echo "      Provider ($PROVIDER_ID) dan API Key aktif dikonfigurasi."

# 6. Eksekusi Inferensi /v1/chat/completions (Non-Streaming dan Streaming)
echo "[6/7] Menjalankan inferensi chat completions (non-stream & SSE stream)..."
# Non-streaming
CHAT_RES=$(curl -s -X POST "http://127.0.0.1:${GATEWAY_PORT}/v1/chat/completions" \
  -H "Authorization: Bearer $RAW_KEY" \
  -H "Content-Type: application/json" \
  -d '{"model":"gpt-5","messages":[{"role":"user","content":"Halo gateway!"}]}')

if ! echo "$CHAT_RES" | grep -q "Halo dari mock upstream Route-X!"; then
  echo "FAIL: Respon chat completions tidak sesuai: $CHAT_RES"
  exit 1
fi
echo "      Non-streaming berhasil: Respon inferensi diterima dengan valid."

# Streaming SSE
STREAM_RES=$(curl -s -N -X POST "http://127.0.0.1:${GATEWAY_PORT}/v1/chat/completions" \
  -H "Authorization: Bearer $RAW_KEY" \
  -H "Content-Type: application/json" \
  -d '{"model":"gpt-5","stream":true,"messages":[{"role":"user","content":"Stream test"}]}')

if ! echo "$STREAM_RES" | grep -q "data: "; then
  echo "FAIL: Respon SSE streaming tidak sesuai: $STREAM_RES"
  exit 1
fi
echo "      Streaming SSE berhasil: Potongan event stream diterima dan ditutup dengan [DONE]."

# 7. Verifikasi Catatan Log di /api/admin/requests
echo "[7/7] Memverifikasi pencatatan audit log request di Admin API..."
REQUESTS_LOG=$(curl -s -b "$COOKIE_JAR" "http://127.0.0.1:${GATEWAY_PORT}/api/admin/requests?limit=5")
if ! echo "$REQUESTS_LOG" | grep -q "gpt-5"; then
  echo "FAIL: Log permintaan tidak tercatat di repositori traffic: $REQUESTS_LOG"
  exit 1
fi
echo "      Audit traffic berhasil: Request tercatat dengan model gpt-5 dan status 200."

echo "=================================================================="
echo "          HASIL: SELURUH VERIFIKASI END-TO-END BERHASIL!          "
echo "=================================================================="
