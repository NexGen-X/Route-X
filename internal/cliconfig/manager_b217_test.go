package cliconfig

import (
	"context"
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

// Berkas ini memuat test pengunci B-217 (Fase 8 kampanye tuntas-logika):
// berkas konfigurasi tool yang ADA tetapi tidak terurai tidak boleh lagi
// ditelan diam-diam lalu ditimpa — Configure membatalkan diri dan berkas
// tidak disentuh; khusus OpenCode, JSONC yang sah menurut tool-nya
// diterima lewat penguraian toleran.

// jalurKonfigB217 adalah lokasi berkas konfigurasi per tool relatif ke HOME.
var jalurKonfigB217 = map[string]string{
	"claude":   filepath.Join(".claude", "settings.json"),
	"opencode": filepath.Join(".config", "opencode", "opencode.json"),
	"qwen":     filepath.Join(".qwen", "settings.json"),
	"crush":    filepath.Join(".config", "crush", "crush.json"),
}

func TestB217KonfigRusakMembatalkanConfigure(t *testing.T) {
	ctx := context.Background()
	rusak := []byte(`{"punya_pengguna": "setelan-penting-b217", "rusak": }`)

	for tool, rel := range jalurKonfigB217 {
		t.Run(tool, func(t *testing.T) {
			home := t.TempDir()
			t.Setenv("HOME", home)
			path := filepath.Join(home, rel)
			if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
				t.Fatalf("MkdirAll: %v", err)
			}
			if err := os.WriteFile(path, rusak, 0o644); err != nil {
				t.Fatalf("WriteFile: %v", err)
			}

			mgr := manajerUjiCipher(t, newMockSettingsStore())
			_, err := mgr.Configure(ctx, ConfigureParams{
				ToolID: tool, Mode: ModeModelOnly, Target: "model-b217",
				GatewayURL: "http://localhost:8080/v1", APIKey: "GANTI_kunci-b217",
			}, "http://localhost:8080/v1")
			if err == nil {
				t.Fatalf("Configure %s berhasil untuk berkas rusak, mau error pembatalan", tool)
			}
			if !strings.Contains(err.Error(), "tidak bisa diurai") {
				t.Errorf("error Configure %s = %q, ingin menyebut berkas tidak bisa diurai", tool, err)
			}
			sesudah, rerr := os.ReadFile(path)
			if rerr != nil {
				t.Fatalf("membaca ulang berkas: %v", rerr)
			}
			if string(sesudah) != string(rusak) {
				t.Errorf("berkas %s berubah sesudah Configure gagal:\ndapat %s\nmau  %s (tidak disentuh)", tool, sesudah, rusak)
			}
		})
	}
}

func TestB217OpenCodeJSONCDiterima(t *testing.T) {
	home := t.TempDir()
	t.Setenv("HOME", home)
	ctx := context.Background()

	path := filepath.Join(home, ".config", "opencode", "opencode.json")
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		t.Fatalf("MkdirAll: %v", err)
	}
	jsonc := `{
  // Konfigurasi pengguna OpenCode — komentar gaya JSONC.
  "$schema": "https://opencode.ai/config.json",
  "theme": "opencode",
  /* komentar blok
     dua baris */
  "keterangan": "dia berkata \"halo // dunia\"",
  "provider": {
    "penyedia-lain": {
      "npm": "@ai-sdk/openai-compatible",
      "options": { "baseURL": "https://contoh.invalid/v1" },
    },
  },
}
`
	if err := os.WriteFile(path, []byte(jsonc), 0o644); err != nil {
		t.Fatalf("WriteFile: %v", err)
	}

	mgr := manajerUjiCipher(t, newMockSettingsStore())
	if _, err := mgr.Configure(ctx, ConfigureParams{
		ToolID: "opencode", Mode: ModeModelOnly, Target: "model-b217",
		GatewayURL: "http://localhost:8080/v1", APIKey: "GANTI_kunci-b217",
	}, "http://localhost:8080/v1"); err != nil {
		t.Fatalf("Configure opencode untuk berkas JSONC sah: %v", err)
	}

	raw, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("membaca hasil: %v", err)
	}
	var cfg map[string]any
	// B-280 (Fase 14) mengubah kontrak keluaran yang dikunci di
	// sini: berkas yang memuat komentar JSONC kini DISUNTING pada
	// teksnya sehingga komentar pengguna bertahan — hasil Configure
	// adalah JSONC yang sah (format resmi OpenCode), bukan lagi
	// JSON ketat hasil normalisasi marshal. Janji inti B-217 tidak
	// berubah: berkas sah tidak ditolak dan seluruh DATANYA utuh;
	// penguraian di bawah karena itu memakai penoleransi yang sama
	// seperti jalur muat.
	if err := json.Unmarshal(uraiJSONCToleran(raw), &cfg); err != nil {
		t.Fatalf("hasil Configure bukan JSONC yang sah: %v (%s)", err, raw)
	}
	if !strings.Contains(string(raw), "// Konfigurasi pengguna OpenCode") ||
		!strings.Contains(string(raw), "/* komentar blok") {
		t.Errorf("komentar pengguna hilang sesudah Configure (B-280):\n%s", raw)
	}
	if cfg["theme"] != "opencode" {
		t.Errorf("theme pengguna hilang: %v", cfg["theme"])
	}
	if cfg["keterangan"] != `dia berkata "halo // dunia"` {
		t.Errorf("keterangan berubah: %q — string berisi // dan kutip lolos harus utuh", cfg["keterangan"])
	}
	if cfg["$schema"] != "https://opencode.ai/config.json" {
		t.Errorf("$schema berubah: %v — // di dalam string URL tidak boleh dianggap komentar", cfg["$schema"])
	}
	providers, _ := cfg["provider"].(map[string]any)
	if _, ada := providers["penyedia-lain"]; !ada {
		t.Errorf("provider lain milik pengguna hilang: %v", providers)
	}
	if _, ada := providers["routex"]; !ada {
		t.Errorf("provider routex tidak ditambahkan: %v", providers)
	}
	if cfg["model"] != "routex/model-b217" {
		t.Errorf("model = %v, ingin routex/model-b217", cfg["model"])
	}
}

func TestB217KonfigSahTetapDigabung(t *testing.T) {
	home := t.TempDir()
	t.Setenv("HOME", home)
	ctx := context.Background()

	path := filepath.Join(home, ".claude", "settings.json")
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		t.Fatalf("MkdirAll: %v", err)
	}
	if err := os.WriteFile(path, []byte(`{"theme": "dark", "setelanLain": {"a": 1}}`), 0o644); err != nil {
		t.Fatalf("WriteFile: %v", err)
	}

	mgr := manajerUjiCipher(t, newMockSettingsStore())
	if _, err := mgr.Configure(ctx, ConfigureParams{
		ToolID: "claude", Mode: ModeModelOnly, Target: "model-b217",
		GatewayURL: "http://localhost:8080/v1", APIKey: "GANTI_kunci-b217",
	}, "http://localhost:8080/v1"); err != nil {
		t.Fatalf("Configure claude: %v", err)
	}
	raw, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("membaca hasil: %v", err)
	}
	var cfg map[string]any
	if err := json.Unmarshal(raw, &cfg); err != nil {
		t.Fatalf("hasil bukan JSON sah: %v", err)
	}
	if cfg["theme"] != "dark" {
		t.Errorf("theme pengguna hilang sesudah Configure: %v", cfg)
	}
	if _, ada := cfg["setelanLain"]; !ada {
		t.Errorf("setelanLain pengguna hilang sesudah Configure: %v", cfg)
	}
}
