package policy

import (
	"context"
	"errors"
	"testing"

	"github.com/NexGen-X/Route-X/internal/database/repo"
)

// TestB366UpdateFilterParamsKosongDitolak adalah pengunci B-366:
// UpdateFilter adalah satu-satunya Update* ber-Params di paket
// policy yang menjawab params kosong dengan baris lama dan
// err=nil (200 idempoten), sementara UpdateBudget/UpdateRateLimit
// di paket yang sama menolak ErrNoChanges (B-354) dan handler
// memetakannya ke 400 no_changes. Kini UpdateFilter seragam.
func TestB366UpdateFilterParamsKosongDitolak(t *testing.T) {
	ctx, r := newRepo(t)

	f, err := r.CreateFilter(ctx, CreateFilterParams{
		Name: "filter-b366", Kind: FilterBlockedPattern, Pattern: "pola", PatternType: PatternSubstring,
	})
	if err != nil {
		t.Fatalf("CreateFilter: %v", err)
	}
	if _, err := r.UpdateFilter(ctx, f.ID, UpdateFilterParams{}); !errors.Is(err, ErrNoChanges) {
		t.Errorf("UpdateFilter(params kosong) = %v, mau ErrNoChanges", err)
	}
	baru := "filter-b366-baru"
	if _, err := r.UpdateFilter(ctx, f.ID, UpdateFilterParams{Name: &baru}); err != nil {
		t.Errorf("UpdateFilter(dengan nama) = %v, mau sukses (kontrol)", err)
	}
}

// TestB367UpdateFilterEnabledMemvalidasiPola adalah pengunci
// B-367: validasi pola B-308 di UpdateFilter dulu hanya berjalan
// bila kind/pola/tipe/kepekaan berubah — params {Enabled: true}
// saja melewatkannya, jadi baris warisan berpola regex rusak bisa
// dinyalakan lewat repo (filter aktif-inert kelas B-326).
// Permukaan HTTP hanya tertutup insidental oleh akar B-370.
// Kini menyalakan lewat UpdateFilter memvalidasi pola efektif
// tersimpan, sejajar SetFilterEnabled (B-326); mematikan tetap
// tidak pernah divalidasi.
func TestB367UpdateFilterEnabledMemvalidasiPola(t *testing.T) {
	ctx := context.Background()
	pool := newTestPool(ctx, t)
	r := New(pool)

	f, err := r.CreateFilter(ctx, CreateFilterParams{
		Name: "warisan-b367", Kind: FilterBlockedPattern, Pattern: "aman", PatternType: PatternSubstring,
	})
	if err != nil {
		t.Fatalf("CreateFilter: %v", err)
	}
	if _, err := pool.Exec(ctx,
		`update content_filters set pattern = '[a-', pattern_type = 'regex', enabled = false where id = $1`,
		f.ID); err != nil {
		t.Fatalf("menanam pola warisan: %v", err)
	}

	nyala := true
	mati := false
	if _, err := r.UpdateFilter(ctx, f.ID, UpdateFilterParams{Enabled: &nyala}); !errors.Is(err, repo.ErrInvalidParameter) {
		t.Errorf("UpdateFilter{Enabled:true} pada pola rusak = %v, mau ErrInvalidParameter", err)
	}
	kini, err := r.GetFilter(ctx, f.ID)
	if err != nil {
		t.Fatalf("GetFilter: %v", err)
	}
	if kini.Enabled {
		t.Error("filter berregex rusak ikut menyala lewat UpdateFilter walau ditolak")
	}
	if _, err := r.UpdateFilter(ctx, f.ID, UpdateFilterParams{Enabled: &mati}); err != nil {
		t.Errorf("UpdateFilter{Enabled:false} pada pola rusak = %v, mau tetap bisa", err)
	}

	// Kontrol: pola sah bisa dinyalakan lewat UpdateFilter.
	g, err := r.CreateFilter(ctx, CreateFilterParams{
		Name: "sehat-b367", Kind: FilterBlockedPattern, Pattern: "kartu[0-9]+", PatternType: PatternRegex,
	})
	if err != nil {
		t.Fatalf("CreateFilter sehat: %v", err)
	}
	if _, err := r.UpdateFilter(ctx, g.ID, UpdateFilterParams{Enabled: &mati}); err != nil {
		t.Fatalf("matikan filter sehat: %v", err)
	}
	hasil, err := r.UpdateFilter(ctx, g.ID, UpdateFilterParams{Enabled: &nyala})
	if err != nil {
		t.Fatalf("UpdateFilter{Enabled:true} pada pola sah = %v, mau sukses", err)
	}
	if !hasil.Enabled {
		t.Error("filter sehat tidak menyala sesudah UpdateFilter{Enabled:true}")
	}
}
