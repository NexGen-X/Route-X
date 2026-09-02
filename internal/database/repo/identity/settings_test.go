package identity

import (
	"encoding/json"
	"errors"
	"testing"

	"github.com/NexGen-X/Route-X/internal/database/repo"
)

// Jalur bahagia seluruh operasi setelan.
func TestSettingsIntegration(t *testing.T) {
	ctx, db := newTestDB(t)
	users := NewUsers(db.Pool)
	settings := NewSettings(db.Pool)
	admin := makeUser(ctx, t, users, "setelan@routex.test")

	const key = "ratelimit.default_rpm"

	created, err := settings.Put(ctx, SettingWrite{
		Key:         "  " + key + "  ",
		Value:       json.RawMessage(`{"limit":600,"burst":60}`),
		Description: "Batas laju bawaan",
		UpdatedBy:   admin.ID,
	})
	if err != nil {
		t.Fatalf("Put: %v", err)
	}

	t.Run("baris baru terisi sesuai masukan", func(t *testing.T) {
		if created.Key != key {
			t.Errorf("Key = %q, ingin sudah dipangkas menjadi %q", created.Key, key)
		}
		if created.Description != "Batas laju bawaan" {
			t.Errorf("Description = %q", created.Description)
		}
		if created.UpdatedBy != admin.ID {
			t.Errorf("UpdatedBy = %q, ingin %q", created.UpdatedBy, admin.ID)
		}
		if created.UpdatedAt.IsZero() {
			t.Error("updated_at kosong")
		}
	})

	t.Run("nilai bisa dibaca kembali sebagai JSON dan di-query PostgreSQL", func(t *testing.T) {
		var decoded struct {
			Limit int `json:"limit"`
			Burst int `json:"burst"`
		}
		if err := json.Unmarshal(created.Value, &decoded); err != nil {
			t.Fatalf("membaca nilai: %v", err)
		}
		if decoded.Limit != 600 || decoded.Burst != 60 {
			t.Errorf("nilai = %+v, ingin limit 600 burst 60", decoded)
		}

		// Tersimpan sebagai jsonb, bukan teks: bagiannya harus bisa di-query server.
		var limit int
		if err := db.Pool.QueryRow(ctx,
			`select (value->>'limit')::int from settings where key = $1`, key).Scan(&limit); err != nil {
			t.Fatalf("query bagian nilai: %v", err)
		}
		if limit != 600 {
			t.Errorf("value->>'limit' = %d, ingin 600", limit)
		}
	})

	t.Run("Get", func(t *testing.T) {
		got, err := settings.Get(ctx, key)
		if err != nil {
			t.Fatalf("Get: %v", err)
		}
		if got.Key != created.Key || string(got.Value) != string(created.Value) {
			t.Errorf("Get mengembalikan baris lain: %+v", got)
		}
		if _, err := settings.Get(ctx, "tidak.ada"); !errors.Is(err, repo.ErrNotFound) {
			t.Errorf("Get kunci yang tidak ada = %v, ingin ErrNotFound", err)
		}
	})

	t.Run("Put memperbarui nilai dan mempertahankan keterangan", func(t *testing.T) {
		updated, err := settings.Put(ctx, SettingWrite{
			Key:   key,
			Value: json.RawMessage(`{"limit":1200,"burst":120}`),
		})
		if err != nil {
			t.Fatalf("Put: %v", err)
		}
		if string(updated.Value) == string(created.Value) {
			t.Error("nilai tidak berubah")
		}
		if updated.Description != "Batas laju bawaan" {
			t.Errorf("Description = %q, ingin keterangan lama dipertahankan", updated.Description)
		}
		if !updated.UpdatedAt.After(created.UpdatedAt) {
			t.Error("updated_at tidak maju")
		}
		if updated.UpdatedBy != "" {
			t.Errorf("UpdatedBy = %q, ingin kosong untuk perubahan oleh sistem", updated.UpdatedBy)
		}

		withDesc, err := settings.Put(ctx, SettingWrite{
			Key:         key,
			Value:       json.RawMessage(`{"limit":1200,"burst":120}`),
			Description: "Keterangan baru",
			UpdatedBy:   admin.ID,
		})
		if err != nil {
			t.Fatalf("Put: %v", err)
		}
		if withDesc.Description != "Keterangan baru" || withDesc.UpdatedBy != admin.ID {
			t.Errorf("keterangan/pengubah tidak diperbarui: %+v", withDesc)
		}
	})

	t.Run("segala bentuk JSON yang sah diterima", func(t *testing.T) {
		values := map[string]string{
			"objek":     `{"a":{"b":[1,2,3]}}`,
			"array":     `[1,2,3]`,
			"angka":     `42`,
			"teks":      `"nyala"`,
			"boolean":   `true`,
			"null JSON": `null`,
		}
		for name, raw := range values {
			t.Run(name, func(t *testing.T) {
				got, err := settings.Put(ctx, SettingWrite{Key: "uji." + name, Value: json.RawMessage(raw)})
				if err != nil {
					t.Fatalf("Put(%s): %v", raw, err)
				}
				if !json.Valid(got.Value) {
					t.Errorf("nilai yang dibaca kembali bukan JSON sah: %s", got.Value)
				}
			})
		}
	})

	t.Run("nilai yang bukan JSON ditolak sebelum menyentuh database", func(t *testing.T) {
		for _, raw := range []string{``, `{`, `bukan json`, `{"a":}`} {
			_, err := settings.Put(ctx, SettingWrite{Key: "uji.rusak", Value: json.RawMessage(raw)})
			if !errors.Is(err, ErrInvalidInput) {
				t.Errorf("Put(%q) = %v, ingin ErrInvalidInput", raw, err)
			}
		}
		if _, err := settings.Get(ctx, "uji.rusak"); !errors.Is(err, repo.ErrNotFound) {
			t.Errorf("baris tertinggal dari penulisan yang ditolak: %v", err)
		}
	})

	t.Run("kunci kosong ditolak constraint tabel", func(t *testing.T) {
		_, err := settings.Put(ctx, SettingWrite{Key: "   ", Value: json.RawMessage(`1`)})
		if !errors.Is(err, repo.ErrConstraint) {
			t.Errorf("Put dengan kunci kosong = %v, ingin ErrConstraint", err)
		}
	})

	t.Run("pengubah yang bukan UUID ditolak", func(t *testing.T) {
		_, err := settings.Put(ctx, SettingWrite{
			Key:       "uji.pengubah",
			Value:     json.RawMessage(`1`),
			UpdatedBy: "bukan-uuid",
		})
		if !errors.Is(err, ErrInvalidInput) {
			t.Errorf("Put = %v, ingin ErrInvalidInput", err)
		}
	})

	t.Run("All terurut kunci", func(t *testing.T) {
		all, err := settings.All(ctx)
		if err != nil {
			t.Fatalf("All: %v", err)
		}
		if len(all) < 2 {
			t.Fatalf("jumlah setelan = %d, ingin lebih dari satu", len(all))
		}
		for i := 1; i < len(all); i++ {
			if all[i].Key < all[i-1].Key {
				t.Errorf("All tidak terurut: %q sebelum %q", all[i-1].Key, all[i].Key)
				break
			}
		}
	})

	t.Run("Delete", func(t *testing.T) {
		if err := settings.Delete(ctx, key); err != nil {
			t.Fatalf("Delete: %v", err)
		}
		if _, err := settings.Get(ctx, key); !errors.Is(err, repo.ErrNotFound) {
			t.Errorf("Get setelah Delete = %v, ingin ErrNotFound", err)
		}
		if err := settings.Delete(ctx, key); !errors.Is(err, repo.ErrNotFound) {
			t.Errorf("Delete kedua = %v, ingin ErrNotFound", err)
		}
	})
}
