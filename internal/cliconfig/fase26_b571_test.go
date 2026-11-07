package cliconfig

// fase26_b571_test.go — pengunci Fase 26 PR-B (cliconfig B-571):
// Hermes sukses-bohong atas pemisah garis tersembunyi (CR /
// U+2028 / U+0085 / U+2029) dalam komentar sesudah penanda
// pertama. Penjaga B-543, pengenal penanda, dan penulis semuanya
// memecah teks hanya pada "\n", sementara YAML mengenal pemisah
// garis lain — satu "baris-\n" dapat memuat dua baris YAML.
// Keempat sub-bentuk terbukti dengan PyYAML sungguhan (oracle):
// Hermes err=nil + berkas tertulis, PyYAML menolak
// "ParserError: expected '<document start>'"; Goose/Aider gagal
// jujur atas keempatnya lewat verifikasi ketat semua-dokumen.
// Perbaikan dua lapis: (1) penjaga B-543 memecah pada pemisah
// garis YAML (pecahBarisYAML) sehingga isi tersembunyi sesudah
// penanda ditolak jujur sebelum berkas apa pun ditulis;
// (2) verifikasi Hermes mengurai ketat ekor sesudah dokumen
// pertama (uraiYAMLHermes), jadi kelas ini tertutup juga bila
// suatu jalur penulis melewati penjaga.
//
// Semua test memanggil applyToolSpecificConfig — jalur Configure
// sungguhan — lewat pembantu terapkanPRD, kecuali dua test
// satuan terakhir yang mengunci masing-masing lapis secara
// langsung (penjaga; ekor verifikasi).

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

// pemisahTersembunyiB571 adalah pemisah garis YAML yang tidak
// terlihat oleh pemecahan "\n": CR saja, NEL, LINE SEPARATOR,
// PARAGRAPH SEPARATOR. CRLF bukan anggota: ia pemisah yang
// terlihat "\n" dan sudah ditangkap penjaga B-543 sejak dulu
// (lihat kontrol TestB571KontrolLolos).
var pemisahTersembunyiB571 = []struct {
	nama string
	sep  string
}{
	{"CR", "\r"},
	{"NEL", "\u0085"},
	{"LS", "\u2028"},
	{"PS", "\u2029"},
}

// posisiB571 adalah tiga posisi pemisah tersembunyi yang
// terbukti sukses-bohong: di komentar penanda KEDUA, di komentar
// penanda PERTAMA, dan di baris komentar biasa sesudah penanda
// pertama. Tiap posisi menyusun masukan dari pemisah yang
// diberikan; isi tersembunyi sesudah pemisah selalu baris
// pemetaan "kunci: 2" — baris YAML kedua yang dilihat pemuat.
var posisiB571 = []struct {
	nama    string
	masukan func(sep string) string
}{
	{"penanda-kedua", func(sep string) string {
		return "lainnya: 1\n...\n... # selesai" + sep + "kunci: 2\n"
	}},
	{"penanda-pertama", func(sep string) string {
		return "lainnya: 1\n... # selesai" + sep + "kunci: 2\n"
	}},
	{"komentar-biasa", func(sep string) string {
		return "lainnya: 1\n...\n# biasa" + sep + "kunci: 2\n"
	}},
}

// TestB571PemisahTersembunyiDitolakJujur mengunci B-571: sapuan
// mekanis pemisah tersembunyi × posisi × tool. Hermes ditolak
// penjaga B-543 ("isi sesudah penanda akhir dokumen");
// Goose/Aider ditolak verifikasi ketat semua-dokumennya
// ("tidak terurai") — kontrol gagal-jujur yang membuktikan
// lubang ini khusus Hermes. Di semua tool berkas TIDAK berubah.
func TestB571PemisahTersembunyiDitolakJujur(t *testing.T) {
	for _, pem := range pemisahTersembunyiB571 {
		for _, pos := range posisiB571 {
			for _, tool := range []string{"hermes", "aider", "goose"} {
				nama := pem.nama + "/" + pos.nama + "/" + tool
				t.Run(nama, func(t *testing.T) {
					home := t.TempDir()
					jalur := jalurKonfigB559(home, tool)
					masukan := pos.masukan(pem.sep)
					tulisBerkasPRD(t, jalur, []byte(masukan))

					err := terapkanPRD(t, home, tool, "")
					if err == nil {
						t.Fatalf("applyToolSpecificConfig %s melaporkan sukses atas pemisah %s di %s (sukses-bohong B-571)", tool, pem.nama, pos.nama)
					}
					switch tool {
					case "hermes":
						if !strings.Contains(err.Error(), "sesudah penanda") {
							t.Errorf("penolakan Hermes tidak berasal dari penjaga B-543: %v", err)
						}
					default:
						if !strings.Contains(err.Error(), "tidak terurai") {
							t.Errorf("penolakan %s tidak berasal dari verifikasi ketat semua-dokumen: %v", tool, err)
						}
					}
					mentah, readErr := os.ReadFile(jalur)
					if readErr != nil {
						t.Fatalf("membaca berkas sesudah penolakan: %v", readErr)
					}
					if string(mentah) != masukan {
						t.Errorf("berkas berubah sesudah penolakan jujur:\n%q", string(mentah))
					}
				})
			}
		}
	}
}

// kontrolB571 adalah masukan yang WAJIB lolos Configure di
// ketiga tool: pemisah garisnya terlihat pemindai "\n" (CRLF)
// atau ekornya memang hanya komentar/penanda (sah menurut
// PyYAML, terbukti dengan pustaka sungguhan).
var kontrolB571 = []struct {
	nama    string
	masukan string
}{
	{"CRLF-seluruh-berkas", "lainnya: 1\r\n...\r\n"},
	{"penanda-kedua-polos", "lainnya: 1\n...\n...\n"},
	{"penanda-kedua-berkomentar", "lainnya: 1\n...\n... # selesai\n"},
	{"komentar-sesudah-penanda-CRLF", "lainnya: 1\n...\n# biasa\r\n"},
	{"komentar-sesudah-penanda-CR-akhir-EOF", "lainnya: 1\n...\n# biasa\r"},
}

// TestB571KontrolLolos adalah kontrol pembeda B-571: ekor yang
// sah menurut PyYAML tetap lolos dan termuat di ketiga tool —
// perbaikan tidak boleh mengubah penolakan menjadi penolakan
// atas segala yang mengandung CR. Baris isi NYATA sesudah
// penanda yang dipisah CRLF (terlihat "\n") tetap ditolak
// penjaga B-543 seperti sebelum perbaikan.
func TestB571KontrolLolos(t *testing.T) {
	for _, tc := range kontrolB571 {
		for _, tool := range []string{"hermes", "aider", "goose"} {
			t.Run(tc.nama+"/"+tool, func(t *testing.T) {
				home := t.TempDir()
				jalur := jalurKonfigB559(home, tool)
				tulisBerkasPRD(t, jalur, []byte(tc.masukan))

				if err := terapkanPRD(t, home, tool, ""); err != nil {
					t.Fatalf("applyToolSpecificConfig %s menolak kontrol sah %q: %v", tool, tc.nama, err)
				}
				konten := bacaBerkasUji(t, jalur)
				doc, err := uraiYAMLUtuh(konten)
				if err != nil {
					t.Fatalf("hasil sunting kontrol %q tidak terurai utuh: %v\n%s", tc.nama, err, konten)
				}
				switch tool {
				case "hermes":
					blok, ok := doc["model"].(map[string]any)
					if !ok || blok["default"] != "model-baru" {
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
				if doc["lainnya"] != 1 {
					t.Errorf("kunci pengguna rusak: %v\n%s", doc, konten)
				}
			})
		}
	}

	// Batas: isi NYATA sesudah penanda yang dipisah CRLF sudah
	// ditolak penjaga B-543 sebelum B-571 — bukan regresi.
	for _, tool := range []string{"hermes", "aider", "goose"} {
		t.Run("isi-nyata-CRLF-ditolak/"+tool, func(t *testing.T) {
			home := t.TempDir()
			jalur := jalurKonfigB559(home, tool)
			masukan := "lainnya: 1\r\n...\r\nkunci: 2\r\n"
			tulisBerkasPRD(t, jalur, []byte(masukan))
			if err := terapkanPRD(t, home, tool, ""); err == nil {
				t.Fatalf("applyToolSpecificConfig %s melaporkan sukses atas isi nyata sesudah penanda (CRLF)", tool)
			}
			mentah, _ := os.ReadFile(jalur)
			if string(mentah) != masukan {
				t.Errorf("berkas berubah sesudah penolakan:\n%q", string(mentah))
			}
		})
	}
}

// TestB571PecahBarisYAML mengunci lapis pertama secara langsung:
// penjaga B-543 memakai pecahBarisYAML, dan pemecah itu sendiri
// membelah pada tiap pemisah garis YAML (CRLF dihitung satu)
// sambil meniru strings.Split pada "\n" untuk teks biasa.
func TestB571PecahBarisYAML(t *testing.T) {
	pecah := []struct {
		nama string
		teks string
		mau  []string
	}{
		{"LF", "a\nb\n", []string{"a", "b", ""}},
		{"CRLF-satu-pemisah", "a\r\nb\r\n", []string{"a", "b", ""}},
		{"CR-saja", "a\rb\r", []string{"a", "b", ""}},
		{"NEL", "a\u0085b", []string{"a", "b"}},
		{"LS", "a\u2028b", []string{"a", "b"}},
		{"PS", "a\u2029b", []string{"a", "b"}},
		{"campur", "a\r\nb\rc\u0085d\u2028e", []string{"a", "b", "c", "d", "e"}},
		{"tanpa-pemisah", "abc", []string{"abc"}},
		{"kosong", "", []string{""}},
		{"CR-akhir-EOF", "# biasa\r", []string{"# biasa", ""}},
	}
	for _, tc := range pecah {
		t.Run(tc.nama, func(t *testing.T) {
			got := pecahBarisYAML(tc.teks)
			if len(got) != len(tc.mau) {
				t.Fatalf("pecahBarisYAML(%q) = %q, mau %q", tc.teks, got, tc.mau)
			}
			for i := range tc.mau {
				if got[i] != tc.mau[i] {
					t.Errorf("pecahBarisYAML(%q)[%d] = %q, mau %q", tc.teks, i, got[i], tc.mau[i])
				}
			}
		})
	}

	for _, pem := range pemisahTersembunyiB571 {
		for _, pos := range posisiB571 {
			t.Run("penjaga/"+pem.nama+"/"+pos.nama, func(t *testing.T) {
				if err := tolakIsiSesudahPenandaHermes(filepath.Join(t.TempDir(), "config.yaml"), pos.masukan(pem.sep)); err == nil {
					t.Errorf("tolakIsiSesudahPenandaHermes meloloskan pemisah %s di %s", pem.nama, pos.nama)
				}
			})
		}
	}
	for _, tc := range kontrolB571 {
		t.Run("penjaga-kontrol/"+tc.nama, func(t *testing.T) {
			if err := tolakIsiSesudahPenandaHermes(filepath.Join(t.TempDir(), "config.yaml"), tc.masukan); err != nil {
				t.Errorf("tolakIsiSesudahPenandaHermes menolak kontrol sah %q: %v", tc.nama, err)
			}
		})
	}
}

// TestB571VerifikasiEkorKetat mengunci lapis kedua secara
// langsung: verifikasi Hermes kini mengurai ketat ekor sesudah
// dokumen pertama, jadi isi tersembunyi yang lolos penjaga
// (mis. jalur penulis masa depan) tetap gagal di verifikasi
// — bukan sukses-bohong. Masukan di sini adalah HASIL susun
// (blok model sah + ekor), bukan berkas masukan Configure.
func TestB571VerifikasiEkorKetat(t *testing.T) {
	blok := "model:\n" +
		"  provider: custom\n" +
		"  default: model-baru\n" +
		"  base_url: http://localhost:8080/v1\n" +
		"  api_mode: chat_completions\n"
	ekor := []struct {
		nama     string
		konten   string
		mauGalat bool
	}{
		{"ekor-sah-penanda-kedua", blok + "...\n... # selesai\n", false},
		{"ekor-sah-komentar", blok + "...\n# biasa\n", false},
		{"ekor-sah-CRLF", blok + "...\r\n", false},
		{"ekor-tersembunyi-CR", blok + "...\n... # selesai\rkunci: 2\n", true},
		{"ekor-tersembunyi-LS", blok + "...\n... # selesai\u2028kunci: 2\n", true},
		{"ekor-tersembunyi-NEL", blok + "...\n# biasa\u0085kunci: 2\n", true},
		{"ekor-tersembunyi-PS", blok + "... # selesai\u2029kunci: 2\n", true},
		{"ekor-dokumen-kedua-nyata", blok + "...\n---\nkunci: 2\n", true},
	}
	for _, tc := range ekor {
		t.Run(tc.nama, func(t *testing.T) {
			err := verifikasiHasilHermes(tc.konten, "model-baru", urlUjiPRD, "")
			if tc.mauGalat && err == nil {
				t.Errorf("verifikasiHasilHermes meloloskan ekor %q (sukses-bohong B-571)", tc.nama)
			}
			if !tc.mauGalat && err != nil {
				t.Errorf("verifikasiHasilHermes menolak ekor sah %q: %v", tc.nama, err)
			}
		})
	}
}
