package webhooks

import (
	"context"
	"crypto/rand"
	"encoding/json"
	"errors"
	"fmt"
	"strings"
	"time"

	"github.com/jackc/pgx/v5"

	"github.com/NexGen-X/Route-X/internal/database/repo"
	"github.com/NexGen-X/Route-X/internal/security"
)

// Daftar status pengiriman webhook.
const (
	StatusPending    = "pending"
	StatusDelivering = "delivering"
	StatusDelivered  = "delivered"
	StatusFailed     = "failed"
	StatusAbandoned  = "abandoned"
)

// Webhook merepresentasikan satu konfigurasi endpoint webhook di database.
type Webhook struct {
	ID                  string
	Name                string
	URL                 string
	Events              []string
	SecretCiphertext    string
	EncryptionKeyID     string
	MaskedHint          string
	Enabled             bool
	MaxRetries          int
	TimeoutMS           int
	LastDeliveryAt      *time.Time
	LastDeliveryStatus  *string
	ConsecutiveFailures int
	CreatedAt           time.Time
	UpdatedAt           time.Time
	CreatedBy           *string
}

// Delivery merepresentasikan satu rekaman antrean dan riwayat pengiriman webhook.
type Delivery struct {
	ID                 int64
	WebhookID          string
	Event              string
	Payload            json.RawMessage
	Status             string
	AttemptCount       int
	NextAttemptAt      time.Time
	LockedAt           *time.Time
	LockedBy           *string
	ResponseStatusCode *int
	ErrorMessage       *string
	CreatedAt          time.Time
	DeliveredAt        *time.Time
}

// Kolom untuk scan tabel webhooks.
const webhookColumns = `id::text, name, url, events,
	secret_ciphertext, encryption_key_id, masked_hint, enabled,
	max_retries, timeout_ms, last_delivery_at, last_delivery_status,
	consecutive_failures, created_at, updated_at, created_by::text`

// scanWebhook membaca baris tunggal webhooks ke struct Webhook.
func scanWebhook(s interface{ Scan(...any) error }) (*Webhook, error) {
	var (
		w         Webhook
		createdBy *string
	)
	err := s.Scan(
		&w.ID, &w.Name, &w.URL, &w.Events,
		&w.SecretCiphertext, &w.EncryptionKeyID, &w.MaskedHint, &w.Enabled,
		&w.MaxRetries, &w.TimeoutMS, &w.LastDeliveryAt, &w.LastDeliveryStatus,
		&w.ConsecutiveFailures, &w.CreatedAt, &w.UpdatedAt, &createdBy,
	)
	if err != nil {
		return nil, err
	}
	w.CreatedBy = createdBy
	return &w, nil
}

// Kolom untuk scan tabel webhook_deliveries.
const deliveryColumns = `id, webhook_id::text, event, payload, status, attempt_count,
	next_attempt_at, locked_at, locked_by, response_status_code, error_message,
	created_at, delivered_at`

const deliveryPrefixedColumns = `d.id, d.webhook_id::text, d.event, d.payload, d.status, d.attempt_count,
	d.next_attempt_at, d.locked_at, d.locked_by, d.response_status_code, d.error_message,
	d.created_at, d.delivered_at`

// scanDelivery membaca baris tunggal webhook_deliveries ke struct Delivery.

func scanDelivery(s interface{ Scan(...any) error }) (*Delivery, error) {
	var d Delivery
	err := s.Scan(
		&d.ID, &d.WebhookID, &d.Event, &d.Payload, &d.Status, &d.AttemptCount,
		&d.NextAttemptAt, &d.LockedAt, &d.LockedBy, &d.ResponseStatusCode, &d.ErrorMessage,
		&d.CreatedAt, &d.DeliveredAt,
	)
	if err != nil {
		return nil, err
	}
	return &d, nil
}

// EnqueueSink adalah antarmuka untuk memasukkan event ke antrean webhook.
type EnqueueSink interface {
	Enqueue(ctx context.Context, event string, payload any) (int, error)
}

// Repo menyediakan operasi basis data untuk webhook dan antrean pengirimannya.

type Repo struct {
	q repo.Querier
}

// NewRepo membuat instance baru Repo webhook.
func NewRepo(q repo.Querier) *Repo {
	return &Repo{q: q}
}

// CreateWebhookParams memuat parameter pembuatan webhook baru.
type CreateWebhookParams struct {
	ID         string // Opsional, bila kosong dihasilkan PostgreSQL
	Name       string
	URL        string
	Events     []string
	RawSecret  string // Plaintext rahasia yang akan dienkripsi
	Cipher     *security.Cipher
	MaxRetries int
	TimeoutMS  int
	CreatedBy  string
	SSRFPolicy security.SSRFPolicy
}

// Create menyimpan webhook baru ke database dengan secret terenkripsi AES-256-GCM.
func (r *Repo) Create(ctx context.Context, p CreateWebhookParams) (*Webhook, error) {
	const op = "membuat webhook"
	if p.Cipher == nil {
		return nil, fmt.Errorf("%s: cipher enkripsi wajib ada", op)
	}
	if strings.TrimSpace(p.Name) == "" {
		return nil, fmt.Errorf("%s: nama webhook tidak boleh kosong", op)
	}
	if strings.TrimSpace(p.URL) == "" {
		return nil, fmt.Errorf("%s: URL webhook tidak boleh kosong", op)
	}
	if err := security.ValidateBaseURL(p.URL, p.SSRFPolicy); err != nil {
		return nil, fmt.Errorf("%s: URL webhook ditolak kebijakan keamanan: %w", op, err)
	}
	if len(p.Events) == 0 {
		return nil, fmt.Errorf("%s: minimal satu event harus dilanggani", op)
	}
	if p.TimeoutMS <= 0 {
		p.TimeoutMS = 10000
	}
	if p.MaxRetries < 0 {
		p.MaxRetries = 5
	}

	// Buat ID baru jika belum disediakan agar AAD bisa langsung diikat ke ID ini.
	id := p.ID
	if id == "" {
		generated, err := newID()
		if err != nil {
			return nil, fmt.Errorf("%s: %w", op, err)
		}
		id = generated
	}

	// AAD wajib terikat ke ID webhook sesuai aturan keamanan fase 10.
	aad := security.WebhookAAD(id)
	ciphertext, err := p.Cipher.Encrypt([]byte(p.RawSecret), aad)
	if err != nil {
		return nil, fmt.Errorf("%s: mengenkripsi secret: %w", op, err)
	}
	hint := MaskSecret(p.RawSecret)

	var createdBy any
	if p.CreatedBy != "" {
		createdBy = p.CreatedBy
	}

	row := r.q.QueryRow(ctx, `
		insert into webhooks (
			id, name, url, events, secret_ciphertext,
			encryption_key_id, masked_hint, max_retries, timeout_ms, created_by
		) values (
			$1::uuid, $2, $3, $4, $5,
			$6, $7, $8, $9, $10::uuid
		)
		returning `+webhookColumns,
		id, p.Name, p.URL, p.Events, ciphertext,
		p.Cipher.KeyID(), hint, p.MaxRetries, p.TimeoutMS, createdBy,
	)

	w, err := scanWebhook(row)
	if err != nil {
		return nil, repo.Err(op, err)
	}
	return w, nil
}

// Get mengambil webhook berdasarkan ID.
func (r *Repo) Get(ctx context.Context, id string) (*Webhook, error) {
	const op = "mengambil webhook"
	row := r.q.QueryRow(ctx, `select `+webhookColumns+` from webhooks where id = $1::uuid`, id)
	w, err := scanWebhook(row)
	if err != nil {
		return nil, repo.Err(op, err)
	}
	return w, nil
}

// List mengambil seluruh webhook terdaftar.
func (r *Repo) List(ctx context.Context) ([]*Webhook, error) {
	const op = "mendaftar seluruh webhook"
	rows, err := r.q.Query(ctx, `select `+webhookColumns+` from webhooks order by created_at desc`)
	if err != nil {
		return nil, repo.Err(op, err)
	}
	defer rows.Close()

	var out []*Webhook
	for rows.Next() {
		w, err := scanWebhook(rows)
		if err != nil {
			return nil, repo.Err(op, err)
		}
		out = append(out, w)
	}
	if err := rows.Err(); err != nil {
		return nil, repo.Err(op, err)
	}
	return out, nil
}

// Enqueue mencari semua webhook aktif yang berlangganan event ini dan memasukkannya ke antrean.
//
// Nilai sensitif di payload harus sudah disaring sebelum memanggil fungsi ini.
func (r *Repo) Enqueue(ctx context.Context, event string, payload any) (int, error) {
	const op = "memasukkan event ke antrean webhook"

	payloadBytes, err := json.Marshal(payload)
	if err != nil {
		return 0, fmt.Errorf("%s: marshal payload: %w", op, err)
	}

	// Temukan webhook aktif yang berlangganan event ini atau langganan wildcard '*'.
	// Indeks GIN webhooks_events_idx melayani pencarian array ini.
	rows, err := r.q.Query(ctx, `
		select id
		from webhooks
		where enabled and (events @> array[$1]::text[] or events @> array['*']::text[])`,
		event,
	)
	if err != nil {
		return 0, repo.Err(op, err)
	}
	defer rows.Close()

	var webhookIDs []string
	for rows.Next() {
		var id string
		if err := rows.Scan(&id); err != nil {
			return 0, repo.Err(op, err)
		}
		webhookIDs = append(webhookIDs, id)
	}
	if err := rows.Err(); err != nil {
		return 0, repo.Err(op, err)
	}

	if len(webhookIDs) == 0 {
		return 0, nil
	}

	// Masukkan setiap target pengiriman ke antrean webhook_deliveries.
	inserted := 0
	for _, wid := range webhookIDs {
		_, err := r.q.Exec(ctx, `
			insert into webhook_deliveries (webhook_id, event, payload, status, next_attempt_at)
			values ($1::uuid, $2, $3, 'pending', now())`,
			wid, event, payloadBytes,
		)
		if err != nil {
			return inserted, repo.Err(op, err)
		}
		inserted++
	}

	return inserted, nil
}

// ClaimDue mengambil baris antrean yang siap diproses dengan query FOR UPDATE SKIP LOCKED.
//
// Status baris langsung diubah menjadi 'delivering' dan sewa (locked_at, locked_by) dicatat.
// Ini menjamin beberapa worker tidak mengambil baris yang sama tanpa saling memblokir.
func (r *Repo) ClaimDue(ctx context.Context, workerID string, limit int) ([]*Delivery, error) {
	const op = "mengambil antrean webhook siap kirim"
	if limit <= 0 {
		limit = 50
	}

	// Query pengambilan sesuai spesifikasi migrasi 0008:
	// CTE for update skip locked diurutkan berdasarkan next_attempt_at.
	rows, err := r.q.Query(ctx, `
		with claimed as (
			select id
			from webhook_deliveries
			where status in ('pending', 'failed') and next_attempt_at <= now()
			order by next_attempt_at
			limit $1
			for update skip locked
		)
		update webhook_deliveries d
		set status = 'delivering',
		    locked_at = now(),
		    locked_by = $2
		from claimed
		where d.id = claimed.id
		returning `+deliveryPrefixedColumns,
		limit, workerID,
	)

	if err != nil {
		return nil, repo.Err(op, err)
	}
	defer rows.Close()

	var out []*Delivery
	for rows.Next() {
		d, err := scanDelivery(rows)
		if err != nil {
			return nil, repo.Err(op, err)
		}
		out = append(out, d)
	}
	if err := rows.Err(); err != nil {
		return nil, repo.Err(op, err)
	}
	return out, nil
}

// RecoverStuck memulihkan pengiriman yang tertinggal di status 'delivering' akibat worker mati.
//
// Mengembalikan jumlah baris yang berhasil dipulihkan ke status 'pending'.
func (r *Repo) RecoverStuck(ctx context.Context, stuckDuration time.Duration) (int64, error) {
	const op = "memulihkan pengiriman webhook tersangkut"
	if stuckDuration <= 0 {
		stuckDuration = 5 * time.Minute
	}

	// Sesuai query migrasi 0008, locked_at dan locked_by wajib di-null-kan saat status keluar dari delivering
	// untuk mematuhi check constraint webhook_deliveries_lease_consistent.
	tag, err := r.q.Exec(ctx, `
		update webhook_deliveries
		set status = 'pending', locked_at = null, locked_by = null
		where status = 'delivering' and locked_at < now() - ($1 * interval '1 millisecond')`,
		stuckDuration.Milliseconds(),
	)
	if err != nil {
		return 0, repo.Err(op, err)
	}
	return tag.RowsAffected(), nil
}

// RecordSuccess menandai pengiriman berhasil (HTTP 2xx) dan mereset kegagalan berurutan webhook.
func (r *Repo) RecordSuccess(ctx context.Context, deliveryID int64, webhookID string, statusCode int) error {
	const op = "mencatat keberhasilan pengiriman webhook"

	// 1. Perbarui riwayat delivery. Sesuai constraint webhook_deliveries_delivered_consistent,
	// delivered_at wajib diisi bila status='delivered'. locked_at wajib null bila status != 'delivering'.
	_, err := r.q.Exec(ctx, `
		update webhook_deliveries
		set status = 'delivered',
		    locked_at = null,
		    locked_by = null,
		    delivered_at = now(),
		    response_status_code = $2,
		    error_message = null
		where id = $1`,
		deliveryID, statusCode,
	)
	if err != nil {
		return repo.Err(op, err)
	}

	// 2. Perbarui tabel induk webhook: reset consecutive_failures ke nol.
	_, err = r.q.Exec(ctx, `
		update webhooks
		set last_delivery_at = now(),
		    last_delivery_status = 'delivered',
		    consecutive_failures = 0,
		    updated_at = now()
		where id = $1::uuid`,
		webhookID,
	)
	if err != nil {
		return repo.Err(op, err)
	}

	return nil
}

// RecordFailure menandai pengiriman gagal, menjadwalkan percobaan berikutnya atau membiarkannya abandoned.
//
// Aturan 6: consecutive_failures dinaikkan, tetapi worker TIDAK BOLEH menonaktifkan endpoint otomatis.
func (r *Repo) RecordFailure(
	ctx context.Context,
	deliveryID int64,
	webhookID string,
	attemptCount int,
	maxRetries int,
	nextAttempt time.Time,
	statusCode *int,
	sanitizedErrMsg string,
) error {
	const op = "mencatat kegagalan pengiriman webhook"

	status := StatusFailed
	// Sesuai definisi skema, max_retries adalah percobaan ulang SETELAH percobaan pertama,
	// sehingga total percobaan adalah 1 + max_retries. Status beralih ke abandoned jika
	// attemptCount sudah melampaui maxRetries.
	if attemptCount > maxRetries {
		status = StatusAbandoned
	}

	// 1. Perbarui tabel deliveries: sewa dilepas (locked_at = null).
	_, err := r.q.Exec(ctx, `
		update webhook_deliveries
		set status = $2,
		    locked_at = null,
		    locked_by = null,
		    attempt_count = $3,
		    next_attempt_at = $4,
		    response_status_code = $5,
		    error_message = nullif($6, '')
		where id = $1`,
		deliveryID, status, attemptCount, nextAttempt, statusCode, sanitizedErrMsg,
	)
	if err != nil {
		return repo.Err(op, err)
	}

	// 2. Perbarui tabel webhooks: consecutive_failures dinaikkan.
	_, err = r.q.Exec(ctx, `
		update webhooks
		set last_delivery_at = now(),
		    last_delivery_status = $2,
		    consecutive_failures = consecutive_failures + 1,
		    updated_at = now()
		where id = $1::uuid`,
		webhookID, status,
	)
	if err != nil {
		return repo.Err(op, err)
	}

	return nil
}

// DeleteRetention membersihkan riwayat pengiriman delivered dan abandoned secara ber-batch.
//
// Memanfaatkan indeks partial webhook_deliveries_created_idx untuk efisiensi.
func (r *Repo) DeleteRetention(ctx context.Context, cutoff time.Time, batchSize int) (int64, error) {
	const op = "membersihkan retensi webhook deliveries"
	if batchSize <= 0 {
		batchSize = 500
	}

	tag, err := r.q.Exec(ctx, `
		with to_delete as (
			select id
			from webhook_deliveries
			where status in ('delivered', 'abandoned') and created_at < $1
			limit $2
		)
		delete from webhook_deliveries
		where id in (select id from to_delete)`,
		cutoff, batchSize,
	)
	if err != nil {
		return 0, repo.Err(op, err)
	}
	return tag.RowsAffected(), nil
}

// GetDelivery mengambil satu rekaman delivery untuk keperluan verifikasi/test.
func (r *Repo) GetDelivery(ctx context.Context, id int64) (*Delivery, error) {
	const op = "mengambil delivery webhook"
	row := r.q.QueryRow(ctx, `select `+deliveryColumns+` from webhook_deliveries where id = $1`, id)
	d, err := scanDelivery(row)
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return nil, repo.ErrNotFound
		}
		return nil, repo.Err(op, err)
	}
	return d, nil
}

// UpdateWebhookParams berisi opsi pembaruan webhook.
type UpdateWebhookParams struct {
	Name       *string
	URL        *string
	Events     []string
	Enabled    *bool
	MaxRetries *int
	TimeoutMS  *int
	SSRFPolicy security.SSRFPolicy
}

// Update memperbarui konfigurasi webhook.
func (r *Repo) Update(ctx context.Context, id string, p UpdateWebhookParams) (*Webhook, error) {
	const op = "memperbarui webhook"
	var (
		setClauses []string
		args       []any
	)
	args = append(args, id)

	if p.Name != nil {
		args = append(args, *p.Name)
		setClauses = append(setClauses, fmt.Sprintf("name = $%d", len(args)))
	}
	if p.URL != nil {
		if err := security.ValidateBaseURL(*p.URL, p.SSRFPolicy); err != nil {
			return nil, fmt.Errorf("%s: URL webhook ditolak kebijakan keamanan: %w", op, err)
		}
		args = append(args, *p.URL)
		setClauses = append(setClauses, fmt.Sprintf("url = $%d", len(args)))
	}
	if p.Events != nil {
		args = append(args, p.Events)
		setClauses = append(setClauses, fmt.Sprintf("events = $%d", len(args)))
	}
	if p.Enabled != nil {
		args = append(args, *p.Enabled)
		setClauses = append(setClauses, fmt.Sprintf("enabled = $%d", len(args)))
	}
	if p.MaxRetries != nil {
		args = append(args, *p.MaxRetries)
		setClauses = append(setClauses, fmt.Sprintf("max_retries = $%d", len(args)))
	}
	if p.TimeoutMS != nil {
		args = append(args, *p.TimeoutMS)
		setClauses = append(setClauses, fmt.Sprintf("timeout_ms = $%d", len(args)))
	}

	if len(setClauses) == 0 {
		return r.Get(ctx, id)
	}

	setClauses = append(setClauses, "updated_at = now()")
	query := fmt.Sprintf(`update webhooks set %s where id = $1::uuid returning %s`,
		strings.Join(setClauses, ", "), webhookColumns)

	row := r.q.QueryRow(ctx, query, args...)
	w, err := scanWebhook(row)
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return nil, repo.ErrNotFound
		}
		return nil, repo.Err(op, err)
	}
	return w, nil
}

// SetEnabled mengaktifkan atau menonaktifkan endpoint webhook.
func (r *Repo) SetEnabled(ctx context.Context, id string, enabled bool) (*Webhook, error) {
	return r.Update(ctx, id, UpdateWebhookParams{Enabled: &enabled})
}

// Delete menghapus konfigurasi webhook beserta seluruh data delivery terkait.
func (r *Repo) Delete(ctx context.Context, id string) error {
	const op = "menghapus webhook"
	tag, err := r.q.Exec(ctx, `delete from webhooks where id = $1::uuid`, id)
	if err != nil {
		return repo.Err(op, err)
	}
	if tag.RowsAffected() == 0 {
		return repo.ErrNotFound
	}
	return nil
}

// ListDeliveries mengambil riwayat pengiriman per webhook dengan keyset pagination berurutan id desc.
func (r *Repo) ListDeliveries(ctx context.Context, webhookID string, page repo.Page) ([]*Delivery, string, error) {
	const op = "mendaftar delivery webhook"
	limit := page.Normalize()

	var (
		query strings.Builder
		args  []any
	)
	args = append(args, webhookID)
	query.WriteString(`select ` + deliveryColumns + ` from webhook_deliveries where webhook_id = $1::uuid `)

	if page.Cursor != "" {
		var cursorID int64
		if _, err := fmt.Sscanf(page.Cursor, "%d", &cursorID); err == nil && cursorID > 0 {
			args = append(args, cursorID)
			query.WriteString(fmt.Sprintf(`and id < $%d `, len(args)))
		}
	}

	args = append(args, limit+1)
	query.WriteString(fmt.Sprintf(`order by id desc limit $%d`, len(args)))

	rows, err := r.q.Query(ctx, query.String(), args...)
	if err != nil {
		return nil, "", repo.Err(op, err)
	}
	defer rows.Close()

	var items []*Delivery
	for rows.Next() {
		d, err := scanDelivery(rows)
		if err != nil {
			return nil, "", repo.Err(op, err)
		}
		items = append(items, d)
	}
	if err := rows.Err(); err != nil {
		return nil, "", repo.Err(op, err)
	}

	var next string
	if len(items) > limit {
		items = items[:limit]
		next = fmt.Sprintf("%d", items[limit-1].ID)
	}

	return items, next, nil
}

// newID menghasilkan UUID v4 acak untuk pengenal entitas sebelum disimpan.
func newID() (string, error) {
	var b [16]byte
	if _, err := rand.Read(b[:]); err != nil {
		return "", fmt.Errorf("membuat pengenal acak: %w", err)
	}
	b[6] = (b[6] & 0x0f) | 0x40
	b[8] = (b[8] & 0x3f) | 0x80
	return fmt.Sprintf("%x-%x-%x-%x-%x", b[0:4], b[4:6], b[6:8], b[8:10], b[10:16]), nil
}
