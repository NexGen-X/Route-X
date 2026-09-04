package rotation

import (
	"context"
	"encoding/base64"
	"errors"
	"fmt"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"

	"github.com/NexGen-X/Route-X/internal/security"
)

var (
	// ErrSameKeys dikembalikan bila kunci lama dan kunci baru bernilai identik.
	ErrSameKeys = errors.New("kunci lama dan kunci baru tidak boleh bernilai sama")
	// ErrInvalidKeyFormat dikembalikan saat kunci base64 tidak valid atau bukan 32 byte.
	ErrInvalidKeyFormat = errors.New("kunci enkripsi harus string base64 dengan panjang tepat 32 byte")
)

// TableResult merangkum statistik rotasi untuk satu tabel basis data.
type TableResult struct {
	TableName string `json:"table_name"`
	Scanned   int    `json:"scanned"`
	Rotated   int    `json:"rotated"`
}

// Result merangkum seluruh hasil eksekusi rotasi kunci enkripsi.
type Result struct {
	DryRun       bool          `json:"dry_run"`
	OldKeyID     string        `json:"old_key_id"`
	NewKeyID     string        `json:"new_key_id"`
	Tables       []TableResult `json:"tables"`
	TotalRotated int           `json:"total_rotated"`
}

// Config memuat parameter yang diperlukan untuk menjalankan rotasi ENCRYPTION_KEY.
type Config struct {
	DB     *pgxpool.Pool
	OldKey string // Base64 32 byte
	NewKey string // Base64 32 byte
	DryRun bool
}

// ParseKey memvalidasi dan mengurai string base64 menjadi slice byte 32 karakter.
// Validasi ini wajib dilakukan di awal sebelum menyentuh koneksi basis data apa pun
// guna mencegah kegagalan di tengah jalan saat transaksi sudah berjalan.
func ParseKey(raw string) ([]byte, error) {
	decoded, err := base64.StdEncoding.DecodeString(raw)
	if err != nil {
		// Coba RawURLEncoding atau RawStdEncoding jika pengguna memasukkan format URL base64
		var decErr error
		decoded, decErr = base64.RawURLEncoding.DecodeString(raw)
		if decErr != nil {
			return nil, fmt.Errorf("%w: gagal melakukan decode base64: %v", ErrInvalidKeyFormat, err)
		}
	}
	if len(decoded) != security.KeyLen {
		return nil, fmt.Errorf("%w: ukuran kunci %d byte, wajib %d byte", ErrInvalidKeyFormat, len(decoded), security.KeyLen)
	}
	return decoded, nil
}

// Rotator bertanggung jawab mengorkestrasi re-enkripsi kolom terenkripsi pada seluruh tabel.
type Rotator struct {
	db        *pgxpool.Pool
	oldCipher *security.Cipher
	newCipher *security.Cipher
	dryRun    bool
}

// NewRotator menginisialisasi instans Rotator dengan cipher lama dan cipher baru.
func NewRotator(cfg Config) (*Rotator, error) {
	if cfg.DB == nil {
		return nil, errors.New("koneksi basis data (db) tidak boleh bernilai nil")
	}
	if cfg.OldKey == "" || cfg.NewKey == "" {
		return nil, errors.New("kunci lama dan kunci baru wajib disediakan")
	}
	if cfg.OldKey == cfg.NewKey {
		return nil, ErrSameKeys
	}

	oldKeyBytes, err := ParseKey(cfg.OldKey)
	if err != nil {
		return nil, fmt.Errorf("kunci lama tidak valid: %w", err)
	}
	newKeyBytes, err := ParseKey(cfg.NewKey)
	if err != nil {
		return nil, fmt.Errorf("kunci baru tidak valid: %w", err)
	}

	oldCipher, err := security.NewCipher(oldKeyBytes)
	if err != nil {
		return nil, fmt.Errorf("membuat cipher kunci lama: %w", err)
	}
	newCipher, err := security.NewCipher(newKeyBytes)
	if err != nil {
		return nil, fmt.Errorf("membuat cipher kunci baru: %w", err)
	}

	// Pastikan Key ID kedua kunci tidak bertabrakan secara kebetulan
	if oldCipher.KeyID() == newCipher.KeyID() {
		return nil, ErrSameKeys
	}

	return &Rotator{
		db:        cfg.DB,
		oldCipher: oldCipher,
		newCipher: newCipher,
		dryRun:    cfg.DryRun,
	}, nil
}

// Execute menjalankan proses rotasi kunci enkripsi pada ketiga tabel:
// 1. provider_credentials
// 2. webhooks
// 3. egress_pool
//
// Aturan Arsitektur & Keamanan:
//   - Wajib SATU transaksi independen per tabel. Dengan satu transaksi per tabel,
//     kegagalan pada satu tabel tidak membatalkan kemajuan tabel sebelumnya yang sudah sukses,
//     tetapi kegagalan di tengah baris satu tabel akan mengembalikan tabel itu ke kondisi konsisten.
//   - Kolom ciphertext dan encryption_key_id WAJIB diperbarui bersamaan dalam baris yang sama.
//     Jika diperbarui terpisah, baris dengan ciphertext baru tapi key ID lama tidak akan
//     pernah bisa didekripsi oleh siapa pun lagi.
//   - AAD (Additional Authenticated Data) WAJIB tetap terikat pada ID unik masing-masing baris
//     agar tidak terjadi kerentanan ciphertext splicing / swapping.
//   - Mode dry-run memverifikasi integritas dekripsi seluruh baris tanpa memodifikasi basis data.
func (r *Rotator) Execute(ctx context.Context) (*Result, error) {
	res := &Result{
		DryRun:   r.dryRun,
		OldKeyID: r.oldCipher.KeyID(),
		NewKeyID: r.newCipher.KeyID(),
		Tables:   make([]TableResult, 0, 3),
	}

	// 1. Rotasi tabel provider_credentials
	t1, err := r.rotateTable(ctx, tableSpec{
		name:         "provider_credentials",
		idColumn:     "id",
		cipherColumn: "ciphertext",
		keyIdColumn:  "encryption_key_id",
		buildAAD:     func(id string) string { return security.CredentialAAD(id) },
	})
	if err != nil {
		return nil, fmt.Errorf("rotasi tabel provider_credentials: %w", err)
	}
	res.Tables = append(res.Tables, t1)
	res.TotalRotated += t1.Rotated

	// 2. Rotasi tabel webhooks
	t2, err := r.rotateTable(ctx, tableSpec{
		name:         "webhooks",
		idColumn:     "id",
		cipherColumn: "secret_ciphertext",
		keyIdColumn:  "encryption_key_id",
		buildAAD:     func(id string) string { return security.WebhookAAD(id) },
	})
	if err != nil {
		return nil, fmt.Errorf("rotasi tabel webhooks: %w", err)
	}
	res.Tables = append(res.Tables, t2)
	res.TotalRotated += t2.Rotated

	// 3. Rotasi tabel egress_pool
	t3, err := r.rotateTable(ctx, tableSpec{
		name:         "egress_pool",
		idColumn:     "id",
		cipherColumn: "proxy_url_encrypted",
		keyIdColumn:  "encryption_key_id",
		buildAAD:     func(id string) string { return security.EgressAAD(id) },
	})
	if err != nil {
		return nil, fmt.Errorf("rotasi tabel egress_pool: %w", err)
	}
	res.Tables = append(res.Tables, t3)
	res.TotalRotated += t3.Rotated

	return res, nil
}

type tableSpec struct {
	name         string
	idColumn     string
	cipherColumn string
	keyIdColumn  string
	buildAAD     func(id string) string
}

type rowRecord struct {
	id         string
	ciphertext string
}

func (r *Rotator) rotateTable(ctx context.Context, spec tableSpec) (TableResult, error) {
	result := TableResult{TableName: spec.name}

	// Buka transaksi tunggal khusus untuk tabel ini.
	tx, err := r.db.Begin(ctx)
	if err != nil {
		return result, fmt.Errorf("memulai transaksi basis data untuk tabel %s: %w", spec.name, err)
	}
	defer tx.Rollback(ctx) // Nol risiko jika tx.Commit() telah dipanggil dengan sukses sebelumnya

	// Ambil seluruh baris yang masih terenkripsi dengan kunci lama (OldKeyID).
	// Baris yang sudah memiliki NewKeyID secara otomatis diabaikan, menjaga sifat idempoten.
	query := fmt.Sprintf(
		`SELECT %s::text, %s FROM %s WHERE %s = $1 ORDER BY %s ASC FOR UPDATE`,
		spec.idColumn, spec.cipherColumn, spec.name, spec.keyIdColumn, spec.idColumn,
	)

	rows, err := tx.Query(ctx, query, r.oldCipher.KeyID())
	if err != nil {
		return result, fmt.Errorf("membaca baris lama pada tabel %s: %w", spec.name, err)
	}

	var records []rowRecord
	for rows.Next() {
		var rec rowRecord
		if err := rows.Scan(&rec.id, &rec.ciphertext); err != nil {
			rows.Close()
			return result, fmt.Errorf("memindai baris tabel %s: %w", spec.name, err)
		}
		records = append(records, rec)
	}
	rows.Close()
	if err := rows.Err(); err != nil {
		return result, fmt.Errorf("iterasi baris tabel %s: %w", spec.name, err)
	}

	result.Scanned = len(records)
	if result.Scanned == 0 {
		// Tidak ada baris yang menggunakan kunci lama; tabel sudah mutakhir atau kosong.
		return result, nil
	}

	// Proses setiap baris: dekripsi dengan oldCipher, verifikasi, dan enkripsi ulang dengan newCipher.
	updateQuery := fmt.Sprintf(
		`UPDATE %s SET %s = $1, %s = $2 WHERE %s = $3::uuid AND %s = $4`,
		spec.name, spec.cipherColumn, spec.keyIdColumn, spec.idColumn, spec.keyIdColumn,
	)

	for _, rec := range records {
		aad := spec.buildAAD(rec.id)

		// Re-enkripsi dengan pembersihan memori seketika (lihat security.Reencrypt).
		newCiphertext, err := security.Reencrypt(r.oldCipher, r.newCipher, rec.ciphertext, aad)
		if err != nil {
			return result, fmt.Errorf("gagal mengenkripsi ulang baris %s pada tabel %s (AAD: %s): %w", rec.id, spec.name, aad, err)
		}

		if !r.dryRun {
			tag, err := tx.Exec(ctx, updateQuery, newCiphertext, r.newCipher.KeyID(), rec.id, r.oldCipher.KeyID())
			if err != nil {
				return result, fmt.Errorf("memperbarui baris %s pada tabel %s: %w", rec.id, spec.name, err)
			}
			if tag.RowsAffected() != 1 {
				return result, fmt.Errorf("konkurensi terdeteksi: baris %s pada tabel %s tidak terpengaruh update", rec.id, spec.name)
			}
		}
		result.Rotated++
	}

	if r.dryRun {
		// Pada mode dry run, jangan pernah melakukan commit. Transaksi dibatalkan secara bersih.
		if err := tx.Rollback(ctx); err != nil && !errors.Is(err, pgx.ErrTxClosed) {
			return result, fmt.Errorf("membatalkan transaksi dry-run: %w", err)
		}
		return result, nil
	}

	// Commit transaksi tabel setelah seluruh baris berhasil diperbarui tanpa error.
	if err := tx.Commit(ctx); err != nil {
		return result, fmt.Errorf("menyimpan transaksi tabel %s ke basis data: %w", spec.name, err)
	}

	return result, nil
}
