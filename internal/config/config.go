// Package config memuat dan memvalidasi seluruh konfigurasi runtime dari environment.
//
// Prinsipnya fail-fast: aplikasi menolak start kalau ada rahasia yang kosong, lemah,
// atau berukuran salah. Semua masalah dikumpulkan dulu lalu dilaporkan sekaligus,
// supaya operator tidak perlu memperbaiki satu error per restart.
package config

import (
	"encoding/base64"
	"errors"
	"fmt"
	"log/slog"
	"net/netip"
	"net/url"
	"os"
	"strconv"
	"strings"
	"time"

	"github.com/joho/godotenv"

	"github.com/NexGen-X/Route-X/internal/security"
)

// Env adalah mode jalan aplikasi.
type Env string

const (
	EnvDevelopment Env = "development"
	EnvStaging     Env = "staging"
	EnvProduction  Env = "production"
)

// IsProduction dipakai untuk mengaktifkan pengerasan yang hanya relevan di produksi
// (cookie Secure wajib, HSTS, larangan rahasia lemah).
func (e Env) IsProduction() bool { return e == EnvProduction }

// Panjang minimum rahasia dalam byte. ENCRYPTION_KEY harus tepat 32 byte karena
// AES-256-GCM mensyaratkan kunci 256-bit.
const (
	encryptionKeyLen     = 32
	minSessionSecretLen  = 32
	minAPIKeyPepperLen   = 32
	defaultMaxRequestMiB = 10
)

// Config memuat seluruh konfigurasi aplikasi. Nilai bertipe security.Secret tidak
// akan pernah tercetak di log walaupun seluruh struct ini di-log.
type Config struct {
	AppEnv    Env
	Port      int
	LogLevel  slog.Level
	PublicURL string

	DatabaseURL security.Secret
	DBMaxConns  int32
	DBMinConns  int32

	RedisURL security.Secret

	// TrustedProxies adalah daftar CIDR proxy yang boleh dipercaya untuk header
	// X-Forwarded-For. Kosong berarti header itu diabaikan sepenuhnya dan IP klien
	// selalu diambil dari RemoteAddr — default aman, karena IP dipakai untuk rate
	// limit dan ban, sehingga spoofing tidak boleh dimungkinkan tanpa konfigurasi
	// eksplisit dari operator.
	TrustedProxies []string

	SessionSecret security.Secret
	EncryptionKey []byte
	APIKeyPepper  []byte

	InitialAdminEmail    string
	InitialAdminPassword security.Secret

	MaxRequestBytes int64
	UpstreamTimeout time.Duration

	// UpstreamAllowHTTP mengizinkan base URL provider berskema http://.
	//
	// Bawaannya false. Yang membutuhkannya adalah upstream yang dijalankan sendiri di
	// jaringan tepercaya (Ollama, vLLM, LM Studio) — bukan api.openai.com, yang selalu
	// https. Karena itu ia setelan, bukan bawaan: mengizinkan http untuk semua provider
	// berarti kredensial upstream bisa terkirim tanpa enkripsi ke host mana pun yang
	// diketikkan operator.
	UpstreamAllowHTTP bool

	// UpstreamAllowedPrivateAddrs adalah alamat privat yang boleh dihubungi meski penjaga
	// SSRF menolak seluruh rentang privat.
	//
	// Bentuknya ALAMAT, bukan nama host, dan itu keharusan bukan pilihan: di lapisan dial
	// nama host sudah hilang, jadi pengecualian bernama hanya bisa bekerja dengan
	// meresolusi nama saat kebijakan dibuat lalu mempercayai hasilnya saat menghubungi —
	// yaitu DNS rebinding yang justru dijaga lapisan itu.
	//
	// Contoh isi untuk Ollama di mesin yang sama: "127.0.0.1,::1".
	UpstreamAllowedPrivateAddrs []netip.Prefix
	ShutdownGrace               time.Duration
	ReadTimeout                 time.Duration
	WriteTimeout                time.Duration

	RequestLogRetentionDays  int
	RequestBodyRetentionDays int

	HealthCheckInterval time.Duration
	UsageRollupInterval time.Duration
}

// Addr mengembalikan alamat listen untuk http.Server.
func (c *Config) Addr() string { return ":" + strconv.Itoa(c.Port) }

// UpstreamSSRFPolicy menyusun kebijakan SSRF untuk seluruh panggilan ke provider.
//
// Satu tempat, bukan disusun ulang di setiap pemanggil: kebijakan yang berbeda antar jalur
// berarti satu jalur yang lebih longgar dari yang lain, dan yang paling longgar itulah yang
// menentukan apa yang benar-benar bisa dihubungi.
func (c *Config) UpstreamSSRFPolicy() security.SSRFPolicy {
	return security.SSRFPolicy{
		AllowHTTP:           c.UpstreamAllowHTTP,
		AllowedPrivateAddrs: c.UpstreamAllowedPrivateAddrs,
	}
}

// Load membaca .env bila ada lalu memuat konfigurasi dari environment.
//
// Variabel yang sudah ada di environment menang atas isi .env, sehingga systemd unit
// atau secret manager bisa menimpa file tanpa mengeditnya.
func Load() (*Config, error) {
	// Absennya .env bukan error: di produksi konfigurasi datang dari environment.
	if err := godotenv.Load(); err != nil && !errors.Is(err, os.ErrNotExist) {
		var pathErr *os.PathError
		if !errors.As(err, &pathErr) {
			return nil, fmt.Errorf("membaca .env: %w", err)
		}
	}
	return loadFrom(os.LookupEnv)
}

// lookupFunc memungkinkan test memuat konfigurasi tanpa menyentuh environment proses.
type lookupFunc func(string) (string, bool)

func loadFrom(lookup lookupFunc) (*Config, error) {
	r := &reader{lookup: lookup}

	cfg := &Config{
		AppEnv:    Env(r.oneOf("APP_ENV", string(EnvDevelopment), string(EnvDevelopment), string(EnvStaging), string(EnvProduction))),
		Port:      r.intRange("PORT", 8080, 1, 65535),
		LogLevel:  r.logLevel("LOG_LEVEL", slog.LevelInfo),
		PublicURL: r.str("PUBLIC_URL", ""),

		DatabaseURL: r.requiredURL("DATABASE_URL", "postgres", "postgresql"),
		DBMaxConns:  int32(r.intRange("DB_MAX_CONNS", 20, 1, 1000)),
		DBMinConns:  int32(r.intRange("DB_MIN_CONNS", 2, 0, 1000)),

		RedisURL:       r.requiredURL("REDIS_URL", "redis", "rediss", "unix"),
		TrustedProxies: r.csv("TRUSTED_PROXIES"),

		SessionSecret: r.requiredSecret("SESSION_SECRET", minSessionSecretLen),
		EncryptionKey: r.requiredKeyBytes("ENCRYPTION_KEY", encryptionKeyLen),
		APIKeyPepper:  r.requiredKeyBytesMin("API_KEY_PEPPER", minAPIKeyPepperLen),

		InitialAdminEmail:    r.str("INITIAL_ADMIN_EMAIL", ""),
		InitialAdminPassword: security.Secret(r.str("INITIAL_ADMIN_PASSWORD", "")),

		MaxRequestBytes: int64(r.intRange("MAX_REQUEST_MIB", defaultMaxRequestMiB, 1, 1024)) << 20,
		UpstreamTimeout: r.duration("UPSTREAM_TIMEOUT", 120*time.Second),

		UpstreamAllowHTTP:           r.boolean("UPSTREAM_ALLOW_HTTP", false),
		UpstreamAllowedPrivateAddrs: r.privateAddrs("UPSTREAM_ALLOWED_PRIVATE_ADDRS"),
		ShutdownGrace:               r.duration("SHUTDOWN_GRACE", 25*time.Second),
		ReadTimeout:                 r.duration("READ_TIMEOUT", 30*time.Second),
		WriteTimeout:                r.duration("WRITE_TIMEOUT", 0), // 0 = tanpa batas, wajib untuk SSE

		RequestLogRetentionDays:  r.intRange("REQUEST_LOG_RETENTION_DAYS", 30, 1, 3650),
		RequestBodyRetentionDays: r.intRange("REQUEST_BODY_RETENTION_DAYS", 7, 1, 3650),

		HealthCheckInterval: r.duration("HEALTH_CHECK_INTERVAL", 60*time.Second),
		UsageRollupInterval: r.duration("USAGE_ROLLUP_INTERVAL", 5*time.Minute),
	}

	cfg.validate(r)

	if len(r.errs) > 0 {
		return nil, fmt.Errorf("konfigurasi tidak valid:\n  - %s", strings.Join(r.errs, "\n  - "))
	}
	return cfg, nil
}

// validate memeriksa aturan yang melibatkan lebih dari satu variabel sekaligus.
func (c *Config) validate(r *reader) {
	if c.DBMinConns > c.DBMaxConns {
		r.fail("DB_MIN_CONNS (%d) tidak boleh lebih besar dari DB_MAX_CONNS (%d)", c.DBMinConns, c.DBMaxConns)
	}
	if c.RequestBodyRetentionDays > c.RequestLogRetentionDays {
		r.fail("REQUEST_BODY_RETENTION_DAYS (%d) tidak boleh melebihi REQUEST_LOG_RETENTION_DAYS (%d)",
			c.RequestBodyRetentionDays, c.RequestLogRetentionDays)
	}
	if c.InitialAdminEmail != "" && !strings.Contains(c.InitialAdminEmail, "@") {
		r.fail("INITIAL_ADMIN_EMAIL bukan alamat email yang sah")
	}

	if !c.AppEnv.IsProduction() {
		return
	}
	// Pengerasan khusus produksi.
	if c.PublicURL == "" {
		r.fail("PUBLIC_URL wajib diisi saat APP_ENV=production")
	} else if u, err := url.Parse(c.PublicURL); err != nil || u.Scheme != "https" {
		r.fail("PUBLIC_URL harus URL https yang sah saat APP_ENV=production")
	}
	if strings.Contains(c.DatabaseURL.Reveal(), "sslmode=disable") {
		r.fail("DATABASE_URL tidak boleh memakai sslmode=disable saat APP_ENV=production")
	}
	if pw := c.InitialAdminPassword; !pw.IsZero() && pw.Len() < 16 {
		r.fail("INITIAL_ADMIN_PASSWORD minimal 16 karakter saat APP_ENV=production")
	}
}

// reader membaca environment sambil mengumpulkan seluruh kesalahan validasi.
type reader struct {
	lookup lookupFunc
	errs   []string
}

func (r *reader) fail(format string, args ...any) {
	r.errs = append(r.errs, fmt.Sprintf(format, args...))
}

func (r *reader) raw(key string) (string, bool) {
	v, ok := r.lookup(key)
	if !ok {
		return "", false
	}
	v = strings.TrimSpace(v)
	return v, v != ""
}

func (r *reader) str(key, def string) string {
	if v, ok := r.raw(key); ok {
		return v
	}
	return def
}

// csv membaca daftar yang dipisah koma, membuang spasi dan entri kosong.
func (r *reader) csv(key string) []string {
	v, ok := r.raw(key)
	if !ok {
		return nil
	}
	var out []string
	for _, part := range strings.Split(v, ",") {
		if part = strings.TrimSpace(part); part != "" {
			out = append(out, part)
		}
	}
	return out
}

func (r *reader) oneOf(key, def string, allowed ...string) string {
	v := r.str(key, def)
	for _, a := range allowed {
		if v == a {
			return v
		}
	}
	r.fail("%s=%q tidak dikenal, pilih salah satu dari: %s", key, v, strings.Join(allowed, ", "))
	return def
}

func (r *reader) intRange(key string, def, min, max int) int {
	v, ok := r.raw(key)
	if !ok {
		return def
	}
	n, err := strconv.Atoi(v)
	if err != nil {
		r.fail("%s=%q bukan bilangan bulat", key, v)
		return def
	}
	if n < min || n > max {
		r.fail("%s=%d di luar rentang yang diizinkan (%d..%d)", key, n, min, max)
		return def
	}
	return n
}

func (r *reader) duration(key string, def time.Duration) time.Duration {
	v, ok := r.raw(key)
	if !ok {
		return def
	}
	d, err := time.ParseDuration(v)
	if err != nil {
		r.fail("%s=%q bukan durasi yang sah (contoh: 30s, 2m, 1h)", key, v)
		return def
	}
	if d < 0 {
		r.fail("%s=%s tidak boleh negatif", key, v)
		return def
	}
	return d
}

func (r *reader) logLevel(key string, def slog.Level) slog.Level {
	v, ok := r.raw(key)
	if !ok {
		return def
	}
	switch strings.ToLower(v) {
	case "debug":
		return slog.LevelDebug
	case "info":
		return slog.LevelInfo
	case "warn", "warning":
		return slog.LevelWarn
	case "error":
		return slog.LevelError
	default:
		r.fail("%s=%q tidak dikenal, pilih: debug, info, warn, error", key, v)
		return def
	}
}

// requiredURL memastikan connection string ada dan skemanya benar, tanpa pernah
// menaruh nilainya di pesan error.
func (r *reader) requiredURL(key string, schemes ...string) security.Secret {
	v, ok := r.raw(key)
	if !ok {
		r.fail("%s wajib diisi", key)
		return ""
	}
	u, err := url.Parse(v)
	if err != nil {
		r.fail("%s bukan URL yang sah", key)
		return ""
	}
	for _, s := range schemes {
		if u.Scheme == s {
			return security.Secret(v)
		}
	}
	r.fail("%s memakai skema %q, yang didukung: %s", key, u.Scheme, strings.Join(schemes, ", "))
	return ""
}

func (r *reader) requiredSecret(key string, minLen int) security.Secret {
	v, ok := r.raw(key)
	if !ok {
		r.fail("%s wajib diisi (hasilkan dengan: openssl rand -base64 48)", key)
		return ""
	}
	if len(v) < minLen {
		r.fail("%s terlalu pendek: %d karakter, minimal %d", key, len(v), minLen)
		return ""
	}
	return security.Secret(v)
}

// requiredKeyBytes mendekode kunci base64 dan mewajibkan panjang byte yang tepat.
func (r *reader) requiredKeyBytes(key string, wantLen int) []byte {
	b := r.decodeKey(key)
	if b == nil {
		return nil
	}
	if len(b) != wantLen {
		r.fail("%s harus tepat %d byte setelah didekode base64, dapat %d (hasilkan dengan: openssl rand -base64 %d)",
			key, wantLen, len(b), wantLen)
		return nil
	}
	return b
}

func (r *reader) requiredKeyBytesMin(key string, minLen int) []byte {
	b := r.decodeKey(key)
	if b == nil {
		return nil
	}
	if len(b) < minLen {
		r.fail("%s minimal %d byte setelah didekode base64, dapat %d", key, minLen, len(b))
		return nil
	}
	return b
}

func (r *reader) decodeKey(key string) []byte {
	v, ok := r.raw(key)
	if !ok {
		r.fail("%s wajib diisi (hasilkan dengan: openssl rand -base64 32)", key)
		return nil
	}
	b, err := base64.StdEncoding.DecodeString(v)
	if err != nil {
		r.fail("%s bukan base64 yang sah", key)
		return nil
	}
	return b
}

// boolean membaca setelan boolean.
//
// Yang diterima hanya bentuk yang dikenal strconv.ParseBool. Nilai yang tidak dikenal
// menjadi error, bukan diperlakukan sebagai false: setelan yang mengendurkan penjagaan
// keamanan tidak boleh salah dibaca ke arah mana pun tanpa satu pun keluhan, dan "yes"
// atau "on" adalah yang paling mungkin diketikkan operator.
func (r *reader) boolean(key string, def bool) bool {
	v, ok := r.raw(key)
	if !ok {
		return def
	}
	b, err := strconv.ParseBool(v)
	if err != nil {
		r.fail("%s harus true atau false, dapat %q", key, v)
		return def
	}
	return b
}

// privateAddrs membaca daftar alamat atau prefix yang dikecualikan penjaga SSRF.
//
// Diurai di sini, bukan di titik pemakaian, supaya nilai yang salah menggagalkan START
// dengan pesan jelas. Kalau diurai saat request pertama, kesalahan ketik pada setelan ini
// muncul sebagai provider yang tidak bisa dihubungi — gejala yang menuntun operator
// menyelidiki jaringan, bukan berkas konfigurasinya.
func (r *reader) privateAddrs(key string) []netip.Prefix {
	values := r.csv(key)
	if len(values) == 0 {
		return nil
	}
	prefixes, err := security.ParsePrivateAddrs(values)
	if err != nil {
		r.fail("%s: %v", key, err)
		return nil
	}
	return prefixes
}
