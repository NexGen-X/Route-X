package cliconfig

import (
	"context"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

// Berkas ini memuat test pengunci B-277 (Fase 14 kampanye
// tuntas-logika): pengenalan header seksi TOML dan blok Hermes yang
// exact-match membuat penulis menambahkan seksi/blok KEDUA untuk
// varian header yang sah (komentar ekor, spasi di dalam kurung /
// sebelum titik dua, BOM), dan khusus Hermes berkomentar sub-kunci
// lama menempel sesudah blok baru sehingga nilai lama menang
// last-wins sementara Configure melaporkan sukses. Sesudah fix,
// pengenalan bersifat toleran dan Configure memperbarui blok yang
// ada — tepat satu seksi/blok, nilai terkelola baru, kunci lain
// pengguna utuh.

// TestB277GrokHeaderBerkomentarDiperbaruiBukanDigandakan: Configure
// Grok atas config yang header seksinya berkomentar harus
// memperbarui seksi itu, bukan menambahkan [model.routex] kedua.
func TestB277GrokHeaderBerkomentarDiperbaruiBukanDigandakan(t *testing.T) {
	home := t.TempDir()
	t.Setenv("HOME", home)
	dir := filepath.Join(home, ".grok")
	if err := os.MkdirAll(dir, 0o755); err != nil {
		t.Fatal(err)
	}
	lama := "[model.routex] # diatur tangan\n" +
		"model = \"model-lama\"\n" +
		"base_url = \"http://lama.invalid/v1\"\n" +
		"kunci_pengguna = \"utuh\"\n"
	if err := os.WriteFile(filepath.Join(dir, "config.toml"), []byte(lama), 0o644); err != nil {
		t.Fatal(err)
	}
	def, ok := cariToolDef("grok-build")
	if !ok {
		t.Fatal("grok-build tidak ditemukan di registri")
	}
	mgr := manajerUjiToolBaru(newMockSettingsStore())
	if err := mgr.applyToolSpecificConfig(context.Background(), home, def, ModeModelOnly, "model-baru", "http://localhost:8080/v1", "GANTI_kunci-grok"); err != nil {
		t.Fatalf("applyToolSpecificConfig grok-build: %v", err)
	}
	konten := bacaBerkasUji(t, filepath.Join(dir, "config.toml"))
	if n := strings.Count(konten, "[model.routex]"); n != 1 {
		t.Errorf("jumlah header [model.routex] = %d, mau tepat 1:\n%s", n, konten)
	}
	harusMemuat(t, konten, `base_url = "http://localhost:8080/v1"`)
	harusMemuat(t, konten, `kunci_pengguna = "utuh"`)
	if strings.Contains(konten, "lama.invalid") {
		t.Errorf("base_url lama tidak diperbarui:\n%s", konten)
	}
}

// TestB277VarianHeaderTOML: keempat varian header yang terbukti
// menggandakan seksi pada probe Putaran Bersih dikenali sebagai
// seksi yang sama oleh rentangSeksiTOML.
func TestB277VarianHeaderTOML(t *testing.T) {
	varian := []string{
		"[model.routex] # komentar",
		"[ model.routex ]",
		"[model.routex ]",
		"\ufeff[model.routex]",
	}
	for _, header := range varian {
		baris := []string{header, "a = 1", "", "[lain]", "b = 2"}
		awal, akhir, ok := rentangSeksiTOML(baris, "model.routex")
		if !ok {
			t.Errorf("header %q tidak dikenali sebagai seksi model.routex", header)
			continue
		}
		if awal != 1 || akhir != 3 {
			t.Errorf("header %q: rentang = (%d,%d), mau (1,3)", header, awal, akhir)
		}
	}
	// Bukan header seksi bernama: nilai array dan header tabel
	// berganda tidak boleh tertukar dengan seksi biasa.
	if _, _, ok := rentangSeksiTOML([]string{"[[model.routex]]", "a = 1"}, "model.routex"); ok {
		t.Error("header [[...]] tertukar dengan seksi biasa")
	}
	// Perbarui lewat varian berkomentar tidak menggandakan.
	hasil := perbaruiKunciSeksiTOML("[model.routex] # c\nmodel = \"lama\"\n", "model.routex",
		[][2]string{{"model", "baru"}})
	if n := strings.Count(hasil, "[model.routex]"); n != 1 {
		t.Errorf("perbaruiKunciSeksiTOML menggandakan seksi:\n%s", hasil)
	}
	harusMemuat(t, hasil, `model = "baru"`)
}

// TestB277HermesHeaderBerkomentarNilaiBaruMenang: blok Hermes lama
// yang header-nya berkomentar harus DIGABUNG per-kunci — sub-kunci
// terkelola lama dibuang (nilai baru menang), sub-kunci lain
// pengguna dipertahankan, dan tepat satu blok `model:` tersisa.
func TestB277HermesHeaderBerkomentarNilaiBaruMenang(t *testing.T) {
	lama := "model: # diatur tangan\n" +
		"  provider: openai\n" +
		"  default: model-lama\n" +
		"  base_url: http://lama.invalid/v1\n" +
		"  api_mode: chat_completions\n" +
		"  catatan_pengguna: utuh\n" +
		"lainnya: 1\n"
	hasil := susunBlokModelHermes(lama, "model-baru", "http://localhost:8080/v1", "GANTI_kunci-hermes")
	if n := strings.Count(hasil, "\nmodel:"); n != 0 {
		t.Errorf("blok model ganda sesudah baris pertama:\n%s", hasil)
	}
	if !strings.HasPrefix(hasil, "model:") {
		t.Errorf("blok model tidak diawali header model::\n%s", hasil)
	}
	harusMemuat(t, hasil, "  default: model-baru")
	harusMemuat(t, hasil, "  catatan_pengguna: utuh")
	if strings.Contains(hasil, "model-lama") || strings.Contains(hasil, "lama.invalid") ||
		strings.Contains(hasil, "provider: openai") {
		t.Errorf("nilai lama masih menempel dan akan menang last-wins:\n%s", hasil)
	}
}

// TestB277HermesVarianHeaderLain: `model :` (spasi sebelum titik
// dua) dan BOM di awal berkas juga dikenali sebagai blok yang sama.
func TestB277HermesVarianHeaderLain(t *testing.T) {
	for _, header := range []string{"model :", "\ufeffmodel:"} {
		lama := header + "\n  provider: openai\n  default: model-lama\n"
		hasil := susunBlokModelHermes(lama, "model-baru", "http://localhost:8080/v1", "")
		hitung := 0
		for _, l := range strings.Split(hasil, "\n") {
			trim := strings.TrimPrefix(l, "\ufeff")
			if trim == "model:" || strings.HasPrefix(trim, "model :") {
				hitung++
			}
		}
		if hitung != 1 {
			t.Errorf("header %q: jumlah blok model = %d, mau 1:\n%s", header, hitung, hasil)
		}
		if strings.Contains(hasil, "model-lama") {
			t.Errorf("header %q: nilai lama tertinggal:\n%s", header, hasil)
		}
	}
	// Kunci lain yang mirip (`models:`) tidak tertukar dengan blok
	// model: blok baru ditambahkan, seksi pengguna dipertahankan.
	hasil := susunBlokModelHermes("models:\n  a: 1\n", "model-baru", "http://localhost:8080/v1", "")
	harusMemuat(t, hasil, "models:")
	harusMemuat(t, hasil, "  a: 1")
	harusMemuat(t, hasil, "  default: model-baru")
}
