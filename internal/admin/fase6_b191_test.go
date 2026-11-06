package admin

import (
	"context"
	"fmt"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/NexGen-X/Route-X/internal/database/repo"
	"github.com/NexGen-X/Route-X/internal/database/repo/traffic"
	"github.com/NexGen-X/Route-X/internal/database/repo/upstream"
	"github.com/NexGen-X/Route-X/internal/gateway"
	"github.com/NexGen-X/Route-X/internal/security"
)

// TestB191UjiModelMenulisJejakRequests adalah pengunci B-191: uji model
// provider mengonsumsi token upstream sungguhan tetapi dulu tidak
// meninggalkan jejak akuntansi apa pun — respons dibuang, tidak ada
// baris requests, tidak ada catatan pemakaian. Kini setiap panggilan
// uji (sukses maupun gagal) menulis SATU baris requests diagnostik
// yang endpoint-nya adalah pola rute uji-model itu sendiri, lengkap
// dengan token dari respons upstream.
func TestB191UjiModelMenulisJejakRequests(t *testing.T) {
	env := setupTestEnv(t)
	ctx := context.Background()
	c := env.newClient(t)
	c.login("superadmin@routex.internal", testPassword)

	prefiks, err := security.ParsePrivateAddrs([]string{"127.0.0.1", "::1"})
	if err != nil {
		t.Fatalf("ParsePrivateAddrs: %v", err)
	}
	provRepo := upstream.NewProviderRepo(env.pool)
	credRepo, err := upstream.NewCredentialRepo(env.pool, env.cipher)
	if err != nil {
		t.Fatalf("buat repo kredensial: %v", err)
	}
	// Factory wajib membawa sumber kredensial sungguhan: ProviderFor
	// menolak provider berkredensial bila pabrik dibuat tanpa sumber.
	env.handlers.factory = gateway.NewFactory(credRepo, nil,
		security.SSRFPolicy{AllowHTTP: true, AllowedPrivateAddrs: prefiks},
		slog.New(slog.DiscardHandler))
	buatProvider := func(nama, base string) *upstream.Provider {
		t.Helper()
		prov, err := provRepo.Create(ctx, upstream.CreateProviderParams{
			Name: nama, DisplayName: nama, Kind: "openai", BaseURL: base,
		})
		if err != nil {
			t.Fatalf("buat provider %s: %v", nama, err)
		}
		if _, err := credRepo.Create(ctx, upstream.CreateCredentialParams{
			ProviderID: prov.ID, Label: "kunci-uji", Secret: security.Secret("GANTI_kunci_uji_b191"),
		}); err != nil {
			t.Fatalf("buat kredensial %s: %v", nama, err)
		}
		return prov
	}

	barisUntuk := func(providerID string) []traffic.Row {
		t.Helper()
		res, err := env.handlers.trafficRepo.List(ctx,
			traffic.Filter{ProviderID: providerID}, repo.Page{Limit: 50})
		if err != nil {
			t.Fatalf("List jejak provider %s: %v", providerID, err)
		}
		return res.Rows
	}

	// Sukses: upstream menjawab chat completion ber-usage.
	srvOK := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		_, _ = fmt.Fprint(w, `{"id":"chatcmpl-uji","object":"chat.completion","created":1,"model":"model-uji","choices":[{"index":0,"message":{"role":"assistant","content":"pong"},"finish_reason":"stop"}],"usage":{"prompt_tokens":7,"completion_tokens":1,"total_tokens":8}}`)
	}))
	defer srvOK.Close()
	provOK := buatProvider("b191-ok", srvOK.URL)

	res, raw := c.do(http.MethodPost, "/api/admin/upstreams/providers/"+provOK.ID+"/test-model",
		map[string]any{"model": "model-uji"}, true)
	if res.StatusCode != http.StatusOK {
		t.Fatalf("uji model sukses: status %d (body %s)", res.StatusCode, raw)
	}
	baris := barisUntuk(provOK.ID)
	if len(baris) != 1 {
		t.Fatalf("jejak uji model sukses = %d baris, mau tepat 1", len(baris))
	}
	b := baris[0]
	if b.StatusCode != 200 {
		t.Errorf("status baris jejak = %d, mau 200", b.StatusCode)
	}
	if b.InputTokens != 7 || b.OutputTokens != 1 {
		t.Errorf("token baris jejak = %d/%d, mau 7/1 dari usage upstream", b.InputTokens, b.OutputTokens)
	}
	if b.Endpoint != "/upstreams/providers/{id}/test-model" {
		t.Errorf("endpoint baris jejak = %q, mau pola rute uji-model sebagai penanda diagnostik", b.Endpoint)
	}
	if !b.Unpriced {
		t.Errorf("baris jejak bertoken tidak tertandai Unpriced — pemakaian uji tidak pernah dihargai (B-190)")
	}

	// Gagal: upstream menjawab 500 — jejaknya baris error, bukan hilang.
	srvGagal := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		http.Error(w, `{"error":{"message":"mesin upstream rusak"}}`, http.StatusInternalServerError)
	}))
	defer srvGagal.Close()
	provGagal := buatProvider("b191-gagal", srvGagal.URL)

	res, raw = c.do(http.MethodPost, "/api/admin/upstreams/providers/"+provGagal.ID+"/test-model",
		map[string]any{"model": "model-uji"}, true)
	if res.StatusCode != http.StatusOK {
		t.Fatalf("uji model gagal tetap menjawab 200 berisi status error: status %d (body %s)", res.StatusCode, raw)
	}
	baris = barisUntuk(provGagal.ID)
	if len(baris) != 1 {
		t.Fatalf("jejak uji model gagal = %d baris, mau tepat 1", len(baris))
	}
	if baris[0].StatusCode != 500 {
		t.Errorf("status baris jejak gagal = %d, mau 500 dari upstream", baris[0].StatusCode)
	}
	if baris[0].ErrorType == "" {
		t.Errorf("baris jejak gagal tanpa error_type")
	}
}
