package upstream

import (
	"context"
	"encoding/json"
	"fmt"
	"time"

	"github.com/NexGen-X/Route-X/internal/database/repo"
	"github.com/NexGen-X/Route-X/internal/oauth"
	"github.com/NexGen-X/Route-X/internal/security"
)

// OAuthSessionMeta adalah metadata sesi otorisasi OAuth upstream untuk pemantauan UI.
type OAuthSessionMeta struct {
	ID               string     `json:"id"`
	ProviderID       string     `json:"provider_id"`
	CredentialID     string     `json:"credential_id"`
	AccountEmail     string     `json:"account_email"`
	AccountName      *string    `json:"account_name,omitempty"`
	ClientID         string     `json:"client_id"`
	RedirectURI      string     `json:"redirect_uri"`
	Scopes           []string   `json:"scopes"`
	LastRefreshedAt  *time.Time `json:"last_refreshed_at,omitempty"`
	LastRefreshError *string    `json:"last_refresh_error,omitempty"`
	Enabled          bool       `json:"enabled"`
	ExpiresAt        *time.Time `json:"expires_at,omitempty"`
	MaskedHint       string     `json:"masked_hint,omitempty"`
	EgressPoolID     *string    `json:"egress_pool_id,omitempty"`
	CreatedAt        time.Time  `json:"created_at"`
	UpdatedAt        time.Time  `json:"updated_at"`
}

// ActiveOAuthSession adalah sesi OAuth aktif dengan refresh token terdekripsi.
type ActiveOAuthSession struct {
	ID           string
	ProviderID   string
	CredentialID string
	AccountEmail string
	ClientID     string
	RedirectURI  string
	RefreshToken security.Secret
	EgressPoolID *string
}

// UpsertOAuthSessionParams adalah masukan penyimpanan atau pembaruan sesi OAuth.
type UpsertOAuthSessionParams struct {
	ProviderID   string
	CredentialID string
	AccountEmail string
	AccountName  *string
	ClientID     string
	RedirectURI  string
	Scopes       []string
	RefreshToken security.Secret
}

// OAuthRepo adalah repository untuk manajemen sesi OAuth provider.
type OAuthRepo struct {
	q      repo.Querier
	cipher *security.Cipher
}

// NewOAuthRepo membuat instance OAuthRepo dengan cipher untuk enkripsi refresh token.
func NewOAuthRepo(q repo.Querier, cipher *security.Cipher) (*OAuthRepo, error) {
	if cipher == nil {
		return nil, fmt.Errorf("oauth repo butuh cipher: %w", security.ErrInvalidKeyLength)
	}
	return &OAuthRepo{q: q, cipher: cipher}, nil
}

// WithQuerier membuat salinan OAuthRepo dengan querier baru (misal transaksi pgx.Tx).
func (r *OAuthRepo) WithQuerier(q repo.Querier) *OAuthRepo {
	return &OAuthRepo{q: q, cipher: r.cipher}
}

const oauthSessionJoinedColumns = `
	s.id::text, s.provider_id::text, s.credential_id::text, s.account_email, s.account_name,
	s.client_id, s.redirect_uri, s.scopes, s.last_refreshed_at, s.last_refresh_error,
	s.enabled, c.expires_at, c.masked_hint, s.created_at, s.updated_at, c.egress_pool_id::text`

func scanOAuthSessionJoined(row pgxRow) (*OAuthSessionMeta, error) {
	var m OAuthSessionMeta
	var scopesRaw []byte
	err := row.Scan(
		&m.ID, &m.ProviderID, &m.CredentialID, &m.AccountEmail, &m.AccountName,
		&m.ClientID, &m.RedirectURI, &scopesRaw, &m.LastRefreshedAt, &m.LastRefreshError,
		&m.Enabled, &m.ExpiresAt, &m.MaskedHint, &m.CreatedAt, &m.UpdatedAt, &m.EgressPoolID,
	)
	if err != nil {
		return nil, err
	}
	if len(scopesRaw) > 0 {
		_ = json.Unmarshal(scopesRaw, &m.Scopes)
	}
	return &m, nil
}

// UpsertSession menyimpan atau memperbarui refresh token sesi OAuth provider.
func (r *OAuthRepo) UpsertSession(ctx context.Context, p UpsertOAuthSessionParams) (*OAuthSessionMeta, error) {
	const op = "menyimpan sesi oauth provider"
	if !idOK(p.ProviderID) || !idOK(p.CredentialID) {
		return nil, fmt.Errorf("%s: %w: ID tidak valid", op, repo.ErrInvalidReference)
	}
	if p.RefreshToken.IsZero() {
		return nil, fmt.Errorf("%s: %w: refresh token kosong", op, repo.ErrConstraint)
	}

	scopesJSON, err := json.Marshal(p.Scopes)
	if err != nil {
		scopesJSON = []byte("[]")
	}

	// Buat ID baru jika baris belum ada
	id, err := newID()
	if err != nil {
		return nil, fmt.Errorf("%s: %w", op, err)
	}

	ciphertext, err := r.cipher.EncryptSecret(p.RefreshToken, security.OAuthSessionAAD(id))
	if err != nil {
		return nil, fmt.Errorf("%s: %w", op, err)
	}

	// Fallback nilai ada di layer aplikasi, bukan di DEFAULT skema (migrasi 0012
	// menghapus default vendor). Konstanta eksplisit di internal/oauth/google.go
	// mudah diaudit dan tidak menyembunyikan identitas vendor di balik skema.
	clientID := p.ClientID
	if clientID == "" {
		clientID = oauth.DefaultAntigravityClientID
	}
	redirectURI := p.RedirectURI
	if redirectURI == "" {
		redirectURI = oauth.DefaultRedirectURI
	}
	tokenURI := oauth.GoogleTokenEndpoint

	query := `
		insert into provider_oauth_sessions (
			id, provider_id, credential_id, account_email, account_name,
			client_id, encryption_key_id, refresh_token_encrypted,
			token_uri, redirect_uri, scopes, last_refreshed_at, last_refresh_error,
			enabled
		) values (
			$1, $2, $3, $4, $5,
			$6, $7, $8,
			$9, $10, $11, now(), null,
			true
		)
		on conflict (provider_id, account_email) do update set
			credential_id = excluded.credential_id,
			account_name = coalesce(excluded.account_name, provider_oauth_sessions.account_name),
			client_id = excluded.client_id,
			encryption_key_id = excluded.encryption_key_id,
			refresh_token_encrypted = excluded.refresh_token_encrypted,
			redirect_uri = excluded.redirect_uri,
			scopes = excluded.scopes,
			last_refreshed_at = now(),
			last_refresh_error = null,
			enabled = true,
			updated_at = now()
		returning id::text`

	var actualID string
	err = r.q.QueryRow(ctx, query,
		id, p.ProviderID, p.CredentialID, p.AccountEmail, p.AccountName,
		clientID, r.cipher.KeyID(), ciphertext,
		tokenURI, redirectURI, scopesJSON,
	).Scan(&actualID)
	if err != nil {
		return nil, repo.Err(op, err)
	}

	// Jika on conflict update terjadi dan id tidak sama, re-encrypt dengan actualID agar AAD cocok
	if actualID != id {
		recipher, err := r.cipher.EncryptSecret(p.RefreshToken, security.OAuthSessionAAD(actualID))
		if err == nil {
			_, _ = r.q.Exec(ctx, `
				update provider_oauth_sessions
				set refresh_token_encrypted = $2, encryption_key_id = $3
				where id = $1`, actualID, recipher, r.cipher.KeyID())
		}
	}

	return r.GetSession(ctx, actualID)
}

// GetSession mengambil metadata sesi OAuth berdasarkan ID.
func (r *OAuthRepo) GetSession(ctx context.Context, id string) (*OAuthSessionMeta, error) {
	const op = "mengambil sesi oauth provider"
	if !idOK(id) {
		return nil, fmt.Errorf("%s: %w", op, repo.ErrNotFound)
	}

	query := `
		select ` + oauthSessionJoinedColumns + `
		from provider_oauth_sessions s
		join provider_credentials c on c.id = s.credential_id
		where s.id = $1`

	meta, err := scanOAuthSessionJoined(r.q.QueryRow(ctx, query, id))
	if err != nil {
		return nil, repo.Err(op, err)
	}
	return meta, nil
}

// ListSessionsByProviderID mengembalikan seluruh sesi OAuth pada satu provider.
func (r *OAuthRepo) ListSessionsByProviderID(ctx context.Context, providerID string) ([]*OAuthSessionMeta, error) {
	const op = "mendaftar sesi oauth provider"
	if !idOK(providerID) {
		return nil, fmt.Errorf("%s: %w: provider_id bukan UUID", op, repo.ErrInvalidReference)
	}

	query := `
		select ` + oauthSessionJoinedColumns + `
		from provider_oauth_sessions s
		join provider_credentials c on c.id = s.credential_id
		where s.provider_id = $1
		order by s.created_at asc`

	rows, err := r.q.Query(ctx, query, providerID)
	if err != nil {
		return nil, repo.Err(op, err)
	}
	defer rows.Close()

	var out []*OAuthSessionMeta
	for rows.Next() {
		m, err := scanOAuthSessionJoined(rows)
		if err != nil {
			return nil, repo.Err(op, err)
		}
		out = append(out, m)
	}
	if err := rows.Err(); err != nil {
		return nil, repo.Err(op, err)
	}
	return out, nil
}

// ListExpiringOAuthSessions mencari seluruh sesi aktif yang token access-nya mendekati masa kedaluwarsa.
func (r *OAuthRepo) ListExpiringOAuthSessions(ctx context.Context, threshold time.Duration) ([]*ActiveOAuthSession, error) {
	const op = "mencari sesi oauth kedaluwarsa"

	query := `
		select s.id::text, s.provider_id::text, s.credential_id::text,
		       s.account_email, s.client_id, s.redirect_uri, s.refresh_token_encrypted,
		       c.egress_pool_id::text
		from provider_oauth_sessions s
		join provider_credentials c on c.id = s.credential_id
		where s.enabled
		  and c.enabled
		  and c.expires_at is not null
		  and c.expires_at <= now() + ($1 * interval '1 second')`

	rows, err := r.q.Query(ctx, query, int64(threshold.Seconds()))
	if err != nil {
		return nil, repo.Err(op, err)
	}
	defer rows.Close()

	var out []*ActiveOAuthSession
	for rows.Next() {
		var id, provID, credID, email, clientID, redirectURI, ciphertext string
		var egressPoolID *string
		if err := rows.Scan(&id, &provID, &credID, &email, &clientID, &redirectURI, &ciphertext, &egressPoolID); err != nil {
			return nil, repo.Err(op, err)
		}

		secret, err := r.cipher.DecryptSecret(ciphertext, security.OAuthSessionAAD(id))
		if err != nil {
			// Jika gagal dekripsi (misal rotasi kunci belum tuntas), catat dan lanjutkan ke sesi lain
			continue
		}

		out = append(out, &ActiveOAuthSession{
			ID:           id,
			ProviderID:   provID,
			CredentialID: credID,
			AccountEmail: email,
			ClientID:     clientID,
			RedirectURI:  redirectURI,
			RefreshToken: secret,
			EgressPoolID: egressPoolID,
		})
	}
	if err := rows.Err(); err != nil {
		return nil, repo.Err(op, err)
	}
	return out, nil
}

// GetActiveSession mendekripsi refresh token satu sesi tertentu.
func (r *OAuthRepo) GetActiveSession(ctx context.Context, sessionID string) (*ActiveOAuthSession, error) {
	const op = "mengambil sesi oauth aktif"
	if !idOK(sessionID) {
		return nil, fmt.Errorf("%s: %w", op, repo.ErrNotFound)
	}

	var provID, credID, email, clientID, redirectURI, ciphertext string
	var egressPoolID *string
	err := r.q.QueryRow(ctx, `
		select s.provider_id::text, s.credential_id::text, s.account_email, s.client_id, s.redirect_uri, s.refresh_token_encrypted, c.egress_pool_id::text
		from provider_oauth_sessions s
		join provider_credentials c on c.id = s.credential_id
		where s.id = $1`, sessionID).Scan(&provID, &credID, &email, &clientID, &redirectURI, &ciphertext, &egressPoolID)
	if err != nil {
		return nil, repo.Err(op, err)
	}

	secret, err := r.cipher.DecryptSecret(ciphertext, security.OAuthSessionAAD(sessionID))
	if err != nil {
		return nil, fmt.Errorf("%s: %w", op, err)
	}

	return &ActiveOAuthSession{
		ID:           sessionID,
		ProviderID:   provID,
		CredentialID: credID,
		AccountEmail: email,
		ClientID:     clientID,
		RedirectURI:  redirectURI,
		RefreshToken: secret,
		EgressPoolID: egressPoolID,
	}, nil
}

// UpdateRefreshStatus memperbarui status terakhir refresh sesi OAuth.
func (r *OAuthRepo) UpdateRefreshStatus(ctx context.Context, sessionID string, errStr *string) error {
	const op = "memperbarui status refresh oauth"
	if !idOK(sessionID) {
		return fmt.Errorf("%s: %w", op, repo.ErrNotFound)
	}

	_, err := r.q.Exec(ctx, `
		update provider_oauth_sessions
		set last_refreshed_at = now(),
		    last_refresh_error = $2,
		    updated_at = now()
		where id = $1`, sessionID, errStr)
	if err != nil {
		return repo.Err(op, err)
	}
	return nil
}

// SetEnabled menyalakan atau mematikan sesi OAuth dan kredensial terkait secara bersamaan.
func (r *OAuthRepo) SetEnabled(ctx context.Context, sessionID string, enabled bool) error {
	const op = "mengubah status sesi oauth"
	if !idOK(sessionID) {
		return fmt.Errorf("%s: %w", op, repo.ErrNotFound)
	}

	var credID string
	err := r.q.QueryRow(ctx, `
		update provider_oauth_sessions
		set enabled = $2, updated_at = now()
		where id = $1
		returning credential_id::text`, sessionID, enabled).Scan(&credID)
	if err != nil {
		return repo.Err(op, err)
	}

	// Sinkronkan status enabled ke tabel provider_credentials
	_, err = r.q.Exec(ctx, `update provider_credentials set enabled = $2 where id = $1`, credID, enabled)
	if err != nil {
		return repo.Err(op, err)
	}
	return nil
}

// Delete menghapus sesi OAuth dan kredensial terkait dari database.
func (r *OAuthRepo) Delete(ctx context.Context, sessionID string) error {
	const op = "menghapus sesi oauth"
	if !idOK(sessionID) {
		return fmt.Errorf("%s: %w", op, repo.ErrNotFound)
	}

	var credID string
	err := r.q.QueryRow(ctx, `
		delete from provider_oauth_sessions
		where id = $1
		returning credential_id::text`, sessionID).Scan(&credID)
	if err != nil {
		return repo.Err(op, err)
	}

	// Hapus kredensial yang terikat
	_, _ = r.q.Exec(ctx, `delete from provider_credentials where id = $1`, credID)
	return nil
}
