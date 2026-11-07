package cliconfig

// fase27_pra_test.go — pengunci Fase 27 PR-A (cliconfig, detektor
// pengkodean .env Hermes, keluarga B-578/B-579/B-580/B-581 +
// rider redaksi B-583):
//
//	B-578 Append ASCII ≥8 byte ke .env UTF-32 berisi non-ASCII
//	      → Configure sukses-bohong (det32 mati oleh 2 grup
//	      menyimpang, det16 buta lewat penjaga NUL-ganda
//	      rapat). Kini gerbang pasca-sunting utf8.Valid atas
//	      konten .env HASIL SUNTING — sebelum tulis apa pun —
//	      menolaknya jujur. Varian ASCII-murni (semua byte
//	      UTF-32 < 0x80) tetap sukses: dotenv membacanya dan
//	      kunci baru terbaca (oracle PB14).
//	B-579 Prepend ASCII ≥6/7 byte ke .env UTF-32 berisi
//	      non-ASCII → sukses-bohong yang sama; gerbang yang
//	      sama menutupnya.
//	B-580 Berkas pendek UTF-32 (<16 byte) / UTF-16 (<8 byte)
//	      berisi non-ASCII → sukses-bohong; gerbang yang sama
//	      menangkap subset berbahaya (byte ≥0x80); subset
//	      ASCII-murni/byte-<0x80 tetap sukses (oracle).
//	B-581 det32 membayangi label UTF-16 atas berkas UTF-16
//	      yang didominasi baris kosong/kontrol: bila det32
//	      menyala tetapi penjelasan residu pendampingnya
//	      didominasi karakter kontrol (byte bidang astral
//	      9–13 yang tak berpenghuni dalam teks nyata —
//	      \n/\t/\r dalam bacaan UTF-16), label dikoreksi ke
//	      label det16. Penolakan tetap jujur; yang berubah
//	      hanya nama pengkodeannya. Preferensi naif ke det16
//	      DITOLAK oleh desain ini: ia akan menghidupkan
//	      kembali salah-label B-569 (sapuan membuktikan det16
//	      menyala salah atas korpus UTF-32 astral).
//	B-583 (BATAS SAH, redaksi saja — tanpa test perilaku yang
//	      mengubah jalur): pola NUL-rapat `abc\0` tetap DITOLAK
//	      jalur klasik det16; test di sini hanya mengunci batas
//	      itu (penolakan + label tetap), bukan perilaku baru.
//	      TEMUAN SAMPING Fase 27: pola (a) stride-3 fase-0
//	      sebagaimana dideskripsikan literal (NUL pada offset
//	      3,6,9,…) TIDAK terreproduksi — tiga varian grid
//	      stride-3 semuanya diam dari det16 (paritas NUL
//	      selang-seling; lihat
//	      TestB583Stride3LiteralTidakMemicuDet16). Konstruksi
//	      PB14 yang sesungguhnya memicu det16 hampir pasti
//	      grid `abc\0` (sisipan tiap 3 huruf).
//
// Semua test pengunci memanggil applyToolSpecificConfig — jalur
// Configure sungguhan — lewat pembantu terapkanPRD, kecuali
// sapuan mekanis yang memanggil fungsi detektor langsung untuk
// menutup ruang bentuk.

import (
	"math/rand"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

// periksaEnvDitolakGerbangUTF8PR27 menegaskan penolakan jujur
// gerbang pasca-sunting B-578: pesan galat menamai "bukan UTF-8",
// .env tak berubah, DAN config.yaml tidak ikut tertulis (gerbang
// berjalan SEBELUM tulis apa pun — bukan sesudah config.yaml).
func periksaEnvDitolakGerbangUTF8PR27(t *testing.T, asli []byte) {
	t.Helper()
	home := t.TempDir()
	envPath := filepath.Join(home, ".hermes", ".env")
	tulisBerkasPRD(t, envPath, asli)

	err := terapkanPRD(t, home, "hermes", "GANTI_kunci-prd")
	if err == nil {
		t.Fatalf("applyToolSpecificConfig melaporkan sukses atas .env yang hasil suntingnya bukan UTF-8")
	}
	if !strings.Contains(err.Error(), "bukan UTF-8") {
		t.Errorf("pesan galat bukan dari gerbang utf8.Valid: %v", err)
	}
	if _, statErr := os.Lstat(filepath.Join(home, ".hermes", "config.yaml")); !os.IsNotExist(statErr) {
		t.Errorf("config.yaml ikut tertulis sesudah penolakan gerbang (stat err=%v)", statErr)
	}
	mentah, _ := os.ReadFile(envPath)
	if string(mentah) != string(asli) {
		t.Error(".env berubah sesudah penolakan jujur")
	}
}

// periksaEnvSuksesPR27 menegaskan Configure berhasil atas .env
// yang hasil suntingnya UTF-8 sah (subset tak-merugikan B-578 /
// B-580: ASCII-murni atau semua byte < 0x80) dan kunci terkelola
// tertulis.
func periksaEnvSuksesPR27(t *testing.T, asli []byte) {
	t.Helper()
	home := t.TempDir()
	envPath := filepath.Join(home, ".hermes", ".env")
	tulisBerkasPRD(t, envPath, asli)

	if err := terapkanPRD(t, home, "hermes", "GANTI_kunci-prd"); err != nil {
		t.Fatalf("applyToolSpecificConfig menolak .env yang hasil suntingnya UTF-8 sah: %v", err)
	}
	konten := bacaBerkasUji(t, envPath)
	if !strings.Contains(konten, "OPENAI_API_KEY="+"GANTI_kunci-prd") {
		t.Errorf("kunci terkelola tidak tertulis:\n%q", konten)
	}
}

// TestB578AppendASCIIDitolakGerbangUTF8 mengunci B-578: append
// ASCII ke .env UTF-32LE/BE berisi non-ASCII kini ditolak
// gerbang utf8.Valid — di base Configure melaporkan SUKSES atas
// semua sel ini (kecuali kontrol k=7 yang sudah ditolak det32).
func TestB578AppendASCIIDitolakGerbangUTF8(t *testing.T) {
	for _, tc := range []struct {
		nama   string
		be     bool
		basis  string
		append string
	}{
		{"LE emoji, append Xx8", false, "A=😀\nB=2\nOPENAI_API_KEY=lama\n", strings.Repeat("X", 8)},
		{"BE emoji, append Xx8", true, "A=😀\nB=2\nOPENAI_API_KEY=lama\n", strings.Repeat("X", 8)},
		{"LE emoji, append Xx25", false, "A=😀\nB=2\nOPENAI_API_KEY=lama\n", strings.Repeat("X", 25)},
		{"BE emoji, append Xx25", true, "A=😀\nB=2\nOPENAI_API_KEY=lama\n", strings.Repeat("X", 25)},
		{"LE emoji, append Xx100", false, "A=😀\nB=2\nOPENAI_API_KEY=lama\n", strings.Repeat("X", 100)},
		{"BE emoji, append Xx100", true, "A=😀\nB=2\nOPENAI_API_KEY=lama\n", strings.Repeat("X", 100)},
		{"LE emoji, append baris kunci 25 byte", false, "A=😀\nB=2\nOPENAI_API_KEY=lama\n", "\nOPENAI_API_KEY=sk-lama\n"},
		{"BE emoji, append baris kunci 25 byte", true, "A=😀\nB=2\nOPENAI_API_KEY=lama\n", "\nOPENAI_API_KEY=sk-lama\n"},
		{"LE emoji, append A=1 B=2 8 byte", false, "A=😀\nB=2\nOPENAI_API_KEY=lama\n", "A=1\nB=2\n"},
		{"BE emoji, append A=1 B=2 8 byte", true, "A=😀\nB=2\nOPENAI_API_KEY=lama\n", "A=1\nB=2\n"},
		{"LE emoji, append newline x8", false, "A=😀\nB=2\nOPENAI_API_KEY=lama\n", strings.Repeat("\n", 8)},
		{"BE emoji, append newline x8", true, "A=😀\nB=2\nOPENAI_API_KEY=lama\n", strings.Repeat("\n", 8)},
		{"LE U+8000, append Xx8", false, "A=\u8000\nB=2\nOPENAI_API_KEY=lama\n", strings.Repeat("X", 8)},
		{"BE U+8000, append Xx8", true, "A=\u8000\nB=2\nOPENAI_API_KEY=lama\n", strings.Repeat("X", 8)},
		{"LE é, append Xx8", false, "A=é\nB=2\nOPENAI_API_KEY=lama\n", strings.Repeat("X", 8)},
		{"BE é, append Xx8", true, "A=é\nB=2\nOPENAI_API_KEY=lama\n", strings.Repeat("X", 8)},
	} {
		t.Run(tc.nama, func(t *testing.T) {
			data := append(append([]byte{}, utf32TanpaBOM(tc.basis, tc.be)...), []byte(tc.append)...)
			periksaEnvDitolakGerbangUTF8PR27(t, data)
		})
	}
}

// TestB578AppendKontrolTetapDitolakDet32 mengunci kontrol B-578:
// append ≤7 byte sudah ditolak det32 di base — penolakan itu
// dipertahankan (label UTF-32 tepat), bukan digantikan gerbang.
func TestB578AppendKontrolTetapDitolakDet32(t *testing.T) {
	for _, be := range []bool{false, true} {
		for _, k := range []int{0, 1, 7} {
			want := "UTF-32LE"
			if be {
				want = "UTF-32BE"
			}
			data := append(append([]byte{}, utf32TanpaBOM("A=😀\nB=2\nOPENAI_API_KEY=lama\n", be)...), []byte(strings.Repeat("X", k))...)
			periksaEnvDitolakLabelTepatPR26(t, data, want)
		}
	}
}

// TestB578AppendASCIIBasisTetapSukses mengunci subset TAK-MERUGIKAN
// B-578 (oracle PB14, diverifikasi ulang Fase 27 dengan
// python-dotenv sungguhan): append ASCII ke .env UTF-32 berisi
// ASCII-murni TETAP sukses — dotenv mengurai berkas pasca-Configure
// tanpa eksepsi dan kunci baru terbaca. Gerbang utf8.Valid tidak
// boleh false-reject subset ini: hasil suntingnya UTF-8 sah
// (semua byte < 0x80).
func TestB578AppendASCIIBasisTetapSukses(t *testing.T) {
	for _, be := range []bool{false, true} {
		for _, k := range []int{8, 25, 112} {
			data := append(append([]byte{}, utf32TanpaBOM("A=1\nB=2\nOPENAI_API_KEY=lama\n", be)...), []byte(strings.Repeat("X", k))...)
			periksaEnvSuksesPR27(t, data)
		}
	}
}

// prepend ASCII ke .env UTF-32 berisi non-ASCII kini ditolak
// gerbang utf8.Valid — di base Configure melaporkan SUKSES atas
// semua sel ini.
func TestB579PrependASCIIDitolakGerbangUTF8(t *testing.T) {
	for _, tc := range []struct {
		nama    string
		be      bool
		prepend string
	}{
		{"LE prepend Xx6", false, strings.Repeat("X", 6)},
		{"LE prepend Xx7", false, strings.Repeat("X", 7)},
		{"LE prepend Xx8", false, strings.Repeat("X", 8)},
		{"LE prepend Xx16", false, strings.Repeat("X", 16)},
		{"BE prepend Xx7", true, strings.Repeat("X", 7)},
		{"BE prepend Xx8", true, strings.Repeat("X", 8)},
		{"BE prepend Xx16", true, strings.Repeat("X", 16)},
		{"LE prepend baris KUNCI=nilai", false, "KUNCI=nilai\n"},
		{"BE prepend baris KUNCI=nilai", true, "KUNCI=nilai\n"},
		{"LE prepend baris komentar", false, "# komentar\n"},
		{"BE prepend baris komentar", true, "# komentar\n"},
		{"LE prepend baris kunci lama", false, "OPENAI_API_KEY=sk-lama\n"},
		{"BE prepend baris kunci lama", true, "OPENAI_API_KEY=sk-lama\n"},
	} {
		t.Run(tc.nama, func(t *testing.T) {
			data := append(append([]byte{}, []byte(tc.prepend)...), utf32TanpaBOM("A=😀\nB=2\n", tc.be)...)
			periksaEnvDitolakGerbangUTF8PR27(t, data)
		})
	}
}

// TestB579PrependKontrolTetapDitolakDet32 mengunci kontrol B-579:
// prepend pendek sudah ditolak det32 di base (LE L≤5, BE L≤6,
// diukur atas basis ASCII `A=1\nB=2\n` — basis yang sama dipakai
// di sini) — penolakan itu dipertahankan, bukan digantikan gerbang.
func TestB579PrependKontrolTetapDitolakDet32(t *testing.T) {
	for _, tc := range []struct {
		be bool
		l  int
	}{
		{false, 1}, {false, 5}, {true, 1}, {true, 5}, {true, 6},
	} {
		data := append(append([]byte{}, []byte(strings.Repeat("X", tc.l))...), utf32TanpaBOM("A=1\nB=2\n", tc.be)...)
		if got := deteksiUTF32TanpaBOM(data); got == "" {
			t.Fatalf("be=%v L=%d: det32 diam di base (kontrol berubah)", tc.be, tc.l)
		}
		home := t.TempDir()
		envPath := filepath.Join(home, ".hermes", ".env")
		tulisBerkasPRD(t, envPath, data)
		if err := terapkanPRD(t, home, "hermes", "GANTI_kunci-prd"); err == nil {
			t.Fatalf("be=%v L=%d: Configure sukses (kontrol B-579 berubah)", tc.be, tc.l)
		}
	}
}

// TestB579PrependASCIIBasisTetapSukses mengunci subset TAK-MERUGIKAN
// B-579 (oracle, seperti B-578): prepend ASCII ke .env UTF-32
// berisi ASCII-murni TETAP sukses — dotenv membaca kunci baru.
// Jendela evasi L=6..16 (LE) / 7..16 (BE) hanya berbahaya bila
// isinya non-ASCII (diuji TestB579PrependASCIIDitolakGerbangUTF8).
func TestB579PrependASCIIBasisTetapSukses(t *testing.T) {
	for _, tc := range []struct {
		be bool
		l  int
	}{
		{false, 6}, {false, 8}, {false, 16}, {true, 7}, {true, 8}, {true, 16},
	} {
		data := append(append([]byte{}, []byte(strings.Repeat("X", tc.l))...), utf32TanpaBOM("A=1\nB=2\n", tc.be)...)
		periksaEnvSuksesPR27(t, data)
	}
}

// TestB580BerkasPendekDitolakGerbangUTF8 mengunci B-580: berkas
// pendek UTF-32 (1–3 karakter) / UTF-16 (1–3 unit) berisi
// non-ASCII yang salah satu byte-nya ≥0x80 kini ditolak gerbang
// utf8.Valid — di base Configure melaporkan SUKSES atas semua
// sel tolak ini. Kolom tolak dikodekan eksplisit per string
// (bukan dihitung dari utf8.Valid — itu yang diuji): tolak
// iff isinya memuat karakter yang byte UTF-32/UTF-16-nya ada
// yang ≥0x80 (é U+00E9, 😀, U+8000); lolos iff semua byte
// < 0x80 (ASCII-murni, Ā U+0100, 中 U+4E2D) — subset yang
// oracle PB14 buktikan dotenv-nya membaca kunci baru.
func TestB580BerkasPendekDitolakGerbangUTF8(t *testing.T) {
	pendek32 := []struct {
		isi   string
		tolak bool
	}{
		{"A", false}, {"=", false}, {"é", true}, {"Ā", false},
		{"😀", true}, {"中", false}, {"A=", false}, {"A=1", false},
		{"Ā=", false}, {"😀=", true}, {"中=", false}, {"A=é", true},
		{"é=1", true}, {"😀=x", true}, {"中=1", false},
	}
	if len(pendek32) != 15 {
		t.Fatalf("korpus UTF-32 pendek = %d, mau 15", len(pendek32))
	}
	for _, be := range []bool{false, true} {
		for _, tc := range pendek32 {
			t.Run("UTF-32 be="+boolStrPR27(be)+" "+tc.isi, func(t *testing.T) {
				data := utf32TanpaBOM(tc.isi, be)
				if tc.tolak {
					periksaEnvDitolakGerbangUTF8PR27(t, data)
				} else {
					periksaEnvSuksesPR27(t, data)
				}
			})
		}
	}
	pendek16 := []struct {
		isi   string
		tolak bool
	}{
		{"A", false}, {"A=", false}, {"A=1", false}, {"A=é", true},
		{"Ā=", false}, {"😀", true}, {"😀=", true},
	}
	if len(pendek16) != 7 {
		t.Fatalf("korpus UTF-16 pendek = %d, mau 7", len(pendek16))
	}
	for _, be := range []bool{false, true} {
		for _, tc := range pendek16 {
			t.Run("UTF-16 be="+boolStrPR27(be)+" "+tc.isi, func(t *testing.T) {
				data := utf16SungguhanTanpaBOM(tc.isi, be)
				if tc.tolak {
					periksaEnvDitolakGerbangUTF8PR27(t, data)
				} else {
					periksaEnvSuksesPR27(t, data)
				}
			})
		}
	}
}

func boolStrPR27(be bool) string {
	if be {
		return "BE"
	}
	return "LE"
}

// TestB581LabelBayanganUTF16Dikoreksi mengunci B-581: berkas
// UTF-16 yang didominasi baris kosong/kontrol kini ditolak
// dengan label UTF-16 yang benar — di base Configure menolaknya
// berlabel "UTF-32LE/BE" (salah). Label netral "UTF-16" dipakai
// bila det16 sendiri tak membuktikan orientasi (tak ada '=');
// itu label jujur, bukan salah-label.
func TestB581LabelBayanganUTF16Dikoreksi(t *testing.T) {
	for _, tc := range []struct {
		nama string
		be   bool
		isi  string
		want string
	}{
		{"LE K=1 + 4 kosong", false, "K=1\n" + strings.Repeat("\n", 4), "UTF-16LE"},
		{"LE 5 baris normal + 20 kosong", false, "A=1\nB=2\nC=3\nD=4\nE=5\n" + strings.Repeat("\n", 20), "UTF-16LE"},
		{"BE \\n= x8", true, strings.Repeat("\n=", 8), "UTF-16BE"},
		{"BE \\nA x8 (netral: tanpa '=')", true, strings.Repeat("\nA", 8), "UTF-16"},
		{"LE =\\n x8", false, strings.Repeat("=\n", 8), "UTF-16LE"},
		{"LE A\\n x8 (netral: tanpa '=')", false, strings.Repeat("A\n", 8), "UTF-16"},
		{"LE \\n x8 (netral: tanpa '=')", false, strings.Repeat("\n", 8), "UTF-16"},
		{"LE KUNCI=nilai + 20 kosong (kontrol: sudah benar di base)", false, "KUNCI=nilai\n" + strings.Repeat("\n", 20), "UTF-16LE"},
		{"LE KUNCI=nilai + 28 kosong (ambang bayangan)", false, "KUNCI=nilai\n" + strings.Repeat("\n", 28), "UTF-16LE"},
		{"LE tab-dominan", false, "K=1\n" + strings.Repeat("\t\n", 6), "UTF-16LE"},
		{"BE tab-dominan", true, "K=1\n" + strings.Repeat("\t\n", 6), "UTF-16BE"},
		{"LE CR-dominan", false, "K=1\r\n" + strings.Repeat("\r\n", 6), "UTF-16LE"},
	} {
		t.Run(tc.nama, func(t *testing.T) {
			data := utf16SungguhanTanpaBOM(tc.isi, tc.be)
			if got := deteksiUTF32TanpaBOM(data); got == "" && tc.nama != "LE KUNCI=nilai + 20 kosong (kontrol: sudah benar di base)" {
				t.Fatalf("kasus tidak membayangi di base: det32 diam (det16=%q)", deteksiUTF16TanpaBOM(data))
			}
			periksaEnvDitolakLabelTepatPR26(t, data, tc.want)
		})
	}
}

// polaNULRapatStride3PR27 membangun pola B-583(a) SECARA LITERAL:
// NUL tiap 3 byte pada offset 3,6,9,… (fase-0) — 60 byte lalu
// baris kunci.
func polaNULRapatStride3PR27() []byte {
	var b []byte
	for i := 0; i < 60; i++ {
		if i > 0 && i%3 == 0 {
			b = append(b, 0)
		} else {
			b = append(b, 'a'+byte(i%26))
		}
	}
	return append(b, []byte("\nKUNCI=GANTI_nilai-b583\n")...)
}

// polaNULRapatStride3NolPR27 adalah varian (a) dengan NUL juga
// pada offset 0 (grid 0,3,6,…).
func polaNULRapatStride3NolPR27() []byte {
	var b []byte
	for i := 0; i < 60; i++ {
		if i%3 == 0 {
			b = append(b, 0)
		} else {
			b = append(b, 'a'+byte(i%26))
		}
	}
	return append(b, []byte("\nKUNCI=GANTI_nilai-b583\n")...)
}

// polaNULRapatABNULPR27 adalah varian (a) `ab\0` berulang
// (grid stride-3, NUL pada offset 2,5,8,…).
func polaNULRapatABNULPR27() []byte {
	return append([]byte(strings.Repeat("ab\x00", 20)), []byte("\nKUNCI=GANTI_nilai-b583\n")...)
}

// polaNULRapatABCNULPR27 membangun pola B-583(b): `abc\0`
// berulang — NUL tiap 4 byte, fase byte-ke-2-pasangan.
func polaNULRapatABCNULPR27() []byte {
	return append([]byte(strings.Repeat("abc\x00", 15)), []byte("\nKUNCI=GANTI_nilai-b583\n")...)
}

// TestB583PolaNULRapatTetapDitolak mengunci BATAS SAH B-583:
// pola NUL-rapat `abc\0` (stride-4, NUL byte-ke-2-pasangan)
// tetap DITOLAK jalur klasik det16 (perilaku TIDAK berubah —
// redaksi saja). Oracle: dotenv membaca KUNCI pada pola ini
// (terbukti PB14, diverifikasi ulang Fase 27), jadi ini
// salah-tolak yang disengaja dan terdokumentasi, bukan deteksi
// yang benar.
func TestB583PolaNULRapatTetapDitolak(t *testing.T) {
	data := polaNULRapatABCNULPR27()
	if got := deteksiUTF16TanpaBOM(data); got == "" {
		t.Fatalf("pola abc\\0 lolos det16 (batas B-583 berubah)")
	}
	periksaEnvDitolakLabelTepatPR26(t, data, "UTF-16")
}

// TestB583Stride3LiteralTidakMemicuDet16 mengunci TEMUAN SAMPING
// Fase 27 atas B-583(a): grid stride-3 literal (NUL pada offset
// 3,6,9,…; varian 0,3,6,…; varian `ab\0`) TIDAK memicu det16 —
// NUL stride-3 selang-seling paritas pasangan sehingga jalur
// klasik tak pernah mencapai dominasi 2:1, dan jalur
// proporsional tak menemukan '=' mandiri. Ketiga varian lolos
// dari detektor dan Configure SUKSES (dotenv membaca KUNCI —
// berkasnya UTF-8 sah). Artinya deskripsi (a) di backlog tidak
// terreproduksi secara literal: konstruksi PB14 yang
// sesungguhnya memicu det16 hampir pasti grid `abc\0` (sisipan
// tiap 3 huruf) — pola yang sama dengan (b). Redaksi B-583
// mendokumentasikan kedua pola sesuai backlog beserta catatan
// ini; perilaku TIDAK diubah.
func TestB583Stride3LiteralTidakMemicuDet16(t *testing.T) {
	for _, tc := range []struct {
		nama string
		data []byte
	}{
		{"NUL di 3,6,9,…", polaNULRapatStride3PR27()},
		{"NUL di 0,3,6,…", polaNULRapatStride3NolPR27()},
		{"ab\\0 berulang", polaNULRapatABNULPR27()},
	} {
		t.Run(tc.nama, func(t *testing.T) {
			if got := deteksiUTF16TanpaBOM(tc.data); got != "" {
				t.Errorf("pola stride-3 %s: det16=%q, mau diam (temuan berubah)", tc.nama, got)
			}
			if got := deteksiUTF32TanpaBOM(tc.data); got != "" {
				t.Errorf("pola stride-3 %s: det32=%q, mau diam", tc.nama, got)
			}
			periksaEnvSuksesPR27(t, tc.data)
		})
	}
}

// teksUTF16AcakPR27 membangkitkan satu berkas UTF-16 searoma
// populasi B-581 (baris normal diselingi baris kosong/kontrol)
// dari generator berbenih tetap — deterministik antar lari.
func teksUTF16AcakPR27(rng *rand.Rand) string {
	baris := []string{"KUNCI=nilai", "A=1", "B=2", "K=x", "A", "=", "K", "\t", "\t\t"}
	var sb strings.Builder
	for n := rng.Intn(37) + 4; n > 0; n-- {
		if rng.Intn(100) < 60 {
			sb.WriteString("\n") // baris kosong
		} else {
			sb.WriteString(baris[rng.Intn(len(baris))])
			sb.WriteString("\n")
		}
	}
	return sb.String()
}

// TestFase27PRASapuanMekanisDetektor menyapu ruang bentuk yang
// diubah PR ini — langsung pada fungsi detektor (cepat, tepat)
// dan pada tingkat Configure untuk sel yang mengubah vonis.
// Tabel sel tercetak lewat -v; ringkasannya juga tertulis di
// badan PR.
func TestFase27PRASapuanMekanisDetektor(t *testing.T) {
	// --- B-578: append × k × orientasi × isi (tingkat Configure) ---
	// k≤7 ditolak det32 (label tepat); 8..105 ditolak gerbang
	// utf8.Valid (jendela evasi atas basis emoji ini — batasnya
	// ditentukan penjaga NUL-ganda det16, ganda*4>=n: basis ASCII
	// PB14 mencapai 112, basis emoji 105 karena emoji tak
	// menyumbang pasangan (0,0)); ≥106 ditolak det16 (label
	// salah — residu tercatat B-578, penolakan tetap jujur, di
	// luar cakupan koreksi label B-581 yang hanya menyentuh
	// cabang det32).
	for _, be := range []bool{false, true} {
		for _, k := range []int{0, 1, 7, 8, 9, 16, 25, 100, 105, 106, 112, 116, 120} {
			data := append(append([]byte{}, utf32TanpaBOM("A=😀\nB=2\nOPENAI_API_KEY=lama\n", be)...), []byte(strings.Repeat("X", k))...)
			got32, got16 := deteksiUTF32TanpaBOM(data), deteksiUTF16TanpaBOM(data)
			t.Logf("B-578 append be=%v k=%3d -> det32=%q det16=%q", be, k, got32, got16)
			switch {
			case k <= 7:
				if got32 == "" {
					t.Errorf("B-578 be=%v k=%d: det32 diam (kontrol berubah)", be, k)
				}
				periksaEnvDitolakLabelTepatPR26(t, data, map[bool]string{false: "UTF-32LE", true: "UTF-32BE"}[be])
			case k <= 105:
				if got32 != "" || got16 != "" {
					t.Errorf("B-578 be=%v k=%d: detektor menyala (det32=%q det16=%q) — sel evasi berubah", be, k, got32, got16)
				}
				periksaEnvDitolakGerbangUTF8PR27(t, data)
			default:
				if got16 == "" {
					t.Errorf("B-578 be=%v k=%d: det16 diam (residu k≥106 berubah)", be, k)
				}
				home := t.TempDir()
				envPath := filepath.Join(home, ".hermes", ".env")
				tulisBerkasPRD(t, envPath, data)
				if err := terapkanPRD(t, home, "hermes", "GANTI_kunci-prd"); err == nil {
					t.Errorf("B-578 be=%v k=%d: Configure sukses (residu k≥116 berubah)", be, k)
				}
			}
		}
	}
	// --- B-579: prepend × L × orientasi (tingkat Configure) ---
	for _, be := range []bool{false, true} {
		for _, l := range []int{1, 5, 6, 7, 8, 16} {
			data := append(append([]byte{}, []byte(strings.Repeat("X", l))...), utf32TanpaBOM("A=😀\nB=2\n", be)...)
			got32, got16 := deteksiUTF32TanpaBOM(data), deteksiUTF16TanpaBOM(data)
			t.Logf("B-579 prepend be=%v L=%2d -> det32=%q det16=%q", be, l, got32, got16)
			if got32 != "" {
				home := t.TempDir()
				envPath := filepath.Join(home, ".hermes", ".env")
				tulisBerkasPRD(t, envPath, data)
				if err := terapkanPRD(t, home, "hermes", "GANTI_kunci-prd"); err == nil {
					t.Errorf("B-579 be=%v L=%d: Configure sukses (kontrol det32 berubah)", be, l)
				}
			} else {
				if got16 != "" {
					t.Errorf("B-579 be=%v L=%d: det16 menyala (det16=%q) — sel evasi berubah", be, l, got16)
				}
				periksaEnvDitolakGerbangUTF8PR27(t, data)
			}
		}
	}
	// --- B-581: sapuan acak UTF-16 searoma populasi PB14 ---
	// Untuk tiap berkas yang det32-nya menyala, koreksi label
	// HARUS sama dengan label det16 (bila det16 menyala) — tidak
	// boleh mengarang label lain; berkas yang det16-nya diam
	// dicatat sebagai residu (tanpa koreksi, label UTF-32
	// dipertahankan seperti base).
	rng := rand.New(rand.NewSource(27))
	var nAcak, nBayang, nKoreksi, nResidu, nSalah int
	for i := 0; i < 2000; i++ {
		isi := teksUTF16AcakPR27(rng)
		be := rng.Intn(2) == 0
		data := utf16SungguhanTanpaBOM(isi, be)
		nAcak++
		enc32 := deteksiUTF32TanpaBOM(data)
		if enc32 == "" {
			continue
		}
		nBayang++
		enc16 := deteksiUTF16TanpaBOM(data)
		k := koreksiLabelBayanganUTF16(data, enc32)
		switch {
		case enc16 == "":
			nResidu++
			if k != "" {
				nSalah++
				t.Errorf("B-581 acak #%d: det16 diam tetapi koreksi=%q (enc32=%q)", i, k, enc32)
			}
		case k == "":
			nSalah++
			t.Errorf("B-581 acak #%d: bayangan tak terkoreksi (enc32=%q enc16=%q, %d byte)", i, enc32, enc16, len(data))
		case k != enc16:
			nSalah++
			t.Errorf("B-581 acak #%d: koreksi=%q ≠ label det16=%q (enc32=%q)", i, k, enc16, enc32)
		default:
			nKoreksi++
		}
	}
	t.Logf("B-581 sapuan acak: %d berkas, %d bayangan det32, %d terkoreksi ke label det16, %d residu (det16 diam), %d salah", nAcak, nBayang, nKoreksi, nResidu, nSalah)
	if nBayang == 0 {
		t.Errorf("B-581 sapuan acak: nol bayangan — generator tidak searoma populasi PB14")
	}
	// --- B-581 anti-salah-positif: korpus UTF-32 sungguhan ---
	// Koreksi TIDAK BOLEH menyala atas satu pun sel UTF-32
	// (B-569 astral × rune × fraksi, B-556, B-570 jangkar rusak,
	// ASCII-murni, BMP berkontrol) — inilah yang membedakan
	// desain ini dari preferensi naif ke det16.
	var n32, nFP int
	periksaTidakTerkoreksi := func(nama string, data []byte) {
		n32++
		if enc32 := deteksiUTF32TanpaBOM(data); enc32 != "" {
			if k := koreksiLabelBayanganUTF16(data, enc32); k != "" {
				nFP++
				t.Errorf("B-581 anti-FP %s: koreksi=%q atas %s sungguhan (enc32=%q enc16=%q)", nama, k, nama, enc32, deteksiUTF16TanpaBOM(data))
			}
		}
	}
	for _, r := range []rune{'😀', '😁', '😂', '😃', '😍', '\U00010001', '\U00020001', '\U0010FFFF'} {
		for _, fa := range []struct{ total, astral int }{
			{14, 0}, {14, 3}, {14, 4}, {14, 5}, {14, 8}, {14, 12}, {14, 14},
			{11, 8}, {8, 8}, {15, 5}, {16, 3}, {6, 2}, {10, 3},
		} {
			for _, be := range []bool{false, true} {
				periksaTidakTerkoreksi("B-569", utf32TanpaBOM(teksUTF32FraksiRunePR26(fa.total, fa.astral, r), be))
			}
		}
	}
	for _, isi := range []string{
		"A=ĀĀĀĀĀĀĀĀ\nB=ĀĀĀĀĀĀĀĀ\n", "A=1\nB=2\nOPENAI_API_KEY=sk-x\n",
		"A=😀😀😀😀\nB=😀😀😀😀\n", "A=\t\nB=\r\nC=1\n",
		"KUNCI=nilai\nLAIN=é\n", "A=\u0085\u0085\nB=1\n",
		"A=" + strings.Repeat("x", 200) + "\n",
	} {
		for _, be := range []bool{false, true} {
			periksaTidakTerkoreksi("B-556/BMP", utf32TanpaBOM(isi, be))
		}
	}
	// B-570: satu jangkar rusak (semua posisi × orientasi × rune).
	for _, tc := range []struct {
		isi string
		be  bool
	}{
		{"A=😀😀😀😀\nB=😀😀😀😀\n", false},
		{"A=😀😀😀😀\nB=😀😀😀😀\n", true},
		{"A=😁😁😁😁\nB=😁😁😁😁\n", false},
		{"A=\U00010001\U00010001\U00010001\U00010001\nB=\U00010001\U00010001\U00010001\U00010001\n", false},
	} {
		data := utf32TanpaBOM(tc.isi, tc.be)
		g := len(data) / 4
		jangkar := func(grup int) int {
			if tc.be {
				return 4 * grup
			}
			return 4*grup + 3
		}
		for _, grup := range []int{0, g / 2, g - 1} {
			periksaTidakTerkoreksi("B-570", rusakBytePR26(data, jangkar(grup), 0x01))
		}
	}
	t.Logf("B-581 anti-salah-positif: %d sel UTF-32, %d salah-koreksi", n32, nFP)
	// --- Preferensi naif ke det16 DITOLAK (bukti desain) ---
	// Pada korpus B-569 di atas, hitung sel yang det16-nya
	// menyala dengan label SALAH (bukan UTF-32): preferensi naif
	// akan salah melabeli sel-sel itu — inilah yang dihidupkan
	// kembali bila det16 didahulukan.
	var nNaif int
	for _, r := range []rune{'😀', '😁', '😂', '😃', '😍', '\U00010001', '\U00020001', '\U0010FFFF'} {
		for _, fa := range []struct{ total, astral int }{
			{14, 8}, {14, 12}, {14, 14}, {11, 8}, {8, 8},
		} {
			for _, be := range []bool{false, true} {
				data := utf32TanpaBOM(teksUTF32FraksiRunePR26(fa.total, fa.astral, r), be)
				if got := deteksiUTF16TanpaBOM(data); got != "" {
					nNaif++
					t.Logf("B-581 anti-naif: rune=U+%04X total=%d astral=%d be=%v -> det16=%q (salah atas UTF-32 sungguhan)", r, fa.total, fa.astral, be, got)
				}
			}
		}
	}
	t.Logf("B-581 anti-naif: %d sel UTF-32 astral-dominan akan salah label bila det16 didahulukan", nNaif)
	if nNaif == 0 {
		t.Errorf("B-581 anti-naif: nol sel — bukti penolakan preferensi naif hilang (korpus berubah?)")
	}
	// --- Gerbang salah-tolak UTF-8 (28 lama + varian baru) ---
	// Tingkat detektor: tak satu pun menyalakan det16/det32.
	// Tingkat Configure: tak satu pun ditolak (gerbang
	// pasca-sunting utf8.Valid tidak false-reject atas UTF-8
	// sah — NUL terisolasi pun UTF-8 sah).
	kontrolUTF8PR27 := []string{
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
		// Varian baru Fase 27 (gerbang pasca-sunting):
		"",                                             // .env kosong
		"# ciao é — hanya komentar\n",                  // hanya komentar
		"LAIN=a\x00b\nOPENAI_API_KEY=lama\n",           // NUL tak-terkelola + kunci terkelola ada
		"KUNCI=x\n\x00",                                // NUL di EOF
		"MOOD=😀\x00😀\nKUNCI=y\n",                       // emoji + NUL
		"KUNCI=a\tb\r\nLAIN=é\r\n",                     // CRLF + tab + non-ASCII
		"CATATAN=a\u2028b\x00\nKUNCI=y\n",              // U+2028 + NUL
		"KUNCI=" + strings.Repeat("x", 500) + "\x00\n", // nilai panjang + NUL
		"LAIN=a\x7f\x00b\nKUNCI=y\n",                   // DEL + NUL
		"LAIN=a\x11b\nKUNCI=y\n",                       // byte 0x11 (kontrol benign B-570)
		"LAIN=\x01\x02b\nKUNCI=y\n",                    // kontrol C0 lain
		"LAIN=\u0085\nKUNCI=y\n",                       // NEL mentah UTF-8
	}
	if len(kontrolUTF8PR27) != 40 {
		t.Fatalf("korpus kontrol UTF-8 = %d varian, mau 40 (28 lama + 12 baru)", len(kontrolUTF8PR27))
	}
	for i, isi := range kontrolUTF8PR27 {
		data := []byte(isi)
		if got := deteksiUTF16TanpaBOM(data); got != "" {
			t.Errorf("kontrol UTF-8 #%d ditolak det16=%q: %q", i, got, isi)
		}
		if got := deteksiUTF32TanpaBOM(data); got != "" {
			t.Errorf("kontrol UTF-8 #%d ditolak det32=%q: %q", i, got, isi)
		}
		periksaEnvSuksesPR27(t, data)
	}
	// B-583: pola `abc\0` tetap ditolak (batas sah) — dicatat di
	// sini sebagai kontrol sapuan, bukan vonis baru; ketiga
	// varian stride-3 literal tetap diam (temuan samping Fase 27,
	// lihat TestB583Stride3LiteralTidakMemicuDet16).
	if got := deteksiUTF16TanpaBOM(polaNULRapatABCNULPR27()); got != "UTF-16" {
		t.Errorf("B-583 abc\\0: det16=%q, mau \"UTF-16\" (batas berubah)", got)
	}
	if got := deteksiUTF32TanpaBOM(polaNULRapatABCNULPR27()); got != "" {
		t.Errorf("B-583 abc\\0: det32=%q menyala (di luar cakupan)", got)
	}
	for _, tc := range []struct {
		nama string
		data []byte
	}{
		{"stride-3 3,6,9,…", polaNULRapatStride3PR27()},
		{"stride-3 0,3,6,…", polaNULRapatStride3NolPR27()},
		{"ab\\0 berulang", polaNULRapatABNULPR27()},
	} {
		if got := deteksiUTF16TanpaBOM(tc.data); got != "" {
			t.Errorf("B-583 %s: det16=%q, mau diam", tc.nama, got)
		}
		if got := deteksiUTF32TanpaBOM(tc.data); got != "" {
			t.Errorf("B-583 %s: det32=%q, mau diam", tc.nama, got)
		}
	}
}
