package anthropic

import (
	"context"
	"io"
	"net/http"
	"strings"
	"testing"
	"time"
)

// ulang adalah reader tak berujung yang mengisi buffer dengan satu byte yang
// sama. Dipakai untuk menguji batas body tanpa mengalokasikan 32MB di awal.
type ulang struct{ b byte }

func (u ulang) Read(p []byte) (int, error) {
	for i := range p {
		p[i] = u.b
	}
	return len(p), nil
}

// readAll wajib memakai LimitReader(maxResponseBytes+1) dan mengembalikan
// error eksplisit begitu body melewati batas — bukan memotong diam-diam.
func TestReadAllMenolakMelebihiBatas(t *testing.T) {
	p, _ := newServer(t, jsonHandler(200, textResponse))
	resp := &http.Response{Body: io.NopCloser(ulang{b: 'x'})}
	if _, err := p.readAll(resp); err == nil {
		t.Fatal("readAll tidak error untuk body melebihi batas")
	} else if !strings.Contains(err.Error(), "melebihi batas") {
		t.Errorf("pesan error = %q, mau menyebut batas", err.Error())
	}
}

// Batas health check bawaan wajib 10 detik, dan Timeout konfigurasi yang lebih
// kecil wajib mempersempitnya.
func TestHealthTimeoutBawaan(t *testing.T) {
	if healthCheckTimeout != 10*time.Second {
		t.Errorf("healthCheckTimeout = %v, mau 10s", healthCheckTimeout)
	}
	if got := healthTimeoutFor(0); got != 10*time.Second {
		t.Errorf("healthTimeoutFor(0) = %v, mau 10s", got)
	}
	if got := healthTimeoutFor(3 * time.Second); got != 3*time.Second {
		t.Errorf("healthTimeoutFor(3s) = %v, mau 3s", got)
	}
	if got := healthTimeoutFor(60 * time.Second); got != 10*time.Second {
		t.Errorf("healthTimeoutFor(60s) = %v, mau 10s", got)
	}
}

// ChatCompletionStream wajib menolak respons JSON biasa dengan error yang
// jelas, bukan mengembalikan aliran kosong tanpa penjelasan.
func TestStreamMenolakResponsJSONBiasa(t *testing.T) {
	p, _ := newServer(t, jsonHandler(200, `{"type":"message","content":[]}`))
	st, err := p.ChatCompletionStream(context.Background(), simpleRequest())
	if err == nil {
		st.Close()
		t.Fatal("ChatCompletionStream() menerima respons JSON biasa tanpa error")
	}
	if !strings.Contains(err.Error(), "bukan aliran SSE") {
		t.Errorf("pesan error = %q, mau menyebut bukan-aliran", err.Error())
	}
}

// Close yang dipanggil berulang wajib aman dan tetap membatalkan context
// turunan aliran.
func TestStreamCloseBerulangAman(t *testing.T) {
	p, _ := newServer(t, sseHandler(
		"event: message_start\ndata: {\"type\":\"message_start\",\"message\":{\"id\":\"m1\",\"usage\":{\"input_tokens\":1}}}\n\n",
	))
	st, err := p.ChatCompletionStream(context.Background(), simpleRequest())
	if err != nil {
		t.Fatalf("ChatCompletionStream() error: %v", err)
	}
	if err := st.Close(); err != nil {
		t.Errorf("Close pertama error: %v", err)
	}
	if err := st.Close(); err != nil {
		t.Errorf("Close kedua error: %v", err)
	}
}
