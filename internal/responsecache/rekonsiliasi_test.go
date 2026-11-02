package responsecache

import (
	"context"
	"testing"
	"time"
)

// TestRekonsiliasiJumlahEntriMemperbaikiKedaluwarsaTTL membuktikan sisa
// B-023 tertutup: entri yang kedaluwarsa alami oleh TTL mengurangi jumlah
// yang dilaporkan Stats() SESUDAH rekonsiliasi berjalan — sebelumnya
// counter ringkas meta:entry_count tidak pernah berkurang sampai Flush,
// jadi total_entries berbohong lebih besar dari isi Redis.
func TestRekonsiliasiJumlahEntriMemperbaikiKedaluwarsaTTL(t *testing.T) {
	e, mr := newTestEngine(t)
	ctx := context.Background()

	entri := func(teks string) *Entry {
		return &Entry{Model: "gpt-5", Teks: teks}
	}

	// Tiga entri ber-TTL pendek: semuanya kedaluwarsa alami.
	e.SetSettings(true, time.Second)
	for _, k := range []string{"k1", "k2", "k3"} {
		if err := e.Set(ctx, k, entri("isi "+k)); err != nil {
			t.Fatalf("Set %s: %v", k, err)
		}
	}
	st, err := e.Stats(ctx)
	if err != nil {
		t.Fatalf("Stats: %v", err)
	}
	if st.TotalEntries != 3 {
		t.Fatalf("TotalEntries sesudah 3 Set = %d, mau 3", st.TotalEntries)
	}

	// TTL lewat: kunci entri hilang dari Redis, tetapi counter ringkas
	// masih menyebut 3 — inilah penyimpangan yang direkonsiliasi.
	mr.FastForward(2 * time.Second)
	st, err = e.Stats(ctx)
	if err != nil {
		t.Fatalf("Stats: %v", err)
	}
	if st.TotalEntries != 3 {
		t.Fatalf("TotalEntries sebelum rekonsiliasi = %d, mau 3 (counter belum terkoreksi)", st.TotalEntries)
	}

	n, err := e.RekonsiliasiJumlahEntri(ctx)
	if err != nil {
		t.Fatalf("RekonsiliasiJumlahEntri: %v", err)
	}
	if n != 0 {
		t.Fatalf("hasil rekonsiliasi = %d, mau 0", n)
	}
	st, err = e.Stats(ctx)
	if err != nil {
		t.Fatalf("Stats: %v", err)
	}
	if st.TotalEntries != 0 {
		t.Fatalf("TotalEntries sesudah rekonsiliasi = %d, mau 0", st.TotalEntries)
	}

	// Koreksi parsial: satu entri ber-TTL pendek dan satu ber-TTL panjang.
	// Counter menyebut 2, isi Redis sebenarnya 1 sesudah yang pendek
	// kedaluwarsa; rekonsiliasi harus mendarat di 1, bukan 0.
	if err := e.Set(ctx, "pendek", entri("pendek")); err != nil {
		t.Fatalf("Set pendek: %v", err)
	}
	e.SetSettings(true, time.Hour)
	if err := e.Set(ctx, "panjang", entri("panjang")); err != nil {
		t.Fatalf("Set panjang: %v", err)
	}
	mr.FastForward(2 * time.Second)

	n, err = e.RekonsiliasiJumlahEntri(ctx)
	if err != nil {
		t.Fatalf("RekonsiliasiJumlahEntri kedua: %v", err)
	}
	if n != 1 {
		t.Fatalf("hasil rekonsiliasi kedua = %d, mau 1", n)
	}
	st, err = e.Stats(ctx)
	if err != nil {
		t.Fatalf("Stats: %v", err)
	}
	if st.TotalEntries != 1 {
		t.Fatalf("TotalEntries akhir = %d, mau 1", st.TotalEntries)
	}

	// Entri yang masih hidup tidak tersentuh rekonsiliasi.
	if _, hit, err := e.Get(ctx, "panjang"); err != nil || !hit {
		t.Fatalf("Get panjang sesudah rekonsiliasi: hit=%v err=%v, mau hit", hit, err)
	}
}

// TestRekonsiliasiJumlahEntriTanpaRedis membuktikan engine tanpa Redis
// adalah no-op yang aman, sama seperti jalur engine lain.
func TestRekonsiliasiJumlahEntriTanpaRedis(t *testing.T) {
	e := NewEngine(nil, nil)
	n, err := e.RekonsiliasiJumlahEntri(context.Background())
	if err != nil || n != 0 {
		t.Fatalf("RekonsiliasiJumlahEntri tanpa redis = %d, %v; mau 0, nil", n, err)
	}
}
