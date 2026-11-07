package cliconfig

import (
	"context"
	"os"
	"path/filepath"
	"strings"
	"testing"

	toml "github.com/pelletier/go-toml/v2"
	yaml "go.yaml.in/yaml/v3"
)

// Berkas ini memuat test pengunci Fase 19 kelompok CLICONFIG
// (B-439…B-444): penyunting teks per-baris yang melaporkan sukses
// atas bentuk sah yang tidak ia kenali. Setiap test memakai
// permukaan yang sudah ada sebelum Fase 19 (penyunting per format,
// Configure, redaksi skrip) dan membuktikan hasil akhir dengan
// parser sungguhan (go-toml / yaml.v3) — nilai EFEKTIF sesudah
// urai silang, bukan sekadar bentuk teks keluaran.

func uraiTOMLUji(t *testing.T, konten string) map[string]any {
	t.Helper()
	var doc map[string]any
	if err := toml.Unmarshal([]byte(konten), &doc); err != nil {
		t.Fatalf("hasil tidak terurai sebagai TOML: %v\n%s", err, konten)
	}
	return doc
}

func uraiYAMLUji(t *testing.T, konten string) map[string]any {
	t.Helper()
	var doc map[string]any
	if err := yaml.Unmarshal([]byte(konten), &doc); err != nil {
		t.Fatalf("hasil tidak terurai sebagai YAML: %v\n%s", err, konten)
	}
	return doc
}

// TestB439TOMLMultilinePreambleCodex adalah pengunci B-439 bentuk
// (1)+(2) probe W5-K1: preamble Codex memuat string multiline yang
// baris isinya mirip kunci terkelola DAN header seksi. Dulu baris
// isi itu ditulis ulang, `[palsu]` memotong preamble, dan pasangan
// baru disisipkan KE DALAM string — kunci tingkat-atas asli tidak
// pernah diperbarui sementara isi pengguna rusak.
func TestB439TOMLMultilinePreambleCodex(t *testing.T) {
	home := t.TempDir()
	t.Setenv("HOME", home)
	dir := filepath.Join(home, ".codex")
	if err := os.MkdirAll(dir, 0o755); err != nil {
		t.Fatal(err)
	}
	awal := "instructions = \"\"\"\n" +
		"model = \"jangan-sentuh\"\n" +
		"[palsu]\n" +
		"\"\"\"\n" +
		"model_provider = \"lama\"\n" +
		"model = \"model-lama\"\n"
	jalur := filepath.Join(dir, "config.toml")
	if err := os.WriteFile(jalur, []byte(awal), 0o644); err != nil {
		t.Fatal(err)
	}
	def, ok := cariToolDef("codex")
	if !ok {
		t.Fatal("codex tidak ditemukan di registri")
	}
	mgr := manajerUjiToolBaru(newMockSettingsStore())
	if err := mgr.applyToolSpecificConfig(context.Background(), home, def, ModeModelOnly, "model-baru", "http://localhost:8080/v1", ""); err != nil {
		t.Fatalf("applyToolSpecificConfig codex: %v", err)
	}
	konten := bacaBerkasUji(t, jalur)
	doc := uraiTOMLUji(t, konten)
	if doc["model"] != "model-baru" {
		t.Errorf("model tingkat-atas = %v, mau model-baru:\n%s", doc["model"], konten)
	}
	if doc["model_provider"] != "routex" {
		t.Errorf("model_provider = %v, mau routex:\n%s", doc["model_provider"], konten)
	}
	isi, _ := doc["instructions"].(string)
	if !strings.Contains(isi, "model = \"jangan-sentuh\"") || !strings.Contains(isi, "[palsu]") {
		t.Errorf("isi string multiline pengguna rusak: %q\n%s", isi, konten)
	}
	seksi, _ := doc["model_providers"].(map[string]any)
	routex, _ := seksi["routex"].(map[string]any)
	if routex["base_url"] != "http://localhost:8080/v1" {
		t.Errorf("model_providers.routex.base_url = %v:\n%s", routex["base_url"], konten)
	}
}

// TestB439TOMLMultilineDiDalamSeksi adalah pengunci B-439 bentuk
// (3) probe W5-K1: di dalam seksi [model.routex] Grok, baris isi
// string multiline yang mirip kunci dulu diganti dan baris kunci
// ASLI sesudahnya dibuang sebagai duplikat — seksi berakhir tanpa
// kunci model sama sekali.
func TestB439TOMLMultilineDiDalamSeksi(t *testing.T) {
	existing := "[model.routex]\n" +
		"catatan = '''\n" +
		"model = \"jangan-sentuh\"\n" +
		"'''\n" +
		"model = \"model-lama\"\n"
	hasil := perbaruiKunciSeksiTOML(existing, "model.routex", [][2]string{
		{"model", "model-baru"},
		{"name", "Route-X"},
	})
	doc := uraiTOMLUji(t, hasil)
	seksi, _ := doc["model"].(map[string]any)
	routex, _ := seksi["routex"].(map[string]any)
	if routex["model"] != "model-baru" {
		t.Errorf("model.routex.model = %v, mau model-baru:\n%s", routex["model"], hasil)
	}
	catatan, _ := routex["catatan"].(string)
	if !strings.Contains(catatan, "model = \"jangan-sentuh\"") {
		t.Errorf("isi string literal multiline rusak: %q\n%s", catatan, hasil)
	}
}

// TestB439TOMLKunciTerkelolaNilaiMultiline: kunci terkelola yang
// NILAI lamanya string multiline diganti seluruh rentangnya dengan
// satu baris kanonik — bukan hanya baris pertamanya, yang akan
// meninggalkan sisa string yatim.
func TestB439TOMLKunciTerkelolaNilaiMultiline(t *testing.T) {
	existing := "model = \"\"\"\nisi lama\n\"\"\"\nmodel_provider = \"lama\"\n"
	hasil := perbaruiPreambleTOML(existing, [][2]string{
		{"model_provider", "routex"},
		{"model", "model-baru"},
	})
	doc := uraiTOMLUji(t, hasil)
	if doc["model"] != "model-baru" || doc["model_provider"] != "routex" {
		t.Errorf("pasangan preamble tidak efektif: %v\n%s", doc, hasil)
	}
	if strings.Contains(hasil, "isi lama") {
		t.Errorf("sisa nilai multiline lama tertinggal:\n%s", hasil)
	}
}

// TestB440HermesIndentasiEmpat adalah pengunci B-440(a): sub-kunci
// blok model: berindentasi 4 (YAML sah) dulu tidak dikenali sebagai
// terkelola — tertinggal sesudah blok baru berindentasi 2 dan
// keluaran menjadi YAML yang TIDAK terurai sama sekali.
func TestB440HermesIndentasiEmpat(t *testing.T) {
	lama := "model:\n" +
		"    provider: penyedia-lama\n" +
		"    default: model-lama\n" +
		"    catatan: simpan-saya\n"
	hasil := susunBlokModelHermes(lama, "model-baru", "http://localhost:8080/v1", "GANTI_kunci-hermes")
	doc := uraiYAMLUji(t, hasil)
	blok, _ := doc["model"].(map[string]any)
	if blok["default"] != "model-baru" || blok["provider"] != "custom" {
		t.Errorf("blok model tidak efektif: %v\n%s", blok, hasil)
	}
	if blok["catatan"] != "simpan-saya" {
		t.Errorf("sub-kunci pengguna hilang: %v\n%s", blok, hasil)
	}
}

// TestB440HermesKomentarKolomNol adalah pengunci B-440(b): komentar
// kolom-0 di tengah blok dulu menghentikan pemindaian blok —
// sub-kunci terkelola sesudahnya tertinggal dan menang last-wins.
func TestB440HermesKomentarKolomNol(t *testing.T) {
	lama := "model:\n" +
		"  provider: penyedia-lama\n" +
		"# komentar pengguna di tengah blok\n" +
		"  default: model-lama\n" +
		"  catatan: simpan-saya\n" +
		"lainnya: 1\n"
	hasil := susunBlokModelHermes(lama, "model-baru", "http://localhost:8080/v1", "GANTI_kunci-hermes")
	doc := uraiYAMLUji(t, hasil)
	blok, _ := doc["model"].(map[string]any)
	if blok["default"] != "model-baru" {
		t.Errorf("default efektif = %v, mau model-baru (nilai basi menang):\n%s", blok["default"], hasil)
	}
	if strings.Contains(hasil, "model-lama") {
		t.Errorf("nilai basi tertinggal:\n%s", hasil)
	}
	harusMemuat(t, hasil, "# komentar pengguna di tengah blok")
	if blok["catatan"] != "simpan-saya" || doc["lainnya"] == nil {
		t.Errorf("isi di luar sub-kunci terkelola rusak: %v\n%s", doc, hasil)
	}
}

// TestB441HermesDuaBlokDigabung adalah pengunci B-441: blok
// `model:` tingkat-atas KEDUA dulu tidak pernah digabung — parser
// YAML membacanya menang utuh atas blok pertama yang diperbarui.
func TestB441HermesDuaBlokDigabung(t *testing.T) {
	lama := "model:\n" +
		"  provider: lama-satu\n" +
		"  default: model-satu\n" +
		"  catatan_satu: a\n" +
		"lainnya: 1\n" +
		"model:\n" +
		"  provider: lama-dua\n" +
		"  default: model-dua\n" +
		"  catatan_dua: b\n"
	hasil := susunBlokModelHermes(lama, "model-baru", "http://localhost:8080/v1", "GANTI_kunci-hermes")
	if n := strings.Count(hasil, "default:"); n != 1 {
		t.Errorf("default muncul %d kali, mau 1 (blok tidak digabung):\n%s", n, hasil)
	}
	doc := uraiYAMLUji(t, hasil)
	blok, _ := doc["model"].(map[string]any)
	if blok["default"] != "model-baru" || blok["provider"] != "custom" {
		t.Errorf("blok gabungan tidak efektif: %v\n%s", blok, hasil)
	}
	if blok["catatan_satu"] != "a" || blok["catatan_dua"] != "b" {
		t.Errorf("sub-kunci pengguna dari kedua blok harus selamat: %v\n%s", blok, hasil)
	}
	if strings.Contains(hasil, "model-satu") || strings.Contains(hasil, "model-dua") {
		t.Errorf("nilai basi dari salah satu blok tertinggal:\n%s", hasil)
	}
}

// TestB442EnvNamaTerkutip adalah pengunci B-442: nama variabel
// .env terkutip (`"OPENAI_API_KEY"=`) dulu tidak dikenali parser —
// baris lama bertahan DAN baris kanonik ditambahkan (variabel
// ganda). Kini dikenali, diganti kanonik, dan duplikat lintas
// bentuk terlipat.
func TestB442EnvNamaTerkutip(t *testing.T) {
	for _, masuk := range []string{
		"\"OPENAI_API_KEY\"=lama\n",
		"'OPENAI_API_KEY'=lama\n",
		"export \"OPENAI_API_KEY\" = lama\n",
	} {
		hasil := perbaruiBarisEnv(masuk, "OPENAI_API_KEY", "baru")
		if n := strings.Count(hasil, "OPENAI_API_KEY"); n != 1 {
			t.Errorf("masukan %q: nama muncul %d kali, mau 1:\n%q", masuk, n, hasil)
		}
		harusMemuat(t, hasil, "OPENAI_API_KEY=baru")
		if strings.Contains(hasil, "lama") {
			t.Errorf("masukan %q: nilai lama tertinggal:\n%q", masuk, hasil)
		}
	}
	// Duplikat lintas-bentuk (terkutip + polos) terlipat jadi satu.
	lama := "\"OPENAI_API_KEY\"=satu\nOPENAI_API_KEY=dua\nLAIN=1\n"
	hasil := perbaruiBarisEnv(lama, "OPENAI_API_KEY", "baru")
	if n := strings.Count(hasil, "OPENAI_API_KEY=baru"); n != 1 {
		t.Errorf("baris kanonik muncul %d kali, mau 1:\n%q", n, hasil)
	}
	if strings.Contains(hasil, "satu") || strings.Contains(hasil, "dua") {
		t.Errorf("duplikat lintas-bentuk tertinggal:\n%q", hasil)
	}
	harusMemuat(t, hasil, "LAIN=1")
}

// TestB443ConfigureKunciBernewlineDitolakBersih adalah pengunci
// B-443: Configure dengan API key ber-newline dulu gagal di tengah
// jalan (shellQuote di tulisUlangKeyEnv) dan cabang kegagalannya
// satu-satunya yang tidak memulihkan jepretan — direktori .routex
// ciptaan bocor sebagai residu. Kini kunci ditolak di pintu masuk
// dan tidak ada residu apa pun di HOME.
func TestB443ConfigureKunciBernewlineDitolakBersih(t *testing.T) {
	home := t.TempDir()
	t.Setenv("HOME", home)
	mgr := NewManager(newMockSettingsStore(), nil, nil)
	_, err := mgr.Configure(context.Background(), ConfigureParams{
		ToolID: "goose", Mode: ModeRouting, Target: "model-x",
		APIKey: "GANTI_kunci\njahat", GatewayURL: "http://localhost:8080/v1",
	}, "")
	if err == nil {
		t.Fatal("Configure melaporkan sukses untuk API key ber-newline")
	}
	if _, statErr := os.Lstat(filepath.Join(home, ".routex")); !os.IsNotExist(statErr) {
		t.Errorf("direktori .routex bocor sesudah Configure gagal (stat err=%v)", statErr)
	}
	entri, _ := os.ReadDir(home)
	if len(entri) != 0 {
		t.Errorf("HOME tidak bersih sesudah Configure gagal: %v", entri)
	}
}

// TestB444RedaksiExportTab adalah pengunci B-444: baris
// `export`+TAB dulu lolos dari redactAPIKeysInScript (tampil TAK
// tersamar via GetExportScript) dan dari removeAPIKeysFromScript
// (tidak dibersihkan syncGlobalEnvScript), karena keduanya hanya
// memotong awalan "export " satu spasi persis.
func TestB444RedaksiExportTab(t *testing.T) {
	m := manajerUjiToolBaru(newMockSettingsStore())
	skrip := "export\tOPENAI_API_KEY='GANTI_rahasia-tab'\n" +
		"export  OPENAI_API_KEY='GANTI_rahasia-duaspasi'\n" +
		"export OPENAI_API_KEY='GANTI_rahasia-kanonik'\n" +
		"VARIABEL_LAIN='biarkan'\n"
	hasil := m.redactAPIKeysInScript(skrip)
	for _, rahasia := range []string{"GANTI_rahasia-tab", "GANTI_rahasia-duaspasi", "GANTI_rahasia-kanonik"} {
		if strings.Contains(hasil, rahasia) {
			t.Errorf("kunci %s masih plaintext sesudah redaksi:\n%s", rahasia, hasil)
		}
	}
	harusMemuat(t, hasil, "VARIABEL_LAIN='biarkan'")
	bersih := m.removeAPIKeysFromScript(skrip)
	if strings.Contains(bersih, "OPENAI_API_KEY") {
		t.Errorf("removeAPIKeysFromScript meninggalkan baris kunci:\n%s", bersih)
	}
	harusMemuat(t, bersih, "VARIABEL_LAIN='biarkan'")
}

// TestVerifikasiHasilAkhirMenolakYAMLGanda membuktikan titik
// verifikasi struktural Fase 19: berkas Goose yang memuat kunci
// tingkat-atas ganda MILIK PENGGUNA (di luar kunci terkelola) dulu
// ditulis ulang apa adanya dan Configure melaporkan sukses atas
// YAML yang tidak terurai. Kini applyToolSpecificConfig menolak
// secara jujur dan berkas lama tidak berubah.
func TestVerifikasiHasilAkhirMenolakYAMLGanda(t *testing.T) {
	home := t.TempDir()
	t.Setenv("HOME", home)
	dir := filepath.Join(home, ".config", "goose")
	if err := os.MkdirAll(dir, 0o755); err != nil {
		t.Fatal(err)
	}
	awal := "GOOSE_PROVIDER: lama\nekstensi_saya: 1\nekstensi_saya: 2\n"
	jalur := filepath.Join(dir, "config.yaml")
	if err := os.WriteFile(jalur, []byte(awal), 0o644); err != nil {
		t.Fatal(err)
	}
	def, ok := cariToolDef("goose")
	if !ok {
		t.Fatal("goose tidak ditemukan di registri")
	}
	mgr := manajerUjiToolBaru(newMockSettingsStore())
	if err := mgr.applyToolSpecificConfig(context.Background(), home, def, ModeModelOnly, "model-baru", "http://localhost:8080/v1", ""); err == nil {
		t.Fatalf("applyToolSpecificConfig melaporkan sukses atas YAML berkunci ganda; berkas kini:\n%s", bacaBerkasUji(t, jalur))
	}
	if konten := bacaBerkasUji(t, jalur); konten != awal {
		t.Errorf("berkas berubah sesudah penolakan jujur:\n%s", konten)
	}
}

// TestVerifikasiHasilAkhirMenolakTOMLRusak membuktikan titik
// verifikasi yang sama untuk TOML: config Codex dengan seksi ganda
// milik pengguna (TOML tak sah) dulu tetap ditulis dan dilaporkan
// sukses; kini ditolak jujur dan berkas tidak berubah.
func TestVerifikasiHasilAkhirMenolakTOMLRusak(t *testing.T) {
	home := t.TempDir()
	t.Setenv("HOME", home)
	dir := filepath.Join(home, ".codex")
	if err := os.MkdirAll(dir, 0o755); err != nil {
		t.Fatal(err)
	}
	awal := "[lain]\na = 1\n[lain]\nb = 2\n"
	jalur := filepath.Join(dir, "config.toml")
	if err := os.WriteFile(jalur, []byte(awal), 0o644); err != nil {
		t.Fatal(err)
	}
	def, ok := cariToolDef("codex")
	if !ok {
		t.Fatal("codex tidak ditemukan di registri")
	}
	mgr := manajerUjiToolBaru(newMockSettingsStore())
	if err := mgr.applyToolSpecificConfig(context.Background(), home, def, ModeModelOnly, "model-baru", "http://localhost:8080/v1", ""); err == nil {
		t.Fatalf("applyToolSpecificConfig melaporkan sukses atas TOML berseksi ganda; berkas kini:\n%s", bacaBerkasUji(t, jalur))
	}
	if konten := bacaBerkasUji(t, jalur); konten != awal {
		t.Errorf("berkas berubah sesudah penolakan jujur:\n%s", konten)
	}
}
