// Package httpx menyediakan lapisan HTTP dasar Route-X: server dengan shutdown
// anggun, middleware yang aman untuk streaming, envelope error kompatibel OpenAI,
// dan penyaji aset SPA.
//
// # Aturan yang mengikat seluruh paket ini
//
// Endpoint /v1/chat/completions mengalirkan token lewat Server-Sent Events, jadi
// apa pun yang membungkus http.ResponseWriter WAJIB:
//
//   - meneruskan Flush (dan FlushError) supaya setiap event langsung terkirim, dan
//   - menyediakan Unwrap() http.ResponseWriter supaya http.ResponseController bisa
//     menemukan writer asli untuk SetReadDeadline/SetWriteDeadline/Hijack.
//
// Wrapper yang menelan Flush mengubah stream menjadi satu respons besar di akhir,
// dan wrapper tanpa Unwrap membuat handler kehilangan kendali atas tenggat koneksi.
//
// Urutan middleware yang disarankan untuk main.go, dari paling luar ke paling dalam:
//
//	httpx.Chain(
//	    httpx.RequestID(),
//	    httpx.RealIP(trustedProxies),
//	    httpx.AccessLog(logger),
//	    httpx.Recover(logger),
//	    httpx.SecurityHeaders(cfg),
//	    httpx.MaxBytes(cfg.MaxRequestBytes),
//	)
//
// RequestID paling luar supaya seluruh log dan envelope error punya ID yang sama;
// Recover di dalam AccessLog supaya status 500 hasil panic ikut tercatat.
package httpx

import (
	"context"
	"errors"
	"fmt"
	"log/slog"
	"net"
	"net/http"
	"sync"
	"time"

	"github.com/NexGen-X/Route-X/internal/config"
	"github.com/NexGen-X/Route-X/internal/observability"
)

const (
	// defaultReadHeaderTimeout membatasi waktu pengiriman header. Ini pertahanan
	// wajib terhadap Slowloris: tanpa batas ini satu koneksi bisa menahan goroutine
	// selamanya dengan mengirim header satu byte per menit.
	defaultReadHeaderTimeout = 10 * time.Second

	// defaultIdleTimeout menutup koneksi keep-alive yang menganggur. Klien OpenAI
	// memakai ulang koneksi, jadi nilainya harus jauh di atas jeda antar-request.
	defaultIdleTimeout = 120 * time.Second

	// defaultShutdownGrace dipakai bila konfigurasi tidak memberi nilai.
	defaultShutdownGrace = 25 * time.Second

	// defaultMaxHeaderBytes 64 KiB. Lebih ketat dari default net/http (1 MiB) karena
	// request gateway hanya butuh Authorization plus beberapa header kecil, dan
	// header besar dikalikan jumlah koneksi adalah jalur mudah menghabiskan memori.
	defaultMaxHeaderBytes = 1 << 16
)

// Server membungkus http.Server dengan siklus hidup yang dikendalikan context.
type Server struct {
	srv    *http.Server
	logger *slog.Logger
	grace  time.Duration

	mu   sync.RWMutex
	addr string
}

// NewServer menyiapkan http.Server dengan seluruh timeout terisi. cfg tidak boleh
// nil; logger nil jatuh ke slog.Default().
//
// WriteTimeout diambil apa adanya dari konfigurasi, termasuk nilai 0 yang berarti
// tanpa batas. Ini disengaja: respons SSE bisa berjalan beberapa menit, dan
// WriteTimeout memasang tenggat pada seluruh respons sehingga nilai positif akan
// memotong stream di tengah jalan. Kalau operator memang mengisinya, handler
// streaming harus memanggil PrepareSSE untuk melepas tenggat per request.
func NewServer(cfg *config.Config, handler http.Handler, logger *slog.Logger) *Server {
	if logger == nil {
		logger = slog.Default()
	}

	grace := cfg.ShutdownGrace
	if grace <= 0 {
		grace = defaultShutdownGrace
	}

	// ReadHeaderTimeout tidak boleh melebihi ReadTimeout, kalau tidak batas header
	// tidak akan pernah tercapai lebih dulu.
	readHeaderTimeout := defaultReadHeaderTimeout
	if cfg.ReadTimeout > 0 && cfg.ReadTimeout < readHeaderTimeout {
		readHeaderTimeout = cfg.ReadTimeout
	}

	s := &Server{
		logger: logger,
		grace:  grace,
		addr:   cfg.Addr(),
	}

	s.srv = &http.Server{
		Addr:              cfg.Addr(),
		Handler:           handler,
		ReadHeaderTimeout: readHeaderTimeout,
		ReadTimeout:       cfg.ReadTimeout,
		WriteTimeout:      cfg.WriteTimeout,
		IdleTimeout:       defaultIdleTimeout,
		MaxHeaderBytes:    defaultMaxHeaderBytes,
		// net/http menulis galat tingkat koneksi ke logger sendiri; salurkan ke slog
		// supaya formatnya seragam dan tidak lolos ke stderr tanpa struktur.
		ErrorLog: slog.NewLogLogger(logger.With(slog.String("component", "net/http")).Handler(), slog.LevelWarn),
		BaseContext: func(net.Listener) context.Context {
			return observability.WithLogger(context.Background(), logger)
		},
	}

	if cfg.ReadTimeout > 0 {
		logger.Debug("READ_TIMEOUT aktif; handler streaming wajib memanggil httpx.PrepareSSE agar context request tidak dibatalkan saat tenggat baca habis",
			slog.Duration("read_timeout", cfg.ReadTimeout))
	}
	if cfg.WriteTimeout > 0 {
		logger.Warn("WRITE_TIMEOUT aktif; respons SSE yang lebih lama dari nilai ini akan terputus kecuali handler memanggil httpx.PrepareSSE",
			slog.Duration("write_timeout", cfg.WriteTimeout))
	}

	return s
}

// Addr mengembalikan alamat listen. Sebelum Run berhasil bind nilainya adalah
// alamat yang diminta (mis. ":8080"); setelahnya alamat nyata, yang berguna saat
// PORT=0 dipakai di test.
func (s *Server) Addr() string {
	s.mu.RLock()
	defer s.mu.RUnlock()
	return s.addr
}

// Run melayani request sampai ctx dibatalkan, lalu melakukan shutdown anggun dengan
// tenggat ShutdownGrace.
//
// Mengembalikan nil bila server berhenti dengan bersih: http.ErrServerClosed adalah
// hasil normal dari Shutdown, bukan kegagalan. Error dikembalikan bila bind gagal,
// Serve berhenti karena sebab lain, atau koneksi masih menggantung saat tenggat
// shutdown habis.
func (s *Server) Run(ctx context.Context) error {
	// Nilai context aplikasi (logger) diwarisi request, tapi pembatalannya TIDAK.
	// Kalau pembatalan ikut diwarisi, sinyal SIGTERM akan membatalkan context semua
	// request in-flight seketika dan memutus stream yang sedang jalan — justru yang
	// harus dihindari oleh shutdown anggun.
	baseCtx := observability.WithLogger(context.WithoutCancel(ctx), s.logger)
	s.srv.BaseContext = func(net.Listener) context.Context { return baseCtx }

	// Listener dibuat terpisah dari Serve supaya kegagalan bind (port sudah dipakai)
	// dilaporkan langsung ke pemanggil, dan supaya alamat efektif diketahui.
	ln, err := net.Listen("tcp", s.srv.Addr)
	if err != nil {
		return fmt.Errorf("membuka listener di %q: %w", s.srv.Addr, err)
	}

	s.mu.Lock()
	s.addr = ln.Addr().String()
	s.mu.Unlock()

	s.logger.InfoContext(ctx, "server http mulai mendengarkan",
		slog.String("addr", s.Addr()),
		slog.Duration("read_header_timeout", s.srv.ReadHeaderTimeout),
		slog.Duration("read_timeout", s.srv.ReadTimeout),
		slog.Duration("write_timeout", s.srv.WriteTimeout),
		slog.Duration("idle_timeout", s.srv.IdleTimeout),
	)

	serveErr := make(chan error, 1)
	go func() { serveErr <- s.srv.Serve(ln) }()

	select {
	case err := <-serveErr:
		if err == nil || errors.Is(err, http.ErrServerClosed) {
			return nil
		}
		return fmt.Errorf("server http berhenti: %w", err)
	case <-ctx.Done():
	}

	// ctx sudah Done, jadi tenggat shutdown harus dibangun dari context tanpa
	// pembatalan — kalau tidak, Shutdown langsung menyerah tanpa menguras apa pun.
	shutdownCtx, cancel := context.WithTimeout(context.WithoutCancel(ctx), s.grace)
	defer cancel()

	s.logger.InfoContext(shutdownCtx, "server http mulai shutdown anggun",
		slog.Duration("grace", s.grace))

	start := time.Now()
	shutdownErr := s.srv.Shutdown(shutdownCtx)
	// Serve selalu kembali dengan ErrServerClosed setelah Shutdown; tunggu supaya
	// tidak ada goroutine yang tertinggal.
	<-serveErr
	elapsed := time.Since(start)

	if shutdownErr != nil {
		// Tenggat habis dengan koneksi masih terbuka: putus paksa, kalau tidak
		// proses tidak akan pernah keluar.
		closeErr := s.srv.Close()
		s.logger.WarnContext(shutdownCtx, "tenggat shutdown habis, koneksi yang tersisa diputus paksa",
			slog.Duration("durasi", elapsed),
			slog.String("error", shutdownErr.Error()),
		)
		if closeErr != nil {
			return fmt.Errorf("shutdown melewati tenggat %s: %w (menutup paksa: %v)", s.grace, shutdownErr, closeErr)
		}
		return fmt.Errorf("shutdown melewati tenggat %s: %w", s.grace, shutdownErr)
	}

	s.logger.InfoContext(shutdownCtx, "server http berhenti dengan bersih",
		slog.Duration("durasi", elapsed))
	return nil
}
