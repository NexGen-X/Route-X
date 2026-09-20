package rotation

import (
	"context"
	"encoding/base64"
	"encoding/json"
	"errors"
	"fmt"
	"strings"

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

// Execute menjalankan proses rotasi kunci enkripsi pada seluruh penyimpanan:
// 1. provider_credentials
// 2. webhooks
// 3. egress_pool
// 4. settings cli:config:*
//
// Aturan Arsitektur & Keamanan:
//   - Wajib SATU transaksi tunggal yang melingkupi KETIGA tabel. security.Cipher hanya
//     mendukung satu kunci aktif (NewCipher). Jika transaksi dipisah per tabel dan kegagalan
//     terjadi di tabel ketiga (egress_pool), tabel 1 dan 2 sudah ter-commit dengan kunci baru
//     sementara tabel 3 masih kunci lama. Akibatnya aplikasi tidak akan pernah bisa mendekripsi
//     sebagian datanya sendiri dengan kunci mana pun. Dengan satu transaksi atomik, seluruh
//     tabel berhasil bersamaan atau dibatalkan bersamaan tanpa meninggalkan inkonsistensi.
//   - Kolom ciphertext dan encryption_key_id WAJIB diperbarui bersamaan dalam baris yang sama.
//     Jika diperbarui terpisah, baris dengan ciphertext baru tapi key ID lama tidak akan
//     pernah bisa didekripsi oleh siapa pun lagi.
//   - AAD (Additional Authenticated Data) untuk egress_pool WAJIB menggunakan
//     security.CredentialAAD(id) persis seperti yang digunakan di internal/database/repo/upstream/egress.go.
//   - Mode dry-run memverifikasi integritas dekripsi seluruh baris tanpa memodifikasi basis data.
func (r *Rotator) Execute(ctx context.Context) (*Result, error) {
	res := &Result{
		DryRun:   r.dryRun,
		OldKeyID: r.oldCipher.KeyID(),
		NewKeyID: r.newCipher.KeyID(),
		Tables:   make([]TableResult, 0, 4),
	}

	// Buka SATU transaksi basis data tunggal yang melingkupi KETIGA tabel.
	tx, err := r.db.Begin(ctx)
	if err != nil {
		return nil, fmt.Errorf("memulai transaksi basis data rotasi: %w", err)
	}
	defer tx.Rollback(ctx)

	specs := []tableSpec{
		{
			name:         "provider_credentials",
			idColumn:     "id",
			cipherColumn: "ciphertext",
			keyIdColumn:  "encryption_key_id",
			buildAAD:     func(id string) string { return security.CredentialAAD(id) },
		},
		{
			name:         "webhooks",
			idColumn:     "id",
			cipherColumn: "secret_ciphertext",
			keyIdColumn:  "encryption_key_id",
			buildAAD:     func(id string) string { return security.WebhookAAD(id) },
		},
		{
			name:         "egress_pool",
			idColumn:     "id",
			cipherColumn: "proxy_url_encrypted",
			keyIdColumn:  "encryption_key_id",
			// AAD egress_pool mengikuti kode aplikasi di internal/database/repo/upstream/egress.go
			buildAAD: func(id string) string { return security.CredentialAAD(id) },
		},
	}

	for _, spec := range specs {
		tRes, err := r.rotateTableInTx(ctx, tx, spec)
		if err != nil {
			return nil, fmt.Errorf("rotasi tabel %s: %w", spec.name, err)
		}
		res.Tables = append(res.Tables, tRes)
		res.TotalRotated += tRes.Rotated
	}

	settingsRes, err := r.rotateSettingsInTx(ctx, tx)
	if err != nil {
		return nil, fmt.Errorf("rotasi settings: %w", err)
	}
	res.Tables = append(res.Tables, settingsRes)
	res.TotalRotated += settingsRes.Rotated

	if r.dryRun {
		// Pada mode dry run, jangan pernah melakukan commit. Transaksi dibatalkan secara bersih.
		if err := tx.Rollback(ctx); err != nil && !errors.Is(err, pgx.ErrTxClosed) {
			return nil, fmt.Errorf("membatalkan transaksi dry-run: %w", err)
		}
		return res, nil
	}

	// Commit transaksi tunggal setelah seluruh baris pada ketiga tabel berhasil diperbarui tanpa error.
	if err := tx.Commit(ctx); err != nil {
		return nil, fmt.Errorf("commit transaksi rotasi ketiga tabel: %w", err)
	}

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

func (r *Rotator) rotateSettingsInTx(ctx context.Context, tx pgx.Tx) (TableResult, error) {
	result := TableResult{TableName: "settings.encrypted"}
	rows, err := tx.Query(ctx, `
		SELECT key, value
		FROM settings
		WHERE key LIKE 'cli:config:%'
		ORDER BY key ASC
		FOR UPDATE`)
	if err != nil {
		return result, fmt.Errorf("membaca settings terenkripsi: %w", err)
	}
	defer rows.Close()

	type settingRecord struct {
		key   string
		value []byte
	}
	var records []settingRecord
	for rows.Next() {
		var rec settingRecord
		if err := rows.Scan(&rec.key, &rec.value); err != nil {
			return result, fmt.Errorf("memindai setting terenkripsi: %w", err)
		}
		records = append(records, rec)
	}
	if err := rows.Err(); err != nil {
		return result, fmt.Errorf("iterasi settings terenkripsi: %w", err)
	}

	for _, rec := range records {
		var obj map[string]json.RawMessage
		if err := json.Unmarshal(rec.value, &obj); err != nil {
			return result, fmt.Errorf("setting %s bukan JSON sah: %w", rec.key, err)
		}

		field, aad := "", ""
		switch {
		case strings.HasPrefix(rec.key, "cli:config:"):
			toolID := strings.TrimPrefix(rec.key, "cli:config:")
			if toolID == "" {
				return result, fmt.Errorf("setting CLI tanpa ID tool")
			}
			field, aad = "api_key_enc", security.CLIToolAAD(toolID)
		}

		var ciphertext string
		if raw := obj[field]; len(raw) == 0 || json.Unmarshal(raw, &ciphertext) != nil || ciphertext == "" {
			continue
		}
		if !r.oldCipher.CanDecrypt(ciphertext) {
			if r.newCipher.CanDecrypt(ciphertext) {
				continue
			}
			return result, fmt.Errorf("setting %s memakai ciphertext tidak dikenal", rec.key)
		}

		result.Scanned++
		rotated, err := security.Reencrypt(r.oldCipher, r.newCipher, ciphertext, aad)
		if err != nil {
			return result, fmt.Errorf("mengenkripsi ulang setting %s: %w", rec.key, err)
		}
		obj[field], _ = json.Marshal(rotated)
		newValue, err := json.Marshal(obj)
		if err != nil {
			return result, fmt.Errorf("menyandikan ulang setting %s: %w", rec.key, err)
		}
		if !r.dryRun {
			tag, err := tx.Exec(ctx, `UPDATE settings SET value = $1, updated_at = now() WHERE key = $2`, newValue, rec.key)
			if err != nil {
				return result, fmt.Errorf("memperbarui setting %s: %w", rec.key, err)
			}
			if tag.RowsAffected() != 1 {
				return result, fmt.Errorf("konkurensi terdeteksi pada setting %s", rec.key)
			}
		}
		result.Rotated++
	}
	return result, nil
}

func (r *Rotator) rotateTableInTx(ctx context.Context, tx pgx.Tx, spec tableSpec) (TableResult, error) {
	result := TableResult{TableName: spec.name}

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

	return result, nil
}
