package admin

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"slices"
	"strconv"
	"strings"
	"time"

	"github.com/go-chi/chi/v5"

	"github.com/NexGen-X/Route-X/internal/auth"
	"github.com/NexGen-X/Route-X/internal/database/repo"
	"github.com/NexGen-X/Route-X/internal/database/repo/upstream"
	"github.com/NexGen-X/Route-X/internal/database/seed"
	"github.com/NexGen-X/Route-X/internal/httpx"
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
	if slices.Contains(principal.Roles, seed.RoleSuperAdmin) {
		return p, nil
	}
	if p.OwnerUserID == nil || *p.OwnerUserID != principal.User.ID {
		return nil, repo.ErrNotFound
	}
	return p, nil
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

		// Kredensial di bawah provider
		pr.With(auth.RequirePermission(seed.PermProvidersRead)).Get("/{id}/credentials", h.listCredentials)
		pr.With(auth.RequirePermission(seed.PermCredentialsWrite)).Post("/{id}/credentials", h.createCredential)
		pr.With(auth.RequirePermission(seed.PermCredentialsWrite)).Delete("/{id}/credentials/{cred_id}", h.deleteCredential)
		pr.With(auth.RequirePermission(seed.PermCredentialsWrite)).Post("/{id}/credentials/{cred_id}/toggle", h.toggleCredential)
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

		// Pricing
		mr.With(auth.RequirePermission(seed.PermModelsWrite)).Post("/mappings/{mapping_id}/pricing", h.setPricing)
		mr.With(auth.RequirePermission(seed.PermModelsRead)).Get("/mappings/{mapping_id}/pricing/history", h.listPricingHistory)
	})

	// 3. Egress Pools
	r.Route("/egress-pools", func(er chi.Router) {
		er.With(auth.RequirePermission(seed.PermProvidersRead)).Get("/", h.listEgressPools)
		er.With(auth.RequirePermission(seed.PermProvidersWrite)).Post("/", h.createEgressPool)
		er.With(auth.RequirePermission(seed.PermProvidersRead)).Get("/{id}", h.getEgressPool)
		er.With(auth.RequirePermission(seed.PermProvidersWrite)).Put("/{id}", h.updateEgressPool)
		er.With(auth.RequirePermission(seed.PermProvidersWrite)).Delete("/{id}", h.deleteEgressPool)
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

	// Isolasi BYOK: Pengguna non-superadmin hanya dapat melihat provider publik
	// atau provider BYOK miliknya sendiri.
	if p, ok := auth.PrincipalFrom(ctx); ok && p != nil {
		if !slices.Contains(p.Roles, seed.RoleSuperAdmin) {
			filter.AccessibleByUserID = p.User.ID
		}
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

func (h *Handlers) createProvider(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	var req createProviderReq
	if err := decodeJSON(r, &req); err != nil {
		httpx.BadRequest(w, r, "invalid_json", err.Error())
		return
	}

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
	if _, err := h.authorizeProviderAccess(ctx, id); err != nil {
		mapRepoError(w, r, err, "provider")
		return
	}

	var req createProviderReq
	if err := decodeJSON(r, &req); err != nil {
		httpx.BadRequest(w, r, "invalid_json", err.Error())
		return
	}

	var params upstream.UpdateProviderParams
	if req.Name != "" {
		params.Name = &req.Name
	}
	if req.DisplayName != "" {
		params.DisplayName = &req.DisplayName
	}
	if req.Kind != "" {
		params.Kind = &req.Kind
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
		params.EgressPoolID = upstream.Set(*req.EgressPoolID)
	}
	params.Metadata = req.Metadata

	var p *upstream.Provider
	err := repo.InTx(ctx, h.pool, func(q repo.Querier) error {
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

	var req struct {
		Enabled bool `json:"enabled"`
	}
	if err := decodeJSON(r, &req); err != nil {
		httpx.BadRequest(w, r, "invalid_json", err.Error())
		return
	}

	var p *upstream.Provider
	err := repo.InTx(ctx, h.pool, func(q repo.Querier) error {
		txRepo := h.providerRepo.WithQuerier(q)
		var err error
		p, err = txRepo.SetEnabled(ctx, id, req.Enabled)
		if err != nil {
			return err
		}
		return h.writeAuditTx(ctx, q, r, "toggle", "provider", id, map[string]any{"enabled": req.Enabled})
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
		httpx.BadRequest(w, r, "factory_unavailable", "pabrik provider tidak tersedia")
		return
	}

	adapter, err := h.factory.ProviderFor(ctx, p)
	if err != nil {
		httpx.BadRequest(w, r, "adapter_error", err.Error())
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

type openAIModelDiscoveryItem struct {
	ID string `json:"id"`
}

type openAIModelDiscoveryResponse struct {
	Data []openAIModelDiscoveryItem `json:"data"`
}

type ollamaModelDiscoveryItem struct {
	Name  string `json:"name"`
	Model string `json:"model"`
}

type ollamaModelDiscoveryResponse struct {
	Models []ollamaModelDiscoveryItem `json:"models"`
}

// syncProviderModels menarik katalog model langsung dari discovery endpoint upstream
// (/v1/models atau /api/tags untuk Ollama) dan mendaftarkannya ke database secara idempoten.
func (h *Handlers) syncProviderModels(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	id := chi.URLParam(r, "id")

	// 1. Otorisasi akses provider (termasuk proteksi multi-tenant BYOK)
	p, err := h.authorizeProviderAccess(ctx, id)
	if err != nil {
		mapRepoError(w, r, err, "provider")
		return
	}

	// 2. Ambil kredensial aktif jika ada dan dekripsi API key
	var apiKey string
	if activeCred, credErr := h.credentialRepo.Active(ctx, p.ID); credErr == nil && activeCred != nil {
		apiKey = activeCred.Secret.Reveal()
	}

	// 3. Siapkan HTTP client dengan kebijakan SSRF wajib
	ssrfPolicy := security.DefaultSSRFPolicy()
	if h.factory != nil {
		ssrfPolicy = h.factory.SSRFPolicy()
	}

	httpClient, err := providers.NewHTTPClient(providers.ClientConfig{
		Timeout:             15 * time.Second,
		SSRFPolicy:          ssrfPolicy,
		MaxIdleConnsPerHost: 2,
	}, false)
	if err != nil {
		httpx.BadRequest(w, r, "http_client_error", err.Error())
		return
	}

	// 4. Susun discovery URL berdasarkan dialek provider
	var discoveryURL string
	lowerKind := strings.ToLower(p.Kind)
	lowerName := strings.ToLower(p.Name)
	trimmedBase := strings.TrimRight(p.BaseURL, "/")

	if lowerKind == "ollama" || strings.Contains(lowerName, "ollama") {
		discoveryURL = trimmedBase + "/api/tags"
	} else if strings.HasSuffix(trimmedBase, "/v1") {
		discoveryURL = trimmedBase + "/models"
	} else {
		discoveryURL = trimmedBase + "/v1/models"
	}

	req, err := http.NewRequestWithContext(ctx, http.MethodGet, discoveryURL, nil)
	if err != nil {
		httpx.BadRequest(w, r, "invalid_url", err.Error())
		return
	}
	req.Header.Set("Accept", "application/json")
	req.Header.Set("User-Agent", "Route-X-Discovery/1.0")
	if apiKey != "" {
		if lowerKind == "anthropic" {
			req.Header.Set("x-api-key", apiKey)
			req.Header.Set("anthropic-version", "2023-06-01")
		} else {
			req.Header.Set("Authorization", "Bearer "+apiKey)
		}
	}

	resp, err := httpClient.Do(req)
	if err != nil {
		h.logger.ErrorContext(ctx, "panggilan discovery model gagal", "url", discoveryURL, "error", err)
		httpx.BadRequest(w, r, "upstream_unreachable", fmt.Sprintf("Gagal menghubungi upstream (%s): %v", discoveryURL, err))
		return
	}
	defer resp.Body.Close()

	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		bodySnippet, _ := io.ReadAll(io.LimitReader(resp.Body, 512))
		httpx.BadRequest(w, r, "upstream_error", fmt.Sprintf("Upstream mengembalikan status %d: %s", resp.StatusCode, string(bodySnippet)))
		return
	}

	bodyBytes, err := io.ReadAll(io.LimitReader(resp.Body, 5<<20)) // 5MB limit
	if err != nil {
		h.logger.ErrorContext(ctx, "gagal membaca respons discovery", "error", err)
		httpx.InternalError(w, r)
		return
	}

	// 5. Ekstraksi model IDs
	var rawIDs []string
	var oaiResp openAIModelDiscoveryResponse
	if err := json.Unmarshal(bodyBytes, &oaiResp); err == nil && len(oaiResp.Data) > 0 {
		for _, item := range oaiResp.Data {
			if trimmed := strings.TrimSpace(item.ID); trimmed != "" {
				rawIDs = append(rawIDs, trimmed)
			}
		}
	} else {
		var ollamaResp ollamaModelDiscoveryResponse
		if err := json.Unmarshal(bodyBytes, &ollamaResp); err == nil && len(ollamaResp.Models) > 0 {
			for _, item := range ollamaResp.Models {
				name := strings.TrimSpace(item.Name)
				if name == "" {
					name = strings.TrimSpace(item.Model)
				}
				if name != "" {
					rawIDs = append(rawIDs, name)
				}
			}
		}
	}

	if len(rawIDs) == 0 {
		httpx.BadRequest(w, r, "no_models_found", "Tidak ada model yang ditemukan dalam respons upstream.")
		return
	}

	slices.Sort(rawIDs)
	uniqueIDs := slices.Compact(rawIDs)

	// 6. Simpan model dan pemetaan ke database secara idempoten
	var syncedModels []string
	for _, mid := range uniqueIDs {
		// Pastikan model kanonik ada
		m, err := h.modelRepo.GetByModelID(ctx, mid)
		if errors.Is(err, repo.ErrNotFound) {
			m, err = h.modelRepo.Create(ctx, upstream.CreateModelParams{
				ModelID:      mid,
				DisplayName:  mid,
				Capabilities: []string{"text"},
			})
			if err != nil {
				// Bila balapan konkurensi, coba baca ulang
				m, _ = h.modelRepo.GetByModelID(ctx, mid)
			}
		}
		if m == nil {
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
		}
	}

	h.writeAudit(ctx, r, "sync_models", "provider", id, map[string]any{
		"provider_name": p.Name,
		"models_count":  len(syncedModels),
	})

	_ = h.respond(w, r, http.StatusOK, SyncModelsResponseDTO{
		Count:   len(syncedModels),
		Models:  syncedModels,
		Message: fmt.Sprintf("Berhasil menyinkronkan %d model dari provider %s", len(syncedModels), p.DisplayName),
	})
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
	Label     string  `json:"label"`
	APIKey    string  `json:"api_key"`
	ExpiresAt *string `json:"expires_at"`
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

	var actorID *string
	if p, ok := auth.PrincipalFrom(ctx); ok && p != nil {
		actorID = &p.User.ID
	}

	var cred *upstream.CredentialMeta
	err := repo.InTx(ctx, h.pool, func(q repo.Querier) error {
		txRepo := h.credentialRepo.WithQuerier(q)
		var err error
		cred, err = txRepo.Create(ctx, upstream.CreateCredentialParams{
			ProviderID: providerID,
			Label:      req.Label,
			Secret:     security.Secret(req.APIKey),
			ExpiresAt:  exp,
			CreatedBy:  actorID,
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

	var req struct {
		Enabled bool `json:"enabled"`
	}
	if err := decodeJSON(r, &req); err != nil {
		httpx.BadRequest(w, r, "invalid_json", err.Error())
		return
	}

	// 5.1 & 5.8: Modifikasi status kredensial bersyarat provider_id dan lakukan audit log di dalam transaksi
	var cred *upstream.CredentialMeta
	err := repo.InTx(ctx, h.pool, func(q repo.Querier) error {
		txRepo := h.credentialRepo.WithQuerier(q)
		var err error
		cred, err = txRepo.SetEnabledOfProvider(ctx, providerID, credID, req.Enabled)
		if err != nil {
			return err
		}
		return h.writeAuditTx(ctx, q, r, "toggle", "credential", credID, map[string]any{"provider_id": providerID, "enabled": req.Enabled})
	})
	if err != nil {
		mapRepoError(w, r, err, "kredensial")
		return
	}

	_ = h.respond(w, r, http.StatusOK, toCredentialMetaDTO(cred))
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

	models := make([]ModelDTO, 0, len(items))
	for _, m := range items {
		models = append(models, toModelDTO(m))
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

	aliases, _ := h.modelRepo.ListAliases(ctx, id)
	mappings, _ := h.modelRepo.ListProviderModels(ctx, upstream.ProviderModelFilter{ModelID: id})

	aliasesDTO := make([]ModelAliasDTO, 0, len(aliases))
	for _, a := range aliases {
		aliasesDTO = append(aliasesDTO, toModelAliasDTO(a))
	}

	mappingsDTO := make([]ModelMappingDTO, 0, len(mappings))
	for _, pm := range mappings {
		mappingsDTO = append(mappingsDTO, toModelMappingDTO(pm))
	}

	_ = h.respond(w, r, http.StatusOK, ModelDetailResponse{
		Model:    toModelDTO(m),
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

	if err := h.modelRepo.RemoveAlias(ctx, alias); err != nil {
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

	if err := h.modelRepo.DetachProvider(ctx, mappingID); err != nil {
		mapRepoError(w, r, err, "pemetaan provider model")
		return
	}

	h.writeAudit(ctx, r, "detach_provider", "provider_model", mappingID, nil)
	_ = h.respond(w, r, http.StatusOK, StatusResponse{Status: "deleted"})
}

type setPriceReq struct {
	InputPer1MUSD       string `json:"input_per_1m_usd"`
	OutputPer1MUSD      string `json:"output_per_1m_usd"`
	CachedInputPer1MUSD string `json:"cached_input_per_1m_usd"`
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
	cachedUSD, err := upstream.ParseUSD(req.CachedInputPer1MUSD)
	if err != nil {
		httpx.BadRequest(w, r, "invalid_price", "cached_input_per_1m_usd tidak valid: "+err.Error())
		return
	}

	var price *upstream.Price
	txErr := repo.InTx(ctx, h.pool, func(q repo.Querier) error {
		txRepo := upstream.NewPricingRepo(q)
		p, err := txRepo.Set(ctx, upstream.SetPriceParams{
			ProviderModelID: mappingID,
			Input:           inUSD,
			Output:          outUSD,
			CachedInput:     &cachedUSD,
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

	h.writeAudit(ctx, r, "set_pricing", "provider_model", mappingID, map[string]any{
		"input": inUSD.String(), "output": outUSD.String(), "cached": cachedUSD.String(),
	})
	_ = h.respond(w, r, http.StatusCreated, toPriceDTO(price))
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
	Name        string `json:"name"`
	Description string `json:"description"`
	ProxyURL    string `json:"proxy_url"`
	Enabled     *bool  `json:"enabled"`
}

func (h *Handlers) createEgressPool(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	var req createEgressReq
	if err := decodeJSON(r, &req); err != nil {
		httpx.BadRequest(w, r, "invalid_json", err.Error())
		return
	}
	if req.ProxyURL == "" {
		httpx.BadRequest(w, r, "missing_proxy_url", "proxy_url wajib diisi")
		return
	}

	ep, err := h.egressRepo.Create(ctx, upstream.CreateEgressParams{
		Name:     req.Name,
		Kind:     "http",
		ProxyURL: security.Secret(req.ProxyURL),
		Enabled:  req.Enabled,
	})
	if err != nil {
		mapRepoError(w, r, err, "egress pool")
		return
	}

	h.writeAudit(ctx, r, "create", "egress_pool", ep.ID, map[string]any{"name": ep.Name})
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

	var params upstream.UpdateEgressParams
	if req.Name != "" {
		params.Name = &req.Name
	}
	params.Enabled = req.Enabled

	ep, err := h.egressRepo.Update(ctx, id, params)
	if err != nil {
		mapRepoError(w, r, err, "egress pool")
		return
	}

	if req.ProxyURL != "" {
		ep, err = h.egressRepo.SetProxyURL(ctx, id, security.Secret(req.ProxyURL))
		if err != nil {
			mapRepoError(w, r, err, "egress pool proxy_url")
			return
		}
	}

	h.writeAudit(ctx, r, "update", "egress_pool", ep.ID, map[string]any{"name": ep.Name})
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
	_ = h.respond(w, r, http.StatusOK, StatusResponse{Status: "deleted"})
}
