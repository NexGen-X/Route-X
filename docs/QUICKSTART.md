# Route-X Quickstart & Developer Integration Guide

Dokumentasi ini memandu pengembang dalam mengintegrasikan aplikasi dengan **Route-X AI Gateway** (`https://api.your-domain.com`). Route-X menyediakan endpoint yang 100% kompatibel dengan protokol standar **OpenAI API**, sehingga Anda dapat langsung menggunakan pustaka resmi OpenAI (Python, Node.js), LangChain, LlamaIndex, atau cURL tanpa perlu mengubah kode aplikasi secara signifikan.

---

## 🖥️ 1. Akses Konsol Web & Setup Admin Pertama

Setelah gateway dinyalakan, buka konsol web di peramban:
`http://localhost:8080/login` (atau domain produksi Anda).

- **Instalasi Baru (First-Run)**: Tampilan login dilengkapi kartu bantuan **"SETUP AWAL"** dengan tombol **"Gunakan Kredensial Default (1-Klik)"**.
- **Kredensial Bawaan**:
  - Email: `admin@routex.local`
  - Password: `RouteX#Initial2026!`
- **Pengalihan Wajib**: Pada login pertama, Anda akan langsung diarahkan untuk membuat kata sandi baru. Setelah kata sandi diperbarui, kartu bantuan setup akan dinonaktifkan secara permanen.

---

## 🔑 2. Autentikasi Klien API

Seluruh permintaan inferensi ke gateway wajib menyertakan kunci API klien (*Client API Key*) yang dibuat melalui menu **Client API Keys** di konsol administrasi (`/#/api-keys`).

Gunakan salah satu format header berikut:
```http
Authorization: Bearer sk_live_xxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxx
```
atau:
```http
x-api-key: sk_live_xxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxx
```

---

## 🚀 3. Endpoint Utama

| Endpoint | Metode | Protokol | Deskripsi |
|---|:---:|:---:|---|
| `https://api.your-domain.com/v1/chat/completions` | `POST` | OpenAI | Inferensi percakapan standar OpenAI (streaming SSE & unary). |
| `https://api.your-domain.com/v1/messages` | `POST` | Anthropic | Protokol asli Anthropic Claude (Claude Code, Cursor, dll.). |
| `https://api.your-domain.com/v1/models` | `GET` | OpenAI | Daftar seluruh model AI aktif dan alias yang tersedia. |
| `https://api.your-domain.com/v1/embeddings` | `POST` | OpenAI | Vektorisasi teks untuk RAG / Pencarian Semantik. |

---

## 💻 4. Contoh Integrasi Kode

### A. Python (Official `openai` SDK)

Instal SDK:
```bash
pip install openai
```

Kode Python:
```python
from openai import OpenAI

# Inisialisasi klien dengan Base URL Route-X
client = OpenAI(
    base_url="https://api.your-domain.com/v1",
    api_key="sk_live_xxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxx"
)

# 1. Non-Streaming Chat Completion
response = client.chat.completions.create(
    model="gpt-5.6-terra",  # Atau alias model lain yang terdaftar di Route-X
    messages=[
        {"role": "system", "content": "Anda adalah asisten AI yang cerdas dan solutif."},
        {"role": "user", "content": "Jelaskan arsitektur AI Gateway dalam 2 kalimat."}
    ],
    temperature=0.7,
    max_tokens=200
)

print(response.choices[0].message.content)

# 2. Streaming Chat Completion (Server-Sent Events)
stream = client.chat.completions.create(
    model="gpt-5.6-terra",
    messages=[{"role": "user", "content": "Tuliskan 3 tips produktivitas."}],
    stream=True
)

for chunk in stream:
    if chunk.choices[0].delta.content is not None:
        print(chunk.choices[0].delta.content, end="", flush=True)
print()
```

---

### B. Node.js / TypeScript (Official `openai` npm)

Instal paket:
```bash
npm install openai
```

Kode Node.js:
```typescript
import OpenAI from 'openai';

const client = new OpenAI({
  baseURL: 'https://api.your-domain.com/v1',
  apiKey: 'sk_live_xxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxx',
});

async function main() {
  const completion = await client.chat.completions.create({
    model: 'gpt-5.6-terra',
    messages: [{ role: 'user', content: 'Halo dari Node.js!' }],
  });

  console.log(completion.choices[0].message.content);
}

main().catch(console.error);
```

---

### C. cURL (Shell / Terminal)

```bash
# Non-streaming
curl -X POST https://api.your-domain.com/v1/chat/completions \
  -H "Authorization: Bearer sk_live_xxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxx" \
  -H "Content-Type: application/json" \
  -d '{
    "model": "gpt-5.6-terra",
    "messages": [
      {"role": "user", "content": "Halo Route-X!"}
    ],
    "max_tokens": 100
  }'

# Streaming (SSE)
curl -N -X POST https://api.your-domain.com/v1/chat/completions \
  -H "Authorization: Bearer sk_live_xxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxx" \
  -H "Content-Type: application/json" \
  -d '{
    "model": "gpt-5.6-terra",
    "messages": [
      {"role": "user", "content": "Ceritakan satu lelucon lucu."}
    ],
    "stream": true
  }'
```

---

### D. LangChain (Python)

```python
from langchain_openai import ChatOpenAI

llm = ChatOpenAI(
    model="gpt-5.6-terra",
    openai_api_base="https://api.your-domain.com/v1",
    openai_api_key="sk_live_xxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxx",
    temperature=0.5
)

response = llm.invoke("Sebutkan 3 kelebihan cloud computing.")
print(response.content)
```

---

### E. LlamaIndex (Python)

```python
from llama_index.llms.openai import OpenAI

llm = OpenAI(
    model="gpt-5.6-terra",
    api_base="https://api.your-domain.com/v1",
    api_key="sk_live_xxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxx"
)

response = llm.complete("Apa itu RAG?")
print(response.text)
```

---

### F. Anthropic Claude Protocol (`/v1/messages`)

Route-X menyediakan native passthrough untuk endpoint Anthropic, memungkinkan perkakas seperti Claude Code, Cursor, atau Anthropic SDK terhubung langsung:

```bash
curl -X POST https://api.your-domain.com/v1/messages \
  -H "x-api-key: sk_live_xxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxx" \
  -H "anthropic-version: 2023-06-01" \
  -H "Content-Type: application/json" \
  -d '{
    "model": "claude-3-5-sonnet",
    "max_tokens": 1024,
    "messages": [
      {"role": "user", "content": "Halo Claude via Route-X!"}
    ]
  }'
```

---

## ⚡ 5. Fitur Cerdas Gateway

1. **Model Aliasing**:
   Anda dapat menggunakan alias umum seperti `gpt-4o`, `claude-3-5-sonnet`, atau nama kustom. Route-X akan otomatis memetakan alias tersebut ke model riil di upstream provider.
2. **Combo Routing (Failover & Load Balancing)**:
   Jika provider utama mengalami gangguan (*timeout*, *rate limit*, atau *out of balance*), Route-X secara otomatis mengalirkan permintaan ke provider cadangan (*cascading failover*) tanpa disadari oleh aplikasi klien.
3. **Tracking Latensi & Biaya Real-Time**:
   Setiap respons menyertakan header pelacakan:
   - `x-request-id`: ID pelacakan unik permintaan untuk penelusuran di menu **Requests**.
   - `x-ratelimit-remaining-requests`: Sisa kuota rate limit.

---

## 🛑 6. Penanganan Error Standar

Route-X mengembalikan format error standar yang seragam:

| Kode HTTP | Error Code | Penyebab & Solusi |
|:---:|---|---|
| `401` | `invalid_api_key` | Kunci API salah, dicabut (*revoked*), atau kadaluarsa. Buat kunci baru di konsol. |
| `403` | `insufficient_scope` | Kunci API tidak memiliki scope `inference`. Tambahkan scope di menu API Keys. |
| `429` | `rate_limit_exceeded` | Batas permintaan per detik/menit terlampaui. Tunggu sesaat atau tingkatkan limit. |
| `402` | `budget_exceeded` | Pagu anggaran akun habis. Hubungi admin untuk menambah budget. |
| `504` | `upstream_timeout` | Upstream provider tidak menjawab dalam waktu batas (timeout). Failover akan otomatis aktif jika dikonfigurasi. |
