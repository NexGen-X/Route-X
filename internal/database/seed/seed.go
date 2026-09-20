// Package seed menanam data yang harus ada agar aplikasi bisa dipakai: katalog izin,
// empat peran bawaan, dan akun admin pertama.
//
// Seluruhnya idempoten dan dijalankan setiap kali aplikasi start. Data ini tidak
// ditanam lewat migrasi SQL karena isinya adalah kebijakan yang bisa berkembang:
// menambah satu izin baru di rilis berikutnya seharusnya cukup dengan mengubah daftar
// di sini, bukan menulis migrasi dan berharap tidak ada operator yang sudah mengubah
// pemetaan perannya sendiri.
package seed

import (
	"context"
	"errors"
	"fmt"
	"log/slog"

	"github.com/jackc/pgx/v5/pgxpool"

	"github.com/NexGen-X/Route-X/internal/config"
	"github.com/NexGen-X/Route-X/internal/database/repo"
	"github.com/NexGen-X/Route-X/internal/database/repo/identity"
	"github.com/NexGen-X/Route-X/internal/security"
)

// Nama peran bawaan. Keempatnya ditandai is_system sehingga tidak bisa dihapus dari
// dashboard — kode dan pemetaan izin di bawah bergantung pada keberadaannya.
const (
	RoleSuperAdmin = "Super Admin"
	RoleAdmin      = "Admin"
	RoleOperator   = "Operator"
	RoleViewer     = "Viewer"
)

// Kunci izin. Formatnya "sumber_daya:aksi", dijaga constraint permissions_key_format.
//
// Tidak ada izin baca untuk kredensial provider: plaintext-nya tidak pernah bisa dibaca
// siapa pun lewat API, jadi izin semacam itu akan menjanjikan sesuatu yang memang tidak
// disediakan sistem.
const (
	PermUsersRead         = "users:read"
	PermUsersWrite        = "users:write"
	PermRolesRead         = "roles:read"
	PermRolesWrite        = "roles:write"
	PermProvidersRead     = "providers:read"
	PermProvidersWrite    = "providers:write"
	PermCredentialsWrite  = "credentials:write"
	PermModelsRead        = "models:read"
	PermModelsWrite       = "models:write"
	PermAPIKeysRead       = "apikeys:read"
	PermAPIKeysWrite      = "apikeys:write"
	PermUsageRead         = "usage:read"
	PermRequestsRead      = "requests:read"
	PermRateLimitsWrite   = "ratelimits:write"
	PermBudgetsWrite      = "budgets:write"
	PermBansWrite         = "bans:write"
	PermFiltersWrite      = "filters:write"
	PermRoutingRead       = "routing:read"
	PermRoutingWrite      = "routing:write"
	PermWebhooksWrite     = "webhooks:write"
	PermIntegrationsWrite = "integrations:write"
	PermAuditRead         = "audit:read"
	PermHealthRead        = "health:read"
	PermSettingsRead      = "settings:read"
	PermSettingsWrite     = "settings:write"
)

// Result melaporkan apa yang dilakukan Run.
type Result struct {
	AdminCreated bool
	Catalog      CatalogResult
}

// Run menanam katalog model dan admin pertama.
func Run(ctx context.Context, pool *pgxpool.Pool, cfg *config.Config, logger *slog.Logger) (Result, error) {
	if logger == nil {
		logger = slog.Default()
	}

	var res Result
	err := repo.InTx(ctx, pool, func(q repo.Querier) error {
		created, err := seedInitialAdmin(ctx, q, cfg, logger)
		if err != nil {
			return err
		}
		res.AdminCreated = created

		cat, err := SeedCatalog(ctx, q)
		if err != nil {
			return fmt.Errorf("menanam katalog model: %w", err)
		}
		res.Catalog = cat
		return nil
	})
	if err != nil {
		return Result{}, err
	}

	logger.Info("seed selesai",
		"admin_created", res.AdminCreated,
		"models_created", res.Catalog.ModelsCreated,
		"models_existing", res.Catalog.ModelsExisting,
		"aliases_created", res.Catalog.AliasesCreated)
	return res, nil
}

// seedInitialAdmin membuat akun admin pertama, hanya bila belum ada pengguna sama sekali.
func seedInitialAdmin(ctx context.Context, q repo.Querier, cfg *config.Config, logger *slog.Logger) (bool, error) {
	users := identity.NewUsers(q)

	count, err := users.Count(ctx)
	if err != nil {
		return false, fmt.Errorf("menghitung pengguna: %w", err)
	}
	if count > 0 {
		return false, nil
	}

	adminEmail := cfg.InitialAdminEmail
	if adminEmail == "" {
		adminEmail = "admin@routex.local"
	}
	adminPassword := cfg.InitialAdminPassword
	if adminPassword.IsZero() {
		adminPassword = security.Secret("RouteX#Initial2026!")
	}

	_, err = users.Create(ctx, identity.NewUser{
		Email:              adminEmail,
		Password:           adminPassword,
		DisplayName:        "Administrator",
		Status:             identity.UserStatusActive,
		MustChangePassword: true,
	})
	if err != nil {
		return false, fmt.Errorf("membuat admin pertama: %w", err)
	}

	logger.Warn("admin pertama dibuat",
		"email", adminEmail,
		"tindakan_wajib", "ganti password saat login pertama")
	return true, nil
}

// isNotFound dan isConflict membungkus pemeriksaan sentinel agar pemanggil di paket ini
// tidak perlu mengimpor errors di banyak tempat.
func isNotFound(err error) bool { return errors.Is(err, repo.ErrNotFound) }
func isConflict(err error) bool { return errors.Is(err, repo.ErrConflict) }

// DefaultAdminEmail adalah alamat admin pertama bila INITIAL_ADMIN_PASSWORD tidak diatur.
// Password defaultnya sendiri tidak ditulis ulang di sini: nilainya bersifat publik di
// repositori dan harus diganti segera, itulah inti pemeriksaan WarnDefaultAdminPending.
const DefaultAdminEmail = "admin@routex.local"

// WarnDefaultAdminPending adalah pemeriksaan fail-fast saat startup: bila akun admin
// pertama masih menanggung MustChangePassword=true, instance ini belum aman.
//
// Latar belakangnya adalah kerentanan admin lemah: ketika INITIAL_ADMIN_PASSWORD tidak
// diatur, seed membuat admin dengan kredensial yang nilainya tersedia publik di
// repositori ini. MustChangePassword=true melindungi sebagian (middleware memaksa ganti
// setelah login), tetapi tidak ada yang mencegah instance dibiarkan dalam keadaan ini
// berminggu-minggu sambil melayani trafik nyata, dan satu kebocoran password default
// berarti kehilangan kendali penuh atas gateway.
//
// Boot TIDAK diblokir: first-run headless tanpa INITIAL_ADMIN_PASSWORD adalah jalur yang
// sah dan satu-satunya cara masuk untuk memperbaikinya. Yang dilakukan hanyalah
// membuat suara yang tidak bisa dilewatkan di log startup.
//
// justCreated=true (seed baru saja membuat admin pertama) menandakan first-run yang
// wajar; false berarti instance sudah pernah boot sebelumnya, sehingga kredensial
// default yang masih aktif adalah indikasi masalah nyata, bukan onboarding.
func WarnDefaultAdminPending(ctx context.Context, pool *pgxpool.Pool, cfg *config.Config, logger *slog.Logger, justCreated bool) {
	if logger == nil {
		logger = slog.Default()
	}

	email := DefaultAdminEmail
	if cfg != nil && cfg.InitialAdminEmail != "" {
		email = cfg.InitialAdminEmail
	}

	u, err := identity.NewUsers(pool).GetByEmail(ctx, email)
	if err != nil {
		// Admin pertama tidak ditemukan, atau terjadi kesalahan baca. Bukan pemeriksaan
		// keamanan yang bisa menegakkan apa pun di titik ini: jangan hentikan boot
		// hanya karena pemeriksaan ini sendiri tidak bisa berjalan.
		if !isNotFound(err) {
			logger.Warn("gagal memeriksa status password admin pertama",
				"email", email, "error", err)
		}
		return
	}
	if !u.MustChangePassword {
		return // aman: password sudah diganti
	}

	// passwordDariEnv=false berarti admin memakai password default yang publik;
	// true berarti operator mengaturnya sendiri lewat INITIAL_ADMIN_PASSWORD, jadi
	// nilainya tidak terlihat di repositori meski tetap wajib diganti.
	passwordDariEnv := cfg != nil && !cfg.InitialAdminPassword.IsZero()

	pesan := fmt.Sprintf(
		"ADMIN PERTAMA %q MASIH MEMAKAI PASSWORD AWAL: instance ini belum aman sampai kata sandinya diganti. "+
			"Dalam arsitektur single-admin, siapa pun yang memegang kredensial ini memiliki kendali penuh. "+
			"Ganti sekarang lewat dashboard (wajib ganti setelah login) atau perintah: routex-rotate set-admin-password",
		email)

	// Bukan first-run tetapi kredensial default publik masih aktif: instance telah
	// terpapar sejak boot pertama. Ini yang paling mendesak, dicatat di level Error
	// supaya monitor dan log shipper tidak melewatkannya.
	if !justCreated && !passwordDariEnv {
		logger.Error(pesan,
			"email", email,
			"sumber_password", "default_publik",
			"first_run", justCreated,
			"tindakan_wajib", "ganti password admin pertama sekarang juga")
		return
	}

	logger.Warn(pesan,
		"email", email,
		"sumber_password", map[bool]string{true: "environment", false: "default_publik"}[passwordDariEnv],
		"first_run", justCreated,
		"tindakan_wajib", "ganti password admin pertama sebelum menerima trafik")
}
