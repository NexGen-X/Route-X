package admin

import (
	"encoding/json"
	"net/http"
	"strings"
	"testing"
)

// TestB369UpdateParsialTidakMenghapusAllowlist adalah pengunci
// B-369: updateAPIKey dulu menginisialisasi ips sebagai slice
// kosong NON-nil tanpa syarat, jadi setiap PUT yang tidak menyebut
// ip_allowlist ikut mengosongkan allowlist tersimpan (repo membaca
// non-nil sebagai SET) — kunci yang tadinya terbatas IP tertentu
// terbuka dari mana saja secara senyap; PUT {} pun menjawab 200.
// Kini handler hanya memasang IPAllowlist bila body menyebut
// field-nya: tidak disebut = biarkan, [] eksplisit = buka dari
// mana saja, dan PUT {} jatuh ke ErrNoChanges repo → 400
// no_changes seperti permukaan admin lain (B-306/B-354).
func TestB369UpdateParsialTidakMenghapusAllowlist(t *testing.T) {
	env := setupTestEnv(t)
	client := env.newClient(t)
	client.login("superadmin@routex.internal", testPassword)

	ambilAllowlist := func(body []byte) []string {
		t.Helper()
		var dto struct {
			Key struct {
				IPAllowlist []string `json:"ip_allowlist"`
			} `json:"key"`
		}
		if err := json.Unmarshal(body, &dto); err != nil {
			t.Fatalf("urai DTO kunci: %v (body: %s)", err, body)
		}
		return dto.Key.IPAllowlist
	}

	res, body := client.do(http.MethodPost, "/api/admin/access/api-keys", map[string]any{
		"name":         "Kunci B369",
		"ip_allowlist": []string{"10.0.0.9"},
	}, true)
	if res.StatusCode != http.StatusCreated {
		t.Fatalf("buat kunci: status = %d, mau 201 (body: %s)", res.StatusCode, body)
	}
	var dibuat struct {
		Key struct {
			ID string `json:"id"`
		} `json:"key"`
	}
	if err := json.Unmarshal(body, &dibuat); err != nil || dibuat.Key.ID == "" {
		t.Fatalf("urai id kunci: %v (body: %s)", err, body)
	}
	url := "/api/admin/access/api-keys/" + dibuat.Key.ID

	// PUT ganti nama saja: allowlist TIDAK boleh ikut terhapus.
	res, body = client.do(http.MethodPut, url, map[string]any{"name": "Kunci B369 Baru"}, true)
	if res.StatusCode != http.StatusOK {
		t.Fatalf("PUT nama-saja: status = %d, mau 200 (body: %s)", res.StatusCode, body)
	}
	if got := ambilAllowlist(body); len(got) != 1 || got[0] != "10.0.0.9/32" {
		t.Errorf("allowlist sesudah PUT nama-saja = %v, mau [10.0.0.9/32] — update parsial menghapus allowlist", got)
	}

	// PUT {} → 400 no_changes.
	res, body = client.do(http.MethodPut, url, map[string]any{}, true)
	if res.StatusCode != http.StatusBadRequest {
		t.Fatalf("PUT {} status = %d, mau 400 (body: %s)", res.StatusCode, body)
	}
	if !strings.Contains(string(body), "no_changes") {
		t.Errorf("PUT {} body = %s, mau memuat kode no_changes", body)
	}

	// PUT [] eksplisit tetap berarti "buka dari mana saja".
	res, body = client.do(http.MethodPut, url, map[string]any{"ip_allowlist": []string{}}, true)
	if res.StatusCode != http.StatusOK {
		t.Fatalf("PUT [] eksplisit: status = %d, mau 200 (body: %s)", res.StatusCode, body)
	}
	if got := ambilAllowlist(body); len(got) != 0 {
		t.Errorf("allowlist sesudah PUT [] = %v, mau [] (buka dari mana saja)", got)
	}

	// PUT dengan nilai baru tetap diterapkan.
	res, body = client.do(http.MethodPut, url, map[string]any{"ip_allowlist": []string{"10.0.0.10"}}, true)
	if res.StatusCode != http.StatusOK {
		t.Fatalf("PUT allowlist baru: status = %d, mau 200 (body: %s)", res.StatusCode, body)
	}
	if got := ambilAllowlist(body); len(got) != 1 || got[0] != "10.0.0.10/32" {
		t.Errorf("allowlist sesudah PUT nilai baru = %v, mau [10.0.0.10/32]", got)
	}
}

// TestB370CaseSensitiveTidakTertimpaUpdateParsial adalah pengunci
// B-370 + B-366 (sisi HTTP): updateContentFilter dulu memasang
// CaseSensitive dari bool polos request tanpa syarat — body yang
// tak menyebut case_sensitive terurai menjadi false dan menimpa
// nilai tersimpan. Field itu kini dibaca sebagai penunjuk lewat
// struct pembungkus (pola yang sama dengan model_id/provider_id):
// tidak disebut = biarkan. PUT {} pada filter kini juga 400
// no_changes lewat ErrNoChanges repo (B-366).
func TestB370CaseSensitiveTidakTertimpaUpdateParsial(t *testing.T) {
	env := setupTestEnv(t)
	client := env.newClient(t)
	client.login("superadmin@routex.internal", testPassword)

	res, body := client.do(http.MethodPost, "/api/admin/gateway/content-filters", map[string]any{
		"name":           "filter-b370",
		"kind":           "blocked_pattern",
		"priority":       100,
		"applies_to":     "request",
		"action":         "block",
		"pattern":        "PolaPeka",
		"pattern_type":   "substring",
		"case_sensitive": true,
	}, true)
	if res.StatusCode != http.StatusCreated {
		t.Fatalf("buat filter: status = %d, mau 201 (body: %s)", res.StatusCode, body)
	}
	var f ContentFilterDTO
	if err := json.Unmarshal(body, &f); err != nil || f.ID == "" {
		t.Fatalf("urai filter: %v (body: %s)", err, body)
	}
	if !f.CaseSensitive {
		t.Fatalf("filter terbuat case_sensitive = false, mau true (body: %s)", body)
	}
	url := "/api/admin/gateway/content-filters/" + f.ID

	// PUT nama-saja: case_sensitive TIDAK boleh berubah.
	res, body = client.do(http.MethodPut, url, map[string]any{"name": "filter-b370-baru"}, true)
	if res.StatusCode != http.StatusOK {
		t.Fatalf("PUT nama-saja: status = %d, mau 200 (body: %s)", res.StatusCode, body)
	}
	var sesudah ContentFilterDTO
	if err := json.Unmarshal(body, &sesudah); err != nil {
		t.Fatalf("urai hasil PUT: %v (body: %s)", err, body)
	}
	if !sesudah.CaseSensitive {
		t.Errorf("case_sensitive sesudah PUT nama-saja = false, mau tetap true — tertimpa bawaan request")
	}

	// PUT case_sensitive:false eksplisit tetap diterapkan.
	res, body = client.do(http.MethodPut, url, map[string]any{"case_sensitive": false}, true)
	if res.StatusCode != http.StatusOK {
		t.Fatalf("PUT case_sensitive:false: status = %d, mau 200 (body: %s)", res.StatusCode, body)
	}
	if err := json.Unmarshal(body, &sesudah); err != nil {
		t.Fatalf("urai hasil PUT: %v (body: %s)", err, body)
	}
	if sesudah.CaseSensitive {
		t.Errorf("case_sensitive sesudah PUT eksplisit false = true, mau false")
	}

	// PUT {} → 400 no_changes (B-366 sisi HTTP).
	res, body = client.do(http.MethodPut, url, map[string]any{}, true)
	if res.StatusCode != http.StatusBadRequest {
		t.Fatalf("PUT {} filter status = %d, mau 400 (body: %s)", res.StatusCode, body)
	}
	if !strings.Contains(string(body), "no_changes") {
		t.Errorf("PUT {} filter body = %s, mau memuat kode no_changes", body)
	}
}
