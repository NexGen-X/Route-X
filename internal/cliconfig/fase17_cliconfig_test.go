package cliconfig

import (
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

// TestB373KunciYAMLTopLevelTerkutip adalah pengunci B-373:
// perbaruiYAMLTopLevel dulu membandingkan nama kunci mentah —
// kunci terkutip ("GOOSE_PROVIDER": "lama", sah menurut YAML)
// tidak pernah cocok, jadi baris lama tertinggal dan pasangan
// baru ditambahkan di akhir: kunci ganda yang bacaan efektifnya
// bergantung parser. Kini nama dinormalkan dari kutipnya
// (pasangan perbaikan TOML B-343).
func TestB373KunciYAMLTopLevelTerkutip(t *testing.T) {
	for _, lama := range []string{
		"\"GOOSE_PROVIDER\": \"lama\"\nGOOSE_HOST: \"http://lama.invalid\"\n",
		"'GOOSE_PROVIDER': 'lama'\nGOOSE_HOST: 'http://lama.invalid'\n",
	} {
		hasil := perbaruiYAMLTopLevel(lama, [][2]string{
			{"GOOSE_PROVIDER", "routex"},
			{"GOOSE_HOST", "http://localhost:8080"},
		})
		if n := strings.Count(hasil, "GOOSE_PROVIDER"); n != 1 {
			t.Errorf("input %q: GOOSE_PROVIDER muncul %d kali, mau 1:\n%s", lama, n, hasil)
		}
		if strings.Contains(hasil, "\"lama\"") || strings.Contains(hasil, "'lama'") || strings.Contains(hasil, "lama.invalid") {
			t.Errorf("input %q: nilai lama tertinggal:\n%s", lama, hasil)
		}
		harusMemuat(t, hasil, "GOOSE_PROVIDER:")
		harusMemuat(t, hasil, "routex")
		harusMemuat(t, hasil, "http://localhost:8080")
	}
}

// TestB372SubkunciHermesTerkutipTersaring adalah pengunci B-372:
// susunBlokModelHermes menyaring subkunci terkelola blok model
// lama lewat kunciSubBarisYAML, yang dulu mengembalikan nama
// mentah — subkunci terkutip ("provider": ...) lolos saringan dan
// tertinggal SESUDAH blok baru; pembaca YAML last-key-wins
// (PyYAML Hermes) membaca nilai LAMA sementara Configure
// melaporkan sukses. Kelas sukses-bohong B-277.
func TestB372SubkunciHermesTerkutipTersaring(t *testing.T) {
	lama := "model:\n" +
		"  \"provider\": \"penyedia-lama\"\n" +
		"  'default': \"model-lama\"\n" +
		"  catatan_pengguna: utuh\n"
	hasil := susunBlokModelHermes(lama, "model-baru", "http://localhost:8080/v1", "GANTI_kunci-hermes")
	if strings.Contains(hasil, "model-lama") || strings.Contains(hasil, "penyedia-lama") {
		t.Errorf("nilai lama terkutip tertinggal dan akan menang last-wins:\n%s", hasil)
	}
	harusMemuat(t, hasil, "  provider: custom")
	harusMemuat(t, hasil, "  default: model-baru")
	harusMemuat(t, hasil, "  catatan_pengguna: utuh")
	if n := strings.Count(hasil, "provider"); n != 1 {
		t.Errorf("provider muncul %d kali, mau 1:\n%s", n, hasil)
	}
}

// TestB374EnvBOM adalah pengunci B-374: perbaruiBarisEnv dulu
// mencocokkan awalan NAMA= pada baris pertama apa adanya — berkas
// .env ber-BOM (penyunting Windows) membuat variabel pertama
// tidak pernah cocok, baris lama tertinggal dan salinan baru
// ditambahkan. Jalur TOML/YAML sudah membuang BOM (B-310/B-344).
func TestB374EnvBOM(t *testing.T) {
	hasil := perbaruiBarisEnv("\ufeffOPENAI_API_KEY=lama\nLAINNYA=1\n", "OPENAI_API_KEY", "baru")
	if strings.Contains(hasil, "\ufeff") {
		t.Errorf("BOM masih menempel pada hasil:\n%q", hasil)
	}
	if n := strings.Count(hasil, "OPENAI_API_KEY="); n != 1 {
		t.Errorf("OPENAI_API_KEY muncul %d kali, mau 1:\n%q", n, hasil)
	}
	harusMemuat(t, hasil, "OPENAI_API_KEY=baru")
	harusMemuat(t, hasil, "LAINNYA=1")
	if strings.Contains(hasil, "lama") {
		t.Errorf("nilai lama tertinggal:\n%q", hasil)
	}
}

// TestB379EnvBarisKosongPenggunaDipertahankan adalah pengunci
// B-379: saringan akhir perbaruiBarisEnv dulu membuang SETIAP
// baris kosong dari berkas yang sudah memuat variabelnya —
// termasuk baris kosong pemisah antargrup milik pengguna. Kini
// hanya baris yang dikosongkan sebagai duplikat yang dibuang.
func TestB379EnvBarisKosongPenggunaDipertahankan(t *testing.T) {
	lama := "GRUP_A=1\n\nOPENAI_API_KEY=lama\n\nGRUP_B=2\n"
	hasil := perbaruiBarisEnv(lama, "OPENAI_API_KEY", "baru")
	if hasil != "GRUP_A=1\n\nOPENAI_API_KEY=baru\n\nGRUP_B=2\n" {
		t.Errorf("hasil = %q, mau baris kosong pemisah dipertahankan persis", hasil)
	}
	// Duplikat tetap dibuang, pemisah tetap utuh.
	lama2 := "OPENAI_API_KEY=satu\n\nOPENAI_API_KEY=dua\n"
	hasil2 := perbaruiBarisEnv(lama2, "OPENAI_API_KEY", "baru")
	if n := strings.Count(hasil2, "OPENAI_API_KEY="); n != 1 {
		t.Errorf("duplikat: OPENAI_API_KEY muncul %d kali, mau 1:\n%q", n, hasil2)
	}
	harusMemuat(t, hasil2, "OPENAI_API_KEY=baru")
	if !strings.Contains(hasil2, "\n\n") {
		t.Errorf("duplikat: baris kosong pemisah ikut terbuang:\n%q", hasil2)
	}
}

// TestB380JSONBOM adalah pengunci keputusan B-380: BOM pada
// berkas JSON kini dibuang di muatKonfigJSON, disamakan dengan
// TOML/YAML/.env — BOM adalah artefak penyunting, bukan isi.
// Berkas yang memang rusak tetap ditolak (perlindungan B-217).
func TestB380JSONBOM(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "settings.json")
	if err := os.WriteFile(path, []byte("\ufeff{\"model\":\"lama\",\"lainnya\":1}"), 0o600); err != nil {
		t.Fatalf("tulis berkas uji: %v", err)
	}
	cfg, err := muatKonfigJSON(path, false)
	if err != nil {
		t.Fatalf("muatKonfigJSON ber-BOM: %v — BOM menggagalkan Configure", err)
	}
	// B-478: angka kini terbaca sebagai json.Number (teks mentah),
	// bukan float64 — itulah yang melestarikan angka besar utuh.
	if cfg["model"] != "lama" || cfg["lainnya"] != json.Number("1") {
		t.Errorf("cfg = %v, mau isi berkas terbaca utuh", cfg)
	}

	pathRusak := filepath.Join(dir, "rusak.json")
	if err := os.WriteFile(pathRusak, []byte("\ufeff{\"model\": "), 0o600); err != nil {
		t.Fatalf("tulis berkas rusak: %v", err)
	}
	if _, err := muatKonfigJSON(pathRusak, false); err == nil {
		t.Error("berkas JSON rusak (walau ber-BOM) lolos, mau tetap ditolak B-217")
	}
}

// TestB368RantaiSymlinkDuaLompatan adalah pengunci B-368 + B-381:
// rantai config → link1 → real (real belum ada). Penyelesaian
// satu lompatan lama membuat tulisAtomik mendarat DI link1
// (menghancurkannya menjadi berkas reguler) dan rollback
// menghapus link1 itu sendiri. Kini tulisan mencapai target
// akhir, seluruh rantai utuh, dan rollback menghapus berkas
// ciptaan beserta direktori induk yang dibuat Configure.
func TestB368RantaiSymlinkDuaLompatan(t *testing.T) {
	dir := t.TempDir()
	sub := filepath.Join(dir, "dotfiles")
	if err := os.MkdirAll(sub, 0o755); err != nil {
		t.Fatalf("mkdir: %v", err)
	}
	// Rantai: <dir>/config.toml → dotfiles/link.toml →
	// dalam/real.toml, dengan direktori "dalam" BELUM ada (B-381).
	linkPerantara := filepath.Join(sub, "link.toml")
	if err := os.Symlink(filepath.Join("..", "dalam", "real.toml"), linkPerantara); err != nil {
		t.Fatalf("symlink perantara: %v", err)
	}
	titikMasuk := filepath.Join(dir, "config.toml")
	if err := os.Symlink(filepath.Join("dotfiles", "link.toml"), titikMasuk); err != nil {
		t.Fatalf("symlink masuk: %v", err)
	}
	targetAkhir := filepath.Join(dir, "dalam", "real.toml")

	j := jepretBerkasConfigure(titikMasuk)
	if err := tulisAtomik(titikMasuk, []byte("isi-baru"), 0o600); err != nil {
		t.Fatalf("tulisAtomik menembus rantai: %v", err)
	}
	isi, err := os.ReadFile(targetAkhir)
	if err != nil || string(isi) != "isi-baru" {
		t.Fatalf("target akhir = %q, %v — mau isi-baru (tulisan tidak mencapai akhir rantai)", isi, err)
	}
	for _, tautan := range []string{titikMasuk, linkPerantara} {
		fi, err := os.Lstat(tautan)
		if err != nil || fi.Mode()&os.ModeSymlink == 0 {
			t.Fatalf("%s tidak lagi symlink sesudah tulis (mode %v, err %v) — tautan perantara hancur", tautan, fi.Mode(), err)
		}
	}

	j.pulihkan()
	if _, err := os.Lstat(targetAkhir); !os.IsNotExist(err) {
		t.Errorf("target ciptaan masih ada sesudah rollback (err=%v)", err)
	}
	if _, err := os.Lstat(filepath.Join(dir, "dalam")); !os.IsNotExist(err) {
		t.Errorf("direktori ciptaan Configure (dalam/) masih ada sesudah rollback (err=%v) — B-381", err)
	}
	for _, tautan := range []string{titikMasuk, linkPerantara} {
		fi, err := os.Lstat(tautan)
		if err != nil || fi.Mode()&os.ModeSymlink == 0 {
			t.Errorf("%s hilang/berubah sesudah rollback (mode %v, err %v)", tautan, fi.Mode(), err)
		}
	}
}
