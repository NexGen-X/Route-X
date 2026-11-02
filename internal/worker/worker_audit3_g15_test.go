package worker

import (
	"context"
	"testing"
	"time"
)

type jobUji struct{ tenggat time.Duration }

func (j jobUji) Name() string                  { return "uji" }
func (j jobUji) Run(context.Context) error     { return nil }
func (j jobUji) TenggatPutaran() time.Duration { return j.tenggat }

// TestTenggatPutaran membuktikan audit3 V-LN-10: tenggat satu putaran job
// diturunkan dari interval dengan lantai 5 menit, dan job boleh menentukan
// tenggatnya sendiri lewat JobBertenggat.
func TestTenggatPutaran(t *testing.T) {
	// Interval pendek: lantai 5 menit menang atas 2× interval.
	if d := tenggatPutaran(jobUji{}, time.Minute); d != 5*time.Minute {
		t.Fatalf("interval 1m: tenggat = %v, mau 5m", d)
	}
	// Interval panjang: 2× interval.
	if d := tenggatPutaran(jobUji{}, time.Hour); d != 2*time.Hour {
		t.Fatalf("interval 1j: tenggat = %v, mau 2j", d)
	}
	// JobBertenggat menang atas bawaan.
	if d := tenggatPutaran(jobUji{tenggat: 42 * time.Second}, time.Hour); d != 42*time.Second {
		t.Fatalf("tenggat sendiri: = %v, mau 42d", d)
	}
	// Tenggat sendiri yang tidak sah (<=0) jatuh ke bawaan.
	if d := tenggatPutaran(jobUji{tenggat: 0}, time.Hour); d != 2*time.Hour {
		t.Fatalf("tenggat sendiri 0: = %v, mau 2j", d)
	}
}
