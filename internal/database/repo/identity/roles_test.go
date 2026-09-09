package identity

import (
	"errors"
	"slices"
	"testing"

	"github.com/NexGen-X/Route-X/internal/database/repo"
)

// Kunci izin yang dipakai test RBAC. Bentuknya mengikuti constraint
// permissions_key_format: "sumber_daya:aksi".
const (
	permProvidersRead  = "providers:read"
	permProvidersWrite = "providers:write"
	permSettingsWrite  = "settings:write"
	permUsersRead      = "users:read"
)

// Jalur bahagia pengelolaan peran, katalog izin, dan kaitan keduanya.
func TestRolesIntegration(t *testing.T) {
	ctx, db := newTestDB(t)
	roles := NewRoles(db.Pool)

	admin := makeRole(ctx, t, roles, "Admin", 20, permProvidersRead, permProvidersWrite, permSettingsWrite)
	viewer := makeRole(ctx, t, roles, "Viewer", 40, permProvidersRead)

	t.Run("List mengurutkan yang paling berkuasa lebih dulu", func(t *testing.T) {
		got, err := roles.List(ctx)
		if err != nil {
			t.Fatalf("List: %v", err)
		}
		if len(got) != 2 {
			t.Fatalf("jumlah peran = %d, ingin 2", len(got))
		}
		if got[0].Name != "Admin" || got[1].Name != "Viewer" {
			t.Errorf("urutan peran = %s, %s; ingin Admin lebih dulu karena rank-nya lebih kecil",
				got[0].Name, got[1].Name)
		}
		if got[0].Rank != 20 || got[0].Description != "peran Admin" || got[0].IsSystem {
			t.Errorf("peran Admin tidak seperti yang dibuat: %+v", got[0])
		}
	})

	t.Run("Get membawa izinnya dalam satu query", func(t *testing.T) {
		counter := newCountingQuerier(db.Pool)
		got, err := NewRoles(counter).Get(ctx, admin.ID)
		if err != nil {
			t.Fatalf("Get: %v", err)
		}
		if counter.count() != 1 {
			t.Errorf("jumlah query = %d, ingin 1", counter.count())
		}
		if got.Name != "Admin" {
			t.Errorf("Name = %q, ingin Admin", got.Name)
		}

		keys := permissionKeys(got.Permissions)
		want := []string{permProvidersRead, permProvidersWrite, permSettingsWrite}
		if !slices.Equal(keys, want) {
			t.Errorf("izin = %v, ingin %v", keys, want)
		}
		for _, p := range got.Permissions {
			if !validUUID(p.ID) || p.CreatedAt.IsZero() {
				t.Errorf("izin %q tidak lengkap: %+v", p.Key, p)
			}
		}
	})

	t.Run("GetByName", func(t *testing.T) {
		got, err := roles.GetByName(ctx, "Viewer")
		if err != nil {
			t.Fatalf("GetByName: %v", err)
		}
		if got.ID != viewer.ID {
			t.Errorf("GetByName mengembalikan peran lain")
		}
		if _, err := roles.GetByName(ctx, "viewer"); !errors.Is(err, repo.ErrNotFound) {
			t.Errorf("GetByName(%q) = %v, ingin ErrNotFound karena nama peka huruf besar-kecil", "viewer", err)
		}
	})

	t.Run("peran tanpa izin tetap terbaca", func(t *testing.T) {
		bare := makeRole(ctx, t, roles, "Tanpa Izin", 90)
		got, err := roles.Get(ctx, bare.ID)
		if err != nil {
			t.Fatalf("Get: %v", err)
		}
		if len(got.Permissions) != 0 {
			t.Errorf("jumlah izin = %d, ingin 0", len(got.Permissions))
		}
		if err := roles.Delete(ctx, bare.ID); err != nil {
			t.Fatalf("Delete: %v", err)
		}
	})

	t.Run("nama peran harus unik", func(t *testing.T) {
		if _, err := roles.Create(ctx, NewRole{Name: "Admin", Rank: 25}); !errors.Is(err, repo.ErrConflict) {
			t.Errorf("Create dengan nama yang sudah dipakai = %v, ingin ErrConflict", err)
		}
	})

	t.Run("nama kosong ditolak constraint tabel", func(t *testing.T) {
		if _, err := roles.Create(ctx, NewRole{Name: "   ", Rank: 50}); !errors.Is(err, repo.ErrConstraint) {
			t.Errorf("Create dengan nama kosong = %v, ingin ErrConstraint", err)
		}
	})

	t.Run("EnsurePermissions idempoten dan memperbarui keterangan", func(t *testing.T) {
		before, err := roles.ListPermissions(ctx)
		if err != nil {
			t.Fatalf("ListPermissions: %v", err)
		}

		if err := roles.EnsurePermissions(ctx, []NewPermission{
			{Key: permProvidersRead, Description: "keterangan baru"},
			{Key: permUsersRead, Description: "baca pengguna"},
		}); err != nil {
			t.Fatalf("EnsurePermissions: %v", err)
		}

		after, err := roles.ListPermissions(ctx)
		if err != nil {
			t.Fatalf("ListPermissions: %v", err)
		}
		if len(after) != len(before)+1 {
			t.Errorf("jumlah izin = %d, ingin %d (hanya users:read yang baru)", len(after), len(before)+1)
		}
		if !slices.IsSorted(permissionKeys(after)) {
			t.Error("ListPermissions tidak terurut kunci")
		}
		for _, p := range after {
			if p.Key == permProvidersRead && p.Description != "keterangan baru" {
				t.Errorf("keterangan %q = %q, ingin diperbarui", p.Key, p.Description)
			}
		}

		// Aman dipanggil ulang: inilah yang terjadi di setiap start aplikasi.
		if err := roles.EnsurePermissions(ctx, []NewPermission{{Key: permUsersRead}}); err != nil {
			t.Fatalf("EnsurePermissions kedua: %v", err)
		}
		if err := roles.EnsurePermissions(ctx, nil); err != nil {
			t.Fatalf("EnsurePermissions tanpa entri: %v", err)
		}
	})

	t.Run("kunci izin salah bentuk ditolak constraint tabel", func(t *testing.T) {
		for _, key := range []string{"providers", "Providers:Read", "providers:read:extra", ""} {
			err := roles.EnsurePermissions(ctx, []NewPermission{{Key: key}})
			if !errors.Is(err, repo.ErrConstraint) {
				t.Errorf("EnsurePermissions(%q) = %v, ingin ErrConstraint", key, err)
			}
		}
	})

	t.Run("SetPermissions mengganti seluruh daftar", func(t *testing.T) {
		if err := roles.SetPermissions(ctx, viewer.ID, []string{permProvidersRead, permUsersRead, permUsersRead}); err != nil {
			t.Fatalf("SetPermissions: %v", err)
		}
		got, err := roles.Get(ctx, viewer.ID)
		if err != nil {
			t.Fatalf("Get: %v", err)
		}
		if want := []string{permProvidersRead, permUsersRead}; !slices.Equal(permissionKeys(got.Permissions), want) {
			t.Errorf("izin = %v, ingin %v", permissionKeys(got.Permissions), want)
		}

		if err := roles.SetPermissions(ctx, viewer.ID, nil); err != nil {
			t.Fatalf("SetPermissions kosong: %v", err)
		}
		got, err = roles.Get(ctx, viewer.ID)
		if err != nil {
			t.Fatalf("Get: %v", err)
		}
		if len(got.Permissions) != 0 {
			t.Errorf("jumlah izin = %d, ingin 0 setelah dikosongkan", len(got.Permissions))
		}

		// Kunci yang tidak ada di katalog tidak boleh diam-diam diabaikan.
		err = roles.SetPermissions(ctx, viewer.ID, []string{permProvidersRead, "tidak:ada"})
		if !errors.Is(err, repo.ErrInvalidReference) {
			t.Errorf("SetPermissions dengan kunci tak dikenal = %v, ingin ErrInvalidReference", err)
		}
		got, err = roles.Get(ctx, viewer.ID)
		if err != nil {
			t.Fatalf("Get: %v", err)
		}
		if len(got.Permissions) != 0 {
			t.Errorf("jumlah izin = %d, ingin tetap 0 — penolakan tidak boleh menyisakan perubahan", len(got.Permissions))
		}
		if err := roles.SetPermissions(ctx, viewer.ID, []string{permProvidersRead}); err != nil {
			t.Fatalf("SetPermissions: %v", err)
		}
	})

	t.Run("peran yang tidak ada", func(t *testing.T) {
		const missing = "8b1f4a2c-0d3e-4f5a-9b8c-7d6e5f4a3b2c"
		if _, err := roles.Get(ctx, missing); !errors.Is(err, repo.ErrNotFound) {
			t.Errorf("Get = %v, ingin ErrNotFound", err)
		}
		if _, err := roles.Get(ctx, "bukan-uuid"); !errors.Is(err, repo.ErrNotFound) {
			t.Errorf("Get dengan ID bukan UUID = %v, ingin ErrNotFound", err)
		}
		if err := roles.Delete(ctx, missing); !errors.Is(err, repo.ErrNotFound) {
			t.Errorf("Delete = %v, ingin ErrNotFound", err)
		}
	})

	t.Run("peran sistem tidak bisa dihapus", func(t *testing.T) {
		system, err := roles.Create(ctx, NewRole{Name: "Super Admin", Rank: 10, IsSystem: true})
		if err != nil {
			t.Fatalf("Create: %v", err)
		}
		if err := roles.Delete(ctx, system.ID); !errors.Is(err, ErrInvalidInput) {
			t.Errorf("Delete peran sistem = %v, ingin ErrInvalidInput", err)
		}
		if _, err := roles.Get(ctx, system.ID); err != nil {
			t.Errorf("peran sistem ikut terhapus: %v", err)
		}
	})
}

// permissionKeys mengambil kunci dari daftar izin.
func permissionKeys(perms []Permission) []string {
	keys := make([]string, len(perms))
	for i, p := range perms {
		keys[i] = p.Key
	}
	return keys
}

// Pemberian dan pencabutan peran, termasuk terjemahan errornya.
func TestRolesGrantRevokeIntegration(t *testing.T) {
	ctx, db := newTestDB(t)
	users := NewUsers(db.Pool)
	roles := NewRoles(db.Pool)

	granter := makeUser(ctx, t, users, "granter@routex.test")
	target := makeUser(ctx, t, users, "target@routex.test")
	admin := makeRole(ctx, t, roles, "Admin", 20, permProvidersWrite)
	viewer := makeRole(ctx, t, roles, "Viewer", 40, permProvidersRead)

	const missingID = "8b1f4a2c-0d3e-4f5a-9b8c-7d6e5f4a3b2c"

	t.Run("Grant", func(t *testing.T) {
		if err := roles.Grant(ctx, target.ID, viewer.ID, granter.ID); err != nil {
			t.Fatalf("Grant: %v", err)
		}
		if err := roles.Grant(ctx, target.ID, admin.ID, ""); err != nil {
			t.Fatalf("Grant tanpa pemberi: %v", err)
		}

		got, err := roles.OfUser(ctx, target.ID)
		if err != nil {
			t.Fatalf("OfUser: %v", err)
		}
		if len(got) != 2 {
			t.Fatalf("jumlah peran = %d, ingin 2", len(got))
		}
		if got[0].Name != "Admin" {
			t.Errorf("peran pertama = %q, ingin Admin karena rank-nya lebih kecil", got[0].Name)
		}
		for _, ur := range got {
			if ur.GrantedAt.IsZero() {
				t.Errorf("granted_at peran %q kosong", ur.Name)
			}
			switch ur.Name {
			case "Viewer":
				if ur.GrantedBy != granter.ID {
					t.Errorf("GrantedBy = %q, ingin %q", ur.GrantedBy, granter.ID)
				}
			case "Admin":
				if ur.GrantedBy != "" {
					t.Errorf("GrantedBy = %q, ingin kosong untuk pemberian oleh sistem", ur.GrantedBy)
				}
			}
		}
	})

	t.Run("peran yang sudah dimiliki dilaporkan sebagai konflik", func(t *testing.T) {
		if err := roles.Grant(ctx, target.ID, viewer.ID, granter.ID); !errors.Is(err, repo.ErrConflict) {
			t.Errorf("Grant kedua = %v, ingin ErrConflict", err)
		}
	})

	t.Run("referensi yang tidak ada dilaporkan", func(t *testing.T) {
		cases := map[string][3]string{
			"peran tidak ada":    {target.ID, missingID, ""},
			"pengguna tidak ada": {missingID, viewer.ID, ""},
			// Pasangan pengguna-peran yang belum ada, supaya yang tertolak benar-benar
			// referensi pemberinya dan bukan keunikan primary key.
			"pemberi tidak ada":   {granter.ID, viewer.ID, missingID},
			"peran bukan UUID":    {target.ID, "bukan-uuid", ""},
			"pengguna bukan UUID": {"bukan-uuid", viewer.ID, ""},
		}
		for name, args := range cases {
			t.Run(name, func(t *testing.T) {
				err := roles.Grant(ctx, args[0], args[1], args[2])
				if !errors.Is(err, repo.ErrInvalidReference) {
					t.Errorf("Grant = %v, ingin ErrInvalidReference", err)
				}
			})
		}
		if err := roles.Grant(ctx, target.ID, viewer.ID, "bukan-uuid"); !errors.Is(err, ErrInvalidInput) {
			t.Errorf("Grant dengan pemberi bukan UUID = %v, ingin ErrInvalidInput", err)
		}
	})

	t.Run("TopRank memberi peringkat terkuat", func(t *testing.T) {
		rank, err := roles.TopRank(ctx, target.ID)
		if err != nil {
			t.Fatalf("TopRank: %v", err)
		}
		if rank != 20 {
			t.Errorf("TopRank = %d, ingin 20", rank)
		}
		if _, err := roles.TopRank(ctx, granter.ID); !errors.Is(err, repo.ErrNotFound) {
			t.Errorf("TopRank pengguna tanpa peran = %v, ingin ErrNotFound", err)
		}
	})

	t.Run("Revoke", func(t *testing.T) {
		// Peran biasa boleh dikosongkan: guard pemegang-terakhir hanya
		// berlaku untuk Super Admin (lihat subtest di bawah), jadi
		// pencabutan pemegang terakhir peran Admin sah di sini.
		if err := roles.Revoke(ctx, target.ID, admin.ID, ""); err != nil {
			t.Fatalf("Revoke: %v", err)
		}
		if err := roles.Revoke(ctx, target.ID, admin.ID, ""); !errors.Is(err, repo.ErrNotFound) {
			t.Errorf("Revoke kedua = %v, ingin ErrNotFound", err)
		}
		got, err := roles.OfUser(ctx, target.ID)
		if err != nil {
			t.Fatalf("OfUser: %v", err)
		}
		if len(got) != 1 || got[0].ID != viewer.ID {
			t.Errorf("peran setelah pencabutan = %v, ingin hanya Viewer", got)
		}
	})

	t.Run("pemegang terakhir Super Admin tidak bisa dicabut", func(t *testing.T) {
		super := makeRole(ctx, t, roles, "Super Admin", 0, permProvidersRead)
		pegang := makeUser(ctx, t, users, "super@routex.test")
		cadangan := makeUser(ctx, t, users, "cadangan@routex.test")
		if err := roles.Grant(ctx, pegang.ID, super.ID, ""); err != nil {
			t.Fatalf("Grant Super Admin: %v", err)
		}
		// Satu-satunya pemegang: ditolak dengan ErrConflict.
		if err := roles.Revoke(ctx, pegang.ID, super.ID, super.ID); !errors.Is(err, repo.ErrConflict) {
			t.Errorf("Revoke pemegang terakhir Super Admin = %v, ingin ErrConflict", err)
		}
		// Setelah ada pemegang kedua, pencabutan pertama sah.
		if err := roles.Grant(ctx, cadangan.ID, super.ID, ""); err != nil {
			t.Fatalf("Grant cadangan: %v", err)
		}
		if err := roles.Revoke(ctx, pegang.ID, super.ID, super.ID); err != nil {
			t.Errorf("Revoke dengan cadangan ada = %v, ingin nil", err)
		}
		// Sekarang cadangan satu-satunya pemegang: ditolak lagi.
		if err := roles.Revoke(ctx, cadangan.ID, super.ID, super.ID); !errors.Is(err, repo.ErrConflict) {
			t.Errorf("Revoke pemegang terakhir kedua = %v, ingin ErrConflict", err)
		}
	})

	t.Run("peran yang masih dipegang tidak bisa dihapus", func(t *testing.T) {
		if err := roles.Delete(ctx, viewer.ID); !errors.Is(err, repo.ErrInvalidReference) {
			t.Errorf("Delete peran yang masih dipakai = %v, ingin ErrInvalidReference", err)
		}
	})

	t.Run("menghapus pengguna ikut mencabut perannya", func(t *testing.T) {
		if err := users.Delete(ctx, target.ID); err != nil {
			t.Fatalf("Delete: %v", err)
		}
		// Sekarang peran itu tidak dipegang siapa pun lagi, jadi bisa dihapus.
		if err := roles.Delete(ctx, viewer.ID); err != nil {
			t.Errorf("Delete peran setelah pemegangnya dihapus: %v", err)
		}
	})

	t.Run("SetPermissions untuk peran yang tidak ada", func(t *testing.T) {
		if err := roles.SetPermissions(ctx, missingID, nil); !errors.Is(err, repo.ErrNotFound) {
			t.Errorf("SetPermissions = %v, ingin ErrNotFound", err)
		}
	})
}

// Izin efektif adalah gabungan izin seluruh peran pengguna: terurut, tanpa duplikat,
// dan dalam satu query.
func TestRolesEffectivePermissionsIntegration(t *testing.T) {
	ctx, db := newTestDB(t)
	users := NewUsers(db.Pool)
	roles := NewRoles(db.Pool)

	user := makeUser(ctx, t, users, "berperan@routex.test")

	// Dua peran yang sengaja berbagi satu izin yang sama: itu keadaan lumrah, dan
	// hasilnya tidak boleh memuat kunci yang sama dua kali.
	admin := makeRole(ctx, t, roles, "Admin", 20, permProvidersRead, permProvidersWrite, permSettingsWrite)
	operator := makeRole(ctx, t, roles, "Operator", 30, permProvidersRead, permUsersRead)

	for _, roleID := range []string{admin.ID, operator.ID} {
		if err := roles.Grant(ctx, user.ID, roleID, ""); err != nil {
			t.Fatalf("Grant: %v", err)
		}
	}

	counter := newCountingQuerier(db.Pool)
	got, err := NewRoles(counter).EffectivePermissions(ctx, user.ID)
	if err != nil {
		t.Fatalf("EffectivePermissions: %v", err)
	}
	if counter.count() != 1 {
		t.Errorf("jumlah query = %d, ingin 1 — izin efektif harus satu join, bukan N+1", counter.count())
	}

	want := []string{permProvidersRead, permProvidersWrite, permSettingsWrite, permUsersRead}
	if !slices.Equal(got, want) {
		t.Errorf("izin efektif = %v, ingin %v", got, want)
	}

	t.Run("pengguna tanpa peran tidak punya izin", func(t *testing.T) {
		lonely := makeUser(ctx, t, users, "sendirian@routex.test")
		got, err := roles.EffectivePermissions(ctx, lonely.ID)
		if err != nil {
			t.Fatalf("EffectivePermissions: %v", err)
		}
		if len(got) != 0 {
			t.Errorf("izin = %v, ingin kosong", got)
		}
	})

	t.Run("HasPermission", func(t *testing.T) {
		for _, key := range want {
			ok, err := roles.HasPermission(ctx, user.ID, key)
			if err != nil {
				t.Fatalf("HasPermission(%q): %v", key, err)
			}
			if !ok {
				t.Errorf("HasPermission(%q) = false, ingin true", key)
			}
		}
		for _, key := range []string{"tidak:ada", "providers:delete", ""} {
			ok, err := roles.HasPermission(ctx, user.ID, key)
			if err != nil {
				t.Fatalf("HasPermission(%q): %v", key, err)
			}
			if ok {
				t.Errorf("HasPermission(%q) = true, ingin false", key)
			}
		}
		// ID yang bukan UUID tidak boleh menghasilkan izin, dan juga tidak boleh
		// menjadi error yang membuat middleware otorisasi gagal terbuka.
		ok, err := roles.HasPermission(ctx, "bukan-uuid", permProvidersRead)
		if err != nil || ok {
			t.Errorf("HasPermission dengan ID bukan UUID = (%v, %v), ingin (false, nil)", ok, err)
		}
	})

	t.Run("mencabut peran mencabut izinnya", func(t *testing.T) {
		if err := roles.Revoke(ctx, user.ID, admin.ID, ""); err != nil {
			t.Fatalf("Revoke: %v", err)
		}
		got, err := roles.EffectivePermissions(ctx, user.ID)
		if err != nil {
			t.Fatalf("EffectivePermissions: %v", err)
		}
		if want := []string{permProvidersRead, permUsersRead}; !slices.Equal(got, want) {
			t.Errorf("izin efektif = %v, ingin %v", got, want)
		}
	})
}
