# Panduan Deployment Gratis: Koyeb + Neon + Upstash

Panduan ini menjelaskan cara mendeploy **Route-X AI Gateway** secara **100% gratis** (*Zero-Dollar Cloud Stack*) tanpa biaya bulanan dengan memanfaatkan layanan gratis terbaik:
1. **Koyeb** ([koyeb.com](https://koyeb.com)) — Menjalankan kontainer Route-X (Eco free instance, 512MB RAM, edge network global, tanpa batasan timeout SSE).
2. **Neon** ([neon.tech](https://neon.tech)) — Serverless PostgreSQL gratis 500MB dengan koneksi SSL native.
3. **Upstash** ([upstash.com](https://upstash.com)) — Serverless Redis gratis hingga 10.000 permintaan per hari.

---

## Ringkasan Arsitektur

```
 Pengguna / Browser / SDK
            │
            ▼
 ┌────────────────────────────────────────┐
 │ Koyeb Web Service (Route-X Gateway)   │
 │ Port 8080 (HTTPS Otomatis Let's Encrypt│
 └───────────────────┬────────────────────┘
                     │
         ┌───────────┴───────────┐
         ▼                       ▼
 ┌───────────────┐       ┌───────────────┐
 │   Neon.tech   │       │    Upstash    │
 │  PostgreSQL   │       │     Redis     │
 │ (Free 500MB)  │       │ (Free 10k/day)│
 └───────────────┘       └───────────────┘
```

---

## Langkah 1: Siapkan Database PostgreSQL Gratis di Neon.tech

1. Buka dan daftar akun di **[Neon.tech](https://neon.tech)** (bisa login via GitHub).
2. Klik **Create Project**, beri nama misal `routex-db`.
3. Setelah selesai dibuat, Neon akan menampilkan kotak **Connection Details**:
   - Pilih tab **Pooled connection** atau **Direct connection**.
   - Salin **Connection string** yang berformat:
     ```text
     postgres://neondb_owner:npg_xxxx@ep-example-123.ap-southeast-1.aws.neon.tech/neondb?sslmode=require
     ```
4. Simpan nilai ini untuk diisikan ke variabel `DATABASE_URL`.

---

## Langkah 2: Siapkan Redis Gratis di Upstash.com

1. Buka dan daftar akun di **[Upstash.com](https://upstash.com)** (bisa login via GitHub).
2. Di konsol Upstash, klik **Create Database**:
   - Pilih tipe **Redis**.
   - Pilih region terdekat (misal `ap-southeast-1` Singapore).
   - Pilih paket **Free**.
3. Setelah database aktif, gulir ke bagian **Connect to your database**:
   - Pilih tab **redis-cli** atau salin URL standar yang diawali dengan `redis://`:
     ```text
     redis://default:xxxxxxxx@ap1-example-123.upstash.io:6379
     ```
4. Simpan nilai ini untuk diisikan ke variabel `REDIS_URL`.

---

## Langkah 3: Hasilkan Kunci Rahasia Keamanan

Jalankan skrip generator di komputer/terminal Anda:

```bash
bash scripts/ops/generate-secrets.sh
```

Skrip ini akan menampilkan kunci rahasia yang siap disalin:
- `SESSION_SECRET`
- `ENCRYPTION_KEY`
- `API_KEY_PEPPER`
- `METRICS_TOKEN`
- `INITIAL_ADMIN_EMAIL` & `INITIAL_ADMIN_PASSWORD`

---

## Langkah 4: Hubungkan & Deploy di Koyeb.com

1. Buka dan daftar akun di **[Koyeb.com](https://www.koyeb.com)** (bisa login via GitHub).
2. Klik **Create App** / **Create Service**:
   - Sumber (*Deployment method*): Pilih **GitHub**.
   - Pilih repositori **Route-X**.
   - Pilih branch: `main`.
3. Di bagian **Builder**:
   - Pilih **Dockerfile** (Koyeb akan membaca `Dockerfile` multi-stage di root project).
4. Di bagian **Instance Type**:
   - Pilih tipe **Eco** -> **Nano** (Paket Gratis: 512 MB RAM, 0.1 vCPU).
   - Pilih region (misal `sin` Singapore atau `fra` Frankfurt).
5. Di bagian **Ports**:
   - Masukkan port: `8080`.
   - Protocol: `HTTP`.
   - Path: `/`.
6. Di bagian **Environment Variables**, klik **Add variable** dan masukkan:

| Kunci (Key) | Nilai (Value) | Keterangan |
| :--- | :--- | :--- |
| `APP_ENV` | `production` | Wajib mode produksi |
| `PORT` | `8080` | Port internal container |
| `LOG_LEVEL` | `info` | Level log aplikasi |
| `PUBLIC_URL` | `https://<nama-app>-<username>.koyeb.app` | URL publik yang diberikan oleh Koyeb |
| `DATABASE_URL` | *(Connection string Neon dari Langkah 1)* | Wajib menyertakan `sslmode=require` |
| `REDIS_URL` | *(Redis URL Upstash dari Langkah 2)* | Dimulai dengan `redis://` |
| `SESSION_SECRET` | *(Dari Langkah 3)* | Token sesi minimal 32 karakter |
| `ENCRYPTION_KEY` | *(Dari Langkah 3)* | Tepat 32-byte Base64 |
| `API_KEY_PEPPER` | *(Dari Langkah 3)* | Minimal 32-byte Base64 |
| `METRICS_TOKEN` | *(Dari Langkah 3)* | Token metrik Prometheus |
| `INITIAL_ADMIN_EMAIL` | `admin@id-tech.cloud` | Email login pertama kali |
| `INITIAL_ADMIN_PASSWORD` | *(Dari Langkah 3)* | Password login pertama kali |
| `METRICS_ALLOW_LOOPBACK` | `false` | Pengerasan keamanan |
| `RATE_LIMIT_FAIL_CLOSED` | `false` | Kebijakan fail-open |

7. Di bagian **Health checks**:
   - Type: `HTTP`.
   - Path: `/healthz`.
   - Port: `8080`.
8. Klik **Deploy**!

---

## Langkah 5: Verifikasi & Login ke Gateway

1. Tunggu 2-3 menit hingga proses build dan deployment selesai dan statusnya berubah menjadi **Healthy** hijau.
2. Buka URL Koyeb Anda di peramban:
   ```text
   https://<nama-app>-<username>.koyeb.app/login
   ```
3. Masuk menggunakan email dan password yang Anda tentukan di `INITIAL_ADMIN_EMAIL` & `INITIAL_ADMIN_PASSWORD`.
4. Anda sekarang memiliki AI Gateway pribadi yang aktif 24/7 tanpa biaya sepeser pun!

---

## Tips & Pemeliharaan:
- **Streaming SSE**: Koyeb mendukung koneksi HTTP streaming panjang tanpa batas timeout 30 detik seperti cloud serverless lainnya.
- **Auto Deploy**: Setiap kali Anda melakukan `git push origin main` ke GitHub, Koyeb akan otomatis membangun ulang dan memperbarui versi terbaru gateway Anda tanpa *downtime*.
