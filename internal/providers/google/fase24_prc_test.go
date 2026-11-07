package google

// fase24_prc_test.go — pengunci Fase 24 PR-C (skema Gemini di
// translate.go):
//
//	B-542 tabrakan penulisan turunan union-type vs "nullable"
//	      eksplisit: aturan prioritas DETERMINISTIK — nullable
//	      eksplisit pada node yang sama selalu menang atas
//	      turunan union (pola penjaga yang sama seperti
//	      const→enum dan exclusive*→minimum/maximum). Sapuan
//	      penulisan turunan lain di normalisasiNode ada di badan
//	      PR: nullable adalah satu-satunya yang tak berpenjaga.
//	B-549 "$dynamicRef"/"$recursiveRef" DIBUANG (masuk
//	      unsupportedSchemaKeys): pipa ini tidak pernah
//	      mendereferensi referensi dinamis — $ref non-lokal pun
//	      dibuang — dan namanya di luar subset OpenAPI 3.0
//	      Gemini; meneruskannya adalah referensi menggantung
//	      yang namanya ditolak validator. Tabel kelas di
//	      doc-comment normalizeSchema kini juga menamai status
//	      kunci dalam-kosakata yang tak terdaftar (kelas DATA
//	      lewat cabang bawaan) — koreksi redaksi B-533.
//
// Semua test memanggil sanitizeSchema — pintu masuk nyata
// normalisasi skema tool untuk provider Google.

import (
	"encoding/json"
	"testing"
)

// TestB542NullableEksplisitSelaluMenang mengunci B-542 dengan
// pengulangan banyak lari: iterasi peta Go bersifat acak, jadi
// satu lari tidak membuktikan apa pun — 500 lari atas skema yang
// sama wajib menghasilkan nullable eksplisit yang sama persis.
// Sebelum perbaikan, sebagian lari menghasilkan true (turunan
// union menimpa false eksplisit).
func TestB542NullableEksplisitSelaluMenang(t *testing.T) {
	schema := json.RawMessage(`{"type": ["string", "null"], "nullable": false}`)
	for i := 0; i < 500; i++ {
		out := uraiHasil(t, sanitizeSchema(schema))
		if out["nullable"] != false {
			t.Fatalf("lari %d: nullable = %v, mau false (eksplisit menang atas turunan union)", i, out["nullable"])
		}
		if out["type"] != "string" {
			t.Fatalf("lari %d: type = %v, mau string", i, out["type"])
		}
	}
}

// TestB542NullableTurunanDanEksplisitLain mengunci arah-arah
// lain aturan B-542: union ber-null TANPA nullable eksplisit
// tetap menurunkan true; nullable eksplisit true tanpa null di
// union tetap true (diteruskan sebagai data); union tanpa null
// dan tanpa eksplisit tidak mengarang nullable.
func TestB542NullableTurunanDanEksplisitLain(t *testing.T) {
	out := uraiHasil(t, sanitizeSchema(json.RawMessage(`{"type": ["integer", "null"]}`)))
	if out["nullable"] != true || out["type"] != "integer" {
		t.Errorf("union ber-null tanpa eksplisit: %v, mau type=integer nullable=true", out)
	}

	out = uraiHasil(t, sanitizeSchema(json.RawMessage(`{"type": ["string"], "nullable": true}`)))
	if out["nullable"] != true {
		t.Errorf("eksplisit true tanpa null di union: %v, mau nullable=true", out)
	}

	out = uraiHasil(t, sanitizeSchema(json.RawMessage(`{"type": ["string"], "nullable": false}`)))
	if out["nullable"] != false {
		t.Errorf("eksplisit false tanpa null di union: %v, mau nullable=false", out)
	}

	out = uraiHasil(t, sanitizeSchema(json.RawMessage(`{"type": ["boolean"]}`)))
	if _, ada := out["nullable"]; ada {
		t.Errorf("union tanpa null tanpa eksplisit mengarang nullable: %v", out)
	}
}

// TestB549ReferensiDinamisDibuang mengunci B-549: $dynamicRef
// dan $recursiveRef dibuang di posisi node (termasuk bersarang
// di properties), saudara kandung skemanya tetap berlaku — sama
// seperti $ref non-lokal yang sudah lebih dulu dibuang.
func TestB549ReferensiDinamisDibuang(t *testing.T) {
	schema := json.RawMessage(`{
		"type": "object",
		"$dynamicRef": "#meta",
		"properties": {
			"anak": {"$recursiveRef": "#", "type": "string"},
			"tetap": {"type": "integer"}
		}
	}`)
	out := uraiHasil(t, sanitizeSchema(schema))
	if _, ada := out["$dynamicRef"]; ada {
		t.Errorf("$dynamicRef lolos di akar: %v", out)
	}
	props, ok := out["properties"].(map[string]any)
	if !ok {
		t.Fatalf("properties hilang: %v", out)
	}
	anak, ok := props["anak"].(map[string]any)
	if !ok {
		t.Fatalf("properties.anak hilang: %v", props)
	}
	if _, ada := anak["$recursiveRef"]; ada {
		t.Errorf("$recursiveRef lolos bersarang: %v", anak)
	}
	if anak["type"] != "string" {
		t.Errorf("saudara kandung $recursiveRef tidak dipertahankan: %v", anak)
	}
	tetap, ok := props["tetap"].(map[string]any)
	if !ok || tetap["type"] != "integer" {
		t.Errorf("properti lain rusak: %v", props)
	}
}
