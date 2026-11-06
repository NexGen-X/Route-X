package admin

import (
	"fmt"
	"net/http"
	"strconv"
	"time"

	"github.com/go-chi/chi/v5"

	"github.com/NexGen-X/Route-X/internal/auth"
	"github.com/NexGen-X/Route-X/internal/database/repo"
	"github.com/NexGen-X/Route-X/internal/database/repo/traffic"
	"github.com/NexGen-X/Route-X/internal/database/repo/upstream"
	"github.com/NexGen-X/Route-X/internal/database/seed"
	"github.com/NexGen-X/Route-X/internal/httpx"
)

func (h *Handlers) observabilityRoutes(r chi.Router) {
	r.With(auth.RequirePermission(seed.PermUsageRead)).Get("/summary", h.getSummary)
	r.With(auth.RequirePermission(seed.PermUsageRead)).Get("/series", h.getSeries)
	r.With(auth.RequirePermission(seed.PermUsageRead)).Get("/breakdown", h.getBreakdown)
	r.With(auth.RequirePermission(seed.PermHealthRead)).Get("/health", h.getHealthSummary)
	r.With(auth.RequirePermission(seed.PermUsageRead)).Get("/metrics/live", h.getLiveMetrics)
}

// parseJendela mengurai nilai jendela relatif seperti "15m", "1h", "24h", "7d",
// "30d" menjadi durasi. Satuan yang dikenal: m (menit), h (jam), d (hari).
func parseJendela(s string) (time.Duration, error) {
	if len(s) < 2 {
		return 0, fmt.Errorf("jendela %q tidak dikenal", s)
	}
	n, err := strconv.Atoi(s[:len(s)-1])
	if err != nil || n <= 0 {
		return 0, fmt.Errorf("jendela %q tidak dikenal", s)
	}
	switch s[len(s)-1] {
	case 'm':
		return time.Duration(n) * time.Minute, nil
	case 'h':
		return time.Duration(n) * time.Hour, nil
	case 'd':
		return time.Duration(n) * 24 * time.Hour, nil
	}
	return 0, fmt.Errorf("jendela %q tidak dikenal", s)
}

// parseTimeRange mengekstrak rentang waktu dari query parameter.
//
// Tiga bentuk diterima, menurut urutan mendesak: from/to eksplisit (RFC3339)
// menang atas window, dan window menang atas time_range (alias warisan halaman
// Permintaan). Tanpa ketiganya, bawaannya 24 jam terakhir.
//
// Nilai yang cacat adalah ERROR, bukan alasan untuk diam-diam jatuh ke bawaan:
// versi lama menelan from/to yang tak terurai dan menjawab data 24 jam terakhir
// berlabel rentang yang diminta (audit3 V-FJ-01, V-EM-30), dan parameter window
// yang dikirim seluruh tombol rentang dashboard tidak dibaca sama sekali
// sehingga semua rentang menampilkan data yang sama.
func parseTimeRange(r *http.Request) (time.Time, time.Time, error) {
	now := time.Now().UTC()
	from := now.Add(-24 * time.Hour)
	to := now

	jendela := r.URL.Query().Get("window")
	if jendela == "" {
		jendela = r.URL.Query().Get("time_range")
	}
	if jendela != "" {
		d, err := parseJendela(jendela)
		if err != nil {
			return time.Time{}, time.Time{}, fmt.Errorf("%w: %v", repo.ErrInvalidParameter, err)
		}
		from = now.Add(-d)
	}
	if s := r.URL.Query().Get("from"); s != "" {
		t, err := time.Parse(time.RFC3339, s)
		if err != nil {
			return time.Time{}, time.Time{}, fmt.Errorf("%w: parameter from %q bukan waktu RFC3339", repo.ErrInvalidParameter, s)
		}
		from = t
	}
	if s := r.URL.Query().Get("to"); s != "" {
		t, err := time.Parse(time.RFC3339, s)
		if err != nil {
			return time.Time{}, time.Time{}, fmt.Errorf("%w: parameter to %q bukan waktu RFC3339", repo.ErrInvalidParameter, s)
		}
		to = t
	}
	if !from.Before(to) {
		return time.Time{}, time.Time{}, fmt.Errorf("%w: rentang waktu kosong (%s tidak sebelum %s)",
			repo.ErrInvalidParameter, from.UTC().Format(time.RFC3339), to.UTC().Format(time.RFC3339))
	}
	return from, to, nil
}

// rentangAtau400 menjalankan parseTimeRange dan menulis 400 bila tidak sah.
func rentangAtau400(w http.ResponseWriter, r *http.Request) (time.Time, time.Time, bool) {
	from, to, err := parseTimeRange(r)
	if err != nil {
		httpx.BadRequest(w, r, "invalid_parameter", "rentang waktu tidak sah: gunakan from/to RFC3339 atau window seperti 1h, 24h, 7d, 30d")
		return time.Time{}, time.Time{}, false
	}
	return from, to, true
}

// queryAgregat menyusun traffic.Query dari parameter bersama summary & series,
// memvalidasi source dan granularity di batas HTTP: nilai di luar himpunan yang
// dikenal adalah 400 di sini, bukan 500 dari cabang default repo (audit3 V-EM-31).
func queryAgregat(r *http.Request, from, to time.Time) (traffic.Query, error) {
	q := traffic.Query{
		From:       from,
		To:         to,
		APIKeyID:   r.URL.Query().Get("api_key_id"),
		ProviderID: r.URL.Query().Get("provider_id"),
		ModelID:    r.URL.Query().Get("model_id"),
	}
	switch src := traffic.Source(r.URL.Query().Get("source")); src {
	case traffic.SourceAuto, traffic.SourceRequests, traffic.SourceHourly, traffic.SourceDaily:
		q.Source = src
	default:
		return q, fmt.Errorf("%w: source %q tidak dikenal", repo.ErrInvalidParameter, src)
	}
	switch gran := traffic.Granularity(r.URL.Query().Get("granularity")); gran {
	case traffic.GranularityAuto, traffic.GranularityHour, traffic.GranularityDay:
		q.Granularity = gran
	default:
		return q, fmt.Errorf("%w: granularity %q tidak dikenal", repo.ErrInvalidParameter, gran)
	}
	return q, nil
}

func (h *Handlers) getSummary(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	from, to, ok := rentangAtau400(w, r)
	if !ok {
		return
	}

	q, err := queryAgregat(r, from, to)
	if err != nil {
		httpx.BadRequest(w, r, "invalid_parameter", "parameter agregasi tidak sah: source atau granularity tidak dikenal")
		return
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
	from, to, ok := rentangAtau400(w, r)
	if !ok {
		return
	}

	q, err := queryAgregat(r, from, to)
	if err != nil {
		httpx.BadRequest(w, r, "invalid_parameter", "parameter agregasi tidak sah: source atau granularity tidak dikenal")
		return
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

// idIrisanLainnya adalah ID sintetis baris agregat sisa breakdown. Bukan ID
// entitas nyata dan tidak akan pernah bertabrakan dengan UUID maupun nama.
const idIrisanLainnya = "__lainnya__"

// selisihTotals menghitung sisa total keseluruhan sesudah dikurangi irisan
// yang ditampilkan, untuk baris agregat "Lainnya". Hanya besaran aditif yang
// bisa diselisihkan dengan jujur: persentil latensi sisa TIDAK bisa diturunkan
// dari persentil per kelompok, jadi dibiarkan kosong (nil) alih-alih dikarang.
func selisihTotals(total, terpakai traffic.Totals) traffic.Totals {
	sisa := traffic.Totals{
		Requests:          total.Requests - terpakai.Requests,
		Success:           total.Success - terpakai.Success,
		Errors:            total.Errors - terpakai.Errors,
		Timeouts:          total.Timeouts - terpakai.Timeouts,
		Retries:           total.Retries - terpakai.Retries,
		Failovers:         total.Failovers - terpakai.Failovers,
		InputTokens:       total.InputTokens - terpakai.InputTokens,
		OutputTokens:      total.OutputTokens - terpakai.OutputTokens,
		CachedInputTokens: total.CachedInputTokens - terpakai.CachedInputTokens,
		ReasoningTokens:   total.ReasoningTokens - terpakai.ReasoningTokens,
		TotalTokens:       total.TotalTokens - terpakai.TotalTokens,
		CostUSD:           upstream.USD(total.CostUSD.Units() - terpakai.CostUSD.Units()),
		UnpricedRequests:  total.UnpricedRequests - terpakai.UnpricedRequests,
	}
	sisa.Latency.Count = total.Latency.Count - terpakai.Latency.Count
	sisa.Latency.SumMS = total.Latency.SumMS - terpakai.Latency.SumMS
	sisa.TTFT.Count = total.TTFT.Count - terpakai.TTFT.Count
	sisa.TTFT.SumMS = total.TTFT.SumMS - terpakai.TTFT.SumMS
	return sisa
}

func (h *Handlers) getBreakdown(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	from, to, ok := rentangAtau400(w, r)
	if !ok {
		return
	}

	// Nama kanonisnya dimension; by adalah alias yang dikirim klien web lama.
	// Nilai tak dikenal adalah 400 — versi lama jatuh diam-diam ke provider
	// sehingga tab model & api_key menampilkan data provider berlabel salah
	// (audit3 V-FJ-02).
	dimStr := r.URL.Query().Get("dimension")
	if dimStr == "" {
		dimStr = r.URL.Query().Get("by")
	}
	var dim traffic.Dimension
	switch dimStr {
	case "", "provider":
		dim = traffic.DimProvider
	case "model":
		dim = traffic.DimModel
	case "api_key":
		dim = traffic.DimAPIKey
	default:
		httpx.BadRequest(w, r, "invalid_parameter", "dimension tidak dikenal: gunakan provider, model, atau api_key")
		return
	}

	limit := 10
	if s := r.URL.Query().Get("limit"); s != "" {
		n, err := strconv.Atoi(s)
		if err != nil || n <= 0 || n > 100 {
			httpx.BadRequest(w, r, "invalid_parameter", "limit harus bilangan 1 sampai 100")
			return
		}
		limit = n
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

	// Persentase dihitung dari TOTAL KESELURUHAN rentang, bukan subtotal
	// irisan yang kebetulan dikembalikan: versi lama membuat kelompok ke-11
	// dan seterusnya hilang tanpa jejak sementara persen selalu berjumlah
	// rapi 100% (audit3 V-FJ-04). Total diambil dari Summary atas query yang
	// sama persis, dan sisanya dilaporkan sebagai satu baris "Lainnya".
	stats, err := h.trafficRepo.Summary(ctx, q)
	if err != nil {
		mapRepoError(w, r, err, "total pemecahan dimensi lalu lintas")
		return
	}

	_ = h.respond(w, r, http.StatusOK, TrafficBreakdownResponse{
		Dimension:     string(dim),
		Items:         susunIrisanBreakdown(slices, stats.Totals),
		TotalRequests: stats.Requests,
	})
}

// susunIrisanBreakdown menyusun DTO breakdown dari irisan top-N dan total
// keseluruhan rentang: Percentage setiap irisan berbasis total itu, dan
// selisih yang tidak terwakili irisan mana pun menjadi baris "Lainnya".
func susunIrisanBreakdown(slices []traffic.Slice, total traffic.Totals) []TrafficSliceDTO {
	totalReqs := total.Requests
	var terpakai traffic.Totals
	items := make([]TrafficSliceDTO, 0, len(slices)+1)
	for _, s := range slices {
		items = append(items, toTrafficSliceDTO(s, totalReqs))
		terpakai.Requests += s.Requests
		terpakai.Success += s.Success
		terpakai.Errors += s.Errors
		terpakai.Timeouts += s.Timeouts
		terpakai.Retries += s.Retries
		terpakai.Failovers += s.Failovers
		terpakai.InputTokens += s.InputTokens
		terpakai.OutputTokens += s.OutputTokens
		terpakai.CachedInputTokens += s.CachedInputTokens
		terpakai.ReasoningTokens += s.ReasoningTokens
		terpakai.TotalTokens += s.TotalTokens
		terpakai.CostUSD = upstream.USD(terpakai.CostUSD.Units() + s.CostUSD.Units())
		terpakai.UnpricedRequests += s.UnpricedRequests
		terpakai.Latency.Count += s.Latency.Count
		terpakai.Latency.SumMS += s.Latency.SumMS
		terpakai.TTFT.Count += s.TTFT.Count
		terpakai.TTFT.SumMS += s.TTFT.SumMS
	}
	if sisa := selisihTotals(total, terpakai); sisa.Requests > 0 {
		items = append(items, toTrafficSliceDTO(traffic.Slice{
			Totals: sisa,
			ID:     idIrisanLainnya,
			Name:   "Lainnya",
		}, totalReqs))
	}
	return items
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
	// Penjaga nil (audit F5): samakan dengan getDiagnostics/getSystemOverview.
	if h.pool == nil {
		httpx.InternalError(w, r)
		return
	}
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
