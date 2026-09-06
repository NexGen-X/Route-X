---
name: route-x-audit
description: Audit teknis mendalam untuk project "Route-X" dengan stack Go (Chi v5, pgx v5, go-redis v9, Prometheus client_golang) di backend dan React 18/TypeScript (Vite, Tailwind CSS, TanStack Query, Recharts, Lucide React) di frontend. Gunakan skill ini setiap kali pengguna minta audit, code review, security review, performance review, atau "cek kesehatan" kode di project Route-X atau project lain yang memakai kombinasi stack ini — baik untuk backend saja, frontend saja, atau full-stack. Juga pakai skill ini kalau pengguna menyebut nama file seperti go.mod atau package.json dari project ini dan minta ditinjau, dievaluasi, atau dicari masalahnya. Trigger juga untuk permintaan seperti "review router Chi saya", "cek koneksi pool pgx", "audit query TanStack Query", atau "apakah setup Redis saya aman".
---

# Audit Route-X

Skill ini memandu audit teknis menyeluruh untuk project Route-X: backend Go (Chi v5 + pgx v5 + puddle + go-redis v9 + Prometheus client_golang) dan frontend React 18/TypeScript (Vite + Tailwind CSS + TanStack Query + Recharts + Lucide React).

Tujuan audit ini bukan sekadar mencari bug, tapi menilai apakah kode sudah memakai masing-masing library sesuai idiom dan best practice-nya — karena bug tersamar paling sering muncul dari pemakaian library yang "asal jalan" tapi menyalahi pola yang dimaksudkan (misalnya connection pool pgx yang tidak pernah di-Close, query key TanStack Query yang tidak konsisten sehingga cache basi, atau label Prometheus dengan kardinalitas tak terbatas yang lama-lama membebani memori).

## Alur kerja

1. **Kumpulkan konteks dulu, jangan menebak.** Sebelum menulis temuan apa pun, baca kode sungguhan:
   - Baca `go.mod` untuk versi Go dan dependency backend.
   - Baca `package.json` (dan `package-lock.json`/`pnpm-lock.yaml` bila ada) untuk versi frontend.
   - Susuri struktur folder (`view` direktori project) untuk memahami organisasi sebelum membaca file satu-satu.
   - Kalau project diunggah sebagai arsip/beberapa file, baca isinya lewat tool komputer — jangan berasumsi dari nama file saja.
   - Kalau ada bagian project yang tidak diberikan (misalnya user hanya kasih folder backend), audit yang tersedia saja dan sebutkan secara eksplisit bagian mana yang tidak tercakup — jangan mengarang temuan untuk kode yang tidak dilihat.

2. **Pilih ruang lingkup.** Kalau permintaan pengguna tidak jelas apakah audit backend, frontend, atau keduanya, tentukan dari file yang tersedia (ada `go.mod` → backend relevan; ada `package.json` dengan React → frontend relevan) alih-alih bertanya balik, kecuali benar-benar ambigu.

3. **Jalankan checklist per domain.** Baca file referensi yang relevan sebelum menulis temuan, karena masing-masing berisi butir-butir spesifik untuk library terkait beserta alasan kenapa tiap butir penting:
   - `references/backend-go.md` — Chi v5, pgx v5 + puddle, go-redis v9, Prometheus client_golang, plus praktik umum Go (error handling, context propagation, concurrency, security).
   - `references/frontend-react.md` — TypeScript, React 18, Vite, Tailwind CSS (+ tailwind-merge/clsx), TanStack Query, Recharts, Lucide React, plus praktik umum frontend.

4. **Cross-check versi.** Bandingkan versi yang tertulis di `go.mod`/`package.json` dengan versi yang disebut pengguna (Go 1.27, Chi v5, pgx v5, go-redis v9, TypeScript 5.6, React 18.3.1, Vite 5.4.9, Tailwind 3.4.14, TanStack Query 5.59). Kalau versi di file berbeda dari yang disebutkan, catat itu sebagai temuan info-level, bukan diam-diam dikoreksi — user perlu tahu ada drift antara ekspektasi dan kenyataan. Jika sebuah versi tidak dikenal atau tampak tidak ada di riwayat rilis publik, sebutkan itu apa adanya tanpa berasumsi ini kesalahan ketik pengguna.

5. **Susun temuan dengan bukti, bukan tebakan.** Setiap temuan harus merujuk ke file dan baris/fungsi spesifik dari kode yang benar-benar dibaca. Kalau ragu apakah sesuatu benar-benar masalah (misalnya pola yang terlihat aneh tapi mungkin sengaja), tandai sebagai "perlu konfirmasi" alih-alih menuduh pasti salah.

6. **Tulis laporan sesuai template di bawah.**

## Format laporan audit

Selalu gunakan struktur berikut. Gunakan bahasa yang sama dengan permintaan pengguna (default Bahasa Indonesia kalau permintaan dalam Bahasa Indonesia).

```markdown
# Audit Route-X — [Backend / Frontend / Full-stack]
Tanggal: [tanggal]
Ruang lingkup: [file/folder yang diaudit, dan yang TIDAK diaudit bila ada]

## Ringkasan eksekutif
[3-5 kalimat: kondisi umum, risiko terbesar, dan apakah project ini secara umum siap produksi]

## Temuan berdasarkan tingkat keparahan

### 🔴 Kritis (berpotensi insiden produksi / kebocoran data / downtime)
- **[Judul temuan]** — `path/file.go:baris` atau `path/File.tsx`
  - Apa yang terjadi: ...
  - Kenapa ini masalah: ...
  - Rekomendasi: ...

### 🟠 Penting (bug laten, technical debt signifikan, atau risiko performa)
[format sama]

### 🟡 Perbaikan (code smell, penyimpangan idiom library, maintainability)
[format sama]

### 🔵 Info (drift versi, catatan observasi, saran opsional)
[format sama]

## Ringkasan per komponen stack
Tabel singkat: komponen (Chi router / pgx-puddle / go-redis / Prometheus / React / Vite / Tailwind / TanStack Query / Recharts / Lucide) → status (Baik / Perlu perhatian / Bermasalah) → catatan 1 baris.

## Rekomendasi prioritas
Daftar terurut 3-7 langkah paling berdampak, bukan daftar ulang semua temuan.
```

Jangan mengubah struktur ini kecuali pengguna secara eksplisit minta format lain (misalnya tabel spreadsheet atau slide).

## Menyampaikan hasil

- Kalau laporan panjang (lebih dari sekitar 1-2 layar), buat sebagai file (`.md`, atau `.docx` kalau pengguna eksplisit minta dokumen Word — cek skill `docx` bila demikian) lewat `create_file` ke `/mnt/user-data/outputs`, lalu `present_files`. Jangan tempel laporan penuh ke chat mobile yang panjang.
- Kalau pengguna hanya minta tinjauan cepat satu bagian kecil (misalnya "cek query ini aja"), jawab langsung di chat tanpa perlu file.
- Selalu tutup dengan menawarkan tindak lanjut konkret, misalnya membuat patch untuk temuan kritis, atau memperdalam satu area tertentu — jangan hanya menyerahkan daftar masalah tanpa jalan keluar.
