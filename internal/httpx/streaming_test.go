package httpx

import (
	"bufio"
	"context"
	"errors"
	"fmt"
	"io"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/NexGen-X/Route-X/internal/config"
)

// readSSEEvent membaca satu event SSE, yaitu blok baris sampai baris kosong.
func readSSEEvent(br *bufio.Reader) (string, error) {
	var event strings.Builder
	for {
		line, err := br.ReadString('\n')
		if err != nil {
			if event.Len() > 0 && errors.Is(err, io.EOF) {
				return event.String(), nil
			}
			return "", err
		}
		if line == "\n" || line == "\r\n" {
			return event.String(), nil
		}
		event.WriteString(line)
	}
}

// gatewayChain menyusun rantai middleware seperti yang akan dipakai main.go.
func gatewayChain(cfg *config.Config, logger *slog.Logger) func(http.Handler) http.Handler {
	return Chain(
		RequestID(),
		RealIP(PrivateProxyPrefixes()),
		AccessLog(logger),
		Recover(logger),
		SecurityHeaders(cfg),
		MaxBytes(cfg.MaxRequestBytes),
	)
}

// TestRantaiLengkapMenjagaStreamingSSE adalah test terpenting di paket ini.
//
// Seluruh middleware dirangkai di depan handler SSE, lalu setiap event harus sampai
// ke klien SATU PER SATU. Buktinya berupa handshake: handler menolak menulis event
// berikutnya sebelum klien mengonfirmasi event sebelumnya. Kalau ada middleware yang
// menahan respons di buffer sampai handler selesai, klien tidak akan pernah menerima
// event pertama, handler tidak akan pernah dapat konfirmasi, dan test gagal karena
// timeout — bukan lolos diam-diam.
func TestRantaiLengkapMenjagaStreamingSSE(t *testing.T) {
	const jumlahEvent = 4
	const payload = `{"model":"gpt-4o","stream":true}`

	logger, logs := newTestLogger()
	cfg := testConfig(config.EnvProduction)

	konfirmasi := make(chan struct{}, jumlahEvent)
	kegagalan := make(chan error, 8)

	handler := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		body, err := io.ReadAll(r.Body)
		if err != nil {
			kegagalan <- fmt.Errorf("membaca body: %w", err)
			return
		}
		if string(body) != payload {
			kegagalan <- fmt.Errorf("body %q, want %q", body, payload)
			return
		}
		if err := PrepareSSE(w, r); err != nil {
			kegagalan <- fmt.Errorf("PrepareSSE: %w", err)
			return
		}

		rc := http.NewResponseController(w)
		for i := range jumlahEvent {
			if _, err := fmt.Fprintf(w, "data: {\"index\":%d}\n\n", i); err != nil {
				kegagalan <- fmt.Errorf("menulis event %d: %w", i, err)
				return
			}
			// Jalur inilah yang wajib tetap hidup di balik wrapper.
			if err := rc.Flush(); err != nil {
				kegagalan <- fmt.Errorf("flush event %d: %w", i, err)
				return
			}
			select {
			case <-konfirmasi:
			case <-time.After(5 * time.Second):
				kegagalan <- fmt.Errorf("event %d tidak dikonfirmasi klien; respons tertahan di buffer", i)
				return
			}
		}
		if _, err := io.WriteString(w, "data: [DONE]\n\n"); err != nil {
			kegagalan <- fmt.Errorf("menulis penutup: %w", err)
			return
		}
		if err := rc.Flush(); err != nil {
			kegagalan <- fmt.Errorf("flush penutup: %w", err)
		}
	})

	selesai, selesaiCh := chainCompletion()
	srv := httptest.NewServer(selesai(gatewayChain(cfg, logger)(handler)))
	defer srv.Close()

	req, err := http.NewRequestWithContext(context.Background(), http.MethodPost,
		srv.URL+"/v1/chat/completions", strings.NewReader(payload))
	if err != nil {
		t.Fatalf("menyiapkan request: %v", err)
	}
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Authorization", "Bearer sk_live_rahasia123")

	resp, err := srv.Client().Do(req)
	if err != nil {
		t.Fatalf("mengirim request: %v", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		t.Fatalf("status %d, want 200", resp.StatusCode)
	}
	if ct := resp.Header.Get("Content-Type"); ct != "text/event-stream" {
		t.Fatalf("Content-Type %q, want text/event-stream", ct)
	}
	// Respons yang tertahan sampai akhir akan punya Content-Length, bukan chunked.
	if len(resp.TransferEncoding) == 0 || resp.TransferEncoding[0] != "chunked" {
		t.Fatalf("TransferEncoding %v, want [chunked]; respons tidak dialirkan", resp.TransferEncoding)
	}
	if resp.Header.Get(HeaderRequestID) == "" {
		t.Error("X-Request-Id hilang dari respons stream")
	}
	if resp.Header.Get("Content-Security-Policy") == "" {
		t.Error("header keamanan hilang dari respons stream")
	}
	if resp.Header.Get("Strict-Transport-Security") == "" {
		t.Error("HSTS hilang dari respons stream di mode produksi")
	}

	events := make(chan string, jumlahEvent+2)
	bacaErr := make(chan error, 1)
	go func() {
		br := bufio.NewReader(resp.Body)
		for {
			event, err := readSSEEvent(br)
			if err != nil {
				bacaErr <- err
				return
			}
			events <- event
		}
	}()

	terima := func(t *testing.T, urutan int) string {
		t.Helper()
		select {
		case event := <-events:
			return event
		case err := <-bacaErr:
			t.Fatalf("stream berakhir sebelum event ke-%d: %v", urutan, err)
		case <-time.After(3 * time.Second):
			t.Fatalf("event ke-%d tidak sampai dalam 3s: respons tidak dialirkan bertahap", urutan)
		}
		return ""
	}

	for i := range jumlahEvent {
		got := terima(t, i)
		want := fmt.Sprintf("data: {\"index\":%d}", i)
		if strings.TrimSpace(got) != want {
			t.Fatalf("event ke-%d = %q, want %q", i, got, want)
		}
		konfirmasi <- struct{}{}
	}

	if got := strings.TrimSpace(terima(t, jumlahEvent)); got != "data: [DONE]" {
		t.Fatalf("event penutup %q, want \"data: [DONE]\"", got)
	}

	select {
	case err := <-bacaErr:
		if !errors.Is(err, io.EOF) {
			t.Fatalf("stream berakhir dengan error: %v", err)
		}
	case <-time.After(3 * time.Second):
		t.Fatal("stream tidak ditutup setelah event penutup")
	}

	close(kegagalan)
	for err := range kegagalan {
		t.Errorf("handler melaporkan kegagalan: %v", err)
	}

	waitDone(t, selesaiCh, 3*time.Second)
	out := logs.String()
	if !strings.Contains(out, `"status":200`) || !strings.Contains(out, "duration_ms") {
		t.Errorf("access log stream tidak lengkap: %s", out)
	}
	if strings.Contains(out, "rahasia123") {
		t.Errorf("access log membocorkan kredensial: %s", out)
	}
}

// TestResponseControllerLengkapDiBalikRantai membuktikan Unwrap benar-benar
// terpasang: tanpa itu setiap kemampuan di bawah ini mengembalikan ErrNotSupported
// begitu ada middleware yang membungkus writer.
func TestResponseControllerLengkapDiBalikRantai(t *testing.T) {
	logger, _ := newTestLogger()
	cfg := testConfig(config.EnvDevelopment)
	hasil := make(chan error, 1)

	handler := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		rc := http.NewResponseController(w)
		if err := rc.SetReadDeadline(time.Time{}); err != nil {
			hasil <- fmt.Errorf("SetReadDeadline: %w", err)
			return
		}
		if err := rc.SetWriteDeadline(time.Time{}); err != nil {
			hasil <- fmt.Errorf("SetWriteDeadline: %w", err)
			return
		}
		if err := rc.EnableFullDuplex(); err != nil {
			hasil <- fmt.Errorf("EnableFullDuplex: %w", err)
			return
		}
		if _, err := io.WriteString(w, "ok"); err != nil {
			hasil <- fmt.Errorf("menulis: %w", err)
			return
		}
		if err := rc.Flush(); err != nil {
			hasil <- fmt.Errorf("Flush: %w", err)
			return
		}
		hasil <- nil
	})

	srv := httptest.NewServer(gatewayChain(cfg, logger)(handler))
	defer srv.Close()

	resp, err := srv.Client().Get(srv.URL + "/v1/models")
	if err != nil {
		t.Fatalf("request: %v", err)
	}
	defer resp.Body.Close()
	if _, err := io.Copy(io.Discard, resp.Body); err != nil {
		t.Fatalf("membaca body: %v", err)
	}

	select {
	case err := <-hasil:
		if err != nil {
			t.Fatalf("kemampuan ResponseController hilang di balik middleware: %v", err)
		}
	case <-time.After(5 * time.Second):
		t.Fatal("handler tidak melaporkan hasil")
	}
}

// TestPrepareSSEBertahanMelewatiReadTimeout menutup jebakan yang paling mudah
// terlewat: dengan ReadTimeout aktif, net/http membatalkan context request begitu
// tenggat baca habis — walaupun yang sedang berjalan adalah penulisan respons.
// PrepareSSE melepas tenggat itu, jadi completion yang lebih panjang dari
// READ_TIMEOUT tetap utuh.
func TestPrepareSSEBertahanMelewatiReadTimeout(t *testing.T) {
	const readTimeout = 200 * time.Millisecond

	logger, _ := newTestLogger()
	cfg := testConfig(config.EnvDevelopment)
	cfg.ReadTimeout = readTimeout
	cfg.WriteTimeout = readTimeout

	lanjut := make(chan struct{})
	ctxErr := make(chan error, 1)

	handler := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if _, err := io.ReadAll(r.Body); err != nil {
			ctxErr <- fmt.Errorf("membaca body: %w", err)
			return
		}
		if err := PrepareSSE(w, r); err != nil {
			ctxErr <- fmt.Errorf("PrepareSSE: %w", err)
			return
		}
		rc := http.NewResponseController(w)
		if _, err := io.WriteString(w, "data: mulai\n\n"); err != nil {
			ctxErr <- err
			return
		}
		if err := rc.Flush(); err != nil {
			ctxErr <- err
			return
		}

		<-lanjut // klien menunggu lebih lama dari ReadTimeout di sini

		if _, err := io.WriteString(w, "data: [DONE]\n\n"); err != nil {
			ctxErr <- fmt.Errorf("menulis setelah tenggat: %w", err)
			return
		}
		if err := rc.Flush(); err != nil {
			ctxErr <- fmt.Errorf("flush setelah tenggat: %w", err)
			return
		}
		ctxErr <- r.Context().Err()
	})

	s := NewServer(cfg, gatewayChain(cfg, logger)(handler), logger)
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	runErr := make(chan error, 1)
	go func() { runErr <- s.Run(ctx) }()

	addr := waitForListen(t, s)

	resp, err := http.Post("http://"+addr+"/v1/chat/completions", "application/json",
		strings.NewReader(`{"stream":true}`))
	if err != nil {
		t.Fatalf("request: %v", err)
	}
	defer resp.Body.Close()

	br := bufio.NewReader(resp.Body)
	if event, err := readSSEEvent(br); err != nil || strings.TrimSpace(event) != "data: mulai" {
		t.Fatalf("event pertama %q, err %v", event, err)
	}

	// Diam lebih lama dari ReadTimeout dan WriteTimeout sekaligus.
	time.Sleep(4 * readTimeout)
	close(lanjut)

	event, err := readSSEEvent(br)
	if err != nil {
		t.Fatalf("event setelah melewati tenggat gagal dibaca: %v", err)
	}
	if strings.TrimSpace(event) != "data: [DONE]" {
		t.Fatalf("event penutup %q", event)
	}

	select {
	case err := <-ctxErr:
		if err != nil {
			t.Fatalf("context request dibatalkan di tengah stream: %v", err)
		}
	case <-time.After(5 * time.Second):
		t.Fatal("handler tidak melaporkan status context")
	}

	cancel()
	select {
	case err := <-runErr:
		if err != nil {
			t.Fatalf("Run: %v", err)
		}
	case <-time.After(10 * time.Second):
		t.Fatal("Run tidak selesai")
	}
}
