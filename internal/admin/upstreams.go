package admin

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"strconv"
	"time"

	"github.com/go-chi/chi/v5"

	"github.com/NexGen-X/Route-X/internal/auth"
	"github.com/NexGen-X/Route-X/internal/database/repo"
	"github.com/NexGen-X/Route-X/internal/database/repo/upstream"
	"github.com/NexGen-X/Route-X/internal/database/seed"
	"github.com/NexGen-X/Route-X/internal/httpx"
	"github.com/NexGen-X/Route-X/internal/security"
	"github.com/NexGen-X/Route-X/internal/webhooks"
)

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

	p, err := h.providerRepo.Get(ctx, id)
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

	p, err := h.providerRepo.Create(ctx, params)
	if err != nil {
		mapRepoError(w, r, err, "provider")
		return
	}

	h.writeAudit(ctx, r, "create", "provider", p.ID, map[string]any{"name": p.Name})
	_ = h.respond(w, r, http.StatusCreated, toProviderDTO(p))
}

func (h *Handlers) updateProvider(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	id := chi.URLParam(r, "id")

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

	p, err := h.providerRepo.Update(ctx, id, params)
	if err != nil {
		mapRepoError(w, r, err, "provider")
		return
	}

	h.writeAudit(ctx, r, "update", "provider", p.ID, map[string]any{"name": p.Name})
	_ = h.respond(w, r, http.StatusOK, toProviderDTO(p))
}

func (h *Handlers) deleteProvider(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	id := chi.URLParam(r, "id")

	if err := h.providerRepo.Delete(ctx, id); err != nil {
		mapRepoError(w, r, err, "provider")
		return
	}

	h.writeAudit(ctx, r, "delete", "provider", id, nil)
	_ = h.respond(w, r, http.StatusOK, StatusResponse{Status: "deleted"})
}

func (h *Handlers) toggleProvider(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	id := chi.URLParam(r, "id")

	var req struct {
		Enabled bool `json:"enabled"`
	}
	if err := decodeJSON(r, &req); err != nil {
		httpx.BadRequest(w, r, "invalid_json", err.Error())
		return
	}

	p, err := h.providerRepo.SetEnabled(ctx, id, req.Enabled)
	if err != nil {
		mapRepoError(w, r, err, "provider")
		return
	}

	h.writeAudit(ctx, r, "toggle", "provider", id, map[string]any{"enabled": req.Enabled})
	_ = h.respond(w, r, http.StatusOK, toProviderDTO(p))
}

func (h *Handlers) listProviderHealthChecks(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	id := chi.URLParam(r, "id")

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

	p, err := h.providerRepo.Get(ctx, id)
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

	_ = h.respond(w, r, http.StatusOK, ProbeProviderResponse{
		Status:    status,
		LatencyMS: latMS,
		Error:     sanitizedErr,
	})
}

// -----------------------------------------------------------------------------
// Credentials Handlers
// -----------------------------------------------------------------------------

func (h *Handlers) listCredentials(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	id := chi.URLParam(r, "id")

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

	cred, err := h.credentialRepo.Create(ctx, upstream.CreateCredentialParams{
		ProviderID: providerID,
		Label:      req.Label,
		Secret:     security.Secret(req.APIKey),
		ExpiresAt:  exp,
		CreatedBy:  actorID,
	})
	if err != nil {
		mapRepoError(w, r, err, "kredensial")
		return
	}

	h.writeAudit(ctx, r, "create", "credential", cred.ID, map[string]any{"provider_id": providerID, "label": cred.Label})
	_ = h.respond(w, r, http.StatusCreated, toCredentialMetaDTO(cred))
}

func (h *Handlers) deleteCredential(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	credID := chi.URLParam(r, "cred_id")

	if err := h.credentialRepo.Delete(ctx, credID); err != nil {
		mapRepoError(w, r, err, "kredensial")
		return
	}

	h.writeAudit(ctx, r, "delete", "credential", credID, nil)
	_ = h.respond(w, r, http.StatusOK, StatusResponse{Status: "deleted"})
}

func (h *Handlers) toggleCredential(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	credID := chi.URLParam(r, "cred_id")

	var req struct {
		Enabled bool `json:"enabled"`
	}
	if err := decodeJSON(r, &req); err != nil {
		httpx.BadRequest(w, r, "invalid_json", err.Error())
		return
	}

	cred, err := h.credentialRepo.SetEnabled(ctx, credID, req.Enabled)
	if err != nil {
		mapRepoError(w, r, err, "kredensial")
		return
	}

	h.writeAudit(ctx, r, "toggle", "credential", credID, map[string]any{"enabled": req.Enabled})
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
