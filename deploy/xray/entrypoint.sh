#!/bin/sh
echo "[Route-X Xray] Pengawas Xray aktif..."
# SEC-002: kredensial hanya boleh hidup di file runtime dari secret store atau
# generator aplikasi. Template tracked tidak boleh pernah dijalankan.
CFG=/etc/xray/runtime/config.json

# Teruskan SIGTERM/SIGINT ke proses xray agar shutdown berjalan graceful.
graceful_stop() {
  if [ -n "${PID:-}" ] && kill -0 "$PID" 2>/dev/null; then
    kill -TERM "$PID" 2>/dev/null || true
    wait "$PID" 2>/dev/null || true
  fi
  exit 0
}
trap graceful_stop TERM INT

while true; do
  if [ -f "$CFG" ]; then
    xray -config "$CFG" &
    PID=$!
    PREV_HASH=$(md5sum "$CFG" 2>/dev/null)
    while kill -0 "$PID" 2>/dev/null; do
      sleep 2
      CURR_HASH=$(md5sum "$CFG" 2>/dev/null)
      if [ "$PREV_HASH" != "$CURR_HASH" ]; then
        echo "[Route-X Xray] Konfigurasi berubah, memperbarui proses Xray..."
        kill -TERM "$PID" 2>/dev/null
        wait "$PID" 2>/dev/null
        break
      fi
    done
  else
    echo "[Route-X Xray] Menunggu $CFG..."
    sleep 3
  fi
done
