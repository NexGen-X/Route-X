package admin

import (
	"net/http"
	"net/netip"
	"strconv"
	"time"

	"github.com/go-chi/chi/v5"

	"github.com/NexGen-X/Route-X/internal/auth"
	"github.com/NexGen-X/Route-X/internal/database/repo"
	"github.com/NexGen-X/Route-X/internal/database/repo/identity"
	"github.com/NexGen-X/Route-X/internal/database/repo/keys"
	"github.com/NexGen-X/Route-X/internal/database/seed"
	"github.com/NexGen-X/Route-X/internal/httpx"
	"github.com/NexGen-X/Route-X/internal/security"
)

func (h *Handlers) accessRoutes(r chi.Router) {
	// 1. Client API Keys
	r.Route("/api-keys", func(kr chi.Router) {
		kr.With(auth.RequirePermission(seed.PermAPIKeysRead)).Get("/", h.listAPIKeys)
		kr.With(auth.RequirePermission(seed.PermAPIKeysWrite)).Post("/", h.createAPIKey)
		kr.With(auth.RequirePermission(seed.PermAPIKeysRead)).Get("/{id}", h.getAPIKey)
		kr.With(auth.RequirePermission(seed.PermAPIKeysWrite)).Put("/{id}", h.updateAPIKey)
		kr.With(auth.RequirePermission(seed.PermAPIKeysWrite)).Post("/{id}/rotate", h.rotateAPIKey)
		kr.With(auth.RequirePermission(seed.PermAPIKeysWrite)).Post("/{id}/revoke", h.revokeAPIKey)
		kr.With(auth.RequirePermission(seed.PermAPIKeysWrite)).Post("/{id}/toggle", h.toggleAPIKey)
		kr.With(auth.RequirePermission(seed.PermAPIKeysWrite)).Post("/{id}/allowed", h.setAllowedAPIKey)
	})

	// 2. Admin Users
	r.Route("/users", func(ur chi.Router) {
		ur.With(auth.RequirePermission(seed.PermUsersRead)).Get("/", h.listUsers)
		ur.With(auth.RequirePermission(seed.PermUsersWrite)).Post("/", h.createUser)
		ur.With(auth.RequirePermission(seed.PermUsersRead)).Get("/{id}", h.getUser)
		ur.With(auth.RequirePermission(seed.PermUsersWrite)).Put("/{id}", h.updateUser)
		ur.With(auth.RequirePermission(seed.PermUsersWrite)).Delete("/{id}", h.deleteUser)
		ur.With(auth.RequirePermission(seed.PermRolesWrite)).Post("/{id}/roles", h.grantUserRole)
		ur.With(auth.RequirePermission(seed.PermRolesWrite)).Delete("/{id}/roles/{role_id}", h.revokeUserRole)
		ur.With(auth.RequirePermission(seed.PermUsersWrite)).Post("/{id}/force-reset-password", h.forceResetPassword)
		ur.With(auth.RequirePermission(seed.PermUsersWrite)).Post("/{id}/revoke-sessions", h.revokeUserSessions)
	})

	// 3. Roles & Permissions
	r.Route("/roles", func(rr chi.Router) {
		rr.With(auth.RequirePermission(seed.PermRolesRead)).Get("/", h.listRoles)
		rr.With(auth.RequirePermission(seed.PermRolesWrite)).Post("/", h.createRole)
		rr.With(auth.RequirePermission(seed.PermRolesRead)).Get("/{id}", h.getRole)
		rr.With(auth.RequirePermission(seed.PermRolesWrite)).Delete("/{id}", h.deleteRole)
		rr.With(auth.RequirePermission(seed.PermRolesWrite)).Put("/{id}/permissions", h.setRolePermissions)
	})

	r.With(auth.RequirePermission(seed.PermRolesRead)).Get("/permissions", h.listPermissions)

	// 4. Sessions
	r.Route("/sessions", func(sr chi.Router) {
		sr.With(auth.RequirePermission(seed.PermUsersRead)).Get("/", h.listSessions)
		sr.With(auth.RequirePermission(seed.PermUsersWrite)).Delete("/{id}", h.revokeSession)
	})
}

// -----------------------------------------------------------------------------
// API Keys Handlers
// -----------------------------------------------------------------------------

func (h *Handlers) listAPIKeys(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	limit := repo.DefaultPageLimit
	if s := r.URL.Query().Get("limit"); s != "" {
		if n, err := strconv.Atoi(s); err == nil && n > 0 {
			limit = n
		}
	}

	filter := keys.ListFilter{
		OwnerUserID: r.URL.Query().Get("user_id"),
		Status:      r.URL.Query().Get("status"),
		Search:      r.URL.Query().Get("search"),
	}

	page := repo.Page{
		Limit:  limit,
		Cursor: r.URL.Query().Get("cursor"),
	}

	items, next, err := h.keyRepo.List(ctx, filter, page)
	if err != nil {
		mapRepoError(w, r, err, "api key")
		return
	}

	type KeyItem struct {
		*keys.Key
		Masked string `json:"masked"`
	}

	var formatted []KeyItem
	for _, k := range items {
		formatted = append(formatted, KeyItem{
			Key:    k,
			Masked: k.Masked(),
		})
	}

	_ = httpx.JSON(w, http.StatusOK, map[string]any{
		"items":       formatted,
		"next_cursor": next,
	})
}

func (h *Handlers) getAPIKey(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	id := chi.URLParam(r, "id")

	k, err := h.keyRepo.GetByID(ctx, id)
	if err != nil {
		mapRepoError(w, r, err, "api key")
		return
	}

	_ = httpx.JSON(w, http.StatusOK, map[string]any{
		"key":    k,
		"masked": k.Masked(),
	})
}

type createAPIKeyReq struct {
	Name                string   `json:"name"`
	OwnerUserID         string   `json:"owner_user_id"`
	Live                bool     `json:"live"`
	Scopes              []string `json:"scopes"`
	RateLimitRPS        *int     `json:"rate_limit_rps"`
	RateLimitRPM        *int     `json:"rate_limit_rpm"`
	RateLimitTPM        *int     `json:"rate_limit_tpm"`
	DailyRequestLimit   *int64   `json:"daily_request_limit"`
	MonthlyRequestLimit *int64   `json:"monthly_request_limit"`
	DailyTokenLimit     *int64   `json:"daily_token_limit"`
	MonthlyTokenLimit   *int64   `json:"monthly_token_limit"`
	IPAllowlist         []string `json:"ip_allowlist"`
	ExpiresAt           *string  `json:"expires_at"`
	ModelIDs            []string `json:"model_ids"`
	ProviderIDs         []string `json:"provider_ids"`
}

func (h *Handlers) createAPIKey(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	var req createAPIKeyReq
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

	var ips []netip.Prefix
	for _, s := range req.IPAllowlist {
		if p, err := netip.ParsePrefix(s); err == nil {
			ips = append(ips, p)
		} else if addr, err := netip.ParseAddr(s); err == nil {
			ips = append(ips, netip.PrefixFrom(addr, addr.BitLen()))
		}
	}

	var actorID *string
	if p, ok := auth.PrincipalFrom(ctx); ok && p != nil {
		actorID = &p.User.ID
	}

	ownerID := req.OwnerUserID
	if ownerID == "" && actorID != nil {
		ownerID = *actorID
	}
	scopes := req.Scopes
	if len(scopes) == 0 {
		scopes = []string{"inference"}
	}

	created, err := h.keyRepo.Create(ctx, keys.CreateParams{
		Name:                req.Name,
		OwnerUserID:         ownerID,
		Live:                req.Live,
		Scopes:              scopes,
		RateLimitRPS:        req.RateLimitRPS,
		RateLimitRPM:        req.RateLimitRPM,
		RateLimitTPM:        req.RateLimitTPM,
		DailyRequestLimit:   req.DailyRequestLimit,
		MonthlyRequestLimit: req.MonthlyRequestLimit,
		DailyTokenLimit:     req.DailyTokenLimit,
		MonthlyTokenLimit:   req.MonthlyTokenLimit,
		IPAllowlist:         ips,
		ExpiresAt:           exp,
		CreatedBy:           actorID,
	})
	if err != nil {
		mapRepoError(w, r, err, "api key")
		return
	}

	if len(req.ModelIDs) > 0 || len(req.ProviderIDs) > 0 {
		_ = h.keyRepo.SetAllowed(ctx, created.Key.ID, req.ModelIDs, req.ProviderIDs)
	}

	h.writeAudit(ctx, r, "create", "api_key", created.Key.ID, map[string]any{"name": created.Key.Name, "prefix": created.Key.Prefix})
	_ = httpx.JSON(w, http.StatusCreated, map[string]any{
		"key":     created.Key,
		"masked":  created.Key.Masked(),
		"token":   created.Raw.Reveal(), // Token mentah hanya dikembalikan saat pembuatan!
		"raw_key": created.Raw.Reveal(),
	})
}

func (h *Handlers) updateAPIKey(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	id := chi.URLParam(r, "id")

	var req struct {
		Name                *string  `json:"name"`
		RateLimitRPS        *int     `json:"rate_limit_rps"`
		RateLimitRPM        *int     `json:"rate_limit_rpm"`
		RateLimitTPM        *int     `json:"rate_limit_tpm"`
		DailyRequestLimit   *int64   `json:"daily_request_limit"`
		MonthlyRequestLimit *int64   `json:"monthly_request_limit"`
		DailyTokenLimit     *int64   `json:"daily_token_limit"`
		MonthlyTokenLimit   *int64   `json:"monthly_token_limit"`
		IPAllowlist         []string `json:"ip_allowlist"`
		ExpiresAt           *string  `json:"expires_at"`
	}
	if err := decodeJSON(r, &req); err != nil {
		httpx.BadRequest(w, r, "invalid_json", err.Error())
		return
	}

	var ips []netip.Prefix
	if req.IPAllowlist != nil {
		for _, s := range req.IPAllowlist {
			if p, err := netip.ParsePrefix(s); err == nil {
				ips = append(ips, p)
			} else if addr, err := netip.ParseAddr(s); err == nil {
				ips = append(ips, netip.PrefixFrom(addr, addr.BitLen()))
			}
		}
	}

	var exp *time.Time
	if req.ExpiresAt != nil && *req.ExpiresAt != "" {
		if t, err := time.Parse(time.RFC3339, *req.ExpiresAt); err == nil {
			exp = &t
		}
	}

	var params keys.UpdateParams
	params.Name = req.Name
	params.IPAllowlist = ips
	if req.RateLimitRPS != nil {
		params.RateLimitRPS = keys.Set(*req.RateLimitRPS)
	}
	if req.RateLimitRPM != nil {
		params.RateLimitRPM = keys.Set(*req.RateLimitRPM)
	}
	if req.RateLimitTPM != nil {
		params.RateLimitTPM = keys.Set(*req.RateLimitTPM)
	}
	if req.DailyRequestLimit != nil {
		params.DailyRequestLimit = keys.Set(*req.DailyRequestLimit)
	}
	if req.MonthlyRequestLimit != nil {
		params.MonthlyRequestLimit = keys.Set(*req.MonthlyRequestLimit)
	}
	if req.DailyTokenLimit != nil {
		params.DailyTokenLimit = keys.Set(*req.DailyTokenLimit)
	}
	if req.MonthlyTokenLimit != nil {
		params.MonthlyTokenLimit = keys.Set(*req.MonthlyTokenLimit)
	}
	if exp != nil {
		params.ExpiresAt = keys.Set(*exp)
	}

	k, err := h.keyRepo.Update(ctx, id, params)
	if err != nil {
		mapRepoError(w, r, err, "api key")
		return
	}

	h.writeAudit(ctx, r, "update", "api_key", id, nil)
	_ = httpx.JSON(w, http.StatusOK, map[string]any{
		"key":    k,
		"masked": k.Masked(),
	})
}

func (h *Handlers) rotateAPIKey(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	id := chi.URLParam(r, "id")

	var actorID *string
	if p, ok := auth.PrincipalFrom(ctx); ok && p != nil {
		actorID = &p.User.ID
	}

	created, err := h.keyRepo.RotateWithPool(ctx, h.pool, id, actorID)
	if err != nil {
		mapRepoError(w, r, err, "rotasi api key")
		return
	}

	h.writeAudit(ctx, r, "rotate", "api_key", id, map[string]any{"new_key_id": created.Key.ID})
	_ = httpx.JSON(w, http.StatusOK, map[string]any{
		"key":    created.Key,
		"masked": created.Key.Masked(),
		"token":  created.Raw.Reveal(),
	})
}

func (h *Handlers) revokeAPIKey(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	id := chi.URLParam(r, "id")

	var req struct {
		Reason string `json:"reason"`
	}
	_ = decodeJSON(r, &req)

	var actorID *string
	if p, ok := auth.PrincipalFrom(ctx); ok && p != nil {
		actorID = &p.User.ID
	}

	reason := req.Reason
	if reason == "" {
		reason = "dicabut oleh admin"
	}

	k, err := h.keyRepo.Revoke(ctx, id, actorID, reason)
	if err != nil {
		mapRepoError(w, r, err, "api key")
		return
	}

	h.writeAudit(ctx, r, "revoke", "api_key", id, map[string]any{"reason": reason})
	_ = httpx.JSON(w, http.StatusOK, map[string]any{
		"key":    k,
		"masked": k.Masked(),
	})
}

func (h *Handlers) toggleAPIKey(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	id := chi.URLParam(r, "id")

	var req struct {
		Status string `json:"status"` // "active" atau "disabled"
	}
	if err := decodeJSON(r, &req); err != nil {
		httpx.BadRequest(w, r, "invalid_json", err.Error())
		return
	}

	k, err := h.keyRepo.SetStatus(ctx, id, req.Status)
	if err != nil {
		mapRepoError(w, r, err, "api key")
		return
	}

	h.writeAudit(ctx, r, "toggle", "api_key", id, map[string]any{"status": req.Status})
	_ = httpx.JSON(w, http.StatusOK, map[string]any{
		"key":    k,
		"masked": k.Masked(),
	})
}

func (h *Handlers) setAllowedAPIKey(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	id := chi.URLParam(r, "id")

	var req struct {
		ModelIDs    []string `json:"model_ids"`
		ProviderIDs []string `json:"provider_ids"`
	}
	if err := decodeJSON(r, &req); err != nil {
		httpx.BadRequest(w, r, "invalid_json", err.Error())
		return
	}

	if err := h.keyRepo.SetAllowed(ctx, id, req.ModelIDs, req.ProviderIDs); err != nil {
		mapRepoError(w, r, err, "izin model/provider api key")
		return
	}

	h.writeAudit(ctx, r, "set_allowed", "api_key", id, map[string]any{"models": req.ModelIDs, "providers": req.ProviderIDs})
	_ = httpx.JSON(w, http.StatusOK, map[string]string{"status": "updated"})
}

// -----------------------------------------------------------------------------
// Admin Users Handlers
// -----------------------------------------------------------------------------

func (h *Handlers) listUsers(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	page := repo.Page{
		Limit:  repo.DefaultPageLimit,
		Cursor: r.URL.Query().Get("cursor"),
	}

	userPage, err := h.usersRepo.List(ctx, identity.UserFilter{}, page)
	if err != nil {
		mapRepoError(w, r, err, "pengguna")
		return
	}

	_ = httpx.JSON(w, http.StatusOK, map[string]any{
		"items":       userPage.Users,
		"next_cursor": userPage.NextCursor,
	})
}

func (h *Handlers) getUser(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	id := chi.URLParam(r, "id")

	u, err := h.usersRepo.GetByID(ctx, id)
	if err != nil {
		mapRepoError(w, r, err, "pengguna")
		return
	}

	userRoles, _ := h.rolesRepo.OfUser(ctx, id)
	perms, _ := h.rolesRepo.EffectivePermissions(ctx, id)

	_ = httpx.JSON(w, http.StatusOK, map[string]any{
		"user":        u,
		"roles":       userRoles,
		"permissions": perms,
	})
}

type createUserReq struct {
	Email              string   `json:"email"`
	Name               string   `json:"name"`
	Password           string   `json:"password"`
	MustChangePassword bool     `json:"must_change_password"`
	RoleIDs            []string `json:"role_ids"`
}

func (h *Handlers) createUser(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	var req createUserReq
	if err := decodeJSON(r, &req); err != nil {
		httpx.BadRequest(w, r, "invalid_json", err.Error())
		return
	}

	var actorID string
	if p, ok := auth.PrincipalFrom(ctx); ok && p != nil {
		actorID = p.User.ID
	}

	var createdUser identity.User
	err := repo.InTx(ctx, h.pool, func(q repo.Querier) error {
		txUsers := identity.NewUsers(q)
		txRoles := identity.NewRoles(q)

		u, err := txUsers.Create(ctx, identity.NewUser{
			Email:              req.Email,
			DisplayName:        req.Name,
			Password:           security.Secret(req.Password),
			MustChangePassword: req.MustChangePassword,
		})
		if err != nil {
			return err
		}
		createdUser = u

		for _, roleID := range req.RoleIDs {
			if err := txRoles.Grant(ctx, u.ID, roleID, actorID); err != nil {
				return err
			}
		}
		return nil
	})

	if err != nil {
		mapRepoError(w, r, err, "pengguna")
		return
	}

	h.writeAudit(ctx, r, "create", "user", createdUser.ID, map[string]any{"email": createdUser.Email})
	_ = httpx.JSON(w, http.StatusCreated, createdUser)
}

func (h *Handlers) updateUser(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	id := chi.URLParam(r, "id")

	var req struct {
		Name   string `json:"name"`
		Status string `json:"status"`
	}
	if err := decodeJSON(r, &req); err != nil {
		httpx.BadRequest(w, r, "invalid_json", err.Error())
		return
	}

	if req.Name != "" {
		_, err := h.usersRepo.UpdateProfile(ctx, id, identity.ProfileUpdate{
			DisplayName: &req.Name,
		})
		if err != nil {
			mapRepoError(w, r, err, "pengguna")
			return
		}
	}

	if req.Status != "" {
		_, err := h.usersRepo.SetStatus(ctx, id, identity.UserStatus(req.Status))
		if err != nil {
			mapRepoError(w, r, err, "status pengguna")
			return
		}
	}

	u, _ := h.usersRepo.GetByID(ctx, id)
	h.writeAudit(ctx, r, "update", "user", id, nil)
	_ = httpx.JSON(w, http.StatusOK, u)
}

func (h *Handlers) deleteUser(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	id := chi.URLParam(r, "id")

	if err := h.usersRepo.Delete(ctx, id); err != nil {
		mapRepoError(w, r, err, "pengguna")
		return
	}

	h.writeAudit(ctx, r, "delete", "user", id, nil)
	_ = httpx.JSON(w, http.StatusOK, map[string]string{"status": "deleted"})
}

func (h *Handlers) grantUserRole(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	id := chi.URLParam(r, "id")

	var req struct {
		RoleID string `json:"role_id"`
	}
	if err := decodeJSON(r, &req); err != nil {
		httpx.BadRequest(w, r, "invalid_json", err.Error())
		return
	}

	var actorID string
	if p, ok := auth.PrincipalFrom(ctx); ok && p != nil {
		actorID = p.User.ID
	}

	if err := h.rolesRepo.Grant(ctx, id, req.RoleID, actorID); err != nil {
		mapRepoError(w, r, err, "pemberian peran")
		return
	}

	h.writeAudit(ctx, r, "grant_role", "user", id, map[string]any{"role_id": req.RoleID})
	_ = httpx.JSON(w, http.StatusOK, map[string]string{"status": "granted"})
}

func (h *Handlers) revokeUserRole(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	id := chi.URLParam(r, "id")
	roleID := chi.URLParam(r, "role_id")

	if err := h.rolesRepo.Revoke(ctx, id, roleID); err != nil {
		mapRepoError(w, r, err, "pencabutan peran")
		return
	}

	h.writeAudit(ctx, r, "revoke_role", "user", id, map[string]any{"role_id": roleID})
	_ = httpx.JSON(w, http.StatusOK, map[string]string{"status": "revoked"})
}

func (h *Handlers) forceResetPassword(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	id := chi.URLParam(r, "id")

	var req struct {
		TempPassword string `json:"temp_password"`
	}
	if err := decodeJSON(r, &req); err != nil {
		httpx.BadRequest(w, r, "invalid_json", err.Error())
		return
	}

	if err := h.usersRepo.ChangePassword(ctx, id, security.Secret(req.TempPassword), true); err != nil {
		mapRepoError(w, r, err, "reset password")
		return
	}

	// Cabut seluruh sesi aktif
	_, _ = h.sessionsRepo.RevokeAllOfUser(ctx, id)

	h.writeAudit(ctx, r, "force_reset_password", "user", id, nil)
	_ = httpx.JSON(w, http.StatusOK, map[string]string{"status": "password_reset"})
}

func (h *Handlers) revokeUserSessions(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	id := chi.URLParam(r, "id")

	count, err := h.sessionsRepo.RevokeAllOfUser(ctx, id)
	if err != nil {
		mapRepoError(w, r, err, "sesi")
		return
	}

	h.writeAudit(ctx, r, "revoke_sessions", "user", id, map[string]any{"count": count})
	_ = httpx.JSON(w, http.StatusOK, map[string]any{"status": "revoked", "count": count})
}

// -----------------------------------------------------------------------------
// Roles & Permissions Handlers
// -----------------------------------------------------------------------------

func (h *Handlers) listRoles(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	roles, err := h.rolesRepo.List(ctx)
	if err != nil {
		mapRepoError(w, r, err, "peran")
		return
	}

	_ = httpx.JSON(w, http.StatusOK, map[string]any{
		"items": roles,
	})
}

func (h *Handlers) getRole(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	id := chi.URLParam(r, "id")

	detail, err := h.rolesRepo.Get(ctx, id)
	if err != nil {
		mapRepoError(w, r, err, "peran")
		return
	}

	_ = httpx.JSON(w, http.StatusOK, detail)
}

func (h *Handlers) createRole(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	var req struct {
		Name        string   `json:"name"`
		Description string   `json:"description"`
		Rank        int      `json:"rank"`
		Permissions []string `json:"permissions"`
	}
	if err := decodeJSON(r, &req); err != nil {
		httpx.BadRequest(w, r, "invalid_json", err.Error())
		return
	}

	var createdRole identity.Role
	err := repo.InTx(ctx, h.pool, func(q repo.Querier) error {
		txRoles := identity.NewRoles(q)
		rl, err := txRoles.Create(ctx, identity.NewRole{
			Name:        req.Name,
			Description: req.Description,
			Rank:        req.Rank,
		})
		if err != nil {
			return err
		}
		createdRole = rl

		if len(req.Permissions) > 0 {
			if err := txRoles.SetPermissions(ctx, rl.ID, req.Permissions); err != nil {
				return err
			}
		}
		return nil
	})

	if err != nil {
		mapRepoError(w, r, err, "peran")
		return
	}

	h.writeAudit(ctx, r, "create", "role", createdRole.ID, map[string]any{"name": createdRole.Name})
	_ = httpx.JSON(w, http.StatusCreated, createdRole)
}

func (h *Handlers) deleteRole(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	id := chi.URLParam(r, "id")

	if err := h.rolesRepo.Delete(ctx, id); err != nil {
		mapRepoError(w, r, err, "peran")
		return
	}

	h.writeAudit(ctx, r, "delete", "role", id, nil)
	_ = httpx.JSON(w, http.StatusOK, map[string]string{"status": "deleted"})
}

func (h *Handlers) setRolePermissions(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	id := chi.URLParam(r, "id")

	var req struct {
		Permissions []string `json:"permissions"`
	}
	if err := decodeJSON(r, &req); err != nil {
		httpx.BadRequest(w, r, "invalid_json", err.Error())
		return
	}

	if err := h.rolesRepo.SetPermissions(ctx, id, req.Permissions); err != nil {
		mapRepoError(w, r, err, "izin peran")
		return
	}

	h.writeAudit(ctx, r, "set_permissions", "role", id, map[string]any{"permissions": req.Permissions})
	_ = httpx.JSON(w, http.StatusOK, map[string]string{"status": "updated"})
}

func (h *Handlers) listPermissions(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	perms, err := h.rolesRepo.ListPermissions(ctx)
	if err != nil {
		mapRepoError(w, r, err, "katalog izin")
		return
	}

	_ = httpx.JSON(w, http.StatusOK, map[string]any{
		"items": perms,
	})
}

// -----------------------------------------------------------------------------
// Sessions Handlers
// -----------------------------------------------------------------------------

func (h *Handlers) listSessions(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	userID := r.URL.Query().Get("user_id")
	if userID == "" {
		if p, ok := auth.PrincipalFrom(ctx); ok && p != nil {
			userID = p.User.ID
		}
	}

	sessions, err := h.sessionsRepo.ListActive(ctx, userID)
	if err != nil {
		mapRepoError(w, r, err, "sesi")
		return
	}

	_ = httpx.JSON(w, http.StatusOK, map[string]any{
		"items": sessions,
	})
}

func (h *Handlers) revokeSession(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	id := chi.URLParam(r, "id")

	if err := h.sessionsRepo.Revoke(ctx, id); err != nil {
		mapRepoError(w, r, err, "sesi")
		return
	}

	h.writeAudit(ctx, r, "revoke", "session", id, nil)
	_ = httpx.JSON(w, http.StatusOK, map[string]string{"status": "revoked"})
}
