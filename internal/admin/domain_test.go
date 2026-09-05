package admin

import (
	"net/http"
	"net/http/httptest"
	"testing"
)

func TestDomainRegex(t *testing.T) {
	valid := []string{
		"example.com",
		"ai.example.com",
		"sub.sub.domain.co.id",
		"my-gateway.org",
		"route-x.dev",
	}

	for _, d := range valid {
		if !domainRegex.MatchString(d) {
			t.Errorf("domain %q harusnya valid, tapi ditolak regex", d)
		}
	}

	invalid := []string{
		"localhost",
		"127.0.0.1",
		"http://example.com",
		"example",
		"example..com",
		"-bad.com",
		"bad-.com",
	}

	for _, d := range invalid {
		if domainRegex.MatchString(d) {
			t.Errorf("domain %q harusnya tidak valid, tapi diterima regex", d)
		}
	}
}

func TestCaddyTLSCheck_Localhost(t *testing.T) {
	h := &Handlers{}

	// Localhost harus selalu diizinkan
	req := httptest.NewRequest(http.MethodGet, "/api/internal/tls-check?domain=localhost", nil)
	rr := httptest.NewRecorder()
	h.CaddyTLSCheck(rr, req)

	if rr.Code != http.StatusOK {
		t.Errorf("status localhost = %d, diharapkan %d", rr.Code, http.StatusOK)
	}

	// 127.0.0.1 harus selalu diizinkan
	req = httptest.NewRequest(http.MethodGet, "/api/internal/tls-check?domain=127.0.0.1", nil)
	rr = httptest.NewRecorder()
	h.CaddyTLSCheck(rr, req)

	if rr.Code != http.StatusOK {
		t.Errorf("status 127.0.0.1 = %d, diharapkan %d", rr.Code, http.StatusOK)
	}

	// Domain kosong harus 400
	req = httptest.NewRequest(http.MethodGet, "/api/internal/tls-check", nil)
	rr = httptest.NewRecorder()
	h.CaddyTLSCheck(rr, req)

	if rr.Code != http.StatusBadRequest {
		t.Errorf("status domain kosong = %d, diharapkan %d", rr.Code, http.StatusBadRequest)
	}

	// Domain yang tidak terdaftar harus 403
	req = httptest.NewRequest(http.MethodGet, "/api/internal/tls-check?domain=random.unauthorized.com", nil)
	rr = httptest.NewRecorder()
	h.CaddyTLSCheck(rr, req)

	if rr.Code != http.StatusForbidden {
		t.Errorf("status domain liar = %d, diharapkan %d", rr.Code, http.StatusForbidden)
	}
}
