# 🚀 Route-X — Next-Gen AI Gateway

<div align="center">

<p align="center">
  <strong>High-Performance, Self-Hosted AI Gateway with OpenAI & Anthropic Dual-Protocol Ingestion, Dynamic Routing, Micro-Budget Governance, and Instant CLI Synchronization.</strong>
</p>

[![Release](https://img.shields.io/badge/Release-v1.0.0.0-10B981?style=for-the-badge&logo=github)](https://github.com/NexGen-X/Route-X/releases/tag/v1.0.0.0)
[![Go Version](https://img.shields.io/badge/Go-1.24+-00ADD8?style=for-the-badge&logo=go)](https://go.dev)
[![React](https://img.shields.io/badge/React-18-61DAFB?style=for-the-badge&logo=react)](https://react.dev)
[![TailwindCSS](https://img.shields.io/badge/Tailwind-3.4-38B2AC?style=for-the-badge&logo=tailwind-css)](https://tailwindcss.com)
[![PostgreSQL](https://img.shields.io/badge/PostgreSQL-16-336791?style=for-the-badge&logo=postgresql)](https://postgresql.org)
[![Redis](https://img.shields.io/badge/Redis-7-DC382D?style=for-the-badge&logo=redis)](https://redis.io)
[![Data-Race Free](https://img.shields.io/badge/Race%20Detector-PASSED%20(0%20Race)-success?style=for-the-badge&logo=shield)](https://github.com/NexGen-X/Route-X)

<p align="center">
  <a href="#-quickstart"><strong>Quickstart</strong></a> •
  <a href="#-core-features"><strong>Core Features</strong></a> •
  <a href="#-architecture"><strong>Architecture</strong></a> •
  <a href="#-developer-cli-ecosystem"><strong>Developer CLIs</strong></a> •
  <a href="#-api-compatibility-matrix"><strong>API Matrix</strong></a> •
  <a href="#-security--reliability"><strong>Security</strong></a> •
  <a href="#-documentation"><strong>Documentation</strong></a>
</p>

---

</div>

## 💡 Why Route-X?

Calling diverse AI APIs directly creates vendor lock-in, unmanaged API key sprawl, unpredictable rate limits, and fragile failovers. **Route-X** acts as a unified reverse proxy that normalizes all upstream dialects, balances traffic, enforces strict financial guardrails, and synchronizes your developer terminal CLI tools—all packaged in a **single, high-speed Go binary** with zero external runtime dependencies.

| Problem Without Route-X | Solution With Route-X |
|---|---|
| **Dialect Incompatibility**: Tools like Claude Code only speak Anthropic schema, while typical gateways only support OpenAI. | **Native Dual-Protocol**: Seamlessly transpile `/v1/chat/completions` and `/v1/messages` bidirectional with full SSE streaming support. |
| **Silent Provider Outages**: If your primary AI model goes down, user requests fail. | **Smart Cascade Failover**: Sub-millisecond automatic fallback to secondary providers with 3-state circuit breaking. |
| **Manual Terminal Configuration**: Editing dotfiles and exporting environment variables for every CLI tool. | **Automated CLI Synchronization**: 1-click config sync for Claude Code, OpenCode, Aider, SGPT, Fabric, and OpenCommit. |
| **Financial Overdrafts**: Surprise monthly bills when rate limits or usage quotas are breached. | **Micro-Budgeting**: Real-time $0.00000001 precision cost accounting and Redis sliding-window token bucket limiters. |
| **Complex Multi-Service Deployments**: Needing separate Node.js servers, web frontends, and auth sidecars. | **Single Static Binary**: React 18 dashboard embedded directly inside the Go executable via `//go:embed`. |

---

## 🌟 Core Features

### 🌐 1. Dual-Protocol Engine (OpenAI & Anthropic Native)
- **100% OpenAI API Compatible**: Drop-in replacement for OpenAI SDK, LangChain, LlamaIndex, LiteLLM, Cursor, Cline, and Open WebUI on `/v1/chat/completions`.
- **Native Anthropic Messages API**: Full native compatibility for `/v1/messages` and `/v1/v1/messages` (specifically tuned for **Claude Code CLI**).
- **Multimodal & Tool Use Transpilation**: Converts roles, arrays, base64 vision images, and function/tool calls seamlessly between provider dialects.
- **Robust SSE Streaming**: Zero-buffering event streaming with UTF-8 multibyte boundary split safety and immediate upstream cancellation on client disconnect.

### 🔀 2. Intelligent Combo Routing & Failover
- **Routing Strategies**:
  - **Single Priority**: Strict tier-based routing.
  - **Cascading Failover**: Automatic retry & cascade to healthy providers on upstream 5xx, timeouts, or rate limits.
  - **Round Robin & Weighted Balancing**: Atomic traffic distribution with stochastic accuracy (<3% variance).
  - **Lowest Latency (EMA)**: Moving-window Exponential Moving Average tracking P95 response times.
- **Smart Context Window Bypass**: Proactively routes queries to higher-context models if the prompt length exceeds the primary model's capacity.
- **3-State Circuit Breaker**: Fast-fails broken upstreams (Closed → Open → Half-Open canary) to preserve gateway throughput.

### 💻 3. Developer CLI Integration Hub
- **Zero-Friction Terminal Setup**: Automatically provisions configuration files for:
  - **Claude Code CLI** (`~/.claude/settings.json`, base URL path normalization)
  - **OpenCode CLI** (`~/.config/opencode/opencode.json`)
  - **Aider** (`~/.aider.conf.yml`)
  - **Shell-GPT / SGPT** (`~/.config/shell_gpt/.sgptrc`)
  - **Fabric** (`~/.config/fabric/.env`)
  - **OpenCommit** (`~/.opencommit`)
- **Live Diagnostics Tool**: Test round-trip latency (in milliseconds) directly from the Web UI to verify endpoint health.
- **Shell Auto-Loader**: Seamless injection via `/etc/profile.d/routex.sh` and `~/.bashrc` for fresh terminal sessions.
- **Zero-Touch Antigravity Protection**: Guaranteed isolation ensuring Antigravity CLI (`agy`) and `~/.gemini` remain 100% untouched.

### 🛡️ 4. Zero-Trust Security Architecture
- **Authenticated Encryption at Rest**: Provider API credentials and master keys encrypted with **AES-256-GCM** using fresh nonces and AEAD verification.
- **Argon2id Password Hashing**: Console credentials protected with constant-time verification to prevent timing side-channel attacks.
- **Transport-Level Anti-SSRF Dialer**: Custom egress dialer blocks DNS rebinding and internal RFC 1918 addresses, loopbacks, and cloud metadata (`169.254.169.254`).
- **Anti-CSRF & Origin Enforcement**: Double-submit cookie verification combined with strict host and forwarded-header validation on all mutating admin endpoints.

### 📊 5. Micro-Budgeting & Real-Time Telemetry
- **High-Precision Accounting**: Tracks costs with 8-decimal micro-precision (`$0.00000001` USD).
- **Partitioned Time-Series Storage**: Every request's TTFT, input/output tokens, latency, and status logged in monthly PostgreSQL partitions.
- **Distributed Redis Rate Limiting**: Sliding-window rate limiters executing atomic Lua scripts for RPM, TPM, and daily quotas.

---

## 🛠️ Architecture

```
                          CLIENT LAYER
 ┌─────────────────┐   ┌─────────────────┐   ┌─────────────────┐
 │ OpenAI SDK / AI │   │ Claude Code CLI │   │ Developer IDEs  │
 │  Agents / WebUI │   │ & Anthropic SDK │   │ (Cursor, Aider) │
 └────────┬────────┘   └────────┬────────┘   └────────┬────────┘
          │ (OpenAI Schema)     │ (Anthropic Schema)  │
          └────────────────┐    │    ┌────────────────┘
                           ▼    ▼    ▼
┌──────────────────────────────────────────────────────────────────┐
│                        ROUTE-X AI GATEWAY                        │
│                                                                  │
│  ┌────────────────────────┐         ┌─────────────────────────┐  │
│  │ Authenticator & Guard  │────────▶│ Dialect Normalizer &    │  │
│  │ (API Key / CSRF / SSRF)│         │ Transpiler (Bidirectional)│ │
│  └────────────────────────┘         └────────────┬────────────┘  │
│                                                  ▼               │
│  ┌────────────────────────┐         ┌─────────────────────────┐  │
│  │ Financial Governance   │◀────────│ Combo Router & Circuit  │  │
│  │ (Redis Sliding Limits) │         │ Breaker (Priority/EMA)  │  │
│  └────────────────────────┘         └────────────┬────────────┘  │
│                                                  ▼               │
│  ┌────────────────────────┐         ┌─────────────────────────┐  │
│  │ Embedded Dashboard     │         │ Egress Proxy Pool       │  │
│  │ (React 18 + Tailwind)  │         │ (Direct / Xray Mesh)    │  │
│  └────────────────────────┘         └────────────┬────────────┘  │
└──────────────────────────────────────────────────┼───────────────┘
          │                                        │
          ├──▶ PostgreSQL 16 (Partitions & State)  ▼
          └──▶ Redis 7 (Distributed Cache & Rate)  UPSTREAM PROVIDERS
                                                   (OpenAI, Anthropic,
                                                    Groq, Custom vLLM)
```

---

## ⚡ Quickstart

### Option A: Automated Native Linux 1-Liner (Recommended for Production)
Installs and configures Route-X, PostgreSQL, Redis, Caddy (with Auto-TLS), and systemd service:

```bash
curl -fsSL https://raw.githubusercontent.com/NexGen-X/Route-X/main/install-native.sh | sudo bash
```

### Option B: Docker Compose
Run the complete containerized stack in isolated containers:

```bash
git clone https://github.com/NexGen-X/Route-X.git
cd Route-X
bash deploy.sh
```

### Option C: Manual Build from Source
Requirements: **Go 1.24+**, **Node.js 20+**, **PostgreSQL 16**, **Redis 7**.

```bash
# 1. Clone repository
git clone https://github.com/NexGen-X/Route-X.git
cd Route-X

# 2. Build React Dashboard
cd web && npm install && npm run build && cd ..

# 3. Compile Go Binary
go build -ldflags "-X main.version=v1.0.0.0" -o ai-gateway ./cmd/ai-gateway

# 4. Run Database Migrations
./ai-gateway -migrate

# 5. Start the Gateway
./ai-gateway
```

---

## 🔐 First-Run Onboarding

Upon initial deployment, open the web console at `http://localhost:8080/login` (or your domain `https://your-domain.com/login`):

- **Default Email**: `admin@routex.local`
- **Default Password**: `RouteX#Initial2026!`
- *💡 Use the **"1-Klik Autofill"** button on the setup card to populate the fields instantly.*
- *🔒 The system enforces an immediate password change on first login. Once updated, setup helper mode is permanently disabled.*

---

## 💻 Developer CLI Ecosystem

Route-X features built-in, out-of-the-box support for the developer terminal CLI ecosystem.

```bash
# Claude Code CLI (Uses Anthropic /v1/messages protocol via Route-X)
claude -p "Explain the Route-X architecture in one sentence."

# OpenCode CLI (Uses OpenAI protocol with direct model resolution)
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

## 📋 API Compatibility Matrix

Route-X exposes standardized endpoints compatible with all official AI SDKs:

| Endpoint | Method | Dialect Schema | Streaming (SSE) | Tool / Function Calling |
|---|:---:|---|:---:|:---:|
| `/v1/chat/completions` | `POST` | OpenAI Format | ✅ Yes | ✅ Yes |
| `/chat/completions` | `POST` | OpenAI Format | ✅ Yes | ✅ Yes |
| `/v1/messages` | `POST` | Anthropic Format | ✅ Yes | ✅ Yes |
| `/messages` | `POST` | Anthropic Format | ✅ Yes | ✅ Yes |
| `/v1/v1/messages` | `POST` | Claude Code Compatibility | ✅ Yes | ✅ Yes |
| `/v1/models` | `GET` | OpenAI Model List | — | — |

### Python Example (OpenAI SDK)
```python
from openai import OpenAI

client = OpenAI(
    base_url="https://your-routex-domain.com/v1",
    api_key="sk_live_your_routex_api_key"
)

response = client.chat.completions.create(
    model="claude-3-7-sonnet", # Handled via dynamic alias resolution
    messages=[{"role": "user", "content": "Hello Route-X!"}],
    stream=True
)

for chunk in response:
    if chunk.choices[0].delta.content:
        print(chunk.choices[0].delta.content, end="", flush=True)
```

### cURL Example (Anthropic Messages API)
```bash
curl -X POST https://your-routex-domain.com/v1/messages \
  -H "x-api-key: sk_live_your_routex_api_key" \
  -H "anthropic-version: 2023-06-01" \
  -H "Content-Type: application/json" \
  -d '{
    "model": "claude-3-5-sonnet",
    "max_tokens": 1024,
    "messages": [{"role": "user", "content": "Hello via Anthropic dialect!"}]
  }'
```

---

## 🛡️ Reliability & Benchmarks

Route-X is built with Go's memory-safe concurrency primitives:
- **Race Detector Tested**: All core packages passed `go test -race` with **0 data races detected**.
- **Time to First Token (TTFT)**: Adds `< 1.2ms` internal proxy overhead for streaming tokens.
- **Connection Reuse**: HTTP keep-alive pooling eliminates TCP handshake latency on warm upstreams.
- **Fail-Open Resilience**: In-memory degraded mode ensures inference requests pass even if Redis temporarily restarts.

---

## 📖 Documentation

- 📘 [**Quickstart Guide**](docs/QUICKSTART.md) — Comprehensive developer setup and client tutorials.
- 📙 [**Operations Runbook**](docs/runbook-operasi.md) — Production operations, service monitoring, and logging.
- 📕 [**Disaster Recovery**](docs/DISASTER_RECOVERY.md) — Database backup, restoration, and outage mitigation.
- 📗 [**OpenAPI Specification**](docs/openapi.yaml) — Raw API schemas and endpoint contracts.

---

## 📄 License & Attribution

Copyright © 2026 **NexGen-X / Route-X Development Team**.  
All rights reserved. Designed for reliable, private, self-hosted AI gateway infrastructure.
