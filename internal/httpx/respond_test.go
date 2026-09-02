package httpx

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strconv"
	"strings"
	"testing"
	"time"

	"github.com/NexGen-X/Route-X/internal/observability"
)

func TestJSONMenulisHeaderDanBody(t *testing.T) {
	rec := httptest.NewRecorder()
	payload := map[string]string{"object": "list"}

	if err := JSON(rec, http.StatusCreated, payload); err != nil {
		t.Fatalf("JSON: %v", err)
	}

	if rec.Code != http.StatusCreated {
		t.Fatalf("status %d, want 201", rec.Code)
	}
	if ct := rec.Header().Get("Content-Type"); ct != contentTypeJSON {
		t.Errorf("Content-Type %q, want %q", ct, contentTypeJSON)
	}
	body := rec.Body.String()
	if got := rec.Header().Get("Content-Length"); got != strconv.Itoa(len(body)) {
		t.Errorf("Content-Length %q tidak sama dengan panjang body %d", got, len(body))
	}
	if rec.Header().Get("Cache-Control") != "no-store" {
		t.Errorf("respons API harus no-store, dapat %q", rec.Header().Get("Cache-Control"))
	}
	if strings.TrimSpace(body) != `{"object":"list"}` {
		t.Errorf("body %q", body)
	}
}

func TestJSONGagalMarshalTanpaMenulisRespons(t *testing.T) {
	rec := httptest.NewRecorder()
	// Channel tidak bisa di-marshal; kegagalan harus terdeteksi sebelum ada byte
	// terkirim sehingga pemanggil masih bisa membalas 500 yang benar.
	if err := JSON(rec, http.StatusOK, make(chan int)); err == nil {
		t.Fatal("marshal tipe yang tidak didukung harus gagal")
	}
	if rec.Body.Len() != 0 {
		t.Fatalf("tidak boleh ada body yang tertulis, dapat %q", rec.Body.String())
	}
}

// TestWriteErrorBentukKontrak mengunci bentuk envelope: klien OpenAI yang sudah ada
// harus bisa memakai gateway ini hanya dengan menukar base URL, jadi nama, urutan,
// dan nilai null setiap field adalah bagian dari kontrak.
func TestWriteErrorBentukKontrak(t *testing.T) {
	rec := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodPost, "/v1/chat/completions", nil)
	req = req.WithContext(observability.WithRequestID(req.Context(), "req_01hx7q9j8k"))

	WriteError(rec, req, http.StatusBadRequest, ErrTypeInvalidRequest, "model_not_found", "model tidak dikenal")

	const want = `{"error":{"message":"model tidak dikenal","type":"invalid_request_error","code":"model_not_found","param":null,"request_id":"req_01hx7q9j8k"}}`
	if got := strings.TrimSpace(rec.Body.String()); got != want {
		t.Fatalf("envelope tidak sesuai kontrak.\n got: %s\nwant: %s", got, want)
	}
	if rec.Code != http.StatusBadRequest {
		t.Fatalf("status %d, want 400", rec.Code)
	}
}

func TestWriteErrorTanpaRequestIDDiContext(t *testing.T) {
	rec := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodGet, "/v1/models", nil)

	WriteError(rec, req, http.StatusNotFound, ErrTypeInvalidRequest, "", "tidak ada")

	// Field tetap ada meski kosong, supaya bentuk respons tidak berubah-ubah.
	const want = `{"error":{"message":"tidak ada","type":"invalid_request_error","code":null,"param":null,"request_id":""}}`
	if got := strings.TrimSpace(rec.Body.String()); got != want {
		t.Fatalf("envelope %s, want %s", got, want)
	}
}

func TestWriteErrorMemotongPesanTerlaluPanjang(t *testing.T) {
	rec := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodGet, "/", nil)

	WriteError(rec, req, http.StatusBadRequest, ErrTypeInvalidRequest, "x", strings.Repeat("a", maxErrorMessageLen*2))

	var resp ErrorResponse
	if err := json.Unmarshal(rec.Body.Bytes(), &resp); err != nil {
		t.Fatalf("body bukan JSON: %v", err)
	}
	if len(resp.Error.Message) > maxErrorMessageLen+3 {
		t.Fatalf("pesan tidak dipotong: %d karakter", len(resp.Error.Message))
	}
}

func TestHelperError(t *testing.T) {
	tests := []struct {
		name     string
		call     func(w http.ResponseWriter, r *http.Request)
		status   int
		errType  string
		code     string
		wantMsg  string
		noDetail bool
	}{
		{
			name: "BadRequest",
			call: func(w http.ResponseWriter, r *http.Request) {
				BadRequest(w, r, "missing_model", "field model wajib diisi")
			},
			status:  http.StatusBadRequest,
			errType: ErrTypeInvalidRequest,
			code:    "missing_model",
			wantMsg: "field model wajib diisi",
		},
		{
			name:    "BadRequest tanpa kode",
			call:    func(w http.ResponseWriter, r *http.Request) { BadRequest(w, r, "", "") },
			status:  http.StatusBadRequest,
			errType: ErrTypeInvalidRequest,
			code:    CodeInvalidRequest,
			wantMsg: msgBadRequest,
		},
		{
			name:    "Unauthorized",
			call:    func(w http.ResponseWriter, r *http.Request) { Unauthorized(w, r, "") },
			status:  http.StatusUnauthorized,
			errType: ErrTypeAuthentication,
			code:    CodeInvalidAPIKey,
			wantMsg: msgUnauthorized,
		},
		{
			name:    "Forbidden",
			call:    func(w http.ResponseWriter, r *http.Request) { Forbidden(w, r, "") },
			status:  http.StatusForbidden,
			errType: ErrTypePermission,
			code:    CodeInsufficientPermission,
			wantMsg: msgForbidden,
		},
		{
			name:    "NotFound",
			call:    func(w http.ResponseWriter, r *http.Request) { NotFound(w, r, "") },
			status:  http.StatusNotFound,
			errType: ErrTypeInvalidRequest,
			code:    CodeNotFound,
			wantMsg: msgNotFound,
		},
		{
			name:    "TooManyRequests",
			call:    func(w http.ResponseWriter, r *http.Request) { TooManyRequests(w, r, "", 0) },
			status:  http.StatusTooManyRequests,
			errType: ErrTypeRateLimit,
			code:    CodeRateLimitExceeded,
			wantMsg: msgTooManyRequests,
		},
		{
			name:    "PayloadTooLarge",
			call:    func(w http.ResponseWriter, r *http.Request) { PayloadTooLarge(w, r, 10485760) },
			status:  http.StatusRequestEntityTooLarge,
			errType: ErrTypeInvalidRequest,
			code:    CodePayloadTooLarge,
			wantMsg: "ukuran body request melebihi batas 10485760 byte",
		},
		{
			name:     "InternalError",
			call:     InternalError,
			status:   http.StatusInternalServerError,
			errType:  ErrTypeAPI,
			code:     CodeInternalError,
			wantMsg:  msgInternalError,
			noDetail: true,
		},
		{
			name:     "ServiceUnavailable",
			call:     func(w http.ResponseWriter, r *http.Request) { ServiceUnavailable(w, r, 0) },
			status:   http.StatusServiceUnavailable,
			errType:  ErrTypeOverloaded,
			code:     CodeServiceUnavailable,
			wantMsg:  msgServiceUnavailable,
			noDetail: true,
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			rec := httptest.NewRecorder()
			req := httptest.NewRequest(http.MethodPost, "/v1/chat/completions", nil)
			req = req.WithContext(observability.WithRequestID(req.Context(), "req_uji"))

			tc.call(rec, req)

			if rec.Code != tc.status {
				t.Fatalf("status %d, want %d", rec.Code, tc.status)
			}
			var resp ErrorResponse
			if err := json.Unmarshal(rec.Body.Bytes(), &resp); err != nil {
				t.Fatalf("body bukan envelope JSON: %v (%s)", err, rec.Body.String())
			}
			if resp.Error.Type != tc.errType {
				t.Errorf("type %q, want %q", resp.Error.Type, tc.errType)
			}
			if resp.Error.Code == nil || *resp.Error.Code != tc.code {
				t.Errorf("code %v, want %q", resp.Error.Code, tc.code)
			}
			if resp.Error.Message != tc.wantMsg {
				t.Errorf("message %q, want %q", resp.Error.Message, tc.wantMsg)
			}
			if resp.Error.RequestID != "req_uji" {
				t.Errorf("request_id %q, want req_uji", resp.Error.RequestID)
			}
			if resp.Error.Param != nil {
				t.Errorf("param harus null, dapat %q", *resp.Error.Param)
			}
			if tc.noDetail {
				// Pesan 5xx tidak boleh membocorkan sumber kegagalan.
				for _, bocor := range []string{"pq:", "pgx", "sql", "panic", "goroutine", ".go:", "redis"} {
					if strings.Contains(strings.ToLower(resp.Error.Message), bocor) {
						t.Errorf("pesan 5xx memuat detail internal %q: %q", bocor, resp.Error.Message)
					}
				}
			}
		})
	}
}

func TestRetryAfter(t *testing.T) {
	tests := []struct {
		retryAfter time.Duration
		want       string
	}{
		{0, ""},
		{-time.Second, ""},
		{500 * time.Millisecond, "1"},
		{1500 * time.Millisecond, "2"},
		{30 * time.Second, "30"},
	}
	for _, tc := range tests {
		rec := httptest.NewRecorder()
		req := httptest.NewRequest(http.MethodPost, "/v1/chat/completions", nil)
		TooManyRequests(rec, req, "", tc.retryAfter)
		if got := rec.Header().Get("Retry-After"); got != tc.want {
			t.Errorf("Retry-After untuk %s = %q, want %q", tc.retryAfter, got, tc.want)
		}
	}
}

func TestPrepareSSEMemasangHeaderStream(t *testing.T) {
	rec := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodPost, "/v1/chat/completions", nil)

	// httptest.ResponseRecorder tidak mendukung tenggat koneksi; PrepareSSE harus
	// memperlakukan itu sebagai bukan kegagalan.
	if err := PrepareSSE(rec, req); err != nil {
		t.Fatalf("PrepareSSE: %v", err)
	}

	want := map[string]string{
		"Content-Type":      "text/event-stream",
		"Cache-Control":     "no-cache, no-transform",
		"Connection":        "keep-alive",
		"X-Accel-Buffering": "no",
	}
	for name, value := range want {
		if got := rec.Header().Get(name); got != value {
			t.Errorf("header %s = %q, want %q", name, got, value)
		}
	}
	if rec.Code != http.StatusOK {
		t.Errorf("status %d, want 200", rec.Code)
	}
	if !rec.Flushed {
		t.Error("header SSE harus langsung dikirim ke klien")
	}
}
