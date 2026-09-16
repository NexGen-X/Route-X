# Panduan Deployment Route-X ke Fly.io

Dokumen ini memandu deployment **Route-X AI Gateway** ke platform cloud **[Fly.io](https://fly.io)** menggunakan arsitektur micro-VM global Anycast.

Fly.io menjalankan kontainer Docker Route-X pada mesin fisik terisolasi dengan latensi ultra-rendah dan sertifikat TLS otomatis di domain `*.fly.dev`.

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

Route-X membutuhkan PostgreSQL dan Redis. Anda memiliki dua opsi gratis:

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

# Catatan: INITIAL_ADMIN_EMAIL & INITIAL_ADMIN_PASSWORD bersifat opsional.
# Jika tidak disetel, Route-X otomatis mengaktifkan setup awal dengan
# admin default (admin@routex.local / RouteX#Initial2026!).
# Anda juga dapat menyetelnya secara eksplisit:
# fly secrets set INITIAL_ADMIN_EMAIL="admin@domain.com" INITIAL_ADMIN_PASSWORD="<password>"
```

---

## 5. Deploy & Nyalakan Gateway

Jalankan perintah deploy:
```bash
fly deploy
```

Fly.io akan:
1. Membangun kontainer Docker secara multi-stage (Node 20 Vite + Go 1.27 + Alpine).
2. Menyalakan mesin di region yang dipilih.
3. Menjalankan auto-migrasi skema database (0001 sampai 0010) dan inisialisasi akun admin.
4. Memvalidasi health check di `/healthz`.

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
   Masuk menggunakan email dan password yang Anda tentukan, atau gunakan kredensial onboarding default (`admin@routex.local` / `RouteX#Initial2026!`) dengan tombol **"Gunakan Kredensial Default (1-Klik)"**. Anda akan otomatis diarahkan untuk mengganti password baru pada login pertama.

---

## 7. Keunggulan Arsitektur di Fly.io

- **Anycast Global IP**: Permintaan dari klien di seluruh dunia otomatis diarahkan ke node terdekat.
- **Streaming SSE Tanpa Batas**: Inferensi model AI yang panjang dapat di-stream tanpa terputus.
- **Auto-restart & Health Monitoring**: Fly.io otomatis me-restart kontainer bila terjadi kendala memori atau jaringan.
