package upstream

import (
	"context"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"github.com/NexGen-X/Route-X/internal/security"
)

// TestB514LaporanPergeseranAtomik adalah pengunci B-514. PB8-22
// menemukan secara baca-kode bahwa penilaian pergeseran pemenang di
// UpdateSecret/MarkAuthFailure tidak atomik (baca-sebelum dan
// baca-sesudah mengapit mutasi tanpa kunci); probe Fase 21 atas kode
// sebelum perbaikan MEMBUKTIKANNYA dinamis: 18 dari 400 putaran
// balapan menunjukkan pemenang bergeser tanpa satu pun laporan.
// Perbaikannya: rangkaian penilaian dikunci per provider lewat
// mulaiPergeseran/kunciPergeseranPerProvider.
//
// Test ini menjalankan skenario probe yang sama dan menuntut NOL
// kebocoran: tiap putaran mulai dari pemenang A (auth_failures kedua
// kredensial 0, last_used_at NULL); dua goroutine dilepas bersamaan
// dari penghalang — MarkAuthFailure(A) (sendirian menggeser A→B) dan
// UpdateSecret(A) (sendirian mengembalikan ke A). Pengamat ketat
// mencicipi Active() selama putaran. Putaran BOCOR bila pengamat
// melihat B sebagai pemenang tetapi laporan putaran itu nol: pemenang
// terbukti berubah di tengah dan tak satu pun mutasi melaporkannya.
func TestB514LaporanPergeseranAtomik(t *testing.T) {
	if testing.Short() {
		t.Skip("butuh PostgreSQL")
	}
	ctx, cancel := context.WithTimeout(context.Background(), 180*time.Second)
	defer cancel()

	pool := newTestPool(ctx, t)
	cipher := testCipher(t, 0xB5)
	credentials, err := NewCredentialRepo(pool, cipher)
	if err != nil {
		t.Fatalf("NewCredentialRepo: %v", err)
	}
	provider := seedProvider(ctx, t, pool, "provider-b514")
	admin := seedUser(ctx, t, pool)
	kedaluwarsa := time.Now().Add(30 * 24 * time.Hour)

	buat := func(label, rahasia string) *CredentialMeta {
		t.Helper()
		meta, err := credentials.Create(ctx, CreateCredentialParams{
			ProviderID: provider.ID,
			Label:      label,
			Secret:     security.Secret(rahasia), ExpiresAt: &kedaluwarsa,
			CreatedBy: &admin,
		})
		if err != nil {
			t.Fatalf("Create %s: %v", label, err)
		}
		return meta
	}
	credA := buat("a-b514", "sk-b514-aa01")
	credB := buat("b-b514", "sk-b514-bb02")

	var totalLaporan atomic.Int64
	credentials.SetSesudahPemenangBergeser(func(_ context.Context, _ string) {
		totalLaporan.Add(1)
	})

	const putaran = 400
	bocor := 0
	for i := 0; i < putaran; i++ {
		if _, err := pool.Exec(ctx, `
			update provider_credentials
			set auth_failures = 0, last_used_at = null
			where provider_id = $1`, provider.ID); err != nil {
			t.Fatalf("reset putaran %d: %v", i, err)
		}
		aktif, err := credentials.Active(ctx, provider.ID)
		if err != nil || aktif == nil || aktif.ID != credA.ID {
			t.Fatalf("putaran %d: pemenang awal bukan A (aktif=%v, err=%v)", i, aktif, err)
		}

		var lihatB atomic.Bool
		stop := make(chan struct{})
		pengamatSelesai := make(chan struct{})
		go func() {
			defer close(pengamatSelesai)
			for {
				select {
				case <-stop:
					return
				default:
				}
				a, err := credentials.Active(ctx, provider.ID)
				if err == nil && a != nil && a.ID == credB.ID {
					lihatB.Store(true)
				}
			}
		}()

		laporanSebelum := totalLaporan.Load()
		mulai := make(chan struct{})
		var wg sync.WaitGroup
		wg.Add(2)
		go func() {
			defer wg.Done()
			<-mulai
			_, _ = credentials.MarkAuthFailure(ctx, credA.ID)
		}()
		go func() {
			defer wg.Done()
			<-mulai
			_ = credentials.UpdateSecret(ctx, credA.ID,
				security.Secret("sk-b514-aa02"), &kedaluwarsa)
		}()
		close(mulai)
		wg.Wait()
		close(stop)
		<-pengamatSelesai

		laporanPutaran := totalLaporan.Load() - laporanSebelum
		if lihatB.Load() && laporanPutaran == 0 {
			bocor++
		}
	}
	if bocor != 0 {
		t.Fatalf("B-514 bocor: %d dari %d putaran menunjukkan pemenang bergeser tanpa laporan", bocor, putaran)
	}
}
