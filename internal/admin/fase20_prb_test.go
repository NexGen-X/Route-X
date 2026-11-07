package admin

// fase20_prb_test.go — pengunci Fase 20 PR-B (admin nilai-sama &
// kejujuran).
//
//	B-467 normalisasi huruf UUID pada deteksi no-op ikatan routing
//	      dan pencocokan bobot (kasus sama harus diakui sebagai
//	      anggota yang sama; bobot tidak boleh hilang senyap).
//	B-468 updateRoutingRule membandingkan NILAI field (dulu
//	      keberadaan pointer) dan pengecualian ikatannya memakai
//	      himpunan teresolusi yang terikat pada aturan ini.
//	B-469 enumerasi permukaan nilai-sama: PUT yang seluruh field
//	      disebutnya nilai-sama → 400 no_changes tanpa audit;
//	      toggle/set ke keadaan yang sudah berlaku → 200 diam
//	      tanpa audit dan tanpa efek samping (siraman cache).
//	B-470 URL webhook kosong → 400 blocked_url, bukan 500.
//	B-471 setAllowed pada kunci yang tidak ada → 404.
//	B-472 hapus konfigurasi domain transaksional: keadaan parsial
//	      tidak lagi gagal-sesudah-terhapus.

import (
	"context"
	"encoding/json"
	"net/http"
	"strings"
	"testing"
	"time"

	"github.com/NexGen-X/Route-X/internal/database/repo/identity"
	upstreamrepo "github.com/NexGen-X/Route-X/internal/database/repo/upstream"
	"github.com/NexGen-X/Route-X/internal/responsecache"
	"github.com/NexGen-X/Route-X/internal/security"
)

// buatEntitasB469 membuat satu entitas per jenis permukaan untuk
// tabel nilai-sama. Seluruh pembuatan berjalan lewat repo supaya
// bentuknya persis seperti data produksi (pool egress tidak bisa
// dibuat lewat API dengan proxy loopback — guard SSRF).
func buatEntitasB469(t *testing.T, env *testEnv) (providerID, modelID, poolID, credID, sesiID string) {
	t.Helper()
	ctx := context.Background()

	provRepo := upstreamrepo.NewProviderRepo(env.pool)
	p, err := provRepo.Create(ctx, upstreamrepo.CreateProviderParams{
		Name: "prov-b469", DisplayName: "Provider B469", Kind: "openai_compatible",
		BaseURL: "https://api.contoh.invalid/v1", Enabled: boolPtr(true),
	})
	if err != nil {
		t.Fatalf("buat provider: %v", err)
	}
	providerID = p.ID

	credRepo, err := upstreamrepo.NewCredentialRepo(env.pool, env.cipher)
	if err != nil {
		t.Fatalf("NewCredentialRepo: %v", err)
	}
	cred, err := credRepo.Create(ctx, upstreamrepo.CreateCredentialParams{
		ProviderID: providerID, Label: "kred-b469",
		Secret: security.Secret("GANTI_kredensial_b469"),
	})
	if err != nil {
		t.Fatalf("buat kredensial: %v", err)
	}
	credID = cred.ID

	modelRepo := upstreamrepo.NewModelRepo(env.pool)
	m, err := modelRepo.Create(ctx, upstreamrepo.CreateModelParams{
		ModelID: "model-b469", DisplayName: "Model B469", Enabled: boolPtr(true),
	})
	if err != nil {
		t.Fatalf("buat model: %v", err)
	}
	modelID = m.ID

	egressRepo, err := upstreamrepo.NewEgressRepo(env.pool, env.cipher)
	if err != nil {
		t.Fatalf("NewEgressRepo: %v", err)
	}
	pool, err := egressRepo.Create(ctx, upstreamrepo.CreateEgressParams{
		Name: "pool-b469", Kind: upstreamrepo.EgressHTTP,
		ProxyURL: security.Secret("http://127.0.0.1:8899"), Enabled: boolPtr(true),
	})
	if err != nil {
		t.Fatalf("buat egress pool: %v", err)
	}
	poolID = pool.ID

	oauthRepo, err := upstreamrepo.NewOAuthRepo(env.pool, env.cipher)
	if err != nil {
		t.Fatalf("NewOAuthRepo: %v", err)
	}
	sesi, err := oauthRepo.UpsertSession(ctx, upstreamrepo.UpsertOAuthSessionParams{
		ProviderID: providerID, CredentialID: credID,
		AccountEmail: "b469@contoh.invalid", ClientID: "client-b469",
		RefreshToken: security.Secret("GANTI_refresh_b469"),
	})
	if err != nil {
		t.Fatalf("buat sesi oauth: %v", err)
	}
	sesiID = sesi.ID
	env.handlers.oauthRepo = oauthRepo
	return providerID, modelID, poolID, credID, sesiID
}

// TestB469UpdateNilaiSamaDitolak membuktikan keluarga PUT: kirim
// ulang nilai yang tersimpan → 400 no_changes dan audit TIDAK
// naik; ubah satu nilai → 200 dan audit naik tepat satu.
func TestB469UpdateNilaiSamaDitolak(t *testing.T) {
	env := setupTestEnv(t)
	c := env.newClient(t)
	c.login("superadmin@routex.internal", testPassword)
	providerID, modelID, poolID, _, _ := buatEntitasB469(t, env)

	kasus := []struct {
		nama          string
		url           string
		tubuhSama     map[string]any
		tubuhBerubah  map[string]any
		aksi, tipeRes string
	}{
		{"provider", "/api/admin/upstreams/providers/" + providerID,
			map[string]any{"display_name": "Provider B469"}, map[string]any{"display_name": "Provider B469 Baru"},
			"update", "provider"},
		{"model", "/api/admin/upstreams/models/" + modelID,
			map[string]any{"display_name": "Model B469"}, map[string]any{"display_name": "Model B469 Baru"},
			"update", "model"},
		{"egress pool", "/api/admin/upstreams/egress-pools/" + poolID,
			map[string]any{"name": "pool-b469"}, map[string]any{"name": "pool-b469-baru"},
			"update", "egress_pool"},
	}

	// Entitas kebijakan & otomasi dibuat lewat API (bentuknya sah).
	mk := func(url string, body map[string]any) string {
		res, b := c.do(http.MethodPost, url, body, true)
		if res.StatusCode != http.StatusOK && res.StatusCode != http.StatusCreated {
			t.Fatalf("buat %s: status = %d (body: %s)", url, res.StatusCode, b)
		}
		var out struct {
			ID  string `json:"id"`
			Key struct {
				ID string `json:"id"`
			} `json:"key"`
		}
		if err := json.Unmarshal(b, &out); err != nil || (out.ID == "" && out.Key.ID == "") {
			t.Fatalf("urai id %s: %v %s", url, err, b)
		}
		if out.ID != "" {
			return out.ID
		}
		return out.Key.ID
	}
	kasus = append(kasus,
		struct {
			nama          string
			url           string
			tubuhSama     map[string]any
			tubuhBerubah  map[string]any
			aksi, tipeRes string
		}{"rate limit", "/api/admin/gateway/rate-limits/" + mk("/api/admin/gateway/rate-limits",
			map[string]any{"scope": "global", "requests_per_minute": 100, "tokens_per_minute": 1000}),
			map[string]any{"tokens_per_minute": 1000}, map[string]any{"tokens_per_minute": 2000},
			"update", "rate_limit"},
		struct {
			nama          string
			url           string
			tubuhSama     map[string]any
			tubuhBerubah  map[string]any
			aksi, tipeRes string
		}{"budget", "/api/admin/gateway/budgets/" + mk("/api/admin/gateway/budgets",
			map[string]any{"name": "pagu-b469", "limit_usd": "10", "period": "monthly", "scope": "global"}),
			map[string]any{"name": "pagu-b469"}, map[string]any{"name": "pagu-b469-baru"},
			"update", "budget"},
		struct {
			nama          string
			url           string
			tubuhSama     map[string]any
			tubuhBerubah  map[string]any
			aksi, tipeRes string
		}{"api key", "/api/admin/access/api-keys/" + mk("/api/admin/access/api-keys",
			map[string]any{"name": "Kunci B469"}),
			map[string]any{"name": "Kunci B469"}, map[string]any{"name": "Kunci B469 Baru"},
			"update", "api_key"},
		struct {
			nama          string
			url           string
			tubuhSama     map[string]any
			tubuhBerubah  map[string]any
			aksi, tipeRes string
		}{"webhook", "/api/admin/automation/webhooks/" + mk("/api/admin/automation/webhooks",
			map[string]any{"name": "webhook-b469", "url": "https://8.8.8.8/hook-b469", "events": []string{"request.completed"}, "secret": "whsec-b469-ujian"}),
			map[string]any{"name": "webhook-b469"}, map[string]any{"name": "webhook-b469-baru"},
			"update", "webhook"},
	)

	for _, k := range kasus {
		dasar := hitungAudit(t, env, k.aksi, k.tipeRes)
		res, body := c.do(http.MethodPut, k.url, k.tubuhSama, true)
		if res.StatusCode != http.StatusBadRequest || !strings.Contains(string(body), "no_changes") {
			t.Errorf("%s nilai-sama: status = %d, mau 400 no_changes (body: %s)", k.nama, res.StatusCode, body)
		}
		if n := hitungAudit(t, env, k.aksi, k.tipeRes); n != dasar {
			t.Errorf("%s nilai-sama: audit %s/%s naik %d → %d — audit palsu", k.nama, k.aksi, k.tipeRes, dasar, n)
		}
		res, body = c.do(http.MethodPut, k.url, k.tubuhBerubah, true)
		if res.StatusCode != http.StatusOK {
			t.Errorf("%s berubah: status = %d, mau 200 (body: %s)", k.nama, res.StatusCode, body)
		}
		if n := hitungAudit(t, env, k.aksi, k.tipeRes); n != dasar+1 {
			t.Errorf("%s berubah: audit naik ke %d, mau %d", k.nama, n, dasar+1)
		}
	}
}

// TestB469ToggleNilaiSamaDiam membuktikan keluarga toggle & set:
// keadaan yang diminta sudah berlaku → 200 tanpa audit; transisi
// nyata → audit naik tepat satu. Untuk kredensial dibuktikan juga
// siraman cache hanya terjadi pada transisi nyata.
func TestB469ToggleNilaiSamaDiam(t *testing.T) {
	env := setupTestEnv(t)
	c := env.newClient(t)
	c.login("superadmin@routex.internal", testPassword)
	providerID, _, poolID, credID, sesiID := buatEntitasB469(t, env)

	mk := func(url string, body map[string]any) string {
		res, b := c.do(http.MethodPost, url, body, true)
		if res.StatusCode != http.StatusOK && res.StatusCode != http.StatusCreated {
			t.Fatalf("buat %s: status = %d (body: %s)", url, res.StatusCode, b)
		}
		var out struct {
			ID  string `json:"id"`
			Key struct {
				ID string `json:"id"`
			} `json:"key"`
		}
		if err := json.Unmarshal(b, &out); err != nil || (out.ID == "" && out.Key.ID == "") {
			t.Fatalf("urai id %s: %v %s", url, err, b)
		}
		if out.ID != "" {
			return out.ID
		}
		return out.Key.ID
	}
	ruleID := mk("/api/admin/gateway/routing-rules", map[string]any{"name": "aturan-b469", "strategy": "priority"})
	rlID := mk("/api/admin/gateway/rate-limits", map[string]any{"scope": "global", "requests_per_minute": 60})
	budgetID := mk("/api/admin/gateway/budgets", map[string]any{"name": "pagu-tgl-b469", "limit_usd": "5", "period": "monthly", "scope": "global"})
	filterID := mk("/api/admin/gateway/content-filters", map[string]any{"name": "filter-tgl-b469", "kind": "blocked_pattern", "pattern": "x-b469", "pattern_type": "substring", "action": "block"})
	keyID := mk("/api/admin/access/api-keys", map[string]any{"name": "Kunci Toggle B469"})
	whID := mk("/api/admin/automation/webhooks", map[string]any{"name": "wh-tgl-b469", "url": "https://8.8.8.8/hook-tgl", "events": []string{"request.completed"}, "secret": "whsec-b469-tgl"})

	kasus := []struct {
		nama          string
		url           string
		aksi, tipeRes string
	}{
		{"provider", "/api/admin/upstreams/providers/" + providerID + "/toggle", "toggle", "provider"},
		{"api key", "/api/admin/access/api-keys/" + keyID + "/toggle", "toggle", "api_key"},
		{"webhook", "/api/admin/automation/webhooks/" + whID + "/toggle", "toggle", "webhook"},
		{"routing rule", "/api/admin/gateway/routing-rules/" + ruleID + "/toggle", "toggle", "routing_rule"},
		{"rate limit", "/api/admin/gateway/rate-limits/" + rlID + "/toggle", "toggle", "rate_limit"},
		{"budget", "/api/admin/gateway/budgets/" + budgetID + "/toggle", "toggle", "budget"},
		{"content filter", "/api/admin/gateway/content-filters/" + filterID + "/toggle", "toggle", "content_filter"},
		{"sesi oauth", "/api/admin/upstreams/providers/" + providerID + "/oauth/sessions/" + sesiID + "/toggle", "toggle_oauth_session", "provider"},
	}
	for _, k := range kasus {
		dasar := hitungAudit(t, env, k.aksi, k.tipeRes)
		res, body := c.do(http.MethodPost, k.url, map[string]any{"enabled": true}, true)
		if res.StatusCode != http.StatusOK {
			t.Errorf("%s nilai-sama: status = %d, mau 200 (body: %s)", k.nama, res.StatusCode, body)
			continue
		}
		if n := hitungAudit(t, env, k.aksi, k.tipeRes); n != dasar {
			t.Errorf("%s nilai-sama: audit naik %d → %d — audit palsu", k.nama, dasar, n)
		}
		res, body = c.do(http.MethodPost, k.url, map[string]any{"enabled": false}, true)
		if res.StatusCode != http.StatusOK {
			t.Errorf("%s transisi: status = %d, mau 200 (body: %s)", k.nama, res.StatusCode, body)
		}
		if n := hitungAudit(t, env, k.aksi, k.tipeRes); n != dasar+1 {
			t.Errorf("%s transisi: audit = %d, mau %d", k.nama, n, dasar+1)
		}
		// Kembalikan ke aktif supaya permukaan lain tak terpengaruh.
		c.do(http.MethodPost, k.url, map[string]any{"enabled": true}, true)
	}

	// Toggle kredensial: nilai-sama tanpa audit DAN tanpa siraman;
	// transisi nyata dengan audit dan siraman.
	mesin := mesinCacheUntukAdmin(t)
	mesin.SetSettings(true, time.Minute)
	env.handlers.responseCache = mesin
	ctxCache := context.Background()
	tanamCache := func() {
		if err := mesin.Set(ctxCache, "kunci-b469", &responsecache.Entry{
			Model: "model-b469", Raw: json.RawMessage(`{"id":"chatcmpl-b469"}`),
		}); err != nil {
			t.Fatalf("tanam entri cache: %v", err)
		}
	}
	hitCache := func() bool {
		_, hit, err := mesin.Get(ctxCache, "kunci-b469")
		return err == nil && hit
	}
	tanamCache()
	dasar := hitungAudit(t, env, "toggle", "credential")
	urlCred := "/api/admin/upstreams/providers/" + providerID + "/credentials/" + credID + "/toggle"
	res, body := c.do(http.MethodPost, urlCred, map[string]any{"enabled": true}, true)
	if res.StatusCode != http.StatusOK {
		t.Fatalf("toggle kredensial nilai-sama: status = %d (body: %s)", res.StatusCode, body)
	}
	if n := hitungAudit(t, env, "toggle", "credential"); n != dasar {
		t.Errorf("toggle kredensial nilai-sama: audit naik ke %d", n)
	}
	if !hitCache() {
		t.Error("toggle kredensial nilai-sama menyiram cache — efek samping no-op")
	}
	res, _ = c.do(http.MethodPost, urlCred, map[string]any{"enabled": false}, true)
	if res.StatusCode != http.StatusOK {
		t.Fatalf("toggle kredensial transisi: status = %d", res.StatusCode)
	}
	if n := hitungAudit(t, env, "toggle", "credential"); n != dasar+1 {
		t.Errorf("toggle kredensial transisi: audit = %d, mau %d", n, dasar+1)
	}
	if hitCache() {
		t.Error("toggle kredensial transisi TIDAK menyiram cache")
	}
	c.do(http.MethodPost, urlCred, map[string]any{"enabled": true}, true)

	// setCredentialEgressPool: lepas-saat-terlepas dan ikat-ulang
	// pool yang sama adalah no-op; pengikatan nyata tetap teraudit.
	urlPool := "/api/admin/upstreams/providers/" + providerID + "/credentials/" + credID + "/egress-pool"
	dasar = hitungAudit(t, env, "update_egress_pool", "credential")
	res, body = c.do(http.MethodPut, urlPool, map[string]any{"egress_pool_id": nil}, true)
	if res.StatusCode != http.StatusOK {
		t.Fatalf("lepas pool saat terlepas: status = %d (body: %s)", res.StatusCode, body)
	}
	if n := hitungAudit(t, env, "update_egress_pool", "credential"); n != dasar {
		t.Errorf("lepas pool saat terlepas: audit naik ke %d — audit palsu", n)
	}
	res, _ = c.do(http.MethodPut, urlPool, map[string]any{"egress_pool_id": poolID}, true)
	if res.StatusCode != http.StatusOK {
		t.Fatalf("ikat pool: status = %d", res.StatusCode)
	}
	if n := hitungAudit(t, env, "update_egress_pool", "credential"); n != dasar+1 {
		t.Fatalf("ikat pool: audit = %d, mau %d", n, dasar+1)
	}
	res, _ = c.do(http.MethodPut, urlPool, map[string]any{"egress_pool_id": poolID}, true)
	if res.StatusCode != http.StatusOK {
		t.Fatalf("ikat ulang pool sama: status = %d", res.StatusCode)
	}
	if n := hitungAudit(t, env, "update_egress_pool", "credential"); n != dasar+1 {
		t.Errorf("ikat ulang pool sama: audit naik ke %d — audit palsu", n)
	}

	// Strategi kredensial: strategi yang berlaku adalah no-op.
	urlStrategi := "/api/admin/upstreams/providers/" + providerID + "/credential-strategy"
	dasar = hitungAudit(t, env, "update_credential_strategy", "provider")
	res, body = c.do(http.MethodPost, urlStrategi, map[string]any{"strategy": "round_robin"}, true)
	if res.StatusCode != http.StatusOK {
		t.Fatalf("strategi nilai-sama: status = %d (body: %s)", res.StatusCode, body)
	}
	if n := hitungAudit(t, env, "update_credential_strategy", "provider"); n != dasar {
		t.Errorf("strategi nilai-sama: audit naik ke %d — audit palsu", n)
	}
	res, _ = c.do(http.MethodPost, urlStrategi, map[string]any{"strategy": "priority"}, true)
	if res.StatusCode != http.StatusOK {
		t.Fatalf("strategi berubah: status = %d", res.StatusCode)
	}
	if n := hitungAudit(t, env, "update_credential_strategy", "provider"); n != dasar+1 {
		t.Errorf("strategi berubah: audit = %d, mau %d", n, dasar+1)
	}

	// Setelan cache: kirim ulang keadaan efektif adalah no-op.
	dasar = hitungAudit(t, env, "update", "response_cache")
	res, body = c.do(http.MethodPost, "/api/admin/system/cache/settings",
		map[string]any{"enabled": true, "ttl_seconds": 3600}, true)
	if res.StatusCode != http.StatusOK {
		t.Fatalf("cache settings perubahan: status = %d (body: %s)", res.StatusCode, body)
	}
	if n := hitungAudit(t, env, "update", "response_cache"); n != dasar+1 {
		t.Fatalf("cache settings perubahan: audit = %d, mau %d", n, dasar+1)
	}
	res, body = c.do(http.MethodPost, "/api/admin/system/cache/settings",
		map[string]any{"enabled": true, "ttl_seconds": 3600}, true)
	if res.StatusCode != http.StatusOK {
		t.Fatalf("cache settings nilai-sama: status = %d (body: %s)", res.StatusCode, body)
	}
	if n := hitungAudit(t, env, "update", "response_cache"); n != dasar+1 {
		t.Errorf("cache settings nilai-sama: audit naik ke %d — audit palsu", n)
	}

	// putSetting: nilai + deskripsi yang sama adalah no-op.
	dasar = hitungAudit(t, env, "put", "setting")
	res, body = c.do(http.MethodPut, "/api/admin/system/settings/kunci-b469",
		map[string]any{"value": map[string]any{"x": 1}, "description": "probe b469"}, true)
	if res.StatusCode != http.StatusOK {
		t.Fatalf("put setting pertama: status = %d (body: %s)", res.StatusCode, body)
	}
	if n := hitungAudit(t, env, "put", "setting"); n != dasar+1 {
		t.Fatalf("put setting pertama: audit = %d, mau %d", n, dasar+1)
	}
	res, _ = c.do(http.MethodPut, "/api/admin/system/settings/kunci-b469",
		map[string]any{"value": map[string]any{"x": 1}, "description": "probe b469"}, true)
	if res.StatusCode != http.StatusOK {
		t.Fatalf("put setting nilai-sama: status = %d", res.StatusCode)
	}
	if n := hitungAudit(t, env, "put", "setting"); n != dasar+1 {
		t.Errorf("put setting nilai-sama: audit naik ke %d — audit palsu", n)
	}
}

// TestB469ContentFilterNilaiSamaDiam mengunci perlakuan khusus
// updateContentFilter: bodynya form-penuh (B-418), jadi no-op
// dijawab 200 diam bergaya endpoint set — tanpa audit dan tanpa
// tulisan — sementara perubahan nyata tetap teraudit.
func TestB469ContentFilterNilaiSamaDiam(t *testing.T) {
	env := setupTestEnv(t)
	c := env.newClient(t)
	c.login("superadmin@routex.internal", testPassword)

	res, body := c.do(http.MethodPost, "/api/admin/gateway/content-filters",
		map[string]any{"name": "filter-b469", "kind": "blocked_pattern", "pattern": "larangan-b469", "pattern_type": "substring", "action": "block"}, true)
	if res.StatusCode != http.StatusOK && res.StatusCode != http.StatusCreated {
		t.Fatalf("buat filter: status = %d (body: %s)", res.StatusCode, body)
	}
	var dibuat struct {
		ID string `json:"id"`
	}
	if err := json.Unmarshal(body, &dibuat); err != nil || dibuat.ID == "" {
		t.Fatalf("urai filter: %v %s", err, body)
	}
	url := "/api/admin/gateway/content-filters/" + dibuat.ID

	dasar := hitungAudit(t, env, "update", "content_filter")
	res, body = c.do(http.MethodPut, url, map[string]any{"name": "filter-b469"}, true)
	if res.StatusCode != http.StatusOK {
		t.Fatalf("PUT nama sama: status = %d, mau 200 diam (body: %s)", res.StatusCode, body)
	}
	if n := hitungAudit(t, env, "update", "content_filter"); n != dasar {
		t.Errorf("PUT nama sama: audit naik ke %d — audit palsu", n)
	}
	res, body = c.do(http.MethodPut, url, map[string]any{"name": "filter-b469-baru"}, true)
	if res.StatusCode != http.StatusOK {
		t.Fatalf("PUT nama berubah: status = %d (body: %s)", res.StatusCode, body)
	}
	if n := hitungAudit(t, env, "update", "content_filter"); n != dasar+1 {
		t.Errorf("PUT nama berubah: audit = %d, mau %d", n, dasar+1)
	}
}

// TestB467B468IkatanDanNilaiRouting membuktikan B-467 (UUID beda
// huruf diakui anggota yang sama; bobot cocok lintas huruf) dan
// B-468 (PUT aturan dengan nilai + ikatan identik tidak teraudit).
func TestB467B468IkatanDanNilaiRouting(t *testing.T) {
	env := setupTestEnv(t)
	c := env.newClient(t)
	c.login("superadmin@routex.internal", testPassword)
	providerID, _, _, _, _ := buatEntitasB469(t, env)
	providerRepo := upstreamrepo.NewProviderRepo(env.pool)
	p2, err := providerRepo.Create(context.Background(), upstreamrepo.CreateProviderParams{
		Name: "prov-b467-b", DisplayName: "Provider B467 B", Kind: "openai_compatible",
		BaseURL: "https://api.contoh.invalid/v1", Enabled: boolPtr(true),
	})
	if err != nil {
		t.Fatal(err)
	}

	res, body := c.do(http.MethodPost, "/api/admin/gateway/routing-rules",
		map[string]any{"name": "aturan-b467", "strategy": "weighted"}, true)
	if res.StatusCode != http.StatusOK && res.StatusCode != http.StatusCreated {
		t.Fatalf("buat aturan: status = %d (body: %s)", res.StatusCode, body)
	}
	var rule struct {
		ID string `json:"id"`
	}
	if err := json.Unmarshal(body, &rule); err != nil || rule.ID == "" {
		t.Fatalf("urai aturan: %v %s", err, body)
	}

	bobot := func() map[string]int {
		res, body := c.do(http.MethodGet, "/api/admin/gateway/routing-rules/"+rule.ID, nil, true)
		if res.StatusCode != http.StatusOK {
			t.Fatalf("GET aturan: status = %d (body: %s)", res.StatusCode, body)
		}
		var dto RoutingRuleDTO
		if err := json.Unmarshal(body, &dto); err != nil {
			t.Fatalf("urai DTO aturan: %v", err)
		}
		out := map[string]int{}
		for _, rp := range dto.Providers {
			if rp.Weight != nil {
				out[rp.ProviderID] = *rp.Weight
			}
		}
		return out
	}

	// Ikatan ditulis dengan id HURUF BESAR dan kunci bobot huruf
	// kecil: anggota harus diakui dan bobotnya menempel (dulu bobot
	// hilang senyap karena kunci peta tidak dinormalisasi).
	atas, bawah := strings.ToUpper(providerID), strings.ToLower(providerID)
	atas2, bawah2 := strings.ToUpper(p2.ID), strings.ToLower(p2.ID)
	res, body = c.do(http.MethodPost, "/api/admin/gateway/routing-rules/"+rule.ID+"/providers",
		map[string]any{
			"provider_ids": []string{atas, atas2},
			"weights":      map[string]int{bawah: 5, bawah2: 7},
		}, true)
	if res.StatusCode != http.StatusOK {
		t.Fatalf("set providers huruf campur: status = %d (body: %s)", res.StatusCode, body)
	}
	if w := bobot(); w[bawah] != 5 || w[bawah2] != 7 {
		t.Fatalf("bobot sesudah set huruf campur = %v, mau {%s:5 %s:7} — bobot hilang atau ikatan ditolak", w, bawah, bawah2)
	}
	if n := hitungAudit(t, env, "set_providers", "routing_rule"); n != 1 {
		t.Fatalf("audit set_providers = %d, mau 1", n)
	}

	// Kirim ulang himpunan yang sama dengan huruf sebaliknya:
	// no-op penuh, tanpa audit kedua dan bobot utuh.
	res, body = c.do(http.MethodPost, "/api/admin/gateway/routing-rules/"+rule.ID+"/providers",
		map[string]any{
			"provider_ids": []string{bawah, bawah2},
			"weights":      map[string]int{atas: 5, atas2: 7},
		}, true)
	if res.StatusCode != http.StatusOK {
		t.Fatalf("set providers identik: status = %d (body: %s)", res.StatusCode, body)
	}
	if n := hitungAudit(t, env, "set_providers", "routing_rule"); n != 1 {
		t.Errorf("set providers identik beda huruf menaikkan audit ke %d — deteksi no-op kalah oleh huruf", n)
	}
	if w := bobot(); w[bawah] != 5 || w[bawah2] != 7 {
		t.Errorf("bobot berubah sesudah set identik: %v", w)
	}

	// B-468 + B-467 lewat PUT aturan: nama sama & ikatan identik
	// (huruf campur) → 200 tanpa audit update.
	res, body = c.do(http.MethodPut, "/api/admin/gateway/routing-rules/"+rule.ID,
		map[string]any{
			"name":         "aturan-b467",
			"provider_ids": []string{atas, atas2},
			"weights":      map[string]int{bawah: 5, bawah2: 7},
		}, true)
	if res.StatusCode != http.StatusOK {
		t.Fatalf("PUT aturan identik: status = %d (body: %s)", res.StatusCode, body)
	}
	if n := hitungAudit(t, env, "update", "routing_rule"); n != 0 {
		t.Errorf("PUT aturan nilai-sama + ikatan identik menaikkan audit update ke %d — audit palsu", n)
	}

	// Kontrol: ubah bobot lewat PUT → audit naik dan bobot berubah.
	res, body = c.do(http.MethodPut, "/api/admin/gateway/routing-rules/"+rule.ID,
		map[string]any{
			"provider_ids": []string{bawah, bawah2},
			"weights":      map[string]int{bawah: 9, bawah2: 7},
		}, true)
	if res.StatusCode != http.StatusOK {
		t.Fatalf("PUT aturan bobot berubah: status = %d (body: %s)", res.StatusCode, body)
	}
	if n := hitungAudit(t, env, "update", "routing_rule"); n != 1 {
		t.Errorf("PUT bobot berubah: audit update = %d, mau 1", n)
	}
	if w := bobot(); w[bawah] != 9 {
		t.Errorf("bobot sesudah PUT berubah = %v, mau %s=9", w, bawah)
	}

	// B-468 kaki field: strategi yang sama persis lewat PUT polos
	// juga bukan perubahan (dulu: pointer disebut = audit).
	res, body = c.do(http.MethodPut, "/api/admin/gateway/routing-rules/"+rule.ID,
		map[string]any{"strategy": "weighted"}, true)
	if res.StatusCode != http.StatusOK {
		t.Fatalf("PUT strategi sama: status = %d (body: %s)", res.StatusCode, body)
	}
	if n := hitungAudit(t, env, "update", "routing_rule"); n != 1 {
		t.Errorf("PUT strategi nilai-sama menaikkan audit update ke %d — audit palsu", n)
	}
}

// TestB470WebhookURLKosong mengunci B-470: URL kosong pada update
// webhook adalah penolakan klien 400 blocked_url, bukan 500.
func TestB470WebhookURLKosong(t *testing.T) {
	env := setupTestEnv(t)
	c := env.newClient(t)
	c.login("superadmin@routex.internal", testPassword)

	res, body := c.do(http.MethodPost, "/api/admin/automation/webhooks",
		map[string]any{"name": "wh-b470", "url": "https://8.8.8.8/hook-b470", "events": []string{"request.completed"}, "secret": "whsec-b470-ujian"}, true)
	if res.StatusCode != http.StatusOK && res.StatusCode != http.StatusCreated {
		t.Fatalf("buat webhook: status = %d (body: %s)", res.StatusCode, body)
	}
	var wh struct {
		ID string `json:"id"`
	}
	if err := json.Unmarshal(body, &wh); err != nil || wh.ID == "" {
		t.Fatalf("urai webhook: %v %s", err, body)
	}

	res, body = c.do(http.MethodPut, "/api/admin/automation/webhooks/"+wh.ID,
		map[string]any{"url": ""}, true)
	if res.StatusCode != http.StatusBadRequest || !strings.Contains(string(body), "blocked_url") {
		t.Fatalf("PUT url kosong: status = %d, mau 400 blocked_url (body: %s)", res.StatusCode, body)
	}
}

// TestB471SetAllowedKunciHantu mengunci B-471: penetapan allowlist
// pada kunci yang tidak ada menjawab 404 (himpunan kosong maupun
// tidak), bukan 200 sukses bohong.
func TestB471SetAllowedKunciHantu(t *testing.T) {
	env := setupTestEnv(t)
	c := env.newClient(t)
	c.login("superadmin@routex.internal", testPassword)

	res, body := c.do(http.MethodPost, "/api/admin/access/api-keys/01900000-0000-7000-8000-00000000b471/allowed",
		map[string]any{"model_ids": []string{}, "provider_ids": []string{}}, true)
	if res.StatusCode != http.StatusNotFound {
		t.Fatalf("kunci hantu himpunan kosong: status = %d, mau 404 (body: %s)", res.StatusCode, body)
	}
	res, body = c.do(http.MethodPost, "/api/admin/access/api-keys/01900000-0000-7000-8000-00000000b471/allowed",
		map[string]any{"model_ids": []string{"model-apa-pun"}, "provider_ids": []string{}}, true)
	if res.StatusCode == http.StatusOK {
		t.Fatalf("kunci hantu himpunan tak-kosong: status = 200 sukses bohong (body: %s)", body)
	}
}

// TestB472HapusDomainTransaksional mengunci B-472 pada tiga
// keadaan: parsial lama (konfigurasi ada, url publik tidak) → 200
// dan konfigurasi benar-benar hilang; tidak ada apa pun → 404
// tanpa satu pun baris tersentuh; lengkap → 200 keduanya hilang.
func TestB472HapusDomainTransaksional(t *testing.T) {
	env := setupTestEnv(t)
	c := env.newClient(t)
	c.login("superadmin@routex.internal", testPassword)
	ctx := context.Background()
	settings := identity.NewSettings(env.pool)
	tulis := func(key, val string) {
		if _, err := settings.Put(ctx, identity.SettingWrite{Key: key, Value: json.RawMessage(val)}); err != nil {
			t.Fatal(err)
		}
	}
	ada := func(key string) bool {
		_, err := settings.Get(ctx, key)
		return err == nil
	}

	// Keadaan parsial warisan: hanya konfigurasi.
	tulis(SettingKeyDomainConfig, `{"domain":"contoh.id","email":"a@b.id","mode":"letsencrypt"}`)
	res, body := c.do(http.MethodDelete, "/api/admin/system/domain", nil, true)
	if res.StatusCode != http.StatusOK {
		t.Fatalf("hapus parsial: status = %d, mau 200 (body: %s)", res.StatusCode, body)
	}
	if ada(SettingKeyDomainConfig) {
		t.Error("konfigurasi domain masih ada sesudah hapus parsial berhasil")
	}

	// Tidak ada apa pun: 404 jujur.
	res, body = c.do(http.MethodDelete, "/api/admin/system/domain", nil, true)
	if res.StatusCode != http.StatusNotFound {
		t.Fatalf("hapus tanpa konfigurasi: status = %d, mau 404 (body: %s)", res.StatusCode, body)
	}

	// Lengkap: keduanya hilang dalam satu jawaban sukses.
	tulis(SettingKeyDomainConfig, `{"domain":"contoh.id","email":"a@b.id","mode":"letsencrypt"}`)
	tulis(SettingKeyPublicURL, `"https://contoh.id"`)
	res, body = c.do(http.MethodDelete, "/api/admin/system/domain", nil, true)
	if res.StatusCode != http.StatusOK {
		t.Fatalf("hapus lengkap: status = %d, mau 200 (body: %s)", res.StatusCode, body)
	}
	if ada(SettingKeyDomainConfig) || ada(SettingKeyPublicURL) {
		t.Error("sisa setelan domain sesudah hapus lengkap")
	}
}
