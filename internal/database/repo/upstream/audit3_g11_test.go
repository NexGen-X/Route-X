package upstream

import (
	"context"
	"errors"
	"testing"

	"github.com/jackc/pgx/v5/pgconn"
)

// TestConstraintPipelineCheckViolationBukan22P02 mereproduksi audit3 V-DT-17:
// constraint 0014 memakai (pipeline->>'attempts')::int, sehingga attempts
// non-numerik melempar 22P02 (invalid_text_representation) yang lolos dari
// pemetaan repo sebagai 500 buram — dan elemen models non-string lolos sama
// sekali. Sejak migrasi 0019, semua bentuk buruk itu adalah 23514
// (check_violation) dari constraint yang sama.
func TestConstraintPipelineCheckViolationBukan22P02(t *testing.T) {
	ctx := context.Background()
	pool := newTestPool(ctx, t)

	for _, tc := range []struct{ nama, pipeline string }{
		{"attempts teks", `{"strategy":"priority","attempts":"abc","models":["a"]}`},
		{"attempts pecahan", `{"strategy":"priority","attempts":1.5,"models":["a"]}`},
		{"attempts raksasa", `{"strategy":"priority","attempts":99999999999999999999,"models":["a"]}`},
		{"attempts string angka di luar rentang", `{"strategy":"priority","attempts":"21","models":["a"]}`},
		{"elemen models bukan string", `{"strategy":"priority","attempts":2,"models":[1,2]}`},
		{"elemen models campuran", `{"strategy":"priority","attempts":2,"models":["a",3]}`},
	} {
		t.Run(tc.nama, func(t *testing.T) {
			_, err := pool.Exec(ctx, `
				insert into routing_rules (name, pipeline) values ($1, $2::jsonb)`,
				"uji-0019", tc.pipeline)
			if err == nil {
				t.Fatalf("pipeline %s diterima padahal harus ditolak", tc.nama)
			}
			var pgErr *pgconn.PgError
			if !errors.As(err, &pgErr) {
				t.Fatalf("error bukan PgError: %v", err)
			}
			if pgErr.Code != "23514" {
				t.Errorf("SQLSTATE = %s (%s), mau 23514 check_violation — 22P02 berarti cast rapuh masih hidup",
					pgErr.Code, pgErr.Message)
			}
			if pgErr.ConstraintName != "routing_rules_pipeline_strategy_valid" {
				t.Errorf("constraint = %q, mau routing_rules_pipeline_strategy_valid", pgErr.ConstraintName)
			}
		})
	}

	// Bentuk yang sah tetap lolos, termasuk attempts yang ditulis sebagai
	// string angka oleh klien JSON yang longgar tipenya.
	if _, err := pool.Exec(ctx, `
		insert into routing_rules (name, pipeline)
		values ('sah', '{"strategy":"priority","attempts":2,"models":["a","b"]}'::jsonb)`); err != nil {
		t.Errorf("pipeline sah ditolak: %v", err)
	}
}

// TestIndeksErrorsGandaSudahDibuang memeriksa separuh lain migrasi 0019
// (audit3 V-DT-14): indeks 0006 yang tersubsumsi 0016 tidak boleh ada lagi,
// dan penggantinya tetap ada.
func TestIndeksErrorsGandaSudahDibuang(t *testing.T) {
	ctx := context.Background()
	pool := newTestPool(ctx, t)

	var lama, baru *string
	if err := pool.QueryRow(ctx, `select to_regclass('requests_errors_idx')::text`).Scan(&lama); err != nil {
		t.Fatalf("memeriksa indeks lama: %v", err)
	}
	if err := pool.QueryRow(ctx, `select to_regclass('requests_errors_all_idx')::text`).Scan(&baru); err != nil {
		t.Fatalf("memeriksa indeks pengganti: %v", err)
	}
	if lama != nil {
		t.Errorf("requests_errors_idx masih ada (%s) — indeks ganda belum dibuang", *lama)
	}
	if baru == nil {
		t.Error("requests_errors_all_idx hilang — pertanyaan error kehilangan indeksnya")
	}
}
