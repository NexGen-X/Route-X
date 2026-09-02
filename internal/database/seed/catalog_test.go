package seed

import (
	"strings"
	"testing"

	"github.com/NexGen-X/Route-X/internal/database/repo/upstream"
)

func TestSeedCatalogCreatesModelsAndAliases(t *testing.T) {
	ctx, pool, logger := testEnv(t)

	res, err := Run(ctx, pool, adminConfig(), logger)
	if err != nil {
		t.Fatalf("Run: %v", err)
	}
	if res.Catalog.ModelsCreated != len(catalogModels) {
		t.Errorf("ModelsCreated = %d, mau %d", res.Catalog.ModelsCreated, len(catalogModels))
	}

	models := upstream.NewModelRepo(pool)
	for _, cm := range catalogModels {
		got, err := models.GetByModelID(ctx, cm.ModelID)
		if err != nil {
			t.Errorf("model %q tidak ada: %v", cm.ModelID, err)
			continue
		}
		if got.DisplayName != cm.DisplayName {
			t.Errorf("model %q display name = %q, mau %q", cm.ModelID, got.DisplayName, cm.DisplayName)
		}
		if len(got.Capabilities) != len(cm.Capabilities) {
			t.Errorf("model %q kemampuan = %v, mau %v", cm.ModelID, got.Capabilities, cm.Capabilities)
		}

		// Alias harus bisa diresolusi ke model kanoniknya.
		for _, alias := range cm.Aliases {
			resolved, err := models.Resolve(ctx, alias)
			if err != nil {
				t.Errorf("alias %q tidak bisa diresolusi: %v", alias, err)
				continue
			}
			if resolved.ModelID != cm.ModelID {
				t.Errorf("alias %q menunjuk %q, mau %q", alias, resolved.ModelID, cm.ModelID)
			}
		}
	}
}

// Seed tidak boleh menimpa penyesuaian operator pada model yang sudah ada.
func TestSeedCatalogIsNonDestructive(t *testing.T) {
	ctx, pool, logger := testEnv(t)
	if _, err := Run(ctx, pool, adminConfig(), logger); err != nil {
		t.Fatalf("Run: %v", err)
	}

	models := upstream.NewModelRepo(pool)
	target := catalogModels[0].ModelID
	before, err := models.GetByModelID(ctx, target)
	if err != nil {
		t.Fatalf("GetByModelID: %v", err)
	}

	// Operator menyesuaikan model: matikan dan ubah prioritasnya.
	off, prio := false, 7
	if _, err := models.Update(ctx, before.ID, upstream.UpdateModelParams{
		Enabled: &off, RoutingPriority: &prio,
	}); err != nil {
		t.Fatalf("Update: %v", err)
	}

	res, err := Run(ctx, pool, adminConfig(), logger)
	if err != nil {
		t.Fatalf("Run kedua: %v", err)
	}
	if res.Catalog.ModelsCreated != 0 {
		t.Errorf("Run kedua membuat %d model, mau 0", res.Catalog.ModelsCreated)
	}
	if res.Catalog.ModelsExisting != len(catalogModels) {
		t.Errorf("ModelsExisting = %d, mau %d", res.Catalog.ModelsExisting, len(catalogModels))
	}

	after, err := models.GetByModelID(ctx, target)
	if err != nil {
		t.Fatalf("GetByModelID kedua: %v", err)
	}
	if after.Enabled {
		t.Error("seed menghidupkan kembali model yang dimatikan operator")
	}
	if after.RoutingPriority != prio {
		t.Errorf("seed menimpa prioritas operator: %d, mau %d", after.RoutingPriority, prio)
	}
}

// Katalog harus konsisten sendiri, tanpa perlu database.
func TestCatalogIsWellFormed(t *testing.T) {
	seenID := map[string]struct{}{}
	seenAlias := map[string]struct{}{}
	known := map[string]struct{}{
		upstream.CapText: {}, upstream.CapVision: {}, upstream.CapReasoning: {},
		upstream.CapTools: {}, upstream.CapEmbeddings: {},
	}

	for _, cm := range catalogModels {
		if cm.ModelID == "" || cm.DisplayName == "" {
			t.Errorf("entri katalog tanpa pengenal atau nama: %+v", cm)
		}
		if _, dup := seenID[cm.ModelID]; dup {
			t.Errorf("model %q terdaftar dua kali", cm.ModelID)
		}
		seenID[cm.ModelID] = struct{}{}

		if len(cm.Capabilities) == 0 {
			t.Errorf("model %q tanpa kemampuan", cm.ModelID)
		}
		for _, c := range cm.Capabilities {
			if _, ok := known[c]; !ok {
				t.Errorf("model %q memakai kemampuan %q yang akan ditolak constraint tabel", cm.ModelID, c)
			}
		}
		if cm.ContextWindow <= 0 {
			t.Errorf("model %q tanpa jendela konteks", cm.ModelID)
		}

		for _, alias := range cm.Aliases {
			if _, dup := seenAlias[alias]; dup {
				t.Errorf("alias %q terdaftar dua kali", alias)
			}
			seenAlias[alias] = struct{}{}
			// Alias tidak boleh menabrak pengenal kanonik mana pun — triggernya akan
			// menolaknya di database, jadi lebih baik tertangkap di sini.
			if _, clash := seenID[alias]; clash {
				t.Errorf("alias %q menabrak pengenal kanonik", alias)
			}
		}
	}

	// Periksa arah sebaliknya juga: pengenal kanonik tidak boleh sama dengan alias mana pun.
	for id := range seenID {
		if _, clash := seenAlias[id]; clash {
			t.Errorf("pengenal kanonik %q menabrak alias", id)
		}
	}
}

// Harga acuan tanpa asal tidak boleh ada: operator tidak punya cara memeriksanya.
func TestReferencePricesCarryProvenance(t *testing.T) {
	prices := ReferencePrices()
	if len(prices) == 0 {
		t.Fatal("tidak ada harga acuan sama sekali")
	}

	for _, p := range prices {
		if p.Source == "" {
			t.Errorf("harga %s/%s tanpa Source", p.ProviderKind, p.UpstreamModelName)
		}
		if p.FetchedOn == "" {
			t.Errorf("harga %s/%s tanpa FetchedOn", p.ProviderKind, p.UpstreamModelName)
		}
		if p.Input <= 0 || p.Output <= 0 {
			t.Errorf("harga %s/%s tidak wajar: input=%s output=%s",
				p.ProviderKind, p.UpstreamModelName, p.Input, p.Output)
		}
		// Output hampir selalu lebih mahal daripada input; kalau tidak, kemungkinan
		// besar dua kolomnya tertukar saat memasukkan data.
		if p.Output < p.Input {
			t.Errorf("harga %s/%s: output (%s) lebih murah dari input (%s) — kolom tertukar?",
				p.ProviderKind, p.UpstreamModelName, p.Output, p.Input)
		}
		// Harga wajib bisa disimpan tepat pada kolom numeric(14,6).
		if !p.Input.IsPriceScale() || !p.Output.IsPriceScale() {
			t.Errorf("harga %s/%s tidak muat pada skala kolom harga", p.ProviderKind, p.UpstreamModelName)
		}
		if p.CachedInput != nil {
			if *p.CachedInput > p.Input {
				t.Errorf("harga %s/%s: cache read (%s) lebih mahal dari input biasa (%s)",
					p.ProviderKind, p.UpstreamModelName, p.CachedInput, p.Input)
			}
			if !p.CachedInput.IsPriceScale() {
				t.Errorf("harga cache %s/%s tidak muat pada skala kolom harga", p.ProviderKind, p.UpstreamModelName)
			}
		}
	}
}

func TestReferencePriceFor(t *testing.T) {
	got := ReferencePriceFor(upstream.KindAnthropic, "claude-sonnet-5")
	if got == nil {
		t.Fatal("harga acuan claude-sonnet-5 tidak ditemukan")
	}
	if got.Input.Decimal() != "2.00000000" || got.Output.Decimal() != "10.00000000" {
		t.Errorf("claude-sonnet-5 = %s/%s, mau 2/10", got.Input, got.Output)
	}
	if !strings.Contains(got.Source, "platform.claude.com") {
		t.Errorf("Source = %q", got.Source)
	}

	// Model tanpa acuan harus mengembalikan nil, bukan angka karangan.
	if p := ReferencePriceFor(upstream.KindOpenAI, "gpt-5"); p != nil {
		t.Errorf("gpt-5 mengembalikan harga %s/%s padahal belum ada acuan resminya", p.Input, p.Output)
	}
	if p := ReferencePriceFor(upstream.KindAnthropic, "model-yang-tidak-ada"); p != nil {
		t.Error("model tak dikenal mengembalikan harga")
	}
}

// Nilai yang dikembalikan harus salinan, supaya pemanggil tidak bisa mengubah katalog.
func TestAccessorsReturnCopies(t *testing.T) {
	a := CatalogModels()
	if len(a) == 0 {
		t.Fatal("katalog kosong")
	}
	a[0].ModelID = "dirusak"
	if CatalogModels()[0].ModelID == "dirusak" {
		t.Error("CatalogModels mengembalikan slice yang bisa dirusak pemanggil")
	}

	p := ReferencePrices()
	p[0].Source = "dirusak"
	if ReferencePrices()[0].Source == "dirusak" {
		t.Error("ReferencePrices mengembalikan slice yang bisa dirusak pemanggil")
	}
}
