package database

import (
	"context"
	"errors"
	"io/fs"
	"strings"
	"sync"
	"testing"
	"testing/fstest"
	"time"
)

// sqlFile membuat entri fstest dengan isi apa adanya.
func sqlFile(body string) *fstest.MapFile { return &fstest.MapFile{Data: []byte(body)} }

// versionsOf mengambil urutan nomor versi hasil parseMigrations.
func versionsOf(ms []migration) []int64 {
	out := make([]int64, len(ms))
	for i, m := range ms {
		out[i] = m.version
	}
	return out
}

func equalInt64s(a, b []int64) bool {
	if len(a) != len(b) {
		return false
	}
	for i := range a {
		if a[i] != b[i] {
			return false
		}
	}
	return true
}

// Urutan penerapan harus numerik. Ini bukan detail: fs.ReadDir mengembalikan nama
// terurut leksikografis, dan begitu jumlah digitnya berbeda urutan leksikografis
// menaruh 00010 sebelum 0002.
func TestParseMigrationsSortsNumerically(t *testing.T) {
	cases := []struct {
		name string
		fsys fstest.MapFS
		want []int64
	}{
		{
			name: "digit sama, urutan masuk acak",
			fsys: fstest.MapFS{
				"0010_ketiga.sql":  sqlFile("select 10;"),
				"0001_pertama.sql": sqlFile("select 1;"),
				"0002_kedua.sql":   sqlFile("select 2;"),
			},
			want: []int64{1, 2, 10},
		},
		{
			name: "jumlah digit berbeda tetap terurut angka",
			fsys: fstest.MapFS{
				"00010_sepuluh.sql": sqlFile("select 10;"),
				"0002_dua.sql":      sqlFile("select 2;"),
				"000003_tiga.sql":   sqlFile("select 3;"),
			},
			want: []int64{2, 3, 10},
		},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			got, err := parseMigrations(tc.fsys)
			if err != nil {
				t.Fatalf("parseMigrations: %v", err)
			}
			if !equalInt64s(versionsOf(got), tc.want) {
				t.Errorf("urutan versi = %v, ingin %v", versionsOf(got), tc.want)
			}
		})
	}
}

// Nama file adalah kontrak: nomor versi dan nama migrasi ikut tercatat di pembukuan,
// jadi format yang meleset harus ditolak saat parse, bukan menghasilkan versi aneh.
func TestParseMigrationsRejectsInvalidNames(t *testing.T) {
	cases := []struct {
		name  string
		entry string
	}{
		{"digit kurang dari empat", "1_bootstrap.sql"},
		{"tanpa nomor versi", "bootstrap.sql"},
		{"pemisah bukan garis bawah", "0001-bootstrap.sql"},
		{"tanpa nama", "0001_.sql"},
		{"huruf kapital", "0001_Bootstrap.sql"},
		{"garis bawah ganda", "0001_boot__strap.sql"},
		{"ekstensi salah", "0001_bootstrap.txt"},
		{"tanpa ekstensi", "0001_bootstrap"},
		{"spasi di nama", "0001_boot strap.sql"},
		{"versi nol", "0000_bootstrap.sql"},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			_, err := parseMigrations(fstest.MapFS{tc.entry: sqlFile("select 1;")})
			if !errors.Is(err, ErrInvalidMigration) {
				t.Errorf("parseMigrations(%q) = %v, ingin ErrInvalidMigration", tc.entry, err)
			}
		})
	}
}

// Dua file dengan nomor sama berarti dua branch memakai versi yang sama; kalau
// dibiarkan, urutan penerapannya bergantung pada nasib dan hanya satu yang tercatat.
func TestParseMigrationsRejectsDuplicateVersion(t *testing.T) {
	_, err := parseMigrations(fstest.MapFS{
		"0001_bootstrap.sql": sqlFile("select 1;"),
		"0001_users.sql":     sqlFile("select 2;"),
	})
	if !errors.Is(err, ErrInvalidMigration) {
		t.Fatalf("err = %v, ingin ErrInvalidMigration", err)
	}
	if !strings.Contains(err.Error(), "dipakai dua kali") {
		t.Errorf("pesan tidak menjelaskan versi ganda: %v", err)
	}
}

func TestParseMigrationsRejectsEmptyInput(t *testing.T) {
	cases := map[string]fs.FS{
		"direktori kosong": fstest.MapFS{},
		"file tanpa isi":   fstest.MapFS{"0001_bootstrap.sql": sqlFile("")},
		"file hanya spasi": fstest.MapFS{"0001_bootstrap.sql": sqlFile("\n\t  \n")},
		"ada subdirektori": fstest.MapFS{"0001_bootstrap.sql": sqlFile("select 1;"), "subdir/0002_x.sql": sqlFile("select 2;")},
	}

	for name, fsys := range cases {
		t.Run(name, func(t *testing.T) {
			if _, err := parseMigrations(fsys); !errors.Is(err, ErrInvalidMigration) {
				t.Errorf("parseMigrations() = %v, ingin ErrInvalidMigration", err)
			}
		})
	}
}

// Nama migrasi yang diurai dipakai apa adanya di pembukuan, jadi ikut diperiksa.
func TestParseMigrationsExtractsNameAndChecksum(t *testing.T) {
	const body = "create table t (id int);\n"
	got, err := parseMigrations(fstest.MapFS{"0007_create_settings_table.sql": sqlFile(body)})
	if err != nil {
		t.Fatalf("parseMigrations: %v", err)
	}
	m := got[0]
	if m.version != 7 {
		t.Errorf("version = %d, ingin 7", m.version)
	}
	if m.name != "create_settings_table" {
		t.Errorf("name = %q, ingin %q", m.name, "create_settings_table")
	}
	if m.filename != "0007_create_settings_table.sql" {
		t.Errorf("filename = %q", m.filename)
	}
	if m.sql != body {
		t.Errorf("sql = %q, ingin isi file apa adanya", m.sql)
	}
	if m.checksum != checksum([]byte(body)) {
		t.Errorf("checksum tidak dihitung dari isi file")
	}
}

// Checksum harus stabil antar proses dan antar versi biner: nilainya tersimpan di
// database, jadi perubahan cara menghitungnya akan membuat semua deploy lama gagal.
func TestChecksumStable(t *testing.T) {
	// SHA-256 dari "select 1;\n", dihitung di luar Go (sha256sum) supaya test ini
	// benar-benar mengunci algoritma dan formatnya, bukan cuma mengulang kodenya.
	const (
		body = "select 1;\n"
		want = "4a45092ccf992ea92250053a80b931b787924ba61648f420555511b84f10ab6c"
	)
	got := checksum([]byte(body))
	if len(got) != 64 {
		t.Errorf("panjang checksum = %d, ingin 64 karakter heksadesimal", len(got))
	}
	if got != want {
		t.Errorf("checksum(%q) = %q, ingin %q", body, got, want)
	}
	if checksum([]byte(body)) != got {
		t.Error("checksum tidak stabil untuk isi yang sama")
	}
	// Perbedaan sekecil spasi pun harus terdeteksi: itu inti pemeriksaan riwayat.
	if checksum([]byte("select 1; \n")) == got {
		t.Error("checksum tidak berubah padahal isi berubah")
	}
}

// Migrasi yang tertanam di biner harus lolos aturan yang sama dengan yang kita
// paksakan ke orang lain. Test ini gagal begitu ada file migrasi baru yang namanya
// salah, tanpa perlu database.
func TestEmbeddedMigrations(t *testing.T) {
	ms, err := embeddedMigrations()
	if err != nil {
		t.Fatalf("embeddedMigrations: %v", err)
	}
	if len(ms) == 0 {
		t.Fatal("tidak ada migrasi yang tertanam")
	}
	if ms[0].version != 1 || ms[0].name != "bootstrap" {
		t.Errorf("migrasi pertama = %d_%s, ingin 1_bootstrap", ms[0].version, ms[0].name)
	}
	if !strings.Contains(ms[0].sql, "settings") {
		t.Error("migrasi bootstrap tidak membuat tabel settings")
	}
	// Nomor versi harus naik ketat setelah diurutkan.
	for i := 1; i < len(ms); i++ {
		if ms[i].version <= ms[i-1].version {
			t.Errorf("versi tidak naik: %d setelah %d", ms[i].version, ms[i-1].version)
		}
	}
}

// --- Test integrasi ----------------------------------------------------------
//
// Semua test di bawah butuh PostgreSQL dan berjalan di schema sekali pakai; lihat
// newTestSchema di db_test.go.

// Migrasi dari nol: semua file diterapkan, pemanggilan kedua tidak mengubah apa pun,
// tabelnya benar-benar terbentuk, dan riwayat yang diedit ditolak.
func TestMigrateIntegration(t *testing.T) {
	if testing.Short() {
		t.Skip("butuh PostgreSQL")
	}
	baseDSN := testDSN(t)

	ctx, cancel := context.WithTimeout(context.Background(), 90*time.Second)
	defer cancel()

	dsn, schema := newTestSchema(ctx, t, baseDSN)

	db, err := Connect(ctx, testConfig(dsn), discardLogger())
	if err != nil {
		t.Fatalf("Connect: %v", err)
	}
	defer db.Close()

	files, err := embeddedMigrations()
	if err != nil {
		t.Fatalf("embeddedMigrations: %v", err)
	}
	tablesBefore := countPublicTables(ctx, t, db)

	applied, err := Migrate(ctx, db, discardLogger())
	if err != nil {
		t.Fatalf("Migrate: %v", err)
	}
	if applied != len(files) {
		t.Fatalf("applied = %d, ingin %d (satu per file migrasi)", applied, len(files))
	}

	t.Run("pemanggilan kedua idempoten", func(t *testing.T) {
		applied, err := Migrate(ctx, db, discardLogger())
		if err != nil {
			t.Fatalf("Migrate kedua: %v", err)
		}
		if applied != 0 {
			t.Errorf("applied = %d, ingin 0", applied)
		}
	})

	t.Run("pembukuan tercatat", func(t *testing.T) {
		var (
			name, sum string
			appliedAt time.Time
		)
		err := db.Pool.QueryRow(ctx,
			`select name, checksum, applied_at from schema_migrations where version = $1`,
			files[0].version).Scan(&name, &sum, &appliedAt)
		if err != nil {
			t.Fatalf("membaca schema_migrations: %v", err)
		}
		if name != files[0].name {
			t.Errorf("name = %q, ingin %q", name, files[0].name)
		}
		if sum != files[0].checksum {
			t.Errorf("checksum tercatat tidak sama dengan checksum file")
		}
		if appliedAt.IsZero() {
			t.Error("applied_at kosong")
		}
	})

	t.Run("tabel settings terbentuk sesuai rancangan", func(t *testing.T) {
		type column struct {
			dataType string
			nullable bool
		}
		want := map[string]column{
			"key":         {"text", false},
			"value":       {"jsonb", false},
			"description": {"text", true},
			"updated_at":  {"timestamp with time zone", false},
			"updated_by":  {"uuid", true},
		}

		rows, err := db.Pool.Query(ctx, `
			select column_name, data_type, is_nullable = 'YES'
			from information_schema.columns
			where table_schema = $1 and table_name = 'settings'`, schema)
		if err != nil {
			t.Fatalf("membaca information_schema: %v", err)
		}
		defer rows.Close()

		got := map[string]column{}
		for rows.Next() {
			var (
				name string
				c    column
			)
			if err := rows.Scan(&name, &c.dataType, &c.nullable); err != nil {
				t.Fatalf("scan kolom: %v", err)
			}
			got[name] = c
		}
		if err := rows.Err(); err != nil {
			t.Fatalf("membaca kolom: %v", err)
		}

		if len(got) != len(want) {
			t.Errorf("jumlah kolom = %d (%v), ingin %d", len(got), got, len(want))
		}
		for name, wantCol := range want {
			gotCol, ok := got[name]
			if !ok {
				t.Errorf("kolom %q tidak ada", name)
				continue
			}
			if gotCol != wantCol {
				t.Errorf("kolom %q = %+v, ingin %+v", name, gotCol, wantCol)
			}
		}

		var pk string
		err = db.Pool.QueryRow(ctx, `
			select a.attname
			from pg_index i
			join pg_attribute a on a.attrelid = i.indrelid and a.attnum = any (i.indkey)
			where i.indrelid = format('%I.settings', $1::text)::regclass and i.indisprimary`, schema).Scan(&pk)
		if err != nil {
			t.Fatalf("membaca primary key: %v", err)
		}
		if pk != "key" {
			t.Errorf("primary key = %q, ingin %q", pk, "key")
		}
	})

	t.Run("nilai jsonb bisa disimpan", func(t *testing.T) {
		if _, err := db.Pool.Exec(ctx,
			`insert into settings (key, value, description) values ($1, $2, $3)`,
			"ratelimit.default_rpm", []byte(`{"limit":600,"burst":60}`), "Batas laju bawaan"); err != nil {
			t.Fatalf("menyimpan setting: %v", err)
		}
		var limit int
		if err := db.Pool.QueryRow(ctx,
			`select (value->>'limit')::int from settings where key = $1`,
			"ratelimit.default_rpm").Scan(&limit); err != nil {
			t.Fatalf("membaca setting: %v", err)
		}
		if limit != 600 {
			t.Errorf("value->>'limit' = %d, ingin 600", limit)
		}
		// Kunci kosong harus ditolak constraint.
		if _, err := db.Pool.Exec(ctx, `insert into settings (key, value) values ('  ', '1'::jsonb)`); err == nil {
			t.Error("kunci kosong diterima, padahal ada constraint settings_key_not_empty")
		}
	})

	t.Run("schema public tidak tersentuh", func(t *testing.T) {
		if after := countPublicTables(ctx, t, db); after != tablesBefore {
			t.Errorf("jumlah tabel di schema public berubah dari %d ke %d — migrasi test bocor keluar schema sekali pakai",
				tablesBefore, after)
		}
	})

	// Sengaja subtest terakhir: pembukuan dirusak dan tidak dibereskan lagi.
	t.Run("checksum yang berubah menghentikan migrasi", func(t *testing.T) {
		if _, err := db.Pool.Exec(ctx,
			`update schema_migrations set checksum = 'sudah-diedit' where version = $1`,
			files[0].version); err != nil {
			t.Fatalf("merusak pembukuan: %v", err)
		}

		applied, err := Migrate(ctx, db, discardLogger())
		if !errors.Is(err, ErrChecksumMismatch) {
			t.Fatalf("Migrate = %v, ingin ErrChecksumMismatch", err)
		}
		if applied != 0 {
			t.Errorf("applied = %d, ingin 0 saat checksum tidak cocok", applied)
		}
		if !strings.Contains(err.Error(), files[0].filename) {
			t.Errorf("pesan error tidak menyebut file yang bermasalah: %v", err)
		}
	})
}

// Beberapa instance aplikasi yang start bersamaan tidak boleh saling menimpa:
// advisory lock harus membuat setiap migrasi diterapkan tepat sekali.
func TestMigrateConcurrentIntegration(t *testing.T) {
	if testing.Short() {
		t.Skip("butuh PostgreSQL")
	}
	baseDSN := testDSN(t)

	ctx, cancel := context.WithTimeout(context.Background(), 90*time.Second)
	defer cancel()

	dsn, _ := newTestSchema(ctx, t, baseDSN)

	files, err := embeddedMigrations()
	if err != nil {
		t.Fatalf("embeddedMigrations: %v", err)
	}

	// Tiap "instance" memakai pool sendiri supaya advisory lock benar-benar diuji
	// antar session, bukan antar goroutine yang berbagi koneksi.
	const instances = 4
	dbs := make([]*DB, instances)
	for i := range dbs {
		cfg := testConfig(dsn)
		cfg.DBMaxConns = 2
		cfg.DBMinConns = 0

		db, err := Connect(ctx, cfg, discardLogger())
		if err != nil {
			t.Fatalf("Connect instance %d: %v", i, err)
		}
		t.Cleanup(db.Close)
		dbs[i] = db
	}

	var (
		wg    sync.WaitGroup
		mu    sync.Mutex
		total int
		errs  []error
	)
	for _, db := range dbs {
		wg.Add(1)
		go func() {
			defer wg.Done()
			applied, err := Migrate(ctx, db, discardLogger())

			mu.Lock()
			defer mu.Unlock()
			total += applied
			if err != nil {
				errs = append(errs, err)
			}
		}()
	}
	wg.Wait()

	for _, err := range errs {
		t.Errorf("Migrate bersamaan gagal: %v", err)
	}
	if total != len(files) {
		t.Errorf("total migrasi diterapkan = %d, ingin %d — advisory lock tidak menjaga eksklusivitas",
			total, len(files))
	}
}
