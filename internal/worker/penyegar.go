package worker

import (
	"context"
	"errors"
	"log/slog"
	"time"

	"github.com/jackc/pgx/v5/pgxpool"

	"github.com/NexGen-X/Route-X/internal/database/repo"
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
	client    *oauth.GoogleOAuthClient
	logger    *slog.Logger
}

// NewPenyegarOAuth membuat PenyegarOAuth dari dependensi yang sama dengan
// worker refresh berkala.
func NewPenyegarOAuth(
	pool *pgxpool.Pool,
	oauthRepo *upstream.OAuthRepo,
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
			Egress: egressRepo,
			Client: oauthClient,
			Logger: logger,
		},
		client: oauthClient,
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

// RefreshKredensial memaksa refresh sesi OAuth yang terikat pada satu
// kredensial, TANPA memandang masa berlakunya (B-210).
//
// Inilah potongan yang hilang dari lingkaran penolakan auth: RefreshProvider
// berbasis ambang kedaluwarsa, jadi access token yang dicabut penyedia jauh
// sebelum kedaluwarsa lokalnya tidak akan pernah direfresh olehnya — token
// mati itu dipilih lagi dan lagi sampai jam lokalnya habis. Pemanggilnya
// adalah Factory lewat WithOAuthCredentialRefresher, tepat sesudah upstream
// menolak kredensial itu dengan kesalahan auth.
//
// Kredensial tanpa sesi OAuth (API key biasa), sesi nonaktif, atau sesi
// milik provider lain adalah no-op tanpa error: ketiganya memang tidak
// punya apa pun untuk direfresh. Kunci advisory yang sama dengan refresh
// lain menjaga tidak ada dua refresh berjalan bersamaan; bila kunci sedang
// dipegang, panggilan ini menyerah tanpa error.
func (p *PenyegarOAuth) RefreshKredensial(ctx context.Context, providerID, credentialID string) error {
	unlock, ok, err := TryAdvisoryLock(ctx, p.pool, LockOAuthRefresh)
	if err != nil {
		return err
	}
	if !ok {
		return nil
	}
	defer unlock()

	meta, err := p.oauthRepo.GetSessionByCredentialID(ctx, credentialID)
	if err != nil {
		if errors.Is(err, repo.ErrNotFound) {
			return nil
		}
		return err
	}
	if !meta.Enabled || meta.ProviderID != providerID {
		return nil
	}
	sess, err := p.oauthRepo.GetActiveSession(ctx, meta.ID)
	if err != nil {
		return err
	}
	return RefreshSesiOAuth(ctx, p.deps, sess)
}
