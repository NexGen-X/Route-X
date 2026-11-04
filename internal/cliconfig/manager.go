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
	"sort"
	"strconv"
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

	// cacheVersi menahan hasil detectVersion per path biner (audit3
	// V-LN-37): Scan mengeksekusi `--version` SETIAP biner pada SETIAP
	// panggilan, jadi membuka halaman CLI berarti ±20 proses anak. Versi
	// biner tidak berubah di tengah jalan; TTL beberapa menit cukup.
	cacheVersiMu sync.Mutex
	cacheVersi   map[string]entriVersi
}

// entriVersi adalah satu hasil deteksi versi yang ditahan.
type entriVersi struct {
	versi string
	pada  time.Time
}

// ttlCacheVersi adalah umur hasil deteksi versi biner yang ditahan.
const ttlCacheVersi = 5 * time.Minute

// namaVarKunciGenerik adalah nama variabel generik yang diisi key.env dari
// kunci tool yang paling baru dikonfigurasi, supaya tool yang membaca nama
// generik tetap mendapat kunci Route-X yang sah.
var namaVarKunciGenerik = []string{
	"ROUTEX_API_KEY",
	"ANTHROPIC_API_KEY",
	"OPENAI_API_KEY",
	"OPENCODE_API_KEY",
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
// tulisAtomik menulis berkas secara atomik: tulis ke berkas sementara di
// direktori yang sama, sesuaikan izin, lalu rename (audit F5). Penulisan
// langsung (os.WriteFile) bisa meninggalkan berkas setengah jadi bila
// proses terputus di tengah — dan berkas-berkas ini (~/.bashrc,
// settings.json milik tool CLI, key.env berisi API key) dibaca proses
// lain apa adanya.
func tulisAtomik(path string, data []byte, perm os.FileMode) error {
	tmp, err := os.CreateTemp(filepath.Dir(path), ".routex-tulis-*")
	if err != nil {
		return err
	}
	tmpName := tmp.Name()
	defer os.Remove(tmpName)
	if _, err := tmp.Write(data); err != nil {
		_ = tmp.Close()
		return err
	}
	if err := tmp.Chmod(perm); err != nil {
		_ = tmp.Close()
		return err
	}
	if err := tmp.Close(); err != nil {
		return err
	}
	return os.Rename(tmpName, path)
}

func (m *Manager) Scan(ctx context.Context, defaultGatewayURL string) ([]ToolStatus, error) {
	m.mu.RLock()
	defer m.mu.RUnlock()

	tools := SupportedTools()
	results := make([]ToolStatus, 0, len(tools))
	homeDir, _ := os.UserHomeDir()

	if defaultGatewayURL == "" {
		defaultGatewayURL = "http://localhost:8080/v1"
	}

	// Deteksi path + versi dihitung lebih dulu, dan eksekusi perintah
	// versi berjalan PARALEL terbatas (B-148): batas per-tool kini
	// 5 detik (timeoutDeteksiVersi) untuk menampung startup tool
	// Python nyata, jadi jalur sekuensial akan membuat waktu Scan
	// terburuk melonjak dari ±20×1,5 detik menjadi ±20×5 detik.
	// Dengan maksimal 8 proses versi serentak, waktu deteksi
	// terburuk Scan tertahan di ±3 gelombang × 5 detik — di bawah
	// waktu terburuk lama — sementara cache versi 5 menit membuat
	// biaya ini hanya terbayar pada pindaian pertama. Setiap
	// goroutine menulis indeks versinya sendiri; cache versi
	// sendiri bergembok (cacheVersiMu).
	execPaths := make([]string, len(tools))
	for i, tool := range tools {
		execPaths[i] = m.findBinary(tool.BinaryNames, homeDir)
	}
	versiTools := make([]string, len(tools))
	{
		var wg sync.WaitGroup
		sem := make(chan struct{}, 8)
		for i, tool := range tools {
			if execPaths[i] == "" || tool.VersionArg == "" {
				continue
			}
			wg.Add(1)
			go func(i int, tool ToolDef, path string) {
				defer wg.Done()
				sem <- struct{}{}
				defer func() { <-sem }()
				versiTools[i] = m.detectVersionUntukTool(ctx, tool, path)
			}(i, tool, execPaths[i])
		}
		wg.Wait()
	}

	for i, tool := range tools {
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
			// Kejujuran katalog (B-130/B-131/B-134/B-135/B-136): tool
			// yang tidak dapat diarahkan ke gateway ditandai apa
			// adanya sejak pindaian, bukan sesudah Configure "sukses".
			GatewaySupported:  tool.UnsupportedReason == "",
			UnsupportedReason: tool.UnsupportedReason,
		}

		// 1. Deteksi executable path (path + versi sudah dihitung
		// paralel di atas, sebelum putaran ini).
		execPath := execPaths[i]
		if execPath != "" {
			status.Installed = true
			status.Path = execPath
			status.Version = versiTools[i]
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

	// Kejujuran katalog: tool yang secara arsitektur tidak dapat
	// diarahkan ke gateway DITOLAK di sini — sebelum penulisan berkas,
	// key.env, atau database apa pun — dengan alasannya. Versi lama
	// melaporkan "berhasil diterapkan" untuk integrasi yang terbukti
	// tidak ada (B-130, B-131, B-134, B-135, B-136).
	if toolDef.UnsupportedReason != "" {
		return nil, fmt.Errorf("%s tidak dapat dikonfigurasi ke gateway: %s", toolDef.Name, toolDef.UnsupportedReason)
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

	settingKey := fmt.Sprintf("cli:config:%s", p.ToolID)

	// Pertahankan kunci tersimpan tool ini bila Configure tidak membawa
	// kunci baru: Put menimpa SELURUH StoredConfig, jadi tanpa langkah ini
	// konfigurasi ulang tanpa kunci diam-diam menghapus kunci tersimpan
	// tool itu sendiri (bagian dari audit3 V-LN-26). Tidak ada alur
	// "hapus kunci" yang disengaja — field kosong berarti "tidak diubah".
	if p.APIKey == "" && m.settings != nil {
		if lama, errGet := m.settings.Get(ctx, settingKey); errGet == nil && len(lama.Value) > 0 {
			var storedLama StoredConfig
			if json.Unmarshal(lama.Value, &storedLama) == nil {
				stored.APIKeyEncrypted = storedLama.APIKeyEncrypted
			}
		}
	}

	rawJSON, err := json.Marshal(stored)
	if err != nil {
		return nil, fmt.Errorf("mengkodekan konfigurasi CLI: %w", err)
	}

	homeDir, _ := os.UserHomeDir()
	fullEnvVars := m.buildEnvVars(toolDef, p.Mode, p.Target, gwURL)

	// Urutan penulisan (audit3 V-LN-35): BERKAS dulu, database TERAKHIR,
	// dan key.env di-snapshot sebelum ditulis ulang. Versi lama menyimpan
	// ke DB lebih dulu — kegagalan langkah berkas sesudahnya meninggalkan
	// DB yang mengklaim konfigurasi yang tidak pernah diterapkan di disk.
	var keyEnvPath string
	var snapshotKeyEnv []byte
	keyEnvAda := false
	if homeDir != "" {
		routexDir := filepath.Join(homeDir, ".routex")
		if err := os.MkdirAll(routexDir, 0755); err != nil {
			return nil, fmt.Errorf("menyiapkan direktori %s: %w", routexDir, err)
		}
		keyEnvPath = filepath.Join(routexDir, "key.env")
		if data, errBaca := os.ReadFile(keyEnvPath); errBaca == nil {
			snapshotKeyEnv, keyEnvAda = data, true
		}
		// key.env DIBANGUN ULANG dari seluruh konfigurasi tersimpan, bukan
		// ditimpa empat nama generik milik tool terakhir saja (audit3
		// V-LN-24/26/27): setiap tool berkonfigurasi mendapat baris
		// variabelnya sendiri, dan tool lain tidak lagi hilang dari berkas
		// hanya karena tool lain dikonfigurasi sesudahnya.
		if err := m.tulisUlangKeyEnv(ctx, keyEnvPath, p.ToolID, p.APIKey, stored); err != nil {
			return nil, err
		}
	}
	pulihkanKeyEnv := func() {
		if keyEnvPath == "" {
			return
		}
		if keyEnvAda {
			_ = tulisAtomik(keyEnvPath, snapshotKeyEnv, 0600)
		} else {
			_ = os.Remove(keyEnvPath)
		}
	}

	if err := m.syncGlobalEnvScript(homeDir, toolDef, fullEnvVars); err != nil {
		pulihkanKeyEnv()
		return nil, fmt.Errorf("menyinkronkan skrip env global CLI: %w", err)
	}

	// Tulis file konfigurasi lokal spesifik jika ada direktorinya.
	if err := m.applyToolSpecificConfig(homeDir, toolDef, p.Mode, p.Target, gwURL, p.APIKey); err != nil {
		pulihkanKeyEnv()
		return nil, fmt.Errorf("menulis konfigurasi lokal %s: %w", toolDef.Name, err)
	}

	// Simpan ke tabel settings database TERAKHIR (lihat urutan di atas).
	if m.settings != nil {
		_, err = m.settings.Put(ctx, identity.SettingWrite{
			Key:         settingKey,
			Value:       rawJSON,
			Description: fmt.Sprintf("Konfigurasi integrasi CLI untuk %s", toolDef.Name),
		})
		if err != nil {
			// Kegagalan persistensi = Configure GAGAL (audit F5): versi lama
			// hanya mencatat log lalu melaporkan sukses, sehingga operator
			// mengira konfigurasi tersimpan padahal hilang saat restart.
			return nil, fmt.Errorf("menyimpan konfigurasi CLI %s ke database: %w", p.ToolID, err)
		}
	}

	execPath := m.findBinary(toolDef.BinaryNames, homeDir)
	status := &ToolStatus{
		ID:             toolDef.ID,
		Name:           toolDef.Name,
		Description:    toolDef.Description,
		Category:       toolDef.Category,
		Installed:      execPath != "",
		Path:           execPath,
		Version:        m.detectVersionUntukTool(ctx, toolDef, execPath),
		SupportedModes: []string{ModeModelOnly, ModeRouting, ModeCombo},
		ActiveMode:     p.Mode,
		ActiveTarget:   p.Target,
		EnvVars:        fullEnvVars,
		EnvVarAPIKey:   toolDef.EnvVarAPIKey,
		ExportSnippet:  m.buildSnippet(fullEnvVars),
		UpdatedAt:      stored.UpdatedAt,
		// Configure hanya mencapai titik ini untuk tool yang didukung
		// gateway (penolakan untuk yang tidak didukung ada di atas).
		GatewaySupported: true,
	}

	return status, nil
}

// kunciTersimpan adalah API key plaintext satu tool beserta waktu
// konfigurasi terakhirnya, hasil dekripsi StoredConfig di database.
type kunciTersimpan struct {
	kunci      string
	diperbarui time.Time
}

// semuaKunciTersimpan membaca API key plaintext SELURUH tool yang
// terkonfigurasi dari tabel settings. Tool tanpa kunci (atau yang gagal
// didekripsi) tidak masuk hasil — kegagalan dekripsi dicatat, tidak
// menggagalkan pembangunan ulang key.env untuk tool lain.
func (m *Manager) semuaKunciTersimpan(ctx context.Context) map[string]kunciTersimpan {
	hasil := make(map[string]kunciTersimpan)
	if m.settings == nil {
		return hasil
	}
	semua, err := m.settings.All(ctx)
	if err != nil {
		if m.logger != nil {
			m.logger.WarnContext(ctx, "gagal membaca seluruh konfigurasi CLI untuk key.env", "error", err)
		}
		return hasil
	}
	for _, s := range semua {
		toolID, ok := strings.CutPrefix(s.Key, "cli:config:")
		if !ok || len(s.Value) == 0 {
			continue
		}
		var stored StoredConfig
		if json.Unmarshal(s.Value, &stored) != nil {
			continue
		}
		kunci := stored.APIKey // legacy plaintext, bila ciphertext absen
		if stored.APIKeyEncrypted != "" {
			kunci = ""
			if m.cipher != nil {
				if dec, errDec := m.cipher.Decrypt(stored.APIKeyEncrypted, security.CLIToolAAD(toolID)); errDec == nil {
					kunci = string(dec)
				} else if m.logger != nil {
					m.logger.WarnContext(ctx, "gagal mendekripsi API key CLI untuk key.env",
						"tool", toolID, "error", errDec)
				}
			}
		}
		if kunci != "" {
			hasil[toolID] = kunciTersimpan{kunci: kunci, diperbarui: stored.UpdatedAt}
		}
	}
	return hasil
}

// tulisUlangKeyEnv membangun ulang ~/.routex/key.env dari seluruh
// konfigurasi tersimpan, dengan konfigurasi tool yang sedang diterapkan
// sebagai lapisan teratas (ia belum ada di database — Put terjadi terakhir).
func (m *Manager) tulisUlangKeyEnv(ctx context.Context, keyEnvPath, toolID, apiKey string, stored StoredConfig) error {
	perTool := m.semuaKunciTersimpan(ctx)

	kunciKini := apiKey
	if kunciKini == "" && stored.APIKeyEncrypted != "" && m.cipher != nil {
		if dec, err := m.cipher.Decrypt(stored.APIKeyEncrypted, security.CLIToolAAD(toolID)); err == nil {
			kunciKini = string(dec)
		}
	}
	if kunciKini != "" {
		perTool[toolID] = kunciTersimpan{kunci: kunciKini, diperbarui: stored.UpdatedAt}
	} else {
		delete(perTool, toolID)
	}

	konten, err := susunKontenKeyEnv(perTool)
	if err != nil {
		return err
	}
	if err := tulisAtomik(keyEnvPath, []byte(konten), 0600); err != nil {
		return fmt.Errorf("menulis %s: %w", keyEnvPath, err)
	}
	return nil
}

// susunKontenKeyEnv merakit isi key.env: satu baris `export <VAR>=<kunci>`
// per variabel API key tool yang terkonfigurasi, ditambah empat nama
// generik yang diisi kunci tool yang PALING BARU dikonfigurasi.
//
// Versi lama berkas ini hanya memuat empat nama generik dari tool terakhir
// yang membawa kunci: variabel khas 10 dari 20 tool (OLLAMA_API_KEY,
// OPENAI_API_KEY khas goose, SRC_ACCESS_TOKEN, dst.) tidak pernah terisi sama sekali
// (audit3 V-LN-24), dan mengonfigurasi tool B menghapus jejak tool A.
func susunKontenKeyEnv(perTool map[string]kunciTersimpan) (string, error) {
	type entriVar struct {
		nilai string
		pada  time.Time
	}
	vars := make(map[string]entriVar)
	set := func(nama, nilai string, pada time.Time) {
		if nama == "" || nilai == "" {
			return
		}
		if lama, ok := vars[nama]; !ok || !pada.Before(lama.pada) {
			vars[nama] = entriVar{nilai: nilai, pada: pada}
		}
	}

	var terbaru *kunciTersimpan
	for toolID, kt := range perTool {
		kt := kt
		// Tool yang tidak didukung gateway tidak menyumbang variabel
		// kunci apa pun — termasuk kandidat nama generik: konfigurasi
		// warisannya (dari versi sebelum penolakan Configure) tidak
		// boleh terus menyebarkan kunci gateway atas nama tool yang
		// tidak pernah benar-benar memakainya.
		def, dikenal := cariToolDef(toolID)
		if !dikenal || def.UnsupportedReason != "" {
			continue
		}
		if terbaru == nil || kt.diperbarui.After(terbaru.diperbarui) {
			salin := kt
			terbaru = &salin
		}
		if def, ok := cariToolDef(toolID); ok {
			set(def.EnvVarAPIKey, kt.kunci, kt.diperbarui)
		}
	}
	if terbaru != nil {
		for _, nama := range namaVarKunciGenerik {
			set(nama, terbaru.kunci, terbaru.diperbarui)
		}
	}

	namaVars := make([]string, 0, len(vars))
	for nama := range vars {
		namaVars = append(namaVars, nama)
	}
	sort.Strings(namaVars)

	var b strings.Builder
	b.WriteString("# Route-X — API key per tool CLI (dibangun ulang otomatis; jangan edit manual)\n")
	// Keputusan desain B-117 (terdokumentasi di backlog): nama generik
	// di bawah (ROUTEX_API_KEY/ANTHROPIC_API_KEY/OPENAI_API_KEY/
	// OPENCODE_API_KEY) SENGAJA dipertahankan karena Claude Code dan
	// OpenCode terbukti bergantung padanya. Konsekuensinya dinyatakan
	// apa adanya di sini: tool LAIN yang tidak dikonfigurasi Route-X
	// tetapi berjalan di shell yang memuat berkas ini dapat membaca
	// nama generik itu. Tool yang dikonfigurasi Route-X kini terikat
	// config/nama khasnya masing-masing (B-101, B-122…B-129).
	b.WriteString("# Catatan B-117: nama generik (OPENAI_API_KEY/ANTHROPIC_API_KEY/dsb.) dibaca tool apa pun\n# di shell yang memuat berkas ini; tool terkonfigurasi Route-X memakai config khasnya sendiri.\n")
	for _, nama := range namaVars {
		q, err := shellQuote(vars[nama].nilai)
		if err != nil {
			return "", fmt.Errorf("mengutip API key untuk berkas env: %w", err)
		}
		fmt.Fprintf(&b, "export %s=%s\n", nama, q)
	}
	return b.String(), nil
}

// cariToolDef adalah pembungkus paket atas daftar tool yang didukung.
func cariToolDef(id string) (ToolDef, bool) {
	for _, t := range SupportedTools() {
		if t.ID == id {
			return t, true
		}
	}
	return ToolDef{}, false
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

		// 3. Cek direktori Node nvm jika ada. Yang dipilih adalah versi
		// Node TERTINGGI secara semver, bukan hasil glob terakhir secara
		// leksikografis — urutan leksikografis menempatkan "v9.x" di atas
		// "v22.x" (audit3 V-LN-36).
		nvmPattern := filepath.Join(homeDir, ".nvm", "versions", "node", "*", "bin", name)
		if matches, err := filepath.Glob(nvmPattern); err == nil && len(matches) > 0 {
			terbaik := matches[0]
			for _, cocok := range matches[1:] {
				if bandingVersiNode(cocok, terbaik) > 0 {
					terbaik = cocok
				}
			}
			return terbaik
		}
	}
	return ""
}

// versiNodeDariPath mengekstrak komponen versi Node ("v22.11.0") dari path
// biner nvm. Kosong bila tidak ditemukan.
func versiNodeDariPath(path string) string {
	for _, bagian := range strings.Split(filepath.ToSlash(path), "/") {
		if len(bagian) > 1 && bagian[0] == 'v' && bagian[1] >= '0' && bagian[1] <= '9' {
			return bagian
		}
	}
	return ""
}

// bandingVersiNode membandingkan dua path biner nvm berdasarkan versi Node
// di path-nya secara numerik per komponen (mayor, minor, patch). Mengembalikan
// >0 bila a lebih baru. Versi yang tak terurai diperlakukan paling tua dan
// seri di antara keduanya diputus secara leksikografis agar deterministik.
func bandingVersiNode(a, b string) int {
	urai := func(v string) [3]int {
		var out [3]int
		for i, s := range strings.Split(strings.TrimPrefix(v, "v"), ".") {
			if i >= 3 {
				break
			}
			n, _ := strconv.Atoi(s)
			out[i] = n
		}
		return out
	}
	va, vb := urai(versiNodeDariPath(a)), urai(versiNodeDariPath(b))
	for i := 0; i < 3; i++ {
		if va[i] != vb[i] {
			return va[i] - vb[i]
		}
	}
	return strings.Compare(a, b)
}

// detectVersionUntukTool menjalankan deteksi versi untuk satu tool.
//
// Tool tanpa VersionArg tidak dieksekusi sama sekali: versinya
// kosong secara jujur. Tool ber-VersionNeedsStoredKey (B-145
// sgpt) dijalankan dengan kunci tersimpan tool itu di env proses anak
// bila konfigurasi tersimpan punya kunci — tanpa itu perintah versinya
// gugur saat import dan versi tool yang sehat selalu tampil kosong.
// Kunci hanya hidup di env proses anak perintah versi; ia tidak
// ditulis ke log, tidak masuk cache (kunci cache hanya mencatat ADA /
// TIDAK adanya kunci), dan tidak kembali ke respons — yang diambil
// dari keluaran hanyalah baris versi lewat ekstrakVersi. Tool lain
// tidak berubah: env proses gateway apa adanya, seperti sebelumnya.
func (m *Manager) detectVersionUntukTool(ctx context.Context, tool ToolDef, path string) string {
	if path == "" || tool.VersionArg == "" {
		return ""
	}
	var env []string
	denganKunci := false
	if tool.VersionNeedsStoredKey && tool.EnvVarAPIKey != "" {
		if kt, ok := m.semuaKunciTersimpan(ctx)[tool.ID]; ok && kt.kunci != "" {
			env = append(os.Environ(), tool.EnvVarAPIKey+"="+kt.kunci)
			denganKunci = true
		}
	}
	return m.detectVersion(ctx, path, tool.VersionArg, env, denganKunci)
}

// detectVersion menjalankan perintah versi untuk biner yang terdeteksi.
// Hasilnya (termasuk kegagalan) ditahan ttlCacheVersi per path+argumen
// supaya Scan tidak mengeksekusi ulang ±20 biner pada setiap panggilan
// (audit3 V-LN-37). env nil berarti warisi env proses gateway; kunci
// cache membedakan lari dengan / tanpa kunci tersimpan (B-145) supaya
// hasil kosong dari lari tanpa kunci tidak menutupi lari berkunci
// sesudah tool dikonfigurasi — nilai kuncinya sendiri tidak pernah
// menjadi bagian kunci cache.
func (m *Manager) detectVersion(ctx context.Context, path string, versionArg string, env []string, denganKunci bool) string {
	if path == "" || versionArg == "" {
		return ""
	}

	kunciCache := path + "\x00" + versionArg + "\x00" + strconv.FormatBool(denganKunci)
	m.cacheVersiMu.Lock()
	if m.cacheVersi != nil {
		if e, ok := m.cacheVersi[kunciCache]; ok && time.Since(e.pada) < ttlCacheVersi {
			m.cacheVersiMu.Unlock()
			return e.versi
		}
	}
	m.cacheVersiMu.Unlock()
	versi := m.detectVersionLangsung(ctx, path, versionArg, env)
	m.cacheVersiMu.Lock()
	if m.cacheVersi == nil {
		m.cacheVersi = make(map[string]entriVersi)
	}
	m.cacheVersi[kunciCache] = entriVersi{versi: versi, pada: time.Now()}
	m.cacheVersiMu.Unlock()
	return versi
}

// timeoutDeteksiVersi adalah batas waktu SATU perintah versi tool
// (B-148). Batas lama 1500 ms terbukti lebih kecil dari waktu start
// nyata tool Python yang sah: `sgpt --version` berkunci terukur
// konsisten 1,66–1,68 detik pada Uji Live Putaran 2 (startup Python +
// impor shell-gpt), sehingga versi sgpt selalu tampil kosong walau
// mekanisme kunci tersimpan B-145 bekerja benar. 5 detik menampung
// startup itu dengan ruang ±3×, dan tetap merupakan batas keras per
// tool yang berlaku SAMA untuk semua tool (bukan pengkhususan sgpt):
// biner yang menggantung tetap terpotong pada batas ini. Biaya total
// tertahan oleh cache versi 5 menit dan deteksi paralel terbatas di
// Scan (lihat komentar di Scan); jalur Configure hanya mendeteksi
// satu tool, jadi paling lama tertahan batas ini sekali.
const timeoutDeteksiVersi = 5 * time.Second

// detectVersionLangsung adalah eksekusi versi yang sebenarnya, tanpa cache.
func (m *Manager) detectVersionLangsung(ctx context.Context, path string, versionArg string, env []string) string {

	tctx, cancel := context.WithTimeout(ctx, timeoutDeteksiVersi)
	defer cancel()

	cmd := exec.CommandContext(tctx, path, versionArg)
	if env != nil {
		cmd.Env = env
	}
	var out bytes.Buffer
	cmd.Stdout = &out
	cmd.Stderr = &out

	if err := cmd.Run(); err != nil {
		return ""
	}

	return ekstrakVersi(out.String())
}

// ekstrakVersi memilih baris versi dari keluaran perintah versi tool.
//
// B-121: versi lama mengambil baris pertama apa adanya, sehingga tool
// yang mencetak baris pembuka non-versi (pada uji live, Continue
// menampilkan harfiah "None") melaporkan omong kosong itu sebagai
// versinya. Keluaran nyata `cn --version` adalah semver polos
// ("1.5.47"), dan tool lain mencetak bentuk seperti "llm, version
// 0.37.1" atau "k8sgpt: 0.4.41 (3644ecf)" — semuanya baris yang memuat
// angka. Aturannya: baris pertama yang memuat angka menang; baris
// placeholder yang diketahui bukan versi ("none", "null", "unknown",
// "undefined") tidak pernah dilaporkan sebagai versi.
func ekstrakVersi(keluaran string) string {
	for _, baris := range strings.Split(keluaran, "\n") {
		ver := strings.TrimSpace(baris)
		if ver == "" {
			continue
		}
		switch strings.ToLower(ver) {
		case "none", "null", "unknown", "undefined", "n/a":
			continue
		}
		if !strings.ContainsAny(ver, "0123456789") {
			continue
		}
		if len(ver) > 60 {
			ver = ver[:60] + "..."
		}
		return ver
	}
	return ""
}

// modelAider mengembalikan nama model dalam bentuk yang dituntut Aider:
// prefiks provider LiteLLM `openai/` (B-098). Tanpa prefiks itu Aider
// 0.86 gagal sebelum request apa pun ("LLM Provider NOT provided") untuk
// semua model OpenAI-compatible; run diagnostik Langkah 6 membuktikan
// `openai/<model>` bekerja. Target yang sudah berprefiks dibiarkan.
func modelAider(target string) string {
	if target == "" || strings.HasPrefix(target, "openai/") {
		return target
	}
	return "openai/" + target
}

// Batas token OpenCommit yang ditulis Configure (B-144, diperbaiki
// B-147).
//
// Nama variabelnya adalah skema v3 yang benar-benar dibaca OpenCommit
// 3.x (dokumentasi resmi tool): OCO_TOKENS_MAX_INPUT = anggaran token
// input per request (bawaan 4096), OCO_TOKENS_MAX_OUTPUT = "max
// response tokens" (bawaan 500) dan OCO_REASONING_MAX_TOKENS = "max
// completion token budget for reasoning models, including hidden
// reasoning tokens" (bawaan 1000). Ketiganya ditulis, bukan hanya
// plafon keluaran: respons Putaran 1 terpotong tepat pada 500 (plafon
// keluaran yang efektif), sementara plafon reasoning bawaan 1000 akan
// menjadi batas efektif berikutnya bagi model reasoning bila hanya
// keluaran yang dinaikkan — menyamakan keduanya menghilangkan
// ambiguitas jalur mana yang menang.
//
// Nilai keluaran/reasoning 4000 dipilih dari bukti, bukan karangan:
// pada Uji Live Putaran 1 reasoning intern-s2 menghabiskan SELURUH
// plafon 500 untuk satu pesan commit sederhana (input 508 token,
// reasoning 500, isi 0 → EMPTY_MESSAGE), padahal isi pesan commit itu
// sendiri pendek (judul konvensional + deskripsi singkat, orde
// puluhan–ratusan token). 4000 adalah 8× plafon yang terbukti gagal —
// ruang reasoning beberapa kali lipat dari konsumsi teramati untuk
// tugas commit, plus isi pesannya — dan tetap plafon (model berhenti
// saat selesai, bukan target), bukan angka tanpa batas.
//
// B-147 (REGRESI B-144, terbukti Uji Live Putaran 2): menulis
// keluaran/reasoning 4000 TANPA menaikkan input membuat oco gagal
// LOKAL pada setiap commit. Rumus tool 3.3.11
// (generateCommitMessageByDiff): anggaran request =
// OCO_TOKENS_MAX_INPUT − 20 − panjang prompt awal − maxOutputTokens;
// dengan input bawaan 4096 hasilnya NEGATIF untuk prompt awal mana
// pun, sehingga splitDiff melempar "Token limit exceeded…" dan UI
// tool menampilkannya sebagai "Insufficient Credits" yang menyesatkan
// (tak satu pun request mencapai gateway). Karena itu Configure kini
// juga menulis OCO_TOKENS_MAX_INPUT=16384: (1) kontrafaktualnya
// terverifikasi live pada Putaran 2 — dengan 16384 oco benar-benar
// mencapai gateway; (2) hitungannya koheren menurut rumus tool:
// 16384 − 20 − prompt awal (orde ratusan token; request teramati
// 508 token) − plafon 4000 menyisakan ±12 ribu token ruang diff per
// request, dan diff yang lebih besar memang dipecah tool per
// anggaran ini. Keluaran/reasoning TETAP 4000: dasar B-144 tidak
// berubah, dan catatan kecepatan reasoning intern-s2 pada prompt oco
// adalah karakteristik upstream, bukan alasan menurunkan plafon.
// Ketiga nilai ini konfigurasi produk: operator dapat mengubahnya
// sesudah Configure lewat `oco config set` / menyunting ~/.opencommit.
const (
	ocoTokensMaxInput     = "16384"
	ocoTokensMaxOutput    = "4000"
	ocoReasoningMaxTokens = "4000"
)

// buildEnvVars menyusun map environment variables untuk tool tertentu.
func (m *Manager) buildEnvVars(tool ToolDef, mode, target, gwURL string) map[string]string {
	vars := make(map[string]string)

	// Kejujuran katalog: tool yang tidak dapat diarahkan ke gateway
	// tidak mendapat ekspor env apa pun — ekspor warisannya terbukti
	// tidak dibaca tool dan hanya mengesankan dukungan yang tidak ada.
	if tool.UnsupportedReason != "" {
		return vars
	}

	// B-101: Continue (cn) tidak membaca variabel env khusus apa pun —
	// konfigurasi sahnya hanya berkas ~/.continue/config.yaml yang
	// ditulis applyToolSpecificConfig. Mengekspor nama rekaan lagi
	// hanya akan menyesatkan dan membiarkan cn jatuh ke config default
	// dari env generik.
	if tool.ID == "continue" {
		return vars
	}

	actualURL := gwURL
	model := target
	switch tool.ID {
	case "claude":
		// Claude Code memakai permukaan Anthropic gateway: base URL
		// tanpa sufiks /v1.
		actualURL = strings.TrimSuffix(gwURL, "/v1")
	case "goose":
		// B-099: OPENAI_HOST yang dibaca Goose adalah host tanpa sufiks
		// /v1 (Goose menambahkan v1/chat/completions sendiri), dan
		// provider-nya harus dinyatakan eksplisit.
		actualURL = strings.TrimSuffix(gwURL, "/v1")
		vars["GOOSE_PROVIDER"] = "openai"
	case "aider":
		model = modelAider(target)
	case "fabric":
		// B-123: tanpa DEFAULT_VENDOR fabric menolak berjalan
		// ("please run 'fabric --setup'"); vendor OpenAI adalah jalur
		// gateway OpenAI-compatible Route-X.
		vars["DEFAULT_VENDOR"] = "OpenAI"
	case "opencommit":
		// B-125: skema v3 opencommit menolak berjalan tanpa
		// OCO_AI_PROVIDER ("set AI provider first").
		vars["OCO_AI_PROVIDER"] = "openai"
		// B-144+B-147: batas token yang memadai DAN koheren untuk
		// model reasoning — bawaan tool (500/1000) habis seluruhnya
		// untuk reasoning (EMPTY_MESSAGE), tetapi plafon 4000 tanpa
		// anggaran input pendamping membuat anggaran request tool
		// negatif (regresi B-147); ketiganya ditulis bersama (lihat
		// konstanta oco* di atas buildEnvVars).
		vars["OCO_TOKENS_MAX_INPUT"] = ocoTokensMaxInput
		vars["OCO_TOKENS_MAX_OUTPUT"] = ocoTokensMaxOutput
		vars["OCO_REASONING_MAX_TOKENS"] = ocoReasoningMaxTokens
	case "copilot":
		// B-128: mode BYOK resmi Copilot CLI menuntut jenis provider
		// dinyatakan eksplisit; tanpa ini base URL tidak dipakai.
		vars["COPILOT_PROVIDER_TYPE"] = "openai"
	}

	if tool.EnvVarBaseURL != "" {
		vars[tool.EnvVarBaseURL] = actualURL
	}

	if tool.EnvVarModel != "" {
		vars[tool.EnvVarModel] = model
	}

	return vars
}

// yamlKutip mengutip nilai sebagai skalar YAML bila memuat karakter yang
// tidak aman sebagai skalar polos. Bentuk kutip ganda JSON adalah skalar
// kutip ganda YAML yang sah.
func yamlKutip(v string) string {
	if v == "" {
		return `""`
	}
	if strings.ContainsAny(v, " \t#\"'\\") || strings.Contains(v, ": ") || strings.HasSuffix(v, ":") {
		q, err := json.Marshal(v)
		if err == nil {
			return string(q)
		}
	}
	return v
}

// perbaruiYAMLTopLevel memperbarui kunci skalar tingkat-atas (tanpa
// indentasi) di dokumen YAML yang sudah ada, mempertahankan baris lain
// apa adanya — config.yaml Goose bisa memuat blok lain milik pengguna
// (mis. extensions) yang tidak boleh dihancurkan Configure. Kunci yang
// dikelola muncul dua kali atau lebih disisakan satu (yang pertama,
// diperbarui); kunci yang belum ada ditambahkan di akhir.
func perbaruiYAMLTopLevel(existing string, pairs [][2]string) string {
	dikelola := make(map[string]bool, len(pairs))
	nilai := make(map[string]string, len(pairs))
	for _, p := range pairs {
		dikelola[p[0]] = true
		nilai[p[0]] = p[1]
	}
	tertulis := make(map[string]bool, len(pairs))
	var out []string
	for _, line := range strings.Split(existing, "\n") {
		kunci := ""
		if line != "" && line[0] != ' ' && line[0] != '\t' && line[0] != '#' {
			if i := strings.Index(line, ":"); i > 0 {
				kunci = strings.TrimSpace(line[:i])
			}
		}
		if kunci != "" && dikelola[kunci] {
			if tertulis[kunci] {
				continue // duplikat kunci terkelola: buang
			}
			tertulis[kunci] = true
			out = append(out, kunci+": "+yamlKutip(nilai[kunci]))
			continue
		}
		out = append(out, line)
	}
	hasil := strings.Join(out, "\n")
	for _, p := range pairs {
		if tertulis[p[0]] {
			continue
		}
		if hasil != "" && !strings.HasSuffix(hasil, "\n") {
			hasil += "\n"
		}
		hasil += p[0] + ": " + yamlKutip(p[1]) + "\n"
	}
	if hasil != "" && !strings.HasSuffix(hasil, "\n") {
		hasil += "\n"
	}
	return hasil
}

// perbaruiExtraOpenAIModels memperbarui berkas extra-openai-models.yaml
// milik SimonW llm (B-124): daftar YAML tingkat-atas berisi satu entri
// per model. Entri untuk model target ditulis ulang dari nilai gateway
// terkini; entri model LAIN milik pengguna dipertahankan apa adanya.
func perbaruiExtraOpenAIModels(existing, target, gwURL string) string {
	var kept []string
	var cur []string
	flush := func() {
		if len(cur) == 0 {
			return
		}
		milikTarget := false
		for _, l := range cur {
			t := strings.TrimSpace(strings.TrimPrefix(strings.TrimSpace(l), "- "))
			if strings.HasPrefix(t, "model_id:") {
				v := strings.Trim(strings.TrimSpace(strings.TrimPrefix(t, "model_id:")), `"'`)
				if v == target {
					milikTarget = true
				}
			}
		}
		if !milikTarget {
			kept = append(kept, cur...)
		}
		cur = nil
	}
	for _, l := range strings.Split(existing, "\n") {
		if strings.HasPrefix(l, "- ") || l == "-" {
			flush()
		}
		if strings.TrimSpace(l) == "" && len(cur) == 0 && len(kept) == 0 {
			continue
		}
		cur = append(cur, l)
	}
	flush()
	for len(kept) > 0 && strings.TrimSpace(kept[len(kept)-1]) == "" {
		kept = kept[:len(kept)-1]
	}
	var b strings.Builder
	if len(kept) > 0 {
		b.WriteString(strings.Join(kept, "\n"))
		b.WriteString("\n")
	}
	fmt.Fprintf(&b, "- model_id: %s\n", yamlKutip(target))
	fmt.Fprintf(&b, "  model_name: %s\n", yamlKutip(target))
	fmt.Fprintf(&b, "  api_base: %s\n", yamlKutip(gwURL))
	b.WriteString("  api_key_name: openai\n")
	return b.String()
}

// susunK8sGPTConfig menyusun isi k8sgpt.yaml (B-129) setara hasil
// `k8sgpt auth add` + `auth default`: seksi ai memuat defaultprovider
// openai dan satu entri provider openai (baseurl/model/password dari
// gateway). Provider non-openai dan seksi tingkat-atas lain milik
// pengguna dipertahankan; entri openai lama ditulis ulang.
func susunK8sGPTConfig(existing, target, gwURL, apiKey string) string {
	// Potong berkas menjadi seksi tingkat-atas (kunci di kolom 0).
	type seksi struct {
		kunci string
		baris []string
	}
	var daftar []seksi
	for _, l := range strings.Split(existing, "\n") {
		kunci := ""
		if l != "" && l[0] != ' ' && l[0] != '\t' && l[0] != '#' {
			if i := strings.Index(l, ":"); i > 0 {
				kunci = strings.TrimSpace(l[:i])
			}
		}
		if kunci != "" || len(daftar) == 0 {
			daftar = append(daftar, seksi{kunci: kunci})
		}
		daftar[len(daftar)-1].baris = append(daftar[len(daftar)-1].baris, l)
	}

	indentDari := func(l string) int {
		return len(l) - len(strings.TrimLeft(l, " "))
	}

	// Saring isi seksi ai lama: buang defaultprovider dan entri
	// provider bernama openai; sisanya (provider lain + kunci lain)
	// dipertahankan.
	var sisaAI []string
	var providerLain []string
	idxAI := -1
	for i, s := range daftar {
		if s.kunci != "ai" {
			continue
		}
		idxAI = i
		baris := s.baris
		// Lewati baris header "ai:" itu sendiri.
		isi := baris[1:]
		for j := 0; j < len(isi); j++ {
			l := isi[j]
			t := strings.TrimSpace(l)
			if strings.HasPrefix(t, "defaultprovider:") {
				continue
			}
			if t == "providers:" {
				induk := indentDari(l)
				j++
				var item []string
				flushItem := func() {
					if len(item) == 0 {
						return
					}
					openai := false
					for _, il := range item {
						it := strings.TrimSpace(strings.TrimPrefix(strings.TrimSpace(il), "- "))
						if it == "name: openai" {
							openai = true
						}
					}
					if !openai {
						providerLain = append(providerLain, item...)
					}
					item = nil
				}
				for ; j < len(isi); j++ {
					il := isi[j]
					if strings.TrimSpace(il) == "" {
						continue
					}
					if indentDari(il) <= induk {
						j--
						break
					}
					if strings.HasPrefix(strings.TrimSpace(il), "- ") {
						flushItem()
					}
					item = append(item, il)
				}
				flushItem()
				continue
			}
			sisaAI = append(sisaAI, l)
		}
	}

	var aiBaru []string
	aiBaru = append(aiBaru, "ai:")
	aiBaru = append(aiBaru, "  defaultprovider: openai")
	aiBaru = append(aiBaru, "  providers:")
	aiBaru = append(aiBaru, "    - name: openai")
	aiBaru = append(aiBaru, "      baseurl: "+yamlKutip(gwURL))
	aiBaru = append(aiBaru, "      model: "+yamlKutip(target))
	if apiKey != "" {
		aiBaru = append(aiBaru, "      password: "+yamlKutip(apiKey))
	}
	aiBaru = append(aiBaru, providerLain...)
	aiBaru = append(aiBaru, sisaAI...)

	var out []string
	tertulis := false
	for i, s := range daftar {
		if i == idxAI {
			out = append(out, aiBaru...)
			tertulis = true
			continue
		}
		// Buang baris kosong ekor dari potongan terakhir hasil Split.
		baris := s.baris
		if i == len(daftar)-1 {
			for len(baris) > 0 && strings.TrimSpace(baris[len(baris)-1]) == "" {
				baris = baris[:len(baris)-1]
			}
		}
		out = append(out, baris...)
	}
	if !tertulis {
		out = append(out, aiBaru...)
	}
	hasil := strings.Join(out, "\n")
	if !strings.HasSuffix(hasil, "\n") {
		hasil += "\n"
	}
	return hasil
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
	if err := tulisAtomik(envFile, []byte(newContent), 0600); err != nil {
		return err
	}

	return m.ensureShellAutoLoad(homeDir)
}

// ensureShellAutoLoad memastikan skrip login shell otomatis memuat ~/.routex/cli-env.sh.
// Kegagalan memasang hook di berkas MILIK PENGGUNA (.bashrc/.zshrc)
// dikembalikan ke pemanggil (audit F5): versi lama menelan semua error
// sehingga status "terkonfigurasi" bisa berarti hook tidak pernah
// terpasang dan env tidak pernah termuat di shell baru. SATU pengecualian:
// hook sistem /etc/profile.d bersifat best-effort — direktori itu milik
// root, dan menggagalkan seluruh Configure untuk gateway yang berjalan
// sebagai pengguna biasa akan mematahkan fitur padahal hook per-pengguna
// sudah cukup. Kegagalannya dicatat sebagai peringatan, bukan ditelan
// diam-diam.
func (m *Manager) ensureShellAutoLoad(homeDir string) error {
	if homeDir == "" {
		return nil
	}
	hook := "\n# >>> Route-X AI Gateway CLI Auto-Loader >>>\nif [ -f \"$HOME/.routex/cli-env.sh\" ]; then\n    . \"$HOME/.routex/cli-env.sh\"\nfi\n# <<< Route-X AI Gateway CLI Auto-Loader <<<\n"

	// Berkas rc ditulis ulang dengan MODE ASLINYA dan mengikuti symlink
	// (audit3 V-LN-33): versi lama memaksa 0644 (menurunkan 0600 milik
	// pengguna) dan tulisAtomik menimpa symlink itu sendiri dengan berkas
	// reguler, memutus tautan ke target yang dikelola dotfiles pengguna.
	pasangHook := func(rcPath string, prepend bool) error {
		data, err := os.ReadFile(rcPath)
		if err != nil {
			return nil // berkas tidak ada: tidak dibuatkan.
		}
		if strings.Contains(string(data), "Route-X AI Gateway CLI Auto-Loader") {
			return nil
		}
		target := rcPath
		if terurai, errEval := filepath.EvalSymlinks(rcPath); errEval == nil {
			target = terurai
		}
		perm := os.FileMode(0644)
		if fi, errStat := os.Stat(target); errStat == nil {
			perm = fi.Mode().Perm()
		}
		konten := string(data) + hook
		if prepend {
			konten = hook + "\n" + string(data)
		}
		if err := tulisAtomik(target, []byte(konten), perm); err != nil {
			return fmt.Errorf("memasang hook di %s: %w", rcPath, err)
		}
		return nil
	}

	if err := pasangHook(filepath.Join(homeDir, ".bashrc"), true); err != nil {
		return err
	}
	if err := pasangHook(filepath.Join(homeDir, ".zshrc"), false); err != nil {
		return err
	}

	profilePath := "/etc/profile.d/routex.sh"
	if fi, err := os.Stat("/etc/profile.d"); err == nil && fi.IsDir() {
		if data, err := os.ReadFile(profilePath); err != nil || !strings.Contains(string(data), "Route-X AI Gateway CLI Auto-Loader") {
			if err := tulisAtomik(profilePath, []byte(hook), 0644); err != nil {
				// Best-effort (lihat komentar fungsi): catat, jangan gagalkan.
				if m.logger != nil {
					m.logger.Warn("hook /etc/profile.d tidak terpasang (best-effort); hook per-pengguna tetap berlaku",
						"path", profilePath, "error", err)
				}
			}
		}
	}
	return nil
}

// applyToolSpecificConfig menulis berkas konfigurasi lokal jika aplikabel.
func (m *Manager) applyToolSpecificConfig(homeDir string, tool ToolDef, mode, target, gwURL, apiKey string) error {
	if homeDir == "" {
		return nil
	}

	switch tool.ID {
	case "claude":
		// Tulis ~/.claude/settings.json dalam skema sah Claude Code 2.x
		// (B-097): variabel hidup di objek `env`, BUKAN kunci root
		// snake_case `anthropic_base_url`/`model` yang tidak dikenal
		// tool (dokumentasi resmi Claude Code, halaman Environment
		// variables: "Add variables under the `env` key in a
		// settings.json file"). Setelan lain milik pengguna yang sudah
		// ada di berkas dipertahankan (pola yang sama dengan
		// opencode.json), dan sisa kunci root salah dari versi lama
		// dibuang. Key ikut ditulis di `env` sebagai ANTHROPIC_API_KEY
		// — nama yang sama dengan yang diekspor key.env untuk tool ini
		// — hanya bila Configure membawa key, konsisten dengan
		// opencode.json yang menaruh apiKey di berkas; berkas 0600
		// karena bisa memuat key itu. Tanpa key baru, entri key lama
		// dihapus supaya tidak ada kredensial hantu (pola V-LN-26).
		claudeDir := filepath.Join(homeDir, ".claude")
		_ = os.MkdirAll(claudeDir, 0755)
		settingsPath := filepath.Join(claudeDir, "settings.json")
		claudeURL := strings.TrimSuffix(gwURL, "/v1")
		cfg := make(map[string]any)
		if data, err := os.ReadFile(settingsPath); err == nil {
			_ = json.Unmarshal(data, &cfg)
			if cfg == nil {
				cfg = make(map[string]any)
			}
		}
		delete(cfg, "anthropic_base_url")
		env, ok := cfg["env"].(map[string]any)
		if !ok || env == nil {
			env = make(map[string]any)
			cfg["env"] = env
		}
		env["ANTHROPIC_BASE_URL"] = claudeURL
		env["ANTHROPIC_MODEL"] = target
		if apiKey != "" {
			env["ANTHROPIC_API_KEY"] = apiKey
		} else {
			delete(env, "ANTHROPIC_API_KEY")
		}
		cfg["model"] = target
		data, err := json.MarshalIndent(cfg, "", "  ")
		if err != nil {
			return fmt.Errorf("mengkodekan settings.json Claude: %w", err)
		}
		if err := tulisAtomik(settingsPath, data, 0600); err != nil {
			return fmt.Errorf("menulis %s: %w", settingsPath, err)
		}

	case "aider":
		// Tulis ~/.aider.conf.yml. Nama model WAJIB berprefiks provider
		// LiteLLM `openai/` (B-098) — sama seperti AIDER_MODEL di env.
		aiderConfigPath := filepath.Join(homeDir, ".aider.conf.yml")
		content := fmt.Sprintf("openai-api-base: %s\nmodel: %s\n", gwURL, modelAider(target))
		if err := tulisAtomik(aiderConfigPath, []byte(content), 0644); err != nil {
			return fmt.Errorf("menulis %s: %w", aiderConfigPath, err)
		}

	case "goose":
		// Tulis ~/.config/goose/config.yaml (B-099) dengan kunci yang
		// benar-benar dibaca Goose 1.x (dokumentasi resmi goose,
		// Configure LLM Provider): GOOSE_PROVIDER, GOOSE_MODEL, dan
		// OPENAI_HOST tanpa sufiks /v1. API key SENGAJA tidak ditulis
		// di berkas ini — dokumentasi goose menyatakan key di
		// config.yaml diabaikan; key sampai ke Goose lewat ekspor
		// OPENAI_API_KEY di key.env/cli-env.sh. Kunci tingkat-atas lain
		// milik pengguna (mis. extensions) dipertahankan.
		gooseDir := filepath.Join(homeDir, ".config", "goose")
		_ = os.MkdirAll(gooseDir, 0755)
		goosePath := filepath.Join(gooseDir, "config.yaml")
		var existing string
		if data, err := os.ReadFile(goosePath); err == nil {
			existing = string(data)
		}
		content := perbaruiYAMLTopLevel(existing, [][2]string{
			{"GOOSE_PROVIDER", "openai"},
			{"GOOSE_MODEL", target},
			{"OPENAI_HOST", strings.TrimSuffix(gwURL, "/v1")},
		})
		if err := tulisAtomik(goosePath, []byte(content), 0644); err != nil {
			return fmt.Errorf("menulis %s: %w", goosePath, err)
		}

	case "continue":
		// Tulis ~/.continue/config.yaml (B-101, perbaikan keamanan):
		// config lokal sah Continue 1.5.x (dokumentasi resmi Continue,
		// config.yaml Reference: name/version/schema wajib, entri
		// models ber-provider openai dengan apiBase + apiKey). Dengan
		// berkas ini cn memakai gateway Route-X; tanpa berkas ini cn
		// membangun config default dari env generik dan mengirim key
		// gateway (ANTHROPIC_API_KEY dari key.env) ke vendor asli.
		// apiBase memakai URL gateway DENGAN sufiks /v1 (bentuk yang
		// dibuktikan run diagnostik Langkah 6). Berkas 0600 karena
		// memuat apiKey.
		continueDir := filepath.Join(homeDir, ".continue")
		_ = os.MkdirAll(continueDir, 0755)
		continuePath := filepath.Join(continueDir, "config.yaml")
		var b strings.Builder
		b.WriteString("name: Route-X\nversion: 1.0.0\nschema: v1\nmodels:\n")
		fmt.Fprintf(&b, "  - name: %s\n", yamlKutip("Route-X "+target))
		b.WriteString("    provider: openai\n")
		fmt.Fprintf(&b, "    model: %s\n", yamlKutip(target))
		fmt.Fprintf(&b, "    apiBase: %s\n", yamlKutip(gwURL))
		if apiKey != "" {
			fmt.Fprintf(&b, "    apiKey: %s\n", yamlKutip(apiKey))
		}
		b.WriteString("    roles:\n      - chat\n      - edit\n      - apply\n      - summarize\n")
		if err := tulisAtomik(continuePath, []byte(b.String()), 0600); err != nil {
			return fmt.Errorf("menulis %s: %w", continuePath, err)
		}

	case "fabric":
		// Tulis ~/.config/fabric/.env dalam bentuk yang benar-benar
		// dibaca fabric 1.4.x (B-123, terbukti live): vendor OpenAI
		// membaca OPENAI_API_BASE_URL — OPENAI_BASE_URL warisan
		// diabaikan dan kunci gateway justru terkirim ke
		// api.openai.com asli. DEFAULT_VENDOR wajib ada, tanpa itu
		// fabric menolak berjalan. Kunci ditulis ke .env ini (bentuk
		// diagnostik yang terbukti) hanya bila Configure membawa
		// kunci; berkas ditulis ulang penuh, jadi konfigurasi ulang
		// tanpa kunci tidak meninggalkan kredensial hantu. Izin 0600
		// karena berkas bisa memuat kunci.
		fabricDir := filepath.Join(homeDir, ".config", "fabric")
		_ = os.MkdirAll(fabricDir, 0755)
		envPath := filepath.Join(fabricDir, ".env")
		var fb strings.Builder
		if apiKey != "" {
			fmt.Fprintf(&fb, "OPENAI_API_KEY=%s\n", apiKey)
		}
		fmt.Fprintf(&fb, "OPENAI_API_BASE_URL=%s\n", gwURL)
		fb.WriteString("DEFAULT_VENDOR=OpenAI\n")
		fmt.Fprintf(&fb, "DEFAULT_MODEL=%s\n", target)
		if err := tulisAtomik(envPath, []byte(fb.String()), 0600); err != nil {
			return fmt.Errorf("menulis %s: %w", envPath, err)
		}
		// Vendor OpenAI fabric memakai Responses API secara bawaan,
		// yang dijawab gateway 400; disableResponsesAPI di
		// config.yaml (dimuat otomatis fabric, terbukti live)
		// mengembalikannya ke Chat Completions. Kunci tingkat-atas
		// lain milik pengguna dipertahankan.
		configPath := filepath.Join(fabricDir, "config.yaml")
		var existingCfg string
		if data, err := os.ReadFile(configPath); err == nil {
			existingCfg = string(data)
		}
		cfgContent := perbaruiYAMLTopLevel(existingCfg, [][2]string{
			{"disableResponsesAPI", "true"},
		})
		if err := tulisAtomik(configPath, []byte(cfgContent), 0644); err != nil {
			return fmt.Errorf("menulis %s: %w", configPath, err)
		}
		// Fabric menuntut pola untuk mode non-interaktif; sediakan
		// satu pola minimal milik Route-X. Pola yang sudah ada tidak
		// ditimpa (mungkin sudah disesuaikan pengguna).
		patternPath := filepath.Join(fabricDir, "patterns", "routex", "system.md")
		if _, err := os.Stat(patternPath); os.IsNotExist(err) {
			_ = os.MkdirAll(filepath.Dir(patternPath), 0755)
			isi := "You are a helpful assistant accessed through the Route-X gateway. Follow the user's instructions and answer directly and concisely.\n"
			if err := tulisAtomik(patternPath, []byte(isi), 0644); err != nil {
				return fmt.Errorf("menulis %s: %w", patternPath, err)
			}
		}

	case "sgpt":
		// Tulis ~/.config/shell_gpt/.sgptrc (B-122): sgpt membaca
		// base URL dari kunci API_BASE_URL (sgpt/config.py), BUKAN
		// OPENAI_BASE_URL — nilai warisan hanya tertolong fallback
		// SDK tanpa sufiks /v1 dan jatuh ke POST /chat/completions.
		sgptDir := filepath.Join(homeDir, ".config", "shell_gpt")
		_ = os.MkdirAll(sgptDir, 0755)
		sgptPath := filepath.Join(sgptDir, ".sgptrc")
		content := fmt.Sprintf("API_BASE_URL=%s\nDEFAULT_MODEL=%s\n", gwURL, target)
		if err := tulisAtomik(sgptPath, []byte(content), 0644); err != nil {
			return fmt.Errorf("menulis %s: %w", sgptPath, err)
		}

	case "llm":
		// Tulis konfigurasi SimonW llm (B-124, terbukti live): model
		// gateway hanya dikenal tool lewat extra-openai-models.yaml
		// (api_key_name: openai membuat llm memakai OPENAI_API_KEY
		// dari lingkungan — tanpanya llm menahan kunci untuk api_base
		// kustom), dan model bawaan hanya lewat default_model.txt
		// (LLM_DEFAULT_MODEL tidak dibaca llm sama sekali). Entri
		// model lain milik pengguna di berkas extra dipertahankan.
		llmDir := filepath.Join(homeDir, ".config", "io.datasette.llm")
		_ = os.MkdirAll(llmDir, 0755)
		extraPath := filepath.Join(llmDir, "extra-openai-models.yaml")
		var existingExtra string
		if data, err := os.ReadFile(extraPath); err == nil {
			existingExtra = string(data)
		}
		if err := tulisAtomik(extraPath, []byte(perbaruiExtraOpenAIModels(existingExtra, target, gwURL)), 0644); err != nil {
			return fmt.Errorf("menulis %s: %w", extraPath, err)
		}
		defaultPath := filepath.Join(llmDir, "default_model.txt")
		if err := tulisAtomik(defaultPath, []byte(target+"\n"), 0644); err != nil {
			return fmt.Errorf("menulis %s: %w", defaultPath, err)
		}

	case "opencommit":
		// Tulis ~/.opencommit dalam skema v3 (B-125, terbukti live):
		// OCO_AI_PROVIDER/OCO_API_KEY/OCO_API_URL. Tanpa
		// OCO_AI_PROVIDER, opencommit v3 gagal di migrasi internalnya
		// sendiri sebelum panggilan model apa pun. Berkas ditulis
		// ulang penuh (kunci lama tidak tertinggal) dan berizin 0600
		// karena bisa memuat kunci.
		ocoPath := filepath.Join(homeDir, ".opencommit")
		var ob strings.Builder
		ob.WriteString("OCO_AI_PROVIDER=openai\n")
		if apiKey != "" {
			fmt.Fprintf(&ob, "OCO_API_KEY=%s\n", apiKey)
		}
		fmt.Fprintf(&ob, "OCO_API_URL=%s\n", gwURL)
		fmt.Fprintf(&ob, "OCO_MODEL=%s\n", target)
		// B-144+B-147: batas token skema v3 (lihat konstanta oco* di
		// atas buildEnvVars) — tanpa ini bawaan tool 500/1000 habis
		// untuk reasoning model reasoning dan oco gagal EMPTY_MESSAGE;
		// tanpa OCO_TOKENS_MAX_INPUT pendamping, plafon 4000 membuat
		// anggaran request tool negatif dan oco gagal lokal (B-147).
		fmt.Fprintf(&ob, "OCO_TOKENS_MAX_INPUT=%s\n", ocoTokensMaxInput)
		fmt.Fprintf(&ob, "OCO_TOKENS_MAX_OUTPUT=%s\n", ocoTokensMaxOutput)
		fmt.Fprintf(&ob, "OCO_REASONING_MAX_TOKENS=%s\n", ocoReasoningMaxTokens)
		if err := tulisAtomik(ocoPath, []byte(ob.String()), 0600); err != nil {
			return fmt.Errorf("menulis %s: %w", ocoPath, err)
		}

	case "k8sgpt":
		// Tulis ~/.config/k8sgpt/k8sgpt.yaml (B-129, setara hasil
		// `k8sgpt auth add -b openai -u <gateway>/v1 -m <target>
		// -p <kunci>` + `k8sgpt auth default -p openai`, keduanya
		// terbukti pada diagnostik live): struktur ai.providers +
		// ai.defaultprovider yang dibaca k8sgpt 0.4.x. Env rekaan
		// K8SGPT_OPENAI* tidak ada di biner dan tidak lagi ditulis.
		// Provider non-openai dan seksi lain milik pengguna
		// dipertahankan. Izin 0600 karena berkas memuat password.
		k8sDir := filepath.Join(homeDir, ".config", "k8sgpt")
		_ = os.MkdirAll(k8sDir, 0755)
		k8sPath := filepath.Join(k8sDir, "k8sgpt.yaml")
		var existingK8s string
		if data, err := os.ReadFile(k8sPath); err == nil {
			existingK8s = string(data)
		}
		if err := tulisAtomik(k8sPath, []byte(susunK8sGPTConfig(existingK8s, target, gwURL, apiKey)), 0600); err != nil {
			return fmt.Errorf("menulis %s: %w", k8sPath, err)
		}

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
			} else {
				// Konfigurasi ulang tanpa kunci tidak boleh meninggalkan
				// kunci LAMA yang pernah ditulis versi sebelumnya di berkas
				// ini (kredensial hantu — audit3 V-LN-26).
				delete(opts, "apiKey")
			}
		}
		models, ok := routexProv["models"].(map[string]any)
		if !ok || models == nil {
			models = make(map[string]any)
			routexProv["models"] = models
		}
		models[target] = map[string]any{"name": target}
		cfg["model"] = "routex/" + target
		data, err := json.MarshalIndent(cfg, "", "  ")
		if err != nil {
			return fmt.Errorf("mengkodekan opencode.json: %w", err)
		}
		// 0600, bukan 0644: berkas ini bisa memuat apiKey plaintext
		// (opts.apiKey di atas) — versi lama membuatnya terbaca seluruh
		// pengguna lokal (audit3 V-LN-25).
		if err := tulisAtomik(configPath, data, 0600); err != nil {
			return fmt.Errorf("menulis %s: %w", configPath, err)
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

// himpunanVarKunci adalah seluruh nama variabel yang membawa API key:
// variabel khas tiap tool DITAMBAH nama generik key.env. Versi lama hanya
// memakai variabel tool, sehingga ROUTEX_API_KEY sendiri — nama yang
// paling utama — tidak pernah dibersihkan/diredaksi (audit3 V-LN-34).
// namaVarKunciWarisan adalah nama variabel kunci yang pernah diekspor
// versi lama tetapi sudah tidak diproduksi lagi (B-099: GOOSE_API_KEY
// tidak dibaca Goose; B-101: CONTINUE_API_KEY tidak dibaca cn). Keduanya
// tetap masuk himpunan redaksi/pembersihan supaya skrip warisan yang
// masih memuatnya dibersihkan, bukan bocor.
// B-125/B-128/B-132: nama kunci skema warisan yang diganti atau entri
// registri yang dicabut (OCO_OPENAI_API_KEY → OCO_API_KEY,
// GITHUB_COPILOT_API_KEY → COPILOT_PROVIDER_API_KEY, CODECOMPANION_*
// untuk tool yang tidak eksis) tetap masuk himpunan yang sama.
var namaVarKunciWarisan = []string{"GOOSE_API_KEY", "CONTINUE_API_KEY", "OCO_OPENAI_API_KEY", "GITHUB_COPILOT_API_KEY", "CODECOMPANION_API_KEY"}

func himpunanVarKunci() map[string]bool {
	keyVars := map[string]bool{}
	for _, t := range SupportedTools() {
		if t.EnvVarAPIKey != "" {
			keyVars[t.EnvVarAPIKey] = true
		}
	}
	for _, nama := range namaVarKunciGenerik {
		keyVars[nama] = true
	}
	for _, nama := range namaVarKunciWarisan {
		keyVars[nama] = true
	}
	return keyVars
}

func (m *Manager) removeAPIKeysFromScript(content string) string {
	keyVars := himpunanVarKunci()
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
// skrip export dengan tampilan tersamar. Baris TANPA prefiks `export ` ikut
// ditangani (audit3 V-LN-34): berkas warisan yang ditulis tangan bisa memuat
// penugasan polos `NAMA=nilai` yang sama berbahayanya.
func (m *Manager) redactAPIKeysInScript(content string) string {
	keyVars := himpunanVarKunci()
	lines := strings.Split(content, "\n")
	for i, line := range lines {
		trimmed := strings.TrimSpace(line)
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
