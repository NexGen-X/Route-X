package admin

import (
	"net/http"
	"strings"
	"time"

	"github.com/go-chi/chi/v5"
	"github.com/go-chi/chi/v5/middleware"

	"github.com/NexGen-X/Route-X/internal/auth"
	"github.com/NexGen-X/Route-X/internal/database/seed"
	"github.com/NexGen-X/Route-X/internal/httpx"
)

// batasWaktuAdmin membungkus middleware.Timeout(chi) dengan satu pengecualian:
// ekspor dan restore backup tidak tunduk pada tenggat generik admin (lihat
// komentar pemakaian di Routes). Pencocokan memakai sufiks path karena router
// ini di-mount di bawah /api/admin.
func batasWaktuAdmin(d time.Duration) func(http.Handler) http.Handler {
	denganTenggat := middleware.Timeout(d)
	return func(next http.Handler) http.Handler {
		terbungkus := denganTenggat(next)
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			if strings.HasSuffix(r.URL.Path, "/system/backup/export") ||
				strings.HasSuffix(r.URL.Path, "/system/backup/restore") {
				next.ServeHTTP(w, r)
				return
			}
			terbungkus.ServeHTTP(w, r)
		})
	}
}

// Routes mengembalikan router chi yang memuat seluruh 7 grup endpoint admin.
func (h *Handlers) Routes() http.Handler {
	r := chi.NewRouter()

	// Metode salah pada rute yang ada menjawab envelope error standar, bukan
	// body kosong bawaan chi (audit3 V-EM-06).
	r.MethodNotAllowed(httpx.MethodNotAllowedHandler)

	// Batas waktu operasi admin: 60 detik mencegah query agregasi atau database lambat
	// menahan koneksi dan goroutine server tanpa batas waktu. Ekspor & restore
	// backup DIKECUALIKAN (audit3 V-EM-07): keduanya operasi berkas yang sah
	// berjalan lebih dari 60 detik pada database besar, dan tenggat middleware
	// dulu membatalkan konteks handler di tengah — membunuh proses psql
	// restore dan memutus unduhan ekspor. Keduanya kini dibatasi tenggat
	// tulis server (WRITE_TIMEOUT) dan pembatalan oleh klien saja.
	r.Use(batasWaktuAdmin(60 * time.Second))

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
	// izin baca kanonis tetap bisa membaca datanya lewat alias. Sejak
	// B-102 pengguna non-admin adalah keadaan nyata (principal tanpa izin
	// apa pun), jadi kesamaan penjagaan ini berdampak langsung — alias
	// tidak boleh menjadi pintu yang lebih longgar dari pintunya.
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
