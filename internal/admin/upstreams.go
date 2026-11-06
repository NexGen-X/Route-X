package admin

import (
	"context"
	"crypto/tls"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"log/slog"
	"net"
	"net/http"
	"net/url"
	"slices"
	"strconv"
	"strings"
	"time"

	"github.com/go-chi/chi/v5"

	"github.com/NexGen-X/Route-X/internal/auth"
	"github.com/NexGen-X/Route-X/internal/database/repo"
	"github.com/NexGen-X/Route-X/internal/database/repo/traffic"
	"github.com/NexGen-X/Route-X/internal/database/repo/upstream"
	"github.com/NexGen-X/Route-X/internal/database/seed"
	"github.com/NexGen-X/Route-X/internal/httpx"
	"github.com/NexGen-X/Route-X/internal/observability"
	"github.com/NexGen-X/Route-X/internal/providers"
	"github.com/NexGen-X/Route-X/internal/security"
	"github.com/NexGen-X/Route-X/internal/webhooks"
)

// authorizeProviderAccess memeriksa izin akses terhadap provider.
//
// Aturan Keamanan BYOK: Provider dengan is_byok = true hanya boleh diakses oleh
// pemiliknya (owner_user_id == principal.User.ID) atau Super Admin. Bila bukan pemiliknya,
// fungsi mengembalikan repo.ErrNotFound agar keberadaan ID provider milik penyewa lain tidak bocor.
func (h *Handlers) authorizeProviderAccess(ctx context.Context, providerID string) (*upstream.Provider, error) {
	p, err := h.providerRepo.Get(ctx, providerID)
	if err != nil {
		return nil, err
	}
	if !p.IsBYOK {
		return p, nil
	}

	principal, ok := auth.PrincipalFrom(ctx)
	if !ok || principal == nil {
		return nil, repo.ErrNotFound
	}
	// Pemiliknya sendiri, atau admin penuh (B-102: is_admin adalah
	// satu-satunya pembeda kewenangan). Di luar keduanya provider
	// diperlakukan seolah tidak ada. Perbandingan pemilik ini yang
	// dijanjikan komentar di atas tetapi dulu tidak pernah dijalankan:
	// principal mana pun yang terautentikasi lolos (B-193).
	if principal.User.IsAdmin {
		return p, nil
	}
	if p.OwnerUserID != nil && *p.OwnerUserID == principal.User.ID {
		return p, nil
	}
	return nil, repo.ErrNotFound
}

func (h *Handlers) upstreamsRoutes(r chi.Router) {
	// 1. Providers
	r.Route("/providers", func(pr chi.Router) {
		pr.With(auth.RequirePermission(seed.PermProvidersRead)).Get("/", h.listProviders)
		pr.With(auth.RequirePermission(seed.PermProvidersWrite)).Post("/", h.createProvider)
		pr.With(auth.RequirePermission(seed.PermProvidersRead)).Get("/{id}", h.getProvider)
		pr.With(auth.RequirePermission(seed.PermProvidersWrite)).Put("/{id}", h.updateProvider)
		pr.With(auth.RequirePermission(seed.PermProvidersWrite)).Delete("/{id}", h.deleteProvider)
		pr.With(auth.RequirePermission(seed.PermProvidersWrite)).Post("/{id}/toggle", h.toggleProvider)
		pr.With(auth.RequirePermission(seed.PermHealthRead)).Get("/{id}/health-checks", h.listProviderHealthChecks)
		pr.With(auth.RequirePermission(seed.PermProvidersWrite)).Post("/{id}/probe", h.probeProvider)
		pr.With(auth.RequirePermission(seed.PermProvidersWrite)).Post("/{id}/sync-models", h.syncProviderModels)

		// Manajemen Model langsung di bawah Provider (Mode Personal / Self-Contained)
		pr.With(auth.RequirePermission(seed.PermModelsRead)).Get("/{id}/models", h.listProviderModels)
		pr.With(auth.RequirePermission(seed.PermModelsWrite)).Post("/{id}/models", h.addProviderModel)
		pr.With(auth.RequirePermission(seed.PermModelsWrite)).Delete("/{id}/models/{mapping_id}", h.detachProviderModel)
		pr.With(auth.RequirePermission(seed.PermProvidersWrite)).Post("/{id}/test-model", h.testProviderModel)

		// Kredensial di bawah provider
		pr.With(auth.RequirePermission(seed.PermProvidersRead)).Get("/{id}/credentials", h.listCredentials)
		pr.With(auth.RequirePermission(seed.PermCredentialsWrite)).Post("/{id}/credentials", h.createCredential)
		pr.With(auth.RequirePermission(seed.PermCredentialsWrite)).Delete("/{id}/credentials/{cred_id}", h.deleteCredential)
		pr.With(auth.RequirePermission(seed.PermCredentialsWrite)).Post("/{id}/credentials/{cred_id}/toggle", h.toggleCredential)
		pr.With(auth.RequirePermission(seed.PermCredentialsWrite)).Put("/{id}/credentials/{cred_id}/egress-pool", h.setCredentialEgressPool)

		// Strategi Rotasi Kredensial Universal
		pr.With(auth.RequirePermission(seed.PermProvidersWrite)).Post("/{id}/credential-strategy", h.setProviderCredentialStrategy)

		// OAuth Multi-Account di bawah provider
		pr.With(auth.RequirePermission(seed.PermProvidersRead)).Get("/{id}/oauth/sessions", h.listOAuthSessions)
		pr.With(auth.RequirePermission(seed.PermCredentialsWrite)).Post("/{id}/oauth/exchange", h.exchangeOAuthCode)
		pr.With(auth.RequirePermission(seed.PermCredentialsWrite)).Post("/{id}/oauth/refresh", h.refreshOAuthSession)
		pr.With(auth.RequirePermission(seed.PermCredentialsWrite)).Post("/{id}/oauth/sessions/{session_id}/toggle", h.toggleOAuthSession)
		pr.With(auth.RequirePermission(seed.PermCredentialsWrite)).Delete("/{id}/oauth/sessions/{session_id}", h.deleteOAuthSession)
	})

	// 2. Models
	r.Route("/models", func(mr chi.Router) {
		mr.With(auth.RequirePermission(seed.PermModelsRead)).Get("/", h.listModels)
		mr.With(auth.RequirePermission(seed.PermModelsWrite)).Post("/", h.createModel)
		mr.With(auth.RequirePermission(seed.PermModelsRead)).Get("/{id}", h.getModel)
		mr.With(auth.RequirePermission(seed.PermModelsWrite)).Put("/{id}", h.updateModel)
		mr.With(auth.RequirePermission(seed.PermModelsWrite)).Delete("/{id}", h.deleteModel)

		// Aliases
		mr.With(auth.RequirePermission(seed.PermModelsWrite)).Post("/{id}/aliases", h.addModelAlias)
		mr.With(auth.RequirePermission(seed.PermModelsWrite)).Delete("/{id}/aliases/{alias}", h.removeModelAlias)

		// Mappings (Attach Provider)
		mr.With(auth.RequirePermission(seed.PermModelsWrite)).Post("/{id}/mappings", h.attachProviderModel)
		mr.With(auth.RequirePermission(seed.PermModelsWrite)).Delete("/mappings/{mapping_id}", h.detachProviderModel)

		// Katalog acuan global (B-090) — segmen statis, terdaftar sebelum
		// "/{id}" supaya tidak tertangkap sebagai ID model.
		mr.With(auth.RequirePermission(seed.PermModelsRead)).Get("/pricing/reference-catalog", h.listReferencePriceCatalog)

		// Pricing
		mr.With(auth.RequirePermission(seed.PermModelsWrite)).Post("/mappings/{mapping_id}/pricing", h.setPricing)
		mr.With(auth.RequirePermission(seed.PermModelsRead)).Get("/mappings/{mapping_id}/pricing/history", h.listPricingHistory)
		mr.With(auth.RequirePermission(seed.PermModelsRead)).Get("/mappings/{mapping_id}/pricing/reference", h.getPricingReference)
	})

	// 3. Egress Pools
	r.Route("/egress-pools", func(er chi.Router) {
		er.With(auth.RequirePermission(seed.PermProvidersRead)).Get("/", h.listEgressPools)
		er.With(auth.RequirePermission(seed.PermProvidersWrite)).Post("/", h.createEgressPool)
		er.With(auth.RequirePermission(seed.PermProvidersWrite)).Post("/provision/cloudflare", h.provisionCloudflareAIGateway)
		er.With(auth.RequirePermission(seed.PermProvidersWrite)).Post("/provision/deno", h.provisionDenoRelay)
		er.With(auth.RequirePermission(seed.PermProvidersRead)).Get("/{id}", h.getEgressPool)
		er.With(auth.RequirePermission(seed.PermProvidersWrite)).Put("/{id}", h.updateEgressPool)
		er.With(auth.RequirePermission(seed.PermProvidersWrite)).Delete("/{id}", h.deleteEgressPool)
		er.With(auth.RequirePermission(seed.PermProvidersWrite)).Post("/{id}/test", h.testEgressPool)
	})
}

// -----------------------------------------------------------------------------
// Provider Handlers
// -----------------------------------------------------------------------------

func (h *Handlers) listProviders(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	limit := repo.DefaultPageLimit
	if s := r.URL.Query().Get("limit"); s != "" {
		if n, err := strconv.Atoi(s); err == nil && n > 0 {
			limit = n
		}
	}

	filter := upstream.ProviderFilter{
		Search: r.URL.Query().Get("search"),
		Kind:   r.URL.Query().Get("kind"),
	}
	if s := r.URL.Query().Get("enabled"); s != "" {
		en := s == "true"
		filter.Enabled = &en
	}
	if s := r.URL.Query().Get("is_byok"); s != "" {
		byok := s == "true"
		filter.IsBYOK = &byok
	}
	if s := r.URL.Query().Get("owner_user_id"); s != "" {
		filter.OwnerUserID = s
	}
	// Isolasi BYOK pada daftar (B-193): principal non-admin hanya melihat
	// provider bersama dan BYOK miliknya sendiri — sejajar
	// authorizeProviderAccess untuk akses per-ID. Filter repo untuk ini
	// (AccessibleByUserID) sudah lama ada tetapi tidak pernah dipasang.
	if principal, ok := auth.PrincipalFrom(ctx); ok && principal != nil && !principal.User.IsAdmin {
		filter.AccessibleByUserID = principal.User.ID
	}

	page := repo.Page{
		Limit:  limit,
		Cursor: r.URL.Query().Get("cursor"),
	}

	items, next, err := h.providerRepo.List(ctx, filter, page)
	if err != nil {
		mapRepoError(w, r, err, "provider")
		return
	}

	provs := make([]ProviderDTO, 0, len(items))
	for _, p := range items {
		provs = append(provs, toProviderDTO(p))
	}

	_ = h.respond(w, r, http.StatusOK, ListEnvelope[ProviderDTO]{
		Items:      provs,
		NextCursor: next,
	})
}

func (h *Handlers) getProvider(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	id := chi.URLParam(r, "id")

	p, err := h.authorizeProviderAccess(ctx, id)
	if err != nil {
		mapRepoError(w, r, err, "provider")
		return
	}

	_ = h.respond(w, r, http.StatusOK, toProviderDTO(p))
}

type createProviderReq struct {
	Name          string          `json:"name"`
	DisplayName   string          `json:"display_name"`
	Kind          string          `json:"kind"`
	BaseURL       string          `json:"base_url"`
	Enabled       *bool           `json:"enabled"`
	Priority      *int            `json:"priority"`
	Weight        *int            `json:"weight"`
	TimeoutMS     *int            `json:"timeout_ms"`
	MaxRetries    *int            `json:"max_retries"`
	RateLimitRPM  *int            `json:"rate_limit_rpm"`
	RateLimitTPM  *int            `json:"rate_limit_tpm"`
	MaxConcurrent *int            `json:"max_concurrent"`
	EgressPoolID  *string         `json:"egress_pool_id"`
	Metadata      json.RawMessage `json:"metadata"`
}

// validasiBaseURLProvider memeriksa base URL provider terhadap kebijakan
// SSRF yang berlaku dan menulis jawaban 400 bila ditolak. Mengembalikan true
// bila URL boleh disimpan/dipakai.
//
// Pemeriksaan ini dulu TIDAK ada di jalur simpan maupun sinkronisasi model:
// alamat metadata cloud (169.254.169.254) dan URL berkredensial tersimpan
// sebagai 201, dan sync model mengirim kunci provider ke URL tersimpan tanpa
// satu pun validasi (audit3 V-EO-09/V-EO-10). Kebijakannya meniru jalur
// inferensi: BYOK selalu paling ketat, provider operator mengikuti kebijakan
// operator (yang bisa mengizinkan HTTP/alamat privat secara sadar).
// Pesan ValidateBaseURL aman diteruskan: ia tidak pernah memuat kredensial
// (URL berkredensial ditolak dengan pesan statis).
func (h *Handlers) validasiBaseURLProvider(w http.ResponseWriter, r *http.Request, raw string, byok bool) bool {
	policy := security.DefaultSSRFPolicy()
	if !byok && h.factory != nil {
		policy = h.factory.SSRFPolicy()
	}
	if err := security.ValidateBaseURL(raw, policy); err != nil {
		httpx.BadRequest(w, r, "invalid_base_url", "base_url provider ditolak: "+err.Error())
		return false
	}
	return true
}

func (h *Handlers) createProvider(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	var req createProviderReq
	if err := decodeJSON(r, &req); err != nil {
		httpx.BadRequest(w, r, "invalid_json", err.Error())
		return
	}

	// base_url divalidasi SAAT DISIMPAN, bukan baru ketahuan salah saat
	// adapter dibangun atau kunci dikirim (audit3 V-EO-10).
	if !h.validasiBaseURLProvider(w, r, req.BaseURL, false) {
		return
	}

	// Normalisasi input agar toleran terhadap variasi UI dan ramah pengguna:
	// 1. Kind: "openai-compatible" dinormalisasi menjadi "openai_compatible"
	req.Kind = strings.TrimSpace(req.Kind)
	if req.Kind == "openai-compatible" {
		req.Kind = "openai_compatible"
	}
	// 2. Name: Bersihkan spasi, konversi ke huruf kecil, dan ganti spasi dengan strip
	req.Name = strings.TrimSpace(strings.ToLower(req.Name))
	req.Name = strings.ReplaceAll(req.Name, " ", "-")

	params := upstream.CreateProviderParams{
		Name:          req.Name,
		DisplayName:   req.DisplayName,
		Kind:          req.Kind,
		BaseURL:       req.BaseURL,
		Enabled:       req.Enabled,
		Priority:      req.Priority,
		Weight:        req.Weight,
		TimeoutMS:     req.TimeoutMS,
		MaxRetries:    req.MaxRetries,
		RateLimitRPM:  req.RateLimitRPM,
		RateLimitTPM:  req.RateLimitTPM,
		MaxConcurrent: req.MaxConcurrent,
		EgressPoolID:  req.EgressPoolID,
		Metadata:      req.Metadata,
	}

	var p *upstream.Provider
	err := repo.InTx(ctx, h.pool, func(q repo.Querier) error {
		txRepo := h.providerRepo.WithQuerier(q)
		var err error
		p, err = txRepo.Create(ctx, params)
		if err != nil {
			return err
		}
		return h.writeAuditTx(ctx, q, r, "create", "provider", p.ID, map[string]any{"name": p.Name})
	})
	if err != nil {
		mapRepoError(w, r, err, "provider")
		return
	}

	_ = h.respond(w, r, http.StatusCreated, toProviderDTO(p))
}

func (h *Handlers) updateProvider(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	id := chi.URLParam(r, "id")

	// Isolasi BYOK: pastikan pemanggil adalah pemilik provider atau Super Admin
	pAda, err := h.authorizeProviderAccess(ctx, id)
	if err != nil {
		mapRepoError(w, r, err, "provider")
		return
	}

	var req createProviderReq
	if err := decodeJSON(r, &req); err != nil {
		httpx.BadRequest(w, r, "invalid_json", err.Error())
		return
	}

	// Sama seperti pembuatan: base_url baru divalidasi sebelum disimpan
	// (audit3 V-EO-10).
	if req.BaseURL != "" && !h.validasiBaseURLProvider(w, r, req.BaseURL, pAda.IsBYOK) {
		return
	}

	var params upstream.UpdateProviderParams
	if req.Name != "" {
		nameNorm := strings.TrimSpace(strings.ToLower(req.Name))
		nameNorm = strings.ReplaceAll(nameNorm, " ", "-")
		params.Name = &nameNorm
	}
	if req.DisplayName != "" {
		params.DisplayName = &req.DisplayName
	}
	if req.Kind != "" {
		kindNorm := strings.TrimSpace(req.Kind)
		if kindNorm == "openai-compatible" {
			kindNorm = "openai_compatible"
		}
		params.Kind = &kindNorm
	}
	if req.BaseURL != "" {
		params.BaseURL = &req.BaseURL
	}
	params.Enabled = req.Enabled
	params.Priority = req.Priority
	params.Weight = req.Weight
	params.TimeoutMS = req.TimeoutMS
	params.MaxRetries = req.MaxRetries
	if req.RateLimitRPM != nil {
		params.RateLimitRPM = upstream.Set(*req.RateLimitRPM)
	}
	if req.RateLimitTPM != nil {
		params.RateLimitTPM = upstream.Set(*req.RateLimitTPM)
	}
	if req.MaxConcurrent != nil {
		params.MaxConcurrent = upstream.Set(*req.MaxConcurrent)
	}
	if req.EgressPoolID != nil {
		if *req.EgressPoolID == "" {
			params.EgressPoolID = upstream.Clear[string]()
		} else {
			params.EgressPoolID = upstream.Set(*req.EgressPoolID)
		}
	}
	params.Metadata = req.Metadata

	var p *upstream.Provider
	err = repo.InTx(ctx, h.pool, func(q repo.Querier) error {
		txRepo := h.providerRepo.WithQuerier(q)
		var err error
		p, err = txRepo.Update(ctx, id, params)
		if err != nil {
			return err
		}
		return h.writeAuditTx(ctx, q, r, "update", "provider", p.ID, map[string]any{"name": p.Name})
	})
	if err != nil {
		mapRepoError(w, r, err, "provider")
		return
	}

	_ = h.respond(w, r, http.StatusOK, toProviderDTO(p))
}

func (h *Handlers) deleteProvider(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	id := chi.URLParam(r, "id")

	// Isolasi BYOK: pastikan pemanggil berhak mengakses provider ini
	if _, err := h.authorizeProviderAccess(ctx, id); err != nil {
		mapRepoError(w, r, err, "provider")
		return
	}

	err := repo.InTx(ctx, h.pool, func(q repo.Querier) error {
		txRepo := h.providerRepo.WithQuerier(q)
		if err := txRepo.Delete(ctx, id); err != nil {
			return err
		}
		return h.writeAuditTx(ctx, q, r, "delete", "provider", id, nil)
	})
	if err != nil {
		mapRepoError(w, r, err, "provider")
		return
	}

	_ = h.respond(w, r, http.StatusOK, StatusResponse{Status: "deleted"})
}

func (h *Handlers) toggleProvider(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	id := chi.URLParam(r, "id")

	// Isolasi BYOK: pastikan pemanggil berhak mengakses provider ini
	if _, err := h.authorizeProviderAccess(ctx, id); err != nil {
		mapRepoError(w, r, err, "provider")
		return
	}

	enabled, err := decodeToggleEnabled(r)
	if err != nil {
		tolakToggle(w, r, err)
		return
	}

	var p *upstream.Provider
	err = repo.InTx(ctx, h.pool, func(q repo.Querier) error {
		txRepo := h.providerRepo.WithQuerier(q)
		var err error
		p, err = txRepo.SetEnabled(ctx, id, enabled)
		if err != nil {
			return err
		}
		return h.writeAuditTx(ctx, q, r, "toggle", "provider", id, map[string]any{"enabled": enabled})
	})
	if err != nil {
		mapRepoError(w, r, err, "provider")
		return
	}

	_ = h.respond(w, r, http.StatusOK, toProviderDTO(p))
}

func (h *Handlers) listProviderHealthChecks(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	id := chi.URLParam(r, "id")

	// Isolasi BYOK: pastikan pemanggil berhak mengakses provider ini
	if _, err := h.authorizeProviderAccess(ctx, id); err != nil {
		mapRepoError(w, r, err, "provider")
		return
	}

	page := repo.Page{
		Limit:  repo.DefaultPageLimit,
		Cursor: r.URL.Query().Get("cursor"),
	}

	items, next, err := h.providerRepo.ListHealthChecks(ctx, id, page)
	if err != nil {
		mapRepoError(w, r, err, "health checks")
		return
	}

	checks := make([]HealthCheckDTO, 0, len(items))
	for _, chk := range items {
		checks = append(checks, toHealthCheckDTO(chk))
	}

	_ = h.respond(w, r, http.StatusOK, ListEnvelope[HealthCheckDTO]{
		Items:      checks,
		NextCursor: next,
	})
}

func (h *Handlers) probeProvider(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	id := chi.URLParam(r, "id")

	// Isolasi BYOK: pastikan pemanggil berhak mengakses provider ini
	p, err := h.authorizeProviderAccess(ctx, id)
	if err != nil {
		mapRepoError(w, r, err, "provider")
		return
	}

	if h.factory == nil {
		tidakTersedia(w, r, "factory_unavailable", "pabrik provider tidak tersedia")
		return
	}

	adapter, err := h.factory.ProviderFor(ctx, p)
	if err != nil {
		h.gagalHulu(w, r, "gagal menyiapkan adapter untuk probe provider", "adapter_error",
			"Gagal menyiapkan adapter provider untuk pemeriksaan kesehatan", err)
		return
	}

	timeout := time.Duration(p.TimeoutMS) * time.Millisecond
	if timeout <= 0 {
		timeout = 10 * time.Second
	}
	chkCtx, cancel := context.WithTimeout(ctx, timeout)
	defer cancel()

	result := adapter.HealthCheck(chkCtx)
	latMS := int(result.Latency.Milliseconds())

	status := "unhealthy"
	if result.Healthy {
		status = "healthy"
	}

	sanitizedErr := webhooks.SanitizeErrorMessage(result.ErrorMessage)
	var sc *int
	if result.StatusCode > 0 {
		val := result.StatusCode
		sc = &val
	}

	_ = h.providerRepo.RecordHealth(ctx, id, upstream.HealthReport{
		Status:       status,
		LatencyMS:    &latMS,
		StatusCode:   sc,
		ErrorKind:    string(result.ErrorKind),
		ErrorMessage: sanitizedErr,
	})

	// 5.8: Tulis catatan audit untuk probe provider karena memicu panggilan jaringan keluar nyata
	h.writeAudit(ctx, r, "probe", "provider", id, map[string]any{
		"status":     status,
		"latency_ms": latMS,
		"healthy":    result.Healthy,
	})

	_ = h.respond(w, r, http.StatusOK, ProbeProviderResponse{
		Status:    status,
		LatencyMS: latMS,
		Error:     sanitizedErr,
	})
}

// modelLifecycle adalah metadata siklus hidup model SEBAGAIMANA disediakan
// respons discovery upstream (B-089). Hanya field yang benar-benar dikirim
// upstream yang diisi — gateway tidak menebak status untuk vendor yang
// tidak menyediakannya.
type modelLifecycle struct {
	Status           string
	ShutdownDate     string
	ReplacementModel string
}

// ada melaporkan apakah upstream menyediakan metadata lifecycle apa pun.
func (l *modelLifecycle) ada() bool {
	return l != nil && (l.Status != "" || l.ShutdownDate != "" || l.ReplacementModel != "")
}

// usang melaporkan apakah metadata ini menandai model sebagai usang /
// terjadwal mati: status yang dikenal sebagai penanda akhir masa, atau
// adanya tanggal shutdown. Model usang TIDAK dinonaktifkan otomatis —
// deprecated_at dan metadata adalah peringatan untuk operator, dan
// gateway tetap melayaninya (keputusan eksplisit B-089; menonaktifkan
// adalah keputusan operator, bukan tebakan gateway).
func (l *modelLifecycle) usang() bool {
	if l == nil {
		return false
	}
	if l.ShutdownDate != "" {
		return true
	}
	switch strings.ToLower(strings.TrimSpace(l.Status)) {
	case "deprecated", "shutdown", "sunset", "retired", "decommissioned", "end_of_life", "eol":
		return true
	}
	return false
}

// aktif melaporkan apakah metadata ini secara eksplisit menyatakan model
// MASIH hidup: metadata benar-benar disediakan upstream dan tidak
// menandai usang. Dipakai memulihkan deprecated_at yang terlanjur
// terpasang (B-207) — hanya bukti positif dari upstream yang boleh
// membersihkannya. Ketiadaan lifecycle BUKAN bukti apa pun: vendor yang
// tidak mengirim lifecycle tidak menyimpulkan modelnya hidup atau mati.
func (l *modelLifecycle) aktif() bool {
	return l.ada() && !l.usang()
}

// modelDiscovery adalah satu model hasil discovery beserta bukti upstream
// yang menyertainya: lifecycle (bila disediakan) dan kapabilitas yang
// DITURUNKAN DARI BUKTI respons (bukan tebakan nama — B-116 butir 9).
// Capabilities nil berarti upstream tidak menyediakan bukti kapabilitas.
type modelDiscovery struct {
	ID           string
	DisplayName  string
	Lifecycle    *modelLifecycle
	Capabilities []string
}

// metadataSyncModel menggabungkan metadata lifecycle + asal kapabilitas ke
// metadata model yang sudah ada, TANPA menghapus kunci lain milik operator.
// existing yang rusak/tidak terurai diperlakukan sebagai objek kosong —
// data lifecycle dari upstream tidak boleh hilang gara-gara metadata lama.
func metadataSyncModel(existing []byte, lc *modelLifecycle, capsSource string) []byte {
	obj := map[string]any{}
	if len(existing) > 0 {
		_ = json.Unmarshal(existing, &obj)
		if obj == nil {
			obj = map[string]any{}
		}
	}
	if lc != nil && lc.ada() {
		obj["lifecycle_source"] = "upstream_discovery"
		if lc.Status != "" {
			obj["lifecycle_status"] = lc.Status
		}
		if lc.ShutdownDate != "" {
			obj["lifecycle_shutdown_date"] = lc.ShutdownDate
		}
		if lc.ReplacementModel != "" {
			obj["lifecycle_replacement_model"] = lc.ReplacementModel
		}
	}
	if capsSource != "" {
		obj["capabilities_source"] = capsSource
	}
	out, err := json.Marshal(obj)
	if err != nil {
		return existing
	}
	return out
}

type openAIModelDiscoveryItem struct {
	ID string `json:"id"`
	// Field lifecycle gaya Intern-AI / OpenAI-compatible (B-089): tidak
	// semua vendor mengirimnya; yang tidak mengirim membiarkannya kosong
	// dan gateway tidak mengarangnya.
	Status           string `json:"status"`
	ShutdownDate     string `json:"shutdown_date"`
	ReplacementModel string `json:"replacement_model"`
}

type openAIModelDiscoveryResponse struct {
	Data []openAIModelDiscoveryItem `json:"data"`
	// Paginasi kursor gaya Anthropic (B-206): has_more/last_id hanya
	// dikirim dialek yang memang berkursor; vendor OpenAI-compatible
	// polos membiarkannya kosong dan gateway tidak mengarangnya.
	HasMore bool   `json:"has_more"`
	LastID  string `json:"last_id"`
}

type ollamaModelDiscoveryItem struct {
	Name  string `json:"name"`
	Model string `json:"model"`
}

type ollamaModelDiscoveryResponse struct {
	Models []ollamaModelDiscoveryItem `json:"models"`
}

type googleModelItem struct {
	Name        string `json:"name"`
	DisplayName string `json:"displayName"`
	// SupportedGenerationMethods adalah bukti kapabilitas dari API Google
	// sendiri (B-116 butir 9): "generateContent" → text, "embedContent" →
	// embeddings. Kosong berarti upstream tidak menyatakannya.
	SupportedGenerationMethods []string `json:"supportedGenerationMethods"`
}

// kapabilitasGoogle menurunkan kapabilitas kanonik dari metode generasi
// yang dinyatakan API Google untuk model ini. Metode lain (countTokens,
// predict, dst.) tidak punya padanan kapabilitas di skema Route-X dan
// sengaja tidak dipetakan.
func kapabilitasGoogle(methods []string) []string {
	var caps []string
	for _, m := range methods {
		switch m {
		case "generateContent":
			if !slices.Contains(caps, upstream.CapText) {
				caps = append(caps, upstream.CapText)
			}
		case "embedContent", "batchEmbedContents":
			if !slices.Contains(caps, upstream.CapEmbeddings) {
				caps = append(caps, upstream.CapEmbeddings)
			}
		}
	}
	return caps
}

type googleModelDiscoveryResponse struct {
	Models []googleModelItem `json:"models"`
	// NextPageToken adalah kursor paginasi Gemini (B-206); kosong
	// berarti halaman terakhir.
	NextPageToken string `json:"nextPageToken"`
}

// syncProviderModels menarik katalog model langsung dari discovery endpoint upstream
// (/v1/models, /api/tags untuk Ollama, atau v1beta/models untuk Google Gemini) dan mendaftarkannya ke database secara idempoten.
func (h *Handlers) syncProviderModels(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	id := chi.URLParam(r, "id")

	// 1. Otorisasi akses provider (termasuk proteksi multi-tenant BYOK)
	p, err := h.authorizeProviderAccess(ctx, id)
	if err != nil {
		mapRepoError(w, r, err, "provider")
		return
	}

	// 2. Validasi base_url tersimpan SEBELUM kredensial apa pun disiapkan:
	// sync mengirim kunci provider ke URL ini, dan versi lama tidak pernah
	// memvalidasinya — kunci melintas via HTTP polos ke URL yang tidak
	// pernah diperiksa bentuknya (audit3 V-EO-09). Kebijakannya sama dengan
	// jalur inferensi: BYOK paling ketat, operator mengikuti factory.
	ssrfPolicy := security.DefaultSSRFPolicy()
	if !p.IsBYOK && h.factory != nil {
		ssrfPolicy = h.factory.SSRFPolicy()
	}
	if err := security.ValidateBaseURL(p.BaseURL, ssrfPolicy); err != nil {
		httpx.BadRequest(w, r, "invalid_base_url", "base_url provider ditolak: "+err.Error())
		return
	}

	// 3. Ambil kredensial aktif jika ada dan dekripsi API key.
	//
	// Kegagalan mengambil kredensial TIDAK lagi ditelan (B-118): versi lama
	// meneruskan discovery TANPA API key pada setiap kegagalan Active —
	// termasuk keadaan "provider punya kredensial tetapi tidak satu pun
	// aktif", pola senyap yang sama dengan akar B-103 di jalur request.
	// Operator lalu melihat sync berjalan (atau hasil katalog publik) tanpa
	// tahu kredensialnya tidak dipakai. Kini keadaannya dibedakan persis
	// seperti pabrik adapter gateway (factory B-103):
	//   - punya kredensial, tak satu pun aktif → sync GAGAL eksplisit 409;
	//   - tidak pernah punya kredensial sama sekali → mode lokal tanpa
	//     key memang sah (Ollama/vLLM/LM Studio tanpa autentikasi);
	//   - kegagalan baca lain (dekripsi, database) → gagal eksplisit 500,
	//     bukan discovery tanpa key yang menyamar sebagai sukses.
	var apiKey string
	activeCred, credErr := h.credentialRepo.Active(ctx, p.ID)
	switch {
	case credErr == nil && activeCred != nil:
		apiKey = activeCred.Secret.Reveal()
	case credErr != nil && errors.Is(credErr, repo.ErrNotFound):
		ada, herr := h.credentialRepo.HasCredentials(ctx, p.ID)
		if herr != nil {
			h.gagalInternal(w, r, "keberadaan kredensial provider tidak bisa diperiksa untuk sinkronisasi model", herr)
			return
		}
		if ada {
			httpx.WriteError(w, r, http.StatusConflict, httpx.ErrTypeInvalidRequest, "provider_credentials_inactive",
				fmt.Sprintf("provider %q punya kredensial tetapi tidak satu pun aktif "+
					"(semua dinonaktifkan atau kedaluwarsa); sinkronisasi model tidak diteruskan tanpa kredensial", p.Name))
			return
		}
		// Tidak pernah punya kredensial: mode lokal tanpa API key.
	case credErr != nil:
		h.gagalInternal(w, r, "kredensial provider tidak bisa diambil untuk sinkronisasi model", credErr)
		return
	}

	// 4. Selesaikan egress provider SEBELUM klien discovery dibuat
	// (B-205): pool provider yang dipatok WAJIB dipakai jalur sync —
	// mengabaikannya mengeluarkan kredensial dari IP server melewati
	// kebijakan egress operator, dan provider yang hanya terjangkau
	// lewat pool sync-nya pasti gagal. Kegagalan memuat pool adalah
	// kegagalan eksplisit, bukan alasan jatuh ke koneksi langsung.
	proxyURL, err := h.egressSync(ctx, p, activeCred)
	if err != nil {
		h.gagalInternal(w, r, "egress provider tidak bisa diselesaikan untuk sinkronisasi model", err)
		return
	}

	// 5. Siapkan HTTP client dengan kebijakan SSRF wajib

	httpClient, err := providers.NewHTTPClient(providers.ClientConfig{
		Timeout:             15 * time.Second,
		SSRFPolicy:          ssrfPolicy,
		ProxyURL:            proxyURL,
		MaxIdleConnsPerHost: 2,
	}, false)
	if err != nil {
		h.gagalInternal(w, r, "gagal membuat klien HTTP discovery model", err)
		return
	}

	// 6. Susun discovery URL berdasarkan dialek provider
	var discoveryURL string
	var isiBody string
	method := http.MethodGet
	lowerKind := strings.ToLower(p.Kind)
	lowerName := strings.ToLower(p.Name)
	trimmedBase := strings.TrimRight(p.BaseURL, "/")
	isAntigravity := strings.Contains(trimmedBase, "cloudcode-pa.googleapis.com") || strings.Contains(lowerName, "antigravity")
	isGoogle := lowerKind == "google" || strings.Contains(trimmedBase, "generativelanguage.googleapis.com")

	if isAntigravity {
		method = http.MethodPost
		discoveryURL = trimmedBase + "/v1internal:fetchAvailableModels"
		if !strings.HasPrefix(discoveryURL, "http") {
			discoveryURL = "https://daily-cloudcode-pa.googleapis.com/v1internal:fetchAvailableModels"
		}
		// Project Code Assist (B-157): daftar model upstream bersifat
		// project-scoped — tanpa project, jawabannya hanya himpunan
		// generik dan model tergating-project tidak pernah terlihat.
		// Project diselesaikan dari sesi OAuth kredensial aktif (tersimpan
		// atau backfill sekali lewat loadCodeAssist); kegagalan resolusi
		// membiarkan body kosong seperti perilaku lama.
		isiBody = "{}"
		if project := h.projectCodeAssistSync(ctx, p, activeCred, apiKey); project != "" {
			if b, merr := json.Marshal(map[string]string{"project": project}); merr == nil {
				isiBody = string(b)
			}
		}
	} else if isGoogle {
		// URL discovery Google diturunkan dari base URL provider
		// (B-206): versi lama mengeraskan host
		// generativelanguage.googleapis.com sehingga provider Google
		// ber-base-URL kustom malah mengirim kuncinya ke host resmi.
		// Bentuk jalurnya sama dengan adapter google ("/v1beta/models"
		// di atas base URL terkonfigurasi).
		discoveryURL = trimmedBase + "/v1beta/models"
	} else if lowerKind == "ollama" || strings.Contains(lowerName, "ollama") {
		discoveryURL = trimmedBase + "/api/tags"
	} else if strings.HasSuffix(trimmedBase, "/v1") {
		discoveryURL = trimmedBase + "/models"
	} else {
		discoveryURL = trimmedBase + "/v1/models"
	}

	// buatReq menyusun permintaan discovery untuk satu URL halaman;
	// header dan autentikasi identik di setiap halaman.
	buatReq := func(urlHalaman string) (*http.Request, error) {
		var body io.Reader
		if isiBody != "" {
			body = strings.NewReader(isiBody)
		}
		rq, err := http.NewRequestWithContext(ctx, method, urlHalaman, body)
		if err != nil {
			return nil, err
		}
		rq.Header.Set("Accept", "application/json")
		if isAntigravity {
			// User-Agent hub adalah penentu katalog fetchAvailableModels
			// (B-157 tahap 4): nilai lain menerima himpunan generik.
			rq.Header.Set("User-Agent", providers.AntigravityHubUserAgent)
			rq.Header.Set("Content-Type", "application/json")
		} else {
			rq.Header.Set("User-Agent", "Route-X-Discovery/1.0")
		}
		if apiKey != "" {
			if isAntigravity || strings.HasPrefix(apiKey, "ya29.") || strings.HasPrefix(apiKey, "Bearer ") {
				rq.Header.Set("Authorization", "Bearer "+strings.TrimPrefix(apiKey, "Bearer "))
			} else if lowerKind == "anthropic" {
				rq.Header.Set("x-api-key", apiKey)
				rq.Header.Set("anthropic-version", "2023-06-01")
			} else if lowerKind == "google" || strings.Contains(trimmedBase, "googleapis.com") {
				rq.Header.Set("x-goog-api-key", apiKey)
			} else {
				rq.Header.Set("Authorization", "Bearer "+apiKey)
			}
		}
		return rq, nil
	}

	// ditemukan HANYA boleh memuat model beserta bukti upstreamnya
	// (lifecycle dan kapabilitas terbukti) dari respons yang benar-benar
	// diterima. Kegagalan discovery adalah kegagalan sync dan dilaporkan
	// apa adanya: menutupinya dengan katalog terkurasi membuat upstream
	// 401/5xx/tak terjangkau tampak seperti discovery berhasil dan
	// memasang model karangan sebagai model aktif provider (B-204).
	var ditemukan []modelDiscovery

	// Paginasi discovery (B-206): tanpa parameter ukuran halaman,
	// katalog Anthropic berhenti di ±20 model bawaan dan Google di
	// ±50 — terpotong senyap di tengah pesan sukses. Ukuran maksimum
	// dialek diminta sejak halaman pertama (persis adapter:
	// limit=1000 / pageSize=1000) dan kursor/token diikuti sampai
	// upstream menyatakan habis. Batas jumlah halaman menjaga sync
	// dari upstream yang tokennya tidak pernah habis; melewatinya
	// adalah kegagalan eksplisit, bukan pemotongan senyap berikutnya.
	const maksHalamanDiscovery = 100
	kursor := ""
	for halaman := 1; ; halaman++ {
		urlHalaman := discoveryURL
		q := url.Values{}
		switch {
		case isGoogle:
			q.Set("pageSize", "1000")
			if kursor != "" {
				q.Set("pageToken", kursor)
			}
		case lowerKind == "anthropic":
			q.Set("limit", "1000")
			if kursor != "" {
				q.Set("after_id", kursor)
			}
		case kursor != "":
			q.Set("after_id", kursor)
		}
		if len(q) > 0 {
			urlHalaman += "?" + q.Encode()
		}

		req, err := buatReq(urlHalaman)
		if err != nil {
			// err adalah *url.Error yang memuat URL tersimpan apa adanya;
			// yang diterima klien hanya pernyataan bahwa URL-nya tidak sah.
			h.logger.WarnContext(ctx, "URL discovery provider tidak sah", "provider_id", p.ID, "error", err)
			httpx.BadRequest(w, r, "invalid_url", "base_url provider tidak membentuk URL discovery yang sah")
			return
		}
		resp, err := httpClient.Do(req)
		if err != nil {
			h.logger.WarnContext(ctx, "panggilan discovery model gagal", "error", webhooks.SanitizeErrorMessage(err.Error()))
			httpx.WriteError(w, r, http.StatusBadGateway, httpx.ErrTypeAPI, "upstream_unreachable", "Gagal menghubungi endpoint discovery provider")
			return
		}
		if resp.StatusCode < 200 || resp.StatusCode >= 300 {
			_, _ = io.Copy(io.Discard, io.LimitReader(resp.Body, 512))
			resp.Body.Close()
			h.logger.WarnContext(ctx, "upstream mengembalikan status non-2xx saat discovery model", "status", resp.StatusCode)
			httpx.WriteError(w, r, http.StatusBadGateway, httpx.ErrTypeAPI, "upstream_error", fmt.Sprintf("Upstream mengembalikan status %d", resp.StatusCode))
			return
		}
		bodyBytes, readErr := io.ReadAll(io.LimitReader(resp.Body, 5<<20)) // 5MB limit
		resp.Body.Close()
		if readErr != nil {
			h.logger.ErrorContext(ctx, "gagal membaca respons discovery", "error", readErr)
			httpx.InternalError(w, r)
			return
		}

		// 7. Ekstraksi model IDs
		var oaiResp openAIModelDiscoveryResponse
		var ollamaResp ollamaModelDiscoveryResponse
		var googleResp googleModelDiscoveryResponse

		// Provider yang memang berdialek Google diurai dengan struct
		// Google-nya sendiri SEBELUM rantai tebak-bentuk di bawah:
		// bentuk {"models":[{"name":...}]} juga cocok dengan bentuk
		// Ollama, dan urutan lama menyerap respons Gemini sebagai
		// Ollama — awalan "models/" tidak dilepas dan nextPageToken
		// tidak pernah terbaca (paginasi Google mati sebelum mulai).
		googleTerurai := false
		if isGoogle && !isAntigravity && json.Unmarshal(bodyBytes, &googleResp) == nil && len(googleResp.Models) > 0 {
			googleTerurai = true
			for _, item := range googleResp.Models {
				name := strings.TrimSpace(item.Name)
				name = strings.TrimPrefix(name, "models/")
				if name != "" {
					ditemukan = append(ditemukan, modelDiscovery{
						ID:           name,
						Capabilities: kapabilitasGoogle(item.SupportedGenerationMethods),
					})
				}
			}
		}

		var antigravityResp struct {
			Models          map[string]json.RawMessage `json:"models"`
			AgentModelSorts []struct {
				Groups []struct {
					ModelIDs []string `json:"modelIds"`
				} `json:"groups"`
			} `json:"agentModelSorts"`
			CommandModelIds []string            `json:"commandModelIds"`
			TieredModelIds  map[string][]string `json:"tieredModelIds"`
		}

		if isAntigravity && json.Unmarshal(bodyBytes, &antigravityResp) == nil && (len(antigravityResp.Models) > 0 || len(antigravityResp.AgentModelSorts) > 0) {
			// Bentuk respons Antigravity yang teramati tidak menyediakan
			// metadata lifecycle/kapabilitas per model yang bisa
			// dipertanggungjawabkan — yang diambil ID-nya (B-089),
			// ditambah displayName sebagai bukti nama tampilan sejak
			// B-157. Nilai per model diurai longgar (RawMessage):
			// satu nilai berbentuk tak dikenal tidak boleh menggagalkan
			// pengambilan ID seluruh katalog.
			for id, mentah := range antigravityResp.Models {
				d := modelDiscovery{ID: id}
				var info struct {
					DisplayName string `json:"displayName"`
				}
				if json.Unmarshal(mentah, &info) == nil {
					d.DisplayName = strings.TrimSpace(info.DisplayName)
				}
				ditemukan = append(ditemukan, d)
			}
			for _, sort := range antigravityResp.AgentModelSorts {
				for _, g := range sort.Groups {
					for _, id := range g.ModelIDs {
						ditemukan = append(ditemukan, modelDiscovery{ID: id})
					}
				}
			}
			for _, id := range antigravityResp.CommandModelIds {
				ditemukan = append(ditemukan, modelDiscovery{ID: id})
			}
			for _, list := range antigravityResp.TieredModelIds {
				for _, id := range list {
					ditemukan = append(ditemukan, modelDiscovery{ID: id})
				}
			}
		} else if !googleTerurai && json.Unmarshal(bodyBytes, &oaiResp) == nil && len(oaiResp.Data) > 0 {
			for _, item := range oaiResp.Data {
				if trimmed := strings.TrimSpace(item.ID); trimmed != "" {
					d := modelDiscovery{ID: trimmed}
					// Metadata lifecycle hanya ada bila vendor memang
					// mengirimnya (Intern-AI: status/shutdown_date/
					// replacement_model). Vendor OpenAI-compatible lain
					// yang tidak mengirim field ini membiarkan Lifecycle
					// nil — tidak dikarang (B-089).
					lc := &modelLifecycle{
						Status:           strings.TrimSpace(item.Status),
						ShutdownDate:     strings.TrimSpace(item.ShutdownDate),
						ReplacementModel: strings.TrimSpace(item.ReplacementModel),
					}
					if lc.ada() {
						d.Lifecycle = lc
					}
					ditemukan = append(ditemukan, d)
				}
			}
		} else if !googleTerurai && json.Unmarshal(bodyBytes, &ollamaResp) == nil && len(ollamaResp.Models) > 0 {
			// /api/tags Ollama tidak menyediakan lifecycle maupun
			// kapabilitas yang bisa dipertanggungjawabkan — ID saja.
			for _, item := range ollamaResp.Models {
				name := strings.TrimSpace(item.Name)
				if name == "" {
					name = strings.TrimSpace(item.Model)
				}
				if name != "" {
					ditemukan = append(ditemukan, modelDiscovery{ID: name})
				}
			}
		} else if !googleTerurai && json.Unmarshal(bodyBytes, &googleResp) == nil && len(googleResp.Models) > 0 {
			for _, item := range googleResp.Models {
				name := strings.TrimSpace(item.Name)
				name = strings.TrimPrefix(name, "models/")
				if name != "" {
					ditemukan = append(ditemukan, modelDiscovery{
						ID:           name,
						Capabilities: kapabilitasGoogle(item.SupportedGenerationMethods),
					})
				}
			}
		}

		// Kursor halaman berikutnya dari respons yang baru diurai;
		// hanya dialek berkursor yang mengisinya.
		kursorBerikut := ""
		if oaiResp.HasMore && oaiResp.LastID != "" {
			kursorBerikut = oaiResp.LastID
		}
		if googleResp.NextPageToken != "" {
			kursorBerikut = googleResp.NextPageToken
		}
		if kursorBerikut == "" {
			break
		}
		if halaman >= maksHalamanDiscovery {
			h.logger.WarnContext(ctx, "paginasi discovery upstream tidak berhenti", "provider_id", p.ID, "halaman", halaman)
			httpx.WriteError(w, r, http.StatusBadGateway, httpx.ErrTypeAPI, "upstream_error", "Paginasi discovery upstream tidak berhenti; sinkronisasi dibatalkan agar katalog tidak terpasang sebagian")
			return
		}
		kursor = kursorBerikut
	}

	if len(ditemukan) == 0 {
		httpx.BadRequest(w, r, "no_models_found", "Tidak ada model yang ditemukan dalam respons upstream.")
		return
	}

	// Urutkan + dedup menurut ID; duplikat menggabungkan bukti pertama yang
	// tersedia (lifecycle/kapabilitas tidak saling menimpa dengan kekosongan).
	slices.SortFunc(ditemukan, func(a, b modelDiscovery) int { return strings.Compare(a.ID, b.ID) })
	unik := ditemukan[:0]
	for _, d := range ditemukan {
		if len(unik) > 0 && unik[len(unik)-1].ID == d.ID {
			if unik[len(unik)-1].Lifecycle == nil {
				unik[len(unik)-1].Lifecycle = d.Lifecycle
			}
			if len(unik[len(unik)-1].Capabilities) == 0 {
				unik[len(unik)-1].Capabilities = d.Capabilities
			}
			if unik[len(unik)-1].DisplayName == "" {
				unik[len(unik)-1].DisplayName = d.DisplayName
			}
			continue
		}
		unik = append(unik, d)
	}
	ditemukan = unik

	// 8. Simpan model dan pemetaan ke database secara idempoten.
	// Kegagalan per-model DIKUMPULKAN dan dicatat (audit F5): versi lama
	// menelan semuanya lalu menjawab "Berhasil menyinkronkan 0 model"
	// dengan status 200 walau yang gagal adalah penyimpanan lokal.
	var syncedModels []string
	var gagal []string
	for _, d := range ditemukan {
		mid := d.ID
		// Pastikan model kanonik ada
		m, err := h.modelRepo.GetByModelID(ctx, mid)
		if err != nil && !errors.Is(err, repo.ErrNotFound) {
			h.logger.WarnContext(ctx, "sinkronisasi model: gagal membaca model", "model_id", mid, "error", err)
			gagal = append(gagal, mid)
			continue
		}
		if errors.Is(err, repo.ErrNotFound) {
			// Kapabilitas model BARU diturunkan dari bukti upstream bila
			// ada (B-116 butir 9); tanpa bukti, bawaan {text} dipertahankan
			// TETAPI ditandai di metadata sebagai belum terverifikasi,
			// supaya operator tahu angka itu bukan pernyataan upstream —
			// model embeddings tanpa bukti tidak lagi tertolak diam-diam
			// tanpa penanda apa pun.
			caps := d.Capabilities
			capsSource := "upstream_discovery"
			if len(caps) == 0 {
				caps = []string{upstream.CapText}
				capsSource = "default_unverified"
			}
			// Nama tampilan dari bukti upstream bila ada (B-157); tanpa
			// bukti, nama = ID seperti sebelumnya — tidak dikarang.
			displayName := d.DisplayName
			if displayName == "" {
				displayName = mid
			}
			params := upstream.CreateModelParams{
				ModelID:      mid,
				DisplayName:  displayName,
				Capabilities: caps,
				Metadata:     metadataSyncModel(nil, d.Lifecycle, capsSource),
			}
			if d.Lifecycle.usang() {
				now := time.Now().UTC()
				params.DeprecatedAt = &now
			}
			m, err = h.modelRepo.Create(ctx, params)
			if err != nil {
				// Bila balapan konkurensi, coba baca ulang
				m, _ = h.modelRepo.GetByModelID(ctx, mid)
			}
		} else if err == nil && (d.Lifecycle.ada() || (d.DisplayName != "" && m.DisplayName == m.ModelID)) {
			// Model yang sudah ada: lifecycle upstream DIPERTAHANKAN ke
			// metadata (dan deprecated_at bila menandai usang) tanpa
			// menyentuh kapabilitas/nama yang mungkin sudah diatur
			// operator (B-089). Satu pengecualian nama sejak B-157: nama
			// yang MASIH berupa ID mentah (belum pernah disentuh operator)
			// diisi displayName upstream bila upstream kini
			// menyediakannya — nama yang sudah diatur operator tidak
			// pernah ditimpa. Kegagalan pembaruan ini dicatat sebagai
			// gagal per-model, bukan ditelan.
			params := upstream.UpdateModelParams{
				Metadata: metadataSyncModel(m.Metadata, d.Lifecycle, ""),
			}
			if d.Lifecycle.usang() && m.DeprecatedAt == nil {
				params.DeprecatedAt = upstream.Set(time.Now().UTC())
			}
			if d.Lifecycle.aktif() && m.DeprecatedAt != nil {
				// B-207: upstream kini menyatakan model ini hidup lagi —
				// deprecated_at dari sync sebelumnya dibersihkan.
				// Sebelumnya branch ini hanya bisa memasang, tidak
				// pernah membersihkan, sehingga model yang pulih
				// tercatat usang selamanya (asimetri tulis-vs-pakai).
				params.DeprecatedAt = upstream.Clear[time.Time]()
			}
			if d.DisplayName != "" && m.DisplayName == m.ModelID {
				params.DisplayName = &d.DisplayName
			}
			if updated, uerr := h.modelRepo.Update(ctx, m.ID, params); uerr == nil {
				m = updated
			} else {
				h.logger.WarnContext(ctx, "sinkronisasi model: gagal menyimpan metadata lifecycle", "model_id", mid, "error", uerr)
				gagal = append(gagal, mid)
				continue
			}
		}
		if m == nil {
			h.logger.WarnContext(ctx, "sinkronisasi model: model tidak tersimpan", "model_id", mid)
			gagal = append(gagal, mid)
			continue
		}

		// Pasang pemetaan provider_models
		_, attachErr := h.modelRepo.AttachProvider(ctx, upstream.AttachProviderParams{
			ModelID:           m.ID,
			ProviderID:        p.ID,
			UpstreamModelName: mid,
		})
		if attachErr == nil || errors.Is(attachErr, repo.ErrConflict) {
			syncedModels = append(syncedModels, mid)
		} else {
			h.logger.WarnContext(ctx, "sinkronisasi model: gagal memasang pemetaan provider", "model_id", mid, "error", attachErr)
			gagal = append(gagal, mid)
		}
	}

	if len(syncedModels) == 0 && len(gagal) > 0 {
		h.logger.ErrorContext(ctx, "sinkronisasi model gagal total", "provider_id", id, "gagal", len(gagal))
		httpx.InternalError(w, r)
		return
	}

	// B-207: model yang terpasang ke provider ini tetapi TIDAK lagi
	// ditemukan di upstream kini DILAPORKAN ke operator — sebelumnya
	// sync hanya menambah/memutakhirkan dan kepergian model dari katalog
	// upstream tidak terlihat di mana pun (model yatim terpasang
	// selamanya). Pelaporan saja, TIDAK dilepas/dinonaktifkan:
	// deprecated ≠ auto-disable adalah keputusan B-089, dan katalog
	// upstream yang sedang tidak lengkap tidak boleh memutus rute yang
	// masih bekerja. Himpunan pembanding adalah model yang DITEMUKAN
	// (bukan yang berhasil disimpan — kegagalan simpan lokal sudah
	// dilaporkan lewat daftar gagal, bukan sebagai model basi).
	// Kegagalan membaca pemetaan hanya meniadakan laporan ini; sync
	// yang sudah berhasil tidak digagalkan karenanya.
	staleModels := []string{}
	staleTerbaca := false
	terpasang, lerr := h.modelRepo.ListProviderModels(ctx, upstream.ProviderModelFilter{ProviderID: id})
	if lerr != nil {
		h.logger.WarnContext(ctx, "sinkronisasi model: gagal membaca pemetaan untuk laporan model basi", "provider_id", id, "error", lerr)
	} else {
		staleTerbaca = true
		ditemukanSet := make(map[string]bool, len(ditemukan))
		for _, d := range ditemukan {
			ditemukanSet[d.ID] = true
		}
		for _, pm := range terpasang {
			if !ditemukanSet[pm.UpstreamModelName] {
				staleModels = append(staleModels, pm.UpstreamModelName)
			}
		}
		slices.Sort(staleModels)
	}

	detailSync := map[string]any{
		"provider_name": p.Name,
		"models_count":  len(syncedModels),
		"failed_count":  len(gagal),
	}
	if staleTerbaca {
		detailSync["stale_models"] = staleModels
		detailSync["stale_count"] = len(staleModels)
	}
	h.writeAudit(ctx, r, "sync_models", "provider", id, detailSync)

	pesan := fmt.Sprintf("Berhasil menyinkronkan %d model dari provider %s", len(syncedModels), p.DisplayName)
	if len(gagal) > 0 {
		pesan += fmt.Sprintf("; %d model gagal disimpan (lihat log server)", len(gagal))
	}
	if staleTerbaca && len(staleModels) > 0 {
		pesan += fmt.Sprintf("; %d model terpasang tidak lagi ada di upstream: %s — tidak diubah, menunggu keputusan operator",
			len(staleModels), strings.Join(staleModels, ", "))
	}
	_ = h.respond(w, r, http.StatusOK, SyncModelsResponseDTO{
		Count:       len(syncedModels),
		Models:      syncedModels,
		StaleModels: staleModels,
		Message:     pesan,
	})
}

// egressSync menyelesaikan URL egress untuk jalur sinkronisasi model
// (B-205) dengan urutan yang sama seperti pabrik adapter gateway:
// pool milik provider, ditimpa pool khusus kredensial aktif bila ada.
//
// Provider tanpa pool menghasilkan Secret kosong — koneksi langsung
// memang konfigurasinya. Sebaliknya pool yang DINYATAKAN tetapi tidak
// bisa dimuat (tidak ada, nonaktif, atau gagal dekripsi) adalah error:
// meneruskan discovery tanpa proxy akan mengeluarkan kredensial dari
// IP server melewati kebijakan egress operator — pola yang ditolak
// keras jalur refresh OAuth dengan gagal-keras (V-EO-21), dan di sini
// pemanggil mengubahnya menjadi kegagalan sync yang eksplisit.
func (h *Handlers) egressSync(ctx context.Context, p *upstream.Provider, activeCred *upstream.ActiveCredential) (security.Secret, error) {
	poolID := ""
	if p.EgressPoolID != nil {
		poolID = *p.EgressPoolID
	}
	if activeCred != nil && activeCred.EgressPoolID != nil && *activeCred.EgressPoolID != "" {
		poolID = *activeCred.EgressPoolID
	}
	if poolID == "" {
		return "", nil
	}
	if h.egressRepo == nil {
		return "", errors.New("provider memakai egress pool tetapi repo egress tidak tersedia")
	}
	return h.egressRepo.ProxyURL(ctx, poolID)
}

// projectCodeAssistSync menyelesaikan project Code Assist untuk jalur
// sinkronisasi model admin (B-157).
//
// Nilai tersimpan pada sesi OAuth kredensial aktif dikembalikan apa adanya;
// sesi yang project-nya kosong diselesaikan SEKALI lewat loadCodeAssist
// memakai access token yang sama dengan yang dipakai discovery, lalu
// disimpan pada sesi (backfill malas — sesi yang login sebelum fitur ini
// ada sembuh tanpa login ulang). Setiap kegagalan menghasilkan "" dan sync
// berlanjut generik dengan body kosong seperti perilaku lama; kegagalannya
// tercatat, project tidak pernah dikarang.
func (h *Handlers) projectCodeAssistSync(ctx context.Context, p *upstream.Provider, activeCred *upstream.ActiveCredential, apiKey string) string {
	if h.oauthRepo == nil || activeCred == nil {
		return ""
	}
	meta, err := h.oauthRepo.GetSessionByCredentialID(ctx, activeCred.ID)
	if err != nil {
		if !errors.Is(err, repo.ErrNotFound) {
			h.logger.WarnContext(ctx, "gagal memuat sesi oauth untuk resolusi project code assist",
				"provider_id", p.ID, "error", err)
		}
		return ""
	}
	if !meta.Enabled {
		return ""
	}
	if meta.CodeAssistProject != nil && *meta.CodeAssistProject != "" {
		return *meta.CodeAssistProject
	}
	if h.oauthClient == nil || apiKey == "" {
		return ""
	}
	// Egress sesi dihormati persis seperti jalur factory (B-205): sesi
	// yang mematok egress pool memanggil loadCodeAssist LEWAT pool itu.
	// Pool yang tidak bisa dimuat berarti panggilan DILEWATI — memanggil
	// tanpa proxy mengeluarkan access token dari IP server melewati
	// kebijakan egress sesi; sync berlanjut tanpa project seperti
	// kegagalan resolusi lain di fungsi ini.
	var proxyURL security.Secret
	if meta.EgressPoolID != nil && *meta.EgressPoolID != "" {
		if h.egressRepo == nil {
			h.logger.WarnContext(ctx, "sesi memakai egress pool tetapi repo egress tidak tersedia; resolusi project saat sync dilewati",
				"provider_id", p.ID)
			return ""
		}
		px, perr := h.egressRepo.ProxyURL(ctx, *meta.EgressPoolID)
		if perr != nil {
			h.logger.WarnContext(ctx, "gagal memuat egress sesi untuk loadCodeAssist saat sync; resolusi project dilewati",
				"provider_id", p.ID, "error", perr)
			return ""
		}
		proxyURL = px
	}
	project, err := h.oauthClient.LoadCodeAssistProject(ctx, apiKey, proxyURL)
	if err != nil {
		h.logger.WarnContext(ctx, "loadCodeAssist gagal saat sinkronisasi model; sync berlanjut tanpa project",
			"provider_id", p.ID, "error", err)
		return ""
	}
	if project == "" {
		return ""
	}
	if err := h.oauthRepo.UpdateCodeAssistProject(ctx, meta.ID, project); err != nil {
		h.logger.WarnContext(ctx, "gagal menyimpan project code assist hasil resolusi sync",
			"provider_id", p.ID, "error", err)
	}
	return project
}

// listProviderModels mengembalikan daftar pemetaan model yang terhubung ke provider tertentu.
func (h *Handlers) listProviderModels(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	id := chi.URLParam(r, "id")

	// Isolasi BYOK: pastikan pemanggil berhak mengakses provider ini
	if _, err := h.authorizeProviderAccess(ctx, id); err != nil {
		mapRepoError(w, r, err, "provider")
		return
	}

	mappings, err := h.modelRepo.ListProviderModels(ctx, upstream.ProviderModelFilter{ProviderID: id})
	if err != nil {
		mapRepoError(w, r, err, "pemetaan provider model")
		return
	}

	dtos := make([]ModelMappingDTO, 0, len(mappings))
	for _, pm := range mappings {
		dtos = append(dtos, toModelMappingDTO(pm))
	}

	_ = h.respond(w, r, http.StatusOK, ListEnvelope[ModelMappingDTO]{
		Items: dtos,
	})
}

type addProviderModelReq struct {
	Name string `json:"name"`
}

// addProviderModel menambahkan pemetaan model secara manual ke provider yang dipilih.
func (h *Handlers) addProviderModel(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	id := chi.URLParam(r, "id")

	p, err := h.authorizeProviderAccess(ctx, id)
	if err != nil {
		mapRepoError(w, r, err, "provider")
		return
	}

	var req addProviderModelReq
	if err := decodeJSON(r, &req); err != nil {
		httpx.BadRequest(w, r, "invalid_json", err.Error())
		return
	}
	name := strings.TrimSpace(req.Name)
	if name == "" {
		httpx.BadRequest(w, r, "missing_name", "nama model wajib diisi")
		return
	}

	// Cari atau buat model kanonik terlebih dahulu. Kemampuan bawaan memakai
	// nilai yang dikenali constraint models_capabilities_known (migrasi 0004):
	// "chat"/"streaming" BUKAN kemampuan sah dan membuat Create selalu gagal.
	m, err := h.modelRepo.GetByModelID(ctx, name)
	if errors.Is(err, repo.ErrNotFound) {
		m, err = h.modelRepo.Create(ctx, upstream.CreateModelParams{
			ModelID:      name,
			DisplayName:  name,
			Capabilities: []string{upstream.CapText},
		})
		if err != nil {
			mapRepoError(w, r, err, "model")
			return
		}
	} else if err != nil {
		mapRepoError(w, r, err, "model")
		return
	}

	pm, err := h.modelRepo.AttachProvider(ctx, upstream.AttachProviderParams{
		ModelID:           m.ID,
		ProviderID:        p.ID,
		UpstreamModelName: name,
	})
	if err != nil {
		if errors.Is(err, repo.ErrConflict) {
			httpx.BadRequest(w, r, "already_attached", "model ini sudah terhubung ke provider")
			return
		}
		mapRepoError(w, r, err, "pemetaan provider model")
		return
	}

	h.writeAudit(ctx, r, "add_provider_model", "provider_model", pm.ID, map[string]any{
		"provider_id": p.ID,
		"model_name":  name,
	})

	_ = h.respond(w, r, http.StatusCreated, toModelMappingDTO(pm))
}

type testProviderModelReq struct {
	Model string `json:"model"`
}

// testProviderModel menguji konektivitas model tertentu langsung ke provider upstream.
func (h *Handlers) testProviderModel(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	id := chi.URLParam(r, "id")

	p, err := h.authorizeProviderAccess(ctx, id)
	if err != nil {
		mapRepoError(w, r, err, "provider")
		return
	}

	var req testProviderModelReq
	if err := decodeJSON(r, &req); err != nil {
		httpx.BadRequest(w, r, "invalid_json", err.Error())
		return
	}
	modelName := strings.TrimSpace(req.Model)
	if modelName == "" {
		httpx.BadRequest(w, r, "missing_model", "nama model untuk pengujian wajib diisi")
		return
	}

	if h.factory == nil {
		tidakTersedia(w, r, "factory_unavailable", "pabrik provider tidak tersedia")
		return
	}

	adapter, err := h.factory.ProviderFor(ctx, p)
	if err != nil {
		h.gagalHulu(w, r, "gagal menyiapkan adapter untuk uji model provider", "adapter_error",
			"Gagal menyiapkan adapter provider untuk pengujian model", err)
		return
	}

	testCtx, cancel := context.WithTimeout(ctx, 20*time.Second)
	defer cancel()

	start := time.Now()
	maxT := 1
	chatReq := &providers.ChatRequest{
		Model:     modelName,
		MaxTokens: &maxT,
		Messages: []providers.Message{
			{Role: providers.RoleUser, Content: "ping"},
		},
	}

	resp, chatErr := adapter.ChatCompletion(testCtx, chatReq)
	duration := time.Since(start)
	latMS := int(duration.Milliseconds())

	// Jejak akuntansi diagnostik (B-191): panggilan ini MEMAKAI token
	// upstream sungguhan, jadi ia wajib tampak sebagai baris requests —
	// sebelumnya respons dibuang dan pemakaiannya tidak tercatat di mana
	// pun. Penulisannya best-effort: kegagalannya tidak boleh mengubah
	// hasil uji yang dilaporkan ke admin.
	h.catatJejakUjiModel(ctx, r, p, modelName, resp, chatErr, latMS)

	if chatErr != nil {
		_ = h.respond(w, r, http.StatusOK, TestModelResponseDTO{
			Status:    "error",
			LatencyMS: latMS,
			Error:     chatErr.Error(),
		})
		return
	}

	_ = h.respond(w, r, http.StatusOK, TestModelResponseDTO{
		Status:    "success",
		LatencyMS: latMS,
		Message:   fmt.Sprintf("Model %s berhasil merespons (%d ms)", modelName, latMS),
	})
}

// catatJejakUjiModel menulis SATU baris requests diagnostik untuk satu
// panggilan uji model (B-191).
//
// Ini pengecualian akuntansi yang SADAR dan kini TERLIHAT: uji model
// melewati jalur gateway (tanpa gerbang anggaran, tanpa pencatatan
// pemakaian anggaran), tetapi ia mengonsumsi token upstream sungguhan —
// meninggalkannya tanpa baris berarti ada pemakaian yang tidak tampak di
// halaman Requests maupun total mana pun. Barisnya ditandai lewat kolom
// endpoint yang memuat pola rute admin uji-model itu sendiri, dan
// Unpriced mengikuti semantik B-190: token terpakai di sini tidak pernah
// dihargai, jadi total > 0 berarti belum berharga — bukan gratis.
// Kegagalan menulis hanya dicatat sebagai WARN; hasil uji untuk admin
// tidak pernah diubah oleh kegagalan diagnostik.
func (h *Handlers) catatJejakUjiModel(ctx context.Context, r *http.Request, p *upstream.Provider, modelName string, resp *providers.ChatResponse, chatErr error, latMS int) {
	if h.trafficRepo == nil {
		return
	}
	endpoint := ""
	if rc := chi.RouteContext(r.Context()); rc != nil {
		endpoint = rc.RoutePattern()
	}
	rec := traffic.Record{
		RequestID:      observability.RequestIDFrom(ctx),
		Method:         http.MethodPost,
		Endpoint:       endpoint,
		RequestedModel: modelName,
		ProviderID:     &p.ID,
		ProviderName:   &p.Name,
		UpstreamModel:  &modelName,
		StatusCode:     http.StatusOK,
		LatencyMS:      latMS,
	}
	if principal, ok := auth.PrincipalFrom(ctx); ok && principal != nil {
		uid := principal.User.ID
		rec.UserID = &uid
	}
	if resp != nil {
		rec.InputTokens = max(resp.Usage.InputTokens, 0)
		rec.OutputTokens = max(resp.Usage.OutputTokens, 0)
		rec.CachedInputTokens = max(resp.Usage.CachedInputTokens, 0)
		rec.ReasoningTokens = max(resp.Usage.ReasoningTokens, 0)
		rec.TotalTokens = max(resp.Usage.TotalTokens, 0)
		if rec.TotalTokens == 0 {
			rec.TotalTokens = rec.InputTokens + rec.OutputTokens
		}
		rec.Unpriced = rec.TotalTokens > 0
	}
	if chatErr != nil {
		rec.StatusCode = http.StatusBadGateway
		var perr *providers.Error
		if errors.As(chatErr, &perr) {
			if perr.StatusCode > 0 {
				rec.StatusCode = perr.StatusCode
			}
			if perr.Kind != "" {
				et := string(perr.Kind)
				rec.ErrorType = &et
			}
			if perr.UpstreamCode != "" {
				ec := perr.UpstreamCode
				rec.ErrorCode = &ec
			}
		}
		em := chatErr.Error()
		rec.ErrorMessage = &em
	}
	if _, _, err := h.trafficRepo.Insert(ctx, rec); err != nil {
		h.logger.WarnContext(ctx, "jejak diagnostik uji model gagal ditulis",
			"provider", p.Name, "model", modelName, "error", err)
	}
}

// -----------------------------------------------------------------------------
// Credentials Handlers
// -----------------------------------------------------------------------------

func (h *Handlers) listCredentials(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	id := chi.URLParam(r, "id")

	// Isolasi BYOK: pastikan pemanggil berhak mengakses provider ini
	if _, err := h.authorizeProviderAccess(ctx, id); err != nil {
		mapRepoError(w, r, err, "provider")
		return
	}

	items, err := h.credentialRepo.List(ctx, id)
	if err != nil {
		mapRepoError(w, r, err, "kredensial")
		return
	}

	creds := make([]CredentialMetaDTO, 0, len(items))
	for _, c := range items {
		creds = append(creds, toCredentialMetaDTO(c))
	}

	_ = h.respond(w, r, http.StatusOK, ListEnvelope[CredentialMetaDTO]{
		Items: creds,
	})
}

type createCredentialReq struct {
	Label        string  `json:"label"`
	APIKey       string  `json:"api_key"`
	ExpiresAt    *string `json:"expires_at"`
	EgressPoolID *string `json:"egress_pool_id"`
}

func (h *Handlers) createCredential(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	providerID := chi.URLParam(r, "id")

	// Isolasi BYOK: pastikan pemanggil berhak mengakses provider ini
	if _, err := h.authorizeProviderAccess(ctx, providerID); err != nil {
		mapRepoError(w, r, err, "provider")
		return
	}

	var req createCredentialReq
	if err := decodeJSON(r, &req); err != nil {
		httpx.BadRequest(w, r, "invalid_json", err.Error())
		return
	}
	if req.APIKey == "" {
		httpx.BadRequest(w, r, "missing_api_key", "api_key wajib diisi")
		return
	}

	var exp *time.Time
	if req.ExpiresAt != nil && *req.ExpiresAt != "" {
		t, err := time.Parse(time.RFC3339, *req.ExpiresAt)
		if err != nil {
			httpx.BadRequest(w, r, "invalid_expires_at", fmt.Sprintf("format expires_at tidak sah (harus RFC3339): %v", err))
			return
		}
		exp = &t
	}

	var poolID *string
	if req.EgressPoolID != nil && *req.EgressPoolID != "" {
		poolID = req.EgressPoolID
	}

	var actorID *string
	if p, ok := auth.PrincipalFrom(ctx); ok && p != nil {
		actorID = &p.User.ID
	}

	var cred *upstream.CredentialMeta
	err := repo.InTx(ctx, h.pool, func(q repo.Querier) error {
		txRepo := h.credentialRepo.WithQuerier(q)
		var err error
		cred, err = txRepo.Create(ctx, upstream.CreateCredentialParams{
			ProviderID:   providerID,
			Label:        req.Label,
			Secret:       security.Secret(req.APIKey),
			ExpiresAt:    exp,
			EgressPoolID: poolID,
			CreatedBy:    actorID,
		})
		if err != nil {
			return err
		}
		return h.writeAuditTx(ctx, q, r, "create", "credential", cred.ID, map[string]any{"provider_id": providerID, "label": cred.Label})
	})
	if err != nil {
		mapRepoError(w, r, err, "kredensial")
		return
	}

	h.kosongkanCacheRespons(ctx)
	_ = h.respond(w, r, http.StatusCreated, toCredentialMetaDTO(cred))
}

func (h *Handlers) deleteCredential(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	providerID := chi.URLParam(r, "id")
	credID := chi.URLParam(r, "cred_id")

	// Isolasi BYOK: pastikan pemanggil berhak mengakses provider ini
	if _, err := h.authorizeProviderAccess(ctx, providerID); err != nil {
		mapRepoError(w, r, err, "provider")
		return
	}

	// 5.1 & 5.8: Hapus kredensial bersyarat provider_id dan lakukan audit log di dalam transaksi yang sama
	err := repo.InTx(ctx, h.pool, func(q repo.Querier) error {
		txRepo := h.credentialRepo.WithQuerier(q)
		if err := txRepo.DeleteOfProvider(ctx, providerID, credID); err != nil {
			return err
		}
		return h.writeAuditTx(ctx, q, r, "delete", "credential", credID, map[string]any{"provider_id": providerID})
	})
	if err != nil {
		mapRepoError(w, r, err, "kredensial")
		return
	}

	h.kosongkanCacheRespons(ctx)
	_ = h.respond(w, r, http.StatusOK, StatusResponse{Status: "deleted"})
}

func (h *Handlers) toggleCredential(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	providerID := chi.URLParam(r, "id")
	credID := chi.URLParam(r, "cred_id")

	// Isolasi BYOK: pastikan pemanggil berhak mengakses provider ini
	if _, err := h.authorizeProviderAccess(ctx, providerID); err != nil {
		mapRepoError(w, r, err, "provider")
		return
	}

	enabled, err := decodeToggleEnabled(r)
	if err != nil {
		tolakToggle(w, r, err)
		return
	}

	// 5.1 & 5.8: Modifikasi status kredensial bersyarat provider_id dan lakukan audit log di dalam transaksi
	var cred *upstream.CredentialMeta
	err = repo.InTx(ctx, h.pool, func(q repo.Querier) error {
		txRepo := h.credentialRepo.WithQuerier(q)
		var err error
		cred, err = txRepo.SetEnabledOfProvider(ctx, providerID, credID, enabled)
		if err != nil {
			return err
		}
		return h.writeAuditTx(ctx, q, r, "toggle", "credential", credID, map[string]any{"provider_id": providerID, "enabled": enabled})
	})
	if err != nil {
		mapRepoError(w, r, err, "kredensial")
		return
	}

	h.kosongkanCacheRespons(ctx)
	_ = h.respond(w, r, http.StatusOK, toCredentialMetaDTO(cred))
}

type setCredentialEgressPoolReq struct {
	EgressPoolID *string `json:"egress_pool_id"`
}

func (h *Handlers) setCredentialEgressPool(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	providerID := chi.URLParam(r, "id")
	credID := chi.URLParam(r, "cred_id")
	if credID == "" {
		credID = chi.URLParam(r, "credId")
	}

	if _, err := h.authorizeProviderAccess(ctx, providerID); err != nil {
		mapRepoError(w, r, err, "provider")
		return
	}

	var req setCredentialEgressPoolReq
	if err := decodeJSON(r, &req); err != nil {
		httpx.BadRequest(w, r, "invalid_json", err.Error())
		return
	}

	poolID := req.EgressPoolID
	if poolID != nil && *poolID == "" {
		poolID = nil
	}

	// Verifikasi kredensial terdaftar di bawah provider ini
	cred, err := h.credentialRepo.Get(ctx, credID)
	if err != nil {
		mapRepoError(w, r, err, "kredensial")
		return
	}
	if cred.ProviderID != providerID {
		httpx.Forbidden(w, r, "Kredensial bukan milik provider ini")
		return
	}

	err = repo.InTx(ctx, h.pool, func(q repo.Querier) error {
		txRepo := h.credentialRepo.WithQuerier(q)
		if err := txRepo.SetCredentialEgressPool(ctx, credID, poolID); err != nil {
			return err
		}
		return h.writeAuditTx(ctx, q, r, "update_egress_pool", "credential", credID, map[string]any{
			"provider_id":    providerID,
			"egress_pool_id": poolID,
		})
	})
	if err != nil {
		mapRepoError(w, r, err, "kredensial")
		return
	}

	updated, err := h.credentialRepo.Get(ctx, credID)
	if err != nil {
		mapRepoError(w, r, err, "kredensial")
		return
	}

	// Pool egress mengubah jalur/akun hulu yang melayani kredensial ini —
	// konteks jawaban berubah tanpa mengubah sidik rute (B-258).
	h.kosongkanCacheRespons(ctx)

	_ = h.respond(w, r, http.StatusOK, toCredentialMetaDTO(updated))
}

// -----------------------------------------------------------------------------
// Models, Aliases, & Pricing Handlers
// -----------------------------------------------------------------------------

func (h *Handlers) listModels(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	limit := repo.DefaultPageLimit
	if s := r.URL.Query().Get("limit"); s != "" {
		if n, err := strconv.Atoi(s); err == nil && n > 0 {
			limit = n
		}
	}

	filter := upstream.ModelFilter{
		Search: r.URL.Query().Get("search"),
		Family: r.URL.Query().Get("family"),
	}
	if s := r.URL.Query().Get("enabled"); s != "" {
		en := s == "true"
		filter.Enabled = &en
	}

	page := repo.Page{
		Limit:  limit,
		Cursor: r.URL.Query().Get("cursor"),
	}

	items, next, err := h.modelRepo.List(ctx, filter, page)
	if err != nil {
		mapRepoError(w, r, err, "model")
		return
	}

	modelIDs := make([]string, len(items))
	for i, m := range items {
		modelIDs[i] = m.ID
	}

	provMap := make(map[string][]ModelProviderSummaryDTO)
	if h.pool != nil && len(modelIDs) > 0 {
		rows, qErr := h.pool.Query(ctx, `
			SELECT pm.model_id::text, p.id::text, p.name, p.display_name, pm.upstream_model_name
			FROM provider_models pm
			JOIN providers p ON pm.provider_id = p.id
			WHERE pm.model_id = ANY($1) AND pm.enabled = true
			ORDER BY p.name ASC
		`, modelIDs)
		if qErr != nil {
			mapRepoError(w, r, qErr, "provider model")
			return
		}
		defer rows.Close()
		for rows.Next() {
			var mid, pid, pname, pdisp, upstreamName string
			if err := rows.Scan(&mid, &pid, &pname, &pdisp, &upstreamName); err != nil {
				mapRepoError(w, r, err, "provider model")
				return
			}
			provMap[mid] = append(provMap[mid], ModelProviderSummaryDTO{
				ProviderID:        pid,
				ProviderName:      pname,
				DisplayName:       pdisp,
				UpstreamModelName: upstreamName,
			})
		}
		if err := rows.Err(); err != nil {
			mapRepoError(w, r, err, "provider model")
			return
		}
	}

	models := make([]ModelDTO, 0, len(items))
	for _, m := range items {
		dto := toModelDTO(m)
		if provs, ok := provMap[m.ID]; ok {
			dto.Providers = provs
		}
		models = append(models, dto)
	}

	_ = h.respond(w, r, http.StatusOK, ListEnvelope[ModelDTO]{
		Items:      models,
		NextCursor: next,
	})
}

func (h *Handlers) getModel(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	id := chi.URLParam(r, "id")

	m, err := h.modelRepo.Get(ctx, id)
	if err != nil {
		mapRepoError(w, r, err, "model")
		return
	}

	// Error sub-query diteruskan (audit F5): versi lama menelannya sehingga
	// kegagalan DB tampil sebagai "model ini tidak punya alias/mapping" —
	// data yang salah secara faktual, bukan daftar kosong yang sah.
	aliases, err := h.modelRepo.ListAliases(ctx, id)
	if err != nil {
		mapRepoError(w, r, err, "alias model")
		return
	}
	mappings, err := h.modelRepo.ListProviderModels(ctx, upstream.ProviderModelFilter{ModelID: id})
	if err != nil {
		mapRepoError(w, r, err, "pemetaan model")
		return
	}

	aliasesDTO := make([]ModelAliasDTO, 0, len(aliases))
	for _, a := range aliases {
		aliasesDTO = append(aliasesDTO, toModelAliasDTO(a))
	}

	mappingsDTO := make([]ModelMappingDTO, 0, len(mappings))
	for _, pm := range mappings {
		mappingsDTO = append(mappingsDTO, toModelMappingDTO(pm))
	}

	modelDTO := toModelDTO(m)
	if h.pool != nil {
		rows, qErr := h.pool.Query(ctx, `
			SELECT pm.model_id::text, p.id::text, p.name, p.display_name, pm.upstream_model_name
			FROM provider_models pm
			JOIN providers p ON pm.provider_id = p.id
			WHERE pm.model_id = $1 AND pm.enabled = true
			ORDER BY p.name ASC
		`, m.ID)
		if qErr != nil {
			mapRepoError(w, r, qErr, "provider model")
			return
		}
		defer rows.Close()
		for rows.Next() {
			var mid, pid, pname, pdisp, upstreamName string
			if err := rows.Scan(&mid, &pid, &pname, &pdisp, &upstreamName); err != nil {
				mapRepoError(w, r, err, "provider model")
				return
			}
			modelDTO.Providers = append(modelDTO.Providers, ModelProviderSummaryDTO{
				ProviderID:        pid,
				ProviderName:      pname,
				DisplayName:       pdisp,
				UpstreamModelName: upstreamName,
			})
		}
		if err := rows.Err(); err != nil {
			mapRepoError(w, r, err, "provider model")
			return
		}
	}

	_ = h.respond(w, r, http.StatusOK, ModelDetailResponse{
		Model:    modelDTO,
		Aliases:  aliasesDTO,
		Mappings: mappingsDTO,
	})
}

type createModelReq struct {
	ModelID         string   `json:"model_id"`
	DisplayName     string   `json:"display_name"`
	Family          *string  `json:"family"`
	Capabilities    []string `json:"capabilities"`
	ContextWindow   *int     `json:"context_window"`
	MaxOutputTokens *int     `json:"max_output_tokens"`
	Enabled         *bool    `json:"enabled"`
	RoutingPriority *int     `json:"routing_priority"`
	RoutingStrategy *string  `json:"routing_strategy"`
}

func (h *Handlers) createModel(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	var req createModelReq
	if err := decodeJSON(r, &req); err != nil {
		httpx.BadRequest(w, r, "invalid_json", err.Error())
		return
	}

	m, err := h.modelRepo.Create(ctx, upstream.CreateModelParams{
		ModelID:         req.ModelID,
		DisplayName:     req.DisplayName,
		Family:          req.Family,
		Capabilities:    req.Capabilities,
		ContextWindow:   req.ContextWindow,
		MaxOutputTokens: req.MaxOutputTokens,
		Enabled:         req.Enabled,
		RoutingPriority: req.RoutingPriority,
		RoutingStrategy: req.RoutingStrategy,
	})
	if err != nil {
		mapRepoError(w, r, err, "model")
		return
	}

	h.writeAudit(ctx, r, "create", "model", m.ID, map[string]any{"model_id": m.ModelID})
	_ = h.respond(w, r, http.StatusCreated, toModelDTO(m))
}

func (h *Handlers) updateModel(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	id := chi.URLParam(r, "id")

	var req createModelReq
	if err := decodeJSON(r, &req); err != nil {
		httpx.BadRequest(w, r, "invalid_json", err.Error())
		return
	}

	var params upstream.UpdateModelParams
	if req.ModelID != "" {
		params.ModelID = &req.ModelID
	}
	if req.DisplayName != "" {
		params.DisplayName = &req.DisplayName
	}
	if req.Family != nil {
		params.Family = upstream.Set(*req.Family)
	}
	params.Capabilities = req.Capabilities
	if req.ContextWindow != nil {
		params.ContextWindow = upstream.Set(*req.ContextWindow)
	}
	if req.MaxOutputTokens != nil {
		params.MaxOutputTokens = upstream.Set(*req.MaxOutputTokens)
	}
	if req.RoutingPriority != nil {
		params.RoutingPriority = req.RoutingPriority
	}
	if req.RoutingStrategy != nil {
		params.RoutingStrategy = upstream.Set(*req.RoutingStrategy)
	}
	params.Enabled = req.Enabled

	m, err := h.modelRepo.Update(ctx, id, params)
	if err != nil {
		mapRepoError(w, r, err, "model")
		return
	}

	h.writeAudit(ctx, r, "update", "model", m.ID, map[string]any{"model_id": m.ModelID})
	_ = h.respond(w, r, http.StatusOK, toModelDTO(m))
}

func (h *Handlers) deleteModel(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	id := chi.URLParam(r, "id")

	if err := h.modelRepo.Delete(ctx, id); err != nil {
		mapRepoError(w, r, err, "model")
		return
	}

	h.writeAudit(ctx, r, "delete", "model", id, nil)
	_ = h.respond(w, r, http.StatusOK, StatusResponse{Status: "deleted"})
}

func (h *Handlers) addModelAlias(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	modelID := chi.URLParam(r, "id")

	var req struct {
		Alias string `json:"alias"`
	}
	if err := decodeJSON(r, &req); err != nil {
		httpx.BadRequest(w, r, "invalid_json", err.Error())
		return
	}

	alias, err := h.modelRepo.AddAlias(ctx, modelID, req.Alias, nil)
	if err != nil {
		mapRepoError(w, r, err, "alias model")
		return
	}

	h.writeAudit(ctx, r, "add_alias", "model", modelID, map[string]any{"alias": req.Alias})
	_ = h.respond(w, r, http.StatusCreated, toModelAliasDTO(alias))
}

func (h *Handlers) removeModelAlias(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	modelID := chi.URLParam(r, "id")
	alias := chi.URLParam(r, "alias")

	if err := h.modelRepo.RemoveAlias(ctx, modelID, alias); err != nil {
		mapRepoError(w, r, err, "alias model")
		return
	}

	h.writeAudit(ctx, r, "remove_alias", "model", modelID, map[string]any{"alias": alias})
	_ = h.respond(w, r, http.StatusOK, StatusResponse{Status: "deleted"})
}

type attachProviderReq struct {
	ProviderID    string `json:"provider_id"`
	UpstreamModel string `json:"upstream_model"`
	Priority      *int   `json:"priority"`
	Weight        *int   `json:"weight"`
	TimeoutMS     *int   `json:"timeout_ms"`
	Enabled       *bool  `json:"enabled"`
}

func (h *Handlers) attachProviderModel(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	modelID := chi.URLParam(r, "id")

	// Dukung resolusi via nama kanonik (mis. "gpt-5") atau UUID
	if m, err := h.modelRepo.Resolve(ctx, modelID); err == nil && m != nil {
		modelID = m.ID
	}

	var req attachProviderReq
	if err := decodeJSON(r, &req); err != nil {
		httpx.BadRequest(w, r, "invalid_json", err.Error())
		return
	}

	pm, err := h.modelRepo.AttachProvider(ctx, upstream.AttachProviderParams{
		ModelID:           modelID,
		ProviderID:        req.ProviderID,
		UpstreamModelName: req.UpstreamModel,
		Priority:          req.Priority,
		Weight:            req.Weight,
		Enabled:           req.Enabled,
	})
	if err != nil {
		mapRepoError(w, r, err, "pemetaan provider model")
		return
	}

	h.writeAudit(ctx, r, "attach_provider", "model", modelID, map[string]any{"provider_id": req.ProviderID, "upstream_model": req.UpstreamModel})
	_ = h.respond(w, r, http.StatusCreated, toModelMappingDTO(pm))
}

func (h *Handlers) detachProviderModel(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	mappingID := chi.URLParam(r, "mapping_id")

	// B-276: pada rute bersarang /providers/{id}/models/{mapping_id},
	// segmen provider WAJIB diverifikasi sebagai pemilik pemetaan.
	// Sebelumnya segmen itu diabaikan total: pemetaan milik provider B
	// bisa dihapus lewat URL provider A dan handler menjawab sukses —
	// integritas referensial URL rusak walau haknya sama (mapping_id
	// UUID global). Rute datar /models/mappings/{mapping_id} tidak
	// menyebut provider, jadi pemeriksaan ini hanya berlaku bila
	// segmen {id} ada. Ketidakcocokan dijawab 404 yang sama seperti
	// pemetaan tidak ada (tidak membocorkan keberadaan pemetaan lain).
	if providerID := chi.URLParam(r, "id"); providerID != "" {
		pm, err := h.modelRepo.GetProviderModel(ctx, mappingID)
		if err != nil {
			mapRepoError(w, r, err, "pemetaan provider model")
			return
		}
		if pm.ProviderID != providerID {
			httpx.NotFound(w, r, "pemetaan tidak ditemukan pada provider ini")
			return
		}
	}

	if err := h.modelRepo.DetachProvider(ctx, mappingID); err != nil {
		mapRepoError(w, r, err, "pemetaan provider model")
		return
	}

	h.writeAudit(ctx, r, "detach_provider", "provider_model", mappingID, nil)
	_ = h.respond(w, r, http.StatusOK, StatusResponse{Status: "deleted"})
}

type setPriceReq struct {
	InputPer1MUSD  string `json:"input_per_1m_usd"`
	OutputPer1MUSD string `json:"output_per_1m_usd"`
	// CachedInputPer1MUSD OPSIONAL: tidak diisi (atau string kosong) berarti
	// harga cache-read mengikuti harga input — semantik nil pada repo. Versi
	// lama mewajibkannya, sehingga keadaan "ikuti input" tidak bisa dinyatakan
	// sama sekali, dan "0" yang diketik operator tersimpan sebagai harga
	// eksplisit nol yang membuat seluruh cache-read tertagih $0 (audit3 V-AB-13).
	// Nilai "0" yang DIISI tetap dihormati sebagai nol eksplisit.
	CachedInputPer1MUSD *string `json:"cached_input_per_1m_usd"`
}

func (h *Handlers) setPricing(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	mappingID := chi.URLParam(r, "mapping_id")

	var req setPriceReq
	if err := decodeJSON(r, &req); err != nil {
		httpx.BadRequest(w, r, "invalid_json", err.Error())
		return
	}

	inUSD, err := upstream.ParseUSD(req.InputPer1MUSD)
	if err != nil {
		httpx.BadRequest(w, r, "invalid_price", "input_per_1m_usd tidak valid: "+err.Error())
		return
	}
	outUSD, err := upstream.ParseUSD(req.OutputPer1MUSD)
	if err != nil {
		httpx.BadRequest(w, r, "invalid_price", "output_per_1m_usd tidak valid: "+err.Error())
		return
	}
	var cachedUSD *upstream.USD
	if req.CachedInputPer1MUSD != nil && strings.TrimSpace(*req.CachedInputPer1MUSD) != "" {
		v, err := upstream.ParseUSD(*req.CachedInputPer1MUSD)
		if err != nil {
			httpx.BadRequest(w, r, "invalid_price", "cached_input_per_1m_usd tidak valid: "+err.Error())
			return
		}
		cachedUSD = &v
	}

	var price *upstream.Price
	txErr := repo.InTx(ctx, h.pool, func(q repo.Querier) error {
		txRepo := upstream.NewPricingRepo(q)
		p, err := txRepo.Set(ctx, upstream.SetPriceParams{
			ProviderModelID: mappingID,
			Input:           inUSD,
			Output:          outUSD,
			CachedInput:     cachedUSD,
		})
		if err != nil {
			return err
		}
		price = p
		return nil
	})

	if txErr != nil {
		mapRepoError(w, r, txErr, "harga model")
		return
	}

	// Cuplikan harga di memori dibatalkan supaya harga baru menagih seketika di
	// instance ini, bukan sampai 30 detik kemudian (audit3 V-AB-09; Pricer.
	// Invalidate sebelumnya tidak punya satu pun pemanggil).
	if h.priceCache != nil {
		h.priceCache.Invalidate()
	}

	auditData := map[string]any{"input": inUSD.String(), "output": outUSD.String()}
	if cachedUSD != nil {
		auditData["cached"] = cachedUSD.String()
	}
	h.writeAudit(ctx, r, "set_pricing", "provider_model", mappingID, auditData)
	_ = h.respond(w, r, http.StatusCreated, toPriceDTO(price))
}

// getPricingReference menawarkan harga acuan resmi untuk satu pemetaan.
//
// Harga acuan katalog seed (seed.ReferencePriceFor) sebelumnya yatim: tidak
// ada satu pun pemanggil produksi, jadi operator yang memasang harga tidak
// pernah ditawari angka resmi beserta asalnya dan harus mengetik dari ingatan
// (audit3 V-AB-12). Endpoint ini hanya MENAWARKAN — memasangnya tetap
// keputusan operator lewat setPricing, karena harga acuan bisa berubah dan
// menanamnya otomatis adalah persis kesalahan yang dihindari katalog itu.
func (h *Handlers) getPricingReference(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	mappingID := chi.URLParam(r, "mapping_id")

	pm, err := h.modelRepo.GetProviderModel(ctx, mappingID)
	if err != nil {
		mapRepoError(w, r, err, "pemetaan provider model")
		return
	}
	prov, err := h.providerRepo.Get(ctx, pm.ProviderID)
	if err != nil {
		mapRepoError(w, r, err, "provider")
		return
	}

	dto := ReferencePriceDTO{ProviderKind: prov.Kind, UpstreamModel: pm.UpstreamModelName}
	if ref := seed.ReferencePriceFor(prov.Kind, pm.UpstreamModelName); ref != nil {
		dto = dtoReferencePrice(ref)
	}
	_ = h.respond(w, r, http.StatusOK, dto)
}

// dtoReferencePrice mengubah satu entri katalog seed menjadi DTO berstatus
// tersedia. Satu jalur konversi untuk endpoint per-pemetaan maupun katalog
// global, supaya angka dan asal yang ditampilkan tidak bisa berbeda.
func dtoReferencePrice(ref *seed.ReferencePrice) ReferencePriceDTO {
	dto := ReferencePriceDTO{
		Available:     true,
		ProviderKind:  ref.ProviderKind,
		UpstreamModel: ref.UpstreamModelName,
		Source:        ref.Source,
		FetchedOn:     ref.FetchedOn,
	}
	input := ref.Input.Decimal()
	output := ref.Output.Decimal()
	dto.InputPer1MUSD = &input
	dto.OutputPer1MUSD = &output
	if ref.CachedInput != nil {
		cached := ref.CachedInput.Decimal()
		dto.CachedInputPer1MUSD = &cached
	}
	return dto
}

// listReferencePriceCatalog mengembalikan SELURUH katalog harga acuan
// bawaan yang dikenal sistem (B-090).
//
// Sumbernya satu-satunya seed.ReferencePrices — himpunan kecil yang asal
// angkanya tercatat. Endpoint ini TIDAK mengarang katalog pasar: field
// note pada respons menyatakan cakupannya apa adanya, dan model yang tidak
// punya entri di sini tetap "harga belum diatur" di dashboard.
func (h *Handlers) listReferencePriceCatalog(w http.ResponseWriter, r *http.Request) {
	prices := seed.ReferencePrices()
	items := make([]ReferencePriceDTO, 0, len(prices))
	for i := range prices {
		items = append(items, dtoReferencePrice(&prices[i]))
	}
	_ = h.respond(w, r, http.StatusOK, ReferencePriceCatalogDTO{
		Items: items,
		Note:  "Daftar ini adalah seluruh harga acuan bawaan yang dikenal sistem (katalog seed), bukan katalog harga pasar yang lengkap. Model tanpa entri di sini tidak punya acuan yang bisa dipertanggungjawabkan dan harganya diisi operator sendiri.",
	})
}

func (h *Handlers) listPricingHistory(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	mappingID := chi.URLParam(r, "mapping_id")

	page := repo.Page{
		Limit:  repo.DefaultPageLimit,
		Cursor: r.URL.Query().Get("cursor"),
	}

	items, next, err := h.pricingRepo.History(ctx, mappingID, page)
	if err != nil {
		mapRepoError(w, r, err, "riwayat harga")
		return
	}

	prices := make([]PriceDTO, 0, len(items))
	for _, p := range items {
		prices = append(prices, toPriceDTO(p))
	}

	_ = h.respond(w, r, http.StatusOK, ListEnvelope[PriceDTO]{
		Items:      prices,
		NextCursor: next,
	})
}

// -----------------------------------------------------------------------------
// Egress Pools Handlers
// -----------------------------------------------------------------------------

func (h *Handlers) listEgressPools(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	enabledOnly := r.URL.Query().Get("enabled") == "true"

	items, err := h.egressRepo.List(ctx, enabledOnly)
	if err != nil {
		mapRepoError(w, r, err, "egress pool")
		return
	}

	pools := make([]EgressPoolDTO, 0, len(items))
	for _, ep := range items {
		pools = append(pools, toEgressPoolDTO(ep))
	}

	_ = h.respond(w, r, http.StatusOK, ListEnvelope[EgressPoolDTO]{
		Items: pools,
	})
}

func (h *Handlers) getEgressPool(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	id := chi.URLParam(r, "id")

	ep, err := h.egressRepo.Get(ctx, id)
	if err != nil {
		mapRepoError(w, r, err, "egress pool")
		return
	}

	_ = h.respond(w, r, http.StatusOK, toEgressPoolDTO(ep))
}

type createEgressReq struct {
	Name string `json:"name"`
	// Description adalah pointer supaya PUT bisa membedakan "field tidak
	// dikirim" (biarkan) dari "dikirim kosong" (kosongkan) — B-116 butir 1:
	// field ini dulu diterima lalu hilang karena tidak ada kolom/DTO-nya.
	Description *string `json:"description"`
	Kind        string  `json:"kind"`
	ProxyURL    string  `json:"proxy_url"`
	Enabled     *bool   `json:"enabled"`
	Weight      *int    `json:"weight"`
	Region      *string `json:"region"`
}

func (h *Handlers) createEgressPool(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	var req createEgressReq
	if err := decodeJSON(r, &req); err != nil {
		httpx.BadRequest(w, r, "invalid_json", err.Error())
		return
	}
	if err := security.ValidateProxyURL(req.ProxyURL, h.ssrfPolicy()); err != nil {
		httpx.BadRequest(w, r, "invalid_proxy_url", err.Error())
		return
	}

	if req.ProxyURL == "" {
		httpx.BadRequest(w, r, "missing_proxy_url", "proxy_url wajib diisi")
		return
	}

	kind := strings.TrimSpace(strings.ToLower(req.Kind))
	if kind == "" {
		kind = "http"
	}

	var actorID *string
	if p, ok := auth.PrincipalFrom(ctx); ok && p != nil {
		actorID = &p.User.ID
	}

	description := ""
	if req.Description != nil {
		description = *req.Description
	}
	ep, err := h.egressRepo.Create(ctx, upstream.CreateEgressParams{
		Name:        req.Name,
		Kind:        kind,
		Description: description,
		ProxyURL:    security.Secret(req.ProxyURL),
		Enabled:     req.Enabled,
		Weight:      req.Weight,
		Region:      req.Region,
		CreatedBy:   actorID,
	})
	if err != nil {
		mapRepoError(w, r, err, "egress pool")
		return
	}

	h.writeAudit(ctx, r, "create", "egress_pool", ep.ID, map[string]any{"name": ep.Name})

	// B-339: pool egress baru mengubah jalur hulu yang bisa
	// melayani; kosongkan cache respons seperti mutasi kredensial
	// lain (B-258).
	h.kosongkanCacheRespons(ctx)

	_ = h.respond(w, r, http.StatusCreated, toEgressPoolDTO(ep))
}

func (h *Handlers) updateEgressPool(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	id := chi.URLParam(r, "id")

	var req createEgressReq
	if err := decodeJSON(r, &req); err != nil {
		httpx.BadRequest(w, r, "invalid_json", err.Error())
		return
	}

	if req.ProxyURL != "" {
		if err := security.ValidateProxyURL(req.ProxyURL, h.ssrfPolicy()); err != nil {
			httpx.BadRequest(w, r, "invalid_proxy_url", err.Error())
			return
		}
	}

	var params upstream.UpdateEgressParams
	if req.Name != "" {
		params.Name = &req.Name
	}
	if req.Kind != "" {
		kindNorm := strings.TrimSpace(strings.ToLower(req.Kind))
		params.Kind = &kindNorm
	}
	// Description diteruskan apa adanya bila field-nya dikirim (termasuk
	// kosong = kosongkan); absen berarti biarkan (B-116 butir 1).
	params.Description = req.Description
	params.Enabled = req.Enabled
	params.Weight = req.Weight
	if req.Region != nil {
		params.Region = upstream.Set(*req.Region)
	}

	// B-270: mutasi dua langkah (field + URL proxy) dan auditnya berjalan
	// dalam SATU transaksi — pola updateProvider. Sebelumnya Update
	// ter-commit lebih dulu dan SetProxyURL menyusul sebagai query terpisah:
	// kegagalan langkah kedua meninggalkan mutasi pertama permanen TANPA
	// baris audit (audit hanya ditulis sesudah keduanya berhasil).
	// B-306: permintaan yang HANYA membawa proxy_url (rotasi kredensial
	// murni dari klien API langsung) tidak punya field untuk Update —
	// memanggilnya membuat repo menolak dengan ErrNoChanges dan seluruh
	// PUT gagal 500 sebelum rotasinya sempat berjalan. Update kini
	// hanya dipanggil bila memang ada perubahan field; proxy_url
	// menyusul lewat SetProxyURL seperti sebelumnya, tetap dalam
	// transaksi dan audit yang sama (B-270).
	adaPerubahanField := params.Name != nil || params.Kind != nil || params.Description != nil ||
		params.Enabled != nil || params.Weight != nil || params.Region.Set

	var ep *upstream.EgressPool
	err := repo.InTx(ctx, h.pool, func(q repo.Querier) error {
		txRepo := h.egressRepo.WithQuerier(q)
		var err error
		if adaPerubahanField {
			ep, err = txRepo.Update(ctx, id, params)
			if err != nil {
				return err
			}
		}
		if req.ProxyURL != "" {
			ep, err = txRepo.SetProxyURL(ctx, id, security.Secret(req.ProxyURL))
			if err != nil {
				return err
			}
		}
		if ep == nil {
			// Tidak ada perubahan field dan tidak ada proxy_url: tidak
			// ada yang dikerjakan — 400 no_changes lewat mapRepoError.
			return upstream.ErrNoChanges
		}
		return h.writeAuditTx(ctx, q, r, "update", "egress_pool", ep.ID, map[string]any{"name": ep.Name})
	})
	if err != nil {
		mapRepoError(w, r, err, "egress pool")
		return
	}

	// B-339: pool yang berubah mengubah jalur hulu yang melayani;
	// kosongkan cache respons (lihat createEgressPool).
	h.kosongkanCacheRespons(ctx)

	_ = h.respond(w, r, http.StatusOK, toEgressPoolDTO(ep))
}

func (h *Handlers) deleteEgressPool(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	id := chi.URLParam(r, "id")

	if err := h.egressRepo.Delete(ctx, id); err != nil {
		mapRepoError(w, r, err, "egress pool")
		return
	}

	h.writeAudit(ctx, r, "delete", "egress_pool", id, nil)

	// B-339: pool terhapus tidak lagi melayani; kosongkan cache
	// respons (lihat createEgressPool).
	h.kosongkanCacheRespons(ctx)

	_ = h.respond(w, r, http.StatusOK, StatusResponse{Status: "deleted"})
}

// testEgressPool melakukan uji konektivitas riil ke jalur proxy keluar via endpoint Cloudflare trace,
// mengukur latensi jaringan riil, serta mendeteksi IP keluar publik dan negara.
func (h *Handlers) testEgressPool(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	id := chi.URLParam(r, "id")

	ep, err := h.egressRepo.Get(ctx, id)
	if err != nil {
		mapRepoError(w, r, err, "egress pool")
		return
	}

	// Diagnostik sengaja memakai varian tanpa syarat aktif: menguji pool
	// yang sedang dinonaktifkan adalah sah, sedangkan ProxyURL jalur lalu
	// lintas menolak pool nonaktif (audit3 V-EO-01).
	proxySecret, err := h.egressRepo.ProxyURLDiagnostik(ctx, id)
	if err != nil {
		mapRepoError(w, r, err, "egress pool proxy_url")
		return
	}

	proxyRaw := proxySecret.Reveal()
	if proxyRaw == "" {
		httpx.BadRequest(w, r, "empty_proxy_url", "URL proxy kosong")
		return
	}

	parsedProxy, err := url.Parse(proxyRaw)
	if err != nil {
		// err memuat URL proxy mentah beserta kredensialnya (*url.Error);
		// klien hanya menerima pernyataan bahwa formatnya tidak valid
		// (audit3 V-EO-23).
		h.logger.WarnContext(ctx, "URL proxy tersimpan tidak bisa diurai saat probe egress", "error", err)
		httpx.BadRequest(w, r, "invalid_proxy_url", "Format URL proxy tidak valid")
		return
	}

	// Dial ke proksi (dan fallback langsung bila proxy tidak menghidangkan koneksi)
	// diperiksa terhadap kebijakan SSRF — dulu transport ini tanpa DialContext
	// berpenjaga sehingga probe tidak tervalidasi (BE-002).
	ssrfPolicy := security.DefaultSSRFPolicy()
	if h.factory != nil {
		ssrfPolicy = h.factory.SSRFPolicy()
	}
	dialer := &net.Dialer{Timeout: 7 * time.Second}

	var transport *http.Transport
	var probeURL string
	var extraHeaders map[string]string
	// probeGatewayCF menandai probe ke URL gateway Cloudflare milik pool:
	// untuknya, respons HTTP apa pun (termasuk 401/404) berarti gateway ada
	// dan terjangkau — bukan kegagalan.
	probeGatewayCF := false

	// Klasifikasi dari host hasil uraian (KlasifikasiRelay), bukan substring
	// URL mentah (audit3 V-EO-08).
	jenisRelay, _ := security.KlasifikasiRelay(proxyRaw)
	if jenisRelay == security.RelayCloudflare {
		// Cloudflare AI Gateway adalah AI reverse proxy edge, bukan forward proxy HTTP CONNECT.
		// Yang diuji adalah URL gateway MILIK pool ini (akun + gateway di
		// path): versi lama mem-probe domain trace umum yang sama untuk
		// semua pool, sehingga gateway yang tidak pernah terbuat pun tampak
		// sehat (audit3 V-EO-05).
		transport = &http.Transport{
			DialContext:       security.GuardedDialContext(ssrfPolicy, dialer),
			TLSClientConfig:   &tls.Config{InsecureSkipVerify: false, MinVersion: tls.VersionTLS12},
			DisableKeepAlives: true,
		}
		probeURL = strings.TrimRight(proxyRaw, "/")
		if parsedProxy.User != nil {
			u := *parsedProxy
			u.User = nil
			probeURL = strings.TrimRight(u.String(), "/")
		}
		probeGatewayCF = true
	} else if jenisRelay == security.RelayDeno {
		// Deno Deploy Relay adalah reverse proxy relay.
		transport = &http.Transport{
			DialContext:       security.GuardedDialContext(ssrfPolicy, dialer),
			TLSClientConfig:   &tls.Config{InsecureSkipVerify: false, MinVersion: tls.VersionTLS12},
			DisableKeepAlives: true,
		}
		probeURL = "https://" + parsedProxy.Host + "/cdn-cgi/trace"
		extraHeaders = map[string]string{"x-target-host": "cloudflare.com"}
		if parsedProxy.User != nil {
			if token, ok := parsedProxy.User.Password(); ok && token != "" {
				extraHeaders["x-relay-token"] = token
			}
		}
	} else {
		transport = &http.Transport{
			Proxy:             http.ProxyURL(parsedProxy),
			DialContext:       security.GuardedDialContext(ssrfPolicy, dialer),
			TLSClientConfig:   &tls.Config{InsecureSkipVerify: false, MinVersion: tls.VersionTLS12},
			DisableKeepAlives: true,
		}
		probeURL = "https://cloudflare.com/cdn-cgi/trace"
	}

	client := &http.Client{
		Transport: transport,
		Timeout:   7 * time.Second,
	}

	start := time.Now()
	probeReq, err := http.NewRequestWithContext(ctx, http.MethodGet, probeURL, nil)
	if err != nil {
		httpx.InternalError(w, r)
		return
	}
	probeReq.Header.Set("User-Agent", "Route-X-Probe/1.0")
	for k, v := range extraHeaders {
		probeReq.Header.Set(k, v)
	}

	resp, err := client.Do(probeReq)
	duration := int(time.Since(start).Milliseconds())

	now := time.Now().UTC()
	if err != nil {
		_ = h.egressRepo.RecordHealth(ctx, id, "unhealthy", nil)
		// B-272: hasil probe mengubah state pool (RecordHealth) — catat
		// auditnya seperti testWebhookPing mencatat test_ping.
		h.writeAudit(ctx, r, "test", "egress_pool", id, map[string]any{"status": "unhealthy"})
		_ = h.respond(w, r, http.StatusOK, EgressProbeResponseDTO{
			Status:    "unhealthy",
			Success:   false,
			CheckedAt: now,
			Message:   fmt.Sprintf("Gagal terhubung melalui proxy %s: %v", ep.Name, err),
			Error:     err.Error(),
		})
		return
	}
	defer resp.Body.Close()

	if probeGatewayCF {
		// Gateway milik pool menjawab: jalurnya sehat apa pun status
		// aplikasinya (gateway tanpa kredensial memang menjawab 401/404).
		if err := h.egressRepo.RecordHealth(ctx, id, "healthy", &duration); err != nil {
			h.logger.ErrorContext(ctx, "gagal mencatat health egress (healthy)",
				slog.String("id", id), slog.String("error", err.Error()))
		}
		h.writeAudit(ctx, r, "test", "egress_pool", id, map[string]any{"status": "healthy"})
		_ = h.respond(w, r, http.StatusOK, EgressProbeResponseDTO{
			Status:    "healthy",
			Success:   true,
			LatencyMS: &duration,
			CheckedAt: now,
			Message:   fmt.Sprintf("Gateway Cloudflare milik pool merespons HTTP %d (Latensi: %dms)", resp.StatusCode, duration),
		})
		return
	}

	if resp.StatusCode != http.StatusOK {
		// AUDIT FIX: kegagalan pencatatan health tidak lagi senyap.
		if err := h.egressRepo.RecordHealth(ctx, id, "unhealthy", nil); err != nil {
			h.logger.ErrorContext(ctx, "gagal mencatat health egress (unhealthy)",
				slog.String("id", id), slog.String("error", err.Error()))
		}
		h.writeAudit(ctx, r, "test", "egress_pool", id, map[string]any{"status": "unhealthy"})
		_ = h.respond(w, r, http.StatusOK, EgressProbeResponseDTO{
			Status:    "unhealthy",
			Success:   false,
			CheckedAt: now,
			Message:   fmt.Sprintf("Proxy merespons dengan kode status HTTP %d", resp.StatusCode),
			Error:     fmt.Sprintf("HTTP status %d", resp.StatusCode),
		})
		return
	}

	// AUDIT FIX: error baca body probe tidak lagi diabaikan — body terpotong
	// membuat exit IP kosong dilaporkan sebagai probe sehat tanpa sinyal apa pun.
	body, readErr := io.ReadAll(io.LimitReader(resp.Body, 4096))
	if readErr != nil {
		h.logger.WarnContext(ctx, "gagal membaca body probe egress",
			slog.String("id", id), slog.String("error", readErr.Error()))
	}
	lines := strings.Split(string(body), "\n")
	var exitIP, loc, colo string
	for _, l := range lines {
		parts := strings.SplitN(l, "=", 2)
		if len(parts) == 2 {
			k := strings.TrimSpace(parts[0])
			v := strings.TrimSpace(parts[1])
			switch k {
			case "ip":
				exitIP = v
			case "loc":
				loc = v
			case "colo":
				colo = v
			}
		}
	}

	// AUDIT FIX: kegagalan pencatatan health "healthy" juga tidak boleh senyap.
	if err := h.egressRepo.RecordHealth(ctx, id, "healthy", &duration); err != nil {
		h.logger.ErrorContext(ctx, "gagal mencatat health egress (healthy)",
			slog.String("id", id), slog.String("error", err.Error()))
	}
	h.writeAudit(ctx, r, "test", "egress_pool", id, map[string]any{"status": "healthy"})

	_ = h.respond(w, r, http.StatusOK, EgressProbeResponseDTO{
		Status:     "healthy",
		Success:    true,
		LatencyMS:  &duration,
		ExitIP:     exitIP,
		Country:    loc,
		Datacenter: colo,
		CheckedAt:  now,
		Message:    fmt.Sprintf("Koneksi berhasil (Latensi: %dms, IP Keluar: %s, Negara: %s)", duration, exitIP, loc),
	})
}
