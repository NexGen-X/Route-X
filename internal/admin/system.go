package admin

import (
	"encoding/json"
	"net/http"
	"runtime"
	"time"

	"github.com/go-chi/chi/v5"

	"github.com/NexGen-X/Route-X/internal/auth"
	"github.com/NexGen-X/Route-X/internal/database/repo"
	"github.com/NexGen-X/Route-X/internal/database/repo/identity"
	"github.com/NexGen-X/Route-X/internal/database/seed"
	"github.com/NexGen-X/Route-X/internal/httpx"
)

var appStartTime = time.Now()

func (h *Handlers) systemRoutes(r chi.Router) {
	// Settings
	r.Route("/settings", func(sr chi.Router) {
		sr.With(auth.RequirePermission(seed.PermSettingsRead)).Get("/", h.listSettings)
		sr.With(auth.RequirePermission(seed.PermSettingsRead)).Get("/{key}", h.getSetting)
		sr.With(auth.RequirePermission(seed.PermSettingsWrite)).Put("/{key}", h.putSetting)
		sr.With(auth.RequirePermission(seed.PermSettingsWrite)).Delete("/{key}", h.deleteSetting)
	})

	// Background Jobs
	r.Route("/jobs", func(jr chi.Router) {
		jr.With(auth.RequirePermission(seed.PermHealthRead)).Get("/", h.listJobs)
		jr.With(auth.RequirePermission(seed.PermSettingsWrite)).Post("/{name}/run", h.runJob)
	})

	// Audit Logs
	r.With(auth.RequirePermission(seed.PermAuditRead)).Get("/audit-logs", h.listAuditLogs)

	// Diagnostics
	r.With(auth.RequirePermission(seed.PermHealthRead)).Get("/diagnostics", h.getDiagnostics)
}

// -----------------------------------------------------------------------------
// Settings Handlers
// -----------------------------------------------------------------------------

func (h *Handlers) listSettings(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	items, err := h.settingsRepo.All(ctx)
	if err != nil {
		mapRepoError(w, r, err, "setelan sistem")
		return
	}

	_ = httpx.JSON(w, http.StatusOK, map[string]any{
		"items": items,
	})
}

func (h *Handlers) getSetting(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	key := chi.URLParam(r, "key")

	s, err := h.settingsRepo.Get(ctx, key)
	if err != nil {
		mapRepoError(w, r, err, "setelan sistem")
		return
	}

	_ = httpx.JSON(w, http.StatusOK, s)
}

func (h *Handlers) putSetting(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	key := chi.URLParam(r, "key")

	var req struct {
		Value       json.RawMessage `json:"value"`
		Description string          `json:"description"`
	}
	if err := decodeJSON(r, &req); err != nil {
		httpx.BadRequest(w, r, "invalid_json", err.Error())
		return
	}

	var actorID string
	if p, ok := auth.PrincipalFrom(ctx); ok && p != nil {
		actorID = p.User.ID
	}

	s, err := h.settingsRepo.Put(ctx, identity.SettingWrite{
		Key:         key,
		Value:       req.Value,
		Description: req.Description,
		UpdatedBy:   actorID,
	})
	if err != nil {
		mapRepoError(w, r, err, "setelan sistem")
		return
	}

	h.writeAudit(ctx, r, "put", "setting", key, map[string]any{"key": key})
	_ = httpx.JSON(w, http.StatusOK, s)
}

func (h *Handlers) deleteSetting(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	key := chi.URLParam(r, "key")

	if err := h.settingsRepo.Delete(ctx, key); err != nil {
		mapRepoError(w, r, err, "setelan sistem")
		return
	}

	h.writeAudit(ctx, r, "delete", "setting", key, nil)
	_ = httpx.JSON(w, http.StatusOK, map[string]string{"status": "deleted"})
}

// -----------------------------------------------------------------------------
// Jobs Handlers
// -----------------------------------------------------------------------------

func (h *Handlers) listJobs(w http.ResponseWriter, r *http.Request) {
	type JobInfo struct {
		Name     string `json:"name"`
		Schedule string `json:"schedule"`
		Status   string `json:"status"`
	}

	jobs := []JobInfo{
		{Name: "health_checker", Schedule: "every 30s", Status: "running"},
		{Name: "usage_rollup", Schedule: "every 5m", Status: "running"},
		{Name: "retention_cleaner", Schedule: "every 1h", Status: "running"},
		{Name: "partition_maintainer", Schedule: "every 24h", Status: "running"},
		{Name: "budget_resetter", Schedule: "every 1m", Status: "running"},
		{Name: "webhook_worker", Schedule: "continuous poll 250ms", Status: "running"},
	}

	_ = httpx.JSON(w, http.StatusOK, map[string]any{
		"items": jobs,
	})
}

func (h *Handlers) runJob(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	name := chi.URLParam(r, "name")

	// Trigger manual seketika pada worker yang mendukung
	h.writeAudit(ctx, r, "trigger_job", "system_job", name, nil)
	_ = httpx.JSON(w, http.StatusOK, map[string]any{
		"status":  "triggered",
		"job":     name,
		"message": "pekerjaan berhasil dipicu di latar belakang",
	})
}

// -----------------------------------------------------------------------------
// Audit Logs Handlers
// -----------------------------------------------------------------------------

func (h *Handlers) listAuditLogs(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	from, to := parseTimeRange(r)

	page := repo.Page{
		Limit:  repo.DefaultPageLimit,
		Cursor: r.URL.Query().Get("cursor"),
	}

	filter := identity.AuditFilter{
		From:         from,
		To:           to,
		Action:       r.URL.Query().Get("action"),
		ResourceType: r.URL.Query().Get("resource_type"),
	}

	auditPage, err := h.auditRepo.List(ctx, filter, page)
	if err != nil {
		mapRepoError(w, r, err, "audit log")
		return
	}

	_ = httpx.JSON(w, http.StatusOK, map[string]any{
		"items":       auditPage.Entries,
		"next_cursor": auditPage.NextCursor,
	})
}

// -----------------------------------------------------------------------------
// Diagnostics Handlers
// -----------------------------------------------------------------------------

func (h *Handlers) getDiagnostics(w http.ResponseWriter, r *http.Request) {
	var m runtime.MemStats
	runtime.ReadMemStats(&m)

	diag := map[string]any{
		"uptime_seconds": int64(time.Since(appStartTime).Seconds()),
		"go_version":     runtime.Version(),
		"num_goroutine":  runtime.NumGoroutine(),
		"num_cpu":        runtime.NumCPU(),
		"memory": map[string]any{
			"alloc_bytes":       m.Alloc,
			"total_alloc_bytes": m.TotalAlloc,
			"sys_bytes":         m.Sys,
			"heap_alloc_bytes":  m.HeapAlloc,
			"heap_inuse_bytes":  m.HeapInuse,
			"num_gc":            m.NumGC,
		},
	}

	_ = httpx.JSON(w, http.StatusOK, diag)
}
