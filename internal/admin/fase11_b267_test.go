package admin

import (
	"context"
	"crypto"
	"crypto/rand"
	"crypto/rsa"
	"crypto/sha256"
	"encoding/base64"
	"encoding/json"
	"fmt"
	"math/big"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/NexGen-X/Route-X/internal/database/repo/upstream"
	"github.com/NexGen-X/Route-X/internal/oauth"
)

// TestFase11B267ExchangeTanpaEmailDitolak membuktikan separuh B-267 di
// lapisan handler: exchange yang tidak menghasilkan email terverifikasi
// (respons token tanpa id_token) DITOLAK dan tidak menyimpan apa pun —
// cadangan lama "google-account" menimpa sesi akun pertama secara senyap.
func TestFase11B267ExchangeTanpaEmailDitolak(t *testing.T) {
	env := setupTestEnv(t)
	ctx := context.Background()
	c := env.newClient(t)
	c.login("superadmin@routex.internal", testPassword)

	tokenSrv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{"access_token":"GANTI_akses_tanpa_idtoken","expires_in":3600,"refresh_token":"GANTI_refresh","token_type":"Bearer"}`))
	}))
	defer tokenSrv.Close()
	env.handlers.oauthClient = oauth.NewGoogleOAuthClient(nil).WithTokenEndpoint(tokenSrv.URL)
	// Test env tidak memasang OAuthRepo pada Config handler (produksi
	// memasangnya di main.go) — pasang eksplisit seperti pola fase 7.
	oauthRepoLokal, err := upstream.NewOAuthRepo(env.pool, env.cipher)
	if err != nil {
		t.Fatalf("NewOAuthRepo: %v", err)
	}
	env.handlers.oauthRepo = oauthRepoLokal

	provRepo := upstream.NewProviderRepo(env.pool)
	prov, err := provRepo.Create(ctx, upstream.CreateProviderParams{
		Name: "b267-tanpa-email", DisplayName: "b267-tanpa-email",
		Kind: "google", BaseURL: "https://generativelanguage.googleapis.com",
	})
	if err != nil {
		t.Fatalf("buat provider: %v", err)
	}

	res, body := c.do(http.MethodPost,
		"/api/admin/upstreams/providers/"+prov.ID+"/oauth/exchange",
		map[string]any{"code": "4/kode-uji"}, true)
	if res.StatusCode != http.StatusBadGateway {
		t.Fatalf("exchange tanpa email: status %d, mau 502 (body %s)", res.StatusCode, body)
	}
	if got := kodeEnvelope(t, body); got != "oauth_exchange_failed" {
		t.Errorf("code = %q, mau oauth_exchange_failed", got)
	}

	credRepo, err := upstream.NewCredentialRepo(env.pool, env.cipher)
	if err != nil {
		t.Fatalf("NewCredentialRepo: %v", err)
	}
	creds, err := credRepo.List(ctx, prov.ID)
	if err != nil {
		t.Fatalf("List kredensial: %v", err)
	}
	if len(creds) != 0 {
		t.Errorf("kredensial tersimpan %d, mau 0 — exchange tanpa email tidak boleh menyimpan apa pun", len(creds))
	}
	oauthRepo, err := upstream.NewOAuthRepo(env.pool, env.cipher)
	if err != nil {
		t.Fatalf("NewOAuthRepo: %v", err)
	}
	sessions, err := oauthRepo.ListSessionsByProviderID(ctx, prov.ID)
	if err != nil {
		t.Fatalf("ListSessionsByProviderID: %v", err)
	}
	if len(sessions) != 0 {
		t.Errorf("sesi tersimpan %d, mau 0", len(sessions))
	}
}

// TestFase11B267ExchangeBerhasilAtomik adalah kontrol positifnya:
// id_token bertanda tangan sah (JWKS tiruan) menghasilkan sesi dengan
// email terverifikasi itu, kredensial terhubung ke sesi, dan provider
// tersinkron ke kind/base_url Google dalam transaksi yang sama.
func TestFase11B267ExchangeBerhasilAtomik(t *testing.T) {
	env := setupTestEnv(t)
	ctx := context.Background()
	c := env.newClient(t)
	c.login("superadmin@routex.internal", testPassword)

	priv, err := rsa.GenerateKey(rand.Reader, 2048)
	if err != nil {
		t.Fatalf("membuat kunci RSA uji: %v", err)
	}
	const kid = "kid-uji-b267"
	jwksSrv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		n := base64.RawURLEncoding.EncodeToString(priv.PublicKey.N.Bytes())
		e := base64.RawURLEncoding.EncodeToString(big.NewInt(int64(priv.PublicKey.E)).Bytes())
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(fmt.Sprintf(
			`{"keys":[{"kid":"%s","kty":"RSA","alg":"RS256","use":"sig","n":"%s","e":"%s"}]}`, kid, n, e)))
	}))
	defer jwksSrv.Close()

	klaim, _ := json.Marshal(map[string]any{
		"iss":            "https://accounts.google.com",
		"aud":            oauth.DefaultAntigravityClientID,
		"exp":            time.Now().Add(time.Hour).Unix(),
		"email":          "tersambung@contoh.test",
		"email_verified": true,
		"name":           "Akun Tersambung",
	})
	header := base64.RawURLEncoding.EncodeToString([]byte(`{"alg":"RS256","kid":"` + kid + `","typ":"JWT"}`))
	data := header + "." + base64.RawURLEncoding.EncodeToString(klaim)
	ringkasan := sha256.Sum256([]byte(data))
	tanda, err := rsa.SignPKCS1v15(rand.Reader, priv, crypto.SHA256, ringkasan[:])
	if err != nil {
		t.Fatalf("menandatangani JWT uji: %v", err)
	}
	idToken := data + "." + base64.RawURLEncoding.EncodeToString(tanda)

	tokenSrv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		_ = json.NewEncoder(w).Encode(map[string]any{
			"access_token":  "GANTI_akses_tersambung",
			"expires_in":    3600,
			"refresh_token": "GANTI_refresh_tersambung",
			"token_type":    "Bearer",
			"id_token":      idToken,
		})
	}))
	defer tokenSrv.Close()

	// Endpoint loadCodeAssist tiruan: menjawab tanpa project agar uji
	// hermetis (kegagalannya pun non-fatal bagi handler).
	caSrv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{}`))
	}))
	defer caSrv.Close()

	env.handlers.oauthClient = oauth.NewGoogleOAuthClient(nil).
		WithTokenEndpoint(tokenSrv.URL).
		WithJWKSURL(jwksSrv.URL).
		WithCodeAssistEndpoint(caSrv.URL)
	oauthRepoLokal, err := upstream.NewOAuthRepo(env.pool, env.cipher)
	if err != nil {
		t.Fatalf("NewOAuthRepo: %v", err)
	}
	env.handlers.oauthRepo = oauthRepoLokal

	provRepo := upstream.NewProviderRepo(env.pool)
	prov, err := provRepo.Create(ctx, upstream.CreateProviderParams{
		Name: "b267-berhasil", DisplayName: "b267-berhasil",
		Kind: upstream.KindOpenAICompatible, BaseURL: "https://contoh.test/v1",
	})
	if err != nil {
		t.Fatalf("buat provider: %v", err)
	}

	res, body := c.do(http.MethodPost,
		"/api/admin/upstreams/providers/"+prov.ID+"/oauth/exchange",
		map[string]any{"code": "4/kode-uji"}, true)
	if res.StatusCode != http.StatusOK {
		t.Fatalf("exchange sah: status %d, mau 200 (body %s)", res.StatusCode, body)
	}

	oauthRepo, err := upstream.NewOAuthRepo(env.pool, env.cipher)
	if err != nil {
		t.Fatalf("NewOAuthRepo: %v", err)
	}
	sessions, err := oauthRepo.ListSessionsByProviderID(ctx, prov.ID)
	if err != nil {
		t.Fatalf("ListSessionsByProviderID: %v", err)
	}
	if len(sessions) != 1 || sessions[0].AccountEmail != "tersambung@contoh.test" {
		t.Fatalf("sesi tersimpan = %+v, mau tepat 1 sesi tersambung@contoh.test", sessions)
	}
	credRepo, err := upstream.NewCredentialRepo(env.pool, env.cipher)
	if err != nil {
		t.Fatalf("NewCredentialRepo: %v", err)
	}
	rahasia, err := credRepo.Reveal(ctx, sessions[0].CredentialID)
	if err != nil {
		t.Fatalf("Reveal kredensial sesi: %v", err)
	}
	if rahasia.Reveal() != "GANTI_akses_tersambung" {
		t.Errorf("access token kredensial sesi = %q, mau GANTI_akses_tersambung", rahasia.Reveal())
	}
	provSesudah, err := provRepo.Get(ctx, prov.ID)
	if err != nil {
		t.Fatalf("Get provider: %v", err)
	}
	if provSesudah.Kind != "google" || provSesudah.BaseURL != "https://generativelanguage.googleapis.com" {
		t.Errorf("provider sesudah exchange = %s %s, mau tersinkron google", provSesudah.Kind, provSesudah.BaseURL)
	}
}
