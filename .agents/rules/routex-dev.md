# Route-X — Aturan Pengembangan & Standar Kerja (routex-dev)

Aturan ini berlaku wajib bagi AI agent dan pengembang yang bekerja di repositori Route-X.

---

## 1. Konvensi Bahasa & Gaya Kode
- **Bahasa**: Seluruh komentar kode, pesan commit, dokumentasi, dan pesan log wajib menggunakan **Bahasa Indonesia** teknis yang jelas dan konsisten.
- **Formatting**: Jalankan `gofmt -l .` sebelum commit. Hasilnya harus bersih (kosong tanpa berkas yang perlu diformat ulang).
- **Integritas Dependensi**: Dilarang mengubah atau menambahkan paket pada `go.mod` / `go.sum` tanpa alasan arsitektural yang jelas dan persetujuan eksplisit.

---

## 2. Pengujian & Penegakan Integritas (Zero-Race)
- **Race Detection**: Seluruh unit test dan integration test harus lolos pemeriksaan detektor balapan data:
  ```bash
  go test -race ./...
  ```
- **Isolasi Database Pengujian**:
  - Dilarang keras menulis data uji ke skema `public` atau basis data operasional/development.
  - Seluruh pengujian database wajib menggunakan skema sekali pakai (*ephemeral schema* `test_*`) dengan pengaturan `search_path` pada DSN koneksi.
  - Skema uji harus dibersihkan secara otomatis di akhir siklus pengujian (`defer dropSchema()`).

---

## 3. Arsitektur Identitas & Otorisasi (Single-Admin)
- **Prinsip Single-Admin**: Seluruh peran dan izin RBAC (`roles`, `permissions`, `user_roles`) telah dihapus secara permanen pada migrasi `0010_drop_roles.sql`.
- **Hak Penuh**: Setiap pengguna terautentikasi adalah `Admin` dengan hak penuh (`*`). Dilarang merekonstruksi tabel atau middleware RBAC multi-role.
- **Onboarding Awal**: Endpoint `GET /api/auth/setup-hint` digunakan untuk mendeteksi instalasi baru dan wajib dinonaktifkan otomatis segera setelah kata sandi awal diganti.

---

## 4. Keamanan & Kriptografi (Zero-Trust)
- **Enkripsi Kredensial**: Kunci API penyedia (provider upstream) dan secret wajib dienkripsi menggunakan **AES-256-GCM** dengan verifikasi tamper AEAD.
- **Hasing Kata Sandi**: Seluruh kata sandi pengguna wajib diamankan menggunakan algoritma **Argon2id** dengan verifikasi waktu konstan (*constant-time comparison*).
- **Proteksi CSRF & SSRF**:
  - Semua mutasi admin wajib lolos validasi *Double-Submit Cookie* dan verifikasi header `Origin`/`Host`.
  - Dialer HTTP keluar (*egress*) wajib menerapkan blokir SSRF terhadap subnet privat, loopback, dan endpoint metadata cloud (`169.254.169.254`).

---

## 5. Dual-Protocol Engine & Streaming (v1.0.0.1 Patch Standards)
- **Kompatibilitas OpenAI**: Rute `/v1/chat/completions` dan `/v1/responses` harus sepenuhnya mematuhi spesifikasi OpenAI, baik non-streaming maupun streaming SSE.
- **Kompatibilitas Anthropic**: Rute `/v1/messages` dan `/v1/v1/messages` harus menangani konversi dua arah secara presisi (roles, multi-modal content parts, tool calls).
- **Integritas Streaming SSE**: Chunk streaming tidak boleh memotong karakter UTF-8 multibyte atau merusak framing JSON event `data: {...}`.
- **Zero-Touch CLI Protection**: Konfigurasi otomatis untuk alat CLI pengembang (Claude Code, OpenCode, Aider, SGPT) tidak boleh mengganggu environment Antigravity (`agy`).

---

## 6. Alur Kerja Git & Penegakan Wajib Pull Request (PR-Only Policy)
- **Larangan Keras Direct Push**: Dilarang keras melakukan komit langsung atau `git push origin main`. Cabang `main` adalah cabang produksi yang dilindungi.
- **Wajib Feature / Task Branch**: Setiap perubahan (fitur, bugfix, refaktor, perapian UI, atau dokumentasi) wajib dikerjakan pada cabang terisolasi baru yang dicabangkan dari `main` mutakhir (`git checkout -b feat/...`, `fix/...`, atau `chore/...`).
- **Verifikasi Lokal Sebelum Push**:
  - `gofmt -l .` (wajib bersih tanpa berkas yang perlu diformat).
  - `go test -race ./...` (wajib 0 data race terdeteksi).
  - `cd web && npm run build` (wajib lulus kompilasi TypeScript dan Vite tanpa error).
- **Wajib Pull Request**: Seluruh integrasi kode ke cabang `main` WAJIB melalui Pull Request menggunakan GitHub CLI (`gh pr create --base main --head <branch>`).
- **Pemantauan & Kepatuhan CI**: Setelah PR dibuat, status CI GitHub Actions (`ci/go` dan `ci/web`) wajib dipantau menggunakan `gh pr checks <PR_NUMBER> --watch`. Dilarang menggabungkan PR jika ada pemeriksaan CI yang gagal atau merah.
- **Metode Penggabungan**: Penggabungan ke `main` hanya diizinkan melalui `gh pr merge <PR_NUMBER> --squash --delete-branch` setelah seluruh checks berstatus hijau.

