#!/usr/bin/env bash
# ==============================================================================
# Route-X — Skrip Otomasi Pemasangan Bare-Metal / VPS Produksi (Linux Systemd)
# ==============================================================================
set -euo pipefail

echo "=================================================================="
echo "          Route-X — Penyiapan Lingkungan Produksi Linux           "
echo "=================================================================="

if [ "$EUID" -ne 0 ]; then
  echo "Error: Skrip ini wajib dijalankan sebagai root (gunakan sudo)."
  exit 1
fi

BIN_SOURCE="${1:-./ai-gateway}"
if [ ! -f "$BIN_SOURCE" ]; then
  echo "Error: Biner sumber '$BIN_SOURCE' tidak ditemukan. Jalankan 'make build' terlebih dahulu."
  exit 1
fi

# 1. Buat pengguna dan grup sistem routex
echo "[1/5] Menyiapkan pengguna sistem 'routex'..."
if ! id -u routex >/dev/null 2>&1; then
  useradd -r -s /bin/false -d /var/lib/route-x routex
  echo "      Pengguna 'routex' berhasil dibuat."
else
  echo "      Pengguna 'routex' sudah ada."
fi

# 2. Buat direktori kerja dan konfigurasi
echo "[2/5] Menyiapkan direktori /etc/route-x dan /var/lib/route-x..."
mkdir -p /etc/route-x /var/lib/route-x
chown -R routex:routex /var/lib/route-x
chmod 750 /var/lib/route-x

# 3. Salin berkas lingkungan jika belum ada
echo "[3/5] Memeriksa berkas konfigurasi /etc/route-x/route-x.env..."
if [ ! -f /etc/route-x/route-x.env ]; then
  if [ -f .env ]; then
    cp .env /etc/route-x/route-x.env
  else
    cp .env.example /etc/route-x/route-x.env
  fi
  chmod 600 /etc/route-x/route-x.env
  chown root:root /etc/route-x/route-x.env
  echo "      Berkas /etc/route-x/route-x.env disalin dengan izin ketat 600."
else
  echo "      Berkas /etc/route-x/route-x.env sudah tersedia."
fi

# 4. Pasang biner biner ke /usr/local/bin
echo "[4/5] Memasang biner ke /usr/local/bin/ai-gateway..."
cp "$BIN_SOURCE" /usr/local/bin/ai-gateway
chmod 755 /usr/local/bin/ai-gateway
chown root:root /usr/local/bin/ai-gateway

# 5. Pasang dan aktifkan unit systemd
echo "[5/5] Mengonfigurasi unit systemd route-x.service..."
cp deploy/systemd/route-x.service /etc/systemd/system/route-x.service
systemctl daemon-reload
systemctl enable route-x.service

echo "------------------------------------------------------------------"
echo "Pemasangan selesai!"
echo "Untuk menyalakan layanan, jalankan:"
echo "  systemctl start route-x"
echo "Untuk melihat log jurnal:"
echo "  journalctl -u route-x -f"
echo "=================================================================="
