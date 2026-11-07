package admin

// fase21_prb_test.go — pengunci Fase 21 PR-B (admin residu nilai-sama
// & kejujuran). Setiap test memanggil jalur yang dinamai butirnya
// ujung-ke-ujung lewat HTTP admin sungguhan:
//
//	B-495 jsonSamaSemantik eksak: pasangan integer yang bertabrakan
//	      di float64 (9007199254740993 vs ...992) lewat putSetting
//	      DAN updateProvider.metadata — dua pemakai helper-nya.
//	B-496 sentinel SSRF ErrCredentialsInURL & ErrInvalidBaseURL
//	      lewat update webhook sungguhan → 400 blocked_url, bukan 500.
//	B-498 setPricing nilai-sama → 200 diam: tanpa baris riwayat
//	      baru, tanpa audit, (jalur: POST pricing dua kali identik).
//	B-499 UUID kapital di field tunggal: match_model_id &
//	      match_api_key_id routing, egress_pool_id kredensial,
//	      egress_pool_id provider, model_id & provider_id filter.
//	B-500 updateDomainConfig transaksional: suntik kegagalan pada
//	      Put kedua (trigger PG) → konfigurasi pertama ikut rollback.
//	B-501 PUT routing nilai-sama tidak menulis baris: updated_at
//	      dibaca SQL sebelum/sesudah dan wajib tidak bergerak;
//	      kontrak PUT {} → 400 no_changes dipertahankan.
//	B-502 match_capabilities urutan-beda = himpunan sama (routing
//	      + capabilities model sebagai permukaan saudara).

import (
	"context"
	"encoding/json"
	"net/http"
	"strings"
	"testing"

	"github.com/NexGen-X/Route-X/internal/database/repo/identity"
	upstreamrepo "github.com/NexGen-X/Route-X/internal/database/repo/upstream"
)

// TestB495JSONSamaSemantikEksak mengunci perbandingan helper-nya
// langsung: integer besar berbeda TIDAK lagi terbaca sama, format
// angka yang setara nilai tetap sama.
func TestB495JSONSamaSemantikEksak(t *testing.T) {
	kasus := []struct {
		a, b string
		sama bool
	}{
		{"9007199254740993", "9007199254740992", false},
		{`{"n":9007199254740993}`, `{"n":9007199254740992}`, false},
		{`[1,9007199254740993]`, `[1,9007199254740992]`, false},
		{"9007199254740993", "9007199254740993", true},
		{"1.50", "1.5", true},
		{"1e2", "100", true},
		{`{"a":1,"b":2}`, `{"b":2,"a":1}`, true},
		{`{"n":1.50}`, `{"n":1.5}`, true},
		{`"9007199254740993"`, "9007199254740993", false},
		{"true", "true", true},
		{"[1,2]", "[2,1]", false},
	}
	for _, k := range kasus {
		if got := jsonSamaSemantik([]byte(k.a), []byte(k.b)); got != k.sama {
			t.Errorf("jsonSamaSemantik(%s, %s) = %v, mau %v", k.a, k.b, got, k.sama)
		}
	}
}

// TestB495PutSettingIntegerBesar membuktikan pemakai pertama
// jsonSamaSemantik: mengganti nilai setting dari 9007199254740993
// ke ...992 adalah PERUBAHAN NYATA yang wajib tertulis + teraudit
// (versi float64 menjawab 200 diam tanpa menulis).
func TestB495PutSettingIntegerBesar(t *testing.T) {
	env := setupTestEnv(t)
	c := env.newClient(t)
	c.login("superadmin@routex.internal", testPassword)
	ctx := context.Background()
	url := "/api/admin/system/settings/kunci-b495"

	put := func(n string) (int, []byte) {
		res, body := c.do(http.MethodPut, url, map[string]any{
			"value":       json.RawMessage(`{"n":` + n + `}`),
			"description": "probe b495",
		}, true)
		return res.StatusCode, body
	}
	terbaca := func() string {
		t.Helper()
		ada, err := identity.NewSettings(env.pool).Get(ctx, "kunci-b495")
		if err != nil {
			t.Fatalf("baca setting: %v", err)
		}
		return string(ada.Value)
	}

	dasar := hitungAudit(t, env, "put", "setting")
	if st, body := put("9007199254740993"); st != http.StatusOK {
		t.Fatalf("PUT pertama: status = %d (body: %s)", st, body)
	}
	if n := hitungAudit(t, env, "put", "setting"); n != dasar+1 {
		t.Fatalf("PUT pertama: audit = %d, mau %d", n, dasar+1)
	}
	if st, body := put("9007199254740992"); st != http.StatusOK {
		t.Fatalf("PUT integer tetangga: status = %d (body: %s)", st, body)
	}
	if n := hitungAudit(t, env, "put", "setting"); n != dasar+2 {
		t.Errorf("PUT integer tetangga: audit = %d, mau %d — perubahan nyata ditelan no-op float64", n, dasar+2)
	}
	if v := terbaca(); !strings.Contains(v, "9007199254740992") {
		t.Errorf("nilai tersimpan = %s, mau memuat 9007199254740992", v)
	}
	// Nilai yang sama persis tetap no-op diam.
	if st, body := put("9007199254740992"); st != http.StatusOK {
		t.Fatalf("PUT nilai sama: status = %d (body: %s)", st, body)
	}
	if n := hitungAudit(t, env, "put", "setting"); n != dasar+2 {
		t.Errorf("PUT nilai sama: audit naik ke %d — audit palsu", n)
	}
}

// TestB495UpdateProviderMetadataIntegerBesar membuktikan pemakai
// kedua: metadata provider dari 9007199254740993 ke ...992 wajib
// diterima sebagai perubahan (versi float64 menjawab 400
// no_changes untuk perubahan nyata).
func TestB495UpdateProviderMetadataIntegerBesar(t *testing.T) {
	env := setupTestEnv(t)
	c := env.newClient(t)
	c.login("superadmin@routex.internal", testPassword)
	providerID, _, _, _, _ := buatEntitasB469(t, env)
	url := "/api/admin/upstreams/providers/" + providerID

	put := func(n string) (int, []byte) {
		res, body := c.do(http.MethodPut, url, map[string]any{
			"metadata": json.RawMessage(`{"n":` + n + `}`),
		}, true)
		return res.StatusCode, body
	}

	dasar := hitungAudit(t, env, "update", "provider")
	if st, body := put("9007199254740993"); st != http.StatusOK {
		t.Fatalf("PUT metadata pertama: status = %d (body: %s)", st, body)
	}
	if st, body := put("9007199254740992"); st != http.StatusOK {
		t.Fatalf("PUT metadata integer tetangga: status = %d, mau 200 (body: %s)", st, body)
	}
	if n := hitungAudit(t, env, "update", "provider"); n != dasar+2 {
		t.Errorf("audit update/provider = %d, mau %d — perubahan metadata nyata ditelan", n, dasar+2)
	}
	if st, body := put("9007199254740992"); st != http.StatusBadRequest || !strings.Contains(string(body), "no_changes") {
		t.Errorf("PUT metadata sama persis: status = %d, mau 400 no_changes (body: %s)", st, body)
	}
}

// TestB496WebhookSentinelURL400 membuktikan dua sentinel saudara
// B-470 kini 400 blocked_url di jalur webhook (dulu 500
// internal_error), lewat update webhook sungguhan.
func TestB496WebhookSentinelURL400(t *testing.T) {
	env := setupTestEnv(t)
	c := env.newClient(t)
	c.login("superadmin@routex.internal", testPassword)

	res, body := c.do(http.MethodPost, "/api/admin/automation/webhooks",
		map[string]any{"name": "webhook-b496", "url": "https://8.8.8.8/hook-b496",
			"events": []string{"request.completed"}, "secret": "whsec-b496-ujian"}, true)
	if res.StatusCode != http.StatusOK && res.StatusCode != http.StatusCreated {
		t.Fatalf("buat webhook: status = %d (body: %s)", res.StatusCode, body)
	}
	var dibuat struct {
		ID string `json:"id"`
	}
	if err := json.Unmarshal(body, &dibuat); err != nil || dibuat.ID == "" {
		t.Fatalf("urai webhook: %v %s", err, body)
	}
	url := "/api/admin/automation/webhooks/" + dibuat.ID

	for _, k := range []struct{ nama, url string }{
		{"kredensial di URL (ErrCredentialsInURL)", "https://user:pass@8.8.8.8/hook"},
		{"tanpa host (ErrInvalidBaseURL)", "https://"},
	} {
		res, body := c.do(http.MethodPut, url, map[string]any{"url": k.url}, true)
		if res.StatusCode != http.StatusBadRequest || !strings.Contains(string(body), "blocked_url") {
			t.Errorf("%s: status = %d, mau 400 blocked_url (body: %s)", k.nama, res.StatusCode, body)
		}
	}
	// Kontrol: URL sah yang berubah tetap diterima.
	res, body = c.do(http.MethodPut, url, map[string]any{"url": "https://8.8.8.8/hook-b496-baru"}, true)
	if res.StatusCode != http.StatusOK {
		t.Errorf("kontrol URL sah: status = %d, mau 200 (body: %s)", res.StatusCode, body)
	}
}

// TestB498SetPricingNilaiSama membuktikan permukaan ke-22 keluarga
// B-469: POST harga identik kedua menjawab 200 diam — baris riwayat
// tidak bertambah dan audit tidak naik; harga berbeda tetap 201
// dengan baris + audit baru.
func TestB498SetPricingNilaiSama(t *testing.T) {
	env := setupTestEnv(t)
	c := env.newClient(t)
	c.login("superadmin@routex.internal", testPassword)
	ctx := context.Background()
	providerID, modelID, _, _, _ := buatEntitasB469(t, env)

	modelRepo := upstreamrepo.NewModelRepo(env.pool)
	pm, err := modelRepo.AttachProvider(ctx, upstreamrepo.AttachProviderParams{
		ModelID: modelID, ProviderID: providerID,
	})
	if err != nil {
		t.Fatalf("AttachProvider: %v", err)
	}
	url := "/api/admin/upstreams/models/mappings/" + pm.ID + "/pricing"
	jumlahBaris := func() int {
		t.Helper()
		var n int
		if err := env.pool.QueryRow(ctx,
			`select count(*) from model_pricing where provider_model_id = $1`, pm.ID).Scan(&n); err != nil {
			t.Fatalf("hitung baris harga: %v", err)
		}
		return n
	}

	dasar := hitungAudit(t, env, "set_pricing", "provider_model")
	res, body := c.do(http.MethodPost, url,
		map[string]any{"input_per_1m_usd": "1.5", "output_per_1m_usd": "2.5"}, true)
	if res.StatusCode != http.StatusCreated {
		t.Fatalf("POST harga pertama: status = %d (body: %s)", res.StatusCode, body)
	}
	if n := jumlahBaris(); n != 1 {
		t.Fatalf("baris harga sesudah POST pertama = %d, mau 1", n)
	}
	res, body = c.do(http.MethodPost, url,
		map[string]any{"input_per_1m_usd": "1.5", "output_per_1m_usd": "2.5"}, true)
	if res.StatusCode != http.StatusOK {
		t.Errorf("POST harga identik: status = %d, mau 200 diam (body: %s)", res.StatusCode, body)
	}
	if n := jumlahBaris(); n != 1 {
		t.Errorf("baris harga sesudah POST identik = %d, mau 1 — riwayat digandakan no-op", n)
	}
	if n := hitungAudit(t, env, "set_pricing", "provider_model"); n != dasar+1 {
		t.Errorf("audit set_pricing = %d, mau %d — audit palsu no-op", n, dasar+1)
	}
	res, body = c.do(http.MethodPost, url,
		map[string]any{"input_per_1m_usd": "3.0", "output_per_1m_usd": "2.5"}, true)
	if res.StatusCode != http.StatusCreated {
		t.Errorf("POST harga berubah: status = %d, mau 201 (body: %s)", res.StatusCode, body)
	}
	if n := jumlahBaris(); n != 2 {
		t.Errorf("baris harga sesudah POST berubah = %d, mau 2", n)
	}
	if n := hitungAudit(t, env, "set_pricing", "provider_model"); n != dasar+2 {
		t.Errorf("audit set_pricing akhir = %d, mau %d", n, dasar+2)
	}
}

// TestB499B501B502RoutingNilaiSama membuktikan tiga residu routing
// sekaligus pada handler yang sama: UUID kapital di match_model_id
// & match_api_key_id (B-499), PUT nilai-sama tidak menulis baris —
// updated_at dibaca SQL (B-501), dan match_capabilities sebagai
// himpunan (B-502). Kontrak PUT {} → 400 no_changes dipertahankan.
func TestB499B501B502RoutingNilaiSama(t *testing.T) {
	env := setupTestEnv(t)
	c := env.newClient(t)
	c.login("superadmin@routex.internal", testPassword)
	ctx := context.Background()
	_, modelID, _, _, _ := buatEntitasB469(t, env)

	res, body := c.do(http.MethodPost, "/api/admin/access/api-keys",
		map[string]any{"name": "Kunci B499"}, true)
	if res.StatusCode != http.StatusOK && res.StatusCode != http.StatusCreated {
		t.Fatalf("buat api key: status = %d (body: %s)", res.StatusCode, body)
	}
	var kunci struct {
		ID  string `json:"id"`
		Key struct {
			ID string `json:"id"`
		} `json:"key"`
	}
	if err := json.Unmarshal(body, &kunci); err != nil {
		t.Fatalf("urai api key: %v", err)
	}
	keyID := kunci.ID
	if keyID == "" {
		keyID = kunci.Key.ID
	}
	if keyID == "" {
		t.Fatalf("id api key kosong: %s", body)
	}

	res, body = c.do(http.MethodPost, "/api/admin/gateway/routing-rules",
		map[string]any{"name": "aturan-b499", "strategy": "priority"}, true)
	if res.StatusCode != http.StatusOK && res.StatusCode != http.StatusCreated {
		t.Fatalf("buat aturan: status = %d (body: %s)", res.StatusCode, body)
	}
	var aturan struct {
		ID string `json:"id"`
	}
	if err := json.Unmarshal(body, &aturan); err != nil || aturan.ID == "" {
		t.Fatalf("urai aturan: %v %s", err, body)
	}
	url := "/api/admin/gateway/routing-rules/" + aturan.ID
	bacaUpdatedAt := func() string {
		t.Helper()
		var ts string
		if err := env.pool.QueryRow(ctx,
			`select updated_at::text from routing_rules where id = $1`, aturan.ID).Scan(&ts); err != nil {
			t.Fatalf("baca updated_at: %v", err)
		}
		return ts
	}
	put := func(b map[string]any) (int, []byte) {
		res, body := c.do(http.MethodPut, url, b, true)
		return res.StatusCode, body
	}

	dasar := hitungAudit(t, env, "update", "routing_rule")
	// Perubahan nyata pertama: pasang match_model_id.
	if st, body := put(map[string]any{"match_model_id": modelID}); st != http.StatusOK {
		t.Fatalf("PUT match_model_id: status = %d (body: %s)", st, body)
	}
	u1 := bacaUpdatedAt()
	// B-499 titik 1 + B-501: ejaan kapital dari UUID yang sama.
	if st, body := put(map[string]any{"match_model_id": strings.ToUpper(modelID)}); st != http.StatusOK {
		t.Fatalf("PUT match_model_id kapital: status = %d (body: %s)", st, body)
	}
	if n := hitungAudit(t, env, "update", "routing_rule"); n != dasar+1 {
		t.Errorf("PUT match_model_id kapital: audit = %d, mau %d — audit palsu beda huruf", n, dasar+1)
	}
	if u := bacaUpdatedAt(); u != u1 {
		t.Errorf("PUT match_model_id kapital: updated_at bergerak %s → %s — baris ditulis untuk no-op (B-501)", u1, u)
	}
	// B-499 titik 2 (dari kode): match_api_key_id.
	if st, body := put(map[string]any{"match_api_key_id": keyID}); st != http.StatusOK {
		t.Fatalf("PUT match_api_key_id: status = %d (body: %s)", st, body)
	}
	u2 := bacaUpdatedAt()
	if st, body := put(map[string]any{"match_api_key_id": strings.ToUpper(keyID)}); st != http.StatusOK {
		t.Fatalf("PUT match_api_key_id kapital: status = %d (body: %s)", st, body)
	}
	if n := hitungAudit(t, env, "update", "routing_rule"); n != dasar+2 {
		t.Errorf("PUT match_api_key_id kapital: audit = %d, mau %d", n, dasar+2)
	}
	if u := bacaUpdatedAt(); u != u2 {
		t.Errorf("PUT match_api_key_id kapital: updated_at bergerak %s → %s", u2, u)
	}
	// B-502: capabilities urutan dibalik adalah himpunan yang sama.
	if st, body := put(map[string]any{"match_capabilities": []string{"vision", "tools"}}); st != http.StatusOK {
		t.Fatalf("PUT capabilities: status = %d (body: %s)", st, body)
	}
	u3 := bacaUpdatedAt()
	if st, body := put(map[string]any{"match_capabilities": []string{"tools", "vision"}}); st != http.StatusOK {
		t.Fatalf("PUT capabilities terbalik: status = %d (body: %s)", st, body)
	}
	if n := hitungAudit(t, env, "update", "routing_rule"); n != dasar+3 {
		t.Errorf("PUT capabilities terbalik: audit = %d, mau %d — urutan himpunan terbaca perubahan", n, dasar+3)
	}
	if u := bacaUpdatedAt(); u != u3 {
		t.Errorf("PUT capabilities terbalik: updated_at bergerak %s → %s", u3, u)
	}
	// B-501 murni: nama yang sama persis tidak menulis baris.
	if st, body := put(map[string]any{"name": "aturan-b499"}); st != http.StatusOK {
		t.Fatalf("PUT nama sama: status = %d (body: %s)", st, body)
	}
	if u := bacaUpdatedAt(); u != u3 {
		t.Errorf("PUT nama sama: updated_at bergerak %s → %s — B-501 belum tertutup", u3, u)
	}
	// Kontrak lama dipertahankan: PUT {} tetap 400 no_changes.
	if st, body := put(map[string]any{}); st != http.StatusBadRequest || !strings.Contains(string(body), "no_changes") {
		t.Errorf("PUT {}: status = %d, mau 400 no_changes (body: %s)", st, body)
	}
	// Perubahan nyata tetap menulis + teraudit.
	if st, body := put(map[string]any{"name": "aturan-b499-baru"}); st != http.StatusOK {
		t.Fatalf("PUT nama baru: status = %d (body: %s)", st, body)
	}
	if n := hitungAudit(t, env, "update", "routing_rule"); n != dasar+4 {
		t.Errorf("PUT nama baru: audit = %d, mau %d", n, dasar+4)
	}
	if u := bacaUpdatedAt(); u == u3 {
		t.Errorf("PUT nama baru: updated_at tidak bergerak — perubahan nyata tidak tertulis")
	}
}

// TestB502ModelCapabilitiesHimpunan membuktikan permukaan saudara
// B-502 di updateModel: capabilities model urutan-beda adalah
// nilai yang sama → 400 no_changes, bukan 200 + audit.
func TestB502ModelCapabilitiesHimpunan(t *testing.T) {
	env := setupTestEnv(t)
	c := env.newClient(t)
	c.login("superadmin@routex.internal", testPassword)
	_, modelID, _, _, _ := buatEntitasB469(t, env)
	url := "/api/admin/upstreams/models/" + modelID

	res, body := c.do(http.MethodPut, url,
		map[string]any{"capabilities": []string{"vision", "tools"}}, true)
	if res.StatusCode != http.StatusOK {
		t.Fatalf("PUT capabilities pertama: status = %d (body: %s)", res.StatusCode, body)
	}
	dasar := hitungAudit(t, env, "update", "model")
	res, body = c.do(http.MethodPut, url,
		map[string]any{"capabilities": []string{"tools", "vision"}}, true)
	if res.StatusCode != http.StatusBadRequest || !strings.Contains(string(body), "no_changes") {
		t.Errorf("PUT capabilities terbalik: status = %d, mau 400 no_changes (body: %s)", res.StatusCode, body)
	}
	if n := hitungAudit(t, env, "update", "model"); n != dasar {
		t.Errorf("PUT capabilities terbalik: audit naik ke %d — audit palsu urutan himpunan", n)
	}
}

// TestB499PermukaanUUIDLain membuktikan sisa titik B-499:
// egress_pool_id kredensial (PUT kapital → 200 diam tanpa audit),
// egress_pool_id provider (PUT kapital → 400 no_changes), dan
// model_id + provider_id filter konten (PUT kapital → 200 diam).
func TestB499PermukaanUUIDLain(t *testing.T) {
	env := setupTestEnv(t)
	c := env.newClient(t)
	c.login("superadmin@routex.internal", testPassword)
	providerID, modelID, poolID, credID, _ := buatEntitasB469(t, env)

	// --- egress_pool_id kredensial ---
	urlPool := "/api/admin/upstreams/providers/" + providerID + "/credentials/" + credID + "/egress-pool"
	dasar := hitungAudit(t, env, "update_egress_pool", "credential")
	res, body := c.do(http.MethodPut, urlPool, map[string]any{"egress_pool_id": poolID}, true)
	if res.StatusCode != http.StatusOK {
		t.Fatalf("ikat pool: status = %d (body: %s)", res.StatusCode, body)
	}
	res, body = c.do(http.MethodPut, urlPool,
		map[string]any{"egress_pool_id": strings.ToUpper(poolID)}, true)
	if res.StatusCode != http.StatusOK {
		t.Fatalf("ikat pool kapital: status = %d (body: %s)", res.StatusCode, body)
	}
	if n := hitungAudit(t, env, "update_egress_pool", "credential"); n != dasar+1 {
		t.Errorf("ikat pool kapital: audit = %d, mau %d — no-op beda huruf teraudit (+ siraman dari kode)", n, dasar+1)
	}

	// --- egress_pool_id provider ---
	urlProv := "/api/admin/upstreams/providers/" + providerID
	dasar = hitungAudit(t, env, "update", "provider")
	res, body = c.do(http.MethodPut, urlProv, map[string]any{"egress_pool_id": poolID}, true)
	if res.StatusCode != http.StatusOK {
		t.Fatalf("PUT egress provider: status = %d (body: %s)", res.StatusCode, body)
	}
	res, body = c.do(http.MethodPut, urlProv,
		map[string]any{"egress_pool_id": strings.ToUpper(poolID)}, true)
	if res.StatusCode != http.StatusBadRequest || !strings.Contains(string(body), "no_changes") {
		t.Errorf("PUT egress provider kapital: status = %d, mau 400 no_changes (body: %s)", res.StatusCode, body)
	}
	if n := hitungAudit(t, env, "update", "provider"); n != dasar+1 {
		t.Errorf("PUT egress provider kapital: audit = %d, mau %d", n, dasar+1)
	}

	// --- model_id & provider_id filter konten ---
	res, body = c.do(http.MethodPost, "/api/admin/gateway/content-filters",
		map[string]any{"name": "filter-b499", "kind": "blocked_pattern", "pattern": "larangan-b499",
			"pattern_type": "substring", "action": "block"}, true)
	if res.StatusCode != http.StatusOK && res.StatusCode != http.StatusCreated {
		t.Fatalf("buat filter: status = %d (body: %s)", res.StatusCode, body)
	}
	var filter struct {
		ID string `json:"id"`
	}
	if err := json.Unmarshal(body, &filter); err != nil || filter.ID == "" {
		t.Fatalf("urai filter: %v %s", err, body)
	}
	urlFilter := "/api/admin/gateway/content-filters/" + filter.ID
	dasar = hitungAudit(t, env, "update", "content_filter")
	putFilter := func(b map[string]any) (int, []byte) {
		res, body := c.do(http.MethodPut, urlFilter, b, true)
		return res.StatusCode, body
	}
	if st, body := putFilter(map[string]any{"model_id": modelID}); st != http.StatusOK {
		t.Fatalf("PUT model_id filter: status = %d (body: %s)", st, body)
	}
	if st, body := putFilter(map[string]any{"model_id": strings.ToUpper(modelID)}); st != http.StatusOK {
		t.Fatalf("PUT model_id filter kapital: status = %d (body: %s)", st, body)
	}
	if n := hitungAudit(t, env, "update", "content_filter"); n != dasar+1 {
		t.Errorf("PUT model_id filter kapital: audit = %d, mau %d", n, dasar+1)
	}
	if st, body := putFilter(map[string]any{"provider_id": providerID}); st != http.StatusOK {
		t.Fatalf("PUT provider_id filter: status = %d (body: %s)", st, body)
	}
	if st, body := putFilter(map[string]any{"provider_id": strings.ToUpper(providerID)}); st != http.StatusOK {
		t.Fatalf("PUT provider_id filter kapital: status = %d (body: %s)", st, body)
	}
	if n := hitungAudit(t, env, "update", "content_filter"); n != dasar+2 {
		t.Errorf("PUT provider_id filter kapital: audit = %d, mau %d", n, dasar+2)
	}
}

// TestB500UpdateDomainConfigTransaksional membuktikan B-500 lewat
// handler sungguhan: trigger PG menggagalkan Put untuk kunci
// system:public_url (Put KEDUA); konfigurasi domain (Put pertama)
// WAJIB ikut rollback — tidak ada baris system:domain:config yang
// ter-commit dari permintaan yang gagal. Kontrol tanpa trigger:
// POST yang sama berhasil dan kedua kunci tertulis.
func TestB500UpdateDomainConfigTransaksional(t *testing.T) {
	env := setupTestEnv(t)
	c := env.newClient(t)
	c.login("superadmin@routex.internal", testPassword)
	ctx := context.Background()

	bersihkan := func() {
		_, _ = env.pool.Exec(ctx, `drop trigger if exists trg_b500 on settings`)
		_, _ = env.pool.Exec(ctx, `drop function if exists gagal_put_public_url_b500()`)
		_, _ = env.pool.Exec(ctx, `delete from settings where key in ($1, $2)`,
			SettingKeyDomainConfig, SettingKeyPublicURL)
	}
	bersihkan()
	defer bersihkan()

	if _, err := env.pool.Exec(ctx, `
		create function gagal_put_public_url_b500() returns trigger as $$
		begin
			if NEW.key = 'system:public_url' then
				raise exception 'suntikan gagal b500';
			end if;
			return NEW;
		end;
		$$ language plpgsql`); err != nil {
		t.Fatalf("buat fungsi suntik: %v", err)
	}
	if _, err := env.pool.Exec(ctx, `
		create trigger trg_b500 before insert or update on settings
		for each row execute function gagal_put_public_url_b500()`); err != nil {
		t.Fatalf("buat trigger suntik: %v", err)
	}

	res, body := c.do(http.MethodPost, "/api/admin/system/domain",
		map[string]any{"domain": "example.com", "mode": "cloudflare"}, true)
	if res.StatusCode == http.StatusOK {
		t.Fatalf("POST domain dengan Put kedua gagal: status = 200, mau galat (body: %s)", body)
	}
	var n int
	if err := env.pool.QueryRow(ctx,
		`select count(*) from settings where key = $1`, SettingKeyDomainConfig).Scan(&n); err != nil {
		t.Fatalf("hitung baris domain config: %v", err)
	}
	if n != 0 {
		t.Errorf("baris %s ter-commit dari permintaan gagal = %d, mau 0 — update tidak transaksional", SettingKeyDomainConfig, n)
	}

	// Kontrol: tanpa suntikan, POST yang sama berhasil utuh.
	bersihkan()
	res, body = c.do(http.MethodPost, "/api/admin/system/domain",
		map[string]any{"domain": "example.com", "mode": "cloudflare"}, true)
	if res.StatusCode != http.StatusOK {
		t.Fatalf("kontrol POST domain: status = %d (body: %s)", res.StatusCode, body)
	}
	for _, kunci := range []string{SettingKeyDomainConfig, SettingKeyPublicURL} {
		if _, err := identity.NewSettings(env.pool).Get(ctx, kunci); err != nil {
			t.Errorf("kontrol: kunci %s tidak tertulis: %v", kunci, err)
		}
	}
}
