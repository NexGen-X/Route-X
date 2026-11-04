#!/usr/bin/env bash
# ==============================================================================
# Route-X Automated Native Linux Installer (All-in-One)
# Memasang dan mengonfigurasi otomatis:
# 1. PostgreSQL (Database primer & auto-setup database/role)
# 2. Redis Server (Distributed cache & rate limiter)
# 3. Caddy Web Server (Edge reverse proxy & auto TLS)
# 4. Route-X Core & Web UI (Biner rilis resmi dari GitHub Releases +
#    verifikasi checksum .sha256; kompilasi dari sumber hanya fallback
#    bila unduhan tidak tersedia, dengan cap versi dev-<sha>)
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

# 3. Menentukan lokasi kode sumber: checkout repo dipakai langsung; di luar
# checkout, source di-clone segar ke /opt/routex/src — baik skrip dijalankan
# dari berkas lokal maupun via pipe/curl, hanya pesannya yang dibedakan.
REPO_URL="https://github.com/NexGen-X/Route-X.git"
INSTALL_DIR="/opt/routex"
SRC_DIR=""

if [ -f "./go.mod" ] && grep -q "github.com/NexGen-X/Route-X" "./go.mod" 2>/dev/null; then
    SRC_DIR="$(pwd)"
elif [ -f "$(dirname "$0")/go.mod" ] && grep -q "github.com/NexGen-X/Route-X" "$(dirname "$0")/go.mod" 2>/dev/null; then
    SRC_DIR="$(cd "$(dirname "$0")" && pwd)"
else
    # Bedakan dua keadaan di luar checkout: berkas lokal di luar repo ($0
    # adalah berkas skrip yang nyata) vs pipe/curl ($0 hanya nama shell,
    # bukan berkas). Versi lama melaporkan keduanya sebagai pipe/curl,
    # menyesatkan operator yang menjalankan berkas lokal (B-082).
    if [ -f "$0" ]; then
        echo "📥 Skrip dijalankan dari berkas lokal di luar checkout repo. Mengunduh source code Route-X ke /opt/routex/src..."
    else
        echo "📥 Mendeteksi eksekusi via pipe/curl. Mengunduh source code Route-X ke /opt/routex/src..."
    fi
    mkdir -p /opt/routex/src
    if [ -d "/opt/routex/src/.git" ]; then
        # Instalasi ulang (B-084): pada lari berikutnya /opt/routex/src
        # dimiliki pengguna layanan 'routex' (tahap konfigurasi di bawah
        # me-chown -R $INSTALL_DIR), sedangkan installer berjalan sebagai
        # root — git menolak membuka repo milik pengguna lain ("detected
        # dubious ownership") dan jalur upgrade macet total di tahap ini.
        # safe.directory diberikan PER-PANGGILAN lewat -c, hanya untuk src
        # installer sendiri: konfigurasi global operator tidak disentuh dan
        # perlindungan kepemilikan untuk direktori lain tidak dilemahkan.
        git -c safe.directory=/opt/routex/src -C /opt/routex/src fetch origin main
        git -c safe.directory=/opt/routex/src -C /opt/routex/src reset --hard origin/main
    else
        # Clone membuat repo BARU (bukan membuka repo yang sudah ada), jadi
        # pemeriksaan kepemilikan tidak berlaku di sini.
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

# Peringatan jebakan domain belakangan (B-082): instalasi tanpa DOMAIN
# melayani sertifikat internal untuk alamat IP. Domain yang dipasang
# SESUDAHNYA (apalagi diproksi Cloudflare) menunjuk ke instalasi ini, tetapi
# Caddy tetap menjawab dengan sertifikat internal itu sehingga domain gagal
# — gejala khasnya error 525 di Cloudflare. Jebakan ini sudah terbukti
# memakan korban nyata, jadi peringatkan sejak awal dan di ringkasan akhir.
if [ -n "$CADDY_TLS_LINE" ]; then
    echo ""
    echo "⚠️ PERINGATAN: Instalasi berjalan TANPA domain (DOMAIN tidak disetel) —"
    echo "   Route-X dilayani lewat alamat IP dengan sertifikat internal Caddy."
    echo "   Bila nanti Anda memasang domain (apalagi diproksi Cloudflare), WAJIB"
    echo "   jalankan ulang installer ini dengan DOMAIN=<domain-anda>: tanpa itu"
    echo "   Caddy tetap melayani sertifikat internal untuk IP dan domain akan"
    echo "   gagal (gejala khas di Cloudflare: error 525)."
fi

# 4. Memperbarui paket sistem dan utilitas dasar
echo ""
echo "📦 [1/6] Memperbarui sistem dan memasang paket inti..."
apt-get update -y
apt-get install -y curl wget git openssl ca-certificates gnupg lsb-release build-essential

# 5. Memasang dan mengonfigurasi PostgreSQL
echo ""
echo "🐘 [2/6] Memeriksa dan memasang PostgreSQL..."
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

# Inisialisasi role dan database bila belum ada.
# CREATEDB (B-104): restore dari panel admin memakai mekanisme skratch-swap
# (B-045) — gateway sebagai role ini harus bisa CREATE DATABASE skratch,
# ALTER DATABASE ... RENAME untuk penukaran, dan DROP DATABASE salinan
# pengaman lama, semuanya atas database MILIKNYA SENDIRI. Tanpa CREATEDB,
# restore selalu gagal 42501 pada instalasi native standar sementara ekspor
# bekerja, jadi fitur pemulihan mati tepat saat paling dibutuhkan. Hak ini
# SENGAJA sempit: hanya CREATEDB pada role aplikasi ini saja — BUKAN
# SUPERUSER, BUKAN CREATEROLE — sehingga role tidak bisa membuat role lain,
# menaikkan haknya, atau menyentuh database milik role lain di luar hak
# pemilik database-nya sendiri (rincian trade-off: PR B-104 dan
# docs/DISASTER_RECOVERY.md §3). Jalur compose tidak lewat sini: di sana
# POSTGRES_USER adalah superuser bawaan image PostgreSQL.
if ! sudo -u postgres psql -tAc "SELECT 1 FROM pg_roles WHERE rolname='${DB_USER}'" | grep -q 1; then
    sudo -u postgres psql -c "CREATE USER ${DB_USER} WITH CREATEDB PASSWORD '${DB_PASS}';"
    echo "   User database '${DB_USER}' berhasil dibuat."
fi
# Selalu selaraskan password role dengan yang tercatat di routex.env
# (audit3 V-DP-06): versi lama hanya menyetel password saat CREATE, jadi
# instalasi ulang sesudah rotasi manual membuat role dan env berbeda dan
# gateway gagal login selamanya. ALTER yang sama kini juga selalu
# menyelaraskan CREATEDB (B-104): menjalankan ulang installer (jalur
# upgrade) menyembuhkan instalasi lama yang role-nya lahir tanpa CREATEDB,
# pola yang sama dengan penyembuhan home pengguna layanan (B-095).
sudo -u postgres psql -c "ALTER USER ${DB_USER} WITH CREATEDB PASSWORD '${DB_PASS}';" >/dev/null
if ! sudo -u postgres psql -tAc "SELECT 1 FROM pg_database WHERE datname='${DB_NAME}'" | grep -q 1; then
    sudo -u postgres psql -c "CREATE DATABASE ${DB_NAME} OWNER ${DB_USER};"
    echo "   Database '${DB_NAME}' berhasil dibuat."
fi
sudo -u postgres psql -c "GRANT ALL PRIVILEGES ON DATABASE ${DB_NAME} TO ${DB_USER};" 2>/dev/null || true

# 6. Memasang dan mengonfigurasi Redis
echo ""
echo "⚡ [3/6] Memeriksa dan memasang Redis Server..."
if ! command -v redis-server &>/dev/null; then
    apt-get install -y redis-server
fi
systemctl enable redis-server
systemctl start redis-server

# 7. Memasang dan mengonfigurasi Caddy Web Server
echo ""
echo "🔒 [4/6] Memeriksa dan memasang Caddy Web Server..."
if ! command -v caddy &>/dev/null; then
    apt-get install -y debian-keyring debian-archive-keyring apt-transport-https
    # Verifikasi GPG WAJIB (audit3 V-DP-07): versi lama menandai repo
    # [trusted=yes], membungkam kegagalan gpg, dan memasang paket dengan
    # --allow-unauthenticated — rantai pasok tanpa verifikasi apa pun.
    # Kegagalan di sini menghentikan instalasi, bukan ditelan.
    curl -1sLf 'https://dl.cloudsmith.io/public/caddy/stable/gpg.key' | gpg --dearmor -o /usr/share/keyrings/caddy-stable-archive-keyring.gpg --yes
    echo "deb [signed-by=/usr/share/keyrings/caddy-stable-archive-keyring.gpg] https://dl.cloudsmith.io/public/caddy/stable/deb/debian any-version main" > /etc/apt/sources.list.d/caddy-stable.list
    apt-get update -y
    apt-get install -y caddy
fi

mkdir -p /etc/caddy
# Route-X mengharuskan HTTPS pada host publik (PUBLIC_URL produksi wajib https
# dan CSRF membandingkan origin), jadi Caddy melayani ${ROUTEX_HOST} langsung
# dengan TLS: Let's Encrypt untuk domain, CA internal untuk IP.
if [ -f /etc/caddy/Caddyfile ] && grep -q "127.0.0.1:8080" /etc/caddy/Caddyfile 2>/dev/null; then
    # Caddyfile yang ada bisa memuat penyesuaian operator: JANGAN timpa
    # senyap (audit3 V-DP-04). Cadangkan dulu dengan cap waktu.
    SALINAN_CADDY="/etc/caddy/Caddyfile.bak-$(date +%Y%m%d%H%M%S)"
    cp /etc/caddy/Caddyfile "$SALINAN_CADDY"
    echo "   ⚠️ Caddyfile lama dicadangkan ke $SALINAN_CADDY sebelum ditulis ulang."
fi
if [ ! -f /etc/caddy/Caddyfile ] || grep -q "127.0.0.1:8080" /etc/caddy/Caddyfile 2>/dev/null; then
    # Belum ada Caddyfile, atau berisi blok Route-X versi lama: tulis ulang.
    # Penyelarasan KOSMETIK ke templat compose (deploy/caddy/Caddyfile),
    # rider B-079: keepalive 90s, dial_timeout 5s, encode minimum_length 512,
    # dan HSTS ber-token preload pada blok HTTPS. Ini BUKAN perbaikan
    # buffering — uji A/B B-079 membuktikan penerusan SSE identik pada kedua
    # bentuk, jadi flush_interval sengaja TIDAK ditambahkan di sini.
    cat << EOF > /etc/caddy/Caddyfile
{
    auto_https disable_redirects
}

http://${ROUTEX_HOST}, :80 {
    encode zstd gzip {
        minimum_length 512
    }

    handle {
        reverse_proxy 127.0.0.1:8080 {
            header_up X-Real-Ip {remote_host}
            transport http {
                dial_timeout 5s
                keepalive 90s
                keepalive_idle_conns 250
            }
        }
    }
}

# Route-X Gateway — host publik: ${ROUTEX_HOST}
${ROUTEX_HOST} {
${CADDY_TLS_LINE}
    encode zstd gzip {
        minimum_length 512
    }

    header {
        Strict-Transport-Security "max-age=31536000; includeSubDomains; preload"
    }

    handle {
        reverse_proxy 127.0.0.1:8080 {
            header_up X-Real-Ip {remote_host}
            transport http {
                dial_timeout 5s
                keepalive 90s
                keepalive_idle_conns 250
            }
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
    encode zstd gzip {
        minimum_length 512
    }

    header {
        Strict-Transport-Security "max-age=31536000; includeSubDomains; preload"
    }

    reverse_proxy 127.0.0.1:8080 {
        header_up X-Real-Ip {remote_host}

        transport http {
            dial_timeout 5s
            keepalive 90s
            keepalive_idle_conns 250
        }
    }
}
EOF
fi
systemctl enable caddy
systemctl restart caddy || true

# 8. Memasang Biner Route-X Core dari rilis resmi.
# RELEASE_TAG adalah satu-satunya tempat versi rilis disebut di installer
# ini — perbarui di sini saat menerbitkan rilis baru.
RELEASE_TAG="v1.7.0"
echo ""
echo "🛠️ [5/6] Menyiapkan biner resmi Route-X Gateway ${RELEASE_TAG}..."
mkdir -p "$INSTALL_DIR"

INSTALLED_FROM_RELEASE=0
if [ -f "$SRC_DIR/bin/ai-gateway" ]; then
    echo "   Ditemukan biner lokal $SRC_DIR/bin/ai-gateway. Menyalin ke $INSTALL_DIR..."
    install -m 755 "$SRC_DIR/bin/ai-gateway" "$INSTALL_DIR/ai-gateway"
    INSTALLED_FROM_RELEASE=1
else
    # Arsitektur dipilih dari uname -m (audit3 V-DP-01): versi lama selalu
    # mengambil tarball amd64 — di mesin ARM64 unduhan "berhasil" tetapi
    # binernya tidak bisa dieksekusi sama sekali. Arsitektur tak dikenal
    # tidak ditebak: lewati unduhan dan pakai fallback kompilasi sumber.
    ARCH="$(uname -m)"
    TARBALL_NAME=""
    case "$ARCH" in
        x86_64|amd64) TARBALL_NAME="routex-${RELEASE_TAG}-linux-amd64.tar.gz" ;;
        aarch64|arm64) TARBALL_NAME="routex-${RELEASE_TAG}-linux-arm64.tar.gz" ;;
        *) echo "   ⚠️ Arsitektur '${ARCH}' tidak dikenal; melewati unduhan biner rilis." ;;
    esac

    if [ -n "$TARBALL_NAME" ]; then
        DOWNLOAD_URL="https://github.com/NexGen-X/Route-X/releases/download/${RELEASE_TAG}/${TARBALL_NAME}"
        echo "   Mengunduh biner rilis statis resmi dari GitHub: ${DOWNLOAD_URL}..."
        # Tarball disimpan dengan NAMA ASLINYA: berkas .sha256 di sisi
        # rilis merekam nama itu, dan sha256sum -c mencocokkan berdasarkannya.
        if curl -fsSL "$DOWNLOAD_URL" -o "/tmp/${TARBALL_NAME}" 2>/dev/null; then
            # Verifikasi checksum dari berkas .sha256 DI SISI RILIS yang
            # sama (audit3 V-DP-02) — bukan dari tarball yang kebetulan ada
            # di repo lokal. Checksum tersedia tetapi TIDAK COCOK = batalkan
            # instalasi; checksum belum diterbitkan rilis lama = peringatan
            # keras, instalasi lanjut dengan mata terbuka.
            if curl -fsSL "${DOWNLOAD_URL}.sha256" -o "/tmp/${TARBALL_NAME}.sha256" 2>/dev/null; then
                if ! (cd /tmp && sha256sum -c "${TARBALL_NAME}.sha256" >/dev/null 2>&1); then
                    echo "❌ Checksum tarball rilis TIDAK COCOK — unduhan dibuang, instalasi dibatalkan." >&2
                    rm -f "/tmp/${TARBALL_NAME}" "/tmp/${TARBALL_NAME}.sha256"
                    exit 1
                fi
                echo "   Checksum SHA-256 tarball terverifikasi dari sisi rilis."
            else
                echo "   ⚠️ PERINGATAN KERAS: rilis ${RELEASE_TAG} tidak menerbitkan berkas .sha256;"
                echo "      integritas tarball TIDAK terverifikasi. Lanjutkan hanya bila Anda"
                echo "      memercayai jalur unduhan ini."
            fi
            rm -f "/tmp/${TARBALL_NAME}.sha256"
            tar -xzf "/tmp/${TARBALL_NAME}" -C "$INSTALL_DIR" ai-gateway
            chmod 755 "$INSTALL_DIR/ai-gateway"
            rm -f "/tmp/${TARBALL_NAME}"
            echo "   Biner Route-X ${RELEASE_TAG} (${ARCH}) berhasil dipasang (bebas kompilasi)!"
            INSTALLED_FROM_RELEASE=1
        else
            echo "   Peringatan: Gagal mengunduh biner rilis. Beralih ke fallback kompilasi dari sumber..."
        fi
    fi
fi

# Fallback kompilasi dari source code bila pengunduhan biner tidak tersedia
if [ "$INSTALLED_FROM_RELEASE" -eq 0 ]; then
    echo "   Memeriksa toolchain kompilasi Node.js & Golang..."
    if ! command -v node &>/dev/null; then
        curl -fsSL https://deb.nodesource.com/setup_24.x | bash - 2>/dev/null || true
        apt-get install -y nodejs
    fi

    if ! command -v go &>/dev/null; then
        # Versi ini wajib mengikuti go.mod (saat ini 1.27.3); toolchain lebih lama
        # menolak membangun modul.
        GO_VERSION="1.27.3"
        ARCH="$(uname -m)"
        case "$ARCH" in
            x86_64) GO_ARCH="amd64" ;;
            aarch64|arm64) GO_ARCH="arm64" ;;
            *) echo "❌ Arsitektur '${ARCH}' tidak didukung untuk fallback kompilasi." >&2; exit 1 ;;
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
    # Cap versi build sumber adalah dev-<sha pendek>: mencap ${RELEASE_TAG}
    # pada biner hasil kompilasi lokal adalah klaim palsu (audit3 V-DP-03).
    # safe.directory per-panggilan juga di sini (B-084): $SRC_DIR dapat berupa
    # src instalasi ulang di atas atau checkout milik pengguna lain sementara
    # installer berjalan sebagai root — tanpa ini cap versi jatuh diam-diam
    # ke "dev-unknown".
    CAP_VERSI="dev-$(git -c safe.directory="$SRC_DIR" -C "$SRC_DIR" rev-parse --short HEAD 2>/dev/null || echo unknown)"
    CGO_ENABLED=0 GOOS=linux go build -ldflags="-s -w -X main.version=${CAP_VERSI}" -o ai-gateway ./cmd/ai-gateway
    install -m 755 ai-gateway "$INSTALL_DIR/ai-gateway"
fi

# 9. Konfigurasi Lingkungan, Systemd Service & Migrasi Database
echo ""
echo "⚙️ [6/6] Menyiapkan konfigurasi, service systemd & migrasi..."
mkdir -p /etc/routex
if [ ! -f /etc/routex/routex.env ]; then
    # Kunci wajib base64 (config mendedekode nilai dan AES-256-GCM
    # mensyaratkan tepat 32 byte); hex 64 karakter didekode menjadi 48 byte
    # dan gateway menolak boot. Format identik scripts/ops/generate-secrets.sh.
    SESSION_SECRET="$(openssl rand -base64 48 | tr -d '\n')"
    ENCRYPTION_KEY="$(openssl rand -base64 32 | tr -d '\n')"
    API_KEY_PEPPER="$(openssl rand -base64 32 | tr -d '\n')"
    METRICS_TOKEN="$(openssl rand -hex 32 | tr -d '\n')"
    ADMIN_PASS="$(openssl rand -base64 18 | tr -d '\n')"

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
TRUSTED_PROXIES=127.0.0.1,::1
ANTIGRAVITY_CLIENT_SECRET=
INITIAL_ADMIN_EMAIL=admin@routex.local
INITIAL_ADMIN_PASSWORD="${ADMIN_PASS}"
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
    # TRUSTED_PROXIES tidak pernah disetel installer native versi lama
    # (audit3 V-DP-20): Caddy di host yang sama adalah proxy satu-satunya,
    # jadi loopback adalah nilai yang tepat dan sempit.
    grep -q "^TRUSTED_PROXIES=" /etc/routex/routex.env || \
        echo "TRUSTED_PROXIES=127.0.0.1,::1" >> /etc/routex/routex.env
    grep -q "^ANTIGRAVITY_CLIENT_SECRET=" /etc/routex/routex.env || \
        echo "ANTIGRAVITY_CLIENT_SECRET=" >> /etc/routex/routex.env
    # PUBLIC_URL https dengan host LAMA (audit3 V-DP-05): hanya diperbarui
    # bila operator menjalankan installer ini dengan DOMAIN/PUBLIC_URL_HOST
    # eksplisit — tanpa itu perbedaan host bisa jadi domain sah pilihan
    # operator dan hanya diperingatkan, tidak ditimpa.
    if grep -Eq '^PUBLIC_URL=https://' /etc/routex/routex.env && \
       ! grep -q "^PUBLIC_URL=https://${ROUTEX_HOST}" /etc/routex/routex.env; then
        if [ -n "${DOMAIN:-${PUBLIC_URL_HOST:-}}" ]; then
            sed -i -E "s|^PUBLIC_URL=.*|PUBLIC_URL=https://${ROUTEX_HOST}|" /etc/routex/routex.env
            echo "   PUBLIC_URL diperbarui ke host yang diminta: https://${ROUTEX_HOST}."
        else
            echo "   ⚠️ PUBLIC_URL di routex.env menunjuk host selain ${ROUTEX_HOST}; dibiarkan apa adanya."
            echo "      Bila host itu sudah usang, jalankan ulang dengan DOMAIN=<host-baru> untuk memperbaruinya."
        fi
    fi
fi

# Pengguna sistem khusus untuk service (audit3 V-DP-22): unit systemd
# menjalankan gateway sebagai 'routex', bukan root.
if ! id routex &>/dev/null; then
    useradd --system --no-create-home --shell /usr/sbin/nologin routex
    echo "   Pengguna sistem 'routex' dibuat."
fi
# Direktori home pengguna layanan WAJIB ada dan dimiliki routex (B-095):
# gateway menyelesaikan $HOME dari entri passwd pengguna ini dan menulis
# konfigurasi CLI ke $HOME/.routex (internal/cliconfig) — useradd di atas
# memakai --no-create-home, jadi tanpa langkah ini /home/routex tidak
# pernah ada dan POST /api/admin/cli/configure gagal permanen dengan
# "mkdir /home/routex: permission denied" (proses layanan tidak bisa
# membuat direktori di /home yang dimiliki root). Langkah ini SENGAJA di
# luar blok pembuatan pengguna: ia berjalan pada SETIAP lari installer,
# sehingga menjalankan ulang installer (jalur upgrade) menyembuhkan
# instalasi lama yang terlanjur tidak punya home.
ROUTEX_HOME="$(getent passwd routex | cut -d: -f6)"
if [ -z "$ROUTEX_HOME" ] || [ "$ROUTEX_HOME" = "/" ]; then
    ROUTEX_HOME="/home/routex"
fi
mkdir -p "$ROUTEX_HOME"
chown routex:routex "$ROUTEX_HOME"
chmod 755 "$ROUTEX_HOME"
echo "   Direktori home pengguna layanan dipastikan ada: $ROUTEX_HOME (milik routex:routex)."
mkdir -p /var/lib/route-x
chown -R routex:routex "$INSTALL_DIR" /etc/routex /var/lib/route-x
chmod 600 /etc/routex/routex.env

# Pasang routex.service
if [ -f "$SRC_DIR/deploy/systemd/routex.service" ]; then
    cp "$SRC_DIR/deploy/systemd/routex.service" /etc/systemd/system/routex.service
fi

# Jalankan migrasi database
MIG_TERAKHIR="$(ls "$SRC_DIR/internal/database/migrations" 2>/dev/null | sort | tail -n 1 || true)"
echo "   Menjalankan migrasi skema database (idempoten; migrasi terakhir di sumber: ${MIG_TERAKHIR:-tidak diketahui})..."
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

# Klaim BERHASIL hanya sesudah /healthz benar-benar menjawab (audit3
# V-DP-08): service yang crash-loop tidak boleh dilaporkan sebagai
# instalasi selesai.
echo "   Memeriksa kesehatan gateway (http://127.0.0.1:8080/healthz)..."
SEHAT=0
for _try in $(seq 1 30); do
    if curl -fsS http://127.0.0.1:8080/healthz >/dev/null 2>&1; then
        SEHAT=1
        break
    fi
    sleep 2
done
if [ "$SEHAT" -ne 1 ]; then
    echo "❌ Gateway tidak menjawab /healthz sesudah ±60 detik — instalasi TIDAK dinyatakan berhasil." >&2
    echo "   Periksa: journalctl -u routex -n 100 --no-pager" >&2
    exit 1
fi
echo "   Gateway sehat (/healthz menjawab)."

echo ""
echo "=================================================================="
echo " ✅ INSTALASI NATIVE ROUTE-X SELESAI & BERHASIL!"
echo "=================================================================="
echo " 📊 Status Layanan Native:"
echo "   - Route-X Core : $(systemctl is-active routex || true)"
echo "   - Caddy Edge   : $(systemctl is-active caddy || true)"
echo "   - PostgreSQL   : $(systemctl is-active postgresql || true)"
echo "   - Redis Server : $(systemctl is-active redis-server || true)"
echo "------------------------------------------------------------------"
echo " 📝 Monitoring Log:"
echo "   journalctl -fu routex"
echo "   journalctl -fu caddy"
echo "------------------------------------------------------------------"
echo " 🎉 SETUP AWAL (FIRST-RUN ONBOARDING):"
echo "   Akses Dashboard : https://${ROUTEX_HOST}/login"
echo "   Email Admin     : admin@routex.local"
if [ -n "${ADMIN_PASS:-}" ]; then
echo "   Password Admin  : ${ADMIN_PASS}"
else
echo "   Password Admin  : (sudah tersimpan di /etc/routex/routex.env)"
fi
echo "   (Dihasilkan acak untuk instalasi ini; tersimpan sebagai INITIAL_ADMIN_PASSWORD"
echo "    di /etc/routex/routex.env — ganti saat login pertama.)"
echo "   * Anda akan otomatis diminta membuat password baru saat login."
echo "------------------------------------------------------------------"
echo " ℹ️ CATATAN HTTPS:"
echo "   Produksi mewajibkan HTTPS (PUBLIC_URL=https://${ROUTEX_HOST})."
[ -n "$CADDY_TLS_LINE" ] && \
    echo "   Host berupa IP sehingga Caddy memakai sertifikat internal (self-signed):" && \
    echo "   browser akan memperingatkan sertifikat tak terpercaya — lanjutkan untuk" && \
    echo "   uji coba, atau pasang domain dan jalankan ulang dengan DOMAIN=domainanda." && \
    echo "   Tanpa menjalankan ulang itu, domain yang dipasang belakangan akan gagal" && \
    echo "   (di Cloudflare tampak sebagai error 525) karena Caddy tetap melayani" && \
    echo "   sertifikat internal untuk IP ini."
[ -z "$CADDY_TLS_LINE" ] && \
    echo "   Sertifikat Let's Encrypt diterbitkan otomatis oleh Caddy untuk domain ini."
echo "   Konfigurasi: /etc/routex/routex.env  |  Caddy: /etc/caddy/Caddyfile"
echo "=================================================================="
