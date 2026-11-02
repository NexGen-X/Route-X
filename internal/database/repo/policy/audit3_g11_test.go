package policy

import (
	"context"
	"errors"
	"testing"

	"github.com/NexGen-X/Route-X/internal/database/repo"
)

// TestUpdateFilterMengosongkanCakupan mereproduksi audit3 V-DT-25:
// mengosongkan cakupan model/provider sebuah filter tidak mungkin dilakukan
// — string kosong ditulis mentah ke kolom uuid dan meledak sebagai 22P02,
// sementara handler menelannya menjadi "tidak berubah". Kini "" berarti
// kosongkan (NULL), dan UUID rusak ditolak bersih sebagai referensi tidak sah.
func TestUpdateFilterMengosongkanCakupan(t *testing.T) {
	ctx := context.Background()
	pool := newTestPool(ctx, t)
	r := New(pool)

	var modelID, providerID string
	if err := pool.QueryRow(ctx, `
		insert into models (model_id, display_name) values ('gpt-uji', 'GPT Uji') returning id::text`).
		Scan(&modelID); err != nil {
		t.Fatalf("menyisipkan model: %v", err)
	}
	if err := pool.QueryRow(ctx, `
		insert into providers (name, display_name, kind, base_url)
		values ('penyedia-uji', 'Penyedia Uji', 'openai_compatible', 'http://127.0.0.1:9') returning id::text`).
		Scan(&providerID); err != nil {
		t.Fatalf("menyisipkan provider: %v", err)
	}

	f, err := r.CreateFilter(ctx, CreateFilterParams{
		Name: "uji cakupan", Kind: FilterBlockedPattern,
		Pattern: "terlarang", PatternType: PatternSubstring,
		ModelID: modelID, ProviderID: providerID,
	})
	if err != nil {
		t.Fatalf("CreateFilter: %v", err)
	}
	if f.ModelID != modelID || f.ProviderID != providerID {
		t.Fatalf("cakupan awal = (%q, %q), mau (%q, %q)", f.ModelID, f.ProviderID, modelID, providerID)
	}

	kosong := ""
	upd, err := r.UpdateFilter(ctx, f.ID, UpdateFilterParams{ModelID: &kosong, ProviderID: &kosong})
	if err != nil {
		t.Fatalf("UpdateFilter mengosongkan: %v", err)
	}
	if upd.ModelID != "" || upd.ProviderID != "" {
		t.Errorf("cakupan sesudah dikosongkan = (%q, %q), mau keduanya kosong", upd.ModelID, upd.ProviderID)
	}

	// Nilai non-kosong yang bukan UUID ditolak sebagai referensi tidak sah,
	// bukan dibiarkan meledak di database sebagai 22P02/500.
	buruk := "bukan-uuid"
	if _, err := r.UpdateFilter(ctx, f.ID, UpdateFilterParams{ModelID: &buruk}); !errors.Is(err, repo.ErrInvalidReference) {
		t.Errorf("UpdateFilter model_id rusak: error = %v, mau ErrInvalidReference", err)
	}
	if _, err := r.UpdateFilter(ctx, f.ID, UpdateFilterParams{ProviderID: &buruk}); !errors.Is(err, repo.ErrInvalidReference) {
		t.Errorf("UpdateFilter provider_id rusak: error = %v, mau ErrInvalidReference", err)
	}

	// Dan cakupan masih bisa DIPASANG kembali sesudah dikosongkan.
	upd, err = r.UpdateFilter(ctx, f.ID, UpdateFilterParams{ModelID: &modelID})
	if err != nil {
		t.Fatalf("UpdateFilter memasang kembali: %v", err)
	}
	if upd.ModelID != modelID {
		t.Errorf("model_id sesudah dipasang kembali = %q, mau %q", upd.ModelID, modelID)
	}
}
