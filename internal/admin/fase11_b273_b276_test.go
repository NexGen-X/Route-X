package admin

import (
	"context"
	"net/http"
	"strings"
	"testing"

	"github.com/NexGen-X/Route-X/internal/database/repo/upstream"
	"github.com/NexGen-X/Route-X/internal/security"
	"github.com/NexGen-X/Route-X/internal/webhooks"
)

// FASE 11 B-273 (PB-28) + B-276 (PB-31).
//
// B-273: updateWebhook menelan secret yang disebut tetapi spasi-saja
// sebagai "tidak diubah" dan menjawab 200 — sementara createWebhook
// menolak nilai yang sama dengan 400 missing_secret. Operator yang
// bermaksud merotasi secret tidak pernah tahu niatnya gagal. Kini:
// field absen = tidak diubah (B-199 utuh); field disebut tetapi
// kosong/spasi = 400 missing_secret.
//
// B-276: detachProviderModel mengabaikan segmen {id} provider pada rute
// bersarang — pemetaan milik provider B bisa dihapus lewat URL provider
// A. Kini pemilik diverifikasi; ketidakcocokan menjawab 404 dan
// pemetaan tidak tersentuh.

func TestFase11B273SecretWebhookSpasiDitolak(t *testing.T) {
	env := setupTestEnv(t)
	ctx := context.Background()
	c := env.newClient(t)
	c.login("superadmin@routex.internal", testPassword)

	repoWebhook := webhooks.NewRepo(env.pool)
	wh, err := repoWebhook.Create(ctx, webhooks.CreateWebhookParams{
		Name: "webhook-b273", URL: "https://example.com/hooks",
		Events:     []string{"provider.unhealthy"},
		RawSecret:  "GANTI_rahasia_b273_awal",
		Cipher:     env.cipher,
		Enabled:    true,
		MaxRetries: 3,
		TimeoutMS:  10000,
		SSRFPolicy: security.SSRFPolicy{AllowHTTP: true, AllowPrivate: true},
	})
	if err != nil {
		t.Fatalf("buat webhook: %v", err)
	}
	awal, err := repoWebhook.Get(ctx, wh.ID)
	if err != nil {
		t.Fatalf("baca webhook: %v", err)
	}

	// Secret spasi-saja: ditolak 400 missing_secret, secret tak berubah.
	res, body := c.do(http.MethodPut, "/api/admin/automation/webhooks/"+wh.ID,
		map[string]any{"secret": "   "}, true)
	if res.StatusCode != http.StatusBadRequest {
		t.Fatalf("PUT secret spasi: status %d, mau 400 (body %s)", res.StatusCode, body)
	}
	if !strings.Contains(string(body), "missing_secret") {
		t.Fatalf("body tidak memuat missing_secret: %s", body)
	}
	kini, err := repoWebhook.Get(ctx, wh.ID)
	if err != nil {
		t.Fatalf("baca webhook sesudah penolakan: %v", err)
	}
	if kini.SecretCiphertext != awal.SecretCiphertext || kini.MaskedHint != awal.MaskedHint {
		t.Error("penolakan 400 mengubah secret tersimpan")
	}

	// Kontrol: field absen tetap berarti tidak diubah (200, B-199 utuh).
	res, body = c.do(http.MethodPut, "/api/admin/automation/webhooks/"+wh.ID,
		map[string]any{"name": "webhook-b273-baru"}, true)
	if res.StatusCode != http.StatusOK {
		t.Fatalf("PUT tanpa secret: status %d, mau 200 (body %s)", res.StatusCode, body)
	}
	kini, err = repoWebhook.Get(ctx, wh.ID)
	if err != nil {
		t.Fatalf("baca webhook sesudah PUT nama: %v", err)
	}
	if kini.SecretCiphertext != awal.SecretCiphertext {
		t.Error("PUT tanpa field secret mengubah secret tersimpan")
	}
}

func TestFase11B276DetachVerifikasiPemilik(t *testing.T) {
	env := setupTestEnv(t)
	ctx := context.Background()
	c := env.newClient(t)
	c.login("superadmin@routex.internal", testPassword)

	provRepo := upstream.NewProviderRepo(env.pool)
	buatProvider := func(nama string) *upstream.Provider {
		t.Helper()
		p, err := provRepo.Create(ctx, upstream.CreateProviderParams{
			Name: nama, DisplayName: nama, Kind: "openai_compatible",
			BaseURL: "https://upstream.contoh.test/v1",
		})
		if err != nil {
			t.Fatalf("buat provider %s: %v", nama, err)
		}
		return p
	}
	provA := buatProvider("prov-b276-a")
	provB := buatProvider("prov-b276-b")

	modelRepo := upstream.NewModelRepo(env.pool)
	var modelID string
	if err := env.pool.QueryRow(ctx,
		`select id::text from models order by created_at limit 1`).Scan(&modelID); err != nil {
		t.Fatalf("ambil model katalog seed: %v", err)
	}
	pm, err := modelRepo.AttachProvider(ctx, upstream.AttachProviderParams{
		ModelID: modelID, ProviderID: provB.ID, UpstreamModelName: "model-milik-b",
	})
	if err != nil {
		t.Fatalf("AttachProvider: %v", err)
	}

	// Hapus lewat URL provider yang SALAH (A): 404, pemetaan tetap ada.
	res, body := c.do(http.MethodDelete,
		"/api/admin/upstreams/providers/"+provA.ID+"/models/"+pm.ID, nil, true)
	if res.StatusCode != http.StatusNotFound {
		t.Fatalf("detach lewat provider salah: status %d, mau 404 (body %s)", res.StatusCode, body)
	}
	if _, err := modelRepo.GetProviderModel(ctx, pm.ID); err != nil {
		t.Fatalf("pemetaan ikut terhapus lewat URL provider yang salah (B-276): %v", err)
	}

	// Kontrol: lewat URL pemilik yang benar (B): 200 dan pemetaan hilang.
	res, body = c.do(http.MethodDelete,
		"/api/admin/upstreams/providers/"+provB.ID+"/models/"+pm.ID, nil, true)
	if res.StatusCode != http.StatusOK {
		t.Fatalf("detach lewat pemilik benar: status %d, mau 200 (body %s)", res.StatusCode, body)
	}
	if _, err := modelRepo.GetProviderModel(ctx, pm.ID); err == nil {
		t.Fatal("pemetaan masih ada sesudah detach yang sah")
	}
}
