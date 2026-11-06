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
	"github.com/NexGen-X/Route-X/internal/responsecache"
	"github.com/NexGen-X/Route-X/internal/security"
)

// TestB339MutasiOAuthEgressMengosongkanCache adalah pengunci B-339:
// tujuh mutasi yang mengubah akun/jalur hulu yang melayani — exchange,
// refresh, toggle, dan delete sesi OAuth, serta create/update/delete
// egress pool — tidak memanggil kosongkanCacheRespons, tidak seperti
// mutasi kredensial lain (pola B-258). Sidik rute di kunci cache
// tidak memuat identitas sesi OAuth maupun pool, jadi entri yang
// disimpan di bawah akun/pool lama tetap tersaji sesudah konteksnya
// berubah. Test ini menanam satu entri cache dan menuntutnya hilang
// sesudah SETIAP mutasi.
func TestB339MutasiOAuthEgressMengosongkanCache(t *testing.T) {
	env := setupTestEnv(t)
	ctx := context.Background()

	mesin := mesinCacheUntukAdmin(t)
	mesin.SetSettings(true, time.Minute)
	env.handlers.responseCache = mesin

	tanam := func() {
		t.Helper()
		if err := mesin.Set(ctx, "kunci-b339", &responsecache.Entry{
			Model: "model-uji", Raw: json.RawMessage(`{"id":"chatcmpl-b339"}`),
		}); err != nil {
			t.Fatalf("tanam entri cache: %v", err)
		}
		if _, hit, err := mesin.Get(ctx, "kunci-b339"); err != nil || !hit {
			t.Fatalf("entri yang baru ditanam tidak terbaca (hit=%v, err=%v)", hit, err)
		}
	}
	sudahKosong := func(aksi string) {
		t.Helper()
		if _, hit, err := mesin.Get(ctx, "kunci-b339"); err != nil || hit {
			t.Errorf("sesudah %s entri cache masih tersaji (hit=%v, err=%v) — mutasi konteks jawaban wajib mengosongkan cache", aksi, hit, err)
		}
	}

	c := env.newClient(t)
	c.login("superadmin@routex.internal", testPassword)

	// --- Egress pool: create, update, delete -------------------------
	res, body := c.do(http.MethodPost, "/api/admin/upstreams/egress-pools", map[string]any{
		"name":      "pool-b339",
		"kind":      "http",
		"proxy_url": "http://pengguna:GANTI_rahasia@proxy.contoh.test:18080",
	}, true)
	if res.StatusCode != http.StatusCreated {
		t.Fatalf("buat egress pool: status %d (body %s)", res.StatusCode, body)
	}
	var pool EgressPoolDTO
	if err := json.Unmarshal(body, &pool); err != nil {
		t.Fatalf("decode egress pool: %v", err)
	}
	// Untuk create sendiri, pool kedua dipakai agar urutan
	// tanam → mutasi → periksa berlaku sama seperti mutasi lainnya.
	tanam()
	res, body = c.do(http.MethodPost, "/api/admin/upstreams/egress-pools", map[string]any{
		"name":      "pool-b339-kedua",
		"kind":      "http",
		"proxy_url": "http://pengguna:GANTI_rahasia@proxy.contoh.test:18081",
	}, true)
	if res.StatusCode != http.StatusCreated {
		t.Fatalf("buat egress pool kedua: status %d (body %s)", res.StatusCode, body)
	}
	sudahKosong("membuat egress pool")

	tanam()
	res, body = c.do(http.MethodPut, "/api/admin/upstreams/egress-pools/"+pool.ID, map[string]any{
		"name": "pool-b339-renamed",
	}, true)
	if res.StatusCode != http.StatusOK {
		t.Fatalf("ubah egress pool: status %d (body %s)", res.StatusCode, body)
	}
	sudahKosong("mengubah egress pool")

	tanam()
	res, body = c.do(http.MethodDelete, "/api/admin/upstreams/egress-pools/"+pool.ID, nil, true)
	if res.StatusCode != http.StatusOK {
		t.Fatalf("hapus egress pool: status %d (body %s)", res.StatusCode, body)
	}
	sudahKosong("menghapus egress pool")

	// --- Sesi OAuth: toggle, refresh, delete -------------------------
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
		Name: "prov-b339", DisplayName: "Provider B-339", Kind: "google",
		BaseURL: "https://generativelanguage.googleapis.com",
	})
	if err != nil {
		t.Fatalf("buat provider: %v", err)
	}
	tokenAkses := security.Secret("GANTI_kunci" + "_akses_b339")
	cred, err := credRepo.Create(ctx, upstream.CreateCredentialParams{
		ProviderID: prov.ID, Label: "oauth:b339@contoh.test",
		Secret: tokenAkses,
	})
	if err != nil {
		t.Fatalf("buat kredensial: %v", err)
	}
	sesi, err := oauthRepo.UpsertSession(ctx, upstream.UpsertOAuthSessionParams{
		ProviderID: prov.ID, CredentialID: cred.ID,
		AccountEmail: "b339@contoh.test", ClientID: "client-b339",
		RefreshToken: security.Secret("GANTI_refresh_token_b339"),
	})
	if err != nil {
		t.Fatalf("buat sesi: %v", err)
	}

	tanam()
	res, body = c.do(http.MethodPost,
		"/api/admin/upstreams/providers/"+prov.ID+"/oauth/sessions/"+sesi.ID+"/toggle",
		map[string]any{"enabled": false}, true)
	if res.StatusCode != http.StatusOK {
		t.Fatalf("toggle sesi: status %d (body %s)", res.StatusCode, body)
	}
	sudahKosong("menonaktifkan sesi OAuth")

	// Refresh memerlukan sesi aktif: nyalakan lagi, lalu refresh lewat
	// endpoint token pengganti (pola B-202).
	res, _ = c.do(http.MethodPost,
		"/api/admin/upstreams/providers/"+prov.ID+"/oauth/sessions/"+sesi.ID+"/toggle",
		map[string]any{"enabled": true}, true)
	if res.StatusCode != http.StatusOK {
		t.Fatalf("menyalakan lagi sesi: status %d", res.StatusCode)
	}
	tokenSrv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{"access_token":"GANTI_token_akses_baru","expires_in":3600,"token_type":"Bearer"}`))
	}))
	defer tokenSrv.Close()
	env.handlers.oauthClient = oauth.NewGoogleOAuthClient(nil).WithTokenEndpoint(tokenSrv.URL)

	tanam()
	res, body = c.do(http.MethodPost,
		"/api/admin/upstreams/providers/"+prov.ID+"/oauth/refresh",
		map[string]any{}, true)
	if res.StatusCode != http.StatusOK {
		t.Fatalf("refresh sesi: status %d (body %s)", res.StatusCode, body)
	}
	sudahKosong("me-refresh sesi OAuth")

	tanam()
	res, body = c.do(http.MethodDelete,
		"/api/admin/upstreams/providers/"+prov.ID+"/oauth/sessions/"+sesi.ID, nil, true)
	if res.StatusCode != http.StatusNoContent {
		t.Fatalf("hapus sesi: status %d (body %s)", res.StatusCode, body)
	}
	sudahKosong("menghapus sesi OAuth")

	// --- Exchange OAuth (id_token bertanda tangan + JWKS tiruan) -----
	priv, err := rsa.GenerateKey(rand.Reader, 2048)
	if err != nil {
		t.Fatalf("membuat kunci RSA uji: %v", err)
	}
	const kid = "kid-uji-b339"
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
		"email":          "tersambung-b339@contoh.test",
		"email_verified": true,
		"name":           "Akun B-339",
	})
	header := base64.RawURLEncoding.EncodeToString([]byte(`{"alg":"RS256","kid":"` + kid + `","typ":"JWT"}`))
	data := header + "." + base64.RawURLEncoding.EncodeToString(klaim)
	ringkasan := sha256.Sum256([]byte(data))
	tanda, err := rsa.SignPKCS1v15(rand.Reader, priv, crypto.SHA256, ringkasan[:])
	if err != nil {
		t.Fatalf("menandatangani JWT uji: %v", err)
	}
	idToken := data + "." + base64.RawURLEncoding.EncodeToString(tanda)
	tokenSrv2 := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		_ = json.NewEncoder(w).Encode(map[string]any{
			"access_token":  "GANTI_akses_tersambung",
			"expires_in":    3600,
			"refresh_token": "GANTI_refresh_tersambung",
			"token_type":    "Bearer",
			"id_token":      idToken,
		})
	}))
	defer tokenSrv2.Close()
	caSrv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{}`))
	}))
	defer caSrv.Close()
	env.handlers.oauthClient = oauth.NewGoogleOAuthClient(nil).
		WithTokenEndpoint(tokenSrv2.URL).
		WithJWKSURL(jwksSrv.URL).
		WithCodeAssistEndpoint(caSrv.URL)

	prov2, err := provRepo.Create(ctx, upstream.CreateProviderParams{
		Name: "prov-b339-exchange", DisplayName: "Provider B-339 Exchange",
		Kind: upstream.KindOpenAICompatible, BaseURL: "https://contoh.test/v1",
	})
	if err != nil {
		t.Fatalf("buat provider exchange: %v", err)
	}
	tanam()
	res, body = c.do(http.MethodPost,
		"/api/admin/upstreams/providers/"+prov2.ID+"/oauth/exchange",
		map[string]any{"code": "4/kode-uji"}, true)
	if res.StatusCode != http.StatusOK {
		t.Fatalf("exchange oauth: status %d (body %s)", res.StatusCode, body)
	}
	sudahKosong("menukar kode OAuth (exchange)")
}
