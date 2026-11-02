package admin

import (
	"context"
	"encoding/json"
	"net/http"
	"testing"

	"github.com/NexGen-X/Route-X/internal/database/repo/upstream"
)

// Test regresi audit3 B-011 untuk permukaan admin harga: V-AB-13 (cached
// input opsional) dan V-AB-12 (endpoint acuan harga katalog).

// buatPemetaan membuat provider + model + pemetaan untuk test harga.
func buatPemetaan(t *testing.T, env *testEnv, kind, namaProvider, modelID, upstreamModel string) *upstream.ProviderModel {
	t.Helper()
	ctx := context.Background()

	providerRepo := upstream.NewProviderRepo(env.pool)
	prov, err := providerRepo.Create(ctx, upstream.CreateProviderParams{
		Name: namaProvider, DisplayName: namaProvider, Kind: kind, BaseURL: "https://contoh.invalid",
	})
	if err != nil {
		t.Fatalf("buat provider: %v", err)
	}
	modelRepo := upstream.NewModelRepo(env.pool)
	m, err := modelRepo.Create(ctx, upstream.CreateModelParams{ModelID: modelID, DisplayName: modelID})
	if err != nil {
		t.Fatalf("buat model: %v", err)
	}
	pm, err := modelRepo.AttachProvider(ctx, upstream.AttachProviderParams{
		ModelID: m.ID, ProviderID: prov.ID, UpstreamModelName: upstreamModel,
	})
	if err != nil {
		t.Fatalf("buat pemetaan: %v", err)
	}
	return pm
}

// TestSetPricingCachedInputOpsional membuktikan V-AB-13: sebelum perbaikan,
// setPricing memaksa cached_input_per_1m_usd non-nil — mengosongkan field
// ditolak 400 dan "0" tersimpan sebagai nol eksplisit, padahal keduanya
// berbeda makna dari "ikuti harga input".
func TestSetPricingCachedInputOpsional(t *testing.T) {
	env := setupTestEnv(t)
	client := env.newClient(t)
	client.login("superadmin@routex.internal", testPassword)

	// Tanpa field cached sama sekali → 201/200 dan tersimpan nil (ikuti input).
	pm := buatPemetaan(t, env, "openai", "prov-harga-a", "model-harga-a", "gpt-harga-a")
	res, body := client.do(http.MethodPost,
		"/api/admin/upstreams/models/mappings/"+pm.ID+"/pricing",
		map[string]any{"input_per_1m_usd": "5.00", "output_per_1m_usd": "25.00"}, true)
	if res.StatusCode != http.StatusOK && res.StatusCode != http.StatusCreated {
		t.Fatalf("setPricing tanpa cached: status = %d (body: %s)", res.StatusCode, body)
	}
	var dto PriceDTO
	if err := json.Unmarshal(body, &dto); err != nil {
		t.Fatalf("urai DTO harga: %v", err)
	}
	if dto.CachedInputPer1MUSD != nil {
		t.Errorf("cached_input = %q, mau nil (ikuti harga input)", *dto.CachedInputPer1MUSD)
	}

	// Dengan "0" eksplisit → tersimpan nol yang memang berbeda dari nil:
	// baca cache benar-benar gratis, bukan mengikuti input.
	pm2 := buatPemetaan(t, env, "openai", "prov-harga-b", "model-harga-b", "gpt-harga-b")
	res, body = client.do(http.MethodPost,
		"/api/admin/upstreams/models/mappings/"+pm2.ID+"/pricing",
		map[string]any{"input_per_1m_usd": "5.00", "output_per_1m_usd": "25.00", "cached_input_per_1m_usd": "0"}, true)
	if res.StatusCode != http.StatusOK && res.StatusCode != http.StatusCreated {
		t.Fatalf("setPricing cached 0: status = %d (body: %s)", res.StatusCode, body)
	}
	var dto2 PriceDTO
	if err := json.Unmarshal(body, &dto2); err != nil {
		t.Fatalf("urai DTO harga kedua: %v", err)
	}
	if dto2.CachedInputPer1MUSD == nil {
		t.Fatal("cached_input = nil, mau nol eksplisit")
	}
	nol, err := upstream.ParseUSD(*dto2.CachedInputPer1MUSD)
	if err != nil || nol != 0 {
		t.Errorf("cached_input = %q, mau angka nol", *dto2.CachedInputPer1MUSD)
	}
}

// TestPricingReferenceEndpoint membuktikan V-AB-12: katalog harga acuan yang
// sebelumnya yatim kini bisa ditawarkan ke operator lewat endpoint khusus.
// Endpoint hanya MENAWARKAN — tidak memasang apa pun secara otomatis.
func TestPricingReferenceEndpoint(t *testing.T) {
	env := setupTestEnv(t)
	client := env.newClient(t)
	client.login("superadmin@routex.internal", testPassword)

	pm := buatPemetaan(t, env, "anthropic", "prov-acuan", "model-acuan", "claude-sonnet-5")
	res, body := client.do(http.MethodGet,
		"/api/admin/upstreams/models/mappings/"+pm.ID+"/pricing/reference", nil, true)
	if res.StatusCode != http.StatusOK {
		t.Fatalf("reference: status = %d (body: %s)", res.StatusCode, body)
	}
	var ref ReferencePriceDTO
	if err := json.Unmarshal(body, &ref); err != nil {
		t.Fatalf("urai DTO acuan: %v", err)
	}
	if !ref.Available {
		t.Fatalf("acuan claude-sonnet-5 tidak tersedia: %+v", ref)
	}
	if ref.ProviderKind != "anthropic" || ref.UpstreamModel != "claude-sonnet-5" {
		t.Errorf("acuan untuk %q/%q, mau anthropic/claude-sonnet-5", ref.ProviderKind, ref.UpstreamModel)
	}
	if ref.InputPer1MUSD == nil {
		t.Fatal("input acuan kosong")
	}
	if v, err := upstream.ParseUSD(*ref.InputPer1MUSD); err != nil || v != upstream.MustParseUSD("2.00") {
		t.Errorf("input acuan = %q, mau 2.00 sesuai katalog seed", *ref.InputPer1MUSD)
	}
	if ref.Source == "" || ref.FetchedOn == "" {
		t.Errorf("sumber/tanggal acuan kosong: %+v — operator tidak bisa menilai kesegarannya", ref)
	}

	// Model tanpa acuan katalog → available=false, bukan angka karangan.
	pm2 := buatPemetaan(t, env, "openai", "prov-tanpa-acuan", "model-tanpa-acuan", "gpt-tidak-ada-di-katalog")
	res, body = client.do(http.MethodGet,
		"/api/admin/upstreams/models/mappings/"+pm2.ID+"/pricing/reference", nil, true)
	if res.StatusCode != http.StatusOK {
		t.Fatalf("reference tanpa acuan: status = %d (body: %s)", res.StatusCode, body)
	}
	var ref2 ReferencePriceDTO
	if err := json.Unmarshal(body, &ref2); err != nil {
		t.Fatalf("urai DTO acuan kedua: %v", err)
	}
	if ref2.Available {
		t.Errorf("acuan untuk model tak dikenal = tersedia (%+v), mau available=false", ref2)
	}
}
