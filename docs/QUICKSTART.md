# Route-X Quickstart & Developer Integration Guide

Dokumentasi ini memandu pengembang dalam mengintegrasikan aplikasi dengan **Route-X AI Gateway** (`https://api.your-domain.com`). Route-X menyediakan endpoint yang kompatibel dengan protokol **OpenAI API** dan **Anthropic Messages API**, sehingga Anda dapat menggunakan pustaka resmi OpenAI (Python, Node.js), SDK Anthropic, LangChain, LlamaIndex, atau cURL dengan perubahan konfigurasi minimal (base URL + API key).

---

## 🖥️ 1. Akses Konsol Web & Setup Admin Pertama

Setelah gateway dinyalakan, buka konsol web di peramban:
`http://localhost:8080/login` (atau domain produksi Anda).

- **Instalasi Baru (First-Run)**: Tampilan login dilengkapi kartu bantuan **"SETUP AWAL"** dengan tombol pintasan untuk mengisi alamat email admin secara otomatis.
- **Kredensial Bawaan**: Tidak ada password default. Akun admin pertama hanya dibuat bila `INITIAL_ADMIN_PASSWORD` disetel sebelum start pertama; installer resmi menghasilkan password acak unik dan menampilkannya sekali di akhir instalasi.
  - Email: nilai `INITIAL_ADMIN_EMAIL` (bawaan `admin@routex.local`)
  - Password: nilai `INITIAL_ADMIN_PASSWORD` yang Anda setel / hasil acak installer.
- **Pengalihan Wajib**: Pada login pertama, Anda akan langsung diarahkan untuk membuat kata sandi baru. Setelah kata sandi diperbarui, kartu bantuan setup akan dinonaktifkan secara otomatis.

---

## 🔑 2. Autentikasi Klien API

Seluruh permintaan inferensi ke gateway wajib menyertakan kunci API klien (*Client API Key*) yang dibuat melalui menu **Kunci API** di konsol administrasi (`#/access/api-keys`).

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
| `https://api.your-domain.com/v1/responses` | `POST` | OpenAI | **Alias** dari Chat Completions (handler yang sama), bukan implementasi penuh OpenAI Responses API. |
| `https://api.your-domain.com/v1/messages` | `POST` | Anthropic | Protokol Anthropic Messages (Claude Code, Cursor, dll.). |
| `https://api.your-domain.com/v1/messages/count_tokens` | `POST` | Anthropic | Estimasi jumlah token sebuah pesan (tokenizer lokal). |
| `https://api.your-domain.com/v1/models` | `GET` | OpenAI | Daftar model yang benar-benar memiliki rute aktif, beserta aliasnya (berpaginasi `limit`/`after_id`). |
| `https://api.your-domain.com/v1/models/{model}` | `GET` | OpenAI | Detail satu model. |
| `https://api.your-domain.com/v1/embeddings` | `POST` | OpenAI | Vektorisasi teks untuk RAG / Pencarian Semantik. |

Varian `/v1/v1/messages` dan `/v1/v1/messages/count_tokens` juga diterima sebagai alias.

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
    model="gpt-5",  # Atau alias model lain yang terdaftar di Route-X
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
    model="gpt-5",
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
    model: 'gpt-5',
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
    "model": "gpt-5",
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
    "model": "gpt-5",
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
    model="gpt-5",
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
    model="gpt-5",
    api_base="https://api.your-domain.com/v1",
    api_key="sk_live_xxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxx"
)

response = llm.complete("Apa itu RAG?")
print(response.text)
```

---

### F. Anthropic Claude Protocol (`/v1/messages`)

Route-X menerima protokol Anthropic Messages dan menerjemahkannya ke bentuk kanonik internal sebelum meneruskannya ke upstream (bukan passthrough mentah), sehingga perkakas seperti Claude Code, Cursor, atau Anthropic SDK dapat terhubung langsung. Block `document` didukung dengan batas berikut: dokumen teks (`text/plain`, termasuk base64) diteruskan sebagai teks ke semua upstream; dokumen PDF base64 diteruskan sebagai block asli hanya ke upstream Anthropic (tanpa kandidat Anthropic, request ditolak `400 unsupported_document_upstream`); dokumen bersumber URL ditolak — kirim isinya sebagai teks atau base64. Batas ukuran satu dokumen 10 MiB sesudah dekode base64:

```bash
curl -X POST https://api.your-domain.com/v1/messages \
  -H "x-api-key: sk_live_xxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxx" \
  -H "anthropic-version: 2023-06-01" \
  -H "Content-Type: application/json" \
  -d '{
    "model": "claude-sonnet-5",
    "max_tokens": 1024,
    "messages": [
      {"role": "user", "content": "Halo Claude via Route-X!"}
    ]
  }'
```

---

## ⚡ 5. Fitur Cerdas Gateway

1. **Model Aliasing**:
   Anda dapat memakai nama model katalog (mis. `gpt-5`, `claude-sonnet-5`) atau alias kustom yang didaftarkan operator di konsol. Route-X memetakan alias tersebut ke model riil di upstream provider.
2. **Combo Routing (Failover & Load Balancing)**:
   Jika provider utama mengalami gangguan (*timeout*, *rate limit*, atau *out of balance*), Route-X secara otomatis mengalirkan permintaan ke provider cadangan (*cascading failover*) tanpa disadari oleh aplikasi klien.
3. **Tracking Latensi & Biaya Real-Time**:
   Setiap respons menyertakan header pelacakan:
   - `X-Request-Id`: ID pelacakan unik permintaan untuk penelusuran di menu **Permintaan**.
   - `X-RateLimit-Limit`, `X-RateLimit-Remaining`, `X-RateLimit-Reset`: batas, sisa, dan waktu reset kuota rate limit kunci Anda (dipasang pada respons yang lolos maupun yang ditolak).

---

## 🛑 6. Penanganan Error Standar

Route-X mengembalikan format error standar yang seragam:

| Kode HTTP | Error Code | Penyebab & Solusi |
|:---:|---|---|
| `400` | `invalid_request_error` / `content_filter` | Isi permintaan tidak sah, atau ditolak filter konten. Periksa kembali payload Anda. |
| `401` | `invalid_api_key` | Kunci API salah, dicabut (*revoked*), kadaluarsa, atau pemiliknya dinonaktifkan. Buat kunci baru di konsol. |
| `402` | `budget_exceeded` | Pagu anggaran yang berlaku untuk kunci/model/provider ini habis atau ditahan reservasi lain. Hubungi admin untuk menaikkan pagu. |
| `403` | `provider_not_allowed` | Kunci API tidak diizinkan memakai provider tujuan permintaan ini. Sesuaikan izin provider pada kunci. |
| `404` | `model_not_found` | Model tidak dikenal atau tidak memiliki rute aktif. Periksa `GET /v1/models`. |
| `429` | `rate_limit_exceeded` | Batas laju terlampaui (kunci Anda, atau seluruh provider untuk model ini sedang membatasi laju). Tunggu sesaat — perhatikan header `X-RateLimit-Reset`. |
| `502` | `upstream_error` / `upstream_auth_failed` / `upstream_permission_denied` / `upstream_quota_exhausted` | Kegagalan di sisi provider atau kredensial gateway ke provider. Ini bukan soal API key Anda — hubungi operator gateway. |
| `503` | `upstream_overloaded` | Seluruh kandidat provider untuk model ini sedang penuh atau ter-skip pemutus arus. Coba lagi beberapa saat lagi. |
| `504` | `upstream_timeout` | Upstream provider tidak menjawab sebelum batas waktu. Failover otomatis berlaku bila ada kandidat lain. |

Permukaan Anthropic (`/v1/messages`) mengembalikan error yang sama dalam envelope Anthropic (`{"type":"error","error":{...}}`).
