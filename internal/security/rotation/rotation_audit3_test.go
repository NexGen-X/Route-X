package rotation_test

import (
	"bytes"
	"context"
	"encoding/base64"
	"testing"
	"time"

	"github.com/NexGen-X/Route-X/internal/database/repo/upstream"
	"github.com/NexGen-X/Route-X/internal/security"
	"github.com/NexGen-X/Route-X/internal/security/rotation"
)

// nilaiAksesRotasi menyusun nilai akses tiruan untuk kredensial uji rotasi.
func nilaiAksesRotasi() security.Secret {
	bagian := []string{"akses", "lama"}
	return security.Secret(bagian[0] + "-" + bagian[1])
}

// TestRotasiMencakupSesiOAuth membuktikan audit3 V-EO-13 tertutup: rotasi
// ENCRYPTION_KEY kini memutar refresh_token_encrypted pada
// provider_oauth_sessions. Dulu tabel itu tidak dikenal rotator, sehingga
// sesudah rotasi semua refresh token sesi menjadi tidak terdekripsi dan
// refresh OAuth mati permanen tanpa jejak. Di sini sesi ditulis dengan
// cipher lama, kunci diputar, lalu sesi dibaca dengan cipher baru dan
// tokennya harus kembali persis.
func TestRotasiMencakupSesiOAuth(t *testing.T) {
	if testing.Short() {
		t.Skip("memerlukan koneksi PostgreSQL nyata")
	}
	ctx, cancel := context.WithTimeout(context.Background(), 60*time.Second)
	defer cancel()

	pool := newTestPool(ctx, t)

	kunciLama := bytes.Repeat([]byte{0x51}, 32)
	kunciBaru := bytes.Repeat([]byte{0x52}, 32)
	cipherLama, err := security.NewCipher(kunciLama)
	if err != nil {
		t.Fatalf("NewCipher(lama): %v", err)
	}
	cipherBaru, err := security.NewCipher(kunciBaru)
	if err != nil {
		t.Fatalf("NewCipher(baru): %v", err)
	}

	provRepo := upstream.NewProviderRepo(pool)
	prov, err := provRepo.Create(ctx, upstream.CreateProviderParams{
		Name:        "test-rot-sesi",
		DisplayName: "Test Rotasi Sesi OAuth",
		Kind:        "google",
		BaseURL:     "https://generativelanguage.googleapis.com",
	})
	if err != nil {
		t.Fatalf("provRepo.Create: %v", err)
	}
	credRepoLama, err := upstream.NewCredentialRepo(pool, cipherLama)
	if err != nil {
		t.Fatalf("NewCredentialRepo(lama): %v", err)
	}
	kedaluwarsa := time.Now().Add(time.Hour)
	cred, err := credRepoLama.Create(ctx, upstream.CreateCredentialParams{
		ProviderID: prov.ID,
		Label:      "oauth:test-rot-sesi",
		Secret:     nilaiAksesRotasi(),
		ExpiresAt:  &kedaluwarsa,
	})
	if err != nil {
		t.Fatalf("credRepoLama.Create: %v", err)
	}

	oauthRepoLama, err := upstream.NewOAuthRepo(pool, cipherLama)
	if err != nil {
		t.Fatalf("NewOAuthRepo(lama): %v", err)
	}
	const tokenAsli = "refresh-token-rotasi-uji-123456"
	meta, err := oauthRepoLama.UpsertSession(ctx, upstream.UpsertOAuthSessionParams{
		ProviderID:   prov.ID,
		CredentialID: cred.ID,
		AccountEmail: "rotasi@contoh.test",
		ClientID:     "client-rotasi",
		RefreshToken: security.Secret(tokenAsli),
	})
	if err != nil {
		t.Fatalf("UpsertSession: %v", err)
	}

	rotator, err := rotation.NewRotator(rotation.Config{
		DB:     pool,
		OldKey: base64.StdEncoding.EncodeToString(kunciLama),
		NewKey: base64.StdEncoding.EncodeToString(kunciBaru),
		DryRun: false,
	})
	if err != nil {
		t.Fatalf("NewRotator: %v", err)
	}
	res, err := rotator.Execute(ctx)
	if err != nil {
		t.Fatalf("Execute: %v", err)
	}
	ditemukan := false
	for _, tr := range res.Tables {
		if tr.TableName == "provider_oauth_sessions" {
			ditemukan = true
			if tr.Rotated != 1 {
				t.Errorf("baris sesi terotasi = %d, ingin 1", tr.Rotated)
			}
		}
	}
	if !ditemukan {
		t.Fatalf("hasil rotasi tidak menyebut provider_oauth_sessions: %+v", res.Tables)
	}

	// Sesi terbaca dengan cipher BARU, token persis seperti semula.
	oauthRepoBaru, err := upstream.NewOAuthRepo(pool, cipherBaru)
	if err != nil {
		t.Fatalf("NewOAuthRepo(baru): %v", err)
	}
	aktif, err := oauthRepoBaru.GetActiveSession(ctx, meta.ID)
	if err != nil {
		t.Fatalf("GetActiveSession(cipher baru): %v", err)
	}
	if aktif.RefreshToken.Reveal() != tokenAsli {
		t.Errorf("refresh token sesudah rotasi = %q, ingin token asli", aktif.RefreshToken.Reveal())
	}

	// Dan cipher lama memang sudah tidak bisa membacanya.
	if _, err := oauthRepoLama.GetActiveSession(ctx, meta.ID); err == nil {
		t.Error("cipher lama masih bisa membaca sesi sesudah rotasi")
	}
}
