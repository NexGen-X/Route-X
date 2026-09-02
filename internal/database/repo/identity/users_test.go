package identity

import (
	"errors"
	"fmt"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/NexGen-X/Route-X/internal/database/repo"
	"github.com/NexGen-X/Route-X/internal/security"
)

// Jalur bahagia seluruh operasi pengguna, dari pembuatan sampai penghapusan.
func TestUsersIntegration(t *testing.T) {
	ctx, db := newTestDB(t)
	users := NewUsers(db.Pool)

	created, err := users.Create(ctx, NewUser{
		Email:              "Operator@routex.test",
		Password:           testPassword,
		DisplayName:        "  Operator Satu  ",
		MustChangePassword: true,
	})
	if err != nil {
		t.Fatalf("Create: %v", err)
	}

	t.Run("baris baru terisi sesuai masukan", func(t *testing.T) {
		if !validUUID(created.ID) {
			t.Errorf("ID = %q, ingin UUID", created.ID)
		}
		if created.Email != "Operator@routex.test" {
			t.Errorf("Email = %q, ingin kapitalisasi asli dipertahankan", created.Email)
		}
		if created.DisplayName != "Operator Satu" {
			t.Errorf("DisplayName = %q, ingin sudah dipangkas spasinya", created.DisplayName)
		}
		if created.Status != UserStatusActive {
			t.Errorf("Status = %q, ingin active sebagai bawaan", created.Status)
		}
		if !created.MustChangePassword {
			t.Error("MustChangePassword = false, ingin true")
		}
		if created.FailedLoginAttempts != 0 || created.LockedUntil != nil || created.LastLoginAt != nil {
			t.Errorf("jejak login pengguna baru tidak bersih: %+v", created)
		}
		if created.CreatedAt.IsZero() || created.UpdatedAt.IsZero() {
			t.Error("created_at/updated_at kosong")
		}
	})

	t.Run("password tersimpan sebagai hash argon2id yang bisa diverifikasi", func(t *testing.T) {
		hash := created.PasswordHash.Reveal()
		if hash == testPassword {
			t.Fatal("password tersimpan apa adanya")
		}
		if !strings.HasPrefix(hash, "$argon2id$") {
			t.Errorf("hash tidak berformat PHC argon2id")
		}
		got, err := security.VerifyPassword(testPassword, hash)
		if err != nil {
			t.Fatalf("VerifyPassword: %v", err)
		}
		if !got.Match {
			t.Error("hash tersimpan tidak cocok dengan password aslinya")
		}
	})

	t.Run("kekuatan password divalidasi sebelum menyentuh database", func(t *testing.T) {
		_, err := users.Create(ctx, NewUser{Email: "lemah@routex.test", Password: "changeme123"})
		if !errors.Is(err, security.ErrPasswordCommon) {
			t.Fatalf("Create dengan password umum = %v, ingin ErrPasswordCommon", err)
		}
		// Tidak boleh ada baris yang tertinggal dari usaha yang ditolak.
		if _, err := users.GetByEmail(ctx, "lemah@routex.test"); !errors.Is(err, repo.ErrNotFound) {
			t.Errorf("GetByEmail = %v, ingin ErrNotFound karena barisnya tidak pernah dibuat", err)
		}
	})

	t.Run("GetByID", func(t *testing.T) {
		got, err := users.GetByID(ctx, created.ID)
		if err != nil {
			t.Fatalf("GetByID: %v", err)
		}
		if got.ID != created.ID || got.Email != created.Email {
			t.Errorf("GetByID mengembalikan baris lain: %+v", got)
		}
	})

	t.Run("Count", func(t *testing.T) {
		n, err := users.Count(ctx)
		if err != nil {
			t.Fatalf("Count: %v", err)
		}
		if n != 1 {
			t.Errorf("Count = %d, ingin 1", n)
		}
	})

	t.Run("UpdateProfile mengubah hanya field yang dikirim", func(t *testing.T) {
		name := "Operator Berubah"
		got, err := users.UpdateProfile(ctx, created.ID, ProfileUpdate{DisplayName: &name})
		if err != nil {
			t.Fatalf("UpdateProfile: %v", err)
		}
		if got.DisplayName != name {
			t.Errorf("DisplayName = %q, ingin %q", got.DisplayName, name)
		}
		if got.Email != created.Email {
			t.Errorf("Email berubah menjadi %q padahal tidak dikirim", got.Email)
		}
		if !got.UpdatedAt.After(created.UpdatedAt) {
			t.Error("updated_at tidak maju — trigger users_set_updated_at tidak jalan")
		}

		empty := ""
		got, err = users.UpdateProfile(ctx, created.ID, ProfileUpdate{DisplayName: &empty})
		if err != nil {
			t.Fatalf("UpdateProfile mengosongkan nama: %v", err)
		}
		if got.DisplayName != "" {
			t.Errorf("DisplayName = %q, ingin kosong", got.DisplayName)
		}

		if _, err := users.UpdateProfile(ctx, created.ID, ProfileUpdate{}); !errors.Is(err, ErrInvalidInput) {
			t.Errorf("UpdateProfile tanpa perubahan = %v, ingin ErrInvalidInput", err)
		}
	})

	t.Run("ChangePassword", func(t *testing.T) {
		const next = "Sandi-Baru-Yang-Panjang-9"
		if err := users.ChangePassword(ctx, created.ID, next, false); err != nil {
			t.Fatalf("ChangePassword: %v", err)
		}

		got, err := users.GetByID(ctx, created.ID)
		if err != nil {
			t.Fatalf("GetByID: %v", err)
		}
		if got.MustChangePassword {
			t.Error("MustChangePassword = true, ingin false setelah pemiliknya mengganti sendiri")
		}
		res, err := security.VerifyPassword(next, got.PasswordHash.Reveal())
		if err != nil {
			t.Fatalf("VerifyPassword: %v", err)
		}
		if !res.Match {
			t.Error("password baru tidak terverifikasi")
		}
		if old, _ := security.VerifyPassword(testPassword, got.PasswordHash.Reveal()); old.Match {
			t.Error("password lama masih berlaku")
		}

		if err := users.ChangePassword(ctx, created.ID, "pendek", false); !errors.Is(err, security.ErrPasswordTooShort) {
			t.Errorf("ChangePassword dengan password pendek = %v, ingin ErrPasswordTooShort", err)
		}
	})

	t.Run("UpdatePasswordHash untuk rehash transparan", func(t *testing.T) {
		hash, err := security.HashPassword(testPassword)
		if err != nil {
			t.Fatalf("HashPassword: %v", err)
		}
		if err := users.UpdatePasswordHash(ctx, created.ID, security.Secret(hash)); err != nil {
			t.Fatalf("UpdatePasswordHash: %v", err)
		}
		got, err := users.GetByID(ctx, created.ID)
		if err != nil {
			t.Fatalf("GetByID: %v", err)
		}
		if got.PasswordHash.Reveal() != hash {
			t.Error("hash tidak tersimpan")
		}
		if err := users.UpdatePasswordHash(ctx, created.ID, ""); !errors.Is(err, ErrInvalidInput) {
			t.Errorf("UpdatePasswordHash dengan hash kosong = %v, ingin ErrInvalidInput", err)
		}
	})

	t.Run("SetStatus hanya menyentuh status", func(t *testing.T) {
		got, err := users.SetStatus(ctx, created.ID, UserStatusDisabled)
		if err != nil {
			t.Fatalf("SetStatus: %v", err)
		}
		if got.Status != UserStatusDisabled {
			t.Errorf("Status = %q, ingin disabled", got.Status)
		}
		if got.CanLogin(time.Now()) {
			t.Error("CanLogin = true untuk akun disabled")
		}
		if _, err := users.SetStatus(ctx, created.ID, "dihapus"); !errors.Is(err, ErrInvalidInput) {
			t.Errorf("SetStatus dengan status tidak dikenal = %v, ingin ErrInvalidInput", err)
		}
		if _, err := users.SetStatus(ctx, created.ID, UserStatusActive); err != nil {
			t.Fatalf("SetStatus kembali ke active: %v", err)
		}
	})

	t.Run("Delete", func(t *testing.T) {
		if err := users.Delete(ctx, created.ID); err != nil {
			t.Fatalf("Delete: %v", err)
		}
		if _, err := users.GetByID(ctx, created.ID); !errors.Is(err, repo.ErrNotFound) {
			t.Errorf("GetByID setelah Delete = %v, ingin ErrNotFound", err)
		}
		if err := users.Delete(ctx, created.ID); !errors.Is(err, repo.ErrNotFound) {
			t.Errorf("Delete kedua = %v, ingin ErrNotFound", err)
		}
	})
}

// Email dibandingkan tanpa memperhatikan huruf besar-kecil, lewat indeks
// users_email_lower_key. Tanpa ini, "Admin@x.com" dan "admin@x.com" akan menjadi dua
// akun berbeda dan siapa pun bisa membuat akun kembar dengan mengubah kapitalisasi.
func TestUsersEmailCaseInsensitiveIntegration(t *testing.T) {
	ctx, db := newTestDB(t)
	users := NewUsers(db.Pool)

	created, err := users.Create(ctx, NewUser{Email: "Admin@x.com", Password: testPassword})
	if err != nil {
		t.Fatalf("Create: %v", err)
	}

	for _, lookup := range []string{"admin@x.com", "ADMIN@X.COM", "Admin@x.com", "  admin@x.com  "} {
		got, err := users.GetByEmail(ctx, lookup)
		if err != nil {
			t.Fatalf("GetByEmail(%q): %v", lookup, err)
		}
		if got.ID != created.ID {
			t.Errorf("GetByEmail(%q) mengembalikan baris lain", lookup)
		}
	}

	for _, dup := range []string{"admin@x.com", "ADMIN@X.COM", "aDmIn@X.cOm"} {
		_, err := users.Create(ctx, NewUser{Email: dup, Password: testPassword})
		if !errors.Is(err, repo.ErrConflict) {
			t.Errorf("Create(%q) = %v, ingin ErrConflict", dup, err)
		}
	}

	// Kapitalisasi berbeda juga tidak boleh lolos lewat pintu belakang UpdateProfile.
	other := makeUser(ctx, t, users, "lain@x.com")
	clash := "ADMIN@x.com"
	if _, err := users.UpdateProfile(ctx, other.ID, ProfileUpdate{Email: &clash}); !errors.Is(err, repo.ErrConflict) {
		t.Errorf("UpdateProfile ke email yang sudah dipakai = %v, ingin ErrConflict", err)
	}

	// Bentuk perbandingannya harus benar-benar cocok dengan ekspresi indeks, bukan
	// hanya menghasilkan baris yang benar. Ini yang membedakan lower(email) = lower($1)
	// dari email ilike $1: keduanya menemukan barisnya, tapi hanya yang pertama bisa
	// dilayani users_email_lower_key — dan jalur ini dilewati setiap percobaan login.
	//
	// Seq scan dimatikan hanya di dalam transaksi ini, karena pada tabel sekecil tabel
	// test seq scan memang selalu lebih murah dan rencana query tidak akan pernah
	// menyentuh indeks apa pun.
	tx, err := db.Pool.Begin(ctx)
	if err != nil {
		t.Fatalf("Begin: %v", err)
	}
	defer func() { _ = tx.Rollback(ctx) }()

	if _, err := tx.Exec(ctx, "set local enable_seqscan = off"); err != nil {
		t.Fatalf("mematikan seq scan: %v", err)
	}
	rows, err := tx.Query(ctx, `explain select id from users where lower(email) = lower($1)`, "admin@x.com")
	if err != nil {
		t.Fatalf("explain: %v", err)
	}
	defer rows.Close()

	var plan strings.Builder
	for rows.Next() {
		var line string
		if err := rows.Scan(&line); err != nil {
			t.Fatalf("scan explain: %v", err)
		}
		plan.WriteString(line)
		plan.WriteString("\n")
	}
	if err := rows.Err(); err != nil {
		t.Fatalf("explain: %v", err)
	}
	if !strings.Contains(plan.String(), "users_email_lower_key") {
		t.Errorf("pencarian email tidak memakai indeks users_email_lower_key, rencananya:\n%s", plan.String())
	}
}

// Pencatatan login berhasil dan gagal, termasuk penguncian saat ambang terlampaui.
func TestUsersLoginTrackingIntegration(t *testing.T) {
	ctx, db := newTestDB(t)
	users := NewUsers(db.Pool)
	user := makeUser(ctx, t, users, "login@routex.test")

	t.Run("login gagal menaikkan penghitung tanpa mengunci", func(t *testing.T) {
		for want := 1; want <= 2; want++ {
			got, err := users.RecordLoginFailure(ctx, user.ID, 3, time.Minute)
			if err != nil {
				t.Fatalf("RecordLoginFailure: %v", err)
			}
			if got.Attempts != want {
				t.Errorf("Attempts = %d, ingin %d", got.Attempts, want)
			}
			if got.Locked || got.LockedUntil != nil {
				t.Errorf("akun terkunci pada percobaan ke-%d, padahal ambangnya 3", want)
			}
		}
	})

	t.Run("percobaan yang melewati ambang mengunci akun", func(t *testing.T) {
		got, err := users.RecordLoginFailure(ctx, user.ID, 3, time.Hour)
		if err != nil {
			t.Fatalf("RecordLoginFailure: %v", err)
		}
		if got.Attempts != 3 {
			t.Fatalf("Attempts = %d, ingin 3", got.Attempts)
		}
		if !got.Locked || got.LockedUntil == nil {
			t.Fatal("akun tidak terkunci padahal ambang terlampaui")
		}
		// Batas waktu dihitung database, jadi hanya rentangnya yang bisa dipastikan.
		if until := *got.LockedUntil; until.Before(time.Now().Add(50 * time.Minute)) {
			t.Errorf("LockedUntil = %v, ingin sekitar satu jam dari sekarang", until)
		}

		stored, err := users.GetByID(ctx, user.ID)
		if err != nil {
			t.Fatalf("GetByID: %v", err)
		}
		if !stored.Locked(time.Now()) || stored.CanLogin(time.Now()) {
			t.Error("baris tersimpan tidak menunjukkan akun terkunci")
		}
		// Penguncian sementara tidak boleh menyentuh status administratif.
		if stored.Status != UserStatusActive {
			t.Errorf("Status = %q, ingin tetap active — penguncian hanya lewat locked_until", stored.Status)
		}
	})

	t.Run("status disabled tidak berubah karena percobaan gagal", func(t *testing.T) {
		disabled := makeUser(ctx, t, users, "disabled@routex.test")
		if _, err := users.SetStatus(ctx, disabled.ID, UserStatusDisabled); err != nil {
			t.Fatalf("SetStatus: %v", err)
		}
		if _, err := users.RecordLoginFailure(ctx, disabled.ID, 1, time.Hour); err != nil {
			t.Fatalf("RecordLoginFailure: %v", err)
		}
		got, err := users.GetByID(ctx, disabled.ID)
		if err != nil {
			t.Fatalf("GetByID: %v", err)
		}
		if got.Status != UserStatusDisabled {
			t.Errorf("Status = %q, ingin tetap disabled", got.Status)
		}
	})

	// Kalau penghitung tidak dimulai dari awal setelah kunci terbuka, satu salah ketik
	// akan langsung mengunci akun lagi — dan karena tidak ada pembukaan kunci sendiri,
	// pemiliknya terkurung sampai ada admin lain yang menolong.
	t.Run("penghitung dimulai dari awal setelah penguncian lewat", func(t *testing.T) {
		expiring := makeUser(ctx, t, users, "kunci-lewat@routex.test")
		for range 3 {
			if _, err := users.RecordLoginFailure(ctx, expiring.ID, 3, time.Hour); err != nil {
				t.Fatalf("RecordLoginFailure: %v", err)
			}
		}
		// Penguncian dibuat seakan-akan sudah lewat, tanpa perlu menunggu.
		if _, err := db.Pool.Exec(ctx,
			`update users set locked_until = now() - interval '1 minute' where id = $1`, expiring.ID); err != nil {
			t.Fatalf("memundurkan locked_until: %v", err)
		}

		got, err := users.RecordLoginFailure(ctx, expiring.ID, 3, time.Hour)
		if err != nil {
			t.Fatalf("RecordLoginFailure: %v", err)
		}
		if got.Attempts != 1 {
			t.Errorf("Attempts = %d, ingin 1 karena penguncian sebelumnya sudah lewat", got.Attempts)
		}
		if got.Locked || got.LockedUntil != nil {
			t.Errorf("akun langsung terkunci lagi: locked=%v locked_until=%v", got.Locked, got.LockedUntil)
		}

		// Ambang satu berarti setiap kegagalan mengunci, termasuk yang pertama setelah
		// kunci sebelumnya lewat.
		strict := makeUser(ctx, t, users, "ambang-satu@routex.test")
		if got, err := users.RecordLoginFailure(ctx, strict.ID, 1, time.Hour); err != nil {
			t.Fatalf("RecordLoginFailure: %v", err)
		} else if !got.Locked {
			t.Error("ambang 1 tidak mengunci pada kegagalan pertama")
		}
		if _, err := db.Pool.Exec(ctx,
			`update users set locked_until = now() - interval '1 minute' where id = $1`, strict.ID); err != nil {
			t.Fatalf("memundurkan locked_until: %v", err)
		}
		if got, err := users.RecordLoginFailure(ctx, strict.ID, 1, time.Hour); err != nil {
			t.Fatalf("RecordLoginFailure: %v", err)
		} else if !got.Locked || got.Attempts != 1 {
			t.Errorf("kegagalan pertama setelah kunci lewat = %+v, ingin terkunci dengan Attempts 1", got)
		}
	})

	t.Run("Unlock membersihkan kunci", func(t *testing.T) {
		got, err := users.Unlock(ctx, user.ID)
		if err != nil {
			t.Fatalf("Unlock: %v", err)
		}
		if got.FailedLoginAttempts != 0 || got.LockedUntil != nil {
			t.Errorf("kunci belum bersih: attempts=%d locked_until=%v", got.FailedLoginAttempts, got.LockedUntil)
		}
		if !got.CanLogin(time.Now()) {
			t.Error("CanLogin = false setelah Unlock")
		}
	})

	t.Run("Unlock mengaktifkan kembali akun yang dikunci admin, bukan yang disabled", func(t *testing.T) {
		if _, err := users.SetStatus(ctx, user.ID, UserStatusLocked); err != nil {
			t.Fatalf("SetStatus: %v", err)
		}
		got, err := users.Unlock(ctx, user.ID)
		if err != nil {
			t.Fatalf("Unlock: %v", err)
		}
		if got.Status != UserStatusActive {
			t.Errorf("Status = %q, ingin active", got.Status)
		}

		if _, err := users.SetStatus(ctx, user.ID, UserStatusDisabled); err != nil {
			t.Fatalf("SetStatus: %v", err)
		}
		got, err = users.Unlock(ctx, user.ID)
		if err != nil {
			t.Fatalf("Unlock: %v", err)
		}
		if got.Status != UserStatusDisabled {
			t.Errorf("Status = %q, ingin tetap disabled — membuka kunci bukan izin menghidupkan akun", got.Status)
		}
		if _, err := users.SetStatus(ctx, user.ID, UserStatusActive); err != nil {
			t.Fatalf("SetStatus: %v", err)
		}
	})

	t.Run("login berhasil menolkan penghitung dan mencatat jejaknya", func(t *testing.T) {
		if _, err := users.RecordLoginFailure(ctx, user.ID, 3, time.Hour); err != nil {
			t.Fatalf("RecordLoginFailure: %v", err)
		}
		if err := users.RecordLoginSuccess(ctx, user.ID, "203.0.113.7"); err != nil {
			t.Fatalf("RecordLoginSuccess: %v", err)
		}

		got, err := users.GetByID(ctx, user.ID)
		if err != nil {
			t.Fatalf("GetByID: %v", err)
		}
		if got.FailedLoginAttempts != 0 || got.LockedUntil != nil {
			t.Errorf("penghitung tidak dinolkan: attempts=%d locked_until=%v", got.FailedLoginAttempts, got.LockedUntil)
		}
		if got.LastLoginAt == nil {
			t.Error("last_login_at tidak terisi")
		}
		if got.LastLoginIP != "203.0.113.7" {
			t.Errorf("LastLoginIP = %q, ingin 203.0.113.7", got.LastLoginIP)
		}
	})

	t.Run("alamat IP yang tidak bisa diurai disimpan sebagai kosong", func(t *testing.T) {
		if err := users.RecordLoginSuccess(ctx, user.ID, "bukan-alamat-ip"); err != nil {
			t.Fatalf("RecordLoginSuccess: %v", err)
		}
		got, err := users.GetByID(ctx, user.ID)
		if err != nil {
			t.Fatalf("GetByID: %v", err)
		}
		if got.LastLoginIP != "" {
			t.Errorf("LastLoginIP = %q, ingin kosong", got.LastLoginIP)
		}
	})

	t.Run("pengguna yang tidak ada", func(t *testing.T) {
		const missing = "8b1f4a2c-0d3e-4f5a-9b8c-7d6e5f4a3b2c"
		if err := users.RecordLoginSuccess(ctx, missing, "203.0.113.7"); !errors.Is(err, repo.ErrNotFound) {
			t.Errorf("RecordLoginSuccess = %v, ingin ErrNotFound", err)
		}
		if _, err := users.RecordLoginFailure(ctx, missing, 3, time.Minute); !errors.Is(err, repo.ErrNotFound) {
			t.Errorf("RecordLoginFailure = %v, ingin ErrNotFound", err)
		}
		if _, err := users.RecordLoginFailure(ctx, "bukan-uuid", 3, time.Minute); !errors.Is(err, repo.ErrNotFound) {
			t.Errorf("RecordLoginFailure dengan ID bukan UUID = %v, ingin ErrNotFound", err)
		}
	})
}

// Inti perlindungan brute force: penghitung kegagalan harus tetap tepat walaupun
// percobaan datang bersamaan.
//
// Kalau kenaikannya dikerjakan sebagai baca-lalu-tulis di Go, percobaan-percobaan yang
// berjalan paralel akan sama-sama membaca nilai lama lalu sama-sama menulis nilai lama
// + 1, sehingga puluhan tebakan hanya terhitung beberapa kali dan akun tidak pernah
// terkunci — penyerang cukup mengirim tebakannya serentak untuk melewati ambang.
// Karena itu yang diuji di sini bukan "penghitungnya naik", tapi "penghitungnya naik
// tepat sebanyak percobaan yang dikirim".
func TestUsersFailedLoginAttemptsAreAtomicIntegration(t *testing.T) {
	ctx, db := newTestDB(t)
	users := NewUsers(db.Pool)
	user := makeUser(ctx, t, users, "brute@routex.test")

	// Jauh lebih banyak dari ukuran pool supaya percobaan benar-benar berebut baris
	// yang sama, bukan berjalan satu-satu.
	const attempts = 40

	var (
		wg      sync.WaitGroup
		mu      sync.Mutex
		errs    []error
		results = make([]LoginFailure, 0, attempts)
		start   = make(chan struct{})
	)
	for range attempts {
		wg.Add(1)
		go func() {
			defer wg.Done()
			<-start // semua goroutine dilepas sekaligus

			// Ambang di atas jumlah percobaan supaya yang diuji murni penghitungnya.
			got, err := users.RecordLoginFailure(ctx, user.ID, attempts+1, time.Hour)

			mu.Lock()
			defer mu.Unlock()
			if err != nil {
				errs = append(errs, err)
				return
			}
			results = append(results, got)
		}()
	}
	close(start)
	wg.Wait()

	for _, err := range errs {
		t.Errorf("RecordLoginFailure bersamaan gagal: %v", err)
	}
	if len(results) != attempts {
		t.Fatalf("percobaan yang berhasil dicatat = %d, ingin %d", len(results), attempts)
	}

	got, err := users.GetByID(ctx, user.ID)
	if err != nil {
		t.Fatalf("GetByID: %v", err)
	}
	if got.FailedLoginAttempts != attempts {
		t.Errorf("failed_login_attempts = %d, ingin %d — kenaikan penghitung tidak atomik, %d percobaan hilang",
			got.FailedLoginAttempts, attempts, attempts-got.FailedLoginAttempts)
	}
	if got.LockedUntil != nil {
		t.Errorf("LockedUntil = %v, ingin kosong karena ambangnya belum tercapai", got.LockedUntil)
	}

	// Setiap percobaan harus mendapat nomor urut yang berbeda. Dua percobaan dengan
	// Attempts yang sama berarti keduanya membaca keadaan yang sama — persis kebocoran
	// yang dicegah pola satu pernyataan UPDATE.
	seen := make(map[int]struct{}, len(results))
	for _, r := range results {
		if _, dup := seen[r.Attempts]; dup {
			t.Errorf("dua percobaan sama-sama melaporkan Attempts = %d", r.Attempts)
		}
		seen[r.Attempts] = struct{}{}
	}

	t.Run("penguncian tidak bisa dilewati dengan percobaan serentak", func(t *testing.T) {
		victim := makeUser(ctx, t, users, "serentak@routex.test")

		const (
			parallel  = 20
			threshold = 3
		)
		var wg sync.WaitGroup
		for range parallel {
			wg.Add(1)
			go func() {
				defer wg.Done()
				_, _ = users.RecordLoginFailure(ctx, victim.ID, threshold, time.Hour)
			}()
		}
		wg.Wait()

		locked, err := users.GetByID(ctx, victim.ID)
		if err != nil {
			t.Fatalf("GetByID: %v", err)
		}
		if locked.FailedLoginAttempts != parallel {
			t.Errorf("failed_login_attempts = %d, ingin %d", locked.FailedLoginAttempts, parallel)
		}
		if !locked.Locked(time.Now()) {
			t.Error("akun tidak terkunci walaupun percobaan gagal jauh melewati ambang")
		}
	})
}

// Paginasi keyset: halaman-halaman berurutan tidak boleh melewatkan atau menggandakan
// baris, walaupun ada pengguna baru dibuat di antara dua pengambilan halaman.
//
// Ini yang membedakannya dari OFFSET. Dengan OFFSET, satu baris baru di puncak daftar
// menggeser seluruh isinya satu posisi, sehingga baris terakhir halaman pertama muncul
// lagi sebagai baris pertama halaman kedua — dan pada daftar yang dibaca sambil terus
// bertambah, itu keadaan normal, bukan kasus tepi.
func TestUsersListKeysetPaginationIntegration(t *testing.T) {
	ctx, db := newTestDB(t)
	users := NewUsers(db.Pool)

	const (
		total    = 5
		pageSize = 2
	)
	// Dibuat berurutan; List mengurutkan terbaru lebih dulu, jadi urutan yang diharapkan
	// adalah kebalikan urutan pembuatan.
	order := make([]string, 0, total)
	for i := range total {
		u := makeUser(ctx, t, users, fmt.Sprintf("halaman-%d@routex.test", i))
		order = append(order, u.ID)
	}
	want := make([]string, 0, total)
	for i := len(order) - 1; i >= 0; i-- {
		want = append(want, order[i])
	}

	first, err := users.List(ctx, UserFilter{}, repo.Page{Limit: pageSize})
	if err != nil {
		t.Fatalf("List halaman pertama: %v", err)
	}
	if len(first.Users) != pageSize {
		t.Fatalf("halaman pertama memuat %d baris, ingin %d", len(first.Users), pageSize)
	}
	if first.NextCursor == "" {
		t.Fatal("NextCursor kosong padahal masih ada baris berikutnya")
	}

	// Baris baru menyusup setelah halaman pertama diambil.
	intruder := makeUser(ctx, t, users, "penyusup@routex.test")

	got := make([]string, 0, total)
	for _, u := range first.Users {
		got = append(got, u.ID)
	}

	cursor := first.NextCursor
	for pages := 0; cursor != ""; pages++ {
		if pages > total {
			t.Fatal("paginasi tidak berhenti — cursor terus menghasilkan halaman baru")
		}
		next, err := users.List(ctx, UserFilter{}, repo.Page{Limit: pageSize, Cursor: cursor})
		if err != nil {
			t.Fatalf("List halaman lanjutan: %v", err)
		}
		if len(next.Users) == 0 {
			t.Fatal("halaman lanjutan kosong padahal cursor-nya diberikan")
		}
		for _, u := range next.Users {
			got = append(got, u.ID)
		}
		cursor = next.NextCursor
	}

	if len(got) != len(want) {
		t.Fatalf("total baris terbaca = %d, ingin %d (%d asli, penyusup tidak boleh ikut)",
			len(got), len(want), total)
	}
	seen := make(map[string]int, len(got))
	for i, id := range got {
		seen[id]++
		if seen[id] > 1 {
			t.Errorf("baris %s terbaca dua kali", id)
		}
		if id != want[i] {
			t.Errorf("baris ke-%d = %s, ingin %s", i, id, want[i])
		}
		if id == intruder.ID {
			t.Error("baris yang baru dibuat ikut terbaca di halaman lanjutan")
		}
	}

	t.Run("cursor rusak dilaporkan", func(t *testing.T) {
		for _, bad := range []string{"bukan-cursor", base64Raw("123|bukan-uuid")} {
			if _, err := users.List(ctx, UserFilter{}, repo.Page{Cursor: bad}); !errors.Is(err, ErrInvalidCursor) {
				t.Errorf("List dengan cursor %q = %v, ingin ErrInvalidCursor", bad, err)
			}
		}
	})

	t.Run("limit dinormalkan", func(t *testing.T) {
		page, err := users.List(ctx, UserFilter{}, repo.Page{})
		if err != nil {
			t.Fatalf("List: %v", err)
		}
		if len(page.Users) != total+1 {
			t.Errorf("baris terbaca = %d, ingin %d dengan limit bawaan", len(page.Users), total+1)
		}
		if page.NextCursor != "" {
			t.Error("NextCursor terisi padahal seluruh baris sudah masuk satu halaman")
		}
	})

	t.Run("filter status", func(t *testing.T) {
		if _, err := users.SetStatus(ctx, intruder.ID, UserStatusDisabled); err != nil {
			t.Fatalf("SetStatus: %v", err)
		}
		page, err := users.List(ctx, UserFilter{Status: UserStatusDisabled}, repo.Page{})
		if err != nil {
			t.Fatalf("List: %v", err)
		}
		if len(page.Users) != 1 || page.Users[0].ID != intruder.ID {
			t.Errorf("filter status disabled mengembalikan %d baris yang salah", len(page.Users))
		}
		if _, err := users.List(ctx, UserFilter{Status: "entah"}, repo.Page{}); !errors.Is(err, ErrInvalidInput) {
			t.Error("filter status tidak dikenal tidak dilaporkan")
		}
	})
}
