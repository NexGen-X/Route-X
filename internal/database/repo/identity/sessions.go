package identity

import (
	"context"
	"crypto/rand"
	"crypto/sha256"
	"encoding/base64"
	"encoding/hex"
	"fmt"
	"time"

	"github.com/jackc/pgx/v5"

	"github.com/NexGen-X/Route-X/internal/database/repo"
	"github.com/NexGen-X/Route-X/internal/security"
)

// sessionTokenBytes adalah jumlah byte acak pada token sesi.
//
// 32 byte berarti 256 bit entropi. Dengan ruang tebakan sebesar itu token tidak perlu
// di-hash lambat seperti password: SHA-256 tanpa salt sudah memadai, dan justru itu
// yang diperlukan agar hash-nya deterministik sehingga sesi bisa dicari lewat indeks
// unik pada satu perbandingan.
const sessionTokenBytes = 32

// DefaultSessionTTL adalah masa berlaku sesi bila pemanggil tidak menentukan sendiri.
const DefaultSessionTTL = 12 * time.Hour

// Session adalah satu baris tabel sessions. Token maupun hash-nya tidak ikut: hash
// tidak berguna bagi pemanggil, dan membawanya keliling hanya menambah tempat ia bisa
// bocor.
type Session struct {
	ID     string `json:"id"`
	UserID string `json:"user_id"`

	IP        string `json:"ip,omitempty"`
	UserAgent string `json:"user_agent,omitempty"`

	CreatedAt  time.Time  `json:"created_at"`
	LastSeenAt time.Time  `json:"last_seen_at"`
	ExpiresAt  time.Time  `json:"expires_at"`
	RevokedAt  *time.Time `json:"revoked_at,omitempty"`
}

// NewSession adalah masukan pembuatan sesi. TTL ≤ 0 memakai DefaultSessionTTL.
type NewSession struct {
	UserID    string
	TTL       time.Duration
	IP        string
	UserAgent string
}

// CreatedSession adalah sesi baru beserta token mentahnya.
type CreatedSession struct {
	Session
	// Token adalah satu-satunya kesempatan membaca token sesi. Yang tersimpan di
	// database hanyalah hash SHA-256-nya, jadi nilai ini tidak bisa dipulihkan lagi
	// setelah respons dikirim — kirimkan langsung sebagai cookie dan jangan simpan
	// ke mana pun.
	Token security.Secret
}

// Sessions adalah repository tabel sessions.
type Sessions struct {
	q repo.Querier
}

// NewSessions membuat repository sesi di atas q.
func NewSessions(q repo.Querier) *Sessions { return &Sessions{q: q} }

// sessionColumns adalah kolom yang dibaca setiap query sesi, dalam urutan yang
// diharapkan scanSession.
const sessionColumns = `id::text, user_id::text, coalesce(host(ip), ''), coalesce(user_agent, ''),
	created_at, last_seen_at, expires_at, revoked_at`

// scanSession membaca satu baris sesi.
func scanSession(row pgx.Row) (Session, error) {
	var s Session
	err := row.Scan(&s.ID, &s.UserID, &s.IP, &s.UserAgent,
		&s.CreatedAt, &s.LastSeenAt, &s.ExpiresAt, &s.RevokedAt)
	if err != nil {
		return Session{}, err
	}
	return s, nil
}

// newSessionToken menghasilkan token sesi acak beserta hash simpanannya.
func newSessionToken() (raw security.Secret, hash string, err error) {
	buf := make([]byte, sessionTokenBytes)
	if _, err := rand.Read(buf); err != nil {
		return "", "", fmt.Errorf("membuat token sesi: %w", err)
	}
	// base64url dipilih daripada heksadesimal supaya token lebih pendek, dan agar
	// aman dipakai sebagai nilai cookie tanpa perlu escaping.
	token := base64.RawURLEncoding.EncodeToString(buf)
	return security.Secret(token), hashSessionToken(token), nil
}

// hashSessionToken menghitung nilai yang disimpan di kolom token_hash.
//
// Yang di-hash adalah token dalam bentuk yang dikirim ke klien, bukan byte acaknya,
// sehingga pencarian sesi cukup meng-hash apa yang datang di header tanpa langkah
// penguraian yang bisa salah.
func hashSessionToken(token string) string {
	sum := sha256.Sum256([]byte(token))
	return hex.EncodeToString(sum[:])
}

// Create membuat sesi baru dan mengembalikan token mentahnya satu kali.
//
// Token dibuat di sini, bukan diterima dari pemanggil, supaya hanya ada satu tempat
// yang menentukan panjang dan sumber keacakannya. Yang masuk database hanya hash-nya:
// dump database karena itu tidak bisa dipakai membajak sesi yang sedang berjalan.
func (r *Sessions) Create(ctx context.Context, in NewSession) (CreatedSession, error) {
	const op = "membuat sesi"
	if !validUUID(in.UserID) {
		return CreatedSession{}, fmt.Errorf("%s: %w", op, repo.ErrInvalidReference)
	}
	ttl := in.TTL
	if ttl <= 0 {
		ttl = DefaultSessionTTL
	}

	raw, hash, err := newSessionToken()
	if err != nil {
		return CreatedSession{}, fmt.Errorf("%s: %w", op, err)
	}

	const query = `insert into sessions (user_id, token_hash, ip, user_agent, expires_at)
	values ($1, $2, $3, $4, now() + make_interval(secs => $5))
	returning ` + sessionColumns

	s, err := scanSession(r.q.QueryRow(ctx, query, in.UserID, hash,
		ipParam(in.IP), textParam(truncate(in.UserAgent, maxUserAgentLen)), seconds(ttl)))
	if err != nil {
		return CreatedSession{}, repo.Err(op, err)
	}
	return CreatedSession{Session: s, Token: raw}, nil
}

// Lookup mencari sesi yang masih berlaku berdasarkan token mentah.
//
// Kedaluwarsa dan pencabutan disaring di dalam query, bukan setelahnya di Go. Bedanya
// bukan gaya: kalau penyaringan ada di Go, setiap jalur keluar yang lupa memeriksanya
// — atau satu pemanggil baru yang memakai hasilnya lebih awal — langsung menjadi
// lubang autentikasi. Dengan syaratnya melekat di query, sesi kedaluwarsa atau yang
// sudah dicabut tidak pernah ada bentuknya sebagai nilai yang bisa dipakai; yang
// keluar hanya repo.ErrNotFound.
//
// Token yang tidak dikenal juga menghasilkan repo.ErrNotFound, tanpa membedakannya
// dari sesi yang kedaluwarsa: pemanggil tidak boleh membocorkan perbedaan itu ke klien.
func (r *Sessions) Lookup(ctx context.Context, token security.Secret) (Session, error) {
	const op = "mencari sesi"
	if token.IsZero() {
		// Permintaan tanpa cookie tidak perlu perjalanan ke database.
		return Session{}, fmt.Errorf("%s: %w", op, repo.ErrNotFound)
	}

	const query = `select ` + sessionColumns + ` from sessions
	where token_hash = $1 and revoked_at is null and expires_at > now()`

	s, err := scanSession(r.q.QueryRow(ctx, query, hashSessionToken(token.Reveal())))
	if err != nil {
		return Session{}, repo.Err(op, err)
	}
	return s, nil
}

// Touch memperbarui last_seen_at sebuah sesi yang masih berlaku.
//
// Dipisah dari Lookup dan tidak dilakukan otomatis di sana: Lookup terjadi pada setiap
// request dashboard, dan menuliskan satu baris pada setiap request akan menghasilkan
// WAL serta baris mati yang sepenuhnya tidak sebanding dengan nilai datanya. Pemanggil
// memutuskan sendiri seberapa sering jejak ini disegarkan.
func (r *Sessions) Touch(ctx context.Context, id string) error {
	const op = "memperbarui jejak sesi"
	if !validUUID(id) {
		return fmt.Errorf("%s: %w", op, repo.ErrNotFound)
	}

	const query = `update sessions set last_seen_at = now()
	where id = $1 and revoked_at is null and expires_at > now()`

	tag, err := r.q.Exec(ctx, query, id)
	if err != nil {
		return repo.Err(op, err)
	}
	if tag.RowsAffected() == 0 {
		return fmt.Errorf("%s: %w", op, repo.ErrNotFound)
	}
	return nil
}

// Revoke mencabut satu sesi.
//
// Aman dipanggil dua kali: waktu pencabutan pertama dipertahankan lewat COALESCE,
// sehingga logout ganda tidak memundurkan jejak audit dan tidak dilaporkan sebagai
// kesalahan. repo.ErrNotFound berarti sesinya benar-benar tidak ada.
func (r *Sessions) Revoke(ctx context.Context, id string) error {
	const op = "mencabut sesi"
	if !validUUID(id) {
		return fmt.Errorf("%s: %w", op, repo.ErrNotFound)
	}

	const query = `update sessions set revoked_at = coalesce(revoked_at, now()) where id = $1`
	tag, err := r.q.Exec(ctx, query, id)
	if err != nil {
		return repo.Err(op, err)
	}
	if tag.RowsAffected() == 0 {
		return fmt.Errorf("%s: %w", op, repo.ErrNotFound)
	}
	return nil
}

// RevokeAllOfUser mencabut seluruh sesi aktif seorang pengguna dan mengembalikan
// jumlahnya.
//
// Dipakai saat password diganti, akun dinonaktifkan, atau peran dicabut — tiga momen
// ketika sesi yang sudah berjalan tidak boleh lagi mewakili kewenangan lama. Kalau
// pemanggil ingin sesi yang sedang dipakainya tetap hidup, buat sesi baru setelah ini.
func (r *Sessions) RevokeAllOfUser(ctx context.Context, userID string) (int, error) {
	const op = "mencabut seluruh sesi pengguna"
	if !validUUID(userID) {
		return 0, fmt.Errorf("%s: %w", op, repo.ErrNotFound)
	}

	const query = `update sessions set revoked_at = now() where user_id = $1 and revoked_at is null`
	tag, err := r.q.Exec(ctx, query, userID)
	if err != nil {
		return 0, repo.Err(op, err)
	}
	return int(tag.RowsAffected()), nil
}

// ListActive mengembalikan sesi yang masih berlaku milik seorang pengguna, paling baru
// terlihat lebih dulu. Inilah daftar "perangkat yang sedang masuk" di dashboard.
func (r *Sessions) ListActive(ctx context.Context, userID string) ([]Session, error) {
	const op = "mendaftar sesi aktif"
	if !validUUID(userID) {
		return nil, fmt.Errorf("%s: %w", op, repo.ErrNotFound)
	}

	const query = `select ` + sessionColumns + ` from sessions
	where user_id = $1 and revoked_at is null and expires_at > now()
	order by last_seen_at desc`

	rows, err := r.q.Query(ctx, query, userID)
	if err != nil {
		return nil, repo.Err(op, err)
	}
	defer rows.Close()

	var out []Session
	for rows.Next() {
		s, err := scanSession(rows)
		if err != nil {
			return nil, repo.Err(op, err)
		}
		out = append(out, s)
	}
	if err := rows.Err(); err != nil {
		return nil, repo.Err(op, err)
	}
	return out, nil
}

// DeleteExpired menghapus sesi yang sudah kedaluwarsa atau dicabut lebih lama dari
// grace, lalu mengembalikan jumlah baris yang terhapus. Dipakai worker pembersih.
//
// Tenggang waktu ada supaya sesi yang baru saja habis masih bisa dilihat saat
// menelusuri keluhan "kok saya tiba-tiba keluar"; setelah lewat, barisnya tidak
// bernilai lagi dan hanya menambah beban indeks.
func (r *Sessions) DeleteExpired(ctx context.Context, grace time.Duration) (int, error) {
	const op = "menghapus sesi kedaluwarsa"

	// Perbandingan dengan NULL menghasilkan NULL, jadi sesi yang belum dicabut hanya
	// tersaring oleh syarat pertama, dan sebaliknya.
	const query = `delete from sessions
	where expires_at < now() - make_interval(secs => $1)
	   or revoked_at < now() - make_interval(secs => $1)`

	tag, err := r.q.Exec(ctx, query, seconds(grace))
	if err != nil {
		return 0, repo.Err(op, err)
	}
	return int(tag.RowsAffected()), nil
}
