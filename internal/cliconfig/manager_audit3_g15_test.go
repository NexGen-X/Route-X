package cliconfig

import (
	"bytes"
	"context"
	"io"
	"log/slog"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/NexGen-X/Route-X/internal/security"
)

// TestSusunKontenKeyEnvGabungan membuktikan audit3 V-LN-24/27: key.env
// memuat variabel khas SETIAP tool yang terkonfigurasi (bukan hanya empat
// nama generik tool terakhir), dan nama generik diisi kunci tool terbaru.
func TestSusunKontenKeyEnvGabungan(t *testing.T) {
	lama := time.Date(2026, 11, 1, 10, 0, 0, 0, time.UTC)
	baru := lama.Add(time.Hour)
	perTool := map[string]kunciTersimpan{
		"ollama":   {kunci: "kunci-ollama", diperbarui: lama},
		"goose":    {kunci: "kunci-goose", diperbarui: lama},
		"opencode": {kunci: "kunci-opencode", diperbarui: baru},
	}
	konten, err := susunKontenKeyEnv(perTool)
	if err != nil {
		t.Fatalf("susunKontenKeyEnv: %v", err)
	}
	// Variabel khas tool yang dulu tidak pernah terisi.
	if !strings.Contains(konten, "export OLLAMA_API_KEY='kunci-ollama'") {
		t.Errorf("OLLAMA_API_KEY tidak terisi dari konfigurasi ollama:\n%s", konten)
	}
	if !strings.Contains(konten, "export GOOSE_API_KEY='kunci-goose'") {
		t.Errorf("GOOSE_API_KEY tidak terisi dari konfigurasi goose:\n%s", konten)
	}
	// Nama generik mengikuti tool yang paling baru dikonfigurasi.
	if !strings.Contains(konten, "export ROUTEX_API_KEY='kunci-opencode'") {
		t.Errorf("ROUTEX_API_KEY harus diisi kunci tool terbaru (opencode):\n%s", konten)
	}
}

// TestBandingVersiNodeSemver membuktikan audit3 V-LN-36: pemilihan biner
// nvm membandingkan versi secara numerik (v22 > v9), bukan leksikografis.
func TestBandingVersiNodeSemver(t *testing.T) {
	v9 := "/home/x/.nvm/versions/node/v9.11.2/bin/claude"
	v22 := "/home/x/.nvm/versions/node/v22.11.0/bin/claude"
	if bandingVersiNode(v22, v9) <= 0 {
		t.Fatal("v22 harus dianggap lebih baru dari v9")
	}
	if bandingVersiNode(v9, v22) >= 0 {
		t.Fatal("v9 harus dianggap lebih lama dari v22")
	}
}

// TestRedactRoutexAPIKey membuktikan audit3 V-LN-34: ROUTEX_API_KEY ikut
// diredaksi, termasuk pada baris tanpa prefiks export.
func TestRedactRoutexAPIKey(t *testing.T) {
	m := &Manager{logger: slog.New(slog.NewTextHandler(io.Discard, nil))}
	skrip := "export ROUTEX_API_KEY='rx_live_supersecretvalue'\nROUTEX_API_KEY=rx_live_supersecretvalue\n"
	hasil := m.redactAPIKeysInScript(skrip)
	if strings.Contains(hasil, "rx_live_supersecretvalue") {
		t.Fatalf("kunci masih plaintext sesudah redaksi:\n%s", hasil)
	}
	hasil2 := m.removeAPIKeysFromScript(skrip)
	if strings.Contains(hasil2, "ROUTEX_API_KEY") {
		t.Fatalf("removeAPIKeysFromScript harus membuang baris ROUTEX_API_KEY:\n%s", hasil2)
	}
}

// TestConfigureDuaToolKeyEnvUtuh membuktikan audit3 V-LN-26/27 di level
// Configure: mengonfigurasi tool kedua TIDAK menghapus variabel tool
// pertama dari key.env.
func TestConfigureDuaToolKeyEnvUtuh(t *testing.T) {
	home := t.TempDir()
	t.Setenv("HOME", home)
	ctx := context.Background()
	logger := slog.New(slog.NewTextHandler(io.Discard, nil))
	store := newMockSettingsStore()
	cipher, err := security.NewCipher(bytes.Repeat([]byte{0x42}, 32))
	if err != nil {
		t.Fatalf("membuat cipher: %v", err)
	}
	mgr := NewManager(store, cipher, logger)

	if _, err := mgr.Configure(ctx, ConfigureParams{
		ToolID: "ollama", Mode: ModeRouting, Target: "llama3",
		APIKey: "kunci-ollama-123",
	}, "http://localhost:8080/v1"); err != nil {
		t.Fatalf("Configure ollama: %v", err)
	}
	if _, err := mgr.Configure(ctx, ConfigureParams{
		ToolID: "goose", Mode: ModeRouting, Target: "llama3",
		APIKey: "kunci-goose-456",
	}, "http://localhost:8080/v1"); err != nil {
		t.Fatalf("Configure goose: %v", err)
	}

	data, err := os.ReadFile(filepath.Join(home, ".routex", "key.env"))
	if err != nil {
		t.Fatalf("key.env tidak ada: %v", err)
	}
	konten := string(data)
	if !strings.Contains(konten, "OLLAMA_API_KEY") || !strings.Contains(konten, "kunci-ollama-123") {
		t.Errorf("key.env kehilangan konfigurasi ollama sesudah goose dikonfigurasi:\n%s", konten)
	}
	if !strings.Contains(konten, "GOOSE_API_KEY") || !strings.Contains(konten, "kunci-goose-456") {
		t.Errorf("key.env tidak memuat konfigurasi goose:\n%s", konten)
	}
	// Generik mengikuti yang terbaru (goose).
	if !strings.Contains(konten, "ROUTEX_API_KEY='kunci-goose-456'") && !strings.Contains(konten, `ROUTEX_API_KEY="kunci-goose-456"`) && !strings.Contains(konten, "ROUTEX_API_KEY=kunci-goose-456") {
		t.Errorf("ROUTEX_API_KEY harus berisi kunci terbaru (goose):\n%s", konten)
	}
}

// TestOpencodeConfigPathSelaras membuktikan audit3 V-LN-36 (sisi path):
// ConfigPaths opencode menunjuk berkas yang benar-benar ditulis Configure.
func TestOpencodeConfigPathSelaras(t *testing.T) {
	def, ok := cariToolDef("opencode")
	if !ok {
		t.Fatal("opencode tidak ditemukan di daftar tool")
	}
	if len(def.ConfigPaths) == 0 || def.ConfigPaths[0] != ".config/opencode/opencode.json" {
		t.Fatalf("ConfigPaths opencode = %v, mau [.config/opencode/opencode.json]", def.ConfigPaths)
	}
}
