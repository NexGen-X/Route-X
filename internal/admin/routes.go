package admin

import (
	"net/http"
	"time"

	"github.com/go-chi/chi/v5"
	"github.com/go-chi/chi/v5/middleware"

	"github.com/NexGen-X/Route-X/internal/auth"
	"github.com/NexGen-X/Route-X/internal/database/seed"
	"github.com/NexGen-X/Route-X/internal/httpx"
)

// Routes mengembalikan router chi yang memuat seluruh 7 grup endpoint admin.
func (h *Handlers) Routes() http.Handler {
	r := chi.NewRouter()

	// Metode salah pada rute yang ada menjawab envelope error standar, bukan
	// body kosong bawaan chi (audit3 V-EM-06).
	r.MethodNotAllowed(httpx.MethodNotAllowedHandler)

	// Batas waktu operasi admin: 60 detik mencegah query agregasi atau database lambat
	// menahan koneksi dan goroutine server tanpa batas waktu.
	r.Use(middleware.Timeout(60 * time.Second))

	// Rantai middleware global admin:
	// 1. Wajib memiliki sesi login aktif di cookie (RequireSession).
	// 2. Wajib sudah menyelesaikan ganti password pertama kali (RequirePasswordChanged).
	// 3. Wajib lolos proteksi double-submit cookie CSRF untuk seluruh mutasi (POST/PUT/DELETE).
	//
	// Rantai ini dipasang UTUH atau TIDAK SAMA SEKALI (audit3 V-AU-12): versi
	// lama memasang tiap penjaga secara kondisional, jadi AuthSvc/CSRF yang
	// nil — salah pasang saat konstruksi — membuat SELURUH permukaan admin
	// (termasuk mutasi) berjalan tanpa autentikasi apa pun, tanpa satu pun
	// sinyal. Sekarang keadaan itu gagal-tertutup: semua rute menjawab 500
	// dan konstruktor mencatat error keras. Produksi tidak terpengaruh —
	// main selalu memasang keduanya.
	if h.authSvc != nil && h.csrf != nil {
		r.Use(h.authSvc.RequireSession())
		r.Use(auth.RequirePasswordChanged())
		r.Use(h.csrf.Protect())
	} else {
		r.Use(h.tolakSemuaAdmin)
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
	// Izin alias disamakan PERSIS dengan rute kanonisnya (audit3 V-AU-14):
	// sebelumnya ketiga alias ini tidak membawa RequirePermission sama
	// sekali, jadi setiap peran non-admin di masa depan yang tidak memegang
	// izin baca kanonis tetap bisa membaca datanya lewat alias. Hari ini
	// dampaknya nol (single-admin: principal selalu memegang "*"), tetapi
	// alias tidak boleh menjadi pintu yang lebih longgar dari pintunya.
	r.With(auth.RequirePermission(seed.PermHealthRead)).Get("/overview", h.getSystemOverview)
	r.With(auth.RequirePermission(seed.PermHealthRead)).Get("/diagnostics", h.getDiagnostics)
	r.With(auth.RequirePermission(seed.PermUsageRead)).Get("/summary", h.getSummary)

	return r
}

// tolakSemuaAdmin menolak setiap permintaan admin dengan 500.
//
// Dipasang oleh Routes menggantikan rantai autentikasi bila layanan auth
// atau penjaga CSRF tidak terpasang pada Handlers: permukaan yang tidak bisa
// membuktikan siapa pemanggilnya tidak boleh melayani apa pun (lihat
// komentar di Routes). Kesalahannya dicatat pada level Error di konstruktor
// dan di sini, supaya salah-pasang itu terlihat, bukan diam.
func (h *Handlers) tolakSemuaAdmin(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		h.logger.ErrorContext(r.Context(), "permintaan admin ditolak: rantai autentikasi tidak terpasang pada Handlers",
			"method", r.Method, "path", r.URL.Path)
		httpx.InternalError(w, r)
	})
}
