# Route-X — Next-Gen AI Gateway

<div align="center">

**Open-source, self-hosted, high-performance AI Gateway with Dynamic Routing, Multi-Provider Failover, Micro-Budget Governance, and Unified Observability.**

[![Release](https://img.shields.io/badge/Release-v1.0.0.0-success?style=flat&logo=github)](https://github.com/NexGen-X/Route-X/releases)
[![Go Version](https://img.shields.io/badge/Go-1.27+-00ADD8?style=flat&logo=go)](https://go.dev)
[![React](https://img.shields.io/badge/React-18-61DAFB?style=flat&logo=react)](https://react.dev)
[![TailwindCSS](https://img.shields.io/badge/Tailwind-3.4-38B2AC?style=flat&logo=tailwind-css)](https://tailwindcss.com)
[![PostgreSQL](https://img.shields.io/badge/PostgreSQL-16-336791?style=flat&logo=postgresql)](https://postgresql.org)
[![Redis](https://img.shields.io/badge/Redis-7-DC382D?style=flat&logo=redis)](https://redis.io)

</div>

---

## 🌟 Fitur Utama (v1.0.0.0)

- **Dual-Protocol Engine (OpenAI & Anthropic Native)**:
  - **100% OpenAI API Compatible**: `/v1/chat/completions` untuk seluruh library AI modern (OpenAI SDK, LangChain, LlamaIndex, Cursor, Cline).
  - **Anthropic Messages API Native**: Mendukung penuh `/v1/messages` dan `/v1/v1/messages` dengan transpilasi skema otomatis untuk Claude Code CLI dan alat berbasis Anthropic lainnya (non-streaming & streaming SSE).
- **Automated Developer CLI Integrations**:
  - Konfigurasi terpadu untuk Claude Code, OpenCode, Aider, Shell-GPT (SGPT), Fabric, dan OpenCommit.
  - Pengujian live diagnostik koneksi dan latensi (ms) langsung dari dashboard.
  - Auto-loader shell (`~/.bashrc` & `/etc/profile.d/routex.sh`) untuk lingkungan terminal instan.
  - Proteksi sistem mutlak untuk Antigravity CLI (`agy`).
- **Combo Routing Pipeline**:
  - **Single Priority**: Rute langsung ke provider dengan latensi terendah.
  - **Cascading Failover**: Pengalihan otomatis saat upstream mengalami gangguan, kehabisan saldo, atau rate limit.
  - **Weighted Load Balancing**: Pembagian beban dinamis dengan algoritma weighted round-robin.
- **Single-Admin Architecture & Keamanan Sederhana**:
  - Dirancang khusus untuk open-source & self-hosted personal/tim kecil tanpa kerumitan matriks peran (RBAC).
  - Seluruh pengguna terautentikasi memiliki peran tunggal `Admin` dengan hak akses penuh (`*`).
  - Proteksi *anti-lockout* permanen pada akun admin aktif terakhir.
  - Hashing kunci API berbasis HMAC-SHA256 ber-pepper server, enkripsi AES-256-GCM kredensial, proteksi SSRF dialer, & Argon2id untuk sandi konsol.
- **First-Run Onboarding (Setup Awal Instan)**:
  - Instalasi baru otomatis mendeteksi ketiadaan admin kustom dan menyediakan kredensial bawaan.
  - Kartu bantuan di halaman Login web dilengkapi tombol **"Gunakan Kredensial Default (1-Klik)"** untuk autofill instan.
  - Pengalihan paksa ganti kata sandi pada login pertama; setelah sandi diperbarui, kartu setup dinonaktifkan permanen.
- **Micro-Budget & Cost Governance**:
  - Pelacakan biaya komputasi presisi tinggi dengan skala 8 desimal (`0.00000001` USD).
  - Pagu pengeluaran harian dan bulanan dengan pemutusan otomatis (*hard stop*) saat kuota terlampaui.
- **Observabilitas & Telemetri Real-Time**:
  - Pencatatan seluruh transaksi di tabel berpartisi Postgres.
  - Metrik latensi P95/P99, throughput TPM/RPM, dan visualisasi deret waktu telemetri.
- **Automated Webhooks & Alerting**:
  - Pengiriman event asinkron berbasis signature HMAC ke Telegram, Slack, atau endpoint pihak ketiga.

---

## ⚡ Instalasi Cepat (Quick Install)

### Opsi A: Docker Compose (Direkomendasikan)
```bash
git clone https://github.com/NexGen-X/Route-X.git
cd Route-X
bash deploy.sh
```

### Opsi B: Native Linux 1-Liner (Route-X + Caddy + Xray + PostgreSQL + Redis)
```bash
curl -fsSL https://raw.githubusercontent.com/NexGen-X/Route-X/main/install-native.sh | sudo bash
```
*(Atau jika sudah mengkloning repositori: `sudo bash install-native.sh`)*

### 🔐 Kredensial Login Pertama Kali:
Buka konsol dashboard di peramban Anda (`http://localhost:8080/login`):
- **Email**: `admin@routex.local`
- **Password**: `RouteX#Initial2026!`
*(Gunakan tombol 1-Klik Autofill pada kartu "SETUP AWAL" di halaman login untuk langsung mengisi kolom kredensial).*

---

## 🚀 Panduan Pengembang (Developer Guide)

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
             ├──▶ PostgreSQL (Metadata, Partitioned Logs, User Accounts)
             ├──▶ Redis (Distributed Cache, Sliding Window Rate Limit)
             └──▶ Upstream AI Providers (TokenHarbor, JustWorker, etc.)
```

---

## 📄 Lisensi

Hak Cipta © 2026 NexGen-X / Route-X Team. Seluruh hak cipta dilindungi.
