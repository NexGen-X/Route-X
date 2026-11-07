package google

// fase21_prc_test.go — pengunci Fase 21 PR-C (sisi Google).
//
//	B-503 dependentRequired diperlakukan sebagai wadah nama →
//	      daftar, bukan node skema.
//	B-504 nilai DATA di enum/default/example/examples tidak
//	      direkursi sebagai skema.
//
// Semua test memanggil sanitizeSchema — pintu masuk nyata
// normalisasi skema tool untuk provider Google.

import (
	"encoding/json"
	"reflect"
	"testing"
)

// uraiHasil membaca keluaran sanitizeSchema sebagai nilai generik.
func uraiHasil(t *testing.T, raw json.RawMessage) map[string]any {
	t.Helper()
	var out map[string]any
	if err := json.Unmarshal(raw, &out); err != nil {
		t.Fatalf("keluaran sanitizeSchema bukan JSON sah: %v (%s)", err, raw)
	}
	return out
}

// TestB503DependentRequiredWadah mengunci B-503: dependentRequired
// adalah wadah nama → daftar-nama. Entri bernama kata kunci ("not",
// "type") selamat persis; sebelum perbaikan entri "not" terhapus
// dan entri "type" berubah dari daftar menjadi skalar.
func TestB503DependentRequiredWadah(t *testing.T) {
	schema := json.RawMessage(`{
		"type": "object",
		"properties": {"a": {"type": "string"}, "b": {"type": "string"}},
		"dependentRequired": {"not": ["a"], "type": ["b"], "biasa": ["not"]}
	}`)
	out := uraiHasil(t, sanitizeSchema(schema))

	dr, ok := out["dependentRequired"].(map[string]any)
	if !ok {
		t.Fatalf("dependentRequired hilang/berubah bentuk: %v", out)
	}
	mau := map[string][]any{
		"not":   {"a"},
		"type":  {"b"},
		"biasa": {"not"},
	}
	for nama, daftar := range mau {
		if !reflect.DeepEqual(dr[nama], daftar) {
			t.Errorf("dependentRequired[%q] = %v, mau %v (nama entri tidak boleh ditafsirkan sebagai kata kunci)", nama, dr[nama], daftar)
		}
	}
}

// TestB504NilaiDataBukanSkema mengunci B-504: objek di dalam enum
// dan objek default adalah DATA — kunci di dalamnya (termasuk yang
// bernama "const"/"not"/"type") tidak boleh ditafsirkan sebagai
// kata kunci skema.
func TestB504NilaiDataBukanSkema(t *testing.T) {
	schema := json.RawMessage(`{
		"type": "object",
		"properties": {
			"mode": {
				"type": "string",
				"enum": [{"const": 5}, {"not": "x"}, "polos"],
				"default": {"not": "x", "const": 1, "type": "y"}
			},
			"contoh": {"examples": [{"type": "objek-data", "not": true}]}
		}
	}`)
	out := uraiHasil(t, sanitizeSchema(schema))

	props := out["properties"].(map[string]any)
	mode := props["mode"].(map[string]any)

	enumMau := []any{
		map[string]any{"const": float64(5)},
		map[string]any{"not": "x"},
		"polos",
	}
	if !reflect.DeepEqual(mode["enum"], enumMau) {
		t.Errorf("enum = %v, mau utuh %v (isi enum adalah data, bukan skema)", mode["enum"], enumMau)
	}
	defaultMau := map[string]any{"not": "x", "const": float64(1), "type": "y"}
	if !reflect.DeepEqual(mode["default"], defaultMau) {
		t.Errorf("default = %v, mau utuh %v (objek default adalah data, bukan skema)", mode["default"], defaultMau)
	}
	contoh := props["contoh"].(map[string]any)
	examplesMau := []any{map[string]any{"type": "objek-data", "not": true}}
	if !reflect.DeepEqual(contoh["examples"], examplesMau) {
		t.Errorf("examples = %v, mau utuh %v", contoh["examples"], examplesMau)
	}
}
