package keys

import (
	"errors"
	"strings"
	"testing"

	"github.com/NexGen-X/Route-X/internal/database/repo"
)

// Test B-201 (lapisan repo): slug model / nama provider yang tidak
// terpecahkan pada replaceAllowed ditolak sebagai ErrInvalidReference
// yang menyebut nilainya — bukan diteruskan mentah ke unnest(uuid[])
// hingga meledak 22P02 tanpa sentinel (terbaca 500 di lapisan atas).
// Daftar putih lama juga tidak boleh rusak oleh penolakan ini.
func TestFase7B201SetAllowedSlugYatimDitolak(t *testing.T) {
	ctx, pool, r := testEnv(t)
	owner := newUser(t, ctx, pool)
	modelID := newModel(t, ctx, pool, "model-nyata-b201")
	providerID := newProvider(t, ctx, pool, "prov-nyata-b201")

	created := mustCreate(t, ctx, r, CreateParams{
		Name: "kunci-b201", OwnerUserID: owner, Live: true,
		AllowedModelIDs:    []string{"model-nyata-b201"},
		AllowedProviderIDs: []string{"prov-nyata-b201"},
	})

	err := r.SetAllowed(ctx, created.Key.ID, []string{"model-yatim-b201"}, nil)
	if !errors.Is(err, repo.ErrInvalidReference) {
		t.Fatalf("SetAllowed slug yatim: err = %v, mau ErrInvalidReference", err)
	}
	if !strings.Contains(err.Error(), "model-yatim-b201") {
		t.Errorf("error tidak menyebut slug yang ditolak: %v", err)
	}
	// Daftar lama utuh sesudah penolakan (transaksi membatalkan delete).
	if ok, _ := r.AllowsModel(ctx, created.Key.ID, modelID); !ok {
		t.Error("daftar putih model lama rusak sesudah SetAllowed ditolak")
	}
	if ok, _ := r.AllowsProvider(ctx, created.Key.ID, providerID); !ok {
		t.Error("daftar putih provider lama rusak sesudah SetAllowed ditolak")
	}

	err = r.SetAllowed(ctx, created.Key.ID, nil, []string{"prov-yatim-b201"})
	if !errors.Is(err, repo.ErrInvalidReference) {
		t.Fatalf("SetAllowed provider yatim: err = %v, mau ErrInvalidReference", err)
	}
	if !strings.Contains(err.Error(), "prov-yatim-b201") {
		t.Errorf("error tidak menyebut provider yang ditolak: %v", err)
	}

	// Kontrol positif: slug nyata tetap bisa dipasang.
	if err := r.SetAllowed(ctx, created.Key.ID, []string{"model-nyata-b201"}, []string{"prov-nyata-b201"}); err != nil {
		t.Fatalf("SetAllowed slug nyata: %v", err)
	}
}
