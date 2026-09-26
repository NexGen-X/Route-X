package ratelimit

import (
	"testing"
)

// BenchmarkMemoryLimiter_Allow mengukur throughput evaluasi token bucket per request
// pada kondisi single-thread dengan kapasitas yang memadai.
func BenchmarkMemoryLimiter_Allow(b *testing.B) {
	// Limiter dengan laju dan kapasitas besar agar evaluasi Allow murni mengukur
	// hot-path evaluasi waktu, penambahan token, dan mutasi internal tanpa I/O.
	limiter := NewMemoryLimiter(1e9, 1e9)

	b.ResetTimer()
	b.ReportAllocs()
	for i := 0; i < b.N; i++ {
		_ = limiter.Allow()
	}
}

// BenchmarkMemoryLimiter_AllowParallel mengukur throughput concurrency token bucket
// di bawah tekanan multi-goroutine simultan (b.RunParallel).
func BenchmarkMemoryLimiter_AllowParallel(b *testing.B) {
	limiter := NewMemoryLimiter(1e9, 1e9)

	b.ResetTimer()
	b.ReportAllocs()
	b.RunParallel(func(pb *testing.PB) {
		for pb.Next() {
			_ = limiter.Allow()
		}
	})
}
