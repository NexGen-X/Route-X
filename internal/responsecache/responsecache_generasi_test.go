package responsecache

import (
	"context"
	"encoding/json"
	"testing"

	"github.com/NexGen-X/Route-X/internal/cache"
)

// Test Fase 20 PR-A: mekanisme generasi cache (B-473/B-474).
//
// Invariant yang dikunci: entri hanya disajikan/ditanam oleh request
// yang cap generasinya sama dengan generasi berjalan. Setiap Flush
// menaikkan generasi; NaikkanGenerasi menggesernya tanpa menghapus.
// Test di sini diskriminatif terhadap perilaku pra-Fase 20, di mana
// Set sesudah Flush selalu menang dan entri pra-kenaikan selalu
// tersaji selama kunci Redis-nya hidup.

func entriUjiGenerasi() *Entry {
	return &Entry{Model: "model-uji", Raw: json.RawMessage(`{"id":"chatcmpl-generasi"}`)}
}

// TestGenerasiSetBerCapDitolakSesudahFlush adalah balapan B-473
// persis: kunci dihitung (MISS + cap generasi), eksekusi berjalan,
// Flush menyela, lalu Set dari request itu tiba. Di kode lama Set
// menanam dan request berikutnya HIT jawaban pra-flush; kini
// penanaman ditolak dan kuncinya tetap MISS.
func TestGenerasiSetBerCapDitolakSesudahFlush(t *testing.T) {
	e, _ := newTestEngine(t)
	ctx := context.Background()
	kunci := "kunci-b473"

	_, capGen, hit, err := e.GetBerCap(ctx, kunci)
	if err != nil {
		t.Fatalf("GetBerCap awal: %v", err)
	}
	if hit {
		t.Fatal("GetBerCap awal harus MISS pada cache kosong")
	}

	if _, err := e.Flush(ctx); err != nil {
		t.Fatalf("Flush: %v", err)
	}

	tertulis, err := e.SetBerCap(ctx, kunci, entriUjiGenerasi(), capGen)
	if err != nil {
		t.Fatalf("SetBerCap cap lama: %v", err)
	}
	if tertulis {
		t.Error("SetBerCap bercap generasi pra-Flush TERTANAM — balapan in-flight B-473 masih menang")
	}
	if _, _, hit, err := e.GetBerCap(ctx, kunci); err != nil || hit {
		t.Errorf("GetBerCap sesudah penolakan: hit=%v err=%v, mau MISS tanpa error", hit, err)
	}

	// Cap generasi yang segar tetap diterima: mekanisme ini menolak
	// yang basi, bukan menutup cache.
	_, capBaru, _, err := e.GetBerCap(ctx, kunci)
	if err != nil {
		t.Fatalf("GetBerCap ulang: %v", err)
	}
	tertulis, err = e.SetBerCap(ctx, kunci, entriUjiGenerasi(), capBaru)
	if err != nil || !tertulis {
		t.Fatalf("SetBerCap cap segar: tertulis=%v err=%v, mau tertanam", tertulis, err)
	}
	if _, _, hit, err := e.GetBerCap(ctx, kunci); err != nil || !hit {
		t.Errorf("GetBerCap sesudah tanam segar: hit=%v err=%v, mau HIT", hit, err)
	}
}

// TestGenerasiEntriTidakDisajikanSesudahGenerasiNaik adalah sisi
// baca B-474: entri yang tertanam pada generasi lama berhenti
// disajikan begitu generasi naik (pergeseran pemenang kredensial),
// walau tidak ada Flush dan kunci Redis-nya masih hidup.
func TestGenerasiEntriTidakDisajikanSesudahGenerasiNaik(t *testing.T) {
	e, mr := newTestEngine(t)
	ctx := context.Background()
	kunci := "kunci-b474-baca"

	if err := e.Set(ctx, kunci, entriUjiGenerasi()); err != nil {
		t.Fatalf("Set: %v", err)
	}
	if _, _, hit, err := e.GetBerCap(ctx, kunci); err != nil || !hit {
		t.Fatalf("GetBerCap sebelum kenaikan: hit=%v err=%v, mau HIT", hit, err)
	}

	if _, err := e.NaikkanGenerasi(ctx); err != nil {
		t.Fatalf("NaikkanGenerasi: %v", err)
	}

	// Kunci Redis entri masih ada secara fisik (kenaikan generasi
	// bukan penghapusan) tetapi tidak boleh lagi disajikan.
	rKey := cache.Key(cache.NamespaceResponseCache, "entry", kunci)
	if !mr.Exists(rKey) {
		t.Fatal("kunci entri hilang dari Redis — NaikkanGenerasi seharusnya tidak menghapus entri")
	}
	if _, _, hit, err := e.GetBerCap(ctx, kunci); err != nil || hit {
		t.Errorf("GetBerCap sesudah kenaikan: hit=%v err=%v, mau MISS", hit, err)
	}
	if _, hit, err := e.Get(ctx, kunci); err != nil || hit {
		t.Errorf("Get sesudah kenaikan: hit=%v err=%v, mau MISS", hit, err)
	}
}

// TestGenerasiFlushMenaikkanGenerasi mengunci bahwa Flush adalah
// salah satu penanda generasi: dua Flush menaikkan generasi dua
// kali, dan kunci generasi sendiri tidak ikut terhapus oleh Flush.
func TestGenerasiFlushMenaikkanGenerasi(t *testing.T) {
	e, _ := newTestEngine(t)
	ctx := context.Background()

	gen, err := e.GenerasiSaatIni(ctx)
	if err != nil || gen != 0 {
		t.Fatalf("generasi awal = %d, %v; mau 0", gen, err)
	}
	if _, err := e.Flush(ctx); err != nil {
		t.Fatalf("Flush #1: %v", err)
	}
	if gen, err = e.GenerasiSaatIni(ctx); err != nil || gen != 1 {
		t.Fatalf("generasi sesudah Flush #1 = %d, %v; mau 1", gen, err)
	}
	if _, err := e.Flush(ctx); err != nil {
		t.Fatalf("Flush #2: %v", err)
	}
	if gen, err = e.GenerasiSaatIni(ctx); err != nil || gen != 2 {
		t.Fatalf("generasi sesudah Flush #2 = %d, %v; mau 2 (kunci generasi tidak boleh ikut terhapus)", gen, err)
	}
}

// TestGenerasiSetBiasaMengikutiGenerasiBerjalan menjaga jalur
// penulis lama (Set tanpa cap): entri dicap generasi yang berjalan
// saat penulisan, jadi ia tetap tersaji sampai generasi bergerak
// lagi — Set bukan jalan memutar gerbang generasi.
func TestGenerasiSetBiasaMengikutiGenerasiBerjalan(t *testing.T) {
	e, _ := newTestEngine(t)
	ctx := context.Background()
	kunci := "kunci-generasi-set"

	if _, err := e.NaikkanGenerasi(ctx); err != nil {
		t.Fatalf("NaikkanGenerasi: %v", err)
	}
	if err := e.Set(ctx, kunci, entriUjiGenerasi()); err != nil {
		t.Fatalf("Set: %v", err)
	}
	entri, _, hit, err := e.GetBerCap(ctx, kunci)
	if err != nil || !hit {
		t.Fatalf("GetBerCap: hit=%v err=%v, mau HIT", hit, err)
	}
	if entri.Generasi != 1 {
		t.Errorf("cap generasi entri = %d, mau 1 (generasi berjalan saat Set)", entri.Generasi)
	}
}
