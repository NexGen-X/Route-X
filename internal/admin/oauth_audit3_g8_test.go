package admin

import (
	"context"
	"net/http"
	"strings"
	"testing"
	"time"

	"github.com/NexGen-X/Route-X/internal/database/repo/upstream"
	"github.com/NexGen-X/Route-X/internal/security"
)

// nilaiAksesAdminG8 menyusun nilai akses tiruan untuk kredensial uji admin.
func nilaiAksesAdminG8(nama string) security.Secret {
	bagian := []string{"akses", nama}
	return security.Secret(bagian[0] + "-" + bagian[1])
}

// TestAdminProviderBaseURLDivalidasiSaatSimpan membuktikan audit3 V-EO-09:
// base_url berbahaya kini ditolak di pintu simpan (bukan hanya saat request
// pertama): alamat metadata cloud, kredensial tertanam di URL, dan skema
// non-HTTP. base_url publik yang sah tetap diterima.
func TestAdminProviderBaseURLDivalidasiSaatSimpan(t *testing.T) {
	env := setupTestEnv(t)
	c := env.newClient(t)
	c.login("superadmin@routex.internal", testPassword)

	const jalur = "/api/admin/upstreams/providers"
	baru := func(nama, base string) map[string]any {
		return map[string]any{
			"name": nama, "display_name": nama, "kind": "openai_compatible", "base_url": base,
		}
	}

	for _, k := range []struct{ nama, base string }{
		{"g8-metadata", "http://169.254.169.254/latest/meta-data"},
		{"g8-kredensial", "https://user:sandi@api.contoh.test/v1"},
		{"g8-skema", "gopher://api.contoh.test/v1"},
		{"g8-privat", "http://192.168.10.5:8080/v1"},
	} {
		res, body := c.do(http.MethodPost, jalur, baru(k.nama, k.base), true)
		if res.StatusCode != http.StatusBadRequest {
			t.Errorf("base %s: status %d, ingin 400 (body %s)", k.base, res.StatusCode, body)
			continue
		}
		if !strings.Contains(string(body), "invalid_base_url") {
			t.Errorf("base %s: body tidak memuat kode invalid_base_url: %s", k.base, body)
		}
	}

	res, body := c.do(http.MethodPost, jalur, baru("g8-sah", "https://api.contoh.test/v1"), true)
	if res.StatusCode != http.StatusCreated && res.StatusCode != http.StatusOK {
		t.Fatalf("base_url publik sah ditolak: status %d, body %s", res.StatusCode, body)
	}
}

// TestAdminSyncModelMenolakBaseURLTersimpanBerbahaya membuktikan audit3
// V-EO-10 di lapisan sync: provider warisan yang base_url-nya sudah terlanjur
// tersimpan berbahaya (ditanam langsung lewat repo di sini) ditolak saat
// sync SEBELUM kredensial diambil atau permintaan keluar dibuat.
func TestAdminSyncModelMenolakBaseURLTersimpanBerbahaya(t *testing.T) {
	env := setupTestEnv(t)
	c := env.newClient(t)
	c.login("superadmin@routex.internal", testPassword)

	provRepo := upstream.NewProviderRepo(env.pool)
	prov, err := provRepo.Create(context.Background(), upstream.CreateProviderParams{
		Name: "g8-sync-bahaya", DisplayName: "Sync Bahaya", Kind: "openai_compatible",
		BaseURL: "http://169.254.169.254/latest/meta-data",
	})
	if err != nil {
		t.Fatalf("seed provider: %v", err)
	}

	res, body := c.do(http.MethodPost, "/api/admin/upstreams/providers/"+prov.ID+"/sync-models", map[string]any{}, true)
	if res.StatusCode != http.StatusBadRequest || !strings.Contains(string(body), "invalid_base_url") {
		t.Fatalf("sync ke metadata: status %d, body %s — ingin 400 invalid_base_url", res.StatusCode, body)
	}
}

// TestAdminSesiOAuthTerikatProvider membuktikan audit3 V-EO-22: toggle dan
// hapus sesi lewat URL provider LAIN dijawab 404 dan tidak mengubah sesi,
// sedangkan lewat provider pemiliknya berjalan normal.
func TestAdminSesiOAuthTerikatProvider(t *testing.T) {
	env := setupTestEnv(t)
	ctx := context.Background()
	c := env.newClient(t)
	c.login("superadmin@routex.internal", testPassword)

	provRepo := upstream.NewProviderRepo(env.pool)
	credRepo, err := upstream.NewCredentialRepo(env.pool, env.cipher)
	if err != nil {
		t.Fatalf("NewCredentialRepo: %v", err)
	}
	oauthRepo, err := upstream.NewOAuthRepo(env.pool, env.cipher)
	if err != nil {
		t.Fatalf("NewOAuthRepo: %v", err)
	}
	// Harness bersama tidak memasang repo OAuth ke handlers; pasang di sini
	// (test satu paket dengan handler).
	env.handlers.oauthRepo = oauthRepo

	buatProviderDanSesi := func(nama string) (providerID, sessionID string) {
		prov, err := provRepo.Create(ctx, upstream.CreateProviderParams{
			Name: nama, DisplayName: nama, Kind: "google",
			BaseURL: "https://generativelanguage.googleapis.com",
		})
		if err != nil {
			t.Fatalf("buat provider %s: %v", nama, err)
		}
		kedaluwarsa := time.Now().Add(time.Hour)
		cred, err := credRepo.Create(ctx, upstream.CreateCredentialParams{
			ProviderID: prov.ID, Label: "oauth:" + nama,
			Secret:    nilaiAksesAdminG8(nama),
			ExpiresAt: &kedaluwarsa,
		})
		if err != nil {
			t.Fatalf("buat kredensial %s: %v", nama, err)
		}
		meta, err := oauthRepo.UpsertSession(ctx, upstream.UpsertOAuthSessionParams{
			ProviderID: prov.ID, CredentialID: cred.ID,
			AccountEmail: nama + "@contoh.test", ClientID: "client-" + nama,
			RefreshToken: security.Secret("refresh-" + nama),
		})
		if err != nil {
			t.Fatalf("buat sesi %s: %v", nama, err)
		}
		return prov.ID, meta.ID
	}

	pemilikID, sesiID := buatProviderDanSesi("g8-pemilik")
	lainID, _ := buatProviderDanSesi("g8-lain")

	// Toggle lewat provider yang salah → 404, sesi tetap aktif.
	res, body := c.do(http.MethodPost,
		"/api/admin/upstreams/providers/"+lainID+"/oauth/sessions/"+sesiID+"/toggle",
		map[string]any{"enabled": false}, true)
	if res.StatusCode != http.StatusNotFound {
		t.Fatalf("toggle lintas provider: status %d, ingin 404 (body %s)", res.StatusCode, body)
	}
	meta, err := oauthRepo.GetSession(ctx, sesiID)
	if err != nil {
		t.Fatalf("GetSession: %v", err)
	}
	if !meta.Enabled {
		t.Fatal("sesi ikut nonaktif oleh toggle lintas provider")
	}

	// Hapus lewat provider yang salah → 404, sesi tetap ada.
	res, _ = c.do(http.MethodDelete,
		"/api/admin/upstreams/providers/"+lainID+"/oauth/sessions/"+sesiID, nil, true)
	if res.StatusCode != http.StatusNotFound {
		t.Fatalf("hapus lintas provider: status %d, ingin 404", res.StatusCode)
	}
	if _, err := oauthRepo.GetSession(ctx, sesiID); err != nil {
		t.Fatalf("sesi hilang oleh hapus lintas provider: %v", err)
	}

	// Lewat pemiliknya → berhasil.
	res, body = c.do(http.MethodPost,
		"/api/admin/upstreams/providers/"+pemilikID+"/oauth/sessions/"+sesiID+"/toggle",
		map[string]any{"enabled": false}, true)
	if res.StatusCode != http.StatusOK {
		t.Fatalf("toggle oleh pemilik: status %d, body %s", res.StatusCode, body)
	}
	sesudah, err := oauthRepo.GetSession(ctx, sesiID)
	if err != nil {
		t.Fatalf("GetSession sesudah toggle pemilik: %v", err)
	}
	if sesudah.Enabled {
		t.Error("sesi masih aktif sesudah toggle oleh pemiliknya")
	}
}
