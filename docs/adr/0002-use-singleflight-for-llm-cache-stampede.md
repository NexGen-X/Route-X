# 2. Gunakan singleflight untuk Mencegah Cache Stampede LLM

Tanggal: 2026-09-06

## Status
Diterima (Accepted)

## Konteks
Permintaan inferensi LLM dapat memakan waktu lama (latensi tinggi). Engine `responsecache` pada Go backend menggunakan Redis untuk menyimpan respons agar permintaan dengan *prompt* identik dapat dilayani lebih cepat.
Jika kunci cache kadaluarsa atau kosong, dan pada saat bersamaan ada lonjakan puluhan/ratusan *request* serupa (Thundering Herd / Cache Stampede), seluruh request tersebut akan *cache miss* dan semuanya diteruskan ke *upstream* LLM secara paralel. Hal ini dapat membebani batas *Rate Limit* upstream dan menghabiskan biaya besar secara mendadak.

## Keputusan
Kita mengimplementasikan pola **Singleflight** (`golang.org/x/sync/singleflight`) pada lapisan cache respons.
Melalui fungsi `GetOrFetch()`, apabila *cache miss* terjadi untuk kunci yang sama, hanya satu permintaan (flight) yang akan dikirim ke *upstream*. Permintaan identik lainnya akan ditahan (blocked) di memori transien Go. Begitu penerbangan pertama mendarat (berhasil mengambil dari *upstream* dan menyimpannya di Redis), semua penunggu akan langsung dikembalikan hasil yang sama dari memori.

## Konsekuensi
- **Positif:** Keamanan infrastruktur upstream sangat terjaga dari beban kejut (*spike*).
- **Positif:** Penghematan dramatis atas tagihan token pada kasus di mana banyak *user* menanyakan hal sama secara bersamaan.
- **Negatif:** Mengonsumsi memori (goroutine tertahan) di level server Go selama masa tunggu, namun ini masih jauh lebih baik dibanding mengonsumsi port keluar dan kuota rate-limit upstream.
