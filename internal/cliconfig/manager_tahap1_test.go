package cliconfig

import (
	"context"
	"io"
	"log/slog"
	"strings"
	"testing"
	"time"
)

// TestConfigureCopilotBYOK membuktikan B-128: mode BYOK resmi
// COPILOT_PROVIDER_* menggantikan GITHUB_COPILOT_* rekaan.
func TestConfigureCopilotBYOK(t *testing.T) {
	home := t.TempDir()
	t.Setenv("HOME", home)
	mgr := NewManager(newMockSettingsStore(), nil, slog.New(slog.NewTextHandler(io.Discard, nil)))
	res, err := mgr.Configure(context.Background(), ConfigureParams{
		ToolID: "copilot", Mode: ModeModelOnly, Target: "intern-s2",
	}, "http://localhost:8080/v1")
	if err != nil {
		t.Fatalf("Configure copilot: %v", err)
	}
	mau := map[string]string{
		"COPILOT_PROVIDER_BASE_URL": "http://localhost:8080/v1",
		"COPILOT_PROVIDER_TYPE":     "openai",
		"COPILOT_MODEL":             "intern-s2",
	}
	for k, v := range mau {
		if res.EnvVars[k] != v {
			t.Errorf("EnvVars[%s] = %q, mau %q", k, res.EnvVars[k], v)
		}
	}
	for k := range res.EnvVars {
		if strings.HasPrefix(k, "GITHUB_COPILOT_") {
			t.Errorf("variabel rekaan warisan masih diekspor: %s", k)
		}
	}
	def, _ := cariToolDef("copilot")
	if def.EnvVarAPIKey != "COPILOT_PROVIDER_API_KEY" {
		t.Errorf("EnvVarAPIKey copilot = %q", def.EnvVarAPIKey)
	}
}

// TestEkstrakVersi membuktikan B-121: baris placeholder seperti
// "None" tidak pernah dilaporkan sebagai versi; keluaran nyata
// tool (semver polos / berprefiks nama) terbaca utuh.
func TestEkstrakVersi(t *testing.T) {
	kasus := []struct{ in, mau string }{
		{"1.5.47\n", "1.5.47"},
		{"None\n1.5.47\n", "1.5.47"},
		{"None\n", ""},
		{"unknown\n", ""},
		{"codex-cli 0.153.4\n", "codex-cli 0.153.4"},
		{"crush version v0.97.1\n", "crush version v0.97.1"},
		{"grok 1.0.13 (5e9a58528b76) [stable]\n", "grok 1.0.13 (5e9a58528b76) [stable]"},
		{"\n\n  \n", ""},
	}
	for _, k := range kasus {
		if got := ekstrakVersi(k.in); got != k.mau {
			t.Errorf("ekstrakVersi(%q) = %q, mau %q", k.in, got, k.mau)
		}
	}
}

// TestKeyEnvMelewatiToolTercabut membuktikan sisi key.env dari
// perombakan registri Tahap 1: kunci warisan milik tool yang sudah
// dicabut tidak lagi disumbangkan — termasuk ke nama generik —
// sementara tool yang bertahan tidak berubah.
func TestKeyEnvMelewatiToolTercabut(t *testing.T) {
	lama := time.Date(2026, 11, 1, 10, 0, 0, 0, time.UTC)
	baru := lama.Add(time.Hour)
	perTool := map[string]kunciTersimpan{
		"claude": {kunci: "kunci-claude", diperbarui: lama},
		"sgpt":   {kunci: "kunci-sgpt", diperbarui: baru},   // tercabut Tahap 1, walau terbaru
		"agy":    {kunci: "kunci-agy", diperbarui: baru},    // tercabut Tahap 1
		"mentat": {kunci: "kunci-mentat", diperbarui: baru}, // sudah dicabut sejak B-133
	}
	konten, err := susunKontenKeyEnv(perTool)
	if err != nil {
		t.Fatalf("susunKontenKeyEnv: %v", err)
	}
	for _, bocor := range []string{"kunci-sgpt", "kunci-agy", "kunci-mentat"} {
		if strings.Contains(konten, bocor) {
			t.Errorf("kunci tool tercabut %q masih disumbangkan:\n%s", bocor, konten)
		}
	}
	if !strings.Contains(konten, "export ANTHROPIC_API_KEY='kunci-claude'") {
		t.Errorf("kunci claude harus tetap terisi:\n%s", konten)
	}
	// Nama generik mengikuti tool BERTAHAN terbaru (claude), bukan
	// tool tercabut yang kebetulan lebih baru.
	if !strings.Contains(konten, "export ROUTEX_API_KEY='kunci-claude'") {
		t.Errorf("nama generik harus mengikuti claude:\n%s", konten)
	}
	// Keputusan desain B-117 dinyatakan di header berkas itu sendiri.
	if !strings.Contains(konten, "B-117") {
		t.Errorf("header key.env harus mencatat keputusan B-117:\n%s", konten)
	}
}
