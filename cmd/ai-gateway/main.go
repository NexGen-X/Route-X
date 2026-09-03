// Command ai-gateway menjalankan seluruh Route-X sebagai satu proses: API gateway,
// API admin, dashboard, aset statis, dan background worker.
//
// Tidak ada layanan terpisah dan tidak ada server frontend — aset dashboard tersemat
// di dalam biner ini lewat paket web.
package main

import (
	"context"
	"errors"
	"flag"
	"fmt"
	"io/fs"
	"log/slog"
	"net/http"
	"net/netip"
	"os"
	"os/signal"
	"strings"
	"syscall"
	"time"

	"github.com/go-chi/chi/v5"

	"github.com/NexGen-X/Route-X/internal/apikey"
	"github.com/NexGen-X/Route-X/internal/auth"
	"github.com/NexGen-X/Route-X/internal/billing"
	"github.com/NexGen-X/Route-X/internal/cache"
	"github.com/NexGen-X/Route-X/internal/config"
	"github.com/NexGen-X/Route-X/internal/contentfilter"
	"github.com/NexGen-X/Route-X/internal/database"
	"github.com/NexGen-X/Route-X/internal/database/repo/keys"
	"github.com/NexGen-X/Route-X/internal/database/repo/policy"
	"github.com/NexGen-X/Route-X/internal/database/repo/upstream"
	"github.com/NexGen-X/Route-X/internal/database/seed"
	"github.com/NexGen-X/Route-X/internal/gateway"
	"github.com/NexGen-X/Route-X/internal/health"
	"github.com/NexGen-X/Route-X/internal/httpx"
	"github.com/NexGen-X/Route-X/internal/observability"
	"github.com/NexGen-X/Route-X/internal/ratelimit"
	"github.com/NexGen-X/Route-X/internal/router"
	"github.com/NexGen-X/Route-X/internal/security"
	"github.com/NexGen-X/Route-X/web"
)

// Diisi saat build lewat -ldflags. Lihat target build di Makefile.
var (
	version = "dev"
	commit  = "none"
	builtAt = "unknown"
)

// poolStatsInterval adalah jeda pembaruan metrik pool koneksi.
const poolStatsInterval = 15 * time.Second

func main() {
	showVersion := flag.Bool("version", false, "cetak versi lalu keluar")
	migrateOnly := flag.Bool("migrate", false, "jalankan migrasi database lalu keluar, tanpa menyalakan server")
	flag.Parse()

	if *showVersion {
		fmt.Printf("ai-gateway %s (commit %s, dibuat %s)\n", version, commit, builtAt)
		return
	}

	if err := run(*migrateOnly); err != nil {
		// Logger utama mungkin belum ada saat kegagalan terjadi (mis. konfigurasi
		// tidak valid), jadi kegagalan fatal selalu ditulis ke stderr apa adanya.
		fmt.Fprintf(os.Stderr, "ai-gateway gagal start: %v\n", err)
		os.Exit(1)
	}
}

func run(migrateOnly bool) error {
	cfg, err := config.Load()
	if err != nil {
		return err
	}

	// Development memakai keluaran teks yang enak dibaca; produksi memakai JSON agar
	// bisa dicerna log shipper.
	logger := observability.NewLogger(os.Stdout, observability.LoggerOptions{
		Level:  cfg.LogLevel,
		Pretty: !cfg.AppEnv.IsProduction(),
	})
	slog.SetDefault(logger)

	// Hash pembanding argon2 disiapkan sekarang, bukan saat login pertama untuk email
	// tak terdaftar. Kalau dibiarkan malas, permintaan pertama itu harus menghitung
	// hash penuh dan justru menonjol dari sisi waktu — kebalikan dari tujuannya.
	security.WarmUp()

	logger.Info("menyalakan ai-gateway",
		"version", version, "commit", commit, "built_at", builtAt,
		"env", string(cfg.AppEnv), "go_version", runtimeVersion())

	// Context ini dibatalkan saat SIGINT/SIGTERM dan menjadi sinyal shutdown untuk
	// server maupun seluruh worker.
	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer stop()

	// --- Database -----------------------------------------------------------
	db, err := database.Connect(ctx, cfg, logger)
	if err != nil {
		return fmt.Errorf("menghubungkan database: %w", err)
	}
	defer db.Close()

	applied, err := database.Migrate(ctx, db, logger)
	if err != nil {
		return fmt.Errorf("menjalankan migrasi: %w", err)
	}
	logger.Info("migrasi database siap", "applied", applied)

	// Seed dijalankan setiap start dan bersifat idempoten: katalog izin dan pemetaan
	// peran ikut tersegarkan bila rilis baru menambah izin.
	if _, err := seed.Run(ctx, db.Pool, cfg, logger); err != nil {
		return fmt.Errorf("menanam data awal: %w", err)
	}

	if migrateOnly {
		logger.Info("mode -migrate selesai, server tidak dinyalakan")
		return nil
	}

	// --- Redis --------------------------------------------------------------
	rdb, err := cache.Connect(ctx, cfg, logger)
	if err != nil {
		return fmt.Errorf("menghubungkan Redis: %w", err)
	}
	defer func() {
		if err := rdb.Close(); err != nil {
			logger.Warn("menutup Redis", "error", err)
		}
	}()

	// --- Observability ------------------------------------------------------
	metrics := observability.NewMetrics()
	go collectPoolStats(ctx, metrics, db, rdb)

	// --- Router -------------------------------------------------------------
	// Pembatas laju login memakai Redis dan sengaja gagal-terbuka: Redis mati tidak
	// boleh membuat tidak seorang pun bisa masuk untuk memperbaikinya, sementara
	// penguncian per akun tetap ditegakkan di database.
	authSvc := auth.NewService(db.Pool, cfg, logger,
		auth.WithLoginLimiter(auth.NewLoginLimiter(rdb, logger)))

	// Permukaan /v1: inilah yang dilihat aplikasi klien. Dirakit sebelum router supaya
	// kegagalan perakitannya menghentikan start, bukan muncul sebagai 404 pada permintaan
	// pertama pelanggan.
	v1, err := buildGatewaySurface(ctx, cfg, logger, metrics, db, rdb)
	if err != nil {
		return fmt.Errorf("merakit permukaan /v1: %w", err)
	}

	mux, err := buildRouter(cfg, logger, metrics, authSvc, v1,
		health.NewChecker("postgres", db.Ping),
		health.NewChecker("redis", rdb.Ping),
	)
	if err != nil {
		return err
	}

	srv := httpx.NewServer(cfg, mux, logger)
	logger.Info("siap menerima permintaan", "addr", srv.Addr(), "dashboard", dashboardURL(cfg, srv.Addr()))

	return srv.Run(ctx)
}

// buildRouter menyusun seluruh rute HTTP beserta rantai middleware-nya.
func buildRouter(
	cfg *config.Config,
	logger *slog.Logger,
	metrics *observability.Metrics,
	authSvc *auth.Service,
	v1 http.Handler,
	checkers ...health.Checker,
) (http.Handler, error) {
	trusted, err := httpx.ParseTrustedProxies(cfg.TrustedProxies)
	if err != nil {
		return nil, fmt.Errorf("TRUSTED_PROXIES: %w", err)
	}
	if len(trusted) == 0 {
		logger.Info("TRUSTED_PROXIES kosong: X-Forwarded-For diabaikan, IP klien diambil dari koneksi langsung")
	}

	r := chi.NewRouter()

	// Urutan rantai ini disengaja: ID request dulu supaya semua log setelahnya bisa
	// dikorelasikan; RealIP sebelum AccessLog supaya IP yang tercatat sudah benar;
	// Recover di dalam AccessLog supaya request yang panic tetap tercatat; MaxBytes
	// paling akhir supaya hanya membatasi handler aplikasi.
	r.Use(
		httpx.RequestID(),
		httpx.RealIP(trusted),
		httpx.AccessLog(logger),
		httpx.Recover(logger),
		httpx.SecurityHeaders(cfg),
		httpx.MaxBytes(cfg.MaxRequestBytes),
	)

	// --- Probe kesehatan ---
	h := health.New(version, commit, logger, checkers...)
	for path, handler := range map[string]http.HandlerFunc{"/healthz": h.Live, "/readyz": h.Ready} {
		r.Get(path, handler)
		// HEAD ikut didaftarkan: banyak load balancer memakai HEAD untuk probe, dan
		// tanpa ini chi menjawab 405 sehingga instance yang sehat dianggap mati.
		r.Head(path, handler)
	}

	// --- Metrik ---
	// Endpoint ini belum berautentikasi: middleware auth baru ada di fase berikutnya.
	// Sampai saat itu, jangan paparkan port ini langsung ke internet di produksi.
	if cfg.AppEnv.IsProduction() {
		logger.Warn("/metrics belum berautentikasi",
			"saran", "batasi akses di reverse proxy atau firewall sampai autentikasi admin terpasang")
	}
	r.Handle("/metrics", metrics.Handler())

	// --- Autentikasi dashboard ---
	//
	// Rute ini tidak dilindungi RequireSession: /login justru jalan masuknya. CSRF juga
	// tidak dipasang di sini — perlindungannya berlaku pada rute admin yang sudah
	// berautentikasi cookie, dan /login sendiri dijaga kewajiban Content-Type JSON
	// plus pembatas laju.
	if authSvc != nil {
		r.Mount("/api/auth", auth.NewHandlers(authSvc).Routes())
	}

	// --- Permukaan API /v1 ---
	//
	// Rantai middleware-nya dirakit di buildGatewaySurface, bukan di sini, karena urutan
	// autentikasi dan pembatasan laju adalah bagian dari kontrak paket apikey — dan
	// memasangnya di dua tempat berarti satu tempat yang bisa lupa.
	if v1 != nil {
		r.Mount("/v1", v1)
	}

	// --- Dashboard (fallback untuk seluruh path yang tidak cocok rute di atas) ---
	dashboard, err := dashboardHandler(logger)
	if err != nil {
		return nil, err
	}
	r.NotFound(dashboard.ServeHTTP)

	return r, nil
}

// buildGatewaySurface merakit permukaan API /v1 beserta rantai middleware-nya.
//
// Urutan rantainya menentukan kebenaran, bukan kerapian:
//
//  1. Authenticate lebih dulu — pemilik kuota adalah key yang sudah terverifikasi.
//  2. Limit sesudahnya — tanpa principal di context tidak ada yang bisa dibatasi, dan
//     Limiter memang menolak rute yang dipasang tanpa Authenticate di depannya.
//  3. Handler gateway paling dalam.
//
// CSRF sengaja TIDAK ada di rantai ini, dan itu bukan kelalaian: rute ini berautentikasi
// Bearer API key, dan browser tidak pernah melampirkan header Authorization sendiri pada
// permintaan lintas situs. CSRF hanya bermakna untuk kredensial yang dikirim browser
// otomatis, yaitu cookie sesi dashboard.
func buildGatewaySurface(
	ctx context.Context,
	cfg *config.Config,
	logger *slog.Logger,
	metrics *observability.Metrics,
	db *database.DB,
	rdb *cache.Redis,
) (http.Handler, error) {
	cipher, err := security.NewCipher(cfg.EncryptionKey)
	if err != nil {
		return nil, fmt.Errorf("menyiapkan cipher kredensial: %w", err)
	}
	creds, err := upstream.NewCredentialRepo(db.Pool, cipher)
	if err != nil {
		return nil, fmt.Errorf("repository kredensial provider: %w", err)
	}
	egress, err := upstream.NewEgressRepo(db.Pool, cipher)
	if err != nil {
		return nil, fmt.Errorf("repository egress pool: %w", err)
	}
	keyRepo, err := keys.New(db.Pool, cfg.APIKeyPepper)
	if err != nil {
		return nil, fmt.Errorf("repository API key: %w", err)
	}

	if len(cfg.UpstreamAllowedPrivateAddrs) > 0 || cfg.UpstreamAllowHTTP {
		// Dicatat karena keduanya mengendurkan penjagaan yang menutup SSRF lewat base URL
		// provider. Operator yang mewarisi pemasangan orang lain harus bisa melihatnya di
		// log start, bukan hanya di berkas .env yang mungkin tidak ia pegang.
		logger.Info("penjagaan jalur keluar ke provider dilonggarkan lewat konfigurasi",
			"alamat_privat_diizinkan", cfg.UpstreamAllowedPrivateAddrs,
			"http_diizinkan", cfg.UpstreamAllowHTTP)
	}

	// State pemutus arus disimpan di Redis supaya seluruh instance sepakat provider mana
	// yang sedang rusak. Pemanasan skrip tidak wajib — jalur keputusannya tetap benar
	// tanpanya — jadi kegagalannya dicatat, bukan menghentikan start.
	breaker := gateway.NewBreaker(rdb, gateway.DefaultBreakerConfig(), logger)
	if err := breaker.Warm(ctx); err != nil {
		logger.Warn("pemanasan skrip pemutus arus gagal", "error", err)
	}

	// --- Kebijakan yang membatasi lalu lintas ---
	//
	// Ketiganya membaca tabelnya sendiri dan menyimpan salinan ber-TTL. Repository yang sama
	// dipakai bersama supaya hanya ada satu jalur pembacaan untuk keempat tabel kebijakan.
	policyRepo := policy.New(db.Pool)
	rates := ratelimit.NewEngine(policyRepo, logger)
	budgets := billing.NewEnforcer(policyRepo, logger)
	filters := contentfilter.NewEngine(policyRepo, logger)

	// Cakupan pembatasan untuk middleware: key, pengguna, IP, dan global. Cakupan MODEL
	// sengaja tidak di sini — modelnya baru diketahui setelah body diurai, dan memeriksanya
	// dua kali berarti setiap permintaan memakan dua jatah kuota pada penghitung yang sama.
	limiter := apikey.NewLimiter(rdb, metrics, logger,
		apikey.WithScopeResolver(func(ctx context.Context, p *apikey.Principal, ip netip.Addr) []apikey.ScopeLimits {
			return rates.Requests(ctx, gateway.TargetsFor(p, "", ip), p.RequestLimits())
		}),
	)
	if err := limiter.Warm(ctx); err != nil {
		logger.Warn("pemanasan skrip pembatas laju gagal", "error", err)
	}

	if inert := filters.Inert(); len(inert) > 0 {
		// Penyaring yang aktif di database tetapi tidak menegakkan apa pun adalah kegagalan
		// yang paling mahal yang bisa dimiliki tabel itu: operator melihatnya "enabled" dan
		// menyangka perlindungannya berjalan. Dilaporkan saat start, bukan hanya di dashboard.
		logger.Warn("ada penyaring konten aktif yang TIDAK menegakkan apa pun", "penyaring", inert)
	}

	models := upstream.NewModelRepo(db.Pool)
	handlers, err := gateway.NewHandlers(gateway.HandlersDeps{
		Models:     models,
		Lister:     models,
		Candidates: upstream.NewProviderRepo(db.Pool),
		Factory:    gateway.NewFactory(creds, egress, cfg.UpstreamSSRFPolicy(), logger),
		// Selector tanpa sumber latensi maupun biaya: keduanya baru ada di Fase 9. Sampai
		// itu, lowest_latency dan lowest_cost memperlakukan seluruh kandidat sebagai "belum
		// diketahui", dan menurut aturan paket router nilai yang tidak diketahui diurutkan
		// PALING BELAKANG — sehingga hasilnya urutan prioritas, bukan urutan yang dikarang
		// dari angka yang tidak ada.
		Engine:   router.NewEngine(upstream.NewRoutingRepo(db.Pool), router.NewSelector(), logger),
		Executor: gateway.NewExecutor(breaker, logger),
		// Pembatasan model dan provider per API key. Bukan opsional di produksi: tanpa ini
		// setiap key boleh memakai setiap model.
		Restrict: keyRepo,
		Guard: gateway.NewGuard(gateway.GuardDeps{
			Filters: filters,
			Budgets: budgets,
			Rates:   rates,
			Limiter: limiter,
			Logger:  logger,
		}),
		Logger: logger,
	})
	if err != nil {
		return nil, err
	}

	authn := apikey.NewAuthenticator(keyRepo, metrics, logger)

	r := chi.NewRouter()
	r.Use(authn.Authenticate(), limiter.Limit())
	r.Mount("/", handlers.Routes())
	return r, nil
}

// dashboardHandler menyajikan aset frontend yang tersemat.
//
// Kalau frontend belum pernah di-build, handler ini menjelaskan cara membangunnya
// alih-alih membalas 404 yang membingungkan.
func dashboardHandler(logger *slog.Logger) (http.Handler, error) {
	distFS, err := web.DistFS()
	if err != nil {
		return nil, fmt.Errorf("membuka aset dashboard yang tersemat: %w", err)
	}

	if _, err := fs.Stat(distFS, "index.html"); err != nil {
		if !errors.Is(err, fs.ErrNotExist) {
			return nil, fmt.Errorf("memeriksa aset dashboard: %w", err)
		}
		logger.Warn("aset dashboard belum tersemat di biner ini",
			"akibat", "permintaan ke / menjelaskan cara membangunnya",
			"perbaikan", "jalankan: make build")
		return notBuiltHandler(), nil
	}

	return httpx.SPAHandler(distFS, httpx.SPAOptions{}), nil
}

// apiPathPrefixes adalah path yang selalu dijawab sebagai API, bukan sebagai
// dokumen dashboard.
var apiPathPrefixes = []string{"/v1/", "/api/"}

// isAPIPath melaporkan apakah path ini milik permukaan API.
func isAPIPath(path string) bool {
	for _, p := range apiPathPrefixes {
		if strings.HasPrefix(path, p) {
			return true
		}
	}
	return false
}

// notBuiltHandler membalas penjelasan singkat saat dashboard belum di-build.
//
// Permintaan ke permukaan API tetap dijawab 404 dengan envelope error standar:
// klien OpenAI-compatible yang memanggil endpoint tak dikenal harus mendapat 404 yang
// bisa diurai, bukan pesan soal build frontend yang tidak ada kaitannya. Perilaku ini
// menyamai SPAHandler saat dashboard sudah di-build, sehingga jawaban untuk endpoint
// yang salah tidak berubah-ubah tergantung apakah frontend tersemat.
func notBuiltHandler() http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if isAPIPath(r.URL.Path) {
			httpx.NotFound(w, r, "Endpoint tidak ditemukan.")
			return
		}
		_ = httpx.JSON(w, http.StatusServiceUnavailable, map[string]any{
			"error": map[string]any{
				"message": "Dashboard belum di-build pada biner ini. Jalankan 'make build' agar aset frontend tersemat.",
				"type":    "api_error",
				"code":    "dashboard_not_built",
			},
			"api_tersedia": []string{"/healthz", "/readyz", "/metrics"},
		})
	})
}

// collectPoolStats memperbarui metrik pool koneksi secara berkala, supaya gauge-nya
// benar-benar mencerminkan keadaan dan bukan selalu nol.
func collectPoolStats(ctx context.Context, m *observability.Metrics, db *database.DB, rdb *cache.Redis) {
	ticker := time.NewTicker(poolStatsInterval)
	defer ticker.Stop()

	update := func() {
		ds := db.Stat()
		m.PoolConnections.WithLabelValues("postgres", "total").Set(float64(ds.TotalConns))
		m.PoolConnections.WithLabelValues("postgres", "idle").Set(float64(ds.IdleConns))
		m.PoolConnections.WithLabelValues("postgres", "in_use").Set(float64(ds.AcquiredConns))
		m.PoolConnections.WithLabelValues("postgres", "max").Set(float64(ds.MaxConns))

		rs := rdb.Stat()
		m.PoolConnections.WithLabelValues("redis", "total").Set(float64(rs.TotalConns))
		m.PoolConnections.WithLabelValues("redis", "idle").Set(float64(rs.IdleConns))
		m.PoolConnections.WithLabelValues("redis", "stale").Set(float64(rs.StaleConns))
		m.PoolConnections.WithLabelValues("redis", "max").Set(float64(rs.PoolSize))
	}

	update() // satu kali di awal supaya metrik tidak kosong sampai tick pertama
	for {
		select {
		case <-ctx.Done():
			return
		case <-ticker.C:
			update()
		}
	}
}

// dashboardURL menyusun URL yang enak diklik di log saat development.
func dashboardURL(cfg *config.Config, addr string) string {
	if cfg.PublicURL != "" {
		return cfg.PublicURL
	}
	return "http://localhost" + addr
}
