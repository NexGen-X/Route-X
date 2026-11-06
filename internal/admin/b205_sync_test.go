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

// TestSyncModelsMenghormatiEgressPool membuktikan B-205: jalur
// sinkronisasi model WAJIB memakai egress pool yang dipatok provider /
// kredensial / sesi, dan kegagalan memuat pool adalah kegagalan
// eksplisit — bukan alasan jatuh ke koneksi langsung dari IP server.
// Host upstream pada test ini sengaja TIDAK bisa diresolusi
// (*.invalid): satu-satunya jalan sampai adalah lewat proxy tiruan,
// jadi keberhasilan sync sendiri adalah bukti egress dipakai. Pada
// kode pra-perbaikan seluruh kasus di bawah gagal (502 koneksi
// langsung, atau 200 dengan egress diabaikan total).
func TestSyncModelsMenghormatiEgressPool(t *testing.T) {
	env := setupTestEnv(t)
	ctx := context.Background()
	c := env.newClient(t)
	c.login("superadmin@routex.internal", testPassword)

	prefiks, err := security.ParsePrivateAddrs([]string{"127.0.0.1", "::1"})
	if err != nil {
		t.Fatalf("ParsePrivateAddrs: %v", err)
	}
	env.handlers.factory = gateway.NewFactory(nil, nil,
		security.SSRFPolicy{AllowHTTP: true, AllowedPrivateAddrs: prefiks},
		slog.New(slog.DiscardHandler))

	egressRepo, err := upstream.NewEgressRepo(env.pool, env.cipher)
	if err != nil {
		t.Fatalf("NewEgressRepo: %v", err)
	}
	env.handlers.egressRepo = egressRepo
	provRepo := upstream.NewProviderRepo(env.pool)
	credRepo, err := upstream.NewCredentialRepo(env.pool, env.cipher)
	if err != nil {
		t.Fatalf("NewCredentialRepo: %v", err)
	}
	modelRepo := upstream.NewModelRepo(env.pool)

	buatPool := func(nama string, aktif bool, proxyURL string) *upstream.EgressPool {
		pool, err := egressRepo.Create(ctx, upstream.CreateEgressParams{
			Name: nama, Kind: upstream.EgressHTTP,
			ProxyURL: security.Secret(proxyURL), Enabled: &aktif,
		})
		if err != nil {
			t.Fatalf("buat pool %s: %v", nama, err)
		}
		return pool
	}
	jumlahPemetaan := func(providerID string) int {
		peta, err := modelRepo.ListProviderModels(ctx, upstream.ProviderModelFilter{ProviderID: providerID})
		if err != nil {
			t.Fatalf("ListProviderModels: %v", err)
		}
		return len(peta)
	}
	jalankanSync := func(providerID string) (int, []byte) {
		res, body := c.do(http.MethodPost, "/api/admin/upstreams/providers/"+providerID+"/sync-models", map[string]any{}, true)
		return res.StatusCode, body
	}

	// --- Kasus A: discovery berjalan LEWAT pool provider -------------
	var muA sync.Mutex
	var uriA []string
	proxyA := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		muA.Lock()
		uriA = append(uriA, r.RequestURI)
		muA.Unlock()
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{"data":[{"id":"model-b205-egress"}]}`))
	}))
	defer proxyA.Close()
	poolA := buatPool("pool-b205-a", true, proxyA.URL)
	provA, err := provRepo.Create(ctx, upstream.CreateProviderParams{
		Name: "b205-egress-provider", DisplayName: "b205-egress-provider", Kind: "openai_compatible",
		BaseURL: "http://upstream-b205.invalid", EgressPoolID: &poolA.ID,
	})
	if err != nil {
		t.Fatalf("buat provider A: %v", err)
	}
	st, body := jalankanSync(provA.ID)
	if st != http.StatusOK {
		t.Fatalf("kasus A: status %d, ingin 200 lewat egress (body %s)", st, body)
	}
	var dto SyncModelsResponseDTO
	if err := json.Unmarshal(body, &dto); err != nil || dto.Count != 1 || len(dto.Models) != 1 || dto.Models[0] != "model-b205-egress" {
		t.Errorf("kasus A: dto = %+v (err %v), ingin tepat model-b205-egress", dto, err)
	}
	muA.Lock()
	if len(uriA) != 1 || !strings.Contains(uriA[0], "upstream-b205.invalid") {
		t.Errorf("kasus A: permintaan proxy = %v, ingin tepat 1 ke upstream-b205.invalid", uriA)
	}
	muA.Unlock()

	// --- Kasus B: pool provider NONAKTIF → gagal eksplisit ------------
	var hitsB int
	proxyB := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		hitsB++
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{"data":[{"id":"model-b205-bocor"}]}`))
	}))
	defer proxyB.Close()
	poolB := buatPool("pool-b205-b", false, proxyB.URL)
	provB, err := provRepo.Create(ctx, upstream.CreateProviderParams{
		Name: "b205-pool-nonaktif", DisplayName: "b205-pool-nonaktif", Kind: "openai_compatible",
		BaseURL: "http://upstream-b205.invalid", EgressPoolID: &poolB.ID,
	})
	if err != nil {
		t.Fatalf("buat provider B: %v", err)
	}
	st, body = jalankanSync(provB.ID)
	if st != http.StatusInternalServerError || !strings.Contains(string(body), "internal_error") {
		t.Errorf("kasus B: status %d, ingin 500 internal_error eksplisit (body %s)", st, body)
	}
	if hitsB != 0 {
		t.Errorf("kasus B: proxy nonaktif menerima %d permintaan, ingin 0", hitsB)
	}
	if n := jumlahPemetaan(provB.ID); n != 0 {
		t.Errorf("kasus B: %d pemetaan terpasang, ingin 0", n)
	}

	// --- Kasus C: pool kredensial menimpa pool provider --------------
	// Pool provider NONAKTIF (akan gagal bila dipakai); pool kredensial
	// aktif melayani. Sukses membuktikan urutan provider→kredensial.
	var muC sync.Mutex
	var uriC []string
	proxyC := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		muC.Lock()
		uriC = append(uriC, r.RequestURI)
		muC.Unlock()
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{"data":[{"id":"model-b205-kredensial"}]}`))
	}))
	defer proxyC.Close()
	poolCMenang := buatPool("pool-b205-c-kredensial", true, proxyC.URL)
	provC, err := provRepo.Create(ctx, upstream.CreateProviderParams{
		Name: "b205-override-kredensial", DisplayName: "b205-override-kredensial", Kind: "openai_compatible",
		BaseURL: "http://upstream-b205.invalid", EgressPoolID: &poolB.ID,
	})
	if err != nil {
		t.Fatalf("buat provider C: %v", err)
	}
	if _, err := credRepo.Create(ctx, upstream.CreateCredentialParams{
		ProviderID: provC.ID, Label: "kunci-b205-c",
		Secret:       security.Secret(strings.Join([]string{"GANTI", "kunci", "uji", "b205"}, "_")),
		EgressPoolID: &poolCMenang.ID,
	}); err != nil {
		t.Fatalf("buat kredensial C: %v", err)
	}
	st, body = jalankanSync(provC.ID)
	if st != http.StatusOK {
		t.Fatalf("kasus C: status %d, ingin 200 lewat pool kredensial (body %s)", st, body)
	}
	muC.Lock()
	if len(uriC) != 1 {
		t.Errorf("kasus C: permintaan proxy kredensial = %v, ingin tepat 1", uriC)
	}
	muC.Unlock()

	// --- Kasus D & E memakai Antigravity + sesi OAuth ------------------
	oauthRepo, err := upstream.NewOAuthRepo(env.pool, env.cipher)
	if err != nil {
		t.Fatalf("NewOAuthRepo: %v", err)
	}
	env.handlers.oauthRepo = oauthRepo

	tokenOAuth := func() security.Secret {
		return security.Secret("ya29." + strings.Join([]string{"uji", "b205", "bukan", "token", "asli"}, "-"))
	}
	buatAntigravity := func(nama, base string, poolCred *string) (*upstream.Provider, *upstream.OAuthSessionMeta) {
		prov, err := provRepo.Create(ctx, upstream.CreateProviderParams{
			Name: nama, DisplayName: nama, Kind: "google", BaseURL: base,
		})
		if err != nil {
			t.Fatalf("buat provider %s: %v", nama, err)
		}
		cred, err := credRepo.Create(ctx, upstream.CreateCredentialParams{
			ProviderID: prov.ID, Label: "oauth:b205@contoh.test",
			Secret: tokenOAuth(), EgressPoolID: poolCred,
		})
		if err != nil {
			t.Fatalf("buat kredensial %s: %v", nama, err)
		}
		sesi, err := oauthRepo.UpsertSession(ctx, upstream.UpsertOAuthSessionParams{
			ProviderID: prov.ID, CredentialID: cred.ID,
			AccountEmail: "b205@contoh.test",
			Scopes:       []string{"https://www.googleapis.com/auth/cloud-platform"},
			RefreshToken: security.Secret("rt-b205-uji"),
		})
		if err != nil {
			t.Fatalf("buat sesi %s: %v", nama, err)
		}
		return prov, sesi
	}

	// --- Kasus D: pool kredensial/sesi NONAKTIF — loadCodeAssist dan
	// discovery TIDAK boleh berjalan sama sekali; sync gagal eksplisit
	// dan access token tidak keluar dari IP server.
	var hitsLoadD, hitsDiscD int
	loadD := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		hitsLoadD++
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{"cloudaicompanionProject":"proj-b205-bocor"}`))
	}))
	defer loadD.Close()
	discD := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		hitsDiscD++
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{"models":{"model-b205-d":{}}}`))
	}))
	defer discD.Close()
	env.handlers.oauthClient = oauth.NewGoogleOAuthClient(nil).
		WithCodeAssistEndpoint(loadD.URL + "/v1internal:loadCodeAssist")
	poolDMati := buatPool("pool-b205-d", false, proxyB.URL)
	provD, _ := buatAntigravity("antigravity-b205-d", discD.URL, &poolDMati.ID)
	st, body = jalankanSync(provD.ID)
	if st != http.StatusInternalServerError {
		t.Errorf("kasus D: status %d, ingin 500 gagal eksplisit (body %s)", st, body)
	}
	if hitsLoadD != 0 || hitsDiscD != 0 {
		t.Errorf("kasus D: loadCodeAssist terpanggil %d kali, discovery %d kali — ingin keduanya 0 (token tidak boleh keluar)", hitsLoadD, hitsDiscD)
	}
	if n := jumlahPemetaan(provD.ID); n != 0 {
		t.Errorf("kasus D: %d pemetaan terpasang, ingin 0", n)
	}

	// --- Kasus E: pool kredensial AKTIF — loadCodeAssist DAN discovery
	// sama-sama berjalan lewat proxy; project hasil resolusi mengalir
	// ke body discovery dan tersimpan pada sesi (backfill B-157 utuh).
	var muE sync.Mutex
	var urutanE []string
	var bodyDiscoveryE []byte
	proxyE := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		mentah, _ := io.ReadAll(r.Body)
		muE.Lock()
		defer muE.Unlock()
		w.Header().Set("Content-Type", "application/json")
		switch {
		case strings.HasSuffix(r.URL.Path, ":loadCodeAssist") || strings.Contains(r.RequestURI, ":loadCodeAssist"):
			urutanE = append(urutanE, "load")
			_, _ = w.Write([]byte(`{"cloudaicompanionProject":"proj-b205"}`))
		case strings.Contains(r.RequestURI, ":fetchAvailableModels"):
			urutanE = append(urutanE, "discovery")
			bodyDiscoveryE = mentah
			_, _ = w.Write([]byte(`{"models":{"gemini-b205-e":{"displayName":"Gemini B205"}}}`))
		default:
			http.NotFound(w, r)
		}
	}))
	defer proxyE.Close()
	poolE := buatPool("pool-b205-e", true, proxyE.URL)
	provE, sesiE := buatAntigravity("antigravity-b205-e", "http://antigravity-b205.invalid", &poolE.ID)
	env.handlers.oauthClient = oauth.NewGoogleOAuthClient(nil).
		WithCodeAssistEndpoint("http://loadassist-b205.invalid/v1internal:loadCodeAssist")
	st, body = jalankanSync(provE.ID)
	if st != http.StatusOK {
		t.Fatalf("kasus E: status %d, ingin 200 lewat egress sesi/kredensial (body %s)", st, body)
	}
	muE.Lock()
	if len(urutanE) != 2 || urutanE[0] != "load" || urutanE[1] != "discovery" {
		t.Errorf("kasus E: urutan panggilan proxy = %v, ingin [load discovery]", urutanE)
	}
	var kiriman map[string]any
	if err := json.Unmarshal(bodyDiscoveryE, &kiriman); err != nil || kiriman["project"] != "proj-b205" {
		t.Errorf("kasus E: body discovery = %s (err %v), ingin memuat project proj-b205", bodyDiscoveryE, err)
	}
	muE.Unlock()
	sesudahE, err := oauthRepo.GetSession(ctx, sesiE.ID)
	if err != nil {
		t.Fatalf("kasus E: GetSession: %v", err)
	}
	if sesudahE.CodeAssistProject == nil || *sesudahE.CodeAssistProject != "proj-b205" {
		t.Errorf("kasus E: project sesi = %v, ingin proj-b205 tersimpan", sesudahE.CodeAssistProject)
	}
}
