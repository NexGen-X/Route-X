package upstream

import (
	"context"
	"testing"
	"time"

	"github.com/NexGen-X/Route-X/internal/security"
)

// TestSimpanHasilExchangeAtomikB267 membuktikan B-267 di lapisan repo:
// kredensial, sesi, dan sinkronisasi provider hasil exchange tersimpan
// dalam SATU transaksi — kegagalan langkah tengah menggulung balik
// penulisan yang sudah terjadi lebih dulu, dan koneksi ulang email yang
// sama memperbarui kredensial yang sama (bukan duplikat).
func TestSimpanHasilExchangeAtomikB267(t *testing.T) {
	if testing.Short() {
		t.Skip("butuh PostgreSQL")
	}
	ctx, cancel := context.WithTimeout(context.Background(), 90*time.Second)
	defer cancel()

	pool := newTestPool(ctx, t)
	cipher := testCipher(t, 0x0A)
	oauthRepo, err := NewOAuthRepo(pool, cipher)
	if err != nil {
		t.Fatalf("NewOAuthRepo: %v", err)
	}
	credRepo, err := NewCredentialRepo(pool, cipher)
	if err != nil {
		t.Fatalf("NewCredentialRepo: %v", err)
	}
	provRepo := NewProviderRepo(pool)

	provider := seedProvider(ctx, t, pool, "b267-exchange")
	kedaluwarsa := time.Now().Add(time.Hour)

	// --- Jalur sukses: kredensial + sesi + sinkronisasi provider. ---
	credID, meta, err := oauthRepo.SimpanHasilExchange(ctx, SimpanHasilExchangeParams{
		ProviderID:           provider.ID,
		Label:                "oauth:b267@contoh.test",
		AccountEmail:         "b267@contoh.test",
		AccountName:          "Pengguna B267",
		AccessToken:          security.Secret("akses-b267-pertama"),
		RefreshToken:         security.Secret("refresh-b267"),
		ExpiresAt:            kedaluwarsa,
		SinkronisasiProvider: true,
		ProviderKind:         "google",
		ProviderBaseURL:      "https://generativelanguage.googleapis.com",
	})
	if err != nil {
		t.Fatalf("SimpanHasilExchange: %v", err)
	}
	if credID == "" || meta == nil || meta.CredentialID != credID {
		t.Fatalf("hasil exchange tidak konsisten: credID=%q meta=%+v", credID, meta)
	}
	rahasia, err := credRepo.Reveal(ctx, credID)
	if err != nil {
		t.Fatalf("Reveal: %v", err)
	}
	if rahasia.Reveal() != "akses-b267-pertama" {
		t.Errorf("access token = %q, mau akses-b267-pertama", rahasia.Reveal())
	}
	prov, err := provRepo.Get(ctx, provider.ID)
	if err != nil {
		t.Fatalf("Get provider: %v", err)
	}
	if prov.Kind != "google" || prov.BaseURL != "https://generativelanguage.googleapis.com" {
		t.Errorf("provider sesudah sinkronisasi = %s %s, mau google generativelanguage", prov.Kind, prov.BaseURL)
	}

	// --- Koneksi ulang email sama: kredensial sama diperbarui. ---
	credID2, _, err := oauthRepo.SimpanHasilExchange(ctx, SimpanHasilExchangeParams{
		ProviderID:   provider.ID,
		Label:        "oauth:b267@contoh.test",
		AccountEmail: "b267@contoh.test",
		AccessToken:  security.Secret("akses-b267-kedua"),
		RefreshToken: security.Secret("refresh-b267"),
		ExpiresAt:    kedaluwarsa,
	})
	if err != nil {
		t.Fatalf("SimpanHasilExchange ulang: %v", err)
	}
	if credID2 != credID {
		t.Errorf("koneksi ulang membuat kredensial baru %q, mau memperbarui %q", credID2, credID)
	}
	sessions, err := oauthRepo.ListSessionsByProviderID(ctx, provider.ID)
	if err != nil {
		t.Fatalf("ListSessionsByProviderID: %v", err)
	}
	if len(sessions) != 1 {
		t.Errorf("jumlah sesi = %d, mau 1 (upsert per email)", len(sessions))
	}

	// --- Gulung balik: pool egress tak dikenal pada jalur perbarui
	// membuat SetCredentialEgressPool gagal SESUDAH UpdateSecret —
	// rahasia tersimpan tidak boleh ikut berubah. ---
	poolHantu := "8f1f0b6e-0000-4000-8000-00000000b267"
	_, _, err = oauthRepo.SimpanHasilExchange(ctx, SimpanHasilExchangeParams{
		ProviderID:   provider.ID,
		Label:        "oauth:b267@contoh.test",
		AccountEmail: "b267@contoh.test",
		AccessToken:  security.Secret("akses-b267-hantu"),
		RefreshToken: security.Secret("refresh-b267"),
		ExpiresAt:    kedaluwarsa,
		EgressPoolID: &poolHantu,
	})
	if err == nil {
		t.Fatal("SimpanHasilExchange dengan pool egress tak dikenal tidak menghasilkan error")
	}
	rahasia, err = credRepo.Reveal(ctx, credID)
	if err != nil {
		t.Fatalf("Reveal sesudah gulung balik: %v", err)
	}
	if rahasia.Reveal() != "akses-b267-kedua" {
		t.Errorf("access token = %q sesudah exchange gagal, mau tetap akses-b267-kedua — penulisan tidak atomik (B-267)", rahasia.Reveal())
	}
}
