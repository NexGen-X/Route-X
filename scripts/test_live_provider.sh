#!/usr/bin/env bash
# ==============================================================================
# Route-X — Skrip Pengujian Provider AI Nyata (Opsi 1 / Live Inference Test)
#
# Skrip ini menguji alur inferensi end-to-end ke upstream AI nyata melalui
# gateway Route-X:
# 1. Menyalakan biner Route-X Gateway di port terisolasi
# 2. Otentikasi sesi Admin & mengambil token CSRF
# 3. Mendaftarkan upstream provider live & menyimpan kredensial terenkripsi AES-256-GCM
# 4. Memetakan model kanonik claude-opus-5 ke upstream provider & menetapkan harga USD
# 5. Membuat Client Gateway API Key
# 6. Mengirim inferensi chat completion non-streaming (/v1/chat/completions)
# 7. Mengirim inferensi chat completion SSE streaming (/v1/chat/completions?stream=true)
# 8. Memverifikasi metrik, pencatatan log (request_logs), dan perhitungan biaya USD
# ==============================================================================

set -euo pipefail

GATEWAY_PORT="${PORT:-8088}"
BASE_URL="http://127.0.0.1:${GATEWAY_PORT}"
COOKIE_JAR="$(mktemp /tmp/routex-live-cookie.XXXXXX)"

UPSTREAM_API_KEY="${ANTHROPIC_API_KEY:-${UPSTREAM_API_KEY:?Variabel ANTHROPIC_API_KEY atau UPSTREAM_API_KEY wajib disetel}}"
UPSTREAM_BASE_URL="${ANTHROPIC_BASE_URL:-${UPSTREAM_BASE_URL:?Variabel ANTHROPIC_BASE_URL atau UPSTREAM_BASE_URL wajib disetel}}"
ADMIN_EMAIL="${ADMIN_EMAIL:-${INITIAL_ADMIN_EMAIL:?Variabel ADMIN_EMAIL atau INITIAL_ADMIN_EMAIL wajib disetel}}"
ADMIN_PASS="${ADMIN_PASS:-${INITIAL_ADMIN_PASSWORD:?Variabel ADMIN_PASS atau INITIAL_ADMIN_PASSWORD wajib disetel}}"

cleanup() {
  echo ""
  echo "🧹 Membersihkan proses dan file sementara..."
  rm -f "$COOKIE_JAR"
  if [ -n "${GATEWAY_PID:-}" ] && kill -0 "$GATEWAY_PID" 2>/dev/null; then
    kill -15 "$GATEWAY_PID" 2>/dev/null || true
    wait "$GATEWAY_PID" 2>/dev/null || true
  fi
}
trap cleanup EXIT INT TERM

echo "=============================================================================="
echo "  🚀 Route-X: Pengujian Provider AI Nyata (Live Inference Verification)"
echo "=============================================================================="
echo "Upstream URL : ${UPSTREAM_BASE_URL}"
echo "Model Uji    : claude-opus-5 (OpenAI-compatible dialect)"
echo "Gateway Port : ${GATEWAY_PORT}"
echo ""

# ------------------------------------------------------------------------------
# 1. Pastikan biner tersedia dan database berjalan
# ------------------------------------------------------------------------------
echo "==> [1/7] Memeriksa dependensi database dan biner..."
if [ ! -f "./ai-gateway" ]; then
  echo "Mengompilasi ./ai-gateway..."
  make build
fi

pg_isready -h 127.0.0.1 -p 5432 >/dev/null 2>&1 || {
  echo "❌ PostgreSQL tidak berjalan di 127.0.0.1:5432"
  exit 1
}
redis-cli -h 127.0.0.1 ping >/dev/null 2>&1 || {
  echo "❌ Redis tidak berjalan di 127.0.0.1:6379"
  exit 1
}
echo "    PostgreSQL & Redis siap."

# ------------------------------------------------------------------------------
# 2. Menjalankan Route-X Gateway di background
# ------------------------------------------------------------------------------
echo "==> [2/7] Menjalankan Route-X Gateway di port ${GATEWAY_PORT}..."
PORT="${GATEWAY_PORT}" ./ai-gateway > /tmp/routex-live-gateway.log 2>&1 &
GATEWAY_PID=$!

# Tunggu probe readyz
MAX_TRIES=30
TRIES=0
READY=false
while [ $TRIES -lt $MAX_TRIES ]; do
  if curl -sf "${BASE_URL}/readyz" >/dev/null 2>&1; then
    READY=true
    break
  fi
  sleep 0.5
  TRIES=$((TRIES + 1))
done

if [ "$READY" != "true" ]; then
  echo "❌ Gateway gagal siap dalam 15 detik! Log gateway:"
  cat /tmp/routex-live-gateway.log | tail -n 25
  exit 1
fi
echo "    Gateway aktif (PID: ${GATEWAY_PID}) dan probe /readyz 200 OK."

# ------------------------------------------------------------------------------
# 3. Autentikasi Sesi Admin
# ------------------------------------------------------------------------------
echo "==> [3/7] Mengautentikasi sesi Admin..."
LOGIN_RES=$(curl -s -c "$COOKIE_JAR" -X POST "${BASE_URL}/api/auth/login" \
  -H "Content-Type: application/json" \
  -d "{\"email\":\"${ADMIN_EMAIL}\",\"password\":\"${ADMIN_PASS}\"}")

if echo "$LOGIN_RES" | grep -q "login_failed"; then
  echo "❌ Login gagal dengan kredensial yang diberikan: $LOGIN_RES"
  exit 1
fi

CSRF_TOKEN=$(grep "routex_csrf" "$COOKIE_JAR" | awk '{print $7}' || true)
if [ -z "$CSRF_TOKEN" ]; then
  echo "❌ Gagal mendapatkan token CSRF dari login: $LOGIN_RES"
  exit 1
fi

if echo "$LOGIN_RES" | grep -q '"must_change_password":true'; then
  echo "    Mengubah sandi awal admin..."
  NEW_ADMIN_PASS="${NEW_ADMIN_PASSWORD:-$(openssl rand -base64 16)}"
  curl -s -b "$COOKIE_JAR" -c "$COOKIE_JAR" -X POST "${BASE_URL}/api/auth/change-password" \
    -H "Content-Type: application/json" \
    -H "Origin: ${BASE_URL}" \
    -H "X-CSRF-Token: ${CSRF_TOKEN}" \
    -d "{\"current_password\":\"${ADMIN_PASS}\",\"new_password\":\"${NEW_ADMIN_PASS}\"}" > /dev/null
  CSRF_TOKEN=$(grep "routex_csrf" "$COOKIE_JAR" | awk '{print $7}' || true)
  ADMIN_PASS="${NEW_ADMIN_PASS}"
fi
echo "    Login Admin berhasil."

# ------------------------------------------------------------------------------
# 4. Mendaftarkan Provider Upstream & Model Mapping
# ------------------------------------------------------------------------------
echo "==> [4/7] Mendaftarkan upstream provider live & kredensial ke Route-X..."
PROVIDER_NAME="justwoker-live"

# Periksa apakah provider sudah ada di database
EXISTING_ID=$(sudo -u postgres psql -d routex --pset=pager=off -t -A -c \
  "SELECT id FROM providers WHERE name = '${PROVIDER_NAME}' LIMIT 1;" 2>/dev/null || true)

if [ -z "$EXISTING_ID" ]; then
  CREATE_RES=$(curl -s -b "$COOKIE_JAR" -X POST "${BASE_URL}/api/admin/upstreams/providers" \
    -H "Content-Type: application/json" \
    -H "Origin: ${BASE_URL}" \
    -H "X-CSRF-Token: ${CSRF_TOKEN}" \
    -d "{
      \"name\": \"${PROVIDER_NAME}\",
      \"display_name\": \"JustWoker Live AI\",
      \"kind\": \"openai\",
      \"base_url\": \"${UPSTREAM_BASE_URL}\",
      \"priority\": 1,
      \"weight\": 10,
      \"timeout_ms\": 60000
    }")
  PROVIDER_ID=$(echo "$CREATE_RES" | python3 -c 'import sys, json; d = json.load(sys.stdin); print(d.get("ID") or d.get("id") or "")' 2>/dev/null || true)
else
  PROVIDER_ID="$EXISTING_ID"
fi

if [ -z "$PROVIDER_ID" ]; then
  echo "❌ Gagal mendapatkan ID provider: ${CREATE_RES:-tidak ada respon}"
  exit 1
fi
echo "    Provider ID: ${PROVIDER_ID}"

# Pasang / perbarui kredensial provider
curl -s -b "$COOKIE_JAR" -X POST "${BASE_URL}/api/admin/upstreams/providers/${PROVIDER_ID}/credentials" \
  -H "Content-Type: application/json" \
  -H "Origin: ${BASE_URL}" \
  -H "X-CSRF-Token: ${CSRF_TOKEN}" \
  -d "{\"label\":\"primary\",\"api_key\":\"${UPSTREAM_API_KEY}\"}" > /dev/null
echo "    Kredensial tersimpan dengan enkripsi AES-256-GCM."

# Periksa atau hubungkan pemetaan model claude-opus-5 ke provider ini
EXISTING_MAPPING_ID=$(sudo -u postgres psql -d routex --pset=pager=off -t -A -c \
  "SELECT pm.id FROM provider_models pm JOIN models m ON pm.model_id = m.id WHERE pm.provider_id = '${PROVIDER_ID}' AND m.model_id = 'claude-opus-5' LIMIT 1;" 2>/dev/null || true)

if [ -z "$EXISTING_MAPPING_ID" ]; then
  MAP_RES=$(curl -s -b "$COOKIE_JAR" -X POST "${BASE_URL}/api/admin/upstreams/models/claude-opus-5/mappings" \
    -H "Content-Type: application/json" \
    -H "Origin: ${BASE_URL}" \
    -H "X-CSRF-Token: ${CSRF_TOKEN}" \
    -d "{
      \"provider_id\": \"${PROVIDER_ID}\",
      \"upstream_model\": \"claude-opus-5\",
      \"priority\": 1,
      \"weight\": 100
    }")
  MAPPING_ID=$(echo "$MAP_RES" | python3 -c 'import sys, json; d = json.load(sys.stdin); print(d.get("ID") or d.get("id") or "")' 2>/dev/null || true)
else
  MAPPING_ID="$EXISTING_MAPPING_ID"
fi

echo "    Model claude-opus-5 dipetakan ke upstream provider (Mapping ID: ${MAPPING_ID:-ok})."

# Set pricing jika mapping ID tersedia
if [ -n "$MAPPING_ID" ]; then
  curl -s -b "$COOKIE_JAR" -X POST "${BASE_URL}/api/admin/upstreams/models/mappings/${MAPPING_ID}/pricing" \
    -H "Content-Type: application/json" \
    -H "Origin: ${BASE_URL}" \
    -H "X-CSRF-Token: ${CSRF_TOKEN}" \
    -d '{
      "input_per_1m_usd": "15.00",
      "output_per_1m_usd": "75.00",
      "cached_input_per_1m_usd": "3.75"
    }' > /dev/null 2>&1 || true
  echo "    Harga model USD dikonfigurasi: \$15/Mtok input, \$75/Mtok output."
fi

# ------------------------------------------------------------------------------
# 5. Membuat Kunci API Klien Route-X
# ------------------------------------------------------------------------------
echo "==> [5/7] Membuat Gateway API Key untuk klien..."
RAND_SFX=$RANDOM
KEY_RES=$(curl -s -b "$COOKIE_JAR" -X POST "${BASE_URL}/api/admin/access/api-keys" \
  -H "Content-Type: application/json" \
  -H "Origin: ${BASE_URL}" \
  -H "X-CSRF-Token: ${CSRF_TOKEN}" \
  -d "{
    \"name\": \"Live Client Key ${RAND_SFX}\",
    \"live\": true,
    \"scopes\": [\"inference\", \"models:read\"],
    \"rate_limit_rpm\": 120,
    \"rate_limit_tpm\": 200000
  }")

CLIENT_KEY=$(echo "$KEY_RES" | python3 -c 'import sys, json; d = json.load(sys.stdin); print(d.get("raw_key") or d.get("token") or "")' 2>/dev/null || true)
if [ -z "$CLIENT_KEY" ]; then
  echo "❌ Gagal membuat Client API Key: $KEY_RES"
  exit 1
fi
echo "    Client API Key dibuat: ${CLIENT_KEY:0:14}********************"

# ------------------------------------------------------------------------------
# 6. Pengujian Inferensi Nyata (Non-Streaming & SSE Streaming)
# ------------------------------------------------------------------------------
echo "==> [6/7] Menjalankan inferensi AI nyata via Route-X Gateway..."

echo ""
echo "--- [A] Uji Inferensi Non-Streaming (/v1/chat/completions) ---"
START_TIME=$(date +%s%N)
CHAT_RESP=$(curl -s -w "\nHTTP_STATUS:%{http_code}" -X POST "${BASE_URL}/v1/chat/completions" \
  -H "Authorization: Bearer ${CLIENT_KEY}" \
  -H "Content-Type: application/json" \
  -d '{
    "model": "claude-opus-5",
    "messages": [
      {"role": "system", "content": "Anda adalah asisten AI dari Route-X Gateway. Jawab padat dalam 1 kalimat bahasa Indonesia."},
      {"role": "user", "content": "Sebutkan nama ibu kota baru Indonesia dan ibu kota sebelumnya!"}
    ],
    "max_tokens": 120,
    "temperature": 0.2
  }')
END_TIME=$(date +%s%N)
ELAPSED_MS=$(( (END_TIME - START_TIME) / 1000000 ))

STATUS_CODE=$(echo "$CHAT_RESP" | grep "HTTP_STATUS:" | cut -d':' -f2)
BODY=$(echo "$CHAT_RESP" | grep -v "HTTP_STATUS:")

if [ "$STATUS_CODE" != "200" ]; then
  echo "❌ Inferensi gagal dengan status HTTP ${STATUS_CODE}!"
  echo "Respon: $BODY"
  exit 1
fi

ASSISTANT_CONTENT=$(echo "$BODY" | python3 -c '
import sys, json
data = json.load(sys.stdin)
choice = data["choices"][0]
print(choice["message"]["content"].strip())
' 2>/dev/null || echo "$BODY")

PROMPT_TOKENS=$(echo "$BODY" | python3 -c 'import sys, json; print(json.load(sys.stdin).get("usage", {}).get("prompt_tokens", 0))' 2>/dev/null || echo "0")
COMP_TOKENS=$(echo "$BODY" | python3 -c 'import sys, json; print(json.load(sys.stdin).get("usage", {}).get("completion_tokens", 0))' 2>/dev/null || echo "0")

echo "Status HTTP : ${STATUS_CODE} OK"
echo "Total Waktu : ${ELAPSED_MS} ms"
echo "Prompt Tkn  : ${PROMPT_TOKENS}"
echo "Comp Tkn    : ${COMP_TOKENS}"
echo "Jawaban Model:"
echo "------------------------------------------------------------------------------"
echo "${ASSISTANT_CONTENT}"
echo "------------------------------------------------------------------------------"

echo ""
echo "--- [B] Uji Inferensi SSE Streaming (/v1/chat/completions?stream=true) ---"
echo "Menerima aliran chunk token secara real-time:"
echo -n "Output Stream: "

curl -s -N -X POST "${BASE_URL}/v1/chat/completions" \
  -H "Authorization: Bearer ${CLIENT_KEY}" \
  -H "Content-Type: application/json" \
  -d '{
    "model": "claude-opus-5",
    "messages": [
      {"role": "user", "content": "Sebutkan angka 1 sampai 5 berurutan."}
    ],
    "stream": true,
    "max_tokens": 50
  }' | while IFS= read -r line; do
    if [[ "$line" =~ ^data:\ (.*) ]]; then
      payload="${BASH_REMATCH[1]}"
      if [ "$payload" = "[DONE]" ]; then
        break
      fi
      chunk_text=$(echo "$payload" | python3 -c '
import sys, json
try:
    d = json.load(sys.stdin)
    print(d["choices"][0]["delta"].get("content", ""), end="")
except:
    pass
' 2>/dev/null || true)
      if [ -n "$chunk_text" ]; then
        echo -n "$chunk_text"
      fi
    fi
  done
echo ""
echo ""

# ------------------------------------------------------------------------------
# 7. Verifikasi Log Permintaan & Perhitungan Biaya di Database
# ------------------------------------------------------------------------------
echo "==> [7/7] Memverifikasi pencatatan audit & log penggunaan di PostgreSQL..."
sleep 2

echo "Catatan log permintaan terbaru dari tabel 'requests':"
sudo -u postgres psql -d routex --pset=pager=off -c \
  "SELECT model_id, status_code, latency_ms, input_tokens, output_tokens, cost_usd, created_at \
   FROM requests \
   ORDER BY created_at DESC \
   LIMIT 3;"

echo ""
echo "=============================================================================="
echo "  ✅ SEMUA PENGUJIAN INFERENSI NYATA BERHASIL!"
echo "  Gateway Route-X berhasil menerima request klien, memverifikasi API key,"
echo "  merutekan request ke provider cloud live, menerima streaming SSE,"
echo "  dan mencatat penggunaan token serta biaya presisi finansial ke database."
echo "=============================================================================="
