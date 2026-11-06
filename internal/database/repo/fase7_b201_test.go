package repo

import (
	"errors"
	"testing"

	"github.com/jackc/pgx/v5/pgconn"
)

// Test B-201: SQLSTATE 22P02 (representasi teks tidak sah — slug
// bukan-UUID yang mencapai cast uuid[]) dipetakan ke
// ErrInvalidReference, bukan jatuh ke cabang default tanpa sentinel
// yang terbaca 500 di lapisan atas.
func TestFase7B201ErrMemetakan22P02(t *testing.T) {
	err := Err("uji", &pgconn.PgError{Code: "22P02", Message: "invalid input syntax for type uuid"})
	if !errors.Is(err, ErrInvalidReference) {
		t.Fatalf("Err(22P02) = %v, mau ErrInvalidReference", err)
	}
}
