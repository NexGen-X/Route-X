package admin

import (
	"context"
	"log/slog"
	"net/http"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/NexGen-X/Route-X/internal/cliconfig"
	"github.com/NexGen-X/Route-X/internal/database/repo/identity"
)

// FASE 14 B-275 (PB-30): configureCLI menerima gateway_url dari body yang
// mengalahkan hierarki tepercaya (setting DB → PUBLIC_URL → turunan
// request), tersimpan persisten ke konfigurasi CLI bersama API key, dan
// tidak teraudit. Kini field body itu diabaikan sepenuhnya: URL yang
// tersimpan selalu hasil resolveGatewayURL. Test ini memasok URL asing
// lewat body dan membuktikan URL asing itu TIDAK muncul di baris
// settings maupun di berkas konfigurasi yang ditulis ke HOME.

func TestFase14B275GatewayURLBodyDiabaikan(t *testing.T) {
	env := setupTestEnv(t)
	home := t.TempDir()
	t.Setenv("HOME", home)

	// Harness baku tidak memasang CLIManager; pasang manajer sungguhan
	// di atas pool & cipher yang sama supaya Configure berjalan penuh.
	env.handlers.cliManager = cliconfig.NewManager(
		identity.NewSettings(env.pool), env.cipher,
		slog.New(slog.DiscardHandler))

	client := env.newClient(t)
	client.login("superadmin@routex.internal", testPassword)

	urlAsing := "https://asing.example/v1"
	res, body := client.do(http.MethodPost, "/api/admin/cli/configure",
		map[string]any{
			"tool_id":     "aider",
			"mode":        "model_only",
			"target":      "model-uji-b275",
			"gateway_url": urlAsing,
		}, true)
	if res.StatusCode != http.StatusOK {
		t.Fatalf("status = %d, mau 200: %s", res.StatusCode, body)
	}

	// URL yang sah adalah turunan request harness ini (jenjang ke-3
	// hierarki): skema http + host server uji + /v1.
	urlSah := client.server.URL + "/v1"

	// 1) Baris settings persisten memuat URL hierarki, bukan URL body.
	var mentah string
	err := env.pool.QueryRow(context.Background(),
		`select value::text from settings where key = 'cli:config:aider'`).
		Scan(&mentah)
	if err != nil {
		t.Fatalf("baris settings cli:config:aider tidak ditemukan: %v", err)
	}
	if strings.Contains(mentah, "asing.example") {
		t.Fatalf("URL body tersimpan persisten di settings (B-275): %s", mentah)
	}
	if !strings.Contains(mentah, urlSah) {
		t.Fatalf("settings tidak memuat URL hierarki %q: %s", urlSah, mentah)
	}

	// 2) Berkas konfigurasi di HOME juga bersih dari URL body dan
	// memuat URL hierarki.
	for _, rel := range []string{
		filepath.Join(".routex", "cli-env.sh"),
		".aider.conf.yml",
	} {
		isi, err := os.ReadFile(filepath.Join(home, rel))
		if err != nil {
			t.Fatalf("berkas %s tidak tertulis: %v", rel, err)
		}
		if strings.Contains(string(isi), "asing.example") {
			t.Fatalf("URL body tertulis di %s (B-275): %s", rel, isi)
		}
		if !strings.Contains(string(isi), urlSah) {
			t.Fatalf("%s tidak memuat URL hierarki %q: %s", rel, urlSah, isi)
		}
	}
}
