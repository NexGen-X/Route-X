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

# Menentukan host publik yang melayani Route-X. PUBLIC_URL divalidasi wajib
# https saat APP_ENV=production, dan CSRF dashboard membandingkan origin request
# dengan origin PUBLIC_URL, jadi edge server (Caddy) juga harus melayani HTTPS
# pada host ini. Operator dengan domain menyetel variabel DOMAIN sebelum
# instalasi; tanpa domain, alamat IP keluar rute default server dipakai.
ROUTEX_HOST="${DOMAIN:-${PUBLIC_URL_HOST:-}}"
if [ -z "$ROUTEX_HOST" ]; then
    ROUTEX_HOST="$(ip -4 route get 1.1.1.1 2>/dev/null | awk '{for (i=1;i<=NF;i++) if ($i=="src") {print $(i+1); exit}}' || true)"
fi
if [ -z "$ROUTEX_HOST" ]; then
    ROUTEX_HOST="$(hostname -I 2>/dev/null | awk '{print $1}' || true)"
fi
if [ -z "$ROUTEX_HOST" ]; then
    ROUTEX_HOST="$(hostname -f 2>/dev/null || hostname)"
fi
echo "🌐 Host publik: ${ROUTEX_HOST}"

# Tidak ada CA publik yang menerbitkan sertifikat untuk alamat IP, jadi Caddy
# memakai CA internalnya (self-signed) untuk IP; domain mendapat sertifikat
# Let's Encrypt otomatis.
CADDY_TLS_LINE=""
case "$ROUTEX_HOST" in
    [0-9]*.[0-9]*.[0-9]*.[0-9]*)
        CADDY_TLS_LINE="    tls internal  # host berupa IP: sertifikat internal Caddy (self-signed)"
        ;;
esac

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
# Route-X mengharuskan HTTPS pada host publik (PUBLIC_URL produksi wajib https
# dan CSRF membandingkan origin), jadi Caddy melayani ${ROUTEX_HOST} langsung
# dengan TLS: Let's Encrypt untuk domain, CA internal untuk IP.
if [ ! -f /etc/caddy/Caddyfile ] || grep -q "127.0.0.1:8080" /etc/caddy/Caddyfile 2>/dev/null; then
    # Belum ada Caddyfile, atau berisi blok Route-X versi lama: tulis ulang.
    cat << EOF > /etc/caddy/Caddyfile
# Route-X Gateway — host publik: ${ROUTEX_HOST}
${ROUTEX_HOST} {
${CADDY_TLS_LINE}
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
else
    # Caddyfile operator lain yang belum memproksi Route-X: tambahkan blok ini.
    cat << EOF >> /etc/caddy/Caddyfile

# Route-X Gateway — host publik: ${ROUTEX_HOST}
${ROUTEX_HOST} {
${CADDY_TLS_LINE}
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

# Konfigurasi Xray hanya mendengarkan di loopback (127.0.0.1) dan mewajibkan
# autentikasi, tetapi kata sandinya berupa placeholder agar repo bebas secret.
# Operator WAJIB mengganti "<GANTI_PASSWORD_INI>" pada kedua berkas di atas
# dengan kata sandi acak kuat, lalu mencocokkannya pada URL egress pool Xray di
# dashboard admin (mis. socks5://routex:<sandi>@xray:10808) sebelum jalur
# keluar Xray dipakai. Tanpa langkah ini Xray menolak koneksi (fail-closed).
for cfg in /var/lib/route-x/xray/config.json /usr/local/etc/xray/config.json; do
    if [ -f "$cfg" ] && grep -q "GANTI_PASSWORD_INI" "$cfg"; then
        echo "   ⚠️  $cfg masih memakai kata sandi placeholder <GANTI_PASSWORD_INI>."
        echo "      Ganti dengan kata sandi acak kuat, lalu cocokkan pada egress"
        echo "      pool Xray di dashboard admin sebelum jalur keluar dipakai."
    fi
done

chmod 755 /var/lib/route-x
chmod 755 /var/lib/route-x/xray
chmod 644 /var/lib/route-x/xray/config.json 2>/dev/null || true
grep -q "127.0.0.1 xray" /etc/hosts || echo "127.0.0.1 xray" >> /etc/hosts
systemctl enable xray
systemctl restart xray || true

# 9. Memasang Biner Route-X Core (Menggunakan Biner Rilis Resmi v1.1.0)
echo ""
echo "🛠️ [6/7] Menyiapkan biner resmi Route-X Gateway v1.1.0..."
mkdir -p "$INSTALL_DIR"

INSTALLED_FROM_RELEASE=0
if [ -f "$SRC_DIR/bin/ai-gateway" ]; then
    echo "   Ditemukan biner lokal $SRC_DIR/bin/ai-gateway. Menyalin ke $INSTALL_DIR..."
    install -m 755 "$SRC_DIR/bin/ai-gateway" "$INSTALL_DIR/ai-gateway"
    INSTALLED_FROM_RELEASE=1
else
    RELEASE_TAG="v1.1.0"
    ARCH="$(uname -m)"
    case "$ARCH" in
        x86_64) TARBALL_NAME="routex-${RELEASE_TAG}-linux-amd64.tar.gz" ;;
        *) TARBALL_NAME="routex-${RELEASE_TAG}-linux-amd64.tar.gz" ;;
    esac

    DOWNLOAD_URL="https://github.com/NexGen-X/Route-X/releases/download/${RELEASE_TAG}/${TARBALL_NAME}"
    echo "   Mengunduh biner rilis statis resmi dari GitHub: ${DOWNLOAD_URL}..."
    if curl -fsSL "$DOWNLOAD_URL" -o /tmp/routex-release.tar.gz 2>/dev/null; then
        tar -xzf /tmp/routex-release.tar.gz -C "$INSTALL_DIR" ai-gateway
        chmod 755 "$INSTALL_DIR/ai-gateway"
        rm -f /tmp/routex-release.tar.gz
        echo "   Biner Route-X ${RELEASE_TAG} berhasil dipasang (bebas kompilasi)!"
        INSTALLED_FROM_RELEASE=1
    else
        echo "   Peringatan: Gagal mengunduh biner rilis. Beralih ke fallback kompilasi dari sumber..."
    fi
fi

# Fallback kompilasi dari source code bila pengunduhan biner tidak tersedia
if [ "$INSTALLED_FROM_RELEASE" -eq 0 ]; then
    echo "   Memeriksa toolchain kompilasi Node.js & Golang..."
    if ! command -v node &>/dev/null; then
        curl -fsSL https://deb.nodesource.com/setup_20.x | bash - 2>/dev/null || true
        apt-get install -y nodejs
    fi

    if ! command -v go &>/dev/null; then
        # Versi ini wajib mengikuti go.mod (saat ini 1.27.1); toolchain lebih lama
        # menolak membangun modul.
        GO_VERSION="1.27.1"
        ARCH="$(uname -m)"
        case "$ARCH" in
            x86_64) GO_ARCH="amd64" ;;
            aarch64|arm64) GO_ARCH="arm64" ;;
            *) GO_ARCH="amd64" ;;
        esac
        wget -q "https://dl.google.com/go/go${GO_VERSION}.linux-${GO_ARCH}.tar.gz" -O /tmp/go.tar.gz
        rm -rf /usr/local/go && tar -C /usr/local -xzf /tmp/go.tar.gz
        rm -f /tmp/go.tar.gz
        export PATH="/usr/local/go/bin:$PATH"
    fi

    echo "   Membangun Web Frontend (npm run build)..."
    cd "$SRC_DIR/web"
    npm install --no-audit --no-fund
    npm run build
    cd "$SRC_DIR"

    echo "   Membangun Biner Core Route-X (go build)..."
    CGO_ENABLED=0 GOOS=linux go build -ldflags="-s -w -X main.version=${RELEASE_TAG:-v1.1.0}" -o ai-gateway ./cmd/ai-gateway
    install -m 755 ai-gateway "$INSTALL_DIR/ai-gateway"
fi

# 10. Konfigurasi Lingkungan, Systemd Service & Migrasi Database
echo ""
echo "⚙️ [7/7] Menyiapkan konfigurasi, service systemd & migrasi..."
mkdir -p /etc/routex
if [ ! -f /etc/routex/routex.env ]; then
    # Kunci wajib base64 (config mendedekode nilai dan AES-256-GCM
    # mensyaratkan tepat 32 byte); hex 64 karakter didekode menjadi 48 byte
    # dan gateway menolak boot. Format identik scripts/ops/generate-secrets.sh.
    SESSION_SECRET="$(openssl rand -base64 48 | tr -d '\n')"
    ENCRYPTION_KEY="$(openssl rand -base64 32 | tr -d '\n')"
    API_KEY_PEPPER="$(openssl rand -base64 32 | tr -d '\n')"
    METRICS_TOKEN="$(openssl rand -hex 32 | tr -d '\n')"

    cat << EOF > /etc/routex/routex.env
APP_ENV=production
PORT=8080
PUBLIC_URL=https://${ROUTEX_HOST}
DATABASE_URL=postgres://${DB_USER}:${DB_PASS}@127.0.0.1:5432/${DB_NAME}?sslmode=prefer
REDIS_URL=redis://127.0.0.1:6379/0
SESSION_SECRET="${SESSION_SECRET}"
ENCRYPTION_KEY="${ENCRYPTION_KEY}"
API_KEY_PEPPER="${API_KEY_PEPPER}"
METRICS_TOKEN="${METRICS_TOKEN}"
RATE_LIMIT_FAIL_CLOSED=false
UPSTREAM_ALLOW_HTTP=false
UPSTREAM_ALLOWED_PRIVATE_ADDRS=127.0.0.1,::1,127.0.0.0/8
EOF
    chmod 600 /etc/routex/routex.env
    echo "   Berkas /etc/routex/routex.env berhasil dibuat."
else
    # Migrasi instalasi lama: koreksi nilai yang membuat validasi produksi
    # menolak boot atau mengendurkan postur keamanan.
    if grep -Eq '^PUBLIC_URL=http://' /etc/routex/routex.env; then
        sed -i -E "s|^PUBLIC_URL=.*|PUBLIC_URL=https://${ROUTEX_HOST}|" /etc/routex/routex.env
        echo "   PUBLIC_URL diperbarui ke https (produksi mewajibkan https)."
    fi
    if grep -q "sslmode=disable" /etc/routex/routex.env; then
        sed -i 's|sslmode=disable|sslmode=prefer|g' /etc/routex/routex.env
        echo "   DATABASE_URL: sslmode=disable -> sslmode=prefer."
    fi
    if grep -Eq '^UPSTREAM_ALLOW_HTTP=true' /etc/routex/routex.env; then
        sed -i -E 's|^UPSTREAM_ALLOW_HTTP=true|UPSTREAM_ALLOW_HTTP=false|' /etc/routex/routex.env
        echo "   UPSTREAM_ALLOW_HTTP dinonaktifkan (bawaan aman produksi)."
    fi
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
echo "   Akses Dashboard : https://${ROUTEX_HOST}/login"
echo "   Email Default   : admin@routex.local"
echo "   Password Default: RouteX#Initial2026!"
echo "   (Atau klik tombol 'Gunakan Kredensial Default (1-Klik)' pada login)"
echo "   * Anda akan otomatis diminta membuat password baru saat login."
echo "------------------------------------------------------------------"
echo " ℹ️ CATATAN HTTPS:"
echo "   Produksi mewajibkan HTTPS (PUBLIC_URL=https://${ROUTEX_HOST})."
[ -n "$CADDY_TLS_LINE" ] && \
    echo "   Host berupa IP sehingga Caddy memakai sertifikat internal (self-signed):" && \
    echo "   browser akan memperingatkan sertifikat tak terpercaya — lanjutkan untuk" && \
    echo "   uji coba, atau pasang domain dan jalankan ulang dengan DOMAIN=domainanda."
[ -z "$CADDY_TLS_LINE" ] && \
    echo "   Sertifikat Let's Encrypt diterbitkan otomatis oleh Caddy untuk domain ini."
echo "   Konfigurasi: /etc/routex/routex.env  |  Caddy: /etc/caddy/Caddyfile"
echo "=================================================================="
