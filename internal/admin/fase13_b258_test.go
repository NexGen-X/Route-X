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

// B-258: mutasi strategi kredensial provider dan pool egress kredensial
// mengubah konteks jawaban (akun/jalur hulu yang melayani) tanpa mengubah
// sidik rute di kunci cache — tetapi kedua handler tidak memanggil
// kosongkanCacheRespons, tidak seperti mutasi kredensial lain
// (tambah/hapus/toggle). Entri lama tetap tersaji walau konteksnya sudah
// berubah. Test ini menanam satu entri cache, menjalankan kedua mutasi,
// dan menuntut entri itu hilang sesudah masing-masing.
func TestFase13B258MutasiStrategiDanEgressMengosongkanCache(t *testing.T) {
	env := setupTestEnv(t)
	ctx := context.Background()

	mesin := mesinCacheUntukAdmin(t)
	mesin.SetSettings(true, time.Minute)
	env.handlers.responseCache = mesin

	providerRepo := upstream.NewProviderRepo(env.pool)
	credentialRepo, _ := upstream.NewCredentialRepo(env.pool, env.cipher)
	prov, err := providerRepo.Create(ctx, upstream.CreateProviderParams{
		Name: "prov-b258", DisplayName: "Provider B-258", Kind: "openai", BaseURL: "https://api.openai.com",
	})
	if err != nil {
		t.Fatalf("buat provider: %v", err)
	}
	cred, err := credentialRepo.Create(ctx, upstream.CreateCredentialParams{
		ProviderID: prov.ID, Label: "key-b258", Secret: security.Secret("GANTI_kunci_uji_b258"),
	})
	if err != nil {
		t.Fatalf("buat kredensial: %v", err)
	}

	tanam := func() {
		t.Helper()
		if err := mesin.Set(ctx, "kunci-b258", &responsecache.Entry{
			Model: "model-uji", Raw: json.RawMessage(`{"id":"chatcmpl-b258"}`),
		}); err != nil {
			t.Fatalf("tanam entri cache: %v", err)
		}
		if _, hit, err := mesin.Get(ctx, "kunci-b258"); err != nil || !hit {
			t.Fatalf("entri yang baru ditanam tidak terbaca (hit=%v, err=%v)", hit, err)
		}
	}
	sudahKosong := func(aksi string) {
		t.Helper()
		if _, hit, err := mesin.Get(ctx, "kunci-b258"); err != nil || hit {
			t.Errorf("sesudah %s entri cache masih tersaji (hit=%v, err=%v) — mutasi konteks jawaban wajib mengosongkan cache", aksi, hit, err)
		}
	}

	c := env.newClient(t)
	c.login("superadmin@routex.internal", testPassword)

	tanam()
	res, body := c.do(http.MethodPost, "/api/admin/upstreams/providers/"+prov.ID+"/credential-strategy",
		map[string]any{"strategy": "priority"}, true)
	if res.StatusCode != http.StatusOK {
		t.Fatalf("set strategi kredensial: status %d (body %s)", res.StatusCode, body)
	}
	sudahKosong("mengubah strategi kredensial")

	// Kaki egress: perubahan NYATA dua arah (ikat ke pool, lalu
	// lepas) wajib menyiram. Sejak Fase 20 PR-B (B-469),
	// lepas-saat-memang-terlepas dan ikat-ulang pool yang sama
	// adalah no-op TANPA siraman — kontraknya dikunci di
	// fase20_prb_test.go; yang dijaga test ini adalah perubahan
	// konteks yang sesungguhnya tetap menyiram seperti sebelumnya.
	egressRepo, err := upstream.NewEgressRepo(env.pool, env.cipher)
	if err != nil {
		t.Fatalf("NewEgressRepo: %v", err)
	}
	pool, err := egressRepo.Create(ctx, upstream.CreateEgressParams{
		Name: "pool-b258", Kind: upstream.EgressHTTP,
		ProxyURL: security.Secret("http://127.0.0.1:8899"), Enabled: boolPtr(true),
	})
	if err != nil {
		t.Fatalf("buat egress pool: %v", err)
	}

	tanam()
	res, body = c.do(http.MethodPut, "/api/admin/upstreams/providers/"+prov.ID+"/credentials/"+cred.ID+"/egress-pool",
		map[string]any{"egress_pool_id": pool.ID}, true)
	if res.StatusCode != http.StatusOK {
		t.Fatalf("ikat pool egress kredensial: status %d (body %s)", res.StatusCode, body)
	}
	sudahKosong("mengikat pool egress kredensial")

	tanam()
	res, body = c.do(http.MethodPut, "/api/admin/upstreams/providers/"+prov.ID+"/credentials/"+cred.ID+"/egress-pool",
		map[string]any{"egress_pool_id": nil}, true)
	if res.StatusCode != http.StatusOK {
		t.Fatalf("lepas pool egress kredensial: status %d (body %s)", res.StatusCode, body)
	}
	sudahKosong("melepas pool egress kredensial")
}
