# Checklist Audit Frontend (web/)

Baca file sungguhan sebelum mencentang butir apa pun. Cek `tsconfig.json`, `vite.config.ts`, `tailwind.config.js/ts`, dan struktur folder `src/` lebih dulu untuk konteks sebelum menyelami komponen satu-satu.

## 1. TypeScript (5.6)

- **Strict mode**: cek `tsconfig.json` — apakah `"strict": true` aktif? Tanpa ini, banyak celah type-safety (implicit any, null checks) lolos tanpa terdeteksi.
- **Pemakaian `any`**: grep penggunaan `any` eksplisit — tiap kemunculan adalah titik di mana type-safety sengaja dimatikan; nilai wajar untuk ada beberapa (misalnya integrasi library pihak ketiga tanpa tipe), tapi kalau tersebar luas di logic bisnis itu tanda type modeling kurang matang.
- **Tipe untuk response API**: apakah ada tipe/interface eksplisit untuk data dari backend (idealnya sinkron dengan struct Go di sisi server), atau data API diperlakukan sebagai `any`/`unknown` tanpa validasi runtime?

## 2. React 18 (18.3.1)

- **Aturan Hooks**: hooks harus dipanggil di top-level, tidak di dalam kondisi/loop. Cek juga dependency array `useEffect`/`useMemo`/`useCallback` — array yang tidak lengkap adalah sumber bug stale-closure yang sangat umum dan sering lolos review biasa.
- **Key pada list**: pastikan key di `.map()` adalah ID stabil dari data, bukan index array (index sebagai key menyebabkan bug state yang salah tempat saat list berubah urutan).
- **Error boundary**: apakah ada Error Boundary di level yang wajar (misalnya per-route) supaya satu komponen error tidak mem-blank-kan seluruh halaman?
- **Concurrent features React 18**: kalau dipakai `useTransition`/`useDeferredValue`/Suspense, cek apakah dipakai pada kasus yang tepat (update non-urgent, data fetching dengan suspense boundary) bukan sekadar ditempel tanpa perlu.
- **Memoization berlebihan vs kurang**: `useMemo`/`useCallback` yang dipasang di semua tempat tanpa profiling adalah over-engineering yang menambah kompleksitas tanpa manfaat jelas; sebaliknya, komponen berat (misalnya yang merender chart Recharts dengan data besar) yang re-render tanpa memoization sama sekali patut dicurigai sebagai masalah performa.

## 3. Vite (5.4.9)

- **Environment variable**: apakah secret/API key sensitif pernah masuk ke `import.meta.env.VITE_*`? Semua variabel berprefix `VITE_` di-bundle ke kode client dan bisa dibaca siapa pun — hanya boleh berisi konfigurasi publik (base URL API, feature flag), bukan API key rahasia.
- **Code splitting**: apakah route-route utama pakai `React.lazy()`/dynamic import, atau seluruh aplikasi jadi satu bundle besar yang memperlambat initial load?
- **Build config**: cek `vite.config.ts` untuk alias path, plugin (misalnya `@vitejs/plugin-react`), dan target build yang sesuai kebutuhan browser support.

## 4. Tailwind CSS (3.4.14) + tailwind-merge & clsx

- **Konsistensi utility**: apakah ada pola berulang panjang (misalnya kombinasi class untuk "card" atau "button") yang di-copy-paste di banyak file alih-alih diekstrak jadi komponen atau util lewat `clsx`/`tailwind-merge`?
- **Pemakaian `tailwind-merge`**: cek apakah komponen yang menerima `className` sebagai prop menggabungkannya dengan `twMerge(...)` agar override class tidak konflik (misalnya default `p-4` vs prop `p-2` — tanpa `twMerge`, Tailwind bisa menerapkan urutan class yang salah karena CSS specificity/urutan sama).
- **Arbitrary values**: penggunaan `[...]` arbitrary value berlebihan (`w-[137px]`) menandakan kemungkinan design token belum didefinisikan dengan baik di `tailwind.config` — bandingkan dengan spacing/color scale yang sudah ada.
- **Dark mode & responsive**: kalau project mendukung dark mode, cek konsistensi prefix `dark:` diterapkan merata, tidak hanya di sebagian komponen.

## 5. TanStack Query / React Query (5.59)

- **Query key**: apakah query key terstruktur konsisten (array dengan bagian variabel di posisi yang jelas, misalnya `['orders', orderId]`), atau string/array ad-hoc yang gampang typo dan menyebabkan cache tidak pernah match/invalidate dengan benar?
- **staleTime & gcTime**: apakah nilai default (staleTime 0) dibiarkan untuk data yang jarang berubah (menyebabkan refetch berlebihan), atau disesuaikan per jenis data?
- **Invalidasi setelah mutasi**: setelah `useMutation` sukses, apakah `queryClient.invalidateQueries()` (atau optimistic update) dipanggil dengan key yang tepat supaya UI tidak menampilkan data basi?
- **Error & loading state**: apakah tiap pemakaian `useQuery` menangani `isError`/`isLoading` di UI, atau hanya mengandalkan `data` yang bisa `undefined` dan menyebabkan crash render?
- **Waterfall fetching**: cek apakah ada query yang saling bergantung dipanggil berurutan padahal bisa paralel (`useQueries` atau query independen), yang memperlambat time-to-interactive.
- **Refetch behavior**: cek konfigurasi `refetchOnWindowFocus`/`refetchOnReconnect` — default aktif bisa menyebabkan request berlebihan untuk dashboard yang sering di-switch tab.

## 6. Recharts

- **ResponsiveContainer**: apakah chart dibungkus `ResponsiveContainer` agar menyesuaikan ukuran parent, atau pakai lebar/tinggi hardcoded yang pecah di layar kecil?
- **Performa data besar**: untuk dataset besar (misalnya metrik time-series granular), cek apakah ada downsampling/agregasi sebelum di-render — Recharts bisa lambat kalau merender ribuan titik data mentah-mentah.
- **Aksesibilitas**: apakah chart punya alternatif teks (misalnya tabel data tersembunyi atau `aria-label`) untuk pembaca layar, karena chart SVG biasanya tidak accessible secara default?
- **Re-render saat data streaming**: kalau dashboard live/real-time, cek apakah update data menyebabkan seluruh chart re-mount (kedip) alih-alih update halus.

## 7. Lucide React

- **Tree-shaking**: pastikan import icon spesifik (`import { Home } from 'lucide-react'`), bukan import keseluruhan library, supaya bundle size tidak membengkak.
- **Aksesibilitas icon**: icon yang berfungsi sebagai tombol tanpa teks (icon-only button) harus punya `aria-label` pada elemen pembungkus; icon dekoratif murni sebaiknya `aria-hidden`.

## 8. Praktik umum frontend

- **Struktur folder & layering**: apakah ada pemisahan jelas antara komponen UI, hooks data-fetching (TanStack Query), dan lapisan API client, atau logic bercampur di satu file besar?
- **Sanitasi output**: kalau ada konten dari user/backend yang dirender sebagai HTML (`dangerouslySetInnerHTML`), cek apakah disanitasi untuk mencegah XSS.
- **Aksesibilitas umum**: label form, kontras warna Tailwind yang dipakai, navigasi keyboard untuk elemen interaktif custom.
- **Testing**: apakah ada unit test komponen (Testing Library) dan/atau e2e (Playwright/Cypress) untuk alur kritis?
