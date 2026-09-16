package auth

import (
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"strings"
	"testing"
	"time"
	"unicode/utf8"

	"github.com/NexGen-X/Route-X/internal/database/repo"
	"github.com/NexGen-X/Route-X/internal/database/repo/identity"
	"github.com/NexGen-X/Route-X/internal/database/seed"
	"github.com/NexGen-X/Route-X/internal/security"
)

func TestLoginSuccess(t *testing.T) {
	e := newEnv(t)
	user := e.makeUser(t, "operator@example.test", seed.RoleOperator)

	res, err := e.svc.Login(e.ctx, "operator@example.test", security.Secret(testPassword),
		"203.0.113.9", "uji/1.0")
	if err != nil {
		t.Fatalf("Login: %v", err)
	}

	if res.Principal.User.ID != user.ID {
		t.Errorf("ID pengguna = %q, mau %q", res.Principal.User.ID, user.ID)
	}
	if res.Token.IsZero() {
		t.Fatal("token sesi kosong")
	}
	if res.Principal.SessionID == "" || res.Principal.SessionID != res.Principal.Session.ID {
		t.Errorf("SessionID = %q, Session.ID = %q", res.Principal.SessionID, res.Principal.Session.ID)
	}
	if got := res.Principal.TopRole(); got != "Admin" {
		t.Errorf("TopRole() = %q, mau %q", got, "Admin")
	}
	if !res.Principal.Can(seed.PermProvidersWrite) {
		t.Errorf("Admin seharusnya punya %s; izin = %v", seed.PermProvidersWrite, res.Principal.Permissions)
	}

	// Email case-insensitive: repository sudah menjaminnya, tetapi jalur login harus
	// benar-benar memakainya.
	if _, err := e.svc.Login(e.ctx, "OPERATOR@Example.TEST", security.Secret(testPassword), "", ""); err != nil {
		t.Errorf("login dengan email berbeda kapitalisasi: %v", err)
	}

	// Token yang baru dibuat harus langsung bisa dipakai.
	p, err := e.svc.Authenticate(e.ctx, res.Token)
	if err != nil {
		t.Fatalf("Authenticate: %v", err)
	}
	if p.SessionID != res.Principal.SessionID {
		t.Errorf("sesi hasil Authenticate = %q, mau %q", p.SessionID, res.Principal.SessionID)
	}

	// Penghitung kegagalan dan penguncian sudah dinolkan RecordLoginSuccess.
	if res.Principal.User.FailedLoginAttempts != 0 || res.Principal.User.LockedUntil != nil {
		t.Errorf("penghitung kegagalan = %d, locked_until = %v; keduanya harus kosong",
			res.Principal.User.FailedLoginAttempts, res.Principal.User.LockedUntil)
	}
}

// Keempat kegagalan login harus tampak identik dari luar: satu tipe error, satu pesan, satu
// sentinel. Yang boleh berbeda hanya Reason, yang tidak pernah dikirim ke klien.
func TestLoginFailuresAreIndistinguishable(t *testing.T) {
	e := newEnv(t)
	e.makeUser(t, "ada@example.test", seed.RoleViewer)
	e.makeUser(t, "mati@example.test", seed.RoleViewer)
	e.makeUser(t, "terkunci@example.test", seed.RoleViewer)

	e.exec(t, `update users set status = 'disabled' where email = 'mati@example.test'`)
	e.exec(t, `update users set locked_until = now() + interval '10 minutes',
		failed_login_attempts = 5 where email = 'terkunci@example.test'`)

	cases := []struct {
		name       string
		email      string
		password   string
		wantReason FailureReason
	}{
		{"email tidak ada", "tidak-ada@example.test", testPassword, ReasonUnknownEmail},
		{"password salah", "ada@example.test", "Password-Yang-Salah-2026", ReasonBadPassword},
		{"akun dimatikan", "mati@example.test", testPassword, ReasonDisabled},
		{"akun terkunci", "terkunci@example.test", testPassword, ReasonLocked},
		{"email kosong", "   ", testPassword, ReasonUnknownEmail},
	}

	messages := map[string]struct{}{}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			res, err := e.svc.Login(e.ctx, tc.email, security.Secret(tc.password), "203.0.113.9", "uji/1.0")
			if err == nil {
				t.Fatalf("login seharusnya gagal, tetapi berhasil: %+v", res.Principal.SessionID)
			}
			if res != nil {
				t.Error("hasil login tidak nil pada kegagalan")
			}
			if !errors.Is(err, ErrLoginFailed) {
				t.Fatalf("err = %v, mau membungkus ErrLoginFailed", err)
			}
			var failure *LoginError
			if !errors.As(err, &failure) {
				t.Fatalf("err = %v, mau bertipe *LoginError", err)
			}
			if failure.Reason != tc.wantReason {
				t.Errorf("Reason = %q, mau %q", failure.Reason, tc.wantReason)
			}
			if err.Error() != MessageLoginFailed {
				t.Errorf("pesan = %q, mau %q", err.Error(), MessageLoginFailed)
			}
			messages[err.Error()] = struct{}{}
		})
	}

	if len(messages) != 1 {
		t.Errorf("kegagalan login menghasilkan %d pesan berbeda, mau tepat 1: %v", len(messages), messages)
	}
}

// Lamanya jawaban untuk email yang tidak terdaftar harus sebanding dengan lamanya jawaban
// untuk password yang salah. Kalau tidak, selisihnya bisa dipakai memetakan akun mana yang
// ada tanpa perlu menebak satu password pun.
//
// Yang dibandingkan adalah nilai MINIMUM dari beberapa percobaan, bukan rata-ratanya:
// gangguan (penjadwalan, autovacuum, GC) hanya bisa menambah waktu, jadi minimum adalah
// perkiraan paling stabil dari biaya sebenarnya dan membuat test ini tidak rewel.
func TestLoginTimingComparable(t *testing.T) {
	e := newEnv(t)
	e.makeUser(t, "ada@example.test", seed.RoleViewer)

	measure := func(email, password string) time.Duration {
		start := time.Now()
		if _, err := e.svc.Login(e.ctx, email, security.Secret(password), "", ""); err == nil {
			t.Fatalf("login %q seharusnya gagal", email)
		}
		return time.Since(start)
	}

	// Pemanasan: koneksi pool, rencana query, dan hash pembanding argon2 tidak boleh ikut
	// terukur pada percobaan pertama.
	measure("tidak-ada@example.test", testPassword)
	measure("ada@example.test", "Password-Yang-Salah-2026")

	const rounds = 3
	unknown, wrong := time.Hour, time.Hour
	for i := 0; i < rounds; i++ {
		unknown = min(unknown, measure("tidak-ada@example.test", testPassword))
		wrong = min(wrong, measure("ada@example.test", "Password-Yang-Salah-2026"))
	}

	// Toleransi longgar dengan sengaja: yang harus dibuktikan adalah kerja argon2 memang
	// dilakukan di kedua jalur — bukan bahwa keduanya sama sampai mikrodetik.
	const tolerance = 3.0
	ratio := float64(max(unknown, wrong)) / float64(min(unknown, wrong))
	if ratio > tolerance {
		t.Errorf("selisih waktu terlalu besar: email tidak ada %v, password salah %v (rasio %.2f > %.1f); "+
			"apakah security.BurnVerifyTime terlewat?", unknown, wrong, ratio, tolerance)
	}
	t.Logf("email tidak ada %v, password salah %v (rasio %.2f)", unknown, wrong, ratio)
}

// Hash berbiaya rendah harus ditulis ulang dengan parameter terkini saat login berhasil,
// tanpa mengganggu login itu sendiri.
func TestLoginRehashesWeakHash(t *testing.T) {
	e := newEnv(t)
	user := e.makeUser(t, "lama@example.test", seed.RoleViewer)

	// Hash dengan biaya jauh di bawah default, seperti yang akan ada di database yang
	// dibuat sebelum parameter biaya dinaikkan.
	weak, err := security.HashPasswordWith(testPassword, security.Argon2Params{
		Memory: 8 * 1024, Iterations: 1, Parallelism: 1, SaltLength: 8, KeyLength: 16,
	})
	if err != nil {
		t.Fatalf("HashPasswordWith: %v", err)
	}
	e.exec(t, `update users set password_hash = $2 where id = $1`, user.ID, weak)

	if _, err := e.svc.Login(e.ctx, "lama@example.test", security.Secret(testPassword), "", ""); err != nil {
		t.Fatalf("Login: %v", err)
	}

	stored := e.passwordHash(t, user.ID)
	if stored == weak {
		t.Fatal("hash lemah tidak ditulis ulang setelah login berhasil")
	}
	wantParams := "m=65536,t=3,p=2"
	if !strings.Contains(stored, wantParams) {
		t.Errorf("hash baru tidak memakai parameter terkini (%s)", wantParams)
	}

	// Password yang sama harus tetap berlaku setelah rehash.
	if _, err := e.svc.Login(e.ctx, "lama@example.test", security.Secret(testPassword), "", ""); err != nil {
		t.Errorf("login setelah rehash: %v", err)
	}

	// Hash yang sudah memakai parameter terkini tidak boleh ditulis ulang lagi.
	before := e.passwordHash(t, user.ID)
	if _, err := e.svc.Login(e.ctx, "lama@example.test", security.Secret(testPassword), "", ""); err != nil {
		t.Fatalf("Login: %v", err)
	}
	if after := e.passwordHash(t, user.ID); after != before {
		t.Error("hash yang sudah terkini ikut ditulis ulang")
	}
}

// Sesi harus bisa dibuat, ditemukan, dan dicabut — dan nilai mentah tokennya tidak boleh
// ada di database dalam bentuk apa pun.
func TestSessionLifecycleAndTokenStorage(t *testing.T) {
	e := newEnv(t)
	e.makeUser(t, "sesi@example.test", seed.RoleViewer)

	res := e.login(t, "sesi@example.test")
	raw := res.Token.Reveal()

	// Yang tersimpan harus hash SHA-256 dari token, bukan tokennya.
	sum := sha256.Sum256([]byte(raw))
	var storedHash string
	err := e.pool.QueryRow(e.ctx, `select token_hash from sessions where id = $1`,
		res.Principal.SessionID).Scan(&storedHash)
	if err != nil {
		t.Fatalf("membaca token_hash: %v", err)
	}
	if storedHash != hex.EncodeToString(sum[:]) {
		t.Errorf("token_hash tersimpan = %q, mau sha256 dari token", storedHash)
	}

	// Token mentah tidak boleh muncul di kolom teks mana pun di tabel sessions.
	var found bool
	err = e.pool.QueryRow(e.ctx, `
		select exists (
			select 1 from sessions
			where token_hash like '%' || $1 || '%'
			   or coalesce(user_agent, '') like '%' || $1 || '%'
		)`, raw).Scan(&found)
	if err != nil {
		t.Fatalf("query: %v", err)
	}
	if found {
		t.Fatal("token sesi mentah ditemukan tersimpan di database")
	}

	// Dicabut lewat Logout, lalu tidak boleh bisa dipakai lagi.
	if err := e.svc.Logout(e.ctx, res.Principal.SessionID, res.Principal.Actor(), "", ""); err != nil {
		t.Fatalf("Logout: %v", err)
	}
	if _, err := e.svc.Authenticate(e.ctx, res.Token); !errors.Is(err, ErrNoSession) {
		t.Errorf("Authenticate setelah logout: err = %v, mau ErrNoSession", err)
	}

	// Logout kedua atas sesi yang sama bukan kesalahan.
	if err := e.svc.Logout(e.ctx, res.Principal.SessionID, res.Principal.Actor(), "", ""); err != nil {
		t.Errorf("logout kedua: %v", err)
	}
}

// Seluruh sebab "tidak ada sesi" harus keluar sebagai ErrNoSession, tanpa membedakan yang
// pernah sah dari yang tidak pernah ada.
func TestAuthenticateRejections(t *testing.T) {
	e := newEnv(t)
	e.makeUser(t, "sesi@example.test", seed.RoleViewer)

	t.Run("token kosong", func(t *testing.T) {
		if _, err := e.svc.Authenticate(e.ctx, ""); !errors.Is(err, ErrNoSession) {
			t.Errorf("err = %v, mau ErrNoSession", err)
		}
	})

	t.Run("token tidak dikenal", func(t *testing.T) {
		if _, err := e.svc.Authenticate(e.ctx, security.Secret("token-yang-tidak-pernah-ada")); !errors.Is(err, ErrNoSession) {
			t.Errorf("err = %v, mau ErrNoSession", err)
		}
	})

	t.Run("sesi kedaluwarsa", func(t *testing.T) {
		res := e.login(t, "sesi@example.test")
		// created_at ikut dimundurkan: constraint sessions_expires_after_creation
		// mensyaratkan expires_at > created_at, jadi sesi kedaluwarsa hanya bisa dibentuk
		// dengan memundurkan keduanya.
		e.exec(t, `update sessions set created_at = now() - interval '2 hours',
			expires_at = now() - interval '1 second' where id = $1`, res.Principal.SessionID)
		if _, err := e.svc.Authenticate(e.ctx, res.Token); !errors.Is(err, ErrNoSession) {
			t.Errorf("err = %v, mau ErrNoSession", err)
		}
	})

	t.Run("akun dimatikan setelah sesi dibuat", func(t *testing.T) {
		user := e.makeUser(t, "nanti-mati@example.test", seed.RoleViewer)
		res := e.login(t, "nanti-mati@example.test")
		e.exec(t, `update users set status = 'disabled' where id = $1`, user.ID)

		if _, err := e.svc.Authenticate(e.ctx, res.Token); !errors.Is(err, ErrNoSession) {
			t.Fatalf("err = %v, mau ErrNoSession", err)
		}
		// Sesinya juga harus sudah dicabut, bukan hanya ditolak.
		var revoked *time.Time
		err := e.pool.QueryRow(e.ctx, `select revoked_at from sessions where id = $1`,
			res.Principal.SessionID).Scan(&revoked)
		if err != nil {
			t.Fatalf("membaca revoked_at: %v", err)
		}
		if revoked == nil {
			t.Error("sesi milik akun yang dimatikan tidak dicabut")
		}
	})

	// Penguncian sementara akibat penebakan password TIDAK boleh mengeluarkan sesi yang
	// sudah berjalan: kalau bisa, siapa pun yang tahu email seorang admin bisa
	// mengeluarkannya kapan saja hanya dengan menebak password lima kali.
	t.Run("penguncian sementara tidak memutus sesi berjalan", func(t *testing.T) {
		user := e.makeUser(t, "nanti-terkunci@example.test", seed.RoleViewer)
		res := e.login(t, "nanti-terkunci@example.test")
		e.exec(t, `update users set locked_until = now() + interval '10 minutes',
			failed_login_attempts = 5 where id = $1`, user.ID)

		if _, err := e.svc.Authenticate(e.ctx, res.Token); err != nil {
			t.Errorf("sesi berjalan seharusnya tetap sah: %v", err)
		}
	})
}

// Penguncian akun setelah ambang percobaan gagal harus terjadi, dan setelah terkunci bahkan
// password yang benar pun ditolak — dengan pesan yang tetap sama.
func TestLoginLockoutAfterThreshold(t *testing.T) {
	e := newEnv(t, WithLoginLockout(3, time.Minute))
	user := e.makeUser(t, "target@example.test", seed.RoleViewer)

	for i := 0; i < 3; i++ {
		_, err := e.svc.Login(e.ctx, "target@example.test", security.Secret("Salah-Sekali-2026"), "", "")
		var failure *LoginError
		if !errors.As(err, &failure) || failure.Reason != ReasonBadPassword {
			t.Fatalf("percobaan %d: err = %v", i+1, err)
		}
	}

	fresh, err := e.users.GetByID(e.ctx, user.ID)
	if err != nil {
		t.Fatalf("GetByID: %v", err)
	}
	if !fresh.Locked(time.Now()) {
		t.Fatalf("akun seharusnya terkunci setelah 3 kegagalan; attempts = %d, locked_until = %v",
			fresh.FailedLoginAttempts, fresh.LockedUntil)
	}

	// Password yang benar sekarang pun ditolak, dan pesannya tetap seragam.
	_, err = e.svc.Login(e.ctx, "target@example.test", security.Secret(testPassword), "", "")
	var failure *LoginError
	if !errors.As(err, &failure) {
		t.Fatalf("err = %v, mau *LoginError", err)
	}
	if failure.Reason != ReasonLocked {
		t.Errorf("Reason = %q, mau %q", failure.Reason, ReasonLocked)
	}
	if err.Error() != MessageLoginFailed {
		t.Errorf("pesan = %q, mau %q", err.Error(), MessageLoginFailed)
	}
}

// Ganti password harus mencabut sesi lain dan mempertahankan sesi yang dipakai.
func TestChangePasswordRevokesOtherSessions(t *testing.T) {
	e := newEnv(t)
	user := e.makeUser(t, "ganti@example.test", seed.RoleAdmin)

	current := e.login(t, "ganti@example.test")
	other := e.login(t, "ganti@example.test")
	third := e.login(t, "ganti@example.test")

	revoked, err := e.svc.ChangePassword(e.ctx, user.ID,
		security.Secret(testPassword), security.Secret(testNewPassword),
		current.Principal.SessionID, "203.0.113.9", "uji/1.0")
	if err != nil {
		t.Fatalf("ChangePassword: %v", err)
	}
	if revoked != 2 {
		t.Errorf("sesi tercabut = %d, mau 2", revoked)
	}

	if _, err := e.svc.Authenticate(e.ctx, current.Token); err != nil {
		t.Errorf("sesi yang dipakai seharusnya tetap hidup: %v", err)
	}
	for i, res := range []*Result{other, third} {
		if _, err := e.svc.Authenticate(e.ctx, res.Token); !errors.Is(err, ErrNoSession) {
			t.Errorf("sesi lain #%d: err = %v, mau ErrNoSession", i, err)
		}
	}

	// Password lama tidak boleh berlaku lagi, password baru harus berlaku.
	if _, err := e.svc.Login(e.ctx, "ganti@example.test", security.Secret(testPassword), "", ""); !errors.Is(err, ErrLoginFailed) {
		t.Errorf("password lama masih diterima: err = %v", err)
	}
	if _, err := e.svc.Login(e.ctx, "ganti@example.test", security.Secret(testNewPassword), "", ""); err != nil {
		t.Errorf("password baru ditolak: %v", err)
	}

	// Kewajiban ganti password ikut selesai.
	fresh, err := e.users.GetByID(e.ctx, user.ID)
	if err != nil {
		t.Fatalf("GetByID: %v", err)
	}
	if fresh.MustChangePassword {
		t.Error("must_change_password masih true setelah password diganti")
	}
}

// Tanpa sesi yang dikecualikan, seluruh sesi pengguna dicabut — bentuk yang dipakai reset
// password oleh admin lain.
func TestChangePasswordWithoutKeepRevokesAll(t *testing.T) {
	e := newEnv(t)
	user := e.makeUser(t, "reset@example.test", seed.RoleViewer)
	first := e.login(t, "reset@example.test")
	second := e.login(t, "reset@example.test")

	revoked, err := e.svc.ChangePassword(e.ctx, user.ID,
		security.Secret(testPassword), security.Secret(testNewPassword), "", "", "")
	if err != nil {
		t.Fatalf("ChangePassword: %v", err)
	}
	if revoked != 2 {
		t.Errorf("sesi tercabut = %d, mau 2", revoked)
	}
	for i, res := range []*Result{first, second} {
		if _, err := e.svc.Authenticate(e.ctx, res.Token); !errors.Is(err, ErrNoSession) {
			t.Errorf("sesi #%d: err = %v, mau ErrNoSession", i, err)
		}
	}
}

func TestChangePasswordRejections(t *testing.T) {
	e := newEnv(t)
	user := e.makeUser(t, "tolak@example.test", seed.RoleViewer)
	res := e.login(t, "tolak@example.test")

	cases := []struct {
		name    string
		current string
		next    string
		wantErr error
	}{
		{"password lama salah", "Bukan-Password-Saya-2026", testNewPassword, ErrWrongPassword},
		{"password baru terlalu pendek", testPassword, "Pendek1", ErrWeakPassword},
		{"password baru terlalu umum", testPassword, "changeme123", ErrWeakPassword},
		{"password baru tanpa variasi", testPassword, "aaaaaaaaaaaaaaa", ErrWeakPassword},
		{"password baru sama", testPassword, testPassword, ErrPasswordUnchanged},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			_, err := e.svc.ChangePassword(e.ctx, user.ID,
				security.Secret(tc.current), security.Secret(tc.next),
				res.Principal.SessionID, "", "")
			if !errors.Is(err, tc.wantErr) {
				t.Fatalf("err = %v, mau membungkus %v", err, tc.wantErr)
			}
			// Sesi yang dipakai tidak boleh tersentuh oleh permintaan yang ditolak.
			if _, err := e.svc.Authenticate(e.ctx, res.Token); err != nil {
				t.Errorf("sesi ikut tercabut padahal permintaan ditolak: %v", err)
			}
		})
	}

	// Aturan spesifik yang dilanggar tetap bisa dikenali, sehingga pesan ke pengguna bisa
	// menjelaskannya.
	_, err := e.svc.ChangePassword(e.ctx, user.ID,
		security.Secret(testPassword), security.Secret("Pendek1"), res.Principal.SessionID, "", "")
	if !errors.Is(err, security.ErrPasswordTooShort) {
		t.Errorf("err = %v, mau membungkus security.ErrPasswordTooShort", err)
	}
	if got := PasswordRuleMessage(err); got != security.ErrPasswordTooShort.Error() {
		t.Errorf("PasswordRuleMessage = %q, mau %q", got, security.ErrPasswordTooShort.Error())
	}

	// Pengguna yang tidak ada.
	_, err = e.svc.ChangePassword(e.ctx, "11111111-1111-1111-1111-111111111111",
		security.Secret(testPassword), security.Secret(testNewPassword), "", "", "")
	if !errors.Is(err, repo.ErrNotFound) {
		t.Errorf("err = %v, mau membungkus repo.ErrNotFound", err)
	}
}

// Audit harus memuat login berhasil, login gagal, dan logout — beserta pelakunya.
func TestAuditTrail(t *testing.T) {
	e := newEnv(t)
	user := e.makeUser(t, "audit@example.test", seed.RoleAdmin)

	if _, err := e.svc.Login(e.ctx, "audit@example.test", security.Secret("Salah-Lagi-2026"), "203.0.113.9", "uji/1.0"); err == nil {
		t.Fatal("login seharusnya gagal")
	}
	if _, err := e.svc.Login(e.ctx, "hantu@example.test", security.Secret(testPassword), "203.0.113.9", "uji/1.0"); err == nil {
		t.Fatal("login seharusnya gagal")
	}
	res := e.login(t, "audit@example.test")
	if err := e.svc.Logout(e.ctx, res.Principal.SessionID, res.Principal.Actor(), "203.0.113.9", "uji/1.0"); err != nil {
		t.Fatalf("Logout: %v", err)
	}
	if _, err := e.svc.ChangePassword(e.ctx, user.ID, security.Secret(testPassword),
		security.Secret(testNewPassword), "", "203.0.113.9", "uji/1.0"); err != nil {
		t.Fatalf("ChangePassword: %v", err)
	}

	want := map[string]int{
		identity.ActionLoginFailed:    2,
		identity.ActionLogin:          1,
		identity.ActionLogout:         1,
		identity.ActionPasswordChange: 1,
	}
	for action, n := range want {
		if got := e.countAudit(t, action); got != n {
			t.Errorf("audit %q = %d catatan, mau %d (semua: %v)", action, got, n, e.auditActions(t))
		}
	}

	// Login yang gagal untuk email tak terdaftar tetap menyimpan cuplikan emailnya, tanpa
	// pelaku, supaya penyapuan alamat bisa ditelusuri.
	var email string
	var actorID *string
	err := e.pool.QueryRow(e.ctx, `
		select coalesce(actor_email, ''), actor_user_id::text from audit_logs
		where action = $1 and actor_user_id is null order by id limit 1`,
		identity.ActionLoginFailed).Scan(&email, &actorID)
	if err != nil {
		t.Fatalf("membaca audit login gagal: %v", err)
	}
	if email != "hantu@example.test" || actorID != nil {
		t.Errorf("audit email tak terdaftar: email = %q, actor = %v", email, actorID)
	}

	// Sebab sebenarnya tercatat di metadata — tempat yang hanya bisa dibaca pemegang izin
	// audit:read, bukan di pesan ke klien.
	var reason string
	err = e.pool.QueryRow(e.ctx, `
		select metadata->>'reason' from audit_logs
		where action = $1 and actor_user_id is not null order by id limit 1`,
		identity.ActionLoginFailed).Scan(&reason)
	if err != nil {
		t.Fatalf("membaca metadata audit: %v", err)
	}
	if reason != string(ReasonBadPassword) {
		t.Errorf("metadata reason = %q, mau %q", reason, ReasonBadPassword)
	}
}

// last_seen_at tidak boleh disegarkan pada setiap request: satu UPDATE per request hanya
// untuk jejak "terakhir terlihat" tidak sebanding dengan baris mati dan WAL yang
// dihasilkannya.
func TestTouchOnlyWhenStale(t *testing.T) {
	e := newEnv(t, WithTouchInterval(30*time.Minute))
	e.makeUser(t, "jejak@example.test", seed.RoleViewer)
	res := e.login(t, "jejak@example.test")

	lastSeen := func() time.Time {
		var at time.Time
		err := e.pool.QueryRow(e.ctx, `select last_seen_at from sessions where id = $1`,
			res.Principal.SessionID).Scan(&at)
		if err != nil {
			t.Fatalf("membaca last_seen_at: %v", err)
		}
		return at
	}

	before := lastSeen()
	p, err := e.svc.Authenticate(e.ctx, res.Token)
	if err != nil {
		t.Fatalf("Authenticate: %v", err)
	}
	e.svc.touchIfStale(e.ctx, p)
	if after := lastSeen(); !after.Equal(before) {
		t.Errorf("last_seen_at berubah (%v -> %v) padahal jejaknya masih baru", before, after)
	}

	// Jejak yang sudah lebih tua dari ambang harus disegarkan.
	e.exec(t, `update sessions set last_seen_at = now() - interval '2 hours' where id = $1`,
		res.Principal.SessionID)
	stale := lastSeen()

	p, err = e.svc.Authenticate(e.ctx, res.Token)
	if err != nil {
		t.Fatalf("Authenticate: %v", err)
	}
	e.svc.touchIfStale(e.ctx, p)
	if after := lastSeen(); !after.After(stale) {
		t.Errorf("last_seen_at tidak disegarkan padahal jejaknya sudah tua (%v -> %v)", stale, after)
	}

	// Ambang nol mematikan pembaruan sepenuhnya.
	quiet := NewService(e.pool, e.cfg, nil, WithTouchInterval(0))
	e.exec(t, `update sessions set last_seen_at = now() - interval '2 hours' where id = $1`,
		res.Principal.SessionID)
	frozen := lastSeen()
	p, err = quiet.Authenticate(e.ctx, res.Token)
	if err != nil {
		t.Fatalf("Authenticate: %v", err)
	}
	quiet.touchIfStale(e.ctx, p)
	if after := lastSeen(); !after.Equal(frozen) {
		t.Errorf("last_seen_at berubah padahal pembaruan dimatikan (%v -> %v)", frozen, after)
	}
}

// Seluruh pengguna terautentikasi memiliki peran Admin dan izin penuh dalam arsitektur single-admin.
func TestLoginUserPrincipal(t *testing.T) {
	e := newEnv(t)
	e.makeUser(t, "admin-uji@example.test", "")

	res := e.login(t, "admin-uji@example.test")
	if res.Principal.Permissions == nil {
		t.Error("Permissions nil, mau slice non-nil")
	}
	if res.Principal.TopRole() != "Admin" {
		t.Errorf("TopRole() = %q, mau 'Admin'", res.Principal.TopRole())
	}
	if !res.Principal.Can(seed.PermHealthRead) {
		t.Error("admin terautentikasi seharusnya punya izin penuh")
	}
}

func TestSetupHint(t *testing.T) {
	e := newEnv(t)

	// Database baru yang telah di-seed memiliki admin default dengan MustChangePassword = true
	hint, err := e.svc.SetupHint(e.ctx)
	if err != nil {
		t.Fatalf("SetupHint: %v", err)
	}
	if !hint.HasDefaultAdmin {
		t.Error("HasDefaultAdmin = false padahal admin default ada dan must_change_password")
	}
	if hint.DefaultEmail != "admin@routex.local" {
		t.Errorf("DefaultEmail = %q", hint.DefaultEmail)
	}
	if hint.DefaultPassword != "RouteX#Initial2026!" {
		t.Errorf("DefaultPassword = %q", hint.DefaultPassword)
	}

	u, err := e.users.GetByEmail(e.ctx, "admin@routex.local")
	if err != nil {
		t.Fatalf("GetByEmail admin default: %v", err)
	}

	// Ganti password sehingga MustChangePassword = false
	if err := e.users.ChangePassword(e.ctx, u.ID, security.Secret("sandi-baru-yang-sangat-kuat-123"), false); err != nil {
		t.Fatalf("ChangePassword: %v", err)
	}

	hint, err = e.svc.SetupHint(e.ctx)
	if err != nil {
		t.Fatalf("SetupHint kedua: %v", err)
	}
	if hint.HasDefaultAdmin {
		t.Error("HasDefaultAdmin = true padahal password sudah diganti")
	}
}

// Hash tersimpan yang tidak bisa diparse adalah kerusakan data, bukan serangan: login
// gagal dengan pesan yang sama, tetapi penghitung kegagalan TIDAK dinaikkan — pemilik akun
// tidak melakukan apa pun yang salah dan menguncinya hanya menambah satu masalah lagi.
func TestLoginBrokenStoredHash(t *testing.T) {
	e := newEnv(t)
	user := e.makeUser(t, "rusak@example.test", seed.RoleViewer)
	e.exec(t, `update users set password_hash = 'bukan-hash-argon2' where id = $1`, user.ID)

	_, err := e.svc.Login(e.ctx, "rusak@example.test", security.Secret(testPassword), "", "")
	var failure *LoginError
	if !errors.As(err, &failure) {
		t.Fatalf("err = %v, mau *LoginError", err)
	}
	if failure.Reason != ReasonBrokenHash {
		t.Errorf("Reason = %q, mau %q", failure.Reason, ReasonBrokenHash)
	}
	if err.Error() != MessageLoginFailed {
		t.Errorf("pesan = %q, mau seragam %q", err.Error(), MessageLoginFailed)
	}

	fresh, err := e.users.GetByID(e.ctx, user.ID)
	if err != nil {
		t.Fatalf("GetByID: %v", err)
	}
	if fresh.FailedLoginAttempts != 0 || fresh.LockedUntil != nil {
		t.Errorf("penghitung kegagalan dinaikkan padahal hash yang rusak: attempts = %d, locked_until = %v",
			fresh.FailedLoginAttempts, fresh.LockedUntil)
	}
	// Tetap tercatat di audit: ini keadaan yang harus terlihat operator.
	if got := e.countAudit(t, identity.ActionLoginFailed); got != 1 {
		t.Errorf("catatan audit = %d, mau 1", got)
	}
}

// Opsi konstruktor adalah bagian dari kontrak yang dipakai router, jadi nilainya dikunci di
// sini — termasuk perilaku nilai yang tidak masuk akal.
func TestServiceOptions(t *testing.T) {
	svc := NewService(nil, nil, nil)
	if svc.SessionTTL() != DefaultSessionTTL {
		t.Errorf("TTL bawaan = %v, mau %v", svc.SessionTTL(), DefaultSessionTTL)
	}
	if svc.TouchInterval() != DefaultTouchInterval {
		t.Errorf("selang touch bawaan = %v, mau %v", svc.TouchInterval(), DefaultTouchInterval)
	}
	if svc.Cookies() == nil || svc.CSRF() == nil {
		t.Fatal("Cookies() atau CSRF() nil")
	}

	svc = NewService(nil, nil, nil,
		WithSessionTTL(2*time.Hour),
		WithTouchInterval(time.Minute),
		WithLoginLockout(7, 3*time.Minute),
		nil, // opsi nil harus dilewati, bukan memicu panic
	)
	if svc.SessionTTL() != 2*time.Hour {
		t.Errorf("TTL = %v", svc.SessionTTL())
	}
	if svc.TouchInterval() != time.Minute {
		t.Errorf("selang touch = %v", svc.TouchInterval())
	}
	if svc.lockThreshold != 7 || svc.lockDuration != 3*time.Minute {
		t.Errorf("penguncian = %d percobaan / %v", svc.lockThreshold, svc.lockDuration)
	}
	// Max-Age cookie harus mengikuti TTL yang sebenarnya dipakai, bukan bawaan.
	if svc.Cookies().TTL() != 2*time.Hour {
		t.Errorf("TTL cookie = %v, mau mengikuti TTL sesi", svc.Cookies().TTL())
	}

	// Nilai tak masuk akal diabaikan alih-alih menghasilkan sesi tanpa masa berlaku.
	svc = NewService(nil, nil, nil, WithSessionTTL(0), WithSessionTTL(-time.Hour), WithTouchInterval(-time.Second))
	if svc.SessionTTL() != DefaultSessionTTL || svc.TouchInterval() != DefaultTouchInterval {
		t.Errorf("TTL = %v, selang touch = %v", svc.SessionTTL(), svc.TouchInterval())
	}
}

func TestPasswordRuleMessage(t *testing.T) {
	cases := map[error]string{
		security.ErrPasswordTooShort:  security.ErrPasswordTooShort.Error(),
		security.ErrPasswordTooLong:   security.ErrPasswordTooLong.Error(),
		security.ErrPasswordCommon:    security.ErrPasswordCommon.Error(),
		security.ErrPasswordNoVariety: security.ErrPasswordNoVariety.Error(),
	}
	for err, want := range cases {
		if got := PasswordRuleMessage(weakPasswordError("uji", err)); got != want {
			t.Errorf("PasswordRuleMessage(%v) = %q, mau %q", err, got, want)
		}
	}
	// Aturan yang tidak dikenali jatuh ke pesan generik: menempelkan err.Error() apa adanya
	// akan ikut membawa awalan operasi internal ke respons API.
	if got := PasswordRuleMessage(errors.New("sesuatu yang lain")); got != ErrWeakPassword.Error() {
		t.Errorf("pesan generik = %q, mau %q", got, ErrWeakPassword.Error())
	}
}

// Email raksasa dari body request tidak boleh masuk utuh ke log maupun ke kolom audit, dan
// pemotongannya tidak boleh menghasilkan UTF-8 yang tidak sah — PostgreSQL akan menolak
// seluruh INSERT audit karenanya.
func TestTruncateEmail(t *testing.T) {
	if got := truncateEmail("a@b.test"); got != "a@b.test" {
		t.Errorf("email pendek berubah menjadi %q", got)
	}

	long := strings.Repeat("a", maxEmailLen+50) + "@example.test"
	if got := truncateEmail(long); len(got) > maxEmailLen {
		t.Errorf("panjang hasil = %d, mau ≤ %d", len(got), maxEmailLen)
	}

	// Rune multibyte tepat di batas pemotongan.
	multibyte := strings.Repeat("ä", maxEmailLen)
	got := truncateEmail(multibyte)
	if len(got) > maxEmailLen {
		t.Errorf("panjang hasil = %d, mau ≤ %d", len(got), maxEmailLen)
	}
	if !utf8.ValidString(got) {
		t.Error("hasil pemotongan bukan UTF-8 yang sah")
	}
}

// Login dengan email raksasa harus ditolak tanpa menyentuh database, tetapi tetap tercatat
// dan tetap membayar waktu argon2.
func TestLoginRejectsOversizedEmail(t *testing.T) {
	e := newEnv(t)
	long := strings.Repeat("a", maxEmailLen+10) + "@example.test"

	_, err := e.svc.Login(e.ctx, long, security.Secret(testPassword), "", "")
	var failure *LoginError
	if !errors.As(err, &failure) || failure.Reason != ReasonUnknownEmail {
		t.Fatalf("err = %v, mau *LoginError dengan ReasonUnknownEmail", err)
	}

	var stored string
	row := e.pool.QueryRow(e.ctx, `select coalesce(actor_email, '') from audit_logs
		where action = $1 order by id limit 1`, identity.ActionLoginFailed)
	if err := row.Scan(&stored); err != nil {
		t.Fatalf("membaca audit: %v", err)
	}
	if len(stored) > maxEmailLen {
		t.Errorf("email di audit sepanjang %d byte, mau ≤ %d", len(stored), maxEmailLen)
	}
}
