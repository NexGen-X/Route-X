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

	"github.com/NexGen-X/Route-X/docs"
	"github.com/NexGen-X/Route-X/internal/admin"
	"github.com/NexGen-X/Route-X/internal/apikey"
	"github.com/NexGen-X/Route-X/internal/auth"
	"github.com/NexGen-X/Route-X/internal/billing"
	"github.com/NexGen-X/Route-X/internal/cache"
	"github.com/NexGen-X/Route-X/internal/cliconfig"
	"github.com/NexGen-X/Route-X/internal/config"
	"github.com/NexGen-X/Route-X/internal/contentfilter"
	"github.com/NexGen-X/Route-X/internal/database"
	"github.com/NexGen-X/Route-X/internal/database/repo/identity"
	"github.com/NexGen-X/Route-X/internal/database/repo/keys"
	"github.com/NexGen-X/Route-X/internal/database/repo/policy"
	"github.com/NexGen-X/Route-X/internal/database/repo/traffic"
	"github.com/NexGen-X/Route-X/internal/database/repo/upstream"
	"github.com/NexGen-X/Route-X/internal/database/seed"
	"github.com/NexGen-X/Route-X/internal/gateway"
	"github.com/NexGen-X/Route-X/internal/health"
	"github.com/NexGen-X/Route-X/internal/httpx"
	"github.com/NexGen-X/Route-X/internal/observability"
	"github.com/NexGen-X/Route-X/internal/ratelimit"
	"github.com/NexGen-X/Route-X/internal/responsecache"
	"github.com/NexGen-X/Route-X/internal/router"
	"github.com/NexGen-X/Route-X/internal/security"
	"github.com/NexGen-X/Route-X/internal/usage"
	"github.com/NexGen-X/Route-X/internal/webhooks"
	"github.com/NexGen-X/Route-X/internal/worker"
	"github.com/NexGen-X/Route-X/web"
)

// Diisi saat build lewat -ldflags. Lihat target build di Makefile.
var (
	version = "v1.1.0"
	commit  = "none"
	builtAt = "unknown"
)

// poolStatsInterval adalah jeda pembaruan metrik pool koneksi.
const poolStatsInterval = 15 * time.Second

// usageDrainTimeout adalah waktu yang diberikan pencatat pemakaian untuk menguras antreannya
// setelah server berhenti menerima permintaan.
//
// Dibangun dari context TANPA pembatalan, karena pada titik ini context aplikasi sudah Done —
// kalau diturunkan darinya, pengurasan menyerah seketika dan setiap restart kehilangan
// catatan permintaan terakhir yang sudah dilayani.
const usageDrainTimeout = 10 * time.Second

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
	v1, adminHandlers, tutupUsage, err := buildGatewaySurface(ctx, cfg, logger, metrics, db, rdb, authSvc)
	if err != nil {
		return fmt.Errorf("merakit permukaan gateway dan admin: %w", err)
	}

	mux, err := buildRouter(cfg, logger, metrics, authSvc, adminHandlers, v1,
		health.NewChecker("postgres", db.Ping),
		health.NewChecker("redis", rdb.Ping),
	)
	if err != nil {
		return err
	}

	srv := httpx.NewServer(cfg, mux, logger)
	logger.Info("siap menerima permintaan", "addr", srv.Addr(), "dashboard", dashboardURL(cfg, srv.Addr()))

	runErr := srv.Run(ctx)

	// Pencatat pemakaian ditutup SETELAH server berhenti, bukan lewat defer di dekat
	// pembuatannya: antreannya masih diisi selama ada permintaan yang berjalan, dan menutupnya
	// lebih dulu berarti setiap permintaan yang dilayani saat shutdown hilang dari log.
	tutupCtx, batalTutup := context.WithTimeout(context.Background(), usageDrainTimeout)
	defer batalTutup()
	if err := tutupUsage(tutupCtx); err != nil {
		logger.Warn("pencatat pemakaian tidak selesai menguras antrean", "error", err)
	}
	return runErr
}

// buildRouter menyusun seluruh rute HTTP beserta rantai middleware-nya.
func buildRouter(
	cfg *config.Config,
	logger *slog.Logger,
	metrics *observability.Metrics,
	authSvc *auth.Service,
	adminHandlers *admin.Handlers,
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
		httpx.MetricsRecorder(metrics),
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
	// Endpoint ini dilindungi di balik sesi admin atau token scraper / koneksi loopback untuk mencegah
	// kebocoran informasi operasional ke publik, namun tetap mengizinkan scraping metrik Prometheus.
	//
	// Pemeriksaan loopback memakai PEER koneksi langsung (lihat PeerIPFrom), bukan
	// IP hasil resolusi proxy: proxy same-host yang tidak terdaftar di
	// TRUSTED_PROXIES tidak lagi membuat trafik luar tampak sebagai loopback.
	// Di produksi METRICS_TOKEN wajib (lihat config.validate) dan loopback
	// default MATI: scraping lokal memakai Bearer token yang sama seperti
	// Prometheus jarak jauh. Setel METRICS_ALLOW_LOOPBACK=true secara eksplisit
	// bila scraper memang hanya menjangkau via loopback tanpa token.
	metricsToken := cfg.MetricsToken.Reveal()
	allowLoopback := cfg.MetricsAllowLoopback
	if authSvc != nil {
		r.With(authSvc.RequireSessionOrBearer(metricsToken, allowLoopback)).Handle("/metrics", metrics.Handler())
	} else {
		if cfg.AppEnv.IsProduction() {
			logger.Warn("/metrics belum berautentikasi",
				"saran", "pastikan authSvc aktif di lingkungan produksi untuk melindungi data operasional")
		}
		r.Handle("/metrics", metrics.Handler())
	}

	// --- Autentikasi dashboard ---
	//
	// Rute ini tidak dilindungi RequireSession: /login justru jalan masuknya. CSRF juga
	// tidak dipasang di sini — perlindungannya berlaku pada rute admin yang sudah
	// berautentikasi cookie, dan /login sendiri dijaga kewajiban Content-Type JSON
	// plus pembatas laju.
	if authSvc != nil {
		r.Mount("/api/auth", auth.NewHandlers(authSvc).Routes())
	}

	// --- REST API Admin (Fase 11) ---
	if adminHandlers != nil {
		r.Mount("/api/admin", adminHandlers.Routes())
		r.Get("/api/internal/tls-check", adminHandlers.CaddyTLSCheck)
	}

	// --- Permukaan API /v1 ---
	//
	// Rantai middleware-nya dirakit di buildGatewaySurface, bukan di sini, karena urutan
	// autentikasi dan pembatasan laju adalah bagian dari kontrak paket apikey — dan
	// memasangnya di dua tempat berarti satu tempat yang bisa lupa.
	if v1 != nil {
		r.Mount("/v1", v1)
	}

	// --- Dokumentasi API OpenAPI (Fase 13) ---
	// Endpoint dokumentasi dilindungi di balik sesi admin untuk mencegah kebocoran peta lengkap 110+ endpoint admin.
	if authSvc != nil {
		r.With(authSvc.RequireSession()).Mount("/docs", docs.Handler())
	} else {
		r.Mount("/docs", docs.Handler())
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
	authSvc *auth.Service,
) (http.Handler, *admin.Handlers, func(context.Context) error, error) {
	cipher, err := security.NewCipher(cfg.EncryptionKey)
	if err != nil {
		return nil, nil, nil, fmt.Errorf("menyiapkan cipher kredensial: %w", err)
	}
	creds, err := upstream.NewCredentialRepo(db.Pool, cipher)
	if err != nil {
		return nil, nil, nil, fmt.Errorf("repository kredensial provider: %w", err)
	}
	egress, err := upstream.NewEgressRepo(db.Pool, cipher)
	if err != nil {
		return nil, nil, nil, fmt.Errorf("repository egress pool: %w", err)
	}
	keyRepo, err := keys.New(db.Pool, cfg.APIKeyPepper)
	if err != nil {
		return nil, nil, nil, fmt.Errorf("repository API key: %w", err)
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

	// --- Webhook & Otomasi Fase 10 ---
	webhookRepo := webhooks.NewRepo(db.Pool)
	webhookDispatcher := webhooks.NewDispatcher(
		webhookRepo, cipher, nil, cfg.UpstreamSSRFPolicy(), logger,
	)

	// Pasang pemantau perpindahan status circuit breaker untuk memproduksi event
	// circuit.opened dan circuit.closed ke antrean webhook.
	breaker.SetStateChangeListener(func(ctx context.Context, providerID, model string, state gateway.State, total, failures int64) {
		ev := "circuit.opened"
		if state == gateway.StateClosed {
			ev = "circuit.closed"
		}
		payload := map[string]any{
			"event":       ev,
			"provider_id": providerID,
			"model":       model,
			"state":       string(state),
			"samples":     total,
			"failures":    failures,
			"timestamp":   time.Now().UTC().Format(time.RFC3339),
		}
		if _, err := webhookRepo.Enqueue(ctx, ev, payload); err != nil {
			logger.WarnContext(ctx, "gagal memasukkan event circuit breaker ke antrean webhook",
				"event", ev, "provider_id", providerID, "model", model, "error", err)
		}
	})

	// --- Kebijakan yang membatasi lalu lintas ---
	//
	// Ketiganya membaca tabelnya sendiri dan menyimpan salinan ber-TTL. Repository yang sama
	// dipakai bersama supaya hanya ada satu jalur pembacaan untuk keempat tabel kebijakan.
	policyRepo := policy.New(db.Pool)
	rates := ratelimit.NewEngine(policyRepo, logger)
	budgets := billing.NewEnforcer(policyRepo, logger, billing.WithWebhookEnqueuer(webhookRepo))
	filters := contentfilter.NewEngine(policyRepo, logger)

	// Cakupan pembatasan untuk middleware: key, pengguna, IP, dan global. Cakupan MODEL
	// sengaja tidak di sini — modelnya baru diketahui setelah body diurai, dan memeriksanya
	// dua kali berarti setiap permintaan memakan dua jatah kuota pada penghitung yang sama.
	limiter := apikey.NewLimiter(rdb, metrics, logger,
		apikey.WithScopeResolver(func(ctx context.Context, p *apikey.Principal, ip netip.Addr) []apikey.ScopeLimits {
			return rates.Requests(ctx, gateway.TargetsFor(p, "", ip), p.RequestLimits())
		}),
		// RATE_LIMIT_FAIL_CLOSED=true berarti pemadaman Redis menolak 429
		// alih-alih meloloskan tanpa batas. Default tetap fail-open supaya
		// login tidak mati saat Redis mati.
		apikey.WithFailClosed(cfg.RateLimitFailClosed),
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

	// --- Pemakaian, biaya, dan angka terukur -------------------------------
	//
	// Tiga bagian yang saling bergantung dan karena itu dirakit bersama:
	//
	//   trafficRepo  menulis log request dan membacanya kembali sebagai agregat
	//   pricer       menghitung biaya, dan menjadi sumber harga strategi lowest_cost
	//   index        menghitung latensi p95 terukur, dan menjadi sumber lowest_latency
	//
	// Tanpa pricer dan index, dua dari enam strategi routing memperlakukan seluruh kandidat
	// sebagai "belum diketahui" — yang menurut aturan paket router diurutkan paling belakang,
	// sehingga hasilnya sama dengan priority. Itu aman, tetapi berarti kedua strategi itu
	// tidak melakukan apa pun.
	trafficRepo := traffic.New(db.Pool)
	pricer := usage.NewPricer(upstream.NewPricingRepo(db.Pool), logger)
	if err := pricer.Warm(ctx); err != nil {
		// Tidak menghentikan start: gateway yang menolak menyala karena tabel harga belum
		// bisa dibaca menukar seluruh lalu lintas dengan ketepatan laporan biaya.
		logger.Warn("pemanasan harga model gagal, biaya awal bisa tercatat nol", "error", err)
	}

	index := usage.NewIndex(trafficRepo, metrics, logger)
	go index.Run(ctx)

	recorder := usage.New(usage.Deps{
		Store:   trafficRepo,
		Spend:   policyRepo,
		Prices:  pricer,
		Metrics: metrics,
		Logger:  logger,
	})

	models := upstream.NewModelRepo(db.Pool)
	providersRepo := upstream.NewProviderRepo(db.Pool)

	// Engine response cache untuk caching cerdas inferensi prompt berulang.
	respCache := responsecache.NewEngine(rdb, logger)

	// Pabrik adapter provider dipakai bersama antara permintaan inferensi dan health checker
	// worker, menegakkan kebijakan SSRF dan cache instance yang konsisten (Aturan 14).
	factory := gateway.NewFactory(creds, egress, cfg.UpstreamSSRFPolicy(), logger,
		gateway.WithCredentialUseMarker(creds))

	handlers, err := gateway.NewHandlers(gateway.HandlersDeps{
		Models:     models,
		Lister:     models,
		Candidates: providersRepo,
		Factory:    factory,
		Engine: router.NewEngine(upstream.NewRoutingRepo(db.Pool),
			router.NewSelector(
				router.WithLatencySource(index),
				router.WithCostSource(pricer),
			), logger),
		Executor: gateway.NewExecutor(breaker, logger),
		// Pembatasan model dan provider per API key. Bukan opsional di produksi: tanpa ini
		// setiap key boleh memakai setiap model.
		Restrict: keyRepo,
		Guard: gateway.NewGuard(gateway.GuardDeps{
			Filters: filters,
			Budgets: budgets,
			Rates:   rates,
			Limiter: limiter,
			Metrics: metrics,
			Logger:  logger,
		}),
		Usage:         recorder,
		ResponseCache: respCache,
		Logger:        logger,
	})
	if err != nil {
		return nil, nil, nil, err
	}

	// --- Background Worker Supervisor (Fase 10) ---
	workerSup := worker.NewSupervisor(metrics, logger)

	// 1. Health checker provider: mengecek endpoint upstream dan mencatat latensi serta metrik
	healthJob := worker.NewHealthCheckerJob(
		db.Pool, providersRepo, factory, webhookRepo, metrics, logger,
	)
	workerSup.Register(healthJob, cfg.HealthCheckInterval, 5*time.Second)

	// 2. Rollup scheduler: menghitung agregasi token & biaya hourly dan daily
	rollupJob := worker.NewRollupWorker(db.Pool, trafficRepo, logger)
	workerSup.Register(rollupJob, cfg.UsageRollupInterval, 10*time.Second)

	// 3. Retention cleaner: membuang partisi lampau dan menghapus batch payload serta delivery
	retentionJob := worker.NewRetentionWorker(
		db.Pool, webhookRepo, trafficRepo,
		cfg.RequestLogRetentionDays, cfg.RequestBodyRetentionDays, logger,
	)
	workerSup.Register(retentionJob, 1*time.Hour, 30*time.Second)

	// 4. Partition maintainer: membuat partisi bulanan masa depan dan mengaudit partisi default
	partitionJob := worker.NewPartitionMaintainerJob(db.Pool, logger)
	workerSup.Register(partitionJob, 12*time.Hour, 15*time.Second)

	// 5. Budget period resetter: memajukan periode anggaran kedaluwarsa dan menghitung ulang pemakaian
	budgetJob := worker.NewBudgetResetterJob(db.Pool, policyRepo, logger)
	workerSup.Register(budgetJob, 1*time.Minute, 5*time.Second)

	// 6. Webhook delivery worker: memproses antrean pengiriman webhook dengan FOR UPDATE SKIP LOCKED
	webhookJob := worker.NewWebhookWorker(db.Pool, webhookRepo, webhookDispatcher, logger)
	workerSup.Register(webhookJob, 1*time.Second, 2*time.Second)

	workerSup.Start(ctx)

	authn := apikey.NewAuthenticator(keyRepo, metrics, logger)

	r := chi.NewRouter()
	// CORS wajib dipasang sebelum autentikasi agar preflight OPTIONS dari browser direspons langsung
	// tanpa ditolak oleh validator Authorization header.
	r.Use(httpx.CORS(nil))
	r.Use(authn.Authenticate(), limiter.Limit())
	r.Mount("/", handlers.Routes())

	// --- REST API Admin Handlers (Fase 11) ---
	settingsRepo := identity.NewSettings(db.Pool)
	cliMgr := cliconfig.NewManager(settingsRepo, cipher, logger)

	adminHandlers := admin.NewHandlers(admin.Config{
		Pool:           db.Pool,
		Redis:          rdb.Client(),
		AuthSvc:        authSvc,
		Logger:         logger,
		ProviderRepo:   providersRepo,
		CredentialRepo: creds,
		ModelRepo:      models,
		PricingRepo:    upstream.NewPricingRepo(db.Pool),
		RoutingRepo:    upstream.NewRoutingRepo(db.Pool),
		EgressRepo:     egress,
		PolicyRepo:     policyRepo,
		KeyRepo:        keyRepo,
		UsersRepo:      identity.NewUsers(db.Pool),
		SessionsRepo:   identity.NewSessions(db.Pool),
		SettingsRepo:   settingsRepo,
		AuditRepo:      identity.NewAudit(db.Pool),
		TrafficRepo:    trafficRepo,
		WebhooksRepo:   webhookRepo,
		Dispatcher:     webhookDispatcher,
		Factory:        factory,
		Breaker:        breaker,
		Supervisor:     workerSup,
		Cipher:         cipher,
		ResponseCache:  respCache,
		CLIManager:     cliMgr,
		Version:        version,
		Commit:         commit,
		BuiltAt:        builtAt,
	})

	closeAll := func(c context.Context) error {
		_ = workerSup.Stop(c)
		_ = breaker.Close(c)
		return recorder.Close(c)
	}

	return r, adminHandlers, closeAll, nil
}

// dashboardHandler menyajikan aset frontend yang tersemat.

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

	spa := httpx.SPAHandler(distFS, httpx.SPAOptions{})
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if isAPIPath(r.URL.Path) {
			httpx.NotFound(w, r, "Endpoint tidak ditemukan.")
			return
		}
		spa.ServeHTTP(w, r)
	}), nil
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
