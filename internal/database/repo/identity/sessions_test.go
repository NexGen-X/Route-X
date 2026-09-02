package identity

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"testing"
	"time"

	"github.com/NexGen-X/Route-X/internal/database"
	"github.com/NexGen-X/Route-X/internal/database/repo"
	"github.com/NexGen-X/Route-X/internal/security"
)

// backdate memundurkan waktu sebuah sesi supaya keadaan yang biasanya butuh menunggu
// bisa diuji langsung.
//
// created_at ikut dimundurkan karena constraint sessions_expires_after_creation
// mewajibkan expires_at tetap lebih besar darinya — persis seperti keadaan sesi yang
// memang sudah lama dibuat.
func backdate(ctx context.Context, t *testing.T, db *database.DB, id string, age time.Duration) {
	t.Helper()
	_, err := db.Pool.Exec(ctx, `update sessions set
		created_at = now() - make_interval(secs => $2),
		expires_at = now() - make_interval(secs => $3)
	where id = $1`, id, age.Seconds()*2, age.Seconds())
	if err != nil {
		t.Fatalf("memundurkan waktu sesi: %v", err)
	}
}

// Jalur bahagia seluruh operasi sesi.
func TestSessionsIntegration(t *testing.T) {
	ctx, db := newTestDB(t)
	users := NewUsers(db.Pool)
	sessions := NewSessions(db.Pool)
	user := makeUser(ctx, t, users, "sesi@routex.test")

	created, err := sessions.Create(ctx, NewSession{
		UserID:    user.ID,
		TTL:       time.Hour,
		IP:        "203.0.113.9",
		UserAgent: "Mozilla/5.0 (uji)",
	})
	if err != nil {
		t.Fatalf("Create: %v", err)
	}

	t.Run("sesi baru terisi lengkap", func(t *testing.T) {
		if !validUUID(created.ID) {
			t.Errorf("ID = %q, ingin UUID", created.ID)
		}
		if created.UserID != user.ID {
			t.Errorf("UserID = %q, ingin %q", created.UserID, user.ID)
		}
		if created.IP != "203.0.113.9" || created.UserAgent != "Mozilla/5.0 (uji)" {
			t.Errorf("jejak klien tidak tersimpan: %+v", created.Session)
		}
		if created.RevokedAt != nil {
			t.Errorf("RevokedAt = %v, ingin kosong", created.RevokedAt)
		}
		if !created.ExpiresAt.After(created.CreatedAt) {
			t.Error("expires_at tidak lebih besar dari created_at")
		}
		if got := created.ExpiresAt.Sub(created.CreatedAt); got < 59*time.Minute || got > 61*time.Minute {
			t.Errorf("masa berlaku = %v, ingin sekitar satu jam", got)
		}
		if created.Token.IsZero() {
			t.Fatal("token mentah tidak dikembalikan")
		}
	})

	t.Run("Lookup menemukan sesi dengan token mentahnya", func(t *testing.T) {
		got, err := sessions.Lookup(ctx, created.Token)
		if err != nil {
			t.Fatalf("Lookup: %v", err)
		}
		if got.ID != created.ID {
			t.Errorf("Lookup mengembalikan sesi lain")
		}
	})

	t.Run("token yang tidak dikenal tidak menemukan apa pun", func(t *testing.T) {
		other, _, err := newSessionToken()
		if err != nil {
			t.Fatalf("newSessionToken: %v", err)
		}
		for _, token := range []security.Secret{other, "", "bukan-token"} {
			if _, err := sessions.Lookup(ctx, token); !errors.Is(err, repo.ErrNotFound) {
				t.Errorf("Lookup dengan token asing = %v, ingin ErrNotFound", err)
			}
		}
	})

	t.Run("Touch memajukan last_seen_at", func(t *testing.T) {
		if err := sessions.Touch(ctx, created.ID); err != nil {
			t.Fatalf("Touch: %v", err)
		}
		got, err := sessions.Lookup(ctx, created.Token)
		if err != nil {
			t.Fatalf("Lookup: %v", err)
		}
		if !got.LastSeenAt.After(created.LastSeenAt) {
			t.Errorf("last_seen_at = %v, ingin lebih baru dari %v", got.LastSeenAt, created.LastSeenAt)
		}
	})

	t.Run("ListActive", func(t *testing.T) {
		second, err := sessions.Create(ctx, NewSession{UserID: user.ID, TTL: time.Hour})
		if err != nil {
			t.Fatalf("Create: %v", err)
		}
		got, err := sessions.ListActive(ctx, user.ID)
		if err != nil {
			t.Fatalf("ListActive: %v", err)
		}
		if len(got) != 2 {
			t.Fatalf("jumlah sesi aktif = %d, ingin 2", len(got))
		}
		if err := sessions.Revoke(ctx, second.ID); err != nil {
			t.Fatalf("Revoke: %v", err)
		}
	})

	t.Run("Revoke aman dipanggil dua kali", func(t *testing.T) {
		extra, err := sessions.Create(ctx, NewSession{UserID: user.ID, TTL: time.Hour})
		if err != nil {
			t.Fatalf("Create: %v", err)
		}
		if err := sessions.Revoke(ctx, extra.ID); err != nil {
			t.Fatalf("Revoke: %v", err)
		}

		var firstRevocation time.Time
		if err := db.Pool.QueryRow(ctx, `select revoked_at from sessions where id = $1`, extra.ID).
			Scan(&firstRevocation); err != nil {
			t.Fatalf("membaca revoked_at: %v", err)
		}
		if err := sessions.Revoke(ctx, extra.ID); err != nil {
			t.Errorf("Revoke kedua = %v, ingin nil", err)
		}

		var secondRevocation time.Time
		if err := db.Pool.QueryRow(ctx, `select revoked_at from sessions where id = $1`, extra.ID).
			Scan(&secondRevocation); err != nil {
			t.Fatalf("membaca revoked_at: %v", err)
		}
		if !secondRevocation.Equal(firstRevocation) {
			t.Error("pencabutan kedua menimpa waktu pencabutan pertama")
		}
	})

	t.Run("sesi yang tidak ada", func(t *testing.T) {
		const missing = "8b1f4a2c-0d3e-4f5a-9b8c-7d6e5f4a3b2c"
		if err := sessions.Touch(ctx, missing); !errors.Is(err, repo.ErrNotFound) {
			t.Errorf("Touch = %v, ingin ErrNotFound", err)
		}
		if err := sessions.Revoke(ctx, missing); !errors.Is(err, repo.ErrNotFound) {
			t.Errorf("Revoke = %v, ingin ErrNotFound", err)
		}
		if err := sessions.Touch(ctx, "bukan-uuid"); !errors.Is(err, repo.ErrNotFound) {
			t.Errorf("Touch dengan ID bukan UUID = %v, ingin ErrNotFound", err)
		}
	})

	t.Run("pengguna yang tidak ada tidak bisa punya sesi", func(t *testing.T) {
		_, err := sessions.Create(ctx, NewSession{
			UserID: "8b1f4a2c-0d3e-4f5a-9b8c-7d6e5f4a3b2c",
			TTL:    time.Hour,
		})
		if !errors.Is(err, repo.ErrInvalidReference) {
			t.Errorf("Create = %v, ingin ErrInvalidReference", err)
		}
		if _, err := sessions.Create(ctx, NewSession{UserID: "bukan-uuid", TTL: time.Hour}); !errors.Is(err, repo.ErrInvalidReference) {
			t.Errorf("Create dengan ID bukan UUID = %v, ingin ErrInvalidReference", err)
		}
	})

	t.Run("RevokeAllOfUser", func(t *testing.T) {
		for range 2 {
			if _, err := sessions.Create(ctx, NewSession{UserID: user.ID, TTL: time.Hour}); err != nil {
				t.Fatalf("Create: %v", err)
			}
		}
		n, err := sessions.RevokeAllOfUser(ctx, user.ID)
		if err != nil {
			t.Fatalf("RevokeAllOfUser: %v", err)
		}
		if n != 3 {
			t.Errorf("jumlah sesi tercabut = %d, ingin 3 (satu sesi awal dan dua yang baru)", n)
		}

		active, err := sessions.ListActive(ctx, user.ID)
		if err != nil {
			t.Fatalf("ListActive: %v", err)
		}
		if len(active) != 0 {
			t.Errorf("masih ada %d sesi aktif setelah semuanya dicabut", len(active))
		}
		if _, err := sessions.Lookup(ctx, created.Token); !errors.Is(err, repo.ErrNotFound) {
			t.Errorf("Lookup setelah pencabutan menyeluruh = %v, ingin ErrNotFound", err)
		}

		// Tidak ada lagi yang bisa dicabut, jadi jumlahnya nol dan bukan error.
		if n, err := sessions.RevokeAllOfUser(ctx, user.ID); err != nil || n != 0 {
			t.Errorf("RevokeAllOfUser kedua = (%d, %v), ingin (0, nil)", n, err)
		}
	})

	t.Run("menghapus pengguna ikut menghapus sesinya", func(t *testing.T) {
		victim := makeUser(ctx, t, users, "hapus@routex.test")
		s, err := sessions.Create(ctx, NewSession{UserID: victim.ID, TTL: time.Hour})
		if err != nil {
			t.Fatalf("Create: %v", err)
		}
		if err := users.Delete(ctx, victim.ID); err != nil {
			t.Fatalf("Delete: %v", err)
		}
		if _, err := sessions.Lookup(ctx, s.Token); !errors.Is(err, repo.ErrNotFound) {
			t.Errorf("Lookup = %v, ingin ErrNotFound", err)
		}
	})
}

// Sesi yang kedaluwarsa atau sudah dicabut tidak boleh pernah muncul sebagai sesi yang
// berlaku — lewat pintu mana pun.
//
// Penyaringannya melekat di query, bukan di Go, justru supaya sifat ini tidak bergantung
// pada pemanggil yang ingat memeriksanya.
func TestSessionsExpiredAndRevokedAreNeverValidIntegration(t *testing.T) {
	ctx, db := newTestDB(t)
	users := NewUsers(db.Pool)
	sessions := NewSessions(db.Pool)
	user := makeUser(ctx, t, users, "kedaluwarsa@routex.test")

	expired, err := sessions.Create(ctx, NewSession{UserID: user.ID, TTL: time.Hour})
	if err != nil {
		t.Fatalf("Create: %v", err)
	}
	backdate(ctx, t, db, expired.ID, time.Hour)

	revoked, err := sessions.Create(ctx, NewSession{UserID: user.ID, TTL: time.Hour})
	if err != nil {
		t.Fatalf("Create: %v", err)
	}
	if err := sessions.Revoke(ctx, revoked.ID); err != nil {
		t.Fatalf("Revoke: %v", err)
	}

	live, err := sessions.Create(ctx, NewSession{UserID: user.ID, TTL: time.Hour})
	if err != nil {
		t.Fatalf("Create: %v", err)
	}

	for name, s := range map[string]CreatedSession{"kedaluwarsa": expired, "dicabut": revoked} {
		t.Run("Lookup menolak sesi "+name, func(t *testing.T) {
			if _, err := sessions.Lookup(ctx, s.Token); !errors.Is(err, repo.ErrNotFound) {
				t.Errorf("Lookup = %v, ingin ErrNotFound", err)
			}
		})
		t.Run("Touch menolak sesi "+name, func(t *testing.T) {
			if err := sessions.Touch(ctx, s.ID); !errors.Is(err, repo.ErrNotFound) {
				t.Errorf("Touch = %v, ingin ErrNotFound", err)
			}
		})
	}

	t.Run("ListActive hanya memuat sesi yang berlaku", func(t *testing.T) {
		got, err := sessions.ListActive(ctx, user.ID)
		if err != nil {
			t.Fatalf("ListActive: %v", err)
		}
		if len(got) != 1 || got[0].ID != live.ID {
			t.Errorf("sesi aktif = %d baris, ingin hanya sesi yang masih hidup", len(got))
		}
	})

	t.Run("DeleteExpired menghormati tenggang waktu", func(t *testing.T) {
		// Tenggang jauh lebih panjang dari umur sesi kedaluwarsa: belum ada yang dibuang.
		n, err := sessions.DeleteExpired(ctx, 24*time.Hour)
		if err != nil {
			t.Fatalf("DeleteExpired: %v", err)
		}
		if n != 0 {
			t.Errorf("terhapus %d baris, ingin 0 karena masih di dalam tenggang", n)
		}

		n, err = sessions.DeleteExpired(ctx, time.Minute)
		if err != nil {
			t.Fatalf("DeleteExpired: %v", err)
		}
		if n != 1 {
			t.Errorf("terhapus %d baris, ingin 1 (hanya yang kedaluwarsa melewati tenggang)", n)
		}

		// Sesi yang baru dicabut juga masih di dalam tenggang; setelah dimundurkan,
		// barulah ikut terbuang.
		if _, err := db.Pool.Exec(ctx,
			`update sessions set revoked_at = now() - interval '1 hour' where id = $1`, revoked.ID); err != nil {
			t.Fatalf("memundurkan revoked_at: %v", err)
		}
		n, err = sessions.DeleteExpired(ctx, time.Minute)
		if err != nil {
			t.Fatalf("DeleteExpired: %v", err)
		}
		if n != 1 {
			t.Errorf("terhapus %d baris, ingin 1 (sesi yang sudah lama dicabut)", n)
		}

		// Sesi yang masih hidup tidak boleh ikut terbuang.
		if _, err := sessions.Lookup(ctx, live.Token); err != nil {
			t.Errorf("sesi yang masih berlaku ikut terhapus: %v", err)
		}
	})
}

// Token mentah tidak boleh ada di database dalam bentuk apa pun: yang tersimpan hanya
// hash SHA-256-nya, sehingga dump database tidak bisa dipakai membajak sesi.
func TestSessionsRawTokenIsNeverStoredIntegration(t *testing.T) {
	ctx, db := newTestDB(t)
	users := NewUsers(db.Pool)
	sessions := NewSessions(db.Pool)
	user := makeUser(ctx, t, users, "token@routex.test")

	created, err := sessions.Create(ctx, NewSession{
		UserID:    user.ID,
		TTL:       time.Hour,
		IP:        "203.0.113.9",
		UserAgent: "Mozilla/5.0 (uji)",
	})
	if err != nil {
		t.Fatalf("Create: %v", err)
	}
	token := created.Token.Reveal()

	// Seluruh baris dicocokkan sebagai teks, bukan hanya kolom token_hash: kalau suatu
	// saat ada yang menyalin token ke kolom lain "sekadar untuk debugging", test ini
	// yang harus menangkapnya. strpos dipakai daripada LIKE karena token base64url
	// memuat "_", yang di LIKE justru berarti wildcard.
	var hits int
	if err := db.Pool.QueryRow(ctx,
		`select count(*) from sessions where strpos(sessions::text, $1) > 0`, token).Scan(&hits); err != nil {
		t.Fatalf("mencari token di tabel sessions: %v", err)
	}
	if hits != 0 {
		t.Errorf("token mentah ditemukan di %d baris tabel sessions", hits)
	}

	// Yang tersimpan harus benar-benar SHA-256 heksadesimal dari token itu.
	sum := sha256.Sum256([]byte(token))
	want := hex.EncodeToString(sum[:])

	var stored string
	if err := db.Pool.QueryRow(ctx, `select token_hash from sessions where id = $1`, created.ID).
		Scan(&stored); err != nil {
		t.Fatalf("membaca token_hash: %v", err)
	}
	if stored != want {
		t.Errorf("token_hash = %q, ingin SHA-256 dari token mentah", stored)
	}

	// Dan hash itu tidak boleh bisa dipakai sebagai token.
	if _, err := sessions.Lookup(ctx, security.Secret(stored)); !errors.Is(err, repo.ErrNotFound) {
		t.Errorf("Lookup dengan hash sebagai token = %v, ingin ErrNotFound", err)
	}
}
