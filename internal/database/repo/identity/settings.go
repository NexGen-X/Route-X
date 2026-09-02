package identity

import (
	"context"
	"encoding/json"
	"fmt"
	"strings"
	"time"

	"github.com/jackc/pgx/v5"

	"github.com/NexGen-X/Route-X/internal/database/repo"
)

// Setting adalah satu baris tabel settings: konfigurasi runtime yang boleh diubah
// operator dari dashboard tanpa restart.
//
// Nilainya jsonb dan dibawa sebagai json.RawMessage, tidak diurai di sini. Setiap
// kunci punya bentuk sendiri — angka, sakelar, objek kebijakan — dan yang tahu bentuk
// mana untuk kunci mana adalah pemakainya, bukan lapisan penyimpanan.
//
// Rahasia tidak boleh disimpan di sini: nilainya terbaca siapa pun yang punya akses
// baca database maupun panel admin.
type Setting struct {
	Key         string          `json:"key"`
	Value       json.RawMessage `json:"value"`
	Description string          `json:"description,omitempty"`
	UpdatedAt   time.Time       `json:"updated_at"`
	// UpdatedBy kosong berarti nilai yang ditanam sistem, atau akun pengubahnya sudah
	// tidak ada.
	UpdatedBy string `json:"updated_by,omitempty"`
}

// SettingWrite adalah masukan penulisan setelan.
type SettingWrite struct {
	Key   string
	Value json.RawMessage
	// Description kosong berarti keterangan yang sudah tersimpan dipertahankan, bukan
	// dihapus: keterangan biasanya ditulis sekali saat kunci diperkenalkan, sementara
	// nilainya berubah berkali-kali sesudahnya.
	Description string
	// UpdatedBy kosong berarti perubahan oleh sistem, bukan oleh seorang admin.
	UpdatedBy string
}

// Settings adalah repository tabel settings.
//
// Tempatnya di paket ini karena kolom updated_by menunjuk ke pengguna, dan setiap
// perubahan setelan berpasangan dengan satu catatan audit — keduanya lazim ditulis
// dalam satu transaksi bersama repository di sini.
type Settings struct {
	q repo.Querier
}

// NewSettings membuat repository setelan di atas q.
func NewSettings(q repo.Querier) *Settings { return &Settings{q: q} }

// settingColumns adalah kolom yang dibaca setiap query setelan.
const settingColumns = `key, value, coalesce(description, ''), updated_at, coalesce(updated_by::text, '')`

// scanSetting membaca satu baris setelan.
func scanSetting(row pgx.Row) (Setting, error) {
	var (
		s     Setting
		value []byte
	)
	if err := row.Scan(&s.Key, &value, &s.Description, &s.UpdatedAt, &s.UpdatedBy); err != nil {
		return Setting{}, err
	}
	s.Value = json.RawMessage(value)
	return s, nil
}

// Get mengambil satu setelan.
func (r *Settings) Get(ctx context.Context, key string) (Setting, error) {
	const op = "mengambil setelan"

	const query = `select ` + settingColumns + ` from settings where key = $1`
	s, err := scanSetting(r.q.QueryRow(ctx, query, strings.TrimSpace(key)))
	if err != nil {
		return Setting{}, repo.Err(op, err)
	}
	return s, nil
}

// All mengambil seluruh setelan, terurut kunci.
//
// Tanpa paginasi dengan sengaja: isi tabel ini adalah konfigurasi aplikasi yang
// jumlahnya puluhan, dan pemanggilnya justru ingin semuanya sekaligus untuk mengisi
// cache di sisi aplikasi.
func (r *Settings) All(ctx context.Context) ([]Setting, error) {
	const op = "mengambil seluruh setelan"

	rows, err := r.q.Query(ctx, `select `+settingColumns+` from settings order by key`)
	if err != nil {
		return nil, repo.Err(op, err)
	}
	defer rows.Close()

	var out []Setting
	for rows.Next() {
		s, err := scanSetting(rows)
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

// Put menulis setelan, membuat barisnya bila belum ada.
//
// Satu pernyataan INSERT ... ON CONFLICT, bukan "cek dulu lalu tulis": dua admin yang
// menyimpan kunci yang sama bersamaan tidak boleh membuat salah satunya gagal dengan
// pelanggaran keunikan.
//
// Nilai divalidasi sebagai JSON di sini. Tanpa itu, nilai yang salah bentuk hanya
// tertolak PostgreSQL sebagai error sintaks tanpa sentinel, sehingga permintaan yang
// sebenarnya salah akan tampak sebagai kegagalan server.
func (r *Settings) Put(ctx context.Context, in SettingWrite) (Setting, error) {
	const op = "menulis setelan"

	if len(in.Value) == 0 || !json.Valid(in.Value) {
		return Setting{}, fmt.Errorf("%s: %w: nilai bukan JSON yang sah", op, ErrInvalidInput)
	}
	if in.UpdatedBy != "" && !validUUID(in.UpdatedBy) {
		return Setting{}, fmt.Errorf("%s: %w: ID pengubah bukan UUID", op, ErrInvalidInput)
	}

	const query = `insert into settings (key, value, description, updated_at, updated_by)
	values ($1, $2, $3, now(), $4)
	on conflict (key) do update set
		value = excluded.value,
		description = coalesce(excluded.description, settings.description),
		updated_at = now(),
		updated_by = excluded.updated_by
	returning ` + settingColumns

	s, err := scanSetting(r.q.QueryRow(ctx, query,
		strings.TrimSpace(in.Key), []byte(in.Value),
		textParam(strings.TrimSpace(in.Description)), textParam(in.UpdatedBy)))
	if err != nil {
		return Setting{}, repo.Err(op, err)
	}
	return s, nil
}

// Delete menghapus setelan, sehingga nilai bawaan di kode berlaku kembali.
func (r *Settings) Delete(ctx context.Context, key string) error {
	const op = "menghapus setelan"

	tag, err := r.q.Exec(ctx, `delete from settings where key = $1`, strings.TrimSpace(key))
	if err != nil {
		return repo.Err(op, err)
	}
	if tag.RowsAffected() == 0 {
		return fmt.Errorf("%s: %w", op, repo.ErrNotFound)
	}
	return nil
}
