package admin

import (
	"fmt"
	"net/http"
	"net/url"
	"strconv"
	"strings"

	"github.com/go-chi/chi/v5"

	"github.com/NexGen-X/Route-X/internal/auth"
	"github.com/NexGen-X/Route-X/internal/database/repo"
	"github.com/NexGen-X/Route-X/internal/database/seed"
	"github.com/NexGen-X/Route-X/internal/httpx"
	"github.com/NexGen-X/Route-X/internal/security"
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
	Enabled    *bool    `json:"enabled"`
	MaxRetries *int     `json:"max_retries"`
	TimeoutMS  *int     `json:"timeout_ms"`
}

// validasiFieldWebhook memeriksa satu field masukan webhook dan mengembalikan
// (kode, pesan) siap pakai untuk 400. Validasi tinggal di handler — bukan
// hanya di CHECK constraint DB — supaya pesan menyebut field dan rentang yang
// sah (audit3 V-WH-03: sebelumnya operator hanya menerima constraint_violation
// generik, dan events kosong justru 500).
func validasiEventsWebhook(events []string) (string, string, bool) {
	if len(events) == 0 {
		return "invalid_events", "events wajib berisi minimal satu event yang dilanggani", false
	}
	for _, ev := range events {
		if !webhooks.ValidEvent(ev) {
			return "invalid_events", fmt.Sprintf("event %q tidak dikenal; pilihan yang sah: %s", ev, strings.Join(webhooks.KnownEvents, ", ")), false
		}
	}
	return "", "", true
}

func (h *Handlers) createWebhook(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	var req createWebhookReq
	if err := decodeJSON(r, &req); err != nil {
		httpx.BadRequest(w, r, "invalid_json", err.Error())
		return
	}
	if strings.TrimSpace(req.Name) == "" {
		httpx.BadRequest(w, r, "invalid_name", "nama webhook wajib diisi")
		return
	}
	if strings.TrimSpace(req.URL) == "" {
		httpx.BadRequest(w, r, "missing_url", "url wajib diisi")
		return
	}
	if code, msg, ok := validasiEventsWebhook(req.Events); !ok {
		httpx.BadRequest(w, r, code, msg)
		return
	}
	if strings.TrimSpace(req.Secret) == "" {
		// Secret kosong berarti tanda tangan HMAC berkunci kosong yang bisa
		// dipalsukan siapa pun yang tahu URL penerima (audit3 V-WH-04).
		httpx.BadRequest(w, r, "missing_secret", "secret wajib diisi: payload webhook selalu ditandatangani dengan secret ini")
		return
	}
	if req.MaxRetries != nil && (*req.MaxRetries < 0 || *req.MaxRetries > 20) {
		httpx.BadRequest(w, r, "invalid_max_retries", "max_retries harus antara 0 dan 20")
		return
	}
	if req.TimeoutMS != nil && (*req.TimeoutMS < 1000 || *req.TimeoutMS > 120000) {
		httpx.BadRequest(w, r, "invalid_timeout_ms", "timeout_ms harus antara 1000 dan 120000")
		return
	}

	var actorID string
	if p, ok := auth.PrincipalFrom(ctx); ok && p != nil {
		actorID = p.User.ID
	}

	// Nilai eksplisit dihormati apa adanya — termasuk max_retries 0 ("sekali
	// kirim saja") yang versi lama telan menjadi bawaan 3, dan enabled false
	// yang versi lama abaikan sehingga webhook lahir aktif (audit3 V-WH-02).
	maxRetries := 3
	if req.MaxRetries != nil {
		maxRetries = *req.MaxRetries
	}
	timeoutMS := 10000
	if req.TimeoutMS != nil {
		timeoutMS = *req.TimeoutMS
	}
	enabled := true
	if req.Enabled != nil {
		enabled = *req.Enabled
	}

	wh, err := h.webhooksRepo.Create(ctx, webhooks.CreateWebhookParams{
		Name:       strings.TrimSpace(req.Name),
		URL:        req.URL,
		Events:     req.Events,
		RawSecret:  req.Secret,
		Cipher:     h.cipher,
		Enabled:    enabled,
		MaxRetries: maxRetries,
		TimeoutMS:  timeoutMS,
		CreatedBy:  actorID,
		SSRFPolicy: h.ssrfPolicy(),
	})
	if err != nil {
		mapRepoError(w, r, err, "webhook")
		return
	}

	h.writeAudit(ctx, r, "create", "webhook", wh.ID, map[string]any{"name": wh.Name, "url": sanitizeAuditURL(wh.URL)})
	_ = h.respond(w, r, http.StatusCreated, toWebhookDTO(wh))
}

func sanitizeAuditURL(raw string) string {
	u, err := url.Parse(raw)
	if err != nil {
		return "[URL TIDAK SAH]"
	}
	u.User = nil
	u.RawQuery = ""
	u.Fragment = ""
	return u.String()
}

func (h *Handlers) ssrfPolicy() security.SSRFPolicy {
	if h.factory != nil {
		return h.factory.SSRFPolicy()
	}
	return security.DefaultSSRFPolicy()
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

	// Field yang dikirim divalidasi di sini juga (pola createWebhook):
	// pesan 400 menyebut field dan rentang yang sah, bukan CHECK DB yang buram.
	if req.Name != nil && strings.TrimSpace(*req.Name) == "" {
		httpx.BadRequest(w, r, "invalid_name", "nama webhook tidak boleh kosong")
		return
	}
	if req.Events != nil {
		if code, msg, ok := validasiEventsWebhook(req.Events); !ok {
			httpx.BadRequest(w, r, code, msg)
			return
		}
	}
	if req.MaxRetries != nil && (*req.MaxRetries < 0 || *req.MaxRetries > 20) {
		httpx.BadRequest(w, r, "invalid_max_retries", "max_retries harus antara 0 dan 20")
		return
	}
	if req.TimeoutMS != nil && (*req.TimeoutMS < 1000 || *req.TimeoutMS > 120000) {
		httpx.BadRequest(w, r, "invalid_timeout_ms", "timeout_ms harus antara 1000 dan 120000")
		return
	}

	wh, err := h.webhooksRepo.Update(ctx, id, webhooks.UpdateWebhookParams{
		Name:       req.Name,
		URL:        req.URL,
		Events:     req.Events,
		Enabled:    req.Enabled,
		MaxRetries: req.MaxRetries,
		TimeoutMS:  req.TimeoutMS,
		SSRFPolicy: h.ssrfPolicy(),
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

	enabled, err := decodeToggleEnabled(r)
	if err != nil {
		tolakToggle(w, r, err)
		return
	}

	wh, err := h.webhooksRepo.SetEnabled(ctx, id, enabled)
	if err != nil {
		mapRepoError(w, r, err, "webhook")
		return
	}

	h.writeAudit(ctx, r, "toggle", "webhook", id, map[string]any{"enabled": enabled})
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

	// Webhook nonaktif tidak boleh menerima trafik apa pun dari gateway,
	// termasuk ping uji (B-109): jalur event otomatis sudah menghormati
	// enabled:false, jadi jalur uji manual tidak boleh menjadi celahnya.
	// Ditolak 400 sebelum dispatcher disentuh — tidak ada yang terkirim.
	if !wh.Enabled {
		httpx.BadRequest(w, r, "webhook_disabled", "webhook nonaktif; aktifkan dulu untuk mengirim ping uji")
		return
	}

	if h.dispatcher == nil {
		tidakTersedia(w, r, "dispatcher_unavailable", "dispatcher webhook tidak aktif")
		return
	}

	// Kirim langsung melalui dispatcher tanpa menyentuh antrean database.
	// Ini mencegah tes ping menyebar ke pelanggan event wildcard (*) dan mematuhi format event berdomain.
	statusCode, duration, pingErr := h.dispatcher.Ping(ctx, wh)
	h.writeAudit(ctx, r, "test_ping", "webhook", id, map[string]any{
		"status_code": statusCode,
		"duration_ms": duration.Milliseconds(),
		"success":     pingErr == nil && statusCode >= 200 && statusCode < 300,
	})

	if pingErr != nil {
		_ = h.respond(w, r, http.StatusOK, WebhookPingResponse{
			Status:     "failed",
			StatusCode: statusCode,
			DurationMS: duration.Milliseconds(),
			Error:      pingErr.Error(),
		})
		return
	}

	statusStr := "ok"
	if statusCode >= 400 {
		statusStr = fmt.Sprintf("HTTP %d", statusCode)
	}
	_ = h.respond(w, r, http.StatusOK, WebhookPingResponse{
		Status:     statusStr,
		StatusCode: statusCode,
		DurationMS: duration.Milliseconds(),
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
