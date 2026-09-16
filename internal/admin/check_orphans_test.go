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

	for normServer := range registeredRoutes {
		if !clientEndpoints[normServer] {
			fmt.Printf("ORPHAN BACKEND ROUTE (No frontend call found): %s\n", normServer)
		}
	}
}
