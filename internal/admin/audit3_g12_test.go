package admin

import (
	"errors"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/NexGen-X/Route-X/internal/database/repo"
	"github.com/NexGen-X/Route-X/internal/database/repo/traffic"
	"github.com/NexGen-X/Route-X/internal/database/repo/upstream"
)

// Grup 12 audit3 — kejujuran dashboard: kontrak rentang waktu, validasi
// parameter agregasi, dan total breakdown.

func TestParseTimeRangeGrup12(t *testing.T) {
	minta := func(query string) *http.Request {
		return httptest.NewRequest("GET", "/api/admin/observability/summary?"+query, nil)
	}

	t.Run("window dihormati, bukan bawaan 24 jam", func(t *testing.T) {
		sebelum := time.Now().UTC()
		from, to, err := parseTimeRange(minta("window=1h"))
		if err != nil {
			t.Fatalf("err = %v", err)
		}
		if d := to.Sub(from); d != time.Hour {
			t.Errorf("rentang = %v, mau tepat 1 jam", d)
		}
		if from.Before(sebelum.Add(-time.Hour-time.Second)) || to.Before(sebelum) {
			t.Errorf("rentang tidak berlabuh ke sekarang: %v..%v", from, to)
		}
	})

	t.Run("alias time_range diterima", func(t *testing.T) {
		from, to, err := parseTimeRange(minta("time_range=15m"))
		if err != nil {
			t.Fatalf("err = %v", err)
		}
		if d := to.Sub(from); d != 15*time.Minute {
			t.Errorf("rentang = %v, mau 15 menit", d)
		}
	})

	t.Run("from/to eksplisit menang atas window", func(t *testing.T) {
		from, to, err := parseTimeRange(minta("window=1h&from=2026-01-01T00:00:00Z&to=2026-01-02T00:00:00Z"))
		if err != nil {
			t.Fatalf("err = %v", err)
		}
		if !from.Equal(time.Date(2026, 1, 1, 0, 0, 0, 0, time.UTC)) || !to.Equal(time.Date(2026, 1, 2, 0, 0, 0, 0, time.UTC)) {
			t.Errorf("rentang = %v..%v", from, to)
		}
	})

	t.Run("nilai cacat adalah error sentinel, bukan bawaan diam-diam", func(t *testing.T) {
		for _, q := range []string{
			"window=abc",
			"window=0h",
			"from=bukan-waktu",
			"to=2026-13-01T00:00:00Z",
			"from=2026-01-02T00:00:00Z&to=2026-01-01T00:00:00Z",
			"from=2026-01-01T00:00:00Z&to=2026-01-01T00:00:00Z",
		} {
			if _, _, err := parseTimeRange(minta(q)); !errors.Is(err, repo.ErrInvalidParameter) {
				t.Errorf("%q: err = %v, mau ErrInvalidParameter", q, err)
			}
		}
	})

	t.Run("tanpa parameter tetap bawaan 24 jam", func(t *testing.T) {
		from, to, err := parseTimeRange(minta(""))
		if err != nil {
			t.Fatalf("err = %v", err)
		}
		if d := to.Sub(from); d != 24*time.Hour {
			t.Errorf("rentang = %v, mau 24 jam", d)
		}
	})
}

// Handler divalidasi di batas HTTP: parameter tak dikenal menjawab 400
// invalid_parameter TANPA menyentuh repo (Handlers{} tanpa repo akan panik
// bila validasi bocor ke lapisan repo).
func TestValidasiParameterAgregatGrup12(t *testing.T) {
	h := &Handlers{}
	kasus := []struct {
		nama    string
		handler func(http.ResponseWriter, *http.Request)
		url     string
	}{
		{"summary source asing", h.getSummary, "/x?source=per_menit"},
		{"summary window cacat", h.getSummary, "/x?window=abc"},
		{"series granularity asing", h.getSeries, "/x?granularity=per_detik"},
		{"breakdown dimension asing", h.getBreakdown, "/x?dimension=region"},
		{"breakdown alias by asing", h.getBreakdown, "/x?by=region"},
		{"breakdown limit cacat", h.getBreakdown, "/x?limit=banyak"},
		{"breakdown limit nol", h.getBreakdown, "/x?limit=0"},
		{"requests status_class asing", h.listRequests, "/x?status_class=aneh"},
		{"requests status_code bukan angka", h.listRequests, "/x?status_code=abc"},
		{"requests status_code di luar rentang", h.listRequests, "/x?status_code=99"},
		{"requests from cacat", h.listRequests, "/x?from=kemarin"},
	}
	for _, k := range kasus {
		t.Run(k.nama, func(t *testing.T) {
			rec := httptest.NewRecorder()
			k.handler(rec, httptest.NewRequest("GET", k.url, nil))
			if rec.Code != http.StatusBadRequest {
				t.Errorf("status = %d, mau 400 (body: %s)", rec.Code, rec.Body.String())
			}
		})
	}
}

func TestSusunIrisanBreakdownGrup12(t *testing.T) {
	// Sepuluh kelompok teratas menjumlah 60 dari total keseluruhan 100:
	// persen harus berbasis 100 dan sisa 40 menjadi baris "Lainnya".
	slices := make([]traffic.Slice, 0, 10)
	for i := 0; i < 10; i++ {
		slices = append(slices, traffic.Slice{
			Totals: traffic.Totals{Requests: 6, Success: 5, Errors: 1, TotalTokens: 60},
			ID:     "p",
			Name:   "p",
		})
	}
	total := traffic.Totals{
		Requests:    100,
		Success:     90,
		Errors:      10,
		TotalTokens: 1000,
		CostUSD:     upstream.USD(1000),
	}

	items := susunIrisanBreakdown(slices, total)
	if len(items) != 11 {
		t.Fatalf("len(items) = %d, mau 11 (10 + Lainnya)", len(items))
	}
	if items[0].Percentage != 6 {
		t.Errorf("persen irisan pertama = %v, mau 6 (dari total 100, bukan subtotal 60)", items[0].Percentage)
	}
	lainnya := items[10]
	if lainnya.ID != idIrisanLainnya || lainnya.Name != "Lainnya" {
		t.Errorf("baris terakhir = %q/%q, mau Lainnya", lainnya.ID, lainnya.Name)
	}
	if lainnya.Requests != 40 || lainnya.Percentage != 40 {
		t.Errorf("Lainnya = %d req / %v%%, mau 40 / 40", lainnya.Requests, lainnya.Percentage)
	}
	if lainnya.TotalTokens != 400 {
		t.Errorf("token Lainnya = %d, mau 400", lainnya.TotalTokens)
	}

	t.Run("tanpa sisa tidak ada baris Lainnya", func(t *testing.T) {
		penuh := []traffic.Slice{{Totals: traffic.Totals{Requests: 100}, ID: "p", Name: "p"}}
		items := susunIrisanBreakdown(penuh, total)
		if len(items) != 1 {
			t.Fatalf("len(items) = %d, mau 1", len(items))
		}
		if items[0].Percentage != 100 {
			t.Errorf("persen = %v, mau 100", items[0].Percentage)
		}
	})
}
