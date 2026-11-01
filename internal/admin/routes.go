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

	// Alias ringkasan baca tingkat atas (Fase 2 R5): hanya tiga alias GET
	// ini yang dipertahankan. Alias CRUD datar lainnya (/providers,
	// /api-keys, /users, /budgets, /ratelimits, /rate-limits,
	// /routing/rules, /routing-rules, /egress/pools, /egress-pools,
	// /webhooks — 23 rute) DIPANGKAS pada Fase 2 karena analisis konsumen
	// di dalam repo membuktikan nol pemanggil: dashboard hanya memakai
	// rute kanonis berkelompok, binary CLI (cmd/routex-apikey dan
	// cmd/routex-rotate) tidak memanggil API admin HTTP sama sekali,
	// skrip dan openapi.yaml hanya mendokumentasikan rute kanonis, dan
	// mobile companion APK yang dulu disebut komentar ini sudah dihapus.
	// Tiga alias baca ini dipertahankan karena pola pemakaiannya
	// (pemantauan/skrip kesehatan eksternal) tidak bisa dibuktikan nol
	// dari dalam repo dan biaya mempertahankannya kecil.
	r.Get("/overview", h.getSystemOverview)
	r.Get("/diagnostics", h.getDiagnostics)
	r.Get("/summary", h.getSummary)

	return r
}
