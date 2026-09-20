package upstream

import (
	"context"
	"errors"
	"strings"
	"testing"
	"time"

	"github.com/jackc/pgx/v5/pgconn"
	"github.com/jackc/pgx/v5/pgxpool"

	"github.com/NexGen-X/Route-X/internal/oauth"
	"github.com/NexGen-X/Route-X/internal/security"
)

// seedOAuthCredential menyiapkan provider dan kredensialnya, syarat FK tabel
// provider_oauth_sessions. Dipakai berulang oleh test di file ini.
func seedOAuthCredential(ctx context.Context, t *testing.T, pool *pgxpool.Pool, cipher *security.Cipher, name string) *CredentialMeta {
	t.Helper()

	provider := seedProvider(ctx, t, pool, name)
	credentials, err := NewCredentialRepo(pool, cipher)
	if err != nil {
		t.Fatalf("NewCredentialRepo: %v", err)
	}
	cred, err := credentials.Create(ctx, CreateCredentialParams{
		ProviderID: provider.ID,
		Label:      "oauth:" + name,
		Secret:     security.Secret(testSecret),
		ExpiresAt:  ptr(time.Now().Add(30 * 24 * time.Hour)),
	})
	if err != nil {
		t.Fatalf("membuat kredensial %s: %v", name, err)
	}
	return cred
}

// TestOAuthSchemaHasNoVendorDefaults membuktikan migrasi 0012 benar-benar menghapus
// DEFAULT kolom client_id dan redirect_uri dari skema: database tidak boleh lagi
// menyembunyikan OAuth Client ID publik vendor maupun redirect URI localhost.
func TestOAuthSchemaHasNoVendorDefaults(t *testing.T) {
	if testing.Short() {
		t.Skip("butuh PostgreSQL")
	}
	ctx, cancel := context.WithTimeout(context.Background(), 90*time.Second)
	defer cancel()

	pool := newTestPool(ctx, t)

	// current_schema() mengikuti search_path pool test, jadi pembacaan tepat pada
	// schema sekali pakai ini, bukan schema public.
	var clientIDDefault, redirectURIDefault, tokenURIDefault *string
	err := pool.QueryRow(ctx, `
		select
			(select column_default from information_schema.columns
			 where table_schema = current_schema() and table_name = 'provider_oauth_sessions' and column_name = 'client_id'),
			(select column_default from information_schema.columns
			 where table_schema = current_schema() and table_name = 'provider_oauth_sessions' and column_name = 'redirect_uri'),
			(select column_default from information_schema.columns
			 where table_schema = current_schema() and table_name = 'provider_oauth_sessions' and column_name = 'token_uri')`).
		Scan(&clientIDDefault, &redirectURIDefault, &tokenURIDefault)
	if err != nil {
		t.Fatalf("membaca default kolom provider_oauth_sessions: %v", err)
	}

	if clientIDDefault != nil {
		t.Errorf("kolom client_id masih punya default %q — seharusnya dihapus migrasi 0012", *clientIDDefault)
	}
	if redirectURIDefault != nil {
		t.Errorf("kolom redirect_uri masih punya default %q — seharusnya dihapus migrasi 0012", *redirectURIDefault)
	}
	if tokenURIDefault == nil {
		t.Error("kolom token_uri kehilangan default-nya — endpoint token Google adalah konstanta vendor yang stabil dan dipertahankan")
	}
}

// TestOAuthInsertWithoutClientIDRejected membuktikan skema sekarang menolak insert
// yang tidak menyatakan client_id (atau redirect_uri) dengan pelanggaran NOT NULL —
// bukannya diam-diam memakai identitas vendor seperti perilaku migrasi 0011.
func TestOAuthInsertWithoutClientIDRejected(t *testing.T) {
	if testing.Short() {
		t.Skip("butuh PostgreSQL")
	}
	ctx, cancel := context.WithTimeout(context.Background(), 90*time.Second)
	defer cancel()

	pool := newTestPool(ctx, t)
	cipher := testCipher(t, 0xB2)
	cred := seedOAuthCredential(ctx, t, pool, cipher, "provider-oauth-skema")

	t.Run("tanpa client_id", func(t *testing.T) {
		id, err := newID()
		if err != nil {
			t.Fatalf("membuat id: %v", err)
		}
		// redirect_uri dinyatakan eksplisit: kita menguji client_id saja.
		_, err = pool.Exec(ctx, `
			insert into provider_oauth_sessions (
				id, provider_id, credential_id, account_email,
				encryption_key_id, refresh_token_encrypted, redirect_uri
			) values ($1, $2, $3, $4, $5, $6, $7)`,
			id, cred.ProviderID, cred.ID, "tanpa-client-id@contoh.test",
			cipher.KeyID(), "ciphertext-palsu", "http://localhost:14567")

		if err == nil {
			t.Fatal("insert tanpa client_id berhasil — default vendor masih ada di skema")
		}
		var pgErr *pgconn.PgError
		if !errors.As(err, &pgErr) {
			t.Fatalf("error bukan PgError: %v", err)
		}
		if pgErr.Code != "23502" {
			t.Fatalf("error code = %q, ingin 23502 (not_null_violation): %v", pgErr.Code, pgErr)
		}
		if !strings.Contains(pgErr.Message, "client_id") {
			t.Fatalf("pesan error tidak menyebut client_id: %q", pgErr.Message)
		}
	})

	t.Run("tanpa redirect_uri", func(t *testing.T) {
		id, err := newID()
		if err != nil {
			t.Fatalf("membuat id: %v", err)
		}
		_, err = pool.Exec(ctx, `
			insert into provider_oauth_sessions (
				id, provider_id, credential_id, account_email,
				encryption_key_id, refresh_token_encrypted, client_id
			) values ($1, $2, $3, $4, $5, $6, $7)`,
			id, cred.ProviderID, cred.ID, "tanpa-redirect@contoh.test",
			cipher.KeyID(), "ciphertext-palsu", "client-id-eksplisit")

		if err == nil {
			t.Fatal("insert tanpa redirect_uri berhasil — default localhost masih ada di skema")
		}
		var pgErr *pgconn.PgError
		if !errors.As(err, &pgErr) {
			t.Fatalf("error bukan PgError: %v", err)
		}
		if pgErr.Code != "23502" {
			t.Fatalf("error code = %q, ingin 23502 (not_null_violation): %v", pgErr.Code, pgErr)
		}
		if !strings.Contains(pgErr.Message, "redirect_uri") {
			t.Fatalf("pesan error tidak menyebut redirect_uri: %q", pgErr.Message)
		}
	})
}

// TestOAuthUpsertFallsBackToApplicationConstants memastikan perilaku aplikasi tidak
// berubah: pemanggil yang tidak meneruskan client_id/redirect_uri tetap dapat sesi
// tersimpan — tapi nilainya sekarang datang dari konstanta eksplisit di kode Go,
// bukan dari DEFAULT yang disembunyikan skema.
func TestOAuthUpsertFallsBackToApplicationConstants(t *testing.T) {
	if testing.Short() {
		t.Skip("butuh PostgreSQL")
	}
	ctx, cancel := context.WithTimeout(context.Background(), 90*time.Second)
	defer cancel()

	pool := newTestPool(ctx, t)
	cipher := testCipher(t, 0xC3)
	cred := seedOAuthCredential(ctx, t, pool, cipher, "provider-oauth-fallback")

	repo, err := NewOAuthRepo(pool, cipher)
	if err != nil {
		t.Fatalf("NewOAuthRepo: %v", err)
	}

	// ClientID dan RedirectURI sengaja dikosongkan seperti pemanggil di admin handler.
	meta, err := repo.UpsertSession(ctx, UpsertOAuthSessionParams{
		ProviderID:   cred.ProviderID,
		CredentialID: cred.ID,
		AccountEmail: "fallback-konstanta@contoh.test",
		Scopes:       []string{"https://www.googleapis.com/auth/cloud-platform"},
		RefreshToken: security.Secret("rt-oauth-rahasia-xyzzy"),
	})
	if err != nil {
		t.Fatalf("UpsertSession: %v", err)
	}

	if meta.ClientID != oauth.DefaultAntigravityClientID {
		t.Errorf("client_id = %q, ingin konstanta aplikasi %q", meta.ClientID, oauth.DefaultAntigravityClientID)
	}
	if meta.RedirectURI != oauth.DefaultRedirectURI {
		t.Errorf("redirect_uri = %q, ingin konstanta aplikasi %q", meta.RedirectURI, oauth.DefaultRedirectURI)
	}

	// Nilai di baris database harus persis sama dengan konstanta, bukan literal
	// duplikat — inilah yang membuat fallback aplikasi bisa diaudit.
	var dbClientID, dbRedirectURI, dbTokenURI string
	err = pool.QueryRow(ctx, `
		select client_id, redirect_uri, token_uri
		from provider_oauth_sessions where id = $1`, meta.ID).
		Scan(&dbClientID, &dbRedirectURI, &dbTokenURI)
	if err != nil {
		t.Fatalf("membaca baris sesi: %v", err)
	}
	if dbClientID != oauth.DefaultAntigravityClientID {
		t.Errorf("baris db client_id = %q, ingin %q", dbClientID, oauth.DefaultAntigravityClientID)
	}
	if dbRedirectURI != oauth.DefaultRedirectURI {
		t.Errorf("baris db redirect_uri = %q, ingin %q", dbRedirectURI, oauth.DefaultRedirectURI)
	}
	if dbTokenURI != oauth.GoogleTokenEndpoint {
		t.Errorf("baris db token_uri = %q, ingin %q", dbTokenURI, oauth.GoogleTokenEndpoint)
	}
}
