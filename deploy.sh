#!/bin/bash
set -e

echo "🚀 Menyiapkan arsitektur Docker Route-X (4 Layanan: Route-X, DB, Redis, Caddy)..."

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

if [ -z "${DOMAIN:-}" ]; then
    echo "⚠️ DOMAIN tidak disetel: PUBLIC_URL akan memakai alamat IP publik utama server"
    echo "   dan sertifikat Caddy untuk IP adalah CA internal (tidak tepercaya publik)."
    echo "   Untuk produksi nyata, jalankan ulang dengan DOMAIN=domainanda.com."
fi

# 1. Pastikan folder konfigurasi tersedia
mkdir -p deploy/caddy

# 2. Otomatis inisialisasi file .env bila belum ada.
# PENJAGA DATA LAMA (audit3 V-DP-10): .env hilang TETAPI volume data
# PostgreSQL masih ada berarti ini BUKAN instalasi baru — menghasilkan
# rahasia baru di atas data lama membuat gateway tidak bisa membaca
# databasenya sendiri (password & kunci enkripsi berubah). Berhenti dan
# minta operator memulihkan .env dari cadangannya.
if [ ! -f .env ] && docker volume ls -q 2>/dev/null | grep -q 'routex_pgdata$'; then
    echo "❌ DITEMUKAN volume data PostgreSQL lama (routex_pgdata) tetapi file .env HILANG." >&2
    echo "   Membuat .env baru akan menghasilkan kunci enkripsi & password DB baru yang" >&2
    echo "   TIDAK cocok dengan data lama. Pulihkan .env dari cadangan Anda, letakkan di" >&2
    echo "   direktori ini, lalu jalankan ulang. Untuk instalasi yang BENAR-BENAR baru," >&2
    echo "   hapus volume lama dulu: docker volume rm <nama>_routex_pgdata" >&2
    exit 1
fi
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
    SEC_ADMIN="$(rand_hex 12)"
    SEC_ADMIN_PRINT="$SEC_ADMIN"

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
    KEY_ADMIN="INITIAL_ADMIN_PASSWORD"

    sed -i "s|^${KEY_SESS}=.*|${KEY_SESS}=${SEC_SESSION}|" .env 2>/dev/null || true
    sed -i "s|^${KEY_ENC}=.*|${KEY_ENC}=${SEC_ENC}|" .env 2>/dev/null || true
    sed -i "s|^${KEY_PEPPER}=.*|${KEY_PEPPER}=${SEC_PEPPER}|" .env 2>/dev/null || true
    sed -i "s|^${KEY_PG}=.*|${KEY_PG}=${SEC_PGPASS}|" .env 2>/dev/null || true
    sed -i "s|^${KEY_METRICS}=.*|${KEY_METRICS}=${SEC_METRICS}|" .env 2>/dev/null || true
    sed -i "s|^${KEY_PURL}=.*|${KEY_PURL}=${SEC_PUBLIC_URL}|" .env 2>/dev/null || true
    sed -i "s|^${KEY_ADMIN}=.*|${KEY_ADMIN}=${SEC_ADMIN}|" .env 2>/dev/null || true
    chmod 600 .env
    echo "   File .env berhasil dibuat dengan aman (izin 600)."
fi

# Instalasi lama yang .env-nya masih berisi placeholder publik bawaan
# .env.example (awalan GANTI_, 32 karakter) diperbaiki otomatis: nilai itu
# BUKAN password rahasia karena tercetak di repositori publik.
KEY_ADMIN="INITIAL_ADMIN_PASSWORD"
ADMIN_VAL="$(grep "^${KEY_ADMIN}=" .env 2>/dev/null | head -1 | cut -d= -f2-)"
if [ "${ADMIN_VAL#GANTI_}" != "$ADMIN_VAL" ] && [ "${#ADMIN_VAL}" = "32" ]; then
    SEC_ADMIN_PRINT="$(rand_hex 12)"
    sed -i "s|^${KEY_ADMIN}=.*|${KEY_ADMIN}=${SEC_ADMIN_PRINT}|" .env
    chmod 600 .env
    echo "🔐 INITIAL_ADMIN_PASSWORD di .env lama masih placeholder publik — diganti nilai acak unik."
fi

# Cap versi biner Docker: sumber versi tunggal repo adalah web/package.json
# (audit3 V-DP-12 — tanpa ini biner Docker selalu tercap "dev"). Operator
# boleh menimpa lewat env VERSION; bila ekstraksi gagal, "dev" dipertahankan
# sebagai cap yang jujur untuk build tanpa identitas rilis.
if [ -z "${VERSION:-}" ] && [ -f web/package.json ]; then
    PKG_VERSION="$(sed -n 's/.*"version"[[:space:]]*:[[:space:]]*"\([^"]*\)".*/\1/p' web/package.json | head -n 1)"
    if [ -n "$PKG_VERSION" ]; then
        VERSION="v${PKG_VERSION}"
    fi
fi
export VERSION="${VERSION:-dev}"
echo "🏷️  Versi biner: ${VERSION}"

# Build SEBELUM down (audit3 V-DP-09): versi lama mematikan layanan dulu
# baru membangun, jadi kegagalan build memperpanjang downtime tanpa alasan.
echo "📦 Membangun image Docker (Kompilasi Frontend & Backend)..."
docker compose build

echo "🧹 Membersihkan kontainer lama..."
docker compose down --remove-orphans
docker rm -f routex-postgres routex-core routex-caddy routex-redis 2>/dev/null || true

echo "🛠️ Menjalankan layanan infrastruktur dasar (PostgreSQL, Redis)..."
docker compose up -d db redis

echo "⏳ Menunggu PostgreSQL siap menerima koneksi..."
for _try in $(seq 1 30); do
    if docker compose exec -T db pg_isready -U routex -d routex_prod >/dev/null 2>&1; then
        echo "   PostgreSQL siap."
        break
    fi
    if [ "$_try" = "30" ]; then
        echo "❌ PostgreSQL tidak siap setelah ±60 detik — periksa: docker compose logs db"
        exit 1
    fi
    sleep 2
done

echo "🔄 Menjalankan inisialisasi dan migrasi database..."
# Menjalankan container routex sementara hanya untuk flag -migrate
docker compose run --rm routex ./ai-gateway -migrate

echo "🌐 Menyalakan Route-X Core dan Caddy Edge Router..."
docker compose up -d routex caddy

# Klaim BERHASIL hanya sesudah /healthz menjawab lewat Caddy (audit3
# V-DP-08) — kontainer 'up' bukan bukti aplikasi sehat.
PROBE_HOST="${DOMAIN:-localhost}"
echo "⏳ Memeriksa kesehatan gateway (https://${PROBE_HOST}/healthz)..."
SEHAT=0
for _try in $(seq 1 30); do
    if curl -fsSk --resolve "${PROBE_HOST}:443:127.0.0.1" "https://${PROBE_HOST}/healthz" >/dev/null 2>&1; then
        SEHAT=1
        break
    fi
    sleep 2
done
if [ "$SEHAT" -ne 1 ]; then
    echo "❌ Gateway tidak menjawab /healthz sesudah ±60 detik — deployment TIDAK dinyatakan berhasil." >&2
    echo "   Periksa: docker compose logs routex" >&2
    exit 1
fi

echo ""
echo "✅ DEPLOYMENT BERHASIL!"
echo "Semua 4 layanan kini berjalan secara terisolasi di dalam kontainer Docker."
echo "--------------------------------------------------------"
echo "📊 Cek status layanan : docker compose ps"
echo "📝 Pantau log Route-X : docker compose logs -f routex"
echo "🔐 Pantau log Caddy   : docker compose logs -f caddy"
echo "--------------------------------------------------------"
echo "🎉 SETUP AWAL (FIRST-RUN ONBOARDING):"
if [ -n "${SEC_ADMIN_PRINT:-}" ]; then
    echo "   Email admin    : admin@routex.local (atau INITIAL_ADMIN_EMAIL di .env)"
    echo "   Password admin : ${SEC_ADMIN_PRINT}"
    echo "   (Dihasilkan acak untuk deploy ini — simpan sekarang dan ganti saat"
    echo "    login pertama. Nilai yang sama tersimpan di .env yang berizin 600.)"
else
    echo "   Gunakan INITIAL_ADMIN_EMAIL / INITIAL_ADMIN_PASSWORD dari file .env Anda."
    echo "   (Tidak ada password default — admin awal hanya dibuat bila variabel"
    echo "    itu terisi sebelum start pertama.)"
fi
echo "--------------------------------------------------------"
