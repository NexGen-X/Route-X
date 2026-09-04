// Package keys memuat akses data untuk API key beserta pembatasannya: cakupan izin,
// daftar putih model dan provider, pembatasan IP, batas laju, dan blokir.
//
// API key di sini adalah kredensial klien terhadap gateway, berbeda dari kredensial
// provider yang dipakai gateway terhadap upstream.
package keys

import (
	"context"
	"errors"
	"fmt"
	"net/netip"
	"strings"
	"time"

	"github.com/NexGen-X/Route-X/internal/database/repo"
	"github.com/NexGen-X/Route-X/internal/security"
)

// Status API key.
const (
	StatusActive   = "active"
	StatusDisabled = "disabled"
	StatusRevoked  = "revoked"
)

// Cakupan izin yang dikenali. Harus sama dengan constraint api_keys_scopes_known
// di migrasi 0005 — kalau salah satu berubah, yang lain wajib ikut berubah.
const (
	ScopeInference  = "inference"
	ScopeModelsRead = "models:read"
	ScopeUsageRead  = "usage:read"
	ScopeAdminRead  = "admin:read"
	ScopeAdminWrite = "admin:write"
)

// ErrKeyNotUsable dikembalikan Authenticate ketika key ditemukan tetapi tidak boleh
// dipakai. Alasannya ada di Rejection agar pemanggil bisa memilih pesan dan kode
// status yang tepat, tanpa perlu menerka dari teks error.
var ErrKeyNotUsable = errors.New("API key tidak bisa dipakai")

// Alasan penolakan.
const (
	ReasonNotFound     = "not_found"
	ReasonDisabled     = "disabled"
	ReasonRevoked      = "revoked"
	ReasonExpired      = "expired"
	ReasonIPNotAllowed = "ip_not_allowed"
	ReasonBanned       = "banned"
)

// Rejection menjelaskan kenapa sebuah key ditolak.
type Rejection struct {
	Reason string
}

func (r *Rejection) Error() string { return "API key ditolak: " + r.Reason }
func (r *Rejection) Unwrap() error { return ErrKeyNotUsable }

// Key adalah baris api_keys sebagaimana dibutuhkan jalur autentikasi.
//
// Nilai mentah key tidak pernah ada di tipe ini — hanya bentuk tersamar. Satu-satunya
// tempat nilai mentah muncul adalah hasil Create, sekali saja.
type Key struct {
	ID          string
	Name        string
	Prefix      string
	Last4       string
	OwnerUserID string
	Status      string
	Scopes      []string

	RateLimitRPS        *int
	RateLimitRPM        *int
	RateLimitTPM        *int
	DailyRequestLimit   *int64
	MonthlyRequestLimit *int64
	DailyTokenLimit     *int64
	MonthlyTokenLimit   *int64

	IPAllowlist []netip.Prefix

	ExpiresAt  *time.Time
	LastUsedAt *time.Time
	RevokedAt  *time.Time

	CreatedAt time.Time
	UpdatedAt time.Time
}

// Masked mengembalikan bentuk siap tampil, mis. "sk_live_****9a21".
func (k *Key) Masked() string { return security.MaskedFromParts(k.Prefix, k.Last4) }

// HasScope melaporkan apakah key memiliki cakupan tertentu.
func (k *Key) HasScope(scope string) bool {
	for _, s := range k.Scopes {
		if s == scope {
			return true
		}
	}
	return false
}

// Created adalah hasil pembuatan key baru.
type Created struct {
	Key *Key
	// Raw hanya ada di sini, sekali. Setelah respons dikirim ke pembuatnya, nilai ini
	// tidak bisa didapat lagi dari mana pun.
	Raw security.Secret
}

// CreateParams adalah masukan pembuatan key.
type CreateParams struct {
	Name        string
	OwnerUserID string
	Live        bool
	Scopes      []string

	RateLimitRPS        *int
	RateLimitRPM        *int
	RateLimitTPM        *int
	DailyRequestLimit   *int64
	MonthlyRequestLimit *int64
	DailyTokenLimit     *int64
	MonthlyTokenLimit   *int64

	IPAllowlist []netip.Prefix
	ExpiresAt   *time.Time
	CreatedBy   *string

	// AllowedModelIDs dan AllowedProviderIDs kosong berarti tanpa pembatasan.
	AllowedModelIDs    []string
	AllowedProviderIDs []string
}

// Repo adalah akses data API key.
type Repo struct {
	q      repo.Querier
	pepper []byte
}

// New membuat Repo. pepper berasal dari config.APIKeyPepper dan wajib tidak kosong;
// tanpa pepper, dump database sudah cukup untuk memverifikasi key hasil tebakan.
func New(q repo.Querier, pepper []byte) (*Repo, error) {
	if len(pepper) == 0 {
		return nil, security.ErrEmptyPepper
	}
	return &Repo{q: q, pepper: pepper}, nil
}

// WithQuerier membuat salinan Repo yang berjalan di atas Querier baru (misal transaksi pgx.Tx dari repo.InTx).
// Dibutuhkan agar pembuatan key beserta batasannya berjalan dalam satu transaksi atomik.
func (r *Repo) WithQuerier(q repo.Querier) *Repo {
	return &Repo{q: q, pepper: r.pepper}
}

// kolom yang selalu dibaca, disatukan agar semua query memilih bentuk baris yang sama.
const keyColumns = `
	id::text, name, key_prefix, last4, owner_user_id::text, status, scopes,
	rate_limit_rps, rate_limit_rpm, rate_limit_tpm,
	daily_request_limit, monthly_request_limit, daily_token_limit, monthly_token_limit,
	ip_allowlist, expires_at, last_used_at, revoked_at, created_at, updated_at`

// scanKey membaca satu baris sesuai keyColumns.
func scanKey(row pgxRow) (*Key, error) {
	var k Key
	var allowlist []netip.Prefix
	err := row.Scan(
		&k.ID, &k.Name, &k.Prefix, &k.Last4, &k.OwnerUserID, &k.Status, &k.Scopes,
		&k.RateLimitRPS, &k.RateLimitRPM, &k.RateLimitTPM,
		&k.DailyRequestLimit, &k.MonthlyRequestLimit, &k.DailyTokenLimit, &k.MonthlyTokenLimit,
		&allowlist, &k.ExpiresAt, &k.LastUsedAt, &k.RevokedAt, &k.CreatedAt, &k.UpdatedAt,
	)
	if err != nil {
		return nil, err
	}
	k.IPAllowlist = allowlist
	return &k, nil
}

// pgxRow adalah antarmuka minimum yang dipenuhi pgx.Row dan pgx.Rows.
type pgxRow interface {
	Scan(dest ...any) error
}

// Create membuat API key baru beserta pembatasannya.
//
// Pembuatan key, daftar putih model, dan daftar putih provider ditulis dalam satu
// transaksi lewat Querier yang sama. Kalau salah satu daftar putih gagal, key tidak
// boleh tetap ada tanpa pembatasan yang diminta — key tanpa pembatasan justru lebih
// longgar daripada yang diinginkan operator.
func (r *Repo) Create(ctx context.Context, p CreateParams) (*Created, error) {
	const op = "membuat API key"

	generated, err := security.GenerateAPIKey(p.Live, r.pepper)
	if err != nil {
		return nil, fmt.Errorf("%s: %w", op, err)
	}

	scopes := p.Scopes
	if scopes == nil {
		scopes = []string{}
	}
	allowlist := p.IPAllowlist
	if allowlist == nil {
		allowlist = []netip.Prefix{}
	}

	var ownerID any = p.OwnerUserID
	if p.OwnerUserID == "" {
		ownerID = nil
	}

	row := r.q.QueryRow(ctx, `
		insert into api_keys (
			name, key_hash, key_prefix, last4, owner_user_id, scopes,
			rate_limit_rps, rate_limit_rpm, rate_limit_tpm,
			daily_request_limit, monthly_request_limit, daily_token_limit, monthly_token_limit,
			ip_allowlist, expires_at, created_by
		) values ($1, $2, $3, $4, $5, $6, $7, $8, $9, $10, $11, $12, $13, $14, $15, $16)
		returning `+keyColumns,
		p.Name, generated.Hash, generated.Prefix, generated.Last4, ownerID, scopes,
		p.RateLimitRPS, p.RateLimitRPM, p.RateLimitTPM,
		p.DailyRequestLimit, p.MonthlyRequestLimit, p.DailyTokenLimit, p.MonthlyTokenLimit,
		allowlist, p.ExpiresAt, p.CreatedBy,
	)

	key, err := scanKey(row)
	if err != nil {
		return nil, repo.Err(op, err)
	}

	if err := r.replaceAllowed(ctx, key.ID, p.AllowedModelIDs, p.AllowedProviderIDs); err != nil {
		return nil, err
	}

	return &Created{Key: key, Raw: generated.Raw}, nil
}

// Authenticate memverifikasi key mentah dari request dan mengembalikan barisnya.
//
// clientIP dipakai untuk menegakkan ip_allowlist dan memeriksa blokir. Pemeriksaan itu
// dilakukan di dalam query, bukan di Go, agar pencocokan subnet memakai operator inet
// PostgreSQL dan tidak ada jalur yang lupa memeriksanya.
func (r *Repo) Authenticate(ctx context.Context, raw string, clientIP netip.Addr) (*Key, error) {
	const op = "memverifikasi API key"

	// Bentuk key diperiksa lebih dulu agar lalu lintas sampah tidak menyentuh database.
	if _, err := security.ParseAPIKey(raw); err != nil {
		return nil, &Rejection{Reason: ReasonNotFound}
	}

	hash := security.HashAPIKey(raw, r.pepper)

	var ip any
	if clientIP.IsValid() {
		ip = clientIP.String()
	}

	row := r.q.QueryRow(ctx, `
		select `+keyColumns+`,
			-- Alasan penolakan dihitung di query supaya satu perjalanan ke database
			-- cukup, dan supaya urutan pemeriksaannya sama untuk semua pemanggil.
			case
				when status = 'revoked'                       then 'revoked'
				when status = 'disabled'                      then 'disabled'
				when expires_at is not null
				     and expires_at <= now()                  then 'expired'
				when cardinality(ip_allowlist) > 0
				     and ($2::inet is null
				          or not exists (select 1 from unnest(ip_allowlist) net
				                         where $2::inet <<= net)) then 'ip_not_allowed'
				when exists (
					select 1 from bans b
					where b.lifted_at is null
					  and (b.expires_at is null or b.expires_at > now())
					  and (
						(b.subject_kind = 'api_key' and b.subject = api_keys.id::text)
						or (b.subject_kind = 'user'  and b.subject = api_keys.owner_user_id::text)
						or ($2::inet is not null and b.subject_kind = 'ip'       and b.subject::inet  = $2::inet)
						or ($2::inet is not null and b.subject_kind = 'ip_range' and $2::inet <<= b.subject::cidr)
					  )
				) then 'banned'
				else null
			end as rejection
		from api_keys
		where key_hash = $1`,
		hash, ip,
	)

	var k Key
	var allowlist []netip.Prefix
	var rejection *string
	err := row.Scan(
		&k.ID, &k.Name, &k.Prefix, &k.Last4, &k.OwnerUserID, &k.Status, &k.Scopes,
		&k.RateLimitRPS, &k.RateLimitRPM, &k.RateLimitTPM,
		&k.DailyRequestLimit, &k.MonthlyRequestLimit, &k.DailyTokenLimit, &k.MonthlyTokenLimit,
		&allowlist, &k.ExpiresAt, &k.LastUsedAt, &k.RevokedAt, &k.CreatedAt, &k.UpdatedAt,
		&rejection,
	)
	if err != nil {
		if wrapped := repo.Err(op, err); errors.Is(wrapped, repo.ErrNotFound) {
			// Key tidak ada dan key yang tidak boleh dipakai dibedakan di sini, tetapi
			// pemanggil HTTP harus tetap membalas 401 yang sama untuk keduanya agar
			// tidak menjadi alat pengintaian.
			return nil, &Rejection{Reason: ReasonNotFound}
		}
		return nil, repo.Err(op, err)
	}
	k.IPAllowlist = allowlist

	if rejection != nil {
		return nil, &Rejection{Reason: *rejection}
	}
	return &k, nil
}

// TouchUsage mencatat pemakaian terakhir. Dipanggil setelah request selesai, di luar
// jalur kritis, dan kegagalannya tidak boleh menggagalkan request.
func (r *Repo) TouchUsage(ctx context.Context, id string, ip netip.Addr) error {
	var ipArg any
	if ip.IsValid() {
		ipArg = ip.String()
	}
	_, err := r.q.Exec(ctx, `
		update api_keys set last_used_at = now(), last_used_ip = $2::inet
		where id = $1`, id, ipArg)
	return repo.Err("mencatat pemakaian API key", err)
}

// replaceAllowed mengganti seluruh daftar putih model dan provider satu key.
func (r *Repo) replaceAllowed(ctx context.Context, keyID string, modelIDs, providerIDs []string) error {
	if _, err := r.q.Exec(ctx, `delete from api_key_allowed_models where api_key_id = $1`, keyID); err != nil {
		return repo.Err("menghapus daftar putih model", err)
	}
	if len(modelIDs) > 0 {
		// unnest dipakai agar seluruh daftar masuk dalam satu perjalanan ke database,
		// bukan satu insert per baris.
		if _, err := r.q.Exec(ctx, `
			insert into api_key_allowed_models (api_key_id, model_id)
			select $1, unnest($2::uuid[])`, keyID, modelIDs); err != nil {
			return repo.Err("menyimpan daftar putih model", err)
		}
	}

	if _, err := r.q.Exec(ctx, `delete from api_key_allowed_providers where api_key_id = $1`, keyID); err != nil {
		return repo.Err("menghapus daftar putih provider", err)
	}
	if len(providerIDs) > 0 {
		if _, err := r.q.Exec(ctx, `
			insert into api_key_allowed_providers (api_key_id, provider_id)
			select $1, unnest($2::uuid[])`, keyID, providerIDs); err != nil {
			return repo.Err("menyimpan daftar putih provider", err)
		}
	}
	return nil
}

// SetAllowed mengganti daftar putih model dan provider satu key.
func (r *Repo) SetAllowed(ctx context.Context, keyID string, modelIDs, providerIDs []string) error {
	return r.replaceAllowed(ctx, keyID, modelIDs, providerIDs)
}

// AllowsModel melaporkan apakah key boleh memakai model tertentu.
//
// Tidak adanya baris pembatasan berarti semua model diizinkan; pemeriksaan itu ada di
// dalam query supaya aturan "kosong berarti semua" tidak perlu diulang di setiap
// pemanggil.
func (r *Repo) AllowsModel(ctx context.Context, keyID, modelID string) (bool, error) {
	var allowed bool
	err := r.q.QueryRow(ctx, `
		select not exists (select 1 from api_key_allowed_models where api_key_id = $1)
		    or exists (select 1 from api_key_allowed_models where api_key_id = $1 and model_id = $2)`,
		keyID, modelID).Scan(&allowed)
	return allowed, repo.Err("memeriksa izin model", err)
}

// AllowsProvider melaporkan apakah key boleh memakai provider tertentu.
func (r *Repo) AllowsProvider(ctx context.Context, keyID, providerID string) (bool, error) {
	var allowed bool
	err := r.q.QueryRow(ctx, `
		select not exists (select 1 from api_key_allowed_providers where api_key_id = $1)
		    or exists (select 1 from api_key_allowed_providers where api_key_id = $1 and provider_id = $2)`,
		keyID, providerID).Scan(&allowed)
	return allowed, repo.Err("memeriksa izin provider", err)
}

// GetByID mengambil satu key.
func (r *Repo) GetByID(ctx context.Context, id string) (*Key, error) {
	key, err := scanKey(r.q.QueryRow(ctx, `select `+keyColumns+` from api_keys where id = $1`, id))
	if err != nil {
		return nil, repo.Err("mengambil API key", err)
	}
	return key, nil
}

// ListFilter menyaring daftar key.
type ListFilter struct {
	OwnerUserID string
	Status      string
	Search      string
}

// List mengembalikan key berpaginasi keyset, terbaru dulu.
//
// Kursornya adalah created_at baris terakhir yang sudah diberikan; keyset dipakai
// daripada OFFSET agar biaya per halaman tetap sama seberapa jauh pun halamannya.
func (r *Repo) List(ctx context.Context, f ListFilter, page repo.Page) ([]*Key, string, error) {
	const op = "mendaftar API key"
	limit := page.Normalize()

	var (
		conds []string
		args  []any
	)
	add := func(cond string, arg any) {
		args = append(args, arg)
		conds = append(conds, fmt.Sprintf(cond, len(args)))
	}

	if f.OwnerUserID != "" {
		add("owner_user_id = $%d", f.OwnerUserID)
	}
	if f.Status != "" {
		add("status = $%d", f.Status)
	}
	if f.Search != "" {
		// Pencarian pada nama dan empat karakter terakhir; keduanya bukan rahasia.
		// Satu argumen dipakai dua kali, jadi kondisinya dirakit langsung dan bukan
		// lewat add() yang mengasumsikan satu placeholder per argumen.
		args = append(args, f.Search)
		n := len(args)
		conds = append(conds, fmt.Sprintf("(name ilike '%%' || $%d || '%%' or last4 = $%d)", n, n))
	}
	if page.Cursor != "" {
		cursor, err := time.Parse(time.RFC3339Nano, page.Cursor)
		if err != nil {
			return nil, "", fmt.Errorf("%s: kursor tidak valid", op)
		}
		add("created_at < $%d", cursor)
	}

	where := ""
	if len(conds) > 0 {
		where = " where " + strings.Join(conds, " and ")
	}
	args = append(args, limit+1) // satu baris lebih untuk mengetahui ada halaman lagi

	rows, err := r.q.Query(ctx, `select `+keyColumns+` from api_keys`+where+
		fmt.Sprintf(` order by created_at desc limit $%d`, len(args)), args...)
	if err != nil {
		return nil, "", repo.Err(op, err)
	}
	defer rows.Close()

	out := make([]*Key, 0, limit)
	for rows.Next() {
		key, err := scanKey(rows)
		if err != nil {
			return nil, "", repo.Err(op, err)
		}
		out = append(out, key)
	}
	if err := rows.Err(); err != nil {
		return nil, "", repo.Err(op, err)
	}

	next := ""
	if len(out) > limit {
		next = out[limit-1].CreatedAt.Format(time.RFC3339Nano)
		out = out[:limit]
	}
	return out, next, nil
}
