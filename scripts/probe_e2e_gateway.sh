#!/usr/bin/env bash
# ==============================================================================
# Route-X — Alat Diagnostik & Probing Sintetik E2E Gateway
# ==============================================================================
# Memvalidasi fungsionalitas inferensi live Route-X:
# 1. Healthz & Readyz probe
# 2. Katalog Model (/v1/models)
# 3. OpenAI Chat Completions (Non-streaming & SSE Streaming)
# 4. Anthropic Messages API (Dual-protocol translation)
# 5. Pengukuran Latensi & Time-To-First-Byte (TTFT)
# ==============================================================================

set -uo pipefail

BASE_URL="${ROUTEX_BASE_URL:-http://localhost:8080}"
API_KEY="${ROUTEX_API_KEY:-}"
TEST_MODEL="${ROUTEX_TEST_MODEL:-}"
VERBOSE=0

COLOR_GREEN="\033[0;32m"
COLOR_RED="\033[0;31m"
COLOR_YELLOW="\033[0;33m"
COLOR_BLUE="\033[0;34m"
COLOR_BOLD="\033[1m"
COLOR_RESET="\033[0m"

log_info()    { echo -e "${COLOR_BLUE}[INFO]${COLOR_RESET} $1"; }
log_success() { echo -e "${COLOR_GREEN}[PASS]${COLOR_RESET} $1"; }
log_warn()    { echo -e "${COLOR_YELLOW}[WARN]${COLOR_RESET} $1"; }
log_error()   { echo -e "${COLOR_RED}[FAIL]${COLOR_RESET} $1"; }

usage() {
  cat <<EOF
Penggunaan: $0 [OPSI]

Opsi:
  -u, --url URL        Base URL gateway Route-X (default: ${BASE_URL})
  -k, --key KUNCI      Kunci API Route-X (Bearer token)
  -m, --model MODEL    Nama model atau alias combo yang akan diuji
  -v, --verbose        Tampilkan payload respons lengkap
  -h, --help           Tampilkan bantuan ini

Contoh:
  $0 -k rx-admin-secret-key -m claude-3-5-sonnet
  $0 --url http://127.0.0.1:8080 -k rx-...
EOF
  exit 0
}

# Parsing Argumen
while [[ $# -gt 0 ]]; do
  case "$1" in
    -u|--url)     BASE_URL="$2"; shift 2 ;;
    -k|--key)     API_KEY="$2"; shift 2 ;;
    -m|--model)   TEST_MODEL="$2"; shift 2 ;;
    -v|--verbose) VERBOSE=1; shift ;;
    -h|--help)    usage ;;
    *) echo "Opsi tidak dikenal: $1"; usage ;;
  esac
done

# Hapus trailing slash dari base url jika ada
BASE_URL="${BASE_URL%/}"

echo "=================================================================="
echo -e "${COLOR_BOLD}Route-X — Pengujian Probing Gateway E2E (v1.0.0.1 Readiness)${COLOR_RESET}"
echo "Target URL : ${BASE_URL}"
echo "Model      : ${TEST_MODEL:-"(auto-detect /v1/models)"}"
echo "=================================================================="
echo ""

TOTAL_TESTS=0
PASSED_TESTS=0
FAILED_TESTS=0

# Helper Curl dengan output metrik
curl_with_metrics() {
  local method="$1"
  local endpoint="$2"
  local headers="$3"
  local body="$4"

  local write_out_format="HTTP_CODE:%{http_code}\nTIME_NAMELOOKUP:%{time_namelookup}\nTIME_CONNECT:%{time_connect}\nTIME_APPCONNECT:%{time_appconnect}\nTIME_PRETRANSFER:%{time_pretransfer}\nTIME_STARTTRANSFER:%{time_starttransfer}\nTIME_TOTAL:%{time_total}\n"
  
  if [[ -n "$body" ]]; then
    curl -s -S -X "$method" "${BASE_URL}${endpoint}" \
      $headers \
      -d "$body" \
      -w "$write_out_format"
  else
    curl -s -S -X "$method" "${BASE_URL}${endpoint}" \
      $headers \
      -w "$write_out_format"
  fi
}

# ------------------------------------------------------------------------------
# 1. Health & Readiness Probe
# ------------------------------------------------------------------------------
log_info "Menguji Probe Liveness (/healthz)..."
TOTAL_TESTS=$((TOTAL_TESTS + 1))
RES_HEALTH=$(curl_with_metrics "GET" "/healthz" "" "")
HTTP_HEALTH=$(echo "$RES_HEALTH" | grep "^HTTP_CODE:" | cut -d: -f2)

if [[ "$HTTP_HEALTH" == "200" ]]; then
  log_success "Endpoint /healthz merespons 200 OK"
  PASSED_TESTS=$((PASSED_TESTS + 1))
else
  log_error "Endpoint /healthz gagal! Status HTTP: ${HTTP_HEALTH:-"Connection Refused"}"
  FAILED_TESTS=$((FAILED_TESTS + 1))
fi

log_info "Menguji Probe Readiness (/readyz)..."
TOTAL_TESTS=$((TOTAL_TESTS + 1))
RES_READY=$(curl_with_metrics "GET" "/readyz" "" "")
HTTP_READY=$(echo "$RES_READY" | grep "^HTTP_CODE:" | cut -d: -f2)

if [[ "$HTTP_READY" == "200" ]]; then
  log_success "Endpoint /readyz merespons 200 OK"
  PASSED_TESTS=$((PASSED_TESTS + 1))
else
  log_warn "Endpoint /readyz merespons HTTP: ${HTTP_READY} (mungkin database/redis belum fully ready)"
fi

# ------------------------------------------------------------------------------
# 2. Katalog Model (/v1/models)
# ------------------------------------------------------------------------------
log_info "Menguji Katalog Model (/v1/models)..."
TOTAL_TESTS=$((TOTAL_TESTS + 1))
AUTH_HEADER=""
if [[ -n "$API_KEY" ]]; then
  AUTH_HEADER="-H \"Authorization: Bearer ${API_KEY}\""
fi

MODELS_RESP=$(curl -s "${BASE_URL}/v1/models" ${API_KEY:+-H "Authorization: Bearer $API_KEY"} || true)
if echo "$MODELS_RESP" | grep -q '"data"'; then
  MODEL_COUNT=$(echo "$MODELS_RESP" | grep -o '"id"' | wc -l || echo "0")
  log_success "Katalog model terdeteksi aktif (${MODEL_COUNT} model tersedia)"
  PASSED_TESTS=$((PASSED_TESTS + 1))
  
  if [[ -z "$TEST_MODEL" ]]; then
    TEST_MODEL=$(echo "$MODELS_RESP" | grep -o '"id": *"[^"]*"' | head -n 1 | cut -d'"' -f4)
    log_info "Model otomatis terpilih untuk pengujian inferensi: ${TEST_MODEL}"
  fi
else
  log_warn "Endpoint /v1/models tidak mengembalikan format OpenAI standar atau butuh otentikasi."
  if [[ -z "$TEST_MODEL" ]]; then
    TEST_MODEL="gpt-4o-mini"
  fi
fi

# ------------------------------------------------------------------------------
# 3. OpenAI Chat Completions (Non-Streaming)
# ------------------------------------------------------------------------------
log_info "Menguji /v1/chat/completions (Non-Streaming) dengan model: ${TEST_MODEL}..."
TOTAL_TESTS=$((TOTAL_TESTS + 1))

REQ_BODY=$(cat <<EOF
{
  "model": "${TEST_MODEL}",
  "messages": [
    {"role": "user", "content": "Halo Route-X, verifikasi integritas sistem v1.0.0.1. Jawab 'OK'."}
  ],
  "max_tokens": 10
}
EOF
)

START_TIME=$(date +%s%N)
CHAT_RESP=$(curl -s -w "\nHTTP_STATUS:%{http_code}\nTIME_TOTAL:%{time_total}\n" \
  -X POST "${BASE_URL}/v1/chat/completions" \
  -H "Content-Type: application/json" \
  ${API_KEY:+-H "Authorization: Bearer $API_KEY"} \
  -d "$REQ_BODY" || true)

HTTP_CHAT=$(echo "$CHAT_RESP" | grep "HTTP_STATUS:" | cut -d: -f2)
TIME_CHAT=$(echo "$CHAT_RESP" | grep "TIME_TOTAL:" | cut -d: -f2)

if [[ "$HTTP_CHAT" == "200" ]]; then
  log_success "Chat completions (Non-Streaming) berhasil (Status: 200, Total: ${TIME_CHAT}s)"
  PASSED_TESTS=$((PASSED_TESTS + 1))
  if [[ $VERBOSE -eq 1 ]]; then
    echo "Body: $(echo "$CHAT_RESP" | head -n -2)"
  fi
else
  log_warn "Chat completions mengembalikan status HTTP ${HTTP_CHAT:-"ERR"}."
  echo "Respons: $(echo "$CHAT_RESP" | head -n -2)"
  FAILED_TESTS=$((FAILED_TESTS + 1))
fi

# ------------------------------------------------------------------------------
# 4. OpenAI Chat Completions (Streaming SSE & TTFT)
# ------------------------------------------------------------------------------
log_info "Menguji /v1/chat/completions (Streaming SSE & TTFT)..."
TOTAL_TESTS=$((TOTAL_TESTS + 1))

STREAM_REQ_BODY=$(cat <<EOF
{
  "model": "${TEST_MODEL}",
  "messages": [
    {"role": "user", "content": "Hitung 1 sampai 3."}
  ],
  "stream": true,
  "max_tokens": 15
}
EOF
)

STREAM_RAW=$(curl -s -N -w "\nHTTP_STATUS:%{http_code}\nTTFT:%{time_starttransfer}\nTIME_TOTAL:%{time_total}\n" \
  -X POST "${BASE_URL}/v1/chat/completions" \
  -H "Content-Type: application/json" \
  ${API_KEY:+-H "Authorization: Bearer $API_KEY"} \
  -d "$STREAM_REQ_BODY" || true)

HTTP_STREAM=$(echo "$STREAM_RAW" | grep "HTTP_STATUS:" | cut -d: -f2)
TTFT_STREAM=$(echo "$STREAM_RAW" | grep "TTFT:" | cut -d: -f2)
TIME_STREAM=$(echo "$STREAM_RAW" | grep "TIME_TOTAL:" | cut -d: -f2)

if [[ "$HTTP_STREAM" == "200" ]] && echo "$STREAM_RAW" | grep -q "data:"; then
  log_success "Streaming SSE valid! TTFT: ${TTFT_STREAM}s, Total: ${TIME_STREAM}s"
  # Validasi penutupan stream [DONE]
  if echo "$STREAM_RAW" | grep -q "data: \[DONE\]"; then
    log_success "SSE Framing: Penanda penutup 'data: [DONE]' terverifikasi bersih"
  else
    log_warn "SSE Framing: Tidak ditemukan penanda 'data: [DONE]' pada akhir payload"
  fi
  PASSED_TESTS=$((PASSED_TESTS + 1))
else
  log_warn "Streaming SSE tidak menghasilkan status 200 atau chunk data tidak terdeteksi (HTTP: ${HTTP_STREAM:-"ERR"})"
  FAILED_TESTS=$((FAILED_TESTS + 1))
fi

# ------------------------------------------------------------------------------
# 5. Anthropic Messages API (/v1/messages)
# ------------------------------------------------------------------------------
log_info "Menguji Anthropic Messages API (/v1/messages)..."
TOTAL_TESTS=$((TOTAL_TESTS + 1))

ANTHROPIC_BODY=$(cat <<EOF
{
  "model": "${TEST_MODEL}",
  "messages": [
    {"role": "user", "content": "Tes Anthropic Messages Route-X"}
  ],
  "max_tokens": 10
}
EOF
)

ANTHROPIC_RESP=$(curl -s -w "\nHTTP_STATUS:%{http_code}\nTIME_TOTAL:%{time_total}\n" \
  -X POST "${BASE_URL}/v1/messages" \
  -H "Content-Type: application/json" \
  -H "anthropic-version: 2023-06-01" \
  ${API_KEY:+-H "x-api-key: $API_KEY"} \
  -d "$ANTHROPIC_BODY" || true)

HTTP_ANTHROPIC=$(echo "$ANTHROPIC_RESP" | grep "HTTP_STATUS:" | cut -d: -f2)
TIME_ANTHROPIC=$(echo "$ANTHROPIC_RESP" | grep "TIME_TOTAL:" | cut -d: -f2)

if [[ "$HTTP_ANTHROPIC" == "200" ]]; then
  log_success "Anthropic Messages API berhasil (Status: 200, Total: ${TIME_ANTHROPIC}s)"
  PASSED_TESTS=$((PASSED_TESTS + 1))
else
  log_warn "Anthropic Messages API mengembalikan status HTTP ${HTTP_ANTHROPIC:-"ERR"}"
  if [[ $VERBOSE -eq 1 ]]; then
    echo "Body: $(echo "$ANTHROPIC_RESP" | head -n -2)"
  fi
  FAILED_TESTS=$((FAILED_TESTS + 1))
fi

# ------------------------------------------------------------------------------
# Ringkasan Pengujian
# ------------------------------------------------------------------------------
echo ""
echo "=================================================================="
echo -e "${COLOR_BOLD}RINGKASAN DIAGNOSTIK PROBING GATEWAY${COLOR_RESET}"
echo "Total Uji  : ${TOTAL_TESTS}"
echo -e "Lolos      : ${COLOR_GREEN}${PASSED_TESTS}${COLOR_RESET}"
echo -e "Gagal/Warn : ${COLOR_RED}${FAILED_TESTS}${COLOR_RESET}"
echo "=================================================================="

if [[ $FAILED_TESTS -eq 0 ]]; then
  echo -e "${COLOR_GREEN}${COLOR_BOLD}Semua probe kritis gateway berfungsi normal!${COLOR_RESET}"
  exit 0
else
  echo -e "${COLOR_YELLOW}Beberapa probe memerlukan pemeriksaan (cek konfigurasi upstream provider atau API key).${COLOR_RESET}"
  exit 1
fi
