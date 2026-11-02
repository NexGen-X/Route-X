package admin

import (
	"context"
	"encoding/json"
	"net/http"
	"testing"

	"github.com/NexGen-X/Route-X/internal/database/repo/upstream"
)

// TestAdminAnggaranDanBatasLajuScopeModelMenyimpanUUID membuktikan perbaikan
// audit3 V-AB-01, V-AB-02, dan V-AB-03 pada jalur API admin:
//
//   - Anggaran scope model yang dibuat dengan scope_id berupa NAMA model tersimpan
//     sebagai UUID model kanonik — bentuk yang dicocokkan jalur request — bukan
//     nama yang tidak pernah bertemu target mana pun.
//   - Anggaran berkala lahir dengan period_end terisi, bukan NULL yang membuatnya
//     kebal terhadap BudgetResetterJob.
//   - Batas laju scope model mendapat normalisasi yang sama.
//   - Nama model yang tidak dikenal ditolak 400, bukan tersimpan sebagai aturan mati.
func TestAdminAnggaranDanBatasLajuScopeModelMenyimpanUUID(t *testing.T) {
	env := setupTestEnv(t)
	ctx := context.Background()

	modelRepo := upstream.NewModelRepo(env.pool)
	m, err := modelRepo.Create(ctx, upstream.CreateModelParams{
		ModelID:     "model-audit3",
		DisplayName: "Model Audit3",
	})
	if err != nil {
		t.Fatalf("buat model uji: %v", err)
	}

	client := env.newClient(t)
	client.login("superadmin@routex.internal", testPassword)

	// Anggaran scope model dengan scope_id = nama model.
	res, body := client.do(http.MethodPost, "/api/admin/gateway/budgets/", map[string]any{
		"name": "pagu model audit3", "scope": "model", "scope_id": "model-audit3",
		"period": "daily", "limit_usd": "5",
	}, true)
	if res.StatusCode != http.StatusCreated {
		t.Fatalf("buat anggaran: status = %d, mau 201 (body: %s)", res.StatusCode, body)
	}
	var dto BudgetDTO
	if err := json.Unmarshal(body, &dto); err != nil {
		t.Fatalf("urai DTO anggaran: %v", err)
	}
	if dto.ScopeID != m.ID {
		t.Errorf("scope_id anggaran = %q, mau UUID model %q", dto.ScopeID, m.ID)
	}
	if dto.PeriodEnd == nil {
		t.Error("period_end anggaran daily = nil, mau terisi sejak pembuatan")
	}

	// Batas laju scope model dengan scope_id = nama model.
	res, body = client.do(http.MethodPost, "/api/admin/gateway/rate-limits/", map[string]any{
		"scope": "model", "scope_id": "model-audit3", "tokens_per_minute": 100,
	}, true)
	if res.StatusCode != http.StatusCreated {
		t.Fatalf("buat batas laju: status = %d, mau 201 (body: %s)", res.StatusCode, body)
	}
	var rl RateLimitDTO
	if err := json.Unmarshal(body, &rl); err != nil {
		t.Fatalf("urai DTO batas laju: %v", err)
	}
	if rl.ScopeID != m.ID {
		t.Errorf("scope_id batas laju = %q, mau UUID model %q", rl.ScopeID, m.ID)
	}

	// Nama model yang tidak dikenal → 400, bukan aturan mati yang tersimpan.
	res, body = client.do(http.MethodPost, "/api/admin/gateway/budgets/", map[string]any{
		"name": "pagu model hantu", "scope": "model", "scope_id": "model-tidak-ada",
		"period": "daily", "limit_usd": "5",
	}, true)
	if res.StatusCode != http.StatusBadRequest {
		t.Errorf("anggaran model tak dikenal: status = %d, mau 400 (body: %s)", res.StatusCode, body)
	}
}
