package admin

import (
	"context"
	"encoding/json"
	"net/http"
	"testing"
	"time"

	"github.com/NexGen-X/Route-X/internal/database/repo/upstream"
	"github.com/NexGen-X/Route-X/internal/responsecache"
	"github.com/NexGen-X/Route-X/internal/security"
)

// TestB392GantiPoolProviderMengosongkanCache adalah pengunci
// B-392: updateProvider tidak pernah mengosongkan response cache,
// padahal mengganti egress_pool_id mengubah jalur egress yang
// melayani jawaban sementara sidik rute di kunci cache tidak
// memuat pool kandidat — entri lama tersaji terus untuk konteks
// jalur yang sudah berubah (kembaran B-258 yang lolos tiga
// sapuan). Kini body yang menyebut egress_pool_id memicu flush;
// update field lain (yang mengubah sidik rute) tidak.
func TestB392GantiPoolProviderMengosongkanCache(t *testing.T) {
	env := setupTestEnv(t)
	ctx := context.Background()

	mesin := mesinCacheUntukAdmin(t)
	mesin.SetSettings(true, time.Minute)
	env.handlers.responseCache = mesin

	providerRepo := upstream.NewProviderRepo(env.pool)
	egressRepo, err := upstream.NewEgressRepo(env.pool, env.cipher)
	if err != nil {
		t.Fatalf("NewEgressRepo: %v", err)
	}
	prov, err := providerRepo.Create(ctx, upstream.CreateProviderParams{
		Name: "prov-b392", DisplayName: "Provider B-392", Kind: "openai", BaseURL: "https://api.openai.com",
	})
	if err != nil {
		t.Fatalf("buat provider: %v", err)
	}
	aktif := true
	pool, err := egressRepo.Create(ctx, upstream.CreateEgressParams{
		Name: "pool-b392", Kind: upstream.EgressHTTP,
		ProxyURL: security.Secret("http://127.0.0.1:18081"), Enabled: &aktif,
	})
	if err != nil {
		t.Fatalf("buat pool egress: %v", err)
	}

	tanam := func() {
		t.Helper()
		if err := mesin.Set(ctx, "kunci-b392", &responsecache.Entry{
			Model: "model-uji", Raw: json.RawMessage(`{"id":"chatcmpl-b392"}`),
		}); err != nil {
			t.Fatalf("tanam entri cache: %v", err)
		}
	}
	hit := func() bool {
		t.Helper()
		_, h, err := mesin.Get(ctx, "kunci-b392")
		if err != nil {
			t.Fatalf("baca cache: %v", err)
		}
		return h
	}

	c := env.newClient(t)
	c.login("superadmin@routex.internal", testPassword)
	url := "/api/admin/upstreams/providers/" + prov.ID

	// Kontrol: update TANPA egress_pool_id tidak mengosongkan
	// cache (field provider lain mengubah sidik rute kuncinya).
	tanam()
	res, body := c.do(http.MethodPut, url, map[string]any{"display_name": "Provider B-392 Baru"}, true)
	if res.StatusCode != http.StatusOK {
		t.Fatalf("PUT display_name: status = %d, mau 200 (body: %s)", res.StatusCode, body)
	}
	if !hit() {
		t.Error("entri cache hilang sesudah update tanpa egress_pool_id — flush terlalu luas")
	}

	// PUT dengan egress_pool_id: cache WAJIB kosong.
	tanam()
	res, body = c.do(http.MethodPut, url, map[string]any{"egress_pool_id": pool.ID}, true)
	if res.StatusCode != http.StatusOK {
		t.Fatalf("PUT egress_pool_id: status = %d, mau 200 (body: %s)", res.StatusCode, body)
	}
	if hit() {
		t.Error("entri cache masih tersaji sesudah ganti egress_pool_id provider — konteks jalur berubah tanpa kunci baru")
	}

	// Melepas ikatan ("" eksplisit = Clear pada DTO ini; null
	// melebur dengan absen) juga perubahan jalur: flush.
	tanam()
	res, body = c.do(http.MethodPut, url, map[string]any{"egress_pool_id": ""}, true)
	if res.StatusCode != http.StatusOK {
		t.Fatalf("PUT egress_pool_id \"\": status = %d, mau 200 (body: %s)", res.StatusCode, body)
	}
	if hit() {
		t.Error("entri cache masih tersaji sesudah melepas egress_pool_id provider")
	}
}
