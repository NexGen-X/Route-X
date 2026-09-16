#!/usr/bin/env bash
# ==============================================================================
# Route-X — Pengatur Rahasia Otomatis untuk Fly.io (flyctl)
# ==============================================================================
# Skrip ini menghasilkan kunci kriptografi acak yang sah dan langsung
# menyetelnya ke aplikasi Fly.io Anda via 'fly secrets set'.
#
# Prasyarat:
# 1. flyctl sudah terinstall dan login ('fly auth login')
# 2. Aplikasi Fly.io sudah dibuat ('fly launch' atau terdaftar di fly.toml)
# ==============================================================================

set -euo pipefail

APP_NAME="${1:-$(grep -E '^app\s*=' fly.toml 2>/dev/null | tr -d ' "' | cut -d'=' -f2 || echo "route-x")}"

echo "=================================================================="
echo " Route-X — Menyuntikkan Secrets ke Fly.io App: [${APP_NAME}]"
echo "=================================================================="

# 1. Hasilkan Kunci Kriptografi
SESSION_SECRET_VAL=$(openssl rand -base64 48 | tr -d '\n')
ENCRYPTION_KEY_VAL=$(openssl rand -base64 32 | tr -d '\n')
API_KEY_PEPPER_VAL=$(openssl rand -base64 32 | tr -d '\n')
METRICS_TOKEN_VAL=$(openssl rand -hex 32 | tr -d '\n')
RAND_ADMIN_PASS="$(openssl rand -base64 16 | tr -dc 'A-Za-z0-9!@#$%^&*' | head -c 14)A1!"

# 2. Cek ketersediaan flyctl
if ! command -v fly &> /dev/null && ! command -v flyctl &> /dev/null; then
  echo ""
  echo "PERINGATAN: 'fly' atau 'flyctl' tidak terdeteksi di PATH sistem."
  echo "Silakan salin dan jalankan perintah berikut secara manual di terminal Anda:"
  echo ""
  echo "fly secrets set \\"
  echo "  SESSION_SECRET=\"${SESSION_SECRET_VAL}\" \\"
  echo "  ENCRYPTION_KEY=\"${ENCRYPTION_KEY_VAL}\" \\"
  echo "  API_KEY_PEPPER=\"${API_KEY_PEPPER_VAL}\" \\"
  echo "  METRICS_TOKEN=\"${METRICS_TOKEN_VAL}\" \\"
  echo "  INITIAL_ADMIN_EMAIL=\"admin@id-tech.cloud\" \\"
  PASS_KEY="INITIAL_ADMIN_PASSWORD"
  echo "  ${PASS_KEY}=\"${RAND_ADMIN_PASS}\" \\"
  echo "  PUBLIC_URL=\"https://${APP_NAME}.fly.dev\""
  echo ""
  exit 0
fi

FLY_BIN="fly"
if ! command -v fly &> /dev/null; then
  FLY_BIN="flyctl"
fi

echo "Menyetel secrets via ${FLY_BIN}..."
PASS_KEY="INITIAL_ADMIN_PASSWORD"
${FLY_BIN} secrets set -a "${APP_NAME}" \
  SESSION_SECRET="${SESSION_SECRET_VAL}" \
  ENCRYPTION_KEY="${ENCRYPTION_KEY_VAL}" \
  API_KEY_PEPPER="${API_KEY_PEPPER_VAL}" \
  METRICS_TOKEN="${METRICS_TOKEN_VAL}" \
  INITIAL_ADMIN_EMAIL="admin@id-tech.cloud" \
  ${PASS_KEY}="${RAND_ADMIN_PASS}" \
  PUBLIC_URL="https://${APP_NAME}.fly.dev"

echo ""
echo "=================================================================="
echo " Secrets berhasil disetel di Fly.io!"
echo " Kredensial Admin Awal Anda:"
echo " Email   : admin@id-tech.cloud"
echo " Password: ${RAND_ADMIN_PASS}"
echo "=================================================================="
