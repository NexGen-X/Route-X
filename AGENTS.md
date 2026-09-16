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
- Jangan commit secret; jangan ubah `go.mod` tanpa alasan; PR via `gh`
- MCP Hermes yang terpasang untuk repo ini: `filesystem` (scope /root/Route-X), `github`, `context7` (dok library), `routexdb` (SELECT-only ke `routex`), `routexredis`
