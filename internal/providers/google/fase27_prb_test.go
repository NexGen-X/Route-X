package google

// fase27_prb_test.go — pengunci Fase 27 PR-B (skema Gemini di
// translate.go):
//
//	B-584 gabung exclusiveMinimum/exclusiveMaximum numerik
//	      dengan batas eksplisit numerik menjadi batas
//	      efektif — arti konjungsi dipertahankan eksak.
//	B-585 segmen indeks larik di cariPointerLokal (RFC 6901).
//	B-586 dekode persen tiap segmen pointer SESUDAH
//	      pemisahan atas "/" (url.PathUnescape).
//
// Semua test memanggil sanitizeSchema — pintu masuk nyata
// normalisasi skema tool untuk provider Google — sehingga
// jalur yang dinamai tiap butir (normalizeSchema /
// resolusiRef / cariPointerLokal) sungguh dijalankan.

import (
	"reflect"
	"testing"
)

// nodeSkema menjalankan sanitizeSchema atas satu node skema dan
// mengembalikannya sebagai peta.
func nodeSkema(t *testing.T, raw string) map[string]any {
	t.Helper()
	doc := dekodeSkema(t, raw)
	peta, ok := doc.(map[string]any)
	if !ok {
		t.Fatalf("hasil sanitizeSchema bukan peta: %v (%s)", doc, raw)
	}
	return peta
}

// TestB584BatasEfektifExclusive mengunci keenam baris tabel
// B-584 + kasus tanpa-batas: batas efektif adalah batas yang
// LEBIH KETAT dari konjungsi kedua kunci — sisi minimum
// memakai nilai TERBESAR dengan ketegasan asalnya (eksklusif
// bila nilai terbesar itu datang dari exclusiveMinimum),
// cermin untuk sisi maximum (nilai TERKECIL).
//
// Diulang 200 lari: hasil tidak boleh bergantung pada urutan
// iterasi peta Go (kelas cacat B-542 — dua kunci menulis
// keluaran yang sama).
func TestB584BatasEfektifExclusive(t *testing.T) {
	kasus := []struct {
		nama      string
		skema     string
		batas     string // "minimum" atau "maximum"
		eksklusif string // "exclusiveMinimum" atau "exclusiveMaximum"
		mauBatas  float64
		mauTanda  bool // true = kunci eksklusif harus ada bernilai true; false = kunci harus DIBUANG
	}{
		// Sisi minimum: eksklusif lebih besar → ia yang berlaku, eksklusif.
		{"min 5 + exclMin 10 → >10", `{"type":"number","minimum":5,"exclusiveMinimum":10}`, "minimum", "exclusiveMinimum", 10, true},
		// Sisi minimum: eksplisit lebih besar → ia yang berlaku, inklusif.
		{"min 15 + exclMin 10 → >=15", `{"type":"number","minimum":15,"exclusiveMinimum":10}`, "minimum", "exclusiveMinimum", 15, false},
		// Sisi maximum: eksklusif lebih kecil → ia yang berlaku, eksklusif.
		{"max 100 + exclMax 50 → <50", `{"type":"number","maximum":100,"exclusiveMaximum":50}`, "maximum", "exclusiveMaximum", 50, true},
		// Sisi maximum: eksplisit lebih kecil → ia yang berlaku, inklusif.
		{"max 40 + exclMax 50 → <=40", `{"type":"number","maximum":40,"exclusiveMaximum":50}`, "maximum", "exclusiveMaximum", 40, false},
		// Tanpa batas eksplisit: terjemahan lama tetap benar.
		{"exclMin 10 saja → >10", `{"type":"number","exclusiveMinimum":10}`, "minimum", "exclusiveMinimum", 10, true},
		{"exclMax 50 saja → <50", `{"type":"number","exclusiveMaximum":50}`, "maximum", "exclusiveMaximum", 50, true},
	}
	for _, tc := range kasus {
		for i := 0; i < 200; i++ {
			out := nodeSkema(t, tc.skema)
			if got := out[tc.batas]; got != tc.mauBatas {
				t.Fatalf("%s (lari %d): %s = %v, mau %v (keluaran %v)", tc.nama, i, tc.batas, got, tc.mauBatas, out)
			}
			got, ada := out[tc.eksklusif]
			if tc.mauTanda {
				if !ada || got != true {
					t.Fatalf("%s (lari %d): %s = %v (ada=%v), mau true (keluaran %v)", tc.nama, i, tc.eksklusif, got, ada, out)
				}
			} else if ada {
				t.Fatalf("%s (lari %d): %s = %v, mau DIBUANG (keluaran %v)", tc.nama, i, tc.eksklusif, got, out)
			}
		}
	}
}

// TestB584BentukBooleanWarisanDiteruskan mengunci bahwa bentuk
// boolean warisan exclusive* (subset OpenAPI) TIDAK ikut
// penggabungan B-584: diteruskan apa adanya bersama batasnya,
// termasuk exclusiveMaximum:false yang harus tetap ada (bukan
// dibuang — penggabungan hanya berlaku untuk bentuk ANGKA).
func TestB584BentukBooleanWarisanDiteruskan(t *testing.T) {
	for _, tc := range []struct {
		skema string
		mau   map[string]any
	}{
		{
			`{"type":"number","minimum":5,"exclusiveMinimum":true}`,
			map[string]any{"type": "number", "minimum": float64(5), "exclusiveMinimum": true},
		},
		{
			`{"type":"number","maximum":50,"exclusiveMaximum":false}`,
			map[string]any{"type": "number", "maximum": float64(50), "exclusiveMaximum": false},
		},
	} {
		out := nodeSkema(t, tc.skema)
		if !reflect.DeepEqual(out, tc.mau) {
			t.Errorf("bentuk boolean %s:\ndapat %v\nmau   %v", tc.skema, out, tc.mau)
		}
	}
}

// TestB584BatasSamaMembunuhMutan mengunci dua baris batas-sama
// B-584 — kasus yang perilaku lamanya BENAR ("benar hanya bila
// nilainya sama", PB14-7) dan wajib tetap benar sesudah
// perbaikan: konjungsi minimum:10 (>=10) dengan
// exclusiveMinimum:10 (>10) adalah >10, BUKAN >=10.
//
// Baris ini membunuh dua mutan sekaligus: (a) penjaga
// "eksplisit selalu menang" (revert ke perilaku lama —
// penanda hilang), dan (b) perbandingan ketat `>` pada
// penggabungan (penanda dibuang saat nilainya sama —
// membaca harfiah `exclusiveMinimum <= minimum` pada
// keputusan B-584; lihat komentar B-584 di translate.go).
// Penegasan memakai DeepEqual atas SELURUH node supaya
// perubahan apa pun pada bentuk keluaran menggagalkannya.
func TestB584BatasSamaMembunuhMutan(t *testing.T) {
	for _, tc := range []struct {
		skema string
		mau   map[string]any
	}{
		{
			`{"type":"number","minimum":10,"exclusiveMinimum":10}`,
			map[string]any{"type": "number", "minimum": float64(10), "exclusiveMinimum": true},
		},
		{
			`{"type":"number","maximum":50,"exclusiveMaximum":50}`,
			map[string]any{"type": "number", "maximum": float64(50), "exclusiveMaximum": true},
		},
	} {
		out := nodeSkema(t, tc.skema)
		if !reflect.DeepEqual(out, tc.mau) {
			t.Errorf("batas sama %s:\ndapat %v\nmau   %v", tc.skema, out, tc.mau)
		}
	}
}

// TestB585RefIndeksLarik mengunci keempat baris tabel B-585 +
// kontrol peta, lewat sanitizeSchema (jalur $ref sungguhan):
// #/allOf/0, #/prefixItems/1, dan #/properties/wadah/allOf/1
// menyelesaikan elemen lariknya; penolakan RFC 6901 ("-",
// indeks di luar rentang, angka ber-nol-di-depan, segmen
// bukan-angka) menjadi skema terbuka {} seperti referensi
// menggantung; kontrol peta #/$defs/… tetap menyelesaikan
// targetnya.
func TestB585RefIndeksLarik(t *testing.T) {
	skema := `{
		"$defs": {"pilihan": {"type": "string", "enum": ["a", "b"]}},
		"type": "object",
		"properties": {
			"nol": {"$ref": "#/allOf/0"},
			"satu": {"$ref": "#/prefixItems/1"},
			"bersarang": {"$ref": "#/properties/wadah/allOf/1"},
			"dalam": {"$ref": "#/allOf/0/properties/dalam"},
			"nolDiDepan": {"$ref": "#/allOf/01"},
			"setrip": {"$ref": "#/allOf/-"},
			"diLuar": {"$ref": "#/allOf/2"},
			"bukanAngka": {"$ref": "#/allOf/0x"},
			"kontrolPeta": {"$ref": "#/$defs/pilihan"},
			"wadah": {"allOf": [{"type": "integer"}, {"type": "number"}]}
		},
		"allOf": [
			{"type": "object", "properties": {"dalam": {"type": "string"}}},
			{"type": "boolean"}
		],
		"prefixItems": [{"type": "string"}, {"type": "number"}]
	}`
	props := nodeSkema(t, skema)["properties"].(map[string]any)
	ambil := func(nama string) map[string]any {
		t.Helper()
		peta, ok := props[nama].(map[string]any)
		if !ok {
			t.Fatalf("properties.%s hilang/bukan peta: %v", nama, props)
		}
		return peta
	}
	mau := map[string]map[string]any{
		"nol":         {"type": "object", "properties": map[string]any{"dalam": map[string]any{"type": "string"}}},
		"satu":        {"type": "number"},
		"bersarang":   {"type": "number"},
		"dalam":       {"type": "string"},
		"kontrolPeta": {"type": "string", "enum": []any{"a", "b"}},
	}
	for nama, mauPeta := range mau {
		if got := ambil(nama); !reflect.DeepEqual(got, mauPeta) {
			t.Errorf("properties.%s:\ndapat %v\nmau   %v", nama, got, mauPeta)
		}
	}
	for _, nama := range []string{"nolDiDepan", "setrip", "diLuar", "bukanAngka"} {
		if got := ambil(nama); len(got) != 0 {
			t.Errorf("properties.%s = %v, mau {} (segmen indeks tak sah harus gagal seperti ref menggantung)", nama, got)
		}
	}
}

// TestB585CariPointerLokalLangsung mengunci cariPointerLokal
// pada tingkat segmen: indeks sah (termasuk "0"), dan
// penolakan "-"/di-luar-rentang/nol-di-depan/"+" —
// masing-masing satu baris, tanpa melalui $ref.
func TestB585CariPointerLokalLangsung(t *testing.T) {
	root := map[string]any{
		"allOf": []any{
			map[string]any{"type": "integer"},
			map[string]any{"type": "boolean"},
		},
		"nama": "x",
	}
	kasus := []struct {
		ref   string
		mauOk bool
		mau   any
	}{
		{"#/allOf/0", true, map[string]any{"type": "integer"}},
		{"#/allOf/1", true, map[string]any{"type": "boolean"}},
		{"#/allOf/01", false, nil},
		{"#/allOf/00", false, nil},
		{"#/allOf/-", false, nil},
		{"#/allOf/2", false, nil},
		{"#/allOf/+1", false, nil},
		{"#/allOf/1x", false, nil},
		{"#/nama", true, "x"},
	}
	for _, tc := range kasus {
		got, ok := cariPointerLokal(root, tc.ref)
		if ok != tc.mauOk {
			t.Errorf("cariPointerLokal(%q): ok = %v, mau %v (nilai %v)", tc.ref, ok, tc.mauOk, got)
			continue
		}
		if tc.mauOk && !reflect.DeepEqual(got, tc.mau) {
			t.Errorf("cariPointerLokal(%q) = %v, mau %v", tc.ref, got, tc.mau)
		}
	}
}

// TestB586SegmenPointerBerkodePersen mengunci ketiga baris
// tabel B-586 lewat sanitizeSchema (jalur $ref sungguhan):
// "#/properties/a%20b" menyelesaikan kunci "a b" (bentuk baku
// RFC 6901 §6); kontrol spasi-mentah dan kontrol biasa tetap
// menyelesaikan targetnya. Tambahan yang diamanatkan
// keputusan: "%2F" di dalam segmen TIDAK membelah segmen
// (dekode sesudah pemisahan atas "/"), dan "%25" menjadi
// persen literal.
func TestB586SegmenPointerBerkodePersen(t *testing.T) {
	skema := `{
		"type": "object",
		"properties": {
			"spasi": {"$ref": "#/properties/a%20b"},
			"garisMiring": {"$ref": "#/properties/a%2Fb"},
			"persen": {"$ref": "#/properties/a%25b"},
			"mentah": {"$ref": "#/properties/a b"},
			"biasa": {"$ref": "#/properties/ab"},
			"escapeBuruk": {"$ref": "#/properties/a%2"},
			"tilde": {"$ref": "#/properties/a~1b"},
			"a b": {"type": "string"},
			"a/b": {"type": "integer"},
			"a%b": {"type": "boolean"},
			"ab": {"type": "number"}
		}
	}`
	props := nodeSkema(t, skema)["properties"].(map[string]any)
	ambil := func(nama string) map[string]any {
		t.Helper()
		peta, ok := props[nama].(map[string]any)
		if !ok {
			t.Fatalf("properties.%s hilang/bukan peta: %v", nama, props)
		}
		return peta
	}
	mau := map[string]map[string]any{
		"spasi":       {"type": "string"},
		"garisMiring": {"type": "integer"},
		"persen":      {"type": "boolean"},
		"mentah":      {"type": "string"},
		"biasa":       {"type": "number"},
		"tilde":       {"type": "integer"},
	}
	for nama, mauPeta := range mau {
		if got := ambil(nama); !reflect.DeepEqual(got, mauPeta) {
			t.Errorf("properties.%s:\ndapat %v\nmau   %v", nama, got, mauPeta)
		}
	}
	if got := ambil("escapeBuruk"); len(got) != 0 {
		t.Errorf("properties.escapeBuruk = %v, mau {} (escape persen tak sah gagal seperti ref menggantung)", got)
	}
}

// TestB586CariPointerLokalLangsung mengunci dekode persen pada
// tingkat segmen: "%20" → spasi, "%2F" → "/" tanpa membelah
// segmen, "%25" → "%", dan pelepasan "~0"/"~1" tetap berlaku
// sesudah dekode persen.
func TestB586CariPointerLokalLangsung(t *testing.T) {
	root := map[string]any{
		"properties": map[string]any{
			"a b": map[string]any{"type": "string"},
			"a/b": map[string]any{"type": "integer"},
			"a%b": map[string]any{"type": "boolean"},
			"ab":  map[string]any{"type": "number"},
		},
	}
	kasus := []struct {
		ref   string
		mauOk bool
		mau   any
	}{
		{"#/properties/a%20b", true, map[string]any{"type": "string"}},
		{"#/properties/a%2Fb", true, map[string]any{"type": "integer"}},
		{"#/properties/a%25b", true, map[string]any{"type": "boolean"}},
		{"#/properties/a b", true, map[string]any{"type": "string"}},
		{"#/properties/ab", true, map[string]any{"type": "number"}},
		{"#/properties/a~1b", true, map[string]any{"type": "integer"}},
		{"#/properties/a%2", false, nil},
		{"#/properties/a%ZZb", false, nil},
	}
	for _, tc := range kasus {
		got, ok := cariPointerLokal(root, tc.ref)
		if ok != tc.mauOk {
			t.Errorf("cariPointerLokal(%q): ok = %v, mau %v (nilai %v)", tc.ref, ok, tc.mauOk, got)
			continue
		}
		if tc.mauOk && !reflect.DeepEqual(got, tc.mau) {
			t.Errorf("cariPointerLokal(%q) = %v, mau %v", tc.ref, got, tc.mau)
		}
	}
}

// TestB585B586RefIndeksBerkodePersen mengunci gabungan kedua
// perbaikan pada satu pointer: indeks larik yang segmen
// sebelumnya berkode-persen, dan kunci peta berkode-persen
// yang dicapai lewat indeks larik.
func TestB585B586RefIndeksBerkodePersen(t *testing.T) {
	skema := `{
		"type": "object",
		"properties": {
			"campur": {"$ref": "#/allOf/0/properties/a%20b"},
			"indeksPersen": {"$ref": "#/prefixItems/1"}
		},
		"allOf": [{"type": "object", "properties": {"a b": {"type": "string"}}}],
		"prefixItems": [{"type": "string"}, {"type": "number"}]
	}`
	props := nodeSkema(t, skema)["properties"].(map[string]any)
	if got := props["campur"].(map[string]any); !reflect.DeepEqual(got, map[string]any{"type": "string"}) {
		t.Errorf("properties.campur = %v, mau {type:string}", got)
	}
	if got := props["indeksPersen"].(map[string]any); !reflect.DeepEqual(got, map[string]any{"type": "number"}) {
		t.Errorf("properties.indeksPersen = %v, mau {type:number}", got)
	}
}
