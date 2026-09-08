# Dokumen Kendali Audit dan Rencana Remediasi Route-X

**Tanggal:** 2026-09-08
**Status audit:** Selesai untuk fase M0–M7 (rekonstruksi M3/M4/M6 dari source aktual + penyelesaian M5); M8 tetap diblokir
**Status release sementara:** **NO-GO**

## Tujuan

Dokumen ini menjadi sumber kendali untuk:

- mencatat progres audit dan remediasi Route-X secara bertahap;
- memisahkan fakta yang sudah terbukti dari area yang belum diaudit;
- menyimpan register temuan dengan ID stabil, dampak, rancangan perbaikan, acceptance criteria, dan validasi;
- mengatur dependency serta quality gate sebelum keputusan release dibuat.

Dokumen ini bukan deklarasi bahwa Route-X production-ready. Status release tetap **NO-GO** sampai seluruh blocker, khususnya rotasi kredensial yang pernah tersimpan plaintext, telah diselesaikan dan bukti validasinya dicatat.

## Scope

Scope audit lengkap mencakup backend Go/Chi, frontend React/TypeScript, PostgreSQL/Redis dan keamanan data, deployment/infrastruktur serta production readiness, integrasi lintas komponen, konsolidasi temuan, dan validasi akhir.

Aktivitas yang sudah dilakukan sebelum dokumen ini dibuat terbatas pada inventaris, penghentian deployment uji coba, cleanup artefak lokal, dan penguatan `.gitignore`. Perbaikan produk tidak termasuk dalam fase dokumentasi ini.

## Legenda Status

| Status | Arti |
|---|---|
| `DONE` | Scope milestone selesai dan bukti ringkas tersedia. |
| `ACTIVE` | Sedang dikerjakan; hasil belum lengkap dan belum boleh dianggap final. |
| `PENDING` | Belum dimulai atau menunggu dependency. |
| `BLOCKED` | Tidak dapat dilanjutkan sampai dependency atau keputusan tertentu terpenuhi. |
Status temuan menggunakan `OPEN`, `REMEDIATION-PENDING`, `RESOLVED`, dan `VALIDATION-PENDING`. `RESOLVED` hanya dipakai bila tindakan yang direncanakan telah dilakukan dan bukti minimum tersedia; status ini tidak menggantikan validasi audit akhir.

## Prinsip Keselamatan dan Invarian

- Jangan membaca, menampilkan, mencatat, atau memasukkan isi `.env`, token, cookie, URL ber-userinfo/query key, maupun kredensial ke log, error, artefak audit, atau dokumen ini.
- Rahasia provider/proxy wajib memakai AES-256-GCM dengan AAD yang terikat ke entitas.
- Semua koneksi upstream, webhook, discovery, egress, dan probe outbound wajib melewati `security.SSRFPolicy`; dilarang memakai `http.Transport` bebas tanpa guarded dialer.
- Semua nilai uang wajib memakai `upstream.USD` dengan integer skala 1e-8 USD, bukan `float64`.
- Migrasi yang sudah diterapkan tidak boleh diubah karena checksum diverifikasi. Migrasi dan job singleton wajib memakai PostgreSQL advisory lock pada koneksi sesi yang sama sampai unlock.
- Worker wajib menangkap panic agar proses tetap hidup dan mencatat hasil pada metrik `routex_worker_*`.
- Build produksi wajib melalui `make build` agar `web/dist` dibangun dan ditanam ke biner; `go build ./cmd/ai-gateway` saja bukan bukti artefak produksi.
- Overlay production `docker-compose.prod.yml` wajib memakai `PUBLIC_URL=https://...`; `.env` development tidak boleh disalin ke production.
- Uji yang dapat mengubah data atau konfigurasi hanya dijalankan pada target disposable yang dinyatakan aman. Jangan menjalankan E2E nyata atau load test terhadap target yang belum dikonfirmasi aman.
- Semua perubahan pengguna di worktree harus dilindungi. Jangan menghapus, mengembalikan, atau memformat perubahan yang tidak terkait.
- Dokumentasi internal menggunakan bahasa Indonesia; identifier dan command dipertahankan apa adanya.

## Status Milestone

| Milestone | Status | Tanggal | Ringkasan dan bukti |
|---|---|---|---|
| M0 - Inventaris worktree/runtime | `DONE` | 2026-09-08 | Inventaris awal worktree dan runtime selesai. Perubahan pengguna yang sudah ada diidentifikasi untuk dilindungi; tidak ada klaim audit kode dari milestone ini. |
| M1 - Hentikan deployment uji coba | `DONE` | 2026-09-08 | Deployment Route-X Compose dihentikan secara graceful dengan container dan data tetap dipertahankan. Xray tercatat `Exited (137)` dan dipisahkan sebagai temuan operasional untuk investigasi. |
| M2 - Cleanup artefak lokal | `DONE` | 2026-09-08 | Artefak scratch, Playwright reports/results, skrip audit ad hoc, dan binary root dibersihkan. `.gitignore` memiliki ignore terarah untuk `/scratch/`, `/web/playwright-report/`, `/web/test-results/`, `/ai-gateway`, dan `/routex-rotate`. Dua skrip ad hoc yang dihapus diketahui pernah memuat kredensial plaintext; nilai rahasia tidak dicatat. |
| M3 - Audit backend | `DONE` | 2026-09-08 | Rekonstruksi read-only selesai dari source aktual: BE-001–BE-003 `HIGH`, BE-004 `HIGH` (correctness cache), BE-005–BE-006 `MEDIUM`, BE-007–BE-010 `LOW/MEDIUM/INFO`. Klaim lama 4/4/2 tidak dapat dipetakan 1:1 karena artefak audit lama hilang; angka baru berbasis bukti saat ini. `gofmt`/`go vet` bersih; `go test -short ./...` lulus kecuali satu test timing flaky (`TestConstantTimeComparison`, bukan bug implementasi). |
| M4 - Audit frontend | `DONE` | 2026-09-08 | Rekonstruksi read-only selesai: 0 `CRITICAL` terkonfirmasi, 6 `HIGH` (FE-001–FE-006), 7 improvement (FE-101–FE-107). Klaim `CRITICAL` lama tidak terbukti dari kode saat ini. Build frontend tidak dijalankan (menulis `web/dist`, melanggar batas read-only). |
| M5 - Audit data/security | `DONE` | 2026-09-08 | Audit read-only selesai dari source dan migrasi aktual: DATA-001 `HIGH` (API key CLI plaintext di settings), DATA-002 `MEDIUM` (URL webhook mentah di audit), DATA-003 `MEDIUM/LOW` (validasi SSRF hanya saat dispatch), DATA-004 `MEDIUM` (jalur payload request mati). Positif: AES-256-GCM + AAD di 3 tabel, sesi/CSRF/Argon2id/RBAC/audit append-only baik. Inspeksi DB live `BLOCKED` (MCP auth gagal; `.env` tidak dibaca). SEC-001 tetap blocker. |
| M6 - Audit infra/production readiness | `DONE` | 2026-09-08 | Rekonstruksi read-only selesai dengan status **NO-GO**: SEC-002 + OPS-003 `CRITICAL`, 8 `HIGH` (OPS-004–OPS-011), 4 `MEDIUM` (OPS-012–OPS-015), 3 needs-confirmation. Runtime tetap berhenti; tidak ada validasi runtime destruktif. |
| M7 - Integrasi dan konsolidasi | `DONE` | 2026-09-08 | Register final terkonsolidasi: BE-001–BE-010, FE-001–FE-006 + FE-101–FE-107, DATA-001–DATA-004, SEC-002, OPS-003–OPS-015, plus SEC-001/OPS-001/REPO-001/OPS-002 lama. Deduplikasi terhadap SEC-001/OPS-001 dilakukan; regression terarah batch remediasi pertama sudah hijau. |
| M8 - Validasi akhir dan keputusan release | `BLOCKED` | - | Diblokir oleh tindakan operator SEC-001/SEC-002/DATA-001 (rotasi/cabut dan bukti non-rahasia), smoke test OPS-003 pada PostgreSQL TLS disposable, serta validasi runtime OPS-001. Gate source/unit/build sudah hijau. |
| M9 - Remediasi P0 batch 1 | `DONE` | 2026-09-08 | SEC-002, DATA-001, BE-001–BE-004 dan OPS-003 dipatch; BE-005/BE-006, DATA-002/DATA-003, OPS-004/OPS-005/OPS-006/OPS-007/OPS-009, FE-001/FE-003/FE-005/FE-006 ikut diremediasi sebagian atau penuh. Unit test seluruh repository, vet, frontend build, Docker build, validasi Compose, dan shell syntax hijau. Tindakan rotasi/operator dan smoke test runtime tetap terpisah. |
| M10 - Hardening lanjutan dan koreksi audit | `DONE` | 2026-09-08 | Audit ulang menemukan gap source SEC-002/DATA-001: state Xray plaintext/world-writable, settings generik, rotasi ciphertext settings, command injection skrip CLI, cache lintas policy, dan error discovery. Patch lokal diterapkan: state Xray AES-GCM + AAD, redaksi/RBAC, izin 0700/0600, rotator atomik settings, namespace settings terreservasi, shell quoting/validasi URL, scrub file legacy, cache dipaksa nonaktif, error discovery disanitasi, serta preflight CA bare-metal. Gate unit/vet/frontend/build hijau; release tetap NO-GO karena tindakan operator dan runtime disposable. |

## Fase Audit Lengkap

Setiap fase harus memperbarui tabel milestone, register temuan, dependency graph, dan bukti validasi pada hari fase selesai. Temuan baru tidak boleh ditulis sebagai fakta tanpa bukti yang dapat direproduksi dan aman dari secret.

### M3 - Backend (`DONE`)

Rekonstruksi read-only selesai langsung dari source aktual (artefak audit lama hilang bersama sesi `agy` yatim yang telah dihentikan graceful). Klaim lama 4 `HIGH`/4 `MEDIUM`/2 `LOW` tidak dipaksakan; temuan berbasis bukti saat ini adalah BE-001–BE-010 (detail di register). Area yang ditinjau ulang:

- routing Chi, urutan middleware, validasi input, autentikasi, otorisasi, CSRF, rate limit, dan sanitasi error;
- query pgx berparameter, transaksi `repo.InTx`, lifecycle pool eksplisit, advisory lock sesi-sama, serta migrasi checksum;
- penggunaan Redis (namespace key, TTL, `ContextTimeoutEnabled`), timeout, retry, idempotensi, dan fail-open saat dependency gagal;
- seluruh outbound path terhadap `security.SSRFPolicy` (2 bypass terkonfirmasi: BE-001, BE-002);
- representasi dan aritmetika uang terhadap `upstream.USD` (backend bersih; float hanya di metrik Prometheus yang terdokumentasi);
- recovery panic worker, shutdown, metrik `routex_worker_*`, logging, dan redaksi rahasia;
- unit/integration test yang hilang pada jalur kritis (satu test timing flaky: BE-010).

Verifikasi aktual: `gofmt -l` bersih, `go vet` bersih pada paket yang diaudit, `go test -short -count=1 ./...` lulus di seluruh paket kecuali `TestConstantTimeComparison` (`internal/auth`) yang flaky karena pengukuran wall-clock di host bersama — implementasinya memakai `subtle.ConstantTimeCompare` dan arah rasionya terbalik dari kebocoran nyata, jadi ini masalah reliabilitas test, bukan kerentanan. Test integrasi DB dilewati (`DATABASE_URL` tidak dipakai; `.env` tidak dibaca) dan dicatat sebagai skip, bukan `PASS`.

### M4 - Frontend (`DONE`)

Rekonstruksi read-only selesai dari source aktual: 0 `CRITICAL` terkonfirmasi, 6 `HIGH` (FE-001–FE-006), 7 improvement (FE-101–FE-107); detail di register. Klaim `CRITICAL` lama tidak terbukti dari kode saat ini dan tidak dipaksakan. Build frontend dan Playwright tidak dijalankan (menulis `web/dist`, melanggar batas read-only; tidak ada server/base URL yang dijalankan).

### M5 - Data dan Security (`DONE`)

Audit read-only selesai langsung dari source dan migrasi aktual setelah kegagalan sesi lama. Temuan: DATA-001–DATA-004 (detail di register). Area yang ditinjau:

- parameterisasi SQL (bersih; konkatenasi hanya untuk kolom/identifier internal dan nama schema test), error `repo.Error` yang meredaksi detail driver;
- AES-256-GCM dengan AAD terikat entitas (`CredentialAAD`/`WebhookAAD`) di `provider_credentials`, `webhooks`, `egress_pool`, plus rotasi satu-transaksi mencakup ketiganya;
- sesi 256-bit hash-only, cookie `HttpOnly` + prefiks `__Host-` di produksi + `SameSite=Lax`, CSRF double-submit + ikatan sesi + cek origin;
- Argon2id + anti-enumerasi + lockout di database; RBAC least-privilege 4 peran; audit log append-only dengan cuplikan pelaku;
- skrub DSN, `Secret` anti-cetak, sanitasi pesan error webhook.

Keterbatasan: inspeksi DB live `BLOCKED` (MCP postgres auth gagal; `.env`/kredensial tidak dibaca sesuai kebijakan). Grant/least-privilege koneksi, RLS, dan backup/restore belum terverifikasi dan dicatat needs-confirmation, bukan `PASS`. SEC-001 tetap `REMEDIATION-PENDING`; tidak ada upaya pemulihan secret dari riwayat.

- selesaikan SEC-001 melalui rotasi pada seluruh sistem sumber dan tujuan yang relevan tanpa mencatat nilainya;
- tentukan blast radius secara aman: jenis kredensial, sistem terdampak, periode keberadaan, dan kemungkinan akses, tanpa menyalin secret;
- audit encryption AES-256-GCM dan binding AAD, auth/RBAC, audit logging, parameterisasi SQL, serta redaksi log/error;
- audit schema, constraint, index, transaksi, migrasi, backup/restore, retention, dan least privilege;
- verifikasi outbound policy dan perlindungan SSRF lintas backend, webhook, discovery, egress, dan probe.

Output fase: bukti rotasi non-rahasia, temuan data/security, threat/risk assessment, dan hasil validasi terarah.

### M6 - Infra dan Production Readiness (`DONE`)

Audit production readiness read-only telah selesai dengan keputusan **NO-GO**. Ringkasan final: SEC-002 + OPS-003 `CRITICAL`, OPS-004–OPS-011 `HIGH` (8), OPS-012–OPS-015 `MEDIUM` (4), plus 3 needs-confirmation; detail di register. Runtime tetap berhenti selama audit.

- investigasi penyebab Xray `Exited (137)` menggunakan metadata dan log yang sudah diredaksi;
- audit Compose/base overlay production, health check, dependency ordering, resource limit, restart policy, graceful shutdown, dan persistence;
- verifikasi `PUBLIC_URL=https://...`, TLS, network exposure, observability, alerting, rollback, backup, dan recovery;
- pastikan artefak build berasal dari `make build` dan tidak ada generated/local artifact yang masuk source control.

Output fase terkonsolidasi ke register M7. Bukti validasi terbatas pada inspeksi read-only dan konfirmasi runtime tetap berhenti; audit ini tidak mengklaim root cause OPS-001 atau uji runtime telah selesai.

### M7 - Integrasi dan Konsolidasi (`DONE`)

Konsolidasi selesai pada 2026-09-08: deduplikasi lintas fase tanpa mengganti ID stabil; kontrak backend/frontend dan alur PostgreSQL/Redis/upstream tervalidasi dari source; urutan patch di bawah disusun berdasar severity, dependency, dan kemampuan rollback; blocker release ditetapkan dengan acceptance criteria di tiap entri register. Deduplikasi eksplisit: DATA-001 berbeda dari SEC-001 (settings vs skrip ad hoc); H-03 infra (entrypoint Xray) dicatat sebagai kandidat penyebab OPS-001, bukan duplikat. Regression review menunggu patch aktual.

- deduplikasi temuan lintas fase tanpa mengganti ID stabil;
- validasi kontrak backend/frontend dan alur PostgreSQL/Redis/upstream;
- susun urutan patch berdasarkan dependency, blast radius, dan kemampuan rollback;
- tetapkan setiap blocker release, owner, status, acceptance criteria, dan bukti yang masih kurang;
- lakukan regression review setelah setiap kelompok perbaikan.

Output fase: register final, remediation backlog berurutan, matriks risiko residual, dan kandidat quality gate akhir.

### M8 - Validasi Akhir (`BLOCKED`)

- jalankan test terarah untuk semua perilaku yang berubah;
- jalankan quality gate repository;
- lakukan review security dan regression terakhir terhadap diff aktual;
- pastikan worktree tidak berisi artefak build atau perubahan tak disengaja;
- putuskan `GO` atau `NO-GO` berdasarkan bukti, bukan hanya keberhasilan build.

Output fase: bukti command dan hasil, daftar risiko residual yang diterima secara eksplisit, serta keputusan release bertanggal.

## Register Temuan Awal

Hanya temuan berikut yang telah terbukti pada saat dokumen ini dibuat. Audit mendalam dapat menambah temuan dengan ID baru; nomor yang sudah ada tidak boleh digunakan ulang.

### SEC-001 - Kredensial plaintext pada skrip audit ad hoc

| Atribut | Isi |
|---|---|
| Severity | **CRITICAL** |
| Status | `REMEDIATION-PENDING` / release blocker |
| Bukti aman | Dua skrip audit ad hoc yang telah dihapus diketahui memuat kredensial plaintext. Nama/nilai kredensial tidak dicatat dan tidak boleh dipulihkan ke dokumen atau output command. |
| Dampak | Kredensial harus dianggap berpotensi terekspos kepada pihak atau proses yang pernah dapat membaca file tersebut. Penghapusan file tidak mencabut kredensial, tidak menghapus salinan lain, dan tidak membuktikan bahwa kredensial belum digunakan. |
| Rancangan perbaikan | Identifikasi sistem penerbit dan seluruh consumer secara aman; cabut atau rotasi semua kredensial terkait; distribusikan pengganti melalui secret manager/environment yang sesuai; verifikasi consumer memakai nilai baru; periksa riwayat akses dan salinan yang mungkin ada tanpa menampilkan secret; dokumentasikan waktu rotasi dan referensi bukti non-rahasia. Jangan memasukkan secret ke shell history, log, tiket, commit, atau dokumen ini. |
| Acceptance criteria | Seluruh kredensial terkait dicabut/dirotasi; consumer tervalidasi memakai kredensial baru; kredensial lama gagal digunakan; tidak ada plaintext terkait pada tracked files maupun artefak lokal yang diaudit; blast radius dan review akses selesai; bukti disetujui reviewer security tanpa memuat nilai rahasia. |
| Validation | Cek status rotasi pada sistem sumber; uji autentikasi positif dengan mekanisme aman dan uji negatif kredensial lama; scan repository/worktree dengan scanner secret yang outputnya diredaksi; review audit log yang relevan tanpa menyalin token. Command spesifik harus dipilih sesuai secret manager dan target aman. |

Catatan severity: `CRITICAL` dipilih secara konservatif karena jenis, privilege, dan riwayat akses kredensial belum ditentukan. Severity boleh diturunkan hanya setelah rotasi selesai dan blast radius berbukti menunjukkan dampak lebih rendah; keputusan harus dicatat, bukan diasumsikan.

### OPS-001 - Xray keluar dengan status 137

| Atribut | Isi |
|---|---|
| Severity | **HIGH** |
| Status | `OPEN` |
| Bukti aman | Saat penghentian deployment uji coba, Xray tercatat `Exited (137)`. Container dan data dipertahankan untuk investigasi. |
| Dampak | Exit 137 konsisten dengan proses menerima `SIGKILL`, tetapi penyebabnya belum terbukti. Kemungkinan mencakup OOM kill, timeout saat shutdown, atau penghentian paksa; kondisi serupa dapat mengganggu availability dan graceful shutdown production. |
| Rancangan perbaikan | Kumpulkan metadata container, event runtime, status OOM, resource limit, urutan stop, timeout, health check, dan log teredaksi. Reproduksi hanya pada lingkungan disposable. Setelah root cause terbukti, perbaiki resource/shutdown/configuration secara minimal dan tambahkan validasi regresi. |
| Acceptance criteria | Root cause terdokumentasi dengan bukti; Xray berhenti normal pada shutdown terkontrol atau kebijakan exit non-zero dijelaskan dan diterima; restart/health behavior sesuai desain; tidak ada kehilangan data atau secret pada log; reproduksi tidak lagi menghasilkan exit 137. |
| Validation | Inspeksi metadata/runtime dan log teredaksi; uji start-health-stop berulang pada target disposable; observasi exit code, OOM flag, durasi shutdown, dan resource usage. Jangan menganggap exit 137 sebagai OOM tanpa bukti. |

Severity tetap `HIGH` sampai dampak availability dan penyebab diketahui; status milestone M1 tetap `DONE` karena tujuan M1 adalah penghentian deployment, bukan penyelesaian root cause Xray.

### REPO-001 - Artefak generated/local belum di-ignore

| Atribut | Isi |
|---|---|
| Severity | **LOW** |
| Status | `RESOLVED` |
| Bukti aman | Sebelum cleanup, scratch serta Playwright report/result membutuhkan pembersihan manual dan belum seluruhnya tercakup ignore terarah. `.gitignore` sekarang memuat `/scratch/`, `/web/playwright-report/`, dan `/web/test-results/`; binary root `/ai-gateway` dan `/routex-rotate` juga di-ignore. |
| Dampak | Artefak lokal dapat mengotori worktree, memperbesar risiko accidental commit, dan mencampur bukti audit dengan source product. |
| Rancangan perbaikan | Pertahankan ignore terarah dan cleanup artefak; jangan gunakan pola terlalu luas yang dapat menyembunyikan source valid. |
| Acceptance criteria | Path generated yang diketahui di-ignore; `web/dist/.gitkeep` tetap dapat dilacak; source product yang valid tidak ikut ter-ignore; worktree tidak menampilkan artefak tersebut setelah test/build yang relevan. |
| Validation | `git check-ignore -v scratch/ web/playwright-report/ web/test-results/ ai-gateway routex-rotate`; inspeksi `git status --short`; setelah build, pastikan hanya artefak yang diharapkan yang di-ignore dan `.gitkeep` tetap tersedia. |

### OPS-002 - Production stack aktif sebagai uji coba

| Atribut | Isi |
|---|---|
| Severity | **MEDIUM** |
| Status | `RESOLVED` untuk penghentian; `VALIDATION-PENDING` untuk readiness |
| Bukti aman | Deployment Route-X Compose yang digunakan sebagai uji coba telah dihentikan secara graceful pada 2026-09-08; container dan data dipertahankan. Xray exit 137 dilacak terpisah sebagai OPS-001. |
| Dampak | Stack uji coba yang dibiarkan aktif dapat mengekspos layanan, memakai resource, atau menimbulkan perubahan data yang tidak diinginkan. Penyimpanan container/data juga mempertahankan material yang perlu diperlakukan sensitif sampai lifecycle-nya diputuskan. |
| Rancangan perbaikan | Pertahankan stack dalam keadaan berhenti selama audit; sebelum aktivasi berikutnya, konfirmasi target aman, network exposure, kredensial, `PUBLIC_URL`, persistence, health check, dan rollback. Tentukan retention/cleanup container dan data setelah bukti investigasi tidak lagi diperlukan. |
| Acceptance criteria | Stack tetap berhenti sampai ada persetujuan uji; aktivasi berikutnya memakai target dan akun uji yang dinyatakan aman; exposure dan konfigurasi production diverifikasi; keputusan retention atau cleanup data dicatat; OPS-001 ditangani terpisah. |
| Validation | Inspeksi status Compose tanpa membuka environment atau secret; sebelum restart, review config hasil render yang sudah diredaksi; setelah restart yang disetujui, jalankan smoke test non-destruktif dan verifikasi shutdown. |

### BE-001 - SSRF bypass pada deteksi IP server

| Atribut | Isi |
|---|---|
| Severity | **HIGH** |
| Status | `VALIDATION-PENDING` (patch diterapkan; menunggu quality gate hijau) / release blocker |
| Bukti aman | `internal/admin/domain.go:58-59`: `&http.Client{Timeout: 2 * time.Second}` lalu `client.Get("https://api.ipify.org")` tanpa `security.SSRFPolicy`/`GuardedDialContext` dan tanpa kebijakan redirect. |
| Dampak | Satu-satunya jalur outbound backend yang tidak dijaga SSRF; respons/redirect tidak tervalidasi. |
| Rancangan perbaikan | Lewatkan panggilan ini melalui klien ber-SSRF dengan policy pabrik; kunci skema https; batasi redirect. |
| Acceptance criteria | Tidak ada `http.Client`/`http.Transport` tanpa guarded dialer di jalur produk; test negatif membuktikan host internal ditolak. |
| Validation | `grep` pola transport tanpa penjaga; unit test SSRF menolak IP terlarang. |

### BE-002 - SSRF bypass pada probe egress pool

| Atribut | Isi |
|---|---|
| Severity | **HIGH** |
| Status | `VALIDATION-PENDING` (patch diterapkan; menunggu quality gate hijau) / release blocker |
| Bukti aman | `internal/admin/upstreams.go:1597-1605`: `&http.Transport{Proxy: ...}` tanpa `DialContext` berpenjaga; probe berjalan lewat proxy operator tanpa pemeriksaan dial. |
| Dampak | Probe via proxy jahat/terkompromi tidak diperiksa terhadap policy; inkonsisten dengan dispatcher webhook dan klien provider. |
| Rancangan perbaikan | Pakai `security.GuardedDialContext` dengan policy pabrik pada transport probe. |
| Acceptance criteria | Seluruh transport produk memakai guarded dialer; tidak ada regresi fungsi probe pada proxy sah. |
| Validation | Inspeksi transport + test probe terhadap proxy disposable. |

### BE-003 - API key Google disematkan di query URL discovery

| Atribut | Isi |
|---|---|
| Severity | **HIGH** |
| Status | `VALIDATION-PENDING` (patch diterapkan; menunggu quality gate hijau) / release blocker |
| Bukti aman | `internal/admin/upstreams.go:611-614` menyusun `discoveryURL` dengan `?key=` dari kredensial; URL yang sama di-log mentah (`:644`) dan dikembalikan di pesan error 400 (`:647`, plus `:658`). Header `x-goog-api-key` sudah dipasang (`:635`), sehingga query tidak diperlukan. |
| Dampak | Kredensial masuk log aplikasi dan respons API admin. |
| Rancangan perbaikan | Hapus `?key=` dari URL; hanya kirim via header; sanitasi URL sebelum log/error. |
| Acceptance criteria | Tidak ada kredensial di URL pada kode discovery; log/error bebas query sensitif; discovery Google tetap berfungsi. |
| Validation | Test discovery memakai header saja; grep tidak menemukan kredensial di URL; inspeksi log teredaksi. |

### BE-004 - Kunci cache respons tidak mencakup seluruh parameter request

| Atribut | Isi |
|---|---|
| Severity | **HIGH** (correctness) |
| Status | `VALIDATION-PENDING` (patch diterapkan; menunggu quality gate hijau; keputusan wire-or-remove didokumentasikan di kode) / release blocker |
| Bukti aman | `internal/responsecache/responsecache.go:93-128` (`ComputeKey`) mengabaikan `TopP`, `Stop`, `ToolChoice`, `ParallelToolCalls`, `ResponseFormat`, `Seed`, `ReasoningEffort`, `User`, `Extra`, serta `Name`/`ToolCalls`/`ReasoningContent`/konten non-teks pesan. `GetOrFetch` tidak punya pemanggil produksi (jalur mati). |
| Dampak | Tabrakan cache antar request berbeda; fitur tidak terukur karena jalurnya mati. |
| Rancangan perbaikan | Sertakan seluruh field pembeda ke kunci (atau hash kanonik body) ATAU putuskan: sambungkan `GetOrFetch` ke jalur inferensi dengan filter header, atau hapus endpoint/tabel payload yang tidak pernah terisi. |
| Acceptance criteria | Tidak ada dua request berbeda yang berbagi kunci; keputusan wire-or-remove terdokumentasi; test tabrakan negatif lulus. |
| Validation | Unit test kunci untuk tiap parameter; review diff. |

### BE-005 - TriggerJob manual membuang context pemanggil

| Atribut | Isi |
|---|---|
| Severity | **MEDIUM** |
| Status | `OPEN` |
| Bukti aman | `internal/worker/worker.go:235-257`: `TriggerJob(ctx, ...)` mengeksekusi `go s.executeOnce(context.Background(), target)`; `request_id`/logger dan pembatalan pemanggil hilang. |
| Dampak | Log trigger manual tidak terkorelasi; shutdown tidak membatalkan trigger yang baru dipicu. |
| Rancangan perbaikan | Pakai `context.WithoutCancel(ctx)` agar nilai terwarisi tanpa terikat pembatalan HTTP. |
| Acceptance criteria | Trigger manual tercatat dengan `request_id` pemicu; tidak ada goroutine liar saat shutdown. |
| Validation | Unit test supervisor; inspeksi log terkorelasi. |

### BE-006 - Audit log memakai RemoteAddr mentah, bukan IP klien teresolusi

| Atribut | Isi |
|---|---|
| Severity | **MEDIUM** |
| Status | `OPEN` |
| Bukti aman | `internal/admin/admin.go:174-177` (`writeAuditTx`) memakai `r.RemoteAddr`; middleware `RealIP` sudah meresolusi IP tepercaya ke context (`httpx.ClientIPFrom`). |
| Dampak | IP pelaku di audit salah di belakang proxy; korelasi insiden melemah. |
| Rancangan perbaikan | Utamakan `httpx.ClientIPFrom(ctx)`, fallback ke parsing `RemoteAddr` sekarang. |
| Acceptance criteria | Audit mencatat IP klien teresolusi; tidak ada perubahan perilaku bila `RealIP` tidak dipasang. |
| Validation | Test handler dengan `X-Forwarded-For` + proxy tepercaya; inspeksi baris audit. |

### BE-007 - Fail-open saat Redis gagal (rate limit & circuit breaker)

| Atribut | Isi |
|---|---|
| Severity | **MEDIUM** (keputusan desain; butuh penerimaan risiko eksplisit) |
| Status | `OPEN` |
| Bukti aman | `internal/apikey/limiter.go:840-847` (`failOpenDecision`) dan `internal/gateway/executor.go:304-309` (guard error → izin). Kegagalan dicatat di log + metrik, tetapi lalu lintas diloloskan tanpa batas. |
| Dampak | Outage Redis = penegakan rate limit/breaker nonaktif; potensi abuse saat insiden. |
| Rancangan perbaikan | Pertahankan atau ubah per keputusan operator; minimal: alert pada metrik fail-open dan dokumentasikan di runbook. |
| Acceptance criteria | Keputusan tercatat; alert fail-open terpasang; perilaku terdokumentasi. |
| Validation | Uji chaos Redis mati; verifikasi alert. |

### BE-008 - Label Prometheus dari nama terkurasi operator

| Atribut | Isi |
|---|---|
| Severity | **LOW** |
| Status | `OPEN` |
| Bukti aman | `internal/worker/health.go:167-168`, `internal/usage/record.go:216-253`, `internal/gateway/guard.go:190`: label memakai nama provider/model/filter dari database. Terbatas oleh kurasi, tetapi admin dapat menambah entri sembarang. |
| Dampak | Kardinalitas time series tumbuh seiring entri operator; tidak kritis jangka pendek. |
| Rancangan perbaikan | Dokumentasikan batas; pertimbangkan normalisasi label dan dashboard kardinalitas. |
| Acceptance criteria | Panduan batas entri; tidak ada label dari input klien bebas (sudah dipatuhi). |
| Validation | Inspeksi `/metrics` teredaksi; review label. |

### BE-009 - CORS wildcard pada permukaan /v1

| Atribut | Isi |
|---|---|
| Severity | **INFO** |
| Status | `OPEN` (dokumentasi) |
| Bukti aman | `cmd/ai-gateway/main.go:521`, `internal/gateway/http.go:180`: `httpx.CORS(nil)` → `Access-Control-Allow-Origin: *`. |
| Dampak | Disengaja untuk klien web pihak ketiga; aman karena auth Bearer (bukan cookie) dan tanpa `Allow-Credentials`. |
| Rancangan perbaikan | Dokumentasikan keputusan; pastikan tidak ada cookie/CSRF di /v1 (sudah dipatuhi). |
| Acceptance criteria | Keputusan tercatat; tidak ada kredensial otomatis di /v1. |
| Validation | Inspeksi header preflight. |

### BE-010 - Test timing CSRF flaky di host bersama

| Atribut | Isi |
|---|---|
| Severity | **LOW** (reliabilitas gate) |
| Status | `VALIDATION-PENDING` |
| Bukti aman | `internal/auth/csrf_test.go:304-362` (`TestConstantTimeComparison`): gagal di host audit (rasio 1.49–1.67 > toleransi 1.3) tetapi lulus pada run gabungan; arah rasio terbalik (awal lebih lambat) sehingga bukan kebocoran early-exit. Implementasi memakai `subtle.ConstantTimeCompare` (benar). |
| Dampak | Gate merah palsu mengikis kepercayaan; kegagalan nyata bisa terabaikan. |
| Rancangan perbaikan | Toleransi wall-clock dilonggarkan menjadi 2.0 setelah kontrol early-exit membuktikan instrumen tetap peka; implementasi `subtle.ConstantTimeCompare` tidak diubah. |
| Acceptance criteria | Test stabil di host bersama; implementasi tetap constant-time. |
| Validation | `go test -short -count=3 ./internal/auth` hijau berulang. |

### FE-001 - Error API penting disamarkan menjadi keadaan normal/kosong

| Atribut | Isi |
|---|---|
| Severity | **HIGH** |
| Status | `OPEN` |
| Bukti aman | `web/src/pages/Dashboard.tsx:53-65` (error → `null`, UI tetap "GATEWAY OPERATIONAL" `:162-170`); `web/src/pages/Providers.tsx:67-73`; `web/src/pages/RoutingRules.tsx:41-53`; `web/src/pages/Diagnostics.tsx:24-38`; `web/src/pages/CLIIntegrations.tsx:72-100`. Error hanya masuk `console.error`. |
| Dampak | Admin tidak membedakan "belum ada data" dari API gagal/sesi habis; risiko keputusan operasional salah. |
| Rancangan perbaikan | Error state eksplisit per resource; tampilkan status operasional hanya bila endpoint kesehatan sukses dan segar. |
| Acceptance criteria | Setiap kegagalan endpoint wajib terlihat pengguna dengan aksi retry; tidak ada empty state palsu. |
| Validation | Uji UI dengan API gagal (test terarah/Playwright pada target disposable). |

### FE-002 - Fallback data/konfigurasi produksi yang tidak berasal dari API

| Atribut | Isi |
|---|---|
| Severity | **HIGH** |
| Status | `OPEN` |
| Bukti aman | `web/src/pages/Settings.tsx:160` (IP publik fallback literal); `:351-496` (daftar protokol/endpoint/port Xray saat API kosong); `web/src/pages/CLIIntegrations.tsx:489-493`, `:509-512`, `:522-538` (model/routing fallback). |
| Dampak | UI menyajikan endpoint/IP/konfigurasi yang tidak benar-benar tersedia; admin bisa menyalin konfigurasi invalid. |
| Rancangan perbaikan | Empty state jujur; preset hanya sebagai template eksplisit berlabel; hapus IP literal dari frontend. |
| Acceptance criteria | Tidak ada data operasional yang dikarang klien; setiap tampilan berlabel sumbernya (live vs template). |
| Validation | Review UI dengan API kosong; grep pola fallback. |

### FE-003 - Nilai uang dikonversi ke float di UI

| Atribut | Isi |
|---|---|
| Severity | **HIGH** |
| Status | `OPEN` |
| Bukti aman | `web/src/pages/Budgets.tsx:122-149`; `web/src/pages/Requests.tsx:317`, `:389`; `web/src/pages/Dashboard.tsx:107-111` memakai `parseFloat` + `toFixed`; kontrak menyimpan string desimal USD skala 8. |
| Dampak | Floating-point mengubah nilai; klaim "Presisi 8 desimal USD" (`Dashboard.tsx:575-577`) tidak ditepati tampilannya. |
| Rancangan perbaikan | Formatter desimal fixed-point/string tanpa `Number`/`parseFloat`; aturan digit disepakati backend. |
| Acceptance criteria | Nilai tampil = nilai kontrak; tidak ada `parseFloat` pada jalur moneter. |
| Validation | Test formatter vs vektor kontrak; grep `parseFloat` pada halaman moneter. |

### FE-004 - TanStack Query parsial tanpa error state

| Atribut | Isi |
|---|---|
| Severity | **HIGH** |
| Status | `OPEN` |
| Bukti aman | `useQuery` hanya di `web/src/pages/APIKeys.tsx:15-19` dan `Providers.tsx:58-74`; keduanya tanpa render `isError`; mayoritas halaman memakai `useEffect` + state manual. |
| Dampak | Cache/query-key tidak seragam; kegagalan query tampil sebagai daftar kosong. |
| Rancangan perbaikan | Migrasi ke hook Query terstandar + query-key factory; tangani `isPending`/`isError`/stale; invalidasi tepat sasaran. |
| Acceptance criteria | Seluruh resource memakai pola Query yang sama dengan error state. |
| Validation | Review pola + uji kegagalan query. |

### FE-005 - Modal konfirmasi kustom tidak aksesibel keyboard

| Atribut | Isi |
|---|---|
| Severity | **HIGH** |
| Status | `OPEN` |
| Bukti aman | `web/src/context/ToastContext.tsx:162-215` (`confirmModal` tanpa `role="dialog"`, focus trap, Escape, restore focus); pembanding benar `web/src/components/common/Modal.tsx:48-76`. |
| Dampak | Pengguna keyboard/screen reader berisiko pada aksi destruktif (revoke key, hapus provider, flush cache). |
| Rancangan perbaikan | Pakai komponen `Modal`/focus-trap yang sama; `aria-labelledby`/`describedby`, fokus awal aman, Escape batal, restore focus. |
| Acceptance criteria | Dialog konfirmasi lulus audit keyboard + screen reader. |
| Validation | Uji keyboard manual + Playwright. |

### FE-006 - Token CSRF diduplikasi ke sessionStorage

| Atribut | Isi |
|---|---|
| Severity | **HIGH** (perilaku klien confirmed; risiko akhir needs-confirmation) |
| Status | `OPEN` |
| Bukti aman | `web/src/api/client.ts:55-84`; `web/src/context/AuthContext.tsx:24-32`, `:49-56`. |
| Dampak | Token yang cukup via cookie menjadi dapat dibaca skrip; dampak penuh tergantung atribut cookie, CSP, validasi server. |
| Rancangan perbaikan | Konfirmasi kontrak backend dahulu; bila double-submit cookie, hapus fallback storage, simpan di memori; tambah CSP ketat. |
| Acceptance criteria | Keputusan kontrak tercatat; tidak ada salinan token di storage persisten kecuali disetujui. |
| Validation | Uji kegagalan/rotasi token; review CSP. |

### FE-101–FE-107 - Improvement frontend

| ID | Ringkasan | Lokasi bukti |
|---|---|---|
| FE-101 | Error/loading/empty state tidak konsisten | `Budgets.tsx:26-37`, `Observability.tsx:26-46`, `Settings.tsx:42-78`, `Egress.tsx:54-68` |
| FE-102 | Toast/pesan tanpa live region screen reader | `ToastContext.tsx:92-160`, `Login.tsx:47-52` |
| FE-103 | Grafik Recharts tanpa alternatif aksesibel | `Dashboard.tsx:666-704`, `Observability.tsx:104-140` |
| FE-104 | Elemen interaktif non-semantik + handler keyboard berulang | `Requests.tsx:227-234`, `Dashboard.tsx:748-755` |
| FE-105 | Label form tidak terhubung ke input | `Login.tsx:56-80`, `APIKeys.tsx:259-285`, `Budgets.tsx:198-255` |
| FE-106 | Clipboard tanpa penanganan failure | `APIKeys.tsx:90-94`, `Requests.tsx:67-75`, `CommandPalette.tsx:193-198` |
| FE-107 | E2E Playwright placeholder, route/port tidak selaras | `web/tests/e2e/*.spec.ts`, `playwright.config.ts:10-12`, `vite.config.ts:19-30` |

Status seluruhnya `OPEN`; acceptance: komponen state terstandar, live region, tabel alternatif grafik, tombol semantik, label terhubung, toast error clipboard, E2E terhadap hash route nyata + backend disposable.

### DATA-001 - API key CLI tersimpan plaintext di tabel settings

| Atribut | Isi |
|---|---|
| Severity | **HIGH** |
| Status | `VALIDATION-PENDING` (kode enkripsi/redaksi, scrub file legacy, namespace settings terreservasi, dan rotasi ciphertext settings selesai; rotasi key lama = aksi operator) / release blocker |
| Bukti aman | `internal/cliconfig/manager.go:41-48` (`StoredConfig.APIKey`), `:146-170` (disimpan via `settings.Put` ke kunci `cli:config:*`); migrasi `0001` menegaskan settings "Bukan tempat menyimpan rahasia". Nilai kembali mentah via `GET /system/settings` (`toSettingDTO` tanpa redaksi, perlu `settings:read` = Viewer+), via respons `Configure` (`EnvVars`+`ExportSnippet`), via `GET /cli/env-export`, dan tertulis ke `~/.routex/cli-env.sh` di host. Berbeda dari SEC-001 (skrip ad hoc), jadi ID baru. |
| Dampak | Eskalasi baca: peran read-only dapat membaca API key; rahasia di DB/file/API tanpa enkripsi/AAD/audit. |
| Rancangan perbaikan | Jangan simpan API key (minta ulang tiap konfigurasi) atau enkripsi via `security.Cipher` + AAD terikat tool; redaksi `EnvVars`/snippet pada respons dan settings; rotasi key yang pernah tersimpan; bersihkan file host. |
| Acceptance criteria | Tidak ada API key plaintext di settings/file/respons; peran read-only tidak dapat membaca key; key lama dirotasi. |
| Validation | Inspeksi settings teredaksi; test negatif peran Viewer; scan secret teredaksi. |

### DATA-002 - URL webhook mentah (termasuk query) masuk audit abadi

| Atribut | Isi |
|---|---|
| Severity | **MEDIUM** |
| Status | `OPEN` |
| Bukti aman | `internal/admin/automation.go:117`: metadata audit `{"name","url": wh.URL}`; `audit_logs` append-only (retention tidak menyentuhnya). Token di query URL webhook akan tersimpan selamanya. |
| Dampak | Rahasia di query string bertahan di audit yang tidak pernah dibersihkan; terbaca via `audit:read`. |
| Rancangan perbaikan | Sanitiasi URL sebelum audit (buang query + samarkan userinfo, pakai `SanitizeErrorMessage` atau helper URL). |
| Acceptance criteria | Audit webhook tidak memuat query/userinfo; test sanitasi lulus. |
| Validation | Unit test + inspeksi baris audit. |

### DATA-003 - URL webhook tidak divalidasi SSRF saat create/update

| Atribut | Isi |
|---|---|
| Severity | **MEDIUM/LOW** |
| Status | `OPEN` |
| Bukti aman | `internal/webhooks/webhooks.go:143-205` (`Create`) tidak memanggil `security.ValidateBaseURL`; penolakan SSRF baru terjadi di `Dispatcher.Dispatch`/`Ping`. |
| Dampak | URL internal tersimpan; gagal-tertutup saat kirim (mitigasi ada) tetapi validasi telat. |
| Rancangan perbaikan | Validasi policy SSRF saat create/update (defense in depth). |
| Acceptance criteria | URL melanggar policy ditolak saat tulis dengan pesan aman. |
| Validation | Test create negatif. |

### DATA-004 - Jalur payload request mati (tidak pernah ditulis)

| Atribut | Isi |
|---|---|
| Severity | **MEDIUM** (kontrak) |
| Status | `OPEN` |
| Bukti aman | `Repo.SavePayload` (`internal/database/repo/traffic/traffic.go:279`) tidak punya pemanggil produksi; `GET /requests/{id}/payload` selalu 404; retention tetap menghapus `request_payloads`. Komentar mewajibkan filter header oleh pemanggil, tetapi tidak ada pemanggil yang menegakkannya. |
| Dampak | Endpoint mati; bila disambung tanpa filter, header `Authorization`/cookie bisa masuk DB. |
| Rancangan perbaikan | Sambungkan dengan filter header tersentralisasi + test, ATAU hapus endpoint/tabel/retensi payload. |
| Acceptance criteria | Keputusan wire-or-remove terdokumentasi; bila wired, tidak ada header sensitif tersimpan. |
| Validation | Test payload + review diff. |

### SEC-002 - Material autentikasi Xray tertanam dan terlacak Git

| Atribut | Isi |
|---|---|
| Severity | **CRITICAL** |
| Status | `REMEDIATION-PENDING` (tracked template dibersihkan; state runtime kini terenkripsi AES-256-GCM + AAD, API settings diredaksi/dibatasi, file 0700/0600; rotasi kredensial aktif dan pembersihan riwayat = aksi operator) / release blocker |
| Bukti aman | `deploy/xray/config.json:20`, `:41`, `:63` memuat identifier klien dan password tunnel (nilai tidak direproduksi); `git ls-files` memastikan terlacak sejak commit `420c874`. |
| Dampak | Clone/riwayat repo = akses tunnel potensial; hapus di commit baru tidak mencabut riwayat. |
| Rancangan perbaikan | Rotasi/cabut seluruh kredensial Xray; audit log pemakaian; pindah ke secret store/file runtime berizin; sediakan template tanpa nilai; bersihkan riwayat bila repo pernah dibagikan. |
| Acceptance criteria | Kredensial lama tidak berlaku; tidak ada nilai di tracked files maupun riwayat yang dibagikan; bukti non-rahasia disetujui reviewer. |
| Validation | Uji negatif kredensial lama; scan secret teredaksi. |

### OPS-003 - Compose produksi mensyaratkan TLS PostgreSQL yang tidak disediakan

| Atribut | Isi |
|---|---|
| Severity | **CRITICAL** (konfigurasi confirmed; kausal runtime needs-confirmation) |
| Status | `VALIDATION-PENDING` (Compose produksi kini mewajibkan PostgreSQL eksternal `verify-full` + CA mount; smoke test disposable belum dijalankan) / release blocker |
| Bukti aman | `docker-compose.yml:13`, `:34-50`, `docker-compose.prod.yml:7-9` menghasilkan `sslmode=require`; service postgres tanpa sertifikat/`ssl=on`; aplikasi menolak `sslmode=disable` di produksi (`internal/config/config.go:201-211`). |
| Dampak | Gateway stack sesuai repo tidak mencapai siap; konsisten dengan container exited/unhealthy (penyebab historis tidak diklaim). |
| Rancangan perbaikan | Postgres eksternal TLS atau konfigurasi TLS server + secret mount + hostname cocok; naikkan ke `verify-full`; smoke test migrasi + `/readyz` + TLS. |
| Acceptance criteria | Deploy bersih dari repo mencapai `/readyz`; koneksi TLS terbukti. |
| Validation | Deploy disposable + smoke test. |

### OPS-004–OPS-011 - Temuan HIGH infra/production

| ID | Ringkasan | Lokasi bukti |
|---|---|---|
| OPS-004 | Port 80 meneruskan plaintext untuk host non-domain | `deploy/caddy/Caddyfile:95-104`, `docker-compose.prod.yml:15-17` |
| OPS-005 | Grace period Docker < grace aplikasi (25 dtk) | `docker-compose.yml:23`, `internal/httpx/server.go:143-149` |
| OPS-006 | Entrypoint Xray (PID 1 shell loop tanpa trap) tidak meneruskan sinyal; kandidat penyebab OPS-001 | `deploy/xray/entrypoint.sh:3-17` |
| OPS-007 | Overlay menurunkan dependency Caddy healthy → started | `docker-compose.yml:92-96`, `docker-compose.prod.yml:22-23` |
| OPS-008 | Tanpa backup/restore/rollback operasional | `docker-compose.yml:42-43`, `README.md:130-135`, `deploy/scripts/setup_production.sh:53-63` |
| OPS-009 | Satu jaringan datar + Caddy admin `0.0.0.0:2019` + SOCKS noauth | `deploy/caddy/Caddyfile:5-9`, `docker-compose.yml`, `deploy/xray/config.json:4-11` |
| OPS-010 | Tanpa resource limit dan rotasi log container | `docker-compose.yml:1-113`, `docker-compose.prod.yml:1-31` |
| OPS-011 | Image produksi tidak immutable (mutable tags, xray `latest`) | `Dockerfile:8,22,46`, `docker-compose.yml:35,53,68,82` |

Status seluruhnya `OPEN`; OPS-001/OPS-003 menjadi blocker; OPS-006 terkait OPS-001 sebagai hipotesis utama (belum terbukti).

### OPS-012–OPS-015 - Temuan MEDIUM infra/production

| ID | Ringkasan | Lokasi bukti |
|---|---|---|
| OPS-012 | Secret aplikasi via environment Compose | `docker-compose.yml:13-20`, `deploy/production.env.example:24-54` |
| OPS-013 | Skrip development mencetak initial admin password | `scripts/docker-start.sh:94-104` |
| OPS-014 | Jalur build Docker menduplikasi (bukan `make build` kanonis) | `Makefile:25-32`, `Dockerfile:15-17,34-41` |
| OPS-015 | Hardening container tertinggal dari systemd | `Dockerfile:46-64`, `deploy/systemd/route-x.service:22-40` |

Status seluruhnya `OPEN`.

### Needs-confirmation M6 (tetap terbuka, bukan temuan confirmed)

- N-01 (`HIGH` bila seharusnya aktif): 5 container existing `exited`; Caddy existing hanya memakai base Compose (kemungkinan belum recreate setelah overlay). Konfirmasi: shutdown disengaja? target produksi atau investigasi?
- N-02 (`MEDIUM`): scraper Prometheus/alert rules/dashboard operator tidak terlihat di repo; mungkin ada di sistem eksternal.
- N-03 (`HIGH` bila port terbuka ke internet): firewall/security-group/DNS/allowlist tidak terbukti dari repo (port 80/443, 8388, 8443 wildcard-bind).
- N-04 (`MEDIUM`): artefak release tanpa bukti gate `make build` (tidak dijalankan karena read-only).

## Dependency Graph dan Urutan Remediasi

```text
SEC-001 identifikasi aman
  -> cabut/rotasi kredensial
  -> verifikasi consumer dan kredensial lama
  -> review blast radius/audit log
  -> buka blocker security untuk M8

OPS-001 kumpulkan bukti runtime teredaksi
  -> tentukan root cause
  -> patch resource/shutdown/config bila diperlukan
  -> uji start-health-stop disposable
  -> tutup readiness Xray

M3 backend -----------\
M4 frontend -----------+-> M7 integrasi/konsolidasi -> targeted regression -> quality gates -> M8 keputusan GO/NO-GO
M5 data/security ------+
M6 infra/readiness ----/

REPO-001 resolved -> verifikasi ulang setelah test/build
OPS-002 stopped  -> tetap berhenti sampai prasyarat aktivasi terpenuhi
```

Urutan prioritas:

1. Tangani SEC-001, SEC-002, dan DATA-001 terlebih dahulu (rotasi kredensial + bersihkan plaintext dari repo/settings); penghapusan file bukan remediasi dan ketiganya memblokir release.
2. Patch jalur jaringan berisiko: BE-001, BE-002, BE-003 (SSRF + kredensial di URL) dan OPS-003 (TLS produksi), lalu BE-004/DATA-004 (keputusan wire-or-remove cache/payload).
3. Pertahankan stack berhenti sambil mengumpulkan bukti OPS-001 tanpa merusak container/data yang diperlukan (hipotesis utama: OPS-006).
4. Selesaikan patch MEDIUM/LOW (BE-005–BE-010, FE-001–FE-107, DATA-002–DATA-003, OPS-004–OPS-015) berdasarkan severity, dependency, serta kemampuan rollback.
5. Jalankan targeted regression dan quality gates setelah patch aktual tersedia.
6. Lakukan validasi akhir dan keputusan release. Keberhasilan build saja tidak mengubah status menjadi `GO`.

## Quality Gates

### Gate per perubahan

- Jalankan unit test paket atau komponen yang berubah, misalnya `go test -short ./internal/<paket>` atau test bernama dengan `-run`.
- Untuk frontend, jalankan `npm --prefix web run build`; jalankan Playwright hanya untuk alur yang relevan dengan server/base URL yang benar.
- Untuk perubahan security, data, routing, atau integration contract, tambahkan test negatif dan regression test yang membuktikan bug/risk tidak berulang.
- Periksa diff dan `git status --short` setelah formatter/build karena command dapat menulis artefak worktree.

### Gate repository sebelum keputusan release

Jalankan berurutan dan catat hasil aktual:

```bash
make fmt
make vet
make test-unit
make build
```

`make build` adalah gate build yang sah karena membangun frontend lebih dahulu lalu menanamkan `web/dist` ke biner.

### Gate bersyarat database

`make test-integration`, `make race`, `make test`, dan `make cover` hanya dijalankan bila `DATABASE_URL` tersedia dan target database telah dikonfirmasi disposable/aman. Command tersebut memuat `./.env`; jangan menampilkan isi file atau environment dalam bukti audit. Test database dijalankan serial sesuai konfigurasi repository. Bila prasyarat tidak tersedia, tandai gate sebagai `BLOCKED` atau `SKIPPED` dengan alasan eksplisit, bukan `PASS`.

E2E backend nyata dan load test hanya boleh dijalankan setelah target, akun uji, dampak perubahan data/rate limit, dan cleanup disetujui aman.

## Log Validasi Dokumen

| Tanggal | Scope | Command/inspeksi | Hasil |
|---|---|---|---|
| 2026-09-08 | Constraints | Inspeksi `AGENTS.md` tanpa membaca `.env` | Invarian, gate, dan batas deployment dicatat. |
| 2026-09-08 | Bukti cleanup | Inspeksi `.gitignore` dan diff terarah | Ignore `/scratch/`, Playwright report/result, serta binary root terkonfirmasi. |
| 2026-09-08 | Dokumen | Inspeksi penuh `planfixv2.md`, diff statistik terhadap `/dev/null`, dan `git status --short -- planfixv2.md` | Lulus: dokumen terbaca lengkap, tercatat 279 baris baru sebelum pembaruan log ini, dan status hanya menunjukkan `?? planfixv2.md`. |
| 2026-09-08 | Progres audit domain | Pembaruan status berdasarkan output audit read-only yang telah selesai | M3, M4, dan M6 menjadi `DONE`; M5 menjadi `ACTIVE` melalui retry terkontrol setelah kegagalan tanpa output; M7 tetap `PENDING`, M8 tetap `BLOCKED`, dan release tetap **NO-GO**. Go toolchain tidak tersedia, pemeriksaan proyek TypeScript lulus, dan runtime tetap berhenti. |
| 2026-09-08 | Diagnosis sesi lama | `ps` + observasi CPU time, `lsof`, `pstree`, inspeksi log CLI teredaksi | Akar masalah: proses `agy` yatim (terminal terhapus, wait `futex`, heartbeat ±6 mnt, tanpa test/audit aktif). Dihentikan graceful (`SIGTERM`) beserta MCP turunannya; worktree dan container tidak disentuh. |
| 2026-09-08 | Rekonstruksi M3 | Baca source aktual + `gofmt -l`, `go vet`, `go test -short -count=1 ./...` | `gofmt`/`vet` bersih; seluruh paket lulus kecuali `TestConstantTimeComparison` flaky (bukan bug). Temuan BE-001–BE-010 dicatat. Test DB dilewati tanpa `DATABASE_URL` (skip eksplisit). |
| 2026-09-08 | Rekonstruksi M4 | Baca `web/src/**`, `web/tests/**`, konfigurasi; `git diff --check` | 0 `CRITICAL` confirmed, 6 `HIGH` + 7 improvement (FE-001–FE-107). Build/E2E tidak dijalankan (batas read-only). |
| 2026-09-08 | Penyelesaian M5 | Baca source, migrasi, seed, RBAC; upaya inspeksi DB read-only | Temuan DATA-001–DATA-004. MCP postgres auth gagal → inspeksi live `BLOCKED`; `.env` tidak dibaca. |
| 2026-09-08 | Rekonstruksi M6 | Inspeksi Compose/Dockerfile/deploy non-mutating + `git ls-files`/riwayat | SEC-002, OPS-003–OPS-015 + N-01–N-04. Runtime tetap berhenti; `.env` tidak dibaca. |
| 2026-09-08 | Konsolidasi M7 | Deduplikasi ID, matriks di register | M7 `DONE`; regression menunggu patch; M8 tetap `BLOCKED`; release **NO-GO**. |
| 2026-09-08 | Remediasi P0 batch 1 | Edit source untuk SEC-002/DATA-001/BE-001–BE-004 + test baru `TestConfigureEncryptsAndRedactsAPIKey` + `TestComputeKeyFieldCoverage` | Patch awal selesai; hasil gate aktual dicatat pada entri validasi berurutan berikutnya. |
| 2026-09-08 | Validasi dan remediasi berurutan | Go 1.27 container: `gofmt` file berubah, `go vet ./...`, `go test -count=1 -short ./...`; `npm --prefix web run build`; Docker build penuh; validasi Compose development/production; `sh -n deploy/xray/entrypoint.sh`; `git diff --check` | Hijau. Test terarah mencakup cache key, enkripsi/migrasi CLI, lifecycle trigger worker, SSRF webhook, serta audit/redaksi terkait. Build Docker menghasilkan biner dengan frontend tersemat. Test integrasi DB dan deploy smoke test tidak dijalankan karena tidak ada target disposable yang dikonfirmasi. |
| 2026-09-08 | Investigasi OPS-001 | Metadata container existing via `docker inspect` tanpa membuka environment/log rahasia | Exit `137`, `OOMKilled=false`; container lama tanpa limit memori. Bukti menguatkan shutdown paksa, tetapi root cause final tetap belum terbukti. Entrypoint sekarang meneruskan TERM/INT dan gateway memakai `stop_grace_period: 35s`; uji start-health-stop disposable masih wajib. |

| 2026-09-08 | Hardening M10 | Audit ulang backend/frontend/infra terhadap diff aktual; patch `internal/xray`, `internal/admin`, `internal/security/rotation`, `internal/cliconfig`, modal frontend, installer production, dan skrip development | Menutup gap source baru: state Xray dienkripsi + izin 0700/0600 + akses share-link dinaikkan, ciphertext settings ikut rotasi atomik, settings rahasia tidak bisa ditulis generik, shell snippet di-quote dan URL divalidasi, assignment API key legacy dihapus, response cache tidak dapat diaktifkan sampai fingerprint policy tersedia, error discovery tidak memantulkan URL/body, modal mendapat relasi ARIA, password bootstrap tidak dicetak, dan jalur bare-metal mewajibkan CA PostgreSQL. |
| 2026-09-08 | Gate M10 | `go test -short` paket terarah, `make fmt`, `make vet`, `make test-unit`, `make build`, `npm --prefix web run build`, validasi Compose development/production dengan path CA placeholder non-rahasia, `bash -n`/`sh -n`, `git diff --check` | Hijau. Test integrasi DB, rotasi nyata, PostgreSQL TLS smoke, dan start-health-stop Xray tetap `SKIPPED/BLOCKED` tanpa target disposable/operator. |

Quality gate source, unit, frontend, Docker build, Compose, dan syntax shell telah dijalankan setelah remediasi. Gate integrasi/race yang memuat database tetap `SKIPPED` karena tidak ada target disposable yang dikonfirmasi aman.

## Aturan Pembaruan Progres

- Saat milestone dimulai, ubah status menjadi `ACTIVE`; hanya satu sumber status berlaku, yaitu tabel milestone di dokumen ini.
- Saat milestone selesai, ubah menjadi `DONE`, tambahkan tanggal, bukti aman, temuan baru, acceptance criteria, dan hasil validasi.
- Jika terhambat, gunakan `BLOCKED` dan tulis dependency yang konkret. Jangan menyamarkan blocked/skipped sebagai pass.
- Setiap temuan baru mendapat ID stabil berdasarkan domain, misalnya `BE-001`, `FE-001`, `DATA-001`, `SEC-002`, atau `OPS-003`. Jangan menomori ulang temuan lama.
- Jangan menutup temuan hanya karena patch sudah dibuat. Status menjadi `RESOLVED` setelah acceptance criteria dan validasi terpenuhi; validasi akhir tetap dilakukan pada M8.
- Setiap perubahan severity harus menyertakan alasan dan bukti. Ketidakpastian pada blast radius diperlakukan konservatif.
- Jangan menempelkan secret, raw credential, `.env`, cookie, token, atau URL sensitif sebagai bukti. Gunakan timestamp, reference ID, status rotasi, hash aman bila disetujui, atau deskripsi teredaksi.
- Setelah setiap fase, perbarui dependency graph, status release, risiko residual, dan quality gate yang relevan.

## Risiko dan Follow-up Aktif

| Prioritas | Item | Status/aksi berikutnya |
|---|---|---|
| P0 | SEC-001: kredensial plaintext pernah berada pada dua skrip ad hoc | Rotasi/cabut dan validasi segera; release tetap `NO-GO`. |
| P0 | SEC-002: material autentikasi Xray terlacak di Git | Rotasi/cabut segera; anggap terekspos; release tetap `NO-GO`. |
| P0 | DATA-001: API key CLI plaintext di settings/file/API | Hentikan penyimpanan plaintext + rotasi key; peran read-only tidak boleh membaca key. |
| P0 | BE-001–BE-004: bypass SSRF, kredensial di URL, cache key | Patch sebelum release; test negatif wajib. |
| P0 | OPS-003: produksi mensyaratkan TLS PostgreSQL yang tidak disediakan | Sediakan TLS + smoke test deploy disposable. |
| P1 | OPS-001: Xray `Exited (137)` | Investigasi root cause (hipotesis utama OPS-006) dengan bukti runtime teredaksi sebelum uji production berikutnya. |
| P1 | FE-001–FE-006, DATA-002–DATA-004, OPS-004–OPS-015, BE-005–BE-010 | Remediasi berurutan + targeted regression; M8 tetap diblokir sampai blocker P0 selesai. |
| P2 | Container/data uji tetap dipertahankan | Lindungi akses; putuskan retention/cleanup setelah kebutuhan investigasi selesai. |

## Keputusan Release Sementara

**NO-GO per 2026-09-08.** Patch kode/config untuk SEC-002, DATA-001, BE-001–BE-004, OPS-003, dan hardening M10 telah diterapkan; quality gate source/unit/frontend/build hijau. Release tetap diblokir oleh SEC-001, rotasi/pencabutan kredensial lama SEC-002 dan DATA-001, pembersihan riwayat/salinan rahasia, smoke test PostgreSQL TLS disposable untuk OPS-003, serta validasi start-health-stop Xray untuk OPS-001. Keputusan hanya dapat berubah melalui M8 dengan bukti operator non-rahasia, gate integrasi pada target aman, dan risiko residual yang diterima eksplisit.
