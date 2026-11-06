package worker

import (
	"context"
	"io"
	"log/slog"
	"testing"
	"time"

	"github.com/NexGen-X/Route-X/internal/database/repo/upstream"
	"github.com/NexGen-X/Route-X/internal/oauth"
	"github.com/NexGen-X/Route-X/internal/security"
)

// TestRefreshKredensialMemaksaDiLuarAmbang membuktikan B-210 di sisi
// penyegar: RefreshKredensial me-refresh sesi yang kredensialnya MASIH lama
// kedaluwarsanya — kasus token dicabut penyedia sebelum waktunya — padahal
// RefreshProvider yang berbasis ambang sengaja melewatinya. Kredensial
// tanpa sesi, provider yang salah, dan sesi nonaktif adalah no-op.
func TestRefreshKredensialMemaksaDiLuarAmbang(t *testing.T) {
	tanpaJeda(t)
	ctx := context.Background()
	env := siapkanRefreshG8(t, false)

	// Geser kedaluwarsa kredensial ke +3 jam: token ini yang dicabut
	// penyedia di tengah masa berlakunya.
	if _, err := env.pool.Exec(ctx,
		`update provider_credentials set expires_at = $2 where id = $1`,
		env.meta.CredentialID, time.Now().Add(3*time.Hour)); err != nil {
		t.Fatalf("menggeser kedaluwarsa kredensial: %v", err)
	}

	klien := &klienOAuthPalsu{perilaku: func(int) (*oauth.TokenResult, error) {
		return &oauth.TokenResult{
			AccessToken:  security.Secret("akses-baru-b210"),
			ExpiresIn:    3600,
			RefreshToken: security.Secret("refresh-rotasi-b210"),
		}, nil
	}}
	p := &PenyegarOAuth{
		pool:      env.pool,
		oauthRepo: env.oauthRepo,
		deps:      env.deps(klien),
		logger:    slog.New(slog.NewTextHandler(io.Discard, nil)),
	}

	// Kontrol ambang: RefreshProvider melewati sesi ini (masa berlaku
	// masih jauh di luar ambang 30 menit) — tidak ada panggilan penyedia.
	if err := p.RefreshProvider(ctx, env.meta.ProviderID); err != nil {
		t.Fatalf("RefreshProvider: %v", err)
	}
	if klien.jumlah.Load() != 0 {
		t.Fatalf("RefreshProvider memanggil penyedia %d kali untuk token yang masih lama kedaluwarsanya, ingin 0", klien.jumlah.Load())
	}

	// Refresh paksa per kredensial: sesi direfresh walau belum waktunya.
	if err := p.RefreshKredensial(ctx, env.meta.ProviderID, env.meta.CredentialID); err != nil {
		t.Fatalf("RefreshKredensial: %v", err)
	}
	if klien.jumlah.Load() != 1 {
		t.Fatalf("penyedia terpanggil %d kali sesudah RefreshKredensial, ingin 1", klien.jumlah.Load())
	}
	rahasia, err := env.credRepo.Reveal(ctx, env.meta.CredentialID)
	if err != nil {
		t.Fatalf("Reveal kredensial: %v", err)
	}
	if rahasia.Reveal() != "akses-baru-b210" {
		t.Errorf("access token = %q, ingin akses-baru-b210", rahasia.Reveal())
	}
	aktif, err := env.oauthRepo.GetActiveSession(ctx, env.meta.ID)
	if err != nil {
		t.Fatalf("GetActiveSession: %v", err)
	}
	if aktif.RefreshToken.Reveal() != "refresh-rotasi-b210" {
		t.Errorf("refresh token = %q, ingin refresh-rotasi-b210", aktif.RefreshToken.Reveal())
	}

	// No-op 1: provider yang salah — sesi ini bukan milik provider itu.
	if err := p.RefreshKredensial(ctx, "8f1f0b6e-0000-4000-8000-00000000b210", env.meta.CredentialID); err != nil {
		t.Fatalf("RefreshKredensial provider salah: %v", err)
	}
	// No-op 2: kredensial tanpa sesi OAuth (API key biasa).
	credPolos, err := env.credRepo.Create(ctx, upstream.CreateCredentialParams{
		ProviderID: env.meta.ProviderID,
		Label:      "api-key-biasa-b210",
		Secret:     security.Secret("kunci-biasa-b210"),
	})
	if err != nil {
		t.Fatalf("Create kredensial polos: %v", err)
	}
	if err := p.RefreshKredensial(ctx, env.meta.ProviderID, credPolos.ID); err != nil {
		t.Fatalf("RefreshKredensial tanpa sesi: %v", err)
	}
	if klien.jumlah.Load() != 1 {
		t.Errorf("penyedia terpanggil %d kali sesudah dua no-op, ingin tetap 1", klien.jumlah.Load())
	}

	// No-op 3: sesi dinonaktifkan — tidak ada lagi yang direfresh.
	if err := env.oauthRepo.SetEnabled(ctx, env.meta.ID, false); err != nil {
		t.Fatalf("SetEnabled sesi: %v", err)
	}
	if err := p.RefreshKredensial(ctx, env.meta.ProviderID, env.meta.CredentialID); err != nil {
		t.Fatalf("RefreshKredensial sesi nonaktif: %v", err)
	}
	if klien.jumlah.Load() != 1 {
		t.Errorf("penyedia terpanggil %d kali sesudah sesi dinonaktifkan, ingin tetap 1", klien.jumlah.Load())
	}
}
