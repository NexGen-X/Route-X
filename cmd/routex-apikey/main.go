// Command routex-apikey membuat dan mendaftar API key gateway Route-X dari terminal.
//
// Tanpa perkakas ini, satu-satunya jalur pembuatan key adalah dashboard admin, yang
// butuh browser, login, dan token CSRF — sehingga skrip provisioning headless terpaksa
// merakit SQL dan menghitung HMAC sendiri, di mana setiap langkahnya adalah peluang
// untuk salah: prefix yang harus eksak, body 43 karakter base62, dan pepper base64 yang
// harus didekode dulu. Perkakas ini memakai fungsi yang sama persis dengan server:
// config.LoadKeyTool untuk menerjemahkan pepper, security.GenerateAPIKey untuk hash, dan
// keys.Repo.Create untuk menyimpan. Karena itu constraint database (prefix, last4,
// cakupan) terpenuhi dengan sendirinya dan tidak ada peluang salah hitung HMAC.
package main

import (
	"context"
	"errors"
	"flag"
	"fmt"
	"io"
	"log/slog"
	"os"
	"os/signal"
	"strings"
	"syscall"
	"text/tabwriter"
	"time"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"

	"github.com/NexGen-X/Route-X/internal/config"
	"github.com/NexGen-X/Route-X/internal/database"
	"github.com/NexGen-X/Route-X/internal/database/repo"
	"github.com/NexGen-X/Route-X/internal/database/repo/keys"
	"github.com/NexGen-X/Route-X/internal/security"
)

// Subcommand yang dikenali.
const (
	subCreate = "create"
	subList   = "list"
)

// Ukuran pool koneksi untuk perkakas sekali jalan ini. Jauh lebih kecil dari server:
// pekerjaannya satu query, bukan melayani ribuan request bersamaan.
const (
	cliDBMaxConns = 3
	cliDBMinConns = 1
	listLimit     = 100
)

func main() {
	ctx, cancel := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer cancel()

	os.Exit(run(ctx, os.Args[1:], os.Stdout, os.Stderr))
}

// run menjalankan subcommand yang diminta dan mengembalikan kode keluarnya.
//
// Pemisahan dari main() agar flag parsing, koneksi database, dan penulisan hasil bisa
// diuji tanpa harus memulai proses baru.
func run(ctx context.Context, args []string, out, errOut io.Writer) int {
	if len(args) == 0 {
		usage(errOut)
		return 1
	}

	switch args[0] {
	case subCreate, subList:
	case "-h", "--help", "help":
		usage(out)
		return 0
	default:
		fmt.Fprintf(errOut, "Error: subcommand %q tidak dikenali.\n\n", args[0])
		usage(errOut)
		return 1
	}

	t := &keyTool{out: out, errOut: errOut}
	defer t.close()

	switch args[0] {
	case subCreate:
		return t.create(ctx, args[1:])
	case subList:
		return t.list(ctx, args[1:])
	default:
		usage(errOut)
		return 1
	}
}

// keyTool adalah perkakas pembuatan API key di atas pool koneksi.
//
// out menerima hasil yang sengaja ditujukan untuk dipipakan (nilai key pada create,
// tabel pada list); errOut menerima hal lainnya — termasuk peringatan — supaya
// `routex-apikey create --name ci > key.txt` benar-benar hanya menangkap keynya.
//

type keyTool struct {
	pool    *pgxpool.Pool
	repo    *keys.Repo
	closeFn func()
	out     io.Writer
	errOut  io.Writer
}

// ensurePool menyiapkan pool koneksi dan repository bila belum ada.
//
// flagDBURL diambil dari --db-url dan menang atas DATABASE_URL. Konfigurasi tetap
// dimuat lewat config.LoadKeyTool supaya pepper diterjemahkan memakai reader yang sama
// persis dengan server — walau --db-url dipakai, pepper tetap datang dari
// API_KEY_PEPPER, karena tidak ada pilihan yang valid lainnya.
func (t *keyTool) ensurePool(ctx context.Context, flagDBURL string) error {
	if t.pool != nil {
		return nil
	}

	cfg, err := config.LoadKeyTool()
	if err != nil {
		return fmt.Errorf("konfigurasi tidak valid: %w", err)
	}

	dsn := cfg.DatabaseURL.Reveal()
	if flagDBURL != "" {
		dsn = flagDBURL
	}

	db, err := connectDB(ctx, dsn)
	if err != nil {
		return fmt.Errorf("tidak bisa terhubung ke basis data: %w", err)
	}
	// Hanya fungsi tutupnya yang disimpan. *database.DB sendiri memuat DSN sebagai
	// security.Secret; kalau disimpan sebagai field tak diekspor, redaction lint repo
	// menolaknya karena keyTool tidak punya metode redaksi sendiri.
	t.closeFn = db.Close
	t.pool = db.Pool

	repo, err := keys.New(db.Pool, cfg.APIKeyPepper)
	if err != nil {
		return err
	}
	t.repo = repo
	return nil
}

// close melepas koneksi yang dibuka ensurePool. Koneksi dari pemanggil tidak ikut
// ditutup — pemiliknya yang bertanggung jawab atasnya.
func (t *keyTool) close() {
	if t.closeFn != nil {
		t.closeFn()
		t.closeFn = nil
	}
}

// create menjalankan subcommand create.
func (t *keyTool) create(ctx context.Context, args []string) int {
	fs := flag.NewFlagSet(subCreate, flag.ContinueOnError)
	fs.SetOutput(t.errOut)

	var (
		name  = fs.String("name", "", "Nama deskriptif key, mis. \"ci-pipeline\" (wajib)")
		live  = fs.Bool("live", false, "Buat key produksi dengan prefix sk_live_; tanpa flag ini dipakai sk_test_")
		owner = fs.String("owner", "", "ID pengguna pemilik key; kosong berarti pengguna pertama yang terdaftar")
		dbURL = fs.String("db-url", "", "URL basis data PostgreSQL (menimpa DATABASE_URL)")
	)

	var scopes scopeFlag
	fs.Var(&scopes, "scope", "Cakupan key (boleh diulang); cakupan yang valid: inference, models:read, usage:read, admin:read, admin:write")

	if err := fs.Parse(args); err != nil {
		return 1
	}

	// Tanpa satu pun --scope, key dibuat dengan cakupan dasar inference.
	if len(scopes) == 0 {
		scopes = scopeFlag{keys.ScopeInference}
	}

	opts := createOptions{
		name:   *name,
		live:   *live,
		scopes: scopes,
		owner:  *owner,
	}
	if err := opts.validate(); err != nil {
		fmt.Fprintf(t.errOut, "Error: %v\n", err)
		fs.Usage()
		return 1
	}

	if err := t.ensurePool(ctx, *dbURL); err != nil {
		fmt.Fprintf(t.errOut, "Error: %v\n", err)
		return 1
	}

	ownerID := opts.owner
	if ownerID == "" {
		var err error
		ownerID, err = defaultOwner(ctx, t.pool)
		if err != nil {
			fmt.Fprintf(t.errOut, "Error: %v\n", err)
			return 1
		}
	}

	created, err := t.repo.Create(ctx, keys.CreateParams{
		Name:        opts.name,
		OwnerUserID: ownerID,
		Live:        opts.live,
		Scopes:      opts.scopes,
	})
	if err != nil {
		fmt.Fprintf(t.errOut, "Error: gagal membuat API key: %v\n", err)
		return 1
	}

	lingkungan := security.KeyPrefixTest
	if opts.live {
		lingkungan = security.KeyPrefixLive
	}

	// Ringkasan ke stderr; nilai key tidak boleh campur di sini agar pipa ke stdout
	// hanya menangkap keynya.
	fmt.Fprintf(t.errOut, "ID         : %s\n", created.Key.ID)
	fmt.Fprintf(t.errOut, "Nama       : %s\n", created.Key.Name)
	fmt.Fprintf(t.errOut, "Lingkungan : %s\n", lingkungan)
	fmt.Fprintf(t.errOut, "Tersamar   : %s\n", created.Key.Masked())
	fmt.Fprintf(t.errOut, "Pemilik    : %s\n", ownerID)
	fmt.Fprintf(t.errOut, "Cakupan    : %s\n", strings.Join(opts.scopes, ", "))
	fmt.Fprintf(t.errOut, "Dibuat     : %s\n", created.Key.CreatedAt.Format(time.RFC3339))
	fmt.Fprintln(t.errOut)
	fmt.Fprintln(t.errOut, "⚠  Key ini hanya ditampilkan sekali. Simpan sekarang.")

	fmt.Fprintln(t.out, created.Raw.Reveal())
	return 0
}

// list menjalankan subcommand list.
//
// Tidak ada nilai key yang bisa ditampilkan: yang tersimpan hanyalah hash, dan hash
// tidak bisa dibalik ke key aslinya. Yang ditampilkan adalah bentuk tersamarnya.
func (t *keyTool) list(ctx context.Context, args []string) int {
	fs := flag.NewFlagSet(subList, flag.ContinueOnError)
	fs.SetOutput(t.errOut)

	dbURL := fs.String("db-url", "", "URL basis data PostgreSQL (menimpa DATABASE_URL)")

	if err := fs.Parse(args); err != nil {
		return 1
	}

	if err := t.ensurePool(ctx, *dbURL); err != nil {
		fmt.Fprintf(t.errOut, "Error: %v\n", err)
		return 1
	}

	rows, _, err := t.repo.List(ctx, keys.ListFilter{}, repo.Page{Limit: listLimit})
	if err != nil {
		fmt.Fprintf(t.errOut, "Error: gagal mendaftar API key: %v\n", err)
		return 1
	}
	if len(rows) == 0 {
		fmt.Fprintln(t.errOut, "Belum ada API key di basis data.")
		return 0
	}

	tw := tabwriter.NewWriter(t.out, 0, 0, 2, ' ', 0)
	fmt.Fprintf(tw, "ID\tNAMA\tKEY\tSTATUS\tCAKUPAN\tDIBUAT\n")
	for _, k := range rows {
		fmt.Fprintf(tw, "%s\t%s\t%s\t%s\t%s\t%s\n",
			k.ID,
			k.Name,
			k.Masked(),
			k.Status,
			strings.Join(k.Scopes, ","),
			k.CreatedAt.Format(time.RFC3339),
		)
	}
	if err := tw.Flush(); err != nil {
		fmt.Fprintf(t.errOut, "Error: menulis daftar API key: %v\n", err)
		return 1
	}
	return 0
}

// createOptions adalah masukan subcommand create.
type createOptions struct {
	name   string
	live   bool
	scopes []string
	owner  string
}

// validate memeriksa masukan create sebelum menyentuh database.
func (o createOptions) validate() error {
	if strings.TrimSpace(o.name) == "" {
		return errors.New("--name wajib diisi")
	}
	if strings.TrimSpace(o.name) != o.name {
		return errors.New("--name tidak boleh diawali atau diakhiri spasi")
	}
	for _, s := range o.scopes {
		if !keys.ValidScope(s) {
			return fmt.Errorf("cakupan %q tidak dikenali; yang valid: %s",
				s, strings.Join(keys.KnownScopes, ", "))
		}
	}
	return nil
}

// defaultOwner mengambil pengguna pertama yang terdaftar, sebagai pemilik default.
//
// Default ini ada karena di instalasi single-admin hampir selalu hanya ada satu
// pengguna; memaksanya mengetik UUID pengguna sendiri hanya menambah langkah yang
// gagal karena salah salin.
func defaultOwner(ctx context.Context, q repo.Querier) (string, error) {
	var id string
	err := q.QueryRow(ctx, `select id::text from users order by created_at, email limit 1`).Scan(&id)
	if errors.Is(err, pgx.ErrNoRows) {
		return "", errors.New("tidak ada pengguna di basis data; sebutkan pemiliknya lewat --owner")
	}
	if err != nil {
		return "", fmt.Errorf("mencari pengguna pertama: %w", err)
	}
	return id, nil
}

// scopeFlag adalah flag berulang: setiap kemunculan --scope menambah cakupan.
//
// Koma juga diterima sebagai pemisah supaya --scope inference,usage:read sekali jalan,
// karena cakupan sendiri memakai titik dua dan tidak ada tabrakan dengan pemisah ini.
type scopeFlag []string

func (s *scopeFlag) String() string { return strings.Join(*s, ",") }

func (s *scopeFlag) Set(v string) error {
	for _, part := range strings.Split(v, ",") {
		if part = strings.TrimSpace(part); part != "" {
			*s = append(*s, part)
		}
	}
	return nil
}

// connectDB membuka pool koneksi memakai pengaturan yang sama dengan server.
//
// database.Connect dipakai, bukan pgxpool langsung, agar pesan errornya sudah dibersihkan
// dari connection string — DSN memuat password dan tidak ada alasan untuk menampilkannya
// ke operator hanya karena koneksi gagal.
func connectDB(ctx context.Context, dsn string) (*database.DB, error) {
	return database.Connect(ctx, &config.Config{
		DatabaseURL: security.Secret(dsn),
		DBMaxConns:  cliDBMaxConns,
		DBMinConns:  cliDBMinConns,
	}, slog.New(slog.NewJSONHandler(io.Discard, nil)))
}

func usage(out io.Writer) {
	fmt.Fprint(out, `routex-apikey — perkakas pembuatan API key gateway Route-X

Pemakaian:
  routex-apikey <subcommand> [flag]

Subcommand:
  create              Membuat API key baru
  list                Mendaftar API key yang ada (tanpa nilai key)

Flag create:
  -name string        Nama deskriptif key, mis. "ci-pipeline" (wajib)
  -live               Buat key produksi dengan prefix sk_live_; tanpa flag ini dipakai sk_test_
  -scope string       Cakupan key (boleh diulang); cakupan yang valid:
                      inference, models:read, usage:read, admin:read, admin:write
  -owner string       ID pengguna pemilik key; kosong berarti pengguna pertama yang terdaftar
  -db-url string      URL basis data (menimpa DATABASE_URL)

Contoh:
  routex-apikey create --name ci-pipeline
  routex-apikey create --name produksi --live --scope admin:read > key.txt
  routex-apikey create --name billing --live --scope inference --scope usage:read
  routex-apikey list

Variabel lingkungan:
  DATABASE_URL        URL koneksi PostgreSQL (wajib jika --db-url tidak diberikan)
  API_KEY_PEPPER      Pepper HMAC API key, base64 (wajib — dipakai server yang sama)
`)
}
