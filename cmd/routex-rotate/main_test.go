package main

import (
	"testing"
)

// Kunci hanya boleh datang dari environment (audit3 V-LN-31): test ini
// mengunci kontrak muatKunciRotasi — kedua variabel wajib ada, dan tidak ada
// jalur lain (flag sudah dihapus dari main).
func TestMuatKunciRotasi(t *testing.T) {
	t.Run("kedua variabel terisi", func(t *testing.T) {
		t.Setenv("OLD_ENCRYPTION_KEY", "kunci-lama-contoh")
		t.Setenv("NEW_ENCRYPTION_KEY", "kunci-baru-contoh")
		lama, baru, err := muatKunciRotasi()
		if err != nil {
			t.Fatalf("muatKunciRotasi: %v", err)
		}
		if lama != "kunci-lama-contoh" || baru != "kunci-baru-contoh" {
			t.Errorf("lama/baru = %q/%q", lama, baru)
		}
	})

	t.Run("salah satu kosong ditolak", func(t *testing.T) {
		t.Setenv("OLD_ENCRYPTION_KEY", "kunci-lama-contoh")
		t.Setenv("NEW_ENCRYPTION_KEY", "")
		if _, _, err := muatKunciRotasi(); err == nil {
			t.Error("muatKunciRotasi tanpa NEW_ENCRYPTION_KEY harus error")
		}
	})

	t.Run("keduanya kosong ditolak", func(t *testing.T) {
		t.Setenv("OLD_ENCRYPTION_KEY", "")
		t.Setenv("NEW_ENCRYPTION_KEY", "")
		if _, _, err := muatKunciRotasi(); err == nil {
			t.Error("muatKunciRotasi tanpa kedua variabel harus error")
		}
	})
}
