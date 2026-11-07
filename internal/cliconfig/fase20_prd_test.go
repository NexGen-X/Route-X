package cliconfig

// fase20_prd_test.go — pengunci Fase 20 PR-D (cliconfig
// B-477…B-485): kelas sukses-bohong lanjutan — pengkodean yang
// tidak bisa dibaca tool, angka JSON yang bergeser senyap, array
// TOML lintas-baris, header YAML berjangkar/flow, toleransi yang
// tidak sama dengan parser tujuan, berkas khusus, dan dokumen
// ganda. Semantik parser tujuan dibuktikan dengan pustaka
// sungguhan saat fase berjalan (PyYAML: multi-dokumen ditolak,
// kunci ganda di luar blok diterima last-wins, jangkar/alias sah;
// python-dotenv: duplikat diterima last-wins, UTF-16 gagal total
// dengan UnicodeDecodeError).

import (
	"context"
	"os"
	"path/filepath"
	"strings"
	"syscall"
	"testing"
)

const urlUjiPRD = "http://localhost:8080/v1"

func utf16LE(teks string) []byte {
	out := []byte{0xFF, 0xFE}
	for _, r := range teks {
		out = append(out, byte(r), byte(r>>8))
	}
	return out
}

func tulisBerkasPRD(t *testing.T, jalur string, isi []byte) {
	t.Helper()
	if err := os.MkdirAll(filepath.Dir(jalur), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(jalur, isi, 0o600); err != nil {
		t.Fatal(err)
	}
}

func terapkanPRD(t *testing.T, home, toolID, apiKey string) error {
	t.Helper()
	def, ok := cariToolDef(toolID)
	if !ok {
		t.Fatalf("tool %s tidak ditemukan di registri", toolID)
	}
	mgr := manajerUjiToolBaru(newMockSettingsStore())
	return mgr.applyToolSpecificConfig(context.Background(), home, def, ModeModelOnly, "model-baru", urlUjiPRD, apiKey)
}

// TestB477EnvUTF16DitolakJujur: .env Hermes berkode UTF-16LE
// ditolak SEBELUM berkas apa pun ditulis — jalur lama menghasilkan
// berkas yang bukan UTF-8 sah maupun UTF-16 sah sambil sukses.
func TestB477EnvUTF16DitolakJujur(t *testing.T) {
	home := t.TempDir()
	envPath := filepath.Join(home, ".hermes", ".env")
	asli := utf16LE("OPENAI_API_KEY=lama\n")
	tulisBerkasPRD(t, envPath, asli)

	err := terapkanPRD(t, home, "hermes", "GANTI_kunci-prd")
	if err == nil {
		t.Fatal("applyToolSpecificConfig melaporkan sukses atas .env UTF-16")
	}
	if !strings.Contains(err.Error(), "UTF-16") {
		t.Errorf("pesan galat tidak menamai pengkodeannya: %v", err)
	}
	if _, statErr := os.Lstat(filepath.Join(home, ".hermes", "config.yaml")); !os.IsNotExist(statErr) {
		t.Errorf("config.yaml ikut tertulis sesudah penolakan (stat err=%v)", statErr)
	}
	mentah, _ := os.ReadFile(envPath)
	if string(mentah) != string(asli) {
		t.Error(".env berubah sesudah penolakan jujur")
	}
}

// TestB478JSONAngkaBesarUtuh: angka di luar presisi float64 pada
// berkas JSON pengguna berjalan pulang-pergi APA ADANYA lewat
// Configure Claude (dulu 9007199254740993 menjadi …992 menetap).
func TestB478JSONAngkaBesarUtuh(t *testing.T) {
	home := t.TempDir()
	jalur := filepath.Join(home, ".claude", "settings.json")
	tulisBerkasPRD(t, jalur, []byte(`{"lainnya": 9007199254740993, "sarang": {"id": 123456789012345678901234567890}}`))

	if err := terapkanPRD(t, home, "claude", ""); err != nil {
		t.Fatalf("applyToolSpecificConfig claude: %v", err)
	}
	konten := bacaBerkasUji(t, jalur)
	harusMemuat(t, konten, "9007199254740993")
	harusMemuat(t, konten, "123456789012345678901234567890")
	if strings.Contains(konten, "9007199254740992") {
		t.Errorf("angka besar bergeser di keluaran:\n%s", konten)
	}
}

// TestB479ArrayMultilineTOML: baris isi array TIDAK dibaca sebagai
// header/kunci, dan nilai array pada kunci terkelola diganti
// sebagai SATU rentang utuh.
func TestB479ArrayMultilineTOML(t *testing.T) {
	// (a) Preamble: array berisi baris mirip header dan kunci.
	awal := "daftar = [\n" +
		"  \"[palsu]\",\n" +
		"  \"model = \\\"jangan\\\"\",\n" +
		"]\n" +
		"model = \"lama\"\n"
	hasil := perbaruiPreambleTOML(awal, [][2]string{{"model", "model-baru"}})
	doc := uraiTOMLUji(t, hasil)
	if doc["model"] != "model-baru" {
		t.Errorf("model = %v, mau model-baru:\n%s", doc["model"], hasil)
	}
	daftar, _ := doc["daftar"].([]any)
	if len(daftar) != 2 || daftar[0] != "[palsu]" {
		t.Errorf("isi array preamble rusak: %v\n%s", doc["daftar"], hasil)
	}
	if _, ada := doc["palsu"]; ada {
		t.Errorf("baris isi array menjadi seksi sungguhan:\n%s", hasil)
	}

	// (b) Nilai array kunci terkelola diganti seluruh rentangnya.
	awal2 := "[model.routex]\n" +
		"model = [\n  \"a\",\n  \"b\",\n]\n" +
		"name = \"simpan\"\n"
	hasil2 := perbaruiKunciSeksiTOML(awal2, "model.routex", [][2]string{{"model", "model-baru"}})
	doc2 := uraiTOMLUji(t, hasil2)
	routex, _ := doc2["model"].(map[string]any)["routex"].(map[string]any)
	if routex["model"] != "model-baru" || routex["name"] != "simpan" {
		t.Errorf("penggantian rentang array gagal: %v\n%s", routex, hasil2)
	}

	// (c) Seksi tidak berakhir di baris kurung di dalam array
	// bersarang; kunci sesudah array tetap milik seksinya.
	awal3 := "[model.routex]\n" +
		"daftar = [\n  [1, 2],\n  [3, 4],\n]\n" +
		"model = \"lama\"\n" +
		"[lain]\na = 1\n"
	hasil3 := perbaruiKunciSeksiTOML(awal3, "model.routex", [][2]string{{"model", "model-baru"}})
	doc3 := uraiTOMLUji(t, hasil3)
	routex3, _ := doc3["model"].(map[string]any)["routex"].(map[string]any)
	if routex3["model"] != "model-baru" {
		t.Errorf("model di seksi berarray bersarang = %v:\n%s", routex3["model"], hasil3)
	}
	if d, _ := routex3["daftar"].([]any); len(d) != 2 {
		t.Errorf("array bersarang rusak: %v\n%s", routex3["daftar"], hasil3)
	}
}

// TestB480GrokBOMDiterima: berkas Grok ber-BOM kini diterima
// (BOM dibuang di jalur seksi juga), sama seperti jalur Codex.
func TestB480GrokBOMDiterima(t *testing.T) {
	home := t.TempDir()
	jalur := filepath.Join(home, ".grok", "config.toml")
	tulisBerkasPRD(t, jalur, []byte("\ufeff[models]\ndefault = \"lama\"\n"))

	if err := terapkanPRD(t, home, "grok-build", ""); err != nil {
		t.Fatalf("applyToolSpecificConfig grok-build atas berkas ber-BOM: %v", err)
	}
	konten := bacaBerkasUji(t, jalur)
	if strings.HasPrefix(konten, "\ufeff") {
		t.Errorf("BOM bertahan di keluaran:\n%q", konten)
	}
	doc := uraiTOMLUji(t, konten)
	models, _ := doc["models"].(map[string]any)
	if models["default"] != "routex" {
		t.Errorf("models.default = %v, mau routex:\n%s", models["default"], konten)
	}
}

// TestB481HermesJangkarDipertahankan: header `model: &m` dikenali
// sebagai blok peta (bukan skalar), jangkarnya dipertahankan pada
// blok gabungan, dan alias ke jangkar itu tetap terurai.
func TestB481HermesJangkarDipertahankan(t *testing.T) {
	lama := "model: &m\n" +
		"  default: model-lama\n" +
		"  catatan: simpan-saya\n" +
		"rujukan: *m\n"
	hasil := susunBlokModelHermes(lama, "model-baru", urlUjiPRD, "GANTI_kunci-prd")
	harusMemuat(t, hasil, "model: &m")
	doc := uraiYAMLUji(t, hasil)
	blok, _ := doc["model"].(map[string]any)
	if blok["default"] != "model-baru" || blok["catatan"] != "simpan-saya" {
		t.Errorf("blok berjangkar tidak tergabung benar: %v\n%s", blok, hasil)
	}
	rujukan, _ := doc["rujukan"].(map[string]any)
	if rujukan["default"] != "model-baru" {
		t.Errorf("alias *m tidak menunjuk blok yang diperbarui: %v\n%s", rujukan, hasil)
	}
	if err := verifikasiHasilHermes(hasil, "model-baru", urlUjiPRD, "GANTI_kunci-prd"); err != nil {
		t.Errorf("verifikasi hasil berjangkar: %v", err)
	}
}

// TestB482HermesFlowMapDipertahankan: entri tak-terkelola flow map
// — termasuk string berkoma — selamat menjadi baris blok; entri
// terkelola ditulis ulang kanonis.
func TestB482HermesFlowMapDipertahankan(t *testing.T) {
	lama := `model: {default: model-lama, provider: penyedia-lain, catatan: "x, y", batas: 3}` + "\n" +
		"lainnya: 1\n"
	hasil := susunBlokModelHermes(lama, "model-baru", urlUjiPRD, "GANTI_kunci-prd")
	doc := uraiYAMLUji(t, hasil)
	blok, _ := doc["model"].(map[string]any)
	if blok["default"] != "model-baru" || blok["provider"] != "custom" {
		t.Errorf("entri terkelola flow map tidak diperbarui: %v\n%s", blok, hasil)
	}
	if blok["catatan"] != "x, y" {
		t.Errorf("entri string berkoma hilang/berubah: %v\n%s", blok, hasil)
	}
	if blok["batas"] != 3 {
		t.Errorf("entri angka tak-terkelola hilang: %v\n%s", blok, hasil)
	}
	if doc["lainnya"] != 1 {
		t.Errorf("isi di luar blok rusak: %v\n%s", doc, hasil)
	}
}

// TestB483HermesToleransiParserTujuan: kunci ganda di LUAR blok
// terkelola diterima verifikasi Hermes (PyYAML last-wins), kunci
// ganda DI DALAM blok tetap ditolak; duplikat variabel
// tak-terkelola di .env tidak menggagalkan Configure.
func TestB483HermesToleransiParserTujuan(t *testing.T) {
	blokSah := "model:\n" +
		"  provider: custom\n" +
		"  default: model-baru\n" +
		"  base_url: " + urlUjiPRD + "\n" +
		"  api_mode: chat_completions\n" +
		"  api_key: GANTI_kunci-prd\n"
	if err := verifikasiHasilHermes("lainnya: 1\nlainnya: 2\n"+blokSah, "model-baru", urlUjiPRD, "GANTI_kunci-prd"); err != nil {
		t.Errorf("kunci ganda di luar blok ditolak, mau ditoleransi last-wins PyYAML: %v", err)
	}
	dalamBlok := "model:\n" +
		"  provider: custom\n  provider: custom\n" +
		"  default: model-baru\n" +
		"  base_url: " + urlUjiPRD + "\n" +
		"  api_mode: chat_completions\n" +
		"  api_key: GANTI_kunci-prd\n"
	if err := verifikasiHasilHermes(dalamBlok, "model-baru", urlUjiPRD, "GANTI_kunci-prd"); err == nil {
		t.Error("kunci ganda DI DALAM blok model diterima, mau tetap ditolak")
	}

	home := t.TempDir()
	tulisBerkasPRD(t, filepath.Join(home, ".hermes", ".env"),
		[]byte("LAIN=satu\nLAIN=dua\n"))
	if err := terapkanPRD(t, home, "hermes", "GANTI_kunci-prd"); err != nil {
		t.Fatalf("Configure Hermes dengan duplikat tak-terkelola di .env: %v", err)
	}
	env := bacaBerkasUji(t, filepath.Join(home, ".hermes", ".env"))
	harusMemuat(t, env, "LAIN=satu")
	harusMemuat(t, env, "LAIN=dua")
	harusMemuat(t, env, "OPENAI_API_KEY=GANTI_kunci-prd")
}

// TestB484BerkasKhususDitolakTanpaMenggantung: FIFO sebagai berkas
// konfigurasi tidak dibaca (ReadFile-nya menggantung tanpa batas)
// dan tidak ditimpa rename — Configure gagal bernama, cepat.
func TestB484BerkasKhususDitolakTanpaMenggantung(t *testing.T) {
	home := t.TempDir()
	jalur := filepath.Join(home, ".codex", "config.toml")
	if err := os.MkdirAll(filepath.Dir(jalur), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := syscall.Mkfifo(jalur, 0o600); err != nil {
		t.Fatalf("membuat FIFO uji: %v", err)
	}
	if j := jepretBerkasConfigure(jalur); !j.takTersentuh {
		t.Error("jepretan FIFO tidak ditandai takTersentuh — jalur baca bisa menggantung")
	}
	if err := terapkanPRD(t, home, "codex", ""); err == nil {
		t.Fatal("applyToolSpecificConfig melaporkan sukses atas FIFO")
	} else if !strings.Contains(err.Error(), "reguler") {
		t.Errorf("pesan galat tidak menamai sebabnya: %v", err)
	}
}

// TestB485HermesMultiDokumenDitolak: config.yaml dua dokumen
// ditolak jujur (PyYAML safe_load menolaknya) dan berkas utuh.
func TestB485HermesMultiDokumenDitolak(t *testing.T) {
	home := t.TempDir()
	jalur := filepath.Join(home, ".hermes", "config.yaml")
	awal := "model:\n  default: model-lama\n---\nlainnya: 1\n"
	tulisBerkasPRD(t, jalur, []byte(awal))

	err := terapkanPRD(t, home, "hermes", "GANTI_kunci-prd")
	if err == nil {
		t.Fatal("applyToolSpecificConfig melaporkan sukses atas config multi-dokumen")
	}
	if !strings.Contains(err.Error(), "dokumen") {
		t.Errorf("pesan galat tidak menamai multi-dokumen: %v", err)
	}
	if konten := bacaBerkasUji(t, jalur); konten != awal {
		t.Errorf("berkas berubah sesudah penolakan:\n%s", konten)
	}
}
