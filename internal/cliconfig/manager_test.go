package cliconfig

import (
	"context"
	"encoding/json"
	"io"
	"log/slog"
	"os"
	"testing"

	"github.com/NexGen-X/Route-X/internal/database/repo/identity"
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
	mgr := NewManager(store, logger)

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
