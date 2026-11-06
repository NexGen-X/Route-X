package admin

import (
	"context"
	"net/url"
	"os"
	"strings"
	"testing"
	"time"

	"github.com/jackc/pgx/v5/pgxpool"
)

// bukaPoolButa membuka pool ke database "postgres" bawaan pada server
// PostgreSQL uji yang sama — database itu tidak memuat satu pun tabel
// aplikasi, sehingga query repo apa pun ke sana pasti gagal. Dipakai
// test B-271 untuk menyuntik kegagalan RevokeAllOfUser secara bedah
// tanpa merusak schema uji (tabel sessions di schema uji menampung
// sesi login admin si penguji dan tidak boleh dihancurkan).
func bukaPoolButa(t *testing.T) *pgxpool.Pool {
	t.Helper()
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
	u, err := url.Parse(dsn)
	if err != nil {
		t.Fatalf("parse DSN: %v", err)
	}
	u.Path = "/postgres"
	q := u.Query()
	q.Del("search_path")
	u.RawQuery = q.Encode()

	ctx, cancel := context.WithTimeout(context.Background(), 15*time.Second)
	defer cancel()
	pool, err := pgxpool.New(ctx, u.String())
	if err != nil {
		t.Fatalf("buka pool buta: %v", err)
	}
	t.Cleanup(pool.Close)
	return pool
}
