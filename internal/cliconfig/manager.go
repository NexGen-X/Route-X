package cliconfig

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
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

// penghapusSetelan adalah kemampuan OPSIONAL store untuk menghapus
// setelan (repo identity.Settings memenuhinya). Pembersihan preferensi
// tool yang dicabut dari registri memakainya bila tersedia; store
// tanpa kemampuan ini cukup mengabaikan id tak dikenal saat memuat.
type penghapusSetelan interface {
	Delete(ctx context.Context, key string) error
}

// Manager mengelola pemindaian perkakas CLI dan penerapan konfigurasi multi-mode.
type Manager struct {
	settings SettingsStore
	cipher   *security.Cipher
	logger   *slog.Logger
	mu       sync.RWMutex

	// cacheVersi menahan hasil detectVersion per path biner (audit3
	// V-LN-37): Scan mengeksekusi `--version` SETIAP biner pada SETIAP
	// panggilan, jadi membuka halaman CLI berarti belasan proses anak. Versi
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
	// B-279: selesaikan symlink pada jalur tujuan lebih dulu, meniru
	// jalur hook rc (pasangHook) yang sudah symlink-aware sejak
	// V-LN-33. Rename apa adanya menimpa symlink dotfiles (GNU Stow/
	// chezmoi) dengan berkas reguler: tautan putus senyap dan berkas
	// target di repo dotfiles tidak pernah terbarui. Dengan menulis
	// ke target hasil EvalSymlinks, tautan dipertahankan dan berkas
	// sementara lahir di direktori target yang sama (rename tetap
	// satu filesystem). Symlink putus (target belum ada) tidak bisa
	// diselesaikan — EvalSymlinks mengembalikan error dan jalur asal
	// dipakai apa adanya, sama seperti perilaku lama.
	if resolved, err := filepath.EvalSymlinks(path); err == nil && resolved != path {
		path = resolved
	}
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

// muatKonfigJSON memuat berkas konfigurasi JSON berbentuk objek milik
// sebuah tool untuk digabungkan dengan setelan Route-X (B-217).
//
// Pembedaan yang dijaga fungsi ini adalah antara "belum ada berkas" dan
// "berkas ada tetapi tidak bisa kami baca":
//   - Berkas tidak ada → peta kosong tanpa error: memulai dari nol
//     memang sah.
//   - Berkas ada tetapi gagal dibaca atau tidak terurai → error. Versi
//     lama menelan kegagalan unmarshal (_ = json.Unmarshal) lalu menimpa
//     seluruh berkas dengan data Route-X saja, sehingga setelan pengguna
//     yang tidak terurai oleh pengurai ketat kami musnah pada Configure
//     pertama. Kegagalannya kini jujur: Configure dibatalkan dan berkas
//     tidak disentuh sama sekali.
//   - toleranJSONC menyalakan penguraian toleran JSONC (komentar + koma
//     menggantung) untuk tool yang format resminya memang JSONC
//     (OpenCode): berkas yang sah menurut tool-nya tidak boleh gagal
//     terurai di sini.
func muatKonfigJSON(path string, toleranJSONC bool) (map[string]any, error) {
	data, err := os.ReadFile(path)
	if errors.Is(err, os.ErrNotExist) {
		return make(map[string]any), nil
	}
	if err != nil {
		return nil, fmt.Errorf("membaca %s: %w", path, err)
	}
	if toleranJSONC {
		data = uraiJSONCToleran(data)
	}
	cfg := make(map[string]any)
	if err := json.Unmarshal(data, &cfg); err != nil {
		return nil, fmt.Errorf("berkas konfigurasi %s tidak bisa diurai (%v); Configure dibatalkan agar setelan di dalamnya tidak tertimpa — perbaiki atau pindahkan berkas itu, lalu coba lagi", path, err)
	}
	if cfg == nil {
		cfg = make(map[string]any)
	}
	return cfg, nil
}

// uraiJSONCToleran mengubah JSONC menjadi JSON ketat yang bisa diterima
// encoding/json: komentar // dan /* */ DI LUAR string dibuang, begitu
// pula koma yang menggantung persis sebelum } atau ]. Isi string —
// termasuk "//" di dalam URL seperti "https://..." — tidak disentuh:
// mesin keadaannya melacak apakah ia sedang di dalam string dan apakah
// karakter sebelumnya backslash.
func uraiJSONCToleran(data []byte) []byte {
	var bersih []byte
	dalamString := false
	lolos := false // karakter sebelumnya backslash, di dalam string
	for i := 0; i < len(data); i++ {
		c := data[i]
		if dalamString {
			bersih = append(bersih, c)
			switch {
			case lolos:
				lolos = false
			case c == '\\':
				lolos = true
			case c == '"':
				dalamString = false
			}
			continue
		}
		switch {
		case c == '"':
			dalamString = true
			bersih = append(bersih, c)
		case c == '/' && i+1 < len(data) && data[i+1] == '/':
			// Komentar baris: lewati sampai newline; newline-nya sendiri
			// dipertahankan sebagai pemisah token.
			for i < len(data) && data[i] != '\n' {
				i++
			}
			i-- // kompensasi increment loop: berhenti tepat sebelum newline
		case c == '/' && i+1 < len(data) && data[i+1] == '*':
			// Komentar blok: lewati sampai */.
			i += 2
			for i+1 < len(data) && !(data[i] == '*' && data[i+1] == '/') {
				i++
			}
			i++ // berhenti di '/' penutup; increment loop melewatinya
		default:
			bersih = append(bersih, c)
		}
	}
	// Koma menggantung: koma yang hanya diikuti whitespace sebelum } atau ].
	var hasil []byte
	dalamString = false
	lolos = false
	for i := 0; i < len(bersih); i++ {
		c := bersih[i]
		if dalamString {
			hasil = append(hasil, c)
			switch {
			case lolos:
				lolos = false
			case c == '\\':
				lolos = true
			case c == '"':
				dalamString = false
			}
			continue
		}
		if c == '"' {
			dalamString = true
			hasil = append(hasil, c)
			continue
		}
		if c == ',' {
			j := i + 1
			for j < len(bersih) && (bersih[j] == ' ' || bersih[j] == '\t' || bersih[j] == '\n' || bersih[j] == '\r') {
				j++
			}
			if j < len(bersih) && (bersih[j] == '}' || bersih[j] == ']') {
				continue // buang koma ini
			}
		}
		hasil = append(hasil, c)
	}
	return hasil
}

// cariConfigPath mengembalikan path absolut berkas konfigurasi
// pertama milik tool yang ada di disk, atau string kosong bila
// tidak ada. Dipakai bersama oleh Scan dan Configure (B-160):
// Configure dulu tidak pernah mengisi ConfigPath, sehingga respons
// Configure selalu melaporkan config_file kosong walau berkasnya
// baru saja ditulis — sementara Scan untuk keadaan yang sama
// melaporkannya dengan benar.
func cariConfigPath(tool ToolDef, homeDir string) string {
	for _, relConfig := range tool.ConfigPaths {
		fullPath := filepath.Join(homeDir, relConfig)
		if _, err := os.Stat(fullPath); err == nil {
			return fullPath
		}
	}
	return ""
}

func (m *Manager) Scan(ctx context.Context, defaultGatewayURL string) ([]ToolStatus, error) {
	m.mu.RLock()
	defer m.mu.RUnlock()

	// Preferensi tersimpan milik tool yang sudah dicabut dari
	// registri dibersihkan/diabaikan di jalur pemuatan ini, tanpa
	// menyentuh preferensi tool yang bertahan.
	m.bersihkanPreferensiToolTercabut(ctx)

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
	// terburuk melonjak beberapa kali lipat.
	// Dengan maksimal 8 proses versi serentak, waktu deteksi
	// terburuk Scan tertahan di beberapa gelombang × 5 detik — di bawah
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
		status.ConfigPath = cariConfigPath(tool, homeDir)

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
	// dan seluruh berkas yang disentuh di-snapshot sebelum ditulis.
	// Versi lama menyimpan ke DB lebih dulu — kegagalan langkah berkas
	// sesudahnya meninggalkan DB yang mengklaim konfigurasi yang tidak
	// pernah diterapkan di disk.
	//
	// B-218: jepretan kini mencakup SEMUA berkas yang disentuh Configure
	// (key.env, cli-env.sh, target hook shell, berkas tool), bukan hanya
	// key.env. Versi sebelumnya: kegagalan syncGlobalEnvScript/
	// applyToolSpecificConfig hanya memulihkan key.env — cli-env.sh,
	// hook .bashrc/.zshrc, dan berkas tool tertinggal dalam keadaan
	// baru — dan kegagalan Put di akhir tidak memulihkan apa pun,
	// sehingga disk (baru) dan DB (lama) berakhir tidak konsisten.
	jepretan := jepretSemuaBerkasConfigure(homeDir, p.ToolID)
	pulihkanSemua := func() {
		for _, j := range jepretan {
			j.pulihkan()
		}
	}
	var keyEnvPath string
	if homeDir != "" {
		routexDir := filepath.Join(homeDir, ".routex")
		if err := os.MkdirAll(routexDir, 0755); err != nil {
			return nil, fmt.Errorf("menyiapkan direktori %s: %w", routexDir, err)
		}
		keyEnvPath = filepath.Join(routexDir, "key.env")
		// key.env DIBANGUN ULANG dari seluruh konfigurasi tersimpan, bukan
		// ditimpa empat nama generik milik tool terakhir saja (audit3
		// V-LN-24/26/27): setiap tool berkonfigurasi mendapat baris
		// variabelnya sendiri, dan tool lain tidak lagi hilang dari berkas
		// hanya karena tool lain dikonfigurasi sesudahnya.
		if err := m.tulisUlangKeyEnv(ctx, keyEnvPath, p.ToolID, p.APIKey, stored); err != nil {
			return nil, err
		}
	}

	if err := m.syncGlobalEnvScript(homeDir, toolDef, fullEnvVars); err != nil {
		pulihkanSemua()
		return nil, fmt.Errorf("menyinkronkan skrip env global CLI: %w", err)
	}

	// Tulis file konfigurasi lokal spesifik jika ada direktorinya.
	// B-215: penulis berkas menerima kunci EFEKTIF — kunci baru dari
	// permintaan ini bila ada, jika tidak kunci tersimpan yang baru
	// saja dipertahankan di StoredConfig di atas (didekripsi di
	// memori, tidak pernah ditulis apa pun selain ke berkas tool
	// itu sendiri). Versi lama meneruskan p.APIKey mentah, sehingga
	// konfigurasi ulang tanpa kunci menghapus api_key dari berkas
	// tool (paling fatal di Crush, yang tak punya jalur env) padahal
	// DB tetap mengklaim kunci tersimpan.
	kunciEfektif := p.APIKey
	if kunciEfektif == "" {
		kunciEfektif = m.kunciTersimpan(stored, p.ToolID)
	}
	if err := m.applyToolSpecificConfig(ctx, homeDir, toolDef, p.Mode, p.Target, gwURL, kunciEfektif); err != nil {
		pulihkanSemua()
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
			// B-218: disk ikut dipulihkan ke jepretan, supaya keadaan
			// lama di DB dan keadaan lama di disk tetap sepasang.
			pulihkanSemua()
			return nil, fmt.Errorf("menyimpan konfigurasi CLI %s ke database: %w", p.ToolID, err)
		}
	}

	execPath := m.findBinary(toolDef.BinaryNames, homeDir)
	status := &ToolStatus{
		ID:          toolDef.ID,
		Name:        toolDef.Name,
		Description: toolDef.Description,
		Category:    toolDef.Category,
		Installed:   execPath != "",
		Path:        execPath,
		Version:     m.detectVersionUntukTool(ctx, toolDef, execPath),
		// B-160: berkas konfigurasi baru saja ditulis oleh
		// applyToolSpecificConfig di atas, jadi laporkan lokasinya
		// dengan helper yang sama seperti Scan — respons Configure
		// tidak lagi ber-config_file kosong untuk tool berbasis
		// berkas.
		ConfigPath:     cariConfigPath(toolDef, homeDir),
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

// --- Jepretan & pemulihan Configure (B-218) ---

// jepretanBerkas adalah potret satu berkas sebelum Configure menulis
// ulang: isi + izin semula, penanda tidak-ada, atau penanda
// takTersentuh untuk berkas yang ada tetapi tidak terbaca sebagai
// berkas biasa (direktori, galat baca) — keadaan yang tidak diketahui
// tidak ditebak saat pemulihan.
type jepretanBerkas struct {
	jalur        string
	isi          []byte
	ada          bool
	izin         os.FileMode
	takTersentuh bool
}

func jepretBerkasConfigure(jalur string) jepretanBerkas {
	fi, err := os.Stat(jalur)
	if err != nil {
		return jepretanBerkas{jalur: jalur}
	}
	j := jepretanBerkas{jalur: jalur, ada: true, izin: fi.Mode().Perm()}
	if fi.IsDir() {
		j.takTersentuh = true
		return j
	}
	isi, err := os.ReadFile(jalur)
	if err != nil {
		j.takTersentuh = true
		return j
	}
	j.isi = isi
	return j
}

// pulihkan mengembalikan berkas ke keadaan jepretan: semula tidak
// ada → ciptaan Configure dihapus; semula ada → ditulis ulang atomik
// dengan isi dan izin semula. Galat pemulihan diabaikan per berkas
// (best effort): galat utama Configure sudah dikembalikan ke
// pemanggil, dan satu berkas yang gagal pulih tidak boleh menghentikan
// pemulihan berkas lain.
func (j jepretanBerkas) pulihkan() {
	if j.takTersentuh {
		return
	}
	if !j.ada {
		_ = os.Remove(j.jalur)
		return
	}
	_ = tulisAtomik(j.jalur, j.isi, j.izin)
}

// jalurBerkasConfigure menyusun SEMUA jalur berkas yang mungkin
// ditulis satu panggilan Configure untuk tool tertentu: key.env,
// cli-env.sh, target hook shell, dan berkas tool dari
// applyToolSpecificConfig.
//
// Target hook dinilai lewat penyelesaian symlink karena pasangHook
// menulis ke berkas hasil EvalSymlinks. /etc/profile.d/routex.sh
// SENGAJA dikecualikan: berkas sistem bersama itu ditulis best-effort,
// isinya deterministik mengikuti key.env, dan menghapus/menimpanya
// saat rollback justru bisa merusak salinan milik shell lain.
//
// WAJIB sinkron dengan penulisnya: jalur tool di sini harus sama
// dengan yang ditulis applyToolSpecificConfig (dan hermes menulis dua
// berkas berurutan — keduanya terdaftar). Menambah case penulisan baru
// di sana tanpa memperbarui peta ini mengembalikan lubang B-218 untuk
// tool itu. copilot (murni env) dan cline (menjalankan CLI resmi yang
// menulis ke tokonya sendiri) tidak menulis berkas dari jalur tool,
// jadi tidak mempunyai entri.
func jalurBerkasConfigure(homeDir, toolID string) []string {
	if homeDir == "" {
		return nil
	}
	jalur := []string{
		filepath.Join(homeDir, ".routex", "key.env"),
		filepath.Join(homeDir, ".routex", "cli-env.sh"),
	}
	for _, rc := range []string{".bashrc", ".zshrc"} {
		rcPath := filepath.Join(homeDir, rc)
		if resolved, err := filepath.EvalSymlinks(rcPath); err == nil {
			rcPath = resolved
		}
		jalur = append(jalur, rcPath)
	}
	switch toolID {
	case "claude":
		jalur = append(jalur, filepath.Join(homeDir, ".claude", "settings.json"))
	case "aider":
		jalur = append(jalur, filepath.Join(homeDir, ".aider.conf.yml"))
	case "goose":
		jalur = append(jalur, filepath.Join(homeDir, ".config", "goose", "config.yaml"))
	case "opencode":
		jalur = append(jalur, filepath.Join(homeDir, ".config", "opencode", "opencode.json"))
	case "codex":
		jalur = append(jalur, filepath.Join(homeDir, ".codex", "config.toml"))
	case "grok-build":
		jalur = append(jalur, filepath.Join(homeDir, ".grok", "config.toml"))
	case "qwen":
		jalur = append(jalur, filepath.Join(homeDir, ".qwen", "settings.json"))
	case "crush":
		jalur = append(jalur, filepath.Join(homeDir, ".config", "crush", "crush.json"))
	case "hermes":
		jalur = append(jalur,
			filepath.Join(homeDir, ".hermes", "config.yaml"),
			filepath.Join(homeDir, ".hermes", ".env"))
	}
	return jalur
}

func jepretSemuaBerkasConfigure(homeDir, toolID string) []jepretanBerkas {
	jalur := jalurBerkasConfigure(homeDir, toolID)
	jepretan := make([]jepretanBerkas, 0, len(jalur))
	for _, p := range jalur {
		jepretan = append(jepretan, jepretBerkasConfigure(p))
	}
	return jepretan
}

// bersihkanPreferensiToolTercabut menangani data preferensi
// tersimpan `cli:config:<id>` untuk id yang TIDAK lagi ada di
// registri (perombakan Tahap 1 mencabut 12 id): entri seperti itu
// diabaikan saat memuat dan — bila store mendukung penghapusan —
// dihapus dari store, TANPA menyentuh entri id yang bertahan.
// Kegagalan membaca/menghapus tidak pernah menggagalkan pemuatan:
// entri tak dikenal tetap diabaikan oleh semua jalur baca lain
// (Scan hanya memuat id registri; susunKontenKeyEnv melewati id tak
// dikenal), jadi pembersihan di sini adalah pemeliharaan, bukan
// syarat kebenaran.
func (m *Manager) bersihkanPreferensiToolTercabut(ctx context.Context) {
	if m.settings == nil {
		return
	}
	semua, err := m.settings.All(ctx)
	if err != nil {
		return
	}
	penghapus, bisaHapus := m.settings.(penghapusSetelan)
	for _, s := range semua {
		toolID, ok := strings.CutPrefix(s.Key, "cli:config:")
		if !ok {
			continue
		}
		if _, dikenal := cariToolDef(toolID); dikenal {
			continue
		}
		if bisaHapus {
			if err := penghapus.Delete(ctx, s.Key); err != nil && m.logger != nil {
				m.logger.WarnContext(ctx, "gagal membersihkan preferensi CLI tool tercabut",
					"tool", toolID, "error", err)
			}
		}
	}
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

// kunciTersimpan mengembalikan API key plaintext dari sebuah
// StoredConfig — ciphertext didekripsi bila ada, jika tidak field
// plaintext warisan dipakai. Kosong berarti tool itu memang tidak
// punya kunci tersimpan (atau dekripsi gagal); pemanggil
// memperlakukannya sebagai "tidak ada kunci", bukan error.
func (m *Manager) kunciTersimpan(stored StoredConfig, toolID string) string {
	if stored.APIKeyEncrypted != "" {
		if m.cipher == nil {
			return ""
		}
		if dec, err := m.cipher.Decrypt(stored.APIKeyEncrypted, security.CLIToolAAD(toolID)); err == nil {
			return string(dec)
		}
		return ""
	}
	return stored.APIKey
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
// yang membawa kunci: variabel khas tiap tool (COPILOT_PROVIDER_API_KEY,
// OPENAI_API_KEY khas goose, dst.) tidak pernah terisi sama sekali
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
	// config/nama khasnya masing-masing.
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
// kosong secara jujur.
func (m *Manager) detectVersionUntukTool(ctx context.Context, tool ToolDef, path string) string {
	if path == "" || tool.VersionArg == "" {
		return ""
	}
	return m.detectVersion(ctx, path, tool.VersionArg)
}

// detectVersion menjalankan perintah versi untuk biner yang terdeteksi.
// Hasilnya (termasuk kegagalan) ditahan ttlCacheVersi per path+argumen
// supaya Scan tidak mengeksekusi ulang seluruh biner registri pada
// setiap panggilan (audit3 V-LN-37).
func (m *Manager) detectVersion(ctx context.Context, path string, versionArg string) string {
	if path == "" || versionArg == "" {
		return ""
	}

	kunciCache := path + "\x00" + versionArg
	m.cacheVersiMu.Lock()
	if m.cacheVersi != nil {
		if e, ok := m.cacheVersi[kunciCache]; ok && time.Since(e.pada) < ttlCacheVersi {
			m.cacheVersiMu.Unlock()
			return e.versi
		}
	}
	m.cacheVersiMu.Unlock()
	versi := m.detectVersionLangsung(ctx, path, versionArg, nil)
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
// nyata tool Python yang sah (terukur konsisten 1,66–1,68 detik pada
// Uji Live Putaran 2 untuk satu tool Python registri lama), sehingga
// versi tool seperti itu selalu tampil kosong. 5 detik menampung
// startup itu dengan ruang ±3×, dan tetap merupakan batas keras per
// tool yang berlaku SAMA untuk semua tool: biner yang menggantung
// tetap terpotong pada batas ini. Biaya total tertahan oleh cache
// versi 5 menit dan deteksi paralel terbatas di Scan (lihat komentar
// di Scan); jalur Configure hanya mendeteksi satu tool, jadi paling
// lama tertahan batas ini sekali.
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
// yang mencetak baris pembuka non-versi (pada uji live, satu tool
// menampilkan harfiah "None") melaporkan omong kosong itu sebagai
// versinya. Keluaran nyata tool registri berbentuk semver polos
// ("1.5.47") atau berprefiks nama seperti "codex-cli 0.153.4" —
// semuanya baris yang memuat angka. Aturannya: baris pertama yang
// memuat angka menang; baris placeholder yang diketahui bukan versi
// ("none", "null", "unknown", "undefined") tidak pernah dilaporkan
// sebagai versi.
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

// buildEnvVars menyusun map environment variables untuk tool tertentu.
func (m *Manager) buildEnvVars(tool ToolDef, mode, target, gwURL string) map[string]string {
	vars := make(map[string]string)

	// Kejujuran katalog: tool yang tidak dapat diarahkan ke gateway
	// tidak mendapat ekspor env apa pun — ekspor warisannya terbukti
	// tidak dibaca tool dan hanya mengesankan dukungan yang tidak ada.
	if tool.UnsupportedReason != "" {
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
// ctx dipakai kasus cline yang menjalankan perintah resmi CLI itu sendiri.
func (m *Manager) applyToolSpecificConfig(ctx context.Context, homeDir string, tool ToolDef, mode, target, gwURL, apiKey string) error {
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
		// — nama yang sama dengan yang diekspor key.env untuk tool ini.
		// apiKey di sini adalah kunci EFEKTIF (B-215): kunci baru bila
		// Configure membawanya, jika tidak kunci tersimpan produk —
		// field kosong pada permintaan berarti "tidak diubah", jadi
		// nilai di berkas tidak pernah dihapus selama produk masih
		// menyimpan kunci untuk tool ini. Berkas 0600 karena bisa
		// memuat key itu.
		claudeDir := filepath.Join(homeDir, ".claude")
		_ = os.MkdirAll(claudeDir, 0755)
		settingsPath := filepath.Join(claudeDir, "settings.json")
		claudeURL := strings.TrimSuffix(gwURL, "/v1")
		// Berkas yang ada tetapi tidak terurai membatalkan Configure
		// (B-217), bukan ditimpa: isinya setelan pengguna yang tidak
		// kami kelola.
		cfg, err := muatKonfigJSON(settingsPath, false)
		if err != nil {
			return err
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
		// Tulis ~/.aider.conf.yml dengan MERGE per-baris (B-216):
		// hanya dua kunci skalar tingkat-atas yang dikelola Route-X
		// (openai-api-base + model), baris lain milik pengguna —
		// kunci provider lain (openai-api-key, anthropic-api-key),
		// opsi model/git/editor, komentar — dipertahankan apa
		// adanya. Versi lama menulis ulang berkas dari nol sehingga
		// seluruh setelan Aider lain hilang permanen. Izin mengikuti
		// berkas lama bila ada (berkas ini lazim memuat kunci).
		// Nama model WAJIB berprefiks provider LiteLLM `openai/`
		// (B-098) — sama seperti AIDER_MODEL di env.
		aiderConfigPath := filepath.Join(homeDir, ".aider.conf.yml")
		var existingAider string
		if data, err := os.ReadFile(aiderConfigPath); err == nil {
			existingAider = string(data)
		}
		content := perbaruiYAMLTopLevel(existingAider, [][2]string{
			{"openai-api-base", gwURL},
			{"model", modelAider(target)},
		})
		if err := tulisAtomik(aiderConfigPath, []byte(content), permBerkasAtau(aiderConfigPath, 0644)); err != nil {
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
		// Izin mengikuti berkas eksisting (B-220): penulisan ulang tidak
		// boleh melonggarkan berkas 0600 milik pengguna menjadi 0644;
		// 0644 hanya bawaan untuk berkas baru.
		if err := tulisAtomik(goosePath, []byte(content), permBerkasAtau(goosePath, 0644)); err != nil {
			return fmt.Errorf("menulis %s: %w", goosePath, err)
		}

	case "opencode":
		// Tulis atau perbarui ~/.config/opencode/opencode.json
		opencodeDir := filepath.Join(homeDir, ".config", "opencode")
		_ = os.MkdirAll(opencodeDir, 0755)
		configPath := filepath.Join(opencodeDir, "opencode.json")
		// Format resmi OpenCode adalah JSONC: penguraian toleran wajib
		// di sini (B-217), karena berkas JSONC yang sah menurut tool-nya
		// selalu gagal di encoding/json dan dulu selalu berakhir ditimpa.
		cfg, err := muatKonfigJSON(configPath, true)
		if err != nil {
			return err
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
		opts, ok := routexProv["options"].(map[string]any)
		if !ok || opts == nil {
			// B-221: berkas warisan/buatan tangan bisa memuat provider
			// routex TANPA sub-objek options; dulu cabang ini senyap
			// melewatkan baseURL/apiKey sementara models dan model
			// tetap diperbarui — provider menunjuk URL lama tetapi
			// Configure melaporkan sukses. Buat sub-objeknya.
			opts = make(map[string]any)
			routexProv["options"] = opts
		}
		opts["baseURL"] = gwURL
		// apiKey di sini adalah kunci EFEKTIF (B-215): kunci baru
		// bila Configure membawanya, jika tidak kunci tersimpan
		// produk diteruskan pemanggil. Field kosong berarti
		// "tidak diubah", jadi konfigurasi ulang tanpa kunci
		// TIDAK menghapus apiKey selama produk masih menyimpan
		// kunci untuk tool ini.
		if apiKey != "" {
			opts["apiKey"] = apiKey
		} else {
			delete(opts, "apiKey")
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

	case "codex":
		// Tulis ~/.codex/config.toml sesuai referensi konfigurasi
		// resmi Codex: kunci root model_provider/model di preamble,
		// lalu seksi [model_providers.routex]. base_url memakai
		// gwURL apa adanya (berakhiran /v1) karena Codex memanggil
		// POST {base_url}/responses. env_key menunjuk OPENAI_API_KEY
		// — nama yang sama dengan yang ditulis produk ke key.env
		// untuk tool ini. Kunci root dan seksi lain milik pengguna
		// dipertahankan.
		codexDir := filepath.Join(homeDir, ".codex")
		_ = os.MkdirAll(codexDir, 0755)
		codexPath := filepath.Join(codexDir, "config.toml")
		var existingCodex string
		if data, err := os.ReadFile(codexPath); err == nil {
			existingCodex = string(data)
		}
		kontenCodex := perbaruiPreambleTOML(existingCodex, [][2]string{
			{"model_provider", "routex"},
			{"model", target},
		})
		// B-222: gabung PER-KUNCI, bukan ganti seluruh seksi — kunci
		// lain yang pengguna taruh di [model_providers.routex]
		// (mis. request_max_retries) adalah konfigurasi sah Codex
		// dan tidak boleh terhapus setiap Configure.
		kontenCodex = perbaruiKunciSeksiTOML(kontenCodex, "model_providers.routex", [][2]string{
			{"name", "Route-X"},
			{"base_url", gwURL},
			{"env_key", "OPENAI_API_KEY"},
			{"wire_api", "responses"},
		})
		if err := tulisAtomik(codexPath, []byte(kontenCodex), permBerkasAtau(codexPath, 0644)); err != nil {
			return fmt.Errorf("menulis %s: %w", codexPath, err)
		}

	case "grok-build":
		// Tulis ~/.grok/config.toml sesuai user guide resmi Grok
		// Build (model kustom): [models] default menunjuk entri
		// [model.routex] yang memuat model, name, base_url, env_key.
		// Kunci [models] lain milik pengguna dipertahankan.
		grokDir := filepath.Join(homeDir, ".grok")
		_ = os.MkdirAll(grokDir, 0755)
		grokPath := filepath.Join(grokDir, "config.toml")
		var existingGrok string
		if data, err := os.ReadFile(grokPath); err == nil {
			existingGrok = string(data)
		}
		kontenGrok := perbaruiKunciSeksiTOML(existingGrok, "models", [][2]string{{"default", "routex"}})
		// B-222: gabung PER-KUNCI seperti codex di atas — kunci lain
		// pengguna di [model.routex] dipertahankan.
		kontenGrok = perbaruiKunciSeksiTOML(kontenGrok, "model.routex", [][2]string{
			{"model", target},
			{"name", "Route-X"},
			{"base_url", gwURL},
			{"env_key", "XAI_API_KEY"},
		})
		if err := tulisAtomik(grokPath, []byte(kontenGrok), permBerkasAtau(grokPath, 0644)); err != nil {
			return fmt.Errorf("menulis %s: %w", grokPath, err)
		}

	case "qwen":
		// Tulis ~/.qwen/settings.json sesuai dokumentasi resmi Qwen
		// Code: modelProviders.openai memuat entri provider (entri
		// ber-id sama diganti, entri lain dipertahankan),
		// security.auth.selectedType "openai", dan model.name.
		// envKey menunjuk OPENAI_API_KEY yang memang diekspor produk
		// di key.env untuk tool ini.
		qwenDir := filepath.Join(homeDir, ".qwen")
		_ = os.MkdirAll(qwenDir, 0755)
		qwenPath := filepath.Join(qwenDir, "settings.json")
		qcfg, err := muatKonfigJSON(qwenPath, false)
		if err != nil {
			return err
		}
		providersQ, ok := qcfg["modelProviders"].(map[string]any)
		if !ok || providersQ == nil {
			providersQ = make(map[string]any)
			qcfg["modelProviders"] = providersQ
		}
		var openaiList []any
		if lama, ok := providersQ["openai"].([]any); ok {
			for _, e := range lama {
				if mEntri, ok := e.(map[string]any); ok {
					if idEntri, _ := mEntri["id"].(string); idEntri == target {
						continue
					}
				}
				openaiList = append(openaiList, e)
			}
		}
		openaiList = append(openaiList, map[string]any{
			"id": target, "name": target, "baseUrl": gwURL, "envKey": "OPENAI_API_KEY",
		})
		providersQ["openai"] = openaiList
		securityQ, ok := qcfg["security"].(map[string]any)
		if !ok || securityQ == nil {
			securityQ = make(map[string]any)
			qcfg["security"] = securityQ
		}
		authQ, ok := securityQ["auth"].(map[string]any)
		if !ok || authQ == nil {
			authQ = make(map[string]any)
			securityQ["auth"] = authQ
		}
		authQ["selectedType"] = "openai"
		modelQ, ok := qcfg["model"].(map[string]any)
		if !ok || modelQ == nil {
			modelQ = make(map[string]any)
			qcfg["model"] = modelQ
		}
		modelQ["name"] = target
		dataQ, err := json.MarshalIndent(qcfg, "", "  ")
		if err != nil {
			return fmt.Errorf("mengkodekan settings.json Qwen: %w", err)
		}
		if err := tulisAtomik(qwenPath, dataQ, permBerkasAtau(qwenPath, 0644)); err != nil {
			return fmt.Errorf("menulis %s: %w", qwenPath, err)
		}

	case "crush":
		// Tulis ~/.config/crush/crush.json sesuai dokumentasi
		// konfigurasi Crush: providers.routex bertipe openai-compat
		// dengan base_url + api_key, models milik provider memuat
		// model target, dan models.large/small tingkat atas menunjuk
		// provider itu. Kunci hidup di berkas ini (tidak ada jalur env
		// per-tool), jadi izin 0600. apiKey di sini adalah kunci
		// EFEKTIF (B-215): konfigurasi ulang tanpa kunci baru
		// meneruskan kunci tersimpan produk, BUKAN menghapus api_key
		// — field kosong berarti "tidak diubah".
		crushDir := filepath.Join(homeDir, ".config", "crush")
		_ = os.MkdirAll(crushDir, 0755)
		crushPath := filepath.Join(crushDir, "crush.json")
		ccfg, err := muatKonfigJSON(crushPath, false)
		if err != nil {
			return err
		}
		if _, ok := ccfg["$schema"]; !ok {
			ccfg["$schema"] = "https://charm.land/crush.json"
		}
		providersC, ok := ccfg["providers"].(map[string]any)
		if !ok || providersC == nil {
			providersC = make(map[string]any)
			ccfg["providers"] = providersC
		}
		routexC, ok := providersC["routex"].(map[string]any)
		if !ok || routexC == nil {
			routexC = make(map[string]any)
			providersC["routex"] = routexC
		}
		routexC["name"] = "Route-X"
		routexC["type"] = "openai-compat"
		routexC["base_url"] = gwURL
		if apiKey != "" {
			routexC["api_key"] = apiKey
		} else {
			delete(routexC, "api_key")
		}
		var modelsC []any
		if lama, ok := routexC["models"].([]any); ok {
			for _, e := range lama {
				if mEntri, ok := e.(map[string]any); ok {
					if idEntri, _ := mEntri["id"].(string); idEntri == target {
						continue
					}
				}
				modelsC = append(modelsC, e)
			}
		}
		modelsC = append(modelsC, map[string]any{"id": target, "name": target})
		routexC["models"] = modelsC
		modelsTop, ok := ccfg["models"].(map[string]any)
		if !ok || modelsTop == nil {
			modelsTop = make(map[string]any)
			ccfg["models"] = modelsTop
		}
		modelsTop["large"] = map[string]any{"provider": "routex", "model": target}
		modelsTop["small"] = map[string]any{"provider": "routex", "model": target}
		dataC, err := json.MarshalIndent(ccfg, "", "  ")
		if err != nil {
			return fmt.Errorf("mengkodekan crush.json: %w", err)
		}
		if err := tulisAtomik(crushPath, dataC, 0600); err != nil {
			return fmt.Errorf("menulis %s: %w", crushPath, err)
		}

	case "hermes":
		// Tulis blok model: di ~/.hermes/config.yaml sesuai
		// dokumentasi resmi Hermes (Configuring Models): provider
		// "custom", default = model target, base_url = gwURL,
		// api_mode chat_completions. Seksi lain di config.yaml
		// dipertahankan; hanya blok model yang diganti. Kunci
		// ditanam sebagai api_key di blok model itu sendiri
		// (B-164): jalur chat Hermes untuk provider kustom membaca
		// api_key dari config, BUKAN dari OPENAI_API_KEY di .env —
		// penanaman .env saja terbukti membuat chat Hermes gagal
		// autentikasi. Karena config.yaml kini memuat kunci, berkas
		// selalu ditulis 0600 (bukan izin lama/0644). Kunci juga
		// tetap digabung ke ~/.hermes/.env (0600) bila Configure
		// membawa kunci; tanpa kunci baru berkas .env tidak
		// disentuh.
		hermesDir := filepath.Join(homeDir, ".hermes")
		_ = os.MkdirAll(hermesDir, 0755)
		hermesPath := filepath.Join(hermesDir, "config.yaml")
		var existingHermes string
		if data, err := os.ReadFile(hermesPath); err == nil {
			existingHermes = string(data)
		}
		kontenHermes := susunBlokModelHermes(existingHermes, target, gwURL, apiKey)
		if err := tulisAtomik(hermesPath, []byte(kontenHermes), 0600); err != nil {
			return fmt.Errorf("menulis %s: %w", hermesPath, err)
		}
		if apiKey != "" {
			envPath := filepath.Join(hermesDir, ".env")
			var existingEnv string
			if data, err := os.ReadFile(envPath); err == nil {
				existingEnv = string(data)
			}
			if err := tulisAtomik(envPath, []byte(perbaruiBarisEnv(existingEnv, "OPENAI_API_KEY", apiKey)), 0600); err != nil {
				return fmt.Errorf("menulis %s: %w", envPath, err)
			}
		}

	case "cline":
		// Cline CLI menyimpan kredensial di store-nya sendiri dan
		// skema berkasnya spesifik-versi, jadi Configure menjalankan
		// perintah resmi CLI itu sendiri (dokumentasi CLI Cline):
		// `cline auth -p openai -k <key> -b <url> -m <model>`.
		// Bila biner tidak terpasang, Configure gagal jujur — tidak
		// ada jalur berkas yang sah untuk ditulis.
		binCline := m.findBinary(tool.BinaryNames, homeDir)
		if binCline == "" {
			return fmt.Errorf("biner cline tidak ditemukan: Configure Cline menjalankan perintah resmi `cline auth` dari CLI itu sendiri, jadi Cline CLI harus terpasang lebih dulu")
		}
		argsCline := []string{"auth", "-p", "openai", "-b", gwURL, "-m", target}
		if apiKey != "" {
			argsCline = append(argsCline, "-k", apiKey)
		}
		ctxCline, batalCline := context.WithTimeout(ctx, 30*time.Second)
		defer batalCline()
		cmdCline := exec.CommandContext(ctxCline, binCline, argsCline...)
		var keluaranCline bytes.Buffer
		cmdCline.Stdout = &keluaranCline
		cmdCline.Stderr = &keluaranCline
		if err := cmdCline.Run(); err != nil {
			ringkas := strings.TrimSpace(keluaranCline.String())
			if apiKey != "" {
				ringkas = strings.ReplaceAll(ringkas, apiKey, "***")
			}
			if len(ringkas) > 300 {
				ringkas = ringkas[:300]
			}
			return fmt.Errorf("menjalankan cline auth: %w: %s", err, ringkas)
		}
	}

	return nil
}

// permBerkasAtau mengembalikan izin berkas yang sudah ada (supaya
// penulisan ulang tidak melonggarkan/mengetatkan izin pilihan
// pengguna), atau izin bawaan bila berkas belum ada.
func permBerkasAtau(path string, bawaan os.FileMode) os.FileMode {
	if fi, err := os.Stat(path); err == nil {
		return fi.Mode().Perm()
	}
	return bawaan
}

// tomlKutip mengutip nilai sebagai string dasar TOML.
func tomlKutip(v string) string {
	return strconv.Quote(v)
}

// terapkanPasanganTOML mengganti baris pasangan kelolaan di dalam
// daftar baris (preamble atau isi seksi), mempertahankan baris lain,
// dan menambahkan pasangan yang belum ada di akhir daftar.
func terapkanPasanganTOML(baris []string, pairs [][2]string) []string {
	nilai := make(map[string]string, len(pairs))
	for _, p := range pairs {
		nilai[p[0]] = p[1]
	}
	tertulis := map[string]bool{}
	hasil := make([]string, 0, len(baris)+len(pairs))
	for _, l := range baris {
		kunci := ""
		if eq := strings.Index(l, "="); eq > 0 {
			if _, ok := nilai[strings.TrimSpace(l[:eq])]; ok {
				kunci = strings.TrimSpace(l[:eq])
			}
		}
		if kunci == "" {
			hasil = append(hasil, l)
			continue
		}
		if tertulis[kunci] {
			continue // duplikat kunci kelolaan dibuang
		}
		tertulis[kunci] = true
		hasil = append(hasil, kunci+" = "+tomlKutip(nilai[kunci]))
	}
	for _, p := range pairs {
		if !tertulis[p[0]] {
			hasil = append(hasil, p[0]+" = "+tomlKutip(p[1]))
		}
	}
	return hasil
}

// perbaruiPreambleTOML memperbarui kunci tingkat-atas (sebelum seksi
// pertama) dokumen TOML — tempat kunci root seperti model_provider
// dan model milik Codex sah berada (kunci root SESUDAH seksi bukan
// lagi tingkat-atas dalam TOML).
func perbaruiPreambleTOML(existing string, pairs [][2]string) string {
	baris := strings.Split(existing, "\n")
	idx := len(baris)
	for i, l := range baris {
		if strings.HasPrefix(strings.TrimSpace(l), "[") {
			idx = i
			break
		}
	}
	hasil := append(terapkanPasanganTOML(baris[:idx], pairs), baris[idx:]...)
	keluaran := strings.Join(hasil, "\n")
	if !strings.HasSuffix(keluaran, "\n") {
		keluaran += "\n"
	}
	return keluaran
}

// namaSeksiTOML mengekstrak nama seksi dari sebuah baris header TOML
// secara TOLERAN (B-277): spasi di dalam kurung (`[ model.routex ]`),
// komentar ekor sesudah kurung tutup (`[model.routex] # catatan`), dan
// BOM di awal berkas diterima — semuanya bentuk sah yang ditulis
// editor. Pengenalan exact-match lama membuat penulis menambahkan
// seksi KEDUA bernama sama (TOML tak sah) alih-alih memperbarui yang
// ada. Mengembalikan "" bila baris bukan header seksi bernama.
// Header tabel-berganda `[[...]]` sengaja tidak dikenali sebagai
// seksi biasa.
func namaSeksiTOML(baris string) string {
	l := strings.TrimSpace(strings.TrimPrefix(baris, "\ufeff"))
	if !strings.HasPrefix(l, "[") || strings.HasPrefix(l, "[[") {
		return ""
	}
	tutup := strings.Index(l, "]")
	if tutup < 0 {
		return ""
	}
	sisa := strings.TrimSpace(l[tutup+1:])
	if sisa != "" && !strings.HasPrefix(sisa, "#") {
		return ""
	}
	return strings.TrimSpace(l[1:tutup])
}

// rentangSeksiTOML mencari rentang isi seksi [nama] (indeks baris isi
// awal dan akhir eksklusif) di dokumen yang sudah dipecah per baris.
// ok=false bila seksi tidak ada.
func rentangSeksiTOML(baris []string, nama string) (awal, akhir int, ok bool) {
	for i, l := range baris {
		if namaSeksiTOML(l) != nama {
			continue
		}
		akhir = len(baris)
		for j := i + 1; j < len(baris); j++ {
			if strings.HasPrefix(strings.TrimSpace(strings.TrimPrefix(baris[j], "\ufeff")), "[") {
				akhir = j
				break
			}
		}
		return i + 1, akhir, true
	}
	return 0, 0, false
}

// perbaruiKunciSeksiTOML memperbarui kunci di dalam seksi TOML yang
// sudah ada tanpa membuang kunci lain di seksi itu; seksi dibuat di
// akhir dokumen bila belum ada.
func perbaruiKunciSeksiTOML(existing, seksi string, pairs [][2]string) string {
	baris := strings.Split(existing, "\n")
	awal, akhir, ok := rentangSeksiTOML(baris, seksi)
	if !ok {
		keluaran := existing
		if keluaran != "" && !strings.HasSuffix(keluaran, "\n") {
			keluaran += "\n"
		}
		var b strings.Builder
		b.WriteString(keluaran)
		if keluaran != "" {
			b.WriteString("\n")
		}
		b.WriteString("[" + seksi + "]\n")
		for _, p := range pairs {
			b.WriteString(p[0] + " = " + tomlKutip(p[1]) + "\n")
		}
		return b.String()
	}
	isi := terapkanPasanganTOML(baris[awal:akhir], pairs)
	hasil := make([]string, 0, len(baris)+len(pairs))
	hasil = append(hasil, baris[:awal]...)
	hasil = append(hasil, isi...)
	hasil = append(hasil, baris[akhir:]...)
	keluaran := strings.Join(hasil, "\n")
	if !strings.HasSuffix(keluaran, "\n") {
		keluaran += "\n"
	}
	return keluaran
}

// pecahHeaderModelHermes mengenali baris header blok `model:`
// tingkat-atas secara TOLERAN (B-277): spasi sebelum titik dua
// (`model :`) dan BOM di awal berkas diterima. Mengembalikan sisa
// teks sesudah titik dua apa adanya ("" untuk peta murni, "# ..."
// untuk header berkomentar, atau nilai skalar) dan ok=false bila
// baris bukan header `model:` tingkat-atas — baris menjorok tidak
// pernah dianggap header.
func pecahHeaderModelHermes(l string) (sisa string, ok bool) {
	teks := strings.TrimPrefix(l, "\ufeff")
	if strings.HasPrefix(teks, " ") || strings.HasPrefix(teks, "\t") {
		return "", false
	}
	idx := strings.Index(teks, ":")
	if idx < 0 || strings.TrimSpace(teks[:idx]) != "model" {
		return "", false
	}
	return strings.TrimSpace(teks[idx+1:]), true
}

// susunBlokModelHermes memperbarui blok `model:` tingkat-atas di
// config.yaml Hermes ke setelan provider kustom Route-X dengan
// PENGGABUNGAN PER-KUNCI (B-222): sub-kunci terkelola
// (provider/default/base_url/api_mode, dan api_key bila kunci
// efektif ada) ditulis ulang dari nilai Route-X, sedangkan sub-kunci
// LAIN milik pengguna di bawah `model:` dipertahankan apa adanya —
// versi lama membuang seluruh baris menjorok di blok itu setiap
// Configure. Seksi lain di luar blok tidak disentuh. Bentuk skalar
// warisan (`model: "..."`) diganti menjadi blok peta; blok
// ditambahkan di akhir bila belum ada. apiKey non-kosong ditanam
// sebagai baris api_key di blok (B-164); apiKey yang diterima
// penulis adalah kunci EFEKTIF (B-215) — kunci baru atau kunci
// tersimpan — jadi blok hanya ditulis tanpa kunci bila produk
// memang tidak menyimpan kunci apa pun untuk tool ini; apiKey
// kosong berarti "tidak diubah", sehingga baris api_key lama di
// blok PERTAHANKAN, tidak dihapus.
func susunBlokModelHermes(existing, target, gwURL, apiKey string) string {
	blok := []string{
		"model:",
		"  provider: custom",
		"  default: " + yamlKutip(target),
		"  base_url: " + yamlKutip(gwURL),
		"  api_mode: chat_completions",
	}
	if apiKey != "" {
		blok = append(blok, "  api_key: "+yamlKutip(apiKey))
	}
	// Sub-kunci yang ditulis ulang blok di atas; kemunculan lamanya
	// disaring. api_key hanya terkelola bila kunci efektif ada —
	// bila kosong, baris lama dipertahankan (lihat kop fungsi).
	terkelola := map[string]bool{
		"provider": true, "default": true, "base_url": true,
		"api_mode": true, "api_key": apiKey != "",
	}
	baris := strings.Split(existing, "\n")
	for i, l := range baris {
		sisa, adalah := pecahHeaderModelHermes(l)
		if !adalah {
			continue
		}
		// B-277: sisa berupa komentar (`model: # catatan`) adalah
		// header PETA, bukan skalar — sub-baris menjorok sesudahnya
		// adalah isi blok lama yang harus digabung per-kunci di
		// bawah. Versi lama memperlakukannya sebagai skalar dan hanya
		// mengganti baris header, sehingga sub-kunci terkelola lama
		// menempel sesudah blok baru dan menang last-wins sementara
		// Configure melaporkan sukses.
		if sisa != "" && !strings.HasPrefix(sisa, "#") {
			// Bentuk skalar satu baris: ganti baris itu saja.
			hasil := make([]string, 0, len(baris)+len(blok))
			hasil = append(hasil, baris[:i]...)
			hasil = append(hasil, blok...)
			hasil = append(hasil, baris[i+1:]...)
			return gabungBarisYAML(hasil)
		}
		// Bentuk peta: isi blok adalah baris kosong/berindentasi
		// sesudahnya, sampai kunci tingkat-atas berikutnya.
		j := i + 1
		for j < len(baris) && (baris[j] == "" || strings.HasPrefix(baris[j], " ") || strings.HasPrefix(baris[j], "\t")) {
			j++
		}
		// B-222: saring sub-baris blok lama — yang terkelola dibuang
		// (ditulis ulang dari blok baru), yang lain dipertahankan
		// apa adanya sesudah baris terkelola. Baris kosong di EKOR
		// segmen adalah pemisah ke seksi berikutnya, bukan isi blok:
		// ia diregenerasi satu kali di bawah, jadi dipangkas di sini
		// agar penggabungan idempoten (pemindaian blok menelan baris
		// kosong itu sebagai bagian segmen pada terapan berikutnya).
		var dipertahankan []string
		for _, sub := range baris[i+1 : j] {
			if kunci := kunciSubBarisYAML(sub); kunci != "" && terkelola[kunci] {
				continue
			}
			dipertahankan = append(dipertahankan, sub)
		}
		for len(dipertahankan) > 0 && dipertahankan[len(dipertahankan)-1] == "" {
			dipertahankan = dipertahankan[:len(dipertahankan)-1]
		}
		hasil := make([]string, 0, len(baris)+len(blok))
		hasil = append(hasil, baris[:i]...)
		hasil = append(hasil, blok...)
		hasil = append(hasil, dipertahankan...)
		if j < len(baris) {
			hasil = append(hasil, "")
		}
		hasil = append(hasil, baris[j:]...)
		return gabungBarisYAML(hasil)
	}
	keluaran := existing
	if keluaran != "" && !strings.HasSuffix(keluaran, "\n") {
		keluaran += "\n"
	}
	if keluaran != "" {
		keluaran += "\n"
	}
	return keluaran + strings.Join(blok, "\n") + "\n"
}

// kunciSubBarisYAML mengekstrak nama kunci dari baris sub-entri blok
// YAML berindentasi tepat satu tingkat ("  nama: nilai" → "nama").
// Baris kosong, komentar, item daftar, kelanjutan nilai berindentasi
// lebih dalam, dan baris tanpa pola kunci mengembalikan "" — semua
// itu selalu dipertahankan apa adanya oleh penggabungan B-222.
func kunciSubBarisYAML(baris string) string {
	satuTingkat := (strings.HasPrefix(baris, "  ") && !strings.HasPrefix(baris, "   ")) ||
		(strings.HasPrefix(baris, "\t") && !strings.HasPrefix(baris, "\t\t"))
	if !satuTingkat {
		return ""
	}
	trim := strings.TrimSpace(baris)
	if trim == "" || strings.HasPrefix(trim, "#") || strings.HasPrefix(trim, "-") {
		return ""
	}
	idx := strings.Index(trim, ":")
	if idx <= 0 {
		return ""
	}
	return strings.TrimSpace(trim[:idx])
}

// gabungBarisYAML menyatukan baris dan memastikan newline penutup.
func gabungBarisYAML(baris []string) string {
	keluaran := strings.Join(baris, "\n")
	if !strings.HasSuffix(keluaran, "\n") {
		keluaran += "\n"
	}
	return keluaran
}

// perbaruiBarisEnv menetapkan satu variabel di konten berkas .env
// (baris NAMA=nilai), mengganti baris lama atau menambahkan di akhir;
// variabel lain dipertahankan.
func perbaruiBarisEnv(existing, nama, nilai string) string {
	baris := strings.Split(existing, "\n")
	tertulis := false
	for i, l := range baris {
		if strings.HasPrefix(l, nama+"=") {
			if tertulis {
				baris[i] = ""
				continue
			}
			baris[i] = nama + "=" + nilai
			tertulis = true
		}
	}
	hasil := make([]string, 0, len(baris)+1)
	for _, l := range baris {
		if l == "" && tertulis {
			continue
		}
		hasil = append(hasil, l)
	}
	keluaran := strings.Join(hasil, "\n")
	if !tertulis {
		if keluaran != "" && !strings.HasSuffix(keluaran, "\n") {
			keluaran += "\n"
		}
		keluaran += nama + "=" + nilai + "\n"
		return keluaran
	}
	if !strings.HasSuffix(keluaran, "\n") {
		keluaran += "\n"
	}
	return keluaran
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
// tidak dibaca Goose). Semuanya tetap masuk himpunan
// redaksi/pembersihan supaya skrip warisan yang masih memuatnya
// dibersihkan, bukan bocor. Nama khas tool yang dicabut pada
// perombakan registri Tahap 1 (opencommit, cody, cursor-agent,
// plandex, ollama, continue, sgpt, dan kawanannya) ikut didaftar di
// sini dengan alasan yang sama.
var namaVarKunciWarisan = []string{"GOOSE_API_KEY", "CONTINUE_API_KEY", "OCO_OPENAI_API_KEY", "OCO_API_KEY", "GITHUB_COPILOT_API_KEY", "CODECOMPANION_API_KEY", "SRC_ACCESS_TOKEN", "CURSOR_API_KEY", "PLANDEX_OPENAI_API_KEY", "OLLAMA_API_KEY"}

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
