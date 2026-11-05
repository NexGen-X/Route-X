<div align="center">

<img src="docs/images/logo.svg" alt="Route-X — Ubin Rute logo" width="88">

<h1>Route-X</h1>

<p><strong>One gateway for every AI provider.</strong></p>

<p>OpenAI- and Anthropic-compatible ingestion, multi-provider routing with failover, hard budget guardrails, and a full operations dashboard — shipped as a single Go binary.</p>

<p>
<a href="https://github.com/NexGen-X/Route-X/releases/tag/v1.7.2"><img src="https://img.shields.io/badge/Release-v1.7.2-10B981?style=for-the-badge&logo=github" alt="Release v1.7.2"></a>
<a href="https://go.dev"><img src="https://img.shields.io/badge/Go-1.27.3+-00ADD8?style=for-the-badge&logo=go" alt="Go 1.27.3+"></a>
<a href="https://react.dev"><img src="https://img.shields.io/badge/React-19-61DAFB?style=for-the-badge&logo=react" alt="React 19"></a>
<a href="https://postgresql.org"><img src="https://img.shields.io/badge/PostgreSQL-16-336791?style=for-the-badge&logo=postgresql" alt="PostgreSQL 16"></a>
<a href="https://redis.io"><img src="https://img.shields.io/badge/Redis-7-DC382D?style=for-the-badge&logo=redis" alt="Redis 7"></a>
<a href="https://github.com/NexGen-X/Route-X/actions/workflows/ci.yml"><img src="https://github.com/NexGen-X/Route-X/actions/workflows/ci.yml/badge.svg" alt="CI status"></a>
<a href="LICENSE"><img src="https://img.shields.io/badge/License-MIT-yellow?style=for-the-badge" alt="License: MIT"></a>
</p>

<p>
<a href="#quickstart"><strong>Quickstart</strong></a> ·
<a href="#features"><strong>Features</strong></a> ·
<a href="#architecture"><strong>Architecture</strong></a> ·
<a href="#api-overview"><strong>API</strong></a> ·
<a href="#installation"><strong>Deployment</strong></a> ·
<a href="#documentation"><strong>Docs</strong></a>
</p>

<img src="docs/images/hero.png" alt="Every client converges on one Route-X gateway, which routes each request out to the right AI provider">

</div>

> **Release status.** The latest release is **v1.7.2** — a fix release on top of v1.7.1. It fixes the Antigravity model catalog: the gateway now resolves the account's Code Assist project (via `loadCodeAssist`, stored per OAuth session, with lazy backfill so existing logins heal without reconnecting) and forwards it to model discovery and to the generate/stream envelopes, so project-gated models that the same account sees in the Antigravity app and agy — for example the model displayed there as "Argon" — can appear in the Route-X catalog too. Model sync also captures upstream display names, so catalog names are no longer raw model IDs. OpenAI Codex CLI remains honestly labelled temporarily unsupported pending Responses API streaming support. This repository is **private** by the owner's decision, so release assets require authenticated GitHub access to download. See [`CHANGELOG.md`](CHANGELOG.md) for the full release notes. Everything described in this README reflects the code in this repository.

> **Scope — read before deploying.** Route-X is an open-source project built for **personal use and small, trusted teams**. Its identity model is deliberately simple — there is no role-based access control and no multi-tenant isolation between dashboard users — but administrator rights are **not** automatic: a dashboard account is a full administrator only when it was explicitly created or promoted with the `admin` role, and accounts created without a role have no admin access at all. Do not expose a Route-X instance to untrusted users, and protect internet-facing instances like any other admin panel that stores paid upstream API credentials.

## Why Route-X?

Calling heterogeneous AI provider APIs directly means dialect fragmentation (OpenAI vs. Anthropic schemas), API-key sprawl across tools, unpredictable rate limits, silent provider outages, and untracked token spend. Route-X sits in front of all of it as a self-hosted reverse proxy and control plane:

<table>
<tr>
<td width="50%" valign="top">

**One endpoint for both dialects** — OpenAI Chat Completions and Anthropic Messages are ingested natively and transpiled in both directions, including streaming.

</td>
<td width="50%" valign="top">

**Routing that survives outages** — priority tiers, weighted / lowest-latency / lowest-cost / round-robin strategies, a 3-state circuit breaker, and cascade failover across providers.

</td>
</tr>
<tr>
<td width="50%" valign="top">

**Budgets that actually hold** — estimated cost is *reserved atomically before* a request runs, so a parallel burst cannot overshoot a limit the way after-the-fact accounting can.

</td>
<td width="50%" valign="top">

**A control plane, not just a proxy** — an embedded dashboard for providers, routing rules, API keys, budgets, webhooks, egress pools, backups, and observability, plus one-click synchronization of developer CLI tools — 10 of the 11 registered tools (OpenAI Codex CLI is registered and detected, but not yet configurable).

</td>
</tr>
</table>

The application tier is a **single static Go binary** (dashboard embedded via `//go:embed` — no Node.js runtime or separate static host). **PostgreSQL 16** and **Redis 7** are required as separate state services.

---

## Features

<table>
<tr>
<td width="50%" valign="top">

### Dual-protocol ingestion

- **OpenAI-compatible**: `/v1/chat/completions`, `/v1/embeddings`, `/v1/models`, `/v1/models/{model}` — a drop-in target for the official OpenAI SDKs, LangChain, LiteLLM, Cursor, Cline, Open WebUI, and similar tools. `/v1/responses` is a native **OpenAI Responses API** surface (non-streaming subset; streaming and stored responses are not supported yet, and chat-completions-shaped bodies sent to it are rejected).
- **Anthropic-native**: `/v1/messages` (plus the `/v1/v1/messages` alias) and `/v1/messages/count_tokens` — a drop-in target for the Anthropic SDK and Claude Code.
- **Bidirectional transpilation** of roles, content blocks, base64 vision payloads, and tool/function calling between the two dialects, with a resilient SSE streaming engine (UTF-8 boundary safety, immediate upstream cancellation on client disconnect).
- **Anthropic document blocks**: text documents and base64 `text/plain` documents are converted to canonical text at their block position and forwarded to any upstream; base64 **PDF** documents are passed through natively to Anthropic-kind upstreams only — if no Anthropic candidate is available the request is rejected with `400 unsupported_document_upstream` before any network call, never silently dropped. URL-sourced documents are rejected (SSRF). Maximum 10 MiB per document.

</td>
<td width="50%" valign="top">

### Routing, resiliency, and rate limiting

- **Combo routing strategies**: strict priority tiering, Efraimidis-Spirakis weighted distribution, lowest tracked P95 latency, lowest cost per token, and atomic round-robin. Candidates without latency/pricing measurements are ranked last, never treated as fastest or free.
- **3-state circuit breaker** (Closed → Open → Half-Open canary probes) with state shared through Redis, plus proactive context-overflow bypass to higher-context models.
- **Distributed rate limiting**: an atomic Redis Lua fixed-window limiter (check-all-windows, then increment-all) enforces RPM/TPM and concurrency limits on the live path, shared across instances.
- **Response cache (opt-in)**: identical non-streaming requests can be served from Redis. Cache keys are isolated **per API key** (domain-separated hash of the key identity + the full canonical request + surface + route fingerprint, so routing changes make old entries unreachable). Only successful, non-streaming responses **without tool calls** are cached in this phase; streaming is never cached. Enable it in **Settings → Performance & Cache**.

</td>
</tr>
<tr>
<td width="50%" valign="top">

### Money: budgets and cost accounting

- **Atomic budget reservation**: before a request runs, its estimated cost is held against every applicable budget (global, API key, provider, model) and settled against the real cost afterwards — a burst of parallel requests cannot spend past the cap.
- **Budget periods follow your calendar**: daily (rolling 24 h), weekly (Monday 00:00), and monthly (the 1st, 00:00) windows are computed in the operator timezone set by `BUDGET_TIMEZONE` (default UTC).
- **Provider-accurate pricing**: costs use the price effective at request time (8-decimal USD precision, no floats in the money path). Anthropic cache-creation tokens are priced from the reported breakdown — **1-hour cache writes at 2×** the input price, **5-minute writes and unattributed remainder at 1.25×**; without a breakdown the previous flat 1.25× behavior applies.

</td>
<td width="50%" valign="top">

### Control plane and integrations

- **Embedded dashboard** ("Calm Console" design language: Claude-inspired dark theme, flat KPI strips instead of stacked cards, full Bahasa Indonesia UI, responsive + PWA for mobile — no separate native app).
- **Webhooks with verifiable signatures**: deliveries are signed with HMAC-SHA256 over `"<timestamp>.<deliveryID>.<final body>"` and sent in the **`X-RouteX-Signature`** header (`sha256=<hex>`). A webhook secret is required (empty secrets are rejected), failed deliveries retry with backoff and terminate in an `abandoned` state instead of retrying forever, and cross-host redirects are refused.
- **Egress pools & optional relays**: per-credential HTTP/SOCKS5 proxy pools with periodic health probing; unhealthy pools are skipped while a healthy alternative exists (fail-open if none is healthy). Deno Deploy and Cloudflare relays are **optional** intermediaries for egress — relays created by Route-X provisioning carry a random per-installation token generated on your own server; the repository contains code only, never your settings or secrets.
- **Developer CLI synchronization**: one-click provisioning for 10 of the 11 registered terminal tools (Claude Code, OpenCode, Grok Build, Qwen Code, Crush, Hermes Agent, Cline, Aider, Goose, and GitHub Copilot CLI) — every supported tool is treated equally and configured the same way. OpenAI Codex CLI is also registered and detected, but it cannot be configured yet: Codex only speaks the Responses API in streaming mode, which the gateway does not support yet.
- **Backup & restore**: one-click database export (streaming) and restore. Restores are **scratch-swap atomic**: the upload is scanned for psql meta-commands, restored into a scratch database in a single transaction, checked against the schema ledger, and only then swapped in — the previous database is kept as `<db>_pre_restore`.
- **Observability**: request inspector with latency waterfall and event timeline, traffic breakdowns whose percentages are computed from true totals, Prometheus metrics, and `/healthz` + `/readyz` probes that return status only (`{"status":"ok"}` — no version or uptime leakage).

</td>
</tr>
<tr>
<td colspan="2" valign="top">

### Accessibility

The dashboard is **designed toward WCAG 2.1 Level AA**: focus trapping in dialogs and the command palette, WAI-ARIA combobox semantics, labelled form controls, and internal axe-core audits reporting zero color-contrast violations across dashboard pages. It is a design target verified by internal checks — Route-X has **not** undergone external WCAG certification.

</td>
</tr>
</table>

---

## Dashboard Preview

Every screenshot below was captured from a live install built from the current `main` branch; the numbers shown are that test gateway's real recorded traffic — Route-X never renders placeholder or mock data.

### Desktop

<table>
<tr>
<td width="50%" valign="top">

**Dashboard** — one plain status line, a flat KPI strip (requests, tokens & cost, P95 latency), a live request feed, and provider health as simple rows.

![Route-X Dashboard — desktop](docs/images/dashboard-desktop.png)

</td>
<td width="50%" valign="top">

**Providers** — every upstream as a two-line row: status dot, model count, priority and weight. Provider presets (OpenAI, Anthropic, Google, OpenRouter, local runtimes, and more) are added with a single click, then mapped to models with per-provider pricing.

![Route-X Providers — desktop](docs/images/penyedia-desktop.png)

</td>
</tr>
<tr>
<td width="50%" valign="top">

**Observability** — underline metric tabs (Requests, Throughput, Tokens, Error Rate, Cost, P95 Latency) over one large chart, traffic composition broken down by provider / model / API key, and server & pool health as plain rows.

![Route-X Observability — desktop](docs/images/observabilitas-desktop.png)

</td>
<td width="50%" valign="top">

**Command Palette (`Ctrl+K` / `⌘K`)** — a Raycast-style palette for the whole console: results grouped by category (Navigation, Quick Actions, CLI Tools), full keyboard navigation.

![Route-X Command Palette — desktop](docs/images/palet-perintah-desktop.png)

</td>
</tr>
<tr>
<td width="50%" valign="top">

**API Keys** — key rows with masked secret, RPM/TPM limits, last-used time, and model/provider scope. When a key has a budget registered in the Budgets system, the row also shows a thin **budget bar with real recorded spend versus its limit** (amber at the alert threshold, red at 100%) — keys without a budget show no bar at all.

![Route-X API Keys — desktop](docs/images/kunci-api-desktop.png)

</td>
<td width="50%" valign="top">

**Settings** — Linear-style left sub-navigation with panel-free content: label–description–control rows for Domain & HTTPS (with DNS guidance and automatic Let's Encrypt), Performance & Cache, Backup & Recovery, and free-form Advanced runtime parameters.

![Route-X Settings — desktop](docs/images/pengaturan-desktop.png)

</td>
</tr>
</table>

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
curl -fsSLO https://github.com/NexGen-X/Route-X/releases/download/v1.7.2/routex-v1.7.2-linux-amd64.tar.gz

# 2. Extract and run (migrations apply automatically on boot)
tar -xzvf routex-v1.7.2-linux-amd64.tar.gz
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

<details>
<summary><strong>Environment variables — the core set</strong></summary>

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
| `ANTIGRAVITY_CLIENT_SECRET` | – | Optional override for the Antigravity/Google OAuth provider. The binary embeds the public desktop-app client value as an automatic fallback (non-secret by Google's design for installed apps); set this only to override the embedded value. |
| `METRICS_TOKEN` / `METRICS_ALLOW_LOOPBACK` | – | Protect the Prometheus `/metrics` endpoint (token and/or loopback-only access). |
| `AUDIT_LOG_RETENTION_DAYS` / `REQUEST_LOG_RETENTION_DAYS` / `REQUEST_BODY_RETENTION_DAYS` | – | Retention windows (defaults 365 / 30 / 7 days). |
| `LOG_LEVEL`, `READ_TIMEOUT`, `WRITE_TIMEOUT`, `SHUTDOWN_GRACE`, `DB_MIN_CONNS`, `DB_MAX_CONNS`, `HEALTH_CHECK_INTERVAL`, `USAGE_ROLLUP_INTERVAL`, `MAX_REQUEST_MIB`, `RATE_LIMIT_FAIL_CLOSED` | – | Operational tuning — see `.env.example`. |

</details>

---

## Architecture

```mermaid
flowchart LR
    subgraph Clients["Clients"]
        C1["OpenAI SDK & compatible tools"]
        C2["Anthropic SDK & Claude Code"]
        C3["IDEs, CLI tools & AI agents"]
    end

    subgraph RouteX["Route-X — single Go binary"]
        IN["Dual-protocol ingestion<br/>/v1/chat/completions · /v1/messages<br/>bidirectional transpilation & SSE streaming"]
        ENG["Routing & policy engine<br/>Redis rate limiter · atomic budget reservation<br/>combo routing · 3-state circuit breaker · cascade failover"]
        DASH["Embedded dashboard<br/>Calm Console — React 19 + Tailwind 3.4 (go:embed)"]
        IN --> ENG
    end

    subgraph State["State services"]
        DB[("PostgreSQL 16")]
        RD[("Redis 7<br/>rate limit · breaker state · response cache")]
    end

    subgraph Upstreams["Upstream providers"]
        P1["Anthropic"]
        P2["OpenAI"]
        P3["Google / OpenRouter / Groq / DeepSeek / Mistral"]
        P4["Local vLLM / Ollama"]
    end

    Clients --> IN
    ENG <--> State
    ENG --> Upstreams
```

Schema changes are append-only migrations (`internal/database/migrations/`, currently **0001–0019**, checksum-verified against the migration ledger, each applied in its own transaction). There are no down-migrations — schema rollback means restoring from backup (see the Disaster Recovery runbook).

---

## API Overview

Public inference surface (mounted under `/v1`):

| Endpoint | Notes |
|---|---|
| `POST /v1/chat/completions` | OpenAI Chat Completions, streaming & non-streaming. |
| `POST /v1/responses` | OpenAI Responses API (native, non-streaming subset) — not a chat-completions alias. |
| `POST /v1/messages` | Anthropic Messages (alias: `POST /v1/v1/messages`). |
| `POST /v1/messages/count_tokens` | Token counting for Anthropic-dialect requests. |
| `POST /v1/embeddings` | OpenAI Embeddings. |
| `GET /v1/models`, `GET /v1/models/{model}` | Model catalog visible to the calling key. |

The full specification lives in [`docs/openapi.yaml`](docs/openapi.yaml) and is served interactively by every instance at **`/docs`**. Coverage note: the specification documents **158 operations** across all surfaces — the `/v1` and `/api/auth` surfaces in full, and the admin surface (`/api/admin`) completely: all **138 of 138** registered admin operations are documented per operation, extracted from and cross-checked against the route registrations in `internal/admin/`.

### Webhook signatures

Every webhook delivery carries:

```
X-RouteX-Signature: sha256=<HMAC-SHA256(secret, "<timestamp>.<deliveryID>.<final body>")>
```

Verify by recomputing the HMAC with your endpoint's secret over the exact timestamp, delivery ID, and raw request body. (The legacy `X-Hub-Signature-256` header was removed — receivers must migrate to the scheme above.)

### Practical examples

<details>
<summary><strong>Python · TypeScript · operations CLI — click to expand</strong></summary>

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

</details>

---

## Benchmarks & Performance

<details>
<summary><strong>Historical micro-benchmark snapshot (PR #78) — click to expand</strong></summary>

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

</details>

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

<details>
<summary><strong>Common symptoms &amp; checks — click to expand</strong></summary>

| Symptom | Check |
|---|---|
| Instance won't start; migration error | Read the boot log — migrations are checksum-verified and applied in order (0001–0019). A checksum mismatch means a migration file was edited after being applied; restore it from git instead of editing the ledger. |
| `/healthz` vs `/readyz` | `GET /healthz` returns `{"status":"ok"}` when the process is up. `GET /readyz` additionally reports per-dependency `checks` (PostgreSQL, Redis) and is cached for 2 seconds. Neither returns version, commit, or uptime. |
| Dashboard loads but requests fail with 502/503 | Check `/readyz` for the failing dependency, then provider health on the Observability page; a fully open circuit breaker returns `503 upstream_overloaded` until its half-open probe succeeds. |
| Login returns 429 | The login limiter allows a limited number of attempts per IP per 15 minutes — wait for the window to slide instead of retrying in a loop. Locked accounts unlock automatically when `locked_until` passes; an admin can also unlock a user from the Users page. |
| Budget timezone seems wrong | Period boundaries are computed in `BUDGET_TIMEZONE` (default UTC). Changing it does not rewrite the `period_end` of budgets already in flight — the new zone applies from the next reset. |
| Antigravity OAuth rejected by Google with an invalid-client error | The embedded fallback client value may have been rotated by Google — set `ANTIGRAVITY_CLIENT_SECRET` in the server environment to override it. |

</details>

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
