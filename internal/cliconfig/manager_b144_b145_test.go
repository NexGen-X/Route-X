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

	"github.com/NexGen-X/Route-X/internal/database/repo/identity"
)

// TestConfigureOpenCommitBatasToken membuktikan B-144: Configure
// OpenCommit menulis batas token skema v3 yang benar-benar dibaca tool
// (OCO_TOKENS_MAX_OUTPUT + OCO_REASONING_MAX_TOKENS, dari dokumentasi
// resmi OpenCommit 3.x) dengan nilai 4000 — 8× plafon bawaan 500 yang
// terbukti habis seluruhnya untuk reasoning intern-s2 pada Uji Live
// Putaran 1 (reasoning 500, isi 0, EMPTY_MESSAGE) — di berkas
// ~/.opencommit DAN di EnvVars hasil. Keluaran oco lain tidak berubah.
func TestConfigureOpenCommitBatasToken(t *testing.T) {
	home := t.TempDir()
	t.Setenv("HOME", home)
	mgr := NewManager(newMockSettingsStore(), nil, slog.New(slog.NewTextHandler(io.Discard, nil)))
	res, err := mgr.Configure(context.Background(), ConfigureParams{
		ToolID: "opencommit", Mode: ModeModelOnly, Target: "intern-s2", APIKey: "kunci-oco-123",
	}, "http://localhost:8080/v1")
	if err != nil {
		t.Fatalf("Configure opencommit: %v", err)
	}
	if res.EnvVars["OCO_TOKENS_MAX_OUTPUT"] != "4000" || res.EnvVars["OCO_REASONING_MAX_TOKENS"] != "4000" {
		t.Errorf("EnvVars batas token opencommit salah: %v", res.EnvVars)
	}
	data, err := os.ReadFile(filepath.Join(home, ".opencommit"))
	if err != nil {
		t.Fatalf(".opencommit tidak ditulis: %v", err)
	}
	for _, mau := range []string{
		"OCO_AI_PROVIDER=openai", "OCO_API_KEY=kunci-oco-123",
		"OCO_API_URL=http://localhost:8080/v1", "OCO_MODEL=intern-s2",
		"OCO_TOKENS_MAX_OUTPUT=4000", "OCO_REASONING_MAX_TOKENS=4000",
	} {
		if !strings.Contains(string(data), mau) {
			t.Errorf(".opencommit tidak memuat %q:\n%s", mau, data)
		}
	}
}

// tulisBinerPalsu menulis skrip executable kecil sebagai pengganti
// biner tool untuk test deteksi versi.
func tulisBinerPalsu(t *testing.T, nama, isi string) string {
	t.Helper()
	p := filepath.Join(t.TempDir(), nama)
	if err := os.WriteFile(p, []byte(isi), 0755); err != nil {
		t.Fatal(err)
	}
	return p
}

// skripSgptPalsu meniru sgpt 1.5.1 persis pada sumbu B-145: tanpa
// OPENAI_API_KEY di env ia gugur saat import (rc 1); dengan kunci ia
// mencetak versinya.
const skripSgptPalsu = "#!/bin/sh\n" +
	"if [ -z \"$OPENAI_API_KEY\" ]; then\n" +
	"  echo 'UsageError: Missing config key: OPENAI_API_KEY' >&2\n" +
	"  exit 1\n" +
	"fi\n" +
	"echo 'ShellGPT 1.5.1'\n"

func manajerDenganKunciSgpt(t *testing.T) *Manager {
	t.Helper()
	store := newMockSettingsStore()
	raw, err := json.Marshal(StoredConfig{Mode: ModeModelOnly, Target: "intern-s2", APIKey: "kunci-sgpt-tersimpan"})
	if err != nil {
		t.Fatal(err)
	}
	store.data["cli:config:sgpt"] = identity.Setting{Key: "cli:config:sgpt", Value: raw}
	return NewManager(store, nil, slog.New(slog.NewTextHandler(io.Discard, nil)))
}

// TestDeteksiVersiSgptMemakaiKunciTersimpan membuktikan B-145(a):
// deteksi versi sgpt berjalan dengan kunci tersimpan tool itu, jadi
// versi tool yang sehat terbaca; kunci tidak bocor ke hasil (yang
// kembali hanya baris versinya).
func TestDeteksiVersiSgptMemakaiKunciTersimpan(t *testing.T) {
	t.Setenv("OPENAI_API_KEY", "")
	def, ok := cariToolDef("sgpt")
	if !ok {
		t.Fatal("sgpt tidak ditemukan")
	}
	if !def.VersionNeedsStoredKey {
		t.Fatal("sgpt wajib VersionNeedsStoredKey (B-145)")
	}
	bin := tulisBinerPalsu(t, "sgpt", skripSgptPalsu)
	mgr := manajerDenganKunciSgpt(t)
	got := mgr.detectVersionUntukTool(context.Background(), def, bin)
	if got != "ShellGPT 1.5.1" {
		t.Errorf("versi sgpt berkunci tersimpan = %q, mau %q", got, "ShellGPT 1.5.1")
	}
	if strings.Contains(got, "kunci-sgpt-tersimpan") {
		t.Errorf("kunci tersimpan bocor ke hasil deteksi: %q", got)
	}
}

// TestDeteksiVersiSgptTanpaKunciTersimpanKosong membuktikan arah
// sebaliknya: tanpa konfigurasi tersimpan berkunci, deteksi sgpt
// tetap kosong (perilaku lama yang jujur) — bukan error, bukan versi
// karangan.
func TestDeteksiVersiSgptTanpaKunciTersimpanKosong(t *testing.T) {
	t.Setenv("OPENAI_API_KEY", "")
	def, _ := cariToolDef("sgpt")
	bin := tulisBinerPalsu(t, "sgpt", skripSgptPalsu)
	mgr := NewManager(newMockSettingsStore(), nil, slog.New(slog.NewTextHandler(io.Discard, nil)))
	if got := mgr.detectVersionUntukTool(context.Background(), def, bin); got != "" {
		t.Errorf("versi sgpt tanpa kunci = %q, mau kosong", got)
	}
}

// TestDeteksiVersiToolLainTidakBerubah membuktikan jalur deteksi tool
// tanpa kebutuhan kunci tidak tersentuh perubahan B-145.
func TestDeteksiVersiToolLainTidakBerubah(t *testing.T) {
	def, _ := cariToolDef("llm")
	bin := tulisBinerPalsu(t, "llm", "#!/bin/sh\necho 'llm, version 0.37.1'\n")
	mgr := NewManager(newMockSettingsStore(), nil, slog.New(slog.NewTextHandler(io.Discard, nil)))
	if got := mgr.detectVersionUntukTool(context.Background(), def, bin); got != "llm, version 0.37.1" {
		t.Errorf("versi llm = %q, mau %q", got, "llm, version 0.37.1")
	}
}

// TestChatbladeTanpaDeteksiVersi membuktikan B-145(b): VersionArg
// chatblade kosong (build teruji live menolak --version dan tidak
// menyediakan jalur versi lain yang sah), dan deteksi TIDAK
// mengeksekusi binernya sama sekali — versi kosong secara jujur.
func TestChatbladeTanpaDeteksiVersi(t *testing.T) {
	def, ok := cariToolDef("chatblade")
	if !ok {
		t.Fatal("chatblade tidak ditemukan")
	}
	if def.VersionArg != "" {
		t.Fatalf("VersionArg chatblade = %q, mau kosong (B-145)", def.VersionArg)
	}
	penanda := filepath.Join(t.TempDir(), "pernah-dijalankan")
	bin := tulisBinerPalsu(t, "chatblade", "#!/bin/sh\necho ran > "+penanda+"\necho 'chatblade 0.7.0'\n")
	mgr := NewManager(newMockSettingsStore(), nil, slog.New(slog.NewTextHandler(io.Discard, nil)))
	if got := mgr.detectVersionUntukTool(context.Background(), def, bin); got != "" {
		t.Errorf("versi chatblade = %q, mau kosong secara jujur", got)
	}
	if _, err := os.Stat(penanda); !os.IsNotExist(err) {
		t.Errorf("biner chatblade tidak boleh dieksekusi untuk deteksi versi")
	}
}
