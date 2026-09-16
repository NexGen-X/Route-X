#!/usr/bin/env bash
# ==============================================================================
# Route-X Automated Native Linux Installer (All-in-One)
# Memasang dan mengonfigurasi otomatis:
# 1. PostgreSQL (Database primer & auto-setup database/role)
# 2. Redis Server (Distributed cache & rate limiter)
# 3. Caddy Web Server (Edge reverse proxy & auto TLS)
# 4. Xray-core (Egress routing & upstream proxying)
# 5. Route-X Core & Web UI (Kompilasi biner native + Systemd service)
# ==============================================================================
set -euo pipefail
export DEBIAN_FRONTEND=noninteractive
export PATH="/usr/local/go/bin:/usr/local/bin:$PATH"

echo "=================================================================="
echo " 🚀 ROUTE-X AUTOMATED NATIVE LINUX INSTALLER"
echo "=================================================================="

# 1. Pastikan dijalankan sebagai root / sudo
if [ "$(id -u)" -ne 0 ]; then
    echo "❌ Error: Skrip ini wajib dijalankan sebagai root (atau gunakan sudo)." >&2
    exit 1
fi

# 2. Pastikan sistem berbasis Debian / Ubuntu
if ! command -v apt-get &>/dev/null; then
    echo "❌ Error: Skrip instalasi native otomatis ini dirancang untuk Debian / Ubuntu." >&2
    exit 1
fi

# 3. Menentukan lokasi kode sumber (lokal atau clone via curl pipe)
REPO_URL="https://github.com/NexGen-X/Route-X.git"
INSTALL_DIR="/opt/routex"
SRC_DIR=""

if [ -f "./go.mod" ] && grep -q "github.com/NexGen-X/Route-X" "./go.mod" 2>/dev/null; then
    SRC_DIR="$(pwd)"
elif [ -f "$(dirname "$0")/go.mod" ] && grep -q "github.com/NexGen-X/Route-X" "$(dirname "$0")/go.mod" 2>/dev/null; then
    SRC_DIR="$(cd "$(dirname "$0")" && pwd)"
else
    echo "📥 Mendeteksi eksekusi via pipe/curl. Mengunduh source code Route-X ke /opt/routex/src..."
    mkdir -p /opt/routex/src
    if [ -d "/opt/routex/src/.git" ]; then
        git -C /opt/routex/src fetch origin main
        git -C /opt/routex/src reset --hard origin/main
    else
        apt-get update -y && apt-get install -y git
        git clone --depth 1 "$REPO_URL" /opt/routex/src
    fi
    SRC_DIR="/opt/routex/src"
fi

echo "📁 Direktori Sumber: $SRC_DIR"

# 4. Memperbarui paket sistem dan utilitas dasar
echo ""
echo "📦 [1/7] Memperbarui sistem dan memasang paket inti..."
apt-get update -y
apt-get install -y curl wget git openssl ca-certificates gnupg lsb-release build-essential

# 5. Memasang dan mengonfigurasi PostgreSQL
echo ""
echo "🐘 [2/7] Memeriksa dan memasang PostgreSQL..."
if ! command -v psql &>/dev/null; then
    apt-get install -y postgresql postgresql-contrib
fi
systemctl enable postgresql
systemctl start postgresql

DB_USER="routex"
DB_NAME="routex_prod"
DB_PASS=""

if [ -f /etc/routex/routex.env ]; then
    DB_PASS=$(grep -E '^DATABASE_URL=' /etc/routex/routex.env | sed -E 's/.*:\/\/routex:([^@]+)@.*/\1/' || true)
fi
if [ -z "$DB_PASS" ]; then
    DB_PASS=$(openssl rand -hex 16)
fi

# Inisialisasi role dan database bila belum ada
if ! sudo -u postgres psql -tAc "SELECT 1 FROM pg_roles WHERE rolname='${DB_USER}'" | grep -q 1; then
    sudo -u postgres psql -c "CREATE USER ${DB_USER} WITH PASSWORD '${DB_PASS}';"
    echo "   User database '${DB_USER}' berhasil dibuat."
fi
if ! sudo -u postgres psql -tAc "SELECT 1 FROM pg_database WHERE datname='${DB_NAME}'" | grep -q 1; then
    sudo -u postgres psql -c "CREATE DATABASE ${DB_NAME} OWNER ${DB_USER};"
    echo "   Database '${DB_NAME}' berhasil dibuat."
fi
sudo -u postgres psql -c "GRANT ALL PRIVILEGES ON DATABASE ${DB_NAME} TO ${DB_USER};" 2>/dev/null || true

# 6. Memasang dan mengonfigurasi Redis
echo ""
echo "⚡ [3/7] Memeriksa dan memasang Redis Server..."
if ! command -v redis-server &>/dev/null; then
    apt-get install -y redis-server
fi
systemctl enable redis-server
systemctl start redis-server

# 7. Memasang dan mengonfigurasi Caddy Web Server
echo ""
echo "🔒 [4/7] Memeriksa dan memasang Caddy Web Server..."
if ! command -v caddy &>/dev/null; then
    apt-get install -y debian-keyring debian-archive-keyring apt-transport-https
    curl -1sLf 'https://dl.cloudsmith.io/public/caddy/stable/gpg.key' | gpg --dearmor -o /usr/share/keyrings/caddy-stable-archive-keyring.gpg --yes 2>/dev/null || true
    curl -1sLf 'https://dl.cloudsmith.io/public/caddy/stable/debian.deb.txt' | tee /etc/apt/sources.list.d/caddy-stable.list >/dev/null
    apt-get update -y
    apt-get install -y caddy
fi

mkdir -p /etc/caddy
if [ ! -f /etc/caddy/Caddyfile ]; then
    cat << 'EOF' > /etc/caddy/Caddyfile
:80 {
    encode zstd gzip

    reverse_proxy 127.0.0.1:8080 {
        header_up X-Real-Ip {remote_host}

        transport http {
            keepalive 300s
            keepalive_idle_conns 250
        }
    }
}
EOF
elif ! grep -q ":80" /etc/caddy/Caddyfile 2>/dev/null; then
    cat << 'EOF' >> /etc/caddy/Caddyfile

:80 {
    encode zstd gzip

    reverse_proxy 127.0.0.1:8080 {
        header_up X-Real-Ip {remote_host}

        transport http {
            keepalive 300s
            keepalive_idle_conns 250
        }
    }
}
EOF
fi
systemctl enable caddy
systemctl restart caddy || true

# 8. Memasang dan mengonfigurasi Xray-core
echo ""
echo "📡 [5/7] Memeriksa dan memasang Xray-core..."
if ! command -v xray &>/dev/null; then
    bash -c "$(curl -L https://github.com/XTLS/Xray-install/raw/main/install-release.sh)" @ install
fi

mkdir -p /etc/systemd/system/xray.service.d
cat << 'EOF' > /etc/systemd/system/xray.service.d/10-donot_touch_single_conf.conf
[Service]
ExecStart=
ExecStart=/usr/local/bin/xray run -config /var/lib/route-x/xray/config.json
EOF

mkdir -p /var/lib/route-x/xray
if [ ! -f /var/lib/route-x/xray/config.json ] && [ -f "$SRC_DIR/deploy/xray/config.json" ]; then
    cp "$SRC_DIR/deploy/xray/config.json" /var/lib/route-x/xray/config.json
fi
if [ -f "$SRC_DIR/deploy/xray/config.json" ]; then
    cp "$SRC_DIR/deploy/xray/config.json" /usr/local/etc/xray/config.json 2>/dev/null || true
fi

chmod 755 /var/lib/route-x
chmod 755 /var/lib/route-x/xray
chmod 644 /var/lib/route-x/xray/config.json 2>/dev/null || true
grep -q "127.0.0.1 xray" /etc/hosts || echo "127.0.0.1 xray" >> /etc/hosts
systemctl enable xray
systemctl restart xray || true

# 9. Memeriksa Lingkungan Kompilasi (Node.js & Golang)
echo ""
echo "🛠️ [6/7] Memeriksa build toolchain (Node.js & Golang)..."
if ! command -v node &>/dev/null; then
    echo "   Memasang Node.js 20 LTS (NodeSource)..."
    curl -fsSL https://deb.nodesource.com/setup_20.x | bash -
    apt-get install -y nodejs
fi

GO_VER=0
if command -v go &>/dev/null; then
    GO_VER="$(go version 2>/dev/null | grep -oE 'go1\.[0-9]+' | cut -d. -f2 || echo 0)"
fi

if [ "$GO_VER" -lt 27 ]; then
    echo "   Golang >= 1.27 diperlukan. Mengunduh Golang 1.27.1..."
    ARCH="$(uname -m)"
    case "$ARCH" in
        x86_64) GO_ARCH="amd64" ;;
        aarch64|arm64) GO_ARCH="arm64" ;;
        *) GO_ARCH="amd64" ;;
    esac
    wget -q "https://dl.google.com/go/go1.27.1.linux-${GO_ARCH}.tar.gz" -O /tmp/go.tar.gz
    rm -rf /usr/local/go && tar -C /usr/local -xzf /tmp/go.tar.gz
    rm -f /tmp/go.tar.gz
    export PATH="/usr/local/go/bin:$PATH"
    grep -q "/usr/local/go/bin" /etc/profile || echo 'export PATH="/usr/local/go/bin:$PATH"' >> /etc/profile
fi

# Kompilasi Web Frontend
echo "   Membangun Antarmuka Web (React/Vite)..."
cd "$SRC_DIR/web"
npm install --no-audit --no-fund
npm run build
cd "$SRC_DIR"

# Kompilasi Backend Gateway
echo "   Membangun Biner Core Route-X (Golang)..."
CGO_ENABLED=0 GOOS=linux go build -ldflags="-s -w" -o ai-gateway ./cmd/ai-gateway

mkdir -p "$INSTALL_DIR"
install -m 755 ai-gateway "$INSTALL_DIR/ai-gateway"

# 10. Konfigurasi Lingkungan, Systemd Service & Migrasi Database
echo ""
echo "⚙️ [7/7] Menyiapkan konfigurasi, service systemd & migrasi..."
mkdir -p /etc/routex
if [ ! -f /etc/routex/routex.env ]; then
    SESSION_SECRET="$(openssl rand -hex 32)"
    ENCRYPTION_KEY="$(openssl rand -base64 32)"
    API_KEY_PEPPER="$(openssl rand -hex 32)"
    METRICS_TOKEN="$(openssl rand -hex 32)"

    cat << EOF > /etc/routex/routex.env
APP_ENV=production
PORT=8080
PUBLIC_URL=http://127.0.0.1:8080
DATABASE_URL=postgres://${DB_USER}:${DB_PASS}@127.0.0.1:5432/${DB_NAME}?sslmode=disable
REDIS_URL=redis://127.0.0.1:6379/0
SESSION_SECRET=${SESSION_SECRET}
ENCRYPTION_KEY=${ENCRYPTION_KEY}
API_KEY_PEPPER=${API_KEY_PEPPER}
METRICS_TOKEN=${METRICS_TOKEN}
RATE_LIMIT_FAIL_CLOSED=false
UPSTREAM_ALLOW_HTTP=true
UPSTREAM_ALLOWED_PRIVATE_ADDRS=127.0.0.1,::1,127.0.0.0/8
EOF
    chmod 600 /etc/routex/routex.env
    echo "   Berkas /etc/routex/routex.env berhasil dibuat."
else
    grep -q "UPSTREAM_ALLOWED_PRIVATE_ADDRS" /etc/routex/routex.env || \
        echo "UPSTREAM_ALLOWED_PRIVATE_ADDRS=127.0.0.1,::1,127.0.0.0/8" >> /etc/routex/routex.env
fi

# Pasang routex.service
if [ -f "$SRC_DIR/deploy/systemd/routex.service" ]; then
    cp "$SRC_DIR/deploy/systemd/routex.service" /etc/systemd/system/routex.service
fi

# Jalankan migrasi database
echo "   Menjalankan migrasi skema database (0001 - 0010)..."
set -a
# shellcheck disable=SC1091
source /etc/routex/routex.env
set +a
cd "$INSTALL_DIR"
./ai-gateway -migrate

# Aktifkan dan jalankan routex.service
systemctl daemon-reload
systemctl enable routex
systemctl restart routex

echo ""
echo "=================================================================="
echo " ✅ INSTALASI NATIVE ROUTE-X SELESAI & BERHASIL!"
echo "=================================================================="
echo " 📊 Status Layanan Native:"
echo "   - Route-X Core : $(systemctl is-active routex || true)"
echo "   - Caddy Edge   : $(systemctl is-active caddy || true)"
echo "   - Xray Proxy   : $(systemctl is-active xray || true)"
echo "   - PostgreSQL   : $(systemctl is-active postgresql || true)"
echo "   - Redis Server : $(systemctl is-active redis-server || true)"
echo "------------------------------------------------------------------"
echo " 📝 Monitoring Log:"
echo "   journalctl -fu routex"
echo "   journalctl -fu caddy"
echo "   journalctl -fu xray"
echo "------------------------------------------------------------------"
echo " 🎉 SETUP AWAL (FIRST-RUN ONBOARDING):"
echo "   Akses Dashboard : http://<IP-SERVER-ANDA>/login atau :8080/login"
echo "   Email Default   : admin@routex.local"
echo "   Password Default: RouteX#Initial2026!"
echo "   (Atau klik tombol 'Gunakan Kredensial Default (1-Klik)' pada login)"
echo "   * Anda akan otomatis diminta membuat password baru saat login."
echo "=================================================================="
