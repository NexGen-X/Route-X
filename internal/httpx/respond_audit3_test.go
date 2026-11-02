package httpx

// Test regresi audit3 grup 3 (B-016 / V-GW-15): kegagalan flush pada
// PrepareSSE terjadi SESUDAH header terkirim dan harus bisa dibedakan
// pemanggil dari kegagalan pra-header, supaya tidak dijawab envelope JSON
// mentah di dalam aliran SSE.

import (
	"errors"
	"net/http"
	"net/http/httptest"
	"testing"
)

// penulisFlushGagal adalah ResponseWriter yang FlushError-nya selalu gagal,
// seperti koneksi klien yang putus tepat saat header SSE dikirim.
type penulisFlushGagal struct {
	header http.Header
	status int
}

func (w *penulisFlushGagal) Header() http.Header {
	if w.header == nil {
		w.header = make(http.Header)
	}
	return w.header
}

func (w *penulisFlushGagal) WriteHeader(status int) { w.status = status }

func (w *penulisFlushGagal) Write(b []byte) (int, error) { return len(b), nil }

func (w *penulisFlushGagal) Flush() {}

func (w *penulisFlushGagal) FlushError() error { return errors.New("koneksi putus saat flush") }

func TestPrepareSSEFlushGagalDitandaiHeaderTerkirim(t *testing.T) {
	w := &penulisFlushGagal{}
	r := httptest.NewRequest(http.MethodPost, "/v1/chat/completions", nil)

	err := PrepareSSE(w, r)
	if err == nil {
		t.Fatal("PrepareSSE = nil, mau error flush")
	}
	if !errors.Is(err, ErrHeaderSSETerkirim) {
		t.Errorf("err = %v, mau terbungkus ErrHeaderSSETerkirim", err)
	}
	if w.status != http.StatusOK {
		t.Errorf("status tertulis = %d, mau 200 — header memang sudah terkirim sebelum flush gagal", w.status)
	}
}
