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

	return r
}
