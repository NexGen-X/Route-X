package admin

import (
	"fmt"
	"github.com/go-chi/chi/v5"
	"net/http"
	"os"
	"path/filepath"
	"regexp"
	"strings"
	"testing"
)

func TestCheckOrphans(t *testing.T) {
	clientTSPath := filepath.Join("..", "..", "web", "src", "api", "client.ts")
	content, _ := os.ReadFile(clientTSPath)

	re := regexp.MustCompile(`['"\x60](/api/(?:admin|auth)/[^'"\x60?]+)`)
	matches := re.FindAllStringSubmatch(string(content), -1)

	clientEndpoints := make(map[string]bool)
	for _, m := range matches {
		endpoint := m[1]
		endpoint = regexp.MustCompile(`\$\{q[^}]*\}`).ReplaceAllString(endpoint, "")
		endpoint = regexp.MustCompile(`\$\{[^}]+\}`).ReplaceAllString(endpoint, "{param}")
		clientEndpoints[strings.TrimSuffix(endpoint, "/")] = true
	}

	h := NewHandlers(Config{})
	serverRouter := chi.NewRouter()
	serverRouter.Mount("/api/admin", h.Routes())

	registeredRoutes := make(map[string]bool)
	_ = chi.Walk(serverRouter, func(method, route string, handler http.Handler, middlewares ...func(http.Handler) http.Handler) error {
		normalized := regexp.MustCompile(`\{[^}]+\}`).ReplaceAllString(route, "{param}")
		normalized = strings.TrimSuffix(normalized, "/")
		registeredRoutes[normalized] = true
		return nil
	})

	// Daftar putih rute yatim yang DISENGAJA: hanya tiga alias ringkasan
	// baca (/summary, /overview, /diagnostics) yang dipertahankan sesudah
	// pemangkasan alias Fase 2 R5 — 23 alias CRUD datar dihapus dari
	// routes.go setelah analisis membuktikan nol konsumen di dalam repo.
	// Di luar daftar ini, rute backend TANPA pemanggil frontend adalah
	// temuan nyata dan MENGGAGALKAN tes ini — sebelum perbaikan F11 tes
	// ini hanya mencetak tanpa assertion sehingga tidak bisa gagal.
	orphanTerdaftar := map[string]bool{
		"/api/admin/diagnostics": true,
		"/api/admin/overview":    true,
		"/api/admin/summary":     true,
	}

	var yatimBaru []string
	for normServer := range registeredRoutes {
		if clientEndpoints[normServer] {
			continue
		}
		if orphanTerdaftar[normServer] {
			fmt.Printf("ORPHAN BACKEND ROUTE (terdaftar/disengaja): %s\n", normServer)
			continue
		}
		yatimBaru = append(yatimBaru, normServer)
	}
	if len(yatimBaru) > 0 {
		t.Errorf("ditemukan %d rute backend yatim BARU tanpa pemanggil frontend dan tidak terdaftar sebagai alias sengaja: %v — panggil dari frontend, hapus rutenya, atau daftarkan di orphanTerdaftar dengan alasan",
			len(yatimBaru), yatimBaru)
	}
}
