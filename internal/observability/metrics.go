package observability

import (
	"net/http"
	"strconv"

	"github.com/prometheus/client_golang/prometheus"
	"github.com/prometheus/client_golang/prometheus/collectors"
	"github.com/prometheus/client_golang/prometheus/promhttp"
)

// Bucket latensi dipisah untuk dua jenis pengukuran yang skalanya sangat berbeda:
// panggilan ke model bahasa bisa puluhan detik, sedangkan request ke dashboard
// biasanya di bawah satu detik.
var (
	// upstreamBuckets menutup rentang panggilan LLM: dari cache hit cepat sampai
	// generasi panjang yang mendekati batas timeout upstream (default 120s).
	upstreamBuckets = []float64{0.05, 0.1, 0.25, 0.5, 1, 2, 5, 10, 20, 30, 60, 120}

	// ttftBuckets menutup time-to-first-token, metrik yang paling dirasakan pengguna
	// pada respons streaming.
	ttftBuckets = []float64{0.025, 0.05, 0.1, 0.2, 0.4, 0.8, 1.6, 3.2, 6.4, 12.8}

	// adminBuckets untuk request dashboard dan API admin.
	adminBuckets = []float64{0.005, 0.01, 0.025, 0.05, 0.1, 0.25, 0.5, 1, 2.5, 5}
)

// Metrics memuat seluruh metrik Prometheus aplikasi beserta registry-nya sendiri.
//
// Registry dibuat terpisah (bukan prometheus.DefaultRegisterer) supaya test bisa
// membuat instance bersih tanpa bentrok pendaftaran ganda, dan supaya tidak ada
// metrik pihak ketiga yang ikut terekspos tanpa disengaja.
//
// Catatan kardinalitas: label provider dan model berasal dari registry model yang
// dikurasi operator, jadi jumlahnya terbatas. Jangan pernah menambahkan label yang
// nilainya berasal dari input klien bebas (misalnya API key, user ID, atau prompt) —
// itu akan meledakkan jumlah time series.
type Metrics struct {
	registry *prometheus.Registry

	// --- Lalu lintas gateway ---
	RequestsTotal   *prometheus.CounterVec
	RequestDuration *prometheus.HistogramVec
	TimeToFirstByte *prometheus.HistogramVec
	TokensTotal     *prometheus.CounterVec
	CostTotal       *prometheus.CounterVec
	// UpstreamFailures memecah kegagalan per KATEGORI, bukan per status HTTP. Tingkat
	// timeout tidak bisa dibaca dari RequestsTotal: timeout, koneksi terputus, dan 5xx
	// upstream semuanya menjadi 502 atau 504 di sisi klien, sehingga status HTTP saja
	// tidak bisa membedakan "provider lambat" dari "provider menolak".
	UpstreamFailures *prometheus.CounterVec

	// --- Ketahanan ---
	RetriesTotal      *prometheus.CounterVec
	FailoversTotal    *prometheus.CounterVec
	CircuitBreaker    *prometheus.GaugeVec
	RateLimitRejected *prometheus.CounterVec
	ContentBlocked    *prometheus.CounterVec

	// --- Kesehatan upstream ---
	ProviderUp        *prometheus.GaugeVec
	ProviderLatencyMS *prometheus.GaugeVec
	// ProviderAvailability berasal dari HASIL permintaan nyata, bukan dari health check.
	// Keduanya berbeda dan keduanya diperlukan: probe yang lolos tidak membuktikan
	// permintaan pelanggan berhasil, dan provider bisa sehat menurut probe sambil
	// menolak setiap permintaan sungguhan karena kuota.
	ProviderAvailability *prometheus.GaugeVec

	// --- HTTP admin & dashboard ---
	HTTPRequestsTotal *prometheus.CounterVec
	HTTPDuration      *prometheus.HistogramVec
	HTTPInFlight      prometheus.Gauge

	// --- Infrastruktur internal ---
	WorkerRunsTotal *prometheus.CounterVec
	WorkerDuration  *prometheus.HistogramVec
	PoolConnections *prometheus.GaugeVec

	// UsageRecords dan UsageQueueDepth adalah pertanggungjawaban pencatat pemakaian.
	// Pencatatan sengaja tidak boleh menggagalkan permintaan, jadi kegagalannya senyap
	// bagi pengguna — dan tanpa kedua angka ini, gateway bisa berjalan berhari-hari
	// tanpa mencatat satu pun baris pemakaian tanpa ada yang tahu. Alasannya sama dengan
	// metrik gagal-terbuka pada pembatas laju.
	UsageRecords    *prometheus.CounterVec
	UsageQueueDepth prometheus.Gauge
}

// NewMetrics membuat registry baru berisi seluruh metrik aplikasi, plus kolektor
// runtime Go dan proses.
func NewMetrics() *Metrics {
	reg := prometheus.NewRegistry()
	reg.MustRegister(
		collectors.NewGoCollector(),
		collectors.NewProcessCollector(collectors.ProcessCollectorOpts{}),
	)

	m := &Metrics{
		registry: reg,

		RequestsTotal: prometheus.NewCounterVec(prometheus.CounterOpts{
			Name: "routex_gateway_requests_total",
			Help: "Jumlah request gateway yang diproses, dipecah per provider, model, status, dan mode streaming.",
		}, []string{"provider", "model", "status", "stream"}),

		RequestDuration: prometheus.NewHistogramVec(prometheus.HistogramOpts{
			Name:    "routex_gateway_request_duration_seconds",
			Help:    "Durasi penuh request gateway sampai respons upstream selesai.",
			Buckets: upstreamBuckets,
		}, []string{"provider", "model"}),

		TimeToFirstByte: prometheus.NewHistogramVec(prometheus.HistogramOpts{
			Name:    "routex_gateway_ttft_seconds",
			Help:    "Time-to-first-token pada respons streaming.",
			Buckets: ttftBuckets,
		}, []string{"provider", "model"}),

		TokensTotal: prometheus.NewCounterVec(prometheus.CounterOpts{
			Name: "routex_gateway_tokens_total",
			Help: "Jumlah token yang dipakai, dipecah per arah (input/output).",
		}, []string{"provider", "model", "direction"}),

		CostTotal: prometheus.NewCounterVec(prometheus.CounterOpts{
			Name: "routex_gateway_cost_usd_total",
			Help: "Estimasi biaya kumulatif dalam USD berdasarkan harga di registry model.",
		}, []string{"provider", "model"}),

		UpstreamFailures: prometheus.NewCounterVec(prometheus.CounterOpts{
			Name: "routex_gateway_upstream_failures_total",
			Help: "Kegagalan upstream per kategori (timeout, network, rate_limit, auth, ...).",
		}, []string{"provider", "kind"}),

		RetriesTotal: prometheus.NewCounterVec(prometheus.CounterOpts{
			Name: "routex_gateway_retries_total",
			Help: "Jumlah percobaan ulang ke upstream, dipecah per alasan.",
		}, []string{"provider", "reason"}),

		FailoversTotal: prometheus.NewCounterVec(prometheus.CounterOpts{
			Name: "routex_gateway_failovers_total",
			Help: "Jumlah perpindahan otomatis dari satu provider ke provider berikutnya.",
		}, []string{"from_provider", "to_provider", "reason"}),

		CircuitBreaker: prometheus.NewGaugeVec(prometheus.GaugeOpts{
			Name: "routex_circuit_breaker_state",
			Help: "State circuit breaker per provider dan model: 0=closed, 1=half_open, 2=open.",
		}, []string{"provider", "model"}),

		RateLimitRejected: prometheus.NewCounterVec(prometheus.CounterOpts{
			Name: "routex_rate_limit_rejected_total",
			Help: "Jumlah request yang ditolak rate limiter, dipecah per cakupan dan jenis batas.",
		}, []string{"scope", "limit"}),

		ContentBlocked: prometheus.NewCounterVec(prometheus.CounterOpts{
			Name: "routex_content_filter_blocked_total",
			Help: "Jumlah request yang diblokir content filter, dipecah per aturan.",
		}, []string{"rule"}),

		ProviderUp: prometheus.NewGaugeVec(prometheus.GaugeOpts{
			Name: "routex_provider_up",
			Help: "Hasil health check terakhir per provider: 1=sehat, 0=tidak sehat.",
		}, []string{"provider"}),

		ProviderLatencyMS: prometheus.NewGaugeVec(prometheus.GaugeOpts{
			Name: "routex_provider_health_latency_ms",
			Help: "Latensi health check terakhir per provider dalam milidetik.",
		}, []string{"provider"}),

		ProviderAvailability: prometheus.NewGaugeVec(prometheus.GaugeOpts{
			Name: "routex_provider_availability_ratio",
			Help: "Rasio permintaan yang berhasil per provider pada jendela pengukuran terakhir, 0..1.",
		}, []string{"provider"}),

		HTTPRequestsTotal: prometheus.NewCounterVec(prometheus.CounterOpts{
			Name: "routex_http_requests_total",
			Help: "Request HTTP ke API admin dan dashboard.",
		}, []string{"method", "route", "status"}),

		HTTPDuration: prometheus.NewHistogramVec(prometheus.HistogramOpts{
			Name:    "routex_http_request_duration_seconds",
			Help:    "Durasi request HTTP admin dan dashboard.",
			Buckets: adminBuckets,
		}, []string{"method", "route"}),

		HTTPInFlight: prometheus.NewGauge(prometheus.GaugeOpts{
			Name: "routex_http_requests_in_flight",
			Help: "Jumlah request HTTP yang sedang diproses.",
		}),

		WorkerRunsTotal: prometheus.NewCounterVec(prometheus.CounterOpts{
			Name: "routex_worker_runs_total",
			Help: "Jumlah eksekusi background worker, dipecah per hasil.",
		}, []string{"worker", "outcome"}),

		WorkerDuration: prometheus.NewHistogramVec(prometheus.HistogramOpts{
			Name:    "routex_worker_duration_seconds",
			Help:    "Durasi eksekusi background worker.",
			Buckets: adminBuckets,
		}, []string{"worker"}),

		PoolConnections: prometheus.NewGaugeVec(prometheus.GaugeOpts{
			Name: "routex_pool_connections",
			Help: "Jumlah koneksi pool per backend (postgres, redis) dan state (total, idle, in_use).",
		}, []string{"backend", "state"}),

		UsageRecords: prometheus.NewCounterVec(prometheus.CounterOpts{
			Name: "routex_usage_records_total",
			Help: "Baris pemakaian yang diproses pencatat, dipecah per hasil (written, failed, dropped).",
		}, []string{"outcome"}),

		UsageQueueDepth: prometheus.NewGauge(prometheus.GaugeOpts{
			Name: "routex_usage_queue_depth",
			Help: "Jumlah catatan pemakaian yang menunggu ditulis ke database.",
		}),
	}

	reg.MustRegister(
		m.RequestsTotal, m.RequestDuration, m.TimeToFirstByte, m.TokensTotal, m.CostTotal,
		m.UpstreamFailures,
		m.RetriesTotal, m.FailoversTotal, m.CircuitBreaker, m.RateLimitRejected, m.ContentBlocked,
		m.ProviderUp, m.ProviderLatencyMS, m.ProviderAvailability,
		m.HTTPRequestsTotal, m.HTTPDuration, m.HTTPInFlight,
		m.WorkerRunsTotal, m.WorkerDuration, m.PoolConnections,
		m.UsageRecords, m.UsageQueueDepth,
	)
	return m
}

// Registry mengembalikan registry agar kolektor tambahan bisa didaftarkan.
func (m *Metrics) Registry() *prometheus.Registry { return m.registry }

// Handler menyajikan endpoint /metrics.
func (m *Metrics) Handler() http.Handler {
	return promhttp.HandlerFor(m.registry, promhttp.HandlerOpts{
		// Error saat scraping dilaporkan sebagai HTTP 500 agar terlihat di monitoring,
		// bukan diam-diam menghasilkan metrik yang tidak lengkap.
		ErrorHandling: promhttp.HTTPErrorOnError,
	})
}

// StatusLabel mengubah kode status HTTP menjadi label. Kode dipakai apa adanya karena
// jumlah kode yang mungkin terbatas, sehingga kardinalitasnya tetap aman.
func StatusLabel(code int) string { return strconv.Itoa(code) }

// StatusClass mengelompokkan kode status menjadi kelas (2xx, 4xx, 5xx) untuk panel
// yang tidak butuh detail per kode.
func StatusClass(code int) string {
	switch {
	case code >= 500:
		return "5xx"
	case code >= 400:
		return "4xx"
	case code >= 300:
		return "3xx"
	case code >= 200:
		return "2xx"
	default:
		return "1xx"
	}
}

// State circuit breaker sebagai nilai gauge. Nilainya numerik supaya bisa di-plot dan
// di-alert langsung tanpa pemetaan label tambahan.
const (
	BreakerClosed   = 0
	BreakerHalfOpen = 1
	BreakerOpen     = 2
)

// BoolGauge mengubah kondisi boolean menjadi nilai gauge Prometheus.
func BoolGauge(ok bool) float64 {
	if ok {
		return 1
	}
	return 0
}
