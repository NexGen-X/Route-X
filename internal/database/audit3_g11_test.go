package database

import (
	"errors"
	"testing"
)

// TestCheckMigrationContinuityMenolakLompatan memastikan rangkaian migrasi
// yang nomornya melompat ditolak sebelum satu pun diterapkan (audit3 V-DT-15):
// file yang hilang dari biner harus berbunyi sebagai error migrasi, bukan
// sebagai error SQL runtime berminggu-minggu kemudian.
func TestCheckMigrationContinuityMenolakLompatan(t *testing.T) {
	rapat := []migration{
		{version: 1, filename: "0001_a.sql"},
		{version: 2, filename: "0002_b.sql"},
		{version: 3, filename: "0003_c.sql"},
	}
	if err := checkMigrationContinuity(rapat); err != nil {
		t.Errorf("rangkaian rapat ditolak: %v", err)
	}
	if err := checkMigrationContinuity(nil); err != nil {
		t.Errorf("rangkaian kosong ditolak: %v", err)
	}

	for nama, set := range map[string][]migration{
		"lompatan di tengah": {
			{version: 1, filename: "0001_a.sql"},
			{version: 3, filename: "0003_c.sql"},
		},
		"lompatan besar ala fixture urut": {
			{version: 1, filename: "0001_a.sql"},
			{version: 2, filename: "0002_b.sql"},
			{version: 10, filename: "0010_j.sql"},
		},
	} {
		err := checkMigrationContinuity(set)
		if err == nil {
			t.Errorf("%s: rangkaian melompat diterima", nama)
			continue
		}
		if !errors.Is(err, ErrInvalidMigration) {
			t.Errorf("%s: error = %v, mau ErrInvalidMigration", nama, err)
		}
	}
}

// TestMigrasiTertanamBerkesinambungan menjaga properti yang sama pada set
// migrasi SUNGGUHAN yang tertanam di biner: menambah file dengan nomor
// melompat (atau menghapus satu) memerahkan test ini, bukan produksi.
func TestMigrasiTertanamBerkesinambungan(t *testing.T) {
	migs, err := embeddedMigrations()
	if err != nil {
		t.Fatalf("embeddedMigrations: %v", err)
	}
	if len(migs) == 0 {
		t.Fatal("tidak ada migrasi tertanam terbaca")
	}
	if migs[0].version != 1 {
		t.Errorf("migrasi pertama = %d (%s), mau 1", migs[0].version, migs[0].filename)
	}
	if err := checkMigrationContinuity(migs); err != nil {
		t.Errorf("set tertanam tidak berkesinambungan: %v", err)
	}
	if got := migs[len(migs)-1].version; got != 23 {
		t.Errorf("migrasi terakhir = %d, mau 23 (perbarui angka ini saat menambah migrasi)", got)
	}
}
