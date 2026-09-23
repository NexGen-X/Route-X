package admin

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"net/url"
	"strconv"
	"strings"
	"time"

	"github.com/go-chi/chi/v5"

	"github.com/NexGen-X/Route-X/internal/auth"
	"github.com/NexGen-X/Route-X/internal/cache"
	"github.com/NexGen-X/Route-X/internal/database/repo"
	"github.com/NexGen-X/Route-X/internal/database/repo/policy"
	"github.com/NexGen-X/Route-X/internal/database/repo/upstream"
	"github.com/NexGen-X/Route-X/internal/database/seed"
	"github.com/NexGen-X/Route-X/internal/httpx"
	"github.com/NexGen-X/Route-X/internal/router"
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

	dtos := make([]RoutingRuleDTO, 0, len(items))
	for _, it := range items {
		dtos = append(dtos, toRoutingRuleDTO(it))
	}

	_ = h.respond(w, r, http.StatusOK, ListEnvelope[RoutingRuleDTO]{
		Items:      dtos,
		NextCursor: next,
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

	_ = h.respond(w, r, http.StatusOK, toRoutingRuleDTO(rule))
}

type createRoutingRuleReq struct {
	Name              string          `json:"name"`
	Description       string          `json:"description"`
	Strategy          string          `json:"strategy"`
	Priority          int             `json:"priority"`
	MatchModelID      *string         `json:"match_model_id"`
	MatchAPIKeyID     *string         `json:"match_api_key_id"`
	MatchCapabilities []string        `json:"match_capabilities"`
	MaxAttempts       *int            `json:"max_attempts"`
	BackoffMS         *int            `json:"backoff_ms"`
	FailureThreshold  *int            `json:"failure_threshold"`
	OpenDurationMS    *int            `json:"open_duration_ms"`
	HalfOpenProbes    *int            `json:"half_open_probes"`
	Enabled           *bool           `json:"enabled"`
	ProviderIDs       []string        `json:"provider_ids"`
	Weights           map[string]int  `json:"weights"`
	Pipeline          json.RawMessage `json:"pipeline"`
	VirtualAlias      *string         `json:"virtual_alias"`
}

func cleanUUID(s *string) *string {
	if s == nil || strings.TrimSpace(*s) == "" {
		return nil
	}
	trimmed := strings.TrimSpace(*s)
	return &trimmed
}

// literalNullJSON adalah representasi JSON null mentah, dipakai membandingkan field
// pipeline mentah supaya "null eksplisit" bisa dibedakan dari "tidak disebut".
var literalNullJSON = []byte("null")

// permintaanPipeline mengubah field pipeline dari body permintaan menjadi jsonb siap
// simpan beserta mode perubahannya.
//
// Pipeline adalah satu-satunya field di permintaan ini yang harus membedakan TIGA
// keadaan: tidak disebut (biarkan resep yang sekarang), null eksplisit (hapus resep,
// kembali ke jalur lama), dan objek (pasang resep). Pointer biasa hanya bisa
// membedakan dua keadaan, sehingga update tidak bisa menghapus resep combo — dan itulah
// jebakan yang membuat alias lama tak pernah bisa dilepas dari aturannya.
//
// Mengembalikan (diatur, data): diatur=false berarti biarkan; diatur=true dengan data nil
// berarti hapus; diatur=true dengan data terisi berarti pasang.
func permintaanPipeline(raw json.RawMessage) (diatur bool, data []byte, err error) {
	if len(raw) == 0 {
		return false, nil, nil
	}
	if bytes.Equal(bytes.TrimSpace(raw), literalNullJSON) {
		return true, nil, nil
	}
	p, err := router.ParseComboPipelineDTO(raw)
	if err != nil {
		return false, nil, err
	}
	// Disimpan ulang dalam bentuk kanonik: field asing yang tidak dikenal constraint
	// maupun mesin dibuang, supaya jsonb di database hanya berisi apa yang dipakai.
	out, mErr := json.Marshal(p)
	if mErr != nil {
		return false, nil, mErr
	}
	return true, out, nil
}

// aliasPermintaan menormalkan field virtual_alias dari body permintaan.
//
// Mengembalikan (diatur, nilai): diatur=false berarti field tidak disebut, biarkan; nilai
// kosong berarti hapus alias (whitespace dianggap kosong karena alias adalah pengenal
// publik, bukan teks bebas).
func aliasPermintaan(v *string) (diatur bool, nilai string) {
	if v == nil {
		return false, ""
	}
	return true, strings.TrimSpace(*v)
}

// pastikanAliasBebas memastikan alias belum diduduki aturan lain.
//
// Pemeriksaan ini memberi pesan 409 yang menjelaskan aturan mana yang memakainya, sebelum
// partial unique index routing_rules_virtual_alias_idx menolaknya sebagai pelanggaran
// constraint generik. Unique index tetap menjadi pengaman terakhir untuk dua permintaan
// yang berlomba — pemeriksaan ini tidak memakai kunci baris, dan itupun sudah cukup
// karena operator admin adalah tunggal.
func (h *Handlers) pastikanAliasBebas(ctx context.Context, alias, idSekarang string) error {
	if alias == "" {
		return nil
	}
	pemilik, err := h.routingRepo.FindByAlias(ctx, alias)
	if errors.Is(err, repo.ErrNotFound) {
		return nil
	}
	if err != nil {
		// Bukan kegagalan keunikan: jangan blokir penyimpanan hanya karena pemeriksaan
		// tambahan ini tidak bisa dibaca. Unique index menangkalu ganda yang sebenarnya.
		return nil
	}
	if pemilik.ID == idSekarang {
		return nil
	}
	return fmt.Errorf("alias virtual %q sudah dipakai aturan %q", alias, pemilik.Name)
}

// tolakAliasYatim menolak alias yang disertai tanpa resep pipeline combo.
//
// Tanpa pipeline, aturan tidak punya apapun untuk di-failover-kan ke model lain, dan
// aliasnya hanya menjebak klien yang memanggilnya: permintaan akan cocok dengan aturan
// yang kondisi pencocokannya tidak pernah diminta siapa pun. Dulu alias yatim seperti ini
// tertinggal diam-diam karena pendaftarannya adalah panggilan API terpisah yang
// kegagalannya hanya dicatat sebagai peringatan.
func tolakAliasYatim(alias string, pipeline []byte) error {
	if alias == "" || len(pipeline) > 0 {
		return nil
	}
	return fmt.Errorf("virtual_alias %q wajib disertai pipeline combo; kirim pipeline atau kosongkan virtual_alias", alias)
}

func (h *Handlers) createRoutingRule(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	var req createRoutingRuleReq
	if err := decodeJSON(r, &req); err != nil {
		httpx.BadRequest(w, r, "invalid_json", err.Error())
		return
	}

	// Pipeline diurai dan divalidasi SEBELUM transaksi. Objek yang cacat ditolak di
	// sini, bukan oleh constraint database sesudah aturan dibuat — penolakan terlambat
	// itu membiarkan operator menyimpan aturan tanpa sengaja dan kehilangan seluruh
	// isian formnya.
	_, pipeline, err := permintaanPipeline(req.Pipeline)
	if err != nil {
		httpx.BadRequest(w, r, "invalid_pipeline", err.Error())
		return
	}
	_, alias := aliasPermintaan(req.VirtualAlias)
	if err := tolakAliasYatim(alias, pipeline); err != nil {
		httpx.BadRequest(w, r, "alias_tanpa_pipeline", err.Error())
		return
	}
	if err := h.pastikanAliasBebas(ctx, alias, ""); err != nil {
		httpx.BadRequest(w, r, "alias_conflict", err.Error())
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

	var rule *upstream.RoutingRule
	err = repo.InTx(ctx, h.pool, func(q repo.Querier) error {
		txRouting := upstream.NewRoutingRepo(q)
		var err error
		rule, err = txRouting.Create(ctx, upstream.CreateRoutingRuleParams{
			Name:              req.Name,
			Description:       desc,
			Priority:          prio,
			MatchModelID:      cleanUUID(req.MatchModelID),
			MatchAPIKeyID:     cleanUUID(req.MatchAPIKeyID),
			MatchCapabilities: req.MatchCapabilities,
			Strategy:          req.Strategy,
			MaxAttempts:       req.MaxAttempts,
			BackoffMS:         req.BackoffMS,
			FailureThreshold:  req.FailureThreshold,
			OpenDurationMS:    req.OpenDurationMS,
			HalfOpenProbes:    req.HalfOpenProbes,
			Enabled:           req.Enabled,
			CreatedBy:         actorID,
			Pipeline:          pipeline,
			VirtualAlias:      &alias,
		})
		if err != nil {
			return err
		}

		if len(req.ProviderIDs) > 0 {
			if err := txRouting.SetProviders(ctx, rule.ID, req.ProviderIDs, req.Weights); err != nil {
				return err
			}
			rule, err = txRouting.Get(ctx, rule.ID)
			if err != nil {
				return err
			}
		}
		return nil
	})
	if err != nil {
		mapRepoError(w, r, err, "aturan routing")
		return
	}
	if rule == nil {
		httpx.InternalError(w, r)
		return
	}

	h.writeAudit(ctx, r, "create", "routing_rule", rule.ID, map[string]any{
		"name":          rule.Name,
		"strategy":      rule.Strategy,
		"virtual_alias": alias,
	})
	_ = h.respond(w, r, http.StatusCreated, toRoutingRuleDTO(rule))
}

func (h *Handlers) updateRoutingRule(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	id := chi.URLParam(r, "id")

	var req createRoutingRuleReq
	if err := decodeJSON(r, &req); err != nil {
		httpx.BadRequest(w, r, "invalid_json", err.Error())
		return
	}

	pipeDiatur, pipeline, err := permintaanPipeline(req.Pipeline)
	if err != nil {
		httpx.BadRequest(w, r, "invalid_pipeline", err.Error())
		return
	}
	aliasDiatur, aliasBaru := aliasPermintaan(req.VirtualAlias)

	// Aturan yang diubah bisa sudah punya alias dan resep combo sebelumnya, jadi
	// permintaan sebagian ini harus digabung dengan keadaan sekarang. Tanpa ini, "biarkan
	// alias" tak bisa dibedakan dari "hapus alias", dan aturan combo yang hanya diubah
	// provider-nya akan kehilangan aliasnya diam-diam.
	existing, err := h.routingRepo.Get(ctx, id)
	if err != nil {
		mapRepoError(w, r, err, "aturan routing")
		return
	}
	pipeAkhir := existing.Pipeline
	if pipeDiatur {
		pipeAkhir = pipeline
	}
	aliasAkhir := ""
	if existing.VirtualAlias != nil {
		aliasAkhir = *existing.VirtualAlias
	}
	if aliasDiatur {
		aliasAkhir = aliasBaru
	}
	if err := tolakAliasYatim(aliasAkhir, pipeAkhir); err != nil {
		httpx.BadRequest(w, r, "alias_tanpa_pipeline", err.Error())
		return
	}
	if err := h.pastikanAliasBebas(ctx, aliasAkhir, id); err != nil {
		httpx.BadRequest(w, r, "alias_conflict", err.Error())
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
		if strings.TrimSpace(*req.MatchModelID) == "" {
			p.MatchModelID = upstream.Clear[string]()
		} else {
			trimmed := strings.TrimSpace(*req.MatchModelID)
			p.MatchModelID = upstream.Set(trimmed)
		}
	}
	if req.MatchAPIKeyID != nil {
		if strings.TrimSpace(*req.MatchAPIKeyID) == "" {
			p.MatchAPIKeyID = upstream.Clear[string]()
		} else {
			trimmed := strings.TrimSpace(*req.MatchAPIKeyID)
			p.MatchAPIKeyID = upstream.Set(trimmed)
		}
	}
	p.MatchCapabilities = req.MatchCapabilities
	p.MaxAttempts = req.MaxAttempts
	p.BackoffMS = req.BackoffMS
	p.FailureThreshold = req.FailureThreshold
	p.OpenDurationMS = req.OpenDurationMS
	p.HalfOpenProbes = req.HalfOpenProbes
	p.Enabled = req.Enabled
	if pipeDiatur {
		if len(pipeline) == 0 {
			p.Pipeline = upstream.Clear[[]byte]()
		} else {
			p.Pipeline = upstream.Set(pipeline)
		}
	}
	if aliasDiatur {
		if aliasBaru == "" {
			p.VirtualAlias = upstream.Clear[string]()
		} else {
			p.VirtualAlias = upstream.Set(aliasBaru)
		}
	}

	var rule *upstream.RoutingRule
	err = repo.InTx(ctx, h.pool, func(q repo.Querier) error {
		txRouting := upstream.NewRoutingRepo(q)
		var err error
		rule, err = txRouting.Update(ctx, id, p)
		if err != nil {
			return err
		}

		if req.ProviderIDs != nil {
			if err := txRouting.SetProviders(ctx, id, req.ProviderIDs, req.Weights); err != nil {
				return err
			}
			rule, err = txRouting.Get(ctx, id)
			if err != nil {
				return err
			}
		}
		return nil
	})
	if err != nil {
		mapRepoError(w, r, err, "aturan routing")
		return
	}
	if rule == nil {
		httpx.InternalError(w, r)
		return
	}

	h.writeAudit(ctx, r, "update", "routing_rule", id, map[string]any{
		"name":          rule.Name,
		"virtual_alias": aliasAkhir,
	})
	_ = h.respond(w, r, http.StatusOK, toRoutingRuleDTO(rule))
}

func (h *Handlers) deleteRoutingRule(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	id := chi.URLParam(r, "id")

	if err := h.routingRepo.Delete(ctx, id); err != nil {
		mapRepoError(w, r, err, "aturan routing")
		return
	}

	h.writeAudit(ctx, r, "delete", "routing_rule", id, nil)
	_ = h.respond(w, r, http.StatusOK, StatusResponse{Status: "deleted"})
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
	_ = h.respond(w, r, http.StatusOK, toRoutingRuleDTO(rule))
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
	_ = h.respond(w, r, http.StatusOK, toRoutingRuleDTO(rule))
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

	dtos := make([]RateLimitDTO, 0, len(items))
	for _, it := range items {
		dtos = append(dtos, toRateLimitDTO(it))
	}

	_ = h.respond(w, r, http.StatusOK, ListEnvelope[RateLimitDTO]{
		Items:      dtos,
		NextCursor: next,
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

	_ = h.respond(w, r, http.StatusOK, toRateLimitDTO(l))
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
	_ = h.respond(w, r, http.StatusCreated, toRateLimitDTO(l))
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
	_ = h.respond(w, r, http.StatusOK, toRateLimitDTO(l))
}

func (h *Handlers) deleteRateLimit(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	id := chi.URLParam(r, "id")

	if err := h.policyRepo.DeleteRateLimit(ctx, id); err != nil {
		mapRepoError(w, r, err, "batas laju")
		return
	}

	h.writeAudit(ctx, r, "delete", "rate_limit", id, nil)
	_ = h.respond(w, r, http.StatusOK, StatusResponse{Status: "deleted"})
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
	_ = h.respond(w, r, http.StatusOK, toRateLimitDTO(l))
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

	dtos := make([]BudgetDTO, 0, len(items))
	for _, it := range items {
		dtos = append(dtos, toBudgetDTO(it))
	}

	_ = h.respond(w, r, http.StatusOK, ListEnvelope[BudgetDTO]{
		Items:      dtos,
		NextCursor: next,
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

	_ = h.respond(w, r, http.StatusOK, toBudgetDTO(b))
}

type createBudgetReq struct {
	Name              string `json:"name"`
	Scope             string `json:"scope"`
	ScopeID           string `json:"scope_id"`
	Period            string `json:"period"`
	LimitUSD          string `json:"limit_usd"`
	MaxSpendUSD       string `json:"max_spend_usd"`
	ActionOnExceed    string `json:"action_on_exceed"`
	Action            string `json:"action"`
	AlertThresholdPct int    `json:"alert_threshold_pct"`
	AlertThreshold    int    `json:"alert_threshold"`
}

func (h *Handlers) createBudget(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	var req createBudgetReq
	if err := decodeJSON(r, &req); err != nil {
		httpx.BadRequest(w, r, "invalid_json", err.Error())
		return
	}

	if req.LimitUSD == "" && req.MaxSpendUSD != "" {
		req.LimitUSD = req.MaxSpendUSD
	}
	if req.ActionOnExceed == "" && req.Action != "" {
		req.ActionOnExceed = req.Action
	}
	if req.AlertThresholdPct == 0 && req.AlertThreshold != 0 {
		req.AlertThresholdPct = req.AlertThreshold
	}

	// Normalisasi cakupan global: scope_id wajib null/kosong di database
	if req.Scope == "global" {
		req.ScopeID = ""
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
	_ = h.respond(w, r, http.StatusCreated, toBudgetDTO(b))
}

func (h *Handlers) updateBudget(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	id := chi.URLParam(r, "id")

	var req struct {
		Name              *string `json:"name"`
		LimitUSD          *string `json:"limit_usd"`
		MaxSpendUSD       *string `json:"max_spend_usd"`
		ActionOnExceed    *string `json:"action_on_exceed"`
		Action            *string `json:"action"`
		AlertThresholdPct *int    `json:"alert_threshold_pct"`
		AlertThreshold    *int    `json:"alert_threshold"`
		Enabled           *bool   `json:"enabled"`
	}
	if err := decodeJSON(r, &req); err != nil {
		httpx.BadRequest(w, r, "invalid_json", err.Error())
		return
	}

	if req.LimitUSD == nil && req.MaxSpendUSD != nil {
		req.LimitUSD = req.MaxSpendUSD
	}
	if req.ActionOnExceed == nil && req.Action != nil {
		req.ActionOnExceed = req.Action
	}
	if req.AlertThresholdPct == nil && req.AlertThreshold != nil {
		req.AlertThresholdPct = req.AlertThreshold
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
	_ = h.respond(w, r, http.StatusOK, toBudgetDTO(b))
}

func (h *Handlers) deleteBudget(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	id := chi.URLParam(r, "id")

	if err := h.policyRepo.DeleteBudget(ctx, id); err != nil {
		mapRepoError(w, r, err, "anggaran")
		return
	}

	h.writeAudit(ctx, r, "delete", "budget", id, nil)
	_ = h.respond(w, r, http.StatusOK, StatusResponse{Status: "deleted"})
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
	_ = h.respond(w, r, http.StatusOK, toBudgetDTO(b))
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
	_ = h.respond(w, r, http.StatusOK, toBudgetDTO(b))
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

	dtos := make([]BanDTO, 0, len(items))
	for _, it := range items {
		dtos = append(dtos, toBanDTO(it))
	}

	_ = h.respond(w, r, http.StatusOK, ListEnvelope[BanDTO]{
		Items: dtos,
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
		t, err := time.Parse(time.RFC3339, *req.ExpiresAt)
		if err != nil {
			httpx.BadRequest(w, r, "invalid_expires_at", fmt.Sprintf("format expires_at tidak sah (harus RFC3339): %v", err))
			return
		}
		exp = &t
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
	_ = h.respond(w, r, http.StatusCreated, toBanDTO(ban))
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
	_ = h.respond(w, r, http.StatusOK, toBanDTO(ban))
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

	dtos := make([]ContentFilterDTO, 0, len(items))
	for _, it := range items {
		dtos = append(dtos, toContentFilterDTO(it))
	}

	_ = h.respond(w, r, http.StatusOK, ListEnvelope[ContentFilterDTO]{
		Items:      dtos,
		NextCursor: next,
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

	_ = h.respond(w, r, http.StatusOK, toContentFilterDTO(f))
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
	_ = h.respond(w, r, http.StatusCreated, toContentFilterDTO(f))
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
	if req.Kind != "" {
		p.Kind = &req.Kind
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
	if req.ModelID != "" {
		p.ModelID = &req.ModelID
	}
	if req.ProviderID != "" {
		p.ProviderID = &req.ProviderID
	}
	if req.MaxRequestBytes != nil {
		p.MaxRequestBytes = req.MaxRequestBytes
	}
	p.Enabled = req.Enabled

	f, err := h.policyRepo.UpdateFilter(ctx, id, p)
	if err != nil {
		mapRepoError(w, r, err, "filter konten")
		return
	}

	h.writeAudit(ctx, r, "update", "content_filter", id, map[string]any{"name": f.Name})
	_ = h.respond(w, r, http.StatusOK, toContentFilterDTO(f))
}

func (h *Handlers) deleteContentFilter(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	id := chi.URLParam(r, "id")

	if err := h.policyRepo.DeleteFilter(ctx, id); err != nil {
		mapRepoError(w, r, err, "filter konten")
		return
	}

	h.writeAudit(ctx, r, "delete", "content_filter", id, nil)
	_ = h.respond(w, r, http.StatusOK, StatusResponse{Status: "deleted"})
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
	_ = h.respond(w, r, http.StatusOK, toContentFilterDTO(f))
}

// -----------------------------------------------------------------------------
// Circuit Breakers Handlers
// -----------------------------------------------------------------------------

func (h *Handlers) listCircuitBreakers(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()

	items := make([]CircuitBreakerDTO, 0)
	if h.redis != nil {
		// Pindai kunci di bawah namespace sirkuit breaker resmi routex:cb:*
		pattern := cache.Key(cache.NamespaceCircuitBreaker) + ":*"
		iter := h.redis.Scan(ctx, 0, pattern, 500).Iterator()
		for iter.Next(ctx) {
			k := iter.Val()
			// Segmen kunci: routex:cb:<provider_id>[:<model>]
			parts := strings.Split(k, ":")
			if len(parts) < 3 {
				continue
			}
			providerID, _ := url.PathUnescape(parts[2])
			model := ""
			if len(parts) >= 4 {
				model, _ = url.PathUnescape(parts[3])
			}

			// Ambil status keputusan state sesungguhnya dari Breaker.State()
			stateStr := "closed"
			if h.breaker != nil {
				st, err := h.breaker.State(ctx, providerID, model)
				if err == nil {
					stateStr = string(st)
				}
			}

			// Ambil metrik kegagalan dan jadwal probe dari Redis hash
			hashData, _ := h.redis.HGetAll(ctx, k).Result()
			var failureCount int64
			for fKey, fVal := range hashData {
				if strings.HasPrefix(fKey, "f") {
					if cnt, err := strconv.ParseInt(fVal, 10, 64); err == nil {
						failureCount += cnt
					}
				}
			}

			var nextProbeAt string
			if openUntilStr, ok := hashData["open_until"]; ok {
				if openUntilMs, err := strconv.ParseInt(openUntilStr, 10, 64); err == nil && openUntilMs > 0 {
					nextProbeAt = time.UnixMilli(openUntilMs).Format(time.RFC3339)
				}
			}

			items = append(items, CircuitBreakerDTO{
				Key:          k,
				ProviderID:   providerID,
				Model:        model,
				State:        stateStr,
				FailureCount: failureCount,
				NextProbeAt:  nextProbeAt,
			})
		}
	}

	_ = h.respond(w, r, http.StatusOK, ListEnvelope[CircuitBreakerDTO]{
		Items: items,
	})
}

type resetBreakerReq struct {
	ProviderID string `json:"provider_id"`
	Model      string `json:"model"`
}

func (h *Handlers) resetCircuitBreaker(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	var req resetBreakerReq
	if err := decodeJSON(r, &req); err != nil {
		httpx.BadRequest(w, r, "invalid_json", err.Error())
		return
	}

	if strings.TrimSpace(req.ProviderID) == "" {
		httpx.BadRequest(w, r, "invalid_request", "provider_id wajib diisi")
		return
	}

	// Kunci Redis SELALU dirakit di sisi server menggunakan helper cache.CircuitBreakerKey.
	// Klien dilarang menyuplai kunci Redis mentah untuk mencegah penghapusan namespace lain
	// (misal routex:rl:apikey:* untuk menghapus rate limit).
	serverKey := cache.CircuitBreakerKey(req.ProviderID, req.Model)

	if h.breaker != nil {
		if err := h.breaker.Reset(ctx, req.ProviderID, req.Model); err != nil {
			httpx.InternalError(w, r)
			return
		}
	} else if h.redis != nil {
		_ = h.redis.Del(ctx, serverKey).Err()
	}

	h.writeAudit(ctx, r, "reset", "circuit_breaker", serverKey, map[string]any{
		"provider_id": req.ProviderID,
		"model":       req.Model,
	})
	_ = h.respond(w, r, http.StatusOK, StatusResponse{Status: "reset"})
}
