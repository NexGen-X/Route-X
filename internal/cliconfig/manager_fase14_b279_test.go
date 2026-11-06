package cliconfig

import (
	"context"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

// Berkas ini memuat test pengunci B-279 (Fase 14 kampanye
// tuntas-logika): tulisAtomik memakai rename ke jalur tujuan apa
// adanya, sehingga symlink dotfiles (GNU Stow/chezmoi) pada berkas
// konfigurasi tool PUTUS pada Configure pertama dan berkas target
// aslinya tidak pernah terbarui — sementara Configure tampak
// sukses. Sesudah fix, penulis menyelesaikan symlink lebih dulu
// (meniru jalur hook rc yang sudah symlink-aware sejak V-LN-33):
// tautan dipertahankan, isi target yang diperbarui.

// TestB279TulisAtomikMengikutiSymlink: unit murni pada penulisnya.
func TestB279TulisAtomikMengikutiSymlink(t *testing.T) {
	dir := t.TempDir()
	target := filepath.Join(dir, "dotfiles", "settings.json")
	if err := os.MkdirAll(filepath.Dir(target), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(target, []byte(`{"tema_dotfiles": true}`), 0o644); err != nil {
		t.Fatal(err)
	}
	tautan := filepath.Join(dir, "settings.json")
	if err := os.Symlink(target, tautan); err != nil {
		t.Fatal(err)
	}
	if err := tulisAtomik(tautan, []byte(`{"baru": true}`), 0o600); err != nil {
		t.Fatalf("tulisAtomik: %v", err)
	}
	fi, err := os.Lstat(tautan)
	if err != nil {
		t.Fatal(err)
	}
	if fi.Mode()&os.ModeSymlink == 0 {
		t.Fatal("symlink PUTUS sesudah tulisAtomik (B-279)")
	}
	isi, err := os.ReadFile(target)
	if err != nil {
		t.Fatal(err)
	}
	if string(isi) != `{"baru": true}` {
		t.Fatalf("isi target = %s, mau konfigurasi baru", isi)
	}
}

// TestB279ConfigureClaudeSymlinkUtuh: jalur penuh — Configure
// Claude atas settings.json ber-symlink mempertahankan tautan dan
// memperbarui target (reproduksi probe Putaran Bersih W5-BARU-3).
func TestB279ConfigureClaudeSymlinkUtuh(t *testing.T) {
	home := t.TempDir()
	t.Setenv("HOME", home)
	dotfiles := filepath.Join(home, "dotfiles-repo")
	if err := os.MkdirAll(dotfiles, 0o755); err != nil {
		t.Fatal(err)
	}
	target := filepath.Join(dotfiles, "claude-settings.json")
	if err := os.WriteFile(target, []byte(`{"tema_dotfiles": true}`), 0o644); err != nil {
		t.Fatal(err)
	}
	claudeDir := filepath.Join(home, ".claude")
	if err := os.MkdirAll(claudeDir, 0o755); err != nil {
		t.Fatal(err)
	}
	tautan := filepath.Join(claudeDir, "settings.json")
	if err := os.Symlink(target, tautan); err != nil {
		t.Fatal(err)
	}
	def, ok := cariToolDef("claude")
	if !ok {
		t.Fatal("claude tidak ditemukan di registri")
	}
	mgr := manajerUjiToolBaru(newMockSettingsStore())
	if err := mgr.applyToolSpecificConfig(context.Background(), home, def, ModeModelOnly, "model-baru", "http://localhost:8080/v1", "GANTI_kunci-claude"); err != nil {
		t.Fatalf("applyToolSpecificConfig claude: %v", err)
	}
	fi, err := os.Lstat(tautan)
	if err != nil {
		t.Fatal(err)
	}
	if fi.Mode()&os.ModeSymlink == 0 {
		t.Fatal("symlink settings.json PUTUS sesudah Configure (B-279)")
	}
	isi := bacaBerkasUji(t, target)
	if strings.Contains(isi, "tema_dotfiles") && !strings.Contains(isi, "localhost:8080") {
		t.Fatalf("target dotfiles tidak terbarui oleh Configure:\n%s", isi)
	}
	harusMemuat(t, isi, "localhost:8080")
}
