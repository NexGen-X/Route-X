package admin

import (
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"runtime"
	"strconv"
	"strings"
	"time"

	"github.com/go-chi/chi/v5"

	"github.com/NexGen-X/Route-X/internal/auth"
	"github.com/NexGen-X/Route-X/internal/database/repo"
	"github.com/NexGen-X/Route-X/internal/database/repo/identity"
	"github.com/NexGen-X/Route-X/internal/database/seed"
	"github.com/NexGen-X/Route-X/internal/httpx"
	"github.com/NexGen-X/Route-X/internal/responsecache"
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
		// Respons domain hanya memuat status infrastruktur ringan (domain, mode
		// HTTPS, status DNS & IP server) — tidak ada kredensial atau rahasia,
		// sehingga cukup settings:read untuk membaca. Mutasi tetap settings:write.
		dr.With(auth.RequirePermission(seed.PermSettingsRead)).Get("/", h.getDomainStatus)
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

	// Database Backup & Disaster Recovery (Ekspor & Pemulihan SQL)
	r.Route("/backup", func(br chi.Router) {
		br.With(auth.RequirePermission(seed.PermSettingsRead)).Get("/status", h.getBackupStatus)
		br.With(auth.RequirePermission(seed.PermSettingsWrite)).Post("/export", h.exportDatabaseSQL)
		br.With(auth.RequirePermission(seed.PermSettingsWrite)).Post("/restore", h.restoreDatabaseSQL)
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

// kunciSettingTerlarang melaporkan apakah kunci setelan wajib diubah
// lewat endpoint khusus dan tidak boleh disentuh endpoint setelan
// generik — untuk TULIS maupun HAPUS. Penjaga ini dulu hanya ada di
// putSetting, sehingga DELETE generik bisa menghapus cli:config:* dan
// melewati alur khusus yang justru dijaga di jalur tulis (B-195).
//
// B-457 (TERBUKTI): daftarnya kini juga memuat kunci yang dimiliki
// handler khusus lain. putSetting generik sebelumnya bisa menulis
// system:response_cache melewati updateCacheSettings: nilai
// persisten berubah, tetapi engine yang berjalan tidak menerapkan
// setelan itu sampai boot ulang — keadaan yang dilaporkan settings
// dan keadaan cache yang berjalan berpisah diam-diam. Hal yang sama
// berlaku untuk kunci domain (system:domain:config &
// system:public_url milik updateDomainConfig/deleteDomainConfig):
// menuliskannya generik melewati validasi DNS, audit configure, dan
// penulisan pasangan kuncinya. Kunci khusus yang ada saat ini
// persis tiga itu ditambah awalan cli:config:* — handler khusus
// adalah satu-satunya jalur tulis/hapus yang sah untuk semuanya.
func kunciSettingTerlarang(key string) bool {
	if strings.HasPrefix(key, "cli:config:") {
		return true
	}
	switch key {
	case responsecache.SettingKey, SettingKeyDomainConfig, SettingKeyPublicURL:
		return true
	}
	return false
}

func (h *Handlers) putSetting(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	key := chi.URLParam(r, "key")
	if kunciSettingTerlarang(key) {
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
	if kunciSettingTerlarang(key) {
		httpx.BadRequest(w, r, "reserved_setting", "Setelan rahasia wajib diubah melalui endpoint khusus")
		return
	}

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
		tidakTersedia(w, r, "supervisor_unavailable", "supervisor worker latar belakang tidak aktif")
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
	from, to, ok := rentangAtau400(w, r)
	if !ok {
		return
	}

	// limit dihormati seperti endpoint daftar lain (pola listAPIKeys):
	// nilai tak sah/diabaikan memakai bawaan, dan repo.Page.Normalize
	// menjepitnya ke MaxPageLimit (B-139).
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

	filter := identity.AuditFilter{
		From:         from,
		To:           to,
		Action:       r.URL.Query().Get("action"),
		ResourceType: r.URL.Query().Get("resource_type"),
		// Filter aktor didukung repo sejak lama (AuditFilter.ActorUserID,
		// teruji di tingkat repo) tetapi tidak pernah dibaca handler ini,
		// jadi penyaringan per aktor tak terjangkau lewat API (B-139).
		ActorUserID: r.URL.Query().Get("actor_user_id"),
	}

	auditPage, err := h.auditRepo.List(ctx, filter, page)
	if err != nil {
		// identity.ErrInvalidInput (mis. actor_user_id bukan UUID) adalah
		// kesalahan pemanggil, bukan kegagalan server: mapRepoError tidak
		// mengenal sentinel paket identity itu dan akan menjawab 500, jadi
		// handler ini menerjemahkannya ke 400 di sini (B-139).
		if errors.Is(err, identity.ErrInvalidInput) {
			httpx.BadRequest(w, r, "invalid_parameter", "parameter actor_user_id harus berupa UUID pengguna")
			return
		}
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
		Version:       h.version,
		Commit:        h.commit,
		BuiltAt:       h.builtAt,
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
	// Keduanya penunjuk (B-399): bool/int64 polos melebur "tidak
	// disebut" menjadi false/0, jadi POST parsial seperti
	// {"ttl_seconds":600} ikut MEMATIKAN cache dan POST {}
	// menonaktifkannya dengan TTL 0. Kini yang tidak disebut
	// diwarisi dari setelan engine yang berlaku.
	Enabled    *bool  `json:"enabled"`
	TTLSeconds *int64 `json:"ttl_seconds"`
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
	// B-202: flush cache adalah aksi yang mengubah keadaan (membuang
	// semua entri) dan sebelumnya tidak meninggalkan jejak audit apa
	// pun, tidak seperti aksi sensitif lain di admin. Jalur engine tak
	// terpasang di atas memang no-op dan tetap tanpa audit.
	h.writeAudit(r.Context(), r, "flush", "response_cache", "", map[string]any{"deleted": n})
	_ = h.respond(w, r, http.StatusOK, CacheFlushResponseDTO{
		Deleted: n,
		Message: fmt.Sprintf("Berhasil menghapus %d entri response cache", n),
	})
}

func (h *Handlers) updateCacheSettings(w http.ResponseWriter, r *http.Request) {
	if h.responseCache == nil {
		tidakTersedia(w, r, "cache_unavailable", "Response cache engine tidak aktif")
		return
	}
	var req CacheSettingsRequest
	// Pakai decodeJSON bersama (audit F5): handler ini sebelumnya satu-satunya
	// yang men-decode tanpa batas 1 MB + DisallowUnknownFields.
	if err := decodeJSON(r, &req); err != nil {
		httpx.BadRequest(w, r, "invalid_json", err.Error())
		return
	}
	// B-399: body tanpa satu pun field bukan pembaruan — tolak
	// no_changes sejajar permukaan update lain (B-306/B-354).
	if req.Enabled == nil && req.TTLSeconds == nil {
		httpx.BadRequest(w, r, "no_changes", "tidak ada perubahan yang diminta untuk setelan response cache")
		return
	}
	// Gabungkan dengan setelan yang berlaku: field yang disebut
	// menang, sisanya diwarisi — POST parsial tidak lagi bisa
	// mematikan cache atau meng-nol-kan TTL secara tidak sengaja.
	enabled := h.responseCache.IsEnabled()
	if req.Enabled != nil {
		enabled = *req.Enabled
	}
	ttlDetik := int64(h.responseCache.TTL() / time.Second)
	if req.TTLSeconds != nil {
		ttlDetik = *req.TTLSeconds
	}
	// Validasi rentang TTL (atas nilai GABUNGAN) sebelum konversi
	// (audit F5): nilai negatif atau raksasa meluap int64 saat
	// dikali time.Second dan tersimpan sebagai durasi acak tanpa
	// satu pun penolakan.
	if ttlDetik < 0 || ttlDetik > 86400 {
		httpx.BadRequest(w, r, "invalid_ttl", "TTLSeconds harus dalam rentang 0–86400 detik")
		return
	}
	// Aktivasi DITERIMA sejak Batch C1: penolakan cache_policy_unsafe
	// historis menuntut fingerprint tenant, routing, dan policy — dan
	// ketiganya kini diterapkan di kunci cache itu sendiri
	// (responsecache.ComputeScopedKey): isolasi per API key dengan
	// identitas ter-hash terpisah domain, model kanonik, permukaan, dan
	// sidik rute; tahap awal hanya respons sukses non-streaming tanpa
	// tool yang dibaca/disimpan (gateway.layakCacheRespons). Tidak ada
	// knob di permukaan ini yang bisa melemahkan isolasi itu — kunci
	// selalu dibentuk kode, bukan pilihan operator — jadi tidak ada
	// konfigurasi tak aman yang perlu ditolak di sini.
	ttl := time.Duration(ttlDetik) * time.Second
	// Persistensikan SEBELUM menerapkan (audit F3): setelan yang hanya
	// hidup di memori hilang saat restart dan boot lama diam-diam
	// menyalakan cache lagi. Kini tabel settings adalah sumber kebenaran;
	// kegagalan menyimpannya menggagalkan handler, bukan sukses palsu.
	// Yang disimpan adalah nilai GABUNGAN (B-399): menyimpan nilai
	// mentah body parsial akan menimpa setelan persisten dengan
	// keadaan yang tidak pernah diminta operator.
	if h.settingsRepo != nil {
		var actorID string
		if p, ok := auth.PrincipalFrom(r.Context()); ok && p != nil {
			actorID = p.User.ID
		}
		persisted, _ := json.Marshal(responsecache.PersistedSettings{Enabled: enabled, TTLSeconds: ttlDetik})
		if _, err := h.settingsRepo.Put(r.Context(), identity.SettingWrite{
			Key:         responsecache.SettingKey,
			Value:       persisted,
			Description: "Setelan response cache (enabled + TTL) yang dipulihkan saat boot",
			UpdatedBy:   actorID,
		}); err != nil {
			mapRepoError(w, r, err, "setelan response cache")
			return
		}
	}
	// Kill switch: SetSettings(false, ...) berlaku seketika — IsEnabled
	// diperiksa pada setiap request, jadi penonaktifan menghentikan
	// penyajian HIT pada request berikutnya tanpa restart.
	h.responseCache.SetSettings(enabled, ttl)
	// B-202: perubahan saklar/TTL cache sebelumnya tidak diaudit —
	// jejaknya hanya baris settings generik tanpa aktor yang jelas.
	h.writeAudit(r.Context(), r, "update", "response_cache", "", map[string]any{
		"enabled":     enabled,
		"ttl_seconds": ttlDetik,
	})
	_ = h.respond(w, r, http.StatusOK, CacheSettingsResponseDTO{
		Enabled:    enabled,
		TTLSeconds: ttlDetik,
		Message:    "Pengaturan response cache berhasil diperbarui",
	})
}
