package health

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"
)

func quietLogger() *slog.Logger {
	return slog.New(slog.NewJSONHandler(&bytes.Buffer{}, nil))
}

// okChecker selalu sehat.
func okChecker(name string) Checker {
	return NewChecker(name, func(context.Context) error { return nil })
}

// failChecker selalu gagal dengan pesan yang memuat detail sensitif, untuk menguji
// bahwa detail itu tidak pernah sampai ke respons HTTP.
func failChecker(name string) Checker {
	return NewChecker(name, func(context.Context) error {
		return errors.New("dial tcp db-internal.rahasia.local:5432: connection refused")
	})
}

func doRequest(t *testing.T, h http.HandlerFunc, path string) *httptest.ResponseRecorder {
	t.Helper()
	rec := httptest.NewRecorder()
	h(rec, httptest.NewRequest(http.MethodGet, path, nil))
	return rec
}

// Liveness tidak boleh bergantung pada dependensi: database mati bukan alasan
// me-restart proses gateway.
func TestLiveIgnoresDependencies(t *testing.T) {
	h := New("1.2.3", "abc1234", quietLogger(), failChecker("postgres"), failChecker("redis"))

	rec := doRequest(t, h.Live, "/healthz")
	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d, mau 200 walau dependensi mati", rec.Code)
	}

	var body liveResponse
	if err := json.Unmarshal(rec.Body.Bytes(), &body); err != nil {
		t.Fatalf("body bukan JSON: %v\n%s", err, rec.Body.String())
	}
	if body.Status != StatusOK {
		t.Errorf("status = %q, mau %q", body.Status, StatusOK)
	}
	if body.Version != "1.2.3" || body.Commit != "abc1234" {
		t.Errorf("version/commit = %q/%q", body.Version, body.Commit)
	}
	if body.UptimeSec < 0 {
		t.Errorf("uptime negatif: %d", body.UptimeSec)
	}
}

func TestReadyAllHealthy(t *testing.T) {
	h := New("1.0.0", "c0ffee", quietLogger(), okChecker("postgres"), okChecker("redis"))

	rec := doRequest(t, h.Ready, "/readyz")
	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d, mau 200", rec.Code)
	}

	var body readyResponse
	if err := json.Unmarshal(rec.Body.Bytes(), &body); err != nil {
		t.Fatalf("body bukan JSON: %v", err)
	}
	if body.Status != StatusOK {
		t.Errorf("status = %q, mau %q", body.Status, StatusOK)
	}
	if len(body.Checks) != 2 {
		t.Fatalf("jumlah check = %d, mau 2: %v", len(body.Checks), body.Checks)
	}
	for name, res := range body.Checks {
		if res.Status != StatusOK {
			t.Errorf("check %q status = %q, mau %q", name, res.Status, StatusOK)
		}
	}
}

func TestReadyReturns503WhenAnyDependencyDown(t *testing.T) {
	h := New("1.0.0", "c0ffee", quietLogger(), okChecker("postgres"), failChecker("redis"))

	rec := doRequest(t, h.Ready, "/readyz")
	if rec.Code != http.StatusServiceUnavailable {
		t.Fatalf("status = %d, mau 503", rec.Code)
	}

	var body readyResponse
	if err := json.Unmarshal(rec.Body.Bytes(), &body); err != nil {
		t.Fatalf("body bukan JSON: %v", err)
	}
	if body.Status != StatusDown {
		t.Errorf("status keseluruhan = %q, mau %q", body.Status, StatusDown)
	}
	if body.Checks["postgres"].Status != StatusOK {
		t.Error("postgres seharusnya tetap ok")
	}
	if body.Checks["redis"].Status != StatusDown {
		t.Error("redis seharusnya down")
	}
}

// Detail kegagalan dependensi hanya boleh ada di log, tidak di respons HTTP: /readyz
// sering terbuka ke probe load balancer dan tidak boleh membocorkan topologi internal.
func TestReadyResponseHidesErrorDetail(t *testing.T) {
	var logBuf bytes.Buffer
	logger := slog.New(slog.NewJSONHandler(&logBuf, nil))

	h := New("1.0.0", "c0ffee", logger, failChecker("postgres"))
	rec := doRequest(t, h.Ready, "/readyz")

	for _, leak := range []string{"db-internal", "rahasia.local", "5432", "connection refused"} {
		if strings.Contains(rec.Body.String(), leak) {
			t.Errorf("respons membocorkan %q: %s", leak, rec.Body.String())
		}
	}

	// Tetapi operator harus bisa menemukannya di log.
	if !strings.Contains(logBuf.String(), "connection refused") {
		t.Errorf("detail kegagalan tidak tercatat di log: %s", logBuf.String())
	}
	if !strings.Contains(logBuf.String(), "postgres") {
		t.Errorf("log tidak menyebut dependensi yang gagal: %s", logBuf.String())
	}
}

// Pemeriksaan dijalankan paralel: total waktu harus mendekati pemeriksaan terlambat,
// bukan jumlah seluruhnya.
func TestReadyChecksRunInParallel(t *testing.T) {
	const delay = 120 * time.Millisecond

	slow := func(name string) Checker {
		return NewChecker(name, func(ctx context.Context) error {
			select {
			case <-time.After(delay):
				return nil
			case <-ctx.Done():
				return ctx.Err()
			}
		})
	}

	h := New("1.0.0", "c0ffee", quietLogger(), slow("a"), slow("b"), slow("c"), slow("d"))

	start := time.Now()
	rec := doRequest(t, h.Ready, "/readyz")
	elapsed := time.Since(start)

	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d, mau 200", rec.Code)
	}
	// Serial akan memakan ~480ms; ambang 2x delay memberi kelonggaran tanpa
	// melemahkan maksud test.
	if elapsed > 2*delay {
		t.Errorf("4 pemeriksaan %v selesai dalam %v — sepertinya berjalan serial", delay, elapsed)
	}
}

// Dependensi yang menggantung tidak boleh membuat probe menggantung selamanya.
func TestReadyAppliesTimeout(t *testing.T) {
	hang := NewChecker("hang", func(ctx context.Context) error {
		<-ctx.Done() // menunggu sampai context dibatalkan
		return ctx.Err()
	})

	h := New("1.0.0", "c0ffee", quietLogger(), hang)

	start := time.Now()
	rec := doRequest(t, h.Ready, "/readyz")
	elapsed := time.Since(start)

	if rec.Code != http.StatusServiceUnavailable {
		t.Errorf("status = %d, mau 503", rec.Code)
	}
	if elapsed > checkTimeout+time.Second {
		t.Errorf("probe butuh %v, melebihi timeout %v", elapsed, checkTimeout)
	}
}

func TestReadyWithoutCheckers(t *testing.T) {
	h := New("1.0.0", "c0ffee", quietLogger())

	rec := doRequest(t, h.Ready, "/readyz")
	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d, mau 200 saat tidak ada dependensi", rec.Code)
	}

	var body readyResponse
	if err := json.Unmarshal(rec.Body.Bytes(), &body); err != nil {
		t.Fatalf("body bukan JSON: %v", err)
	}
	if len(body.Checks) != 0 {
		t.Errorf("checks = %v, mau kosong", body.Checks)
	}
}

func TestNewCheckerName(t *testing.T) {
	c := NewChecker("postgres", func(context.Context) error { return nil })
	if c.Name() != "postgres" {
		t.Errorf("Name() = %q, mau \"postgres\"", c.Name())
	}
	if err := c.Check(context.Background()); err != nil {
		t.Errorf("Check() = %v, mau nil", err)
	}
}

// New harus tetap aman dipakai walau logger nil.
func TestNewWithNilLogger(t *testing.T) {
	h := New("1.0.0", "c0ffee", nil, failChecker("x"))
	if rec := doRequest(t, h.Ready, "/readyz"); rec.Code != http.StatusServiceUnavailable {
		t.Errorf("status = %d, mau 503", rec.Code)
	}
}

func TestRoutes(t *testing.T) {
	h := New("1.0.0", "c0ffee", quietLogger(), okChecker("postgres"))
	mux := http.NewServeMux()
	h.Routes(mux)

	for path, want := range map[string]int{"/healthz": 200, "/readyz": 200} {
		rec := httptest.NewRecorder()
		mux.ServeHTTP(rec, httptest.NewRequest(http.MethodGet, path, nil))
		if rec.Code != want {
			t.Errorf("%s status = %d, mau %d", path, rec.Code, want)
		}
	}
}
