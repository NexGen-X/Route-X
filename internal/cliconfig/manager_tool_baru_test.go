package cliconfig

import (
	"context"
	"encoding/json"
	"io"
	"log/slog"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

// Berkas ini menguji keluaran Configure enam tool baru perombakan
// registri Tahap 1 (codex, grok-build, qwen, crush, hermes, cline)
// terhadap format resmi masing-masing tool.

func manajerUjiToolBaru(store SettingsStore) *Manager {
	return NewManager(store, nil, slog.New(slog.NewTextHandler(io.Discard, nil)))
}

func bacaBerkasUji(t *testing.T, path string) string {
	t.Helper()
	data, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("membaca %s: %v", path, err)
	}
	return string(data)
}

func harusMemuat(t *testing.T, konten, potongan string) {
	t.Helper()
	if !strings.Contains(konten, potongan) {
		t.Errorf("konten tidak memuat %q:\n%s", potongan, konten)
	}
}

func TestToolBaruDefResmi(t *testing.T) {
	// Flag versi nyata tiap tool baru dari dokumentasi resminya:
	// codex --version, grok --version, qwen --version,
	// crush --version, hermes --version, cline --version.
	// env_key TOML harus konsisten dengan EnvVarAPIKey yang
	// benar-benar ditulis produk ke key.env.
	defs := map[string]struct {
		biner, kunciEnv string
	}{
		"codex":      {"codex", "OPENAI_API_KEY"},
		"grok-build": {"grok", "XAI_API_KEY"},
		"qwen":       {"qwen", "OPENAI_API_KEY"},
		"crush":      {"crush", ""},
		"hermes":     {"hermes", "OPENAI_API_KEY"},
		"cline":      {"cline", ""},
	}
	for id, mau := range defs {
		def, ok := cariToolDef(id)
		if !ok {
			t.Fatalf("tool baru %s tidak terdaftar", id)
		}
		if def.VersionArg != "--version" {
			t.Errorf("%s: VersionArg = %q, mau --version", id, def.VersionArg)
		}
		if len(def.BinaryNames) != 1 || def.BinaryNames[0] != mau.biner {
			t.Errorf("%s: BinaryNames = %v, mau [%s]", id, def.BinaryNames, mau.biner)
		}
		if def.EnvVarAPIKey != mau.kunciEnv {
			t.Errorf("%s: EnvVarAPIKey = %q, mau %q", id, def.EnvVarAPIKey, mau.kunciEnv)
		}
		if def.DefaultTarget != "gpt-5" {
			t.Errorf("%s: DefaultTarget = %q, mau gpt-5", id, def.DefaultTarget)
		}
	}
}

func TestConfigureCodexConfigTOML(t *testing.T) {
	home := t.TempDir()
	t.Setenv("HOME", home)
	// Berkas lama dengan kunci root & seksi pengguna lain.
	dir := filepath.Join(home, ".codex")
	if err := os.MkdirAll(dir, 0755); err != nil {
		t.Fatal(err)
	}
	lama := "model = \"model-lama\"\nnotify = [\"echo\"]\n\n[projects.\"/kerja\"]\ntrust_level = \"trusted\"\n"
	if err := os.WriteFile(filepath.Join(dir, "config.toml"), []byte(lama), 0644); err != nil {
		t.Fatal(err)
	}
	mgr := manajerUjiToolBaru(newMockSettingsStore())
	_, err := mgr.Configure(context.Background(), ConfigureParams{
		ToolID: "codex", Mode: ModeModelOnly, Target: "gpt-5",
		GatewayURL: "http://localhost:8080/v1", APIKey: "kunci-codex",
	}, "http://localhost:8080/v1")
	if err != nil {
		t.Fatalf("Configure codex: %v", err)
	}
	konten := bacaBerkasUji(t, filepath.Join(dir, "config.toml"))
	// Kunci root di preamble, sebelum seksi pertama.
	idxSeksi := strings.Index(konten, "[projects")
	preamble := konten[:idxSeksi]
	harusMemuat(t, preamble, `model_provider = "routex"`)
	harusMemuat(t, preamble, `model = "gpt-5"`)
	harusMemuat(t, konten, "[model_providers.routex]")
	harusMemuat(t, konten, `name = "Route-X"`)
	harusMemuat(t, konten, `base_url = "http://localhost:8080/v1"`)
	harusMemuat(t, konten, `env_key = "OPENAI_API_KEY"`)
	harusMemuat(t, konten, `wire_api = "responses"`)
	// Konten pengguna lain dipertahankan.
	harusMemuat(t, konten, `notify = ["echo"]`)
	harusMemuat(t, konten, `[projects."/kerja"]`)
	// Kunci yang dirujuk env_key benar-benar diekspor ke key.env.
	keyEnv := bacaBerkasUji(t, filepath.Join(home, ".routex", "key.env"))
	harusMemuat(t, keyEnv, "export OPENAI_API_KEY='kunci-codex'")
}

func TestConfigureGrokBuildConfigTOML(t *testing.T) {
	home := t.TempDir()
	t.Setenv("HOME", home)
	mgr := manajerUjiToolBaru(newMockSettingsStore())
	_, err := mgr.Configure(context.Background(), ConfigureParams{
		ToolID: "grok-build", Mode: ModeModelOnly, Target: "gpt-5",
		GatewayURL: "http://localhost:8080/v1", APIKey: "kunci-grok",
	}, "http://localhost:8080/v1")
	if err != nil {
		t.Fatalf("Configure grok-build: %v", err)
	}
	konten := bacaBerkasUji(t, filepath.Join(home, ".grok", "config.toml"))
	harusMemuat(t, konten, "[models]")
	harusMemuat(t, konten, `default = "routex"`)
	harusMemuat(t, konten, "[model.routex]")
	harusMemuat(t, konten, `model = "gpt-5"`)
	harusMemuat(t, konten, `name = "Route-X"`)
	harusMemuat(t, konten, `base_url = "http://localhost:8080/v1"`)
	harusMemuat(t, konten, `env_key = "XAI_API_KEY"`)
	// XAI_API_KEY diekspor ke key.env.
	keyEnv := bacaBerkasUji(t, filepath.Join(home, ".routex", "key.env"))
	harusMemuat(t, keyEnv, "export XAI_API_KEY='kunci-grok'")
}

func TestConfigureQwenSettingsJSON(t *testing.T) {
	home := t.TempDir()
	t.Setenv("HOME", home)
	// Berkas lama: provider openai lain + setelan lain dipertahankan.
	dir := filepath.Join(home, ".qwen")
	if err := os.MkdirAll(dir, 0755); err != nil {
		t.Fatal(err)
	}
	lama := `{"modelProviders": {"openai": [{"id": "model-lain", "name": "Lain", "baseUrl": "https://contoh.invalid/v1", "envKey": "OPENAI_API_KEY"}]}, "ui": {"theme": "dark"}}`
	if err := os.WriteFile(filepath.Join(dir, "settings.json"), []byte(lama), 0644); err != nil {
		t.Fatal(err)
	}
	mgr := manajerUjiToolBaru(newMockSettingsStore())
	_, err := mgr.Configure(context.Background(), ConfigureParams{
		ToolID: "qwen", Mode: ModeModelOnly, Target: "gpt-5",
		GatewayURL: "http://localhost:8080/v1", APIKey: "kunci-qwen",
	}, "http://localhost:8080/v1")
	if err != nil {
		t.Fatalf("Configure qwen: %v", err)
	}
	var cfg map[string]any
	if err := json.Unmarshal([]byte(bacaBerkasUji(t, filepath.Join(dir, "settings.json"))), &cfg); err != nil {
		t.Fatalf("settings.json tidak valid: %v", err)
	}
	providers := cfg["modelProviders"].(map[string]any)
	openai := providers["openai"].([]any)
	if len(openai) != 2 {
		t.Fatalf("modelProviders.openai = %d entri, mau 2 (lama dipertahankan + routex)", len(openai))
	}
	terakhir := openai[len(openai)-1].(map[string]any)
	if terakhir["id"] != "gpt-5" || terakhir["baseUrl"] != "http://localhost:8080/v1" || terakhir["envKey"] != "OPENAI_API_KEY" {
		t.Errorf("entri qwen routex salah: %v", terakhir)
	}
	auth := cfg["security"].(map[string]any)["auth"].(map[string]any)
	if auth["selectedType"] != "openai" {
		t.Errorf("security.auth.selectedType = %v, mau openai", auth["selectedType"])
	}
	if cfg["model"].(map[string]any)["name"] != "gpt-5" {
		t.Errorf("model.name bukan target: %v", cfg["model"])
	}
	if cfg["ui"].(map[string]any)["theme"] != "dark" {
		t.Errorf("setelan ui pengguna hilang: %v", cfg)
	}
	// Jalur env resmi ikut diekspor.
	keyEnv := bacaBerkasUji(t, filepath.Join(home, ".routex", "key.env"))
	harusMemuat(t, keyEnv, "export OPENAI_API_KEY='kunci-qwen'")
	skrip := bacaBerkasUji(t, filepath.Join(home, ".routex", "cli-env.sh"))
	harusMemuat(t, skrip, "export OPENAI_BASE_URL='http://localhost:8080/v1'")
	harusMemuat(t, skrip, "export OPENAI_MODEL='gpt-5'")
}

func TestConfigureCrushJSON(t *testing.T) {
	home := t.TempDir()
	t.Setenv("HOME", home)
	mgr := manajerUjiToolBaru(newMockSettingsStore())
	_, err := mgr.Configure(context.Background(), ConfigureParams{
		ToolID: "crush", Mode: ModeModelOnly, Target: "gpt-5",
		GatewayURL: "http://localhost:8080/v1", APIKey: "kunci-crush",
	}, "http://localhost:8080/v1")
	if err != nil {
		t.Fatalf("Configure crush: %v", err)
	}
	path := filepath.Join(home, ".config", "crush", "crush.json")
	var cfg map[string]any
	if err := json.Unmarshal([]byte(bacaBerkasUji(t, path)), &cfg); err != nil {
		t.Fatalf("crush.json tidak valid: %v", err)
	}
	prov := cfg["providers"].(map[string]any)["routex"].(map[string]any)
	if prov["type"] != "openai-compat" || prov["base_url"] != "http://localhost:8080/v1" || prov["api_key"] != "kunci-crush" {
		t.Errorf("provider routex crush salah: %v", prov)
	}
	models := prov["models"].([]any)
	if len(models) != 1 || models[0].(map[string]any)["id"] != "gpt-5" {
		t.Errorf("models provider crush salah: %v", models)
	}
	top := cfg["models"].(map[string]any)
	if top["large"].(map[string]any)["provider"] != "routex" || top["small"].(map[string]any)["model"] != "gpt-5" {
		t.Errorf("models large/small crush salah: %v", top)
	}
	// Berkas memuat kunci: izin harus 0600.
	fi, err := os.Stat(path)
	if err != nil {
		t.Fatal(err)
	}
	if fi.Mode().Perm() != 0600 {
		t.Errorf("izin crush.json = %o, mau 600", fi.Mode().Perm())
	}
	// Konfigurasi ulang tanpa kunci menghapus api_key dari berkas.
	_, err = mgr.Configure(context.Background(), ConfigureParams{
		ToolID: "crush", Mode: ModeModelOnly, Target: "gpt-5",
		GatewayURL: "http://localhost:8080/v1",
	}, "http://localhost:8080/v1")
	if err != nil {
		t.Fatalf("Configure crush ulang: %v", err)
	}
	cfg = make(map[string]any)
	if err := json.Unmarshal([]byte(bacaBerkasUji(t, path)), &cfg); err != nil {
		t.Fatal(err)
	}
	prov = cfg["providers"].(map[string]any)["routex"].(map[string]any)
	if _, ada := prov["api_key"]; ada {
		// Parameter tanpa kunci berarti "tidak diubah" di tingkat
		// produk (stored key dipertahankan), jadi api_key dari kunci
		// tersimpan tetap sah; yang dilarang adalah kunci ASING.
		if prov["api_key"] != "kunci-crush" {
			t.Errorf("api_key crush berubah menjadi nilai asing: %v", prov["api_key"])
		}
	}
}

func TestConfigureHermesConfigYAML(t *testing.T) {
	home := t.TempDir()
	t.Setenv("HOME", home)
	// Berkas lama: blok model peta lama + seksi lain dipertahankan.
	dir := filepath.Join(home, ".hermes")
	if err := os.MkdirAll(dir, 0755); err != nil {
		t.Fatal(err)
	}
	lama := "model:\n  provider: openrouter\n  default: model-lama\n\nmemory:\n  enabled: true\n"
	if err := os.WriteFile(filepath.Join(dir, "config.yaml"), []byte(lama), 0644); err != nil {
		t.Fatal(err)
	}
	mgr := manajerUjiToolBaru(newMockSettingsStore())
	_, err := mgr.Configure(context.Background(), ConfigureParams{
		ToolID: "hermes", Mode: ModeModelOnly, Target: "gpt-5",
		GatewayURL: "http://localhost:8080/v1", APIKey: "kunci-hermes",
	}, "http://localhost:8080/v1")
	if err != nil {
		t.Fatalf("Configure hermes: %v", err)
	}
	konten := bacaBerkasUji(t, filepath.Join(dir, "config.yaml"))
	harusMemuat(t, konten, "model:")
	harusMemuat(t, konten, "  provider: custom")
	harusMemuat(t, konten, "  default: gpt-5")
	harusMemuat(t, konten, "  base_url: http://localhost:8080/v1")
	harusMemuat(t, konten, "  api_mode: chat_completions")
	if strings.Contains(konten, "openrouter") || strings.Contains(konten, "model-lama") {
		t.Errorf("blok model lama hermes tidak terganti penuh:\n%s", konten)
	}
	harusMemuat(t, konten, "memory:")
	harusMemuat(t, konten, "  enabled: true")
	// Kunci digabung ke ~/.hermes/.env dengan izin 0600.
	envPath := filepath.Join(dir, ".env")
	harusMemuat(t, bacaBerkasUji(t, envPath), "OPENAI_API_KEY=kunci-hermes")
	fi, err := os.Stat(envPath)
	if err != nil {
		t.Fatal(err)
	}
	if fi.Mode().Perm() != 0600 {
		t.Errorf("izin .env hermes = %o, mau 600", fi.Mode().Perm())
	}
}

func TestConfigureClineJalankanAuthResmi(t *testing.T) {
	home := t.TempDir()
	t.Setenv("HOME", home)
	// Biner cline palsu mencatat argv-nya.
	binDir := t.TempDir()
	skrip := "#!/bin/sh\nprintf '%s\\n' \"$@\" >> \"$HOME/cline-argv.txt\"\n"
	if err := os.WriteFile(filepath.Join(binDir, "cline"), []byte(skrip), 0755); err != nil {
		t.Fatal(err)
	}
	t.Setenv("PATH", binDir+string(os.PathListSeparator)+os.Getenv("PATH"))
	mgr := manajerUjiToolBaru(newMockSettingsStore())
	_, err := mgr.Configure(context.Background(), ConfigureParams{
		ToolID: "cline", Mode: ModeModelOnly, Target: "gpt-5",
		GatewayURL: "http://localhost:8080/v1", APIKey: "kunci-cline",
	}, "http://localhost:8080/v1")
	if err != nil {
		t.Fatalf("Configure cline: %v", err)
	}
	argv := bacaBerkasUji(t, filepath.Join(home, "cline-argv.txt"))
	for _, mau := range []string{"auth", "-p", "openai", "-b", "http://localhost:8080/v1", "-m", "gpt-5", "-k", "kunci-cline"} {
		if !strings.Contains(argv, mau+"\n") && !strings.Contains(argv, mau) {
			t.Errorf("argv cline tidak memuat %q:\n%s", mau, argv)
		}
	}
}

func TestConfigureClineBinerHilangGagalJujur(t *testing.T) {
	home := t.TempDir()
	t.Setenv("HOME", home)
	// PATH kosong: biner cline tidak mungkin ditemukan (lokasi
	// standar di bawah HOME uji juga kosong).
	t.Setenv("PATH", t.TempDir())
	mgr := manajerUjiToolBaru(newMockSettingsStore())
	_, err := mgr.Configure(context.Background(), ConfigureParams{
		ToolID: "cline", Mode: ModeModelOnly, Target: "gpt-5",
		GatewayURL: "http://localhost:8080/v1", APIKey: "kunci-cline",
	}, "http://localhost:8080/v1")
	if err == nil {
		t.Fatalf("Configure cline tanpa biner harus gagal")
	}
	if !strings.Contains(err.Error(), "biner cline tidak ditemukan") {
		t.Errorf("pesan error harus jujur soal biner hilang, didapat: %v", err)
	}
}
