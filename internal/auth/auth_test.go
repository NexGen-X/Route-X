package auth

import (
	"bytes"
	"context"
	"crypto/rand"
	"encoding/hex"
	"fmt"
	"log/slog"
	"net/url"
	"os"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"

	"github.com/NexGen-X/Route-X/internal/config"
	"github.com/NexGen-X/Route-X/internal/database"
	"github.com/NexGen-X/Route-X/internal/database/repo/identity"
	"github.com/NexGen-X/Route-X/internal/database/seed"
	"github.com/NexGen-X/Route-X/internal/observability"
	"github.com/NexGen-X/Route-X/internal/security"
)

// Nilai uji yang sengaja ganjil supaya kemunculannya di sebuah pesan error, baris log,
// atau body respons tidak mungkin kebetulan. Ketiganya tetap lolos
// security.ValidatePasswordStrength.
const (
	testPassword    = "Kata-Sandi-Uji-Xyzzy-2026"
	testNewPassword = "Kata-Sandi-Baru-Plugh-2026"
)

// testSessionSecret memenuhi panjang minimum SESSION_SECRET sehingga NewCSRF tidak jatuh
// ke kunci acak per proses.
const testSessionSecret = "rahasia-sesi-uji-yang-panjangnya-lebih-dari-32-karakter"

// testSchemaPrefix menandai schema sekali pakai milik paket ini. Awalannya khas per paket
// supaya pemeriksaan sisa schema tidak salah menuduh test paket lain yang mungkin berjalan
// bersamaan di database yang sama.
const testSchemaPrefix = "test_auth_"

// TestMain membungkam logger default, memanaskan pembanding waktu argon2, lalu memastikan
// tidak ada schema sekali pakai yang tertinggal.
func TestMain(m *testing.M) {
	slog.SetDefault(slog.New(slog.DiscardHandler))

	// security.BurnVerifyTime menyusun hash pembandingnya pada pemakaian PERTAMA, jadi
	// panggilan pertama jauh lebih lambat daripada panggilan berikutnya (satu hashing
	// penuh, bukan satu verifikasi). Dipanaskan di sini karena dua test bergantung
	// padanya: pengukuran waktu jawaban, dan test -race — pengisian malas itu menulis ke
	// variabel paket tanpa penyelarasan, jadi ia harus selesai sebelum ada dua goroutine
	// yang memanggilnya.
	security.BurnVerifyTime()

	code := m.Run()
	if code == 0 {
		if err := checkNoLeftoverSchemas(); err != nil {
			fmt.Fprintf(os.Stderr, "FAIL: %v\n", err)
			code = 1
		}
	}
	os.Exit(code)
}

// checkNoLeftoverSchemas mencari schema sekali pakai yang belum terhapus.
//
// Diperiksa di TestMain, bukan di sebuah test biasa: hanya di titik ini seluruh t.Cleanup
// sudah dijalankan.
func checkNoLeftoverSchemas() error {
	if testing.Short() {
		return nil
	}
	dsn := envDSN()
	if dsn == "" {
		return nil
	}

	ctx, cancel := context.WithTimeout(context.Background(), 15*time.Second)
	defer cancel()

	pool, err := pgxpool.New(ctx, dsn)
	if err != nil {
		// Database tidak bisa dihubungi berarti test integrasinya juga terlewat.
		return nil
	}
	defer pool.Close()

	rows, err := pool.Query(ctx,
		`select nspname from pg_namespace where nspname like $1 order by nspname`,
		testSchemaPrefix+"%")
	if err != nil {
		return nil
	}
	defer rows.Close()

	var leftover []string
	for rows.Next() {
		var name string
		if err := rows.Scan(&name); err != nil {
			return nil
		}
		leftover = append(leftover, name)
	}
	if len(leftover) > 0 {
		return fmt.Errorf("schema sekali pakai tertinggal di database: %s", strings.Join(leftover, ", "))
	}
	return nil
}

func envDSN() string {
	for _, key := range []string{"TEST_DATABASE_URL", "DATABASE_URL"} {
		if dsn := strings.TrimSpace(os.Getenv(key)); dsn != "" {
			return dsn
		}
	}
	return ""
}

// --- Buffer log --------------------------------------------------------------

// syncBuffer adalah buffer yang aman ditulis dari beberapa goroutine.
//
// Handler slog bisa dipanggil dari mana saja, dan test yang menjalankan server httptest
// tidak bisa memastikan semuanya terjadi di goroutine test. Tanpa mutex, -race akan
// menemukannya.
type syncBuffer struct {
	mu  sync.Mutex
	buf bytes.Buffer
}

func (b *syncBuffer) Write(p []byte) (int, error) {
	b.mu.Lock()
	defer b.mu.Unlock()
	return b.buf.Write(p)
}

func (b *syncBuffer) String() string {
	b.mu.Lock()
	defer b.mu.Unlock()
	return b.buf.String()
}

// --- Lingkungan test ---------------------------------------------------------

// env adalah satu lingkungan test lengkap: schema database sendiri, katalog izin dan peran
// bawaan yang sudah ditanam, dan layanan sesi yang menulis log ke buffer.
type env struct {
	ctx  context.Context
	pool *pgxpool.Pool
	svc  *Service
	cfg  *config.Config
	logs *syncBuffer

	users    *identity.Users
	sessions *identity.Sessions
}

// newEnv menyiapkan lingkungan test di atas schema sekali pakai.
func newEnv(t *testing.T, opts ...Option) *env {
	t.Helper()
	ctx, pool := newTestDB(t)

	logs := &syncBuffer{}
	logger := observability.NewLogger(logs, observability.LoggerOptions{Level: slog.LevelDebug})

	cfg := &config.Config{
		AppEnv:        config.EnvDevelopment,
		PublicURL:     "",
		SessionSecret: security.Secret(testSessionSecret),
	}

	// Katalog model ditanam lewat seed yang sama dengan produksi.
	// Admin pertama tidak ikut dibuat karena cfg tidak memuat
	// INITIAL_ADMIN_*; test membuat penggunanya sendiri.
	if _, err := seed.Run(ctx, pool, cfg, slog.New(slog.DiscardHandler)); err != nil {
		t.Fatalf("seed: %v", err)
	}

	svc := NewService(pool, cfg, logger, opts...)
	return &env{
		ctx: ctx, pool: pool, svc: svc, cfg: cfg, logs: logs,
		users:    identity.NewUsers(pool),
		sessions: identity.NewSessions(pool),
	}
}

// newTestDB menyiapkan schema kosong bernama unik, menjalankan migrasi lengkap di
// dalamnya, dan menghapus schema itu saat test selesai.
//
// search_path dikirim sebagai query parameter DSN karena pgx meneruskan parameter tak
// dikenal sebagai runtime parameter saat koneksi dibuka, sehingga setiap koneksi yang
// dibuat pool ikut terarahkan ke schema itu. Dengan begitu tidak ada satu pun tabel test
// yang menyentuh schema public database development.
func newTestDB(t *testing.T) (context.Context, *pgxpool.Pool) {
	t.Helper()
	if testing.Short() {
		t.Skip("butuh PostgreSQL")
	}
	baseDSN := envDSN()
	if baseDSN == "" {
		t.Skip("TEST_DATABASE_URL maupun DATABASE_URL tidak diset — test integrasi dilewati")
	}
	if !strings.HasPrefix(baseDSN, "postgres://") && !strings.HasPrefix(baseDSN, "postgresql://") {
		t.Skip("DSN test bukan URL postgres:// sehingga search_path test tidak bisa dipasang")
	}

	ctx, cancel := context.WithTimeout(context.Background(), 180*time.Second)
	t.Cleanup(cancel)

	admin, err := pgxpool.New(ctx, baseDSN)
	if err != nil {
		t.Fatalf("membuka koneksi admin: %v", err)
	}
	t.Cleanup(admin.Close)

	schema := testSchemaPrefix + randomHex(t, 6)
	ident := pgx.Identifier{schema}.Sanitize()
	if _, err := admin.Exec(ctx, "create schema "+ident); err != nil {
		t.Fatalf("membuat schema %s: %v", schema, err)
	}
	t.Cleanup(func() {
		// Context baru: ctx test bisa sudah kedaluwarsa saat pembersihan jalan, dan schema
		// yang tertinggal akan mengotori database development.
		cleanupCtx, cancelCleanup := context.WithTimeout(context.Background(), 30*time.Second)
		defer cancelCleanup()
		if _, err := admin.Exec(cleanupCtx, "drop schema "+ident+" cascade"); err != nil {
			t.Errorf("membersihkan schema %s: %v", schema, err)
		}
	})

	dbCfg := &config.Config{
		AppEnv:      config.EnvDevelopment,
		DatabaseURL: security.Secret(withSearchPath(t, baseDSN, schema)),
		DBMaxConns:  8,
		DBMinConns:  0,
	}
	db, err := database.Connect(ctx, dbCfg, slog.New(slog.DiscardHandler))
	if err != nil {
		t.Fatalf("Connect: %v", err)
	}
	t.Cleanup(db.Close)

	if _, err := database.Migrate(ctx, db, slog.New(slog.DiscardHandler)); err != nil {
		t.Fatalf("Migrate: %v", err)
	}
	return ctx, db.Pool
}

func withSearchPath(t *testing.T, dsn, schema string) string {
	t.Helper()
	u, err := url.Parse(dsn)
	if err != nil {
		t.Fatalf("DSN test tidak bisa diparse: %v", err)
	}
	q := u.Query()
	q.Set("search_path", schema)
	u.RawQuery = q.Encode()
	return u.String()
}

func randomHex(t *testing.T, n int) string {
	t.Helper()
	buf := make([]byte, n)
	if _, err := rand.Read(buf); err != nil {
		t.Fatalf("membuat nilai acak: %v", err)
	}
	return hex.EncodeToString(buf)
}

// --- Fixture ----------------------------------------------------------------

// makeUser membuat pengguna aktif dengan testPassword.
func (e *env) makeUser(t *testing.T, email, _ string) identity.User {
	t.Helper()
	u, err := e.users.Create(e.ctx, identity.NewUser{
		Email:       email,
		Password:    security.Secret(testPassword),
		DisplayName: "Uji " + email,
		Status:      identity.UserStatusActive,
	})
	if err != nil {
		t.Fatalf("membuat pengguna %q: %v", email, err)
	}
	return u
}

// login menjalankan Login dan menghentikan test bila gagal.
func (e *env) login(t *testing.T, email string) *Result {
	t.Helper()
	res, err := e.svc.Login(e.ctx, email, security.Secret(testPassword), "203.0.113.9", "uji/1.0")
	if err != nil {
		t.Fatalf("Login(%q): %v", email, err)
	}
	return res
}

// exec menjalankan satu pernyataan SQL langsung, untuk menyiapkan keadaan yang tidak
// disediakan API publik (mis. akun terkunci) dan untuk memeriksa isi tabel.
func (e *env) exec(t *testing.T, sql string, args ...any) {
	t.Helper()
	if _, err := e.pool.Exec(e.ctx, sql, args...); err != nil {
		t.Fatalf("exec: %v", err)
	}
}

// auditActions mengembalikan seluruh aksi audit yang tercatat, terlama lebih dulu.
func (e *env) auditActions(t *testing.T) []string {
	t.Helper()
	rows, err := e.pool.Query(e.ctx, `select action from audit_logs order by id`)
	if err != nil {
		t.Fatalf("membaca audit: %v", err)
	}
	defer rows.Close()

	var out []string
	for rows.Next() {
		var action string
		if err := rows.Scan(&action); err != nil {
			t.Fatalf("scan audit: %v", err)
		}
		out = append(out, action)
	}
	if err := rows.Err(); err != nil {
		t.Fatalf("membaca audit: %v", err)
	}
	return out
}

// countAudit menghitung catatan audit untuk satu aksi.
func (e *env) countAudit(t *testing.T, action string) int {
	t.Helper()
	var n int
	err := e.pool.QueryRow(e.ctx, `select count(*) from audit_logs where action = $1`, action).Scan(&n)
	if err != nil {
		t.Fatalf("menghitung audit %q: %v", action, err)
	}
	return n
}

// passwordHash membaca hash password tersimpan seorang pengguna.
func (e *env) passwordHash(t *testing.T, userID string) string {
	t.Helper()
	var hash string
	if err := e.pool.QueryRow(e.ctx, `select password_hash from users where id = $1`, userID).Scan(&hash); err != nil {
		t.Fatalf("membaca hash password: %v", err)
	}
	return hash
}
