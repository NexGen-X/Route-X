# 🚀 Route-X — Next-Generation AI Gateway

<div align="center">

<p align="center">
  <strong>Enterprise-grade, ultra-low-latency AI Gateway with native dual-protocol ingestion (OpenAI & Anthropic), intelligent multi-model combo routing, sub-microsecond zero-allocation hot-path, micro-budget financial governance, and instant developer CLI synchronization.</strong>
</p>

[![Release](https://img.shields.io/badge/Release-v1.3.1-10B981?style=for-the-badge&logo=github)](https://github.com/NexGen-X/Route-X/releases/tag/v1.3.1)
[![Go Version](https://img.shields.io/badge/Go-1.24+-00ADD8?style=for-the-badge&logo=go)](https://go.dev)
[![React](https://img.shields.io/badge/React-18-61DAFB?style=for-the-badge&logo=react)](https://react.dev)
[![TailwindCSS](https://img.shields.io/badge/Tailwind-3.4-38B2AC?style=for-the-badge&logo=tailwind-css)](https://tailwindcss.com)
[![PostgreSQL](https://img.shields.io/badge/PostgreSQL-16-336791?style=for-the-badge&logo=postgresql)](https://postgresql.org)
[![Redis](https://img.shields.io/badge/Redis-7-DC382D?style=for-the-badge&logo=redis)](https://redis.io)
[![Data-Race Free](https://img.shields.io/badge/Race%20Detector-PASSED%20(0%20Race)-success?style=for-the-badge&logo=shield)](https://github.com/NexGen-X/Route-X)
[![A11y WCAG 2.1 AA](https://img.shields.io/badge/A11y-WCAG%202.1%20AA-blueviolet?style=for-the-badge&logo=w3c)](https://www.w3.org/WAI/standards-guidelines/wcag/)
[![Unit Tests](https://img.shields.io/badge/Unit%20Tests-171%2F171%20Passed-brightgreen?style=for-the-badge&logo=vitest)](https://github.com/NexGen-X/Route-X)
[![Hot Path](https://img.shields.io/badge/Hot--Path-Sub--Microsecond%20Zero--Alloc-FF6B6B?style=for-the-badge&logo=speedtest)](https://github.com/NexGen-X/Route-X)

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

**Route-X** provides an enterprise-ready reverse proxy and control plane that abstracts upstream providers behind unified interfaces, enforces real-time financial guardrails, executes sub-microsecond routing decisions, and coordinates developer terminal tools—shipped as a **single, self-contained Go binary** with zero external frontend runtime dependencies.

| Enterprise Pain Point | Route-X Solution |
|---|---|
| **Dialect Incompatibility**: Tools like Claude Code expect Anthropic Messages schema, while standard enterprise gateways only ingest OpenAI. | **Native Dual-Protocol Engine**: Transparent, zero-copy transpilation between `/v1/chat/completions` and `/v1/messages` with bidirectional streaming. |
| **Upstream Provider Outages**: Silent model downtime interrupts critical inference pipelines. | **Cascading Failover & 3-State Circuit Breaker**: Sub-millisecond canary probes, automatic failover cascades, and proactive context window bypass. |
| **Hot-Path Proxy Latency Overhead**: Heavy gateways introduce 10–50ms proxy latency per hop. | **Sub-Microsecond Zero-Alloc Core**: 23ns rule matching and 87ns token-bucket evaluation running in pure Go memory. |
| **Inaccessible Internal Tools**: Compliance failure on enterprise accessibility audits. | **WCAG 2.1 Level AA Certified UI**: Strict WAI-ARIA comboboxes, FocusTrap modal containment, explicit label bindings, and screen-reader readiness. |
| **Developer Onboarding Friction**: Manual terminal configuration and dotfile management across 6+ CLI tools. | **Automated CLI Synchronization**: 1-click declarative provisioning for Claude Code, OpenCode, Aider, Shell-GPT, Fabric, and OpenCommit. |
| **Runaway AI Operational Costs**: Surprise billing spikes when API keys or automated agents loop indefinitely. | **Micro-Budget Governance**: Real-time `$0.00000001` precision accounting with Redis sliding-window token bucket enforcement. |
| **Fragile Multi-Container Deployments**: Requiring Node.js runtime, Nginx sidecars, and separate static asset hosts. | **Single Static Executable**: React 18 dashboard embedded directly into the Go binary via `//go:embed`. |

---

## 🌟 Core Pillars

### 🌐 1. Dual-Protocol Native Ingestion
- **OpenAI Standard Compatibility**: Drop-in target for official OpenAI Python/TypeScript SDKs, LangChain, LlamaIndex, LiteLLM, Cursor IDE, Cline, and Open WebUI via `/v1/chat/completions`.
- **Native Anthropic Messages Ingestion**: Drop-in target for Anthropic SDK and **Claude Code CLI** via `/v1/messages` and `/v1/v1/messages`.
- **Bidirectional Dialect Transpiler**: Converts roles, arrays, base64 vision payloads, and structured function/tool calling between OpenAI and Anthropic specifications on the fly.
- **Resilient SSE Streaming Engine**: Zero-buffering event streaming pipeline with UTF-8 multibyte boundary split safety and immediate upstream cancellation upon client disconnect.

### ⚡ 2. Sub-Microsecond Performance & Zero-Alloc Hot-Path
Route-X hot-path is engineered for extreme throughput with minimal garbage collection overhead:
- **Zero-Allocation Rule Matching**: Evaluates model IDs, API keys, and model capability constraints in just **23 nanoseconds** with **0 heap allocations**.
- **Deterministic FNV-1a Tie-Breaker**: High-entropy 64-bit jitter hash eliminates load skew on balanced weights in **35 nanoseconds** without pseudo-random locking.
- **In-Memory Token Bucket Limiting**: Evaluates rate limits in **87 nanoseconds** (single-goroutine) and **118 nanoseconds** (highly concurrent multi-goroutine).

### 🔀 3. Intelligent Multi-Model Combo Routing & Resiliency
- **Sophisticated Balancing Strategies**:
  - **Priority Tiering**: Strict deterministic failover across upstream tiers.
  - **Weighted Distribution**: Efraimidis-Spirakis reservoir sampling with FNV-1a tie-breaking (<3% statistical variance).
  - **Lowest Latency (EMA)**: Dynamic Exponential Moving Average tracking P95 response times.
  - **Lowest Cost**: Dynamic routing to the lowest cost-per-token provider matching requirements.
  - **Round Robin**: Lock-free atomic round-robin across healthy endpoints.
- **Proactive Context Overflow Bypass**: Seamlessly upgrades requests to higher-context upstream models when prompt length exceeds the primary provider's limit.
- **3-State Circuit Breaker**: Evaluates health transitions (`Closed` ➔ `Open` ➔ `Half-Open` canary) with zero thread contention to protect gateway throughput.

### ♿ 4. Full Accessibility (WCAG 2.1 Level AA Certified)
- **Focus Management**: Complete focus trapping (`FocusTrap`) on Command Palette (`Cmd+K`), Dialogs, and Mobile Drawers.
- **WAI-ARIA Combobox Specification**: Fully compliant searchable Command Palette with `role="dialog"`, `role="combobox"`, `role="listbox"`, and `role="option"`.
- **Form Association & Screen Reader Support**: 100% explicit `<label htmlFor>` and `<input id>` associations, semantic error alerts (`role="alert"`), and `aria-label` standardization on all icon-only triggers.
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

### 🛡️ 6. Zero-Trust Financial & Operational Guardrails
- **Micro-Budget Precision**: Real-time cost accounting with 8-decimal precision (`$0.00000001` USD) across users, keys, and teams.
- **AES-256-GCM Credential Vault**: Provider credentials and API keys encrypted at rest using AES-256-GCM with distinct nonces and AEAD verification.
- **Transport Anti-SSRF Dialer**: Custom network dialer strictly blocks DNS rebinding, internal RFC 1918 networks, IPv6 loopbacks, and cloud metadata endpoints (`169.254.169.254`).
- **Argon2id Authentication**: Cryptographically sound password hashing with constant-time verification preventing timing side-channel attacks.

### 📦 7. Single Self-Contained Binary
- **Zero Web Server Dependencies**: React 18 production bundle is embedded directly into the Go executable via Go's native `//go:embed` directive.
- **Zero External Runtime**: No Node.js runtime, Nginx, or external static servers required in production.

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
            R1["In-Memory Token Bucket Limiter (87ns)"]
            R2["Micro-Budget Accounting ($0.00000001)"]
            R3["Rule Matching Engine (23ns Zero-Alloc)"]
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
            U2["WCAG 2.1 AA Accessible UI"]
        end
    end

    subgraph State["3. Storage & State Layer"]
        DB[("PostgreSQL 16\n(Partitions & State)")]
        RD[("Redis 7\n(Distributed Token Bucket)")]
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

Official benchmark results executed on dedicated **Intel(R) Xeon(R) Platinum 8259CL CPU @ 2.50GHz** (PR #78):

| Benchmark Test | Operations / sec | Time / op | Memory / op | Allocations / op |
|---|:---:|:---:|:---:|:---:|
| **`BenchmarkRule_Matches`** | **~43,400,000 ops/s** | **23.05 ns/op** | **0 B/op** | **0 allocs/op** |
| **`BenchmarkDeterministicTieBreaker`** | **~28,400,000 ops/s** | **35.13 ns/op** | **0 B/op** | **0 allocs/op** |
| **`BenchmarkMemoryLimiter_Allow`** | **~11,500,000 ops/s** | **87.08 ns/op** | **0 B/op** | **0 allocs/op** |
| **`BenchmarkMemoryLimiter_AllowParallel`** | **~8,400,000 ops/s** | **118.90 ns/op** | **0 B/op** | **0 allocs/op** |
| **`BenchmarkComboPipeline_Evaluate`** | **~7,500,000 ops/s** | **132.90 ns/op** | 32 B/op | 1 allocs/op |
| **`BenchmarkSelectCandidate_Priority`** | **~6,500,000 ops/s** | **154.70 ns/op** | 48 B/op | 1 allocs/op |
| **`BenchmarkFormatKey`** | **~14,300,000 ops/s** | **69.80 ns/op** | 16 B/op | 1 allocs/op |
| **`BenchmarkHash`** (SHA-256 Digest) | ~1,800,000 ops/s | 557.10 ns/op | 176 B/op | 3 allocs/op |
| **`BenchmarkSelectCandidate_RoundRobin`** | ~2,850,000 ops/s | 350.90 ns/op | 88 B/op | 4 allocs/op |
| **`BenchmarkSelectCandidate_LowestLatency`** | ~1,670,000 ops/s | 598.00 ns/op | 48 B/op | 1 allocs/op |
| **`BenchmarkSelectCandidate_LowestCost`** | ~1,720,000 ops/s | 579.70 ns/op | 48 B/op | 1 allocs/op |
| **`BenchmarkSelectCandidate_Weighted`** | ~880,000 ops/s | 1,139.00 ns/op | 48 B/op | 1 allocs/op |

> **Takeaway**: Critical routing decisions and rate-limit evaluations execute in **less than 150 nanoseconds**, introducing virtually zero measurable overhead to the upstream LLM round-trip time.

---

## ⚡ Quickstart

### Option A: Pre-Built Release Binary (Fastest)
Download and run the official static Linux binary with zero build steps:

```bash
# 1. Download official v1.3.1 release
curl -fsSLO https://github.com/NexGen-X/Route-X/releases/download/v1.3.1/routex-v1.3.1-linux-amd64.tar.gz

# 2. Extract archive
tar -xzvf routex-v1.3.1-linux-amd64.tar.gz

# 3. Apply schema migrations & run gateway
./ai-gateway -migrate
./ai-gateway
```

### Option B: Automated Native Linux 1-Liner (Production Systemd)
Installs Route-X Core, PostgreSQL 16, Redis 7, and Caddy Reverse Proxy with Auto-TLS via systemd:

```bash
curl -fsSL https://raw.githubusercontent.com/NexGen-X/Route-X/main/install-native.sh | sudo bash
```

#### 🔄 Zero-Downtime Live Updates
When updating an active staging or production instance:
```bash
git pull origin main
make live-update
```
*Performs an isolated atomic build (`ai-gateway.new`), applies idempotent migrations, swaps the binary, and reloads `routex.service` in under 250ms without dropping active streaming connections.*

### Option C: Docker Stack Deployment
Deploy the complete containerized stack in isolated containers:

```bash
git clone https://github.com/NexGen-X/Route-X.git
cd Route-X
bash deploy.sh
```

### Option D: Build from Source
Requirements: **Go 1.24+**, **Node.js 20+**, **PostgreSQL 16**, **Redis 7**.

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

Navigate to `http://localhost:8080/login` (or your domain `https://your-domain.com/login`):

- **Default Email**: `admin@routex.local`
- **Default Password**: `RouteX#Initial2026!`
- *💡 Click **"1-Klik Autofill"** on the initial setup card to populate credentials.*
- *🔒 Forced password rotation is strictly enforced upon first authentication. Once updated, the setup helper is permanently disabled.*

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
        {"role": "user", "content": "How does Route-X achieve sub-microsecond routing?"}
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
Every pull request and release is validated through 7 non-negotiable gates:
1. **Layer 1 (TypeScript Strictness)**: `npx tsc -b` passes with **0 type errors** and **0 `any` bypasses**.
2. **Layer 2 (Frontend Unit Testing)**: Vitest suite runs **171/171 tests passing** (100% green).
3. **Layer 3 (Asset Compilation)**: Vite production bundle builds cleanly under 10 seconds.
4. **Layer 4 (Go Concurrency & Race Detection)**: `go test -race ./...` verifies **0 data races** across all packages.
5. **Layer 5 (Go Formatting & Linting)**: `gofmt -l .` reports zero syntax or indentation defects.
6. **Layer 6 (SQL Schema Integrity)**: All 15+ PostgreSQL migrations run idempotently with atomic rollback semantics.
7. **Layer 7 (End-to-End Synthetic Validation)**: Playwright test suite verifies auth session reuse, 16 navigational routes, and zero layout overflow across 5 browser profiles.

---

## 📖 Documentation

- 📘 [**Quickstart Guide**](docs/QUICKSTART.md) — Comprehensive developer setup and client tutorials.
- 📙 [**Operations Runbook**](docs/runbook-operasi.md) — Production operations, service monitoring, and logging.
- 📕 [**Disaster Recovery**](docs/DISASTER_RECOVERY.md) — Database backup, restoration, and outage mitigation.
- 📗 [**OpenAPI Specification**](docs/openapi.yaml) — Raw API schemas and endpoint contracts.
- 📜 [**Multi-Agent Governance Rules**](.agents/rules/multi_agent_orchestration.md) — Full 13-agent orchestration specification.

---

## 📄 License & Attribution

Copyright © 2026 **NexGen-X / Route-X Development Team**.  
All rights reserved. Designed for reliable, high-performance, private, self-hosted AI gateway infrastructure.
