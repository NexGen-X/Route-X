package cliconfig

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"
)

// TestB309HeaderTOMLTerkutipDikenali membuktikan header seksi TOML yang
// sah tetapi terkutip dinormalkan ke nama polos yang sama, sehingga
// penulis memperbarui seksi lama alih-alih menambahkan duplikatnya.
func TestB309HeaderTOMLTerkutipDikenali(t *testing.T) {
	for _, k := range []struct{ baris, ingin string }{
		{`["model.routex"]`, "model.routex"},
		{`[model_providers."routex"]`, "model_providers.routex"},
		{`['models']`, "models"},
		{`[ "model.routex" ] # catatan`, "model.routex"},
	} {
		if got := namaSeksiTOML(k.baris); got != k.ingin {
			t.Errorf("namaSeksiTOML(%q) = %q, ingin %q", k.baris, got, k.ingin)
		}
	}

	existing := "[\"model.routex\"]\nmodel = \"lama\"\nbase_url = \"http://lama\"\nkunci_pengguna = \"utuh\"\n"
	hasil := perbaruiKunciSeksiTOML(existing, "model.routex", [][2]string{
		{"model", "baru"}, {"base_url", "http://baru"},
	})
	if n := strings.Count(hasil, "model.routex"); n != 1 {
		t.Fatalf("hasil harus memuat tepat satu header model.routex, dapat %d:\n%s", n, hasil)
	}
	for _, ingin := range []string{`model = "baru"`, `base_url = "http://baru"`, `kunci_pengguna = "utuh"`} {
		if !strings.Contains(hasil, ingin) {
			t.Errorf("hasil kehilangan/mengabaikan %q:\n%s", ingin, hasil)
		}
	}
	if strings.Contains(hasil, `"lama"`) || strings.Contains(hasil, "http://lama") {
		t.Errorf("nilai seksi terkutip lama masih menang:\n%s", hasil)
	}
}

// TestB310PreambleTOMLBOM membuktikan preamble Codex yang diawali BOM
// diperbarui di tempat: tidak ada kunci root duplikat dan kunci baru
// tidak jatuh ke dalam seksi pertama.
func TestB310PreambleTOMLBOM(t *testing.T) {
	berpreamble := "\ufeffmodel = \"lama\"\nmodel_provider = \"lama\"\n\n[model_providers.routex]\nname = \"Route-X\"\n"
	hasil := perbaruiPreambleTOML(berpreamble, [][2]string{
		{"model_provider", "routex"}, {"model", "baru"},
	})
	if n := strings.Count(hasil, "model ="); n != 1 {
		t.Fatalf("kunci model root harus tepat satu, dapat %d:\n%q", n, hasil)
	}
	if n := strings.Count(hasil, "model_provider ="); n != 1 {
		t.Fatalf("kunci model_provider root harus tepat satu, dapat %d:\n%q", n, hasil)
	}
	if !strings.Contains(hasil, `model = "baru"`) || !strings.Contains(hasil, `model_provider = "routex"`) {
		t.Errorf("nilai preamble ber-BOM tidak diperbarui:\n%q", hasil)
	}

	langsungSeksi := "\ufeff[model_providers.routex]\nname = \"Route-X\"\n"
	hasil2 := perbaruiPreambleTOML(langsungSeksi, [][2]string{
		{"model_provider", "routex"}, {"model", "baru"},
	})
	idxSeksi := strings.Index(hasil2, "[model_providers.routex]")
	idxModel := strings.Index(hasil2, `model = "baru"`)
	if idxModel < 0 || idxSeksi < 0 || idxModel > idxSeksi {
		t.Errorf("kunci root harus lahir sebelum seksi pertama:\n%q", hasil2)
	}
	if strings.HasPrefix(hasil2, "\ufeff[") {
		t.Errorf("header seksi pertama masih ber-BOM sesudah preamble diperbarui:\n%q", hasil2)
	}
}

// TestB311TulisAtomikSymlinkPutus membuktikan symlink yang targetnya
// belum ada tetap dipertahankan: tulisan diarahkan ke target leksikal
// dan direktori induk target dibuat.
func TestB311TulisAtomikSymlinkPutus(t *testing.T) {
	dir := t.TempDir()
	target := filepath.Join(dir, "dotfiles", "config.toml")
	link := filepath.Join(dir, "config.toml")
	if err := os.Symlink(target, link); err != nil {
		t.Fatalf("membuat symlink putus: %v", err)
	}
	if err := tulisAtomik(link, []byte("isi-target"), 0600); err != nil {
		t.Fatalf("tulisAtomik: %v", err)
	}
	fi, err := os.Lstat(link)
	if err != nil {
		t.Fatalf("Lstat link: %v", err)
	}
	if fi.Mode()&os.ModeSymlink == 0 {
		t.Fatalf("jalur config berubah menjadi berkas reguler; symlink putus: mode %v", fi.Mode())
	}
	data, err := os.ReadFile(target)
	if err != nil {
		t.Fatalf("membaca target symlink: %v", err)
	}
	if string(data) != "isi-target" {
		t.Errorf("isi target = %q, ingin %q", data, "isi-target")
	}
}

// TestB320KeyEnvSeriDeterministik membuktikan pemenang key.env pada
// UpdatedAt yang seri persis tidak bergantung pada urutan iterasi map:
// pemutus serinya adalah ID tool leksikografis terbesar, diterapkan
// konsisten untuk variabel bersama maupun nama generik.
func TestB320KeyEnvSeriDeterministik(t *testing.T) {
	seri := time.Date(2026, 11, 7, 10, 0, 0, 0, time.UTC)
	for i := 0; i < 100; i++ {
		konten, err := susunKontenKeyEnv(map[string]kunciTersimpan{
			"claude": {kunci: "kunci-claude", diperbarui: seri},
			"goose":  {kunci: "kunci-goose", diperbarui: seri},
			"qwen":   {kunci: "kunci-qwen", diperbarui: seri},
		})
		if err != nil {
			t.Fatalf("susunKontenKeyEnv: %v", err)
		}
		if !strings.Contains(konten, "export ROUTEX_API_KEY='kunci-qwen'") {
			t.Fatalf("lari %d: ROUTEX_API_KEY tidak deterministik ke qwen:\n%s", i, konten)
		}
		if !strings.Contains(konten, "export OPENAI_API_KEY='kunci-qwen'") {
			t.Fatalf("lari %d: OPENAI_API_KEY tidak deterministik ke qwen:\n%s", i, konten)
		}
	}
}
