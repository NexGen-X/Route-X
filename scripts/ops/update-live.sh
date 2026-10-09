#!/usr/bin/env bash
# ==============================================================================
# Route-X Zero-Downtime Live Rebuild & Hot-Reload Script
# Menjalankan pembaruan biner Route-X secara atomik tanpa mematikan database,
# cache Redis, atau reverse proxy Caddy.
# Dilengkapi validasi dependensi ketat, idempotensi concurrency lock,
# serta mekanisme safe rollback otomatis.
# ==============================================================================
set -euo pipefail
export PATH="/usr/local/go/bin:/usr/local/bin:/usr/bin:/bin:$PATH"

# Menentukan lokasi direktori repositori dan instalasi
SCRIPT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"
REPO_DIR="$(cd "$SCRIPT_DIR/../.." && pwd)"

SRC_DIR="${SRC_DIR:-$REPO_DIR}"
INSTALL_DIR="${INSTALL_DIR:-/opt/routex}"
ENV_FILE="${ENV_FILE:-/etc/routex/routex.env}"
SERVICE_NAME="${SERVICE_NAME:-routex}"
MAX_RETRIES="${MAX_RETRIES:-15}"
RETRY_DELAY="${RETRY_DELAY:-1}"

# State tracking untuk safe rollback & cleanup
ROLLBACK_REQUIRED=0
BACKUP_CREATED=0
LOCK_FD=200

echo "=================================================================="
echo " ⚡ ROUTE-X LIVE STAGING HOT-RELOAD & REBUILD"
echo "=================================================================="

# ------------------------------------------------------------------------------
# 1. Idempotensi & Concurrency Control (File Lock)
# ------------------------------------------------------------------------------
LOCK_FILE="/var/run/routex-update.lock"
if ! touch "$LOCK_FILE" 2>/dev/null; then
    LOCK_FILE="/tmp/routex-update.lock"
fi

eval "exec ${LOCK_FD}>\"$LOCK_FILE\""
if ! flock -n "$LOCK_FD"; then
    echo "❌ Error: Proses pembaruan Route-X lain sedang berjalan (lock: $LOCK_FILE)." >&2
    exit 1
fi

# ------------------------------------------------------------------------------
# Prosedur Rollback & Cleanup Trap
# ------------------------------------------------------------------------------
rollback_service() {
    # Nonaktifkan flag agar rollback tidak dijalankan berulang (non-reentrant)
    ROLLBACK_REQUIRED=0
    echo ""
    echo "🚨 =============================================================="
    echo " ⚠️ MEMULAI PROSEDUR ROLLBACK OTOMATIS (SAFE ROLLBACK)"
    echo "=================================================================="

    if [ "$BACKUP_CREATED" -eq 1 ] && [ -f "${INSTALL_DIR}/ai-gateway.bak" ]; then
        echo "🔄 [Rollback 1/3] Mengembalikan biner sebelumnya dari ${INSTALL_DIR}/ai-gateway.bak..."
        cp -fp "${INSTALL_DIR}/ai-gateway.bak" "${INSTALL_DIR}/ai-gateway"
        chmod 755 "${INSTALL_DIR}/ai-gateway"

        echo "🔄 [Rollback 2/3] Me-restart layanan ${SERVICE_NAME}.service..."
        if systemctl restart "$SERVICE_NAME"; then
            echo "🔍 [Rollback 3/3] Memverifikasi kesehatan biner pulihan di /healthz..."
            local rb_healthy=0
            local rb_count=0
            while [ "$rb_count" -lt "$MAX_RETRIES" ]; do
                local rb_code
                rb_code="$(curl -s -o /dev/null -w "%{http_code}" "http://127.0.0.1:${GATEWAY_PORT}/healthz" 2>/dev/null || true)"
                if [ "$rb_code" = "200" ]; then
                    rb_healthy=1
                    break
                fi
                rb_count=$((rb_count + 1))
                sleep "$RETRY_DELAY"
            done

            if [ "$rb_healthy" -eq 1 ]; then
                echo "✅ Rollback berhasil! Layanan ${SERVICE_NAME} kembali online dan sehat (200 OK)."
            else
                echo "❌ Peringatan kritis: Layanan di-rollback tetapi /healthz belum merespons 200 OK." >&2
                echo "   Periksa log: journalctl -u ${SERVICE_NAME} -n 50 --no-pager" >&2
            fi
        else
            echo "❌ Gagal me-restart ${SERVICE_NAME} saat prosedur rollback!" >&2
        fi
    else
        echo "⚠️ Tidak ada berkas cadangan (${INSTALL_DIR}/ai-gateway.bak) yang dapat dipulihkan." >&2
    fi
    echo "=================================================================="
}

cleanup() {
    local exit_code=$?
    # Bersihkan berkas biner sementara .new jika tersisa
    if [ -f "${INSTALL_DIR}/ai-gateway.new" ]; then
        rm -f "${INSTALL_DIR}/ai-gateway.new"
    fi

    # Jalankan rollback jika status memerlukan rollback
    if [ "$ROLLBACK_REQUIRED" -eq 1 ]; then
        rollback_service
    fi

    # Lepaskan lock file
    flock -u "$LOCK_FD" 2>/dev/null || true

    exit "$exit_code"
}
trap cleanup EXIT
trap 'exit 130' INT
trap 'exit 143' TERM

# ------------------------------------------------------------------------------
# 2. Validasi Lingkungan & Hak Akses
# ------------------------------------------------------------------------------
if [ "$(id -u)" -ne 0 ]; then
    echo "❌ Error: Skrip ini wajib dijalankan sebagai root atau dengan sudo." >&2
    exit 1
fi

if [ ! -f "$ENV_FILE" ]; then
    echo "❌ Error: Berkas konfigurasi lingkungan $ENV_FILE tidak ditemukan." >&2
    echo "   Jalankan ./install-native.sh terlebih dahulu untuk inisialisasi awal." >&2
    exit 1
fi

if [ ! -d "$SRC_DIR" ] || [ ! -f "$SRC_DIR/go.mod" ]; then
    echo "❌ Error: Direktori sumber $SRC_DIR tidak valid (go.mod tidak ditemukan)." >&2
    exit 1
fi

# ------------------------------------------------------------------------------
# 3. Validasi Dependensi Wajib Sistem
# ------------------------------------------------------------------------------
REQUIRED_TOOLS=("systemctl" "curl" "go" "git" "flock" "chmod" "cp" "mv")
MISSING_TOOLS=()
for tool in "${REQUIRED_TOOLS[@]}"; do
    if ! command -v "$tool" >/dev/null 2>&1; then
        MISSING_TOOLS+=("$tool")
    fi
done

if [ ${#MISSING_TOOLS[@]} -gt 0 ]; then
    echo "❌ Error: Dependensi sistem berikut wajib terpasang tetapi tidak ditemukan:" >&2
    for tool in "${MISSING_TOOLS[@]}"; do
        echo "   - $tool" >&2
    done
    exit 1
fi

# Baca port dari berkas konfigurasi lingkungan (default fallback 8080)
GATEWAY_PORT="8080"
PARSED_PORT="$(grep -E '^PORT=' "$ENV_FILE" | head -n1 | cut -d'=' -f2 | tr -d ' "' || true)"
if [ -n "$PARSED_PORT" ]; then
    GATEWAY_PORT="$PARSED_PORT"
fi

mkdir -p "$INSTALL_DIR"
cd "$SRC_DIR"

# ------------------------------------------------------------------------------
# 4. Validasi & Kompilasi Frontend Web UI (Opsional / Otomatis)
# ------------------------------------------------------------------------------
BUILD_WEB="${BUILD_WEB:-auto}"
SHOULD_BUILD_WEB=0

if [ "$BUILD_WEB" = "always" ] || { [ ! -d "$SRC_DIR/internal/server/dist" ] && [ ! -d "$SRC_DIR/web/dist" ]; }; then
    SHOULD_BUILD_WEB=1
elif [ "$BUILD_WEB" = "auto" ] && command -v git &>/dev/null; then
    # Cek jika ada perubahan pada folder web/ pada commit terakhir
    if git diff --name-only HEAD@{1} HEAD 2>/dev/null | grep -q "^web/"; then
        SHOULD_BUILD_WEB=1
    fi
fi

if [ "$SHOULD_BUILD_WEB" -eq 1 ]; then
    if ! command -v npm >/dev/null 2>&1; then
        echo "❌ Error: Node.js / npm tidak ditemukan untuk membangun aset Web UI." >&2
        exit 1
    fi
    echo "📦 [1/5] Membangun aset frontend Web UI..."
    cd "$SRC_DIR/web"
    npm install --no-audit --no-fund
    npm run build
    cd "$SRC_DIR"
else
    echo "⏭️ [1/5] Frontend Web UI tidak berubah / tidak perlu dibangun ulang."
fi

# ------------------------------------------------------------------------------
# 5. Dapatkan Versi Git & Kompilasi Biner Baru
# ------------------------------------------------------------------------------
GIT_COMMIT="$(git rev-parse --short HEAD 2>/dev/null || echo "unknown")"
VERSION="$(git describe --tags --always 2>/dev/null || echo "v1.4.0-live")"
BUILD_DATE="$(date -u +%Y-%m-%dT%H:%M:%SZ)"

echo "🔨 [2/5] Mengompilasi biner baru (${VERSION} @ ${GIT_COMMIT})..."
CGO_ENABLED=0 go build -trimpath \
    -ldflags="-s -w -X main.version=${VERSION} -X main.commit=${GIT_COMMIT} -X main.builtAt=${BUILD_DATE}" \
    -o "${INSTALL_DIR}/ai-gateway.new" ./cmd/ai-gateway

chmod 755 "${INSTALL_DIR}/ai-gateway.new"

# Verifikasi integritas dasar biner baru
if [ ! -x "${INSTALL_DIR}/ai-gateway.new" ]; then
    echo "❌ Error: Berkas biner ${INSTALL_DIR}/ai-gateway.new tidak dapat dieksekusi." >&2
    exit 1
fi

# Pre-flight check: uji eksekusi flag -version biner baru
if ! "${INSTALL_DIR}/ai-gateway.new" -version >/dev/null 2>&1; then
    echo "❌ Error: Biner baru gagal menjalankan pre-flight check (-version)." >&2
    exit 1
fi

# ------------------------------------------------------------------------------
# 6. Jalankan Migrasi Skema Database Non-Destruktif
# ------------------------------------------------------------------------------
echo "🗄️ [3/5] Menjalankan migrasi skema database secara non-destruktif..."
set -a
# shellcheck disable=SC1090
source "$ENV_FILE"
set +a
"${INSTALL_DIR}/ai-gateway.new" -migrate

# ------------------------------------------------------------------------------
# 7. Penukaran Biner Atomik & Restart Layanan
# ------------------------------------------------------------------------------
echo "🚀 [4/5] Menyiapkan cadangan biner dan menukar biner atomik..."

# Cadangkan biner yang sedang berjalan jika ada
if [ -f "${INSTALL_DIR}/ai-gateway" ]; then
    cp -fp "${INSTALL_DIR}/ai-gateway" "${INSTALL_DIR}/ai-gateway.bak"
    BACKUP_CREATED=1
fi

# Mulai fase kritis: aktifkan flag rollback jika terjadi kegagalan
ROLLBACK_REQUIRED=1

# Tukar biner secara atomik
mv -f "${INSTALL_DIR}/ai-gateway.new" "${INSTALL_DIR}/ai-gateway"

echo "🔄 Me-restart layanan ${SERVICE_NAME}.service..."
systemctl restart "$SERVICE_NAME"

# ------------------------------------------------------------------------------
# 8. Validasi Kesehatan Layanan (Health Check Probing)
# ------------------------------------------------------------------------------
echo "🔍 [5/5] Menunggu validasi endpoint /healthz (port ${GATEWAY_PORT})..."
HEALTHY=0
RETRY_COUNT=0

while [ "$RETRY_COUNT" -lt "$MAX_RETRIES" ]; do
    HTTP_CODE="$(curl -s -o /dev/null -w "%{http_code}" "http://127.0.0.1:${GATEWAY_PORT}/healthz" 2>/dev/null || true)"
    if [ "$HTTP_CODE" = "200" ]; then
        HEALTHY=1
        break
    fi
    RETRY_COUNT=$((RETRY_COUNT + 1))
    sleep "$RETRY_DELAY"
done

echo ""
if [ "$HEALTHY" -eq 1 ]; then
    # Penukaran berhasil dan layanan sehat, batalkan kebutuhan rollback
    ROLLBACK_REQUIRED=0

    echo "=================================================================="
    echo " ✅ LIVE UPDATE SUKSES & ONLINE!"
    echo "=================================================================="
    echo " 🔹 Versi Gateway Aktif: ${VERSION} (${GIT_COMMIT})"
    echo " 🔹 Status Layanan     : $(systemctl is-active "$SERVICE_NAME")"
    echo " 🔹 Endpoint Lokal     : http://127.0.0.1:${GATEWAY_PORT}/healthz (200 OK)"
    echo " 🔹 Edge Proxy         : http://localhost/ (Caddy port 80/443)"
    echo " 🔹 Biner Cadangan     : ${INSTALL_DIR}/ai-gateway.bak (tersedia)"
    echo "=================================================================="
else
    echo "=================================================================="
    echo " ❌ PERINGATAN: Layanan belum merespons 200 OK di /healthz setelah ${MAX_RETRIES} detik."
    echo " Memulai safe rollback otomatis ke versi sebelumnya..."
    echo "=================================================================="
    rollback_service
    exit 1
fi
