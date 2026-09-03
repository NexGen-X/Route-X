#!/usr/bin/env bash
# Script Uji Beban Ringan dan Verifikasi Latensi Route-X AI Gateway
set -euo pipefail

TARGET_URL="${1:-http://127.0.0.1:8080}"
TOTAL_REQUESTS="${2:-500}"
CONCURRENCY="${3:-20}"

echo "================================================================"
echo "          Route-X AI Gateway — Uji Beban Ringan                 "
echo "================================================================"
echo "Target Endpoint : ${TARGET_URL}/healthz"
echo "Total Permintaan: ${TOTAL_REQUESTS}"
echo "Konkurensi      : ${CONCURRENCY}"
echo "----------------------------------------------------------------"

# Jalankan pengujian beban konkurensi menggunakan curl paralel
START_TIME=$(date +%s%N)
SUCCESS_COUNT=0
ERROR_COUNT=0

TEMP_DIR=$(mktemp -d)
trap 'rm -rf "$TEMP_DIR"' EXIT

echo "Mengirim permintaan secara konkuren..."
for i in $(seq 1 "$TOTAL_REQUESTS"); do
  echo "${TARGET_URL}/healthz"
done | xargs -n 1 -P "$CONCURRENCY" curl -s -o "$TEMP_DIR/out_\$$.log" -w "%{http_code} %{time_total}\n" > "$TEMP_DIR/results.txt"

END_TIME=$(date +%s%N)
DURATION_MS=$(( (END_TIME - START_TIME) / 1000000 ))
DURATION_SEC=$(awk -v ms="$DURATION_MS" 'BEGIN { printf "%.3f", ms / 1000 }')

TOTAL_RESPONSES=$(wc -l < "$TEMP_DIR/results.txt")
SUCCESS_200=$(grep -c "^200" "$TEMP_DIR/results.txt" || true)
NON_200=$(( TOTAL_RESPONSES - SUCCESS_200 ))

# Hitung statistik latensi rata-rata dan RPS
AVG_LATENCY_MS=$(awk '{ sum += $2; count++ } END { if (count > 0) printf "%.2f", (sum / count) * 1000; else print "0" }' "$TEMP_DIR/results.txt")
RPS=$(awk -v count="$TOTAL_RESPONSES" -v sec="$DURATION_SEC" 'BEGIN { if (sec > 0) printf "%.1f", count / sec; else print "0" }')

echo "----------------------------------------------------------------"
echo "Hasil Pengujian:"
echo "  Total Respon Diterima : ${TOTAL_RESPONSES} / ${TOTAL_REQUESTS}"
echo "  Respon Berhasil (200) : ${SUCCESS_200}"
echo "  Respon Gagal / Non-200: ${NON_200}"
echo "  Durasi Keseluruhan    : ${DURATION_SEC} detik (${DURATION_MS} ms)"
echo "  Latensi Rata-rata     : ${AVG_LATENCY_MS} ms"
echo "  Throughput (RPS)      : ${RPS} req/detik"
echo "================================================================"

if [ "$NON_200" -eq 0 ]; then
  echo "STATUS: UJI BEBAN BERHASIL (Tingkat Keberhasilan 100%)"
  exit 0
else
  echo "STATUS: TERDAPAT KEGAGALAN"
  exit 1
fi
