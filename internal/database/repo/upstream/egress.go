package upstream

import (
	"context"
	"fmt"
	"time"

	"github.com/NexGen-X/Route-X/internal/database/repo"
	"github.com/NexGen-X/Route-X/internal/security"
)

// EgressPool adalah satu jalur keluar untuk permintaan upstream, sebagaimana boleh
// dilihat manusia.
//
// Seperti CredentialMeta, tipe ini tidak punya field untuk URL proxy — baik terenkripsi
// maupun tidak. URL-nya hanya keluar lewat ProxyURL, yang hasilnya bertipe
// security.Secret.
type EgressPool struct {
	ID   string
	Name string
	Kind string
	// MaskedHint bukan rahasia, mis. "http://user:****@host:3128".
	MaskedHint      string
	EncryptionKeyID string

	Enabled bool
	Weight  int
	Region  *string

	LastHealthStatus *string
	LastHealthAt     *time.Time
	LastLatencyMS    *int

	CreatedAt time.Time
	UpdatedAt time.Time
	CreatedBy *string
}

// EgressRepo adalah akses data jalur keluar.
type EgressRepo struct {
	q      repo.Querier
	cipher *security.Cipher
}

// NewEgressRepo membuat EgressRepo. cipher wajib ada karena URL proxy hampir selalu
// memuat kredensial dan karena itu tidak pernah disimpan sebagai teks biasa.
func NewEgressRepo(q repo.Querier, cipher *security.Cipher) (*EgressRepo, error) {
	if cipher == nil {
		return nil, fmt.Errorf("jalur keluar tidak bisa dilayani tanpa cipher: %w", security.ErrInvalidKeyLength)
	}
	return &EgressRepo{q: q, cipher: cipher}, nil
}

// egressColumns tidak memuat proxy_url_encrypted.
const egressColumns = `
	id::text, name, kind, masked_hint, encryption_key_id,
	enabled, weight, region,
	last_health_status, last_health_at, last_latency_ms,
	created_at, updated_at, created_by::text`

// scanEgress membaca satu baris sesuai egressColumns.
func scanEgress(row pgxRow) (*EgressPool, error) {
	var e EgressPool
	err := row.Scan(
		&e.ID, &e.Name, &e.Kind, &e.MaskedHint, &e.EncryptionKeyID,
		&e.Enabled, &e.Weight, &e.Region,
		&e.LastHealthStatus, &e.LastHealthAt, &e.LastLatencyMS,
		&e.CreatedAt, &e.UpdatedAt, &e.CreatedBy,
	)
	if err != nil {
		return nil, err
	}
	return &e, nil
}

// CreateEgressParams adalah masukan pembuatan jalur keluar.
type CreateEgressParams struct {
	Name string
	// Kind adalah EgressHTTP, EgressHTTPS, atau EgressSOCKS5.
	Kind string
	// ProxyURL lengkap dengan kredensialnya, mis. "http://user:sandi@host:3128".
	ProxyURL security.Secret

	Enabled   *bool
	Weight    *int
	Region    *string
	CreatedBy *string
}

// Create menyimpan jalur keluar baru dengan URL proxy terenkripsi.
//
// Pengenal barisnya dibuat lebih dulu di Go dengan alasan yang sama seperti pada
// kredensial provider: AAD memuat pengenal itu, jadi nilainya harus sudah pasti sebelum
// enkripsi. AAD-nya memakai security.CredentialAAD karena internal/security adalah
// satu-satunya tempat yang boleh menentukan bentuk AAD, dan pengenal UUID sudah cukup
// unik untuk mengikat ciphertext ke barisnya.
func (r *EgressRepo) Create(ctx context.Context, p CreateEgressParams) (*EgressPool, error) {
	const op = "membuat jalur keluar"

	if err := refID(op, "created_by", p.CreatedBy); err != nil {
		return nil, err
	}
	if p.ProxyURL.IsZero() {
		return nil, fmt.Errorf("%s: %w: URL proxy kosong", op, repo.ErrConstraint)
	}

	id, err := newID()
	if err != nil {
		return nil, fmt.Errorf("%s: %w", op, err)
	}
	ciphertext, err := r.cipher.EncryptSecret(p.ProxyURL, security.CredentialAAD(id))
	if err != nil {
		return nil, fmt.Errorf("%s: %w", op, err)
	}

	row := r.q.QueryRow(ctx, `
		insert into egress_pool (
			id, name, kind, proxy_url_encrypted, encryption_key_id, masked_hint,
			enabled, weight, region, created_by
		) values ($1, $2, $3, $4, $5, $6, $7, $8, $9, $10)
		returning `+egressColumns,
		id, p.Name, p.Kind, ciphertext, r.cipher.KeyID(), maskProxyURL(p.ProxyURL),
		boolOr(p.Enabled, true), intOr(p.Weight, DefaultWeight), p.Region, p.CreatedBy,
	)

	pool, err := scanEgress(row)
	if err != nil {
		return nil, repo.Err(op, err)
	}
	return pool, nil
}

// Get mengambil satu jalur keluar.
func (r *EgressRepo) Get(ctx context.Context, id string) (*EgressPool, error) {
	const op = "mengambil jalur keluar"
	if !idOK(id) {
		return nil, fmt.Errorf("%s: %w", op, repo.ErrNotFound)
	}

	pool, err := scanEgress(r.q.QueryRow(ctx,
		`select `+egressColumns+` from egress_pool where id = $1`, id))
	if err != nil {
		return nil, repo.Err(op, err)
	}
	return pool, nil
}

// List mengembalikan jalur keluar, terurut nama. Tanpa paginasi: jumlahnya sekelas
// jumlah region yang dipakai operator, bukan jumlah pengguna.
func (r *EgressRepo) List(ctx context.Context, enabledOnly bool) ([]*EgressPool, error) {
	const op = "mendaftar jalur keluar"

	where := ""
	if enabledOnly {
		where = " where enabled"
	}
	rows, err := r.q.Query(ctx, `select `+egressColumns+` from egress_pool`+where+` order by name asc`)
	if err != nil {
		return nil, repo.Err(op, err)
	}
	defer rows.Close()

	var out []*EgressPool
	for rows.Next() {
		pool, err := scanEgress(rows)
		if err != nil {
			return nil, repo.Err(op, err)
		}
		out = append(out, pool)
	}
	if err := rows.Err(); err != nil {
		return nil, repo.Err(op, err)
	}
	return out, nil
}

// ProxyURL mengembalikan URL proxy yang sudah didekripsi, siap dipakai membentuk
// transport HTTP. Hasilnya tidak boleh masuk log maupun respons API.
func (r *EgressRepo) ProxyURL(ctx context.Context, id string) (security.Secret, error) {
	const op = "membuka URL proxy jalur keluar"
	if !idOK(id) {
		return "", fmt.Errorf("%s: %w", op, repo.ErrNotFound)
	}

	var ciphertext string
	if err := r.q.QueryRow(ctx,
		`select proxy_url_encrypted from egress_pool where id = $1`, id).Scan(&ciphertext); err != nil {
		return "", repo.Err(op, err)
	}

	url, err := r.cipher.DecryptSecret(ciphertext, security.CredentialAAD(id))
	if err != nil {
		return "", fmt.Errorf("%s (jalur keluar %s): %w", op, id, err)
	}
	return url, nil
}

// SetProxyURL mengganti URL proxy sebuah jalur keluar.
//
// Pengenal barisnya tidak berubah, jadi AAD-nya tetap sama dan ciphertext baru tetap
// terikat ke baris yang sama.
func (r *EgressRepo) SetProxyURL(ctx context.Context, id string, proxyURL security.Secret) (*EgressPool, error) {
	const op = "mengganti URL proxy jalur keluar"
	if !idOK(id) {
		return nil, fmt.Errorf("%s: %w", op, repo.ErrNotFound)
	}
	if proxyURL.IsZero() {
		return nil, fmt.Errorf("%s: %w: URL proxy kosong", op, repo.ErrConstraint)
	}

	ciphertext, err := r.cipher.EncryptSecret(proxyURL, security.CredentialAAD(id))
	if err != nil {
		return nil, fmt.Errorf("%s: %w", op, err)
	}

	pool, err := scanEgress(r.q.QueryRow(ctx, `
		update egress_pool
		set proxy_url_encrypted = $2, encryption_key_id = $3, masked_hint = $4
		where id = $1
		returning `+egressColumns,
		id, ciphertext, r.cipher.KeyID(), maskProxyURL(proxyURL)))
	if err != nil {
		return nil, repo.Err(op, err)
	}
	return pool, nil
}

// UpdateEgressParams adalah pembaruan sebagian sebuah jalur keluar. URL proxy diganti
// lewat SetProxyURL, bukan di sini, supaya jalur enkripsinya hanya satu.
type UpdateEgressParams struct {
	Name    *string
	Kind    *string
	Enabled *bool
	Weight  *int
	Region  Opt[string]
}

// Update mengubah field yang diminta saja.
func (r *EgressRepo) Update(ctx context.Context, id string, p UpdateEgressParams) (*EgressPool, error) {
	const op = "memperbarui jalur keluar"
	if !idOK(id) {
		return nil, fmt.Errorf("%s: %w", op, repo.ErrNotFound)
	}

	set := newUpdateSet(id)
	if p.Name != nil {
		set.add("name", *p.Name)
	}
	if p.Kind != nil {
		set.add("kind", *p.Kind)
	}
	if p.Enabled != nil {
		set.add("enabled", *p.Enabled)
	}
	if p.Weight != nil {
		set.add("weight", *p.Weight)
	}
	addOpt(set, "region", p.Region)

	if set.empty() {
		return nil, fmt.Errorf("%s: %w", op, ErrNoChanges)
	}

	pool, err := scanEgress(r.q.QueryRow(ctx,
		`update egress_pool set `+set.clause()+` where id = $1 returning `+egressColumns, set.args...))
	if err != nil {
		return nil, repo.Err(op, err)
	}
	return pool, nil
}

// Delete menghapus jalur keluar. Provider yang memakainya tidak ikut terhapus: kolom
// egress_pool_id mereka menjadi NULL (on delete set null), artinya lalu lintasnya kembali
// keluar tanpa proxy — perubahan perilaku yang perlu diketahui operator.
func (r *EgressRepo) Delete(ctx context.Context, id string) error {
	const op = "menghapus jalur keluar"
	if !idOK(id) {
		return fmt.Errorf("%s: %w", op, repo.ErrNotFound)
	}

	tag, err := r.q.Exec(ctx, `delete from egress_pool where id = $1`, id)
	if err != nil {
		return repo.Err(op, err)
	}
	if tag.RowsAffected() == 0 {
		return fmt.Errorf("%s: %w", op, repo.ErrNotFound)
	}
	return nil
}

// RecordHealth memperbarui cuplikan kesehatan jalur keluar. Tidak ada tabel riwayat
// untuk jalur keluar, jadi hanya cuplikannya yang disimpan.
func (r *EgressRepo) RecordHealth(ctx context.Context, id string, status string, latencyMS *int) error {
	const op = "mencatat kesehatan jalur keluar"
	if !idOK(id) {
		return fmt.Errorf("%s: %w", op, repo.ErrNotFound)
	}

	tag, err := r.q.Exec(ctx, `
		update egress_pool
		set last_health_status = $2, last_health_at = now(), last_latency_ms = $3
		where id = $1`, id, status, latencyMS)
	if err != nil {
		return repo.Err(op, err)
	}
	if tag.RowsAffected() == 0 {
		return fmt.Errorf("%s: %w", op, repo.ErrNotFound)
	}
	return nil
}

// Reencrypt memindahkan URL proxy dari kunci lama ke kunci aktif, dengan cara dan alasan
// yang sama seperti CredentialRepo.Reencrypt. Rotasi kunci harus mencakup tabel ini juga;
// kalau tidak, kunci lama tetap harus disimpan selamanya.
func (r *EgressRepo) Reencrypt(ctx context.Context, old *security.Cipher, limit int) (int, error) {
	const op = "memutar kunci URL proxy jalur keluar"

	if old == nil {
		return 0, fmt.Errorf("%s: kunci lama tidak diberikan", op)
	}
	if old.KeyID() == r.cipher.KeyID() {
		return 0, fmt.Errorf("%s: kunci lama sama dengan kunci aktif (%s), tidak ada yang perlu diputar",
			op, r.cipher.KeyID())
	}
	if limit <= 0 {
		limit = repo.DefaultPageLimit
	}

	type staleRow struct{ id, ciphertext string }

	rows, err := r.q.Query(ctx, `
		select id::text, proxy_url_encrypted from egress_pool
		where encryption_key_id = $1
		order by created_at asc, id asc
		limit $2`, old.KeyID(), limit)
	if err != nil {
		return 0, repo.Err(op, err)
	}
	var stale []staleRow
	for rows.Next() {
		var s staleRow
		if err := rows.Scan(&s.id, &s.ciphertext); err != nil {
			rows.Close()
			return 0, repo.Err(op, err)
		}
		stale = append(stale, s)
	}
	rows.Close()
	if err := rows.Err(); err != nil {
		return 0, repo.Err(op, err)
	}

	rotated := 0
	for _, s := range stale {
		next, err := security.Reencrypt(old, r.cipher, s.ciphertext, security.CredentialAAD(s.id))
		if err != nil {
			return rotated, fmt.Errorf("%s (jalur keluar %s): %w", op, s.id, err)
		}
		tag, err := r.q.Exec(ctx, `
			update egress_pool set proxy_url_encrypted = $2, encryption_key_id = $3
			where id = $1 and encryption_key_id = $4`,
			s.id, next, r.cipher.KeyID(), old.KeyID())
		if err != nil {
			return rotated, repo.Err(op, err)
		}
		if tag.RowsAffected() == 1 {
			rotated++
		}
	}
	return rotated, nil
}
