package admin

import (
	"context"
	"encoding/json"
	"errors"
	"net/http"
	"net/netip"
	"testing"

	"github.com/NexGen-X/Route-X/internal/database/repo/keys"
	"github.com/NexGen-X/Route-X/internal/database/repo/traffic"
)

// TestSiklusHidupKunciB080 membuktikan desain siklus hidup kunci hasil
// keputusan B-080 di tingkat HTTP penuh:
//
//   - toggle membaca body {enabled: bool} dan bekerja dua arah, sementara
//     body lama {status: string} ditolak;
//   - DELETE menghapus permanen dan kunci yang terhapus seketika tidak
//     bisa autentikasi lagi;
//   - riwayat request milik kunci yang terhapus tetap terbaca utuh,
//     termasuk snapshot nama kuncinya;
//   - endpoint revoke sudah tidak ada.
func TestSiklusHidupKunciB080(t *testing.T) {
	env := setupTestEnv(t)
	c := env.newClient(t)
	c.login("superadmin@routex.internal", testPassword)

	ctx := context.Background()
	keyRepo, err := keys.New(env.pool, []byte("pepper-uji-32-byte-harus-cukup!"))
	if err != nil {
		t.Fatalf("keys.New: %v", err)
	}
	trafficRepo := traffic.New(env.pool)
	ip := netip.MustParseAddr("203.0.113.9")

	// --- Buat kunci lewat API. ---
	res, body := c.do(http.MethodPost, "/api/admin/access/api-keys", map[string]any{
		"name": "Kunci B-080",
	}, true)
	if res.StatusCode != http.StatusCreated {
		t.Fatalf("buat kunci: status %d, body %s", res.StatusCode, body)
	}
	var created struct {
		Key struct {
			ID   string `json:"id"`
			Name string `json:"name"`
		} `json:"key"`
		RawKey string `json:"raw_key"`
	}
	if err := json.Unmarshal(body, &created); err != nil || created.Key.ID == "" || created.RawKey == "" {
		t.Fatalf("parse respons buat kunci: err=%v body=%s", err, body)
	}
	base := "/api/admin/access/api-keys/" + created.Key.ID

	alasanTolak := func() string {
		t.Helper()
		_, err := keyRepo.Authenticate(ctx, created.RawKey, ip)
		var rej *keys.Rejection
		if !errors.As(err, &rej) {
			t.Fatalf("Authenticate: err = %v, mau *keys.Rejection", err)
		}
		return rej.Reason
	}

	// Kunci baru bisa autentikasi.
	if _, err := keyRepo.Authenticate(ctx, created.RawKey, ip); err != nil {
		t.Fatalf("kunci baru tidak bisa autentikasi: %v", err)
	}

	// --- Catat satu request atas nama kunci ini, seperti recorder gateway. ---
	namaKunci := created.Key.Name
	reqPK, reqAt, err := trafficRepo.Insert(ctx, traffic.Record{
		RequestID:      "uji-b080-rantai-hapus",
		APIKeyID:       &created.Key.ID,
		APIKeyName:     &namaKunci,
		Method:         "POST",
		Endpoint:       "/v1/chat/completions",
		RequestedModel: "gpt-5",
		StatusCode:     http.StatusOK,
	})
	if err != nil {
		t.Fatalf("catat request: %v", err)
	}

	// --- Toggle {enabled} dua arah. ---
	type balasanToggle struct {
		Key struct {
			Status  string `json:"status"`
			Enabled bool   `json:"enabled"`
		} `json:"key"`
	}

	res, body = c.do(http.MethodPost, base+"/toggle", map[string]any{"enabled": false}, true)
	if res.StatusCode != http.StatusOK {
		t.Fatalf("toggle nonaktif: status %d, body %s", res.StatusCode, body)
	}
	var mati balasanToggle
	if err := json.Unmarshal(body, &mati); err != nil || mati.Key.Status != keys.StatusDisabled || mati.Key.Enabled {
		t.Errorf("toggle nonaktif = %+v (err %v), mau status disabled dan enabled=false", mati.Key, err)
	}
	if got := alasanTolak(); got != keys.ReasonDisabled {
		t.Errorf("alasan tolak kunci nonaktif = %q, mau %q", got, keys.ReasonDisabled)
	}

	res, body = c.do(http.MethodPost, base+"/toggle", map[string]any{"enabled": true}, true)
	if res.StatusCode != http.StatusOK {
		t.Fatalf("toggle aktif: status %d, body %s", res.StatusCode, body)
	}
	var hidup balasanToggle
	if err := json.Unmarshal(body, &hidup); err != nil || hidup.Key.Status != keys.StatusActive || !hidup.Key.Enabled {
		t.Errorf("toggle aktif = %+v (err %v), mau status active dan enabled=true", hidup.Key, err)
	}
	if _, err := keyRepo.Authenticate(ctx, created.RawKey, ip); err != nil {
		t.Errorf("kunci yang dihidupkan kembali tidak bisa autentikasi: %v", err)
	}

	// Body lama {status} ditolak sebagai field tak dikenal, dan tidak mengubah apa pun.
	res, _ = c.do(http.MethodPost, base+"/toggle", map[string]any{"status": "disabled"}, true)
	if res.StatusCode != http.StatusBadRequest {
		t.Errorf("toggle body lama {status}: status %d, mau 400", res.StatusCode)
	}
	if _, err := keyRepo.Authenticate(ctx, created.RawKey, ip); err != nil {
		t.Errorf("kunci berubah sesudah toggle body lama ditolak: %v", err)
	}

	// --- DELETE menghapus permanen. ---
	res, body = c.do(http.MethodDelete, base, nil, true)
	if res.StatusCode != http.StatusOK {
		t.Fatalf("hapus kunci: status %d, body %s", res.StatusCode, body)
	}
	var terhapus struct {
		Status string `json:"status"`
	}
	if err := json.Unmarshal(body, &terhapus); err != nil || terhapus.Status != "deleted" {
		t.Errorf("respons hapus = %s (err %v), mau {\"status\":\"deleted\"}", body, err)
	}

	res, _ = c.do(http.MethodGet, base, nil, false)
	if res.StatusCode != http.StatusNotFound {
		t.Errorf("GET kunci terhapus: status %d, mau 404", res.StatusCode)
	}

	// Seketika tidak bisa autentikasi lagi, sebagai kunci yang tidak ada.
	if got := alasanTolak(); got != keys.ReasonNotFound {
		t.Errorf("alasan tolak kunci terhapus = %q, mau %q", got, keys.ReasonNotFound)
	}

	// --- Riwayat request selamat, utuh dengan snapshot namanya. ---
	detail, err := trafficRepo.Get(ctx, reqPK, reqAt)
	if err != nil {
		t.Fatalf("baca ulang request milik kunci terhapus: %v", err)
	}
	if detail.APIKeyID != created.Key.ID || detail.APIKeyName != namaKunci {
		t.Errorf("request = api_key_id %q nama %q, mau tetap %q / %q",
			detail.APIKeyID, detail.APIKeyName, created.Key.ID, namaKunci)
	}
	if detail.RequestID != "uji-b080-rantai-hapus" || detail.StatusCode != http.StatusOK {
		t.Errorf("request berubah isi: %+v", detail)
	}

	// Hapus dua kali berarti tidak ditemukan.
	res, _ = c.do(http.MethodDelete, base, nil, true)
	if res.StatusCode != http.StatusNotFound {
		t.Errorf("hapus kunci yang sama dua kali: status %d, mau 404", res.StatusCode)
	}

	// Endpoint revoke sudah tidak ada.
	res, _ = c.do(http.MethodPost, base+"/revoke", map[string]any{"reason": "uji"}, true)
	if res.StatusCode != http.StatusNotFound {
		t.Errorf("POST revoke pada rute yang dicabut: status %d, mau 404", res.StatusCode)
	}
}
