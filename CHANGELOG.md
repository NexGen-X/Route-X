# Changelog

Semua perubahan penting pada project Route-X didokumentasikan dalam berkas ini.

Format berkas ini mengacu pada prinsip [Keep a Changelog](https://keepachangelog.com/en/1.1.0/) dan mematuhi [Semantic Versioning](https://semver.org/).

---

## [Unreleased] - 2026-09-20

Penghapusan menyeluruh integrasi Xray-core dari Route-X. Keputusan diambil
berdasarkan audit risiko/manfaat di deployment produksi: integrasi Xray hanya
memberi 1 manfaat (egress SOCKS5) namun mendominasi banyak risiko (open proxy,
permukaan serangan, kompleksitas operasional, titik gagal tunggal). Egress pool
generik (HTTP/HTTPS/SOCKS5) dipertahankan penuh — proxy pihak ketiga (Cloudflare
Gateway, BrightData, VPS sendiri) tetap dapat dipakai tanpa Xray.

### Removed

- **Integrasi Xray-core dihapus seluruhnya** dari kode dan artifak deployment:
  paket `internal/xray/` (orchestrator + config generator VLESS Reality),
  `deploy/xray/`, `deploy/systemd/xray.drop-in.conf`, service `xray` di
  `docker-compose.yml`, dan instalasi binary xray-core di `install-native.sh`.
- **Sinkronisasi config Xray otomatis** (`syncXrayConfig`, `ensureXrayEgressPool`,
  `getXrayLinks`, `encodeXrayState`/`decodeXrayState`) dihapus dari `internal/admin`.
- **UI terkait Xray dihapus**: panel "Integrasi Xray-Core: VLESS Reality Stealth
  Tunnel" di halaman Settings, kartu Xray di Dashboard, dan preset Xray SOCKS5/HTTP
  di form Egress (form manual proxy tetap utuh).
- **Env `XRAY_BRIDGE_HOST`** tidak lagi dibaca (const/field config dihapus). Env
  lama pada deployment yang ada diabaikan tanpa error — aman, tapi sebaiknya
  dibersihkan dari `.env`/`docker-compose` deployment masing-masing.
- Fungsi `security.XrayStateAAD()` dihapus bersama cabang rotasinya.

### Keamanan

- **Permukaan SSRF Docker dipersempit**: entri hostname `xray` dihapus dari
  `UPSTREAM_ALLOWED_PRIVATE_ADDRS` di `docker-compose.yml` (pengecualian khusus
  container xray yang tidak lagi ada). Loopback tetap diizinkan untuk egress pool
  generik.
- **Tidak ada lagi forward proxy yang dipasang atau dijalankan** oleh skrip
  deployment resmi. Sebelumnya `install-native.sh` memasang xray-core dari
  upstream `XTLS/Xray-install` dan `deploy.sh` menjalankan container xray.

### Migration

- **`0013_drop_xray_settings.sql`**: menghapus record `settings` dengan key
  `system:xray:config` (state kredensial Xray terenkripsi yang kini yatim).
  Tabel `egress_pool` dan `domains` **tidak disentuh** — pool proxy generik
  dipertahankan utuh.

### Catatan Operasional

- Pool egress lama bernama "⚡ Xray Stealth Tunnel (Local)" (jika ada di DB
  deployment Anda, dibuat oleh `ensureXrayEgressPool` versi sebelumnya) tetap
  berfungsi sebagai SOCKS5 generik dan dapat dihapus/diperbarui via dashboard
  admin. Migrasi 0013 tidak menyentuhnya.
- Layanan `xray.service` pada host native (jika dipasang sebelumnya) menjadi
  yatim setelah perubahan ini — nonaktifkan dan bersihkan manual:
  `systemctl disable --now xray`.

---

## [Unreleased] - 2026-09-20

Tindak lanjut pembersihan setelah penghapusan Xray (#37).

### Keamanan

- **Izin `GET /api/domain` dilonggarkan dari `settings:write` ke `settings:read`**.
  Sebelumnya izin tulis diperlukan karena respons memuat tautan provisioning
  Xray yang setara kredensial. Setelah Xray dihapus, respons hanya berisi status
  infrastruktur ringan (domain, mode HTTPS, status DNS & IP server), sehingga
  cukup `settings:read`. Mutasi (`POST`/`DELETE`) tetap `settings:write`.
  (Area arsitektur Single-Admin: setiap user terautentikasi adalah Admin penuh,
  perubahan ini terutama menjaga prinsip least-privilege & konsistensi rute.)

### Tests

- **Test AAD untuk `CLIToolAAD` & `OAuthSessionAAD` ditambahkan**
  (`TestCLIToolAADDiikatKeIDTool`, `TestOAuthSessionAADDiikatKeIDSesi`).
  Kedua AAD ini adalah satu-satunya AAD settings yang masih aktif di rotasi kunci
  setelah `XrayStateAAD` dihapus, namun sebelumnya tidak punya test khusus.
  Test memverifikasi format terikat ke ID, membedakan tiap record, dan tidak
  bertabrakan dengan `CredentialAAD`/`WebhookAAD`.

---

## [v1.1.1] - 2026-09-20

Rilis perbaikan keamanan & keandalan hasil audit menyeluruh 6 fase. Semua perbaikan
kritis/high di bawah telah melewati verifikasi 4-lapis (wire protocol, backend & DB,
frontend & kontrak, live ground-truth) dengan bukti nyata.

### 🔒 Keamanan

- **[CRITICAL] Tidak ada lagi open proxy Xray di jalur runtime** (#29, #30):
  config statis maupun hasil-generate kini bind loopback (`127.0.0.1`) + wajibkan
  autentikasi (`auth: password`) pada inbound SOCKS5/HTTP. Sebelumnya gateway
  menimpa config live dengan `0.0.0.0` + `noauth` setiap sinkronisasi domain,
  mengembalikan celah tepat setelah deploy.
- **[CRITICAL] Hentikan pembocoran kata sandi admin default via `/setup-hint`** (#33):
  endpoint publik tidak lagi mengirim `default_password` plaintext. Ditambah
  fail-fast startup: admin default yang belum ganti kata sandi dicatat dengan
  severity ERROR (bukan first-run) dan akses fitur diblokir sampai diganti.
- **[HIGH] Kunci API CLI pindah ke sessionStorage** (#32): tidak lagi persisten
  tanpa batas di localStorage; migrasi sekali-jalan menghapus jejak lama.
- **[HIGH] Migrasi 0012: hapus DEFAULT hardcoded OAuth Client ID vendor** (#34):
  skema tidak lagi menyimpan client_id Google Antigravity publik sebagai default;
  insert tanpa nilai ditolak DB (NOT NULL). Data operator eksisting utuh.
- **[HIGH] Branch protection aktif pada `main`**: required status checks ketat
  (`go` + `web`), larangan force-push & deletion.

### 🛠️ Keandalan & Kebenaran

- **[CRITICAL] Streaming `tool_use` `/v1/messages`** (#26): blok SSE lengkap &
  berurutan (content_block_start → input_json_delta → stop_reason `tool_use`);
  sebelumnya terpotong di tengah JSON.
- **[HIGH] Kontrak DTO frontend ↔ backend** (#31): timeline event memakai `kind`
  (bukan `event_type`), viewer payload membaca `request_body`/`response_body`
  (sebelumnya dead code selamanya), dan chart cost Observability memplot angka
  (`cost_usd` string → number untuk recharts).
- **[CRITICAL] Deploy bisa boot** (#27): `deploy.sh` menghasilkan `ENCRYPTION_KEY`
  32-byte base64 yang valid (sebelumnya 48-byte → gateway tolak start),
  `PUBLIC_URL` https, `sslmode prefer`, dan `UPSTREAM_ALLOW_HTTP=false`.

### 🧪 Kualitas

- **[HIGH] CI menjalankan `go test -race`** (#28) plus secret-lint, permission
  minimal, dan pinned SHA untuk seluruh GitHub Actions.

### Catatan Operasional

- Migrasi 0012 bersifat DDL murni; diterapkan otomatis saat boot biner baru.
  Instance produksi yang masih memakai biner pra-v1.1.1 akan naik dari 11 → 12
  pada deploy pertama — verifikasi `schema_migrations` setelah deploy.
- Setelah deploy, layanan perlu restart satu kali agar config Xray baru tertulis
  ke disk dan biner memuat perbaikan runtime.

---

## [v1.1.0] - 2026-09-17

### 🌟 Paradigma "Low Floor, High Ceiling" & Pengalaman Personal Developer
- **Resep Cepat 1-Klik di Dashboard (Quick Setup Recipes)**:
  - Penambahan kartu resep siap pakai di Dashboard:
    - 🛠️ *Coding Asisten*: Panduan kilat dan cuplikan variabel shell untuk menghubungkan Cursor, Claude Code, Cline, atau OpenCode secara instan.
    - 💰 *Hemat Biaya 90%*: Strategi mengarahkan prompt rutin ke model murah (DeepSeek V3 / Groq LLaMA) dengan fallback otomatis ke GPT-4o / Claude 3.5 Sonnet.
    - 🛡️ *Anti-Downtime*: Panduan failover multi-upstream otomatis (< 200ms) saat provider utama mengalami gangguan atau rate limit.
- **Progressive Disclosure pada Penambahan Provider**:
  - Tampilan default dibuat super ringkas: cukup pilih logo penyedia, beri nama label, dan masukkan API Key.
  - Parameter teknis tingkat lanjut (ID Unik, Dialek Kind, Base URL kustom, Jalur Egress Proxy, Bobot, dan Timeout) disembunyikan secara rapi di balik lipatan *Pengaturan Lanjutan*.
- **Progressive Disclosure pada Perutean Cerdas (Routing Rules)**:
  - Form perutean disederhanakan: pengguna cukup memilih target model dan strategi utama.
  - Konfigurasi batas toleransi kegagalan (*max attempts*) dan jeda backoff (*backoff ms*) dirapikan ke dalam akordeon opsi toleransi lanjutan.
- **Penyatuan Pembuatan Kunci API & Alokasi Anggaran Cepat**:
  - Pengguna dapat langsung menetapkan pagu pengeluaran bulanan (USD) saat membuat Kunci API baru.
  - Penambahan chip nominal instan (`+$10`, `+$25`, `+$50`, `+$100`) untuk penetapan anggaran secepat kilat.
  - Batas kecepatan RPM (Pesan/Menit) dan TPM (Token/Menit) diorganisir dalam lipatan pembatasan lanjutan yang rapi.
- **Template Pagu Anggaran Personal di Halaman Budgets**:
  - Template 1-klik di Drawer dan kartu Empty State:
    - 🟢 *Dev Hemat ($10/bln)*: Pagu $10 dengan batas peringatan 80%.
    - 🔵 *Koding Rutin ($30/bln)*: Pagu $30 dengan batas peringatan 85%.
    - 🟣 *Power User ($100/bln)*: Pagu $100 dengan batas peringatan 90%.
- **Template Batas Laju Cepat di Halaman Rate Limits**:
  - Template 1-klik: *Dev / Sandbox (60 RPM)*, *Standar IDE (300 RPM)*, dan *High Load (1200 RPM)*.
- **Universal Clipboard Fallback**:
  - Penambahan mekanisme fallback klasik `document.execCommand('copy')` pada utilitas clipboard, menjamin tombol salin snippet dan kunci API bekerja 100% andal di seluruh lingkungan browser (Localhost, IP VPS, HTTP LAN `http://192.168.x.x:8080`, dan WebView).
- **Penyempurnaan Nada Komunikasi Antarmuka**:
  - Transformasi bahasa antarmuka menjadi ramah bagi pengembang mandiri (*personal developer*), melokalisasi subjudul dashboard, dan menjaga tata letak tetap lapang (*uncluttered*).

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
