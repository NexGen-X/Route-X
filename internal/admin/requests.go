package admin

import (
	"net/http"
	"strconv"
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
	from, to := parseTimeRange(r)

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

	res, err := h.trafficRepo.List(ctx, filter, page)
	if err != nil {
		mapRepoError(w, r, err, "log request")
		return
	}

	_ = httpx.JSON(w, http.StatusOK, map[string]any{
		"items":       res.Rows,
		"next_cursor": res.NextCursor,
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

	_ = httpx.JSON(w, http.StatusOK, detail)
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

	_ = httpx.JSON(w, http.StatusOK, map[string]any{
		"items": events,
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

	_ = httpx.JSON(w, http.StatusOK, payload)
}
