package traffic

import (
	"testing"
	"time"

	"github.com/NexGen-X/Route-X/internal/database/repo"
)

// TestB190PenandaBelumBerhargaRoundTripDanAgregat adalah pengunci B-190
// di lapisan repo: penanda unpriced pada baris requests harus bertahan
// lewat Insert → List/Get, terhitung di Totals.UnpricedRequests dari
// sumber mentah, dan terbawa rollup jam ke sumber usage_hourly — supaya
// angka "belum berharga" di dasbor sama dari jendela pendek maupun
// panjang.
func TestB190PenandaBelumBerhargaRoundTripDanAgregat(t *testing.T) {
	ctx, _, r := testEnv(t)

	belum := sampleRecord()
	belum.RequestID = "req-b190-belum"
	belum.Unpriced = true
	belum.CostUSD = 0
	if _, _, err := r.Insert(ctx, belum); err != nil {
		t.Fatalf("Insert baris belum berharga: %v", err)
	}

	sudah := sampleRecord()
	sudah.RequestID = "req-b190-sudah"
	sudah.Unpriced = false
	if _, _, err := r.Insert(ctx, sudah); err != nil {
		t.Fatalf("Insert baris berharga: %v", err)
	}

	// List membawa penandanya.
	hasil, err := r.List(ctx, Filter{}, repo.Page{Limit: 50})
	if err != nil {
		t.Fatalf("List: %v", err)
	}
	penanda := map[string]bool{}
	var idBelum string
	for _, row := range hasil.Rows {
		penanda[row.RequestID] = row.Unpriced
		if row.RequestID == "req-b190-belum" {
			idBelum = row.ID
		}
	}
	if !penanda["req-b190-belum"] {
		t.Errorf("baris req-b190-belum kehilangan penanda Unpriced di List")
	}
	if penanda["req-b190-sudah"] {
		t.Errorf("baris req-b190-sudah tertandai Unpriced di List")
	}

	// Detail (inspector) membawa penanda yang sama.
	det, err := r.Get(ctx, idBelum, time.Time{})
	if err != nil {
		t.Fatalf("Get: %v", err)
	}
	if !det.Unpriced {
		t.Errorf("Detail req-b190-belum tidak Unpriced")
	}

	// Ringkasan dari sumber mentah menghitung tepat satu.
	jendela := Query{
		From:   time.Now().Add(-time.Hour),
		To:     time.Now().Add(time.Hour),
		Source: SourceRequests,
	}
	mentah, err := r.Summary(ctx, jendela)
	if err != nil {
		t.Fatalf("Summary mentah: %v", err)
	}
	if mentah.UnpricedRequests != 1 {
		t.Errorf("UnpricedRequests mentah = %d, mau 1", mentah.UnpricedRequests)
	}

	// Rollup jam berjalan, lalu sumber hourly melaporkan angka yang sama.
	if _, err := r.RollupHourly(ctx, time.Now()); err != nil {
		t.Fatalf("RollupHourly: %v", err)
	}
	jendela.Source = SourceHourly
	perJam, err := r.Summary(ctx, jendela)
	if err != nil {
		t.Fatalf("Summary hourly: %v", err)
	}
	if perJam.UnpricedRequests != 1 {
		t.Errorf("UnpricedRequests hourly = %d, mau 1 (rollup tidak membawa unpriced_count)", perJam.UnpricedRequests)
	}
}
