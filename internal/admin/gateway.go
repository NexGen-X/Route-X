package admin

import (
	"net/http"
	"strconv"
	"time"

	"github.com/go-chi/chi/v5"

	"github.com/NexGen-X/Route-X/internal/auth"
	"github.com/NexGen-X/Route-X/internal/database/repo"
	"github.com/NexGen-X/Route-X/internal/database/repo/policy"
	"github.com/NexGen-X/Route-X/internal/database/repo/upstream"
	"github.com/NexGen-X/Route-X/internal/database/seed"
	"github.com/NexGen-X/Route-X/internal/httpx"
)

func (h *Handlers) gatewayRoutes(r chi.Router) {
	// 1. Routing Rules
	r.Route("/routing-rules", func(rr chi.Router) {
		rr.With(auth.RequirePermission(seed.PermRoutingRead)).Get("/", h.listRoutingRules)
		rr.With(auth.RequirePermission(seed.PermRoutingWrite)).Post("/", h.createRoutingRule)
		rr.With(auth.RequirePermission(seed.PermRoutingRead)).Get("/{id}", h.getRoutingRule)
		rr.With(auth.RequirePermission(seed.PermRoutingWrite)).Put("/{id}", h.updateRoutingRule)
		rr.With(auth.RequirePermission(seed.PermRoutingWrite)).Delete("/{id}", h.deleteRoutingRule)
		rr.With(auth.RequirePermission(seed.PermRoutingWrite)).Post("/{id}/toggle", h.toggleRoutingRule)
		rr.With(auth.RequirePermission(seed.PermRoutingWrite)).Post("/{id}/providers", h.setRuleProviders)
	})

	// 2. Rate Limits
	r.Route("/rate-limits", func(rl chi.Router) {
		rl.With(auth.RequirePermission(seed.PermSettingsRead)).Get("/", h.listRateLimits)
		rl.With(auth.RequirePermission(seed.PermRateLimitsWrite)).Post("/", h.createRateLimit)
		rl.With(auth.RequirePermission(seed.PermSettingsRead)).Get("/{id}", h.getRateLimit)
		rl.With(auth.RequirePermission(seed.PermRateLimitsWrite)).Put("/{id}", h.updateRateLimit)
		rl.With(auth.RequirePermission(seed.PermRateLimitsWrite)).Delete("/{id}", h.deleteRateLimit)
		rl.With(auth.RequirePermission(seed.PermRateLimitsWrite)).Post("/{id}/toggle", h.toggleRateLimit)
	})

	// 3. Budgets
	r.Route("/budgets", func(br chi.Router) {
		br.With(auth.RequirePermission(seed.PermUsageRead)).Get("/", h.listBudgets)
		br.With(auth.RequirePermission(seed.PermBudgetsWrite)).Post("/", h.createBudget)
		br.With(auth.RequirePermission(seed.PermUsageRead)).Get("/{id}", h.getBudget)
		br.With(auth.RequirePermission(seed.PermBudgetsWrite)).Put("/{id}", h.updateBudget)
		br.With(auth.RequirePermission(seed.PermBudgetsWrite)).Delete("/{id}", h.deleteBudget)
		br.With(auth.RequirePermission(seed.PermBudgetsWrite)).Post("/{id}/toggle", h.toggleBudget)
		br.With(auth.RequirePermission(seed.PermBudgetsWrite)).Post("/{id}/reset", h.resetBudget)
	})

	// 4. Bans
	r.Route("/bans", func(bnr chi.Router) {
		bnr.With(auth.RequirePermission(seed.PermSettingsRead)).Get("/", h.listBans)
		bnr.With(auth.RequirePermission(seed.PermBansWrite)).Post("/", h.createBan)
		bnr.With(auth.RequirePermission(seed.PermBansWrite)).Post("/{id}/lift", h.liftBan)
	})

	// 5. Content Filters
	r.Route("/content-filters", func(cfr chi.Router) {
		cfr.With(auth.RequirePermission(seed.PermSettingsRead)).Get("/", h.listContentFilters)
		cfr.With(auth.RequirePermission(seed.PermFiltersWrite)).Post("/", h.createContentFilter)
		cfr.With(auth.RequirePermission(seed.PermSettingsRead)).Get("/{id}", h.getContentFilter)
		cfr.With(auth.RequirePermission(seed.PermFiltersWrite)).Put("/{id}", h.updateContentFilter)
		cfr.With(auth.RequirePermission(seed.PermFiltersWrite)).Delete("/{id}", h.deleteContentFilter)
		cfr.With(auth.RequirePermission(seed.PermFiltersWrite)).Post("/{id}/toggle", h.toggleContentFilter)
	})

	// 6. Circuit Breakers
	r.Route("/circuit-breakers", func(cbr chi.Router) {
		cbr.With(auth.RequirePermission(seed.PermHealthRead)).Get("/", h.listCircuitBreakers)
		cbr.With(auth.RequirePermission(seed.PermProvidersWrite)).Post("/reset", h.resetCircuitBreaker)
	})
}

// -----------------------------------------------------------------------------
// Routing Rules Handlers
// -----------------------------------------------------------------------------

func (h *Handlers) listRoutingRules(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	limit := repo.DefaultPageLimit
	if s := r.URL.Query().Get("limit"); s != "" {
		if n, err := strconv.Atoi(s); err == nil && n > 0 {
			limit = n
		}
	}

	filter := upstream.RoutingRuleFilter{
		Search: r.URL.Query().Get("search"),
	}
	if s := r.URL.Query().Get("enabled"); s != "" {
		en := s == "true"
		filter.Enabled = &en
	}

	page := repo.Page{
		Limit:  limit,
		Cursor: r.URL.Query().Get("cursor"),
	}

	items, next, err := h.routingRepo.List(ctx, filter, page)
	if err != nil {
		mapRepoError(w, r, err, "aturan routing")
		return
	}

	_ = httpx.JSON(w, http.StatusOK, map[string]any{
		"items":       items,
		"next_cursor": next,
	})
}

func (h *Handlers) getRoutingRule(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	id := chi.URLParam(r, "id")

	rule, err := h.routingRepo.Get(ctx, id)
	if err != nil {
		mapRepoError(w, r, err, "aturan routing")
		return
	}

	_ = httpx.JSON(w, http.StatusOK, rule)
}

type createRoutingRuleReq struct {
	Name              string         `json:"name"`
	Description       string         `json:"description"`
	Strategy          string         `json:"strategy"`
	Priority          int            `json:"priority"`
	MatchModelID      *string        `json:"match_model_id"`
	MatchAPIKeyID     *string        `json:"match_api_key_id"`
	MatchCapabilities []string       `json:"match_capabilities"`
	MaxAttempts       *int           `json:"max_attempts"`
	BackoffMS         *int           `json:"backoff_ms"`
	FailureThreshold  *int           `json:"failure_threshold"`
	OpenDurationMS    *int           `json:"open_duration_ms"`
	HalfOpenProbes    *int           `json:"half_open_probes"`
	Enabled           *bool          `json:"enabled"`
	ProviderIDs       []string       `json:"provider_ids"`
	Weights           map[string]int `json:"weights"`
}

func (h *Handlers) createRoutingRule(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	var req createRoutingRuleReq
	if err := decodeJSON(r, &req); err != nil {
		httpx.BadRequest(w, r, "invalid_json", err.Error())
		return
	}

	var desc *string
	if req.Description != "" {
		desc = &req.Description
	}
	var prio *int
	if req.Priority > 0 {
		prio = &req.Priority
	}

	var actorID *string
	if p, ok := auth.PrincipalFrom(ctx); ok && p != nil {
		actorID = &p.User.ID
	}

	rule, err := h.routingRepo.Create(ctx, upstream.CreateRoutingRuleParams{
		Name:              req.Name,
		Description:       desc,
		Priority:          prio,
		MatchModelID:      req.MatchModelID,
		MatchAPIKeyID:     req.MatchAPIKeyID,
		MatchCapabilities: req.MatchCapabilities,
		Strategy:          req.Strategy,
		MaxAttempts:       req.MaxAttempts,
		BackoffMS:         req.BackoffMS,
		FailureThreshold:  req.FailureThreshold,
		OpenDurationMS:    req.OpenDurationMS,
		HalfOpenProbes:    req.HalfOpenProbes,
		Enabled:           req.Enabled,
		CreatedBy:         actorID,
	})
	if err != nil {
		mapRepoError(w, r, err, "aturan routing")
		return
	}

	if len(req.ProviderIDs) > 0 {
		_ = h.routingRepo.SetProviders(ctx, rule.ID, req.ProviderIDs, req.Weights)
		rule, _ = h.routingRepo.Get(ctx, rule.ID)
	}

	h.writeAudit(ctx, r, "create", "routing_rule", rule.ID, map[string]any{"name": rule.Name, "strategy": rule.Strategy})
	_ = httpx.JSON(w, http.StatusCreated, rule)
}

func (h *Handlers) updateRoutingRule(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	id := chi.URLParam(r, "id")

	var req createRoutingRuleReq
	if err := decodeJSON(r, &req); err != nil {
		httpx.BadRequest(w, r, "invalid_json", err.Error())
		return
	}

	var p upstream.UpdateRoutingRuleParams
	if req.Name != "" {
		p.Name = &req.Name
	}
	if req.Description != "" {
		p.Description = upstream.Set(req.Description)
	}
	if req.Priority > 0 {
		p.Priority = &req.Priority
	}
	if req.Strategy != "" {
		p.Strategy = &req.Strategy
	}
	if req.MatchModelID != nil {
		p.MatchModelID = upstream.Set(*req.MatchModelID)
	}
	if req.MatchAPIKeyID != nil {
		p.MatchAPIKeyID = upstream.Set(*req.MatchAPIKeyID)
	}
	p.MatchCapabilities = req.MatchCapabilities
	p.MaxAttempts = req.MaxAttempts
	p.BackoffMS = req.BackoffMS
	p.FailureThreshold = req.FailureThreshold
	p.OpenDurationMS = req.OpenDurationMS
	p.HalfOpenProbes = req.HalfOpenProbes
	p.Enabled = req.Enabled

	rule, err := h.routingRepo.Update(ctx, id, p)
	if err != nil {
		mapRepoError(w, r, err, "aturan routing")
		return
	}

	if req.ProviderIDs != nil {
		_ = h.routingRepo.SetProviders(ctx, id, req.ProviderIDs, req.Weights)
		rule, _ = h.routingRepo.Get(ctx, id)
	}

	h.writeAudit(ctx, r, "update", "routing_rule", id, map[string]any{"name": rule.Name})
	_ = httpx.JSON(w, http.StatusOK, rule)
}

func (h *Handlers) deleteRoutingRule(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	id := chi.URLParam(r, "id")

	if err := h.routingRepo.Delete(ctx, id); err != nil {
		mapRepoError(w, r, err, "aturan routing")
		return
	}

	h.writeAudit(ctx, r, "delete", "routing_rule", id, nil)
	_ = httpx.JSON(w, http.StatusOK, map[string]string{"status": "deleted"})
}

func (h *Handlers) toggleRoutingRule(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	id := chi.URLParam(r, "id")

	var req struct {
		Enabled bool `json:"enabled"`
	}
	if err := decodeJSON(r, &req); err != nil {
		httpx.BadRequest(w, r, "invalid_json", err.Error())
		return
	}

	rule, err := h.routingRepo.SetEnabled(ctx, id, req.Enabled)
	if err != nil {
		mapRepoError(w, r, err, "aturan routing")
		return
	}

	h.writeAudit(ctx, r, "toggle", "routing_rule", id, map[string]any{"enabled": req.Enabled})
	_ = httpx.JSON(w, http.StatusOK, rule)
}

func (h *Handlers) setRuleProviders(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	id := chi.URLParam(r, "id")

	var req struct {
		ProviderIDs []string       `json:"provider_ids"`
		Weights     map[string]int `json:"weights"`
	}
	if err := decodeJSON(r, &req); err != nil {
		httpx.BadRequest(w, r, "invalid_json", err.Error())
		return
	}

	if err := h.routingRepo.SetProviders(ctx, id, req.ProviderIDs, req.Weights); err != nil {
		mapRepoError(w, r, err, "provider aturan routing")
		return
	}

	rule, _ := h.routingRepo.Get(ctx, id)
	h.writeAudit(ctx, r, "set_providers", "routing_rule", id, map[string]any{"providers": req.ProviderIDs})
	_ = httpx.JSON(w, http.StatusOK, rule)
}

// -----------------------------------------------------------------------------
// Rate Limits Handlers
// -----------------------------------------------------------------------------

func (h *Handlers) listRateLimits(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	page := repo.Page{
		Limit:  repo.DefaultPageLimit,
		Cursor: r.URL.Query().Get("cursor"),
	}

	items, next, err := h.policyRepo.ListRateLimits(ctx, page)
	if err != nil {
		mapRepoError(w, r, err, "batas laju")
		return
	}

	_ = httpx.JSON(w, http.StatusOK, map[string]any{
		"items":       items,
		"next_cursor": next,
	})
}

func (h *Handlers) getRateLimit(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	id := chi.URLParam(r, "id")

	l, err := h.policyRepo.GetRateLimit(ctx, id)
	if err != nil {
		mapRepoError(w, r, err, "batas laju")
		return
	}

	_ = httpx.JSON(w, http.StatusOK, l)
}

type createRateLimitReq struct {
	Scope               string `json:"scope"`
	ScopeID             string `json:"scope_id"`
	RequestsPerSecond   *int   `json:"requests_per_second"`
	RequestsPerMinute   *int   `json:"requests_per_minute"`
	TokensPerMinute     *int   `json:"tokens_per_minute"`
	DailyRequestLimit   *int64 `json:"daily_request_limit"`
	MonthlyRequestLimit *int64 `json:"monthly_request_limit"`
	DailyTokenLimit     *int64 `json:"daily_token_limit"`
	MonthlyTokenLimit   *int64 `json:"monthly_token_limit"`
}

func (h *Handlers) createRateLimit(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	var req createRateLimitReq
	if err := decodeJSON(r, &req); err != nil {
		httpx.BadRequest(w, r, "invalid_json", err.Error())
		return
	}

	var actorID string
	if p, ok := auth.PrincipalFrom(ctx); ok && p != nil {
		actorID = p.User.ID
	}

	l, err := h.policyRepo.CreateRateLimit(ctx, policy.CreateRateLimitParams{
		Scope:               req.Scope,
		ScopeID:             req.ScopeID,
		RequestsPerSecond:   req.RequestsPerSecond,
		RequestsPerMinute:   req.RequestsPerMinute,
		TokensPerMinute:     req.TokensPerMinute,
		DailyRequestLimit:   req.DailyRequestLimit,
		MonthlyRequestLimit: req.MonthlyRequestLimit,
		DailyTokenLimit:     req.DailyTokenLimit,
		MonthlyTokenLimit:   req.MonthlyTokenLimit,
		CreatedBy:           actorID,
	})
	if err != nil {
		mapRepoError(w, r, err, "batas laju")
		return
	}

	h.writeAudit(ctx, r, "create", "rate_limit", l.ID, map[string]any{"scope": l.Scope, "scope_id": l.ScopeID})
	_ = httpx.JSON(w, http.StatusCreated, l)
}

func (h *Handlers) updateRateLimit(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	id := chi.URLParam(r, "id")

	var req struct {
		RequestsPerSecond   *int   `json:"requests_per_second"`
		RequestsPerMinute   *int   `json:"requests_per_minute"`
		TokensPerMinute     *int   `json:"tokens_per_minute"`
		DailyRequestLimit   *int64 `json:"daily_request_limit"`
		MonthlyRequestLimit *int64 `json:"monthly_request_limit"`
		DailyTokenLimit     *int64 `json:"daily_token_limit"`
		MonthlyTokenLimit   *int64 `json:"monthly_token_limit"`
		Enabled             *bool  `json:"enabled"`
	}
	if err := decodeJSON(r, &req); err != nil {
		httpx.BadRequest(w, r, "invalid_json", err.Error())
		return
	}

	l, err := h.policyRepo.UpdateRateLimit(ctx, id, policy.UpdateRateLimitParams{
		RequestsPerSecond:   req.RequestsPerSecond,
		RequestsPerMinute:   req.RequestsPerMinute,
		TokensPerMinute:     req.TokensPerMinute,
		DailyRequestLimit:   req.DailyRequestLimit,
		MonthlyRequestLimit: req.MonthlyRequestLimit,
		DailyTokenLimit:     req.DailyTokenLimit,
		MonthlyTokenLimit:   req.MonthlyTokenLimit,
		Enabled:             req.Enabled,
	})
	if err != nil {
		mapRepoError(w, r, err, "batas laju")
		return
	}

	h.writeAudit(ctx, r, "update", "rate_limit", id, nil)
	_ = httpx.JSON(w, http.StatusOK, l)
}

func (h *Handlers) deleteRateLimit(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	id := chi.URLParam(r, "id")

	if err := h.policyRepo.DeleteRateLimit(ctx, id); err != nil {
		mapRepoError(w, r, err, "batas laju")
		return
	}

	h.writeAudit(ctx, r, "delete", "rate_limit", id, nil)
	_ = httpx.JSON(w, http.StatusOK, map[string]string{"status": "deleted"})
}

func (h *Handlers) toggleRateLimit(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	id := chi.URLParam(r, "id")

	var req struct {
		Enabled bool `json:"enabled"`
	}
	if err := decodeJSON(r, &req); err != nil {
		httpx.BadRequest(w, r, "invalid_json", err.Error())
		return
	}

	l, err := h.policyRepo.SetRateLimitEnabled(ctx, id, req.Enabled)
	if err != nil {
		mapRepoError(w, r, err, "batas laju")
		return
	}

	h.writeAudit(ctx, r, "toggle", "rate_limit", id, map[string]any{"enabled": req.Enabled})
	_ = httpx.JSON(w, http.StatusOK, l)
}

// -----------------------------------------------------------------------------
// Budgets Handlers
// -----------------------------------------------------------------------------

func (h *Handlers) listBudgets(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	page := repo.Page{
		Limit:  repo.DefaultPageLimit,
		Cursor: r.URL.Query().Get("cursor"),
	}

	items, next, err := h.policyRepo.ListBudgets(ctx, page)
	if err != nil {
		mapRepoError(w, r, err, "anggaran")
		return
	}

	_ = httpx.JSON(w, http.StatusOK, map[string]any{
		"items":       items,
		"next_cursor": next,
	})
}

func (h *Handlers) getBudget(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	id := chi.URLParam(r, "id")

	b, err := h.policyRepo.GetBudget(ctx, id)
	if err != nil {
		mapRepoError(w, r, err, "anggaran")
		return
	}

	_ = httpx.JSON(w, http.StatusOK, b)
}

type createBudgetReq struct {
	Name              string `json:"name"`
	Scope             string `json:"scope"`
	ScopeID           string `json:"scope_id"`
	Period            string `json:"period"`
	LimitUSD          string `json:"limit_usd"`
	ActionOnExceed    string `json:"action_on_exceed"`
	AlertThresholdPct int    `json:"alert_threshold_pct"`
}

func (h *Handlers) createBudget(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	var req createBudgetReq
	if err := decodeJSON(r, &req); err != nil {
		httpx.BadRequest(w, r, "invalid_json", err.Error())
		return
	}

	limitUSD, err := upstream.ParseUSD(req.LimitUSD)
	if err != nil {
		httpx.BadRequest(w, r, "invalid_amount", "limit_usd tidak sah: "+err.Error())
		return
	}

	var actorID string
	if p, ok := auth.PrincipalFrom(ctx); ok && p != nil {
		actorID = p.User.ID
	}

	b, err := h.policyRepo.CreateBudget(ctx, policy.CreateBudgetParams{
		Name:              req.Name,
		Scope:             req.Scope,
		ScopeID:           req.ScopeID,
		Period:            req.Period,
		LimitUSD:          limitUSD,
		ActionOnExceed:    req.ActionOnExceed,
		AlertThresholdPct: req.AlertThresholdPct,
		CreatedBy:         actorID,
	})
	if err != nil {
		mapRepoError(w, r, err, "anggaran")
		return
	}

	h.writeAudit(ctx, r, "create", "budget", b.ID, map[string]any{"name": b.Name, "limit_usd": limitUSD.String()})
	_ = httpx.JSON(w, http.StatusCreated, b)
}

func (h *Handlers) updateBudget(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	id := chi.URLParam(r, "id")

	var req struct {
		Name              *string `json:"name"`
		LimitUSD          *string `json:"limit_usd"`
		ActionOnExceed    *string `json:"action_on_exceed"`
		AlertThresholdPct *int    `json:"alert_threshold_pct"`
		Enabled           *bool   `json:"enabled"`
	}
	if err := decodeJSON(r, &req); err != nil {
		httpx.BadRequest(w, r, "invalid_json", err.Error())
		return
	}

	var lim *upstream.USD
	if req.LimitUSD != nil {
		parsed, err := upstream.ParseUSD(*req.LimitUSD)
		if err != nil {
			httpx.BadRequest(w, r, "invalid_amount", "limit_usd tidak sah: "+err.Error())
			return
		}
		lim = &parsed
	}

	b, err := h.policyRepo.UpdateBudget(ctx, id, policy.UpdateBudgetParams{
		Name:              req.Name,
		LimitUSD:          lim,
		ActionOnExceed:    req.ActionOnExceed,
		AlertThresholdPct: req.AlertThresholdPct,
		Enabled:           req.Enabled,
	})
	if err != nil {
		mapRepoError(w, r, err, "anggaran")
		return
	}

	h.writeAudit(ctx, r, "update", "budget", id, map[string]any{"name": b.Name})
	_ = httpx.JSON(w, http.StatusOK, b)
}

func (h *Handlers) deleteBudget(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	id := chi.URLParam(r, "id")

	if err := h.policyRepo.DeleteBudget(ctx, id); err != nil {
		mapRepoError(w, r, err, "anggaran")
		return
	}

	h.writeAudit(ctx, r, "delete", "budget", id, nil)
	_ = httpx.JSON(w, http.StatusOK, map[string]string{"status": "deleted"})
}

func (h *Handlers) toggleBudget(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	id := chi.URLParam(r, "id")

	var req struct {
		Enabled bool `json:"enabled"`
	}
	if err := decodeJSON(r, &req); err != nil {
		httpx.BadRequest(w, r, "invalid_json", err.Error())
		return
	}

	b, err := h.policyRepo.SetBudgetEnabled(ctx, id, req.Enabled)
	if err != nil {
		mapRepoError(w, r, err, "anggaran")
		return
	}

	h.writeAudit(ctx, r, "toggle", "budget", id, map[string]any{"enabled": req.Enabled})
	_ = httpx.JSON(w, http.StatusOK, b)
}

func (h *Handlers) resetBudget(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	id := chi.URLParam(r, "id")

	now := time.Now().UTC()
	b, err := h.policyRepo.ResetPeriod(ctx, id, now, nil)
	if err != nil {
		mapRepoError(w, r, err, "reset anggaran")
		return
	}

	h.writeAudit(ctx, r, "reset", "budget", id, nil)
	_ = httpx.JSON(w, http.StatusOK, b)
}

// -----------------------------------------------------------------------------
// Bans Handlers
// -----------------------------------------------------------------------------

func (h *Handlers) listBans(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	page := repo.Page{
		Limit: repo.DefaultPageLimit,
	}

	items, err := h.policyRepo.ActiveBans(ctx, page)
	if err != nil {
		mapRepoError(w, r, err, "pemblokiran")
		return
	}

	_ = httpx.JSON(w, http.StatusOK, map[string]any{
		"items": items,
	})
}

type createBanReq struct {
	SubjectKind string  `json:"subject_kind"`
	Subject     string  `json:"subject"`
	Reason      string  `json:"reason"`
	ExpiresAt   *string `json:"expires_at"`
}

func (h *Handlers) createBan(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	var req createBanReq
	if err := decodeJSON(r, &req); err != nil {
		httpx.BadRequest(w, r, "invalid_json", err.Error())
		return
	}

	var exp *time.Time
	if req.ExpiresAt != nil && *req.ExpiresAt != "" {
		if t, err := time.Parse(time.RFC3339, *req.ExpiresAt); err == nil {
			exp = &t
		}
	}

	var actorID string
	if p, ok := auth.PrincipalFrom(ctx); ok && p != nil {
		actorID = p.User.ID
	}

	ban, err := h.policyRepo.CreateBan(ctx, policy.CreateBanParams{
		SubjectKind: req.SubjectKind,
		Subject:     req.Subject,
		Reason:      req.Reason,
		ExpiresAt:   exp,
		CreatedBy:   actorID,
	})
	if err != nil {
		mapRepoError(w, r, err, "pemblokiran")
		return
	}

	h.writeAudit(ctx, r, "create", "ban", ban.ID, map[string]any{"kind": ban.SubjectKind, "subject": ban.Subject})
	_ = httpx.JSON(w, http.StatusCreated, ban)
}

func (h *Handlers) liftBan(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	id := chi.URLParam(r, "id")

	var actorID string
	if p, ok := auth.PrincipalFrom(ctx); ok && p != nil {
		actorID = p.User.ID
	}

	ban, err := h.policyRepo.Lift(ctx, id, actorID)
	if err != nil {
		mapRepoError(w, r, err, "pemblokiran")
		return
	}

	h.writeAudit(ctx, r, "lift", "ban", id, nil)
	_ = httpx.JSON(w, http.StatusOK, ban)
}

// -----------------------------------------------------------------------------
// Content Filters Handlers
// -----------------------------------------------------------------------------

func (h *Handlers) listContentFilters(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	page := repo.Page{
		Limit:  repo.DefaultPageLimit,
		Cursor: r.URL.Query().Get("cursor"),
	}

	items, next, err := h.policyRepo.ListFilters(ctx, page)
	if err != nil {
		mapRepoError(w, r, err, "filter konten")
		return
	}

	_ = httpx.JSON(w, http.StatusOK, map[string]any{
		"items":       items,
		"next_cursor": next,
	})
}

func (h *Handlers) getContentFilter(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	id := chi.URLParam(r, "id")

	f, err := h.policyRepo.GetFilter(ctx, id)
	if err != nil {
		mapRepoError(w, r, err, "filter konten")
		return
	}

	_ = httpx.JSON(w, http.StatusOK, f)
}

type createFilterReq struct {
	Name            string `json:"name"`
	Description     string `json:"description"`
	Kind            string `json:"kind"`
	Priority        int    `json:"priority"`
	AppliesTo       string `json:"applies_to"`
	Action          string `json:"action"`
	Pattern         string `json:"pattern"`
	PatternType     string `json:"pattern_type"`
	CaseSensitive   bool   `json:"case_sensitive"`
	MaxEvalMS       int    `json:"max_eval_ms"`
	ModelID         string `json:"model_id"`
	ProviderID      string `json:"provider_id"`
	MaxRequestBytes *int64 `json:"max_request_bytes"`
	Enabled         *bool  `json:"enabled"`
}

func (h *Handlers) createContentFilter(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	var req createFilterReq
	if err := decodeJSON(r, &req); err != nil {
		httpx.BadRequest(w, r, "invalid_json", err.Error())
		return
	}

	var actorID string
	if p, ok := auth.PrincipalFrom(ctx); ok && p != nil {
		actorID = p.User.ID
	}

	f, err := h.policyRepo.CreateFilter(ctx, policy.CreateFilterParams{
		Name:            req.Name,
		Description:     req.Description,
		Kind:            req.Kind,
		Priority:        req.Priority,
		AppliesTo:       req.AppliesTo,
		Action:          req.Action,
		Pattern:         req.Pattern,
		PatternType:     req.PatternType,
		CaseSensitive:   req.CaseSensitive,
		MaxEvalMS:       req.MaxEvalMS,
		ModelID:         req.ModelID,
		ProviderID:      req.ProviderID,
		MaxRequestBytes: req.MaxRequestBytes,
		CreatedBy:       actorID,
	})
	if err != nil {
		mapRepoError(w, r, err, "filter konten")
		return
	}

	h.writeAudit(ctx, r, "create", "content_filter", f.ID, map[string]any{"name": f.Name, "kind": f.Kind})
	_ = httpx.JSON(w, http.StatusCreated, f)
}

func (h *Handlers) updateContentFilter(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	id := chi.URLParam(r, "id")

	var req createFilterReq
	if err := decodeJSON(r, &req); err != nil {
		httpx.BadRequest(w, r, "invalid_json", err.Error())
		return
	}

	var p policy.UpdateFilterParams
	if req.Name != "" {
		p.Name = &req.Name
	}
	if req.Description != "" {
		p.Description = &req.Description
	}
	if req.Priority > 0 {
		p.Priority = &req.Priority
	}
	if req.AppliesTo != "" {
		p.AppliesTo = &req.AppliesTo
	}
	if req.Action != "" {
		p.Action = &req.Action
	}
	if req.Pattern != "" {
		p.Pattern = &req.Pattern
	}
	if req.PatternType != "" {
		p.PatternType = &req.PatternType
	}
	p.CaseSensitive = &req.CaseSensitive
	if req.MaxEvalMS > 0 {
		p.MaxEvalMS = &req.MaxEvalMS
	}
	p.Enabled = req.Enabled

	f, err := h.policyRepo.UpdateFilter(ctx, id, p)
	if err != nil {
		mapRepoError(w, r, err, "filter konten")
		return
	}

	h.writeAudit(ctx, r, "update", "content_filter", id, map[string]any{"name": f.Name})
	_ = httpx.JSON(w, http.StatusOK, f)
}

func (h *Handlers) deleteContentFilter(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	id := chi.URLParam(r, "id")

	if err := h.policyRepo.DeleteFilter(ctx, id); err != nil {
		mapRepoError(w, r, err, "filter konten")
		return
	}

	h.writeAudit(ctx, r, "delete", "content_filter", id, nil)
	_ = httpx.JSON(w, http.StatusOK, map[string]string{"status": "deleted"})
}

func (h *Handlers) toggleContentFilter(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	id := chi.URLParam(r, "id")

	var req struct {
		Enabled bool `json:"enabled"`
	}
	if err := decodeJSON(r, &req); err != nil {
		httpx.BadRequest(w, r, "invalid_json", err.Error())
		return
	}

	f, err := h.policyRepo.SetFilterEnabled(ctx, id, req.Enabled)
	if err != nil {
		mapRepoError(w, r, err, "filter konten")
		return
	}

	h.writeAudit(ctx, r, "toggle", "content_filter", id, map[string]any{"enabled": req.Enabled})
	_ = httpx.JSON(w, http.StatusOK, f)
}

// -----------------------------------------------------------------------------
// Circuit Breakers Handlers
// -----------------------------------------------------------------------------

func (h *Handlers) listCircuitBreakers(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	type BreakerInfo struct {
		Key   string `json:"key"`
		State string `json:"state"`
	}

	var items []BreakerInfo
	if h.redis != nil {
		iter := h.redis.Scan(ctx, 0, "routex:breaker:*", 500).Iterator()
		for iter.Next(ctx) {
			k := iter.Val()
			val, _ := h.redis.Get(ctx, k).Result()
			items = append(items, BreakerInfo{
				Key:   k,
				State: val,
			})
		}
	}

	_ = httpx.JSON(w, http.StatusOK, map[string]any{
		"items": items,
	})
}

func (h *Handlers) resetCircuitBreaker(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	var req struct {
		Key string `json:"key"`
	}
	if err := decodeJSON(r, &req); err != nil {
		httpx.BadRequest(w, r, "invalid_json", err.Error())
		return
	}

	if h.redis != nil && req.Key != "" {
		_ = h.redis.Del(ctx, req.Key).Err()
	}

	h.writeAudit(ctx, r, "reset", "circuit_breaker", req.Key, nil)
	_ = httpx.JSON(w, http.StatusOK, map[string]string{"status": "reset"})
}
