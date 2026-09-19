#!/bin/bash
set -e

echo "🚀 Menyiapkan arsitektur Docker Route-X (5 Layanan: Route-X, DB, Redis, Xray, Caddy)..."

# rand_b64 menghasilkan N byte acak terenkode base64 standar.
#
# ENCRYPTION_KEY dan API_KEY_PEPPER wajib dikirimkan sebagai base64: validasi
# config (internal/config/config.go) mendedekode nilainya, dan AES-256-GCM
# mensyaratkan tepat 32 byte. openssl rand -hex 32 menghasilkan 64 karakter
# heksadesimal yang justru didekode menjadi 48 byte, sehingga gateway menolak
# boot. Format ini identik dengan scripts/ops/generate-secrets.sh.
rand_b64() {
    local out
    out="$(openssl rand -base64 "$1" 2>/dev/null)"
    if [ -z "$out" ]; then
        out="$(head -c "$1" /dev/urandom | base64 2>/dev/null)"
    fi
    printf '%s' "$out"
}

# rand_hex menghasilkan 2*N karakter heksadesimal untuk sandi dan token bebas
# format (tidak didekode panjang tetap oleh config).
rand_hex() {
    openssl rand -hex "$1" 2>/dev/null || head -c "$1" /dev/urandom | od -An -tx1 | tr -d ' \n'
}

# detect_host mencari alamat publik utama server: IP keluar rute default adalah
# alamat yang dilihat oleh klien eksternal. Dipakai sebagai host HTTPS default
# PUBLIC_URL; operator dengan domain sendiri menyetel variabel DOMAIN.
detect_host() {
    local host
    host="$(ip -4 route get 1.1.1.1 2>/dev/null | awk '{for (i=1;i<=NF;i++) if ($i=="src") {print $(i+1); exit}}' || true)"
    if [ -z "$host" ]; then
        host="$(hostname -I 2>/dev/null | awk '{print $1}' || true)"
    fi
    if [ -z "$host" ]; then
        host="localhost"
    fi
    printf '%s' "$host"
}

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
    SEC_SESSION="$(rand_b64 48)"
    SEC_ENC="$(rand_b64 32)"
    SEC_PEPPER="$(rand_b64 32)"
    SEC_PGPASS="$(rand_hex 16)"
    SEC_METRICS="$(rand_hex 32)"

    # PUBLIC_URL wajib https saat APP_ENV=production (validasi config menolak
    # boot bila skemanya http). Host diambil dari DOMAIN operator atau alamat
    # publik utama server.
    SEC_PUBLIC_URL="https://${PUBLIC_URL_HOST:-${DOMAIN:-$(detect_host)}}"

    KEY_SESS="SESSION_SECRET"
    KEY_ENC="ENCRYPTION_KEY"
    KEY_PEPPER="API_KEY_PEPPER"
    KEY_PG="POSTGRES_PASSWORD"
    KEY_METRICS="METRICS_TOKEN"
    KEY_PURL="PUBLIC_URL"

    sed -i "s|^${KEY_SESS}=.*|${KEY_SESS}=${SEC_SESSION}|" .env 2>/dev/null || true
    sed -i "s|^${KEY_ENC}=.*|${KEY_ENC}=${SEC_ENC}|" .env 2>/dev/null || true
    sed -i "s|^${KEY_PEPPER}=.*|${KEY_PEPPER}=${SEC_PEPPER}|" .env 2>/dev/null || true
    sed -i "s|^${KEY_PG}=.*|${KEY_PG}=${SEC_PGPASS}|" .env 2>/dev/null || true
    sed -i "s|^${KEY_METRICS}=.*|${KEY_METRICS}=${SEC_METRICS}|" .env 2>/dev/null || true
    sed -i "s|^${KEY_PURL}=.*|${KEY_PURL}=${SEC_PUBLIC_URL}|" .env 2>/dev/null || true
    echo "   File .env berhasil dibuat dengan aman."
fi

# 3. Pastikan konfigurasi default Xray tersedia
# Konfigurasi ini hanya mendengarkan di loopback (127.0.0.1) dan mewajibkan
# autentikasi (accounts) supaya tidak menjadi open proxy. Kata sandi sengaja
# berupa placeholder agar repo bebas secret; ganti "<GANTI_PASSWORD_INI>"
# saat deploy dan cocokkan pada URL egress pool Xray di dashboard admin.
if [ ! -f deploy/xray/config.json ]; then
    echo "📡 Menyiapkan konfigurasi bawaan Xray SOCKS5 (10808) & HTTP (10809)..."
    cat << 'EOF' > deploy/xray/config.json
{
  "log": { "loglevel": "warning" },
  "inbounds": [{
    "tag": "socks-in",
    "port": 10808,
    "listen": "127.0.0.1",
    "protocol": "socks",
    "settings": {
      "auth": "password",
      "accounts": [{ "user": "routex", "pass": "<GANTI_PASSWORD_INI>" }],
      "udp": true
    }
  }, {
    "tag": "http-in",
    "port": 10809,
    "listen": "127.0.0.1",
    "protocol": "http",
    "settings": {
      "accounts": [{ "user": "routex", "pass": "<GANTI_PASSWORD_INI>" }],
      "allowTransparent": false
    }
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
