package cliconfig

// fase26_pra_test.go — pengunci Fase 26 PR-A (cliconfig, detektor
// pengkodean .env Hermes, keluarga B-568/B-569/B-570):
//
//	B-568 Cabang "kedua orientasi layak → netral" TERJANGKAU
//	      (bukan cabang mati): pasangan surrogate Latin-1
//	      (hi U+00D8–U+00DB, lo U+00DC–U+00DF) di satu
//	      orientasi membentuk pasangan surrogate sah di
//	      orientasi lawan, sehingga kedua sisi layak
//	      bersamaan. Perilaku TIDAK berubah —
//	      yang diperbaiki hanya komentar (klaim "cabang mati"
//	      + bukti aritmetikanya gugur) dan sapuan mekanis
//	      diperluas dengan sel pasangan Latin-1. Test
//	      pengunci: label netral "UTF-16" atas konstruksi
//	      skala 1/2 — bila cabang itu dihapus berdasar klaim
//	      lama, labelnya jatuh ke LE/BE dan test ini gagal
//	      (dibuktikan dengan mutasi, bukan stash).
//	B-569 .env UTF-32 astral-dominan salah label "UTF-16"
//	      karena Configure memeriksa det16 DULU: jalur klasik
//	      det16 menyala iff A + z1 > 2·z0 (A = grup astral,
//	      z0/z1 = astral berbita-rendah/-kedua 0). Kini det32
//	      diperiksa SEBELUM det16 di Configure.
//	B-570 Satu byte jangkar rusak (0x00→0x01) mematikan det32
//	      sepenuhnya (jangkar UNANIM) → Configure sukses-bohong
//	      atas .env UTF-32. Kini jangkar menoleransi tepat
//	      satu grup menyimpang (nol[3] >= g-1 LE /
//	      nol[0] >= g-1 BE); dua grup menyimpang tetap lolos
//	      (batas tertulis, dikunci test).
//
// Semua test pengunci memanggil applyToolSpecificConfig — jalur
// Configure sungguhan — lewat pembantu terapkanPRD, kecuali
// sapuan mekanis yang memanggil fungsi detektor langsung untuk
// menutup ruang bentuk (orientasi × fraksi astral × posisi
// jangkar rusak × label B-556).

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

// rusakBytePR26 menyalin data dan merusak satu byte pada offset
// yang diminta (model byte jangkar UTF-32 yang rusak 0x00→0x01).
func rusakBytePR26(data []byte, off int, val byte) []byte {
	out := append([]byte{}, data...)
	out[off] = val
	return out
}

// periksaEnvDitolakLabelTepatPR26 menegaskan penolakan jujur
// jalur Hermes dengan label pengkodean TEPAT (bukan sekadar
// substring): pesan galat harus memuat "berkode <enc> tanpa BOM"
// — membedakan label netral "UTF-16" dari "UTF-16LE"/"UTF-16BE".
func periksaEnvDitolakLabelTepatPR26(t *testing.T, asli []byte, enc string) {
	t.Helper()
	home := t.TempDir()
	envPath := filepath.Join(home, ".hermes", ".env")
	tulisBerkasPRD(t, envPath, asli)

	err := terapkanPRD(t, home, "hermes", "GANTI_kunci-prd")
	if err == nil {
		t.Fatalf("applyToolSpecificConfig melaporkan sukses atas .env %s", enc)
	}
	if !strings.Contains(err.Error(), "berkode "+enc+" tanpa BOM") {
		t.Errorf("pesan galat tidak menamai pengkodean %q secara tepat: %v", enc, err)
	}
	if _, statErr := os.Lstat(filepath.Join(home, ".hermes", "config.yaml")); !os.IsNotExist(statErr) {
		t.Errorf("config.yaml ikut tertulis sesudah penolakan (stat err=%v)", statErr)
	}
	mentah, _ := os.ReadFile(envPath)
	if string(mentah) != string(asli) {
		t.Error(".env berubah sesudah penolakan jujur")
	}
}

// konstruksiDuaOrientasiPR26 adalah konstruksi B-568: 6 pasang
// byte yang LE dan BE-nya sama-sama {latin:3 anti:1 cacat:0}
// layak — pasangan surrogate Latin-1 U+00D8–U+00DF (parameter
// hi/lo) membentuk pasangan surrogate sah di orientasi lawan.
func konstruksiDuaOrientasiPR26(hi, lo byte) []byte {
	return []byte{0x3D, 0x00, hi, 0x00, lo, 0x00, 0x00, 0x3D, 0x00, hi, 0x00, lo}
}

// TestB568CabangKeduaOrientasiLayakTerjangkau mengunci B-568:
// cabang pertama switch detektor ("kedua orientasi layak" →
// "UTF-16") TERJANGKAU dan labelnya netral. Perilaku ini sudah
// benar di base (test ini LULUS di base); yang dibuktikan di
// sini adalah labelnya — mutasi "hapus cabang pertama" (klaim
// cabang-mati yang gugur) menjatuhkan label ke LE/BE dan
// menggagalkan test ini.
func TestB568CabangKeduaOrientasiLayakTerjangkau(t *testing.T) {
	for _, tc := range []struct {
		nama string
		data []byte
	}{
		{"skala 1: 6 pasang, latin 3/3 (D8/DC)",
			konstruksiDuaOrientasiPR26(0xD8, 0xDC)},
		{"skala 2: konstruksi ganda, latin 6/6",
			append(append([]byte{}, konstruksiDuaOrientasiPR26(0xD8, 0xDC)...), konstruksiDuaOrientasiPR26(0xD8, 0xDC)...)},
		{"varian D9/DD, latin 3/3",
			konstruksiDuaOrientasiPR26(0xD9, 0xDD)},
		{"varian DB/DF, latin 3/3 (ujung rentang: hi D8–DB, lo DC–DF)",
			konstruksiDuaOrientasiPR26(0xDB, 0xDF)},
	} {
		t.Run(tc.nama, func(t *testing.T) {
			le := nilaiOrientasiUTF16(tc.data, len(tc.data)/2, true)
			be := nilaiOrientasiUTF16(tc.data, len(tc.data)/2, false)
			if !le.layak() || !be.layak() {
				t.Fatalf("konstruksi tidak layak di kedua sisi: LE=%+v BE=%+v", le, be)
			}
			periksaEnvDitolakLabelTepatPR26(t, tc.data, "UTF-16")
		})
	}
}

// TestB568SapuanPasanganLatin1 menyapu sel pasangan Latin-1
// Fase 25 yang tak pernah disapu: hi U+00D8–U+00DB (yang di
// orientasi lawan terbaca surrogate TINGGI U+D800–U+DB00) × lo
// U+00DC–U+00DF (surrogate RENDAH U+DC00–U+DF00) — 16 sel;
// setiap sel layak di KEDUA orientasi dan berlabel netral
// "UTF-16". (Hi di luar D8–DB — mis. DF — bukan surrogate
// tinggi sehingga pasangannya cacat; itu batas rentang,
// bukan sel sapuan.)
func TestB568SapuanPasanganLatin1(t *testing.T) {
	for hi := 0xD8; hi <= 0xDB; hi++ {
		for lo := 0xDC; lo <= 0xDF; lo++ {
			data := konstruksiDuaOrientasiPR26(byte(hi), byte(lo))
			le := nilaiOrientasiUTF16(data, len(data)/2, true)
			be := nilaiOrientasiUTF16(data, len(data)/2, false)
			t.Logf("Latin-1 hi=U+%04X lo=U+%04X -> LE=%+v BE=%+v det16=%q", hi, lo, le, be, deteksiUTF16TanpaBOM(data))
			if !le.layak() || !be.layak() {
				t.Errorf("hi=U+%04X lo=U+%04X: tidak layak di kedua sisi (LE=%+v BE=%+v)", hi, lo, le, be)
			}
			if got := deteksiUTF16TanpaBOM(data); got != "UTF-16" {
				t.Errorf("hi=U+%04X lo=U+%04X: det16=%q, mau \"UTF-16\" netral", hi, lo, got)
			}
		}
	}
}

// TestB569EnvUTF32AstralBitaRendahDitolakBenar mengunci B-569:
// .env UTF-32LE/BE astral-dominan yang byte-rendah astralnya
// ≠ 0 kini ditolak dengan label UTF-32 yang benar — di base
// Configure menolaknya berlabel "UTF-16" (netral, salah).
// 10 bentuk × LE+BE: 4 emoji berbita-rendah ≠ 0, 3 skalar
// (U+10001, U+20001, U+10FFFF), 3 campuran z0 (5/4/3 dari 8
// astral — z0=5/4 adalah kontrol yang sudah benar di base).
func TestB569EnvUTF32AstralBitaRendahDitolakBenar(t *testing.T) {
	for _, tc := range []struct {
		nama string
		be   bool
		enc  string
		isi  string
	}{
		{"LE emoji U+1F601 8/14", false, "UTF-32LE", "A=😁😁😁😁\nB=😁😁😁😁\n"},
		{"BE emoji U+1F601 8/14", true, "UTF-32BE", "A=😁😁😁😁\nB=😁😁😁😁\n"},
		{"LE emoji U+1F602 8/14", false, "UTF-32LE", "A=😂😂😂😂\nB=😂😂😂😂\n"},
		{"BE emoji U+1F602 8/14", true, "UTF-32BE", "A=😂😂😂😂\nB=😂😂😂😂\n"},
		{"LE emoji U+1F603 8/14", false, "UTF-32LE", "A=😃😃😃😃\nB=😃😃😃😃\n"},
		{"BE emoji U+1F603 8/14", true, "UTF-32BE", "A=😃😃😃😃\nB=😃😃😃😃\n"},
		{"LE emoji U+1F60D 8/14", false, "UTF-32LE", "A=😍😍😍😍\nB=😍😍😍😍\n"},
		{"BE emoji U+1F60D 8/14", true, "UTF-32BE", "A=😍😍😍😍\nB=😍😍😍😍\n"},
		{"LE U+10001 8/14", false, "UTF-32LE", "A=\U00010001\U00010001\U00010001\U00010001\nB=\U00010001\U00010001\U00010001\U00010001\n"},
		{"BE U+10001 8/14", true, "UTF-32BE", "A=\U00010001\U00010001\U00010001\U00010001\nB=\U00010001\U00010001\U00010001\U00010001\n"},
		{"LE U+20001 8/14", false, "UTF-32LE", "A=\U00020001\U00020001\U00020001\U00020001\nB=\U00020001\U00020001\U00020001\U00020001\n"},
		{"BE U+20001 8/14", true, "UTF-32BE", "A=\U00020001\U00020001\U00020001\U00020001\nB=\U00020001\U00020001\U00020001\U00020001\n"},
		{"LE U+10FFFF 8/14", false, "UTF-32LE", "A=\U0010FFFF\U0010FFFF\U0010FFFF\U0010FFFF\nB=\U0010FFFF\U0010FFFF\U0010FFFF\U0010FFFF\n"},
		{"BE U+10FFFF 8/14", true, "UTF-32BE", "A=\U0010FFFF\U0010FFFF\U0010FFFF\U0010FFFF\nB=\U0010FFFF\U0010FFFF\U0010FFFF\U0010FFFF\n"},
		{"LE campuran z0=5 dari 8 astral (kontrol, sudah benar)", false, "UTF-32LE", "A=😀😀😀😀😀😁😁😁\nB=1\n"},
		{"BE campuran z0=5 dari 8 astral (kontrol, sudah benar)", true, "UTF-32BE", "A=😀😀😀😀😀😁😁😁\nB=1\n"},
		{"LE campuran z0=4 dari 8 astral (kontrol, sudah benar)", false, "UTF-32LE", "A=😀😀😀😀😁😁😁😁\nB=1\n"},
		{"BE campuran z0=4 dari 8 astral (kontrol, sudah benar)", true, "UTF-32BE", "A=😀😀😀😀😁😁😁😁\nB=1\n"},
		{"LE campuran z0=3 dari 8 astral (A+z1=8 > 2·z0=6)", false, "UTF-32LE", "A=😀😀😀😁😁\nB=😁😁😁\n"},
		{"BE campuran z0=3 dari 8 astral (A+z1=8 > 2·z0=6)", true, "UTF-32BE", "A=😀😀😀😁😁\nB=😁😁😁\n"},
	} {
		t.Run(tc.nama, func(t *testing.T) {
			periksaEnvDitolakLabelTepatPR26(t, utf32TanpaBOM(tc.isi, tc.be), tc.enc)
		})
	}
}

// TestB570JangkarRusakSatuByteTetapDitolak mengunci B-570: .env
// UTF-32LE/BE yang SATU byte jangkarnya rusak (0x00→0x01) kini
// tetap ditolak jujur dengan label yang benar — di base
// Configure melaporkan SUKSES atas berkas yang tidak bisa
// dibaca Hermes (UnicodeDecodeError total, terbukti oracle).
// Grup yang dirusak: 'B' (BMP), astral, dan fraksi rendah.
func TestB570JangkarRusakSatuByteTetapDitolak(t *testing.T) {
	for _, tc := range []struct {
		nama string
		be   bool
		enc  string
		isi  string
		grup int // indeks grup 4-byte yang jangkarnya dirusak
	}{
		{"LE 8/14 😀, jangkar grup 'B' rusak", false, "UTF-32LE", "A=😀😀😀😀\nB=😀😀😀😀\n", 7},
		{"LE 8/14 😀, jangkar grup ASTRAL rusak", false, "UTF-32LE", "A=😀😀😀😀\nB=😀😀😀😀\n", 2},
		{"BE 8/14 😀, jangkar grup 'B' rusak", true, "UTF-32BE", "A=😀😀😀😀\nB=😀😀😀😀\n", 7},
		{"BE 8/14 😀, jangkar grup ASTRAL rusak", true, "UTF-32BE", "A=😀😀😀😀\nB=😀😀😀😀\n", 2},
		{"LE 4/14 😀, jangkar grup 'B' rusak", false, "UTF-32LE", "A=😀😀\nB=😀😀\nC=1\n", 5},
		{"BE 4/14 😀, jangkar grup 'B' rusak", true, "UTF-32BE", "A=😀😀\nB=😀😀\nC=1\n", 5},
		{"LE 5/15 😀, jangkar grup 'B' rusak", false, "UTF-32LE", "A=😀😀\nB=😀😀😀\nC=1\n", 5},
		{"LE 3/16 😀, jangkar grup 'D' rusak", false, "UTF-32LE", "AB=😀\nC=😀😀\nDE=12\n", 10},
		{"LE 4/14 😁, jangkar grup 'B' rusak", false, "UTF-32LE", "A=😁😁\nB=😁😁\nC=1\n", 5},
		{"LE 8/14 😁, jangkar grup 'B' rusak (batas B-569)", false, "UTF-32LE", "A=😁😁😁😁\nB=😁😁😁😁\n", 7},
		{"BE 8/14 😁, jangkar grup 'B' rusak (batas B-569)", true, "UTF-32BE", "A=😁😁😁😁\nB=😁😁😁😁\n", 7},
		{"LE 8/14 U+10001, jangkar grup 'B' rusak", false, "UTF-32LE", "A=\U00010001\U00010001\U00010001\U00010001\nB=\U00010001\U00010001\U00010001\U00010001\n", 7},
	} {
		t.Run(tc.nama, func(t *testing.T) {
			data := utf32TanpaBOM(tc.isi, tc.be)
			jangkar := 4*tc.grup + 3
			if tc.be {
				jangkar = 4 * tc.grup
			}
			periksaEnvDitolakLabelTepatPR26(t, rusakBytePR26(data, jangkar, 0x01), tc.enc)
		})
	}
}

// TestB570DuaByteJangkarRusakTetapLolos mengunci BATAS
// tertulis pelonggaran B-570: DUA byte jangkar rusak tetap
// lolos dari det32 (jangkar menoleransi tepat satu grup
// menyimpang) — dan det16 pun diam atas 😀 berbita-rendah 0,
// sehingga Configure sukses seperti di base. Batas ini
// disengaja: jangkar 2-dari-g bukan lagi bukti UTF-32.
func TestB570DuaByteJangkarRusakTetapLolos(t *testing.T) {
	home := t.TempDir()
	envPath := filepath.Join(home, ".hermes", ".env")
	data := rusakBytePR26(rusakBytePR26(utf32TanpaBOM("A=😀😀😀😀\nB=😀😀😀😀\n", false), 4*7+3, 0x01), 4*9+3, 0x01)
	tulisBerkasPRD(t, envPath, data)
	if err := terapkanPRD(t, home, "hermes", "GANTI_kunci-prd"); err != nil {
		t.Fatalf("batas B-570 berubah: dua jangkar rusak kini ditolak: %v", err)
	}
}

// TestB570KontrolBMPJangkarRusakKiniDitolak mengunci
// pembalikan kontrol benign Putaran #13: .env UTF-32LE
// BMP-murni ("A=1\nB=2\nOPENAI_API_KEY=...") yang satu byte
// jangkarnya rusak kini DITOLAK seperti kembar utuhnya yang
// sudah ditolak B-548. Oracle python-dotenv (Fase 26,
// terverifikasi ulang): dotenv atas berkas ini — utuh maupun
// rusak — hanya mengembalikan nama kunci ber-NUL sampah
// ("A\x00\x00\x00", "OPENAI_API_KEY" ber-NUL antar-huruf),
// TIDAK PERNAH kunci terkelola yang bisa dibaca Hermes; jadi
// penolakan ini konsisten dengan B-548, bukan salah-tolak
// atas berkas yang bisa dibaca.
func TestB570KontrolBMPJangkarRusakKiniDitolak(t *testing.T) {
	data := rusakBytePR26(utf32TanpaBOM("A=1\nB=2\nOPENAI_API_KEY=sk-x\n", false), 4*4+3, 0x01)
	periksaEnvDitolakLabelTepatPR26(t, data, "UTF-32LE")
}

// teksUTF32FraksiRunePR26 seperti teksUTF32Fraksi tetapi
// karakter astralnya rune yang diminta — dipakai sapuan B-569.
func teksUTF32FraksiRunePR26(total, astral int, r rune) string {
	if astral >= total {
		return strings.Repeat(string(r), total)
	}
	return latinStrPRC(total-astral) + strings.Repeat(string(r), astral)
}

// TestFase26PRASapuanMekanisDetektor menyapu ruang bentuk
// detektor yang diubah PR ini — orientasi × fraksi astral ×
// rune astral × posisi jangkar rusak × label B-556 — langsung
// pada fungsi detektor. Tabel sel tercetak lewat -v;
// ringkasannya juga tertulis di badan PR.
func TestFase26PRASapuanMekanisDetektor(t *testing.T) {
	// --- B-569: UTF-32 astral × rune × fraksi × orientasi ---
	// det32 harus menamai orientasi yang benar; det16 yang
	// menyala di sel ini adalah salah-label yang kini dibayangi
	// pengurutan ulang Configure (dicatat, bukan ditegakkan).
	for _, r := range []rune{'😀', '😁', '😂', '😃', '😍', '\U00010001', '\U00020001', '\U0010FFFF'} {
		for _, fa := range []struct{ total, astral int }{
			{14, 0}, {14, 3}, {14, 4}, {14, 5}, {14, 8}, {14, 12}, {14, 14},
			{11, 8}, {8, 8}, {15, 5}, {16, 3}, {6, 2}, {10, 3},
		} {
			for _, be := range []bool{false, true} {
				isi := teksUTF32FraksiRunePR26(fa.total, fa.astral, r)
				data := utf32TanpaBOM(isi, be)
				want := "UTF-32LE"
				if be {
					want = "UTF-32BE"
				}
				got32 := deteksiUTF32TanpaBOM(data)
				got16 := deteksiUTF16TanpaBOM(data)
				t.Logf("B-569 rune=U+%04X total=%2d astral=%2d be=%v -> det32=%q det16=%q", r, fa.total, fa.astral, be, got32, got16)
				if got32 != want {
					t.Errorf("B-569 rune=U+%04X total=%d astral=%d be=%v: det32=%q, mau %q", r, fa.total, fa.astral, be, got32, want)
				}
			}
		}
	}
	// --- B-569 efek samping: label kasus B-556 tak berubah ---
	// det32 (jangkar DILONGGARKAN) tidak boleh membayangi det16
	// atas berkas UTF-16 sungguhan; label det16 harus tepat.
	for _, tc := range []struct {
		nama string
		be   bool
		want string
		isi  string
	}{
		{"B-556 LE didominasi U+0100", false, "UTF-16LE", "A=ĀĀĀĀĀĀĀĀ\nB=ĀĀĀĀĀĀĀĀ\n"},
		{"B-556 BE didominasi U+0100", true, "UTF-16BE", "A=ĀĀĀĀĀĀĀĀ\nB=ĀĀĀĀĀĀĀĀ\n"},
		{"B-556 netral 8 NEL", false, "UTF-16", "AB" + strings.Repeat("\u0085", 8) + "\n"},
		{"B-563 LE Latin-padat + 1 yatim", false, "UTF-16LE", "KUNCI=nilai\n"},
		{"B-563 LE satu NEL + 32 CJK", false, "UTF-16LE", "A=" + strings.Repeat("中", 32) + "\u0085\n"},
		{"B-563 BE satu NEL + 32 CJK", true, "UTF-16BE", "A=" + strings.Repeat("中", 32) + "\u0085\n"},
		{"B-541 LE pendek 3 suara", false, "UTF-16LE", "A=中\n"},
		{"B-541 BE pendek 3 suara", true, "UTF-16BE", "A=中\n"},
		{"B-541/B-547 LE emoji-dominan", false, "UTF-16LE", "A=😀😀😀😀😀😀😀😀\nB=😀😀😀😀😀😀😀😀\n"},
		{"B-541/B-547 BE emoji-dominan", true, "UTF-16BE", "A=😀😀😀😀😀😀😀😀\nB=😀😀😀😀😀😀😀😀\n"},
		{"B-541/B-547 LE CJK ekstensi", false, "UTF-16LE", "A=𐀀𐀀𐀀𐀀𐀀𐀀𐀀𐀀\nB=𐀀𐀀𐀀𐀀𐀀𐀀𐀀𐀀\n"},
		{"B-541 LE kunci normal + emoji panjang", false, "UTF-16LE", "KUNCI_A=GANTI_awal\nKUNCI_B=😀😀😀😀😀😀😀😀😀😀😀😀😀😀😀😀😀😀😀😀\n"},
	} {
		data := utf16SungguhanTanpaBOM(tc.isi, tc.be)
		// Kasus yatim B-563 butuh unit yatim di EOF, bukan teks.
		if strings.HasPrefix(tc.nama, "B-563 LE Latin-padat") {
			unit := append(append([]uint16{}, unitTeksPRC(tc.isi)...), 0xD800)
			data = utf16DariUnitPRC(unit, false)
		}
		got32 := deteksiUTF32TanpaBOM(data)
		got16 := deteksiUTF16TanpaBOM(data)
		t.Logf("B-556 %s -> det32=%q det16=%q (mau det16=%q)", tc.nama, got32, got16, tc.want)
		if got32 != "" {
			t.Errorf("B-556 %s: det32=%q membayangi det16 (pengurutan ulang merusak label)", tc.nama, got32)
		}
		if got16 != tc.want {
			t.Errorf("B-556 %s: det16=%q, mau %q", tc.nama, got16, tc.want)
		}
	}
	// --- B-570: jangkar rusak × posisi × orientasi × rune ---
	for _, tc := range []struct {
		isi string
		be  bool
	}{
		{"A=😀😀😀😀\nB=😀😀😀😀\n", false},
		{"A=😀😀😀😀\nB=😀😀😀😀\n", true},
		{"A=😁😁😁😁\nB=😁😁😁😁\n", false},
		{"A=😁😁😁😁\nB=😁😁😁😁\n", true},
		{"A=\U00010001\U00010001\U00010001\U00010001\nB=\U00010001\U00010001\U00010001\U00010001\n", false},
		{"A=\U0010FFFF\U0010FFFF\U0010FFFF\U0010FFFF\nB=\U0010FFFF\U0010FFFF\U0010FFFF\U0010FFFF\n", true},
		{"A=😀😀\nB=😀😀\nC=1\nD=2\n", false},
		{"AB=😀\nC=😀😀\nDE=12\n", false},
	} {
		data := utf32TanpaBOM(tc.isi, tc.be)
		g := len(data) / 4
		want := "UTF-32LE"
		if tc.be {
			want = "UTF-32BE"
		}
		jangkar := func(grup int) int {
			if tc.be {
				return 4 * grup
			}
			return 4*grup + 3
		}
		for _, grup := range []int{0, g / 2, g - 1} {
			got := deteksiUTF32TanpaBOM(rusakBytePR26(data, jangkar(grup), 0x01))
			t.Logf("B-570 be=%v grup=%d/%d -> det32=%q (mau %q)", tc.be, grup, g, got, want)
			if got != want {
				t.Errorf("B-570 be=%v grup=%d/%d: det32=%q, mau %q", tc.be, grup, g, got, want)
			}
		}
		// Dua grup rusak: tetap lolos (batas tertulis).
		got2 := deteksiUTF32TanpaBOM(rusakBytePR26(rusakBytePR26(data, jangkar(0), 0x01), jangkar(g-1), 0x01))
		if got2 != "" {
			t.Errorf("B-570 be=%v: dua jangkar rusak det32=%q, mau \"\" (batas)", tc.be, got2)
		}
	}
	// --- Gerbang salah-tolak UTF-8 (28 varian, tingkat detektor) ---
	// 15 korpus Fase 24 + 4 kontrol detektor Fase 25 PR-C +
	// B-522 + B-531 + 7 varian baru Fase 26 (di bawah).
	kontrolUTF8PR26 := []string{
		"# komentar éè ê — catatan rilis\nKUNCI=GANTI_nilai-b541\n",
		"CATATAN=中文中文中文中文中文中文中文中文\nKUNCI=GANTI_nilai-b541\n",
		"名前=テストテストテストテスト\nKUNCI=GANTI_nilai-b541\n",
		"SAPA=مرحبا بالعالم مرحبا بالعالم\nKUNCI=GANTI_nilai-b541\n",
		"MOOD=😀😀😀😀😀😀😀😀😀😀\nKUNCI=GANTI_nilai-b541\n",
		"LAIN=ü\x00x\nCATATAN=naïve façade — déjà vu\nKUNCI=GANTI_nilai-b541\n",
		"HARGA=€100 £50 ¥1000 ✓✗\nKUNCI=GANTI_nilai-b541\n",
		"CATATAN=𐀀𐀀𐀀𐀀𐀀𐀀𐀀𐀀\nKUNCI=GANTI_nilai-b541\n",
		"MOOD=😀中文😀中文😀中文😀中文\nKUNCI=GANTI_nilai-b541\n",
		"CATATAN=éèêàáâöüßÿñç\nKUNCI=GANTI_nilai-b541\n",
		"이름=테스트테스트테스트\nKUNCI=GANTI_nilai-b541\n",
		"LAIN=a\x00b\nLAIN2=ü\x00x\nLAIN3=naïve\x00façade\nKUNCI=GANTI_nilai-b541\n",
		"LAIN=\x00ab\nLAIN2=cd\x00\nKUNCI=GANTI_nilai-b541\n",
		"HARGA=€100 😀 ¥1000 𐀀 ✓\nKUNCI=GANTI_nilai-b541\n",
		"# مرحبا بالعالم\n名前=テスト\nKUNCI=GANTI_nilai-b541\n",
		"LAIN=a\x00b\nOPENAI_API_KEY=<redacted>\n",
		"LAIN=a\x00bc\nLAIN2=de\x00f\nLAIN3=gh\x00i\nOPENAI_API_KEY=<redacted>\n",
		"KUNCI=GANTI_nilai-b570\n",
		"KUNCI=GANTI_a\tb\r\nLAIN=x\r\n",
		"CATATAN=a\u2028b\nKUNCI=GANTI_nilai-b570\n",
		"LAIN=a\x7fb\nKUNCI=GANTI_nilai-b570\n",
		"KUNCI=" + strings.Repeat("x", 200) + "\x00\n",
		"MOOD=😀\x00😀\nKUNCI=GANTI_nilai-b570\n",
		"LAIN=ab\x00\x11cd\nKUNCI=GANTI_nilai-b570\n",
		"LAIN2=cd\x00\x11\nKUNCI=GANTI_nilai-b570\n",
		"LAIN=\x00\nKUNCI=GANTI_nilai-b570\n",
		"KUNCI=GANTI_nilai-b570\x00\n",
		"CATATAN=Ωμέγα\nKUNCI=GANTI_nilai-b570\n",
	}
	// Ambang tak-sah byte2=0x11 (kontrol benign B-570): berkas
	// UTF-8 sah yang memuat byte 0x11 di dalam nilai — 0x11 di
	// luar rentang astral det32 (0x01–0x10) dan bukan NUL —
	// det32 TIDAK boleh menyala. (Catatan: pola berulang
	// "AB\x11\x00" BUKAN kontrol ini — NUL berstride-4-nya
	// menyalakan jalur klasik det16 di base, di luar cakupan
	// PR ini; lihat laporan.)
	polaAmbang := "LAIN=a\x11b\nKUNCI=GANTI_nilai-b570\n"
	if len(kontrolUTF8PR26) != 28 {
		t.Fatalf("korpus kontrol UTF-8 = %d varian, mau 28", len(kontrolUTF8PR26))
	}
	for i, isi := range kontrolUTF8PR26 {
		data := []byte(isi)
		if got := deteksiUTF16TanpaBOM(data); got != "" {
			t.Errorf("kontrol UTF-8 #%d ditolak det16=%q: %q", i, got, isi)
		}
		if got := deteksiUTF32TanpaBOM(data); got != "" {
			t.Errorf("kontrol UTF-8 #%d ditolak det32=%q: %q", i, got, isi)
		}
	}
	if got := deteksiUTF32TanpaBOM([]byte(polaAmbang)); got != "" {
		t.Errorf("kontrol ambang byte2=0x11 ditolak det32=%q (mau lolos)", got)
	}
	if got := deteksiUTF16TanpaBOM([]byte(polaAmbang)); got != "" {
		t.Errorf("kontrol ambang byte2=0x11 ditolak det16=%q (mau lolos)", got)
	}
}

// TestB570KontrolUTF8BaruTidakTertolak mengunci 7 varian
// kontrol UTF-8 baru Fase 26 pada tingkat Configure sungguhan:
// Configure berhasil dan kunci terkelola tertulis.
func TestB570KontrolUTF8BaruTidakTertolak(t *testing.T) {
	for _, isi := range []string{
		"KUNCI=GANTI_nilai-b570\n",
		"KUNCI=GANTI_a\tb\r\nLAIN=x\r\n",
		"CATATAN=a\u2028b\nKUNCI=GANTI_nilai-b570\n",
		"LAIN=a\x7fb\nKUNCI=GANTI_nilai-b570\n",
		"KUNCI=" + strings.Repeat("x", 200) + "\x00\n",
		"MOOD=😀\x00😀\nKUNCI=GANTI_nilai-b570\n",
		"LAIN=a\x11b\nKUNCI=GANTI_nilai-b570\n",
	} {
		home := t.TempDir()
		envPath := filepath.Join(home, ".hermes", ".env")
		tulisBerkasPRD(t, envPath, []byte(isi))
		if err := terapkanPRD(t, home, "hermes", "GANTI_kunci-prd"); err != nil {
			t.Fatalf("applyToolSpecificConfig menolak .env UTF-8 sah: %q: %v", isi, err)
		}
		konten := bacaBerkasUji(t, envPath)
		if !strings.Contains(konten, "OPENAI_API_KEY="+"GANTI_kunci-prd") {
			t.Errorf("kunci terkelola tidak tertulis untuk kontrol %q:\n%q", isi, konten)
		}
	}
}
