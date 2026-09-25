#!/usr/bin/env bash
# ==============================================================================
# Route-X Zero-Downtime Live Rebuild & Hot-Reload Script
# Menjalankan pembaruan biner Route-X secara atomik tanpa mematikan database,
# cache Redis, atau reverse proxy Caddy.
# ==============================================================================
set -euo pipefail

SRC_DIR="${SRC_DIR:-/root/Route-X}"
INSTALL_DIR="${INSTALL_DIR:-/opt/routex}"
ENV_FILE="${ENV_FILE:-/etc/routex/routex.env}"
SERVICE_NAME="routex"

echo "=================================================================="
echo " ⚡ ROUTE-X LIVE STAGING HOT-RELOAD & REBUILD"
echo "=================================================================="

# 1. Pastikan dijalankan sebagai root
if [ "$(id -u)" -ne 0 ]; then
    echo "❌ Error: Skrip ini wajib dijalankan sebagai root atau dengan sudo." >&2
    exit 1
fi

if [ ! -f "$ENV_FILE" ]; then
    echo "❌ Error: Berkas konfigurasi lingkungan $ENV_FILE tidak ditemukan." >&2
    echo "   Jalankan ./install-native.sh terlebih dahulu untuk inisialisasi awal." >&2
    exit 1
fi

mkdir -p "$INSTALL_DIR"
cd "$SRC_DIR"

# 2. Periksa apakah frontend Web UI perlu dibangun ulang
BUILD_WEB="${BUILD_WEB:-auto}"
if [ "$BUILD_WEB" = "always" ] || [ ! -d "$SRC_DIR/internal/server/dist" ] && [ ! -d "$SRC_DIR/web/dist" ]; then
    echo "📦 [1/4] Membangun aset frontend Web UI..."
    cd "$SRC_DIR/web"
    npm install --no-audit --no-fund
    npm run build
    cd "$SRC_DIR"
elif [ "$BUILD_WEB" = "auto" ] && command -v git &>/dev/null; then
    # Cek jika ada perubahan pada folder web/ pada commit terakhir
    if git diff --name-only HEAD@{1} HEAD 2>/dev/null | grep -q "^web/"; then
        echo "📦 [1/4] Mendeteksi perubahan frontend, membangun ulang Web UI..."
        cd "$SRC_DIR/web"
        npm install --no-audit --no-fund
        npm run build
        cd "$SRC_DIR"
    else
        echo "⏭️ [1/4] Frontend Web UI tidak berubah, melewati kompilasi npm."
    fi
else
    echo "⏭️ [1/4] Melewati kompilasi frontend."
fi

# 3. Dapatkan versi dan hash git terbaru
GIT_COMMIT="$(git rev-parse --short HEAD 2>/dev/null || echo "unknown")"
VERSION="$(git describe --tags --always 2>/dev/null || echo "v1.2.1-live")"
echo "🔨 [2/4] Mengompilasi biner baru (${VERSION} @ ${GIT_COMMIT})..."

CGO_ENABLED=0 go build -trimpath -ldflags="-s -w -X main.version=${VERSION}" -o "${INSTALL_DIR}/ai-gateway.new" ./cmd/ai-gateway
chmod 755 "${INSTALL_DIR}/ai-gateway.new"

# 4. Jalankan migrasi skema database secara non-destructive
echo "🗄️ [3/4] Menjalankan migrasi skema database (data tetap aman)..."
set -a
# shellcheck disable=SC1090
source "$ENV_FILE"
set +a
"${INSTALL_DIR}/ai-gateway.new" -migrate

# 5. Tukar biner secara atomik & restart layanan gateway
echo "🚀 [4/4] Menukar biner atomik & me-restart ${SERVICE_NAME}.service..."
mv -f "${INSTALL_DIR}/ai-gateway.new" "${INSTALL_DIR}/ai-gateway"
systemctl restart "$SERVICE_NAME"

# 6. Validasi kesehatan layanan (Health Check)
echo "🔍 Menunggu validasi endpoint /healthz..."
MAX_RETRIES=10
RETRY_COUNT=0
HEALTHY=0

while [ "$RETRY_COUNT" -lt "$MAX_RETRIES" ]; do
    HTTP_CODE="$(curl -s -o /dev/null -w "%{http_code}" http://127.0.0.1:8080/healthz 2>/dev/null || true)"
    if [ "$HTTP_CODE" = "200" ]; then
        HEALTHY=1
        break
    fi
    RETRY_COUNT=$((RETRY_COUNT + 1))
    sleep 1
done

echo ""
if [ "$HEALTHY" -eq 1 ]; then
    echo "=================================================================="
    echo " ✅ LIVE UPDATE SUKSES & ONLINE!"
    echo "=================================================================="
    echo " 🔹 Versi Gateway Aktif: ${VERSION} (${GIT_COMMIT})"
    echo " 🔹 Status Layanan     : $(systemctl is-active "$SERVICE_NAME")"
    echo " 🔹 Endpoint Lokal     : http://127.0.0.1:8080/healthz (200 OK)"
    echo " 🔹 Edge Proxy         : http://localhost/ (Caddy port 80/443)"
    echo "=================================================================="
else
    echo "=================================================================="
    echo " ⚠️ PERINGATAN: Layanan belum merespons 200 OK di /healthz."
    echo " Periksa log layanan dengan: journalctl -u ${SERVICE_NAME} -n 30 --no-pager"
    echo "=================================================================="
    exit 1
fi
