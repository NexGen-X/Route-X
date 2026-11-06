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

	// Daftar putih rute yatim yang DISENGAJA, dua kategori:
	//
	// (1) Tiga alias ringkasan baca (/summary, /overview, /diagnostics)
	// yang dipertahankan sesudah pemangkasan alias Fase 2 R5 — 23 alias
	// CRUD datar dihapus dari routes.go setelah analisis membuktikan
	// nol konsumen di dalam repo.
	//
	// (2) Rute-rute B-286 (Fase 14): wrapper klien web untuk permukaan
	// ini dihapus dari client.ts karena nol pemanggil UI (grup
	// filters/bans/sessions utuh + metode sebaran), TETAPI rutenya
	// SENGAJA dipertahankan hidup sebagai permukaan API langsung —
	// keputusan entri B-286 membatasi penghapusan pada wrapper klien
	// dan menyatakan rute backend tidak disentuh. Pendaftaran di sini
	// mendokumentasikan kesengajaan itu; gerbang tetap menggagalkan
	// tes untuk yatim BARU apa pun di luar daftar.
	//
	// Di luar daftar ini, rute backend TANPA pemanggil frontend adalah
	// temuan nyata dan MENGGAGALKAN tes ini — sebelum perbaikan F11 tes
	// ini hanya mencetak tanpa assertion sehingga tidak bisa gagal.
	orphanTerdaftar := map[string]bool{
		"/api/admin/diagnostics": true,
		"/api/admin/overview":    true,
		"/api/admin/summary":     true,
		// Kategori (2) — B-286:
		"/api/admin/access/sessions":                           true,
		"/api/admin/access/sessions/{param}":                   true,
		"/api/admin/access/sessions/revoke-all":                true,
		"/api/admin/gateway/bans":                              true,
		"/api/admin/gateway/bans/{param}/lift":                 true,
		"/api/admin/gateway/content-filters":                   true,
		"/api/admin/gateway/content-filters/{param}":           true,
		"/api/admin/gateway/content-filters/{param}/toggle":    true,
		"/api/admin/gateway/rate-limits/{param}/toggle":        true,
		"/api/admin/observability/metrics/live":                true,
		"/api/admin/requests/{param}":                          true,
		"/api/admin/upstreams/models/{param}":                  true,
		"/api/admin/upstreams/models/{param}/aliases/{param}":  true,
		"/api/admin/upstreams/models/{param}/mappings":         true,
		"/api/admin/upstreams/models/mappings/{param}":         true,
		"/api/admin/upstreams/providers/{param}/health-checks": true,
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
