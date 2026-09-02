// Package health menyajikan endpoint liveness dan readiness.
//
// Pembagiannya sengaja tegas, karena keduanya menjawab pertanyaan berbeda:
//
//   - /healthz (liveness) menjawab "proses ini masih hidup?" dan TIDAK menyentuh
//     dependensi apa pun. Kalau endpoint ini gagal, satu-satunya obat yang masuk akal
//     adalah restart. Memeriksa database di sini justru berbahaya: gangguan database
//     akan membuat orchestrator me-restart seluruh instance gateway berulang kali,
//     padahal restart tidak memperbaiki database.
//   - /readyz (readiness) menjawab "instance ini siap menerima lalu lintas?" dan
//     memang memeriksa dependensi. Kalau gagal, instance dikeluarkan dari load
//     balancer sampai dependensinya pulih — tanpa perlu di-restart.
package health

import (
	"context"
	"log/slog"
	"net/http"
	"sync"
	"time"

	"github.com/NexGen-X/Route-X/internal/httpx"
)

// Checker adalah satu dependensi yang bisa diperiksa kesiapannya.
//
// Paket ini sengaja tidak mengimpor internal/database atau internal/cache: keduanya
// disuntikkan dari main.go sebagai Checker. Dengan begitu health tidak ikut bergantung
// pada pgx maupun go-redis, dan dependensi baru bisa ditambahkan tanpa mengubah paket
// ini sama sekali.
type Checker interface {
	// Name adalah pengenal singkat yang muncul di respons, mis. "postgres".
	Name() string
	// Check mengembalikan nil bila dependensi sehat.
	Check(ctx context.Context) error
}

// checkerFunc mengadaptasi fungsi biasa menjadi Checker.
type checkerFunc struct {
	name string
	fn   func(context.Context) error
}

func (c checkerFunc) Name() string                    { return c.name }
func (c checkerFunc) Check(ctx context.Context) error { return c.fn(ctx) }

// NewChecker membungkus fungsi menjadi Checker, sehingga main.go bisa menulis
// health.NewChecker("postgres", db.Ping) tanpa membuat tipe baru.
func NewChecker(name string, fn func(context.Context) error) Checker {
	return checkerFunc{name: name, fn: fn}
}

// Status adalah nilai status keseluruhan.
type Status string

const (
	StatusOK       Status = "ok"
	StatusDegraded Status = "degraded"
	StatusDown     Status = "down"
)

// checkTimeout membatasi setiap pemeriksaan dependensi. Nilainya pendek karena
// probe readiness dipanggil sering dan harus cepat memberi jawaban.
const checkTimeout = 3 * time.Second

// Handler menyajikan endpoint liveness dan readiness.
type Handler struct {
	version   string
	commit    string
	startedAt time.Time
	checkers  []Checker
	logger    *slog.Logger
}

// New membuat handler. checkers hanya dipakai oleh Ready, tidak oleh Live.
func New(version, commit string, logger *slog.Logger, checkers ...Checker) *Handler {
	if logger == nil {
		logger = slog.Default()
	}
	return &Handler{
		version:   version,
		commit:    commit,
		startedAt: time.Now(),
		checkers:  checkers,
		logger:    logger,
	}
}

// liveResponse adalah bentuk respons /healthz.
type liveResponse struct {
	Status    Status `json:"status"`
	Version   string `json:"version"`
	Commit    string `json:"commit"`
	UptimeSec int64  `json:"uptime_seconds"`
}

// Live menangani /healthz. Selalu 200 selama proses masih bisa melayani request.
func (h *Handler) Live(w http.ResponseWriter, r *http.Request) {
	_ = httpx.JSON(w, http.StatusOK, liveResponse{
		Status:    StatusOK,
		Version:   h.version,
		Commit:    h.commit,
		UptimeSec: int64(time.Since(h.startedAt).Seconds()),
	})
}

// checkResult adalah hasil satu dependensi di respons /readyz.
//
// Sengaja tidak memuat pesan error. Endpoint ini bisa terbuka ke jaringan (probe load
// balancer), dan pesan error dependensi gampang memuat nama host internal, port, atau
// potongan connection string. Detail lengkapnya ditulis ke log, tempat operator
// memang mencarinya.
type checkResult struct {
	Status    Status  `json:"status"`
	LatencyMS float64 `json:"latency_ms"`
}

// readyResponse adalah bentuk respons /readyz.
type readyResponse struct {
	Status    Status                 `json:"status"`
	Version   string                 `json:"version"`
	Commit    string                 `json:"commit"`
	UptimeSec int64                  `json:"uptime_seconds"`
	Checks    map[string]checkResult `json:"checks"`
}

// Ready menangani /readyz: memeriksa semua dependensi secara paralel lalu membalas
// 200 bila semuanya sehat, atau 503 bila ada yang tidak.
func (h *Handler) Ready(w http.ResponseWriter, r *http.Request) {
	ctx, cancel := context.WithTimeout(r.Context(), checkTimeout)
	defer cancel()

	results := make(map[string]checkResult, len(h.checkers))
	var (
		mu      sync.Mutex
		wg      sync.WaitGroup
		allGood = true
	)

	// Dependensi diperiksa paralel supaya total waktu ditentukan pemeriksaan terlambat,
	// bukan jumlah seluruhnya.
	for _, c := range h.checkers {
		wg.Add(1)
		go func(c Checker) {
			defer wg.Done()

			start := time.Now()
			err := c.Check(ctx)
			latency := float64(time.Since(start).Microseconds()) / 1000

			status := StatusOK
			if err != nil {
				status = StatusDown
			}

			mu.Lock()
			results[c.Name()] = checkResult{Status: status, LatencyMS: latency}
			if err != nil {
				allGood = false
			}
			mu.Unlock()

			if err != nil {
				// Detail hanya ke log — lihat komentar pada checkResult.
				h.logger.ErrorContext(ctx, "pemeriksaan kesiapan gagal",
					"dependency", c.Name(), "latency_ms", latency, "error", err.Error())
			}
		}(c)
	}
	wg.Wait()

	overall, code := StatusOK, http.StatusOK
	if !allGood {
		overall, code = StatusDown, http.StatusServiceUnavailable
	}

	_ = httpx.JSON(w, code, readyResponse{
		Status:    overall,
		Version:   h.version,
		Commit:    h.commit,
		UptimeSec: int64(time.Since(h.startedAt).Seconds()),
		Checks:    results,
	})
}

// Routes memasang kedua endpoint pada mux apa pun yang menerima http.HandlerFunc.
func (h *Handler) Routes(mux interface {
	HandleFunc(pattern string, handler func(http.ResponseWriter, *http.Request))
}) {
	mux.HandleFunc("/healthz", h.Live)
	mux.HandleFunc("/readyz", h.Ready)
}
