package cliconfig

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

// Berkas ini memuat test pengunci B-281 dan B-282 (Fase 14 kampanye
// tuntas-logika), dua cacat penulis konfigurasi CLI:
//   - B-281: buildSnippet & syncGlobalEnvScript mengiterasi map Go
//     secara langsung — urutan baris export cli-env.sh acak antar
//     Configure identik, berkas tidak idempoten byte.
//   - B-282: yamlKutip menulis skalar berubah-tipe secara polos
//     (123/true/null/*jangkar/[daftar]/{peta}) ke config YAML
//     goose/aider/hermes, sehingga nilai berubah makna saat dibaca
//     ulang tool.

// TestB282YamlKutipSkalarAmbigau: nilai ambigu-tipe selalu dikutip;
// nama model lazim tetap polos (tidak ada kutip berlebihan yang
// mengubah tampilan config normal).
func TestB282YamlKutipSkalarAmbigu(t *testing.T) {
	wajibKutip := []string{
		"123", "-42", "1.5", "1e5", "0x1F", "0o17",
		"true", "FALSE", "yes", "Off", "on", "null", "~",
		"*jangkar", "&jangkar", "!tag", "[daftar]", "{peta}",
		"2026-11-06", ".inf", ".nan", "-", "?",
	}
	for _, v := range wajibKutip {
		if got := yamlKutip(v); !strings.HasPrefix(got, "\"") {
			t.Errorf("yamlKutip(%q) = %q, mau dikutip (berubah tipe/struktur bila polos)", v, got)
		}
	}
	tetapPolos := []string{
		"gemini-4-argon-medium", "openai/gpt-5", "gpt-4o",
		"3.5-turbo", "llama-3.1-8b", "claude-sonnet-5",
	}
	for _, v := range tetapPolos {
		if got := yamlKutip(v); got != v {
			t.Errorf("yamlKutip(%q) = %q, mau tetap polos", v, got)
		}
	}
	// Jalur penulis sungguhan: target numerik di blok Hermes
	// terbit sebagai string terkutip, bukan angka YAML.
	hasil := susunBlokModelHermes("", "123", "http://localhost:8080/v1", "")
	harusMemuat(t, hasil, `  default: "123"`)
}

// TestB281SnippetDeterministik: buildSnippet atas map yang sama
// selalu menghasilkan byte yang sama, terurut abjad.
func TestB281SnippetDeterministik(t *testing.T) {
	mgr := manajerUjiToolBaru(newMockSettingsStore())
	vars := map[string]string{
		"ZULU_VAR": "1", "ALPHA_VAR": "2", "MIKE_VAR": "3",
		"BRAVO_VAR": "4", "YANKEE_VAR": "5", "CHARLIE_VAR": "6",
	}
	pertama := mgr.buildSnippet(vars)
	for i := 0; i < 50; i++ {
		if got := mgr.buildSnippet(vars); got != pertama {
			t.Fatalf("buildSnippet tidak deterministik pada lari ke-%d:\n%s\n---\n%s", i, pertama, got)
		}
	}
	idxAlpha := strings.Index(pertama, "ALPHA_VAR")
	idxZulu := strings.Index(pertama, "ZULU_VAR")
	if idxAlpha < 0 || idxZulu < 0 || idxAlpha > idxZulu {
		t.Fatalf("urutan snippet tidak terurut abjad:\n%s", pertama)
	}
}

// TestB281CliEnvIdempotenByte: syncGlobalEnvScript berulang atas
// variabel yang sama menghasilkan cli-env.sh yang byte-identik
// setiap kali (probe Putaran Bersih membuktikan urutan dua baris
// export bertukar antar Configure identik pada kode lama).
func TestB281CliEnvIdempotenByte(t *testing.T) {
	home := t.TempDir()
	t.Setenv("HOME", home)
	def, ok := cariToolDef("opencode")
	if !ok {
		t.Fatal("opencode tidak ditemukan di registri")
	}
	mgr := manajerUjiToolBaru(newMockSettingsStore())
	vars := map[string]string{
		"OPENCODE_API_BASE": "http://localhost:8080/v1",
		"OPENCODE_MODEL":    "routex/model-uji",
		"OPENCODE_API_KEY":  "GANTI_kunci",
		"EXTRA_D":           "d", "EXTRA_C": "c",
		"EXTRA_B": "b", "EXTRA_A": "a",
	}
	jalur := filepath.Join(home, ".routex", "cli-env.sh")
	distinct := map[string]bool{}
	for i := 0; i < 30; i++ {
		if err := mgr.syncGlobalEnvScript(home, def, vars); err != nil {
			t.Fatalf("syncGlobalEnvScript: %v", err)
		}
		isi, err := os.ReadFile(jalur)
		if err != nil {
			t.Fatal(err)
		}
		distinct[string(isi)] = true
	}
	if len(distinct) != 1 {
		t.Fatalf("cli-env.sh tidak idempoten byte: %d varian berbeda dari 30 lari identik (B-281)", len(distinct))
	}
}
