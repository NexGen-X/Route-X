// Package database menyediakan pool koneksi PostgreSQL dan runner migrasi yang
// dipakai seluruh aplikasi.
//
// Ada dua hal yang dijaga ketat di sini:
//
//  1. Connection string tidak pernah bocor. DSN memuat password database, sementara
//     pgx dengan sengaja menyisipkan connection string ke dalam pesan errornya.
//     Semua error yang keluar dari paket ini karena itu disaring lebih dulu; lihat
//     scrubDSN untuk alasan lengkapnya.
//  2. Migrasi aman dijalankan banyak instance sekaligus. Runner mengambil advisory
//     lock sebelum menyentuh skema, jadi beberapa replika yang start bersamaan tidak
//     saling menimpa; lihat Migrate.
package database

import (
	"context"
	"errors"
	"fmt"
	"log/slog"
	"net/url"
	"regexp"
	"strings"
	"time"

	"github.com/jackc/pgx/v5/pgxpool"

	"github.com/NexGen-X/Route-X/internal/config"
	"github.com/NexGen-X/Route-X/internal/security"
)

// Sentinel error supaya pemanggil bisa membedakan salah konfigurasi — yang tidak
// akan sembuh dengan retry, jadi aplikasi lebih baik gagal start — dari database
// yang sekadar belum siap.
var (
	// ErrInvalidDSN berarti DATABASE_URL salah bentuk atau kosong.
	ErrInvalidDSN = errors.New("DATABASE_URL tidak valid")
	// ErrUnavailable berarti database tidak bisa dihubungi atau tidak menjawab.
	ErrUnavailable = errors.New("database tidak bisa dihubungi")
	// ErrClosed berarti DB dipakai padahal poolnya belum terbentuk atau sudah ditutup.
	ErrClosed = errors.New("pool database belum siap")
)

// Batas waktu dan umur koneksi. Nilai-nilai ini dipilih untuk produksi di belakang
// pgbouncer/NAT gateway, bukan untuk laptop, karena itu tetap dipakai apa adanya di
// semua environment agar perilaku development sama dengan produksi.
const (
	// connectTimeout membatasi dial + TLS handshake + autentikasi satu koneksi baru.
	// Tanpa ini, host yang paket SYN-nya di-drop firewall bikin request menggantung
	// sampai timeout TCP kernel (bisa lebih dari dua menit).
	connectTimeout = 10 * time.Second

	// verifyTimeout membatasi ping verifikasi saat start: cukup untuk satu dial penuh
	// plus satu round-trip.
	verifyTimeout = connectTimeout + 5*time.Second

	// pingTimeout membatasi Ping. Probe /readyz harus menjawab cepat; lebih baik
	// melaporkan "belum siap" daripada menahan probe sampai kena timeout-nya sendiri.
	pingTimeout = 5 * time.Second

	// maxConnLifetime sengaja di bawah satu jam supaya koneksi ikut berdaur ulang
	// mengikuti idle timeout NAT gateway dan supaya failover primary/replica tidak
	// menyisakan koneksi basi selamanya.
	maxConnLifetime = 55 * time.Minute

	// maxConnLifetimeJitter mencegah seluruh pool mati bersamaan lalu menabrak
	// database dengan badai koneksi baru.
	maxConnLifetimeJitter = 5 * time.Minute

	// maxConnIdleTime melepas koneksi yang tidak terpakai supaya di luar jam sibuk
	// aplikasi tidak menahan slot koneksi database.
	maxConnIdleTime = 5 * time.Minute

	// healthCheckPeriod adalah jarak antar pemeriksaan koneksi idle sekaligus jeda
	// pengisian pool sampai MinConns.
	healthCheckPeriod = 30 * time.Second

	// fallbackMaxConns dipakai kalau config tidak memberi ukuran pool yang masuk akal.
	// config.Load() selalu memberi nilai sah, jadi ini hanya jaring untuk Config yang
	// dirakit manual (test, tool sekali jalan).
	fallbackMaxConns int32 = 20
)

// DB membungkus pool koneksi pgx.
//
// Pool diekspor supaya paket repository bisa memakai pgx langsung tanpa lapisan
// abstraksi yang tidak menambah nilai. Yang tidak diekspor cuma DSN-nya.
type DB struct {
	Pool *pgxpool.Pool

	// dsn disimpan untuk membersihkan pesan error dari jejak connection string.
	// Bertipe security.Secret supaya tetap tersamar kalau DB ikut ter-log.
	dsn security.Secret
}

// Connect membangun pool koneksi dan memastikan database benar-benar menjawab.
//
// Ukuran pool diambil dari config (DB_MAX_CONNS/DB_MIN_CONNS), bukan dari parameter
// pool_* di dalam DSN, supaya cuma ada satu sumber kebenaran. logger boleh nil.
//
// Pemanggil bertanggung jawab memanggil Close saat aplikasi berhenti.
func Connect(ctx context.Context, cfg *config.Config, logger *slog.Logger) (*DB, error) {
	if cfg == nil {
		return nil, fmt.Errorf("%w: config nil", ErrInvalidDSN)
	}
	logger = loggerOr(logger)

	dsn := cfg.DatabaseURL.Reveal()
	if strings.TrimSpace(dsn) == "" {
		return nil, fmt.Errorf("%w: DATABASE_URL kosong", ErrInvalidDSN)
	}

	poolCfg, err := pgxpool.ParseConfig(dsn)
	if err != nil {
		return nil, fmt.Errorf("%w: %s", ErrInvalidDSN, scrubDSN(err.Error(), dsn))
	}
	applyPoolSettings(poolCfg, cfg)

	pool, err := pgxpool.NewWithConfig(ctx, poolCfg)
	if err != nil {
		return nil, fmt.Errorf("%w: menyiapkan pool: %s", ErrUnavailable, scrubDSN(err.Error(), dsn))
	}
	db := &DB{Pool: pool, dsn: cfg.DatabaseURL}

	// pgxpool membuka koneksi secara lazy, jadi host mati atau password salah baru
	// terasa saat query pertama. Satu round-trip dipaksa di sini supaya aplikasi
	// gagal start dengan pesan jelas, bukan gagal di request pertama pengguna.
	verifyCtx, cancel := context.WithTimeout(ctx, verifyTimeout)
	defer cancel()
	if err := db.ping(verifyCtx); err != nil {
		pool.Close()
		return nil, err
	}

	logger.Info("pool database siap",
		slog.String("host", poolCfg.ConnConfig.Host),
		slog.Int("port", int(poolCfg.ConnConfig.Port)),
		slog.String("database", poolCfg.ConnConfig.Database),
		slog.Int("max_conns", int(poolCfg.MaxConns)),
		slog.Int("min_conns", int(poolCfg.MinConns)),
	)
	return db, nil
}

// applyPoolSettings menimpa apa pun yang datang dari DSN dengan kebijakan aplikasi.
//
// Parameter pool_max_conns dan kawan-kawan di dalam DSN sengaja tidak dihormati:
// ukuran pool adalah keputusan kapasitas yang tempatnya di environment aplikasi,
// bukan diselundupkan lewat connection string.
func applyPoolSettings(poolCfg *pgxpool.Config, cfg *config.Config) {
	maxConns := cfg.DBMaxConns
	if maxConns < 1 {
		maxConns = fallbackMaxConns
	}
	minConns := cfg.DBMinConns
	if minConns < 0 {
		minConns = 0
	}
	if minConns > maxConns {
		minConns = maxConns
	}

	poolCfg.MaxConns = maxConns
	poolCfg.MinConns = minConns
	poolCfg.MaxConnLifetime = maxConnLifetime
	poolCfg.MaxConnLifetimeJitter = maxConnLifetimeJitter
	poolCfg.MaxConnIdleTime = maxConnIdleTime
	poolCfg.HealthCheckPeriod = healthCheckPeriod
	poolCfg.ConnConfig.ConnectTimeout = connectTimeout

	// application_name membuat koneksi kita gampang dikenali di pg_stat_activity saat
	// menelusuri query lambat. DSN tetap boleh menentukan nilainya sendiri.
	if poolCfg.ConnConfig.RuntimeParams == nil {
		poolCfg.ConnConfig.RuntimeParams = map[string]string{}
	}
	if poolCfg.ConnConfig.RuntimeParams["application_name"] == "" {
		poolCfg.ConnConfig.RuntimeParams["application_name"] = "route-x"
	}
}

// Close menutup seluruh koneksi dan menunggu yang masih dipakai selesai. Aman
// dipanggil pada DB nil maupun dua kali.
func (db *DB) Close() {
	if db == nil || db.Pool == nil {
		return
	}
	db.Pool.Close()
}

// Ping memastikan database masih menjawab, dipakai probe /readyz.
//
// Batas waktunya selalu dipasang (paling lama pingTimeout, atau lebih cepat kalau
// ctx punya deadline sendiri) supaya probe tidak pernah menggantung.
func (db *DB) Ping(ctx context.Context) error {
	if db == nil || db.Pool == nil {
		return ErrClosed
	}
	ctx, cancel := context.WithTimeout(ctx, pingTimeout)
	defer cancel()
	return db.ping(ctx)
}

// ping menjalankan round-trip tanpa memasang batas waktunya sendiri, supaya
// pemanggil di dalam paket ini bisa memilih batas waktu yang sesuai tahapannya.
func (db *DB) ping(ctx context.Context) error {
	if db == nil || db.Pool == nil {
		return ErrClosed
	}
	if err := db.Pool.Ping(ctx); err != nil {
		return fmt.Errorf("%w: %s", ErrUnavailable, db.scrub(err))
	}
	return nil
}

// scrub membersihkan pesan error dari jejak connection string milik DB ini.
func (db *DB) scrub(err error) string {
	if err == nil {
		return ""
	}
	dsn := ""
	if db != nil {
		dsn = db.dsn.Reveal()
	}
	return scrubDSN(err.Error(), dsn)
}

// PoolStats adalah cuplikan keadaan pool pada satu titik waktu.
//
// Sengaja berupa struct nilai berisi tipe dasar, bukan *pgxpool.Stat, supaya paket
// health dan observability bisa memakainya tanpa ikut mengimpor pgx, dan supaya
// gampang diserialisasi ke respons /readyz.
type PoolStats struct {
	// Ukuran pool saat ini.
	MaxConns          int32 `json:"max_conns"`
	TotalConns        int32 `json:"total_conns"`
	IdleConns         int32 `json:"idle_conns"`
	AcquiredConns     int32 `json:"acquired_conns"`
	ConstructingConns int32 `json:"constructing_conns"`

	// Penghitung kumulatif sepanjang umur proses. AcquireCount naik terus; kalau
	// EmptyAcquireCount ikut naik cepat berarti pool terlalu kecil.
	AcquireCount         int64 `json:"acquire_count"`
	EmptyAcquireCount    int64 `json:"empty_acquire_count"`
	CanceledAcquireCount int64 `json:"canceled_acquire_count"`
	NewConnsCount        int64 `json:"new_conns_count"`

	// Total waktu tunggu, dalam nanodetik saat diserialisasi ke JSON.
	AcquireDuration      time.Duration `json:"acquire_duration_ns"`
	EmptyAcquireWaitTime time.Duration `json:"empty_acquire_wait_time_ns"`
}

// Stat mengembalikan cuplikan keadaan pool untuk /readyz dan metrik Prometheus.
//
// Aman dipanggil pada DB nil atau pool yang sudah ditutup: hasilnya PoolStats
// kosong, bukan panic, supaya endpoint kesehatan tetap bisa menjawab saat database
// justru sedang bermasalah.
func (db *DB) Stat() PoolStats {
	if db == nil || db.Pool == nil {
		return PoolStats{}
	}
	s := db.Pool.Stat()
	return PoolStats{
		MaxConns:             s.MaxConns(),
		TotalConns:           s.TotalConns(),
		IdleConns:            s.IdleConns(),
		AcquiredConns:        s.AcquiredConns(),
		ConstructingConns:    s.ConstructingConns(),
		AcquireCount:         s.AcquireCount(),
		EmptyAcquireCount:    s.EmptyAcquireCount(),
		CanceledAcquireCount: s.CanceledAcquireCount(),
		NewConnsCount:        s.NewConnsCount(),
		AcquireDuration:      s.AcquireDuration(),
		EmptyAcquireWaitTime: s.EmptyAcquireWaitTime(),
	}
}

// loggerOr mengembalikan logger yang pasti bisa dipakai, supaya pemanggil boleh
// mengirim nil.
func loggerOr(l *slog.Logger) *slog.Logger {
	if l == nil {
		return slog.Default()
	}
	return l
}

// --- Penyaring connection string ---------------------------------------------

// redactedMark menggantikan potongan DSN yang berhasil dikenali di pesan error.
const redactedMark = "[REDACTED]"

var (
	// pgxParsePrefix mencocokkan awalan pesan *pgconn.ParseConfigError, yang selalu
	// memuat connection string mentah di antara backtick:
	//
	//	cannot parse `postgres://...`: failed to configure TLS (sslmode is invalid)
	//
	// Yang berguna bagi operator adalah bagian setelah titik dua, jadi awalannya
	// dibuang. Kalau format pgx berubah dan pola ini tidak cocok, tidak ada yang
	// bocor: penggantian di bawah masih jalan.
	pgxParsePrefix = regexp.MustCompile("^cannot parse `[^`]*`: ")

	// passwordKV menangkap password pada DSN bentuk keyword/value, termasuk yang
	// diapit kutip tunggal: "host=db password='rahasia' sslmode=require".
	passwordKV = regexp.MustCompile(`password=(?:'([^']*)'|([^\s&]+))`)

	// userinfoPW menangkap password pada userinfo URL tanpa lewat net/url, supaya
	// DSN yang justru gagal diparse url.Parse tetap tersaring.
	userinfoPW = regexp.MustCompile(`://[^/:@\s]*:([^@/\s]+)@`)
)

// scrubDSN membuang jejak connection string dari sebuah pesan error.
//
// Kenapa ini perlu: pgx menempelkan connection string ke pesan errornya. Penyamaran
// bawaan pgx (pgconn.redactPW) cuma menutup password di userinfo URL dan pada bentuk
// keyword/value, sehingga DSN seperti
//
//	postgres://routex@db:5432/routex?password=rahasia&sslmode=entah
//
// tetap bocor utuh ke log lewat error parse. Kerahasiaan DSN terlalu penting untuk
// digantungkan pada detail implementasi library pihak ketiga, jadi setiap pesan yang
// keluar dari paket ini disaring di sini: DSN utuh maupun setiap potongan yang
// dikenali sebagai password diganti penanda.
func scrubDSN(msg, dsn string) string {
	msg = pgxParsePrefix.ReplaceAllString(msg, "")
	if dsn != "" {
		msg = strings.ReplaceAll(msg, dsn, redactedMark)
		for _, s := range secretsIn(dsn) {
			msg = strings.ReplaceAll(msg, s, redactedMark)
		}
	}
	if strings.TrimSpace(msg) == "" {
		return "penyebab tidak bisa ditampilkan tanpa membocorkan connection string"
	}
	return msg
}

// secretsIn mengumpulkan potongan DSN yang tidak boleh muncul di pesan error:
// password di userinfo URL, password sebagai query parameter, dan password pada
// bentuk keyword/value.
func secretsIn(dsn string) []string {
	var out []string
	add := func(s string) {
		if s == "" {
			return
		}
		out = append(out, s)
		// Bentuk ter-escape dan ter-decode bisa berbeda ("p%40ss" vs "p@ss") dan
		// keduanya bisa muncul di pesan error, jadi dua-duanya disamarkan.
		if dec, err := url.QueryUnescape(s); err == nil && dec != s {
			out = append(out, dec)
		}
	}

	if u, err := url.Parse(dsn); err == nil {
		if pw, ok := u.User.Password(); ok {
			add(pw)
		}
		add(u.Query().Get("password"))
	}
	for _, m := range passwordKV.FindAllStringSubmatch(dsn, -1) {
		add(m[1])
		add(m[2])
	}
	for _, m := range userinfoPW.FindAllStringSubmatch(dsn, -1) {
		add(m[1])
	}
	return out
}
