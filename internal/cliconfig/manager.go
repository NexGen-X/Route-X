package cliconfig

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"log/slog"
	"net/url"
	"os"
	"os/exec"
	"path/filepath"
	"reflect"
	"sort"
	"strconv"
	"strings"
	"sync"
	"time"
	"unicode/utf8"

	"github.com/NexGen-X/Route-X/internal/database/repo/identity"
	"github.com/NexGen-X/Route-X/internal/security"
	toml "github.com/pelletier/go-toml/v2"
	yaml "go.yaml.in/yaml/v3"
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
// lompatanSymlink adalah satu tautan dalam rantai symlink: jalur
// tautannya sendiri dan teks target mentah yang tertulis di
// dalamnya (persis seperti dikembalikan Readlink, bisa relatif).
type lompatanSymlink struct {
	jalur  string
	target string
}

// selesaikanRantaiSymlink mengikuti rantai symlink dari jalur
// secara leksikal — tautan demi tautan, target relatif diartikan
// terhadap direktori tautannya — sampai komponen terakhir bukan
// symlink, dan mengembalikan daftar lompatannya beserta jalur
// target akhir. Dipakai untuk rantai yang TIDAK bisa diselesaikan
// EvalSymlinks karena putus di suatu lompatan (B-368): penyelesaian
// satu lompatan B-311 berhenti di tautan perantara, dan rename
// mendarat di atasnya menghancurkannya. Batas 40 lompatan meniru
// batas kernel; rantai yang melampauinya berhenti di titik itu.
//
// Nilai kembalian ketiga, tuntas, membedakan "berhenti karena
// memang selesai" dari "berhenti karena batas" (B-393): tanpa
// pembeda ini rantai SIKLUS (a→b→a) tampak persis seperti rantai
// sah yang panjang — penyelesaian berhenti di sebuah symlink yang
// masih hidup, dan pemanggil yang memercayainya menulis menembus
// atau menimpa tautan pengguna sambil melaporkan sukses. tuntas
// false berarti jalur akhir MASIH symlink: rantai tidak selesai,
// dan pemanggil wajib menolak bertindak atasnya.
func selesaikanRantaiSymlink(jalur string) ([]lompatanSymlink, string, bool) {
	var rantai []lompatanSymlink
	kini := jalur
	for len(rantai) < 40 {
		fi, err := os.Lstat(kini)
		if err != nil || fi.Mode()&os.ModeSymlink == 0 {
			return rantai, kini, true
		}
		mentah, err := os.Readlink(kini)
		if err != nil {
			return rantai, kini, true
		}
		rantai = append(rantai, lompatanSymlink{jalur: kini, target: mentah})
		berikut := mentah
		if !filepath.IsAbs(berikut) {
			berikut = filepath.Join(filepath.Dir(kini), berikut)
		}
		kini = filepath.Clean(berikut)
	}
	// Batas tercapai: tuntas hanya bila titik berhenti bukan lagi
	// symlink (rantai sah tepat 40 lompatan) — titik yang tidak
	// ada (ujung putus) juga tuntas, karena memang itulah target
	// leksikalnya (kasus B-368).
	fi, err := os.Lstat(kini)
	tuntas := err != nil || fi.Mode()&os.ModeSymlink == 0
	return rantai, kini, tuntas
}

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
	// satu filesystem).
	if resolved, err := filepath.EvalSymlinks(path); err == nil && resolved != path {
		path = resolved
	} else if fi, lerr := os.Lstat(path); lerr == nil && fi.Mode()&os.ModeSymlink != 0 {
		// B-311+B-368: symlink yang tidak terselesaikan EvalSymlinks
		// (putus di suatu lompatan) diselesaikan RANTAINYA secara
		// leksikal sampai target akhir — penyelesaian satu lompatan
		// lama berhenti di tautan perantara bila rantai punya dua
		// lompatan atau lebih, dan rename mendarat di atas tautan
		// perantara itu menghancurkannya menjadi berkas reguler.
		// Direktori induk target akhir dibuat, lalu tulisan mendarat
		// di sana; seluruh rantai tautan dipertahankan.
		rantai, akhir, tuntas := selesaikanRantaiSymlink(path)
		if len(rantai) == 0 {
			return fmt.Errorf("symlink %s tidak bisa diselesaikan", path)
		}
		// B-393: rantai yang tidak tuntas (siklus, atau lebih
		// panjang dari batas) TIDAK boleh ditulis menembusnya:
		// "akhir" masih sebuah symlink, dan rename ke sana
		// menghancurkan tautan pengguna sambil mengembalikan
		// sukses. Gagal eksplisit, tidak menulis apa pun.
		if !tuntas {
			return fmt.Errorf("rantai symlink %s tidak tuntas sesudah %d lompatan (kemungkinan siklus); menolak menulis", path, len(rantai))
		}
		if merr := os.MkdirAll(filepath.Dir(akhir), 0755); merr != nil {
			return merr
		}
		path = akhir
	}
	// B-484: tujuan yang SUDAH ADA dan bukan berkas reguler (FIFO,
	// socket, perangkat) ditolak bernama sebelum rename: rename
	// akan MENIMPA penghuni khususnya secara senyap dan Configure
	// melaporkan sukses atas konfigurasi yang tidak pernah bisa
	// dibaca tool dari FIFO/socket itu.
	if fi, lerr := os.Lstat(path); lerr == nil && !fi.Mode().IsRegular() {
		return fmt.Errorf("%s bukan berkas reguler (mode %s); menolak menimpa berkas khusus", path, fi.Mode().Type())
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
//
// bacaTeksKonfig membaca berkas konfigurasi yang MUNGKIN belum
// ada: belum ada → ("", nil). Berkas yang ADA tetapi bukan berkas
// reguler (FIFO/socket/perangkat) adalah galat bernama (B-484):
// os.ReadFile pada FIFO menggantung tanpa batas menunggu penulis,
// jadi keberadaannya tidak pernah boleh mencapai ReadFile. Galat
// baca lain juga dikembalikan — menelan galat baca lalu menimpa
// berkas dari nol adalah kelas B-217.
func bacaTeksKonfig(path string) (string, error) {
	fi, err := os.Stat(path)
	if errors.Is(err, os.ErrNotExist) {
		return "", nil
	}
	if err != nil {
		return "", fmt.Errorf("memeriksa %s: %w", path, err)
	}
	if !fi.Mode().IsRegular() {
		return "", fmt.Errorf("%s bukan berkas reguler (mode %s); Configure tidak membaca berkas khusus", path, fi.Mode().Type())
	}
	data, err := os.ReadFile(path)
	if errors.Is(err, os.ErrNotExist) {
		return "", nil
	}
	if err != nil {
		return "", fmt.Errorf("membaca %s: %w", path, err)
	}
	return string(data), nil
}

// errBerkasKhusus menandai penolakan membaca berkas yang ada tetapi
// bukan berkas reguler (B-494). Pemanggil membedakannya dari galat
// baca biasa: sebagian situs menelan galat baca biasa (berkas belum
// ada memang dilewati), tetapi berkas khusus yang ADA tidak boleh
// pernah dibaca senyap — membacanya menggantung (FIFO) dan
// melewatinya senyap mengklaim langkah yang tidak terjadi.
var errBerkasKhusus = errors.New("berkas khusus tidak dibaca")

// bacaReguler membaca berkas dengan penjaga yang sama seperti
// bacaTeksKonfig (B-484): Stat lebih dulu; berkas yang ada tetapi
// bukan reguler ditolak dengan errBerkasKhusus TANPA mencapai
// os.ReadFile — ReadFile pada FIFO menggantung tanpa batas menunggu
// penulis. Galat lain (termasuk belum ada) diteruskan apa adanya
// supaya semantik tiap pemanggil tidak berubah.
func bacaReguler(path string) ([]byte, error) {
	if fi, err := os.Stat(path); err == nil && !fi.Mode().IsRegular() {
		return nil, fmt.Errorf("%s bukan berkas reguler (mode %s): %w", path, fi.Mode().Type(), errBerkasKhusus)
	}
	return os.ReadFile(path)
}

func muatKonfigJSON(path string, toleranJSONC bool) (map[string]any, error) {
	// B-484: penjaga berkas khusus lebih dulu — ReadFile pada
	// FIFO menggantung; muatKonfigJSON adalah pintu baca keempat
	// tool JSON.
	if fi, serr := os.Stat(path); serr == nil && !fi.Mode().IsRegular() {
		return nil, fmt.Errorf("%s bukan berkas reguler (mode %s); Configure tidak membaca berkas khusus", path, fi.Mode().Type())
	}
	data, err := os.ReadFile(path)
	if errors.Is(err, os.ErrNotExist) {
		return make(map[string]any), nil
	}
	if err != nil {
		return nil, fmt.Errorf("membaca %s: %w", path, err)
	}
	// KEPUTUSAN SADAR (B-380): BOM UTF-8 di awal berkas dibuang,
	// disamakan dengan jalur TOML (B-310), YAML (B-344), dan .env
	// (B-374). BOM adalah artefak penyunting, bukan isi — encoding/
	// json menolaknya ("invalid character 'ï'"), sehingga berkas
	// JSON yang sah dari penyunting Windows menggagalkan Configure
	// keempat tool pemakai fungsi ini sementara berkas TOML/YAML
	// yang sama-sama ber-BOM diterima. Perbedaan per format itu
	// tidak punya alasan produk, jadi diseragamkan. Yang tetap
	// ditolak adalah berkas yang memang tidak terurai sesudah BOM
	// dibuang (perlindungan B-217 di bawah tidak berubah).
	data = bytes.TrimPrefix(data, []byte("\ufeff"))
	if toleranJSONC {
		data = uraiJSONCToleran(data)
	}
	cfg, err := uraiPetaJSON(data)
	if err != nil {
		return nil, fmt.Errorf("berkas konfigurasi %s tidak bisa diurai (%v); Configure dibatalkan agar setelan di dalamnya tidak tertimpa — perbaiki atau pindahkan berkas itu, lalu coba lagi", path, err)
	}
	return cfg, nil
}

// uraiPetaJSON mengurai dokumen JSON berbentuk objek dengan
// UseNumber (B-478): angka dipertahankan sebagai teks mentahnya
// (json.Number), BUKAN float64. Unmarshal baku mengubah
// 9007199254740993 menjadi 9007199254740992 secara senyap, dan
// penulisan ulang hasil gabungan lalu MENETAPKAN kerusakan itu ke
// berkas pengguna (id numerik besar, batas presisi) sementara
// Configure melaporkan sukses. Dengan Number, angka yang tidak
// dikelola Route-X berjalan pulang-pergi apa adanya. Muatan
// sesudah dokumen pertama ditolak, setara Unmarshal.
func uraiPetaJSON(data []byte) (map[string]any, error) {
	dec := json.NewDecoder(bytes.NewReader(data))
	dec.UseNumber()
	cfg := make(map[string]any)
	if err := dec.Decode(&cfg); err != nil {
		return nil, err
	}
	if _, err := dec.Token(); !errors.Is(err, io.EOF) {
		if err == nil {
			err = fmt.Errorf("muatan tambahan sesudah dokumen JSON")
		}
		return nil, err
	}
	if cfg == nil {
		cfg = make(map[string]any)
	}
	return cfg, nil
}

// verifikasiHasilJSON adalah gerbang hasil akhir jalur JSON
// (B-478), setara verifikasiHasilTOML/verifikasiHasilHermes:
// konten yang akan ditulis diurai ulang dengan UseNumber dan wajib
// SAMA persis secara semantik dengan peta hasil mutasi yang
// diniatkan. Perbedaan apa pun — angka besar yang bergeser, kunci
// yang hilang/berubah di perjalanan marshal — menggagalkan
// Configure secara jujur sebelum berkas ditulis.
func verifikasiHasilJSON(konten []byte, harapan map[string]any) error {
	terbaca, err := uraiPetaJSON(konten)
	if err != nil {
		return fmt.Errorf("hasil sunting JSON tidak terurai: %w", err)
	}
	if !reflect.DeepEqual(terbaca, harapan) {
		return fmt.Errorf("hasil sunting JSON tidak sama dengan konfigurasi yang diniatkan (kemungkinan angka presisi atau kunci berubah di perjalanan)")
	}
	return nil
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
	// B-443: APIKey divalidasi di pintu masuk dengan aturan yang
	// sama seperti Target. Versi lama tidak memvalidasinya sama
	// sekali, jadi kunci ber-newline baru ketahuan di tengah jalan
	// (shellQuote di susunKontenKeyEnv) — sesudah direktori .routex
	// diciptakan — dengan pesan galat yang buram.
	if strings.ContainsAny(p.APIKey, "\x00\r\n") {
		return nil, fmt.Errorf("API key memuat karakter terlarang")
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

	// Konfigurasi tersimpan sebelumnya dibaca utuh satu kali, untuk
	// dua keperluan: (a) mempertahankan kunci bila Configure tidak
	// membawa kunci baru (di bawah), dan (b) bahan kompensasi B-278
	// bila persistensi gagal sesudah efek eksternal diterapkan.
	var tersimpanLama *StoredConfig
	if m.settings != nil {
		if lama, errGet := m.settings.Get(ctx, settingKey); errGet == nil && len(lama.Value) > 0 {
			var sl StoredConfig
			if json.Unmarshal(lama.Value, &sl) == nil {
				tersimpanLama = &sl
			}
		}
	}

	// Pertahankan kunci tersimpan tool ini bila Configure tidak membawa
	// kunci baru: Put menimpa SELURUH StoredConfig, jadi tanpa langkah ini
	// konfigurasi ulang tanpa kunci diam-diam menghapus kunci tersimpan
	// tool itu sendiri (bagian dari audit3 V-LN-26). Tidak ada alur
	// "hapus kunci" yang disengaja — field kosong berarti "tidak diubah".
	if p.APIKey == "" && tersimpanLama != nil {
		stored.APIKeyEncrypted = tersimpanLama.APIKeyEncrypted
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
			// B-443: cabang ini dulu satu-satunya cabang kegagalan
			// pasca-jepretan yang kembali TANPA pulihkanSemua, jadi
			// direktori .routex ciptaan MkdirAll di atas bocor
			// sebagai residu kosong sesudah Configure yang "gagal
			// dan dipulihkan". Kini simetris dengan cabang lain.
			pulihkanSemua()
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
			// B-278: khusus Cline, efek Configure yang sesungguhnya
			// adalah store internal cline yang ditulis `cline auth`
			// oleh applyToolSpecificConfig — di luar jangkauan
			// jepretan berkas mana pun. Kompensasi best-effort:
			// jalankan ulang auth dengan konfigurasi tersimpan
			// sebelumnya supaya store cline kembali sepasang dengan
			// DB yang tetap lama. Ini BUKAN rollback sejati (proses
			// eksternalnya bisa gagal juga — kegagalannya dilaporkan
			// menyatu di error), dan bila belum pernah ada
			// konfigurasi tersimpan tidak ada keadaan lama untuk
			// dipulihkan: Configure pertama yang gagal Put tetap
			// meninggalkan store cline terkonfigurasi tanpa baris
			// DB — jendela sisa yang dinyatakan apa adanya di sini,
			// menyembuh sendiri pada Configure ulang yang berhasil.
			if p.ToolID == "cline" && tersimpanLama != nil {
				if errKomp := m.applyToolSpecificConfig(ctx, homeDir, toolDef, tersimpanLama.Mode, tersimpanLama.Target, tersimpanLama.GatewayURL, m.kunciTersimpan(*tersimpanLama, p.ToolID)); errKomp != nil {
					return nil, fmt.Errorf("menyimpan konfigurasi CLI %s ke database: %w (kompensasi store cline dengan konfigurasi lama juga gagal: %v)", p.ToolID, err, errKomp)
				}
			}
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
	// Keadaan symlink (B-333), terisi bila jalur adalah tautan saat
	// dijepret: teks target mentahnya, jalur target terselesaikan
	// secara leksikal, dan potret targetnya sendiri.
	tautan              bool
	targetTautan        string
	targetTerselesaikan string
	targetAda           bool
	targetIsi           []byte
	targetIzin          os.FileMode
	targetTakTersentuh  bool
	// rantai (B-368): SELURUH lompatan rantai symlink dari jalur ke
	// target akhir, bukan hanya lompatan pertama — pemulihan wajib
	// bisa membangun kembali setiap tautan perantara, bukan hanya
	// yang di titik masuk.
	rantai []lompatanSymlink
	// dirCiptaan (B-394): leluhur direktori berkas (atau target
	// tautannya) yang BELUM ada saat dijepret, terdalam → terluar.
	// B-381 hanya mencatatnya di cabang symlink; cabang polos —
	// jalur umum — tidak mencatat apa pun, jadi direktori yang
	// diciptakan Configure lewat MkdirAll di jalur-jalur tool
	// bertahan sesudah rollback. Pemulihan menghapus tiap entri
	// dengan os.Remove (hanya berhasil bila kosong — isi asing
	// mempertahankannya).
	dirCiptaan []string
}

// dirBelumAda mengembalikan leluhur direktori jalur yang belum
// ada, terdalam → terluar, berhenti di leluhur pertama yang ada.
func dirBelumAda(jalur string) []string {
	var out []string
	d := filepath.Dir(jalur)
	for {
		if _, err := os.Lstat(d); err == nil {
			return out
		}
		out = append(out, d)
		induk := filepath.Dir(d)
		if induk == d {
			return out
		}
		d = induk
	}
}

func jepretBerkasConfigure(jalur string) jepretanBerkas {
	// B-333: jalur dinilai dengan Lstat lebih dulu. os.Stat mengikuti
	// tautan, jadi symlink PUTUS terbaca "tidak ada" — padahal
	// Configure menulis menembus tautan itu (B-311 menciptakan
	// targetnya), dan rollback untuk "tidak ada" akan menghapus
	// TAUTAN pengguna, bukan berkas ciptaan Configure.
	lfi, lerr := os.Lstat(jalur)
	if lerr != nil {
		// B-394: berkas belum ada — catat leluhur direktorinya
		// yang juga belum ada, supaya pemulihan bisa membongkar
		// direktori ciptaan Configure di cabang polos juga.
		return jepretanBerkas{jalur: jalur, dirCiptaan: dirBelumAda(jalur)}
	}
	if lfi.Mode()&os.ModeSymlink != 0 {
		j := jepretanBerkas{jalur: jalur, ada: true, tautan: true}
		// B-368: jepret SELURUH rantai, bukan satu lompatan —
		// penyelesaian satu lompatan lama membuat target
		// "terselesaikan" berhenti di tautan perantara, sehingga
		// potret target dan pemulihannya menilai berkas yang salah.
		rantai, akhir, tuntas := selesaikanRantaiSymlink(jalur)
		if len(rantai) == 0 {
			j.takTersentuh = true
			return j
		}
		// B-393: rantai yang tidak tuntas (siklus) tidak bisa
		// dinilai keadaannya — os.Stat pada targetnya menjawab
		// ELOOP dan terbaca "target tidak ada", yang di pemulihan
		// berarti MENGHAPUS jalur itu (tautan pengguna sendiri).
		// Tandai tak tersentuh: Configure akan gagal eksplisit di
		// tulisAtomik, dan rollback tidak menyentuh berkas ini.
		if !tuntas {
			j.takTersentuh = true
			return j
		}
		j.dirCiptaan = dirBelumAda(akhir)
		j.rantai = rantai
		j.targetTautan = rantai[0].target
		j.targetTerselesaikan = akhir
		tfi, err := os.Stat(j.targetTerselesaikan)
		if err != nil {
			return j // tautan putus: target belum ada
		}
		// B-484: target non-reguler (direktori, FIFO, socket)
		// tidak pernah DIBACA: os.ReadFile pada FIFO menggantung
		// tanpa batas menunggu penulis. Tandai tak tersentuh —
		// tulisAtomik menolaknya bernama di jalur tulis.
		if !tfi.Mode().IsRegular() {
			j.targetTakTersentuh = true
			return j
		}
		isi, err := os.ReadFile(j.targetTerselesaikan)
		if err != nil {
			j.targetTakTersentuh = true
			return j
		}
		j.targetAda = true
		j.targetIsi = isi
		j.targetIzin = tfi.Mode().Perm()
		return j
	}
	j := jepretanBerkas{jalur: jalur, ada: true, izin: lfi.Mode().Perm()}
	// B-484: direktori dan berkas khusus (FIFO/socket/perangkat)
	// tidak dibaca — ReadFile pada FIFO menggantung tanpa batas.
	// Keduanya ditandai tak tersentuh; penolakan bernama terjadi
	// di tulisAtomik saat Configure benar-benar menulis.
	if !lfi.Mode().IsRegular() {
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
// dengan isi dan izin semula. Untuk jalur yang semula SYMLINK
// (B-333), yang dipulihkan adalah tautannya sendiri (dibuat ulang
// bila hilang/terganti) plus keadaan targetnya: target yang semula
// ada ditulis ulang isinya; target yang semula tidak ada — berkas
// yang diciptakan Configure menembus tautan — dihapus di jalurnya
// yang terselesaikan, BUKAN di jalur tautan. Galat pemulihan
// diabaikan per berkas (best effort): galat utama Configure sudah
// dikembalikan ke pemanggil, dan satu berkas yang gagal pulih tidak
// boleh menghentikan pemulihan berkas lain.
func (j jepretanBerkas) pulihkan() {
	if j.takTersentuh {
		return
	}
	if j.tautan {
		// B-368: pulihkan SETIAP lompatan rantai yang penghuninya
		// berubah dari jepretan (hilang, atau terganti berkas
		// reguler) — versi satu-lompatan hanya menjaga tautan titik
		// masuk dan membiarkan tautan perantara yang hancur tetap
		// hancur. Penghuni direktori sungguhan tidak pernah
		// disentuh.
		for _, l := range j.rantai {
			mentah, err := os.Readlink(l.jalur)
			if err == nil && mentah == l.target {
				continue
			}
			if fi, lerr := os.Lstat(l.jalur); lerr == nil && fi.IsDir() {
				continue
			}
			_ = os.Remove(l.jalur)
			_ = os.Symlink(l.target, l.jalur)
		}
		if j.targetTakTersentuh {
			return
		}
		if !j.targetAda {
			_ = os.Remove(j.targetTerselesaikan)
			// B-381+B-394: direktori leluhur yang lahir karena
			// Configure (MkdirAll di tulisAtomik untuk target
			// tautan putus) dihapus lagi — seluruh rantai leluhur
			// yang belum ada saat dijepret, bukan hanya induk
			// langsung. os.Remove hanya berhasil bila direktori
			// itu kosong, jadi isi asing apa pun mempertahankannya.
			for _, d := range j.dirCiptaan {
				_ = os.Remove(d)
			}
			return
		}
		_ = tulisAtomik(j.targetTerselesaikan, j.targetIsi, j.targetIzin)
		return
	}
	if !j.ada {
		_ = os.Remove(j.jalur)
		// B-394: bongkar juga direktori leluhur ciptaan Configure
		// di cabang polos (MkdirAll di jalur-jalur tool) — B-381
		// hanya melakukannya di cabang symlink, jadi jalur umum
		// meninggalkan direktori kosong asing sesudah rollback.
		for _, d := range j.dirCiptaan {
			_ = os.Remove(d)
		}
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
// jadi tidak mempunyai entri. Khusus cline, efek store-nya pada
// kegagalan Put ditangani kompensasi B-278 di Configure (auth ulang
// dengan konfigurasi lama), bukan oleh jepretan ini.
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
		oleh  string
	}
	vars := make(map[string]entriVar)
	set := func(nama, nilai string, pada time.Time, oleh string) {
		if nama == "" || nilai == "" {
			return
		}
		// B-320: pemenang ditentukan oleh (waktu, ID tool) secara
		// leksikografis, bukan urutan iterasi map. Pada timestamp
		// seri persis, ID tool terbesar menang di semua titik —
		// variabel bersama dan nama generik tidak bisa memilih
		// pemenang berbeda hanya karena urutan map berubah.
		if lama, ok := vars[nama]; !ok || pada.After(lama.pada) ||
			(pada.Equal(lama.pada) && oleh > lama.oleh) {
			vars[nama] = entriVar{nilai: nilai, pada: pada, oleh: oleh}
		}
	}

	var terbaru *kunciTersimpan
	terbaruID := ""
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
		if terbaru == nil || kt.diperbarui.After(terbaru.diperbarui) ||
			(kt.diperbarui.Equal(terbaru.diperbarui) && toolID > terbaruID) {
			salin := kt
			terbaru = &salin
			terbaruID = toolID
		}
		if def, ok := cariToolDef(toolID); ok {
			set(def.EnvVarAPIKey, kt.kunci, kt.diperbarui, toolID)
		}
	}
	if terbaru != nil {
		for _, nama := range namaVarKunciGenerik {
			set(nama, terbaru.kunci, terbaru.diperbarui, terbaruID)
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
	if strings.ContainsAny(v, " \t#\"'\\") || strings.Contains(v, ": ") || strings.HasSuffix(v, ":") || yamlPerluKutip(v) {
		q, err := json.Marshal(v)
		if err == nil {
			return string(q)
		}
	}
	return v
}

// yamlPerluKutip melaporkan apakah skalar polos v akan dibaca ulang
// pengurai YAML sebagai BUKAN string atau sebagai struktur (B-282):
// nilai seperti `123`/`true`/`null`/`on` berubah tipe (int/bool/
// null), `*jangkar` menjadi alias (galat urai bila jangkarnya tak
// terdefinisi), dan `[daftar]`/`{peta}` menjadi koleksi flow.
// Versi lama menulis semuanya polos sehingga nilai konfigurasi
// berubah makna diam-diam antar tulis-baca di goose/aider/hermes.
func yamlPerluKutip(v string) bool {
	// Indikator struktur/flow di awal skalar (alias, jangkar, tag,
	// blok skalar, koleksi flow, dsb.) — selalu kutip. Perbandingan
	// per-byte aman: seluruh indikator adalah ASCII, sedangkan byte
	// pembuka UTF-8 multibyte selalu >= 0x80.
	if strings.IndexByte("*&!|>%@`[]{},", v[0]) >= 0 {
		return true
	}
	// Indikator yang bermakna bila berdiri sendiri atau diikuti
	// spasi: item urutan, kunci pemetaan, kunci eksplisit.
	if v == "-" || v == "?" || v == ":" ||
		strings.HasPrefix(v, "- ") || strings.HasPrefix(v, "? ") || strings.HasPrefix(v, ": ") {
		return true
	}
	// Kata skalar non-string pada skema inti YAML (bool/null/
	// nilai istimewa), dalam varian huruf lazimnya.
	switch strings.ToLower(v) {
	case "true", "false", "yes", "no", "on", "off", "null", "~",
		".inf", "-.inf", "+.inf", ".nan":
		return true
	}
	return yamlAngkaAtauTanggal(v)
}

// yamlAngkaAtauTanggal melaporkan apakah v akan diurai YAML sebagai
// angka (utuh/desimal/eksponen/heksadesimal/oktal) atau timestamp —
// keduanya bukan string.
func yamlAngkaAtauTanggal(v string) bool {
	s := strings.TrimPrefix(strings.TrimPrefix(v, "+"), "-")
	if s == "" {
		return false
	}
	// Heksadesimal & oktal gaya inti YAML (0x…, 0o…).
	lower := strings.ToLower(s)
	if strings.HasPrefix(lower, "0x") || strings.HasPrefix(lower, "0o") {
		return true
	}
	// Timestamp ISO di awal nilai (mis. 2026-11-06 atau
	// 2026-11-06T07:00:00Z): digit-digit tanggal + pemisah '-'.
	if len(v) >= 10 && v[4] == '-' && v[7] == '-' &&
		isDigitByte(v[0]) && isDigitByte(v[1]) && isDigitByte(v[2]) && isDigitByte(v[3]) &&
		isDigitByte(v[5]) && isDigitByte(v[6]) && isDigitByte(v[8]) && isDigitByte(v[9]) {
		return true
	}
	// Angka murni/desimal/eksponen. ParseFloat menolak nama model
	// lazim ("gpt-4o", "3.5-turbo") sehingga nilai itu tetap polos.
	_, err := strconv.ParseFloat(v, 64)
	return err == nil
}

// isDigitByte melaporkan apakah b adalah digit ASCII.
func isDigitByte(b byte) bool { return b >= '0' && b <= '9' }

// perbaruiYAMLTopLevel memperbarui kunci skalar tingkat-atas (tanpa
// indentasi) di dokumen YAML yang sudah ada, mempertahankan baris lain
// apa adanya — config.yaml Goose bisa memuat blok lain milik pengguna
// (mis. extensions) yang tidak boleh dihancurkan Configure. Kunci yang
// dikelola muncul dua kali atau lebih disisakan satu (yang pertama,
// diperbarui); kunci yang belum ada ditambahkan di akhir.
// normalkanKunciYAML membuang pasangan kutip luar yang seimbang
// dari nama kunci YAML: parser YAML membaca "provider", 'provider',
// dan provider sebagai kunci yang SAMA, jadi perbandingan nama
// harus menormalkannya lebih dulu (B-372/B-373 — pasangan perbaikan
// TOML B-343). Kutip tunggal yang lolos di dalamnya ditulis ” oleh
// YAML dan dilipat kembali ke satu '. Nama tanpa kutip luar
// dikembalikan apa adanya.
func normalkanKunciYAML(nama string) string {
	if len(nama) >= 2 {
		if nama[0] == '"' && nama[len(nama)-1] == '"' {
			return nama[1 : len(nama)-1]
		}
		if nama[0] == '\'' && nama[len(nama)-1] == '\'' {
			return strings.ReplaceAll(nama[1:len(nama)-1], "''", "'")
		}
	}
	return nama
}

// barisPenandaAkhirDokumen melaporkan apakah baris ini penanda
// akhir dokumen YAML "..." pada tingkat atas: kolom 0 dan isinya
// persis "..." (diikuti akhir baris atau komentar). Penanda ini sah
// pada config satu dokumen — PyYAML menerimanya — dan penyunting
// Hermes sudah menanganinya dengan benar; penyunting bersama
// Goose/Aider disamakan di perbaruiYAMLTopLevel (B-523).
//
// B-535: sisa baris dipangkas juga dari "\r" — baris hasil pecahan
// "\n" atas berkas CRLF (bawaan penyunting Windows) menyisakan CR
// di ekornya, dan "...\r" dulu tidak dikenali sebagai penanda,
// sehingga config CRLF yang sah gagal di-Configure. Sapuan
// pencocokan baris lain di penyunting yang sama: ekstraksi kunci
// perbaruiYAMLTopLevel memakai TrimSpace (sudah membuang \r) atas
// potongan SEBELUM titik dua pertama, jadi akhir baris tidak
// pernah mencapai perbandingan kunci — fungsi ini satu-satunya
// permukaan yang peka CR.
func barisPenandaAkhirDokumen(line string) bool {
	_, ok := pecahPenandaAkhirDokumen(line)
	return ok
}

// pecahPenandaAkhirDokumen mengenali baris penanda akhir dokumen
// dan mengembalikan teks komentarnya ("" bila tak berkomentar):
// kolom 0, diawali "...", dan sisa sesudahnya — dipangkas spasi/
// tab/CR di kedua sisi — kosong atau diawali '#'. B-544: bentuk
// "...\t# selesai" (pemisah TAB antara penanda dan komentar)
// termasuk dikenali di sini — pemuat Hermes (PyYAML) menolak
// baris itu mentah-mentah (ScannerError, terbukti dengan pustaka
// sungguhan), jadi penyunting menormalkannya saat menulis (lihat
// normalkanBarisPenanda) alih-alih mewariskan berkas yang tidak
// terbaca itu apa adanya. Sisa bukan-komentar ("... abc") BUKAN
// penanda: sesudah "..." hanya komentar yang sah di baris itu.
func pecahPenandaAkhirDokumen(line string) (komentar string, ok bool) {
	if line == "" || line[0] == ' ' || line[0] == '\t' {
		return "", false
	}
	if !strings.HasPrefix(line, "...") {
		return "", false
	}
	sisa := strings.TrimRight(line[3:], " \t\r")
	if sisa == "" {
		return "", true
	}
	sisa = strings.TrimLeft(sisa, " \t")
	if strings.HasPrefix(sisa, "#") {
		return sisa, true
	}
	return "", false
}

// normalkanBarisPenanda menulis ulang baris penanda akhir dokumen
// ke bentuk kanonik "..." atau "... # komentar" (B-544): pemisah
// tab/rangkap di antara penanda dan komentarnya dilipat menjadi
// satu spasi, dan CR akhir berkas CRLF dipertahankan. B-559:
// normalisasi ini diterapkan pada SETIAP kemunculan penanda saat
// menulis, bukan hanya yang pertama — penanda kedua+ berbentuk
// tab/tanpa-spasi yang dulu diteruskan mentah ditolak PyYAML.
// Keputusan NORMALISASI (bukan penolakan): penanda itu sendiri dikenali
// maksudnya oleh penyunting dan bentuk kanoniknya diterima semua
// pemuat — sejajar preseden pembuangan BOM B-344; menolak akan
// menghukum berkas yang satu-satunya dosa adalah tab warisan
// penyunting lain. Toleransi pemuat Goose (serde_yaml) TIDAK
// diklaim di sini — normalisasi diterapkan seragam oleh
// penyunting bersama sebelum pemuat mana pun membaca hasilnya.
func normalkanBarisPenanda(line string) string {
	komentar, ok := pecahPenandaAkhirDokumen(line)
	if !ok {
		return line
	}
	hasil := "..."
	if komentar != "" {
		hasil += " " + komentar
	}
	if strings.HasSuffix(line, "\r") {
		hasil += "\r"
	}
	return hasil
}

// pelacakSkalarYAML melacak apakah posisi pemindaian sedang
// berada DI DALAM skalar terkutip multiline YAML (B-550 — analog
// pemindai TOML B-439, untuk penyunting baris bersama Goose/Aider
// dan penyusun blok Hermes). Skalar terkutip adalah satu-satunya
// nilai YAML yang isinya boleh memuat baris kolom-0 yang
// menyerupai kunci atau penanda dokumen: penyunting baris yang
// tidak melacaknya membaca baris isi itu sebagai struktur — kunci
// terkelola "ditemukan" DI DALAM nilai string orang, atau blok
// model Hermes "ditemukan" di tengah kutipan.
//
// Skalar blok (|/>) dilacak HANYA sebagai keadaan (B-560): isinya
// terikat indentasi dan kedua penyunting sudah menghormati
// indentasi, jadi baris isinya tidak pernah mencapai kolom 0
// sebagai kunci semu — alasan B-550 itu benar untuk
// isi-sebagai-struktur. Tetapi isi-sebagai-keadaan-pelacak tidak
// ditutupnya: kutip pada posisi pembuka di isi blok yang tak
// menutup pada baris yang sama dulu MEMBUKA skalar semu yang
// menelan struktur asli sesudah blok berakhir (B-560). Kini
// pelacak mengenali pembuka skalar blok dan menahan pemindaian
// kutip selama di dalam blok; baris isinya sendiri tetap bukan
// struktur (ia menjorok).
type pelacakSkalarYAML struct {
	kutip byte // 0 di luar skalar; '"' atau '\'' di dalam
	blok  int  // -1 di luar skalar blok; bila >= 0, indentasi induk skalar blok (|/>) yang sedang berjalan
}

// lintasi memindai SATU baris dan memperbarui keadaan; nilai
// kembali true berarti baris ini DIMULAI di dalam skalar
// (seluruhnya isi nilai: bukan kunci, bukan penanda, bukan batas
// blok). Di luar skalar, kutip hanya MEMBUKA pada posisi nilai —
// awal baris, atau sesudah pemisah struktural (":" pemetaan, "-"
// daftar, "[{," aliran); di posisi lain ia huruf biasa skalar
// polos (apos dalam kata seperti "don't" tidak membuka). '#'
// yang didahului spasi/tab di luar skalar memulai komentar:
// sisa baris inert.
//
// B-560: di dalam skalar blok (|/>) baris isinya tidak dipindai
// kutip sama sekali — kutip di sana huruf isi, bukan pembuka
// skalar semu. Baris yang MENGAKHIRI blok (tak-kosong dan tidak
// menjorok lebih dalam dari induknya) adalah baris struktural:
// ia dipindai normal dengan keadaan di luar skalar, jadi nilai
// kembali false untuknya walau keadaan blok masih aktif saat
// baris itu dimulai.
func (p *pelacakSkalarYAML) lintasi(line string) bool {
	mulaiDalam := p.kutip != 0
	if p.kutip == 0 && p.blok >= 0 {
		if strings.TrimSpace(line) == "" || lebarIndentasiYAML(line) > p.blok {
			return true
		}
		p.blok = -1
	}
	for i := 0; i < len(line); i++ {
		c := line[i]
		switch p.kutip {
		case '"':
			if c == '\\' {
				i++ // lewati karakter yang di-escape
			} else if c == '"' {
				p.kutip = 0
			}
			continue
		case '\'':
			if c == '\'' {
				if i+1 < len(line) && line[i+1] == '\'' {
					i++ // '' = apos yang di-escape
				} else {
					p.kutip = 0
				}
			}
			continue
		}
		if c == '#' && (i == 0 || line[i-1] == ' ' || line[i-1] == '\t') {
			return mulaiDalam
		}
		if c == '"' || c == '\'' {
			j := i - 1
			for j >= 0 && (line[j] == ' ' || line[j] == '\t') {
				j--
			}
			if j < 0 || line[j] == ':' || line[j] == '-' || line[j] == '[' || line[j] == '{' || line[j] == ',' {
				p.kutip = c
			}
		}
	}
	// Baris ini dapat MEMBUKA skalar blok bila berbentuk
	// "kunci: |" atau "- |" (B-560); pembuka hanya dikenali di
	// luar skalar terkutip.
	if p.kutip == 0 {
		if induk, ok := awalSkalarBlokYAML(line); ok {
			p.blok = induk
		}
	}
	return mulaiDalam
}

// awalSkalarBlokYAML melaporkan apakah baris ini MEMBUKA skalar
// blok YAML (|/>) dan indentasi induknya (B-560): pemetaan
// "kunci: |" (kunci boleh terkutip, nilai boleh membawa
// indikator chomping +/-, indikator indentasi 1-9, dan komentar)
// atau item urutan "- |". Nilai yang hanya diawali '|' tetapi
// berlanjut huruf lain ("|teks") adalah skalar polos, bukan
// pembuka blok. Baris kosong/komentar dan baris tanpa pemisah
// nilai bukan pembuka.
func awalSkalarBlokYAML(line string) (induk int, ok bool) {
	trim := strings.TrimSpace(line)
	if trim == "" || strings.HasPrefix(trim, "#") {
		return -1, false
	}
	induk = lebarIndentasiYAML(line)
	sisa := line[induk:]
	// Item urutan "- |" / "- > ...": induknya indentasi garis '-'.
	if strings.HasPrefix(sisa, "-") && (len(sisa) == 1 || sisa[1] == ' ' || sisa[1] == '\t') {
		return induk, akhirSkalarBlokYAML(strings.TrimLeft(sisa[1:], " \t"))
	}
	// Kunci terkutip di awal ("kunci: x": |): lewati sampai
	// penutupnya — ':' di dalamnya bukan pemisah nilai — lalu
	// pemisah ':' harus langsung mengikuti (boleh berjarak
	// spasi). Tanpa cabang ini, ':' di dalam kunci terkutip
	// yang diikuti spasi salah dibaca sebagai pemisah.
	if sisa[0] == '"' || sisa[0] == '\'' {
		q := sisa[0]
		j := 1
		for j < len(sisa) {
			if q == '"' && sisa[j] == '\\' {
				j += 2
				continue
			}
			if sisa[j] == q {
				if q == '\'' && j+1 < len(sisa) && sisa[j+1] == '\'' {
					j += 2
					continue
				}
				break
			}
			j++
		}
		if j >= len(sisa) {
			return -1, false // kunci terkutip tak tertutup
		}
		sisa = strings.TrimLeft(sisa[j+1:], " \t")
		if !strings.HasPrefix(sisa, ":") {
			return -1, false
		}
		return induk, akhirSkalarBlokYAML(strings.TrimLeft(sisa[1:], " \t"))
	}
	// Cari ':' pemisah nilai: yang diikuti spasi/tab/akhir baris.
	idx := -1
	for k := 0; k < len(sisa); k++ {
		if sisa[k] != ':' {
			continue
		}
		if k+1 >= len(sisa) || sisa[k+1] == ' ' || sisa[k+1] == '\t' {
			idx = k
			break
		}
	}
	if idx <= 0 {
		return -1, false
	}
	return induk, akhirSkalarBlokYAML(strings.TrimLeft(sisa[idx+1:], " \t"))
}

// akhirSkalarBlokYAML melaporkan apakah nilai (sesudah "kunci:"
// atau "-") adalah indikator skalar blok: '|' atau '>' diikuti
// paling banyak satu indikator chomping (+/-) dan satu indikator
// indentasi (angka 1-9), dalam urutan mana pun, lalu akhir baris
// atau komentar.
func akhirSkalarBlokYAML(nilai string) bool {
	if nilai == "" || (nilai[0] != '|' && nilai[0] != '>') {
		return false
	}
	sisa := nilai[1:]
	for k := 0; k < 2; k++ {
		if sisa == "" {
			break
		}
		if sisa[0] == '+' || sisa[0] == '-' {
			sisa = sisa[1:]
		} else if sisa[0] >= '1' && sisa[0] <= '9' {
			sisa = sisa[1:]
		} else {
			break
		}
	}
	sisa = strings.TrimLeft(sisa, " \t")
	return sisa == "" || strings.HasPrefix(sisa, "#")
}

// pelacakSkalarBaruYAML membuat pelacak dalam keadaan awal:
// di luar skalar terkutip maupun skalar blok. Konstruktor ini
// WAJIB dipakai, bukan literal struct kosong — nilai nol blok
// (0) akan salah dibaca sebagai "di dalam skalar blok berinduk
// 0", sedangkan keadaan awal adalah -1 (di luar blok).
func pelacakSkalarBaruYAML() *pelacakSkalarYAML {
	return &pelacakSkalarYAML{blok: -1}
}

// petaDalamSkalarYAML memindai seluruh baris sekali dan
// mengembalikan, per indeks, apakah baris itu DIMULAI di dalam
// skalar terkutip multiline ATAU skalar blok (lihat
// pelacakSkalarYAML).
func petaDalamSkalarYAML(baris []string) []bool {
	p := pelacakSkalarBaruYAML()
	dalam := make([]bool, len(baris))
	for i, l := range baris {
		dalam[i] = p.lintasi(l)
	}
	return dalam
}

// pindaiJangkarKunciYAML memindai dokumen masukan sampai penanda
// akhir dokumen dan mengembalikan jangkar PERTAMA yang terpasang
// pada tiap kunci terkelola (B-551): jangkar bisa duduk di
// kemunculan mana pun — termasuk duplikat yang akan dibuang —
// dan menjatuhkannya saat baris terkelola diganti membuat alias
// *nama di tempat lain menggantung ("unknown anchor") dan
// Configure gagal di verifikasi atas config yang sah.
func pindaiJangkarKunciYAML(existing string, dikelola map[string]bool) map[string]string {
	hasil := map[string]string{}
	baris := strings.Split(existing, "\n")
	dalam := petaDalamSkalarYAML(baris)
	for i, line := range baris {
		if dalam[i] {
			continue
		}
		if barisPenandaAkhirDokumen(line) {
			break
		}
		if line == "" || line[0] == ' ' || line[0] == '\t' || line[0] == '#' {
			continue
		}
		idx := strings.Index(line, ":")
		if idx <= 0 {
			continue
		}
		kunci := normalkanKunciYAML(strings.TrimSpace(line[:idx]))
		if kunci == "" || !dikelola[kunci] || hasil[kunci] != "" {
			continue
		}
		if j, _ := pisahJangkarHermes(strings.TrimSpace(line[idx+1:])); j != "" {
			hasil[kunci] = j
		}
	}
	return hasil
}

func perbaruiYAMLTopLevel(existing string, pairs [][2]string) string {
	// B-344: BOM di awal dokumen dibuang lebih dulu, sama seperti
	// jalur TOML (B-310). TrimSpace tidak membuang BOM, jadi kunci
	// pertama ber-BOM tidak pernah cocok dengan nama polosnya —
	// baris lamanya tertinggal dan pasangan baru ditambahkan di
	// akhir: kunci ganda di config Goose/Aider pengguna.
	existing = strings.TrimPrefix(existing, "\ufeff")
	dikelola := make(map[string]bool, len(pairs))
	nilai := make(map[string]string, len(pairs))
	for _, p := range pairs {
		dikelola[p[0]] = true
		nilai[p[0]] = p[1]
	}
	tertulis := make(map[string]bool, len(pairs))
	// B-551: jangkar yang terpasang pada baris kunci terkelola
	// (kemunculan mana pun) dipertahankan pada baris pengganti.
	jangkar := pindaiJangkarKunciYAML(existing, dikelola)
	tulisBaris := func(kunci string) string {
		if j := jangkar[kunci]; j != "" {
			return kunci + ": &" + j + " " + yamlKutip(nilai[kunci])
		}
		return kunci + ": " + yamlKutip(nilai[kunci])
	}
	var out []string
	// B-523: posisi baris penanda akhir dokumen "..." di out, atau
	// -1 bila tidak ada. Sesudah penanda, tidak ada lagi isi dokumen:
	// baris-baris diteruskan utuh tanpa dibaca sebagai kunci, dan
	// kunci terkelola yang belum tertulis disisipkan SEBELUM
	// penanda — versi lama menempelkannya di akhir teks, yaitu DI
	// LUAR dokumen, sehingga verifikasi urai gagal ("hasil sunting
	// YAML tidak terurai") atas config satu dokumen yang sah.
	indeksPenanda := -1
	// B-550: baris yang dimulai DI DALAM skalar terkutip multiline
	// adalah isi nilai, bukan struktur — tidak dibaca sebagai
	// kunci maupun penanda. B-560: baris isi skalar blok juga
	// bukan struktur (lihat pelacakSkalarYAML).
	pelacak := pelacakSkalarBaruYAML()
	for _, line := range strings.Split(existing, "\n") {
		if indeksPenanda >= 0 {
			// B-559: penanda kedua+ DINORMALISASI juga — cabang
			// ini dulu meneruskan baris sesudah penanda pertama
			// apa adanya, sehingga bentuk tab/tanpa-spasi
			// tertulis mentah dan ditolak PyYAML. Pelacak tetap
			// dimajukan di sini supaya baris mirip-penanda yang
			// berada DI DALAM skalar (isi string pengguna) tidak
			// ikut ditulis ulang.
			if !pelacak.lintasi(line) && barisPenandaAkhirDokumen(line) {
				out = append(out, normalkanBarisPenanda(line))
			} else {
				out = append(out, line)
			}
			continue
		}
		if pelacak.lintasi(line) {
			out = append(out, line)
			continue
		}
		if barisPenandaAkhirDokumen(line) {
			indeksPenanda = len(out)
			// B-544/B-559: penanda ditulis dalam bentuk kanoniknya
			// (tiap kemunculan, bukan hanya pertama).
			out = append(out, normalkanBarisPenanda(line))
			continue
		}
		kunci := ""
		if line != "" && line[0] != ' ' && line[0] != '\t' && line[0] != '#' {
			if i := strings.Index(line, ":"); i > 0 {
				// B-373: kunci yang dikutip ("GOOSE_PROVIDER": ...)
				// dinormalkan sebelum dibandingkan — parser YAML
				// membacanya sebagai kunci yang sama, tetapi
				// perbandingan mentah melewatkannya sehingga baris
				// lama tertinggal dan pasangan baru ditambahkan:
				// kunci ganda yang bacaan efektifnya bergantung
				// parser. Baris yang cocok ditulis ulang tanpa
				// kutip (bentuk kanonik keluaran fungsi ini).
				kunci = normalkanKunciYAML(strings.TrimSpace(line[:i]))
			}
		}
		if kunci != "" && dikelola[kunci] {
			if tertulis[kunci] {
				continue // duplikat kunci terkelola: buang
			}
			tertulis[kunci] = true
			out = append(out, tulisBaris(kunci))
			continue
		}
		out = append(out, line)
	}
	var belum []string
	for _, p := range pairs {
		if !tertulis[p[0]] {
			belum = append(belum, tulisBaris(p[0]))
		}
	}
	if indeksPenanda >= 0 && len(belum) > 0 {
		// B-523: sisipkan kunci yang belum tertulis SEBELUM penanda
		// akhir dokumen; penanda tetap baris penutup dokumen.
		sisip := make([]string, 0, len(out)+len(belum))
		sisip = append(sisip, out[:indeksPenanda]...)
		sisip = append(sisip, belum...)
		sisip = append(sisip, out[indeksPenanda:]...)
		hasil := strings.Join(sisip, "\n")
		if hasil != "" && !strings.HasSuffix(hasil, "\n") {
			hasil += "\n"
		}
		return hasil
	}
	hasil := strings.Join(out, "\n")
	for _, baris := range belum {
		if hasil != "" && !strings.HasSuffix(hasil, "\n") {
			hasil += "\n"
		}
		hasil += baris + "\n"
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

// kunciMapTerurut mengembalikan kunci map dalam urutan abjad yang
// deterministik (B-281): iterasi map Go bersifat acak, sehingga
// berkas yang disusun dari map yang sama bisa berbeda byte antar
// dua lari identik — diff berisik dan uji idempotensi byte gagal
// sesekali. Pola yang sama sudah dipakai susunKontenKeyEnv.
func kunciMapTerurut(vars map[string]string) []string {
	kunci := make([]string, 0, len(vars))
	for k := range vars {
		kunci = append(kunci, k)
	}
	sort.Strings(kunci)
	return kunci
}

// buildSnippet menyusun teks export shell untuk terminal.
func (m *Manager) buildSnippet(vars map[string]string) string {
	var sb strings.Builder
	for _, k := range kunciMapTerurut(vars) {
		v := vars[k]
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

	// Baca konten lama jika ada. B-494: lewat bacaReguler — cli-env.sh
	// adalah berkas milik Route-X sendiri yang dibaca pada SETIAP
	// Configure; FIFO di jalurnya menggantungkan Configure sambil
	// memegang mutex manajer. Berkas khusus ditolak bernama di sini
	// (tulisAtomik di bawah juga akan menolaknya, tetapi gagal di
	// baca lebih jujur daripada membangun konten pengganti dulu).
	var existing string
	if data, err := bacaReguler(envFile); err == nil {
		existing = string(data)
	} else if errors.Is(err, errBerkasKhusus) {
		return err
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
	for _, k := range kunciMapTerurut(vars) {
		v := vars[k]
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
		// B-494: lewat bacaReguler — FIFO di .bashrc/.zshrc
		// menggantungkan ReadFile tanpa batas. Berkas khusus yang
		// ADA ditolak bernama (melewatinya senyap akan mengklaim
		// hook terpasang padahal tidak); galat baca biasa tetap
		// berarti "tidak ada: tidak dibuatkan" seperti sebelumnya.
		data, err := bacaReguler(rcPath)
		if err != nil {
			if errors.Is(err, errBerkasKhusus) {
				return err
			}
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
		// B-494: bacaan pemeriksa ini juga lewat bacaReguler — berkas
		// khusus di jalur sistem tidak dibaca dan tidak ditimpa;
		// cukup dicatat sebagai peringatan best-effort (semantik
		// blok ini memang best-effort).
		if data, err := bacaReguler(profilePath); errors.Is(err, errBerkasKhusus) {
			if m.logger != nil {
				m.logger.Warn("hook /etc/profile.d dilewati: berkas tujuan bukan berkas reguler",
					"path", profilePath, "error", err)
			}
		} else if err != nil || !strings.Contains(string(data), "Route-X AI Gateway CLI Auto-Loader") {
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
		if err := verifikasiHasilJSON(data, cfg); err != nil {
			return err
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
		existingAider, err := bacaTeksKonfig(aiderConfigPath)
		if err != nil {
			return err
		}
		// B-497: config multi-dokumen ditolak jujur seperti Hermes
		// (B-485) — pemuat Aider adalah PyYAML safe_load, yang
		// terbukti menolak aliran multi-dokumen; penggabung
		// per-baris di bawah akan merestrukturisasi dokumen pertama
		// dan melaporkan sukses atas berkas yang tak termuat.
		if err := tolakYAMLDokumenGanda(aiderConfigPath, existingAider, "Aider (PyYAML safe_load) hanya menerima satu dokumen"); err != nil {
			return err
		}
		pasanganAider := [][2]string{
			{"openai-api-base", gwURL},
			{"model", modelAider(target)},
		}
		content := perbaruiYAMLTopLevel(existingAider, pasanganAider)
		if err := verifikasiHasilYAMLTingkatAtas(content, pasanganAider); err != nil {
			return err
		}
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
		existing, err := bacaTeksKonfig(goosePath)
		if err != nil {
			return err
		}
		// B-497: config multi-dokumen ditolak jujur seperti Hermes
		// (B-485) — penggabung tingkat-atas di bawah bekerja pada
		// teks gabungan dan verifikasi hanya membaca dokumen
		// pertama, jadi sukses atas aliran multi-dokumen adalah
		// sukses-bohong kelas yang sama walau semantik pemuat
		// Goose atas dokumen kedua tidak dibuktikan dengan biner
		// sungguhan (lihat badan PR).
		if err := tolakYAMLDokumenGanda(goosePath, existing, "penyunting Route-X hanya menerapkan konfigurasi ke satu dokumen"); err != nil {
			return err
		}
		pasanganGoose := [][2]string{
			{"GOOSE_PROVIDER", "openai"},
			{"GOOSE_MODEL", target},
			{"OPENAI_HOST", strings.TrimSuffix(gwURL, "/v1")},
		}
		content := perbaruiYAMLTopLevel(existing, pasanganGoose)
		if err := verifikasiHasilYAMLTingkatAtas(content, pasanganGoose); err != nil {
			return err
		}
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
		// B-280: bila berkas yang ada memuat komentar JSONC, sunting
		// TEKS aslinya secara bedah agar komentar pengguna bertahan —
		// penulisan ulang dari peta (MarshalIndent) membuang seluruh
		// komentar walau datanya selamat. Hasil sunting hanya dipakai
		// bila lolos verifikasi semantik terhadap mutasi peta yang
		// sama persis; selain itu penulis jatuh ke jalur marshal di
		// bawah (data tetap benar, komentar hilang seperti dulu).
		tertulis := false
		// B-494: bacaan mentah sunting-bedah ini lewat bacaReguler juga
		// (muatKonfigJSON di atas sudah menolak berkas khusus bernama;
		// penjaga di sini menutup celah balapan di antara keduanya —
		// galat apa pun tetap hanya melewatkan jalur bedah, seperti
		// sebelumnya).
		if mentah, errBaca := bacaReguler(configPath); errBaca == nil {
			// B-404: BOM dibuang dari teks mentah SEBELUM jalur
			// sunting JSONC — muatKonfigJSON sudah membuangnya
			// untuk peta (B-380), tetapi bacaan mentah di sini
			// tidak, jadi berkas ber-BOM + komentar selalu gagal
			// disunting (pemindai bedah menolak BOM di awal) dan
			// jatuh senyap ke penulisan ulang marshal yang
			// membuang seluruh komentar pengguna — kehilangan
			// yang justru dicegah B-280.
			mentahTeks := strings.TrimPrefix(string(mentah), "\ufeff")
			if jsoncMengandungKomentar(mentahTeks) {
				if hasil, errSunting := suntingOpenCodeJSONC(mentahTeks, target, gwURL, apiKey); errSunting == nil &&
					hasilSuntingOpenCodeSah(hasil, cfg, target, gwURL, apiKey) {
					if err := tulisAtomik(configPath, []byte(hasil), 0600); err != nil {
						return fmt.Errorf("menulis %s: %w", configPath, err)
					}
					tertulis = true
				}
			}
		}
		if tertulis {
			return nil
		}
		mutasiCfgOpenCode(cfg, target, gwURL, apiKey)
		data, err := json.MarshalIndent(cfg, "", "  ")
		if err != nil {
			return fmt.Errorf("mengkodekan opencode.json: %w", err)
		}
		if err := verifikasiHasilJSON(data, cfg); err != nil {
			return err
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
		existingCodex, err := bacaTeksKonfig(codexPath)
		if err != nil {
			return err
		}
		pasanganPreambleCodex := [][2]string{
			{"model_provider", "routex"},
			{"model", target},
		}
		pasanganSeksiCodex := [][2]string{
			{"name", "Route-X"},
			{"base_url", gwURL},
			{"env_key", "OPENAI_API_KEY"},
			{"wire_api", "responses"},
		}
		kontenCodex := perbaruiPreambleTOML(existingCodex, pasanganPreambleCodex)
		// B-222: gabung PER-KUNCI, bukan ganti seluruh seksi — kunci
		// lain yang pengguna taruh di [model_providers.routex]
		// (mis. request_max_retries) adalah konfigurasi sah Codex
		// dan tidak boleh terhapus setiap Configure.
		kontenCodex = perbaruiKunciSeksiTOML(kontenCodex, "model_providers.routex", pasanganSeksiCodex)
		if err := verifikasiHasilTOML(kontenCodex, pasanganPreambleCodex, map[string][][2]string{
			"model_providers.routex": pasanganSeksiCodex,
		}); err != nil {
			return err
		}
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
		existingGrok, err := bacaTeksKonfig(grokPath)
		if err != nil {
			return err
		}
		pasanganModelsGrok := [][2]string{{"default", "routex"}}
		pasanganRoutexGrok := [][2]string{
			{"model", target},
			{"name", "Route-X"},
			{"base_url", gwURL},
			{"env_key", "XAI_API_KEY"},
		}
		kontenGrok := perbaruiKunciSeksiTOML(existingGrok, "models", pasanganModelsGrok)
		// B-222: gabung PER-KUNCI seperti codex di atas — kunci lain
		// pengguna di [model.routex] dipertahankan.
		kontenGrok = perbaruiKunciSeksiTOML(kontenGrok, "model.routex", pasanganRoutexGrok)
		if err := verifikasiHasilTOML(kontenGrok, nil, map[string][][2]string{
			"models":       pasanganModelsGrok,
			"model.routex": pasanganRoutexGrok,
		}); err != nil {
			return err
		}
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
		if err := verifikasiHasilJSON(dataQ, qcfg); err != nil {
			return err
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
		if err := verifikasiHasilJSON(dataC, ccfg); err != nil {
			return err
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
		envPath := filepath.Join(hermesDir, ".env")
		// B-477: .env berkode UTF-16/32 ditolak SEBELUM berkas apa
		// pun ditulis. Jalur lama menjalankannya lewat penyunting
		// teks UTF-8 dan menghasilkan berkas yang bukan UTF-8 sah
		// maupun UTF-16 sah sementara Configure melaporkan sukses
		// dan menyimpan baris DB — padahal python-dotenv Hermes
		// gagal membaca berkas UTF-16 sama sekali
		// (UnicodeDecodeError, terbukti dengan pustaka sungguhan).
		var existingEnv string
		if apiKey != "" {
			isiEnv, errEnv := bacaTeksKonfig(envPath)
			if errEnv != nil {
				return errEnv
			}
			existingEnv = isiEnv
			if enc := deteksiBOMNonUTF8([]byte(isiEnv)); enc != "" {
				return fmt.Errorf("berkas %s berkode %s; Hermes hanya membaca .env UTF-8 — simpan ulang berkas itu sebagai UTF-8, lalu coba lagi (Configure dibatalkan, tidak ada berkas yang diubah)", envPath, enc)
			}
			// B-522: UTF-16 TANPA BOM lolos dari pemeriksa BOM di
			// atas — isinya terbaca sebagai teks yang nama
			// terkelolanya tidak terbaca, baris lama dipertahankan
			// dan baris bersih ditempel, dan Configure melaporkan
			// sukses atas berkas yang tetap tidak bisa dibaca
			// Hermes. Pola NUL-nya dideteksi terpisah dengan
			// syarat bukti+dominasi+kerapatan (lihat
			// deteksiUTF16TanpaBOM; diperketat B-531) supaya
			// NUL terisolasi yang ditoleransi B-507 tidak ikut
			// tertolak.
			if enc := deteksiUTF16TanpaBOM([]byte(isiEnv)); enc != "" {
				return fmt.Errorf("berkas %s terdeteksi berkode %s tanpa BOM; Hermes hanya membaca .env UTF-8 — simpan ulang berkas itu sebagai UTF-8, lalu coba lagi (Configure dibatalkan, tidak ada berkas yang diubah)", envPath, enc)
			}
			// B-548: UTF-32 TANPA BOM juga lolos dari pemeriksa
			// BOM, dan penjaga NUL-ganda rapat di detektor UTF-16
			// justru MELEPASKANNYA sebagai "bukan UTF-16" — tanpa
			// pemeriksa ini berkas UTF-32 menempuh jalur sukses
			// yang sama dengan UTF-16 tanpa BOM dulu. Pola NUL
			// per-4-byte-nya dideteksi terpisah (lihat
			// deteksiUTF32TanpaBOM).
			if enc := deteksiUTF32TanpaBOM([]byte(isiEnv)); enc != "" {
				return fmt.Errorf("berkas %s terdeteksi berkode %s tanpa BOM; Hermes hanya membaca .env UTF-8 — simpan ulang berkas itu sebagai UTF-8, lalu coba lagi (Configure dibatalkan, tidak ada berkas yang diubah)", envPath, enc)
			}
		}
		existingHermes, err := bacaTeksKonfig(hermesPath)
		if err != nil {
			return err
		}
		// B-485: config multi-dokumen ditolak jujur. Penyusun blok
		// bekerja pada teks gabungan dan verifikasi hanya membaca
		// dokumen pertama, tetapi PyYAML safe_load — pemuat Hermes
		// — MENOLAK aliran multi-dokumen ("expected a single
		// document in the stream", terbukti dengan pustaka
		// sungguhan): sukses di sini adalah sukses-bohong.
		// (B-497: penjaga yang sama kini dipakai semua penulis YAML
		// lewat tolakYAMLDokumenGanda.)
		if err := tolakYAMLDokumenGanda(hermesPath, existingHermes, "Hermes (PyYAML safe_load) hanya menerima satu dokumen"); err != nil {
			return err
		}
		// B-543: isi non-kosong SESUDAH penanda "..." tanpa "---"
		// adalah dokumen tambahan yang tidak terhitung penjaga di
		// atas bila penghitungnya galat — tolak jujur di sini,
		// sebelum penyusunan apa pun.
		if err := tolakIsiSesudahPenandaHermes(hermesPath, existingHermes); err != nil {
			return err
		}
		kontenHermes := susunBlokModelHermes(existingHermes, target, gwURL, apiKey)
		if err := verifikasiHasilHermes(kontenHermes, target, gwURL, apiKey); err != nil {
			return err
		}
		if err := tulisAtomik(hermesPath, []byte(kontenHermes), 0600); err != nil {
			return fmt.Errorf("menulis %s: %w", hermesPath, err)
		}
		if apiKey != "" {
			// existingEnv sudah dibaca (dan lolos deteksi B-477)
			// di awal case ini.
			kontenEnv := perbaruiBarisEnv(existingEnv, "OPENAI_API_KEY", apiKey)
			if err := verifikasiEnvHermes(kontenEnv, "OPENAI_API_KEY"); err != nil {
				return err
			}
			if err := tulisAtomik(envPath, []byte(kontenEnv), 0600); err != nil {
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

// mutasiCfgOpenCode menerapkan mutasi konfigurasi Route-X pada
// peta opencode.json hasil urai: $schema, provider.routex (options
// baseURL + apiKey efektif, models.<target>), dan model aktif.
// Inilah SATU-SATUNYA definisi semantik penulis opencode — jalur
// marshal memakainya langsung, dan jalur sunting teks B-280
// diverifikasi terhadap hasil mutasi ini.
func mutasiCfgOpenCode(cfg map[string]any, target, gwURL, apiKey string) {
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
}

// suntingOpenCodeJSONC menerapkan mutasi yang sama seperti
// mutasiCfgOpenCode langsung pada TEKS JSONC, memakai penyunting
// bedah jsonc_edit.go: hanya rentang nilai kunci terkelola yang
// berubah, komentar dan format lain bertahan. Semantiknya meniru
// jalur peta titik demi titik — termasuk rangka provider routex
// lengkap (npm + options + models) bila provider itu belum ada
// atau bukan objek.
func suntingOpenCodeJSONC(s, target, gwURL, apiKey string) (string, error) {
	kutip := func(v string) string {
		b, _ := json.Marshal(v)
		return string(b)
	}
	akar, err := akarObjekJSONC(s)
	if err != nil {
		return "", err
	}
	if s, err = tetapkanKunciJSONC(s, akar, "$schema", kutip("https://opencode.ai/config.json")); err != nil {
		return "", err
	}
	var prov int
	if s, prov, err = pastikanObjekAnakJSONC(s, akar, "provider"); err != nil {
		return "", err
	}
	_, ada, bukanObjek, err := cariObjekAnakJSONC(s, prov, "routex")
	if err != nil {
		return "", err
	}
	if !ada || bukanObjek {
		rangkaOpts := map[string]any{"baseURL": gwURL}
		if apiKey != "" {
			rangkaOpts["apiKey"] = apiKey
		}
		rangka, err := json.Marshal(map[string]any{
			"npm":     "@ai-sdk/openai-compatible",
			"options": rangkaOpts,
			"models":  map[string]any{},
		})
		if err != nil {
			return "", err
		}
		if s, err = tetapkanKunciJSONC(s, prov, "routex", string(rangka)); err != nil {
			return "", err
		}
	} else {
		var opt int
		rx, _, _, err := cariObjekAnakJSONC(s, prov, "routex")
		if err != nil {
			return "", err
		}
		if s, opt, err = pastikanObjekAnakJSONC(s, rx, "options"); err != nil {
			return "", err
		}
		if s, err = tetapkanKunciJSONC(s, opt, "baseURL", kutip(gwURL)); err != nil {
			return "", err
		}
		if apiKey != "" {
			if s, err = tetapkanKunciJSONC(s, opt, "apiKey", kutip(apiKey)); err != nil {
				return "", err
			}
		} else {
			if s, err = hapusKunciJSONC(s, opt, "apiKey"); err != nil {
				return "", err
			}
		}
	}
	// models.<target> pada provider routex (dibuat bila belum ada).
	rx, _, _, err := cariObjekAnakJSONC(s, prov, "routex")
	if err != nil {
		return "", err
	}
	var mdl int
	if s, mdl, err = pastikanObjekAnakJSONC(s, rx, "models"); err != nil {
		return "", err
	}
	if s, err = tetapkanKunciJSONC(s, mdl, target, `{"name": `+kutip(target)+`}`); err != nil {
		return "", err
	}
	if s, err = tetapkanKunciJSONC(s, akar, "model", kutip("routex/"+target)); err != nil {
		return "", err
	}
	return s, nil
}

// hasilSuntingOpenCodeSah memverifikasi hasil sunting teks B-280:
// diurai ulang secara toleran, hasilnya wajib sama persis
// (DeepEqual) dengan mutasiCfgOpenCode atas konfigurasi awal.
// Gerbang ini yang membuat jalur sunting aman: bentuk JSONC apa
// pun yang disalahartikan pemindai bedah gugur ke jalur marshal,
// tidak pernah menjadi config yang salah.
func hasilSuntingOpenCodeSah(hasil string, cfgAwal map[string]any, target, gwURL, apiKey string) bool {
	// B-478: kedua sisi diurai dengan UseNumber — perbandingan
	// float64 lama menyamakan 9007199254740993 dengan …992,
	// sehingga suntingan yang menggeser angka besar pengguna
	// tetap dinyatakan "sah" dan dipakai.
	terurai, err := uraiPetaJSON(uraiJSONCToleran([]byte(hasil)))
	if err != nil {
		return false
	}
	rawHarapan, err := json.Marshal(cfgAwal)
	if err != nil {
		return false
	}
	harapan, err := uraiPetaJSON(rawHarapan)
	if err != nil {
		return false
	}
	mutasiCfgOpenCode(harapan, target, gwURL, apiKey)
	return reflect.DeepEqual(terurai, harapan)
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

// pemindaiMultilineTOML melacak keadaan string multiline TOML
// ("""…""" dasar dan ”'…”' literal) lintas baris (B-439).
// Penyunting TOML di berkas ini bekerja per baris; tanpa kesadaran
// ini, baris ISI string multiline yang mirip kunci/header
// diperlakukan sebagai kode sungguhan: isi teks pengguna ditulis
// ulang, kunci asli dibuang sebagai "duplikat", dan batas preamble
// /seksi terpotong di tengah string — sementara Configure tetap
// melaporkan sukses. Pemindai ini hanya menjawab satu pertanyaan
// per baris: apakah baris itu DIMULAI di dalam string multiline.
type pemindaiMultilineTOML struct {
	// multi adalah karakter pembatas string multiline yang sedang
	// terbuka ('"' atau '\''), 0 bila di luar string multiline.
	multi byte
	// kurung adalah kedalaman kurung siku [...] yang sedang
	// terbuka di luar string/komentar (B-479): nilai array boleh
	// membentang banyak baris dan isinya boleh diawali karakter
	// kurung — tanpa pelacakan ini, baris isi array dibaca sebagai
	// header seksi dan awal array kunci terkelola diganti baris
	// tunggal sementara sisa arraynya yatim.
	kurung int
	// kurawal adalah kedalaman kurung kurawal {...} tabel inline
	// yang sedang terbuka di luar string/komentar (B-505): tabel
	// inline juga boleh membentang banyak baris (sah bagi parser
	// TOML produk sendiri), dan baris isinya berbentuk
	// `kunci = nilai` — tanpa pelacakan ini baris isi dibaca
	// sebagai kunci tingkat-atas/seksi sungguhan: penyunting
	// mengganti "model" DI DALAM tabel dan membuang model
	// tingkat-atas asli sebagai duplikat.
	kurawal int
}

// diDalamStruktur melaporkan apakah posisi kini berada di dalam
// nilai yang membentang lintas baris: string multiline, array
// yang belum tertutup, ATAU tabel inline yang belum tertutup.
// Baris yang dimulai dalam keadaan ini adalah kelanjutan nilai,
// bukan kode.
func (p *pemindaiMultilineTOML) diDalamStruktur() bool {
	return p.multi != 0 || p.kurung > 0 || p.kurawal > 0
}

// pindaiBaris memindai satu baris dan memajukan keadaan multiline
// ke akhir baris itu. String satu baris dan komentar ditangani
// supaya pembatas di dalamnya tidak membuka string multiline
// palsu.
func (p *pemindaiMultilineTOML) pindaiBaris(l string) {
	i := 0
	for i < len(l) {
		if p.multi != 0 {
			// Di dalam string multiline hanya pembatas tutup yang
			// bermakna. Pada string dasar, \" adalah escape dan
			// bukan bagian dari pembatas.
			if p.multi == '"' && l[i] == '\\' {
				i += 2
				continue
			}
			if l[i] == p.multi && i+2 < len(l) && l[i+1] == p.multi && l[i+2] == p.multi {
				p.multi = 0
				i += 3
				continue
			}
			i++
			continue
		}
		switch c := l[i]; c {
		case '#':
			return // komentar: sisa baris bukan kode
		case '[':
			p.kurung++
			i++
		case ']':
			if p.kurung > 0 {
				p.kurung--
			}
			i++
		case '{':
			p.kurawal++
			i++
		case '}':
			if p.kurawal > 0 {
				p.kurawal--
			}
			i++
		case '"', '\'':
			if i+2 < len(l) && l[i+1] == c && l[i+2] == c {
				p.multi = c
				i += 3
				continue
			}
			// String satu baris: lompat ke penutupnya pada baris
			// ini juga supaya isinya tidak ditafsirkan sebagai kode.
			i++
			for i < len(l) {
				if c == '"' && l[i] == '\\' {
					i += 2
					continue
				}
				if l[i] == c {
					i++
					break
				}
				i++
			}
		default:
			i++
		}
	}
}

// petaBarisStrukturTOML mengembalikan, untuk tiap baris, apakah
// baris itu dimulai di dalam NILAI lintas-baris (string multiline,
// array belum tertutup, atau tabel inline belum tertutup) —
// B-479+B-505. Penentu batas preamble dan
// seksi memakai peta ini: hanya baris yang dimulai di luar
// struktur apa pun yang boleh dinilai sebagai header seksi.
func petaBarisStrukturTOML(baris []string) []bool {
	p := &pemindaiMultilineTOML{}
	diDalam := make([]bool, len(baris))
	for i, l := range baris {
		diDalam[i] = p.diDalamStruktur()
		p.pindaiBaris(l)
	}
	return diDalam
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
	pindai := &pemindaiMultilineTOML{}
	for i := 0; i < len(baris); i++ {
		l := baris[i]
		// B-439+B-479: baris isi string multiline ATAU array
		// lintas-baris adalah TEKS/nilai pengguna, bukan kode —
		// dipertahankan apa adanya dan tidak pernah dinilai
		// sebagai kunci/header, berapa pun miripnya.
		if pindai.diDalamStruktur() {
			hasil = append(hasil, l)
			pindai.pindaiBaris(l)
			continue
		}
		kunci := ""
		if eq := strings.Index(l, "="); eq > 0 {
			// B-343: sisi-kiri dinormalkan sebelum dibandingkan —
			// kunci terkutip (`"model" = …`, `'model' = …`) adalah
			// bentuk sah TOML yang menamai kunci yang sama, tetapi
			// perbandingan mentah tidak pernah mencocokkannya:
			// baris lama tertinggal DAN pasangan kelolaan
			// ditambahkan di akhir (kunci ganda, TOML tak sah).
			// Normalisasinya memakai logika segmen yang sama
			// seperti nama seksi (B-309); penggantinya di bawah
			// ditulis dengan kunci polos kanonis ini.
			if nama := normalisasiNamaSeksiTOML(strings.TrimSpace(l[:eq])); nama != "" {
				if _, ok := nilai[nama]; ok {
					kunci = nama
				}
			}
		}
		if kunci == "" {
			hasil = append(hasil, l)
			pindai.pindaiBaris(l)
			continue
		}
		// Kunci terkelola ditemukan. Penetapannya bisa membentang
		// beberapa baris bila nilainya string multiline atau array
		// (B-479): seluruh rentang itu diganti SATU baris kanonik
		// (atau dibuang sebagai duplikat) — mengganti baris
		// pertamanya saja akan meninggalkan sisa nilai yatim yang
		// merusak dokumen.
		sesudah := *pindai
		akhir := i
		sesudah.pindaiBaris(l)
		for sesudah.diDalamStruktur() && akhir+1 < len(baris) {
			akhir++
			sesudah.pindaiBaris(baris[akhir])
		}
		if !tertulis[kunci] {
			tertulis[kunci] = true
			hasil = append(hasil, kunci+" = "+tomlKutip(nilai[kunci]))
		}
		*pindai = sesudah
		i = akhir
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
	// B-310: BOM di awal dokumen dibuang lebih dulu. TrimSpace tidak
	// membuang BOM, jadi tanpa langkah ini kunci pertama ber-BOM
	// tidak pernah cocok dengan nama polosnya (duplikat) dan header
	// seksi pertama tidak terdeteksi sebagai awal seksi (kunci root
	// baru jatuh ke dalam seksi).
	existing = strings.TrimPrefix(existing, "\ufeff")
	baris := strings.Split(existing, "\n")
	// B-439+B-479: baris pembuka-[ di dalam string multiline atau
	// array adalah isi nilai, bukan awal seksi — batas preamble
	// hanya dihitung dari baris yang berada di luar struktur.
	diDalam := petaBarisStrukturTOML(baris)
	idx := len(baris)
	for i, l := range baris {
		if diDalam[i] {
			continue
		}
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
	// Kurung tutup dicari dengan menghormati segmen terkutip: `]`
	// di dalam string TOML bukan penutup header.
	tutup := -1
	var kutip byte
	escape := false
	for i := 1; i < len(l); i++ {
		c := l[i]
		if kutip == '"' && escape {
			escape = false
			continue
		}
		if kutip == '"' && c == '\\' {
			escape = true
			continue
		}
		if kutip != 0 {
			if c == kutip {
				kutip = 0
			}
			continue
		}
		if c == '"' || c == '\'' {
			kutip = c
			continue
		}
		if c == ']' {
			tutup = i
			break
		}
	}
	if tutup < 0 {
		return ""
	}
	sisa := strings.TrimSpace(l[tutup+1:])
	if sisa != "" && !strings.HasPrefix(sisa, "#") {
		return ""
	}
	return normalisasiNamaSeksiTOML(l[1:tutup])
}

// normalisasiNamaSeksiTOML mengubah nama seksi TOML menjadi bentuk
// titik polos yang dibandingkan penulis (B-309): setiap segmen kunci
// bertitik boleh ditulis polos, sebagai string dasar ("…"), atau
// sebagai string literal ('…') — ketiganya menamai tabel yang sama.
// Titik di dalam segmen terkutip BUKAN pemisah segmen.
func normalisasiNamaSeksiTOML(nama string) string {
	var segmen []string
	var b strings.Builder
	var kutip byte
	escape := false
	flush := func() {
		seg := strings.TrimSpace(b.String())
		if len(seg) >= 2 {
			if seg[0] == '"' && seg[len(seg)-1] == '"' {
				if uq, err := strconv.Unquote(seg); err == nil {
					seg = uq
				} else {
					seg = seg[1 : len(seg)-1]
				}
			} else if seg[0] == '\'' && seg[len(seg)-1] == '\'' {
				seg = seg[1 : len(seg)-1]
			}
		}
		segmen = append(segmen, seg)
		b.Reset()
	}
	for i := 0; i < len(nama); i++ {
		c := nama[i]
		if kutip == '"' && escape {
			b.WriteByte(c)
			escape = false
			continue
		}
		if kutip == '"' && c == '\\' {
			b.WriteByte(c)
			escape = true
			continue
		}
		if kutip != 0 {
			b.WriteByte(c)
			if c == kutip {
				kutip = 0
			}
			continue
		}
		if c == '"' || c == '\'' {
			kutip = c
			b.WriteByte(c)
			continue
		}
		if c == '.' {
			flush()
			continue
		}
		b.WriteByte(c)
	}
	flush()
	return strings.Join(segmen, ".")
}

// rentangSeksiTOML mencari rentang isi seksi [nama] (indeks baris isi
// awal dan akhir eksklusif) di dokumen yang sudah dipecah per baris.
// ok=false bila seksi tidak ada.
func rentangSeksiTOML(baris []string, nama string) (awal, akhir int, ok bool) {
	// B-439+B-479: header di dalam string multiline atau array
	// adalah isi nilai — ia tidak membuka seksi dan tidak menutup
	// seksi yang sedang berjalan.
	diDalam := petaBarisStrukturTOML(baris)
	for i, l := range baris {
		if diDalam[i] || namaSeksiTOML(l) != nama {
			continue
		}
		akhir = len(baris)
		for j := i + 1; j < len(baris); j++ {
			if diDalam[j] {
				continue
			}
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
	// B-480: BOM dibuang di sini juga, bukan hanya di jalur
	// preamble (B-310) — Grok memanggil fungsi ini langsung atas
	// berkas mentah, jadi BOM dulu BERTAHAN di keluaran dan
	// verifikasi hasil akhir (go-toml) menolaknya: Configure Grok
	// gagal untuk berkas ber-BOM yang oleh jalur Codex (yang
	// melewati preamble) diterima.
	existing = strings.TrimPrefix(existing, "\ufeff")
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
	// B-402: nama kunci dinormalkan dengan normalkanKunciYAML
	// seperti semua pencocok kunci YAML lain di berkas ini —
	// header berkutip ("model": / 'model':) adalah kunci yang
	// sama bagi parser YAML, tetapi perbandingan mentah dulu
	// melewatkannya, jadi blok model: BARU ditambahkan di bawah
	// blok berkutip yang sudah ada dan dokumen berakhir dengan
	// dua blok model yang bertentangan.
	if idx < 0 || normalkanKunciYAML(strings.TrimSpace(teks[:idx])) != "model" {
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
	// B-536: baris penanda akhir dokumen "..." (bila ada) MENUTUP
	// dokumen bagi pemuat Hermes — blok `model:` hanya hidup di
	// DALAM dokumen. Versi lama memperlakukan teks sebagai satu
	// hamparan: blok baru untuk config berpenanda TANPA blok model
	// ditempel di akhir teks, yaitu SESUDAH penanda di luar
	// dokumen, dan verifikasi hasil menolaknya ("blok model tidak
	// efektif") atas config yang sah. Kini pemindaian rentang
	// berhenti di penanda, dan blok baru disisipkan SEBELUM
	// penanda (lihat cabang tanpa-rentang di bawah); baris-baris
	// sesudah penanda tidak pernah menjadi isi dokumen.
	// B-550: peta baris yang dimulai DI DALAM skalar terkutip
	// multiline — baris isi nilai tidak boleh dibaca sebagai
	// penanda, header blok, atau batas blok. B-560: peta yang
	// sama kini mencakup isi skalar blok.
	dalam := petaDalamSkalarYAML(baris)
	indeksPenanda := -1
	for i, l := range baris {
		if !dalam[i] && barisPenandaAkhirDokumen(l) {
			// B-544/B-559: SETIAP penanda ditulis dalam bentuk
			// kanoniknya, bukan hanya yang pertama — versi lama
			// menormalkan lalu `break`, sehingga penanda kedua+
			// berbentuk tab/tanpa-spasi diteruskan mentah dan
			// lolos penjaga B-543 yang mengecualikannya,
			// sementara verifikasi hanya mengurai dokumen
			// pertama. Penanda di dalam skalar (dalam[i]) adalah
			// isi string dan tidak disentuh.
			baris[i] = normalkanBarisPenanda(l)
			if indeksPenanda < 0 {
				indeksPenanda = i
			}
		}
	}
	// Cari SEMUA blok `model:` tingkat-atas beserta rentangnya.
	// B-441: versi lama berhenti di kemunculan PERTAMA dan langsung
	// kembali — blok kedua tertinggal apa adanya dan, sebagai
	// duplikat kunci tingkat-atas yang menang last-wins di parser
	// YAML Hermes, membatalkan seluruh pembaruan blok pertama
	// sementara Configure melaporkan sukses. Kini semua blok
	// digabung menjadi satu.
	//
	// B-481+B-482: header boleh membawa JANGKAR (`model: &m`) dan
	// nilai boleh berbentuk FLOW MAP satu/rentang baris
	// (`model: {default: lama, catatan: x}`) — keduanya bentuk sah
	// YAML yang dulu dibaca sebagai "skalar": blok diganti dan
	// seluruh isi flow map (termasuk kunci tak-terkelola) hilang
	// senyap, dan jangkar yang dirujuk alias `*m` di tempat lain
	// ikut musnah. Kini jangkar dipertahankan pada header blok
	// gabungan dan entri tak-terkelola flow map diurai tingkat-atas
	// lalu dipertahankan sebagai baris blok biasa.
	type rentangBlok struct {
		awal, akhir int
		jangkar     string   // nama jangkar header ("" bila tidak ada)
		aliran      []string // baris tak-terkelola dari flow map, sudah bentuk blok
	}
	var rentang []rentangBlok
	for i, l := range baris {
		if indeksPenanda >= 0 && i >= indeksPenanda {
			break // B-536: sesudah penanda = di luar dokumen
		}
		if dalam[i] {
			continue // B-550: isi skalar multiline, bukan header
		}
		sisa, adalah := pecahHeaderModelHermes(l)
		if !adalah {
			continue
		}
		jangkar, inti := pisahJangkarHermes(sisa)
		if strings.HasPrefix(inti, "{") {
			// Flow map: kumpulkan teks sampai kurungnya seimbang
			// (boleh membentang beberapa baris).
			teks := inti
			j := i
			for !flowMapHermesSeimbang(teks) && j+1 < len(baris) {
				j++
				teks += "\n" + baris[j]
			}
			barisAliran, okFlow := pecahFlowMapHermes(teks, terkelola)
			if !okFlow {
				// Bentuk yang tidak bisa diurai tingkat-atas
				// dengan aman: kembalikan dokumen APA ADANYA —
				// verifikasi hasil akhir akan menolaknya secara
				// jujur (nilai terkelola lama tidak sama dengan
				// nilai Route-X), bukan menebak dan berisiko
				// menghilangkan isi pengguna. B-559: yang
				// dikembalikan adalah baris-baris sesudah
				// normalisasi penanda (gabung pecah-baris itu
				// pulang-pergi persis) — bukan `existing`
				// mentah — supaya penanda kedua+ berbentuk
				// tab/tanpa-spasi tidak ikut lolos mentah di
				// jalur ini.
				return strings.Join(baris, "\n")
			}
			rentang = append(rentang, rentangBlok{awal: i, akhir: j + 1, jangkar: jangkar, aliran: barisAliran})
			i = j
			continue
		}
		// B-277: sisa berupa komentar (`model: # catatan`) adalah
		// header PETA, bukan skalar — sub-baris menjorok sesudahnya
		// adalah isi blok lama yang harus digabung per-kunci di
		// bawah.
		if inti != "" && !strings.HasPrefix(inti, "#") {
			// Bentuk skalar satu baris: rentangnya baris itu saja.
			rentang = append(rentang, rentangBlok{awal: i, akhir: i + 1, jangkar: jangkar})
			continue
		}
		// Bentuk peta: isi blok adalah baris sesudahnya yang kosong,
		// berindentasi, ATAU komentar — pada kedalaman berapa pun.
		// B-440: komentar kolom-0 di tengah blok dulu MENGHENTIKAN
		// pemindaian, sehingga sub-kunci terkelola sesudahnya
		// tertinggal dan menang last-wins; dalam YAML baris komentar
		// tidak mengakhiri pemetaan. Blok berakhir di baris isi
		// kolom-0 pertama yang bukan komentar.
		j := i + 1
		for j < len(baris) {
			if dalam[j] {
				// B-550: baris isi skalar multiline yang dibuka
				// baris blok ini adalah ISI blok pada kolom berapa
				// pun — ia tidak mengakhiri blok.
				j++
				continue
			}
			sub := baris[j]
			trim := strings.TrimSpace(sub)
			if trim == "" || strings.HasPrefix(trim, "#") ||
				strings.HasPrefix(sub, " ") || strings.HasPrefix(sub, "\t") {
				j++
				continue
			}
			break
		}
		rentang = append(rentang, rentangBlok{awal: i, akhir: j, jangkar: jangkar})
	}
	if len(rentang) == 0 {
		if indeksPenanda >= 0 {
			// B-536: belum ada blok model DI DALAM dokumen —
			// sisipkan blok baru tepat sebelum baris penanda,
			// dengan pemisah satu baris kosong seperti jalur
			// tempel-akhir di bawah; penanda dan baris-baris
			// sesudahnya diteruskan utuh.
			kepala := strings.TrimRight(strings.Join(baris[:indeksPenanda], "\n"), "\n")
			hasil := ""
			if kepala != "" {
				hasil = kepala + "\n\n"
			}
			hasil += strings.Join(blok, "\n") + "\n" +
				strings.Join(baris[indeksPenanda:], "\n")
			if !strings.HasSuffix(hasil, "\n") {
				hasil += "\n"
			}
			return hasil
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
	// B-222 + B-440: saring isi SEMUA blok lama — sub-kunci
	// terkelola dibuang (ditulis ulang dari blok baru), sub-kunci
	// lain dipertahankan dan digabung. Baris kosong di EKOR hasil
	// gabungan adalah pemisah ke seksi berikutnya, bukan isi blok:
	// ia diregenerasi satu kali saat penyusunan di bawah agar
	// penggabungan idempoten.
	var dipertahankan []string
	for _, r := range rentang {
		if r.aliran != nil {
			// Blok flow map: isinya sudah diurai menjadi baris
			// blok tak-terkelola saat rentang dicari (B-482).
			dipertahankan = append(dipertahankan, r.aliran...)
			continue
		}
		dipertahankan = append(dipertahankan, saringIsiBlokHermes(baris[r.awal+1:r.akhir], terkelola)...)
	}
	// B-481: jangkar blok PERTAMA dipertahankan pada header blok
	// gabungan, supaya alias `*nama` di bagian lain dokumen tetap
	// menunjuk blok model yang diperbarui.
	//
	// B-506: yang dipertahankan adalah jangkar DISTINCT PERTAMA dari
	// blok MANA PUN — B-481 hanya melihat blok pertama, jadi jangkar
	// yang hanya ada di blok kedua hilang saat penggabungan, alias
	// `*nama`-nya menggantung, dan verifikasi menolak Configure
	// ("unknown anchor"). Batas yang disadari: satu node YAML hanya
	// bisa membawa SATU jangkar, jadi bila blok-blok membawa nama
	// jangkar yang BERBEDA-beda, hanya yang pertama yang bisa hidup
	// di header gabungan — jangkar lain menunjuk peta pra-gabung
	// yang berbeda-beda dan tidak punya rumah setia di satu node;
	// menulis ulang situs alias di teks pengguna (yang bisa berada
	// di dalam string/komentar) ditolak sebagai bedah yang tidak
	// aman. Kasus itu tetap gagal jujur di verifikasi, berkas utuh.
	jangkarGabungan := ""
	for _, r := range rentang {
		if r.jangkar != "" {
			jangkarGabungan = r.jangkar
			break
		}
	}
	if jangkarGabungan != "" {
		blok[0] = "model: &" + jangkarGabungan
	}
	for len(dipertahankan) > 0 && dipertahankan[len(dipertahankan)-1] == "" {
		dipertahankan = dipertahankan[:len(dipertahankan)-1]
	}
	// Susun ulang: blok gabungan menempati posisi blok PERTAMA;
	// blok-blok lain dibuang rentangnya; baris di luar blok
	// dipertahankan apa adanya.
	dalamRentang := make([]bool, len(baris))
	for _, r := range rentang {
		for k := r.awal; k < r.akhir; k++ {
			dalamRentang[k] = true
		}
	}
	hasil := make([]string, 0, len(baris)+len(blok))
	hasil = append(hasil, baris[:rentang[0].awal]...)
	hasil = append(hasil, blok...)
	hasil = append(hasil, dipertahankan...)
	sisaBaris := make([]string, 0, len(baris))
	for k := rentang[0].akhir; k < len(baris); k++ {
		if !dalamRentang[k] {
			sisaBaris = append(sisaBaris, baris[k])
		}
	}
	// Satu baris kosong pemisah sebelum isi berikutnya (bila blok
	// gabungan tidak di ekor dokumen dan pemisah belum ada).
	// B-552: penanda akhir dokumen BUKAN "isi berikutnya" — ia
	// penutup dokumen, dan cabang sisip-tanpa-blok menempelkan
	// blok tepat di atasnya tanpa pemisah; menyisipkan pemisah di
	// sini membuat Configure atas berkas berpenanda tidak
	// byte-idempoten (lari kedua menambah satu baris kosong,
	// lari ketiga baru stabil).
	adaIsiSesudah := false
	for _, l := range sisaBaris {
		if strings.TrimSpace(l) != "" {
			adaIsiSesudah = true
			break
		}
	}
	if adaIsiSesudah && len(sisaBaris) > 0 && sisaBaris[0] != "" &&
		!barisPenandaAkhirDokumen(sisaBaris[0]) {
		hasil = append(hasil, "")
	}
	hasil = append(hasil, sisaBaris...)
	return gabungBarisYAML(hasil)
}

// pisahJangkarHermes memisahkan awalan jangkar YAML (`&nama`)
// dari sisa header blok model Hermes (B-481). Nama jangkar sah
// terdiri dari huruf/angka/`-`/`_`; bentuk lain (mis. `&` saja,
// atau teks yang diawali `&` tetapi bukan jangkar) mengembalikan
// sisa apa adanya dengan jangkar kosong.
func pisahJangkarHermes(sisa string) (jangkar, inti string) {
	if !strings.HasPrefix(sisa, "&") {
		return "", sisa
	}
	token, rest := sisa, ""
	if spasi := strings.IndexAny(sisa, " \t"); spasi >= 0 {
		token, rest = sisa[:spasi], strings.TrimSpace(sisa[spasi:])
	}
	nama := token[1:]
	if nama == "" {
		return "", sisa
	}
	for i := 0; i < len(nama); i++ {
		c := nama[i]
		if !(c >= 'a' && c <= 'z' || c >= 'A' && c <= 'Z' || c >= '0' && c <= '9' || c == '-' || c == '_') {
			return "", sisa
		}
	}
	return nama, rest
}

// pindaiFlowHermes menjalankan fungsi fn atas setiap byte teks
// flow YAML yang berada di LUAR string terkutip, sambil melacak
// kedalaman kurung {}/[]; fn menerima byte dan kedalaman SEBELUM
// byte itu. String tanda-kutip-ganda menghormati escape backslash;
// tanda-kutip-tunggal menghormati penggandaan ” ala YAML.
func pindaiFlowHermes(teks string, fn func(c byte, kedalaman int)) {
	kedalaman := 0
	var kutip byte
	for i := 0; i < len(teks); i++ {
		c := teks[i]
		if kutip == '"' {
			if c == '\\' {
				i++
			} else if c == '"' {
				kutip = 0
			}
			continue
		}
		if kutip == '\'' {
			if c == '\'' {
				if i+1 < len(teks) && teks[i+1] == '\'' {
					i++
				} else {
					kutip = 0
				}
			}
			continue
		}
		switch c {
		case '"', '\'':
			kutip = c
		case '{', '[':
			fn(c, kedalaman)
			kedalaman++
		case '}', ']':
			kedalaman--
			fn(c, kedalaman)
		default:
			fn(c, kedalaman)
		}
	}
}

// flowMapHermesSeimbang melaporkan apakah teks flow yang diawali
// "{" sudah tertutup seimbang (kurung terluarnya kembali ke 0).
func flowMapHermesSeimbang(teks string) bool {
	seimbang := false
	pindaiFlowHermes(teks, func(c byte, kedalaman int) {
		if c == '}' && kedalaman == 0 {
			seimbang = true
		}
	})
	return seimbang
}

// pecahFlowMapHermes mengurai flow map YAML (`{k: v, ...}`) pada
// TINGKAT ATASNYA saja (B-482): entri dipisah pada koma kedalaman
// 0 di luar string, kunci dipisah dari nilai pada titik dua
// kedalaman 0 pertama. Entri tak-terkelola dikembalikan sebagai
// baris blok kanonis ("  kunci: nilai") dengan teks nilai apa
// adanya — termasuk nilai bersarang dan string berkoma — supaya
// tidak ada isi pengguna yang hilang. ok=false bila bentuknya
// tidak bisa diurai dengan aman (pemanggil membiarkan dokumen apa
// adanya dan verifikasi menolak secara jujur).
func pecahFlowMapHermes(teks string, terkelola map[string]bool) (baris []string, ok bool) {
	teks = strings.TrimSpace(teks)
	if !strings.HasPrefix(teks, "{") {
		return nil, false
	}
	// Cari kurung tutup pasangan pada kedalaman 0; sesudahnya
	// hanya boleh ada komentar.
	tutup := -1
	{
		kedalaman := 0
		var kutip byte
		for i := 0; i < len(teks) && tutup < 0; i++ {
			c := teks[i]
			if kutip == '"' {
				if c == '\\' {
					i++
				} else if c == '"' {
					kutip = 0
				}
				continue
			}
			if kutip == '\'' {
				if c == '\'' {
					if i+1 < len(teks) && teks[i+1] == '\'' {
						i++
					} else {
						kutip = 0
					}
				}
				continue
			}
			switch c {
			case '"', '\'':
				kutip = c
			case '{', '[':
				kedalaman++
			case '}', ']':
				kedalaman--
				if kedalaman == 0 {
					tutup = i
				}
			}
		}
	}
	if tutup < 0 {
		return nil, false
	}
	if sisa := strings.TrimSpace(teks[tutup+1:]); sisa != "" && !strings.HasPrefix(sisa, "#") {
		return nil, false
	}
	dalam := teks[1:tutup]
	// Pisah entri pada koma kedalaman 0.
	var entri []string
	{
		kedalaman := 0
		mulai := 0
		var kutip byte
		for i := 0; i < len(dalam); i++ {
			c := dalam[i]
			if kutip == '"' {
				if c == '\\' {
					i++
				} else if c == '"' {
					kutip = 0
				}
				continue
			}
			if kutip == '\'' {
				if c == '\'' {
					if i+1 < len(dalam) && dalam[i+1] == '\'' {
						i++
					} else {
						kutip = 0
					}
				}
				continue
			}
			switch c {
			case '"', '\'':
				kutip = c
			case '{', '[':
				kedalaman++
			case '}', ']':
				kedalaman--
			case ',':
				if kedalaman == 0 {
					entri = append(entri, dalam[mulai:i])
					mulai = i + 1
				}
			}
		}
		entri = append(entri, dalam[mulai:])
	}
	for _, e := range entri {
		e = strings.TrimSpace(e)
		if e == "" {
			continue // koma menggantung di akhir: sah, tanpa entri
		}
		idx := indeksTitikDuaFlow(e)
		if idx < 0 {
			return nil, false
		}
		kunciMentah := strings.TrimSpace(e[:idx])
		nilai := strings.TrimSpace(e[idx+1:])
		if kunciMentah == "" {
			return nil, false
		}
		if terkelola[normalkanKunciYAML(kunciMentah)] {
			continue // ditulis ulang dari blok kanonis
		}
		if nilai == "" {
			baris = append(baris, "  "+kunciMentah+":")
		} else {
			baris = append(baris, "  "+kunciMentah+": "+nilai)
		}
	}
	return baris, true
}

// indeksTitikDuaFlow mencari titik dua pemisah kunci/nilai pada
// kedalaman 0 di luar string dalam satu entri flow map; -1 bila
// tidak ada. Titik dua WAJIB diikuti spasi/akhir (aturan YAML
// untuk kunci polos) supaya nilai seperti URL "http://…" yang
// menjadi kunci aneh tidak terbelah di tengah.
func indeksTitikDuaFlow(e string) int {
	kedalaman := 0
	var kutip byte
	for i := 0; i < len(e); i++ {
		c := e[i]
		if kutip == '"' {
			if c == '\\' {
				i++
			} else if c == '"' {
				kutip = 0
			}
			continue
		}
		if kutip == '\'' {
			if c == '\'' {
				if i+1 < len(e) && e[i+1] == '\'' {
					i++
				} else {
					kutip = 0
				}
			}
			continue
		}
		switch c {
		case '"', '\'':
			kutip = c
		case '{', '[':
			kedalaman++
		case '}', ']':
			kedalaman--
		case ':':
			if kedalaman == 0 && (i+1 == len(e) || e[i+1] == ' ' || e[i+1] == '\t') {
				return i
			}
		}
	}
	return -1
}

// saringIsiBlokHermes menyaring isi satu blok `model:` Hermes
// (baris-baris sesudah header) untuk penggabungan B-222: sub-kunci
// terkelola dibuang bersama baris-baris bersarang di bawahnya,
// sub-kunci lain dipertahankan. Isi dipecah menjadi segmen per
// sub-kunci supaya duplikat sub-kunci tak-terkelola — termasuk dari
// blok kedua yang digabung B-441 — terlipat ke kemunculan TERAKHIR,
// sesuai bacaan last-wins parser YAML atas berkas asalnya.
//
// B-440: tingkat sub-kunci adalah indentasi sub-kunci PERTAMA blok
// itu, berapa pun lebarnya (2, 4, tab) — definisi lama "persis 2
// spasi atau 1 tab" membuat sub-kunci berindentasi 4 tidak dikenali
// sebagai terkelola, tertinggal sesudah blok baru berindentasi 2,
// dan keluaran menjadi YAML yang tidak terurai sama sekali.
func saringIsiBlokHermes(subbaris []string, terkelola map[string]bool) []string {
	type segmen struct {
		kunci string // "" untuk baris lepas sebelum sub-kunci pertama
		baris []string
	}
	// B-550: baris yang dimulai DI DALAM skalar terkutip multiline
	// adalah kelanjutan nilai sub-kunci yang membukanya — bukan
	// batas segmen, dan isinya tidak boleh digeser indentasinya.
	dalam := petaDalamSkalarYAML(subbaris)
	// Tingkat indentasi sub-kunci = indentasi baris isi (bukan
	// kosong/komentar) berindentasi pertama di blok ini.
	tingkat := -1
	for _, sub := range subbaris {
		trim := strings.TrimSpace(sub)
		if trim == "" || strings.HasPrefix(trim, "#") || !barisBerindentasiYAML(sub) {
			continue
		}
		tingkat = lebarIndentasiYAML(sub)
		break
	}
	var seg []segmen
	kini := -1 // indeks segmen aktif; -1 = belum ada
	for idxSub, sub := range subbaris {
		trim := strings.TrimSpace(sub)
		kunci := ""
		if !dalam[idxSub] && trim != "" && !strings.HasPrefix(trim, "#") && barisBerindentasiYAML(sub) &&
			tingkat >= 0 && lebarIndentasiYAML(sub) <= tingkat {
			kunci = kunciSubBarisYAML(sub)
		}
		// Baris komentar dan baris kosong masuk segmen LEPAS
		// tersendiri yang tidak pernah dibuang: menempelkannya ke
		// segmen sub-kunci sebelumnya akan membuang komentar
		// pengguna hanya karena sub-kunci di atasnya terkelola.
		longgar := trim == "" || strings.HasPrefix(trim, "#")
		if kunci != "" {
			seg = append(seg, segmen{kunci: kunci})
			kini = len(seg) - 1
		} else if kini < 0 || (longgar && seg[kini].kunci != "") {
			seg = append(seg, segmen{})
			kini = len(seg) - 1
		}
		barisKeluar := sub
		// Komentar kolom-0 di tengah blok dipertahankan isinya,
		// tetapi dalam keluaran ia WAJIB menjorok: ditulis di
		// kolom-0 ia akan mengakhiri pemetaan `model:` keluaran
		// dan baris-baris sesudahnya menjadi yatim tak terurai.
		if strings.HasPrefix(trim, "#") && !barisBerindentasiYAML(sub) {
			barisKeluar = "  " + trim
		}
		// B-440: sub-kunci yang dipertahankan ditulis ulang pada
		// indentasi kanonik blok baru (2 spasi), digeser sebesar
		// selisih tingkat blok lamanya — baris isi berindentasi 4
		// yang dipertahankan apa adanya di bawah blok baru
		// berindentasi 2 adalah YAML yang tidak terurai (parser
		// menolak campuran tingkat itu). Baris bersarang digeser
		// dengan delta yang sama sehingga struktur relatifnya
		// utuh; baris komentar tidak disentuh (inert di kedalaman
		// berapa pun).
		if !dalam[idxSub] && tingkat > 0 && tingkat != 2 && trim != "" && !strings.HasPrefix(trim, "#") &&
			lebarIndentasiYAML(sub) >= tingkat {
			barisKeluar = "  " + sub[tingkat:]
		}
		seg[kini].baris = append(seg[kini].baris, barisKeluar)
	}
	// Kemunculan terakhir tiap sub-kunci tak-terkelola yang menang;
	// segmen terkelola selalu dibuang.
	terakhir := map[string]int{}
	for i, s := range seg {
		if s.kunci != "" {
			terakhir[s.kunci] = i
		}
	}
	var hasil []string
	for i, s := range seg {
		if s.kunci != "" && (terkelola[s.kunci] || terakhir[s.kunci] != i) {
			continue
		}
		hasil = append(hasil, s.baris...)
	}
	return hasil
}

// barisBerindentasiYAML melaporkan apakah baris diawali spasi/tab.
func barisBerindentasiYAML(l string) bool {
	return strings.HasPrefix(l, " ") || strings.HasPrefix(l, "\t")
}

// lebarIndentasiYAML menghitung lebar indentasi awal baris dalam
// jumlah karakter spasi/tab.
func lebarIndentasiYAML(l string) int {
	n := 0
	for n < len(l) && (l[n] == ' ' || l[n] == '\t') {
		n++
	}
	return n
}

// kunciSubBarisYAML mengekstrak nama kunci dari baris sub-entri blok
// YAML ("<indentasi>nama: nilai" → "nama"), untuk indentasi berapa
// pun lebarnya. Penentuan apakah baris itu sub-kunci LANGSUNG blok
// (bukan kelanjutan nilai yang menjorok lebih dalam) adalah tugas
// pemanggil (saringIsiBlokHermes, yang membandingkan lebar
// indentasi dengan tingkat bloknya) — B-440: definisi lama di sini
// ("tepat 2 spasi atau 1 tab") membuat sub-kunci sah berindentasi 4
// tidak pernah dikenali sebagai sub-kunci. Baris kosong, komentar,
// item daftar, dan baris tanpa pola kunci mengembalikan "".
//
// Nama yang dikembalikan DINORMALKAN dari kutip luarnya (B-372):
// saringan terkelola di susunBlokModelHermes membandingkan nama ini
// dengan "provider"/"default" — subkunci terkutip lolos saringan
// dan tertinggal SESUDAH blok baru, dan pembaca YAML
// last-key-wins (PyYAML Hermes) membaca nilai LAMA sementara
// Configure melaporkan sukses (kelas sukses-bohong B-277).
func kunciSubBarisYAML(baris string) string {
	if !barisBerindentasiYAML(baris) {
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
	return normalkanKunciYAML(strings.TrimSpace(trim[:idx]))
}

// gabungBarisYAML menyatukan baris dan memastikan newline penutup.
func gabungBarisYAML(baris []string) string {
	keluaran := strings.Join(baris, "\n")
	if !strings.HasSuffix(keluaran, "\n") {
		keluaran += "\n"
	}
	return keluaran
}

// potongAwalanExport membuang awalan kata kunci `export` dari teks
// baris env yang sudah dipangkas, bila kata itu diikuti spasi ATAU
// tab (satu atau lebih). Nama yang diawali "export" tanpa pemisah
// (mis. variabel `exportX`) bukan awalan dan dibiarkan.
func potongAwalanExport(s string) string {
	if !strings.HasPrefix(s, "export") {
		return s
	}
	sisa := s[len("export"):]
	if sisa == "" || (sisa[0] != ' ' && sisa[0] != '\t') {
		return s
	}
	return strings.TrimSpace(sisa)
}

// namaVariabelEnv membaca nama variabel dari satu baris .env,
// toleran terhadap bentuk sah yang ditulis tangan (B-403):
// indentasi di awal baris, awalan "export " (spasi/tab), dan spasi
// di sekitar "=". B-442: nama TERKUTIP (`"OPENAI_API_KEY"=…` atau
// `'OPENAI_API_KEY'=…` — bentuk yang dibaca parser dotenv seperti
// python-dotenv) dinormalkan dengan membuang pasangan kutip
// luarnya, sejajar perlakuan kunci YAML (B-372) dan TOML (B-343) di
// berkas ini; versi lama tidak mengenalinya sehingga baris lama
// tertinggal dan variabel terduplikasi. Mengembalikan "" untuk
// baris yang bukan penetapan variabel (komentar, baris kosong,
// tanpa "=").
func namaVariabelEnv(l string) string {
	s := potongAwalanExport(strings.TrimSpace(l))
	idx := strings.Index(s, "=")
	if idx < 0 {
		return ""
	}
	nama := strings.TrimSpace(s[:idx])
	if len(nama) >= 2 {
		if (nama[0] == '"' && nama[len(nama)-1] == '"') ||
			(nama[0] == '\'' && nama[len(nama)-1] == '\'') {
			nama = strings.TrimSpace(nama[1 : len(nama)-1])
		}
	}
	return nama
}

// pecahBarisEnv membelah satu baris .env/skrip menjadi nama dan
// nilai variabelnya memakai pengenalan yang sama seperti
// namaVariabelEnv (awalan export spasi/tab, nama terkutip, spasi di
// sekitar "="). Dipakai fungsi redaksi/pembersihan skrip supaya
// bentuk tulis yang dikenali di SATU tempat dikenali di semua
// tempat (B-444). ok=false untuk baris bukan penetapan variabel.
func pecahBarisEnv(l string) (nama, nilai string, ok bool) {
	s := potongAwalanExport(strings.TrimSpace(l))
	idx := strings.Index(s, "=")
	if idx < 0 {
		return "", "", false
	}
	nama = namaVariabelEnv(l)
	if nama == "" {
		return "", "", false
	}
	return nama, strings.TrimSpace(s[idx+1:]), true
}

// perbaruiBarisEnv menetapkan satu variabel di konten berkas .env
// (baris NAMA=nilai), mengganti baris lama atau menambahkan di akhir;
// variabel lain dipertahankan.
func perbaruiBarisEnv(existing, nama, nilai string) string {
	// B-374: BOM di awal berkas (penyunting Windows) dibuang lebih
	// dulu, sejajar jalur TOML (B-310) dan YAML (B-344): BOM
	// menempel pada nama variabel pertama sehingga awalan NAMA=
	// tidak pernah cocok — baris lama tertinggal dan salinan baru
	// ditambahkan, meninggalkan variabel ganda yang bacaan
	// efektifnya bergantung parser dotenv.
	existing = strings.TrimPrefix(existing, "\ufeff")
	baris := strings.Split(existing, "\n")
	tertulis := false
	// B-379: saringan akhir dulu membuang SETIAP baris kosong dari
	// berkas yang sudah memuat variabelnya — termasuk baris kosong
	// pemisah antargrup yang sengaja ditulis pengguna. Yang wajib
	// dibuang hanya baris yang dikosongkan fungsi ini sendiri
	// sebagai duplikat; catat indeksnya.
	dibuang := make(map[int]bool)
	for i, l := range baris {
		// B-403: pencocokan lewat namaVariabelEnv, bukan awalan
		// mentah NAMA= — bentuk sah yang ditulis tangan (indentasi,
		// awalan "export ", spasi di sekitar "=") dulu tidak pernah
		// cocok, jadi baris lama tertinggal dan salinan kanonik
		// ditambahkan: variabel ganda yang bacaan efektifnya
		// bergantung parser dotenv. Baris yang cocok diganti
		// bentuk kanoniknya.
		if namaVariabelEnv(l) == nama {
			if tertulis {
				baris[i] = ""
				dibuang[i] = true
				continue
			}
			baris[i] = nama + "=" + nilai
			tertulis = true
		}
	}
	hasil := make([]string, 0, len(baris)+1)
	for i, l := range baris {
		if dibuang[i] {
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

// --- Verifikasi hasil akhir Configure (B-439…B-442) ---
//
// Kelas bug B-439/B-440/B-441 berbagi satu watak: penyunting teks
// per-baris melaporkan sukses atas bentuk sah yang tidak ia
// kenali. Perbaikan per-bentuk saja tidak menutup kelasnya, jadi
// setiap berkas TOML/YAML/.env yang disunting Configure kini
// diverifikasi HASIL AKHIRNYA di titik bersama ini sebelum ditulis:
// konten final wajib terurai dengan parser sungguhan (pelletier
// go-toml untuk TOML, go.yaml.in/yaml/v3 untuk YAML — keduanya
// dependensi yang sudah dipakai repo) DAN kunci terkelola yang
// diniatkan wajib terbaca efektif dengan nilai yang diniatkan.
// Kegagalan verifikasi mengembalikan galat dari
// applyToolSpecificConfig, dan Configure memulihkan seluruh
// jepretan berkasnya (pulihkanSemua) — tidak ada lagi sukses atas
// hasil yang tidak terbukti.

func verifikasiHasilTOML(konten string, preamble [][2]string, seksi map[string][][2]string) error {
	var doc map[string]any
	if err := toml.Unmarshal([]byte(konten), &doc); err != nil {
		return fmt.Errorf("hasil sunting TOML tidak terurai: %w", err)
	}
	for _, p := range preamble {
		if v, ok := doc[p[0]].(string); !ok || v != p[1] {
			return fmt.Errorf("hasil sunting TOML: kunci tingkat-atas %q tidak efektif (terbaca %v)", p[0], doc[p[0]])
		}
	}
	for namaSeksi, pairs := range seksi {
		tbl := doc
		for _, seg := range strings.Split(namaSeksi, ".") {
			anak, ok := tbl[seg].(map[string]any)
			if !ok {
				return fmt.Errorf("hasil sunting TOML: seksi %q tidak efektif", namaSeksi)
			}
			tbl = anak
		}
		for _, p := range pairs {
			if v, ok := tbl[p[0]].(string); !ok || v != p[1] {
				return fmt.Errorf("hasil sunting TOML: kunci %q di seksi %q tidak efektif (terbaca %v)", p[0], namaSeksi, tbl[p[0]])
			}
		}
	}
	return nil
}

// uraiYAMLUtuh mengurai seluruh dokumen dalam konten YAML; galat
// urai apa pun — termasuk kunci ganda yang ditolak parser —
// dikembalikan apa adanya.
func uraiYAMLUtuh(konten string) (map[string]any, error) {
	dec := yaml.NewDecoder(strings.NewReader(konten))
	var pertama map[string]any
	for {
		var doc map[string]any
		err := dec.Decode(&doc)
		if errors.Is(err, io.EOF) {
			break
		}
		if err != nil {
			return nil, err
		}
		if pertama == nil {
			pertama = doc
		}
	}
	return pertama, nil
}

func verifikasiHasilYAMLTingkatAtas(konten string, pairs [][2]string) error {
	doc, err := uraiYAMLUtuh(konten)
	if err != nil {
		// B-560: galat duplikasi atas kunci TERKELOLA yang
		// disertai kutip tak tertutup di isi skalar blok TIDAK
		// boleh menamai duplikasi sebagai sebab — akarnya
		// adalah skalar semu yang menelan struktur asli.
		// Duplikasi kunci TAK-terkelola yang tulen tetap
		// dilaporkan apa adanya (di sana duplikasi memang
		// sebabnya).
		if sebab := diagnosisKutipSkalarBlok(konten, pairs, err); sebab != "" {
			return fmt.Errorf("hasil sunting YAML: %s", sebab)
		}
		return fmt.Errorf("hasil sunting YAML tidak terurai: %w", err)
	}
	for _, p := range pairs {
		if v, ok := doc[p[0]].(string); !ok || v != p[1] {
			return fmt.Errorf("hasil sunting YAML: kunci tingkat-atas %q tidak efektif (terbaca %v)", p[0], doc[p[0]])
		}
	}
	return nil
}

// diagnosisKutipSkalarBlok memeriksa apakah galat urai hasil
// sunting adalah duplikasi kunci TERKELOLA yang disebabkan kutip
// tak tertutup di isi skalar blok (B-560) — bukan duplikasi di
// berkas masukan: penyunting membuang duplikat terkelola yang
// terlihat, jadi kunci terkelola yang tertulis dua kali berarti
// penyunting salah membaca struktur berkas. Mengembalikan pesan
// sebab-akar, atau "" bila diagnosis tidak berlaku (galat bukan
// duplikasi, kunci yang ganda bukan terkelola, atau tidak ada
// kutip blok yang terbukti menelan baris kunci itu — pesan
// duplikasi apa adanya dari pemanggil tetap jujur di sana).
func diagnosisKutipSkalarBlok(konten string, pairs [][2]string, errUrai error) string {
	kunci := kunciGandaDariGalatYAML(errUrai)
	if kunci == "" {
		return ""
	}
	terkelola := false
	for _, p := range pairs {
		if p[0] == kunci {
			terkelola = true
			break
		}
	}
	if !terkelola {
		return ""
	}
	if pembuka, ok := cariKutipBlokMenelanKunciYAML(konten, kunci); ok {
		return fmt.Sprintf("kunci terkelola %q tertulis dua kali karena kutip tak tertutup di isi skalar blok (%q) membuat penyunting salah membaca struktur — bukan duplikasi di berkas masukan; tutup kutip itu, lalu coba lagi", kunci, pembuka)
	}
	return fmt.Sprintf("kunci terkelola %q tertulis dua kali karena penyunting salah membaca struktur berkas — bukan duplikasi di berkas masukan (penyunting membuang duplikat terkelola yang terlihat); periksa struktur di sekitar kunci itu, lalu coba lagi", kunci)
}

// kunciGandaDariGalatYAML mengambil nama kunci dari galat
// duplikasi yaml.v3 ("mapping key \"X\" already defined").
// "" bila galatnya bukan duplikasi.
func kunciGandaDariGalatYAML(err error) string {
	if err == nil {
		return ""
	}
	pesan := err.Error()
	awalan := `mapping key "`
	i := strings.Index(pesan, awalan)
	if i < 0 {
		return ""
	}
	sisa := pesan[i+len(awalan):]
	j := strings.Index(sisa, `"`)
	if j < 0 {
		return ""
	}
	return sisa[:j]
}

// kunciKolomNolYAML mengekstrak nama kunci dari baris kolom-0
// "nama: ..." ("" bila bukan baris kunci kolom-0).
func kunciKolomNolYAML(line string) string {
	if line == "" || line[0] == ' ' || line[0] == '\t' || line[0] == '#' {
		return ""
	}
	idx := strings.Index(line, ":")
	if idx <= 0 {
		return ""
	}
	return normalkanKunciYAML(strings.TrimSpace(line[:idx]))
}

// cariKutipBlokMenelanKunciYAML membuktikan mekanisme B-560 atas
// teks hasil sunting: ia mensimulasikan pelacak NAIF pra-B-560
// (kutip di isi skalar blok pun membuka skalar) dan melaporkan
// baris pembuka skalar semu itu — HANYA bila skalar semu
// tersebut masih terbuka saat baris kunci terkelola yang ganda
// tercapai, yaitu bila ia terbukti menelan baris itu. Batas
// ketepatan: simulasi memakai pengenal pembuka blok yang sama
// dengan pelacak (awalSkalarBlokYAML); bentuk pembuka yang tidak
// dikenali pelacak tidak terdiagnosis di sini dan jatuh ke pesan
// umum diagnosisKutipSkalarBlok.
func cariKutipBlokMenelanKunciYAML(konten, kunci string) (pembuka string, ok bool) {
	induk := -1 // indentasi induk skalar blok (keadaan BENAR)
	var kutipBenar byte
	var naif byte // keadaan kutip NAIF: isi blok pun membuka
	pembukaAktif := ""
	for _, line := range strings.Split(konten, "\n") {
		// 1. Status isi-blok menurut keadaan BENAR (kutip di
		// isi blok tidak membuka; baris kosong/lebih menjorok
		// adalah isi, baris lain mengakhiri blok).
		isiBlok := false
		if kutipBenar == 0 && induk >= 0 {
			if strings.TrimSpace(line) == "" || lebarIndentasiYAML(line) > induk {
				isiBlok = true
			} else {
				induk = -1
			}
		}
		// 2. Skalar semu NAIF: pindai baris ini tanpa
		// pengecualian isi blok.
		naifSebelum := naif
		naif = pindaiKutipBarisYAML(line, naif)
		if isiBlok {
			if naifSebelum == 0 && naif != 0 {
				pembukaAktif = strings.TrimSpace(line)
			} else if naif == 0 {
				pembukaAktif = ""
			}
		} else if naif == 0 {
			pembukaAktif = ""
		}
		// 3. Baris kunci terkelola yang ditelan skalar semu?
		if !isiBlok && naifSebelum != 0 && pembukaAktif != "" && kunciKolomNolYAML(line) == kunci {
			return pembukaAktif, true
		}
		// 4. Perbarui keadaan BENAR (bukan untuk baris isi blok).
		if !isiBlok {
			kutipBenar = pindaiKutipBarisYAML(line, kutipBenar)
			if kutipBenar == 0 {
				if i, buka := awalSkalarBlokYAML(line); buka {
					induk = i
				}
			}
		}
	}
	return "", false
}

// pindaiKutipBarisYAML memindai satu baris untuk keadaan skalar
// terkutip dan mengembalikan keadaan di akhir baris — aturan
// yang sama dengan pelacakSkalarYAML.lintasi, tanpa keadaan
// skalar blok (dipakai diagnosis B-560: simulasi naif dan
// pelacakan keadaan benar di dalam pemindai diagnosis).
func pindaiKutipBarisYAML(line string, kutip byte) byte {
	for i := 0; i < len(line); i++ {
		c := line[i]
		switch kutip {
		case '"':
			if c == '\\' {
				i++
			} else if c == '"' {
				kutip = 0
			}
			continue
		case '\'':
			if c == '\'' {
				if i+1 < len(line) && line[i+1] == '\'' {
					i++
				} else {
					kutip = 0
				}
			}
			continue
		}
		if c == '#' && (i == 0 || line[i-1] == ' ' || line[i-1] == '\t') {
			return kutip
		}
		if c == '"' || c == '\'' {
			j := i - 1
			for j >= 0 && (line[j] == ' ' || line[j] == '\t') {
				j--
			}
			if j < 0 || line[j] == ':' || line[j] == '-' || line[j] == '[' || line[j] == '{' || line[j] == ',' {
				kutip = c
			}
		}
	}
	return kutip
}

// verifikasiHasilHermes memeriksa blok `model:` hasil susunan
// Hermes: dokumen wajib terurai dan sub-kunci terkelola wajib
// terbaca efektif dari blok itu. Penguraiannya memakai toleransi
// parser tujuan Hermes (uraiYAMLHermes, B-483) — BUKAN pengurai
// ketat bersama, karena PyYAML menerima kunci ganda di luar blok
// terkelola sementara yaml.v3 menolaknya: menolak di sini berarti
// Configure gagal atas berkas yang dibaca Hermes dengan benar.
// B-571: toleransi itu hanya berlaku DI DALAM dokumen pertama —
// ekor sesudahnya diperiksa ketat oleh uraiYAMLHermes (dokumen
// tambahan atau galat urai di ekor adalah galat), jadi
// verifikasi dokumen-pertama tidak lagi menjadi batas yang
// membiarkan isi tersembunyi sesudah "..." lolos.
func verifikasiHasilHermes(konten, target, gwURL, apiKey string) error {
	doc, err := uraiYAMLHermes(konten)
	if err != nil {
		return fmt.Errorf("hasil sunting YAML Hermes tidak terurai: %w", err)
	}
	blok, ok := doc["model"].(map[string]any)
	if !ok {
		return fmt.Errorf("hasil sunting YAML Hermes: blok model tidak efektif")
	}
	mau := [][2]string{
		{"provider", "custom"},
		{"default", target},
		{"base_url", gwURL},
		{"api_mode", "chat_completions"},
	}
	if apiKey != "" {
		mau = append(mau, [2]string{"api_key", apiKey})
	}
	for _, p := range mau {
		if v, ok := blok[p[0]].(string); !ok || v != p[1] {
			return fmt.Errorf("hasil sunting YAML Hermes: sub-kunci %q tidak efektif (terbaca %v)", p[0], blok[p[0]])
		}
	}
	return nil
}

// deteksiBOMNonUTF8 mengenali tanda urutan byte (BOM) pengkodean
// non-UTF-8 di awal berkas (B-477): UTF-16LE/BE dan UTF-32LE/BE.
// Mengembalikan nama pengkodeannya, atau "" bila tidak ada — BOM
// UTF-8 BUKAN temuan fungsi ini (ia artefak penyunting yang
// ditangani jalur B-310/B-344/B-374 dengan membuangnya).
func deteksiBOMNonUTF8(data []byte) string {
	switch {
	case len(data) >= 4 && data[0] == 0xFF && data[1] == 0xFE && data[2] == 0x00 && data[3] == 0x00:
		return "UTF-32LE"
	case len(data) >= 4 && data[0] == 0x00 && data[1] == 0x00 && data[2] == 0xFE && data[3] == 0xFF:
		return "UTF-32BE"
	case len(data) >= 2 && data[0] == 0xFF && data[1] == 0xFE:
		return "UTF-16LE"
	case len(data) >= 2 && data[0] == 0xFE && data[1] == 0xFF:
		return "UTF-16BE"
	}
	return ""
}

// deteksiUTF16TanpaBOM mengenali berkas UTF-16 yang TIDAK membawa
// BOM (B-522): teks yang hurufnya mayoritas Latin/ASCII terkode
// UTF-16LE menaruh NUL di posisi GANJIL tiap pasangan byte,
// UTF-16BE di posisi GENAP. deteksiBOMNonUTF8 tidak melihat berkas
// seperti ini sama sekali, dan isinya lalu terbaca sebagai "teks
// UTF-8" yang nama terkelolanya tak terbaca.
//
// Dua jalur bukti berjalan berurutan:
//
//  1. Jalur klasik B-531 (perilaku terkunci test): setiap BYTE NUL
//     memilih paritasnya sendiri di SELURUH data — karakter U+xx00
//     menyumbang tepat satu suara lawan (tidak lagi veto seperti
//     klasifikasi per-pasangan versi pertama), karakter U+0000
//     menyumbang satu suara ke dua paritas (menetralkan dirinya),
//     dan karakter polos di atas U+00FF tidak memilih. Vonis per
//     orientasi menuntut bukti minimal 4 byte NUL pada paritasnya,
//     dominasi ≥2:1 atas paritas lawan, dan kerapatan ≥1/4 dari
//     seluruh pasangan.
//
//  2. Jalur proporsional B-541/B-547 (klasifikasi hasil DEKODE per
//     orientasi, lihat nilaiOrientasiUTF16): jalur klasik gugur
//     pada berkas yang BUKTI strukturalnya kuat tetapi suara
//     NUL-nya encer — berkas pendek yang hanya mengumpulkan 3
//     suara, dan nilai yang didominasi karakter astral (emoji,
//     CJK ekstensi) yang dalam hitungan byte mentah terbaca
//     sebagai suara lawan atau mengencerkan kerapatan di bawah
//     1/4. Klasifikasi dekode menetralkan keduanya: pasangan
//     pengganti (surrogate) yang sah dinilai NETRAL sebagai satu
//     karakter astral, bukan dua suara mentah. Orientasi menyala
//     bila dekodenya memuat ≥3 karakter Latin/struktural
//     termasuk '=', dan karakter Latinnya mendominasi >2:1 atas
//     karakter U+xx00 DAN atas karakter cacat (B-563: cacat
//     proporsional, bukan lagi syarat bebas-cacat) — bukti
//     berbentuk teks .env, bukan ambang suara mentah.
//
// KUALIFIKASI AMBANG (koreksi redaksi atas klaim versi B-531 yang
// menyatakan teks Latin "selalu" memenuhi jalur klasik — klaim
// itu GUGUR di Putaran Bersih #11): berkas yang bukti struktural
// Latinnya kurang dari 3 karakter, yang tidak memuat '=', yang
// rasio Latin:anti-nya TIDAK melebihi 2:1 (batasnya ketat: tepat
// 2:1, 5:3, atau lebih rendah semuanya lolos — bukan hanya dua
// rasio contoh itu), yang cacatnya mencapai separuh Latinnya
// (B-563: cacat kini proporsional, bukan veto — satu cacat di
// antara ≥3 Latin tidak memveto, tetapi dominasi cacat tetap
// menggugurkan), atau yang pasangan NUL-gandanya rapat (≥1/4)
// tetap LOLOS dari kedua jalur — itu batas bukti yang disengaja,
// bukan janji deteksi total. BATAS CAMPURAN ORIENTASI (B-564,
// keputusan induk: batas sah): janji detektor mencakup berkas
// yang ADALAH satu pengkodean (termasuk yang salah label),
// BUKAN sambungan byte dua pengkodean — sambungan seimbang
// UTF-16LE + UTF-16BE (dan urutan baliknya) menghasilkan dekode
// simetris di kedua orientasi dan LOLOS dari semua jalur;
// berkas seperti itu bukan pengkodean sah mana pun dan tak
// dihasilkan penyunting mana pun. Label orientasi (B-556)
// diambil dari klasifikasi dekode: label klasik hanya
// dipertahankan bila dekode orientasinya masuk akal sebagai
// teks .env; bila hanya orientasi lawan yang masuk akal, label
// lawan yang dipakai (berkas LE yang didominasi U+0100 dulu
// berlabel BE karena byte mentahnya meniru paritas BE); bila
// keduanya tidak masuk akal, label netral "UTF-16" — vonisnya
// tetap penolakan, hanya nama orientasinya yang tidak dikarang.
// Label netral itu dalam praktik SELALU lahir dari jalur klasik
// (B-556), bukan dari cabang "kedua orientasi layak" di bawah —
// cabang itu cabang mati yang dipertahankan (lihat catatannya):
// satu unit Latin di satu orientasi selalu anti di orientasi
// lawan, sehingga kedua orientasi tak pernah layak bersamaan.
//
// Gerbang arah sebaliknya (toleransi B-507) tetap mutlak: berkas
// UTF-8 sah yang memuat NUL TERISOLASI/TERSEBAR di baris
// tak-terkelola tidak boleh tertolak. NUL tersebar tidak pernah
// mencapai bukti minimal + dominasi + kerapatan jalur klasik, dan
// dekode orientasi mana pun atasnya memuat karakter cacat atau
// kekurangan bukti Latin/'=' untuk jalur proporsional; satu pola
// UTF-8 yang tertolak hanyalah NUL rapat satu-paritas yang
// bit-per-bit identik dengan UTF-16 asli — tak terbedakan oleh
// detektor pola-NUL mana pun, dan terbaca sebagai kunci sampah
// oleh python-dotenv juga. Pasangan NUL-ganda yang RAPAT (≥1/4
// pasangan) menandai pola UTF-32 tanpa BOM atau biner padat NUL,
// bukan UTF-16 teks — di luar vonis fungsi ini (UTF-32 dinilai
// deteksiUTF32TanpaBOM; bentuk ber-BOM tertangkap
// deteksiBOMNonUTF8).
func deteksiUTF16TanpaBOM(data []byte) string {
	n := len(data) / 2 // pasangan byte utuh yang dinilai
	if n < 4 {
		return ""
	}
	var nulGanjil, nulGenap, ganda int
	for i := 0; i < n; i++ {
		a, b := data[2*i], data[2*i+1]
		if a == 0 {
			nulGenap++
		}
		if b == 0 {
			nulGanjil++
		}
		if a == 0 && b == 0 {
			ganda++
		}
	}
	if ganda*4 >= n {
		return ""
	}
	// Jalur proporsional didahulukan: buktinya berbentuk teks
	// hasil dekode, lebih spesifik daripada suara byte mentah,
	// dan labelnya selalu orientasi yang terbukti dari dekode.
	le := nilaiOrientasiUTF16(data, n, true)
	be := nilaiOrientasiUTF16(data, n, false)
	// Cabang pertama switch di bawah adalah CABANG MATI yang
	// DIPERTAHANKAN dengan sengaja (koreksi redaksi B-541):
	// kedua orientasi tak pernah layak bersamaan — setiap unit
	// Latin di satu orientasi adalah unit anti (U+xx00) di
	// orientasi lawan, jadi anti_lawan ≥ latin; layak di kedua
	// sisi sekaligus menuntut latin > 2×anti ≥ 2×latin_lawan >
	// 4×latin, mustahil untuk latin ≥ 3. Sapuan mekanis
	// Fase 25 (fase25_prc_test.go) menegaskan tidak satu pun
	// bentuk yang disapu memenuhi kedua sisi bersamaan. Cabang
	// ini dipertahankan sebagai jangkar niat (bila keduanya
	// layak, labelnya netral) dan TIDAK dihapus: penghapusan
	// tidak mengubah perilaku apa pun, sementara keberadaannya
	// mendokumentasikan vonis simetris yang dituju.
	switch {
	case le.layak() && be.layak():
		return "UTF-16"
	case le.layak():
		return "UTF-16LE"
	case be.layak():
		return "UTF-16BE"
	}
	// NUL di posisi ganjil = byte tinggi UTF-16LE; genap = BE.
	klasik := ""
	if nulGanjil >= 4 && nulGanjil >= 2*nulGenap+1 && nulGanjil*4 >= n {
		klasik = "UTF-16LE"
	} else if nulGenap >= 4 && nulGenap >= 2*nulGanjil+1 && nulGenap*4 >= n {
		klasik = "UTF-16BE"
	}
	if klasik == "" {
		return ""
	}
	// B-556: label klasik dari suara mentah bisa menunjuk
	// orientasi yang salah (dominasi karakter di atas U+00FF
	// membalik paritas suara). Pertahankan hanya bila dekode
	// orientasinya masuk akal; bila hanya lawan yang masuk akal,
	// pakai label lawan; bila tidak ada, label netral.
	masukAkalKlasik := (klasik == "UTF-16LE" && le.masukAkal()) || (klasik == "UTF-16BE" && be.masukAkal())
	if masukAkalKlasik {
		return klasik
	}
	if klasik == "UTF-16LE" && be.masukAkal() {
		return "UTF-16BE"
	}
	if klasik == "UTF-16BE" && le.masukAkal() {
		return "UTF-16LE"
	}
	return "UTF-16"
}

// penilaianOrientasiUTF16 adalah hasil klasifikasi unit-unit
// hasil dekode satu orientasi (LE atau BE) atas data yang sama.
type penilaianOrientasiUTF16 struct {
	latin         int  // ASCII tercetak + \t\r\n + Latin-1 (U+00A0–U+00FF)
	anti          int  // U+xx00: dalam orientasi lawan terbaca sebagai NUL paritas lawan
	cacat         int  // kontrol non-struktural + surrogate tak berpasangan
	adaSamaDengan bool // '=' struktural .env hadir dalam dekode
}

// masukAkal: dekode orientasi ini menyerupai teks .env —
// memuat bukti Latin struktural termasuk '=', dan karakter
// Latinnya mendominasi >2:1 atas karakter cacat (B-563).
// Syarat cacat==0 versi lama adalah veto satu karakter: satu
// U+0085 (NEL) atau satu surrogate yatim di EOF menggugurkan
// vonis atas berkas yang selebihnya teks .env sah. Kini cacat
// dinilai proporsional seperti kelas Anti (Fase 23): satu cacat
// di antara ≥3 karakter Latin tidak lagi memveto; dekode yang
// cacatnya mencapai separuh Latinnya tetap tidak masuk akal.
func (p penilaianOrientasiUTF16) masukAkal() bool {
	return p.latin >= 3 && p.adaSamaDengan && p.latin > 2*p.cacat
}

// layak: masuk akal DAN karakter Latin mendominasi >2:1 atas
// karakter U+xx00 — syarat vonis jalur proporsional (B-541/B-547).
func (p penilaianOrientasiUTF16) layak() bool {
	return p.masukAkal() && p.latin > 2*p.anti
}

// nilaiOrientasiUTF16 mendekode n pasangan byte sebagai unit
// UTF-16 orientasi yang diminta dan mengklasifikasikan tiap unit.
// Pasangan surrogate yang sah (satu karakter astral) dihitung
// NETRAL dan memakan dua unit: dalam hitungan byte mentah jalur
// klasik, unit rendah surrogate (U+DC00–U+DFFF) terbaca sebagai
// suara lawan dan unit tingginya mengencerkan kerapatan —
// keduanya artefak pengkodean, bukan bukti orientasi.
func nilaiOrientasiUTF16(data []byte, n int, le bool) penilaianOrientasiUTF16 {
	unit := func(i int) uint16 {
		if le {
			return uint16(data[2*i]) | uint16(data[2*i+1])<<8
		}
		return uint16(data[2*i])<<8 | uint16(data[2*i+1])
	}
	var p penilaianOrientasiUTF16
	for i := 0; i < n; i++ {
		u := unit(i)
		switch {
		case u >= 0xD800 && u <= 0xDBFF:
			// Surrogate tinggi: sah hanya berpasangan dengan
			// surrogate rendah tepat sesudahnya.
			if i+1 < n {
				if v := unit(i + 1); v >= 0xDC00 && v <= 0xDFFF {
					i++ // karakter astral: netral, dua unit
					continue
				}
			}
			p.cacat++
		case u >= 0xDC00 && u <= 0xDFFF:
			p.cacat++ // surrogate rendah yatim
		case u == '=':
			p.latin++
			p.adaSamaDengan = true
		case u >= 0x20 && u <= 0x7E, u == '\t', u == '\r', u == '\n', u >= 0xA0 && u <= 0xFF:
			p.latin++
		case u >= 0x100 && u&0xFF == 0:
			p.anti++
		case u >= 0x01 && u <= 0x08, u == 0x0B, u == 0x0C, u >= 0x0E && u <= 0x1F, u >= 0x7F && u <= 0x9F:
			p.cacat++
		default:
			// Netral: U+0000 dan karakter BMP lain di atas U+00FF
			// yang bukan U+xx00 (CJK, simbol, dsb.) — sah dalam
			// teks .env asing, tidak memilih orientasi.
		}
	}
	return p
}

// deteksiUTF32TanpaBOM mengenali berkas UTF-32 yang TIDAK membawa
// BOM dari pola NUL per-4-byte-nya (B-548): karakter Latin/ASCII
// terkode UTF-32LE menaruh NUL pada residu byte 1, 2, dan 3 tiap
// grup 4 byte (residu 3 SELALU NUL — titik kode di atas U+FFFFFF
// tidak ada), UTF-32BE simetris pada residu 0, 1, 2 (residu 0
// selalu NUL). deteksiUTF16TanpaBOM sengaja melepaskan pola ini
// lewat penjaga NUL-ganda rapatnya. Syarat residu pendamping
// (≥3/4 grup "terjelaskan") menuntut byte kedua-tertinggi juga
// mayoritas terjelaskan — teks Latin memenuhinya (titik kode
// < U+10000); pola UTF-16 yang dibaca per-4-byte tidak:
// UTF-16LE ASCII menaruh NUL hanya pada separuh residu yang
// dituntut UTF-32LE, UTF-16BE simetris, dan UTF-8 realistis
// tidak pernah menaruh NUL pada SEMUA grup di satu residu pun.
// Grup TERJELASKAN bila residu pendampingnya NUL (karakter BMP)
// ATAU membentuk skalar astral sah (B-562): karakter astral
// (≥ U+10000) menaruh byte bukan-NUL pada residu pendamping —
// byte itu nilai byte kedua-tertinggi skalarnya sendiri, bukan
// bukti melawan UTF-32. LE: byte2 0x01–0x10 dengan byte3 NUL
// (nilai 0x10000–0x10FFFF); BE simetris byte1 0x01–0x10 dengan
// byte0 NUL. Tanpa penghitungan ini berkas UTF-32 yang
// fraksi astralnya di atas 1/4 lolos (residu pendamping jatuh
// di bawah ambang 3/4). Mengembalikan nama pengkodean, label
// netral "UTF-32" bila kedua orientasi menyala (pola simetris
// karangan), atau "" bila bukan UTF-32 tanpa BOM.
func deteksiUTF32TanpaBOM(data []byte) string {
	g := len(data) / 4 // grup 4 byte utuh yang dinilai
	if g < 4 {
		return ""
	}
	var nol [4]int
	var astralLE, astralBE int // grup yang residu pendampingnya
	// membentuk skalar astral sah (B-562), bukan NUL
	for i := 0; i < g; i++ {
		for r := 0; r < 4; r++ {
			if data[4*i+r] == 0 {
				nol[r]++
			}
		}
		if data[4*i+3] == 0 && data[4*i+2] >= 0x01 && data[4*i+2] <= 0x10 {
			astralLE++
		}
		if data[4*i] == 0 && data[4*i+1] >= 0x01 && data[4*i+1] <= 0x10 {
			astralBE++
		}
	}
	le := nol[3] == g && (nol[2]+astralLE)*4 >= 3*g
	be := nol[0] == g && (nol[1]+astralBE)*4 >= 3*g
	switch {
	case le && be:
		return "UTF-32"
	case le:
		return "UTF-32LE"
	case be:
		return "UTF-32BE"
	}
	return ""
}

// pecahBarisYAML memecah teks pada pemisah garis YAML (B-571):
// LF, CRLF (satu pemisah), CR saja, dan pemisah Unicode NEL
// (U+0085), LINE SEPARATOR (U+2028), PARAGRAPH SEPARATOR
// (U+2029) — semuanya pemisah garis menurut spesifikasi YAML
// dan diperlakukan demikian oleh PyYAML maupun yaml.v3.
// strings.Split pada "\n" saja buta terhadap pemisah-pemisah
// ini: satu "baris-\n" dapat memuat dua baris YAML, sehingga
// pemindai berbasis baris membaca isi sesudah penanda sebagai
// komentar padahal pemuat melihat dokumen tambahan (kelas
// sukses-bohong B-571). Hasilnya meniru strings.Split pada
// "\n": pemisah akhir menghasilkan elemen kosong penutup, dan
// teks tanpa pemisah menghasilkan satu elemen utuh.
func pecahBarisYAML(teks string) []string {
	var baris []string
	awal := 0
	for i := 0; i < len(teks); {
		r, lebar := utf8.DecodeRuneInString(teks[i:])
		switch r {
		case '\r':
			baris = append(baris, teks[awal:i])
			i += lebar
			if i < len(teks) && teks[i] == '\n' {
				i++
			}
			awal = i
		case '\n', '\u0085', '\u2028', '\u2029':
			baris = append(baris, teks[awal:i])
			i += lebar
			awal = i
		default:
			i += lebar
		}
	}
	baris = append(baris, teks[awal:])
	return baris
}

// tolakIsiSesudahPenandaHermes menolak config Hermes yang memuat
// isi non-kosong non-komentar SESUDAH penanda akhir dokumen "..."
// (B-543): isi itu berada di luar dokumen yang disusun penyunting
// dan hanya diverifikasi dari dokumen pertamanya, tetapi PyYAML
// safe_load — pemuat Hermes — menolak aliran yang memuat dokumen
// tambahan, jadi Configure yang berhasil di sini adalah
// sukses-bohong. Penjaga jumlahDokumenYAML tidak menutup kelas
// ini: ia diam bila penghitungnya sendiri GALAT atas aliran itu.
// Penolakan di sini bersifat tekstual dan tidak bergantung pada
// parser mana pun: baris sesudah penanda pertama hanya boleh
// kosong, komentar, atau penanda lagi. Baris "---" sesudah
// penanda adalah awal dokumen tambahan dan ikut tertolak (bentuk
// itu biasanya lebih dulu tertangkap tolakYAMLDokumenGanda).
//
// B-559: pengecualian "penanda lagi" di sini SENGAJA tidak
// dibatasi pada penanda kanonik — penanda berbentuk tab atau
// tanpa-spasi sesudah penanda pertama tetap dikecualikan, karena
// penulis (susunBlokModelHermes, perbaruiYAMLTopLevel)
// menormalkan SETIAP kemunculan penanda ke bentuk kanonik saat
// menulis (B-544/B-559). Membatasi pengecualian pada bentuk
// kanonik akan membalik keputusan B-544 (normalisasi, bukan
// penolakan) untuk penanda kedua+; yang dijaga penjaga ini
// adalah ISI, bukan bentuk penandanya.
// B-571: pemecahan memakai pecahBarisYAML, bukan strings.Split
// pada "\n" — CR, NEL, LS, dan PS adalah pemisah garis YAML
// yang menyembunyikan satu baris utuh di dalam satu "baris-\n":
// komentar penanda (pertama maupun kedua) atau komentar biasa
// yang memuat pemisah itu diikuti isi membuat pemuat melihat
// dokumen tambahan sesudah "...", sementara pemindaian "\n"
// membaca semuanya sebagai satu baris komentar/penanda dan
// meloloskan. Normalisasi (memecah pemisah tersembunyi menjadi
// "\n" lalu menulis) TIDAK menyelamatkan masukan ini: isinya
// tetap dokumen kedua sesudah "..." yang ditolak PyYAML —
// penolakan jujur di sini adalah satu-satunya hasil benar.
// Penulis (susunBlokModelHermes, perbaruiYAMLTopLevel) SENGAJA
// tetap memakai pemecahan "\n": pemisah "\r" di akhir baris
// adalah CR berkas CRLF yang dipertahankan (B-544) — penjaga
// inilah yang melihat baris YAML sejati sebelum penulis mana
// pun berjalan, dan Goose/Aider sudah gagal jujur atas kelas
// ini lewat verifikasi ketat semua-dokumennya.
func tolakIsiSesudahPenandaHermes(path, konten string) error {
	baris := pecahBarisYAML(konten)
	dalam := petaDalamSkalarYAML(baris)
	indeksPenanda := -1
	for i, l := range baris {
		if !dalam[i] && barisPenandaAkhirDokumen(l) {
			indeksPenanda = i
			break
		}
	}
	if indeksPenanda < 0 {
		return nil
	}
	for _, l := range baris[indeksPenanda+1:] {
		trim := strings.TrimSpace(l)
		if trim == "" || strings.HasPrefix(trim, "#") || barisPenandaAkhirDokumen(l) {
			continue
		}
		return fmt.Errorf("berkas %s memuat isi sesudah penanda akhir dokumen '...' (%q); isi itu di luar dokumen yang dibaca Hermes (PyYAML safe_load) — gabungkan menjadi satu dokumen, lalu coba lagi (Configure dibatalkan, tidak ada berkas yang diubah)", path, trim)
	}
	return nil
}

// tolakYAMLDokumenGanda menolak aliran YAML multi-dokumen atas
// nama pemuat tool tujuan (B-497, menggeneralisasi penjaga Hermes
// B-485): penyunting Route-X bekerja pada teks gabungan dan
// verifikasi hanya membaca dokumen pertama, sementara pemuat
// tujuan menolak aliran multi-dokumen atau membaca satu dokumen —
// sukses di sini adalah sukses-bohong kelas B-485 yang sama.
// alasanPemuat menyebut pemuatnya persis (terbukti di badan PR).
func tolakYAMLDokumenGanda(path, konten, alasanPemuat string) error {
	if nDok, errDok := jumlahDokumenYAML(konten); errDok == nil && nDok > 1 {
		return fmt.Errorf("berkas %s memuat %d dokumen YAML; %s — gabungkan menjadi satu dokumen, lalu coba lagi (Configure dibatalkan, tidak ada berkas yang diubah)", path, nDok, alasanPemuat)
	}
	return nil
}

// jumlahDokumenYAML menghitung dokumen dalam satu aliran YAML
// (B-485). Galat urai dikembalikan apa adanya.
func jumlahDokumenYAML(konten string) (int, error) {
	dec := yaml.NewDecoder(strings.NewReader(konten))
	n := 0
	for {
		var doc any
		err := dec.Decode(&doc)
		if errors.Is(err, io.EOF) {
			return n, nil
		}
		if err != nil {
			return 0, err
		}
		n++
	}
}

// nodeYAMLKeAnyToleran mengubah simpul YAML menjadi nilai Go
// dengan semantik parser tujuan Hermes (PyYAML): kunci ganda di
// pemetaan BUKAN galat — kemunculan TERAKHIR yang menang, di
// kedalaman berapa pun (B-483; terbukti dengan PyYAML sungguhan:
// safe_load atas kunci ganda diterima dan membaca nilai terakhir).
func nodeYAMLKeAnyToleran(n *yaml.Node) any {
	switch n.Kind {
	case yaml.MappingNode:
		out := make(map[string]any, len(n.Content)/2)
		for i := 0; i+1 < len(n.Content); i += 2 {
			out[n.Content[i].Value] = nodeYAMLKeAnyToleran(n.Content[i+1])
		}
		return out
	case yaml.SequenceNode:
		out := make([]any, 0, len(n.Content))
		for _, anak := range n.Content {
			out = append(out, nodeYAMLKeAnyToleran(anak))
		}
		return out
	case yaml.AliasNode:
		if n.Alias != nil {
			return nodeYAMLKeAnyToleran(n.Alias)
		}
		return nil
	case yaml.ScalarNode:
		var v any
		if err := n.Decode(&v); err != nil {
			return n.Value
		}
		return v
	}
	return nil
}

// uraiYAMLHermes memeriksa dokumen YAML untuk verifikasi Hermes
// dengan toleransi parser tujuannya (B-483): kunci tingkat-atas
// ganda DI LUAR blok `model:` ditoleransi last-wins (PyYAML
// menerimanya), sedangkan ISI blok `model:` sendiri tetap diurai
// KETAT — duplikat di dalam blok terkelola tetap galat, karena di
// sanalah nilai yang diverifikasi hidup.
func uraiYAMLHermes(konten string) (map[string]any, error) {
	dec := yaml.NewDecoder(strings.NewReader(konten))
	var akar yaml.Node
	if err := dec.Decode(&akar); err != nil {
		return nil, err
	}
	// B-571: ekor sesudah dokumen pertama diperiksa ketat —
	// dokumen tambahan ATAU galat urai di ekor adalah galat.
	// PyYAML safe_load (pemuat Hermes) menolak aliran apa pun
	// yang memuat isi sesudah "..." yang bukan komentar, baris
	// kosong, atau penanda lagi, sementara verifikasi lama hanya
	// mengurai dokumen pertama — isi sesudah "..." yang
	// menyembunyikan pemisah garis (CR/LS/NEL/PS) lolos sebagai
	// sukses-bohong. Toleransi B-483 TIDAK berubah: ia hanya
	// berlaku DI DALAM dokumen pertama (kunci ganda di luar blok
	// model); ekor apa pun memang ditolak pemuat tujuan.
	// Penanda "..." berulang, komentar, dan baris kosong di
	// ekor TIDAK menghasilkan dokumen — Decode mengembalikan
	// EOF atasnya (terbukti atas ketiganya + CRLF), jadi tidak
	// ada penolakan palsu atas ekor yang sah.
	for {
		var sisa yaml.Node
		err := dec.Decode(&sisa)
		if errors.Is(err, io.EOF) {
			break
		}
		if err != nil {
			return nil, fmt.Errorf("isi sesudah dokumen pertama tidak terurai: %w", err)
		}
		return nil, errors.New("dokumen YAML tambahan sesudah dokumen pertama")
	}
	if akar.Kind == 0 || len(akar.Content) == 0 {
		return nil, nil
	}
	dok := akar.Content[0]
	if dok.Kind != yaml.MappingNode {
		return nil, nil
	}
	out := make(map[string]any, len(dok.Content)/2)
	for i := 0; i+1 < len(dok.Content); i += 2 {
		kunciNode := dok.Content[i]
		nilaiNode := dok.Content[i+1]
		if normalkanKunciYAML(kunciNode.Value) == "model" && nilaiNode.Kind == yaml.MappingNode {
			var blok map[string]any
			if err := nilaiNode.Decode(&blok); err != nil {
				return nil, err
			}
			out["model"] = blok
			continue
		}
		out[kunciNode.Value] = nodeYAMLKeAnyToleran(nilaiNode)
	}
	return out, nil
}

// verifikasiEnvHermes memeriksa hasil sunting .env Hermes
// (B-477+B-483): variabel TERKELOLA tidak boleh ganda dan tidak
// boleh memuat byte NUL — NUL pada baris terkelola adalah jejak
// pengkodean UTF-16/32 yang terbawa ke HASIL GARAPAN (kelas
// sukses-bohong B-477: berkas dilaporkan tertulis padahal pemuat
// gagal membacanya). Duplikat variabel LAIN ditoleransi:
// python-dotenv menerimanya dengan bacaan last-wins (terbukti
// dengan pustaka sungguhan), dan baris-baris itu milik pengguna
// yang dipertahankan apa adanya oleh perbaruiBarisEnv.
//
// B-507: penolakan NUL dulu berlaku atas SELURUH berkas, termasuk
// NUL warisan di baris tak-terkelola yang tidak disentuh sunting
// ini — lebih ketat dari parser tujuan: python-dotenv terbukti
// membaca berkas dengan NUL di variabel lain dengan nilainya
// utuh. Kini penolakan dipersempit ke baris terkelola (satu-satunya
// bagian yang ditulis fungsi ini); NUL di baris lain adalah isi
// pengguna yang memang terbaca oleh tool-nya.
func verifikasiEnvHermes(konten, namaTerkelola string) error {
	terlihat := false
	for _, l := range strings.Split(konten, "\n") {
		if namaVariabelEnv(l) != namaTerkelola {
			continue
		}
		if strings.ContainsRune(l, '\x00') {
			return fmt.Errorf("hasil sunting .env memuat byte NUL pada variabel terkelola %q (jejak pengkodean UTF-16/32); Hermes tidak bisa membaca berkas seperti itu", namaTerkelola)
		}
		if terlihat {
			return fmt.Errorf("hasil sunting .env memuat variabel terkelola ganda %q", namaTerkelola)
		}
		terlihat = true
	}
	return nil
}

// GetExportScript membaca seluruh skrip lingkungan gabungan.
func (m *Manager) GetExportScript(homeDir string) (string, error) {
	if homeDir == "" {
		homeDir, _ = os.UserHomeDir()
	}
	envFile := filepath.Join(homeDir, ".routex", "cli-env.sh")
	// B-494: lewat bacaReguler — handler API memanggil ini per
	// permintaan; FIFO di cli-env.sh menggantungkannya tanpa batas.
	// Berkas khusus menjadi galat bernama, bukan gantungan.
	data, err := bacaReguler(envFile)
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
		// B-444: pengenalan baris lewat pecahBarisEnv (parser yang
		// sama dengan perbaruiBarisEnv) — potongan awalan lama
		// `TrimPrefix(trimmed, "export ")` hanya menerima SATU spasi
		// persis, jadi bentuk `export`+TAB lolos dari pembersihan.
		if nama, _, ok := pecahBarisEnv(line); ok && keyVars[nama] {
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
		// B-444: pengenalan baris lewat pecahBarisEnv (parser yang
		// sama dengan perbaruiBarisEnv) — potongan awalan lama
		// `TrimPrefix(trimmed, "export ")` hanya menerima SATU spasi
		// persis, jadi baris `export`+TAB tampil TAK tersamar lewat
		// GetExportScript.
		varName, val, ok := pecahBarisEnv(line)
		if !ok || !keyVars[varName] {
			continue
		}
		val = strings.Trim(val, "\"'")
		if val == "" {
			continue
		}
		// Indentasi asli baris dipertahankan di depan bentuk
		// kanonik tersamar.
		prefixPart := line[:len(line)-len(strings.TrimLeft(line, " \t"))]
		lines[i] = fmt.Sprintf("%sexport %s=\"%s\"", prefixPart, varName, security.MaskAPIKey(val))
	}
	return strings.Join(lines, "\n")
}
