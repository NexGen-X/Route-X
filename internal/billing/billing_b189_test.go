package billing

import (
	"context"
	"testing"
	"time"

	"github.com/NexGen-X/Route-X/internal/database/repo/policy"
	"github.com/NexGen-X/Route-X/internal/database/repo/upstream"
)

// TestTahanBawaanMasihTerhitungLewatBatasRencanaB189 adalah pengunci
// B-189: dengan TTL BAWAAN (tanpa WithHoldTTL — konfigurasi produksi di
// cmd/ai-gateway), tahanan permintaan yang masih sah berjalan WAJIB tetap
// terhitung sesudah 3 menit (di atas TTL lama 2 menit, di bawah batas
// total rencana gateway 5 menit). Pada TTL lama tahanan pertama sudah
// gugur di titik ini dan tahanan kedua lolos — pagu $1.00 terlampaui
// $1.60 oleh dua permintaan paralel yang lambat. Tahanan tetap wajib
// gugur sendiri pada akhirnya (jendela kebocoran terbatas): sesudah
// 11 menit ruang harus bebas lagi.
func TestTahanBawaanMasihTerhitungLewatBatasRencanaB189(t *testing.T) {
	now := time.Date(2026, 11, 6, 10, 0, 0, 0, time.UTC)
	src := &sumber{budgets: []*policy.Budget{
		anggaran("g", policy.ScopeGlobal, "", "1.00", "0", nil),
	}}
	e := NewEnforcer(src, nil, WithClock(func() time.Time { return now }))

	est := upstream.MustParseUSD("0.80")
	if v := e.Tahan(context.Background(), nil, est); v.Blocked {
		t.Fatal("tahanan pertama ditolak padahal anggaran kosong")
	}

	now = now.Add(3 * time.Minute)
	if v := e.Tahan(context.Background(), nil, est); !v.Blocked {
		t.Error("tahanan kedua lolos pada menit ke-3: tahanan pertama sudah gugur " +
			"sebelum spent-nya mendarat — pagu terlampaui (B-189)")
	}

	now = now.Add(8 * time.Minute) // total 11 menit sejak tahanan pertama
	if v := e.Tahan(context.Background(), nil, est); v.Blocked {
		t.Error("tahanan tetap dihitung sesudah 11 menit — TTL bawaan tidak lagi membatasi kebocoran")
	}
}
