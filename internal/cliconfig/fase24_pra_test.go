package cliconfig

// fase24_pra_test.go — pengunci Fase 24 PR-A (cliconfig, detektor
// pengkodean .env Hermes):
//
//	B-541+B-547 residu "pengenceran suara" detektor UTF-16 tanpa
//	      BOM: berkas pendek yang hanya mengumpulkan 3 suara NUL
//	      dan berkas yang nilainya didominasi karakter astral
//	      (emoji/CJK ekstensi — dalam byte mentah terbaca sebagai
//	      suara lawan atau pengencer kerapatan) kini tertangkap
//	      jalur proporsional berbasis klasifikasi hasil DEKODE.
//	B-548 UTF-32 tanpa BOM dikenali dari pola NUL per-4-byte dan
//	      ditolak jujur (sebelumnya lolos: penjaga NUL-ganda rapat
//	      detektor UTF-16 justru melepaskannya).
//	B-556 label orientasi pesan galat diambil dari klasifikasi
//	      dekode, bukan paritas suara mentah: berkas LE yang
//	      didominasi U+0100/astral tidak lagi berlabel BE; label
//	      netral "UTF-16" bila tak satu pun orientasi terbukti.
//
// Gerbang MUTLAK arah sebaliknya dipertahankan: korpus kontrol
// .env UTF-8 realistis yang beragam (komentar non-ASCII, nilai
// multibyte padat CJK/kana/Arab/emoji, NUL terisolasi campuran)
// TIDAK satu pun tertolak — salah-tolak tetap NOL.
//
// Semua test memanggil applyToolSpecificConfig — jalur Configure
// sungguhan — lewat pembantu terapkanPRD.

import (
	"path/filepath"
	"strings"
	"testing"
	"unicode/utf16"
)

// utf16SungguhanTanpaBOM mengkodekan teks sebagai UTF-16LE/BE
// TANPA BOM dengan encoder sungguhan (unicode/utf16) — berbeda
// dari utf16TanpaBOM (Fase 22) yang memotong rune astral menjadi
// satu unit; di sini pasangan surrogate terbentuk dengan benar.
func utf16SungguhanTanpaBOM(teks string, be bool) []byte {
	var out []byte
	for _, u := range utf16.Encode([]rune(teks)) {
		if be {
			out = append(out, byte(u>>8), byte(u))
		} else {
			out = append(out, byte(u), byte(u>>8))
		}
	}
	return out
}

// utf32TanpaBOM mengkodekan teks sebagai UTF-32LE/BE TANPA BOM.
func utf32TanpaBOM(teks string, be bool) []byte {
	var out []byte
	for _, r := range teks {
		if be {
			out = append(out, byte(r>>24), byte(r>>16), byte(r>>8), byte(r))
		} else {
			out = append(out, byte(r), byte(r>>8), byte(r>>16), byte(r>>24))
		}
	}
	return out
}

// TestB541EnvUTF16PendekTigaSuaraDitolak mengunci residu pertama
// B-541: berkas PENDEK yang bukti strukturalnya lengkap tetapi
// hanya mengumpulkan 3 suara NUL pada paritas menyala — di bawah
// bukti minimal 4 suara jalur klasik B-531, sehingga dulu lolos.
// Jalur proporsional menilai dekodenya: 3 karakter Latin
// struktural termasuk '=' dan nol cacat/anti.
func TestB541EnvUTF16PendekTigaSuaraDitolak(t *testing.T) {
	for _, tc := range []struct {
		nama string
		be   bool
		enc  string
	}{
		{"LE satu baris CJK", false, "UTF-16LE"},
		{"BE satu baris CJK", true, "UTF-16BE"},
	} {
		t.Run(tc.nama, func(t *testing.T) {
			periksaEnvUTF16DitolakB531(t, utf16SungguhanTanpaBOM("A=中\n", tc.be), tc.enc)
		})
	}
}

// TestB541B547EnvUTF16AstralDominanDitolak mengunci residu
// pengenceran oleh karakter ASTRAL (B-541 emoji, B-547 U+20000):
// dalam byte mentah, unit rendah surrogate menaruh NUL pada
// paritas lawan dan unit tingginya mengencerkan kerapatan,
// sehingga jalur klasik gugur ATAU menyala pada orientasi yang
// salah. Klasifikasi dekode menghitung pasangan surrogate yang
// sah sebagai SATU karakter astral netral.
func TestB541B547EnvUTF16AstralDominanDitolak(t *testing.T) {
	for _, tc := range []struct {
		nama string
		be   bool
		enc  string
		isi  string
	}{
		{"LE didominasi emoji U+1F600", false, "UTF-16LE",
			"A=😀😀😀😀😀😀😀😀\nB=😀😀😀😀😀😀😀😀\n"},
		{"BE didominasi emoji U+1F600", true, "UTF-16BE",
			"A=😀😀😀😀😀😀😀😀\nB=😀😀😀😀😀😀😀😀\n"},
		{"LE didominasi CJK ekstensi U+20000", false, "UTF-16LE",
			"A=𐀀𐀀𐀀𐀀𐀀𐀀𐀀𐀀\nB=𐀀𐀀𐀀𐀀𐀀𐀀𐀀𐀀\n"},
		{"LE nilai emoji panjang kunci normal", false, "UTF-16LE",
			"KUNCI_A=GANTI_awal\nKUNCI_B=😀😀😀😀😀😀😀😀😀😀😀😀😀😀😀😀😀😀😀😀\n"},
	} {
		t.Run(tc.nama, func(t *testing.T) {
			periksaEnvUTF16DitolakB531(t, utf16SungguhanTanpaBOM(tc.isi, tc.be), tc.enc)
		})
	}
}

// TestB556LabelOrientasiDariDekode mengunci B-556: berkas LE yang
// didominasi U+0100 (Ā) — byte mentahnya menaruh NUL pada paritas
// GENAP sehingga suara klasik menamai BE — kini berlabel
// orientasi sebenarnya dari klasifikasi dekode (dekode BE atasnya
// memuat karakter kontrol cacat; dekode LE masuk akal). Cermin BE
// berlaku sebaliknya.
func TestB556LabelOrientasiDariDekode(t *testing.T) {
	for _, tc := range []struct {
		nama string
		be   bool
		enc  string
	}{
		{"LE didominasi U+0100 berlabel LE", false, "UTF-16LE"},
		{"BE didominasi U+0100 berlabel BE", true, "UTF-16BE"},
	} {
		t.Run(tc.nama, func(t *testing.T) {
			isi := "A=ĀĀĀĀĀĀĀĀ\nB=ĀĀĀĀĀĀĀĀ\n"
			periksaEnvUTF16DitolakB531(t, utf16SungguhanTanpaBOM(isi, tc.be), tc.enc)
		})
	}
}

// TestB556LabelNetralBilaOrientasiTakTerbukti mengunci cabang
// netral B-556: suara klasik menyala (NUL rapat satu paritas dari
// unit U+0085 berurutan) tetapi dekode kedua orientasi tidak
// masuk akal sebagai teks .env (LE memuat cacat kontrol dan tanpa
// '='; BE tanpa Latin/'=') — vonis tetap penolakan, labelnya
// netral "UTF-16", bukan karangan orientasi.
func TestB556LabelNetralBilaOrientasiTakTerbukti(t *testing.T) {
	isi := "AB" + strings.Repeat("\u0085", 8) + "\n"
	periksaEnvUTF16DitolakB531(t, utf16SungguhanTanpaBOM(isi, false), "UTF-16")
}

// TestB548EnvUTF32TanpaBOMDitolak mengunci B-548 dua arah:
// .env Hermes UTF-32LE/BE tanpa BOM ditolak jujur dengan pesan
// menamai pengkodeannya — sebelumnya Configure melaporkan sukses
// atas berkas yang tidak bisa dibaca Hermes.
func TestB548EnvUTF32TanpaBOMDitolak(t *testing.T) {
	isi := "VARIABEL_LAIN=GANTI_nilai-b548\nVARIABEL_KEDUA=GANTI_juga-b548\n"
	for _, tc := range []struct {
		nama string
		be   bool
		enc  string
	}{
		{"UTF-32LE tanpa BOM", false, "UTF-32LE"},
		{"UTF-32BE tanpa BOM", true, "UTF-32BE"},
	} {
		t.Run(tc.nama, func(t *testing.T) {
			periksaEnvUTF16DitolakB531(t, utf32TanpaBOM(isi, tc.be), tc.enc)
		})
	}
}

// TestB541KorpusKontrolUTF8TidakTertolak adalah gerbang mutlak
// arah sebaliknya untuk seluruh PR-A: korpus .env UTF-8 realistis
// yang BERAGAM — komentar non-ASCII, nilai multibyte padat dari
// beberapa aksara, emoji UTF-8, dan NUL terisolasi campuran —
// tidak satu pun tertolak oleh jalur proporsional maupun detektor
// UTF-32; Configure berhasil dan kunci terkelola tertulis.
func TestB541KorpusKontrolUTF8TidakTertolak(t *testing.T) {
	for _, tc := range []struct {
		nama string
		isi  string
	}{
		{"komentar Latin-1 dan tanda hubung panjang",
			"# komentar éè ê — catatan rilis\nKUNCI=GANTI_nilai-b541\n"},
		{"nilai CJK padat",
			"CATATAN=中文中文中文中文中文中文中文中文\nKUNCI=GANTI_nilai-b541\n"},
		{"nilai kana padat",
			"名前=テストテストテストテスト\nKUNCI=GANTI_nilai-b541\n"},
		{"nilai Arab padat",
			"SAPA=مرحبا بالعالم مرحبا بالعالم\nKUNCI=GANTI_nilai-b541\n"},
		{"nilai emoji UTF-8 padat",
			"MOOD=😀😀😀😀😀😀😀😀😀😀\nKUNCI=GANTI_nilai-b541\n"},
		{"campuran non-ASCII dan NUL terisolasi",
			"LAIN=ü\x00x\nCATATAN=naïve façade — déjà vu\nKUNCI=GANTI_nilai-b541\n"},
		{"nilai mata uang dan simbol",
			"HARGA=€100 £50 ¥1000 ✓✗\nKUNCI=GANTI_nilai-b541\n"},
	} {
		t.Run(tc.nama, func(t *testing.T) {
			home := t.TempDir()
			envPath := filepath.Join(home, ".hermes", ".env")
			tulisBerkasPRD(t, envPath, []byte(tc.isi))

			if err := terapkanPRD(t, home, "hermes", "GANTI_kunci-prd"); err != nil {
				t.Fatalf("applyToolSpecificConfig hermes menolak .env UTF-8 sah (%s): %v", tc.nama, err)
			}
			konten := bacaBerkasUji(t, envPath)
			if !strings.Contains(konten, "OPENAI_API_KEY="+"GANTI_kunci-prd") {
				t.Errorf("kunci terkelola tidak tertulis untuk kontrol %s:\n%q", tc.nama, konten)
			}
		})
	}
}
