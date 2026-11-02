package admin

// Test regresi audit3 grup 10 (B-045 — backup & restore aman).
//
// Dua lapis:
//  1. Unit pemindai dump (pindaiDumpSQL) — tanpa database: payload persis
//     bukti V-AU-18 (\ ! menciptakan berkas lewat psql) dan variannya wajib
//     ditolak, dump pg_dump yang sah (termasuk blok COPY berisi backslash
//     dan baris \restrict pg_dump modern) wajib lolos.
//  2. Integrasi restore terhadap DATABASE SUNGGUHAN sekali pakai yang
//     dibuat test ini sendiri (BUKAN routex_qa, BUKAN skema public):
//     meta-command tidak dieksekusi, error di tengah dump tidak mengubah
//     produksi, versi skema masa depan ditolak, dan round-trip
//     ekspor→ubah→restore mengembalikan keadaan semula sambil menyisakan
//     database pengaman <db>_pre_restore.

import (
	"bytes"
	"context"
	"crypto/rand"
	"encoding/hex"
	"errors"
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

// ---------------------------------------------------------------------------
// 1. Unit pemindai dump
// ---------------------------------------------------------------------------

func TestPindaiDumpSQLMenolakMetaCommand(t *testing.T) {
	kasus := map[string]string{
		"bukti V-AU-18 persis":     "CREATE TABLE x (a int);\n\\! touch /tmp/routex-rce-bukti\n",
		"meta di baris pertama":    "\\! id\nCREATE TABLE x (a int);\n",
		"meta di tengah baris":     "SELECT 1; \\! touch /tmp/x\n",
		"meta copy klien":          "\\copy t from '/etc/passwd'\n",
		"meta set":                 "\\set a b\nSELECT 1;\n",
		"meta sambung database":    "\\c postgres\n",
		"copy program dari server": "COPY t FROM PROGRAM 'id';\n",
		"copy program ke server":   "COPY t TO PROGRAM 'id';\n",
		"backslash liar di SQL":    "SELECT 1 \\g\n",
	}
	for nama, isi := range kasus {
		t.Run(nama, func(t *testing.T) {
			err := pindaiDumpSQL(strings.NewReader(isi))
			if !errors.Is(err, errDumpMetaCommand) {
				t.Fatalf("pindaiDumpSQL = %v, mau errDumpMetaCommand", err)
			}
		})
	}
}

func TestPindaiDumpSQLMenerimaDumpSah(t *testing.T) {
	kasus := map[string]string{
		"dump pg_dump biasa": "-- PostgreSQL database dump\n" +
			"SET statement_timeout = 0;\n" +
			"CREATE TABLE public.t (a text);\n" +
			"INSERT INTO public.t VALUES ('teks biasa');\n",
		"restrict pg_dump modern": "\\restrict KunciAcak123456\n" +
			"CREATE TABLE public.t (a text);\n" +
			"\\unrestrict KunciAcak123456\n",
		"blok copy berisi backslash": "COPY public.t (a) FROM stdin;\n" +
			"\\\\baris data berawalan backslash\n" +
			"baris \\! bukan perintah di dalam data\n" +
			"\\.\n" +
			"CREATE TABLE public.lain (b int);\n",
		"backslash di dalam string": "INSERT INTO public.t VALUES ('C:\\\\folder\\\\berkas');\n",
		"backslash di komentar":     "-- catatan \\! bukan perintah\nSELECT 1;\n/* blok \\c juga aman */\nSELECT 2;\n",
		"fungsi dollar-quote": "CREATE FUNCTION public.f() RETURNS void AS $func$\n" +
			"BEGIN\n\\! baris di badan fungsi, bukan meta-command\nEND;\n$func$ LANGUAGE plpgsql;\n",
		"parameter posisi bukan dollar-quote": "CREATE VIEW public.v AS SELECT $1;\n",
		// Regresi bug nyata yang ditemukan test round-trip: blok komentar
		// pg_dump mendahului kepala COPY dan tabelnya KOSONG (data langsung
		// penutup \.) — penanda komentar tidak boleh mencemari deteksi COPY.
		"copy didahului komentar pg_dump": "--\n" +
			"-- Data for Name: api_key_allowed_models; Type: TABLE DATA; Schema: public; Owner: -\n" +
			"--\n\n" +
			"COPY public.api_key_allowed_models (api_key_id, model_id) FROM stdin;\n\\.\n",
	}
	for nama, isi := range kasus {
		t.Run(nama, func(t *testing.T) {
			if err := pindaiDumpSQL(strings.NewReader(isi)); err != nil {
				t.Fatalf("pindaiDumpSQL = %v, mau nil", err)
			}
		})
	}
}

func TestPindaiDumpSQLMenolakTerpotong(t *testing.T) {
	kasus := map[string]string{
		"string tak berujung": "INSERT INTO public.t VALUES ('abc",
		"copy tak berujung":   "COPY public.t (a) FROM stdin;\n1\n2\n",
		"dollar tak berujung": "CREATE FUNCTION f() RETURNS void AS $func$\nBEGIN\n",
	}
	for nama, isi := range kasus {
		t.Run(nama, func(t *testing.T) {
			err := pindaiDumpSQL(strings.NewReader(isi))
			if !errors.Is(err, errDumpTerpotong) {
				t.Fatalf("pindaiDumpSQL = %v, mau errDumpTerpotong", err)
			}
		})
	}
}

// TestBatasWaktuAdminMengecualikanBackup membuktikan ekspor & restore tidak
// lagi terbunuh tenggat generik admin 60 detik (V-EM-07), sementara rute
// admin lain tetap bertenggat.
func TestBatasWaktuAdminMengecualikanBackup(t *testing.T) {
	// Handler yang menghormati konteks (seperti handler nyata yang memakai
	// exec.CommandContext/query ber-ctx): tenggat middleware membatalkannya
	// dan middleware.Timeout menulis 504; tanpa tenggat ia selesai sendiri.
	lambat := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		select {
		case <-r.Context().Done():
			return
		case <-time.After(150 * time.Millisecond):
			w.WriteHeader(http.StatusOK)
		}
	})
	handler := batasWaktuAdmin(50 * time.Millisecond)(lambat)

	for _, path := range []string{"/api/admin/system/backup/export", "/api/admin/system/backup/restore"} {
		rec := httptest.NewRecorder()
		handler.ServeHTTP(rec, httptest.NewRequest(http.MethodPost, path, nil))
		if rec.Code != http.StatusOK {
			t.Errorf("path %s status = %d, mau 200 (dikecualikan dari tenggat)", path, rec.Code)
		}
	}

	rec := httptest.NewRecorder()
	handler.ServeHTTP(rec, httptest.NewRequest(http.MethodGet, "/api/admin/system/settings", nil))
	if rec.Code != http.StatusGatewayTimeout {
		t.Errorf("rute biasa status = %d, mau 504 (tenggat tetap berlaku)", rec.Code)
	}
}

// ---------------------------------------------------------------------------
// 2. Integrasi restore pada database sekali pakai
// ---------------------------------------------------------------------------

// lingkunganRestore menyiapkan satu database sungguhan sekali pakai yang
// sudah dimigrasikan penuh, beserta Handlers yang pool-nya menunjuk ke sana.
type lingkunganRestore struct {
	t        *testing.T
	namaDB   string
	pengaman string
	pool     *pgxpool.Pool
	maint    *pgxpool.Pool
	handler  *Handlers
}

func siapkanLingkunganRestore(t *testing.T) *lingkunganRestore {
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
	// DSN target bersih: tanpa search_path (restore memang menolaknya) dan
	// menunjuk database sekali pakai milik test ini.
	u.RawQuery = ""
	var acak [4]byte
	if _, err := rand.Read(acak[:]); err != nil {
		t.Fatalf("rand: %v", err)
	}
	namaDB := "test_restore_" + hex.EncodeToString(acak[:])

	u.Path = "/postgres"
	maint, err := pgxpool.New(ctx, u.String())
	if err != nil {
		t.Fatalf("pool pemeliharaan: %v", err)
	}
	if _, err := maint.Exec(ctx, "CREATE DATABASE "+pgxKutipIdent(namaDB)); err != nil {
		maint.Close()
		t.Fatalf("membuat database uji: %v", err)
	}

	u.Path = "/" + namaDB
	pool, err := pgxpool.New(ctx, u.String())
	if err != nil {
		maint.Close()
		t.Fatalf("pool target: %v", err)
	}

	env := &lingkunganRestore{
		t:        t,
		namaDB:   namaDB,
		pengaman: namaDatabasePengaman(namaDB),
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
		maint.Close()
	})

	// Skema penuh aplikasi, sama seperti instalasi nyata.
	if _, err := database.Migrate(ctx, &database.DB{Pool: pool}, env.handler.logger); err != nil {
		t.Fatalf("migrasi database uji: %v", err)
	}
	return env
}

// tanamPenanda membuat tabel penanda sederhana berisi satu baris.
func (e *lingkunganRestore) tanamPenanda(isi string) {
	e.t.Helper()
	ctx := context.Background()
	if _, err := e.pool.Exec(ctx, `CREATE TABLE IF NOT EXISTS penanda (isi text)`); err != nil {
		e.t.Fatalf("membuat tabel penanda: %v", err)
	}
	if _, err := e.pool.Exec(ctx, `DELETE FROM penanda`); err != nil {
		e.t.Fatalf("mengosongkan penanda: %v", err)
	}
	if _, err := e.pool.Exec(ctx, `INSERT INTO penanda VALUES ($1)`, isi); err != nil {
		e.t.Fatalf("menanam penanda: %v", err)
	}
}

// isiPenanda membaca seluruh isi tabel penanda dari database bernama target
// memakai pool BARU (pool lama bisa memegang koneksi yang diputus penukaran).
func (e *lingkunganRestore) isiPenanda(database string) []string {
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

// panggilRestore mengirim satu unggahan multipart ke handler restore.
func (e *lingkunganRestore) panggilRestore(namaFile string, isi []byte) *httptest.ResponseRecorder {
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

func TestRestoreMetaCommandTidakDieksekusi(t *testing.T) {
	env := siapkanLingkunganRestore(t)
	env.tanamPenanda("ASLI")

	berkasBukti := filepath.Join(t.TempDir(), "routex-rce-bukti-g10")
	dump := "CREATE TABLE dari_backup (penanda text);\n" +
		"INSERT INTO dari_backup VALUES ('BACKUP');\n" +
		"\\! touch " + berkasBukti + "\n"

	rec := env.panggilRestore("jahat.sql", []byte(dump))
	if rec.Code != http.StatusBadRequest {
		t.Fatalf("status = %d, mau 400. body: %s", rec.Code, rec.Body.String())
	}
	if !strings.Contains(rec.Body.String(), "invalid_backup_content") {
		t.Errorf("body tidak memuat kode invalid_backup_content: %s", rec.Body.String())
	}
	if _, err := os.Stat(berkasBukti); !os.IsNotExist(err) {
		t.Fatalf("meta-command psql DIEKSEKUSI: berkas bukti %s tercipta", berkasBukti)
	}
	// Tidak satu pun pernyataan dump boleh terterapkan ke produksi.
	var ada bool
	if err := env.pool.QueryRow(context.Background(),
		`SELECT EXISTS (SELECT 1 FROM information_schema.tables WHERE table_name = 'dari_backup')`).Scan(&ada); err != nil {
		t.Fatalf("memeriksa tabel dump: %v", err)
	}
	if ada {
		t.Errorf("tabel dari_backup terterapkan padahal restore ditolak")
	}
	if isi := env.isiPenanda(env.namaDB); len(isi) != 1 || isi[0] != "ASLI" {
		t.Errorf("penanda produksi berubah: %v", isi)
	}
}

func TestRestoreErrorDiTengahTidakMengubahProduksi(t *testing.T) {
	env := siapkanLingkunganRestore(t)
	env.tanamPenanda("ASLI")

	// Tiruan bukti V-DT-01: dump --clean yang menjatuhkan tabel, mengisi
	// nilai backup, lalu meledak di tengah.
	dump := "DROP TABLE IF EXISTS penanda;\n" +
		"CREATE TABLE penanda (isi text);\n" +
		"INSERT INTO penanda VALUES ('BACKUP');\n" +
		"INSERT INTO tabel_yang_tidak_ada VALUES (1);\n"

	rec := env.panggilRestore("rusak.sql", []byte(dump))
	if rec.Code != http.StatusBadRequest {
		t.Fatalf("status = %d, mau 400. body: %s", rec.Code, rec.Body.String())
	}
	if !strings.Contains(rec.Body.String(), "restore_failed") {
		t.Errorf("body tidak memuat kode restore_failed: %s", rec.Body.String())
	}
	// V-EM-02: stderr psql mentah (nama tabel dari pesan error PG) tidak
	// boleh terpantul ke klien.
	if strings.Contains(rec.Body.String(), "tabel_yang_tidak_ada") {
		t.Errorf("respons membocorkan stderr psql mentah: %s", rec.Body.String())
	}
	if isi := env.isiPenanda(env.namaDB); len(isi) != 1 || isi[0] != "ASLI" {
		t.Errorf("produksi tertinggal setengah ter-restore: penanda = %v, mau [ASLI]", isi)
	}
}

func TestRestoreVersiSkemaMasaDepanDitolak(t *testing.T) {
	env := siapkanLingkunganRestore(t)
	env.tanamPenanda("ASLI")

	dump := "CREATE TABLE schema_migrations (version bigint PRIMARY KEY, name text NOT NULL, checksum text NOT NULL, applied_at timestamptz NOT NULL DEFAULT now());\n" +
		"INSERT INTO schema_migrations VALUES (9999, 'migrasi_masa_depan', 'x');\n"

	rec := env.panggilRestore("masa-depan.sql", []byte(dump))
	if rec.Code != http.StatusBadRequest {
		t.Fatalf("status = %d, mau 400. body: %s", rec.Code, rec.Body.String())
	}
	if !strings.Contains(rec.Body.String(), "incompatible_backup") {
		t.Errorf("body tidak memuat kode incompatible_backup: %s", rec.Body.String())
	}
	if isi := env.isiPenanda(env.namaDB); len(isi) != 1 || isi[0] != "ASLI" {
		t.Errorf("penanda produksi berubah: %v", isi)
	}
}

func TestRestoreRoundTripEksporRestore(t *testing.T) {
	env := siapkanLingkunganRestore(t)
	env.tanamPenanda("AWAL")

	// Ekspor lewat handler (jalur POST baru, streaming berkas sementara).
	req := httptest.NewRequest(http.MethodPost, "/api/admin/system/backup/export?format=sql", nil)
	recExp := httptest.NewRecorder()
	env.handler.exportDatabaseSQL(recExp, req)
	if recExp.Code != http.StatusOK {
		t.Fatalf("ekspor status = %d. body: %s", recExp.Code, recExp.Body.String())
	}
	dump := recExp.Body.Bytes()
	if !strings.Contains(string(dump), "PostgreSQL database dump") {
		t.Fatalf("hasil ekspor bukan dump PostgreSQL")
	}

	// Tirukan dump dari pg_dump 17+ (client boleh lebih baru daripada
	// server — kejadian nyata di CI): baris SET transaction_timeout di
	// pembuka tidak dikenal server PostgreSQL 16 dan harus disaring jalur
	// restore, bukan menggugurkan seluruh pemulihan.
	dump = []byte(strings.Replace(string(dump),
		"SET lock_timeout = 0;",
		"SET lock_timeout = 0;\nSET transaction_timeout = 0;", 1))
	if !strings.Contains(string(dump), "SET transaction_timeout") {
		t.Fatalf("jangkar SET lock_timeout tidak ditemukan di dump ekspor")
	}

	// Ubah produksi sesudah backup diambil.
	if _, err := env.pool.Exec(context.Background(), `INSERT INTO penanda VALUES ('SESUDAH_BACKUP')`); err != nil {
		t.Fatalf("mengubah penanda: %v", err)
	}

	rec := env.panggilRestore("routex_backup.sql", dump)
	if rec.Code != http.StatusOK {
		t.Fatalf("restore status = %d. body: %s", rec.Code, rec.Body.String())
	}
	if !strings.Contains(rec.Body.String(), `"safety_database":"`+env.pengaman+`"`) {
		t.Errorf("respons tidak menyebut database pengaman %s: %s", env.pengaman, rec.Body.String())
	}

	// Produksi kembali ke keadaan backup; perubahan sesudahnya hilang dari
	// produksi tetapi utuh di database pengaman.
	if isi := env.isiPenanda(env.namaDB); len(isi) != 1 || isi[0] != "AWAL" {
		t.Errorf("penanda produksi = %v, mau [AWAL]", isi)
	}
	if isi := env.isiPenanda(env.pengaman); len(isi) != 2 {
		t.Errorf("penanda pengaman = %v, mau 2 baris (AWAL + SESUDAH_BACKUP)", isi)
	}
}

// TestSiapkanBerkasRestoreMenyaringPembuka: baris SET transaction_timeout
// hanya dibuang di wilayah pembuka dump; baris serupa di badan fungsi (isi)
// dipertahankan persis, dan dump tanpa baris itu disalin utuh.
func TestSiapkanBerkasRestoreMenyaringPembuka(t *testing.T) {
	tanpaBaris := "-- komentar pg_dump\n" +
		"SET statement_timeout = 0;\n" +
		"SET client_encoding = 'UTF8';\n" +
		"SELECT pg_catalog.set_config('search_path', '', false);\n" +
		"\n" +
		"CREATE FUNCTION publik.contoh() RETURNS void AS $$\n" +
		"BEGIN\n" +
		"    SET transaction_timeout = 0;\n" +
		"END;\n" +
		"$$ LANGUAGE plpgsql;\n"
	denganBaris := strings.Replace(tanpaBaris,
		"SET client_encoding = 'UTF8';",
		"SET client_encoding = 'UTF8';\nSET transaction_timeout = 0;", 1)

	jalankan := func(t *testing.T, isi string) string {
		t.Helper()
		path := filepath.Join(t.TempDir(), "dump.sql")
		if err := os.WriteFile(path, []byte(isi), 0o600); err != nil {
			t.Fatalf("menulis dump uji: %v", err)
		}
		efektif, err := siapkanBerkasRestore(path)
		if err != nil {
			t.Fatalf("siapkanBerkasRestore: %v", err)
		}
		defer os.Remove(efektif)
		hasil, err := os.ReadFile(efektif)
		if err != nil {
			t.Fatalf("membaca berkas efektif: %v", err)
		}
		return string(hasil)
	}

	if got := jalankan(t, denganBaris); got != tanpaBaris {
		t.Errorf("hasil saringan tidak sama dengan dump tanpa baris transaction_timeout:\n--- got ---\n%s\n--- mau ---\n%s", got, tanpaBaris)
	}
	if got := jalankan(t, tanpaBaris); got != tanpaBaris {
		t.Errorf("dump tanpa baris sasaran harus disalin utuh, got:\n%s", got)
	}
}
