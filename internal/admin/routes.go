package admin

import (
	"net/http"
	"time"

	"github.com/go-chi/chi/v5"
	"github.com/go-chi/chi/v5/middleware"

	"github.com/NexGen-X/Route-X/internal/auth"
)

// Routes mengembalikan router chi yang memuat seluruh 7 grup endpoint admin.
func (h *Handlers) Routes() http.Handler {
	r := chi.NewRouter()

	// Batas waktu operasi admin: 60 detik mencegah query agregasi atau database lambat
	// menahan koneksi dan goroutine server tanpa batas waktu.
	r.Use(middleware.Timeout(60 * time.Second))

	// Rantai middleware global admin:
	// 1. Wajib memiliki sesi login aktif di cookie (RequireSession).
	// 2. Wajib sudah menyelesaikan ganti password pertama kali (RequirePasswordChanged).
	// 3. Wajib lolos proteksi double-submit cookie CSRF untuk seluruh mutasi (POST/PUT/DELETE).
	if h.authSvc != nil {
		r.Use(h.authSvc.RequireSession())
	}
	r.Use(auth.RequirePasswordChanged())

	if h.csrf != nil {
		r.Use(h.csrf.Protect())
	}

	// Pemasangan rute per domain administratif
	r.Route("/observability", h.observabilityRoutes)
	r.Route("/requests", h.requestsRoutes)
	r.Route("/upstreams", h.upstreamsRoutes)
	r.Route("/gateway", h.gatewayRoutes)
	r.Route("/access", h.accessRoutes)
	r.Route("/automation", h.automationRoutes)
	r.Route("/system", h.systemRoutes)
	r.Route("/cli", h.cliRoutes)

	// Top-level aliases untuk kenyamanan mobile companion APK, CLI, dan integrasi pihak ketiga
	r.Get("/overview", h.getSystemOverview)
	r.Get("/diagnostics", h.getDiagnostics)
	r.Get("/summary", h.getSummary)

	r.Route("/providers", func(sub chi.Router) {
		sub.Get("/", h.listProviders)
		sub.Post("/", h.createProvider)
		sub.Post("/{id}/toggle", h.toggleProvider)
	})

	r.Route("/api-keys", func(sub chi.Router) {
		sub.Get("/", h.listAPIKeys)
		sub.Post("/", h.createAPIKey)
		sub.Post("/{id}/revoke", h.revokeAPIKey)
		sub.Post("/{id}/toggle", h.toggleAPIKey)
	})

	r.Route("/users", func(sub chi.Router) {
		sub.Get("/", h.listUsers)
		sub.Post("/", h.createUser)
		sub.Delete("/{id}", h.deleteUser)
	})

	r.Route("/egress/pools", func(sub chi.Router) {
		sub.Get("/", h.listEgressPools)
		sub.Post("/", h.createEgressPool)
	})
	r.Route("/egress-pools", func(sub chi.Router) {
		sub.Get("/", h.listEgressPools)
		sub.Post("/", h.createEgressPool)
	})

	r.Route("/budgets", func(sub chi.Router) {
		sub.Get("/", h.listBudgets)
		sub.Post("/", h.createBudget)
		sub.Put("/{id}", h.updateBudget)
		sub.Post("/{id}/toggle", h.toggleBudget)
	})

	r.Route("/ratelimits", func(sub chi.Router) {
		sub.Get("/", h.listRateLimits)
		sub.Post("/", h.createRateLimit)
		sub.Post("/{id}/toggle", h.toggleRateLimit)
	})
	r.Route("/rate-limits", func(sub chi.Router) {
		sub.Get("/", h.listRateLimits)
		sub.Post("/", h.createRateLimit)
		sub.Post("/{id}/toggle", h.toggleRateLimit)
	})

	r.Route("/routing/rules", func(sub chi.Router) {
		sub.Get("/", h.listRoutingRules)
		sub.Post("/", h.createRoutingRule)
		sub.Post("/{id}/toggle", h.toggleRoutingRule)
	})
	r.Route("/routing-rules", func(sub chi.Router) {
		sub.Get("/", h.listRoutingRules)
		sub.Post("/", h.createRoutingRule)
		sub.Post("/{id}/toggle", h.toggleRoutingRule)
	})

	r.Route("/webhooks", func(sub chi.Router) {
		sub.Get("/", h.listWebhooks)
		sub.Post("/", h.createWebhook)
		sub.Post("/{id}/toggle", h.toggleWebhook)
		sub.Post("/{id}/test", h.testWebhookPing)
	})

	return r
}
