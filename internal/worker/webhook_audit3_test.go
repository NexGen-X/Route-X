package worker

import (
	"context"
	"sync"
	"testing"
	"time"

	"github.com/NexGen-X/Route-X/internal/webhooks"
)

// klaimPalsu adalah WebhookClaimer tiruan: batch tetap, tanpa database.
type klaimPalsu struct {
	batch []*webhooks.Delivery

	mu        sync.Mutex
	renewIDs  []int64
	renewCall int
}

func (k *klaimPalsu) ClaimDue(_ context.Context, _ string, _ int) ([]*webhooks.Delivery, error) {
	return k.batch, nil
}

func (k *klaimPalsu) RecoverStuck(_ context.Context, _ time.Duration) (int64, error) {
	return 0, nil
}

func (k *klaimPalsu) RenewLeases(_ context.Context, ids []int64, _ string) error {
	k.mu.Lock()
	defer k.mu.Unlock()
	k.renewCall++
	k.renewIDs = append([]int64(nil), ids...)
	return nil
}

// kirimPalsu adalah WebhookDispatcher tiruan yang tidur per pengiriman dan
// mencatat tingkat keserentakan maksimum yang tercapai.
type kirimPalsu struct {
	tidur time.Duration

	mu        sync.Mutex
	serentak  int
	puncak    int
	selesaiID map[int64]bool
}

func (k *kirimPalsu) Dispatch(_ context.Context, d *webhooks.Delivery) error {
	k.mu.Lock()
	k.serentak++
	if k.serentak > k.puncak {
		k.puncak = k.serentak
	}
	k.mu.Unlock()

	time.Sleep(k.tidur)

	k.mu.Lock()
	k.serentak--
	if k.selesaiID == nil {
		k.selesaiID = map[int64]bool{}
	}
	k.selesaiID[d.ID] = true
	k.mu.Unlock()
	return nil
}

func batchUji(n int) []*webhooks.Delivery {
	out := make([]*webhooks.Delivery, n)
	for i := range out {
		out[i] = &webhooks.Delivery{ID: int64(i + 1), WebhookID: "wh-uji", Event: "provider.unhealthy"}
	}
	return out
}

func pekerjaUji(repo WebhookClaimer, disp WebhookDispatcher) *WebhookWorker {
	w := NewWebhookWorker(nil, repo, disp, nil)
	// Lewati pemulihan sewa pada Run pertama supaya test tidak menyentuh pool.
	w.lastRecovery = time.Now()
	return w
}

// TestWebhookWorkerDispatchParalel membuktikan V-WH-07 tertutup: batch tidak
// lagi dikirim serial — endpoint lambat tidak menahan delivery sehat.
func TestWebhookWorkerDispatchParalel(t *testing.T) {
	const n = 6
	kirim := &kirimPalsu{tidur: 300 * time.Millisecond}
	w := pekerjaUji(&klaimPalsu{batch: batchUji(n)}, kirim)

	mulai := time.Now()
	if err := w.Run(context.Background()); err != nil {
		t.Fatalf("Run: %v", err)
	}
	durasi := time.Since(mulai)

	// Serial murni akan memakan 6×300ms = 1,8 detik; paralel terbatas (5)
	// menyelesaikannya dalam ±2 gelombang ≈ 600ms.
	if durasi > 1200*time.Millisecond {
		t.Fatalf("batch %d delivery × 300ms selesai dalam %v — masih terlalu serial", n, durasi)
	}
	if kirim.puncak < 2 {
		t.Fatalf("puncak keserentakan = %d, mau minimal 2", kirim.puncak)
	}
	if len(kirim.selesaiID) != n {
		t.Fatalf("delivery selesai = %d, mau %d", len(kirim.selesaiID), n)
	}
}

// TestWebhookWorkerMenjagaSewaBatch membuktikan V-WH-12 tertutup: selama
// batch berjalan, sewa (locked_at) batch diperbarui berkala oleh worker
// pemiliknya sehingga item ekor tidak dianggap tersangkut instance lain.
func TestWebhookWorkerMenjagaSewaBatch(t *testing.T) {
	klaim := &klaimPalsu{batch: batchUji(3)}
	kirim := &kirimPalsu{tidur: 150 * time.Millisecond}
	w := pekerjaUji(klaim, kirim)
	w.concurrency = 1 // paksa antre di memori: kondisi ekor batch
	w.leaseRenewInterval = 25 * time.Millisecond

	if err := w.Run(context.Background()); err != nil {
		t.Fatalf("Run: %v", err)
	}

	klaim.mu.Lock()
	defer klaim.mu.Unlock()
	if klaim.renewCall == 0 {
		t.Fatal("RenewLeases tidak pernah dipanggil selama batch berjalan")
	}
	if len(klaim.renewIDs) != 3 {
		t.Fatalf("RenewLeases memperbarui %d ID, mau seluruh batch (3)", len(klaim.renewIDs))
	}
}
