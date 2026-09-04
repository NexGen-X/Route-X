package traffic

import (
	"context"
	"encoding/base64"
	"encoding/json"
	"fmt"
	"strconv"
	"strings"
	"time"

	"github.com/NexGen-X/Route-X/internal/database/repo"
)

// Kolom pengurutan yang diizinkan.
//
// Daftar putih, bukan nama kolom dari pemanggil: nama kolom masuk ke SQL sebagai
// identifier dan tidak bisa diparameterkan, jadi hanya nilai yang ada di sini yang
// boleh sampai ke query.
//
// Semua kolom di sini NOT NULL. Kolom nullable seperti ttft_ms sengaja tidak bisa
// diurutkan: paginasi keyset memakai perbandingan baris `(a, b, c) < ($1, $2, $3)`,
// dan perbandingan itu tidak punya arti yang konsisten begitu salah satu sisinya NULL —
// hasilnya halaman yang melewatkan atau menggandakan baris.
const (
	SortTime    = "time"
	SortLatency = "latency"
	SortTokens  = "tokens"
	SortCost    = "cost"
)

var sortColumns = map[string]string{
	SortTime:    "created_at",
	SortLatency: "latency_ms",
	SortTokens:  "total_tokens",
	SortCost:    "cost_usd",
}

// Filter menyaring log request. Sesuai kebutuhan halaman Requests: pencarian, filter
// provider, model, status, API key, dan rentang waktu.
type Filter struct {
	// From dan To membatasi rentang waktu. Nol berarti DefaultWindow terakhir; batas
	// ini selalu ada agar partition pruning bekerja.
	From time.Time
	To   time.Time

	ProviderID string
	ModelID    string
	APIKeyID   string

	// Status menerima kode persis (mis. "429") atau kelas ("2xx", "4xx", "5xx").
	Status string
	// OnlyErrors membatasi ke request yang GAGAL, dan gagal di sini berarti status >= 400
	// ATAU error_type terisi.
	//
	// Bagian kedua bukan kelengkapan berlebihan: aliran yang terputus setelah sebagian
	// terkirim sampai ke klien sebagai 200, karena pada titik itu status sudah terkunci dan
	// klien memang menerima jawaban sebagian. Menyaring dengan status saja membuat seluruh
	// kelas kegagalan streaming tidak pernah muncul di chip "Errors" — kegagalan yang justru
	// paling ingin dilihat operator. Harganya: indeks partial requests_errors_idx tidak lagi
	// melayani syarat ini seluruhnya, dan sisanya disaring di atas indeks rentang waktu.
	// Definisi yang sama dipakai agregasi di stats.go dan rollup.go.
	OnlyErrors bool

	// Search mencocokkan request_id, nama model yang diminta, atau nama provider.
	Search string

	// Sort adalah salah satu konstanta Sort*; kosong berarti SortTime.
	Sort string
	// Ascending membalik arah pengurutan. Bawaannya menurun (terbaru dulu).
	Ascending bool
}

// Result adalah satu halaman hasil.
type Result struct {
	Rows       []Row
	NextCursor string
}

// cursor menandai posisi paginasi keyset.
//
// Nilai kolom pengurutan saja tidak cukup sebagai penanda: pada beban tinggi banyak
// request punya created_at, latensi, atau biaya yang sama persis, dan penanda yang
// tidak unik akan melewatkan atau menggandakan baris di batas halaman. Karena itu
// penanda selalu membawa created_at dan id sebagai pemecah seri, sehingga
// urutannya total.
type cursor struct {
	Sort      string    `json:"s"`
	Value     string    `json:"v"`
	CreatedAt time.Time `json:"t"`
	ID        string    `json:"i"`
}

func (c cursor) encode() string {
	raw, err := json.Marshal(c)
	if err != nil {
		return ""
	}
	return base64.RawURLEncoding.EncodeToString(raw)
}

func decodeCursor(s string) (cursor, error) {
	raw, err := base64.RawURLEncoding.DecodeString(s)
	if err != nil {
		return cursor{}, ErrInvalidCursor
	}
	var c cursor
	if err := json.Unmarshal(raw, &c); err != nil {
		return cursor{}, ErrInvalidCursor
	}
	if c.ID == "" || c.CreatedAt.IsZero() {
		return cursor{}, ErrInvalidCursor
	}
	return c, nil
}

// rowColumns adalah kolom yang dibaca List, disatukan agar bentuk barisnya konsisten.
const rowColumns = `
	id::text, created_at, request_id,
	coalesce(api_key_id::text, ''), coalesce(api_key_name, ''),
	method, endpoint, requested_model,
	coalesce(model_name, ''), coalesce(provider_name, ''), coalesce(upstream_model, ''),
	status_code, coalesce(error_type, ''), coalesce(error_code, ''), stream,
	input_tokens, output_tokens, total_tokens,
	(cost_usd * 100000000)::bigint,
	latency_ms, ttft_ms, retry_count, failover_count`

func scanRow(s interface{ Scan(...any) error }) (Row, error) {
	var r Row
	err := s.Scan(
		&r.ID, &r.CreatedAt, &r.RequestID,
		&r.APIKeyID, &r.APIKeyName,
		&r.Method, &r.Endpoint, &r.RequestedModel,
		&r.ModelName, &r.ProviderName, &r.UpstreamModel,
		&r.StatusCode, &r.ErrorType, &r.ErrorCode, &r.Stream,
		&r.InputTokens, &r.OutputTokens, &r.TotalTokens, &r.CostUSD,
		&r.LatencyMS, &r.TTFTMS, &r.RetryCount, &r.FailoverCount,
	)
	return r, err
}

// List mengembalikan satu halaman log request.
func (r *Repo) List(ctx context.Context, f Filter, p repo.Page) (Result, error) {
	const op = "mendaftar log request"

	sortKey := f.Sort
	if sortKey == "" {
		sortKey = SortTime
	}
	sortCol, ok := sortColumns[sortKey]
	if !ok {
		return Result{}, fmt.Errorf("%s: %w: kolom pengurutan %q tidak dikenal", op, repo.ErrConstraint, f.Sort)
	}

	from, to := f.From, f.To
	if to.IsZero() {
		to = time.Now()
	}
	if from.IsZero() {
		from = to.Add(-DefaultWindow)
	}

	var (
		conds = []string{"created_at >= $1", "created_at < $2"}
		args  = []any{from, to}
	)
	add := func(format string, arg any) {
		args = append(args, arg)
		conds = append(conds, fmt.Sprintf(format, len(args)))
	}

	if f.ProviderID != "" {
		add("provider_id = $%d", f.ProviderID)
	}
	if f.ModelID != "" {
		add("model_id = $%d", f.ModelID)
	}
	if f.APIKeyID != "" {
		add("api_key_id = $%d", f.APIKeyID)
	}
	if f.OnlyErrors {
		conds = append(conds, "(status_code >= 400 or error_type is not null)")
	}
	if f.Status != "" {
		switch strings.ToLower(f.Status) {
		case "2xx", "3xx", "4xx", "5xx":
			base, _ := strconv.Atoi(f.Status[:1])
			args = append(args, base*100, base*100+100)
			conds = append(conds, fmt.Sprintf("status_code >= $%d and status_code < $%d", len(args)-1, len(args)))
		default:
			code, err := strconv.Atoi(f.Status)
			if err != nil {
				return Result{}, fmt.Errorf("%s: status %q bukan kode atau kelas yang sah", op, f.Status)
			}
			add("status_code = $%d", code)
		}
	}
	if f.Search != "" {
		// Dicocokkan ke tiga kolom yang semuanya bukan rahasia. Satu argumen dipakai
		// tiga kali, jadi kondisinya dirakit langsung.
		args = append(args, f.Search)
		n := len(args)
		conds = append(conds, fmt.Sprintf(
			"(request_id = $%d or requested_model ilike '%%' || $%d || '%%' or provider_name ilike '%%' || $%d || '%%')",
			n, n, n))
	}

	dir, cmp := "desc", "<"
	if f.Ascending {
		dir, cmp = "asc", ">"
	}

	if p.Cursor != "" {
		c, err := decodeCursor(p.Cursor)
		if err != nil {
			return Result{}, fmt.Errorf("%s: %w", op, err)
		}
		if c.Sort != sortKey {
			// Mengganti kolom pengurutan membuat penanda lama tidak bermakna; lebih baik
			// ditolak daripada menghasilkan halaman yang melewatkan baris.
			return Result{}, fmt.Errorf("%s: %w (kolom pengurutan berubah)", op, ErrInvalidCursor)
		}
		// Perbandingan baris memakai urutan total (kolom sort, created_at, id).
		args = append(args, c.Value, c.CreatedAt, c.ID)
		n := len(args)
		conds = append(conds, fmt.Sprintf("(%s, created_at, id) %s ($%d::%s, $%d, $%d::uuid)",
			sortCol, cmp, n-2, sortColumnType(sortCol), n-1, n))
	}

	limit := p.Normalize()
	args = append(args, limit+1)

	query := `select ` + rowColumns + ` from requests where ` + strings.Join(conds, " and ") +
		fmt.Sprintf(" order by %s %s, created_at %s, id %s limit $%d", sortCol, dir, dir, dir, len(args))

	rows, err := r.q.Query(ctx, query, args...)
	if err != nil {
		return Result{}, repo.Err(op, err)
	}
	defer rows.Close()

	out := make([]Row, 0, limit)
	for rows.Next() {
		row, err := scanRow(rows)
		if err != nil {
			return Result{}, repo.Err(op, err)
		}
		out = append(out, row)
	}
	if err := rows.Err(); err != nil {
		return Result{}, repo.Err(op, err)
	}

	res := Result{Rows: out}
	if len(out) > limit {
		last := out[limit-1]
		res.Rows = out[:limit]
		res.NextCursor = cursor{
			Sort:      sortKey,
			Value:     sortValueOf(last, sortKey),
			CreatedAt: last.CreatedAt,
			ID:        last.ID,
		}.encode()
	}
	return res, nil
}

// sortColumnType memberi tipe SQL untuk cast nilai kursor, supaya perbandingan baris
// memakai tipe yang sama dengan kolomnya dan tetap bisa memakai indeks.
func sortColumnType(col string) string {
	switch col {
	case "created_at":
		return "timestamptz"
	case "cost_usd":
		return "numeric"
	default:
		return "int"
	}
}

// sortValueOf mengambil nilai kolom pengurutan dari sebuah baris sebagai teks.
func sortValueOf(r Row, sortKey string) string {
	switch sortKey {
	case SortLatency:
		return strconv.Itoa(r.LatencyMS)
	case SortTokens:
		return strconv.Itoa(r.TotalTokens)
	case SortCost:
		// Dikembalikan ke bentuk desimal agar cast ke numeric di query tepat.
		return fmt.Sprintf("%d.%08d", r.CostUSD/100000000, r.CostUSD%100000000)
	default:
		return r.CreatedAt.Format(time.RFC3339Nano)
	}
}

// Get mengambil satu request lengkap untuk inspector.
func (r *Repo) Get(ctx context.Context, id string, createdAt time.Time) (Detail, error) {
	const op = "mengambil detail request"

	// created_at disertakan sebagai syarat agar perencana bisa memangkas partisi;
	// tanpa itu pencarian satu baris harus menyentuh indeks setiap partisi.
	conds := "id = $1"
	args := []any{id}
	if !createdAt.IsZero() {
		conds += " and created_at = $2"
		args = append(args, createdAt)
	}

	var d Detail
	err := r.q.QueryRow(ctx, `select `+rowColumns+`,
			coalesce(user_id::text, ''), coalesce(model_id::text, ''), coalesce(provider_id::text, ''),
			coalesce(error_message, ''), cached_input_tokens, reasoning_tokens, upstream_latency_ms,
			coalesce(routing_strategy, ''), routing_decision,
			coalesce(host(client_ip), ''), coalesce(user_agent, '')
		from requests where `+conds, args...).Scan(
		&d.ID, &d.CreatedAt, &d.RequestID,
		&d.APIKeyID, &d.APIKeyName,
		&d.Method, &d.Endpoint, &d.RequestedModel,
		&d.ModelName, &d.ProviderName, &d.UpstreamModel,
		&d.StatusCode, &d.ErrorType, &d.ErrorCode, &d.Stream,
		&d.InputTokens, &d.OutputTokens, &d.TotalTokens, &d.CostUSD,
		&d.LatencyMS, &d.TTFTMS, &d.RetryCount, &d.FailoverCount,
		&d.UserID, &d.ModelID, &d.ProviderID,
		&d.ErrorMessage, &d.CachedInputTokens, &d.ReasoningTokens, &d.UpstreamLatencyMS,
		&d.RoutingStrategy, &d.RoutingDecision,
		&d.ClientIP, &d.UserAgent,
	)
	if err != nil {
		return Detail{}, repo.Err(op, err)
	}
	return d, nil
}

// Events mengembalikan timeline satu request, berurut.
func (r *Repo) Events(ctx context.Context, requestPK string, createdAt time.Time) ([]Event, error) {
	const op = "mengambil timeline request"

	conds := "request_pk = $1"
	args := []any{requestPK}
	if !createdAt.IsZero() {
		conds += " and created_at >= $2 and created_at < $2 + interval '1 day'"
		args = append(args, createdAt)
	}

	rows, err := r.q.Query(ctx, `
		select seq, kind, provider_id::text, provider_name, upstream_model,
		       status_code, latency_ms, error_kind, error_message, detail, created_at
		from request_events where `+conds+` order by seq`, args...)
	if err != nil {
		return nil, repo.Err(op, err)
	}
	defer rows.Close()

	var out []Event
	for rows.Next() {
		var e Event
		if err := rows.Scan(&e.Seq, &e.Kind, &e.ProviderID, &e.ProviderName, &e.UpstreamModel,
			&e.StatusCode, &e.LatencyMS, &e.ErrorKind, &e.ErrorMessage, &e.Detail, &e.CreatedAt); err != nil {
			return nil, repo.Err(op, err)
		}
		out = append(out, e)
	}
	return out, repo.Err(op, rows.Err())
}

// Payload mengambil body request dan respons satu request.
func (r *Repo) Payload(ctx context.Context, requestPK string, createdAt time.Time) (Payload, error) {
	const op = "mengambil payload request"

	var p Payload
	err := r.q.QueryRow(ctx, `
		select request_headers, request_body, response_headers, response_body, truncated, size_bytes
		from request_payloads where request_pk = $1 and created_at = $2`,
		requestPK, createdAt).Scan(
		&p.RequestHeaders, &p.RequestBody, &p.ResponseHeaders, &p.ResponseBody,
		&p.Truncated, &p.SizeBytes)
	if err != nil {
		return Payload{}, repo.Err(op, err)
	}
	return p, nil
}
