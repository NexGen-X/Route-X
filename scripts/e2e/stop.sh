#!/usr/bin/env bash
# Menghentikan proses ai-gateway dan mock server pengujian secara aman
set -euo pipefail

echo "Menghentikan layanan ai-gateway dan mock server..."

# Gunakan pkill -x agar mencocokkan nama biner persis, BUKAN pkill -f yang membunuh shell pemanggil
if pkill -x ai-gateway 2>/dev/null; then
  echo "Proses ai-gateway dihentikan (pkill -x)."
else
  echo "Proses ai-gateway tidak sedang berjalan."
fi

# Hentikan mock upstream server bila ada berkas PID atau proses Python aktif
if [ -f "/tmp/routex_mock_upstream.pid" ]; then
  PID=$(cat "/tmp/routex_mock_upstream.pid" 2>/dev/null || true)
  if [ -n "$PID" ] && kill -0 "$PID" 2>/dev/null; then
    kill "$PID" 2>/dev/null || true
    echo "Proses mock upstream ($PID) dihentikan."
  fi
  rm -f "/tmp/routex_mock_upstream.pid"
fi

pkill -f "scripts/e2e/mock_upstream.py" 2>/dev/null || true

echo "Seluruh proses pengujian telah dihentikan."
