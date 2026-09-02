package upstream

import (
	"context"
	"errors"
	"strings"
	"testing"
	"time"

	"github.com/NexGen-X/Route-X/internal/database/repo"
	"github.com/NexGen-X/Route-X/internal/security"
)

// rahasiaTest sengaja ganjil supaya kemunculannya di isi database tidak mungkin kebetulan.
const testSecret = "sk-proj-rahasia-xyzzy-9a21"

// rowsAsText mengembalikan seluruh baris sebuah tabel sebagai teks JSON, satu per baris.
//
// to_jsonb dipakai supaya SEMUA kolom ikut terbaca tanpa perlu menyebutkannya satu per
// satu: kalau nanti ada kolom baru yang menyimpan plaintext, test kebocoran di bawah ikut
// menangkapnya tanpa perlu diubah.
func rowsAsText(ctx context.Context, t *testing.T, q repo.Querier, table string) []string {
	t.Helper()

	rows, err := q.Query(ctx, `select to_jsonb(x)::text from `+table+` x`)
	if err != nil {
		t.Fatalf("membaca isi %s: %v", table, err)
	}
	defer rows.Close()

	var out []string
	for rows.Next() {
		var line string
		if err := rows.Scan(&line); err != nil {
			t.Fatalf("membaca baris %s: %v", table, err)
		}
		out = append(out, line)
	}
	if err := rows.Err(); err != nil {
		t.Fatalf("membaca isi %s: %v", table, err)
	}
	return out
}

// Kredensial disimpan terenkripsi: plaintext tidak boleh ada di kolom mana pun, dan
// ciphertext-nya terikat ke baris tempat ia disimpan.
func TestCredentialEncryptionIntegration(t *testing.T) {
	if testing.Short() {
		t.Skip("butuh PostgreSQL")
	}
	ctx, cancel := context.WithTimeout(context.Background(), 90*time.Second)
	defer cancel()

	pool := newTestPool(ctx, t)
	cipher := testCipher(t, 0xA1)
	credentials, err := NewCredentialRepo(pool, cipher)
	if err != nil {
		t.Fatalf("NewCredentialRepo: %v", err)
	}
	provider := seedProvider(ctx, t, pool, "provider-kredensial")
	admin := seedUser(ctx, t, pool)

	meta, err := credentials.Create(ctx, CreateCredentialParams{
		ProviderID: provider.ID,
		Secret:     security.Secret(testSecret),
		ExpiresAt:  ptr(time.Now().Add(30 * 24 * time.Hour)),
		CreatedBy:  &admin,
	})
	if err != nil {
		t.Fatalf("Create: %v", err)
	}

	switch {
	case meta.Label != DefaultCredentialLabel:
		t.Errorf("label = %q, ingin %q", meta.Label, DefaultCredentialLabel)
	case meta.MaskedHint != "sk-****9a21":
		t.Errorf("masked_hint = %q, ingin %q", meta.MaskedHint, "sk-****9a21")
	case meta.EncryptionKeyID != cipher.KeyID():
		t.Errorf("encryption_key_id = %q, ingin %q", meta.EncryptionKeyID, cipher.KeyID())
	case !meta.Enabled:
		t.Error("kredensial baru seharusnya aktif")
	case meta.AuthFailures != 0:
		t.Errorf("auth_failures = %d, ingin 0", meta.AuthFailures)
	case meta.LastUsedAt != nil:
		t.Errorf("last_used_at = %v, ingin nil", meta.LastUsedAt)
	}

	t.Run("plaintext tidak ada di kolom mana pun", func(t *testing.T) {
		for _, row := range rowsAsText(ctx, t, pool, "provider_credentials") {
			if strings.Contains(row, testSecret) {
				t.Fatal("baris provider_credentials memuat plaintext kredensial")
			}
			// Bagian rahasia setelah awalan juga tidak boleh muncul, termasuk lewat cuplikan.
			if strings.Contains(row, "rahasia-xyzzy") {
				t.Fatal("baris provider_credentials memuat potongan rahasia kredensial")
			}
		}
	})

	t.Run("ciphertext sesuai format yang dijaga skema", func(t *testing.T) {
		var ciphertext string
		err := pool.QueryRow(ctx,
			`select ciphertext from provider_credentials where id = $1`, meta.ID).Scan(&ciphertext)
		if err != nil {
			t.Fatalf("membaca ciphertext: %v", err)
		}
		parts := strings.Split(ciphertext, ".")
		if len(parts) != 3 || parts[0] != "v1" || parts[1] != cipher.KeyID() {
			t.Errorf("ciphertext = %q, ingin berbentuk v1.<key_id>.<payload>", ciphertext)
		}
	})

	t.Run("bisa didekripsi kembali dengan nilai yang sama", func(t *testing.T) {
		secret, err := credentials.Reveal(ctx, meta.ID)
		if err != nil {
			t.Fatalf("Reveal: %v", err)
		}
		if secret.Reveal() != testSecret {
			t.Error("nilai hasil dekripsi tidak sama dengan yang disimpan")
		}
		// Secret tidak boleh bocor lewat pencetakan biasa.
		if strings.Contains(secret.String(), "rahasia") {
			t.Error("Secret tercetak apa adanya")
		}
	})

	t.Run("gagal didekripsi bila AAD-nya diganti", func(t *testing.T) {
		var ciphertext string
		err := pool.QueryRow(ctx,
			`select ciphertext from provider_credentials where id = $1`, meta.ID).Scan(&ciphertext)
		if err != nil {
			t.Fatalf("membaca ciphertext: %v", err)
		}

		other, err := newID()
		if err != nil {
			t.Fatalf("newID: %v", err)
		}
		if _, err := cipher.Decrypt(ciphertext, security.CredentialAAD(other)); !errors.Is(err, security.ErrDecryptionFailed) {
			t.Errorf("Decrypt dengan AAD lain = %v, ingin ErrDecryptionFailed", err)
		}
	})

	t.Run("ciphertext yang dipindahkan ke baris lain tidak bisa dibuka", func(t *testing.T) {
		// Inilah serangan yang dicegah AAD: seseorang dengan akses tulis ke database
		// memindahkan blob kredensial ke baris provider lain, supaya gateway memakai
		// kredensial itu untuk upstream yang salah.
		other := seedProvider(ctx, t, pool, "provider-penampung")
		victim, err := credentials.Create(ctx, CreateCredentialParams{
			ProviderID: other.ID,
			Secret:     security.Secret("sk-lain-0000"),
		})
		if err != nil {
			t.Fatalf("Create kredensial kedua: %v", err)
		}

		if _, err := pool.Exec(ctx, `
			update provider_credentials
			set ciphertext = (select ciphertext from provider_credentials where id = $1)
			where id = $2`, meta.ID, victim.ID); err != nil {
			t.Fatalf("memindahkan ciphertext: %v", err)
		}

		if _, err := credentials.Reveal(ctx, victim.ID); !errors.Is(err, security.ErrDecryptionFailed) {
			t.Errorf("Reveal baris yang ciphertext-nya dipindahkan = %v, ingin ErrDecryptionFailed", err)
		}
	})
}

// Rotasi kunci: kredensial berkunci lama dipindahkan ke kunci baru dan tetap terbaca.
func TestCredentialKeyRotationIntegration(t *testing.T) {
	if testing.Short() {
		t.Skip("butuh PostgreSQL")
	}
	ctx, cancel := context.WithTimeout(context.Background(), 90*time.Second)
	defer cancel()

	pool := newTestPool(ctx, t)
	oldCipher := testCipher(t, 0xA1)
	newCipher := testCipher(t, 0xB2)

	oldRepo, err := NewCredentialRepo(pool, oldCipher)
	if err != nil {
		t.Fatalf("NewCredentialRepo: %v", err)
	}
	newRepo, err := NewCredentialRepo(pool, newCipher)
	if err != nil {
		t.Fatalf("NewCredentialRepo: %v", err)
	}

	provider := seedProvider(ctx, t, pool, "provider-rotasi")
	meta, err := oldRepo.Create(ctx, CreateCredentialParams{
		ProviderID: provider.ID,
		Secret:     security.Secret(testSecret),
	})
	if err != nil {
		t.Fatalf("Create dengan kunci lama: %v", err)
	}

	t.Run("kunci baru belum bisa membaca baris lama", func(t *testing.T) {
		if _, err := newRepo.Reveal(ctx, meta.ID); !errors.Is(err, security.ErrKeyMismatch) {
			t.Errorf("Reveal dengan kunci baru = %v, ingin ErrKeyMismatch", err)
		}
		counts, err := newRepo.CountByKeyID(ctx)
		if err != nil {
			t.Fatalf("CountByKeyID: %v", err)
		}
		if counts[oldCipher.KeyID()] != 1 || counts[newCipher.KeyID()] != 0 {
			t.Errorf("sebaran kunci = %v, ingin satu baris pada kunci lama", counts)
		}
	})

	t.Run("rotasi memindahkan baris dan nilainya tetap sama", func(t *testing.T) {
		rotated, err := newRepo.Reencrypt(ctx, oldCipher, 10)
		if err != nil {
			t.Fatalf("Reencrypt: %v", err)
		}
		if rotated != 1 {
			t.Fatalf("baris berpindah = %d, ingin 1", rotated)
		}

		secret, err := newRepo.Reveal(ctx, meta.ID)
		if err != nil {
			t.Fatalf("Reveal setelah rotasi: %v", err)
		}
		if secret.Reveal() != testSecret {
			t.Error("nilai setelah rotasi tidak sama dengan sebelumnya")
		}

		after, err := newRepo.Get(ctx, meta.ID)
		if err != nil {
			t.Fatalf("Get: %v", err)
		}
		if after.EncryptionKeyID != newCipher.KeyID() {
			t.Errorf("encryption_key_id = %q, ingin %q", after.EncryptionKeyID, newCipher.KeyID())
		}
	})

	t.Run("rotasi kedua tidak menemukan pekerjaan", func(t *testing.T) {
		rotated, err := newRepo.Reencrypt(ctx, oldCipher, 10)
		if err != nil {
			t.Fatalf("Reencrypt: %v", err)
		}
		if rotated != 0 {
			t.Errorf("baris berpindah = %d, ingin 0", rotated)
		}
	})

	t.Run("kunci lama yang sama dengan kunci aktif ditolak", func(t *testing.T) {
		if _, err := newRepo.Reencrypt(ctx, newCipher, 10); err == nil {
			t.Error("Reencrypt dengan kunci yang sama seharusnya gagal")
		}
	})

	t.Run("plaintext tetap tidak ada di database setelah rotasi", func(t *testing.T) {
		for _, row := range rowsAsText(ctx, t, pool, "provider_credentials") {
			if strings.Contains(row, testSecret) {
				t.Fatal("baris provider_credentials memuat plaintext setelah rotasi")
			}
		}
	})
}

// Pemilihan kredensial aktif, penandaan pemakaian, dan siklus hidup lainnya.
func TestCredentialLifecycleIntegration(t *testing.T) {
	if testing.Short() {
		t.Skip("butuh PostgreSQL")
	}
	ctx, cancel := context.WithTimeout(context.Background(), 90*time.Second)
	defer cancel()

	pool := newTestPool(ctx, t)
	cipher := testCipher(t, 0xC3)
	credentials, err := NewCredentialRepo(pool, cipher)
	if err != nil {
		t.Fatalf("NewCredentialRepo: %v", err)
	}
	provider := seedProvider(ctx, t, pool, "provider-siklus")

	create := func(label, secret string) *CredentialMeta {
		t.Helper()
		meta, err := credentials.Create(ctx, CreateCredentialParams{
			ProviderID: provider.ID,
			Label:      label,
			Secret:     security.Secret(secret),
		})
		if err != nil {
			t.Fatalf("Create %s: %v", label, err)
		}
		return meta
	}

	first := create("pertama", "sk-pertama-1111")
	second := create("kedua", "sk-kedua-2222")
	third := create("ketiga", "sk-ketiga-3333")

	t.Run("label ganda pada satu provider menjadi ErrConflict", func(t *testing.T) {
		_, err := credentials.Create(ctx, CreateCredentialParams{
			ProviderID: provider.ID, Label: "pertama", Secret: security.Secret("sk-kembar"),
		})
		if !errors.Is(err, repo.ErrConflict) {
			t.Fatalf("Create label ganda = %v, ingin ErrConflict", err)
		}
	})

	t.Run("provider yang tidak ada menjadi ErrInvalidReference", func(t *testing.T) {
		ghostID, err := newID()
		if err != nil {
			t.Fatalf("newID: %v", err)
		}
		_, err = credentials.Create(ctx, CreateCredentialParams{
			ProviderID: ghostID, Secret: security.Secret("sk-hantu"),
		})
		if !errors.Is(err, repo.ErrInvalidReference) {
			t.Errorf("Create pada provider hantu = %v, ingin ErrInvalidReference", err)
		}
	})

	t.Run("kredensial kosong ditolak sebelum menyentuh database", func(t *testing.T) {
		_, err := credentials.Create(ctx, CreateCredentialParams{ProviderID: provider.ID})
		if !errors.Is(err, repo.ErrConstraint) {
			t.Errorf("Create tanpa rahasia = %v, ingin ErrConstraint", err)
		}
	})

	t.Run("daftar metadata tidak memuat rahasia", func(t *testing.T) {
		list, err := credentials.List(ctx, provider.ID)
		if err != nil {
			t.Fatalf("List: %v", err)
		}
		if len(list) != 3 {
			t.Fatalf("jumlah kredensial = %d, ingin 3", len(list))
		}
		for _, meta := range list {
			if !strings.HasPrefix(meta.MaskedHint, "sk-") || !strings.Contains(meta.MaskedHint, "****") {
				t.Errorf("masked_hint = %q, ingin bentuk tersamar", meta.MaskedHint)
			}
		}
	})

	t.Run("yang belum pernah dipakai dipilih lebih dulu", func(t *testing.T) {
		// Semua masih nol kegagalan dan belum pernah dipakai, jadi yang terlama dibuat menang.
		active, err := credentials.Active(ctx, provider.ID)
		if err != nil {
			t.Fatalf("Active: %v", err)
		}
		if active.ID != first.ID {
			t.Fatalf("kredensial terpilih = %s, ingin %s", active.Label, first.Label)
		}
		if active.Secret.Reveal() != "sk-pertama-1111" {
			t.Error("rahasia hasil dekripsi tidak sesuai")
		}
	})

	t.Run("yang paling lama tidak dipakai dipilih berikutnya", func(t *testing.T) {
		if err := credentials.MarkUsed(ctx, first.ID); err != nil {
			t.Fatalf("MarkUsed pertama: %v", err)
		}
		active, err := credentials.Active(ctx, provider.ID)
		if err != nil {
			t.Fatalf("Active: %v", err)
		}
		if active.ID != second.ID {
			t.Errorf("kredensial terpilih = %s, ingin %s", active.Label, second.Label)
		}

		meta, err := credentials.Get(ctx, first.ID)
		if err != nil {
			t.Fatalf("Get: %v", err)
		}
		if meta.LastUsedAt == nil {
			t.Error("last_used_at tidak terisi setelah MarkUsed")
		}
	})

	t.Run("yang gagal autentikasi dihindari selama masih ada yang lain", func(t *testing.T) {
		if err := credentials.MarkUsed(ctx, second.ID); err != nil {
			t.Fatalf("MarkUsed kedua: %v", err)
		}
		failures, err := credentials.MarkAuthFailure(ctx, third.ID)
		if err != nil {
			t.Fatalf("MarkAuthFailure: %v", err)
		}
		if failures != 1 {
			t.Errorf("auth_failures = %d, ingin 1", failures)
		}

		// pertama dan kedua sudah dipakai (tanpa kegagalan), ketiga punya satu kegagalan,
		// jadi ketiga tetap yang terakhir dipilih.
		active, err := credentials.Active(ctx, provider.ID)
		if err != nil {
			t.Fatalf("Active: %v", err)
		}
		if active.ID == third.ID {
			t.Error("kredensial yang gagal autentikasi terpilih padahal ada yang sehat")
		}
	})

	t.Run("kredensial mati dan kedaluwarsa tidak pernah dipilih", func(t *testing.T) {
		for _, id := range []string{first.ID, second.ID} {
			if _, err := credentials.SetEnabled(ctx, id, false); err != nil {
				t.Fatalf("SetEnabled: %v", err)
			}
		}
		active, err := credentials.Active(ctx, provider.ID)
		if err != nil {
			t.Fatalf("Active: %v", err)
		}
		if active.ID != third.ID {
			t.Fatalf("kredensial terpilih = %s, ingin %s", active.Label, third.Label)
		}

		if _, err := pool.Exec(ctx,
			`update provider_credentials set expires_at = now() - interval '1 hour' where id = $1`,
			third.ID); err != nil {
			t.Fatalf("menua-kan kredensial: %v", err)
		}
		if _, err := credentials.Active(ctx, provider.ID); !errors.Is(err, repo.ErrNotFound) {
			t.Errorf("Active tanpa kredensial layak = %v, ingin ErrNotFound", err)
		}
	})

	t.Run("hapus lalu tidak ditemukan", func(t *testing.T) {
		if err := credentials.Delete(ctx, third.ID); err != nil {
			t.Fatalf("Delete: %v", err)
		}
		if err := credentials.Delete(ctx, third.ID); !errors.Is(err, repo.ErrNotFound) {
			t.Errorf("Delete kedua = %v, ingin ErrNotFound", err)
		}
		if err := credentials.MarkUsed(ctx, third.ID); !errors.Is(err, repo.ErrNotFound) {
			t.Errorf("MarkUsed pada baris terhapus = %v, ingin ErrNotFound", err)
		}
	})

	t.Run("kredensial ikut terhapus bersama providernya", func(t *testing.T) {
		if err := NewProviderRepo(pool).Delete(ctx, provider.ID); err != nil {
			t.Fatalf("Delete provider: %v", err)
		}
		list, err := credentials.List(ctx, provider.ID)
		if err != nil {
			t.Fatalf("List: %v", err)
		}
		if len(list) != 0 {
			t.Errorf("masih ada %d kredensial setelah providernya dihapus", len(list))
		}
	})
}
