package database

import (
	"bytes"
	"context"
	"crypto/rand"
	"encoding/hex"
	"errors"
	"log/slog"
	"net/url"
	"os"
	"strings"
	"testing"
	"time"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"

	"github.com/NexGen-X/Route-X/internal/config"
	"github.com/NexGen-X/Route-X/internal/observability"
	"github.com/NexGen-X/Route-X/internal/security"
)

// testPassword sengaja dibuat ganjil supaya kemunculannya di sebuah pesan error tidak
// mungkin kebetulan.
const testPassword = "sup3r-s3cr3t-passw0rd-xyzzy"

// discardLogger membuang seluruh log supaya keluaran test tetap bersih.
func discardLogger() *slog.Logger { return slog.New(slog.DiscardHandler) }

// testConfig merakit config minimal yang dibutuhkan Connect.
func testConfig(dsn string) *config.Config {
	return &config.Config{
		AppEnv:      config.EnvDevelopment,
		DatabaseURL: security.Secret(dsn),
		DBMaxConns:  4,
		DBMinConns:  0,
	}
}

// assertNoLeak memastikan sebuah pesan tidak membocorkan password maupun connection
// string.
//
// Pesannya sendiri sengaja tidak pernah dicetak: kalau assertion ini gagal, isi pesan
// itu justru yang tidak boleh masuk keluaran test.
func assertNoLeak(t *testing.T, what, msg, dsn, password string) {
	t.Helper()
	if password != "" && strings.Contains(msg, password) {
		t.Errorf("%s memuat password (panjang pesan %d karakter)", what, len(msg))
	}
	if dsn != "" && strings.Contains(msg, dsn) {
		t.Errorf("%s memuat connection string utuh", what)
	}
}

// DSN rusak harus ditolak sebelum ada usaha menyentuh jaringan, dan pesan errornya
// tidak boleh membawa password. Ini alasan utama keberadaan scrubDSN: pgx menempelkan
// connection string ke pesan errornya sendiri.
func TestConnectRejectsBadDSN(t *testing.T) {
	cases := []struct {
		name string
		dsn  string
	}{
		{
			name: "skema bukan postgres",
			dsn:  "mysql://routex:" + testPassword + "@127.0.0.1:3306/routex",
		},
		{
			name: "bukan URL maupun keyword value",
			dsn:  "://" + testPassword,
		},
		{
			name: "port bukan angka",
			dsn:  "postgres://routex:" + testPassword + "@127.0.0.1:port/routex",
		},
		{
			// Kasus yang lolos dari penyamaran bawaan pgx: password dikirim sebagai
			// query parameter, bukan di userinfo.
			name: "password di query parameter",
			dsn:  "postgres://routex@127.0.0.1:5432/routex?password=" + testPassword + "&sslmode=entahapa",
		},
		{
			name: "bentuk keyword value dengan sslmode salah",
			dsn:  "host=127.0.0.1 user=routex password=" + testPassword + " sslmode=entahapa",
		},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			// Batas waktu ketat: kalau Connect sampai mencoba dial, test ini gagal
			// karena lambat, bukan menunggu timeout jaringan.
			ctx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
			defer cancel()

			db, err := Connect(ctx, testConfig(tc.dsn), discardLogger())
			if err == nil {
				db.Close()
				t.Fatal("Connect berhasil padahal DSN tidak sah")
			}
			if !errors.Is(err, ErrInvalidDSN) {
				t.Errorf("errors.Is(err, ErrInvalidDSN) = false")
			}
			assertNoLeak(t, "pesan error Connect", err.Error(), tc.dsn, testPassword)
		})
	}
}

// DSN kosong tidak boleh diteruskan ke pgx: ParseConfig("") justru berhasil dan
// jatuh ke default libpq (localhost, user OS), jadi salah konfigurasi akan terlihat
// sebagai kegagalan koneksi yang membingungkan.
func TestConnectRejectsEmptyDSN(t *testing.T) {
	for _, dsn := range []string{"", "   "} {
		db, err := Connect(context.Background(), testConfig(dsn), discardLogger())
		if err == nil {
			db.Close()
			t.Fatalf("Connect(%q) berhasil padahal DATABASE_URL kosong", dsn)
		}
		if !errors.Is(err, ErrInvalidDSN) {
			t.Errorf("Connect(%q): errors.Is(err, ErrInvalidDSN) = false, err = %v", dsn, err)
		}
	}
}

func TestConnectRejectsNilConfig(t *testing.T) {
	if _, err := Connect(context.Background(), nil, discardLogger()); !errors.Is(err, ErrInvalidDSN) {
		t.Errorf("Connect(nil) = %v, ingin ErrInvalidDSN", err)
	}
}

// Logger nil harus diterima: bukan alasan panic di jalur startup.
func TestConnectAcceptsNilLogger(t *testing.T) {
	if _, err := Connect(context.Background(), testConfig("://tidak-sah"), nil); err == nil {
		t.Fatal("ingin error untuk DSN tidak sah")
	}
}

func TestScrubDSN(t *testing.T) {
	cases := []struct {
		name string
		msg  string
		dsn  string
		// want boleh kosong kalau yang diperiksa hanya ketiadaan rahasia.
		want string
	}{
		{
			name: "awalan pgx dengan DSN di antara backtick dibuang",
			msg:  "cannot parse `postgres://routex:" + testPassword + "@db:5432/routex`: failed to configure TLS (sslmode is invalid: entahapa)",
			dsn:  "postgres://routex:" + testPassword + "@db:5432/routex",
			want: "failed to configure TLS (sslmode is invalid: entahapa)",
		},
		{
			name: "password di userinfo",
			msg:  "gagal: postgres://routex:" + testPassword + "@db:5432/routex",
			dsn:  "postgres://routex:" + testPassword + "@db:5432/routex",
			want: "gagal: " + redactedMark,
		},
		{
			name: "password di query parameter",
			msg:  "sslmode is invalid, password=" + testPassword + " terbaca",
			dsn:  "postgres://routex@db:5432/routex?password=" + testPassword,
			want: "sslmode is invalid, password=" + redactedMark + " terbaca",
		},
		{
			name: "password bentuk keyword value berkutip",
			msg:  "nilai " + testPassword + " tidak diterima",
			dsn:  "host=db user=routex password='" + testPassword + "' sslmode=require",
			want: "nilai " + redactedMark + " tidak diterima",
		},
		{
			name: "password ter-escape di URL juga tersamar saat sudah didecode",
			msg:  `parse "postgres://routex:p%40ss@db/routex": ada masalah dengan p@ss`,
			dsn:  "postgres://routex:p%40ss@db/routex",
			want: `parse "` + redactedMark + `": ada masalah dengan ` + redactedMark,
		},
		{
			name: "pesan tanpa rahasia dibiarkan utuh",
			msg:  "server closed the connection unexpectedly",
			dsn:  "postgres://routex:" + testPassword + "@db:5432/routex",
			want: "server closed the connection unexpectedly",
		},
		{
			name: "pesan yang seluruhnya rahasia diganti keterangan",
			msg:  "cannot parse `postgres://x`: ",
			dsn:  "postgres://x",
			want: "penyebab tidak bisa ditampilkan tanpa membocorkan connection string",
		},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			got := scrubDSN(tc.msg, tc.dsn)
			if got != tc.want {
				t.Errorf("scrubDSN() = %q, ingin %q", got, tc.want)
			}
			assertNoLeak(t, "hasil scrubDSN", got, tc.dsn, testPassword)
		})
	}
}

// Ukuran pool adalah keputusan environment aplikasi; parameter pool_* di dalam DSN
// sengaja tidak dihormati supaya tidak ada dua sumber kebenaran.
func TestApplyPoolSettings(t *testing.T) {
	cases := []struct {
		name             string
		dsn              string
		max, min         int32
		wantMax, wantMin int32
	}{
		{
			name: "config menang atas pool_max_conns di DSN",
			dsn:  "postgres://routex:pw@127.0.0.1:5432/routex?pool_max_conns=99&pool_min_conns=98",
			max:  7, min: 3, wantMax: 7, wantMin: 3,
		},
		{
			name: "max tidak masuk akal jatuh ke bawaan",
			dsn:  "postgres://routex:pw@127.0.0.1:5432/routex",
			max:  0, min: 0, wantMax: fallbackMaxConns, wantMin: 0,
		},
		{
			name: "min di atas max dijepit",
			dsn:  "postgres://routex:pw@127.0.0.1:5432/routex",
			max:  5, min: 50, wantMax: 5, wantMin: 5,
		},
		{
			name: "min negatif dijepit ke nol",
			dsn:  "postgres://routex:pw@127.0.0.1:5432/routex",
			max:  5, min: -1, wantMax: 5, wantMin: 0,
		},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			poolCfg, err := pgxpool.ParseConfig(tc.dsn)
			if err != nil {
				t.Fatalf("ParseConfig: %v", err)
			}
			applyPoolSettings(poolCfg, &config.Config{DBMaxConns: tc.max, DBMinConns: tc.min})

			if poolCfg.MaxConns != tc.wantMax {
				t.Errorf("MaxConns = %d, ingin %d", poolCfg.MaxConns, tc.wantMax)
			}
			if poolCfg.MinConns != tc.wantMin {
				t.Errorf("MinConns = %d, ingin %d", poolCfg.MinConns, tc.wantMin)
			}
			if poolCfg.ConnConfig.ConnectTimeout != connectTimeout {
				t.Errorf("ConnectTimeout = %v, ingin %v", poolCfg.ConnConfig.ConnectTimeout, connectTimeout)
			}
			if poolCfg.MaxConnLifetime != maxConnLifetime || poolCfg.MaxConnIdleTime != maxConnIdleTime {
				t.Errorf("umur koneksi tidak diset: lifetime=%v idle=%v", poolCfg.MaxConnLifetime, poolCfg.MaxConnIdleTime)
			}
			if poolCfg.HealthCheckPeriod != healthCheckPeriod {
				t.Errorf("HealthCheckPeriod = %v, ingin %v", poolCfg.HealthCheckPeriod, healthCheckPeriod)
			}
			if got := poolCfg.ConnConfig.RuntimeParams["application_name"]; got != "route-x" {
				t.Errorf("application_name = %q, ingin %q", got, "route-x")
			}
		})
	}
}

// application_name dari DSN tidak boleh ditimpa: operator kadang memberi nama khusus
// per deployment untuk membedakannya di pg_stat_activity.
func TestApplyPoolSettingsKeepsApplicationNameFromDSN(t *testing.T) {
	poolCfg, err := pgxpool.ParseConfig("postgres://routex:pw@127.0.0.1:5432/routex?application_name=route-x-worker")
	if err != nil {
		t.Fatalf("ParseConfig: %v", err)
	}
	applyPoolSettings(poolCfg, &config.Config{DBMaxConns: 2, DBMinConns: 1})
	if got := poolCfg.ConnConfig.RuntimeParams["application_name"]; got != "route-x-worker" {
		t.Errorf("application_name = %q, ingin nilai dari DSN dipertahankan", got)
	}
}

// Endpoint kesehatan memanggil metode ini justru saat database sedang bermasalah,
// jadi DB yang belum siap harus menjawab, bukan panic.
func TestMethodsSafeWithoutPool(t *testing.T) {
	for _, tc := range []struct {
		name string
		db   *DB
	}{
		{"DB nil", nil},
		{"pool belum terbentuk", &DB{}},
	} {
		t.Run(tc.name, func(t *testing.T) {
			tc.db.Close() // tidak boleh panic, juga saat dipanggil dua kali
			tc.db.Close()

			if err := tc.db.Ping(context.Background()); !errors.Is(err, ErrClosed) {
				t.Errorf("Ping() = %v, ingin ErrClosed", err)
			}
			if got := tc.db.Stat(); got != (PoolStats{}) {
				t.Errorf("Stat() = %+v, ingin PoolStats kosong", got)
			}
			if _, err := Migrate(context.Background(), tc.db, discardLogger()); !errors.Is(err, ErrClosed) {
				t.Errorf("Migrate() = %v, ingin ErrClosed", err)
			}
		})
	}
}

// --- Perkakas test integrasi -------------------------------------------------

// testDSN mengembalikan DSN database untuk test integrasi, atau melewati test kalau
// tidak ada yang bisa dipakai.
//
// TEST_DATABASE_URL diutamakan supaya CI bisa diarahkan ke database sekali pakai
// tanpa menyentuh database development.
func testDSN(t *testing.T) string {
	t.Helper()
	for _, key := range []string{"TEST_DATABASE_URL", "DATABASE_URL"} {
		dsn := strings.TrimSpace(os.Getenv(key))
		if dsn == "" {
			continue
		}
		if !strings.HasPrefix(dsn, "postgres://") && !strings.HasPrefix(dsn, "postgresql://") {
			t.Skipf("%s bukan URL postgres:// sehingga search_path test tidak bisa dipasang", key)
		}
		return dsn
	}
	t.Skip("TEST_DATABASE_URL maupun DATABASE_URL tidak diset — test integrasi dilewati")
	return ""
}

// newTestSchema membuat schema kosong bernama unik dan mengembalikan DSN yang
// search_path-nya sudah mengarah ke schema itu.
//
// Dengan begitu migrasi test tidak pernah menyentuh tabel database development:
// seluruh tabel dibuat di dalam schema sekali pakai, yang dihapus lagi di t.Cleanup.
// search_path dikirim sebagai query parameter DSN karena pgx meneruskan parameter tak
// dikenal sebagai runtime parameter saat startup koneksi, sehingga setiap koneksi
// yang dibuka pool ikut terarahkan.
func newTestSchema(ctx context.Context, t *testing.T, baseDSN string) (dsn, schema string) {
	t.Helper()

	admin, err := pgxpool.New(ctx, baseDSN)
	if err != nil {
		t.Fatalf("membuka koneksi admin: %v", err)
	}
	t.Cleanup(admin.Close)

	buf := make([]byte, 6)
	if _, err := rand.Read(buf); err != nil {
		t.Fatalf("membuat nama schema acak: %v", err)
	}
	schema = "test_mig_" + hex.EncodeToString(buf)
	ident := pgx.Identifier{schema}.Sanitize()

	if _, err := admin.Exec(ctx, "create schema "+ident); err != nil {
		t.Fatalf("membuat schema %s: %v", schema, err)
	}
	t.Cleanup(func() {
		// Context baru: ctx test bisa sudah kedaluwarsa saat pembersihan jalan, dan
		// schema yang tertinggal akan mengotori database development.
		cleanupCtx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
		defer cancel()
		if _, err := admin.Exec(cleanupCtx, "drop schema "+ident+" cascade"); err != nil {
			t.Errorf("membersihkan schema %s: %v", schema, err)
		}
	})

	u, err := url.Parse(baseDSN)
	if err != nil {
		t.Fatalf("DSN test tidak bisa diparse: %v", err)
	}
	q := u.Query()
	q.Set("search_path", schema)
	u.RawQuery = q.Encode()
	return u.String(), schema
}

// countPublicTables menghitung tabel di schema public, dipakai untuk membuktikan test
// integrasi tidak bocor keluar schema sekali pakainya.
func countPublicTables(ctx context.Context, t *testing.T, db *DB) int {
	t.Helper()
	var n int
	err := db.Pool.QueryRow(ctx,
		`select count(*) from information_schema.tables where table_schema = 'public'`).Scan(&n)
	if err != nil {
		t.Fatalf("menghitung tabel di schema public: %v", err)
	}
	return n
}

// passwordOf mengambil password dari sebuah DSN untuk dipakai sebagai pembanding.
// Nilainya tidak pernah dicetak ke mana pun.
func passwordOf(dsn string) string {
	u, err := url.Parse(dsn)
	if err != nil {
		return ""
	}
	pw, _ := u.User.Password()
	return pw
}

// --- Test integrasi ----------------------------------------------------------

// Connect terhadap database sungguhan: pool terbentuk dengan ukuran dari config,
// Ping menjawab, Stat melaporkan angka yang masuk akal, dan tidak ada rahasia yang
// ikut tercetak ke log.
func TestConnectIntegration(t *testing.T) {
	if testing.Short() {
		t.Skip("butuh PostgreSQL")
	}
	baseDSN := testDSN(t)

	ctx, cancel := context.WithTimeout(context.Background(), 60*time.Second)
	defer cancel()

	dsn, _ := newTestSchema(ctx, t, baseDSN)

	// Log diarahkan ke buffer supaya bisa dipastikan tidak ada connection string yang
	// tercetak saat pool dibangun.
	var logs bytes.Buffer
	logger := observability.NewLogger(&logs, observability.LoggerOptions{Level: slog.LevelDebug})

	cfg := testConfig(dsn)
	cfg.DBMaxConns = 3
	cfg.DBMinConns = 1

	db, err := Connect(ctx, cfg, logger)
	if err != nil {
		t.Fatalf("Connect: %v", err)
	}
	defer db.Close()

	if err := db.Ping(ctx); err != nil {
		t.Fatalf("Ping: %v", err)
	}

	st := db.Stat()
	if st.MaxConns != cfg.DBMaxConns {
		t.Errorf("Stat().MaxConns = %d, ingin %d", st.MaxConns, cfg.DBMaxConns)
	}
	if st.TotalConns < 1 {
		t.Errorf("Stat().TotalConns = %d, ingin minimal 1 setelah Ping", st.TotalConns)
	}
	if st.AcquireCount < 1 {
		t.Errorf("Stat().AcquireCount = %d, ingin minimal 1 setelah Ping", st.AcquireCount)
	}
	if st.AcquiredConns != 0 {
		t.Errorf("Stat().AcquiredConns = %d, ingin 0 karena semua koneksi sudah dilepas", st.AcquiredConns)
	}

	assertNoLeak(t, "log Connect", logs.String(), baseDSN, passwordOf(baseDSN))

	// Setelah pool ditutup, probe kesehatan tetap harus menjawab dengan error, bukan
	// panic: /readyz dipanggil justru saat shutdown sedang berjalan.
	db.Close()
	if err := db.Ping(ctx); err == nil {
		t.Error("Ping setelah Close berhasil, ingin error")
	}
}
