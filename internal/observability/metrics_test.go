package observability

import (
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

// scrape mengambil keluaran endpoint /metrics sebagai teks.
func scrape(t *testing.T, m *Metrics) string {
	t.Helper()
	rec := httptest.NewRecorder()
	m.Handler().ServeHTTP(rec, httptest.NewRequest(http.MethodGet, "/metrics", nil))

	if rec.Code != http.StatusOK {
		t.Fatalf("status /metrics = %d, mau 200", rec.Code)
	}
	body, err := io.ReadAll(rec.Body)
	if err != nil {
		t.Fatalf("baca body: %v", err)
	}
	return string(body)
}

func TestNewMetricsRegistersEverything(t *testing.T) {
	m := NewMetrics()

	// Beri nilai ke setiap metrik agar muncul di keluaran scrape.
	m.RequestsTotal.WithLabelValues("openai", "gpt-5", "200", "true").Inc()
	m.RequestDuration.WithLabelValues("openai", "gpt-5").Observe(1.5)
	m.TimeToFirstByte.WithLabelValues("openai", "gpt-5").Observe(0.21)
	m.TokensTotal.WithLabelValues("openai", "gpt-5", "input").Add(1200)
	m.CostTotal.WithLabelValues("openai", "gpt-5").Add(0.0123)
	m.UpstreamFailures.WithLabelValues("openai", "timeout").Inc()
	m.RetriesTotal.WithLabelValues("openai", "timeout").Inc()
	m.FailoversTotal.WithLabelValues("openai", "anthropic", "circuit_open").Inc()
	m.CircuitBreaker.WithLabelValues("openai", "gpt-5").Set(BreakerOpen)
	m.RateLimitRejected.WithLabelValues("api_key", "rpm").Inc()
	m.ContentBlocked.WithLabelValues("blocked_pattern").Inc()
	m.ProviderUp.WithLabelValues("openai").Set(BoolGauge(true))
	m.ProviderLatencyMS.WithLabelValues("openai").Set(84)
	m.ProviderAvailability.WithLabelValues("openai").Set(0.995)
	m.HTTPRequestsTotal.WithLabelValues("GET", "/api/requests", "200").Inc()
	m.HTTPDuration.WithLabelValues("GET", "/api/requests").Observe(0.03)
	m.HTTPInFlight.Set(3)
	m.WorkerRunsTotal.WithLabelValues("health_checker", "success").Inc()
	m.WorkerDuration.WithLabelValues("health_checker").Observe(0.4)
	m.PoolConnections.WithLabelValues("postgres", "idle").Set(5)
	m.UsageRecords.WithLabelValues("written").Inc()
	m.UsageQueueDepth.Set(12)

	body := scrape(t, m)

	want := []string{
		"routex_gateway_requests_total",
		"routex_gateway_request_duration_seconds",
		"routex_gateway_ttft_seconds",
		"routex_gateway_tokens_total",
		"routex_gateway_cost_usd_total",
		"routex_gateway_retries_total",
		"routex_gateway_failovers_total",
		"routex_circuit_breaker_state",
		"routex_rate_limit_rejected_total",
		"routex_content_filter_blocked_total",
		"routex_provider_up",
		"routex_provider_health_latency_ms",
		"routex_http_requests_total",
		"routex_http_request_duration_seconds",
		"routex_http_requests_in_flight",
		"routex_worker_runs_total",
		"routex_worker_duration_seconds",
		"routex_pool_connections",
		"routex_gateway_upstream_failures_total",
		"routex_provider_availability_ratio",
		"routex_usage_records_total",
		"routex_usage_queue_depth",
	}
	for _, name := range want {
		if !strings.Contains(body, name) {
			t.Errorf("metrik %s tidak ada di keluaran /metrics", name)
		}
	}

	// Kolektor runtime Go dan proses harus ikut terdaftar.
	for _, name := range []string{"go_goroutines", "process_open_fds"} {
		if !strings.Contains(body, name) {
			t.Errorf("kolektor bawaan %s tidak terdaftar", name)
		}
	}
}

// Registry terpisah per instance: dua kali NewMetrics tidak boleh panic karena
// pendaftaran ganda. Ini yang membuat test paralel aman.
func TestNewMetricsIsolatedRegistries(t *testing.T) {
	a := NewMetrics()
	b := NewMetrics()

	a.RequestsTotal.WithLabelValues("openai", "gpt-5", "200", "false").Add(7)

	if !strings.Contains(scrape(t, a), `routex_gateway_requests_total{model="gpt-5",provider="openai",status="200",stream="false"} 7`) {
		t.Error("nilai tidak terlihat di registry pertama")
	}
	if strings.Contains(scrape(t, b), `provider="openai"`) {
		t.Error("registry kedua ikut terkontaminasi nilai registry pertama")
	}
	if a.Registry() == b.Registry() {
		t.Error("dua instance berbagi registry yang sama")
	}
}

func TestStatusClass(t *testing.T) {
	tests := []struct {
		code int
		want string
	}{
		{100, "1xx"}, {200, "2xx"}, {204, "2xx"}, {301, "3xx"},
		{400, "4xx"}, {401, "4xx"}, {429, "4xx"},
		{500, "5xx"}, {502, "5xx"}, {503, "5xx"},
		{0, "1xx"},
	}
	for _, tc := range tests {
		if got := StatusClass(tc.code); got != tc.want {
			t.Errorf("StatusClass(%d) = %q, mau %q", tc.code, got, tc.want)
		}
	}
}

func TestStatusLabel(t *testing.T) {
	if got := StatusLabel(429); got != "429" {
		t.Errorf("StatusLabel(429) = %q, mau \"429\"", got)
	}
}

func TestBoolGauge(t *testing.T) {
	if BoolGauge(true) != 1 {
		t.Error("BoolGauge(true) != 1")
	}
	if BoolGauge(false) != 0 {
		t.Error("BoolGauge(false) != 0")
	}
}

func TestBreakerStateValues(t *testing.T) {
	// Nilainya dipakai di dashboard dan alert, jadi urutannya tidak boleh berubah
	// tanpa disadari.
	if BreakerClosed != 0 || BreakerHalfOpen != 1 || BreakerOpen != 2 {
		t.Errorf("nilai state breaker berubah: closed=%d half_open=%d open=%d",
			BreakerClosed, BreakerHalfOpen, BreakerOpen)
	}
}
