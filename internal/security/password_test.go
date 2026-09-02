package security

import (
	"errors"
	"strings"
	"sync"
	"testing"

	"golang.org/x/crypto/argon2"
)

// testParams memakai biaya rendah supaya suite test tetap cepat. Parameter produksi
// diuji terpisah lewat weakerThanDefault.
var testParams = Argon2Params{Memory: 8 * 1024, Iterations: 1, Parallelism: 1, SaltLength: 16, KeyLength: 32}

func TestHashPasswordFormat(t *testing.T) {
	hash, err := HashPasswordWith("kata-sandi-yang-kuat-123", testParams)
	if err != nil {
		t.Fatalf("HashPasswordWith: %v", err)
	}

	if !strings.HasPrefix(hash, "$argon2id$v=") {
		t.Errorf("prefix hash salah: %s", hash)
	}
	if parts := strings.Split(hash, "$"); len(parts) != 6 {
		t.Errorf("hash punya %d bagian, mau 6: %s", len(parts), hash)
	}
	// Password tidak boleh muncul di hasil.
	if strings.Contains(hash, "kata-sandi") {
		t.Errorf("hash memuat password: %s", hash)
	}
}

// Salt acak: password sama harus menghasilkan hash berbeda.
func TestHashPasswordUsesRandomSalt(t *testing.T) {
	const pw = "kata-sandi-yang-kuat-123"
	seen := make(map[string]struct{}, 20)
	for i := 0; i < 20; i++ {
		h, err := HashPasswordWith(pw, testParams)
		if err != nil {
			t.Fatalf("HashPasswordWith: %v", err)
		}
		if _, dup := seen[h]; dup {
			t.Fatal("hash terulang — salt tidak acak")
		}
		seen[h] = struct{}{}
	}
}

func TestVerifyPassword(t *testing.T) {
	const pw = "kata-sandi-yang-kuat-123"
	hash, err := HashPasswordWith(pw, testParams)
	if err != nil {
		t.Fatalf("HashPasswordWith: %v", err)
	}

	t.Run("password benar cocok", func(t *testing.T) {
		res, err := VerifyPassword(pw, hash)
		if err != nil {
			t.Fatalf("VerifyPassword: %v", err)
		}
		if !res.Match {
			t.Error("Match = false untuk password yang benar")
		}
	})

	t.Run("password salah tidak cocok tanpa error", func(t *testing.T) {
		for _, wrong := range []string{"", "salah", pw + "x", strings.ToUpper(pw), " " + pw} {
			res, err := VerifyPassword(wrong, hash)
			if err != nil {
				t.Errorf("password salah %q menghasilkan error: %v", wrong, err)
			}
			if res.Match {
				t.Errorf("password salah %q dinyatakan cocok", wrong)
			}
		}
	})
}

// Hash dengan biaya di bawah default harus ditandai untuk di-hash ulang, supaya
// peningkatan parameter bisa diterapkan bertahap saat pengguna login.
func TestVerifyPasswordFlagsRehash(t *testing.T) {
	const pw = "kata-sandi-yang-kuat-123"

	weak, err := HashPasswordWith(pw, testParams)
	if err != nil {
		t.Fatalf("HashPasswordWith: %v", err)
	}
	res, err := VerifyPassword(pw, weak)
	if err != nil {
		t.Fatalf("VerifyPassword: %v", err)
	}
	if !res.Match {
		t.Fatal("Match = false")
	}
	if !res.NeedsRehash {
		t.Error("NeedsRehash = false untuk hash berbiaya rendah")
	}

	// Hash dengan parameter default tidak perlu di-hash ulang.
	strong, err := HashPassword(pw)
	if err != nil {
		t.Fatalf("HashPassword: %v", err)
	}
	res, err = VerifyPassword(pw, strong)
	if err != nil {
		t.Fatalf("VerifyPassword: %v", err)
	}
	if !res.Match {
		t.Error("Match = false untuk hash parameter default")
	}
	if res.NeedsRehash {
		t.Error("NeedsRehash = true untuk hash parameter default")
	}
}

func TestVerifyPasswordRejectsBadHashFormat(t *testing.T) {
	tests := []struct {
		name    string
		encoded string
	}{
		{"kosong", ""},
		{"bukan argon2", "$2a$10$abcdefghijklmnopqrstuv"},
		{"algoritma lain", "$argon2i$v=19$m=8192,t=1,p=1$c2FsdA$aGFzaA"},
		{"bagian kurang", "$argon2id$v=19$m=8192,t=1,p=1$c2FsdA"},
		{"bagian berlebih", "$argon2id$v=19$m=8192,t=1,p=1$c2FsdA$aGFzaA$extra"},
		{"parameter rusak", "$argon2id$v=19$m=abc,t=1,p=1$c2FsdA$aGFzaA"},
		{"memory nol", "$argon2id$v=19$m=0,t=1,p=1$c2FsdA$aGFzaA"},
		{"iterasi nol", "$argon2id$v=19$m=8192,t=0,p=1$c2FsdA$aGFzaA"},
		{"salt bukan base64", "$argon2id$v=19$m=8192,t=1,p=1$!!!$aGFzaA"},
		{"hash bukan base64", "$argon2id$v=19$m=8192,t=1,p=1$c2FsdA$!!!"},
		{"salt kosong", "$argon2id$v=19$m=8192,t=1,p=1$$aGFzaA"},
		{"versi rusak", "$argon2id$v=xx$m=8192,t=1,p=1$c2FsdA$aGFzaA"},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			if _, err := VerifyPassword("apa saja", tc.encoded); err == nil {
				t.Error("hash rusak seharusnya menghasilkan error")
			}
		})
	}
}

func TestVerifyPasswordRejectsIncompatibleVersion(t *testing.T) {
	// argon2.Version saat ini 19; pakai versi lain untuk memicu ketidakcocokan.
	other := argon2.Version + 1
	encoded := strings.Replace(
		"$argon2id$v=19$m=8192,t=1,p=1$c2FsdHNhbHRzYWx0c2E$aGFzaGhhc2hoYXNoaGFzaGhhc2hoYXNoaGE",
		"v=19", "v="+itoa(other), 1)

	_, err := VerifyPassword("apa saja", encoded)
	if !errors.Is(err, ErrIncompatibleVersion) {
		t.Errorf("err = %v, mau ErrIncompatibleVersion", err)
	}
}

func itoa(n int) string {
	if n == 0 {
		return "0"
	}
	var b []byte
	for n > 0 {
		b = append([]byte{byte('0' + n%10)}, b...)
		n /= 10
	}
	return string(b)
}

func TestValidatePasswordStrength(t *testing.T) {
	tests := []struct {
		name     string
		password string
		wantErr  error
	}{
		{"kuat", "kata-sandi-yang-kuat-123", nil},
		{"panjang pas dengan dua kelas", "abcdefghij12", nil},
		{"hanya huruf", "abcdefghijklmnop", ErrPasswordNoVariety},
		{"hanya angka", "123456789012345", ErrPasswordNoVariety},
		{"terlalu pendek", "pendek1", ErrPasswordTooShort},
		{"kosong", "", ErrPasswordTooShort},
		{"umum: changeme123", "changeme123", ErrPasswordCommon},
		{"umum beda huruf besar", "ChangeMe123", ErrPasswordCommon},
		{"terlalu panjang", strings.Repeat("a1", MaxPasswordLength), ErrPasswordTooLong},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			err := ValidatePasswordStrength(tc.password)
			if !errors.Is(err, tc.wantErr) {
				t.Errorf("err = %v, mau %v", err, tc.wantErr)
			}
		})
	}
}

// Panjang dihitung per rune, bukan per byte, supaya password non-ASCII tidak ditolak
// atau diterima secara keliru.
func TestValidatePasswordStrengthCountsRunes(t *testing.T) {
	// 12 rune, tetapi lebih dari 12 byte.
	if err := ValidatePasswordStrength("sandiরাহাসz1"); err != nil {
		t.Errorf("password 12 rune ditolak: %v", err)
	}
	// 11 rune multibyte harus tetap ditolak karena kurang dari minimum.
	if err := ValidatePasswordStrength("রাহাসz1"); !errors.Is(err, ErrPasswordTooShort) {
		t.Errorf("err = %v, mau ErrPasswordTooShort", err)
	}
}

func TestHashPasswordRejectsOversized(t *testing.T) {
	huge := strings.Repeat("a", MaxPasswordLength+1)
	if _, err := HashPasswordWith(huge, testParams); !errors.Is(err, ErrPasswordTooLong) {
		t.Errorf("err = %v, mau ErrPasswordTooLong", err)
	}
}

func TestWeakerThanDefault(t *testing.T) {
	d := DefaultArgon2Params
	if weakerThanDefault(d) {
		t.Error("parameter default dianggap lemah")
	}

	stronger := d
	stronger.Memory *= 2
	if weakerThanDefault(stronger) {
		t.Error("parameter lebih kuat dianggap lemah")
	}

	for name, mutate := range map[string]func(*Argon2Params){
		"memory":     func(p *Argon2Params) { p.Memory /= 2 },
		"iterations": func(p *Argon2Params) { p.Iterations-- },
		"keyLength":  func(p *Argon2Params) { p.KeyLength -= 8 },
		"saltLength": func(p *Argon2Params) { p.SaltLength -= 8 },
	} {
		weak := d
		mutate(&weak)
		if !weakerThanDefault(weak) {
			t.Errorf("%s lebih rendah tidak terdeteksi lemah", name)
		}
	}
}

// BurnVerifyTime harus aman dipanggil berulang dan tidak panic.
func TestBurnVerifyTime(t *testing.T) {
	for i := 0; i < 3; i++ {
		BurnVerifyTime()
	}
}

// BurnVerifyTime dipanggil dari jalur login, jadi pasti dipanggil bersamaan. Versi
// pertamanya mengisi hash pembanding secara malas tanpa penyelarasan dan race
// detector melaporkannya; test ini menjaga agar itu tidak kembali.
func TestBurnVerifyTimeConcurrent(t *testing.T) {
	var wg sync.WaitGroup
	for i := 0; i < 8; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			BurnVerifyTime()
		}()
	}
	wg.Wait()
}

func TestWarmUpIsIdempotent(t *testing.T) {
	WarmUp()
	WarmUp()

	var wg sync.WaitGroup
	for i := 0; i < 4; i++ {
		wg.Add(1)
		go func() { defer wg.Done(); WarmUp() }()
	}
	wg.Wait()

	// Sesudah WarmUp, BurnVerifyTime harus benar-benar melakukan verifikasi, bukan
	// keluar lebih awal karena hash pembanding kosong.
	if dummyHash == "" {
		t.Fatal("WarmUp tidak menyiapkan hash pembanding")
	}
}
