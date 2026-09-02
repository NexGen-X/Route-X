// Package traffic memuat akses data log request: penulisan hasil request, timeline
// percobaan, payload, dan query yang melayani halaman Requests beserta inspector-nya.
//
// Tabel yang dilayani dipartisi bulanan, jadi setiap query yang bisa membawa batas
// waktu WAJIB membawanya — tanpa itu perencana harus menyentuh semua partisi dan
// keuntungan partisi hilang. Filter rentang waktu karena itu punya nilai bawaan, bukan
// opsional.
package traffic

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"net/netip"
	"strings"
	"time"

	"github.com/NexGen-X/Route-X/internal/database/repo"
)

// DefaultWindow adalah rentang waktu bawaan bila pemanggil tidak menentukan.
//
// Ada nilai bawaan justru supaya tidak ada query tanpa batas waktu: pada tabel
// terpartisi, query tanpa batas menyentuh seluruh partisi.
const DefaultWindow = 24 * time.Hour

// Batas ukuran payload yang disimpan. Body yang lebih besar dipotong dan ditandai,
// karena menyimpan payload raksasa membuat tabel membengkak tanpa menambah nilai
// diagnostik yang berarti.
const MaxPayloadBytes = 256 * 1024

// ErrInvalidCursor dikembalikan saat kursor paginasi tidak bisa dibaca.
var ErrInvalidCursor = errors.New("kursor paginasi tidak sah")

// Record adalah satu baris log request yang akan ditulis setelah request selesai.
//
// Field bertipe pointer berarti "tidak diketahui": request yang ditolak sebelum
// resolusi model tidak punya provider, dan request non-streaming tidak punya TTFT.
type Record struct {
	RequestID string

	APIKeyID   *string
	APIKeyName *string
	UserID     *string

	Method         string
	Endpoint       string
	RequestedModel string

	ModelID       *string
	ModelName     *string
	ProviderID    *string
	ProviderName  *string
	UpstreamModel *string

	StatusCode   int
	ErrorType    *string
	ErrorCode    *string
	ErrorMessage *string

	Stream bool

	InputTokens       int
	OutputTokens      int
	CachedInputTokens int
	ReasoningTokens   int
	TotalTokens       int

	// CostUSD dalam satuan 10^-8 USD, sejalan dengan skala kolom numeric(16,8).
	// Bilangan bulat dipakai, bukan float64, agar penjumlahan biaya tetap eksak.
	CostUSD int64

	LatencyMS         int
	TTFTMS            *int
	UpstreamLatencyMS *int

	RetryCount    int
	FailoverCount int

	RoutingStrategy *string
	RoutingDecision json.RawMessage

	ClientIP  *netip.Addr
	UserAgent *string
}

// Row adalah satu baris pada tabel log request.
type Row struct {
	ID        string
	CreatedAt time.Time
	RequestID string

	APIKeyID   string
	APIKeyName string

	Method         string
	Endpoint       string
	RequestedModel string
	ModelName      string
	ProviderName   string
	UpstreamModel  string

	StatusCode int
	ErrorType  string
	ErrorCode  string
	Stream     bool

	InputTokens  int
	OutputTokens int
	TotalTokens  int
	CostUSD      int64

	LatencyMS int
	TTFTMS    *int

	RetryCount    int
	FailoverCount int
}

// Detail adalah isi request inspector.
type Detail struct {
	Row

	UserID            string
	ModelID           string
	ProviderID        string
	ErrorMessage      string
	CachedInputTokens int
	ReasoningTokens   int
	UpstreamLatencyMS *int
	RoutingStrategy   string
	RoutingDecision   json.RawMessage
	ClientIP          string
	UserAgent         string
}

// Event adalah satu langkah pada timeline percobaan sebuah request.
type Event struct {
	Seq           int
	Kind          string
	ProviderID    *string
	ProviderName  *string
	UpstreamModel *string
	StatusCode    *int
	LatencyMS     *int
	ErrorKind     *string
	ErrorMessage  *string
	Detail        json.RawMessage
	CreatedAt     time.Time
}

// Jenis event, harus sama dengan constraint request_events_kind_valid di migrasi 0006.
const (
	EventAttemptStarted   = "attempt_started"
	EventAttemptSucceeded = "attempt_succeeded"
	EventAttemptFailed    = "attempt_failed"
	EventRetryScheduled   = "retry_scheduled"
	EventFailover         = "failover"
	EventCircuitOpen      = "circuit_open"
	EventRateLimited      = "rate_limited"
	EventBudgetExceeded   = "budget_exceeded"
	EventContentBlocked   = "content_blocked"
	EventStreamStarted    = "stream_started"
	EventStreamCompleted  = "stream_completed"
)

// Payload adalah body request dan respons untuk satu request.
//
// Header di sini WAJIB sudah disaring oleh pemanggil: Authorization, X-Api-Key, dan
// Cookie tidak boleh pernah mencapai tabel ini.
type Payload struct {
	RequestHeaders  json.RawMessage
	RequestBody     json.RawMessage
	ResponseHeaders json.RawMessage
	ResponseBody    json.RawMessage
	Truncated       bool
	SizeBytes       int
}

// Repo adalah akses data log request.
type Repo struct{ q repo.Querier }

// New membuat Repo.
func New(q repo.Querier) *Repo { return &Repo{q: q} }

// Insert menulis satu baris log dan mengembalikan kunci barisnya.
//
// created_at diserahkan ke default kolom (now()) agar baris selalu mendarat di partisi
// yang benar, dan dikembalikan bersama ID karena keduanya membentuk kunci utama —
// event dan payload memerlukan keduanya untuk ditulis ke partisi yang sama.
func (r *Repo) Insert(ctx context.Context, rec Record) (id string, createdAt time.Time, err error) {
	const op = "menulis log request"

	var ip any
	if rec.ClientIP != nil && rec.ClientIP.IsValid() {
		ip = rec.ClientIP.String()
	}
	decision := rec.RoutingDecision
	if len(decision) == 0 {
		decision = json.RawMessage("{}")
	}

	err = r.q.QueryRow(ctx, `
		insert into requests (
			request_id, api_key_id, api_key_name, user_id,
			method, endpoint, requested_model,
			model_id, model_name, provider_id, provider_name, upstream_model,
			status_code, error_type, error_code, error_message, stream,
			input_tokens, output_tokens, cached_input_tokens, reasoning_tokens, total_tokens,
			cost_usd, latency_ms, ttft_ms, upstream_latency_ms,
			retry_count, failover_count, routing_strategy, routing_decision,
			client_ip, user_agent
		) values (
			$1, $2, $3, $4, $5, $6, $7, $8, $9, $10, $11, $12, $13, $14, $15, $16, $17,
			$18, $19, $20, $21, $22,
			-- Biaya dikirim sebagai bilangan bulat satuan 10^-8 lalu diskalakan di SQL,
			-- sehingga tidak ada tahap floating point di jalur mana pun.
			($23::bigint / 100000000.0)::numeric(16,8),
			$24, $25, $26, $27, $28, $29, $30, $31::inet, $32
		)
		returning id::text, created_at`,
		rec.RequestID, rec.APIKeyID, rec.APIKeyName, rec.UserID,
		rec.Method, rec.Endpoint, rec.RequestedModel,
		rec.ModelID, rec.ModelName, rec.ProviderID, rec.ProviderName, rec.UpstreamModel,
		rec.StatusCode, rec.ErrorType, rec.ErrorCode, rec.ErrorMessage, rec.Stream,
		rec.InputTokens, rec.OutputTokens, rec.CachedInputTokens, rec.ReasoningTokens, rec.TotalTokens,
		rec.CostUSD, rec.LatencyMS, rec.TTFTMS, rec.UpstreamLatencyMS,
		rec.RetryCount, rec.FailoverCount, rec.RoutingStrategy, decision,
		ip, rec.UserAgent,
	).Scan(&id, &createdAt)
	if err != nil {
		return "", time.Time{}, repo.Err(op, err)
	}
	return id, createdAt, nil
}

// AppendEvents menulis seluruh timeline satu request dalam satu pernyataan.
//
// Satu perjalanan ke database untuk semua event, bukan satu per event: timeline ditulis
// pada setiap request, dan pada jalur sepanas itu jumlah perjalanan ke database jauh
// lebih menentukan daripada ukuran pernyataannya.
func (r *Repo) AppendEvents(ctx context.Context, requestPK string, createdAt time.Time, events []Event) error {
	if len(events) == 0 {
		return nil
	}
	const op = "menulis timeline request"

	var (
		values []string
		args   []any
	)
	args = append(args, requestPK, createdAt)
	for _, e := range events {
		detail := e.Detail
		if len(detail) == 0 {
			detail = json.RawMessage("{}")
		}
		base := len(args)
		args = append(args, e.Seq, e.Kind, e.ProviderID, e.ProviderName, e.UpstreamModel,
			e.StatusCode, e.LatencyMS, e.ErrorKind, e.ErrorMessage, detail)
		values = append(values, fmt.Sprintf(
			"($1, $2, $%d, $%d, $%d, $%d, $%d, $%d, $%d, $%d, $%d, $%d)",
			base+1, base+2, base+3, base+4, base+5, base+6, base+7, base+8, base+9, base+10))
	}

	_, err := r.q.Exec(ctx, `
		insert into request_events (
			request_pk, created_at, seq, kind, provider_id, provider_name, upstream_model,
			status_code, latency_ms, error_kind, error_message, detail
		) values `+strings.Join(values, ", "), args...)
	return repo.Err(op, err)
}

// SavePayload menyimpan body request dan respons.
//
// createdAt harus sama dengan milik baris requests-nya, agar payload mendarat di
// partisi yang sama dan bisa dihapus bersama saat retensi berakhir.
func (r *Repo) SavePayload(ctx context.Context, requestPK string, createdAt time.Time, p Payload) error {
	_, err := r.q.Exec(ctx, `
		insert into request_payloads (
			request_pk, created_at, request_headers, request_body,
			response_headers, response_body, truncated, size_bytes
		) values ($1, $2, $3, $4, $5, $6, $7, $8)
		on conflict (request_pk, created_at) do update set
			request_headers = excluded.request_headers,
			request_body    = excluded.request_body,
			response_headers = excluded.response_headers,
			response_body   = excluded.response_body,
			truncated       = excluded.truncated,
			size_bytes      = excluded.size_bytes`,
		requestPK, createdAt, p.RequestHeaders, p.RequestBody,
		p.ResponseHeaders, p.ResponseBody, p.Truncated, p.SizeBytes)
	return repo.Err("menyimpan payload request", err)
}
