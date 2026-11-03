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

	"github.com/NexGen-X/Route-X/internal/database/repo/identity"
	"github.com/NexGen-X/Route-X/internal/security"
)

// mockSettingsStore mengimplementasikan SettingsStore di memori untuk pengujian.
type mockSettingsStore struct {
	data map[string]identity.Setting
}

func newMockSettingsStore() *mockSettingsStore {
	return &mockSettingsStore{data: make(map[string]identity.Setting)}
}

func (m *mockSettingsStore) Get(ctx context.Context, key string) (identity.Setting, error) {
	if s, ok := m.data[key]; ok {
		return s, nil
	}
	return identity.Setting{}, os.ErrNotExist
}

func (m *mockSettingsStore) Put(ctx context.Context, s identity.SettingWrite) (identity.Setting, error) {
	res := identity.Setting{
		Key:         s.Key,
		Value:       s.Value,
		Description: s.Description,
	}
	m.data[s.Key] = res
	return res, nil
}

func (m *mockSettingsStore) All(ctx context.Context) ([]identity.Setting, error) {
	out := make([]identity.Setting, 0, len(m.data))
	for _, v := range m.data {
		out = append(out, v)
	}
	return out, nil
}

func TestSupportedTools(t *testing.T) {
	tools := SupportedTools()
	if len(tools) < 15 {
		t.Fatalf("diharapkan minimal 15 tools terdaftar, didapat %d", len(tools))
	}

	seen := make(map[string]bool)
	for _, tool := range tools {
		if tool.ID == "" {
			t.Errorf("tool %+v memiliki ID kosong", tool)
		}
		if seen[tool.ID] {
			t.Errorf("ID tool duplikat: %s", tool.ID)
		}
		seen[tool.ID] = true
	}
}

func TestManagerScanAndConfigure(t *testing.T) {
	ctx := context.Background()
	logger := slog.New(slog.NewTextHandler(io.Discard, nil))
	store := newMockSettingsStore()
	mgr := NewManager(store, nil, logger)

	// 1. Uji Scan awal
	statuses, err := mgr.Scan(ctx, "http://localhost:8080/v1")
	if err != nil {
		t.Fatalf("Scan gagal: %v", err)
	}
	if len(statuses) != len(SupportedTools()) {
		t.Fatalf("jumlah tool status (%d) tidak cocok dengan definisi (%d)", len(statuses), len(SupportedTools()))
	}

	// 2. Uji Konfigurasi Mode 1: Model Only
	res, err := mgr.Configure(ctx, ConfigureParams{
		ToolID: "claude",
		Mode:   ModeModelOnly,
		Target: "claude-3-7-sonnet",
	}, "http://localhost:8080/v1")
	if err != nil {
		t.Fatalf("Configure mode model_only gagal: %v", err)
	}
	if res.ActiveMode != ModeModelOnly || res.ActiveTarget != "claude-3-7-sonnet" {
		t.Errorf("hasil configure tidak cocok: %+v", res)
	}

	// 3. Uji Konfigurasi Mode 2: Routing
	res, err = mgr.Configure(ctx, ConfigureParams{
		ToolID: "agy",
		Mode:   ModeRouting,
		Target: "coding-fast",
	}, "http://localhost:8080/v1")
	if err != nil {
		t.Fatalf("Configure mode routing gagal: %v", err)
	}
	if res.ActiveMode != ModeRouting || res.ActiveTarget != "coding-fast" {
		t.Errorf("hasil configure routing tidak cocok: %+v", res)
	}

	// 4. Uji Konfigurasi Mode 3: Combo Routing
	res, err = mgr.Configure(ctx, ConfigureParams{
		ToolID: "aider",
		Mode:   ModeCombo,
		Target: "combo:qwen2.5-coder->claude-3-7-sonnet",
	}, "http://localhost:8080/v1")
	if err != nil {
		t.Fatalf("Configure mode combo gagal: %v", err)
	}
	if res.ActiveMode != ModeCombo || res.ActiveTarget != "combo:qwen2.5-coder->claude-3-7-sonnet" {
		t.Errorf("hasil configure combo tidak cocok: %+v", res)
	}

	// 5. Uji Validasi Mode Tidak Valid
	_, err = mgr.Configure(ctx, ConfigureParams{
		ToolID: "aider",
		Mode:   "invalid_mode",
	}, "http://localhost:8080/v1")
	if err == nil {
		t.Fatalf("seharusnya error ketika mode tidak valid")
	}

	// 6. Uji persistensi di store
	setting, err := store.Get(ctx, "cli:config:claude")
	if err != nil {
		t.Fatalf("setting claude tidak ditemukan di store: %v", err)
	}
	var stored StoredConfig
	if err := json.Unmarshal(setting.Value, &stored); err != nil {
		t.Fatalf("unmarshal setting gagal: %v", err)
	}
	if stored.Mode != ModeModelOnly || stored.Target != "claude-3-7-sonnet" {
		t.Errorf("stored config tidak cocok: %+v", stored)
	}
}

// TestConfigureEncryptsAndRedactsAPIKey membuktikan DATA-001 teratasi:
// key tidak tersimpan plaintext di settings, tidak kembali plaintext di respons,
// dan tetap dapat didekripsi kembali lewat jalur internal.
func TestConfigureEncryptsAndRedactsAPIKey(t *testing.T) {
	t.Setenv("HOME", t.TempDir())
	ctx := context.Background()
	logger := slog.New(slog.NewTextHandler(io.Discard, nil))
	store := newMockSettingsStore()

	cipher, err := security.NewCipher(bytes.Repeat([]byte{0x42}, 32))
	if err != nil {
		t.Fatalf("membuat cipher: %v", err)
	}
	mgr := NewManager(store, cipher, logger)

	const secretKey = "rx_live_rahasia1234567890"

	res, err := mgr.Configure(ctx, ConfigureParams{
		ToolID: "claude",
		Mode:   ModeModelOnly,
		Target: "claude-3-7-sonnet",
		APIKey: secretKey,
	}, "http://localhost:8080/v1")
	if err != nil {
		t.Fatalf("Configure gagal: %v", err)
	}

	// 1. Settings tidak memuat plaintext — hanya ciphertext.
	setting, err := store.Get(ctx, "cli:config:claude")
	if err != nil {
		t.Fatalf("setting tidak ditemukan: %v", err)
	}
	if bytes.Contains(setting.Value, []byte(secretKey)) {
		t.Fatalf("plaintext key masih ada di settings: %s", setting.Value)
	}
	var stored StoredConfig
	if err := json.Unmarshal(setting.Value, &stored); err != nil {
		t.Fatalf("unmarshal setting gagal: %v", err)
	}
	if stored.APIKey != "" {
		t.Errorf("field api_key legacy masih diisi: %q", stored.APIKey)
	}
	if stored.APIKeyEncrypted == "" {
		t.Fatalf("field api_key_enc kosong; enkripsi tidak berjalan")
	}

	// 2. Respons tidak membawa plaintext di EnvVars maupun snippet.
	respJSON, _ := json.Marshal(res)
	if bytes.Contains(respJSON, []byte(secretKey)) {
		t.Fatalf("respons Configure masih memuat plaintext key: %s", respJSON)
	}
	for name, val := range res.EnvVars {
		if strings.Contains(val, secretKey) {
			t.Errorf("EnvVars[%s] memuat plaintext key", name)
		}
	}
	if strings.Contains(res.ExportSnippet, "API_KEY") {
		t.Errorf("ExportSnippet masih memuat variabel API key: %s", res.ExportSnippet)
	}

	// 3. Jalur internal masih bisa mendekripsi (roundtrip).
	plain, err := cipher.Decrypt(stored.APIKeyEncrypted, security.CLIToolAAD("claude"))
	if err != nil {
		t.Fatalf("decrypt gagal: %v", err)
	}
	if string(plain) != secretKey {
		t.Errorf("roundtrip tidak cocok: didapat panjang %d", len(plain))
	}

	// 4. Scan juga hanya menampilkan versi tersamar.
	statuses, err := mgr.Scan(ctx, "http://localhost:8080/v1")
	if err != nil {
		t.Fatalf("Scan gagal: %v", err)
	}
	for _, st := range statuses {
		for name, val := range st.EnvVars {
			if strings.Contains(val, secretKey) {
				t.Errorf("Scan EnvVars[%s] memuat plaintext key (tool %s)", name, st.ID)
			}
		}
	}

	homeDir, err := os.UserHomeDir()
	if err != nil {
		t.Fatalf("membaca home dir: %v", err)
	}
	script, err := os.ReadFile(filepath.Join(homeDir, ".routex", "cli-env.sh"))
	if err != nil {
		t.Fatalf("membaca skrip CLI: %v", err)
	}
	if bytes.Contains(script, []byte(secretKey)) {
		t.Fatal("plaintext key masih ditulis ke skrip CLI")
	}
}

func TestScanMigratesLegacyAPIKey(t *testing.T) {
	ctx := context.Background()
	store := newMockSettingsStore()
	legacy := StoredConfig{Mode: ModeModelOnly, Target: "model", APIKey: "legacy-secret"}
	raw, _ := json.Marshal(legacy)
	store.data["cli:config:claude"] = identity.Setting{Key: "cli:config:claude", Value: raw}
	cipher, err := security.NewCipher(bytes.Repeat([]byte{0x24}, 32))
	if err != nil {
		t.Fatal(err)
	}
	mgr := NewManager(store, cipher, slog.New(slog.NewTextHandler(io.Discard, nil)))

	if _, err := mgr.Scan(ctx, "http://localhost:8080/v1"); err != nil {
		t.Fatalf("Scan gagal: %v", err)
	}
	setting := store.data["cli:config:claude"]
	if bytes.Contains(setting.Value, []byte("legacy-secret")) {
		t.Fatal("API key legacy belum dihapus dari settings")
	}
	var migrated StoredConfig
	if err := json.Unmarshal(setting.Value, &migrated); err != nil {
		t.Fatal(err)
	}
	if migrated.APIKey != "" || migrated.APIKeyEncrypted == "" {
		t.Fatalf("hasil migrasi tidak sah: %+v", migrated)
	}
}

// TestConfigureClaudeSettingsSkemaEnv membuktikan B-097: settings.json
// Claude Code ditulis dalam skema yang benar-benar dibaca tool — objek
// `env` berisi ANTHROPIC_BASE_URL (tanpa /v1), ANTHROPIC_MODEL, dan
// ANTHROPIC_API_KEY bila key diberikan — bukan kunci root snake_case
// warisan. Setelan pengguna lain dipertahankan, sisa kunci root salah
// dibuang, dan berkas berizin 0600 karena bisa memuat key.
func TestConfigureClaudeSettingsSkemaEnv(t *testing.T) {
	home := t.TempDir()
	t.Setenv("HOME", home)
	claudeDir := filepath.Join(home, ".claude")
	if err := os.MkdirAll(claudeDir, 0755); err != nil {
		t.Fatal(err)
	}
	lama := `{"anthropic_base_url":"http://lama","env":{"VARIABEL_LAIN":"tetap"},"permissions":{"allow":["Bash"]}}`
	if err := os.WriteFile(filepath.Join(claudeDir, "settings.json"), []byte(lama), 0644); err != nil {
		t.Fatal(err)
	}

	ctx := context.Background()
	mgr := NewManager(newMockSettingsStore(), nil, slog.New(slog.NewTextHandler(io.Discard, nil)))
	res, err := mgr.Configure(ctx, ConfigureParams{
		ToolID: "claude", Mode: ModeModelOnly, Target: "intern-s2",
		APIKey: "kunci-uji-claude-123",
	}, "http://localhost:8080/v1")
	if err != nil {
		t.Fatalf("Configure claude: %v", err)
	}
	if res.EnvVars["ANTHROPIC_BASE_URL"] != "http://localhost:8080" {
		t.Errorf("EnvVars ANTHROPIC_BASE_URL = %q, mau tanpa /v1", res.EnvVars["ANTHROPIC_BASE_URL"])
	}

	settingsPath := filepath.Join(claudeDir, "settings.json")
	fi, err := os.Stat(settingsPath)
	if err != nil {
		t.Fatalf("settings.json tidak ditulis: %v", err)
	}
	if fi.Mode().Perm() != 0600 {
		t.Errorf("izin settings.json = %o, mau 600 (bisa memuat API key)", fi.Mode().Perm())
	}
	data, err := os.ReadFile(settingsPath)
	if err != nil {
		t.Fatal(err)
	}
	var cfg map[string]any
	if err := json.Unmarshal(data, &cfg); err != nil {
		t.Fatalf("settings.json bukan JSON sah: %v", err)
	}
	if _, ada := cfg["anthropic_base_url"]; ada {
		t.Errorf("kunci root anthropic_base_url warisan masih ada: %s", data)
	}
	env, ok := cfg["env"].(map[string]any)
	if !ok {
		t.Fatalf("settings.json tidak punya objek env: %s", data)
	}
	if env["ANTHROPIC_BASE_URL"] != "http://localhost:8080" {
		t.Errorf("env.ANTHROPIC_BASE_URL = %v, mau http://localhost:8080", env["ANTHROPIC_BASE_URL"])
	}
	if env["ANTHROPIC_MODEL"] != "intern-s2" {
		t.Errorf("env.ANTHROPIC_MODEL = %v, mau intern-s2", env["ANTHROPIC_MODEL"])
	}
	if env["ANTHROPIC_API_KEY"] != "kunci-uji-claude-123" {
		t.Errorf("env.ANTHROPIC_API_KEY = %v, mau key yang dikonfigurasi", env["ANTHROPIC_API_KEY"])
	}
	if env["VARIABEL_LAIN"] != "tetap" {
		t.Errorf("variabel env pengguna lain hilang: %v", env)
	}
	if cfg["model"] != "intern-s2" {
		t.Errorf("model tingkat-atas = %v, mau intern-s2", cfg["model"])
	}
	if _, ok := cfg["permissions"]; !ok {
		t.Errorf("setelan permissions pengguna hilang: %s", data)
	}
}

// TestConfigureAiderPrefiksOpenAI membuktikan B-098: nama model Aider
// ditulis `openai/<model>` di .aider.conf.yml DAN di env AIDER_MODEL.
func TestConfigureAiderPrefiksOpenAI(t *testing.T) {
	home := t.TempDir()
	t.Setenv("HOME", home)
	ctx := context.Background()
	mgr := NewManager(newMockSettingsStore(), nil, slog.New(slog.NewTextHandler(io.Discard, nil)))
	res, err := mgr.Configure(ctx, ConfigureParams{
		ToolID: "aider", Mode: ModeModelOnly, Target: "intern-s2",
	}, "http://localhost:8080/v1")
	if err != nil {
		t.Fatalf("Configure aider: %v", err)
	}
	if res.EnvVars["AIDER_MODEL"] != "openai/intern-s2" {
		t.Errorf("AIDER_MODEL = %q, mau openai/intern-s2", res.EnvVars["AIDER_MODEL"])
	}
	if strings.Contains(res.ExportSnippet, "AIDER_MODEL='intern-s2'") {
		t.Errorf("snippet masih memuat AIDER_MODEL tanpa prefiks: %s", res.ExportSnippet)
	}
	data, err := os.ReadFile(filepath.Join(home, ".aider.conf.yml"))
	if err != nil {
		t.Fatalf(".aider.conf.yml tidak ditulis: %v", err)
	}
	if !strings.Contains(string(data), "model: openai/intern-s2") {
		t.Errorf(".aider.conf.yml tidak memuat model berprefiks:\n%s", data)
	}
	if !strings.Contains(string(data), "openai-api-base: http://localhost:8080/v1") {
		t.Errorf(".aider.conf.yml tidak memuat openai-api-base gateway:\n%s", data)
	}
	if modelAider("openai/sudah") != "openai/sudah" {
		t.Error("modelAider tidak boleh menggandakan prefiks openai/")
	}
}

// TestConfigureGooseConfigDanEnv membuktikan B-099 dan B-100: Goose
// mendapat config.yaml yang sah (GOOSE_PROVIDER/GOOSE_MODEL/OPENAI_HOST
// tanpa /v1, blok pengguna lain dipertahankan, TANPA api key di berkas),
// ekspor env yang benar-benar dibacanya, dan VersionArg --version.
func TestConfigureGooseConfigDanEnv(t *testing.T) {
	home := t.TempDir()
	t.Setenv("HOME", home)
	gooseDir := filepath.Join(home, ".config", "goose")
	if err := os.MkdirAll(gooseDir, 0755); err != nil {
		t.Fatal(err)
	}
	lama := "GOOSE_MODEL: model-lama\nextensions:\n  developer:\n    enabled: true\n"
	if err := os.WriteFile(filepath.Join(gooseDir, "config.yaml"), []byte(lama), 0644); err != nil {
		t.Fatal(err)
	}

	def, ok := cariToolDef("goose")
	if !ok {
		t.Fatal("goose tidak ditemukan")
	}
	if def.VersionArg != "--version" {
		t.Errorf("VersionArg goose = %q, mau --version (B-100)", def.VersionArg)
	}
	if def.EnvVarBaseURL != "OPENAI_HOST" || def.EnvVarAPIKey != "OPENAI_API_KEY" {
		t.Errorf("definisi env goose = %q/%q, mau OPENAI_HOST/OPENAI_API_KEY", def.EnvVarBaseURL, def.EnvVarAPIKey)
	}

	ctx := context.Background()
	mgr := NewManager(newMockSettingsStore(), nil, slog.New(slog.NewTextHandler(io.Discard, nil)))
	res, err := mgr.Configure(ctx, ConfigureParams{
		ToolID: "goose", Mode: ModeModelOnly, Target: "intern-s2",
		APIKey: "kunci-uji-goose-123",
	}, "http://localhost:8080/v1")
	if err != nil {
		t.Fatalf("Configure goose: %v", err)
	}
	if res.EnvVars["GOOSE_PROVIDER"] != "openai" {
		t.Errorf("GOOSE_PROVIDER = %q, mau openai", res.EnvVars["GOOSE_PROVIDER"])
	}
	if res.EnvVars["OPENAI_HOST"] != "http://localhost:8080" {
		t.Errorf("OPENAI_HOST = %q, mau tanpa /v1", res.EnvVars["OPENAI_HOST"])
	}
	if res.EnvVars["GOOSE_MODEL"] != "intern-s2" {
		t.Errorf("GOOSE_MODEL = %q, mau intern-s2", res.EnvVars["GOOSE_MODEL"])
	}

	data, err := os.ReadFile(filepath.Join(gooseDir, "config.yaml"))
	if err != nil {
		t.Fatalf("config.yaml goose tidak ditulis: %v", err)
	}
	konten := string(data)
	for _, mau := range []string{"GOOSE_PROVIDER: openai", "GOOSE_MODEL: intern-s2", "OPENAI_HOST: http://localhost:8080", "extensions:", "developer:"} {
		if !strings.Contains(konten, mau) {
			t.Errorf("config.yaml goose tidak memuat %q:\n%s", mau, konten)
		}
	}
	if strings.Count(konten, "GOOSE_MODEL:") != 1 {
		t.Errorf("GOOSE_MODEL harus tepat satu sesudah pembaruan:\n%s", konten)
	}
	for _, terlarang := range []string{"model-lama", "kunci-uji-goose-123", "GOOSE_API_KEY", "GOOSE_OPENAI_HOST", "/v1"} {
		if strings.Contains(konten, terlarang) {
			t.Errorf("config.yaml goose memuat %q yang tidak sah:\n%s", terlarang, konten)
		}
	}

	keyEnv, err := os.ReadFile(filepath.Join(home, ".routex", "key.env"))
	if err != nil {
		t.Fatalf("key.env tidak ada: %v", err)
	}
	if !strings.Contains(string(keyEnv), "OPENAI_API_KEY='kunci-uji-goose-123'") {
		t.Errorf("key.env harus mengekspor kunci goose sebagai OPENAI_API_KEY:\n%s", keyEnv)
	}
	if strings.Contains(string(keyEnv), "GOOSE_API_KEY") {
		t.Errorf("key.env masih mengekspor GOOSE_API_KEY:\n%s", keyEnv)
	}
}

// TestConfigureContinueConfigLokal membuktikan B-101 (keamanan): cn
// mendapat ~/.continue/config.yaml yang sah menunjuk gateway (provider
// openai, apiBase dengan /v1, apiKey key produk, izin 0600), dan tidak
// ada lagi ekspor variabel rekaan CONTINUE_* di hasil Configure maupun
// cli-env.sh.
func TestConfigureContinueConfigLokal(t *testing.T) {
	home := t.TempDir()
	t.Setenv("HOME", home)
	ctx := context.Background()
	mgr := NewManager(newMockSettingsStore(), nil, slog.New(slog.NewTextHandler(io.Discard, nil)))
	res, err := mgr.Configure(ctx, ConfigureParams{
		ToolID: "continue", Mode: ModeModelOnly, Target: "intern-s2",
		APIKey: "kunci-uji-continue-123",
	}, "http://localhost:8080/v1")
	if err != nil {
		t.Fatalf("Configure continue: %v", err)
	}
	if len(res.EnvVars) != 0 {
		t.Errorf("EnvVars continue harus kosong (cn tidak membaca env khusus), didapat %v", res.EnvVars)
	}
	if strings.Contains(res.ExportSnippet, "CONTINUE_") {
		t.Errorf("snippet continue masih memuat variabel rekaan: %s", res.ExportSnippet)
	}

	cfgPath := filepath.Join(home, ".continue", "config.yaml")
	fi, err := os.Stat(cfgPath)
	if err != nil {
		t.Fatalf("~/.continue/config.yaml tidak ditulis: %v", err)
	}
	if fi.Mode().Perm() != 0600 {
		t.Errorf("izin config.yaml continue = %o, mau 600 (memuat apiKey)", fi.Mode().Perm())
	}
	data, err := os.ReadFile(cfgPath)
	if err != nil {
		t.Fatal(err)
	}
	konten := string(data)
	for _, mau := range []string{
		"name: Route-X", "schema: v1", "provider: openai",
		"model: intern-s2", "apiBase: http://localhost:8080/v1",
		"apiKey: kunci-uji-continue-123",
	} {
		if !strings.Contains(konten, mau) {
			t.Errorf("config.yaml continue tidak memuat %q:\n%s", mau, konten)
		}
	}

	script, err := os.ReadFile(filepath.Join(home, ".routex", "cli-env.sh"))
	if err != nil {
		t.Fatalf("cli-env.sh tidak ada: %v", err)
	}
	if strings.Contains(string(script), "CONTINUE_API") || strings.Contains(string(script), "CONTINUE_MODEL") {
		t.Errorf("cli-env.sh masih mengekspor variabel rekaan Continue:\n%s", script)
	}

	def, ok := cariToolDef("continue")
	if !ok {
		t.Fatal("continue tidak ditemukan")
	}
	if def.EnvVarBaseURL != "" || def.EnvVarModel != "" || def.EnvVarAPIKey != "OPENAI_API_KEY" {
		t.Errorf("definisi env continue = %q/%q/%q, mau kosong/kosong/OPENAI_API_KEY",
			def.EnvVarBaseURL, def.EnvVarAPIKey, def.EnvVarModel)
	}
}
