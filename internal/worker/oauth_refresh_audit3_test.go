package worker

import (
	"bytes"
	"context"
	"errors"
	"fmt"
	"io"
	"log/slog"
	"strings"
	"sync/atomic"
	"testing"
	"time"

	"github.com/jackc/pgx/v5/pgxpool"

	"github.com/NexGen-X/Route-X/internal/database/repo/upstream"
	"github.com/NexGen-X/Route-X/internal/oauth"
	"github.com/NexGen-X/Route-X/internal/security"
)

// nilaiAksesWorkerG8 menyusun nilai akses tiruan untuk kredensial uji.
func nilaiAksesWorkerG8() security.Secret {
	bagian := []string{"akses", "g8"}
	return security.Secret(bagian[0] + "-" + bagian[1])
}

// klienOAuthPalsu adalah OAuthRefreshClient tiruan dengan perilaku yang bisa
// diatur per test dan penghitung percobaan.
type klienOAuthPalsu struct {
	perilaku func(percobaan int) (*oauth.TokenResult, error)
	jumlah   atomic.Int32
}

func (k *klienOAuthPalsu) RefreshAccessTokenWithProxy(_ context.Context, _, _, _ string, _ security.Secret) (*oauth.TokenResult, error) {
	n := int(k.jumlah.Add(1))
	return k.perilaku(n)
}

// lingkunganRefreshG8 menyiapkan repo sungguhan + satu sesi OAuth yang
// kredensialnya sudah kedaluwarsa di schema test sekali pakai.
type lingkunganRefreshG8 struct {
	pool      *pgxpool.Pool
	oauthRepo *upstream.OAuthRepo
	credRepo  *upstream.CredentialRepo
	egress    *upstream.EgressRepo
	sesi      *upstream.ActiveOAuthSession
	meta      *upstream.OAuthSessionMeta
}

func siapkanRefreshG8(t *testing.T, pakaiEgressNonaktif bool) *lingkunganRefreshG8 {
	t.Helper()
	if testing.Short() {
		t.Skip("butuh PostgreSQL")
	}
	ctx := context.Background()
	pool := testPool(t)

	cipher, err := security.NewCipher(bytes.Repeat([]byte{0x67}, 32))
	if err != nil {
		t.Fatalf("NewCipher: %v", err)
	}
	provRepo := upstream.NewProviderRepo(pool)
	nama := fmt.Sprintf("g8-%d", time.Now().UnixNano()%100000000)
	prov, err := provRepo.Create(ctx, upstream.CreateProviderParams{
		Name: nama, DisplayName: "Uji Refresh G8", Kind: "google",
		BaseURL: "https://generativelanguage.googleapis.com",
	})
	if err != nil {
		t.Fatalf("Create provider: %v", err)
	}
	credRepo, err := upstream.NewCredentialRepo(pool, cipher)
	if err != nil {
		t.Fatalf("NewCredentialRepo: %v", err)
	}
	kedaluwarsa := time.Now().Add(-time.Hour)
	cred, err := credRepo.Create(ctx, upstream.CreateCredentialParams{
		ProviderID: prov.ID,
		Label:      "oauth:" + nama,
		Secret:     nilaiAksesWorkerG8(),
		ExpiresAt:  &kedaluwarsa,
	})
	if err != nil {
		t.Fatalf("Create kredensial: %v", err)
	}
	oauthRepo, err := upstream.NewOAuthRepo(pool, cipher)
	if err != nil {
		t.Fatalf("NewOAuthRepo: %v", err)
	}
	egressRepo, err := upstream.NewEgressRepo(pool, cipher)
	if err != nil {
		t.Fatalf("NewEgressRepo: %v", err)
	}
	if pakaiEgressNonaktif {
		// Sesi terikat ke pool yang kemudian DINONAKTIFKAN: ProxyURL kini
		// menjawab ErrNotFound (fix V-EO-01), dan refresh wajib gagal keras
		// karenanya — bukan diam-diam beralih ke koneksi langsung.
		poolEgress, err := egressRepo.Create(ctx, upstream.CreateEgressParams{
			Name:     "pool-g8-" + nama,
			Kind:     upstream.EgressHTTP,
			ProxyURL: security.Secret("http://proxy.contoh.test:3128"),
		})
		if err != nil {
			t.Fatalf("Create pool egress: %v", err)
		}
		// Ikatan egress sesi OAuth hidup di kredensialnya.
		if err := credRepo.SetCredentialEgressPool(ctx, cred.ID, &poolEgress.ID); err != nil {
			t.Fatalf("SetCredentialEgressPool: %v", err)
		}
		nonaktif := false
		if _, err := egressRepo.Update(ctx, poolEgress.ID, upstream.UpdateEgressParams{Enabled: &nonaktif}); err != nil {
			t.Fatalf("Nonaktifkan pool: %v", err)
		}
	}
	meta, err := oauthRepo.UpsertSession(ctx, upstream.UpsertOAuthSessionParams{
		ProviderID:   prov.ID,
		CredentialID: cred.ID,
		AccountEmail: nama + "@contoh.test",
		ClientID:     "client-g8",
		RefreshToken: security.Secret("refresh-awal-g8"),
	})
	if err != nil {
		t.Fatalf("UpsertSession: %v", err)
	}
	sesi, err := oauthRepo.GetActiveSession(ctx, meta.ID)
	if err != nil {
		t.Fatalf("GetActiveSession: %v", err)
	}
	return &lingkunganRefreshG8{pool: pool, oauthRepo: oauthRepo, credRepo: credRepo, egress: egressRepo, sesi: sesi, meta: meta}
}

func (l *lingkunganRefreshG8) deps(klien OAuthTokenRefresher) OAuthRefreshDeps {
	return OAuthRefreshDeps{
		OAuth:  l.oauthRepo,
		Creds:  l.credRepo,
		Egress: l.egress,
		Client: klien,
		Logger: slog.New(slog.NewTextHandler(io.Discard, nil)),
	}
}

// tanpaJeda mematikan jeda antarpercobaan refresh selama test berjalan.
func tanpaJeda(t *testing.T) {
	t.Helper()
	lama := jedaPercobaanRefresh
	jedaPercobaanRefresh = []time.Duration{0, 0}
	t.Cleanup(func() { jedaPercobaanRefresh = lama })
}

// TestRefreshSesiPermanenMenonaktifkan membuktikan audit3 V-EO-17: penolakan
// permanen (invalid_grant) TIDAK di-retry membabi buta — sesi DAN kredensial
// dinonaktifkan dan alasan tercatat, agar admin tahu akun harus dihubungkan
// ulang.
func TestRefreshSesiPermanenMenonaktifkan(t *testing.T) {
	tanpaJeda(t)
	env := siapkanRefreshG8(t, false)
	klien := &klienOAuthPalsu{perilaku: func(int) (*oauth.TokenResult, error) {
		return nil, fmt.Errorf("gagal: %w", oauth.ErrTokenDitolakPermanen)
	}}

	err := RefreshSesiOAuth(context.Background(), env.deps(klien), env.sesi)
	if err == nil {
		t.Fatal("refresh permanen tidak menghasilkan error")
	}
	if klien.jumlah.Load() != 1 {
		t.Errorf("percobaan = %d, ingin 1 (permanen tidak di-retry)", klien.jumlah.Load())
	}
	meta, err := env.oauthRepo.GetSession(context.Background(), env.meta.ID)
	if err != nil {
		t.Fatalf("GetSession: %v", err)
	}
	if meta.Enabled {
		t.Error("sesi masih aktif sesudah penolakan permanen")
	}
	if meta.LastRefreshError == nil || *meta.LastRefreshError == "" {
		t.Error("last_refresh_error kosong sesudah penolakan permanen")
	}
	cred, err := env.credRepo.Get(context.Background(), env.meta.CredentialID)
	if err != nil {
		t.Fatalf("Get kredensial: %v", err)
	}
	if cred.Enabled {
		t.Error("kredensial masih aktif sesudah penolakan permanen")
	}
}

// TestRefreshSesiTransienDanRotasiToken membuktikan kegagalan transien
// di-retry terbatas lalu berhasil (V-EO-17), dan refresh token ROTASI dari
// penyedia tersimpan menggantikan yang lama (V-EO-18).
func TestRefreshSesiTransienDanRotasiToken(t *testing.T) {
	tanpaJeda(t)
	env := siapkanRefreshG8(t, false)
	klien := &klienOAuthPalsu{perilaku: func(n int) (*oauth.TokenResult, error) {
		if n < 3 {
			return nil, errors.New("502 dari penyedia")
		}
		return &oauth.TokenResult{
			AccessToken:  security.Secret("akses-baru-g8"),
			ExpiresIn:    3600,
			RefreshToken: security.Secret("refresh-rotasi-g8"),
		}, nil
	}}

	if err := RefreshSesiOAuth(context.Background(), env.deps(klien), env.sesi); err != nil {
		t.Fatalf("RefreshSesiOAuth: %v", err)
	}
	if klien.jumlah.Load() != 3 {
		t.Errorf("percobaan = %d, ingin 3", klien.jumlah.Load())
	}
	aktif, err := env.oauthRepo.GetActiveSession(context.Background(), env.meta.ID)
	if err != nil {
		t.Fatalf("GetActiveSession: %v", err)
	}
	if aktif.RefreshToken.Reveal() != "refresh-rotasi-g8" {
		t.Errorf("refresh token tersimpan = %q, ingin hasil rotasi", aktif.RefreshToken.Reveal())
	}
	rahasia, err := env.credRepo.Reveal(context.Background(), env.meta.CredentialID)
	if err != nil {
		t.Fatalf("Reveal kredensial: %v", err)
	}
	if rahasia.Reveal() != "akses-baru-g8" {
		t.Errorf("access token tersimpan = %q, ingin akses-baru-g8", rahasia.Reveal())
	}
	meta, err := env.oauthRepo.GetSession(context.Background(), env.meta.ID)
	if err != nil {
		t.Fatalf("GetSession: %v", err)
	}
	if meta.LastRefreshError != nil {
		t.Errorf("last_refresh_error = %q sesudah sukses, ingin bersih", *meta.LastRefreshError)
	}
}

// TestRefreshSesiExpiresInNolDanEgressHilang membuktikan dua gerbang keras:
// expires_in <= 0 dianggap kegagalan tanpa retry (V-EO-20) dan pool egress
// yang hilang membuat refresh gagal TANPA pernah memanggil penyedia
// (V-EO-21 — tidak ada fallback koneksi langsung senyap).
func TestRefreshSesiExpiresInNolDanEgressHilang(t *testing.T) {
	tanpaJeda(t)
	ctx := context.Background()

	env := siapkanRefreshG8(t, false)
	klienNol := &klienOAuthPalsu{perilaku: func(int) (*oauth.TokenResult, error) {
		return &oauth.TokenResult{AccessToken: security.Secret("x"), ExpiresIn: 0}, nil
	}}
	if err := RefreshSesiOAuth(ctx, env.deps(klienNol), env.sesi); err == nil ||
		!strings.Contains(err.Error(), "expires_in") {
		t.Fatalf("expires_in 0: err = %v, ingin kegagalan expires_in", err)
	}
	if klienNol.jumlah.Load() != 1 {
		t.Errorf("percobaan expires_in = %d, ingin 1", klienNol.jumlah.Load())
	}
	meta, _ := env.oauthRepo.GetSession(ctx, env.meta.ID)
	if !meta.Enabled {
		t.Error("sesi nonaktif gara-gara expires_in tidak sah (bukan penolakan permanen)")
	}
	if meta.LastRefreshError == nil {
		t.Error("kegagalan expires_in tidak tercatat")
	}

	env2 := siapkanRefreshG8(t, true)
	klien2 := &klienOAuthPalsu{perilaku: func(int) (*oauth.TokenResult, error) {
		return &oauth.TokenResult{AccessToken: security.Secret("x"), ExpiresIn: 3600}, nil
	}}
	if err := RefreshSesiOAuth(ctx, env2.deps(klien2), env2.sesi); err == nil ||
		!strings.Contains(err.Error(), "egress") {
		t.Fatalf("egress hilang: err = %v, ingin kegagalan memuat egress", err)
	}
	if klien2.jumlah.Load() != 0 {
		t.Errorf("penyedia terpanggil %d kali padahal egress gagal dimuat", klien2.jumlah.Load())
	}
}
