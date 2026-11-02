package admin

import (
	"context"
	"encoding/json"
	"net/http"
	"testing"
)

// TestAdminUpdateContentFilterMengosongkanCakupanModel mereproduksi lapis
// HTTP dari audit3 V-DT-25: mengirim "model_id": "" pada PUT harus benar-
// benar mengosongkan cakupan filter. Sebelum perbaikan, handler hanya
// meneruskan nilai non-kosong ke repo, jadi pengosongan ditelan diam-diam
// (HTTP 200, nilai lama bertahan) — dan repo sendiri menulis "" mentah ke
// kolom uuid. Field yang TIDAK dikirim harus tetap berarti "tidak diubah".
func TestAdminUpdateContentFilterMengosongkanCakupanModel(t *testing.T) {
	env := setupTestEnv(t)
	client := env.newClient(t)
	client.login("superadmin@routex.internal", testPassword)

	ctx := context.Background()
	var modelID string
	if err := env.pool.QueryRow(ctx, `
		insert into models (model_id, display_name) values ('model-uji-g11', 'Model Uji G11')
		returning id::text`).Scan(&modelID); err != nil {
		t.Fatalf("menyisipkan model: %v", err)
	}

	payloadDasar := map[string]any{
		"name":         "filter-cakupan-g11",
		"description":  "Filter uji cakupan grup 11",
		"kind":         "blocked_pattern",
		"priority":     50,
		"applies_to":   "request",
		"action":       "block",
		"pattern":      "pola_terlarang",
		"pattern_type": "substring",
		"enabled":      true,
	}

	denganModel := func() map[string]any {
		p := make(map[string]any, len(payloadDasar)+1)
		for k, v := range payloadDasar {
			p[k] = v
		}
		p["model_id"] = modelID
		return p
	}

	resCreate, bodyCreate := client.do(http.MethodPost, "/api/admin/gateway/content-filters", denganModel(), true)
	if resCreate.StatusCode != http.StatusCreated {
		t.Fatalf("create filter status = %d, body: %s", resCreate.StatusCode, string(bodyCreate))
	}
	var created struct {
		ID      string `json:"id"`
		ModelID string `json:"model_id"`
	}
	if err := json.Unmarshal(bodyCreate, &created); err != nil {
		t.Fatalf("unmarshal created filter: %v", err)
	}
	if created.ModelID != modelID {
		t.Fatalf("model_id saat dibuat = %q, mau %q", created.ModelID, modelID)
	}

	ambilModelID := func() string {
		t.Helper()
		res, body := client.do(http.MethodGet, "/api/admin/gateway/content-filters/"+created.ID, nil, false)
		if res.StatusCode != http.StatusOK {
			t.Fatalf("get filter status = %d, body: %s", res.StatusCode, string(body))
		}
		var got struct {
			ModelID string `json:"model_id"`
		}
		if err := json.Unmarshal(body, &got); err != nil {
			t.Fatalf("unmarshal get filter: %v", err)
		}
		return got.ModelID
	}

	// 1. PUT dengan "model_id": "" — cakupan harus kosong sesudahnya.
	tanpaModel := denganModel()
	tanpaModel["model_id"] = ""
	resUpd, bodyUpd := client.do(http.MethodPut, "/api/admin/gateway/content-filters/"+created.ID, tanpaModel, true)
	if resUpd.StatusCode != http.StatusOK {
		t.Fatalf("update filter (kosongkan) status = %d, body: %s", resUpd.StatusCode, string(bodyUpd))
	}
	if got := ambilModelID(); got != "" {
		t.Errorf("model_id sesudah dikosongkan = %q, mau kosong", got)
	}

	// 2. Pasang kembali, lalu PUT TANPA field model_id — nilai harus bertahan
	// (absen tidak sama dengan kosong).
	resUpd, _ = client.do(http.MethodPut, "/api/admin/gateway/content-filters/"+created.ID, denganModel(), true)
	if resUpd.StatusCode != http.StatusOK {
		t.Fatalf("update filter (pasang lagi) status = %d", resUpd.StatusCode)
	}
	resUpd, _ = client.do(http.MethodPut, "/api/admin/gateway/content-filters/"+created.ID, payloadDasar, true)
	if resUpd.StatusCode != http.StatusOK {
		t.Fatalf("update filter (tanpa field) status = %d", resUpd.StatusCode)
	}
	if got := ambilModelID(); got != modelID {
		t.Errorf("model_id sesudah PUT tanpa field = %q, mau tetap %q", got, modelID)
	}
}
