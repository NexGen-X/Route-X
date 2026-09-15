#!/usr/bin/env bash
# ==============================================================================
# Route-X — Generator Kunci Keamanan & Rahasia Produksi
# ==============================================================================
# Menghasilkan pasangan kunci kriptografi yang 100% valid sesuai spesifikasi
# Route-X (Base64 32-byte untuk AES-256-GCM dan HMAC-SHA256).
# ==============================================================================

set -euo pipefail

echo "=================================================================="
echo " Route-X — Kunci Kriptografi Siap Pakai untuk Produksi"
echo " (Kompatibel dengan Koyeb, Render, Fly.io, Docker, & VPS)"
echo "=================================================================="
echo ""
echo "# Salin nilai-nilai berikut ke panel Environment Variables layanan Anda:"
echo ""

# 1. SESSION_SECRET (Min 32 chars Base64)
SESSION_SECRET=$(openssl rand -base64 48 | tr -d '\n')
echo "SESSION_SECRET=\"${SESSION_SECRET}\""

# 2. ENCRYPTION_KEY (Tepat 32 bytes didekode Base64 = Kunci AES-256-GCM)
ENCRYPTION_KEY=$(openssl rand -base64 32 | tr -d '\n')
echo "ENCRYPTION_KEY=\"${ENCRYPTION_KEY}\""

# 3. API_KEY_PEPPER (Min 32 bytes didekode Base64 = HMAC-SHA256 pepper)
API_KEY_PEPPER=$(openssl rand -base64 32 | tr -d '\n')
echo "API_KEY_PEPPER=\"${API_KEY_PEPPER}\""

# 4. METRICS_TOKEN (Min 32 chars hex = Scraper authentication token)
METRICS_TOKEN=$(openssl rand -hex 32 | tr -d '\n')
echo "METRICS_TOKEN=\"${METRICS_TOKEN}\""

echo ""
echo "# Kredensial Super Administrator Awal:"
echo "INITIAL_ADMIN_EMAIL=\"admin@id-tech.cloud\""
RAND_PASS="$(openssl rand -base64 16 | tr -dc 'A-Za-z0-9!@#$%^&*' | head -c 14)A1!"
KEY_NAME="INITIAL_ADMIN_PASSWORD"
echo "${KEY_NAME}=\"${RAND_PASS}\""

echo ""
echo "=================================================================="
echo " CATATAN KEAMANAN PENTING:"
echo " 1. Simpan salinan nilai di atas di Password Manager aman Anda."
echo " 2. Jangan pernah melakukan commit berkas .env atau kunci ini ke Git."
echo "=================================================================="
