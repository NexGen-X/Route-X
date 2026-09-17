#!/bin/bash
set -e

echo "🚀 Menyiapkan arsitektur Docker Route-X (5 Layanan: Route-X, DB, Redis, Xray, Caddy)..."

# 1. Pastikan folder konfigurasi tersedia
mkdir -p deploy/caddy deploy/xray

# 2. Otomatis inisialisasi file .env bila belum ada
if [ ! -f .env ]; then
    echo "🔑 Membuat file konfigurasi .env baru dengan kunci rahasia unik..."
    if [ -f .env.example ]; then
        cp .env.example .env
    else
        touch .env
    fi

    # Hasilkan kunci kriptografi acak aman
    SEC_SESSION=$(openssl rand -hex 32 2>/dev/null || cat /dev/urandom | tr -dc 'a-f0-9' | fold -w 64 | head -n 1)
    SEC_ENC=$(openssl rand -hex 32 2>/dev/null || cat /dev/urandom | tr -dc 'a-f0-9' | fold -w 64 | head -n 1)
    SEC_PEPPER=$(openssl rand -hex 32 2>/dev/null || cat /dev/urandom | tr -dc 'a-f0-9' | fold -w 64 | head -n 1)
    SEC_PGPASS=$(openssl rand -hex 16 2>/dev/null || cat /dev/urandom | tr -dc 'a-f0-9' | fold -w 32 | head -n 1)
    SEC_METRICS=$(openssl rand -hex 16 2>/dev/null || cat /dev/urandom | tr -dc 'a-f0-9' | fold -w 32 | head -n 1)

    sed -i "s/^SESSION_SECRET=.*/SESSION_SECRET=${SEC_SESSION}/" .env 2>/dev/null || true
    sed -i "s/^ENCRYPTION_KEY=.*/ENCRYPTION_KEY=${SEC_ENC}/" .env 2>/dev/null || true
    sed -i "s/^API_KEY_PEPPER=.*/API_KEY_PEPPER=${SEC_PEPPER}/" .env 2>/dev/null || true
    sed -i "s/^POSTGRES_PASSWORD=.*/POSTGRES_PASSWORD=${SEC_PGPASS}/" .env 2>/dev/null || true
    sed -i "s/^METRICS_TOKEN=.*/METRICS_TOKEN=${SEC_METRICS}/" .env 2>/dev/null || true
    echo "   File .env berhasil dibuat dengan aman."
fi

# 3. Pastikan konfigurasi default Xray tersedia
if [ ! -f deploy/xray/config.json ]; then
    echo "📡 Menyiapkan konfigurasi bawaan Xray SOCKS5 (10808) & HTTP (10809)..."
    cat << 'EOF' > deploy/xray/config.json
{
  "log": { "loglevel": "warning" },
  "inbounds": [{
    "port": 10808,
    "listen": "0.0.0.0",
    "protocol": "socks",
    "settings": { "auth": "noauth", "udp": true }
  }, {
    "port": 10809,
    "listen": "0.0.0.0",
    "protocol": "http",
    "settings": {}
  }],
  "outbounds": [{
    "protocol": "freedom",
    "settings": {}
  }]
}
EOF
fi

echo "📦 Membangun image Docker (Kompilasi Frontend & Backend)..."
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
