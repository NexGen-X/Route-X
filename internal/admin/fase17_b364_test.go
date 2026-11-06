package admin

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/NexGen-X/Route-X/internal/provision"
	"github.com/NexGen-X/Route-X/internal/responsecache"
)

// TestB364ProvisioningMengosongkanCache adalah pengunci B-364:
// createEgressPool biasa menyiram cache respons sesudah membuat
// pool aktif (B-339), tetapi dua jalur provisioning — Cloudflare
// AI Gateway dan Deno relay — membuat pool aktif lewat
// egressRepo.Create langsung tanpa siraman, jadi entri yang
// disimpan di bawah himpunan egress lama tetap tersaji. Kini
// kedua handler memanggil kosongkanCacheRespons seperti
// permukaan egress lainnya. Klien pihak ketiga diarahkan ke
// server tiruan lewat jahitan konstruktor di provision.go.
func TestB364ProvisioningMengosongkanCache(t *testing.T) {
	env := setupTestEnv(t)
	ctx := context.Background()

	mesin := mesinCacheUntukAdmin(t)
	mesin.SetSettings(true, time.Minute)
	env.handlers.responseCache = mesin

	tanam := func() {
		t.Helper()
		if err := mesin.Set(ctx, "kunci-b364", &responsecache.Entry{
			Model: "model-uji", Raw: json.RawMessage(`{"id":"chatcmpl-b364"}`),
		}); err != nil {
			t.Fatalf("tanam entri cache: %v", err)
		}
	}
	sudahKosong := func(aksi string) {
		t.Helper()
		if _, hit, err := mesin.Get(ctx, "kunci-b364"); err != nil || hit {
			t.Errorf("sesudah %s entri cache masih tersaji (hit=%v, err=%v) — provisioning pool aktif wajib mengosongkan cache", aksi, hit, err)
		}
	}

	c := env.newClient(t)
	c.login("superadmin@routex.internal", testPassword)

	// --- Cloudflare AI Gateway ------------------------------------
	stubCF := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodPost || r.URL.Path != "/accounts/acc-b364/ai-gateway/gateways" {
			http.NotFound(w, r)
			return
		}
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{"success":true,"errors":[],"messages":[],"result":{"id":"routex-ai-gateway"}}`))
	}))
	t.Cleanup(stubCF.Close)
	asliCF := buatKlienCloudflare
	buatKlienCloudflare = func(string, time.Duration) *provision.CloudflareClient {
		return provision.NewCloudflareClient(stubCF.URL, 5*time.Second)
	}
	t.Cleanup(func() { buatKlienCloudflare = asliCF })

	tanam()
	res, body := c.do(http.MethodPost, "/api/admin/upstreams/egress-pools/provision/cloudflare", map[string]any{
		"account_id": "acc-b364",
		"api_token":  "GANTI_token_cloudflare",
		"name":       "pool-cf-b364",
	}, true)
	if res.StatusCode != http.StatusCreated {
		t.Fatalf("provisioning cloudflare: status %d (body %s)", res.StatusCode, body)
	}
	sudahKosong("provisioning Cloudflare")

	// --- Deno relay -------------------------------------------------
	stubDeno := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		switch {
		case r.Method == http.MethodPost && r.URL.Path == "/projects":
			w.WriteHeader(http.StatusCreated)
			_, _ = w.Write([]byte(`{"id":"proj-b364"}`))
		case r.Method == http.MethodPost && r.URL.Path == "/projects/proj-b364/deployments":
			w.WriteHeader(http.StatusCreated)
			_, _ = w.Write([]byte(`{"id":"dep-b364","domains":["relay-b364.deno.dev"]}`))
		default:
			http.NotFound(w, r)
		}
	}))
	t.Cleanup(stubDeno.Close)
	asliDeno := buatKlienDeno
	buatKlienDeno = func(string, time.Duration) *provision.DenoClient {
		return provision.NewDenoClient(stubDeno.URL, 5*time.Second)
	}
	t.Cleanup(func() { buatKlienDeno = asliDeno })

	tanam()
	res, body = c.do(http.MethodPost, "/api/admin/upstreams/egress-pools/provision/deno", map[string]any{
		"access_token": "GANTI_token_deno",
		"project_name": "relay-b364",
		"name":         "pool-deno-b364",
	}, true)
	if res.StatusCode != http.StatusCreated {
		t.Fatalf("provisioning deno: status %d (body %s)", res.StatusCode, body)
	}
	sudahKosong("provisioning Deno")
}
