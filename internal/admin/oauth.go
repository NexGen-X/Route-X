package admin

import (
	"fmt"
	"net/http"
	"strings"
	"time"

	"github.com/go-chi/chi/v5"

	"github.com/NexGen-X/Route-X/internal/database/repo/upstream"
	"github.com/NexGen-X/Route-X/internal/httpx"
	"github.com/NexGen-X/Route-X/internal/security"
)

type exchangeOAuthCodeReq struct {
	Code         string  `json:"code"`
	RedirectURI  string  `json:"redirect_uri"`
	EgressPoolID *string `json:"egress_pool_id"`
}

type refreshOAuthSessionReq struct {
	SessionID string `json:"session_id"`
}

type toggleOAuthSessionReq struct {
	Enabled bool `json:"enabled"`
}

type setCredentialStrategyReq struct {
	Strategy string `json:"strategy"`
}

// exchangeOAuthCode menukarkan kode otorisasi Google OAuth via Fallback,
// mendaftarkan kredensial baru, mengaitkan sesi OAuth, dan mengupdate provider.
func (h *Handlers) exchangeOAuthCode(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	providerID := chi.URLParam(r, "id")

	p, err := h.authorizeProviderAccess(ctx, providerID)
	if err != nil {
		mapRepoError(w, r, err, "provider")
		return
	}

	var req exchangeOAuthCodeReq
	if err := decodeJSON(r, &req); err != nil {
		httpx.BadRequest(w, r, "invalid_json", err.Error())
		return
	}

	if req.Code == "" {
		httpx.BadRequest(w, r, "missing_code", "Kode otorisasi tidak boleh kosong")
		return
	}

	var proxyURL security.Secret
	var poolID *string
	if req.EgressPoolID != nil && *req.EgressPoolID != "" {
		poolID = req.EgressPoolID
		if h.egressRepo != nil {
			pURL, err := h.egressRepo.ProxyURL(ctx, *poolID)
			if err != nil {
				httpx.BadRequest(w, r, "invalid_egress_pool", "Gagal memuat URL proxy egress pool: "+err.Error())
				return
			}
			proxyURL = pURL
		}
	}

	// Backend mengatur penukaran langsung ke Google Token Endpoint (lewat proxy bila ditentukan)
	tokenRes, err := h.oauthClient.ExchangeAuthCodeWithProxy(ctx, req.Code, req.RedirectURI, "", "", proxyURL)
	if err != nil {
		httpx.BadRequest(w, r, "oauth_exchange_failed", err.Error())
		return
	}

	email := tokenRes.AccountEmail
	if email == "" {
		email = "google-account"
	}
	label := "oauth:" + email
	expiresAt := time.Now().Add(time.Duration(tokenRes.ExpiresIn) * time.Second)

	// Periksa apakah kredensial dengan label yang sama sudah ada pada provider ini
	var credID string
	existingCreds, err := h.credentialRepo.List(ctx, providerID)
	if err == nil {
		for _, c := range existingCreds {
			if c.Label == label {
				credID = c.ID
				break
			}
		}
	}

	if credID != "" {
		// Update rahasia yang ada
		if err := h.credentialRepo.UpdateSecret(ctx, credID, tokenRes.AccessToken, &expiresAt); err != nil {
			httpx.BadRequest(w, r, "credential_update_failed", "Gagal memperbarui kredensial provider: "+err.Error())
			return
		}
		if poolID != nil {
			_ = h.credentialRepo.SetCredentialEgressPool(ctx, credID, poolID)
		}
	} else {
		// Buat kredensial baru di provider_credentials
		credMeta, err := h.credentialRepo.Create(ctx, upstream.CreateCredentialParams{
			ProviderID:   providerID,
			Label:        label,
			Secret:       tokenRes.AccessToken,
			ExpiresAt:    &expiresAt,
			EgressPoolID: poolID,
		})
		if err != nil {
			httpx.BadRequest(w, r, "credential_create_failed", "Gagal menyimpan kredensial provider: "+err.Error())
			return
		}
		credID = credMeta.ID
	}

	// Simpan ke provider_oauth_sessions
	sessionMeta, err := h.oauthRepo.UpsertSession(ctx, upstream.UpsertOAuthSessionParams{
		ProviderID:   providerID,
		CredentialID: credID,
		AccountEmail: email,
		AccountName:  &tokenRes.AccountName,
		Scopes:       tokenRes.Scopes,
		RefreshToken: tokenRes.RefreshToken,
	})
	if err != nil {
		httpx.BadRequest(w, r, "session_save_failed", "Gagal menyimpan sesi oauth: "+err.Error())
		return
	}

	// Otomatis sinkronkan kind & base_url ke endpoint resmi Gemini
	if p.Kind != "google" || !strings.Contains(p.BaseURL, "googleapis.com") {
		googleBaseURL := "https://generativelanguage.googleapis.com"
		googleKind := "google"
		_, _ = h.providerRepo.Update(ctx, providerID, upstream.UpdateProviderParams{
			Kind:    &googleKind,
			BaseURL: &googleBaseURL,
		})
	}

	h.writeAudit(ctx, r, "connect_oauth", "provider", providerID, map[string]any{
		"account_email": email,
		"session_id":    sessionMeta.ID,
	})

	_ = h.respond(w, r, http.StatusOK, OAuthExchangeResponseDTO{
		Status:       "connected",
		AccountEmail: email,
		AccountName:  tokenRes.AccountName,
		ExpiresAt:    expiresAt.Format(time.RFC3339),
		Session:      sessionMeta,
	})
}

// listOAuthSessions mendaftar seluruh akun OAuth yang terhubung pada provider ini.
func (h *Handlers) listOAuthSessions(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	providerID := chi.URLParam(r, "id")

	if _, err := h.authorizeProviderAccess(ctx, providerID); err != nil {
		mapRepoError(w, r, err, "provider")
		return
	}

	sessions, err := h.oauthRepo.ListSessionsByProviderID(ctx, providerID)
	if err != nil {
		mapRepoError(w, r, err, "oauth_session")
		return
	}

	_ = h.respond(w, r, http.StatusOK, OAuthSessionsResponseDTO{
		Items: sessions,
	})
}

// refreshOAuthSession memicu pembaharuan token manual untuk satu sesi atau seluruh sesi provider.
func (h *Handlers) refreshOAuthSession(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	providerID := chi.URLParam(r, "id")

	if _, err := h.authorizeProviderAccess(ctx, providerID); err != nil {
		mapRepoError(w, r, err, "provider")
		return
	}

	var req refreshOAuthSessionReq
	_ = decodeJSON(r, &req)

	var sessionsToRefresh []string
	if req.SessionID != "" {
		sessionsToRefresh = append(sessionsToRefresh, req.SessionID)
	} else {
		all, err := h.oauthRepo.ListSessionsByProviderID(ctx, providerID)
		if err != nil {
			mapRepoError(w, r, err, "oauth_session")
			return
		}
		for _, s := range all {
			if s.Enabled {
				sessionsToRefresh = append(sessionsToRefresh, s.ID)
			}
		}
	}

	refreshedCount := 0
	var lastErr error

	for _, sessID := range sessionsToRefresh {
		activeSess, err := h.oauthRepo.GetActiveSession(ctx, sessID)
		if err != nil {
			lastErr = err
			continue
		}

		var proxyURL security.Secret
		if activeSess.EgressPoolID != nil && *activeSess.EgressPoolID != "" && h.egressRepo != nil {
			pURL, err := h.egressRepo.ProxyURL(ctx, *activeSess.EgressPoolID)
			if err != nil {
				h.logger.WarnContext(ctx, "gagal memuat proxy URL untuk refresh oauth manual",
					"session_id", sessID, "egress_pool_id", *activeSess.EgressPoolID, "error", err)
			} else {
				proxyURL = pURL
			}
		}

		res, err := h.oauthClient.RefreshAccessTokenWithProxy(ctx, activeSess.RefreshToken.Reveal(), activeSess.ClientID, "", proxyURL)
		if err != nil {
			errStr := err.Error()
			_ = h.oauthRepo.UpdateRefreshStatus(ctx, sessID, &errStr)
			lastErr = err
			continue
		}

		expiresAt := time.Now().Add(time.Duration(res.ExpiresIn) * time.Second)
		if err := h.credentialRepo.UpdateSecret(ctx, activeSess.CredentialID, res.AccessToken, &expiresAt); err != nil {
			errStr := err.Error()
			_ = h.oauthRepo.UpdateRefreshStatus(ctx, sessID, &errStr)
			lastErr = err
			continue
		}

		_ = h.oauthRepo.UpdateRefreshStatus(ctx, sessID, nil)
		refreshedCount++
	}

	if refreshedCount == 0 && lastErr != nil {
		httpx.BadRequest(w, r, "refresh_failed", fmt.Sprintf("Gagal me-refresh token: %v", lastErr))
		return
	}

	_ = h.respond(w, r, http.StatusOK, OAuthRefreshResponseDTO{
		Status:         "refreshed",
		RefreshedCount: refreshedCount,
	})
}

// toggleOAuthSession menyalakan atau mematikan salah satu akun OAuth tanpa menghapusnya.
func (h *Handlers) toggleOAuthSession(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	providerID := chi.URLParam(r, "id")
	sessionID := chi.URLParam(r, "session_id")

	if _, err := h.authorizeProviderAccess(ctx, providerID); err != nil {
		mapRepoError(w, r, err, "provider")
		return
	}

	var req toggleOAuthSessionReq
	if err := decodeJSON(r, &req); err != nil {
		httpx.BadRequest(w, r, "invalid_json", err.Error())
		return
	}

	if err := h.oauthRepo.SetEnabled(ctx, sessionID, req.Enabled); err != nil {
		mapRepoError(w, r, err, "oauth_session")
		return
	}

	h.writeAudit(ctx, r, "toggle_oauth_session", "provider", providerID, map[string]any{
		"session_id": sessionID,
		"enabled":    req.Enabled,
	})

	_ = h.respond(w, r, http.StatusOK, OAuthToggleResponseDTO{
		Status:    "updated",
		SessionID: sessionID,
		Enabled:   req.Enabled,
	})
}

// deleteOAuthSession menghapus sesi OAuth dan kredensial terkait.
func (h *Handlers) deleteOAuthSession(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	providerID := chi.URLParam(r, "id")
	sessionID := chi.URLParam(r, "session_id")

	if _, err := h.authorizeProviderAccess(ctx, providerID); err != nil {
		mapRepoError(w, r, err, "provider")
		return
	}

	if err := h.oauthRepo.Delete(ctx, sessionID); err != nil {
		mapRepoError(w, r, err, "oauth_session")
		return
	}

	h.writeAudit(ctx, r, "delete_oauth_session", "provider", providerID, map[string]any{
		"session_id": sessionID,
	})

	w.WriteHeader(http.StatusNoContent)
}

// setProviderCredentialStrategy mengatur strategi rotasi antar-akun/kredensial untuk provider ini.
func (h *Handlers) setProviderCredentialStrategy(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	providerID := chi.URLParam(r, "id")

	if _, err := h.authorizeProviderAccess(ctx, providerID); err != nil {
		mapRepoError(w, r, err, "provider")
		return
	}

	var req setCredentialStrategyReq
	if err := decodeJSON(r, &req); err != nil {
		httpx.BadRequest(w, r, "invalid_json", err.Error())
		return
	}

	req.Strategy = strings.TrimSpace(strings.ToLower(req.Strategy))
	if req.Strategy != "round_robin" && req.Strategy != "priority" {
		httpx.BadRequest(w, r, "invalid_strategy", "Strategi harus 'round_robin' atau 'priority'")
		return
	}

	p, err := h.providerRepo.SetCredentialStrategy(ctx, providerID, req.Strategy)
	if err != nil {
		mapRepoError(w, r, err, "provider")
		return
	}

	h.writeAudit(ctx, r, "update_credential_strategy", "provider", providerID, map[string]any{
		"credential_strategy": req.Strategy,
	})

	_ = h.respond(w, r, http.StatusOK, toProviderDTO(p))
}
