package cache

import (
	"bytes"
	"context"
	"crypto/rand"
	"encoding/hex"
	"errors"
	"fmt"
	"io"
	"log/slog"
	"os"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/alicebob/miniredis/v2"
	"github.com/alicebob/miniredis/v2/server"
	"github.com/redis/go-redis/v9"

	"github.com/NexGen-X/Route-X/internal/config"
	"github.com/NexGen-X/Route-X/internal/observability"
	"github.com/NexGen-X/Route-X/internal/security"
)

// passwordUji adalah penanda yang dicari di setiap pesan error dan setiap baris log.
// Kalau string ini pernah muncul, ada jalur yang membocorkan kredensial.
const passwordUji = "sangat-rahasia"

// TestMain membungkam logger default proses.
//
// Log internal go-redis dialihkan Connect ke slog, dan pesan dari goroutine pemelihara
// pool jatuh ke slog.Default() karena context-nya tidak membawa logger. Beberapa test di
// sini sengaja mematikan Redis, yang memicu puluhan pesan semacam itu dan menenggelamkan
// keluaran test.
func TestMain(m *testing.M) {
	slog.SetDefault(observability.NewLogger(io.Discard, observability.LoggerOptions{Level: slog.LevelError}))
	os.Exit(m.Run())
}

// skripUjiRateLimit meniru bentuk skrip yang akan dipakai rate limiter: menambah
// penghitung dan memasang TTL hanya ketika penghitung baru dibuat, sebagai satu operasi
// yang tidak bisa disalip instance lain.
//
// Mengembalikan nomor hit bila masih di dalam kuota, 0 bila kuota terlampaui.
var skripUjiRateLimit = NewScript(`
local hits = redis.call('INCR', KEYS[1])
if hits == 1 then
	redis.call('PEXPIRE', KEYS[1], ARGV[1])
end
if hits > tonumber(ARGV[2]) then
	return 0
end
return hits
`)

// --- Perkakas test ----------------------------------------------------------

// loggerUji membuat logger sungguhan yang menulis ke buffer, supaya isi log bisa
// diperiksa — bukan hanya dibuang.
func loggerUji() (*slog.Logger, *bytes.Buffer) {
	var buf bytes.Buffer
	return observability.NewLogger(&buf, observability.LoggerOptions{Level: slog.LevelDebug}), &buf
}

// redisUji menyalakan miniredis lalu menyambungkannya lewat Connect, sehingga jalur yang
// diuji sama dengan jalur produksi.
func redisUji(t *testing.T, dsn string) (*Redis, *bytes.Buffer) {
	t.Helper()

	logger, logBuf := loggerUji()
	cfg := &config.Config{
		AppEnv:   config.EnvDevelopment,
		RedisURL: security.Secret(dsn),
	}
	r, err := Connect(context.Background(), cfg, logger)
	if err != nil {
		t.Fatalf("Connect gagal: %v", err)
	}
	t.Cleanup(func() { _ = r.Close() })
	return r, logBuf
}

// miniredisUji menyalakan miniredis dan mengembalikan koneksi yang sudah siap.
func miniredisUji(t *testing.T) (*Redis, *miniredis.Miniredis, *bytes.Buffer) {
	t.Helper()
	mr := miniredis.RunT(t)
	r, logBuf := redisUji(t, "redis://"+mr.Addr()+"/0")
	return r, mr, logBuf
}

// assertTidakBocor memastikan sebuah teks tidak memuat satu pun potongan rahasia.
func assertTidakBocor(t *testing.T, apa, teks string, rahasia ...string) {
	t.Helper()
	for _, s := range rahasia {
		if s == "" {
			continue
		}
		if strings.Contains(teks, s) {
			t.Fatalf("%s membocorkan %q:\n%s", apa, s, teks)
		}
	}
}

func mustInt64(t *testing.T, v any) int64 {
	t.Helper()
	n, ok := v.(int64)
	if !ok {
		t.Fatalf("hasil skrip bertipe %T, mau int64: %v", v, v)
	}
	return n
}

// --- Connect ----------------------------------------------------------------

func TestConnect(t *testing.T) {
	r, mr, logBuf := miniredisUji(t)

	if err := r.Ping(context.Background()); err != nil {
		t.Fatalf("Ping gagal: %v", err)
	}
	if r.Client() == nil {
		t.Fatal("Client() nil setelah Connect berhasil")
	}
	if got := r.Client().Options().Addr; got != mr.Addr() {
		t.Fatalf("Addr = %q, mau %q", got, mr.Addr())
	}
	if !strings.Contains(logBuf.String(), "redis terhubung") {
		t.Fatalf("koneksi berhasil tidak tercatat di log:\n%s", logBuf.String())
	}
}

// TestConnectDenganPasswordTidakMencatatnya menguji jalur sukses pada Redis yang meminta
// AUTH: koneksi harus jadi, dan password tidak boleh terbit di log walau DSN memuatnya.
func TestConnectDenganPasswordTidakMencatatnya(t *testing.T) {
	mr := miniredis.RunT(t)
	mr.RequireUserAuth("routex", passwordUji)

	dsn := "redis://routex:" + passwordUji + "@" + mr.Addr() + "/0"
	r, logBuf := redisUji(t, dsn)

	if err := r.Ping(context.Background()); err != nil {
		t.Fatalf("Ping gagal pada redis ber-AUTH: %v", err)
	}
	assertTidakBocor(t, "log", logBuf.String(), passwordUji, dsn)
}

// TestConnectMenolakDSNTidakValid adalah inti jaminan keamanan paket ini: apa pun bentuk
// kegagalannya, DSN dan passwordnya tidak boleh ikut ke pesan error. Pesan error berakhir
// di log start dan sering ditempel ke tiket dukungan.
func TestConnectMenolakDSNTidakValid(t *testing.T) {
	tests := []struct {
		name string
		dsn  string
	}{
		// url.Parse gagal di sini dan errornya (*url.Error) mencetak URL mentah utuh.
		{"port bukan angka", "redis://user:" + passwordUji + "@127.0.0.1:bukan-port/0"},
		{"karakter kendali di URL", "redis://user:" + passwordUji + "@127.0.0.1:6379/0\n"},
		{"tanda kurung siku tidak seimbang", "redis://user:" + passwordUji + "@[::1:6379/0"},

		// Skema ditolak sebelum pustaka menyentuh DSN.
		{"skema tidak didukung", "http://user:" + passwordUji + "@127.0.0.1:6379/0"},
		{"tanpa skema", "user:" + passwordUji + "@127.0.0.1:6379"},

		// redis.ParseURL yang menolak.
		{"nomor database bukan angka", "redis://user:" + passwordUji + "@127.0.0.1:6379/bukan-angka"},
		{"jalur terlalu dalam", "redis://user:" + passwordUji + "@127.0.0.1:6379/0/1"},
		{"opsi query tak dikenal", "redis://user:" + passwordUji + "@127.0.0.1:6379/0?opsi_ngawur=1"},
		{"durasi query tidak sah", "redis://user:" + passwordUji + "@127.0.0.1:6379/0?dial_timeout=besok"},

		// DSN sah tapi tidak ada yang menjawab: kegagalan pindah ke tahap PING.
		{"redis tidak menjawab", "redis://user:" + passwordUji + "@127.0.0.1:1/0"},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			logger, logBuf := loggerUji()
			cfg := &config.Config{AppEnv: config.EnvDevelopment, RedisURL: security.Secret(tc.dsn)}

			r, err := Connect(context.Background(), cfg, logger)
			if err == nil {
				_ = r.Close()
				t.Fatalf("Connect menerima DSN %q, seharusnya menolak", tc.dsn)
			}
			if r != nil {
				t.Fatal("Connect mengembalikan koneksi bersama error")
			}
			assertTidakBocor(t, "pesan error", err.Error(), passwordUji, tc.dsn)
			assertTidakBocor(t, "log", logBuf.String(), passwordUji, tc.dsn)
			// Pesannya tetap harus menolong: sebutkan variabel yang salah.
			if !strings.Contains(err.Error(), "REDIS_URL") && !strings.Contains(err.Error(), "redis") {
				t.Fatalf("pesan error tidak menyebut sumber masalah: %q", err.Error())
			}
		})
	}
}

// TestConnectPasswordTerescapeJugaTersaring menutup celah bentuk: password bisa muncul
// dalam bentuk mentah yang masih ter-escape maupun bentuk terdekode, tergantung pustaka
// mana yang melaporkan.
func TestConnectPasswordTerescapeJugaTersaring(t *testing.T) {
	const mentah = "sangat%40rahasia"
	const terdekode = "sangat@rahasia"
	dsn := "redis://user:" + mentah + "@127.0.0.1:6379/bukan-angka"

	logger, logBuf := loggerUji()
	cfg := &config.Config{AppEnv: config.EnvDevelopment, RedisURL: security.Secret(dsn)}

	_, err := Connect(context.Background(), cfg, logger)
	if err == nil {
		t.Fatal("Connect menerima nomor database yang tidak sah")
	}
	assertTidakBocor(t, "pesan error", err.Error(), mentah, terdekode, dsn)
	assertTidakBocor(t, "log", logBuf.String(), mentah, terdekode, dsn)
}

func TestConnectMenolakKonfigurasiKosong(t *testing.T) {
	t.Run("config nil", func(t *testing.T) {
		if _, err := Connect(context.Background(), nil, slog.Default()); err == nil {
			t.Fatal("config nil diterima")
		}
	})
	t.Run("REDIS_URL kosong", func(t *testing.T) {
		cfg := &config.Config{AppEnv: config.EnvDevelopment}
		if _, err := Connect(context.Background(), cfg, slog.Default()); err == nil {
			t.Fatal("REDIS_URL kosong diterima")
		}
	})
	t.Run("logger nil tetap jalan", func(t *testing.T) {
		mr := miniredis.RunT(t)
		cfg := &config.Config{
			AppEnv:   config.EnvDevelopment,
			RedisURL: security.Secret("redis://" + mr.Addr() + "/0"),
		}
		r, err := Connect(context.Background(), cfg, nil)
		if err != nil {
			t.Fatalf("Connect dengan logger nil gagal: %v", err)
		}
		t.Cleanup(func() { _ = r.Close() })
	})
}

// TestConnectMenutupPoolSaatPingGagal menjaga agar kegagalan start tidak meninggalkan
// koneksi menggantung. Proses yang gagal start biasanya dicoba ulang oleh supervisor, dan
// kebocoran semacam ini menumpuk setiap percobaan sampai Redis menolak klien baru.
//
// Hanya PING yang dibuat gagal, bukan seluruh perintah: dengan begitu dial dan handshake
// tetap berhasil dan pool benar-benar terisi, sehingga yang diuji adalah pembereskan pool
// yang sudah jadi — bukan pool yang gagal terbentuk sejak awal.
func TestConnectMenutupPoolSaatPingGagal(t *testing.T) {
	mr := miniredis.RunT(t)
	mr.Server().SetPreHook(func(c *server.Peer, cmd string, args ...string) bool {
		if !strings.EqualFold(cmd, "PING") {
			return false
		}
		c.WriteError("SIMULASIGAGAL ping ditolak")
		return true
	})

	cfg := &config.Config{
		AppEnv:   config.EnvDevelopment,
		RedisURL: security.Secret("redis://" + mr.Addr() + "/0"),
	}
	logger, _ := loggerUji()

	r, err := Connect(context.Background(), cfg, logger)
	if err == nil {
		_ = r.Close()
		t.Fatal("Connect berhasil padahal PING dibalas error")
	}
	if mr.TotalConnectionCount() == 0 {
		t.Fatal("tidak ada koneksi yang pernah dibuat, test tidak menguji apa yang dimaksud")
	}

	// Penutupan socket terlihat di sisi server secara asinkron.
	tungguSampai(t, 5*time.Second, "koneksi ke redis masih terbuka setelah Connect gagal",
		func() bool { return mr.CurrentConnectionCount() == 0 })
}

func TestConnectMenghormatiDeadlineContext(t *testing.T) {
	mr := miniredis.RunT(t)
	addr := mr.Addr()
	mr.Close()

	ctx, cancel := context.WithTimeout(context.Background(), 50*time.Millisecond)
	defer cancel()

	cfg := &config.Config{AppEnv: config.EnvDevelopment, RedisURL: security.Secret("redis://" + addr + "/0")}
	logger, _ := loggerUji()

	mulai := time.Now()
	if _, err := Connect(ctx, cfg, logger); err == nil {
		t.Fatal("Connect berhasil padahal redis mati")
	}
	// Tanpa penghormatan deadline, retry bawaan akan memakan waktu jauh lebih lama.
	if elapsed := time.Since(mulai); elapsed > connectPingTimeout {
		t.Fatalf("Connect mengabaikan deadline context: butuh %s", elapsed)
	}
}

// --- Close, Ping, Stat ------------------------------------------------------

func TestCloseIdempoten(t *testing.T) {
	mr := miniredis.RunT(t)
	cfg := &config.Config{AppEnv: config.EnvDevelopment, RedisURL: security.Secret("redis://" + mr.Addr() + "/0")}
	logger, _ := loggerUji()

	r, err := Connect(context.Background(), cfg, logger)
	if err != nil {
		t.Fatalf("Connect gagal: %v", err)
	}
	if err := r.Close(); err != nil {
		t.Fatalf("Close pertama: %v", err)
	}
	// Jalur shutdown tidak perlu menjaga urutan: Close kedua bukan error.
	if err := r.Close(); err != nil {
		t.Fatalf("Close kedua: %v", err)
	}
	// Penerima nil juga aman, supaya main bisa memanggilnya walau Connect gagal.
	var nihil *Redis
	if err := nihil.Close(); err != nil {
		t.Fatalf("Close pada nil: %v", err)
	}
}

func TestPingGagalSetelahRedisMati(t *testing.T) {
	r, mr, _ := miniredisUji(t)
	if err := r.Ping(context.Background()); err != nil {
		t.Fatalf("Ping awal gagal: %v", err)
	}

	mr.Close()

	err := r.Ping(context.Background())
	if err == nil {
		t.Fatal("Ping berhasil padahal redis mati")
	}
	assertTidakBocor(t, "pesan error ping", err.Error(), passwordUji)
}

// TestPingMemasangDeadlineSendiri memastikan probe /readyz tidak pernah menggantung tanpa
// batas ketika pemanggil lupa memasang deadline.
func TestPingMemasangDeadlineSendiri(t *testing.T) {
	r, mr, _ := miniredisUji(t)
	mr.Close()

	mulai := time.Now()
	if err := r.Ping(context.Background()); err == nil {
		t.Fatal("Ping berhasil padahal redis mati")
	}
	if elapsed := time.Since(mulai); elapsed > 5*time.Second {
		t.Fatalf("Ping tanpa deadline pemanggil menggantung %s", elapsed)
	}
}

func TestStat(t *testing.T) {
	r, _, _ := miniredisUji(t)
	ctx := context.Background()

	if err := r.Client().Set(ctx, Key("test", "stat"), "v", time.Minute).Err(); err != nil {
		t.Fatalf("SET gagal: %v", err)
	}

	s := r.Stat()
	if s.PoolSize <= 0 {
		t.Fatalf("PoolSize = %d, mau > 0", s.PoolSize)
	}
	if s.PoolSize != r.Client().Options().PoolSize {
		t.Fatalf("PoolSize = %d, tidak cocok dengan konfigurasi klien %d", s.PoolSize, r.Client().Options().PoolSize)
	}
	if s.TotalConns == 0 {
		t.Fatal("TotalConns = 0 padahal sudah ada perintah yang jalan")
	}
	if s.Hits+s.Misses == 0 {
		t.Fatal("Hits dan Misses dua-duanya 0 padahal pool sudah dipakai")
	}
	if s.WaitDuration < 0 {
		t.Fatalf("WaitDuration negatif: %s", s.WaitDuration)
	}
}

// TestSetGetLewatClient memverifikasi jalur yang dipakai paket lain: klien mentah plus
// kunci yang disusun Key().
func TestSetGetLewatClient(t *testing.T) {
	r, mr, _ := miniredisUji(t)
	ctx := context.Background()
	key := Key("test", "roundtrip", "model", "openai/gpt-4o")

	if err := r.Client().Set(ctx, key, "halo", time.Minute).Err(); err != nil {
		t.Fatalf("SET gagal: %v", err)
	}
	got, err := r.Client().Get(ctx, key).Result()
	if err != nil {
		t.Fatalf("GET gagal: %v", err)
	}
	if got != "halo" {
		t.Fatalf("GET = %q, mau %q", got, "halo")
	}
	if ttl := mr.TTL(key); ttl <= 0 || ttl > time.Minute {
		t.Fatalf("TTL = %s, mau di antara 0 dan 1m", ttl)
	}
	// Kunci yang benar-benar tersimpan harus bernamespace.
	for _, k := range mr.Keys() {
		if !strings.HasPrefix(k, KeyPrefix+":") {
			t.Fatalf("kunci %q ditulis di luar namespace %q", k, KeyPrefix)
		}
	}

	if _, err := r.Client().Get(ctx, Key("test", "tidak-ada")).Result(); err != redis.Nil {
		t.Fatalf("GET kunci kosong mengembalikan %v, mau redis.Nil", err)
	}
}

// --- Skrip Lua --------------------------------------------------------------

func TestRunScript(t *testing.T) {
	r, mr, _ := miniredisUji(t)
	ctx := context.Background()
	key := Key("test", "script", "rl")

	const limit = 2
	const windowMS = 60000

	// Dua panggilan pertama masih di dalam kuota dan mengembalikan nomor hit.
	for i := int64(1); i <= limit; i++ {
		v, err := r.RunScript(ctx, skripUjiRateLimit, []string{key}, windowMS, limit)
		if err != nil {
			t.Fatalf("RunScript panggilan %d: %v", i, err)
		}
		if got := mustInt64(t, v); got != i {
			t.Fatalf("panggilan %d mengembalikan %d, mau %d", i, got, i)
		}
	}

	// Panggilan ketiga melampaui kuota.
	v, err := r.RunScript(ctx, skripUjiRateLimit, []string{key}, windowMS, limit)
	if err != nil {
		t.Fatalf("RunScript melewati kuota: %v", err)
	}
	if got := mustInt64(t, v); got != 0 {
		t.Fatalf("panggilan melewati kuota mengembalikan %d, mau 0", got)
	}

	// TTL dipasang oleh skrip di panggilan pertama, bukan oleh kode Go.
	if ttl := mr.TTL(key); ttl <= 0 || ttl > windowMS*time.Millisecond {
		t.Fatalf("skrip tidak memasang TTL: %s", ttl)
	}
}

// TestRunScriptFallbackNoscript membuktikan janji EVALSHA-dengan-fallback: setelah cache
// skrip Redis dikosongkan (SCRIPT FLUSH, atau restart Redis di produksi), pemanggilan
// berikutnya tetap berhasil karena badan skrip dikirim ulang lewat EVAL.
func TestRunScriptFallbackNoscript(t *testing.T) {
	r, _, _ := miniredisUji(t)
	ctx := context.Background()
	key := Key("test", "script", "noscript")

	skrip := NewScript(`return redis.call('INCR', KEYS[1])`)

	if _, err := r.RunScript(ctx, skrip, []string{key}); err != nil {
		t.Fatalf("RunScript pertama: %v", err)
	}
	// Sekarang digest-nya ter-cache di sisi Redis.
	ada, err := skrip.Exists(ctx, r.Client()).Result()
	if err != nil {
		t.Fatalf("SCRIPT EXISTS: %v", err)
	}
	if len(ada) != 1 || !ada[0] {
		t.Fatalf("skrip tidak ter-cache setelah dijalankan: %v", ada)
	}

	if err := r.Client().ScriptFlush(ctx).Err(); err != nil {
		t.Fatalf("SCRIPT FLUSH: %v", err)
	}
	ada, err = skrip.Exists(ctx, r.Client()).Result()
	if err != nil {
		t.Fatalf("SCRIPT EXISTS setelah flush: %v", err)
	}
	if len(ada) != 1 || ada[0] {
		t.Fatalf("cache skrip tidak benar-benar kosong: %v", ada)
	}

	v, err := r.RunScript(ctx, skrip, []string{key})
	if err != nil {
		t.Fatalf("RunScript setelah SCRIPT FLUSH: %v", err)
	}
	if got := mustInt64(t, v); got != 2 {
		t.Fatalf("penghitung = %d, mau 2 — skrip tidak jalan di percobaan kedua", got)
	}
}

func TestLoadScript(t *testing.T) {
	r, _, _ := miniredisUji(t)
	ctx := context.Background()

	skrip := NewScript(`return 42`)
	if err := r.LoadScript(ctx, skrip); err != nil {
		t.Fatalf("LoadScript: %v", err)
	}

	ada, err := skrip.Exists(ctx, r.Client()).Result()
	if err != nil {
		t.Fatalf("SCRIPT EXISTS: %v", err)
	}
	if len(ada) != 1 || !ada[0] {
		t.Fatalf("skrip tidak ter-cache setelah LoadScript: %v", ada)
	}

	v, err := r.RunScript(ctx, skrip, nil)
	if err != nil {
		t.Fatalf("RunScript setelah pemanasan: %v", err)
	}
	if got := mustInt64(t, v); got != 42 {
		t.Fatalf("hasil = %d, mau 42", got)
	}
}

func TestSkripNilDitolak(t *testing.T) {
	r, _, _ := miniredisUji(t)
	ctx := context.Background()

	if _, err := r.RunScript(ctx, nil, nil); err == nil {
		t.Fatal("RunScript menerima skrip nil")
	}
	if err := r.LoadScript(ctx, nil); err == nil {
		t.Fatal("LoadScript menerima skrip nil")
	}
}

func TestRunScriptMengembalikanErrorSkrip(t *testing.T) {
	r, _, _ := miniredisUji(t)
	ctx := context.Background()

	skrip := NewScript(`return redis.error_reply('SENGAJA gagal')`)
	if _, err := r.RunScript(ctx, skrip, nil); err == nil {
		t.Fatal("error dari dalam skrip tidak diteruskan")
	}
}

// --- Opsi, penyaring rahasia, dan pembantu ----------------------------------

func TestApplyProductionDefaults(t *testing.T) {
	opt := &redis.Options{Addr: "127.0.0.1:6379"}
	applyProductionDefaults(opt)

	if opt.PoolSize < minPoolSize || opt.PoolSize > maxPoolSize {
		t.Fatalf("PoolSize = %d, mau di dalam [%d,%d]", opt.PoolSize, minPoolSize, maxPoolSize)
	}
	if opt.MinIdleConns < minIdleFloor || opt.MinIdleConns > minIdleCap {
		t.Fatalf("MinIdleConns = %d, mau di dalam [%d,%d]", opt.MinIdleConns, minIdleFloor, minIdleCap)
	}
	if opt.MinIdleConns > opt.PoolSize {
		t.Fatalf("MinIdleConns (%d) melebihi PoolSize (%d)", opt.MinIdleConns, opt.PoolSize)
	}
	if opt.MaxIdleConns != opt.PoolSize {
		t.Fatalf("MaxIdleConns = %d, mau %d", opt.MaxIdleConns, opt.PoolSize)
	}
	if opt.PoolTimeout <= opt.ReadTimeout {
		t.Fatalf("PoolTimeout (%s) harus lebih panjang dari ReadTimeout (%s)", opt.PoolTimeout, opt.ReadTimeout)
	}
	if opt.ConnMaxIdleTime >= opt.ConnMaxLifetime {
		t.Fatalf("ConnMaxIdleTime (%s) harus lebih pendek dari ConnMaxLifetime (%s)",
			opt.ConnMaxIdleTime, opt.ConnMaxLifetime)
	}
	if opt.MaxRetries != defaultMaxRetries {
		t.Fatalf("MaxRetries = %d, mau %d", opt.MaxRetries, defaultMaxRetries)
	}
	if opt.DialerRetries != defaultDialerRetries {
		t.Fatalf("DialerRetries = %d, mau %d — lapisan retry ganda menahan request terlalu lama",
			opt.DialerRetries, defaultDialerRetries)
	}
	if opt.MinRetryBackoff >= opt.MaxRetryBackoff {
		t.Fatalf("backoff tidak masuk akal: min %s, max %s", opt.MinRetryBackoff, opt.MaxRetryBackoff)
	}
	if opt.ClientName != clientName {
		t.Fatalf("ClientName = %q, mau %q", opt.ClientName, clientName)
	}
	if !opt.ContextTimeoutEnabled {
		t.Fatal("ContextTimeoutEnabled harus menyala, kalau tidak deadline request tidak menular ke redis")
	}
	for name, d := range map[string]time.Duration{
		"DialTimeout":  opt.DialTimeout,
		"ReadTimeout":  opt.ReadTimeout,
		"WriteTimeout": opt.WriteTimeout,
	} {
		if d <= 0 {
			t.Fatalf("%s = %s, mau > 0", name, d)
		}
	}
}

// TestParseOptionsMenghormatiQueryDSN memastikan setelan eksplisit dari operator tidak
// ditimpa bawaan paket ini.
func TestParseOptionsMenghormatiQueryDSN(t *testing.T) {
	dsn := "redis://127.0.0.1:6379/3?pool_size=7&min_idle_conns=1&max_retries=1" +
		"&dial_timeout=9s&read_timeout=8s&write_timeout=7s&pool_timeout=6s&client_name=lain"

	opt, err := parseOptions(dsn, dsnSecrets(dsn))
	if err != nil {
		t.Fatalf("parseOptions: %v", err)
	}
	applyProductionDefaults(opt)

	tests := []struct {
		name string
		got  any
		want any
	}{
		{"DB", opt.DB, 3},
		{"PoolSize", opt.PoolSize, 7},
		{"MinIdleConns", opt.MinIdleConns, 1},
		{"MaxRetries", opt.MaxRetries, 1},
		{"DialTimeout", opt.DialTimeout, 9 * time.Second},
		{"ReadTimeout", opt.ReadTimeout, 8 * time.Second},
		{"WriteTimeout", opt.WriteTimeout, 7 * time.Second},
		{"PoolTimeout", opt.PoolTimeout, 6 * time.Second},
		{"ClientName", opt.ClientName, "lain"},
	}
	for _, tc := range tests {
		if tc.got != tc.want {
			t.Errorf("%s = %v, mau %v", tc.name, tc.got, tc.want)
		}
	}
}

func TestParseOptionsSkemaYangDidukung(t *testing.T) {
	tests := []struct {
		name    string
		dsn     string
		wantErr bool
	}{
		{"redis", "redis://127.0.0.1:6379/0", false},
		{"rediss", "rediss://127.0.0.1:6379/0", false},
		{"unix", "unix:///var/run/redis/redis.sock", false},
		{"http ditolak", "http://127.0.0.1:6379/0", true},
		{"postgres ditolak", "postgres://127.0.0.1:5432/db", true},
		{"tanpa skema ditolak", "127.0.0.1:6379", true},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			opt, err := parseOptions(tc.dsn, dsnSecrets(tc.dsn))
			if tc.wantErr {
				if err == nil {
					t.Fatalf("parseOptions(%q) diterima, seharusnya ditolak", tc.dsn)
				}
				return
			}
			if err != nil {
				t.Fatalf("parseOptions(%q): %v", tc.dsn, err)
			}
			// rediss harus otomatis menyalakan TLS; itu satu-satunya beda yang penting.
			if wantTLS := strings.HasPrefix(tc.dsn, "rediss://"); wantTLS != (opt.TLSConfig != nil) {
				t.Fatalf("TLS aktif = %v, mau %v", opt.TLSConfig != nil, wantTLS)
			}
		})
	}
}

func TestScrub(t *testing.T) {
	tests := []struct {
		name    string
		msg     string
		secrets []string
		want    string
	}{
		{"tanpa rahasia", "gagal", nil, "gagal"},
		{"rahasia kosong dilewati", "gagal", []string{""}, "gagal"},
		{"satu kemunculan", "dsn=redis://a:rahasia@h/0", []string{"rahasia"}, "dsn=redis://a:[REDACTED]@h/0"},
		{"semua kemunculan", "rahasia dan rahasia", []string{"rahasia"}, "[REDACTED] dan [REDACTED]"},
		{"beberapa rahasia", "a=1 b=2", []string{"1", "2"}, "a=[REDACTED] b=[REDACTED]"},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			if got := scrub(tc.msg, tc.secrets); got != tc.want {
				t.Fatalf("scrub = %q, mau %q", got, tc.want)
			}
		})
	}
}

func TestDsnSecrets(t *testing.T) {
	dsn := "redis://pengguna:sangat%40rahasia@127.0.0.1:6379/0"
	got := dsnSecrets(dsn)

	// Tiga bentuk harus terkumpul: DSN utuh, userinfo mentah, dan password terdekode.
	for _, want := range []string{dsn, "pengguna:sangat%40rahasia", "sangat@rahasia"} {
		found := false
		for _, s := range got {
			if s == want {
				found = true
				break
			}
		}
		if !found {
			t.Fatalf("dsnSecrets tidak memuat %q, hanya %q", want, got)
		}
	}

	// DSN yang tidak bisa di-parse tetap harus tersaring utuh.
	rusak := "redis://a:b@h:bukan-port/0"
	if s := dsnSecrets(rusak); len(s) == 0 || s[0] != rusak {
		t.Fatalf("dsnSecrets pada DSN rusak = %q, mau memuat DSN utuh", s)
	}
}

// TestSafeErrorMenjagaRantai memastikan penyaringan pesan tidak mengorbankan errors.Is.
func TestSafeErrorMenjagaRantai(t *testing.T) {
	cause := context.DeadlineExceeded
	err := safeErrorf(cause, []string{"rahasia"}, "cache: gagal pada %s", "rahasia")

	if strings.Contains(err.Error(), "rahasia") {
		t.Fatalf("pesan masih memuat rahasia: %q", err.Error())
	}
	if !errors.Is(err, context.DeadlineExceeded) {
		t.Fatal("errors.Is tidak bisa menembus ke cause")
	}
	// Membungkus ulang tetap memakai Error(), jadi teks aslinya tidak kembali terbuka.
	if wrapped := fmt.Errorf("start: %w", err); strings.Contains(wrapped.Error(), "rahasia") {
		t.Fatalf("pembungkusan ulang membocorkan rahasia: %q", wrapped.Error())
	}
}

func TestIsLocalAddr(t *testing.T) {
	tests := []struct {
		network string
		addr    string
		want    bool
	}{
		{"unix", "/var/run/redis/redis.sock", true},
		{"tcp", "127.0.0.1:6379", true},
		{"tcp", "localhost:6379", true},
		{"tcp", "[::1]:6379", true},
		{"tcp", "10.0.0.5:6379", false},
		{"tcp", "redis.internal:6379", false},
		{"tcp", "tanpa-port", false},
	}
	for _, tc := range tests {
		if got := isLocalAddr(tc.network, tc.addr); got != tc.want {
			t.Errorf("isLocalAddr(%q, %q) = %v, mau %v", tc.network, tc.addr, got, tc.want)
		}
	}
}

func TestClamp(t *testing.T) {
	tests := []struct{ v, lo, hi, want int }{
		{5, 1, 10, 5},
		{0, 1, 10, 1},
		{50, 1, 10, 10},
		{1, 1, 1, 1},
	}
	for _, tc := range tests {
		if got := clamp(tc.v, tc.lo, tc.hi); got != tc.want {
			t.Errorf("clamp(%d,%d,%d) = %d, mau %d", tc.v, tc.lo, tc.hi, got, tc.want)
		}
	}
}

// TestProduksiLoopbackTidakDiperingatkan menjaga agar peringatan TLS tidak berisik pada
// pemasangan yang lazim: Redis di host yang sama, diakses lewat loopback.
func TestProduksiLoopbackTidakDiperingatkan(t *testing.T) {
	mr := miniredis.RunT(t)
	logger, logBuf := loggerUji()
	cfg := &config.Config{
		AppEnv:   config.EnvProduction,
		RedisURL: security.Secret("redis://" + mr.Addr() + "/0"),
	}
	r, err := Connect(context.Background(), cfg, logger)
	if err != nil {
		t.Fatalf("Connect: %v", err)
	}
	t.Cleanup(func() { _ = r.Close() })

	if strings.Contains(logBuf.String(), "tidak terenkripsi") {
		t.Fatalf("loopback di produksi diperingatkan tanpa perlu:\n%s", logBuf.String())
	}
}

// tungguSampai menunggu sebuah kondisi terpenuhi. Dipakai untuk hal yang terjadi asinkron
// di sisi server, seperti penutupan socket.
func tungguSampai(t *testing.T, batas time.Duration, pesan string, kondisi func() bool) {
	t.Helper()
	deadline := time.Now().Add(batas)
	for time.Now().Before(deadline) {
		if kondisi() {
			return
		}
		time.Sleep(5 * time.Millisecond)
	}
	t.Fatalf("kondisi tidak terpenuhi dalam %s: %s", batas, pesan)
}

// --- Test integrasi ---------------------------------------------------------
//
// Test di bawah menyentuh Redis sungguhan. Aturannya ketat karena instance yang dipakai
// bisa jadi instance development yang sedang berisi data: setiap test memakai prefix kunci
// unik, membersihkan tepat kunci miliknya sendiri di t.Cleanup, dan tidak pernah memanggil
// FLUSHDB atau FLUSHALL.

// dsnIntegrasi mengambil DSN untuk test integrasi, atau melewati test bila tidak ada.
func dsnIntegrasi(t *testing.T) string {
	t.Helper()
	// TEST_REDIS_URL didahulukan supaya CI bisa mengarahkan test ke instance terpisah
	// tanpa mengubah REDIS_URL yang dipakai aplikasi.
	for _, name := range []string{"TEST_REDIS_URL", "REDIS_URL"} {
		if v := strings.TrimSpace(os.Getenv(name)); v != "" {
			return v
		}
	}
	t.Skip("butuh Redis: setel TEST_REDIS_URL atau REDIS_URL")
	return ""
}

// prefixUjiUnik membuat prefix kunci yang tidak mungkin bertabrakan dengan data
// development maupun dengan test lain yang berjalan bersamaan.
func prefixUjiUnik(t *testing.T) string {
	t.Helper()
	var b [8]byte
	if _, err := rand.Read(b[:]); err != nil {
		t.Fatalf("membangkitkan prefix acak: %v", err)
	}
	return Key("test", t.Name(), hex.EncodeToString(b[:]))
}

// bersihkanPrefix menghapus semua kunci di bawah satu prefix lewat SCAN + DEL.
//
// Sengaja bukan FLUSHDB: database yang sama bisa memuat data development, atau bahkan data
// aplikasi lain kalau operator memakai instance bersama. Menghapus isinya bukan wewenang
// sebuah test.
func bersihkanPrefix(t *testing.T, r *Redis, prefix string) {
	t.Helper()
	ctx := context.Background()

	var cursor uint64
	var dihapus int
	for {
		keys, next, err := r.Client().Scan(ctx, cursor, prefix+"*", 200).Result()
		if err != nil {
			t.Errorf("SCAN saat bersih-bersih: %v", err)
			return
		}
		if len(keys) > 0 {
			if err := r.Client().Del(ctx, keys...).Err(); err != nil {
				t.Errorf("DEL saat bersih-bersih: %v", err)
				return
			}
			dihapus += len(keys)
		}
		cursor = next
		if cursor == 0 {
			break
		}
	}

	// Pastikan benar-benar bersih, supaya kebocoran kunci test terlihat sekarang dan
	// bukan menumpuk di instance development.
	sisa, err := r.Client().Keys(ctx, prefix+"*").Result()
	if err != nil {
		t.Errorf("verifikasi bersih-bersih: %v", err)
		return
	}
	if len(sisa) > 0 {
		t.Errorf("masih ada %d kunci di bawah %q setelah bersih-bersih: %v", len(sisa), prefix, sisa)
	}
	t.Logf("bersih-bersih: %d kunci di bawah %q dihapus", dihapus, prefix)
}

// redisIntegrasi menyambung ke Redis sungguhan dan menyiapkan prefix kunci sekali pakai.
func redisIntegrasi(t *testing.T) (*Redis, string) {
	t.Helper()

	dsn := dsnIntegrasi(t)
	logger, _ := loggerUji()
	cfg := &config.Config{AppEnv: config.EnvDevelopment, RedisURL: security.Secret(dsn)}

	r, err := Connect(context.Background(), cfg, logger)
	if err != nil {
		t.Fatalf("Connect ke redis integrasi: %v", err)
	}
	// Cleanup berjalan LIFO: bersih-bersih dulu, penutupan koneksi belakangan.
	t.Cleanup(func() { _ = r.Close() })

	prefix := prefixUjiUnik(t)
	t.Cleanup(func() { bersihkanPrefix(t, r, prefix) })

	return r, prefix
}

func TestIntegrationRedisRoundTrip(t *testing.T) {
	if testing.Short() {
		t.Skip("butuh Redis")
	}
	r, prefix := redisIntegrasi(t)
	ctx := context.Background()

	key := prefix + ":roundtrip"
	if err := r.Client().Set(ctx, key, "nilai-uji", time.Minute).Err(); err != nil {
		t.Fatalf("SET: %v", err)
	}
	got, err := r.Client().Get(ctx, key).Result()
	if err != nil {
		t.Fatalf("GET: %v", err)
	}
	if got != "nilai-uji" {
		t.Fatalf("GET = %q, mau %q", got, "nilai-uji")
	}

	if _, err := r.Client().Get(ctx, prefix+":tidak-ada").Result(); err != redis.Nil {
		t.Fatalf("GET kunci kosong = %v, mau redis.Nil", err)
	}

	// Ping dan Stat juga harus benar di Redis sungguhan, karena keduanya menyuplai /readyz.
	if err := r.Ping(ctx); err != nil {
		t.Fatalf("Ping: %v", err)
	}
	if s := r.Stat(); s.TotalConns == 0 || s.PoolSize == 0 {
		t.Fatalf("Stat tampak tidak terisi: %+v", s)
	}
}

func TestIntegrationRedisTTLBerlaku(t *testing.T) {
	if testing.Short() {
		t.Skip("butuh Redis")
	}
	r, prefix := redisIntegrasi(t)
	ctx := context.Background()

	const ttl = 30 * time.Second
	kunciBerTTL := prefix + ":dengan-ttl"
	kunciAbadi := prefix + ":tanpa-ttl"

	if err := r.Client().Set(ctx, kunciBerTTL, "v", ttl).Err(); err != nil {
		t.Fatalf("SET dengan TTL: %v", err)
	}
	if err := r.Client().Set(ctx, kunciAbadi, "v", 0).Err(); err != nil {
		t.Fatalf("SET tanpa TTL: %v", err)
	}

	sisa, err := r.Client().TTL(ctx, kunciBerTTL).Result()
	if err != nil {
		t.Fatalf("TTL: %v", err)
	}
	if sisa <= 0 || sisa > ttl {
		t.Fatalf("TTL = %s, mau di antara 0 dan %s", sisa, ttl)
	}

	// -1 berarti kunci ada tanpa kedaluwarsa; go-redis melaporkannya sebagai -1s.
	abadi, err := r.Client().TTL(ctx, kunciAbadi).Result()
	if err != nil {
		t.Fatalf("TTL kunci abadi: %v", err)
	}
	if abadi >= 0 {
		t.Fatalf("TTL kunci tanpa kedaluwarsa = %s, mau negatif", abadi)
	}

	// Kedaluwarsa yang benar-benar terjadi diuji dengan jendela pendek.
	kunciSingkat := prefix + ":singkat"
	if err := r.Client().Set(ctx, kunciSingkat, "v", 300*time.Millisecond).Err(); err != nil {
		t.Fatalf("SET TTL singkat: %v", err)
	}
	tungguSampai(t, 5*time.Second, "kunci ber-TTL singkat tidak kedaluwarsa", func() bool {
		n, err := r.Client().Exists(ctx, kunciSingkat).Result()
		return err == nil && n == 0
	})
}

// TestIntegrationRedisSkripLuaAtomik adalah alasan utama rate limiter memakai Lua: dengan
// 50 penantang berebut kuota 10, jumlah yang lolos harus tepat 10 dan nomor hit-nya harus
// membentuk 1..10 tanpa duplikat. Kalau operasinya tidak atomik, akan ada nomor yang
// terbit dua kali atau kuota yang terlampaui.
func TestIntegrationRedisSkripLuaAtomik(t *testing.T) {
	if testing.Short() {
		t.Skip("butuh Redis")
	}
	r, prefix := redisIntegrasi(t)
	ctx := context.Background()

	const (
		limit     = 10
		penantang = 50
		windowMS  = 10000
	)
	key := prefix + ":rl"

	// Pemanasan cache skrip lebih dulu, sehingga jalur yang diuji adalah EVALSHA.
	if err := r.LoadScript(ctx, skripUjiRateLimit); err != nil {
		t.Fatalf("LoadScript: %v", err)
	}

	hasil := make([]int64, penantang)
	errs := make([]error, penantang)
	mulai := make(chan struct{})

	var wg sync.WaitGroup
	for i := 0; i < penantang; i++ {
		wg.Add(1)
		go func(i int) {
			defer wg.Done()
			<-mulai // lepas serentak supaya benar-benar berebut
			v, err := r.RunScript(ctx, skripUjiRateLimit, []string{key}, windowMS, limit)
			if err != nil {
				errs[i] = err
				return
			}
			n, ok := v.(int64)
			if !ok {
				errs[i] = fmt.Errorf("hasil bertipe %T, mau int64", v)
				return
			}
			hasil[i] = n
		}(i)
	}
	close(mulai)
	wg.Wait()

	for i, err := range errs {
		if err != nil {
			t.Fatalf("penantang %d gagal: %v", i, err)
		}
	}

	terlihat := make(map[int64]int, limit)
	var lolos int
	for _, n := range hasil {
		if n == 0 {
			continue
		}
		lolos++
		terlihat[n]++
	}
	if lolos != limit {
		t.Fatalf("%d penantang lolos, mau tepat %d", lolos, limit)
	}
	for n := int64(1); n <= limit; n++ {
		if terlihat[n] != 1 {
			t.Fatalf("nomor hit %d muncul %d kali, mau tepat sekali (hasil: %v)", n, terlihat[n], hasil)
		}
	}

	// Skrip juga bertanggung jawab memasang TTL, supaya penghitung jendela lama hilang
	// sendiri dan tidak perlu pekerjaan latar untuk membersihkannya.
	sisa, err := r.Client().PTTL(ctx, key).Result()
	if err != nil {
		t.Fatalf("PTTL: %v", err)
	}
	if sisa <= 0 || sisa > windowMS*time.Millisecond {
		t.Fatalf("TTL penghitung = %s, mau di antara 0 dan %s", sisa, windowMS*time.Millisecond)
	}
}

// TestIntegrationRedisFallbackNoscript membuktikan pemulihan setelah cache skrip Redis
// kosong — yang di produksi terjadi saat Redis di-restart atau di-SCRIPT FLUSH.
//
// SCRIPT FLUSH aman dipakai di sini: ia mengosongkan cache skrip, bukan data.
func TestIntegrationRedisFallbackNoscript(t *testing.T) {
	if testing.Short() {
		t.Skip("butuh Redis")
	}
	r, prefix := redisIntegrasi(t)
	ctx := context.Background()

	skrip := NewScript(`return redis.call('INCR', KEYS[1])`)
	key := prefix + ":noscript"

	if _, err := r.RunScript(ctx, skrip, []string{key}); err != nil {
		t.Fatalf("RunScript pertama: %v", err)
	}
	if err := r.Client().ScriptFlush(ctx).Err(); err != nil {
		t.Fatalf("SCRIPT FLUSH: %v", err)
	}

	v, err := r.RunScript(ctx, skrip, []string{key})
	if err != nil {
		t.Fatalf("RunScript setelah SCRIPT FLUSH: %v", err)
	}
	if n, ok := v.(int64); !ok || n != 2 {
		t.Fatalf("penghitung = %v (%T), mau int64 2", v, v)
	}
}
