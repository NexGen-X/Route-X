package cliconfig

import (
	"encoding/json"
	"strings"
	"testing"
)

// Test pengunci unit penyunting JSONC sadar-teks (B-280):
// penetapan, penyisipan, dan penghapusan kunci pada teks JSONC
// mempertahankan komentar dan entri lain apa adanya.

func TestJSONCEditTetapkanNilaiAda(t *testing.T) {
	s := "{\n  // catatan\n  \"a\": 1,\n  \"b\": \"lama\"\n}\n"
	akar, err := akarObjekJSONC(s)
	if err != nil {
		t.Fatal(err)
	}
	hasil, err := tetapkanKunciJSONC(s, akar, "b", `"baru"`)
	if err != nil {
		t.Fatal(err)
	}
	harusMemuat(t, hasil, "// catatan")
	harusMemuat(t, hasil, `"b": "baru"`)
	harusMemuat(t, hasil, `"a": 1`)
}

func TestJSONCEditSisipKunciBaruMengikutiIndentasi(t *testing.T) {
	s := "{\n    \"a\": 1\n}\n"
	akar, err := akarObjekJSONC(s)
	if err != nil {
		t.Fatal(err)
	}
	hasil, err := tetapkanKunciJSONC(s, akar, "b", `{"x": true}`)
	if err != nil {
		t.Fatal(err)
	}
	harusMemuat(t, hasil, "    \"a\": 1,\n    \"b\": {\"x\": true}\n}")
	// Hasil harus terurai toleran menjadi peta yang benar.
	cfg := make(map[string]any)
	if err := json.Unmarshal(uraiJSONCToleran([]byte(hasil)), &cfg); err != nil {
		t.Fatalf("hasil sisip tidak terurai: %v\n%s", err, hasil)
	}
	if cfg["a"] != float64(1) {
		t.Fatalf("nilai a berubah: %v", cfg)
	}
}

func TestJSONCEditSisipKeObjekKosongBerkomentar(t *testing.T) {
	s := "{ /* kosong */ }"
	akar, err := akarObjekJSONC(s)
	if err != nil {
		t.Fatal(err)
	}
	hasil, err := tetapkanKunciJSONC(s, akar, "a", "1")
	if err != nil {
		t.Fatal(err)
	}
	harusMemuat(t, hasil, "/* kosong */")
	harusMemuat(t, hasil, `"a": 1`)
	if _, _, err := uraianObjekJSONC(hasil, 0); err != nil {
		t.Fatalf("hasil tidak terurai: %v\n%s", err, hasil)
	}
}

func TestJSONCEditHapusKunci(t *testing.T) {
	kasus := []struct {
		nama  string
		s     string
		kunci string
	}{
		{"tengah", "{\n  \"a\": 1,\n  \"b\": 2,\n  \"c\": 3\n}\n", "b"},
		{"pertama", "{\n  \"a\": 1,\n  \"b\": 2\n}\n", "a"},
		{"terakhir", "{\n  \"a\": 1,\n  \"b\": 2\n}\n", "b"},
		{"satu-satunya", "{\n  \"a\": 1\n}\n", "a"},
	}
	for _, k := range kasus {
		t.Run(k.nama, func(t *testing.T) {
			akar, err := akarObjekJSONC(k.s)
			if err != nil {
				t.Fatal(err)
			}
			hasil, err := hapusKunciJSONC(k.s, akar, k.kunci)
			if err != nil {
				t.Fatal(err)
			}
			if strings.Contains(hasil, `"`+k.kunci+`"`) {
				t.Errorf("kunci %q masih ada:\n%s", k.kunci, hasil)
			}
			// Hasil wajib tetap JSONC yang terurai (koma menggantung
			// pun ditoleransi pengurai).
			cfg := make(map[string]any)
			if err := json.Unmarshal(uraiJSONCToleran([]byte(hasil)), &cfg); err != nil {
				t.Fatalf("hasil hapus tidak terurai: %v\n%s", err, hasil)
			}
		})
	}
}

func TestJSONCMengandungKomentar(t *testing.T) {
	if jsoncMengandungKomentar(`{"u": "https://contoh.invalid/v1"}`) {
		t.Error("URL di dalam string dianggap komentar")
	}
	if !jsoncMengandungKomentar("{\n// halo\n}") {
		t.Error("komentar baris tidak terdeteksi")
	}
	if !jsoncMengandungKomentar("{ /* halo */ }") {
		t.Error("komentar blok tidak terdeteksi")
	}
	if jsoncMengandungKomentar(`{"a": 1}`) {
		t.Error("JSON polos dianggap berkomentar")
	}
}
