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
