package cliconfig

// fase25_prb_test.go — pengunci Fase 25 PR-B (cliconfig: penanda
// kedua+ & pelacak skalar blok):
//
//	B-559 penanda KEDUA+ berbentuk tab/tanpa-spasi DINORMALISASI
//	      saat menulis (bukan hanya penanda pertama, B-544) —
//	      penjaga B-543 mengecualikan baris mirip-penanda sesudah
//	      penanda pertama, dan verifikasi Hermes hanya mengurai
//	      dokumen pertama, sehingga bentuk-bentuk itu dulu lolos
//	      sebagai sukses-bohong: Configure err=nil, berkas
//	      tertulis dengan penanda kedua apa adanya, PyYAML
//	      menolak hasilnya (ScannerError/ParserError, terbukti
//	      dengan pustaka sungguhan). Sapuan mekanis atas
//	      pengenal penanda menemukan 7 bentuk yang ditolak
//	      PyYAML — ketujuhnya dikunci di sini (lima bentuk
//	      PB12-2 berada di antaranya); kontrol pembeda penanda
//	      kedua polos/berkomentar-spasi lolos dan termuat.
//	B-560 pelacak skalar YAML memperhitungkan keadaan skalar
//	      BLOK (|/>): kutip pada posisi pembuka di ISI blok yang
//	      tak menutup pada baris yang sama tidak lagi membuka
//	      skalar semu yang menelan struktur asli sesudah blok
//	      berakhir. Masukan terbukti SAH menurut PyYAML: Goose
//	      dulu gagal "mapping key … already defined" (pesan
//	      menyesatkan — menamai duplikasi, bukan sebabnya),
//	      Aider sama; Hermes dulu sukses tetapi hasil memuat DUA
//	      blok `model:` (nilai efektif last-wins tetap benar —
//	      degradasi berkas). Kontrol negatif: kutip sesudah kata
//	      polos di isi blok tidak meracuni. Alasan tertulis
//	      B-550 dikualifikasi: benar untuk isi-sebagai-struktur,
//	      tidak menutup isi-sebagai-keadaan-pelacak.
//	Rider redaksi: komentar kepala fase24_prb_test.go menamai
//	      test B-544 `…DitolakNormalisasi`, nama sebenarnya
//	      `TestB544PenandaTabDinormalisasi` — diperbaiki.
//
// Semua test memanggil applyToolSpecificConfig — jalur Configure
// sungguhan — lewat pembantu terapkanPRD.

import (
	"path/filepath"
	"strings"
	"testing"
)

// bentukPenandaKeduaB559 adalah sapuan mekanis bentuk penanda
// kedua yang DITOLAK PyYAML (diperiksa dengan pustaka sungguhan
// atas aliran "lainnya: 1\n...\n<bentuk>\n"): tiap bentuk adalah
// baris penanda kedua (tanpa newline) beserta bentuk kanonik yang
// wajib tertulis sesudah Configure.
var bentukPenandaKeduaB559 = []struct {
	nama    string
	bentuk  string // baris penanda kedua mentah (tanpa \n)
	kanonik string // baris yang wajib tertulis (tanpa \n)
	crlf    bool   // masukan seluruhnya CRLF (bentuk ini saja)
}{
	{"tab akhir", "...\t", "...", false},
	{"tab sebelum komentar", "...\t# selesai", "... # selesai", false},
	{"tanpa spasi sebelum komentar", "...# selesai", "... # selesai", false},
	{"tanpa spasi, komentar menempel", "...#selesai", "... #selesai", false},
	{"spasi lalu tab sebelum komentar", "... \t# selesai", "... # selesai", false},
	{"tab ganda sebelum komentar", "...\t\t# selesai", "... # selesai", false},
	{"tab sebelum komentar CRLF", "...\t# selesai", "... # selesai", true},
}

// jalurKonfigB559 mengembalikan jalur berkas config tiap tool di
// bawah home uji.
func jalurKonfigB559(home, tool string) string {
	switch tool {
	case "hermes":
		return filepath.Join(home, ".hermes", "config.yaml")
	case "aider":
		return filepath.Join(home, ".aider.conf.yml")
	default: // goose
		return filepath.Join(home, ".config", "goose", "config.yaml")
	}
}

// masukanB559 menyusun config satu dokumen dengan penanda pertama
// kanonik dan penanda kedua berbentuk tc.bentuk; denganBlok
// menaruh blok model lama sebelum penanda (jalur gabung Hermes).
func masukanB559(tc struct {
	nama    string
	bentuk  string
	kanonik string
	crlf    bool
}, denganBlok bool) string {
	awalan := "lainnya: 1\n"
	if denganBlok {
		awalan = "model:\n  default: lama\nlainnya: 1\n"
	}
	if tc.crlf {
		awalan = strings.ReplaceAll(awalan, "\n", "\r\n")
		return awalan + "...\r\n" + tc.bentuk + "\r\n"
	}
	return awalan + "...\n" + tc.bentuk + "\n"
}

// hitungBarisKanonikB559 menghitung baris yang — sesudah buang CR
// akhir — sama persis dengan kanonik.
func hitungBarisKanonikB559(konten, kanonik string) int {
	n := 0
	for _, l := range strings.Split(konten, "\n") {
		if strings.TrimSuffix(l, "\r") == kanonik {
			n++
		}
	}
	return n
}

// TestB559PenandaKeduaDinormalisasi mengunci B-559: penanda kedua
// berbentuk tab/tanpa-spasi DINORMALISASI ke bentuk kanonik saat
// menulis, di ketiga tool (Hermes lewat susunBlokModelHermes
// cabang tanpa-blok dan cabang gabung-blok; Aider & Goose lewat
// penyunting bersama perbaruiYAMLTopLevel). Sebelum perbaikan
// Configure sukses tetapi penanda kedua tertulis apa adanya dan
// PyYAML menolak hasilnya.
func TestB559PenandaKeduaDinormalisasi(t *testing.T) {
	for _, tc := range bentukPenandaKeduaB559 {
		for _, tool := range []string{"hermes", "aider", "goose"} {
			for _, denganBlok := range []bool{false, true} {
				if denganBlok && tool != "hermes" {
					continue // cabang gabung-blok hanya ada di Hermes
				}
				nama := tc.nama + "/" + tool
				if denganBlok {
					nama += "/dengan-blok"
				}
				t.Run(nama, func(t *testing.T) {
					home := t.TempDir()
					jalur := jalurKonfigB559(home, tool)
					tulisBerkasPRD(t, jalur, []byte(masukanB559(tc, denganBlok)))

					if err := terapkanPRD(t, home, tool, ""); err != nil {
						t.Fatalf("applyToolSpecificConfig %s atas penanda kedua %q: %v", tool, tc.bentuk, err)
					}
					konten := bacaBerkasUji(t, jalur)
					if n := hitungBarisKanonikB559(konten, tc.kanonik); n < 1 {
						t.Errorf("penanda kedua tidak ternormalisasi ke %q:\n%q", tc.kanonik, konten)
					}
					if strings.Contains(konten, "...\t") || strings.Contains(konten, "...#") {
						t.Errorf("bentuk penanda tab/tanpa-spasi masih tertinggal:\n%q", konten)
					}
					doc, err := uraiYAMLUtuh(konten)
					if err != nil {
						t.Fatalf("hasil sunting tidak terurai utuh: %v\n%s", err, konten)
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
						t.Errorf("kunci pengguna sesudah penanda rusak: %v\n%s", doc, konten)
					}
				})
			}
		}
	}
}

// TestB559PenandaKeduaKontrolLolos adalah kontrol pembeda B-559:
// penanda kedua POLOS dan BERKOMENTAR-SPASI (termasuk spasi
// ganda yang diterima PyYAML) lolos dan termuat — Configure
// berhasil dan hasilnya terurai utuh; bentuk spasi ganda
// dilipat ke kanonik satu spasi oleh penulis.
func TestB559PenandaKeduaKontrolLolos(t *testing.T) {
	for _, tc := range []struct {
		nama    string
		bentuk  string
		kanonik string
	}{
		{"polos", "...", "..."},
		{"komentar spasi", "... # selesai", "... # selesai"},
		{"komentar spasi ganda", "...  # selesai", "... # selesai"},
	} {
		t.Run(tc.nama, func(t *testing.T) {
			home := t.TempDir()
			jalur := filepath.Join(home, ".hermes", "config.yaml")
			tulisBerkasPRD(t, jalur, []byte("lainnya: 1\n...\n"+tc.bentuk+"\n"))

			if err := terapkanPRD(t, home, "hermes", ""); err != nil {
				t.Fatalf("applyToolSpecificConfig hermes atas penanda kedua %q: %v", tc.bentuk, err)
			}
			konten := bacaBerkasUji(t, jalur)
			if n := hitungBarisKanonikB559(konten, tc.kanonik); n < 1 {
				t.Errorf("penanda kedua %q tidak termuat sebagai %q:\n%q", tc.bentuk, tc.kanonik, konten)
			}
			doc, err := uraiYAMLUtuh(konten)
			if err != nil {
				t.Fatalf("hasil sunting tidak terurai utuh: %v\n%s", err, konten)
			}
			blok, ok := doc["model"].(map[string]any)
			if !ok || blok["default"] != "model-baru" {
				t.Errorf("blok model tidak efektif: %v\n%s", doc["model"], konten)
			}
		})
	}
}

// hitungHeaderKolomNolB560 menghitung baris kolom-0 yang sama
// persis dengan header (mis. "model:" — bukan "model: lama"
// di dalam skalar, bukan sub-baris menjorok).
func hitungHeaderKolomNolB560(konten, header string) int {
	n := 0
	for _, l := range strings.Split(konten, "\n") {
		if strings.TrimSuffix(l, "\r") == header {
			n++
		}
	}
	return n
}

// TestB560KutipSkalarBlokTakMeracuni mengunci B-560 untuk ketiga
// tool: kutip tak tertutup di ISI skalar blok (| dan >) pada
// posisi pembuka (awal baris isi, atau sesudah ':') tidak membuka
// skalar semu — struktur asli sesudah blok tetap terbaca.
// Masukan terbukti SAH menurut PyYAML; sebelum perbaikan Goose
// dan Aider gagal dengan pesan duplikasi yang menyesatkan,
// Hermes menulis dua blok `model:`.
func TestB560KutipSkalarBlokTakMeracuni(t *testing.T) {
	t.Run("goose", func(t *testing.T) {
		for _, tc := range []struct {
			nama    string
			isi     string
			catatan string
		}{
			{"kutip ganda awal baris isi",
				"catatan: |\n  \"kutip tak tertutup\n  lanjut\nGOOSE_MODEL: lama\nlainnya: 1\n",
				"\"kutip tak tertutup\nlanjut\n"},
			{"kutip ganda sesudah kolon",
				"catatan: |\n  contoh: \"kutip tak tertutup\n  lanjut\nGOOSE_MODEL: lama\nlainnya: 1\n",
				"contoh: \"kutip tak tertutup\nlanjut\n"},
			{"kutip tunggal awal baris isi",
				"catatan: |\n  'kutip tak tertutup\n  lanjut\nGOOSE_MODEL: lama\nlainnya: 1\n",
				"'kutip tak tertutup\nlanjut\n"},
			{"kutip ganda di skalar lipat",
				"catatan: >\n  \"kutip tak tertutup\n  lanjut\nGOOSE_MODEL: lama\nlainnya: 1\n",
				"\"kutip tak tertutup lanjut\n"},
		} {
			t.Run(tc.nama, func(t *testing.T) {
				home := t.TempDir()
				jalur := filepath.Join(home, ".config", "goose", "config.yaml")
				tulisBerkasPRD(t, jalur, []byte(tc.isi))

				if err := terapkanPRD(t, home, "goose", ""); err != nil {
					t.Fatalf("applyToolSpecificConfig goose atas skalar blok berkutip (%s): %v", tc.nama, err)
				}
				konten := bacaBerkasUji(t, jalur)
				doc, err := uraiYAMLUtuh(konten)
				if err != nil {
					t.Fatalf("hasil sunting tidak terurai utuh: %v\n%s", err, konten)
				}
				if doc["catatan"] != tc.catatan {
					t.Errorf("isi skalar blok rusak: %q, mau %q\n%s", doc["catatan"], tc.catatan, konten)
				}
				if doc["GOOSE_MODEL"] != "model-baru" {
					t.Errorf("GOOSE_MODEL = %v, mau model-baru:\n%s", doc["GOOSE_MODEL"], konten)
				}
				if n := strings.Count(konten, "GOOSE_MODEL:"); n != 1 {
					t.Errorf("kemunculan 'GOOSE_MODEL:' = %d, mau 1:\n%s", n, konten)
				}
			})
		}
	})

	t.Run("aider", func(t *testing.T) {
		for _, tc := range []struct {
			nama    string
			isi     string
			catatan string
		}{
			{"kutip ganda awal baris isi",
				"catatan: |\n  \"kutip tak tertutup\n  lanjut\nmodel: lama\nlainnya: 1\n",
				"\"kutip tak tertutup\nlanjut\n"},
			{"kutip ganda sesudah kolon",
				"catatan: |\n  contoh: \"kutip tak tertutup\n  lanjut\nmodel: lama\nlainnya: 1\n",
				"contoh: \"kutip tak tertutup\nlanjut\n"},
		} {
			t.Run(tc.nama, func(t *testing.T) {
				home := t.TempDir()
				jalur := filepath.Join(home, ".aider.conf.yml")
				tulisBerkasPRD(t, jalur, []byte(tc.isi))

				if err := terapkanPRD(t, home, "aider", ""); err != nil {
					t.Fatalf("applyToolSpecificConfig aider atas skalar blok berkutip (%s): %v", tc.nama, err)
				}
				konten := bacaBerkasUji(t, jalur)
				doc, err := uraiYAMLUtuh(konten)
				if err != nil {
					t.Fatalf("hasil sunting tidak terurai utuh: %v\n%s", err, konten)
				}
				if doc["catatan"] != tc.catatan {
					t.Errorf("isi skalar blok rusak: %q, mau %q\n%s", doc["catatan"], tc.catatan, konten)
				}
				if doc["model"] != "openai/model-baru" {
					t.Errorf("model aider = %v, mau openai/model-baru:\n%s", doc["model"], konten)
				}
				if n := hitungHeaderKolomNolB560(konten, "model: openai/model-baru"); n != 1 {
					t.Errorf("baris terkelola 'model:' = %d, mau 1:\n%s", n, konten)
				}
			})
		}
	})

	t.Run("hermes", func(t *testing.T) {
		for _, tc := range []struct {
			nama    string
			isi     string
			catatan string
		}{
			{"kutip ganda sesudah kolon",
				"catatan: |\n  contoh: \"kutip tak tertutup\n  lanjut\nmodel:\n  default: lama\n",
				"contoh: \"kutip tak tertutup\nlanjut\n"},
			{"kutip ganda awal baris isi",
				"catatan: |\n  \"kutip tak tertutup\n  lanjut\nmodel:\n  default: lama\n",
				"\"kutip tak tertutup\nlanjut\n"},
			{"kutip ganda di skalar lipat",
				"catatan: >\n  \"kutip tak tertutup\n  lanjut\nmodel:\n  default: lama\n",
				"\"kutip tak tertutup lanjut\n"},
		} {
			t.Run(tc.nama, func(t *testing.T) {
				home := t.TempDir()
				jalur := filepath.Join(home, ".hermes", "config.yaml")
				tulisBerkasPRD(t, jalur, []byte(tc.isi))

				if err := terapkanPRD(t, home, "hermes", ""); err != nil {
					t.Fatalf("applyToolSpecificConfig hermes atas skalar blok berkutip (%s): %v", tc.nama, err)
				}
				konten := bacaBerkasUji(t, jalur)
				doc, err := uraiYAMLUtuh(konten)
				if err != nil {
					t.Fatalf("hasil sunting tidak terurai utuh: %v\n%s", err, konten)
				}
				if doc["catatan"] != tc.catatan {
					t.Errorf("isi skalar blok rusak: %q, mau %q\n%s", doc["catatan"], tc.catatan, konten)
				}
				blok, ok := doc["model"].(map[string]any)
				if !ok || blok["default"] != "model-baru" {
					t.Errorf("blok model tidak efektif: %v\n%s", doc["model"], konten)
				}
				if n := hitungHeaderKolomNolB560(konten, "model:"); n != 1 {
					t.Errorf("header blok 'model:' kolom-0 = %d, mau 1 (dua blok = degradasi B-560):\n%s", n, konten)
				}
			})
		}
	})
}

// TestB560KutipSesudahKataPolosTakMeracuni adalah kontrol negatif
// B-560: kutip sesudah kata polos di isi skalar blok BUKAN posisi
// pembuka, jadi tidak pernah membuka skalar semu — Configure
// berhasil sebelum maupun sesudah perbaikan, dan struktur asli
// tetap terbaca.
func TestB560KutipSesudahKataPolosTakMeracuni(t *testing.T) {
	for _, tool := range []string{"goose", "aider", "hermes"} {
		t.Run(tool, func(t *testing.T) {
			home := t.TempDir()
			jalur := jalurKonfigB559(home, tool)
			var isi string
			switch tool {
			case "goose":
				isi = "catatan: |\n  kata \"kutip tak tertutup\n  lanjut\nGOOSE_MODEL: lama\nlainnya: 1\n"
			case "aider":
				isi = "catatan: |\n  kata \"kutip tak tertutup\n  lanjut\nmodel: lama\nlainnya: 1\n"
			default:
				isi = "catatan: |\n  kata \"kutip tak tertutup\n  lanjut\nmodel:\n  default: lama\n"
			}
			tulisBerkasPRD(t, jalur, []byte(isi))

			if err := terapkanPRD(t, home, tool, ""); err != nil {
				t.Fatalf("applyToolSpecificConfig %s atas kutip sesudah kata polos: %v", tool, err)
			}
			konten := bacaBerkasUji(t, jalur)
			doc, err := uraiYAMLUtuh(konten)
			if err != nil {
				t.Fatalf("hasil sunting tidak terurai utuh: %v\n%s", err, konten)
			}
			if doc["catatan"] != "kata \"kutip tak tertutup\nlanjut\n" {
				t.Errorf("isi skalar blok rusak: %q\n%s", doc["catatan"], konten)
			}
			switch tool {
			case "goose":
				if doc["GOOSE_MODEL"] != "model-baru" {
					t.Errorf("GOOSE_MODEL = %v, mau model-baru:\n%s", doc["GOOSE_MODEL"], konten)
				}
			case "aider":
				if doc["model"] != "openai/model-baru" {
					t.Errorf("model aider = %v, mau openai/model-baru:\n%s", doc["model"], konten)
				}
			default:
				blok, ok := doc["model"].(map[string]any)
				if !ok || blok["default"] != "model-baru" {
					t.Errorf("blok model tidak efektif: %v\n%s", doc["model"], konten)
				}
				if n := hitungHeaderKolomNolB560(konten, "model:"); n != 1 {
					t.Errorf("header blok 'model:' kolom-0 = %d, mau 1:\n%s", n, konten)
				}
			}
		})
	}
}

// TestB560PesanGalatTakMenamaiDuplikasi mengunci sisi pesan
// B-560: bila hasil sunting memuat kunci TERKELOLA ganda dan
// berkas itu memuat kutip tak tertutup di isi skalar blok,
// galat verifikasi Goose/Aider WAJIB menamai kutip skalar blok
// itu sebagai sebab — bukan duplikasi ("already defined").
// Duplikasi kunci TAK-terkelola yang tulen tetap dilaporkan
// apa adanya, karena di sana duplikasi memang sebabnya.
func TestB560PesanGalatTakMenamaiDuplikasi(t *testing.T) {
	t.Run("terkelola ganda karena kutip blok", func(t *testing.T) {
		konten := "catatan: |\n  \"kutip tak tertutup\n  lanjut\nGOOSE_MODEL: lama\nGOOSE_MODEL: model-baru\n"
		pasangan := [][2]string{{"GOOSE_MODEL", "model-baru"}}
		err := verifikasiHasilYAMLTingkatAtas(konten, pasangan)
		if err == nil {
			t.Fatal("verifikasi menerima hasil berkunci terkelola ganda")
		}
		if strings.Contains(err.Error(), "already defined") {
			t.Errorf("pesan galat menamai duplikasi sebagai sebab: %v", err)
		}
		if !strings.Contains(err.Error(), "skalar blok") {
			t.Errorf("pesan galat tidak menamai kutip skalar blok: %v", err)
		}
		if !strings.Contains(err.Error(), "kutip tak tertutup") {
			t.Errorf("pesan galat tidak menunjuk baris kutipnya: %v", err)
		}
	})

	t.Run("tak-terkelola ganda tetap dilaporkan duplikasi", func(t *testing.T) {
		konten := "lainnya: 1\nlainnya: 2\nGOOSE_MODEL: model-baru\n"
		pasangan := [][2]string{{"GOOSE_MODEL", "model-baru"}}
		err := verifikasiHasilYAMLTingkatAtas(konten, pasangan)
		if err == nil {
			t.Fatal("verifikasi menerima hasil berkunci tak-terkelola ganda")
		}
		if !strings.Contains(err.Error(), "already defined") {
			t.Errorf("pesan galat atas duplikasi tulen tidak lagi menyebut duplikasinya: %v", err)
		}
	})
}

// TestB560PengenalanPembukaSkalarBlok mengunci enumerasi mekanis
// pengenal pembuka skalar blok (B-560): varian indikator
// chomping/indentasi/komentar, kunci terkutip, item urutan, dan
// bentuk-bentuk yang BUKAN pembuka (nilai polos berawalan '|',
// flow map, komentar) — plus perilaku pelacak atas baris kosong
// di dalam blok dan header blok palsu di dalam skalar terkutip.
func TestB560PengenalanPembukaSkalarBlok(t *testing.T) {
	for _, tc := range []struct {
		baris string
		mau   bool
		induk int
	}{
		{"catatan: |", true, 0},
		{"catatan: >", true, 0},
		{"catatan: |-", true, 0},
		{"catatan: |+", true, 0},
		{"catatan: |2", true, 0},
		{"catatan: |-2", true, 0},
		{"catatan: |2-", true, 0},
		{"catatan: >- # komentar", true, 0},
		{"catatan: | # komentar", true, 0},
		{"  catatan: |", true, 2},
		{`"catatan: x": |`, true, 0},
		{"- |", true, 0},
		{"  - |", true, 2},
		{"catatan: | ", true, 0},
		{"catatan: |teks", false, -1},
		{`catatan: "teks"`, false, -1},
		{"catatan: {a: b}", false, -1},
		{"model: {default: x}", false, -1},
		{"# komentar: |", false, -1},
		{"", false, -1},
		{"catatan:", false, -1},
		{"catatan: ||", false, -1},
		{"catatan: >2x", false, -1},
	} {
		induk, ok := awalSkalarBlokYAML(tc.baris)
		if ok != tc.mau || (tc.mau && induk != tc.induk) {
			t.Errorf("awalSkalarBlokYAML(%q) = (%d, %v), mau (%d, %v)", tc.baris, induk, ok, tc.induk, tc.mau)
		}
	}

	// Pelacak: baris kosong di dalam blok tidak mengakhirinya;
	// header blok palsu di dalam skalar terkutip tidak membuka blok.
	baris := []string{
		"catatan: |",
		"",
		"  \"kutip",
		"",
		"  lanjut",
		"GOOSE_MODEL: lama",
	}
	dalam := petaDalamSkalarYAML(baris)
	mau := []bool{false, true, true, true, true, false}
	for i, m := range mau {
		if dalam[i] != m {
			t.Errorf("dalam[%d] (%q) = %v, mau %v", i, baris[i], dalam[i], m)
		}
	}
	baris2 := []string{
		`k: "teks`,
		"catatan: |",
		`  lanjut"`,
		"GOOSE_MODEL: lama",
	}
	dalam2 := petaDalamSkalarYAML(baris2)
	mau2 := []bool{false, true, true, false}
	for i, m := range mau2 {
		if dalam2[i] != m {
			t.Errorf("dalam2[%d] (%q) = %v, mau %v", i, baris2[i], dalam2[i], m)
		}
	}
}
