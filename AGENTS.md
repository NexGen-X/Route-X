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
- Bahasa komentar/log di codebase ini: Indonesia. Ikuti gaya yang ada.
- Migrasi baru: tambah file `internal/database/migrations/NNNN_nama.sql` berurutan (terakhir: `0010_drop_roles.sql`)
- **Alur Kerja Git Wajib PR (PR-Only Policy)**: Dilarang keras melakukan komit atau push langsung ke branch `main`. Setiap perubahan wajib dibuat pada branch terisolasi (`feat/*`, `fix/*`, `chore/*`), didorong ke remote, diajukan melalui `gh pr create`, dipantau hingga status CI hijau (`gh pr checks <id> --watch`), dan digabungkan melalui `gh pr merge <id> --squash --delete-branch`.
- Jangan commit secret; jangan ubah `go.mod` tanpa alasan.
- MCP Resmi yang terpasang untuk repo ini:
  - `codebase-memory-mcp`: Knowledge Graph & AST indeks kode Go & TS (`search_graph`, `trace_path`, `get_code_snippet`)
  - `postgres`: Akses langsung inspeksi DB `routex` (localhost:5432)
  - `redis`: Akses langsung inspeksi cache & sliding-window rate limit (localhost:6379)
  - `puppeteer`: Browser headless untuk validasi UI konsol web React (`web/`)

## Prosedur Rilis & Patch v1.1.0 (Checklist Pra-Rilis)
Sebelum mempublikasikan patch atau rilis `v1.1.0`, lakukan verifikasi berurutan berikut:
1. **Audit Data Race**: Jalankan `go test -race ./...` (wajib 0 data race terdeteksi).
2. **Formatting Go**: Jalankan `gofmt -l .` (wajib kosong/bersih).
3. **Integritas Database**: Pastikan berkas migrasi `internal/database/migrations/` berurutan rapi dan tidak merusak skema produksi.
4. **Build Frontend**: Jalankan `cd /root/Route-X/web && npm run build` (wajib kompilasi `tsc` dan `vite build` sukses tanpa error tipe).
5. **Uji Probing Gateway E2E**: Jalankan `./scripts/probe_e2e_gateway.sh` untuk memvalidasi rute `/v1/chat/completions` (non-stream & SSE stream) serta `/v1/messages` Anthropic.
6. **Validasi Observabilitas**: Jalankan `./scripts/check_metrics.sh` untuk memverifikasi endpoint `/metrics` tidak merekam kegagalan terselubung.
7. **Sinkronisasi Versi**:
   - Perbarui versi pada `web/package.json` menjadi `1.1.0`.
   - Tambahkan catatan perubahan rilis pada `CHANGELOG.md` di bawah seksi `## [v1.1.0]`.
8. **Git Tagging & Publikasi**:
   - Lakukan commit perubahan: `git commit -m "release: v1.1.0 - feature release"`
   - Buat tag git: `git tag -a v1.1.0 -m "Release v1.1.0"`
   - Buat release via GitHub CLI: `gh release create v1.1.0 --title "v1.1.0" --notes-file ...`
