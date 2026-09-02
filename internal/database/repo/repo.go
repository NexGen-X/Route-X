// Package repo memuat lapisan akses data. Satu tipe repository per domain, semuanya
// memakai query berparameter — tidak ada SQL yang dirakit dengan penggabungan string.
package repo

import (
	"context"
	"errors"
	"fmt"
	"strings"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgconn"
	"github.com/jackc/pgx/v5/pgxpool"
)

// Querier adalah antarmuka minimum yang dipenuhi baik *pgxpool.Pool maupun pgx.Tx.
//
// Setiap repository menyimpan Querier, bukan pool konkret, sehingga method yang sama
// bisa dipakai di dalam maupun di luar transaksi. Tanpa ini, operasi yang harus atomik
// (membuat pengguna sekaligus memberinya peran, memutar API key sekaligus mencatat
// audit) akan memerlukan method duplikat.
type Querier interface {
	Exec(ctx context.Context, sql string, args ...any) (pgconn.CommandTag, error)
	Query(ctx context.Context, sql string, args ...any) (pgx.Rows, error)
	QueryRow(ctx context.Context, sql string, args ...any) pgx.Row
}

// Sentinel error yang dipakai seluruh repository. Lapisan HTTP memetakannya ke kode
// status: ErrNotFound → 404, ErrConflict → 409, ErrInvalidReference → 400,
// ErrConstraint → 400.
var (
	// ErrNotFound berarti baris yang diminta tidak ada.
	ErrNotFound = errors.New("data tidak ditemukan")
	// ErrConflict berarti pelanggaran keunikan, mis. email atau nama yang sudah dipakai.
	ErrConflict = errors.New("data sudah ada")
	// ErrInvalidReference berarti referensi ke baris yang tidak ada.
	ErrInvalidReference = errors.New("referensi ke data yang tidak ada")
	// ErrConstraint berarti nilai melanggar aturan tabel.
	ErrConstraint = errors.New("nilai melanggar aturan tabel")
)

// Kode error PostgreSQL yang diterjemahkan. Lihat
// https://www.postgresql.org/docs/16/errcodes-appendix.html
const (
	pgUniqueViolation     = "23505"
	pgForeignKeyViolation = "23503"
	pgCheckViolation      = "23514"
	pgNotNullViolation    = "23502"
	pgExclusionViolation  = "23P01"
)

// Error adalah error terjemahan dari lapisan data.
//
// Tipe ini membawa sentinel, nama constraint, dan kode SQLSTATE, tetapi TIDAK membawa
// pesan, detail, atau hint dari driver. Ketiganya bisa memuat nilai baris — untuk
// tabel seperti provider_credentials itu berarti kredensial ikut masuk ke log begitu
// error dicetak. Nama constraint tetap dibawa karena itulah satu-satunya keterangan
// yang benar-benar berguna saat menyusun pesan bagi pengguna.
type Error struct {
	// Op adalah keterangan operasi yang gagal, mis. "menyimpan kredensial".
	Op string
	// Kind adalah salah satu sentinel: ErrNotFound, ErrConflict, ErrInvalidReference,
	// atau ErrConstraint. Nil untuk kegagalan yang tidak terpetakan.
	Kind error
	// Constraint adalah nama constraint yang dilanggar, bila ada.
	Constraint string
	// Code adalah SQLSTATE dari PostgreSQL, bila errornya berasal dari server.
	Code string
}

func (e *Error) Error() string {
	var b strings.Builder
	b.WriteString(e.Op)
	b.WriteString(": ")
	if e.Kind != nil {
		b.WriteString(e.Kind.Error())
	} else {
		b.WriteString("kegagalan database")
	}
	if e.Constraint != "" {
		b.WriteString(" (constraint ")
		b.WriteString(e.Constraint)
		b.WriteString(")")
	} else if e.Kind == nil && e.Code != "" {
		b.WriteString(" (kode ")
		b.WriteString(e.Code)
		b.WriteString(")")
	}
	return b.String()
}

// Unwrap membuka ke sentinel, sehingga errors.Is(err, ErrConflict) bekerja.
func (e *Error) Unwrap() error { return e.Kind }

// Err menerjemahkan error pgx menjadi *Error dengan sentinel yang sesuai.
func Err(op string, err error) error {
	if err == nil {
		return nil
	}
	if errors.Is(err, pgx.ErrNoRows) {
		return &Error{Op: op, Kind: ErrNotFound}
	}

	var pgErr *pgconn.PgError
	if errors.As(err, &pgErr) {
		e := &Error{Op: op, Constraint: pgErr.ConstraintName, Code: pgErr.Code}
		switch pgErr.Code {
		case pgUniqueViolation, pgExclusionViolation:
			e.Kind = ErrConflict
		case pgForeignKeyViolation:
			e.Kind = ErrInvalidReference
		case pgCheckViolation, pgNotNullViolation:
			e.Kind = ErrConstraint
		default:
			// Kode lain tidak dipetakan ke sentinel apa pun; hanya kodenya dilaporkan.
			e.Constraint = ""
		}
		return e
	}

	// Bukan error dari server: timeout, koneksi terputus, context dibatalkan. Ini aman
	// diteruskan dan penting agar errors.Is(err, context.DeadlineExceeded) tetap jalan.
	return fmt.Errorf("%s: %w", op, err)
}

// ConstraintName mengambil nama constraint dari error, "" bila bukan error constraint.
// Dipakai lapisan HTTP untuk memilih pesan yang tepat bagi pengguna.
func ConstraintName(err error) string {
	var dbErr *Error
	if errors.As(err, &dbErr) {
		return dbErr.Constraint
	}
	// Juga menerima error mentah dari driver, untuk pemanggil yang belum lewat Err.
	var pgErr *pgconn.PgError
	if errors.As(err, &pgErr) {
		return pgErr.ConstraintName
	}
	return ""
}

// Code mengambil SQLSTATE dari error, "" bila tidak berasal dari server.
func Code(err error) string {
	var dbErr *Error
	if errors.As(err, &dbErr) {
		return dbErr.Code
	}
	var pgErr *pgconn.PgError
	if errors.As(err, &pgErr) {
		return pgErr.Code
	}
	return ""
}

// InTx menjalankan fn di dalam satu transaksi, commit bila fn mengembalikan nil dan
// rollback bila tidak.
//
// Rollback juga dijalankan lewat defer sebagai jaring pengaman terhadap panic; setelah
// commit berhasil, rollback tidak berefek apa-apa sehingga aman dipanggil dua kali.
func InTx(ctx context.Context, pool *pgxpool.Pool, fn func(Querier) error) error {
	tx, err := pool.Begin(ctx)
	if err != nil {
		return Err("memulai transaksi", err)
	}
	// Context terpisah tidak dipakai di sini: bila context sudah dibatalkan, koneksinya
	// memang harus dilepas, dan pgx menandai transaksi sebagai gagal.
	defer func() { _ = tx.Rollback(ctx) }()

	if err := fn(tx); err != nil {
		return err
	}
	if err := tx.Commit(ctx); err != nil {
		return Err("commit transaksi", err)
	}
	return nil
}

// Page adalah parameter paginasi keyset.
//
// Keyset dipilih daripada OFFSET karena log request bisa berisi jutaan baris: OFFSET
// memaksa PostgreSQL membaca lalu membuang seluruh baris sebelum halaman yang diminta,
// sehingga halaman ke-1000 jauh lebih lambat daripada halaman pertama. Keyset selalu
// berbiaya sama.
type Page struct {
	// Limit adalah jumlah maksimum baris. Nol memakai DefaultPageLimit.
	Limit int
	// Cursor adalah penanda posisi dari halaman sebelumnya. Kosong berarti dari awal.
	Cursor string
}

// Batas paginasi.
const (
	DefaultPageLimit = 50
	MaxPageLimit     = 500
)

// Normalize mengembalikan limit yang sudah dibatasi ke rentang yang aman. Batas atas
// mencegah satu permintaan menarik seluruh tabel ke memori.
func (p Page) Normalize() int {
	switch {
	case p.Limit <= 0:
		return DefaultPageLimit
	case p.Limit > MaxPageLimit:
		return MaxPageLimit
	default:
		return p.Limit
	}
}
