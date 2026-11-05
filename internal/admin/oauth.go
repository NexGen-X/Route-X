package admin

import (
	"net/http"
	"strings"
	"time"

	"github.com/go-chi/chi/v5"

	"github.com/NexGen-X/Route-X/internal/database/repo"
	"github.com/NexGen-X/Route-X/internal/database/repo/upstream"
	"github.com/NexGen-X/Route-X/internal/httpx"
	"github.com/NexGen-X/Route-X/internal/security"
	"github.com/NexGen-X/Route-X/internal/worker"
)

type exchangeOAuthCodeReq struct {
	Code         string  `json:"code"`
	RedirectURI  string  `json:"redirect_uri"`
	EgressPoolID *string `json:"egress_pool_id"`
}

type refreshOAuthSessionReq struct {
	SessionID string `json:"session_id"`
}

// toggleOAuthSessionReq memakai *bool untuk alasan yang sama dengan
// toggleEnabledReq (B-088): enabled yang absen wajib ditolak 400, bukan
// dibaca sebagai false.
type toggleOAuthSessionReq struct {
	Enabled *bool `json:"enabled"`
}

type setCredentialStrategyReq struct {
	Strategy string `json:"strategy"`
}

// exchangeOAuthCode menukarkan kode otorisasi Google OAuth via Fallback,
// mendaftarkan kredensial baru, mengaitkan sesi OAuth, dan mengupdate provider.
func (h *Handlers) exchangeOAuthCode(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	providerID := chi.URLParam(r, "id")

	// Penjaga nil (audit F5): dependensi opsional yang tidak terpasang
	// harus menjadi jawaban bersih (kini 503, audit3 V-EM-05), bukan panic per-request.
	if h.oauthClient == nil {
		tidakTersedia(w, r, "oauth_unavailable", "Klien OAuth tidak tersedia pada instalasi ini")
		return
	}

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
				// Lewat mapRepoError: teks error repo tidak pernah terpantul,
				// dan pool yang tidak ada menjadi 404, bukan 400 generik.
				mapRepoError(w, r, err, "egress pool")
				return
			}
			proxyURL = pURL
		}
	}

	// Backend mengatur penukaran langsung ke Google Token Endpoint (lewat proxy bila ditentukan)
	tokenRes, err := h.oauthClient.ExchangeAuthCodeWithProxy(ctx, req.Code, req.RedirectURI, "", "", proxyURL)
	if err != nil {
		// Kegagalan dependensi eksternal: 502 + pesan umum. Teks aslinya adalah
		// string *url.Error Go utuh (URL endpoint token + rincian timeout
		// internal) dan hanya boleh sampai ke log (audit3 V-EM-01).
		h.gagalHulu(w, r, "penukaran kode otorisasi oauth gagal", "oauth_exchange_failed",
			"Gagal menukarkan kode otorisasi dengan penyedia OAuth; periksa koneksi ke penyedia lalu coba lagi", err)
		return
	}

	// Masa berlaku yang tidak sah dari penyedia berarti kredensial lahir
	// kedaluwarsa dan tidak akan pernah terpilih — tolak di sini dengan
	// pesan jelas alih-alih menyimpan kredensial mati (audit3 V-EO-20).
	if tokenRes.ExpiresIn <= 0 {
		h.gagalHulu(w, r, "respons token oauth tidak memuat expires_in yang sah", "oauth_exchange_failed",
			"Penyedia OAuth mengembalikan token tanpa masa berlaku yang sah; coba lagi", nil)
		return
	}

	email := tokenRes.AccountEmail
	if email == "" {
		email = "google-account"
	}
	label := "oauth:" + email
	expiresAt := time.Now().Add(time.Duration(tokenRes.ExpiresIn) * time.Second)

	// Periksa apakah kredensial dengan label yang sama sudah ada pada provider ini.
	// Error List diteruskan (audit F5): versi lama memperlakukan kegagalan
	// baca sebagai "belum ada kredensial" dan membuat duplikat.
	var credID string
	existingCreds, err := h.credentialRepo.List(ctx, providerID)
	if err != nil {
		mapRepoError(w, r, err, "kredensial provider")
		return
	}
	for _, c := range existingCreds {
		if c.Label == label {
			credID = c.ID
			break
		}
	}

	if credID != "" {
		// Update rahasia yang ada
		if err := h.credentialRepo.UpdateSecret(ctx, credID, tokenRes.AccessToken, &expiresAt); err != nil {
			mapRepoError(w, r, err, "kredensial provider")
			return
		}
		if poolID != nil {
			// Pool yang diminta pengguna WAJIB terpasang atau handler gagal
			// (audit F5): menelannya berarti trafik OAuth keluar lewat
			// jalur lain dari yang dikonfigurasi, tanpa sinyal apa pun.
			if err := h.credentialRepo.SetCredentialEgressPool(ctx, credID, poolID); err != nil {
				mapRepoError(w, r, err, "kredensial provider")
				return
			}
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
			mapRepoError(w, r, err, "kredensial provider")
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
		mapRepoError(w, r, err, "sesi oauth")
		return
	}

	// Resolusi project Code Assist (B-157): sesi yang baru terhubung
	// langsung diselesaikan project-nya lewat loadCodeAssist dan disimpan,
	// agar daftar model provider bersifat project-scoped sejak sync
	// pertama. Kegagalan di sini TIDAK menggagalkan koneksi yang sudah
	// berhasil — sesi tetap "connected" dan resolusi akan dicoba lagi
	// secara malas pada pemakaian berikutnya (sync/factory); kegagalannya
	// tercatat, bukan ditelan.
	if project, perr := h.oauthClient.LoadCodeAssistProject(ctx, tokenRes.AccessToken.Reveal(), proxyURL); perr != nil {
		h.logger.WarnContext(ctx, "resolusi project code assist setelah exchange oauth gagal; dicoba lagi pada pemakaian berikutnya",
			"provider_id", providerID, "session_id", sessionMeta.ID, "error", perr)
	} else if project != "" {
		if uerr := h.oauthRepo.UpdateCodeAssistProject(ctx, sessionMeta.ID, project); uerr != nil {
			h.logger.WarnContext(ctx, "gagal menyimpan project code assist setelah exchange oauth",
				"provider_id", providerID, "session_id", sessionMeta.ID, "error", uerr)
		}
	}

	// Otomatis sinkronkan kind & base_url ke endpoint resmi Gemini.
	// Kegagalan dicatat keras (audit F5): sesi sudah tersimpan, jadi handler
	// tetap "connected", tetapi operator harus tahu sinkronisasi provider
	// tidak terjadi — bukan ditelan senyap.
	if p.Kind != "google" || !strings.Contains(p.BaseURL, "googleapis.com") {
		googleBaseURL := "https://generativelanguage.googleapis.com"
		googleKind := "google"
		if _, err := h.providerRepo.Update(ctx, providerID, upstream.UpdateProviderParams{
			Kind:    &googleKind,
			BaseURL: &googleBaseURL,
		}); err != nil {
			h.logger.ErrorContext(ctx, "gagal menyinkronkan kind/base_url provider google setelah oauth", "provider_id", providerID, "error", err)
		}
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

	// Daftar kosong harus ter serialisasi sebagai [] bukan null (B-116
	// butir 2): repo mengembalikan nil untuk provider tanpa sesi, dan
	// klien berhak membedakan "belum ada sesi" dari "field hilang".
	if sessions == nil {
		sessions = make([]*upstream.OAuthSessionMeta, 0)
	}

	_ = h.respond(w, r, http.StatusOK, OAuthSessionsResponseDTO{
		Items: sessions,
	})
}

// refreshOAuthSession memicu pembaharuan token manual untuk satu sesi atau seluruh sesi provider.
func (h *Handlers) refreshOAuthSession(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	providerID := chi.URLParam(r, "id")

	if h.oauthClient == nil {
		tidakTersedia(w, r, "oauth_unavailable", "Klien OAuth tidak tersedia pada instalasi ini")
		return
	}

	if _, err := h.authorizeProviderAccess(ctx, providerID); err != nil {
		mapRepoError(w, r, err, "provider")
		return
	}

	var req refreshOAuthSessionReq
	_ = decodeJSON(r, &req)

	var sessionsToRefresh []string
	if req.SessionID != "" {
		// Sesi yang disebut eksplisit WAJIB milik provider di URL dan dalam
		// keadaan aktif: versi lama me-refresh sesi provider mana pun (dan
		// sesi nonaktif) hanya bermodal ID-nya (audit3 V-EO-22).
		meta, err := h.oauthRepo.GetSession(ctx, req.SessionID)
		if err != nil {
			mapRepoError(w, r, err, "oauth_session")
			return
		}
		if meta.ProviderID != providerID {
			mapRepoError(w, r, repo.ErrNotFound, "oauth_session")
			return
		}
		if !meta.Enabled {
			httpx.BadRequest(w, r, "session_disabled", "Sesi OAuth nonaktif tidak bisa di-refresh; aktifkan dulu")
			return
		}
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

	// Kunci advisory yang sama dengan worker berkala dan penyegar
	// on-demand: refresh manual tidak boleh balapan dengan keduanya atas
	// sesi yang sama (audit3 V-EO-19).
	unlock, terkunci, err := worker.TryAdvisoryLock(ctx, h.pool, worker.LockOAuthRefresh)
	if err != nil {
		h.gagalInternal(w, r, "gagal mengambil kunci refresh oauth", err)
		return
	}
	if !terkunci {
		httpx.WriteError(w, r, http.StatusConflict, httpx.ErrTypeAPI, "refresh_in_progress",
			"Refresh OAuth lain sedang berjalan; coba lagi sebentar")
		return
	}
	defer unlock()

	refreshedCount := 0
	var lastErr error

	// Logika per sesi satu-satunya ada di worker.RefreshSesiOAuth: egress
	// gagal-keras, percobaan ulang transien, validasi expires_in, refresh
	// token rotasi tersimpan, dan penonaktifan saat ditolak permanen
	// (audit3 V-EO-17 s.d. V-EO-21).
	deps := worker.OAuthRefreshDeps{
		OAuth:  h.oauthRepo,
		Creds:  h.credentialRepo,
		Egress: h.egressRepo,
		Client: h.oauthClient,
		Logger: h.logger,
	}
	for _, sessID := range sessionsToRefresh {
		activeSess, err := h.oauthRepo.GetActiveSession(ctx, sessID)
		if err != nil {
			lastErr = err
			continue
		}

		if err := worker.RefreshSesiOAuth(ctx, deps, activeSess); err != nil {
			lastErr = err
			continue
		}
		refreshedCount++
	}

	if refreshedCount == 0 && lastErr != nil {
		// Sama seperti penukaran kode: sebab asli (error jaringan/token dari
		// penyedia) hanya untuk log; klien menerima 502 berkode tetap.
		h.gagalHulu(w, r, "refresh token oauth gagal untuk semua sesi", "refresh_failed",
			"Gagal me-refresh token dari penyedia OAuth; periksa koneksi ke penyedia lalu coba lagi", lastErr)
		return
	}

	_ = h.respond(w, r, http.StatusOK, OAuthRefreshResponseDTO{
		Status:         "refreshed",
		RefreshedCount: refreshedCount,
	})
}

// pastikanSesiMilikProvider memverifikasi sesi yang disebut URL benar-benar
// milik provider di URL yang sama. Mengembalikan false (dan sudah menulis
// jawabannya) bila tidak: sesi provider lain dijawab 404 agar keberadaannya
// tidak terkonfirmasi lewat URL provider yang salah (audit3 V-EO-22).
func (h *Handlers) pastikanSesiMilikProvider(w http.ResponseWriter, r *http.Request, providerID, sessionID string) bool {
	meta, err := h.oauthRepo.GetSession(r.Context(), sessionID)
	if err != nil {
		mapRepoError(w, r, err, "oauth_session")
		return false
	}
	if meta.ProviderID != providerID {
		mapRepoError(w, r, repo.ErrNotFound, "oauth_session")
		return false
	}
	return true
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
	if req.Enabled == nil {
		tolakToggle(w, r, errEnabledAbsen)
		return
	}
	enabled := *req.Enabled

	if !h.pastikanSesiMilikProvider(w, r, providerID, sessionID) {
		return
	}

	if err := h.oauthRepo.SetEnabled(ctx, sessionID, enabled); err != nil {
		mapRepoError(w, r, err, "oauth_session")
		return
	}

	h.writeAudit(ctx, r, "toggle_oauth_session", "provider", providerID, map[string]any{
		"session_id": sessionID,
		"enabled":    enabled,
	})

	_ = h.respond(w, r, http.StatusOK, OAuthToggleResponseDTO{
		Status:    "updated",
		SessionID: sessionID,
		Enabled:   enabled,
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

	if !h.pastikanSesiMilikProvider(w, r, providerID, sessionID) {
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
