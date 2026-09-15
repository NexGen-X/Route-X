# Route-X Disaster Recovery & Backup Runbook

Panduan ini mendokumentasikan prosedur pencadangan otomatis (*backup*) dan pemulihan darurat (*disaster recovery*) untuk database produksi Route-X (`routex_prod`).

---

## 🗄️ 1. Mekanisme Backup Otomatis

- **Skrip Eksekusi**: [`scripts/ops/backup-db.sh`](../scripts/ops/backup-db.sh) / `/usr/local/bin/routex-backup.sh`
- **Jadwal Otomatis**: Setiap hari pukul `02:00 UTC` via `/etc/cron.d/routex-backup`
- **Lokasi Cadangan**: `/var/backups/routex/`
- **Format Berkas**: `routex_prod_YYYYMMDD_HHMMSS.sql.gz`
- **Tingkat Kompresi**: Gzip Level 9 (menghemat >80% ruang penyimpanan)
- **Rotasi Otomatis**: Berkas cadangan yang berusia lebih dari **7 hari** dihapus secara otomatis.
- **Berkas Log**: `/var/log/routex/backup.log`

### Menjalankan Backup Manual (On-Demand)
Jika Anda ingin mencadangkan database sebelum melakukan perubahan besar atau migrasi:
```bash
/usr/local/bin/routex-backup.sh
```

Periksa log hasil backup:
```bash
tail -n 20 /var/log/routex/backup.log
```

---

## 🚑 2. Prosedur Pemulihan Darurat (*Restore Procedure*)

Jika terjadi kehilangan data, kegagalan disk, atau kesalahan operasi:

### Langkah 1: Hentikan Layanan Route-X Sementara
Mencegah lalu lintas baru menulis ke database saat proses pemulihan:
```bash
systemctl stop routex
```

### Langkah 2: Pilih Berkas Cadangan Terbaru
Lihat daftar cadangan yang tersedia di `/var/backups/routex/`:
```bash
ls -lht /var/backups/routex/routex_prod_*.sql.gz | head -n 5
```

### Langkah 3: Uji Integritas Berkas Cadangan
Pastikan berkas gzip tidak terkorupsi:
```bash
gzip -t /var/backups/routex/routex_prod_YYYYMMDD_HHMMSS.sql.gz
```

### Langkah 4: Pulihkan Database PostgreSQL
Jalankan dekrompresi dan restore langsung ke `routex_prod`:
```bash
# Opsi A: Menggunakan kredensial dari routex.env secara otomatis
set -a && . /etc/routex/routex.env && set +a
gunzip -c /var/backups/routex/routex_prod_YYYYMMDD_HHMMSS.sql.gz | psql "$DATABASE_URL"

# Opsi B: Menggunakan user lokal postgres
gunzip -c /var/backups/routex/routex_prod_YYYYMMDD_HHMMSS.sql.gz | sudo -u postgres psql -d routex_prod
```

### Langkah 5: Nyalakan Kembali Layanan Route-X
```bash
systemctl start routex
systemctl status routex --no-pager
```

### Langkah 6: Verifikasi Status Pemulihan
1. Periksa endpoint kesehatan gateway:
   ```bash
   curl -i http://127.0.0.1:8080/health
   ```
2. Pastikan tabel metadata (roles, models, api_keys) terisi normal:
   ```bash
   sudo -u postgres psql -d routex_prod -c "SELECT COUNT(*) FROM roles; SELECT COUNT(*) FROM models;"
   ```
