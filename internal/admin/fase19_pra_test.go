package admin

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/NexGen-X/Route-X/internal/database/repo/identity"
	"github.com/NexGen-X/Route-X/internal/database/repo/upstream"
	"github.com/NexGen-X/Route-X/internal/oauth"
	"github.com/NexGen-X/Route-X/internal/responsecache"
	"github.com/NexGen-X/Route-X/internal/security"
)

// Test Fase 19 PR-A: keluarga flush (B-436/B-437/B-438), deteksi
// no-op himpunan-penuh (B-434), updateUser nilai-sama (B-433), dan
// penjaga kunci setelan khusus pada putSetting generik (B-457).
// Setiap test diskriminatif: gagal di kode basis, lulus sesudah fix.

// tanamEntriLaluNonaktifkan menanam satu entri cache saat engine
// aktif lalu mematikan engine — keadaan persis jendela cache-mati
// yang menjadi akar B-436: entri tetap tersimpan di Redis dan akan
// tersaji lagi begitu engine diaktifkan kembali, kecuali ada
// siraman yang berjalan TANPA syarat enabled.
func tanamEntriLaluNonaktifkan(t *testing.T, mesin *responsecache.Engine, kunci string) {
	t.Helper()
	ctx := context.Background()
	mesin.SetSettings(true, time.Minute)
	if err := mesin.Set(ctx, kunci, &responsecache.Entry{
		Model: "model-uji", Raw: json.RawMessage(`{"id":"chatcmpl-fase19"}`),
	}); err != nil {
		t.Fatalf("tanam entri cache: %v", err)
	}
	mesin.SetSettings(false, time.Minute)
}

// entriMasihHidup mengaktifkan kembali engine dan melaporkan
// apakah entri dengan kunci itu masih tersaji.
func entriMasihHidup(t *testing.T, mesin *responsecache.Engine, kunci string) bool {
	t.Helper()
	mesin.SetSettings(true, time.Minute)
	_, hit, err := mesin.Get(context.Background(), kunci)
	if err != nil {
		t.Fatalf("baca entri cache: %v", err)
	}
	return hit
}

// TestB436FlushBerjalanSaatCacheNonaktif adalah pengunci B-436
// sisi admin: kosongkanCacheRespons dulu NO-OP selama cache
// nonaktif (gerbang IsEnabled), sehingga mutasi di jendela
// cache-mati tidak pernah menyiram dan entri basi hidup lagi saat
// re-aktivasi. Kini siraman berjalan tanpa syarat enabled.
func TestB436FlushBerjalanSaatCacheNonaktif(t *testing.T) {
	mesin := mesinCacheUntukAdmin(t)
	h := NewHandlers(Config{ResponseCache: mesin})

	tanamEntriLaluNonaktifkan(t, mesin, "kunci-b436")
	h.kosongkanCacheRespons(context.Background())

	if entriMasihHidup(t, mesin, "kunci-b436") {
		t.Error("entri cache masih hidup sesudah kosongkanCacheRespons pada engine nonaktif — flush masih tergerbang enabled (B-436)")
	}
}

// TestB437RestoreMenyiramCacheRespons adalah pengunci B-437:
// restoreDatabaseSQL menukar SELURUH database (provider,
// kredensial, sesi OAuth, pemetaan) tetapi dulu tidak menyiram
// cache respons — entri yang dihitung dari konteks pra-restore
// tersaji lagi sesudahnya. Kini restore sukses menyiram total.
func TestB437RestoreMenyiramCacheRespons(t *testing.T) {
	env := siapkanLingkunganRestore(t)
	mesin := mesinCacheUntukAdmin(t)
	env.handler.responseCache = mesin

	env.tanamPenanda("AWAL")

	req := httptest.NewRequest(http.MethodPost, "/api/admin/system/backup/export?format=sql", nil)
	recExp := httptest.NewRecorder()
	env.handler.exportDatabaseSQL(recExp, req)
	if recExp.Code != http.StatusOK {
		t.Fatalf("ekspor status = %d. body: %s", recExp.Code, recExp.Body.String())
	}
	dump := recExp.Body.Bytes()

	tanamEntriLaluNonaktifkan(t, mesin, "kunci-b437")

	rec := env.panggilRestore("routex_backup.sql", dump)
	if rec.Code != http.StatusOK {
		t.Fatalf("restore status = %d. body: %s", rec.Code, rec.Body.String())
	}

	if entriMasihHidup(t, mesin, "kunci-b437") {
		t.Error("entri cache pra-restore masih tersaji sesudah restore sukses — restore tidak menyiram cache respons (B-437)")
	}
}

// TestB438SyncBackfillProjectMenyiramCache adalah pengunci B-438
// sisi admin: projectCodeAssistSync menyimpan project Code Assist
// hasil backfill ke sesi (konteks layan yang tidak masuk kunci
// cache) tanpa flush, sehingga jawaban yang ter-cache untuk sesi
// tanpa project tetap tersaji. Kini penyimpanan yang berhasil
// diikuti siraman.
func TestB438SyncBackfillProjectMenyiramCache(t *testing.T) {
	env := setupTestEnv(t)
	ctx := context.Background()

	mesin := mesinCacheUntukAdmin(t)
	env.handlers.responseCache = mesin

	provRepo := upstream.NewProviderRepo(env.pool)
	prov, err := provRepo.Create(ctx, upstream.CreateProviderParams{
		Name: "prov-b438", DisplayName: "Provider B-438", Kind: "google",
		BaseURL: "https://generativelanguage.googleapis.com",
	})
	if err != nil {
		t.Fatalf("buat provider: %v", err)
	}
	credRepo, err := upstream.NewCredentialRepo(env.pool, env.cipher)
	if err != nil {
		t.Fatalf("NewCredentialRepo: %v", err)
	}
	awalan := "GANTI_"
	rahasiaKredensial := security.Secret(awalan + "kunci_akses_b438")
	parameterKred := upstream.CreateCredentialParams{
		ProviderID: prov.ID, Label: "oauth:b438@contoh.test",
	}
	parameterKred.Secret = rahasiaKredensial
	cred, err := credRepo.Create(ctx, parameterKred)
	if err != nil {
		t.Fatalf("buat kredensial: %v", err)
	}
	oauthRepo, err := upstream.NewOAuthRepo(env.pool, env.cipher)
	if err != nil {
		t.Fatalf("NewOAuthRepo: %v", err)
	}
	env.handlers.oauthRepo = oauthRepo
	if _, err := oauthRepo.UpsertSession(ctx, upstream.UpsertOAuthSessionParams{
		ProviderID: prov.ID, CredentialID: cred.ID,
		AccountEmail: "b438@contoh.test", ClientID: "client-b438",
		RefreshToken: security.Secret("GANTI_refresh_token_b438"),
	}); err != nil {
		t.Fatalf("buat sesi: %v", err)
	}

	caSrv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{"cloudaicompanionProject":"proj-b438"}`))
	}))
	defer caSrv.Close()
	env.handlers.oauthClient = oauth.NewGoogleOAuthClient(nil).WithCodeAssistEndpoint(caSrv.URL)

	tanamEntriLaluNonaktifkan(t, mesin, "kunci-b438")

	didapat := env.handlers.projectCodeAssistSync(ctx, prov,
		&upstream.ActiveCredential{ID: cred.ID}, "GANTI_token_akses_b438")
	if didapat != "proj-b438" {
		t.Fatalf("projectCodeAssistSync = %q, mau proj-b438", didapat)
	}
	meta, err := oauthRepo.GetSessionByCredentialID(ctx, cred.ID)
	if err != nil {
		t.Fatalf("muat sesi: %v", err)
	}
	if meta.CodeAssistProject == nil || *meta.CodeAssistProject != "proj-b438" {
		t.Fatalf("project sesi = %v, mau proj-b438 (prasyarat backfill tidak terpenuhi)", meta.CodeAssistProject)
	}

	if entriMasihHidup(t, mesin, "kunci-b438") {
		t.Error("entri cache masih tersaji sesudah backfill project via sync — jalur sync tidak menyiram (B-438)")
	}
}

// TestB457PutSettingGenerikTidakBisaMenulisKunciKhusus adalah
// pembuktian sekaligus pengunci B-457: putSetting generik dulu
// hanya menjaga awalan cli:config:, sehingga kunci milik handler
// khusus — system:response_cache (updateCacheSettings) dan
// system:domain:config / system:public_url (updateDomainConfig) —
// bisa ditulis mentah: baris persisten berubah tetapi engine /
// keadaan berjalan tidak menerapkannya sampai reboot. Kini
// ketiganya reserved_setting untuk PUT maupun DELETE generik.
func TestB457PutSettingGenerikTidakBisaMenulisKunciKhusus(t *testing.T) {
	env := setupTestEnv(t)
	admin := env.newClient(t)
	admin.login(env.adminUser.Email, testPassword)
	ctx := context.Background()
	repoSet := env.handlers.settingsRepo

	if _, err := repoSet.Put(ctx, identity.SettingWrite{
		Key:   "system:response_cache",
		Value: json.RawMessage(`{"enabled":false,"ttl_seconds":60}`),
	}); err != nil {
		t.Fatalf("tanam setelan cache: %v", err)
	}

	res, raw := admin.do(http.MethodPut, "/api/admin/system/settings/system:response_cache",
		map[string]any{"value": json.RawMessage(`{"enabled":true,"ttl_seconds":120}`)}, true)
	if res.StatusCode != http.StatusBadRequest {
		t.Fatalf("PUT generik system:response_cache status = %d, mau 400 (body: %s)", res.StatusCode, raw)
	}
	if !mengandungKode(raw, "reserved_setting") {
		t.Errorf("kode error = %s, mau reserved_setting", raw)
	}
	tersimpan, err := repoSet.Get(ctx, "system:response_cache")
	if err != nil {
		t.Fatalf("baca setelan cache: %v", err)
	}
	if !strings.Contains(string(tersimpan.Value), `"ttl_seconds": 60`) {
		t.Errorf("nilai system:response_cache berubah menjadi %s padahal PUT generik ditolak — putSetting melewati updateCacheSettings (B-457)", tersimpan.Value)
	}

	// Kunci domain juga milik handler khusus: DELETE generik
	// tidak boleh menghapusnya dari bawah updateDomainConfig.
	if _, err := repoSet.Put(ctx, identity.SettingWrite{
		Key:   "system:domain:config",
		Value: json.RawMessage(`{"mode":"single"}`),
	}); err != nil {
		t.Fatalf("tanam setelan domain: %v", err)
	}
	res, raw = admin.do(http.MethodDelete, "/api/admin/system/settings/system:domain:config", nil, true)
	if res.StatusCode != http.StatusBadRequest {
		t.Fatalf("DELETE generik system:domain:config status = %d, mau 400 (body: %s)", res.StatusCode, raw)
	}
	if !mengandungKode(raw, "reserved_setting") {
		t.Errorf("kode error DELETE = %s, mau reserved_setting", raw)
	}
	if _, err := repoSet.Get(ctx, "system:domain:config"); err != nil {
		t.Errorf("baris system:domain:config hilang padahal DELETE ditolak: %v", err)
	}

	// Kontrol: kunci biasa tetap bisa ditulis lewat jalur generik.
	res, raw = admin.do(http.MethodPut, "/api/admin/system/settings/b457.kunci.biasa",
		map[string]any{"value": json.RawMessage(`"halo"`)}, true)
	if res.StatusCode != http.StatusOK {
		t.Errorf("PUT kunci biasa status = %d, mau 200 (body: %s)", res.StatusCode, raw)
	}
}

// TestB433UpdateUserNilaiSamaNoChanges adalah pengunci B-433:
// updateUser dengan body BERISI tetapi nilai-sama dengan yang
// tersimpan dulu menjawab 200 + audit "update" padahal nol
// mutasi (dan untuk nama/status bahkan tetap menulis DB). Kini
// nilai-sama total = 400 no_changes tanpa audit, konsisten pola
// Fase 17/18; perubahan nyata tetap 200 + teraudit.
func TestB433UpdateUserNilaiSamaNoChanges(t *testing.T) {
	env := setupTestEnv(t)
	ctx := context.Background()
	c := env.newClient(t)
	c.login("superadmin@routex.internal", testPassword)

	res, body := c.do(http.MethodPost, "/api/admin/access/users", map[string]any{
		"email": "target-b433@routex.test", "password": "GANTI_sandi_uji_b433", "name": "Target B433",
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

	// Tiga bentuk nilai-sama: peran yang sudah dipegang, nama yang
	// sama persis, status yang sedang berlaku.
	for nama, payload := range map[string]map[string]any{
		"peran sama":  {"role": "user"},
		"nama sama":   {"name": "Target B433"},
		"status sama": {"status": "active"},
	} {
		res, body := c.do(http.MethodPut, url, payload, true)
		if res.StatusCode != http.StatusBadRequest {
			t.Errorf("%s: status = %d, mau 400 (body: %s)", nama, res.StatusCode, body)
		}
		if !strings.Contains(string(body), "no_changes") {
			t.Errorf("%s: body = %s, mau memuat kode no_changes", nama, body)
		}
	}
	if n := jumlahAuditUpdate(); n != 0 {
		t.Errorf("baris audit update = %d sesudah tiga PUT nilai-sama, mau 0 — audit palsu (B-433)", n)
	}

	// Kontrol: perubahan nyata tetap 200 dan teraudit.
	res, body = c.do(http.MethodPut, url, map[string]any{"name": "Target B433 Baru"}, true)
	if res.StatusCode != http.StatusOK {
		t.Fatalf("PUT nama baru: status = %d, mau 200 (body: %s)", res.StatusCode, body)
	}
	res, body = c.do(http.MethodPut, url, map[string]any{"role": "admin"}, true)
	if res.StatusCode != http.StatusOK {
		t.Fatalf("PUT peran admin: status = %d, mau 200 (body: %s)", res.StatusCode, body)
	}
	if n := jumlahAuditUpdate(); n != 2 {
		t.Errorf("baris audit update = %d sesudah dua perubahan nyata, mau 2", n)
	}
}

// TestB434SetAllowedIdentikTanpaAudit adalah pengunci B-434
// permukaan setAllowed: penetapan himpunan yang IDENTIK dengan
// yang tersimpan dulu tetap DELETE+INSERT dan menulis audit
// "set_allowed" seolah ada perubahan. Kini no-op terdeteksi atas
// himpunan UUID hasil resolusi (body boleh slug/nama maupun
// UUID): respons tetap 200, tanpa audit baru.
func TestB434SetAllowedIdentikTanpaAudit(t *testing.T) {
	env := setupTestEnv(t)
	ctx := context.Background()

	provRepo := upstream.NewProviderRepo(env.pool)
	prov, err := provRepo.Create(ctx, upstream.CreateProviderParams{
		Name: "prov-b434", DisplayName: "Provider B-434", Kind: "openai", BaseURL: "https://api.openai.com",
	})
	if err != nil {
		t.Fatalf("buat provider: %v", err)
	}

	c := env.newClient(t)
	c.login("superadmin@routex.internal", testPassword)

	res, body := c.do(http.MethodPost, "/api/admin/upstreams/models", map[string]any{
		"model_id": "model-b434", "display_name": "Model B-434",
	}, true)
	if res.StatusCode != http.StatusCreated {
		t.Fatalf("buat model: status = %d (body: %s)", res.StatusCode, body)
	}
	var model struct {
		ID string `json:"id"`
	}
	if err := json.Unmarshal(body, &model); err != nil || model.ID == "" {
		t.Fatalf("urai id model: %v (body: %s)", err, body)
	}

	res, body = c.do(http.MethodPost, "/api/admin/access/api-keys", map[string]any{
		"name": "Kunci B434",
	}, true)
	if res.StatusCode != http.StatusCreated {
		t.Fatalf("buat kunci: status = %d (body: %s)", res.StatusCode, body)
	}
	var kunci struct {
		Key struct {
			ID string `json:"id"`
		} `json:"key"`
	}
	if err := json.Unmarshal(body, &kunci); err != nil || kunci.Key.ID == "" {
		t.Fatalf("urai id kunci: %v (body: %s)", err, body)
	}
	url := "/api/admin/access/api-keys/" + kunci.Key.ID + "/allowed"

	// Penetapan pertama (perubahan nyata, bentuk slug/nama).
	res, body = c.do(http.MethodPost, url, map[string]any{
		"model_ids": []string{"model-b434"}, "provider_ids": []string{"prov-b434"},
	}, true)
	if res.StatusCode != http.StatusOK {
		t.Fatalf("set pertama: status = %d (body: %s)", res.StatusCode, body)
	}
	if n := hitungAudit(t, env, "set_allowed", "api_key"); n != 1 {
		t.Fatalf("audit set_allowed = %d sesudah set pertama, mau 1", n)
	}

	// Penetapan identik — kali ini dalam bentuk UUID: tetap 200,
	// tanpa audit perubahan baru.
	res, body = c.do(http.MethodPost, url, map[string]any{
		"model_ids": []string{model.ID}, "provider_ids": []string{prov.ID},
	}, true)
	if res.StatusCode != http.StatusOK {
		t.Fatalf("set identik: status = %d, mau 200 (body: %s)", res.StatusCode, body)
	}
	if n := hitungAudit(t, env, "set_allowed", "api_key"); n != 1 {
		t.Errorf("audit set_allowed = %d sesudah set identik, mau tetap 1 — no-op himpunan masih diaudit sebagai perubahan (B-434)", n)
	}

	// Kontrol: himpunan yang berubah tetap diterapkan + diaudit.
	res, body = c.do(http.MethodPost, url, map[string]any{
		"model_ids": []string{}, "provider_ids": []string{"prov-b434"},
	}, true)
	if res.StatusCode != http.StatusOK {
		t.Fatalf("set berubah: status = %d (body: %s)", res.StatusCode, body)
	}
	if n := hitungAudit(t, env, "set_allowed", "api_key"); n != 2 {
		t.Errorf("audit set_allowed = %d sesudah set berubah, mau 2", n)
	}
}

// TestB434SetRuleProvidersIdentikNoOp adalah pengunci B-434
// permukaan setRuleProviders: pemasangan ikatan yang identik
// (urutan + bobot) dulu tetap DELETE+INSERT + audit
// "set_providers". Kini 200 tanpa audit baru.
func TestB434SetRuleProvidersIdentikNoOp(t *testing.T) {
	env := setupTestEnv(t)
	ctx := context.Background()

	provRepo := upstream.NewProviderRepo(env.pool)
	provA, err := provRepo.Create(ctx, upstream.CreateProviderParams{
		Name: "prov-b434-a", DisplayName: "Provider B-434 A", Kind: "openai", BaseURL: "https://api.openai.com",
	})
	if err != nil {
		t.Fatalf("buat provider A: %v", err)
	}
	provB, err := provRepo.Create(ctx, upstream.CreateProviderParams{
		Name: "prov-b434-b", DisplayName: "Provider B-434 B", Kind: "openai", BaseURL: "https://api.openai.com",
	})
	if err != nil {
		t.Fatalf("buat provider B: %v", err)
	}

	c := env.newClient(t)
	c.login("superadmin@routex.internal", testPassword)

	res, body := c.do(http.MethodPost, "/api/admin/gateway/routing-rules", map[string]any{
		"name": "Aturan B434", "strategy": "priority",
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
	url := "/api/admin/gateway/routing-rules/" + aturan.ID + "/providers"

	res, body = c.do(http.MethodPost, url, map[string]any{
		"provider_ids": []string{provA.ID},
	}, true)
	if res.StatusCode != http.StatusOK {
		t.Fatalf("pasang pertama: status = %d (body: %s)", res.StatusCode, body)
	}
	if n := hitungAudit(t, env, "set_providers", "routing_rule"); n != 1 {
		t.Fatalf("audit set_providers = %d sesudah pasang pertama, mau 1", n)
	}

	res, body = c.do(http.MethodPost, url, map[string]any{
		"provider_ids": []string{provA.ID},
	}, true)
	if res.StatusCode != http.StatusOK {
		t.Fatalf("pasang identik: status = %d, mau 200 (body: %s)", res.StatusCode, body)
	}
	if n := hitungAudit(t, env, "set_providers", "routing_rule"); n != 1 {
		t.Errorf("audit set_providers = %d sesudah pasang identik, mau tetap 1 — no-op ikatan masih diaudit (B-434)", n)
	}

	// Kontrol: ikatan berbeda tetap diterapkan + diaudit.
	res, body = c.do(http.MethodPost, url, map[string]any{
		"provider_ids": []string{provB.ID},
	}, true)
	if res.StatusCode != http.StatusOK {
		t.Fatalf("pasang berbeda: status = %d (body: %s)", res.StatusCode, body)
	}
	if n := hitungAudit(t, env, "set_providers", "routing_rule"); n != 2 {
		t.Errorf("audit set_providers = %d sesudah pasang berbeda, mau 2", n)
	}
}

// TestB434UpdateRoutingRuleProviderIdentikNoOp adalah pengunci
// B-434 permukaan provider_ids pada updateRoutingRule: PUT yang
// hanya menyebut provider_ids yang SUDAH terikat dulu tetap
// SetProviders (DELETE+INSERT) dan menulis audit "update". Kini
// total no-op (field ditelan ErrNoChanges + ikatan identik)
// menjawab 200 dengan keadaan tersimpan, tanpa audit.
func TestB434UpdateRoutingRuleProviderIdentikNoOp(t *testing.T) {
	env := setupTestEnv(t)
	ctx := context.Background()

	provRepo := upstream.NewProviderRepo(env.pool)
	provA, err := provRepo.Create(ctx, upstream.CreateProviderParams{
		Name: "prov-b434-c", DisplayName: "Provider B-434 C", Kind: "openai", BaseURL: "https://api.openai.com",
	})
	if err != nil {
		t.Fatalf("buat provider A: %v", err)
	}
	provB, err := provRepo.Create(ctx, upstream.CreateProviderParams{
		Name: "prov-b434-d", DisplayName: "Provider B-434 D", Kind: "openai", BaseURL: "https://api.openai.com",
	})
	if err != nil {
		t.Fatalf("buat provider B: %v", err)
	}

	c := env.newClient(t)
	c.login("superadmin@routex.internal", testPassword)

	res, body := c.do(http.MethodPost, "/api/admin/gateway/routing-rules", map[string]any{
		"name": "Aturan B434 PUT", "strategy": "priority",
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

	// Ikat provA lebih dulu lewat endpoint pasang provider.
	res, body = c.do(http.MethodPost, url+"/providers", map[string]any{
		"provider_ids": []string{provA.ID},
	}, true)
	if res.StatusCode != http.StatusOK {
		t.Fatalf("pasang provider awal: status = %d (body: %s)", res.StatusCode, body)
	}

	// PUT provider_ids yang sama persis → 200, ikatan tampil,
	// tanpa audit "update".
	res, body = c.do(http.MethodPut, url, map[string]any{
		"provider_ids": []string{provA.ID},
	}, true)
	if res.StatusCode != http.StatusOK {
		t.Fatalf("PUT provider identik: status = %d, mau 200 (body: %s)", res.StatusCode, body)
	}
	if !strings.Contains(string(body), provA.ID) {
		t.Errorf("respons PUT = %s, mau memuat provider %s terikat", body, provA.ID)
	}
	if n := hitungAudit(t, env, "update", "routing_rule"); n != 0 {
		t.Errorf("audit update aturan = %d sesudah PUT provider identik, mau 0 — no-op masih diaudit sebagai update (B-434)", n)
	}

	// Kontrol: PUT provider berbeda tetap 200 + teraudit.
	res, body = c.do(http.MethodPut, url, map[string]any{
		"provider_ids": []string{provB.ID},
	}, true)
	if res.StatusCode != http.StatusOK {
		t.Fatalf("PUT provider berbeda: status = %d (body: %s)", res.StatusCode, body)
	}
	if n := hitungAudit(t, env, "update", "routing_rule"); n != 1 {
		t.Errorf("audit update aturan = %d sesudah PUT provider berbeda, mau 1", n)
	}
}
