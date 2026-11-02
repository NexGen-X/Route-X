package admin

import (
	"bytes"
	"compress/gzip"
	"context"
	"crypto/rand"
	"encoding/hex"
	"errors"
	"fmt"
	"io"
	"log/slog"
	"net/http"
	"net/url"
	"os"
	"os/exec"
	"strings"
	"time"

	"github.com/jackc/pgx/v5/pgxpool"

	"github.com/NexGen-X/Route-X/internal/database"
	"github.com/NexGen-X/Route-X/internal/httpx"
	"github.com/NexGen-X/Route-X/internal/observability"
)

// getBackupStatus mengembalikan ringkasan statistik ukuran database dan info berkas cadangan lokal.
func (h *Handlers) getBackupStatus(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()

	var dbName, dbSize string
	var dbBytes int64
	var totalTables int

	row := h.pool.QueryRow(ctx, `
		SELECT 
			current_database(),
			pg_size_pretty(pg_database_size(current_database())),
			pg_database_size(current_database()),
			(SELECT count(*) FROM information_schema.tables WHERE table_schema = current_schema())
	`)
	if err := row.Scan(&dbName, &dbSize, &dbBytes, &totalTables); err != nil {
		mapRepoError(w, r, err, "status database")
		return
	}

	var modelsCount, providersCount, rulesCount, apiKeysCount int
	_ = h.pool.QueryRow(ctx, `SELECT count(*) FROM models`).Scan(&modelsCount)
	_ = h.pool.QueryRow(ctx, `SELECT count(*) FROM providers`).Scan(&providersCount)
	_ = h.pool.QueryRow(ctx, `SELECT count(*) FROM routing_rules`).Scan(&rulesCount)
	_ = h.pool.QueryRow(ctx, `SELECT count(*) FROM api_keys`).Scan(&apiKeysCount)

	status := BackupStatusDTO{
		DatabaseName:      dbName,
		DatabaseSize:      dbSize,
		DatabaseBytes:     dbBytes,
		TotalTables:       totalTables,
		ModelsCount:       modelsCount,
		ProvidersCount:    providersCount,
		RoutingRulesCount: rulesCount,
		APIKeysCount:      apiKeysCount,
	}

	// Periksa berkas cadangan terakhir di /var/backups/routex atau /var/backups bila ada
	backupDirs := []string{"/var/backups/routex", "/var/backups"}
	var latestFile os.FileInfo
	for _, dir := range backupDirs {
		if entries, err := os.ReadDir(dir); err == nil {
			for _, entry := range entries {
				if !entry.IsDir() && strings.HasPrefix(entry.Name(), "routex") &&
					(strings.HasSuffix(entry.Name(), ".sql.gz") || strings.HasSuffix(entry.Name(), ".sql") || strings.HasSuffix(entry.Name(), ".dump")) {
					if info, err := entry.Info(); err == nil {
						if latestFile == nil || info.ModTime().After(latestFile.ModTime()) {
							latestFile = info
						}
					}
				}
			}
		}
	}
	if latestFile != nil {
		sizeStr := fmt.Sprintf("%.1f KB", float64(latestFile.Size())/1024)
		if latestFile.Size() > 1024*1024 {
			sizeStr = fmt.Sprintf("%.1f MB", float64(latestFile.Size())/(1024*1024))
		}
		status.LastServerBackup = &ServerBackupInfo{
			FileName:  latestFile.Name(),
			FileSize:  sizeStr,
			CreatedAt: latestFile.ModTime(),
		}
	}

	_ = h.respond(w, r, http.StatusOK, status)
}

// cleanConnStringAndSchema membersihkan URI koneksi database dari parameter yang tidak didukung libpq/pg_dump (seperti search_path)
// dan mengekstrak nama skema jika ditentukan.
func cleanConnStringAndSchema(rawConn string) (string, string) {
	u, err := url.Parse(rawConn)
	if err != nil {
		return rawConn, ""
	}
	q := u.Query()
	schema := q.Get("search_path")
	q.Del("search_path")
	u.RawQuery = q.Encode()
	return u.String(), schema
}

// pgToolEnv menyusun environment untuk pg_dump/psql agar kredensial database
// TIDAK lewat argv proses (AUDIT FIX: sebelumnya conn string utuh — termasuk
// password — tampil di /proc/<pid>/cmdline dan terbaca pengguna host lain).
// Kredensial dikirim via PGPASSWORD/PGHOST/PGPORT/PGUSER/PGDATABASE, dan nama
// basis data dirujuk tanpa URI di argv.
func pgToolEnv(connStr string) ([]string, string, error) {
	u, err := url.Parse(connStr)
	if err != nil {
		return nil, "", fmt.Errorf("memarsing connection string: %w", err)
	}
	env := os.Environ()
	dbName := strings.TrimPrefix(u.Path, "/")
	if pw, ok := u.User.Password(); ok {
		env = append(env, "PGPASSWORD="+pw)
	}
	if ui := u.User.Username(); ui != "" {
		env = append(env, "PGUSER="+ui)
	}
	if host := u.Hostname(); host != "" {
		env = append(env, "PGHOST="+host)
	}
	if p := u.Port(); p != "" {
		env = append(env, "PGPORT="+p)
	}
	if dbName == "" {
		return nil, "", fmt.Errorf("nama basis data tidak ada pada connection string")
	}
	if ssl := u.Query().Get("sslmode"); ssl != "" {
		env = append(env, "PGSSLMODE="+ssl)
	}
	return env, dbName, nil
}

// exportDatabaseSQL mengalirkan dump SQL database terkompresi (.sql.gz) atau teks polos (.sql) langsung ke klien.
//
// Sejak audit3 B-045 ekspor adalah POST (bukan GET): dump penuh adalah data
// sensitif dan operasi berat, jadi ia wajib melewati CSRF seperti restore —
// tautan <a href> polos tidak lagi bisa memicunya (V-AU-15).
//
// pg_dump menulis ke BERKAS SEMENTARA di disk, bukan buffer memori: versi
// lama menahan seluruh dump (plus salinan gzip-nya) di RAM proses gateway
// sehingga ekspor database besar berisiko OOM (V-DT-02). Header respons
// tetap hanya dikirim SETELAH dump terbukti utuh (warisan perbaikan F5:
// kegagalan pg_dump di tengah tidak boleh menghasilkan unduhan rusak
// berstatus 200); sesudah itu isi berkas dialirkan (gzip on-the-fly).
func (h *Handlers) exportDatabaseSQL(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	format := r.URL.Query().Get("format")
	if format == "" {
		format = "sql.gz"
	}
	if format != "sql" && format != "sql.gz" {
		httpx.BadRequest(w, r, "invalid_format", "format cadangan harus 'sql' atau 'sql.gz'")
		return
	}

	connStr, schema := cleanConnStringAndSchema(h.pool.Config().ConnString())
	timestamp := time.Now().UTC().Format("20060102_150405")

	baseArgs := []string{"--clean", "--if-exists", "--no-owner", "--no-privileges"}
	if schema != "" {
		baseArgs = append(baseArgs, "-n", schema)
	}
	// AUDIT FIX: kredensial via environment (PGPASSWORD dkk.), bukan URI di argv.
	env, dbName, envErr := pgToolEnv(connStr)
	if envErr != nil {
		h.logger.ErrorContext(ctx, "gagal menyiapkan environment pg_dump", slog.Any("err", envErr))
		httpx.InternalError(w, r)
		return
	}
	baseArgs = append(baseArgs, dbName)

	tmp, err := os.CreateTemp("", "routex-dump-*.sql")
	if err != nil {
		h.logger.ErrorContext(ctx, "gagal membuat berkas sementara ekspor", slog.Any("err", err))
		httpx.InternalError(w, r)
		return
	}
	defer os.Remove(tmp.Name())
	defer tmp.Close()

	{
		cmd := exec.CommandContext(ctx, "pg_dump", baseArgs...)
		cmd.Env = env
		cmd.Stdout = tmp
		var stderr bytes.Buffer
		cmd.Stderr = &stderr
		if err := cmd.Run(); err != nil {
			// stderr pg_dump bisa memuat baris \connect yang menyisipkan DSN dan
			// token sesi: jangan catat mentah ke log (audit3 V-EM-34).
			h.logger.ErrorContext(ctx, "gagal mengekspor database", slog.String("error", observability.ScrubLogText(stderr.String())), slog.Any("err", err))
			httpx.InternalError(w, r)
			return
		}
	}
	info, err := tmp.Stat()
	if err != nil {
		h.logger.ErrorContext(ctx, "gagal membaca berkas dump sementara", slog.Any("err", err))
		httpx.InternalError(w, r)
		return
	}
	if _, err := tmp.Seek(0, io.SeekStart); err != nil {
		h.logger.ErrorContext(ctx, "gagal memutar berkas dump sementara", slog.Any("err", err))
		httpx.InternalError(w, r)
		return
	}

	if format == "sql" {
		fileName := fmt.Sprintf("routex_backup_%s.sql", timestamp)
		w.Header().Set("Content-Type", "application/sql; charset=utf-8")
		w.Header().Set("Content-Disposition", fmt.Sprintf("attachment; filename=%q", fileName))
		w.Header().Set("Cache-Control", "no-store")
		w.Header().Set("Content-Length", fmt.Sprintf("%d", info.Size()))
		_, _ = io.Copy(w, tmp)
	} else {
		fileName := fmt.Sprintf("routex_backup_%s.sql.gz", timestamp)
		w.Header().Set("Content-Type", "application/gzip")
		w.Header().Set("Content-Disposition", fmt.Sprintf("attachment; filename=%q", fileName))
		w.Header().Set("Cache-Control", "no-store")
		gw := gzip.NewWriter(w)
		_, _ = io.Copy(gw, tmp)
		_ = gw.Close()
	}

	h.writeAudit(ctx, r, "export", "database_backup", "all", map[string]any{"format": format})
}

// maksUkuranDumpSQL membatasi ukuran dump SQL SESUDAH dekompresi yang boleh
// diproses restore (pertahanan gzip-bomb: unggahan terkompresi dibatasi
// 50 MB oleh MaxBytesReader, tetapi hasil dekompresinya bisa jauh lebih
// besar). 1 GiB jauh di atas dump wajar instalasi pribadi.
const maksUkuranDumpSQL = 1 << 30

// errBackupTidakKompatibel menandai dump yang versi skema/ledger-nya tidak
// bisa diterima biner ini (dari versi lebih baru, atau tanpa ledger migrasi).
var errBackupTidakKompatibel = errors.New("versi skema cadangan tidak kompatibel")

// hasilRestore merangkum keluaran jalur restore yang berhasil.
type hasilRestore struct {
	DatabasePengaman string // nama database salinan pra-restore (keadaan sebelum restore)
	VersiSkema       int64  // versi migrasi akhir database hasil restore
}

// restoreDatabaseSQL memulihkan database dari berkas SQL yang diunggah (.sql atau .sql.gz).
//
// Desain sejak audit3 B-045 (menggantikan penyaluran psql langsung ke
// database produksi):
//  1. Unggahan dispool ke berkas sementara (dengan dekompresi terbatas).
//  2. Isi dump DIPERIKSA pemindai keamanan (backup_sql.go): meta-command
//     psql (\ ! dkk.) dan COPY ... PROGRAM ditolak — berkas cadangan tidak
//     pernah lagi menjadi jalur eksekusi perintah OS (V-AU-18).
//  3. Dump di-restore ke DATABASE SKRATCH sekali pakai memakai psql
//     --single-transaction + ON_ERROR_STOP: error di tengah membatalkan
//     seluruh transaksi skratch, database produksi tidak tersentuh sama
//     sekali (V-DT-01).
//  4. Versi ledger migrasi dump diperiksa terhadap biner ini: lebih baru →
//     ditolak; lebih lama → skratch dimigrasikan sampai versi terkini
//     SEBELUM penukaran, jadi tidak ada skema hibrida (V-DT-03).
//  5. Penukaran atomik di tingkat database: produksi diganti nama menjadi
//     <db>_pre_restore (salinan pengaman/rollback manual), skratch diganti
//     nama menjadi nama produksi. Tulisan yang masuk selama restore pergi
//     ke database lama — tersimpan utuh di salinan pengaman, tidak menyusup
//     ke hasil restore (V-DT-02).
//
// Kegagalan pada langkah mana pun sebelum penukaran meninggalkan produksi
// persis seperti semula, dan pesan error ke klien selalu generik — stderr
// psql hanya masuk log server (V-EM-02).
func (h *Handlers) restoreDatabaseSQL(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()

	// Batasi ukuran request body hingga 50 MB
	r.Body = http.MaxBytesReader(w, r.Body, 50<<20)
	if err := r.ParseMultipartForm(50 << 20); err != nil {
		httpx.BadRequest(w, r, "file_too_large", "ukuran berkas backup melebihi batas maksimal 50 MB")
		return
	}

	file, header, err := r.FormFile("file")
	if err != nil {
		httpx.BadRequest(w, r, "missing_file", "berkas cadangan 'file' (.sql atau .sql.gz) wajib diunggah")
		return
	}
	defer file.Close()

	lowerName := strings.ToLower(header.Filename)
	if !strings.HasSuffix(lowerName, ".sql") && !strings.HasSuffix(lowerName, ".sql.gz") {
		httpx.BadRequest(w, r, "invalid_file_extension", "ekstensi berkas cadangan harus berupa .sql atau .sql.gz")
		return
	}

	// Spool unggahan menjadi berkas SQL polos sementara (dekompresi gzip
	// dialirkan dan dibatasi maksUkuranDumpSQL).
	berkasSQL, err := h.spoolDumpUnggahan(file)
	if err != nil {
		if errors.Is(err, errDumpTerlaluBesar) {
			httpx.BadRequest(w, r, "file_too_large", "isi berkas cadangan sesudah dekompresi melebihi batas maksimal")
			return
		}
		if errors.Is(err, errDumpKosong) {
			httpx.BadRequest(w, r, "invalid_file", "berkas cadangan kosong atau rusak")
			return
		}
		h.logger.ErrorContext(ctx, "gagal menyiapkan berkas restore", slog.Any("err", err))
		httpx.InternalError(w, r)
		return
	}
	defer os.Remove(berkasSQL)

	// Pemeriksaan keamanan isi dump SEBELUM psql melihatnya.
	f, err := os.Open(berkasSQL)
	if err != nil {
		h.logger.ErrorContext(ctx, "gagal membuka berkas restore", slog.Any("err", err))
		httpx.InternalError(w, r)
		return
	}
	scanErr := pindaiDumpSQL(f)
	_ = f.Close()
	if scanErr != nil {
		h.logger.WarnContext(ctx, "restore ditolak oleh pemindai dump",
			slog.String("filename", header.Filename), slog.Any("err", scanErr))
		if errors.Is(scanErr, errDumpMetaCommand) {
			httpx.BadRequest(w, r, "invalid_backup_content", "berkas cadangan mengandung perintah meta psql atau COPY PROGRAM yang dilarang")
			return
		}
		httpx.BadRequest(w, r, "invalid_file", "berkas cadangan terpotong atau tidak lengkap")
		return
	}

	connStr, schema := cleanConnStringAndSchema(h.pool.Config().ConnString())
	if schema != "" {
		// Penukaran berbasis nama database hanya bermakna bila aplikasi
		// memakai skema bawaan (public). Deployment produksi (installer
		// native maupun Compose) tidak menyetel search_path; konfigurasi
		// search_path kustom (lingkungan uji multi-skema) diarahkan ke
		// pemulihan manual dengan pg_dump/psql.
		h.logger.WarnContext(ctx, "restore ditolak: konfigurasi search_path kustom", slog.String("schema", schema))
		httpx.BadRequest(w, r, "restore_not_supported", "restore lewat dashboard tidak didukung untuk konfigurasi search_path kustom; gunakan pemulihan manual")
		return
	}

	// Siapkan berkas efektif untuk psql: saring baris pembuka yang hanya
	// dikenal server PostgreSQL yang lebih baru daripada server tujuan
	// (lihat siapkanBerkasRestore).
	berkasEfektif, err := siapkanBerkasRestore(berkasSQL)
	if err != nil {
		h.logger.ErrorContext(ctx, "gagal menyiapkan berkas restore efektif", slog.Any("err", err))
		httpx.InternalError(w, r)
		return
	}
	defer os.Remove(berkasEfektif)

	hasil, err := h.jalankanRestore(ctx, berkasEfektif, connStr)
	if err != nil {
		h.logger.ErrorContext(ctx, "gagal memulihkan database dari backup sql",
			slog.String("filename", header.Filename), slog.Any("err", err))
		if errors.Is(err, errBackupTidakKompatibel) {
			httpx.BadRequest(w, r, "incompatible_backup", "versi skema berkas cadangan tidak kompatibel dengan versi aplikasi ini")
			return
		}
		httpx.BadRequest(w, r, "restore_failed", "Gagal memulihkan database. Rincian kegagalan tercatat di log server.")
		return
	}

	h.writeAudit(ctx, r, "restore", "database_backup", "all", map[string]any{
		"filename":        header.Filename,
		"size":            header.Size,
		"safety_database": hasil.DatabasePengaman,
		"schema_version":  hasil.VersiSkema,
	})

	_ = h.respond(w, r, http.StatusOK, BackupRestoreResponse{
		Status:         "success",
		SafetyDatabase: hasil.DatabasePengaman,
		SchemaVersion:  hasil.VersiSkema,
	})
}

var (
	errDumpTerlaluBesar = errors.New("dump sesudah dekompresi melebihi batas ukuran")
	errDumpKosong       = errors.New("dump kosong atau rusak")
)

// spoolDumpUnggahan menulis isi unggahan (mentah atau gzip) ke berkas
// sementara SQL polos dan mengembalikan path-nya. Pemanggil wajib menghapus
// berkasnya.
func (h *Handlers) spoolDumpUnggahan(file io.Reader) (string, error) {
	// Deteksi kompresi gzip lewat magic bytes (0x1f, 0x8b)
	headerBytes := make([]byte, 2)
	n, _ := io.ReadFull(file, headerBytes)
	if n < 2 {
		return "", errDumpKosong
	}
	fullReader := io.MultiReader(bytes.NewReader(headerBytes[:n]), file)

	var src io.Reader
	if headerBytes[0] == 0x1f && headerBytes[1] == 0x8b {
		gzReader, err := gzip.NewReader(fullReader)
		if err != nil {
			return "", fmt.Errorf("%w: arsip gzip rusak: %v", errDumpKosong, err)
		}
		defer gzReader.Close()
		src = gzReader
	} else {
		src = fullReader
	}

	tmp, err := os.CreateTemp("", "routex-restore-*.sql")
	if err != nil {
		return "", err
	}
	tertulis, err := io.Copy(tmp, io.LimitReader(src, maksUkuranDumpSQL+1))
	if err != nil {
		_ = tmp.Close()
		_ = os.Remove(tmp.Name())
		return "", err
	}
	if err := tmp.Close(); err != nil {
		_ = os.Remove(tmp.Name())
		return "", err
	}
	if tertulis > maksUkuranDumpSQL {
		_ = os.Remove(tmp.Name())
		return "", errDumpTerlaluBesar
	}
	if tertulis == 0 {
		_ = os.Remove(tmp.Name())
		return "", errDumpKosong
	}
	return tmp.Name(), nil
}

// poolDatabaseLain membangun pool kecil yang menunjuk database lain pada
// server yang sama dengan pool utama (dipakai untuk database skratch dan
// database pemeliharaan "postgres").
func (h *Handlers) poolDatabaseLain(ctx context.Context, database string, maksKoneksi int32) (*pgxpool.Pool, error) {
	cfg := h.pool.Config().Copy()
	cfg.ConnConfig.Database = database
	cfg.MaxConns = maksKoneksi
	cfg.MinConns = 0
	return pgxpool.NewWithConfig(ctx, cfg)
}

// jalankanRestore mengeksekusi langkah 3–5 desain restore (skratch →
// verifikasi versi → migrasi → tukar). Error yang membungkus
// errBackupTidakKompatibel berarti dump ditolak karena versi; error lain
// adalah kegagalan operasional dan produksi dijamin belum berubah.
func (h *Handlers) jalankanRestore(ctx context.Context, berkasSQL, connStr string) (*hasilRestore, error) {
	target := h.pool.Config().ConnConfig.Database
	scratch := "routex_restore_" + suffixAcak()
	safety := namaDatabasePengaman(target)

	maint, err := h.poolDatabaseLain(ctx, "postgres", 2)
	if err != nil {
		return nil, fmt.Errorf("menyiapkan koneksi pemeliharaan: %w", err)
	}
	defer maint.Close()

	if _, err := maint.Exec(ctx, "CREATE DATABASE "+pgxKutipIdent(scratch)); err != nil {
		return nil, fmt.Errorf("membuat database skratch: %w", err)
	}
	// Skratch selalu dibersihkan bila penukaran tidak terjadi.
	sudahTukar := false
	defer func() {
		if sudahTukar {
			return
		}
		bctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
		defer cancel()
		_ = jatuhkanDatabase(bctx, maint, scratch)
	}()

	// Langkah 3: restore ke skratch dalam SATU transaksi.
	env, _, envErr := pgToolEnv(connStr)
	if envErr != nil {
		return nil, fmt.Errorf("menyiapkan environment psql: %w", envErr)
	}
	{
		// -X: abaikan psqlrc operator (perilaku hanya ditentukan dump).
		cmd := exec.CommandContext(ctx, "psql",
			"-X", "-v", "ON_ERROR_STOP=1", "--single-transaction",
			"-f", berkasSQL, scratch)
		cmd.Env = env
		var stderr bytes.Buffer
		cmd.Stderr = &stderr
		cmd.Stdout = &bytes.Buffer{}
		if err := cmd.Run(); err != nil {
			// stderr psql mentah tidak boleh tertanam di error yang mengalir
			// ke log/pemanggil — bisa memuat DSN & token sesi (audit3
			// V-EM-34). Versi tersanitasi tetap memberi konteks kegagalan.
			return nil, fmt.Errorf("psql restore skratch gagal: %w (stderr: %s)", err, observability.ScrubLogText(strings.TrimSpace(stderr.String())))
		}
	}

	scratchPool, err := h.poolDatabaseLain(ctx, scratch, 2)
	if err != nil {
		return nil, fmt.Errorf("membuka pool skratch: %w", err)
	}
	defer scratchPool.Close()

	// Langkah 4: verifikasi & samakan versi skema di skratch.
	versi, err := versiLedgerSkema(ctx, scratchPool)
	if err != nil {
		return nil, fmt.Errorf("%w: ledger migrasi tidak ditemukan pada cadangan (%v)", errBackupTidakKompatibel, err)
	}
	terkini, err := database.LatestMigrationVersion()
	if err != nil {
		return nil, fmt.Errorf("membaca versi migrasi biner: %w", err)
	}
	if versi > terkini {
		return nil, fmt.Errorf("%w: cadangan berasal dari skema versi %d, biner ini hanya mengenal sampai %d", errBackupTidakKompatibel, versi, terkini)
	}
	if versi < terkini {
		// Migrasi dijalankan di skratch — kegagalannya tidak menyentuh
		// produksi sama sekali, dan hasil akhirnya skema yang persis sama
		// dengan yang diharapkan biner ini (tidak ada skema hibrida).
		if _, err := database.Migrate(ctx, &database.DB{Pool: scratchPool}, h.logger); err != nil {
			return nil, fmt.Errorf("memigrasikan database skratch: %w", err)
		}
		versi, err = versiLedgerSkema(ctx, scratchPool)
		if err != nil || versi != terkini {
			return nil, fmt.Errorf("%w: migrasi skratch berhenti di versi %d (target %d)", errBackupTidakKompatibel, versi, terkini)
		}
	}
	// Pool skratch harus ditutup sebelum rename: koneksinya sendiri adalah
	// "pengguna lain" bagi ALTER DATABASE.
	scratchPool.Close()

	// Langkah 5: penukaran nama database.
	if err := tukarDatabase(ctx, maint, target, scratch, safety); err != nil {
		return nil, fmt.Errorf("menukar database: %w", err)
	}
	sudahTukar = true
	return &hasilRestore{DatabasePengaman: safety, VersiSkema: terkini}, nil
}

// versiLedgerSkema membaca versi migrasi tertinggi yang tercatat di
// schema_migrations sebuah database.
func versiLedgerSkema(ctx context.Context, pool *pgxpool.Pool) (int64, error) {
	var v int64
	err := pool.QueryRow(ctx, `SELECT COALESCE(MAX(version), 0) FROM schema_migrations`).Scan(&v)
	return v, err
}

// tukarDatabase mengganti database target dengan skratch secara atomik di
// tingkat nama: target → safety (salinan pengaman), skratch → target.
// Kegagalan di tengah mengembalikan nama target seperti semula.
func tukarDatabase(ctx context.Context, maint *pgxpool.Pool, target, scratch, safety string) error {
	// Salinan pengaman dari restore sebelumnya sudah tidak relevan begitu
	// restore baru dimulai: singkirkan lebih dulu.
	if err := jatuhkanDatabase(ctx, maint, safety); err != nil {
		return fmt.Errorf("membersihkan salinan pengaman lama: %w", err)
	}

	sudahDipindah := false
	var errAkhir error
	for percobaan := 0; percobaan < 3; percobaan++ {
		if percobaan > 0 {
			select {
			case <-ctx.Done():
				return ctx.Err()
			case <-time.After(300 * time.Millisecond):
			}
		}
		// Pool aplikasi bisa membuka koneksi baru ke target kapan saja;
		// putuskan semua koneksi ke kedua database tepat sebelum rename.
		if err := putuskanKoneksi(ctx, maint, target, scratch); err != nil {
			errAkhir = err
			continue
		}
		if !sudahDipindah {
			if _, err := maint.Exec(ctx, "ALTER DATABASE "+pgxKutipIdent(target)+" RENAME TO "+pgxKutipIdent(safety)); err != nil {
				errAkhir = err
				continue
			}
			sudahDipindah = true
		}
		if _, err := maint.Exec(ctx, "ALTER DATABASE "+pgxKutipIdent(scratch)+" RENAME TO "+pgxKutipIdent(target)); err != nil {
			errAkhir = err
			if sudahDipindah {
				// Putar balik: kembalikan nama produksi dari salinan pengaman.
				_ = putuskanKoneksi(ctx, maint, safety)
				if _, rbErr := maint.Exec(ctx, "ALTER DATABASE "+pgxKutipIdent(safety)+" RENAME TO "+pgxKutipIdent(target)); rbErr == nil {
					sudahDipindah = false
				}
			}
			continue
		}
		return nil
	}
	return errAkhir
}

// jatuhkanDatabase memutus koneksi lalu menghapus database bila ada.
func jatuhkanDatabase(ctx context.Context, maint *pgxpool.Pool, nama string) error {
	if err := putuskanKoneksi(ctx, maint, nama); err != nil {
		return err
	}
	_, err := maint.Exec(ctx, "DROP DATABASE IF EXISTS "+pgxKutipIdent(nama))
	return err
}

// putuskanKoneksi mengakhiri semua backend yang terhubung ke database yang
// disebut (syarat ALTER DATABASE RENAME / DROP DATABASE).
func putuskanKoneksi(ctx context.Context, maint *pgxpool.Pool, database ...string) error {
	for _, nama := range database {
		if _, err := maint.Exec(ctx, `
			SELECT pg_terminate_backend(pid) FROM pg_stat_activity
			WHERE datname = $1 AND pid <> pg_backend_pid()`, nama); err != nil {
			return err
		}
	}
	return nil
}

// pgxKutipIdent mengutip identifier PostgreSQL dengan aman (nama database di
// jalur restore seluruhnya bentukan server, ini pertahanan berlapis).
func pgxKutipIdent(nama string) string {
	return `"` + strings.ReplaceAll(nama, `"`, `""`) + `"`
}

// namaDatabasePengaman menyusun nama salinan pra-restore: <db>_pre_restore,
// dibersihkan ke karakter identifier aman dan dibatasi 63 byte (batas PG).
func namaDatabasePengaman(target string) string {
	var b strings.Builder
	for _, c := range strings.ToLower(target) {
		if (c >= 'a' && c <= 'z') || (c >= '0' && c <= '9') || c == '_' {
			b.WriteRune(c)
		} else {
			b.WriteByte('_')
		}
	}
	nama := b.String() + "_pre_restore"
	if len(nama) > 63 {
		nama = nama[:63]
	}
	return nama
}

// suffixAcak mengembalikan 10 karakter hex acak untuk nama skratch.
func suffixAcak() string {
	var buf [5]byte
	if _, err := rand.Read(buf[:]); err != nil {
		// crypto/rand yang gagal adalah keadaan fatal yang langka; jatuhkan
		// ke penanda waktu agar alur tetap berjalan dengan nama unik.
		return fmt.Sprintf("%010x", time.Now().UnixNano()&0xffffffffff)
	}
	return hex.EncodeToString(buf[:])
}
