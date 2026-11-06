package docs_test

import (
	"net/http"
	"net/http/httptest"
	"reflect"
	"regexp"
	"strconv"
	"strings"
	"testing"

	"github.com/NexGen-X/Route-X/docs"
	"github.com/NexGen-X/Route-X/internal/database/repo/upstream"
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

// anakPeta menelusuri rangkaian kunci pada peta hasil urai YAML;
// mengembalikan nil bila ada kunci yang hilang atau simpul bukan peta.
func anakPeta(v any, kunci ...string) any {
	cur := v
	for _, k := range kunci {
		m, ok := cur.(map[string]any)
		if !ok {
			return nil
		}
		cur, ok = m[k]
		if !ok {
			return nil
		}
	}
	return cur
}

// TestOpenAPISemantikTerkunci (Fase 10 — B-232, B-233, B-234, B-235,
// B-236, B-237) mengunci kelas drift dokumen-terhadap-kode yang
// ditemukan Jalur Sapu: angka klaim cakupan, tag tak terdeklarasi,
// operasi admin tanpa security, envelope error permukaan Anthropic,
// semantik status tls-check, kelengkapan skema OAuthSessionMeta
// terhadap struct DTO-nya, dan enum status per-dependensi /readyz.
// Test B-230 hanya membuktikan dokumen TERURAI dan $ref teresolusi;
// test ini membuktikan isi dokumen COCOK dengan perilaku kode.
func TestOpenAPISemantikTerkunci(t *testing.T) {
	var doc map[string]any
	if err := yaml.Unmarshal(docs.OpenAPIYAML, &doc); err != nil {
		t.Fatalf("openapi.yaml harus YAML yang valid: %v", err)
	}
	paths, ok := doc["paths"].(map[string]any)
	if !ok {
		t.Fatal("blok paths tidak terurai")
	}
	metode := []string{"get", "post", "put", "patch", "delete", "head", "options", "trace"}
	type operasi struct {
		jalan, metode string
		isi           map[string]any
	}
	var semua []operasi
	for jalan, mentah := range paths {
		item, ok := mentah.(map[string]any)
		if !ok {
			continue
		}
		for _, m := range metode {
			if op, ok := item[m].(map[string]any); ok {
				semua = append(semua, operasi{jalan, m, op})
			}
		}
	}
	if len(semua) == 0 {
		t.Fatal("tidak ada operasi terurai dari blok paths")
	}

	t.Run("jumlah operasi sesuai klaim header (B-235)", func(t *testing.T) {
		admin := 0
		for _, op := range semua {
			if strings.HasPrefix(op.jalan, "/api/admin") {
				admin++
			}
		}
		deskripsi, _ := anakPeta(doc, "info", "description").(string)
		for _, pola := range []struct {
			re   *regexp.Regexp
			mau  int
			dari string
		}{
			{regexp.MustCompile(`/api/admin \((\d+) operasi\)`), admin, "klaim admin di info.description"},
			{regexp.MustCompile(`total (\d+) operasi`), len(semua), "klaim total di info.description"},
			{regexp.MustCompile(`seluruh (\d+) operasi terdaftar`), admin, "klaim banner seksi admin"},
		} {
			teks := deskripsi
			if pola.dari == "klaim banner seksi admin" {
				teks = string(docs.OpenAPIYAML)
			}
			m := pola.re.FindStringSubmatch(teks)
			if m == nil {
				t.Errorf("%s: pola klaim tidak ditemukan", pola.dari)
				continue
			}
			angka, _ := strconv.Atoi(m[1])
			if angka != pola.mau {
				t.Errorf("%s: dokumen mengklaim %d, hitungan terurai %d", pola.dari, angka, pola.mau)
			}
		}
	})

	t.Run("semua tag terdeklarasi (B-234)", func(t *testing.T) {
		terdeklarasi := map[string]bool{}
		if daftar, ok := doc["tags"].([]any); ok {
			for _, e := range daftar {
				if nama, ok := anakPeta(e, "name").(string); ok {
					terdeklarasi[nama] = true
				}
			}
		}
		for _, op := range semua {
			daftar, _ := op.isi["tags"].([]any)
			for _, e := range daftar {
				if nama, ok := e.(string); ok && !terdeklarasi[nama] {
					t.Errorf("operasi %s %s memakai tag tak terdeklarasi %q", op.metode, op.jalan, nama)
				}
			}
		}
	})

	t.Run("operasi admin wajib security (B-234)", func(t *testing.T) {
		for _, op := range semua {
			if !strings.HasPrefix(op.jalan, "/api/admin") {
				continue
			}
			sec, ok := op.isi["security"].([]any)
			if !ok || len(sec) == 0 {
				t.Errorf("operasi admin %s %s tanpa security — terbaca publik dalam semantik OpenAPI", op.metode, op.jalan)
			}
		}
	})

	t.Run("envelope error permukaan Anthropic (B-233)", func(t *testing.T) {
		// skemaError menyelesaikan respons berstatus ke $ref skema JSON-nya,
		// mengikuti $ref komponen respons bila ada.
		skemaError := func(op map[string]any, status string) string {
			resp := anakPeta(op, "responses", status)
			if ref, ok := anakPeta(resp, "$ref").(string); ok {
				resp = resolusiRefNilai(doc, ref)
			}
			ref, _ := anakPeta(resp, "content", "application/json", "schema", "$ref").(string)
			return ref
		}
		kasus := []struct {
			jalan, metode string
			wajib         []string
		}{
			{"/v1/messages", "post", []string{"400", "401", "402", "403", "404", "429", "502", "503", "504"}},
			{"/v1/v1/messages", "post", []string{"400", "401", "402", "403", "404", "429", "502", "503", "504"}},
			{"/v1/messages/count_tokens", "post", []string{"400", "401", "403", "404"}},
			{"/v1/v1/messages/count_tokens", "post", []string{"400", "401", "403", "404"}},
		}
		for _, k := range kasus {
			op, ok := anakPeta(paths, k.jalan, k.metode).(map[string]any)
			if !ok {
				t.Errorf("operasi %s %s tidak ditemukan", k.metode, k.jalan)
				continue
			}
			responses, _ := op["responses"].(map[string]any)
			for _, status := range k.wajib {
				if _, ada := responses[status]; !ada {
					t.Errorf("%s %s: status %s tidak didokumentasikan", k.metode, k.jalan, status)
					continue
				}
				if ref := skemaError(op, status); ref != "#/components/schemas/AnthropicErrorEnvelope" {
					t.Errorf("%s %s status %s: skema error %q, mau envelope Anthropic", k.metode, k.jalan, status, ref)
				}
			}
		}
	})

	t.Run("tls-check mendokumentasikan 403 dan 429 (B-232)", func(t *testing.T) {
		op, ok := anakPeta(paths, "/api/internal/tls-check", "get").(map[string]any)
		if !ok {
			t.Fatal("operasi tls-check tidak ditemukan")
		}
		responses, _ := op["responses"].(map[string]any)
		for _, status := range []string{"200", "400", "403", "429"} {
			if _, ada := responses[status]; !ada {
				t.Errorf("tls-check: status %s tidak didokumentasikan", status)
			}
		}
	})

	t.Run("skema OAuthSessionMeta selengkap struct DTO (B-236)", func(t *testing.T) {
		props, ok := anakPeta(doc, "components", "schemas", "OAuthSessionMeta", "properties").(map[string]any)
		if !ok {
			t.Fatal("skema OAuthSessionMeta tidak terurai")
		}
		tipe := reflect.TypeOf(upstream.OAuthSessionMeta{})
		for i := 0; i < tipe.NumField(); i++ {
			tag := tipe.Field(i).Tag.Get("json")
			nama, _, _ := strings.Cut(tag, ",")
			if nama == "" || nama == "-" {
				continue
			}
			if _, ada := props[nama]; !ada {
				t.Errorf("skema OAuthSessionMeta kehilangan field DTO %q", nama)
			}
		}
	})

	t.Run("enum status per-dependensi readyz hanya ok/down (B-237)", func(t *testing.T) {
		for _, status := range []string{"200", "503"} {
			enumMentah := anakPeta(paths, "/readyz", "get", "responses", status,
				"content", "application/json", "schema", "properties", "checks",
				"additionalProperties", "properties", "status", "enum")
			daftar, ok := enumMentah.([]any)
			if !ok {
				t.Errorf("readyz %s: enum status per-dependensi tidak terurai", status)
				continue
			}
			for _, e := range daftar {
				if e == "degraded" {
					t.Errorf("readyz %s: enum per-dependensi memuat degraded — kode hanya menulis ok/down per check", status)
				}
			}
		}
	})
}

// resolusiRefNilai seperti resolusiRef tetapi mengembalikan simpul
// targetnya (nil bila menggantung).
func resolusiRefNilai(doc map[string]any, ref string) any {
	cur := any(doc)
	for _, bagian := range strings.Split(strings.TrimPrefix(ref, "#/"), "/") {
		bagian = strings.ReplaceAll(strings.ReplaceAll(bagian, "~1", "/"), "~0", "~")
		m, ok := cur.(map[string]any)
		if !ok {
			return nil
		}
		cur, ok = m[bagian]
		if !ok {
			return nil
		}
	}
	return cur
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
