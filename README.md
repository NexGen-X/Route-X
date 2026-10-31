# 🚀 Route-X — Next-Generation AI Gateway

<div align="center">

<p align="center">
  <strong>A self-hosted, single-admin AI Gateway for personal use and small trusted teams — with native dual-protocol ingestion (OpenAI & Anthropic), intelligent multi-model combo routing, fast in-memory routing decisions, precise budget accounting, and instant developer CLI synchronization.</strong>
</p>

[![Release](https://img.shields.io/badge/Release-v1.4.0-10B981?style=for-the-badge&logo=github)](https://github.com/NexGen-X/Route-X/releases/tag/v1.4.0)
[![Go Version](https://img.shields.io/badge/Go-1.27.1+-00ADD8?style=for-the-badge&logo=go)](https://go.dev)
[![React](https://img.shields.io/badge/React-18-61DAFB?style=for-the-badge&logo=react)](https://react.dev)
[![TailwindCSS](https://img.shields.io/badge/Tailwind-3.4-38B2AC?style=for-the-badge&logo=tailwind-css)](https://tailwindcss.com)
[![PostgreSQL](https://img.shields.io/badge/PostgreSQL-16-336791?style=for-the-badge&logo=postgresql)](https://postgresql.org)
[![Redis](https://img.shields.io/badge/Redis-7-DC382D?style=for-the-badge&logo=redis)](https://redis.io)
[![CI](https://github.com/NexGen-X/Route-X/actions/workflows/ci.yml/badge.svg)](https://github.com/NexGen-X/Route-X/actions/workflows/ci.yml)
[![A11y WCAG 2.1 AA](https://img.shields.io/badge/A11y-WCAG%202.1%20AA%20Design%20Target-blueviolet?style=for-the-badge&logo=w3c)](https://www.w3.org/WAI/standards-guidelines/wcag/)

<p align="center">
  <a href="#-why-route-x"><strong>Why Route-X</strong></a> •
  <a href="#-core-pillars"><strong>Core Pillars</strong></a> •
  <a href="#%EF%B8%8F-architecture"><strong>Architecture</strong></a> •
  <a href="#-benchmarks--performance"><strong>Benchmarks</strong></a> •
  <a href="#-quickstart"><strong>Quickstart</strong></a> •
  <a href="#-developer-cli-ecosystem"><strong>Developer CLIs</strong></a> •
  <a href="#-practical-code-examples"><strong>Code Examples</strong></a> •
  <a href="#-multi-agent-architecture--governance"><strong>Multi-Agent Governance</strong></a> •
  <a href="#-documentation"><strong>Documentation</strong></a>
</p>

---

</div>

## 💡 Why Route-X?

Direct integration with heterogeneous AI model APIs leads to severe operational vulnerabilities: dialect fragmentation, API key sprawl, unpredictable rate limits, silent provider outages, and untracked token expenditures. 

**Route-X** provides a self-hosted reverse proxy and control plane that abstracts upstream providers behind unified interfaces, enforces budget guardrails, executes fast in-memory routing decisions, and coordinates developer terminal tools — shipped as a **single Go binary for the application tier** (dashboard embedded, no Node.js or separate static host) with **PostgreSQL 16 and Redis 7 as required external state services**.

> **Scope — please read before deploying.** Route-X is built and maintained as an **open-source project for personal use and small, trusted teams**. Its identity model is **single-admin**: every authenticated dashboard user has full administrator rights (`*`) — there is no role-based access control and no multi-tenant isolation between dashboard users. Do not expose a Route-X instance to untrusted users, and protect internet-facing instances like any other admin panel that stores paid upstream API credentials.

| Pain Point | Route-X Solution |
|---|---|
| **Dialect Incompatibility**: Tools like Claude Code expect Anthropic Messages schema, while standard gateways only ingest OpenAI. | **Native Dual-Protocol Engine**: Transpilation between `/v1/chat/completions` and `/v1/messages` with bidirectional streaming. |
| **Upstream Provider Outages**: Silent model downtime interrupts critical inference pipelines. | **Cascading Failover & 3-State Circuit Breaker**: Automatic failover cascades, half-open canary probes, and context-window bypass to higher-context models. |
| **Hot-Path Proxy Latency Overhead**: Heavy gateways introduce 10–50ms proxy latency per hop. | **Fast In-Memory Routing**: Rule matching measured at ~23ns in a single-rule micro-benchmark (see Benchmarks — full candidate selection costs more), with distributed rate limiting enforced by atomic Redis Lua scripts. |
| **Inaccessible Internal Tools**: Dashboard features that keyboard and screen-reader users cannot operate. | **WCAG 2.1 Level AA Design Target**: WAI-ARIA comboboxes, FocusTrap modal containment, labelled form controls, and screen-reader readiness. The UI is designed toward AA; it is not externally certified. |
| **Developer Onboarding Friction**: Manual terminal configuration and dotfile management across 6+ CLI tools. | **Automated CLI Synchronization**: 1-click declarative provisioning for Claude Code, OpenCode, Aider, Shell-GPT, Fabric, and OpenCommit. |
| **Runaway AI Operational Costs**: Surprise billing spikes when API keys or automated agents loop indefinitely. | **Precise Budget Accounting**: Cost accounting at `$0.00000001` (10⁻⁸ USD) integer precision — no floating point in the money path — with budget checks against periodically refreshed spend state. |
| **Fragile Multi-Container Deployments**: Requiring Node.js runtime, Nginx sidecars, and separate static asset hosts for the app tier. | **Single Static Executable for the App Tier**: React 18 dashboard embedded directly into the Go binary via `//go:embed`. PostgreSQL and Redis still run as their own services. |

---

## 🌟 Core Pillars

### 🌐 1. Dual-Protocol Native Ingestion
- **OpenAI Standard Compatibility**: Drop-in target for official OpenAI Python/TypeScript SDKs, LangChain, LlamaIndex, LiteLLM, Cursor IDE, Cline, and Open WebUI via `/v1/chat/completions`. (`/v1/responses` is also routed, as a compatibility alias of the same chat-completions handler — it is not a separate, full implementation of the OpenAI Responses API.)
- **Native Anthropic Messages Ingestion**: Drop-in target for Anthropic SDK and **Claude Code CLI** via `/v1/messages` and `/v1/v1/messages`.
- **Bidirectional Dialect Transpiler**: Converts roles, arrays, base64 vision payloads, and structured function/tool calling between OpenAI and Anthropic specifications on the fly.
- **Resilient SSE Streaming Engine**: Zero-buffering event streaming pipeline with UTF-8 multibyte boundary split safety and immediate upstream cancellation upon client disconnect.

### ⚡ 2. Fast In-Memory Routing Decisions
Route-X's routing hot-path is engineered to add as little overhead as possible to the upstream LLM round-trip:
- **Allocation-Free Rule Matching (micro-benchmark)**: Matching a single rule against model IDs, API keys, and capability constraints measures **~23 nanoseconds with 0 heap allocations**. Real requests evaluate the full rule set and candidate ordering on top of this, so end-to-end routing costs more — see the Benchmarks section for the complete, honestly-scoped table.
- **Deterministic FNV-1a Tie-Breaker**: 64-bit hash jitter reduces load skew on balanced weights in **~35 nanoseconds** in its micro-benchmark, without pseudo-random locking.
- **Distributed Rate Limiting in Production**: The limiter that actually enforces limits on live traffic is an **atomic Redis Lua fixed-window limiter** (check-all-windows, then increment-all), shared across instances — not the in-memory token bucket, which exists as a benchmarked component (`~87ns` single-goroutine) and is not wired into the production request path.

### 🔀 3. Intelligent Multi-Model Combo Routing & Resiliency
- **Sophisticated Balancing Strategies**:
  - **Priority Tiering**: Strict deterministic failover across upstream tiers.
  - **Weighted Distribution**: Efraimidis-Spirakis weighted ordering with FNV-1a tie-breaking, applied across the whole failover order — not just the first pick.
  - **Lowest Latency**: Dynamic routing based on tracked P95 response times; candidates without measurements are ranked last rather than treated as fastest.
  - **Lowest Cost**: Dynamic routing to the lowest cost-per-token provider matching requirements; candidates without pricing are ranked last rather than treated as free.
  - **Round Robin**: Atomic round-robin across healthy endpoints.
- **Proactive Context Overflow Bypass**: Upgrades requests to higher-context upstream models when prompt length exceeds the primary provider's limit.
- **3-State Circuit Breaker**: Health transitions (`Closed` ➔ `Open` ➔ `Half-Open` canary probes) tracked atomically in Redis Lua, shared across instances, to protect gateway throughput.

### ♿ 4. Accessibility (Designed Toward WCAG 2.1 Level AA)
- **Focus Management**: Focus trapping (`FocusTrap`) on Command Palette (`Cmd+K`), Dialogs, and Mobile Drawers, with Escape handling and body scroll-lock.
- **WAI-ARIA Combobox Specification**: Searchable Command Palette with `role="dialog"`, `role="combobox"`, `role="listbox"`, and `role="option"`.
- **Form Association & Screen Reader Support**: Form controls associated with explicit `<label htmlFor>` or `aria-label`, semantic error alerts (`role="alert"`), and `aria-label` standardization on icon-only triggers. Accessibility is a design target verified by internal checks — Route-X has not undergone external WCAG certification, and some areas (for example muted-text contrast at small sizes) are still being improved.
- **Decorative SVG Hardening**: `aria-hidden="true"` applied across all decorative Lucide SVG icons to eliminate screen-reader noise.

### 💻 5. Universal Developer CLI Synchronization
- **One-Click Provisioning Hub**:
  - **Claude Code CLI** (`~/.claude/settings.json`, base URL path normalization)
  - **OpenCode CLI** (`~/.config/opencode/opencode.json`)
  - **Aider** (`~/.aider.conf.yml`)
  - **Shell-GPT / SGPT** (`~/.config/shell_gpt/.sgptrc`)
  - **Fabric** (`~/.config/fabric/.env`)
  - **OpenCommit** (`~/.opencommit`)
- **Interactive Latency Diagnostics**: Execute live round-trip latency probes directly from the Web Console.
- **Strict Antigravity CLI Protection**: Guaranteed isolation ensuring Antigravity CLI (`agy`) and `~/.gemini` remain completely untouched and protected.

### 🛡️ 6. Financial & Operational Guardrails
- **Micro-Budget Precision**: Cost accounting with 8-decimal precision (`$0.00000001` USD) across budget scopes (global, API key, provider, and model). Budget checks read periodically refreshed spend state and spend is recorded after each request completes, so enforcement is near-real-time rather than a hard pre-authorization hold: a burst of parallel requests can overshoot a limit before the recorded spend catches up.
- **AES-256-GCM Credential Vault**: Provider credentials and API keys encrypted at rest using AES-256-GCM with distinct nonces and AEAD verification.
- **Transport Anti-SSRF Dialer**: Custom network dialer strictly blocks DNS rebinding, internal RFC 1918 networks, IPv6 loopbacks, and cloud metadata endpoints (`169.254.169.254`).
- **Argon2id Authentication**: Cryptographically sound password hashing with constant-time verification preventing timing side-channel attacks.

### 📦 7. Single Binary for the Application Tier
- **Zero Web Server Dependencies**: React 18 production bundle is embedded directly into the Go executable via Go's native `//go:embed` directive.
- **No App-Tier Runtime Dependencies**: No Node.js runtime, Nginx, or external static servers required to serve the gateway and dashboard. **PostgreSQL 16 and Redis 7 are still required** as separate state services — "single binary" describes the application tier, not the whole stack.

---

## 🏗️ Architecture

The following diagram illustrates how Route-X ingests client traffic, enforces security and rate policies, evaluates routing rules, and dispatches to upstream providers:

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
            I1["Anti-SSRF / Anti-CSRF Guard"]
            I2["Argon2id / API Key Auth (AES-256-GCM)"]
            I3["Native Dual-Protocol Parser (/v1/chat & /v1/messages)"]
        end

        subgraph Engine["Policy & Routing Pipeline"]
            R1["Distributed Rate Limiter (Redis Lua)"]
            R2["Micro-Budget Accounting ($0.00000001)"]
            R3["Rule Matching Engine (In-Memory)"]
            R4["Combo Router (Priority / Weighted / EMA / Cost)"]
            R5["3-State Circuit Breaker & Cascade Failover"]
        end

        subgraph Egress["Egress & Telemetry"]
            E1["Per-Credential SOCKS5 / HTTP Proxy Pool"]
            E2["SSE Streaming Normalizer & Transpiler"]
            E3["Monthly Partitioned Logger"]
        end
        
        subgraph UI["Embedded Dashboard"]
            U1["React 18 + Tailwind 3.4 (go:embed)"]
            U2["WCAG 2.1 AA-Oriented UI"]
        end
    end

    subgraph State["3. Storage & State Layer"]
        DB[("PostgreSQL 16\n(Partitions & State)")]
        RD[("Redis 7\n(Rate Limit & Breaker State)")]
    end

    subgraph Upstreams["4. Upstream LLM Providers"]
        P1["Anthropic (Claude 3.5 / 3.7)"]
        P2["OpenAI (GPT-4o / o1 / o3)"]
        P3["Groq / DeepSeek / Mistral"]
        P4["Local vLLM / Ollama Cluster"]
    end

    Clients --> Ingress
    Ingress --> Engine
    Engine <--> State
    Engine --> Egress
    Egress --> Upstreams
    UI -.-> Ingress
```

---

## ⚡ Benchmarks & Performance

Historical benchmark results measured on a dedicated **Intel(R) Xeon(R) Platinum 8259CL CPU @ 2.50GHz** (PR #78). These figures are a snapshot from that machine and that PR — they are **not** re-measured per release, results vary by hardware, and they describe individual micro-benchmarks, not end-to-end request latency. Reproduce them yourself with `go test -bench . ./internal/router/ ./internal/ratelimit/ ./internal/gateway/`:

| Benchmark Test | Operations / sec | Time / op | Memory / op | Allocations / op |
|---|:---:|:---:|:---:|:---:|
| **`BenchmarkRule_Matches`** | **~43,400,000 ops/s** | **23.05 ns/op** | **0 B/op** | **0 allocs/op** |
| **`BenchmarkDeterministicTieBreaker`** | **~28,400,000 ops/s** | **35.13 ns/op** | **0 B/op** | **0 allocs/op** |
| **`BenchmarkMemoryLimiter_Allow`** | **~11,500,000 ops/s** | **87.08 ns/op** | **0 B/op** | **0 allocs/op** |
| **`BenchmarkMemoryLimiter_AllowParallel`** | **~8,400,000 ops/s** | **118.90 ns/op** | **0 B/op** | **0 allocs/op** |
| **`BenchmarkComboPipeline_Evaluate`** | **~7,500,000 ops/s** | **132.90 ns/op** | 32 B/op | 1 allocs/op |
| **`BenchmarkSelectCandidate_Priority`** | **~6,500,000 ops/s** | **154.70 ns/op** | 48 B/op | 1 allocs/op |
| **`BenchmarkFormatKey`** | **~14,300,000 ops/s** | **69.80 ns/op** | 16 B/op | 1 allocs/op |
| **`BenchmarkSelectCandidate_RoundRobin`** | ~2,850,000 ops/s | 350.90 ns/op | 88 B/op | 4 allocs/op |
| **`BenchmarkSelectCandidate_LowestLatency`** | ~1,670,000 ops/s | 598.00 ns/op | 48 B/op | 1 allocs/op |
| **`BenchmarkSelectCandidate_LowestCost`** | ~1,720,000 ops/s | 579.70 ns/op | 48 B/op | 1 allocs/op |
| **`BenchmarkSelectCandidate_Weighted`** | ~880,000 ops/s | 1,139.00 ns/op | 48 B/op | 1 allocs/op |

> **Takeaway, stated precisely**: the allocation-free micro-benchmarks (rule matching, tie-breaking, in-memory limiter component) run in **under 150 nanoseconds**, while full candidate selection measured **~155ns–1,139ns** depending on strategy in the same snapshot. Production rate limiting additionally costs a Redis round-trip by design, because its state is distributed. In all cases the gateway's own decision overhead is small next to an upstream LLM round-trip — but "sub-microsecond, zero-allocation" only ever described the micro-benchmarks, not the whole request path.

---

## ⚡ Quickstart

### Option A: Pre-Built Release Binary (Fastest)
Download and run the official static Linux binary with zero build steps:

```bash
# 1. Download official v1.4.0 release
curl -fsSLO https://github.com/NexGen-X/Route-X/releases/download/v1.4.0/routex-v1.4.0-linux-amd64.tar.gz

# 2. Extract archive
tar -xzvf routex-v1.4.0-linux-amd64.tar.gz

# 3. Apply schema migrations & run gateway
./ai-gateway -migrate
./ai-gateway
```

### Option B: Automated Native Linux Installer (Systemd)
Installs Route-X Core, PostgreSQL 16, Redis 7, and Caddy Reverse Proxy with Auto-TLS via systemd:

```bash
curl -fsSL https://raw.githubusercontent.com/NexGen-X/Route-X/main/install-native.sh | sudo bash
```

> Review the script before running it, as with any `curl | sudo bash` installer, and verify the installed version afterwards with `./ai-gateway -version` — the installer pins its own release tag, which may lag the newest GitHub release.

#### 🔄 Live Updates with Automatic Rollback
When updating an active instance:
```bash
git pull origin main
make live-update
```
*Builds the new binary in isolation, applies migrations before the swap, swaps the binary atomically, probes `/healthz`, and automatically rolls back to the previous binary if the health check fails. The service restarts during the swap, so this is a brief-restart update with a safety net — not a guaranteed zero-downtime or sub-second cutover, and in-flight streaming connections are not promised to survive it.*

### Option C: Docker Stack Deployment
Deploy the complete containerized stack in isolated containers:

```bash
git clone https://github.com/NexGen-X/Route-X.git
cd Route-X
bash deploy.sh
```

### Option D: Build from Source
Requirements: **Go 1.27.1+**, **Node.js 20+**, **PostgreSQL 16**, **Redis 7**.

```bash
# 1. Clone repository
git clone https://github.com/NexGen-X/Route-X.git
cd Route-X

# 2. Build full package (frontend, backend, operations cli)
make build

# 3. Migrate & run
./bin/ai-gateway -migrate
./bin/ai-gateway
```

---

## 🔐 First-Run Onboarding

**There is no default password.** Earlier documentation listed a published default credential — that value must be considered burned and must never be used. The current code works as follows:

1. **Set the initial admin password before the first run.** The seed creates the first admin **only** when `INITIAL_ADMIN_PASSWORD` is set (minimum 16 characters in production). If it is empty, admin creation is *skipped* — no account with a known password is ever created.
   ```bash
   export INITIAL_ADMIN_EMAIL="admin@routex.local"   # optional; this is the default email
   export INITIAL_ADMIN_PASSWORD="$(openssl rand -base64 24)"  # generate your own; store it safely
   ```
2. Navigate to `http://localhost:8080/login` (or your domain `https://your-domain.com/login`) and sign in with that email and the password you just set.
- *💡 The setup-hint card on the login page can auto-fill the admin email — the password itself is never sent by that API.*
- *🔒 The initial admin is created with `MustChangePassword`, so changing the password is mandatory at first login. Once changed, the setup-hint card is permanently disabled.*
- *⚠️ Instances originally seeded by older versions with the previously published default password must change it immediately (or delete and re-seed the admin) — anyone who has read this repository knows that value.*

---

## 💻 Developer CLI Ecosystem

Route-X natively synchronizes terminal environments for zero-friction AI developer workflows:

```bash
# Claude Code CLI (Ingested via Anthropic /v1/messages)
claude -p "Explain the Route-X architecture in one sentence."

# OpenCode CLI (Ingested via OpenAI /v1/chat/completions)
opencode run "Analyze current performance bottlenecks."

# Aider CLI (Pair programming)
aider --model routex/claude-3-7-sonnet
```

### Supported CLI Integrations Matrix

| Developer Tool | Protocol Used | Config File Target | Verification Status |
|---|---|---|:---:|
| **Claude Code** | Anthropic Native (`/v1/messages`) | `~/.claude/settings.json` | ✅ Verified (Exit Code 0) |
| **OpenCode** | OpenAI Compatible (`/v1/chat/completions`) | `~/.config/opencode/opencode.json` | ✅ Verified (Exit Code 0) |
| **Aider** | OpenAI / Anthropic Hybrid | `~/.aider.conf.yml` | ✅ Verified |
| **Shell-GPT (SGPT)** | OpenAI Compatible | `~/.config/shell_gpt/.sgptrc` | ✅ Verified |
| **Fabric** | OpenAI Compatible | `~/.config/fabric/.env` | ✅ Verified |
| **OpenCommit** | OpenAI Compatible | `~/.opencommit` | ✅ Verified |
| **Antigravity CLI** | *Protected Native* | *Isolated / Untouched* | 🛡️ Locked (Zero-Touch) |

---

## 💻 Practical Code Examples

### 1. Python (Official OpenAI SDK)
```python
from openai import OpenAI

client = OpenAI(
    base_url="https://ai.yourdomain.com/v1",
    api_key="sk_live_your_routex_api_key",
)

# Stream response with automatic combo routing & failover
stream = client.chat.completions.create(
    model="claude-3-7-sonnet", # Aliased combo rule
    messages=[
        {"role": "system", "content": "You are a helpful software architect."},
        {"role": "user", "content": "How does Route-X keep its routing overhead low?"}
    ],
    stream=True,
)

for chunk in stream:
    if chunk.choices[0].delta.content:
        print(chunk.choices[0].delta.content, end="", flush=True)
print()
```

### 2. TypeScript / Node.js (OpenAI SDK)
```typescript
import OpenAI from "openai";

const client = new OpenAI({
  baseURL: "https://ai.yourdomain.com/v1",
  apiKey: process.env.ROUTEX_API_KEY || "sk_live_your_routex_api_key",
});

async function main() {
  const completion = await client.chat.completions.create({
    model: "combo-balanced",
    messages: [{ role: "user", content: "Summarize the benefits of Route-X." }],
  });

  console.log(completion.choices[0]?.message?.content);
}

main().catch(console.error);
```

### 3. cURL (Anthropic Messages API Dialect)
```bash
curl -X POST https://ai.yourdomain.com/v1/messages \
  -H "x-api-key: sk_live_your_routex_api_key" \
  -H "anthropic-version: 2023-06-01" \
  -H "content-type: application/json" \
  -d '{
    "model": "claude-3-5-sonnet",
    "max_tokens": 1024,
    "messages": [
      {"role": "user", "content": "Hello Route-X from the Anthropic native dialect!"}
    ]
  }'
```

---

## 🧰 Operations CLI (Headless Key Management)

Generate and manage API keys directly on headless servers without opening a web browser:

```bash
# Build operations utility
make build-cli

# Export configuration
export DATABASE_URL="postgres://routex:****@127.0.0.1:5432/routex?sslmode=disable"
export API_KEY_PEPPER="$(openssl rand -base64 32)"   # Must match Route-X server pepper

# Provision a production inference key
./bin/routex-apikey create --name "ci-agent" --live --scope inference

# List active keys (masked hints only; raw secrets are never persisted)
./bin/routex-apikey list
```

---

## 🤖 Multi-Agent Architecture & Governance

Route-X is built and maintained under a rigorous **Multi-Agent Orchestration Protocol** comprising **13 specialized sub-agents** operating across bounded context tiers:

```
┌────────────────────────────────────────────────────────────────────────┐
│                   LEAD ORCHESTRATOR / ARCHITECT                        │
├────────────────────┬────────────────────┬──────────────────────────────┤
│ Core Engineering   │ Quality & Security │ Delivery & Operations        │
├────────────────────┼────────────────────┼──────────────────────────────┤
│ • Backend Go Dev   │ • QA Automation    │ • Release & Delivery Eng.    │
│ • Frontend TS/React│ • Security Auditing│ • Database Administrator     │
│ • Protocol Router  │ • A11y Specialist  │ • Site Reliability Engineer  │
│ • Egress Specialist│ • Docs & Technical │ • Operations Provisioner     │
└────────────────────┴────────────────────┴──────────────────────────────┘
```

### The 7-Layer QA Verification Protocol
Pull requests and releases are validated through 7 gates (live status: the CI badge at the top of this README — this document intentionally does not hard-code test counts, which go stale):
1. **Layer 1 (TypeScript Strictness)**: `npx tsc -b` passes with **0 type errors**; the web source currently contains **0 `any` bypasses**.
2. **Layer 2 (Frontend Unit Testing)**: The Vitest suite (200+ cases and growing) must pass in CI.
3. **Layer 3 (Asset Compilation)**: The Vite production bundle builds cleanly.
4. **Layer 4 (Go Concurrency & Race Detection)**: CI runs `go test -race ./...` across all packages; the Go suite contains 1,100+ test functions, including database-backed integration tests that require PostgreSQL (and skip without it locally).
5. **Layer 5 (Go Formatting & Vetting)**: `gofmt -l .` reports no defects and `go vet ./...` passes; CI also runs `govulncheck`.
6. **Layer 6 (SQL Schema Integrity)**: All **16 PostgreSQL migrations (0001–0016)** are checksum-verified against the migration ledger and each applies inside its own transaction. There are no down-migrations — schema rollback means restoring from backup, as described in the Disaster Recovery runbook.
7. **Layer 7 (End-to-End Synthetic Validation)**: The Playwright suite (run against a staging instance) verifies auth session reuse and core navigation across desktop and mobile browser projects.

---

## 📖 Documentation

- 📘 [**Quickstart Guide**](docs/QUICKSTART.md) — Comprehensive developer setup and client tutorials.
- 📙 [**Operations Runbook**](docs/runbook-operasi.md) — Production operations, service monitoring, and logging.
- 📕 [**Disaster Recovery**](docs/DISASTER_RECOVERY.md) — Database backup, restoration, and outage mitigation.
- 📗 [**OpenAPI Specification**](docs/openapi.yaml) — Schemas for the public gateway endpoints and a subset of the admin API. It does **not** yet cover the full admin surface (100+ operations in code); the dashboard's API client and the admin route registrations in `internal/admin/` are the current source of truth for admin endpoints.
- 📜 [**Multi-Agent Governance Rules**](.agents/rules/multi_agent_orchestration.md) — Full 13-agent orchestration specification.

---

## 📄 License & Attribution

Copyright © 2026 **NexGen-X / Route-X Development Team**.  
All rights reserved. Designed for reliable, high-performance, private, self-hosted AI gateway infrastructure.
