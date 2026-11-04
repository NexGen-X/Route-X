# Route-X — AGENTS.md

AI Gateway (Go 1.27 + React/Vite). Repo: github.com/NexGen-X/Route-X, branch utama `main`.

## Struktur
- `cmd/ai-gateway/` — binary utama gateway; `cmd/routex-rotate/` — rotasi kredensial
- `internal/` — admin, apikey, auth, billing, cache, config, database (+migrations, repo, seed), gateway, models, providers, router, worker, dsb.
- `web/` — dashboard React 19 + Vite + TS (`routex-dashboard`); `docs/` — openapi.yaml + embed docs, panduan pengguna & runbook operasi

## Perintah
- Build: `go build ./...` (dari root repo)
- Test Go: `go test ./...` (butuh `TEST_DATABASE_URL` atau `DATABASE_URL` postgres:// menunjuk DB sekali pakai)
- Format: `gofmt -l .` harus kosong sebelum commit
- Web: `cd web && npm install && npm run build` (tsc + vite)

## Database & Redis
- Postgres utama: localhost:5432, DB `routex` (owner `routex`), Redis utama: localhost:6379
- Test integrasi memakai schema sekali pakai (`test_*`) + `search_path` di DSN, dibersihkan otomatis — JANGAN menulis ke schema `public` maupun DB development saat test

## Konfigurasi app & Arsitektur Identitas
- Wajib: `DATABASE_URL` (postgres://), `REDIS_URL` (redis://), `SESSION_SECRET`, `ENCRYPTION_KEY`, `API_KEY_PEPPER`
- Opsional: `INITIAL_ADMIN_EMAIL` (bawaan `admin@routex.local`); `INITIAL_ADMIN_PASSWORD` wajib disetel agar seed membuat admin pertama (dengan `MustChangePassword: true`) — jika kosong, TIDAK ada admin yang dibuat dan tidak ada password default.
- Arsitektur identitas dua tingkat (B-102): peran RBAC (`roles`, `permissions`, `user_roles`) telah dihapus (migrasi 0010), tetapi sejak migrasi 0020 kewenangan dibedakan oleh kolom `users.is_admin` — `true` = `Admin` dengan hak penuh (`*`); `false` (bawaan pembuatan pengguna) = pengguna biasa tanpa izin admin, ditolak 403 di rute admin. Bidang `role` (`admin`/`user`) pada create/update user adalah satu-satunya jalur mengubahnya, selain bootstrap admin pertama yang eksplisit admin.
- Endpoint `GET /api/auth/setup-hint` mendeteksi status onboarding awal dan dinonaktifkan otomatis begitu kata sandi diperbarui.
- Tidak ada `.env` di repo — baca `internal/config/config.go` untuk daftar env lengkap

## Aturan
- **Pedoman Aturan & Tata Kelola Agen Resmi**:
  - [Aturan Orkestrasi & Koordinasi Multi-Agent](.agents/rules/multi_agent_orchestration.md) — Tata kelola resmi koordinasi multi-agent, katalog 25 peran sub-agent Route-X (master prompt spesialis yang benar-benar ada: 7 berkas di `.agents/prompts/`), pipeline orkestrasi 4-tahap, format serah terima pesan 5-seksi (`send_message`), isolasi branch Git kerja vs koordinasi komunikasi aktif, dan zero-any TypeScript.
  - [Aturan Pengembangan & Standar Kerja (routex-dev)](.agents/rules/routex-dev.md) — Konvensi Go & TS, zero-race, arsitektur single-admin, dan keamanan zero-trust.
  - [Standar Verifikasi Multi-Aspek](.agents/rules/multi_aspect_verification.md) — 4 lapisan verifikasi empiris, wire protocol live probe, dan protokol anti-halusinasi.
- Bahasa komentar/log di codebase ini: Indonesia. Ikuti gaya yang ada.
- Migrasi baru: tambah file `internal/database/migrations/NNNN_nama.sql` berurutan (terakhir saat ini: 0021)
- **Alur Kerja Git Wajib PR (PR-Only Policy)**: Dilarang keras melakukan komit atau push langsung ke branch `main`. Setiap perubahan wajib dibuat pada branch terisolasi (`feat/*`, `fix/*`, `chore/*`), didorong ke remote, diajukan melalui `gh pr create`, dipantau hingga status CI hijau (`gh pr checks <id> --watch`), dan digabungkan melalui `gh pr merge <id> --squash --delete-branch`.
- **Standar Verifikasi Multi-Aspek & Protokol 7-Layer QA (Wajib Bukti Nyata)**:
  - Dilarang keras menyajikan hasil mentah, asumsi, atau klaim penyelesaian tugas hanya berdasarkan spekulasi kode atau mock lokal semata.
  - Setiap integrasi sistem luar (LLM provider, proxy tunnel, cloud gateway, API eksternal) wajib dibedah dan diverifikasi perilakunya secara nyata terhadap spesifikasi resmi dan wire protocol (uji curl/probe live) sebelum diimplementasikan.
  - Wajib membedakan secara tegas antara Forward Proxy (L4/L7 HTTP CONNECT tunnel) dengan Reverse Proxy (L7 BaseURL routing). Keduanya tidak boleh dicampuradukkan.
  - Setiap penyetoran hasil pekerjaan wajib membuktikan **Protokol 7-Layer QA**:
    1. **Layer 1: Frontend Type & Unit Tests** — Kompilasi `npm run build` bebas error TypeScript (0 errors) dan seluruh test suite lulus (`npm test`).
    2. **Layer 2: Backend Concurrency & Races** — Audit konkurensi Go bebas data race (`go test -race ./...`).
    3. **Layer 3: Theme Tokens, Design Hygiene, & Responsiveness** — Kepatuhan token warna Tailwind/CSS, UI bersih, dan responsif lintas ukuran viewport.
    4. **Layer 4: Live Gateway Wire-Protocol Probe** — Uji probe live (`./scripts/probe_e2e_gateway.sh`) untuk rute completions non-streaming & SSE streaming serta headers protokol.
    5. **Layer 5: Target Component Functional Integrity** — Integritas logika fungsional, state management, form handlers, dan edge cases pada komponen target.
    6. **Layer 6: Security & Zero-Leakage Sanitization** — Penanganan kredensial aman (`security.Secret`), redaksi data sensitif tak diekspor, proteksi CSRF & token sanitization.
    7. **Layer 7 (Wajib): Cross-Module Blast Radius & Data-Flow Integration** — Verifikasi alur data menyeluruh lintas modul (Database -> Backend API -> Routing Engine -> Web Pages/CLI Integrations/Observability) guna mencegah putusnya data flow pada komponen dependen.
- **Aturan Wajib Operasional & Komunikasi**:
  - **Zero-Hallucination & Empirical Proof**: Dilarang keras berasumsi atau mengklaim fungsionalitas berjalan tanpa bukti eksekusi riil (output terminal nyata, log status HTTP, DTO query aktual).
  - **Pelaporan Kejanggalan di Luar Scope**: Jika menemukan kejanggalan, anomali, kode usang/rapuh, atau risiko regresi di luar cakupan tugas saat ini, agent WAJIB mencatat dan menyajikannya dalam seksi khusus *[TEMUAN DI LUAR SCOPE & KEJANGGALAN SISTEM]* pada laporan.
  - **Konsultasi Antar-Agent**: Komunikasi antar-agent (coder, tester, reviewer, orchestrator) wajib menggunakan format baku, bertukar temuan empiris, dan tidak membuat asumsi sepihak tanpa validasi lintas peran.
- Jangan commit secret; jangan ubah `go.mod` tanpa alasan.
- MCP yang biasa terpasang di lingkungan kerja repo ini (ketersediaan bergantung mesin — verifikasi sebelum mengandalkan):
  - `codebase-memory-mcp`: Knowledge Graph & AST indeks kode Go & TS (`search_graph`, `trace_path`, `get_code_snippet`)
  - `postgres`: Akses langsung inspeksi DB `routex` (localhost:5432)
  - `redis`: Akses langsung inspeksi cache & sliding-window rate limit (localhost:6379)
  - `puppeteer`: Browser headless untuk validasi UI konsol web React (`web/`)

## Prosedur Rilis & Patch Modern Route-X (Checklist Pra-Rilis)
Sebelum mempublikasikan patch atau rilis baru, lakukan verifikasi berurutan berikut:
1. **Audit Data Race**: Jalankan `go test -race ./...` (wajib 0 data race terdeteksi).
2. **Formatting Go**: Jalankan `gofmt -l .` (wajib kosong/bersih).
3. **Integritas Database**: Pastikan berkas migrasi `internal/database/migrations/` berurutan rapi (saat ini 0001 sampai 0021) dan tidak merusak skema produksi.
4. **Build Frontend**: Jalankan `cd web && npm run build && npm test` (wajib kompilasi `tsc`, `vite build`, dan unit test lolos tanpa error).
5. **Uji Probing Gateway E2E**: Jalankan `./scripts/probe_e2e_gateway.sh` untuk memvalidasi rute `/v1/chat/completions` (non-stream & SSE stream) serta `/v1/messages` native Anthropic.
6. **Validasi Observabilitas**: Jalankan `./scripts/check_metrics.sh` untuk memverifikasi endpoint `/metrics` tidak merekam kegagalan terselubung.
7. **Sinkronisasi Versi**:
   - Perbarui versi pada `web/package.json` sesuai target rilis (misal `1.5.2`).
   - Tambahkan catatan perubahan rilis pada `CHANGELOG.md` di bawah seksi versi terkait (pindahkan isi `[Unreleased]` yang relevan).
   - Perbarui badge versi di `README.md` dan fallback versi di `Makefile` serta skrip instalasi/operasional.
8. **Git Tagging & Publikasi**:
   - Lakukan commit perubahan: `git commit -m "release: vX.Y.Z - ..."`
   - Buat tag git: `git tag -a vX.Y.Z -m "Release vX.Y.Z"`
   - Buat release via GitHub CLI: `gh release create vX.Y.Z --title "vX.Y.Z" --notes-file ...`
   - Terbitkan checksum SATU berkas per tarball di rilis yang sama: `sha256sum routex-vX.Y.Z-linux-amd64.tar.gz > routex-vX.Y.Z-linux-amd64.tar.gz.sha256` (ulangi untuk arm64), lalu unggah tiap `.sha256` sebagai aset rilis — installer native mengunduh `<tarball>.sha256` dari rilis dan memverifikasinya; tanpa berkas ini installer hanya memperingatkan keras.
