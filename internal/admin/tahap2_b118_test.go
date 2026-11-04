package admin

import (
	"context"
	"net/http"
	"strings"
	"testing"

	"github.com/NexGen-X/Route-X/internal/database/repo/upstream"
	"github.com/NexGen-X/Route-X/internal/security"
)

// TestSyncModelsKredensialNonaktifGagalEksplisit membuktikan B-118:
// syncProviderModels versi lama menelan kegagalan mengambil kredensial
// aktif dan meneruskan discovery TANPA API key — pola senyap sekeluarga
// B-103. Provider yang PUNYA kredensial tetapi semuanya nonaktif kini
// menjawab 409 provider_credentials_inactive, sedangkan provider yang
// memang tidak pernah punya kredensial tetap boleh sync mode lokal
// (kegagalannya, bila ada, adalah kegagalan upstream — bukan 409 ini).
func TestSyncModelsKredensialNonaktifGagalEksplisit(t *testing.T) {
	env := setupTestEnv(t)
	ctx := context.Background()
	c := env.newClient(t)
	c.login("superadmin@routex.internal", testPassword)

	provRepo := upstream.NewProviderRepo(env.pool)
	credRepo, err := upstream.NewCredentialRepo(env.pool, env.cipher)
	if err != nil {
		t.Fatalf("NewCredentialRepo: %v", err)
	}

	buatProvider := func(nama string) *upstream.Provider {
		prov, err := provRepo.Create(ctx, upstream.CreateProviderParams{
			Name: nama, DisplayName: nama, Kind: "openai_compatible",
			BaseURL: "https://api.contoh.test/v1",
		})
		if err != nil {
			t.Fatalf("buat provider %s: %v", nama, err)
		}
		return prov
	}

	// Provider dengan kredensial yang dinonaktifkan operator.
	nilaiKredensial := func() security.Secret {
		bagian := []string{"GANTI", "kunci", "uji", "b118"}
		return security.Secret(strings.Join(bagian, "_"))
	}
	prov := buatProvider("b118-nonaktif")
	cred, err := credRepo.Create(ctx, upstream.CreateCredentialParams{
		ProviderID: prov.ID, Label: "kunci-b118",
		Secret: nilaiKredensial(),
	})
	if err != nil {
		t.Fatalf("buat kredensial: %v", err)
	}
	if _, err := credRepo.SetEnabled(ctx, cred.ID, false); err != nil {
		t.Fatalf("nonaktifkan kredensial: %v", err)
	}

	res, body := c.do(http.MethodPost, "/api/admin/upstreams/providers/"+prov.ID+"/sync-models", map[string]any{}, true)
	if res.StatusCode != http.StatusConflict {
		t.Fatalf("sync berkredensial nonaktif: status %d, mau 409 (body %s)", res.StatusCode, body)
	}
	if !strings.Contains(string(body), "provider_credentials_inactive") {
		t.Errorf("body tidak memuat kode provider_credentials_inactive: %s", body)
	}

	// Provider tanpa kredensial sama sekali: BUKAN 409 — mode lokal sah,
	// sync diteruskan (hasilnya kegagalan upstream 502 di lingkungan uji
	// tanpa jaringan, atau katalog fallback; yang dibuktikan di sini
	// hanya bahwa penolakan kredensial tidak menimpanya).
	provLokal := buatProvider("b118-tanpa-kredensial")
	res, body = c.do(http.MethodPost, "/api/admin/upstreams/providers/"+provLokal.ID+"/sync-models", map[string]any{}, true)
	if res.StatusCode == http.StatusConflict || strings.Contains(string(body), "provider_credentials_inactive") {
		t.Errorf("sync tanpa kredensial ikut ditolak 409: status %d, body %s", res.StatusCode, body)
	}
}
