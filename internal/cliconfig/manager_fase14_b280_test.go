package cliconfig

import (
	"context"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

// Berkas ini memuat test pengunci B-280 (Fase 14 kampanye
// tuntas-logika): OpenCode dimuat toleran JSONC (B-217) tetapi
// ditulis ulang via MarshalIndent sehingga seluruh komentar
// pengguna di opencode.json hilang pada Configure pertama. Sesudah
// fix, berkas berkomentar disunting secara bedah pada teksnya —
// komentar, kunci lain, dan format bertahan; hanya nilai terkelola
// yang berubah. Hasil sunting diverifikasi semantik terhadap
// mutasi peta sebelum ditulis.

func tulisOpenCodeUji(t *testing.T, home, isi string) string {
	t.Helper()
	dir := filepath.Join(home, ".config", "opencode")
	if err := os.MkdirAll(dir, 0o755); err != nil {
		t.Fatal(err)
	}
	jalur := filepath.Join(dir, "opencode.json")
	if err := os.WriteFile(jalur, []byte(isi), 0o644); err != nil {
		t.Fatal(err)
	}
	return jalur
}

func konfigurasiOpenCodeUji(t *testing.T, home, target, gwURL, apiKey string) {
	t.Helper()
	def, ok := cariToolDef("opencode")
	if !ok {
		t.Fatal("opencode tidak ditemukan di registri")
	}
	mgr := manajerUjiToolBaru(newMockSettingsStore())
	if err := mgr.applyToolSpecificConfig(context.Background(), home, def, ModeModelOnly, target, gwURL, apiKey); err != nil {
		t.Fatalf("applyToolSpecificConfig opencode: %v", err)
	}
}

// TestB280KomentarOpenCodeBertahan: komentar baris & blok serta
// setelan lain pengguna selamat dari Configure; provider routex
// terpasang dengan nilai yang benar.
func TestB280KomentarOpenCodeBertahan(t *testing.T) {
	home := t.TempDir()
	t.Setenv("HOME", home)
	awal := "{\n" +
		"  // komentar pengguna opencode — jangan hilang\n" +
		"  \"theme\": \"dracula\",\n" +
		"  /* blok catatan\n" +
		"     bertingkat */\n" +
		"  \"provider\": {\n" +
		"    // provider lain milik pengguna\n" +
		"    \"lain\": { \"options\": { \"baseURL\": \"http://lain.invalid/v1\" } }\n" +
		"  },\n" +
		"  \"model\": \"lain/model-lama\"\n" +
		"}\n"
	jalur := tulisOpenCodeUji(t, home, awal)
	konfigurasiOpenCodeUji(t, home, "model-baru", "http://localhost:8080/v1", "GANTI_kunci-opencode")
	konten := bacaBerkasUji(t, jalur)
	for _, mau := range []string{
		"// komentar pengguna opencode — jangan hilang",
		"/* blok catatan",
		"// provider lain milik pengguna",
		"\"theme\": \"dracula\"",
		"http://lain.invalid/v1",
		// Rangka provider baru disisipkan sebagai JSON ringkas —
		// yang dikunci adalah nilainya, bukan format sisipannya.
		"\"baseURL\":\"http://localhost:8080/v1\"",
		"\"model\": \"routex/model-baru\"",
	} {
		harusMemuat(t, konten, mau)
	}
}

// TestB280ProviderRoutexAdaDiperbaruiDiTempat: provider routex
// yang sudah ada (dengan komentar di dalam bloknya) diperbarui
// nilainya tanpa menggandakan blok dan tanpa membuang komentar.
func TestB280ProviderRoutexAdaDiperbaruiDiTempat(t *testing.T) {
	home := t.TempDir()
	t.Setenv("HOME", home)
	awal := "{\n" +
		"  \"provider\": {\n" +
		"    \"routex\": {\n" +
		"      \"npm\": \"@ai-sdk/openai-compatible\",\n" +
		"      \"options\": {\n" +
		"        // url gateway lama\n" +
		"        \"baseURL\": \"http://lama.invalid/v1\",\n" +
		"        \"apiKey\": \"GANTI_kunci-lama\"\n" +
		"      },\n" +
		"      \"models\": {}\n" +
		"    }\n" +
		"  }\n" +
		"}\n"
	jalur := tulisOpenCodeUji(t, home, awal)
	konfigurasiOpenCodeUji(t, home, "model-baru", "http://localhost:8080/v1", "GANTI_kunci-baru")
	konten := bacaBerkasUji(t, jalur)
	harusMemuat(t, konten, "// url gateway lama")
	harusMemuat(t, konten, "\"baseURL\": \"http://localhost:8080/v1\"")
	harusMemuat(t, konten, "\"apiKey\": \"GANTI_kunci-baru\"")
	if strings.Contains(konten, "lama.invalid") || strings.Contains(konten, "GANTI_kunci-lama") {
		t.Errorf("nilai lama tertinggal:\n%s", konten)
	}
	if n := strings.Count(konten, "\"routex\""); n != 1 {
		t.Errorf("blok routex tergandakan (%d):\n%s", n, konten)
	}
}

// TestB280ApiKeyKosongDihapusKomentarTetap: kunci efektif kosong
// menghapus apiKey dari options (semantik jalur peta) sementara
// komentar di sekitarnya bertahan.
func TestB280ApiKeyKosongDihapusKomentarTetap(t *testing.T) {
	home := t.TempDir()
	t.Setenv("HOME", home)
	awal := "{\n" +
		"  // kepala berkas\n" +
		"  \"provider\": { \"routex\": { \"options\": { \"baseURL\": \"http://lama.invalid/v1\", \"apiKey\": \"GANTI_kunci-lama\" } } }\n" +
		"}\n"
	jalur := tulisOpenCodeUji(t, home, awal)
	konfigurasiOpenCodeUji(t, home, "model-baru", "http://localhost:8080/v1", "")
	konten := bacaBerkasUji(t, jalur)
	harusMemuat(t, konten, "// kepala berkas")
	if strings.Contains(konten, "apiKey") || strings.Contains(konten, "GANTI_kunci-lama") {
		t.Errorf("apiKey tidak terhapus pada kunci efektif kosong:\n%s", konten)
	}
	harusMemuat(t, konten, "\"baseURL\": \"http://localhost:8080/v1\"")
}

// TestB280KomaMenggantungDanKomentar: JSONC dengan trailing comma
// — sah menurut tool-nya — tetap terkonfigurasi dan komentarnya
// bertahan.
func TestB280KomaMenggantungDanKomentar(t *testing.T) {
	home := t.TempDir()
	t.Setenv("HOME", home)
	awal := "{\n" +
		"  \"theme\": \"dracula\", // tema pilihan\n" +
		"}\n"
	jalur := tulisOpenCodeUji(t, home, awal)
	konfigurasiOpenCodeUji(t, home, "model-baru", "http://localhost:8080/v1", "GANTI_kunci")
	konten := bacaBerkasUji(t, jalur)
	harusMemuat(t, konten, "// tema pilihan")
	harusMemuat(t, konten, "\"model\": \"routex/model-baru\"")
}
