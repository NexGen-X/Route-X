package worker

import (
	"context"
	"fmt"
	"log/slog"
	"sync"
	"time"

	"github.com/jackc/pgx/v5/pgxpool"

	"github.com/NexGen-X/Route-X/internal/observability"
)

// Namespace advisory lock PostgreSQL untuk background worker.
//
// Menggunakan prefiks 0x5258574F ("RXWO" dalam ASCII) pada 32-bit teratas.
// Nilai ini dijamin TIDAK PERNAH bertabrakan dengan kunci migrasi database
// 0x524F55544558 ("ROUTEX" dalam ASCII) di internal/database/migrate.go.
//
// Bertabrakan dengan kunci migrasi berarti worker bisa menggagalkan migrasi saat deploy,
// atau migrasi yang sedang berjalan mematikan fungsi rollup dan retensi.
const (
	lockWorkerPrefix    int64 = 0x5258574F00000000
	LockRollup          int64 = lockWorkerPrefix | 0x01
	LockRetention       int64 = lockWorkerPrefix | 0x02
	LockPartition       int64 = lockWorkerPrefix | 0x03
	LockBudget          int64 = lockWorkerPrefix | 0x04
	LockHealth          int64 = lockWorkerPrefix | 0x05
	LockWebhookRecovery int64 = lockWorkerPrefix | 0x06
)

// Outcome string untuk label metrik routex_worker_runs_total.
const (
	OutcomeSuccess = "success"
	OutcomeError   = "error"
	OutcomePanic   = "panic"
	OutcomeSkipped = "skipped"
)

// TryAdvisoryLock mencoba mendapatkan advisory lock PostgreSQL tingkat sesi.
//
// Menggunakan koneksi mandiri yang dipinjam dari pool (pool.Acquire). Bila berhasil, fungsi
// unlock melepaskan kuncinya dan mengembalikan koneksinya ke pool. Bila proses worker mati
// mendadak atau koneksi terputus, PostgreSQL secara otomatis melepaskan kunci sesi ini.
func TryAdvisoryLock(ctx context.Context, pool *pgxpool.Pool, lockKey int64) (func(), bool, error) {
	if pool == nil {
		return func() {}, true, nil
	}

	conn, err := pool.Acquire(ctx)
	if err != nil {
		return nil, false, fmt.Errorf("mengambil koneksi untuk advisory lock: %w", err)
	}

	var acquired bool
	err = conn.QueryRow(ctx, "select pg_try_advisory_lock($1)", lockKey).Scan(&acquired)
	if err != nil {
		conn.Release()
		return nil, false, fmt.Errorf("mengeksekusi pg_try_advisory_lock: %w", err)
	}

	if !acquired {
		conn.Release()
		return nil, false, nil
	}

	unlock := func() {
		defer conn.Release()
		uCtx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
		defer cancel()
		_, _ = conn.Exec(uCtx, "select pg_advisory_unlock($1)", lockKey)
	}

	return unlock, true, nil
}

// Job merepresentasikan satu unit tugas berkala yang dijalankan oleh Supervisor.
type Job interface {
	Name() string
	Run(ctx context.Context) error
}

// JobFunc pembungkus fungsi biasa menjadi implementasi Job.
type JobFunc struct {
	JobName string
	Fn      func(ctx context.Context) error
}

func (j JobFunc) Name() string                  { return j.JobName }
func (j JobFunc) Run(ctx context.Context) error { return j.Fn(ctx) }

// ScheduledJob membungkus Job dengan interval eksekusinya.
type ScheduledJob struct {
	Job      Job
	Interval time.Duration
	// InitialDelay memberi jeda sebelum putaran pertama dimulai (misal agar start server tuntas).
	InitialDelay time.Duration
}

// Supervisor mengelola siklus hidup seluruh background worker goroutine.
//
// Menyediakan isolasi kegagalan mutlak:
// 1. Panic di satu job ditangkap (recover), dicatat ke log, dan dihitung di metrik routex_worker_runs_total{outcome="panic"}.
// 2. Error di satu job TIDAK BOLEH mematikan proses atau job lain.
// 3. Durasi setiap eksekusi dicatat ke routex_worker_duration_seconds.
type Supervisor struct {
	jobs    []ScheduledJob
	metrics *observability.Metrics
	logger  *slog.Logger

	cancel context.CancelFunc
	wg     sync.WaitGroup
}

// NewSupervisor membuat instance baru Supervisor.
func NewSupervisor(metrics *observability.Metrics, logger *slog.Logger) *Supervisor {
	if logger == nil {
		logger = slog.Default()
	}
	return &Supervisor{
		metrics: metrics,
		logger:  logger,
	}
}

// Register mendaftarkan job berkala ke supervisor.
func (s *Supervisor) Register(job Job, interval time.Duration, initialDelay time.Duration) {
	if interval <= 0 {
		interval = 1 * time.Minute
	}
	s.jobs = append(s.jobs, ScheduledJob{
		Job:          job,
		Interval:     interval,
		InitialDelay: initialDelay,
	})
}

// Start menjalankan seluruh worker yang terdaftar dalam goroutine terpisah.
func (s *Supervisor) Start(ctx context.Context) {
	runCtx, cancel := context.WithCancel(ctx)
	s.cancel = cancel

	for _, sj := range s.jobs {
		s.wg.Add(1)
		go s.runLoop(runCtx, sj)
	}
}

// Stop menghentikan seluruh worker dengan tertib dan menunggu hingga semua putaran aktif selesai.
func (s *Supervisor) Stop() {
	if s.cancel != nil {
		s.cancel()
	}
	s.wg.Wait()
}

// runLoop menjalankan perulangan satu job dengan penanganan panic dan pencatatan metrik.
func (s *Supervisor) runLoop(ctx context.Context, sj ScheduledJob) {
	defer s.wg.Done()

	if sj.InitialDelay > 0 {

		select {
		case <-ctx.Done():
			return
		case <-time.After(sj.InitialDelay):
		}
	}

	ticker := time.NewTicker(sj.Interval)
	defer ticker.Stop()

	// Jalankan langsung pada putaran pertama
	s.executeOnce(ctx, sj.Job)

	for {
		select {
		case <-ctx.Done():
			return
		case <-ticker.C:
			s.executeOnce(ctx, sj.Job)
		}
	}
}

// executeOnce mengeksekusi satu kali run job dengan perlindungan panic, isolasi error, dan observabilitas.
func (s *Supervisor) executeOnce(ctx context.Context, j Job) {
	name := j.Name()
	start := time.Now()
	outcome := OutcomeSuccess

	defer func() {
		duration := time.Since(start)
		if r := recover(); r != nil {
			outcome = OutcomePanic
			s.logger.ErrorContext(ctx, "panic tertangkap pada background worker",
				"worker", name,
				"panic", fmt.Sprint(r),
				"durasi_ms", duration.Milliseconds(),
			)
		}

		// Laporkan metrik jika tersedia
		if s.metrics != nil {
			s.metrics.WorkerRunsTotal.WithLabelValues(name, outcome).Inc()
			s.metrics.WorkerDuration.WithLabelValues(name).Observe(duration.Seconds())
		}
	}()

	if err := j.Run(ctx); err != nil {
		outcome = OutcomeError
		s.logger.WarnContext(ctx, "eksekusi background worker gagal",
			"worker", name,
			"error", err,
			"durasi_ms", time.Since(start).Milliseconds(),
		)
		return
	}
}
