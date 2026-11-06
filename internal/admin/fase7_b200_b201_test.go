package admin

import (
	"context"
	"encoding/json"
	"net/http"
	"strings"
	"testing"
	"time"

	"github.com/NexGen-X/Route-X/internal/database/repo/upstream"
)

// Test B-201: slug model / nama provider yang tidak dikenal pada daftar
// putih API key ditolak 400 yang menyebut nilainya — di jalur buat
// maupun jalur set-allowed. Pra-fix entri itu mengalir mentah ke
// unnest(uuid[]) di repo dan meledak 22P02 yang terbaca 500.
func TestFase7B201DaftarPutihSlugTidakDikenal(t *testing.T) {
	env := setupTestEnv(t)
	ctx := context.Background()
	c := env.newClient(t)
	c.login("superadmin@routex.internal", testPassword)

	provRepo := upstream.NewProviderRepo(env.pool)
	if _, err := provRepo.Create(ctx, upstream.CreateProviderParams{
		Name: "penyedia-b201", DisplayName: "penyedia-b201", Kind: "openai_compatible",
		BaseURL: "https://upstream.test",
	}); err != nil {
		t.Fatalf("buat provider: %v", err)
	}
	modelRepo := upstream.NewModelRepo(env.pool)
	if _, err := modelRepo.Create(ctx, upstream.CreateModelParams{
		ModelID: "model-b201-x", DisplayName: "model-b201-x",
	}); err != nil {
		t.Fatalf("buat model: %v", err)
	}

	// Buat dengan slug model karangan → 400 invalid_model_ids menyebut nilainya.
	res, body := c.do(http.MethodPost, "/api/admin/access/api-keys", map[string]any{
		"name": "kunci-b201-a", "model_ids": []string{"model-karangan-b201"},
	}, true)
	if res.StatusCode != http.StatusBadRequest ||
		!strings.Contains(string(body), "invalid_model_ids") ||
		!strings.Contains(string(body), "model-karangan-b201") {
		t.Fatalf("buat key slug model karangan: status %d, body %s", res.StatusCode, body)
	}

	// Buat dengan nama provider karangan → 400 invalid_provider_ids.
	res, body = c.do(http.MethodPost, "/api/admin/access/api-keys", map[string]any{
		"name": "kunci-b201-b", "provider_ids": []string{"penyedia-karangan-b201"},
	}, true)
	if res.StatusCode != http.StatusBadRequest ||
		!strings.Contains(string(body), "invalid_provider_ids") ||
		!strings.Contains(string(body), "penyedia-karangan-b201") {
		t.Fatalf("buat key provider karangan: status %d, body %s", res.StatusCode, body)
	}

	// Buat dengan slug + nama yang nyata → 201.
	res, body = c.do(http.MethodPost, "/api/admin/access/api-keys", map[string]any{
		"name":         "kunci-b201-sah",
		"model_ids":    []string{"model-b201-x"},
		"provider_ids": []string{"penyedia-b201"},
	}, true)
	if res.StatusCode != http.StatusCreated {
		t.Fatalf("buat key sah: status %d, body %s", res.StatusCode, body)
	}
	var dibuat struct {
		Key struct {
			ID string `json:"id"`
		} `json:"key"`
	}
	if err := json.Unmarshal(body, &dibuat); err != nil || dibuat.Key.ID == "" {
		t.Fatalf("urai respons buat key: %v, body %s", err, body)
	}

	// Set-allowed dengan slug karangan → 400 yang sama (bukan 500).
	res, body = c.do(http.MethodPost, "/api/admin/access/api-keys/"+dibuat.Key.ID+"/allowed", map[string]any{
		"model_ids": []string{"model-karangan-lain"},
	}, true)
	if res.StatusCode != http.StatusBadRequest ||
		!strings.Contains(string(body), "invalid_model_ids") ||
		!strings.Contains(string(body), "model-karangan-lain") {
		t.Fatalf("set-allowed slug karangan: status %d, body %s", res.StatusCode, body)
	}

	// Set-allowed dengan slug nyata → 200.
	res, body = c.do(http.MethodPost, "/api/admin/access/api-keys/"+dibuat.Key.ID+"/allowed", map[string]any{
		"model_ids":    []string{"model-b201-x"},
		"provider_ids": []string{"penyedia-b201"},
	}, true)
	if res.StatusCode != http.StatusOK {
		t.Fatalf("set-allowed sah: status %d, body %s", res.StatusCode, body)
	}
}

// Test B-200: createAPIKey menerapkan validasi bentuk yang sama dengan
// updateAPIKey — nama kosong, scope karangan, batas <= 0, expires_at
// lampau, dan pemilik tidak ada/nonaktif semuanya 400 berkode sebelum
// menyentuh database. Pra-fix jalur buat menerima semuanya (meledak di
// constraint DB sebagai 500 atau tersimpan cacat).
func TestFase7B200ValidasiCreateAPIKey(t *testing.T) {
	env := setupTestEnv(t)
	ctx := context.Background()
	c := env.newClient(t)
	c.login("superadmin@routex.internal", testPassword)

	// Dua pengguna uji: satu aktif, satu nonaktif.
	buatPengguna := func(email, status string) string {
		t.Helper()
		var id string
		err := env.pool.QueryRow(ctx, `
			insert into users (email, password_hash, status)
			values ($1, '$argon2id$v=19$m=8192,t=1,p=1$c2FsdHNhbHRzYWx0c2E$aGFzaGhhc2hoYXNoaGFzaGhhc2hoYXNoaGE', $2)
			returning id::text`, email, status).Scan(&id)
		if err != nil {
			t.Fatalf("buat pengguna %s: %v", email, err)
		}
		return id
	}
	aktifID := buatPengguna("pemilik-aktif-b200@contoh.test", "active")
	nonaktifID := buatPengguna("pemilik-nonaktif-b200@contoh.test", "disabled")

	kemarin := time.Now().Add(-24 * time.Hour).UTC().Format(time.RFC3339)
	kasus := []struct {
		nama string
		body map[string]any
		kode string
	}{
		{"nama kosong", map[string]any{}, "invalid_name"},
		{"scope karangan", map[string]any{"name": "k-scope", "scopes": []string{"terbang"}}, "invalid_scopes"},
		{"batas nol", map[string]any{"name": "k-batas", "daily_request_limit": 0}, "invalid_limit"},
		{"batas negatif", map[string]any{"name": "k-batas-neg", "rate_limit_rpm": -5}, "invalid_limit"},
		{"expires lampau", map[string]any{"name": "k-exp", "expires_at": kemarin}, "expires_in_past"},
		{"pemilik tidak ada", map[string]any{"name": "k-pemilik", "owner_user_id": "018f0000-0000-7000-8000-000000000000"}, "invalid_owner_user_id"},
		{"pemilik nonaktif", map[string]any{"name": "k-pemilik-nonaktif", "owner_user_id": nonaktifID}, "invalid_owner_user_id"},
	}
	for _, k := range kasus {
		t.Run(k.nama, func(t *testing.T) {
			res, body := c.do(http.MethodPost, "/api/admin/access/api-keys", k.body, true)
			if res.StatusCode != http.StatusBadRequest || !strings.Contains(string(body), k.kode) {
				t.Fatalf("status %d, body %s; mau 400 berkode %s", res.StatusCode, body, k.kode)
			}
		})
	}

	// Kontrol positif: pemilik aktif yang nyata diterima.
	res, body := c.do(http.MethodPost, "/api/admin/access/api-keys", map[string]any{
		"name": "kunci-b200-sah", "owner_user_id": aktifID,
		"scopes": []string{"inference"}, "daily_request_limit": 100,
		"expires_at": time.Now().Add(24 * time.Hour).UTC().Format(time.RFC3339),
	}, true)
	if res.StatusCode != http.StatusCreated {
		t.Fatalf("buat key sah dengan pemilik aktif: status %d, body %s", res.StatusCode, body)
	}
}
