#!/usr/bin/env bash
# Runbook index non-blokir Route-X: membuat index BARU dengan CONCURRENTLY.
#
# Kapan dipakai: hanya saat menambah index baru di produksi yang tabelnya sudah
# besar (requests, request_events, request_payloads). Migrasi bawaan memakai
# CREATE INDEX biasa di dalam transaksi (aman untuk tabel kecil/kosong), jadi
# skrip ini TIDAK mengulang index yang sudah ada -- ia template untuk index baru.
#
# Cara pakai:
#   1. Tulis definisi index baru di variabel INDEXES di bawah (satu per baris).
#   2. Jalankan saat trafik sepi: ./scripts/ops/create-indexes-concurrently.sh
#   3. Awasi progres di sesi psql lain:
#        select phase, blocks_total, blocks_done from pg_stat_progress_create_index;
#
# Syarat: DATABASE_URL wajib diset. Skrip berhenti pada error pertama (set -e).
# CONCURRENTLY tidak bisa di dalam blok transaksi, jadi tiap index jalan sendiri.
set -euo pipefail

: "${DATABASE_URL:?[REDACTED] isi DATABASE_URL dulu}"

# Tulis index BARU di sini, format: "nama_index|perintah CREATE INDEX CONCURRENTLY ..."
# Contoh (jangan aktifkan tanpa kebutuhan nyata):
INDEXES=(
  # "requests_contoh_baru|CREATE INDEX CONCURRENTLY IF NOT EXISTS requests_contoh_baru ON requests (created_at DESC) WHERE status_code >= 500;"
)

if [ "${#INDEXES[@]}" -eq 0 ]; then
  echo "Tidak ada index baru terdaftar. Tambahkan dulu ke variabel INDEXES di skrip ini."
  echo "Cek index yang hilang dengan: psql \"\$DATABASE_URL\" -c \"select schemaname, tablename from pg_stat_user_tables where seq_scan > 1000;\""
  exit 0
fi

for entri in "${INDEXES[@]}"; do
  nama="${entri%%|*}"
  perintah="${entri#*|}"
  echo "==> membuat index ${nama} ..."
  # statement_timeout 0: build index besar bisa lama, jangan dibunuh timeout default.
  psql "$DATABASE_URL" -v ON_ERROR_STOP=1 \
    -c "SET statement_timeout = 0;" \
    -c "${perintah}"
  echo "==> ${nama} selesai."
done

echo "Semua index selesai. Validasi:"
psql "$DATABASE_URL" -c "select indexrelid::regclass as index, indisvalid as valid from pg_index where indexrelid::regclass::text like '%requests%' or indexrelid::regclass::text like '%payloads%';"
