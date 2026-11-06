package cliconfig

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

// TestB333RollbackSymlinkPutusTidakMenghapusTautan adalah pengunci
// B-333: jepretan Configure memakai os.Stat (mengikuti tautan), jadi
// symlink PUTUS terbaca "tidak ada". Configure menulis menembus
// tautan (B-311: target diciptakan secara leksikal), tetapi rollback
// untuk jepretan "tidak ada" menjalankan os.Remove pada JALUR —
// yang terhapus adalah TAUTAN pengguna, sementara berkas target
// ciptaan Configure tertinggal yatim. Kini jepretan menilai tautan
// dengan Lstat: rollback memulihkan tautannya sendiri dan keadaan
// targetnya, tidak pernah menghapus jalur yang semula symlink.
func TestB333RollbackSymlinkPutusTidakMenghapusTautan(t *testing.T) {
	home := t.TempDir()
	target := filepath.Join(home, "dotfiles", "codex.toml") // belum ada
	tautan := filepath.Join(home, ".codex", "config.toml")
	if err := os.MkdirAll(filepath.Dir(tautan), 0o755); err != nil {
		t.Fatalf("MkdirAll: %v", err)
	}
	if err := os.Symlink(filepath.Join("..", "dotfiles", "codex.toml"), tautan); err != nil {
		t.Fatalf("Symlink: %v", err)
	}

	j := jepretBerkasConfigure(tautan)
	// Configure menulis menembus tautan (target lahir + terisi).
	if err := tulisAtomik(tautan, []byte("model = \"baru\"\n"), 0o644); err != nil {
		t.Fatalf("tulisAtomik menembus tautan: %v", err)
	}
	if _, err := os.Stat(target); err != nil {
		t.Fatalf("target seharusnya lahir dari penulisan Configure: %v", err)
	}

	j.pulihkan()

	fi, err := os.Lstat(tautan)
	if err != nil {
		t.Fatalf("tautan hilang sesudah rollback: %v", err)
	}
	if fi.Mode()&os.ModeSymlink == 0 {
		t.Fatalf("jalur sesudah rollback bukan symlink lagi (mode %v) — tautan pengguna diganti berkas biasa", fi.Mode())
	}
	mentah, err := os.Readlink(tautan)
	if err != nil || mentah != filepath.Join("..", "dotfiles", "codex.toml") {
		t.Errorf("target tautan = %q (err %v), mau tetap %q", mentah, err, filepath.Join("..", "dotfiles", "codex.toml"))
	}
	if _, err := os.Stat(target); !os.IsNotExist(err) {
		t.Errorf("berkas target ciptaan Configure masih ada sesudah rollback (err %v) — seharusnya ikut dihapus", err)
	}
}

// TestB333RollbackSymlinkSehatMemulihkanTarget: tautan yang targetnya
// ADA — rollback mengembalikan isi target semula dan tautan tidak
// tersentuh (jalur ini kebetulan selamat di kode lama lewat
// Stat-mengikuti-tautan; dikunci supaya tidak mundur).
func TestB333RollbackSymlinkSehatMemulihkanTarget(t *testing.T) {
	home := t.TempDir()
	target := filepath.Join(home, "dotfiles", "codex.toml")
	tulisBerkasUji(t, target, "model = \"asli\"\n", 0o600)
	tautan := filepath.Join(home, ".codex", "config.toml")
	if err := os.MkdirAll(filepath.Dir(tautan), 0o755); err != nil {
		t.Fatalf("MkdirAll: %v", err)
	}
	if err := os.Symlink(filepath.Join("..", "dotfiles", "codex.toml"), tautan); err != nil {
		t.Fatalf("Symlink: %v", err)
	}

	j := jepretBerkasConfigure(tautan)
	if err := tulisAtomik(tautan, []byte("model = \"baru\"\n"), 0o644); err != nil {
		t.Fatalf("tulisAtomik: %v", err)
	}
	j.pulihkan()

	harusSamaIsi(t, target, "model = \"asli\"\n")
	fi, err := os.Lstat(tautan)
	if err != nil || fi.Mode()&os.ModeSymlink == 0 {
		t.Errorf("tautan rusak sesudah rollback: fi=%v err=%v", fi, err)
	}
}

// TestB343KunciTOMLTerkutipDikenali adalah pengunci B-343:
// terapkanPasanganTOML membandingkan sisi-kiri mentah, jadi kunci
// yang ditulis terkutip — bentuk sah TOML, `"model" = "lama"` atau
// `'model' = 'lama'` — tidak pernah cocok dengan nama polosnya:
// baris lama dipertahankan DAN pasangan kelolaan ditambahkan di
// akhir, menghasilkan kunci ganda (TOML tak sah). Kini sisi-kiri
// dinormalkan dengan logika segmen yang sama seperti nama seksi
// (B-309) dan penggantinya ditulis dengan kunci polos kanonis.
func TestB343KunciTOMLTerkutipDikenali(t *testing.T) {
	pairs := [][2]string{{"model", "baru"}, {"model_provider", "routex"}}

	kasus := []string{
		"\"model\" = \"lama\"\nmodel_provider = \"x\"\n",
		"'model' = 'lama'\nmodel_provider = \"x\"\n",
	}
	for _, masukan := range kasus {
		keluaran := perbaruiPreambleTOML(masukan, pairs)
		if n := strings.Count(keluaran, "model ="); n != 1 {
			t.Errorf("masukan %q: baris model = %d, mau tepat 1:\n%s", masukan, n, keluaran)
		}
		if !strings.Contains(keluaran, "model = \"baru\"") {
			t.Errorf("masukan %q: nilai model tidak diperbarui:\n%s", masukan, keluaran)
		}
		if strings.Contains(keluaran, "\"lama\"") || strings.Contains(keluaran, "'lama'") {
			t.Errorf("masukan %q: nilai lama masih tertinggal:\n%s", masukan, keluaran)
		}
		if !strings.Contains(keluaran, "model_provider = \"routex\"") {
			t.Errorf("masukan %q: model_provider tidak diperbarui:\n%s", masukan, keluaran)
		}
	}

	// Kontrol: kunci polos berperilaku seperti sebelumnya, dan kunci
	// terkutip yang BUKAN kelolaan tidak disentuh.
	keluaran := perbaruiPreambleTOML("model = \"lama\"\n\"lain\" = 1\n", pairs)
	if !strings.Contains(keluaran, "model = \"baru\"") || !strings.Contains(keluaran, "\"lain\" = 1") {
		t.Errorf("kontrol polos/tak-kelolaan menyimpang:\n%s", keluaran)
	}
}

// TestB344YAMLBerBOMTidakMenggandakanKunci adalah pengunci B-344:
// perbaruiYAMLTopLevel tidak membuang BOM di awal dokumen —
// TrimSpace tidak membuang BOM, jadi kunci pertama ber-BOM tidak
// pernah cocok dengan nama polosnya (kunci lama tertinggal, kunci
// baru ditambahkan di akhir: ganda). Pelajaran yang sama sudah
// dibayar di jalur TOML (B-310); kini jalur YAML ikut membuang BOM
// lebih dulu.
func TestB344YAMLBerBOMTidakMenggandakanKunci(t *testing.T) {
	masukan := "\ufeffGOOSE_PROVIDER: \"lama\"\nGOOSE_MODEL: \"m1\"\n"
	keluaran := perbaruiYAMLTopLevel(masukan, [][2]string{{"GOOSE_PROVIDER", "routex"}})
	if n := strings.Count(keluaran, "GOOSE_PROVIDER:"); n != 1 {
		t.Errorf("GOOSE_PROVIDER muncul %d kali, mau tepat 1:\n%q", n, keluaran)
	}
	if !strings.Contains(keluaran, "GOOSE_PROVIDER: routex") {
		t.Errorf("nilai GOOSE_PROVIDER tidak diperbarui:\n%q", keluaran)
	}
	if strings.Contains(keluaran, "\ufeff") {
		t.Errorf("BOM masih tertinggal di keluaran:\n%q", keluaran)
	}
	if !strings.Contains(keluaran, "GOOSE_MODEL: \"m1\"") {
		t.Errorf("kunci lain ikut berubah:\n%q", keluaran)
	}
}
