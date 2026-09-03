# Pedoman dan Aturan Kerja Agen Route-X

Dokumen ini berlaku untuk seluruh subagent dan asisten yang bekerja pada repositori Route-X.

## 1. Konvensi Bahasa
- **Komentar kode, pesan error, dan dokumentasi internal**: Wajib dalam bahasa Indonesia. Komentar harus menjelaskan *mengapa* sebuah keputusan teknis diambil dan dampak jika diambil sebaliknya.
- **Nama identifier (variabel, struct, interface, function)** dan **pesan commit git**: Tetap dalam bahasa Inggris.

## 2. Standar Kualitas & Arsitektur
- **Presisi Keuangan**: Nilai moneter selalu menggunakan `upstream.USD` (integer skala 8 desimal / 1e-8 USD), tidak pernah `float64`.
- **Advisory Locks**: Pekerjaan singleton atau migrasi wajib memakai PostgreSQL advisory lock (`pg_advisory_lock`) pada koneksi tingkat sesi mandiri untuk mencegah tabrakan konkurensi.
- **Isolasi Kegagalan**: Background worker tidak boleh mematikan proses utama saat terjadi error atau panic. Setiap job wajib menangkap panic dan mencatat kegagalan ke metrik Prometheus `routex_worker_*`.
- **SSRF & Keamanan**: Seluruh komunikasi ke upstream wajib melalui `security.SSRFPolicy` dan cipher enkripsi AES-256-GCM dengan AAD yang diikat ke entitas terkait.

## 3. Gerbang Verifikasi Sebelum Selesai
Sebelum menandai tugas selesai atau melakukan commit:
1. `make fmt`
2. `make vet`
3. `go test -race ./...`
4. `make build`
