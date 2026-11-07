package cliconfig

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

// TestB393SiklusSymlinkGagalAman adalah pengunci B-393:
// selesaikanRantaiSymlink berhenti di batas 40 lompatan tanpa
// membedakan siklus dari rantai sah yang panjang — pada siklus
// a→b→a, tulisAtomik men-rename berkas reguler ke atas tautan a
// (menghancurkannya) sambil mengembalikan nil, dan jepretan
// membaca os.Stat ELOOP sebagai "target tidak ada" sehingga
// rollback menghapus tautan pengguna sendiri. Kini rantai tak
// tuntas = error eksplisit tanpa tulisan, dan jepretan tak
// tersentuh.
func TestB393SiklusSymlinkGagalAman(t *testing.T) {
	dir := t.TempDir()
	a := filepath.Join(dir, "a.conf")
	b := filepath.Join(dir, "b.conf")
	if err := os.Symlink("b.conf", a); err != nil {
		t.Fatalf("symlink a: %v", err)
	}
	if err := os.Symlink("a.conf", b); err != nil {
		t.Fatalf("symlink b: %v", err)
	}

	rantai, _, tuntas := selesaikanRantaiSymlink(a)
	if tuntas {
		t.Fatalf("selesaikanRantaiSymlink melaporkan tuntas untuk siklus (rantai %d lompatan)", len(rantai))
	}

	if err := tulisAtomik(a, []byte("isi"), 0o600); err == nil {
		t.Fatal("tulisAtomik melaporkan sukses menulis menembus siklus symlink")
	}
	for _, tautan := range []string{a, b} {
		fi, err := os.Lstat(tautan)
		if err != nil || fi.Mode()&os.ModeSymlink == 0 {
			t.Fatalf("%s tidak lagi symlink sesudah percobaan tulis (mode %v, err %v) — tautan pengguna hancur", tautan, fi.Mode(), err)
		}
	}

	j := jepretBerkasConfigure(a)
	if !j.takTersentuh {
		t.Error("jepretan siklus tidak ditandai takTersentuh — rollback bisa menghapus tautan pengguna")
	}
	j.pulihkan() // wajib no-op untuk berkas ini
	for _, tautan := range []string{a, b} {
		if _, err := os.Lstat(tautan); err != nil {
			t.Errorf("%s hilang sesudah pulihkan jepretan siklus: %v", tautan, err)
		}
	}
}

// TestB394RollbackMenghapusDirCiptaanCabangPolos adalah pengunci
// B-394: jepretan cabang symlink mencatat direktori induk ciptaan
// Configure (B-381), tetapi cabang POLOS tidak mencatat apa pun —
// direktori yang diciptakan Configure lewat MkdirAll di
// jalur-jalur tool bertahan sesudah rollback pada jalur umum.
// Kini kedua cabang mencatat leluhur yang belum ada dan
// pemulihan membongkarnya (hanya bila kosong).
func TestB394RollbackMenghapusDirCiptaanCabangPolos(t *testing.T) {
	dir := t.TempDir()
	jalur := filepath.Join(dir, "baru", "dalam", "konfig.json")

	j := jepretBerkasConfigure(jalur)
	if len(j.dirCiptaan) != 2 {
		t.Fatalf("dirCiptaan = %v, mau 2 leluhur (dalam, baru)", j.dirCiptaan)
	}

	// Tirukan Configure: buat direktori + berkas.
	if err := os.MkdirAll(filepath.Dir(jalur), 0o755); err != nil {
		t.Fatalf("MkdirAll: %v", err)
	}
	if err := os.WriteFile(jalur, []byte("{}"), 0o600); err != nil {
		t.Fatalf("WriteFile: %v", err)
	}

	j.pulihkan()
	if _, err := os.Lstat(jalur); !os.IsNotExist(err) {
		t.Errorf("berkas ciptaan masih ada sesudah rollback (err=%v)", err)
	}
	if _, err := os.Lstat(filepath.Join(dir, "baru")); !os.IsNotExist(err) {
		t.Errorf("direktori ciptaan Configure masih ada sesudah rollback (err=%v) — B-394", err)
	}

	// Kontrol: direktori ber-isi asing dipertahankan.
	jalur2 := filepath.Join(dir, "lain", "konfig.json")
	j2 := jepretBerkasConfigure(jalur2)
	if err := os.MkdirAll(filepath.Dir(jalur2), 0o755); err != nil {
		t.Fatalf("MkdirAll 2: %v", err)
	}
	if err := os.WriteFile(jalur2, []byte("{}"), 0o600); err != nil {
		t.Fatalf("WriteFile 2: %v", err)
	}
	if err := os.WriteFile(filepath.Join(dir, "lain", "milik-pengguna.txt"), []byte("jangan hapus"), 0o644); err != nil {
		t.Fatalf("WriteFile asing: %v", err)
	}
	j2.pulihkan()
	if _, err := os.Lstat(filepath.Join(dir, "lain", "milik-pengguna.txt")); err != nil {
		t.Errorf("berkas asing hilang sesudah rollback: %v", err)
	}
	if _, err := os.Lstat(filepath.Join(dir, "lain")); err != nil {
		t.Errorf("direktori ber-isi asing ikut terhapus: %v", err)
	}
}

// TestB402HeaderModelHermesTerkutip adalah pengunci B-402:
// pecahHeaderModelHermes membandingkan nama kunci mentah, satu-
// satunya pencocok YAML di berkas ini yang tidak memakai
// normalkanKunciYAML — header berkutip ("model":) tidak dikenali
// dan blok model: kedua ditambahkan. Kini berkutip dikenali.
func TestB402HeaderModelHermesTerkutip(t *testing.T) {
	for _, baris := range []string{`model:`, `"model":`, `'model':`, `model :`} {
		if _, ok := pecahHeaderModelHermes(baris); !ok {
			t.Errorf("pecahHeaderModelHermes(%q) ok=false, mau true", baris)
		}
	}
	if _, ok := pecahHeaderModelHermes(`"lain":`); ok {
		t.Error("header \"lain\" dikenali sebagai model")
	}

	// Integrasi: blok berkutip diperbarui di tempat, bukan
	// digandakan.
	lama := "\"model\":\n  provider: penyedia-lama\n"
	hasil := susunBlokModelHermes(lama, "model-baru", "http://localhost:8080/v1", "GANTI_kunci-hermes")
	if strings.Contains(hasil, "penyedia-lama") {
		t.Errorf("blok model berkutip tidak diperbarui, nilai lama tertinggal:\n%s", hasil)
	}
	if n := strings.Count(hasil, "default:"); n != 1 {
		t.Errorf("default muncul %d kali, mau 1 (blok tidak digandakan):\n%s", n, hasil)
	}
}

// TestB403EnvBentukTulisTangan adalah pengunci B-403:
// perbaruiBarisEnv hanya mencocokkan awalan persis NAMA= — bentuk
// sah yang ditulis tangan (export NAMA=..., NAMA = ..., indentasi)
// tidak pernah cocok, jadi variabel terduplikasi. Kini ketiganya
// dikenali dan diganti bentuk kanoniknya, duplikat lintas-bentuk
// terlipat menjadi satu.
func TestB403EnvBentukTulisTangan(t *testing.T) {
	kasus := []struct{ nama, masuk, namaVar string }{
		{"export", "export OPENAI_API_KEY=lama\n", "OPENAI_API_KEY"},
		{"spasi sekitar sama-dengan", "OPENAI_API_KEY = lama\n", "OPENAI_API_KEY"},
		{"indentasi", "  OPENAI_API_KEY=lama\n", "OPENAI_API_KEY"},
		{"export plus spasi", "export OPENAI_API_KEY = lama\n", "OPENAI_API_KEY"},
	}
	for _, k := range kasus {
		t.Run(k.nama, func(t *testing.T) {
			hasil := perbaruiBarisEnv(k.masuk, k.namaVar, "baru")
			if n := strings.Count(hasil, k.namaVar+"="); n != 1 {
				t.Errorf("variabel muncul %d kali, mau 1:\n%q", n, hasil)
			}
			harusMemuat(t, hasil, k.namaVar+"=baru")
			if strings.Contains(hasil, "lama") {
				t.Errorf("nilai lama tertinggal:\n%q", hasil)
			}
		})
	}

	// Duplikat lintas-bentuk terlipat menjadi satu baris kanonik;
	// komentar yang menyebut nama variabel tidak disentuh.
	lama := "# OPENAI_API_KEY=contoh\nOPENAI_API_KEY=satu\nexport OPENAI_API_KEY=dua\nLAIN=1\n"
	hasil := perbaruiBarisEnv(lama, "OPENAI_API_KEY", "baru")
	if n := strings.Count(hasil, "OPENAI_API_KEY=baru"); n != 1 {
		t.Errorf("baris kanonik muncul %d kali, mau 1:\n%q", n, hasil)
	}
	if strings.Contains(hasil, "satu") || strings.Contains(hasil, "dua") {
		t.Errorf("duplikat lintas-bentuk tertinggal:\n%q", hasil)
	}
	harusMemuat(t, hasil, "# OPENAI_API_KEY=contoh")
	harusMemuat(t, hasil, "LAIN=1")
}

// TestB404OpenCodeBOMBerkomentar adalah pengunci B-404: jalur
// sunting JSONC OpenCode membaca berkas mentah TANPA membuang BOM
// (muatKonfigJSON membuangnya sejak B-380, bacaan mentah tidak),
// jadi opencode.json ber-BOM + komentar selalu gagal disunting
// dan jatuh senyap ke penulisan ulang MarshalIndent yang membuang
// seluruh komentar — kehilangan yang dicegah B-280. Kini BOM
// dibuang sebelum sunting dan komentar bertahan.
func TestB404OpenCodeBOMBerkomentar(t *testing.T) {
	home := t.TempDir()
	t.Setenv("HOME", home)
	awal := "\ufeff{\n" +
		"  // komentar pengguna opencode — jangan hilang\n" +
		"  \"theme\": \"dracula\",\n" +
		"  \"model\": \"lain/model-lama\"\n" +
		"}\n"
	jalur := tulisOpenCodeUji(t, home, awal)
	konfigurasiOpenCodeUji(t, home, "model-baru", "http://localhost:8080/v1", "GANTI_kunci-opencode")
	konten := bacaBerkasUji(t, jalur)
	harusMemuat(t, konten, "// komentar pengguna opencode — jangan hilang")
	harusMemuat(t, konten, "\"theme\": \"dracula\"")
	harusMemuat(t, konten, "\"model\": \"routex/model-baru\"")
	if strings.Contains(konten, "\ufeff") {
		t.Errorf("BOM masih ada pada hasil:\n%q", konten)
	}
}
