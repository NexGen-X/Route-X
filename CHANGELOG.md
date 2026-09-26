# Changelog

Semua perubahan penting pada project Route-X didokumentasikan dalam berkas ini.

Format berkas ini mengacu pada prinsip [Keep a Changelog](https://keepachangelog.com/en/1.1.0/) dan mematuhi [Semantic Versioning](https://semver.org/).

## [v1.3.1] - 2026-09-26

Rilis pembaruan kualitas dan performa yang menghadirkan penguatan aksesibilitas WCAG 2.1 Level AA, remediasi menyeluruh sistem notifikasi & toast, implementasi benchmark suite resmi backend Go, audit integritas skema database PostgreSQL, dan audit kesiapan pengujian End-to-End (E2E) Playwright.

### Added
- **Benchmark Suite Resmi Backend Go (PR #78)**:
  - Implementasi 12 benchmark mikro terverifikasi untuk mengukur performa hot-path komponen inti gateway Route-X pada arsitektur Intel Xeon Platinum 8259CL @ 2.50GHz:
    - `BenchmarkRule_Matches`: 23.05 ns/op, 0 B/op, 0 allocs/op (~43.4M ops/sec).
    - `BenchmarkDeterministicTieBreaker`: 35.13 ns/op, 0 B/op, 0 allocs/op (~28.4M ops/sec) via FNV-1a 64-bit jitter computation.
    - `BenchmarkMemoryLimiter_Allow`: 87.08 ns/op, 0 B/op, 0 allocs/op (~11.5M ops/sec) in-memory token bucket limiter.
    - `BenchmarkMemoryLimiter_AllowParallel`: 118.9 ns/op, 0 B/op, 0 allocs/op (~8.4M ops/sec) pada konkurensi multi-goroutine.
    - `BenchmarkComboPipeline_Evaluate`: 132.9 ns/op, 32 B/op, 1 allocs/op (~7.5M ops/sec) untuk evaluasi cascade multi-model combo pipeline.
    - `BenchmarkSelectCandidate_Priority`: 154.7 ns/op, 48 B/op, 1 allocs/op (~6.5M ops/sec) pada seleksi kandidat berjenjang.
    - `BenchmarkFormatKey`: 69.80 ns/op, 16 B/op, 1 allocs/op (~14.3M ops/sec) formatting dan masking hint visual.
    - `BenchmarkHash`: 557.1 ns/op, 176 B/op, 3 allocs/op cryptographic digest hashing kunci API.
    - `BenchmarkSelectCandidate_RoundRobin`: 449.2 ns/op, 88 B/op, 4 allocs/op.
    - `BenchmarkSelectCandidate_LowestLatency`: 624.4 ns/op, 48 B/op, 1 allocs/op.
    - `BenchmarkSelectCandidate_LowestCost`: 640.9 ns/op, 48 B/op, 1 allocs/op.
    - `BenchmarkSelectCandidate_Weighted`: 1,303 ns/op, 48 B/op, 1 allocs/op.
- **Audit Integritas Skema Database SQL (FIND-DB-01 & FIND-DB-02)**:
  - Validasi menyeluruh 15 berkas migrasi PostgreSQL native (`0001_initial_schema.sql` s.d. `0015_credential_egress_pool.sql`).
  - Konfirmasi kepatuhan transaksi atomik migrasi, integritas foreign key cascade, dan audit performa indeks partisi waktu (`request_logs`).
  - Pembakuan tata kelola Standar Operasional Prosedur (SOP) evolusi skema masa depan yang diuji coba pada migrasi `0016_expand_model_capabilities.sql`.
- **Audit Kesiapan Pengujian End-to-End (E2E) Playwright**:
  - Arsitektur deduplikasi autentikasi (`auth.setup.ts`): Setup login tepat 1x untuk Admin dan Viewer, menyimpan session reuse ke `.auth-admin.json` dan `.auth-viewer.json` guna melindungi rate limit login (20 request/IP per 15 menit).
  - Verifikasi kesiapan lintas 5 peramban: Desktop Chromium, Desktop Firefox, Desktop Safari (WebKit), Mobile Chrome (Pixel 7), dan Mobile Safari (iPhone 14).
  - Cakupan 7 file skenario pengujian komprehensif: `aksi-tulis.spec.ts` (CSRF & Origin mutation cleanup), `alur-inti.spec.ts`, `auth.setup.ts`, `dashboard-latensi.spec.ts`, `menyeluruh.spec.ts` (16 rute navigasi), `mobile-responsif.spec.ts` (drawer & 0 horizontal overflow), dan `sisa-kritis.spec.ts` (CRUD providers, egress, users, filter).

### Changed
- **Remediasi Menyeluruh Sistem Notifikasi & Toast (PR #76)**:
  - Kenaikan z-index kontainer menjadi `z-[9999]` guna mencegah tumpang-tindih visual dengan Modal dan Drawer.
  - Penambahan padding dinamis `top-[max(1rem,env(safe-area-inset-top))]` untuk mendukung perangkat berlayar poni / notch.
  - Perbaikan animasi progress bar countdown yang macet di 100% menggunakan animasi CSS keyframe dari 100% ke 0% selama durasi toast.
  - Implementasi interaksi Pause on Hover (`onMouseEnter` & `onMouseLeave`) untuk membekukan timer dan animasi progress bar secara mulus.
  - Sentralisasi penanganan error tipe aman melalui utilitas `getErrorMessage(err: unknown, fallback?)` di `web/src/utils/error.ts` dengan 30 skenario unit test.
  - Lokalisasi teknis dan pembersihan pola rapuh `(err.message || err)` pada halaman APIKeys, Diagnostics, Webhooks, Users, dan RuleProvidersDrawer.
- **Penguatan Aksesibilitas WCAG 2.1 Level AA (PR #77 - FIND-FE-05)**:
  - Integrasi `FocusTrap` dan WAI-ARIA Combobox pada Command Palette (`role="dialog"`, `aria-modal="true"`, `role="combobox"`, `aria-controls="command-palette-results"`, `role="listbox"`, `role="option"`, `aria-selected`).
  - Standardisasi `aria-label` deskriptif pada seluruh tombol icon-only di seluruh aplikasi (ModelsTab, CredentialsTab, Providers, BudgetCard, Observability, CLIToolCard, CLIApiKeyToolbar).
  - Perbaikan asosiasi `<label htmlFor>` dan `<input id>` eksplisit pada formulir (Webhooks, Users, CreateProviderModal, Select).
  - Penambahan `aria-hidden="true"` pada seluruh ikon Lucide SVG dekoratif untuk mencegah polusi narasi screen reader.
  - Implementasi WAI-ARIA landmark navigation `<nav aria-label="Navigasi Utama">`, `aria-current="page"`, `role="tablist"`, `role="tab"`, dan status `role="alert"` pada `PageErrorBoundary`.
- **Penyempurnaan Dokumentasi Enterprise**:
  - Perombakan total `README.md` dengan badge modern, matriks kinerja benchmark, diagram arsitektur Mermaid interaktif, dan panduan deployment komprehensif.

---

## [v1.3.0] - 2026-09-26

Rilis besar yang menghadirkan dekomposisi arsitektur modular pada seluruh halaman frontend, pencapaian Strict Typing 0 `any`, test suite komprehensif (137 tests passing), kodifikasi resmi tata kelola orkestrasi 13 sub-agent, serta optimasi backend router Go.

### Added
- **Dekomposisi Arsitektur Modular Frontend (9 Halaman Monolitik)**:
  - Dekomposisi struktural menyeluruh dari komponen monolitik menjadi sub-komponen modular, terisolasi, dan mudah dipelihara pada seluruh 9 halaman:
    - **Providers**: Dekomposisi kartu provider, modal multi-akun/kredensial, dan drawer manajemen model.
    - **RoutingRules**: Pemisahan visualizer graph perutean, modal form strategi, dan tabel aturan perutean.
    - **CLIIntegrations**: Modularisasi runner probe latensi, kartu konfigurasi CLI tool, dan modal instruksi snippet.
    - **Dashboard**: Ekstraksi metrik kartu ringkasan, grafik lalu lintas & biaya, quick recipes, serta feed aktivitas.
    - **Egress**: Modularisasi kartu pool egress, modal form proksi SOCKS5/HTTP, dan tabel status kesehatan.
    - **Requests**: Pemisahan visualizer waterfall latensi, inspector payload request/response, dan tabel log terpartisi.
    - **Settings**: Dekomposisi formulir konfigurasi runtime, manajemen kunci enkripsi, dan visualizer domain/infrastruktur.
    - **Budgets**: Pemisahan kartu ringkasan pagu pengeluaran, progress bar kuota mikro, dan modal template anggaran.
    - **Models**: Dekomposisi tabel katalog model upstream, visualizer harga/token, dan modal sinkronisasi.
- **Strict Typing 0 `any` Frontend & SDK Client**:
  - Eliminasi total 100% tipe `any` di seluruh komponen, hooks, utilitas, dan antarmuka TypeScript frontend (`tsc -b` zero error).
  - Standardisasi tipe data kontraktual pada SDK client `web/src/api/client.ts` dengan generic terdefinisi penuh, payload validation aman, dan deklarasi ambient window modal.
- **Test Suite Komprehensif (137 Tests Passing Lintas 12 Test Files)**:
  - Penambahan unit dan integration test suite berbasis Vitest dan React Testing Library dengan 137 pengujian passing 100% di 12 berkas tes.
- **Kodifikasi Tata Kelola Multi-Agent Orchestration**:
  - Kodifikasi resmi tata kelola orkestrasi koordinasi 13 sub-agent pada berkas `.agents/rules/multi_agent_orchestration.md` untuk standardisasi protokol handoff, bounded scope, pembagian peran, dan jaminan kualitas otomatis.

### Changed
- **Optimasi Backend Router Go & Audit Ratelimit**:
  - Penggantian pseudorandom jitter dengan **FNV-1a deterministic hash tie-breaker** pada algoritma weighted routing untuk mencegah fluktuasi distribusi beban pada bobot berimbang.
  - Audit sinkronisasi ratelimit Redis multi-replica: verifikasi konsistensi evaluasi token bucket sliding-window atomik pada lingkungan cluster dan multi-replica.

---

## [v1.2.1] - 2026-09-24

Rilis pembaruan antarmuka terpadu, isolasi jalur keluar per-akun, eliminasi selektor native mobile, serta penyederhanaan navigasi Route-X.

### Added
- **Universal Multi-Account & Multi-Credential Pooling (#50, #51)**:
  - Deteksi cerdas preset terdaftar (`matchExistingProvider`): otomatis mengarahkan ke mode penambahan akun ke pool provider yang sudah ada tanpa menduplikasi entitas provider.
  - Segregasi presisi antara Google Antigravity (`antigravity-deepmind`) dan Google Gemini (`google-gemini`) pada form kredensial OAuth.
  - Dukungan multi-akun OAuth dan API Key dengan input nama akun deskriptif serta shortcut `+ Akun / Key` langsung dari kartu provider.
- **Per-Account Egress Pool Isolation & Migrasi Skema (#52)**:
  - Migration `0015_credential_egress_pool.sql`: menambahkan kolom `egress_pool_id` opsional pada tabel `provider_credentials`.
  - Setiap akun atau kunci API kini dapat memiliki jalur keluar (proxy/direct) terisolasi secara independen.
  - Runtime proxy gateway memprioritaskan egress credential jika dikonfigurasi, dengan fallback ke default provider egress pool.
- **Universal Themed Picker UI/UX (#53)**:
  - Komponen `Select.tsx` kustom terpadu dengan varian `form` dan `compact`.
  - Mode responsif mobile (< 640px): Custom Bottom Sheet Route-X via React Portal dengan drag handle, blur backdrop, indikator radio `#BEF264`, safe-area insets, dan body scroll locking.
  - Mode desktop (≥ 640px): Floating glassmorphism popover dengan navigasi keyboard lengkap.
  - Menggantikan 100% elemen native HTML `<select>` di seluruh aplikasi, mengeliminasi picker bawaan Android/browser ungu/abu-abu.

### Changed
- **Streamline Navigasi Dashboard (#54)**:
  - Menghapus menu redundan "Models & Pricing" dari Sidebar dan Command Palette (`Cmd+K`).
  - Pengelolaan dan sinkronisasi model kini terpusat langsung di dalam Drawer masing-masing Provider.
  - Rute warisan `/models` dan `/upstreams/models` dialihkan secara mulus (*graceful redirect*) ke halaman Providers tanpa broken route.

---

## [v1.2.0] - 2026-09-21

Rilis yang menghapus integrasi Xray-core secara menyeluruh (#37) beserta
tindak lanjut hardening (#38). Keputusan diambil berdasarkan audit
risiko/manfaat di deployment produksi: integrasi Xray hanya memberi 1 manfaat
(egress SOCKS5) namun mendominasi banyak risiko (open proxy, permukaan
serangan, kompleksitas operasional, titik gagal tunggal). Egress pool generik
(HTTP/HTTPS/SOCKS5) dipertahankan penuh — proxy pihak ketiga (Cloudflare
Gateway, BrightData, VPS sendiri) tetap dapat dipakai tanpa Xray.

### ⚠️ BREAKING CHANGES

- **Env `XRAY_BRIDGE_HOST` dihapus** dari config; tidak lagi dibaca saat boot.
  Env lama pada deployment yang ada diabaikan tanpa error (aman), tapi sebaiknya
  dibersihkan dari `.env`/`docker-compose` deployment masing-masing.
- **Service `xray` dihapus dari `docker-compose.yml`** dan `deploy.sh` tidak
  lagi menjalankannya. Deployment Docker harus `docker compose down` sebelum
  upgrade untuk menghindari container yatim.
- **`install-native.sh` tidak lagi memasang xray-core** dari upstream
  `XTLS/Xray-install`. `xray.service` pada host native yang dipasang sebelumnya
  menjadi yatim — nonaktifkan manual: `systemctl disable --now xray`.
- **Migration `0013_drop_xray_settings.sql`** menghapus record `settings` key
  `system:xray:config`. Tabel `egress_pool` dan `domains` **tidak disentuh**.

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
- Fungsi `security.XrayStateAAD()` dihapus bersama cabang rotasinya.

### Keamanan

- **Permukaan SSRF Docker dipersempit**: entri hostname `xray` dihapus dari
  `UPSTREAM_ALLOWED_PRIVATE_ADDRS` di `docker-compose.yml` dan `.env.example`
  (pengecualian khusus container xray yang tidak lagi ada). Loopback tetap
  diizinkan untuk egress pool generik.
- **Tidak ada lagi forward proxy yang dipasang atau dijalankan** oleh skrip
  deployment resmi. Sebelumnya `install-native.sh` memasang xray-core dari
  upstream `XTLS/Xray-install` dan `deploy.sh` menjalankan container xray.
- **Izin `GET /api/domain` dilonggarkan dari `settings:write` ke `settings:read`**
  (#38). Sebelumnya izin tulis diperlukan karena respons memuat tautan
  provisioning Xray yang setara kredensial. Setelah Xray dihapus, respons hanya
  berisi status infrastruktur ringan (domain, mode HTTPS, status DNS & IP
  server), sehingga cukup `settings:read`. Mutasi (`POST`/`DELETE`) tetap
  `settings:write`. (Arsitektur Single-Admin: setiap user terautentikasi adalah
  Admin penuh; perubahan ini menjaga prinsip least-privilege & konsistensi rute.)

### Tests

- **Test AAD untuk `CLIToolAAD` & `OAuthSessionAAD` ditambahkan** (#38)
  (`TestCLIToolAADDiikatKeIDTool`, `TestOAuthSessionAADDiikatKeIDSesi`).
  Kedua AAD ini adalah satu-satunya AAD settings yang masih aktif di rotasi kunci
  setelah `XrayStateAAD` dihapus, namun sebelumnya tidak punya test khusus.
  Test memverifikasi format terikat ke ID, membedakan tiap record, dan tidak
  bertabrakan dengan `CredentialAAD`/`WebhookAAD`.

### Catatan Operasional

- Pool egress lama bernama "⚡ Xray Stealth Tunnel (Local)" (jika ada di DB
  deployment Anda, dibuat oleh `ensureXrayEgressPool` versi sebelumnya) tetap
  berfungsi sebagai SOCKS5 generik dan dapat dihapus/diperbarui via dashboard
  admin. Migrasi 0013 tidak menyentuhnya.

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
