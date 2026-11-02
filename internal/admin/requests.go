package admin

import (
	"net/http"
	"strconv"
	"strings"
	"time"

	"github.com/go-chi/chi/v5"

	"github.com/NexGen-X/Route-X/internal/auth"
	"github.com/NexGen-X/Route-X/internal/database/repo"
	"github.com/NexGen-X/Route-X/internal/database/repo/traffic"
	"github.com/NexGen-X/Route-X/internal/database/seed"
	"github.com/NexGen-X/Route-X/internal/httpx"
)

func (h *Handlers) requestsRoutes(r chi.Router) {
	r.With(auth.RequirePermission(seed.PermRequestsRead)).Get("/", h.listRequests)
	r.With(auth.RequirePermission(seed.PermRequestsRead)).Get("/{id}", h.getRequest)
	r.With(auth.RequirePermission(seed.PermRequestsRead)).Get("/{id}/events", h.getRequestEvents)
	r.With(auth.RequirePermission(seed.PermRequestsRead)).Get("/{id}/payload", h.getRequestPayload)
}

func (h *Handlers) listRequests(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	from, to, ok := rentangAtau400(w, r)
	if !ok {
		return
	}

	limit := repo.DefaultPageLimit
	if s := r.URL.Query().Get("limit"); s != "" {
		if n, err := strconv.Atoi(s); err == nil && n > 0 {
			limit = n
		}
	}

	page := repo.Page{
		Limit:  limit,
		Cursor: r.URL.Query().Get("cursor"),
	}

	filter := traffic.Filter{
		From:       from,
		To:         to,
		ProviderID: r.URL.Query().Get("provider_id"),
		ModelID:    r.URL.Query().Get("model_id"),
		APIKeyID:   r.URL.Query().Get("api_key_id"),
		Status:     r.URL.Query().Get("status"),
		OnlyErrors: r.URL.Query().Get("only_errors") == "true",
		Search:     r.URL.Query().Get("search"),
		Sort:       r.URL.Query().Get("sort"),
		Ascending:  r.URL.Query().Get("order") == "asc",
	}

	// Alias yang dikirim halaman Permintaan (audit3 V-FJ-03): status_class dan
	// status_code diterima di sini dan diterjemahkan ke filter kanonis.
	// Versi lama tidak membaca keduanya sama sekali sehingga tab Sukses/Error
	// dan filter kode status tidak berpengaruh apa pun. Kelas "error" berarti
	// definisi gagal repo (status >= 400 ATAU error_type terisi) — himpunan
	// yang sama dengan chip Errors, termasuk stream 200 yang terputus.
	switch kelas := strings.ToLower(r.URL.Query().Get("status_class")); kelas {
	case "":
	case "error", "errors":
		filter.OnlyErrors = true
	case "success":
		filter.Status = "2xx"
	case "2xx", "3xx", "4xx", "5xx":
		filter.Status = kelas
	default:
		httpx.BadRequest(w, r, "invalid_parameter", "status_class tidak dikenal: gunakan success, error, atau kelas seperti 2xx/4xx/5xx")
		return
	}
	if s := r.URL.Query().Get("status_code"); s != "" {
		n, err := strconv.Atoi(s)
		if err != nil || n < 100 || n > 599 {
			httpx.BadRequest(w, r, "invalid_parameter", "status_code harus kode HTTP 100 sampai 599")
			return
		}
		// Kode persis lebih spesifik daripada kelas: ia menang.
		filter.Status = s
		filter.OnlyErrors = false
	}

	res, err := h.trafficRepo.List(ctx, filter, page)
	if err != nil {
		mapRepoError(w, r, err, "log request")
		return
	}

	rows := make([]TrafficRowDTO, 0, len(res.Rows))
	for _, row := range res.Rows {
		rows = append(rows, toTrafficRowDTO(row))
	}

	_ = h.respond(w, r, http.StatusOK, ListEnvelope[TrafficRowDTO]{
		Items:      rows,
		NextCursor: res.NextCursor,
	})
}

func (h *Handlers) getRequest(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	id := chi.URLParam(r, "id")

	var createdAt time.Time
	if s := r.URL.Query().Get("created_at"); s != "" {
		if t, err := time.Parse(time.RFC3339Nano, s); err == nil {
			createdAt = t
		} else if t, err := time.Parse(time.RFC3339, s); err == nil {
			createdAt = t
		}
	}

	detail, err := h.trafficRepo.Get(ctx, id, createdAt)
	if err != nil {
		mapRepoError(w, r, err, "detail request")
		return
	}

	_ = h.respond(w, r, http.StatusOK, toTrafficDetailDTO(&detail))
}

func (h *Handlers) getRequestEvents(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	id := chi.URLParam(r, "id")

	var createdAt time.Time
	if s := r.URL.Query().Get("created_at"); s != "" {
		if t, err := time.Parse(time.RFC3339Nano, s); err == nil {
			createdAt = t
		} else if t, err := time.Parse(time.RFC3339, s); err == nil {
			createdAt = t
		}
	}

	events, err := h.trafficRepo.Events(ctx, id, createdAt)
	if err != nil {
		mapRepoError(w, r, err, "timeline request")
		return
	}

	evs := make([]TrafficEventDTO, 0, len(events))
	for _, e := range events {
		evs = append(evs, toTrafficEventDTO(e))
	}

	_ = h.respond(w, r, http.StatusOK, TrafficEventsResponse{
		Items:  evs,
		Events: evs,
	})
}

func (h *Handlers) getRequestPayload(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	id := chi.URLParam(r, "id")

	var createdAt time.Time
	if s := r.URL.Query().Get("created_at"); s != "" {
		if t, err := time.Parse(time.RFC3339Nano, s); err == nil {
			createdAt = t
		} else if t, err := time.Parse(time.RFC3339, s); err == nil {
			createdAt = t
		}
	}

	payload, err := h.trafficRepo.Payload(ctx, id, createdAt)
	if err != nil {
		mapRepoError(w, r, err, "payload request")
		return
	}

	_ = h.respond(w, r, http.StatusOK, toTrafficPayloadDTO(&payload))
}
