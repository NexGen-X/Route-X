package cache

import (
	"context"
	"errors"
	"fmt"

	"github.com/redis/go-redis/v9"
)

// Skrip Lua adalah cara paket lain mendapatkan operasi atomik di Redis.
//
// Rate limiter perlu "baca penghitung, tambah, pasang TTL kalau baru, putuskan
// tolak/terima" sebagai satu langkah; circuit breaker perlu "catat kegagalan, dan kalau
// ambang terlampaui pindahkan status ke open" sebagai satu langkah. Dipecah menjadi
// beberapa perintah, dua instance gateway yang berjalan bersamaan bisa saling menyalip
// dan melewatkan batas. WATCH/MULTI bisa dipakai, tetapi harus mengulang saat konflik;
// skrip Lua dieksekusi Redis satu per satu sampai selesai sehingga tidak perlu diulang.

// NewScript menyiapkan skrip Lua yang bisa dipakai ulang.
//
// Ini benar-benar hanya penerusan ke redis.NewScript. Gunanya menjaga agar pemakai
// (ratelimit, circuit breaker) cukup bergantung pada paket ini dan tidak mengimpor
// go-redis hanya untuk mendeklarasikan skrip — sekaligus menyimpan satu tempat kalau
// nanti kita perlu membubuhkan instrumentasi atau registri skrip untuk pemanasan cache.
//
// Panggil sekali per skrip pada level paket, bukan per request: nilai yang dikembalikan
// menyimpan digest SHA-1-nya dan aman dipakai bersamaan dari banyak goroutine.
func NewScript(src string) *redis.Script { return redis.NewScript(src) }

// RunScript menjalankan skrip Lua terhadap klien milik instance ini.
//
// Strategi EVALSHA-dengan-fallback-ke-EVAL yang diminta rate limiter sudah disediakan
// redis.Script.Run: ia mengirim EVALSHA lebih dulu (hemat, karena badan skrip tidak ikut
// dikirim setiap kali), lalu mengulang dengan EVAL saat Redis membalas NOSCRIPT — yang
// terjadi setelah SCRIPT FLUSH atau restart Redis. Jadi fungsi ini tidak menambahkan
// logika apa pun di atasnya; ia hanya mengikat klien aplikasi, sehingga pemanggil tidak
// perlu memegang *redis.Client, dan meratakan hasilnya menjadi (any, error) yang biasa.
//
// Nilai kembalinya mengikuti pemetaan tipe go-redis: number Lua menjadi int64, string
// menjadi string, table menjadi []any, dan nil menjadi error redis.Nil. Skrip yang
// memanggil redis.error_reply akan tampak sebagai error biasa di sini.
//
// Error dibungkus dengan %w, jadi pemanggil tetap bisa memeriksa
// errors.Is(err, redis.Nil) untuk membedakan "skrip mengembalikan nil" dari kegagalan
// sungguhan.
func (r *Redis) RunScript(ctx context.Context, script *redis.Script, keys []string, args ...any) (any, error) {
	if script == nil {
		return nil, errors.New("cache: skrip nil")
	}
	res, err := script.Run(ctx, r.client, keys, args...).Result()
	if err != nil {
		return nil, fmt.Errorf("cache: menjalankan skrip lua %s: %w", script.Hash(), err)
	}
	return res, nil
}

// LoadScript menaruh skrip ke cache skrip Redis lewat SCRIPT LOAD tanpa menjalankannya.
//
// Dipakai saat start untuk memanaskan cache: tanpa ini, permintaan pertama yang memakai
// skrip menanggung satu putaran EVALSHA yang gagal ditambah EVAL berisi badan skrip.
// Bukan keharusan — RunScript tetap benar tanpa pemanasan — tapi ia memindahkan biaya itu
// dari jalur request ke jalur start.
func (r *Redis) LoadScript(ctx context.Context, script *redis.Script) error {
	if script == nil {
		return errors.New("cache: skrip nil")
	}
	if err := script.Load(ctx, r.client).Err(); err != nil {
		return fmt.Errorf("cache: memuat skrip lua %s: %w", script.Hash(), err)
	}
	return nil
}
