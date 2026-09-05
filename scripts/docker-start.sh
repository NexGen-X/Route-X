#!/usr/bin/env bash
# ==============================================================================
# Route-X Personal AI Gateway — Skrip Peluncur Otomatis Docker Compose (1-Klik)
# Menjalankan 3 kontainer terintegrasi: Route-X Gateway + PostgreSQL 16 + Redis 7
# ==============================================================================
set -euo pipefail

SCRIPT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"
ROOT_DIR="$(cd "$SCRIPT_DIR/.." && pwd)"

cd "$ROOT_DIR"

echo "=================================================================="
echo "          Route-X AI Gateway — Memulai Docker Stack              "
echo "=================================================================="

# 1. Periksa ketersediaan Docker dan Docker Compose
if ! command -v docker &>/dev/null; then
  echo "Error: Docker belum terpasang di sistem ini."
  echo "Silakan pasang Docker terlebih dahulu."
  exit 1
fi

if ! docker compose version &>/dev/null; then
  echo "Error: Docker Compose plugin belum terpasang."
  exit 1
fi

# 2. Periksa berkas konfigurasi .env, buat otomatis jika belum ada
ENV_FILE="$ROOT_DIR/.env"
if [ ! -f "$ENV_FILE" ]; then
  echo "[1/4] Berkas .env belum ditemukan, membuat konfigurasi otomatis..."
  RAND_SEC=$(openssl rand -base64 48 | tr -dc 'a-zA-Z0-9' | head -c 48)
  RAND_ENC=$(openssl rand -base64 32)
  RAND_PEP=$(openssl rand -base64 32)
  RAND_PGPASS=$(openssl rand -base64 24 | tr -dc 'a-zA-Z0-9' | head -c 24)
  RAND_ADMINPASS=$(openssl rand -base64 16 | tr -dc 'a-zA-Z0-9' | head -c 16)

  cat <<EOF > "$ENV_FILE"
# Konfigurasi Lingkungan Route-X AI Gateway (Otomatis dibuat)
APP_ENV=development
PORT=8080
LOG_LEVEL=debug

POSTGRES_USER=routex
POSTGRES_PASSWORD=${RAND_PGPASS}
POSTGRES_DB=routex
DB_SSLMODE=disable

SESSION_SECRET=${RAND_SEC}
ENCRYPTION_KEY=${RAND_ENC}
API_KEY_PEPPER=${RAND_PEP}

INITIAL_ADMIN_EMAIL=admin@route-x.local
INITIAL_ADMIN_PASSWORD=${RAND_ADMINPASS}

UPSTREAM_ALLOW_HTTP=true
UPSTREAM_ALLOWED_PRIVATE_ADDRS=127.0.0.1
EOF
  echo "      Konfigurasi .env berhasil dibuat dengan kata sandi acak yang aman."
else
  echo "[1/4] Menggunakan konfigurasi yang ada di .env."
fi

# 3. Bangun dan jalankan seluruh stack 3 kontainer
echo "[2/4] Menjalankan 3 kontainer terpadu (Gateway, PostgreSQL 16, Redis 7)..."
docker compose up -d --build

# 4. Tunggu hingga service gateway siap melayani
echo "[3/4] Menunggu gateway siap (healthcheck)..."
GATEWAY_READY=false
for i in $(seq 1 30); do
  if curl -s -f "http://127.0.0.1:8080/healthz" &>/dev/null; then
    GATEWAY_READY=true
    break
  fi
  sleep 1
done

if [ "$GATEWAY_READY" = false ]; then
  echo "Peringatan: Gateway membutuhkan waktu lebih lama untuk merespon probe /healthz."
  echo "Periksa log dengan: docker compose logs gateway"
  exit 1
fi

echo "[4/4] Seluruh 3 layanan telah aktif dan sehat!"
echo ""
echo "=================================================================="
echo "               Route-X Berhasil Dijalankan!                       "
echo "=================================================================="
echo "  🌐 Dashboard Web    : http://localhost:8080"
echo "  🤖 API Gateway      : http://localhost:8080/v1/chat/completions"
echo "  🩺 Health Probe     : http://localhost:8080/readyz"
echo ""
echo "  👤 Akun Admin Bawaan:"
echo "     Email    : $(grep -E '^INITIAL_ADMIN_EMAIL=' "$ENV_FILE" | cut -d'=' -f2- || echo 'admin@route-x.local')"
echo "     Password : $(grep -E '^INITIAL_ADMIN_PASSWORD=' "$ENV_FILE" | cut -d'=' -f2- || echo '(lihat di .env)')"
echo ""
echo "  💡 Tips CLI:"
echo "     Untuk menghentikan: ./scripts/docker-stop.sh"
echo "     Untuk melihat log : docker compose logs -f gateway"
echo "=================================================================="
