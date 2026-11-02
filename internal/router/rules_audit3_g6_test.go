package router

import (
	"fmt"
	"testing"
	"time"

	"github.com/NexGen-X/Route-X/internal/database/repo/upstream"
)

// Test regresi audit3 grup 6 (B-028): nama aturan yang menabrak nama model
// tidak lagi melewatkan MatchModelID, dan penghitung round robin basi
// disapu.

// TestFirstMatchNamaAturanTidakMembajakModelLain memverifikasi V-BR-10:
// aturan berprioritas rendah yang NAMANYA persis nama sebuah model, dengan
// MatchModelID menunjuk model lain, tidak boleh melayani lalu lintas model
// itu — aturan yang benar untuk model itu yang menang.
func TestFirstMatchNamaAturanTidakMembajakModelLain(t *testing.T) {
	tepat := &Rule{Name: "aturan-m10", Priority: 1, MatchModelID: "m10"}
	pembajak := &Rule{Name: "m10", Priority: 99, MatchModelID: "m1"}
	rules := []*Rule{tepat, pembajak}

	req := Request{ModelID: "m10", ModelName: "m10"}
	if pembajak.Matches(req) {
		t.Fatal("aturan bernama sama dengan model lolos Matches walau MatchModelID-nya model lain")
	}
	got := FirstMatch(rules, req)
	if got != tepat {
		t.Fatalf("FirstMatch = %v, mau aturan tepat (prioritas 1, MatchModelID m10)", got)
	}
}

// TestMatchesNamaAturanTanpaSyaratModelTetapBisaDipanggil memastikan
// perbaikan V-BR-10 tidak membunuh jalur memanggil aturan dari namanya
// untuk aturan yang memang tidak menyatakan syarat model: namanya tetap
// target eksplisit di FirstMatch, dan Matches-nya lolos karena
// MatchModelID kosong berarti tidak membatasi.
func TestMatchesNamaAturanTanpaSyaratModelTetapBisaDipanggil(t *testing.T) {
	r := &Rule{Name: "rute-khusus", Priority: 50}
	if !r.Matches(Request{ModelID: "m10", ModelName: "rute-khusus"}) {
		t.Fatal("aturan tanpa MatchModelID ditolak untuk permintaan yang menyebut namanya")
	}
}

// TestSelectorSapuPenghitungRRBasi memverifikasi V-LN-18: penghitung
// round robin milik kunci yang sudah lama tidak dipakai dibuang saat peta
// melewati ambang sapu, sementara penghitung yang masih segar bertahan.
func TestSelectorSapuPenghitungRRBasi(t *testing.T) {
	s := NewSelector()
	cands := []*upstream.RouteCandidate{{ProviderName: "a"}, {ProviderName: "b"}}

	// Satu kunci yang akan dibuat basi.
	s.putar("kunci-basi", cands)
	v, ok := s.rr.Load("kunci-basi")
	if !ok {
		t.Fatal("penghitung kunci-basi tidak terbentuk")
	}
	v.(*penghitungRR).terakhir.Store(time.Now().Add(-2 * time.Hour).UnixNano())

	// Banjiri kunci segar sampai melewati ambang sapu.
	for i := 0; i < ambangSapuRR+16; i++ {
		s.putar(fmt.Sprintf("kunci-segar-%d", i), cands)
	}

	if _, ok := s.rr.Load("kunci-basi"); ok {
		t.Error("penghitung basi masih ada sesudah penyapuan")
	}
	if _, ok := s.rr.Load("kunci-segar-0"); !ok {
		t.Error("penghitung segar ikut terbuang oleh penyapuan")
	}
	if _, ok := s.rr.Load(fmt.Sprintf("kunci-segar-%d", ambangSapuRR+15)); !ok {
		t.Error("penghitung terbaru tidak ada")
	}
}
