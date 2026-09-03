package usage

import (
	"testing"
	"time"
)

// Dipangkas alih-alih dibulatkan, permintaan 0,6 ms tercatat 0 ms — dan seluruh permintaan
// cepat menghilang dari histogram latensi, membuat slot pertama menjadi tempat sampah yang
// tidak bisa dibaca.
func TestMilidetikMembulatkanBukanMemangkas(t *testing.T) {
	for _, tc := range []struct {
		d   time.Duration
		mau int
	}{
		{0, 0},
		{-time.Second, 0},
		{600 * time.Microsecond, 1},
		{1499 * time.Microsecond, 1},
		{1500 * time.Microsecond, 2},
		{2 * time.Second, 2000},
	} {
		if got := milidetik(tc.d); got != tc.mau {
			t.Errorf("milidetik(%v) = %d, mau %d", tc.d, got, tc.mau)
		}
	}
}

func TestStatusTerpakaiMenjepitDiLuarRentang(t *testing.T) {
	for _, tc := range []struct{ in, mau int }{
		{0, 500}, {99, 500}, {600, 500}, {-1, 500},
		{100, 100}, {200, 200}, {499, 499}, {599, 599},
	} {
		if got := statusTerpakai(tc.in); got != tc.mau {
			t.Errorf("statusTerpakai(%d) = %d, mau %d", tc.in, got, tc.mau)
		}
	}
}

// routing_decision yang kosong harus menjadi nil supaya kolomnya memakai default '{}',
// bukan objek berisi field kosong yang harus disaring pembacanya.
func TestRoutingJSONKosongMenjadiNil(t *testing.T) {
	if got := (Routing{}).json(); got != nil {
		t.Errorf("Routing kosong = %s, mau nil", got)
	}
	if got := (Routing{Strategy: "priority"}).json(); got == nil {
		t.Error("Routing berisi strategi menjadi nil")
	}
}

func TestAlasanMemakaiPengenalStabil(t *testing.T) {
	// SkipReason adalah kalimat untuk manusia dan bisa diperbaiki kapan saja; label metrik
	// yang berubah memutus setiap grafik yang memakainya.
	if got := alasan(Attempt{SkipReason: "pemutus arus terbuka"}); got != "circuit_open" {
		t.Errorf("alasan pelewatan = %q, mau circuit_open", got)
	}
	if got := alasan(Attempt{ErrorKind: "timeout"}); got != "timeout" {
		t.Errorf("alasan kegagalan = %q, mau timeout", got)
	}
	if got := alasan(Attempt{}); got != "none" {
		t.Errorf("alasan percobaan berhasil = %q, mau none", got)
	}
}
