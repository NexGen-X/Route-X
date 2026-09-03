package policy

import (
	"context"
	"fmt"
	"strings"
	"time"

	"github.com/NexGen-X/Route-X/internal/database/repo"
)

// ContentFilter adalah satu baris content_filters.
//
// Satu tipe untuk enam kind, dengan kolom yang hanya terisi pada kind tertentu. Bentuk itu
// mengikuti tabelnya, dan tabelnya begitu karena constraint content_filters_shape_valid
// sudah menjamin kolom yang penting bagi satu kind tidak mungkin kosong. Artinya pembaca di
// Go boleh mempercayai bentuknya tanpa memeriksa ulang — dan kalau suatu hari tidak boleh
// lagi, yang harus berubah adalah constraint itu, bukan pemeriksaan yang tersebar.
type ContentFilter struct {
	ID          string
	Name        string
	Description string

	Kind      string
	Priority  int
	AppliesTo string
	Action    string

	// Pola, hanya untuk kind blocked_pattern dan allowed_pattern.
	Pattern       string
	PatternType   string
	CaseSensitive bool
	// MaxEvalMS adalah anggaran waktu pencocokan satu aturan, ditegakkan aplikasi.
	MaxEvalMS int

	// Pembatasan entitas, hanya untuk kind model_restriction dan provider_restriction.
	ModelID    string
	ProviderID string

	// MaxRequestBytes hanya untuk kind request_size.
	MaxRequestBytes *int64

	// Moderasi eksternal, hanya untuk kind moderation.
	ModerationIntegrationID string
	ModerationCategories    []string
	ModerationThreshold     *float64

	Enabled   bool
	CreatedAt time.Time
	UpdatedAt time.Time
}

const filterColumns = `id::text, name, coalesce(description, ''), kind, priority, applies_to, action,
	coalesce(pattern, ''), coalesce(pattern_type, ''), case_sensitive, max_eval_ms,
	coalesce(model_id::text, ''), coalesce(provider_id::text, ''), max_request_bytes,
	coalesce(moderation_integration_id::text, ''), moderation_categories, moderation_threshold,
	enabled, created_at, updated_at`

// scanFilter membaca satu baris content_filters sesuai filterColumns.
func scanFilter(s interface{ Scan(...any) error }) (*ContentFilter, error) {
	var f ContentFilter
	err := s.Scan(&f.ID, &f.Name, &f.Description, &f.Kind, &f.Priority, &f.AppliesTo, &f.Action,
		&f.Pattern, &f.PatternType, &f.CaseSensitive, &f.MaxEvalMS,
		&f.ModelID, &f.ProviderID, &f.MaxRequestBytes,
		&f.ModerationIntegrationID, &f.ModerationCategories, &f.ModerationThreshold,
		&f.Enabled, &f.CreatedAt, &f.UpdatedAt)
	if err != nil {
		return nil, err
	}
	return &f, nil
}

// ActiveFilters mengembalikan seluruh penyaring aktif dalam urutan evaluasi.
//
// Urutannya priority menaik, lalu nama. Priority menaik karena itu yang tertulis di skema
// ("makin kecil makin dulu"), dan nama sebagai pemutus seri supaya urutannya sama di setiap
// instance: dua penyaring berpriority sama yang keduanya memblokir akan melaporkan aturan
// yang berbeda kepada pengguna kalau urutannya tidak tetap, dan operator yang menerima
// keluhan tidak akan bisa mereproduksinya.
func (r *Repo) ActiveFilters(ctx context.Context) ([]*ContentFilter, error) {
	const op = "mengambil penyaring konten aktif"

	rows, err := r.q.Query(ctx, `
		select `+filterColumns+`
		from content_filters
		where enabled
		order by priority, name, id`)
	if err != nil {
		return nil, repo.Err(op, err)
	}
	defer rows.Close()

	var out []*ContentFilter
	for rows.Next() {
		f, err := scanFilter(rows)
		if err != nil {
			return nil, repo.Err(op, err)
		}
		out = append(out, f)
	}
	if err := rows.Err(); err != nil {
		return nil, repo.Err(op, err)
	}
	return out, nil
}

// CreateFilterParams adalah masukan pembuatan penyaring konten.
type CreateFilterParams struct {
	Name        string
	Description string

	Kind      string
	Priority  int
	AppliesTo string
	Action    string

	Pattern       string
	PatternType   string
	CaseSensitive bool
	MaxEvalMS     int

	ModelID    string
	ProviderID string

	MaxRequestBytes *int64

	ModerationIntegrationID string
	ModerationCategories    []string
	ModerationThreshold     *float64

	CreatedBy string
}

// CreateFilter menyimpan satu penyaring konten baru.
//
// Kesesuaian bentuk dengan kind tidak diperiksa di sini: constraint
// content_filters_shape_valid sudah menegakkannya, dan menyalin aturannya ke Go akan
// menghasilkan dua definisi yang bisa berbeda pendapat setelah salah satu diubah.
func (r *Repo) CreateFilter(ctx context.Context, p CreateFilterParams) (*ContentFilter, error) {
	const op = "membuat penyaring konten"

	var createdBy any
	if p.CreatedBy != "" {
		if !idOK(p.CreatedBy) {
			return nil, fmt.Errorf("%s: %w: created_by bukan UUID", op, repo.ErrInvalidReference)
		}
		createdBy = p.CreatedBy
	}
	for nama, id := range map[string]string{"model_id": p.ModelID, "provider_id": p.ProviderID,
		"moderation_integration_id": p.ModerationIntegrationID} {
		if id != "" && !idOK(id) {
			return nil, fmt.Errorf("%s: %w: %s bukan UUID", op, repo.ErrInvalidReference, nama)
		}
	}

	priority := p.Priority
	if priority == 0 {
		priority = 100
	}
	appliesTo := p.AppliesTo
	if appliesTo == "" {
		appliesTo = AppliesToRequest
	}
	action := p.Action
	if action == "" {
		action = ActionBlock
	}
	maxEval := p.MaxEvalMS
	if maxEval == 0 {
		maxEval = 50
	}
	categories := p.ModerationCategories
	if categories == nil {
		categories = []string{}
	}

	row := r.q.QueryRow(ctx, `
		insert into content_filters (name, description, kind, priority, applies_to, action,
			pattern, pattern_type, case_sensitive, max_eval_ms,
			model_id, provider_id, max_request_bytes,
			moderation_integration_id, moderation_categories, moderation_threshold, created_by)
		values ($1, $2, $3, $4, $5, $6, $7, $8, $9, $10, $11, $12, $13, $14, $15, $16, $17)
		returning `+filterColumns,
		p.Name, nullIfEmpty(p.Description), p.Kind, priority, appliesTo, action,
		nullIfEmpty(p.Pattern), nullIfEmpty(p.PatternType), p.CaseSensitive, maxEval,
		nullIfEmpty(p.ModelID), nullIfEmpty(p.ProviderID), p.MaxRequestBytes,
		nullIfEmpty(p.ModerationIntegrationID), categories, p.ModerationThreshold, createdBy)

	f, err := scanFilter(row)
	if err != nil {
		return nil, repo.Err(op, err)
	}
	return f, nil
}

// SetFilterEnabled menyalakan atau mematikan satu penyaring.
func (r *Repo) SetFilterEnabled(ctx context.Context, id string, enabled bool) (*ContentFilter, error) {
	const op = "mengubah status penyaring konten"
	if !idOK(id) {
		return nil, fmt.Errorf("%s: %w", op, repo.ErrNotFound)
	}

	row := r.q.QueryRow(ctx, `
		update content_filters set enabled = $2, updated_at = now()
		where id = $1
		returning `+filterColumns, id, enabled)

	f, err := scanFilter(row)
	if err != nil {
		return nil, repo.Err(op, err)
	}
	return f, nil
}

// DeleteFilter menghapus satu penyaring.
func (r *Repo) DeleteFilter(ctx context.Context, id string) error {
	const op = "menghapus penyaring konten"
	if !idOK(id) {
		return fmt.Errorf("%s: %w", op, repo.ErrNotFound)
	}

	tag, err := r.q.Exec(ctx, `delete from content_filters where id = $1`, id)
	if err != nil {
		return repo.Err(op, err)
	}
	if tag.RowsAffected() == 0 {
		return fmt.Errorf("%s: %w", op, repo.ErrNotFound)
	}
	return nil
}

// RecordEvalTimeout mencatat bahwa pencocokan pola aturan ini melewati anggaran waktunya.
//
// Ini satu-satunya umpan balik yang dimiliki operator tentang pola yang terlalu mahal.
// Tanpa penghitung ini, aturan yang selalu kehabisan waktu akan tampak seperti aturan yang
// tidak pernah cocok — dan pada aturan pemblokir, "tidak pernah cocok" berarti
// perlindungan yang disangka aktif sebenarnya tidak pernah berlaku sekali pun.
//
// Kegagalannya tidak boleh menggagalkan permintaan: yang hilang adalah satu angka pada
// penghitung diagnostik, bukan jawaban bagi pengguna. Pemanggil di jalur request memanggil
// ini di luar jalur kritis.
func (r *Repo) RecordEvalTimeout(ctx context.Context, id string) error {
	const op = "mencatat waktu habis penyaring konten"
	if !idOK(id) {
		return fmt.Errorf("%s: %w", op, repo.ErrNotFound)
	}

	_, err := r.q.Exec(ctx, `
		update content_filters
		set eval_timeout_count = eval_timeout_count + 1, last_timeout_at = now()
		where id = $1`, id)
	if err != nil {
		return repo.Err(op, err)
	}
	return nil
}

// FilterStats adalah cuplikan umpan balik penegakan anggaran waktu satu penyaring.
type FilterStats struct {
	ID               string
	Name             string
	EvalTimeoutCount int64
	LastTimeoutAt    *time.Time
}

// TimingOutFilters mengembalikan penyaring yang pernah kehabisan waktu, terbanyak lebih
// dulu. Dipakai dashboard untuk menunjukkan aturan yang perlu diperbaiki operator.
func (r *Repo) TimingOutFilters(ctx context.Context, page repo.Page) ([]*FilterStats, error) {
	const op = "mengambil penyaring yang kehabisan waktu"

	rows, err := r.q.Query(ctx, `
		select id::text, name, eval_timeout_count, last_timeout_at
		from content_filters
		where eval_timeout_count > 0
		order by eval_timeout_count desc, name
		limit $1`, page.Normalize())
	if err != nil {
		return nil, repo.Err(op, err)
	}
	defer rows.Close()

	var out []*FilterStats
	for rows.Next() {
		var s FilterStats
		if err := rows.Scan(&s.ID, &s.Name, &s.EvalTimeoutCount, &s.LastTimeoutAt); err != nil {
			return nil, repo.Err(op, err)
		}
		out = append(out, &s)
	}
	if err := rows.Err(); err != nil {
		return nil, repo.Err(op, err)
	}
	return out, nil
}

// GetFilter mengambil satu baris content_filters berdasarkan id.
func (r *Repo) GetFilter(ctx context.Context, id string) (*ContentFilter, error) {
	const op = "mengambil penyaring konten"
	if !idOK(id) {
		return nil, fmt.Errorf("%s: %w", op, repo.ErrNotFound)
	}

	row := r.q.QueryRow(ctx, `select `+filterColumns+` from content_filters where id = $1`, id)
	f, err := scanFilter(row)
	if err != nil {
		return nil, repo.Err(op, err)
	}
	return f, nil
}

// UpdateFilterParams berisi opsi pembaruan penyaring konten.
type UpdateFilterParams struct {
	Name          *string
	Description   *string
	Priority      *int
	AppliesTo     *string
	Action        *string
	Pattern       *string
	PatternType   *string
	CaseSensitive *bool
	MaxEvalMS     *int
	Enabled       *bool
}

// UpdateFilter memperbarui konfigurasi penyaring konten.
func (r *Repo) UpdateFilter(ctx context.Context, id string, p UpdateFilterParams) (*ContentFilter, error) {
	const op = "memperbarui penyaring konten"
	if !idOK(id) {
		return nil, fmt.Errorf("%s: %w", op, repo.ErrNotFound)
	}

	var (
		setClauses []string
		args       []any
	)
	args = append(args, id)

	if p.Name != nil {
		args = append(args, *p.Name)
		setClauses = append(setClauses, fmt.Sprintf("name = $%d", len(args)))
	}
	if p.Description != nil {
		args = append(args, *p.Description)
		setClauses = append(setClauses, fmt.Sprintf("description = $%d", len(args)))
	}
	if p.Priority != nil {
		args = append(args, *p.Priority)
		setClauses = append(setClauses, fmt.Sprintf("priority = $%d", len(args)))
	}
	if p.AppliesTo != nil {
		args = append(args, *p.AppliesTo)
		setClauses = append(setClauses, fmt.Sprintf("applies_to = $%d", len(args)))
	}
	if p.Action != nil {
		args = append(args, *p.Action)
		setClauses = append(setClauses, fmt.Sprintf("action = $%d", len(args)))
	}
	if p.Pattern != nil {
		args = append(args, *p.Pattern)
		setClauses = append(setClauses, fmt.Sprintf("pattern = $%d", len(args)))
	}
	if p.PatternType != nil {
		args = append(args, *p.PatternType)
		setClauses = append(setClauses, fmt.Sprintf("pattern_type = $%d", len(args)))
	}
	if p.CaseSensitive != nil {
		args = append(args, *p.CaseSensitive)
		setClauses = append(setClauses, fmt.Sprintf("case_sensitive = $%d", len(args)))
	}
	if p.MaxEvalMS != nil {
		args = append(args, *p.MaxEvalMS)
		setClauses = append(setClauses, fmt.Sprintf("max_eval_ms = $%d", len(args)))
	}
	if p.Enabled != nil {
		args = append(args, *p.Enabled)
		setClauses = append(setClauses, fmt.Sprintf("enabled = $%d", len(args)))
	}

	if len(setClauses) == 0 {
		return r.GetFilter(ctx, id)
	}

	setClauses = append(setClauses, "updated_at = now()")
	query := fmt.Sprintf(`update content_filters set %s where id = $1 returning %s`,
		strings.Join(setClauses, ", "), filterColumns)

	row := r.q.QueryRow(ctx, query, args...)
	f, err := scanFilter(row)
	if err != nil {
		return nil, repo.Err(op, err)
	}
	return f, nil
}

// ListFilters mengambil seluruh penyaring konten dengan keyset pagination.
func (r *Repo) ListFilters(ctx context.Context, page repo.Page) ([]*ContentFilter, string, error) {
	const op = "mendaftar penyaring konten"
	limit := page.Normalize()

	var (
		query strings.Builder
		args  []any
	)
	query.WriteString(`select ` + filterColumns + ` from content_filters `)

	if page.Cursor != "" {
		ts, id, err := decodeCursor(op, page.Cursor)
		if err != nil {
			return nil, "", err
		}
		args = append(args, ts, id)
		query.WriteString(fmt.Sprintf(`where (created_at, id) < ($1, $2) `))
	}

	args = append(args, limit+1)
	query.WriteString(fmt.Sprintf(`order by created_at desc, id desc limit $%d`, len(args)))

	rows, err := r.q.Query(ctx, query.String(), args...)
	if err != nil {
		return nil, "", repo.Err(op, err)
	}
	defer rows.Close()

	var items []*ContentFilter
	for rows.Next() {
		f, err := scanFilter(rows)
		if err != nil {
			return nil, "", repo.Err(op, err)
		}
		items = append(items, f)
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
