#!/bin/sh
echo "[Route-X Xray] Pengawas Xray aktif..."
while true; do
  if [ -f /etc/xray/config.json ]; then
    xray -config /etc/xray/config.json &
    PID=$!
    PREV_HASH=$(md5sum /etc/xray/config.json 2>/dev/null)
    while kill -0 $PID 2>/dev/null; do
      sleep 2
      CURR_HASH=$(md5sum /etc/xray/config.json 2>/dev/null)
      if [ "$PREV_HASH" != "$CURR_HASH" ]; then
        echo "[Route-X Xray] Konfigurasi berubah, memperbarui proses Xray..."
        kill -TERM $PID 2>/dev/null
        wait $PID 2>/dev/null
        break
      fi
    done
  else
    echo "[Route-X Xray] Menunggu /etc/xray/config.json..."
    sleep 3
  fi
done
