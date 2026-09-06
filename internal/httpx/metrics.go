package httpx

import (
	"net/http"
	"strconv"
	"time"

	"github.com/NexGen-X/Route-X/internal/observability"
	"github.com/go-chi/chi/v5"
)

// MetricsRecorder adalah middleware untuk mencatat request HTTP ke Prometheus.
// Membaca path rute asli dari Chi (mis. /api/users/{id}) untuk mencegah ledakan kardinalitas.
func MetricsRecorder(m *observability.Metrics) func(http.Handler) http.Handler {
	return func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
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

			method := r.Method
			if method == "" {
				method = "GET"
			}
			status := strconv.Itoa(rw.status)

			m.HTTPRequestsTotal.WithLabelValues(method, routePattern, status).Inc()
			m.HTTPDuration.WithLabelValues(method, routePattern).Observe(time.Since(start).Seconds())
		})
	}
}
