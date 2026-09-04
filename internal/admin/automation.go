package admin

import (
	"net/http"
	"strconv"
	"time"

	"github.com/go-chi/chi/v5"

	"github.com/NexGen-X/Route-X/internal/auth"
	"github.com/NexGen-X/Route-X/internal/database/repo"
	"github.com/NexGen-X/Route-X/internal/database/seed"
	"github.com/NexGen-X/Route-X/internal/httpx"
	"github.com/NexGen-X/Route-X/internal/webhooks"
)

func (h *Handlers) automationRoutes(r chi.Router) {
	// Webhooks
	r.Route("/webhooks", func(wr chi.Router) {
		wr.With(auth.RequirePermission(seed.PermSettingsRead)).Get("/", h.listWebhooks)
		wr.With(auth.RequirePermission(seed.PermWebhooksWrite)).Post("/", h.createWebhook)
		wr.With(auth.RequirePermission(seed.PermSettingsRead)).Get("/{id}", h.getWebhook)
		wr.With(auth.RequirePermission(seed.PermWebhooksWrite)).Put("/{id}", h.updateWebhook)
		wr.With(auth.RequirePermission(seed.PermWebhooksWrite)).Delete("/{id}", h.deleteWebhook)
		wr.With(auth.RequirePermission(seed.PermWebhooksWrite)).Post("/{id}/toggle", h.toggleWebhook)
		wr.With(auth.RequirePermission(seed.PermWebhooksWrite)).Post("/{id}/test", h.testWebhookPing)

		// Riwayat pengiriman webhook
		wr.With(auth.RequirePermission(seed.PermSettingsRead)).Get("/{id}/deliveries", h.listWebhookDeliveries)
	})

	r.With(auth.RequirePermission(seed.PermSettingsRead)).Get("/deliveries/{delivery_id}", h.getDelivery)
}

func (h *Handlers) listWebhooks(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	items, err := h.webhooksRepo.List(ctx)
	if err != nil {
		mapRepoError(w, r, err, "webhook")
		return
	}

	whs := make([]WebhookDTO, 0, len(items))
	for _, wh := range items {
		whs = append(whs, toWebhookDTO(wh))
	}

	_ = h.respond(w, r, http.StatusOK, ListEnvelope[WebhookDTO]{
		Items: whs,
	})
}

func (h *Handlers) getWebhook(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	id := chi.URLParam(r, "id")

	wh, err := h.webhooksRepo.Get(ctx, id)
	if err != nil {
		mapRepoError(w, r, err, "webhook")
		return
	}

	_ = h.respond(w, r, http.StatusOK, toWebhookDTO(wh))
}

type createWebhookReq struct {
	Name       string   `json:"name"`
	URL        string   `json:"url"`
	Events     []string `json:"events"`
	Secret     string   `json:"secret"`
	Enabled    bool     `json:"enabled"`
	MaxRetries *int     `json:"max_retries"`
	TimeoutMS  *int     `json:"timeout_ms"`
}

func (h *Handlers) createWebhook(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	var req createWebhookReq
	if err := decodeJSON(r, &req); err != nil {
		httpx.BadRequest(w, r, "invalid_json", err.Error())
		return
	}
	if req.URL == "" {
		httpx.BadRequest(w, r, "missing_url", "url wajib diisi")
		return
	}

	var actorID string
	if p, ok := auth.PrincipalFrom(ctx); ok && p != nil {
		actorID = p.User.ID
	}

	maxRetries := 3
	if req.MaxRetries != nil && *req.MaxRetries > 0 {
		maxRetries = *req.MaxRetries
	}
	timeoutMS := 10000
	if req.TimeoutMS != nil && *req.TimeoutMS > 0 {
		timeoutMS = *req.TimeoutMS
	}

	wh, err := h.webhooksRepo.Create(ctx, webhooks.CreateWebhookParams{
		Name:       req.Name,
		URL:        req.URL,
		Events:     req.Events,
		RawSecret:  req.Secret,
		Cipher:     h.cipher,
		MaxRetries: maxRetries,
		TimeoutMS:  timeoutMS,
		CreatedBy:  actorID,
	})
	if err != nil {
		mapRepoError(w, r, err, "webhook")
		return
	}

	h.writeAudit(ctx, r, "create", "webhook", wh.ID, map[string]any{"name": wh.Name, "url": wh.URL})
	_ = h.respond(w, r, http.StatusCreated, toWebhookDTO(wh))
}

func (h *Handlers) updateWebhook(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	id := chi.URLParam(r, "id")

	var req struct {
		Name       *string  `json:"name"`
		URL        *string  `json:"url"`
		Events     []string `json:"events"`
		Enabled    *bool    `json:"enabled"`
		MaxRetries *int     `json:"max_retries"`
		TimeoutMS  *int     `json:"timeout_ms"`
	}
	if err := decodeJSON(r, &req); err != nil {
		httpx.BadRequest(w, r, "invalid_json", err.Error())
		return
	}

	wh, err := h.webhooksRepo.Update(ctx, id, webhooks.UpdateWebhookParams{
		Name:       req.Name,
		URL:        req.URL,
		Events:     req.Events,
		Enabled:    req.Enabled,
		MaxRetries: req.MaxRetries,
		TimeoutMS:  req.TimeoutMS,
	})
	if err != nil {
		mapRepoError(w, r, err, "webhook")
		return
	}

	h.writeAudit(ctx, r, "update", "webhook", id, map[string]any{"name": wh.Name})
	_ = h.respond(w, r, http.StatusOK, toWebhookDTO(wh))
}

func (h *Handlers) deleteWebhook(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	id := chi.URLParam(r, "id")

	if err := h.webhooksRepo.Delete(ctx, id); err != nil {
		mapRepoError(w, r, err, "webhook")
		return
	}

	h.writeAudit(ctx, r, "delete", "webhook", id, nil)
	_ = h.respond(w, r, http.StatusOK, StatusResponse{Status: "deleted"})
}

func (h *Handlers) toggleWebhook(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	id := chi.URLParam(r, "id")

	var req struct {
		Enabled bool `json:"enabled"`
	}
	if err := decodeJSON(r, &req); err != nil {
		httpx.BadRequest(w, r, "invalid_json", err.Error())
		return
	}

	wh, err := h.webhooksRepo.SetEnabled(ctx, id, req.Enabled)
	if err != nil {
		mapRepoError(w, r, err, "webhook")
		return
	}

	h.writeAudit(ctx, r, "toggle", "webhook", id, map[string]any{"enabled": req.Enabled})
	_ = h.respond(w, r, http.StatusOK, toWebhookDTO(wh))
}

func (h *Handlers) testWebhookPing(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	id := chi.URLParam(r, "id")

	wh, err := h.webhooksRepo.Get(ctx, id)
	if err != nil {
		mapRepoError(w, r, err, "webhook")
		return
	}

	// Buat payload uji ping
	pingPayload := map[string]any{
		"event":      "ping",
		"webhook_id": wh.ID,
		"timestamp":  time.Now().UTC().Format(time.RFC3339),
		"message":    "Uji koneksi webhook dari Route-X Admin",
	}

	n, err := h.webhooksRepo.Enqueue(ctx, "ping", pingPayload)
	if err != nil {
		mapRepoError(w, r, err, "antrean webhook test")
		return
	}

	h.writeAudit(ctx, r, "test_ping", "webhook", id, nil)
	_ = h.respond(w, r, http.StatusOK, WebhookPingResponse{
		Status:   "enqueued",
		Enqueued: n,
	})
}

func (h *Handlers) listWebhookDeliveries(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	id := chi.URLParam(r, "id")

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

	items, next, err := h.webhooksRepo.ListDeliveries(ctx, id, page)
	if err != nil {
		mapRepoError(w, r, err, "delivery webhook")
		return
	}

	deliveries := make([]DeliveryDTO, 0, len(items))
	for _, d := range items {
		deliveries = append(deliveries, toDeliveryDTO(d))
	}

	_ = h.respond(w, r, http.StatusOK, ListEnvelope[DeliveryDTO]{
		Items:      deliveries,
		NextCursor: next,
	})
}

func (h *Handlers) getDelivery(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	idStr := chi.URLParam(r, "delivery_id")

	id, err := strconv.ParseInt(idStr, 10, 64)
	if err != nil {
		httpx.BadRequest(w, r, "invalid_id", "delivery_id harus bilangan bulat")
		return
	}

	d, err := h.webhooksRepo.GetDelivery(ctx, id)
	if err != nil {
		mapRepoError(w, r, err, "delivery webhook")
		return
	}

	_ = h.respond(w, r, http.StatusOK, toDeliveryDTO(d))
}
