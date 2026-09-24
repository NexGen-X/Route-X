package worker

import (
	"context"
	"log/slog"
	"time"

	"github.com/jackc/pgx/v5/pgxpool"

	"github.com/NexGen-X/Route-X/internal/database/repo"
	"github.com/NexGen-X/Route-X/internal/database/repo/upstream"
	"github.com/NexGen-X/Route-X/internal/oauth"
	"github.com/NexGen-X/Route-X/internal/security"
)

// OAuthRefreshWorker adalah background worker berkala yang memantau dan me-refresh
// access token provider OAuth (seperti Google Antigravity) sebelum kedaluwarsa.
type OAuthRefreshWorker struct {
	pool        *pgxpool.Pool
	oauthRepo   *upstream.OAuthRepo
	credRepo    *upstream.CredentialRepo
	egressRepo  *upstream.EgressRepo
	oauthClient *oauth.GoogleOAuthClient
	logger      *slog.Logger
	threshold   time.Duration
}

// NewOAuthRefreshWorker membuat instance baru OAuthRefreshWorker.
func NewOAuthRefreshWorker(
	pool *pgxpool.Pool,
	oauthRepo *upstream.OAuthRepo,
	credRepo *upstream.CredentialRepo,
	egressRepo *upstream.EgressRepo,
	oauthClient *oauth.GoogleOAuthClient,
	logger *slog.Logger,
) *OAuthRefreshWorker {
	if logger == nil {
		logger = slog.Default()
	}
	if oauthClient == nil {
		oauthClient = oauth.NewGoogleOAuthClient(nil)
	}
	return &OAuthRefreshWorker{
		pool:        pool,
		oauthRepo:   oauthRepo,
		credRepo:    credRepo,
		egressRepo:  egressRepo,
		oauthClient: oauthClient,
		logger:      logger,
		threshold:   15 * time.Minute, // Refresh token yang tersisa <= 15 menit
	}
}

func (w *OAuthRefreshWorker) Name() string { return "oauth_token_refresher" }

// Run memindai seluruh sesi OAuth yang hampir kedaluwarsa dan memperbaruinya ke server Google.
func (w *OAuthRefreshWorker) Run(ctx context.Context) error {
	unlock, ok, err := TryAdvisoryLock(ctx, w.pool, LockOAuthRefresh)
	if err != nil {
		return repo.Err("advisory lock oauth refresh", err)
	}
	if !ok {
		return ErrJobSkipped
	}
	defer unlock()

	sessions, err := w.oauthRepo.ListExpiringOAuthSessions(ctx, w.threshold)
	if err != nil {
		w.logger.Error("gagal memindai sesi oauth kedaluwarsa", "error", err)
		return err
	}

	if len(sessions) == 0 {
		return nil
	}

	w.logger.Info("menemukan sesi oauth yang perlu diperbarui", "count", len(sessions))

	for _, sess := range sessions {
		var proxyURL security.Secret
		if sess.EgressPoolID != nil && *sess.EgressPoolID != "" && w.egressRepo != nil {
			pURL, err := w.egressRepo.ProxyURL(ctx, *sess.EgressPoolID)
			if err != nil {
				w.logger.Warn("gagal mengambil URL proxy untuk refresh sesi oauth",
					"session_id", sess.ID, "egress_pool_id", *sess.EgressPoolID, "error", err)
			} else {
				proxyURL = pURL
			}
		}

		res, err := w.oauthClient.RefreshAccessTokenWithProxy(ctx, sess.RefreshToken.Reveal(), sess.ClientID, "", proxyURL)
		if err != nil {
			errStr := err.Error()
			_ = w.oauthRepo.UpdateRefreshStatus(ctx, sess.ID, &errStr)
			w.logger.Warn("gagal memperbarui token oauth upstream",
				"session_id", sess.ID,
				"provider_id", sess.ProviderID,
				"email", sess.AccountEmail,
				"error", err,
			)
			continue
		}

		expiresAt := time.Now().Add(time.Duration(res.ExpiresIn) * time.Second)
		if err := w.credRepo.UpdateSecret(ctx, sess.CredentialID, res.AccessToken, &expiresAt); err != nil {
			errStr := "gagal menyimpan token baru: " + err.Error()
			_ = w.oauthRepo.UpdateRefreshStatus(ctx, sess.ID, &errStr)
			w.logger.Error("gagal menyimpan token access oauth baru ke database",
				"session_id", sess.ID,
				"error", err,
			)
			continue
		}

		_ = w.oauthRepo.UpdateRefreshStatus(ctx, sess.ID, nil)
		w.logger.Info("berhasil memperbarui token oauth upstream",
			"session_id", sess.ID,
			"provider_id", sess.ProviderID,
			"email", sess.AccountEmail,
			"expires_in_sec", res.ExpiresIn,
		)
	}

	return nil
}
