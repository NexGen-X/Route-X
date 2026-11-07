package cliconfig

// fase27_prd_test.go — pengunci Fase 27 PR-D (cliconfig
// B-588/B-589/B-590/B-591): penulis YAML memecah hanya pada
// "\n", sehingga pemisah garis YAML lain (CR, NEL U+0085, LS
// U+2028, PS U+2029) menyembunyikan satu baris utuh di dalam
// satu "baris-\n"; dan TAB diterima sebagai pemisah di
// posisi-posisi yang ditolak PyYAML.
//
// Semantik pemuat dibuktikan dengan PyYAML sungguhan (oracle)
// sebelum perbaikan ditulis:
//   - B-588: "GOOSE_MODEL: lama<SEP>lainnya: 2" terurai PyYAML
//     menjadi dua kunci; penulis lama mengganti/membuang baris
//     itu utuh dan "lainnya" hilang senyap (27 kasus x 4 pemisah).
//   - B-590: "model:" terselundup di isi skalar blok / komentar /
//     sesudah kutip tertutup adalah kunci tingkat-atas sungguhan
//     bagi PyYAML; penulis lama menempel blok kedua (9 kasus).
//   - B-591: kunci terkelola terselundup membuat penulis
//     menempel duplikat dan verifikasi ketat menolak config yang
//     sah (15 kasus).
//   - B-589: TAB di posisi pemisah (sesudah ":", di dalam nilai
//     polos, sebelum komentar, sebelum ":", di flow-seq/flow-map,
//     di akhir nilai, di kelanjutan polos, indentasi, sesudah "-",
//     sesudah "---") ditolak PyYAML dengan ScannerError, tetapi
//     yaml.v3 menerimanya dan Configure sukses-bohong (22 kasus);
//     TAB di dalam skalar terkutip, di dalam komentar, dan di
//     tengah isi skalar blok DITERIMA PyYAML dan wajib lolos.
//     Kaki Goose: perilaku serde_yaml belum terbukti — pesan
//     galat Goose menandai itu secara eksplisit (lihat B-589).
//
// Semua test end-to-end memanggil applyToolSpecificConfig — jalur
// Configure sungguhan — lewat pembantu terapkanPRD, kecuali
// pengunci lapis (pemecah berpemisah, diagnosis pesan, sapuan
// situs) yang mengunci fungsinya secara langsung.

import (
	"errors"
	"os"
	"strings"
	"testing"
)

// kasusPemisahB588 adalah sapuan mekanis B-588: tiap bentuk
// masukan dijalankan atas keempat pemisah tersembunyi dan di
// ketiga tool yang bersangkutan. Bentuknya meniru bukti
// Putaran Bersih #14 (baris kunci Goose/Aider, duplikat
// terkelola, flow-map Aider, sub-kunci blok Hermes).
var kasusPemisahB588 = []struct {
	nama    string
	tool    string
	masukan func(sep string) string
	periksa func(t *testing.T, doc map[string]any, konten string)
}{
	{
		"goose-terkelola+pengguna", "goose",
		func(sep string) string { return "GOOSE_MODEL: lama" + sep + "lainnya: 2\n" },
		func(t *testing.T, doc map[string]any, konten string) {
			if doc["GOOSE_MODEL"] != "model-baru" {
				t.Errorf("GOOSE_MODEL = %v, mau model-baru:\n%s", doc["GOOSE_MODEL"], konten)
			}
			if doc["lainnya"] != 2 {
				t.Errorf("kunci pengguna 'lainnya' hilang/berubah: %v\n%s", doc["lainnya"], konten)
			}
		},
	},
	{
		"aider-terkelola+pengguna", "aider",
		func(sep string) string { return "model: lama" + sep + "lainnya: 2\n" },
		func(t *testing.T, doc map[string]any, konten string) {
			if doc["model"] != "openai/model-baru" {
				t.Errorf("model = %v, mau openai/model-baru:\n%s", doc["model"], konten)
			}
			if doc["lainnya"] != 2 {
				t.Errorf("kunci pengguna 'lainnya' hilang/berubah: %v\n%s", doc["lainnya"], konten)
			}
		},
	},
	{
		"goose-duplikat+pengguna", "goose",
		func(sep string) string { return "GOOSE_MODEL: lama\nGOOSE_MODEL: lama2" + sep + "lainnya: 2\n" },
		func(t *testing.T, doc map[string]any, konten string) {
			if doc["GOOSE_MODEL"] != "model-baru" {
				t.Errorf("GOOSE_MODEL = %v, mau model-baru:\n%s", doc["GOOSE_MODEL"], konten)
			}
			if doc["lainnya"] != 2 {
				t.Errorf("kunci pengguna 'lainnya' ikut terbuang bersama duplikat terkelola: %v\n%s", doc["lainnya"], konten)
			}
			if n := strings.Count(konten, "GOOSE_MODEL:"); n != 1 {
				t.Errorf("GOOSE_MODEL tertulis %d kali, mau 1:\n%s", n, konten)
			}
		},
	},
	{
		"aider-duplikat+pengguna", "aider",
		func(sep string) string { return "model: lama\nmodel: lama2" + sep + "lainnya: 2\n" },
		func(t *testing.T, doc map[string]any, konten string) {
			if doc["model"] != "openai/model-baru" {
				t.Errorf("model = %v, mau openai/model-baru:\n%s", doc["model"], konten)
			}
			if doc["lainnya"] != 2 {
				t.Errorf("kunci pengguna 'lainnya' ikut terbuang bersama duplikat terkelola: %v\n%s", doc["lainnya"], konten)
			}
		},
	},
	{
		"aider-flowmap-terkelola+pengguna", "aider",
		func(sep string) string { return "model: {a: 1}" + sep + "lainnya: 2\n" },
		func(t *testing.T, doc map[string]any, konten string) {
			if doc["model"] != "openai/model-baru" {
				t.Errorf("model = %v, mau openai/model-baru:\n%s", doc["model"], konten)
			}
			if doc["lainnya"] != 2 {
				t.Errorf("kunci pengguna 'lainnya' hilang bersama baris flow-map terkelola: %v\n%s", doc["lainnya"], konten)
			}
		},
	},
	{
		"aider-flowmap+terkelola-tersembunyi", "aider",
		func(sep string) string { return "lainnya: {a: 1}" + sep + "model: lama\n" },
		func(t *testing.T, doc map[string]any, konten string) {
			if doc["model"] != "openai/model-baru" {
				t.Errorf("model terselundup tidak ditulis ulang: %v\n%s", doc["model"], konten)
			}
			aliran, _ := doc["lainnya"].(map[string]any)
			if aliran["a"] != 1 {
				t.Errorf("flow-map pengguna rusak: %v\n%s", doc["lainnya"], konten)
			}
		},
	},
	{
		"hermes-subkunci-tersembunyi", "hermes",
		func(sep string) string {
			return "model:\n  default: lama" + sep + "  catatan: simpan\n  provider: lain\n"
		},
		func(t *testing.T, doc map[string]any, konten string) {
			blok, _ := doc["model"].(map[string]any)
			if blok["default"] != "model-baru" {
				t.Errorf("model.default = %v, mau model-baru:\n%s", blok["default"], konten)
			}
			if blok["catatan"] != "simpan" {
				t.Errorf("sub-kunci pengguna 'catatan' ikut terbuang bersama sub-kunci terkelola: %v\n%s", blok["catatan"], konten)
			}
			if blok["provider"] != "custom" {
				t.Errorf("model.provider = %v, mau custom:\n%s", blok["provider"], konten)
			}
		},
	},
	{
		"hermes-subkunci-duplikat-terkelola", "hermes",
		func(sep string) string {
			return "model:\n  default: lama\n  default: lama2" + sep + "  catatan: simpan\n"
		},
		func(t *testing.T, doc map[string]any, konten string) {
			blok, _ := doc["model"].(map[string]any)
			if blok["default"] != "model-baru" {
				t.Errorf("model.default = %v, mau model-baru:\n%s", blok["default"], konten)
			}
			if blok["catatan"] != "simpan" {
				t.Errorf("sub-kunci pengguna 'catatan' ikut terbuang bersama duplikat terkelola: %v\n%s", blok["catatan"], konten)
			}
		},
	},
}

// TestB588KunciPenggunaTersembunyiSelamat mengunci B-588:
// kunci pengguna yang tersembunyi di balik pemisah garis di
// baris yang ditulis-ulang/dibuang penulis harus selamat —
// penulis kini melihat baris sejati (pecahBarisYAML) dan
// baris tersembunyi itu dipertahankan sebagai baris sendiri.
func TestB588KunciPenggunaTersembunyiSelamat(t *testing.T) {
	for _, pem := range pemisahTersembunyiB571 {
		for _, tc := range kasusPemisahB588 {
			t.Run(pem.nama+"/"+tc.nama, func(t *testing.T) {
				home := t.TempDir()
				jalur := jalurKonfigB559(home, tc.tool)
				tulisBerkasPRD(t, jalur, []byte(tc.masukan(pem.sep)))

				if err := terapkanPRD(t, home, tc.tool, ""); err != nil {
					t.Fatalf("applyToolSpecificConfig %s menolak masukan sah (%s/%s): %v", tc.tool, pem.nama, tc.nama, err)
				}
				konten := bacaBerkasUji(t, jalur)
				doc := uraiYAMLUji(t, konten)
				tc.periksa(t, doc, konten)
			})
		}
	}
}

// kasusPemisahB590 adalah sapuan mekanis B-590: "model:"
// Hermes yang terselundup pemisah garis wajib dikenali
// sebagai blok lama dan digabung — bukan diduplikasi.
var kasusPemisahB590 = []struct {
	nama    string
	masukan func(sep string) string
}{
	{"blok-skalar", func(sep string) string {
		return "catatan: |\n  isi" + sep + "model:\n  default: lama\n"
	}},
	{"komentar", func(sep string) string {
		return "# komentar" + sep + "model:\n  default: lama\n"
	}},
	{"kutip-tertutup", func(sep string) string {
		return "kunci: \"nilai\"" + sep + "model:\n  default: lama\n"
	}},
	{"terlihat+terselundup", func(sep string) string {
		return "model:\n  default: baru\nlainnya: 1" + sep + "model:\n  default: lama\n"
	}},
	{"terlihat+flowmap-terselundup", func(sep string) string {
		return "model:\n  default: baru\nlainnya: 1" + sep + "model: {default: lama}\n"
	}},
}

// hitungHeaderModelHermes menghitung baris header `model:`
// tingkat-atas (bentuk peta maupun flow-map) dalam keluaran.
func hitungHeaderModelHermes(konten string) int {
	n := 0
	for _, l := range pecahBarisYAML(konten) {
		if _, ok := pecahHeaderModelHermes(l); ok {
			n++
		}
	}
	return n
}

// TestB590HermesDuaBlokModel mengunci B-590: hasil Configure
// Hermes atas "model:" terselundup memuat tepat SATU blok
// model dengan nilai Route-X yang efektif — bukan dua blok
// yang bertentangan (lama + baru).
func TestB590HermesDuaBlokModel(t *testing.T) {
	for _, pem := range pemisahTersembunyiB571 {
		for _, tc := range kasusPemisahB590 {
			t.Run(pem.nama+"/"+tc.nama, func(t *testing.T) {
				home := t.TempDir()
				jalur := jalurKonfigB559(home, "hermes")
				tulisBerkasPRD(t, jalur, []byte(tc.masukan(pem.sep)))

				if err := terapkanPRD(t, home, "hermes", ""); err != nil {
					t.Fatalf("applyToolSpecificConfig hermes menolak masukan sah (%s/%s): %v", pem.nama, tc.nama, err)
				}
				konten := bacaBerkasUji(t, jalur)
				if n := hitungHeaderModelHermes(konten); n != 1 {
					t.Errorf("keluaran memuat %d header model:, mau 1 (degradasi B-590):\n%s", n, konten)
				}
				doc := uraiYAMLUji(t, konten)
				blok, _ := doc["model"].(map[string]any)
				if blok["default"] != "model-baru" {
					t.Errorf("model.default = %v, mau model-baru:\n%s", blok["default"], konten)
				}
			})
		}
	}
}

// nilaiIsiSkalarBlokB591 adalah nilai skalar blok "isi" yang
// dibaca pemuat sesudah pemisah yang mengakhiri baris isinya:
// CR dan NEL dinormalkan menjadi "\n" oleh PyYAML maupun
// yaml.v3, sedangkan LS dan PS dipertahankan apa adanya di
// dalam nilai (terbukti oracle PyYAML: 'isi\u2028' /
// 'isi\u2029'). Penulis mempertahankan pemisah asalnya, jadi
// nilai ini harus sama sebelum dan sesudah Configure.
func nilaiIsiSkalarBlokB591(sep string) string {
	if sep == "\u2028" || sep == "\u2029" {
		return "isi" + sep
	}
	return "isi\n"
}

// kasusPemisahB591 adalah sapuan mekanis B-591: kunci
// terkelola terselundup yang dulu membuat Configure gagal
// atas config yang sah (penulis menempel duplikat, lalu
// verifikasi ketat menolaknya) kini ditulis ulang di tempat
// dan Configure berhasil.
var kasusPemisahB591 = []struct {
	nama    string
	tool    string
	masukan func(sep string) string
	periksa func(t *testing.T, doc map[string]any, konten string, sep string)
}{
	{
		"aider-terkelola-skalar", "aider",
		func(sep string) string { return "catatan: |\n  isi" + sep + "model: lama\n" },
		func(t *testing.T, doc map[string]any, konten string, sep string) {
			if doc["model"] != "openai/model-baru" {
				t.Errorf("model = %v, mau openai/model-baru:\n%s", doc["model"], konten)
			}
			if doc["catatan"] != nilaiIsiSkalarBlokB591(sep) {
				t.Errorf("catatan rusak: %q, mau %q\n%s", doc["catatan"], nilaiIsiSkalarBlokB591(sep), konten)
			}
		},
	},
	{
		"aider-terkelola-komentar", "aider",
		func(sep string) string { return "# komentar" + sep + "model: lama\n" },
		func(t *testing.T, doc map[string]any, konten string, _ string) {
			if doc["model"] != "openai/model-baru" {
				t.Errorf("model = %v, mau openai/model-baru:\n%s", doc["model"], konten)
			}
		},
	},
	{
		"goose-terkelola-skalar", "goose",
		func(sep string) string { return "catatan: |\n  isi" + sep + "GOOSE_MODEL: lama\n" },
		func(t *testing.T, doc map[string]any, konten string, sep string) {
			if doc["GOOSE_MODEL"] != "model-baru" {
				t.Errorf("GOOSE_MODEL = %v, mau model-baru:\n%s", doc["GOOSE_MODEL"], konten)
			}
			if doc["catatan"] != nilaiIsiSkalarBlokB591(sep) {
				t.Errorf("catatan rusak: %q, mau %q\n%s", doc["catatan"], nilaiIsiSkalarBlokB591(sep), konten)
			}
		},
	},
	{
		"goose-terkelola-komentar", "goose",
		func(sep string) string { return "# komentar" + sep + "GOOSE_MODEL: lama\n" },
		func(t *testing.T, doc map[string]any, konten string, _ string) {
			if doc["GOOSE_MODEL"] != "model-baru" {
				t.Errorf("GOOSE_MODEL = %v, mau model-baru:\n%s", doc["GOOSE_MODEL"], konten)
			}
		},
	},
	{
		"hermes-terlihat+terselundup", "hermes",
		func(sep string) string {
			return "model:\n  default: baru\nlainnya: 1" + sep + "model:\n  default: lama\n"
		},
		func(t *testing.T, doc map[string]any, konten string, _ string) {
			blok, _ := doc["model"].(map[string]any)
			if blok["default"] != "model-baru" {
				t.Errorf("model.default = %v, mau model-baru:\n%s", blok["default"], konten)
			}
			if n := hitungHeaderModelHermes(konten); n != 1 {
				t.Errorf("keluaran memuat %d header model:, mau 1:\n%s", n, konten)
			}
		},
	},
	{
		"hermes-terlihat+flowmap-terselundup", "hermes",
		func(sep string) string {
			return "model:\n  default: baru\nlainnya: 1" + sep + "model: {default: lama}\n"
		},
		func(t *testing.T, doc map[string]any, konten string, _ string) {
			blok, _ := doc["model"].(map[string]any)
			if blok["default"] != "model-baru" {
				t.Errorf("model.default = %v, mau model-baru:\n%s", blok["default"], konten)
			}
			if n := hitungHeaderModelHermes(konten); n != 1 {
				t.Errorf("keluaran memuat %d header model:, mau 1:\n%s", n, konten)
			}
		},
	},
}

// TestB591PenolakanPalsuDitutup mengunci B-591: Configure atas
// config sah dengan kunci terkelola terselundup tidak lagi
// gagal — penulis melihat kunci itu dan menulisnya ulang di
// tempat, jadi tidak ada duplikat yang ditolak verifikasi.
func TestB591PenolakanPalsuDitutup(t *testing.T) {
	for _, pem := range pemisahTersembunyiB571 {
		for _, tc := range kasusPemisahB591 {
			t.Run(pem.nama+"/"+tc.nama, func(t *testing.T) {
				home := t.TempDir()
				jalur := jalurKonfigB559(home, tc.tool)
				tulisBerkasPRD(t, jalur, []byte(tc.masukan(pem.sep)))

				if err := terapkanPRD(t, home, tc.tool, ""); err != nil {
					t.Fatalf("applyToolSpecificConfig %s menolak config sah (%s/%s): %v", tc.tool, pem.nama, tc.nama, err)
				}
				konten := bacaBerkasUji(t, jalur)
				doc := uraiYAMLUji(t, konten)
				tc.periksa(t, doc, konten, pem.sep)
			})
		}
	}
}

// TestB588PecahBarisBerpemisaahYAML mengunci lapis pemecah
// secara langsung: pecahBarisBerpemisaahYAML membelah pada
// tiap pemisah garis YAML (CRLF dihitung satu) sambil
// menyimpan pemisahnya, dan gabungan ulang dengan
// sambungBarisBerpemisaahYAML adalah identitas byte atas teks
// asal; gabungBarisBerpemisaahYAML memastikan newline penutup
// seperti gabungBarisYAML yang digantikannya.
func TestB588PecahBarisBerpemisaahYAML(t *testing.T) {
	pecah := []struct {
		nama string
		teks string
		mau  []barisYAML
	}{
		{"LF", "a\nb\n", []barisYAML{{"a", "\n"}, {"b", "\n"}, {"", ""}}},
		{"CRLF-satu-pemisah", "a\r\nb\r\n", []barisYAML{{"a", "\r\n"}, {"b", "\r\n"}, {"", ""}}},
		{"CR-saja", "a\rb\r", []barisYAML{{"a", "\r"}, {"b", "\r"}, {"", ""}}},
		{"NEL", "a\u0085b", []barisYAML{{"a", "\u0085"}, {"b", ""}}},
		{"LS", "a\u2028b", []barisYAML{{"a", "\u2028"}, {"b", ""}}},
		{"PS", "a\u2029b", []barisYAML{{"a", "\u2029"}, {"b", ""}}},
		{"campur", "a\r\nb\rc\u0085d\u2028e", []barisYAML{{"a", "\r\n"}, {"b", "\r"}, {"c", "\u0085"}, {"d", "\u2028"}, {"e", ""}}},
		{"tanpa-pemisah", "abc", []barisYAML{{"abc", ""}}},
		{"kosong", "", []barisYAML{{"", ""}}},
		{"CR-akhir-EOF", "# biasa\r", []barisYAML{{"# biasa", "\r"}, {"", ""}}},
	}
	for _, tc := range pecah {
		t.Run(tc.nama, func(t *testing.T) {
			got := pecahBarisBerpemisaahYAML(tc.teks)
			if len(got) != len(tc.mau) {
				t.Fatalf("pecahBarisBerpemisaahYAML(%q) = %q, mau %q", tc.teks, got, tc.mau)
			}
			for i := range tc.mau {
				if got[i] != tc.mau[i] {
					t.Errorf("pecahBarisBerpemisaahYAML(%q)[%d] = %q, mau %q", tc.teks, i, got[i], tc.mau[i])
				}
			}
			if ulang := sambungBarisBerpemisaahYAML(got); ulang != tc.teks {
				t.Errorf("sambung(pecah(%q)) = %q, mau identitas byte", tc.teks, ulang)
			}
			if teks := teksBarisYAML(got); len(teks) != len(pecahBarisYAML(tc.teks)) {
				t.Errorf("teksBarisYAML/pecahBarisYAML tidak sejalan untuk %q", tc.teks)
			} else {
				for i := range teks {
					if teks[i] != pecahBarisYAML(tc.teks)[i] {
						t.Errorf("teksBarisYAML(%q)[%d] = %q, mau %q", tc.teks, i, teks[i], pecahBarisYAML(tc.teks)[i])
					}
				}
			}
		})
	}
	t.Run("gabung-memastikan-newline", func(t *testing.T) {
		if got := gabungBarisBerpemisaahYAML(pecahBarisBerpemisaahYAML("a\nb")); got != "a\nb\n" {
			t.Errorf("gabung(\"a\\nb\") = %q, mau \"a\\nb\\n\"", got)
		}
		if got := gabungBarisBerpemisaahYAML(pecahBarisBerpemisaahYAML("a\nb\n")); got != "a\nb\n" {
			t.Errorf("gabung(\"a\\nb\\n\") = %q, mau \"a\\nb\\n\"", got)
		}
		if got := gabungBarisBerpemisaahYAML(pecahBarisBerpemisaahYAML("a\rb")); got != "a\rb\n" {
			t.Errorf("gabung(\"a\\rb\") = %q, mau \"a\\rb\\n\"", got)
		}
	})
	t.Run("nama-pemisah", func(t *testing.T) {
		for pem, mau := range map[string]string{
			"\r": "CR", "\u0085": "NEL (U+0085)",
			"\u2028": "LINE SEPARATOR (U+2028)", "\u2029": "PARAGRAPH SEPARATOR (U+2029)",
		} {
			if got := namaPemisahGarisYAML(pem); got != mau {
				t.Errorf("namaPemisahGarisYAML(%q) = %q, mau %q", pem, got, mau)
			}
		}
	})
}

// TestB588DoubleConfigureByteStabil mengunci byte-stabilitas
// ganda Configure atas masukan berpemisah tersembunyi: lari
// kedua tidak boleh mengubah satu byte pun dari lari pertama,
// dan kunci pengguna yang tadinya tersembunyi tetap ada
// sesudah kedua lari.
func TestB588DoubleConfigureByteStabil(t *testing.T) {
	for _, pem := range pemisahTersembunyiB571 {
		for _, tool := range []string{"goose", "aider", "hermes"} {
			t.Run(pem.nama+"/"+tool, func(t *testing.T) {
				home := t.TempDir()
				jalur := jalurKonfigB559(home, tool)
				var masukan string
				switch tool {
				case "goose":
					masukan = "GOOSE_MODEL: lama" + pem.sep + "lainnya: 2\n"
				case "aider":
					masukan = "model: lama" + pem.sep + "lainnya: 2\n"
				default:
					masukan = "# komentar" + pem.sep + "model:\n  default: lama\n"
				}
				tulisBerkasPRD(t, jalur, []byte(masukan))

				if err := terapkanPRD(t, home, tool, ""); err != nil {
					t.Fatalf("lari pertama %s (%s): %v", tool, pem.nama, err)
				}
				pertama := bacaBerkasUji(t, jalur)
				if err := terapkanPRD(t, home, tool, ""); err != nil {
					t.Fatalf("lari kedua %s (%s): %v", tool, pem.nama, err)
				}
				kedua := bacaBerkasUji(t, jalur)
				if pertama != kedua {
					t.Errorf("lari kedua mengubah berkas (%s/%s):\n--- lari 1 ---\n%q\n--- lari 2 ---\n%q", tool, pem.nama, pertama, kedua)
				}
				doc := uraiYAMLUji(t, kedua)
				switch tool {
				case "goose":
					if doc["GOOSE_MODEL"] != "model-baru" || doc["lainnya"] != 2 {
						t.Errorf("hasil ganda goose rusak: %v\n%s", doc, kedua)
					}
				case "aider":
					if doc["model"] != "openai/model-baru" || doc["lainnya"] != 2 {
						t.Errorf("hasil ganda aider rusak: %v\n%s", doc, kedua)
					}
				default:
					blok, _ := doc["model"].(map[string]any)
					if blok["default"] != "model-baru" {
						t.Errorf("hasil ganda hermes rusak: %v\n%s", doc, kedua)
					}
					if n := hitungHeaderModelHermes(kedua); n != 1 {
						t.Errorf("hasil ganda hermes memuat %d header model:, mau 1:\n%s", n, kedua)
					}
				}
			})
		}
	}
}

// TestB588SeranganGabunganB559B560B571 mengunci bahwa
// perbaikan B-588 tidak merusak tiga lapis tetangga: (a)
// masukan gabungan — pemisah tersembunyi SEBELUM penanda +
// kutip di isi skalar blok (B-560) + penanda kedua berbentuk
// tab (B-559) — berhasil dengan semua isi utuh; (b) pemisah
// tersembunyi SESUDAH penanda yang menyembunyikan isi (B-571)
// tetap ditolak jujur dan berkas utuh.
func TestB588SeranganGabunganB559B560B571(t *testing.T) {
	t.Run("gabungan-sukses", func(t *testing.T) {
		for _, pem := range pemisahTersembunyiB571 {
			t.Run(pem.nama, func(t *testing.T) {
				home := t.TempDir()
				jalur := jalurKonfigB559(home, "hermes")
				masukan := "catatan: |\n" +
					"  \"kutip tak tertutup di isi blok\n" +
					"lainnya: 1" + pem.sep + "model:\n" +
					"  default: lama\n" +
					"...\n" +
					"...\t# selesai\n"
				tulisBerkasPRD(t, jalur, []byte(masukan))

				if err := terapkanPRD(t, home, "hermes", ""); err != nil {
					t.Fatalf("applyToolSpecificConfig hermes atas gabungan (%s): %v", pem.nama, err)
				}
				konten := bacaBerkasUji(t, jalur)
				doc := uraiYAMLUji(t, konten)
				if doc["catatan"] != "\"kutip tak tertutup di isi blok\n" {
					t.Errorf("isi skalar blok rusak: %q\n%s", doc["catatan"], konten)
				}
				if doc["lainnya"] != 1 {
					t.Errorf("lainnya = %v, mau 1:\n%s", doc["lainnya"], konten)
				}
				blok, _ := doc["model"].(map[string]any)
				if blok["default"] != "model-baru" {
					t.Errorf("model.default = %v, mau model-baru:\n%s", blok["default"], konten)
				}
				if n := hitungHeaderModelHermes(konten); n != 1 {
					t.Errorf("keluaran memuat %d header model:, mau 1:\n%s", n, konten)
				}
				if strings.Contains(konten, "...\\t") {
					t.Errorf("penanda kedua berbentuk tab tidak dinormalisasi:\n%s", konten)
				}
				if !strings.Contains(konten, "... # selesai") {
					t.Errorf("penanda kedua kanonik tidak tertulis:\n%s", konten)
				}
			})
		}
	})

	t.Run("gabungan-b571-tetap-ditolak", func(t *testing.T) {
		for _, pem := range pemisahTersembunyiB571 {
			t.Run(pem.nama, func(t *testing.T) {
				home := t.TempDir()
				jalur := jalurKonfigB559(home, "hermes")
				masukan := "lainnya: 1" + pem.sep + "model:\n" +
					"  default: lama\n" +
					"...\n" +
					"# biasa" + pem.sep + "kunci: 2\n"
				tulisBerkasPRD(t, jalur, []byte(masukan))

				err := terapkanPRD(t, home, "hermes", "")
				if err == nil {
					t.Fatalf("applyToolSpecificConfig hermes meloloskan isi tersembunyi sesudah penanda (%s) — regresi B-571", pem.nama)
				}
				if !strings.Contains(err.Error(), "sesudah penanda") {
					t.Errorf("penolakan bukan dari penjaga B-543: %v", err)
				}
				mentah, readErr := os.ReadFile(jalur)
				if readErr != nil {
					t.Fatalf("membaca berkas sesudah penolakan: %v", readErr)
				}
				if string(mentah) != masukan {
					t.Errorf("berkas berubah sesudah penolakan:\n%q", string(mentah))
				}
			})
		}
	})
}

// TestB591PesanGalatMenamaiPemisahTersembunyi mengunci redaksi
// R1 W4: bila verifikasi tetap menemukan kunci terkelola ganda
// dan penyebabnya adalah kunci yang tersembunyi di balik
// pemisah garis di dalam satu baris-"\n", pesan galat WAJIB
// menamai pemisah garis tersembunyi itu — bukan menyalahkan
// kutip di isi skalar blok (diagnosis B-560 yang keliru untuk
// kelas ini) atau struktur secara umum saja.
func TestB591PesanGalatMenamaiPemisahTersembunyi(t *testing.T) {
	pasanganAider := [][2]string{
		{"openai-api-base", urlUjiPRD},
		{"model", "openai/model-baru"},
	}
	for _, pem := range pemisahTersembunyiB571 {
		t.Run("kutip-di-isi-blok/"+pem.nama, func(t *testing.T) {
			// Keluaran hipotetis penulis lama: baris isi skalar
			// blok memuat kutip pembuka DAN kunci terkelola
			// terselundup; baris terkelola kanonik tertempel di
			// akhir — duplikat bagi yaml.v3.
			konten := "catatan: |\n" +
				"  \"pembuka" + pem.sep + "model: lama\n" +
				"model: openai/model-baru\n" +
				"openai-api-base: " + urlUjiPRD + "\n"
			err := verifikasiHasilYAMLTingkatAtas(konten, pasanganAider)
			if err == nil {
				t.Fatalf("verifikasi meloloskan duplikat terkelola (%s)", pem.nama)
			}
			if !strings.Contains(err.Error(), "pemisah garis tersembunyi") {
				t.Errorf("pesan galat tidak menamai pemisah garis tersembunyi (%s): %v", pem.nama, err)
			}
			if strings.Contains(err.Error(), "kutip tak tertutup") {
				t.Errorf("pesan galat masih menyalahkan kutip (%s): %v", pem.nama, err)
			}
		})
		t.Run("tanpa-kutip/"+pem.nama, func(t *testing.T) {
			konten := "catatan: |\n" +
				"  isi" + pem.sep + "model: lama\n" +
				"model: openai/model-baru\n" +
				"openai-api-base: " + urlUjiPRD + "\n"
			err := verifikasiHasilYAMLTingkatAtas(konten, pasanganAider)
			if err == nil {
				t.Fatalf("verifikasi meloloskan duplikat terkelola (%s)", pem.nama)
			}
			if !strings.Contains(err.Error(), "pemisah garis tersembunyi") {
				t.Errorf("pesan galat tidak menamai pemisah garis tersembunyi (%s): %v", pem.nama, err)
			}
		})
	}

	// Cabang umum: galat duplikasi terkelola tanpa jejak
	// pemisah tersembunyi tetap menyebut pemisah itu sebagai
	// kemungkinan penyebab (bukan diagnosis pasti).
	t.Run("cabang-umum-menyebut-kemungkinan", func(t *testing.T) {
		err := diagnosisKutipSkalarBlok(
			"model: openai/model-baru\nmodel: openai/model-baru\nopenai-api-base: "+urlUjiPRD+"\n",
			pasanganAider,
			errors.New(`mapping key "model" already defined at line 2`))
		if err == "" {
			t.Fatal("diagnosis kosong atas duplikat terkelola")
		}
		if !strings.Contains(err, "pemisah garis tersembunyi") {
			t.Errorf("pesan umum tidak menyebut pemisah garis tersembunyi sebagai kemungkinan: %s", err)
		}
	})
}

// kasusTabB589 adalah sapuan mekanis B-589: tiap posisi TAB
// yang ditolak PyYAML (oracle: ScannerError "found character
// '\t' that cannot start any token") dijalankan di ketiga tool.
// barisTab adalah nomor baris (1-based, dihitung pada baris
// YAML sejati) yang wajib disebut pesan galatnya.
var kasusTabB589 = []struct {
	nama    string
	masukan string
	baris   int
}{
	{"sesudah-titik-dua", "a:\tb\n", 1},
	{"dalam-nilai-polos", "a: b\tc\n", 1},
	{"sebelum-komentar", "a: 1\t# c\n", 1},
	{"sebelum-titik-dua", "a\t: 1\n", 1},
	{"flow-seq", "a: [1,\t2]\n", 1},
	{"flow-map", "a: {x:\t1}\n", 1},
	{"tab-akhir-nilai", "a: b\t\n", 1},
	{"nilai-multiline-polos", "a: baris1\n  baris2\tlanjut\n", 2},
	{"blok-isi-indent-tab", "a: |\n\tx\n", 2},
	{"blok-baris-hanya-tab", "a: |\n  x\n\t\n  y\n", 3},
	{"indentasi-pemetaan", "\ta: 1\n", 1},
	{"sesudah-dash", "- \ta\n", 1},
	{"sesudah---", "---\ta: 1\n", 1},
	{"komentar-indent-tab", "\t# komentar\na: 1\n", 1},
}

// masukanTabTerkelolaB589 adalah TAB pada baris kunci terkelola
// tiap tool — baris yang justru akan ditulis ulang penulis,
// tetapi masukannya tetap berkas yang tidak terbaca pemuat
// tujuan dan wajib ditolak jujur, bukan ditulis ulang diam-diam.
func masukanTabTerkelolaB589(tool string) (string, int) {
	switch tool {
	case "goose":
		return "GOOSE_MODEL:\tlama\n", 1
	case "aider":
		return "model:\tlama\n", 1
	default: // hermes
		return "model:\n  default:\tlama\n", 2
	}
}

// TestB589TabDitolakJujur mengunci B-589: Configure atas
// berkas yang memuat TAB di posisi pemisah yang ditolak
// PyYAML gagal jujur SEBELUM berkas apa pun ditulis, dengan
// pesan yang menamai TAB dan nomor barisnya; berkas utuh.
// Untuk Goose pesannya menandai bahwa perilaku serde_yaml
// atas posisi itu belum terbukti (bukan klaim yang
// dikarang); untuk Aider/Hermes pesannya menamai PyYAML.
func TestB589TabDitolakJujur(t *testing.T) {
	for _, tc := range kasusTabB589 {
		for _, tool := range []string{"hermes", "aider", "goose"} {
			t.Run(tc.nama+"/"+tool, func(t *testing.T) {
				home := t.TempDir()
				jalur := jalurKonfigB559(home, tool)
				tulisBerkasPRD(t, jalur, []byte(tc.masukan))

				err := terapkanPRD(t, home, tool, "")
				if err == nil {
					t.Fatalf("applyToolSpecificConfig %s melaporkan sukses atas TAB di %s (sukses-bohong B-589)", tool, tc.nama)
				}
				pesan := err.Error()
				if !strings.Contains(pesan, "TAB") {
					t.Errorf("pesan galat tidak menamai TAB (%s/%s): %v", tc.nama, tool, err)
				}
				if !strings.Contains(pesan, "baris "+itoa(tc.baris)) {
					t.Errorf("pesan galat tidak menamai baris %d (%s/%s): %v", tc.baris, tc.nama, tool, err)
				}
				switch tool {
				case "goose":
					if !strings.Contains(pesan, "serde_yaml") {
						t.Errorf("pesan Goose tidak menandai status serde_yaml (%s): %v", tc.nama, err)
					}
				default:
					if !strings.Contains(pesan, "PyYAML") {
						t.Errorf("pesan %s tidak menamai PyYAML (%s): %v", tool, tc.nama, err)
					}
				}
				mentah, readErr := os.ReadFile(jalur)
				if readErr != nil {
					t.Fatalf("membaca berkas sesudah penolakan: %v", readErr)
				}
				if string(mentah) != tc.masukan {
					t.Errorf("berkas berubah sesudah penolakan jujur:\n%q", string(mentah))
				}
			})
		}
	}

	for _, tool := range []string{"hermes", "aider", "goose"} {
		t.Run("terkelola/"+tool, func(t *testing.T) {
			masukan, baris := masukanTabTerkelolaB589(tool)
			home := t.TempDir()
			jalur := jalurKonfigB559(home, tool)
			tulisBerkasPRD(t, jalur, []byte(masukan))

			err := terapkanPRD(t, home, tool, "")
			if err == nil {
				t.Fatalf("applyToolSpecificConfig %s melaporkan sukses atas TAB di baris terkelola", tool)
			}
			if !strings.Contains(err.Error(), "TAB") || !strings.Contains(err.Error(), "baris "+itoa(baris)) {
				t.Errorf("pesan galat tidak menamai TAB + baris %d: %v", baris, err)
			}
			mentah, _ := os.ReadFile(jalur)
			if string(mentah) != masukan {
				t.Errorf("berkas berubah sesudah penolakan:\n%q", string(mentah))
			}
		})
	}
}

// kontrolTabB589 adalah masukan yang WAJIB lolos Configure di
// ketiga tool: TAB-nya berada di posisi yang DITERIMA PyYAML
// (oracle) — di dalam skalar terkutip, di dalam komentar, di
// tengah isi skalar blok — atau pada baris penanda yang
// dinormalisasi penulis (B-544/B-559). Perbaikan tidak boleh
// mengubah penolakan TAB menjadi penolakan atas segala TAB.
var kontrolTabB589 = []struct {
	nama    string
	masukan string
	periksa func(t *testing.T, doc map[string]any, konten string)
}{
	{
		"kutip-ganda", "a: \"x\ty\"\nlainnya: 1\n",
		func(t *testing.T, doc map[string]any, konten string) {
			if doc["a"] != "x\ty" || doc["lainnya"] != 1 {
				t.Errorf("nilai berkutip rusak: %v\n%s", doc, konten)
			}
		},
	},
	{
		"kutip-tunggal", "a: 'x\ty'\nlainnya: 1\n",
		func(t *testing.T, doc map[string]any, konten string) {
			if doc["a"] != "x\ty" || doc["lainnya"] != 1 {
				t.Errorf("nilai berkutip rusak: %v\n%s", doc, konten)
			}
		},
	},
	{
		"komentar", "a: 1 # komen\tdengan tab\nlainnya: 1\n",
		func(t *testing.T, doc map[string]any, konten string) {
			if doc["a"] != 1 || doc["lainnya"] != 1 {
				t.Errorf("nilai berkomentar rusak: %v\n%s", doc, konten)
			}
		},
	},
	{
		"blok-tengah-isi", "a: |\n  teks\tlanjutan\nlainnya: 1\n",
		func(t *testing.T, doc map[string]any, konten string) {
			if doc["a"] != "teks\tlanjutan\n" || doc["lainnya"] != 1 {
				t.Errorf("isi skalar blok rusak (TAB tengah isi wajib utuh): %v\n%s", doc, konten)
			}
		},
	},
	{
		"kunci-terkutip", "\"a\tb\": 1\nlainnya: 1\n",
		func(t *testing.T, doc map[string]any, konten string) {
			if doc["a\tb"] != 1 || doc["lainnya"] != 1 {
				t.Errorf("kunci terkutip rusak: %v\n%s", doc, konten)
			}
		},
	},
	{
		"kutip-multiline", "a: \"baris1\nbaris2\tx\"\nlainnya: 1\n",
		func(t *testing.T, doc map[string]any, konten string) {
			if doc["a"] != "baris1 baris2\tx" || doc["lainnya"] != 1 {
				t.Errorf("skalar terkutip multiline rusak: %v\n%s", doc, konten)
			}
		},
	},
	{
		"penanda-tab", "lainnya: 1\n...\n...\t# selesai\n",
		func(t *testing.T, doc map[string]any, konten string) {
			if doc["lainnya"] != 1 {
				t.Errorf("lainnya = %v, mau 1:\n%s", doc["lainnya"], konten)
			}
			if strings.Contains(konten, "...\t") {
				t.Errorf("penanda kedua berbentuk tab tidak dinormalisasi:\n%s", konten)
			}
		},
	},
}

// TestB589TabSahLolos adalah kontrol pembeda B-589: TAB di
// posisi sah tetap lolos dan nilainya utuh di ketiga tool.
func TestB589TabSahLolos(t *testing.T) {
	for _, tc := range kontrolTabB589 {
		for _, tool := range []string{"hermes", "aider", "goose"} {
			t.Run(tc.nama+"/"+tool, func(t *testing.T) {
				home := t.TempDir()
				jalur := jalurKonfigB559(home, tool)
				tulisBerkasPRD(t, jalur, []byte(tc.masukan))

				if err := terapkanPRD(t, home, tool, ""); err != nil {
					t.Fatalf("applyToolSpecificConfig %s menolak TAB sah %q: %v", tool, tc.nama, err)
				}
				konten := bacaBerkasUji(t, jalur)
				doc := uraiYAMLUji(t, konten)
				tc.periksa(t, doc, konten)
				switch tool {
				case "hermes":
					blok, _ := doc["model"].(map[string]any)
					if blok["default"] != "model-baru" {
						t.Errorf("blok model tidak efektif: %v\n%s", doc["model"], konten)
					}
				case "aider":
					if doc["model"] != "openai/model-baru" {
						t.Errorf("model aider = %v, mau openai/model-baru:\n%s", doc["model"], konten)
					}
				case "goose":
					if doc["GOOSE_MODEL"] != "model-baru" {
						t.Errorf("GOOSE_MODEL = %v, mau model-baru:\n%s", doc["GOOSE_MODEL"], konten)
					}
				}
			})
		}
	}
}

// TestB589MutasiTab mengunci bahwa vonis penjaga TAB mengikuti
// POSISI tab, bukan sekadar keberadaan byte TAB: memindahkan
// satu TAB dari posisi pemisah ke dalam kutip / komentar /
// isi skalar blok membalik vonis dari tolak menjadi lolos,
// dan sebaliknya.
func TestB589MutasiTab(t *testing.T) {
	pasangan := []struct {
		nama  string
		tolak string // TAB di posisi pemisah
		lolos string // TAB yang sama dipindah ke posisi sah
	}{
		{"ke-kutip", "a: b\tc\n", "a: \"b\tc\"\n"},
		{"ke-komentar", "a: 1\t# c\n", "a: 1 # c\tkomentar\n"},
		{"ke-isi-blok", "a: |\n\tx\n", "a: |\n  x\ty\n"},
	}
	for _, p := range pasangan {
		for _, tool := range []string{"hermes", "aider", "goose"} {
			t.Run(p.nama+"/"+tool, func(t *testing.T) {
				home := t.TempDir()
				jalur := jalurKonfigB559(home, tool)

				tulisBerkasPRD(t, jalur, []byte(p.tolak))
				if err := terapkanPRD(t, home, tool, ""); err == nil {
					t.Fatalf("mutasi %s/%s: TAB pemisah diloloskan", p.nama, tool)
				}
				tulisBerkasPRD(t, jalur, []byte(p.lolos))
				if err := terapkanPRD(t, home, tool, ""); err != nil {
					t.Fatalf("mutasi %s/%s: TAB sah ditolak: %v", p.nama, tool, err)
				}
			})
		}
	}
}

// situsPemisahDiizinkanB588 adalah tabel sapuan mekanis atas
// semua situs penulis yang memecah pada "\n" di manager.go:
// tiap fungsi yang masih memakai strings.Split(..., "\n")
// dicantumkan beserta alasannya (bukan penulis YAML). Fungsi
// penulis YAML (perbaruiYAMLTopLevel, susunBlokModelHermes,
// pindaiJangkarKunciYAML, cariKutipBlokMenelanKunciYAML)
// TIDAK BOLEH ada di tabel ini: mereka memecah pada pemisah
// garis YAML (pecahBarisYAML / pecahBarisBerpemisaahYAML).
var situsPemisahDiizinkanB588 = map[string]string{
	"ekstrakVersi":            "keluaran perintah versi tool, bukan YAML",
	"perbaruiPreambleTOML":    "penulis TOML, bukan YAML",
	"perbaruiKunciSeksiTOML":  "penulis TOML, bukan YAML",
	"perbaruiBarisEnv":        "penulis .env, bukan YAML",
	"verifikasiEnvHermes":     "verifikasi .env, bukan YAML",
	"removeAPIKeysFromScript": "skrip shell, bukan YAML",
	"redactAPIKeysInScript":   "skrip shell, bukan YAML",
}

// TestFase27PRDSapuanMekanisSitusPemisah mengunci sapuan
// mekanis B-588: tidak ada satu pun situs di manager.go yang
// memecah teks YAML pada "\n" saja — tiap strings.Split(,
// "\n") yang tersisa berada di fungsi non-YAML yang
// terdaftar di situsPemisahDiizinkanB588, dan tiap fungsi
// penulis YAML memakai pemecah pemisah-garis YAML.
func TestFase27PRDSapuanMekanisSitusPemisah(t *testing.T) {
	// go test berjalan dengan cwd direktori paket.
	mentah, err := os.ReadFile("manager.go")
	if err != nil {
		t.Fatalf("membaca manager.go: %v", err)
	}
	baris := strings.Split(string(mentah), "\n")
	fungsiKini := ""
	situs := map[string][]int{} // fungsi -> nomor baris pemecah "\n"
	for i, l := range baris {
		trim := strings.TrimSpace(l)
		if strings.HasPrefix(trim, "func ") {
			fungsiKini = namaFungsiDariBaris(trim)
		}
		if strings.Contains(l, "strings.Split(") && strings.Contains(l, `"\n"`) {
			situs[fungsiKini] = append(situs[fungsiKini], i+1)
		}
	}
	for fungsi, nomor := range situs {
		alasan, ok := situsPemisahDiizinkanB588[fungsi]
		if !ok {
			t.Errorf("fungsi %s masih memecah pada \"\\n\" (baris %v) — situs penulis YAML wajib memakai pecahBarisYAML/pecahBarisBerpemisaahYAML", fungsi, nomor)
		} else {
			t.Logf("diizinkan: %s (baris %v) — %s", fungsi, nomor, alasan)
		}
	}
	if len(situs) == 0 {
		t.Fatal("sapuan tidak menemukan satu pun strings.Split(..., \"\\n\") — pemindai sapuan rusak")
	}
	for _, fungsi := range []string{"perbaruiYAMLTopLevel", "susunBlokModelHermes"} {
		if !fungsiMemakaiPemecah(baris, fungsi, "pecahBarisBerpemisaahYAML") {
			t.Errorf("fungsi penulis %s tidak memakai pecahBarisBerpemisaahYAML", fungsi)
		}
	}
	for _, fungsi := range []string{"pindaiJangkarKunciYAML", "cariKutipBlokMenelanKunciYAML"} {
		if !fungsiMemakaiPemecah(baris, fungsi, "pecahBarisYAML") {
			t.Errorf("fungsi %s tidak memakai pecahBarisYAML", fungsi)
		}
	}
}

// fungsiMemakaiPemecah melaporkan apakah badan fungsi bernama
// memakai pemecah yang disebutkan (pencarian tekstual kasar
// atas rentang badan fungsi).
func fungsiMemakaiPemecah(baris []string, fungsi, pemecah string) bool {
	diDalam := false
	for _, l := range baris {
		trim := strings.TrimSpace(l)
		if strings.HasPrefix(trim, "func ") {
			nama := namaFungsiDariBaris(trim)
			if diDalam {
				return false
			}
			diDalam = nama == fungsi
			continue
		}
		if diDalam && strings.Contains(l, pemecah) {
			return true
		}
	}
	return false
}

// namaFungsiDariBaris mengambil nama fungsi dari satu baris
// deklarasi ("func nama(...)" atau "func (m *Manager)
// nama(...)").
func namaFungsiDariBaris(baris string) string {
	sisa := strings.TrimPrefix(baris, "func ")
	sisa = strings.TrimSpace(sisa)
	if strings.HasPrefix(sisa, "(") {
		if j := strings.Index(sisa, ")"); j >= 0 {
			sisa = strings.TrimSpace(sisa[j+1:])
		}
	}
	if j := strings.Index(sisa, "("); j >= 0 {
		sisa = sisa[:j]
	}
	return strings.TrimSpace(sisa)
}

// itoa adalah strconv.Itoa lokal (menghindari impor baru di
// berkas test ini).
func itoa(n int) string {
	if n == 0 {
		return "0"
	}
	neg := n < 0
	if neg {
		n = -n
	}
	var b [20]byte
	i := len(b)
	for n > 0 {
		i--
		b[i] = byte('0' + n%10)
		n /= 10
	}
	if neg {
		i--
		b[i] = '-'
	}
	return string(b[i:])
}
