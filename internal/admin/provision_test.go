package admin

import (
	"bytes"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"
)

func TestProvisionCloudflareValidation(t *testing.T) {
	h := NewHandlers(Config{})

	// 1. Missing account_id
	body, _ := json.Marshal(map[string]string{
		"api_token": "token123",
	})
	req := httptest.NewRequest(http.MethodPost, "/api/admin/upstreams/egress-pools/provision/cloudflare", bytes.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	w := httptest.NewRecorder()

	h.provisionCloudflareAIGateway(w, req)
	if w.Code != http.StatusBadRequest {
		t.Errorf("status = %d, want %d", w.Code, http.StatusBadRequest)
	}

	// 2. Missing api_token
	body, _ = json.Marshal(map[string]string{
		"account_id": "acc123",
	})
	req = httptest.NewRequest(http.MethodPost, "/api/admin/upstreams/egress-pools/provision/cloudflare", bytes.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	w = httptest.NewRecorder()

	h.provisionCloudflareAIGateway(w, req)
	if w.Code != http.StatusBadRequest {
		t.Errorf("status = %d, want %d", w.Code, http.StatusBadRequest)
	}
}

func TestProvisionDenoValidation(t *testing.T) {
	h := NewHandlers(Config{})

	// 1. Missing access_token
	body, _ := json.Marshal(map[string]string{
		"project_name": "my-project",
	})
	req := httptest.NewRequest(http.MethodPost, "/api/admin/upstreams/egress-pools/provision/deno", bytes.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	w := httptest.NewRecorder()

	h.provisionDenoRelay(w, req)
	if w.Code != http.StatusBadRequest {
		t.Errorf("status = %d, want %d", w.Code, http.StatusBadRequest)
	}
}
