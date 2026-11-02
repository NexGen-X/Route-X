package security

import (
	"bytes"
	"errors"
	"testing"
)

// --- B-044 / V-LN-30: masa tenggang dekripsi kunci lama ----------------------

// Selama masa tenggang, ciphertext berkunci lama tetap bisa didekripsi,
// enkripsi baru selalu memakai kunci utama, dan kunci ketiga tetap ditolak.
func TestCipherKunciWarisan(t *testing.T) {
	kunciLama := bytes.Repeat([]byte{0x21}, KeyLen)
	kunciBaru := bytes.Repeat([]byte{0x22}, KeyLen)
	kunciAsing := bytes.Repeat([]byte{0x23}, KeyLen)

	cipherLama, err := NewCipher(kunciLama)
	if err != nil {
		t.Fatalf("NewCipher(lama): %v", err)
	}
	const aad = "provider_credential:uji-warisan"
	ctLama, err := cipherLama.Encrypt([]byte("rahasia-lama"), aad)
	if err != nil {
		t.Fatalf("Encrypt(lama): %v", err)
	}

	grace, err := NewCipherWithLegacy(kunciBaru, kunciLama)
	if err != nil {
		t.Fatalf("NewCipherWithLegacy: %v", err)
	}

	// Ciphertext lama terbaca selama masa tenggang.
	pt, err := grace.Decrypt(ctLama, aad)
	if err != nil {
		t.Fatalf("Decrypt warisan: %v", err)
	}
	if string(pt) != "rahasia-lama" {
		t.Errorf("plaintext = %q, mau %q", pt, "rahasia-lama")
	}
	if !grace.CanDecrypt(ctLama) {
		t.Error("CanDecrypt(ciphertext lama) = false selama masa tenggang")
	}

	// Enkripsi baru memakai kunci utama: cipher lama murni tidak bisa membacanya,
	// cipher masa tenggang bisa.
	ctBaru, err := grace.Encrypt([]byte("rahasia-baru"), aad)
	if err != nil {
		t.Fatalf("Encrypt(baru): %v", err)
	}
	if _, err := cipherLama.Decrypt(ctBaru, aad); !errors.Is(err, ErrKeyMismatch) {
		t.Errorf("Decrypt oleh kunci lama = %v, mau ErrKeyMismatch", err)
	}
	if pt, err := grace.Decrypt(ctBaru, aad); err != nil || string(pt) != "rahasia-baru" {
		t.Errorf("Decrypt baru = %q, %v", pt, err)
	}

	// Kunci ketiga yang tidak terdaftar tetap ditolak.
	cipherAsing, err := NewCipher(kunciAsing)
	if err != nil {
		t.Fatalf("NewCipher(asing): %v", err)
	}
	ctAsing, err := cipherAsing.Encrypt([]byte("rahasia-asing"), aad)
	if err != nil {
		t.Fatalf("Encrypt(asing): %v", err)
	}
	if _, err := grace.Decrypt(ctAsing, aad); !errors.Is(err, ErrKeyMismatch) {
		t.Errorf("Decrypt asing = %v, mau ErrKeyMismatch", err)
	}
	if grace.CanDecrypt(ctAsing) {
		t.Error("CanDecrypt(ciphertext asing) = true, mau false")
	}

	// Tanpa kunci warisan, ciphertext lama ditolak seperti sebelumnya.
	ketat, err := NewCipher(kunciBaru)
	if err != nil {
		t.Fatalf("NewCipher(baru): %v", err)
	}
	if _, err := ketat.Decrypt(ctLama, aad); !errors.Is(err, ErrKeyMismatch) {
		t.Errorf("Decrypt tanpa warisan = %v, mau ErrKeyMismatch", err)
	}
}

// Kunci warisan yang sama dengan kunci utama adalah salah konfigurasi yang
// menyembunyikan rotasi yang belum terjadi — ditolak saat konstruksi.
func TestCipherKunciWarisanSamaDenganUtama(t *testing.T) {
	kunci := bytes.Repeat([]byte{0x24}, KeyLen)
	if _, err := NewCipherWithLegacy(kunci, kunci); err == nil {
		t.Error("NewCipherWithLegacy(kunci, kunci yang sama) harus error")
	}
	if _, err := NewCipherWithLegacy(kunci, bytes.Repeat([]byte{0x25}, 16)); err == nil {
		t.Error("kunci warisan 16 byte harus error")
	}
	// Nol kunci warisan setara NewCipher biasa.
	if _, err := NewCipherWithLegacy(kunci); err != nil {
		t.Errorf("NewCipherWithLegacy tanpa warisan: %v", err)
	}
	// Kunci warisan kosong (env tidak disetel) dilewati, bukan error.
	if _, err := NewCipherWithLegacy(kunci, nil); err != nil {
		t.Errorf("NewCipherWithLegacy warisan nil: %v", err)
	}
}
