package apikey

import (
	"bytes"
	"context"
	"crypto/rand"
	"encoding/hex"
	"io"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"net/netip"
	"os"
	"strings"
	"testing"
	"time"

	"github.com/jackc/pgx/v5/pgxpool"

	"github.com/NexGen-X/Route-X/internal/config"
	"github.com/NexGen-X/Route-X/internal/database"
	"github.com/NexGen-X/Route-X/internal/database/repo/keys"
	"github.com/NexGen-X/Route-X/internal/httpx"
	"github.com/NexGen-X/Route-X/internal/security"
)

// Test di berkas ini memakai keys.Repo yang sungguhan di atas PostgreSQL. Yang diuji
// adalah sambungan antara middleware dan lapisan data: bahwa alasan penolakan yang
// dihitung di dalam query benar-benar berakhir sebagai 401 seragam, dan bahwa pencatatan
// pemakaian betul-betul menulis ke baris yang tepat. Keputusan bentuk respons sendiri
// sudah diuji tanpa database di middleware_test.go.

var testPepper = []byte("pepper-uji-apikey-yang-cukup-panjang!")

// pgEnv menyiapkan schema sementara berisi skema lengkap, lalu membersihkannya.
//
// Setiap test memakai schema-nya sendiri agar bisa jalan paralel dan TIDAK PERNAH
// menyentuh schema public milik database development.
func pgEnv(t *testing.T) (context.Context, *pgxpool.Pool, *keys.Repo) {
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
	schema := "test_apikey_" + hex.EncodeToString(buf)

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
	cfg := &config.Config{
		DatabaseURL: security.Secret(dsn + sep + "search_path=" + schema),
		DBMaxConns:  4,
		DBMinConns:  1,
	}

	logger := slog.New(slog.NewJSONHandler(io.Discard, nil))
	db, err := database.Connect(ctx, cfg, logger)
	if err != nil {
		t.Fatalf("menghubungkan database uji: %v", err)
	}
	t.Cleanup(db.Close)

	if _, err := database.Migrate(ctx, db, logger); err != nil {
		t.Fatalf("menjalankan migrasi: %v", err)
	}

	repo, err := keys.New(db.Pool, testPepper)
	if err != nil {
		t.Fatalf("keys.New: %v", err)
	}
	return ctx, db.Pool, repo
}

func pgUser(t *testing.T, ctx context.Context, pool *pgxpool.Pool) string {
	t.Helper()
	buf := make([]byte, 4)
	_, _ = rand.Read(buf)

	var id string
	err := pool.QueryRow(ctx, `
		insert into users (email, password_hash) values ($1, $2) returning id::text`,
		"admin-"+hex.EncodeToString(buf)+"@example.test",
		"$argon2id$v=19$m=8192,t=1,p=1$c2FsdHNhbHRzYWx0c2E$aGFzaGhhc2hoYXNoaGFzaGhhc2hoYXNoaGE",
	).Scan(&id)
	if err != nil {
		t.Fatalf("membuat pengguna: %v", err)
	}
	return id
}

func pgExec(t *testing.T, ctx context.Context, pool *pgxpool.Pool, sql string, args ...any) {
	t.Helper()
	if _, err := pool.Exec(ctx, sql, args...); err != nil {
		t.Fatalf("exec: %v", err)
	}
}

// pgCreate membuat satu API key dan mengembalikan nilai mentahnya beserta barisnya.
func pgCreate(t *testing.T, ctx context.Context, repo *keys.Repo, p keys.CreateParams) (string, *keys.Key) {
	t.Helper()
	created, err := repo.Create(ctx, p)
	if err != nil {
		t.Fatalf("Create: %v", err)
	}
	return created.Raw.Reveal(), created.Key
}

func ptr[T any](v T) *T { return &v }

// Key yang sah lolos, principal-nya ada di context, dan pemakaiannya tercatat.
func TestIntegrationAuthenticateSuccess(t *testing.T) {
	ctx, pool, repo := pgEnv(t)
	owner := pgUser(t, ctx, pool)

	raw, key := pgCreate(t, ctx, repo, keys.CreateParams{
		Name: "kunci produksi", OwnerUserID: owner, Live: true,
		Scopes: []string{keys.ScopeInference, keys.ScopeModelsRead},
	})

	a := NewAuthenticator(repo, nil, discardLogger(), WithTouchInterval(time.Nanosecond))

	var seen *Principal
	r := httptest.NewRequest("POST", "/v1/chat/completions", nil)
	r.Header.Set("Authorization", "Bearer "+raw)
	rec := serve(a, okHandler(&seen), r)

	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d, mau 200 (body: %s)", rec.Code, rec.Body.String())
	}
	if seen == nil {
		t.Fatal("principal tidak ada di context handler")
	}
	if seen.ID() != key.ID {
		t.Errorf("principal.ID() = %q, mau %q", seen.ID(), key.ID)
	}
	if seen.Masked() != key.Masked() {
		t.Errorf("principal.Masked() = %q, mau %q", seen.Masked(), key.Masked())
	}
	if !seen.Can(keys.ScopeInference) || seen.Can(keys.ScopeAdminWrite) {
		t.Errorf("cakupan principal = %v", seen.Scopes())
	}

	// TouchUsage berjalan di goroutine latar, jadi hasilnya ditunggu — sekaligus
	// memastikan UPDATE-nya sudah selesai sebelum schema dibersihkan.
	deadline := time.Now().Add(3 * time.Second)
	for {
		var lastUsed *time.Time
		var lastIP *string
		err := pool.QueryRow(ctx,
			`select last_used_at, host(last_used_ip) from api_keys where id = $1`, key.ID).
			Scan(&lastUsed, &lastIP)
		if err != nil {
			t.Fatalf("membaca last_used_at: %v", err)
		}
		if lastUsed != nil {
			if lastIP == nil || *lastIP != "192.0.2.1" {
				t.Errorf("last_used_ip = %v, mau 192.0.2.1", lastIP)
			}
			break
		}
		if time.Now().After(deadline) {
			t.Fatal("last_used_at tidak pernah diperbarui")
		}
		time.Sleep(20 * time.Millisecond)
	}
}

// Setiap alasan penolakan yang dihitung repo harus berakhir sebagai 401 dengan body yang
// identik byte per byte. Ini versi ujung-ke-ujung dari kebijakan yang sama di
// middleware_test.go, lewat query yang sesungguhnya.
func TestIntegrationRejectionsShareIdenticalBody(t *testing.T) {
	ctx, pool, repo := pgEnv(t)
	owner := pgUser(t, ctx, pool)

	newKey := func(p keys.CreateParams) (string, *keys.Key) {
		p.OwnerUserID = owner
		p.Live = true
		p.Scopes = []string{keys.ScopeInference}
		if p.Name == "" {
			p.Name = "kunci uji"
		}
		return pgCreate(t, ctx, repo, p)
	}

	type scenario struct {
		name string
		raw  string
	}
	var scenarios []scenario

	// not_found: bentuknya sah dan pepper-nya benar, tapi barisnya tidak pernah dibuat.
	absent, err := security.GenerateAPIKey(true, testPepper)
	if err != nil {
		t.Fatalf("GenerateAPIKey: %v", err)
	}
	scenarios = append(scenarios, scenario{keys.ReasonNotFound, absent.Raw.Reveal()})

	rawDisabled, keyDisabled := newKey(keys.CreateParams{Name: "dimatikan"})
	pgExec(t, ctx, pool, `update api_keys set status='disabled' where id=$1`, keyDisabled.ID)
	scenarios = append(scenarios, scenario{keys.ReasonDisabled, rawDisabled})

	rawRevoked, keyRevoked := newKey(keys.CreateParams{Name: "dicabut"})
	pgExec(t, ctx, pool, `update api_keys set status='revoked', revoked_at=now() where id=$1`, keyRevoked.ID)
	scenarios = append(scenarios, scenario{keys.ReasonRevoked, rawRevoked})

	rawExpired, _ := newKey(keys.CreateParams{Name: "kedaluwarsa", ExpiresAt: ptr(time.Now().Add(-time.Hour))})
	scenarios = append(scenarios, scenario{keys.ReasonExpired, rawExpired})

	// Daftar putih yang tidak memuat 192.0.2.1, yaitu alamat yang dipakai httptest.
	rawWrongIP, _ := newKey(keys.CreateParams{
		Name:        "ip terbatas",
		IPAllowlist: mustPrefixes(t, "10.0.0.0/8"),
	})
	scenarios = append(scenarios, scenario{keys.ReasonIPNotAllowed, rawWrongIP})

	// Blokir dipasang pada key itu sendiri, bukan pada IP atau pengguna, supaya
	// skenario lain di schema yang sama tidak ikut terkena.
	rawBanned, keyBanned := newKey(keys.CreateParams{Name: "diblokir"})
	pgExec(t, ctx, pool,
		`insert into bans (subject_kind, subject, reason) values ('api_key', $1, 'uji')`, keyBanned.ID)
	scenarios = append(scenarios, scenario{keys.ReasonBanned, rawBanned})

	// Pencatatan pemakaian dimatikan: jalur penolakan tidak menyentuhnya, dan tidak ada
	// UPDATE latar yang bisa berbenturan dengan pembersihan schema.
	a := NewAuthenticator(repo, nil, discardLogger(), WithTouchInterval(0))

	var (
		first     []byte
		firstName string
	)
	for _, tc := range scenarios {
		t.Run(tc.name, func(t *testing.T) {
			next := http.HandlerFunc(func(http.ResponseWriter, *http.Request) {
				t.Error("handler dijalankan padahal autentikasi gagal")
			})
			r := httptest.NewRequest("POST", "/v1/chat/completions", nil)
			r.Header.Set("Authorization", "Bearer "+tc.raw)
			rec := serve(a, next, r)

			if rec.Code != http.StatusUnauthorized {
				t.Fatalf("status = %d, mau 401 (body: %s)", rec.Code, rec.Body.String())
			}
			if containsAny(rec.Body.String(), tc.raw, tc.name) {
				t.Errorf("body membocorkan key atau alasan: %s", rec.Body.String())
			}
			if first == nil {
				first, firstName = bytes.Clone(rec.Body.Bytes()), tc.name
				return
			}
			if !bytes.Equal(first, rec.Body.Bytes()) {
				t.Errorf("body berbeda dari kasus %q:\n %q\n %q", firstName, first, rec.Body.Bytes())
			}
		})
	}

	// Pembanding positif: key yang sehat tetap lolos di schema yang sama, sehingga
	// keseragaman di atas tidak bisa dicapai dengan menolak semuanya.
	rawOK, _ := newKey(keys.CreateParams{Name: "sehat"})
	r := httptest.NewRequest("POST", "/v1/chat/completions", nil)
	r.Header.Set("Authorization", "Bearer "+rawOK)
	if rec := serve(a, okHandler(nil), r); rec.Code != http.StatusOK {
		t.Fatalf("key sehat ditolak: status %d (%s)", rec.Code, rec.Body.String())
	}
}

// Cakupan yang tersimpan di database menentukan hasil RequireScope.
func TestIntegrationRequireScope(t *testing.T) {
	ctx, pool, repo := pgEnv(t)
	owner := pgUser(t, ctx, pool)

	raw, _ := pgCreate(t, ctx, repo, keys.CreateParams{
		Name: "hanya baca model", OwnerUserID: owner, Live: true,
		Scopes: []string{keys.ScopeModelsRead},
	})
	a := NewAuthenticator(repo, nil, discardLogger(), WithTouchInterval(0))

	run := func(scope string) int {
		next := RequireScope(scope)(okHandler(nil))
		r := httptest.NewRequest("POST", "/v1/chat/completions", nil)
		r.Header.Set("Authorization", "Bearer "+raw)
		rec := httptest.NewRecorder()
		httpx.RealIP(nil)(a.Authenticate()(next)).ServeHTTP(rec, r)
		return rec.Code
	}

	if got := run(keys.ScopeModelsRead); got != http.StatusOK {
		t.Errorf("cakupan yang dimiliki: status = %d, mau 200", got)
	}
	if got := run(keys.ScopeInference); got != http.StatusForbidden {
		t.Errorf("cakupan yang tidak dimiliki: status = %d, mau 403", got)
	}
}

// mustPrefixes menyusun daftar putih IP untuk CreateParams.
func mustPrefixes(t *testing.T, values ...string) []netip.Prefix {
	t.Helper()
	out := make([]netip.Prefix, 0, len(values))
	for _, v := range values {
		prefix, err := netip.ParsePrefix(v)
		if err != nil {
			t.Fatalf("netip.ParsePrefix(%q): %v", v, err)
		}
		out = append(out, prefix)
	}
	return out
}

func containsAny(haystack string, needles ...string) bool {
	for _, n := range needles {
		if n != "" && strings.Contains(haystack, n) {
			return true
		}
	}
	return false
}
