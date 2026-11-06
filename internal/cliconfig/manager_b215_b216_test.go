package cliconfig

import (
	"bytes"
	"context"
	"encoding/json"
	"io"
	"log/slog"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/NexGen-X/Route-X/internal/security"
)

// Berkas ini memuat test pengunci B-215 dan B-216 (Fase 4 kampanye
// tuntas-logika): konfigurasi ulang CLI tanpa kunci baru TIDAK boleh
// menghapus kunci yang tersimpan, dan penulisan konfigurasi TIDAK
// boleh menghancurkan setelan pengguna yang tidak dikelola Route-X.

func manajerUjiCipher(t *testing.T, store SettingsStore) *Manager {
	t.Helper()
	cipher, err := security.NewCipher(bytes.Repeat([]byte{0x33}, 32))
	if err != nil {
		t.Fatalf("membuat cipher: %v", err)
	}
	return NewManager(store, cipher, slog.New(slog.NewTextHandler(io.Discard, nil)))
}

// TestB215KunciTersimpanSelamatKonfigurasiUlang membuktikan kunci yang
// tersimpan (terenkripsi di settings) tetap sampai ke berkas tool saat
// Configure diulang TANPA kunci: crush.json (satu-satunya rumah kunci
// Crush), settings.json Claude (env.ANTHROPIC_API_KEY), opencode.json
// (options.apiKey), dan blok model Hermes (api_key) semuanya
// mempertahankan kunci tersimpan — dan kunci BARU tetap menimpanya.
func TestB215KunciTersimpanSelamatKonfigurasiUlang(t *testing.T) {
	home := t.TempDir()
	t.Setenv("HOME", home)
	ctx := context.Background()
	store := newMockSettingsStore()
	mgr := manajerUjiCipher(t, store)

	const kunci = "GANTI_kunci-tersimpan-b215"
	const kunciBaru = "GANTI_kunci-baru-b215"

	// --- Crush ---
	if _, err := mgr.Configure(ctx, ConfigureParams{
		ToolID: "crush", Mode: ModeModelOnly, Target: "gpt-5",
		GatewayURL: "http://localhost:8080/v1", APIKey: kunci,
	}, "http://localhost:8080/v1"); err != nil {
		t.Fatalf("Configure crush: %v", err)
	}
	if _, err := mgr.Configure(ctx, ConfigureParams{
		ToolID: "crush", Mode: ModeModelOnly, Target: "gpt-5-mini",
		GatewayURL: "http://localhost:8080/v1",
	}, "http://localhost:8080/v1"); err != nil {
		t.Fatalf("Configure crush ulang tanpa kunci: %v", err)
	}
	crushPath := filepath.Join(home, ".config", "crush", "crush.json")
	var ccfg map[string]any
	if err := json.Unmarshal([]byte(bacaBerkasUji(t, crushPath)), &ccfg); err != nil {
		t.Fatalf("crush.json tidak valid: %v", err)
	}
	provCrush := ccfg["providers"].(map[string]any)["routex"].(map[string]any)
	if provCrush["api_key"] != kunci {
		t.Errorf("B-215 crush: api_key sesudah konfigurasi ulang = %v, mau kunci tersimpan %q", provCrush["api_key"], kunci)
	}
	// Kunci baru tetap menimpa kunci tersimpan.
	if _, err := mgr.Configure(ctx, ConfigureParams{
		ToolID: "crush", Mode: ModeModelOnly, Target: "gpt-5-mini",
		GatewayURL: "http://localhost:8080/v1", APIKey: kunciBaru,
	}, "http://localhost:8080/v1"); err != nil {
		t.Fatalf("Configure crush kunci baru: %v", err)
	}
	ccfg = make(map[string]any)
	if err := json.Unmarshal([]byte(bacaBerkasUji(t, crushPath)), &ccfg); err != nil {
		t.Fatal(err)
	}
	provCrush = ccfg["providers"].(map[string]any)["routex"].(map[string]any)
	if provCrush["api_key"] != kunciBaru {
		t.Errorf("crush: api_key sesudah kunci baru = %v, mau %q", provCrush["api_key"], kunciBaru)
	}

	// --- Claude ---
	if _, err := mgr.Configure(ctx, ConfigureParams{
		ToolID: "claude", Mode: ModeModelOnly, Target: "intern-s2",
		GatewayURL: "http://localhost:8080/v1", APIKey: kunci,
	}, "http://localhost:8080/v1"); err != nil {
		t.Fatalf("Configure claude: %v", err)
	}
	if _, err := mgr.Configure(ctx, ConfigureParams{
		ToolID: "claude", Mode: ModeModelOnly, Target: "model-lain",
		GatewayURL: "http://localhost:8080/v1",
	}, "http://localhost:8080/v1"); err != nil {
		t.Fatalf("Configure claude ulang tanpa kunci: %v", err)
	}
	var claudeCfg map[string]any
	if err := json.Unmarshal([]byte(bacaBerkasUji(t, filepath.Join(home, ".claude", "settings.json"))), &claudeCfg); err != nil {
		t.Fatalf("settings.json claude tidak valid: %v", err)
	}
	envClaude := claudeCfg["env"].(map[string]any)
	if envClaude["ANTHROPIC_API_KEY"] != kunci {
		t.Errorf("B-215 claude: ANTHROPIC_API_KEY sesudah konfigurasi ulang = %v, mau kunci tersimpan", envClaude["ANTHROPIC_API_KEY"])
	}

	// --- OpenCode ---
	if _, err := mgr.Configure(ctx, ConfigureParams{
		ToolID: "opencode", Mode: ModeModelOnly, Target: "intern-s2",
		GatewayURL: "http://localhost:8080/v1", APIKey: kunci,
	}, "http://localhost:8080/v1"); err != nil {
		t.Fatalf("Configure opencode: %v", err)
	}
	if _, err := mgr.Configure(ctx, ConfigureParams{
		ToolID: "opencode", Mode: ModeModelOnly, Target: "model-lain",
		GatewayURL: "http://localhost:8080/v1",
	}, "http://localhost:8080/v1"); err != nil {
		t.Fatalf("Configure opencode ulang tanpa kunci: %v", err)
	}
	var ocCfg map[string]any
	if err := json.Unmarshal([]byte(bacaBerkasUji(t, filepath.Join(home, ".config", "opencode", "opencode.json"))), &ocCfg); err != nil {
		t.Fatalf("opencode.json tidak valid: %v", err)
	}
	optsOC := ocCfg["provider"].(map[string]any)["routex"].(map[string]any)["options"].(map[string]any)
	if optsOC["apiKey"] != kunci {
		t.Errorf("B-215 opencode: apiKey sesudah konfigurasi ulang = %v, mau kunci tersimpan", optsOC["apiKey"])
	}

	// --- Hermes ---
	if _, err := mgr.Configure(ctx, ConfigureParams{
		ToolID: "hermes", Mode: ModeModelOnly, Target: "intern-s2",
		GatewayURL: "http://localhost:8080/v1", APIKey: kunci,
	}, "http://localhost:8080/v1"); err != nil {
		t.Fatalf("Configure hermes: %v", err)
	}
	if _, err := mgr.Configure(ctx, ConfigureParams{
		ToolID: "hermes", Mode: ModeModelOnly, Target: "model-lain",
		GatewayURL: "http://localhost:8080/v1",
	}, "http://localhost:8080/v1"); err != nil {
		t.Fatalf("Configure hermes ulang tanpa kunci: %v", err)
	}
	hermesKonten := bacaBerkasUji(t, filepath.Join(home, ".hermes", "config.yaml"))
	if !strings.Contains(hermesKonten, "api_key: "+kunci) {
		t.Errorf("B-215 hermes: blok model kehilangan api_key tersimpan sesudah konfigurasi ulang:\n%s", hermesKonten)
	}
}

// TestB216AiderMergeMempertahankanSetelanLain membuktikan Configure
// Aider hanya mengelola dua kunci (openai-api-base + model) di
// ~/.aider.conf.yml: baris kunci provider lain, opsi pengguna, dan
// komentar dipertahankan persis; kunci kelolaan diganti nilainya;
// duplikat kunci kelolaan disisakan satu.
func TestB216AiderMergeMempertahankanSetelanLain(t *testing.T) {
	home := t.TempDir()
	t.Setenv("HOME", home)
	ctx := context.Background()

	lama := "# konfigurasi aider saya\n" +
		"openai-api-key: GANTI_kunci-openai-pribadi\n" +
		"anthropic-api-key: GANTI_kunci-anthropic-pribadi\n" +
		"openai-api-base: http://lama.example/v1\n" +
		"model: openai/model-lama\n" +
		"dark-mode: true\n" +
		"auto-commits: false\n" +
		"# komentar pengguna di akhir\n"
	path := filepath.Join(home, ".aider.conf.yml")
	if err := os.WriteFile(path, []byte(lama), 0644); err != nil {
		t.Fatal(err)
	}

	mgr := NewManager(newMockSettingsStore(), nil, slog.New(slog.NewTextHandler(io.Discard, nil)))
	if _, err := mgr.Configure(ctx, ConfigureParams{
		ToolID: "aider", Mode: ModeModelOnly, Target: "intern-s2",
	}, "http://localhost:8080/v1"); err != nil {
		t.Fatalf("Configure aider: %v", err)
	}

	hasil := bacaBerkasUji(t, path)
	for _, mau := range []string{
		"# konfigurasi aider saya",
		"openai-api-key: GANTI_kunci-openai-pribadi",
		"anthropic-api-key: GANTI_kunci-anthropic-pribadi",
		"dark-mode: true",
		"auto-commits: false",
		"# komentar pengguna di akhir",
		"openai-api-base: http://localhost:8080/v1",
		"model: openai/intern-s2",
	} {
		if !strings.Contains(hasil, mau) {
			t.Errorf("B-216: hasil kehilangan/mengubah baris %q:\n%s", mau, hasil)
		}
	}
	if strings.Contains(hasil, "model-lama") || strings.Contains(hasil, "lama.example") {
		t.Errorf("B-216: nilai lama kunci kelolaan masih tersisa:\n%s", hasil)
	}
	if n := strings.Count(hasil, "openai-api-base:"); n != 1 {
		t.Errorf("B-216: openai-api-base muncul %d kali, mau 1:\n%s", n, hasil)
	}
	if n := strings.Count(hasil, "\nmodel:"); n != 1 {
		t.Errorf("B-216: kunci model muncul %d kali, mau 1:\n%s", n, hasil)
	}
}
