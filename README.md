# Route-X — Self-Hosted AI Gateway

<div align="center">

**One gateway for every AI provider: OpenAI- and Anthropic-compatible ingestion, multi-provider routing with failover, hard budget guardrails, and a full operations dashboard — shipped as a single Go binary.**

[![Release](https://img.shields.io/badge/Release-v1.5.1-10B981?style=for-the-badge&logo=github)](https://github.com/NexGen-X/Route-X/releases/tag/v1.5.1)
[![Go Version](https://img.shields.io/badge/Go-1.27.3+-00ADD8?style=for-the-badge&logo=go)](https://go.dev)
[![React](https://img.shields.io/badge/React-18-61DAFB?style=for-the-badge&logo=react)](https://react.dev)
[![PostgreSQL](https://img.shields.io/badge/PostgreSQL-16-336791?style=for-the-badge&logo=postgresql)](https://postgresql.org)
[![Redis](https://img.shields.io/badge/Redis-7-DC382D?style=for-the-badge&logo=redis)](https://redis.io)
[![CI](https://github.com/NexGen-X/Route-X/actions/workflows/ci.yml/badge.svg)](https://github.com/NexGen-X/Route-X/actions/workflows/ci.yml)
[![License: MIT](https://img.shields.io/badge/License-MIT-yellow?style=for-the-badge)](LICENSE)

</div>

> **Release status.** The latest release is **v1.5.1**. The `main` branch is ahead of that release — it carries the audit3 hardening campaign and follow-up batches that have not been tagged yet. Everything described in this README reflects the code in this repository; see [`CHANGELOG.md`](CHANGELOG.md) (`[Unreleased]`) for what is newer than v1.5.1.

> **Scope — read before deploying.** Route-X is an open-source project built for **personal use and small, trusted teams**. Its identity model is **single-admin**: every authenticated dashboard user has full administrator rights — there is no role-based access control and no multi-tenant isolation between dashboard users. Do not expose a Route-X instance to untrusted users, and protect internet-facing instances like any other admin panel that stores paid upstream API credentials.

## Why Route-X?

Calling heterogeneous AI provider APIs directly means dialect fragmentation (OpenAI vs. Anthropic schemas), API-key sprawl across tools, unpredictable rate limits, silent provider outages, and untracked token spend. Route-X sits in front of all of it as a self-hosted reverse proxy and control plane:

- **One endpoint for both dialects** — OpenAI Chat Completions and Anthropic Messages are ingested natively and transpiled in both directions, including streaming.
- **Routing that survives outages** — priority tiers, weighted / lowest-latency / lowest-cost / round-robin strategies, a 3-state circuit breaker, and cascade failover across providers.
- **Budgets that actually hold** — estimated cost is *reserved atomically before* a request runs, so a parallel burst cannot overshoot a limit the way after-the-fact accounting can.
- **A control plane, not just a proxy** — an embedded dashboard for providers, routing rules, API keys, budgets, webhooks, egress pools, backups, and observability, plus one-click synchronization of 20 developer CLI tools.

The application tier is a **single static Go binary** (dashboard embedded via `//go:embed` — no Node.js runtime or separate static host). **PostgreSQL 16** and **Redis 7** are required as separate state services.

---

## Features

### Dual-protocol ingestion
- **OpenAI-compatible**: `/v1/chat/completions`, `/v1/embeddings`, `/v1/models`, `/v1/models/{model}` — a drop-in target for the official OpenAI SDKs, LangChain, LiteLLM, Cursor, Cline, Open WebUI, and similar tools. `/v1/responses` is routed as a **compatibility alias** of the chat-completions handler; it is not a separate implementation of the OpenAI Responses API.
- **Anthropic-native**: `/v1/messages` (plus the `/v1/v1/messages` alias) and `/v1/messages/count_tokens` — a drop-in target for the Anthropic SDK and Claude Code.
- **Bidirectional transpilation** of roles, content blocks, base64 vision payloads, and tool/function calling between the two dialects, with a resilient SSE streaming engine (UTF-8 boundary safety, immediate upstream cancellation on client disconnect).
- **Anthropic document blocks**: text documents and base64 `text/plain` documents are converted to canonical text at their block position and forwarded to any upstream; base64 **PDF** documents are passed through natively to Anthropic-kind upstreams only — if no Anthropic candidate is available the request is rejected with `400 unsupported_document_upstream` before any network call, never silently dropped. URL-sourced documents are rejected (SSRF). Maximum 10 MiB per document.

### Routing, resiliency, and rate limiting
- **Combo routing strategies**: strict priority tiering, Efraimidis-Spirakis weighted distribution, lowest tracked P95 latency, lowest cost per token, and atomic round-robin. Candidates without latency/pricing measurements are ranked last, never treated as fastest or free.
- **3-state circuit breaker** (Closed → Open → Half-Open canary probes) with state shared through Redis, plus proactive context-overflow bypass to higher-context models.
- **Distributed rate limiting**: an atomic Redis Lua fixed-window limiter (check-all-windows, then increment-all) enforces RPM/TPM and concurrency limits on the live path, shared across instances.
- **Response cache (opt-in)**: identical non-streaming requests can be served from Redis. Cache keys are isolated **per API key** (domain-separated hash of the key identity + the full canonical request + surface + route fingerprint, so routing changes make old entries unreachable). Only successful, non-streaming responses **without tool calls** are cached in this phase; streaming is never cached. Enable it in **Settings → Performance & Cache**.

### Money: budgets and cost accounting
- **Atomic budget reservation**: before a request runs, its estimated cost is held against every applicable budget (global, API key, provider, model) and settled against the real cost afterwards — a burst of parallel requests cannot spend past the cap.
- **Budget periods follow your calendar**: daily (rolling 24 h), weekly (Monday 00:00), and monthly (the 1st, 00:00) windows are computed in the operator timezone set by `BUDGET_TIMEZONE` (default UTC).
- **Provider-accurate pricing**: costs use the price effective at request time (8-decimal USD precision, no floats in the money path). Anthropic cache-creation tokens are priced from the reported breakdown — **1-hour cache writes at 2×** the input price, **5-minute writes and unattributed remainder at 1.25×**; without a breakdown the previous flat 1.25× behavior applies.

### Control plane and integrations
- **Embedded dashboard** ("Calm Console" design language: Claude-inspired dark theme, flat KPI strips instead of stacked cards, full Bahasa Indonesia UI, responsive + PWA for mobile — no separate native app).
- **Webhooks with verifiable signatures**: deliveries are signed with HMAC-SHA256 over `"<timestamp>.<deliveryID>.<final body>"` and sent in the **`X-RouteX-Signature`** header (`sha256=<hex>`). A webhook secret is required (empty secrets are rejected), failed deliveries retry with backoff and terminate in an `abandoned` state instead of retrying forever, and cross-host redirects are refused.
- **Egress pools & optional relays**: per-credential HTTP/SOCKS5 proxy pools with periodic health probing; unhealthy pools are skipped while a healthy alternative exists (fail-open if none is healthy). Deno Deploy and Cloudflare relays are **optional** intermediaries for egress — relays created by Route-X provisioning carry a random per-installation token generated on your own server; the repository contains code only, never your settings or secrets.
- **Developer CLI synchronization**: one-click provisioning for 20 terminal tools (Claude Code, OpenCode, Aider, Ollama, Goose, Cursor Agent, Copilot, and more). The Antigravity CLI (`agy`) and `~/.gemini` are strictly protected and never touched.
- **Backup & restore**: one-click database export (streaming) and restore. Restores are **scratch-swap atomic**: the upload is scanned for psql meta-commands, restored into a scratch database in a single transaction, checked against the schema ledger, and only then swapped in — the previous database is kept as `<db>_pre_restore`.
- **Observability**: request inspector with latency waterfall and event timeline, traffic breakdowns whose percentages are computed from true totals, Prometheus metrics, and `/healthz` + `/readyz` probes that return status only (`{"status":"ok"}` — no version or uptime leakage).

### Accessibility
The dashboard is **designed toward WCAG 2.1 Level AA**: focus trapping in dialogs and the command palette, WAI-ARIA combobox semantics, labelled form controls, and internal axe-core audits reporting zero color-contrast violations across dashboard pages. It is a design target verified by internal checks — Route-X has **not** undergone external WCAG certification.

---

## Dashboard Preview

Every screenshot below was captured from a live install built from the current `main` branch; the numbers shown are that test gateway's real recorded traffic — Route-X never renders placeholder or mock data.

### Desktop

**Dashboard** — one plain status line, a flat KPI strip (requests, tokens & cost, P95 latency), a live request feed, and provider health as simple rows.

![Route-X Dashboard — desktop](docs/images/dashboard-desktop.png)

**Providers** — every upstream as a two-line row: status dot, model count, priority and weight. Provider presets (OpenAI, Anthropic, Google, OpenRouter, local runtimes, and more) are added with a single click, then mapped to models with per-provider pricing.

![Route-X Providers — desktop](docs/images/penyedia-desktop.png)

**Observability** — underline metric tabs (Requests, Throughput, Tokens, Error Rate, Cost, P95 Latency) over one large chart, traffic composition broken down by provider / model / API key, and server & pool health as plain rows.

![Route-X Observability — desktop](docs/images/observabilitas-desktop.png)

**Command Palette (`Ctrl+K` / `⌘K`)** — a Raycast-style palette for the whole console: results grouped by category (Navigation, Quick Actions, CLI Tools), full keyboard navigation.

![Route-X Command Palette — desktop](docs/images/palet-perintah-desktop.png)

**API Keys** — key rows with masked secret, RPM/TPM limits, last-used time, and model/provider scope. When a key has a budget registered in the Budgets system, the row also shows a thin **budget bar with real recorded spend versus its limit** (amber at the alert threshold, red at 100%) — keys without a budget show no bar at all.

![Route-X API Keys — desktop](docs/images/kunci-api-desktop.png)

**Settings** — Linear-style left sub-navigation with panel-free content: label–description–control rows for Domain & HTTPS (with DNS guidance and automatic Let's Encrypt), Performance & Cache, Backup & Recovery, and free-form Advanced runtime parameters.

![Route-X Settings — desktop](docs/images/pengaturan-desktop.png)

### Mobile (responsive web + PWA — no separate native app)

| Dashboard | Providers | API Keys with budget bar |
|---|---|---|
| <img src="docs/images/dashboard-mobile.png" alt="Route-X Dashboard — mobile" width="260"> | <img src="docs/images/penyedia-mobile.png" alt="Route-X Providers — mobile" width="260"> | <img src="docs/images/kunci-api-mobile.png" alt="Route-X API Keys with per-key budget bar — mobile" width="260"> |

**Request detail as a bottom sheet** — tapping any request in the mobile list raises its full inspector as a sheet from the bottom (status, tokens, cost, latency waterfall, event timeline, payload & cURL) without leaving the list or losing scroll context.

<img src="docs/images/permintaan-sheet-mobile.png" alt="Route-X Request detail bottom sheet — mobile" width="300">

---

## Quickstart

The fastest path is the automated native installer (details under [Installation](#installation)); for a guided first run — creating your first provider, model mapping, and API key — follow the [Quickstart Guide](docs/QUICKSTART.md). Point any OpenAI- or Anthropic-compatible client at the gateway:

```bash
# OpenAI dialect
curl http://localhost:8080/v1/chat/completions \
  -H "Authorization: Bearer sk_live_<your-routex-key>" \
  -H "content-type: application/json" \
  -d '{"model":"gpt-5","messages":[{"role":"user","content":"Hello"}]}'

# Anthropic dialect
curl http://localhost:8080/v1/messages \
  -H "x-api-key: sk_live_<your-routex-key>" \
  -H "anthropic-version: 2023-06-01" \
  -H "content-type: application/json" \
  -d '{"model":"claude-sonnet-4.5","max_tokens":1024,"messages":[{"role":"user","content":"Hello"}]}'
```

---

## Installation

### Option A: Automated native installer (recommended for Linux servers)

Installs Route-X, PostgreSQL 16, Redis 7, and Caddy (reverse proxy with automatic TLS) under systemd. The installer downloads the official release binary for your architecture (`amd64`/`arm64`, selected via `uname -m`) and verifies its published `.sha256` checksum before installing; compiling from source is only a fallback. An existing Caddyfile is backed up before anything is overwritten.

```bash
curl -fsSL https://raw.githubusercontent.com/NexGen-X/Route-X/main/install-native.sh | sudo bash
```

> Review the script before running it, as with any `curl | sudo bash` installer. The installer pins a release tag (`RELEASE_TAG` in the script), which can lag the newest GitHub release — verify afterwards with `./ai-gateway -version`.

#### Updating a native install

```bash
git pull origin main
make live-update
```

This builds the new binary in isolation, applies migrations before the swap, swaps the binary atomically, probes `/healthz`, and rolls back to the previous binary if the health check fails. The service restarts during the swap, so this is a **brief-restart update with a safety net** — not a guaranteed zero-downtime cutover, and in-flight streaming connections are not promised to survive it.

### Option B: Docker stack

Deploys the complete containerized stack (gateway, PostgreSQL, Redis, Caddy):

```bash
git clone https://github.com/NexGen-X/Route-X.git
cd Route-X
bash deploy.sh
```

### Option C: Pre-built release binary

```bash
# 1. Download the official release (adjust the architecture if needed)
curl -fsSLO https://github.com/NexGen-X/Route-X/releases/download/v1.5.1/routex-v1.5.1-linux-amd64.tar.gz

# 2. Extract and run (migrations apply automatically on boot)
tar -xzvf routex-v1.5.1-linux-amd64.tar.gz
./ai-gateway
```

You need PostgreSQL 16 and Redis 7 reachable, plus the required environment variables below.

### Option D: Build from source

Requirements: **Go 1.27.3+**, **Node.js 24+**, **PostgreSQL 16**, **Redis 7**.

```bash
git clone https://github.com/NexGen-X/Route-X.git
cd Route-X
make build           # frontend + gateway binary + operations CLI into ./bin
./bin/ai-gateway     # migrations apply automatically on boot
```

Hosted alternatives are documented in the [Fly.io](docs/deploy-fly-io.md) and [Render](docs/deploy-render.md) guides.

---

## First-Run Onboarding

**There is no default password.** The first admin is created **only** when `INITIAL_ADMIN_PASSWORD` is set (minimum 16 characters in production). If it is empty, admin creation is *skipped* — no account with a known password is ever created.

```bash
export INITIAL_ADMIN_EMAIL="admin@routex.local"                 # optional; this is the default
export INITIAL_ADMIN_PASSWORD="$(openssl rand -base64 24)"      # generate your own; store it safely
```

1. Start Route-X, then open `http://localhost:8080/login` (or `https://your-domain.com/login`) and sign in with that email and password.
2. The initial admin is created with `MustChangePassword`, so changing the password is mandatory at first login. Once changed, the login page's setup hint is permanently disabled.
3. ⚠️ Instances originally seeded by older versions with the previously published default password must change it immediately — anyone who has read this repository's history knows that value.

---

## Configuration

Route-X is configured entirely through environment variables. The table below lists the core set; the complete, commented reference is [`.env.example`](.env.example), and the authoritative source is `internal/config/config.go`. Note: the meaning of `0` is **not uniform** across knobs (for some it means "disabled", for others "use default" or "unlimited") — check `.env.example` per variable before setting `0`.

| Variable | Required | Purpose / default |
|---|:---:|---|
| `DATABASE_URL` | ✅ | PostgreSQL connection string. |
| `REDIS_URL` | ✅ | Redis connection string. |
| `SESSION_SECRET` | ✅ | Signs session CSRF tokens. Rotating it invalidates CSRF tokens, **not** sessions. |
| `ENCRYPTION_KEY` | ✅ | Base64 32-byte key encrypting provider credentials (AES-256-GCM). |
| `ENCRYPTION_KEY_PREVIOUS` | – | Previous encryption key during rotation grace period. |
| `API_KEY_PEPPER` | ✅ | Server-side pepper for API-key HMAC hashing. **Never rotate it routinely** — rotating invalidates every existing API key. |
| `INITIAL_ADMIN_EMAIL` / `INITIAL_ADMIN_PASSWORD` | – | First-admin seed (see onboarding). Password ≥ 16 chars in production; empty password = no admin created. |
| `PORT` | – | HTTP port (default `8080`). |
| `PUBLIC_URL` | – | Public base URL used for generated links and gateway URL discovery. |
| `APP_ENV` | – | `production` enables production hardening (e.g. `__Host-` session cookie on HTTPS). |
| `SESSION_TTL` | – | Sliding session lifetime, 5 min – 7 days (default 12 h); an absolute 7-day cap and a limit of 10 concurrent sessions per user always apply. |
| `BUDGET_TIMEZONE` | – | IANA timezone for budget period boundaries (default `UTC`). An invalid name fails startup instead of silently falling back. |
| `TRUSTED_PROXIES` | – | Proxy addresses whose `X-Forwarded-For` is trusted. On native installs the installer sets loopback only; the Docker stack pins its compose subnet. Leave unset if Route-X is directly exposed. |
| `UPSTREAM_TIMEOUT` | – | Default per-request upstream timeout (default 120 s). |
| `UPSTREAM_ALLOW_HTTP` / `UPSTREAM_ALLOWED_PRIVATE_ADDRS` | – | Controlled exceptions to the anti-SSRF egress policy (e.g. local runtimes). Off/empty by default. |
| `ANTIGRAVITY_CLIENT_SECRET` | ⚠️ | **Required for the Antigravity/Google OAuth provider.** It is read from the environment only — no value is embedded in the binary. |
| `METRICS_TOKEN` / `METRICS_ALLOW_LOOPBACK` | – | Protect the Prometheus `/metrics` endpoint (token and/or loopback-only access). |
| `AUDIT_LOG_RETENTION_DAYS` / `REQUEST_LOG_RETENTION_DAYS` / `REQUEST_BODY_RETENTION_DAYS` | – | Retention windows (defaults 365 / 30 / 7 days). |
| `LOG_LEVEL`, `READ_TIMEOUT`, `WRITE_TIMEOUT`, `SHUTDOWN_GRACE`, `DB_MIN_CONNS`, `DB_MAX_CONNS`, `HEALTH_CHECK_INTERVAL`, `USAGE_ROLLUP_INTERVAL`, `MAX_REQUEST_MIB`, `RATE_LIMIT_FAIL_CLOSED` | – | Operational tuning — see `.env.example`. |

---

## Architecture

```mermaid
flowchart TD
    subgraph Clients["1. Ingestion Layer"]
        C1["OpenAI SDK / Python / TS"]
        C2["Claude Code CLI / Anthropic SDK"]
        C3["Developer IDEs (Cursor, Cline, Aider)"]
        C4["AI Agents & WebUI"]
    end

    subgraph Gateway["2. Route-X Gateway Core (Single Binary)"]
        subgraph Ingress["Security & Ingress"]
            I1["Anti-SSRF Dialer / CSRF Guard"]
            I2["Argon2id Passwords / API-Key Auth (HMAC-SHA256)"]
            I3["Dual-Protocol Parser (/v1/chat & /v1/messages)"]
        end

        subgraph Engine["Policy & Routing Pipeline"]
            R1["Distributed Rate Limiter (Redis Lua)"]
            R2["Budget Reservation & Cost Accounting"]
            R3["Rule Matching Engine (In-Memory)"]
            R4["Combo Router (Priority / Weighted / Latency / Cost)"]
            R5["3-State Circuit Breaker & Cascade Failover"]
        end

        subgraph Egress["Egress & Telemetry"]
            E1["Per-Credential Proxy / Egress Pools"]
            E2["SSE Streaming Normalizer & Transpiler"]
            E3["Monthly Partitioned Request Log"]
        end

        subgraph UI["Embedded Dashboard"]
            U1["React 18 + Tailwind 3.4 (go:embed)"]
            U2["Calm Console UI (Bahasa Indonesia)"]
        end
    end

    subgraph State["3. Storage & State Layer"]
        DB[("PostgreSQL 16\n(migrations 0001–0019, partitions & state)")]
        RD[("Redis 7\n(rate limit, breaker, response cache)")]
    end

    subgraph Upstreams["4. Upstream LLM Providers"]
        P1["Anthropic"]
        P2["OpenAI"]
        P3["Google / OpenRouter / Groq / DeepSeek / Mistral"]
        P4["Local vLLM / Ollama"]
    end

    Clients --> Ingress
    Ingress --> Engine
    Engine <--> State
    Engine --> Egress
    Egress --> Upstreams
    UI -.-> Ingress
```

Schema changes are append-only migrations (`internal/database/migrations/`, currently **0001–0019**, checksum-verified against the migration ledger, each applied in its own transaction). There are no down-migrations — schema rollback means restoring from backup (see the Disaster Recovery runbook).

---

## API Overview

Public inference surface (mounted under `/v1`):

| Endpoint | Notes |
|---|---|
| `POST /v1/chat/completions` | OpenAI Chat Completions, streaming & non-streaming. |
| `POST /v1/responses` | Compatibility **alias** of chat completions — not a full Responses API implementation. |
| `POST /v1/messages` | Anthropic Messages (alias: `POST /v1/v1/messages`). |
| `POST /v1/messages/count_tokens` | Token counting for Anthropic-dialect requests. |
| `POST /v1/embeddings` | OpenAI Embeddings. |
| `GET /v1/models`, `GET /v1/models/{model}` | Model catalog visible to the calling key. |

The full specification lives in [`docs/openapi.yaml`](docs/openapi.yaml) and is served interactively by every instance at **`/docs`**. Coverage note: the `/v1` and `/api/auth` surfaces are documented in full; the admin surface (`/api/admin`, 135 registered operations) is documented for its core operations — full per-operation admin coverage is being completed, and the admin route registrations in `internal/admin/` remain the source of truth in the meantime.

### Webhook signatures

Every webhook delivery carries:

```
X-RouteX-Signature: sha256=<HMAC-SHA256(secret, "<timestamp>.<deliveryID>.<final body>")>
```

Verify by recomputing the HMAC with your endpoint's secret over the exact timestamp, delivery ID, and raw request body. (The legacy `X-Hub-Signature-256` header was removed — receivers must migrate to the scheme above.)

### Practical examples

**Python (OpenAI SDK)**

```python
from openai import OpenAI

client = OpenAI(
    base_url="https://ai.yourdomain.com/v1",
    api_key="sk_live_your_routex_api_key",
)

stream = client.chat.completions.create(
    model="claude-sonnet-4.5",  # a model name or routing-rule alias
    messages=[
        {"role": "system", "content": "You are a helpful software architect."},
        {"role": "user", "content": "How does Route-X keep its routing overhead low?"},
    ],
    stream=True,
)
for chunk in stream:
    if chunk.choices[0].delta.content:
        print(chunk.choices[0].delta.content, end="", flush=True)
print()
```

**TypeScript (OpenAI SDK)**

```typescript
import OpenAI from "openai";

const client = new OpenAI({
  baseURL: "https://ai.yourdomain.com/v1",
  apiKey: process.env.ROUTEX_API_KEY,
});

const completion = await client.chat.completions.create({
  model: "combo-balanced", // routing-rule alias
  messages: [{ role: "user", content: "Summarize the benefits of Route-X." }],
});
console.log(completion.choices[0]?.message?.content);
```

**Operations CLI (headless key management)** — provision and list gateway API keys on a headless server without a browser:

```bash
make build-cli
export DATABASE_URL="postgres://routex:****@127.0.0.1:5432/routex?sslmode=disable"
export API_KEY_PEPPER="<must match the server's pepper>"

./bin/routex-apikey create --name "ci-agent" --live --scope inference
./bin/routex-apikey list   # masked hints only; raw secrets are never persisted
```

---

## Benchmarks & Performance

Historical micro-benchmark results measured on a dedicated **Intel Xeon Platinum 8259CL @ 2.50 GHz** (PR #78). These figures are a snapshot from that machine and that PR — they are **not** re-measured per release, vary by hardware, and describe individual micro-benchmarks, not end-to-end request latency. Reproduce them with `go test -bench . ./internal/router/ ./internal/ratelimit/ ./internal/gateway/`:

| Benchmark | Time / op | Memory / op | Allocations / op |
|---|:---:|:---:|:---:|
| `BenchmarkRule_Matches` | 23.05 ns | 0 B | 0 |
| `BenchmarkDeterministicTieBreaker` | 35.13 ns | 0 B | 0 |
| `BenchmarkMemoryLimiter_Allow` | 87.08 ns | 0 B | 0 |
| `BenchmarkMemoryLimiter_AllowParallel` | 118.90 ns | 0 B | 0 |
| `BenchmarkComboPipeline_Evaluate` | 132.90 ns | 32 B | 1 |
| `BenchmarkSelectCandidate_Priority` | 154.70 ns | 48 B | 1 |
| `BenchmarkSelectCandidate_RoundRobin` | 350.90 ns | 88 B | 4 |
| `BenchmarkSelectCandidate_LowestCost` | 579.70 ns | 48 B | 1 |
| `BenchmarkSelectCandidate_LowestLatency` | 598.00 ns | 48 B | 1 |
| `BenchmarkSelectCandidate_Weighted` | 1,139.00 ns | 48 B | 1 |

Stated precisely: the allocation-free micro-benchmarks (rule matching, tie-breaking, the in-memory limiter component) run in **under 150 ns**, and full candidate selection measured **~155 ns – 1,139 ns** depending on strategy in the same snapshot. The in-memory limiter above is a benchmarked component — the limiter that enforces limits on live traffic is the Redis Lua limiter, which additionally costs a Redis round-trip by design. In all cases the gateway's own decision overhead is small next to an upstream LLM round-trip.

---

## Development & Testing

Prerequisites: **Go 1.27.3**, **Node.js 24**, **PostgreSQL 16**, **Redis 7**.

```bash
make build        # web bundle + gateway binary + operations CLI into ./bin
make test         # Go test suite (integration tests use disposable test_* schemas
                  # and skip automatically when no test database is configured)

# Frontend
cd web && npm ci && npm run build && npm test

# The same gates CI enforces (jobs: go, web, docker)
gofmt -l .                       # must print nothing
go vet ./...
go test -race ./...              # race detector across all packages
./scripts/probe_e2e_gateway.sh   # live wire-protocol probe against a running gateway
./scripts/check_metrics.sh       # metrics endpoint sanity
```

The Playwright end-to-end suite lives in `web/e2e/` (desktop + mobile browser projects) and runs against a staging instance with seeded providers. Contributions follow the PR-only workflow described in [`CONTRIBUTING.md`](CONTRIBUTING.md).

---

## Security

Route-X stores paid upstream credentials, so its defaults are conservative. The essentials (full model in [`SECURITY.md`](SECURITY.md)):

- **API keys are never stored recoverably.** A gateway API key is stored only as an **HMAC-SHA256 hash keyed with the server pepper** (`API_KEY_PEPPER`) — lookup is by deterministic hash, and a database leak does not reveal usable keys without the pepper.
- **Provider credentials are encrypted at rest** with **AES-256-GCM** (`ENCRYPTION_KEY`, with `ENCRYPTION_KEY_PREVIOUS` as a rotation grace key). Passwords are hashed with **Argon2id**.
- **Sessions**: maximum 10 concurrent sessions per user, sliding TTL with a hard 7-day absolute cap, and an admin revoke-all switch. Admin mutations are CSRF-protected.
- **Egress anti-SSRF**: a custom dialer blocks DNS rebinding, RFC 1918 destinations, IPv6 loopback, and cloud metadata endpoints (`169.254.169.254`) unless you explicitly allow-list them.
- **Restore safety**: backup restores are scanned for psql meta-commands and applied via scratch-database swap (see Features).

To report a vulnerability, follow [`SECURITY.md`](SECURITY.md) — please do not open a public issue for security reports.

---

## Troubleshooting

| Symptom | Check |
|---|---|
| Instance won't start; migration error | Read the boot log — migrations are checksum-verified and applied in order (0001–0019). A checksum mismatch means a migration file was edited after being applied; restore it from git instead of editing the ledger. |
| `/healthz` vs `/readyz` | `GET /healthz` returns `{"status":"ok"}` when the process is up. `GET /readyz` additionally reports per-dependency `checks` (PostgreSQL, Redis) and is cached for 2 seconds. Neither returns version, commit, or uptime. |
| Dashboard loads but requests fail with 502/503 | Check `/readyz` for the failing dependency, then provider health on the Observability page; a fully open circuit breaker returns `503 upstream_overloaded` until its half-open probe succeeds. |
| Login returns 429 | The login limiter allows a limited number of attempts per IP per 15 minutes — wait for the window to slide instead of retrying in a loop. Locked accounts unlock automatically when `locked_until` passes; an admin can also unlock a user from the Users page. |
| Budget timezone seems wrong | Period boundaries are computed in `BUDGET_TIMEZONE` (default UTC). Changing it does not rewrite the `period_end` of budgets already in flight — the new zone applies from the next reset. |
| Antigravity provider errors about a missing secret | Set `ANTIGRAVITY_CLIENT_SECRET` in the server environment — since the audit3 hardening it is never embedded in the binary. |

More operational procedures live in the [Operations Runbook](docs/runbook-operasi.md) and [Disaster Recovery](docs/DISASTER_RECOVERY.md) documents.

---

## Documentation

- 📘 [**Quickstart Guide**](docs/QUICKSTART.md) — developer setup and client tutorials.
- 📙 [**Operations Runbook**](docs/runbook-operasi.md) — production operations, secret rotation, monitoring.
- 📕 [**Disaster Recovery**](docs/DISASTER_RECOVERY.md) — backup, restoration, and outage mitigation.
- 📗 [**OpenAPI Specification**](docs/openapi.yaml) — also served interactively at `/docs` on every instance.
- 🚀 [**Native Deploy Runbook**](docs/runbook-deploy.md) · [**Fly.io Guide**](docs/deploy-fly-io.md) · [**Render Guide**](docs/deploy-render.md)
- 🧪 [**Load & E2E Runbook**](docs/runbook-beban-e2e.md) — load-test and end-to-end staging procedures (Indonesian).
- 🤝 [**Contributing**](CONTRIBUTING.md) · 🔐 [**Security Policy**](SECURITY.md) · 📋 [**Changelog**](CHANGELOG.md)
- 🤖 Internal multi-agent governance notes live under [`.agents/`](.agents/) — they describe how this repository is maintained, not how to use Route-X.

---

## License

Route-X is open-source software released under the [MIT License](LICENSE).

Copyright © 2026 NexGen-X (Arjun Wijaya).
