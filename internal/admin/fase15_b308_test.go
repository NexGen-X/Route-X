package admin

import (
	"encoding/json"
	"net/http"
	"strings"
	"testing"
)

// TestB308RegexFilterRusakDitolakSaatTulis adalah pengunci B-308: pola
// regex yang tidak bisa dikompilasi dulu tersimpan dengan 201 dan
// tampil aktif, padahal mesin penyaring menandainya rusak dan aturan
// tidak menegakkan apa pun. Kini penolakan terjadi di titik tulis
// (400 invalid_parameter), pada buat maupun ubah — termasuk ubah yang
// hanya mengganti tipe pola menjadi regex atas pola lama yang rusak.
// Regex yang sah dan tipe non-regex tidak terpengaruh.
func TestB308RegexFilterRusakDitolakSaatTulis(t *testing.T) {
	env := setupTestEnv(t)
	client := env.newClient(t)
	client.login("superadmin@routex.internal", testPassword)

	res, body := client.do(http.MethodPost, "/api/admin/gateway/content-filters", map[string]any{
		"name": "filter-regex-rusak", "kind": "blocked_pattern",
		"pattern": "([a-z", "pattern_type": "regex",
	}, true)
	if res.StatusCode != http.StatusBadRequest {
		t.Fatalf("buat regex rusak: status = %d, mau 400 (body: %s)", res.StatusCode, string(body))
	}
	if !strings.Contains(string(body), "invalid_parameter") {
		t.Errorf("buat regex rusak: body = %s, mau memuat invalid_parameter", string(body))
	}

	// Kontrol: substring polos yang bukan regex sah disimpan.
	res, body = client.do(http.MethodPost, "/api/admin/gateway/content-filters", map[string]any{
		"name": "filter-substring", "kind": "blocked_pattern",
		"pattern": "([a-z", "pattern_type": "substring",
	}, true)
	if res.StatusCode != http.StatusCreated {
		t.Fatalf("buat substring: status = %d, mau 201 (body: %s)", res.StatusCode, string(body))
	}
	var dibuat struct {
		ID string `json:"id"`
	}
	if err := json.Unmarshal(body, &dibuat); err != nil || dibuat.ID == "" {
		t.Fatalf("urai filter: %v (body: %s)", err, string(body))
	}

	// Ubah tipe pola filter substring itu menjadi regex: pola efektif
	// menjadi regex rusak → 400.
	res, body = client.do(http.MethodPut, "/api/admin/gateway/content-filters/"+dibuat.ID, map[string]any{
		"pattern_type": "regex",
	}, true)
	if res.StatusCode != http.StatusBadRequest {
		t.Fatalf("ubah tipe ke regex atas pola rusak: status = %d, mau 400 (body: %s)", res.StatusCode, string(body))
	}

	// Kontrol: regex yang sah diterima.
	res, body = client.do(http.MethodPost, "/api/admin/gateway/content-filters", map[string]any{
		"name": "filter-regex-sah", "kind": "blocked_pattern",
		"pattern": "(kata|frasa)-[0-9]+", "pattern_type": "regex",
	}, true)
	if res.StatusCode != http.StatusCreated {
		t.Fatalf("buat regex sah: status = %d, mau 201 (body: %s)", res.StatusCode, string(body))
	}
}
