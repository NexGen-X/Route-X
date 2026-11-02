package admin

import (
	"sync/atomic"
	"testing"
)

// cacheKebijakanPalsu menghitung panggilan Invalidate untuk membuktikan
// wiring triage B-005: mutasi admin harus membatalkan salinan kebijakan
// gateway (batas laju & filter konten) tanpa menunggu TTL.
type cacheKebijakanPalsu struct{ dipanggil atomic.Int32 }

func (c *cacheKebijakanPalsu) Invalidate() { c.dipanggil.Add(1) }

func TestInvalidateKebijakanMendelegasikanKeEngine(t *testing.T) {
	rates := &cacheKebijakanPalsu{}
	filters := &cacheKebijakanPalsu{}
	h := &Handlers{rateLimits: rates, contentFilters: filters}

	h.invalidateRateLimits()
	h.invalidateRateLimits()
	h.invalidateContentFilters()

	if got := rates.dipanggil.Load(); got != 2 {
		t.Errorf("invalidateRateLimits: Invalidate terpanggil %d kali, mau 2", got)
	}
	if got := filters.dipanggil.Load(); got != 1 {
		t.Errorf("invalidateContentFilters: Invalidate terpanggil %d kali, mau 1", got)
	}
}

func TestInvalidateKebijakanAmanTanpaEngine(t *testing.T) {
	h := &Handlers{}
	// Tidak boleh panic: engine memang opsional (lihat Config).
	h.invalidateRateLimits()
	h.invalidateContentFilters()
}
