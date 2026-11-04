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
GATEWAY_PORT="${PORT:-8080}"
# Jendela probe kesehatan: bawaan 60 x 2 detik = ±120 detik (V-DP-16).
# Bawaan lama 15x1 detik terlalu pendek untuk start dingin + migrasi,
# sehingga update yang sehat pun dinyatakan gagal dan di-rollback.
MAX_RETRIES="${MAX_RETRIES:-60}"
RETRY_DELAY="${RETRY_DELAY:-2}"

# State tracking untuk safe rollback & cleanup
ROLLBACK_REQUIRED=0
BACKUP_CREATED=0
LOCK_FD=200

echo "=================================================================="
echo " ⚡ ROUTE-X LIVE STAGING HOT-RELOAD & REBUILD"
echo "=================================================================="

# ------------------------------------------------------------------------------
# 1. Validasi Lingkungan & Hak Akses (Fail-Fast)
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
# 2. Idempotensi & Concurrency Control (File Lock Mutex)
# ------------------------------------------------------------------------------
LOCK_FILE="/var/run/routex-update.lock"
if [ ! -d "/var/run" ] && [ -d "/run" ]; then
    LOCK_FILE="/run/routex-update.lock"
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
    set +e
    echo ""
    echo "🚨 =============================================================="
    echo " ⚠️ MEMULAI PROSEDUR ROLLBACK OTOMATIS (SAFE ROLLBACK)"
    echo "=================================================================="

    echo "ℹ️ CATATAN SKEMA: rollback ini HANYA memulihkan biner. Skema database TIDAK"
    echo "   ikut dipulihkan (versi skema sebelum update: ${SKEMA_SEBELUM:-tidak tercatat};"
    echo "   sesudah migrasi update ini: ${SKEMA_SESUDAH:-tidak tercatat}). Migrasi maju"
    echo "   umumnya tetap bisa dibaca biner lama untuk satu rilis; bila tidak, pulihkan"
    echo "   database dari cadangan — jangan berharap rollback biner memperbaikinya."

    if [ "$BACKUP_CREATED" -eq 1 ] && [ -f "${INSTALL_DIR}/ai-gateway.bak" ]; then
        echo "🔄 [Rollback 1/3] Mengembalikan biner sebelumnya dari ${INSTALL_DIR}/ai-gateway.bak..."
        cp -fp "${INSTALL_DIR}/ai-gateway.bak" "${INSTALL_DIR}/ai-gateway.rollback.tmp"
        chmod 755 "${INSTALL_DIR}/ai-gateway.rollback.tmp"
        mv -f "${INSTALL_DIR}/ai-gateway.rollback.tmp" "${INSTALL_DIR}/ai-gateway"

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
    set +e
    # Bersihkan berkas biner sementara jika tersisa
    rm -f "${INSTALL_DIR}/ai-gateway.new" "${INSTALL_DIR}/ai-gateway.rollback.tmp" 2>/dev/null || true

    # Jalankan rollback jika status memerlukan rollback
    if [ "$ROLLBACK_REQUIRED" -eq 1 ]; then
        rollback_service
    fi

    # Lepaskan lock file dan tutup file descriptor
    flock -u "$LOCK_FD" 2>/dev/null || true
    eval "exec ${LOCK_FD}>&-" 2>/dev/null || true

    exit "$exit_code"
}
trap cleanup EXIT
trap 'exit 130' INT
trap 'exit 143' TERM

# ------------------------------------------------------------------------------
# 3. Validasi Dependensi Wajib Sistem
# ------------------------------------------------------------------------------
REQUIRED_TOOLS=("systemctl" "curl" "go" "git" "flock" "chmod" "cp" "mv" "rm" "grep" "cut" "tr" "date")
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

# Validasi bahwa layanan systemd terdaftar
if ! systemctl cat "$SERVICE_NAME" >/dev/null 2>&1; then
    echo "❌ Error: Layanan systemd '${SERVICE_NAME}.service' tidak ditemukan di sistem." >&2
    echo "   Pastikan unit service telah dipasang di /etc/systemd/system/." >&2
    exit 1
fi

# Baca port dari berkas konfigurasi lingkungan jika tersedia
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

# Berkas state mencatat commit yang terakhir BERHASIL dideploy oleh skrip
# ini. Deteksi perubahan web membandingkan state itu dengan HEAD — versi
# lama memakai reflog HEAD@{1} yang isinya bergantung riwayat lokal repo
# dan bisa menunjuk commit yang salah (audit3 V-DP-14).
STATE_FILE="${INSTALL_DIR}/.routex-last-deployed"
LAST_DEPLOYED=""
if [ -f "$STATE_FILE" ]; then
    LAST_DEPLOYED="$(tr -d '[:space:]' < "$STATE_FILE" 2>/dev/null || true)"
fi

if [ "$BUILD_WEB" = "always" ] || [ ! -d "$SRC_DIR/web/dist" ]; then
    SHOULD_BUILD_WEB=1
elif [ "$BUILD_WEB" = "auto" ] && command -v git &>/dev/null; then
    if [ -z "$LAST_DEPLOYED" ]; then
        # Tanpa state tidak ada dasar perbandingan yang bisa dipercaya:
        # bangun selalu (aman), jangan menebak dari reflog.
        SHOULD_BUILD_WEB=1
    elif git -c safe.directory="$SRC_DIR" -C "$SRC_DIR" diff --name-only "$LAST_DEPLOYED" HEAD 2>/dev/null | grep -q "^web/"; then
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
# safe.directory diberikan PER-PANGGILAN lewat -c, hanya untuk $SRC_DIR
# milik jalur update ini (B-085, pola yang sama dengan perbaikan installer
# B-084): checkout dapat dimiliki pengguna layanan sementara skrip berjalan
# sebagai root — tanpa ini git menolak repo ("detected dubious ownership")
# dan cap versi/deteksi perubahan jatuh diam-diam ke nilai bawaan.
# Konfigurasi global operator tidak disentuh dan direktori lain tidak
# dilemahkan perlindungannya.
GIT_COMMIT="$(git -c safe.directory="$SRC_DIR" -C "$SRC_DIR" rev-parse --short HEAD 2>/dev/null || echo "unknown")"
VERSION="$(git -c safe.directory="$SRC_DIR" -C "$SRC_DIR" describe --tags --always 2>/dev/null || echo "v1.7.0-live")"
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

# Rekam versi skema sebelum & sesudah migrasi supaya pesan rollback jujur
# tentang apa yang tidak dipulihkannya (audit3 V-DP-15). Best-effort: psql
# tidak selalu ada di host native; ketiadaannya tidak menggagalkan update.
versi_skema() {
    if command -v psql >/dev/null 2>&1 && [ -n "${DATABASE_URL:-}" ]; then
        psql "$DATABASE_URL" -tAc "SELECT COALESCE(MAX(version),0) FROM schema_migrations" 2>/dev/null | tr -d '[:space:]' || true
    fi
}
SKEMA_SEBELUM="$(versi_skema)"
"${INSTALL_DIR}/ai-gateway.new" -migrate
SKEMA_SESUDAH="$(versi_skema)"
echo "   Versi skema: ${SKEMA_SEBELUM:-tidak tercatat} -> ${SKEMA_SESUDAH:-tidak tercatat}"

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
    # Catat commit yang terdeploy untuk deteksi perubahan berikutnya.
    git -c safe.directory="$SRC_DIR" -C "$SRC_DIR" rev-parse HEAD > "$STATE_FILE" 2>/dev/null || true

    echo "=================================================================="
    echo " ✅ LIVE UPDATE SUKSES & ONLINE!"
    echo "=================================================================="
    echo " 🔹 Versi Gateway Aktif: ${VERSION} (${GIT_COMMIT})"
    echo " 🔹 Status Layanan     : $(systemctl is-active "$SERVICE_NAME")"
    echo " 🔹 Endpoint Lokal     : http://127.0.0.1:${GATEWAY_PORT}/healthz (200 OK)"
    if command -v caddy >/dev/null 2>&1 && systemctl is-active --quiet caddy 2>/dev/null; then
        echo " 🔹 Edge Proxy         : http://localhost/ (Caddy aktif)"
    fi
    echo " 🔹 Biner Cadangan     : ${INSTALL_DIR}/ai-gateway.bak (tersedia)"
    echo "=================================================================="
else
    echo "=================================================================="
    echo " ❌ PERINGATAN: Layanan belum merespons 200 OK di /healthz setelah ±$((MAX_RETRIES * RETRY_DELAY)) detik."
    echo " Memulai safe rollback otomatis ke versi sebelumnya..."
    echo "=================================================================="
    rollback_service
    exit 1
fi
