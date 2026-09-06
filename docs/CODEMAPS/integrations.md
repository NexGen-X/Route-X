# Integrations & External Services Codemap

**Last Updated:** 2026-09-06  
**Area:** Upstream AI Providers, Egress Tunnels, Webhooks, and Observability Integrations  
**Entry Points:**
- [`internal/providers/providers.go`](file:///root/Route-X/internal/providers/providers.go) — Adapter factory & provider interfaces
- [`internal/xray/xray.go`](file:///root/Route-X/internal/xray/xray.go) — Stealth egress proxy and multi-protocol tunneling engine
- [`internal/webhooks/dispatcher.go`](file:///root/Route-X/internal/webhooks/dispatcher.go) — Webhook event delivery engine
- [`docs/embed.go`](file:///root/Route-X/docs/embed.go) — Scalar OpenAPI 3.1 interactive documentation handler

---

## Architecture

```
+-----------------------------------------------------------------------------------------+
|                                  External Integrations                                  |
|                                                                                         |
|  +-----------------------------------------------------------------------------------+  |
|  |                           Route-X Gateway Core                                    |  |
|  +-------------------+--------------------+-------------------+----------------------+  |
|                      |                    |                   |                         |
|                      v                    v                   v                         v
|             +-----------------+  +-----------------+  +---------------+  +-----------+  |
|             | Provider Layer  |  | Egress Proxies  |  |   Webhooks    |  | Metrics & |  |
|             | Wire Translators|  | Xray Subsystem  |  |  Dispatcher   |  | OpenAPI   |  |
|             +--------+--------+  +--------+--------+  +-------+-------+  +-----+-----+  |
|                      |                    |                   |                |        |
+----------------------|--------------------|-------------------|----------------|--------+
                       |                    |                   |                |
         +-------------+-------------+      |                   |                |
         |             |             |      |                   |                |
         v             v             v      v                   v                v
    +---------+   +---------+   +---------+ |           +---------------+  +-----------+
    | OpenAI  |   |Anthropic|   | Google  | |           | Client App    |  |Prometheus |
    | Official|   | Claude  |   | Gemini  | |           | Webhook URL   |  |Scraper    |
    | API     |   | Messages|   | Content | |           | (HMAC-SHA256) |  |/metrics   |
    +---------+   +---------+   +---------+ |           +---------------+  +-----------+
         ^             ^             ^      |
         |             |             |      |
         +-------------+-------------+------+
                       |
               (via Xray Reality,
                VMess, Trojan, SS)
```

---

## Key Modules

### 1. Upstream AI Providers ([`internal/providers/`](file:///root/Route-X/internal/providers/))
Route-X menyediakan adapter dua arah yang menerjemahkan format standar OpenAI wire format ke format spesifik tiap penyedia:

| Adapter | Lokasi | Format yang Diterjemahkan |
| :--- | :--- | :--- |
| **OpenAI** | [`internal/providers/openai/`](file:///root/Route-X/internal/providers/openai/) | Native passthrough OpenAI `/v1/chat/completions`, embeddings, streaming SSE |
| **Anthropic** | [`internal/providers/anthropic/`](file:///root/Route-X/internal/providers/anthropic/) | Translasi OpenAI schema ke Anthropic Messages API (`/v1/messages`), event streaming SSE |
| **Google** | [`internal/providers/google/`](file:///root/Route-X/internal/providers/google/) | Translasi OpenAI schema ke Gemini Generative Language API (`generateContent`, `streamGenerateContent`) |
| **Compatible** | [`internal/providers/compatible/`](file:///root/Route-X/internal/providers/compatible/) | Mendukung vLLM, Ollama, Groq, Mistral, Together, OpenRouter, DeepSeek |
| **Custom** | [`internal/providers/custom/`](file:///root/Route-X/internal/providers/custom/) | Header kustom, model mapping, override base URL |

### 2. Multi-Protocol Egress Tunnels ([`internal/xray/`](file:///root/Route-X/internal/xray/))
Untuk mengakses provider di wilayah terbatas geolokasi atau jaringan sensor:
- **Protokol:** VLESS Reality (dengan TLS camouflage), VMess, Trojan, Shadowsocks, standar HTTP/SOCKS5.
- **Proxy Pool:** Pool egress dapat dikaitkan langsung ke provider tertentu untuk rotasi otomatis.

### 3. Webhooks Automation ([`internal/webhooks/`](file:///root/Route-X/internal/webhooks/))
- **Keamanan:** Setiap muatan payload ditandatangani menggunakan **HMAC-SHA256** pada header `X-RouteX-Signature`.
- **Toleransi Galat:** *Equal jitter exponential backoff* dengan penanganan retry otomatis hingga 5 kali.
- **Isolasi Database:** Pengambilan antrean pengiriman menggunakan query non-blocking `SELECT ... FOR UPDATE SKIP LOCKED`.

### 4. Observability & Dokumentasi
- **Prometheus Metrics:** Terbuka di `/metrics` via `httpx.MetricsRecorder` dan dilindungi dengan bearer token atau izin loopback.
- **Scalar OpenAPI 3.1:** Menyajikan dokumentasi interaktif pada `/docs` dari spesifikasi [`docs/openapi.yaml`](file:///root/Route-X/docs/openapi.yaml). Seluruh file JavaScript Scalar disematkan lokal mematuhi Content Security Policy.

---

## Related Areas
- [Backend Architecture Codemap](file:///root/Route-X/docs/CODEMAPS/backend.md)
- [Workers Codemap](file:///root/Route-X/docs/CODEMAPS/workers.md)
