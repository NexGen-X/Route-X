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
//
// Kedua endpoint publik tanpa autentikasi, jadi responsnya TIDAK memuat versi,
// commit, uptime, latensi, atau pesan error dependensi (audit3 V-LN-06): informasi
// itu milik operator dan tersedia di log serta endpoint admin. Yang disajikan
// hanya status — cukup untuk probe orchestrator dan load balancer.
package health

import (
	"context"
	"fmt"
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
	name      string
	fn        func(context.Context) error
	degradasi bool
}

func (c checkerFunc) Name() string                    { return c.name }
func (c checkerFunc) Check(ctx context.Context) error { return c.fn(ctx) }

// nonKritis menandai Checker yang kegagalannya menurunkan status menjadi
// degraded (tetap HTTP 200), bukan down (HTTP 503).
func (c checkerFunc) nonKritis() bool { return c.degradasi }

// NewChecker membungkus fungsi menjadi Checker KRITIS: kegagalannya membuat
// /readyz menjawab down/503, karena tanpa dependensi itu gateway tidak bisa
// melayani lalu lintas secara benar.
func NewChecker(name string, fn func(context.Context) error) Checker {
	return checkerFunc{name: name, fn: fn}
}

// NewCheckerNonKritis membungkus fungsi menjadi Checker NON-kritis:
// kegagalannya membuat /readyz menjawab degraded tetapi tetap HTTP 200.
//
// Pakai untuk dependensi yang ketiadaannya ditoleransi desain gateway —
// misalnya Redis, yang jalurnya memang fail-open untuk chat (penegakan batas
// laju berhenti, tetapi permintaan tetap dilayani). Menjawab 503 untuk
// dependensi semacam itu justru mengeluarkan instance dari load balancer
// tanpa alasan yang sejalan dengan perilaku gateway sendiri (audit3 V-LN-03).
func NewCheckerNonKritis(name string, fn func(context.Context) error) Checker {
	return checkerFunc{name: name, fn: fn, degradasi: true}
}

// apakahNonKritis membaca penanda non-kritis dari Checker yang membawanya.
func apakahNonKritis(c Checker) bool {
	nk, ok := c.(interface{ nonKritis() bool })
	return ok && nk.nonKritis()
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

// readyCacheTTL adalah umur hasil /readyz yang disajikan ulang tanpa memeriksa
// dependensi lagi. /readyz publik tanpa autentikasi; tanpa cache ini setiap
// probe memicu round-trip segar ke seluruh dependensi, sehingga endpoint
// kesiapan sendiri menjadi penguat beban yang murah dari internet
// (audit3 V-LN-02). Dua detik cukup untuk memutus amplifikasi itu tanpa
// membuat perubahan status terasa lambat bagi orchestrator.
const readyCacheTTL = 2 * time.Second

// Handler menyajikan endpoint liveness dan readiness.
type Handler struct {
	checkers []Checker
	logger   *slog.Logger

	// muReady menyerialkan pemeriksaan readyz: probe yang datang bersamaan
	// menunggu pemeriksaan yang sedang berjalan lalu memakai hasilnya dari
	// cache (singleflight), bukan menjalankan pemeriksaannya sendiri-sendiri.
	muReady     sync.Mutex
	cacheKode   int
	cacheHasil  readyResponse
	cacheSampai time.Time
}

// New membuat handler. checkers hanya dipakai oleh Ready, tidak oleh Live.
//
// Parameter version dan commit dipertahankan demi kompatibilitas pemanggil,
// tetapi TIDAK lagi disajikan di respons (audit3 V-LN-06) — versi berjalan
// tersedia untuk operator lewat log startup dan endpoint admin.
func New(version, commit string, logger *slog.Logger, checkers ...Checker) *Handler {
	_, _ = version, commit
	if logger == nil {
		logger = slog.Default()
	}
	return &Handler{
		checkers: checkers,
		logger:   logger,
	}
}

// liveResponse adalah bentuk respons /healthz. Sengaja hanya status:
// endpoint ini publik dan versi/commit/uptime adalah informasi operasional
// yang tidak dibutuhkan probe mana pun (audit3 V-LN-06).
type liveResponse struct {
	Status Status `json:"status"`
}

// Live menangani /healthz. Selalu 200 selama proses masih bisa melayani request.
func (h *Handler) Live(w http.ResponseWriter, r *http.Request) {
	_ = httpx.JSON(w, http.StatusOK, liveResponse{Status: StatusOK})
}

// checkResult adalah hasil satu dependensi di respons /readyz.
//
// Sengaja hanya memuat status. Endpoint ini bisa terbuka ke jaringan (probe
// load balancer), dan pesan error maupun latensi dependensi adalah informasi
// internal; detail lengkapnya ditulis ke log, tempat operator memang
// mencarinya.
type checkResult struct {
	Status Status `json:"status"`
}

// readyResponse adalah bentuk respons /readyz.
type readyResponse struct {
	Status Status                 `json:"status"`
	Checks map[string]checkResult `json:"checks"`
}

// Ready menangani /readyz: memeriksa semua dependensi secara paralel lalu
// membalas 200 bila semuanya sehat, 200 dengan status degraded bila hanya
// dependensi non-kritis yang gagal, atau 503 bila ada dependensi kritis yang
// gagal. Hasil disajikan dari cache selama readyCacheTTL (lihat konstanta itu).
func (h *Handler) Ready(w http.ResponseWriter, r *http.Request) {
	h.muReady.Lock()
	if time.Now().Before(h.cacheSampai) {
		kode, hasil := h.cacheKode, h.cacheHasil
		h.muReady.Unlock()
		_ = httpx.JSON(w, kode, hasil)
		return
	}
	kode, hasil := h.periksa(r.Context())
	h.cacheKode, h.cacheHasil = kode, hasil
	h.cacheSampai = time.Now().Add(readyCacheTTL)
	h.muReady.Unlock()

	_ = httpx.JSON(w, kode, hasil)
}

// periksa menjalankan seluruh Checker secara paralel dan menyusun hasilnya.
func (h *Handler) periksa(ctxProbe context.Context) (int, readyResponse) {
	ctx, cancel := context.WithTimeout(ctxProbe, checkTimeout)
	defer cancel()

	results := make(map[string]checkResult, len(h.checkers))
	var (
		mu           sync.Mutex
		wg           sync.WaitGroup
		adaKritis    bool
		adaNonKritis bool
	)

	// Dependensi diperiksa paralel supaya total waktu ditentukan pemeriksaan terlambat,
	// bukan jumlah seluruhnya.
	for _, c := range h.checkers {
		wg.Add(1)
		go func(c Checker) {
			defer wg.Done()
			// Panic di dalam Checker berjalan di goroutine ini — di luar
			// jangkauan middleware Recover maupun penangkap net/http — jadi
			// tanpa recover di sini satu Checker yang panik menjatuhkan
			// SELURUH proses gateway (audit3 V-LN-01). Panic diperlakukan
			// sebagai kegagalan dependensi itu sendiri.
			var err error
			func() {
				defer func() {
					if rec := recover(); rec != nil {
						err = fmt.Errorf("panic pada pemeriksaan kesiapan: %v", rec)
					}
				}()
				err = c.Check(ctx)
			}()

			status := StatusOK
			if err != nil {
				status = StatusDown
			}

			mu.Lock()
			results[c.Name()] = checkResult{Status: status}
			if err != nil {
				if apakahNonKritis(c) {
					adaNonKritis = true
				} else {
					adaKritis = true
				}
			}
			mu.Unlock()

			if err != nil {
				// Detail hanya ke log — lihat komentar pada checkResult.
				h.logger.ErrorContext(ctx, "pemeriksaan kesiapan gagal",
					"dependency", c.Name(), "error", err.Error())
			}
		}(c)
	}
	wg.Wait()

	overall, code := StatusOK, http.StatusOK
	switch {
	case adaKritis:
		overall, code = StatusDown, http.StatusServiceUnavailable
	case adaNonKritis:
		overall = StatusDegraded
	}

	return code, readyResponse{Status: overall, Checks: results}
}

// Routes memasang kedua endpoint pada mux apa pun yang menerima http.HandlerFunc.
func (h *Handler) Routes(mux interface {
	HandleFunc(pattern string, handler func(http.ResponseWriter, *http.Request))
}) {
	mux.HandleFunc("/healthz", h.Live)
	mux.HandleFunc("/readyz", h.Ready)
}
