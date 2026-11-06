package admin

import (
	"encoding/json"
	"net/http"
	"strings"
	"testing"
)

// TestB307AllowlistBitHostDitolakDenganNama adalah pengunci B-307:
// entri ip_allowlist berbit host (10.0.0.1/24) dulu LOLOS validasi
// handler lalu ditolak kolom cidr[] DB sebagai 400 invalid_reference
// generik yang tidak menyebut field maupun entrinya. Kini handler
// menolaknya sebagai invalid_ip_allowlist dengan menyebut entri dan
// bentuk jaringannya — pada jalur buat maupun ubah. Awalan jaringan
// yang rapi dan alamat tunggal tetap diterima.
func TestB307AllowlistBitHostDitolakDenganNama(t *testing.T) {
	env := setupTestEnv(t)
	client := env.newClient(t)
	client.login("superadmin@routex.internal", testPassword)

	res, body := client.do(http.MethodPost, "/api/admin/access/api-keys", map[string]any{
		"name":         "Kunci Bit Host",
		"ip_allowlist": []string{"10.0.0.1/24"},
	}, true)
	if res.StatusCode != http.StatusBadRequest {
		t.Fatalf("buat dengan bit host: status = %d, mau 400 (body: %s)", res.StatusCode, string(body))
	}
	if !strings.Contains(string(body), "invalid_ip_allowlist") ||
		!strings.Contains(string(body), "10.0.0.1/24") ||
		!strings.Contains(string(body), "10.0.0.0/24") {
		t.Errorf("respons tidak menyebut kode/entri/bentuk jaringan: %s", string(body))
	}

	// Kontrol: awalan jaringan rapi dan alamat tunggal diterima.
	res, body = client.do(http.MethodPost, "/api/admin/access/api-keys", map[string]any{
		"name":         "Kunci Rapi",
		"ip_allowlist": []string{"10.0.0.0/24", "10.0.0.1"},
	}, true)
	if res.StatusCode != http.StatusCreated {
		t.Fatalf("buat dengan allowlist sah: status = %d, mau 201 (body: %s)", res.StatusCode, string(body))
	}
	var dibuat struct {
		Key struct {
			ID string `json:"id"`
		} `json:"key"`
	}
	if err := json.Unmarshal(body, &dibuat); err != nil || dibuat.Key.ID == "" {
		t.Fatalf("urai kunci yang dibuat: %v (body: %s)", err, string(body))
	}

	// Jalur ubah: penolakan yang sama.
	res, body = client.do(http.MethodPut, "/api/admin/access/api-keys/"+dibuat.Key.ID, map[string]any{
		"ip_allowlist": []string{"192.168.1.7/16"},
	}, true)
	if res.StatusCode != http.StatusBadRequest {
		t.Fatalf("ubah dengan bit host: status = %d, mau 400 (body: %s)", res.StatusCode, string(body))
	}
	if !strings.Contains(string(body), "invalid_ip_allowlist") ||
		!strings.Contains(string(body), "192.168.1.7/16") ||
		!strings.Contains(string(body), "192.168.0.0/16") {
		t.Errorf("respons ubah tidak menyebut kode/entri/bentuk jaringan: %s", string(body))
	}
}
