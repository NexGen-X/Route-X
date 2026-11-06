package usage

import (
	"context"
	"testing"
)

// TestRecorderMenandaiBarisBelumBerhargaB190 adalah pengunci B-190 di
// lapisan pencatat: event yang tokennya terpakai tetapi pemetaan
// modelnya tidak punya harga berlaku wajib menghasilkan baris requests
// bertanda Unpriced — supaya dasbor menampilkan UNPRICED, bukan $0 yang
// menyiratkan gratis. Keadaan nol yang sah (berharga, cache HIT) tidak
// boleh ikut tertandai.
func TestRecorderMenandaiBarisBelumBerhargaB190(t *testing.T) {
	t.Run("tanpa harga tertandai", func(t *testing.T) {
		r, store, _, _ := pencatatUji(t)
		ev := eventLengkap()
		ev.ProviderModelID = "pm-tanpa-harga"
		r.Record(context.Background(), ev)
		tutup(t, r)

		baris := store.semua()
		if len(baris) != 1 {
			t.Fatalf("baris tertulis = %d, mau 1", len(baris))
		}
		if baris[0].CostUSD != 0 {
			t.Errorf("cost baris tanpa harga = %d, mau 0", baris[0].CostUSD)
		}
		if !baris[0].Unpriced {
			t.Errorf("baris tanpa harga tidak tertandai Unpriced — dasbor akan menampilkannya sebagai $0")
		}
	})

	t.Run("berharga tidak tertandai", func(t *testing.T) {
		r, store, _, _ := pencatatUji(t)
		r.Record(context.Background(), eventLengkap())
		tutup(t, r)

		baris := store.semua()
		if len(baris) != 1 {
			t.Fatalf("baris tertulis = %d, mau 1", len(baris))
		}
		if baris[0].CostUSD <= 0 {
			t.Errorf("cost baris berharga = %d, mau > 0", baris[0].CostUSD)
		}
		if baris[0].Unpriced {
			t.Errorf("baris berharga tertandai Unpriced")
		}
	})

	t.Run("cache HIT tidak tertandai", func(t *testing.T) {
		r, store, _, _ := pencatatUji(t)
		ev := eventLengkap()
		ev.CacheHit = true
		// Bahkan tanpa harga sekali pun, HIT adalah nol yang sah
		// (B-188), bukan belum berharga.
		ev.ProviderModelID = "pm-tanpa-harga"
		r.Record(context.Background(), ev)
		tutup(t, r)

		baris := store.semua()
		if len(baris) != 1 {
			t.Fatalf("baris tertulis = %d, mau 1", len(baris))
		}
		if baris[0].Unpriced {
			t.Errorf("baris cache HIT tertandai Unpriced — nol HIT adalah desain, bukan ketiadaan harga")
		}
	})
}
