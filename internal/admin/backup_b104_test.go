package admin

// Test regresi B-104 — restore dari panel admin pada instalasi standar.
//
// Temuan Langkah 7: mekanisme skratch-swap (B-045) menuntut hak CREATEDB —
// CREATE DATABASE skratch dan ALTER DATABASE ... RENAME saat penukaran
// sama-sama mensyaratkannya di PostgreSQL — tetapi role aplikasi hasil
// installer native lahir TANPA hak itu, jadi restore selalu gagal 42501
// sementara ekspor bekerja. Perbaikannya ada di installer (role aplikasi
// diberi CREATEDB, diselaraskan pada setiap lari ulang), dan test ini
// membuktikan kontrak haknya dari sisi produk:
//
//  1. Role terbatas persis hasil installer LAMA (pemilik database, tanpa
//     CREATEDB, tanpa SUPERUSER/CREATEROLE) bisa mengekspor, tetapi
//     restore-nya gagal di langkah pembuatan skratch dengan 42501 dan
//     produksi tidak berubah.
//  2. Sesudah ALTER ... WITH CREATEDB persis baris installer baru
//     diterapkan, restore yang sama berhasil ujung-ke-ujung lewat handler
//     HTTP produk: skratch tertukar, database pengaman berisi keadaan
//     pra-restore, versi skema cocok, dan role TETAP bukan superuser —
//     CREATEDB adalah satu-satunya hak tambahan yang dibutuhkan jalur ini.
//
// Pool aplikasi di seluruh test ini adalah role terbatas itu; koneksi
// superuser (DSN uji) hanya dipakai untuk menyiapkan/membersihkan
// lingkungan dan menerapkan baris installer, sama seperti operator
// menjalankan sudo -u postgres saat instalasi.

import (
	"bytes"
	"context"
	"crypto/rand"
	"encoding/hex"
	"fmt"
	"io"
	"log/slog"
	"mime/multipart"
	"net/http"
	"net/http/httptest"
	"net/url"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/jackc/pgx/v5/pgxpool"

	"github.com/NexGen-X/Route-X/internal/database"
)

// kataSandiRoleB104 adalah placeholder uji mengikuti konvensi GANTI_... repo.
const kataSandiRoleB104 = "GANTI_b104_kata_sandi_uji"

type lingkunganB104 struct {
	t        *testing.T
	namaDB   string
	pengaman string
	role     string
	pool     *pgxpool.Pool // pool APLIKASI: role terbatas, bukan superuser
	maint    *pgxpool.Pool // pool operator (DSN uji) untuk setup/pembersihan
	handler  *Handlers
}

func siapkanLingkunganB104(t *testing.T) *lingkunganB104 {
	t.Helper()
	if testing.Short() {
		t.Skip("melewati test integrasi restore pada -short")
	}
	dsn := ""
	for _, key := range []string{"TEST_DATABASE_URL", "DATABASE_URL"} {
		if v := strings.TrimSpace(os.Getenv(key)); v != "" {
			dsn = v
			break
		}
	}
	if dsn == "" {
		t.Skip("TEST_DATABASE_URL atau DATABASE_URL belum diatur")
	}
	for _, tool := range []string{"pg_dump", "psql"} {
		if _, err := exec.LookPath(tool); err != nil {
			t.Skipf("%s tidak ada di PATH — test restore membutuhkan perangkat klien PostgreSQL", tool)
		}
	}

	ctx := context.Background()
	u, err := url.Parse(dsn)
	if err != nil {
		t.Fatalf("parse DSN: %v", err)
	}
	u.RawQuery = ""

	var acak [4]byte
	if _, err := rand.Read(acak[:]); err != nil {
		t.Fatalf("rand: %v", err)
	}
	suffix := hex.EncodeToString(acak[:])
	role := "test_b104_" + suffix
	namaDB := "test_b104_" + suffix

	u.Path = "/postgres"
	maint, err := pgxpool.New(ctx, u.String())
	if err != nil {
		t.Fatalf("pool pemeliharaan: %v", err)
	}

	// Role persis hasil installer native LAMA: LOGIN + password, tanpa
	// satu pun hak istimewa (bawaan PostgreSQL: NOCREATEDB NOSUPERUSER
	// NOCREATEROLE) — keadaan yang melahirkan B-104.
	if _, err := maint.Exec(ctx, fmt.Sprintf(
		"CREATE ROLE %s LOGIN PASSWORD '%s'", pgxKutipIdent(role), kataSandiRoleB104)); err != nil {
		maint.Close()
		t.Fatalf("membuat role terbatas: %v", err)
	}
	if _, err := maint.Exec(ctx, "CREATE DATABASE "+pgxKutipIdent(namaDB)+" OWNER "+pgxKutipIdent(role)); err != nil {
		maint.Close()
		t.Fatalf("membuat database uji: %v", err)
	}

	// Pool aplikasi terhubung SEBAGAI role terbatas itu.
	u.Path = "/" + namaDB
	u.User = url.UserPassword(role, kataSandiRoleB104)
	pool, err := pgxpool.New(ctx, u.String())
	if err != nil {
		maint.Close()
		t.Fatalf("pool aplikasi role terbatas: %v", err)
	}

	env := &lingkunganB104{
		t:        t,
		namaDB:   namaDB,
		pengaman: namaDatabasePengaman(namaDB),
		role:     role,
		pool:     pool,
		maint:    maint,
		handler: &Handlers{
			pool:   pool,
			logger: slog.New(slog.NewTextHandler(io.Discard, nil)),
		},
	}
	t.Cleanup(func() {
		pool.Close()
		bctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
		defer cancel()
		_ = jatuhkanDatabase(bctx, maint, namaDB)
		_ = jatuhkanDatabase(bctx, maint, env.pengaman)
		_, _ = maint.Exec(bctx, "DROP ROLE IF EXISTS "+pgxKutipIdent(role))
		maint.Close()
	})

	// Migrasi penuh berjalan sebagai role terbatas (pemilik database) —
	// sama seperti boot gateway pada instalasi native nyata.
	if _, err := database.Migrate(ctx, &database.DB{Pool: pool}, env.handler.logger); err != nil {
		t.Fatalf("migrasi database uji sebagai role terbatas: %v", err)
	}
	return env
}

// hakRole membaca bendera hak role uji dari katalog.
func (e *lingkunganB104) hakRole() (createdb, superuser, createrole bool) {
	e.t.Helper()
	err := e.maint.QueryRow(context.Background(),
		`SELECT rolcreatedb, rolsuper, rolcreaterole FROM pg_roles WHERE rolname = $1`, e.role).
		Scan(&createdb, &superuser, &createrole)
	if err != nil {
		e.t.Fatalf("membaca hak role: %v", err)
	}
	return createdb, superuser, createrole
}

func (e *lingkunganB104) tanamPenanda(isi string) {
	e.t.Helper()
	ctx := context.Background()
	if _, err := e.pool.Exec(ctx, `CREATE TABLE IF NOT EXISTS penanda (isi text)`); err != nil {
		e.t.Fatalf("membuat tabel penanda: %v", err)
	}
	if _, err := e.pool.Exec(ctx, `INSERT INTO penanda VALUES ($1)`, isi); err != nil {
		e.t.Fatalf("menanam penanda %q: %v", isi, err)
	}
}

// isiPenanda membaca penanda dari database bernama target memakai pool
// BARU sebagai role terbatas (pool lama bisa terputus oleh penukaran).
func (e *lingkunganB104) isiPenanda(database string) []string {
	e.t.Helper()
	ctx := context.Background()
	cfg := e.pool.Config().Copy()
	cfg.ConnConfig.Database = database
	cfg.MinConns = 0
	pool, err := pgxpool.NewWithConfig(ctx, cfg)
	if err != nil {
		e.t.Fatalf("pool pembaca %s: %v", database, err)
	}
	defer pool.Close()
	rows, err := pool.Query(ctx, `SELECT isi FROM penanda ORDER BY isi`)
	if err != nil {
		e.t.Fatalf("membaca penanda di %s: %v", database, err)
	}
	defer rows.Close()
	var hasil []string
	for rows.Next() {
		var s string
		if err := rows.Scan(&s); err != nil {
			e.t.Fatalf("scan penanda: %v", err)
		}
		hasil = append(hasil, s)
	}
	return hasil
}

func (e *lingkunganB104) panggilRestoreHTTP(namaFile string, isi []byte) *httptest.ResponseRecorder {
	e.t.Helper()
	var buf bytes.Buffer
	mw := multipart.NewWriter(&buf)
	part, err := mw.CreateFormFile("file", namaFile)
	if err != nil {
		e.t.Fatalf("membuat form file: %v", err)
	}
	if _, err := part.Write(isi); err != nil {
		e.t.Fatalf("menulis form file: %v", err)
	}
	if err := mw.Close(); err != nil {
		e.t.Fatalf("menutup multipart: %v", err)
	}
	req := httptest.NewRequest(http.MethodPost, "/api/admin/system/backup/restore", &buf)
	req.Header.Set("Content-Type", mw.FormDataContentType())
	rec := httptest.NewRecorder()
	e.handler.restoreDatabaseSQL(rec, req)
	return rec
}

func TestRestoreB104RoleAplikasiButuhCreatedb(t *testing.T) {
	env := siapkanLingkunganB104(t)
	ctx := context.Background()

	// Keadaan awal persis instalasi standar pra-perbaikan.
	if createdb, superuser, createrole := env.hakRole(); createdb || superuser || createrole {
		t.Fatalf("keadaan awal role tidak sesuai instalasi lama: createdb=%v super=%v createrole=%v",
			createdb, superuser, createrole)
	}

	// Data + cadangan lewat jalur ekspor produk (sebagai role terbatas:
	// ekspor memang selalu bekerja, inilah asimetri B-104).
	env.tanamPenanda("AWAL")
	reqExp := httptest.NewRequest(http.MethodPost, "/api/admin/system/backup/export?format=sql", nil)
	recExp := httptest.NewRecorder()
	env.handler.exportDatabaseSQL(recExp, reqExp)
	if recExp.Code != http.StatusOK {
		t.Fatalf("ekspor status = %d. body: %s", recExp.Code, recExp.Body.String())
	}
	dump := recExp.Body.Bytes()
	if !strings.Contains(string(dump), "PostgreSQL database dump") {
		t.Fatalf("hasil ekspor bukan dump PostgreSQL")
	}
	env.tanamPenanda("SESUDAH_BACKUP")

	// (1) SEBELUM perbaikan: restore gagal di pembuatan database skratch
	// dengan 42501, persis kegagalan drill Langkah 7 di produksi.
	berkasDump := filepath.Join(t.TempDir(), "routex_backup.sql")
	if err := os.WriteFile(berkasDump, dump, 0o600); err != nil {
		t.Fatalf("menulis dump uji: %v", err)
	}
	_, errRestore := env.handler.jalankanRestore(ctx, berkasDump, env.pool.Config().ConnString())
	if errRestore == nil {
		t.Fatalf("restore tanpa CREATEDB ternyata berhasil; kontrak hak B-104 berubah")
	}
	if !strings.Contains(errRestore.Error(), "42501") &&
		!strings.Contains(errRestore.Error(), "permission denied to create database") {
		t.Fatalf("error restore = %v, mau kegagalan hak membuat database (42501)", errRestore)
	}
	// Lewat HTTP handler: 400 restore_failed generik, produksi utuh, dan
	// tidak ada database pengaman/skratch yang tertinggal.
	rec := env.panggilRestoreHTTP("routex_backup.sql", dump)
	if rec.Code != http.StatusBadRequest || !strings.Contains(rec.Body.String(), "restore_failed") {
		t.Fatalf("restore HTTP status = %d, mau 400 restore_failed. body: %s", rec.Code, rec.Body.String())
	}
	if isi := env.isiPenanda(env.namaDB); len(isi) != 2 {
		t.Fatalf("produksi berubah sesudah restore gagal: penanda = %v, mau 2 baris", isi)
	}
	var dbLiar int
	if err := env.maint.QueryRow(ctx,
		`SELECT count(*) FROM pg_database WHERE datname = $1 OR datname LIKE 'routex_restore_%'`,
		env.pengaman).Scan(&dbLiar); err != nil {
		t.Fatalf("memeriksa database liar: %v", err)
	}
	if dbLiar != 0 {
		t.Fatalf("ada %d database pengaman/skratch tertinggal sesudah restore gagal", dbLiar)
	}

	// Terapkan perbaikan installer persis seperti baris barunya: ALTER
	// yang berjalan pada SETIAP lari installer (instalasi lama sembuh
	// lewat lari ulang), hanya menambah CREATEDB.
	if _, err := env.maint.Exec(ctx, fmt.Sprintf(
		"ALTER USER %s WITH CREATEDB PASSWORD '%s'", pgxKutipIdent(env.role), kataSandiRoleB104)); err != nil {
		t.Fatalf("menerapkan ALTER installer B-104: %v", err)
	}
	if createdb, superuser, createrole := env.hakRole(); !createdb || superuser || createrole {
		t.Fatalf("sesudah perbaikan hak role salah: createdb=%v super=%v createrole=%v, mau (true,false,false)",
			createdb, superuser, createrole)
	}

	// (2) SESUDAH perbaikan: restore yang sama berhasil ujung-ke-ujung.
	rec = env.panggilRestoreHTTP("routex_backup.sql", dump)
	if rec.Code != http.StatusOK {
		t.Fatalf("restore sesudah CREATEDB status = %d. body: %s", rec.Code, rec.Body.String())
	}
	if !strings.Contains(rec.Body.String(), `"safety_database":"`+env.pengaman+`"`) {
		t.Errorf("respons tidak menyebut database pengaman %s: %s", env.pengaman, rec.Body.String())
	}
	terkini, err := database.LatestMigrationVersion()
	if err != nil {
		t.Fatalf("membaca versi migrasi biner: %v", err)
	}
	if !strings.Contains(rec.Body.String(), fmt.Sprintf(`"schema_version":%d`, terkini)) {
		t.Errorf("schema_version pada respons tidak cocok dengan versi biner %d: %s", terkini, rec.Body.String())
	}

	// Data utuh: produksi kembali ke keadaan cadangan, keadaan sesudah
	// cadangan utuh di database pengaman, ledger skema cocok.
	if isi := env.isiPenanda(env.namaDB); len(isi) != 1 || isi[0] != "AWAL" {
		t.Errorf("penanda produksi = %v, mau [AWAL]", isi)
	}
	if isi := env.isiPenanda(env.pengaman); len(isi) != 2 {
		t.Errorf("penanda pengaman = %v, mau 2 baris (AWAL + SESUDAH_BACKUP)", isi)
	}
	cfg := env.pool.Config().Copy()
	cfg.ConnConfig.Database = env.namaDB
	cfg.MinConns = 0
	poolBaru, err := pgxpool.NewWithConfig(ctx, cfg)
	if err != nil {
		t.Fatalf("pool pembaca versi: %v", err)
	}
	defer poolBaru.Close()
	versi, err := versiLedgerSkema(ctx, poolBaru)
	if err != nil {
		t.Fatalf("membaca ledger skema hasil restore: %v", err)
	}
	if versi != terkini {
		t.Errorf("versi skema hasil restore = %d, mau %d", versi, terkini)
	}
}
