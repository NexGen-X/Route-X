# Backend Architecture Codemap

**Last Updated:** 2026-09-06  
**Area:** Go Backend & Core Inference Gateway  
**Entry Points:**
- [`cmd/ai-gateway/main.go`](file:///root/Route-X/cmd/ai-gateway/main.go) — Single binary main entry point, dependency injection, HTTP server setup
- [`cmd/routex-rotate/main.go`](file:///root/Route-X/cmd/routex-rotate/main.go) — Standalone administrative credential re-encryption tool
- [`internal/gateway/http.go`](file:///root/Route-X/internal/gateway/http.go) — LLM inference routing pipeline & streaming engine
- [`internal/responsecache/responsecache.go`](file:///root/Route-X/internal/responsecache/responsecache.go) — Redis response caching engine with Singleflight deduplication

---

## Architecture

```
+-----------------------------------------------------------------------------------------+
|                                    Backend Request Pipeline                             |
|                                                                                         |
|       Client Request (Bearer sk_live_... / Session Cookie)                              |
|                               |                                                         |
|                               v                                                         |
|  +-----------------------------------------------------------------------------------+  |
|  |                             Root Router (Chi v5)                                  |  |
|  |                                                                                   |  |
|  |   [httpx.Recover]       --> Panic handler & stack logging                         |  |
|  |   [httpx.SecurityHeaders] -> HSTS, CSP, X-Content-Type-Options                    |  |
|  |   [httpx.MaxBytes]      --> 10 MB payload limiter                                 |  |
|  |   [httpx.MetricsRecorder] -> In-flight & total request metrics per Chi route      |  |
|  |   [httpx.CORS]          --> Preflight OPTIONS handler                             |  |
|  +-----------------------------------+-----------------------------------------------+  |
|                                      |                                                  |
|         +----------------------------+----------------------------+                     |
|         |                                                         |                     |
|         v                                                         v                     |
|  +---------------------------------------+       +-----------------------------------+  |
|  |     Inference Surface (/v1/*)         |       |      Admin API Surface (/api/*)   |  |
|  |                                       |       |                                   |  |
|  |  1. apikey.Authenticator              |       |  1. auth.RequireSessionOrBearer   |  |
|  |  2. ratelimit.Limiter (Redis Lua)     |       |  2. auth.RequireCSRF              |  |
|  |  3. contentfilter.Filter              |       |  3. RBAC (admin/operator/viewer)  |  |
|  |  4. responsecache.GetOrFetch()        |       |  4. admin.Routes() handlers       |  |
|  |     └─ Singleflight deduplication     |       +-----------------------------------+  |
|  |  5. router.Engine (6 strategies)      |                                              |
|  |  6. gateway.CircuitBreaker            |                                              |
|  |  7. providers.Adapter (OpenAI wire)   |                                              |
|  |  8. usage.Recorder & Billing          |                                              |
|  +-------------------+-------------------+                                              |
|                      |                                                                  |
+----------------------|------------------------------------------------------------------+
                       |
        +--------------+--------------+
        |                             |
        v                             v
+------------------+          +------------------+
|  Upstream LLM    |          |  Redis 7 Cluster |
|  (via Xray pool) |          |  (Cache & Locks) |
+------------------+          +------------------+
```

---

## Key Modules & Recent Features

### 1. Response Cache & Singleflight Engine
Sesuai [ADR 0002](file:///root/Route-X/docs/adr/0002-use-singleflight-for-llm-cache-stampede.md), sistem caching mengintegrasikan **`golang.org/x/sync/singleflight`** di dalam [`internal/responsecache/responsecache.go`](file:///root/Route-X/internal/responsecache/responsecache.go).

- **Cache Miss Thundering Herd Prevention:** Ketika puluhan request identik masuk bersamaan dan cache kosong/kadaluarsa, fungsi `GetOrFetch(ctx, key, fetchFn)` mengunci penerbangan pertama via `sfg.Do(key, ...)`.
- **Transient Blocking:** Seluruh goroutine penunggu ditahan secara transien di memori Go. Begitu panggilan pertama berhasil disimpan ke Redis, seluruh penunggu langsung menerima hasil yang sama tanpa mengirim request ganda ke upstream AI.

```go
func (e *Engine) GetOrFetch(ctx context.Context, key string, fetchFn func() (*Entry, error)) (*Entry, bool, error) {
    if !e.IsEnabled() || key == "" {
        return fetchFn()
    }
    // 1. Coba baca dari Redis
    if entry, hit, err := e.Get(ctx, key); err == nil && hit && entry != nil {
        return entry, true, nil
    }
    // 2. Singleflight deduplikasi goroutine konkuren
    res, err, _ := e.sfg.Do(key, func() (any, error) {
        newEntry, fetchErr := fetchFn()
        if fetchErr == nil && newEntry != nil {
            _ = e.Set(context.WithoutCancel(ctx), key, newEntry)
        }
        return newEntry, fetchErr
    })
    return res.(*Entry), false, err
}
```

### 2. Prometheus MetricsRecorder Middleware
Diimplementasikan di [`internal/httpx/metrics.go`](file:///root/Route-X/internal/httpx/metrics.go) dan disuntikkan ke root router di [`cmd/ai-gateway/main.go`](file:///root/Route-X/cmd/ai-gateway/main.go):
- Mengukur durasi request (`HTTPDuration`), status code (`HTTPRequestsTotal`), dan in-flight request (`HTTPInFlight`).
- **Mencegah Ledakan Kardinalitas:** Menggunakan path pola Chi (`routeCtx.RoutePattern()`, misal `/v1/models/{id}`) alih-alih URL mentah.
- **Proteksi Scraper `/metrics`:** Endpoint dilindungi oleh `RequireSessionOrBearer(metricsToken, allowLoopback)` agar aman dari akses luar namun dapat di-scrape oleh Prometheus internal.

### 3. Paket Utama (Internal Packages)

| Paket | Lokasi Direktori | Tugas & Fungsi Utama |
| :--- | :--- | :--- |
| **`gateway`** | [`internal/gateway/`](file:///root/Route-X/internal/gateway/) | Pipeline inferensi LLM, streaming SSE dengan `http.ResponseController`, fallback & retry |
| **`responsecache`** | [`internal/responsecache/`](file:///root/Route-X/internal/responsecache/) | Engine cache respon inferensi berbasis Redis + Singleflight |
| **`router`** | [`internal/router/`](file:///root/Route-X/internal/router/) | Smart routing: `priority`, `lowest_cost`, `lowest_latency`, `weighted`, `round_robin`, `fallback` |
| **`apikey`** | [`internal/apikey/`](file:///root/Route-X/internal/apikey/) | Otentikasi Client Key HMAC-SHA256 ber-pepper, pembatasan model/provider/scope |
| **`auth`** | [`internal/auth/`](file:///root/Route-X/internal/auth/) | Otentikasi Admin Console: Argon2id password hashing, CSRF token, Session store |
| **`ratelimit`** | [`internal/ratelimit/`](file:///root/Route-X/internal/ratelimit/) | Distributed sliding window rate limiter berbasis Redis Lua script (RPS, RPM, TPM) |
| **`security`** | [`internal/security/`](file:///root/Route-X/internal/security/) | Enkripsi AES-256-GCM dengan AAD terikat UUID, SSRF IP-range filter guard |
| **`admin`** | [`internal/admin/`](file:///root/Route-X/internal/admin/) | 7 domain REST API untuk konsol manajemen, keyset paginator, audit trail |
| **`httpx`** | [`internal/httpx/`](file:///root/Route-X/internal/httpx/) | Middleware suite (`Recover`, `SecurityHeaders`, `MetricsRecorder`, `CORS`, `MaxBytes`) |

---

## Data Flow

1. Klien mengirim inferensi `POST /v1/chat/completions` dengan header `Authorization: Bearer sk_live_...`.
2. `httpx.MetricsRecorder` menaikkan counter `HTTPInFlight` dan memulai timer latensi.
3. `apikey.Authenticator` mengekstrak token, menghitung hash HMAC dengan pepper rahasia, memvalidasi kuota dan batas model.
4. `ratelimit.Limiter` memeriksa sliding window RPM/TPM di Redis melalui skrip Lua atomik.
5. `responsecache.GetOrFetch` memeriksa cache Redis; jika miss, Singleflight mengunci eksekusi bersama agar tidak membebani upstream.
6. `router.Engine` memilih upstream provider terbaik berdasarkan strategi aktif dan status circuit breaker.
7. Gateway menerjemahkan format wire ke adapter provider terkait (OpenAI, Anthropic, Gemini) via tunnel Xray jika dikonfigurasi.
8. Streaming chunk SSE di-flush secara non-buffered ke klien via `http.ResponseController`.
9. `usage.Recorder` secara asinkron menyimpan token used, TTFT, dan biaya finansial dengan presisi 8 desimal (`upstream.USD`).

---

## External Dependencies

| Module / Package | Versi | Peran |
| :--- | :--- | :--- |
| `github.com/go-chi/chi/v5` | `v5.3.2` | HTTP Multiplexer & Sub-router |
| `golang.org/x/sync` | `v0.22.0` | Concurrency primitives (`singleflight.Group`) |
| `github.com/redis/go-redis/v9` | `v9.22.0` | Distributed caching, rate-limiting, circuit breaker |
| `github.com/jackc/pgx/v5` | `v5.10.0` | High-performance PostgreSQL driver & connection pool |
| `github.com/prometheus/client_golang` | `v1.24.1` | Metrik instrumentasi operasional `/metrics` |
| `golang.org/x/crypto` | `v0.55.0` | Argon2id & cryptographic helpers |

---

## Setup & Testing Commands

```bash
# Analisis statis kode Go
make fmt
make vet

# Jalankan seluruh unit test (tanpa dependensi DB)
make test-unit

# Jalankan pengujian race condition
make race

# Kompilasi biner produksi
make build
```

## Related Areas
- [Frontend Architecture Codemap](file:///root/Route-X/docs/CODEMAPS/frontend.md)
- [Database Architecture Codemap](file:///root/Route-X/docs/CODEMAPS/database.md)
- [ADR 0002: Singleflight LLM Cache Stampede](file:///root/Route-X/docs/adr/0002-use-singleflight-for-llm-cache-stampede.md)
