#!/usr/bin/env bash
# ==============================================================================
# Route-X — Generator Kunci Lingkungan untuk Deployment Render
# ==============================================================================
# Menghasilkan pasangan kunci kriptografi yang 100% valid sesuai spesifikasi
# Route-X (Base64 32-byte untuk AES-256-GCM dan HMAC-SHA256).
# ==============================================================================

set -euo pipefail

echo "=================================================================="
echo " Route-X — Kunci Kriptografi Siap Pakai untuk Render Environment"
echo "=================================================================="
echo ""
echo "# Salin nilai berikut ke tab Environment Variables di Render Dashboard:"
echo ""

# 1. SESSION_SECRET (Min 32 chars Base64)
SESSION_SECRET=$(openssl rand -base64 48 | tr -d '\n')
echo "SESSION_SECRET=\"${SESSION_SECRET}\""

# 2. ENCRYPTION_KEY (Tepat 32 bytes didekode Base64 = AES-256 key)
ENCRYPTION_KEY=$(openssl rand -base64 32 | tr -d '\n')
echo "ENCRYPTION_KEY=\"${ENCRYPTION_KEY}\""

# 3. API_KEY_PEPPER (Min 32 bytes didekode Base64 = HMAC pepper)
API_KEY_PEPPER=$(openssl rand -base64 32 | tr -d '\n')
echo "API_KEY_PEPPER=\"${API_KEY_PEPPER}\""

# 4. METRICS_TOKEN (Min 32 chars hex = Scraper authentication token)
METRICS_TOKEN=$(openssl rand -hex 32 | tr -d '\n')
echo "METRICS_TOKEN=\"${METRICS_TOKEN}\""

echo ""
echo "# Setelan Akun Administrator Awal (Opsional - default: admin@routex.local / RouteX#Initial2026!):"
echo "INITIAL_ADMIN_EMAIL=\"admin@example.com\""
PASS_VAL="$(openssl rand -base64 16 | tr -dc 'A-Za-z0-9!@#$%^&*' | head -c 16)A1!"
KEY_NAME="INITIAL_ADMIN_PASSWORD"
echo "${KEY_NAME}=\"${PASS_VAL}\""

echo ""
echo "# Domain Publik (Ganti dengan nama web service Anda di Render):"
echo "PUBLIC_URL=\"https://route-x.onrender.com\""

echo ""
echo "=================================================================="
echo " CATATAN KEAMANAN:"
echo " Simpan salinan kunci di password manager Anda. Jangan lakukan commit"
echo " kunci rahasia ini ke dalam Git repository publik."
echo "=================================================================="
