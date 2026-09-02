package auth

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"log/slog"
	"strconv"
	"strings"
	"time"

	"github.com/NexGen-X/Route-X/internal/cache"
)

// Batas bawaan pembatasan laju login.
//
// Angkanya dipilih untuk membuat penyapuan password tidak praktis tanpa mengganggu orang
// yang sedang lupa passwordnya. Batas per IP lebih longgar daripada per email karena satu
// alamat bisa mewakili seluruh kantor di belakang NAT; batas per email lebih ketat karena
// tidak ada alasan sah satu akun dicoba sepuluh kali dalam seperempat jam.
const (
	// DefaultLoginRateWindow adalah lebar jendela penghitungan.
	DefaultLoginRateWindow = 15 * time.Minute
	// DefaultLoginIPLimit adalah jumlah percobaan maksimum per alamat IP per jendela.
	DefaultLoginIPLimit = 20
	// DefaultLoginEmailLimit adalah jumlah percobaan maksimum per email per jendela.
	DefaultLoginEmailLimit = 10
)

// loginCounterScript menaikkan penghitung dan memasang masa berlakunya sebagai satu
// operasi, lalu melaporkan nilai penghitung beserta sisa umur kuncinya.
//
// Harus berupa skrip, bukan INCR lalu EXPIRE terpisah. Dua perintah terpisah membuka dua
// lubang sekaligus: dua instance gateway yang menaikkan penghitung bersamaan bisa
// sama-sama menganggap kunci itu baru, dan lebih buruk, sebuah proses yang mati di antara
// INCR dan EXPIRE meninggalkan penghitung tanpa masa berlaku — yang berarti akun atau
// alamat itu terkunci selamanya sampai ada yang menghapus kuncinya dengan tangan.
//
// PTTL diambil di dalam skrip yang sama supaya nilai Retry-After yang dikirim ke klien
// benar-benar sisa umur jendela ini, bukan lebar jendela penuh yang selalu terlalu lama.
var loginCounterScript = cache.NewScript(`
local hits = redis.call('INCR', KEYS[1])
if hits == 1 then
	redis.call('PEXPIRE', KEYS[1], ARGV[1])
end
return {hits, redis.call('PTTL', KEYS[1])}
`)

// LoginLimiter membatasi laju percobaan login per alamat IP dan per email.
//
// # Mengapa Redis, dan mengapa ia bukan pertahanan utama
//
// Penguncian per akun sudah ada di database (identity.Users.RecordLoginFailure): lima
// kegagalan berturut-turut mengunci akun lima belas menit, atomik, dan tahan restart.
// Yang TIDAK ditutupnya adalah pola sebaliknya — satu percobaan untuk masing-masing dari
// seribu alamat email, yang tidak pernah menyentuh ambang akun mana pun tetapi memaksa
// server melakukan seribu kali kerja argon2 seberat 64 MiB. Itulah yang dijaga di sini,
// dan itu harus dibagi antar instance, jadi tempatnya Redis dan bukan memori proses.
//
// Karena pembagian peran itu, pembatas ini GAGAL TERBUKA: Redis yang mati atau lambat
// membuat percobaan diloloskan, bukan ditolak. Pilihan itu sadar — kalau ia gagal
// tertutup, matinya Redis berarti tidak ada seorang pun bisa masuk ke dashboard, termasuk
// untuk memperbaiki Redis, sementara jaminan yang benar-benar melindungi akun individual
// tetap berjalan di database.
//
// Penerima nil sah dan berarti tanpa pembatasan, sehingga pemanggil tidak perlu
// bercabang saat Redis tidak dikonfigurasi.
type LoginLimiter struct {
	redis  *cache.Redis
	logger *slog.Logger

	window     time.Duration
	ipLimit    int
	emailLimit int
}

// LimiterOption menyetel LoginLimiter saat konstruksi.
type LimiterOption func(*LoginLimiter)

// WithLoginRateWindow mengubah lebar jendela penghitungan. Nilai ≤ 0 diabaikan.
func WithLoginRateWindow(d time.Duration) LimiterOption {
	return func(l *LoginLimiter) {
		if d > 0 {
			l.window = d
		}
	}
}

// WithLoginRateLimits mengubah batas per IP dan per email. Nilai ≤ 0 mematikan batas yang
// bersangkutan tanpa mematikan yang lain.
func WithLoginRateLimits(perIP, perEmail int) LimiterOption {
	return func(l *LoginLimiter) {
		l.ipLimit = perIP
		l.emailLimit = perEmail
	}
}

// NewLoginLimiter membuat pembatas laju login di atas Redis.
//
// rdb nil menghasilkan pembatas yang selalu meloloskan, supaya deployment tanpa Redis —
// dan test yang tidak memerlukannya — tetap bisa memakai jalur kode yang sama.
func NewLoginLimiter(rdb *cache.Redis, logger *slog.Logger, opts ...LimiterOption) *LoginLimiter {
	if logger == nil {
		logger = slog.Default()
	}
	l := &LoginLimiter{
		redis:      rdb,
		logger:     logger,
		window:     DefaultLoginRateWindow,
		ipLimit:    DefaultLoginIPLimit,
		emailLimit: DefaultLoginEmailLimit,
	}
	for _, opt := range opts {
		if opt != nil {
			opt(l)
		}
	}
	return l
}

// Allow mencatat satu percobaan login dan melaporkan apakah ia boleh dilanjutkan.
//
// Nilai kedua adalah perkiraan lama tunggu, untuk dipasang di header Retry-After.
//
// Penghitung per IP diperiksa lebih dulu dan penghitung per email TIDAK disentuh bila IP
// sudah melampaui batasnya. Urutan itu penting: kalau keduanya selalu dinaikkan, satu
// penyerang yang sudah diblokir masih bisa terus menghabiskan kuota email milik korban dan
// dengan itu menghalangi pemilik akun yang sah masuk dari tempat lain.
func (l *LoginLimiter) Allow(ctx context.Context, ip, email string) (bool, time.Duration) {
	if l == nil || l.redis == nil {
		return true, 0
	}

	if ip != "" && l.ipLimit > 0 {
		if ok, retryAfter := l.hit(ctx, "login_ip", ipBucketKey(ip), l.ipLimit); !ok {
			return false, retryAfter
		}
	}
	if id := emailBucketKey(email); id != "" && l.emailLimit > 0 {
		if ok, retryAfter := l.hit(ctx, "login_email", id, l.emailLimit); !ok {
			return false, retryAfter
		}
	}
	return true, 0
}

// ResetEmail melepas penghitung email setelah login berhasil.
//
// Hanya penghitung email yang dilepas, tidak penghitung IP. Beberapa salah ketik lalu
// berhasil masuk adalah kejadian normal dan tidak boleh menghalangi pemilik akun besok;
// sebaliknya satu login berhasil dari sebuah alamat NAT tidak boleh menjadi cara
// membersihkan jejak penyapuan password dari alamat yang sama.
func (l *LoginLimiter) ResetEmail(ctx context.Context, email string) {
	if l == nil || l.redis == nil {
		return
	}
	id := emailBucketKey(email)
	if id == "" {
		return
	}
	key := cache.RateLimitKey("login_email", id, l.bucket(time.Now()))
	// DEL atas kunci yang tidak ada bukan kesalahan bagi Redis, jadi tidak perlu
	// membedakan "tidak ada penghitung" dari "berhasil dihapus".
	if err := l.redis.Client().Del(ctx, key).Err(); err != nil {
		l.logger.Warn("gagal melepas penghitung batas laju login", "error", err.Error())
	}
}

// hit menaikkan satu penghitung dan membandingkannya dengan batas.
func (l *LoginLimiter) hit(ctx context.Context, scope, id string, limit int) (bool, time.Duration) {
	key := cache.RateLimitKey(scope, id, l.bucket(time.Now()))

	res, err := l.redis.RunScript(ctx, loginCounterScript, []string{key}, l.window.Milliseconds())
	if err != nil {
		// Gagal terbuka. Lihat penjelasan di dokumentasi tipe.
		l.logger.Warn("pembatas laju login tidak bisa menghubungi redis, percobaan diloloskan",
			"scope", scope, "error", err.Error())
		return true, 0
	}

	hits, ttl := parseCounterResult(res)
	if hits <= int64(limit) {
		return true, 0
	}
	if ttl <= 0 {
		// PTTL bisa melaporkan -1 (tanpa masa berlaku) atau -2 (kunci hilang) bila kunci
		// baru saja kedaluwarsa di antara dua perintah. Jendela penuh dipakai sebagai
		// perkiraan yang aman, karena angka nol akan mengundang klien mencoba seketika.
		ttl = l.window
	}
	return false, ttl
}

// bucket menghasilkan label jendela waktu.
//
// Nomor jendela ikut ke dalam kunci, jadi penghitung jendela yang sudah lewat hilang
// sendiri lewat masa berlakunya dan tidak perlu ada pekerjaan latar yang membersihkannya.
// Konsekuensi bentuk jendela tetap ini diketahui: percobaan yang menumpuk persis di
// perbatasan dua jendela bisa mencapai dua kali batas dalam waktu singkat. Untuk pembatas
// yang tugasnya menahan penyapuan berjam-jam, itu tidak mengubah apa pun — dan jaminan per
// akun di database tidak punya perbatasan seperti ini.
func (l *LoginLimiter) bucket(now time.Time) string {
	seconds := int64(l.window.Seconds())
	if seconds <= 0 {
		seconds = 1
	}
	return strconv.FormatInt(now.Unix()/seconds, 10)
}

// parseCounterResult membaca nilai kembalian skrip Lua.
//
// Ditulis defensif karena hasil skrip melewati pemetaan tipe go-redis: table Lua menjadi
// []any berisi int64. Bentuk yang tidak dikenali diperlakukan sebagai nol, yang berarti
// "loloskan" — sejalan dengan sifat gagal-terbuka pembatas ini.
func parseCounterResult(res any) (hits int64, ttl time.Duration) {
	values, ok := res.([]any)
	if !ok || len(values) < 2 {
		return 0, 0
	}
	if n, ok := values[0].(int64); ok {
		hits = n
	}
	if n, ok := values[1].(int64); ok && n > 0 {
		ttl = time.Duration(n) * time.Millisecond
	}
	return hits, ttl
}

// ipBucketKey menyiapkan alamat IP sebagai bagian kunci.
func ipBucketKey(ip string) string { return strings.TrimSpace(ip) }

// emailBucketKey mengubah email menjadi bagian kunci yang tidak bisa dibaca ulang.
//
// Yang masuk Redis adalah hash, bukan emailnya. Redis di sini bukan penyimpan data
// identitas: isinya tidak dienkripsi, sering dipakai bersama aplikasi lain, dan mudah
// terbaca lewat SCAN oleh siapa pun yang punya akses ke instance-nya. Daftar alamat email
// admin sebuah gateway adalah tepat jenis data yang tidak perlu berada di sana. Hash 128
// bit sudah lebih dari cukup untuk membedakan alamat, dan karena bentuknya heksadesimal
// ia juga tidak menuntut penyandian tambahan saat masuk kunci.
func emailBucketKey(email string) string {
	normalized := strings.ToLower(strings.TrimSpace(email))
	if normalized == "" {
		return ""
	}
	sum := sha256.Sum256([]byte(normalized))
	return hex.EncodeToString(sum[:16])
}
