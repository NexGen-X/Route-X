package httpx

import (
	"net/http"
	"strconv"
	"strings"
	"sync/atomic"
	"time"

	"github.com/NexGen-X/Route-X/internal/observability"
	"github.com/go-chi/chi/v5"
)

// gatewayDalamPenerbangan menghitung request /v1 yang SEDANG dilayani saat
// ini juga. Inilah angka yang dijanjikan label "in-flight" di ringkasan
// sistem — bukan jumlah koneksi database yang sedang terpakai, yang dulu
// dilaporkan di bawah nama itu (audit3 V-FJ-06): koneksi pool mencakup
// worker latar dan query admin, dan tidak pernah nol walau gateway menganggur.
var gatewayDalamPenerbangan atomic.Int64

// TrackGatewayInFlight menaikkan penghitung selama handler melayani, lalu
// menurunkannya — termasuk saat handler panik (defer tetap berjalan sebelum
// middleware pemulih di atasnya menjawab 500).
func TrackGatewayInFlight(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		gatewayDalamPenerbangan.Add(1)
		defer gatewayDalamPenerbangan.Add(-1)
		next.ServeHTTP(w, r)
	})
}

// GatewayInFlight mengembalikan jumlah request gateway yang sedang dilayani.
func GatewayInFlight() int64 {
	return gatewayDalamPenerbangan.Load()
}

// jalurTanpaMetrikHTTP adalah jalur yang tidak dicatat metrik HTTP admin:
// probe kesehatan dan endpoint metrik itu sendiri. Mencatatnya mencemari
// histogram admin (bucket maksimum 5 detik) dan membuat metrik tentang
// metrik — derau yang menutupi sinyal dashboard (audit3 V-EM-26).
var jalurTanpaMetrikHTTP = map[string]bool{
	"/healthz": true,
	"/readyz":  true,
	"/metrics": true,
}

// tanpaMetrikHTTP melaporkan apakah satu request dilewati pencatatan metrik
// HTTP admin/dashboard. Selain jalur probe di atas, SELURUH permukaan /v1
// dilewati: middleware ini terpasang di router akar sehingga ikut mencatat
// lalu lintas gateway, padahal histogramnya dinyatakan untuk "request HTTP
// admin dan dashboard" (bucket maksimum 5 detik) — setiap stream LLM yang
// sah berdurasi >5 detik menumpuk di bucket +Inf dan mengaburkan latensi
// admin yang sebenarnya (audit3 V-EM-26). Lalu lintas /v1 punya keluarga
// metriknya sendiri (routex_gateway_*).
func tanpaMetrikHTTP(path string) bool {
	if jalurTanpaMetrikHTTP[path] {
		return true
	}
	return path == "/v1" || strings.HasPrefix(path, "/v1/")
}

// MetricsRecorder adalah middleware untuk mencatat request HTTP ke Prometheus.
// Membaca path rute asli dari Chi (mis. /api/users/{id}) untuk mencegah ledakan kardinalitas.
func MetricsRecorder(m *observability.Metrics) func(http.Handler) http.Handler {
	// m nil berarti metrik dimatikan (mis. test ringan): teruskan saja tanpa
	// mencatat, JANGAN panic nil-dereference pada setiap request.
	if m == nil {
		return func(next http.Handler) http.Handler { return next }
	}
	return func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			if tanpaMetrikHTTP(r.URL.Path) {
				next.ServeHTTP(w, r)
				return
			}
			m.HTTPInFlight.Inc()
			defer m.HTTPInFlight.Dec()

			start := time.Now()

			// Membungkus response writer untuk mendapatkan status code
			rw := wrapResponseWriter(w)

			next.ServeHTTP(rw, r)

			// Mendapatkan pattern rute dari chi context (jika ada).
			routeCtx := chi.RouteContext(r.Context())
			routePattern := "unknown"
			if routeCtx != nil && routeCtx.RoutePattern() != "" {
				routePattern = routeCtx.RoutePattern()
			} else {
				routePattern = "unmatched"
			}

			method := sanitizeMethod(r.Method)
			status := strconv.Itoa(rw.status)

			m.HTTPRequestsTotal.WithLabelValues(method, routePattern, status).Inc()
			m.HTTPDuration.WithLabelValues(method, routePattern).Observe(time.Since(start).Seconds())
		})
	}
}

// sanitizeMethod menormalkan metode HTTP ke kumpulan metode standar RFC untuk mencegah
// ledakan kardinalitas metrik akibat request dengan metode acak dari pemindai/penyerang.
func sanitizeMethod(method string) string {
	switch method {
	case http.MethodGet, http.MethodPost, http.MethodPut, http.MethodDelete,
		http.MethodPatch, http.MethodHead, http.MethodOptions, http.MethodConnect, http.MethodTrace:
		return method
	case "":
		return http.MethodGet
	default:
		return "OTHER"
	}
}
