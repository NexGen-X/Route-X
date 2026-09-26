package apikey

import (
	"testing"
)

// BenchmarkHash mengukur performa hashing kunci API format rx-... dengan algoritma SHA-256.
func BenchmarkHash(b *testing.B) {
	const key = "rx-live-9f83ab29c84e170d5a3e118902ac78de9b43f10"

	b.ResetTimer()
	b.ReportAllocs()
	for i := 0; i < b.N; i++ {
		_ = Hash(key)
	}
}

// BenchmarkFormatKey mengukur efisiensi formatting dan masking hint kunci API
// untuk pembuatan representasi visual/audit yang aman.
func BenchmarkFormatKey(b *testing.B) {
	const key = "rx-live-9f83ab29c84e170d5a3e118902ac78de9b43f10"

	b.ResetTimer()
	b.ReportAllocs()
	for i := 0; i < b.N; i++ {
		_ = FormatKey(key)
	}
}
