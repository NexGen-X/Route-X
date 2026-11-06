package admin

// Test regresi Tahap 3 batch fix menyeluruh (4 Nov 2026):
//
//   - B-087: test backend khusus toggle provider (dua arah, 400/403/404,
//     dan efek tersisih dari kandidat routing).
//   - B-088: body toggle tanpa field enabled eksplisit ditolak 400 di
//     SEMUA toggle sekeluarga — tidak pernah ditafsirkan sebagai false.
//   - B-109: ping uji webhook menolak webhook nonaktif tanpa mengirim.
//   - B-112: respons mutasi kunci API memuat cakupan tersimpan yang utuh.
//   - B-113: semantik rotate dikunci — ID baru + kunci lama revoked.
//   - B-115: menghapus webhook mempertahankan riwayat delivery-nya.

import (
	"context"
	"encoding/json"
	"io"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"net/netip"
	"strconv"
	"strings"
	"sync/atomic"
	"testing"
	"time"

	"github.com/jackc/pgx/v5/pgxpool"

	"github.com/NexGen-X/Route-X/internal/auth"
	"github.com/NexGen-X/Route-X/internal/database/repo/identity"
	"github.com/NexGen-X/Route-X/internal/database/repo/keys"
	"github.com/NexGen-X/Route-X/internal/database/repo/upstream"
	"github.com/NexGen-X/Route-X/internal/security"
	"github.com/NexGen-X/Route-X/internal/webhooks"
)

// ---------------------------------------------------------------------------
// B-087 + B-088 (toggle provider)
// ---------------------------------------------------------------------------

func TestToggleProviderDuaArahBodyKosongDanRoutingB087(t *testing.T) {
	env := setupTestEnv(t)
	ctx := context.Background()
	c := env.newClient(t)
	c.login("superadmin@routex.internal", testPassword)

	providerRepo := upstream.NewProviderRepo(env.pool)
	modelRepo := upstream.NewModelRepo(env.pool)

	prov, err := providerRepo.Create(ctx, upstream.CreateProviderParams{
		Name: "prov-toggle-b087", DisplayName: "Provider Toggle B-087",
		Kind: "openai_compatible", BaseURL: "https://b087.contoh.test/v1",
	})
	if err != nil {
		t.Fatalf("buat provider: %v", err)
	}
	model, err := modelRepo.Create(ctx, upstream.CreateModelParams{
		ModelID: "model-uji-b087", DisplayName: "Model Uji B-087",
		Capabilities: []string{upstream.CapText},
	})
	if err != nil {
		t.Fatalf("buat model: %v", err)
	}
	if _, err := modelRepo.AttachProvider(ctx, upstream.AttachProviderParams{
		ModelID: model.ID, ProviderID: prov.ID,
	}); err != nil {
		t.Fatalf("tempel model ke provider: %v", err)
	}

	jumlahKandidat := func() int {
		t.Helper()
		cands, err := providerRepo.RouteCandidates(ctx, upstream.RouteQuery{ModelID: model.ID})
		if err != nil {
			t.Fatalf("RouteCandidates: %v", err)
		}
		n := 0
		for _, cand := range cands {
			if cand.ProviderID == prov.ID {
				n++
			}
		}
		return n
	}
	if n := jumlahKandidat(); n != 1 {
		t.Fatalf("kandidat awal = %d, mau 1", n)
	}

	jalur := "/api/admin/upstreams/providers/" + prov.ID + "/toggle"

	// --- Matikan: 200, DTO enabled=false, provider tersisih dari routing. ---
	res, body := c.do(http.MethodPost, jalur, map[string]any{"enabled": false}, true)
	if res.StatusCode != http.StatusOK {
		t.Fatalf("toggle nonaktif: status %d, body %s", res.StatusCode, body)
	}
	var dto ProviderDTO
	if err := json.Unmarshal(body, &dto); err != nil {
		t.Fatalf("urai DTO toggle: %v", err)
	}
	if dto.Enabled {
		t.Error("DTO toggle nonaktif melaporkan enabled=true")
	}
	if n := jumlahKandidat(); n != 0 {
		t.Errorf("kandidat sesudah provider dimatikan = %d, mau 0 (tersisih dari routing)", n)
	}

	// --- Body kosong {} → 400 enabled_required, keadaan TIDAK berubah. ---
	res, body = c.do(http.MethodPost, jalur, map[string]any{}, true)
	if res.StatusCode != http.StatusBadRequest {
		t.Fatalf("toggle body {}: status %d, mau 400 (body %s)", res.StatusCode, body)
	}
	if !strings.Contains(string(body), "enabled_required") {
		t.Errorf("toggle body {} tidak memuat kode enabled_required: %s", body)
	}
	p, err := providerRepo.Get(ctx, prov.ID)
	if err != nil {
		t.Fatalf("baca provider: %v", err)
	}
	if p.Enabled {
		t.Error("provider berubah aktif sesudah toggle body {} ditolak — body kosong terbaca sebagai perintah")
	}

	// --- Tanpa body sama sekali → 400 juga. ---
	res, _ = c.do(http.MethodPost, jalur, nil, true)
	if res.StatusCode != http.StatusBadRequest {
		t.Errorf("toggle tanpa body: status %d, mau 400", res.StatusCode)
	}

	// --- Hidupkan lagi: 200, kandidat routing kembali. ---
	res, body = c.do(http.MethodPost, jalur, map[string]any{"enabled": true}, true)
	if res.StatusCode != http.StatusOK {
		t.Fatalf("toggle aktif: status %d, body %s", res.StatusCode, body)
	}
	dto = ProviderDTO{}
	if err := json.Unmarshal(body, &dto); err != nil {
		t.Fatalf("urai DTO toggle aktif: %v", err)
	}
	if !dto.Enabled {
		t.Error("DTO toggle aktif melaporkan enabled=false")
	}
	if n := jumlahKandidat(); n != 1 {
		t.Errorf("kandidat sesudah provider dihidupkan = %d, mau 1", n)
	}

	// --- ID yang tidak ada → 404. ---
	res, _ = c.do(http.MethodPost,
		"/api/admin/upstreams/providers/00000000-0000-0000-0000-000000000000/toggle",
		map[string]any{"enabled": true}, true)
	if res.StatusCode != http.StatusNotFound {
		t.Errorf("toggle provider hantu: status %d, mau 404", res.StatusCode)
	}

	// --- Pengguna non-admin → 403 (B-102: izin diturunkan dari kolom peran). ---
	biasa := env.newClient(t)
	biasa.login("viewer@routex.internal", testPassword)
	res, _ = biasa.do(http.MethodPost, jalur, map[string]any{"enabled": false}, true)
	if res.StatusCode != http.StatusForbidden {
		t.Errorf("toggle oleh non-admin: status %d, mau 403", res.StatusCode)
	}
}

// TestToggleKeluargaBodyKosongDitolakB088 membuktikan perbaikan B-088
// berlaku seragam: untuk setiap toggle sekeluarga, body {} dijawab 400
// dan resource TIDAK berubah, sedangkan body {enabled} yang eksplisit
// tetap bekerja. (Toggle provider dicakup tuntas oleh test B-087.)
func TestToggleKeluargaBodyKosongDitolakB088(t *testing.T) {
	env := setupTestEnv(t)
	ctx := context.Background()
	c := env.newClient(t)
	c.login("superadmin@routex.internal", testPassword)

	providerRepo := upstream.NewProviderRepo(env.pool)
	credentialRepo, err := upstream.NewCredentialRepo(env.pool, env.cipher)
	if err != nil {
		t.Fatalf("NewCredentialRepo: %v", err)
	}
	modelRepo := upstream.NewModelRepo(env.pool)

	prov, err := providerRepo.Create(ctx, upstream.CreateProviderParams{
		Name: "prov-keluarga-b088", DisplayName: "Provider Keluarga B-088",
		Kind: "openai_compatible", BaseURL: "https://b088.contoh.test/v1",
	})
	if err != nil {
		t.Fatalf("buat provider: %v", err)
	}
	cred, err := credentialRepo.Create(ctx, upstream.CreateCredentialParams{
		ProviderID: prov.ID, Label: "kred-b088", Secret: security.Secret("sk-uji-b088"),
	})
	if err != nil {
		t.Fatalf("buat kredensial: %v", err)
	}
	model, err := modelRepo.Create(ctx, upstream.CreateModelParams{
		ModelID: "model-uji-b088", DisplayName: "Model Uji B-088",
		Capabilities: []string{upstream.CapText},
	})
	if err != nil {
		t.Fatalf("buat model: %v", err)
	}

	buatLewatAPI := func(nama, jalur string, payload map[string]any) string {
		t.Helper()
		res, body := c.do(http.MethodPost, jalur, payload, true)
		if res.StatusCode != http.StatusCreated {
			t.Fatalf("buat %s: status %d, body %s", nama, res.StatusCode, body)
		}
		var dto struct {
			ID  string `json:"id"`
			Key struct {
				ID string `json:"id"`
			} `json:"key"`
		}
		if err := json.Unmarshal(body, &dto); err != nil {
			t.Fatalf("urai %s: err=%v body=%s", nama, err, body)
		}
		if dto.ID == "" {
			dto.ID = dto.Key.ID
		}
		if dto.ID == "" {
			t.Fatalf("urai %s: id kosong, body=%s", nama, body)
		}
		return dto.ID
	}

	keyID := buatLewatAPI("kunci API", "/api/admin/access/api-keys", map[string]any{"name": "Kunci B-088"})
	ruleID := buatLewatAPI("aturan routing", "/api/admin/gateway/routing-rules", map[string]any{
		// match_model_id wajib menyertai virtual_alias sejak B-165.
		"name": "aturan-b088", "strategy": "priority", "virtual_alias": "alias-b088", "pipeline": pipelineValid,
		"match_model_id": model.ID,
	})
	limitID := buatLewatAPI("batas laju", "/api/admin/gateway/rate-limits/", map[string]any{
		"scope": "model", "scope_id": model.ID, "tokens_per_minute": 100,
	})
	_ = limitID
	budgetID := buatLewatAPI("anggaran", "/api/admin/gateway/budgets/", map[string]any{
		"name": "pagu b088", "scope": "model", "scope_id": model.ID,
		"period": "daily", "limit_usd": "5",
	})
	filterID := buatLewatAPI("filter konten", "/api/admin/gateway/content-filters", map[string]any{
		"name": "filter-b088", "kind": "blocked_pattern", "priority": 50,
		"applies_to": "request", "action": "block",
		"pattern": "rahasia-b088", "pattern_type": "substring", "enabled": true,
	})
	webhookID := buatLewatAPI("webhook", "/api/admin/automation/webhooks", map[string]any{
		"name": "webhook-b088", "url": "https://example.com/hooks",
		"events": []string{"provider.unhealthy"}, "secret": "whsec_rahasia_uji_b088_panjang",
	})

	kasus := []struct {
		nama  string
		jalur string
	}{
		{"kredensial", "/api/admin/upstreams/providers/" + prov.ID + "/credentials/" + cred.ID + "/toggle"},
		{"kunci API", "/api/admin/access/api-keys/" + keyID + "/toggle"},
		{"aturan routing", "/api/admin/gateway/routing-rules/" + ruleID + "/toggle"},
		{"batas laju", "/api/admin/gateway/rate-limits/" + limitID + "/toggle"},
		{"anggaran", "/api/admin/gateway/budgets/" + budgetID + "/toggle"},
		{"filter konten", "/api/admin/gateway/content-filters/" + filterID + "/toggle"},
		{"webhook", "/api/admin/automation/webhooks/" + webhookID + "/toggle"},
	}
	for _, k := range kasus {
		t.Run(k.nama, func(t *testing.T) {
			res, body := c.do(http.MethodPost, k.jalur, map[string]any{}, true)
			if res.StatusCode != http.StatusBadRequest {
				t.Fatalf("toggle {} %s: status %d, mau 400 (body %s)", k.nama, res.StatusCode, body)
			}
			if !strings.Contains(string(body), "enabled_required") {
				t.Errorf("toggle {} %s: body tidak memuat enabled_required: %s", k.nama, body)
			}
			// Bentuk eksplisit tetap bekerja dua arah.
			res, body = c.do(http.MethodPost, k.jalur, map[string]any{"enabled": false}, true)
			if res.StatusCode != http.StatusOK {
				t.Fatalf("toggle false %s: status %d, body %s", k.nama, res.StatusCode, body)
			}
			res, body = c.do(http.MethodPost, k.jalur, map[string]any{"enabled": true}, true)
			if res.StatusCode != http.StatusOK {
				t.Fatalf("toggle true %s: status %d, body %s", k.nama, res.StatusCode, body)
			}
		})
	}

	// Keadaan akhir membuktikan {} di atas tidak pernah mematikan apa pun
	// di tengah jalan: kredensial dan webhook tetap aktif di database.
	meta, err := credentialRepo.Get(ctx, cred.ID)
	if err != nil {
		t.Fatalf("baca kredensial: %v", err)
	}
	if !meta.Enabled {
		t.Error("kredensial berakhir nonaktif — ada toggle {} yang lolos sebagai false")
	}
}

// TestToggleSesiOAuthBodyKosongDitolakB088 menutup anggota keluarga toggle
// yang terakhir: sesi OAuth.
func TestToggleSesiOAuthBodyKosongDitolakB088(t *testing.T) {
	env := setupTestEnv(t)
	ctx := context.Background()
	c := env.newClient(t)
	c.login("superadmin@routex.internal", testPassword)

	provRepo := upstream.NewProviderRepo(env.pool)
	credRepo, err := upstream.NewCredentialRepo(env.pool, env.cipher)
	if err != nil {
		t.Fatalf("NewCredentialRepo: %v", err)
	}
	oauthRepo, err := upstream.NewOAuthRepo(env.pool, env.cipher)
	if err != nil {
		t.Fatalf("NewOAuthRepo: %v", err)
	}
	env.handlers.oauthRepo = oauthRepo

	prov, err := provRepo.Create(ctx, upstream.CreateProviderParams{
		Name: "prov-oauth-b088", DisplayName: "Provider OAuth B-088", Kind: "google",
		BaseURL: "https://generativelanguage.googleapis.com",
	})
	if err != nil {
		t.Fatalf("buat provider: %v", err)
	}
	kedaluwarsa := time.Now().Add(time.Hour)
	cred, err := credRepo.Create(ctx, upstream.CreateCredentialParams{
		ProviderID: prov.ID, Label: "oauth:b088",
		Secret: security.Secret("token-akses-b088"), ExpiresAt: &kedaluwarsa,
	})
	if err != nil {
		t.Fatalf("buat kredensial: %v", err)
	}
	sesi, err := oauthRepo.UpsertSession(ctx, upstream.UpsertOAuthSessionParams{
		ProviderID: prov.ID, CredentialID: cred.ID,
		AccountEmail: "b088@contoh.test", ClientID: "client-b088",
		RefreshToken: security.Secret("refresh-b088"),
	})
	if err != nil {
		t.Fatalf("buat sesi: %v", err)
	}

	jalur := "/api/admin/upstreams/providers/" + prov.ID + "/oauth/sessions/" + sesi.ID + "/toggle"
	res, body := c.do(http.MethodPost, jalur, map[string]any{}, true)
	if res.StatusCode != http.StatusBadRequest {
		t.Fatalf("toggle sesi {}: status %d, mau 400 (body %s)", res.StatusCode, body)
	}
	meta, err := oauthRepo.GetSession(ctx, sesi.ID)
	if err != nil {
		t.Fatalf("GetSession: %v", err)
	}
	if !meta.Enabled {
		t.Error("sesi OAuth berubah nonaktif sesudah toggle {} ditolak")
	}
	res, body = c.do(http.MethodPost, jalur, map[string]any{"enabled": false}, true)
	if res.StatusCode != http.StatusOK {
		t.Fatalf("toggle sesi false: status %d, body %s", res.StatusCode, body)
	}
}

// ---------------------------------------------------------------------------
// B-109 — ping uji webhook menghormati enabled:false
// ---------------------------------------------------------------------------

func TestPingWebhookNonaktifDitolakB109(t *testing.T) {
	env := setupTestEnv(t)
	ctx := context.Background()
	c := env.newClient(t)
	c.login("superadmin@routex.internal", testPassword)

	var terkirim atomic.Int32
	penerima := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		terkirim.Add(1)
		w.WriteHeader(http.StatusOK)
	}))
	defer penerima.Close()

	// Penerima uji adalah server loopback HTTP: pembuatan webhook dan
	// dispatcher uji memakai kebijakan SSRF longgar khusus test
	// (kebijakan produksi tetap ketat).
	kebijakanUji := security.SSRFPolicy{AllowHTTP: true, AllowPrivate: true}
	repoWebhook := webhooks.NewRepo(env.pool)
	env.handlers.dispatcher = webhooks.NewDispatcher(repoWebhook, env.cipher, nil,
		kebijakanUji, slog.New(slog.NewTextHandler(io.Discard, nil)))

	buat := func(nama string, enabled bool) string {
		t.Helper()
		wh, err := repoWebhook.Create(ctx, webhooks.CreateWebhookParams{
			Name: nama, URL: penerima.URL + "/hooks",
			Events:     []string{"provider.unhealthy"},
			RawSecret:  "whsec_rahasia_uji_b109_panjang",
			Cipher:     env.cipher,
			Enabled:    enabled,
			MaxRetries: 3,
			TimeoutMS:  10000,
			SSRFPolicy: kebijakanUji,
		})
		if err != nil {
			t.Fatalf("buat webhook %s: %v", nama, err)
		}
		return wh.ID
	}

	// Webhook AKTIF: ping terkirim dan dijawab sukses (kontrol positif).
	aktifID := buat("webhook-b109-aktif", true)
	res, body := c.do(http.MethodPost, "/api/admin/automation/webhooks/"+aktifID+"/test", nil, true)
	if res.StatusCode != http.StatusOK {
		t.Fatalf("ping webhook aktif: status %d, body %s", res.StatusCode, body)
	}
	if n := terkirim.Load(); n != 1 {
		t.Fatalf("ping webhook aktif terkirim %d kali, mau 1", n)
	}

	// Webhook NONAKTIF: 400 webhook_disabled dan penerima tidak disentuh.
	nonaktifID := buat("webhook-b109-nonaktif", false)
	res, body = c.do(http.MethodPost, "/api/admin/automation/webhooks/"+nonaktifID+"/test", nil, true)
	if res.StatusCode != http.StatusBadRequest {
		t.Fatalf("ping webhook nonaktif: status %d, mau 400 (body %s)", res.StatusCode, body)
	}
	if !strings.Contains(string(body), "webhook_disabled") {
		t.Errorf("ping nonaktif tidak memuat kode webhook_disabled: %s", body)
	}
	if n := terkirim.Load(); n != 1 {
		t.Errorf("penerima menerima %d ping, mau tetap 1 — ping ke webhook nonaktif ikut terkirim", n)
	}
}

// ---------------------------------------------------------------------------
// B-112 + B-113 — respons mutasi kunci & semantik rotate
// ---------------------------------------------------------------------------

func TestMutasiKunciMemuatCakupanDanRotateB112B113(t *testing.T) {
	env := setupTestEnv(t)
	ctx := context.Background()
	c := env.newClient(t)
	c.login("superadmin@routex.internal", testPassword)

	modelRepo := upstream.NewModelRepo(env.pool)
	providerRepo := upstream.NewProviderRepo(env.pool)
	model, err := modelRepo.Create(ctx, upstream.CreateModelParams{
		ModelID: "model-uji-b112", DisplayName: "Model Uji B-112",
		Capabilities: []string{upstream.CapText},
	})
	if err != nil {
		t.Fatalf("buat model: %v", err)
	}
	prov, err := providerRepo.Create(ctx, upstream.CreateProviderParams{
		Name: "prov-b112", DisplayName: "Provider B-112",
		Kind: "openai_compatible", BaseURL: "https://b112.contoh.test/v1",
	})
	if err != nil {
		t.Fatalf("buat provider: %v", err)
	}

	// Buat kunci DENGAN cakupan model+provider.
	res, body := c.do(http.MethodPost, "/api/admin/access/api-keys", map[string]any{
		"name":         "Kunci B-112",
		"model_ids":    []string{model.ID},
		"provider_ids": []string{prov.ID},
	}, true)
	if res.StatusCode != http.StatusCreated {
		t.Fatalf("buat kunci: status %d, body %s", res.StatusCode, body)
	}
	type balasanKunci struct {
		Key struct {
			ID               string   `json:"id"`
			Status           string   `json:"status"`
			AllowedModels    []string `json:"allowed_models"`
			AllowedProviders []string `json:"allowed_providers"`
		} `json:"key"`
		RawKey string `json:"raw_key"`
	}
	var created balasanKunci
	if err := json.Unmarshal(body, &created); err != nil {
		t.Fatalf("urai respons buat: %v", err)
	}
	keyID := created.Key.ID
	base := "/api/admin/access/api-keys/" + keyID

	cekCakupan := func(tahap string, b []byte) balasanKunci {
		t.Helper()
		var out balasanKunci
		if err := json.Unmarshal(b, &out); err != nil {
			t.Fatalf("%s: urai respons: %v (body %s)", tahap, err, b)
		}
		if len(out.Key.AllowedModels) != 1 || out.Key.AllowedModels[0] != model.ID {
			t.Errorf("%s: allowed_models = %v, mau [%s] (cakupan tersimpan utuh)", tahap, out.Key.AllowedModels, model.ID)
		}
		if len(out.Key.AllowedProviders) != 1 || out.Key.AllowedProviders[0] != prov.ID {
			t.Errorf("%s: allowed_providers = %v, mau [%s]", tahap, out.Key.AllowedProviders, prov.ID)
		}
		return out
	}
	cekCakupan("buat", body)

	// PUT update (mutasi non-cakupan) → respons tetap memuat cakupan utuh.
	res, body = c.do(http.MethodPut, base, map[string]any{"name": "Kunci B-112 Diperbarui"}, true)
	if res.StatusCode != http.StatusOK {
		t.Fatalf("update kunci: status %d, body %s", res.StatusCode, body)
	}
	cekCakupan("update", body)

	// Toggle → respons tetap memuat cakupan utuh.
	res, body = c.do(http.MethodPost, base+"/toggle", map[string]any{"enabled": false}, true)
	if res.StatusCode != http.StatusOK {
		t.Fatalf("toggle kunci: status %d, body %s", res.StatusCode, body)
	}
	cekCakupan("toggle", body)
	res, _ = c.do(http.MethodPost, base+"/toggle", map[string]any{"enabled": true}, true)
	if res.StatusCode != http.StatusOK {
		t.Fatalf("toggle kunci aktif lagi: status %d", res.StatusCode)
	}

	// Rotate (B-113): ID BARU, cakupan warisan tampil utuh di respons,
	// kunci lama revoked dan raw lama mati.
	res, body = c.do(http.MethodPost, base+"/rotate", nil, true)
	if res.StatusCode != http.StatusOK {
		t.Fatalf("rotate kunci: status %d, body %s", res.StatusCode, body)
	}
	rotated := cekCakupan("rotate", body)
	if rotated.Key.ID == "" || rotated.Key.ID == keyID {
		t.Fatalf("rotate mengembalikan ID %q, mau ID BARU berbeda dari %q (semantik B-113)", rotated.Key.ID, keyID)
	}
	if rotated.RawKey == "" || rotated.RawKey == created.RawKey {
		t.Error("rotate tidak menerbitkan raw_key baru")
	}

	keyRepo, err := keys.New(env.pool, []byte("pepper-uji-32-byte-harus-cukup!"))
	if err != nil {
		t.Fatalf("keys.New: %v", err)
	}
	lama, err := keyRepo.GetByID(ctx, keyID)
	if err != nil {
		t.Fatalf("baca kunci lama: %v", err)
	}
	if lama.Status != keys.StatusRevoked {
		t.Errorf("kunci lama sesudah rotate berstatus %q, mau %q", lama.Status, keys.StatusRevoked)
	}
	ip := netip.MustParseAddr("203.0.113.10")
	if _, err := keyRepo.Authenticate(ctx, created.RawKey, ip); err == nil {
		t.Error("raw_key lama masih bisa autentikasi sesudah rotate")
	}
	if _, err := keyRepo.Authenticate(ctx, rotated.RawKey, ip); err != nil {
		t.Errorf("raw_key baru tidak bisa autentikasi: %v", err)
	}
	// Rantai rotasi terbaca: kunci baru menunjuk kunci lama.
	rantai, err := keyRepo.Lineage(ctx, rotated.Key.ID)
	if err != nil {
		t.Fatalf("Lineage: %v", err)
	}
	if len(rantai) != 2 {
		t.Errorf("rantai rotasi = %d baris, mau 2 (lama + baru)", len(rantai))
	}
}

// ---------------------------------------------------------------------------
// B-115 — hapus webhook mempertahankan riwayat delivery
// ---------------------------------------------------------------------------

func TestHapusWebhookMempertahankanDeliveryB115(t *testing.T) {
	env := setupTestEnv(t)
	ctx := context.Background()
	c := env.newClient(t)
	c.login("superadmin@routex.internal", testPassword)

	res, body := c.do(http.MethodPost, "/api/admin/automation/webhooks", map[string]any{
		"name": "webhook-b115", "url": "https://example.com/hooks",
		"events": []string{"provider.unhealthy"}, "secret": "whsec_rahasia_uji_b115_panjang",
	}, true)
	if res.StatusCode != http.StatusCreated {
		t.Fatalf("buat webhook: status %d, body %s", res.StatusCode, body)
	}
	var wh WebhookDTO
	if err := json.Unmarshal(body, &wh); err != nil {
		t.Fatalf("urai webhook: %v", err)
	}

	// Tanam satu delivery selesai sebagai riwayat.
	var deliveryID int64
	if err := env.pool.QueryRow(ctx, `
		insert into webhook_deliveries (webhook_id, event, payload, status, delivered_at)
		values ($1::uuid, 'provider.unhealthy', '{"kunci":"nilai"}'::jsonb, 'delivered', now())
		returning id`, wh.ID).Scan(&deliveryID); err != nil {
		t.Fatalf("tanam delivery: %v", err)
	}

	// Migrasi 0021: tidak ada lagi FK dari webhook_deliveries ke webhooks.
	var fk int
	if err := env.pool.QueryRow(ctx, `
		select count(*) from pg_constraint
		where conrelid = 'webhook_deliveries'::regclass and contype = 'f'`).Scan(&fk); err != nil {
		t.Fatalf("periksa constraint: %v", err)
	}
	if fk != 0 {
		t.Errorf("webhook_deliveries masih mempunyai %d foreign key, mau 0 (migrasi 0021)", fk)
	}

	// Hapus webhook lewat API.
	res, body = c.do(http.MethodDelete, "/api/admin/automation/webhooks/"+wh.ID, nil, true)
	if res.StatusCode != http.StatusOK {
		t.Fatalf("hapus webhook: status %d, body %s", res.StatusCode, body)
	}
	res, _ = c.do(http.MethodGet, "/api/admin/automation/webhooks/"+wh.ID, nil, false)
	if res.StatusCode != http.StatusNotFound {
		t.Fatalf("webhook sesudah hapus: status %d, mau 404", res.StatusCode)
	}

	// Riwayat tetap terbaca per ID delivery…
	res, body = c.do(http.MethodGet, "/api/admin/automation/deliveries/"+itoa64(deliveryID), nil, false)
	if res.StatusCode != http.StatusOK {
		t.Fatalf("GET delivery milik webhook terhapus: status %d, mau 200 (body %s)", res.StatusCode, body)
	}
	var d DeliveryDTO
	if err := json.Unmarshal(body, &d); err != nil {
		t.Fatalf("urai delivery: %v", err)
	}
	if d.WebhookID != wh.ID || d.Status != "delivered" {
		t.Errorf("delivery = webhook %q status %q, mau %q/delivered", d.WebhookID, d.Status, wh.ID)
	}

	// …dan lewat daftar per ID webhook.
	res, body = c.do(http.MethodGet, "/api/admin/automation/webhooks/"+wh.ID+"/deliveries", nil, false)
	if res.StatusCode != http.StatusOK {
		t.Fatalf("daftar delivery webhook terhapus: status %d, body %s", res.StatusCode, body)
	}
	var daftar struct {
		Items []DeliveryDTO `json:"items"`
	}
	if err := json.Unmarshal(body, &daftar); err != nil {
		t.Fatalf("urai daftar delivery: %v", err)
	}
	if len(daftar.Items) != 1 || daftar.Items[0].ID != deliveryID {
		t.Errorf("daftar delivery = %+v, mau tepat delivery %d", daftar.Items, deliveryID)
	}
}

func itoa64(v int64) string { return strconv.FormatInt(v, 10) }

// ---------------------------------------------------------------------------
// B-114 — audit restore lintas-instance: aktor tidak ada di DB tujuan
// ---------------------------------------------------------------------------

// TestWriteAuditRestoreLintasInstanceB114 mereproduksi sisa B-077 pada
// skenario migrasi antar-instance: database hasil restore berasal dari
// instance LAIN, sehingga pengguna yang melakukan restore di instance
// ini tidak mempunyai baris di tabel users database tujuan. Tulisan
// audit dengan actor_user_id aktor itu ditolak foreign key — tetapi
// catatan restore tidak boleh hilang: ia wajib mendarat lewat cuplikan
// aktor (email/peran) dengan actor_user_id NULL dan ID asal tercatat di
// metadata, sesuai desain skema audit_logs.
func TestWriteAuditRestoreLintasInstanceB114(t *testing.T) {
	env := siapkanLingkunganRestore(t)
	ctx := context.Background()

	// Aktor dari "instance lain": UUID sah yang pasti tidak ada di
	// database tujuan (database uji ini hanya bermigrasi, tanpa seed
	// pengguna satu pun).
	const idAsing = "11111111-2222-3333-4444-555555555555"
	var jumlahPengguna int
	if err := env.pool.QueryRow(ctx, `select count(*) from users`).Scan(&jumlahPengguna); err != nil {
		t.Fatalf("hitung users: %v", err)
	}
	if jumlahPengguna != 0 {
		t.Fatalf("database tujuan memuat %d users; fixture lintas-instance tidak murni", jumlahPengguna)
	}

	principal := &auth.Principal{
		User:  identity.User{ID: idAsing, Email: "admin@instance-lain.test"},
		Roles: []string{"Admin"},
	}
	ctxAktor := auth.WithPrincipal(ctx, principal)
	req := httptest.NewRequest(http.MethodPost, "/api/admin/system/backup/restore", nil)
	req = req.WithContext(ctxAktor)

	env.handler.writeAuditRestore(ctxAktor, req, map[string]any{"filename": "uji-b114.sql"})

	if n := env.hitungAuditRestore(); n != 1 {
		t.Fatalf("baris audit restore = %d, mau tepat 1 (jejak restore lintas-instance hilang)", n)
	}

	// Barisnya wajib bercuplikan aktor yang benar, tanpa actor_user_id.
	cfg := env.pool.Config().Copy()
	cfg.ConnConfig.Database = env.namaDB
	cfg.MinConns = 0
	pool, err := pgxpool.NewWithConfig(ctx, cfg)
	if err != nil {
		t.Fatalf("pool pembaca: %v", err)
	}
	defer pool.Close()
	var actorUserID, actorEmail, actorRole *string
	var metadata []byte
	if err := pool.QueryRow(ctx, `
		select actor_user_id::text, actor_email, actor_role, metadata
		from audit_logs where action = 'restore' and resource_type = 'database_backup'`,
	).Scan(&actorUserID, &actorEmail, &actorRole, &metadata); err != nil {
		t.Fatalf("baca baris audit: %v", err)
	}
	if actorUserID != nil {
		t.Errorf("actor_user_id = %q, mau NULL (aktor tidak ada di DB tujuan)", *actorUserID)
	}
	if actorEmail == nil || *actorEmail != "admin@instance-lain.test" {
		t.Errorf("actor_email = %v, mau cuplikan admin@instance-lain.test", actorEmail)
	}
	if actorRole == nil || *actorRole != "Admin" {
		t.Errorf("actor_role = %v, mau Admin", actorRole)
	}
	if !strings.Contains(string(metadata), idAsing) || !strings.Contains(string(metadata), "actor_user_id_unmapped") {
		t.Errorf("metadata tidak mencatat actor_user_id_unmapped=%s: %s", idAsing, metadata)
	}
}
