package admin

import (
	"context"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/NexGen-X/Route-X/internal/database/repo/upstream"
	"github.com/NexGen-X/Route-X/internal/oauth"
	"github.com/NexGen-X/Route-X/internal/security"
	"github.com/NexGen-X/Route-X/internal/webhooks"
)

// hitungAudit menghitung baris audit untuk satu aksi + jenis sumber daya.
func hitungAudit(t *testing.T, env *testEnv, action, resourceType string) int {
	t.Helper()
	var n int
	err := env.pool.QueryRow(context.Background(),
		`select count(*) from audit_logs where action = $1 and resource_type = $2`,
		action, resourceType).Scan(&n)
	if err != nil {
		t.Fatalf("hitung audit %s/%s: %v", action, resourceType, err)
	}
	return n
}

// Test B-199: secret webhook bisa dirotasi lewat PUT update — ciphertext
// dan masked_hint terganti, dekripsi menghasilkan secret baru; update
// tanpa secret (atau secret kosong) tidak menyentuh secret tersimpan.
// Pra-fix: PUT memuat field secret ditolak 400 invalid_json karena field
// tidak dikenal, jadi rotasi hanya bisa lewat hapus + buat ulang.
func TestFase7B199RotasiSecretWebhookLewatUpdate(t *testing.T) {
	env := setupTestEnv(t)
	ctx := context.Background()
	c := env.newClient(t)
	c.login("superadmin@routex.internal", testPassword)

	repoWebhook := webhooks.NewRepo(env.pool)
	kebijakanUji := security.SSRFPolicy{AllowHTTP: true, AllowPrivate: true}
	wh, err := repoWebhook.Create(ctx, webhooks.CreateWebhookParams{
		Name: "webhook-rotasi-uji", URL: "https://example.com/hooks",
		Events:     []string{"provider.unhealthy"},
		RawSecret:  "GANTI_rahasia_lama_webhook",
		Cipher:     env.cipher,
		Enabled:    true,
		MaxRetries: 3,
		TimeoutMS:  10000,
		SSRFPolicy: kebijakanUji,
	})
	if err != nil {
		t.Fatalf("buat webhook: %v", err)
	}

	bacaSecret := func() (ciphertext, hint, plaintext string) {
		t.Helper()
		terbaru, err := repoWebhook.Get(ctx, wh.ID)
		if err != nil {
			t.Fatalf("baca webhook: %v", err)
		}
		plain, err := env.cipher.Decrypt(terbaru.SecretCiphertext, security.WebhookAAD(wh.ID))
		if err != nil {
			t.Fatalf("dekripsi secret webhook: %v", err)
		}
		return terbaru.SecretCiphertext, terbaru.MaskedHint, string(plain)
	}

	ctAwal, hintAwal, plainAwal := bacaSecret()
	if plainAwal != "GANTI_rahasia_lama_webhook" {
		t.Fatalf("secret awal terdekripsi %q, mau secret pembuatan", plainAwal)
	}

	// Rotasi lewat PUT dengan field secret.
	res, body := c.do(http.MethodPut, "/api/admin/automation/webhooks/"+wh.ID,
		map[string]any{"secret": "GANTI_rahasia_baru_9f3k"}, true)
	if res.StatusCode != http.StatusOK {
		t.Fatalf("PUT rotasi secret: status %d, mau 200 (body %s)", res.StatusCode, body)
	}
	ctBaru, hintBaru, plainBaru := bacaSecret()
	if ctBaru == ctAwal {
		t.Error("ciphertext tidak berubah sesudah rotasi")
	}
	if plainBaru != "GANTI_rahasia_baru_9f3k" {
		t.Errorf("secret terdekripsi %q, mau secret baru", plainBaru)
	}
	if hintBaru == hintAwal || hintBaru != webhooks.MaskSecret("GANTI_rahasia_baru_9f3k") {
		t.Errorf("masked_hint = %q, mau hint dari secret baru", hintBaru)
	}
	if strings.Contains(string(body), "GANTI_rahasia_baru") {
		t.Error("respons update membocorkan nilai secret")
	}

	// Audit mencatat fakta rotasinya, bukan nilainya.
	var metadata string
	if err := env.pool.QueryRow(ctx,
		`select metadata::text from audit_logs
		  where action = 'update' and resource_type = 'webhook' and resource_id = $1
		  order by occurred_at desc limit 1`, wh.ID).Scan(&metadata); err != nil {
		t.Fatalf("baca audit update webhook: %v", err)
	}
	if !strings.Contains(metadata, `"secret_rotated": true`) {
		t.Errorf("metadata audit tidak mencatat secret_rotated: %s", metadata)
	}
	if strings.Contains(metadata, "GANTI_rahasia") {
		t.Errorf("metadata audit membocorkan nilai secret: %s", metadata)
	}

	// Update tanpa secret: secret tersimpan tidak berubah.
	res, body = c.do(http.MethodPut, "/api/admin/automation/webhooks/"+wh.ID,
		map[string]any{"name": "webhook-rotasi-uji-2"}, true)
	if res.StatusCode != http.StatusOK {
		t.Fatalf("PUT tanpa secret: status %d (body %s)", res.StatusCode, body)
	}
	ctKini, _, _ := bacaSecret()
	if ctKini != ctBaru {
		t.Error("update tanpa secret mengubah ciphertext")
	}

	// Secret kosong eksplisit: sejak B-273 DITOLAK 400 missing_secret —
	// field yang disebut tetapi kosong bukan lagi "tidak diubah" yang
	// senyap (lihat TestFase11B273SecretWebhookSpasiDitolak). Yang
	// dipertahankan dari B-199 di sini: penolakan itu tidak mengubah
	// secret tersimpan sedikit pun.
	res, body = c.do(http.MethodPut, "/api/admin/automation/webhooks/"+wh.ID,
		map[string]any{"secret": ""}, true)
	if res.StatusCode != http.StatusBadRequest {
		t.Fatalf("PUT secret kosong: status %d, mau 400 sejak B-273 (body %s)", res.StatusCode, body)
	}
	ctKini2, _, plainKini := bacaSecret()
	if ctKini2 != ctBaru || plainKini != "GANTI_rahasia_baru_9f3k" {
		t.Error("penolakan secret kosong mengubah secret tersimpan")
	}
}

// Test B-202 (cache): flush dan perubahan setelan cache menulis audit.
// Pra-fix kedua handler tidak memanggil writeAudit sama sekali.
func TestFase7B202AuditCacheFlushDanSettings(t *testing.T) {
	env := setupTestEnv(t)
	env.handlers.responseCache = mesinCacheUntukAdmin(t)
	c := env.newClient(t)
	c.login("superadmin@routex.internal", testPassword)

	if n := hitungAudit(t, env, "flush", "response_cache"); n != 0 {
		t.Fatalf("audit flush sudah ada %d sebelum aksi", n)
	}
	res, body := c.do(http.MethodPost, "/api/admin/system/cache/flush", nil, true)
	if res.StatusCode != http.StatusOK {
		t.Fatalf("flush cache: status %d (body %s)", res.StatusCode, body)
	}
	if n := hitungAudit(t, env, "flush", "response_cache"); n != 1 {
		t.Errorf("audit flush response_cache = %d baris, mau 1", n)
	}

	res, body = c.do(http.MethodPost, "/api/admin/system/cache/settings",
		map[string]any{"enabled": true, "ttl_seconds": 600}, true)
	if res.StatusCode != http.StatusOK {
		t.Fatalf("update setelan cache: status %d (body %s)", res.StatusCode, body)
	}
	if n := hitungAudit(t, env, "update", "response_cache"); n != 1 {
		t.Errorf("audit update response_cache = %d baris, mau 1", n)
	}
}

// Test B-202 (OAuth): refresh manual sesi OAuth menulis audit.
// Pra-fix handler refresh tidak memanggil writeAudit sama sekali.
func TestFase7B202AuditRefreshOAuth(t *testing.T) {
	env := setupTestEnv(t)
	ctx := context.Background()
	c := env.newClient(t)
	c.login("superadmin@routex.internal", testPassword)

	// Endpoint token pengganti yang menjawab token akses baru.
	tokenSrv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{"access_token":"GANTI_token_akses_baru","expires_in":3600,"token_type":"Bearer"}`))
	}))
	defer tokenSrv.Close()

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
	env.handlers.oauthClient = oauth.NewGoogleOAuthClient(nil).WithTokenEndpoint(tokenSrv.URL)

	prov, err := provRepo.Create(ctx, upstream.CreateProviderParams{
		Name: "oauth-refresh-audit", DisplayName: "oauth-refresh-audit", Kind: "google",
		BaseURL: "https://generativelanguage.googleapis.com",
	})
	if err != nil {
		t.Fatalf("buat provider: %v", err)
	}
	tokenAksesLama := security.Secret("GANTI_token" + "_akses_lama")
	cred, err := credRepo.Create(ctx, upstream.CreateCredentialParams{
		ProviderID: prov.ID, Label: "oauth:refresh-audit",
		Secret: tokenAksesLama,
	})
	if err != nil {
		t.Fatalf("buat kredensial: %v", err)
	}
	if _, err := oauthRepo.UpsertSession(ctx, upstream.UpsertOAuthSessionParams{
		ProviderID: prov.ID, CredentialID: cred.ID,
		AccountEmail: "refresh@contoh.test", ClientID: "client-refresh-audit",
		RefreshToken: security.Secret("GANTI_refresh_token_uji"),
	}); err != nil {
		t.Fatalf("buat sesi: %v", err)
	}

	res, body := c.do(http.MethodPost,
		"/api/admin/upstreams/providers/"+prov.ID+"/oauth/refresh",
		map[string]any{}, true)
	if res.StatusCode != http.StatusOK {
		t.Fatalf("refresh oauth: status %d (body %s)", res.StatusCode, body)
	}
	if !strings.Contains(string(body), `"refreshed_count":1`) {
		t.Fatalf("refresh tidak melaporkan 1 sesi berhasil: %s", body)
	}
	var n int
	if err := env.pool.QueryRow(ctx,
		`select count(*) from audit_logs
		  where action = 'refresh_oauth_session' and resource_type = 'provider' and resource_id = $1`,
		prov.ID).Scan(&n); err != nil {
		t.Fatalf("hitung audit refresh: %v", err)
	}
	if n != 1 {
		t.Errorf("audit refresh_oauth_session = %d baris, mau 1", n)
	}
}
