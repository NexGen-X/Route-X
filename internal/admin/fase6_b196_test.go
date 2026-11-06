package admin

import (
	"context"
	"encoding/json"
	"net/http"
	"strings"
	"testing"

	"github.com/NexGen-X/Route-X/internal/database/repo/keys"
	"github.com/NexGen-X/Route-X/internal/database/repo/upstream"
)

// TestB196ScopeKebijakanDiselesaikanDiBatasTulis adalah pengunci
// B-196: normalisasi scope_id dulu hanya berlaku untuk cakupan model;
// api_key/user/provider/ip lolos mentah sehingga aturan lahir mati
// (penegak mencocokkan persis UUID/IP kanonik) dan pelanggaran
// constraint kebijakan jatuh ke pesan generik. Kini semua cakupan
// diselesaikan saat dibuat, dan constraint yang tersisa menyebut
// field-nya.
func TestB196ScopeKebijakanDiselesaikanDiBatasTulis(t *testing.T) {
	env := setupTestEnv(t)
	ctx := context.Background()
	client := env.newClient(t)
	client.login("superadmin@routex.internal", testPassword)

	// Entitas nyata untuk ditunjuk.
	keyRepo, err := keys.New(env.pool, []byte("pepper-uji-32-byte-harus-cukup!"))
	if err != nil {
		t.Fatalf("repo kunci: %v", err)
	}
	kunci, err := keyRepo.Create(ctx, keys.CreateParams{
		Name: "kunci-b196", OwnerUserID: env.viewerUser.ID,
	})
	if err != nil {
		t.Fatalf("buat kunci: %v", err)
	}
	provRepo := upstream.NewProviderRepo(env.pool)
	prov, err := provRepo.Create(ctx, upstream.CreateProviderParams{
		Name: "b196-provider", DisplayName: "B196", Kind: "openai", BaseURL: "https://api.openai.com",
	})
	if err != nil {
		t.Fatalf("buat provider: %v", err)
	}

	buatRL := func(body map[string]any) (int, []byte) {
		res, raw := client.do(http.MethodPost, "/api/admin/gateway/rate-limits/", body, true)
		return res.StatusCode, raw
	}
	buatBudget := func(body map[string]any) (int, []byte) {
		res, raw := client.do(http.MethodPost, "/api/admin/gateway/budgets/", body, true)
		return res.StatusCode, raw
	}

	// --- Cakupan sah: tersimpan dengan scope_id yang bisa ditegakkan ---
	sah := []struct {
		nama    string
		scope   string
		scopeID string
		mauID   string
	}{
		{"api_key nyata", "api_key", kunci.Key.ID, kunci.Key.ID},
		{"user nyata", "user", env.viewerUser.ID, env.viewerUser.ID},
		{"provider nyata", "provider", prov.ID, prov.ID},
		{"ip v6 tak kanonik dikanonikkan", "ip", "2001:0DB8:0000:0000:0000:FF00:0042:8329", "2001:db8::ff00:42:8329"},
	}
	for _, tc := range sah {
		st, raw := buatRL(map[string]any{
			"scope": tc.scope, "scope_id": tc.scopeID, "requests_per_minute": 10,
		})
		if st != http.StatusCreated {
			t.Errorf("%s: status = %d, mau 201 (body %s)", tc.nama, st, raw)
			continue
		}
		var dto RateLimitDTO
		if err := json.Unmarshal(raw, &dto); err != nil {
			t.Fatalf("%s: urai DTO: %v", tc.nama, err)
		}
		if dto.ScopeID != tc.mauID {
			t.Errorf("%s: scope_id tersimpan = %q, mau %q", tc.nama, dto.ScopeID, tc.mauID)
		}
	}

	// --- Cakupan tak sah: ditolak 400 invalid_scope_id, bukan aturan mati ---
	takSah := []struct {
		nama    string
		scope   string
		scopeID string
	}{
		{"api_key bukan uuid", "api_key", "bukan-uuid-sama-sekali"},
		{"api_key uuid hantu", "api_key", "00000000-0000-0000-0000-0000000000ff"},
		{"user hantu", "user", "00000000-0000-0000-0000-0000000000ff"},
		{"provider hantu", "provider", "00000000-0000-0000-0000-0000000000ff"},
		{"ip bukan alamat", "ip", "semarang"},
	}
	for _, tc := range takSah {
		st, raw := buatRL(map[string]any{
			"scope": tc.scope, "scope_id": tc.scopeID, "requests_per_minute": 10,
		})
		if st != http.StatusBadRequest {
			t.Errorf("%s: status = %d, mau 400 — aturan mati tersimpan (body %s)", tc.nama, st, raw)
			continue
		}
		if !strings.Contains(string(raw), "invalid_scope_id") {
			t.Errorf("%s: kode error = %s, mau invalid_scope_id", tc.nama, raw)
		}
	}

	// Anggaran berbagi validator yang sama.
	st, raw := buatBudget(map[string]any{
		"name": "pagu user hantu", "scope": "user",
		"scope_id": "00000000-0000-0000-0000-0000000000ff",
		"period":   "daily", "limit_usd": "5",
	})
	if st != http.StatusBadRequest || !strings.Contains(string(raw), "invalid_scope_id") {
		t.Errorf("anggaran user hantu: status = %d, mau 400 invalid_scope_id (body %s)", st, raw)
	}
	st, raw = buatBudget(map[string]any{
		"name": "pagu user nyata", "scope": "user", "scope_id": env.viewerUser.ID,
		"period": "daily", "limit_usd": "5",
	})
	if st != http.StatusCreated {
		t.Errorf("anggaran user nyata: status = %d, mau 201 (body %s)", st, raw)
	}

	// --- Constraint yang tersisa menyebut field-nya, bukan generik ---
	pesanField := []struct {
		nama string
		buat func() (int, []byte)
		mau  string
	}{
		{"scope batas laju asing", func() (int, []byte) {
			return buatRL(map[string]any{"scope": "benua", "scope_id": "x", "requests_per_minute": 1})
		}, "scope"},
		{"batas laju tanpa satu pun batas", func() (int, []byte) {
			return buatRL(map[string]any{"scope": "global"})
		}, "minimal satu batas"},
		{"period anggaran asing", func() (int, []byte) {
			return buatBudget(map[string]any{
				"name": "pagu period asing", "scope": "global",
				"period": "jam-jaman", "limit_usd": "5",
			})
		}, "period"},
		{"aksi anggaran asing", func() (int, []byte) {
			return buatBudget(map[string]any{
				"name": "pagu aksi asing", "scope": "global", "period": "daily",
				"limit_usd": "5", "action_on_exceed": "ledakkan",
			})
		}, "action_on_exceed"},
		{"nama anggaran kosong", func() (int, []byte) {
			return buatBudget(map[string]any{
				"name": "   ", "scope": "global", "period": "weekly", "limit_usd": "5",
			})
		}, "name"},
	}
	for _, tc := range pesanField {
		st, raw := tc.buat()
		if st != http.StatusBadRequest {
			t.Errorf("%s: status = %d, mau 400 (body %s)", tc.nama, st, raw)
			continue
		}
		if !strings.Contains(string(raw), tc.mau) {
			t.Errorf("%s: pesan = %s, mau menyebut %q (bukan generik)", tc.nama, raw, tc.mau)
		}
		if strings.Contains(string(raw), "nilai input melanggar batasan") {
			t.Errorf("%s: pesan masih generik: %s", tc.nama, raw)
		}
	}
}
