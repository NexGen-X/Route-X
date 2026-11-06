package admin

import (
	"context"
	"encoding/json"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/NexGen-X/Route-X/internal/database/repo/upstream"
	"github.com/NexGen-X/Route-X/internal/gateway"
	"github.com/NexGen-X/Route-X/internal/security"
)

// Test B-207: sinkronisasi model kini melaporkan model terpasang yang
// sudah tidak ada di upstream (tanpa melepas/menonaktifkannya — B-089),
// dan membersihkan deprecated_at bila upstream menyatakan model hidup
// lagi. Pra-fix sync hanya menambah: model yang pergi dari katalog
// upstream tidak terlihat di mana pun, dan deprecated_at yang terlanjur
// terpasang tidak pernah bisa pulih.
func TestFase7B207SyncMelaporkanModelBasiDanMemulihkanDeprecated(t *testing.T) {
	env := setupTestEnv(t)
	ctx := context.Background()
	c := env.newClient(t)
	c.login("superadmin@routex.internal", testPassword)

	prefiks, err := security.ParsePrivateAddrs([]string{"127.0.0.1", "::1"})
	if err != nil {
		t.Fatalf("ParsePrivateAddrs: %v", err)
	}
	env.handlers.factory = gateway.NewFactory(nil, nil,
		security.SSRFPolicy{AllowHTTP: true, AllowedPrivateAddrs: prefiks},
		slog.New(slog.DiscardHandler))

	provRepo := upstream.NewProviderRepo(env.pool)
	modelRepo := upstream.NewModelRepo(env.pool)

	serverJSON := func(body string) *httptest.Server {
		return httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			w.Header().Set("Content-Type", "application/json")
			_, _ = w.Write([]byte(body))
		}))
	}
	buatProvider := func(nama, base string) *upstream.Provider {
		t.Helper()
		prov, err := provRepo.Create(ctx, upstream.CreateProviderParams{
			Name: nama, DisplayName: nama, Kind: "openai_compatible", BaseURL: base,
		})
		if err != nil {
			t.Fatalf("buat provider %s: %v", nama, err)
		}
		return prov
	}
	pasangModel := func(prov *upstream.Provider, modelID string, deprecated bool) {
		t.Helper()
		params := upstream.CreateModelParams{ModelID: modelID, DisplayName: modelID}
		if deprecated {
			now := time.Now().UTC().Add(-time.Hour)
			params.DeprecatedAt = &now
		}
		m, err := modelRepo.Create(ctx, params)
		if err != nil {
			t.Fatalf("buat model %s: %v", modelID, err)
		}
		if _, err := modelRepo.AttachProvider(ctx, upstream.AttachProviderParams{
			ModelID: m.ID, ProviderID: prov.ID, UpstreamModelName: modelID,
		}); err != nil {
			t.Fatalf("pasang model %s: %v", modelID, err)
		}
	}
	// Struct lokal (bukan SyncModelsResponseDTO) supaya test terkompilasi
	// juga pada kode pra-fix dan gagal di sana pada nilainya.
	type dtoSync struct {
		Count       int      `json:"count"`
		Models      []string `json:"models"`
		StaleModels []string `json:"stale_models"`
		Message     string   `json:"message"`
	}
	sync := func(prov *upstream.Provider) dtoSync {
		t.Helper()
		res, body := c.do(http.MethodPost, "/api/admin/upstreams/providers/"+prov.ID+"/sync-models", map[string]any{}, true)
		if res.StatusCode != http.StatusOK {
			t.Fatalf("sync %s: status %d (body %s)", prov.Name, res.StatusCode, body)
		}
		var dto dtoSync
		if err := json.Unmarshal(body, &dto); err != nil {
			t.Fatalf("sync %s: urai dto: %v", prov.Name, err)
		}
		return dto
	}

	// --- Model basi: terpasang lokal, hilang dari upstream ------------
	srvStale := serverJSON(`{"data":[{"id":"model-b207-tetap"}]}`)
	defer srvStale.Close()
	provStale := buatProvider("b207-basi", srvStale.URL)
	pasangModel(provStale, "model-b207-hilang", false)

	dto := sync(provStale)
	if len(dto.StaleModels) != 1 || dto.StaleModels[0] != "model-b207-hilang" {
		t.Errorf("stale_models = %v, mau [model-b207-hilang]", dto.StaleModels)
	}
	if !strings.Contains(dto.Message, "tidak lagi ada di upstream") ||
		!strings.Contains(dto.Message, "model-b207-hilang") {
		t.Errorf("pesan sync tidak melaporkan model basi: %q", dto.Message)
	}
	// Model basi TIDAK diubah: pemetaan tetap terpasang (keputusan B-089).
	peta, err := modelRepo.ListProviderModels(ctx, upstream.ProviderModelFilter{ProviderID: provStale.ID})
	if err != nil {
		t.Fatalf("ListProviderModels: %v", err)
	}
	adaBasi := false
	for _, pm := range peta {
		if pm.UpstreamModelName == "model-b207-hilang" {
			adaBasi = true
		}
	}
	if !adaBasi {
		t.Error("pemetaan model basi dilepas oleh sync — seharusnya hanya dilaporkan")
	}
	// Audit sync memuat daftar model basi.
	var metadata string
	if err := env.pool.QueryRow(ctx,
		`select metadata::text from audit_logs
		  where action = 'sync_models' and resource_type = 'provider' and resource_id = $1
		  order by occurred_at desc limit 1`, provStale.ID).Scan(&metadata); err != nil {
		t.Fatalf("baca audit sync: %v", err)
	}
	if !strings.Contains(metadata, `"stale_models"`) || !strings.Contains(metadata, "model-b207-hilang") {
		t.Errorf("metadata audit sync tidak memuat stale_models: %s", metadata)
	}

	// --- Pemulihan: deprecated_at dibersihkan oleh lifecycle aktif ----
	srvPulih := serverJSON(`{"data":[{"id":"model-b207-pulih","status":"active"}]}`)
	defer srvPulih.Close()
	provPulih := buatProvider("b207-pulih", srvPulih.URL)
	pasangModel(provPulih, "model-b207-pulih", true)

	sync(provPulih)
	mPulih, err := modelRepo.GetByModelID(ctx, "model-b207-pulih")
	if err != nil {
		t.Fatalf("baca model pulih: %v", err)
	}
	if mPulih.DeprecatedAt != nil {
		t.Errorf("deprecated_at model pulih = %v, mau nil (dibersihkan)", mPulih.DeprecatedAt)
	}

	// --- Kontrol: tanpa lifecycle, deprecated_at tidak disimpulkan ----
	srvKontrol := serverJSON(`{"data":[{"id":"model-b207-kontrol"}]}`)
	defer srvKontrol.Close()
	provKontrol := buatProvider("b207-kontrol", srvKontrol.URL)
	pasangModel(provKontrol, "model-b207-kontrol", true)

	dtoKontrol := sync(provKontrol)
	mKontrol, err := modelRepo.GetByModelID(ctx, "model-b207-kontrol")
	if err != nil {
		t.Fatalf("baca model kontrol: %v", err)
	}
	if mKontrol.DeprecatedAt == nil {
		t.Error("deprecated_at model kontrol dibersihkan tanpa bukti lifecycle — ketiadaan lifecycle bukan bukti hidup")
	}
	if len(dtoKontrol.StaleModels) != 0 {
		t.Errorf("stale_models kontrol = %v, mau kosong (modelnya masih ditemukan)", dtoKontrol.StaleModels)
	}
}
