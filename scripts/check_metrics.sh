#!/usr/bin/env bash
# ==============================================================================
# Route-X — Alat Pemantauan & Pengecekan Metrik Gateway
# ==============================================================================
# Menarik dan mem-parsing metrik Prometheus (/metrics) Route-X:
# - Total permintaan & tingkat kegagalan (throughput & error rate)
# - Latensi & TTFT
# - Status Circuit Breaker & Failover antar provider
# - Pemakaian token & penolakan Rate Limit
# ==============================================================================

set -uo pipefail

BASE_URL="${ROUTEX_BASE_URL:-http://localhost:8080}"
METRICS_TOKEN="${METRICS_TOKEN:-}"
RAW_MODE=0
WATCH_INTERVAL=0

COLOR_GREEN="\033[0;32m"
COLOR_RED="\033[0;31m"
COLOR_YELLOW="\033[0;33m"
COLOR_BLUE="\033[0;34m"
COLOR_CYAN="\033[0;36m"
COLOR_BOLD="\033[1m"
COLOR_RESET="\033[0m"

usage() {
  cat <<EOF
Penggunaan: $0 [OPSI]

Opsi:
  -u, --url URL          Base URL gateway Route-X (default: ${BASE_URL})
  -t, --token TOKEN      METRICS_TOKEN jika diwajibkan di mode produksi
  -w, --watch DETIK      Mode pemantauan berkala setiap N detik (contoh: -w 3)
  --raw                  Cetak keluaran /metrics mentah tanpa parsing
  -h, --help             Tampilkan bantuan ini

Contoh:
  $0 -u http://127.0.0.1:8080
  $0 -t 0123456789abcdef0123456789abcdef -w 2
EOF
  exit 0
}

# Parsing Opsi
while [[ $# -gt 0 ]]; do
  case "$1" in
    -u|--url)    BASE_URL="$2"; shift 2 ;;
    -t|--token)  METRICS_TOKEN="$2"; shift 2 ;;
    -w|--watch)  WATCH_INTERVAL="$2"; shift 2 ;;
    --raw)       RAW_MODE=1; shift ;;
    -h|--help)   usage ;;
    *) echo "Opsi tidak dikenal: $1"; usage ;;
  esac
done

BASE_URL="${BASE_URL%/}"

fetch_and_parse() {
  local auth_header=""
  if [[ -n "$METRICS_TOKEN" ]]; then
    auth_header="-H \"Authorization: Bearer ${METRICS_TOKEN}\""
  fi

  local raw_metrics
  raw_metrics=$(curl -s -f "${BASE_URL}/metrics" ${METRICS_TOKEN:+-H "Authorization: Bearer $METRICS_TOKEN"} 2>/dev/null || true)

  if [[ -z "$raw_metrics" ]]; then
    echo -e "${COLOR_RED}[FAIL] Tidak dapat mengambil metrik dari ${BASE_URL}/metrics (Pastikan server aktif dan token valid).${COLOR_RESET}"
    return 1
  fi

  if [[ $RAW_MODE -eq 1 ]]; then
    echo "$raw_metrics"
    return 0
  fi

  # Header
  local now
  now=$(date '+%Y-%m-%d %H:%M:%S')
  echo -e "${COLOR_BOLD}==================================================================${COLOR_RESET}"
  echo -e "${COLOR_CYAN}Route-X Observability — Snapshot Metrik Gateway [${now}]${COLOR_RESET}"
  echo -e "${COLOR_BOLD}Target: ${BASE_URL}/metrics${COLOR_RESET}"
  echo -e "${COLOR_BOLD}==================================================================${COLOR_RESET}"
  echo ""

  # 1. Ringkasan Permintaan Gateway
  echo -e "${COLOR_BOLD}1. Lalu Lintas Gateway & Status Kode:${COLOR_RESET}"
  local req_lines
  req_lines=$(echo "$raw_metrics" | grep '^routex_gateway_requests_total' || true)
  if [[ -n "$req_lines" ]]; then
    echo "$req_lines" | while read -r line; do
      local labels val
      labels=$(echo "$line" | sed -E 's/routex_gateway_requests_total\{([^}]+)\}.*/\1/')
      val=$(echo "$line" | awk '{print $NF}')
      echo "  • ${labels}: ${COLOR_GREEN}${val}${COLOR_RESET}"
    done
  else
    echo "  (Belum ada data permintaan tercatat)"
  fi
  echo ""

  # 2. Ketahanan & Failover
  echo -e "${COLOR_BOLD}2. Ketahanan & Failover:${COLOR_RESET}"
  local failovers retries rate_limits circuit_breakers
  failovers=$(echo "$raw_metrics" | grep '^routex_resilience_failovers_total' | awk '{sum+=$NF} END {print sum+0}')
  retries=$(echo "$raw_metrics" | grep '^routex_resilience_retries_total' | awk '{sum+=$NF} END {print sum+0}')
  rate_limits=$(echo "$raw_metrics" | grep '^routex_resilience_rate_limit_rejected_total' | awk '{sum+=$NF} END {print sum+0}')
  
  echo -e "  • Total Retries             : ${COLOR_CYAN}${retries}${COLOR_RESET}"
  echo -e "  • Total Failovers           : ${COLOR_YELLOW}${failovers}${COLOR_RESET}"
  echo -e "  • Rate Limit Ditolak        : ${COLOR_RED}${rate_limits}${COLOR_RESET}"

  circuit_breakers=$(echo "$raw_metrics" | grep '^routex_resilience_circuit_breaker_state' || true)
  if [[ -n "$circuit_breakers" ]]; then
    echo "  • Status Circuit Breaker:"
    echo "$circuit_breakers" | while read -r line; do
      local pval
      pval=$(echo "$line" | awk '{print $NF}')
      local provider
      provider=$(echo "$line" | sed -E 's/.*provider="([^"]+)".*/\1/')
      local state_str="Closed (Normal)"
      if [[ "$pval" == "1" ]]; then
        state_str="${COLOR_YELLOW}Half-Open (Testing)${COLOR_RESET}"
      elif [[ "$pval" == "2" ]]; then
        state_str="${COLOR_RED}Open (Tripped/Bypass)${COLOR_RESET}"
      else
        state_str="${COLOR_GREEN}Closed (Normal)${COLOR_RESET}"
      fi
      echo -e "    - Provider [${provider}]: ${state_str}"
    done
  fi
  echo ""

  # 3. Kesehatan Provider Upstream
  echo -e "${COLOR_BOLD}3. Kesehatan Provider Upstream:${COLOR_RESET}"
  local provider_ups
  provider_ups=$(echo "$raw_metrics" | grep '^routex_provider_up' || true)
  if [[ -n "$provider_ups" ]]; then
    echo "$provider_ups" | while read -r line; do
      local pstate
      pstate=$(echo "$line" | awk '{print $NF}')
      local pname
      pname=$(echo "$line" | sed -E 's/.*provider="([^"]+)".*/\1/')
      if [[ "$pstate" == "1" ]]; then
        echo -e "  • [${pname}]: ${COLOR_GREEN}ONLINE (UP)${COLOR_RESET}"
      else
        echo -e "  • [${pname}]: ${COLOR_RED}OFFLINE (DOWN)${COLOR_RESET}"
      fi
    done
  else
    echo "  (Belum ada provider yang diinisialisasi)"
  fi
  echo ""

  # 4. Konsumsi Token & Biaya
  echo -e "${COLOR_BOLD}4. Konsumsi Token & Biaya Komputasi:${COLOR_RESET}"
  local tokens cost
  tokens=$(echo "$raw_metrics" | grep '^routex_gateway_tokens_total' | awk '{sum+=$NF} END {print sum+0}')
  cost=$(echo "$raw_metrics" | grep '^routex_gateway_cost_total_usd' | awk '{sum+=$NF} END {printf "%.6f", sum}')
  echo -e "  • Akumulasi Token           : ${COLOR_CYAN}${tokens}${COLOR_RESET}"
  echo -e "  • Akumulasi Biaya Est.      : ${COLOR_GREEN}\$${cost} USD${COLOR_RESET}"
  echo ""
  echo -e "${COLOR_BOLD}==================================================================${COLOR_RESET}"
}

if [[ $WATCH_INTERVAL -gt 0 ]]; then
  while true; do
    clear
    fetch_and_parse
    sleep "$WATCH_INTERVAL"
  done
else
  fetch_and_parse
fi
