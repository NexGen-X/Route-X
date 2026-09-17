package cliconfig

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"log/slog"
	"net/url"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"sync"
	"time"

	"github.com/NexGen-X/Route-X/internal/database/repo/identity"
	"github.com/NexGen-X/Route-X/internal/security"
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
	cipher   *security.Cipher
	logger   *slog.Logger
	mu       sync.RWMutex
}

// NewManager membuat instance baru Manager.
func NewManager(settings SettingsStore, cipher *security.Cipher, logger *slog.Logger) *Manager {
	return &Manager{
		settings: settings,
		cipher:   cipher,
		logger:   logger,
	}
}

// StoredConfig merepresentasikan format JSON yang disimpan di tabel settings.
//
// APIKeyEncrypted memuat ciphertext AES-256-GCM (bukan plaintext) dengan AAD
// terikat ID tool (security.CLIToolAAD). Field api_key versi lama tetap
// dipertahankan untuk migrasi bertahap: nilai legacy hanya dipakai saat
// ciphertext absen, dan ditulis ulang dalam bentuk terenkripsi pada kesempatan
// pertama — ia tidak pernah kembali ke respons API.
type StoredConfig struct {
	Mode            string    `json:"mode"`
	Target          string    `json:"target"`
	APIKey          string    `json:"api_key,omitempty"` // legacy plaintext; hanya dibaca saat migrasi
	APIKeyEncrypted string    `json:"api_key_enc,omitempty"`
	GatewayURL      string    `json:"gateway_url,omitempty"`
	UpdatedAt       time.Time `json:"updated_at"`
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
			EnvVarAPIKey:   tool.EnvVarAPIKey,
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

		// 3. Baca preferensi non-rahasia yang tersimpan di database.
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
					m.migrateLegacyAPIKey(ctx, tool.ID, setting, stored)
				}
			}
		}

		// 4. Susun Environment Variables dan Export Snippet tanpa API key.
		envVars := m.buildEnvVars(tool, status.ActiveMode, status.ActiveTarget, defaultGatewayURL)
		status.EnvVars = envVars
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
	if strings.ContainsAny(p.Target, "\x00\r\n") {
		return nil, fmt.Errorf("target model memuat karakter terlarang")
	}

	gwURL := p.GatewayURL
	if gwURL == "" {
		gwURL = defaultGatewayURL
		if gwURL == "" {
			gwURL = "http://localhost:8080/v1"
		}
	}
	parsedURL, err := url.Parse(gwURL)
	if err != nil || (parsedURL.Scheme != "http" && parsedURL.Scheme != "https") || parsedURL.Host == "" || parsedURL.User != nil || parsedURL.RawQuery != "" || parsedURL.Fragment != "" || strings.ContainsAny(gwURL, "\x00\r\n") {
		return nil, fmt.Errorf("gateway URL tidak sah atau memuat komponen terlarang")
	}

	stored := StoredConfig{
		Mode:       p.Mode,
		Target:     p.Target,
		GatewayURL: gwURL,
		UpdatedAt:  time.Now().UTC(),
	}
	if p.APIKey != "" && m.cipher != nil {
		encrypted, encErr := m.cipher.Encrypt([]byte(p.APIKey), security.CLIToolAAD(p.ToolID))
		if encErr != nil {
			return nil, fmt.Errorf("mengenkripsi API key CLI: %w", encErr)
		}
		stored.APIKeyEncrypted = encrypted
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

	// 2. Tulis berkas shell script lingkungan terpadu ~/.routex/cli-env.sh.
	// API key tidak ditulis ke disk; operator memasukkannya ke lingkungan proses
	// CLI melalui secret manager atau shell aktif.
	homeDir, _ := os.UserHomeDir()
	if p.APIKey != "" && homeDir != "" {
		routexDir := filepath.Join(homeDir, ".routex")
		_ = os.MkdirAll(routexDir, 0755)
		keyEnvPath := filepath.Join(routexDir, "key.env")
		if qKey, err := shellQuote(p.APIKey); err == nil {
			keyContent := fmt.Sprintf("export ROUTEX_API_KEY=%s\nexport ANTHROPIC_API_KEY=%s\nexport OPENAI_API_KEY=%s\nexport OPENCODE_API_KEY=%s\n",
				qKey, qKey, qKey, qKey)
			_ = os.WriteFile(keyEnvPath, []byte(keyContent), 0600)
		}
	}

	fullEnvVars := m.buildEnvVars(toolDef, p.Mode, p.Target, gwURL)

	_ = m.syncGlobalEnvScript(homeDir, toolDef, fullEnvVars)

	// 3. Tulis file konfigurasi lokal spesifik jika ada direktorinya.
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
		EnvVars:        fullEnvVars,
		EnvVarAPIKey:   toolDef.EnvVarAPIKey,
		ExportSnippet:  m.buildSnippet(fullEnvVars),
		UpdatedAt:      stored.UpdatedAt,
	}

	return status, nil
}

// migrateLegacyAPIKey mengganti field api_key plaintext dari versi lama dengan
// ciphertext pada pembacaan pertama. Nilainya tidak pernah diteruskan ke respons.
func (m *Manager) migrateLegacyAPIKey(ctx context.Context, toolID string, setting identity.Setting, stored StoredConfig) {
	if stored.APIKey == "" || stored.APIKeyEncrypted != "" || m.cipher == nil || m.settings == nil {
		return
	}
	encrypted, err := m.cipher.Encrypt([]byte(stored.APIKey), security.CLIToolAAD(toolID))
	if err != nil {
		m.logger.WarnContext(ctx, "gagal mengenkripsi API key CLI legacy", "tool", toolID, "error", err)
		return
	}
	stored.APIKey = ""
	stored.APIKeyEncrypted = encrypted
	raw, err := json.Marshal(stored)
	if err != nil {
		return
	}
	if _, err := m.settings.Put(ctx, identity.SettingWrite{
		Key:         setting.Key,
		Value:       raw,
		Description: setting.Description,
		UpdatedBy:   setting.UpdatedBy,
	}); err != nil {
		m.logger.WarnContext(ctx, "gagal memigrasikan API key CLI legacy", "tool", toolID, "error", err)
	}
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

	actualURL := gwURL
	if tool.ID == "claude" {
		actualURL = strings.TrimSuffix(gwURL, "/v1")
	}

	if tool.EnvVarBaseURL != "" {
		vars[tool.EnvVarBaseURL] = actualURL
	}

	if tool.EnvVarModel != "" {
		vars[tool.EnvVarModel] = target
	}

	return vars
}

// shellQuote mengutip nilai environment untuk shell POSIX tanpa membuka ruang
// ekspansi command, variabel, atau pemutusan perintah.
func shellQuote(value string) (string, error) {
	if strings.ContainsAny(value, "\x00\r\n") {
		return "", fmt.Errorf("nilai environment memuat karakter terlarang")
	}
	return "'" + strings.ReplaceAll(value, "'", `'"'"'`) + "'", nil
}

// buildSnippet menyusun teks export shell untuk terminal.
func (m *Manager) buildSnippet(vars map[string]string) string {
	var sb strings.Builder
	for k, v := range vars {
		quoted, err := shellQuote(v)
		if err != nil {
			continue
		}
		sb.WriteString(fmt.Sprintf("export %s=%s\n", k, quoted))
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

	header := "#!/bin/bash\n# Route-X Personal AI Gateway - CLI Environments\n# Muat skrip ini dengan: source ~/.routex/cli-env.sh\n\nif [ -f \"$HOME/.routex/key.env\" ]; then\n    . \"$HOME/.routex/key.env\"\nfi\n\n"
	if !strings.HasPrefix(existing, "#!/bin/bash") {
		existing = header
	} else if !strings.Contains(existing, "key.env") {
		existing = strings.Replace(existing, "#!/bin/bash\n", header, 1)
	}

	markerStart := fmt.Sprintf("# --- BEGIN ROUTE-X %s ---", strings.ToUpper(tool.ID))
	markerEnd := fmt.Sprintf("# --- END ROUTE-X %s ---", strings.ToUpper(tool.ID))

	var block strings.Builder
	block.WriteString(markerStart + "\n")
	for k, v := range vars {
		quoted, err := shellQuote(v)
		if err != nil {
			return err
		}
		block.WriteString(fmt.Sprintf("export %s=%s\n", k, quoted))
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

	// Hapus assignment API key legacy dari file host, lalu batasi izin file.
	newContent = m.removeAPIKeysFromScript(newContent)
	if err := os.WriteFile(envFile, []byte(newContent), 0600); err != nil {
		return err
	}

	m.ensureShellAutoLoad(homeDir)
	return nil
}

// ensureShellAutoLoad memastikan skrip login shell otomatis memuat ~/.routex/cli-env.sh.
func (m *Manager) ensureShellAutoLoad(homeDir string) {
	if homeDir == "" {
		return
	}
	hook := "\n# >>> Route-X AI Gateway CLI Auto-Loader >>>\nif [ -f \"$HOME/.routex/cli-env.sh\" ]; then\n    . \"$HOME/.routex/cli-env.sh\"\nfi\n# <<< Route-X AI Gateway CLI Auto-Loader <<<\n"

	bashrcPath := filepath.Join(homeDir, ".bashrc")
	if data, err := os.ReadFile(bashrcPath); err == nil {
		if !strings.Contains(string(data), "Route-X AI Gateway CLI Auto-Loader") {
			_ = os.WriteFile(bashrcPath, []byte(hook+"\n"+string(data)), 0644)
		}
	}

	zshrcPath := filepath.Join(homeDir, ".zshrc")
	if data, err := os.ReadFile(zshrcPath); err == nil {
		if !strings.Contains(string(data), "Route-X AI Gateway CLI Auto-Loader") {
			_ = os.WriteFile(zshrcPath, []byte(string(data)+hook), 0644)
		}
	}

	profilePath := "/etc/profile.d/routex.sh"
	if fi, err := os.Stat("/etc/profile.d"); err == nil && fi.IsDir() {
		if data, err := os.ReadFile(profilePath); err != nil || !strings.Contains(string(data), "Route-X AI Gateway CLI Auto-Loader") {
			_ = os.WriteFile(profilePath, []byte(hook), 0644)
		}
	}
}

// applyToolSpecificConfig menulis berkas konfigurasi lokal jika aplikabel.
func (m *Manager) applyToolSpecificConfig(homeDir string, tool ToolDef, mode, target, gwURL, apiKey string) error {
	if homeDir == "" {
		return nil
	}

	switch tool.ID {
	case "agy":
		// CRITICAL: Jangan pernah mengubah atau menyentuh konfigurasi Antigravity CLI.
		return nil

	case "claude":
		// Tulis ~/.claude/settings.json
		claudeDir := filepath.Join(homeDir, ".claude")
		_ = os.MkdirAll(claudeDir, 0755)
		settingsPath := filepath.Join(claudeDir, "settings.json")
		claudeURL := strings.TrimSuffix(gwURL, "/v1")
		cfg := map[string]any{
			"anthropic_base_url": claudeURL,
			"model":              target,
		}
		data, err := json.MarshalIndent(cfg, "", "  ")
		if err == nil {
			_ = os.WriteFile(settingsPath, data, 0644)
		}

	case "aider":
		// Tulis ~/.aider.conf.yml
		aiderConfigPath := filepath.Join(homeDir, ".aider.conf.yml")
		content := fmt.Sprintf("openai-api-base: %s\nmodel: %s\n", gwURL, target)
		_ = os.WriteFile(aiderConfigPath, []byte(content), 0644)

	case "fabric":
		// Tulis ~/.config/fabric/.env
		fabricDir := filepath.Join(homeDir, ".config", "fabric")
		_ = os.MkdirAll(fabricDir, 0755)
		envPath := filepath.Join(fabricDir, ".env")
		content := fmt.Sprintf("OPENAI_BASE_URL=%s\nDEFAULT_MODEL=%s\n", gwURL, target)
		_ = os.WriteFile(envPath, []byte(content), 0644)

	case "sgpt":
		// Tulis ~/.config/shell_gpt/.sgptrc
		sgptDir := filepath.Join(homeDir, ".config", "shell_gpt")
		_ = os.MkdirAll(sgptDir, 0755)
		sgptPath := filepath.Join(sgptDir, ".sgptrc")
		content := fmt.Sprintf("OPENAI_BASE_URL=%s\nDEFAULT_MODEL=%s\n", gwURL, target)
		_ = os.WriteFile(sgptPath, []byte(content), 0644)

	case "opencommit":
		// Tulis ~/.opencommit
		ocoPath := filepath.Join(homeDir, ".opencommit")
		content := fmt.Sprintf("OCO_OPENAI_BASE_PATH=%s\nOCO_MODEL=%s\n", gwURL, target)
		_ = os.WriteFile(ocoPath, []byte(content), 0644)

	case "opencode":
		// Tulis atau perbarui ~/.config/opencode/opencode.json
		opencodeDir := filepath.Join(homeDir, ".config", "opencode")
		_ = os.MkdirAll(opencodeDir, 0755)
		configPath := filepath.Join(opencodeDir, "opencode.json")
		var cfg map[string]any
		if data, err := os.ReadFile(configPath); err == nil {
			_ = json.Unmarshal(data, &cfg)
		}
		if cfg == nil {
			cfg = make(map[string]any)
		}
		cfg["$schema"] = "https://opencode.ai/config.json"
		providers, ok := cfg["provider"].(map[string]any)
		if !ok || providers == nil {
			providers = make(map[string]any)
			cfg["provider"] = providers
		}
		routexProv, ok := providers["routex"].(map[string]any)
		if !ok || routexProv == nil {
			routexProv = map[string]any{
				"npm": "@ai-sdk/openai-compatible",
				"options": map[string]any{
					"baseURL": gwURL,
				},
				"models": map[string]any{},
			}
			providers["routex"] = routexProv
		}
		if opts, ok := routexProv["options"].(map[string]any); ok {
			opts["baseURL"] = gwURL
			if apiKey != "" {
				opts["apiKey"] = apiKey
			}
		}
		models, ok := routexProv["models"].(map[string]any)
		if !ok || models == nil {
			models = make(map[string]any)
			routexProv["models"] = models
		}
		models[target] = map[string]any{"name": target}
		cfg["model"] = "routex/" + target
		if data, err := json.MarshalIndent(cfg, "", "  "); err == nil {
			_ = os.WriteFile(configPath, data, 0644)
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
	// Redaksi tetap dipertahankan untuk membersihkan file legacy yang mungkin
	// dibuat versi lama sebelum isinya dikirim melalui API.
	return m.redactAPIKeysInScript(string(data)), nil
}

func (m *Manager) removeAPIKeysFromScript(content string) string {
	keyVars := map[string]bool{}
	for _, t := range SupportedTools() {
		if t.EnvVarAPIKey != "" {
			keyVars[t.EnvVarAPIKey] = true
		}
	}
	lines := strings.Split(content, "\n")
	kept := lines[:0]
	for _, line := range lines {
		trimmed := strings.TrimSpace(line)
		rest := strings.TrimPrefix(trimmed, "export ")
		eq := strings.Index(rest, "=")
		if eq > 0 && keyVars[strings.TrimSpace(rest[:eq])] {
			continue
		}
		kept = append(kept, line)
	}
	return strings.Join(kept, "\n")
}

// redactAPIKeysInScript mengganti nilai variabel API key yang dikenali dalam
// skrip export dengan tampilan tersamar.
func (m *Manager) redactAPIKeysInScript(content string) string {
	keyVars := map[string]bool{}
	for _, t := range SupportedTools() {
		if t.EnvVarAPIKey != "" {
			keyVars[t.EnvVarAPIKey] = true
		}
	}
	lines := strings.Split(content, "\n")
	for i, line := range lines {
		trimmed := strings.TrimSpace(line)
		if !strings.HasPrefix(trimmed, "export ") {
			continue
		}
		rest := strings.TrimPrefix(trimmed, "export ")
		eq := strings.Index(rest, "=")
		if eq <= 0 {
			continue
		}
		varName := strings.TrimSpace(rest[:eq])
		if !keyVars[varName] {
			continue
		}
		val := strings.TrimSpace(rest[eq+1:])
		val = strings.Trim(val, "\"")
		if val == "" {
			continue
		}
		prefixPart := ""
		if idx := strings.Index(line, "export "); idx >= 0 {
			prefixPart = line[:idx]
		}
		lines[i] = fmt.Sprintf("%sexport %s=\"%s\"", prefixPart, varName, security.MaskAPIKey(val))
	}
	return strings.Join(lines, "\n")
}
