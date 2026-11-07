package cliconfig

// fase21_prd_test.go — pengunci Fase 21 PR-D (cliconfig):
//
//	B-494 tiga pembaca FIFO tersisa (cli-env.sh, .bashrc,
//	      GetExportScript) disamakan ke penjaga berkas-reguler
//	      B-484 — ditolak bernama, tidak menggantung.
//	B-497 penolakan YAML multi-dokumen untuk Goose & Aider
//	      (menggeneralisasi penjaga Hermes B-485).
//	B-505 pemindai TOML melacak tabel inline lintas-baris.
//	B-506 penggabungan blok model: Hermes mempertahankan
//	      jangkar dari blok mana pun, bukan hanya blok pertama.
//	B-507 toleransi NUL .env dipersempit ke baris terkelola,
//	      disamakan ke python-dotenv terbukti.
//
// Jalur nyata yang dipanggil: Manager.syncGlobalEnvScript (dipanggil
// Configure pada setiap penerapan), Manager.GetExportScript (jalur
// API), applyToolSpecificConfig per tool (terapkanPRD), dan
// susunBlokModelHermes + verifikasiHasilHermes untuk B-506.

import (
	"os"
	"path/filepath"
	"strings"
	"syscall"
	"testing"
	"time"
)

// harusSelesai menjalankan f dengan batas waktu: pembaca FIFO yang
// menggantung adalah kegagalan test, bukan gantungan suite.
func harusSelesai(t *testing.T, nama string, f func() error) error {
	t.Helper()
	selesai := make(chan error, 1)
	go func() { selesai <- f() }()
	select {
	case err := <-selesai:
		return err
	case <-time.After(10 * time.Second):
		t.Fatalf("%s menggantung >10 detik — pembaca FIFO belum berpenjaga", nama)
		return nil
	}
}

func buatFIFO(t *testing.T, jalur string) {
	t.Helper()
	if err := os.MkdirAll(filepath.Dir(jalur), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := syscall.Mkfifo(jalur, 0o600); err != nil {
		t.Fatalf("membuat FIFO uji: %v", err)
	}
}

func manajerDanDef(t *testing.T, toolID string) (*Manager, ToolDef) {
	t.Helper()
	def, ok := cariToolDef(toolID)
	if !ok {
		t.Fatalf("tool %s tidak ditemukan di registri", toolID)
	}
	return manajerUjiToolBaru(newMockSettingsStore()), def
}

// TestB494FIFOCliEnvDitolakCepat mengunci situs syncGlobalEnvScript:
// FIFO di ~/.routex/cli-env.sh — berkas milik Route-X sendiri yang
// dibaca pada SETIAP Configure — ditolak bernama dan cepat.
func TestB494FIFOCliEnvDitolakCepat(t *testing.T) {
	home := t.TempDir()
	buatFIFO(t, filepath.Join(home, ".routex", "cli-env.sh"))
	mgr, def := manajerDanDef(t, "opencode")

	err := harusSelesai(t, "syncGlobalEnvScript atas FIFO cli-env.sh", func() error {
		return mgr.syncGlobalEnvScript(home, def, map[string]string{"OPENCODE_API_KEY": "GANTI_kunci"})
	})
	if err == nil {
		t.Fatal("syncGlobalEnvScript melaporkan sukses atas FIFO cli-env.sh")
	}
	if !strings.Contains(err.Error(), "reguler") {
		t.Errorf("pesan galat tidak menamai sebabnya: %v", err)
	}
}

// TestB494FIFOBashrcDitolakCepat mengunci situs pasangHook: FIFO di
// ~/.bashrc menggantungkan Configure lewat ensureShellAutoLoad;
// kini ditolak bernama dan cepat.
func TestB494FIFOBashrcDitolakCepat(t *testing.T) {
	home := t.TempDir()
	buatFIFO(t, filepath.Join(home, ".bashrc"))
	mgr, def := manajerDanDef(t, "opencode")

	err := harusSelesai(t, "syncGlobalEnvScript atas FIFO .bashrc", func() error {
		return mgr.syncGlobalEnvScript(home, def, map[string]string{"OPENCODE_API_KEY": "GANTI_kunci"})
	})
	if err == nil {
		t.Fatal("syncGlobalEnvScript melaporkan sukses atas FIFO .bashrc")
	}
	if !strings.Contains(err.Error(), "reguler") {
		t.Errorf("pesan galat tidak menamai sebabnya: %v", err)
	}
}

// TestB494FIFOGetExportScript mengunci situs GetExportScript:
// handler API memanggilnya per permintaan; FIFO di cli-env.sh kini
// menjadi galat bernama, bukan gantungan per permintaan.
func TestB494FIFOGetExportScript(t *testing.T) {
	home := t.TempDir()
	buatFIFO(t, filepath.Join(home, ".routex", "cli-env.sh"))
	mgr, _ := manajerDanDef(t, "opencode")

	var err error
	harusSelesai(t, "GetExportScript atas FIFO cli-env.sh", func() error {
		_, err = mgr.GetExportScript(home)
		return nil
	})
	if err == nil {
		t.Fatal("GetExportScript melaporkan sukses atas FIFO cli-env.sh")
	}
	if !strings.Contains(err.Error(), "reguler") {
		t.Errorf("pesan galat tidak menamai sebabnya: %v", err)
	}
}

// TestB497GooseMultiDokumenDitolak mengunci B-497 untuk Goose:
// config dua dokumen ditolak jujur lewat jalur apply sungguhan
// dan berkas utuh.
func TestB497GooseMultiDokumenDitolak(t *testing.T) {
	home := t.TempDir()
	jalur := filepath.Join(home, ".config", "goose", "config.yaml")
	awal := "GOOSE_PROVIDER: openai\n---\nlainnya: 1\n"
	tulisBerkasPRD(t, jalur, []byte(awal))

	err := terapkanPRD(t, home, "goose", "")
	if err == nil {
		t.Fatal("applyToolSpecificConfig goose melaporkan sukses atas config multi-dokumen")
	}
	if !strings.Contains(err.Error(), "dokumen") {
		t.Errorf("pesan galat tidak menamai multi-dokumen: %v", err)
	}
	if konten := bacaBerkasUji(t, jalur); konten != awal {
		t.Errorf("berkas berubah sesudah penolakan:\n%s", konten)
	}
}

// TestB497AiderMultiDokumenDitolak mengunci B-497 untuk Aider
// (pemuat PyYAML safe_load, terbukti menolak multi-dokumen).
func TestB497AiderMultiDokumenDitolak(t *testing.T) {
	home := t.TempDir()
	jalur := filepath.Join(home, ".aider.conf.yml")
	awal := "openai-api-base: http://lama.test/v1\n---\nlainnya: 1\n"
	tulisBerkasPRD(t, jalur, []byte(awal))

	err := terapkanPRD(t, home, "aider", "")
	if err == nil {
		t.Fatal("applyToolSpecificConfig aider melaporkan sukses atas config multi-dokumen")
	}
	if !strings.Contains(err.Error(), "dokumen") {
		t.Errorf("pesan galat tidak menamai multi-dokumen: %v", err)
	}
	if konten := bacaBerkasUji(t, jalur); konten != awal {
		t.Errorf("berkas berubah sesudah penolakan:\n%s", konten)
	}
}

// TestB505TOMLInlineLintasBaris mengunci B-505: tabel inline
// lintas-baris di preamble Codex — baris `model = ...` DI DALAM
// tabel adalah isi nilai, bukan kunci tingkat-atas. Penyunting
// mengganti model tingkat-atas sungguhan dan membiarkan isi tabel
// apa adanya; sebelum perbaikan isi tabel yang diganti dan model
// asli terbuang sebagai "duplikat" (Configure menolak config sah).
func TestB505TOMLInlineLintasBaris(t *testing.T) {
	home := t.TempDir()
	jalur := filepath.Join(home, ".codex", "config.toml")
	awal := "cfg = {\n  model = \"di-dalam-inline\",\n  lain = 1\n}\nmodel = \"model-lama\"\n"
	tulisBerkasPRD(t, jalur, []byte(awal))

	if err := terapkanPRD(t, home, "codex", ""); err != nil {
		t.Fatalf("applyToolSpecificConfig codex menolak config sah bertabel-inline lintas-baris: %v", err)
	}
	konten := bacaBerkasUji(t, jalur)
	if !strings.Contains(konten, "model = \"di-dalam-inline\"") {
		t.Errorf("isi tabel inline ikut terganti/hilang:\n%s", konten)
	}
	if !strings.Contains(konten, "model = \"model-baru\"") {
		t.Errorf("model tingkat-atas tidak diperbarui:\n%s", konten)
	}
	if n := strings.Count(konten, "model = \"model-lama\""); n != 0 {
		t.Errorf("model lama tingkat-atas masih ada (%d):\n%s", n, konten)
	}
}

// TestB506JangkarBlokKedua mengunci B-506: jangkar yang hanya ada
// di blok `model:` KEDUA hidup di header blok gabungan, supaya
// alias di luar blok tetap terurai dan verifikasi lolos. Sebelum
// perbaikan hanya jangkar blok pertama yang dipertahankan.
func TestB506JangkarBlokKedua(t *testing.T) {
	lama := "model:\n" +
		"  default: lama-1\n" +
		"model: &kedua\n" +
		"  default: lama-2\n" +
		"  catatan: tetap\n" +
		"rujukan: *kedua\n"
	hasil := susunBlokModelHermes(lama, "model-baru", urlUjiPRD, "GANTI_kunci-prd")

	if !strings.Contains(hasil, "model: &kedua") {
		t.Errorf("jangkar blok kedua tidak dipertahankan di blok gabungan:\n%s", hasil)
	}
	if err := verifikasiHasilHermes(hasil, "model-baru", urlUjiPRD, "GANTI_kunci-prd"); err != nil {
		t.Errorf("verifikasi hasil berjangkar blok-kedua: %v\n%s", err, hasil)
	}
	terurai, err := uraiYAMLHermes(hasil)
	if err != nil {
		t.Fatalf("hasil tidak terurai (alias menggantung?): %v\n%s", err, hasil)
	}
	rujukan, ok := terurai["rujukan"].(map[string]any)
	if !ok {
		t.Fatalf("alias *kedua tidak terurai menjadi peta model: %v", terurai["rujukan"])
	}
	if rujukan["default"] != "model-baru" {
		t.Errorf("alias menunjuk default %v, mau model-baru (blok gabungan)", rujukan["default"])
	}
	if rujukan["catatan"] != "tetap" {
		t.Errorf("sub-kunci tak-terkelola dari blok kedua hilang: %v", rujukan)
	}
}

// TestB507NULVariabelLainDitoleransi mengunci B-507 dua arah:
// NUL warisan di variabel TAK-terkelola ditoleransi (python-dotenv
// terbukti membacanya utuh), NUL pada baris TERKELOLA tetap galat
// (jejak pengkodean pada hasil garapan, kelas B-477).
func TestB507NULVariabelLainDitoleransi(t *testing.T) {
	if err := verifikasiEnvHermes("LAIN=a\x00b\nOPENAI_API_KEY=nilai\n", "OPENAI_API_KEY"); err != nil {
		t.Errorf("NUL di variabel lain ditolak, mau ditoleransi seperti python-dotenv: %v", err)
	}
	if err := verifikasiEnvHermes("OPENAI_API_KEY=a\x00b\n", "OPENAI_API_KEY"); err == nil {
		t.Error("NUL pada baris terkelola diterima, mau galat")
	}
}

// TestB507HermesEnvNULWarisanLolos adalah pasangan ujung-ke-ujung
// B-507: apply Hermes atas .env yang memuat NUL di variabel lain
// berhasil, baris warisan itu utuh, dan kunci terkelola tertulis.
func TestB507HermesEnvNULWarisanLolos(t *testing.T) {
	home := t.TempDir()
	envPath := filepath.Join(home, ".hermes", ".env")
	tulisBerkasPRD(t, envPath, []byte("LAIN=a\x00b\nOPENAI_API_KEY=lama\n"))

	if err := terapkanPRD(t, home, "hermes", "GANTI_kunci-prd"); err != nil {
		t.Fatalf("applyToolSpecificConfig hermes menolak .env ber-NUL warisan: %v", err)
	}
	konten := bacaBerkasUji(t, envPath)
	if !strings.Contains(konten, "LAIN=a\x00b") {
		t.Errorf("baris warisan ber-NUL tidak utuh:\n%q", konten)
	}
	if !strings.Contains(konten, "OPENAI_API_KEY=GANTI_kunci-prd") {
		t.Errorf("kunci terkelola tidak tertulis:\n%q", konten)
	}
}
