# Route-X — Next-Gen AI Gateway

<div align="center">

**Enterprise-grade, high-performance AI Gateway with Dynamic Routing, Multi-Provider Failover, Budget Governance, and Unified Observability.**

[![Go Version](https://img.shields.io/badge/Go-1.22+-00ADD8?style=flat&logo=go)](https://go.dev)
[![React](https://img.shields.io/badge/React-18-61DAFB?style=flat&logo=react)](https://react.dev)
[![TailwindCSS](https://img.shields.io/badge/Tailwind-3.4-38B2AC?style=flat&logo=tailwind-css)](https://tailwindcss.com)
[![PostgreSQL](https://img.shields.io/badge/PostgreSQL-16-336791?style=flat&logo=postgresql)](https://postgresql.org)
[![Redis](https://img.shields.io/badge/Redis-7-DC382D?style=flat&logo=redis)](https://redis.io)

</div>

---

## 🌟 Fitur Utama

- **100% OpenAI API Compatible**: Mendukung integrasi *drop-in* untuk seluruh library AI modern (OpenAI Python/Node SDK, LangChain, LlamaIndex, LiteLLM, cURL).
- **Combo Routing Pipeline**:
  - **Single Priority**: Rute langsung ke provider dengan latensi terendah.
  - **Cascading Failover**: Pengalihan otomatis saat upstream mengalami gangguan, kehabisan saldo, atau rate limit.
  - **Weighted Load Balancing**: Pembagian beban dinamis dengan algoritma weighted round-robin.
- **Enterprise RBAC & Keamanan**:
  - Peran *Super Admin* dengan proteksi *anti-lockout* permanen.
  - 5 grup perizinan matriks hak akses.
  - Hashing kunci API berbasis HMAC-SHA256 ber-pepper rahasia server.
- **Micro-Budget & Cost Governance**:
  - Pelacakan biaya komputasi presisi tinggi dengan skala 8 desimal (`0.00000001` USD).
  - Pagu pengeluaran harian dan bulanan dengan pemutusan otomatis (*hard stop*) saat kuota terlampaui.
- **Observabilitas & Telemetri Real-Time**:
  - Pencatatan seluruh transaksi di tabel berpartisi Postgres.
  - Metrik latensi P95/P99, throughput TPM/RPM, dan visualisasi deret waktu telemetri.
- **Automated Webhooks & Alerting**:
  - Pengiriman event asinkron berbasis signature HMAC ke Telegram, Slack, atau endpoint pihak ketiga.

---

## 🚀 Panduan Memulai Cepat (Quickstart)

Silakan buka dokumentasi integrasi lengkap di:
👉 [**docs/QUICKSTART.md**](docs/QUICKSTART.md)

### Contoh Singkat (Python)
```python
from openai import OpenAI

client = OpenAI(
    base_url="https://id-tech.cloud/v1",
    api_key="sk_live_xxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxx"
)

response = client.chat.completions.create(
    model="gpt-5.6-terra",
    messages=[{"role": "user", "content": "Halo Route-X!"}]
)
print(response.choices[0].message.content)
```

---

## 🛠️ Arsitektur Sistem

```
Klien / SDK (Python, JS, cURL)
             │  (HTTPS /v1/chat/completions)
             ▼
┌─────────────────────────────────────────────────────────┐
│                    Route-X AI Gateway                   │
│                                                         │
│  ┌────────────────┐   ┌──────────────────────────────┐  │
│  │ Authenticator  │──▶│ Model Router & Combo Engine  │  │
│  │ (HMAC-SHA256)  │   │ (Single / Failover / W-RR)   │  │
│  └────────────────┘   └──────────────┬───────────────┘  │
│                                      ▼                  │
│  ┌────────────────┐   ┌──────────────────────────────┐  │
│  │ Usage & Budget │◀──│ Upstream Dispatcher (SSE/Req)│  │
│  │ Governance     │   │ (Anthropic, OpenAI, Custom)  │  │
│  └────────────────┘   └──────────────────────────────┘  │
└─────────────────────────────────────────────────────────┘
             │
             ├──▶ PostgreSQL (Metadata, Partitioned Logs, RBAC)
             ├──▶ Redis (Distributed Cache, Sliding Window Rate Limit)
             └──▶ Upstream AI Providers (TokenHarbor, JustWorker, etc.)
```

---

## 📄 Lisensi

Hak Cipta © 2026 NexGen-X / Route-X Team. Seluruh hak cipta dilindungi.
