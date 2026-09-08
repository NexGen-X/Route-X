# Route-X Agent Guide

## Peta sistem
- Agen OpenCode: `default_agent` adalah `route-x-orchestrator` (`.opencode/agents/`); agen generik `orchestrator` tersedia sebagai entry point sekunder via Tab/`@orchestrator`, tetapi tidak mengenal invarian Route-X dan katalog 258 subagennya belum terpasang — pasang sesuai kebutuhan via `npx -y @mouyox21/opencode-agents install <nama>`.
- Produk adalah satu biner: `cmd/ai-gateway` merangkai API Chi, dashboard React tersemat, migrasi, dan worker; `cmd/routex-rotate` adalah utilitas rotasi rahasia.
- Frontend berada di `web/`; `make build` wajib membangun `web/dist` lebih dulu lalu menanamkannya ke biner. Jangan mengandalkan `go build ./cmd/ai-gateway` untuk artefak produksi.
- Migrasi berada di `internal/database/migrations/`, ditanam lewat `go:embed`, dijalankan saat startup, dan harus bernama `NNNN_nama_snake_case.sql`. Jangan mengubah migrasi yang sudah diterapkan karena checksum diverifikasi.

## Perintah yang sering salah ditebak
- Gate cepat tanpa layanan eksternal: `make fmt && make vet && make test-unit && make build`.
- Satu paket/test Go: `go test -short ./internal/<paket>` atau `go test -short ./internal/<paket> -run '^TestName$'`.
- Gate DB memuat `./.env` otomatis dan sengaja gagal bila `DATABASE_URL` tidak ada: `make test-integration`, `make race`, `make test`, dan `make cover`. Test DB dipaksa `-p 1` karena tiap paket memeriksa schema test yang tersisa.
- Build/typecheck frontend: `npm --prefix web run build`. `web/package.json` tidak menyediakan script lint atau unit-test.
- Playwright: `npx --prefix web playwright test web/tests/e2e/<file>.spec.ts`; config mengarah ke `http://localhost:5173`, sedangkan Vite dev server memakai port `3000`, dan config tidak menyalakan `webServer`. Nyalakan server/base URL yang cocok sebelum E2E.
- E2E backend nyata ada di `scripts/e2e/e2e_verify.sh`; script memuat `.env`, membuat/menghapus data uji, dan memerlukan biner, PostgreSQL, Redis, Python, serta kredensial admin. Jalankan hanya pada database disposable. `scripts/e2e/load_test.sh` juga mengubah rate limit dan menulis artefak.

## Invarian yang wajib dipertahankan
- Nilai uang memakai `upstream.USD` (integer skala 1e-8 USD), bukan `float64`.
- Semua koneksi upstream, webhook, discovery, egress, dan probe outbound harus melewati `security.SSRFPolicy`; jangan membuat `http.Transport` bebas tanpa guarded dialer.
- Rahasia provider/proxy memakai AES-256-GCM dengan AAD yang terikat ke entitas. Jangan membaca, mencetak, atau memasukkan `.env`, URL ber-userinfo/query key, cookie, maupun token ke log/error.
- Migrasi dan job singleton memakai PostgreSQL advisory lock pada koneksi sesi yang sama hingga unlock.
- Worker harus menangkap panic, tidak mematikan proses, dan mencatat hasil ke metrik `routex_worker_*`.
- Komentar, pesan error, dan dokumentasi internal berbahasa Indonesia; identifier dan pesan commit berbahasa Inggris.

## Batas perubahan dan verifikasi
- Baca `git status` dan diff sebelum mengedit; jangan menghapus atau memformat ulang perubahan pengguna yang tidak terkait. Artefak lokal seperti `web/playwright-report/`, `web/test-results/`, dan skrip audit ad hoc bukan source produk.
- Untuk perubahan produk, mulai dari test paket/build frontend terarah, lalu jalankan gate cepat di atas. Jalankan integrasi/race hanya ketika `DATABASE_URL` tersedia dan scope membutuhkannya; laporkan skip secara eksplisit.
- `make fmt` dan `make build` menulis worktree (`web/dist`, binary); periksa diff/status sesudahnya dan jangan commit artefak build. `web/dist/.gitkeep` harus tetap ada agar embed pada clone bersih valid.
- Deployment produksi memakai overlay `docker-compose.prod.yml` dan mensyaratkan `PUBLIC_URL=https://...`; jangan menyalin `.env` development ke produksi. Uji production harus non-destruktif kecuali target dan akun uji dinyatakan aman.
