#!/usr/bin/env bash
# Membersihkan seluruh baris data pengujian e2e dari basis data Route-X
set -euo pipefail

DSN="${DATABASE_URL:-}"
if [ -z "$DSN" ] && [ -f ".env" ]; then
  DSN=$(grep -E "^DATABASE_URL=" .env | cut -d'=' -f2- | tr -d '"' | tr -d "'" || true)
fi

if [ -z "$DSN" ]; then
  echo "PERINGATAN: DATABASE_URL tidak ditemukan. Pembersihan data database dilewati."
  exit 0
fi

echo "Membersihkan baris data pengujian e2e dari database..."

# Hapus provider uji e2e-mock-% (menghapus credentials, health_checks, model_mappings via FK cascade)
psql "$DSN" -c "DELETE FROM providers WHERE name LIKE 'e2e-mock%';"

# Hapus API key uji e2e (menghapus permissions, allowed models/providers via FK cascade)
psql "$DSN" -c "DELETE FROM api_keys WHERE name LIKE 'E2E Key%' OR name LIKE 'Live Client Key%' OR name LIKE 'e2e-%';"

# Bersihkan request logs bertanda e2e bila ada
psql "$DSN" -c "DELETE FROM requests WHERE requested_model = 'gpt-5' AND (api_key_id IS NULL OR NOT EXISTS (SELECT 1 FROM api_keys WHERE id = requests.api_key_id));" 2>/dev/null || true

# Hapus berkas sementara
rm -f /tmp/routex_mock_upstream.pid /tmp/routex_cookie_jar* mock_server.py

echo "Pembersihan selesai: Basis data bersih dari seluruh artefak uji e2e."
