package worker

import (
	"context"
	"log/slog"
	"time"

	"github.com/jackc/pgx/v5/pgxpool"

	"github.com/NexGen-X/Route-X/internal/database/repo/upstream"
	"github.com/NexGen-X/Route-X/internal/oauth"
)

// ambangPenyegarOnDemand adalah sisa masa berlaku maksimum yang memicu
// refresh on-demand dari jalur request — sama dengan ambang worker berkala.
const ambangPenyegarOnDemand = 30 * time.Minute

// PenyegarOAuth menyediakan refresh token OAuth ON-DEMAND untuk jalur
// request (gateway.Factory).
//
// Dulu tidak ada refresh sama sekali di jalur request: kredensial yang
// kedaluwarsa disingkirkan CredentialRepo.Active secara permanen dan request
// gagal 502 sampai worker berkala (yang mungkin juga gagal) memulihkannya
// (audit3 V-EO-15). Penyegar ini dipanggil factory tepat saat Active tidak
// menemukan kredensial yang bisa dipakai untuk sebuah provider.
type PenyegarOAuth struct {
	pool      *pgxpool.Pool
	oauthRepo *upstream.OAuthRepo
	deps      OAuthRefreshDeps
	logger    *slog.Logger
}

// NewPenyegarOAuth membuat PenyegarOAuth dari dependensi yang sama dengan
// worker refresh berkala.
func NewPenyegarOAuth(
	pool *pgxpool.Pool,
	oauthRepo *upstream.OAuthRepo,
	credRepo *upstream.CredentialRepo,
	egressRepo *upstream.EgressRepo,
	oauthClient *oauth.GoogleOAuthClient,
	logger *slog.Logger,
) *PenyegarOAuth {
	if logger == nil {
		logger = slog.Default()
	}
	if oauthClient == nil {
		oauthClient = oauth.NewGoogleOAuthClient(nil)
	}
	return &PenyegarOAuth{
		pool:      pool,
		oauthRepo: oauthRepo,
		deps: OAuthRefreshDeps{
			OAuth:  oauthRepo,
			Creds:  credRepo,
			Egress: egressRepo,
			Client: oauthClient,
			Logger: logger,
		},
		logger: logger,
	}
}

// RefreshProvider me-refresh seluruh sesi aktif provider ini yang
// kredensialnya sudah kedaluwarsa atau hampir kedaluwarsa.
//
// Kunci advisory yang sama dengan worker berkala dan refresh manual admin
// (LockOAuthRefresh) menjaga ketiganya tidak pernah me-refresh sesi yang
// sama bersamaan (audit3 V-EO-19). Bila kunci sedang dipegang pihak lain,
// panggilan ini menyerah tanpa error — refresh sedang berjalan, dan
// pemanggil boleh mencoba membaca kredensial lagi.
func (p *PenyegarOAuth) RefreshProvider(ctx context.Context, providerID string) error {
	unlock, ok, err := TryAdvisoryLock(ctx, p.pool, LockOAuthRefresh)
	if err != nil {
		return err
	}
	if !ok {
		return nil
	}
	defer unlock()

	metas, err := p.oauthRepo.ListSessionsByProviderID(ctx, providerID)
	if err != nil {
		return err
	}

	ambang := time.Now().Add(ambangPenyegarOnDemand)
	for _, m := range metas {
		if !m.Enabled {
			continue
		}
		// Hanya sesi yang kredensialnya tidak bisa dipakai (atau sebentar
		// lagi tidak bisa) yang direfresh — sesi sehat tidak disentuh.
		if m.ExpiresAt != nil && m.ExpiresAt.After(ambang) {
			continue
		}
		sess, err := p.oauthRepo.GetActiveSession(ctx, m.ID)
		if err != nil {
			p.logger.WarnContext(ctx, "gagal memuat sesi oauth untuk refresh on-demand",
				"session_id", m.ID, "provider_id", providerID, "error", err)
			continue
		}
		if err := RefreshSesiOAuth(ctx, p.deps, sess); err != nil {
			p.logger.WarnContext(ctx, "refresh oauth on-demand gagal",
				"session_id", m.ID, "provider_id", providerID, "error", err)
			continue
		}
		p.logger.InfoContext(ctx, "refresh oauth on-demand berhasil",
			"session_id", m.ID, "provider_id", providerID)
	}
	return nil
}
