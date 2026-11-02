# Panduan Deployment Route-X ke Render (render.com)

Dokumen ini memandu deployment Route-X ke platform cloud **Render** menggunakan **Blueprint (`render.yaml`)**.

Arsitektur di Render:
- **Web Service (Docker)**: Menjalankan binary Go `ai-gateway` yang meng-embed frontend SPA React Vite.
- **Managed PostgreSQL**: Basis data primer, auto-migrasi schema, dan penyiapan akun admin awal saat start.
- **Managed Redis**: Rate limiter dan state terdistribusi gateway, termasuk penyimpanan *response cache* — fitur opsional yang **bawaan nonaktif** dan diaktifkan operator dari konsol (Pengaturan → Performa & Cache) dengan isolasi per API key.

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
- `INITIAL_ADMIN_PASSWORD`: Kata sandi awal yang kuat (WAJIB agar admin pertama dibuat; tidak ada default).

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
   - `INITIAL_ADMIN_EMAIL`: (Opsional) Masukkan email admin Anda; bawaan `admin@routex.local`.
   - `INITIAL_ADMIN_PASSWORD`: **Wajib diisi** agar akun admin pertama dibuat. Tidak ada password default — bila kosong, tidak ada admin yang dibuat sama sekali.
6. Klik **Apply**. Render akan mulai membangun container Docker dan menginisialisasi database serta Redis.

Catatan: `render.yaml` tidak mendeklarasikan `ANTIGRAVITY_CLIENT_SECRET`. Bila Anda memakai integrasi Antigravity (Google OAuth), tambahkan variabel itu secara manual di Render Dashboard → Environment — client secret dibaca dari environment saja, tidak tertanam di biner. Daftar variabel lengkap: `.env.render.example`.

---

## 4. Proses Build & Startup Otomatis

Selama proses deploy:
1. **Stage 1 (Frontend Build)**: Render menjalankan `npm ci` dan `npm run build` di dalam Node 24.
2. **Stage 2 (Backend Build)**: Binary Go `ai-gateway` (Go 1.27.3) dikompilasi dengan menyematkan aset `web/dist` ke dalamnya.
3. **Stage 3 (Runtime)**: Image Alpine 3.24 minimalis dijalankan (memuat `postgresql-client`, yang dibutuhkan fitur Backup/Restore admin).
4. **Bootstrapping**:
   - `ai-gateway` otomatis terhubung ke `DATABASE_URL` dan menjalankan migrasi skema database (migrasi `0001` sampai `0019`).
   - `seed.Run` membuat akun Admin pertama hanya jika tabel pengguna kosong DAN `INITIAL_ADMIN_PASSWORD` disetel (email dari `INITIAL_ADMIN_EMAIL`, bawaan `admin@routex.local`; tanpa password, tidak ada admin yang dibuat).
   - Server mendengarkan pada port yang diatur Render (`PORT=10000`).

---

## 5. Verifikasi Deployment

Setelah status di Render Dashboard berubah menjadi **Live**:

1. **Uji Health Check**:
   ```bash
   curl -i https://route-x.onrender.com/healthz
   ```
   Respons yang diharapkan (endpoint publik hanya menyajikan status — version/commit/uptime tidak lagi diekspos):
   ```
   HTTP/2 200
   {"status":"ok"}
   ```
   Untuk kesiapan dependensi gunakan `/readyz`: respons `{"status":"ok","checks":{...}}` dengan status per dependensi (PostgreSQL kritis; Redis non-kritis — gagalnya menghasilkan status `degraded` tetapi tetap HTTP 200).

2. **Masuk ke Konsol Administrasi**:
   - Buka browser ke `https://route-x.onrender.com/login`.
   - Masuk menggunakan email dan kata sandi yang Anda tentukan pada `INITIAL_ADMIN_EMAIL` & `INITIAL_ADMIN_PASSWORD`.

---

## 6. Pengaturan Domain Kustom (Opsional)

Jika Anda ingin menggunakan domain Anda sendiri (misal `ai.domainanda.com`):
1. Buka layanan `route-x` di Render Dashboard -> **Settings** -> **Custom Domains**.
2. Tambahkan domain Anda dan ikuti panduan CNAME/DNS yang diberikan oleh Render (Render otomatis menyediakan sertifikat TLS Let's Encrypt).
3. Perbarui variabel lingkungan `PUBLIC_URL` di Render ke `https://ai.domainanda.com`.
