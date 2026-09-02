package main

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/NexGen-X/Route-X/internal/config"
	"github.com/NexGen-X/Route-X/internal/health"
	"github.com/NexGen-X/Route-X/internal/observability"
)

func testLogger() *slog.Logger {
	return slog.New(slog.NewJSONHandler(&bytes.Buffer{}, nil))
}

func testConfig(env config.Env) *config.Config {
	return &config.Config{
		AppEnv:          env,
		Port:            8080,
		MaxRequestBytes: 1 << 20,
	}
}

// newTestRouter menyusun router yang sama dengan produksi, memakai checker palsu
// sehingga tidak butuh Postgres maupun Redis.
func newTestRouter(t *testing.T, cfg *config.Config, checkers ...health.Checker) http.Handler {
	t.Helper()
	r, err := buildRouter(cfg, testLogger(), observability.NewMetrics(), nil, checkers...)
	if err != nil {
		t.Fatalf("buildRouter: %v", err)
	}
	return r
}

func okChecker(name string) health.Checker {
	return health.NewChecker(name, func(context.Context) error { return nil })
}

func do(t *testing.T, h http.Handler, method, path string) *httptest.ResponseRecorder {
	t.Helper()
	rec := httptest.NewRecorder()
	h.ServeHTTP(rec, httptest.NewRequest(method, path, nil))
	return rec
}

// Probe kesehatan harus menjawab GET maupun HEAD: banyak load balancer memakai HEAD,
// dan sebelum perbaikan ini chi menjawab 405 sehingga instance sehat dianggap mati.
func TestHealthProbesAcceptGetAndHead(t *testing.T) {
	r := newTestRouter(t, testConfig(config.EnvDevelopment), okChecker("postgres"))

	for _, path := range []string{"/healthz", "/readyz"} {
		for _, method := range []string{http.MethodGet, http.MethodHead} {
			t.Run(method+" "+path, func(t *testing.T) {
				if rec := do(t, r, method, path); rec.Code != http.StatusOK {
					t.Errorf("status = %d, mau 200", rec.Code)
				}
			})
		}
	}
}

func TestReadyzReflectsDependencyFailure(t *testing.T) {
	failing := health.NewChecker("postgres", func(context.Context) error {
		return errors.New("dial tcp 10.0.0.5:5432: connection refused")
	})
	r := newTestRouter(t, testConfig(config.EnvDevelopment), failing)

	rec := do(t, r, http.MethodGet, "/readyz")
	if rec.Code != http.StatusServiceUnavailable {
		t.Fatalf("status = %d, mau 503", rec.Code)
	}
	// Detail internal tidak boleh keluar lewat probe.
	if strings.Contains(rec.Body.String(), "10.0.0.5") {
		t.Errorf("respons membocorkan host internal: %s", rec.Body.String())
	}
}

func TestMetricsEndpointServed(t *testing.T) {
	r := newTestRouter(t, testConfig(config.EnvDevelopment))

	rec := do(t, r, http.MethodGet, "/metrics")
	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d, mau 200", rec.Code)
	}
	if !strings.Contains(rec.Body.String(), "routex_") {
		t.Error("keluaran /metrics tidak memuat metrik aplikasi")
	}
}

// Endpoint API yang belum ada harus menjawab 404 dengan envelope OpenAI, bukan pesan
// soal dashboard yang belum di-build. Jawaban untuk endpoint salah tidak boleh
// berubah tergantung apakah frontend sudah tersemat.
func TestAPIPathsGet404NotDashboardMessage(t *testing.T) {
	r := newTestRouter(t, testConfig(config.EnvDevelopment))

	for _, path := range []string{
		"/v1/chat/completions",
		"/v1/models",
		"/v1/embeddings",
		"/api/requests",
	} {
		t.Run(path, func(t *testing.T) {
			rec := do(t, r, http.MethodPost, path)
			if rec.Code != http.StatusNotFound {
				t.Fatalf("status = %d, mau 404", rec.Code)
			}
			if strings.Contains(rec.Body.String(), "dashboard") {
				t.Errorf("permintaan API dijawab pesan dashboard: %s", rec.Body.String())
			}

			var body struct {
				Error struct {
					Message string `json:"message"`
					Type    string `json:"type"`
				} `json:"error"`
			}
			if err := json.Unmarshal(rec.Body.Bytes(), &body); err != nil {
				t.Fatalf("body bukan JSON: %v\n%s", err, rec.Body.String())
			}
			if body.Error.Type == "" || body.Error.Message == "" {
				t.Errorf("envelope error tidak lengkap: %s", rec.Body.String())
			}
		})
	}
}

// Path dashboard tetap mendapat penjelasan cara membangunnya.
func TestDashboardPathsExplainMissingBuild(t *testing.T) {
	r := newTestRouter(t, testConfig(config.EnvDevelopment))

	for _, path := range []string{"/", "/dashboard", "/requests"} {
		rec := do(t, r, http.MethodGet, path)
		if rec.Code != http.StatusServiceUnavailable {
			t.Errorf("%s status = %d, mau 503", path, rec.Code)
		}
		if !strings.Contains(rec.Body.String(), "dashboard_not_built") {
			t.Errorf("%s tidak menjelaskan penyebabnya: %s", path, rec.Body.String())
		}
	}
}

func TestIsAPIPath(t *testing.T) {
	tests := []struct {
		path string
		want bool
	}{
		{"/v1/chat/completions", true},
		{"/v1/", true},
		{"/api/requests", true},
		{"/v1", false}, // tanpa garis miring: ini bisa jadi rute dashboard
		{"/", false},
		{"/dashboard", false},
		{"/assets/index-abc123.js", false},
		{"", false},
	}
	for _, tc := range tests {
		if got := isAPIPath(tc.path); got != tc.want {
			t.Errorf("isAPIPath(%q) = %v, mau %v", tc.path, got, tc.want)
		}
	}
}

// Rantai middleware harus terpasang pada semua rute.
func TestMiddlewareChainApplied(t *testing.T) {
	r := newTestRouter(t, testConfig(config.EnvDevelopment), okChecker("postgres"))
	rec := do(t, r, http.MethodGet, "/healthz")

	required := []string{
		"X-Request-Id",
		"X-Content-Type-Options",
		"Referrer-Policy",
		"X-Frame-Options",
		"Content-Security-Policy",
		"Permissions-Policy",
	}
	for _, h := range required {
		if rec.Header().Get(h) == "" {
			t.Errorf("header %s tidak ada", h)
		}
	}
	// HSTS hanya di produksi.
	if rec.Header().Get("Strict-Transport-Security") != "" {
		t.Error("HSTS terpasang di development")
	}
}

func TestHSTSOnlyInProduction(t *testing.T) {
	r := newTestRouter(t, testConfig(config.EnvProduction), okChecker("postgres"))
	rec := do(t, r, http.MethodGet, "/healthz")

	if rec.Header().Get("Strict-Transport-Security") == "" {
		t.Error("HSTS tidak terpasang di produksi")
	}
}

func TestBuildRouterRejectsBadTrustedProxies(t *testing.T) {
	cfg := testConfig(config.EnvDevelopment)
	cfg.TrustedProxies = []string{"bukan-cidr"}

	if _, err := buildRouter(cfg, testLogger(), observability.NewMetrics(), nil); err == nil {
		t.Fatal("TRUSTED_PROXIES tidak valid seharusnya menggagalkan startup")
	}
}

func TestDashboardHandlerWithoutBuild(t *testing.T) {
	// Pada repo bersih, web/dist hanya memuat .gitkeep, jadi jalur "belum di-build"
	// inilah yang aktif.
	h, err := dashboardHandler(testLogger())
	if err != nil {
		t.Fatalf("dashboardHandler: %v", err)
	}

	rec := do(t, h, http.MethodGet, "/")
	if rec.Code != http.StatusServiceUnavailable && rec.Code != http.StatusOK {
		t.Errorf("status = %d, mau 503 (belum di-build) atau 200 (sudah di-build)", rec.Code)
	}
}

func TestDashboardURL(t *testing.T) {
	cfg := testConfig(config.EnvDevelopment)
	if got := dashboardURL(cfg, ":8080"); got != "http://localhost:8080" {
		t.Errorf("dashboardURL = %q", got)
	}

	cfg.PublicURL = "https://gateway.example.com"
	if got := dashboardURL(cfg, ":8080"); got != "https://gateway.example.com" {
		t.Errorf("dashboardURL dengan PublicURL = %q", got)
	}
}

func TestRuntimeVersion(t *testing.T) {
	if !strings.HasPrefix(runtimeVersion(), "go") {
		t.Errorf("runtimeVersion() = %q", runtimeVersion())
	}
}
