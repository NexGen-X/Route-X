package admin

import (
	"context"
	"encoding/json"
	"io"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync"
	"testing"

	"github.com/NexGen-X/Route-X/internal/database/repo/upstream"
	"github.com/NexGen-X/Route-X/internal/gateway"
	"github.com/NexGen-X/Route-X/internal/oauth"
	"github.com/NexGen-X/Route-X/internal/security"
)

// TestSyncModelsAntigravityProjectDanDisplayName membuktikan B-157
// ujung-ke-ujung di lapisan admin: sync provider Antigravity yang sesinya
// belum punya project menyelesaikan project SEKALI lewat loadCodeAssist
// (backfill malas), menyimpannya pada sesi, meneruskannya sebagai body
// fetchAvailableModels, dan menangkap displayName upstream ke katalog.
// Sync kedua memakai project tersimpan — loadCodeAssist tidak dipanggil
// lagi.
func TestSyncModelsAntigravityProjectDanDisplayName(t *testing.T) {
	env := setupTestEnv(t)
	ctx := context.Background()
	c := env.newClient(t)
	c.login("superadmin@routex.internal", testPassword)

	// Upstream tiruan: satu server melayani loadCodeAssist dan
	// fetchAvailableModels, mencatat berapa kali masing-masing dipanggil
	// dan body discovery terakhir.
	var mu sync.Mutex
	panggilanLoad := 0
	var bodyDiscovery [][]byte
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		mentah, _ := io.ReadAll(r.Body)
		mu.Lock()
		switch {
		case strings.HasSuffix(r.URL.Path, ":loadCodeAssist"):
			panggilanLoad++
			mu.Unlock()
			w.Header().Set("Content-Type", "application/json")
			_, _ = w.Write([]byte(`{"cloudaicompanionProject":"proj-b157"}`))
		case strings.HasSuffix(r.URL.Path, ":fetchAvailableModels"):
			bodyDiscovery = append(bodyDiscovery, mentah)
			mu.Unlock()
			w.Header().Set("Content-Type", "application/json")
			_, _ = w.Write([]byte(`{
				"models": {
					"gemini-3-pro": {"displayName": "Gemini 3 Pro"},
					"model-argon-x": {"displayName": "Argon"}
				},
				"agentModelSorts": [{"groups": [{"modelIds": ["gemini-3-pro"]}]}]
			}`))
		default:
			mu.Unlock()
			http.NotFound(w, r)
		}
	}))
	defer srv.Close()

	// Pasang dependensi yang harness bersama tidak sediakan: repo OAuth,
	// klien OAuth yang endpoint Code Assist-nya menunjuk server tiruan,
	// dan factory ber-kebijakan SSRF loopback agar discovery ke 127.0.0.1
	// sah (pengecualian per alamat, pola yang sama dengan test adapter).
	oauthRepo, err := upstream.NewOAuthRepo(env.pool, env.cipher)
	if err != nil {
		t.Fatalf("NewOAuthRepo: %v", err)
	}
	env.handlers.oauthRepo = oauthRepo
	env.handlers.oauthClient = oauth.NewGoogleOAuthClient(nil).
		WithCodeAssistEndpoint(srv.URL + "/v1internal:loadCodeAssist")
	prefiks, err := security.ParsePrivateAddrs([]string{"127.0.0.1", "::1"})
	if err != nil {
		t.Fatalf("ParsePrivateAddrs: %v", err)
	}
	env.handlers.factory = gateway.NewFactory(nil, nil,
		security.SSRFPolicy{AllowHTTP: true, AllowedPrivateAddrs: prefiks},
		slog.New(slog.DiscardHandler))

	provRepo := upstream.NewProviderRepo(env.pool)
	credRepo, err := upstream.NewCredentialRepo(env.pool, env.cipher)
	if err != nil {
		t.Fatalf("NewCredentialRepo: %v", err)
	}
	prov, err := provRepo.Create(ctx, upstream.CreateProviderParams{
		Name: "antigravity-b157", DisplayName: "Antigravity B157", Kind: "google",
		BaseURL: srv.URL,
	})
	if err != nil {
		t.Fatalf("buat provider: %v", err)
	}
	// Access token sesi: awalan ya29. membuat sync mengirimnya sebagai
	// Bearer, persis kredensial hasil exchange OAuth. Nilainya rakitan
	// potongan agar bukan literal berbentuk token asli.
	tokenAkses := security.Secret("ya29." + strings.Join([]string{"uji", "b157", "bukan", "token", "asli"}, "-"))
	cred, err := credRepo.Create(ctx, upstream.CreateCredentialParams{
		ProviderID: prov.ID, Label: "oauth:project@contoh.test", Secret: tokenAkses,
	})
	if err != nil {
		t.Fatalf("buat kredensial: %v", err)
	}
	sesi, err := oauthRepo.UpsertSession(ctx, upstream.UpsertOAuthSessionParams{
		ProviderID:   prov.ID,
		CredentialID: cred.ID,
		AccountEmail: "project@contoh.test",
		Scopes:       []string{"https://www.googleapis.com/auth/cloud-platform"},
		RefreshToken: security.Secret("rt-b157-uji"),
	})
	if err != nil {
		t.Fatalf("UpsertSession: %v", err)
	}
	if sesi.CodeAssistProject != nil {
		t.Fatalf("sesi baru sudah ber-project %q — premis backfill gugur", *sesi.CodeAssistProject)
	}

	jalurSync := "/api/admin/upstreams/providers/" + prov.ID + "/sync-models"

	// --- Sync pertama: backfill berjalan ---
	res, body := c.do(http.MethodPost, jalurSync, map[string]any{}, true)
	if res.StatusCode != http.StatusOK {
		t.Fatalf("sync pertama: status %d, body %s", res.StatusCode, body)
	}
	var dto SyncModelsResponseDTO
	if err := json.Unmarshal(body, &dto); err != nil {
		t.Fatalf("urai respons sync: %v", err)
	}
	if dto.Count != 2 {
		t.Fatalf("models_count = %d, ingin 2 (body %s)", dto.Count, body)
	}

	mu.Lock()
	if panggilanLoad != 1 {
		t.Errorf("loadCodeAssist dipanggil %d kali pada sync pertama, ingin tepat 1", panggilanLoad)
	}
	if len(bodyDiscovery) != 1 {
		t.Fatalf("fetchAvailableModels tercatat %d kali, ingin 1", len(bodyDiscovery))
	}
	var kiriman map[string]any
	if err := json.Unmarshal(bodyDiscovery[0], &kiriman); err != nil {
		t.Fatalf("body discovery bukan JSON: %v", err)
	}
	if kiriman["project"] != "proj-b157" {
		t.Errorf("body discovery project = %v, ingin proj-b157 (body %s)", kiriman["project"], bodyDiscovery[0])
	}
	mu.Unlock()

	// Project tersimpan pada sesi.
	sesudah, err := oauthRepo.GetSession(ctx, sesi.ID)
	if err != nil {
		t.Fatalf("GetSession: %v", err)
	}
	if sesudah.CodeAssistProject == nil || *sesudah.CodeAssistProject != "proj-b157" {
		t.Fatalf("project sesi sesudah sync = %v, ingin proj-b157", sesudah.CodeAssistProject)
	}

	// displayName upstream masuk katalog; ID mentah tidak lagi jadi nama.
	modelRepo := upstream.NewModelRepo(env.pool)
	for id, nama := range map[string]string{"gemini-3-pro": "Gemini 3 Pro", "model-argon-x": "Argon"} {
		m, err := modelRepo.GetByModelID(ctx, id)
		if err != nil {
			t.Fatalf("model %s tidak tersinkron: %v", id, err)
		}
		if m.DisplayName != nama {
			t.Errorf("display_name %s = %q, ingin %q", id, m.DisplayName, nama)
		}
	}

	// --- Sync kedua: project tersimpan dipakai, tanpa loadCodeAssist lagi ---
	res, body = c.do(http.MethodPost, jalurSync, map[string]any{}, true)
	if res.StatusCode != http.StatusOK {
		t.Fatalf("sync kedua: status %d, body %s", res.StatusCode, body)
	}
	mu.Lock()
	defer mu.Unlock()
	if panggilanLoad != 1 {
		t.Errorf("loadCodeAssist dipanggil %d kali sesudah sync kedua, ingin tetap 1 (precedence tersimpan)", panggilanLoad)
	}
	if len(bodyDiscovery) != 2 {
		t.Fatalf("fetchAvailableModels tercatat %d kali, ingin 2", len(bodyDiscovery))
	}
	kiriman = nil
	if err := json.Unmarshal(bodyDiscovery[1], &kiriman); err != nil {
		t.Fatalf("body discovery kedua bukan JSON: %v", err)
	}
	if kiriman["project"] != "proj-b157" {
		t.Errorf("body discovery kedua project = %v, ingin proj-b157", kiriman["project"])
	}
}
