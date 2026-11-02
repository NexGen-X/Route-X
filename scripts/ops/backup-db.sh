#!/usr/bin/env bash
# ==============================================================================
# Route-X Automated Database Backup & Disaster Recovery
# ==============================================================================
set -euo pipefail

ENV_FILE="/etc/routex/routex.env"
BACKUP_DIR="/var/backups/routex"
RETENTION_DAYS=7
LOG_FILE="/var/log/routex/backup.log"

# Pastikan folder log dan backup tersedia
mkdir -p "$BACKUP_DIR"
mkdir -p "$(dirname "$LOG_FILE")"
chmod 700 "$BACKUP_DIR"

log() {
  local msg="[$(date -u '+%Y-%m-%dT%H:%M:%SZ')] $1"
  echo "$msg"
  echo "$msg" >> "$LOG_FILE"
}

log "Memulai pencadangan database Route-X..."

# Baca DATABASE_URL dari berkas environment resmi
if [[ ! -f "$ENV_FILE" ]]; then
  log "ERROR: Berkas konfigurasi $ENV_FILE tidak ditemukan!"
  exit 1
fi

DATABASE_URL=$(grep -E '^DATABASE_URL=' "$ENV_FILE" | cut -d '=' -f 2- | tr -d '"' | tr -d "'")
if [[ -z "$DATABASE_URL" ]]; then
  log "ERROR: DATABASE_URL tidak ditemukan di $ENV_FILE!"
  exit 1
fi

TIMESTAMP=$(date -u '+%Y%m%d_%H%M%S')
BACKUP_FILE="${BACKUP_DIR}/routex_prod_${TIMESTAMP}.sql.gz"

# Uraikan DATABASE_URL menjadi komponen koneksi diskret (audit3 V-DP-19):
# URL utuh sebagai argumen pg_dump memuat password dan tampil di daftar
# proses (ps aux) untuk dibaca semua pengguna lokal. Password hanya boleh
# lewat environment PGPASSWORD, yang tidak tampil di daftar proses.
dburl="${DATABASE_URL#*://}"
userinfo="${dburl%%@*}"
hostdb="${dburl#*@}"
PGUSER="${userinfo%%:*}"
PGPASSWORD_BAK="${userinfo#*:}"
hostport="${hostdb%%/*}"
PGDATABASE_BAK="${hostdb#*/}"
PGDATABASE_BAK="${PGDATABASE_BAK%%\?*}"
PGHOST_BAK="${hostport%%:*}"
PGPORT_BAK="${hostport##*:}"
if [[ "$PGPORT_BAK" == "$PGHOST_BAK" ]]; then
  PGPORT_BAK="5432"
fi
# Password dari installer berbentuk heksadesimal (tanpa karakter terenkode);
# nilai yang mengandung %XX diuraikan python3 bila tersedia.
if [[ "$PGPASSWORD_BAK" == *%* ]] && command -v python3 >/dev/null 2>&1; then
  PGPASSWORD_BAK="$(printf '%s' "$PGPASSWORD_BAK" | python3 -c 'import sys,urllib.parse; print(urllib.parse.unquote(sys.stdin.read()))')"
fi
export PGUSER PGHOST="$PGHOST_BAK" PGPORT="$PGPORT_BAK" PGDATABASE="$PGDATABASE_BAK"
export PGPASSWORD="$PGPASSWORD_BAK"
case "$DATABASE_URL" in
  *sslmode=*)
    PGSSLMODE_BAK="$(printf '%s' "$DATABASE_URL" | sed -n 's/.*sslmode=\([a-z-]*\).*/\1/p')"
    export PGSSLMODE="$PGSSLMODE_BAK"
    ;;
esac

log "Menjalankan pg_dump ke ${BACKUP_FILE}..."

# Jalankan dump terkompresi gzip level 9
if pg_dump --clean --if-exists --no-owner --no-privileges | gzip -9 > "$BACKUP_FILE"; then
  chmod 600 "$BACKUP_FILE"
else
  log "ERROR: Gagal menjalankan pg_dump!"
  rm -f "$BACKUP_FILE"
  exit 1
fi

# Uji integritas arsip
if gzip -t "$BACKUP_FILE"; then
  FILE_SIZE=$(du -h "$BACKUP_FILE" | cut -f 1)
  log "SUKSES: Pencadangan selesai. Ukuran berkas: ${FILE_SIZE} (${BACKUP_FILE})"
else
  log "ERROR: Arsip backup terkorupsi atau gagal uji integritas!"
  rm -f "$BACKUP_FILE"
  exit 1
fi

# Rotasi berkas backup lama (hapus yang lebih dari RETENTION_DAYS hari)
DELETED_COUNT=$(find "$BACKUP_DIR" -type f -name "routex_prod_*.sql.gz" -mtime +"$RETENTION_DAYS" | wc -l)
if [[ "$DELETED_COUNT" -gt 0 ]]; then
  find "$BACKUP_DIR" -type f -name "routex_prod_*.sql.gz" -mtime +"$RETENTION_DAYS" -delete
  log "Rotasi: Menghapus ${DELETED_COUNT} berkas backup lama (> ${RETENTION_DAYS} hari)."
fi

TOTAL_BACKUPS=$(find "$BACKUP_DIR" -type f -name "routex_prod_*.sql.gz" | wc -l)
log "Total cadangan tersimpan di ${BACKUP_DIR}: ${TOTAL_BACKUPS} berkas."
