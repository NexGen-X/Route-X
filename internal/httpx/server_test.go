package httpx

import (
	"context"
	"errors"
	"io"
	"net"
	"net/http"
	"strings"
	"testing"
	"time"

	"github.com/NexGen-X/Route-X/internal/config"
	"github.com/NexGen-X/Route-X/internal/observability"
)

func TestNewServerMengisiSeluruhTimeout(t *testing.T) {
	logger, _ := newTestLogger()

	t.Run("WriteTimeout nol dipertahankan untuk SSE", func(t *testing.T) {
		cfg := testConfig(config.EnvProduction)
		cfg.ReadTimeout = 30 * time.Second
		cfg.WriteTimeout = 0

		s := NewServer(cfg, http.NotFoundHandler(), logger)

		if s.srv.WriteTimeout != 0 {
			t.Errorf("WriteTimeout %s, want 0 supaya stream SSE tidak terpotong", s.srv.WriteTimeout)
		}
		if s.srv.ReadTimeout != 30*time.Second {
			t.Errorf("ReadTimeout %s, want 30s", s.srv.ReadTimeout)
		}
		if s.srv.ReadHeaderTimeout <= 0 {
			t.Error("ReadHeaderTimeout wajib diisi sebagai pertahanan Slowloris")
		}
		if s.srv.ReadHeaderTimeout != defaultReadHeaderTimeout {
			t.Errorf("ReadHeaderTimeout %s, want %s", s.srv.ReadHeaderTimeout, defaultReadHeaderTimeout)
		}
		if s.srv.IdleTimeout <= 0 {
			t.Error("IdleTimeout wajib diisi")
		}
		if s.srv.MaxHeaderBytes != defaultMaxHeaderBytes {
			t.Errorf("MaxHeaderBytes %d, want %d", s.srv.MaxHeaderBytes, defaultMaxHeaderBytes)
		}
		if s.srv.BaseContext == nil {
			t.Error("BaseContext wajib diisi supaya request mewarisi context aplikasi")
		}
	})

	t.Run("ReadHeaderTimeout tidak melebihi ReadTimeout", func(t *testing.T) {
		cfg := testConfig(config.EnvDevelopment)
		cfg.ReadTimeout = 3 * time.Second

		s := NewServer(cfg, http.NotFoundHandler(), logger)

		if s.srv.ReadHeaderTimeout != 3*time.Second {
			t.Errorf("ReadHeaderTimeout %s, want 3s", s.srv.ReadHeaderTimeout)
		}
	})

	t.Run("ShutdownGrace nol memakai default", func(t *testing.T) {
		cfg := testConfig(config.EnvDevelopment)
		cfg.ShutdownGrace = 0

		if got := NewServer(cfg, http.NotFoundHandler(), logger).grace; got != defaultShutdownGrace {
			t.Errorf("grace %s, want %s", got, defaultShutdownGrace)
		}
	})

	t.Run("logger nil tidak membuat panic", func(t *testing.T) {
		if NewServer(testConfig(config.EnvDevelopment), http.NotFoundHandler(), nil) == nil {
			t.Error("NewServer mengembalikan nil")
		}
	})
}

func TestServerRunShutdownAnggun(t *testing.T) {
	logger, logs := newTestLogger()
	cfg := testConfig(config.EnvDevelopment)

	s := NewServer(cfg, http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusNoContent)
	}), logger)

	ctx, cancel := context.WithCancel(context.Background())
	runErr := make(chan error, 1)
	go func() { runErr <- s.Run(ctx) }()

	addr := waitForListen(t, s)

	resp, err := http.Get("http://" + addr + "/v1/models")
	if err != nil {
		t.Fatalf("request ke server: %v", err)
	}
	_ = resp.Body.Close()
	if resp.StatusCode != http.StatusNoContent {
		t.Fatalf("status %d, want 204", resp.StatusCode)
	}

	cancel()

	select {
	case err := <-runErr:
		if err != nil {
			t.Fatalf("Run mengembalikan error pada shutdown normal: %v", err)
		}
	case <-time.After(10 * time.Second):
		t.Fatal("Run tidak selesai setelah context dibatalkan")
	}

	out := logs.String()
	for _, want := range []string{"mulai mendengarkan", "mulai shutdown anggun", "berhenti dengan bersih", "durasi"} {
		if !strings.Contains(out, want) {
			t.Errorf("log tidak memuat %q\nlog: %s", want, out)
		}
	}

	// Listener harus benar-benar tertutup.
	if conn, err := net.DialTimeout("tcp", addr, 500*time.Millisecond); err == nil {
		_ = conn.Close()
		t.Error("port masih menerima koneksi setelah shutdown")
	}
}

func TestServerRunMelaporkanKegagalanBind(t *testing.T) {
	logger, _ := newTestLogger()

	// Kuasai satu port lebih dulu supaya bind kedua pasti gagal.
	ln, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatalf("menyiapkan listener: %v", err)
	}
	defer ln.Close()

	_, port, err := net.SplitHostPort(ln.Addr().String())
	if err != nil {
		t.Fatalf("memecah alamat: %v", err)
	}

	cfg := testConfig(config.EnvDevelopment)
	s := NewServer(cfg, http.NotFoundHandler(), logger)
	s.srv.Addr = net.JoinHostPort("127.0.0.1", port)

	err = s.Run(context.Background())
	if err == nil {
		t.Fatal("Run harus melaporkan kegagalan bind")
	}
	if !strings.Contains(err.Error(), "membuka listener") {
		t.Fatalf("pesan error %q tidak menjelaskan kegagalan bind", err)
	}
}

// TestServerBaseContextMewarisiNilaiTanpaPembatalan menjaga inti shutdown anggun:
// request yang sedang jalan harus tetap hidup setelah sinyal berhenti, dan tetap
// bisa memakai logger aplikasi dari context.
func TestServerBaseContextMewarisiNilaiTanpaPembatalan(t *testing.T) {
	logger, logs := newTestLogger()
	cfg := testConfig(config.EnvDevelopment)

	mulai := make(chan struct{})
	lanjut := make(chan struct{})
	ctxErr := make(chan error, 1)

	s := NewServer(cfg, http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		observability.LoggerFrom(r.Context()).Info("logger aplikasi tersedia di handler")
		close(mulai)
		<-lanjut
		ctxErr <- r.Context().Err()
		w.WriteHeader(http.StatusNoContent)
	}), logger)

	ctx, cancel := context.WithCancel(context.Background())
	runErr := make(chan error, 1)
	go func() { runErr <- s.Run(ctx) }()

	addr := waitForListen(t, s)

	respCh := make(chan int, 1)
	go func() {
		resp, err := http.Get("http://" + addr + "/v1/models")
		if err != nil {
			respCh <- -1
			return
		}
		_, _ = io.Copy(io.Discard, resp.Body)
		_ = resp.Body.Close()
		respCh <- resp.StatusCode
	}()

	select {
	case <-mulai:
	case <-time.After(5 * time.Second):
		t.Fatal("handler tidak pernah mulai")
	}

	// Sinyal berhenti tiba sementara request masih diproses.
	cancel()
	time.Sleep(100 * time.Millisecond)
	close(lanjut)

	select {
	case err := <-ctxErr:
		if err != nil {
			t.Fatalf("context request dibatalkan saat shutdown: %v; stream in-flight akan terputus", err)
		}
	case <-time.After(5 * time.Second):
		t.Fatal("handler tidak menyelesaikan pemeriksaan context")
	}

	select {
	case status := <-respCh:
		if status != http.StatusNoContent {
			t.Fatalf("status %d, want 204", status)
		}
	case <-time.After(5 * time.Second):
		t.Fatal("klien tidak menerima respons")
	}

	select {
	case err := <-runErr:
		if err != nil {
			t.Fatalf("Run: %v", err)
		}
	case <-time.After(10 * time.Second):
		t.Fatal("Run tidak selesai")
	}

	if !strings.Contains(logs.String(), "logger aplikasi tersedia di handler") {
		t.Errorf("logger tidak diwarisi lewat BaseContext: %s", logs.String())
	}
}

func TestServerRunMengembalikanErrorSaatShutdownMelewatiTenggat(t *testing.T) {
	logger, logs := newTestLogger()
	cfg := testConfig(config.EnvDevelopment)
	cfg.ShutdownGrace = 150 * time.Millisecond

	lepaskan := make(chan struct{})
	s := NewServer(cfg, http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		<-lepaskan
		w.WriteHeader(http.StatusOK)
	}), logger)

	ctx, cancel := context.WithCancel(context.Background())
	runErr := make(chan error, 1)
	go func() { runErr <- s.Run(ctx) }()

	addr := waitForListen(t, s)

	go func() {
		resp, err := http.Get("http://" + addr + "/lambat")
		if err == nil {
			_, _ = io.Copy(io.Discard, resp.Body)
			_ = resp.Body.Close()
		}
	}()

	// Beri waktu request tersambung dan menggantung di handler.
	time.Sleep(200 * time.Millisecond)
	cancel()

	select {
	case err := <-runErr:
		if err == nil {
			t.Fatal("shutdown yang melewati tenggat harus dilaporkan sebagai error")
		}
		if !errors.Is(err, context.DeadlineExceeded) {
			t.Fatalf("error %v, want membungkus context.DeadlineExceeded", err)
		}
	case <-time.After(10 * time.Second):
		t.Fatal("Run tidak selesai")
	}
	close(lepaskan)

	if !strings.Contains(logs.String(), "diputus paksa") {
		t.Errorf("pemutusan paksa tidak tercatat: %s", logs.String())
	}
}
