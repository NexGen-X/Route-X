package admin

import (
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"runtime"
	"time"

	"github.com/go-chi/chi/v5"

	"github.com/NexGen-X/Route-X/internal/auth"
	"github.com/NexGen-X/Route-X/internal/database/repo"
	"github.com/NexGen-X/Route-X/internal/database/repo/identity"
	"github.com/NexGen-X/Route-X/internal/database/seed"
	"github.com/NexGen-X/Route-X/internal/httpx"
	"github.com/NexGen-X/Route-X/internal/worker"
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

	dtos := make([]SettingDTO, 0, len(items))
	for _, it := range items {
		dtos = append(dtos, toSettingDTO(it))
	}

	_ = h.respond(w, r, http.StatusOK, ListEnvelope[SettingDTO]{
		Items: dtos,
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

	_ = h.respond(w, r, http.StatusOK, toSettingDTO(s))
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
	_ = h.respond(w, r, http.StatusOK, toSettingDTO(s))
}

func (h *Handlers) deleteSetting(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	key := chi.URLParam(r, "key")

	if err := h.settingsRepo.Delete(ctx, key); err != nil {
		mapRepoError(w, r, err, "setelan sistem")
		return
	}

	h.writeAudit(ctx, r, "delete", "setting", key, nil)
	_ = h.respond(w, r, http.StatusOK, StatusResponse{Status: "deleted"})
}

// -----------------------------------------------------------------------------
// Jobs Handlers
// -----------------------------------------------------------------------------

func (h *Handlers) listJobs(w http.ResponseWriter, r *http.Request) {
	if h.supervisor == nil {
		_ = h.respond(w, r, http.StatusOK, ListEnvelope[JobDTO]{
			Items: make([]JobDTO, 0),
		})
		return
	}

	workerJobs := h.supervisor.Jobs()
	jobs := make([]JobDTO, len(workerJobs))
	for i, j := range workerJobs {
		jobs[i] = JobDTO{
			Name:           j.Name,
			Schedule:       j.Schedule,
			Interval:       j.Interval,
			Status:         j.Status,
			LastRunAt:      j.LastRunAt,
			LastStatus:     j.LastStatus,
			LastDurationMS: j.LastDurationMS,
			LastError:      j.LastError,
		}
	}

	_ = h.respond(w, r, http.StatusOK, ListEnvelope[JobDTO]{
		Items: jobs,
	})
}

func (h *Handlers) runJob(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	name := chi.URLParam(r, "name")

	if h.supervisor == nil {
		httpx.BadRequest(w, r, "supervisor_unavailable", "supervisor worker latar belakang tidak aktif")
		return
	}

	if err := h.supervisor.TriggerJob(ctx, name); err != nil {
		if errors.Is(err, worker.ErrJobNotFound) {
			httpx.NotFound(w, r, fmt.Sprintf("pekerjaan latar belakang %q tidak ditemukan", name))
			return
		}
		httpx.InternalError(w, r)
		return
	}

	// Trigger manual seketika pada worker yang valid
	h.writeAudit(ctx, r, "trigger_job", "system_job", name, nil)
	_ = h.respond(w, r, http.StatusOK, JobRunResponse{
		Status:  "triggered",
		Job:     name,
		Message: "pekerjaan berhasil dipicu di latar belakang",
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

	dtos := make([]AuditEntryDTO, 0, len(auditPage.Entries))
	for _, e := range auditPage.Entries {
		dtos = append(dtos, toAuditEntryDTO(e))
	}

	_ = h.respond(w, r, http.StatusOK, ListEnvelope[AuditEntryDTO]{
		Items:      dtos,
		NextCursor: auditPage.NextCursor,
	})
}

// -----------------------------------------------------------------------------
// Diagnostics Handlers
// -----------------------------------------------------------------------------

func (h *Handlers) getDiagnostics(w http.ResponseWriter, r *http.Request) {
	var m runtime.MemStats
	runtime.ReadMemStats(&m)

	var poolStats *DBPoolStatsDTO
	if h.pool != nil {
		stat := h.pool.Stat()
		poolStats = &DBPoolStatsDTO{
			TotalConns:       stat.TotalConns(),
			IdleConns:        stat.IdleConns(),
			AcquiredConns:    stat.AcquiredConns(),
			MaxConns:         stat.MaxConns(),
			AcquireCount:     stat.AcquireCount(),
			EmptyAcquireWait: stat.EmptyAcquireCount(),
		}
	}

	diag := DiagnosticsDTO{
		UptimeSeconds: int64(time.Since(appStartTime).Seconds()),
		GoVersion:     runtime.Version(),
		NumGoroutine:  runtime.NumGoroutine(),
		NumCPU:        runtime.NumCPU(),
		Memory: DiagnosticsMemoryDTO{
			AllocBytes:      m.Alloc,
			TotalAllocBytes: m.TotalAlloc,
			SysBytes:        m.Sys,
			HeapAllocBytes:  m.HeapAlloc,
			HeapInuseBytes:  m.HeapInuse,
			NumGC:           m.NumGC,
		},
		DBPool: poolStats,
	}

	_ = h.respond(w, r, http.StatusOK, diag)
}
