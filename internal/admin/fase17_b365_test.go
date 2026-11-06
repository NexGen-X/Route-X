package admin

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/NexGen-X/Route-X/internal/database/repo/upstream"
	"github.com/NexGen-X/Route-X/internal/oauth"
	"github.com/NexGen-X/Route-X/internal/responsecache"
	"github.com/NexGen-X/Route-X/internal/security"
)

// TestB365RefreshGagalTotalDenganPenonaktifanTetapMenyiram adalah
// pengunci B-365 sisi handler admin: refreshOAuthSession menjawab
// 502 pada cabang kegagalan total TANPA melewati siraman B-339 di
// ujung handler — padahal kegagalan itu bisa mencakup penonaktifan
// sesi (refresh token ditolak permanen), yang mengubah himpunan
// kredensial. Kini cabang itu menyiram bila callback
// SesudahNonaktif mencatat penonaktifan selama putaran.
func TestB365RefreshGagalTotalDenganPenonaktifanTetapMenyiram(t *testing.T) {
	env := setupTestEnv(t)
	ctx := context.Background()

	mesin := mesinCacheUntukAdmin(t)
	mesin.SetSettings(true, time.Minute)
	env.handlers.responseCache = mesin

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
		Name: "prov-b365", DisplayName: "Provider B-365", Kind: "google",
		BaseURL: "https://generativelanguage.googleapis.com",
	})
	if err != nil {
		t.Fatalf("buat provider: %v", err)
	}
	tokenAkses := security.Secret("GANTI_kunci" + "_akses_b365")
	cred, err := credRepo.Create(ctx, upstream.CreateCredentialParams{
		ProviderID: prov.ID, Label: "oauth:b365@contoh.test",
		Secret: tokenAkses,
	})
	if err != nil {
		t.Fatalf("buat kredensial: %v", err)
	}
	sesi, err := oauthRepo.UpsertSession(ctx, upstream.UpsertOAuthSessionParams{
		ProviderID: prov.ID, CredentialID: cred.ID,
		AccountEmail: "b365@contoh.test", ClientID: "client-b365",
		RefreshToken: security.Secret("GANTI_refresh_token_b365"),
	})
	if err != nil {
		t.Fatalf("buat sesi: %v", err)
	}

	// Endpoint token Google tiruan: refresh token ditolak permanen.
	tokenSrv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusBadRequest)
		_, _ = w.Write([]byte(`{"error":"invalid_grant","error_description":"Token has been expired or revoked."}`))
	}))
	defer tokenSrv.Close()
	env.handlers.oauthClient = oauth.NewGoogleOAuthClient(nil).WithTokenEndpoint(tokenSrv.URL)

	if err := mesin.Set(ctx, "kunci-b365", &responsecache.Entry{
		Model: "model-uji", Raw: json.RawMessage(`{"id":"chatcmpl-b365"}`),
	}); err != nil {
		t.Fatalf("tanam entri cache: %v", err)
	}

	c := env.newClient(t)
	c.login("superadmin@routex.internal", testPassword)
	res, body := c.do(http.MethodPost,
		"/api/admin/upstreams/providers/"+prov.ID+"/oauth/refresh",
		map[string]any{}, true)
	if res.StatusCode != http.StatusBadGateway {
		t.Fatalf("refresh gagal total: status %d, mau 502 (body %s)", res.StatusCode, body)
	}

	// Sesi memang dinonaktifkan oleh penolakan permanen...
	var aktif bool
	if err := env.pool.QueryRow(ctx,
		`select enabled from provider_oauth_sessions where id = $1`, sesi.ID).Scan(&aktif); err != nil {
		t.Fatalf("baca status sesi: %v", err)
	}
	if aktif {
		t.Fatal("sesi masih aktif sesudah invalid_grant — prasyarat skenario B-365 tidak terpenuhi")
	}
	// ...dan walau jawabannya 502, cache tetap tersiram.
	if _, hit, err := mesin.Get(ctx, "kunci-b365"); err != nil || hit {
		t.Errorf("entri cache masih tersaji sesudah refresh gagal total yang menonaktifkan sesi (hit=%v, err=%v)", hit, err)
	}
}
