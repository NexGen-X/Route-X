#!/bin/bash
set -e

echo "🚀 Memulai Instalasi Route-X secara NATIVE (Biner & Systemd)..."

echo "🧹 Mematikan versi Docker (jika sedang berjalan)..."
docker compose down || true

echo "📦 1/4 Membangun Antarmuka Web (React/Vite)..."
cd /root/Route-X/web
npm install
npm run build
cd ..

echo "📡 Memasang Xray-core NATIVE..."
bash -c "$(curl -L https://github.com/XTLS/Xray-install/raw/main/install-release.sh)" @ install

echo "🔨 2/4 Membangun Biner Core (Golang)..."
CGO_ENABLED=0 GOOS=linux go build -o ai-gateway ./cmd/ai-gateway

echo "🛑 3/4 Menghentikan layanan sementara..."
systemctl stop routex || true
systemctl stop caddy || true

echo "📁 Menyalin ke direktori produksi (/opt/routex)..."
mkdir -p /opt/routex
cp ai-gateway /opt/routex/
# Beri izin eksekusi
chmod +x /opt/routex/ai-gateway

echo "🔄 4/4 Menjalankan Migrasi Skema Database Asli..."
# Memuat Environment Host agar migrasi terhubung ke DB asli Host
set -a
source /etc/routex/routex.env
set +a
cd /opt/routex
./ai-gateway -migrate

echo "🌐 Menyalakan Layanan Systemd (Route-X & Caddy)..."
systemctl daemon-reload
systemctl enable --now routex caddy

echo "✅ Selesai! Route-X kini berjalan murni 100% NATIVE di OS Host Anda."
echo "   - Cek log Route-X: journalctl -fu routex"
echo "   - Cek log Caddy  : journalctl -fu caddy"
