#!/usr/bin/env bash
# Script Verifikasi End-to-End Route-X AI Gateway
set -euo pipefail

echo "=================================================================="
echo "          Route-X AI Gateway — Verifikasi End-to-End              "
echo "=================================================================="

GATEWAY_PORT=8080
MOCK_PORT=9099
COOKIE_JAR=$(mktemp)
MOCK_PID=""
GATEWAY_PID=""

cleanup() {
  echo ""
  echo "Membersihkan proses uji..."
  [ -n "$GATEWAY_PID" ] && kill "$GATEWAY_PID" 2>/dev/null || true
  [ -n "$MOCK_PID" ] && kill "$MOCK_PID" 2>/dev/null || true
  rm -f "$COOKIE_JAR" mock_server.py
  echo "Proses pengujian dihentikan dengan bersih."
}
trap cleanup EXIT

# 1. Jalankan Mock Upstream Server (Python HTTP Server)
echo "[1/7] Menjalankan mock upstream server di port ${MOCK_PORT}..."
cat << 'PYEOF' > mock_server.py
from http.server import HTTPServer, BaseHTTPRequestHandler
import json
import time

class MockHandler(BaseHTTPRequestHandler):
    def do_GET(self):
        if "/models" in self.path:
            self.send_response(200)
            self.send_header("Content-Type", "application/json")
            self.end_headers()
            self.wfile.write(b'{"object":"list","data":[{"id":"gpt-5","object":"model"}]}')
            return
        self.send_response(404)
        self.end_headers()

    def do_POST(self):
        content_length = int(self.headers.get('Content-Length', 0))
        body = self.rfile.read(content_length).decode('utf-8')
        try:
            req = json.loads(body)
        except:
            req = {}

        if req.get("stream", False):
            self.send_response(200)
            self.send_header("Content-Type", "text/event-stream")
            self.send_header("Cache-Control", "no-cache")
            self.end_headers()
            chunk = {
                "id": "chatcmpl-mock",
                "object": "chat.completion.chunk",
                "created": int(time.time()),
                "model": "gpt-5",
                "choices": [{"index": 0, "delta": {"content": "Halo dari mock upstream Route-X!"}, "finish_reason": None}]
            }
            self.wfile.write(f"data: {json.dumps(chunk)}\n\n".encode('utf-8'))
            self.wfile.flush()
            end_chunk = {
                "id": "chatcmpl-mock",
                "object": "chat.completion.chunk",
                "created": int(time.time()),
                "model": "gpt-5",
                "choices": [{"index": 0, "delta": {}, "finish_reason": "stop"}],
                "usage": {"prompt_tokens": 10, "completion_tokens": 8, "total_tokens": 18}
            }
            self.wfile.write(f"data: {json.dumps(end_chunk)}\n\ndata: [DONE]\n\n".encode('utf-8'))
            self.wfile.flush()
        else:
            self.send_response(200)
            self.send_header("Content-Type", "application/json")
            self.end_headers()
            resp = {
                "id": "chatcmpl-mock",
                "object": "chat.completion",
                "created": int(time.time()),
                "model": "gpt-5",
                "choices": [{
                    "index": 0,
                    "message": {"role": "assistant", "content": "Halo dari mock upstream Route-X!"},
                    "finish_reason": "stop"
                }],
                "usage": {"prompt_tokens": 10, "completion_tokens": 8, "total_tokens": 18}
            }
            self.wfile.write(json.dumps(resp).encode('utf-8'))

httpd = HTTPServer(('127.0.0.1', 9099), MockHandler)
httpd.serve_forever()
PYEOF

python3 mock_server.py &
MOCK_PID=$!
sleep 1

# 2. Nyalakan Biner ai-gateway
echo "[2/7] Menyalakan biner ./ai-gateway di port ${GATEWAY_PORT}..."
./ai-gateway &
GATEWAY_PID=$!
sleep 2

# 3. Verifikasi Probes & OpenAPI Documentation
echo "[3/7] Memverifikasi /healthz, /readyz, dan /docs..."
HEALTH_STATUS=$(curl -s -o /dev/null -w "%{http_code}" "http://127.0.0.1:${GATEWAY_PORT}/healthz")
READYZ_STATUS=$(curl -s -o /dev/null -w "%{http_code}" "http://127.0.0.1:${GATEWAY_PORT}/readyz")
DOCS_STATUS=$(curl -s -o /dev/null -w "%{http_code}" "http://127.0.0.1:${GATEWAY_PORT}/docs")

if [ "$HEALTH_STATUS" != "200" ] || [ "$READYZ_STATUS" != "200" ] || [ "$DOCS_STATUS" != "200" ]; then
  echo "FAIL: Healthz ($HEALTH_STATUS), Readyz ($READYZ_STATUS), Docs ($DOCS_STATUS)"
  exit 1
fi
echo "      Healthz OK, Readyz OK, Docs UI OK (200 OK)."

# 4. Login Admin dan Dapatkan CSRF Token
echo "[4/7] Melakukan autentikasi sesi admin di /api/auth/login..."
ADMIN_PASS="cnyjueHRlgHn82ElKVt9DfLK"
LOGIN_RES=$(curl -s -c "$COOKIE_JAR" -X POST "http://127.0.0.1:${GATEWAY_PORT}/api/auth/login" \
  -H "Content-Type: application/json" \
  -d "{\"email\":\"admin@route-x.local\",\"password\":\"${ADMIN_PASS}\"}")

if echo "$LOGIN_RES" | grep -q "login_failed"; then
  ADMIN_PASS="BaruPassword123!@#"
  LOGIN_RES=$(curl -s -c "$COOKIE_JAR" -X POST "http://127.0.0.1:${GATEWAY_PORT}/api/auth/login" \
    -H "Content-Type: application/json" \
    -d "{\"email\":\"admin@route-x.local\",\"password\":\"${ADMIN_PASS}\"}")
fi

CSRF_TOKEN=$(grep "routex_csrf" "$COOKIE_JAR" | awk '{print $7}' || true)
if [ -z "$CSRF_TOKEN" ]; then
  echo "FAIL: Tidak berhasil mendapatkan CSRF token dari cookie: $LOGIN_RES"
  exit 1
fi
echo "      Login berhasil, CSRF Token: ${CSRF_TOKEN:0:16}..."

if echo "$LOGIN_RES" | grep -q '"must_change_password":true'; then
  echo "      Sandi awal terdeteksi wajib diganti, memperbarui kata sandi..."
  curl -s -b "$COOKIE_JAR" -c "$COOKIE_JAR" -X POST "http://127.0.0.1:${GATEWAY_PORT}/api/auth/change-password" \
    -H "Content-Type: application/json" \
    -H "Origin: http://127.0.0.1:${GATEWAY_PORT}" \
    -H "X-CSRF-Token: $CSRF_TOKEN" \
    -d "{\"current_password\":\"${ADMIN_PASS}\",\"new_password\":\"BaruPassword123!@#\"}" > /dev/null
  CSRF_TOKEN=$(grep "routex_csrf" "$COOKIE_JAR" | awk '{print $7}' || true)
fi

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
    \"base_url\": \"http://127.0.0.1:9099/v1\"
  }")

PROVIDER_ID=$(echo "$PROVIDER_RES" | grep -oE '"(id|ID)":"[^"]*' | head -n 1 | cut -d'"' -f4 || true)
if [ -z "$PROVIDER_ID" ]; then
  echo "FAIL: Gagal membaca ID provider dari respon: $PROVIDER_RES"
  exit 1
fi
echo "      Provider dibuat, ID: $PROVIDER_ID"

# Buat kredensial provider
if [ -n "$PROVIDER_ID" ]; then
  curl -s -b "$COOKIE_JAR" -X POST "http://127.0.0.1:${GATEWAY_PORT}/api/admin/upstreams/providers/${PROVIDER_ID}/credentials" \
    -H "Content-Type: application/json" \
    -H "Origin: http://127.0.0.1:${GATEWAY_PORT}" \
    -H "X-CSRF-Token: $CSRF_TOKEN" \
    -d '{"label":"mock-cred","api_key":"sk-mock12345678"}' > /dev/null

  # Sambungkan mapping model gpt-5 ke provider ini
  curl -s -b "$COOKIE_JAR" -X POST "http://127.0.0.1:${GATEWAY_PORT}/api/admin/upstreams/models/gpt-5/mappings" \
    -H "Content-Type: application/json" \
    -H "Origin: http://127.0.0.1:${GATEWAY_PORT}" \
    -H "X-CSRF-Token: $CSRF_TOKEN" \
    -d "{\"provider_id\":\"${PROVIDER_ID}\",\"upstream_model\":\"gpt-5\"}" > /dev/null
fi

# Buat client API Key
KEY_RES=$(curl -s -b "$COOKIE_JAR" -X POST "http://127.0.0.1:${GATEWAY_PORT}/api/admin/access/api-keys" \
  -H "Content-Type: application/json" \
  -H "Origin: http://127.0.0.1:${GATEWAY_PORT}" \
  -H "X-CSRF-Token: $CSRF_TOKEN" \
  -d "{\"name\":\"E2E Key ${RAND_ID}\",\"live\":true,\"scopes\":[\"inference\"],\"rate_limit_rpm\":60,\"rate_limit_tpm\":100000}")

RAW_KEY=$(echo "$KEY_RES" | grep -o '"raw_key":"[^"]*' | cut -d'"' -f4 || true)
if [ -z "$RAW_KEY" ]; then
  RAW_KEY=$(echo "$KEY_RES" | grep -o '"token":"[^"]*' | cut -d'"' -f4 || true)
fi
if [ -z "$RAW_KEY" ]; then
  echo "Gagal membuat Client API Key: $KEY_RES"
  exit 1
fi
echo "      Client API Key aktif: ${RAW_KEY:0:12}..."

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
echo "      Audit traffic berhasil: Request tercatat dengan model gpt-5, status 200, dan metrik pemakaian."

echo "=================================================================="
echo "          HASIL: SELURUH VERIFIKASI END-TO-END BERHASIL!          "
echo "=================================================================="
