package cliconfig

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"log/slog"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"sync"
	"time"

	"github.com/NexGen-X/Route-X/internal/database/repo/identity"
)

// SettingsStore adalah antarmuka minimum untuk membaca dan menyimpan preferensi setelan CLI.
type SettingsStore interface {
	Get(ctx context.Context, key string) (identity.Setting, error)
	Put(ctx context.Context, s identity.SettingWrite) (identity.Setting, error)
	All(ctx context.Context) ([]identity.Setting, error)
}

// Manager mengelola pemindaian perkakas CLI dan penerapan konfigurasi multi-mode.
type Manager struct {
	settings SettingsStore
	logger   *slog.Logger
	mu       sync.RWMutex
}

// NewManager membuat instance baru Manager.
func NewManager(settings SettingsStore, logger *slog.Logger) *Manager {
	return &Manager{
		settings: settings,
		logger:   logger,
	}
}

// StoredConfig merepresentasikan format JSON yang disimpan di tabel settings.
type StoredConfig struct {
	Mode       string    `json:"mode"`
	Target     string    `json:"target"`
	APIKey     string    `json:"api_key,omitempty"`
	GatewayURL string    `json:"gateway_url,omitempty"`
	UpdatedAt  time.Time `json:"updated_at"`
}

// Scan memeriksa keberadaan perkakas CLI di sistem dan memuat konfigurasi aktifnya.
func (m *Manager) Scan(ctx context.Context, defaultGatewayURL string) ([]ToolStatus, error) {
	m.mu.RLock()
	defer m.mu.RUnlock()

	tools := SupportedTools()
	results := make([]ToolStatus, 0, len(tools))
	homeDir, _ := os.UserHomeDir()

	if defaultGatewayURL == "" {
		defaultGatewayURL = "http://localhost:8080/v1"
	}

	for _, tool := range tools {
		status := ToolStatus{
			ID:             tool.ID,
			Name:           tool.Name,
			Description:    tool.Description,
			Category:       tool.Category,
			SupportedModes: []string{ModeModelOnly, ModeRouting, ModeCombo},
			ActiveMode:     tool.DefaultMode,
			ActiveTarget:   tool.DefaultTarget,
			EnvVars:        make(map[string]string),
		}

		// 1. Deteksi executable path
		execPath := m.findBinary(tool.BinaryNames, homeDir)
		if execPath != "" {
			status.Installed = true
			status.Path = execPath
			status.Version = m.detectVersion(ctx, execPath, tool.VersionArg)
		}

		// 2. Deteksi lokasi config file jika ada di sistem
		for _, relConfig := range tool.ConfigPaths {
			fullPath := filepath.Join(homeDir, relConfig)
			if _, err := os.Stat(fullPath); err == nil {
				status.ConfigPath = fullPath
				break
			}
		}

		// 3. Baca preferensi tersimpan di database jika ada
		settingKey := fmt.Sprintf("cli:config:%s", tool.ID)
		if m.settings != nil {
			if setting, err := m.settings.Get(ctx, settingKey); err == nil && len(setting.Value) > 0 {
				var stored StoredConfig
				if err := json.Unmarshal(setting.Value, &stored); err == nil {
					if stored.Mode != "" {
						status.ActiveMode = stored.Mode
					}
					if stored.Target != "" {
						status.ActiveTarget = stored.Target
					}
					status.UpdatedAt = stored.UpdatedAt
				}
			}
		}

		// 4. Susun Environment Variables dan Export Snippet
		status.EnvVars = m.buildEnvVars(tool, status.ActiveMode, status.ActiveTarget, defaultGatewayURL)
		status.ExportSnippet = m.buildSnippet(status.EnvVars)

		results = append(results, status)
	}

	return results, nil
}

// Configure menerapkan konfigurasi baru untuk suatu tool dan menyimpannya secara persisten.
func (m *Manager) Configure(ctx context.Context, p ConfigureParams, defaultGatewayURL string) (*ToolStatus, error) {
	m.mu.Lock()
	defer m.mu.Unlock()

	toolDef, found := m.findToolDef(p.ToolID)
	if !found {
		return nil, fmt.Errorf("perkakas CLI dengan id '%s' tidak didukung", p.ToolID)
	}

	// Validasi mode
	if p.Mode != ModeModelOnly && p.Mode != ModeRouting && p.Mode != ModeCombo {
		return nil, fmt.Errorf("mode '%s' tidak valid; pilih model_only, routing, atau combo", p.Mode)
	}

	if p.Target == "" {
		p.Target = toolDef.DefaultTarget
	}

	gwURL := p.GatewayURL
	if gwURL == "" {
		gwURL = defaultGatewayURL
		if gwURL == "" {
			gwURL = "http://localhost:8080/v1"
		}
	}

	stored := StoredConfig{
		Mode:       p.Mode,
		Target:     p.Target,
		APIKey:     p.APIKey,
		GatewayURL: gwURL,
		UpdatedAt:  time.Now().UTC(),
	}

	rawJSON, err := json.Marshal(stored)
	if err != nil {
		return nil, fmt.Errorf("mengkodekan konfigurasi CLI: %w", err)
	}

	// 1. Simpan ke tabel settings database
	if m.settings != nil {
		settingKey := fmt.Sprintf("cli:config:%s", p.ToolID)
		_, err = m.settings.Put(ctx, identity.SettingWrite{
			Key:         settingKey,
			Value:       rawJSON,
			Description: fmt.Sprintf("Konfigurasi integrasi CLI untuk %s", toolDef.Name),
		})
		if err != nil {
			m.logger.Warn("gagal menyimpan konfigurasi CLI ke database", "tool", p.ToolID, "error", err)
		}
	}

	// 2. Tulis berkas shell script lingkungan terpadu ~/.routex/cli-env.sh
	homeDir, _ := os.UserHomeDir()
	envVars := m.buildEnvVars(toolDef, p.Mode, p.Target, gwURL)
	if p.APIKey != "" && toolDef.EnvVarAPIKey != "" {
		envVars[toolDef.EnvVarAPIKey] = p.APIKey
	}

	_ = m.syncGlobalEnvScript(homeDir, toolDef, envVars)

	// 3. Tulis file konfigurasi lokal spesifik jika ada direktorinya
	_ = m.applyToolSpecificConfig(homeDir, toolDef, p.Mode, p.Target, gwURL, p.APIKey)

	execPath := m.findBinary(toolDef.BinaryNames, homeDir)
	status := &ToolStatus{
		ID:             toolDef.ID,
		Name:           toolDef.Name,
		Description:    toolDef.Description,
		Category:       toolDef.Category,
		Installed:      execPath != "",
		Path:           execPath,
		Version:        m.detectVersion(ctx, execPath, toolDef.VersionArg),
		SupportedModes: []string{ModeModelOnly, ModeRouting, ModeCombo},
		ActiveMode:     p.Mode,
		ActiveTarget:   p.Target,
		EnvVars:        envVars,
		ExportSnippet:  m.buildSnippet(envVars),
		UpdatedAt:      stored.UpdatedAt,
	}

	return status, nil
}

// findToolDef mencari definisi tool berdasarkan ID.
func (m *Manager) findToolDef(id string) (ToolDef, bool) {
	for _, t := range SupportedTools() {
		if t.ID == id {
			return t, true
		}
	}
	return ToolDef{}, false
}

// findBinary mencari executable di PATH dan direktori umum Linux/macOS.
func (m *Manager) findBinary(binaryNames []string, homeDir string) string {
	for _, name := range binaryNames {
		// 1. Cek langsung lewat PATH
		if p, err := exec.LookPath(name); err == nil {
			return p
		}

		// 2. Cek lokasi-lokasi standar non-standard PATH
		candidates := []string{
			filepath.Join(homeDir, ".local", "bin", name),
			filepath.Join(homeDir, ".cargo", "bin", name),
			filepath.Join(homeDir, "go", "bin", name),
			filepath.Join("/usr", "local", "bin", name),
			filepath.Join("/usr", "bin", name),
			filepath.Join("/snap", "bin", name),
		}

		for _, cand := range candidates {
			if fi, err := os.Stat(cand); err == nil && !fi.IsDir() && fi.Mode()&0111 != 0 {
				return cand
			}
		}

		// 3. Cek direktori Node nvm jika ada
		nvmPattern := filepath.Join(homeDir, ".nvm", "versions", "node", "*", "bin", name)
		if matches, err := filepath.Glob(nvmPattern); err == nil && len(matches) > 0 {
			return matches[len(matches)-1] // ambil versi node terbaru
		}
	}
	return ""
}

// detectVersion menjalankan perintah versi untuk biner yang terdeteksi.
func (m *Manager) detectVersion(ctx context.Context, path string, versionArg string) string {
	if path == "" || versionArg == "" {
		return ""
	}

	tctx, cancel := context.WithTimeout(ctx, 1500*time.Millisecond)
	defer cancel()

	cmd := exec.CommandContext(tctx, path, versionArg)
	var out bytes.Buffer
	cmd.Stdout = &out
	cmd.Stderr = &out

	if err := cmd.Run(); err != nil {
		return ""
	}

	lines := strings.Split(strings.TrimSpace(out.String()), "\n")
	if len(lines) > 0 {
		ver := strings.TrimSpace(lines[0])
		if len(ver) > 60 {
			ver = ver[:60] + "..."
		}
		return ver
	}
	return ""
}

// buildEnvVars menyusun map environment variables untuk tool tertentu.
func (m *Manager) buildEnvVars(tool ToolDef, mode, target, gwURL string) map[string]string {
	vars := make(map[string]string)

	if tool.EnvVarBaseURL != "" {
		vars[tool.EnvVarBaseURL] = gwURL
	}

	if tool.EnvVarModel != "" {
		vars[tool.EnvVarModel] = target
	}

	// Atur API key default jika belum diset khusus
	if tool.EnvVarAPIKey != "" {
		vars[tool.EnvVarAPIKey] = "rx_live_personal_gateway"
	}

	return vars
}

// buildSnippet menyusun teks export shell untuk terminal.
func (m *Manager) buildSnippet(vars map[string]string) string {
	var sb strings.Builder
	for k, v := range vars {
		sb.WriteString(fmt.Sprintf("export %s=\"%s\"\n", k, v))
	}
	return strings.TrimRight(sb.String(), "\n")
}

// syncGlobalEnvScript memperbarui berkas skrip lingkungan terpadu ~/.routex/cli-env.sh.
func (m *Manager) syncGlobalEnvScript(homeDir string, tool ToolDef, vars map[string]string) error {
	if homeDir == "" {
		return nil
	}

	routexDir := filepath.Join(homeDir, ".routex")
	if err := os.MkdirAll(routexDir, 0755); err != nil {
		return err
	}

	envFile := filepath.Join(routexDir, "cli-env.sh")

	// Baca konten lama jika ada
	var existing string
	if data, err := os.ReadFile(envFile); err == nil {
		existing = string(data)
	}

	header := "#!/bin/bash\n# Route-X Personal AI Gateway - CLI Environments\n# Muat skrip ini dengan: source ~/.routex/cli-env.sh\n\n"
	if !strings.HasPrefix(existing, "#!/bin/bash") {
		existing = header
	}

	markerStart := fmt.Sprintf("# --- BEGIN ROUTE-X %s ---", strings.ToUpper(tool.ID))
	markerEnd := fmt.Sprintf("# --- END ROUTE-X %s ---", strings.ToUpper(tool.ID))

	var block strings.Builder
	block.WriteString(markerStart + "\n")
	for k, v := range vars {
		block.WriteString(fmt.Sprintf("export %s=\"%s\"\n", k, v))
	}
	block.WriteString(markerEnd)

	newContent := ""
	if strings.Contains(existing, markerStart) && strings.Contains(existing, markerEnd) {
		before := existing[:strings.Index(existing, markerStart)]
		after := existing[strings.Index(existing, markerEnd)+len(markerEnd):]
		newContent = before + block.String() + after
	} else {
		newContent = existing + "\n" + block.String() + "\n"
	}

	return os.WriteFile(envFile, []byte(newContent), 0644)
}

// applyToolSpecificConfig menulis berkas konfigurasi lokal jika aplikabel.
func (m *Manager) applyToolSpecificConfig(homeDir string, tool ToolDef, mode, target, gwURL, apiKey string) error {
	if homeDir == "" {
		return nil
	}

	switch tool.ID {
	case "claude":
		// Tulis ~/.claude/settings.json jika folder ~/.claude ada
		claudeDir := filepath.Join(homeDir, ".claude")
		if fi, err := os.Stat(claudeDir); err == nil && fi.IsDir() {
			settingsPath := filepath.Join(claudeDir, "settings.json")
			// Tambahkan konfigurasi baseUrl aman
			cfg := map[string]any{
				"anthropic_base_url": gwURL,
				"model":              target,
			}
			data, err := json.MarshalIndent(cfg, "", "  ")
			if err == nil {
				_ = os.WriteFile(settingsPath, data, 0644)
			}
		}

	case "aider":
		// Tulis ~/.aider.conf.yml
		aiderConfigPath := filepath.Join(homeDir, ".aider.conf.yml")
		content := fmt.Sprintf("openai-api-base: %s\nmodel: %s\n", gwURL, target)
		_ = os.WriteFile(aiderConfigPath, []byte(content), 0644)

	case "fabric":
		// Tulis ~/.config/fabric/.env jika direktori ada
		fabricDir := filepath.Join(homeDir, ".config", "fabric")
		if fi, err := os.Stat(fabricDir); err == nil && fi.IsDir() {
			envPath := filepath.Join(fabricDir, ".env")
			content := fmt.Sprintf("OPENAI_BASE_URL=%s\nDEFAULT_MODEL=%s\n", gwURL, target)
			_ = os.WriteFile(envPath, []byte(content), 0644)
		}
	}

	return nil
}

// GetExportScript membaca seluruh skrip lingkungan gabungan.
func (m *Manager) GetExportScript(homeDir string) (string, error) {
	if homeDir == "" {
		homeDir, _ = os.UserHomeDir()
	}
	envFile := filepath.Join(homeDir, ".routex", "cli-env.sh")
	data, err := os.ReadFile(envFile)
	if err != nil {
		if os.IsNotExist(err) {
			return "# Belum ada konfigurasi CLI yang diterapkan.\nPilih CLI di atas dan klik 'Terapkan ke CLI'.", nil
		}
		return "", err
	}
	return string(data), nil
}
