package database

import (
	"cmp"
	"context"
	"crypto/sha256"
	"embed"
	"encoding/hex"
	"errors"
	"fmt"
	"io/fs"
	"log/slog"
	"regexp"
	"slices"
	"strconv"
	"strings"
	"time"

	"github.com/jackc/pgx/v5/pgxpool"
)

// File migrasi ditanam ke biner supaya deploy cukup mengirim satu file dan tidak
// mungkin ada biner yang jalan dengan direktori migrasi versi lain.
//
//go:embed migrations/*.sql
var migrationsFS embed.FS

// migrationsDir adalah subdirektori tempat file .sql tertanam.
const migrationsDir = "migrations"

// advisoryLockKey adalah kunci pg_advisory_lock untuk seluruh proses migrasi.
//
// Semua instance Route-X yang start bersamaan memperebutkan kunci yang sama: satu
// mengubah skema, yang lain menunggu lalu mendapati pekerjaannya sudah selesai.
// Angkanya tidak istimewa (ASCII "ROUTEX"), yang penting konstan dan tidak
// bertabrakan dengan pemakaian advisory lock lain di database yang sama.
const advisoryLockKey int64 = 0x524F55544558

// lockWaitTimeout membatasi lama menunggu instance lain menyelesaikan migrasinya.
//
// Kalau terlampaui kita menyerah dan gagal start: orchestrator akan menjalankan
// ulang, dan itu jauh lebih baik daripada proses yang menggantung tanpa penjelasan.
const lockWaitTimeout = 60 * time.Second

// unlockTimeout membatasi upaya melepas kunci saat pembersihan.
const unlockTimeout = 10 * time.Second

// ledgerTable adalah tabel pembukuan migrasi.
const ledgerTable = "schema_migrations"

var (
	// ErrInvalidMigration berarti kumpulan file migrasinya sendiri yang salah:
	// nama tidak sesuai format, versi ganda, atau file kosong. Ini bug build,
	// bukan masalah runtime.
	ErrInvalidMigration = errors.New("file migrasi tidak valid")

	// ErrChecksumMismatch berarti isi file migrasi yang sudah pernah diterapkan
	// berubah. Migrasi tidak dilanjutkan karena database dan biner sudah tidak
	// sepakat soal riwayat skema.
	ErrChecksumMismatch = errors.New("checksum migrasi tidak cocok")
)

// migration adalah satu file migrasi yang sudah diurai.
type migration struct {
	version  int64  // nomor dari prefix nama file
	name     string // bagian setelah nomor, tanpa ekstensi
	filename string
	sql      string
	checksum string // SHA-256 heksadesimal dari isi file
}

// migrationFilePattern mewajibkan nama NNNN_nama_snake_case.sql.
//
// Minimal empat digit supaya urutan numerik dan urutan tampilan di file listing
// sama sampai migrasi ke-9999, lalu nama huruf kecil yang dipisah garis bawah agar
// tidak ada dua gaya penamaan yang hidup bersamaan.
var migrationFilePattern = regexp.MustCompile(`^(\d{4,})_([a-z0-9]+(?:_[a-z0-9]+)*)\.sql$`)

// checksum menghitung sidik jari isi file migrasi.
//
// Sengaja byte-exact: perubahan spasi pun terdeteksi, karena tujuannya memang
// menangkap file migrasi yang diedit setelah diterapkan.
func checksum(body []byte) string {
	sum := sha256.Sum256(body)
	return hex.EncodeToString(sum[:])
}

// embeddedMigrations mengurai migrasi yang tertanam di biner.
func embeddedMigrations() ([]migration, error) {
	src, err := fs.Sub(migrationsFS, migrationsDir)
	if err != nil {
		return nil, fmt.Errorf("membuka direktori migrasi tertanam: %w", err)
	}
	return parseMigrations(src)
}

// parseMigrations membaca dan memvalidasi seluruh file migrasi di akar fsys, lalu
// mengembalikannya terurut naik menurut nomor versi.
//
// Dipisah dari Migrate supaya seluruh aturan penamaan, pengurutan, dan checksum bisa
// diuji tanpa database.
func parseMigrations(fsys fs.FS) ([]migration, error) {
	entries, err := fs.ReadDir(fsys, ".")
	if err != nil {
		return nil, fmt.Errorf("membaca direktori migrasi: %w", err)
	}

	out := make([]migration, 0, len(entries))
	seen := make(map[int64]string, len(entries))

	for _, e := range entries {
		name := e.Name()
		if e.IsDir() {
			return nil, fmt.Errorf("%w: %q adalah direktori, direktori migrasi harus datar", ErrInvalidMigration, name)
		}
		m := migrationFilePattern.FindStringSubmatch(name)
		if m == nil {
			return nil, fmt.Errorf("%w: nama %q tidak sesuai format NNNN_nama_snake_case.sql", ErrInvalidMigration, name)
		}
		// Nomor versi diurai sebagai angka, bukan dibandingkan sebagai string, supaya
		// 0010 datang setelah 0002 — pengurutan leksikografis mulai salah begitu
		// jumlah digitnya berbeda.
		version, err := strconv.ParseInt(m[1], 10, 64)
		if err != nil {
			return nil, fmt.Errorf("%w: nomor versi pada %q tidak bisa dibaca: %v", ErrInvalidMigration, name, err)
		}
		if version <= 0 {
			return nil, fmt.Errorf("%w: nomor versi pada %q harus lebih besar dari nol", ErrInvalidMigration, name)
		}
		if prev, dup := seen[version]; dup {
			return nil, fmt.Errorf("%w: versi %d dipakai dua kali (%q dan %q)", ErrInvalidMigration, version, prev, name)
		}
		body, err := fs.ReadFile(fsys, name)
		if err != nil {
			return nil, fmt.Errorf("membaca %q: %w", name, err)
		}
		if strings.TrimSpace(string(body)) == "" {
			return nil, fmt.Errorf("%w: %q tidak berisi apa pun", ErrInvalidMigration, name)
		}

		seen[version] = name
		out = append(out, migration{
			version:  version,
			name:     m[2],
			filename: name,
			sql:      string(body),
			checksum: checksum(body),
		})
	}

	if len(out) == 0 {
		return nil, fmt.Errorf("%w: tidak ada file migrasi yang ditemukan", ErrInvalidMigration)
	}

	slices.SortFunc(out, func(a, b migration) int { return cmp.Compare(a.version, b.version) })
	return out, nil
}

// appliedMigration adalah satu baris pembukuan di schema_migrations.
type appliedMigration struct {
	version   int64
	name      string
	checksum  string
	appliedAt time.Time
}

// Migrate menerapkan semua migrasi yang belum tercatat dan mengembalikan jumlah yang
// baru diterapkan.
//
// Aman dipanggil di setiap start aplikasi, termasuk kalau beberapa instance start
// bersamaan: seluruh rangkaian dijaga pg_advisory_lock, jadi instance kedua menunggu
// lalu tidak menemukan pekerjaan dan mengembalikan applied == 0.
//
// Setiap migrasi berjalan di transaksinya sendiri bersama pencatatan bukunya,
// sehingga tidak mungkin ada migrasi yang tercatat tapi tidak jalan atau sebaliknya.
// Migrasi yang gagal membatalkan dirinya sendiri dan menghentikan sisanya; migrasi
// sebelumnya yang sudah sukses tetap terpasang. Konsekuensinya, pernyataan yang tidak
// bisa jalan di dalam transaksi (mis. CREATE INDEX CONCURRENTLY) belum didukung.
//
// logger boleh nil.
func Migrate(ctx context.Context, db *DB, logger *slog.Logger) (applied int, err error) {
	if db == nil || db.Pool == nil {
		return 0, ErrClosed
	}
	logger = loggerOr(logger)

	migrations, err := embeddedMigrations()
	if err != nil {
		return 0, err
	}

	// Advisory lock bersifat per-session, jadi seluruh rangkaian dikerjakan di satu
	// koneksi yang dipegang sampai selesai — termasuk pelepasan kuncinya.
	conn, err := db.Pool.Acquire(ctx)
	if err != nil {
		return 0, fmt.Errorf("%w: mengambil koneksi untuk migrasi: %s", ErrUnavailable, db.scrub(err))
	}
	defer conn.Release()

	unlock, err := lockMigrations(ctx, conn, logger)
	if err != nil {
		return 0, err
	}
	defer unlock()

	if err := ensureLedger(ctx, conn); err != nil {
		return 0, err
	}
	ledger, err := loadLedger(ctx, conn)
	if err != nil {
		return 0, err
	}

	if err := checkLedgerAgainstFiles(migrations, ledger, logger); err != nil {
		return 0, err
	}

	var maxApplied int64
	for version := range ledger {
		maxApplied = max(maxApplied, version)
	}

	for _, m := range migrations {
		if _, done := ledger[m.version]; done {
			continue
		}
		// Nomor di bawah versi tertinggi yang sudah diterapkan berarti ada dua branch
		// yang dikerjakan paralel. Tetap dijalankan supaya deploy tidak macet, tapi
		// dicatat karena urutan di database ini sudah tidak sama dengan urutan di
		// database lain yang migrasinya lebih dulu.
		if m.version < maxApplied {
			logger.Warn("migrasi diterapkan di luar urutan",
				slog.Int64("version", m.version),
				slog.Int64("max_applied", maxApplied),
				slog.String("file", m.filename),
			)
		}

		start := time.Now()
		if err := applyOne(ctx, conn, m); err != nil {
			return applied, err
		}
		applied++
		logger.Info("migrasi diterapkan",
			slog.Int64("version", m.version),
			slog.String("name", m.name),
			slog.String("checksum", m.checksum),
			slog.Duration("took", time.Since(start)),
		)
	}

	logger.Info("migrasi selesai",
		slog.Int("applied", applied),
		slog.Int("total", len(migrations)),
	)
	return applied, nil
}

// lockMigrations mengambil advisory lock dan mengembalikan fungsi pelepasnya.
//
// Kunci dilepas eksplisit lewat pg_advisory_unlock dan tidak menggantungkan diri pada
// koneksi yang tertutup, karena koneksi di sini akan kembali ke pool dan dipakai lagi.
func lockMigrations(ctx context.Context, conn *pgxpool.Conn, logger *slog.Logger) (func(), error) {
	lockCtx, cancel := context.WithTimeout(ctx, lockWaitTimeout)
	defer cancel()

	if _, err := conn.Exec(lockCtx, "select pg_advisory_lock($1)", advisoryLockKey); err != nil {
		return nil, fmt.Errorf("mengambil advisory lock migrasi (kunci %d): %w", advisoryLockKey, err)
	}

	return func() {
		// Context terpisah: kunci harus tetap dilepas walaupun ctx pemanggil sudah
		// dibatalkan karena migrasinya gagal atau kena timeout.
		unlockCtx, cancel := context.WithTimeout(context.WithoutCancel(ctx), unlockTimeout)
		defer cancel()
		if _, err := conn.Exec(unlockCtx, "select pg_advisory_unlock($1)", advisoryLockKey); err != nil {
			// Bukan alasan menggagalkan startup: kunci ikut lepas sendiri begitu
			// koneksinya benar-benar ditutup. Tapi operator harus tahu.
			logger.Error("gagal melepas advisory lock migrasi",
				slog.Int64("key", advisoryLockKey),
				slog.String("error", err.Error()),
			)
		}
	}, nil
}

// ensureLedger membuat tabel pembukuan kalau belum ada.
func ensureLedger(ctx context.Context, conn *pgxpool.Conn) error {
	const stmt = `create table if not exists ` + ledgerTable + ` (
    version    bigint      primary key,
    name       text        not null,
    checksum   text        not null,
    applied_at timestamptz not null default now()
)`
	if _, err := conn.Exec(ctx, stmt); err != nil {
		return fmt.Errorf("menyiapkan tabel %s: %w", ledgerTable, err)
	}
	return nil
}

// loadLedger membaca seluruh migrasi yang sudah tercatat.
func loadLedger(ctx context.Context, conn *pgxpool.Conn) (map[int64]appliedMigration, error) {
	rows, err := conn.Query(ctx, `select version, name, checksum, applied_at from `+ledgerTable)
	if err != nil {
		return nil, fmt.Errorf("membaca tabel %s: %w", ledgerTable, err)
	}
	defer rows.Close()

	out := map[int64]appliedMigration{}
	for rows.Next() {
		var a appliedMigration
		if err := rows.Scan(&a.version, &a.name, &a.checksum, &a.appliedAt); err != nil {
			return nil, fmt.Errorf("membaca baris %s: %w", ledgerTable, err)
		}
		out[a.version] = a
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("membaca tabel %s: %w", ledgerTable, err)
	}
	return out, nil
}

// checkLedgerAgainstFiles membandingkan pembukuan dengan file yang tertanam.
//
// Checksum yang berbeda berarti riwayat migrasi diedit: migrasi itu sudah jalan di
// database ini dengan isi yang lain, jadi tidak ada cara aman untuk melanjutkan.
// Perbaikannya selalu membuat migrasi baru, bukan mengubah yang lama.
func checkLedgerAgainstFiles(migrations []migration, ledger map[int64]appliedMigration, logger *slog.Logger) error {
	known := make(map[int64]struct{}, len(migrations))
	for _, m := range migrations {
		known[m.version] = struct{}{}

		prev, done := ledger[m.version]
		if !done {
			continue
		}
		if prev.checksum != m.checksum {
			return fmt.Errorf("%w: %s pernah diterapkan pada %s dengan checksum %s, tapi isi filenya sekarang berchecksum %s — jangan mengubah migrasi yang sudah jalan, buat migrasi baru",
				ErrChecksumMismatch, m.filename, prev.appliedAt.UTC().Format(time.RFC3339), prev.checksum, m.checksum)
		}
	}

	// Pembukuan memuat versi yang tidak ada filenya: biasanya biner ini lebih tua
	// dari databasenya (rollback aplikasi tanpa rollback skema). Bukan error — skema
	// yang lebih maju tetap kompatibel — tapi operator perlu tahu.
	for version, a := range ledger {
		if _, ok := known[version]; !ok {
			logger.Warn("database memuat migrasi yang tidak ada di biner ini",
				slog.Int64("version", version),
				slog.String("name", a.name),
			)
		}
	}
	return nil
}

// applyOne menjalankan satu migrasi berikut pencatatannya dalam satu transaksi.
func applyOne(ctx context.Context, conn *pgxpool.Conn, m migration) error {
	tx, err := conn.Begin(ctx)
	if err != nil {
		return fmt.Errorf("membuka transaksi untuk %s: %w", m.filename, err)
	}
	// Rollback setelah Commit tidak berbahaya (pgx menjawab ErrTxClosed yang kita
	// abaikan), jadi satu defer ini menutup semua jalur keluar. Context-nya dilepas
	// dari pembatalan supaya transaksi yang gagal karena timeout tetap dibersihkan.
	defer func() { _ = tx.Rollback(context.WithoutCancel(ctx)) }()

	if _, err := tx.Exec(ctx, m.sql); err != nil {
		return fmt.Errorf("menjalankan %s: %w", m.filename, err)
	}

	const record = `insert into ` + ledgerTable + ` (version, name, checksum) values ($1, $2, $3)`
	if _, err := tx.Exec(ctx, record, m.version, m.name, m.checksum); err != nil {
		return fmt.Errorf("mencatat %s ke %s: %w", m.filename, ledgerTable, err)
	}

	if err := tx.Commit(ctx); err != nil {
		return fmt.Errorf("commit %s: %w", m.filename, err)
	}
	return nil
}
