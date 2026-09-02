package upstream

import (
	"context"
	"errors"
	"strings"
	"testing"
	"time"

	"github.com/NexGen-X/Route-X/internal/database/repo"
)

// Jalur bahagia CRUD model beserta penegakan daftar kemampuan.
func TestModelCRUDIntegration(t *testing.T) {
	if testing.Short() {
		t.Skip("butuh PostgreSQL")
	}
	ctx, cancel := context.WithTimeout(context.Background(), 90*time.Second)
	defer cancel()

	pool := newTestPool(ctx, t)
	models := NewModelRepo(pool)

	created, err := models.Create(ctx, CreateModelParams{
		ModelID:         "gpt-5",
		DisplayName:     "GPT-5",
		Family:          ptr("gpt"),
		ContextWindow:   ptr(400000),
		MaxOutputTokens: ptr(128000),
		Capabilities:    []string{CapText, CapVision, CapTools},
		RoutingStrategy: ptr(StrategyLowestLatency),
		Metadata:        []byte(`{"vendor":"openai"}`),
	})
	if err != nil {
		t.Fatalf("Create: %v", err)
	}

	switch {
	case !idOK(created.ID):
		t.Errorf("ID = %q, bukan UUID", created.ID)
	case len(created.Capabilities) != 3 || !created.HasCapability(CapVision):
		t.Errorf("capabilities = %v", created.Capabilities)
	case created.RoutingPriority != DefaultPriority:
		t.Errorf("routing_priority = %d, ingin %d", created.RoutingPriority, DefaultPriority)
	case !created.Enabled || created.IsDeprecated():
		t.Errorf("model baru = %+v, ingin aktif dan belum usang", created)
	case created.ContextWindow == nil || *created.ContextWindow != 400000:
		t.Errorf("context_window = %v", created.ContextWindow)
	}

	t.Run("kemampuan di luar daftar ditolak", func(t *testing.T) {
		_, err := models.Create(ctx, CreateModelParams{
			ModelID: "model-salah-kemampuan", DisplayName: "Salah",
			Capabilities: []string{CapText, "telepati"},
		})
		if !errors.Is(err, repo.ErrConstraint) {
			t.Fatalf("Create = %v, ingin ErrConstraint", err)
		}
		if !strings.Contains(err.Error(), "models_capabilities_known") {
			t.Errorf("pesan tidak menyebut constraint: %v", err)
		}
	})

	t.Run("model_id ganda menjadi ErrConflict", func(t *testing.T) {
		_, err := models.Create(ctx, CreateModelParams{ModelID: "gpt-5", DisplayName: "Kembar"})
		if !errors.Is(err, repo.ErrConflict) {
			t.Errorf("Create = %v, ingin ErrConflict", err)
		}
	})

	t.Run("get menurut pengenal dan menurut model_id", func(t *testing.T) {
		byID, err := models.Get(ctx, created.ID)
		if err != nil {
			t.Fatalf("Get: %v", err)
		}
		byModelID, err := models.GetByModelID(ctx, "gpt-5")
		if err != nil {
			t.Fatalf("GetByModelID: %v", err)
		}
		if byID.ID != byModelID.ID {
			t.Error("Get dan GetByModelID mengembalikan baris berbeda")
		}
		if _, err := models.GetByModelID(ctx, "tidak-ada"); !errors.Is(err, repo.ErrNotFound) {
			t.Errorf("GetByModelID tidak dikenal = %v, ingin ErrNotFound", err)
		}
	})

	t.Run("update sebagian termasuk mengosongkan dan menandai usang", func(t *testing.T) {
		deprecated := time.Now().Truncate(time.Millisecond)
		updated, err := models.Update(ctx, created.ID, UpdateModelParams{
			DisplayName:  ptr("GPT-5 (usang)"),
			Family:       Clear[string](),
			Capabilities: []string{CapText},
			DeprecatedAt: Set(deprecated),
			Enabled:      ptr(false),
		})
		if err != nil {
			t.Fatalf("Update: %v", err)
		}
		switch {
		case updated.Family != nil:
			t.Errorf("family = %v, ingin dikosongkan", updated.Family)
		case len(updated.Capabilities) != 1 || updated.Capabilities[0] != CapText:
			t.Errorf("capabilities = %v, ingin hanya text", updated.Capabilities)
		case !updated.IsDeprecated():
			t.Error("deprecated_at tidak terisi")
		case updated.Enabled:
			t.Error("model masih aktif")
		case updated.ContextWindow == nil || *updated.ContextWindow != 400000:
			t.Errorf("context_window = %v, seharusnya tidak ikut berubah", updated.ContextWindow)
		}
	})

	t.Run("update tanpa perubahan ditolak", func(t *testing.T) {
		if _, err := models.Update(ctx, created.ID, UpdateModelParams{}); !errors.Is(err, ErrNoChanges) {
			t.Errorf("Update kosong = %v, ingin ErrNoChanges", err)
		}
	})

	t.Run("hapus lalu tidak ditemukan", func(t *testing.T) {
		if err := models.Delete(ctx, created.ID); err != nil {
			t.Fatalf("Delete: %v", err)
		}
		if _, err := models.Get(ctx, created.ID); !errors.Is(err, repo.ErrNotFound) {
			t.Errorf("Get setelah dihapus = %v, ingin ErrNotFound", err)
		}
	})
}

// Alias dan resolusi nama, termasuk kedua arah larangan tabrakan dengan model_id.
func TestModelAliasIntegration(t *testing.T) {
	if testing.Short() {
		t.Skip("butuh PostgreSQL")
	}
	ctx, cancel := context.WithTimeout(context.Background(), 90*time.Second)
	defer cancel()

	pool := newTestPool(ctx, t)
	models := NewModelRepo(pool)
	admin := seedUser(ctx, t, pool)

	canonical := seedModel(ctx, t, pool, "claude-sonnet-4-6", CapText, CapTools)
	other := seedModel(ctx, t, pool, "gpt-5-mini", CapText)

	alias, err := models.AddAlias(ctx, canonical.ID, "sonnet-terbaru", &admin)
	if err != nil {
		t.Fatalf("AddAlias: %v", err)
	}
	if alias.ModelID != canonical.ID || alias.CreatedBy == nil || *alias.CreatedBy != admin {
		t.Errorf("alias = %+v", alias)
	}

	t.Run("resolusi mencoba model kanonik lalu alias", func(t *testing.T) {
		viaKanonik, err := models.Resolve(ctx, "claude-sonnet-4-6")
		if err != nil {
			t.Fatalf("Resolve kanonik: %v", err)
		}
		if viaKanonik.ID != canonical.ID {
			t.Errorf("resolusi kanonik = %s, ingin %s", viaKanonik.ModelID, canonical.ModelID)
		}

		viaAlias, err := models.Resolve(ctx, "sonnet-terbaru")
		if err != nil {
			t.Fatalf("Resolve alias: %v", err)
		}
		if viaAlias.ID != canonical.ID {
			t.Errorf("resolusi alias = %s, ingin %s", viaAlias.ModelID, canonical.ModelID)
		}
		if viaAlias.ModelID != "claude-sonnet-4-6" {
			t.Errorf("model_id hasil resolusi alias = %q, ingin nama kanonik", viaAlias.ModelID)
		}

		if _, err := models.Resolve(ctx, "model-yang-tidak-ada"); !errors.Is(err, repo.ErrNotFound) {
			t.Errorf("Resolve nama asing = %v, ingin ErrNotFound", err)
		}
	})

	t.Run("model yang dimatikan tetap bisa diselesaikan", func(t *testing.T) {
		// Pemanggil berhak membalas "model dimatikan", bukan "model tidak dikenal".
		if _, err := models.Update(ctx, other.ID, UpdateModelParams{Enabled: ptr(false)}); err != nil {
			t.Fatalf("Update: %v", err)
		}
		got, err := models.Resolve(ctx, "gpt-5-mini")
		if err != nil {
			t.Fatalf("Resolve model mati: %v", err)
		}
		if got.Enabled {
			t.Error("model seharusnya sudah mati")
		}
	})

	t.Run("alias yang bertabrakan dengan model_id ditolak", func(t *testing.T) {
		_, err := models.AddAlias(ctx, canonical.ID, "gpt-5-mini", nil)
		if !errors.Is(err, repo.ErrConflict) {
			t.Fatalf("AddAlias bertabrakan = %v, ingin ErrConflict", err)
		}
		// Pesannya harus menjelaskan sebabnya, bukan sekadar "data sudah ada".
		if !strings.Contains(err.Error(), "model_id kanonik") {
			t.Errorf("pesan tidak menjelaskan tabrakan dengan model kanonik: %v", err)
		}
	})

	t.Run("model_id yang bertabrakan dengan alias ditolak", func(t *testing.T) {
		_, err := models.Create(ctx, CreateModelParams{
			ModelID: "sonnet-terbaru", DisplayName: "Penabrak Alias",
		})
		if !errors.Is(err, repo.ErrConflict) {
			t.Fatalf("Create bertabrakan = %v, ingin ErrConflict", err)
		}
		if !strings.Contains(err.Error(), "alias") {
			t.Errorf("pesan tidak menjelaskan tabrakan dengan alias: %v", err)
		}

		_, err = models.Update(ctx, other.ID, UpdateModelParams{ModelID: ptr("sonnet-terbaru")})
		if !errors.Is(err, repo.ErrConflict) {
			t.Errorf("Update model_id ke nama alias = %v, ingin ErrConflict", err)
		}
	})

	t.Run("alias ganda menjadi ErrConflict", func(t *testing.T) {
		if _, err := models.AddAlias(ctx, other.ID, "sonnet-terbaru", nil); !errors.Is(err, repo.ErrConflict) {
			t.Errorf("AddAlias ganda = %v, ingin ErrConflict", err)
		}
	})

	t.Run("daftar dan hapus alias", func(t *testing.T) {
		if _, err := models.AddAlias(ctx, canonical.ID, "claude-terbaru", nil); err != nil {
			t.Fatalf("AddAlias kedua: %v", err)
		}
		list, err := models.ListAliases(ctx, canonical.ID)
		if err != nil {
			t.Fatalf("ListAliases: %v", err)
		}
		if len(list) != 2 || list[0].Alias != "claude-terbaru" {
			t.Errorf("daftar alias = %+v, ingin dua alias terurut abjad", list)
		}

		if err := models.RemoveAlias(ctx, "claude-terbaru"); err != nil {
			t.Fatalf("RemoveAlias: %v", err)
		}
		if err := models.RemoveAlias(ctx, "claude-terbaru"); !errors.Is(err, repo.ErrNotFound) {
			t.Errorf("RemoveAlias kedua = %v, ingin ErrNotFound", err)
		}
	})

	t.Run("alias ikut terhapus bersama modelnya", func(t *testing.T) {
		if err := models.Delete(ctx, canonical.ID); err != nil {
			t.Fatalf("Delete model: %v", err)
		}
		if _, err := models.Resolve(ctx, "sonnet-terbaru"); !errors.Is(err, repo.ErrNotFound) {
			t.Errorf("Resolve alias setelah model dihapus = %v, ingin ErrNotFound", err)
		}
	})
}

// Penyaringan daftar model, termasuk penyaringan kemampuan yang wajib memakai indeks GIN.
func TestModelListIntegration(t *testing.T) {
	if testing.Short() {
		t.Skip("butuh PostgreSQL")
	}
	ctx, cancel := context.WithTimeout(context.Background(), 90*time.Second)
	defer cancel()

	pool := newTestPool(ctx, t)
	models := NewModelRepo(pool)

	seedModel(ctx, t, pool, "model-teks", CapText)
	seedModel(ctx, t, pool, "model-visi", CapText, CapVision)
	seedModel(ctx, t, pool, "model-visi-alat", CapText, CapVision, CapTools)
	seedModel(ctx, t, pool, "model-embedding", CapEmbeddings)
	deprecated := seedModel(ctx, t, pool, "model-usang", CapText)
	if _, err := models.Update(ctx, deprecated.ID, UpdateModelParams{
		DeprecatedAt: Set(time.Now()), Enabled: ptr(false),
	}); err != nil {
		t.Fatalf("menandai usang: %v", err)
	}

	for _, tc := range []struct {
		name   string
		filter ModelFilter
		want   []string
	}{
		{"semua", ModelFilter{}, []string{"model-embedding", "model-teks", "model-usang", "model-visi", "model-visi-alat"}},
		{"berkemampuan visi", ModelFilter{Capabilities: []string{CapVision}}, []string{"model-visi", "model-visi-alat"}},
		{"visi dan alat sekaligus", ModelFilter{Capabilities: []string{CapVision, CapTools}}, []string{"model-visi-alat"}},
		{"aktif saja", ModelFilter{Enabled: ptr(true)}, []string{"model-embedding", "model-teks", "model-visi", "model-visi-alat"}},
		{"tanpa yang usang", ModelFilter{ExcludeDeprecated: true}, []string{"model-embedding", "model-teks", "model-visi", "model-visi-alat"}},
		{"pencarian", ModelFilter{Search: "visi"}, []string{"model-visi", "model-visi-alat"}},
	} {
		t.Run(tc.name, func(t *testing.T) {
			got, _, err := models.List(ctx, tc.filter, repo.Page{})
			if err != nil {
				t.Fatalf("List: %v", err)
			}
			if len(got) != len(tc.want) {
				t.Fatalf("jumlah = %d (%v), ingin %d (%v)", len(got), modelIDsOf(got), len(tc.want), tc.want)
			}
			for i := range got {
				if got[i].ModelID != tc.want[i] {
					t.Errorf("urutan = %v, ingin %v", modelIDsOf(got), tc.want)
					break
				}
			}
		})
	}

	t.Run("paginasi keyset membaca semua baris tepat sekali", func(t *testing.T) {
		seen := map[string]bool{}
		cursor := ""
		for range 10 {
			page, next, err := models.List(ctx, ModelFilter{}, repo.Page{Limit: 2, Cursor: cursor})
			if err != nil {
				t.Fatalf("List: %v", err)
			}
			for _, m := range page {
				if seen[m.ModelID] {
					t.Fatalf("model %s muncul dua kali", m.ModelID)
				}
				seen[m.ModelID] = true
			}
			if next == "" {
				break
			}
			cursor = next
		}
		if len(seen) != 5 {
			t.Errorf("model terbaca = %d, ingin 5", len(seen))
		}
	})
}

// namaModel mengambil daftar model_id untuk pesan error yang bisa dibaca.
func modelIDsOf(models []*Model) []string {
	out := make([]string, len(models))
	for i, m := range models {
		out[i] = m.ModelID
	}
	return out
}

// Penyaringan kemampuan wajib dilayani indeks GIN models_capabilities_idx: dengan unnest
// atau ANY, setiap baris harus dibongkar lebih dulu sehingga seluruh tabel dibaca.
func TestModelCapabilityFilterUsesGINIntegration(t *testing.T) {
	if testing.Short() {
		t.Skip("butuh PostgreSQL")
	}
	ctx, cancel := context.WithTimeout(context.Background(), 120*time.Second)
	defer cancel()

	pool := newTestPool(ctx, t)
	seedBulkModels(ctx, t, pool, 5000)

	sql, args := listModelsQuery(ModelFilter{Capabilities: []string{CapVision}}, "", repo.DefaultPageLimit)
	plan := explain(ctx, t, pool, sql, args...)
	t.Logf("rencana filter kemampuan:\n%s", plan)

	if !strings.Contains(plan, "models_capabilities_idx") {
		t.Errorf("rencana tidak memakai indeks GIN models_capabilities_idx:\n%s", plan)
	}
	if strings.Contains(plan, "Seq Scan on models") {
		t.Errorf("rencana membaca seluruh tabel models:\n%s", plan)
	}

	t.Run("hasilnya tetap benar", func(t *testing.T) {
		got, _, err := NewModelRepo(pool).List(ctx, ModelFilter{Capabilities: []string{CapVision}}, repo.Page{})
		if err != nil {
			t.Fatalf("List: %v", err)
		}
		if len(got) != 5 {
			t.Fatalf("jumlah model bervisi = %d, ingin 5", len(got))
		}
		for _, m := range got {
			if !m.HasCapability(CapVision) {
				t.Errorf("model %s ikut terpilih tanpa kemampuan visi", m.ModelID)
			}
		}
	})
}

// CRUD pemetaan model ke provider.
func TestProviderModelCRUDIntegration(t *testing.T) {
	if testing.Short() {
		t.Skip("butuh PostgreSQL")
	}
	ctx, cancel := context.WithTimeout(context.Background(), 90*time.Second)
	defer cancel()

	pool := newTestPool(ctx, t)
	models := NewModelRepo(pool)

	model := seedModel(ctx, t, pool, "gpt-4o", CapText, CapVision)
	primary := seedProvider(ctx, t, pool, "openai")
	fallback := seedProvider(ctx, t, pool, "azure-openai")

	t.Run("nama upstream kosong memakai nama kanonik", func(t *testing.T) {
		pm, err := models.AttachProvider(ctx, AttachProviderParams{
			ModelID: model.ID, ProviderID: primary.ID,
		})
		if err != nil {
			t.Fatalf("AttachProvider: %v", err)
		}
		if pm.UpstreamModelName != "gpt-4o" {
			t.Errorf("upstream_model_name = %q, ingin %q", pm.UpstreamModelName, "gpt-4o")
		}
		if !pm.Enabled || !pm.SupportsStreaming || !pm.SupportsTools {
			t.Errorf("nilai bawaan pemetaan tidak terpasang: %+v", pm)
		}
		if pm.Priority != nil || pm.Weight != nil {
			t.Errorf("priority/weight = %v/%v, ingin nil supaya mewarisi provider", pm.Priority, pm.Weight)
		}
	})

	pmFallback, err := models.AttachProvider(ctx, AttachProviderParams{
		ModelID:           model.ID,
		ProviderID:        fallback.ID,
		UpstreamModelName: "gpt-4o-2024-08-06",
		Priority:          ptr(5),
		Weight:            ptr(3),
		MaxContextWindow:  ptr(64000),
		SupportsTools:     ptr(false),
	})
	if err != nil {
		t.Fatalf("AttachProvider cadangan: %v", err)
	}

	t.Run("pasangan model-provider ganda menjadi ErrConflict", func(t *testing.T) {
		_, err := models.AttachProvider(ctx, AttachProviderParams{
			ModelID: model.ID, ProviderID: fallback.ID, UpstreamModelName: "apa-saja",
		})
		if !errors.Is(err, repo.ErrConflict) {
			t.Errorf("AttachProvider ganda = %v, ingin ErrConflict", err)
		}
	})

	t.Run("provider yang tidak ada menjadi ErrInvalidReference", func(t *testing.T) {
		ghostID, err := newID()
		if err != nil {
			t.Fatalf("newID: %v", err)
		}
		_, err = models.AttachProvider(ctx, AttachProviderParams{
			ModelID: model.ID, ProviderID: ghostID, UpstreamModelName: "gpt-4o",
		})
		if !errors.Is(err, repo.ErrInvalidReference) {
			t.Errorf("AttachProvider ke provider hantu = %v, ingin ErrInvalidReference", err)
		}
	})

	t.Run("get dan daftar", func(t *testing.T) {
		got, err := models.GetProviderModel(ctx, pmFallback.ID)
		if err != nil {
			t.Fatalf("GetProviderModel: %v", err)
		}
		if got.MaxContextWindow == nil || *got.MaxContextWindow != 64000 || got.SupportsTools {
			t.Errorf("pemetaan = %+v", got)
		}

		byModel, err := models.ListProviderModels(ctx, ProviderModelFilter{ModelID: model.ID})
		if err != nil {
			t.Fatalf("ListProviderModels per model: %v", err)
		}
		if len(byModel) != 2 || byModel[0].ID != pmFallback.ID {
			t.Errorf("daftar per model = %d baris, yang pertama harus berprioritas 5", len(byModel))
		}

		byProvider, err := models.ListProviderModels(ctx, ProviderModelFilter{ProviderID: primary.ID})
		if err != nil {
			t.Fatalf("ListProviderModels per provider: %v", err)
		}
		if len(byProvider) != 1 {
			t.Errorf("daftar per provider = %d baris, ingin 1", len(byProvider))
		}
	})

	t.Run("update sebagian termasuk mengosongkan penimpaan", func(t *testing.T) {
		updated, err := models.UpdateProviderModel(ctx, pmFallback.ID, UpdateProviderModelParams{
			UpstreamModelName: ptr("gpt-4o-2024-11-20"),
			Priority:          Clear[int](),
			Enabled:           ptr(false),
		})
		if err != nil {
			t.Fatalf("UpdateProviderModel: %v", err)
		}
		switch {
		case updated.UpstreamModelName != "gpt-4o-2024-11-20":
			t.Errorf("upstream_model_name = %q", updated.UpstreamModelName)
		case updated.Priority != nil:
			t.Errorf("priority = %v, ingin dikosongkan", updated.Priority)
		case updated.Weight == nil || *updated.Weight != 3:
			t.Errorf("weight = %v, seharusnya tidak ikut berubah", updated.Weight)
		case updated.Enabled:
			t.Error("pemetaan masih aktif")
		}

		enabledOnly, err := models.ListProviderModels(ctx, ProviderModelFilter{ModelID: model.ID, Enabled: ptr(true)})
		if err != nil {
			t.Fatalf("ListProviderModels aktif: %v", err)
		}
		if len(enabledOnly) != 1 {
			t.Errorf("pemetaan aktif = %d, ingin 1", len(enabledOnly))
		}
	})

	t.Run("detach lalu tidak ditemukan", func(t *testing.T) {
		if err := models.DetachProvider(ctx, pmFallback.ID); err != nil {
			t.Fatalf("DetachProvider: %v", err)
		}
		if err := models.DetachProvider(ctx, pmFallback.ID); !errors.Is(err, repo.ErrNotFound) {
			t.Errorf("DetachProvider kedua = %v, ingin ErrNotFound", err)
		}
	})

	t.Run("pemetaan ikut terhapus bersama providernya", func(t *testing.T) {
		if err := NewProviderRepo(pool).Delete(ctx, primary.ID); err != nil {
			t.Fatalf("Delete provider: %v", err)
		}
		remaining, err := models.ListProviderModels(ctx, ProviderModelFilter{ModelID: model.ID})
		if err != nil {
			t.Fatalf("ListProviderModels: %v", err)
		}
		if len(remaining) != 0 {
			t.Errorf("masih ada %d pemetaan setelah providernya dihapus", len(remaining))
		}
	})
}
