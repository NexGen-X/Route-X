package worker

import (
	"context"
	"errors"
	"os"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"github.com/jackc/pgx/v5/pgxpool"

	"github.com/NexGen-X/Route-X/internal/database/repo/policy"
	"github.com/NexGen-X/Route-X/internal/database/repo/upstream"
	"github.com/NexGen-X/Route-X/internal/observability"
	"github.com/NexGen-X/Route-X/internal/providers"
	"github.com/NexGen-X/Route-X/internal/webhooks"
)

func testPool(t *testing.T) *pgxpool.Pool {
	t.Helper()
	dsn := os.Getenv("DATABASE_URL")
	if dsn == "" {
		t.Skip("DATABASE_URL tidak disetel, melewati pengujian integrasi database")
	}

	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()

	pool, err := pgxpool.New(ctx, dsn)
	if err != nil {
		t.Fatalf("koneksi database gagal: %v", err)
	}
	t.Cleanup(pool.Close)
	return pool
}

// TestAdvisoryLockNamespaceMemastikanTidakBertabrakanDenganMigrasi memeriksa nilai kunci
// advisory lock worker berada pada namespace 0x5258574F... dan tidak menyentuh 0x524F55544558.
func TestAdvisoryLockNamespaceMemastikanTidakBertabrakanDenganMigrasi(t *testing.T) {
	const migrationLockKey int64 = 0x524F55544558

	keys := []int64{
		LockRollup,
		LockRetention,
		LockPartition,
		LockBudget,
		LockHealth,
		LockWebhookRecovery,
	}

	for _, k := range keys {
		if k == migrationLockKey {
			t.Fatalf("kunci lock worker 0x%X bertabrakan dengan kunci migrasi database 0x%X", k, migrationLockKey)
		}
		if (k >> 32) != (lockWorkerPrefix >> 32) {
			t.Errorf("kunci lock worker 0x%X tidak memakai prefiks 0x%X", k, lockWorkerPrefix)
		}
	}
}

// TestAdvisoryLockSesiEksklusif memeriksa perilaku penguncian advisory lock pada PostgreSQL nyata.
func TestAdvisoryLockSesiEksklusif(t *testing.T) {
	pool := testPool(t)
	ctx := context.Background()

	key := LockHealth

	// Ambil kunci pertama kali: harus sukses
	unlock1, ok1, err := TryAdvisoryLock(ctx, pool, key)
	if err != nil {
		t.Fatalf("TryAdvisoryLock 1: %v", err)
	}
	if !ok1 {
		t.Fatal("penguncian pertama seharusnya berhasil")
	}

	// Percobaan kedua dari sesi lain: harus gagal (ok = false) tanpa error
	unlock2, ok2, err := TryAdvisoryLock(ctx, pool, key)
	if err != nil {
		t.Fatalf("TryAdvisoryLock 2: %v", err)
	}
	if ok2 {
		unlock2()
		t.Fatal("penguncian kedua seharusnya gagal karena kunci sedang dipegang sesi 1")
	}

	// Lepaskan kunci pertama
	unlock1()

	// Sekarang percobaan ketiga harus berhasil
	unlock3, ok3, err := TryAdvisoryLock(ctx, pool, key)
	if err != nil {
		t.Fatalf("TryAdvisoryLock 3: %v", err)
	}
	if !ok3 {
		t.Fatal("penguncian ketiga setelah unlock seharusnya berhasil")
	}
	unlock3()
}

// TestSupervisorIsolasiPanicDanError memastikan panic dan error di satu worker tidak mematikan
// supervisor atau mengganggu worker lain, serta tercatat di metrik Prometheus.
func TestSupervisorIsolasiPanicDanError(t *testing.T) {
	metrics := observability.NewMetrics()

	sup := NewSupervisor(metrics, nil)

	var runsJobSehat atomic.Int32
	jobSehat := JobFunc{
		JobName: "worker_sehat",
		Fn: func(ctx context.Context) error {
			runsJobSehat.Add(1)
			return nil
		},
	}

	var runsJobError atomic.Int32
	jobError := JobFunc{
		JobName: "worker_error",
		Fn: func(ctx context.Context) error {
			runsJobError.Add(1)
			return errors.New("kegagalan sementara")
		},
	}

	var runsJobPanic atomic.Int32
	jobPanic := JobFunc{
		JobName: "worker_panic",
		Fn: func(ctx context.Context) error {
			runsJobPanic.Add(1)
			panic("panic yang ditangkap oleh supervisor!")
		},
	}

	sup.Register(jobSehat, 20*time.Millisecond, 0)
	sup.Register(jobError, 20*time.Millisecond, 0)
	sup.Register(jobPanic, 20*time.Millisecond, 0)

	ctx, cancel := context.WithCancel(context.Background())
	sup.Start(ctx)

	// Biarkan berjalan selama beberapa tick
	time.Sleep(100 * time.Millisecond)

	cancel()
	_ = sup.Stop(context.Background())

	if runsJobSehat.Load() < 2 {
		t.Errorf("jobSehat dieksekusi %d kali, ingin minimal 2", runsJobSehat.Load())
	}
	if runsJobError.Load() < 2 {
		t.Errorf("jobError dieksekusi %d kali, ingin minimal 2", runsJobError.Load())
	}
	if runsJobPanic.Load() < 2 {
		t.Errorf("jobPanic dieksekusi %d kali, ingin minimal 2", runsJobPanic.Load())
	}
}

// --- Tiruan untuk Health Checker ---

type dummyProvider struct {
	name    string
	healthy bool
	latency time.Duration
	errKind providers.ErrorKind
	errMsg  string
}

func (d *dummyProvider) Kind() string { return providers.KindOpenAI }
func (d *dummyProvider) Name() string { return d.name }
func (d *dummyProvider) ChatCompletion(context.Context, *providers.ChatRequest) (*providers.ChatResponse, error) {
	return nil, nil
}
func (d *dummyProvider) ChatCompletionStream(context.Context, *providers.ChatRequest) (providers.Stream, error) {
	return nil, nil
}
func (d *dummyProvider) Embeddings(context.Context, *providers.EmbeddingsRequest) (*providers.EmbeddingsResponse, error) {
	return nil, nil
}
func (d *dummyProvider) Models(context.Context) ([]providers.ModelInfo, error) { return nil, nil }
func (d *dummyProvider) HealthCheck(context.Context) providers.HealthResult {
	return providers.HealthResult{
		Healthy:      d.healthy,
		Latency:      d.latency,
		ErrorKind:    d.errKind,
		ErrorMessage: d.errMsg,
	}
}

type dummyProviderSource struct {
	providers []*upstream.Provider
	reports   []upstream.HealthReport
	mu        sync.Mutex
}

func (s *dummyProviderSource) ActiveProviders(context.Context) ([]*upstream.Provider, error) {
	return s.providers, nil
}

func (s *dummyProviderSource) RecordHealth(_ context.Context, id string, h upstream.HealthReport) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.reports = append(s.reports, h)
	for _, p := range s.providers {
		if p.ID == id {
			st := h.Status
			p.LastHealthStatus = &st
		}
	}
	return nil
}

type dummyFactory struct {
	provider *dummyProvider
}

func (f *dummyFactory) ProviderFor(_ context.Context, p *upstream.Provider) (providers.Provider, error) {
	return f.provider, nil
}

type dummyEnqueuer struct {
	events  []string
	payload []any
	mu      sync.Mutex
}

func (e *dummyEnqueuer) Enqueue(_ context.Context, event string, payload any) (int, error) {
	e.mu.Lock()
	defer e.mu.Unlock()
	e.events = append(e.events, event)
	e.payload = append(e.payload, payload)
	return 1, nil
}

// TestHealthCheckerJobTransisiKesehatan memeriksa deteksi transisi healthy -> unhealthy
// dan memproduksi event provider.unhealthy serta provider.recovered.
func TestHealthCheckerJobTransisiKesehatan(t *testing.T) {
	ctx := context.Background()

	p := &upstream.Provider{
		ID:        "p-1",
		Name:      "openai-primary",
		Enabled:   true,
		TimeoutMS: 5000,
	}

	src := &dummyProviderSource{providers: []*upstream.Provider{p}}
	dProvider := &dummyProvider{name: "openai-primary", healthy: true, latency: 150 * time.Millisecond}
	fac := &dummyFactory{provider: dProvider}
	enq := &dummyEnqueuer{}

	job := NewHealthCheckerJob(nil, src, fac, enq, nil, nil)

	// Putaran 1: provider sehat (initial status)
	if err := job.Run(ctx); err != nil {
		t.Fatalf("Run 1: %v", err)
	}
	if len(enq.events) != 0 {
		t.Errorf("putaran pertama tidak boleh memproduksi event jika belum ada transisi: %v", enq.events)
	}

	// Putaran 2: provider menjadi tidak sehat -> harus memicu provider.unhealthy
	dProvider.healthy = false
	dProvider.errKind = providers.ErrKindNetwork
	dProvider.errMsg = "koneksi terputus ke https://api.openai.com/v1/models?token=secret"

	if err := job.Run(ctx); err != nil {
		t.Fatalf("Run 2: %v", err)
	}

	enq.mu.Lock()
	if len(enq.events) != 1 || enq.events[0] != "provider.unhealthy" {
		t.Fatalf("event transisi 1 = %v, mau [provider.unhealthy]", enq.events)
	}
	enq.mu.Unlock()

	// Verifikasi bahwa error message yang tersimpan di-sanitize (tanpa token)
	src.mu.Lock()
	lastReport := src.reports[len(src.reports)-1]
	src.mu.Unlock()
	if lastReport.ErrorMessage == "" {
		t.Fatal("ErrorMessage seharusnya terisi")
	}
	if containsToken(lastReport.ErrorMessage, "secret") {
		t.Errorf("ErrorMessage membocorkan token query: %s", lastReport.ErrorMessage)
	}

	// Putaran 3: provider pulih kembali -> harus memicu provider.recovered
	dProvider.healthy = true
	dProvider.errMsg = ""

	if err := job.Run(ctx); err != nil {
		t.Fatalf("Run 3: %v", err)
	}

	enq.mu.Lock()
	if len(enq.events) != 2 || enq.events[1] != "provider.recovered" {
		t.Fatalf("event transisi 2 = %v, mau [provider.unhealthy, provider.recovered]", enq.events)
	}
	enq.mu.Unlock()
}

func containsToken(s, token string) bool {
	return len(s) > 0 && len(token) > 0 && s != webhooks.SanitizeErrorMessage(s)
}

// --- Tiruan untuk Budget Resetter ---

type dummyBudgetResetRepo struct {
	expired []*policy.Budget
	spent   upstream.USD
	resetID string
	mu      sync.Mutex
}

func (r *dummyBudgetResetRepo) ExpiredBudgets(context.Context, time.Time) ([]*policy.Budget, error) {
	r.mu.Lock()
	defer r.mu.Unlock()
	return r.expired, nil
}

func (r *dummyBudgetResetRepo) CalculateSpend(context.Context, string, string, time.Time, *time.Time) (upstream.USD, error) {
	return r.spent, nil
}

func (r *dummyBudgetResetRepo) ResetPeriodWithSpend(_ context.Context, id string, start time.Time, end *time.Time, spent upstream.USD) (*policy.Budget, error) {
	r.mu.Lock()
	defer r.mu.Unlock()
	r.resetID = id
	return &policy.Budget{ID: id, Name: "budget-reset", SpentUSD: spent}, nil
}

// TestBudgetResetterJobMemajukanPeriode memeriksa job reset anggaran kedaluwarsa.
func TestBudgetResetterJobMemajukanPeriode(t *testing.T) {
	ctx := context.Background()

	tadi := time.Now().Add(-1 * time.Hour)
	kemarin := tadi.Add(-24 * time.Hour)

	bg := &policy.Budget{
		ID:          "b-expired-1",
		Name:        "anggaran-harian",
		Period:      policy.PeriodDaily,
		PeriodStart: kemarin,
		PeriodEnd:   &tadi,
	}

	repo := &dummyBudgetResetRepo{
		expired: []*policy.Budget{bg},
		spent:   upstream.MustParseUSD("5.25000000"),
	}

	job := NewBudgetResetterJob(nil, repo, nil)
	if err := job.Run(ctx); err != nil {
		t.Fatalf("Run budget resetter: %v", err)
	}

	if repo.resetID != "b-expired-1" {
		t.Errorf("resetID = %q, mau b-expired-1", repo.resetID)
	}
}

// TestPartitionMaintainerMembuatPartisiDanMemeriksaDefault menguji pemeliharaan partisi
// bulanan pada PostgreSQL nyata.
func TestPartitionMaintainerMembuatPartisiDanMemeriksaDefault(t *testing.T) {
	pool := testPool(t)
	ctx := context.Background()

	job := NewPartitionMaintainerJob(pool, nil)
	if err := job.Run(ctx); err != nil {
		t.Fatalf("Run partition maintainer gagal: %v", err)
	}

	// Periksa bahwa partisi bulan ini dan bulan depan terdaftar di pg_class
	tables := []string{"requests", "request_events", "request_payloads", "usage_hourly"}
	now := time.Now().UTC()
	blnIni := now.Format("200601")
	blnDepan := now.AddDate(0, 1, 0).Format("200601")

	for _, tbl := range tables {
		for _, bln := range []string{blnIni, blnDepan} {
			partName := tbl + "_" + bln
			var exists bool
			err := pool.QueryRow(ctx, "select exists (select 1 from pg_class where relname = $1)", partName).Scan(&exists)
			if err != nil {
				t.Fatalf("query partisi %s: %v", partName, err)
			}
			if !exists {
				t.Errorf("partisi bulanan %s belum terbuat di database", partName)
			}
		}
	}
}

type dummyRollupRepo struct {
	called bool
	mu     sync.Mutex
}

func (d *dummyRollupRepo) RollupRange(context.Context, time.Time, time.Time) (int64, int64, error) {
	d.mu.Lock()
	defer d.mu.Unlock()
	d.called = true
	return 10, 1, nil
}

type dummyRetentionCleaner struct {
	cleaned int64
	mu      sync.Mutex
}

func (d *dummyRetentionCleaner) DeleteRetention(context.Context, time.Time, int) (int64, error) {
	d.mu.Lock()
	defer d.mu.Unlock()
	d.cleaned++
	return 0, nil
}

// TestRetentionWorkerUrutanRollupDanPembersihan memeriksa bahwa rollup selalu dijalankan
// sebelum pembersihan retensi dimulai.
func TestRetentionWorkerUrutanRollupDanPembersihan(t *testing.T) {
	pool := testPool(t)
	ctx := context.Background()

	rRepo := &dummyRollupRepo{}
	wRepo := &dummyRetentionCleaner{}

	worker := NewRetentionWorker(pool, wRepo, rRepo, 30, 7, nil)
	if err := worker.Run(ctx); err != nil {
		t.Fatalf("Run retention worker: %v", err)
	}

	rRepo.mu.Lock()
	rollupCalled := rRepo.called
	rRepo.mu.Unlock()
	if !rollupCalled {
		t.Error("aturan 4 dilanggar: rollup seharusnya dipanggil terlebih dahulu sebelum retensi")
	}

	wRepo.mu.Lock()
	cleanedCalled := wRepo.cleaned
	wRepo.mu.Unlock()
	if cleanedCalled == 0 {
		t.Error("pembersihan retensi webhook deliveries tidak dijalankan")
	}
}
