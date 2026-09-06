# Checklist Audit Backend Go

Baca file sungguhan sebelum mencentang butir apa pun. Untuk tiap butir, cari bukti konkret (nama fungsi, file) — jangan menyimpulkan dari nama package saja.

## 1. Router — Chi v5 (github.com/go-chi/chi/v5)

- **Struktur routing**: apakah route dikelompokkan pakai `r.Route()`/`r.Group()` per domain/resource, atau semua route ditumpuk flat di satu fungsi besar? Routing flat menyulitkan penambahan middleware per-grup nanti.
- **Middleware chain**: urutan middleware penting. Cek apakah recovery middleware (`middleware.Recoverer`) dipasang paling luar — kalau tidak, satu panic di handler bisa mematikan seluruh goroutine server tanpa response yang rapi ke klien.
- **Timeout**: apakah ada `middleware.Timeout` atau context timeout eksplisit di handler yang memanggil DB/Redis/HTTP eksternal? Tanpa ini, request yang macet bisa menggantung selamanya dan menghabiskan koneksi.
- **Versioning API**: apakah ada pemisahan versi (`/api/v1/...`) atau perubahan breaking langsung menimpa endpoint lama?
- **CORS & header keamanan**: kalau ini API yang diakses dari frontend web/, cek middleware CORS eksplisit (bukan wildcard `*` di production) dan header seperti `X-Content-Type-Options`.
- **Context propagation**: apakah `r.Context()` diteruskan ke pemanggilan pgx/redis, atau dibuat context baru (`context.Background()`) di tengah handler? Yang kedua membuat cancellation dari klien tidak pernah sampai ke query DB — request yang sudah dibatalkan klien tetap jalan penuh di server.
- **Validasi input**: apakah body/query param divalidasi sebelum dipakai (struct tag validator, atau manual check), termasuk terhadap kemungkinan payload berlebihan (request body size limit)?

## 2. Database — pgx v5 + puddle (github.com/jackc/pgx/v5)

- **Konfigurasi pool**: cek `pgxpool.Config` — apakah `MaxConns`, `MinConns`, `MaxConnLifetime`, `MaxConnIdleTime`, dan `HealthCheckPeriod` diset eksplisit sesuai kapasitas DB, atau dibiarkan default? Default puddle bisa jauh dari optimal untuk beban production.
- **Kebocoran koneksi/rows**: setiap `pool.Query()` harus diikuti `defer rows.Close()` di jalur yang benar (termasuk jalur error sebelum baris pertama dibaca). Cari pola `rows, err := ...` tanpa `defer rows.Close()` segera setelahnya.
- **Parameterized query**: pastikan tidak ada string concatenation/`fmt.Sprintf` untuk membangun SQL dari input pengguna — ini celah SQL injection. pgx mendukung parameter `$1, $2, ...`; semua input harus lewat situ.
- **Transaksi**: cek `pool.Begin()` selalu diikuti `defer tx.Rollback(ctx)` sebelum `tx.Commit()` — rollback setelah commit sukses adalah no-op yang aman di pgx, jadi pola defer ini wajib ada di setiap transaksi untuk mencegah koneksi tersangkut kalau ada error di tengah.
- **Context per-query**: apakah tiap query pakai context yang punya timeout (dari request atau `context.WithTimeout` eksplisit), terutama untuk query yang berpotensi lambat (agregasi, laporan)?
- **Prepared statement & batching**: untuk query yang dipanggil berulang dalam loop, cek apakah pakai `pgx.Batch` untuk mengurangi round-trip, atau malah query satu-satu di dalam loop (N+1 pattern).
- **Migrasi skema**: apakah ada tooling migrasi (goose, migrate, dsb.) dan apakah skema di migrasi konsisten dengan struct Go yang di-scan?
- **Scanning hasil**: cek penggunaan `pgx.RowToStructByName` atau manual `Scan()` — pastikan jumlah dan tipe kolom yang di-scan cocok dengan query, dan error dari `Scan` selalu dicek.

## 3. Caching — go-redis v9 (github.com/redis/go-redis/v9)

- **TTL pada key cache**: apakah setiap `Set`/`SetEx` menyertakan TTL? Key tanpa TTL bisa menumpuk selamanya dan jadi kebocoran memori Redis yang perlahan.
- **Cache stampede**: untuk key populer yang mahal dihitung ulang, apakah ada mitigasi (lock/singleflight, atau TTL jitter) agar banyak request bersamaan tidak sama-sama miss dan menghantam DB sekaligus saat key kedaluwarsa?
- **Penanganan cache miss/error**: apakah error dari Redis (termasuk `redis.Nil` untuk key tidak ada) dibedakan dari error koneksi sungguhan, dan apakah ada fallback ke DB / graceful degradation kalau Redis down — atau apakah Redis jadi single point of failure yang bisa menjatuhkan seluruh endpoint?
- **Connection pool**: cek `redis.Options{PoolSize, MinIdleConns, ...}` — default go-redis v9 mungkin cukup untuk trafik kecil tapi perlu ditinjau untuk beban production.
- **Context propagation**: sama seperti pgx, pastikan context request diteruskan ke pemanggilan go-redis (`rdb.Get(ctx, key)`), bukan context kosong.
- **Naming convention key**: apakah ada pola penamaan key yang konsisten (misalnya `resource:id:field`) untuk memudahkan invalidasi dan debugging, atau string key ad-hoc tersebar di banyak file?
- **Pipeline/transaksi**: untuk operasi multi-key yang perlu atomik atau efisien, cek pemakaian `Pipeline()`/`TxPipeline()` alih-alih memanggil command satu-satu.

## 4. Observability — Prometheus client_golang (github.com/prometheus/client_golang)

- **Kardinalitas label**: ini yang paling sering jadi masalah tersembunyi. Cek apakah ada label metrik yang nilainya tak terbatas (user ID, request ID, path dengan parameter dinamis seperti `/users/123`). Ini bikin jumlah time series meledak dan membebani memori Prometheus maupun aplikasi.
- **Jenis metrik yang tepat**: Counter untuk nilai yang hanya naik (total request), Gauge untuk nilai naik-turun (koneksi aktif), Histogram untuk distribusi (latency). Cek apakah dipakai sesuai kegunaannya.
- **Endpoint `/metrics`**: apakah endpoint ini diekspos tanpa autentikasi ke publik? Idealnya dibatasi ke jaringan internal/scraper saja, karena bisa membocorkan detail internal (nama endpoint, pola trafik).
- **Middleware metrik HTTP**: apakah ada middleware Chi yang otomatis mencatat latency & status code semua request, atau instrumentasi manual tersebar tidak konsisten di tiap handler?
- **Registrasi metrik**: pastikan metrik didaftarkan sekali (variabel package-level atau lewat DI), bukan dibuat ulang tiap request — membuat Counter/Histogram baru tiap request adalah bug umum yang membuat registrasi gagal atau nilai tidak terakumulasi.

## 5. Praktik umum Go

- **Error handling**: apakah error di-wrap dengan konteks (`fmt.Errorf("...: %w", err)`) sehingga jejaknya bisa ditelusuri, atau error mentah langsung dikembalikan/di-log tanpa konteks?
- **Panic vs error**: panic seharusnya hanya untuk kondisi benar-benar tak terduga (bug), bukan untuk alur kontrol normal seperti validasi input gagal.
- **Concurrency safety**: kalau ada shared state (cache in-memory, counter), cek proteksi mutex atau penggunaan channel yang benar. Cari goroutine yang di-spawn tanpa mekanisme untuk menunggu selesai (potensi goroutine leak) terutama goroutine yang dibuat per-request.
- **Graceful shutdown**: apakah server menangani `SIGTERM`/`SIGINT` untuk menutup koneksi pool (pgx, redis) dan menyelesaikan request yang sedang berjalan sebelum proses mati?
- **Konfigurasi & secrets**: apakah kredensial DB/Redis diambil dari environment variable/secret manager, bukan hardcoded di kode?
- **Logging**: apakah logging terstruktur (misalnya JSON, dengan request ID untuk tracing lintas layanan) atau `fmt.Println` tersebar?
- **Test coverage**: apakah ada unit test untuk logic bisnis inti dan/atau integration test untuk lapisan DB (misalnya pakai testcontainers)?
