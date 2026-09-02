package identity

import (
	"context"
	"encoding/json"
	"fmt"
	"strconv"
	"strings"
	"time"

	"github.com/NexGen-X/Route-X/internal/database/repo"
)

// Nama aksi audit.
//
// Dijadikan konstanta, bukan string yang ditulis ulang di setiap handler, karena nilai
// ini yang dipakai menyaring audit log: satu salah ketik di satu handler membuat
// aksinya tidak pernah muncul pada penyaringan mana pun, dan kesalahan seperti itu
// baru terasa saat catatan justru sedang dibutuhkan. Bentuknya "sumber_daya.aksi"
// supaya awalannya bisa dipakai memilih satu kelompok aksi sekaligus.
const (
	// ActionLogin dicatat setelah sesi berhasil dibuat.
	ActionLogin = "auth.login"
	// ActionLogout dicatat saat pengguna mencabut sesinya sendiri.
	ActionLogout = "auth.logout"
	// ActionLoginFailed dicatat pada setiap percobaan login yang gagal, termasuk untuk
	// email yang tidak terdaftar — di situ actor_user_id kosong dan hanya cuplikan
	// email yang tersimpan.
	ActionLoginFailed = "auth.login_failed"
	// ActionPasswordChange dicatat setelah password berhasil diganti pemiliknya.
	ActionPasswordChange = "auth.password_change"
	// ActionPasswordChangeFailed dicatat saat penggantian password ditolak, mis. karena
	// password lama salah atau yang baru terlalu lemah.
	ActionPasswordChangeFailed = "auth.password_change_failed"

	// ActionProviderCreate, ActionProviderUpdate, dan ActionProviderDelete mencatat
	// perubahan pada provider upstream.
	ActionProviderCreate = "provider.create"
	ActionProviderUpdate = "provider.update"
	ActionProviderDelete = "provider.delete"

	// ActionAPIKeyCreate, ActionAPIKeyRotate, dan ActionAPIKeyRevoke mencatat siklus
	// hidup API key. Nilai key-nya sendiri tidak boleh masuk metadata.
	ActionAPIKeyCreate = "api_key.create"
	ActionAPIKeyRotate = "api_key.rotate"
	ActionAPIKeyRevoke = "api_key.revoke"

	// ActionRoutingUpdate mencatat perubahan aturan perutean.
	ActionRoutingUpdate = "routing.update"
	// ActionSettingUpdate mencatat perubahan setelan runtime.
	ActionSettingUpdate = "setting.update"
)

// Jenis sumber daya audit, dipakai bersama resource_id untuk menelusuri riwayat satu
// objek lewat indeks audit_logs_resource_idx.
const (
	ResourceUser     = "user"
	ResourceRole     = "role"
	ResourceSession  = "session"
	ResourceProvider = "provider"
	ResourceAPIKey   = "api_key"
	ResourceRouting  = "routing"
	ResourceSetting  = "setting"
)

// Actor adalah pelaku sebuah aksi.
//
// Email dan Role disimpan sebagai cuplikan di baris audit, bukan hanya sebagai
// referensi ke users: kolom actor_user_id di-set null saat akun pelaku dihapus, dan
// catatan audit harus tetap menjawab "siapa yang melakukan ini" setelah akunnya lama
// tidak ada.
type Actor struct {
	// UserID boleh kosong untuk aksi oleh sistem atau percobaan login dengan email
	// yang tidak terdaftar.
	UserID string
	Email  string
	Role   string
}

// ActorOf menyusun cuplikan pelaku dari pengguna dan nama peran yang sedang dipakainya.
func ActorOf(u User, role string) Actor {
	return Actor{UserID: u.ID, Email: u.Email, Role: role}
}

// Event adalah satu aksi yang akan dicatat.
type Event struct {
	Actor        Actor
	Action       string
	ResourceType string
	// ResourceID boleh kosong untuk aksi yang tidak menyasar satu objek tertentu.
	ResourceID string

	IP        string
	UserAgent string
	// RequestID menautkan catatan audit dengan log aplikasi untuk request yang sama.
	RequestID string

	// Metadata adalah detail tambahan. Nilai sensitif WAJIB sudah tersamar sebelum
	// masuk sini; membungkusnya dengan security.Secret sudah cukup, karena tipe itu
	// selalu terserialisasi sebagai "[REDACTED]".
	Metadata map[string]any
}

// Entry adalah satu baris audit_logs yang dibaca kembali.
type Entry struct {
	ID          int64     `json:"id"`
	OccurredAt  time.Time `json:"occurred_at"`
	ActorUserID string    `json:"actor_user_id,omitempty"`
	ActorEmail  string    `json:"actor_email,omitempty"`
	ActorRole   string    `json:"actor_role,omitempty"`

	Action       string `json:"action"`
	ResourceType string `json:"resource_type"`
	ResourceID   string `json:"resource_id,omitempty"`

	IP        string `json:"ip,omitempty"`
	UserAgent string `json:"user_agent,omitempty"`
	RequestID string `json:"request_id,omitempty"`

	Metadata json.RawMessage `json:"metadata"`
}

// AuditFilter menyaring pembacaan audit log. Field kosong berarti tidak menyaring.
type AuditFilter struct {
	ActorUserID  string
	Action       string
	ResourceType string
	ResourceID   string
	// From inklusif, To eksklusif.
	From time.Time
	To   time.Time
}

// AuditPage adalah satu halaman audit log.
type AuditPage struct {
	Entries []Entry `json:"entries"`
	// NextCursor kosong berarti tidak ada halaman berikutnya.
	NextCursor string `json:"next_cursor,omitempty"`
}

// Audit adalah repository tabel audit_logs.
type Audit struct {
	q repo.Querier
}

// NewAudit membuat repository audit di atas q. Kirim pgx.Tx dari repo.InTx bila
// catatan audit harus ikut dibatalkan saat aksinya sendiri gagal.
func NewAudit(q repo.Querier) *Audit { return &Audit{q: q} }

// entryColumns adalah kolom yang dibaca setiap query audit.
const entryColumns = `id, occurred_at, coalesce(actor_user_id::text, ''), coalesce(actor_email, ''),
	coalesce(actor_role, ''), action, resource_type, coalesce(resource_id, ''),
	coalesce(host(ip), ''), coalesce(user_agent, ''), coalesce(request_id, ''), metadata`

// Write mencatat satu aksi.
//
// Action dan ResourceType wajib ada: catatan tanpa keduanya tidak bisa disaring dan
// karena itu tidak punya nilai sebagai audit. Sisanya opsional — pelaku boleh kosong
// untuk aksi sistem, dan alamat IP yang tidak bisa diurai disimpan sebagai NULL.
func (r *Audit) Write(ctx context.Context, e Event) error {
	const op = "menulis catatan audit"

	action := strings.TrimSpace(e.Action)
	resourceType := strings.TrimSpace(e.ResourceType)
	if action == "" || resourceType == "" {
		return fmt.Errorf("%s: %w: aksi dan jenis sumber daya wajib diisi", op, ErrInvalidInput)
	}
	if e.Actor.UserID != "" && !validUUID(e.Actor.UserID) {
		return fmt.Errorf("%s: %w: ID pelaku bukan UUID", op, ErrInvalidInput)
	}

	metadata, err := marshalMetadata(e.Metadata)
	if err != nil {
		return fmt.Errorf("%s: %w", op, err)
	}

	const query = `insert into audit_logs
		(actor_user_id, actor_email, actor_role, action, resource_type, resource_id,
		 ip, user_agent, request_id, metadata)
	values ($1, $2, $3, $4, $5, $6, $7, $8, $9, $10)`

	_, err = r.q.Exec(ctx, query,
		textParam(e.Actor.UserID), textParam(strings.TrimSpace(e.Actor.Email)), textParam(e.Actor.Role),
		action, resourceType, textParam(e.ResourceID),
		ipParam(e.IP), textParam(truncate(e.UserAgent, maxUserAgentLen)),
		textParam(truncate(e.RequestID, maxRequestIDLen)), metadata)
	if err != nil {
		return repo.Err(op, err)
	}
	return nil
}

// marshalMetadata mengubah detail tambahan menjadi jsonb. Nil menjadi objek kosong,
// bukan NULL, supaya kolomnya selalu bisa di-query dengan operator jsonb tanpa
// pemeriksaan NULL di setiap query.
func marshalMetadata(m map[string]any) ([]byte, error) {
	if len(m) == 0 {
		return []byte(`{}`), nil
	}
	raw, err := json.Marshal(m)
	if err != nil {
		// Pesan json.Marshal bisa memuat isi nilai yang gagal diserialisasi, jadi
		// hanya jenis kesalahannya yang diteruskan.
		return nil, fmt.Errorf("%w: metadata tidak bisa diubah menjadi JSON", ErrInvalidInput)
	}
	return raw, nil
}

// List mengembalikan satu halaman audit log, terbaru lebih dulu.
//
// Filter yang tidak dipakai tidak menyisakan potongan apa pun di SQL, sehingga
// PostgreSQL bisa memilih indeks yang paling sesuai dengan penyaringan yang benar-benar
// diminta — per pelaku, per aksi, atau per sumber daya. Paginasinya keyset atas
// (occurred_at, id), jadi biayanya tetap sama pada halaman ke seribu.
func (r *Audit) List(ctx context.Context, f AuditFilter, p repo.Page) (AuditPage, error) {
	const op = "mendaftar catatan audit"

	conds := &clauseList{}
	if f.ActorUserID != "" {
		if !validUUID(f.ActorUserID) {
			return AuditPage{}, fmt.Errorf("%s: %w: ID pelaku bukan UUID", op, ErrInvalidInput)
		}
		conds.add("actor_user_id = %s::uuid", f.ActorUserID)
	}
	if f.Action != "" {
		conds.add("action = %s", f.Action)
	}
	if f.ResourceType != "" {
		conds.add("resource_type = %s", f.ResourceType)
	}
	if f.ResourceID != "" {
		conds.add("resource_id = %s", f.ResourceID)
	}
	if !f.From.IsZero() {
		conds.add("occurred_at >= %s::timestamptz", f.From)
	}
	if !f.To.IsZero() {
		conds.add("occurred_at < %s::timestamptz", f.To)
	}
	if p.Cursor != "" {
		at, id, err := decodeCursor(p.Cursor)
		if err != nil {
			return AuditPage{}, fmt.Errorf("%s: %w", op, err)
		}
		n, err := strconv.ParseInt(id, 10, 64)
		if err != nil {
			return AuditPage{}, fmt.Errorf("%s: %w: pengenal di dalam cursor bukan bilangan", op, ErrInvalidCursor)
		}
		conds.add("(occurred_at, id) < (%s::timestamptz, %s::bigint)", at, n)
	}

	limit := p.Normalize()
	where := conds.where()
	rowLimit := conds.placeholder(limit + 1)
	query := `select ` + entryColumns + ` from audit_logs` + where +
		` order by occurred_at desc, id desc limit ` + rowLimit

	rows, err := r.q.Query(ctx, query, conds.args...)
	if err != nil {
		return AuditPage{}, repo.Err(op, err)
	}
	defer rows.Close()

	page := AuditPage{Entries: make([]Entry, 0, limit)}
	for rows.Next() {
		var (
			e        Entry
			metadata []byte
		)
		if err := rows.Scan(&e.ID, &e.OccurredAt, &e.ActorUserID, &e.ActorEmail, &e.ActorRole,
			&e.Action, &e.ResourceType, &e.ResourceID,
			&e.IP, &e.UserAgent, &e.RequestID, &metadata); err != nil {
			return AuditPage{}, repo.Err(op, err)
		}
		e.Metadata = json.RawMessage(metadata)
		page.Entries = append(page.Entries, e)
	}
	if err := rows.Err(); err != nil {
		return AuditPage{}, repo.Err(op, err)
	}

	if len(page.Entries) > limit {
		last := page.Entries[limit-1]
		page.Entries = page.Entries[:limit]
		page.NextCursor = encodeCursor(last.OccurredAt, strconv.FormatInt(last.ID, 10))
	}
	return page, nil
}
