package httpx

import (
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/go-chi/chi/v5"

	"github.com/NexGen-X/Route-X/internal/config"
	"github.com/NexGen-X/Route-X/internal/observability"
)

// TestSecurityHeadersHSTSPublicURLHTTPS membuktikan audit3 V-LN-22: HSTS
// menyala bila PUBLIC_URL berawalan https walau APP_ENV bukan production.
func TestSecurityHeadersHSTSPublicURLHTTPS(t *testing.T) {
	cfg := &config.Config{AppEnv: config.EnvDevelopment, PublicURL: "https://gateway.example.com"}
	h := SecurityHeaders(cfg)(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
	}))
	rec := httptest.NewRecorder()
	h.ServeHTTP(rec, httptest.NewRequest(http.MethodGet, "/", nil))
	if rec.Header().Get("Strict-Transport-Security") == "" {
		t.Fatal("HSTS harus terpasang untuk PUBLIC_URL https di luar production")
	}
	if csp := rec.Header().Get("Content-Security-Policy"); !strings.Contains(csp, "upgrade-insecure-requests") {
		t.Fatalf("CSP harus memuat upgrade-insecure-requests untuk HTTPS, dapat %q", csp)
	}
}

// TestSecurityHeadersTanpaHSTSHTTPPolos: pemasangan HTTP polos development
// tetap tanpa HSTS (pinning localhost yang merusak proyek lain).
func TestSecurityHeadersTanpaHSTSHTTPPolos(t *testing.T) {
	cfg := &config.Config{AppEnv: config.EnvDevelopment, PublicURL: "http://localhost:8080"}
	h := SecurityHeaders(cfg)(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
	}))
	rec := httptest.NewRecorder()
	h.ServeHTTP(rec, httptest.NewRequest(http.MethodGet, "/", nil))
	if rec.Header().Get("Strict-Transport-Security") != "" {
		t.Fatal("HSTS tidak boleh terpasang untuk HTTP polos development")
	}
}

// TestMetricsRecorderMelewatiProbe membuktikan audit3 V-EM-26: /healthz,
// /readyz, dan /metrics tidak dicatat ke metrik HTTP admin.
func TestMetricsRecorderMelewatiProbe(t *testing.T) {
	m := observability.NewMetrics()
	r := chi.NewRouter()
	r.Use(MetricsRecorder(m))
	ok := func(w http.ResponseWriter, req *http.Request) { w.WriteHeader(http.StatusOK) }
	r.Get("/healthz", ok)
	r.Get("/readyz", ok)
	r.Get("/metrics", ok)
	r.Get("/api/admin/users", ok)
	r.HandleFunc("/v1/chat/completions", ok)

	for _, path := range []string{"/healthz", "/readyz", "/metrics", "/api/admin/users", "/v1/chat/completions"} {
		rec := httptest.NewRecorder()
		r.ServeHTTP(rec, httptest.NewRequest(http.MethodGet, path, nil))
		if rec.Code != http.StatusOK {
			t.Fatalf("%s: status = %d, mau 200", path, rec.Code)
		}
	}

	mfs, err := m.Registry().Gather()
	if err != nil {
		t.Fatalf("gather: %v", err)
	}
	routes := map[string]bool{}
	for _, mf := range mfs {
		if mf.GetName() != "routex_http_requests_total" {
			continue
		}
		for _, metric := range mf.GetMetric() {
			for _, lp := range metric.GetLabel() {
				if lp.GetName() == "route" {
					routes[lp.GetValue()] = true
				}
			}
		}
	}
	for _, dilarang := range []string{"/healthz", "/readyz", "/metrics", "/v1/chat/completions"} {
		if routes[dilarang] {
			t.Fatalf("route %s tidak boleh tercatat di metrik HTTP admin", dilarang)
		}
	}
	if !routes["/api/admin/users"] {
		t.Fatal("route admin biasa harus tetap tercatat")
	}
}
