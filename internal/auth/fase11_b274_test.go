package auth

import (
	"context"
	"testing"

	"github.com/NexGen-X/Route-X/internal/database/seed"
	"github.com/NexGen-X/Route-X/internal/security"
)

// FASE 11 B-274 (PB-29): ChangePassword menolkan penghitung kegagalan di DB
// (di dalam transaksinya) tetapi tidak pernah melepas ember email limiter
// Redis — padahal Login melepasnya saat berhasil (ResetEmail). Sisa
// penghitung lama mengotori jendela berjalan dan bisa menolak percobaan sah
// berikutnya. Test ini menanam penghitung email lewat jalur Allow limiter
// yang sama, mengganti password, lalu membuktikan ember itu sudah dilepas:
// Allow berikutnya menghitung dari awal (di bawah batas kecil yang dipasang).

func TestFase11B274ChangePasswordMelepasEmberEmail(t *testing.T) {
	limiter, _ := newLimiter(t, WithLoginRateLimits(0, 3)) // batas email: 3 per jendela
	e := newEnv(t, WithLoginLimiter(limiter))
	user := e.makeUser(t, "b274@example.test", seed.RoleAdmin)
	ctx := context.Background()
	const ip = "198.51.100.77"

	// Tanam dua hit pada ember email lewat jalur Allow yang asli.
	for i := 0; i < 2; i++ {
		if ok, _ := limiter.Allow(ctx, ip, user.Email); !ok {
			t.Fatalf("Allow ke-%d ditolak sebelum waktunya", i+1)
		}
	}
	// Ember terisi sebagian (2/3) — sisa kegagalan lama yang dimaksud
	// B-274. ChangePassword sendiri mengonsumsi satu hit Allow (3/3,
	// masih lolos), berhasil, dan — sesudah fix — melepas embernya.

	baru := security.Secret("GANTI_password_baru_b274!")
	if _, err := e.svc.ChangePassword(ctx, user.ID, security.Secret(testPassword), baru, "", ip, "uji-b274"); err != nil {
		t.Fatalf("ChangePassword: %v", err)
	}

	// Sesudah ganti password berhasil, ember email harus sudah dilepas:
	// Allow dari IP lain lolos lagi (menghitung dari awal). Tanpa fix,
	// ember tetap 3/3 dan Allow ini ditolak.
	if ok, _ := limiter.Allow(ctx, "203.0.113.9", user.Email); !ok {
		t.Fatal("ember email belum dilepas sesudah ChangePassword berhasil (B-274)")
	}
}
