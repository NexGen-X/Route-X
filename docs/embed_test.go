package docs_test

import (
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/NexGen-X/Route-X/docs"
)

func TestDocsEmbeddedYAML(t *testing.T) {
	if len(docs.OpenAPIYAML) == 0 {
		t.Fatal("OpenAPI YAML yang disematkan tidak boleh kosong")
	}

	content := string(docs.OpenAPIYAML)
	if !strings.Contains(content, "openapi: 3.1.0") {
		t.Errorf("spesifikasi OpenAPI harus versi 3.1.0, didapat:\n%.100s", content)
	}
	if !strings.Contains(content, "Route-X AI Gateway API") {
		t.Errorf("judul dokumentasi tidak sesuai")
	}
}

func TestDocsHandlerEndpoints(t *testing.T) {
	handler := docs.Handler()

	tests := []struct {
		name         string
		path         string
		wantCode     int
		wantContains string
	}{
		{
			name:         "UI Dokumentasi HTML",
			path:         "/docs",
			wantCode:     http.StatusOK,
			wantContains: "Route-X — Dokumentasi API Gateway",
		},
		{
			name:         "UI Dokumentasi HTML dengan trailing slash",
			path:         "/docs/",
			wantCode:     http.StatusOK,
			wantContains: "scalar",
		},
		{
			name:         "File Mentah openapi.yaml",
			path:         "/docs/openapi.yaml",
			wantCode:     http.StatusOK,
			wantContains: "openapi: 3.1.0",
		},
		{
			name:         "Script Lokal scalar.standalone.js",
			path:         "/docs/scalar.standalone.js",
			wantCode:     http.StatusOK,
			wantContains: "Scalar",
		},
		{
			name:         "Path tidak dikenal",
			path:         "/docs/tidak-ada",
			wantCode:     http.StatusNotFound,
			wantContains: "404",
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			req := httptest.NewRequest(http.MethodGet, tc.path, nil)
			rec := httptest.NewRecorder()
			handler.ServeHTTP(rec, req)

			if rec.Code != tc.wantCode {
				t.Fatalf("path %s status = %d, mau %d", tc.path, rec.Code, tc.wantCode)
			}
			if !strings.Contains(rec.Body.String(), tc.wantContains) {
				t.Errorf("respon tidak memuat %q", tc.wantContains)
			}
		})
	}
}
