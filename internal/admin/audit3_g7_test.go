package admin

import (
	"context"
	"encoding/json"
	"net/http"
	"strings"
	"testing"
)

// TestAdminWebhookValidasiDanEnabled membuktikan B-031 tertutup di permukaan
// HTTP: enabled:false dihormati, validasi menyebut field (bukan
// constraint_violation buram), events kosong = 400 (bukan 500), secret kosong
// ditolak, dan max_retries 0 eksplisit tidak lagi ditelan menjadi bawaan.
func TestAdminWebhookValidasiDanEnabled(t *testing.T) {
	env := setupTestEnv(t)
	c := env.newClient(t)
	c.login("superadmin@routex.internal", testPassword)

	const jalur = "/api/admin/automation/webhooks"
	sah := map[string]any{
		"name":   "webhook-uji-g7",
		"url":    "https://example.com/hooks",
		"events": []string{"provider.unhealthy"},
		"secret": "whsec_rahasia_uji_g7_panjang",
	}

	// enabled:false harus tersimpan nonaktif (V-WH-02).
	body := map[string]any{}
	for k, v := range sah {
		body[k] = v
	}
	body["enabled"] = false
	res, respBody := c.do(http.MethodPost, jalur, body, true)
	if res.StatusCode != http.StatusCreated {
		t.Fatalf("buat webhook nonaktif: status %d, body %s", res.StatusCode, respBody)
	}
	var dto WebhookDTO
	if err := json.Unmarshal(respBody, &dto); err != nil {
		t.Fatalf("parse DTO: %v", err)
	}
	if dto.Enabled {
		t.Error("webhook yang diminta enabled:false lahir aktif")
	}

	// Tanpa field enabled → bawaan aktif, seperti perilaku lama yang sah.
	body2 := map[string]any{}
	for k, v := range sah {
		body2[k] = v
	}
	body2["name"] = "webhook-uji-g7-aktif"
	res, respBody = c.do(http.MethodPost, jalur, body2, true)
	if res.StatusCode != http.StatusCreated {
		t.Fatalf("buat webhook bawaan: status %d, body %s", res.StatusCode, respBody)
	}
	dto = WebhookDTO{}
	if err := json.Unmarshal(respBody, &dto); err != nil {
		t.Fatalf("parse DTO bawaan: %v", err)
	}
	if !dto.Enabled {
		t.Error("webhook tanpa field enabled tidak lahir aktif")
	}

	// max_retries 0 eksplisit dihormati (dulu ditelan menjadi 3).
	body3 := map[string]any{}
	for k, v := range sah {
		body3[k] = v
	}
	body3["name"] = "webhook-uji-g7-nol"
	body3["max_retries"] = 0
	res, respBody = c.do(http.MethodPost, jalur, body3, true)
	if res.StatusCode != http.StatusCreated {
		t.Fatalf("buat webhook max_retries 0: status %d, body %s", res.StatusCode, respBody)
	}
	dto = WebhookDTO{}
	if err := json.Unmarshal(respBody, &dto); err != nil {
		t.Fatalf("parse DTO nol: %v", err)
	}
	if dto.MaxRetries != 0 {
		t.Errorf("max_retries eksplisit 0 tersimpan %d", dto.MaxRetries)
	}

	// Kasus tidak sah: semua 400 dengan kode/pesan menyebut field (V-WH-03/04).
	kasus := []struct {
		nama      string
		ubah      map[string]any
		kode      string
		muatPesan string
	}{
		{"events kosong", map[string]any{"events": []string{}}, "invalid_events", "events"},
		{"event ngawur", map[string]any{"events": []string{"event.ngawur"}}, "invalid_events", "event.ngawur"},
		{"secret kosong", map[string]any{"secret": "   "}, "missing_secret", "secret"},
		{"max_retries 99", map[string]any{"max_retries": 99}, "invalid_max_retries", "max_retries"},
		{"timeout 50", map[string]any{"timeout_ms": 50}, "invalid_timeout_ms", "timeout_ms"},
		{"nama hanya spasi", map[string]any{"name": "   "}, "invalid_name", "nama"},
	}
	for _, k := range kasus {
		t.Run(k.nama, func(t *testing.T) {
			b := map[string]any{}
			for kk, vv := range sah {
				b[kk] = vv
			}
			b["name"] = "webhook-uji-g7-" + k.nama
			for kk, vv := range k.ubah {
				b[kk] = vv
			}
			res, respBody := c.do(http.MethodPost, jalur, b, true)
			if res.StatusCode != http.StatusBadRequest {
				t.Fatalf("status = %d, mau 400 (body: %s)", res.StatusCode, respBody)
			}
			var envErr struct {
				Code    string `json:"code"`
				Message string `json:"message"`
			}
			// Envelope error httpx bisa bersarang di field "error".
			var mentah map[string]any
			_ = json.Unmarshal(respBody, &mentah)
			if e, ok := mentah["error"].(map[string]any); ok {
				envErr.Code, _ = e["code"].(string)
				envErr.Message, _ = e["message"].(string)
			} else {
				_ = json.Unmarshal(respBody, &envErr)
			}
			if envErr.Code != k.kode {
				t.Errorf("kode error = %q, mau %q (body: %s)", envErr.Code, k.kode, respBody)
			}
			if !strings.Contains(envErr.Message, k.muatPesan) {
				t.Errorf("pesan error %q tidak menyebut %q", envErr.Message, k.muatPesan)
			}
		})
	}
}

// sinkEventPalsu merekam event yang dipancarkan handler admin.
type sinkEventPalsu struct {
	events []string
}

func (s *sinkEventPalsu) Enqueue(_ context.Context, event string, _ any) (int, error) {
	s.events = append(s.events, event)
	return 1, nil
}

func (s *sinkEventPalsu) punya(event string) bool {
	for _, e := range s.events {
		if e == event {
			return true
		}
	}
	return false
}

// TestAdminEventAPIKeyDanBanDiproduksi membuktikan produser api_key.created,
// api_key.revoked, dan ban.created terpasang (audit3 V-WH-08).
func TestAdminEventAPIKeyDanBanDiproduksi(t *testing.T) {
	env := setupTestEnv(t)
	sink := &sinkEventPalsu{}
	env.handlers.webhookEvents = sink

	c := env.newClient(t)
	c.login("superadmin@routex.internal", testPassword)

	// Buat API key → api_key.created.
	res, body := c.do(http.MethodPost, "/api/admin/access/api-keys", map[string]any{
		"name": "kunci-uji-g7",
	}, true)
	if res.StatusCode != http.StatusCreated {
		t.Fatalf("buat api key: status %d, body %s", res.StatusCode, body)
	}
	var dibuat struct {
		Key struct {
			ID string `json:"id"`
		} `json:"key"`
	}
	if err := json.Unmarshal(body, &dibuat); err != nil || dibuat.Key.ID == "" {
		t.Fatalf("parse respons buat key: err=%v body=%s", err, body)
	}
	if !sink.punya("api_key.created") {
		t.Errorf("api_key.created tidak dipancarkan; events=%v", sink.events)
	}

	// Cabut API key → api_key.revoked.
	res, body = c.do(http.MethodPost, "/api/admin/access/api-keys/"+dibuat.Key.ID+"/revoke", map[string]any{
		"reason": "uji g7",
	}, true)
	if res.StatusCode != http.StatusOK {
		t.Fatalf("cabut api key: status %d, body %s", res.StatusCode, body)
	}
	if !sink.punya("api_key.revoked") {
		t.Errorf("api_key.revoked tidak dipancarkan; events=%v", sink.events)
	}

	// Buat ban → ban.created.
	res, body = c.do(http.MethodPost, "/api/admin/gateway/bans", map[string]any{
		"subject_kind": "ip",
		"subject":      "203.0.113.7",
		"reason":       "uji g7",
	}, true)
	if res.StatusCode != http.StatusCreated {
		t.Fatalf("buat ban: status %d, body %s", res.StatusCode, body)
	}
	if !sink.punya("ban.created") {
		t.Errorf("ban.created tidak dipancarkan; events=%v", sink.events)
	}
}
