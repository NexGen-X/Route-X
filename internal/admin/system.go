package admin

import (
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"runtime"
	"strings"
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

	// Domain & Automatic HTTPS Management
	r.Route("/domain", func(dr chi.Router) {
		// Respons domain memuat tautan provisioning Xray yang setara kredensial.
		// Karena itu baca domain memerlukan settings:write, bukan akses Viewer.
		dr.With(auth.RequirePermission(seed.PermSettingsWrite)).Get("/", h.getDomainStatus)
		dr.With(auth.RequirePermission(seed.PermSettingsWrite)).Post("/", h.updateDomainConfig)
		dr.With(auth.RequirePermission(seed.PermSettingsWrite)).Delete("/", h.deleteDomainConfig)
	})

	// Background Jobs
	r.Route("/jobs", func(jr chi.Router) {
		jr.With(auth.RequirePermission(seed.PermHealthRead)).Get("/", h.listJobs)
		jr.With(auth.RequirePermission(seed.PermSettingsWrite)).Post("/{name}/run", h.runJob)
	})

	// Audit Logs
	r.With(auth.RequirePermission(seed.PermAuditRead)).Get("/audit-logs", h.listAuditLogs)

	// Diagnostics & System Overview
	r.With(auth.RequirePermission(seed.PermHealthRead)).Get("/diagnostics", h.getDiagnostics)
	r.With(auth.RequirePermission(seed.PermHealthRead)).Get("/overview", h.getSystemOverview)

	// Response Cache (Fitur 1)
	r.Route("/cache", func(cr chi.Router) {
		cr.With(auth.RequirePermission(seed.PermHealthRead)).Get("/", h.getCacheStats)
		cr.With(auth.RequirePermission(seed.PermSettingsWrite)).Post("/flush", h.flushCache)
		cr.With(auth.RequirePermission(seed.PermSettingsWrite)).Post("/settings", h.updateCacheSettings)
	})
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
	if strings.HasPrefix(key, "cli:config:") || key == SettingKeyXrayConfig {
		httpx.BadRequest(w, r, "reserved_setting", "Setelan rahasia wajib diubah melalui endpoint khusus")
		return
	}

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

// -----------------------------------------------------------------------------
// Response Cache Handlers
// -----------------------------------------------------------------------------

type CacheSettingsRequest struct {
	Enabled    bool  `json:"enabled"`
	TTLSeconds int64 `json:"ttl_seconds"`
}

func (h *Handlers) getCacheStats(w http.ResponseWriter, r *http.Request) {
	if h.responseCache == nil {
		_ = h.respond(w, r, http.StatusOK, CacheStatsDTO{
			Enabled:      false,
			TTLSeconds:   0,
			Hits:         0,
			Misses:       0,
			TotalEntries: 0,
		})
		return
	}
	stats, err := h.responseCache.Stats(r.Context())
	if err != nil {
		h.logger.ErrorContext(r.Context(), "gagal membaca statistik cache", "error", err)
		httpx.InternalError(w, r)
		return
	}
	_ = h.respond(w, r, http.StatusOK, CacheStatsDTO{
		Enabled:      stats.Enabled,
		TTLSeconds:   stats.TTLSeconds,
		Hits:         stats.Hits,
		Misses:       stats.Misses,
		TotalEntries: stats.TotalEntries,
	})
}

func (h *Handlers) flushCache(w http.ResponseWriter, r *http.Request) {
	if h.responseCache == nil {
		_ = h.respond(w, r, http.StatusOK, CacheFlushResponseDTO{
			Deleted: 0,
			Message: "Response cache engine tidak terpasang",
		})
		return
	}
	n, err := h.responseCache.Flush(r.Context())
	if err != nil {
		h.logger.ErrorContext(r.Context(), "gagal membersihkan response cache", "error", err)
		httpx.InternalError(w, r)
		return
	}
	_ = h.respond(w, r, http.StatusOK, CacheFlushResponseDTO{
		Deleted: n,
		Message: fmt.Sprintf("Berhasil menghapus %d entri response cache", n),
	})
}

func (h *Handlers) updateCacheSettings(w http.ResponseWriter, r *http.Request) {
	if h.responseCache == nil {
		httpx.BadRequest(w, r, "cache_unavailable", "Response cache engine tidak aktif")
		return
	}
	var req CacheSettingsRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		httpx.BadRequest(w, r, "invalid_json", "Body permintaan tidak valid")
		return
	}
	if req.Enabled {
		httpx.BadRequest(w, r, "cache_policy_unsafe", "Response cache dinonaktifkan sampai fingerprint tenant, routing, dan policy diterapkan")
		return
	}
	ttl := time.Duration(req.TTLSeconds) * time.Second
	h.responseCache.SetSettings(false, ttl)
	_ = h.respond(w, r, http.StatusOK, CacheSettingsResponseDTO{
		Enabled:    false,
		TTLSeconds: req.TTLSeconds,
		Message:    "Pengaturan response cache berhasil diperbarui",
	})
}
