package admin

import (
	"net/http"
	"strconv"
	"time"

	"github.com/go-chi/chi/v5"

	"github.com/NexGen-X/Route-X/internal/auth"
	"github.com/NexGen-X/Route-X/internal/database/repo/traffic"
	"github.com/NexGen-X/Route-X/internal/database/seed"
)

func (h *Handlers) observabilityRoutes(r chi.Router) {
	r.With(auth.RequirePermission(seed.PermUsageRead)).Get("/summary", h.getSummary)
	r.With(auth.RequirePermission(seed.PermUsageRead)).Get("/series", h.getSeries)
	r.With(auth.RequirePermission(seed.PermUsageRead)).Get("/breakdown", h.getBreakdown)
	r.With(auth.RequirePermission(seed.PermHealthRead)).Get("/health", h.getHealthSummary)
	r.With(auth.RequirePermission(seed.PermUsageRead)).Get("/metrics/live", h.getLiveMetrics)
}

// parseTimeRange mengekstrak query parameter from & to (RFC3339). Jika kosong, default 24 jam terakhir.
func parseTimeRange(r *http.Request) (time.Time, time.Time) {
	now := time.Now().UTC()
	from := now.Add(-24 * time.Hour)
	to := now

	if s := r.URL.Query().Get("from"); s != "" {
		if t, err := time.Parse(time.RFC3339, s); err == nil {
			from = t
		}
	}
	if s := r.URL.Query().Get("to"); s != "" {
		if t, err := time.Parse(time.RFC3339, s); err == nil {
			to = t
		}
	}
	return from, to
}

func (h *Handlers) getSummary(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	from, to := parseTimeRange(r)

	q := traffic.Query{
		From:        from,
		To:          to,
		APIKeyID:    r.URL.Query().Get("api_key_id"),
		ProviderID:  r.URL.Query().Get("provider_id"),
		ModelID:     r.URL.Query().Get("model_id"),
		Source:      traffic.Source(r.URL.Query().Get("source")),
		Granularity: traffic.Granularity(r.URL.Query().Get("granularity")),
	}

	stats, err := h.trafficRepo.Summary(ctx, q)
	if err != nil {
		mapRepoError(w, r, err, "agregasi statistik")
		return
	}

	_ = h.respond(w, r, http.StatusOK, toTrafficStatsDTO(&stats))
}

func (h *Handlers) getSeries(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	from, to := parseTimeRange(r)

	q := traffic.Query{
		From:        from,
		To:          to,
		APIKeyID:    r.URL.Query().Get("api_key_id"),
		ProviderID:  r.URL.Query().Get("provider_id"),
		ModelID:     r.URL.Query().Get("model_id"),
		Source:      traffic.Source(r.URL.Query().Get("source")),
		Granularity: traffic.Granularity(r.URL.Query().Get("granularity")),
	}

	points, err := h.trafficRepo.Series(ctx, q)
	if err != nil {
		mapRepoError(w, r, err, "deret waktu lalu lintas")
		return
	}

	pts := make([]TrafficPointDTO, 0, len(points))
	for _, p := range points {
		pts = append(pts, toTrafficPointDTO(p))
	}

	_ = h.respond(w, r, http.StatusOK, TrafficSeriesResponse{
		Items:  pts,
		Points: pts,
	})
}

func (h *Handlers) getBreakdown(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	from, to := parseTimeRange(r)

	dimStr := r.URL.Query().Get("dimension")
	var dim traffic.Dimension
	switch dimStr {
	case "provider":
		dim = traffic.DimProvider
	case "model":
		dim = traffic.DimModel
	case "api_key":
		dim = traffic.DimAPIKey
	default:
		dim = traffic.DimProvider
	}

	limit := 10
	if s := r.URL.Query().Get("limit"); s != "" {
		if n, err := strconv.Atoi(s); err == nil && n > 0 && n <= 100 {
			limit = n
		}
	}

	q := traffic.Query{
		From:       from,
		To:         to,
		APIKeyID:   r.URL.Query().Get("api_key_id"),
		ProviderID: r.URL.Query().Get("provider_id"),
		ModelID:    r.URL.Query().Get("model_id"),
	}

	slices, err := h.trafficRepo.Breakdown(ctx, q, dim, limit)
	if err != nil {
		mapRepoError(w, r, err, "pemecahan dimensi lalu lintas")
		return
	}

	var totalReqs int64
	for _, s := range slices {
		totalReqs += s.Requests
	}

	items := make([]TrafficSliceDTO, 0, len(slices))
	for _, s := range slices {
		items = append(items, toTrafficSliceDTO(s, totalReqs))
	}

	_ = h.respond(w, r, http.StatusOK, TrafficBreakdownResponse{
		Dimension: string(dim),
		Items:     items,
	})
}

func (h *Handlers) getHealthSummary(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()

	providers, err := h.providerRepo.ActiveProviders(ctx)
	if err != nil {
		mapRepoError(w, r, err, "ketersediaan provider")
		return
	}

	statuses := make([]ProviderHealthDTO, 0, len(providers))
	for _, p := range providers {
		status := "unknown"
		if p.LastHealthStatus != nil {
			status = *p.LastHealthStatus
		}
		var checkedAt *string
		if p.LastHealthAt != nil {
			s := p.LastHealthAt.UTC().Format(time.RFC3339)
			checkedAt = &s
		}
		statuses = append(statuses, ProviderHealthDTO{
			ID:            p.ID,
			Name:          p.Name,
			Status:        status,
			LastCheckedAt: checkedAt,
		})
	}

	_ = h.respond(w, r, http.StatusOK, HealthSummaryResponse{
		Providers: statuses,
	})
}

func (h *Handlers) getLiveMetrics(w http.ResponseWriter, r *http.Request) {
	stat := h.pool.Stat()

	resp := LiveMetricsResponse{
		DBPool: DBPoolStatsDTO{
			TotalConns:       stat.TotalConns(),
			IdleConns:        stat.IdleConns(),
			AcquiredConns:    stat.AcquiredConns(),
			MaxConns:         stat.MaxConns(),
			AcquireCount:     stat.AcquireCount(),
			EmptyAcquireWait: stat.EmptyAcquireCount(),
		},
	}

	if h.redis != nil {
		ctx := r.Context()
		pong, err := h.redis.Ping(ctx).Result()
		redisStatus := "ok"
		if err != nil {
			redisStatus = "error"
		}
		resp.Redis = &RedisStatsDTO{
			Status: redisStatus,
			Ping:   pong,
		}
	}

	_ = h.respond(w, r, http.StatusOK, resp)
}
