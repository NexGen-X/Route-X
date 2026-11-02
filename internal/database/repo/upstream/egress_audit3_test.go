package upstream

import (
	"context"
	"errors"
	"testing"
	"time"

	"github.com/NexGen-X/Route-X/internal/database/repo"
	"github.com/NexGen-X/Route-X/internal/security"
)

// TestEgressProxyURLMenghormatiEnabled membuktikan audit3 V-EO-01 tertutup:
// pool yang dinonaktifkan TIDAK lagi menyerahkan URL proxy-nya ke jalur
// request (dulu query ProxyURL tidak menyaring enabled sama sekali),
// sedangkan jalur diagnostik admin tetap bisa membacanya untuk tombol Tes.
func TestEgressProxyURLMenghormatiEnabled(t *testing.T) {
	if testing.Short() {
		t.Skip("butuh PostgreSQL")
	}
	ctx, cancel := context.WithTimeout(context.Background(), 90*time.Second)
	defer cancel()

	pool := newTestPool(ctx, t)
	cipher := testCipher(t, 0xE1)
	egress, err := NewEgressRepo(pool, cipher)
	if err != nil {
		t.Fatalf("NewEgressRepo: %v", err)
	}

	created, err := egress.Create(ctx, CreateEgressParams{
		Name:     "pool-audit3-g8",
		Kind:     EgressHTTP,
		ProxyURL: security.Secret(testProxyURL),
	})
	if err != nil {
		t.Fatalf("Create: %v", err)
	}

	// Aktif: URL bisa dibaca jalur request.
	if _, err := egress.ProxyURL(ctx, created.ID); err != nil {
		t.Fatalf("ProxyURL (aktif): %v", err)
	}

	nonaktif := false
	if _, err := egress.Update(ctx, created.ID, UpdateEgressParams{Enabled: &nonaktif}); err != nil {
		t.Fatalf("Update nonaktifkan: %v", err)
	}

	// Nonaktif: jalur request menolak seolah pool tidak ada.
	if _, err := egress.ProxyURL(ctx, created.ID); !errors.Is(err, repo.ErrNotFound) {
		t.Fatalf("ProxyURL (nonaktif) = %v, ingin repo.ErrNotFound", err)
	}

	// Diagnostik admin tetap bisa membaca pool nonaktif untuk tombol Tes.
	got, err := egress.ProxyURLDiagnostik(ctx, created.ID)
	if err != nil {
		t.Fatalf("ProxyURLDiagnostik (nonaktif): %v", err)
	}
	if got.Reveal() != testProxyURL {
		t.Error("URL diagnostik tidak sama dengan yang disimpan")
	}
}
