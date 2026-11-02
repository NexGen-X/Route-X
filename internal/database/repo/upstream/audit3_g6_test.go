package upstream

import (
	"context"
	"testing"
	"time"

	"github.com/NexGen-X/Route-X/internal/security"
)

// Test regresi audit3 grup 6 (B-028) untuk lapisan repo: snapshot
// kesehatan basi tidak lagi menyisihkan provider, dan auth_failures
// tidak lagi mengalahkan prioritas kredensial.

// TestRouteCandidatesSnapshotKesehatanBasiTidakMenyisihkan memverifikasi
// V-BR-09: provider yang snapshot-nya 'unhealthy' tetapi sudah berumur
// berhari-hari (worker kesehatan berhenti) kembali menjadi kandidat,
// sementara snapshot 'unhealthy' yang segar tetap menyisihkannya.
func TestRouteCandidatesSnapshotKesehatanBasiTidakMenyisihkan(t *testing.T) {
	if testing.Short() {
		t.Skip("butuh PostgreSQL")
	}
	ctx, cancel := context.WithTimeout(context.Background(), 90*time.Second)
	defer cancel()

	pool := newTestPool(ctx, t)
	providers := NewProviderRepo(pool)
	model := seedModel(ctx, t, pool, "gpt-5", CapText)

	sehat := seedProvider(ctx, t, pool, "selalu-sehat")
	sakit := seedProvider(ctx, t, pool, "sakit-lalu-basi")
	seedAttachment(ctx, t, pool, model.ID, sehat.ID, nil)
	seedAttachment(ctx, t, pool, model.ID, sakit.ID, nil)

	adaSakit := func() bool {
		cands, err := providers.RouteCandidates(ctx, RouteQuery{ModelID: model.ID})
		if err != nil {
			t.Fatalf("RouteCandidates: %v", err)
		}
		for _, c := range cands {
			if c.ProviderID == sakit.ID {
				return true
			}
		}
		return false
	}

	// Snapshot unhealthy SEGAR: provider tersisih (perilaku yang benar).
	if err := providers.RecordHealth(ctx, sakit.ID, HealthReport{Status: HealthUnhealthy}); err != nil {
		t.Fatalf("RecordHealth: %v", err)
	}
	if adaSakit() {
		t.Fatal("provider unhealthy segar ikut menjadi kandidat, mau tersisih")
	}

	// Snapshot yang sama, dibekukan tiga hari: kabar lama, bukan keadaan —
	// provider harus kembali menjadi kandidat.
	if _, err := pool.Exec(ctx,
		`update providers set last_health_at = now() - interval '3 days' where id = $1`, sakit.ID); err != nil {
		t.Fatalf("membekukan snapshot: %v", err)
	}
	if !adaSakit() {
		t.Fatal("provider dengan snapshot unhealthy basi tetap tersisih, mau kembali menjadi kandidat")
	}
}

// TestCredentialActivePrioritasMengalahkanAuthFailures memverifikasi
// V-BR-13: pada strategi priority, satu kegagalan autentikasi historis
// tidak menurunkan kredensial prioritas utama di bawah cadangannya;
// auth_failures hanya memecah seri di antara prioritas yang sama.
func TestCredentialActivePrioritasMengalahkanAuthFailures(t *testing.T) {
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
	provider := seedProvider(ctx, t, pool, "provider-prioritas")
	if _, err := pool.Exec(ctx,
		`update providers set credential_strategy = 'priority' where id = $1`, provider.ID); err != nil {
		t.Fatalf("menyetel strategi priority: %v", err)
	}

	prio := func(n int) *int { return &n }
	utama, err := credentials.Create(ctx, CreateCredentialParams{
		ProviderID: provider.ID, Label: "utama", Secret: security.Secret("sk-utama-1111"), Priority: prio(1),
	})
	if err != nil {
		t.Fatalf("Create utama: %v", err)
	}
	if _, err := credentials.Create(ctx, CreateCredentialParams{
		ProviderID: provider.ID, Label: "cadangan", Secret: security.Secret("sk-cadangan-2222"), Priority: prio(2),
	}); err != nil {
		t.Fatalf("Create cadangan: %v", err)
	}

	aktif := func() string {
		t.Helper()
		cred, err := credentials.Active(ctx, provider.ID)
		if err != nil {
			t.Fatalf("Active: %v", err)
		}
		return cred.ID
	}

	if got := aktif(); got != utama.ID {
		t.Fatalf("Active awal = %s, mau kredensial utama %s", got, utama.ID)
	}

	// Satu kegagalan historis pada kredensial utama: prioritas tetap
	// memilihnya — cadangan tidak naik hanya karena penghitung kumulatif
	// milik utama bergerak.
	if _, err := credentials.MarkAuthFailure(ctx, utama.ID); err != nil {
		t.Fatalf("MarkAuthFailure: %v", err)
	}
	if got := aktif(); got != utama.ID {
		t.Fatalf("Active sesudah 1 kegagalan utama = %s, mau tetap utama %s", got, utama.ID)
	}

	// Di dalam prioritas yang sama, auth_failures tetap memecah seri:
	// saudara seprioritas tanpa kegagalan didahulukan.
	saudara, err := credentials.Create(ctx, CreateCredentialParams{
		ProviderID: provider.ID, Label: "saudara", Secret: security.Secret("sk-saudara-3333"), Priority: prio(1),
	})
	if err != nil {
		t.Fatalf("Create saudara: %v", err)
	}
	if got := aktif(); got != saudara.ID {
		t.Fatalf("Active di dalam prioritas sama = %s, mau saudara tanpa kegagalan %s", got, saudara.ID)
	}
}
