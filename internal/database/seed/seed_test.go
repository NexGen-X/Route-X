package seed

import (
	"context"
	"crypto/rand"
	"encoding/hex"
	"errors"
	"io"
	"log/slog"
	"os"
	"strings"
	"testing"

	"github.com/jackc/pgx/v5/pgxpool"

	"github.com/NexGen-X/Route-X/internal/config"
	"github.com/NexGen-X/Route-X/internal/database"
	"github.com/NexGen-X/Route-X/internal/database/repo"
	"github.com/NexGen-X/Route-X/internal/database/repo/identity"
	"github.com/NexGen-X/Route-X/internal/security"
)

const testAdminPassword = "sandi-admin-uji-yang-kuat-123"

func testEnv(t *testing.T) (context.Context, *pgxpool.Pool, *slog.Logger) {
	t.Helper()
	if testing.Short() {
		t.Skip("butuh PostgreSQL")
	}

	dsn := os.Getenv("TEST_DATABASE_URL")
	if dsn == "" {
		dsn = os.Getenv("DATABASE_URL")
	}
	if dsn == "" {
		t.Skip("TEST_DATABASE_URL / DATABASE_URL tidak diset")
	}

	ctx := context.Background()
	buf := make([]byte, 6)
	if _, err := rand.Read(buf); err != nil {
		t.Fatalf("acak: %v", err)
	}
	schema := "test_seed_" + hex.EncodeToString(buf)

	admin, err := pgxpool.New(ctx, dsn)
	if err != nil {
		t.Skipf("tidak bisa terhubung ke Postgres: %v", err)
	}
	if _, err := admin.Exec(ctx, "create schema "+schema); err != nil {
		admin.Close()
		t.Fatalf("membuat schema: %v", err)
	}
	t.Cleanup(func() {
		if _, err := admin.Exec(context.Background(), "drop schema "+schema+" cascade"); err != nil {
			t.Errorf("membersihkan schema %s: %v", schema, err)
		}
		admin.Close()
	})

	sep := "?"
	if strings.Contains(dsn, "?") {
		sep = "&"
	}
	logger := slog.New(slog.NewJSONHandler(io.Discard, nil))
	db, err := database.Connect(ctx, &config.Config{
		DatabaseURL: security.Secret(dsn + sep + "search_path=" + schema),
		DBMaxConns:  4, DBMinConns: 1,
	}, logger)
	if err != nil {
		t.Fatalf("menghubungkan database uji: %v", err)
	}
	t.Cleanup(db.Close)

	if _, err := database.Migrate(ctx, db, logger); err != nil {
		t.Fatalf("menjalankan migrasi: %v", err)
	}
	return ctx, db.Pool, logger
}

func adminConfig() *config.Config {
	return &config.Config{
		InitialAdminEmail:    "admin@uji.test",
		InitialAdminPassword: security.Secret(testAdminPassword),
	}
}

func TestRunCreatesAdminAndCatalog(t *testing.T) {
	ctx, pool, logger := testEnv(t)

	res, err := Run(ctx, pool, adminConfig(), logger)
	if err != nil {
		t.Fatalf("Run: %v", err)
	}
	if !res.AdminCreated {
		t.Error("AdminCreated = false pada database kosong")
	}
	if res.Catalog.ModelsCreated == 0 {
		t.Error("Catalog.ModelsCreated = 0 pada database kosong")
	}

	// Admin pertama: aktif, dipaksa mengganti password.
	users := identity.NewUsers(pool)
	admin, err := users.GetByEmail(ctx, "admin@uji.test")
	if err != nil {
		t.Fatalf("admin pertama tidak ada: %v", err)
	}
	if !admin.MustChangePassword {
		t.Error("MustChangePassword = false; password dari environment harus dipaksa diganti")
	}
	if admin.Status != identity.UserStatusActive {
		t.Errorf("status admin = %q", admin.Status)
	}

	// Password yang tertanam harus benar-benar bisa dipakai login.
	ok, err := security.VerifyPassword(testAdminPassword, admin.PasswordHash.Reveal())
	if err != nil {
		t.Fatalf("VerifyPassword: %v", err)
	}
	if !ok.Match {
		t.Error("password admin yang di-seed tidak cocok")
	}
}

// Dijalankan setiap start, jadi harus idempoten.
func TestRunIsIdempotent(t *testing.T) {
	ctx, pool, logger := testEnv(t)
	cfg := adminConfig()

	first, err := Run(ctx, pool, cfg, logger)
	if err != nil {
		t.Fatalf("Run pertama: %v", err)
	}
	second, err := Run(ctx, pool, cfg, logger)
	if err != nil {
		t.Fatalf("Run kedua: %v", err)
	}

	if second.AdminCreated {
		t.Error("Run kedua membuat admin lagi")
	}
	_ = first

	// Tidak ada duplikat pengguna.
	var userCount int
	mustScan(t, pool, `select count(*) from users`, &userCount)
	if userCount != 1 {
		t.Errorf("jumlah pengguna = %d, mau 1", userCount)
	}
}

// Tanpa kredensial admin di environment, seed membuat admin dengan kredensial bawaan.
func TestRunWithoutAdminCredentials(t *testing.T) {
	ctx, pool, logger := testEnv(t)

	res, err := Run(ctx, pool, &config.Config{}, logger)
	if err != nil {
		t.Fatalf("Run: %v", err)
	}
	if !res.AdminCreated {
		t.Error("AdminCreated = false dengan kredensial default")
	}

	users := identity.NewUsers(pool)
	admin, err := users.GetByEmail(ctx, "admin@routex.local")
	if err != nil {
		t.Fatalf("admin default tidak ada: %v", err)
	}
	if !admin.MustChangePassword {
		t.Error("MustChangePassword = false; password default harus dipaksa diganti")
	}

	ok, err := security.VerifyPassword("RouteX#Initial2026!", admin.PasswordHash.Reveal())
	if err != nil {
		t.Fatalf("VerifyPassword: %v", err)
	}
	if !ok.Match {
		t.Error("password admin default tidak cocok")
	}
}

// Admin pertama hanya dibuat pada database yang benar-benar belum punya pengguna.
func TestRunSkipsAdminWhenAnyUserExists(t *testing.T) {
	ctx, pool, logger := testEnv(t)

	users := identity.NewUsers(pool)
	if _, err := users.Create(ctx, identity.NewUser{
		Email: "orang.lain@uji.test", Password: security.Secret("sandi-lain-yang-kuat-123"),
		Status: identity.UserStatusActive,
	}); err != nil {
		t.Fatalf("membuat pengguna lain: %v", err)
	}

	res, err := Run(ctx, pool, adminConfig(), logger)
	if err != nil {
		t.Fatalf("Run: %v", err)
	}
	if res.AdminCreated {
		t.Error("admin dibuat padahal sudah ada pengguna lain")
	}
	if _, err := users.GetByEmail(ctx, "admin@uji.test"); !errors.Is(err, repo.ErrNotFound) {
		t.Errorf("admin dari environment seharusnya tidak dibuat: err = %v", err)
	}
}

// Password lemah harus ditolak, bukan diterima diam-diam.
func TestRunRejectsWeakAdminPassword(t *testing.T) {
	ctx, pool, logger := testEnv(t)

	_, err := Run(ctx, pool, &config.Config{
		InitialAdminEmail:    "admin@uji.test",
		InitialAdminPassword: security.Secret("changeme"),
	}, logger)
	if err == nil {
		t.Fatal("password lemah seharusnya menggagalkan seed")
	}

	var userCount int
	mustScan(t, pool, `select count(*) from users`, &userCount)
	if userCount != 0 {
		t.Errorf("jumlah user setelah kegagalan = %d, mau 0 (transaksi tidak di-rollback)", userCount)
	}
}

func mustScan(t *testing.T, pool *pgxpool.Pool, sql string, dest ...any) {
	t.Helper()
	if err := pool.QueryRow(context.Background(), sql).Scan(dest...); err != nil {
		t.Fatalf("query %q: %v", sql, err)
	}
}
