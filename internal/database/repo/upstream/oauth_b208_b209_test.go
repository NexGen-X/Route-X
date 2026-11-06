package upstream

import (
	"context"
	"testing"
	"time"

	"github.com/NexGen-X/Route-X/internal/security"
)

// TestOAuthListExpiringMelewatiProviderNonaktif membuktikan B-209:
// ListExpiringOAuthSessions tidak lagi mendaftarkan sesi yang provider-nya
// dinonaktifkan operator. Versi lama hanya menyaring s.enabled dan
// c.enabled, sehingga worker me-refresh (dan merotasi refresh token) sesi
// milik provider mati selamanya. Test ini hanya memakai API yang sudah ada
// sebelum perbaikan — ia GAGAL di kode lama.
func TestOAuthListExpiringMelewatiProviderNonaktif(t *testing.T) {
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
	provRepo := NewProviderRepo(pool)

	meta := buatSesiUjiG8(ctx, t, pool, cipher, oauthRepo, "b209-nonaktif", "refresh-b209", time.Now().Add(-time.Hour))

	terdaftar := func() bool {
		sessions, err := oauthRepo.ListExpiringOAuthSessions(ctx, 30*time.Minute)
		if err != nil {
			t.Fatalf("ListExpiringOAuthSessions: %v", err)
		}
		for _, s := range sessions {
			if s.ID == meta.ID {
				return true
			}
		}
		return false
	}

	if !terdaftar() {
		t.Fatal("sesi kedaluwarsa milik provider aktif tidak terdaftar untuk di-refresh")
	}

	if _, err := provRepo.SetEnabled(ctx, meta.ProviderID, false); err != nil {
		t.Fatalf("SetEnabled provider false: %v", err)
	}
	if terdaftar() {
		t.Error("sesi milik provider nonaktif masih terdaftar untuk di-refresh (B-209)")
	}

	if _, err := provRepo.SetEnabled(ctx, meta.ProviderID, true); err != nil {
		t.Fatalf("SetEnabled provider true: %v", err)
	}
	if !terdaftar() {
		t.Error("sesi tidak terdaftar lagi sesudah provider diaktifkan kembali")
	}
}

// TestSimpanHasilRefreshAtomik membuktikan B-208 di lapisan repo:
// SimpanHasilRefresh menulis access token baru, refresh token rotasi, dan
// pembersihan status dalam satu transaksi — kegagalan pada langkah sesi
// menggulung balik penulisan kredensial yang sudah terjadi lebih dulu.
func TestSimpanHasilRefreshAtomik(t *testing.T) {
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

	meta := buatSesiUjiG8(ctx, t, pool, cipher, oauthRepo, "b208-atomik", "refresh-lama-b208", time.Now().Add(-time.Hour))

	// Tanam status error lama agar pembersihannya terbukti.
	gagalLama := "kegagalan lama b208"
	if err := oauthRepo.UpdateRefreshStatus(ctx, meta.ID, &gagalLama); err != nil {
		t.Fatalf("UpdateRefreshStatus: %v", err)
	}

	rotasi := security.Secret("refresh-rotasi-b208")
	kedaluwarsaBaru := time.Now().Add(time.Hour)
	if err := oauthRepo.SimpanHasilRefresh(ctx, SimpanHasilRefreshParams{
		SessionID:        meta.ID,
		CredentialID:     meta.CredentialID,
		AccessToken:      security.Secret("akses-baru-b208"),
		ExpiresAt:        kedaluwarsaBaru,
		RefreshTokenBaru: &rotasi,
	}); err != nil {
		t.Fatalf("SimpanHasilRefresh: %v", err)
	}

	rahasia, err := credRepo.Reveal(ctx, meta.CredentialID)
	if err != nil {
		t.Fatalf("Reveal kredensial: %v", err)
	}
	if rahasia.Reveal() != "akses-baru-b208" {
		t.Errorf("access token tersimpan = %q, ingin akses-baru-b208", rahasia.Reveal())
	}
	aktif, err := oauthRepo.GetActiveSession(ctx, meta.ID)
	if err != nil {
		t.Fatalf("GetActiveSession: %v", err)
	}
	if aktif.RefreshToken.Reveal() != "refresh-rotasi-b208" {
		t.Errorf("refresh token tersimpan = %q, ingin refresh-rotasi-b208", aktif.RefreshToken.Reveal())
	}
	sesudah, err := oauthRepo.GetSession(ctx, meta.ID)
	if err != nil {
		t.Fatalf("GetSession: %v", err)
	}
	if sesudah.LastRefreshError != nil {
		t.Errorf("last_refresh_error = %q sesudah simpan sukses, ingin bersih", *sesudah.LastRefreshError)
	}
	if sesudah.ExpiresAt == nil || sesudah.ExpiresAt.Before(time.Now().Add(30*time.Minute)) {
		t.Errorf("expires_at kredensial = %v, ingin masa berlaku baru ±1 jam", sesudah.ExpiresAt)
	}

	// Tanpa rotasi: refresh token tersimpan tidak disentuh.
	if err := oauthRepo.SimpanHasilRefresh(ctx, SimpanHasilRefreshParams{
		SessionID:    meta.ID,
		CredentialID: meta.CredentialID,
		AccessToken:  security.Secret("akses-kedua-b208"),
		ExpiresAt:    kedaluwarsaBaru,
	}); err != nil {
		t.Fatalf("SimpanHasilRefresh tanpa rotasi: %v", err)
	}
	aktif, err = oauthRepo.GetActiveSession(ctx, meta.ID)
	if err != nil {
		t.Fatalf("GetActiveSession kedua: %v", err)
	}
	if aktif.RefreshToken.Reveal() != "refresh-rotasi-b208" {
		t.Errorf("refresh token berubah menjadi %q padahal tidak ada rotasi", aktif.RefreshToken.Reveal())
	}

	// Gulung balik: sesi tidak dikenal + rotasi → penulisan sesi gagal,
	// dan access token kredensial TIDAK boleh ikut berubah.
	rotasiHantu := security.Secret("refresh-hantu-b208")
	err = oauthRepo.SimpanHasilRefresh(ctx, SimpanHasilRefreshParams{
		SessionID:        "8f1f0b6e-0000-4000-8000-00000000b208",
		CredentialID:     meta.CredentialID,
		AccessToken:      security.Secret("akses-hantu-b208"),
		ExpiresAt:        kedaluwarsaBaru,
		RefreshTokenBaru: &rotasiHantu,
	})
	if err == nil {
		t.Fatal("SimpanHasilRefresh dengan sesi tidak dikenal tidak menghasilkan error")
	}
	rahasia, err = credRepo.Reveal(ctx, meta.CredentialID)
	if err != nil {
		t.Fatalf("Reveal kredensial sesudah gulung balik: %v", err)
	}
	if rahasia.Reveal() != "akses-kedua-b208" {
		t.Errorf("access token = %q sesudah penyimpanan gagal, ingin tetap akses-kedua-b208 — penulisan tidak atomik (B-208)", rahasia.Reveal())
	}
}
