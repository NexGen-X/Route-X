package upstream

import (
	"bytes"
	"context"
	"crypto/rand"
	"encoding/hex"
	"errors"
	"fmt"
	"log/slog"
	"net/url"
	"os"
	"strings"
	"testing"
	"time"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"

	"github.com/NexGen-X/Route-X/internal/database"
	"github.com/NexGen-X/Route-X/internal/database/repo"
	"github.com/NexGen-X/Route-X/internal/security"
)

// schemaPrefix menandai schema sekali pakai milik paket ini, sehingga sisa schema yang
// tertinggal bisa dikenali dan dilaporkan tanpa mengganggu paket lain yang memakai
// database yang sama.
const schemaPrefix = "test_upstream_"

// TestMain menjalankan seluruh test lalu memastikan tidak ada schema sekali pakai yang
// tertinggal.
//
// Pemeriksaan ini ada karena test integrasi di sini membuat schema di database
// development: satu pembersihan yang terlewat akan menumpuk tanpa terlihat sampai
// database penuh dengan puluhan schema mati.
//
// Yang ditemukan hanya dilaporkan, tidak dihapus. Kalau paket ini kebetulan dijalankan
// dua kali bersamaan pada database yang sama, schema milik proses lain akan ikut terlihat
// di sini — dan menghapusnya berarti mematikan test yang sedang berjalan.
func TestMain(m *testing.M) {
	code := m.Run()

	if !testing.Short() {
		if left := leftoverSchemas(); len(left) > 0 {
			fmt.Fprintf(os.Stderr, "schema test tertinggal: %s\n", strings.Join(left, ", "))
			if code == 0 {
				code = 1
			}
		}
	}
	os.Exit(code)
}

// leftoverSchemas mengembalikan schema milik paket ini yang masih ada di database.
func leftoverSchemas() []string {
	dsn := ""
	for _, key := range []string{"TEST_DATABASE_URL", "DATABASE_URL"} {
		if v := strings.TrimSpace(os.Getenv(key)); v != "" {
			dsn = v
			break
		}
	}
	if dsn == "" {
		return nil
	}

	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()

	pool, err := pgxpool.New(ctx, dsn)
	if err != nil {
		return nil
	}
	defer pool.Close()

	rows, err := pool.Query(ctx,
		`select schema_name from information_schema.schemata where schema_name like $1`,
		schemaPrefix+"%")
	if err != nil {
		return nil
	}
	defer rows.Close()

	var out []string
	for rows.Next() {
		var name string
		if err := rows.Scan(&name); err != nil {
			return out
		}
		out = append(out, name)
	}
	return out
}

// --- Perkakas test integrasi -------------------------------------------------

// discardLogger membuang seluruh log supaya keluaran test tetap bersih.
func discardLogger() *slog.Logger { return slog.New(slog.DiscardHandler) }

// testDSN mengembalikan DSN untuk test integrasi, atau melewati test kalau tidak ada.
//
// TEST_DATABASE_URL diutamakan supaya CI bisa diarahkan ke database sekali pakai tanpa
// menyentuh database development.
func testDSN(t *testing.T) string {
	t.Helper()
	for _, key := range []string{"TEST_DATABASE_URL", "DATABASE_URL"} {
		dsn := strings.TrimSpace(os.Getenv(key))
		if dsn == "" {
			continue
		}
		if !strings.HasPrefix(dsn, "postgres://") && !strings.HasPrefix(dsn, "postgresql://") {
			t.Skipf("%s bukan URL postgres:// sehingga search_path test tidak bisa dipasang", key)
		}
		return dsn
	}
	t.Skip("TEST_DATABASE_URL maupun DATABASE_URL tidak diset — test integrasi dilewati")
	return ""
}

// newTestPool menyiapkan schema sekali pakai bermigrasi lengkap dan mengembalikan pool
// yang search_path-nya sudah mengarah ke sana.
//
// Seluruh tabel dibuat di dalam schema itu dan schema-nya dihapus di t.Cleanup, jadi test
// tidak pernah menyentuh schema public maupun data development. search_path dikirim
// sebagai parameter DSN karena pgx meneruskan parameter tak dikenal sebagai runtime
// parameter, sehingga setiap koneksi yang dibuka pool ikut terarahkan.
func newTestPool(ctx context.Context, t *testing.T) *pgxpool.Pool {
	t.Helper()
	baseDSN := testDSN(t)

	admin, err := pgxpool.New(ctx, baseDSN)
	if err != nil {
		t.Fatalf("membuka koneksi admin: %v", err)
	}
	t.Cleanup(admin.Close)

	buf := make([]byte, 6)
	if _, err := rand.Read(buf); err != nil {
		t.Fatalf("membuat nama schema acak: %v", err)
	}
	schema := schemaPrefix + hex.EncodeToString(buf)
	ident := pgx.Identifier{schema}.Sanitize()

	if _, err := admin.Exec(ctx, "create schema "+ident); err != nil {
		t.Fatalf("membuat schema %s: %v", schema, err)
	}
	t.Cleanup(func() {
		// Context baru: ctx test bisa sudah kedaluwarsa saat pembersihan jalan, dan schema
		// yang tertinggal akan mengotori database development.
		cleanupCtx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
		defer cancel()
		if _, err := admin.Exec(cleanupCtx, "drop schema "+ident+" cascade"); err != nil {
			t.Errorf("membersihkan schema %s: %v", schema, err)
		}
	})

	u, err := url.Parse(baseDSN)
	if err != nil {
		t.Fatalf("DSN test tidak bisa diparse: %v", err)
	}
	q := u.Query()
	q.Set("search_path", schema)
	u.RawQuery = q.Encode()

	pool, err := pgxpool.New(ctx, u.String())
	if err != nil {
		t.Fatalf("membuka pool test: %v", err)
	}
	t.Cleanup(pool.Close)

	if _, err := database.Migrate(ctx, &database.DB{Pool: pool}, discardLogger()); err != nil {
		t.Fatalf("menjalankan migrasi ke schema %s: %v", schema, err)
	}
	return pool
}

// testCipher membuat Cipher dari kunci yang bisa diulang. seed yang berbeda menghasilkan
// kunci berbeda, jadi rotasi kunci bisa diuji tanpa menebak nilai kunci.
func testCipher(t *testing.T, seed byte) *security.Cipher {
	t.Helper()
	cipher, err := security.NewCipher(bytes.Repeat([]byte{seed}, security.KeyLen))
	if err != nil {
		t.Fatalf("membuat cipher test: %v", err)
	}
	return cipher
}

// seedUser menyisipkan pengguna dan mengembalikan pengenalnya. Dipakai untuk kolom
// created_by dan untuk pemilik provider BYOK.
func seedUser(ctx context.Context, t *testing.T, q repo.Querier) string {
	t.Helper()
	buf := make([]byte, 6)
	if _, err := rand.Read(buf); err != nil {
		t.Fatalf("membuat email acak: %v", err)
	}

	var id string
	err := q.QueryRow(ctx, `
		insert into users (email, password_hash, display_name)
		values ($1, 'argon2id$palsu', 'Pengguna Test')
		returning id::text`, "u-"+hex.EncodeToString(buf)+"@contoh.test").Scan(&id)
	if err != nil {
		t.Fatalf("menyisipkan pengguna: %v", err)
	}
	return id
}

// seedProvider membuat provider dengan nilai wajar; name harus unik antar pemanggil.
func seedProvider(ctx context.Context, t *testing.T, q repo.Querier, name string) *Provider {
	t.Helper()
	provider, err := NewProviderRepo(q).Create(ctx, CreateProviderParams{
		Name:        name,
		DisplayName: strings.ToUpper(name),
		Kind:        KindOpenAICompatible,
		BaseURL:     "https://" + name + ".contoh.test/v1",
	})
	if err != nil {
		t.Fatalf("menyisipkan provider %s: %v", name, err)
	}
	return provider
}

// seedModel membuat model kanonik dengan kemampuan yang diminta.
func seedModel(ctx context.Context, t *testing.T, q repo.Querier, modelID string, capabilities ...string) *Model {
	t.Helper()
	model, err := NewModelRepo(q).Create(ctx, CreateModelParams{
		ModelID:      modelID,
		DisplayName:  strings.ToUpper(modelID),
		Capabilities: capabilities,
	})
	if err != nil {
		t.Fatalf("menyisipkan model %s: %v", modelID, err)
	}
	return model
}

// seedAttachment memetakan model ke provider.
func seedAttachment(ctx context.Context, t *testing.T, q repo.Querier, modelID, providerID string, priority *int) *ProviderModel {
	t.Helper()
	pm, err := NewModelRepo(q).AttachProvider(ctx, AttachProviderParams{
		ModelID:    modelID,
		ProviderID: providerID,
		Priority:   priority,
	})
	if err != nil {
		t.Fatalf("memetakan model ke provider: %v", err)
	}
	return pm
}

// explain menjalankan EXPLAIN untuk sebuah query dan mengembalikan rencananya sebagai
// teks satu blok.
//
// Dipakai untuk membuktikan dua query terpanas benar-benar dilayani indeks. Membaca
// rencana lebih jujur daripada mengukur waktu: pada tabel kecil, sequential scan bisa
// terlihat cepat padahal biayanya tumbuh seiring tabelnya.
func explain(ctx context.Context, t *testing.T, q repo.Querier, sql string, args ...any) string {
	t.Helper()

	rows, err := q.Query(ctx, "explain "+sql, args...)
	if err != nil {
		t.Fatalf("menjalankan explain: %v", err)
	}
	defer rows.Close()

	var plan strings.Builder
	for rows.Next() {
		var line string
		if err := rows.Scan(&line); err != nil {
			t.Fatalf("membaca baris rencana: %v", err)
		}
		plan.WriteString(line)
		plan.WriteString("\n")
	}
	if err := rows.Err(); err != nil {
		t.Fatalf("membaca rencana: %v", err)
	}
	return plan.String()
}

// analyzeTables memperbarui statistik perencana. Tanpa ini, perencana masih menganggap
// tabel yang baru diisi kosong dan memilih sequential scan untuk alasan yang tidak ada
// hubungannya dengan bentuk query.
func analyzeTables(ctx context.Context, t *testing.T, q repo.Querier, tables ...string) {
	t.Helper()
	for _, table := range tables {
		if _, err := q.Exec(ctx, "analyze "+pgx.Identifier{table}.Sanitize()); err != nil {
			t.Fatalf("analyze %s: %v", table, err)
		}
	}
}

// ptr mengembalikan pointer ke v, untuk mengisi field opsional di test.
func ptr[T any](v T) *T { return &v }

// seedBulkModels mengisi tabel models dengan banyak baris, lalu memperbarui statistik
// perencana.
//
// Diperlukan oleh test yang membuktikan pemakaian indeks: pada tabel kecil, sequential
// scan memang pilihan termurah, jadi rencana query di sana tidak membuktikan apa pun
// tentang bentuk querynya. Kemampuan "vision" sengaja dibuat jarang (lima baris) supaya
// penyaringan kemampuan benar-benar selektif, seperti keadaan sebenarnya.
func seedBulkModels(ctx context.Context, t *testing.T, q repo.Querier, modelCount int) {
	t.Helper()

	if _, err := q.Exec(ctx, `
		insert into models (model_id, display_name, capabilities, routing_priority)
		select 'bulk-m-' || i, 'Bulk Model ' || i,
			case when i <= 5 then array['text', 'vision'] else array['text'] end,
			100
		from generate_series(1, $1) i`, modelCount); err != nil {
		t.Fatalf("mengisi models massal: %v", err)
	}
	analyzeTables(ctx, t, q, "models")
}

// seedBulkRouting mengisi provider, model, dan pemetaannya untuk menguji jalur routing.
func seedBulkRouting(ctx context.Context, t *testing.T, q repo.Querier, providerCount, modelCount, perModel int) {
	t.Helper()

	if _, err := q.Exec(ctx, `
		insert into providers (name, display_name, kind, base_url, priority, weight)
		select 'bulk-p-' || i, 'Bulk Provider ' || i, 'openai_compatible',
			'https://bulk-' || i || '.contoh.test/v1', 100 + i, 1
		from generate_series(1, $1) i`, providerCount); err != nil {
		t.Fatalf("mengisi providers massal: %v", err)
	}
	seedBulkModels(ctx, t, q, modelCount)

	// Hanya beberapa provider per model, seperti keadaan sebenarnya: satu model dilayani
	// segelintir provider, bukan semuanya. Bentuk sebaran ini yang menentukan rencana
	// query — kalau setiap model dipetakan ke semua provider, membaca seluruh tabel
	// providers memang menjadi pilihan termurah dan test indeksnya berhenti membuktikan
	// apa pun.
	if _, err := q.Exec(ctx, `
		insert into provider_models (model_id, provider_id, upstream_model_name)
		select m.id, p.id, m.model_id
		from models m
		cross join lateral (
			select id from providers
			where name like 'bulk-p-%'
			order by md5(id::text || m.id::text)
			limit $1
		) p
		where m.model_id like 'bulk-m-%'`, perModel); err != nil {
		t.Fatalf("mengisi provider_models massal: %v", err)
	}

	analyzeTables(ctx, t, q, "providers", "models", "provider_models")
}

// --- Unit test perkakas paket ------------------------------------------------

// Pengenal salah bentuk harus ditolak sebelum menyentuh database, supaya artinya menjadi
// "tidak ada" dan bukan kegagalan internal.
func TestIDOK(t *testing.T) {
	valid, err := newID()
	if err != nil {
		t.Fatalf("newID: %v", err)
	}
	if !idOK(valid) {
		t.Errorf("idOK(%q) = false, ingin true", valid)
	}
	// Versi 4 dan varian RFC 4122 harus tertanam di tempat yang benar.
	if valid[14] != '4' {
		t.Errorf("nibble versi = %q, ingin '4'", valid[14])
	}
	if !strings.ContainsRune("89ab", rune(valid[19])) {
		t.Errorf("nibble varian = %q, ingin salah satu dari 8, 9, a, b", valid[19])
	}

	// Dua pengenal berurutan tidak boleh sama.
	other, err := newID()
	if err != nil {
		t.Fatalf("newID: %v", err)
	}
	if other == valid {
		t.Error("newID mengembalikan nilai yang sama dua kali")
	}

	for _, in := range []string{
		"",
		"bukan-uuid",
		"123",
		// Panjang benar tapi bukan heksadesimal.
		"zzzzzzzz-zzzz-zzzz-zzzz-zzzzzzzzzzzz",
		// Tanpa tanda hubung.
		"3f2504e04f8911d39a0c0305e82c3301",
		// Kelebihan satu karakter.
		valid + "0",
	} {
		if idOK(in) {
			t.Errorf("idOK(%q) = true, ingin false", in)
		}
	}
}

// Cuplikan kredensial harus mengenali awalan penyedia tanpa membuka bagian rahasianya.
func TestMaskCredential(t *testing.T) {
	for _, tc := range []struct {
		in   string
		want string
	}{
		{"sk-proj-rahasia-xyzzy-9a21", "sk-****9a21"},
		{"sk_live_0123456789abcdef9a21", "sk_****9a21"},
		{"AIzaSyAbcdefghijklmnop1234", "****1234"},
		// Nilai yang terlalu pendek untuk disamarkan dengan aman diganti bintang
		// seluruhnya oleh security.Mask.
		{"pendek", "******"},
		{"", "****"},
	} {
		t.Run(tc.in, func(t *testing.T) {
			got := maskCredential(security.Secret(tc.in))
			if got != tc.want {
				t.Errorf("maskCredential(%q) = %q, ingin %q", tc.in, got, tc.want)
			}
			// Cuplikan tidak boleh memuat bagian tengah kredensial.
			if len(tc.in) > 12 && strings.Contains(got, tc.in[4:12]) {
				t.Errorf("cuplikan %q membocorkan bagian tengah kredensial", got)
			}
		})
	}
}

// Cuplikan URL proxy harus menutup sandinya dan menyisakan host supaya masih bisa dikenali.
func TestMaskProxyURL(t *testing.T) {
	for _, tc := range []struct {
		in   string
		want string
	}{
		{"http://pengguna:sandi@proxy.test:3128", "http://pengguna:****@proxy.test:3128"},
		{"socks5://pengguna:sandi@proxy.test:1080", "socks5://pengguna:****@proxy.test:1080"},
		{"http://pengguna@proxy.test:3128", "http://pengguna@proxy.test:3128"},
		{"http://proxy.test:3128", "http://proxy.test:3128"},
		{"bukan url", "****"},
		{"", "****"},
	} {
		t.Run(tc.in, func(t *testing.T) {
			got := maskProxyURL(security.Secret(tc.in))
			if got != tc.want {
				t.Errorf("maskProxyURL(%q) = %q, ingin %q", tc.in, got, tc.want)
			}
			if strings.Contains(got, "sandi") {
				t.Errorf("cuplikan %q masih memuat sandi", got)
			}
		})
	}
}

// Kursor keyset harus bolak-balik utuh, termasuk bagian pengenalnya.
func TestCursorRoundTrip(t *testing.T) {
	ts := time.Date(2026, 9, 2, 10, 30, 0, 123456000, time.UTC)
	id := "3f2504e0-4f89-41d3-9a0c-0305e82c3301"

	cursor := encodeCursor(ts, id)
	gotTS, gotID, err := decodeCursor("uji", cursor)
	if err != nil {
		t.Fatalf("decodeCursor(%q): %v", cursor, err)
	}
	if !gotTS.Equal(ts) {
		t.Errorf("waktu = %s, ingin %s", gotTS, ts)
	}
	if gotID != id {
		t.Errorf("pengenal = %q, ingin %q", gotID, id)
	}

	for _, in := range []string{"", "tanpa-pemisah", "2026-09-02T10:30:00Z|", "bukan-waktu|" + id} {
		if _, _, err := decodeCursor("uji", in); !errors.Is(err, repo.ErrConstraint) {
			t.Errorf("decodeCursor(%q) = %v, ingin ErrConstraint", in, err)
		}
	}
}

// Perakit klausa SET hanya menyebut kolom yang benar-benar diminta berubah, dengan urutan
// yang tetap supaya teks SQL-nya stabil.
func TestUpdateSet(t *testing.T) {
	set := newUpdateSet("kunci")
	if !set.empty() {
		t.Error("perakit baru seharusnya kosong")
	}

	set.add("nama", "baru")
	addOpt(set, "wilayah", Clear[string]())
	addOpt(set, "bobot", Set(7))
	addOpt(set, "diabaikan", Opt[int]{})

	if set.empty() {
		t.Error("perakit seharusnya tidak kosong lagi")
	}
	want := "nama = $2, wilayah = $3, bobot = $4"
	if set.clause() != want {
		t.Errorf("clause() = %q, ingin %q", set.clause(), want)
	}
	if len(set.args) != 4 {
		t.Fatalf("jumlah argumen = %d, ingin 4", len(set.args))
	}
	if set.args[0] != "kunci" || set.args[1] != "baru" || set.args[2] != nil || set.args[3] != 7 {
		t.Errorf("argumen = %v", set.args)
	}
}

// Nilai bawaan hanya dipakai ketika pemanggil memang tidak mengisi apa pun.
func TestDefaultValues(t *testing.T) {
	if got := intOr(nil, 42); got != 42 {
		t.Errorf("intOr(nil, 42) = %d", got)
	}
	if got := intOr(ptr(0), 42); got != 0 {
		t.Errorf("intOr(ptr(0), 42) = %d, ingin 0 — nol adalah nilai yang sah", got)
	}
	if got := boolOr(nil, true); !got {
		t.Error("boolOr(nil, true) = false")
	}
	if got := boolOr(ptr(false), true); got {
		t.Error("boolOr(ptr(false), true) = true")
	}
	if got := strOr("  ", "bawaan"); got != "bawaan" {
		t.Errorf("strOr(spasi) = %q, ingin bawaan", got)
	}
	if got := nullIfEmpty("  "); got != nil {
		t.Errorf("nullIfEmpty(spasi) = %v, ingin nil", got)
	}
	if got := nullIfEmpty("isi"); got != "isi" {
		t.Errorf("nullIfEmpty = %v", got)
	}
}

// Setiap operasi yang menerima pengenal harus menolak pengenal salah bentuk dengan arti
// yang benar — "tidak ada" untuk baris yang dicari, "referensi tidak sah" untuk baris yang
// ditunjuk — dan tidak boleh menyentuh database sama sekali.
//
// Karena itu test ini memakai Querier nil: kalau ada satu jalur yang lolos ke database,
// test ini panik alih-alih diam-diam mengubah error menjadi kegagalan internal.
func TestMalformedIDRejectedWithoutDatabase(t *testing.T) {
	const malformed = "bukan-uuid"
	ctx := context.Background()
	cipher := testCipher(t, 0xF6)

	providers := NewProviderRepo(nil)
	models := NewModelRepo(nil)
	pricing := NewPricingRepo(nil)
	credentials, err := NewCredentialRepo(nil, cipher)
	if err != nil {
		t.Fatalf("NewCredentialRepo: %v", err)
	}
	egress, err := NewEgressRepo(nil, cipher)
	if err != nil {
		t.Fatalf("NewEgressRepo: %v", err)
	}

	notFound := map[string]func() error{
		"providers.Get":              func() error { _, err := providers.Get(ctx, malformed); return err },
		"providers.Update":           func() error { _, err := providers.Update(ctx, malformed, UpdateProviderParams{}); return err },
		"providers.Delete":           func() error { return providers.Delete(ctx, malformed) },
		"providers.RecordHealth":     func() error { return providers.RecordHealth(ctx, malformed, HealthReport{}) },
		"providers.ListHealthChecks": func() error { _, _, err := providers.ListHealthChecks(ctx, malformed, repo.Page{}); return err },
		"models.Get":                 func() error { _, err := models.Get(ctx, malformed); return err },
		"models.Update":              func() error { _, err := models.Update(ctx, malformed, UpdateModelParams{}); return err },
		"models.Delete":              func() error { return models.Delete(ctx, malformed) },
		"models.UpdateProviderModel": func() error {
			_, err := models.UpdateProviderModel(ctx, malformed, UpdateProviderModelParams{})
			return err
		},
		"models.DetachProvider":   func() error { return models.DetachProvider(ctx, malformed) },
		"models.GetProviderModel": func() error { _, err := models.GetProviderModel(ctx, malformed); return err },
		"credentials.Get":         func() error { _, err := credentials.Get(ctx, malformed); return err },
		"credentials.Reveal":      func() error { _, err := credentials.Reveal(ctx, malformed); return err },
		"credentials.SetEnabled":  func() error { _, err := credentials.SetEnabled(ctx, malformed, true); return err },
		"credentials.Delete":      func() error { return credentials.Delete(ctx, malformed) },
		"credentials.MarkUsed":    func() error { return credentials.MarkUsed(ctx, malformed) },
		"credentials.MarkAuthFailure": func() error {
			_, err := credentials.MarkAuthFailure(ctx, malformed)
			return err
		},
		"pricing.Current":     func() error { _, err := pricing.Current(ctx, malformed); return err },
		"pricing.At":          func() error { _, err := pricing.At(ctx, malformed, time.Now()); return err },
		"pricing.History":     func() error { _, _, err := pricing.History(ctx, malformed, repo.Page{}); return err },
		"egress.Get":          func() error { _, err := egress.Get(ctx, malformed); return err },
		"egress.ProxyURL":     func() error { _, err := egress.ProxyURL(ctx, malformed); return err },
		"egress.SetProxyURL":  func() error { _, err := egress.SetProxyURL(ctx, malformed, "http://a:b@c:1"); return err },
		"egress.Update":       func() error { _, err := egress.Update(ctx, malformed, UpdateEgressParams{}); return err },
		"egress.Delete":       func() error { return egress.Delete(ctx, malformed) },
		"egress.RecordHealth": func() error { return egress.RecordHealth(ctx, malformed, HealthHealthy, nil) },
	}
	for name, call := range notFound {
		t.Run(name, func(t *testing.T) {
			if err := call(); !errors.Is(err, repo.ErrNotFound) {
				t.Errorf("%s = %v, ingin ErrNotFound", name, err)
			}
		})
	}

	invalidRef := map[string]func() error{
		"providers.RouteCandidates": func() error {
			_, err := providers.RouteCandidates(ctx, RouteQuery{ModelID: malformed})
			return err
		},
		"providers.RouteCandidates pemilik": func() error {
			id, _ := newID()
			_, err := providers.RouteCandidates(ctx, RouteQuery{ModelID: id, OwnerUserID: malformed})
			return err
		},
		"providers.List pemilik": func() error {
			_, _, err := providers.List(ctx, ProviderFilter{OwnerUserID: malformed}, repo.Page{})
			return err
		},
		"models.AddAlias":    func() error { _, err := models.AddAlias(ctx, malformed, "alias", nil); return err },
		"models.ListAliases": func() error { _, err := models.ListAliases(ctx, malformed); return err },
		"models.AttachProvider": func() error {
			_, err := models.AttachProvider(ctx, AttachProviderParams{ModelID: malformed, ProviderID: malformed})
			return err
		},
		"models.ListProviderModels": func() error {
			_, err := models.ListProviderModels(ctx, ProviderModelFilter{ModelID: malformed})
			return err
		},
		"credentials.Create": func() error {
			_, err := credentials.Create(ctx, CreateCredentialParams{ProviderID: malformed, Secret: "sk-x"})
			return err
		},
		"credentials.Active": func() error { _, err := credentials.Active(ctx, malformed); return err },
		"credentials.List":   func() error { _, err := credentials.List(ctx, malformed); return err },
	}
	for name, call := range invalidRef {
		t.Run(name, func(t *testing.T) {
			if err := call(); !errors.Is(err, repo.ErrInvalidReference) {
				t.Errorf("%s = %v, ingin ErrInvalidReference", name, err)
			}
		})
	}
}

// Pengenal kunci aktif dilaporkan apa adanya, supaya dashboard bisa membandingkannya
// dengan sebaran kunci dari CountByKeyID.
func TestActiveKeyID(t *testing.T) {
	cipher := testCipher(t, 0xF7)
	credentials, err := NewCredentialRepo(nil, cipher)
	if err != nil {
		t.Fatalf("NewCredentialRepo: %v", err)
	}
	if credentials.ActiveKeyID() != cipher.KeyID() {
		t.Errorf("ActiveKeyID() = %q, ingin %q", credentials.ActiveKeyID(), cipher.KeyID())
	}
}
