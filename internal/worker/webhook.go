package worker

import (
	"context"
	"fmt"
	"log/slog"
	"os"
	"sync"
	"time"

	"github.com/jackc/pgx/v5/pgxpool"

	"github.com/NexGen-X/Route-X/internal/database/repo"
	"github.com/NexGen-X/Route-X/internal/webhooks"
)

// WebhookClaimer mendefinisikan operasi klaim antrean dan pemulihan sewa webhook.
type WebhookClaimer interface {
	ClaimDue(ctx context.Context, workerID string, limit int) ([]*webhooks.Delivery, error)
	RecoverStuck(ctx context.Context, stuckDuration time.Duration) (int64, error)
	RenewLeases(ctx context.Context, deliveryIDs []int64, workerID string) error
}

// WebhookDispatcher mendefinisikan pengiriman satu delivery webhook.
type WebhookDispatcher interface {
	Dispatch(ctx context.Context, delivery *webhooks.Delivery) error
}

// WebhookWorker memproses antrean pengiriman webhook dengan klaim FOR UPDATE SKIP LOCKED.
//
// Aturan 6 & 7:
//   - Mengambil antrean siap kirim (next_attempt_at <= now()) dengan FOR UPDATE SKIP LOCKED.
//     Ini memungkinkan beberapa worker berjalan paralel tanpa tabrakan atau saling tunggu.
//   - Menjalankan pemulihan sewa tertinggal (status 'delivering' > 5 menit) secara berkala.
//   - Backoff disimpan di next_attempt_at dan dihitung dengan equal jitter.
type WebhookWorker struct {
	pool          *pgxpool.Pool
	repo          WebhookClaimer
	dispatcher    WebhookDispatcher
	workerID      string
	batchSize     int
	stuckInterval time.Duration
	lastRecovery  time.Time
	// concurrency membatasi pengiriman serentak dalam satu batch.
	concurrency int
	// leaseRenewInterval adalah jeda pembaruan sewa batch yang sedang berjalan.
	leaseRenewInterval time.Duration
	logger             *slog.Logger
}

// NewWebhookWorker membuat instance baru WebhookWorker.
func NewWebhookWorker(
	pool *pgxpool.Pool,
	repo WebhookClaimer,
	dispatcher WebhookDispatcher,
	logger *slog.Logger,
) *WebhookWorker {
	if logger == nil {
		logger = slog.Default()
	}
	hostname, _ := os.Hostname()
	wid := fmt.Sprintf("worker-%s-%d", hostname, os.Getpid())

	return &WebhookWorker{
		pool:               pool,
		repo:               repo,
		dispatcher:         dispatcher,
		workerID:           wid,
		batchSize:          25,
		stuckInterval:      2 * time.Minute,
		concurrency:        5,
		leaseRenewInterval: 30 * time.Second,
		logger:             logger,
	}
}

func (w *WebhookWorker) Name() string { return "webhook_delivery_worker" }

// Run mengambil batch antrean dan memproses pengiriman satu per satu.
func (w *WebhookWorker) Run(ctx context.Context) error {
	now := time.Now()

	// 1. Jalankan pemulihan sewa tersangkut bila interval sudah lewat
	if now.Sub(w.lastRecovery) >= w.stuckInterval {
		w.runStuckRecovery(ctx)
		w.lastRecovery = now
	}

	// 2. Ambil batch delivery yang due dengan FOR UPDATE SKIP LOCKED
	deliveries, err := w.repo.ClaimDue(ctx, w.workerID, w.batchSize)
	if err != nil {
		return repo.Err("mengambil antrean webhook due", err)
	}

	if len(deliveries) == 0 {
		return nil
	}

	w.logger.DebugContext(ctx, "memproses batch webhook delivery", "jumlah", len(deliveries), "worker_id", w.workerID)

	// Jaga sewa batch selama batch berjalan (audit3 V-WH-12): ClaimDue mencap
	// locked_at seluruh batch di awal, jadi tanpa pembaruan berkala item ekor
	// yang masih antre di memori bisa melewati ambang stuck (5 menit) dan
	// dipulihkan — lalu dikirim ganda — oleh instance lain.
	ids := make([]int64, len(deliveries))
	for i, d := range deliveries {
		ids[i] = d.ID
	}
	stopJaga := w.jagaSewa(ctx, ids)
	defer stopJaga()

	// Dispatch PARALEL terbatas (audit3 V-WH-07): versi lama mengirim serial,
	// sehingga satu endpoint lambat menahan seluruh batch di belakangnya
	// (pada verifikasi live, delivery sehat tertahan tepat 20 detik menunggu
	// endpoint yang tidur 20 detik).
	sem := make(chan struct{}, w.concurrency)
	var wg sync.WaitGroup
	for _, d := range deliveries {
		select {
		case <-ctx.Done():
			wg.Wait()
			return ctx.Err()
		case sem <- struct{}{}:
		}
		wg.Add(1)
		go func(d *webhooks.Delivery) {
			defer wg.Done()
			defer func() { <-sem }()

			if err := w.dispatcher.Dispatch(ctx, d); err != nil {
				// Dispatch TIDAK pernah gagal diam-diam: setiap kegagalan sudah
				// ditulis ke webhook_deliveries (status + next_attempt_at) oleh
				// Dispatcher.RecordFailure sebelum error ini kembali. Yang tersisa
				// di sini hanyalah visibilitas log + metrik worker generik.
				w.logger.WarnContext(ctx, "gagal mengirim webhook delivery",
					"delivery_id", d.ID, "webhook_id", d.WebhookID, "event", d.Event,
					"attempt", d.AttemptCount+1, "error", err)
			}
		}(d)
	}
	wg.Wait()

	return nil
}

// jagaSewa memperbarui locked_at batch setiap leaseRenewInterval sampai
// fungsi berhenti yang dikembalikan dipanggil (batch selesai).
func (w *WebhookWorker) jagaSewa(ctx context.Context, ids []int64) func() {
	ctx, cancel := context.WithCancel(ctx)
	done := make(chan struct{})
	go func() {
		defer close(done)
		t := time.NewTicker(w.leaseRenewInterval)
		defer t.Stop()
		for {
			select {
			case <-ctx.Done():
				return
			case <-t.C:
				if err := w.repo.RenewLeases(ctx, ids, w.workerID); err != nil {
					w.logger.WarnContext(ctx, "gagal memperbarui sewa batch webhook", "error", err)
				}
			}
		}
	}()
	return func() {
		cancel()
		<-done
	}
}

// runStuckRecovery memulihkan pengiriman yang tersangkut dengan advisory lock.
func (w *WebhookWorker) runStuckRecovery(ctx context.Context) {
	unlock, ok, err := TryAdvisoryLock(ctx, w.pool, LockWebhookRecovery)
	if err != nil || !ok {
		return
	}
	defer unlock()

	recovered, err := w.repo.RecoverStuck(ctx, 5*time.Minute)
	if err != nil {
		w.logger.WarnContext(ctx, "pemulihan sewa webhook tersangkut gagal", "error", err)
		return
	}
	if recovered > 0 {
		w.logger.InfoContext(ctx, "berhasil memulihkan pengiriman webhook tersangkut", "jumlah", recovered)
	}
}
