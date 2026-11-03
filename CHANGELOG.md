# Changelog

Semua perubahan penting pada project Route-X didokumentasikan dalam berkas ini.

Format berkas ini mengacu pada prinsip [Keep a Changelog](https://keepachangelog.com/en/1.1.0/) dan mematuhi [Semantic Versioning](https://semver.org/).

## [Unreleased]

### Added
- **UI harga per pemetaan model di dashboard**: drawer **Harga** pada setiap pemetaan di tab Model provider (halaman Penyedia) kini menampilkan tiga hal yang selama ini hanya bisa diakses lewat API — (1) **harga efektif** yang sedang menagih, diturunkan dari riwayat harga dengan predikat rentang efektif yang sama dengan backend; (2) **harga acuan (reference)** katalog bawaan untuk pemetaan itu beserta sumber dan tanggal ambilnya, dinyatakan jelas sebagai tawaran awal yang tidak menagih; dan (3) **formulir pasang harga manual** yang menulis lewat endpoint `POST /api/admin/upstreams/models/mappings/{id}/pricing` yang sudah ada (tombol "Pakai harga acuan sebagai isian" hanya mengisi formulir — pemasangan tetap keputusan operator). Keadaan jujur ditampilkan apa adanya: pemetaan tanpa harga berlaku dinyatakan tercatat *unpriced*, dan pemetaan tanpa acuan katalog dinyatakan tidak punya acuan yang bisa dipertanggungjawabkan (gateway tidak mengarang angka). Tidak ada endpoint backend baru: reference hanya tersedia per-pemetaan dari backend, jadi tidak ada tabel katalog acuan global.

### Fixed
- **Restore dari panel admin kini benar-benar bekerja pada instalasi native standar (B-104)**: terbukti pada drill Langkah 7 bahwa restore selalu gagal `42501: permission denied to create database` pada instalasi standar, sementara ekspor bekerja — fitur pemulihan mati tepat saat paling dibutuhkan. Akarnya: mekanisme restore skratch-swap (B-045) dijalankan gateway memakai role aplikasi dan menuntut hak `CREATEDB` di dua titik (membuat database skratch; `ALTER DATABASE ... RENAME` saat penukaran nama — pemilik database saja tidak cukup untuk keduanya), tetapi `install-native.sh` membuat role `routex` tanpa hak itu. Installer kini membuat role dengan `CREATEDB` dan **menyelaraskannya pada setiap lari** lewat baris `ALTER USER` yang sama yang menyelaraskan password, sehingga menjalankan ulang installer (jalur upgrade) menyembuhkan instalasi lama tanpa langkah manual. **Trade-off keamanan (sadar, terlingkup):** hak baru yang ada hanyalah `CREATEDB` pada satu-satunya role aplikasi itu — bukan `SUPERUSER` dan bukan `CREATEROLE`; pemegangnya (proses gateway / siapa pun yang membobol kredensial `DATABASE_URL`, yang tanpanya pun sudah bisa membaca, mengubah, dan menghapus seluruh isi database aplikasi) kini juga bisa membuat/menghapus/mengganti nama database *milik role itu sendiri* (skratch, database aktif, dan salinan `<db>_pre_restore`), dengan risiko tambahan utama berupa ketersediaan dan pengisian disk oleh database baru — diterima dalam konteks instalasi pribadi satu pengguna di mana PostgreSQL hanya melayani Route-X. Alternatifnya kalah: mekanisme restore tanpa pembuatan database menuntut perombakan skratch-swap yang teruji (B-045/B-077) atau kredensial pembantu berhalk setara yang tetap harus dipegang gateway. Jalur compose/Docker tidak berubah: di sana role aplikasi memang superuser bawaan image PostgreSQL. `docs/DISASTER_RECOVERY.md` §3 dan `docs/runbook-operasi.md` §1 kini menyatakan syarat `CREATEDB` ini secara eksplisit, termasuk obat untuk pemasangan manual. Test regresi baru membuktikan kontrak haknya ujung-ke-ujung lewat handler produk sebagai role terbatas non-superuser: ekspor bekerja, restore gagal 42501 sebelum `CREATEDB`, dan berhasil penuh (skratch tertukar, database pengaman utuh, versi skema cocok) sesudahnya.
- **PERBAIKAN KEAMANAN — menonaktifkan kredensial provider kini benar-benar menghentikan trafiknya (B-103)**: terbukti live (Langkah 7, A-045, dua kali) bahwa menonaktifkan satu-satunya kredensial provider berjenis `openai_compatible`/`custom` tidak berpengaruh apa pun — toggle menjawab 200 dan kolom `enabled` berubah, tetapi chat tetap 200 mencapai upstream dan `last_used_at` kredensial tidak bergerak. Akarnya bukan di pemilihan kredensial (`CredentialRepo.Active` memang selalu menyaring `enabled=true` dan belum kedaluwarsa), melainkan di pabrik adapter gateway: `ErrNotFound` dari `Active` untuk kedua jenis itu selalu ditafsirkan sebagai "provider lokal tanpa autentikasi" dan request diteruskan **tanpa kredensial apa pun**, sehingga "tidak pernah punya kredensial" dan "semua kredensial dinonaktifkan" tidak bisa dibedakan. Kini keduanya dibedakan secara eksplisit lewat pemeriksaan baru `CredentialRepo.HasCredentials`: mode tanpa kredensial hanya bertahan untuk provider yang **tidak pernah punya baris kredensial sama sekali** (mode lokal Ollama/vLLM/LM Studio yang sah); provider yang punya kredensial tetapi tidak satu pun aktif — termasuk yang kedaluwarsa semua, dan termasuk bila pemeriksaan keberadaannya sendiri gagal (gagal-tertutup) — kini **gagal eksplisit** di pabrik dan tidak pernah mencapai upstream, terklasifikasi mengikuti konvensi gateway yang ada sebagai kegagalan autentikasi hulu (failover ke kandidat lain tetap berjalan; bila semua kandidat gagal, klien menerima 502 `upstream_auth_failed`). Test regresi setara repro live: di tingkat pabrik dengan upstream penghitung hit (semua kredensial nonaktif → adapter gagal dan nol hit ke upstream untuk kelima jenis provider; kredensial aktif → chat normal dengan header autentikasi dan penandaan pemakaian berjalan; tanpa kredensial sama sekali → mode lokal tetap dilayani) dan di tingkat repo terhadap PostgreSQL (kredensial nonaktif tidak pernah terpilih di antara banyak kredensial, `last_used_at` kredensial aktif bergerak, dan `HasCredentials` tetap `true` sesudah semua kredensial dinonaktifkan).
- **PERBAIKAN KEAMANAN — pembuatan pengguna tidak lagi selalu menghasilkan admin penuh (B-102)**: sebelum ini `CreateUserRequest`/`UpdateUserRequest` tidak memiliki field peran, lapisan auth memberi peran `Admin` + izin `*` kepada SETIAP pengguna terautentikasi (`Principal.Can` selalu true, `loadAuthorization` selalu `["Admin"]/["*"]`, DTO admin menulis literal `["Admin"]`), dan terbukti live sesi pengguna hasil `createUser` membaca `GET /api/admin/access/users` dengan 200 — tidak ada jalur produk untuk membuat pengguna non-admin. Kini peran adalah data sungguhan: migrasi `0020` menambah kolom `users.is_admin` (bawaan `false`; pengguna lama di-backfill `true` karena mereka memang admin faktual, supaya upgrade tidak mengunci operator keluar), `POST /api/admin/access/users` dan `PUT /api/admin/access/users/{id}` menerima field `role` (`"admin"`/`"user"`; nilai lain ditolak 400), **bawaan tanpa field peran adalah pengguna biasa**, dan izin principal diturunkan dari kolom itu (admin = `["Admin"]/["*"]`, biasa = `["User"]`/`[]`) sehingga middleware `RequirePermission` yang sudah terpasang di seluruh rute admin kini benar-benar menolak non-admin dengan 403 — termasuk pada sesi yang sedang berjalan, karena principal dimuat ulang dari baris pengguna tiap request. Penjaga admin-terakhir kini menghitung admin aktif (bukan pengguna aktif), penurunan peran akun sendiri ditolak, dan bootstrap admin pertama dari `INITIAL_ADMIN_PASSWORD` tetap admin penuh secara eksplisit. Panel Pengguna mendapat pemilih peran pada formulir tambah dan drawer kelola, dan badge peran kini menampilkan data backend. **BREAKING (perilaku)**: akun yang dibuat tanpa `role: "admin"` tidak lagi bisa membuka permukaan admin.
- **Claude Code modern tidak lagi ditolak 400 untuk model non-Claude gara-gara `thinking.type: "adaptive"` (B-096)**: Claude Code 2.x mengirim `thinking: {"type":"adaptive"}` secara bawaan pada `POST /v1/messages`, dan pengurai permukaan Anthropic menolak nilai itu untuk semua rute — terbukti live enam kali 400 pada model intern-s2 (upstream OpenAI-compatible), padahal rantai yang sama menjawab 200 begitu thinking tidak dikirim. Penolakan kini dicabut dari titik yang tidak tahu upstream-nya (decode) dan pemetaannya diputuskan di titik yang tahu: untuk kandidat **Anthropic**, `adaptive` diteruskan apa adanya (tanpa `budget_tokens` karangan; temperature/top_p distrip seperti thinking menyala lainnya); untuk kandidat **non-Anthropic**, `adaptive` distrip per kandidat di `untukKandidat` — maknanya "model memutuskan sendiri" paling setia diwakili dengan tidak mengirim parameter thinking apa pun (bawaan upstream berlaku), bukan mengarang budget atau `reasoning_effort` — dan adapter OpenAI/Google juga mengabaikannya sebagai lapis pengaman. Perilaku `enabled`/`disabled` tidak berubah di jalur mana pun, dan nilai `thinking.type` yang benar-benar tidak dikenal tetap ditolak 400.
- **Keluaran Configure untuk lima tool CLI diperbaiki ke format yang benar-benar dibaca tool-nya (B-097, B-098, B-099, B-100, B-101)**: (B-097) `~/.claude/settings.json` kini ditulis dalam skema sah Claude Code 2.x — objek `env` berisi `ANTHROPIC_BASE_URL` (tanpa `/v1`), `ANTHROPIC_MODEL`, dan `ANTHROPIC_API_KEY` bila key diberikan, plus `model` tingkat-atas — menggantikan kunci root snake_case `anthropic_base_url`/`model` yang tidak dikenal tool; setelan pengguna lain di berkas dipertahankan dan izin berkas diperketat ke 0600. (B-098) Nama model Aider kini selalu berprefiks provider LiteLLM `openai/<model>` di `.aider.conf.yml` dan env `AIDER_MODEL` — tanpa prefiks itu Aider gagal sebelum request apa pun. (B-099) Goose kini mendapat `~/.config/goose/config.yaml` yang sah (`GOOSE_PROVIDER: openai`, `GOOSE_MODEL`, `OPENAI_HOST` tanpa sufiks `/v1`; blok lain milik pengguna dipertahankan) dan ekspor `GOOSE_PROVIDER=openai`/`OPENAI_HOST`/`OPENAI_API_KEY`; ekspor rekaan lama `GOOSE_OPENAI_HOST`/`GOOSE_API_KEY` yang tidak dibaca Goose dihapus (key sengaja tidak ditulis ke config.yaml karena Goose mengabaikan key di berkas itu). (B-100) `VersionArg` goose diganti dari subcommand `version` yang ditolak Goose menjadi `--version`. (B-101, **perbaikan keamanan**) Continue CLI (`cn`) kini mendapat `~/.continue/config.yaml` lokal yang sah (provider `openai`, `apiBase` gateway dengan `/v1`, `apiKey` key produk, izin 0600) sehingga cn memakai gateway dan tidak lagi jatuh ke config default dari env generik yang mengirim key gateway ke vendor asli; ekspor rekaan `CONTINUE_API_BASE`/`CONTINUE_API_KEY`/`CONTINUE_MODEL` yang tidak dibaca cn dihapus, dan biner `cn` kini ikut terdeteksi. Nama generik di `key.env` (`ANTHROPIC_API_KEY`, `OPENAI_API_KEY`, dll.) dipertahankan karena tool yang terbukti bekerja (Claude Code, OpenCode) bergantung padanya — sisa ketegangan desainnya dicatat sebagai entri backlog tersendiri.
- **Configure CLI mati total di instalasi native diperbaiki (B-095)**: installer kini memastikan direktori home pengguna layanan `routex` ada dan dimiliki `routex:routex` pada setiap lari — termasuk instalasi ulang/upgrade, yang dengan begitu menyembuhkan instalasi lama. Sebelumnya pengguna dibuat dengan `useradd --no-create-home` dan home-nya tidak pernah dibuat oleh langkah mana pun, sehingga `POST /api/admin/cli/configure` selalu gagal `400 configure_failed` (`mkdir /home/routex: permission denied`) sampai operator membuat home secara manual. Unit systemd kini juga memasukkan `-/home/routex` ke `ReadWritePaths` agar home tetap bisa ditulis di bawah `ProtectSystem=strict`.
- **Installer native macet total saat dijalankan ulang diperbaiki (B-084)**: operasi git atas direktori sumber installer sendiri kini memakai `git -c safe.directory=<src>` per panggilan — `fetch`/`reset --hard` atas `/opt/routex/src` pada jalur upgrade, dan `rev-parse` atas `SRC_DIR` pada fallback kompilasi. Sebelumnya installer yang berjalan sebagai root ditolak git (`detected dubious ownership`) karena src dimiliki pengguna layanan `routex`, sehingga upgrade instalasi native yang sudah berjalan gugur di tahap 0 (dan cap versi fallback diam-diam menjadi `dev-unknown`). Perbaikan terlingkup hanya pada src installer: konfigurasi global operator tidak diubah, perlindungan kepemilikan untuk direktori lain tidak dilemahkan, dan perilaku instalasi segar tidak berubah.

### Changed
- **Templat Caddyfile native diselaraskan kosmetik ke templat compose (rider B-079)**: pada ketiga blok situs yang ditulis `install-native.sh` — `keepalive` 300s → 90s, `dial_timeout 5s` ditambahkan, dan `encode` kini memakai `minimum_length 512`; kedua blok HTTPS kini juga menyetel HSTS ber-token `preload` (`max-age=31536000; includeSubDomains; preload`) — semuanya mengikuti `deploy/caddy/Caddyfile`. Penyelarasan ini kosmetik, BUKAN perbaikan buffering: uji A/B B-079 sudah membuktikan penerusan SSE identik pada kedua bentuk konfigurasi, dan `flush_interval` sengaja tidak diubah.
- **Kebijakan dependabot diperketat**: pembaruan minor/patch kini digabung menjadi satu PR grup per ekosistem (gomod, npm, github-actions, docker), dan bump major untuk dependensi terpaku — `react`, `react-dom`, `@types/react`, `@types/react-dom`, `vite`, `lucide-react` (npm) dan `node` (docker) — diabaikan agar menjadi pekerjaan upgrade terencana, bukan banjir PR mingguan. Batas PR terbuka diturunkan (npm/gomod/github-actions 5→3, docker 3→2). Hasil triase pendamping atas lima PR major yang menumpuk: `lucide-react` 0.453.0 → 1.66.0 digabung sesudah terbukti hijau (PR #218); `vite` 8 (konflik peer `@vitejs/plugin-react`), pasangan React 19 (konflik peer timbal-balik react/react-dom), dan `node` 26-alpine (bertentangan pin Node 24 LTS) ditutup sebagai pekerjaan upgrade terencana.
- **Toolchain build web dinaikkan terkoordinasi: Vite 5.4 → 8.3 dan `@vitejs/plugin-react` 4.7 → 6.2** (Langkah 5a, menutup triase dependabot PR #221 yang ERESOLVE sendirian): kedua paket dinaikkan dalam satu PR karena plugin-react 6 hanya menerima Vite `^8.0.0`, dan Vite 8 mengganti Rollup+esbuild dengan Rolldown+Oxc. Satu-satunya penyesuaian konfigurasi: `build.rollupOptions.output.manualChunks` bentuk objek — yang sudah tidak didukung Vite 8 dan membuat build gagal — diganti `build.rolldownOptions.output.codeSplitting` dengan nama chunk yang sama persis (`vendor-charts`, `vendor-icons`), sehingga pemisahan chunk dan penyaringan preload HTML tidak berubah perilaku. plugin-react 6 tidak lagi memakai Babel (transform JSX & Fast Refresh kini lewat Oxc); repo tidak memakai plugin Babel/kustom apa pun lewat opsi plugin-react, jadi tidak ada konfigurasi tambahan. React tetap 18, Vitest tetap 2 (ia menggendong salinan Vite 5 internalnya sendiri; seluruh 318 test web lulus tanpa penyesuaian), TypeScript tidak berubah. Syarat Node Vite 8 (`^20.19 || >=22.12`) terpenuhi oleh pin Node 24 di CI dan Dockerfile.
- **Framework dashboard dinaikkan satu paket atomik: React 18.3 → 19.3** (Langkah 5b, menutup triase dependabot pasangan React 19 yang ERESOLVE bila dinaikkan terpisah): `react` & `react-dom` 18.3.1 → 19.3.1 dan `@types/react` & `@types/react-dom` 18.3.x → 19.3.0 dinaikkan bersama dalam satu PR karena peer timbal-baliknya (`react-dom` 19.3.1 mensyaratkan `react` `^19.3.1`; `@types/react-dom` 19.3.0 mensyaratkan `@types/react` `^19.3.0`). Tidak ada dependensi pendamping yang perlu dinaikkan: seluruh konsumen React yang terpasang sudah menyatakan dukungan React 19 pada versi di lockfile — `recharts` 2.15.4, `@tanstack/react-query` 5.105.0, `focus-trap-react` 12.0.3, `lucide-react` 1.66.0, dan `@testing-library/react` 16.3.3 (peer semuanya mencakup `^19`). Satu-satunya penyesuaian kode yang dipaksa tipe React 19: prop `fileInputRef` pada tipe props tab Pencadangan kini `React.RefObject<HTMLInputElement | null>`, mengikuti kembalian `useRef<HTMLInputElement>(null)` pada `@types/react` 19 (build awal gagal TS2322 di `Settings.tsx`). Vite/plugin-react/Vitest/TypeScript tidak berubah; seluruh 318 test web lulus dan build bersih dari `npm ci` lockfile baru.

## [1.6.1] - 2026-11-03

Rilis sesudah v1.6.0: siklus hidup kunci API disederhanakan (B-080 — memuat **dua perubahan BREAKING** pada permukaan admin kunci API, lihat Changed), tiga gesekan installer & pesan dari uji instalasi VPS diperbaiki (B-082), dan dependensi uji web diperbarui. Tidak ada migrasi skema baru; jalur upgrade dari v1.6.0 lurus (biner diganti, skema tetap 0019).

### Added
- **Endpoint `DELETE /api/admin/access/api-keys/{id}`** (B-080): penghapusan kunci API secara permanen dengan izin `apikeys:write`. Kunci yang terhapus seketika tidak bisa autentikasi lagi; riwayat request miliknya tetap tersimpan dan terbaca (baris request membawa snapshot nama kunci). Dashboard kini menampilkan tombol **Aktifkan/Nonaktifkan**, **Rotasi**, dan **Hapus** (dengan dialog konfirmasi yang menyebut nama kunci) pada halaman Kunci API.

### Changed
- **BREAKING — Body toggle kunci API diselaraskan (B-080)**: `POST /api/admin/access/api-keys/{id}/toggle` kini membaca `{enabled: boolean}` seperti semua toggle lain di sistem. Body lama `{status: "active"|"disabled"}` ditolak `400` (field tak dikenal). Keputusan pemilik: siklus hidup kunci disederhanakan menjadi hanya aktif/nonaktif, rotasi, dan hapus permanen.
- **BREAKING — Endpoint `POST /api/admin/access/api-keys/{id}/revoke` dihapus (B-080)**: pencabutan sebagai operasi tersendiri dicabut; perannya digantikan toggle nonaktif (sementara) dan DELETE (permanen). Status `revoked` tidak lagi diproduksi jalur admin — baris lama berstatus `revoked` tetap tertolak autentikasi, tetap final (tidak bisa diaktifkan lagi), dan bisa dihapus lewat DELETE. Webhook `api_key.revoked` tetap dipancarkan saat kunci mati permanen (kini oleh rotasi dan oleh DELETE, dibedakan lewat field `reason`).
- **Dependensi uji web diperbarui**: `happy-dom` 20.14.6 → 21.0.4 (PR #223, dependabot) — devDependency lingkungan test dashboard; seluruh test web lulus tanpa penyesuaian kode, tanpa perubahan perilaku produk.

### Fixed
- **Tiga gesekan installer & pesan dari uji VPS (B-082)**: deteksi mode eksekusi `install-native.sh` kini membedakan berkas lokal di luar checkout dari pipe/curl (dulu semua eksekusi di luar checkout dilaporkan sebagai pipe/curl); pesan penolakan `base_url` http di mode produksi kini menyebut obatnya (`UPSTREAM_ALLOW_HTTP=true`; alamat privat tetap dibatasi `UPSTREAM_ALLOWED_PRIVATE_ADDRS`); installer tanpa `DOMAIN` kini memperingatkan sejak awal dan di ringkasan akhir bahwa memasang domain belakangan wajib diikuti menjalankan ulang installer dengan `DOMAIN=` — tanpa itu domain gagal (di Cloudflare tampak sebagai error 525).

## [1.6.0] - 2026-11-03

Rangkaian perubahan sesudah v1.5.1: kampanye hardening **audit3** (15 grup fix atas 314 temuan terverifikasi dari audit 50 area, PR #204–#224), batch sisa A1/A2/B1/B2 (PR #225–#228), fitur Batch C (PR #229–#230), batch penutup (PR #231), batch kejujuran UI (PR #236), dan penyetaraan CLI (PR #238). Termasuk migrasi skema **0017–0019**; jalur upgrade dari database v1.5.1 (skema 0016) terverifikasi tanpa kehilangan data.

### Breaking Changes
- **Tanda tangan webhook diganti** (audit3 grup 7, PR #210): delivery kini ditandatangani HMAC-SHA256 atas `"<timestamp>.<deliveryID>.<body final>"` dan dikirim lewat header **`X-RouteX-Signature`** (`sha256=<hex>`). Header warisan `X-Hub-Signature-256` **dibuang** — penerima webhook harus memverifikasi skema baru. Secret webhook kini wajib non-kosong; webhook tanpa secret ditolak `400`.
- **`ANTIGRAVITY_CLIENT_SECRET` wajib dari environment** (audit3 grup 8, PR #211): client secret Antigravity tidak lagi tertanam di biner (fallback obfuskasi dihapus). Tanpa env ini, provider Antigravity/Google OAuth berhenti dengan error eksplisit.

### Added
- **Response cache bisa diaktifkan dengan aman** (Batch C1, PR #229): penolakan `cache_policy_unsafe` dicabut. Sidik kunci cache = hash identitas API key terpisah domain + request kanonik penuh + permukaan + sidik rute, sehingga entri terisolasi per kunci dan perubahan routing membuat entri lama tak terjangkau. Tahap awal hanya respons sukses non-streaming tanpa tool call; streaming tidak pernah di-cache. Tombol Aktifkan di Pengaturan → Performa & Cache kini berfungsi.
- **Block `document` Anthropic didukung** (Batch C2, PR #230): dokumen teks & base64 `text/plain` menjadi teks kanonik di posisi block-nya (terkirim ke semua upstream); PDF base64 diteruskan setia sebagai block native ke upstream Anthropic (tanpa kandidat Anthropic → `400 unsupported_document_upstream` sebelum jaringan). Maksimum 10 MiB per dokumen; sumber URL tetap ditolak (SSRF).
- **Zona waktu operator untuk periode anggaran** (Batch B1, PR #227): env `BUDGET_TIMEZONE` (bawaan `UTC`, nama IANA tak sah menggagalkan boot). Batas weekly/monthly dihitung di zona operator; daily tetap 24 jam bergulir. Mengubah setelan tidak menulis ulang `period_end` yang sedang berjalan.
- **Tarif cache-creation Anthropic terurai** (Batch B1, PR #227): rincian `cache_creation` kini dihargai per pecahan — 1 jam = 2× harga input, 5 menit/sisa = 1,25×; tanpa rincian dari upstream, perilaku lama (semua 1,25×) berlaku persis.
- **Endpoint + UI buka kunci pengguna** (batch penutup, PR #231): `POST /api/admin/access/users/{id}/unlock` dan tombol Buka Kunci di halaman Pengguna untuk akun terkunci (menutup B-076).
- **Endpoint acuan harga** `GET /api/admin/upstreams/models/mappings/{mapping_id}/pricing/reference` (audit3 grup 2) dan cadangan `.sha256` per tarball pada rilis (installer memverifikasinya) (grup 15).
- **Perintah CLI `routex-apikey create --expires-days`** (1–730 hari) (audit3 grup 15).

### Changed
- **Anggaran menegakkan secara atomik** (audit3 grup 1–2): `period_end` selalu terisi dan selaras kalender (migrasi 0017 menyembuhkan baris warisan), estimasi biaya **direservasi sebelum request berjalan** (`Enforcer.Tahan`), scope model disimpan sebagai UUID, dan kandidat yang anggarannya habis disisihkan dari failover.
- **Backup/restore ditulis ulang atomik** (audit3 grup 10, entri Kritis B-045): restore skratch-swap — unggahan dipindai anti meta-command psql, direstore ke database skratch `--single-transaction`, dicek ledger skema, lalu ditukar nama (database lama dipertahankan sebagai `<db>_pre_restore`). Migrasi 0018 memperbaiki akar "cadangan tidak bisa dipulihkan sejak 0007" (search_path fungsi histogram). Ekspor kini `POST` streaming. Catatan audit aksi restore kini ditulis lewat koneksi segar sesudah swap (batch penutup, PR #231).
- **`/healthz` & `/readyz` hanya mengembalikan status** (audit3 grup 15): field version/commit/uptime dicabut dari respons publik; `/readyz` di-cache 2 detik.
- **Installer & deploy diperkeras** (audit3 grup 15): arsitektur dipilih dari `uname -m` (ARM64 tidak lagi mengambil tarball amd64), checksum rilis diverifikasi, Caddyfile dicadangkan sebelum ditimpa, `TRUSTED_PROXIES` disetel eksplisit (loopback di native; subnet compose terpaku `172.30.0.0/16`), `UPSTREAM_TIMEOUT` terpasang ujung-ke-ujung.
- **Sesi & autentikasi diperketat** (audit3 grup 9): batas 10 sesi bersamaan per pengguna, plafon mutlak 7 hari di atas TTL geser, saklar revoke-all, penjaga admin-terakhir ber-advisory-lock.
- **Cache statistik akurat**: `entry_count` direkonsiliasi berkala terhadap kedaluwarsa TTL alami (Batch A2); event usage yang ter-drop dari antrean recorder kini tetap ditagih dengan kalkulator yang sama (Batch A2).
- **Routing menyisihkan egress pool tak sehat** (Batch A2): pool berstatus `unhealthy` segar (≤5 menit) dilewati selama ada alternatif sehat; fail-open tercatat di log bila semua pool tak sehat.
- **Invalidasi kebijakan seketika** (Batch B2): perubahan batas laju & filter konten dari admin kini menginvalidasi salinan gateway seketika (`Invalidate`), bukan menunggu TTL 5 detik.
- **Halaman web lebih jujur & aksesibel** (audit3 grup 12–13 + Batch A1): kontrak agregasi FE↔BE selaras (`window` dihormati, persen breakdown dari total keseluruhan + baris "Lainnya"), penanda daftar terpotong tampil di UI, feed Dashboard satu tombol asli (axe `nested-interactive` 0), jenjang heading diperbaiki, dan ringkasan provider menampilkan penanda gagal-muat, bukan daftar kosong.
- **Dokumentasi repo dirombak** (PR #232–#235; restyle presentasi README PR #237): README ditulis ulang profesional; `CONTRIBUTING.md` dan `SECURITY.md` dibuat; klaim basi (enkripsi API key, hitungan migrasi, perilaku anggaran pra-reservasi) dikoreksi ke kondisi kode.
- **CLI Antigravity (`agy`) disetarakan dengan CLI lain** (B-081): kunci read-only khusus agy dicabut — cabang khusus di manajer konfigurasi CLI, penjaga mode/target/terapkan di halaman Integrasi CLI, serta rendering khusus kartu tool (badge "Dilindungi Sistem", banner kunci, tombol "Terkunci (Sistem)") dihapus. `agy` kini tool env-only normal yang dapat dikonfigurasi persis seperti 19 CLI lainnya.

### Fixed
- **RCE via restore backup (Kritis, B-045)**: meta-command psql (`\!`) dari berkas unggahan tidak lagi bisa tereksekusi — pemindai lexer menolaknya sebelum psql (lihat Changed di atas).
- **Bypass CSRF**: satu header `Authorization` sampah tidak lagi mengalahkan cookie sah; pengecualian CSRF hanya berasal dari `Principal.ViaHeader` (audit3 grup 9).
- **API key pemilik nonaktif/wajib-ganti-password** kini ditolak 401 di permukaan inferensi (audit3 grup 9).
- **Panic gateway** saat semua kandidat ter-skip breaker: kini 503 `upstream_overloaded` yang rapi dan tercatat dengan status yang sama seperti yang diterima klien (bukan 499) (audit3 grup 3).
- **Slot konkurensi streaming bocor**: limit `max_concurrent` kini dipegang sampai aliran berakhir (audit3 grup 6).
- **Webhook retry abadi ±1×/detik**: delivery kini mencapai status terminal `abandoned`; token Telegram/Discord disamarkan dari error DB/log (audit3 grup 7).
- **SSRF saat simpan provider & sync model**: `base_url` divalidasi (alamat metadata cloud ditolak `400 invalid_base_url`) dan ada gerbang resolusi nama tujuan (audit3 grup 8).
- **Endpoint ikatan provider aturan routing** yang selalu menjawab 500 (`ErrNeedsTx`) kini 200 untuk ikatan sah; penegakan "provider wajib memetakan model" di server (audit3 grup 13).
- **Paginasi kunci kehilangan baris** (20 dari 30 tak terjangkau): kursor kini tuple `(created_at, id)`; pola pencarian `%`/`_` di-escape (audit3 grup 11).
- **Penolakan middleware 401/429** kini tercatat di tabel requests, dan parameter dashboard yang sebelumnya diabaikan backend (`window`, filter status/provider) kini bekerja (audit3 grup 12).
- **Restore cadangan sendiri mustahil dipulihkan sejak migrasi 0007** — akar diperbaiki migrasi 0018 (audit3 grup 10).
- **Manifest PWA disajikan `text/plain` di mode native**: Content-Type kini eksplisit per aset, `sw.js`/manifest `no-cache`, `favicon.ico` & `robots.txt` tersedia (audit3 grup 14).
- **Constraint pipeline** melempar 22P02 untuk bentuk buruk: ditulis ulang migrasi 0019 sehingga ditolak 23514; indeks ganda warisan dibuang (audit3 grup 11).
- **`UPSTREAM_TIMEOUT` mati** dan gauge circuit breaker yatim: keduanya kini terpasang ujung-ke-ujung (audit3 grup 15).
- **Berkas CLI tidak aman**: `opencode.json` kini 0600 tanpa kredensial hantu; `key.env` dibangun ulang dari semua tool (audit3 grup 15).
- **Tab Payload inspektor tampil sebagai kegagalan padahal payload memang tidak disimpan** (B-078, PR #236): keadaan tak-tersimpan (`404 not_found`) kini ditampilkan sebagai penjelasan netral bahwa Route-X tidak merekam body request/respons; kegagalan muat sungguhan tetap tampil sebagai banner error.

## [1.5.1] - 2026-11-02

Rilis patch: satu perbaikan kebocoran kuota token pada jalur streaming, dilabuhkan dari PR #137 yang tidak dapat di-merge langsung (branch-nya bercabang dari era PR #136 dan akan merevert Fase 2–7). Tidak ada perubahan skema, API, atau antarmuka dari v1.5.0.

### Fixed
- **Pencatatan token aliran SSE yang diputus klien** (PR #202, pelabuhan PR #137): pencatatan pemakaian token setelah respons selesai kini memakai context yang dilepas dari pembatalan request (`context.WithoutCancel` + tenggat 5 detik via helper `ctxCatatToken`). Sebelumnya, bila klien menutup koneksi di tengah aliran, context request yang sudah batal membuat operasi Redis ditolak dan token yang sudah dibayar ke upstream tidak pernah memakan kuota — klien yang selalu memutus aliran sebelum token terakhir dapat memakai token secara gratis. Perbaikan yang sama diterapkan ke `CatatTokenKolomProvider` (penghitung kolom `rate_limit_tpm` provider), yang dipanggil berpasangan pada jalur yang sama dan memiliki cacat identik. Test regresi `TestCatatTokenTetapJalanSaatKlienMemutusAliran` dilabuhkan dari #137.

## [1.5.0] - 2026-11-02

Rilis penyelarasan penuh Calm Console: seluruh antarmuka kini mengikuti bahasa desain sampel secara utuh — Pengaturan tanpa panel, palet perintah berkelompok, Observabilitas tanpa kartu, lembar bawah detail Permintaan di mobile, dan batang anggaran per kunci dari data nyata — di atas fondasi konsolidasi Fase 2 dan perombakan UI/UX Fase 3–4. Installer native kini mematok tag rilis v1.5.0 sehingga unduhan biner resmi dan fallback kompilasi sama-sama menghasilkan versi ini.

### Added
- **Batang anggaran per kunci API** (Fase 7, PR #200): baris kunci menampilkan batang tipis belanja tercatat versus batas dari entitas Budget (`scope: api_key`) — hanya untuk kunci yang benar-benar memiliki anggaran terdaftar.
- **Lembar bawah (bottom sheet) detail Permintaan di mobile** (Fase 7, PR #200) lewat prop `sheetOnMobile` pada Modal bersama; pemakai Modal lain tidak berubah.
- **Palet perintah berkelompok per kategori** (Fase 7, PR #199): hasil Ctrl+K kini dikelompokkan Navigasi / Aksi Cepat / Perkakas CLI dengan judul grup.
- **Galeri dokumentasi visual README yang rinci**: 10 tangkapan layar UI terkini (desktop + mobile) dengan penjelasan tiap permukaan, diganti dari instalasi nyata rilis ini.

### Changed
- **Penyelarasan tema persis ke sampel (Fase 6, PR #196–#197)**: token warna disamakan piksel-per-piksel dengan sampel Calm Console (sidebar netral `#161616`, status pastel `#3ECF8E`/`#F26D6D`/`#F5B84C`, teks `#ECECEC`/`#B9B9B9`) dan font sans resmi diganti Inter → **Noto Sans**; screenshot README diganti build Fase 6.
- **Pengaturan & Observabilitas tanpa panel (Fase 7, PR #198–#199)**: seluruh isi tab Pengaturan (Domain, Cache, Cadangan, Lanjutan) dan tiga panel terakhir Observabilitas diratakan menjadi baris/seksi polos bergaris rambut mengikuti sampel.
- **Sinkronisasi repo (Fase 5, PR #194–#195)**: README, CHANGELOG, prompt agen, dan seluruh skrip diselaraskan ke keadaan kode (versi tampil installer & probe v1.4.0 pada masanya, tautan `file:///root` diperbaiki).
- **Calm Console (Fase 4, PR #191–#193)**: perampingan visual menyeluruh mengikuti tema gelap Muse — versi tidak lagi tampil berulang di shell, hero Dashboard menjadi baris status tunggal, metrik menjadi strip KPI tanpa kartu bertumpuk, kartu Penyedia & Kunci API menjadi daftar baris, Observabilitas memakai tab garis bawah, dan Pengaturan memakai sub-navigasi ala Linear. Terverifikasi E2E menyeluruh 229 tes (0 gagal) dan audit kontras axe 0 pelanggaran.
- **Bahasa Indonesia penuh untuk UI (Fase 3, PR #186)** mengikuti glosarium baku, dan **logo digambar ulang flat "Ubin Rute"** (PR #185) termasuk favicon dan ikon PWA. Versi aplikasi kini bersumber tunggal dari `web/package.json` dan badge jumlah tool CLI dihitung dari registri backend, bukan angka statis (PR #184).
- **Konsolidasi permukaan (Fase 2, PR #175–#181)**: halaman Diagnostics dipensiunkan ke seksi Kesehatan Sistem di Observabilitas, kontrol cache dipindah ke tab Performa & Cache di Pengaturan, panel circuit breaker menampilkan konfigurasi efektif dari aturan prioritas tertinggi, dan 23 alias rute API lama yang terbukti nol konsumen dipangkas (3 alias baca dipertahankan).

### Removed
- **Aplikasi mobile native (`apps/mobile/`) dihapus** sebagai bagian dari redesign UI/UX ala Muse: satu antarmuka web responsif + PWA menjadi satu-satunya klien resmi Route-X untuk desktop dan mobile. Job CI `mobile` dan seluruh referensi ke `apps/mobile/` dibersihkan. Alias API backend yang sebelumnya dipakai companion app (`/overview`, `/diagnostics`, `/summary`) dipertahankan karena juga dipakai CLI dan integrasi pihak ketiga. Kode mobile terakhir sebelum penghapusan ditandai tag `pre-mobile-removal`.

## [v1.4.0] - 2026-10-07

Rilis besar yang menghadirkan Redesain Total UI/UX berestetika DeepSeek & Hermes, spatial UI pada inspeksi permintaan (Requests), penguatan sanitasi token zero-leakage, touch target optimal untuk perangkat seluler, pembersihan repo hygiene, serta sinkronisasi dokumentasi lintas aspek.

### Added
- **Total UI/UX Redesign — DeepSeek & Hermes Aesthetic (PR #122)**:
  - Transformasi visual komprehensif pada 15+ halaman web dan mobile companion app mengadopsi estetika DeepSeek & Hermes: tipografi presisi, aksen biru/indigo sleek (`#3B82F6`), kartu berkontur subtle, dan transisi mulus.
  - Redesain halaman login elegan dengan kartu setup onboarding aman dan pemisahan kredensial.
  - Optimasi touch targets mobile (≥44px) dan safe-area insets responsif lintas perangkat genggam.
- **Spatial UI & Waterfall Inspector pada Halaman Requests (PR #108)**:
  - Antarmuka spatial logs dengan visualisasi latensi waterfall per-tahap (DNS, TCP, TLS, TTFT, Total).
  - Virtual scrolling performa tinggi untuk puluhan ribu log permintaan tanpa degradasi frame rate.
- **Arsitektur UI/UX Obsidian Noir SOTA (PR #116)**:
  - Standardisasi sistem navigasi Drawer dan TopBar mobile responsif serta eliminasi scroll horizontal pada seluruh breakpoint.

### Changed
- **Penyelarasan Dokumentasi & Repo Hygiene**:
  - Penyelarasan nomor versi rilis `v1.4.0` secara konsisten pada backend Go (`cmd/ai-gateway`), spesifikasi OpenAPI 3.1 (`docs/openapi.yaml`), dashboard frontend (`web/package.json`), aplikasi mobile (`apps/mobile/package.json`), probe diagnostic (`probe_e2e_gateway.sh`), `Makefile`, dan `README.md`.
  - Pembaruan tema antarmuka interaktif Scalar pada `/docs` (`docs/embed.go`) menyelaraskan warna aksen dan palet tema dengan estetika DeepSeek.
  - Perbaikan referensi batas migrasi SQL di `AGENTS.md` (diperbarui hingga migrasi ke-15 `0015_credential_egress_pool.sql`).
  - Pembersihan berkas skrip scratch lama dari pohon Git (`redesign.py`, `fix_types.py`, `scratch_hash.go`, `scratch/refactor.js`).

### Security
- **Penegakan Zero-Leakage Session Token**:
  - Penghapusan token sesi dari body respon JSON `/login` dan `/logout`; autentikasi murni dikelola melalui cookie aman berkategori `__Host-` (`HttpOnly`, `Secure`, `SameSite=Lax`).
  - Endpoint `/api/auth/setup-hint` diamankan untuk hanya mengekspos alamat email awal tanpa pernah membocorkan kata sandi awal secara publik ke browser.

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
