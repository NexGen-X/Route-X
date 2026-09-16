package identity

import (
	"context"
	"crypto/rand"
	"encoding/base64"
	"encoding/hex"
	"errors"
	"fmt"
	"log/slog"
	"net/url"
	"os"
	"strings"
	"testing"
	"time"
	"unicode/utf8"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"

	"github.com/NexGen-X/Route-X/internal/config"
	"github.com/NexGen-X/Route-X/internal/database"
	"github.com/NexGen-X/Route-X/internal/database/repo"
	"github.com/NexGen-X/Route-X/internal/security"
)

// testPassword sengaja ganjil supaya kemunculannya di sebuah pesan error tidak mungkin
// kebetulan. Bentuknya tetap lolos security.ValidatePasswordStrength.
const testPassword = "Kata-Sandi-Uji-Xyzzy-2026"

// testSchemaPrefix menandai schema sekali pakai milik paket ini.
//
// Awalannya khas per paket supaya pemeriksaan sisa schema di TestMain tidak salah
// menuduh test paket lain yang mungkin berjalan bersamaan di database yang sama.
const testSchemaPrefix = "test_ident_"

// TestMain menjalankan test lalu memastikan tidak ada schema sekali pakai yang
// tertinggal.
//
// Pemeriksaannya di sini, bukan di sebuah test biasa, karena hanya di titik ini seluruh
// t.Cleanup sudah dijalankan — sebuah test yang memeriksanya akan menghitung schema
// milik test yang belum selesai membersihkan diri.
func TestMain(m *testing.M) {
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
		// Database tidak bisa dihubungi berarti test integrasinya juga terlewat, jadi
		// tidak mungkin ada schema yang tertinggal.
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

// envDSN mengembalikan DSN test dari environment, "" bila tidak ada.
func envDSN() string {
	for _, key := range []string{"TEST_DATABASE_URL", "DATABASE_URL"} {
		if dsn := strings.TrimSpace(os.Getenv(key)); dsn != "" {
			return dsn
		}
	}
	return ""
}

// discardLogger membuang seluruh log supaya keluaran test tetap bersih.
func discardLogger() *slog.Logger { return slog.New(slog.DiscardHandler) }

// testDSN mengembalikan DSN database untuk test integrasi, atau melewati test kalau
// tidak ada yang bisa dipakai.
func testDSN(t *testing.T) string {
	t.Helper()
	dsn := envDSN()
	if dsn == "" {
		t.Skip("TEST_DATABASE_URL maupun DATABASE_URL tidak diset — test integrasi dilewati")
	}
	if !strings.HasPrefix(dsn, "postgres://") && !strings.HasPrefix(dsn, "postgresql://") {
		t.Skip("DSN test bukan URL postgres:// sehingga search_path test tidak bisa dipasang")
	}
	return dsn
}

// newTestDB menyiapkan database untuk satu test: schema kosong bernama unik, migrasi
// lengkap di dalamnya, dan penghapusan schema itu saat test selesai.
//
// search_path dikirim sebagai query parameter DSN karena pgx meneruskan parameter tak
// dikenal sebagai runtime parameter saat koneksi dibuka, sehingga setiap koneksi yang
// dibuat pool ikut terarahkan ke schema itu. Dengan begitu tidak ada satu pun tabel
// test yang menyentuh schema public database development.
func newTestDB(t *testing.T) (context.Context, *database.DB) {
	t.Helper()
	if testing.Short() {
		t.Skip("butuh PostgreSQL")
	}
	baseDSN := testDSN(t)

	ctx, cancel := context.WithTimeout(context.Background(), 120*time.Second)
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
		// Context baru: ctx test bisa sudah kedaluwarsa saat pembersihan jalan, dan
		// schema yang tertinggal akan mengotori database development.
		cleanupCtx, cancelCleanup := context.WithTimeout(context.Background(), 30*time.Second)
		defer cancelCleanup()
		if _, err := admin.Exec(cleanupCtx, "drop schema "+ident+" cascade"); err != nil {
			t.Errorf("membersihkan schema %s: %v", schema, err)
		}
	})

	db, err := database.Connect(ctx, testConfig(withSearchPath(t, baseDSN, schema)), discardLogger())
	if err != nil {
		t.Fatalf("Connect: %v", err)
	}
	t.Cleanup(db.Close)

	if _, err := database.Migrate(ctx, db, discardLogger()); err != nil {
		t.Fatalf("Migrate: %v", err)
	}
	return ctx, db
}

// testConfig merakit config minimal yang dibutuhkan database.Connect.
func testConfig(dsn string) *config.Config {
	return &config.Config{
		AppEnv:      config.EnvDevelopment,
		DatabaseURL: security.Secret(dsn),
		DBMaxConns:  8,
		DBMinConns:  0,
	}
}

// withSearchPath menambahkan parameter search_path ke sebuah DSN.
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

// base64Raw mengemas isi cursor apa adanya, tanpa lewat encodeCursor, supaya bentuk
// cursor yang salah bisa diuji.
func base64Raw(s string) string { return base64.RawURLEncoding.EncodeToString([]byte(s)) }

// randomHex menghasilkan n byte acak sebagai heksadesimal.
func randomHex(t *testing.T, n int) string {
	t.Helper()
	buf := make([]byte, n)
	if _, err := rand.Read(buf); err != nil {
		t.Fatalf("membuat nilai acak: %v", err)
	}
	return hex.EncodeToString(buf)
}

// --- Perkakas fixture --------------------------------------------------------

// makeUser membuat pengguna dengan password test dan menghentikan test bila gagal.
func makeUser(ctx context.Context, t *testing.T, r *Users, email string) User {
	t.Helper()
	u, err := r.Create(ctx, NewUser{Email: email, Password: testPassword, DisplayName: "Uji " + email})
	if err != nil {
		t.Fatalf("Create(%q): %v", email, err)
	}
	return u
}

// --- Test tanpa database -----------------------------------------------------

// Cursor harus pulang persis seperti saat dikirim: kesalahan sekecil satu mikrodetik
// membuat halaman berikutnya melewatkan atau mengulang satu baris.
func TestCursorRoundTrip(t *testing.T) {
	cases := map[string]struct {
		at time.Time
		id string
	}{
		"uuid":            {time.Date(2026, 9, 2, 10, 30, 0, 123456000, time.UTC), "8b1f4a2c-0d3e-4f5a-9b8c-7d6e5f4a3b2c"},
		"id bilangan":     {time.Date(2026, 1, 1, 0, 0, 0, 0, time.UTC), "918273645"},
		"presisi mikro":   {time.Date(2026, 9, 2, 10, 30, 0, 999999000, time.UTC), "1"},
		"sebelum epoch":   {time.Date(1969, 7, 20, 20, 17, 40, 0, time.UTC), "42"},
		"zona non-UTC":    {time.Date(2026, 9, 2, 17, 30, 0, 0, time.FixedZone("WIB", 7*3600)), "7"},
		"presisi nanodet": {time.Date(2026, 9, 2, 10, 30, 0, 123456789, time.UTC), "9"},
	}

	for name, tc := range cases {
		t.Run(name, func(t *testing.T) {
			gotAt, gotID, err := decodeCursor(encodeCursor(tc.at, tc.id))
			if err != nil {
				t.Fatalf("decodeCursor: %v", err)
			}
			if !gotAt.Equal(tc.at) {
				t.Errorf("waktu = %v, ingin %v", gotAt, tc.at)
			}
			if gotID != tc.id {
				t.Errorf("id = %q, ingin %q", gotID, tc.id)
			}
		})
	}
}

// Cursor rusak harus dilaporkan, bukan diam-diam dianggap halaman pertama: pemanggil
// yang tidak diberi tahu akan mengulang halaman pertama tanpa batas.
func TestDecodeCursorRejectsGarbage(t *testing.T) {
	cases := map[string]string{
		"bukan base64":       "!!!bukan base64!!!",
		"tanpa pemisah":      base64Raw("1756800000000000000"),
		"tanpa id":           base64Raw("1756800000000000000|"),
		"tanpa waktu":        base64Raw("|8b1f4a2c"),
		"waktu bukan angka":  base64Raw("kemarin|8b1f4a2c"),
		"kosong sama sekali": base64Raw("|"),
	}

	for name, cursor := range cases {
		t.Run(name, func(t *testing.T) {
			if _, _, err := decodeCursor(cursor); !errors.Is(err, ErrInvalidCursor) {
				t.Errorf("decodeCursor(%q) = %v, ingin ErrInvalidCursor", cursor, err)
			}
		})
	}
}

func TestValidUUID(t *testing.T) {
	valid := []string{
		"8b1f4a2c-0d3e-4f5a-9b8c-7d6e5f4a3b2c",
		"8B1F4A2C-0D3E-4F5A-9B8C-7D6E5F4A3B2C",
		"00000000-0000-0000-0000-000000000000",
	}
	for _, s := range valid {
		if !validUUID(s) {
			t.Errorf("validUUID(%q) = false, ingin true", s)
		}
	}

	invalid := []string{
		"",
		"8b1f4a2c0d3e4f5a9b8c7d6e5f4a3b2c",                        // tanpa tanda hubung
		"{8b1f4a2c-0d3e-4f5a-9b8c-7d6e5f4a3b2c}",                  // berkurung kurawal
		"8b1f4a2c-0d3e-4f5a-9b8c-7d6e5f4a3b2",                     // terlalu pendek
		"8b1f4a2c-0d3e-4f5a-9b8c-7d6e5f4a3b2cc",                   // terlalu panjang
		"8b1f4a2c-0d3e-4f5a-9b8c-7d6e5f4a3b2g",                    // bukan heksadesimal
		"8b1f4a2c_0d3e_4f5a_9b8c_7d6e5f4a3b2c",                    // pemisah salah
		"'; drop table users; --                             xxx", // panjangnya pas 36
	}
	for _, s := range invalid {
		if validUUID(s) {
			t.Errorf("validUUID(%q) = true, ingin false", s)
		}
	}
}

// Pemotongan tidak boleh meninggalkan rune separuh: nilai seperti itu akan ditolak
// PostgreSQL sebagai byte UTF-8 tidak sah dan menggagalkan seluruh penulisan.
func TestTruncate(t *testing.T) {
	cases := []struct {
		name  string
		in    string
		limit int
		want  string
	}{
		{"lebih pendek dari batas", "Mozilla/5.0", 512, "Mozilla/5.0"},
		{"tepat di batas", "abcde", 5, "abcde"},
		{"dipotong di batas ASCII", "abcdefghij", 4, "abcd"},
		{"rune multibyte tidak terbelah", "aä", 2, "a"},
		{"seluruhnya multibyte", "äöü", 4, "äö"},
		{"batas nol", "apa pun", 0, ""},
		{"kosong", "", 8, ""},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			if got := truncate(tc.in, tc.limit); got != tc.want {
				t.Errorf("truncate(%q, %d) = %q, ingin %q", tc.in, tc.limit, got, tc.want)
			}
			if !utf8.ValidString(truncate(tc.in, tc.limit)) {
				t.Errorf("truncate(%q, %d) menghasilkan UTF-8 tidak sah", tc.in, tc.limit)
			}
		})
	}
}

// Alamat yang tidak bisa diurai menjadi NULL, bukan error: catatan login tidak boleh
// gagal hanya karena header proxy berisi sampah.
func TestIPParam(t *testing.T) {
	cases := map[string]any{
		"127.0.0.1":                  "127.0.0.1",
		" 10.1.2.3 ":                 "10.1.2.3",
		"::1":                        "::1",
		"::ffff:127.0.0.1":           "127.0.0.1",
		"2001:db8::1":                "2001:db8::1",
		"":                           nil,
		"bukan-alamat":               nil,
		"127.0.0.1:8080":             nil,
		"'; drop table sessions; --": nil,
		"10.1.2.3, 10.1.2.4":         nil,
	}

	for in, want := range cases {
		if got := ipParam(in); got != want {
			t.Errorf("ipParam(%q) = %v, ingin %v", in, got, want)
		}
	}
}

func TestSecondsClampsNegative(t *testing.T) {
	if got := seconds(-time.Hour); got != 0 {
		t.Errorf("seconds(-1h) = %v, ingin 0", got)
	}
	if got := seconds(90 * time.Second); got != 90 {
		t.Errorf("seconds(90s) = %v, ingin 90", got)
	}
}

// Nilai tidak boleh pernah masuk ke dalam string SQL; yang masuk hanya placeholder.
func TestClauseListKeepsValuesOutOfSQL(t *testing.T) {
	c := &clauseList{}
	c.add("status = %s", "active")
	c.add("(created_at, id) < (%s::timestamptz, %s::uuid)", time.Unix(0, 0), "8b1f4a2c")

	wantWhere := " where status = $1 and (created_at, id) < ($2::timestamptz, $3::uuid)"
	if got := c.where(); got != wantWhere {
		t.Errorf("where() = %q, ingin %q", got, wantWhere)
	}
	if got := c.placeholder(51); got != "$4" {
		t.Errorf("placeholder() = %q, ingin $4", got)
	}
	if len(c.args) != 4 {
		t.Fatalf("jumlah argumen = %d, ingin 4", len(c.args))
	}
	if c.args[0] != "active" || c.args[3] != 51 {
		t.Errorf("argumen tidak sesuai urutan penambahan: %v", c.args)
	}

	empty := &clauseList{}
	if !empty.empty() || empty.where() != "" {
		t.Error("clauseList kosong harus menghasilkan klausa WHERE kosong")
	}
}

func TestUserStatusValid(t *testing.T) {
	for _, s := range []UserStatus{UserStatusActive, UserStatusDisabled, UserStatusLocked} {
		if !s.Valid() {
			t.Errorf("UserStatus(%q).Valid() = false", s)
		}
	}
	for _, s := range []UserStatus{"", "aktif", "ACTIVE", "deleted"} {
		if s.Valid() {
			t.Errorf("UserStatus(%q).Valid() = true", s)
		}
	}
}

func TestUserLockedAndCanLogin(t *testing.T) {
	now := time.Date(2026, 9, 2, 12, 0, 0, 0, time.UTC)
	future := now.Add(time.Minute)
	past := now.Add(-time.Minute)

	cases := []struct {
		name             string
		user             User
		locked, canLogin bool
	}{
		{"aktif tanpa kunci", User{Status: UserStatusActive}, false, true},
		{"aktif tapi terkunci", User{Status: UserStatusActive, LockedUntil: &future}, true, false},
		{"kunci sudah lewat", User{Status: UserStatusActive, LockedUntil: &past}, false, true},
		{"dinonaktifkan", User{Status: UserStatusDisabled}, false, false},
		{"dikunci admin", User{Status: UserStatusLocked}, false, false},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			if got := tc.user.Locked(now); got != tc.locked {
				t.Errorf("Locked() = %v, ingin %v", got, tc.locked)
			}
			if got := tc.user.CanLogin(now); got != tc.canLogin {
				t.Errorf("CanLogin() = %v, ingin %v", got, tc.canLogin)
			}
		})
	}
}

// Token sesi harus acak dan hash-nya harus deterministik: yang pertama membuat token
// tidak bisa ditebak, yang kedua membuat sesi bisa dicari lewat indeks.
func TestNewSessionToken(t *testing.T) {
	const samples = 32
	seen := make(map[string]struct{}, samples)

	for range samples {
		raw, hash, err := newSessionToken()
		if err != nil {
			t.Fatalf("newSessionToken: %v", err)
		}
		token := raw.Reveal()

		if len(token) != 43 {
			t.Fatalf("panjang token = %d, ingin 43 karakter base64url dari 32 byte", len(token))
		}
		if strings.ContainsAny(token, "+/=") {
			t.Errorf("token memuat karakter yang tidak aman untuk cookie")
		}
		if len(hash) != 64 {
			t.Errorf("panjang hash = %d, ingin 64 karakter heksadesimal", len(hash))
		}
		if strings.Contains(hash, token) {
			t.Error("hash memuat tokennya sendiri")
		}
		if hash != hashSessionToken(token) {
			t.Error("hashSessionToken tidak deterministik untuk token yang sama")
		}
		if _, dup := seen[token]; dup {
			t.Fatal("token yang sama dihasilkan dua kali")
		}
		seen[token] = struct{}{}
	}

	// Nilai tetap, dihitung di luar Go dengan sha256sum, supaya algoritma dan
	// formatnya benar-benar terkunci: hash inilah yang tersimpan di database, jadi
	// mengubah cara menghitungnya akan membuat seluruh sesi yang sedang berjalan tidak
	// bisa ditemukan lagi.
	const (
		token = "route-x"
		want  = "040d8acd31f5861350a35de0a5b83ac2a81171af2357fd5cf47a72277e2090e6"
	)
	if got := hashSessionToken(token); got != want {
		t.Errorf("hashSessionToken(%q) = %q, ingin %q", token, got, want)
	}
}

// Metadata kosong harus menjadi objek JSON kosong, bukan NULL, dan nilai yang tidak
// bisa diserialisasi harus dilaporkan tanpa ikut membawa isinya ke pesan error.
func TestMarshalMetadata(t *testing.T) {
	got, err := marshalMetadata(nil)
	if err != nil {
		t.Fatalf("marshalMetadata(nil): %v", err)
	}
	if string(got) != "{}" {
		t.Errorf("marshalMetadata(nil) = %q, ingin {}", got)
	}

	got, err = marshalMetadata(map[string]any{"provider": "openai", "attempts": 3})
	if err != nil {
		t.Fatalf("marshalMetadata: %v", err)
	}
	if !strings.Contains(string(got), `"provider":"openai"`) {
		t.Errorf("hasil = %q, ingin memuat provider", got)
	}

	// Rahasia yang dibungkus security.Secret harus keluar tersamar tanpa usaha khusus
	// dari pemanggil.
	got, err = marshalMetadata(map[string]any{"token": security.Secret(testPassword)})
	if err != nil {
		t.Fatalf("marshalMetadata: %v", err)
	}
	if strings.Contains(string(got), testPassword) {
		t.Error("security.Secret di dalam metadata tidak tersamar")
	}

	if _, err := marshalMetadata(map[string]any{"kanal": make(chan int)}); !errors.Is(err, ErrInvalidInput) {
		t.Errorf("marshalMetadata dengan nilai tak terserialisasi = %v, ingin ErrInvalidInput", err)
	}
}

// --- Test integrasi lintas repository ----------------------------------------

// Terjemahan error harus utuh sampai ke pemanggil: lapisan HTTP memilih kode status
// hanya berdasarkan sentinel repo, jadi satu pelanggaran yang tidak terpetakan akan
// muncul sebagai 500 untuk permintaan yang sebenarnya salah dari sisi klien.
func TestErrorTranslationIntegration(t *testing.T) {
	ctx, db := newTestDB(t)
	users := NewUsers(db.Pool)
	sessions := NewSessions(db.Pool)
	audit := NewAudit(db.Pool)
	settings := NewSettings(db.Pool)

	makeUser(ctx, t, users, "terjemahan@routex.test")

	const missing = "8b1f4a2c-0d3e-4f5a-9b8c-7d6e5f4a3b2c"

	cases := []struct {
		name string
		// constraint, bila diisi, harus muncul di pesan supaya lapisan HTTP bisa
		// memilih kalimat yang tepat untuk pengguna.
		constraint string
		want       error
		run        func() error
	}{
		{
			name:       "keunikan email",
			constraint: "users_email_lower_key",
			want:       repo.ErrConflict,
			run: func() error {
				_, err := users.Create(ctx, NewUser{Email: "TERJEMAHAN@routex.test", Password: testPassword})
				return err
			},
		},
		{
			name: "sesi untuk pengguna yang tidak ada",
			want: repo.ErrInvalidReference,
			run: func() error {
				_, err := sessions.Create(ctx, NewSession{UserID: missing, TTL: time.Hour})
				return err
			},
		},
		{
			name: "pelaku audit yang tidak ada",
			want: repo.ErrInvalidReference,
			run: func() error {
				return audit.Write(ctx, Event{
					Actor:        Actor{UserID: missing},
					Action:       ActionLogin,
					ResourceType: ResourceSession,
				})
			},
		},
		{
			name: "pengguna yang tidak ada",
			want: repo.ErrNotFound,
			run:  func() error { _, err := users.GetByID(ctx, missing); return err },
		},
		{
			name: "setelan yang tidak ada",
			want: repo.ErrNotFound,
			run:  func() error { return settings.Delete(ctx, "tidak.ada") },
		},
		{
			name:       "email kosong",
			constraint: "users_email_not_empty",
			want:       repo.ErrConstraint,
			run: func() error {
				_, err := users.Create(ctx, NewUser{Email: "   ", Password: testPassword})
				return err
			},
		},
		{
			name:       "kunci setelan kosong",
			constraint: "settings_key_not_empty",
			want:       repo.ErrConstraint,
			run: func() error {
				_, err := settings.Put(ctx, SettingWrite{Key: " ", Value: []byte(`1`)})
				return err
			},
		},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			err := tc.run()
			if !errors.Is(err, tc.want) {
				t.Fatalf("err = %v, ingin %v", err, tc.want)
			}
			if tc.constraint != "" && !strings.Contains(err.Error(), tc.constraint) {
				t.Errorf("pesan tidak menyebut constraint %q: %v", tc.constraint, err)
			}
		})
	}
}

// Pesan error tidak boleh membawa nilai baris.
//
// pgx menempelkan detail barisnya ke pesan error — untuk pelanggaran keunikan, itu
// termasuk nilai kolom yang bertabrakan. Di tabel ini nilai seperti itu berarti hash
// password dan hash token sesi, dan pesan error adalah tempat yang paling mudah bocor:
// ia masuk ke log, ke pelacak error, dan kadang ke respons API.
func TestErrorMessagesDoNotLeakSecretsIntegration(t *testing.T) {
	ctx, db := newTestDB(t)
	users := NewUsers(db.Pool)
	sessions := NewSessions(db.Pool)
	settings := NewSettings(db.Pool)

	const email = "rahasia@routex.test"
	user := makeUser(ctx, t, users, email)
	hash := user.PasswordHash.Reveal()

	session, err := sessions.Create(ctx, NewSession{UserID: user.ID, TTL: time.Hour})
	if err != nil {
		t.Fatalf("Create: %v", err)
	}
	token := session.Token.Reveal()
	if err := sessions.Revoke(ctx, session.ID); err != nil {
		t.Fatalf("Revoke: %v", err)
	}

	secrets := map[string]string{
		"password":   testPassword,
		"hash":       hash,
		"token sesi": token,
		"hash token": hashSessionToken(token),
		"email":      email,
	}

	newEmail := "RAHASIA@routex.test"
	failures := map[string]error{}
	add := func(name string, err error) {
		if err == nil {
			t.Errorf("%s: ingin error, dapat nil", name)
			return
		}
		failures[name] = err
	}

	_, err = users.Create(ctx, NewUser{Email: newEmail, Password: testPassword})
	add("Create dengan email yang sudah dipakai", err)

	other := makeUser(ctx, t, users, "lain@routex.test")
	_, err = users.UpdateProfile(ctx, other.ID, ProfileUpdate{Email: &newEmail})
	add("UpdateProfile ke email yang sudah dipakai", err)

	_, err = users.Create(ctx, NewUser{Email: "lemah@routex.test", Password: security.Secret(testPassword[:6])})
	add("Create dengan password terlalu pendek", err)

	err = users.ChangePassword(ctx, "8b1f4a2c-0d3e-4f5a-9b8c-7d6e5f4a3b2c", testPassword, false)
	add("ChangePassword untuk pengguna yang tidak ada", err)

	err = users.UpdatePasswordHash(ctx, "8b1f4a2c-0d3e-4f5a-9b8c-7d6e5f4a3b2c", security.Secret(hash))
	add("UpdatePasswordHash untuk pengguna yang tidak ada", err)

	_, err = sessions.Lookup(ctx, session.Token)
	add("Lookup sesi yang sudah dicabut", err)

	_, err = sessions.Create(ctx, NewSession{UserID: "8b1f4a2c-0d3e-4f5a-9b8c-7d6e5f4a3b2c", TTL: time.Hour})
	add("Create sesi untuk pengguna yang tidak ada", err)

	_, err = settings.Put(ctx, SettingWrite{Key: "uji.rusak", Value: []byte(`{"password":"` + testPassword)})
	add("Put setelan dengan JSON rusak", err)

	for name, err := range failures {
		msg := err.Error()
		for what, secret := range secrets {
			if strings.Contains(msg, secret) {
				// Pesannya sendiri tidak dicetak: kalau assertion ini gagal, isi pesan
				// itu justru yang tidak boleh masuk keluaran test.
				t.Errorf("%s: pesan error memuat %s (panjang pesan %d karakter)", name, what, len(msg))
			}
		}
	}
}

// Repository menyimpan repo.Querier, bukan pool konkret, supaya beberapa domain bisa
// diubah dalam satu transaksi tanpa method duplikat. Test ini yang memastikan janji itu
// benar-benar berlaku, termasuk pembatalannya.
func TestRepositoriesShareTransactionIntegration(t *testing.T) {
	ctx, db := newTestDB(t)
	users := NewUsers(db.Pool)
	audit := NewAudit(db.Pool)

	var userID string
	err := repo.InTx(ctx, db.Pool, func(q repo.Querier) error {
		created, err := NewUsers(q).Create(ctx, NewUser{Email: "tx@routex.test", Password: testPassword})
		if err != nil {
			return err
		}
		userID = created.ID
		return NewAudit(q).Write(ctx, Event{
			Actor:        ActorOf(created, "Admin"),
			Action:       ActionLogin,
			ResourceType: ResourceSession,
		})
	})
	if err != nil {
		t.Fatalf("InTx: %v", err)
	}

	if _, err := users.GetByID(ctx, userID); err != nil {
		t.Errorf("pengguna tidak tersimpan: %v", err)
	}
	page, err := audit.List(ctx, AuditFilter{ActorUserID: userID}, repo.Page{})
	if err != nil || len(page.Entries) != 1 {
		t.Errorf("catatan audit tidak tersimpan: %v (%d catatan)", err, len(page.Entries))
	}

	t.Run("transaksi yang gagal tidak menyisakan apa pun", func(t *testing.T) {
		sentinel := errors.New("dibatalkan sengaja")
		err := repo.InTx(ctx, db.Pool, func(q repo.Querier) error {
			if _, err := NewUsers(q).Create(ctx, NewUser{Email: "batal@routex.test", Password: testPassword}); err != nil {
				return err
			}
			return sentinel
		})
		if !errors.Is(err, sentinel) {
			t.Fatalf("InTx = %v, ingin error dari fn diteruskan", err)
		}
		if _, err := users.GetByEmail(ctx, "batal@routex.test"); !errors.Is(err, repo.ErrNotFound) {
			t.Errorf("GetByEmail = %v, ingin ErrNotFound karena transaksinya dibatalkan", err)
		}
	})
}

// Test integrasi hanya boleh bekerja di dalam schema sekali pakainya.
//
// Kalau isolasinya bocor, testnya sendiri tetap lulus — yang rusak adalah database
// development orang lain, dan itu baru terasa jauh kemudian. Karena itu isolasinya
// diperiksa langsung, bukan diandalkan.
func TestSchemaIsolationIntegration(t *testing.T) {
	ctx, db := newTestDB(t)

	var schema string
	if err := db.Pool.QueryRow(ctx, `select current_schema()`).Scan(&schema); err != nil {
		t.Fatalf("membaca current_schema(): %v", err)
	}
	if !strings.HasPrefix(schema, testSchemaPrefix) {
		t.Fatalf("current_schema() = %q, ingin schema sekali pakai berawalan %q", schema, testSchemaPrefix)
	}

	for _, table := range []string{
		"users", "sessions", "audit_logs", "settings", "schema_migrations",
	} {
		var n int
		if err := db.Pool.QueryRow(ctx,
			`select count(*) from information_schema.tables where table_schema = $1 and table_name = $2`,
			schema, table).Scan(&n); err != nil {
			t.Fatalf("membaca information_schema: %v", err)
		}
		if n != 1 {
			t.Errorf("tabel %q tidak ada di schema %s", table, schema)
		}
	}

	// Baris yang ditulis test tidak boleh terlihat dari schema public.
	email := "isolasi-" + randomHex(t, 6) + "@routex.test"
	makeUser(ctx, t, NewUsers(db.Pool), email)

	var publicUsers int
	if err := db.Pool.QueryRow(ctx,
		`select count(*) from information_schema.tables where table_schema = 'public' and table_name = 'users'`).
		Scan(&publicUsers); err != nil {
		t.Fatalf("memeriksa tabel users di schema public: %v", err)
	}
	if publicUsers == 0 {
		return
	}

	var leaked int
	if err := db.Pool.QueryRow(ctx,
		`select count(*) from public.users where lower(email) = lower($1)`, email).Scan(&leaked); err != nil {
		t.Fatalf("mencari baris test di schema public: %v", err)
	}
	if leaked != 0 {
		t.Errorf("%d baris test bocor ke public.users", leaked)
	}
}
