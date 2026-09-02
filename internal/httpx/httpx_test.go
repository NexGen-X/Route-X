package httpx

import (
	"bytes"
	"log/slog"
	"net"
	"net/http"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/NexGen-X/Route-X/internal/config"
	"github.com/NexGen-X/Route-X/internal/observability"
)

// syncBuffer menampung keluaran log dengan aman: handler menulis dari goroutine
// server sementara test membaca dari goroutine test, dan `go test -race` akan
// mengeluh kalau aksesnya tidak dilindungi.
type syncBuffer struct {
	mu  sync.Mutex
	buf bytes.Buffer
}

func (b *syncBuffer) Write(p []byte) (int, error) {
	b.mu.Lock()
	defer b.mu.Unlock()
	return b.buf.Write(p)
}

func (b *syncBuffer) String() string {
	b.mu.Lock()
	defer b.mu.Unlock()
	return b.buf.String()
}

func newTestLogger() (*slog.Logger, *syncBuffer) {
	buf := &syncBuffer{}
	return observability.NewLogger(buf, observability.LoggerOptions{Level: slog.LevelDebug}), buf
}

// testConfig memakai Port 0 supaya sistem operasi memilih port bebas, dan
// WriteTimeout 0 supaya SSE tidak terpotong.
func testConfig(env config.Env) *config.Config {
	return &config.Config{
		AppEnv:          env,
		Port:            0,
		MaxRequestBytes: 1 << 20,
		ShutdownGrace:   3 * time.Second,
		ReadTimeout:     0,
		WriteTimeout:    0,
	}
}

// chainCompletion mengembalikan middleware penanda selesai untuk dipasang paling
// luar. Tanpa ini test bisa membaca buffer log sebelum AccessLog — yang berjalan
// setelah handler — selesai menulis.
func chainCompletion() (func(http.Handler) http.Handler, <-chan struct{}) {
	done := make(chan struct{})
	var once sync.Once
	mw := func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			defer once.Do(func() { close(done) })
			next.ServeHTTP(w, r)
		})
	}
	return mw, done
}

func waitDone(t *testing.T, done <-chan struct{}, timeout time.Duration) {
	t.Helper()
	select {
	case <-done:
	case <-time.After(timeout):
		t.Fatalf("rantai middleware tidak selesai dalam %s", timeout)
	}
}

// unknownLengthBody menyembunyikan panjang body dari httptest.NewRequest sehingga
// ContentLength menjadi -1, meniru request chunked yang panjangnya baru diketahui
// saat dibaca.
type unknownLengthBody struct{ reader *strings.Reader }

func newUnknownLengthBody(payload string) *unknownLengthBody {
	return &unknownLengthBody{reader: strings.NewReader(payload)}
}

func (b *unknownLengthBody) Read(p []byte) (int, error) { return b.reader.Read(p) }

// recordingWriter mencatat setiap pemanggilan WriteHeader, yang tidak bisa diamati
// lewat httptest.ResponseRecorder karena recorder mengunci status pertama.
type recordingWriter struct {
	header http.Header
	codes  []int
	body   bytes.Buffer
}

func newRecordingWriter() *recordingWriter {
	return &recordingWriter{header: make(http.Header)}
}

func (w *recordingWriter) Header() http.Header { return w.header }

func (w *recordingWriter) Write(p []byte) (int, error) { return w.body.Write(p) }

func (w *recordingWriter) WriteHeader(code int) { w.codes = append(w.codes, code) }

// waitForListen menunggu Run berhasil bind lalu mengembalikan alamat yang bisa
// dihubungi klien (loopback, bukan alamat wildcard hasil bind).
func waitForListen(t *testing.T, s *Server) string {
	t.Helper()
	deadline := time.Now().Add(5 * time.Second)
	for time.Now().Before(deadline) {
		addr := s.Addr()
		if _, port, err := net.SplitHostPort(addr); err == nil && port != "0" {
			target := net.JoinHostPort("127.0.0.1", port)
			conn, err := net.DialTimeout("tcp", target, 250*time.Millisecond)
			if err == nil {
				_ = conn.Close()
				return target
			}
		}
		time.Sleep(5 * time.Millisecond)
	}
	t.Fatal("server tidak siap menerima koneksi dalam 5s")
	return ""
}
