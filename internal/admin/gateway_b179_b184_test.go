package admin

import (
	"context"
	"encoding/json"
	"net/http"
	"strings"
	"testing"
)

// TestB179PipelineModelTidakDikenalDitolak: anggota resep combo yang tidak
// ada di katalog wajib ditolak di jalur tulis (B-179). Sebelumnya aturan
// seperti ini tersimpan sah, lalu runtime melewati anggota itu dengan log
// Warn saja dan resep menyusut diam-diam.
func TestB179PipelineModelTidakDikenalDitolak(t *testing.T) {
	env := setupTestEnv(t)
	client := env.newClient(t)
	client.login("superadmin@routex.internal", testPassword)

	res, body := client.do(http.MethodPost, "/api/admin/gateway/routing-rules", map[string]any{
		"name": "B179 Anggota Ngawur",
		"pipeline": map[string]any{
			"strategy": "priority", "attempts": 3,
			"models": []string{"gpt-5", "model-ngawur-b179"},
		},
	}, true)
	if res.StatusCode != http.StatusBadRequest {
		t.Fatalf("status = %d, mau 400, body: %s", res.StatusCode, string(body))
	}
	if !strings.Contains(string(body), "pipeline_model_tidak_dikenal") {
		t.Errorf("respons tidak memuat kode pipeline_model_tidak_dikenal: %s", string(body))
	}
	if !strings.Contains(string(body), "model-ngawur-b179") {
		t.Errorf("respons tidak menyebut anggota yang tidak dikenal: %s", string(body))
	}

	// Kontrol: resep dengan anggota yang dikenal tersimpan seperti biasa.
	res, body = client.do(http.MethodPost, "/api/admin/gateway/routing-rules", map[string]any{
		"name":     "B179 Kontrol Sah",
		"pipeline": pipelineValid,
	}, true)
	if res.StatusCode != http.StatusCreated {
		t.Fatalf("kontrol status = %d, mau 201, body: %s", res.StatusCode, string(body))
	}
}

// TestB180PipelineMenyusutSetelahResolusiDitolak: resep yang sesudah
// resolusi menyisakan kurang dari dua model BERBEDA wajib ditolak (B-180) —
// mencakup daftar satu model dan dua sebutan untuk model kanonik yang sama
// (nama + UUID). Resep seperti itu tidak pernah masuk jalur combo karena
// Rule.Combo() menuntut lebih dari satu anggota.
func TestB180PipelineMenyusutSetelahResolusiDitolak(t *testing.T) {
	env := setupTestEnv(t)
	client := env.newClient(t)
	client.login("superadmin@routex.internal", testPassword)

	var uuidGPT5 string
	if err := env.pool.QueryRow(context.Background(),
		"select id from models where model_id = 'gpt-5' limit 1").Scan(&uuidGPT5); err != nil {
		t.Fatalf("cari UUID gpt-5: %v", err)
	}

	for _, tc := range []struct {
		nama   string
		models []string
	}{
		{"satu model", []string{"gpt-5"}},
		{"nama dan UUID model yang sama", []string{"gpt-5", uuidGPT5}},
	} {
		t.Run(tc.nama, func(t *testing.T) {
			res, body := client.do(http.MethodPost, "/api/admin/gateway/routing-rules", map[string]any{
				"name": "B180 " + tc.nama,
				"pipeline": map[string]any{
					"strategy": "priority", "attempts": 3, "models": tc.models,
				},
			}, true)
			if res.StatusCode != http.StatusBadRequest {
				t.Fatalf("status = %d, mau 400, body: %s", res.StatusCode, string(body))
			}
			if !strings.Contains(string(body), "pipeline_model_berulang") {
				t.Errorf("respons tidak memuat kode pipeline_model_berulang: %s", string(body))
			}
		})
	}

	// Jalur ubah: mengganti resep sah menjadi resep satu model juga ditolak.
	res, body := client.do(http.MethodPost, "/api/admin/gateway/routing-rules", map[string]any{
		"name":     "B180 Ubah Resep",
		"pipeline": pipelineValid,
	}, true)
	if res.StatusCode != http.StatusCreated {
		t.Fatalf("buat aturan awal status = %d, body: %s", res.StatusCode, string(body))
	}
	var dibuat struct {
		ID string `json:"id"`
	}
	if err := json.Unmarshal(body, &dibuat); err != nil || dibuat.ID == "" {
		t.Fatalf("urai aturan awal: %v, body: %s", err, string(body))
	}
	res, body = client.do(http.MethodPut, "/api/admin/gateway/routing-rules/"+dibuat.ID, map[string]any{
		"pipeline": map[string]any{
			"strategy": "priority", "attempts": 3, "models": []string{"gemini-2-5-pro"},
		},
	}, true)
	if res.StatusCode != http.StatusBadRequest {
		t.Fatalf("ubah status = %d, mau 400, body: %s", res.StatusCode, string(body))
	}
	if !strings.Contains(string(body), "pipeline_model_berulang") {
		t.Errorf("respons ubah tidak memuat kode pipeline_model_berulang: %s", string(body))
	}
}

// TestB183KapabilitasMustahilDitolak: match_capabilities yang menggabungkan
// embeddings dengan kapabilitas lain tidak akan pernah cocok di permukaan
// mana pun (B-183) dan wajib ditolak di jalur tulis; embeddings sendirian
// dan kombinasi chat biasa tetap sah.
func TestB183KapabilitasMustahilDitolak(t *testing.T) {
	env := setupTestEnv(t)
	client := env.newClient(t)
	client.login("superadmin@routex.internal", testPassword)

	res, body := client.do(http.MethodPost, "/api/admin/gateway/routing-rules", map[string]any{
		"name":               "B183 Campuran Mustahil",
		"match_capabilities": []string{"embeddings", "text"},
	}, true)
	if res.StatusCode != http.StatusBadRequest {
		t.Fatalf("status = %d, mau 400, body: %s", res.StatusCode, string(body))
	}
	if !strings.Contains(string(body), "kapabilitas_tidak_mungkin") {
		t.Errorf("respons tidak memuat kode kapabilitas_tidak_mungkin: %s", string(body))
	}

	// Kontrol: embeddings sendirian sah (permukaan embeddings meminta
	// tepat {embeddings}).
	res, body = client.do(http.MethodPost, "/api/admin/gateway/routing-rules", map[string]any{
		"name":               "B183 Embeddings Murni",
		"match_capabilities": []string{"embeddings"},
	}, true)
	if res.StatusCode != http.StatusCreated {
		t.Fatalf("kontrol embeddings status = %d, mau 201, body: %s", res.StatusCode, string(body))
	}
	var dibuat struct {
		ID string `json:"id"`
	}
	if err := json.Unmarshal(body, &dibuat); err != nil || dibuat.ID == "" {
		t.Fatalf("urai kontrol: %v, body: %s", err, string(body))
	}

	// Jalur ubah: mengubah aturan sah menjadi campuran mustahil ditolak.
	res, body = client.do(http.MethodPut, "/api/admin/gateway/routing-rules/"+dibuat.ID, map[string]any{
		"match_capabilities": []string{"embeddings", "tools"},
	}, true)
	if res.StatusCode != http.StatusBadRequest {
		t.Fatalf("ubah status = %d, mau 400, body: %s", res.StatusCode, string(body))
	}
	if !strings.Contains(string(body), "kapabilitas_tidak_mungkin") {
		t.Errorf("respons ubah tidak memuat kode kapabilitas_tidak_mungkin: %s", string(body))
	}

	// Kontrol: kombinasi chat biasa sah.
	res, body = client.do(http.MethodPost, "/api/admin/gateway/routing-rules", map[string]any{
		"name":               "B183 Chat Biasa",
		"match_capabilities": []string{"text", "vision"},
	}, true)
	if res.StatusCode != http.StatusCreated {
		t.Fatalf("kontrol chat status = %d, mau 201, body: %s", res.StatusCode, string(body))
	}
}

// TestB184IkatanProviderComboDitolak: ikatan provider pada aturan
// ber-pipeline tidak pernah diterapkan mesin di jalur combo (B-184), jadi
// ketiga jalur yang bisa menciptakan keadaan itu — buat, ubah, dan pasang
// provider — wajib menolaknya dengan kode yang menyebut alasannya.
func TestB184IkatanProviderComboDitolak(t *testing.T) {
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
	provider := buatProvider("prov-b184")

	// (a) Buat aturan combo langsung dengan ikatan provider.
	res, body := client.do(http.MethodPost, "/api/admin/gateway/routing-rules", map[string]any{
		"name":         "B184 Buat Combo Terikat",
		"pipeline":     pipelineValid,
		"provider_ids": []string{provider},
	}, true)
	if res.StatusCode != http.StatusBadRequest {
		t.Fatalf("(a) status = %d, mau 400, body: %s", res.StatusCode, string(body))
	}
	if !strings.Contains(string(body), "provider_tidak_berlaku_untuk_combo") {
		t.Errorf("(a) respons tidak memuat kode B-184: %s", string(body))
	}

	// (b) Pasang provider ke aturan combo yang sudah ada.
	combo := simpanAlias(t, env, client, "B184 Combo Pasang", "b184-pasang")
	res, body = client.do(http.MethodPost, "/api/admin/gateway/routing-rules/"+combo.ID+"/providers", map[string]any{
		"provider_ids": []string{provider},
	}, true)
	if res.StatusCode != http.StatusBadRequest {
		t.Fatalf("(b) status = %d, mau 400, body: %s", res.StatusCode, string(body))
	}
	if !strings.Contains(string(body), "provider_tidak_berlaku_untuk_combo") {
		t.Errorf("(b) respons tidak memuat kode B-184: %s", string(body))
	}

	// (c) Ubah aturan combo dengan menyebut provider_ids.
	res, body = client.do(http.MethodPut, "/api/admin/gateway/routing-rules/"+combo.ID, map[string]any{
		"provider_ids": []string{provider},
	}, true)
	if res.StatusCode != http.StatusBadRequest {
		t.Fatalf("(c) status = %d, mau 400, body: %s", res.StatusCode, string(body))
	}
	if !strings.Contains(string(body), "provider_tidak_berlaku_untuk_combo") {
		t.Errorf("(c) respons tidak memuat kode B-184: %s", string(body))
	}

	// (d) Pasang pipeline ke aturan non-combo yang sudah terikat provider.
	provPunya := buatProvider("prov-b184-punya-model")
	resMap, bodyMap := client.do(http.MethodPost, "/api/admin/upstreams/providers/"+provPunya+"/models", map[string]any{
		"name": "model-uji-b184",
	}, true)
	if resMap.StatusCode != http.StatusCreated {
		t.Fatalf("(d) petakan model status = %d, body: %s", resMap.StatusCode, string(bodyMap))
	}
	var modelB184 string
	if err := env.pool.QueryRow(context.Background(),
		"select id from models where model_id = 'model-uji-b184' limit 1").Scan(&modelB184); err != nil {
		t.Fatalf("(d) cari UUID model-uji-b184: %v", err)
	}
	res, body = client.do(http.MethodPost, "/api/admin/gateway/routing-rules", map[string]any{
		"name":           "B184 Terikat Dulu",
		"strategy":       "priority",
		"match_model_id": modelB184,
		"provider_ids":   []string{provPunya},
	}, true)
	if res.StatusCode != http.StatusCreated {
		t.Fatalf("(d) buat aturan terikat status = %d, mau 201, body: %s", res.StatusCode, string(body))
	}
	var terikat struct {
		ID string `json:"id"`
	}
	if err := json.Unmarshal(body, &terikat); err != nil || terikat.ID == "" {
		t.Fatalf("(d) urai aturan terikat: %v, body: %s", err, string(body))
	}
	res, body = client.do(http.MethodPut, "/api/admin/gateway/routing-rules/"+terikat.ID, map[string]any{
		"pipeline": pipelineValid,
	}, true)
	if res.StatusCode != http.StatusBadRequest {
		t.Fatalf("(d) status = %d, mau 400, body: %s", res.StatusCode, string(body))
	}
	if !strings.Contains(string(body), "provider_tidak_berlaku_untuk_combo") {
		t.Errorf("(d) respons tidak memuat kode B-184: %s", string(body))
	}
}
