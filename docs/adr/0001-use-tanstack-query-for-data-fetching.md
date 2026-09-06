# 1. Gunakan TanStack Query untuk Data Fetching di Frontend

Tanggal: 2026-09-06

## Status
Diterima (Accepted)

## Konteks
Sebelumnya, komponen React di frontend (seperti `APIKeys.tsx` dan `Providers.tsx`) menggunakan pola manual `useState` dan `useEffect` untuk mengambil data dari backend. Pendekatan ini rentan terhadap *race condition*, re-fetching yang tidak perlu pada setiap perpindahan halaman, tidak mendukung *caching* secara bawaan, dan membutuhkan boilerplate kode yang banyak untuk menangani status `isLoading` dan error. 
Pustaka `@tanstack/react-query` sudah terinstal di `package.json` namun belum diimplementasikan.

## Keputusan
Kita akan secara eksklusif menggunakan **TanStack Query (`useQuery`, `useMutation`)** untuk seluruh manajemen state *server* (data fetching) di React. 
Penggunaan `useEffect` mentah untuk sinkronisasi data dari API tidak lagi diperbolehkan.

## Konsekuensi
- **Positif:** Manajemen state otomatis (stale-while-revalidate), perlindungan dari multiple fetch di waktu yang sama, *boilerplate* jauh lebih sedikit, UX lebih mulus.
- **Positif:** Mudah memicu pembaruan data secara deklaratif dengan `queryClient.invalidateQueries()`.
- **Negatif:** Semua engineer yang baru bergabung perlu mempelajari siklus *cache* milik TanStack Query.
