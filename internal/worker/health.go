package worker

import (
	"context"
	"fmt"
	"log/slog"
	"sync"
	"time"

	"github.com/jackc/pgx/v5/pgxpool"

	"github.com/NexGen-X/Route-X/internal/database/repo/upstream"
	"github.com/NexGen-X/Route-X/internal/observability"
	"github.com/NexGen-X/Route-X/internal/providers"
	"github.com/NexGen-X/Route-X/internal/webhooks"
)

// ProviderSource menyediakan daftar provider dan pencatatan riwayat kesehatan.
type ProviderSource interface {
	ActiveProviders(ctx context.Context) ([]*upstream.Provider, error)
	RecordHealth(ctx context.Context, providerID string, h upstream.HealthReport) error
}

// ProviderFactory memasok adapter provider siap pakai. Dipenuhi oleh *gateway.Factory.
type ProviderFactory interface {
	ProviderFor(ctx context.Context, p *upstream.Provider) (providers.Provider, error)
}

// HealthCheckerJob memeriksa kesehatan upstream provider secara berkala.
//
// Sesuai aturan 14:
// - Menggunakan pabrik (gateway.Factory.ProviderFor) yang sama dengan jalur inferensi.
// - Menghindari penyusunan RouteCandidate palsu yang mengarang pengenal model.
// - Berbagi penjaga SSRF dan egress proxy yang sama persis dengan lalu lintas produksi.
type HealthCheckerJob struct {
	pool     *pgxpool.Pool
	source   ProviderSource
	factory  ProviderFactory
	enqueuer webhooks.EnqueueSink
	metrics  *observability.Metrics
	logger   *slog.Logger

	mu         sync.Mutex
	lastStatus map[string]string
}

// NewHealthCheckerJob membuat instance baru HealthCheckerJob.
func NewHealthCheckerJob(
	pool *pgxpool.Pool,
	source ProviderSource,
	factory ProviderFactory,
	enqueuer webhooks.EnqueueSink,
	metrics *observability.Metrics,
	logger *slog.Logger,
) *HealthCheckerJob {
	if logger == nil {
		logger = slog.Default()
	}
	return &HealthCheckerJob{
		pool:       pool,
		source:     source,
		factory:    factory,
		enqueuer:   enqueuer,
		metrics:    metrics,
		logger:     logger,
		lastStatus: make(map[string]string),
	}
}

func (h *HealthCheckerJob) Name() string { return "provider_health_checker" }

// Run mengeksekusi satu putaran health check untuk seluruh provider yang aktif.
func (h *HealthCheckerJob) Run(ctx context.Context) error {
	unlock, ok, err := TryAdvisoryLock(ctx, h.pool, LockHealth)
	if err != nil {
		return fmt.Errorf("advisory lock health check: %w", err)
	}
	if !ok {
		// Instance lain sedang menjalankan health check, lewati putaran ini dengan tenang
		return nil
	}
	defer unlock()

	list, err := h.source.ActiveProviders(ctx)
	if err != nil {
		return fmt.Errorf("mengambil daftar provider aktif: %w", err)
	}

	for _, p := range list {

		if !p.Enabled {
			continue
		}
		h.checkOne(ctx, p)
	}

	return nil
}

// checkOne memeriksa satu provider dan mencatat perubahannya.
func (h *HealthCheckerJob) checkOne(ctx context.Context, p *upstream.Provider) {
	adapter, err := h.factory.ProviderFor(ctx, p)
	if err != nil {
		h.logger.WarnContext(ctx, "gagal menyiapkan adapter provider untuk health check",
			"provider_id", p.ID, "provider_name", p.Name, "error", err)
		h.recordResult(ctx, p, providers.HealthResult{
			Healthy:      false,
			Latency:      0,
			ErrorKind:    providers.ErrKindNetwork,
			ErrorMessage: err.Error(),
		})
		return
	}

	// Buat context dengan batas waktu timeout provider
	timeout := time.Duration(p.TimeoutMS) * time.Millisecond
	if timeout <= 0 {
		timeout = 10 * time.Second
	}
	chkCtx, cancel := context.WithTimeout(ctx, timeout)
	defer cancel()

	result := adapter.HealthCheck(chkCtx)
	h.recordResult(ctx, p, result)
}

// recordResult menyimpan status ke database, metrik Prometheus, dan memproduksi event webhook jika status berganti.
func (h *HealthCheckerJob) recordResult(ctx context.Context, p *upstream.Provider, res providers.HealthResult) {
	status := "unhealthy"
	if res.Healthy {
		status = "healthy"
	}

	var sc *int
	if res.StatusCode > 0 {
		val := res.StatusCode
		sc = &val
	}

	// Aturan 9: Error message wajib disaring agar tidak membocorkan URL mentah dengan query string.
	sanitizedErr := webhooks.SanitizeErrorMessage(res.ErrorMessage)

	latMS := int(res.Latency.Milliseconds())
	report := upstream.HealthReport{
		Status:       status,
		LatencyMS:    &latMS,
		StatusCode:   sc,
		ErrorKind:    string(res.ErrorKind),
		ErrorMessage: sanitizedErr,
	}

	if err := h.source.RecordHealth(ctx, p.ID, report); err != nil {
		h.logger.WarnContext(ctx, "gagal mencatat hasil health check ke database",
			"provider_id", p.ID, "provider_name", p.Name, "error", err)
	}

	// Perbarui metrik Prometheus
	if h.metrics != nil {
		upVal := 0.0
		if res.Healthy {
			upVal = 1.0
		}
		h.metrics.ProviderUp.WithLabelValues(p.Name).Set(upVal)
		h.metrics.ProviderLatencyMS.WithLabelValues(p.Name).Set(float64(res.Latency.Milliseconds()))
	}

	// Deteksi perpindahan status untuk memproduksi event webhook
	h.mu.Lock()
	prevStatus, known := h.lastStatus[p.ID]
	h.lastStatus[p.ID] = status
	h.mu.Unlock()

	// Hanya produksi event saat terjadi transisi konkret
	if known && prevStatus != status && h.enqueuer != nil {
		if status == "unhealthy" {
			payload := map[string]any{
				"event":         "provider.unhealthy",
				"provider_id":   p.ID,
				"provider_name": p.Name,
				"status":        "unhealthy",
				"error_kind":    string(res.ErrorKind),
				"error_message": sanitizedErr,
				"timestamp":     time.Now().UTC().Format(time.RFC3339),
			}
			if _, err := h.enqueuer.Enqueue(ctx, "provider.unhealthy", payload); err != nil {
				h.logger.WarnContext(ctx, "gagal memasukkan event provider.unhealthy ke antrean webhook",
					"provider", p.Name, "error", err)
			}
		} else if status == "healthy" {
			payload := map[string]any{
				"event":         "provider.recovered",
				"provider_id":   p.ID,
				"provider_name": p.Name,
				"status":        "healthy",
				"latency_ms":    int(res.Latency.Milliseconds()),
				"timestamp":     time.Now().UTC().Format(time.RFC3339),
			}
			if _, err := h.enqueuer.Enqueue(ctx, "provider.recovered", payload); err != nil {
				h.logger.WarnContext(ctx, "gagal memasukkan event provider.recovered ke antrean webhook",
					"provider", p.Name, "error", err)
			}
		}
	}
}
