# Changelog

Semua perubahan penting pada project Route-X didokumentasikan dalam berkas ini.

Format berkas ini mengacu pada prinsip [Keep a Changelog](https://keepachangelog.com/en/1.1.0/) dan mematuhi [Semantic Versioning](https://semver.org/).

---

## [v1.0.1] - 2026-09-17

### 🎨 Penyempurnaan UI/UX & Stabilitas Tampilan (Live Staging Review)
- **Requests Inspector**:
  - Perbaikan penelusuran timeline event dan payload menggunakan UUID primary key `req.id` dan parameter partisi `created_at`.
  - Penanganan anggun respon 404 payload saat retensi body request dinonaktifkan demi privasi (`catch(() => null)`), menghilangkan banner error semu dan langsung menampilkan rincian latensi waterfall, biaya, dan perintah cURL.
- **Katalog Model & Pricing**:
  - Standardisasi label tombol aksi pada mode kartu grid menjadi `Harga` (selaras dan konsisten dengan tampilan tabel).
- **Navigasi & Router Aliases**:
  - Penambahan alias rute `/upstreams/routing` dan `/webhooks` pada router aplikasi `App.tsx`.
  - Lokalisasi subjudul halaman Webhooks ke Bahasa Indonesia teknis yang konsisten.
- **Perapian Tata Letak & Kenyamanan Pengguna (PR #6 & PR #8)**:
  - Perbaikan jarak vertikal dan keterbacaan tabel di semua resolusi.
  - Pembersihan kode warna hex arbitrer menjadi token desain terstandar.
  - Transformasi responsif mobile (viewport 390px) pada daftar kartu, laci navigasi, dan dialog aksi.
- **Alur Kerja Pengembang (PR-Only Policy)**:
  - Penegakan aturan wajib Pull Request dan perlindungan cabang `main` pada aturan repositori `.agents/rules/routex-dev.md`.

---

## [v1.0.0] - 2026-09-17

### 🚀 Sorotan Rilis Resmi Perdana (Initial Release)
Route-X v1.0.0 adalah rilis resmi pertama dari gateway AI berkinerja tinggi, self-hosted, dan open-source yang dirancang untuk mengorkestrasi inferensi multi-provider, routing cerdas, tata kelola anggaran presisi, dan integrasi mulus dengan ekosistem CLI developer.

### ✨ Fitur Utama yang Dirilis
- **Mesin Dual-Protocol (OpenAI & Anthropic Native)**:
  - Dukungan 100% kompatibel OpenAI pada rute `/v1/chat/completions` (baik non-streaming maupun streaming SSE).
  - Dukungan asli Anthropic Messages API pada rute `/v1/messages` dan `/v1/v1/messages` dengan konversi format bolak-balik otomatis (roles, message parts, multimodal vision, tool use / function calling).
  - Registry katalog `/v1/models` yang terpadu dengan pemetaan dinamis ke model alias (mis. `claude-3-7-sonnet`, `claude-3-5-sonnet`, `claude-3-5-haiku`).
- **Pusat Integrasi CLI Pengembang Terpadu**:
  - Konfigurasi otomatis untuk developer tools: **Claude Code CLI**, **OpenCode CLI**, **Aider**, **Shell-GPT (SGPT)**, **Fabric**, dan **OpenCommit**.
  - Alat uji diagnostik live (probes ke rute `/v1/messages` dan `/v1/chat/completions` dengan pelaporan latensi milidetik langsung di dashboard).
  - Auto-loader shell ramah subshell (`~/.bashrc` dan `/etc/profile.d/routex.sh`) untuk penyuntikan environment instan ke terminal baru.
  - **Zero-Touch Protection**: Perlindungan mutlak dan isolasi penuh untuk Antigravity CLI (`agy`).
- **Combo Routing & Failover Engine**:
  - Algoritma routing: *Single Priority*, *Cascading Failover*, *Round Robin*, *Weighted Load Balancing*, dan *Lowest Latency EMA*.
  - *Context Window Smart Bypass*: Pengalihan proaktif jika prompt melebihi kapasitas jendela konteks provider tertentu.
  - *3-State Circuit Breaker*: Proteksi isolasi kesalahan upstream (Closed → Open → Half-Open).
- **Keamanan & Kriptografi Berlapis (Zero-Trust)**:
  - Enkripsi kredensial provider & API key menggunakan **AES-256-GCM** dengan verifikasi tamper AEAD.
  - Pengamanan kata sandi admin menggunakan **Argon2id** dengan verifikasi waktu konstan (kebal *timing attacks*).
  - Proteksi anti-CSRF ganda (*Double-Submit Cookie*) dan verifikasi header `Origin`/`Host` pada seluruh mutasi admin.
  - Pencegah SSRF level transport dialer untuk memblokir perutean ke subnet privat, loopback, dan metadata cloud (`169.254.169.254`).
- **Tata Kelola Anggaran & Pembatasan Laju (Financial Guardrails)**:
  - Pelacakan biaya komputasi hingga skala mikro 8 desimal (`0.00000001` USD).
  - Rate limiting *sliding-window* terdistribusi berbasis Redis dan skrip atomik Lua.
  - Batas pagu pengeluaran harian dan bulanan dengan fitur *hard stop* otomatis.
  - Pencatatan seluruh transaksi permintaan (latency, TTFT, input tokens, output tokens) ke tabel partisi PostgreSQL `requests`.
- **Konsol Web Tersemat (Single Binary)**:
  - Antarmuka dashboard React 18 + TailwindCSS terkompilasi dan disematkan langsung di dalam biner Go tunggal (`//go:embed`).
  - Alur *First-Run Onboarding* dengan bantuan kredensial default 1-klik dan kewajiban ganti sandi aman.

### 🛡️ Hasil Audit & Pengujian
- **Race Detector**: 0 data race terdeteksi pada seluruh pengujian `go test -race` di paket gateway, router, cliconfig, security, auth, dan apikey.
- **Transmisi Streaming SSE**: Framing terverifikasi stabil tanpa pemutusan karakter UTF-8 multibyte.
- **Integritas Skema Basis Data**: 10 migrasi skema (`0001` s/d `0010_drop_roles.sql`) terpasang bersih tanpa data yatim.

---
Hak Cipta © 2026 NexGen-X / Tim Pengembang Route-X.
