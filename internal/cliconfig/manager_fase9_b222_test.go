package cliconfig

import (
	"context"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

// Berkas ini memuat test pengunci B-222 (Fase 9 kampanye
// tuntas-logika): penulis Codex, Grok Build, dan Hermes mengganti
// SELURUH isi seksi/blok yang dikelolanya, menghapus kunci lain yang
// sah milik pengguna di seksi yang sama setiap Configure. Sesudah
// fix, penggabungan bersifat per-kunci: kunci terkelola diperbarui,
// kunci lain dipertahankan.

// TestB222CodexKunciLainDalamSeksiUtuh: kunci pengguna di dalam
// [model_providers.routex] sendiri (mis. request_max_retries)
// selamat dari penulisan ulang, sementara kunci terkelola diperbarui.
func TestB222CodexKunciLainDalamSeksiUtuh(t *testing.T) {
	home := t.TempDir()
	t.Setenv("HOME", home)
	dir := filepath.Join(home, ".codex")
	if err := os.MkdirAll(dir, 0o755); err != nil {
		t.Fatal(err)
	}
	lama := "model = \"model-lama\"\n\n[model_providers.routex]\n" +
		"name = \"Route-X\"\n" +
		"base_url = \"http://lama.invalid/v1\"\n" +
		"request_max_retries = 7\n" +
		"stream_idle_timeout_ms = 45000\n"
	if err := os.WriteFile(filepath.Join(dir, "config.toml"), []byte(lama), 0o644); err != nil {
		t.Fatal(err)
	}
	def, ok := cariToolDef("codex")
	if !ok {
		t.Fatal("codex tidak ditemukan di registri")
	}
	mgr := manajerUjiToolBaru(newMockSettingsStore())
	if err := mgr.applyToolSpecificConfig(context.Background(), home, def, ModeModelOnly, "gpt-5", "http://localhost:8080/v1", "GANTI_kunci-codex"); err != nil {
		t.Fatalf("applyToolSpecificConfig codex: %v", err)
	}
	konten := bacaBerkasUji(t, filepath.Join(dir, "config.toml"))
	harusMemuat(t, konten, "request_max_retries = 7")
	harusMemuat(t, konten, "stream_idle_timeout_ms = 45000")
	harusMemuat(t, konten, `base_url = "http://localhost:8080/v1"`)
	harusMemuat(t, konten, `wire_api = "responses"`)
	if strings.Contains(konten, "lama.invalid") {
		t.Errorf("base_url lama tidak diperbarui:\n%s", konten)
	}
}

// TestB222GrokKunciLainDalamSeksiUtuh: pola yang sama untuk
// [model.routex] Grok Build lewat Configure penuh.
func TestB222GrokKunciLainDalamSeksiUtuh(t *testing.T) {
	home := t.TempDir()
	t.Setenv("HOME", home)
	dir := filepath.Join(home, ".grok")
	if err := os.MkdirAll(dir, 0o755); err != nil {
		t.Fatal(err)
	}
	lama := "[model.routex]\n" +
		"model = \"model-lama\"\n" +
		"max_retries = 3\n"
	if err := os.WriteFile(filepath.Join(dir, "config.toml"), []byte(lama), 0o644); err != nil {
		t.Fatal(err)
	}
	mgr := manajerUjiToolBaru(newMockSettingsStore())
	if _, err := mgr.Configure(context.Background(), ConfigureParams{
		ToolID: "grok-build", Mode: ModeModelOnly, Target: "gpt-5",
		GatewayURL: "http://localhost:8080/v1", APIKey: "GANTI_kunci-grok",
	}, "http://localhost:8080/v1"); err != nil {
		t.Fatalf("Configure grok-build: %v", err)
	}
	konten := bacaBerkasUji(t, filepath.Join(dir, "config.toml"))
	harusMemuat(t, konten, "max_retries = 3")
	harusMemuat(t, konten, `model = "gpt-5"`)
	harusMemuat(t, konten, `base_url = "http://localhost:8080/v1"`)
	if strings.Contains(konten, "model-lama") {
		t.Errorf("model lama tidak diperbarui:\n%s", konten)
	}
}

// TestB222HermesSubKunciLainUtuh: sub-kunci pengguna di bawah blok
// model: Hermes (di luar yang dikelola Route-X) dipertahankan;
// api_key lama dipertahankan bila kunci efektif kosong; hasil
// penggabungan idempoten.
func TestB222HermesSubKunciLainUtuh(t *testing.T) {
	existing := "model:\n" +
		"  provider: custom\n" +
		"  default: model-lama\n" +
		"  max_tokens: 4096\n" +
		"  catatan_model: jaga-saya\n" +
		"  api_key: kunci-lama\n" +
		"seksi_lain: utuh\n"

	hasil := susunBlokModelHermes(existing, "model-baru", "http://localhost:8080/v1", "GANTI_kunci-baru")
	harusMemuat(t, hasil, "  default: model-baru")
	harusMemuat(t, hasil, "  max_tokens: 4096")
	harusMemuat(t, hasil, "  catatan_model: jaga-saya")
	harusMemuat(t, hasil, "  api_key: GANTI_kunci-baru")
	harusMemuat(t, hasil, "seksi_lain: utuh")
	if strings.Contains(hasil, "model-lama") || strings.Contains(hasil, "kunci-lama") {
		t.Errorf("nilai terkelola lama tidak diganti:\n%s", hasil)
	}

	// apiKey kosong = tidak diubah: baris api_key lama tidak dihapus.
	hasil2 := susunBlokModelHermes(existing, "model-baru", "http://localhost:8080/v1", "")
	harusMemuat(t, hasil2, "  api_key: kunci-lama")
	harusMemuat(t, hasil2, "  max_tokens: 4096")

	// Idempoten: menerapkan ulang pada hasil tidak mengubah apa pun.
	ulang := susunBlokModelHermes(hasil, "model-baru", "http://localhost:8080/v1", "GANTI_kunci-baru")
	if ulang != hasil {
		t.Errorf("penggabungan tidak idempoten:\n--- pertama ---\n%s\n--- kedua ---\n%s", hasil, ulang)
	}
}
