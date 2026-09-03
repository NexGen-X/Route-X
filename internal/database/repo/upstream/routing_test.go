package upstream

import (
	"context"
	"crypto/rand"
	"encoding/hex"
	"errors"
	"fmt"
	"strings"
	"testing"
	"time"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgconn"

	"github.com/NexGen-X/Route-X/internal/database/repo"
)

// --- Perkakas test aturan routing ---------------------------------------------

// seedAPIKey menyisipkan API key milik owner dan mengembalikan pengenalnya.
//
// Disisipkan lewat SQL langsung, bukan lewat repo API key: yang dibutuhkan test ini hanya
// baris yang bisa ditunjuk match_api_key_id, dan meminjam repo paket lain akan mengikat
// test routing pada aturan pembuatan key yang tidak ada hubungannya dengan routing.
func seedAPIKey(ctx context.Context, t *testing.T, q repo.Querier, ownerID string) string {
	t.Helper()

	// key_hash dijaga constraint api_keys_hash_format sebagai 64 digit heksadesimal.
	buf := make([]byte, 32)
	if _, err := rand.Read(buf); err != nil {
		t.Fatalf("membuat hash key acak: %v", err)
	}

	var id string
	err := q.QueryRow(ctx, `
		insert into api_keys (name, key_hash, key_prefix, last4, owner_user_id)
		values ('key test', $1, 'sk_live_', 'ab12', $2)
		returning id::text`, hex.EncodeToString(buf), ownerID).Scan(&id)
	if err != nil {
		t.Fatalf("menyisipkan api key: %v", err)
	}
	return id
}

// seedRule membuat aturan routing dengan nama dan priority yang diminta.
func seedRule(ctx context.Context, t *testing.T, q repo.Querier, name string, priority int) *RoutingRule {
	t.Helper()
	rule, err := NewRoutingRepo(q).Create(ctx, CreateRoutingRuleParams{
		Name:     name,
		Priority: ptr(priority),
	})
	if err != nil {
		t.Fatalf("membuat aturan %s: %v", name, err)
	}
	return rule
}

// seedRuleProvider menyisipkan satu kandidat lewat SQL langsung, dengan position yang
// ditentukan pemanggil.
//
// Dipakai justru untuk hal yang tidak bisa diuji lewat SetProviders: SetProviders selalu
// menomori position mengikuti urutan slice, sehingga urutan penyisipan dan urutan position
// tidak pernah berbeda di sana. Di sini keduanya sengaja dibuat berbeda supaya terbukti
// pembacaan mengurut menurut position, bukan menurut urutan baris yang kebetulan tersimpan.
func seedRuleProvider(ctx context.Context, t *testing.T, q repo.Querier, ruleID, providerID string, position int) {
	t.Helper()
	_, err := q.Exec(ctx, `
		insert into routing_rule_providers (rule_id, provider_id, position)
		values ($1, $2, $3)`, ruleID, providerID, position)
	if err != nil {
		t.Fatalf("menyisipkan kandidat pada position %d: %v", position, err)
	}
}

// backdateRule memaksa created_at sebuah aturan.
//
// Urutan evaluasi ditengahi created_at, dan created_at bawaan berasal dari now() yang
// resolusinya jauh lebih halus daripada jarak antar pemanggilan Create di test. Tanpa
// memaksa nilainya, test urutan hanya akan menguji bahwa jam database berjalan maju —
// bukan bahwa penengahnya dipakai, apalagi bahwa dua aturan yang created_at-nya IDENTIK
// tetap punya urutan yang pasti.
func backdateRule(ctx context.Context, t *testing.T, q repo.Querier, ruleID string, at time.Time) {
	t.Helper()
	if _, err := q.Exec(ctx,
		`update routing_rules set created_at = $2 where id = $1`, ruleID, at); err != nil {
		t.Fatalf("memaksa created_at aturan: %v", err)
	}
}

// seedBulkRules mengisi routing_rules dengan banyak baris, sebagian kecil saja yang aktif,
// lalu memperbarui statistik perencana.
//
// Sebagian kecil aktif bukan penyederhanaan: indeks routing_rules_eval_idx adalah indeks
// partial "where enabled", dan justru tumpukan aturan lama yang dimatikan — bukan
// dihapus — yang membuatnya selektif. Pada tabel yang seluruhnya aktif, membaca semuanya
// lalu mengurutkan memang pilihan termurah, dan rencana query di sana tidak membuktikan
// apa pun tentang bentuk querynya.
func seedBulkRules(ctx context.Context, t *testing.T, q repo.Querier, total, active int) {
	t.Helper()
	if _, err := q.Exec(ctx, `
		insert into routing_rules (name, priority, enabled)
		select 'bulk-r-' || i, i % 1000, i <= $2
		from generate_series(1, $1) i`, total, active); err != nil {
		t.Fatalf("mengisi routing_rules massal: %v", err)
	}
	analyzeTables(ctx, t, q, "routing_rules")
}

// countingQuerier membungkus Querier dan menghitung pernyataan yang benar-benar dikirim ke
// database.
//
// Dipakai membuktikan ActiveRules tidak N+1 sebagai ANGKA. Membaca rencana query tidak bisa
// menjawab pertanyaan itu: rencana sebuah query tetap terlihat sehat meskipun querynya
// dijalankan sekali per aturan, dan justru pengulangan itulah yang mahal.
type countingQuerier struct {
	inner   repo.Querier
	queries int
}

func (c *countingQuerier) Exec(ctx context.Context, sql string, args ...any) (pgconn.CommandTag, error) {
	c.queries++
	return c.inner.Exec(ctx, sql, args...)
}

func (c *countingQuerier) Query(ctx context.Context, sql string, args ...any) (pgx.Rows, error) {
	c.queries++
	return c.inner.Query(ctx, sql, args...)
}

func (c *countingQuerier) QueryRow(ctx context.Context, sql string, args ...any) pgx.Row {
	c.queries++
	return c.inner.QueryRow(ctx, sql, args...)
}

// ruleNamesOf mengambil daftar nama aturan untuk pesan error yang bisa dibaca.
func ruleNamesOf(rules []*RoutingRule) []string {
	out := make([]string, len(rules))
	for i, rule := range rules {
		out[i] = rule.Name
	}
	return out
}

// positionsOf mengambil daftar position kandidat untuk pesan error yang bisa dibaca.
func positionsOf(rule *RoutingRule) []int {
	out := make([]int, len(rule.Providers))
	for i, c := range rule.Providers {
		out[i] = c.Position
	}
	return out
}

// Jalur bahagia CRUD aturan routing, termasuk nilai bawaan yang wajib sama dengan default
// kolom di migrasi 0008 dan penegakan constraint tabelnya.
func TestRoutingRuleCRUDIntegration(t *testing.T) {
	if testing.Short() {
		t.Skip("butuh PostgreSQL")
	}
	ctx, cancel := context.WithTimeout(context.Background(), 90*time.Second)
	defer cancel()

	pool := newTestPool(ctx, t)
	rules := NewRoutingRepo(pool)
	admin := seedUser(ctx, t, pool)
	model := seedModel(ctx, t, pool, "gpt-5", CapText, CapVision)
	apiKey := seedAPIKey(ctx, t, pool, admin)
	provider := seedProvider(ctx, t, pool, "openai")

	created, err := rules.Create(ctx, CreateRoutingRuleParams{Name: "aturan bawaan"})
	if err != nil {
		t.Fatalf("Create: %v", err)
	}

	switch {
	case !idOK(created.ID):
		t.Errorf("ID = %q, bukan UUID", created.ID)
	case created.Priority != DefaultPriority:
		t.Errorf("priority = %d, ingin %d", created.Priority, DefaultPriority)
	case created.Strategy != StrategyPriority:
		t.Errorf("strategy = %q, ingin %q", created.Strategy, StrategyPriority)
	case created.MaxAttempts != DefaultMaxAttempts || created.BackoffMS != DefaultBackoffMS:
		t.Errorf("retry = %d percobaan / %d ms, ingin %d / %d",
			created.MaxAttempts, created.BackoffMS, DefaultMaxAttempts, DefaultBackoffMS)
	case created.FailureThreshold != DefaultFailureThreshold ||
		created.OpenDurationMS != DefaultOpenDurationMS ||
		created.HalfOpenProbes != DefaultHalfOpenProbes:
		t.Errorf("circuit breaker = %d/%d/%d, ingin %d/%d/%d",
			created.FailureThreshold, created.OpenDurationMS, created.HalfOpenProbes,
			DefaultFailureThreshold, DefaultOpenDurationMS, DefaultHalfOpenProbes)
	case !created.Enabled:
		t.Error("aturan baru seharusnya aktif")
	case created.Description != nil || created.CreatedBy != nil:
		t.Errorf("description/created_by = %v/%v, ingin nil/nil", created.Description, created.CreatedBy)
	case created.MatchModelID != nil || created.MatchAPIKeyID != nil || len(created.MatchCapabilities) != 0:
		t.Error("aturan tanpa kondisi seharusnya tidak membatasi apa pun")
	case len(created.Providers) != 0:
		t.Errorf("kandidat = %d, ingin kosong: aturan baru berlaku atas semua provider", len(created.Providers))
	}

	full, err := rules.Create(ctx, CreateRoutingRuleParams{
		Name:              "khusus visi",
		Description:       ptr("permintaan bergambar dari satu key"),
		Priority:          ptr(10),
		MatchModelID:      &model.ID,
		MatchAPIKeyID:     &apiKey,
		MatchCapabilities: []string{CapVision, CapTools},
		Strategy:          StrategyWeighted,
		MaxAttempts:       ptr(5),
		BackoffMS:         ptr(0),
		FailureThreshold:  ptr(20),
		OpenDurationMS:    ptr(60000),
		HalfOpenProbes:    ptr(3),
		Enabled:           ptr(false),
		CreatedBy:         &admin,
	})
	if err != nil {
		t.Fatalf("Create lengkap: %v", err)
	}
	switch {
	case full.Description == nil || *full.Description != "permintaan bergambar dari satu key":
		t.Errorf("description = %v", full.Description)
	case full.MatchModelID == nil || *full.MatchModelID != model.ID:
		t.Errorf("match_model_id = %v, ingin %s", full.MatchModelID, model.ID)
	case full.MatchAPIKeyID == nil || *full.MatchAPIKeyID != apiKey:
		t.Errorf("match_api_key_id = %v, ingin %s", full.MatchAPIKeyID, apiKey)
	case strings.Join(full.MatchCapabilities, ",") != CapVision+","+CapTools:
		t.Errorf("match_capabilities = %v", full.MatchCapabilities)
	case full.Strategy != StrategyWeighted:
		t.Errorf("strategy = %q", full.Strategy)
	// backoff_ms nol adalah nilai yang sah — "tanpa jeda" — jadi nol tidak boleh diganti
	// nilai bawaan.
	case full.BackoffMS != 0:
		t.Errorf("backoff_ms = %d, ingin 0", full.BackoffMS)
	case full.Enabled:
		t.Error("aturan seharusnya dibuat dalam keadaan mati")
	case full.CreatedBy == nil || *full.CreatedBy != admin:
		t.Errorf("created_by = %v, ingin %s", full.CreatedBy, admin)
	}

	t.Run("get mengembalikan aturan yang sama, termasuk yang dimatikan", func(t *testing.T) {
		got, err := rules.Get(ctx, full.ID)
		if err != nil {
			t.Fatalf("Get: %v", err)
		}
		if got.ID != full.ID || got.Name != full.Name || got.Priority != full.Priority {
			t.Errorf("Get mengembalikan baris berbeda: %+v", got)
		}
		if len(got.Providers) != 0 {
			t.Errorf("kandidat = %d, ingin kosong", len(got.Providers))
		}
	})

	t.Run("deskripsi berisi spasi saja disimpan sebagai tidak ada", func(t *testing.T) {
		blank, err := rules.Create(ctx, CreateRoutingRuleParams{
			Name:        "aturan tanpa deskripsi",
			Description: ptr("   "),
		})
		if err != nil {
			t.Fatalf("Create: %v", err)
		}
		if blank.Description != nil {
			t.Errorf("description = %q, ingin nil: kolomnya bermakna tidak ada, bukan ada tapi kosong",
				*blank.Description)
		}
	})

	t.Run("nama ganda menjadi ErrConflict", func(t *testing.T) {
		_, err := rules.Create(ctx, CreateRoutingRuleParams{Name: created.Name})
		if !errors.Is(err, repo.ErrConflict) {
			t.Fatalf("Create nama ganda = %v, ingin ErrConflict", err)
		}
		if !strings.Contains(err.Error(), "routing_rules_name_key") {
			t.Errorf("pesan tidak menyebut constraint yang dilanggar: %v", err)
		}
	})

	t.Run("nilai yang dilarang constraint menjadi ErrConstraint", func(t *testing.T) {
		for _, tc := range []struct {
			name       string
			params     CreateRoutingRuleParams
			constraint string
		}{
			{"strategi tak dikenal", CreateRoutingRuleParams{
				Name: "strategi ngawur", Strategy: "acak",
			}, "routing_rules_strategy_valid"},
			{"kemampuan tak dikenal", CreateRoutingRuleParams{
				Name: "kemampuan ngawur", MatchCapabilities: []string{"telepati"},
			}, "routing_rules_capabilities_known"},
			{"percobaan di luar rentang", CreateRoutingRuleParams{
				Name: "sabar berlebihan", MaxAttempts: ptr(99),
			}, "routing_rules_attempts_range"},
			{"circuit terbuka terlalu singkat", CreateRoutingRuleParams{
				Name: "circuit kilat", OpenDurationMS: ptr(10),
			}, "routing_rules_open_duration"},
			{"nama kosong", CreateRoutingRuleParams{Name: "   "}, "routing_rules_name_not_empty"},
		} {
			t.Run(tc.name, func(t *testing.T) {
				_, err := rules.Create(ctx, tc.params)
				if !errors.Is(err, repo.ErrConstraint) {
					t.Fatalf("Create = %v, ingin ErrConstraint", err)
				}
				if !strings.Contains(err.Error(), tc.constraint) {
					t.Errorf("pesan tidak menyebut %s: %v", tc.constraint, err)
				}
			})
		}
	})

	t.Run("referensi ke baris yang tidak ada menjadi ErrInvalidReference", func(t *testing.T) {
		ghost, err := newID()
		if err != nil {
			t.Fatalf("newID: %v", err)
		}
		_, err = rules.Create(ctx, CreateRoutingRuleParams{
			Name: "aturan hantu", MatchModelID: &ghost,
		})
		if !errors.Is(err, repo.ErrInvalidReference) {
			t.Fatalf("Create dengan model hantu = %v, ingin ErrInvalidReference", err)
		}
		_, err = rules.Create(ctx, CreateRoutingRuleParams{
			Name: "aturan salah bentuk", CreatedBy: ptr("123"),
		})
		if !errors.Is(err, repo.ErrInvalidReference) {
			t.Errorf("Create dengan created_by salah bentuk = %v, ingin ErrInvalidReference", err)
		}
	})

	t.Run("update sebagian, kandidat tetap ikut terbaca", func(t *testing.T) {
		if err := SetRuleProviders(ctx, pool, full.ID, []string{provider.ID}, nil); err != nil {
			t.Fatalf("SetRuleProviders: %v", err)
		}

		updated, err := rules.Update(ctx, full.ID, UpdateRoutingRuleParams{
			Name:        ptr("khusus visi dan alat"),
			Priority:    ptr(5),
			Description: Clear[string](),
			MaxAttempts: ptr(4),
			Enabled:     ptr(true),
		})
		if err != nil {
			t.Fatalf("Update: %v", err)
		}
		switch {
		case updated.Name != "khusus visi dan alat":
			t.Errorf("name = %q", updated.Name)
		case updated.Priority != 5:
			t.Errorf("priority = %d, ingin 5", updated.Priority)
		case updated.Description != nil:
			t.Errorf("description = %v, ingin dikosongkan", updated.Description)
		case updated.MaxAttempts != 4:
			t.Errorf("max_attempts = %d, ingin 4", updated.MaxAttempts)
		case !updated.Enabled:
			t.Error("aturan seharusnya dinyalakan")
		case updated.Strategy != StrategyWeighted:
			t.Errorf("strategy = %q, seharusnya tidak ikut berubah", updated.Strategy)
		case updated.MatchModelID == nil || *updated.MatchModelID != model.ID:
			t.Errorf("match_model_id = %v, seharusnya tidak ikut berubah", updated.MatchModelID)
		case !updated.UpdatedAt.After(full.UpdatedAt):
			t.Error("updated_at tidak dimajukan trigger")
		// Update tidak menyentuh kandidat, tetapi kandidat yang ada wajib ikut terbaca:
		// daftar kosong pada hasil Update akan terbaca pemanggil sebagai "berlaku atas
		// semua provider", bukan sebagai "tidak diambil".
		case len(updated.Providers) != 1 || updated.Providers[0].ProviderID != provider.ID:
			t.Errorf("kandidat = %v, ingin satu kandidat %s", updated.Providers, provider.ID)
		}
	})

	t.Run("mengosongkan kondisi melebarkan aturan", func(t *testing.T) {
		wide, err := rules.Update(ctx, full.ID, UpdateRoutingRuleParams{
			MatchModelID:      Clear[string](),
			MatchAPIKeyID:     Clear[string](),
			MatchCapabilities: []string{},
		})
		if err != nil {
			t.Fatalf("Update: %v", err)
		}
		if wide.MatchModelID != nil || wide.MatchAPIKeyID != nil || len(wide.MatchCapabilities) != 0 {
			t.Errorf("kondisi masih membatasi: %v / %v / %v",
				wide.MatchModelID, wide.MatchAPIKeyID, wide.MatchCapabilities)
		}
	})

	t.Run("update tanpa perubahan ditolak", func(t *testing.T) {
		if _, err := rules.Update(ctx, full.ID, UpdateRoutingRuleParams{}); !errors.Is(err, ErrNoChanges) {
			t.Errorf("Update kosong = %v, ingin ErrNoChanges", err)
		}
	})

	t.Run("set enabled", func(t *testing.T) {
		off, err := rules.SetEnabled(ctx, full.ID, false)
		if err != nil {
			t.Fatalf("SetEnabled: %v", err)
		}
		if off.Enabled {
			t.Error("aturan masih aktif setelah dimatikan")
		}
	})

	t.Run("pengenal salah bentuk tidak menjadi kegagalan internal", func(t *testing.T) {
		if _, err := rules.Get(ctx, "bukan-uuid"); !errors.Is(err, repo.ErrNotFound) {
			t.Errorf("Get(bukan-uuid) = %v, ingin ErrNotFound", err)
		}
		if _, err := rules.Update(ctx, "bukan-uuid", UpdateRoutingRuleParams{Priority: ptr(1)}); !errors.Is(err, repo.ErrNotFound) {
			t.Errorf("Update(bukan-uuid) = %v, ingin ErrNotFound", err)
		}
	})

	t.Run("delete lalu tidak ditemukan", func(t *testing.T) {
		if err := rules.Delete(ctx, full.ID); err != nil {
			t.Fatalf("Delete: %v", err)
		}
		if err := rules.Delete(ctx, full.ID); !errors.Is(err, repo.ErrNotFound) {
			t.Errorf("Delete kedua = %v, ingin ErrNotFound", err)
		}
		if _, err := rules.Get(ctx, full.ID); !errors.Is(err, repo.ErrNotFound) {
			t.Errorf("Get setelah dihapus = %v, ingin ErrNotFound", err)
		}
		// Kandidatnya ikut terhapus lewat cascade, bukan tertinggal menunjuk aturan mati.
		var sisa int
		if err := pool.QueryRow(ctx,
			`select count(*) from routing_rule_providers where rule_id = $1`, full.ID).Scan(&sisa); err != nil {
			t.Fatalf("menghitung kandidat sisa: %v", err)
		}
		if sisa != 0 {
			t.Errorf("kandidat sisa = %d, ingin 0", sisa)
		}
	})
}

// Penggantian daftar kandidat: penomoran position, penggantian yang benar-benar mengganti,
// dan penjagaan yang tidak boleh bisa dilewati.
func TestRoutingRuleSetProvidersIntegration(t *testing.T) {
	if testing.Short() {
		t.Skip("butuh PostgreSQL")
	}
	ctx, cancel := context.WithTimeout(context.Background(), 90*time.Second)
	defer cancel()

	pool := newTestPool(ctx, t)
	rules := NewRoutingRepo(pool)

	satu := seedProvider(ctx, t, pool, "satu")
	dua := seedProvider(ctx, t, pool, "dua")
	tiga := seedProvider(ctx, t, pool, "tiga")
	rule := seedRule(ctx, t, pool, "aturan berkandidat", 10)

	t.Run("SetProviders di luar transaksi ditolak", func(t *testing.T) {
		err := rules.SetProviders(ctx, rule.ID, []string{satu.ID}, nil)
		if !errors.Is(err, ErrNeedsTx) {
			t.Errorf("SetProviders di luar transaksi = %v, ingin ErrNeedsTx", err)
		}
	})

	t.Run("position mengikuti urutan slice", func(t *testing.T) {
		err := SetRuleProviders(ctx, pool, rule.ID,
			[]string{satu.ID, dua.ID, tiga.ID}, map[string]int{dua.ID: 7})
		if err != nil {
			t.Fatalf("SetRuleProviders: %v", err)
		}

		got, err := rules.Get(ctx, rule.ID)
		if err != nil {
			t.Fatalf("Get: %v", err)
		}
		if len(got.Providers) != 3 {
			t.Fatalf("kandidat = %d, ingin 3", len(got.Providers))
		}
		for i, want := range []string{satu.ID, dua.ID, tiga.ID} {
			c := got.Providers[i]
			if c.ProviderID != want || c.Position != i+1 {
				t.Errorf("kandidat ke-%d = %s pada position %d, ingin %s pada position %d",
					i, c.ProviderID, c.Position, want, i+1)
			}
		}
		// Bobot hanya dipasang untuk provider yang disebut di map; sisanya NULL, yang berarti
		// memakai providers.weight.
		if got.Providers[1].Weight == nil || *got.Providers[1].Weight != 7 {
			t.Errorf("weight kandidat kedua = %v, ingin 7", got.Providers[1].Weight)
		}
		if got.Providers[0].Weight != nil || got.Providers[2].Weight != nil {
			t.Errorf("weight kandidat lain = %v / %v, ingin nil",
				got.Providers[0].Weight, got.Providers[2].Weight)
		}
	})

	t.Run("penggantian mengganti, bukan menambah", func(t *testing.T) {
		if err := SetRuleProviders(ctx, pool, rule.ID, []string{tiga.ID, satu.ID}, nil); err != nil {
			t.Fatalf("SetRuleProviders: %v", err)
		}

		got, err := rules.Get(ctx, rule.ID)
		if err != nil {
			t.Fatalf("Get: %v", err)
		}
		if len(got.Providers) != 2 {
			t.Fatalf("kandidat = %d, ingin 2 — daftar lama seharusnya hilang seluruhnya", len(got.Providers))
		}
		if got.Providers[0].ProviderID != tiga.ID || got.Providers[1].ProviderID != satu.ID {
			t.Errorf("urutan kandidat salah: %v", got.Providers)
		}
		// Bobot 7 milik daftar lama tidak boleh bertahan: yang diganti adalah seluruh
		// daftar, bukan hanya keanggotaannya.
		for i, c := range got.Providers {
			if c.Weight != nil {
				t.Errorf("weight kandidat ke-%d = %v, ingin nil", i, c.Weight)
			}
		}
		// Position dinomori ulang dari 1, bukan melanjutkan penomoran daftar lama.
		if got.Providers[0].Position != 1 || got.Providers[1].Position != 2 {
			t.Errorf("position = %v, ingin [1 2]", positionsOf(got))
		}
	})

	t.Run("daftar kosong mengembalikan aturan ke semua provider", func(t *testing.T) {
		if err := SetRuleProviders(ctx, pool, rule.ID, nil, nil); err != nil {
			t.Fatalf("SetRuleProviders kosong: %v", err)
		}
		got, err := rules.Get(ctx, rule.ID)
		if err != nil {
			t.Fatalf("Get: %v", err)
		}
		if len(got.Providers) != 0 {
			t.Errorf("kandidat = %d, ingin kosong", len(got.Providers))
		}
	})

	t.Run("aturan yang tidak ada tetap ErrNotFound meski daftarnya kosong", func(t *testing.T) {
		ghost, err := newID()
		if err != nil {
			t.Fatalf("newID: %v", err)
		}
		// Inilah yang tidak bisa disimpulkan dari jumlah baris: nol baris terhapus dan nol
		// baris tersisip adalah hasil yang sama dengan aturan sah yang memang tanpa kandidat.
		if err := SetRuleProviders(ctx, pool, ghost, nil, nil); !errors.Is(err, repo.ErrNotFound) {
			t.Errorf("SetProviders daftar kosong atas aturan hantu = %v, ingin ErrNotFound", err)
		}
		if err := SetRuleProviders(ctx, pool, ghost, []string{satu.ID}, nil); !errors.Is(err, repo.ErrNotFound) {
			t.Errorf("SetProviders atas aturan hantu = %v, ingin ErrNotFound", err)
		}
	})

	t.Run("provider ganda ditolak dengan menyebut providernya", func(t *testing.T) {
		err := SetRuleProviders(ctx, pool, rule.ID, []string{satu.ID, dua.ID, satu.ID}, nil)
		if !errors.Is(err, repo.ErrConflict) {
			t.Fatalf("SetProviders dengan provider ganda = %v, ingin ErrConflict", err)
		}
		if !strings.Contains(err.Error(), satu.ID) {
			t.Errorf("pesan tidak menyebut provider yang ganda: %v", err)
		}
		// Penolakan terjadi sebelum apa pun berubah, jadi daftar sebelumnya tetap utuh.
		got, err := rules.Get(ctx, rule.ID)
		if err != nil {
			t.Fatalf("Get: %v", err)
		}
		if len(got.Providers) != 0 {
			t.Errorf("kandidat = %d, ingin tetap kosong setelah penolakan", len(got.Providers))
		}
	})

	t.Run("provider yang tidak ada menjadi ErrInvalidReference", func(t *testing.T) {
		ghost, err := newID()
		if err != nil {
			t.Fatalf("newID: %v", err)
		}
		if err := SetRuleProviders(ctx, pool, rule.ID, []string{satu.ID, ghost}, nil); !errors.Is(err, repo.ErrInvalidReference) {
			t.Errorf("SetProviders dengan provider hantu = %v, ingin ErrInvalidReference", err)
		}
		// Transaksinya dibatalkan seluruhnya: provider pertama pun tidak tersisip.
		got, err := rules.Get(ctx, rule.ID)
		if err != nil {
			t.Fatalf("Get: %v", err)
		}
		if len(got.Providers) != 0 {
			t.Errorf("kandidat = %d, ingin kosong: penyisipan sebagian tidak boleh bertahan", len(got.Providers))
		}
	})

	t.Run("pengenal salah bentuk dibedakan artinya", func(t *testing.T) {
		err := repo.InTx(ctx, pool, func(q repo.Querier) error {
			return NewRoutingRepo(q).SetProviders(ctx, "bukan-uuid", nil, nil)
		})
		if !errors.Is(err, repo.ErrNotFound) {
			t.Errorf("rule_id salah bentuk = %v, ingin ErrNotFound", err)
		}
		err = repo.InTx(ctx, pool, func(q repo.Querier) error {
			return NewRoutingRepo(q).SetProviders(ctx, rule.ID, []string{"bukan-uuid"}, nil)
		})
		if !errors.Is(err, repo.ErrInvalidReference) {
			t.Errorf("provider_id salah bentuk = %v, ingin ErrInvalidReference", err)
		}
	})
}

// Urutan evaluasi adalah janji utama tabel ini: berurut priority naik, ditengahi created_at,
// dan pasti — termasuk untuk dua aturan yang created_at-nya identik.
func TestActiveRulesOrderIntegration(t *testing.T) {
	if testing.Short() {
		t.Skip("butuh PostgreSQL")
	}
	ctx, cancel := context.WithTimeout(context.Background(), 90*time.Second)
	defer cancel()

	pool := newTestPool(ctx, t)
	rules := NewRoutingRepo(pool)

	satu := seedProvider(ctx, t, pool, "satu")
	dua := seedProvider(ctx, t, pool, "dua")
	tiga := seedProvider(ctx, t, pool, "tiga")

	// created_at dipaksa supaya urutan yang diuji adalah urutan yang dijanjikan skema, bukan
	// urutan pemanggilan Create yang kebetulan sejalan dengannya.
	dasar := time.Date(2026, 9, 1, 8, 0, 0, 0, time.UTC)
	paling := seedRule(ctx, t, pool, "paling khusus", 10)
	backdateRule(ctx, t, pool, paling.ID, dasar.Add(time.Hour))
	kembarA := seedRule(ctx, t, pool, "sederajat a", 50)
	backdateRule(ctx, t, pool, kembarA.ID, dasar)
	kembarB := seedRule(ctx, t, pool, "sederajat b", 50)
	backdateRule(ctx, t, pool, kembarB.ID, dasar)
	belakangan := seedRule(ctx, t, pool, "sederajat tapi lebih baru", 50)
	backdateRule(ctx, t, pool, belakangan.ID, dasar.Add(2*time.Hour))
	cadangan := seedRule(ctx, t, pool, "cadangan", 100)
	backdateRule(ctx, t, pool, cadangan.ID, dasar)

	mati := seedRule(ctx, t, pool, "aturan mati", 1)
	if _, err := rules.SetEnabled(ctx, mati.ID, false); err != nil {
		t.Fatalf("mematikan aturan: %v", err)
	}

	// Kandidat disisipkan dengan urutan baris yang sengaja tidak sama dengan urutan position.
	seedRuleProvider(ctx, t, pool, paling.ID, tiga.ID, 3)
	seedRuleProvider(ctx, t, pool, paling.ID, satu.ID, 1)
	seedRuleProvider(ctx, t, pool, paling.ID, dua.ID, 2)

	// Dua aturan dengan (priority, created_at) yang sama ditengahi id, jadi urutan yang benar
	// dihitung di sini alih-alih diasumsikan sama dengan urutan pembuatan.
	pertama, kedua := kembarA, kembarB
	if kembarB.ID < kembarA.ID {
		pertama, kedua = kembarB, kembarA
	}
	want := []string{paling.Name, pertama.Name, kedua.Name, belakangan.Name, cadangan.Name}

	active, err := rules.ActiveRules(ctx)
	if err != nil {
		t.Fatalf("ActiveRules: %v", err)
	}
	if got := ruleNamesOf(active); strings.Join(got, "|") != strings.Join(want, "|") {
		t.Fatalf("urutan evaluasi = %v, ingin %v", got, want)
	}

	t.Run("aturan yang dimatikan tidak pernah ikut dievaluasi", func(t *testing.T) {
		for _, rule := range active {
			if rule.ID == mati.ID {
				t.Fatalf("aturan mati %q ikut dievaluasi meskipun priority-nya paling kecil", rule.Name)
			}
		}
	})

	t.Run("urutannya sama pada pemanggilan berulang", func(t *testing.T) {
		// Bukti bahwa urutannya tidak bergantung pada urutan baris yang kebetulan terbaca.
		// Tanpa penengah id, dua aturan dengan created_at identik boleh keluar dalam urutan
		// apa pun, dan "aturan pertama yang cocok menang" akan berarti aturan yang berbeda.
		for i := range 5 {
			ulang, err := rules.ActiveRules(ctx)
			if err != nil {
				t.Fatalf("ActiveRules ulangan %d: %v", i, err)
			}
			if got := ruleNamesOf(ulang); strings.Join(got, "|") != strings.Join(want, "|") {
				t.Fatalf("urutan ulangan %d = %v, ingin %v", i, got, want)
			}
		}
	})

	t.Run("kandidat terurut position, bukan urutan penyisipan", func(t *testing.T) {
		got := active[0]
		if got.ID != paling.ID {
			t.Fatalf("aturan pertama = %q, ingin %q", got.Name, paling.Name)
		}
		if len(got.Providers) != 3 {
			t.Fatalf("kandidat = %d, ingin 3", len(got.Providers))
		}
		for i, want := range []string{satu.ID, dua.ID, tiga.ID} {
			if got.Providers[i].ProviderID != want || got.Providers[i].Position != i+1 {
				t.Fatalf("kandidat = %v pada position %v, ingin urutan satu, dua, tiga",
					got.Providers, positionsOf(got))
			}
		}
	})

	t.Run("aturan tanpa kandidat terbaca dengan daftar kosong, bukan error", func(t *testing.T) {
		for _, rule := range active[1:] {
			if len(rule.Providers) != 0 {
				t.Errorf("aturan %q punya %d kandidat, padahal tidak pernah dipasangi",
					rule.Name, len(rule.Providers))
			}
		}
	})
}

// ActiveRules dipanggil di jalur request, jadi jumlah pernyataan yang dikirimnya tidak boleh
// tumbuh mengikuti jumlah aturan.
func TestActiveRulesNotNPlusOneIntegration(t *testing.T) {
	if testing.Short() {
		t.Skip("butuh PostgreSQL")
	}
	ctx, cancel := context.WithTimeout(context.Background(), 120*time.Second)
	defer cancel()

	pool := newTestPool(ctx, t)
	satu := seedProvider(ctx, t, pool, "satu")
	dua := seedProvider(ctx, t, pool, "dua")

	pasangKandidat := func(ruleID string) {
		t.Helper()
		if err := SetRuleProviders(ctx, pool, ruleID, []string{satu.ID, dua.ID}, nil); err != nil {
			t.Fatalf("SetRuleProviders: %v", err)
		}
	}

	penghitung := &countingQuerier{inner: pool}
	rules := NewRoutingRepo(penghitung)

	for i := range 3 {
		pasangKandidat(seedRule(ctx, t, pool, fmt.Sprintf("aturan-kecil-%d", i), 10+i).ID)
	}
	penghitung.queries = 0
	kecil, err := rules.ActiveRules(ctx)
	if err != nil {
		t.Fatalf("ActiveRules: %v", err)
	}
	queriesKecil := penghitung.queries

	for i := range 37 {
		pasangKandidat(seedRule(ctx, t, pool, fmt.Sprintf("aturan-besar-%d", i), 20+i).ID)
	}
	penghitung.queries = 0
	besar, err := rules.ActiveRules(ctx)
	if err != nil {
		t.Fatalf("ActiveRules: %v", err)
	}
	queriesBesar := penghitung.queries

	if len(kecil) != 3 || len(besar) != 40 {
		t.Fatalf("jumlah aturan = %d lalu %d, ingin 3 lalu 40", len(kecil), len(besar))
	}
	// Tanpa pemeriksaan ini, penghitung yang tidak pernah terpakai — misalnya karena repo
	// ternyata dibangun di atas pool langsung — akan membuat kedua angka nol dan seluruh
	// perbandingan di bawah lulus tanpa menguji apa pun.
	if queriesKecil < 1 {
		t.Fatalf("pernyataan tercatat = %d: ActiveRules tidak melewati penghitung", queriesKecil)
	}
	if queriesBesar != queriesKecil {
		t.Errorf("pernyataan = %d untuk 3 aturan tapi %d untuk 40 aturan: biayanya tumbuh mengikuti jumlah aturan",
			queriesKecil, queriesBesar)
	}
	// Batasnya sengaja longgar: satu atau dua pernyataan tetap tetap, sedangkan satu
	// pernyataan per aturan tidak. Yang dijaga adalah sifatnya, bukan angka pastinya.
	if queriesBesar > 2 {
		t.Errorf("pernyataan untuk 40 aturan = %d, ingin paling banyak 2", queriesBesar)
	}

	// Menghitung pernyataan saja bisa dipenuhi query yang salah, jadi isinya ikut diperiksa.
	for _, rule := range besar {
		if len(rule.Providers) != 2 {
			t.Fatalf("aturan %q punya %d kandidat, ingin 2", rule.Name, len(rule.Providers))
		}
		if rule.Providers[0].ProviderID != satu.ID || rule.Providers[1].ProviderID != dua.ID {
			t.Fatalf("kandidat aturan %q tercampur antar aturan: %v", rule.Name, rule.Providers)
		}
	}

	t.Run("penyaringan aturan aktif dilayani indeks partial", func(t *testing.T) {
		// Tabel yang sebagian besar barisnya aturan lama yang dimatikan: bentuk itulah yang
		// membuat routing_rules_eval_idx selektif, dan yang membuat rencana query di sini
		// membuktikan sesuatu tentang bentuk querynya.
		seedBulkRules(ctx, t, pool, 5000, 20)

		plan := explain(ctx, t, pool, activeRulesSQL)
		t.Logf("rencana aturan aktif:\n%s", plan)

		if !strings.Contains(plan, "routing_rules_eval_idx") {
			t.Errorf("rencana tidak memakai routing_rules_eval_idx:\n%s", plan)
		}
		if strings.Contains(plan, "Seq Scan on routing_rules") {
			t.Errorf("rencana membaca seluruh routing_rules:\n%s", plan)
		}
	})
}

// Cascade migrasi 0008 diuji sampai ke ARTINYA, bukan hanya sampai baris mana yang hilang:
// menghapus model atau API key membuang aturannya, sedangkan menghapus provider hanya
// membuang kandidatnya — dan aturan yang kehilangan kandidat terakhirnya berubah arti.
func TestRoutingRuleCascadeIntegration(t *testing.T) {
	if testing.Short() {
		t.Skip("butuh PostgreSQL")
	}
	ctx, cancel := context.WithTimeout(context.Background(), 90*time.Second)
	defer cancel()

	pool := newTestPool(ctx, t)
	rules := NewRoutingRepo(pool)

	pemilik := seedUser(ctx, t, pool)
	penyusun := seedUser(ctx, t, pool)
	model := seedModel(ctx, t, pool, "gpt-5", CapText)
	apiKey := seedAPIKey(ctx, t, pool, pemilik)
	satu := seedProvider(ctx, t, pool, "satu")
	dua := seedProvider(ctx, t, pool, "dua")
	tiga := seedProvider(ctx, t, pool, "tiga")

	buat := func(name string, p CreateRoutingRuleParams) *RoutingRule {
		t.Helper()
		p.Name = name
		rule, err := rules.Create(ctx, p)
		if err != nil {
			t.Fatalf("Create %s: %v", name, err)
		}
		return rule
	}
	perModel := buat("mensyaratkan model", CreateRoutingRuleParams{MatchModelID: &model.ID})
	perKey := buat("mensyaratkan key", CreateRoutingRuleParams{MatchAPIKeyID: &apiKey})
	berjejak := buat("punya penyusun", CreateRoutingRuleParams{CreatedBy: &penyusun})
	berkandidat := buat("dua kandidat", CreateRoutingRuleParams{})
	sekandidat := buat("satu kandidat", CreateRoutingRuleParams{})

	if err := SetRuleProviders(ctx, pool, berkandidat.ID, []string{satu.ID, dua.ID}, nil); err != nil {
		t.Fatalf("SetRuleProviders: %v", err)
	}
	if err := SetRuleProviders(ctx, pool, sekandidat.ID, []string{tiga.ID}, nil); err != nil {
		t.Fatalf("SetRuleProviders: %v", err)
	}

	t.Run("model dihapus, aturan yang mensyaratkannya ikut hilang", func(t *testing.T) {
		// Janji migrasinya: aturan yang mensyaratkan model yang sudah tidak ada tidak akan
		// pernah cocok lagi, jadi menyimpannya hanya menyisakan konfigurasi mati di dashboard.
		if err := NewModelRepo(pool).Delete(ctx, model.ID); err != nil {
			t.Fatalf("menghapus model: %v", err)
		}
		if _, err := rules.Get(ctx, perModel.ID); !errors.Is(err, repo.ErrNotFound) {
			t.Errorf("Get setelah modelnya dihapus = %v, ingin ErrNotFound", err)
		}
	})

	t.Run("api key dihapus, aturan yang mensyaratkannya ikut hilang", func(t *testing.T) {
		if _, err := pool.Exec(ctx, `delete from api_keys where id = $1`, apiKey); err != nil {
			t.Fatalf("menghapus api key: %v", err)
		}
		if _, err := rules.Get(ctx, perKey.ID); !errors.Is(err, repo.ErrNotFound) {
			t.Errorf("Get setelah keynya dihapus = %v, ingin ErrNotFound", err)
		}
	})

	t.Run("penyusun dihapus, aturannya tetap ada tanpa jejak penyusun", func(t *testing.T) {
		// created_by memakai on delete set null, bukan cascade: aturan yang masih dipakai
		// tidak boleh hilang hanya karena orang yang memasangnya keluar dari organisasi.
		if _, err := pool.Exec(ctx, `delete from users where id = $1`, penyusun); err != nil {
			t.Fatalf("menghapus pengguna: %v", err)
		}
		got, err := rules.Get(ctx, berjejak.ID)
		if err != nil {
			t.Fatalf("Get: %v", err)
		}
		if got.CreatedBy != nil {
			t.Errorf("created_by = %v, ingin nil", got.CreatedBy)
		}
	})

	t.Run("provider dihapus, hanya kandidatnya yang hilang", func(t *testing.T) {
		if err := NewProviderRepo(pool).Delete(ctx, satu.ID); err != nil {
			t.Fatalf("menghapus provider: %v", err)
		}
		got, err := rules.Get(ctx, berkandidat.ID)
		if err != nil {
			t.Fatalf("Get: %v", err)
		}
		if len(got.Providers) != 1 || got.Providers[0].ProviderID != dua.ID {
			t.Fatalf("kandidat = %v, ingin hanya %s", got.Providers, dua.ID)
		}
		// position tidak dinomori ulang, jadi daftarnya kini berlubang: mulai dari 2. Itu
		// tidak apa-apa karena position hanya dibaca sebagai URUTAN, bukan sebagai indeks —
		// tetapi urutannya harus tetap naik, dan itulah yang dijaga di sini.
		if got.Providers[0].Position != 2 {
			t.Errorf("position = %v, ingin [2]: penomoran tidak dirapatkan oleh cascade", positionsOf(got))
		}
	})

	t.Run("kandidat terakhir hilang berarti aturan berlaku atas semua provider", func(t *testing.T) {
		if err := NewProviderRepo(pool).Delete(ctx, tiga.ID); err != nil {
			t.Fatalf("menghapus provider: %v", err)
		}
		got, err := rules.Get(ctx, sekandidat.ID)
		if err != nil {
			t.Fatalf("Get: %v", err)
		}
		if len(got.Providers) != 0 {
			t.Fatalf("kandidat = %v, ingin kosong", got.Providers)
		}
		if !got.Enabled {
			t.Error("aturan seharusnya masih aktif: yang dihapus providernya, bukan aturannya")
		}

		// Ini pergeseran arti yang paling mudah terlewat: aturan yang tadinya membatasi diri
		// pada satu provider sekarang berlaku atas SEMUA provider, dan ia tetap ikut
		// dievaluasi seperti biasa. Tidak ada error, tidak ada tanda apa pun — jadi
		// dashboard-lah yang harus memberitahu operator, dan test ini yang menjaga bahwa
		// lapisan data memang melaporkan keadaan itu apa adanya.
		active, err := rules.ActiveRules(ctx)
		if err != nil {
			t.Fatalf("ActiveRules: %v", err)
		}
		ketemu := false
		for _, rule := range active {
			if rule.ID == sekandidat.ID {
				ketemu = true
				if len(rule.Providers) != 0 {
					t.Errorf("kandidat = %v, ingin kosong", rule.Providers)
				}
			}
		}
		if !ketemu {
			t.Error("aturan tanpa kandidat tidak ikut dievaluasi, padahal daftar kosong berarti semua provider")
		}
	})
}

// Penyaringan dan paginasi keyset daftar aturan untuk dashboard.
func TestRoutingRuleListIntegration(t *testing.T) {
	if testing.Short() {
		t.Skip("butuh PostgreSQL")
	}
	ctx, cancel := context.WithTimeout(context.Background(), 90*time.Second)
	defer cancel()

	pool := newTestPool(ctx, t)
	rules := NewRoutingRepo(pool)
	model := seedModel(ctx, t, pool, "gpt-5", CapText)
	provider := seedProvider(ctx, t, pool, "openai")

	var pertama string
	for i := range 7 {
		p := CreateRoutingRuleParams{
			Name:     fmt.Sprintf("daftar-%d", i),
			Priority: ptr(10 + i),
			Strategy: StrategyPriority,
		}
		if i%2 == 0 {
			p.Strategy = StrategyWeighted
		}
		if i >= 5 {
			p.MatchModelID = &model.ID
			p.Description = ptr("khusus katalog gambar")
		}
		if i == 6 {
			p.Enabled = ptr(false)
		}
		rule, err := rules.Create(ctx, p)
		if err != nil {
			t.Fatalf("Create %d: %v", i, err)
		}
		if i == 0 {
			pertama = rule.ID
		}
	}
	if err := SetRuleProviders(ctx, pool, pertama, []string{provider.ID}, map[string]int{provider.ID: 3}); err != nil {
		t.Fatalf("SetRuleProviders: %v", err)
	}

	t.Run("filter status, strategi, model, dan pencarian", func(t *testing.T) {
		for _, tc := range []struct {
			name   string
			filter RoutingRuleFilter
			want   int
		}{
			{"semua", RoutingRuleFilter{}, 7},
			{"aktif", RoutingRuleFilter{Enabled: ptr(true)}, 6},
			{"tidak aktif", RoutingRuleFilter{Enabled: ptr(false)}, 1},
			{"strategi weighted", RoutingRuleFilter{Strategy: StrategyWeighted}, 4},
			{"menyebut model", RoutingRuleFilter{ModelID: model.ID}, 2},
			{"pencarian nama", RoutingRuleFilter{Search: "daftar-3"}, 1},
			{"pencarian deskripsi", RoutingRuleFilter{Search: "katalog"}, 2},
			{"pencarian tanpa hasil", RoutingRuleFilter{Search: "tidak-ada"}, 0},
			{"gabungan", RoutingRuleFilter{Enabled: ptr(true), ModelID: model.ID}, 1},
		} {
			t.Run(tc.name, func(t *testing.T) {
				got, _, err := rules.List(ctx, tc.filter, repo.Page{})
				if err != nil {
					t.Fatalf("List: %v", err)
				}
				if len(got) != tc.want {
					t.Errorf("jumlah = %d (%v), ingin %d", len(got), ruleNamesOf(got), tc.want)
				}
			})
		}
	})

	t.Run("kandidat ikut terbaca di daftar", func(t *testing.T) {
		got, _, err := rules.List(ctx, RoutingRuleFilter{Search: "daftar-0"}, repo.Page{})
		if err != nil {
			t.Fatalf("List: %v", err)
		}
		if len(got) != 1 {
			t.Fatalf("jumlah = %d, ingin 1", len(got))
		}
		if len(got[0].Providers) != 1 || got[0].Providers[0].Weight == nil || *got[0].Providers[0].Weight != 3 {
			t.Errorf("kandidat = %v, ingin satu kandidat berbobot 3", got[0].Providers)
		}
	})

	t.Run("paginasi keyset membaca semua aturan tepat sekali", func(t *testing.T) {
		// Batas halaman dikenakan pada ATURAN, bukan pada baris hasil join: aturan berkandidat
		// tidak boleh menghabiskan kuota halaman, dan kandidatnya tidak boleh terpotong.
		seen := map[string]bool{}
		cursor := ""
		for range 10 {
			page, next, err := rules.List(ctx, RoutingRuleFilter{}, repo.Page{Limit: 2, Cursor: cursor})
			if err != nil {
				t.Fatalf("List: %v", err)
			}
			if len(page) > 2 {
				t.Fatalf("halaman berisi %d aturan, melebihi limit", len(page))
			}
			for _, rule := range page {
				if seen[rule.ID] {
					t.Fatalf("aturan %s muncul dua kali", rule.Name)
				}
				seen[rule.ID] = true
			}
			if next == "" {
				break
			}
			cursor = next
		}
		if len(seen) != 7 {
			t.Errorf("aturan terbaca = %d, ingin 7", len(seen))
		}
	})

	t.Run("kursor rusak ditolak sebagai pelanggaran aturan", func(t *testing.T) {
		if _, _, err := rules.List(ctx, RoutingRuleFilter{}, repo.Page{Cursor: "bukan-kursor"}); !errors.Is(err, repo.ErrConstraint) {
			t.Errorf("List dengan kursor rusak = %v, ingin ErrConstraint", err)
		}
	})

	t.Run("model_id salah bentuk menjadi ErrInvalidReference", func(t *testing.T) {
		if _, _, err := rules.List(ctx, RoutingRuleFilter{ModelID: "bukan-uuid"}, repo.Page{}); !errors.Is(err, repo.ErrInvalidReference) {
			t.Errorf("List dengan model_id salah bentuk = %v, ingin ErrInvalidReference", err)
		}
	})
}

// Pengenal salah bentuk harus ditolak sebelum menyentuh database, dengan arti yang benar:
// "tidak ada" untuk aturan yang dicari, "referensi tidak sah" untuk baris yang ditunjuk.
//
// Querier-nya nil, jadi satu jalur yang lolos ke database akan panik alih-alih diam-diam
// mengubah error menjadi kegagalan internal.
func TestRoutingMalformedIDRejectedWithoutDatabase(t *testing.T) {
	const malformed = "bukan-uuid"
	ctx := context.Background()
	rules := NewRoutingRepo(nil)

	notFound := map[string]func() error{
		"Get":    func() error { _, err := rules.Get(ctx, malformed); return err },
		"Update": func() error { _, err := rules.Update(ctx, malformed, UpdateRoutingRuleParams{}); return err },
		"Delete": func() error { return rules.Delete(ctx, malformed) },
	}
	for name, call := range notFound {
		t.Run(name, func(t *testing.T) {
			if err := call(); !errors.Is(err, repo.ErrNotFound) {
				t.Errorf("%s = %v, ingin ErrNotFound", name, err)
			}
		})
	}

	t.Run("List", func(t *testing.T) {
		_, _, err := rules.List(ctx, RoutingRuleFilter{ModelID: malformed}, repo.Page{})
		if !errors.Is(err, repo.ErrInvalidReference) {
			t.Errorf("List = %v, ingin ErrInvalidReference", err)
		}
	})

	t.Run("SetProviders", func(t *testing.T) {
		// Pemeriksaan transaksi mendahului pemeriksaan pengenal, seperti pada PricingRepo.Set:
		// tanpa transaksi, penggantian daftar tidak boleh dimulai sama sekali.
		if err := rules.SetProviders(ctx, malformed, nil, nil); !errors.Is(err, ErrNeedsTx) {
			t.Errorf("SetProviders = %v, ingin ErrNeedsTx", err)
		}
	})
}
