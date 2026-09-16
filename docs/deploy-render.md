# Panduan Deployment Route-X ke Render (render.com)

Dokumen ini memandu deployment Route-X ke platform cloud **Render** menggunakan **Blueprint (`render.yaml`)**.

Arsitektur di Render:
- **Web Service (Docker)**: Menjalankan binary Go `ai-gateway` yang meng-embed frontend SPA React Vite.
- **Managed PostgreSQL**: Basis data primer, auto-migrasi schema, dan penyiapan akun admin awal saat start.
- **Managed Redis**: Distributed cache, rate limiter per-IP/per-email, dan response caching.

---

## 1. Prasyarat

1. Akun aktif di [Render.com](https://render.com).
2. Repositori GitHub Route-X yang sudah terhubung ke akun Render Anda.
3. OpenSSL di terminal lokal untuk menghasilkan kunci kriptografi.

---

## 2. Generate Kunci Kriptografi

Sebelum membuat layanan di Render, hasilkan kunci keamanan produksi dengan menjalankan skrip berikut di mesin lokal:

```bash
bash scripts/ops/generate-render-env.sh
```

Skrip ini akan mencetak format konfigurasi siap salin:
- `SESSION_SECRET`: Token rahasia penandatanganan cookie sesi (min 32 karakter).
- `ENCRYPTION_KEY`: Kunci AES-256-GCM (tepat 32-byte Base64).
- `API_KEY_PEPPER`: Pepper hash kunci API (min 32-byte Base64).
- `METRICS_TOKEN`: Token autentikasi scraper Prometheus.
- `INITIAL_ADMIN_EMAIL`: Email administrator awal (opsional, default: `admin@routex.local`).
- `INITIAL_ADMIN_PASSWORD`: Kata sandi awal yang kuat (opsional, default: `RouteX#Initial2026!`).

---

## 3. Deployment Menggunakan Render Blueprint (Rekomendasi)

Blueprint `render.yaml` di repositori ini secara otomatis mendirikan Web Service, Managed PostgreSQL, dan Managed Redis secara terpadu.

1. Buka **[Render Dashboard](https://dashboard.render.com/)**.
2. Klik tombol **New +** di pojok kanan atas, lalu pilih **Blueprint**.
3. Hubungkan repositori GitHub **Route-X**.
4. Render akan otomatis mendeteksi berkas `render.yaml` dan menampilkan rencana sumber daya:
   - **Service**: `route-x` (Web Service - Docker)
   - **Database**: `route-x-db` (PostgreSQL)
   - **Service**: `route-x-redis` (Redis)
5. Isi variabel lingkungan (*Environment Variables*) yang ditandai wajib di layar:
   - `PUBLIC_URL`: URL layanan Render Anda (contoh: `https://route-x.onrender.com` atau domain kustom Anda).
   - `ENCRYPTION_KEY`: Masukkan nilai dari skrip langkah 2.
   - `API_KEY_PEPPER`: Masukkan nilai dari skrip langkah 2.
   - `INITIAL_ADMIN_EMAIL`: (Opsional) Masukkan email admin Anda atau biarkan default.
   - `INITIAL_ADMIN_PASSWORD`: (Opsional) Masukkan kata sandi admin Anda atau biarkan default.
6. Klik **Apply**. Render akan mulai membangun container Docker dan menginisialisasi database serta Redis.

---

## 4. Proses Build & Startup Otomatis

Selama proses deploy:
1. **Stage 1 (Frontend Build)**: Render menjalankan `npm ci` dan `npm run build` di dalam Node 20.
2. **Stage 2 (Backend Build)**: Binary Go `ai-gateway` dikompilasi dengan menyematkan aset `web/dist` ke dalamnya.
3. **Stage 3 (Runtime)**: Image Alpine Linux minimalis (~25MB) dijalankan.
4. **Bootstrapping**:
   - `ai-gateway` otomatis terhubung ke `DATABASE_URL` dan menjalankan migrasi skema database (`0001_initial.sql` sampai `0010_drop_roles.sql`).
   - `seed.Run` otomatis membuat akun Admin pertama jika tabel pengguna masih kosong (menggunakan `INITIAL_ADMIN_*` jika disetel, atau default onboarding `admin@routex.local` / `RouteX#Initial2026!`).
   - Server mendengarkan pada port yang diatur Render (`PORT=10000`).

---

## 5. Verifikasi Deployment

Setelah status di Render Dashboard berubah menjadi **Live**:

1. **Uji Health Check**:
   ```bash
   curl -i https://route-x.onrender.com/healthz
   ```
   Respons yang diharapkan:
   ```json
   HTTP/2 200
   {"status":"ok","version":"dev","commit":"none","uptime_seconds":15}
   ```

2. **Masuk ke Konsol Administrasi**:
   - Buka browser ke `https://route-x.onrender.com/login`.
   - Masuk menggunakan email dan kata sandi yang Anda tentukan pada `INITIAL_ADMIN_EMAIL` & `INITIAL_ADMIN_PASSWORD`, atau gunakan tombol **"Gunakan Kredensial Default (1-Klik)"** pada kartu onboarding jika menggunakan kredensial bawaan (`admin@routex.local` / `RouteX#Initial2026!`).

---

## 6. Pengaturan Domain Kustom (Opsional)

Jika Anda ingin menggunakan domain Anda sendiri (misal `ai.domainanda.com`):
1. Buka layanan `route-x` di Render Dashboard -> **Settings** -> **Custom Domains**.
2. Tambahkan domain Anda dan ikuti panduan CNAME/DNS yang diberikan oleh Render (Render otomatis menyediakan sertifikat TLS Let's Encrypt).
3. Perbarui variabel lingkungan `PUBLIC_URL` di Render ke `https://ai.domainanda.com`.
