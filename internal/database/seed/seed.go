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

// catalog adalah seluruh izin yang dikenali sistem.
var catalog = []identity.NewPermission{
	{Key: PermUsersRead, Description: "Melihat daftar dan detail pengguna admin"},
	{Key: PermUsersWrite, Description: "Membuat, mengubah, dan menghapus pengguna admin"},
	{Key: PermRolesRead, Description: "Melihat peran dan izinnya"},
	{Key: PermRolesWrite, Description: "Mengubah peran dan pemberian peran"},
	{Key: PermProvidersRead, Description: "Melihat provider dan status kesehatannya"},
	{Key: PermProvidersWrite, Description: "Membuat, mengubah, dan menghapus provider"},
	{Key: PermCredentialsWrite, Description: "Menyimpan dan memutar kredensial provider"},
	{Key: PermModelsRead, Description: "Melihat registry model, alias, dan harga"},
	{Key: PermModelsWrite, Description: "Mengubah registry model, alias, dan harga"},
	{Key: PermAPIKeysRead, Description: "Melihat API key dalam bentuk tersamar"},
	{Key: PermAPIKeysWrite, Description: "Membuat, memutar, dan mencabut API key"},
	{Key: PermUsageRead, Description: "Melihat pemakaian dan biaya"},
	{Key: PermRequestsRead, Description: "Melihat log request dan detailnya"},
	{Key: PermRateLimitsWrite, Description: "Mengubah batas laju"},
	{Key: PermBudgetsWrite, Description: "Mengubah anggaran"},
	{Key: PermBansWrite, Description: "Memblokir dan membuka blokir"},
	{Key: PermFiltersWrite, Description: "Mengubah filter konten"},
	{Key: PermRoutingRead, Description: "Melihat aturan routing"},
	{Key: PermRoutingWrite, Description: "Mengubah aturan routing"},
	{Key: PermWebhooksWrite, Description: "Mengubah webhook"},
	{Key: PermIntegrationsWrite, Description: "Mengubah integrasi pihak ketiga"},
	{Key: PermAuditRead, Description: "Melihat audit log"},
	{Key: PermHealthRead, Description: "Melihat health check"},
	{Key: PermSettingsRead, Description: "Melihat setelan sistem"},
	{Key: PermSettingsWrite, Description: "Mengubah setelan sistem"},
}

// readOnly adalah semua izin baca, dipakai peran Viewer.
var readOnly = []string{
	PermUsersRead, PermRolesRead, PermProvidersRead, PermModelsRead, PermAPIKeysRead,
	PermUsageRead, PermRequestsRead, PermRoutingRead, PermAuditRead, PermHealthRead,
	PermSettingsRead,
}

// operatorExtra adalah izin tulis yang dibutuhkan untuk mengoperasikan gateway
// sehari-hari, tanpa kewenangan mengubah siapa yang boleh masuk.
var operatorExtra = []string{
	PermProvidersWrite, PermCredentialsWrite, PermAPIKeysWrite,
	PermRateLimitsWrite, PermBansWrite,
}

// roleDefinition adalah satu peran bawaan beserta izinnya.
type roleDefinition struct {
	name        string
	description string
	rank        int
	permissions []string
}

// definitions menjelaskan keempat peran.
//
// Pemisahan Admin dan Super Admin ada pada kewenangan atas identitas: Admin mengelola
// seluruh gateway tetapi tidak bisa membuat pengguna baru atau mengubah peran. Tanpa
// batas itu, setiap Admin bisa menaikkan dirinya menjadi Super Admin dan pembedaan
// keduanya tidak bermakna.
var definitions = []roleDefinition{
	{
		name:        RoleSuperAdmin,
		description: "Kendali penuh, termasuk pengelolaan pengguna dan peran",
		rank:        0,
		permissions: allPermissionKeys(),
	},
	{
		name:        RoleAdmin,
		description: "Mengelola seluruh gateway, tanpa kewenangan atas pengguna dan peran",
		rank:        10,
		permissions: exclude(allPermissionKeys(), PermUsersWrite, PermRolesWrite),
	},
	{
		name:        RoleOperator,
		description: "Menjalankan operasi harian: provider, kredensial, API key, batas laju, blokir",
		rank:        20,
		permissions: append(append([]string{}, readOnly...), operatorExtra...),
	},
	{
		name:        RoleViewer,
		description: "Hanya melihat, tanpa kewenangan mengubah apa pun",
		rank:        30,
		permissions: readOnly,
	},
}

func allPermissionKeys() []string {
	out := make([]string, 0, len(catalog))
	for _, p := range catalog {
		out = append(out, p.Key)
	}
	return out
}

func exclude(keys []string, drop ...string) []string {
	skip := make(map[string]struct{}, len(drop))
	for _, d := range drop {
		skip[d] = struct{}{}
	}
	out := make([]string, 0, len(keys))
	for _, k := range keys {
		if _, ok := skip[k]; !ok {
			out = append(out, k)
		}
	}
	return out
}

// Result melaporkan apa yang dilakukan Run.
type Result struct {
	PermissionsEnsured int
	RolesCreated       int
	RolesUpdated       int
	AdminCreated       bool
	Catalog            CatalogResult
}

// Run menanam katalog izin, peran bawaan, dan admin pertama.
//
// Seluruhnya dalam satu transaksi: gateway yang punya peran tetapi tanpa izinnya, atau
// admin tanpa peran, adalah keadaan yang lebih membingungkan daripada gagal start
// dengan pesan jelas.
func Run(ctx context.Context, pool *pgxpool.Pool, cfg *config.Config, logger *slog.Logger) (Result, error) {
	if logger == nil {
		logger = slog.Default()
	}

	var res Result
	err := repo.InTx(ctx, pool, func(q repo.Querier) error {
		roles := identity.NewRoles(q)

		if err := roles.EnsurePermissions(ctx, catalog); err != nil {
			return fmt.Errorf("menanam katalog izin: %w", err)
		}
		res.PermissionsEnsured = len(catalog)

		for _, def := range definitions {
			existing, err := roles.GetByName(ctx, def.name)
			switch {
			case err == nil:
				// Peran sudah ada: pemetaan izinnya tetap disegarkan supaya izin yang
				// ditambahkan di rilis baru ikut berlaku tanpa langkah manual.
				if err := roles.SetPermissions(ctx, existing.ID, def.permissions); err != nil {
					return fmt.Errorf("menyegarkan izin peran %q: %w", def.name, err)
				}
				res.RolesUpdated++
			case errors.Is(err, repo.ErrNotFound):
				created, err := roles.Create(ctx, identity.NewRole{
					Name: def.name, Description: def.description, Rank: def.rank, IsSystem: true,
				})
				if err != nil {
					return fmt.Errorf("membuat peran %q: %w", def.name, err)
				}
				if err := roles.SetPermissions(ctx, created.ID, def.permissions); err != nil {
					return fmt.Errorf("menetapkan izin peran %q: %w", def.name, err)
				}
				res.RolesCreated++
			default:
				return fmt.Errorf("memeriksa peran %q: %w", def.name, err)
			}
		}

		created, err := seedInitialAdmin(ctx, q, cfg, logger)
		if err != nil {
			return err
		}
		res.AdminCreated = created

		catalog, err := SeedCatalog(ctx, q)
		if err != nil {
			return fmt.Errorf("menanam katalog model: %w", err)
		}
		res.Catalog = catalog
		return nil
	})
	if err != nil {
		return Result{}, err
	}

	logger.Info("seed selesai",
		"permissions", res.PermissionsEnsured,
		"roles_created", res.RolesCreated,
		"roles_updated", res.RolesUpdated,
		"admin_created", res.AdminCreated,
		"models_created", res.Catalog.ModelsCreated,
		"models_existing", res.Catalog.ModelsExisting,
		"aliases_created", res.Catalog.AliasesCreated)
	return res, nil
}

// seedInitialAdmin membuat akun admin pertama, hanya bila belum ada pengguna sama sekali.
//
// Syarat "belum ada pengguna sama sekali" penting: memeriksa berdasarkan email akan
// menghidupkan kembali akun yang sengaja dihapus operator setiap kali aplikasi
// di-restart.
func seedInitialAdmin(ctx context.Context, q repo.Querier, cfg *config.Config, logger *slog.Logger) (bool, error) {
	users := identity.NewUsers(q)

	count, err := users.Count(ctx)
	if err != nil {
		return false, fmt.Errorf("menghitung pengguna: %w", err)
	}
	if count > 0 {
		return false, nil
	}

	if cfg.InitialAdminEmail == "" || cfg.InitialAdminPassword.IsZero() {
		// Aplikasi tetap jalan: jalur API gateway tidak memerlukan akun admin. Tetapi
		// dashboard belum bisa dimasuki, jadi keadaan ini harus terang di log.
		//
		// Password acak sengaja TIDAK dibuat lalu dicetak: password tidak boleh masuk
		// log dalam keadaan apa pun, dan mencetaknya ke stdout tetap berarti ia tersimpan
		// di jurnal systemd dan di riwayat terminal.
		logger.Error("belum ada pengguna admin dan INITIAL_ADMIN_EMAIL/INITIAL_ADMIN_PASSWORD kosong",
			"akibat", "dashboard belum bisa dimasuki; jalur API gateway tetap berfungsi",
			"perbaikan", "isi INITIAL_ADMIN_EMAIL dan INITIAL_ADMIN_PASSWORD di .env lalu jalankan ulang")
		return false, nil
	}

	admin, err := users.Create(ctx, identity.NewUser{
		Email:       cfg.InitialAdminEmail,
		Password:    cfg.InitialAdminPassword,
		DisplayName: "Administrator",
		Status:      identity.UserStatusActive,
		// Password awal berasal dari environment, jadi ia ada di file dan mungkin di
		// riwayat shell. Penggantian pada login pertama dipaksakan.
		MustChangePassword: true,
	})
	if err != nil {
		return false, fmt.Errorf("membuat admin pertama: %w", err)
	}

	roles := identity.NewRoles(q)
	superAdmin, err := roles.GetByName(ctx, RoleSuperAdmin)
	if err != nil {
		return false, fmt.Errorf("mengambil peran %s: %w", RoleSuperAdmin, err)
	}
	// grantedBy dibiarkan kosong: pemberian ini dilakukan sistem, bukan manusia.
	if err := roles.Grant(ctx, admin.ID, superAdmin.ID, ""); err != nil {
		return false, fmt.Errorf("memberi peran %s ke admin pertama: %w", RoleSuperAdmin, err)
	}

	logger.Warn("admin pertama dibuat dari environment",
		"email", cfg.InitialAdminEmail,
		"peran", RoleSuperAdmin,
		"tindakan_wajib", "ganti password saat login pertama, lalu hapus INITIAL_ADMIN_PASSWORD dari .env")
	return true, nil
}

// isNotFound dan isConflict membungkus pemeriksaan sentinel agar pemanggil di paket ini
// tidak perlu mengimpor errors di banyak tempat.
func isNotFound(err error) bool { return errors.Is(err, repo.ErrNotFound) }
func isConflict(err error) bool { return errors.Is(err, repo.ErrConflict) }
