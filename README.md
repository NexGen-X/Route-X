# Route-X — AI Gateway & Intelligent Router

[![Go](https://img.shields.io/badge/Go-1.27.1-00ADD8?style=flat&logo=go)](https://golang.org)
[![PostgreSQL](https://img.shields.io/badge/PostgreSQL-16-336791?style=flat&logo=postgresql)](https://postgresql.org)
[![Redis](https://img.shields.io/badge/Redis-7-DC382D?style=flat&logo=redis)](https://redis.io)
[![License](https://img.shields.io/badge/License-Proprietary-BEF264?style=flat)](LICENSE)
[![Tests](https://img.shields.io/badge/Tests-100%25%20Passing%20(-race)-4ADE80?style=flat)]()

**Route-X** adalah gateway inferensi kecerdasan buatan (LLM Gateway & Router) tingkat enterprise berkinerja tinggi yang dirancang dengan ketahanan failover otomatis, enkripsi bertingkat, akuntansi biaya finansial berpresisi 8 desimal (`upstream.USD`), jejak audit lengkap, serta antarmuka visual tersemat dalam biner tunggal mandiri.

---

## Arsitektur Sistem

Route-X beroperasi sebagai biner tunggal (`./ai-gateway`, ~25 MB) yang menggabungkan API Gateway, Admin REST API, SPA Dashboard, dan Background Worker Supervisor tanpa memerlukan runtime Node.js di server produksi.

```mermaid
flowchart TD
    subgraph Clients["Klien & Aplikasi"]
        SDK["Klien SDK OpenAI / LangChain / LlamaIndex"]
        Browser["Browser Konsol Admin"]
    end

    subgraph RouteX["Route-X Gateway (Biner Tunggal :8080)"]
        Router["HTTP Router (Chi)"]
        AuthM["Autentikasi (HMAC-SHA256 Peppered / Argon2id)"]
        RL["Distributed Rate Limiter (Redis)"]
        CB["Circuit Breaker (Redis Lua)"]
        Engine["Routing Engine (Priority, Cost, Latency, Weighted)"]
        Audit["Audit Logger & Keyset Paginator"]
        SPA["Embedded Dashboard SPA (React + Tailwind)"]
        Docs["OpenAPI 3.1 Documentation (/docs)"]
    end

    subgraph Storage["Penyimpanan Data"]
        PG[("PostgreSQL 16 (pgxpool)")]
        RD[("Redis 7")]
    end

    subgraph Upstreams["Penyedia Hulu (Upstream Providers)"]
        OAI["OpenAI"]
        ANT["Anthropic"]
        GEM["Google Gemini"]
        LOCAL["vLLM / Ollama (Self-Hosted)"]
    end

    SDK -->|Bearer sk_live_...| Router
    Browser -->|Cookie + CSRF| Router
    Router --> AuthM --> RL --> CB --> Engine
    Engine --> Upstreams
    Router --> SPA
    Router --> Docs
    RouteX <--> PG
    RouteX <--> RD
```

---

## Fitur Utama

- **Kompatibilitas Penuh OpenAI Wire Format**:
  Dukungan langsung untuk `/v1/chat/completions`, `/v1/responses`, `/v1/embeddings`, dan `/v1/models`. Pengguna cukup mengganti `base_url` pada pustaka resmi OpenAI (Python, JS/TS, Go).
- **Enkripsi Kredensial Multi-Lapis**:
  Seluruh kunci API provider dan konfigurasi proxy keluar dienkripsi menggunakan cipher **AES-256-GCM** dengan data terotentikasi tambahan (AAD) yang diikat ke UUID entitas pemilik.
- **Perlindungan SSRF Dua Lapis**:
  Pencegahan serangan Server-Side Request Forgery via `security.SSRFPolicy` yang memblokir rentang alamat IP privat, loopback, link-local, dan multicast saat gateway menghubungi upstream provider maupun memanggil webhook.
- **Mesin Perutean Adaptif & Failover Otomatis**:
  Mendukung 6 strategi perutean: `priority`, `lowest_cost`, `lowest_latency`, `weighted`, `round_robin`, dan `fallback` dengan ambang batas percobaan ulang (retry backoff).
- **Pemutus Arus Terdistribusi (Circuit Breaker)**:
  Penyelarasan status pemutus arus (Closed, Half-Open, Open) antar-instance melalui Redis Lua Scripting ber-evaluasi atomik dan pemicuan notifikasi event `circuit.opened`.
- **Presisi Akuntansi Biaya Finansial**:
  Perhitungan biaya inferensi berbasis nilai integer skala 8 desimal (`upstream.USD` = $10^{-8}$) untuk mencegah galat akumulasi pembulatan `float64`.
- **Observabilitas & Request Inspector**:
  Pencatatan latensi TTFT (Time-to-First-Token), volume token masukan/keluaran, visualisasi failover `request_events`, penyimpanan raw payload ber-enkripsi, dan ekspor metrik Prometheus `/metrics`.
- **Otomasi Webhook**:
  Pengiriman asinkron bertenaga `FOR UPDATE SKIP LOCKED`, tanda tangan digital **HMAC-SHA256**, toleransi kegagalan dengan *equal jitter exponential backoff*, dan pemulihan *stuck lease*.
- **Supervisor Worker Mandiri**:
  Manajemen pekerjaan singleton (Health Checker, Usage Rollup, Retention Cleaner, Partition Maintainer, Budget Resetter, Webhook Worker) berpelindung isolasi panic dan PostgreSQL Advisory Lock (`pg_advisory_lock`).
- **Dashboard Web Tersemat (Dark Theme)**:
  Dibangun dengan React 18, TypeScript, Tailwind CSS, dan Recharts yang disematkan langsung via `go:embed all:dist` dengan palet warna elegan (`#0A0A0A`, `#101010`, aksen lime `#BEF264`).
- **Dokumentasi Interaktif OpenAPI 3.1**:
  Dokumentasi antarmuka publik dan administrasi yang disajikan pada `/docs` via Scalar dengan dukungan pencarian instan dan pengujian coba langsung (try-it-out).

---

## Panduan Memulai Cepat (Quickstart)

### Prasyarat
- **Go**: Versi 1.27+
- **PostgreSQL**: Versi 16+
- **Redis**: Versi 7+
- **Node.js**: Versi 20+ (hanya jika membangun frontend dari sumber)

### 1. Klon Repositori & Konfigurasi Lingkungan
```bash
git clone https://github.com/NexGen-X/Route-X.git
cd Route-X
cp .env.example .env
```

Sesuaikan nilai pada `.env`:
```ini
APP_ENV=development
PORT=8080
DATABASE_URL=postgres://routex:password@127.0.0.1:5432/routex?sslmode=disable
REDIS_URL=redis://127.0.0.1:6379/0
SESSION_SECRET=
ENCRYPTION_KEY=
API_KEY_PEPPER=
INITIAL_ADMIN_EMAIL=admin@routex.internal
INITIAL_ADMIN_PASSWORD=
```

### 2. Kompilasi Biner Tunggal
Target `make build` akan membangun bundel antarmuka web dan mengompilasi seluruh sistem ke dalam satu biner:
```bash
make build
```
Hasil kompilasi akan berada di `./ai-gateway` (ukuran ~25 MB).

### 3. Menjalankan Gateway
```bash
./ai-gateway
```
Saat dijalankan pertama kali, Route-X secara otomatis mengeksekusi migrasi skema database, menanam data peran (RBAC), hak akses, dan akun administrator awal.

Akses dashboard pada:
- **Dashboard Web**: `http://localhost:8080/`
- **Dokumentasi API**: `http://localhost:8080/docs`
- **Metrik Prometheus**: `http://localhost:8080/metrics`
- **Pemeriksaan Liveness**: `http://localhost:8080/healthz`
- **Pemeriksaan Readiness**: `http://localhost:8080/readyz`

---

## Penggunaan Klien (Client SDK)

Klien dapat langsung terhubung ke Route-X hanya dengan mengubah `base_url` dan melampirkan Client API Key yang dibuat melalui konsol admin.

### 1. Menggunakan cURL
```bash
curl -X POST http://localhost:8080/v1/chat/completions \
  -H "Authorization: Bearer sk_live_your_api_key" \
  -H "Content-Type: application/json" \
  -d '{
    "model": "gpt-5",
    "messages": [
      {"role": "user", "content": "Jelaskan keunggulan Route-X Gateway!"}
    ],
    "temperature": 0.7
  }'
```

### 2. Menggunakan Python (OpenAI SDK)
```python
from openai import OpenAI

client = OpenAI(
    base_url="http://localhost:8080/v1",
    api_key="sk_live_your_api_key"
)

response = client.chat.completions.create(
    model="gpt-5",
    messages=[
        {"role": "user", "content": "Halo Route-X!"}
    ],
    stream=True
)

for chunk in response:
    if chunk.choices[0].delta.content:
        print(chunk.choices[0].delta.content, end="", flush=True)
print()
```

### 3. Menggunakan Node.js / TypeScript (OpenAI SDK)
```typescript
import OpenAI from "openai";

const openai = new OpenAI({
  baseURL: "http://localhost:8080/v1",
  apiKey: "sk_live_your_api_key",
});

async function main() {
  const completion = await openai.chat.completions.create({
    model: "gpt-5",
    messages: [{ role: "user", content: "Halo dari TypeScript!" }],
  });

  console.log(completion.choices[0].message.content);
}

main();
```

---

## Penerapan dengan Docker & Docker Compose (Opsi Kontainer)

Route-X menyediakan konfigurasi Docker multi-stage dan Docker Compose siap pakai yang memaketkan Gateway, PostgreSQL 16, dan Redis 7.

### 1. Menjalankan Docker Compose Mandiri (Pengembangan Lokal)
```bash
# Salin konfigurasi environment docker
cp .env.docker.example .env
# Lengkapi variabel rahasia di .env

# Jalankan seluruh stack (Route-X Gateway + PostgreSQL 16 + Redis 7)
docker compose up -d --build
```
Layanan akan aktif di `http://localhost:8080` (lingkungan development).

### 2. Menjalankan Docker Compose Produksi (dengan Caddy TLS)
Untuk mengaktifkan otomatisasi sertifikat HTTPS via Let's Encrypt / ZeroSSL dengan port 8080 tertutup rapat dari akses luar:
```bash
# Pastikan .env memiliki APP_ENV=production dan PUBLIC_URL=https://<domain>
docker compose -f docker-compose.yml -f docker-compose.prod.yml up -d
```

---

## Penerapan Produksi Linux (Bare-Metal & VPS)

Tersedia dua opsi konfigurasi penerapan langsung pada server Linux:

### Opsi A: Skrip Otomasi Terpadu (Direkomendasikan)
Gunakan skrip otomatisasi di [`deploy/scripts/setup_production.sh`](deploy/scripts/setup_production.sh):
```bash
# Build biner terlebih dahulu
make build

# Jalankan pemasangan otomatis
sudo ./deploy/scripts/setup_production.sh ./ai-gateway

# Lengkapi nilai rahasia di /etc/route-x/route-x.env
sudo nano /etc/route-x/route-x.env

# Nyalakan layanan
sudo systemctl start route-x
sudo systemctl status route-x
```

### Opsi B: Pemasangan Manual Systemd
Unit layanan systemd yang telah diperkeras (*security-hardened*) tersedia di [`deploy/systemd/route-x.service`](deploy/systemd/route-x.service).

1. Pasang biner dan berkas konfigurasi templat produksi (JANGAN salin `.env` lokal):
   ```bash
   sudo cp ./ai-gateway /usr/local/bin/ai-gateway
   sudo mkdir -p /etc/route-x /var/lib/route-x
   sudo cp deploy/production.env.example /etc/route-x/route-x.env
   sudo chown -R root:root /etc/route-x
   sudo chmod 600 /etc/route-x/route-x.env
   # Buka dan lengkapi rahasia mandiri:
   sudo nano /etc/route-x/route-x.env
   ```

2. Buat pengguna sistem `routex`:
   ```bash
   sudo useradd -r -s /bin/false -d /var/lib/route-x routex
   sudo chown -R routex:routex /var/lib/route-x
   ```

3. Aktifkan layanan systemd:
   ```bash
   sudo cp deploy/systemd/route-x.service /etc/systemd/system/route-x.service
   sudo systemctl daemon-reload
   sudo systemctl enable --now route-x
   sudo systemctl status route-x
   ```

---

## Konfigurasi Reverse Proxy & Streaming SSE

Untuk menyajikan gateway di balik domain publik dengan sertifikat TLS dan memastikan streaming inferensi teks (Server-Sent Events) tidak mengalami buffering:

### 1. Caddy (Otomatis HTTPS Let's Encrypt)
Konfigurasi Caddy siap pakai tersedia di [`deploy/caddy/Caddyfile`](deploy/caddy/Caddyfile). Bagian terpenting adalah parameter `flush_interval -1` yang menonaktifkan buffer respon streaming:
```caddy
gateway.example.com {
    reverse_proxy 127.0.0.1:8080 {
        flush_interval -1
    }
}
```

### 2. Nginx
Konfigurasi Nginx siap pakai tersedia di [`deploy/nginx/route-x.conf`](deploy/nginx/route-x.conf):
```nginx
location / {
    proxy_pass http://127.0.0.1:8080;
    proxy_http_version 1.1;
    proxy_set_header Host $host;
    proxy_set_header X-Real-IP $remote_addr;
    proxy_set_header X-Forwarded-For $proxy_add_x_forwarded_for;

    # KRITIS untuk streaming SSE (Server-Sent Events)
    proxy_buffering off;
    proxy_cache off;
    chunked_transfer_encoding on;
    proxy_read_timeout 600s;
}
```

---

## Verifikasi Kualitas & Pengujian

Route-X menerapkan 4 gerbang verifikasi kualitas kode ketat sebelum setiap perubahan di-commit:

```bash
# 1. Format kode Go
make fmt

# 2. Analisis statis go vet
make vet

# 3. Seluruh pengujian konkurensi dengan race detector
make test

# 4. Uji Beban Ringan Konkurensi
./scripts/load_test.sh http://127.0.0.1:8080 500 20

# 5. Verifikasi Menyeluruh End-to-End
./scripts/e2e_verify.sh
```

---

## Struktur Direktori Repositori

```
Route-X/
├── cmd/
│   └── ai-gateway/             # Titik masuk aplikasi utama biner tunggal
├── deploy/
│   └── systemd/                # Unit konfigurasi layanan systemd Linux
├── docs/
│   ├── openapi.yaml            # Spesifikasi resmi OpenAPI 3.1.0
│   ├── embed.go                # Handler penyaji dokumentasi interaktif Scalar
│   └── PLAN.md                 # Rencana kerja & catatan sejarah tiap fase
├── internal/
│   ├── admin/                  # REST API administrasi & manajemen gateway
│   ├── apikey/                 # Autentikasi API Key klien & extraction
│   ├── auth/                   # Autentikasi sesi konsol, CSRF, Argon2id
│   ├── billing/                # Pengawasan anggaran & kontrol biaya
│   ├── cache/                  # Klien Redis & skrip atomik
│   ├── config/                 # Parser konfigurasi lingkungan (.env)
│   ├── contentfilter/          # Mesin penyaring konten masukan & keluaran
│   ├── database/               # Driver pgxpool, migrasi PostgreSQL, seed
│   ├── gateway/                # Reverse proxy, codec wire-format, circuit breaker
│   ├── health/                 # Probe kesehatan /healthz dan /readyz
│   ├── httpx/                  # Utilitas HTTP, SPAHandler, envelope galat
│   ├── observability/          # Metrik Prometheus & logger terstruktur
│   ├── providers/              # Adapter OpenAI, Anthropic, Gemini, Ollama
│   ├── ratelimit/              # Pembatas laju kuota terdistribusi
│   ├── router/                 # Strategi perutean inferensi & aturan failover
│   ├── security/               # Cipher AES-256-GCM, SSRFPolicy, hashing
│   ├── usage/                  # Pencatat pemakaian token & kalkulator latensi
│   ├── webhooks/               # Dispatcher event bertanda tangan HMAC-SHA256
│   └── worker/                 # Supervisor background worker singleton
├── scripts/
│   ├── load_test.sh            # Skrip uji beban konkurensi & throughput
│   └── e2e_verify.sh           # Skrip verifikasi end-to-end menyeluruh
├── web/                        # Aplikasi SPA Frontend (React 18, Vite, Tailwind)
└── Makefile                    # Otomasi pengujian, formatting, dan build
```

---

## Lisensi

Hak Cipta © 2026 NexGen-X Engineering. Seluruh hak dilindungi undang-undang.
