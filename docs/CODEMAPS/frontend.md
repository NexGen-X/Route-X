# Frontend Architecture Codemap

**Last Updated:** 2026-09-06  
**Area:** Frontend Single Page Application (SPA)  
**Entry Points:**
- [`web/src/main.tsx`](file:///root/Route-X/web/src/main.tsx) — Root application bootstrapping with TanStack Query provider
- [`web/src/App.tsx`](file:///root/Route-X/web/src/App.tsx) — Main layout, router view switcher, Command Palette, Toast notifications
- [`web/src/api/client.ts`](file:///root/Route-X/web/src/api/client.ts) — Typed REST API client communicating with backend `/api/admin`

---

## Architecture

```
+-----------------------------------------------------------------------------------------+
|                                    React 18 SPA Architecture                             |
|                                                                                         |
|  +-----------------------------------------------------------------------------------+  |
|  |                            web/src/main.tsx                                       |  |
|  |   QueryClientProvider (staleTime: 5m, retry: 1, refetchOnWindowFocus: false)      |  |
|  +-----------------------------------+-----------------------------------------------+  |
|                                      |                                                  |
|                                      v                                                  |
|  +-----------------------------------------------------------------------------------+  |
|  |                             web/src/App.tsx                                       |  |
|  |   AuthProvider  -->  ToastProvider  -->  AppLayout (Sidebar, Header, ViewSwitcher) |  |
|  +-----------------------------------+-----------------------------------------------+  |
|                                      |                                                  |
|         +----------------------------+----------------------------+                     |
|         |                                                         |                     |
|         v                                                         v                     |
|  +---------------------------------------+       +-----------------------------------+  |
|  |              Page Views               |       |         UI Component Base         |  |
|  |  APIKeys.tsx      (TanStack useQuery) |       |  cn() (clsx + tailwind-merge)     |  |
|  |  Providers.tsx    (TanStack useQuery) |       |  Button.tsx (focus ring & state)  |  |
|  |  Dashboard.tsx    (Stats & Recharts)  |       |  Card.tsx (bordered dark cards)   |  |
|  |  RoutingRules.tsx (Pipeline Canvas)   |       |  Badge.tsx (variants & chips)     |  |
|  |  Requests.tsx     (Waterfall latency) |       |  Drawer.tsx, Modal.tsx, Select    |  |
|  |  Egress.tsx       (Xray proxies)      |       |  CommandPalette.tsx (Cmd+K)       |  |
|  +-------------------+-------------------+       +-----------------+-----------------+  |
|                      |                                             |                    |
|                      v                                             |                    |
|  +---------------------------------------+                         |                    |
|  |       web/src/api/client.ts           |                         |                    |
|  |  Typed fetch() with CSRF & Session    |                         |                    |
|  +-------------------+-------------------+                         |                    |
|                      |                                             |                    |
+----------------------|---------------------------------------------|--------------------+
                       |                                             |
                       v                                             v
       Backend API (/api/admin/*)                   Tailwind CSS Utility Classes
```

---

## Key Modules & Patterns

### 1. Server State Management: TanStack Query v5
Sesuai [ADR 0001](file:///root/Route-X/docs/adr/0001-use-tanstack-query-for-data-fetching.md), seluruh sinkronisasi data server dimigrasikan ke **TanStack Query (`useQuery`, `useMutation`, `useQueryClient`)** menggantikan polling atau `useEffect` manual.

```tsx
// Query fetching deklaratif
const queryClient = useQueryClient();
const { data: keysData, isLoading } = useQuery({
  queryKey: ['apiKeys'],
  queryFn: () => api.apiKeys.list(),
});

// Mutasi dan invalidasi cache deklaratif
await api.apiKeys.create(...);
queryClient.invalidateQueries({ queryKey: ['apiKeys'] });
```

### 2. Styling System: Tailwind `cn()` Utility
Pola modular styling Tailwind distandarisasi menggunakan utilitas `cn()` di [`web/src/utils/cn.ts`](file:///root/Route-X/web/src/utils/cn.ts) menggabungkan `clsx` dan `tailwind-merge` untuk resolusi konflik kelas secara deterministik.

| File Komponen | Peran | Penggunaan `cn()` |
| :--- | :--- | :--- |
| [`web/src/utils/cn.ts`](file:///root/Route-X/web/src/utils/cn.ts) | Core merge helper | `export function cn(...inputs: ClassValue[]) { return twMerge(clsx(inputs)); }` |
| [`web/src/components/common/Button.tsx`](file:///root/Route-X/web/src/components/common/Button.tsx) | Button interaktif | Menggabungkan varian (`primary`, `secondary`, `danger`, `ghost`), ukuran, loader, dan fokus ring |
| [`web/src/components/common/Card.tsx`](file:///root/Route-X/web/src/components/common/Card.tsx) | Wadah container | `cn('bg-bg-surface border border-border rounded-card overflow-hidden', className)` |
| [`web/src/components/common/Badge.tsx`](file:///root/Route-X/web/src/components/common/Badge.tsx) | Status chip | `cn('inline-flex items-center gap-1 font-medium rounded-chip', variants[v], className)` |
| [`web/src/components/common/Drawer.tsx`](file:///root/Route-X/web/src/components/common/Drawer.tsx) | Slide-out drawer | Backdrop blur & transisi slide-in untuk konfigurasi provider & detail request |

### 3. Modul Halaman (Pages)

| Modul Halaman | Jalur File | Sumber Data / Fitur Utama |
| :--- | :--- | :--- |
| **APIKeys** | [`web/src/pages/APIKeys.tsx`](file:///root/Route-X/web/src/pages/APIKeys.tsx) | Manajemen client API key, rotasi instan, pencabutan hak akses, limit RPM/TPM |
| **Providers** | [`web/src/pages/Providers.tsx`](file:///root/Route-X/web/src/pages/Providers.tsx) | 23 AI provider presets, pengujian latensi live, sinkronisasi model hulu, drawer detail |
| **Dashboard** | [`web/src/pages/Dashboard.tsx`](file:///root/Route-X/web/src/pages/Dashboard.tsx) | Metrik real-time, volume request, ringkasan biaya token USD, throughput |
| **RoutingRules** | [`web/src/pages/RoutingRules.tsx`](file:///root/Route-X/web/src/pages/RoutingRules.tsx) | Aturan failover, pipeline visualizer, pembobotan provider |
| **Egress** | [`web/src/pages/Egress.tsx`](file:///root/Route-X/web/src/pages/Egress.tsx) | Pool proxy keluar, konfigurasi tunnel stealth Xray (Reality, VMess, Trojan) |
| **Requests** | [`web/src/pages/Requests.tsx`](file:///root/Route-X/web/src/pages/Requests.tsx) | Request inspector, waterfall latensi TTFT, jejak event perutean, payload viewer |
| **Observability** | [`web/src/pages/Observability.tsx`](file:///root/Route-X/web/src/pages/Observability.tsx) | Grafik Prometheus, latensi persentil P50/P90/P99, status circuit breaker |

---

## Data Flow

1. Pengguna membuka antarmuka; `App.tsx` memeriksa status sesi via `AuthContext`.
2. Halaman memicu `useQuery` dengan query key unik (misal `['apiKeys']`, `['providers']`).
3. `api/client.ts` mengirim HTTP request ke backend `/api/admin/*` dengan cookie sesi dan header `X-CSRF-Token`.
4. Hasil respons di-*cache* oleh TanStack Query dengan `staleTime: 5 menit`. Perpindahan tab/halaman instan tanpa refetch berlebihan.
5. Saat pengguna mengubah data (buat key, putar rahasia, simpan konfigurasi), mutasi memicu `queryClient.invalidateQueries(...)`, memperbarui state secara deklaratif.

---

## External Dependencies

| Dependensi | Versi | Tujuan |
| :--- | :--- | :--- |
| `react` / `react-dom` | `^18.3.1` | UI Library inti |
| `@tanstack/react-query` | `^5.59.0` | Asynchronous state management & caching |
| `tailwind-merge` | `^2.5.4` | Deterministic Tailwind class conflict resolution |
| `clsx` | `^2.1.1` | Conditional className composer |
| `lucide-react` | `^0.453.0` | Ikonografi UI |
| `recharts` | `^2.13.0` | Grafik visualisasi performa dan analitik |
| `@fontsource/inter` | `^5.3.0` | Tipografi Sans-serif (tersemat lokal) |
| `@fontsource/jetbrains-mono` | `^5.3.0` | Tipografi Monospace (tersemat lokal) |

---

## Build & Testing Commands

```bash
# Direktori frontend
cd web

# 1. Jalankan development server lokal
npm run dev

# 2. Type-check dan bundle untuk produksi
npm run build

# 3. Jalankan pengujian end-to-end browser (Playwright)
npx playwright test
```

## Related Areas
- [Backend Architecture Codemap](file:///root/Route-X/docs/CODEMAPS/backend.md)
- [ADR 0001: TanStack Query Adoption](file:///root/Route-X/docs/adr/0001-use-tanstack-query-for-data-fetching.md)
