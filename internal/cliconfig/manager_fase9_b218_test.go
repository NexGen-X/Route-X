package cliconfig

import (
	"context"
	"errors"
	"os"
	"path/filepath"
	"testing"

	"github.com/NexGen-X/Route-X/internal/database/repo/identity"
)

// Berkas ini memuat test pengunci B-218 (Fase 9 kampanye
// tuntas-logika): kegagalan Configure di tahap mana pun — penulisan
// berkas tool maupun Put settings terakhir — harus memulihkan
// SELURUH berkas yang sudah disentuh (key.env, cli-env.sh, hook
// shell, berkas tool) ke keadaan sebelum panggilan, bukan hanya
// key.env, supaya disk dan DB tidak berakhir tidak konsisten.

// mockSettingsStorePutGagal membungkus store normal tetapi Put selalu
// gagal, meniru kegagalan persistensi di langkah terakhir Configure.
type mockSettingsStorePutGagal struct {
	dasar *mockSettingsStore
}

func (m *mockSettingsStorePutGagal) Get(ctx context.Context, key string) (identity.Setting, error) {
	return m.dasar.Get(ctx, key)
}

func (m *mockSettingsStorePutGagal) Put(ctx context.Context, s identity.SettingWrite) (identity.Setting, error) {
	return identity.Setting{}, errors.New("kegagalan Put disengaja untuk uji B-218")
}

func (m *mockSettingsStorePutGagal) All(ctx context.Context) ([]identity.Setting, error) {
	return m.dasar.All(ctx)
}

func tulisBerkasUji(t *testing.T, jalur, isi string, izin os.FileMode) {
	t.Helper()
	if err := os.MkdirAll(filepath.Dir(jalur), 0o755); err != nil {
		t.Fatalf("MkdirAll %s: %v", jalur, err)
	}
	if err := os.WriteFile(jalur, []byte(isi), izin); err != nil {
		t.Fatalf("WriteFile %s: %v", jalur, err)
	}
}

func harusSamaIsi(t *testing.T, jalur, mau string) {
	t.Helper()
	raw, err := os.ReadFile(jalur)
	if err != nil {
		t.Fatalf("ReadFile %s: %v", jalur, err)
	}
	if string(raw) != mau {
		t.Errorf("isi %s berubah sesudah Configure gagal:\n--- mau ---\n%s\n--- dapat ---\n%s", jalur, mau, raw)
	}
}

// TestB218PutGagalMemulihkanSemuaBerkas: Configure claude yang gagal
// di Put terakhir harus meninggalkan key.env, cli-env.sh, .bashrc,
// dan settings.json PERSIS seperti sebelum panggilan. Di kode lama
// hanya key.env yang (tidak) tersentuh pemulihan pada jalur ini —
// Put gagal tidak memulihkan apa pun sama sekali.
func TestB218PutGagalMemulihkanSemuaBerkas(t *testing.T) {
	ctx := context.Background()
	home := t.TempDir()
	t.Setenv("HOME", home)

	keyEnvAwal := "PENANDA_KEYENV_LAMA=1\n"
	cliEnvAwal := "# PENANDA_CLIENV_LAMA\n"
	bashrcAwal := "# bashrc pengguna\nPENANDA_BASHRC=1\n"
	settingsAwal := "{\"env\": {\"ANTHROPIC_MODEL\": \"model-lama\"}, \"penanda\": \"jangan-hilang\"}\n"

	tulisBerkasUji(t, filepath.Join(home, ".routex", "key.env"), keyEnvAwal, 0o600)
	tulisBerkasUji(t, filepath.Join(home, ".routex", "cli-env.sh"), cliEnvAwal, 0o644)
	tulisBerkasUji(t, filepath.Join(home, ".bashrc"), bashrcAwal, 0o644)
	tulisBerkasUji(t, filepath.Join(home, ".claude", "settings.json"), settingsAwal, 0o644)

	mgr := manajerUjiCipher(t, &mockSettingsStorePutGagal{dasar: newMockSettingsStore()})
	_, err := mgr.Configure(ctx, ConfigureParams{
		ToolID: "claude", Mode: ModeModelOnly, Target: "model-b218",
		GatewayURL: "http://localhost:8080/v1", APIKey: "GANTI_kunci-b218",
	}, "http://localhost:8080/v1")
	if err == nil {
		t.Fatal("Configure harus gagal bila Put settings gagal")
	}

	harusSamaIsi(t, filepath.Join(home, ".routex", "key.env"), keyEnvAwal)
	harusSamaIsi(t, filepath.Join(home, ".routex", "cli-env.sh"), cliEnvAwal)
	harusSamaIsi(t, filepath.Join(home, ".bashrc"), bashrcAwal)
	harusSamaIsi(t, filepath.Join(home, ".claude", "settings.json"), settingsAwal)
}

// TestB218HermesGagalDiBerkasKeduaMemulihkanYangPertama: penulis
// hermes menulis config.yaml LALU .env; .env yang berupa direktori
// membuat penulisan kedua gagal. config.yaml (sudah terlanjur
// ditulis) dan cli-env.sh (sudah tersinkron) harus kembali ke
// keadaan awal. Di kode lama config.yaml tertinggal berisi model
// baru dan cli-env.sh tertinggal tersinkron.
func TestB218HermesGagalDiBerkasKeduaMemulihkanYangPertama(t *testing.T) {
	ctx := context.Background()
	home := t.TempDir()
	t.Setenv("HOME", home)

	configAwal := "model:\n  provider: custom\n  default: model-lama\npenanda_lain: utuh\n"
	cliEnvAwal := "# PENANDA_CLIENV_LAMA\n"

	tulisBerkasUji(t, filepath.Join(home, ".hermes", "config.yaml"), configAwal, 0o644)
	if err := os.MkdirAll(filepath.Join(home, ".hermes", ".env"), 0o755); err != nil {
		t.Fatalf("MkdirAll .env: %v", err)
	}
	tulisBerkasUji(t, filepath.Join(home, ".routex", "cli-env.sh"), cliEnvAwal, 0o644)

	mgr := manajerUjiCipher(t, newMockSettingsStore())
	_, err := mgr.Configure(ctx, ConfigureParams{
		ToolID: "hermes", Mode: ModeModelOnly, Target: "model-b218",
		GatewayURL: "http://localhost:8080/v1", APIKey: "GANTI_kunci-b218",
	}, "http://localhost:8080/v1")
	if err == nil {
		t.Fatal("Configure hermes harus gagal bila .env tidak dapat ditulis")
	}

	harusSamaIsi(t, filepath.Join(home, ".hermes", "config.yaml"), configAwal)
	harusSamaIsi(t, filepath.Join(home, ".routex", "cli-env.sh"), cliEnvAwal)
}
