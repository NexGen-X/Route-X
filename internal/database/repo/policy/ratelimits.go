package policy

import (
	"context"
	"fmt"
	"strings"
	"time"

	"github.com/NexGen-X/Route-X/internal/database/repo"
)

// RateLimit adalah satu baris rate_limits: batas untuk satu cakupan.
//
// Seluruh batas bertipe pointer karena NULL punya arti sendiri di skema ini — "tidak
// dibatasi pada cakupan ini", yang berbeda dari nol. Nol pada kolom batas akan berarti
// "tidak boleh satu pun permintaan", dan constraint tabel tidak melarangnya, jadi
// perbedaan itu harus dipertahankan sampai ke pemakainya.
type RateLimit struct {
	ID      string
	Scope   string
	ScopeID string

	RequestsPerSecond   *int
	RequestsPerMinute   *int
	TokensPerMinute     *int
	DailyRequestLimit   *int64
	MonthlyRequestLimit *int64
	DailyTokenLimit     *int64
	MonthlyTokenLimit   *int64

	Enabled   bool
	CreatedAt time.Time
	UpdatedAt time.Time
}

const rateLimitColumns = `id::text, scope, coalesce(scope_id, ''),
	requests_per_second, requests_per_minute, tokens_per_minute,
	daily_request_limit, monthly_request_limit, daily_token_limit, monthly_token_limit,
	enabled, created_at, updated_at`

// scanRateLimit membaca satu baris rate_limits sesuai rateLimitColumns.
func scanRateLimit(s interface{ Scan(...any) error }) (*RateLimit, error) {
	var l RateLimit
	err := s.Scan(&l.ID, &l.Scope, &l.ScopeID,
		&l.RequestsPerSecond, &l.RequestsPerMinute, &l.TokensPerMinute,
		&l.DailyRequestLimit, &l.MonthlyRequestLimit, &l.DailyTokenLimit, &l.MonthlyTokenLimit,
		&l.Enabled, &l.CreatedAt, &l.UpdatedAt)
	if err != nil {
		return nil, err
	}
	return &l, nil
}

// ActiveRateLimits mengembalikan SELURUH batas laju yang aktif.
//
// Seluruhnya, bukan yang cocok dengan satu permintaan: pemanggil mencocokkannya di memori
// dari salinan ber-TTL. Lihat catatan paket untuk alasannya.
//
// Urutannya menempatkan cakupan global lebih dulu, lalu per cakupan menurut waktu
// pembuatan. Urutan yang tetap membuat penggabungan batas menghasilkan hasil yang sama
// setiap kali — dua baris pada cakupan yang sama untuk entitas yang sama memang tidak
// dilarang skema, dan tanpa urutan yang tetap batas mana yang menang akan berpindah-pindah
// antar instance.
func (r *Repo) ActiveRateLimits(ctx context.Context) ([]*RateLimit, error) {
	const op = "mengambil batas laju aktif"

	rows, err := r.q.Query(ctx, `
		select `+rateLimitColumns+`
		from rate_limits
		where enabled
		order by (scope <> 'global'), scope, created_at, id`)
	if err != nil {
		return nil, repo.Err(op, err)
	}
	defer rows.Close()

	var out []*RateLimit
	for rows.Next() {
		l, err := scanRateLimit(rows)
		if err != nil {
			return nil, repo.Err(op, err)
		}
		out = append(out, l)
	}
	if err := rows.Err(); err != nil {
		return nil, repo.Err(op, err)
	}
	return out, nil
}

// CreateRateLimitParams adalah masukan pembuatan batas laju.
type CreateRateLimitParams struct {
	Scope   string
	ScopeID string

	RequestsPerSecond   *int
	RequestsPerMinute   *int
	TokensPerMinute     *int
	DailyRequestLimit   *int64
	MonthlyRequestLimit *int64
	DailyTokenLimit     *int64
	MonthlyTokenLimit   *int64

	CreatedBy string
}

// CreateRateLimit menyimpan satu batas laju baru.
//
// Bentuk masukan tidak divalidasi di sini: constraint rate_limits_scope_valid,
// rate_limits_scope_id_presence, dan rate_limits_at_least_one sudah menegakkannya di
// database, dan menyalin aturan yang sama ke Go berarti dua tempat yang bisa berbeda
// pendapat. Yang dikembalikan adalah repo.ErrConstraint dengan nama constraint-nya, yang
// cukup bagi lapisan HTTP untuk menyusun pesan.
func (r *Repo) CreateRateLimit(ctx context.Context, p CreateRateLimitParams) (*RateLimit, error) {
	const op = "membuat batas laju"

	var createdBy any
	if p.CreatedBy != "" {
		if !idOK(p.CreatedBy) {
			return nil, fmt.Errorf("%s: %w: created_by bukan UUID", op, repo.ErrInvalidReference)
		}
		createdBy = p.CreatedBy
	}

	row := r.q.QueryRow(ctx, `
		insert into rate_limits (scope, scope_id, requests_per_second, requests_per_minute,
			tokens_per_minute, daily_request_limit, monthly_request_limit,
			daily_token_limit, monthly_token_limit, created_by)
		values ($1, $2, $3, $4, $5, $6, $7, $8, $9, $10)
		returning `+rateLimitColumns,
		p.Scope, nullIfEmpty(p.ScopeID),
		p.RequestsPerSecond, p.RequestsPerMinute, p.TokensPerMinute,
		p.DailyRequestLimit, p.MonthlyRequestLimit, p.DailyTokenLimit, p.MonthlyTokenLimit,
		createdBy)

	l, err := scanRateLimit(row)
	if err != nil {
		return nil, repo.Err(op, err)
	}
	return l, nil
}

// SetRateLimitEnabled menyalakan atau mematikan satu batas laju.
//
// Dimatikan, bukan dihapus, adalah tindakan yang lebih sering dibutuhkan operator: batas
// yang ternyata terlalu ketat perlu dilepas dulu lalu dipasang lagi setelah angkanya
// diperbaiki, dan menghapusnya menghilangkan nilai lamanya.
func (r *Repo) SetRateLimitEnabled(ctx context.Context, id string, enabled bool) (*RateLimit, error) {
	const op = "mengubah status batas laju"
	if !idOK(id) {
		return nil, fmt.Errorf("%s: %w", op, repo.ErrNotFound)
	}

	row := r.q.QueryRow(ctx, `
		update rate_limits set enabled = $2, updated_at = now()
		where id = $1
		returning `+rateLimitColumns, id, enabled)

	l, err := scanRateLimit(row)
	if err != nil {
		return nil, repo.Err(op, err)
	}
	return l, nil
}

// DeleteRateLimit menghapus satu batas laju.
func (r *Repo) DeleteRateLimit(ctx context.Context, id string) error {
	const op = "menghapus batas laju"
	if !idOK(id) {
		return fmt.Errorf("%s: %w", op, repo.ErrNotFound)
	}

	tag, err := r.q.Exec(ctx, `delete from rate_limits where id = $1`, id)
	if err != nil {
		return repo.Err(op, err)
	}
	if tag.RowsAffected() == 0 {
		return fmt.Errorf("%s: %w", op, repo.ErrNotFound)
	}
	return nil
}

// GetRateLimit mengambil satu baris rate_limits berdasarkan id.
func (r *Repo) GetRateLimit(ctx context.Context, id string) (*RateLimit, error) {
	const op = "mengambil batas laju"
	if !idOK(id) {
		return nil, fmt.Errorf("%s: %w", op, repo.ErrNotFound)
	}

	row := r.q.QueryRow(ctx, `select `+rateLimitColumns+` from rate_limits where id = $1`, id)
	l, err := scanRateLimit(row)
	if err != nil {
		return nil, repo.Err(op, err)
	}
	return l, nil
}

// UpdateRateLimitParams berisi opsi pembaruan batas laju.
type UpdateRateLimitParams struct {
	RequestsPerSecond   *int
	RequestsPerMinute   *int
	TokensPerMinute     *int
	DailyRequestLimit   *int64
	MonthlyRequestLimit *int64
	DailyTokenLimit     *int64
	MonthlyTokenLimit   *int64
	Enabled             *bool
}

// UpdateRateLimit memperbarui nilai pembatasan laju.
func (r *Repo) UpdateRateLimit(ctx context.Context, id string, p UpdateRateLimitParams) (*RateLimit, error) {
	const op = "memperbarui batas laju"
	if !idOK(id) {
		return nil, fmt.Errorf("%s: %w", op, repo.ErrNotFound)
	}

	var (
		setClauses []string
		args       []any
	)
	args = append(args, id)

	if p.RequestsPerSecond != nil {
		args = append(args, *p.RequestsPerSecond)
		setClauses = append(setClauses, fmt.Sprintf("requests_per_second = $%d", len(args)))
	}
	if p.RequestsPerMinute != nil {
		args = append(args, *p.RequestsPerMinute)
		setClauses = append(setClauses, fmt.Sprintf("requests_per_minute = $%d", len(args)))
	}
	if p.TokensPerMinute != nil {
		args = append(args, *p.TokensPerMinute)
		setClauses = append(setClauses, fmt.Sprintf("tokens_per_minute = $%d", len(args)))
	}
	if p.DailyRequestLimit != nil {
		args = append(args, *p.DailyRequestLimit)
		setClauses = append(setClauses, fmt.Sprintf("daily_request_limit = $%d", len(args)))
	}
	if p.MonthlyRequestLimit != nil {
		args = append(args, *p.MonthlyRequestLimit)
		setClauses = append(setClauses, fmt.Sprintf("monthly_request_limit = $%d", len(args)))
	}
	if p.DailyTokenLimit != nil {
		args = append(args, *p.DailyTokenLimit)
		setClauses = append(setClauses, fmt.Sprintf("daily_token_limit = $%d", len(args)))
	}
	if p.MonthlyTokenLimit != nil {
		args = append(args, *p.MonthlyTokenLimit)
		setClauses = append(setClauses, fmt.Sprintf("monthly_token_limit = $%d", len(args)))
	}
	if p.Enabled != nil {
		args = append(args, *p.Enabled)
		setClauses = append(setClauses, fmt.Sprintf("enabled = $%d", len(args)))
	}

	if len(setClauses) == 0 {
		return r.GetRateLimit(ctx, id)
	}

	setClauses = append(setClauses, "updated_at = now()")
	query := fmt.Sprintf(`update rate_limits set %s where id = $1 returning %s`,
		strings.Join(setClauses, ", "), rateLimitColumns)

	row := r.q.QueryRow(ctx, query, args...)
	l, err := scanRateLimit(row)
	if err != nil {
		return nil, repo.Err(op, err)
	}
	return l, nil
}

// ListRateLimits mengambil daftar seluruh aturan rate_limits dengan keyset pagination.
func (r *Repo) ListRateLimits(ctx context.Context, page repo.Page) ([]*RateLimit, string, error) {
	const op = "mendaftar batas laju"
	limit := page.Normalize()

	var (
		query strings.Builder
		args  []any
	)
	query.WriteString(`select ` + rateLimitColumns + ` from rate_limits `)

	if page.Cursor != "" {
		ts, id, err := decodeCursor(op, page.Cursor)
		if err != nil {
			return nil, "", err
		}
		args = append(args, ts, id)
		query.WriteString(`where (created_at, id) < ($1, $2) `)
	}

	args = append(args, limit+1)
	query.WriteString(fmt.Sprintf(`order by created_at desc, id desc limit $%d`, len(args)))

	rows, err := r.q.Query(ctx, query.String(), args...)
	if err != nil {
		return nil, "", repo.Err(op, err)
	}
	defer rows.Close()

	var items []*RateLimit
	for rows.Next() {
		l, err := scanRateLimit(rows)
		if err != nil {
			return nil, "", repo.Err(op, err)
		}
		items = append(items, l)
	}
	if err := rows.Err(); err != nil {
		return nil, "", repo.Err(op, err)
	}

	var next string
	if len(items) > limit {
		items = items[:limit]
		last := items[limit-1]
		next = encodeCursor(last.CreatedAt, last.ID)
	}

	return items, next, nil
}
