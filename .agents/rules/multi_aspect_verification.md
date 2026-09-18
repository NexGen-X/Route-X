# Standar Verifikasi Multi-Aspek & Protokol Anti-Halusinasi

## Konteks & Prinsip Wajib
Dilarang keras menyajikan hasil mentah, asumsi teoretis, atau klaim penyelesaian tugas hanya berdasarkan spekulasi kode atau mock lokal semata.
Semua klaim hasil pekerjaan, integrasi provider luar, maupun perbaikan bug wajib dibuktikan dengan bukti empiris nyata melalui 4 lapisan verifikasi.

## 4 Lapisan Verifikasi Sebelum Melaporkan Hasil

1. **Wire & Network Protocol (Protokol Jaringan Nyata)**:
   - Wajib membedah dan menguji wire protocol nyata (menggunakan `curl` atau probe langsung) sebelum dan sesudah koding.
   - Periksa status code riil, payload JSON, dan header penting (`cf-ray`, `x-request-id`).
   - Bedakan secara tegas antara Forward Proxy (L4/L7 HTTP CONNECT tunnel) dengan Reverse Proxy (L7 BaseURL routing).
   - Uji mode streaming (SSE framing: `data: ...\n\n`) dan non-streaming secara terpisah.

2. **Backend Architecture & Database Integrity**:
   - `go test -v ./...` wajib 100% PASS (zero failure).
   - Verifikasi data nyata di database PostgreSQL via SQL riil: periksa skema kolom, constraint, dan enkripsi AES-256-GCM pada data rahasia.
   - Periksa status layanan systemd (`routex.service`) dan endpoint probe `/healthz` & `/readyz`.

3. **Frontend & API Contract Interface**:
   - Kompilasi TypeScript `npm --prefix web run build` wajib bersih tanpa type error.
   - Penuhi kontrak arsitektur DTO (`adalahDTO()`) agar tidak membocorkan data sensitif ke browser.
   - Pastikan perlindungan CSRF dan cookie sesi bekerja dengan benar.

4. **Live Ground-Truth End-to-End**:
   - Uji alur utuh (*roundtrip*): Klien -> Gateway Route-X -> Upstream Pihak Ketiga -> Klien.
   - Sajikan data metrik riil: angka latensi nyata (ms), IP keluar publik (*exit IP*), dan lokasi datacenter PoP yang terdeteksi.
