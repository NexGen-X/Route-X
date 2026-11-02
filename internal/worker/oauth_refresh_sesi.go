package worker

import (
	"context"
	"errors"
	"fmt"
	"log/slog"
	"time"

	"github.com/NexGen-X/Route-X/internal/database/repo/upstream"
	"github.com/NexGen-X/Route-X/internal/oauth"
	"github.com/NexGen-X/Route-X/internal/security"
)

// OAuthRefreshDeps adalah dependensi satu kali refresh sesi OAuth.
//
// Refresh sesi dulu tersalin di tiga tempat (worker berkala, refresh manual
// admin, dan tidak ada sama sekali di jalur request) dengan perilaku yang
// berbeda-beda di tiap salinan — salinan worker menelan kegagalan memuat
// egress lalu keluar langsung, salinan admin membuang refresh token hasil
// rotasi, dan tidak satu pun menonaktifkan sesi yang ditolak permanen.
// Sekarang logikanya satu, di sini, dan ketiga pemanggil memakainya
// (audit3 B-038: V-EO-17 s.d. V-EO-21).
// OAuthTokenRefresher adalah permukaan klien OAuth yang dibutuhkan refresh
// sesi; *oauth.GoogleOAuthClient memenuhinya.
type OAuthTokenRefresher interface {
	RefreshAccessTokenWithProxy(ctx context.Context, refreshToken, clientID, clientSecret string, proxyURL security.Secret) (*oauth.TokenResult, error)
}

type OAuthRefreshDeps struct {
	OAuth  *upstream.OAuthRepo
	Creds  *upstream.CredentialRepo
	Egress *upstream.EgressRepo
	Client OAuthTokenRefresher
	Logger *slog.Logger
}

// maksPercobaanRefresh adalah jumlah percobaan refresh per sesi dalam satu
// putaran untuk kegagalan transien. Kegagalan permanen tidak pernah dicoba
// ulang — hasilnya tidak akan berubah.
const maksPercobaanRefresh = 3

// jedaPercobaanRefresh adalah jeda antarpercobaan transien.
var jedaPercobaanRefresh = []time.Duration{1 * time.Second, 3 * time.Second}

// RefreshSesiOAuth me-refresh satu sesi OAuth sampai tuntas: memuat egress
// (gagal = gagal keras, tidak pernah jatuh ke koneksi langsung), mencoba
// ulang kegagalan transien, memvalidasi masa berlaku token baru, menyimpan
// refresh token hasil rotasi, dan menonaktifkan sesi yang ditolak permanen.
//
// Status terakhir selalu dicatat lewat UpdateRefreshStatus — termasuk untuk
// kegagalan memuat egress dan dekripsi yang dulu hanya hidup di log — agar
// keadaan sesi jujur terlihat di UI (audit3 V-EO-13/V-EO-17).
func RefreshSesiOAuth(ctx context.Context, d OAuthRefreshDeps, sess *upstream.ActiveOAuthSession) error {
	logger := d.Logger
	if logger == nil {
		logger = slog.Default()
	}
	catat := func(err error) {
		var errStr *string
		if err != nil {
			s := err.Error()
			errStr = &s
		}
		if uerr := d.OAuth.UpdateRefreshStatus(ctx, sess.ID, errStr); uerr != nil {
			logger.WarnContext(ctx, "gagal mencatat status refresh sesi oauth",
				"session_id", sess.ID, "provider_id", sess.ProviderID, "email", sess.AccountEmail, "error", uerr)
		}
	}

	// Egress yang dipatok operator adalah kebijakan, bukan saran: gagal
	// memuatnya menghentikan refresh sesi ini. Versi lama hanya me-Warn lalu
	// melanjutkan dengan koneksi langsung (audit3 V-EO-21).
	var proxyURL security.Secret
	if sess.EgressPoolID != nil && *sess.EgressPoolID != "" {
		if d.Egress == nil {
			err := errors.New("sesi memakai egress pool tetapi sumber egress tidak tersedia")
			catat(err)
			return err
		}
		pURL, err := d.Egress.ProxyURL(ctx, *sess.EgressPoolID)
		if err != nil {
			err = fmt.Errorf("gagal memuat URL egress pool untuk refresh: %w", err)
			catat(err)
			return err
		}
		proxyURL = pURL
	}

	var res *oauth.TokenResult
	var err error
	for percobaan := 1; ; percobaan++ {
		res, err = d.Client.RefreshAccessTokenWithProxy(ctx, sess.RefreshToken.Reveal(), sess.ClientID, "", proxyURL)
		if err == nil || errors.Is(err, oauth.ErrTokenDitolakPermanen) || percobaan >= maksPercobaanRefresh {
			break
		}
		jeda := jedaPercobaanRefresh[len(jedaPercobaanRefresh)-1]
		if percobaan-1 < len(jedaPercobaanRefresh) {
			jeda = jedaPercobaanRefresh[percobaan-1]
		}
		logger.WarnContext(ctx, "refresh token oauth gagal, mencoba lagi",
			"session_id", sess.ID, "provider_id", sess.ProviderID, "email", sess.AccountEmail, "percobaan", percobaan, "error", err)
		select {
		case <-ctx.Done():
			err = ctx.Err()
		case <-time.After(jeda):
			continue
		}
		break
	}
	if err != nil {
		catat(err)
		if errors.Is(err, oauth.ErrTokenDitolakPermanen) {
			// Eskalasi (audit3 V-EO-17): sesi yang refresh token-nya ditolak
			// permanen dinonaktifkan beserta kredensialnya, supaya berhenti
			// dicoba/dipilih dan keadaannya terlihat jelas oleh operator.
			// Menghubungkan ulang akun adalah satu-satunya jalan pulih.
			if serr := d.OAuth.SetEnabled(ctx, sess.ID, false); serr != nil {
				logger.ErrorContext(ctx, "gagal menonaktifkan sesi oauth yang ditolak permanen",
					"session_id", sess.ID, "provider_id", sess.ProviderID, "email", sess.AccountEmail, "error", serr)
			} else {
				logger.ErrorContext(ctx, "sesi oauth dinonaktifkan: refresh token ditolak permanen, hubungkan ulang akun",
					"session_id", sess.ID, "provider_id", sess.ProviderID, "email", sess.AccountEmail)
			}
		}
		return err
	}

	// Masa berlaku yang tidak sah berarti token baru lahir kedaluwarsa dan
	// langsung tersingkir dari pemilihan kredensial, sementara putaran ini
	// tercatat sukses — lingkaran yang tidak menghasilkan apa pun selain
	// beban. Perlakukan sebagai kegagalan refresh (audit3 V-EO-20).
	if res.ExpiresIn <= 0 {
		err := fmt.Errorf("respons refresh token tidak memuat expires_in yang sah (%d)", res.ExpiresIn)
		catat(err)
		return err
	}

	expiresAt := time.Now().Add(time.Duration(res.ExpiresIn) * time.Second)
	if err := d.Creds.UpdateSecret(ctx, sess.CredentialID, res.AccessToken, &expiresAt); err != nil {
		err = fmt.Errorf("gagal menyimpan token baru: %w", err)
		catat(err)
		return err
	}

	// Refresh token hasil rotasi WAJIB disimpan: penyedia yang merotasinya
	// membatalkan token lama, dan membuang nilai baru memutus sesi secara
	// permanen pada refresh berikutnya (audit3 V-EO-18).
	if !res.RefreshToken.IsZero() && res.RefreshToken.Reveal() != sess.RefreshToken.Reveal() {
		if err := d.OAuth.UpdateRefreshToken(ctx, sess.ID, res.RefreshToken); err != nil {
			err = fmt.Errorf("gagal menyimpan refresh token hasil rotasi: %w", err)
			catat(err)
			return err
		}
	}

	catat(nil)
	return nil
}
