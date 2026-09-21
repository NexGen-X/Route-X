package main

import (
	"bytes"
	"context"
	"crypto/rand"
	"encoding/hex"
	"errors"
	"fmt"
	"io"
	"log/slog"
	"net/netip"
	"os"
	"strings"
	"testing"

	"github.com/jackc/pgx/v5/pgxpool"

	"github.com/NexGen-X/Route-X/internal/config"
	"github.com/NexGen-X/Route-X/internal/database"
	"github.com/NexGen-X/Route-X/internal/database/repo/keys"
	"github.com/NexGen-X/Route-X/internal/security"
)

// Pepper uji sama panjangnya dengan yang dipakai paket keys, memakai konstanta yang
// berbeda agar tidak ada peluang ikatan terselubung antar paket test.
var testPepper = []byte("pepper-cli-uji-yang-cukup-32byte!")

// testSchema membuat schema sementara berisi skema lengkap, lalu membersihkannya.
//
// Sama seperti test di paket keys: setiap test memakai schema sendiri agar bisa jalan
// paralel dan tidak pernah menyentuh schema public maupun database produksi.
func testSchema(t *testing.T) (dsn string, pool *pgxpool.Pool) {
	t.Helper()
	if testing.Short() {
		t.Skip("butuh PostgreSQL")
	}

	base := os.Getenv("TEST_DATABASE_URL")
	if base == "" {
		base = os.Getenv("DATABASE_URL")
	}
	if base == "" {
		t.Skip("TEST_DATABASE_URL / DATABASE_URL tidak diset")
	}

	ctx := context.Background()
	buf := make([]byte, 6)
	if _, err := rand.Read(buf); err != nil {
		t.Fatalf("acak: %v", err)
	}
	schema := "test_apikey_" + hex.EncodeToString(buf)

	admin, err := pgxpool.New(ctx, base)
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

	// search_path diteruskan sebagai runtime parameter, jadi seluruh migrasi dan query
	// test mendarat di schema sementara.
	sep := "?"
	if strings.Contains(base, "?") {
		sep = "&"
	}
	dsn = base + sep + "search_path=" + schema

	db, err := database.Connect(ctx, &config.Config{
		DatabaseURL: security.Secret(dsn),
		DBMaxConns:  4,
		DBMinConns:  1,
	}, slog.New(slog.NewJSONHandler(io.Discard, nil)))
	if err != nil {
		t.Fatalf("menghubungkan database uji: %v", err)
	}
	t.Cleanup(db.Close)

	if _, err := database.Migrate(ctx, db, slog.New(slog.NewJSONHandler(io.Discard, nil))); err != nil {
		t.Fatalf("menjalankan migrasi: %v", err)
	}

	return dsn, db.Pool
}

// testEnv menyiapkan schema sementara beserta keyTool di atasnya.
func testEnv(t *testing.T) (context.Context, *pgxpool.Pool, *keyTool) {
	t.Helper()

	_, pool := testSchema(t)

	r, err := keys.New(pool, testPepper)
	if err != nil {
		t.Fatalf("keys.New: %v", err)
	}

	return context.Background(), pool, &keyTool{
		pool:   pool,
		repo:   r,
		out:    &bytes.Buffer{},
		errOut: &bytes.Buffer{},
	}
}

func newUser(t *testing.T, ctx context.Context, pool *pgxpool.Pool) string {
	t.Helper()
	buf := make([]byte, 4)
	_, _ = rand.Read(buf)

	var id string
	err := pool.QueryRow(ctx, `
		insert into users (email, password_hash) values ($1, $2) returning id::text`,
		fmt.Sprintf("cli-%s@example.test", hex.EncodeToString(buf)),
		"$argon2id$v=19$m=8192,t=1,p=1$c2FsdHNhbHRzYWx0c2E$aGFzaGhhc2hoYXNoaGFzaGhhc2hoYXNoaGE",
	).Scan(&id)
	if err != nil {
		t.Fatalf("membuat pengguna: %v", err)
	}
	return id
}

// --- validasi masukan --------------------------------------------------------

func TestValidateCreateOptions(t *testing.T) {
	tests := []struct {
		name    string
		opts    createOptions
		wantErr string
	}{
		{
			name:    "nama kosong",
			opts:    createOptions{name: "", scopes: scopeFlag{"inference"}},
			wantErr: "--name wajib diisi",
		},
		{
			name:    "nama hanya spasi",
			opts:    createOptions{name: "   ", scopes: scopeFlag{"inference"}},
			wantErr: "--name wajib diisi",
		},
		{
			name:    "nama bergaris spasi",
			opts:    createOptions{name: " ci ", scopes: scopeFlag{"inference"}},
			wantErr: "tidak boleh diawali atau diakhiri spasi",
		},
		{
			name:    "cakupan asing",
			opts:    createOptions{name: "ci", scopes: scopeFlag{"root:everything"}},
			wantErr: "cakupan \"root:everything\" tidak dikenali",
		},
		{
			name: "cakupan valid",
			opts: createOptions{
				name:   "ci",
				scopes: scopeFlag{"inference", "usage:read", "admin:write"},
			},
		},
		{
			name: "nama dan cakupan tunggal",
			opts: createOptions{name: "ci", scopes: scopeFlag{"models:read"}},
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			err := tc.opts.validate()
			if tc.wantErr == "" {
				if err != nil {
					t.Fatalf("validate() = %v, mau nil", err)
				}
				return
			}
			if err == nil {
				t.Fatalf("validate() = nil, mau error yang memuat %q", tc.wantErr)
			}
			if !strings.Contains(err.Error(), tc.wantErr) {
				t.Errorf("validate() = %q, tidak memuat %q", err, tc.wantErr)
			}
		})
	}
}

func TestScopeFlagParsing(t *testing.T) {
	t.Run("repeatable", func(t *testing.T) {
		var s scopeFlag
		if err := s.Set("inference"); err != nil {
			t.Fatalf("Set: %v", err)
		}
		if err := s.Set("usage:read"); err != nil {
			t.Fatalf("Set: %v", err)
		}
		want := scopeFlag{"inference", "usage:read"}
		if len(s) != len(want) || s[0] != want[0] || s[1] != want[1] {
			t.Errorf("scopeFlag = %v, mau %v", s, want)
		}
	})

	t.Run("koma sebagai pemisah", func(t *testing.T) {
		var s scopeFlag
		if err := s.Set("inference, usage:read , models:read"); err != nil {
			t.Fatalf("Set: %v", err)
		}
		want := scopeFlag{"inference", "usage:read", "models:read"}
		if len(s) != len(want) {
			t.Fatalf("scopeFlag = %v, mau %v", s, want)
		}
		for i := range want {
			if s[i] != want[i] {
				t.Errorf("scopeFlag[%d] = %q, mau %q", i, s[i], want[i])
			}
		}
	})

	t.Run("entri kosong dibuang", func(t *testing.T) {
		var s scopeFlag
		if err := s.Set("inference,, ,"); err != nil {
			t.Fatalf("Set: %v", err)
		}
		if len(s) != 1 || s[0] != "inference" {
			t.Errorf("scopeFlag = %v, mau [inference]", s)
		}
	})
}

// --- subcommand create -------------------------------------------------------

// Key mentah hanya boleh muncul di stdout, dan tidak boleh muncul di stderr sama sekali
// — termasuk bukan sebagai bagian dari hash atau bentuk tersamar.
func TestCreateOutputSeparation(t *testing.T) {
	ctx, pool, t_ := testEnv(t)
	owner := newUser(t, ctx, pool)

	out := &bytes.Buffer{}
	errOut := &bytes.Buffer{}
	t_.out, t_.errOut = out, errOut

	code := t_.create(ctx, []string{"--name", "ci-pipeline", "--owner", owner})
	if code != 0 {
		t.Fatalf("create = %d, mau 0; stderr: %s", code, errOut.String())
	}

	raw := strings.TrimSpace(out.String())
	if raw == "" {
		t.Fatal("stdout kosong, seharusnya berisi nilai key")
	}
	if !strings.HasPrefix(raw, security.KeyPrefixTest) {
		t.Errorf("stdout = %q..., seharusnya diawali %s", raw[:12], security.KeyPrefixTest)
	}
	if _, err := security.ParseAPIKey(raw); err != nil {
		t.Errorf("stdout bukan API key yang sah: %v", err)
	}

	// stdout harus berisi tepat satu baris: keynya saja, tanpa peringatan atau metadata.
	if got := out.String(); got != raw+"\n" {
		t.Errorf("stdout = %q, seharusnya hanya key diikuti satu baris baru", got)
	}

	// Peringatan wajib ada, dan harus di stderr.
	if !strings.Contains(errOut.String(), "Key ini hanya ditampilkan sekali") {
		t.Errorf("stderr tidak memuat peringatan: %q", errOut.String())
	}
	for _, label := range []string{"ID", "Lingkungan", "Tersamar", "Pemilik", "Cakupan", "Dibuat"} {
		if !strings.Contains(errOut.String(), label) {
			t.Errorf("stderr tidak memuat %q: %q", label, errOut.String())
		}
	}
	if !strings.Contains(errOut.String(), "Cakupan    : inference") {
		t.Errorf("stderr = %q, cakupan default seharusnya inference", errOut.String())
	}

	// Kebocoran: nilai mentah tidak boleh muncul di stderr.
	body := strings.TrimPrefix(raw, security.KeyPrefixTest)
	if strings.Contains(errOut.String(), body) {
		t.Error("stderr memuat bagian acak key — key seharusnya hanya di stdout")
	}
	if strings.Contains(errOut.String(), raw) {
		t.Error("stderr memuat nilai key utuh")
	}
}

func TestCreateLivePrefix(t *testing.T) {
	ctx, pool, t_ := testEnv(t)
	owner := newUser(t, ctx, pool)

	out := &bytes.Buffer{}
	errOut := &bytes.Buffer{}
	t_.out, t_.errOut = out, errOut

	code := t_.create(ctx, []string{"--name", "produksi", "--live", "--owner", owner})
	if code != 0 {
		t.Fatalf("create = %d, mau 0; stderr: %s", code, errOut.String())
	}

	raw := strings.TrimSpace(out.String())
	if !strings.HasPrefix(raw, security.KeyPrefixLive) {
		t.Errorf("key live = %q..., seharusnya diawali %s", raw[:12], security.KeyPrefixLive)
	}
	if !strings.Contains(errOut.String(), "sk_live_") {
		t.Errorf("stderr tidak menandai lingkungan live: %q", errOut.String())
	}
}

func TestCreatePersistsAndAuthenticates(t *testing.T) {
	ctx, pool, t_ := testEnv(t)
	owner := newUser(t, ctx, pool)

	out := &bytes.Buffer{}
	t_.out, t_.errOut = out, &bytes.Buffer{}

	scopes := []string{"inference", "usage:read"}
	code := t_.create(ctx, []string{
		"--name", "billing", "--owner", owner,
		"--scope", "inference", "--scope", "usage:read",
	})
	if code != 0 {
		t.Fatalf("create = %d, mau 0", code)
	}
	raw := strings.TrimSpace(out.String())

	// Baris di database harus sejalan dengan yang dilaporkan.
	var (
		gotName, gotPrefix, gotLast4 string
		gotScopes                    []string
	)
	err := pool.QueryRow(ctx, `
		select name, key_prefix, last4, scopes from api_keys where owner_user_id = $1`,
		owner).Scan(&gotName, &gotPrefix, &gotLast4, &gotScopes)
	if err != nil {
		t.Fatalf("query api_keys: %v", err)
	}
	if gotName != "billing" || gotPrefix != security.KeyPrefixTest {
		t.Errorf("baris = (%q, %q), mau (billing, %s)", gotName, gotPrefix, security.KeyPrefixTest)
	}
	if len(gotLast4) != 4 || !strings.HasSuffix(raw, gotLast4) {
		t.Errorf("last4 = %q, harus 4 karakter terakhir key", gotLast4)
	}
	if len(gotScopes) != len(scopes) {
		t.Errorf("scopes = %v, mau %v", gotScopes, scopes)
	}

	// Authenticate memakai pepper yang sama persis — inilah yang membuktikan tidak ada
	// salah hitung HMAC antara pembuatan dan verifikasi.
	ip := netip.MustParseAddr("203.0.113.9")
	k, err := t_.repo.Authenticate(ctx, raw, ip)
	if err != nil {
		t.Fatalf("Authenticate: %v", err)
	}
	if k.Name != "billing" {
		t.Errorf("Authenticate: name = %q", k.Name)
	}
	for _, s := range scopes {
		if !k.HasScope(s) {
			t.Errorf("Authenticate: key tidak punya cakupan %q", s)
		}
	}
}

func TestCreateRejectsBadScopeBeforeDB(t *testing.T) {
	ctx, pool, t_ := testEnv(t)
	owner := newUser(t, ctx, pool)

	errOut := &bytes.Buffer{}
	t_.out, t_.errOut = &bytes.Buffer{}, errOut

	code := t_.create(ctx, []string{"--name", "rusak", "--owner", owner, "--scope", "root"})
	if code != 1 {
		t.Fatalf("create = %d, mau 1", code)
	}
	if !strings.Contains(errOut.String(), "tidak dikenali") {
		t.Errorf("stderr = %q, tidak menjelaskan cakupan asing", errOut)
	}

	// Tidak boleh ada baris yang tertulis.
	var n int
	if err := pool.QueryRow(ctx, `select count(*) from api_keys`).Scan(&n); err != nil {
		t.Fatalf("count: %v", err)
	}
	if n != 0 {
		t.Errorf("terdapat %d baris api_keys, seharusnya 0 — cakupan asing harus ditolak sebelum menyentuh database", n)
	}
}

func TestCreateWithDBURLFlag(t *testing.T) {
	dsn, pool := testSchema(t)
	owner := newUser(t, context.Background(), pool)

	// keyTool tanpa pool: koneksi harus dibuka olehnya dari --db-url.
	t_ := &keyTool{out: &bytes.Buffer{}, errOut: &bytes.Buffer{}}
	defer t_.close()

	// DATABASE_URL lingkungan sengaja menunjuk ke host yang tidak ada; --db-url wajib
	// menang, kalau tidak test ini gagal di sambungan.
	t.Setenv("DATABASE_URL", "postgres://host-tidak-ada-4f2a:5432/nolinux?sslmode=disable")
	t.Setenv("API_KEY_PEPPER", "MDEyMzQ1Njc4OWFiY2RlZjAxMjM0NTY3ODlhYmNkZWY=")

	out := &bytes.Buffer{}
	t_.out = out

	code := t_.create(context.Background(), []string{"--name", "lewat-flag", "--owner", owner, "--db-url", dsn})
	if code != 0 {
		t.Fatalf("create = %d, mau 0; stderr: %s", code, t_.errOut.(*bytes.Buffer).String())
	}

	raw := strings.TrimSpace(out.String())
	if !strings.HasPrefix(raw, security.KeyPrefixTest) {
		t.Errorf("key = %q..., seharusnya diawali %s", raw[:12], security.KeyPrefixTest)
	}

	var n int
	if err := pool.QueryRow(context.Background(),
		`select count(*) from api_keys where name = 'lewat-flag'`).Scan(&n); err != nil {
		t.Fatalf("verifikasi baris: %v", err)
	}
	if n != 1 {
		t.Errorf("api_keys lewat-flag = %d baris, mau 1", n)
	}
}

func TestCreateRequiresName(t *testing.T) {
	ctx, _, t_ := testEnv(t)

	errOut := &bytes.Buffer{}
	t_.out, t_.errOut = &bytes.Buffer{}, errOut

	if code := t_.create(ctx, []string{}); code != 1 {
		t.Fatalf("create tanpa --name = %d, mau 1", code)
	}
	if !strings.Contains(errOut.String(), "--name wajib diisi") {
		t.Errorf("stderr = %q, tidak menjelaskan kekurangan --name", errOut)
	}
}

func TestCreateDefaultOwner(t *testing.T) {
	ctx, pool, t_ := testEnv(t)
	owner := newUser(t, ctx, pool)

	out := &bytes.Buffer{}
	t_.out, t_.errOut = out, &bytes.Buffer{}

	// Tanpa --owner: pemilik default adalah pengguna pertama.
	code := t_.create(ctx, []string{"--name", "tanpa-pemilik-eksplisit"})
	if code != 0 {
		t.Fatalf("create = %d, mau 0", code)
	}

	var gotOwner string
	if err := pool.QueryRow(ctx, `select owner_user_id::text from api_keys where name = $1`,
		"tanpa-pemilik-eksplisit").Scan(&gotOwner); err != nil {
		t.Fatalf("query: %v", err)
	}
	if gotOwner != owner {
		t.Errorf("pemilik default = %q, mau pengguna pertama %q", gotOwner, owner)
	}
}

func TestCreateWithoutUser(t *testing.T) {
	ctx, _, t_ := testEnv(t)

	errOut := &bytes.Buffer{}
	t_.out, t_.errOut = &bytes.Buffer{}, errOut

	// Schema baru belum punya pengguna, jadi pemilik default tidak bisa ditentukan.
	code := t_.create(ctx, []string{"--name", "ci"})
	if code != 1 {
		t.Fatalf("create = %d, mau 1", code)
	}
	if !strings.Contains(errOut.String(), "tidak ada pengguna") {
		t.Errorf("stderr = %q, seharusnya menjelaskan tidak ada pengguna", errOut)
	}
}

func TestCreateUnknownOwner(t *testing.T) {
	ctx, _, t_ := testEnv(t)

	errOut := &bytes.Buffer{}
	t_.out, t_.errOut = &bytes.Buffer{}, errOut

	code := t_.create(ctx, []string{"--name", "ci", "--owner", "11111111-1111-1111-1111-111111111111"})
	if code != 1 {
		t.Fatalf("create = %d, mau 1", code)
	}
	if !strings.Contains(errOut.String(), "gagal membuat API key") {
		t.Errorf("stderr = %q, seharusnya melaporkan kegagalan pembuatan", errOut)
	}
}

// --- subcommand list ---------------------------------------------------------

func TestListShowsMaskedOnly(t *testing.T) {
	ctx, pool, t_ := testEnv(t)
	owner := newUser(t, ctx, pool)

	out := &bytes.Buffer{}
	t_.out, t_.errOut = out, &bytes.Buffer{}

	// Buat dua key; nilai mentahnya dikumpulkan untuk dibandingkan dengan output list.
	var raws []string
	for _, name := range []string{"kunci-a", "kunci-b"} {
		buf := &bytes.Buffer{}
		t_.out = buf
		if code := t_.create(ctx, []string{"--name", name, "--owner", owner, "--live"}); code != 0 {
			t.Fatalf("create %s = %d", name, code)
		}
		raws = append(raws, strings.TrimSpace(buf.String()))
	}

	out = &bytes.Buffer{}
	t_.out = out
	if code := t_.list(ctx, nil); code != 0 {
		t.Fatalf("list = %d, mau 0; out: %s", code, out.String())
	}

	got := out.String()
	for _, raw := range raws {
		if strings.Contains(got, raw) {
			t.Error("output list memuat nilai key mentah")
		}
		if !strings.Contains(got, security.MaskAPIKey(raw)) {
			t.Errorf("output list tidak memuat bentuk tersamar untuk key %s***", raw[:9])
		}
	}
	for _, header := range []string{"ID", "NAMA", "STATUS", "CAKUPAN", "DIBUAT"} {
		if !strings.Contains(got, header) {
			t.Errorf("output list tidak memuat header %q", header)
		}
	}
}

func TestListEmpty(t *testing.T) {
	ctx, _, t_ := testEnv(t)

	errOut := &bytes.Buffer{}
	t_.out, t_.errOut = &bytes.Buffer{}, errOut

	if code := t_.list(ctx, nil); code != 0 {
		t.Fatalf("list = %d, mau 0", code)
	}
	if !strings.Contains(errOut.String(), "Belum ada API key") {
		t.Errorf("stderr = %q, seharusnya melaporkan tidak ada key", errOut)
	}
}

// --- subcommand dispatch -----------------------------------------------------

func TestRunRejectsUnknownSubcommand(t *testing.T) {
	errOut := &bytes.Buffer{}
	code := run(context.Background(), []string{"tidak-ada"}, &bytes.Buffer{}, errOut)
	if code != 1 {
		t.Fatalf("run = %d, mau 1", code)
	}
	if !strings.Contains(errOut.String(), "tidak dikenali") {
		t.Errorf("stderr = %q, seharusnya menolak subcommand asing", errOut)
	}
}

func TestRunNoArgsShowsUsage(t *testing.T) {
	errOut := &bytes.Buffer{}
	code := run(context.Background(), nil, &bytes.Buffer{}, errOut)
	if code != 1 {
		t.Fatalf("run = %d, mau 1", code)
	}
	if !strings.Contains(errOut.String(), "routex-apikey") {
		t.Errorf("stderr = %q, seharusnya menampilkan pemakaian", errOut)
	}
}

func TestRunHelpExitsZero(t *testing.T) {
	out := &bytes.Buffer{}
	if code := run(context.Background(), []string{"--help"}, out, &bytes.Buffer{}); code != 0 {
		t.Fatalf("run --help = %d, mau 0", code)
	}
	if !strings.Contains(out.String(), "create") || !strings.Contains(out.String(), "list") {
		t.Errorf("output --help tidak menjelaskan subcommand: %q", out)
	}
}

// GenerateAPIKey memang menghasilkan key 43 karakter base62; CLI mengandalkan ini dan
// tidak boleh memeriksa panjangnya sendiri.
func TestGeneratedKeyShape(t *testing.T) {
	for _, live := range []bool{false, true} {
		g, err := security.GenerateAPIKey(live, testPepper)
		if err != nil {
			t.Fatalf("GenerateAPIKey(%v): %v", live, err)
		}
		wantPrefix := security.KeyPrefixTest
		if live {
			wantPrefix = security.KeyPrefixLive
		}
		if g.Prefix != wantPrefix {
			t.Errorf("prefix = %q, mau %q", g.Prefix, wantPrefix)
		}
		if _, err := security.ParseAPIKey(g.Raw.Reveal()); err != nil {
			t.Errorf("key hasilGenerateAPIKey tidak lulus ParseAPIKey: %v", err)
		}
		if g.Masked != security.MaskedFromParts(g.Prefix, g.Last4) {
			t.Errorf("Masked = %q, mau %q", g.Masked, security.MaskedFromParts(g.Prefix, g.Last4))
		}
	}
}

func TestEmptyPepperRejected(t *testing.T) {
	if _, err := security.GenerateAPIKey(true, nil); !errors.Is(err, security.ErrEmptyPepper) {
		t.Errorf("GenerateAPIKey dengan pepper kosong: err = %v, mau ErrEmptyPepper", err)
	}
}
