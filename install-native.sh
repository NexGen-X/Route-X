#!/bin/bash
set -e

SCRIPT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"

echo "🚀 Memulai Instalasi Route-X secara NATIVE (Biner & Systemd)..."

echo "🧹 Mematikan versi Docker (jika sedang berjalan)..."
docker compose down || true

echo "📦 1/4 Membangun Antarmuka Web (React/Vite)..."
cd "$SCRIPT_DIR/web"
npm install
npm run build
cd "$SCRIPT_DIR"

echo "📡 Memasang dan Mengonfigurasi Xray-core NATIVE..."
if ! command -v xray &> /dev/null; then
    bash -c "$(curl -L https://github.com/XTLS/Xray-install/raw/main/install-release.sh)" @ install
else
    echo "   Xray-core sudah terpasang."
fi

# Konfigurasi Xray Drop-in dan Direktori Konfigurasi
mkdir -p /etc/systemd/system/xray.service.d
cat << 'EOF' > /etc/systemd/system/xray.service.d/10-donot_touch_single_conf.conf
[Service]
ExecStart=
ExecStart=/usr/local/bin/xray run -config /var/lib/route-x/xray/config.json
EOF

mkdir -p /var/lib/route-x/xray
if [ ! -f /var/lib/route-x/xray/config.json ]; then
    cp "$SCRIPT_DIR/deploy/xray/config.json" /var/lib/route-x/xray/config.json
fi
cp "$SCRIPT_DIR/deploy/xray/config.json" /usr/local/etc/xray/config.json 2>/dev/null || true

# Atur hak akses agar user nobody (xray) dan routex dapat membaca
chmod 755 /var/lib/route-x
chmod 755 /var/lib/route-x/xray
chmod 644 /var/lib/route-x/xray/config.json

# Pastikan hostname xray terpetakan ke localhost (kompatibel dengan konfigurasi socks5://xray:10808)
grep -q "127.0.0.1 xray" /etc/hosts || echo "127.0.0.1 xray" >> /etc/hosts

echo "🔨 2/4 Membangun Biner Core (Golang)..."
CGO_ENABLED=0 GOOS=linux go build -ldflags="-s -w" -o ai-gateway ./cmd/ai-gateway

echo "🛑 3/4 Menghentikan layanan sementara..."
systemctl stop routex || true
systemctl stop caddy || true
systemctl stop xray || true

echo "📁 Menyalin ke direktori produksi (/opt/routex)..."
mkdir -p /opt/routex
install -m 755 ai-gateway /opt/routex/ai-gateway

# Pasang unit systemd routex jika tersedia
if [ -f "$SCRIPT_DIR/deploy/systemd/routex.service" ]; then
    cp "$SCRIPT_DIR/deploy/systemd/routex.service" /etc/systemd/system/routex.service
fi

echo "🔄 4/4 Menjalankan Migrasi Skema Database Asli..."
if [ -f /etc/routex/routex.env ]; then
    # Pastikan konfigurasi SSRF mengizinkan proxy lokal (127.0.0.1 / Xray)
    grep -q "UPSTREAM_ALLOWED_PRIVATE_ADDRS" /etc/routex/routex.env || echo "UPSTREAM_ALLOWED_PRIVATE_ADDRS=127.0.0.1,::1,127.0.0.0/8" >> /etc/routex/routex.env

    set -a
    source /etc/routex/routex.env
    set +a
    cd /opt/routex
    ./ai-gateway -migrate
else
    echo "⚠️ /etc/routex/routex.env tidak ditemukan, lewati migrasi otomatis."
fi

echo "🌐 Menyalakan Layanan Systemd (Route-X, Caddy & Xray)..."
systemctl daemon-reload
systemctl enable --now routex caddy xray

echo "✅ Selesai! Route-X kini berjalan murni 100% NATIVE di OS Host Anda."
echo "   - Cek log Route-X: journalctl -fu routex"
echo "   - Cek log Caddy  : journalctl -fu caddy"
echo "   - Cek log Xray   : journalctl -fu xray"
echo "   - Status Layanan :"
systemctl is-active routex caddy xray

echo ""
echo "=================================================================="
echo " 🎉 SETUP AWAL (FIRST-RUN ONBOARDING):"
echo " Kredensial Administrator Bawaan:"
echo "   - Email    : admin@routex.local"
echo "   - Password : RouteX#Initial2026!"
echo "   (Atau klik tombol 'Gunakan Kredensial Default' di halaman Login)"
echo "   * Anda akan langsung diminta memperbarui kata sandi saat pertama login."
echo "=================================================================="

