package keys

import (
	"testing"
	"time"

	"github.com/NexGen-X/Route-X/internal/database/repo"
)

// TestListPaginasiTidakMenghilangkanBarisSatuTransaksi mereproduksi
// audit3 V-DT-22: now() adalah waktu transaksi, jadi key yang dibuat dalam
// satu transaksi berbagi created_at yang identik. Dengan kursor waktu-saja,
// halaman berhenti tepat di nilai seri itu dan seluruh baris seri di
// belakangnya tidak pernah terjangkau halaman mana pun.
func TestListPaginasiTidakMenghilangkanBarisSatuTransaksi(t *testing.T) {
	ctx, pool, r := testEnv(t)
	owner := newUser(t, ctx, pool)

	const jumlah = 30
	terbuat := make(map[string]bool, jumlah)
	for i := 0; i < jumlah; i++ {
		c := mustCreate(t, ctx, r, CreateParams{Name: "kunci massal", OwnerUserID: owner, Live: true})
		terbuat[c.Key.ID] = true
	}
	// Paksa seluruhnya ke satu created_at yang sama persis, meniru satu
	// transaksi impor: inilah kondisi yang membunuh kursor lama.
	saat := time.Date(2026, 10, 5, 8, 30, 0, 123456000, time.UTC)
	if _, err := pool.Exec(ctx, `update api_keys set created_at = $1 where owner_user_id = $2`, saat, owner); err != nil {
		t.Fatalf("menyamakan created_at: %v", err)
	}

	terlihat := make(map[string]bool, jumlah)
	kursor := ""
	for halaman := 0; ; halaman++ {
		if halaman > 20 {
			t.Fatal("paginasi tidak kunjung selesai")
		}
		out, next, err := r.List(ctx, ListFilter{OwnerUserID: owner}, repo.Page{Limit: 7, Cursor: kursor})
		if err != nil {
			t.Fatalf("List halaman %d: %v", halaman, err)
		}
		for _, k := range out {
			if terlihat[k.ID] {
				t.Errorf("key %s muncul dua kali", k.ID)
			}
			terlihat[k.ID] = true
		}
		if next == "" {
			break
		}
		kursor = next
	}
	if len(terlihat) != jumlah {
		t.Errorf("key terlihat = %d, mau %d — ada baris yang hilang di paginasi", len(terlihat), jumlah)
	}
	for id := range terbuat {
		if !terlihat[id] {
			t.Errorf("key %s tidak pernah muncul di halaman mana pun", id)
		}
	}
}

// TestListPencarianWildcardHarfiah memastikan teks pencarian diperlakukan
// harfiah (audit3 V-DT-23): "%" yang diketik operator bukan wildcard yang
// mengembalikan seluruh tabel.
func TestListPencarianWildcardHarfiah(t *testing.T) {
	ctx, pool, r := testEnv(t)
	owner := newUser(t, ctx, pool)

	pertama := mustCreate(t, ctx, r, CreateParams{Name: "promo 100% spesial", OwnerUserID: owner, Live: true})
	mustCreate(t, ctx, r, CreateParams{Name: "promo 1000 biasa", OwnerUserID: owner, Live: true})
	mustCreate(t, ctx, r, CreateParams{Name: "reguler", OwnerUserID: owner, Live: true})

	cari := func(search string) []*Key {
		t.Helper()
		out, _, err := r.List(ctx, ListFilter{OwnerUserID: owner, Search: search}, repo.Page{Limit: 50})
		if err != nil {
			t.Fatalf("List pencarian %q: %v", search, err)
		}
		return out
	}

	// "100%" harfiah hanya cocok ke nama yang benar-benar memuat "100%";
	// tanpa pelolosan, pola ILIKE-nya ikut menangkap "promo 1000 biasa".
	if got := cari("100%"); len(got) != 1 || got[0].Name != "promo 100% spesial" {
		names := make([]string, len(got))
		for i, k := range got {
			names[i] = k.Name
		}
		t.Errorf("pencarian %q = %v, mau hanya [promo 100%% spesial]", "100%", names)
	}
	// "%" saja: tanpa pelolosan ia adalah "cocokkan semua" (3 baris).
	if got := cari("%"); len(got) != 1 || got[0].Name != "promo 100% spesial" {
		t.Errorf("pencarian %% mengembalikan %d baris, mau 1 (yang namanya memuat persen harfiah)", len(got))
	}
	// "_" juga wildcard satu karakter; tidak ada nama yang memuatnya.
	if got := cari("_"); len(got) != 0 {
		t.Errorf("pencarian _ mengembalikan %d baris, mau 0", len(got))
	}
	// Cabang kesetaraan last4 memakai nilai MENTAH: mencarinya harus tetap
	// menemukan kuncinya (argumen ter-escape tidak boleh bocor ke cabang =).
	if got := cari(pertama.Key.Last4); len(got) == 0 {
		t.Error("pencarian berdasarkan last4 tidak menemukan kunci — cabang kesetaraan rusak")
	}
}

// TestCursorKeysetKeys menguji format kursor bertuple dan penerimaan kursor
// warisan waktu-saja.
func TestCursorKeysetKeys(t *testing.T) {
	ts := time.Date(2026, 10, 5, 8, 30, 0, 123456000, time.UTC)
	id := "11111111-1111-1111-1111-111111111111"

	gots, gotID, err := decodeCursor("uji", encodeCursor(ts, id))
	if err != nil {
		t.Fatalf("decodeCursor: %v", err)
	}
	if !gots.Equal(ts) || gotID != id {
		t.Errorf("hasil pecah kursor = (%v, %q), mau (%v, %q)", gots, gotID, ts, id)
	}

	// Kursor warisan (pra-perbaikan) hanya memuat waktu: diterima dengan
	// pengenal kosong supaya paginasi yang sedang berjalan tidak putus.
	gots, gotID, err = decodeCursor("uji", ts.Format(time.RFC3339Nano))
	if err != nil {
		t.Fatalf("decodeCursor warisan: %v", err)
	}
	if !gots.Equal(ts) || gotID != "" {
		t.Errorf("kursor warisan = (%v, %q), mau (%v, kosong)", gots, gotID, ts)
	}

	if _, _, err := decodeCursor("uji", "bukan-kursor-sama-sekali"); err == nil {
		t.Error("kursor rusak diterima")
	}
	if _, _, err := decodeCursor("uji", ts.Format(time.RFC3339Nano)+"|"); err == nil {
		t.Error("kursor dengan pengenal kosong diterima")
	}
}
