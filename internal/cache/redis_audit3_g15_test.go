package cache

import (
	"context"
	"net/url"
	"testing"

	"github.com/redis/go-redis/v9"
)

// TestMaxRetriesNolEksplisitDinonaktifkan membuktikan audit3 V-LN-13:
// max_retries=0 yang ditulis EKSPLISIT di DSN berarti "jangan retry" (-1),
// bukan bawaan go-redis.
func TestMaxRetriesNolEksplisitDinonaktifkan(t *testing.T) {
	opt := &redis.Options{}
	q := url.Values{"max_retries": {"0"}}
	applyProductionDefaults(opt, q)
	if opt.MaxRetries != -1 {
		t.Fatalf("max_retries=0 eksplisit harus menjadi -1 (nonaktif), dapat %d", opt.MaxRetries)
	}
}

// TestMaxRetriesAbsenPakaiBawaan: tanpa penyebutan di DSN, bawaan produksi
// tetap berlaku.
func TestMaxRetriesAbsenPakaiBawaan(t *testing.T) {
	opt := &redis.Options{}
	applyProductionDefaults(opt, nil)
	if opt.MaxRetries != defaultMaxRetries {
		t.Fatalf("max_retries absen harus memakai bawaan %d, dapat %d", defaultMaxRetries, opt.MaxRetries)
	}
}

// TestPingKlienNilTidakPanic membuktikan audit3 V-LN-01 (sisi cache):
// Ping pada Redis tanpa klien mengembalikan error, bukan panic.
func TestPingKlienNilTidakPanic(t *testing.T) {
	r := &Redis{}
	if err := r.Ping(context.Background()); err == nil {
		t.Fatal("Ping tanpa klien harus mengembalikan error")
	}
	var rn *Redis
	if err := rn.Ping(context.Background()); err == nil {
		t.Fatal("Ping pada penerima nil harus mengembalikan error")
	}
}
