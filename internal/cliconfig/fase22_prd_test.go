package cliconfig

// fase22_prd_test.go — pengunci Fase 22 PR-D (cliconfig):
//
//	B-522 deteksi UTF-16 TANPA BOM di jalur baca .env terkelola
//	      Hermes → penolakan jujur seperti B-477, TANPA membuka
//	      kembali toleransi NUL terisolasi B-507.
//	B-523 penyunting baris Goose & Aider mengenali penanda akhir
//	      dokumen "..." pada config satu dokumen yang sah:
//	      kunci terkelola disisipkan SEBELUM penanda, bukan
//	      ditempel sesudahnya di luar dokumen.
//
// Semua test memanggil applyToolSpecificConfig — jalur Configure
// sungguhan — lewat pembantu terapkanPRD.

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

// utf16TanpaBOM mengkodekan teks sebagai UTF-16LE/BE TANPA BOM —
// bentuk yang lolos dari deteksiBOMNonUTF8 (B-522).
func utf16TanpaBOM(teks string, be bool) []byte {
	var out []byte
	for _, r := range teks {
		if be {
			out = append(out, byte(r>>8), byte(r))
		} else {
			out = append(out, byte(r), byte(r>>8))
		}
	}
	return out
}

// TestB522EnvUTF16TanpaBOMDitolak mengunci B-522 dua arah
// pengkodean: .env Hermes UTF-16LE maupun UTF-16BE tanpa BOM
// ditolak sebelum berkas apa pun ditulis, dengan pesan yang
// menamai pengkodeannya.
func TestB522EnvUTF16TanpaBOMDitolak(t *testing.T) {
	isi := "VARIABEL_LAIN=GANTI_nilai-b522\nVARIABEL_KEDUA=GANTI_juga-b522\n"
	for _, tc := range []struct {
		nama string
		be   bool
		enc  string
	}{
		{"UTF-16LE tanpa BOM", false, "UTF-16LE"},
		{"UTF-16BE tanpa BOM", true, "UTF-16BE"},
	} {
		t.Run(tc.nama, func(t *testing.T) {
			home := t.TempDir()
			envPath := filepath.Join(home, ".hermes", ".env")
			asli := utf16TanpaBOM(isi, tc.be)
			tulisBerkasPRD(t, envPath, asli)

			err := terapkanPRD(t, home, "hermes", "GANTI_kunci-prd")
			if err == nil {
				t.Fatalf("applyToolSpecificConfig melaporkan sukses atas .env %s", tc.nama)
			}
			if !strings.Contains(err.Error(), tc.enc) || !strings.Contains(err.Error(), "tanpa BOM") {
				t.Errorf("pesan galat tidak menamai pengkodean + tanpa BOM: %v", err)
			}
			if _, statErr := os.Lstat(filepath.Join(home, ".hermes", "config.yaml")); !os.IsNotExist(statErr) {
				t.Errorf("config.yaml ikut tertulis sesudah penolakan (stat err=%v)", statErr)
			}
			mentah, _ := os.ReadFile(envPath)
			if string(mentah) != string(asli) {
				t.Error(".env berubah sesudah penolakan jujur")
			}
		})
	}
}

// TestB522EnvUTF8NULIsolasiTidakTertolak adalah kontrol arah
// sebaliknya: berkas UTF-8 sah bernama mirip — NUL terisolasi di
// variabel tak-terkelola, toleransi B-507 — TIDAK tertolak oleh
// detektor baru; Configure Hermes berhasil dan baris warisan utuh.
func TestB522EnvUTF8NULIsolasiTidakTertolak(t *testing.T) {
	home := t.TempDir()
	envPath := filepath.Join(home, ".hermes", ".env")
	tulisBerkasPRD(t, envPath, []byte("LAIN=a\x00b\nOPENAI_API_KEY=GANTI_lama\n"))

	if err := terapkanPRD(t, home, "hermes", "GANTI_kunci-prd"); err != nil {
		t.Fatalf("applyToolSpecificConfig hermes menolak .env UTF-8 ber-NUL terisolasi: %v", err)
	}
	konten := bacaBerkasUji(t, envPath)
	if !strings.Contains(konten, "LAIN=a\x00b") {
		t.Errorf("baris warisan ber-NUL tidak utuh:\n%q", konten)
	}
	if !strings.Contains(konten, "OPENAI_API_KEY=GANTI_kunci-prd") {
		t.Errorf("kunci terkelola tidak tertulis:\n%q", konten)
	}
}

// periksaDokumenPenandaB523 menegaskan hasil Configure atas config
// berpenanda "...": terurai sebagai SATU dokumen utuh, kunci lama
// tak-terkelola tetap di dalam dokumen, dan penanda tetap menjadi
// baris penutup (kunci terkelola tertulis SEBELUM penanda).
func periksaDokumenPenandaB523(t *testing.T, jalur, kunciTerkelola, nilaiTerkelola string) {
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
	if got := doc[kunciTerkelola]; got != nilaiTerkelola {
		t.Errorf("%s = %v, mau %q (kunci terkelola harus efektif di dalam dokumen):\n%s",
			kunciTerkelola, got, nilaiTerkelola, konten)
	}
	baris := strings.Split(strings.TrimRight(konten, "\n"), "\n")
	if terakhir := strings.TrimSpace(baris[len(baris)-1]); terakhir != "..." {
		t.Errorf("baris terakhir = %q, mau penanda '...' tetap menutup dokumen:\n%s", terakhir, konten)
	}
}

// TestB523GoosePenandaAkhirDokumen mengunci B-523 untuk Goose:
// config satu dokumen yang diakhiri "..." (sah bagi PyYAML)
// berhasil di-Configure — dulu gagal "hasil sunting YAML tidak
// terurai" karena kunci terkelola ditempel sesudah penanda.
func TestB523GoosePenandaAkhirDokumen(t *testing.T) {
	home := t.TempDir()
	jalur := filepath.Join(home, ".config", "goose", "config.yaml")
	tulisBerkasPRD(t, jalur, []byte("lainnya: 1\n...\n"))

	if err := terapkanPRD(t, home, "goose", ""); err != nil {
		t.Fatalf("applyToolSpecificConfig goose atas config berpenanda '...': %v", err)
	}
	periksaDokumenPenandaB523(t, jalur, "GOOSE_MODEL", "model-baru")
}

// TestB523AiderPenandaAkhirDokumen mengunci B-523 untuk Aider
// (penyunting yang sama, pemuat PyYAML yang sama).
func TestB523AiderPenandaAkhirDokumen(t *testing.T) {
	home := t.TempDir()
	jalur := filepath.Join(home, ".aider.conf.yml")
	tulisBerkasPRD(t, jalur, []byte("lainnya: 1\n...\n"))

	if err := terapkanPRD(t, home, "aider", ""); err != nil {
		t.Fatalf("applyToolSpecificConfig aider atas config berpenanda '...': %v", err)
	}
	periksaDokumenPenandaB523(t, jalur, "openai-api-base", urlUjiPRD)
}
