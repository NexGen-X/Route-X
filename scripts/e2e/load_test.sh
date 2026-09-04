#!/usr/bin/env bash
# Script Uji Beban Gateway Sebenarnya (/v1/chat/completions) dengan Mock Upstream
set -euo pipefail

SCRIPT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"
ROOT_DIR="$(cd "$SCRIPT_DIR/../.." && pwd)"

if [ -f "$ROOT_DIR/.env" ]; then
  set -a
  # shellcheck disable=SC1091
  source "$ROOT_DIR/.env"
  set +a
fi

TOTAL_REQUESTS="${1:-200}"
CONCURRENCY="${2:-10}"
GATEWAY_PORT="${3:-8080}"
MOCK_PORT=9099

echo "================================================================"
echo "      Route-X AI Gateway — Uji Beban Riil /v1/chat/completions   "
echo "================================================================"
echo "Target Endpoint : http://127.0.0.1:${GATEWAY_PORT}/v1/chat/completions"
echo "Model           : gpt-5"
echo "Total Permintaan: ${TOTAL_REQUESTS}"
echo "Konkurensi      : ${CONCURRENCY}"
echo "----------------------------------------------------------------"

COOKIE_JAR=$(mktemp /tmp/routex_load_cookie_XXXXXX)
TEMP_DIR=$(mktemp -d /tmp/routex_load_test_XXXXXX)
MOCK_PID=""
GATEWAY_PID=""

cleanup() {
  echo ""
  echo "Membersihkan proses uji beban dan artefak..."
  if [ -n "${DATABASE_URL:-}" ]; then
    psql "$DATABASE_URL" -c "UPDATE rate_limits SET enabled = true WHERE scope = 'global';" > /dev/null 2>&1 || true
  fi
  [ -n "$GATEWAY_PID" ] && kill "$GATEWAY_PID" 2>/dev/null || true
  [ -n "$MOCK_PID" ] && kill "$MOCK_PID" 2>/dev/null || true
  rm -rf "$TEMP_DIR" "$COOKIE_JAR"
  "$SCRIPT_DIR/stop.sh" >/dev/null 2>&1 || true
  "$SCRIPT_DIR/clean.sh" >/dev/null 2>&1 || true
  echo "Proses pengujian beban selesai dan lingkungan dibersihkan."
}
trap cleanup EXIT

# 1. Pastikan mock upstream berjalan
if ! curl -s "http://127.0.0.1:${MOCK_PORT}/healthz" >/dev/null 2>&1; then
  echo "Menjalankan mock upstream di port ${MOCK_PORT}..."
  MOCK_UPSTREAM_PORT="$MOCK_PORT" python3 "$SCRIPT_DIR/mock_upstream.py" &
  MOCK_PID=$!
  echo "$MOCK_PID" > /tmp/routex_mock_upstream.pid
  sleep 1
fi

# 2. Pastikan gateway berjalan
if ! curl -s "http://127.0.0.1:${GATEWAY_PORT}/healthz" >/dev/null 2>&1; then
  echo "Menjalankan ai-gateway di port ${GATEWAY_PORT}..."
  "$ROOT_DIR/ai-gateway" &
  GATEWAY_PID=$!
  sleep 2
fi

# 3. Setup provider & API key
ADMIN_EMAIL="${ADMIN_EMAIL:-${INITIAL_ADMIN_EMAIL:-admin@route-x.local}}"
ADMIN_PASS="${ADMIN_PASS:-${INITIAL_ADMIN_PASSWORD:?Variabel ADMIN_PASS atau INITIAL_ADMIN_PASSWORD wajib disetel}}"

curl -s -c "$COOKIE_JAR" -X POST "http://127.0.0.1:${GATEWAY_PORT}/api/auth/login" \
  -H "Content-Type: application/json" \
  -d "{\"email\":\"${ADMIN_EMAIL}\",\"password\":\"${ADMIN_PASS}\"}" > /dev/null

CSRF_TOKEN=$(grep "routex_csrf" "$COOKIE_JAR" | awk '{print $7}' || true)
RAND_ID=$RANDOM
PROVIDER_NAME="e2e-mock-load-${RAND_ID}"

PROVIDER_RES=$(curl -s -b "$COOKIE_JAR" -X POST "http://127.0.0.1:${GATEWAY_PORT}/api/admin/upstreams/providers" \
  -H "Content-Type: application/json" \
  -H "Origin: http://127.0.0.1:${GATEWAY_PORT}" \
  -H "X-CSRF-Token: $CSRF_TOKEN" \
  -d "{\"name\":\"${PROVIDER_NAME}\",\"display_name\":\"Mock Load Provider\",\"kind\":\"openai\",\"base_url\":\"http://127.0.0.1:${MOCK_PORT}/v1\"}")

PROVIDER_ID=$(echo "$PROVIDER_RES" | grep -oE '"(id|ID)":"[^"]*' | head -n 1 | cut -d'"' -f4 || true)

# Kredensial & mapping
curl -s -b "$COOKIE_JAR" -X POST "http://127.0.0.1:${GATEWAY_PORT}/api/admin/upstreams/providers/${PROVIDER_ID}/credentials" \
  -H "Content-Type: application/json" -H "Origin: http://127.0.0.1:${GATEWAY_PORT}" -H "X-CSRF-Token: $CSRF_TOKEN" \
  -d '{"label":"load-cred","api_key":"mock-load-key"}' > /dev/null

curl -s -b "$COOKIE_JAR" -X POST "http://127.0.0.1:${GATEWAY_PORT}/api/admin/upstreams/models/gpt-5/mappings" \
  -H "Content-Type: application/json" -H "Origin: http://127.0.0.1:${GATEWAY_PORT}" -H "X-CSRF-Token: $CSRF_TOKEN" \
  -d "{\"provider_id\":\"${PROVIDER_ID}\",\"upstream_model\":\"gpt-5\"}" > /dev/null

# Client API key dengan rate limit tinggi agar tidak terkena pembatas laju saat load test
KEY_RES=$(curl -s -b "$COOKIE_JAR" -X POST "http://127.0.0.1:${GATEWAY_PORT}/api/admin/access/api-keys" \
  -H "Content-Type: application/json" -H "Origin: http://127.0.0.1:${GATEWAY_PORT}" -H "X-CSRF-Token: $CSRF_TOKEN" \
  -d "{\"name\":\"E2E Key Load ${RAND_ID}\",\"live\":true,\"scopes\":[\"inference\"],\"rate_limit_rpm\":10000,\"rate_limit_tpm\":10000000}")

RAW_KEY=$(echo "$KEY_RES" | grep -o '"raw_key":"[^"]*' | cut -d'"' -f4 || true)
if [ -z "$RAW_KEY" ]; then
  RAW_KEY=$(echo "$KEY_RES" | grep -o '"token":"[^"]*' | cut -d'"' -f4 || true)
fi

echo "Setup berhasil. Memulai pengiriman inferensi secara konkuren..."
if [ -n "${DATABASE_URL:-}" ]; then
  psql "$DATABASE_URL" -c "UPDATE rate_limits SET enabled = false WHERE scope = 'global';" > /dev/null 2>&1 || true
fi

PAYLOAD='{"model":"gpt-5","messages":[{"role":"user","content":"benchmark load test"}]}'

START_TIME=$(date +%s%N)

# Kirim permintaan secara paralel ke /v1/chat/completions
# Perbaikan issue 6.3: gunakan -o /dev/null agar tidak menimpa satu file log tunggal
for i in $(seq 1 "$TOTAL_REQUESTS"); do
  echo "http://127.0.0.1:${GATEWAY_PORT}/v1/chat/completions"
done | xargs -P "$CONCURRENCY" -I {} curl -s -o /dev/null \
  -X POST "{}" \
  -H "Authorization: Bearer $RAW_KEY" \
  -H "Content-Type: application/json" \
  -d "$PAYLOAD" \
  -w "%{http_code} %{time_total}\n" > "$TEMP_DIR/results.txt"

END_TIME=$(date +%s%N)
DURATION_MS=$(( (END_TIME - START_TIME) / 1000000 ))
DURATION_SEC=$(awk -v ms="$DURATION_MS" 'BEGIN { printf "%.3f", ms / 1000 }')

TOTAL_RESPONSES=$(wc -l < "$TEMP_DIR/results.txt")
SUCCESS_200=$(grep -c "^200" "$TEMP_DIR/results.txt" || true)
NON_200=$(( TOTAL_RESPONSES - SUCCESS_200 ))

AVG_LATENCY_MS=$(awk '{ sum += $2; count++ } END { if (count > 0) printf "%.2f", (sum / count) * 1000; else print "0" }' "$TEMP_DIR/results.txt")
RPS=$(awk -v count="$SUCCESS_200" -v sec="$DURATION_SEC" 'BEGIN { if (sec > 0) printf "%.1f", count / sec; else print "0" }')

# Hitung P50, P90, P95
sort -n -k2 "$TEMP_DIR/results.txt" > "$TEMP_DIR/sorted.txt"
P50_MS=$(awk -v total="$TOTAL_RESPONSES" 'NR==int(total*0.50)+1 { printf "%.2f", $2 * 1000 }' "$TEMP_DIR/sorted.txt")
P90_MS=$(awk -v total="$TOTAL_RESPONSES" 'NR==int(total*0.90)+1 { printf "%.2f", $2 * 1000 }' "$TEMP_DIR/sorted.txt")
P95_MS=$(awk -v total="$TOTAL_RESPONSES" 'NR==int(total*0.95)+1 { printf "%.2f", $2 * 1000 }' "$TEMP_DIR/sorted.txt")

ARTIFACT_FILE="$ROOT_DIR/docs/artifacts/load_test_results.txt"
cat << EOF | tee "$ARTIFACT_FILE"
================================================================
Route-X AI Gateway — Hasil Pengujian Beban Inferensi (/v1)
Waktu Pengujian       : $(date -u +"%Y-%m-%dT%H:%M:%SZ")
Endpoint              : /v1/chat/completions (model: gpt-5)
Total Permintaan      : ${TOTAL_REQUESTS}
Konkurensi Paralel    : ${CONCURRENCY}
----------------------------------------------------------------
Metrik Hasil:
  Total Respon        : ${TOTAL_RESPONSES} / ${TOTAL_REQUESTS}
  Sukses (HTTP 200)   : ${SUCCESS_200}
  Gagal (Non-200)     : ${NON_200}
  Durasi Uji          : ${DURATION_SEC} detik (${DURATION_MS} ms)
  Throughput (RPS)    : ${RPS} req/detik
  Latensi Rata-rata   : ${AVG_LATENCY_MS} ms
  Latensi P50         : ${P50_MS:-0} ms
  Latensi P90         : ${P90_MS:-0} ms
  Latensi P95         : ${P95_MS:-0} ms
================================================================
EOF

echo "Artefak hasil uji beban disimpan di: $ARTIFACT_FILE"

if [ "$NON_200" -eq 0 ] && [ "$TOTAL_RESPONSES" -eq "$TOTAL_REQUESTS" ]; then
  echo "STATUS: UJI BEBAN INFERENSI BERHASIL (Tingkat Keberhasilan 100%)"
  exit 0
else
  echo "STATUS: TERDAPAT KEGAGALAN DALAM UJI BEBAN"
  exit 1
fi
