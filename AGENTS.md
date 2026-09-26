# Route-X — AGENTS.md

AI Gateway (Go 1.27 + React/Vite). Repo: github.com/NexGen-X/Route-X, branch utama `main`.

## Struktur
- `cmd/ai-gateway/` — binary utama gateway; `cmd/routex-rotate/` — rotasi kredensial
- `internal/` — admin, apikey, auth, billing, cache, config, database (+migrations, repo, seed), gateway, models, providers, router, worker, dsb.
- `web/` — dashboard React 18 + Vite + TS (`routex-dashboard`); `docs/` — openapi.yaml + embed docs

## Perintah
- Build: `cd /root/Route-X && go build ./...`
- Test Go: `go test ./...` (butuh `TEST_DATABASE_URL` atau `DATABASE_URL` postgres:// menunjuk DB sekali pakai)
- Format: `gofmt -l .` harus kosong sebelum commit
- Web: `cd web && npm install && npm run build` (tsc + vite)

## Database & Redis
- Postgres utama: localhost:5432, DB `routex` (owner `routex`), Redis utama: localhost:6379
- Test integrasi memakai schema sekali pakai (`test_*`) + `search_path` di DSN, dibersihkan otomatis — JANGAN menulis ke schema `public` maupun DB development saat test
- Role terbatas tersedia: `hermes_reader` (SELECT-only di `routex`), `hermes_tester` (CREATEDB, DB `routex_test`)
- Smoke env ephemeral lama ada di `/var/tmp/routex-m8-local-20260908/` (pg 51885, redis 45193) — jangan dipakai untuk kerja baru

## Konfigurasi app & Arsitektur Identitas
- Wajib: `DATABASE_URL` (postgres://), `REDIS_URL` (redis://), `SESSION_SECRET`, `ENCRYPTION_KEY`, `API_KEY_PEPPER`
- Opsional: `INITIAL_ADMIN_EMAIL`, `INITIAL_ADMIN_PASSWORD` (jika kosong, seed otomatis membuat default admin `admin@routex.local` / `RouteX#Initial2026!` dengan `MustChangePassword: true`)
- Arsitektur Single-Admin: Seluruh peran RBAC (`roles`, `permissions`, `user_roles`) telah dihapus (migrasi 0010). Setiap user terautentikasi adalah `Admin` dengan hak penuh (`*`).
- Endpoint `GET /api/auth/setup-hint` mendeteksi status onboarding awal dan dinonaktifkan otomatis begitu kata sandi diperbarui.
- Tidak ada `.env` di repo — baca `internal/config/config.go` untuk daftar env lengkap

## Aturan
- **Pedoman Aturan & Tata Kelola Agen Resmi**:
  - [Aturan Orkestrasi & Koordinasi Multi-Agent](file:///root/Route-X/.agents/rules/multi_agent_orchestration.md) — Tata kelola resmi koordinasi multi-agent, katalog 13 sub-agent Route-X, pipeline orkestrasi 4-tahap, format serah terima pesan 5-seksi (`send_message`), isolasi branch Git kerja vs koordinasi komunikasi aktif, dan zero-any TypeScript.
  - [Aturan Pengembangan & Standar Kerja (routex-dev)](file:///root/Route-X/.agents/rules/routex-dev.md) — Konvensi Go & TS, zero-race, arsitektur single-admin, dan keamanan zero-trust.
  - [Standar Verifikasi Multi-Aspek](file:///root/Route-X/.agents/rules/multi_aspect_verification.md) — 4 lapisan verifikasi empiris, wire protocol live probe, dan protokol anti-halusinasi.
- Bahasa komentar/log di codebase ini: Indonesia. Ikuti gaya yang ada.
- Migrasi baru: tambah file `internal/database/migrations/NNNN_nama.sql` berurutan (terakhir: `0010_drop_roles.sql`)
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
- MCP Resmi yang terpasang untuk repo ini:
  - `codebase-memory-mcp`: Knowledge Graph & AST indeks kode Go & TS (`search_graph`, `trace_path`, `get_code_snippet`)
  - `postgres`: Akses langsung inspeksi DB `routex` (localhost:5432)
  - `redis`: Akses langsung inspeksi cache & sliding-window rate limit (localhost:6379)
  - `puppeteer`: Browser headless untuk validasi UI konsol web React (`web/`)

## Prosedur Rilis & Patch Modern Route-X (v1.2.x Checklist Pra-Rilis)
Sebelum mempublikasikan patch atau rilis `v1.2.x`, lakukan verifikasi berurutan berikut:
1. **Audit Data Race**: Jalankan `go test -race ./...` (wajib 0 data race terdeteksi).
2. **Formatting Go**: Jalankan `gofmt -l .` (wajib kosong/bersih).
3. **Integritas Database**: Pastikan berkas migrasi `internal/database/migrations/` berurutan rapi (saat ini 0001 sampai 0015) dan tidak merusak skema produksi.
4. **Build Frontend**: Jalankan `cd /root/Route-X/web && npm run build && npm test` (wajib kompilasi `tsc`, `vite build`, dan unit test lolos tanpa error).
5. **Uji Probing Gateway E2E**: Jalankan `./scripts/probe_e2e_gateway.sh` untuk memvalidasi rute `/v1/chat/completions` (non-stream & SSE stream) serta `/v1/messages` native Anthropic.
6. **Validasi Observabilitas**: Jalankan `./scripts/check_metrics.sh` untuk memverifikasi endpoint `/metrics` tidak merekam kegagalan terselubung.
7. **Sinkronisasi Versi**:
   - Perbarui versi pada `web/package.json` sesuai target rilis (misal `1.2.1`).
   - Tambahkan catatan perubahan rilis pada `CHANGELOG.md` di bawah seksi versi terkait.
   - Perbarui badge versi di `README.md` dan fallback versi di `Makefile` serta skrip instalasi/operasional.
8. **Git Tagging & Publikasi**:
   - Lakukan commit perubahan: `git commit -m "release: v1.2.x - ..."`
   - Buat tag git: `git tag -a v1.2.x -m "Release v1.2.x"`
   - Buat release via GitHub CLI: `gh release create v1.2.x --title "v1.2.x" --notes-file ...`
