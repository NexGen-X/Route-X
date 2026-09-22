package upstream

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"slices"
	"strings"
	"testing"
	"time"

	"github.com/jackc/pgx/v5/pgconn"

	"github.com/NexGen-X/Route-X/internal/database/repo"
)

// pipelineShape adalah bentuk resep combo menurut package ini. Didefinisikan lokal
// karena meminjam router.ComboPipeline akan membuat paket itu mengimpor paket ini
// secara melingkar — dan yang diuji di sini adalah transfer jsonb ke []byte, bukan
// penguraiannya (urusan router.RuleFromRow).
type pipelineShape struct {
	Strategy string   `json:"strategy"`
	Attempts int      `json:"attempts"`
	Models   []string `json:"models"`
}

// seedPipelineRule menyisipkan aturan langsung dengan kolom pipeline dan/atau
// virtual_alias yang diminta, lalu mengembalikan pengenalnya.
//
// Dilewati repo.Create karena pipeline memang belum bisa ditulis lewat sana pada PR #1
// — yang diuji di sini adalah pembacaan kolom yang dipasang migrasi 0014, bukan cara
// isinya dibuat. pipeline kosong berarti NULL (jalur lama).
func seedPipelineRule(ctx context.Context, t *testing.T, q repo.Querier, name, pipeline, alias string) string {
	t.Helper()

	var aliasArg any
	if alias != "" {
		aliasArg = alias
	}
	var pipelineArg any
	if pipeline != "" {
		pipelineArg = pipeline
	}

	var id string
	err := q.QueryRow(ctx, `
		insert into routing_rules (name, pipeline, virtual_alias)
		values ($1, $2::jsonb, $3)
		returning id::text`, name, pipelineArg, aliasArg).Scan(&id)
	if err != nil {
		t.Fatalf("menyisipkan aturan pipeline %s: %v", name, err)
	}
	return id
}

// constraintName mengeluarkan nama constraint dari error PostgreSQL, atau "" bila
// errornya bukan pelanggaran constraint. Dipakai supaya pesan test menyebut persis
// constraint mana yang melindungi setiap aturan.
func constraintName(t *testing.T, err error) string {
	t.Helper()
	var pgErr *pgconn.PgError
	if errors.As(err, &pgErr) {
		return pgErr.ConstraintName
	}
	return ""
}

// Kolom pipeline dan virtual_alias yang dipasang migrasi 0014 harus terbaca apa adanya
// lewat repo, baik di Get (jalur dashboard) maupun di ActiveRules (jalur request).
func TestRoutingPipelineBacaIntegration(t *testing.T) {
	if testing.Short() {
		t.Skip("butuh PostgreSQL")
	}
	ctx, cancel := context.WithTimeout(context.Background(), 90*time.Second)
	defer cancel()

	pool := newTestPool(ctx, t)
	rules := NewRoutingRepo(pool)

	const resep = `{"strategy":"round_robin","attempts":3,"models":["gpt-4o-mini","gemini-2.5-flash","claude-3-haiku"]}`
	comboID := seedPipelineRule(ctx, t, pool, "combo baca", resep, "murah-cerdas")
	lamaID := seedPipelineRule(ctx, t, pool, "model saja baca", "", "")

	t.Run("Get mengembalikan pipeline dan virtual_alias", func(t *testing.T) {
		got, err := rules.Get(ctx, comboID)
		if err != nil {
			t.Fatalf("Get: %v", err)
		}
		if len(got.Pipeline) == 0 {
			t.Fatal("Pipeline kosong padahal baris membawa resep combo")
		}
		var shape pipelineShape
		if err := json.Unmarshal(got.Pipeline, &shape); err != nil {
			t.Fatalf("mengurai pipeline yang dibaca: %v", err)
		}
		switch {
		case shape.Strategy != "round_robin":
			t.Errorf("strategy = %q, ingin round_robin", shape.Strategy)
		case shape.Attempts != 3:
			t.Errorf("attempts = %d, ingin 3 (anggaran total lintas model)", shape.Attempts)
		case len(shape.Models) != 3 || shape.Models[0] != "gpt-4o-mini":
			t.Errorf("models = %v, ingin 3 model terurut seperti ditulis", shape.Models)
		}
		if got.VirtualAlias == nil || *got.VirtualAlias != "murah-cerdas" {
			t.Errorf("VirtualAlias = %v, ingin %q", got.VirtualAlias, "murah-cerdas")
		}
	})

	t.Run("Get aturan tanpa pipeline membaca nil, bukan error", func(t *testing.T) {
		got, err := rules.Get(ctx, lamaID)
		if err != nil {
			t.Fatalf("Get: %v", err)
		}
		// NULL jsonb dipindai sebagai slice nil — panjang nol, bukan error. Inilah yang
		// membuat aturan lama tetap terbaca sebagai Model Only tanpa penanganan khusus.
		if got.Pipeline != nil {
			t.Errorf("Pipeline = %v, ingin nil untuk aturan tanpa resep", got.Pipeline)
		}
		if got.VirtualAlias != nil {
			t.Errorf("VirtualAlias = %v, ingin nil", got.VirtualAlias)
		}
	})

	t.Run("ActiveRules membawa pipeline di jalur request", func(t *testing.T) {
		active, err := rules.ActiveRules(ctx)
		if err != nil {
			t.Fatalf("ActiveRules: %v", err)
		}
		for _, rule := range active {
			if rule.ID == comboID && len(rule.Pipeline) == 0 {
				t.Errorf("aturan %q: pipeline hilang di ActiveRules", rule.Name)
			}
			if rule.ID == lamaID && rule.Pipeline != nil {
				t.Errorf("aturan %q: pipeline tidak nil di ActiveRules", rule.Name)
			}
		}
	})
}

// Constraint pipeline di migrasi 0014 hanya menerima 4 strategi UI, BUKAN 6 nilai enum
// strategy. Kedua nilai tambahan (weighted, capability) sah di kolom strategy lama,
// jadi test ini juga membuktikan kedua constraint tidak tertukar: pipeline dengan
// weighted ditolak meski nilai itu diterima routing_rules_strategy_valid.
func TestRoutingPipelineConstraintIntegration(t *testing.T) {
	if testing.Short() {
		t.Skip("butuh PostgreSQL")
	}
	ctx, cancel := context.WithTimeout(context.Background(), 90*time.Second)
	defer cancel()

	pool := newTestPool(ctx, t)
	rules := NewRoutingRepo(pool)

	// Strategi yang diterima: 4 tombol UI.
	for _, strategi := range []string{"priority", "round_robin", "lowest_latency", "lowest_cost"} {
		t.Run("strategi UI "+strategi+" diterima", func(t *testing.T) {
			resep := fmt.Sprintf(`{"strategy":%q,"attempts":2,"models":["a","b"]}`, strategi)
			// Nama harus unik antar test karena routing_rules_name_key unique.
			id := seedPipelineRule(ctx, t, pool, "combo-"+strings.ReplaceAll(strategi, "_", "-"), resep, "")
			if id == "" {
				t.Fatal("pengenal aturan kosong")
			}
		})
	}

	// Ambang batas yang dijanjikan komentar migrasi: models 1..8, attempts 1..20.
	t.Run("ambang batas models dan attempts diterima", func(t *testing.T) {
		// 8 model adalah batas atas, bukan nilai acak: UI tidak menyuguhkan lebih.
		models := make([]string, 8)
		for i := range models {
			models[i] = fmt.Sprintf("m%d", i)
		}
		resep := fmt.Sprintf(`{"strategy":"priority","attempts":20,"models":["%s"]}`,
			strings.Join(models, `","`))
		if id := seedPipelineRule(ctx, t, pool, "combo-ambang-atas", resep, ""); id == "" {
			t.Fatal("pengenal aturan kosong")
		}
	})

	// Penolakan. Setiap kasus harus menyebut persis constraint mana yang melindunginya,
	// karena "data tidak valid" saja tidak memberi tahu operator apa yang harus diperbaiki.
	for _, tc := range []struct {
		name       string
		pipeline   string
		constraint string
	}{
		{"strategi weighted hanya sah di kolom lama",
			`{"strategy":"weighted","attempts":2,"models":["a","b"]}`, "routing_rules_pipeline_strategy_valid"},
		{"strategi capability hanya sah di kolom lama",
			`{"strategy":"capability","attempts":2,"models":["a","b"]}`, "routing_rules_pipeline_strategy_valid"},
		{"strategi tak dikenal",
			`{"strategy":"acak","attempts":2,"models":["a","b"]}`, "routing_rules_pipeline_strategy_valid"},
		{"models kosong",
			`{"strategy":"priority","attempts":2,"models":[]}`, "routing_rules_pipeline_strategy_valid"},
		{"models lebih dari 8",
			`{"strategy":"priority","attempts":2,"models":["1","2","3","4","5","6","7","8","9"]}`,
			"routing_rules_pipeline_strategy_valid"},
		{"attempts di bawah satu",
			`{"strategy":"priority","attempts":0,"models":["a","b"]}`, "routing_rules_pipeline_strategy_valid"},
		{"attempts di atas dua puluh",
			`{"strategy":"priority","attempts":21,"models":["a","b"]}`, "routing_rules_pipeline_strategy_valid"},
		{"pipeline bukan objek",
			`["prioritas"]`, "routing_rules_pipeline_strategy_valid"},
		{"models bukan array",
			`{"strategy":"priority","attempts":2,"models":"a"}`, "routing_rules_pipeline_strategy_valid"},
		{"attempts hilang sama sekali",
			`{"strategy":"priority","models":["a","b"]}`, "routing_rules_pipeline_strategy_valid"},
		{"attempts null secara eksplisit",
			`{"strategy":"priority","attempts":null,"models":["a","b"]}`, "routing_rules_pipeline_strategy_valid"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			_, err := pool.Exec(ctx, `
				insert into routing_rules (name, pipeline)
				values ($1, $2::jsonb)`, "ditolak-"+strings.ReplaceAll(tc.name, " ", "-"), tc.pipeline)
			if err == nil {
				t.Fatalf("pipeline %q diterima padahal harus ditolak", tc.pipeline)
			}
			if got := constraintName(t, err); !strings.Contains(got, tc.constraint) {
				t.Errorf("constraint = %q, ingin %q (error: %v)", got, tc.constraint, err)
			}
		})
	}

	t.Run("virtual_alias ganda ditolak", func(t *testing.T) {
		seedPipelineRule(ctx, t, pool, "alias pertama", `{"strategy":"priority","attempts":2,"models":["a","b"]}`, "sama")

		_, err := pool.Exec(ctx, `
			insert into routing_rules (name, virtual_alias)
			values ('alias kedua', 'sama')`)
		if err == nil {
			t.Fatal("virtual_alias ganda diterima")
		}
		// Partial unique ditegakkan sebagai index unik (lihat komentar migrasi 0014),
		// jadi namanya tercatat di index, bukan di constraint.
		var pgErr *pgconn.PgError
		if !errors.As(err, &pgErr) {
			t.Fatalf("error bukan PgError: %v", err)
		}
		if !strings.Contains(pgErr.ConstraintName, "routing_rules_virtual_alias_idx") {
			t.Errorf("index pelanggaran = %q, ingin routing_rules_virtual_alias_idx (error: %v)",
				pgErr.ConstraintName, err)
		}
	})

	// NULL alias bukan pelanggaran unique: beberapa aturan boleh tidak punya alias
	// sekaligus. Tanpa partial unique, kolom yang nullable ini akan menolak baris kedua.
	t.Run("beberapa aturan tanpa alias diperbolehkan", func(t *testing.T) {
		for _, nama := range []string{"tanpa alias a", "tanpa alias b"} {
			if id := seedPipelineRule(ctx, t, pool, nama, "", ""); id == "" {
				t.Fatal("pengenal aturan kosong")
			}
		}
	})

	// Aturan lama yang tidak menyentuh pipeline sama sekali tetap sah — ini adalah
	// janji utama migrasi 0014 dan alasan kolomnya dibuat nullable.
	t.Run("aturan lama tanpa pipeline tetap sah", func(t *testing.T) {
		if _, err := rules.Create(ctx, CreateRoutingRuleParams{Name: "aturan lawas"}); err != nil {
			t.Fatalf("Create aturan tanpa pipeline: %v", err)
		}
	})

	// Constraint memakai cast (pipeline->>'attempts')::int, jadi attempts non-numerik
	// ditolak saat CAST — sebelum CHECK sempat dievaluasi. Hasil akhirnya sama dengan
	// pelanggaran constraint (pipeline tidak tersimpan), tapi laporannya SQLSTATE
	// 22P02, bukan 23514. Test terpisah ini menjaga kedua jawaban itu tidak tertukar.
	t.Run("attempts non-numerik ditolak saat cast, bukan saat check", func(t *testing.T) {
		_, err := pool.Exec(ctx, `
			insert into routing_rules (name, pipeline)
			values ('attempts teks', '{"strategy":"priority","attempts":"banyak","models":["a","b"]}'::jsonb)`)
		if err == nil {
			t.Fatal("attempts non-numerik diterima")
		}
		var pgErr *pgconn.PgError
		if !errors.As(err, &pgErr) {
			t.Fatalf("error bukan PgError: %v", err)
		}
		if pgErr.Code != "22P02" {
			t.Errorf("SQLSTATE = %q, ingin 22P02 (invalid input syntax for type integer): %v",
				pgErr.Code, err)
		}
	})
}

// Migrasi 0014 harus benar-benar memasang kolom dan constraintnya di schema test,
// termasuk saat dijalankan berurutan dengan seluruh migrasi sebelumnya. newTestPool
// memakai database.Migrate penuh, jadi test ini adalah bukti rantai migrasi utuh.
func TestRoutingPipelineMigrasi0014Integration(t *testing.T) {
	if testing.Short() {
		t.Skip("butuh PostgreSQL")
	}
	ctx, cancel := context.WithTimeout(context.Background(), 90*time.Second)
	defer cancel()

	pool := newTestPool(ctx, t)

	rows, err := pool.Query(ctx, `
		select column_name
		from information_schema.columns
		where table_schema = current_schema()
		  and table_name = 'routing_rules'
		  and column_name in ('pipeline', 'virtual_alias')
		order by column_name`)
	if err != nil {
		t.Fatalf("membaca daftar kolom: %v", err)
	}
	var kolom []string
	for rows.Next() {
		var nama string
		if err := rows.Scan(&nama); err != nil {
			t.Fatalf("membaca baris kolom: %v", err)
		}
		kolom = append(kolom, nama)
	}
	if err := rows.Err(); err != nil {
		t.Fatalf("menutup pembacaan kolom: %v", err)
	}
	rows.Close()
	if len(kolom) != 2 {
		t.Fatalf("kolom pipeline/virtual_alias = %v, ingin keduanya ada di schema test", kolom)
	}

	// Constraint CHECK pipeline ada di pg_constraint.
	rows, err = pool.Query(ctx, `
		select conname
		from pg_constraint
		where conrelid = 'routing_rules'::regclass
		order by conname`)
	if err != nil {
		t.Fatalf("membaca daftar constraint: %v", err)
	}
	var constraints []string
	for rows.Next() {
		var nama string
		if err := rows.Scan(&nama); err != nil {
			t.Fatalf("membaca baris constraint: %v", err)
		}
		constraints = append(constraints, nama)
	}
	if err := rows.Err(); err != nil {
		t.Fatalf("menutup pembacaan constraint: %v", err)
	}
	rows.Close()

	// Partial unique alias ditegakkan sebagai index unik (lihat komentar migrasi 0014),
	// jadi ia dicari di pg_class/pg_indexes, bukan pg_constraint.
	rows, err = pool.Query(ctx, `
		select indexname
		from pg_indexes
		where schemaname = current_schema()
		  and tablename = 'routing_rules'
		order by indexname`)
	if err != nil {
		t.Fatalf("membaca daftar index: %v", err)
	}
	var indexes []string
	for rows.Next() {
		var nama string
		if err := rows.Scan(&nama); err != nil {
			t.Fatalf("membaca baris index: %v", err)
		}
		indexes = append(indexes, nama)
	}
	if err := rows.Err(); err != nil {
		t.Fatalf("menutup pembacaan index: %v", err)
	}
	rows.Close()

	for _, want := range []string{"routing_rules_pipeline_strategy_valid"} {
		if !slices.Contains(constraints, want) {
			t.Errorf("constraint %q tidak terpasang (daftar: %s)", want, strings.Join(constraints, ", "))
		}
	}
	if want := "routing_rules_virtual_alias_idx"; !slices.Contains(indexes, want) {
		t.Errorf("index %q tidak terpasang (daftar: %s)", want, strings.Join(indexes, ", "))
	}
}
