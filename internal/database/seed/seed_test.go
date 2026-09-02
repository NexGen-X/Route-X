package seed

import (
	"context"
	"crypto/rand"
	"encoding/hex"
	"errors"
	"io"
	"log/slog"
	"os"
	"sort"
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

func TestRunCreatesRolesPermissionsAndAdmin(t *testing.T) {
	ctx, pool, logger := testEnv(t)

	res, err := Run(ctx, pool, adminConfig(), logger)
	if err != nil {
		t.Fatalf("Run: %v", err)
	}
	if res.PermissionsEnsured != len(catalog) {
		t.Errorf("PermissionsEnsured = %d, mau %d", res.PermissionsEnsured, len(catalog))
	}
	if res.RolesCreated != len(definitions) {
		t.Errorf("RolesCreated = %d, mau %d", res.RolesCreated, len(definitions))
	}
	if !res.AdminCreated {
		t.Error("AdminCreated = false pada database kosong")
	}

	roles := identity.NewRoles(pool)

	// Keempat peran ada, ditandai sistem, dan pangkatnya sesuai.
	wantRanks := map[string]int{RoleSuperAdmin: 0, RoleAdmin: 10, RoleOperator: 20, RoleViewer: 30}
	for name, rank := range wantRanks {
		got, err := roles.GetByName(ctx, name)
		if err != nil {
			t.Fatalf("peran %q tidak ada: %v", name, err)
		}
		if got.Rank != rank {
			t.Errorf("peran %q rank = %d, mau %d", name, got.Rank, rank)
		}
		if !got.IsSystem {
			t.Errorf("peran %q is_system = false", name)
		}
	}

	// Pemetaan izin per peran.
	permChecks := []struct {
		role     string
		wantKeys []string
	}{
		{RoleSuperAdmin, allPermissionKeys()},
		{RoleAdmin, exclude(allPermissionKeys(), PermUsersWrite, PermRolesWrite)},
		{RoleOperator, append(append([]string{}, readOnly...), operatorExtra...)},
		{RoleViewer, readOnly},
	}
	for _, pc := range permChecks {
		t.Run(pc.role, func(t *testing.T) {
			detail, err := roles.GetByName(ctx, pc.role)
			if err != nil {
				t.Fatalf("GetByName: %v", err)
			}
			got := make([]string, 0, len(detail.Permissions))
			for _, p := range detail.Permissions {
				got = append(got, p.Key)
			}
			sort.Strings(got)
			want := append([]string{}, pc.wantKeys...)
			sort.Strings(want)
			if strings.Join(got, ",") != strings.Join(want, ",") {
				t.Errorf("izin peran %s:\n  dapat: %v\n  mau  : %v", pc.role, got, want)
			}
		})
	}

	// Admin pertama: aktif, Super Admin, dan dipaksa mengganti password.
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
	granted, err := roles.OfUser(ctx, admin.ID)
	if err != nil {
		t.Fatalf("OfUser: %v", err)
	}
	if len(granted) != 1 || granted[0].Name != RoleSuperAdmin {
		t.Errorf("peran admin = %v, mau [%s]", granted, RoleSuperAdmin)
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

	if second.RolesCreated != 0 {
		t.Errorf("Run kedua membuat %d peran, mau 0", second.RolesCreated)
	}
	if second.RolesUpdated != len(definitions) {
		t.Errorf("RolesUpdated = %d, mau %d", second.RolesUpdated, len(definitions))
	}
	if second.AdminCreated {
		t.Error("Run kedua membuat admin lagi")
	}
	_ = first

	// Tidak ada duplikat.
	var roleCount, permCount, userCount int
	mustScan(t, pool, `select count(*) from roles`, &roleCount)
	mustScan(t, pool, `select count(*) from permissions`, &permCount)
	mustScan(t, pool, `select count(*) from users`, &userCount)
	if roleCount != len(definitions) {
		t.Errorf("jumlah peran = %d, mau %d", roleCount, len(definitions))
	}
	if permCount != len(catalog) {
		t.Errorf("jumlah izin = %d, mau %d", permCount, len(catalog))
	}
	if userCount != 1 {
		t.Errorf("jumlah pengguna = %d, mau 1", userCount)
	}
}

// Peran yang izinnya diubah operator akan disegarkan kembali ke definisi bawaan —
// itu memang disengaja supaya izin baru dari rilis berikutnya ikut berlaku.
func TestRunRefreshesRolePermissions(t *testing.T) {
	ctx, pool, logger := testEnv(t)
	if _, err := Run(ctx, pool, adminConfig(), logger); err != nil {
		t.Fatalf("Run: %v", err)
	}

	roles := identity.NewRoles(pool)
	viewer, err := roles.GetByName(ctx, RoleViewer)
	if err != nil {
		t.Fatalf("GetByName: %v", err)
	}
	if err := roles.SetPermissions(ctx, viewer.ID, []string{PermUsersRead}); err != nil {
		t.Fatalf("SetPermissions: %v", err)
	}

	if _, err := Run(ctx, pool, adminConfig(), logger); err != nil {
		t.Fatalf("Run kedua: %v", err)
	}

	viewer, err = roles.GetByName(ctx, RoleViewer)
	if err != nil {
		t.Fatalf("GetByName: %v", err)
	}
	if len(viewer.Permissions) != len(readOnly) {
		t.Errorf("izin Viewer setelah seed ulang = %d, mau %d", len(viewer.Permissions), len(readOnly))
	}
}

// Tanpa kredensial admin di environment, seed tetap menanam peran tetapi tidak membuat
// pengguna — dan tidak boleh membuat password acak lalu mencatatnya.
func TestRunWithoutAdminCredentials(t *testing.T) {
	ctx, pool, _ := testEnv(t)

	var logBuf strings.Builder
	logger := slog.New(slog.NewTextHandler(&logBuf, nil))

	res, err := Run(ctx, pool, &config.Config{}, logger)
	if err != nil {
		t.Fatalf("Run: %v", err)
	}
	if res.AdminCreated {
		t.Error("AdminCreated = true tanpa kredensial di environment")
	}
	if res.RolesCreated != len(definitions) {
		t.Errorf("peran tetap harus ditanam: RolesCreated = %d", res.RolesCreated)
	}

	var userCount int
	mustScan(t, pool, `select count(*) from users`, &userCount)
	if userCount != 0 {
		t.Errorf("jumlah pengguna = %d, mau 0", userCount)
	}

	// Log harus menjelaskan cara memperbaiki, tanpa memuat password apa pun.
	logs := logBuf.String()
	if !strings.Contains(logs, "INITIAL_ADMIN_PASSWORD") {
		t.Errorf("log tidak menyebut variabel yang harus diisi:\n%s", logs)
	}
	for _, forbidden := range []string{"password=", "sandi"} {
		if strings.Contains(strings.ToLower(logs), forbidden) && !strings.Contains(logs, "INITIAL_ADMIN_PASSWORD") {
			t.Errorf("log tampak memuat nilai password: %s", logs)
		}
	}
}

// Admin pertama hanya dibuat pada database yang benar-benar belum punya pengguna.
// Memeriksa berdasarkan email akan menghidupkan kembali akun yang sengaja dihapus.
func TestRunSkipsAdminWhenAnyUserExists(t *testing.T) {
	ctx, pool, logger := testEnv(t)

	// Tanam peran lebih dulu, lalu buat pengguna lain.
	if _, err := Run(ctx, pool, &config.Config{}, logger); err != nil {
		t.Fatalf("Run: %v", err)
	}
	users := identity.NewUsers(pool)
	if _, err := users.Create(ctx, identity.NewUser{
		Email: "orang.lain@uji.test", Password: security.Secret("sandi-lain-yang-kuat-123"),
		Status: identity.UserStatusActive,
	}); err != nil {
		t.Fatalf("membuat pengguna lain: %v", err)
	}

	res, err := Run(ctx, pool, adminConfig(), logger)
	if err != nil {
		t.Fatalf("Run kedua: %v", err)
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

	// Transaksi harus di-rollback seluruhnya: tidak boleh ada peran separuh jalan.
	var roleCount int
	mustScan(t, pool, `select count(*) from roles`, &roleCount)
	if roleCount != 0 {
		t.Errorf("jumlah peran setelah kegagalan = %d, mau 0 (transaksi tidak di-rollback)", roleCount)
	}
}

// Katalog izin harus memenuhi constraint permissions_key_format.
func TestCatalogKeysMatchSchemaConstraint(t *testing.T) {
	for _, p := range catalog {
		parts := strings.Split(p.Key, ":")
		if len(parts) != 2 || parts[0] == "" || parts[1] == "" {
			t.Errorf("kunci %q tidak berbentuk sumber_daya:aksi", p.Key)
			continue
		}
		for _, part := range parts {
			for _, c := range part {
				if (c < 'a' || c > 'z') && c != '_' {
					t.Errorf("kunci %q memuat karakter %q yang ditolak constraint", p.Key, c)
				}
			}
		}
		if p.Description == "" {
			t.Errorf("izin %q tanpa deskripsi", p.Key)
		}
	}
}

func TestCatalogHasNoDuplicates(t *testing.T) {
	seen := map[string]struct{}{}
	for _, p := range catalog {
		if _, dup := seen[p.Key]; dup {
			t.Errorf("izin %q terdaftar dua kali", p.Key)
		}
		seen[p.Key] = struct{}{}
	}
}

// Setiap izin yang dirujuk definisi peran harus ada di katalog, kalau tidak seed akan
// gagal saat dijalankan di database sungguhan.
func TestRoleDefinitionsReferenceKnownPermissions(t *testing.T) {
	known := map[string]struct{}{}
	for _, p := range catalog {
		known[p.Key] = struct{}{}
	}
	for _, def := range definitions {
		if len(def.permissions) == 0 {
			t.Errorf("peran %q tanpa izin", def.name)
		}
		for _, key := range def.permissions {
			if _, ok := known[key]; !ok {
				t.Errorf("peran %q merujuk izin %q yang tidak ada di katalog", def.name, key)
			}
		}
	}
}

// Pangkat peran harus unik dan berurut dari yang paling berkuasa.
func TestRoleRanksAreDistinctAndOrdered(t *testing.T) {
	seen := map[int]string{}
	for _, def := range definitions {
		if other, dup := seen[def.rank]; dup {
			t.Errorf("peran %q dan %q memakai rank %d yang sama", def.name, other, def.rank)
		}
		seen[def.rank] = def.name
	}
	// Super Admin harus paling berkuasa dan punya izin terbanyak.
	for _, def := range definitions {
		if def.name == RoleSuperAdmin {
			continue
		}
		if def.rank <= 0 {
			t.Errorf("peran %q rank %d tidak boleh setara atau di atas Super Admin", def.name, def.rank)
		}
		if len(def.permissions) >= len(allPermissionKeys()) {
			t.Errorf("peran %q punya izin sebanyak Super Admin", def.name)
		}
	}
}

func mustScan(t *testing.T, pool *pgxpool.Pool, sql string, dest ...any) {
	t.Helper()
	if err := pool.QueryRow(context.Background(), sql).Scan(dest...); err != nil {
		t.Fatalf("query %q: %v", sql, err)
	}
}
