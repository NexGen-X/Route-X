package admin

import (
	"context"
	"encoding/json"
	"net/http"
	"strings"
	"testing"
	"time"

	"github.com/NexGen-X/Route-X/internal/database/repo/upstream"
	"github.com/NexGen-X/Route-X/internal/security"
)

// TestB398IkatPoolKredensialBedakanAbsenDariLepas adalah pengunci
// B-398: setCredentialEgressPool melebur absen/null/"" menjadi
// perintah melepas ikatan yang sama, jadi PUT {} ikut MEMUTUS
// ikatan pool kredensial dan dijawab 200. Kini absen = 400
// no_changes dengan ikatan utuh; null/"" eksplisit = lepas.
func TestB398IkatPoolKredensialBedakanAbsenDariLepas(t *testing.T) {
	env := setupTestEnv(t)
	ctx := context.Background()

	providerRepo := upstream.NewProviderRepo(env.pool)
	credentialRepo, err := upstream.NewCredentialRepo(env.pool, env.cipher)
	if err != nil {
		t.Fatalf("NewCredentialRepo: %v", err)
	}
	egressRepo, err := upstream.NewEgressRepo(env.pool, env.cipher)
	if err != nil {
		t.Fatalf("NewEgressRepo: %v", err)
	}
	prov, err := providerRepo.Create(ctx, upstream.CreateProviderParams{
		Name: "prov-b398", DisplayName: "Provider B-398", Kind: "openai", BaseURL: "https://api.openai.com",
	})
	if err != nil {
		t.Fatalf("buat provider: %v", err)
	}
	cred, err := credentialRepo.Create(ctx, upstream.CreateCredentialParams{
		ProviderID: prov.ID, Label: "key-b398", Secret: security.Secret("GANTI_kunci_uji_b398"),
	})
	if err != nil {
		t.Fatalf("buat kredensial: %v", err)
	}
	aktif := true
	pool, err := egressRepo.Create(ctx, upstream.CreateEgressParams{
		Name: "pool-b398", Kind: upstream.EgressHTTP,
		ProxyURL: security.Secret("http://127.0.0.1:18080"), Enabled: &aktif,
	})
	if err != nil {
		t.Fatalf("buat pool egress: %v", err)
	}

	c := env.newClient(t)
	c.login("superadmin@routex.internal", testPassword)
	url := "/api/admin/upstreams/providers/" + prov.ID + "/credentials/" + cred.ID + "/egress-pool"

	ikatan := func() *string {
		t.Helper()
		kini, err := credentialRepo.Get(ctx, cred.ID)
		if err != nil {
			t.Fatalf("baca kredensial: %v", err)
		}
		return kini.EgressPoolID
	}

	// Ikat dulu dengan nilai eksplisit.
	res, body := c.do(http.MethodPut, url, map[string]any{"egress_pool_id": pool.ID}, true)
	if res.StatusCode != http.StatusOK {
		t.Fatalf("ikat pool: status = %d, mau 200 (body: %s)", res.StatusCode, body)
	}
	if got := ikatan(); got == nil || *got != pool.ID {
		t.Fatalf("ikatan sesudah PUT nilai = %v, mau %s", got, pool.ID)
	}

	// PUT {} → 400 no_changes DAN ikatan tidak berubah.
	res, body = c.do(http.MethodPut, url, map[string]any{}, true)
	if res.StatusCode != http.StatusBadRequest {
		t.Fatalf("PUT {}: status = %d, mau 400 (body: %s)", res.StatusCode, body)
	}
	if !strings.Contains(string(body), "no_changes") {
		t.Errorf("PUT {} body = %s, mau memuat kode no_changes", body)
	}
	if got := ikatan(); got == nil || *got != pool.ID {
		t.Errorf("ikatan sesudah PUT {} = %v, mau tetap %s — body kosong melepas ikatan", got, pool.ID)
	}

	// null eksplisit tetap berarti lepas ikatan.
	res, body = c.do(http.MethodPut, url, map[string]any{"egress_pool_id": nil}, true)
	if res.StatusCode != http.StatusOK {
		t.Fatalf("PUT null: status = %d, mau 200 (body: %s)", res.StatusCode, body)
	}
	if got := ikatan(); got != nil {
		t.Errorf("ikatan sesudah PUT null = %v, mau nil (terlepas)", got)
	}

	// "" eksplisit juga melepas (sesudah diikat ulang).
	res, body = c.do(http.MethodPut, url, map[string]any{"egress_pool_id": pool.ID}, true)
	if res.StatusCode != http.StatusOK {
		t.Fatalf("ikat ulang: status = %d, mau 200 (body: %s)", res.StatusCode, body)
	}
	res, body = c.do(http.MethodPut, url, map[string]any{"egress_pool_id": ""}, true)
	if res.StatusCode != http.StatusOK {
		t.Fatalf("PUT \"\": status = %d, mau 200 (body: %s)", res.StatusCode, body)
	}
	if got := ikatan(); got != nil {
		t.Errorf("ikatan sesudah PUT \"\" = %v, mau nil (terlepas)", got)
	}
}

// TestB399CacheParsialTidakMematikan adalah pengunci B-399:
// updateCacheSettings menerapkan bool/int64 polos tanpa syarat —
// POST {"ttl_seconds":600} ikut mematikan cache dan POST {}
// menonaktifkannya dengan TTL 0. Kini field tak-disebut diwarisi
// dari setelan engine yang berlaku, dan body kosong 400.
func TestB399CacheParsialTidakMematikan(t *testing.T) {
	mesin := mesinCacheUntukAdmin(t)
	mesin.SetSettings(true, 30*time.Minute)
	h := NewHandlers(Config{ResponseCache: mesin})

	// POST TTL saja: aktif diwarisi, TTL berubah.
	w := panggilUpdateCache(t, h, `{"ttl_seconds":600}`)
	if w.Code != http.StatusOK {
		t.Fatalf("POST ttl-saja: status = %d, mau 200: %s", w.Code, w.Body.String())
	}
	if !mesin.IsEnabled() {
		t.Error("cache mati sesudah POST ttl-saja — enabled false karangan dari field absen")
	}
	if mesin.TTL() != 10*time.Minute {
		t.Errorf("TTL = %v, mau 10 menit", mesin.TTL())
	}
	var dto CacheSettingsResponseDTO
	if err := json.Unmarshal(w.Body.Bytes(), &dto); err != nil || !dto.Enabled || dto.TTLSeconds != 600 {
		t.Errorf("DTO = %+v (err %v), mau enabled=true ttl=600 gabungan", dto, err)
	}

	// POST enabled saja: TTL diwarisi.
	w = panggilUpdateCache(t, h, `{"enabled":false}`)
	if w.Code != http.StatusOK {
		t.Fatalf("POST enabled-saja: status = %d, mau 200: %s", w.Code, w.Body.String())
	}
	if mesin.IsEnabled() {
		t.Error("nonaktifkan eksplisit tidak berlaku")
	}
	if mesin.TTL() != 10*time.Minute {
		t.Errorf("TTL sesudah POST enabled-saja = %v, mau tetap 10 menit", mesin.TTL())
	}

	// POST {} → 400 no_changes, keadaan tidak berubah.
	w = panggilUpdateCache(t, h, `{}`)
	if w.Code != http.StatusBadRequest {
		t.Fatalf("POST {}: status = %d, mau 400: %s", w.Code, w.Body.String())
	}
	if !strings.Contains(w.Body.String(), "no_changes") {
		t.Errorf("POST {} body = %s, mau memuat kode no_changes", w.Body.String())
	}
	if mesin.IsEnabled() || mesin.TTL() != 10*time.Minute {
		t.Errorf("keadaan berubah sesudah POST {}: enabled=%v ttl=%v", mesin.IsEnabled(), mesin.TTL())
	}

	// TTL gabungan tetap divalidasi rentangnya.
	w = panggilUpdateCache(t, h, `{"ttl_seconds":999999}`)
	if w.Code != http.StatusBadRequest || !strings.Contains(w.Body.String(), "invalid_ttl") {
		t.Errorf("POST ttl raksasa: status = %d, mau 400 invalid_ttl: %s", w.Code, w.Body.String())
	}
}

// TestB400UpdateUserKosongDitolakTanpaAudit adalah pengunci
// B-400: PUT {} pada pengguna sebelumnya menjawab 200 dan tetap
// menulis baris audit "update" — jejak perubahan yang tidak
// pernah terjadi. Kini 400 no_changes tanpa audit; update nyata
// tetap 200 dan teraudit.
func TestB400UpdateUserKosongDitolakTanpaAudit(t *testing.T) {
	env := setupTestEnv(t)
	ctx := context.Background()
	c := env.newClient(t)
	c.login("superadmin@routex.internal", testPassword)

	res, body := c.do(http.MethodPost, "/api/admin/access/users", map[string]any{
		"email": "target-b400@routex.test", "password": "GANTI_sandi_uji_b400", "name": "Target B400",
	}, true)
	if res.StatusCode != http.StatusCreated {
		t.Fatalf("buat user: status = %d, mau 201 (body: %s)", res.StatusCode, body)
	}
	var dibuat struct {
		ID string `json:"id"`
	}
	if err := json.Unmarshal(body, &dibuat); err != nil || dibuat.ID == "" {
		t.Fatalf("urai id user: %v (body: %s)", err, body)
	}
	url := "/api/admin/access/users/" + dibuat.ID

	jumlahAuditUpdate := func() int {
		t.Helper()
		var n int
		err := env.pool.QueryRow(ctx,
			`select count(*) from audit_logs where resource_type = 'user' and resource_id = $1 and action = 'update'`,
			dibuat.ID).Scan(&n)
		if err != nil {
			t.Fatalf("hitung audit: %v", err)
		}
		return n
	}

	res, body = c.do(http.MethodPut, url, map[string]any{}, true)
	if res.StatusCode != http.StatusBadRequest {
		t.Fatalf("PUT {}: status = %d, mau 400 (body: %s)", res.StatusCode, body)
	}
	if !strings.Contains(string(body), "no_changes") {
		t.Errorf("PUT {} body = %s, mau memuat kode no_changes", body)
	}
	if n := jumlahAuditUpdate(); n != 0 {
		t.Errorf("baris audit update = %d sesudah PUT {}, mau 0 — audit palsu untuk perubahan yang tidak terjadi", n)
	}

	// Kontrol: update nyata tetap bekerja dan teraudit.
	res, body = c.do(http.MethodPut, url, map[string]any{"name": "Target B400 Baru"}, true)
	if res.StatusCode != http.StatusOK {
		t.Fatalf("PUT nama: status = %d, mau 200 (body: %s)", res.StatusCode, body)
	}
	if n := jumlahAuditUpdate(); n != 1 {
		t.Errorf("baris audit update = %d sesudah PUT nama, mau 1", n)
	}
}

// TestB401RoutingRuleProviderIDsSaja adalah pengunci B-401:
// updateRoutingRule menjalankan Update field lebih dulu di dalam
// transaksi; body berisi provider_ids SAJA menghasilkan params
// kosong → ErrNoChanges repo membatalkan transaksi → 400 palsu,
// padahal SetProviders-nya sah. Kini sentinel itu ditelan hanya
// bila provider_ids disebut; PUT {} tetap 400 no_changes.
func TestB401RoutingRuleProviderIDsSaja(t *testing.T) {
	env := setupTestEnv(t)
	ctx := context.Background()

	providerRepo := upstream.NewProviderRepo(env.pool)
	prov, err := providerRepo.Create(ctx, upstream.CreateProviderParams{
		Name: "prov-b401", DisplayName: "Provider B-401", Kind: "openai", BaseURL: "https://api.openai.com",
	})
	if err != nil {
		t.Fatalf("buat provider: %v", err)
	}

	c := env.newClient(t)
	c.login("superadmin@routex.internal", testPassword)

	res, body := c.do(http.MethodPost, "/api/admin/gateway/routing-rules", map[string]any{
		"name": "Aturan B401", "strategy": "priority",
	}, true)
	if res.StatusCode != http.StatusCreated && res.StatusCode != http.StatusOK {
		t.Fatalf("buat aturan: status = %d (body: %s)", res.StatusCode, body)
	}
	var aturan struct {
		ID string `json:"id"`
	}
	if err := json.Unmarshal(body, &aturan); err != nil || aturan.ID == "" {
		t.Fatalf("urai id aturan: %v (body: %s)", err, body)
	}
	url := "/api/admin/gateway/routing-rules/" + aturan.ID

	// PUT provider_ids saja → 200 dan ikatan terpasang.
	res, body = c.do(http.MethodPut, url, map[string]any{
		"provider_ids": []string{prov.ID},
	}, true)
	if res.StatusCode != http.StatusOK {
		t.Fatalf("PUT provider_ids-saja: status = %d, mau 200 (body: %s)", res.StatusCode, body)
	}
	if !strings.Contains(string(body), prov.ID) {
		t.Errorf("respons PUT = %s, mau memuat provider %s terikat", body, prov.ID)
	}

	// Kontrol: PUT {} tetap 400 no_changes (sentinel tidak ditelan
	// tanpa provider_ids).
	res, body = c.do(http.MethodPut, url, map[string]any{}, true)
	if res.StatusCode != http.StatusBadRequest {
		t.Fatalf("PUT {}: status = %d, mau 400 (body: %s)", res.StatusCode, body)
	}
	if !strings.Contains(string(body), "no_changes") {
		t.Errorf("PUT {} body = %s, mau memuat kode no_changes", body)
	}
}
