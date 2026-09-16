#!/bin/bash
set -e

echo "🚀 Menyiapkan arsitektur Docker Route-X..."

# Pastikan folder konfigurasi tersedia
mkdir -p deploy/caddy deploy/xray

echo "📦 Membangun ulang image Docker (Kompilasi Frontend & Backend)..."
echo "🧹 Membersihkan kontainer lama..."
docker compose down --remove-orphans
docker rm -f routex-postgres routex-gateway routex-caddy routex-redis routex-xray 2>/dev/null || true
docker compose build

echo "🛠️ Menjalankan layanan infrastruktur dasar (PostgreSQL, Redis, Xray)..."
docker compose up -d db redis xray

echo "⏳ Menunggu PostgreSQL siap menerima koneksi (5 detik)..."
sleep 5

echo "🔄 Menjalankan inisialisasi dan migrasi database..."
# Menjalankan container routex sementara hanya untuk flag -migrate
docker compose run --rm routex ./ai-gateway -migrate

echo "🌐 Menyalakan Route-X Core dan Caddy Edge Router..."
docker compose up -d routex caddy

echo ""
echo "✅ DEPLOYMENT BERHASIL!"
echo "Semua 5 layanan kini berjalan secara terisolasi di dalam kontainer Docker."
echo "--------------------------------------------------------"
echo "📊 Cek status layanan : docker compose ps"
echo "📝 Pantau log Route-X : docker compose logs -f routex"
echo "🔐 Pantau log Caddy   : docker compose logs -f caddy"
echo "--------------------------------------------------------"
echo "🎉 SETUP AWAL (FIRST-RUN ONBOARDING):"
echo "   Kredensial Default Login Pertama Kali:"
echo "   - Email    : admin@routex.local"
echo "   - Password : RouteX#Initial2026!"
echo "   (Atau gunakan tombol '1-Klik Autofill' pada halaman Login Web)"
echo "   * Anda akan otomatis diarahkan untuk mengganti kata sandi baru."
echo "--------------------------------------------------------"
