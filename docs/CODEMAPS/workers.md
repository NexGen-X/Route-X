# Background Workers Architecture Codemap

**Last Updated:** 2026-09-06  
**Area:** Background Worker Supervisor & Singleton Schedulers  
**Entry Points:**
- [`internal/worker/worker.go`](file:///root/Route-X/internal/worker/worker.go) — Supervisor engine, advisory locking, panic isolation

---

## Architecture

```
+-----------------------------------------------------------------------------------------+
|                               Worker Supervisor Architecture                            |
|                                                                                         |
|       Supervisor Loop (Bootstrapped in cmd/ai-gateway/main.go)                          |
|                               |                                                         |
|                               v                                                         |
|  +-----------------------------------------------------------------------------------+  |
|  |                PostgreSQL Advisory Lock (pg_try_advisory_lock)                    |  |
|  |     Lock ID: 0x5258574F ("RXWO") -> Memastikan hanya 1 instance yang aktif        |  |
|  +-----------------------------------+-----------------------------------------------+  |
|                                      |                                                  |
|                     (Jika lock didapat, nyalakan goroutines)                             |
|                                      |                                                  |
|         +----------------------------+----------------------------+                     |
|         |                            |                            |                     |
|         v                            v                            v                     |
|  +---------------+          +-----------------+          +-----------------+            |
|  | Health Checker|          |  Usage Rollup   |          | Retention Clean |            |
|  | Interval: 30s |          |  Interval: 5m   |          | Interval: 1h    |            |
|  | Probe upstream|          |  Hourly & daily |          | Purge old logs  |            |
|  +---------------+          +-----------------+          +-----------------+            |
|         |                            |                            |                     |
|         v                            v                            v                     |
|  +---------------+          +-----------------+          +-----------------+            |
|  | Partition Mgt |          | Budget Resetter |          | Webhook Worker  |            |
|  | Interval: 12h |          | Interval: 1h    |          | Interval: 2s    |            |
|  | Create tables |          | Reset spent_usd |          | SKIP LOCKED     |            |
|  +---------------+          +-----------------+          +-----------------+            |
|                                                                                         |
+-----------------------------------------------------------------------------------------+
```

---

## Background Worker Jobs

Setiap worker dibungkus fungsi isolasi `defer func() { if r := recover(); r != nil { ... } }` sehingga kegagalan satu job tidak akan pernah menyebabkan aplikasi atau proses gateway mengalami *crash*.

| Worker Job | File Sumber | Interval Default | Tugas & Algoritma |
| :--- | :--- | :--- | :--- |
| **Health Checker** | [`health.go`](file:///root/Route-X/internal/worker/health.go) | 30 Detik | Menguji konektivitas probe ringan ke upstream provider. Mengubah status `is_healthy` di database dan memicu penyesuaian perutean adaptif jika provider down. |
| **Usage Rollup** | [`rollup.go`](file:///root/Route-X/internal/worker/rollup.go) | 5 Menit | Merekam agregasi pemakaian token masukan/keluaran, request total, dan biaya ke tabel `usage_hourly` dan `usage_daily` untuk mempercepat query analitik dashboard. |
| **Retention Cleaner** | [`retention.go`](file:///root/Route-X/internal/worker/retention.go) | 1 Jam | Menghapus log request mentah dan jejak audit lama yang telah melampaui batas waktu retensi konfigurasi untuk menghemat kapasitas disk PostgreSQL. |
| **Partition Maintainer** | [`partition.go`](file:///root/Route-X/internal/worker/partition.go) | 12 Jam | Memeriksa dan membuat partisi tabel `requests` masa depan secara preemptive sehingga penulisan data inferensi bervolume tinggi tidak terputus. |
| **Budget Resetter** | [`budget.go`](file:///root/Route-X/internal/worker/budget.go) | 1 Jam | Memeriksa siklus kalender anggaran bulanan dan mereset nilai `spent_usd` pada saat pergantian periode penagihan. |
| **Webhook Worker** | [`webhook.go`](file:///root/Route-X/internal/worker/webhook.go) | 2 Detik | Mengambil event tertunda menggunakan `SELECT ... FOR UPDATE SKIP LOCKED` dan mengeksekusi pengiriman payload dengan exponential jitter backoff. |

---

## Graceful Lifecycle

- **Start:** Diluncurkan secara asynchronous di goroutine latar belakang saat server HTTP dinyalakan.
- **Stop:** Merespons sinyal `SIGINT`/`SIGTERM` via `context.Context`. Supervisor menunggu job yang sedang aktif menyelesaikan operasinya (drain timeout 15 detik), kemudian melepas advisory lock `RXWO` di PostgreSQL.

---

## Related Areas
- [Backend Architecture Codemap](file:///root/Route-X/docs/CODEMAPS/backend.md)
- [Database Architecture Codemap](file:///root/Route-X/docs/CODEMAPS/database.md)
- [Integrations Codemap](file:///root/Route-X/docs/CODEMAPS/integrations.md)
