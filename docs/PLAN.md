# Route-X — Enterprise AI Gateway (Go, single binary)

> Berkas ini adalah rencana yang berlaku dan ikut ter-commit di repo. Salinan lama di
> `~/.claude/plans/eager-weaving-unicorn.md` hanya berisi penunjuk ke sini.

## Context

Membangun AI Gateway kelas produksi dari nol: satu proses Go yang menjadi perantara antara
aplikasi klien dan banyak provider AI (OpenAI, Anthropic, Gemini, OpenAI-compatible, custom,
BYOK), lengkap dengan smart routing, failover, rate limiting, cost tracking, observability,
audit log, dan dashboard admin — semuanya di satu binary tanpa Docker dan tanpa server
frontend terpisah.

Kondisi awal yang sudah diverifikasi di mesin ini:

- `/root` kosong dari proyek apa pun. Tidak ada `go.mod`, `package.json`, atau `CLAUDE.md`.
- `NexGen-X/Route-X` di GitHub **bukan** proyek lama: dibuat 2026-09-02, hanya 2 commit —
  import baseline `OmniRoute v3.8.51` lalu commit rebranding. Isinya TypeScript/Next.js
  (15.551 file, 8.905 `.ts`, 5.500 file test), memakai SQLite, bukan Postgres/Redis.
  Sumbernya `diegosouzapw/OmniRoute` — publik, MIT, 60.2k bintang — jadi seluruh isinya tetap
  bisa diambil ulang kapan pun.
- Go **belum** terpasang. PostgreSQL dan Redis **belum** terpasang. Node v24.20.0 dan npm
  12.0.2 sudah ada (hanya dipakai saat build frontend, tidak di runtime produksi).
- Ubuntu 24.04.4, 4 vCPU, 15 GiB RAM, 305 GB disk bebas, akses root, `gh` login sebagai
  `NexGen-X`.

Hasil akhir yang dituju: `./ai-gateway` dijalankan di Ubuntu, lalu `http://server:PORT`
menyajikan API gateway, dashboard admin, dan aset statis dari proses yang sama.

## Keputusan yang sudah dikonfirmasi

| Topik | Keputusan |
| --- | --- |
| Repo lama | Rename `NexGen-X/Route-X` → `NexGen-X/arsip-gate` (tidak ada yang dihapus), lalu buat repo baru kosong `NexGen-X/Route-X` |
| Direktori kerja | Clone repo baru ke `/root/Route-X` |
| Prasyarat | Pasang Go stable terbaru + PostgreSQL 16 + Redis di mesin ini (systemd, tanpa Docker) |
| Acuan visual | Screenshot sudah diterima — dipakai sebagai bahasa visual di Fase 12 |
| Urutan kerja | Bertahap sampai tuntas; setiap fase ditutup dengan build dan test hijau |

## Status pengerjaan

| Fase | Status | Catatan |
| --- | --- | --- |
| 0 — Repo & lingkungan | ✅ Selesai | Go 1.27.1, PostgreSQL 16.15, Redis 7.0.15 terpasang & aktif; repo lama aman di `arsip-gate`; scaffold + Makefile + `.env` jalan |
| 1 — Fondasi | ✅ Selesai | 8 paket, biner 22 MB jalan; `go test -race ./...` hijau; coverage 89–100% per paket |
| 2 — Data layer | ✅ Selesai | 8 migrasi, 5 paket repository + seeder; 14 paket hijau termasuk `-race`; coverage 87–100% |
| 3 — Auth, RBAC, audit | ✅ Selesai | `internal/auth` 92,4%; login/logout/CSRF/RBAC terpasang di router; diuji end-to-end pada biner |
| 4 — API key | ✅ Selesai | `internal/apikey` 99,7%; `repo/keys` 91,3%; 401 identik byte-per-byte di 9 jalur |
| 5 — Provider & model registry | ✅ Selesai | Kontrak 93,5%; openai 98,4%; anthropic 96,7%; google 96,9% |
| 6 — Routing & resilience | ✅ Selesai | `internal/router` (6 strategi + mesin aturan), `internal/gateway` (pemutus arus Redis, retry, failover); 3 paket hijau `-race` |
| 7 — Endpoint gateway | ✅ Selesai | `/v1/chat/completions`, `/v1/responses`, `/v1/embeddings`, `/v1/models`; SSE, codec dua arah, pabrik adapter bercache; terpasang di biner |
| 8 — Rate limit, budget, ban, filter | ✅ Selesai | `repo/policy` (4 tabel), `internal/ratelimit` (6 cakupan), `internal/billing` (anggaran), `internal/contentfilter` (6 jenis aturan); semuanya terpasang di jalur `/v1` |
| 9 — Usage, cost, observability | ✅ Selesai | `internal/usage` 90,0% (pencatat asinkron, harga, cuplikan latensi), `repo/traffic` 88,6% (rollup jam & hari + agregasi baca), `internal/gateway` 89,2%; `lowest_cost` dan `lowest_latency` akhirnya benar-benar berbeda dari `priority`; `budgets.spent_usd` bergerak |
| 10 — Worker | ⏳ Berikutnya | Penjadwal rollup, health checker, retensi, dan pemeliharaan partisi. Repository-nya sudah ada; yang belum ada penjadwalnya |
| 11 — Admin REST API | ⬜ | |
| 12 — Dashboard | ⬜ | Acuan visual sudah ada |
| 13 — Dokumentasi API | ⬜ | |
| 14 — Pengerasan & verifikasi | ⬜ | |


## Tech stack

- **Go** stable terbaru, `net/http` + **chi** router (kompatibel stdlib, kontrol penuh atas
  SSE/streaming lewat `http.ResponseController`).
- **PostgreSQL** via **pgx/v5** + `pgxpool`. Query parameterized semua, tanpa ORM; pola
  repository per domain. Migrasi SQL bernomor di-`go:embed`, dijalankan otomatis saat start
  dengan `pg_advisory_lock` supaya aman multi-instance.
- **Redis** via **go-redis/v9** untuk rate limit terdistribusi, state circuit breaker, dan
  cache. Semua operasi atomik pakai skrip Lua.
- **Frontend**: React 19 + TypeScript + Vite + Tailwind CSS + TanStack Query + Recharts,
  di-build ke `web/dist` lalu ditanam ke binary dengan `go:embed`. Node hanya dipakai saat
  build; produksi tidak butuh Node sama sekali.
- **Logging**: `log/slog` JSON dengan request-scoped logger dan redaksi rahasia.
  **Metrics**: `prometheus/client_golang` di `/metrics`.

## Struktur direktori

```
Route-X/
├── cmd/ai-gateway/main.go        # satu-satunya entrypoint
├── internal/
│   ├── config/                   # env loading + validasi ketat saat boot
│   ├── database/                 # pgxpool, migration runner, repository per domain
│   ├── cache/                    # klien Redis + skrip Lua
│   ├── security/                 # AES-256-GCM, argon2id, hashing key, redaksi, SSRF guard
│   ├── auth/                     # sesi, login, CSRF, RBAC
│   ├── apikey/                   # generate, hash, verifikasi, scope, limit, rotate
│   ├── models/                   # model registry, alias, pricing
│   ├── providers/                # abstraksi + adapter per tipe provider
│   │   ├── openai/ anthropic/ google/ compatible/ custom/
│   ├── router/                   # 6 strategi routing + routing rules engine
│   ├── gateway/                  # pipeline inti: retry, backoff, circuit breaker, failover, SSE
│   ├── ratelimit/                # RPS/RPM/TPM/harian/bulanan di Redis
│   ├── usage/                    # perekaman + agregasi pemakaian
│   ├── billing/                  # perhitungan biaya, budget, enforcement
│   ├── contentfilter/            # pola blokir/izin, batas ukuran, restriksi model/provider
│   ├── observability/            # metrics, percentile, request ID, structured log
│   ├── health/                   # health checker provider
│   ├── webhooks/                 # pengiriman dengan HMAC + retry
│   ├── audit/                    # audit log administratif
│   ├── worker/                   # supervisor background job
│   ├── admin/                    # REST API untuk dashboard
│   └── httpx/                    # server, middleware chain, error response, embed FS
├── web/                          # sumber React + TS (di-build ke web/dist, di-embed)
├── migrations/                   # .sql bernomor
├── docs/openapi.yaml             # disajikan di /docs
├── .env.example
└── Makefile
```

## Skema database

Tabel inti: `users`, `roles`, `user_roles`, `sessions`, `api_keys`, `providers`,
`provider_credentials`, `models`, `model_aliases`, `model_pricing`, `routing_rules`,
`requests`, `request_events`, `usage_hourly`, `usage_daily`, `budgets`, `rate_limits`,
`bans`, `content_filters`, `audit_logs`, `webhooks`, `webhook_deliveries`, `health_checks`,
`egress_pool`, `integrations`, `settings`, `schema_migrations`.

Keputusan penting untuk performa log request:

- `requests` dipartisi bulanan (declarative partitioning) dengan worker yang membuat partisi
  bulan depan otomatis. Ini yang membuat query log tetap cepat saat data membesar.
- Indeks komposit sesuai pola filter dashboard: `(created_at DESC)`,
  `(api_key_id, created_at DESC)`, `(provider_id, created_at DESC)`,
  `(model, created_at DESC)`, `(status, created_at DESC)`, plus indeks partial untuk error.
- Payload besar (body request/response) disimpan terpisah di `request_events` dengan retensi
  sendiri, supaya tabel `requests` tetap ramping untuk agregasi.
- Percentile (P50/P90/P95/P99, TTFT) dihitung di Postgres dengan `percentile_disc` atas
  rentang waktu; angka live RPS diambil dari counter Redis. Tidak ada angka karangan.

## Pipeline gateway (inti produk)

Satu permintaan klien melewati urutan ini, setiap tahap punya unit test sendiri:

```
request ID → panic recovery → security headers → batas ukuran body
  → autentikasi API key (hash lookup) → cek IP allowlist → cek scope
  → content filter → rate limit (Redis) → cek budget & ban
  → resolusi model (alias → model kanonik)
  → daftar kandidat provider (strategi routing)
  → untuk setiap kandidat:
        cek circuit breaker → cek health → translate request ke dialek provider
        → eksekusi dengan timeout → sukses: relay/stream + rekam usage
                                  → gagal: klasifikasi error → backoff → kandidat berikutnya
  → rekam requests + request_events + usage → audit bila aksi admin
```

Detail yang menentukan kualitasnya:

- **Representasi kanonik internal** bergaya OpenAI. Adapter Anthropic Messages dan Gemini
  `generateContent` menerjemahkan dua arah. Menambah provider baru = satu paket adapter yang
  memenuhi interface `providers.Provider`, tanpa menyentuh gateway.
- **Streaming**: byte SSE diteruskan ke klien sambil di-tee ke parser inkremental untuk
  mengambil TTFT (waktu ke delta konten pertama) dan token usage dari chunk terakhir. Flush
  per event, jadi klien menerima token secepat provider mengirimnya.
- **Circuit breaker** per (provider, model): tiga state closed/open/half-open, rasio kegagalan
  pada sliding window, state di Redis agar konsisten lintas instance dengan jalur cepat di
  memori.
- **Retry** hanya untuk error yang memang aman diulang (timeout, 429, 5xx, koneksi), dengan
  exponential backoff + jitter dan batas dari konfigurasi provider. Error 4xx dari klien tidak
  diulang.
- **6 strategi routing**: priority, round robin, weighted, lowest latency (dari latensi p95
  terukur), lowest cost (dari model pricing), dan capability-based (vision/tools/reasoning/
  embeddings). Dipilih per routing rule, dengan fallback berurutan A → B → C.

## Keamanan

- Password admin: **argon2id**. Sesi: token acak 32 byte, disimpan sebagai hash SHA-256,
  cookie `__Host-` HttpOnly + Secure + SameSite=Lax, dengan expiry dan rotasi.
- API key: format `sk_live_<random>`, disimpan sebagai HMAC-SHA256 dengan pepper server —
  raw key hanya ditampilkan sekali saat dibuat, setelah itu selalu tersamar
  (`sk_live_****9a21`).
- Kredensial provider/BYOK: **AES-256-GCM** envelope encryption, nonce acak per record, AAD
  diikat ke ID kredensial. Tidak ada satu pun endpoint yang mengembalikan plaintext-nya.
- **SSRF guard** untuk base URL custom/BYOK: hanya skema yang diizinkan, resolusi DNS
  diperiksa, dan `net.Dialer.Control` memeriksa ulang IP yang benar-benar didial untuk menolak
  loopback/link-local/private/metadata (169.254.169.254) — ini menutup DNS rebinding.
- CSRF double-submit + pemeriksaan Origin untuk semua request admin yang mengubah state.
- Redaksi rahasia terpusat di `slog.ReplaceAttr` plus tipe secret yang `String()`-nya selalu
  tersamar, sehingga API key, Authorization header, password, dan kredensial provider tidak
  pernah masuk log atau pesan error.
- Security header lengkap: CSP dengan nonce, HSTS, X-Content-Type-Options, Referrer-Policy,
  frame-ancestors, Permissions-Policy. Batas ukuran body dan timeout di semua handler.

## Design tokens (dari screenshot acuan)

Diambil langsung dari gambar yang Anda kirim, dipakai sebagai satu-satunya sumber warna dan
bentuk di Fase 12:

| Token | Nilai | Pemakaian |
| --- | --- | --- |
| `bg-base` | `#0A0A0A` | latar halaman |
| `bg-sidebar` | `#0C0C0C` + border kanan `#1C1C1C` | sidebar |
| `bg-surface` | `#101010` | kartu |
| `bg-surface-2` | `#141414` | kartu bersarang, header tabel |
| `border` | `#1F1F1F` | garis kartu dan baris tabel |
| `text-primary` | `#F5F5F5` | judul, nilai penting |
| `text-secondary` | `#A1A1AA` | label, deskripsi |
| `text-muted` | `#6B7280` | header tabel, section label |
| `accent` | lime `#BEF264` | logo, chip aktif, angka kunci |
| `accent-bg` | `rgba(190,242,100,0.10)` | latar chip aktif |
| `success` `error` `warn` | `#4ADE80` `#F87171` `#FBBF24` | status pill |
| radius | kartu `14px`, dalam `12px`, nav `10px`, chip `999px` | |

Detail bentuk yang ditiru: sidebar ~256px dengan logo bulat lime berisi inisial, nama produk +
subjudul kecil; section label huruf kapital ~11px dengan letter-spacing lebar; item nav ikon
garis 18px + label 14px, state aktif berupa pill gelap dengan teks putih; judul halaman ~32px
diikuti deskripsi dua baris; kartu "Request log" dengan sub-judul dan baris chip filter
(`All`/`Errors`/`Success`, masing-masing dengan hitungan); tabel padat dua baris per sel
(nama tebal di atas, mono redup di bawah), header dengan indikator sort, garis pemisah sangat
tipis. Tipografi: sans geometris untuk UI, monospace untuk ID/model/timestamp.

**Catatan penting soal navigasi:** sidebar di screenshot berasal dari produk lain — isinya
`Bansos Page`, `Farm`, `Bot Logs`, `Inbox`. Yang saya pakai adalah **navigasi dari spesifikasi
tertulis Anda** (Usage, Rate Limits, Budgets, Bans, Routing Rules, Health Checks, Jobs,
Webhooks, Audit Logs, dst.). Jadi gambar itu saya ambil sebagai bahasa visual — warna,
tipografi, kerapatan, bentuk — bukan sebagai daftar menu.

## Catatan hasil Fase 1

Paket yang sudah jadi beserta coverage-nya: `security` 96,0% (Secret, AES-256-GCM + rotasi
kunci, argon2id, API key HMAC), `config` 91,4%, `observability` 90,9% (slog beredaksi + 18
metrik Prometheus), `database` 89,0% (pgxpool + migration runner ber-advisory-lock),
`cache` 98,5% (Redis + namespacing kunci + Lua), `httpx` 95,5% (server, middleware, envelope
error OpenAI, SPA handler), `health` 100%, `cmd/ai-gateway` 31,3% (sisanya diverifikasi lewat
uji end-to-end biner sungguhan).

Empat temuan dari fase ini yang mengubah rencana teknis di fase berikutnya:

- **`READ_TIMEOUT` bisa memutus SSE, bukan hanya `WRITE_TIMEOUT`.** `net/http` menyimpan satu
  pembacaan latar untuk mendeteksi klien terputus, dan pembacaan itu kena tenggat
  `ReadTimeout` lalu membatalkan context request. Dengan default 30 detik, setiap completion
  yang lebih panjang akan mati di tengah jalan. Karena itu ada `httpx.PrepareSSE(w, r)` yang
  melepas kedua tenggat lewat `ResponseController` dan memasang `X-Accel-Buffering: no`.
  **Handler streaming di Fase 7 wajib memanggilnya.**
- **pgx membocorkan DSN lewat pesan error parse-nya sendiri**, termasuk password yang ditulis
  sebagai query parameter — penyamaran bawaan pgx hanya menutup userinfo dan bentuk
  `password=`. Sudah diverifikasi langsung. Semua error dari `internal/database` karena itu
  disaring lebih dulu.
- **Probe kesehatan harus menerima HEAD, bukan hanya GET.** Terlihat saat uji end-to-end:
  banyak load balancer memakai HEAD, dan `r.Get` saja membuat chi menjawab 405 sehingga
  instance yang sehat dianggap mati.
- **Endpoint API yang belum ada harus menjawab 404 berenvelope OpenAI**, bukan pesan soal
  dashboard yang belum di-build. Jawaban untuk endpoint salah tidak boleh berubah tergantung
  apakah frontend sudah tersemat.

Verifikasi end-to-end yang sudah dijalankan pada biner nyata: migrasi berjalan otomatis saat
start, `/healthz` dan `/readyz` menjawab GET dan HEAD, `/metrics` melaporkan angka pool koneksi
yang benar-benar hidup, `/readyz` berubah menjadi 503 saat Redis dimatikan lalu kembali 200
setelah dinyalakan, detail kegagalan hanya muncul di log dan tidak di respons HTTP, dan SIGTERM
menghasilkan shutdown anggun dengan exit code 0.

## Catatan hasil Fase 2

Delapan migrasi diterapkan dan idempoten. Lima paket repository: `repo` (96,6%),
`identity` (90,9%), `upstream` (89,6%), `keys` (89,3%), `traffic` (93,8%), plus `seed`
(87,0%). Seluruh 14 paket hijau termasuk `-race`.

Temuan dan keputusan yang mengubah fase berikutnya:

- **Persentil latensi wajib lewat histogram, bukan rata-rata persentil.** Terukur: dengan
  1.000 sampel cepat dan 10 sampel lambat, p95 sebenarnya 88 ms, merata-ratakan p95
  harian menghasilkan 4.548 ms, histogram gabungan menghasilkan 88,38 ms. Dashboard di
  Fase 9 dan 12 **wajib** memakai `usage_histogram_sum` + `usage_histogram_percentile`
  untuk rentang gabungan.
- **Paginasi keyset harus berurutan total** `(kolom sort, created_at, id)`. Penanda satu
  kolom melewatkan atau menggandakan baris saat banyak request punya nilai identik.
- **Tipe uang: `upstream.USD` bilangan bulat satuan 10⁻⁸ USD**, bukan `float64`. Semua
  kode yang menghitung biaya di Fase 9 harus memakainya.
- **`repo.Error` tidak membawa pesan driver.** `Detail` dan `Hint` dari PostgreSQL bisa
  memuat nilai baris; untuk `provider_credentials` itu berarti kredensial masuk log.
  Lapisan HTTP mengambil keterangan dari `repo.ConstraintName()` dan `repo.Code()`.
- **Seed menyegarkan pemetaan izin peran setiap start**, jadi izin baru cukup ditambahkan
  ke katalog di `internal/database/seed`. Admin pertama hanya dibuat bila tabel `users`
  benar-benar kosong, dan password acak tidak pernah dibuat lalu dicatat.
- **Migrasi tersemat di biner**, jadi iterasi skema pakai `go run ./cmd/ai-gateway
  -migrate`, bukan biner hasil build lama.
- **Test integrasi berbagi satu database**, jadi jalankan `go test -p 1` — paket paralel
  membuat pemeriksaan "tidak ada schema tertinggal" saling menuduh.

Belum dikerjakan di Fase 2 dan sengaja ditunda: **seed registry model beserta harga**.
Mekanismenya sudah ada di `upstream.PricingRepo`, tetapi menanam angka harga yang tidak
bisa saya pastikan akan menghasilkan laporan biaya yang salah tanpa peringatan. Harga
perlu diverifikasi terhadap halaman harga masing-masing penyedia sebelum ditanam.

## Catatan hasil Fase 6

`internal/router` (4 file, 6 strategi + mesin aturan) dan `internal/gateway`
(pemutus arus Redis + executor retry/failover). Keduanya hijau `-race`.

Keputusan yang mengikat fase berikutnya:

- **Strategi routing menghasilkan URUTAN, bukan satu pemenang.** Failover menjanjikan
  "provider berikutnya", dan "berikutnya" hanya bermakna kalau strateginya mengurutkan.
  Memilih satu lalu menyusun sisanya dengan cara lain membuat `lowest_cost` mengirim
  permintaan kedua ke provider mahal padahal ada yang lebih murah, dan membuat `weighted`
  hanya berlaku pada percobaan pertama.
- **Nilai yang tidak diketahui diurutkan PALING BELAKANG.** Harga yang belum diisi bernilai
  nol, dan nol selalu termurah — tanpa aturan ini `lowest_cost` secara sistematis
  mengarahkan seluruh lalu lintas ke pemetaan model yang harganya lupa dimasukkan, dan
  laporan biayanya tetap terlihat wajar. Sama untuk latensi: provider yang belum pernah
  diukur adalah yang paling belum terbukti, bukan yang tercepat.
- **Penyaringan kemampuan berlaku untuk SEMUA strategi**, bukan hanya `capability`:
  streaming yang tidak didukung atau jendela konteks yang terlalu kecil membuat permintaan
  GAGAL, bukan melambat.
- **`Execute` dan `ExecuteStream` dipisah karena umur context-nya berlawanan.** Pada operasi
  biasa context percobaan harus mati begitu op kembali; pada streaming ia harus hidup justru
  setelahnya. Memakai `Execute` untuk streaming menghasilkan aliran yang selalu terputus
  tepat sebelum byte pertama. Tahap MEMBUKA aliran tetap dibatasi, karena klien streaming
  `internal/providers` sengaja tidak memasang `http.Client.Timeout`.
- **Tenggat percobaan wajib dikoreksi menjadi `timeout`, bukan `canceled`.** Keduanya sampai
  ke pemanggil sebagai context yang selesai, dan akibatnya berlawanan: timeout aman
  dialihkan, pembatalan klien tidak. Tanpa koreksi itu failover tidak pernah berjalan untuk
  kegagalan paling umum yang dimiliki gateway.
- **Error mentah dari adapter TIDAK diulang.** `providers.ClassifyTransport` mengembalikan
  `ErrKindNetwork` untuk apa pun yang tidak dikenalinya, dan network aman diulang maupun
  dialihkan — mempercayainya membuat setiap bug adapter diulang tiga kali di setiap
  provider. Ini bug sungguhan yang tertangkap test sendiri saat Fase 6.
- **Backoff memakai equal jitter**, separuh dijamin separuh diundi. Bagian yang dijamin
  karena jitter penuh boleh menghasilkan nol dan mengulang seketika ke provider yang baru
  mengirim 429 hanya menghasilkan 429 kedua; bagian yang diundi karena tanpa jitter seluruh
  permintaan yang terkena satu pembatasan laju mengulang pada milidetik yang sama.
- **`Retry-After` dipatuhi hanya sampai 5 detik.** Lebih dari itu kandidatnya DILEWATI dan
  permintaan dialihkan — provider yang membatasi laju biasa mengirim jeda dalam hitungan
  menit.
- **Cache aturan routing memakai TTL 5 detik, bukan pembatalan-saat-tulis.** Multi-instance:
  perubahan di satu instance tidak bisa membatalkan cache instance lain tanpa pub/sub. TTL
  memberi batas keterlambatan yang bisa dijelaskan dalam satu kalimat. `Engine.Invalidate()`
  ada untuk API admin Fase 11; pub/sub Redis adalah tempat memperbaikinya kalau nanti perlu.
- **Kegagalan membaca aturan tidak menggagalkan permintaan**, dan pemuatan ulang yang gagal
  mempertahankan aturan lama. Satu tabel konfigurasi yang tidak terbaca tidak boleh
  mematikan seluruh lalu lintas inference.

**Cacat gate yang ditemukan dan diperbaiki di fase ini — memengaruhi klaim Fase 2–5.**
`make test` dan `make race` tidak memuat `.env`, sehingga SELURUH test integrasi Postgres
melewati dirinya sendiri dan gate tetap hijau. Terukur: paket repository selesai dalam 1
detik tanpa `.env`, 29–75 detik dengan `.env`. Artinya klaim "hijau termasuk `-race`" untuk
Fase 2–5 lebih lemah dari yang tertulis — unit test-nya memang hijau, tetapi test
integrasinya tidak pernah dijalankan oleh target itu. Makefile sekarang memuat `.env` dan
punya penjaga `require-db` yang berhenti dengan pesan jelas alih-alih hijau hampa. Seluruh
suite diverifikasi ulang setelahnya: hijau dengan `-race`, integrasi benar-benar berjalan.

**Batas yang diketahui dan belum ditutup:** `routing_rules` menyimpan `failure_threshold`,
`open_duration_ms`, dan `half_open_probes` PER ATURAN, sedangkan `gateway.Breaker` memakai
satu `BreakerConfig` untuk seluruh proses. State pemutus arus sendiri memang seharusnya per
(provider, model) dan bukan per aturan — provider yang mati tetap mati siapa pun yang
merutekan ke sana, dan memecahnya per aturan akan melipatgandakan waktu deteksi. Yang belum
tersalurkan adalah AMBANGNYA. Ditambah: kolom `failure_threshold` bertipe hitungan (1..1000),
bukan rasio, sehingga skema itu belum bisa menyatakan konfigurasi rasio yang dipakai
breaker — perlu migrasi (`failure_ratio`, `min_samples`, `window_ms`) atau reinterpretasi
yang ditulis eksplisit. Jangan menulis `FailureThreshold: float64(rule.FailureThreshold)`:
5 akan menjadi rasio 5,0 yang dijepit ke 1,0, yaitu breaker yang praktis mati tanpa satu pun
tanda.

## Catatan hasil Fase 7

`internal/gateway` sekarang memuat permukaan API lengkap: `http.go` (rute, SSE, penolakan
bersebab), `errors_http.go` (pemetaan kegagalan upstream ke status), `codec.go` (pengurai
body ke bentuk kanonik + penyimpul kemampuan dan token), `factory.go` (pabrik adapter
bercache). Empat berkas test menyertainya; seluruh paket hijau `-race`. Permukaan itu
DIPASANG di `cmd/ai-gateway` di belakang `apikey.Authenticate()` lalu `apikey.Limit()`.

Keputusan yang mengikat fase berikutnya:

- **Invarian codec dijaga oleh bentuk kode, bukan disiplin.** Seluruh field tingkat atas
  dibaca ke satu peta, setiap field kanonik DIKELUARKAN saat diurai, dan sisa peta itulah
  `Extra`. Menambah field kanonik baru tanpa mengeluarkannya menjadi mustahil, karena
  satu-satunya cara membacanya adalah lewat `ambil()` yang sekaligus menghapus. Tanpa itu,
  field yang salah tempat dikirim DUA KALI ke upstream atau hilang tanpa jejak — keduanya
  senyap.
- **`providers.EmbeddingsRequest` kini punya `Extra`.** Tanpa itu satu-satunya pilihan
  lapisan HTTP adalah membuang field embedding yang belum dikenal, yaitu menjalankan
  permintaan dengan parameter yang bukan parameter pengirimnya. Adapter openai
  meneruskannya lewat `applyExtra` yang sama seperti chat; Gemini mengabaikannya dan
  alasannya ditulis di `embedSingle`.
- **`max_completion_tokens` SENGAJA tidak dipetakan ke `MaxTokens`.** Model penalaran OpenAI
  menolak `max_tokens`, jadi memetakannya mengubah permintaan yang sah menjadi 400 dari
  upstream. Ia lewat `Extra`, dan `EstimateTokens` membacanya dari sana sebagai batas atas
  keluaran. Hal yang sama untuk `n` dan `stream_options`.
- **Jenis bagian konten yang tidak dikenal DITOLAK, tidak dilewati.** Melewatinya berarti
  prompt yang sampai ke model bukan prompt yang dikirim pengguna, dan jawabannya tetap
  200 — pengguna menerima jawaban atas pertanyaan yang tidak pernah ia ajukan. Harganya:
  jenis bagian baru harus ditambahkan di `uraikanBagian` lebih dulu.
- **Pesan error codec menyebut JALUR field, bukan nilainya** (`messages[2].content[0].type`).
  Pesan itu menjadi badan 400 sekaligus masuk log bersama, dan body permintaan adalah milik
  penyewa yang mengirimnya. Ada test yang menaruh penanda rahasia di posisi konten dan
  menggagalkan build kalau penanda itu muncul di pesan error mana pun.
- **Kunci cache adapter memuat kredensial, base URL, timeout, DAN egress pool** — sebagai
  satu SHA-256, bukan apa adanya, karena kunci ikut tercetak di log. Kredensial diwakili ID
  baris DAN sidik jari nilainya: operator bisa mengganti isi kredensial tanpa mengganti
  barisnya, dan kunci ber-ID saja membuat gateway memakai nilai lama sampai restart.
- **Kredensial diambil SETIAP permintaan** (satu query), dan itu harga yang dibayar sengaja
  supaya rotasi berlaku pada permintaan berikutnya. Yang di-cache adalah adapternya —
  yaitu `http.Transport`, yaitu connection pool dan sesi TLS.
- **`upstream.CredentialRepo.MarkUsed` belum dipanggil siapa pun.** Begitu Fase 9
  memanggilnya, `Active` mulai bergiliran antar kredensial per permintaan. Kunci cache
  sudah menanganinya (satu entri per kredensial, dengan pembuangan LRU) — jangan
  menyederhanakannya menjadi satu entri per provider.
- **Pabrik memvalidasi base URL untuk keluarga openai, karena `openai.New` sengaja tidak.**
  Kontrak `providers.ClientConfig` menyatakan pemanggil yang memvalidasi, dan pemanggil itu
  adalah pabrik. Justru `openai_compatible` dan `custom` yang base URL-nya berasal dari
  operator atau pengguna BYOK. `anthropic.New` dan `google.New` memvalidasi sendiri.
- **Ketika lalu lintas lewat egress proxy, penjaga SSRF di lapisan dial memeriksa PROXY,
  bukan tujuan akhir.** Untuk kandidat berproxy, validasi base URL di pabrik adalah
  satu-satunya pemeriksaan yang melihat tujuan sebenarnya. Karena itu egress pool hanya
  boleh diisi operator, tidak pernah pengguna — API admin Fase 11 wajib menegakkan itu.
- **Dua setelan baru: `UPSTREAM_ALLOW_HTTP` dan `UPSTREAM_ALLOWED_PRIVATE_ADDRS`.** Tanpa
  keduanya gateway TIDAK BISA menghubungi Ollama/vLLM di mesin yang sama, yang merupakan
  pemakaian utama kind `openai_compatible`. Bentuknya ALAMAT, bukan nama host, dengan
  alasan yang sama seperti temuan SSRF Fase 5. Keduanya diurai saat boot dan dicatat di log
  start bila aktif. `Config.UpstreamSSRFPolicy()` adalah satu-satunya tempat kebijakan itu
  disusun.
- **`chi.Mount` TIDAK menulis ulang `r.URL.Path`** — pemotongan prefiks hidup di
  `RouteContext.RoutePath`. Sub-router yang dirakit salah tetap lulus setiap test yang
  memanggil `Routes()` langsung lalu menjawab 404 di produksi, jadi ada test di kedua sisi:
  satu memasang `Routes()` di bawah `/v1`, satu memeriksa `buildRouter` benar-benar
  memasangnya. Untuk pencatatan request Fase 9: `r.URL.Path` memuat path LENGKAP, dan itu
  yang diinginkan.

**Batas yang diketahui dan sengaja dibiarkan terbuka:**

- **Field TAK DIKENAL di dalam satu pesan tidak diteruskan.** `providers.Message` tidak
  punya tempat untuknya, dan menambahkannya berarti setiap adapter harus memutuskan cara
  menerjemahkannya per pesan plus `messagesToWire` harus berhenti memakai struct bertipe.
  Yang hilang dalam praktik adalah field KELUARAN yang ikut terkirim balik saat klien
  mengulang riwayat (`refusal`, `annotations`, `audio`) — OpenAI sendiri mengabaikannya
  pada permintaan masuk. Yang benar-benar belum bisa dipakai lewat permukaan ini adalah
  field masukan per pesan milik dialek lain, mis. `cache_control` milik Anthropic.
- **Audio tidak menuntut kemampuan apa pun**, karena skema kemampuan di migrasi 0003 belum
  punya nilai untuk audio (hanya `text`, `vision`, `reasoning`, `tools`, `embeddings`).
  Memetakannya ke `vision` salah di kedua arah. Perkiraan tokennya pun hanya LANTAI tetap,
  karena biaya audio ditentukan durasi dan durasi tidak ada di wire format. Kedua celah
  tertutup bersama oleh satu migrasi yang menambah nilai kemampuan audio.
- **`Selector` dipasang tanpa `LatencySource` maupun `CostSource`.** Keduanya belum ada —
  `FromHealthSnapshot` yang disebut di doc `internal/router/router.go` BELUM ditulis.
  Akibatnya `lowest_latency` dan `lowest_cost` memperlakukan semua kandidat sebagai "belum
  diketahui", yang menurut aturan paket itu diurutkan paling belakang, sehingga hasil
  akhirnya urutan prioritas. Ini benar dan aman, tetapi berarti dua dari enam strategi
  BELUM benar-benar berbeda dari `priority` sampai Fase 9 memasang sumbernya. Jangan
  laporkan keduanya sebagai berfungsi sebelum itu.
- **`providers.max_retries` TIDAK dipakai jalur request.** Terukur saat verifikasi biner:
  provider dengan `max_retries = 0` tetap dicoba tiga kali sebelum dialihkan, karena jumlah
  percobaan seluruhnya datang dari `routing_rules.max_attempts` (atau bawaan 3) lewat
  `PlanFromRule`. `RouteCandidate.MaxRetries` dibaca dari database dan dibawa sampai ke
  kandidat, lalu tidak dikonsumsi siapa pun. Ini kelas cacat yang sama dengan ambang pemutus
  arus per aturan: kolom yang bisa diisi operator, tampak berlaku, dan tidak berlaku. Yang
  harus diputuskan sebelum menutupnya: mana yang menang ketika aturan dan provider
  menyebutkan angka berbeda — dan jawabannya harus ditulis, bukan disimpulkan dari kode.
- **Ambang pemutus arus per aturan masih belum tersalurkan** — batas yang sama seperti yang
  dicatat di Fase 6, dan belum berubah di fase ini.

**Verifikasi end-to-end pada biner sungguhan** (upstream palsu berdialek OpenAI di
`127.0.0.1:8123`, provider `openai_compatible` tanpa kredensial, API key nyata di database,
`UPSTREAM_ALLOW_HTTP=true` dan `UPSTREAM_ALLOWED_PRIVATE_ADDRS=127.0.0.1,::1`):

1. Keempat rute `/v1` menjawab 401 berenvelope identik tanpa API key — termasuk path `/v1`
   yang tidak ada, jadi autentikasi berlaku sebelum pencocokan rute.
2. `GET /v1/models` mengembalikan registry gateway (7 model), `owned_by` dari keluarga
   model, `created` dalam detik.
3. `POST /v1/chat/completions` non-streaming: 200, `Cache-Control: no-store`, body upstream
   APA ADANYA termasuk `system_fingerprint` dan field yang belum dikenal gateway. Upstream
   melaporkan field tingkat atas yang benar-benar ia terima:
   `[field_baru_klien, frequency_penalty, max_tokens, messages, model, stream]` — Extra
   diteruskan, tidak ada yang dikirim dua kali, dan nama model yang dikirim adalah nama
   UPSTREAM.
4. Streaming: `Content-Type: text/event-stream`, `X-Accel-Buffering: no`, lima chunk
   diteruskan apa adanya, ditutup `data: [DONE]`.
5. `POST /v1/embeddings`: 200, body upstream apa adanya.
6. Penolakan: jenis bagian konten tak dikenal menjadi 400 yang menyebut
   `messages[0].content[0].type` TANPA mengutip nilai dari body; model tak dikenal menjadi
   404 `model_not_found`.
7. Failover: provider prioritas 1 mati (`http://127.0.0.1:9`), prioritas 10 sehat. Klien
   menerima 200 dari yang sehat, dan log mencatat
   `[smoke-mati: network ×3, smoke-lokal: berhasil]`. State pemutus arus benar-benar
   tertulis di Redis untuk kedua provider.

Seluruh baris uji dan kunci Redis-nya dibersihkan setelah verifikasi.

## Catatan hasil Fase 8

Empat paket baru: `internal/database/repo/policy` (tabel `rate_limits`, `budgets`, `bans`,
`content_filters`), `internal/ratelimit`, `internal/billing`, `internal/contentfilter`, plus
`internal/gateway/guard.go` yang memasangnya di titik-titik yang tepat dalam satu permintaan.
Enam cakupan pembatasan berjalan: global, api_key, user, ip, model, provider.

**Keputusan bentuk yang mengikat fase berikutnya:**

- **Mesin jendela Redis TIDAK dipindahkan.** Skrip Lua yang memeriksa dan menaikkan seluruh
  penghitung dalam satu langkah tetap di `internal/apikey`, tempat ia dibangun di Fase 4.
  `internal/ratelimit` adalah lapisan KEBIJAKAN saja: membaca tabel, menyimpan salinan
  ber-TTL, menyusun daftar cakupan. Memindahkan mesinnya berarti menulis ulang satu-satunya
  bagian pembatas laju yang benar-benar sulit, atau punya dua salinannya. Struktur direktori
  di rencana ini menyiratkan sebaliknya; yang berlaku adalah pembagian ini.
- **Pembacaan tabel kebijakan mengambil SELURUH baris aktif.** Empat tabel itu kecil dan
  hanya berubah ketika operator mengubahnya, sementara memeriksa enam cakupan per permintaan
  lewat query terpisah berarti enam perjalanan ke database sebelum satu byte dikirim ke
  provider. Pemanggil mencocokkannya di memori dari salinan ber-TTL 5 detik.
- **Setiap cakupan adalah ember sendiri.** Batas per key dan batas per pengguna ditegakkan
  BERSAMAAN, tidak digabungkan — itulah gunanya: satu pengguna bisa punya sepuluh key
  masing-masing 60 rpm sementara totalnya tetap 100 rpm. Yang digabungkan hanya beberapa
  baris untuk cakupan DAN entitas yang sama, dengan yang lebih dulu menang per field.
- **Kolom batas di `api_keys` menang per field** atas baris `rate_limits` bercakupan
  `api_key` untuk key yang sama. Itu angka yang ditampilkan dashboard pada key tersebut.
- **Kontrak `apikey.ScopeResolver` LENGKAP, bukan tambahan.** Hasilnya MENGGANTIKAN cakupan
  key dan IP bawaan. Kalau digabungkan, cakupan key diperiksa dua kali dan setiap permintaan
  memakan dua jatah kuota — gejalanya hanya "batasnya separuh dari yang saya setel", yaitu
  laporan bug yang sangat sulit dilacak. Karena itu juga `WithKeyLimitDefaults` dan
  `WithIPLimits` DIABAIKAN saat resolver dipasang, dan doc keduanya sudah menyebutkannya.
- **Cakupan MODEL diperiksa di jalur gateway, bukan di middleware**, karena modelnya baru
  diketahui setelah body diurai dan aliasnya diselesaikan. Konsekuensinya dua perjalanan ke
  Redis per permintaan: satu di middleware untuk key/pengguna/IP/global, satu di gateway
  untuk model. Menyatukannya berarti menunda seluruh penegakan sampai body terbaca.
- **Batas cakupan PROVIDER mengembalikan `ErrKindRateLimit`, bukan penolakan permintaan.**
  Mesin eksekusi memperlakukannya sebagai kegagalan yang aman dialihkan, sehingga provider
  yang kuotanya penuh DILEWATI dan kandidat berikutnya dicoba. Menolak seluruh permintaan di
  sana akan membuang kandidat lain yang sehat. Inilah kolom `providers.rate_limit_rpm`:
  dipaksakan gateway sebelum memanggil upstream supaya kuota pihak ketiga tidak terlanggar.
- **Pembatasan provider dari penyaring konten MENYARING KANDIDAT**, tidak menolak permintaan.
  Kalau semuanya tersaring, jawabannya 503 `no_provider_for_model` yang bisa ditindaklanjuti
  operator — bukan 502 dari provider yang sengaja tidak pernah dihubungi.
- **Anggaran habis dijawab 402, bukan 429.** Anggaran tidak pulih dengan menunggu; ia pulih
  saat periodenya berganti. `Retry-After` yang menunjuk ke pergantian bulan hanya membuat
  pustaka klien tidur berhari-hari lalu mengulang. Pesannya menyebut NAMA anggaran tetapi
  tidak angkanya: batas dan pemakaian satu penyewa bukan hal yang pantas dikirim ke penyewa
  mana pun yang memicunya.
- **Pencatatan token terjadi SEBELUM penyaring jawaban.** Kalau dibalik, klien bisa
  menghabiskan token tanpa batas dengan mengirim permintaan yang jawabannya selalu tertahan
  penyaring — tokennya sudah ditagihkan upstream, tetapi kuotanya tidak pernah terpakai.
- **Token aliran dicatat lewat `defer`**, bukan hanya pada jalur selesai-normal: aliran bisa
  berakhir karena klien menutup koneksi, dan token yang sudah dihasilkan tetap ditagihkan
  provider. Tanpa itu, klien yang selalu memutus aliran lebih awal tidak pernah memakan
  kuota tokennya.
- **Batas token selalu bisa dilewati satu permintaan.** Besarnya permintaan baru diketahui
  setelah dijawab, jadi yang digerbangi adalah permintaan BERIKUTNYA. Tidak ada pembatas
  token yang bisa menghindarinya tanpa menebak jumlah token di muka, dan menebak berarti
  menolak permintaan yang sebenarnya kecil.
- **`max_eval_ms` pada penyaring BUKAN penangkal ReDoS.** `regexp` Go memakai RE2 yang
  berjalan linear dan tidak bisa meledak karena backtracking. Yang dijaga anggaran itu adalah
  biaya nyata: puluhan aturan dikali permintaan sepanjang megabyte, di jalur yang sedang
  ditunggu pengguna. Teks di bawah 4 KiB dievaluasi tanpa goroutine penjaga karena
  penjaganya lebih mahal daripada pencocokannya.
- **Arah kesalahan penyaring tidak simetris, dan kodenya mengikuti itu.** Daftar HITAM yang
  tidak bisa dievaluasi (regex tidak sah, anggaran waktu habis) MELOLOSKAN; daftar PUTIH
  MEMBLOKIR. "Tidak diketahui" pada daftar hitam berarti belum terbukti buruk; pada daftar
  putih berarti belum terbukti boleh. Membalik salah satunya menghasilkan kegagalan yang
  mahal di kedua arah.
- **Aturan yang aktif tetapi tidak menegakkan apa pun dilaporkan lewat `Engine.Inert()`** dan
  dicatat ke log saat start. Dua sebabnya: aturan moderasi (belum didukung) dan regex yang
  tidak bisa dikompilasi — bentuk regex tidak diperiksa constraint tabel mana pun. Tanpa
  pelaporan ini, operator melihat aturannya "enabled" di dashboard dan menyangka
  perlindungannya berjalan sementara tidak satu pun permintaan pernah diperiksa olehnya.
- **Blokir (`bans`) ternyata SUDAH lengkap sejak Fase 4.** Pemeriksaannya menempel pada query
  autentikasi API key di `repo/keys`, mencakup keempat jenis subjek (ip, ip_range, api_key,
  user) dalam satu perjalanan yang sama dengan pengambilan barisnya. Fase 8 hanya menambahkan
  pengelolaannya (buat, cabut, daftar, bersihkan yang kedaluwarsa). Jangan memindahkan
  pemeriksaannya ke jalur terpisah: itu satu query tambahan per permintaan untuk tabel yang
  hampir selalu kosong.

**Batas yang diketahui dan sengaja dibiarkan terbuka:**

- **Penyaring konten TIDAK berlaku pada jawaban streaming.** Memeriksa setiap potongan
  sendiri-sendiri melewatkan pola yang melintasi batas potongan — memberi rasa aman yang
  tidak benar — sementara menahan seluruh aliran sampai selesai demi memeriksanya
  membatalkan seluruh gunanya streaming. Menutupnya butuh keputusan tentang mana yang
  dikorbankan, dan keputusan itu belum diambil.
- **Anggaran belum benar-benar menahan biaya sampai Fase 9.** Yang menaikkan
  `budgets.spent_usd` adalah pencatat usage, dan itu Fase 9. Penegakannya sudah terpasang dan
  teruji, tetapi angka yang ditegakkannya masih nol kecuali diisi dari luar.
- **`budgets.spent_usd` berskala 6 sementara `upstream.USD` berskala 8**, jadi setiap
  `AddSpend` dibulatkan PostgreSQL — selisih maksimum 5e-7 USD per permintaan. Angka yang
  berwenang untuk penagihan adalah `requests.cost_usd` (skala 8), dan worker pemelihara
  periode di Fase 10 bisa menghitung ulang `spent_usd` dari sana.
- **Moderasi konten eksternal belum dikerjakan.** Kolomnya ada di `content_filters` dan
  aturannya bisa dibuat, tetapi ia tidak memeriksa apa pun dan muncul di `Inert()`.
  Membutuhkan `internal/webhooks`/integrasi yang baru ada di Fase 10.
- **`providers.max_retries` masih tidak dipakai jalur request** — temuan Fase 7, belum
  berubah di fase ini.

**Verifikasi end-to-end pada biner sungguhan** (upstream palsu di loopback, provider
`openai_compatible` tanpa kredensial, API key nyata, baris kebijakan disisipkan lalu dihapus;
setiap perubahan kebijakan diberi jeda 6 detik karena TTL salinannya 5 detik):

| Yang diuji | Hasil |
| --- | --- |
| Dasar tanpa kebijakan | 200 |
| Pola terlarang `rahasia` | 400 `content_filter`, pesan menyebut nama kebijakan, tanpa isi permintaan maupun polanya; permintaan tanpa kata itu tetap 200 |
| Batas ukuran 64 byte | 413 `payload_too_large` |
| Batas laju cakupan model 1 rpm | permintaan kedua 429 dengan `Retry-After: 38`, `X-RateLimit-Limit: 1`, `Remaining: 0`, `Reset` berupa epoch |
| Anggaran global habis | 402 `budget_exceeded`, tanpa membocorkan angka anggaran |
| Batas token harian 100 (upstream melaporkan 60/permintaan) | permintaan ke-1 dan ke-2 lolos, ke-3 ditolak 429 |
| Blokir API key | 401 berenvelope identik dengan key salah; kembali 200 setelah blokir dicabut |
| Pembatasan provider | 503 `no_provider_for_model` — kandidat tersaring, bukan 502 dari provider yang tidak dihubungi |

Seluruh baris uji, kunci Redis, dan prosesnya dibersihkan setelah verifikasi.

## Catatan hasil Fase 9

Dua paket baru dan satu repository yang dilengkapi: `internal/usage` (pencatat asinkron,
`Pricer` sebagai sumber biaya router, `Index` sebagai sumber latensi router),
`internal/database/repo/traffic` mendapat `rollup.go` (mengisi `usage_hourly` dan
`usage_daily`) dan `stats.go` (Summary, Series, Breakdown, latensi per pemetaan), plus
`internal/gateway/usage.go` yang menyerahkan hasil setiap permintaan `/v1` ke pencatat.
Seluruh paket hijau `-race`.

**Yang berubah bagi pengguna:** halaman Requests akhirnya punya isi, `budgets.spent_usd`
bergerak, dan `lowest_cost` serta `lowest_latency` berhenti berperilaku seperti `priority`.

**Keputusan yang mengikat fase berikutnya:**

- **Token punya DUA bentuk, dan batasnya hanya di satu fungsi.** Kolom token di `requests`
  memakai konvensi OpenAI yang BERSARANG — `input_tokens` sudah memuat
  `cached_input_tokens`, `output_tokens` sudah memuat `reasoning_tokens` — karena itulah
  bentuk yang diteruskan apa adanya ke klien di body respons, dan angka di log request harus
  sama dengan angka yang dilihat klien. Perhitungan biaya menuntut keempat bagian SALING
  LEPAS, dan konversinya ada di `usage.TokensFor`. Terukur pada laporan usage yang bentuknya
  biasa (1000/400 input, 500/200 output): bentuk terpisah menghasilkan 942.000 satuan 10⁻⁸,
  bentuk bersarang yang dipakai apa adanya menghasilkan 1.362.000 — **44% lebih mahal**,
  tanpa satu pun error. Menjumlahkan keempat kolom `requests` akan menghitung token cache dan
  token penalaran dua kali; `total_tokens = input + output`.
- **Sukses dan gagal ditentukan `status_code` DAN `error_type`.** Aliran yang terputus setelah
  sebagian terkirim sampai ke klien sebagai 200 — status itu benar, karena klien memang
  menerima jawaban sebagian — jadi menghitung kegagalan dari status saja melewatkan seluruh
  kelas kegagalan streaming. Definisi yang berlaku: sukses = `status_code < 400 and error_type
  is null`, gagal = kebalikannya; keduanya komplemen persis sehingga
  `success_count + error_count = request_count`. `timeout_count` dihitung dari `error_type`
  tanpa syarat status, dengan alasan yang sama. Konsekuensi yang sudah diambil:
  `traffic.Filter.OnlyErrors` ikut diubah menjadi `(status_code >= 400 or error_type is not
  null)`, sehingga indeks partial `requests_errors_idx` tidak lagi melayani seluruh syaratnya.
- **`error_type` adalah kosakata TERTUTUP untuk agregasi, `error_code` adalah kode yang
  dilihat klien.** Kegagalan upstream memakai `providers.ErrorKind` apa adanya (`timeout`,
  `network`, `rate_limit`, …); penolakan gateway dipetakan dari kode envelope lewat
  `tipeKesalahan` di `internal/gateway/usage.go`; sisanya `"gateway"`. Menambah kode envelope
  baru tanpa menambahkannya ke peta itu membuat baris tercatat sebagai `"gateway"` — bukan
  kesalahan, tetapi kehilangan pemecahan. `error_code` dan `error_message` TIDAK disusun ulang
  di pencatat, melainkan dibaca kembali dari envelope yang benar-benar tertulis: dua tempat
  yang memutuskan hal yang sama akan berbeda, dan yang dicari saat pengguna mengeluh adalah
  kode yang ia lihat.
- **Status HTTP dibaca dari respons, bukan dilaporkan tiap jalan keluar.** `pencatatRespons`
  membungkus `ResponseWriter` dan mencatat status pertama beserta envelope error-nya; jejak
  diserahkan lewat satu `defer` per handler. Itu yang membuat titik keluar BARU ikut tercatat
  tanpa harus diingat — dan handler `/v1` punya belasan titik keluar. Wrapper itu WAJIB punya
  `Unwrap()`: tanpanya `httpx.PrepareSSE` tidak bisa melepas tenggat baca/tulis dan setiap
  respons streaming mati pada detik `READ_TIMEOUT`.
- **Pencatatan asinkron, dan alasan pertamanya bukan performa.** Context permintaan sudah MATI
  ketika pencatatan berjalan, jadi `INSERT` dengan context itu gagal setiap kali, bukan
  kadang-kadang. Penulis memakai context baru bertenggat sendiri. Antrean berbatas (4.096) dan
  penuh berarti catatan DIBUANG, bukan permintaan ditahan; `routex_usage_records_total`
  memecah hasilnya (`written`, `failed`, `dropped`, `spend_failed`) supaya pembuangan itu bisa
  dipertanggungjawabkan. Metrik diambil SEBELUM antrean — kalau tidak, hilangnya satu baris
  database ikut menghilangkan angka yang melaporkan hilangnya baris itu. Satu-satunya yang
  dihitung di penulis adalah biaya, karena ia butuh cuplikan harga yang pemuatannya pembacaan
  database. **`Recorder.Close` wajib dipanggil SETELAH server berhenti menerima permintaan**;
  `main` sudah menyusunnya begitu, dengan tenggat pengurasan 10 detik dari context tanpa
  pembatalan.
- **TTFT diukur setelah flush, dan hanya pada potongan KONTEN.** Aliran berdialek OpenAI
  dibuka dengan potongan yang hanya membawa peran (`"delta":{"role":"assistant"}`), dan
  potongan itu datang sebelum token pertama dihasilkan. Terukur pada upstream uji yang menunda
  50 ms lalu 120 ms lagi: menandai pada potongan pertama apa pun melaporkan ~50 ms, menandai
  pada potongan konten melaporkan 174 ms — dan yang kedua itulah yang dialami pengguna.
- **Persentil: satu baris pakai kolom, lebih dari satu baris WAJIB pakai histogram.** Kolom
  `latency_p50..p99` pada `usage_hourly` persis untuk jamnya sendiri (dihitung
  `percentile_disc` atas `requests`); kolom yang sama pada `usage_daily` dan SEMUA pembacaan
  rentang gabungan lewat `usage_histogram_sum` + `usage_histogram_percentile`. Terukur ulang
  di test permanen `TestIntegrationPersentilGabunganBukanRataRataPersentil`: p95 persis
  **80 ms**, histogram gabungan **88,79 ms**, rata-rata p95 per bucket **30.040 ms**.
- **Sumber baca dipilih wholesale, tidak pernah dicampur.** `traffic.Query.Source` memilih
  tabel (`requests` untuk rentang ≤ 7 hari, `usage_daily` di atas itu, atau eksplisit), dan
  `Stats.Source` melaporkan mana yang dipakai. Mencampurnya berarti separuh grafik memakai
  definisi persentil yang berbeda dari separuh lainnya. Harganya diakui: berpindah sumber bisa
  menggeser p95 sebesar lebar satu slot histogram — terukur 6.000 ms (persis) menjadi
  6.833 ms (histogram) pada data verifikasi.
- **`Pricer` punya dua pintu karena dua jalurnya berbeda.** `Price(ctx, …)` memuat ulang
  cuplikan yang kedaluwarsa dan dipakai pencatat (asinkron, boleh menyentuh database);
  `Cost(…)` hanya membaca cuplikan dan dipakai router pada setiap kandidat (tidak boleh
  menunggu apa pun). TTL 30 detik, lebih pendek daripada cache lain di proyek ini justru
  karena yang dibayar keterlambatan di sini adalah uang. `Warm` dipanggil saat start;
  kegagalannya dicatat dan TIDAK menghentikan start.
- **`Index` memakai `coalesce(ttft_ms, latency_ms)` dan hanya request BERHASIL.** Untuk
  streaming yang ditunggu pengguna adalah token pertama, bukan panjang jawabannya; dan
  provider yang menolak setiap permintaan dalam 5 ms adalah yang tercepat menurut angka
  mentah. Ambang 10 sampel pada jendela 30 menit, disegarkan tiap menit oleh satu goroutine
  yang `main` nyalakan (pola yang sama dengan `collectPoolStats`). Pemetaan di bawah ambang
  TIDAK muncul di hasil, dan `Selector` memperlakukan yang tidak ada sebagai "belum terukur"
  lalu jatuh ke `last_latency_ms` health check — cadangan itu bukan sementara, ia melayani
  pemetaan yang lalu lintasnya sepi.
- **`FromHealthSnapshot` tidak pernah ada dan tidak dibuat.** Doc `internal/router` menyebutnya;
  cadangan yang dimaksud sudah hidup di `Selector.latensi`, dan menulis tipe kedua hanya
  menggandakan aturan yang sama. Doc-nya sudah diperbaiki agar menunjuk ke tempat yang benar.
- **`CredentialRepo.MarkUsed` kini dipanggil, dijeda 5 detik per kredensial, di luar jalur
  permintaan.** Dipasang eksplisit lewat `gateway.WithCredentialUseMarker` karena ia MENGUBAH
  PERILAKU: `Active` mengurutkan dengan `last_used_at asc nulls first`, jadi menulis kolom itu
  membuat beberapa kredensial pada satu provider bergiliran. Jedanya ada karena giliran
  per-permintaan berharga satu `UPDATE` ke baris yang sama untuk SETIAP permintaan inference —
  dan pada provider dengan satu kredensial, keadaan paling umum, tidak ada apa pun yang
  digilir. Giliran per permintaan bisa didapat dengan menurunkan `jedaTandaiPemakaian` ke nol;
  yang harus diketahui sebelum melakukannya adalah baris panas itu. **Kunci cache adapter
  jangan disederhanakan menjadi satu entri per provider** — sekarang giliran itu nyata.
- **Label metrik hanya boleh dari registry.** `Event.RequestedModel` datang dari klien dan
  TIDAK BOLEH menjadi label; yang dipakai `Event.ModelName` (nama kanonik) dan
  `Event.ProviderName`. Ada test permanen yang mengirim nama model karangan lalu menggagalkan
  build kalau nama itu muncul di `/metrics`. Provider yang tidak terpilih menghasilkan label
  kosong (`provider=""`), dan itu apa adanya: permintaan yang ditolak sebelum routing memang
  tidak punya provider.
- **`GET /v1/models` sengaja tidak dicatat** ke `requests`: ia tidak punya model, tidak punya
  provider, dan tidak memakai satu token pun, jadi barisnya hanya derau di halaman Requests.
  Jejaknya ada di access log.
- **Rollup punya repository, belum punya penjadwal.** `RollupHourly`, `RollupDaily`, dan
  `RollupRange` idempoten (menimpa, bukan menambah) dan aman dijalankan ulang; yang belum ada
  adalah worker yang memanggilnya, dan itu Fase 10. Sampai itu, `usage_hourly` dan
  `usage_daily` kosong di pemasangan baru, dan pembacaan rentang panjang mengembalikan nol —
  bukan error. `Stats.Source` yang membuat keadaan itu bisa dibedakan dari "tidak ada lalu
  lintas". Catatan untuk penjadwalnya: menghitung ulang jam yang baris mentahnya sudah dibuang
  retensi TIDAK menolkan baris agregatnya, ia membiarkan nilai lama — jadi urutan yang benar
  adalah rollup dulu, retensi kemudian.

**Batas yang diketahui dan sengaja dibiarkan terbuka:**

- **`request_payloads` masih tidak punya penulis.** `SavePayload` ada sejak Fase 2 dan tidak
  dipanggil siapa pun. Menangkap body berarti menyimpan prompt pengguna di database, dan
  keputusan itu butuh dua hal yang belum ada: setelan yang menyalakannya secara sadar, dan
  bentuk yang masuk akal untuk jawaban SSE (yang bukan satu dokumen JSON). Request inspector
  Fase 12-lah yang menentukan kebutuhannya; sampai itu, membangunnya setengah jalan hanya
  menambah data sensitif tanpa pembaca.
- **Penolakan yang terjadi SEBELUM handler tidak masuk `requests`.** 401 dari
  `apikey.Authenticate()` dan 429 cakupan key/pengguna/IP/global dari `apikey.Limit()` terjadi
  di middleware, yang tidak punya model, token, maupun provider untuk dicatat. Keduanya
  terlihat di access log dan di metrik (`routex_rate_limit_rejected_total`), tetapi halaman
  Requests tidak akan memuatnya. Menutupnya berarti memindahkan pencatatan ke middleware, dan
  di sana separuh kolomnya kosong.
- **Metrik HTTP admin dan worker belum diamati.** `routex_http_*`, `routex_worker_*`
  terdaftar tetapi tidak ada yang mengisinya: permukaan admin baru ada di Fase 11 dan worker
  di Fase 10. `routex_provider_up` dan `routex_provider_health_latency_ms` juga masih kosong
  karena health checker provider adalah Fase 10 — gauge yang absen lebih baik daripada gauge
  yang salah.
- **`routex_gateway_cost_usd_total` bertipe float64** seperti semua counter Prometheus, jadi
  ia bisa menyimpang beberapa satuan 10⁻⁸ setelah jutaan penjumlahan. Angka yang berwenang
  untuk penagihan tetap `requests.cost_usd` (numeric, skala 8).
- **`budgets.spent_usd` berskala 6 sementara `upstream.USD` berskala 8**, jadi setiap
  `AddSpend` dibulatkan PostgreSQL — selisih maksimum 5e-7 USD per permintaan. Temuan Fase 8,
  belum berubah; worker pemelihara periode Fase 10 bisa menghitungnya ulang dari
  `requests.cost_usd`.
- **Harga bisa terlambat satu TTL (30 detik) di instance LAIN.** `Pricer.Invalidate()` ada
  untuk API admin Fase 11 dan hanya berlaku pada instance yang dihubungi; batas yang sama
  seperti cache aturan routing. Biaya yang sudah tercatat tidak dihitung ulang sendiri —
  `model_pricing` menyimpan riwayatnya, jadi perhitungan ulang mungkin lewat `PricingRepo.At`
  dengan join `provider_models` pada `(provider_id, model_id)`, karena `requests` menyimpan
  keduanya dan bukan `provider_model_id`.
- **`providers.max_retries` masih tidak dipakai jalur request** — temuan Fase 7, belum berubah.
- **Penyaring konten masih tidak berlaku pada jawaban streaming** — batas Fase 8, belum berubah.
- **Moderasi konten eksternal masih belum didukung** — batas Fase 8, belum berubah.
- **Ambang pemutus arus per aturan routing masih belum tersalurkan** — batas Fase 6, belum
  berubah.

**Verifikasi end-to-end pada biner sungguhan** (dua upstream palsu berdialek OpenAI di
loopback yang melaporkan usage BERSARANG seperti OpenAI — `prompt_tokens` 1000 dengan
`cached_tokens` 400, `completion_tokens` 500 dengan `reasoning_tokens` 200 — provider
`openai_compatible`, harga 3,00/15,00/0,30 USD per juta token, API key nyata):

| Yang diuji | Hasil |
| --- | --- |
| Chat non-streaming | 200; baris `requests` memuat token 1000/400/500/200/1500 apa adanya dan `cost_usd = 0.00942000` — angka yang sama dengan yang dihitung tangan di test unit |
| Chat streaming | 200, 6 peristiwa SSE ditutup `[DONE]`; `ttft_ms = 174` sementara potongan peran datang pada ~50 ms, jadi penandaan TTFT memang menunggu potongan konten |
| Embeddings | 200; 9 token, `cost_usd = 0.00002700` |
| Model tak dikenal | 404; baris tercatat dengan `requested_model = "tidak-ada"`, `error_type = model_not_found`, tanpa model kanonik maupun provider |
| `request_events` | satu baris `attempt_succeeded` per permintaan berhasil, `detail` memuat nomor percobaan dan state pemutus arus |
| `routing_decision` | strategi, aturan, dan kandidat berurut dengan penanda `chosen` |
| Anggaran | `budgets.spent_usd` bergerak 0 → 0,018840 setelah dua permintaan; inilah celah Fase 8 yang tertutup |
| `lowest_cost` vs `priority` | dua provider melayani model yang sama, yang mahal berprioritas LEBIH BAIK. `priority` memilih yang mahal (0,25 USD), `lowest_cost` memilih yang murah (0,00942 USD) — 26× lebih murah, hanya karena sumber harga terpasang |
| `lowest_latency` vs `priority` | dengan p95 terukur 96 ms vs 4 s, `lowest_latency` memilih yang cepat sementara `priority` tetap memilih yang berprioritas lebih baik |
| Failover | provider prioritas 1 dialihkan ke `http://127.0.0.1:9`; klien menerima 200 dari provider berikutnya, baris tercatat `failover_count = 1`, timeline `attempt_failed (network) → attempt_succeeded` |
| Giliran kredensial | dua kredensial pada satu provider; keduanya terpakai, dan `last_used_at` ditulis paling sering sekali per 5 detik per kredensial |
| Rollup & agregasi | `RollupRange` menghasilkan 5 baris jam dan 5 baris hari; `Summary` dari `requests` p95 6.000 ms, dari `usage_hourly` 6.833 ms; `Breakdown` per provider memisahkan biaya 0,113 vs 1,50 USD dan memuat potongan tanpa provider; granularitas jam atas `usage_daily` ditolak dengan pesan yang menyebut sebabnya |
| `/metrics` | `routex_usage_records_total{outcome="written"} 12`, `usage_queue_depth 0`, biaya per provider, `upstream_failures_total{kind="network"}`, `failovers_total`, dan `provider_availability_ratio` berlabel NAMA provider |

Seluruh baris uji, kredensial uji, kunci Redis, dan prosesnya dibersihkan setelah verifikasi.

## Catatan hasil Fase 3

`internal/auth` 92,4% coverage, 57 fungsi test. Diuji end-to-end pada biner sungguhan:
password salah dan email tak terdaftar mengembalikan body identik, login benar
mengembalikan Super Admin dengan 25 izin dan `must_change_password`, `password_hash`
tidak pernah muncul di respons, `/me` menuntut cookie, logout tanpa token CSRF ditolak
403, dan kedua kegagalan login tercatat di `audit_logs`.

Yang mengikat fase berikutnya:

- **CSRF hanya untuk rute berautentikasi cookie.** Rute API yang memakai Bearer API key
  tidak butuh CSRF — browser tidak pernah melampirkan header `Authorization` sendiri.
  Jangan pasang `CSRF().Protect()` di grup `/v1/*`.
- **Urutan middleware rute admin:** `RequireSession()` → `RequirePasswordChanged()` →
  `CSRF().Protect()` → `RequirePermission(...)`. `RequireSession` sengaja tetap
  meloloskan akun yang wajib ganti password, karena form ganti password juga butuh sesi.
- **Kode error milik paket auth, bukan `httpx`.** `httpx.Unauthorized` memasang
  `invalid_api_key`, yang menyesatkan untuk form login dashboard dan tidak bisa
  membedakan "password salah" / "sesi habis" / "wajib ganti password" — tiga hal dengan
  tindak lanjut berbeda di UI Fase 12.
- **Pembatas laju login gagal-terbuka.** Redis mati meloloskan percobaan; penguncian per
  akun di database tetap berlaku. Gagal-tertutup berarti tidak seorang pun bisa masuk
  untuk memperbaiki Redis.
- **`security.WarmUp()` wajib dipanggil saat start** (sudah di `main.go`). Tanpa itu
  panggilan `BurnVerifyTime` pertama menghitung hash penuh dan justru menonjol dari sisi
  waktu.

Cacat di kode Fase 1 yang ditemukan saat Fase 3: `security.BurnVerifyTime()` mengisi hash
pembandingnya secara malas tanpa penyelarasan — data race sungguhan, terkonfirmasi race
detector. Sudah dijaga `sync.Once` dan ada test konkurensi permanennya.

Catatan kebersihan riwayat: empat file `repo/upstream` mendapat sentuhan akhir setelah
commit Fase 2 sudah dibuat, jadi perubahan itu ikut di commit Fase 3. Kodenya
terverifikasi (gate lengkap dijalankan atas isi yang di-commit), hanya penempatan
commit-nya yang tidak rapi.

## Fase implementasi

Setiap fase ditutup dengan `gofmt`, `go vet`, `go build`, dan `go test ./...` hijau sebelum
lanjut. Tidak ada fase yang meninggalkan TODO pada fungsi inti.

**Fase 0 — Repo & lingkungan.** Rename `Route-X` → `arsip-gate` (`gh repo rename`), buat repo
privat baru `Route-X`, clone ke `/root/Route-X`. Pasang Go stable terbaru dari tarball resmi ke
`/usr/local/go`, PostgreSQL 16 dan Redis via apt, buat database + role aplikasi. Inisialisasi
`go.mod`, `Makefile`, `.env.example`, `.gitignore`.

**Fase 1 — Fondasi.** `config` dengan validasi ketat saat boot (gagal cepat kalau
`ENCRYPTION_KEY`/`SESSION_SECRET` lemah atau kosong), slog JSON, pgxpool, migration runner,
klien Redis, server HTTP + middleware chain, graceful shutdown, `/healthz` dan `/readyz`,
kerangka `go:embed`.

**Fase 2 — Data layer.** Seluruh migrasi + partisi `requests`, repository per domain, seed:
role bawaan, admin pertama dari env, dan model registry awal berisi model + harga nyata untuk
OpenAI, Anthropic, dan Gemini.

**Fase 3 — Auth, RBAC, audit.** argon2id, sesi, CSRF, login/logout, empat peran (Super Admin,
Admin, Operator, Viewer) sebagai middleware, penulis audit log yang dipakai semua mutasi admin.

**Fase 4 — API key.** Generate, hash, verifikasi, scope, allowed models/providers, IP
restriction, limit token/budget, expiry, rotate, revoke, disable, tampilan tersamar.

**Fase 5 — Provider & model registry.** Interface `Provider` + adapter OpenAI, Anthropic,
Google, OpenAI-compatible, custom HTTP. Enkripsi kredensial, BYOK, SSRF guard, health check
nyata ke endpoint provider, alias model, pricing.

**Fase 6 — Routing & resilience.** Enam strategi, routing rules engine, circuit breaker,
retry + exponential backoff, timeout berlapis, failover otomatis antar provider.

**Fase 7 — Endpoint gateway.** `POST /v1/chat/completions`, `POST /v1/responses`,
`GET /v1/models`, `POST /v1/embeddings` — streaming dan non-streaming, SSE, tool/function
calling, system message, passthrough multimodal. Target: aplikasi OpenAI-compatible yang ada
cukup mengganti base URL dan API key.

**Fase 8 — Rate limit, budget, ban, content filter.** Sliding window Lua di Redis untuk
RPS/RPM/TPM/harian/bulanan per API key, user, provider, model, dan IP; respons 429 dengan
header `Retry-After` dan `X-RateLimit-*` yang benar.

**Fase 9 — Usage, cost, observability.** Perekaman per request, agregasi, biaya dari model
pricing, percentile dan TTFT, error/timeout/retry rate, availability provider, `/metrics`
Prometheus.

**Fase 10 — Worker.** Supervisor `errgroup` dengan start/stop bersih dan `pg_advisory_lock`
untuk worker singleton: health checker, rollup usage, cleanup retensi, pengiriman webhook
(queue + backoff + HMAC), pemeliharaan partisi.

**Fase 11 — Admin REST API.** Endpoint nyata untuk setiap halaman dashboard, termasuk query
log request dengan search, filter provider/model/status/API key, rentang waktu, sorting, dan
pagination keyset.

**Fase 12 — Dashboard.** Design system dark, sidebar 5 grup (collapsed mode, tooltip, active
state, responsif), Dashboard, Observability, Requests + request inspector, dan seluruh halaman
Upstreams/Gateway/Automation/System. Semua terhubung ke API Fase 11 — tanpa angka karangan dan
tanpa tombol mati. **Dimulai setelah screenshot acuan Anda masuk**; sebelum menulis chart saya
muat skill `dataviz`, dan untuk kualitas visual skill `artifact-design`.

**Fase 13 — Dokumentasi API.** `docs/openapi.yaml` untuk semua endpoint publik, disajikan di
`/docs`.

**Fase 14 — Pengerasan & verifikasi akhir.** `go test -race ./...`, uji beban ringan, unit
systemd, README operasional, build produksi, lalu verifikasi end-to-end menyeluruh.

## Testing

**Unit** (tanpa dependensi eksternal): strategi routing dan urutan failover, circuit breaker,
perhitungan biaya dari pricing, hashing + verifikasi API key, AES-256-GCM, argon2id, redaksi
rahasia, SSRF guard, content filter, translator Anthropic/Gemini, parser SSE (TTFT + usage),
rate limiter (via `miniredis`).

**Integrasi** (Postgres dan Redis nyata di mesin ini): migrasi naik dari kosong, semua
repository, rate limit terdistribusi, dan alur gateway penuh lewat stack HTTP asli.

Adapter provider diuji terhadap `httptest.Server` yang bicara wire format OpenAI, Anthropic,
dan Gemini — termasuk kasus stream terpotong, 429, dan 5xx. Ini test double untuk upstream
pihak ketiga, bukan fungsionalitas produk yang dipalsukan: kode provider yang diuji adalah
klien HTTP nyata yang sama dengan yang dipakai di produksi.

Uji terhadap provider sungguhan hanya dijalankan bila Anda menyediakan API key — beri tahu
kalau ada, akan saya pakai untuk smoke test terakhir.

## Verifikasi end-to-end

Setelah Fase 14, saya jalankan dan laporkan hasil nyatanya:

1. `make build` → menghasilkan `./ai-gateway` tunggal.
2. `./ai-gateway` → migrasi jalan otomatis, worker start, port siap.
3. Login admin, buat provider yang menunjuk ke upstream uji lokal, buat API key.
4. `curl /v1/chat/completions` non-streaming dan streaming — memastikan klien
   OpenAI-compatible bekerja hanya dengan menukar base URL.
5. Buka dashboard: request tadi muncul di log dengan token, biaya, latensi, dan TTFT yang
   cocok dengan respons upstream; buka request inspector-nya.
6. Uji rate limit sampai dapat 429 yang benar, lalu matikan provider utama untuk membuktikan
   failover otomatis ke provider berikutnya.
7. `go test -race ./...` dan `go vet ./...` hijau seluruhnya.

## Catatan terbuka

- **Screenshot acuan sudah diterima** dan token desainnya diekstrak ke bagian "Design tokens"
  di atas. Navigasi tetap mengikuti spesifikasi tertulis Anda, bukan menu di gambar — alasannya
  dijelaskan di bagian itu.
- **Rename repo memutus redirect nama lama.** Setelah `Route-X` dipakai ulang oleh repo baru,
  URL `NexGen-X/Route-X` yang lama tidak lagi mengarah ke `arsip-gate`. Tidak ada data yang
  hilang — 7 branch dependabot, riwayat, dan setting ikut pindah ke `arsip-gate`.
- **Tidak ada yang dihapus.** Kode TypeScript lama tetap utuh di `NexGen-X/arsip-gate` dan
  sumber aslinya tetap tersedia di `diegosouzapw/OmniRoute` (publik, MIT).





## Keadaan saat sesi ini berhenti

Delapan commit di `NexGen-X/Route-X`, semuanya lewat gate lengkap (`gofmt`, `go vet`,
`go test -race`, `make build`) sebelum di-push.

**Selesai dan terverifikasi:** Fase 0, 1, 2, 3, plus fondasi Fase 5 (kontrak
`internal/providers`, klasifikasi error yang menggerakkan retry/failover, penjaga SSRF
dua lapis), katalog model, dan perbaikan perkakas (`make test -p 1`, `schemas-clean`).

**Sedang dikerjakan empat subagent saat sesi berhenti:**

1. Mutasi `internal/database/repo/keys` — rotate, revoke, disable, update, expire, lineage
2. `internal/apikey` — autentikasi `/v1/*` + pembatas laju terdistribusi
3. `internal/providers/openai` — adapter OpenAI/compatible/custom
4. `internal/providers/anthropic` + `internal/providers/google` — adapter penerjemah

Hasil keempatnya belum saya integrasikan, verifikasi ulang, atau commit. **Langkah
pertama saat melanjutkan:** jalankan gate lengkap, periksa hasil keempat paket itu di
repo asli (subagent memverifikasi di keadaan yang bisa berbeda), lalu commit per fase.

**Belum dimulai:** Fase 6 (routing & resilience), 7 (endpoint gateway + SSE), 8 (rate
limit/budget/ban/filter di jalur request), 9 (usage & observability), 10 (worker), 11 (API
admin), 12 (dashboard), 13 (OpenAPI), 14 (pengerasan akhir).

Yang perlu diingat saat melanjutkan, semuanya sudah tercatat di catatan hasil tiap fase di
atas: handler streaming Fase 7 **wajib** memanggil `httpx.PrepareSSE`; persentil rentang
gabungan **wajib** lewat `usage_histogram_sum` + `usage_histogram_percentile`; biaya
memakai `upstream.USD` bukan `float64`; `Error.StreamedBytes > 0` melarang retry maupun
failover; dan jalur `/v1/*` tidak boleh dipasangi CSRF.

## Dua temuan itu: sudah diverifikasi dan diperbaiki

Keduanya **terbukti nyata** sebelum ditambal, dan reproduksinya sekarang menjadi test permanen.

**1. `security.Secret` memang tidak cukup di field TAK DIEKSPOR — dan bocornya lebih luas
daripada yang dilaporkan.** Terukur: `%v` pada `anthropic.Provider` dan `google.Provider`
mencetak API key mentah; `database.DB` mencetak seluruh connection string beserta
password-nya. Empat tipe lain bocor hanya lewat `%s` — jalur verb-salah `fmt` membongkar isi
struct di balik field pointer yang tampak aman di `%v`: `auth.Handlers`, `auth.Service`
(DATABASE_URL, SESSION_SECRET), serta `stream` di anthropic dan google.

Semuanya kini punya `String`, `GoString`, `LogValue`. Receiver **nilai** bila tipenya boleh
disalin, **pointer** bila memuat lock — `go vet` menolak receiver nilai di sana, dan
larangan yang sama itulah yang menutup jalur cetak-nilai, jadi receiver pointer sekaligus
wajib dan cukup.

Penjaganya `TestRedaksiFieldTakDiekspor` di `internal/security`: membaca seluruh sumber repo
(234 struct), menelusuri penjangkauan lintas paket, dan menggagalkan build test untuk tipe
baru yang melewatkannya. Ditambah tiga test perilaku di paket adapter — lint hanya bisa
membuktikan metodenya ada, bukan bahwa isinya bersih.

Satu jalur sengaja dibiarkan terbuka dan didokumentasikan: struct **anonim** yang menyimpan
`*Provider` di field tak diekspor lalu dicetak `%s`. Redaksi harus tinggal di pemilik field,
dan struct anonim di dalam badan fungsi tidak punya tempat itu. Tipe bernama — yakni seluruh
repo — sudah tertutup.

**2. `AllowedPrivateHosts` memang tidak berlaku di lapisan dial.** Terverifikasi
ujung-ke-ujung: `ValidateBaseURL` meloloskan `127.0.0.1`, dial-nya diblokir.

Pengecualiannya kini **per alamat** (`AllowedPrivateAddrs []netip.Prefix`) dan `CheckAddr`
yang membacanya, sehingga kedua lapisan sepakat. Alamat alih-alih nama bukan kompromi
ergonomis: di lapisan dial nama sudah hilang, jadi pengecualian bernama hanya bisa bekerja
dengan meresolusi saat kebijakan dibuat lalu mempercayai hasilnya saat menghubungi — tepat
DNS rebinding yang dijaga lapisan itu. `ParsePrivateAddrs` menolak nama host dengan pesan
yang menyebutkan jalan keluarnya. `localhost` tetap lolos validasi bila loopback sudah
dikecualikan lewat alamat.

Ketiga test adapter kini memakai pengecualian per alamat, bukan `AllowPrivate`, sehingga
sisa penjagaan rentang tetap aktif selama test berjalan.

## Semantik yang perlu disepakati antar adapter

`StreamEvent.Raw` diisi adapter OpenAI dengan **muatan `data:` saja** (objek chunk JSON),
bukan blok SSE lengkap — framing `data: `, baris kosong, dan `[DONE]` dibuat lapisan HTTP
gateway di Fase 7. Adapter anthropic/google harus mengikuti bentuk yang sama, dan doc
`StreamEvent.Raw` di `providers.go` perlu diperjelas agar tidak ambigu.
