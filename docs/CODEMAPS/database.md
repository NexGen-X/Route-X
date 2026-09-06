# Database Architecture Codemap

**Last Updated:** 2026-09-06  
**Area:** Database Storage & Repositories  
**Entry Points:**
- [`internal/database/db.go`](file:///root/Route-X/internal/database/db.go) — PostgreSQL connection pool lifecycle (`pgxpool.Pool`)
- [`internal/database/migrate.go`](file:///root/Route-X/internal/database/migrate.go) — Embedded schema migration engine with advisory lock
- [`internal/database/repo/repo.go`](file:///root/Route-X/internal/database/repo/repo.go) — Domain repository container & transaction managers

---

## Architecture

```
+-----------------------------------------------------------------------------------------+
|                                  PostgreSQL 16 Layer                                    |
|                                                                                         |
|       Application Services (Admin, Gateway, Worker, Authenticator)                     |
|                               |                                                         |
|                               v                                                         |
|  +-----------------------------------------------------------------------------------+  |
|  |                           internal/database/repo                                  |  |
|  |                                                                                   |  |
|  |   repo/identity  -->  Users, Sessions, RBAC (Admin, Operator, Viewer)             |  |
|  |   repo/keys      -->  Client API Keys (Peppered HMAC-SHA256 hashes)               |  |
|  |   repo/upstream  -->  Providers, Credentials (AES-256-GCM), Models, Egress Pools  |  |
|  |   repo/policy    -->  Routing Rules, Budgets, Content Filters, Bans               |  |
|  |   repo/traffic   -->  Requests, Events, Latency, Usage Rollups                    |  |
|  +-----------------------------------+-----------------------------------------------+  |
|                                      |                                                  |
|                                      v                                                  |
|  +-----------------------------------------------------------------------------------+  |
|  |                         internal/database/migrate.go                              |  |
|  |   pg_advisory_lock(0x52584d49) -> Automatic embedded migration execution          |  |
|  |   0001_bootstrap.sql ... 0009_budget_alerts.sql                                   |  |
|  +-----------------------------------+-----------------------------------------------+  |
|                                      |                                                  |
|                                      v                                                  |
|  +-----------------------------------------------------------------------------------+  |
|  |                              jackc/pgx/v5 (pgxpool)                               |  |
|  |   Connection Pool, Parameterized SQL Queries (Zero ORM Overhead), Keyset Paging   |  |
|  +-----------------------------------------------------------------------------------+  |
+-----------------------------------------------------------------------------------------+
```

---

## Database Migrations

File migrasi disimpan di [`internal/database/migrations/`](file:///root/Route-X/internal/database/migrations/) dan disematkan langsung ke biner menggunakan `go:embed`:

| Migrasi | File | Lingkup Skema |
| :--- | :--- | :--- |
| `0001` | [`0001_bootstrap.sql`](file:///root/Route-X/internal/database/migrations/0001_bootstrap.sql) | Ekstensi `pgcrypto`, skema dasar tabel `schema_migrations`, fungsi trigger update waktu |
| `0002` | [`0002_auth.sql`](file:///root/Route-X/internal/database/migrations/0002_auth.sql) | Tabel `users`, `sessions`, `roles`, `permissions`, `audit_logs` |
| `0003` | [`0003_providers.sql`](file:///root/Route-X/internal/database/migrations/0003_providers.sql) | Tabel `providers`, `provider_credentials` (terenkripsi AES-GCM), `egress_pools` |
| `0004` | [`0004_models.sql`](file:///root/Route-X/internal/database/migrations/0004_models.sql) | Tabel `models`, alias perutean model, dan konfigurasi harga token |
| `0005` | [`0005_apikeys.sql`](file:///root/Route-X/internal/database/migrations/0005_apikeys.sql) | Tabel `api_keys`, `api_key_models`, `api_key_providers` |
| `0006` | [`0006_requests.sql`](file:///root/Route-X/internal/database/migrations/0006_requests.sql) | Tabel partitioned `requests` & `request_events` untuk inspeksi jejak LLM |
| `0007` | [`0007_usage.sql`](file:///root/Route-X/internal/database/migrations/0007_usage.sql) | Tabel agregasi `usage_hourly` dan `usage_daily` untuk laporan analitik cepat |
| `0008` | [`0008_automation.sql`](file:///root/Route-X/internal/database/migrations/0008_automation.sql) | Tabel `webhooks`, `webhook_deliveries`, `routing_rules`, `content_filters` |
| `0009` | [`0009_budget_alerts.sql`](file:///root/Route-X/internal/database/migrations/0009_budget_alerts.sql) | Tabel `budgets` dan sistem pelaporan ambang batas pengeluaran |

---

## Domain Repositories

### 1. `repo/keys` ([`internal/database/repo/keys`](file:///root/Route-X/internal/database/repo/keys))
- Menyimpan Client API Key dengan hash HMAC-SHA256 ber-pepper rahasia (token mentah `sk_live_...` hanya ditampilkan sekali saat pembuatan).
- Mendukung pemutaran kunci instan (`Rotate`) dan pencabutan permanen (`Revoke`).

### 2. `repo/upstream` ([`internal/database/repo/upstream`](file:///root/Route-X/internal/database/repo/upstream))
- Mengelola provider inferensi, pool egress proxy keluar, dan model upstream.
- Kredensial rahasia provider disimpan terenkripsi via cipher **AES-256-GCM** dengan AAD yang diikat ke ID unik kredensial.

### 3. `repo/traffic` ([`internal/database/repo/traffic`](file:///root/Route-X/internal/database/repo/traffic))
- Pencatatan request inferensi berkecepatan tinggi dengan keyset pagination (`WHERE id < $1 ORDER BY id DESC LIMIT $2`).
- Rollup agregasi data per jam dan per hari untuk efisiensi kueri analitik dashboard tanpa memindai seluruh tabel mentah.

---

## External Dependencies

| Module | Versi | Catatan |
| :--- | :--- | :--- |
| `github.com/jackc/pgx/v5` | `v5.10.0` | Driver PostgreSQL murni tanpa dependensi CGO |
| `github.com/jackc/puddle/v2` | `v2.2.2` | Connection pool backend untuk `pgx` |

---

## Related Areas
- [Backend Architecture Codemap](file:///root/Route-X/docs/CODEMAPS/backend.md)
- [Workers Codemap](file:///root/Route-X/docs/CODEMAPS/workers.md)
