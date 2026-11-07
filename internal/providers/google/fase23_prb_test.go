package google

// fase23_prb_test.go — pengunci Fase 23 PR-B (sisi Google),
// keluarga perlakuan DATA pada normalizeSchema:
//
//	B-533 ekstensi vendor "x-*" dan setiap kunci di luar
//	      kosakata adalah data (anotasi) — diteruskan utuh,
//	      tidak direkursi sebagai node skema.
//	B-540 "$vocabulary"/"externalDocs"/"xml" masuk kasus data
//	      yang sama; klaim "lolos utuh" Fase 22 kini benar juga
//	      pada bentuk bermusuhan.
//
// Semua test memanggil sanitizeSchema — pintu masuk nyata
// normalisasi skema tool untuk provider Google.

import (
	"encoding/json"
	"reflect"
	"testing"
)

// TestB533EkstensiVendorData mengunci B-533 pada bentuk probe
// Putaran #10: nilai "x-custom" yang memuat entri bernama kata
// kunci ("not", "$ref", "const", daftar "type", objek bersarang)
// diteruskan UTUH sebagai data — dulu "not"/"$ref" hilang,
// "const" menjadi enum karangan, daftar runtuh, objek bersarang
// menjadi {}. Bentuk bersarang di dalam properti sama.
func TestB533EkstensiVendorData(t *testing.T) {
	schema := json.RawMessage(`{
		"type": "object",
		"properties": {
			"a": {"type": "string", "x-vendor": {"not": "data", "type": "data"}}
		},
		"x-custom": {"not": 1, "type": ["a"], "const": 2, "$ref": "r", "format": "f", "nested": {"not": {"type": "dalam"}}}
	}`)
	out := uraiHasil(t, sanitizeSchema(schema))

	mau := map[string]any{
		"not":    float64(1),
		"type":   []any{"a"},
		"const":  float64(2),
		"$ref":   "r",
		"format": "f",
		"nested": map[string]any{"not": map[string]any{"type": "dalam"}},
	}
	if !reflect.DeepEqual(out["x-custom"], mau) {
		t.Errorf("x-custom = %v, mau utuh sebagai data %v", out["x-custom"], mau)
	}
	props, ok := out["properties"].(map[string]any)
	if !ok {
		t.Fatalf("properties hilang/berubah bentuk: %v", out)
	}
	a, ok := props["a"].(map[string]any)
	if !ok {
		t.Fatalf("properties.a hilang/berubah bentuk: %v", props)
	}
	mauVendor := map[string]any{"not": "data", "type": "data"}
	if !reflect.DeepEqual(a["x-vendor"], mauVendor) {
		t.Errorf("properties.a.x-vendor = %v, mau utuh %v", a["x-vendor"], mauVendor)
	}
	if a["type"] != "string" {
		t.Errorf("properties.a.type = %v, mau string (kontrol: node skema tetap dinormalisasi)", a["type"])
	}
}

// TestB533KunciAsingData mengunci kelas terbuka kedua B-533:
// kunci yang sama sekali di luar kosakata JSON Schema/OpenAPI
// adalah anotasi bernilai data bebas — peta di bawahnya utuh,
// termasuk entri bernama kata kunci di dalamnya. Kontrol arah
// sebaliknya: kata kunci aplikator yang MEMANG pembawa subskema
// (allOf) tetap direkursi dan dinormalisasi sebagai node.
func TestB533KunciAsingData(t *testing.T) {
	schema := json.RawMessage(`{
		"type": "object",
		"petunjukKustom": {"not": {"type": ["a", "b"]}, "const": 7, "daftar": [{"$ref": "r"}]},
		"allOf": [{"type": ["string", "null"], "additionalProperties": false}]
	}`)
	out := uraiHasil(t, sanitizeSchema(schema))

	mau := map[string]any{
		"not":   map[string]any{"type": []any{"a", "b"}},
		"const": float64(7),
		"daftar": []any{
			map[string]any{"$ref": "r"},
		},
	}
	if !reflect.DeepEqual(out["petunjukKustom"], mau) {
		t.Errorf("petunjukKustom = %v, mau utuh sebagai data %v", out["petunjukKustom"], mau)
	}
	allOf, ok := out["allOf"].([]any)
	if !ok || len(allOf) != 1 {
		t.Fatalf("allOf hilang/berubah bentuk: %v", out)
	}
	elemen, ok := allOf[0].(map[string]any)
	if !ok {
		t.Fatalf("allOf[0] bukan node: %v", allOf)
	}
	if elemen["type"] != "string" || elemen["nullable"] != true {
		t.Errorf("allOf[0] = %v, mau type terjemahan \"string\" + nullable true (kontrol: aplikator tetap node)", elemen)
	}
	if _, ada := elemen["additionalProperties"]; ada {
		t.Errorf("additionalProperties di dalam allOf[0] masih ada — aplikator tidak dinormalisasi sebagai node: %v", elemen)
	}
}

// TestB540MetadataOpenAPIData mengunci B-540: "$vocabulary",
// "externalDocs", dan "xml" kini kasus data — bentuk bermusuhan
// probe Putaran #10 (entri bernama "not"/"$ref"/"const" di
// bawahnya) diteruskan utuh, dan bentuk sahnya tetap utuh.
func TestB540MetadataOpenAPIData(t *testing.T) {
	schema := json.RawMessage(`{
		"type": "object",
		"$vocabulary": {"not": true, "type": false, "$ref": true, "format": true, "const": true},
		"externalDocs": {"description": "dok", "url": "https://contoh.invalid/dok", "not": 1, "const": 2},
		"xml": {"name": "akar", "not": {"type": "dalam"}}
	}`)
	out := uraiHasil(t, sanitizeSchema(schema))

	mauVocab := map[string]any{
		"not": true, "type": false, "$ref": true, "format": true, "const": true,
	}
	if !reflect.DeepEqual(out["$vocabulary"], mauVocab) {
		t.Errorf("$vocabulary = %v, mau utuh sebagai data %v", out["$vocabulary"], mauVocab)
	}
	mauDocs := map[string]any{
		"description": "dok", "url": "https://contoh.invalid/dok",
		"not": float64(1), "const": float64(2),
	}
	if !reflect.DeepEqual(out["externalDocs"], mauDocs) {
		t.Errorf("externalDocs = %v, mau utuh sebagai data %v", out["externalDocs"], mauDocs)
	}
	mauXML := map[string]any{
		"name": "akar", "not": map[string]any{"type": "dalam"},
	}
	if !reflect.DeepEqual(out["xml"], mauXML) {
		t.Errorf("xml = %v, mau utuh sebagai data %v", out["xml"], mauXML)
	}
}
