package google

// fase22_prc_test.go — pengunci Fase 22 PR-C (sisi Google),
// keluarga sapuan mekanis kata kunci bernilai-peta/data pada
// normalizeSchema:
//
//	B-524 dependencies (wadah campuran nama → skema ATAU daftar
//	      nama) diperlakukan sebagai wadah, dan unevaluatedItems
//	      diputuskan konsisten dengan unevaluatedProperties:
//	      dibuang (keluarga aplikator "unevaluated*" di luar
//	      subset OpenAPI 3.0 Gemini, alasan grup B-161).
//	B-528 propertyDependencies adalah data murni
//	      (nama → nama → daftar nama) — diteruskan utuh.
//	B-529 discriminator.mapping adalah data (nama → string) —
//	      objek discriminator diteruskan utuh.
//
// Sapuan mekanis lengkapnya (semua kata kunci bernilai-peta/data
// dalam spek JSON Schema draft 4–2020-12 + objek skema OpenAPI)
// ada di badan PR. Semua test memanggil sanitizeSchema — pintu
// masuk nyata normalisasi skema tool untuk provider Google.

import (
	"encoding/json"
	"reflect"
	"testing"
)

// TestB524DependenciesWadahCampuran mengunci B-524: dependencies
// adalah wadah nama → (skema | daftar nama). Nama entri yang
// kebetulan kata kunci tidak boleh ditafsirkan: entri "not"
// berbentuk daftar selamat persis (dulu terhapus oleh saringan
// node), entri "type" berbentuk skema dinormalkan sebagai node
// (dulu lolos mentah lewat kasus type), dan bentuk daftar biasa
// tetap lolos sebagai daun.
func TestB524DependenciesWadahCampuran(t *testing.T) {
	schema := json.RawMessage(`{
		"type": "object",
		"properties": {
			"a": {"type": "string"},
			"b": {"type": "string"},
			"x": {"type": "string"}
		},
		"dependencies": {
			"not": ["a"],
			"type": {
				"properties": {"x": {"type": "string"}},
				"required": ["x"],
				"additionalProperties": false
			},
			"biasa": ["not", "type"]
		}
	}`)
	out := uraiHasil(t, sanitizeSchema(schema))

	dep, ok := out["dependencies"].(map[string]any)
	if !ok {
		t.Fatalf("dependencies hilang/berubah bentuk: %v", out)
	}
	if !reflect.DeepEqual(dep["not"], []any{"a"}) {
		t.Errorf("dependencies[\"not\"] = %v, mau [a] — entri bernama kata kunci tidak boleh terhapus/ditafsirkan", dep["not"])
	}
	mauSkema := map[string]any{
		"properties": map[string]any{"x": map[string]any{"type": "string"}},
		"required":   []any{"x"},
	}
	if !reflect.DeepEqual(dep["type"], mauSkema) {
		t.Errorf("dependencies[\"type\"] = %v, mau skema ternormalisasi %v (additionalProperties di dalamnya ikut terbuang sebagai node)", dep["type"], mauSkema)
	}
	if !reflect.DeepEqual(dep["biasa"], []any{"not", "type"}) {
		t.Errorf("dependencies[\"biasa\"] = %v, mau [not type] utuh sebagai daftar nama", dep["biasa"])
	}
}

// TestB524UnevaluatedItemsKonsisten mengunci keputusan B-524 atas
// inkonsistensi yang dicatat Putaran #9: unevaluatedItems kini
// dibuang persis seperti saudaranya unevaluatedProperties (yang
// sudah dibuang sejak B-161), bukan diteruskan.
func TestB524UnevaluatedItemsKonsisten(t *testing.T) {
	schema := json.RawMessage(`{
		"type": "object",
		"properties": {"a": {"type": "string"}},
		"unevaluatedProperties": false,
		"unevaluatedItems": false,
		"items": {"type": "string", "unevaluatedItems": {"type": "number"}}
	}`)
	out := uraiHasil(t, sanitizeSchema(schema))

	if _, ada := out["unevaluatedProperties"]; ada {
		t.Errorf("unevaluatedProperties masih ada di keluaran: %v", out)
	}
	if _, ada := out["unevaluatedItems"]; ada {
		t.Errorf("unevaluatedItems masih ada di keluaran — harus dibuang konsisten dgn unevaluatedProperties: %v", out)
	}
	items, ok := out["items"].(map[string]any)
	if !ok {
		t.Fatalf("items hilang/berubah bentuk: %v", out)
	}
	if _, ada := items["unevaluatedItems"]; ada {
		t.Errorf("unevaluatedItems bersarang masih ada di keluaran: %v", items)
	}
	if items["type"] != "string" {
		t.Errorf("items.type = %v, mau string (kontrol: kata kunci sah utuh)", items["type"])
	}
}

// TestB528PropertyDependenciesData mengunci B-528:
// propertyDependencies adalah data murni nama → nama → daftar
// nama; seluruh nilainya diteruskan byte-setara sebagai data,
// termasuk nama-nama bertabrakan dgn kata kunci di kedua tingkat.
func TestB528PropertyDependenciesData(t *testing.T) {
	schema := json.RawMessage(`{
		"type": "object",
		"propertyDependencies": {
			"not": {"type": ["a", "b"]},
			"biasa": {"x": ["not"]}
		}
	}`)
	out := uraiHasil(t, sanitizeSchema(schema))

	mau := map[string]any{
		"not":   map[string]any{"type": []any{"a", "b"}},
		"biasa": map[string]any{"x": []any{"not"}},
	}
	if !reflect.DeepEqual(out["propertyDependencies"], mau) {
		t.Errorf("propertyDependencies = %v, mau utuh %v (data, bukan skema)", out["propertyDependencies"], mau)
	}
}

// TestB529DiscriminatorData mengunci B-529: objek discriminator
// OpenAPI — khususnya mapping nama varian → string — adalah data
// dan diteruskan utuh; nama varian yang kebetulan kata kunci
// ("not", "type") tidak boleh terhapus/berubah.
func TestB529DiscriminatorData(t *testing.T) {
	schema := json.RawMessage(`{
		"type": "object",
		"properties": {"jenis": {"type": "string"}},
		"discriminator": {
			"propertyName": "jenis",
			"mapping": {"not": "ModulNot", "type": "ModulType", "biasa": "ModulBiasa"}
		}
	}`)
	out := uraiHasil(t, sanitizeSchema(schema))

	mau := map[string]any{
		"propertyName": "jenis",
		"mapping": map[string]any{
			"not":   "ModulNot",
			"type":  "ModulType",
			"biasa": "ModulBiasa",
		},
	}
	if !reflect.DeepEqual(out["discriminator"], mau) {
		t.Errorf("discriminator = %v, mau utuh %v (mapping adalah data)", out["discriminator"], mau)
	}
}
