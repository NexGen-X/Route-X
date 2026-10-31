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

	// Daftar putih rute yatim yang DISENGAJA, diverifikasi manual pada
	// audit ulang 1 Nov 2026 (klaster F11): rute-rute ini adalah alias
	// kompat lama berbentuk datar (kanonisnya berkelompok: /access/*,
	// /gateway/*, /upstreams/*, /system/*) yang tetap dipertahankan untuk
	// CLI dan klien pihak ketiga, ditambah alias ringkasan pendamping
	// (/summary, /overview, /diagnostics) yang juga dipakai non-dashboard.
	// Di luar daftar ini, rute backend TANPA pemanggil frontend adalah
	// temuan nyata dan MENGGAGALKAN tes ini — sebelum perbaikan F11 tes
	// ini hanya mencetak tanpa assertion sehingga tidak bisa gagal.
	orphanTerdaftar := map[string]bool{
		"/api/admin/api-keys":                     true,
		"/api/admin/api-keys/{param}/revoke":      true,
		"/api/admin/api-keys/{param}/toggle":      true,
		"/api/admin/budgets":                      true,
		"/api/admin/budgets/{param}":              true,
		"/api/admin/budgets/{param}/toggle":       true,
		"/api/admin/diagnostics":                  true,
		"/api/admin/egress-pools":                 true,
		"/api/admin/egress/pools":                 true,
		"/api/admin/overview":                     true,
		"/api/admin/providers":                    true,
		"/api/admin/providers/{param}/toggle":     true,
		"/api/admin/rate-limits":                  true,
		"/api/admin/rate-limits/{param}/toggle":   true,
		"/api/admin/ratelimits":                   true,
		"/api/admin/ratelimits/{param}/toggle":    true,
		"/api/admin/routing-rules":                true,
		"/api/admin/routing-rules/{param}/toggle": true,
		"/api/admin/routing/rules":                true,
		"/api/admin/routing/rules/{param}/toggle": true,
		"/api/admin/summary":                      true,
		"/api/admin/users":                        true,
		"/api/admin/users/{param}":                true,
		"/api/admin/webhooks":                     true,
		"/api/admin/webhooks/{param}/test":        true,
		"/api/admin/webhooks/{param}/toggle":      true,
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
