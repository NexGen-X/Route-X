package admin

import (
	"context"
	"errors"
	"fmt"
	"net/http"
	"net/netip"
	"strconv"
	"strings"
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

	keysDTO := make([]KeyDTO, 0, len(items))
	for _, k := range items {
		keysDTO = append(keysDTO, toKeyDTO(k, nil, nil))
	}

	_ = h.respond(w, r, http.StatusOK, ListEnvelope[KeyDTO]{
		Items:      keysDTO,
		NextCursor: next,
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

	_ = h.respond(w, r, http.StatusOK, KeyDetailResponse{
		Key:    toKeyDTO(k, nil, nil),
		Masked: k.Masked(),
	})
}

type createAPIKeyReq struct {
	Name                string   `json:"name"`
	Label               string   `json:"label"`
	OwnerUserID         string   `json:"owner_user_id"`
	Live                bool     `json:"live"`
	Scopes              []string `json:"scopes"`
	RateLimitRPS        *int     `json:"rate_limit_rps"`
	RateLimitRPM        *int     `json:"rate_limit_rpm"`
	RateLimitTPM        *int     `json:"rate_limit_tpm"`
	RPMLimit            *int     `json:"rpm_limit"`
	TPMLimit            *int     `json:"tpm_limit"`
	DailyRequestLimit   *int64   `json:"daily_request_limit"`
	MonthlyRequestLimit *int64   `json:"monthly_request_limit"`
	DailyTokenLimit     *int64   `json:"daily_token_limit"`
	MonthlyTokenLimit   *int64   `json:"monthly_token_limit"`
	IPAllowlist         []string `json:"ip_allowlist"`
	ExpiresAt           *string  `json:"expires_at"`
	ModelIDs            []string `json:"model_ids"`
	ProviderIDs         []string `json:"provider_ids"`
	AllowedModels       []string `json:"allowed_models"`
	AllowedProviders    []string `json:"allowed_providers"`
}

func (h *Handlers) createAPIKey(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	var req createAPIKeyReq
	if err := decodeJSON(r, &req); err != nil {
		httpx.BadRequest(w, r, "invalid_json", err.Error())
		return
	}

	// Normalisasi alias field
	if req.Name == "" && req.Label != "" {
		req.Name = req.Label
	}
	if req.RateLimitRPM == nil && req.RPMLimit != nil {
		req.RateLimitRPM = req.RPMLimit
	}
	if req.RateLimitTPM == nil && req.TPMLimit != nil {
		req.RateLimitTPM = req.TPMLimit
	}
	if len(req.ModelIDs) == 0 && len(req.AllowedModels) > 0 {
		req.ModelIDs = req.AllowedModels
	}
	if len(req.ProviderIDs) == 0 && len(req.AllowedProviders) > 0 {
		req.ProviderIDs = req.AllowedProviders
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

	var ips []netip.Prefix
	for _, s := range req.IPAllowlist {
		s = strings.TrimSpace(s)
		if s == "" {
			continue
		}
		if p, err := netip.ParsePrefix(s); err == nil {
			ips = append(ips, p)
		} else if addr, err := netip.ParseAddr(s); err == nil {
			ips = append(ips, netip.PrefixFrom(addr, addr.BitLen()))
		} else {
			httpx.BadRequest(w, r, "invalid_ip_allowlist", fmt.Sprintf("entri ip_allowlist tidak sah: %q", s))
			return
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

	// Bungkus pembuatan key beserta seluruh pembatasannya dalam SATU transaksi atomik.
	// Jika pembatasan model/provider gagal, key tidak boleh tercipta bebas ke publik.
	var created *keys.Created
	err := repo.InTx(ctx, h.pool, func(q repo.Querier) error {
		txKeyRepo := h.keyRepo.WithQuerier(q)
		var err error
		created, err = txKeyRepo.Create(ctx, keys.CreateParams{
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
			AllowedModelIDs:     req.ModelIDs,
			AllowedProviderIDs:  req.ProviderIDs,
		})
		return err
	})
	if err != nil {
		mapRepoError(w, r, err, "api key")
		return
	}

	h.writeAudit(ctx, r, "create", "api_key", created.Key.ID, map[string]any{"name": created.Key.Name, "prefix": created.Key.Prefix})
	_ = h.respond(w, r, http.StatusCreated, KeyCreatedResponse{
		Key:    toKeyDTO(created.Key, req.ModelIDs, req.ProviderIDs),
		Masked: created.Key.Masked(),
		RawKey: created.Raw.Reveal(),
	})
}

// nilaiAtau mengambil isi pointer atau nilai bawaan bila nil.
func nilaiAtau[T any](p *T, bawaan T) T {
	if p == nil {
		return bawaan
	}
	return *p
}

func (h *Handlers) updateAPIKey(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	id := chi.URLParam(r, "id")

	var req struct {
		Name                *string  `json:"name"`
		Label               *string  `json:"label"`
		RateLimitRPS        *int     `json:"rate_limit_rps"`
		RateLimitRPM        *int     `json:"rate_limit_rpm"`
		RateLimitTPM        *int     `json:"rate_limit_tpm"`
		RPMLimit            *int     `json:"rpm_limit"`
		TPMLimit            *int     `json:"tpm_limit"`
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

	if req.Name == nil && req.Label != nil {
		req.Name = req.Label
	}
	if req.RateLimitRPM == nil && req.RPMLimit != nil {
		req.RateLimitRPM = req.RPMLimit
	}
	if req.RateLimitTPM == nil && req.TPMLimit != nil {
		req.RateLimitTPM = req.TPMLimit
	}

	// ips dimulai dari slice KOSONG non-nil: body [] berarti "buka dari mana
	// saja", dan repo membacanya dari kenon-nilan (nil = biarkan, kosong
	// non-nil = buka). var ips []netip.Prefix di sini akan mengubah [] menjadi
	// nil dan niat membuka diam-diam menjadi "tidak berubah".
	ips := []netip.Prefix{}
	if req.IPAllowlist != nil {
		for _, s := range req.IPAllowlist {
			s = strings.TrimSpace(s)
			if s == "" {
				continue
			}
			if p, err := netip.ParsePrefix(s); err == nil {
				ips = append(ips, p)
			} else if addr, err := netip.ParseAddr(s); err == nil {
				ips = append(ips, netip.PrefixFrom(addr, addr.BitLen()))
			} else {
				httpx.BadRequest(w, r, "invalid_ip_allowlist", fmt.Sprintf("entri ip_allowlist tidak sah: %q", s))
				return
			}
		}
	}

	// expires_at "": niat membuka (tanpa kedaluwarsa) BELUM didukung kontrak —
	// repo sudah punya keys.Clear, tetapi pointer *string tidak bisa membedakan
	// "tidak disebut" (nil) dari "kosongkan" (""). Daripada diam-diam
	// mengabaikan ("biarkan"), tolak dengan pesan yang menunjuk jalan keluar.
	if req.ExpiresAt != nil && *req.ExpiresAt == "" {
		httpx.BadRequest(w, r, "expires_clear_unsupported", "pengosongan expires_at belum didukung lewat endpoint ini; hubungi operator database")
		return
	}
	var exp *time.Time
	if req.ExpiresAt != nil && *req.ExpiresAt != "" {
		t, err := time.Parse(time.RFC3339, *req.ExpiresAt)
		if err != nil {
			httpx.BadRequest(w, r, "invalid_expires_at", fmt.Sprintf("format expires_at tidak sah (harus RFC3339): %v", err))
			return
		}
		if t.Before(time.Now()) {
			httpx.BadRequest(w, r, "expires_in_past", "expires_at tidak boleh di masa lalu")
			return
		}
		exp = &t
	}

	// Batas <= 0 DITOLAK di sini dengan 400: DB menolaknya lewat constraint
	// (hasilnya 500 yang membingungkan), dan tidak satu pun batas <= 0 yang
	// bermakna. Tanpa-batas dinyatakan dengan NULL, dan kontrak pembuka NULL
	// belum ada — lihat catatan di atas ExpiresAt.
	for _, batas := range []struct {
		nama  string
		ada   bool
		nilai int64
	}{
		{"rate_limit_rps", req.RateLimitRPS != nil, int64(nilaiAtau(req.RateLimitRPS, 0))},
		{"rate_limit_rpm", req.RateLimitRPM != nil, int64(nilaiAtau(req.RateLimitRPM, 0))},
		{"rate_limit_tpm", req.RateLimitTPM != nil, int64(nilaiAtau(req.RateLimitTPM, 0))},
		{"daily_request_limit", req.DailyRequestLimit != nil, nilaiAtau(req.DailyRequestLimit, 0)},
		{"monthly_request_limit", req.MonthlyRequestLimit != nil, nilaiAtau(req.MonthlyRequestLimit, 0)},
		{"daily_token_limit", req.DailyTokenLimit != nil, nilaiAtau(req.DailyTokenLimit, 0)},
		{"monthly_token_limit", req.MonthlyTokenLimit != nil, nilaiAtau(req.MonthlyTokenLimit, 0)},
	} {
		if batas.ada && batas.nilai <= 0 {
			httpx.BadRequest(w, r, "invalid_limit", "batas "+batas.nama+" harus bilangan positif")
			return
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
	_ = h.respond(w, r, http.StatusOK, KeyDetailResponse{
		Key:    toKeyDTO(k, nil, nil),
		Masked: k.Masked(),
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
	_ = h.respond(w, r, http.StatusOK, KeyCreatedResponse{
		Key:    toKeyDTO(created.Key, nil, nil),
		Masked: created.Key.Masked(),
		RawKey: created.Raw.Reveal(),
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
	_ = h.respond(w, r, http.StatusOK, KeyDetailResponse{
		Key:    toKeyDTO(k, nil, nil),
		Masked: k.Masked(),
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
	_ = h.respond(w, r, http.StatusOK, KeyDetailResponse{
		Key:    toKeyDTO(k, nil, nil),
		Masked: k.Masked(),
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
	_ = h.respond(w, r, http.StatusOK, StatusResponse{Status: "updated"})
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

	usersDTO := make([]UserDTO, 0, len(userPage.Users))
	for _, u := range userPage.Users {
		// Error di sini berarti 500: menampilkan peran kosong seolah pengguna
		// tak punya hak membuat operator salah menyimpulkan kewenangan.
		roles, err := h.rolesRepo.OfUser(ctx, u.ID)
		if err != nil {
			mapRepoError(w, r, err, "peran pengguna")
			return
		}
		roleNames := make([]string, 0, len(roles))
		for _, r := range roles {
			roleNames = append(roleNames, r.Name)
		}
		usersDTO = append(usersDTO, toUserDTO(&u, roleNames))
	}

	_ = h.respond(w, r, http.StatusOK, ListEnvelope[UserDTO]{
		Items:      usersDTO,
		NextCursor: userPage.NextCursor,
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

	// Sama seperti listUsers: error berarti 500, bukan peran/izin kosong yang
	// menyesatkan operator tentang kewenangan pengguna.
	userRoles, err := h.rolesRepo.OfUser(ctx, id)
	if err != nil {
		mapRepoError(w, r, err, "peran pengguna")
		return
	}
	perms, err := h.rolesRepo.EffectivePermissions(ctx, id)
	if err != nil {
		mapRepoError(w, r, err, "izin pengguna")
		return
	}

	roleNames := make([]string, 0, len(userRoles))
	roleDTOs := make([]RoleDTO, 0, len(userRoles))
	for _, role := range userRoles {
		roleNames = append(roleNames, role.Name)
		roleDTOs = append(roleDTOs, toRoleDTO(role.Role))
	}
	if perms == nil {
		perms = make([]string, 0)
	}

	_ = h.respond(w, r, http.StatusOK, UserDetailResponse{
		User:        toUserDTO(&u, roleNames),
		Roles:       roleDTOs,
		Permissions: perms,
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

	// Sama seperti grantUserRole: tidak boleh mencetak peran di atas pelaku
	// lewat pembuatan pengguna. Dicek SEBELUM InTx supaya tidak ada user
	// yatim bila salah satu peran ditolak.
	for _, roleID := range req.RoleIDs {
		if msg, ditolak := h.peranDiAtasPelaku(ctx, actorID, roleID); ditolak {
			httpx.BadRequest(w, r, "rank_forbidden", msg)
			return
		}
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
	userRoles, err := h.rolesRepo.OfUser(ctx, createdUser.ID)
	if err != nil {
		mapRepoError(w, r, err, "peran pengguna")
		return
	}
	roleNames := make([]string, 0, len(userRoles))
	for _, role := range userRoles {
		roleNames = append(roleNames, role.Name)
	}
	_ = h.respond(w, r, http.StatusCreated, toUserDTO(&createdUser, roleNames))
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
		// Menonaktifkan/mengunci pemegang TERAKHIR Super Admin DITOLAK: sama
		// saja dengan menghapusnya — manajemen identitas terkunci — tetapi
		// lewat jalur yang tidak dijaga Revoke maupun deleteUser.
		// Hanya status non-aktif yang dijaga; mengaktifkan kembali selalu boleh.
		if identity.UserStatus(req.Status) != identity.UserStatusActive {
			if super, err := h.rolesRepo.GetByName(ctx, seed.RoleSuperAdmin); err == nil {
				if memegang, err := h.memegangPeran(ctx, id, super.ID); err == nil && memegang {
					if n, err := h.rolesRepo.CountHolders(ctx, super.ID); err == nil && n <= 1 {
						httpx.BadRequest(w, r, "last_superadmin_forbidden", "pengguna ini pemegang terakhir peran Super Admin; beri peran itu ke admin lain dulu sebelum menonaktifkannya")
						return
					}
				}
			}
		}
		_, err := h.usersRepo.SetStatus(ctx, id, identity.UserStatus(req.Status))
		if err != nil {
			mapRepoError(w, r, err, "status pengguna")
			return
		}
	}

	// Mengubah status akun SENDIRI menjadi non-aktif DITOLAK: salah klik atau
	// sesi curian langsung mengunci admin keluar tanpa jejak khusus — sama
	// bahayanya dengan self-delete yang sudah ditolak deleteUser.
	if req.Status != "" && identity.UserStatus(req.Status) != identity.UserStatusActive {
		if p, ok := auth.PrincipalFrom(ctx); ok && p != nil && p.User.ID == id {
			httpx.BadRequest(w, r, "self_disable_forbidden", "akun sendiri tidak bisa dinonaktifkan; minta admin lain yang melakukannya")
			return
		}
	}

	// Gagal baca-ulang setelah update berarti 500, BUKAN 200 dengan user
	// kosong: menjawab zero-value membuat operator mengira pengguna terhapus
	// datanya, dan peran/izin kosong disangka "tak punya hak".
	u, err := h.usersRepo.GetByID(ctx, id)
	if err != nil {
		mapRepoError(w, r, err, "pengguna")
		return
	}
	h.writeAudit(ctx, r, "update", "user", id, nil)
	userRoles, err := h.rolesRepo.OfUser(ctx, id)
	if err != nil {
		mapRepoError(w, r, err, "peran pengguna")
		return
	}
	roleNames := make([]string, 0, len(userRoles))
	for _, role := range userRoles {
		roleNames = append(roleNames, role.Name)
	}
	_ = h.respond(w, r, http.StatusOK, toUserDTO(&u, roleNames))
}

func (h *Handlers) deleteUser(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	id := chi.URLParam(r, "id")

	// Menghapus akun sendiri DITOLAK: lewat sesi curian ini berarti lockout
	// seketika, dan lewat klik yang salah berarti kehilangan akses admin tanpa
	// ada yang bisa mengembalikannya kecuali akses DB langsung.
	if p, ok := auth.PrincipalFrom(ctx); ok && p != nil && p.User.ID == id {
		httpx.BadRequest(w, r, "self_delete_forbidden", "akun sendiri tidak bisa dihapus; minta admin lain yang menghapusnya")
		return
	}
	// Menghapus satu-satunya pemegang Super Admin DITOLAK: tanpa ini,
	// deleteUser menjadi jalan memutar pencabutan terakhir yang sudah dijaga
	// Revoke — hapus usernya langsung, perannya ikut hilang via cascade.
	if super, err := h.rolesRepo.GetByName(ctx, seed.RoleSuperAdmin); err == nil {
		if memegang, err := h.memegangPeran(ctx, id, super.ID); err == nil && memegang {
			if n, err := h.rolesRepo.CountHolders(ctx, super.ID); err == nil && n <= 1 {
				httpx.BadRequest(w, r, "last_superadmin_forbidden", "pengguna ini pemegang terakhir peran Super Admin; beri peran itu ke admin lain dulu")
				return
			}
		}
	}

	if err := h.usersRepo.Delete(ctx, id); err != nil {
		mapRepoError(w, r, err, "pengguna")
		return
	}

	h.writeAudit(ctx, r, "delete", "user", id, nil)
	_ = h.respond(w, r, http.StatusOK, StatusResponse{Status: "deleted"})
}

// memegangPeran melaporkan apakah pengguna memegang satu peran. Error berarti
// "tidak diketahui" — pemanggil memperlakukannya sebagai bukan pemegang supaya
// permintaan tidak gagal karena pemeriksaan tambahan, sementara penegakan
// sesungguhnya tetap di lapisan repo.
func (h *Handlers) memegangPeran(ctx context.Context, userID, roleID string) (bool, error) {
	peran, err := h.rolesRepo.OfUser(ctx, userID)
	if err != nil {
		return false, err
	}
	for _, p := range peran {
		if p.ID == roleID {
			return true, nil
		}
	}
	return false, nil
}

// peranDiAtasPelaku melaporkan apakah peran target LEBIH berkuasa (rank lebih
// kecil) dari peran terkuat pelaku. Rank kecil = berkuasa (Super Admin = 0).
// Kegagalan membaca rank di kedua sisi berarti tolak — aman secara default:
// pemberian peran tidak boleh lolos karena pemeriksaannya yang rusak.
func (h *Handlers) peranDiAtasPelaku(ctx context.Context, actorID, roleID string) (string, bool) {
	target, err := h.rolesRepo.Get(ctx, roleID)
	if err != nil {
		return "peran target tidak ditemukan", true
	}
	if actorID == "" {
		return "identitas pemberi tidak diketahui", true
	}
	pelaku, err := h.rolesRepo.TopRank(ctx, actorID)
	if err != nil {
		return "peringkat pemberi tidak bisa dipastikan", true
	}
	if target.Role.Rank < pelaku {
		return "peran ini lebih berkuasa dari peran Anda; mintalah kepada pemegang peran yang setara atau lebih tinggi", true
	}
	return "", false
}

// superAdminID mengembalikan ID peran "Super Admin", atau "" bila tidak
// ditemukan. Kegagalan di sini berarti guard pemegang-terakhir Revoke
// dilewati (fail-open): revokeUserRole tetap menolak hapus-diri lewat
// lapisan lain, dan kegagalan baca peran adalah keadaan sementara yang
// tidak boleh mengunci pencabutan peran biasa.
func (h *Handlers) superAdminID(ctx context.Context) string {
	super, err := h.rolesRepo.GetByName(ctx, seed.RoleSuperAdmin)
	if err != nil {
		return ""
	}
	return super.ID
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

	// Pelaku tidak boleh memberi peran yang LEBIH berkuasa dari dirinya sendiri:
	// tanpa ini, siapa pun yang memegang roles:write (mis. peran kustom yang
	// diberi izin itu) bisa mencetak Super Admin untuk dirinya sendiri.
	// TopRank gagal (mis. pelaku tanpa peran) berarti tolak — aman secara default.
	if msg, ditolak := h.peranDiAtasPelaku(ctx, actorID, req.RoleID); ditolak {
		httpx.BadRequest(w, r, "rank_forbidden", msg)
		return
	}

	if err := h.rolesRepo.Grant(ctx, id, req.RoleID, actorID); err != nil {
		mapRepoError(w, r, err, "pemberian peran")
		return
	}
	h.writeAudit(ctx, r, "grant_role", "user", id, map[string]any{"role_id": req.RoleID})
	_ = h.respond(w, r, http.StatusOK, StatusResponse{Status: "granted"})
}

func (h *Handlers) revokeUserRole(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	id := chi.URLParam(r, "id")
	roleID := chi.URLParam(r, "role_id")

	if err := h.rolesRepo.Revoke(ctx, id, roleID, h.superAdminID(ctx)); err != nil {
		// Penolakan pemegang-terakhir punya pesannya sendiri: pesan generik
		// mapRepoError ("sudah ada atau melanggar keunikan") menyesatkan di
		// sini karena tidak ada yang "sudah ada".
		if errors.Is(err, repo.ErrConflict) {
			httpx.BadRequest(w, r, "last_holder_forbidden", "pengguna ini pemegang terakhir peran Super Admin; beri peran itu ke pengguna lain dulu sebelum mencabutnya")
			return
		}
		mapRepoError(w, r, err, "pencabutan peran")
		return
	}

	h.writeAudit(ctx, r, "revoke_role", "user", id, map[string]any{"role_id": roleID})
	_ = h.respond(w, r, http.StatusOK, StatusResponse{Status: "revoked"})
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

	// Pencabutan sesi adalah bagian INVARIANSI reset paksa: password sudah
	// diganti tetapi sesi bajakan tetap hidup bila ini gagal diam-diam.
	// Kegagalannya dilaporkan eksplisit (500 + jumlah sesi yang BERHASIL
	// dicabut) supaya operator tahu sesi mana yang masih hidup.
	revoked, err := h.sessionsRepo.RevokeAllOfUser(ctx, id)
	if err != nil {
		h.logger.ErrorContext(ctx, "reset password berhasil tetapi pencabutan sesi gagal",
			"sesi_tercabut", revoked, "user_id", id, "error", err)
		httpx.WriteError(w, r, http.StatusInternalServerError, httpx.ErrTypeAPI,
			"revoke_failed", "password sudah diganti tetapi pencabutan sesi gagal; cabut manual lewat revoke-sessions")
		return
	}

	h.writeAudit(ctx, r, "force_reset_password", "user", id, map[string]any{"sesi_tercabut": revoked})
	_ = h.respond(w, r, http.StatusOK, StatusResponse{Status: "password_reset"})
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
	_ = h.respond(w, r, http.StatusOK, StatusCountResponse{Status: "revoked", Count: int64(count)})
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

	rolesDTO := make([]RoleDTO, 0, len(roles))
	for _, rl := range roles {
		rolesDTO = append(rolesDTO, toRoleDTO(rl))
	}

	_ = h.respond(w, r, http.StatusOK, ListEnvelope[RoleDTO]{
		Items: rolesDTO,
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

	_ = h.respond(w, r, http.StatusOK, toRoleDetailDTO(&detail))
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
	_ = h.respond(w, r, http.StatusCreated, toRoleDTO(createdRole))
}

func (h *Handlers) deleteRole(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	id := chi.URLParam(r, "id")

	// Peran sistem tidak bisa dihapus lewat API: tolak eksplisit dengan 400
	// yang jelas, bukan mengandalkan penolakan repo di bawah.
	if detail, err := h.rolesRepo.Get(ctx, id); err == nil && detail.Role.IsSystem {
		httpx.BadRequest(w, r, "system_role_forbidden", "peran sistem tidak boleh dihapus")
		return
	}

	// Peran kustom dihapus BERSAMA pemegangnya dalam satu transaksi: Revoke
	// menolak pencabutan pemegang TERAKHIR (guard anti-lockout), dan baris
	// user_roles memakai ON DELETE RESTRICT — tanpa pelepasan ini, peran
	// kustom yang pernah dipakai satu orang pun tidak bisa dihapus lewat API
	// (revoke -> 409, delete -> FK error). Guard Revoke tetap berlaku untuk
	// peran yang TETAP ADA; yang dihapus di sini kehilangan maknanya sehingga
	// penolakan pemegang-terakhir tidak lagi relevan.
	err := repo.InTx(ctx, h.pool, func(q repo.Querier) error {
		txRoles := identity.NewRoles(q)
		if _, err := q.Exec(ctx, `delete from user_roles where role_id = $1`, id); err != nil {
			return err
		}
		return txRoles.Delete(ctx, id)
	})
	if err != nil {
		mapRepoError(w, r, err, "peran")
		return
	}

	h.writeAudit(ctx, r, "delete", "role", id, nil)
	_ = h.respond(w, r, http.StatusOK, StatusResponse{Status: "deleted"})
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
	_ = h.respond(w, r, http.StatusOK, StatusResponse{Status: "updated"})
}

func (h *Handlers) listPermissions(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	perms, err := h.rolesRepo.ListPermissions(ctx)
	if err != nil {
		mapRepoError(w, r, err, "katalog izin")
		return
	}

	permDTOs := make([]PermissionDTO, 0, len(perms))
	for _, p := range perms {
		permDTOs = append(permDTOs, toPermissionDTO(p))
	}

	_ = h.respond(w, r, http.StatusOK, ListEnvelope[PermissionDTO]{
		Items: permDTOs,
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

	sessionDTOs := make([]SessionDTO, 0, len(sessions))
	for _, s := range sessions {
		sessionDTOs = append(sessionDTOs, toSessionDTO(s))
	}

	_ = h.respond(w, r, http.StatusOK, ListEnvelope[SessionDTO]{
		Items: sessionDTOs,
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
	_ = h.respond(w, r, http.StatusOK, StatusResponse{Status: "revoked"})
}
