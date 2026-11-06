package providers

import (
	"testing"
	"time"
)

// TestB318RetryAfterMelewatiEntriRusak adalah pengunci B-318:
// RetryAfterFromDetails dulu berhenti (return 0) pada entri RetryInfo
// PERTAMA yang retryDelay-nya tidak terurai, sehingga petunjuk jeda
// yang sah pada entri berikutnya tidak pernah terbaca. Kini entri
// rusak dilewati dan entri sah pertama yang menang.
func TestB318RetryAfterMelewatiEntriRusak(t *testing.T) {
	details := []ErrorDetail{
		{Type: "type.googleapis.com/google.rpc.RetryInfo", RetryDelay: "rusak"},
		{Type: "type.googleapis.com/google.rpc.RetryInfo", RetryDelay: "7s"},
	}
	if got := RetryAfterFromDetails(details); got != 7*time.Second {
		t.Errorf("RetryAfterFromDetails = %v, mau 7s — entri rusak menghentikan pemindaian", got)
	}
	// Kontrol: tanpa entri sah tetap 0; entri bukan RetryInfo diabaikan.
	if got := RetryAfterFromDetails([]ErrorDetail{{Type: "type.googleapis.com/google.rpc.RetryInfo", RetryDelay: "rusak"}}); got != 0 {
		t.Errorf("kontrol semua-rusak = %v, mau 0", got)
	}
	if got := RetryAfterFromDetails([]ErrorDetail{{Type: "type.googleapis.com/google.rpc.ErrorInfo", RetryDelay: "3s"}}); got != 0 {
		t.Errorf("kontrol bukan-RetryInfo = %v, mau 0", got)
	}
}
