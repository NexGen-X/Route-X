package worker

import (
	"context"
	"errors"
	"fmt"
	"log/slog"
	"sync"
	"time"

	"github.com/jackc/pgx/v5/pgxpool"

	"github.com/NexGen-X/Route-X/internal/database/repo"
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

// ErrJobSkipped dikembalikan job ketika advisory lock sedang dipegang instance lain,
// sehingga Supervisor dapat mencatat putaran tersebut sebagai OutcomeSkipped.
var ErrJobSkipped = errors.New("worker: pekerjaan dilewati karena advisory lock sedang dipegang instance lain")

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
		return nil, false, repo.Err("mengambil koneksi untuk advisory lock", err)
	}

	var acquired bool
	err = conn.QueryRow(ctx, "select pg_try_advisory_lock($1)", lockKey).Scan(&acquired)
	if err != nil {
		conn.Release()
		return nil, false, repo.Err("mengeksekusi pg_try_advisory_lock", err)
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

// ErrJobNotFound dikembalikan ketika nama job yang dipicu tidak terdaftar pada supervisor.
var ErrJobNotFound = errors.New("worker: job tidak ditemukan")

// ErrSupervisorStopped menandakan job manual tidak dapat diterima karena tidak
// ada lifecycle supervisor yang dapat membatalkan dan menunggunya.
var ErrSupervisorStopped = errors.New("worker: supervisor belum berjalan")

// JobInfo memuat informasi status terkini dari satu background job untuk admin dan observabilitas.
type JobInfo struct {
	Name           string        `json:"name"`
	Schedule       string        `json:"schedule"`
	Interval       string        `json:"interval"`
	Status         string        `json:"status"`
	LastRunAt      *time.Time    `json:"last_run_at,omitempty"`
	LastDuration   time.Duration `json:"last_duration"`
	LastDurationMS int64         `json:"last_duration_ms"`
	LastStatus     string        `json:"last_status"`
	LastError      string        `json:"last_error,omitempty"`
}

// jobRuntimeState menyimpan state eksekusi internal satu job berjadwal.
type jobRuntimeState struct {
	job          ScheduledJob
	lastRunAt    *time.Time
	lastDuration time.Duration
	lastStatus   string // "ok", "failed", "running", "idle"
	lastError    string
}

// Supervisor mengelola siklus hidup seluruh background worker goroutine.
//
// Menyediakan isolasi kegagalan mutlak:
// 1. Panic di satu job ditangkap (recover), dicatat ke log, dan dihitung di metrik routex_worker_runs_total{outcome="panic"}.
// 2. Error di satu job TIDAK BOLEH mematikan proses atau job lain.
// 3. Durasi setiap eksekusi dicatat ke routex_worker_duration_seconds.
// 4. Status eksekusi dan durasi riil dilaporkan ke dashboard admin.
type Supervisor struct {
	mu      sync.RWMutex
	jobs    []*jobRuntimeState
	metrics *observability.Metrics
	logger  *slog.Logger

	cancel context.CancelFunc
	runCtx context.Context
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
	s.mu.Lock()
	defer s.mu.Unlock()

	s.jobs = append(s.jobs, &jobRuntimeState{
		job: ScheduledJob{
			Job:          job,
			Interval:     interval,
			InitialDelay: initialDelay,
		},
		lastStatus: "idle",
	})
}

// formatSchedule memformat durasi interval menjadi string jadwal manusiawi yang deskriptif.
func formatSchedule(d time.Duration) string {
	if d <= 0 {
		return "manual"
	}
	if d >= 24*time.Hour && d%(24*time.Hour) == 0 {
		return fmt.Sprintf("every %dh", int(d.Hours()))
	}
	if d >= time.Hour && d%time.Hour == 0 {
		return fmt.Sprintf("every %dh", int(d.Hours()))
	}
	if d >= time.Minute && d%time.Minute == 0 {
		return fmt.Sprintf("every %dm", int(d.Minutes()))
	}
	if d >= time.Second && d%time.Second == 0 {
		return fmt.Sprintf("every %ds", int(d.Seconds()))
	}
	return fmt.Sprintf("every %s", d.String())
}

// Jobs mengembalikan status terkini dari seluruh background job yang terdaftar secara thread-safe.
func (s *Supervisor) Jobs() []JobInfo {
	if s == nil {
		return nil
	}
	s.mu.RLock()
	defer s.mu.RUnlock()

	result := make([]JobInfo, len(s.jobs))
	for i, st := range s.jobs {
		sched := formatSchedule(st.job.Interval)
		status := "running"
		if s.cancel == nil {
			status = "stopped"
		}
		var runAt *time.Time
		if st.lastRunAt != nil {
			t := *st.lastRunAt
			runAt = &t
		}
		result[i] = JobInfo{
			Name:           st.job.Job.Name(),
			Schedule:       sched,
			Interval:       sched,
			Status:         status,
			LastRunAt:      runAt,
			LastDuration:   st.lastDuration,
			LastDurationMS: st.lastDuration.Milliseconds(),
			LastStatus:     st.lastStatus,
			LastError:      st.lastError,
		}
	}
	return result
}

// TriggerJob memicu eksekusi manual satu job tertentu berdasarkan nama di latar belakang.
// Mengembalikan ErrJobNotFound jika nama tidak ditemukan di supervisor.
func (s *Supervisor) TriggerJob(ctx context.Context, name string) error {
	if s == nil {
		return errors.New("worker: supervisor belum diinisialisasi")
	}

	s.mu.Lock()
	var target *jobRuntimeState
	for _, j := range s.jobs {
		if j.job.Job.Name() == name {
			target = j
			break
		}
	}
	if target == nil {
		s.mu.Unlock()
		return fmt.Errorf("%w: %s", ErrJobNotFound, name)
	}
	if s.runCtx == nil {
		s.mu.Unlock()
		return ErrSupervisorStopped
	}

	// Nilai context pemicu (request ID/logger) dipertahankan, sedangkan lifecycle
	// eksekusi mengikuti supervisor agar Stop membatalkan dan menunggunya.
	runCtx := mergeContextValues(s.runCtx, context.WithoutCancel(ctx))
	s.wg.Add(1)
	s.mu.Unlock()
	go func() {
		defer s.wg.Done()
		s.executeOnce(runCtx, target)
	}()
	return nil
}

// Start menjalankan seluruh worker yang terdaftar dalam goroutine terpisah.
func (s *Supervisor) Start(ctx context.Context) {
	s.mu.Lock()
	// Start yang kedua DITOLAK diam-diam: tanpa guard ini, pemanggilan ganda
	// menimpa cancel/runCtx dan melahirkan loop ganda — dua rollup, dua
	// retensi, dua pengirim webhook — plus goroutine loop lama yang bocor
	// karena cancel-nya sudah tertimpa dan tidak bisa dibatalkan lagi.
	if s.cancel != nil {
		s.mu.Unlock()
		return
	}
	runCtx, cancel := context.WithCancel(ctx)
	s.cancel = cancel
	s.runCtx = runCtx
	jobsCopy := make([]*jobRuntimeState, len(s.jobs))
	copy(jobsCopy, s.jobs)
	s.mu.Unlock()

	for _, sj := range jobsCopy {
		s.wg.Add(1)
		go s.runLoop(runCtx, sj)
	}
}

// Stop menghentikan seluruh worker dengan tertib dan menunggu hingga semua putaran aktif selesai
// atau batas tenggat waktu context tercapai.
func (s *Supervisor) Stop(ctx context.Context) error {
	s.mu.Lock()
	cancel := s.cancel
	s.cancel = nil
	s.runCtx = nil
	s.mu.Unlock()

	if cancel != nil {
		cancel()
	}

	done := make(chan struct{})
	go func() {
		s.wg.Wait()
		close(done)
	}()

	select {
	case <-done:
		return nil
	case <-ctx.Done():
		return ctx.Err()
	}
}

// valueContext mengambil pembatalan/deadline dari lifecycle supervisor dan
// nilai observabilitas dari request yang memicu job manual.
type valueContext struct {
	context.Context
	values context.Context
}

func (c valueContext) Value(key any) any {
	if value := c.values.Value(key); value != nil {
		return value
	}
	return c.Context.Value(key)
}

func mergeContextValues(lifecycle, values context.Context) context.Context {
	return valueContext{Context: lifecycle, values: values}
}

// runLoop menjalankan perulangan satu job dengan penanganan panic dan pencatatan metrik.
func (s *Supervisor) runLoop(ctx context.Context, st *jobRuntimeState) {
	defer s.wg.Done()

	if st.job.InitialDelay > 0 {
		select {
		case <-ctx.Done():
			return
		case <-time.After(st.job.InitialDelay):
		}
	}

	ticker := time.NewTicker(st.job.Interval)
	defer ticker.Stop()

	// Jalankan langsung pada putaran pertama
	s.executeOnce(ctx, st)

	for {
		select {
		case <-ctx.Done():
			return
		case <-ticker.C:
			s.executeOnce(ctx, st)
		}
	}
}

// executeOnce mengeksekusi satu kali run job dengan perlindungan panic, isolasi error, dan observabilitas.
func (s *Supervisor) executeOnce(ctx context.Context, st *jobRuntimeState) {
	name := st.job.Job.Name()
	start := time.Now()
	outcome := OutcomeSuccess

	s.mu.Lock()
	st.lastRunAt = &start
	st.lastStatus = "running"
	s.mu.Unlock()

	var runErr error
	defer func() {
		duration := time.Since(start)
		errMsg := ""
		lastStat := "ok"

		if r := recover(); r != nil {
			outcome = OutcomePanic
			lastStat = "failed"
			errMsg = fmt.Sprintf("panic: %v", r)
			s.logger.ErrorContext(ctx, "panic tertangkap pada background worker",
				"worker", name,
				"panic", fmt.Sprint(r),
				"durasi_ms", duration.Milliseconds(),
			)
		} else if errors.Is(runErr, ErrJobSkipped) {
			outcome = OutcomeSkipped
			lastStat = "skipped"
			s.logger.DebugContext(ctx, "eksekusi background worker dilewati",
				"worker", name,
				"alasan", "advisory_lock_held_by_peer",
				"durasi_ms", duration.Milliseconds(),
			)
		} else if runErr != nil {
			outcome = OutcomeError
			lastStat = "failed"
			errMsg = runErr.Error()
			s.logger.WarnContext(ctx, "eksekusi background worker gagal",
				"worker", name,
				"error", runErr,
				"durasi_ms", duration.Milliseconds(),
			)
		}

		s.mu.Lock()
		st.lastDuration = duration
		st.lastStatus = lastStat
		st.lastError = errMsg
		s.mu.Unlock()

		// Laporkan metrik jika tersedia
		if s.metrics != nil {
			s.metrics.WorkerRunsTotal.WithLabelValues(name, outcome).Inc()
			s.metrics.WorkerDuration.WithLabelValues(name).Observe(duration.Seconds())
		}
	}()

	runErr = st.job.Job.Run(ctx)
}
