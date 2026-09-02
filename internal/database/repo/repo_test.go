package repo

import (
	"context"
	"errors"
	"os"
	"strings"
	"testing"
	"time"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgconn"
	"github.com/jackc/pgx/v5/pgxpool"
)

func TestPageNormalize(t *testing.T) {
	tests := []struct {
		name  string
		limit int
		want  int
	}{
		{"nol memakai default", 0, DefaultPageLimit},
		{"negatif memakai default", -5, DefaultPageLimit},
		{"nilai wajar dipakai apa adanya", 25, 25},
		{"batas atas dipakai apa adanya", MaxPageLimit, MaxPageLimit},
		{"melebihi batas dipotong", MaxPageLimit + 1, MaxPageLimit},
		{"sangat besar dipotong", 1_000_000, MaxPageLimit},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			if got := (Page{Limit: tc.limit}).Normalize(); got != tc.want {
				t.Errorf("Normalize() = %d, mau %d", got, tc.want)
			}
		})
	}
}

func TestErrNil(t *testing.T) {
	if got := Err("operasi", nil); got != nil {
		t.Errorf("Err(nil) = %v, mau nil", got)
	}
}

func TestErrNoRows(t *testing.T) {
	err := Err("mengambil pengguna", pgx.ErrNoRows)
	if !errors.Is(err, ErrNotFound) {
		t.Fatalf("err = %v, mau ErrNotFound", err)
	}
	if !strings.Contains(err.Error(), "mengambil pengguna") {
		t.Errorf("pesan kehilangan keterangan operasi: %v", err)
	}
}

func TestErrTranslatesPgCodes(t *testing.T) {
	tests := []struct {
		name       string
		code       string
		constraint string
		want       error
	}{
		{"unique violation", "23505", "users_email_lower_key", ErrConflict},
		{"exclusion violation", "23P01", "budgets_overlap", ErrConflict},
		{"foreign key violation", "23503", "api_keys_owner_user_id_fkey", ErrInvalidReference},
		{"check violation", "23514", "providers_kind_valid", ErrConstraint},
		{"not null violation", "23502", "", ErrConstraint},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			pgErr := &pgconn.PgError{Code: tc.code, ConstraintName: tc.constraint}
			err := Err("menyimpan baris", pgErr)

			if !errors.Is(err, tc.want) {
				t.Fatalf("err = %v, mau %v", err, tc.want)
			}
			if tc.constraint != "" && !strings.Contains(err.Error(), tc.constraint) {
				t.Errorf("pesan tidak menyebut constraint %q: %v", tc.constraint, err)
			}
			if !strings.Contains(err.Error(), "menyimpan baris") {
				t.Errorf("pesan kehilangan keterangan operasi: %v", err)
			}
		})
	}
}

// Kode yang tidak dikenal tetap dilaporkan, tetapi hanya kodenya — pesan driver bisa
// memuat nilai baris.
func TestErrUnknownPgCodeReportsCodeOnly(t *testing.T) {
	pgErr := &pgconn.PgError{
		Code:    "42P01",
		Message: "relation \"tabel_yang_hilang\" does not exist",
	}
	err := Err("menjalankan query", pgErr)

	if !strings.Contains(err.Error(), "42P01") {
		t.Errorf("pesan tidak menyebut kode: %v", err)
	}
	for _, sentinel := range []error{ErrNotFound, ErrConflict, ErrInvalidReference, ErrConstraint} {
		if errors.Is(err, sentinel) {
			t.Errorf("kode tak dikenal keliru dipetakan ke %v", sentinel)
		}
	}
}

// Inti keputusan desain paket ini: pesan dan detail dari driver TIDAK diteruskan,
// karena keduanya bisa memuat nilai baris — untuk tabel seperti provider_credentials
// itu berarti membocorkan kredensial ke log.
func TestErrDoesNotLeakDriverMessageOrRowValues(t *testing.T) {
	pgErr := &pgconn.PgError{
		Code:           "23505",
		ConstraintName: "provider_credentials_provider_id_label_key",
		Message:        "duplicate key value violates unique constraint",
		Detail:         "Key (ciphertext)=(v1.abc.RAHASIA-YANG-BOCOR) already exists.",
		Hint:           "coba nilai lain untuk RAHASIA-YANG-BOCOR",
		Where:          "PL/pgSQL function ... RAHASIA-YANG-BOCOR",
	}
	err := Err("menyimpan kredensial", pgErr)

	for _, leak := range []string{"RAHASIA-YANG-BOCOR", "duplicate key value", "coba nilai lain"} {
		if strings.Contains(err.Error(), leak) {
			t.Errorf("pesan error membocorkan %q: %v", leak, err)
		}
	}
	// Nama constraint tetap ada karena itulah keterangan yang berguna bagi pengguna.
	if !strings.Contains(err.Error(), "provider_credentials_provider_id_label_key") {
		t.Errorf("nama constraint hilang: %v", err)
	}
}

// Error non-server diteruskan utuh, supaya errors.Is terhadap context masih bekerja —
// tanpa ini, pemanggil tidak bisa membedakan timeout dari kegagalan sungguhan.
func TestErrPreservesNonServerErrors(t *testing.T) {
	tests := []struct {
		name string
		err  error
	}{
		{"deadline terlampaui", context.DeadlineExceeded},
		{"context dibatalkan", context.Canceled},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			err := Err("menghubungi database", tc.err)
			if !errors.Is(err, tc.err) {
				t.Errorf("errors.Is(err, %v) = false; err = %v", tc.err, err)
			}
		})
	}
}

func TestConstraintName(t *testing.T) {
	pgErr := &pgconn.PgError{Code: "23505", ConstraintName: "users_email_lower_key"}
	if got := ConstraintName(pgErr); got != "users_email_lower_key" {
		t.Errorf("ConstraintName = %q", got)
	}
	// Juga harus bekerja pada error yang sudah dibungkus Err.
	if got := ConstraintName(Err("op", pgErr)); got != "users_email_lower_key" {
		t.Errorf("ConstraintName pada error terbungkus = %q", got)
	}
	for _, err := range []error{nil, errors.New("biasa"), context.Canceled} {
		if got := ConstraintName(err); got != "" {
			t.Errorf("ConstraintName(%v) = %q, mau kosong", err, got)
		}
	}
}

// --- InTx: butuh database sungguhan -----------------------------------------

func testPool(t *testing.T) (context.Context, *pgxpool.Pool) {
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
	pool, err := pgxpool.New(ctx, dsn)
	if err != nil {
		t.Skipf("tidak bisa terhubung ke Postgres: %v", err)
	}
	t.Cleanup(pool.Close)

	// Tabel sementara terikat sesi, jadi ia hilang sendiri saat koneksi dilepas dan
	// tidak pernah menyentuh skema aplikasi. Pool dibatasi satu koneksi agar seluruh
	// query test memakai sesi yang sama, syarat agar tabel temp terlihat.
	pool.Config().MaxConns = 1
	if _, err := pool.Exec(ctx, `create temp table if not exists tx_probe (n int primary key)`); err != nil {
		t.Skipf("tidak bisa membuat tabel uji: %v", err)
	}
	t.Cleanup(func() { _, _ = pool.Exec(context.Background(), `drop table if exists tx_probe`) })
	return ctx, pool
}

func TestInTxCommitsOnSuccess(t *testing.T) {
	ctx, pool := testPool(t)

	err := InTx(ctx, pool, func(q Querier) error {
		_, err := q.Exec(ctx, `insert into tx_probe (n) values (1)`)
		return err
	})
	if err != nil {
		t.Fatalf("InTx: %v", err)
	}

	var n int
	if err := pool.QueryRow(ctx, `select count(*) from tx_probe where n = 1`).Scan(&n); err != nil {
		t.Fatalf("query: %v", err)
	}
	if n != 1 {
		t.Errorf("baris tercommit = %d, mau 1", n)
	}
}

func TestInTxRollsBackOnError(t *testing.T) {
	ctx, pool := testPool(t)
	sentinel := errors.New("gagal di tengah")

	err := InTx(ctx, pool, func(q Querier) error {
		if _, err := q.Exec(ctx, `insert into tx_probe (n) values (2)`); err != nil {
			return err
		}
		return sentinel
	})
	if !errors.Is(err, sentinel) {
		t.Fatalf("err = %v, mau sentinel", err)
	}

	var n int
	if err := pool.QueryRow(ctx, `select count(*) from tx_probe where n = 2`).Scan(&n); err != nil {
		t.Fatalf("query: %v", err)
	}
	if n != 0 {
		t.Errorf("baris tersisa setelah rollback = %d, mau 0", n)
	}
}

// Panic di dalam transaksi tidak boleh meninggalkan transaksi terbuka yang menahan
// koneksi dan kunci sampai pool-nya mati.
func TestInTxRollsBackOnPanic(t *testing.T) {
	ctx, pool := testPool(t)

	func() {
		defer func() {
			if r := recover(); r == nil {
				t.Error("panic seharusnya diteruskan ke pemanggil")
			}
		}()
		_ = InTx(ctx, pool, func(q Querier) error {
			_, _ = q.Exec(ctx, `insert into tx_probe (n) values (3)`)
			panic("kegagalan tak terduga")
		})
	}()

	var n int
	if err := pool.QueryRow(ctx, `select count(*) from tx_probe where n = 3`).Scan(&n); err != nil {
		t.Fatalf("query setelah panic: %v", err)
	}
	if n != 0 {
		t.Errorf("baris tersisa setelah panic = %d, mau 0", n)
	}
}

// Querier harus benar-benar dipenuhi baik pool maupun transaksi — inilah alasan
// antarmuka ini ada.
func TestQuerierSatisfiedByPoolAndTx(t *testing.T) {
	ctx, pool := testPool(t)

	var _ Querier = pool

	tx, err := pool.Begin(ctx)
	if err != nil {
		t.Fatalf("Begin: %v", err)
	}
	defer func() { _ = tx.Rollback(ctx) }()

	var _ Querier = tx

	// Dan keduanya benar-benar bisa dipakai lewat antarmuka itu.
	for name, q := range map[string]Querier{"pool": pool, "tx": tx} {
		var one int
		if err := q.QueryRow(ctx, `select 1`).Scan(&one); err != nil || one != 1 {
			t.Errorf("%s lewat Querier: got %d, err %v", name, one, err)
		}
	}
	_ = time.Now
}

func TestCode(t *testing.T) {
	pgErr := &pgconn.PgError{Code: "23505", ConstraintName: "users_email_lower_key"}

	if got := Code(pgErr); got != "23505" {
		t.Errorf("Code pada error driver = %q, mau \"23505\"", got)
	}
	if got := Code(Err("op", pgErr)); got != "23505" {
		t.Errorf("Code pada error terbungkus = %q, mau \"23505\"", got)
	}
	for _, err := range []error{nil, errors.New("biasa"), context.Canceled, Err("op", pgx.ErrNoRows)} {
		if got := Code(err); got != "" {
			t.Errorf("Code(%v) = %q, mau kosong", err, got)
		}
	}
}

// Bentuk pesan *Error harus stabil: lapisan HTTP dan log bergantung padanya.
func TestErrorMessageShape(t *testing.T) {
	tests := []struct {
		name string
		err  *Error
		want string
	}{
		{"dengan constraint", &Error{Op: "menyimpan", Kind: ErrConflict, Constraint: "c_key"},
			"menyimpan: data sudah ada (constraint c_key)"},
		{"tanpa constraint", &Error{Op: "mengambil", Kind: ErrNotFound},
			"mengambil: data tidak ditemukan"},
		{"tak terpetakan dengan kode", &Error{Op: "query", Code: "42P01"},
			"query: kegagalan database (kode 42P01)"},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			if got := tc.err.Error(); got != tc.want {
				t.Errorf("Error() = %q, mau %q", got, tc.want)
			}
		})
	}
}
