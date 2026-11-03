# Panduan Deployment Route-X ke Fly.io

Dokumen ini memandu deployment **Route-X AI Gateway** ke platform cloud **[Fly.io](https://fly.io)** menggunakan micro-VM (Fly Machines).

Fly.io menjalankan kontainer Docker Route-X pada mesin terisolasi dengan sertifikat TLS otomatis di domain `*.fly.dev`. Konfigurasi acuan repo adalah `fly.toml` (region bawaan `sin`, 1 CPU shared, 512 MB) — sesuaikan dengan kebutuhan Anda.

---

## 1. Prasyarat

1. Akun aktif di **[Fly.io](https://fly.io)**.
2. CLI resmi **`flyctl`** telah terinstal di komputer Anda:
   - **macOS / Linux**:
     ```bash
     curl -L https://fly.io/install.sh | sh
     ```
   - **Windows (PowerShell)**:
     ```powershell
     iwr https://fly.io/install.ps1 -useb | iex
     ```
3. Login ke akun Fly.io via terminal:
   ```bash
   fly auth login
   ```

---

## 2. Inisialisasi Aplikasi di Fly.io

1. Buka folder repositori **Route-X** di terminal:
   ```bash
   cd /path/to/Route-X
   ```

2. Jalankan perintah inisialisasi:
   ```bash
   fly launch --no-deploy
   ```
   - Pilih nama aplikasi yang unik (contoh: `my-routex-gateway`).
   - Pilih region terdekat (contoh: `sin` untuk Singapore / Asia Tenggara).
   - Saat ditanya apakah ingin memodifikasi pengaturan, pilih **No** (konfigurasi sudah diatur di `fly.toml`).

---

## 3. Menyiapkan PostgreSQL & Redis

Route-X membutuhkan PostgreSQL dan Redis. Anda memiliki dua opsi:

### Opsi A: Menggunakan Layanan Internal Fly.io (Paling Praktis)
1. **Buat Database PostgreSQL**:
   ```bash
   fly postgres create --name my-routex-db --vm-size shared-cpu-1x --initial-cluster-size 1 --volume-size 3
   fly postgres attach my-routex-db --app my-routex-gateway
   ```
   *(Perintah ini otomatis menginjeksi `DATABASE_URL` ke aplikasi Route-X)*.

2. **Buat Redis (Upstash for Fly)**:
   ```bash
   fly redis create --name my-routex-redis --plan free
   ```
   *(Perintah ini otomatis menginjeksi `REDIS_URL` ke aplikasi Route-X)*.

---

### Opsi B: Menggunakan Database Eksternal (Neon & Upstash Cloud)
Jika Anda menggunakan Neon.tech dan Upstash.com:
```bash
fly secrets set DATABASE_URL="postgres://neondb_owner:password@ep-xyz.ap-southeast-1.aws.neon.tech/neondb?sslmode=require"
fly secrets set REDIS_URL="redis://default:password@endpoint.upstash.io:6379"
```

---

## 4. Setel Kunci Rahasia Keamanan (Secrets)

Jalankan skrip bantuan otomatis yang kami sediakan:
```bash
bash scripts/ops/fly-set-secrets.sh <nama-aplikasi-anda>
```

Atau jalankan perintah `fly secrets set` secara manual:
```bash
fly secrets set \
  SESSION_SECRET="$(openssl rand -base64 48)" \
  ENCRYPTION_KEY="$(openssl rand -base64 32)" \
  API_KEY_PEPPER="$(openssl rand -base64 32)" \
  METRICS_TOKEN="$(openssl rand -hex 32)" \
  PUBLIC_URL="https://<nama-aplikasi-anda>.fly.dev"

# Catatan: INITIAL_ADMIN_EMAIL opsional, INITIAL_ADMIN_PASSWORD WAJIB disetel
# agar akun admin pertama dibuat. Tidak ada password default: tanpa
# INITIAL_ADMIN_PASSWORD, seed tidak membuat admin sama sekali.
# Anda juga dapat menyetelnya secara eksplisit:
# fly secrets set INITIAL_ADMIN_EMAIL="admin@domain.com" INITIAL_ADMIN_PASSWORD="<password>"

# Wajib HANYA bila memakai integrasi Antigravity (Google OAuth): client
# secret dibaca dari environment saja — tidak ada nilai tertanam di biner.
# fly secrets set ANTIGRAVITY_CLIENT_SECRET="<client-secret-antigravity>"
```

Daftar lengkap variabel yang dibaca aplikasi ada di `.env.fly.example` dan `internal/config/config.go`. Nilai bawaan `fly.toml` sudah menyetel `METRICS_ALLOW_LOOPBACK=false`, `RATE_LIMIT_FAIL_CLOSED=false`, dan pool DB `DB_MAX_CONNS=15`/`DB_MIN_CONNS=2` untuk ukuran mesin Fly.

---

## 5. Deploy & Nyalakan Gateway

Jalankan perintah deploy:
```bash
fly deploy
```

Fly.io akan:
1. Membangun kontainer Docker secara multi-stage (Node 24 Vite + Go 1.27.3 + Alpine 3.24, sesuai `Dockerfile`).
2. Menyalakan mesin di region yang dipilih.
3. Menjalankan auto-migrasi skema database (0001 sampai 0020) dan inisialisasi akun admin.
4. Memvalidasi health check di `/healthz` (respons hanya `{"status":"ok"}`; interval 30 detik sesuai `fly.toml`).

---

## 6. Verifikasi & Akses Dashboard

1. **Cek Status & Log**:
   ```bash
   fly status
   fly logs
   ```
2. **Cek Health Check**:
   ```bash
   curl -i https://<nama-aplikasi-anda>.fly.dev/healthz
   ```
3. **Buka Dashboard**:
   ```bash
   fly open /login
   ```
   Masuk menggunakan email dan password dari `INITIAL_ADMIN_EMAIL`/`INITIAL_ADMIN_PASSWORD` yang Anda setel. Anda akan otomatis diarahkan untuk mengganti password baru pada login pertama.

---

## 7. Catatan Arsitektur di Fly.io

- **Anycast**: Permintaan ke domain `*.fly.dev` diarahkan Fly ke mesin terdekat yang sehat.
- **Streaming SSE**: `fly.toml` repo memakai concurrency berbasis requests (soft limit 200, hard limit 500 per mesin) dan tidak memutus koneksi streaming; inferensi panjang dapat di-stream sampai selesai.
- **Health Monitoring**: Fly memeriksa `/healthz` berkala; mesin yang gagal pemeriksaan dikeluarkan dari rotasi sampai pulih. `auto_stop_machines = "stop"` dengan `min_machines_running = 1` berarti mesin idle dapat berhenti dan dibangunkan lagi oleh request berikutnya (ada jeda cold-start).
