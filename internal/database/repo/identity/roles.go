package identity

import (
	"context"
	"fmt"
	"time"

	"github.com/NexGen-X/Route-X/internal/database/repo"
)

// Role adalah satu baris tabel roles.
type Role struct {
	ID          string `json:"id"`
	Name        string `json:"name"`
	Description string `json:"description,omitempty"`
	// IsSystem menandai peran bawaan yang tidak boleh dihapus dari dashboard karena
	// kode dan seed bergantung padanya.
	IsSystem bool `json:"is_system"`
	// Rank adalah tingkat kewenangan; makin kecil makin berkuasa.
	Rank      int       `json:"rank"`
	CreatedAt time.Time `json:"created_at"`
}

// Permission adalah satu baris tabel permissions.
type Permission struct {
	ID string `json:"id"`
	// Key berbentuk "sumber_daya:aksi", mis. "providers:write".
	Key         string    `json:"key"`
	Description string    `json:"description,omitempty"`
	CreatedAt   time.Time `json:"created_at"`
}

// RoleDetail adalah peran beserta izin yang dimilikinya.
type RoleDetail struct {
	Role
	Permissions []Permission `json:"permissions"`
}

// UserRole adalah peran yang dimiliki seorang pengguna beserta jejak pemberiannya.
type UserRole struct {
	Role
	GrantedAt time.Time `json:"granted_at"`
	// GrantedBy kosong berarti peran diberikan sistem, atau akun pemberinya sudah
	// dihapus — kolomnya ON DELETE SET NULL, bukan cascade, supaya menghapus admin
	// tidak menghapus catatan siapa yang pernah memberi kewenangan.
	GrantedBy string `json:"granted_by,omitempty"`
}

// NewRole adalah masukan pembuatan peran.
type NewRole struct {
	Name        string
	Description string
	// Rank wajib diisi: makin kecil makin berkuasa.
	Rank int
	// IsSystem hanya untuk peran yang ditanam saat bootstrap, bukan untuk peran yang
	// dibuat dari dashboard.
	IsSystem bool
}

// NewPermission adalah satu entri katalog izin.
type NewPermission struct {
	Key         string
	Description string
}

// Roles adalah repository untuk seluruh gugus RBAC: roles, permissions,
// role_permissions, dan user_roles.
//
// Keempatnya satu tipe, bukan empat, karena tidak ada pemanggil yang memakai salah
// satunya sendirian: menampilkan peran berarti menampilkan izinnya, dan memberi peran
// berarti memeriksa peringkat peran itu.
type Roles struct {
	q repo.Querier
}

// NewRoles membuat repository RBAC di atas q.
func NewRoles(q repo.Querier) *Roles { return &Roles{q: q} }

// roleColumns adalah kolom peran, dengan awalan alias tabel.
const roleColumns = `%[1]s.id::text, %[1]s.name, coalesce(%[1]s.description, ''),
	%[1]s.is_system, %[1]s.rank, %[1]s.created_at`

// permissionColumns adalah kolom izin, dengan awalan alias tabel.
const permissionColumns = `%[1]s.id::text, %[1]s.key, coalesce(%[1]s.description, ''), %[1]s.created_at`

// List mengembalikan seluruh peran, yang paling berkuasa lebih dulu.
func (r *Roles) List(ctx context.Context) ([]Role, error) {
	const op = "mendaftar peran"

	query := `select ` + fmt.Sprintf(roleColumns, "r") + ` from roles r order by r.rank, r.name`
	rows, err := r.q.Query(ctx, query)
	if err != nil {
		return nil, repo.Err(op, err)
	}
	defer rows.Close()

	var out []Role
	for rows.Next() {
		var role Role
		if err := rows.Scan(&role.ID, &role.Name, &role.Description,
			&role.IsSystem, &role.Rank, &role.CreatedAt); err != nil {
			return nil, repo.Err(op, err)
		}
		out = append(out, role)
	}
	if err := rows.Err(); err != nil {
		return nil, repo.Err(op, err)
	}
	return out, nil
}

// Get mengambil satu peran beserta izinnya berdasarkan ID.
func (r *Roles) Get(ctx context.Context, id string) (RoleDetail, error) {
	const op = "mengambil peran"
	if !validUUID(id) {
		return RoleDetail{}, fmt.Errorf("%s: %w", op, repo.ErrNotFound)
	}
	return r.detail(ctx, op, "r.id = $1", id)
}

// GetByName mengambil satu peran beserta izinnya berdasarkan nama. Perbandingannya
// peka huruf besar-kecil, sama seperti constraint unik pada kolomnya.
func (r *Roles) GetByName(ctx context.Context, name string) (RoleDetail, error) {
	const op = "mengambil peran berdasarkan nama"
	return r.detail(ctx, op, "r.name = $1", name)
}

// detail membaca peran beserta izinnya dalam satu perjalanan.
//
// LEFT JOIN dipakai daripada dua query supaya peran tanpa izin pun tetap terbaca, dan
// supaya jumlah query tidak tumbuh mengikuti jumlah izin.
func (r *Roles) detail(ctx context.Context, op, where, arg string) (RoleDetail, error) {
	query := `select ` + fmt.Sprintf(roleColumns, "r") + `,
		p.id::text, p.key, coalesce(p.description, ''), p.created_at
	from roles r
	left join role_permissions rp on rp.role_id = r.id
	left join permissions p on p.id = rp.permission_id
	where ` + where + `
	order by p.key`

	rows, err := r.q.Query(ctx, query, arg)
	if err != nil {
		return RoleDetail{}, repo.Err(op, err)
	}
	defer rows.Close()

	var (
		out   RoleDetail
		found bool
	)
	for rows.Next() {
		var (
			role Role
			perm Permission
			// Kolom izin bernilai NULL untuk peran yang belum punya izin sama sekali.
			permID, permKey, permDesc *string
			permCreated               *time.Time
		)
		if err := rows.Scan(&role.ID, &role.Name, &role.Description,
			&role.IsSystem, &role.Rank, &role.CreatedAt,
			&permID, &permKey, &permDesc, &permCreated); err != nil {
			return RoleDetail{}, repo.Err(op, err)
		}
		if !found {
			out.Role = role
			found = true
		}
		if permID == nil || permKey == nil {
			continue
		}
		perm.ID, perm.Key = *permID, *permKey
		if permDesc != nil {
			perm.Description = *permDesc
		}
		if permCreated != nil {
			perm.CreatedAt = *permCreated
		}
		out.Permissions = append(out.Permissions, perm)
	}
	if err := rows.Err(); err != nil {
		return RoleDetail{}, repo.Err(op, err)
	}
	if !found {
		return RoleDetail{}, fmt.Errorf("%s: %w", op, repo.ErrNotFound)
	}
	return out, nil
}

// Create menambahkan peran baru.
func (r *Roles) Create(ctx context.Context, in NewRole) (Role, error) {
	const op = "membuat peran"

	const query = `insert into roles (name, description, is_system, rank)
	values ($1, $2, $3, $4)
	returning id::text, name, coalesce(description, ''), is_system, rank, created_at`

	var role Role
	err := r.q.QueryRow(ctx, query, in.Name, textParam(in.Description), in.IsSystem, in.Rank).
		Scan(&role.ID, &role.Name, &role.Description, &role.IsSystem, &role.Rank, &role.CreatedAt)
	if err != nil {
		return Role{}, repo.Err(op, err)
	}
	return role, nil
}

// Delete menghapus peran yang bukan peran sistem.
//
// Peran yang masih dipegang seseorang tidak bisa dihapus: user_roles.role_id memakai
// ON DELETE RESTRICT, sehingga usaha itu menghasilkan repo.ErrInvalidReference. Cabut
// dulu perannya dari semua pengguna, atau memang jangan dihapus.
func (r *Roles) Delete(ctx context.Context, id string) error {
	const op = "menghapus peran"
	if !validUUID(id) {
		return fmt.Errorf("%s: %w", op, repo.ErrNotFound)
	}

	tag, err := r.q.Exec(ctx, `delete from roles where id = $1 and not is_system`, id)
	if err != nil {
		return repo.Err(op, err)
	}
	if tag.RowsAffected() > 0 {
		return nil
	}

	// Nol baris bisa berarti dua hal yang berbeda bagi pemanggil, jadi dibedakan —
	// hanya di jalur gagal, sehingga jalur normal tetap satu perjalanan.
	var isSystem bool
	if err := r.q.QueryRow(ctx, `select is_system from roles where id = $1`, id).Scan(&isSystem); err != nil {
		return repo.Err(op, err)
	}
	if !isSystem {
		// Barisnya ada dan bukan peran sistem, tapi tetap tidak terhapus: hanya
		// mungkin kalau ada perubahan bersamaan. Pemanggil sebaiknya mencoba lagi.
		return fmt.Errorf("%s: %w", op, repo.ErrNotFound)
	}
	return fmt.Errorf("%s: %w: peran sistem tidak boleh dihapus", op, ErrInvalidInput)
}

// ListPermissions mengembalikan seluruh katalog izin, terurut kunci.
func (r *Roles) ListPermissions(ctx context.Context) ([]Permission, error) {
	const op = "mendaftar izin"

	query := `select ` + fmt.Sprintf(permissionColumns, "p") + ` from permissions p order by p.key`
	rows, err := r.q.Query(ctx, query)
	if err != nil {
		return nil, repo.Err(op, err)
	}
	defer rows.Close()

	var out []Permission
	for rows.Next() {
		var p Permission
		if err := rows.Scan(&p.ID, &p.Key, &p.Description, &p.CreatedAt); err != nil {
			return nil, repo.Err(op, err)
		}
		out = append(out, p)
	}
	if err := rows.Err(); err != nil {
		return nil, repo.Err(op, err)
	}
	return out, nil
}

// EnsurePermissions menyelaraskan katalog izin dengan daftar yang dikenal aplikasi.
//
// Dipanggil saat start: izin yang belum ada ditambahkan, keterangan yang berubah
// diperbarui, dan izin yang tidak lagi disebut dibiarkan — menghapusnya akan
// mencabut kewenangan yang mungkin masih dirujuk peran yang dibuat operator.
// Seluruhnya satu pernyataan, jadi aman dijalankan beberapa instance sekaligus.
//
// Kunci yang tidak berbentuk "sumber_daya:aksi" ditolak constraint
// permissions_key_format sebagai repo.ErrConstraint.
func (r *Roles) EnsurePermissions(ctx context.Context, perms []NewPermission) error {
	const op = "menyelaraskan katalog izin"
	if len(perms) == 0 {
		return nil
	}

	keys := make([]string, len(perms))
	descriptions := make([]string, len(perms))
	for i, p := range perms {
		keys[i] = p.Key
		descriptions[i] = p.Description
	}

	const query = `insert into permissions (key, description)
	select t.key, nullif(t.description, '')
	from unnest($1::text[], $2::text[]) as t(key, description)
	on conflict (key) do update set description = coalesce(excluded.description, permissions.description)`

	if _, err := r.q.Exec(ctx, query, keys, descriptions); err != nil {
		return repo.Err(op, err)
	}
	return nil
}

// SetPermissions mengganti seluruh izin sebuah peran dengan daftar kunci yang diberikan.
// Daftar kosong mencabut semua izin peran itu.
//
// Kunci yang tidak ada di katalog menghasilkan repo.ErrInvalidReference tanpa menyisakan
// perubahan apa pun: katalog diperiksa lebih dulu, sebelum ada baris yang disentuh.
// Pemeriksaan terpisah itu tidak membuka perlombaan, karena katalog izin hanya bertambah
// — paket ini sengaja tidak menyediakan penghapusan izin, justru supaya kewenangan yang
// masih dirujuk peran tidak bisa hilang dari bawah.
//
// Pencabutan dan penambahannya sendiri satu pernyataan lewat CTE, jadi peran tidak
// pernah sempat terlihat tanpa izin di tengah proses walaupun pemanggil tidak memakai
// transaksi.
func (r *Roles) SetPermissions(ctx context.Context, roleID string, keys []string) error {
	const op = "menyetel izin peran"
	if !validUUID(roleID) {
		return fmt.Errorf("%s: %w", op, repo.ErrNotFound)
	}

	// Kunci ganda tidak boleh membuat pemeriksaan jumlah di bawah salah menuduh.
	unique := make([]string, 0, len(keys))
	seen := make(map[string]struct{}, len(keys))
	for _, k := range keys {
		if _, dup := seen[k]; dup {
			continue
		}
		seen[k] = struct{}{}
		unique = append(unique, k)
	}

	const check = `select
		(select count(*) from roles where id = $1),
		(select count(*) from permissions where key = any($2::text[]))`

	var roleCount, matched int
	if err := r.q.QueryRow(ctx, check, roleID, unique).Scan(&roleCount, &matched); err != nil {
		return repo.Err(op, err)
	}
	if roleCount == 0 {
		return fmt.Errorf("%s: %w", op, repo.ErrNotFound)
	}
	if matched != len(unique) {
		return fmt.Errorf("%s: %w: %d dari %d kunci izin tidak ada di katalog",
			op, repo.ErrInvalidReference, len(unique)-matched, len(unique))
	}

	const query = `with wanted as (
		select id from permissions where key = any($2::text[])
	), removed as (
		delete from role_permissions
		where role_id = $1 and permission_id not in (select id from wanted)
	)
	insert into role_permissions (role_id, permission_id)
	select $1, id from wanted
	on conflict do nothing`

	if _, err := r.q.Exec(ctx, query, roleID, unique); err != nil {
		return repo.Err(op, err)
	}
	return nil
}

// Grant memberikan peran kepada pengguna. grantedBy boleh kosong untuk pemberian oleh
// sistem, mis. saat bootstrap.
//
// Peran yang sudah dimiliki menghasilkan repo.ErrConflict, dan pengguna, peran, atau
// pemberi yang tidak ada menghasilkan repo.ErrInvalidReference. Keduanya sengaja
// dilaporkan, bukan diabaikan: pemanggil yang mengira sedang mengubah kewenangan
// seseorang harus tahu kalau tidak ada yang berubah.
//
// Aturan "admin tidak boleh memberi peran di atas dirinya sendiri" tidak diterapkan di
// sini — itu kebijakan yang butuh identitas pelaku, dan tempatnya di lapisan auth.
// TopRank menyediakan angka yang diperlukan untuk memutuskannya.
func (r *Roles) Grant(ctx context.Context, userID, roleID, grantedBy string) error {
	const op = "memberi peran"
	if !validUUID(userID) || !validUUID(roleID) {
		return fmt.Errorf("%s: %w", op, repo.ErrInvalidReference)
	}
	if grantedBy != "" && !validUUID(grantedBy) {
		return fmt.Errorf("%s: %w: pemberi peran bukan UUID", op, ErrInvalidInput)
	}

	const query = `insert into user_roles (user_id, role_id, granted_by) values ($1, $2, $3)`
	if _, err := r.q.Exec(ctx, query, userID, roleID, textParam(grantedBy)); err != nil {
		return repo.Err(op, err)
	}
	return nil
}

// Revoke mencabut satu peran dari pengguna. Peran yang memang tidak dimiliki
// menghasilkan repo.ErrNotFound.
//
// Mencabut pemegang TERAKHIR peran Super Admin DITOLAK dengan repo.ErrConflict:
// tanpa ini, pencabutan "Super Admin" terakhir — entah oleh admin yang salah
// klik atau lewat sesi curian — mengunci seluruh manajemen identitas tanpa
// satu pun permintaan yang terlihat gagal. Peran BIASA boleh dikosongkan:
// itu keadaan sah (mis. peran baru yang belum dipakai, atau peran yang
// penghuninya dipindah sebelum peran itu dihapus).
//
// superAdminID adalah ID peran "Super Admin". String kosong berarti "tidak
// diketahui": guard dilewati (fail-open) supaya pemakaian lama dan skrip
// yang tidak mengenal konsep itu tidak terkunci. Pemanggil HTTP wajib
// mengisinya; lihat revokeUserRole.
//
// Penolakannya atomik dalam SATU statement (klausa EXISTS di dalam DELETE),
// bukan hitung-dulu-hapus-kemudian: dua pencabutan bersamaan tidak bisa
// sama-sama melihat "masih ada satu lagi" lalu dua-duanya menghapus, karena
// baris kedua baru terhapus bila baris lain masih ada SAAT statement-nya jalan.
func (r *Roles) Revoke(ctx context.Context, userID, roleID, superAdminID string) error {
	const op = "mencabut peran"
	if !validUUID(userID) || !validUUID(roleID) {
		return fmt.Errorf("%s: %w", op, repo.ErrNotFound)
	}

	// Guard pemegang-terakhir hanya untuk Super Admin. Klausa EXISTS di
	// bawah hanya ditambahkan bila roleID adalah Super Admin yang dikenal.
	jagaTerakhir := superAdminID != "" && roleID == superAdminID
	query := `delete from user_roles where user_id = $1 and role_id = $2`
	if jagaTerakhir {
		query += `
		 and exists (select 1 from user_roles where role_id = $2 and user_id <> $1)`
	}
	tag, err := r.q.Exec(ctx, query, userID, roleID)
	if err != nil {
		return repo.Err(op, err)
	}
	if tag.RowsAffected() > 0 {
		return nil
	}
	// Nol baris berarti salah satu dari dua hal: tidak memegang peran itu
	// (404), atau satu-satunya pemegang Super Admin (409). Dibedakan dengan
	// satu query — balapan di sini tidak berbahaya karena hanya menentukan
	// PESAN error, sementara penegakan sesungguhnya sudah terjadi di
	// statement atomik.
	var memegang bool
	if err := r.q.QueryRow(ctx,
		`select exists (select 1 from user_roles where user_id = $1 and role_id = $2)`,
		userID, roleID).Scan(&memegang); err != nil {
		return repo.Err(op, err)
	}
	if memegang && jagaTerakhir {
		return fmt.Errorf("%s: peran Super Admin hanya dimiliki satu pengguna, pencabutan terakhir ditolak: %w",
			op, repo.ErrConflict)
	}
	return fmt.Errorf("%s: %w", op, repo.ErrNotFound)
}

// CountHolders menghitung pengguna yang memegang satu peran. Untuk validasi di
// lapisan handler (mis. sebelum menghapus pengguna), bukan untuk penegakan —
// penegakan pencabutan ada di Revoke yang transaksional.
func (r *Roles) CountHolders(ctx context.Context, roleID string) (int, error) {
	const op = "menghitung pemegang peran"
	if !validUUID(roleID) {
		return 0, fmt.Errorf("%s: %w", op, repo.ErrNotFound)
	}
	var n int
	if err := r.q.QueryRow(ctx,
		`select count(*) from user_roles where role_id = $1`, roleID).Scan(&n); err != nil {
		return 0, repo.Err(op, err)
	}
	return n, nil
}

// OfUser mengembalikan peran yang dimiliki pengguna, yang paling berkuasa lebih dulu.
func (r *Roles) OfUser(ctx context.Context, userID string) ([]UserRole, error) {
	const op = "mengambil peran pengguna"
	if !validUUID(userID) {
		return nil, fmt.Errorf("%s: %w", op, repo.ErrNotFound)
	}

	query := `select ` + fmt.Sprintf(roleColumns, "r") + `,
		ur.granted_at, coalesce(ur.granted_by::text, '')
	from user_roles ur
	join roles r on r.id = ur.role_id
	where ur.user_id = $1
	order by r.rank, r.name`

	rows, err := r.q.Query(ctx, query, userID)
	if err != nil {
		return nil, repo.Err(op, err)
	}
	defer rows.Close()

	var out []UserRole
	for rows.Next() {
		var ur UserRole
		if err := rows.Scan(&ur.ID, &ur.Name, &ur.Description, &ur.IsSystem, &ur.Rank,
			&ur.CreatedAt, &ur.GrantedAt, &ur.GrantedBy); err != nil {
			return nil, repo.Err(op, err)
		}
		out = append(out, ur)
	}
	if err := rows.Err(); err != nil {
		return nil, repo.Err(op, err)
	}
	return out, nil
}

// EffectivePermissions mengembalikan gabungan izin dari seluruh peran pengguna,
// terurut dan tanpa duplikat.
//
// Satu query dengan join, bukan "ambil peran lalu ambil izin tiap peran": pemeriksaan
// kewenangan terjadi pada setiap request dashboard, dan pola N+1 di sana berarti
// jumlah query ikut bertambah setiap kali seorang admin diberi peran tambahan.
// Duplikat dihilangkan database lewat DISTINCT — dua peran yang berbagi izin yang sama
// adalah keadaan normal, bukan pengecualian.
func (r *Roles) EffectivePermissions(ctx context.Context, userID string) ([]string, error) {
	const op = "mengambil izin efektif pengguna"
	if !validUUID(userID) {
		return nil, fmt.Errorf("%s: %w", op, repo.ErrNotFound)
	}

	const query = `select distinct p.key
	from user_roles ur
	join role_permissions rp on rp.role_id = ur.role_id
	join permissions p on p.id = rp.permission_id
	where ur.user_id = $1
	order by p.key`

	rows, err := r.q.Query(ctx, query, userID)
	if err != nil {
		return nil, repo.Err(op, err)
	}
	defer rows.Close()

	var out []string
	for rows.Next() {
		var key string
		if err := rows.Scan(&key); err != nil {
			return nil, repo.Err(op, err)
		}
		out = append(out, key)
	}
	if err := rows.Err(); err != nil {
		return nil, repo.Err(op, err)
	}
	return out, nil
}

// HasPermission melaporkan apakah pengguna memiliki satu izin tertentu.
//
// Dipisah dari EffectivePermissions karena EXISTS berhenti pada baris pertama yang
// cocok, sementara memuat seluruh daftar izin hanya untuk memeriksa satu di antaranya
// membuat middleware otorisasi bekerja lebih banyak daripada yang perlu.
func (r *Roles) HasPermission(ctx context.Context, userID, permissionKey string) (bool, error) {
	const op = "memeriksa izin pengguna"
	// ID yang salah bentuk berarti tidak ada izin, bukan kegagalan. Bedanya penting:
	// pemanggilnya adalah middleware otorisasi, dan error di sana lebih mudah salah
	// ditangani menjadi "lanjutkan saja" daripada jawaban false yang tegas.
	if !validUUID(userID) {
		return false, nil
	}

	const query = `select exists (
		select 1
		from user_roles ur
		join role_permissions rp on rp.role_id = ur.role_id
		join permissions p on p.id = rp.permission_id
		where ur.user_id = $1 and p.key = $2
	)`

	var ok bool
	if err := r.q.QueryRow(ctx, query, userID, permissionKey).Scan(&ok); err != nil {
		return false, repo.Err(op, err)
	}
	return ok, nil
}

// TopRank mengembalikan peringkat terkuat — angka terkecil — di antara peran pengguna.
//
// Dipakai untuk menegakkan aturan bahwa admin tidak bisa memberikan peran yang lebih
// berkuasa daripada perannya sendiri. Pengguna tanpa peran menghasilkan
// repo.ErrNotFound: pemanggil harus memutuskan sendiri apa arti "tidak punya peran"
// pada aturan yang sedang diperiksa, bukan menerima angka yang kebetulan nol dan
// karenanya tampak paling berkuasa.
func (r *Roles) TopRank(ctx context.Context, userID string) (int, error) {
	const op = "mengambil peringkat peran pengguna"
	if !validUUID(userID) {
		return 0, fmt.Errorf("%s: %w", op, repo.ErrNotFound)
	}

	const query = `select r.rank
	from user_roles ur
	join roles r on r.id = ur.role_id
	where ur.user_id = $1
	order by r.rank
	limit 1`

	var rank int
	if err := r.q.QueryRow(ctx, query, userID).Scan(&rank); err != nil {
		return 0, repo.Err(op, err)
	}
	return rank, nil
}
