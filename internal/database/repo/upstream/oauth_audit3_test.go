package upstream

import (
	"context"
	"strings"
	"testing"
	"time"

	"github.com/jackc/pgx/v5/pgxpool"

	"github.com/NexGen-X/Route-X/internal/security"
)

// nilaiAksesUji menyusun nilai akses tiruan untuk kredensial sesi uji.
func nilaiAksesUji(nama string) security.Secret {
	return security.Secret("akses" + "-" + "token" + "-" + nama)
}

// buatSesiUjiG8 menyiapkan satu sesi OAuth lengkap lewat repo sungguhan di
// pool yang diberikan, dengan masa berlaku kredensial sesuai parameter.
func buatSesiUjiG8(ctx context.Context, t *testing.T, pool *pgxpool.Pool, cipher *security.Cipher, oauthRepo *OAuthRepo, nama, refreshToken string, kredensialKedaluwarsaPada time.Time) *OAuthSessionMeta {
	t.Helper()

	provider := seedProvider(ctx, t, pool, nama)
	credentials, err := NewCredentialRepo(pool, cipher)
	if err != nil {
		t.Fatalf("NewCredentialRepo: %v", err)
	}
	cred, err := credentials.Create(ctx, CreateCredentialParams{
		ProviderID: provider.ID,
		Label:      "oauth:" + nama,
		Secret:     nilaiAksesUji(nama),
		ExpiresAt:  &kredensialKedaluwarsaPada,
	})
	if err != nil {
		t.Fatalf("membuat kredensial %s: %v", nama, err)
	}

	meta, err := oauthRepo.UpsertSession(ctx, UpsertOAuthSessionParams{
		ProviderID:   provider.ID,
		CredentialID: cred.ID,
		AccountEmail: nama + "@contoh.test",
		ClientID:     "client-" + nama,
		RefreshToken: security.Secret(refreshToken),
	})
	if err != nil {
		t.Fatalf("UpsertSession %s: %v", nama, err)
	}
	return meta
}

// TestOAuthUpsertDanRotasiRefreshToken membuktikan dua hal audit3 grup 8:
//   - V-EO-14: UpsertSession pada konflik (provider, email) menimpa baris
//     lama secara atomik — token hasil upsert kedua yang terbaca, bukan
//     error 500 dari pool querier yang tidak mendukung LookupSessionID.
//   - V-EO-18: UpdateRefreshToken menyimpan token rotasi baru dan sesi
//     membacanya kembali persis.
func TestOAuthUpsertDanRotasiRefreshToken(t *testing.T) {
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

	jauh := time.Now().Add(30 * 24 * time.Hour)
	meta := buatSesiUjiG8(ctx, t, pool, cipher, oauthRepo, "g8-upsert", "refresh-versi-1", jauh)

	// Upsert kedua untuk (provider, email) yang sama: jalur konflik.
	meta2, err := oauthRepo.UpsertSession(ctx, UpsertOAuthSessionParams{
		ProviderID:   meta.ProviderID,
		CredentialID: meta.CredentialID,
		AccountEmail: meta.AccountEmail,
		ClientID:     "client-g8-upsert",
		RefreshToken: security.Secret("refresh-versi-2"),
	})
	if err != nil {
		t.Fatalf("UpsertSession konflik: %v", err)
	}
	if meta2.ID != meta.ID {
		t.Fatalf("upsert konflik membuat sesi baru (%s != %s)", meta2.ID, meta.ID)
	}
	aktif, err := oauthRepo.GetActiveSession(ctx, meta.ID)
	if err != nil {
		t.Fatalf("GetActiveSession: %v", err)
	}
	if aktif.RefreshToken.Reveal() != "refresh-versi-2" {
		t.Fatalf("token sesudah upsert konflik = %q, ingin refresh-versi-2", aktif.RefreshToken.Reveal())
	}

	// Rotasi refresh token saat refresh: token baru tersimpan terenkripsi
	// dan terbaca kembali persis.
	if err := oauthRepo.UpdateRefreshToken(ctx, meta.ID, "refresh-rotasi-3"); err != nil {
		t.Fatalf("UpdateRefreshToken: %v", err)
	}
	aktif, err = oauthRepo.GetActiveSession(ctx, meta.ID)
	if err != nil {
		t.Fatalf("GetActiveSession sesudah rotasi: %v", err)
	}
	if aktif.RefreshToken.Reveal() != "refresh-rotasi-3" {
		t.Fatalf("token sesudah rotasi = %q, ingin refresh-rotasi-3", aktif.RefreshToken.Reveal())
	}
}

// TestOAuthListExpiringMencatatCiphertextRusak membuktikan audit3 V-EO-13 di
// lapisan repo: sesi yang refresh token-nya tidak bisa didekripsi dilewati
// dari daftar refresh TETAPI kegagalannya tercatat di last_refresh_error —
// tidak lagi hilang senyap di log saja.
func TestOAuthListExpiringMencatatCiphertextRusak(t *testing.T) {
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

	// Kredensial sudah kedaluwarsa → sesi masuk daftar ListExpiring.
	meta := buatSesiUjiG8(ctx, t, pool, cipher, oauthRepo, "g8-rusak", "refresh-utuh", time.Now().Add(-time.Hour))

	// Rusak ciphertext sesi: ganti dengan enkripsi cipher LAIN sehingga
	// dekripsi oleh cipher repo pasti gagal.
	cipherLain := testCipher(t, 0x0B)
	rusak, err := cipherLain.EncryptSecret(security.Secret("token-lain"), security.OAuthSessionAAD(meta.ID))
	if err != nil {
		t.Fatalf("EncryptSecret cipher lain: %v", err)
	}
	if _, err := pool.Exec(ctx,
		`update provider_oauth_sessions set refresh_token_encrypted = $1, encryption_key_id = $2 where id = $3`,
		rusak, cipherLain.KeyID(), meta.ID); err != nil {
		t.Fatalf("merusak ciphertext: %v", err)
	}

	sessions, err := oauthRepo.ListExpiringOAuthSessions(ctx, 30*time.Minute)
	if err != nil {
		t.Fatalf("ListExpiringOAuthSessions: %v", err)
	}
	for _, s := range sessions {
		if s.ID == meta.ID {
			t.Fatal("sesi berciphertext rusak ikut terdaftar untuk di-refresh")
		}
	}

	sesudah, err := oauthRepo.GetSession(ctx, meta.ID)
	if err != nil {
		t.Fatalf("GetSession: %v", err)
	}
	if sesudah.LastRefreshError == nil || !strings.Contains(*sesudah.LastRefreshError, "dekripsi") {
		t.Fatalf("last_refresh_error = %v, ingin catatan kegagalan dekripsi", sesudah.LastRefreshError)
	}
}
