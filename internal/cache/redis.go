// Package cache menyediakan satu koneksi Redis bersama untuk seluruh aplikasi: rate
// limiter, circuit breaker, cache hasil health check provider, dan penghitung pemakaian.
//
// Ada dua hal yang dijaga ketat di sini.
//
// Pertama, rahasia tidak boleh bocor. DSN Redis bisa memuat password, sementara pesan
// error pustaka menyertakan URL mentah — net/url mengembalikan *url.Error yang mencetak
// seluruh URL, password dan semuanya. Karena itu tidak ada satu pun error dari paket ini
// yang menyalin teks error yang berasal dari DSN tanpa disaring lebih dulu.
//
// Kedua, kunci selalu bernamespace. Instance Redis bisa dipakai bersama aplikasi lain,
// jadi setiap kunci dibentuk lewat Key() di keys.go. Konsekuensinya, paket ini tidak
// pernah memakai FLUSHDB atau FLUSHALL — termasuk di test.
package cache

import (
	"context"
	"errors"
	"fmt"
	"log/slog"
	"net"
	"net/url"
	"runtime"
	"strings"
	"sync"
	"time"

	"github.com/redis/go-redis/v9"

	"github.com/NexGen-X/Route-X/internal/config"
	"github.com/NexGen-X/Route-X/internal/observability"
)

// Parameter bawaan untuk pemakaian produksi. Semuanya hanya dipasang bila DSN tidak
// menentukan nilainya sendiri lewat query string, sehingga operator tetap bisa menyetel
// per-deployment tanpa rilis ulang (mis. "redis://host:6379/0?pool_size=200").
const (
	// connsPerCPU mengikuti bawaan go-redis. Beban Redis di gateway ini pendek-pendek
	// (INCR, EVAL rate limit, GET status breaker), bukan operasi yang menahan koneksi
	// lama, jadi kelipatan CPU sudah memadai.
	connsPerCPU = 10
	// Pool dibatasi dua arah: batas bawah supaya mesin kecil tetap punya cadangan saat
	// lonjakan, batas atas supaya mesin ber-CPU banyak tidak membuka ratusan koneksi
	// dan menghabiskan kuota "maxclients" Redis.
	minPoolSize = 10
	maxPoolSize = 100

	// Sebagian koneksi dijaga tetap hangat supaya request pertama setelah periode sepi
	// tidak menanggung biaya dial + HELLO.
	minIdleFloor = 2
	minIdleCap   = 10

	defaultDialTimeout  = 5 * time.Second
	defaultReadTimeout  = 3 * time.Second
	defaultWriteTimeout = 3 * time.Second
	// PoolTimeout dibuat lebih panjang dari ReadTimeout: menunggu koneksi kosong sedikit
	// lebih lama masih jauh lebih baik daripada menolak request padahal pool cuma sesak
	// sesaat.
	defaultPoolTimeout = defaultReadTimeout + time.Second

	// Redis kadang memutus koneksi yang menganggur, dan load balancer di depannya bisa
	// berpindah node. Mendaur koneksi lebih awal membuat pemutusan itu terjadi saat
	// koneksi tidak dipakai, bukan di tengah request.
	defaultConnMaxIdleTime = 5 * time.Minute
	defaultConnMaxLifetime = 30 * time.Minute

	// Retry menutup kegagalan sekejap (failover, koneksi diputus sepihak). Angkanya
	// ditahan kecil karena rate limiter berada di jalur request: lebih baik cepat gagal
	// dan mengambil keputusan fail-open daripada menahan request pengguna.
	defaultMaxRetries      = 3
	defaultMinRetryBackoff = 20 * time.Millisecond
	defaultMaxRetryBackoff = 500 * time.Millisecond

	// go-redis punya lapisan retry kedua di dalam satu percobaan perintah: bawaannya
	// mencoba dial 5 kali dengan jeda 100ms. Dikalikan MaxRetries di atas, satu perintah
	// bisa mencoba dial 24 kali dan menahan request pengguna hampir dua detik sebelum
	// menyerah. Rate limiter berada di jalur request, jadi lapisan itu dimatikan (1 =
	// sekali dial per percobaan) dan anggaran retry dipegang MaxRetries saja: empat
	// percobaan dengan backoff singkat, selesai di bawah 200ms.
	defaultDialerRetries = 1

	// clientName muncul di CLIENT LIST dan SLOWLOG milik Redis, sehingga operator bisa
	// membedakan koneksi gateway dari tetangga yang memakai instance yang sama.
	clientName = "routex"

	// connectPingTimeout membatasi verifikasi saat start. Cukup longgar untuk menampung
	// retry di atas, tapi tetap membuat proses gagal cepat kalau Redis memang mati.
	connectPingTimeout = 5 * time.Second

	// pingFallbackTimeout dipakai Ping bila pemanggil tidak memasang deadline sendiri,
	// supaya probe kesehatan tidak pernah menggantung tanpa batas.
	pingFallbackTimeout = 2 * time.Second
)

// Redis adalah pembungkus klien Redis milik aplikasi.
//
// Satu instance dibuat saat start lalu dibagikan ke seluruh subsistem: klien go-redis
// sudah aman dipakai bersamaan dari banyak goroutine dan memelihara pool koneksinya
// sendiri, jadi jangan membuat instance kedua per request.
type Redis struct {
	client *redis.Client
}

// Connect membuka pool koneksi ke Redis lalu memverifikasinya dengan PING bertimeout.
//
// Yang dikembalikan sudah siap pakai: kalau fungsi ini mengembalikan nil error, Redis
// terbukti menjawab. Kalau gagal, pool sudah ditutup kembali sehingga tidak ada goroutine
// atau socket yang tertinggal.
//
// Tidak ada pesan errornya yang memuat DSN atau password — lihat safeErrorf.
func Connect(ctx context.Context, cfg *config.Config, logger *slog.Logger) (*Redis, error) {
	if cfg == nil {
		return nil, errors.New("cache: konfigurasi nil")
	}
	if logger == nil {
		// Dipanggil dari main sebelum logger siap masih harus jalan, jangan panik.
		logger = slog.Default()
	}
	if cfg.RedisURL.IsZero() {
		return nil, errors.New("cache: REDIS_URL belum diatur")
	}
	installRedisLogger()

	dsn := cfg.RedisURL.Reveal()
	secrets := dsnSecrets(dsn)

	opt, err := parseOptions(dsn, secrets)
	if err != nil {
		return nil, err
	}
	applyProductionDefaults(opt)

	client := redis.NewClient(opt)

	pingCtx, cancel := context.WithTimeout(ctx, connectPingTimeout)
	defer cancel()
	if err := client.Ping(pingCtx).Err(); err != nil {
		// NewClient sudah menyalakan goroutine pemelihara pool; tanpa Close di sini
		// setiap kegagalan start akan menetes.
		_ = client.Close()
		return nil, safeErrorf(err, secrets, "cache: redis di %s tidak menjawab PING", opt.Addr)
	}

	// opt.Addr aman di-log: ia hasil parsing terstruktur dan hanya memuat host:port
	// (atau path socket), tidak pernah userinfo.
	logger.LogAttrs(ctx, slog.LevelInfo, "redis terhubung",
		slog.String("addr", opt.Addr),
		slog.Int("db", opt.DB),
		slog.Int("pool_size", opt.PoolSize),
		slog.Bool("tls", opt.TLSConfig != nil),
	)

	if cfg.AppEnv.IsProduction() && opt.TLSConfig == nil && !isLocalAddr(opt.Network, opt.Addr) {
		// Bukan error: sebagian deployment memakai jaringan privat yang sudah terenkripsi
		// di lapisan bawah. Tapi trafik rate limit dan status breaker melewati kabel ini,
		// jadi operator perlu tahu kalau ia terkirim polos.
		logger.LogAttrs(ctx, slog.LevelWarn, "koneksi redis produksi tidak terenkripsi",
			slog.String("addr", opt.Addr),
			slog.String("saran", "pakai skema rediss:// bila Redis berada di luar host ini"),
		)
	}

	return &Redis{client: client}, nil
}

// Client membuka klien go-redis mentah untuk operasi yang tidak dibungkus paket ini —
// rate limiter, circuit breaker, dan pemakai lain yang butuh pipeline, transaksi, atau
// perintah spesifik.
//
// Pemanggil wajib menyusun kuncinya lewat Key() di keys.go; itu satu-satunya hal yang
// hilang saat menembus pembungkus ini.
func (r *Redis) Client() *redis.Client { return r.client }

// Ping memeriksa Redis masih menjawab. Dipakai probe /readyz.
//
// Bila ctx belum punya deadline, dipasang batas bawaan: probe kesehatan yang menggantung
// lebih buruk daripada probe yang melaporkan gagal, karena orchestrator jadi tidak
// pernah mendapat jawaban.
func (r *Redis) Ping(ctx context.Context) error {
	if _, ok := ctx.Deadline(); !ok {
		var cancel context.CancelFunc
		ctx, cancel = context.WithTimeout(ctx, pingFallbackTimeout)
		defer cancel()
	}
	if err := r.client.Ping(ctx).Err(); err != nil {
		return fmt.Errorf("cache: ping redis gagal: %w", err)
	}
	return nil
}

// Close menutup seluruh koneksi di pool. Dipanggil sekali saat shutdown.
//
// Aman dipanggil berulang dan pada penerima nil, supaya jalur shutdown tidak perlu
// menjaga urutan inisialisasi.
func (r *Redis) Close() error {
	if r == nil || r.client == nil {
		return nil
	}
	if err := r.client.Close(); err != nil && !errors.Is(err, redis.ErrClosed) {
		return fmt.Errorf("cache: menutup koneksi redis: %w", err)
	}
	return nil
}

// PoolStats adalah cuplikan kondisi pool koneksi, untuk /readyz dan metrik Prometheus.
//
// Tipe sendiri, bukan *redis.PoolStats, karena dua alasan: bentuk yang diekspos ke
// endpoint HTTP tidak boleh berubah hanya karena pustaka menambah field, dan
// WaitDuration lebih berguna sebagai time.Duration daripada int64 nanodetik.
//
// Yang paling menunjukkan pool mulai sesak adalah Timeouts dan WaitCount: keduanya naik
// ketika request harus menunggu koneksi kosong.
type PoolStats struct {
	// PoolSize adalah batas dasar hasil konfigurasi, jadi TotalConns bisa dibaca sebagai
	// rasio pemakaian, bukan angka lepas.
	PoolSize int

	TotalConns uint32
	IdleConns  uint32
	StaleConns uint32

	Hits     uint32
	Misses   uint32
	Timeouts uint32

	WaitCount    uint32
	WaitDuration time.Duration
}

// Stat mengambil cuplikan kondisi pool saat ini.
func (r *Redis) Stat() PoolStats {
	s := r.client.PoolStats()
	return PoolStats{
		PoolSize:     r.client.Options().PoolSize,
		TotalConns:   s.TotalConns,
		IdleConns:    s.IdleConns,
		StaleConns:   s.StaleConns,
		Hits:         s.Hits,
		Misses:       s.Misses,
		Timeouts:     s.Timeouts,
		WaitCount:    s.WaitCount,
		WaitDuration: time.Duration(s.WaitDurationNs),
	}
}

// --- Parsing DSN ------------------------------------------------------------

// parseOptions menerjemahkan DSN menjadi opsi klien tanpa membiarkan DSN ikut ke pesan
// error.
func parseOptions(dsn string, secrets []string) (*redis.Options, error) {
	// url.Parse dijalankan lebih dulu dengan sengaja. Kalau DSN-nya tidak bisa di-parse,
	// error yang dikembalikan adalah *url.Error yang mencetak URL mentah lengkap dengan
	// password. Error itu dibuang seluruhnya, bukan dibungkus: alasan kegagalannya
	// ("invalid port", "invalid control character") tidak sebanding dengan risiko
	// membocorkan kredensial ke log.
	u, err := url.Parse(dsn)
	if err != nil {
		return nil, safeErrorf(nil, secrets, "cache: REDIS_URL bukan URL yang sah")
	}

	// Skema diperiksa sendiri supaya pesannya menyebut pilihan yang didukung. Nilainya
	// aman dicetak: url.Parse memisahkan skema sebelum userinfo, jadi ia tidak mungkin
	// memuat password.
	switch u.Scheme {
	case "redis", "rediss", "unix":
	default:
		return nil, safeErrorf(nil, secrets,
			"cache: skema REDIS_URL %q tidak didukung, gunakan redis://, rediss://, atau unix://", u.Scheme)
	}

	opt, err := redis.ParseURL(dsn)
	if err != nil {
		// Sisa kegagalan ParseURL (nomor database bukan angka, opsi query tak dikenal)
		// hanya mengutip potongan yang bukan kredensial, dan alasannya betul-betul
		// menolong operator — jadi teksnya ikut, setelah dilewatkan penyaring.
		return nil, safeErrorf(nil, secrets, "cache: REDIS_URL tidak bisa dipakai: %s", err.Error())
	}
	return opt, nil
}

// applyProductionDefaults mengisi parameter pool, timeout, dan retry yang belum
// ditentukan.
//
// Semua field diperiksa nol lebih dulu karena redis.ParseURL menuliskan nilai dari query
// string DSN ke field yang sama; nol berarti "tidak disebut di DSN". Dengan begitu nilai
// eksplisit dari operator selalu menang atas bawaan di sini.
func applyProductionDefaults(opt *redis.Options) {
	if opt.PoolSize == 0 {
		opt.PoolSize = clamp(connsPerCPU*runtime.GOMAXPROCS(0), minPoolSize, maxPoolSize)
	}
	if opt.MinIdleConns == 0 {
		opt.MinIdleConns = clamp(opt.PoolSize/10, minIdleFloor, minIdleCap)
	}
	if opt.MaxIdleConns == 0 {
		// Koneksi menganggur tidak ditutup hanya karena pool lapang; yang memangkasnya
		// adalah ConnMaxIdleTime. Ini menghindari churn dial saat trafik bergelombang.
		opt.MaxIdleConns = opt.PoolSize
	}
	if opt.DialTimeout == 0 {
		opt.DialTimeout = defaultDialTimeout
	}
	if opt.DialerRetries == 0 {
		opt.DialerRetries = defaultDialerRetries
	}
	if opt.ReadTimeout == 0 {
		opt.ReadTimeout = defaultReadTimeout
	}
	if opt.WriteTimeout == 0 {
		opt.WriteTimeout = defaultWriteTimeout
	}
	if opt.PoolTimeout == 0 {
		opt.PoolTimeout = defaultPoolTimeout
	}
	if opt.ConnMaxIdleTime == 0 {
		opt.ConnMaxIdleTime = defaultConnMaxIdleTime
	}
	if opt.ConnMaxLifetime == 0 {
		opt.ConnMaxLifetime = defaultConnMaxLifetime
	}
	if opt.MaxRetries == 0 {
		opt.MaxRetries = defaultMaxRetries
	}
	if opt.MinRetryBackoff == 0 {
		opt.MinRetryBackoff = defaultMinRetryBackoff
	}
	if opt.MaxRetryBackoff == 0 {
		opt.MaxRetryBackoff = defaultMaxRetryBackoff
	}
	if opt.ClientName == "" {
		opt.ClientName = clientName
	}

	// Wajib menyala: tanpa ini go-redis mengabaikan deadline di context dan hanya memakai
	// ReadTimeout, sehingga batas waktu request HTTP tidak menular ke panggilan Redis.
	opt.ContextTimeoutEnabled = true
}

func clamp(v, lo, hi int) int { return min(max(v, lo), hi) }

// isLocalAddr melaporkan apakah alamat ini tidak meninggalkan host, sehingga trafiknya
// tidak perlu TLS.
func isLocalAddr(network, addr string) bool {
	if network == "unix" {
		return true
	}
	host, _, err := net.SplitHostPort(addr)
	if err != nil {
		host = addr
	}
	if host == "localhost" {
		return true
	}
	ip := net.ParseIP(host)
	return ip != nil && ip.IsLoopback()
}

// --- Jembatan log internal go-redis -----------------------------------------

// redisLoggerOnce menjaga agar jembatan hanya dipasang sekali. SetLogger di go-redis
// bersifat global, jadi pemasangan berulang (mis. beberapa test) tidak ada gunanya dan
// hanya membuat siapa yang menang bergantung pada urutan.
var redisLoggerOnce sync.Once

// installRedisLogger mengalihkan log internal go-redis ke slog.
//
// Secara bawaan go-redis menulis langsung ke os.Stderr dengan format teksnya sendiri.
// Akibatnya pesan yang paling dibutuhkan saat Redis bermasalah — "connection pool: failed
// to dial", "context canceled" dari pool — justru tidak pernah sampai ke log shipper yang
// hanya membaca JSON, dan tidak ikut level maupun tujuan log aplikasi.
func installRedisLogger() {
	redisLoggerOnce.Do(func() { redis.SetLogger(redisLogBridge{}) })
}

// redisLogBridge memenuhi antarmuka logging go-redis.
//
// Tipenya tidak menyebut nama antarmukanya karena antarmuka itu berada di paket internal
// go-redis; pemenuhannya bersifat struktural, cukup dari bentuk method Printf.
type redisLogBridge struct{}

func (redisLogBridge) Printf(ctx context.Context, format string, v ...any) {
	// Logger diambil dari context bila ada — go-redis meneruskan context perintah yang
	// gagal, sehingga pesannya bisa ikut terkorelasi dengan request. Kalau tidak ada
	// (pesan dari goroutine pemelihara pool), jatuh ke logger default proses.
	msg := strings.TrimSpace(fmt.Sprintf(format, v...))
	observability.LoggerFrom(ctx).LogAttrs(ctx, slog.LevelWarn, msg,
		slog.String("component", "go-redis"))
}

// --- Error yang aman di-log -------------------------------------------------

// redactedPlaceholder menggantikan potongan rahasia yang tercegat penyaring.
const redactedPlaceholder = "[REDACTED]"

// safeError menyembunyikan teks error asli sambil mempertahankan rantainya.
//
// Dua-duanya dibutuhkan: pesan yang dibaca manusia — dan yang masuk log atau respons
// HTTP — dijamin bersih dari kredensial, tapi errors.Is/errors.As tetap bisa menembus ke
// error asli, misalnya untuk membedakan context.DeadlineExceeded dari connection refused.
//
// Yang penting, fmt dan slog selalu lewat Error(), jadi membungkus ulang error ini
// dengan %w tidak membuka kembali teks aslinya.
type safeError struct {
	msg   string
	cause error
}

func (e *safeError) Error() string { return e.msg }
func (e *safeError) Unwrap() error { return e.cause }

// safeErrorf membentuk error dengan pesan tulisan sendiri, menempelkan alasan dari cause
// bila ada, lalu melewatkan seluruh hasilnya ke scrub.
func safeErrorf(cause error, secrets []string, format string, args ...any) error {
	msg := scrub(fmt.Sprintf(format, args...), secrets)
	if cause != nil {
		msg += ": " + scrub(cause.Error(), secrets)
	}
	return &safeError{msg: msg, cause: cause}
}

// scrub membuang setiap kemunculan nilai rahasia dari sebuah pesan.
//
// Ini lapisan kedua, bukan pertahanan utama: pesan error di paket ini dibentuk sendiri
// dan tidak menyalin teks yang berpotensi memuat DSN. scrub ada supaya perubahan pustaka
// di kemudian hari — atau jalur error yang belum terpikirkan — tidak diam-diam membuka
// kebocoran.
//
// Penyaringan sengaja dilakukan apa adanya, termasuk untuk rahasia yang sangat pendek
// yang bisa mencacah pesan sampai tak terbaca. Pesan rusak masih jauh lebih baik daripada
// password yang terbit di log.
func scrub(msg string, secrets []string) string {
	for _, s := range secrets {
		if s == "" {
			continue
		}
		msg = strings.ReplaceAll(msg, s, redactedPlaceholder)
	}
	return msg
}

// dsnSecrets mengumpulkan potongan teks yang tidak boleh muncul di pesan error.
//
// Tiga bentuk dikumpulkan karena tiga-tiganya bisa muncul di pesan pustaka yang berbeda:
// DSN utuh (dicetak *url.Error), userinfo mentah yang masih ter-escape
// ("user:p%40ss"), dan password dalam bentuk terdekode ("p@ss").
func dsnSecrets(dsn string) []string {
	secrets := []string{dsn}

	u, err := url.Parse(dsn)
	if err != nil || u.User == nil {
		// DSN utuh tetap tersaring; ini yang dicetak url.Error saat parsing gagal.
		return secrets
	}
	if raw := u.User.String(); raw != "" {
		secrets = append(secrets, raw)
	}
	if pw, ok := u.User.Password(); ok && pw != "" {
		secrets = append(secrets, pw)
	}
	return secrets
}
