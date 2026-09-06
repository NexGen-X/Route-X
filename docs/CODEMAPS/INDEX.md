# Route-X Architecture Codemaps Index

**Last Updated:** 2026-09-06  
**Repository:** [Route-X](file:///root/Route-X)  
**Entry Points:**
- Backend: [`cmd/ai-gateway/main.go`](file:///root/Route-X/cmd/ai-gateway/main.go)
- Secret Rotation Tool: [`cmd/routex-rotate/main.go`](file:///root/Route-X/cmd/routex-rotate/main.go)
- Frontend: [`web/src/main.tsx`](file:///root/Route-X/web/src/main.tsx)
- OpenAPI Documentation: [`docs/openapi.yaml`](file:///root/Route-X/docs/openapi.yaml), [`docs/embed.go`](file:///root/Route-X/docs/embed.go)

---

## Overview

Route-X adalah enterprise AI Inference Gateway dan intelligent router berkinerja tinggi yang didistribusikan sebagai **biner tunggal** mandiri (~25 MB). Route-X menyatukan reverse proxy inferensi LLM, perutean multi-provider pintar, mitigasi cache stampede berbasis Singleflight, manajemen kuota/anggaran terdistribusi, worker supervisor background, dan konsol web SPA tersemat (React 18 + TanStack Query + Tailwind CSS).

```
+-----------------------------------------------------------------------------------------+
|                                    Route-X Gateway                                      |
|                                                                                         |
|  +--------------------+   +---------------------+   +--------------------------------+  |
|  |   Frontend SPA     |   |   Public Gateway    |   |         Admin REST API         |  |
|  |  React 18 + Vite   |   |   /v1/chat/...      |   |           /api/admin           |  |
|  |   TanStack Query   |   |   SSE Streaming     |   |       Session + CSRF RBAC      |  |
|  +---------+----------+   +----------+----------+   +---------------+----------------+  |
|            |                         |                              |                   |
|            v                         v                              v                   |
|  +-----------------------------------------------------------------------------------+  |
|  |                            HTTP Router (Go Chi v5)                                |  |
|  |   httpx.Recover | httpx.SecurityHeaders | httpx.MaxBytes | httpx.MetricsRecorder  |  |
|  +-----------------------------------+-----------------------------------------------+  |
|                                      |                                                  |
|     +--------------------------------+--------------------------------+                 |
|     |                                |                                |                 |
|     v                                v                                v                 |
|  +--------------------+   +--------------------+   +---------------------------------+  |
|  |  Response Cache    |   |   Smart Router     |   |        Resilience Pipeline      |  |
|  |  Redis + Single-   |   |  6 Strategies &    |   |  Circuit Breaker (Redis Lua)    |  |
|  |  flight Protection |   |  Failover Rules    |   |  Retry Backoff + Egress Tunnels |  |
|  +---------+----------+   +----------+---------+   +-----------------+---------------+  |
|            |                         |                               |                  |
|            +-------------------------+-------------------------------+                  |
|                                      |                                                  |
|                                      v                                                  |
|                +-------------------------------------------+                            |
|                |         Upstream Adapters & Xray          |                            |
|                |  OpenAI, Anthropic, Gemini, Custom/vLLM   |                            |
|                +---------------------+---------------------+                            |
|                                      |                                                  |
+--------------------------------------|--------------------------------------------------+
                                       |
                   +-------------------+-------------------+
                   |                                       |
                   v                                       v
         +--------------------+                 +--------------------+
         |   PostgreSQL 16    |                 |      Redis 7       |
         |    (jackc/pgx)     |                 |   (go-redis/v9)    |
         +--------------------+                 +--------------------+
```

---

## Area Codemaps

| Codemap | Deskripsi Area & Komponen Utama | File Tautan |
| :--- | :--- | :--- |
| **[Frontend Codemap](file:///root/Route-X/docs/CODEMAPS/frontend.md)** | Arsitektur SPA React 18, TanStack Query v5, utilitas CSS `cn()`, komponen UI modular, routing, context providers | [`frontend.md`](file:///root/Route-X/docs/CODEMAPS/frontend.md) |
| **[Backend Codemap](file:///root/Route-X/docs/CODEMAPS/backend.md)** | Biner tunggal Go Chi v5, Singleflight response cache, middleware chain, router inferensi, security pipeline, REST admin | [`backend.md`](file:///root/Route-X/docs/CODEMAPS/backend.md) |
| **[Database Codemap](file:///root/Route-X/docs/CODEMAPS/database.md)** | Skema PostgreSQL 16, driver `pgxpool`, sistem migrasi embedded, struktur repository domain, keyset pagination | [`database.md`](file:///root/Route-X/docs/CODEMAPS/database.md) |
| **[Integrations Codemap](file:///root/Route-X/docs/CODEMAPS/integrations.md)** | Adapter provider LLM, tunnel siluman Xray (Reality/VMess/Trojan), webhook dispatcher asinkron, Prometheus scraper | [`integrations.md`](file:///root/Route-X/docs/CODEMAPS/integrations.md) |
| **[Workers Codemap](file:///root/Route-X/docs/CODEMAPS/workers.md)** | Background worker supervisor, PostgreSQL advisory lock singleton, health checks, retention cleaner, usage rollups | [`workers.md`](file:///root/Route-X/docs/CODEMAPS/workers.md) |

---

## Architectural Decision Records (ADR)

- [ADR 0001: Use TanStack Query for Data Fetching in Frontend](file:///root/Route-X/docs/adr/0001-use-tanstack-query-for-data-fetching.md)
- [ADR 0002: Use Singleflight for LLM Response Cache Stampede Prevention](file:///root/Route-X/docs/adr/0002-use-singleflight-for-llm-cache-stampede.md)

---

## Quick Setup & Verifikasi

```bash
# 1. Unduh dependensi backend dan frontend
make deps

# 2. Jalankan pengujian statis dan unit
make fmt
make vet
make test-unit

# 3. Kompilasi biner mandiri (termasuk build frontend SPA ke web/dist)
make build

# 4. Jalankan gateway inferensi
./ai-gateway
```
