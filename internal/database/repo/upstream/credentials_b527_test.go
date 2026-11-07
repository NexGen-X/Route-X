package upstream

import (
	"context"
	"sync/atomic"
	"testing"
	"time"

	"github.com/NexGen-X/Route-X/internal/security"
)

// TestB527CallbackBolehMenyentuhMutasiPelaporSama mengunci kontrak
// B-527: callback pergeseran pemenang TIDAK dipanggil sambil kunci
// per-provider terpegang, jadi callback yang di dalamnya menyentuh
// mutasi pelapor provider yang sama (yang mengambil kunci yang
// sama) menyelesaikan panggilannya — di kode lama rangkaian ini
// deadlock karena mutex Go tidak re-entrant.
//
// Jalur yang dilatih adalah mutasi repo SUNGGUHAN: MarkAuthFailure
// atas A menggeser pemenang ke B; callback (pertama kali) membalas
// dengan MarkAuthFailure atas B yang menggeser pemenang kembali
// ke A dan memicu callback kedua (dijaga agar tidak rekursif).
// Seluruh rangkaian berjalan di goroutine dengan batas waktu:
// deadlock tampil sebagai test gagal, bukan gantungan CI.
func TestB527CallbackBolehMenyentuhMutasiPelaporSama(t *testing.T) {
	if testing.Short() {
		t.Skip("butuh PostgreSQL")
	}
	ctx, cancel := context.WithTimeout(context.Background(), 90*time.Second)
	defer cancel()

	pool := newTestPool(ctx, t)
	cipher := testCipher(t, 0xC8)
	credentials, err := NewCredentialRepo(pool, cipher)
	if err != nil {
		t.Fatalf("NewCredentialRepo: %v", err)
	}
	provider := seedProvider(ctx, t, pool, "provider-b527")
	admin := seedUser(ctx, t, pool)
	kedaluwarsa := time.Now().Add(30 * 24 * time.Hour)

	credA, err := credentials.Create(ctx, CreateCredentialParams{
		ProviderID: provider.ID, Label: "a-b527",
		Secret:    security.Secret("akses-b527-a"),
		ExpiresAt: &kedaluwarsa, CreatedBy: &admin,
	})
	if err != nil {
		t.Fatalf("Create A: %v", err)
	}
	credB, err := credentials.Create(ctx, CreateCredentialParams{
		ProviderID: provider.ID, Label: "b-b527",
		Secret:    security.Secret("akses-b527-b"),
		ExpiresAt: &kedaluwarsa, CreatedBy: &admin,
	})
	if err != nil {
		t.Fatalf("Create B: %v", err)
	}
	// Seri auth_failures (0, 0); last_used_at A lebih dulu → A
	// pemenang. MarkAuthFailure(A) → A=1 > B=0 → B pemenang.
	// MarkAuthFailure(B) sesudahnya → B=1, seri, last_used_at A
	// lebih dulu → A pemenang lagi.
	if _, err := pool.Exec(ctx, `
		update provider_credentials set last_used_at = now() - interval '2 hours' where id = $1`, credA.ID); err != nil {
		t.Fatalf("menata last_used A: %v", err)
	}
	if _, err := pool.Exec(ctx, `
		update provider_credentials set last_used_at = now() - interval '1 hour' where id = $1`, credB.ID); err != nil {
		t.Fatalf("menata last_used B: %v", err)
	}

	var panggilan atomic.Int64
	var sudahMasuk atomic.Bool
	credentials.SetSesudahPemenangBergeser(func(cbCtx context.Context, providerID string) {
		panggilan.Add(1)
		if !sudahMasuk.Swap(true) {
			// Sentuhan kembali ke mutasi pelapor provider yang
			// sama DARI DALAM callback: hanya mungkin selesai
			// bila callback berjalan tanpa kunci terpegang.
			if _, err := credentials.MarkAuthFailure(cbCtx, credB.ID); err != nil {
				t.Errorf("MarkAuthFailure dari dalam callback: %v", err)
			}
		}
	})

	selesai := make(chan error, 1)
	go func() {
		_, err := credentials.MarkAuthFailure(ctx, credA.ID)
		selesai <- err
	}()
	select {
	case err := <-selesai:
		if err != nil {
			t.Fatalf("MarkAuthFailure A: %v", err)
		}
	case <-time.After(20 * time.Second):
		t.Fatal("rangkaian callback tidak selesai dalam 20 detik — callback masih dipanggil sambil kunci terpegang (deadlock B-527)")
	}

	if n := panggilan.Load(); n != 2 {
		t.Fatalf("callback terpanggil %d kali, mau 2 (geser A→B lalu B→A dari dalam callback)", n)
	}
	aktif, err := credentials.Active(ctx, provider.ID)
	if err != nil || aktif.ID != credA.ID {
		t.Fatalf("pemenang akhir bukan A (aktif=%v, err=%v)", aktif, err)
	}
}
