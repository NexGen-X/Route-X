package admin

// Test regresi audit3 grup 5 — kontrak error admin (B-066): decodeJSON tidak
// membocorkan detail Go (V-EM-09), konflik keunikan 409 (V-EM-04), dependensi
// tak terpasang 503 & kegagalan hulu 502 tanpa teks mentah (V-EM-01/02/03),
// dan 405 ber-envelope (V-EM-06).

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/go-chi/chi/v5"

	"github.com/NexGen-X/Route-X/internal/database/repo"
	"github.com/NexGen-X/Route-X/internal/httpx"
)

func kodeEnvelope(t *testing.T, body []byte) string {
	t.Helper()
	var env httpx.ErrorResponse
	if err := json.Unmarshal(body, &env); err != nil {
		t.Fatalf("body bukan envelope error: %v (body: %s)", err, body)
	}
	if env.Error.Code == nil {
		return ""
	}
	return *env.Error.Code
}

// --- V-EM-09: decodeJSON tersanitasi ------------------------------------------

func TestDecodeJSONTidakMembocorkanDetailGo(t *testing.T) {
	type structUji struct {
		RateLimitRPM int `json:"rate_limit_rpm"`
	}
	kasus := map[string]string{
		"tipe salah":    `{"rate_limit_rpm":"bukan-angka"}`,
		"field asing":   `{"field_hantu":1}`,
		"json rusak":    `{"rate_limit_rpm":`,
		"data berlebih": `{"rate_limit_rpm":1} {"x":2}`,
	}
	for nama, body := range kasus {
		t.Run(nama, func(t *testing.T) {
			r := httptest.NewRequest(http.MethodPost, "/", strings.NewReader(body))
			var dst structUji
			err := decodeJSON(r, &dst)
			if err == nil {
				t.Fatalf("body %s diterima, mau error", body)
			}
			pesan := err.Error()
			for _, bocor := range []string{"cannot unmarshal", "struct field", "structUji", "json:"} {
				if strings.Contains(pesan, bocor) {
					t.Errorf("pesan error membocorkan %q: %s", bocor, pesan)
				}
			}
			if !strings.Contains(pesan, "bukan JSON yang sah") {
				t.Errorf("pesan = %q, mau pesan generik errBodyJSON", pesan)
			}
		})
	}
}

// --- V-EM-04: konflik keunikan → 409 -------------------------------------------

func TestMapRepoErrorKonflikJadi409(t *testing.T) {
	w := httptest.NewRecorder()
	r := httptest.NewRequest(http.MethodPost, "/api/admin/upstreams/providers", nil)
	mapRepoError(w, r, fmt.Errorf("%w: nama sudah dipakai", repo.ErrConflict), "provider")
	if w.Code != http.StatusConflict {
		t.Fatalf("status = %d, mau 409 (body: %s)", w.Code, w.Body.String())
	}
	if got := kodeEnvelope(t, w.Body.Bytes()); got != "conflict" {
		t.Errorf("code = %q, mau conflict", got)
	}
}

func TestProviderDuplikatDibalas409(t *testing.T) {
	env := setupTestEnv(t)
	client := env.newClient(t)
	client.login("superadmin@routex.internal", testPassword)

	payload := map[string]any{
		"name":         "provider-duplikat-g5",
		"display_name": "Provider Duplikat G5",
		"kind":         "openai",
		"base_url":     "https://api.openai.com/v1",
	}
	res, _ := client.do(http.MethodPost, "/api/admin/upstreams/providers", payload, true)
	if res.StatusCode != http.StatusCreated {
		t.Fatalf("buat pertama: status = %d, mau 201", res.StatusCode)
	}
	res, body := client.do(http.MethodPost, "/api/admin/upstreams/providers", payload, true)
	if res.StatusCode != http.StatusConflict {
		t.Fatalf("buat duplikat: status = %d, mau 409 (body: %s)", res.StatusCode, body)
	}
	if got := kodeEnvelope(t, body); got != "conflict" {
		t.Errorf("code = %q, mau conflict", got)
	}
}

// --- V-EM-03: helper status dependensi ------------------------------------------

func TestTidakTersedia503DanGagalHulu502(t *testing.T) {
	r := httptest.NewRequest(http.MethodPost, "/", nil)

	w := httptest.NewRecorder()
	tidakTersedia(w, r, "oauth_unavailable", "Klien OAuth belum dikonfigurasi")
	if w.Code != http.StatusServiceUnavailable {
		t.Fatalf("tidakTersedia: status = %d, mau 503", w.Code)
	}
	if got := kodeEnvelope(t, w.Body.Bytes()); got != "oauth_unavailable" {
		t.Errorf("code = %q, mau oauth_unavailable", got)
	}

	h := &Handlers{logger: slog.New(slog.NewTextHandler(io.Discard, nil))}
	w = httptest.NewRecorder()
	h.gagalHulu(w, r, "penukaran kode OAuth gagal", "oauth_exchange_failed",
		"Gagal menukar kode otorisasi dengan provider OAuth",
		errors.New(`Post "https://oauth2.googleapis.com/token": dial tcp: token=RAHASIA`))
	if w.Code != http.StatusBadGateway {
		t.Fatalf("gagalHulu: status = %d, mau 502 (body: %s)", w.Code, w.Body.String())
	}
	if strings.Contains(w.Body.String(), "RAHASIA") || strings.Contains(w.Body.String(), "oauth2.googleapis.com") {
		t.Errorf("gagalHulu membocorkan teks mentah ke klien: %s", w.Body.String())
	}
	if got := kodeEnvelope(t, w.Body.Bytes()); got != "oauth_exchange_failed" {
		t.Errorf("code = %q, mau oauth_exchange_failed", got)
	}
}

// TestExchangeOAuthTanpaKlienDibalas503 membuktikan penjaga nil pada endpoint
// exchange kini menjawab 503 (bukan 400 seperti sebelumnya).
func TestExchangeOAuthTanpaKlienDibalas503(t *testing.T) {
	h := NewHandlers(Config{})
	rctx := chi.NewRouteContext()
	rctx.URLParams.Add("id", "075812e2-2369-4f27-81c8-2bf58985653b")
	r := httptest.NewRequest(http.MethodPost, "/upstreams/providers/x/oauth/exchange",
		strings.NewReader(`{"code":"abc","redirect_uri":"http://localhost/cb"}`))
	r = r.WithContext(context.WithValue(r.Context(), chi.RouteCtxKey, rctx))
	w := httptest.NewRecorder()
	h.exchangeOAuthCode(w, r)
	if w.Code != http.StatusServiceUnavailable {
		t.Fatalf("status = %d, mau 503 (body: %s)", w.Code, w.Body.String())
	}
	if got := kodeEnvelope(t, w.Body.Bytes()); got != "oauth_unavailable" {
		t.Errorf("code = %q, mau oauth_unavailable", got)
	}
}

// --- V-EM-06: 405 ber-envelope pada permukaan admin ------------------------------

func TestAdminMetodeSalahBerEnvelope405(t *testing.T) {
	env := setupTestEnv(t)
	client := env.newClient(t)
	client.login("superadmin@routex.internal", testPassword)

	res, body := client.do(http.MethodDelete, "/api/admin/observability/summary", nil, true)
	if res.StatusCode != http.StatusMethodNotAllowed {
		t.Fatalf("status = %d, mau 405 (body: %s)", res.StatusCode, body)
	}
	if len(body) == 0 {
		t.Fatal("body 405 kosong, mau envelope error")
	}
	if got := kodeEnvelope(t, body); got != "method_not_allowed" {
		t.Errorf("code = %q, mau method_not_allowed", got)
	}
}
