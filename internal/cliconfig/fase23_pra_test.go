package cliconfig

// fase23_pra_test.go — pengunci Fase 23 PR-A (cliconfig):
//
//	B-531 detektor UTF-16 tanpa BOM diperketat: satu karakter
//	      U+xx00 lazim tidak lagi menggugurkan vonis, dan tiga
//	      kelas lolos lain (berkas <8 pasangan, satu U+0000
//	      tertanam, dominasi karakter di atas U+00FF) tertutup —
//	      sementara arah salah-tolak atas .env UTF-8 sah tetap
//	      NOL (gerbang mutlak toleransi B-507).
//	B-535 penanda akhir dokumen "..." ber-CRLF dikenali
//	      penyunting Goose & Aider (config dari penyunting
//	      Windows).
//	B-536 Hermes: config.yaml berpenanda "..." TANPA blok
//	      model: berhasil di-Configure — blok baru disisipkan
//	      SEBELUM penanda, di dalam dokumen.
//
// Semua test memanggil applyToolSpecificConfig — jalur Configure
// sungguhan — lewat pembantu terapkanPRD.

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

// periksaEnvUTF16DitolakB531 menegaskan penolakan jujur jalur
// Hermes atas .env UTF-16 tanpa BOM: galat menamai pengkodean,
// .env tidak berubah, config.yaml tidak ikut tertulis.
func periksaEnvUTF16DitolakB531(t *testing.T, asli []byte, enc string) {
	t.Helper()
	home := t.TempDir()
	envPath := filepath.Join(home, ".hermes", ".env")
	tulisBerkasPRD(t, envPath, asli)

	err := terapkanPRD(t, home, "hermes", "GANTI_kunci-prd")
	if err == nil {
		t.Fatalf("applyToolSpecificConfig melaporkan sukses atas .env %s", enc)
	}
	if !strings.Contains(err.Error(), enc) || !strings.Contains(err.Error(), "tanpa BOM") {
		t.Errorf("pesan galat tidak menamai pengkodean + tanpa BOM: %v", err)
	}
	if _, statErr := os.Lstat(filepath.Join(home, ".hermes", "config.yaml")); !os.IsNotExist(statErr) {
		t.Errorf("config.yaml ikut tertulis sesudah penolakan (stat err=%v)", statErr)
	}
	mentah, _ := os.ReadFile(envPath)
	if string(mentah) != string(asli) {
		t.Error(".env berubah sesudah penolakan jujur")
	}
}

// TestB531EnvUTF16KarakterUmumDitolak mengunci kelas utama
// B-531: TEPAT SATU karakter U+xx00 yang lazim di 512 byte
// pertama tidak lagi meloloskan .env UTF-16 tanpa BOM — versi
// B-522 menggugurkan vonis karena karakter itu terbaca sebagai
// pola orientasi lawan.
func TestB531EnvUTF16KarakterUmumDitolak(t *testing.T) {
	for _, tc := range []struct {
		nama string
		be   bool
		enc  string
		isi  string
	}{
		{"LE satu U+2100 di awal nilai", false, "UTF-16LE",
			"CATATAN=℀penting\nVARIABEL_LAIN=GANTI_nilai-b531\n"},
		{"LE satu U+4E00 di tengah nilai", false, "UTF-16LE",
			"VARIABEL_LAIN=GANTI_awal-一-akhir\nVARIABEL_KEDUA=GANTI_juga-b531\n"},
		{"BE satu U+2600 di awal nilai", true, "UTF-16BE",
			"CATATAN=☀cerah\nVARIABEL_LAIN=GANTI_nilai-b531\n"},
	} {
		t.Run(tc.nama, func(t *testing.T) {
			periksaEnvUTF16DitolakB531(t, utf16TanpaBOM(tc.isi, tc.be), tc.enc)
		})
	}
}

// TestB531EnvUTF16KelasLainDitolak mengunci tiga kelas lolos
// lain di luar catatan B-522: berkas lebih pendek dari 8
// pasangan, satu karakter U+0000 tertanam, dan teks yang
// didominasi karakter di atas U+00FF (CJK).
func TestB531EnvUTF16KelasLainDitolak(t *testing.T) {
	for _, tc := range []struct {
		nama string
		be   bool
		enc  string
		isi  string
	}{
		{"LE berkas pendek 4 pasangan", false, "UTF-16LE", "X=1\n"},
		{"LE satu U+0000 tertanam", false, "UTF-16LE",
			"VARIABEL=ab\x00cd\nVARIABEL_LAIN=GANTI_nilai-b531\n"},
		{"LE dominasi CJK", false, "UTF-16LE", "K=中文中文v\n"},
	} {
		t.Run(tc.nama, func(t *testing.T) {
			periksaEnvUTF16DitolakB531(t, utf16TanpaBOM(tc.isi, tc.be), tc.enc)
		})
	}
}

// TestB531EnvUTF8NULTersebarTidakTertolak adalah kontrol arah
// sebaliknya (gerbang mutlak B-531): .env UTF-8 sah dengan
// BEBERAPA NUL tersebar paritas campuran di baris tak-terkelola
// TIDAK tertolak — NUL tersebar tidak pernah memenuhi bukti +
// dominasi + kerapatan detektor baru sekaligus.
func TestB531EnvUTF8NULTersebarTidakTertolak(t *testing.T) {
	home := t.TempDir()
	envPath := filepath.Join(home, ".hermes", ".env")
	tulisBerkasPRD(t, envPath, []byte("LAIN=a\x00bc\nLAIN2=de\x00f\nLAIN3=gh\x00i\nOPENAI_API_KEY=GANTI_lama\n"))

	if err := terapkanPRD(t, home, "hermes", "GANTI_kunci-prd"); err != nil {
		t.Fatalf("applyToolSpecificConfig hermes menolak .env UTF-8 ber-NUL tersebar: %v", err)
	}
	konten := bacaBerkasUji(t, envPath)
	for _, warisan := range []string{"LAIN=a\x00bc", "LAIN2=de\x00f", "LAIN3=gh\x00i"} {
		if !strings.Contains(konten, warisan) {
			t.Errorf("baris warisan %q tidak utuh:\n%q", warisan, konten)
		}
	}
	if !strings.Contains(konten, "OPENAI_API_KEY=GANTI_kunci-prd") {
		t.Errorf("kunci terkelola tidak tertulis:\n%q", konten)
	}
}

// TestB535GoosePenandaCRLF mengunci B-535 untuk Goose: config
// satu dokumen CRLF yang diakhiri "..." (bawaan penyunting
// Windows) berhasil di-Configure — dulu penanda "...\r" tidak
// dikenali dan Configure gagal "hasil sunting YAML tidak
// terurai".
func TestB535GoosePenandaCRLF(t *testing.T) {
	home := t.TempDir()
	jalur := filepath.Join(home, ".config", "goose", "config.yaml")
	tulisBerkasPRD(t, jalur, []byte("lainnya: 1\r\n...\r\n"))

	if err := terapkanPRD(t, home, "goose", ""); err != nil {
		t.Fatalf("applyToolSpecificConfig goose atas config CRLF berpenanda '...': %v", err)
	}
	periksaDokumenPenandaB523(t, jalur, "GOOSE_MODEL", "model-baru")
}

// TestB535AiderPenandaCRLF mengunci B-535 untuk Aider
// (penyunting bersama yang sama).
func TestB535AiderPenandaCRLF(t *testing.T) {
	home := t.TempDir()
	jalur := filepath.Join(home, ".aider.conf.yml")
	tulisBerkasPRD(t, jalur, []byte("lainnya: 1\r\n...\r\n"))

	if err := terapkanPRD(t, home, "aider", ""); err != nil {
		t.Fatalf("applyToolSpecificConfig aider atas config CRLF berpenanda '...': %v", err)
	}
	periksaDokumenPenandaB523(t, jalur, "openai-api-base", urlUjiPRD)
}

// periksaHermesPenandaB536 menegaskan hasil Configure Hermes
// atas config berpenanda "...": terurai sebagai SATU dokumen,
// blok model efektif DI DALAM dokumen dengan nilai Route-X,
// kunci tak-terkelola utuh, dan penanda tetap baris penutup.
func periksaHermesPenandaB536(t *testing.T, jalur string) {
	t.Helper()
	konten := bacaBerkasUji(t, jalur)
	doc, err := uraiYAMLUtuh(konten)
	if err != nil {
		t.Fatalf("hasil sunting tidak terurai utuh: %v\n%s", err, konten)
	}
	if n, errN := jumlahDokumenYAML(konten); errN != nil || n != 1 {
		t.Errorf("jumlah dokumen = %d (err=%v), mau 1", n, errN)
	}
	if doc["lainnya"] == nil {
		t.Errorf("kunci tak-terkelola 'lainnya' hilang dari dokumen:\n%s", konten)
	}
	blok, ok := doc["model"].(map[string]any)
	if !ok {
		t.Fatalf("blok model tidak efektif di dalam dokumen: %v\n%s", doc["model"], konten)
	}
	if blok["default"] != "model-baru" || blok["provider"] != "custom" ||
		blok["base_url"] != urlUjiPRD || blok["api_mode"] != "chat_completions" {
		t.Errorf("isi blok model tidak sesuai nilai Route-X: %v\n%s", blok, konten)
	}
	baris := strings.Split(strings.TrimRight(konten, "\n"), "\n")
	if terakhir := strings.TrimSpace(baris[len(baris)-1]); terakhir != "..." {
		t.Errorf("baris terakhir = %q, mau penanda '...' tetap menutup dokumen:\n%s", terakhir, konten)
	}
}

// TestB536HermesPenandaTanpaBlokModel mengunci B-536: config
// Hermes berpenanda "..." TANPA blok model: yang sudah ada
// berhasil di-Configure pada kedua bentuk akhir baris — dulu
// blok baru ditempel SESUDAH penanda (di luar dokumen) dan
// verifikasi menolaknya "blok model tidak efektif".
func TestB536HermesPenandaTanpaBlokModel(t *testing.T) {
	for _, tc := range []struct {
		nama string
		isi  string
	}{
		{"LF", "lainnya: 1\n...\n"},
		{"CRLF", "lainnya: 1\r\n...\r\n"},
	} {
		t.Run(tc.nama, func(t *testing.T) {
			home := t.TempDir()
			jalur := filepath.Join(home, ".hermes", "config.yaml")
			tulisBerkasPRD(t, jalur, []byte(tc.isi))

			if err := terapkanPRD(t, home, "hermes", ""); err != nil {
				t.Fatalf("applyToolSpecificConfig hermes atas config berpenanda tanpa blok model (%s): %v", tc.nama, err)
			}
			periksaHermesPenandaB536(t, jalur)
		})
	}
}

// TestB536HermesPenandaDenganBlokModel adalah kontrol arah
// sebaliknya: config berpenanda DENGAN blok model: yang sudah
// ada tetap diperbarui di tempat dan penanda tetap menutup
// dokumen (jalur yang sudah benar sebelum B-536).
func TestB536HermesPenandaDenganBlokModel(t *testing.T) {
	home := t.TempDir()
	jalur := filepath.Join(home, ".hermes", "config.yaml")
	tulisBerkasPRD(t, jalur, []byte("model:\n  default: lama\nlainnya: 1\n...\n"))

	if err := terapkanPRD(t, home, "hermes", ""); err != nil {
		t.Fatalf("applyToolSpecificConfig hermes atas config berpenanda dengan blok model: %v", err)
	}
	periksaHermesPenandaB536(t, jalur)
}
