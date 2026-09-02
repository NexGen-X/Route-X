package keys

import (
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
	"time"

	"github.com/jackc/pgx/v5/pgxpool"

	"github.com/NexGen-X/Route-X/internal/config"
	"github.com/NexGen-X/Route-X/internal/database"
	"github.com/NexGen-X/Route-X/internal/database/repo"
	"github.com/NexGen-X/Route-X/internal/security"
)

var testPepper = []byte("pepper-uji-yang-cukup-panjang-32byte!")

// testEnv menyiapkan schema sementara berisi skema lengkap, lalu membersihkannya.
//
// Setiap test memakai schema-nya sendiri agar bisa jalan paralel dan tidak pernah
// menyentuh schema public milik database development.
func testEnv(t *testing.T) (context.Context, *pgxpool.Pool, *Repo) {
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
	schema := "test_keys_" + hex.EncodeToString(buf)

	// Koneksi administratif untuk membuat dan menghapus schema.
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

	// search_path diteruskan sebagai runtime parameter lewat connection string, jadi
	// seluruh migrasi dan query test mendarat di schema sementara.
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

	r, err := New(db.Pool, testPepper)
	if err != nil {
		t.Fatalf("New: %v", err)
	}
	return ctx, db.Pool, r
}

// newUser membuat satu pengguna dan mengembalikan ID-nya.
func newUser(t *testing.T, ctx context.Context, pool *pgxpool.Pool) string {
	t.Helper()
	buf := make([]byte, 4)
	_, _ = rand.Read(buf)

	var id string
	err := pool.QueryRow(ctx, `
		insert into users (email, password_hash) values ($1, $2) returning id::text`,
		fmt.Sprintf("admin-%s@example.test", hex.EncodeToString(buf)),
		"$argon2id$v=19$m=8192,t=1,p=1$c2FsdHNhbHRzYWx0c2E$aGFzaGhhc2hoYXNoaGFzaGhhc2hoYXNoaGE",
	).Scan(&id)
	if err != nil {
		t.Fatalf("membuat pengguna: %v", err)
	}
	return id
}

func mustCreate(t *testing.T, ctx context.Context, r *Repo, p CreateParams) *Created {
	t.Helper()
	c, err := r.Create(ctx, p)
	if err != nil {
		t.Fatalf("Create: %v", err)
	}
	return c
}

func TestCreateAndAuthenticate(t *testing.T) {
	ctx, pool, r := testEnv(t)
	owner := newUser(t, ctx, pool)

	created := mustCreate(t, ctx, r, CreateParams{
		Name: "kunci produksi", OwnerUserID: owner, Live: true,
		Scopes: []string{ScopeInference, ScopeModelsRead},
	})

	raw := created.Raw.Reveal()
	if !strings.HasPrefix(raw, security.KeyPrefixLive) {
		t.Errorf("prefix key = %q", raw[:8])
	}
	if created.Key.Masked() != security.KeyPrefixLive+"****"+created.Key.Last4 {
		t.Errorf("Masked() = %q", created.Key.Masked())
	}
	if !created.Key.HasScope(ScopeInference) || created.Key.HasScope(ScopeAdminWrite) {
		t.Errorf("Scopes = %v", created.Key.Scopes)
	}

	got, err := r.Authenticate(ctx, raw, netip.MustParseAddr("203.0.113.9"))
	if err != nil {
		t.Fatalf("Authenticate: %v", err)
	}
	if got.ID != created.Key.ID {
		t.Errorf("ID = %q, mau %q", got.ID, created.Key.ID)
	}
}

// Nilai mentah key tidak boleh ada di database dalam bentuk apa pun.
func TestRawKeyNeverStored(t *testing.T) {
	ctx, pool, r := testEnv(t)
	owner := newUser(t, ctx, pool)

	created := mustCreate(t, ctx, r, CreateParams{Name: "k", OwnerUserID: owner, Live: true})
	raw := created.Raw.Reveal()
	body := strings.TrimPrefix(raw, security.KeyPrefixLive)

	// Cari nilai mentah di SETIAP kolom teks tabel api_keys.
	var found bool
	err := pool.QueryRow(ctx, `
		select exists (
			select 1 from api_keys
			where key_hash like '%' || $1 || '%'
			   or name     like '%' || $1 || '%'
			   or last4    = $1
		)`, body).Scan(&found)
	if err != nil {
		t.Fatalf("query: %v", err)
	}
	if found {
		t.Fatal("badan key mentah ditemukan tersimpan di database")
	}

	// last4 memang disimpan dan itu disengaja; pastikan hanya empat karakter itu.
	var stored string
	if err := pool.QueryRow(ctx, `select last4 from api_keys where id = $1`, created.Key.ID).Scan(&stored); err != nil {
		t.Fatalf("query last4: %v", err)
	}
	if stored != body[len(body)-4:] {
		t.Errorf("last4 tersimpan = %q, mau %q", stored, body[len(body)-4:])
	}
}

func TestAuthenticateRejections(t *testing.T) {
	anyIP := netip.MustParseAddr("203.0.113.9")

	// prepare menerima env subtest secara eksplisit; menangkap pool atau owner dari
	// luar akan menulis ke schema yang salah karena tiap subtest punya schema sendiri.
	tests := []struct {
		name       string
		params     CreateParams
		prepare    func(t *testing.T, ctx context.Context, pool *pgxpool.Pool, keyID, ownerID string)
		ip         netip.Addr
		wantReason string
	}{
		{
			name: "dicabut",
			prepare: func(t *testing.T, ctx context.Context, pool *pgxpool.Pool, keyID, _ string) {
				exec(t, ctx, pool, `update api_keys set status='revoked', revoked_at=now() where id=$1`, keyID)
			},
			ip: anyIP, wantReason: ReasonRevoked,
		},
		{
			name: "dimatikan",
			prepare: func(t *testing.T, ctx context.Context, pool *pgxpool.Pool, keyID, _ string) {
				exec(t, ctx, pool, `update api_keys set status='disabled' where id=$1`, keyID)
			},
			ip: anyIP, wantReason: ReasonDisabled,
		},
		{
			name:   "kedaluwarsa",
			params: CreateParams{ExpiresAt: ptr(time.Now().Add(-time.Hour))},
			ip:     anyIP, wantReason: ReasonExpired,
		},
		{
			name:   "IP di luar daftar putih",
			params: CreateParams{IPAllowlist: []netip.Prefix{netip.MustParsePrefix("10.0.0.0/8")}},
			ip:     anyIP, wantReason: ReasonIPNotAllowed,
		},
		{
			name: "diblokir lewat api_key",
			prepare: func(t *testing.T, ctx context.Context, pool *pgxpool.Pool, keyID, _ string) {
				exec(t, ctx, pool, `insert into bans (subject_kind, subject, reason) values ('api_key', $1, 'uji')`, keyID)
			},
			ip: anyIP, wantReason: ReasonBanned,
		},
		{
			name: "diblokir lewat pengguna",
			prepare: func(t *testing.T, ctx context.Context, pool *pgxpool.Pool, _, ownerID string) {
				exec(t, ctx, pool, `insert into bans (subject_kind, subject, reason) values ('user', $1, 'uji')`, ownerID)
			},
			ip: anyIP, wantReason: ReasonBanned,
		},
		{
			name: "diblokir lewat IP tunggal",
			prepare: func(t *testing.T, ctx context.Context, pool *pgxpool.Pool, _, _ string) {
				exec(t, ctx, pool, `insert into bans (subject_kind, subject, reason) values ('ip', '203.0.113.9', 'uji')`)
			},
			ip: anyIP, wantReason: ReasonBanned,
		},
		{
			name: "diblokir lewat rentang IP",
			prepare: func(t *testing.T, ctx context.Context, pool *pgxpool.Pool, _, _ string) {
				exec(t, ctx, pool, `insert into bans (subject_kind, subject, reason) values ('ip_range', '203.0.113.0/24', 'uji')`)
			},
			ip: anyIP, wantReason: ReasonBanned,
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			ctx, pool, r := testEnv(t)
			owner := newUser(t, ctx, pool)

			p := tc.params
			p.Name = "kunci uji"
			p.OwnerUserID = owner
			p.Live = true

			created := mustCreate(t, ctx, r, p)
			if tc.prepare != nil {
				tc.prepare(t, ctx, pool, created.Key.ID, owner)
			}

			_, err := r.Authenticate(ctx, created.Raw.Reveal(), tc.ip)
			assertRejection(t, err, tc.wantReason)
		})
	}
}

func TestAuthenticateUnknownKey(t *testing.T) {
	ctx, _, r := testEnv(t)

	// Key berbentuk sah tetapi tidak terdaftar.
	unknown, err := security.GenerateAPIKey(true, testPepper)
	if err != nil {
		t.Fatalf("GenerateAPIKey: %v", err)
	}
	_, err = r.Authenticate(ctx, unknown.Raw.Reveal(), netip.MustParseAddr("203.0.113.9"))
	assertRejection(t, err, ReasonNotFound)

	// Bentuk yang tidak sah ditolak tanpa menyentuh database.
	for _, bad := range []string{"", "bukan-key", "sk_live_pendek", strings.Repeat("x", 200)} {
		_, err := r.Authenticate(ctx, bad, netip.MustParseAddr("203.0.113.9"))
		assertRejection(t, err, ReasonNotFound)
	}
}

func TestIPAllowlistMatchesSubnet(t *testing.T) {
	ctx, pool, r := testEnv(t)
	owner := newUser(t, ctx, pool)

	created := mustCreate(t, ctx, r, CreateParams{
		Name: "k", OwnerUserID: owner, Live: true,
		IPAllowlist: []netip.Prefix{
			netip.MustParsePrefix("10.0.0.0/8"),
			netip.MustParsePrefix("192.168.1.7/32"),
		},
	})
	raw := created.Raw.Reveal()

	allowed := []string{"10.0.0.1", "10.255.255.254", "192.168.1.7"}
	denied := []string{"192.168.1.8", "203.0.113.9", "11.0.0.1"}

	for _, ip := range allowed {
		if _, err := r.Authenticate(ctx, raw, netip.MustParseAddr(ip)); err != nil {
			t.Errorf("IP %s seharusnya diizinkan: %v", ip, err)
		}
	}
	for _, ip := range denied {
		_, err := r.Authenticate(ctx, raw, netip.MustParseAddr(ip))
		assertRejection(t, err, ReasonIPNotAllowed)
	}
}

// Daftar putih kosong berarti semua diizinkan; aturan itu harus ditegakkan di query.
func TestAllowsModelAndProvider(t *testing.T) {
	ctx, pool, r := testEnv(t)
	owner := newUser(t, ctx, pool)

	modelA := newModel(t, ctx, pool, "model-a")
	modelB := newModel(t, ctx, pool, "model-b")
	providerA := newProvider(t, ctx, pool, "prov-a")
	providerB := newProvider(t, ctx, pool, "prov-b")

	t.Run("tanpa pembatasan berarti semua", func(t *testing.T) {
		created := mustCreate(t, ctx, r, CreateParams{Name: "bebas", OwnerUserID: owner, Live: true})
		for _, id := range []string{modelA, modelB} {
			if ok, err := r.AllowsModel(ctx, created.Key.ID, id); err != nil || !ok {
				t.Errorf("AllowsModel(%s) = %v, %v; mau true", id, ok, err)
			}
		}
		if ok, _ := r.AllowsProvider(ctx, created.Key.ID, providerB); !ok {
			t.Error("AllowsProvider seharusnya true tanpa pembatasan")
		}
	})

	t.Run("dengan pembatasan hanya yang terdaftar", func(t *testing.T) {
		created := mustCreate(t, ctx, r, CreateParams{
			Name: "terbatas", OwnerUserID: owner, Live: true,
			AllowedModelIDs:    []string{modelA},
			AllowedProviderIDs: []string{providerA},
		})
		if ok, _ := r.AllowsModel(ctx, created.Key.ID, modelA); !ok {
			t.Error("model yang diizinkan ditolak")
		}
		if ok, _ := r.AllowsModel(ctx, created.Key.ID, modelB); ok {
			t.Error("model di luar daftar putih diizinkan")
		}
		if ok, _ := r.AllowsProvider(ctx, created.Key.ID, providerA); !ok {
			t.Error("provider yang diizinkan ditolak")
		}
		if ok, _ := r.AllowsProvider(ctx, created.Key.ID, providerB); ok {
			t.Error("provider di luar daftar putih diizinkan")
		}
	})
}

func TestCreateRejectsUnknownReference(t *testing.T) {
	ctx, pool, r := testEnv(t)
	owner := newUser(t, ctx, pool)

	// Pemilik yang tidak ada.
	_, err := r.Create(ctx, CreateParams{
		Name: "k", OwnerUserID: "11111111-1111-1111-1111-111111111111", Live: true,
	})
	if !errors.Is(err, repo.ErrInvalidReference) {
		t.Errorf("pemilik tidak ada: err = %v, mau ErrInvalidReference", err)
	}

	// Model daftar putih yang tidak ada.
	_, err = r.Create(ctx, CreateParams{
		Name: "k", OwnerUserID: owner, Live: true,
		AllowedModelIDs: []string{"22222222-2222-2222-2222-222222222222"},
	})
	if !errors.Is(err, repo.ErrInvalidReference) {
		t.Errorf("model tidak ada: err = %v, mau ErrInvalidReference", err)
	}

	// Cakupan yang tidak dikenali ditolak constraint tabel.
	_, err = r.Create(ctx, CreateParams{
		Name: "k", OwnerUserID: owner, Live: true, Scopes: []string{"root:everything"},
	})
	if !errors.Is(err, repo.ErrConstraint) {
		t.Errorf("cakupan asing: err = %v, mau ErrConstraint", err)
	}
}

// Pesan error tidak boleh memuat hash maupun nilai baris.
func TestErrorsDoNotLeakRowValues(t *testing.T) {
	ctx, pool, r := testEnv(t)
	owner := newUser(t, ctx, pool)

	created := mustCreate(t, ctx, r, CreateParams{Name: "k", OwnerUserID: owner, Live: true})
	hash := security.HashAPIKey(created.Raw.Reveal(), testPepper)

	// Paksa pelanggaran keunikan pada key_hash.
	_, err := pool.Exec(ctx, `
		insert into api_keys (name, key_hash, key_prefix, last4, owner_user_id)
		values ('duplikat', $1, 'sk_live_', 'aaaa', $2)`, hash, owner)
	wrapped := repo.Err("menyisipkan duplikat", err)

	if !errors.Is(wrapped, repo.ErrConflict) {
		t.Fatalf("err = %v, mau ErrConflict", wrapped)
	}
	if strings.Contains(wrapped.Error(), hash) {
		t.Errorf("pesan error memuat hash key: %v", wrapped)
	}
}

func TestListKeysetPagination(t *testing.T) {
	ctx, pool, r := testEnv(t)
	owner := newUser(t, ctx, pool)

	const total = 12
	for i := 0; i < total; i++ {
		mustCreate(t, ctx, r, CreateParams{
			Name: fmt.Sprintf("kunci-%02d", i), OwnerUserID: owner, Live: true,
		})
		// created_at dipakai sebagai kursor, jadi urutannya harus bisa dibedakan.
		exec(t, ctx, pool, `update api_keys set created_at = now() - make_interval(secs => $1)
			where name = $2`, total-i, fmt.Sprintf("kunci-%02d", i))
	}

	seen := map[string]int{}
	cursor := ""
	pages := 0
	for {
		batch, next, err := r.List(ctx, ListFilter{OwnerUserID: owner}, repo.Page{Limit: 5, Cursor: cursor})
		if err != nil {
			t.Fatalf("List: %v", err)
		}
		for _, k := range batch {
			seen[k.ID]++
		}
		pages++
		if next == "" || pages > 10 {
			break
		}
		cursor = next
	}

	if len(seen) != total {
		t.Errorf("terlihat %d key unik, mau %d", len(seen), total)
	}
	for id, n := range seen {
		if n != 1 {
			t.Errorf("key %s muncul %d kali", id, n)
		}
	}
}

func TestTouchUsage(t *testing.T) {
	ctx, pool, r := testEnv(t)
	owner := newUser(t, ctx, pool)
	created := mustCreate(t, ctx, r, CreateParams{Name: "k", OwnerUserID: owner, Live: true})

	if err := r.TouchUsage(ctx, created.Key.ID, netip.MustParseAddr("203.0.113.9")); err != nil {
		t.Fatalf("TouchUsage: %v", err)
	}

	key, err := r.GetByID(ctx, created.Key.ID)
	if err != nil {
		t.Fatalf("GetByID: %v", err)
	}
	if key.LastUsedAt == nil {
		t.Error("LastUsedAt masih nil setelah TouchUsage")
	}
}

func TestNewRequiresPepper(t *testing.T) {
	for _, pepper := range [][]byte{nil, {}} {
		if _, err := New(nil, pepper); !errors.Is(err, security.ErrEmptyPepper) {
			t.Errorf("pepper %v: err = %v, mau ErrEmptyPepper", pepper, err)
		}
	}
}

// --- helper ---------------------------------------------------------------

func ptr[T any](v T) *T { return &v }

func exec(t *testing.T, ctx context.Context, pool *pgxpool.Pool, sql string, args ...any) {
	t.Helper()
	if _, err := pool.Exec(ctx, sql, args...); err != nil {
		t.Fatalf("exec: %v", err)
	}
}

func newModel(t *testing.T, ctx context.Context, pool *pgxpool.Pool, name string) string {
	t.Helper()
	var id string
	err := pool.QueryRow(ctx, `
		insert into models (model_id, display_name) values ($1, $1) returning id::text`, name).Scan(&id)
	if err != nil {
		t.Fatalf("membuat model: %v", err)
	}
	return id
}

func newProvider(t *testing.T, ctx context.Context, pool *pgxpool.Pool, name string) string {
	t.Helper()
	var id string
	err := pool.QueryRow(ctx, `
		insert into providers (name, display_name, kind, base_url)
		values ($1, $1, 'openai_compatible', 'https://upstream.test') returning id::text`, name).Scan(&id)
	if err != nil {
		t.Fatalf("membuat provider: %v", err)
	}
	return id
}

func assertRejection(t *testing.T, err error, wantReason string) {
	t.Helper()
	if err == nil {
		t.Fatalf("seharusnya ditolak dengan alasan %q", wantReason)
	}
	if !errors.Is(err, ErrKeyNotUsable) {
		t.Fatalf("err = %v, mau membungkus ErrKeyNotUsable", err)
	}
	var rej *Rejection
	if !errors.As(err, &rej) {
		t.Fatalf("err = %v, mau bertipe *Rejection", err)
	}
	if rej.Reason != wantReason {
		t.Errorf("alasan = %q, mau %q", rej.Reason, wantReason)
	}
}
