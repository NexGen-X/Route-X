package cliconfig

import (
	"context"
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

// Berkas ini memuat test pengunci Fase 9 kampanye tuntas-logika untuk
// tiga temuan cliconfig: B-219 (Copilot melaporkan direktori sebagai
// config), B-220 (izin goose dilonggarkan paksa ke 0644), dan B-221
// (OpenCode: provider routex tanpa sub-objek options tidak pernah
// diperbarui URL/kuncinya).

// TestB219CopilotTidakMelaporkanDirektoriSebagaiConfig membuktikan
// Scan tidak lagi menyajikan direktori $COPILOT_HOME sebagai berkas
// konfigurasi: Configure Copilot murni env dan tidak menulis berkas
// apa pun, jadi config_path yang jujur adalah kosong.
func TestB219CopilotTidakMelaporkanDirektoriSebagaiConfig(t *testing.T) {
	ctx := context.Background()
	home := t.TempDir()
	t.Setenv("HOME", home)
	if err := os.MkdirAll(filepath.Join(home, ".copilot"), 0o755); err != nil {
		t.Fatalf("MkdirAll .copilot: %v", err)
	}

	mgr := manajerUjiCipher(t, newMockSettingsStore())
	statuses, err := mgr.Scan(ctx, "http://localhost:8080/v1")
	if err != nil {
		t.Fatalf("Scan: %v", err)
	}
	for _, st := range statuses {
		if st.ID != "copilot" {
			continue
		}
		if st.ConfigPath != "" {
			t.Errorf("ConfigPath copilot = %q, mau kosong (direktori %s bukan berkas konfigurasi yang dikelola produk)",
				st.ConfigPath, filepath.Join(home, ".copilot"))
		}
		return
	}
	t.Fatal("status copilot tidak ditemukan di hasil Scan")
}

// TestB220IzinGooseDipertahankan membuktikan Configure goose tidak
// melonggarkan izin berkas eksisting yang lebih ketat: berkas 0600
// milik pengguna harus tetap 0600 sesudah ditulis ulang.
func TestB220IzinGooseDipertahankan(t *testing.T) {
	ctx := context.Background()
	home := t.TempDir()
	t.Setenv("HOME", home)

	goosePath := filepath.Join(home, ".config", "goose", "config.yaml")
	if err := os.MkdirAll(filepath.Dir(goosePath), 0o755); err != nil {
		t.Fatalf("MkdirAll: %v", err)
	}
	awal := "GOOSE_PROVIDER: openai\nGOOSE_MODEL: model-lama\nCATATAN_PRIBADI: jangan-lolos\n"
	if err := os.WriteFile(goosePath, []byte(awal), 0o600); err != nil {
		t.Fatalf("WriteFile: %v", err)
	}

	mgr := manajerUjiCipher(t, newMockSettingsStore())
	if _, err := mgr.Configure(ctx, ConfigureParams{
		ToolID: "goose", Mode: ModeModelOnly, Target: "model-b220",
		GatewayURL: "http://localhost:8080/v1", APIKey: "GANTI_kunci-b220",
	}, "http://localhost:8080/v1"); err != nil {
		t.Fatalf("Configure goose: %v", err)
	}

	fi, err := os.Stat(goosePath)
	if err != nil {
		t.Fatalf("Stat: %v", err)
	}
	if got := fi.Mode().Perm(); got != 0o600 {
		t.Errorf("izin config.yaml goose sesudah Configure = %o, mau 0600 (izin eksisting dipertahankan)", got)
	}
	isi, err := os.ReadFile(goosePath)
	if err != nil {
		t.Fatalf("ReadFile: %v", err)
	}
	if !strings.Contains(string(isi), "GOOSE_MODEL") || !strings.Contains(string(isi), "model-b220") {
		t.Errorf("GOOSE_MODEL tidak diperbarui:\n%s", isi)
	}
	if !strings.Contains(string(isi), "CATATAN_PRIBADI") || !strings.Contains(string(isi), "jangan-lolos") {
		t.Errorf("kunci lain milik pengguna hilang:\n%s", isi)
	}
}

// TestB221OpenCodeOptionsDibuat membuktikan provider routex warisan
// tanpa sub-objek options tetap menerima baseURL + apiKey saat
// Configure: options dibuat, bukan dilewati diam-diam.
func TestB221OpenCodeOptionsDibuat(t *testing.T) {
	ctx := context.Background()
	home := t.TempDir()
	t.Setenv("HOME", home)

	configPath := filepath.Join(home, ".config", "opencode", "opencode.json")
	if err := os.MkdirAll(filepath.Dir(configPath), 0o755); err != nil {
		t.Fatalf("MkdirAll: %v", err)
	}
	warisan := `{
  "$schema": "https://opencode.ai/config.json",
  "provider": {
    "routex": {
      "npm": "@ai-sdk/openai-compatible",
      "models": {}
    }
  }
}
`
	if err := os.WriteFile(configPath, []byte(warisan), 0o644); err != nil {
		t.Fatalf("WriteFile: %v", err)
	}

	mgr := manajerUjiCipher(t, newMockSettingsStore())
	if _, err := mgr.Configure(ctx, ConfigureParams{
		ToolID: "opencode", Mode: ModeModelOnly, Target: "model-b221",
		GatewayURL: "http://localhost:8080/v1", APIKey: "GANTI_kunci-b221",
	}, "http://localhost:8080/v1"); err != nil {
		t.Fatalf("Configure opencode: %v", err)
	}

	raw, err := os.ReadFile(configPath)
	if err != nil {
		t.Fatalf("ReadFile: %v", err)
	}
	var cfg map[string]any
	if err := json.Unmarshal(raw, &cfg); err != nil {
		t.Fatalf("hasil bukan JSON sah: %v", err)
	}
	providers, _ := cfg["provider"].(map[string]any)
	routex, _ := providers["routex"].(map[string]any)
	opts, ok := routex["options"].(map[string]any)
	if !ok {
		t.Fatalf("options tidak dibuat pada provider routex warisan: %s", raw)
	}
	if opts["baseURL"] != "http://localhost:8080/v1" {
		t.Errorf("options.baseURL = %v, mau http://localhost:8080/v1", opts["baseURL"])
	}
	if opts["apiKey"] != "GANTI_kunci-b221" {
		t.Errorf("options.apiKey = %v, mau kunci efektif Configure", opts["apiKey"])
	}
	if cfg["model"] != "routex/model-b221" {
		t.Errorf("model = %v, mau routex/model-b221", cfg["model"])
	}
}
