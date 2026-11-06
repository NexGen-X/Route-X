package worker

import (
	"context"
	"log/slog"
	"time"

	"github.com/jackc/pgx/v5/pgxpool"

	"github.com/NexGen-X/Route-X/internal/database/repo"
	"github.com/NexGen-X/Route-X/internal/database/repo/upstream"
	"github.com/NexGen-X/Route-X/internal/oauth"
)

// OAuthRefreshWorker adalah background worker berkala yang memantau dan me-refresh
// access token provider OAuth (seperti Google Antigravity) sebelum kedaluwarsa.
type OAuthRefreshWorker struct {
	pool        *pgxpool.Pool
	oauthRepo   *upstream.OAuthRepo
	egressRepo  *upstream.EgressRepo
	oauthClient *oauth.GoogleOAuthClient
	logger      *slog.Logger
	threshold   time.Duration
}

// NewOAuthRefreshWorker membuat instance baru OAuthRefreshWorker.
func NewOAuthRefreshWorker(
	pool *pgxpool.Pool,
	oauthRepo *upstream.OAuthRepo,
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
		egressRepo:  egressRepo,
		oauthClient: oauthClient,
		logger:      logger,
		// Ambang 30 menit terhadap interval putaran 10 menit: versi lama
		// memakai 15 menit tanpa percobaan ulang dalam putaran, sehingga
		// satu kegagalan transien menggeser refresh berikutnya ke SETELAH
		// kedaluwarsa dan membuka jendela mati (audit3 V-EO-16). Ambang
		// yang lebih lebar dari dua kali interval + percobaan ulang di
		// RefreshSesiOAuth menutup jendela itu.
		threshold: 30 * time.Minute,
	}
}

// deps menyusun dependensi refresh bersama dari field worker.
func (w *OAuthRefreshWorker) deps() OAuthRefreshDeps {
	return OAuthRefreshDeps{
		OAuth:  w.oauthRepo,
		Egress: w.egressRepo,
		Client: w.oauthClient,
		Logger: w.logger,
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

	// Logika per sesi tinggal di RefreshSesiOAuth (oauth_refresh_sesi.go),
	// dipakai bersama refresh manual admin dan refresh on-demand jalur
	// request — dulu ketiganya (yang ketiga: tidak ada) berperilaku beda.
	deps := w.deps()
	for _, sess := range sessions {
		if err := RefreshSesiOAuth(ctx, deps, sess); err != nil {
			w.logger.Warn("gagal memperbarui token oauth upstream",
				"session_id", sess.ID,
				"provider_id", sess.ProviderID,
				"email", sess.AccountEmail,
				"error", err,
			)
			continue
		}
		w.logger.Info("berhasil memperbarui token oauth upstream",
			"session_id", sess.ID,
			"provider_id", sess.ProviderID,
			"email", sess.AccountEmail,
		)
	}

	return nil
}
