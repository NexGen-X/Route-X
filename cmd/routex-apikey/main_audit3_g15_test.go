package main

import "testing"

// TestCreateOptionsExpiresDays membuktikan audit3 V-LN-32: validasi flag
// -expires-days selaras jalur admin (0 = tanpa kedaluwarsa, atau 1–730).
func TestCreateOptionsExpiresDays(t *testing.T) {
	for _, tc := range []struct {
		hari int
		sah  bool
	}{
		{0, true},
		{1, true},
		{730, true},
		{731, false},
		{-1, false},
	} {
		o := createOptions{name: "uji", expiresDays: tc.hari}
		err := o.validate()
		if tc.sah && err != nil {
			t.Errorf("expiresDays=%d harus sah, dapat error %v", tc.hari, err)
		}
		if !tc.sah && err == nil {
			t.Errorf("expiresDays=%d harus ditolak", tc.hari)
		}
	}
}
