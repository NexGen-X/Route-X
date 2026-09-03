# Prompt serah-terima Fase 10 — Worker

Untuk dijalankan agen lain (Antigravity CLI). Disusun 2026-09-03 setelah Fase 9 ditutup dan
di-push (`7f86bb8`). Isinya diverifikasi terhadap kode dan skema yang benar-benar ada di
repo pada saat itu, bukan terhadap rencana saja.

Setiap tempat yang semula menyerahkan pilihan ke pelaksana sudah diubah menjadi keputusan.
Itu disengaja: keputusan-keputusan ini punya arah kesalahan yang tidak simetris, dan
menyerahkannya ke agen berarti separuh kemungkinan hasilnya harus dibongkar lagi. Kalau
salah satunya ternyata tidak bisa dijalankan, LAPORKAN beserta sebabnya — jangan diganti
diam-diam.

---

Lanjutkan pembangunan Route-X di /root/Route-X (repo NexGen-X/Route-X, branch main).
Kerjakan Fase 10 secara otonom sampai tuntas.

## Baca dulu

docs/PLAN.md adalah rencana yang berlaku. Baca bagian "Fase 10" DAN seluruh bagian
"Catatan hasil Fase N" sebelum menulis satu baris kode — catatan itu memuat keputusan
yang mengikat fase ini beserta alasannya, dan beberapa di antaranya berlawanan dengan
tebakan yang wajar.

Baca juga komentar KEPALA di tiga migrasi ini. Ketiganya bukan dokumentasi tambahan,
melainkan spesifikasi: mereka memuat query yang harus dipakai worker beserta indeks
yang dibuat khusus untuk query itu.
  - internal/database/migrations/0008_automation.sql, blok komentar di atas
    webhook_deliveries — memuat query pengambilan antrean (for update skip locked) dan
    query pemulihan sewa yang tertinggal, lengkap dengan alasannya.
  - internal/database/migrations/0006_requests.sql, bagian partisi awal — menyebut
    kewajiban worker memperingatkan bila partisi DEFAULT tidak kosong.
  - internal/database/migrations/0007_usage.sql, kepala berkas — aturan persentil dan
    kontrak rollup "hitung ulang lalu timpa, bukan menambah selisih".

Setelah fase selesai, docs/PLAN.md yang harus diperbarui.

## Fase 10 — Worker

Supervisor errgroup dengan start/stop bersih dan pg_advisory_lock untuk job singleton:
health checker provider, penjadwal rollup usage, cleanup retensi, pemeliharaan partisi,
reset periode anggaran, dan pengiriman webhook (queue + backoff + HMAC).

Webhook TETAP di fase ini, tetapi produsen event-nya dibatasi enam event saja — lihat
aturan 10. Alasan ia tetap di sini: mesinnya (sewa, pemulihan baris tersangkut, backoff yang
tahan restart) adalah mesin yang sama dengan job lain di fase ini, dan dua fase sebelumnya
sudah meninggalkan catatan yang menunjuk ke Fase 10 untuknya —
`billing.Enforcer.Announce` sampai sekarang hanya menulis log dan doc-nya menyebut Fase 10
sebagai tempat pengirimannya.

## Yang SUDAH ada — jangan dibangun ulang

- traffic.Repo.RollupHourly / RollupDaily / RollupRange (Fase 9). Idempoten: menimpa,
  bukan menambah, jadi aman dijalankan berulang untuk bucket yang sama. Penjadwal TIDAK
  boleh menulis SQL rollup sendiri.
- traffic.Repo.Summary / Series / Breakdown / LatencyByProviderModel — sisi baca agregasi.
- upstream.ProviderRepo.RecordHealth(ctx, providerID, HealthReport) — memperbarui cuplikan
  di providers DAN menulis riwayat ke health_checks dalam SATU pernyataan, plus semantik
  consecutive_failures yang sudah diputuskan ('healthy' mereset, 'unhealthy' menaikkan,
  'degraded' tidak mengubah). Jangan menulis ulang salah satunya.
- providers.Provider.HealthCheck(ctx) HealthResult — dipenuhi setiap adapter, dan sengaja
  TIDAK mengembalikan error karena kegagalan ADALAH hasilnya.
- gateway.Factory.Provider(ctx, *upstream.RouteCandidate) — pabrik adapter bercache.
- security.NewCipher + security.CredentialAAD — pola AAD yang harus diikuti untuk secret
  webhook.
- policy.Repo.ResetPeriod (sudah melepas alerted_at — jangan dirusak), MarkAlerted,
  PurgeExpired (bans kedaluwarsa).
- billing.Enforcer.Announce — SATU-SATUNYA jalur peringatan ambang anggaran saat ini;
  inilah titik yang tepat untuk memproduksi event budget.threshold.
- observability.Metrics sudah punya WorkerRunsTotal, WorkerDuration, ProviderUp, dan
  ProviderLatencyMS. Keempatnya TERDAFTAR TETAPI BELUM ADA YANG MENGISINYA; Fase 10 yang
  mengisinya.
- config.Config sudah punya HealthCheckInterval, UsageRollupInterval,
  RequestLogRetentionDays, RequestBodyRetentionDays.
- Fungsi SQL create_monthly_partition(parent text, month date) ada di migrasi 0006 dan
  belum punya satu pun pemanggil Go.
- repo.InTx untuk transaksi.

Perhatikan: internal/worker dan internal/webhooks SUDAH ADA sebagai direktori tetapi
KOSONG. Begitu juga internal/admin dan internal/models. Keberadaan direktorinya bukan
bukti ada pekerjaan di dalamnya.

## Aturan yang WAJIB dipatuhi

1. Kunci pg_advisory_lock worker TIDAK BOLEH bertabrakan dengan kunci migrasi
   0x524F55544558 di internal/database/migrate.go. Runner migrasi memegang kunci itu
   selama seluruh migrasi berjalan; worker yang memakai kunci sama akan memblokir start
   atau diblokir olehnya. Pilih ruang kunci sendiri, dan tulis alasannya di dekat
   konstantanya.

2. Kegagalan satu job TIDAK BOLEH mematikan proses atau job lain. errgroup bawaan
   membatalkan seluruh saudaranya pada error pertama, dan itu justru kebalikan dari yang
   dibutuhkan: satu health check yang gagal tidak boleh menghentikan pengiriman webhook.
   Setiap job menahan errornya sendiri, mencatatnya, menaikkan
   routex_worker_runs_total{worker,outcome}, lalu mencoba lagi pada tick berikutnya. Hanya
   kegagalan PERAKITAN yang boleh merambat ke pemanggil. Job yang panic juga harus
   dipulihkan — worker yang mati karena satu baris data rusak adalah worker yang berhenti
   diam-diam.

3. ROLLUP DULU, RETENSI KEMUDIAN. Menghitung ulang jam yang baris mentahnya sudah dibuang
   retensi TIDAK menolkan baris agregatnya, ia membiarkan nilai lama apa adanya. Urutan
   yang salah menghasilkan angka yang terlihat wajar dan tidak bisa dilacak lagi.
   Penjadwal rollup juga harus menghitung ulang JENDELA BELAKANG, bukan hanya jam
   berjalan: request bisa datang terlambat, dan menghitung ulang itu murah serta idempoten.

4. Retensi memakai DUA mekanisme, dan pembagiannya sudah ditetapkan:

    a. requests, request_events, dan usage_hourly dibersihkan dengan MEMBUANG PARTISI, dan
       hanya partisi yang SELURUH rentangnya lebih tua daripada batas retensi. Konsekuensi
       yang harus ditulis di komentar dan di plan: data tersimpan sampai hampir satu bulan
       lebih lama daripada REQUEST_LOG_RETENTION_DAYS. Itu diterima, dan alternatifnya
       tidak: DELETE atas tabel terbesar di sistem meninggalkan tabel membengkak yang
       menuntut VACUUM FULL, membanjiri WAL, dan berjalan tepat pada jam tersibuk.
    b. request_payloads dibersihkan dengan PENGHAPUSAN BERBATCH, karena
       REQUEST_BODY_RETENTION_DAYS bawaannya 7 hari — lebih pendek daripada satu partisi
       bulanan, jadi membuang partisi tidak akan pernah memenuhi batas itu. Batasi ukuran
       batch dan beri jeda antar batch: tabel ini memuat body, jadi barisnya besar dan satu
       DELETE tanpa batas akan menahan kunci lebih lama daripada yang pantas di jalur mana
       pun. Riwayat webhook_deliveries yang sudah 'delivered' atau 'abandoned' dibersihkan
       dengan cara yang sama; indeks partial webhook_deliveries_created_idx sudah dibuat
       untuk query itu.

5. Pemeliharaan partisi WAJIB memperingatkan bila partisi DEFAULT tidak kosong. Migrasi
   0006 menyebutkan alasannya: memasang partisi baru untuk rentang yang barisnya sudah
   mendarat di DEFAULT menuntut pemindahan baris, dan itu tidak akan terjadi sendiri.

6. Antrean webhook memakai query yang sudah ditulis di migrasi 0008 apa adanya:
   "for update skip locked" untuk pengambilan, dan update pemulihan untuk baris yang
   tertinggal di status 'delivering' setelah worker mati. Indeks partial
   webhook_deliveries_due_idx dan _stuck_idx dibuat persis untuk kedua query itu — pola
   pengambilan yang lain akan melewatkan indeksnya tanpa satu pun tanda. Constraint
   webhook_deliveries_lease_consistent menuntut sewa dibersihkan di pernyataan yang SAMA
   dengan perubahan statusnya.

   webhooks.consecutive_failures DINAIKKAN dan direset, tetapi worker TIDAK BOLEH
   menonaktifkan endpoint secara otomatis walau kolomnya menyebut kemungkinan itu. Endpoint
   yang mati sepuluh menit karena deploy-nya sendiri akan dimatikan gateway lalu tetap mati
   sampai ada manusia yang sadar — dan saluran yang baru saja dimatikan itu justru saluran
   yang dipakai untuk memberi tahu. Angkanya dicatat dan dilaporkan; keputusan mematikan
   milik operator.

7. Backoff webhook disimpan di next_attempt_at, bukan ditahan di memori: itu yang membuat
   antrean tetap benar setelah worker restart. Pakai jitter, dengan alasan yang sama
   seperti backoff upstream di internal/gateway/executor.go — baca bagian itu dulu.

8. Secret webhook terenkripsi AES-256-GCM dan AAD-nya WAJIB terikat ke id webhook.
   security.CredentialAAD adalah polanya; tambahkan padanannya untuk webhook. Format
   ciphertext dijaga constraint webhooks_secret_format: v1.<key_id>.<base64url>.

9. webhook_deliveries.payload DISIMPAN PERMANEN. Nilai sensitif wajib sudah tersamar
   sebelum masuk — API key, prompt pengguna, atau isi jawaban di dalamnya adalah kebocoran
   yang tidak bisa ditarik kembali. Begitu juga error_message di webhook_deliveries dan
   health_checks: jangan pernah menulis error mentah klien HTTP ke sana, karena URL bisa
   memuat token di query string.

10. Fase ini memproduksi TEPAT ENAM event, tidak lebih:

        provider.unhealthy   provider.recovered    (dari health checker fase ini)
        budget.threshold     budget.exceeded       (dari billing.Enforcer.Announce)
        circuit.opened       circuit.closed        (dari gateway.Breaker)

    Ketiga pasangan itu dipilih karena informasinya SUDAH ada di tangan pada titik itu,
    volumenya terikat jumlah provider dan jumlah anggaran — bukan jumlah permintaan — dan
    ketiganya peristiwa yang memang ingin diketahui operator pada saat terjadi.

    JANGAN memproduksi request.completed maupun request.failed. Keduanya sah menurut
    constraint webhooks_events_known, dan justru itu bahayanya: satu webhook yang
    melangganinya menghasilkan SATU BARIS webhook_deliveries PER PERMINTAAN INFERENSI, di
    tabel yang sengaja TIDAK dipartisi dan barisnya diperbarui berkali-kali. Volumenya sama
    dengan tabel requests, tanpa satu pun mekanisme yang menahannya.

    JANGAN memproduksi ratelimit.exceeded maupun content.blocked. Alasannya sama tetapi
    lebih buruk: keduanya paling sering terjadi justru saat sedang ada serangan atau
    penyalahgunaan, yaitu saat beban tulis tambahan paling tidak diinginkan. Keduanya sudah
    terhitung di routex_rate_limit_rejected_total dan routex_content_filter_blocked_total,
    dan alert dari metrik tidak menambah satu baris pun ke database.

    JANGAN memproduksi api_key.created, api_key.revoked, maupun ban.created: ketiganya
    peristiwa MUTASI ADMIN, dan mutasi admin baru ada di Fase 11. Memproduksinya sekarang
    berarti menebak bentuk payload sebelum pemanggilnya ada.

    Langganan '*' tetap sah dan tidak perlu diperlakukan khusus — ia menerima enam event
    yang benar-benar diproduksi, bukan yang belum ada. Tulis di plan event mana yang masih
    belum punya produsen, supaya operator tidak menyangka langganannya bekerja.

11. Reset periode anggaran butuh query BARU: policy.Repo.ActiveBudgets justru
    MENGECUALIKAN anggaran yang period_end-nya sudah lewat, jadi tidak ada satu pun jalur
    yang saat ini bisa menemukannya. Saat mereset, hitung ulang spent_usd dari
    requests.cost_usd yang berskala 8 — jangan percaya angka akumulasinya, karena kolom
    budgets.spent_usd berskala 6 sehingga setiap AddSpend dibulatkan PostgreSQL.

12. Nilai uang memakai upstream.USD (bilangan bulat satuan 1e-8), tidak pernah float64.

13. Health checker provider BUKAN milik internal/health. Paket itu adalah endpoint
    liveness/readiness dan SENGAJA tidak mengimpor internal/database maupun internal/cache
    — doc paketnya menjelaskan alasannya. Pemeriksaan provider butuh repository provider
    dan pabrik adapter, jadi tempatnya di internal/worker.

14. gateway.Factory.Provider menerima *upstream.RouteCandidate, bukan baris provider,
    karena ia dibangun untuk jalur routing per pemetaan model. Health check per provider
    tidak selalu punya pemetaan model, jadi TAMBAHKAN jalan masuk baru di pabrik yang
    menerima apa yang benar-benar dibutuhkannya dan memakai jalur pembuatan, cache, serta
    penjagaan SSRF yang SAMA.

    Jangan menyusun *upstream.RouteCandidate palsu dari baris provider. Nilai
    ProviderModelID dan UpstreamModelName yang dikarang akan ikut ke log dan ke jejak
    diagnostik pabrik, dan pengenal karangan di sana adalah hal yang berikutnya dicari
    orang lalu tidak ditemukan di tabel mana pun.

    Jangan membuat http.Client kedua di luar pabrik. Penjaga SSRF di lapisan dial dan
    dukungan egress pool hidup di sana; klien kedua berarti health check menempuh jalur
    keluar yang berbeda dari lalu lintas sungguhan, sehingga ia bisa melaporkan sehat untuk
    endpoint yang justru tidak bisa dihubungi jalur produksi — kegagalan health check yang
    paling mahal, karena ia meyakinkan.

15. Persentil untuk rentang GABUNGAN wajib lewat usage_histogram_sum +
    usage_histogram_percentile. Terukur dan tercatat di plan: 1.000 sampel cepat + 10
    sampel lambat menghasilkan p95 sebenarnya 80 ms, rata-rata p95 per bucket 30.040 ms,
    histogram gabungan 88,79 ms.

16. repo.Error sengaja tidak membawa pesan driver: Detail dan Hint PostgreSQL bisa memuat
    nilai baris.

## Bahasa dan gaya

Balasan ke saya dan SELURUH komentar kode, nama test, serta pesan error dalam bahasa
Indonesia. Komentar menjelaskan ALASAN sebuah keputusan diambil dan apa yang rusak kalau
diambil sebaliknya — komentar yang cuma menerjemahkan nama fungsi adalah komentar yang
salah. Nama identifier dan commit message tetap Inggris.

Baca internal/gateway/guard.go, internal/ratelimit/ratelimit.go, dan
internal/database/repo/traffic/rollup.go dulu untuk merasakan gayanya.

## Penutup fase

1. gofmt, go vet, go test -race ./... (pakai `make race` — target itu memuat .env;
   `go test` langsung TIDAK, dan tanpa .env seluruh test integrasi melewati dirinya
   sendiri lalu terlihat hijau palsu. Suite butuh -p 1). Lalu `make build`.
2. Verifikasi end-to-end pada biner sungguhan, bukan hanya unit test. Minimal: rollup
   benar-benar mengisi usage_hourly dan usage_daily dari lalu lintas nyata, health check
   menulis health_checks dan mengisi routex_provider_up, partisi bulan depan terbuat,
   retensi membuang yang seharusnya dan TIDAK membuang yang lain, satu webhook benar-benar
   terkirim dengan tanda tangan yang bisa diverifikasi, dan pengiriman yang gagal
   dijadwalkan ulang lalu akhirnya abandoned.
3. Perbarui docs/PLAN.md: tabel status (Fase 10 ✅, Fase 11 ⏳), bagian "Catatan hasil
   Fase 10" berisi keputusan yang mengikat fase berikutnya beserta alasannya, dan daftar
   batas yang diketahui dan sengaja dibiarkan terbuka.
4. Commit dan push ke origin main.

## Catatan lingkungan

- Go ada di /usr/local/go/bin. PostgreSQL 16 dan Redis hidup di mesin ini.
- `grep` dan `find` adalah fungsi shell yang membungkus ugrep. Pakai `command grep` dan
  `command find` untuk perilaku GNU yang bisa diramalkan.
- `pkill -f ai-gateway` MEMBUNUH shell pemanggilnya sendiri, karena command line shell itu
  memuat pola yang dicari; gejalanya perintah berhenti dengan exit 143/144 di tengah
  jalan. Pakai `pkill -x ai-gateway`. Untuk menyalakan biner yang bertahan setelah
  perintah selesai, tulis skrip kecil yang memuat .env lalu `exec ./ai-gateway`, dan
  jalankan dengan `setsid skrip.sh > log 2>&1 < /dev/null &`.
- Belum ada API admin sampai Fase 11, jadi baris uji disisipkan dengan tangan. Resep yang
  terbukti: upstream palsu berdialek OpenAI di 127.0.0.1; provider kind
  `openai_compatible` (kind itu sah TANPA kredensial, jadi tidak perlu merakit ciphertext
  AES); `api_keys` kolomnya key_prefix dan last4, dan scopes harus subset dari
  {inference, models:read, usage:read, admin:read, admin:write}; key_hash = HMAC-SHA256
  hex dari key mentah memakai API_KEY_PEPPER yang sudah DIDEKODE base64; jalankan biner
  dengan UPSTREAM_ALLOW_HTTP=true UPSTREAM_ALLOWED_PRIVATE_ADDRS=127.0.0.1,::1 — tanpa
  keduanya penjaga SSRF memblokir setiap upstream loopback. Tabel kebijakan di-cache 5
  detik, jadi beri jeda 6 detik setelah menyisipkan baris. Bersihkan seluruh baris uji dan
  kunci Redis setelahnya.
- Untuk membuat baris yang butuh ciphertext AES (provider_credentials, webhooks): tulis
  program sekali pakai di direktori berawalan `_` (go tool mengabaikannya, jadi tidak ikut
  ./...), panggil repository-nya lewat cipher aplikasi, jalankan `go run ./_dir/main.go`,
  lalu HAPUS direktorinya sebelum commit.
- Perkakas e2e yang Anda buat WAJIB ditaruh di scripts/e2e/ dan ikut di-commit, bukan
  dibuang setelah dipakai: upstream palsu, penyisip baris uji, penyala/penghenti biner, dan
  pembersih. Sejauh ini setiap fase membangunnya ulang dari nol, dan Fase 11, 12, serta 14
  membutuhkan yang sama. Skrip penghenti memakai `pkill -x`, bukan `pkill -f` — lihat
  catatan di atas.

## Batas terbuka dari fase sebelumnya

Jangan diam-diam mengubahnya; kalau menyentuhnya, catat di plan.
- providers.max_retries tidak dipakai jalur request (temuan Fase 7). JANGAN ditutup di fase
  ini: ia soal jalur permintaan, bukan worker, dan menutupnya menuntut satu keputusan yang
  harus DITULIS lebih dulu — mana yang menang ketika routing_rules.max_attempts dan
  providers.max_retries menyebut angka berbeda. Menyimpulkan jawabannya dari kode yang
  kebetulan ditulis lebih dulu adalah cara mendapat perilaku yang tidak pernah dipilih siapa
  pun.
- Ambang pemutus arus per aturan routing belum tersalurkan (Fase 6). Kolom
  failure_threshold bertipe hitungan, bukan rasio, sementara gateway.Breaker memakai
  rasio; JANGAN menulis FailureThreshold: float64(rule.FailureThreshold) — 5 akan menjadi
  rasio 5,0 yang dijepit ke 1,0, yaitu breaker yang praktis mati tanpa satu pun tanda.
- Penyaring konten tidak berlaku pada jawaban streaming (Fase 8).
- Moderasi konten eksternal belum didukung; ia menunggu tabel integrations, yang sampai
  sekarang belum punya satu pun repository. JANGAN ditutup di fase ini, dan tabel
  integrations dibiarkan tanpa repository. Alasannya bukan kekurangan waktu: moderasi
  eksternal menambah panggilan jaringan DI DALAM jalur permintaan, jadi ia menuntut
  keputusan tentang arah kegagalan (lolos atau blokir saat penyedia moderasi tidak
  menjawab) dan anggaran waktunya. Fase 8 sudah menetapkan arah kegagalan yang tidak
  simetris untuk penyaring lain — daftar hitam yang tidak bisa dievaluasi MELOLOSKAN,
  daftar putih MEMBLOKIR — dan moderasi eksternal harus mendapat perlakuan setara, bukan
  ditempelkan sebagai pekerjaan sampingan di fase worker. Fase worker yang menambah
  panggilan jaringan ke jalur permintaan adalah cara paling rapi untuk mendapat moderasi
  yang gagal-lolos tanpa ada yang memutuskannya.
- request_payloads belum punya penulis; keputusan penangkapan body ditunda ke Fase 12
  (Fase 9).
- Penolakan yang terjadi di middleware (401, dan 429 cakupan key/pengguna/IP/global) tidak
  masuk tabel requests (Fase 9).
- routex_http_* dan routex_worker_* terdaftar tanpa pengisi; yang kedua menjadi tugas fase
  ini.
