package docs_test

import (
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/NexGen-X/Route-X/docs"
	"go.yaml.in/yaml/v3"
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

// TestOpenAPIYAMLValidDanRefTeresolusi (B-230): dokumen harus
// benar-benar TERURAI sebagai YAML — pemeriksaan strings.Contains
// lama membiarkan dokumen yang tidak bisa di-parse oleh klien
// OpenAPI mana pun lolos CI (dua skalar polos memuat ": " di dalam
// kurung kurawal). Setiap $ref internal juga harus teresolusi ke
// simpul nyata; test ini menangkap satu ref menggantung
// (#/components/responses/Error) yang ikut diperbaiki bersama B-230.
func TestOpenAPIYAMLValidDanRefTeresolusi(t *testing.T) {
	var doc map[string]any
	if err := yaml.Unmarshal(docs.OpenAPIYAML, &doc); err != nil {
		t.Fatalf("openapi.yaml harus YAML yang valid: %v", err)
	}
	if got := doc["openapi"]; got != "3.1.0" {
		t.Errorf("versi openapi = %v, mau 3.1.0", got)
	}
	paths, ok := doc["paths"].(map[string]any)
	if !ok || len(paths) == 0 {
		t.Fatal("blok paths kosong atau tidak ada — dokumen tidak terurai sebagaimana mestinya")
	}

	var refs []string
	var kumpulkan func(v any)
	kumpulkan = func(v any) {
		switch n := v.(type) {
		case map[string]any:
			for k, val := range n {
				if k == "$ref" {
					if s, ok := val.(string); ok {
						refs = append(refs, s)
					}
				}
				kumpulkan(val)
			}
		case []any:
			for _, e := range n {
				kumpulkan(e)
			}
		}
	}
	kumpulkan(doc)
	if len(refs) == 0 {
		t.Fatal("tidak ada $ref terkumpul — dokumen tidak terurai sebagaimana mestinya")
	}
	for _, ref := range refs {
		if !strings.HasPrefix(ref, "#/") {
			t.Errorf("$ref eksternal tidak dipakai dokumen ini: %s", ref)
			continue
		}
		if !resolusiRef(doc, ref) {
			t.Errorf("$ref menggantung (target tidak ada): %s", ref)
		}
	}
}

// resolusiRef menelusuri JSON Pointer internal (#/a/b/...) di dalam
// dokumen hasil urai, mengikuti unescape ~1 dan ~0.
func resolusiRef(doc map[string]any, ref string) bool {
	cur := any(doc)
	for _, bagian := range strings.Split(strings.TrimPrefix(ref, "#/"), "/") {
		bagian = strings.ReplaceAll(strings.ReplaceAll(bagian, "~1", "/"), "~0", "~")
		m, ok := cur.(map[string]any)
		if !ok {
			return false
		}
		cur, ok = m[bagian]
		if !ok {
			return false
		}
	}
	return true
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
