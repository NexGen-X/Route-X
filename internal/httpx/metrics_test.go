package httpx

import (
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/NexGen-X/Route-X/internal/observability"
	"github.com/go-chi/chi/v5"
)

func TestMetricsRecorder_Nil(t *testing.T) {
	mw := MetricsRecorder(nil)
	handler := mw(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
	}))

	req := httptest.NewRequest(http.MethodGet, "/test", nil)
	rec := httptest.NewRecorder()
	handler.ServeHTTP(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d, mau 200", rec.Code)
	}
}

func TestMetricsRecorder_NormalRoute(t *testing.T) {
	m := observability.NewMetrics()
	r := chi.NewRouter()
	r.Use(MetricsRecorder(m))

	r.Get("/items/{id}", func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
		_, _ = w.Write([]byte("ok"))
	})

	req := httptest.NewRequest(http.MethodGet, "/items/42", nil)
	rec := httptest.NewRecorder()
	r.ServeHTTP(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d, mau 200", rec.Code)
	}

	// Cek apakah pattern rute tercatat dengan benar sebagai /items/{id} dan bukan /items/42
	reg := m.Registry()
	mfs, err := reg.Gather()
	if err != nil {
		t.Fatalf("gather: %v", err)
	}

	var foundReq, foundDur bool
	for _, mf := range mfs {
		if mf.GetName() == "routex_http_requests_total" {
			for _, metric := range mf.GetMetric() {
				labels := make(map[string]string)
				for _, lp := range metric.GetLabel() {
					labels[lp.GetName()] = lp.GetValue()
				}
				if labels["route"] == "/items/{id}" && labels["method"] == "GET" && labels["status"] == "200" {
					foundReq = true
					if metric.GetCounter().GetValue() != 1 {
						t.Errorf("counter val = %v, mau 1", metric.GetCounter().GetValue())
					}
				}
			}
		}
		if mf.GetName() == "routex_http_request_duration_seconds" {
			for _, metric := range mf.GetMetric() {
				labels := make(map[string]string)
				for _, lp := range metric.GetLabel() {
					labels[lp.GetName()] = lp.GetValue()
				}
				if labels["route"] == "/items/{id}" && labels["method"] == "GET" {
					foundDur = true
				}
			}
		}
	}

	if !foundReq {
		t.Error("routex_http_requests_total dengan route /items/{id} tidak ditemukan")
	}
	if !foundDur {
		t.Error("routex_http_request_duration_seconds dengan route /items/{id} tidak ditemukan")
	}
}

func TestMetricsRecorder_NotFoundRoute(t *testing.T) {
	m := observability.NewMetrics()
	r := chi.NewRouter()
	r.NotFound(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusNotFound)
		_, _ = w.Write([]byte("not found"))
	})
	handler := MetricsRecorder(m)(r)

	req := httptest.NewRequest(http.MethodGet, "/nonexistent/path/123", nil)
	rec := httptest.NewRecorder()
	handler.ServeHTTP(rec, req)

	if rec.Code != http.StatusNotFound {
		t.Fatalf("status = %d, mau 404", rec.Code)
	}

	mfs, err := m.Registry().Gather()
	if err != nil {
		t.Fatalf("gather: %v", err)
	}

	var recordedRoute string
	for _, mf := range mfs {
		if mf.GetName() == "routex_http_requests_total" {
			for _, metric := range mf.GetMetric() {
				for _, lp := range metric.GetLabel() {
					if lp.GetName() == "route" {
						recordedRoute = lp.GetValue()
					}
				}
			}
		}
	}

	t.Logf("Route label untuk 404 / NotFound: %q", recordedRoute)
	if recordedRoute != "unmatched" {
		t.Errorf("recordedRoute = %q, mau \"unmatched\"", recordedRoute)
	}
}

func TestSanitizeMethod(t *testing.T) {
	tests := []struct {
		input string
		want  string
	}{
		{"GET", "GET"},
		{"POST", "POST"},
		{"PUT", "PUT"},
		{"DELETE", "DELETE"},
		{"PATCH", "PATCH"},
		{"HEAD", "HEAD"},
		{"OPTIONS", "OPTIONS"},
		{"CONNECT", "CONNECT"},
		{"TRACE", "TRACE"},
		{"", "GET"},
		{"CUSTOM_METHOD", "OTHER"},
		{"FOOBAR", "OTHER"},
		{"get", "OTHER"}, // HTTP method bersifat case-sensitive menurut RFC 7230
	}

	for _, tc := range tests {
		if got := sanitizeMethod(tc.input); got != tc.want {
			t.Errorf("sanitizeMethod(%q) = %q, mau %q", tc.input, got, tc.want)
		}
	}
}

func TestMetricsRecorder_RandomMethodNormalized(t *testing.T) {
	m := observability.NewMetrics()
	r := chi.NewRouter()
	r.Use(MetricsRecorder(m))

	r.Handle("/ping", http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
	}))

	req := httptest.NewRequest("RANDOM_EXPLOIT_METHOD", "/ping", nil)
	rec := httptest.NewRecorder()
	r.ServeHTTP(rec, req)

	mfs, err := m.Registry().Gather()
	if err != nil {
		t.Fatalf("gather: %v", err)
	}

	var foundOther bool
	for _, mf := range mfs {
		if mf.GetName() == "routex_http_requests_total" {
			for _, metric := range mf.GetMetric() {
				for _, lp := range metric.GetLabel() {
					if lp.GetName() == "method" && lp.GetValue() == "OTHER" {
						foundOther = true
					}
				}
			}
		}
	}

	if !foundOther {
		t.Error("metode HTTP acak tidak dinormalkan ke \"OTHER\" pada routex_http_requests_total")
	}
}
