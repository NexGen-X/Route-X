package identity

import (
	"context"
	"fmt"
	"strings"
	"time"

	"github.com/jackc/pgx/v5"

	"github.com/NexGen-X/Route-X/internal/database/repo"
	"github.com/NexGen-X/Route-X/internal/security"
)

// UserStatus adalah keadaan administratif sebuah akun, sesuai constraint
// users_status_valid.
//
// Status terpisah dari penguncian brute force: locked_until adalah kunci sementara
// yang lepas sendiri, sedangkan status adalah keputusan manusia. Pemisahan ini yang
// membuat percobaan login gagal tidak bisa mengubah akun yang sudah dinonaktifkan
// menjadi seakan-akan hanya terkunci sementara.
type UserStatus string

// Nilai UserStatus yang sah.
const (
	// UserStatusActive berarti akun boleh masuk.
	UserStatusActive UserStatus = "active"
	// UserStatusDisabled berarti akun dimatikan admin dan tidak bisa masuk lagi.
	UserStatusDisabled UserStatus = "disabled"
	// UserStatusLocked berarti akun dikunci admin, biasanya setelah insiden.
	UserStatusLocked UserStatus = "locked"
)

// Valid melaporkan apakah status ada di daftar yang diterima database.
func (s UserStatus) Valid() bool {
	switch s {
	case UserStatusActive, UserStatusDisabled, UserStatusLocked:
		return true
	default:
		return false
	}
}

// Ambang penguncian bawaan, dipakai RecordLoginFailure bila pemanggil tidak
// menentukan sendiri. Nilainya cukup longgar untuk salah ketik berulang, tapi
// membuat penebakan password menjadi tidak praktis.
const (
	// DefaultLoginFailureThreshold adalah jumlah kegagalan berturut-turut sebelum
	// akun dikunci sementara.
	DefaultLoginFailureThreshold = 5
	// DefaultLoginLockDuration adalah lama penguncian sementara.
	DefaultLoginLockDuration = 15 * time.Minute
)

// User adalah satu baris tabel users.
//
// PasswordHash ikut dibawa karena jalur login memerlukannya untuk verifikasi, tetapi
// bertipe security.Secret dan bertanda json:"-": nilainya tidak bisa ikut terserialisasi
// ke respons API maupun tercetak ke log walaupun seseorang lupa menyaringnya.
type User struct {
	ID           string          `json:"id"`
	Email        string          `json:"email"`
	DisplayName  string          `json:"display_name,omitempty"`
	Status       UserStatus      `json:"status"`
	PasswordHash security.Secret `json:"-"`

	FailedLoginAttempts int        `json:"failed_login_attempts"`
	LockedUntil         *time.Time `json:"locked_until,omitempty"`

	LastLoginAt        *time.Time `json:"last_login_at,omitempty"`
	LastLoginIP        string     `json:"last_login_ip,omitempty"`
	MustChangePassword bool       `json:"must_change_password"`

	CreatedAt time.Time `json:"created_at"`
	UpdatedAt time.Time `json:"updated_at"`
}

// Locked melaporkan apakah akun sedang terkunci sementara pada waktu now.
func (u User) Locked(now time.Time) bool {
	return u.LockedUntil != nil && u.LockedUntil.After(now)
}

// CanLogin melaporkan apakah akun boleh masuk pada waktu now, yaitu berstatus active
// dan tidak sedang terkunci sementara.
//
// Ini sengaja diperiksa di Go, bukan disaring di dalam query pencarian pengguna:
// lapisan auth tetap perlu tahu bedanya "password salah", "akun dinonaktifkan", dan
// "akun terkunci sampai jam berapa" untuk memilih pesan dan mencatat audit yang tepat.
func (u User) CanLogin(now time.Time) bool {
	return u.Status == UserStatusActive && !u.Locked(now)
}

// NewUser adalah masukan pembuatan pengguna. Status kosong berarti active.
type NewUser struct {
	Email       string
	Password    security.Secret
	DisplayName string
	Status      UserStatus
	// MustChangePassword memaksa penggantian password saat login pertama, dipakai
	// untuk admin bootstrap yang password awalnya berasal dari environment.
	MustChangePassword bool
}

// ProfileUpdate adalah perubahan profil sebagian: field nil berarti tidak diubah.
//
// Pointer dipakai daripada nilai kosong karena keduanya berbeda maksud — "jangan
// sentuh nama tampilan" tidak sama dengan "kosongkan nama tampilan".
type ProfileUpdate struct {
	Email       *string
	DisplayName *string
}

// UserFilter menyaring daftar pengguna. Status kosong berarti semua status.
type UserFilter struct {
	Status UserStatus
}

// UserPage adalah satu halaman daftar pengguna.
type UserPage struct {
	Users []User `json:"users"`
	// NextCursor kosong berarti tidak ada halaman berikutnya.
	NextCursor string `json:"next_cursor,omitempty"`
}

// LoginFailure adalah keadaan akun setelah satu percobaan login gagal dicatat.
type LoginFailure struct {
	// Attempts adalah jumlah kegagalan berturut-turut setelah percobaan ini.
	Attempts int
	// LockedUntil berisi batas waktu penguncian bila ambang terlampaui.
	LockedUntil *time.Time
	// Locked berarti percobaan ini yang membuat akun terkunci atau memperpanjang
	// penguncian yang sedang berjalan.
	Locked bool
}

// Users adalah repository tabel users.
type Users struct {
	q repo.Querier
}

// NewUsers membuat repository pengguna di atas q. Kirim *pgxpool.Pool untuk operasi
// biasa, atau pgx.Tx dari repo.InTx bila operasinya harus atomik bersama repository
// lain — misalnya membuat pengguna sekaligus memberinya peran.
func NewUsers(q repo.Querier) *Users { return &Users{q: q} }

// userColumns adalah daftar kolom yang dibaca setiap query pengguna, dalam urutan
// yang diharapkan scanUser.
const userColumns = `id::text, email, coalesce(display_name, ''), password_hash, status,
	failed_login_attempts, locked_until, last_login_at, coalesce(host(last_login_ip), ''),
	must_change_password, created_at, updated_at`

// scanUser membaca satu baris pengguna.
func scanUser(row pgx.Row) (User, error) {
	var (
		u      User
		hash   string
		status string
	)
	err := row.Scan(&u.ID, &u.Email, &u.DisplayName, &hash, &status,
		&u.FailedLoginAttempts, &u.LockedUntil, &u.LastLoginAt, &u.LastLoginIP,
		&u.MustChangePassword, &u.CreatedAt, &u.UpdatedAt)
	if err != nil {
		return User{}, err
	}
	u.PasswordHash = security.Secret(hash)
	u.Status = UserStatus(status)
	return u, nil
}

// Create menambahkan pengguna baru dan mengembalikan barisnya.
//
// Kekuatan password diperiksa lebih dulu, sebelum ada pekerjaan hashing maupun
// perjalanan ke database: dengan urutan ini kebijakan password berlaku untuk semua
// pemanggil, bukan hanya untuk form yang kebetulan memvalidasinya di sisi HTTP.
//
// Email disimpan apa adanya termasuk huruf besar-kecilnya, sementara keunikannya
// dijaga indeks users_email_lower_key; email yang sudah dipakai — dalam kapitalisasi
// apa pun — menghasilkan repo.ErrConflict. Email kosong sengaja tidak diperiksa di
// sini supaya aturannya hanya hidup di satu tempat, yaitu constraint
// users_email_not_empty.
func (r *Users) Create(ctx context.Context, in NewUser) (User, error) {
	const op = "membuat pengguna"

	if err := security.ValidatePasswordStrength(in.Password.Reveal()); err != nil {
		return User{}, fmt.Errorf("%s: %w: %w", op, repo.ErrConstraint, err)
	}
	status := in.Status
	if status == "" {
		status = UserStatusActive
	}
	if !status.Valid() {
		return User{}, fmt.Errorf("%s: %w: %w: status %q tidak dikenal", op, repo.ErrConstraint, ErrInvalidInput, status)
	}

	hash, err := security.HashPassword(in.Password.Reveal())
	if err != nil {
		// Pesan dari HashPassword tidak memuat password, tapi tetap dilewatkan repo.Err
		// supaya bentuk error paket ini seragam.
		return User{}, repo.Err(op, err)
	}

	const query = `insert into users (email, password_hash, display_name, status, must_change_password)
	values ($1, $2, $3, $4, $5)
	returning ` + userColumns

	u, err := scanUser(r.q.QueryRow(ctx, query,
		strings.TrimSpace(in.Email), hash, textParam(strings.TrimSpace(in.DisplayName)),
		string(status), in.MustChangePassword))
	if err != nil {
		return User{}, repo.Err(op, err)
	}
	return u, nil
}

// GetByID mengambil pengguna berdasarkan ID.
func (r *Users) GetByID(ctx context.Context, id string) (User, error) {
	const op = "mengambil pengguna berdasarkan ID"
	if !validUUID(id) {
		return User{}, fmt.Errorf("%s: %w", op, repo.ErrNotFound)
	}

	const query = `select ` + userColumns + ` from users where id = $1`
	u, err := scanUser(r.q.QueryRow(ctx, query, id))
	if err != nil {
		return User{}, repo.Err(op, err)
	}
	return u, nil
}

// GetByEmail mengambil pengguna berdasarkan email, tanpa memperhatikan huruf
// besar-kecil.
//
// Perbandingannya lower(email) = lower($1) — bentuk yang sama persis dengan indeks
// users_email_lower_key, sehingga pencarian ini memakai indeks itu. Menurunkan huruf
// di sisi Go akan salah untuk karakter non-ASCII karena aturan strings.ToLower dan
// lower() PostgreSQL tidak identik.
func (r *Users) GetByEmail(ctx context.Context, email string) (User, error) {
	const op = "mengambil pengguna berdasarkan email"

	const query = `select ` + userColumns + ` from users where lower(email) = lower($1)`
	u, err := scanUser(r.q.QueryRow(ctx, query, strings.TrimSpace(email)))
	if err != nil {
		return User{}, repo.Err(op, err)
	}
	return u, nil
}

// Count mengembalikan jumlah pengguna. Dipakai jalur bootstrap untuk memutuskan
// apakah admin pertama perlu dibuat dari environment.
func (r *Users) Count(ctx context.Context) (int, error) {
	const op = "menghitung pengguna"
	var n int
	if err := r.q.QueryRow(ctx, `select count(*) from users`).Scan(&n); err != nil {
		return 0, repo.Err(op, err)
	}
	return n, nil
}

// List mengembalikan satu halaman pengguna, terbaru lebih dulu.
//
// Paginasinya keyset atas (created_at, id): posisi halaman berikutnya ditentukan
// baris terakhir, bukan OFFSET. Selain berbiaya tetap, sifat itu juga yang membuat
// halaman-halaman berurutan tidak melewatkan maupun menggandakan baris ketika ada
// pengguna baru dibuat di antara dua pengambilan halaman — dengan OFFSET, baris baru
// di puncak daftar akan menggeser seluruh isi dan membuat satu baris muncul dua kali.
func (r *Users) List(ctx context.Context, f UserFilter, p repo.Page) (UserPage, error) {
	const op = "mendaftar pengguna"

	conds := &clauseList{}
	if f.Status != "" {
		if !f.Status.Valid() {
			return UserPage{}, fmt.Errorf("%s: %w: status %q tidak dikenal", op, ErrInvalidInput, f.Status)
		}
		conds.add("status = %s", string(f.Status))
	}
	if p.Cursor != "" {
		at, id, err := decodeCursor(p.Cursor)
		if err != nil {
			return UserPage{}, fmt.Errorf("%s: %w", op, err)
		}
		if !validUUID(id) {
			return UserPage{}, fmt.Errorf("%s: %w: pengenal di dalam cursor bukan UUID", op, ErrInvalidCursor)
		}
		conds.add("(created_at, id) < (%s::timestamptz, %s::uuid)", at, id)
	}

	limit := p.Normalize()
	// Satu baris lebih diminta daripada yang dikembalikan. Itu cara mengetahui masih
	// ada halaman berikutnya tanpa query COUNT tambahan, sekaligus mencegah cursor
	// yang menuntun ke halaman kosong.
	where := conds.where()
	rowLimit := conds.placeholder(limit + 1)
	query := `select ` + userColumns + ` from users` + where +
		` order by created_at desc, id desc limit ` + rowLimit

	rows, err := r.q.Query(ctx, query, conds.args...)
	if err != nil {
		return UserPage{}, repo.Err(op, err)
	}
	defer rows.Close()

	page := UserPage{Users: make([]User, 0, limit)}
	for rows.Next() {
		u, err := scanUser(rows)
		if err != nil {
			return UserPage{}, repo.Err(op, err)
		}
		page.Users = append(page.Users, u)
	}
	if err := rows.Err(); err != nil {
		return UserPage{}, repo.Err(op, err)
	}

	if len(page.Users) > limit {
		last := page.Users[limit-1]
		page.Users = page.Users[:limit]
		page.NextCursor = encodeCursor(last.CreatedAt, last.ID)
	}
	return page, nil
}

// UpdateProfile mengubah email dan/atau nama tampilan, lalu mengembalikan baris
// terbaru. Field yang nil tidak disentuh; nama tampilan yang dikirim kosong menjadi
// NULL. Kolom updated_at dipelihara trigger users_set_updated_at, bukan di sini.
func (r *Users) UpdateProfile(ctx context.Context, id string, in ProfileUpdate) (User, error) {
	const op = "memperbarui profil pengguna"
	if !validUUID(id) {
		return User{}, fmt.Errorf("%s: %w", op, repo.ErrNotFound)
	}

	sets := &clauseList{}
	if in.Email != nil {
		sets.add("email = %s", strings.TrimSpace(*in.Email))
	}
	if in.DisplayName != nil {
		sets.add("display_name = %s", textParam(strings.TrimSpace(*in.DisplayName)))
	}
	if sets.empty() {
		return User{}, fmt.Errorf("%s: %w: tidak ada field yang diubah", op, ErrInvalidInput)
	}

	assignments := sets.join(", ")
	idHolder := sets.placeholder(id)
	query := `update users set ` + assignments + ` where id = ` + idHolder + ` returning ` + userColumns

	u, err := scanUser(r.q.QueryRow(ctx, query, sets.args...))
	if err != nil {
		return User{}, repo.Err(op, err)
	}
	return u, nil
}

// ChangePassword mengganti password pengguna setelah memvalidasi kekuatannya.
//
// mustChange menandai password sebagai sementara: dipakai saat admin mereset password
// orang lain, sehingga pemiliknya wajib menggantinya di login berikutnya.
//
// Sesi yang sedang berjalan tidak dicabut di sini. Itu keputusan pemanggil, dan
// biasanya harus atomik bersama penggantian ini — rakit keduanya di dalam repo.InTx
// bersama Sessions.RevokeAllOfUser.
func (r *Users) ChangePassword(ctx context.Context, id string, password security.Secret, mustChange bool) error {
	const op = "mengganti password pengguna"
	if !validUUID(id) {
		return fmt.Errorf("%s: %w", op, repo.ErrNotFound)
	}
	if err := security.ValidatePasswordStrength(password.Reveal()); err != nil {
		return fmt.Errorf("%s: %w: %w", op, repo.ErrConstraint, err)
	}

	hash, err := security.HashPassword(password.Reveal())
	if err != nil {
		return repo.Err(op, err)
	}

	const query = `update users set password_hash = $2, must_change_password = $3 where id = $1`
	tag, err := r.q.Exec(ctx, query, id, hash, mustChange)
	if err != nil {
		return repo.Err(op, err)
	}
	if tag.RowsAffected() == 0 {
		return fmt.Errorf("%s: %w", op, repo.ErrNotFound)
	}
	return nil
}

// UpdatePasswordHash menimpa hash password dengan hash yang sudah dihitung pemanggil.
//
// Dipakai untuk rehash transparan: saat login berhasil dan security.VerifyPassword
// melaporkan NeedsRehash, lapisan auth menghitung hash baru dengan parameter biaya
// terkini lalu menyimpannya di sini. Kekuatan password tidak divalidasi lagi — yang
// masuk sudah berupa hash, bukan password.
func (r *Users) UpdatePasswordHash(ctx context.Context, id string, hash security.Secret) error {
	const op = "memperbarui hash password"
	if !validUUID(id) {
		return fmt.Errorf("%s: %w", op, repo.ErrNotFound)
	}
	if hash.IsZero() {
		return fmt.Errorf("%s: %w: hash kosong", op, ErrInvalidInput)
	}

	const query = `update users set password_hash = $2 where id = $1`
	tag, err := r.q.Exec(ctx, query, id, hash.Reveal())
	if err != nil {
		return repo.Err(op, err)
	}
	if tag.RowsAffected() == 0 {
		return fmt.Errorf("%s: %w", op, repo.ErrNotFound)
	}
	return nil
}

// SetStatus mengubah status administratif akun dan mengembalikan baris terbaru.
//
// Hanya kolom status yang disentuh. Penguncian sementara akibat percobaan login gagal
// dibersihkan dengan Unlock, bukan dengan mengaktifkan kembali akun di sini.
func (r *Users) SetStatus(ctx context.Context, id string, status UserStatus) (User, error) {
	const op = "mengubah status pengguna"
	if !validUUID(id) {
		return User{}, fmt.Errorf("%s: %w", op, repo.ErrNotFound)
	}
	if !status.Valid() {
		return User{}, fmt.Errorf("%s: %w: %w: status %q tidak dikenal", op, repo.ErrConstraint, ErrInvalidInput, status)
	}

	const query = `update users set status = $2 where id = $1 returning ` + userColumns
	u, err := scanUser(r.q.QueryRow(ctx, query, id, string(status)))
	if err != nil {
		return User{}, repo.Err(op, err)
	}
	return u, nil
}

// Unlock membuka akun yang terkunci: penghitung kegagalan dinolkan dan penguncian
// sementara dihapus.
//
// Status 'locked' sekalian dikembalikan ke 'active' karena itulah maksud admin yang
// menekan "buka kunci". Akun 'disabled' tetap disabled — membuka kunci bukan izin
// untuk menghidupkan kembali akun yang sengaja dimatikan.
func (r *Users) Unlock(ctx context.Context, id string) (User, error) {
	const op = "membuka kunci pengguna"
	if !validUUID(id) {
		return User{}, fmt.Errorf("%s: %w", op, repo.ErrNotFound)
	}

	const query = `update users set
		failed_login_attempts = 0,
		locked_until = null,
		status = case when status = 'locked' then 'active' else status end
	where id = $1
	returning ` + userColumns

	u, err := scanUser(r.q.QueryRow(ctx, query, id))
	if err != nil {
		return User{}, repo.Err(op, err)
	}
	return u, nil
}

// Delete menghapus pengguna.
//
// Sesi dan pemberian perannya ikut terhapus lewat ON DELETE CASCADE, sementara
// catatan audit tetap ada: audit_logs.actor_user_id di-set null dan cuplikan
// actor_email/actor_role yang tersimpan di sana tetap menjawab siapa pelakunya.
func (r *Users) Delete(ctx context.Context, id string) error {
	const op = "menghapus pengguna"
	if !validUUID(id) {
		return fmt.Errorf("%s: %w", op, repo.ErrNotFound)
	}

	tag, err := r.q.Exec(ctx, `delete from users where id = $1`, id)
	if err != nil {
		return repo.Err(op, err)
	}
	if tag.RowsAffected() == 0 {
		return fmt.Errorf("%s: %w", op, repo.ErrNotFound)
	}
	return nil
}

// RecordLoginSuccess mencatat login yang berhasil: penghitung kegagalan dinolkan,
// penguncian sementara dilepas, dan jejak login terakhir diperbarui.
//
// Penghitung wajib dinolkan di sini, bukan dibiarkan kedaluwarsa sendiri. Kalau tidak,
// kegagalan yang menumpuk selama berbulan-bulan pada akun yang tetap dipakai normal
// akhirnya akan mengunci pemiliknya tanpa ada serangan sama sekali.
//
// ip boleh kosong atau tidak bisa diurai; nilainya lalu disimpan sebagai NULL.
func (r *Users) RecordLoginSuccess(ctx context.Context, id, ip string) error {
	const op = "mencatat login berhasil"
	if !validUUID(id) {
		return fmt.Errorf("%s: %w", op, repo.ErrNotFound)
	}

	const query = `update users set
		failed_login_attempts = 0,
		locked_until = null,
		last_login_at = now(),
		last_login_ip = $2
	where id = $1`

	tag, err := r.q.Exec(ctx, query, id, ipParam(ip))
	if err != nil {
		return repo.Err(op, err)
	}
	if tag.RowsAffected() == 0 {
		return fmt.Errorf("%s: %w", op, repo.ErrNotFound)
	}
	return nil
}

// RecordLoginFailure menaikkan penghitung kegagalan login dan mengunci akun sementara
// begitu ambang terlampaui.
//
// threshold ≤ 0 memakai DefaultLoginFailureThreshold, lockFor ≤ 0 memakai
// DefaultLoginLockDuration. Penghitung dimulai dari satu lagi bila penguncian
// sebelumnya sudah kedaluwarsa.
//
// Kenaikan penghitung dikerjakan database di dalam satu pernyataan UPDATE, dengan
// nilai barunya dihitung dari kolomnya sendiri (failed_login_attempts + 1) dan bukan
// dibaca ke Go lalu dituliskan kembali. Ini bukan soal menghemat satu perjalanan:
// pola baca-lalu-tulis bocor persis pada kondisi yang ingin dicegah. Beberapa
// percobaan yang datang bersamaan akan sama-sama membaca nilai lama, lalu sama-sama
// menuliskan nilai lama + 1, sehingga puluhan tebakan paralel hanya menaikkan
// penghitung satu kali dan penguncian tidak pernah tercapai. Dengan satu pernyataan
// UPDATE, PostgreSQL mengunci baris: percobaan kedua menunggu, lalu menghitung ulang
// seluruh ekspresi SET-nya di atas versi baris yang sudah diperbarui, jadi setiap
// percobaan benar-benar terhitung.
//
// Penguncian menyentuh locked_until saja dan tidak mengubah status. Akun yang sudah
// 'disabled' tidak boleh berubah menjadi seakan-akan hanya terkunci sementara, dan
// kunci yang lepas sendiri tidak perlu campur tangan admin untuk dibersihkan.
func (r *Users) RecordLoginFailure(ctx context.Context, id string, threshold int, lockFor time.Duration) (LoginFailure, error) {
	const op = "mencatat login gagal"
	if !validUUID(id) {
		return LoginFailure{}, fmt.Errorf("%s: %w", op, repo.ErrNotFound)
	}
	if threshold <= 0 {
		threshold = DefaultLoginFailureThreshold
	}
	if lockFor <= 0 {
		lockFor = DefaultLoginLockDuration
	}

	// Penghitung dimulai dari awal begitu penguncian sebelumnya lewat. Tanpa itu,
	// penghitung yang sudah melewati ambang membuat satu salah ketik sesudah kunci
	// terbuka langsung mengunci akun lagi selama satu periode penuh — dan karena tidak
	// ada pembukaan kunci sendiri, pemilik akun yang sedang panik bisa terkurung tanpa
	// jalan keluar selain campur tangan admin lain. Laju serangan tetap terbatas
	// ambang percobaan per periode penguncian.
	//
	// Semuanya tetap satu pernyataan UPDATE: seluruh ekspresi SET dihitung terhadap
	// versi baris yang sama, yang sudah dikunci PostgreSQL, jadi sifat atomiknya tidak
	// berubah. Batas waktu penguncian dihitung dari now() milik database supaya tidak
	// bergantung pada jam proses aplikasi, yang bisa berbeda antar replika.
	const query = `update users set
		failed_login_attempts = case
			when locked_until is not null and locked_until <= now() then 1
			else failed_login_attempts + 1
		end,
		locked_until = case
			when locked_until is not null and locked_until <= now()
				then case when $2 <= 1 then now() + make_interval(secs => $3) end
			when failed_login_attempts + 1 >= $2 then now() + make_interval(secs => $3)
			else locked_until
		end
	where id = $1
	returning failed_login_attempts, locked_until, locked_until is not null and locked_until > now()`

	var out LoginFailure
	err := r.q.QueryRow(ctx, query, id, threshold, seconds(lockFor)).
		Scan(&out.Attempts, &out.LockedUntil, &out.Locked)
	if err != nil {
		return LoginFailure{}, repo.Err(op, err)
	}
	return out, nil
}
