package cliconfig

// fase25_prc_test.go — pengunci Fase 25 PR-C (cliconfig, detektor
// pengkodean .env Hermes):
//
//	B-562 UTF-32 tanpa BOM yang fraksi astralnya di atas 1/4
//	      lolos kedua detektor: karakter astral menyumbang byte
//	      pendamping bukan-NUL sehingga residu pendamping jatuh
//	      di bawah ambang 3/4, dan penjaga NUL-ganda detektor
//	      UTF-16 justru melepaskannya. Kini residu pendamping
//	      UTF-32 menghitung grup astral sebagai "terjelaskan"
//	      (skalar astral sah), bukan sebagai bukti melawan.
//	B-563 Satu unit kelas Cacat memveto jalur proporsional
//	      detektor UTF-16: satu U+0085 (NEL) atau satu
//	      surrogate yatim di EOF menggugurkan vonis atas berkas
//	      yang selebihnya teks .env sah. Kini Cacat dinilai
//	      proporsional seperti kelas Anti (Fase 23): Latin
//	      harus mendominasi >2:1 atas Cacat, bukan cacat==0.
//	B-564 Sambungan byte dua orientasi (LE+BE) seimbang lolos
//	      semua jalur — KEPUTUSAN INDUK: BATAS SAH, TANPA
//	      PERUBAHAN PERILAKU. Butir ini ditutup
//	      SELESAI-TERCATAT lewat penyempitan klaim tertulis
//	      detektor (komentar KUALIFIKASI AMBANG); TIDAK ADA
//	      test perilaku untuk campuran-orientasi karena
//	      perilakunya sengaja tidak berubah.
//
// Semua test pengunci memanggil applyToolSpecificConfig — jalur
// Configure sungguhan — lewat pembantu terapkanPRD, kecuali
// sapuan mekanis yang memanggil fungsi detektor langsung untuk
// menutup ruang bentuk (orientasi × fraksi astral × kelas Cacat
// × panjang berkas).

import (
	"strings"
	"testing"
)

// utf16DariUnitPRC mengkodekan unit UTF-16 mentah (boleh memuat
// surrogate yatim) sebagai UTF-16LE/BE tanpa BOM.
func utf16DariUnitPRC(units []uint16, be bool) []byte {
	var out []byte
	for _, u := range units {
		if be {
			out = append(out, byte(u>>8), byte(u))
		} else {
			out = append(out, byte(u), byte(u>>8))
		}
	}
	return out
}

// unitTeksPRC mengubah teks BMP menjadi unit UTF-16-nya.
func unitTeksPRC(teks string) []uint16 {
	var out []uint16
	for _, r := range teks {
		out = append(out, uint16(r))
	}
	return out
}

// TestB562EnvUTF32AstralDominanDitolak mengunci B-562: .env
// UTF-32LE/BE tanpa BOM yang didominasi karakter astral (emoji
// U+1F600 dan CJK ekstensi U+20000) kini ditolak jujur dengan
// label orientasi yang benar — sebelumnya Configure melaporkan
// sukses atas berkas yang tidak bisa dibaca Hermes
// (UnicodeDecodeError total, terbukti dengan pustaka sungguhan).
func TestB562EnvUTF32AstralDominanDitolak(t *testing.T) {
	for _, tc := range []struct {
		nama   string
		be     bool
		enc    string
		isi    string
		astral string
	}{
		{"LE emoji 8/14 grup astral", false, "UTF-32LE",
			"A=😀😀😀😀\nB=😀😀😀😀\n", "8/14"},
		{"BE emoji 8/14 grup astral", true, "UTF-32BE",
			"A=😀😀😀😀\nB=😀😀😀😀\n", "8/14"},
		{"LE emoji 8/11 grup astral", false, "UTF-32LE",
			"A=😀😀😀😀😀😀😀😀\n", "8/11"},
		{"BE emoji 8/11 grup astral", true, "UTF-32BE",
			"A=😀😀😀😀😀😀😀😀\n", "8/11"},
		{"LE CJK ekstensi U+20000 8/14 grup astral", false, "UTF-32LE",
			"A=𐀀𐀀𐀀𐀀\nB=𐀀𐀀𐀀𐀀\n", "8/14"},
		{"LE emoji 3/10 grup astral (di atas 1/4)", false, "UTF-32LE",
			"A=😀😀😀\nB=1\n", "3/10"},
		{"LE emoji 2/6 grup astral (berkas pendek)", false, "UTF-32LE",
			"A=😀😀\n", "2/6"},
		{"LE emoji 12/15 grup astral", false, "UTF-32LE",
			"K=😀😀😀😀😀😀😀😀😀😀😀😀\n", "12/15"},
	} {
		t.Run(tc.nama, func(t *testing.T) {
			periksaEnvUTF16DitolakB531(t, utf32TanpaBOM(tc.isi, tc.be), tc.enc)
		})
	}
}

// TestB562EnvUTF32BatasSeperempatTetapDitolak mengunci batas
// ambang lama: fraksi astral TEPAT 1/4 sudah tertangkap sebelum
// perbaikan (residu pendamping tepat 3/4) dan harus tetap
// tertangkap sesudah penghitungan astral.
func TestB562EnvUTF32BatasSeperempatTetapDitolak(t *testing.T) {
	periksaEnvUTF16DitolakB531(t, utf32TanpaBOM("A=😀😀\nB=\n", false), "UTF-32LE")
}

// TestB563SatuCacatTidakLagiMemveto mengunci B-563: satu unit
// Cacat (NEL U+0085, kontrol U+0001, surrogate yatim di EOF
// maupun di tengah) tidak lagi memveto jalur proporsional —
// berkas UTF-16LE/BE sah yang selebihnya teks .env kini
// ditolak jujur. Bentuk pembanding tanpa cacatnya sudah
// tertangkap sebelum perbaikan; bentuk berkacatnya lolos.
func TestB563SatuCacatTidakLagiMemveto(t *testing.T) {
	for _, tc := range []struct {
		nama string
		be   bool
		enc  string
		data []byte
	}{
		{"LE satu NEL + 32 CJK", false, "UTF-16LE",
			utf16SungguhanTanpaBOM("A="+strings.Repeat("中", 32)+"\u0085\n", false)},
		{"BE satu NEL + 32 CJK", true, "UTF-16BE",
			utf16SungguhanTanpaBOM("A="+strings.Repeat("中", 32)+"\u0085\n", true)},
		{"LE satu SOH + 32 CJK", false, "UTF-16LE",
			utf16SungguhanTanpaBOM("A="+strings.Repeat("中", 32)+"\x01\n", false)},
		{"LE surrogate tinggi yatim di EOF + 32 CJK", false, "UTF-16LE",
			utf16DariUnitPRC(append(append([]uint16{}, unitTeksPRC("A="+strings.Repeat("中", 32)+"\n")...), 0xD800), false)},
		{"BE surrogate tinggi yatim di EOF + 32 CJK", true, "UTF-16BE",
			utf16DariUnitPRC(append(append([]uint16{}, unitTeksPRC("A="+strings.Repeat("中", 32)+"\n")...), 0xD800), true)},
		{"LE surrogate rendah yatim di EOF + 32 CJK", false, "UTF-16LE",
			utf16DariUnitPRC(append(append([]uint16{}, unitTeksPRC("A="+strings.Repeat("中", 32)+"\n")...), 0xDC00), false)},
		{"LE surrogate tinggi yatim di tengah + 32 CJK", false, "UTF-16LE",
			utf16DariUnitPRC(append(append(append([]uint16{}, unitTeksPRC("A="+strings.Repeat("中", 32))...), 0xD800), unitTeksPRC("B=1\n")...), false)},
		{"LE dua NEL, Latin 5 + 32 CJK", false, "UTF-16LE",
			utf16SungguhanTanpaBOM("AB=C"+strings.Repeat("中", 32)+"\u0085\u0085\n", false)},
	} {
		t.Run(tc.nama, func(t *testing.T) {
			periksaEnvUTF16DitolakB531(t, tc.data, tc.enc)
		})
	}
}

// TestB563LabelOrientasiTetapBenar mengunci bahwa sesudah Cacat
// proporsional, berkas Latin-padat dengan satu surrogate yatim
// yang dulu berlabel netral (jalur klasik + dekode tak-masuk-akal
// karena veto) kini berlabel orientasi sebenarnya — vonisnya
// tetap penolakan, hanya namanya yang tidak lagi netral.
func TestB563LabelOrientasiTetapBenar(t *testing.T) {
	unit := append(append([]uint16{}, unitTeksPRC("KUNCI=nilai\n")...), 0xD800)
	periksaEnvUTF16DitolakB531(t, utf16DariUnitPRC(unit, false), "UTF-16LE")
}

// TestB556LabelNetralDelapanCacatTetapNetral adalah penjaga arah
// sebaliknya untuk B-563: dekode yang cacatnya mencapai separuh
// Latinnya (8 NEL atas 3 Latin) tetap tidak masuk akal, sehingga
// label netral B-556 dipertahankan.
func TestB556LabelNetralDelapanCacatTetapNetral(t *testing.T) {
	isi := "AB" + strings.Repeat("\u0085", 8) + "\n"
	periksaEnvUTF16DitolakB531(t, utf16SungguhanTanpaBOM(isi, false), "UTF-16")
}

// latinStrPRC membangun tepat n unit Latin (termasuk '=' bila
// n >= 2 dan '\n' bila n >= 3) — dipakai sapuan mekanis.
func latinStrPRC(n int) string {
	switch {
	case n <= 0:
		return ""
	case n == 1:
		return "A"
	case n == 2:
		return "A="
	default:
		return "A=" + strings.Repeat("x", n-3) + "\n"
	}
}

// teksUTF32Fraksi membangun teks sejumlah total grup UTF-32
// dengan astral di antaranya karakter astral — dipakai sapuan
// mekanis. Bagian Latinnya latinStrPRC(total-astral).
func teksUTF32Fraksi(total, astral int) string {
	if astral >= total {
		return strings.Repeat("😀", total)
	}
	return latinStrPRC(total-astral) + strings.Repeat("😀", astral)
}

// teksUTF16Cacat membangun teks UTF-16 sejumlah total unit
// dengan latin unit Latin (termasuk '=' dan '\n'), cacat unit
// Cacat (NEL), dan sisanya CJK netral — dipakai sapuan mekanis.
func teksUTF16Cacat(latin, cacat, cjk int) string {
	return latinStrPRC(latin) + strings.Repeat("中", cjk) + strings.Repeat("\u0085", cacat)
}

// TestFase25PRCSapuanMekanisDetektor menyapu ruang bentuk
// detektor yang diubah PR ini — orientasi × fraksi astral ×
// kelas Cacat × panjang berkas — langsung pada fungsi detektor,
// dan menegaskan vonis tiap sel. Sel-sel di sini (CJK/NEL/
// Latin) tetap tak memenuhi kedua sisi bersamaan — tetapi itu
// sifat SEL ini, bukan ketaktercapaian cabang: B-568 membuktikan
// cabang "kedua orientasi layak" TERJANGKAU lewat pasangan
// Latin-1 (fase26_pra_test.go). Tabel sel tercetak lewat -v;
// ringkasannya juga tertulis di badan PR.
func TestFase25PRCSapuanMekanisDetektor(t *testing.T) {
	// --- UTF-32: fraksi astral × orientasi × panjang ---
	for _, total := range []int{4, 7, 8, 12, 14, 40, 200} {
		for _, astral := range []int{0, 1, 2, 3, 4, 6, 7, 8, 9, 12, 14} {
			if astral > total {
				continue
			}
			for _, be := range []bool{false, true} {
				isi := teksUTF32Fraksi(total, astral)
				data := utf32TanpaBOM(isi, be)
				want := "UTF-32LE"
				if be {
					want = "UTF-32BE"
				}
				got32 := deteksiUTF32TanpaBOM(data)
				got16 := deteksiUTF16TanpaBOM(data)
				t.Logf("UTF-32 total=%3d astral=%2d be=%v -> det32=%q det16=%q",
					total, astral, be, got32, got16)
				if got32 != want {
					t.Errorf("UTF-32 total=%d astral=%d be=%v: det32=%q, mau %q", total, astral, be, got32, want)
				}
				if got16 != "" {
					t.Errorf("UTF-32 total=%d astral=%d be=%v: det16=%q, mau \"\" (salah label)", total, astral, be, got16)
				}
			}
		}
	}
	// --- UTF-16: kelas Cacat × Latin × panjang (CJK netral) ---
	for _, tc := range []struct {
		latin, cacat, cjk int
		tolak             bool // layak proporsional menuntut latin > 2*cacat
	}{
		{3, 0, 32, true},
		{3, 1, 32, true},  // B-563a: satu NEL tidak lagi memveto
		{3, 2, 32, false}, // batas proporsional: 3 tidak > 4
		{3, 3, 32, false},
		{6, 0, 32, true},
		{6, 1, 32, true},
		{6, 2, 32, true},  // 6 > 4
		{6, 3, 32, false}, // batas proporsional: 6 tidak > 6
		{3, 1, 0, true},   // berkas pendek: jalur klasik ikut menyala
		{3, 1, 8, true},
		{3, 1, 100, true},
		{6, 2, 100, true},
		{2, 0, 32, false}, // lantai bukti: Latin < 3
		{2, 1, 32, false},
	} {
		for _, be := range []bool{false, true} {
			isi := teksUTF16Cacat(tc.latin, tc.cacat, tc.cjk)
			data := utf16SungguhanTanpaBOM(isi, be)
			le := nilaiOrientasiUTF16(data, len(data)/2, true)
			beP := nilaiOrientasiUTF16(data, len(data)/2, false)
			got := deteksiUTF16TanpaBOM(data)
			t.Logf("UTF-16 latin=%d cacat=%d cjk=%3d be=%v -> layakLE=%v layakBE=%v det16=%q (tolak=%v)",
				tc.latin, tc.cacat, tc.cjk, be, le.layak(), beP.layak(), got, tc.tolak)
			if le.layak() && beP.layak() {
				t.Errorf("UTF-16 latin=%d cacat=%d cjk=%d be=%v: kedua orientasi layak — mustahil untuk sel ini (tanpa pasangan Latin-1 D8–DB/DC–DF, B-568); bila ini tercapai, sapuan atau penilai berubah", tc.latin, tc.cacat, tc.cjk, be)
			}
			// Orientasi benar harus layak tepat bila tolak;
			// orientasi lawan tidak pernah layak.
			benar := le
			if be {
				benar = beP
			}
			if benar.layak() != tc.tolak {
				t.Errorf("UTF-16 latin=%d cacat=%d cjk=%d be=%v: layak orientasi benar=%v, mau %v", tc.latin, tc.cacat, tc.cjk, be, benar.layak(), tc.tolak)
			}
			if tc.tolak && got == "" {
				t.Errorf("UTF-16 latin=%d cacat=%d cjk=%d be=%v: lolos, mau ditolak", tc.latin, tc.cacat, tc.cjk, be)
			}
			if !tc.tolak && got != "" {
				t.Errorf("UTF-16 latin=%d cacat=%d cjk=%d be=%v: ditolak (%q), mau lolos (batas proporsional/lantai)", tc.latin, tc.cacat, tc.cjk, be, got)
			}
		}
	}
	// --- Kontrol UTF-8 di tingkat detektor (harus lolos) ---
	// (B-564 TIDAK diuji perilakunya di sini: sambungan
	// LE+BE sengaja tidak berubah dan ditutup lewat redaksi.)
	for _, isi := range []string{
		"# komentar éè ê — catatan rilis\nKUNCI=GANTI_nilai-b541\n",
		"CATATAN=𐀀𐀀𐀀𐀀𐀀𐀀𐀀𐀀\nKUNCI=GANTI_nilai-b541\n",
		"MOOD=😀😀😀😀😀😀😀😀😀😀\nKUNCI=GANTI_nilai-b541\n",
		"LAIN=ü\x00x\nCATATAN=naïve façade — déjà vu\nKUNCI=GANTI_nilai-b541\n",
	} {
		data := []byte(isi)
		if got := deteksiUTF16TanpaBOM(data); got != "" {
			t.Errorf("kontrol UTF-8 ditolak det16=%q: %q", got, isi)
		}
		if got := deteksiUTF32TanpaBOM(data); got != "" {
			t.Errorf("kontrol UTF-8 ditolak det32=%q: %q", got, isi)
		}
	}
}
