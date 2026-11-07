package upstream

import (
	"context"
	"testing"
	"time"

	"github.com/NexGen-X/Route-X/internal/security"
)

// TestB474PergeseranPemenangMelapor adalah pengunci B-474: mutasi
// kredensial di jalur NON-admin (MarkAuthFailure dari request path,
// UpdateSecret dari rantai refresh) yang menggeser kredensial
// pemenang Active() wajib memanggil callback pergeseran — yang di
// produksi terpasang ke kenaikan generasi response cache — sementara
// mutasi yang tidak menggeser pemenang dan rotasi pemakaian normal
// (MarkUsed) tidak memanggilnya.
//
// Urutan skenario deterministik pada strategi round_robin (kunci
// urut: auth_failures, lalu last_used_at NULLS FIRST, lalu
// created_at): A dibuat lebih dulu sehingga A pemenang awal.
func TestB474PergeseranPemenangMelapor(t *testing.T) {
	if testing.Short() {
		t.Skip("butuh PostgreSQL")
	}
	ctx, cancel := context.WithTimeout(context.Background(), 90*time.Second)
	defer cancel()

	pool := newTestPool(ctx, t)
	cipher := testCipher(t, 0xB4)
	credentials, err := NewCredentialRepo(pool, cipher)
	if err != nil {
		t.Fatalf("NewCredentialRepo: %v", err)
	}
	provider := seedProvider(ctx, t, pool, "provider-b474")
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
	credA := buat("a-b474", "sk-b474-aaaa00000000000000000000000001")
	credB := buat("b-b474", "sk-b474-bbbb00000000000000000000000002")

	var laporan []string
	credentials.SetSesudahPemenangBergeser(func(_ context.Context, providerID string) {
		laporan = append(laporan, providerID)
	})

	pemenang := func() string {
		t.Helper()
		aktif, err := credentials.Active(ctx, provider.ID)
		if err != nil {
			t.Fatalf("Active: %v", err)
		}
		return aktif.ID
	}
	if got := pemenang(); got != credA.ID {
		t.Fatalf("pemenang awal = %s, mau A (%s)", got, credA.ID)
	}

	// MarkUsed pada pemenang memang menggeser pemenang (LRU), tetapi
	// SENGAJA tidak dilaporkan: rotasi pemakaian normal bukan
	// perubahan keadaan kredensial (lihat komentar MarkUsed).
	if err := credentials.MarkUsed(ctx, credA.ID); err != nil {
		t.Fatalf("MarkUsed: %v", err)
	}
	if got := pemenang(); got != credB.ID {
		t.Fatalf("pemenang sesudah MarkUsed(A) = %s, mau B (%s)", got, credB.ID)
	}
	if len(laporan) != 0 {
		t.Fatalf("MarkUsed memicu %d laporan pergeseran, mau 0", len(laporan))
	}

	// Satu kegagalan auth pada pemenang B: auth_failures memimpin
	// kunci urut, jadi A (0) menyalip B (1) — WAJIB dilaporkan.
	if _, err := credentials.MarkAuthFailure(ctx, credB.ID); err != nil {
		t.Fatalf("MarkAuthFailure B: %v", err)
	}
	if got := pemenang(); got != credA.ID {
		t.Fatalf("pemenang sesudah MarkAuthFailure(B) = %s, mau A (%s)", got, credA.ID)
	}
	if len(laporan) != 1 || laporan[0] != provider.ID {
		t.Fatalf("laporan sesudah MarkAuthFailure penggeser = %v, mau [%s]", laporan, provider.ID)
	}

	// Kegagalan kedua pada B yang sudah kalah: pemenang tidak
	// bergerak, tidak ada laporan baru.
	if _, err := credentials.MarkAuthFailure(ctx, credB.ID); err != nil {
		t.Fatalf("MarkAuthFailure B #2: %v", err)
	}
	if got := pemenang(); got != credA.ID {
		t.Fatalf("pemenang sesudah MarkAuthFailure(B) #2 = %s, mau A", got)
	}
	if len(laporan) != 1 {
		t.Fatalf("MarkAuthFailure tanpa pergeseran memicu laporan: total %d, mau 1", len(laporan))
	}

	// Refresh rahasia B (jalur SimpanHasilRefresh) mereset
	// auth_failures B ke 0; seri dipecah last_used_at: B belum
	// pernah dipakai (NULL lebih dulu) menyalip A — WAJIB dilaporkan.
	if err := credentials.UpdateSecret(ctx, credB.ID,
		security.Secret("sk-b474-bbbb11111111111111111111111103"), &kedaluwarsa); err != nil {
		t.Fatalf("UpdateSecret B: %v", err)
	}
	if got := pemenang(); got != credB.ID {
		t.Fatalf("pemenang sesudah UpdateSecret(B) = %s, mau B (%s)", got, credB.ID)
	}
	if len(laporan) != 2 {
		t.Fatalf("laporan sesudah UpdateSecret penggeser: total %d, mau 2", len(laporan))
	}

	// Salinan transaksional (WithQuerier — dipakai SimpanHasilTukar)
	// mewarisi callback: kegagalan auth pada B lewat salinan itu
	// menggeser pemenang kembali ke A dan WAJIB dilaporkan juga.
	if _, err := credentials.WithQuerier(pool).MarkAuthFailure(ctx, credB.ID); err != nil {
		t.Fatalf("MarkAuthFailure B via WithQuerier: %v", err)
	}
	if got := pemenang(); got != credA.ID {
		t.Fatalf("pemenang sesudah MarkAuthFailure via salinan = %s, mau A", got)
	}
	if len(laporan) != 3 {
		t.Fatalf("callback tidak diwarisi WithQuerier: total laporan %d, mau 3", len(laporan))
	}
}
