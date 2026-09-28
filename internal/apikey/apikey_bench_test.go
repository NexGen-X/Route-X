package apikey

import (
	"testing"
)

// BenchmarkFormatKey mengukur efisiensi formatting dan masking hint kunci API
// untuk pembuatan representasi visual/audit yang aman.
//
// BenchmarkHash dihapus bersama fungsi Hash (SHA-256 tanpa pepper) pada remediasi
// audit: hashing kunci API resmi kini hanya lewat internal/security (HMAC-SHA256 +
// pepper), dan benchmark jalur yang tidak sah tidak diperlukan lagi.
func BenchmarkFormatKey(b *testing.B) {
	const key = "rx-live-9f83ab29c84e170d5a3e118902ac78de9b43f10"

	b.ResetTimer()
	b.ReportAllocs()
	for i := 0; i < b.N; i++ {
		_ = FormatKey(key)
	}
}
