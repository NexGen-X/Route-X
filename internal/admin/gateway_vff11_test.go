package admin

import (
	"encoding/json"
	"net/http"
	"strings"
	"testing"
)

// TestIkatanProviderWajibMemetakanModel menguji audit3 V-FF-11: provider
// yang diikat ke aturan routing wajib memetakan model target aturan itu
// (baris provider_models). Ikatan ke provider tanpa pemetaan adalah
// konfigurasi mati — kandidatnya tidak akan pernah terpilih untuk model
// tersebut — dan dulu diterima senyap di jalur buat, ubah, dan pasang
// provider. Kini ketiganya menolak 400 provider_tidak_memetakan_model.
func TestIkatanProviderWajibMemetakanModel(t *testing.T) {
	env := setupTestEnv(t)
	client := env.newClient(t)
	client.login("superadmin@routex.internal", testPassword)

	buatProvider := func(nama string) string {
		t.Helper()
		res, body := client.do(http.MethodPost, "/api/admin/upstreams/providers", map[string]any{
			"name":     nama,
			"kind":     "openai_compatible",
			"base_url": "https://contoh.test/v1",
		}, true)
		if res.StatusCode != http.StatusCreated {
			t.Fatalf("buat provider %s status = %d, body: %s", nama, res.StatusCode, string(body))
		}
		var prov struct {
			ID string `json:"id"`
		}
		if err := json.Unmarshal(body, &prov); err != nil || prov.ID == "" {
			t.Fatalf("urai provider %s: %v, body: %s", nama, err, string(body))
		}
		return prov.ID
	}

	provPunya := buatProvider("prov-punya-model")
	provPolos := buatProvider("prov-tanpa-model")

	// Petakan model kanonik ke provPunya saja.
	resMap, bodyMap := client.do(http.MethodPost, "/api/admin/upstreams/providers/"+provPunya+"/models", map[string]any{
		"name": "model-uji-vff11",
	}, true)
	if resMap.StatusCode != http.StatusCreated {
		t.Fatalf("petakan model status = %d, body: %s", resMap.StatusCode, string(bodyMap))
	}

	// Ambil ID model kanonik dari daftar model.
	resModels, bodyModels := client.do(http.MethodGet, "/api/admin/upstreams/models", nil, false)
	if resModels.StatusCode != http.StatusOK {
		t.Fatalf("daftar model status = %d, body: %s", resModels.StatusCode, string(bodyModels))
	}
	var daftar struct {
		Items []struct {
			ID      string `json:"id"`
			ModelID string `json:"model_id"`
		} `json:"items"`
	}
	if err := json.Unmarshal(bodyModels, &daftar); err != nil {
		t.Fatalf("urai daftar model: %v", err)
	}
	var modelID string
	for _, m := range daftar.Items {
		if m.ModelID == "model-uji-vff11" {
			modelID = m.ID
			break
		}
	}
	if modelID == "" {
		t.Fatalf("model-uji-vff11 tidak ditemukan di daftar model: %s", string(bodyModels))
	}

	// 1. Buat aturan mengikat provider TANPA pemetaan -> 400 menyebut nama.
	res, body := client.do(http.MethodPost, "/api/admin/gateway/routing-rules", map[string]any{
		"name":           "aturan-ikat-salah",
		"strategy":       "priority",
		"match_model_id": modelID,
		"provider_ids":   []string{provPolos},
	}, true)
	if res.StatusCode != http.StatusBadRequest {
		t.Fatalf("buat aturan ikat provider tanpa pemetaan status = %d, mau 400, body: %s", res.StatusCode, string(body))
	}
	if !strings.Contains(string(body), "provider_tidak_memetakan_model") {
		t.Errorf("respons tidak memuat kode provider_tidak_memetakan_model: %s", string(body))
	}
	if !strings.Contains(string(body), "prov-tanpa-model") {
		t.Errorf("respons tidak menyebut nama provider pelanggar: %s", string(body))
	}

	// 2. Buat aturan mengikat provider DENGAN pemetaan -> 201.
	res, body = client.do(http.MethodPost, "/api/admin/gateway/routing-rules", map[string]any{
		"name":           "aturan-ikat-benar",
		"strategy":       "priority",
		"match_model_id": modelID,
		"provider_ids":   []string{provPunya},
	}, true)
	if res.StatusCode != http.StatusCreated {
		t.Fatalf("buat aturan ikat provider berpetakan status = %d, mau 201, body: %s", res.StatusCode, string(body))
	}
	var aturan struct {
		ID string `json:"id"`
	}
	if err := json.Unmarshal(body, &aturan); err != nil || aturan.ID == "" {
		t.Fatalf("urai aturan: %v, body: %s", err, string(body))
	}

	// 3. Endpoint pasang provider: ikatan salah -> 400; ikatan benar -> 200.
	res, body = client.do(http.MethodPost, "/api/admin/gateway/routing-rules/"+aturan.ID+"/providers", map[string]any{
		"provider_ids": []string{provPolos},
	}, true)
	if res.StatusCode != http.StatusBadRequest {
		t.Fatalf("pasang provider tanpa pemetaan status = %d, mau 400, body: %s", res.StatusCode, string(body))
	}
	if !strings.Contains(string(body), "provider_tidak_memetakan_model") {
		t.Errorf("respons pasang provider tidak memuat kode penolakan: %s", string(body))
	}

	res, body = client.do(http.MethodPost, "/api/admin/gateway/routing-rules/"+aturan.ID+"/providers", map[string]any{
		"provider_ids": []string{provPunya},
	}, true)
	if res.StatusCode != http.StatusOK {
		t.Fatalf("pasang provider berpetakan status = %d, mau 200, body: %s", res.StatusCode, string(body))
	}

	// 4. Ubah aturan mengganti ikatan ke provider tanpa pemetaan -> 400.
	res, body = client.do(http.MethodPut, "/api/admin/gateway/routing-rules/"+aturan.ID, map[string]any{
		"name":           "aturan-ikat-benar",
		"strategy":       "priority",
		"match_model_id": modelID,
		"provider_ids":   []string{provPolos},
	}, true)
	if res.StatusCode != http.StatusBadRequest {
		t.Fatalf("ubah aturan ke provider tanpa pemetaan status = %d, mau 400, body: %s", res.StatusCode, string(body))
	}
	if !strings.Contains(string(body), "provider_tidak_memetakan_model") {
		t.Errorf("respons ubah aturan tidak memuat kode penolakan: %s", string(body))
	}

	// 5. Aturan TANPA model target bebas mengikat provider mana pun.
	res, body = client.do(http.MethodPost, "/api/admin/gateway/routing-rules", map[string]any{
		"name":         "aturan-tanpa-model",
		"strategy":     "priority",
		"provider_ids": []string{provPolos},
	}, true)
	if res.StatusCode != http.StatusCreated {
		t.Fatalf("buat aturan tanpa model target status = %d, mau 201, body: %s", res.StatusCode, string(body))
	}
}
