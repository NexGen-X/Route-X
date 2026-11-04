# Route-X Disaster Recovery & Backup Runbook

Panduan pencadangan dan pemulihan database Route-X. Ada dua jalur resmi:

1. **Skrip server** [`scripts/ops/backup-db.sh`](../scripts/ops/backup-db.sh) — dump terjadwal di sisi server (untuk cadangan berkala di luar aplikasi).
2. **Panel admin** (Pengaturan → Cadangan & Pemulihan) — ekspor dan restore dari dalam aplikasi. **Restore produksi WAJIB lewat jalur ini**: restore admin memakai mekanisme skratch-swap yang aman (lihat §3).

> ⚠️ **Jangan pernah memulihkan dump langsung ke database produksi** (`gunzip | psql "$DATABASE_URL"` dan sejenisnya). Prosedur lama itu sudah dihapus dari dokumen ini: ia menulis ke database aktif tanpa pemeriksaan isi, tanpa satu transaksi, dan tanpa salinan pengaman — satu berkas cadangan yang rusak atau berbahaya langsung menjadi kerusakan produksi. Pemulihan `psql` manual hanya boleh diarahkan ke database **buang/scratch** untuk verifikasi (§4).

---

## 1. Backup otomatis di server (skrip)

Fakta skrip [`scripts/ops/backup-db.sh`](../scripts/ops/backup-db.sh) saat ini:

- Membaca `DATABASE_URL` dari `/etc/routex/routex.env`; password diteruskan ke `pg_dump` lewat environment `PGPASSWORD` (tidak pernah lewat argumen proses).
- Menulis ke `/var/backups/routex/` (izin `700`) dengan pola nama `routex_prod_YYYYMMDD_HHMMSS.sql.gz`, kompresi gzip level 9, izin berkas `600`.
- Menguji integritas arsip (`gzip -t`) sebelum menyatakan sukses; log ke `/var/log/routex/backup.log`.
- **Rotasi otomatis: 7 hari** — berkas `routex_prod_*.sql.gz` yang lebih tua dihapus di akhir run yang sukses.

Repo **tidak** memasang penjadwalan apa pun secara otomatis. Bila ingin backup harian, pasang skrip dan cron-nya sendiri, contoh:

```bash
sudo install -m 700 scripts/ops/backup-db.sh /usr/local/bin/routex-backup.sh

# /etc/cron.d/routex-backup — contoh: setiap hari pukul 02:00 UTC
0 2 * * * root /usr/local/bin/routex-backup.sh
```

Menjalankan backup manual (mis. sebelum perubahan besar):

```bash
sudo /usr/local/bin/routex-backup.sh
tail -n 20 /var/log/routex/backup.log
ls -lht /var/backups/routex/ | head
```

## 2. Ekspor dari panel admin

Di konsol: **Pengaturan → Cadangan & Pemulihan → Ekspor**, atau lewat API (POST, tunduk CSRF seperti mutasi admin lain):

```
POST /api/admin/system/backup/export?format=sql.gz   (bawaan)
POST /api/admin/system/backup/export?format=sql      (teks polos)
```

Ekspor mengalirkan dump `pg_dump` (opsi `--clean --if-exists --no-owner --no-privileges`) sebagai unduhan `routex_backup_YYYYMMDD_HHMMSS.sql.gz`. Simpan hasilnya di luar server produksi.

## 3. Restore produksi — lewat panel admin (skratch-swap)

Di konsol: **Pengaturan → Cadangan & Pemulihan → Restore**, unggah berkas `.sql` atau `.sql.gz` (unggahan terkompresi dibatasi 50 MB; hasil dekompresi dibatasi 1 GiB). Atau lewat API: `POST /api/admin/system/backup/restore`.

Alur yang dijalankan server (audit3 B-045):

1. Unggahan dispool ke berkas sementara dan **dipindai**: meta-command psql (`\!` dan sejenisnya) serta `COPY ... PROGRAM` ditolak dengan `400 invalid_backup_content` — berkas cadangan tidak bisa menjadi jalur eksekusi perintah OS.
2. Dump di-restore ke **database skratch** sekali pakai memakai `psql --single-transaction` + `ON_ERROR_STOP` — kegagalan di tengah membatalkan seluruhnya; database produksi belum tersentuh.
3. Versi skema skratch diperiksa ke ledger migrasi: lebih baru dari biner → ditolak; lebih lama → dimigrasikan di skratch sampai versi terkini (saat ini 0021).
4. Nama database **ditukar secara atomik**: database lama dipertahankan utuh sebagai **`<nama-db>_pre_restore`**, skratch menjadi database aktif. Respons sukses memuat `safety_database` dan `schema_version`.
5. Gateway melanjutkan layanan memakai database hasil restore.

**Syarat hak database (B-104):** langkah 2 dan 4 dijalankan gateway memakai role aplikasi dari `DATABASE_URL`, jadi role itu wajib memiliki hak **`CREATEDB`** — PostgreSQL mensyaratkannya untuk membuat database skratch dan untuk `ALTER DATABASE ... RENAME` pada penukaran (pemilik database saja tidak cukup untuk kedua operasi itu). Pemutusan koneksi (`pg_terminate_backend`) dan penghapusan salinan pengaman hanya menyentuh backend/database milik role itu sendiri. Installer native memberikan tepat hak ini — hanya `CREATEDB`, bukan `SUPERUSER`/`CREATEROLE` — sejak pembuatan role, dan **setiap menjalankan ulang installer menyelaraskannya kembali**, sehingga instalasi lama yang role-nya lahir tanpa `CREATEDB` sembuh cukup dengan menjalankan ulang installer (sebelum perbaikan ini, restore pada instalasi native standar selalu gagal `42501: permission denied to create database` sementara ekspor bekerja). Pemasangan manual di luar installer wajib menyetelnya sendiri, mis. `ALTER ROLE <role-aplikasi> WITH CREATEDB;` sebagai superuser. Jalur compose/Docker tidak terpengaruh: di sana role aplikasi adalah superuser bawaan image PostgreSQL.

Sesudah restore:

```bash
curl -s http://127.0.0.1:8080/healthz   # {"status":"ok"}
curl -s http://127.0.0.1:8080/readyz    # status + checks per dependensi
```

Lalu buka konsol dan periksa hitungan ringkas di Pengaturan → Cadangan & Pemulihan (model, provider, aturan routing, API key) serta lakukan satu request uji.

### Rollback bila hasil restore tidak beres

Database pra-restore masih utuh sebagai `<nama-db>_pre_restore`. Untuk kembali:

1. Hentikan layanan (`systemctl stop routex`).
2. Dari database `postgres`, ganti nama database aktif saat ini menjadi nama lain (mis. `<nama-db>_gagal`), lalu ganti nama `<nama-db>_pre_restore` menjadi `<nama-db>`.
3. Nyalakan kembali layanan dan ulangi verifikasi di atas.

## 4. Verifikasi berkas cadangan (database buang)

Untuk sekadar membuktikan sebuah dump bisa dibaca — tanpa menyentuh produksi:

```bash
gzip -t /var/backups/routex/routex_prod_YYYYMMDD_HHMMSS.sql.gz

sudo -u postgres createdb routex_verifikasi
gunzip -c /var/backups/routex/routex_prod_YYYYMMDD_HHMMSS.sql.gz | psql -d routex_verifikasi -v ON_ERROR_STOP=1
psql -d routex_verifikasi -tAc "SELECT count(*) FROM schema_migrations;"   # harus: 21
sudo -u postgres dropdb routex_verifikasi
```
