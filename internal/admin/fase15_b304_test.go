package admin

import (
	"encoding/json"
	"net/http"
	"testing"
)

// TestB304HapusAliasMenghormatiModel adalah pengunci B-304:
// removeModelAlias dulu mengabaikan segmen {id} model di URL — repo
// menghapus alias dari model MANA PUN yang memilikinya, dan audit
// tercatat atas model di URL. Kini alias hanya terhapus lewat URL
// model pemiliknya; lewat model lain → 404 dan alias utuh.
func TestB304HapusAliasMenghormatiModel(t *testing.T) {
	env := setupTestEnv(t)
	client := env.newClient(t)
	client.login("superadmin@routex.internal", testPassword)

	buatModel := func(modelID string) string {
		t.Helper()
		res, body := client.do(http.MethodPost, "/api/admin/upstreams/models", map[string]any{
			"model_id": modelID, "display_name": modelID,
		}, true)
		if res.StatusCode != http.StatusCreated {
			t.Fatalf("buat model %s: status = %d, body: %s", modelID, res.StatusCode, string(body))
		}
		var m struct {
			ID string `json:"id"`
		}
		if err := json.Unmarshal(body, &m); err != nil {
			t.Fatalf("urai model: %v", err)
		}
		return m.ID
	}
	idA := buatModel("model-a-b304")
	idB := buatModel("model-b-b304")

	res, body := client.do(http.MethodPost, "/api/admin/upstreams/models/"+idB+"/aliases", map[string]any{
		"alias": "alias-milik-b",
	}, true)
	if res.StatusCode != http.StatusCreated {
		t.Fatalf("tambah alias ke model B: status = %d, body: %s", res.StatusCode, string(body))
	}

	// Lewat URL model A: harus 404 dan alias tetap ada.
	res, body = client.do(http.MethodDelete, "/api/admin/upstreams/models/"+idA+"/aliases/alias-milik-b", nil, true)
	if res.StatusCode != http.StatusNotFound {
		t.Fatalf("DELETE alias milik B lewat URL A: status = %d, mau 404 (body: %s)", res.StatusCode, string(body))
	}
	res, body = client.do(http.MethodGet, "/api/admin/upstreams/models/"+idB, nil, false)
	if res.StatusCode != http.StatusOK {
		t.Fatalf("GET model B: status = %d, body: %s", res.StatusCode, string(body))
	}
	var gotB struct {
		Aliases []struct {
			Alias string `json:"alias"`
		} `json:"aliases"`
	}
	_ = json.Unmarshal(body, &gotB)
	ada := false
	for _, a := range gotB.Aliases {
		if a.Alias == "alias-milik-b" {
			ada = true
		}
	}
	if !ada {
		t.Fatalf("alias milik B hilang setelah DELETE lewat URL A — B-304 (body: %s)", string(body))
	}

	// Lewat URL model B: 200 dan alias hilang.
	res, body = client.do(http.MethodDelete, "/api/admin/upstreams/models/"+idB+"/aliases/alias-milik-b", nil, true)
	if res.StatusCode != http.StatusOK {
		t.Fatalf("DELETE alias lewat URL B: status = %d, mau 200 (body: %s)", res.StatusCode, string(body))
	}
	res, body = client.do(http.MethodGet, "/api/admin/upstreams/models/"+idB, nil, false)
	_ = json.Unmarshal(body, &gotB)
	for _, a := range gotB.Aliases {
		if a.Alias == "alias-milik-b" {
			t.Fatalf("alias masih ada setelah DELETE lewat URL pemiliknya (body: %s)", string(body))
		}
	}
}
