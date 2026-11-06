package worker

// Test pengunci B-287 (Fase 14): Supervisor.Start memanggil
// s.wg.Add(1) DI LUAR kunci s.mu sedangkan Stop memanggil
// s.wg.Wait() dari goroutine lain — pelanggaran kontrak WaitGroup
// ("Add positif saat penghitung nol tidak boleh balapan dengan
// Wait") yang ditangkap race detector pada harness stres Putaran
// Bersih (1 laporan / 120 iterasi). Perbaikan: WaitGroup
// PER-GENERASI — Start menciptakan instans baru di dalam kunci,
// Stop mengambil alih instans generasi berjalan di dalam kunci
// lalu Wait di instans itu; seluruh Add pada satu instans dengan
// demikian selalu mendahului Wait-nya lewat rantai kunci yang
// sama. Test ini menekan pola Start-vs-Stop bersamaan; di bawah
// -race ia GAGAL pada implementasi lama dan harus bersih
// sesudahnya.

import (
	"context"
	"io"
	"log/slog"
	"sync"
	"testing"
	"time"

	"github.com/NexGen-X/Route-X/internal/observability"
)

type jobB287 struct{}

func (jobB287) Name() string { return "job_b287" }
func (jobB287) Run(ctx context.Context) error {
	select {
	case <-ctx.Done():
		return ctx.Err()
	case <-time.After(2 * time.Millisecond):
		return nil
	}
}

func TestB287StartStopBersamaanTanpaRace(t *testing.T) {
	for iter := 0; iter < 120; iter++ {
		sup := NewSupervisor(observability.NewMetrics(), slog.New(slog.NewTextHandler(io.Discard, nil)))
		// Tiga job = tiga Add per Start: jendela balapan selebar
		// harness adversarial Putaran Bersih.
		sup.Register(jobB287{}, time.Hour, 0)
		sup.Register(jobB287{}, time.Hour, 0)
		sup.Register(jobB287{}, time.Hour, 0)
		ctx, cancel := context.WithCancel(context.Background())
		var wg sync.WaitGroup
		wg.Add(2)
		go func() { defer wg.Done(); sup.Start(ctx) }()
		go func() {
			defer wg.Done()
			stopCtx, sc := context.WithTimeout(context.Background(), 2*time.Second)
			defer sc()
			_ = sup.Stop(stopCtx)
		}()
		wg.Wait()
		cancel()
		// Stop pembersih: loop yang lahir sesudah Stop pertama tetap
		// harus berhenti dan WaitGroup generasinya terkuras.
		stopCtx, sc := context.WithTimeout(context.Background(), 2*time.Second)
		_ = sup.Stop(stopCtx)
		sc()
	}
}
