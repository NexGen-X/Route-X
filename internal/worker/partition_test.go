package worker

import (
	"testing"
	"time"
)

// TestCalculatePartitionMonthsNormalisasiAwalBulan menguji bahwa tanggal-tanggal kritis
// di akhir bulan (29, 30, 31 Januari) tidak melompati bulan Februari saat menghitung partisi masa depan.
func TestCalculatePartitionMonthsNormalisasiAwalBulan(t *testing.T) {
	testDates := []struct {
		name string
		now  time.Time
	}{
		{
			name: "31 Januari (kasus kritis overflow ke Maret)",
			now:  time.Date(2026, time.January, 31, 23, 59, 59, 0, time.UTC),
		},
		{
			name: "30 Januari",
			now:  time.Date(2026, time.January, 30, 12, 0, 0, 0, time.UTC),
		},
		{
			name: "29 Januari",
			now:  time.Date(2026, time.January, 29, 8, 30, 0, 0, time.UTC),
		},
		{
			name: "29 Februari Tahun Kabisat",
			now:  time.Date(2024, time.February, 29, 15, 0, 0, 0, time.UTC),
		},
	}

	for _, tc := range testDates {
		t.Run(tc.name, func(t *testing.T) {
			months := CalculatePartitionMonths(tc.now, 2)
			if len(months) != 3 {
				t.Fatalf("diharapkan 3 tanggal partisi (bulan 0, 1, 2), didapat: %d", len(months))
			}

			// Bulan ke-0 harus awal bulan tc.now
			expectedM0 := time.Date(tc.now.Year(), tc.now.Month(), 1, 0, 0, 0, 0, time.UTC)
			if !months[0].Equal(expectedM0) {
				t.Errorf("bulan 0 tidak sesuai: didapat %v, diharapkan %v", months[0], expectedM0)
			}

			// Kasus khusus Januari: bulan ke-1 WAJIB Februari, bulan ke-2 WAJIB Maret
			if tc.now.Month() == time.January {
				expectedFeb := time.Date(tc.now.Year(), time.February, 1, 0, 0, 0, 0, time.UTC)
				expectedMar := time.Date(tc.now.Year(), time.March, 1, 0, 0, 0, 0, time.UTC)

				if !months[1].Equal(expectedFeb) {
					t.Errorf("bulan 1 gagal menghasilkan Februari: didapat %v, diharapkan %v", months[1], expectedFeb)
				}
				if !months[2].Equal(expectedMar) {
					t.Errorf("bulan 2 gagal menghasilkan Maret: didapat %v, diharapkan %v", months[2], expectedMar)
				}
			}
		})
	}
}
