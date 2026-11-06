package admin

import (
	"context"
	"encoding/json"
	"fmt"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync"
	"testing"

	"github.com/NexGen-X/Route-X/internal/database/repo/upstream"
	"github.com/NexGen-X/Route-X/internal/gateway"
	"github.com/NexGen-X/Route-X/internal/security"
)

// TestSyncModelsPaginasiDanBaseGoogle membuktikan B-206: discovery
// sync mengikuti paginasi dialek dan menurunkan URL Google dari base
// URL provider. Versi lama meminta SATU halaman tanpa parameter
// ukuran (Anthropic berhenti di ±20 model bawaan, Google di ±50) dan
// mengeraskan host generativelanguage.googleapis.com untuk semua
// provider Google — katalog terpotong senyap, dan kunci provider
// ber-base-URL kustom terkirim ke host resmi.
func TestSyncModelsPaginasiDanBaseGoogle(t *testing.T) {
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

	provRepo := upstream.NewProviderRepo(env.pool)
	modelRepo := upstream.NewModelRepo(env.pool)
	buatProvider := func(nama, kind, base string) *upstream.Provider {
		prov, err := provRepo.Create(ctx, upstream.CreateProviderParams{
			Name: nama, DisplayName: nama, Kind: kind, BaseURL: base,
		})
		if err != nil {
			t.Fatalf("buat provider %s: %v", nama, err)
		}
		return prov
	}
	jalankanSync := func(providerID string) (int, []byte) {
		res, body := c.do(http.MethodPost, "/api/admin/upstreams/providers/"+providerID+"/sync-models", map[string]any{}, true)
		return res.StatusCode, body
	}
	jumlahPemetaan := func(providerID string) int {
		peta, err := modelRepo.ListProviderModels(ctx, upstream.ProviderModelFilter{ProviderID: providerID})
		if err != nil {
			t.Fatalf("ListProviderModels: %v", err)
		}
		return len(peta)
	}

	// --- Anthropic: dua halaman kursor, limit maksimum sejak awal ----
	var muA sync.Mutex
	var kueriA []string
	srvA := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		muA.Lock()
		kueriA = append(kueriA, r.URL.RawQuery)
		muA.Unlock()
		w.Header().Set("Content-Type", "application/json")
		if r.URL.Query().Get("after_id") == "" {
			_, _ = w.Write([]byte(`{"data":[{"id":"claude-b206-1"},{"id":"claude-b206-2"}],"has_more":true,"last_id":"claude-b206-2"}`))
			return
		}
		_, _ = w.Write([]byte(`{"data":[{"id":"claude-b206-3"}],"has_more":false}`))
	}))
	defer srvA.Close()
	provA := buatProvider("anthropic-b206", "anthropic", srvA.URL)
	st, body := jalankanSync(provA.ID)
	if st != http.StatusOK {
		t.Fatalf("anthropic: status %d, ingin 200 (body %s)", st, body)
	}
	var dto SyncModelsResponseDTO
	if err := json.Unmarshal(body, &dto); err != nil {
		t.Fatalf("anthropic: urai dto: %v", err)
	}
	if dto.Count != 3 {
		t.Errorf("anthropic: count = %d, ingin 3 (kedua halaman) — body %s", dto.Count, body)
	}
	muA.Lock()
	if len(kueriA) != 2 {
		t.Errorf("anthropic: %d permintaan, ingin 2 halaman", len(kueriA))
	} else {
		if !strings.Contains(kueriA[0], "limit=1000") {
			t.Errorf("anthropic: kueri halaman 1 = %q, ingin memuat limit=1000", kueriA[0])
		}
		if !strings.Contains(kueriA[1], "after_id=claude-b206-2") {
			t.Errorf("anthropic: kueri halaman 2 = %q, ingin after_id=claude-b206-2", kueriA[1])
		}
	}
	muA.Unlock()

	// --- Google: URL dari base kustom + pageToken ----------------------
	var muG sync.Mutex
	var jalurG []string
	srvG := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		muG.Lock()
		jalurG = append(jalurG, r.URL.Path+"?"+r.URL.RawQuery)
		muG.Unlock()
		w.Header().Set("Content-Type", "application/json")
		if r.URL.Query().Get("pageToken") == "" {
			_, _ = w.Write([]byte(`{"models":[{"name":"models/gemini-b206-1","supportedGenerationMethods":["generateContent"]}],"nextPageToken":"tok2"}`))
			return
		}
		_, _ = w.Write([]byte(`{"models":[{"name":"models/gemini-b206-2","supportedGenerationMethods":["embedContent"]}]}`))
	}))
	defer srvG.Close()
	provG := buatProvider("google-b206", "google", srvG.URL)
	st, body = jalankanSync(provG.ID)
	if st != http.StatusOK {
		t.Fatalf("google: status %d, ingin 200 dari base kustom (body %s)", st, body)
	}
	if err := json.Unmarshal(body, &dto); err != nil {
		t.Fatalf("google: urai dto: %v", err)
	}
	if dto.Count != 2 {
		t.Errorf("google: count = %d, ingin 2 (kedua halaman) — body %s", dto.Count, body)
	}
	muG.Lock()
	if len(jalurG) != 2 {
		t.Errorf("google: %d permintaan ke base kustom, ingin 2 halaman", len(jalurG))
	} else {
		if !strings.HasPrefix(jalurG[0], "/v1beta/models?") || !strings.Contains(jalurG[0], "pageSize=1000") {
			t.Errorf("google: permintaan 1 = %q, ingin /v1beta/models dengan pageSize=1000", jalurG[0])
		}
		if !strings.Contains(jalurG[1], "pageToken=tok2") {
			t.Errorf("google: permintaan 2 = %q, ingin pageToken=tok2", jalurG[1])
		}
	}
	muG.Unlock()
	if n := jumlahPemetaan(provG.ID); n != 2 {
		t.Errorf("google: %d pemetaan, ingin 2", n)
	}

	// --- Kontrol OpenAI-compatible tanpa paginasi: tepat 1 permintaan --
	var hitsO int
	srvO := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		hitsO++
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{"data":[{"id":"model-b206-pol"}]}`))
	}))
	defer srvO.Close()
	provO := buatProvider("b206-kontrol", "openai_compatible", srvO.URL)
	st, _ = jalankanSync(provO.ID)
	if st != http.StatusOK || hitsO != 1 {
		t.Errorf("kontrol: status %d dengan %d permintaan, ingin 200 dan tepat 1", st, hitsO)
	}

	// --- Paginasi liar: token tidak pernah habis → gagal eksplisit -----
	var hitsL int
	srvL := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		hitsL++
		w.Header().Set("Content-Type", "application/json")
		_, _ = fmt.Fprintf(w, `{"data":[{"id":"liar-%d"}],"has_more":true,"last_id":"liar-%d"}`, hitsL, hitsL)
	}))
	defer srvL.Close()
	provL := buatProvider("anthropic-b206-liar", "anthropic", srvL.URL)
	st, body = jalankanSync(provL.ID)
	if st != http.StatusBadGateway || !strings.Contains(string(body), "upstream_error") {
		t.Errorf("liar: status %d, ingin 502 upstream_error (body %s)", st, body)
	}
	if hitsL != 100 {
		t.Errorf("liar: %d permintaan, ingin berhenti tepat di batas 100 halaman", hitsL)
	}
	if n := jumlahPemetaan(provL.ID); n != 0 {
		t.Errorf("liar: %d pemetaan terpasang sebagian, ingin 0", n)
	}
}
