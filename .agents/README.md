# `.agents/` — Konteks Kerja Internal Agen (BUKAN Dokumentasi Pengguna)

> ⚠️ **Seluruh isi folder ini adalah konteks kerja internal untuk AI agent yang bekerja di repositori Route-X.** Ini bukan dokumentasi pengguna, bukan dokumentasi produk, dan bukan janji fitur. Pembaca manusia yang mencari dokumentasi Route-X harus mulai dari [`README.md`](../README.md) dan [`docs/`](../docs/).
>
> **Aturan menang:** bila isi berkas mana pun di folder ini bertentangan dengan kode sumber atau [`AGENTS.md`](../AGENTS.md) repo, **kode dan `AGENTS.md` yang menang**. Fakta keras di folder ini terakhir diselaraskan ke kode pada 3 Nov 2026 (PR perombakan dokumentasi PR-4); sesudah itu, verifikasi ulang ke kode sebelum mengandalkan angka apa pun di sini.

## Isi folder

### `prompts/` — 7 master prompt spesialis

| Berkas | Peran |
| :--- | :--- |
| `orchestrator_master_prompt.md` | Lead System Orchestrator — koordinasi & dekomposisi tugas |
| `backend_router_master_prompt.md` | Backend Router Specialist — inti gateway Go |
| `frontend_uiux_master_prompt.md` | Frontend UI/UX Architect — dashboard Calm Console |
| `qa_audit_master_prompt.md` | Independent QA Auditor — Protokol 7-Layer QA |
| `devops_release_master_prompt.md` | Release Delivery & DevOps Specialist — PR-only & rilis |
| `database_architect_master_prompt.md` | Database Architect — migrasi & PostgreSQL |
| `security_auditor_master_prompt.md` | Security & Red Team Auditor |
| `MASTER_PROMPT_FRAMEWORK.md` | Kerangka anatomi master prompt + format handoff 5 seksi |

### `rules/` — 3 berkas aturan

| Berkas | Isi |
| :--- | :--- |
| `routex-dev.md` | Aturan pengembangan & standar kerja (paling dekat ke kode; bila ragu, berkas ini + `AGENTS.md` yang dipegang) |
| `multi_agent_orchestration.md` | Tata kelola orkestrasi multi-agent + katalog peran |
| `multi_aspect_verification.md` | Standar verifikasi multi-aspek & anti-halusinasi |

## Catatan keselarasan (3 Nov 2026)

- **Hitungan agen:** katalog di `rules/multi_agent_orchestration.md` mendaftar **25 peran** (2.1–2.25). Itu taksonomi peran *aspiratif* untuk pendelegasian tugas, bukan daftar agen yang berjalan; master prompt spesialis yang benar-benar ada adalah **7 berkas** di `prompts/` di atas. Penyebutan "13 sub-agent" di berkas yang sama sebelumnya adalah sisa versi lama katalog dan sudah dikoreksi.
- **Fakta keras yang diselaraskan ke kode:** Go **1.27.3** (`go.mod`), React **19** + Node **24** (`web/package.json`, `Dockerfile`), PostgreSQL **16**, migrasi sebanyak jumlah berkas di `internal/database/migrations/` (sumber kebenaran hitungannya, agar tidak basi di tiap migrasi), font UI **Noto Sans** (mono: JetBrains Mono) dengan token Calm Console, tabel log permintaan bernama **`requests`**, `/v1/responses` berstatus permukaan **OpenAI Responses API asli** subset non-streaming (B-105; bukan alias Chat Completions), nama cookie sesi `routex_session` (prefiks `__Host-` hanya pada produksi/HTTPS), target aksesibilitas **WCAG 2.1 AA**, dan model **single-admin** (RBAC dihapus pada migrasi 0010).
- **Yang sengaja TIDAK dijadikan klaim:** standar "WCAG 2.2+", "Adaptive Contrast", peran edge/WASM & realtime WebRTC, dan kompatibilitas absolut "100%" — semuanya aspirasi lama tanpa dasar di kode dan sudah dilabeli/dihapus dari berkas terkait.
