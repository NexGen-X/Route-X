package security

import (
	"errors"
	"strings"
	"testing"
)

var testPepper = []byte("pepper-uji-yang-panjangnya-cukup-32-byte!")

func TestGenerateAPIKeyFormat(t *testing.T) {
	tests := []struct {
		name       string
		live       bool
		wantPrefix string
	}{
		{"live", true, KeyPrefixLive},
		{"test", false, KeyPrefixTest},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			key, err := GenerateAPIKey(tc.live, testPepper)
			if err != nil {
				t.Fatalf("GenerateAPIKey: %v", err)
			}

			raw := key.Raw.Reveal()
			if !strings.HasPrefix(raw, tc.wantPrefix) {
				t.Errorf("prefix key = %q, mau %q", raw[:8], tc.wantPrefix)
			}
			if key.Prefix != tc.wantPrefix {
				t.Errorf("field Prefix = %q, mau %q", key.Prefix, tc.wantPrefix)
			}
			if got := len(raw) - len(tc.wantPrefix); got != keyBodyLen {
				t.Errorf("panjang bagian acak = %d, mau %d", got, keyBodyLen)
			}

			// Bentuk yang dihasilkan harus bisa diparse kembali.
			if _, err := ParseAPIKey(raw); err != nil {
				t.Errorf("key hasil generate tidak lolos ParseAPIKey: %v", err)
			}

			if !strings.HasSuffix(raw, key.Last4) || len(key.Last4) != last4Len {
				t.Errorf("Last4 = %q tidak cocok dengan akhir key", key.Last4)
			}
			if key.Masked != tc.wantPrefix+"****"+key.Last4 {
				t.Errorf("Masked = %q, mau %q", key.Masked, tc.wantPrefix+"****"+key.Last4)
			}
		})
	}
}

// Raw bertipe Secret supaya tidak bisa tercetak karena kelalaian, dan Hash tidak boleh
// memuat key aslinya.
func TestGenerateAPIKeyDoesNotLeak(t *testing.T) {
	key, err := GenerateAPIKey(true, testPepper)
	if err != nil {
		t.Fatalf("GenerateAPIKey: %v", err)
	}

	raw := key.Raw.Reveal()
	if key.Raw.String() != redacted {
		t.Errorf("Raw tercetak: %s", key.Raw.String())
	}
	if strings.Contains(key.Hash, raw) {
		t.Error("Hash memuat key mentah")
	}
	body := strings.TrimPrefix(raw, key.Prefix)
	if strings.Contains(key.Masked, body[:len(body)-last4Len]) {
		t.Errorf("Masked menyisakan badan key: %s", key.Masked)
	}
}

func TestGenerateAPIKeyUnique(t *testing.T) {
	const n = 500
	seen := make(map[string]struct{}, n)
	for i := 0; i < n; i++ {
		key, err := GenerateAPIKey(true, testPepper)
		if err != nil {
			t.Fatalf("GenerateAPIKey: %v", err)
		}
		raw := key.Raw.Reveal()
		if _, dup := seen[raw]; dup {
			t.Fatal("key terulang — sumber acak bermasalah")
		}
		seen[raw] = struct{}{}
	}
}

func TestGenerateAPIKeyRequiresPepper(t *testing.T) {
	for _, pepper := range [][]byte{nil, {}} {
		if _, err := GenerateAPIKey(true, pepper); !errors.Is(err, ErrEmptyPepper) {
			t.Errorf("pepper %v: err = %v, mau ErrEmptyPepper", pepper, err)
		}
	}
}

func TestHashAPIKeyDeterministicAndPepperBound(t *testing.T) {
	const raw = "sk_live_" + "AAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAA"

	a := HashAPIKey(raw, testPepper)
	b := HashAPIKey(raw, testPepper)
	if a != b {
		t.Error("hash tidak deterministik — key tidak akan bisa dicari lewat indeks")
	}
	if len(a) != 64 {
		t.Errorf("panjang hash hex = %d, mau 64", len(a))
	}

	// Pepper berbeda harus menghasilkan hash berbeda: inilah yang membuat dump
	// database saja tidak cukup untuk memverifikasi key.
	if HashAPIKey(raw, []byte("pepper-lain-yang-berbeda-sekali!!")) == a {
		t.Error("hash tidak bergantung pada pepper")
	}

	// Key berbeda harus menghasilkan hash berbeda.
	if HashAPIKey(raw+"x", testPepper) == a {
		t.Error("hash tidak bergantung pada isi key")
	}
}

func TestVerifyAPIKey(t *testing.T) {
	key, err := GenerateAPIKey(true, testPepper)
	if err != nil {
		t.Fatalf("GenerateAPIKey: %v", err)
	}
	raw := key.Raw.Reveal()

	if !VerifyAPIKey(raw, key.Hash, testPepper) {
		t.Error("key yang benar dinyatakan tidak cocok")
	}

	tests := []struct {
		name   string
		raw    string
		hash   string
		pepper []byte
	}{
		{"key salah", raw[:len(raw)-1] + "Z", key.Hash, testPepper},
		{"key kosong", "", key.Hash, testPepper},
		{"hash kosong", raw, "", testPepper},
		{"pepper salah", raw, key.Hash, []byte("pepper-yang-salah-sekali-sekali!")},
		{"pepper kosong", raw, key.Hash, nil},
		{"hash bukan hex", raw, "bukan-hex", testPepper},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			if VerifyAPIKey(tc.raw, tc.hash, tc.pepper) {
				t.Error("seharusnya tidak cocok")
			}
		})
	}
}

func TestParseAPIKey(t *testing.T) {
	valid := strings.Repeat("a", keyBodyLen)

	tests := []struct {
		name       string
		raw        string
		wantPrefix string
		wantErr    bool
	}{
		{"live sah", KeyPrefixLive + valid, KeyPrefixLive, false},
		{"test sah", KeyPrefixTest + valid, KeyPrefixTest, false},
		{"tanpa prefix", valid, "", true},
		{"prefix asing", "pk_live_" + valid, "", true},
		{"terlalu pendek", KeyPrefixLive + "abc", "", true},
		{"terlalu panjang", KeyPrefixLive + valid + "a", "", true},
		{"karakter di luar base62", KeyPrefixLive + strings.Repeat("a", keyBodyLen-1) + "-", "", true},
		{"memuat spasi", KeyPrefixLive + strings.Repeat("a", keyBodyLen-1) + " ", "", true},
		{"kosong", "", "", true},
		{"hanya prefix", KeyPrefixLive, "", true},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			prefix, err := ParseAPIKey(tc.raw)
			if tc.wantErr {
				if err == nil {
					t.Fatal("seharusnya ditolak")
				}
				if !errors.Is(err, ErrInvalidKeyFormat) {
					t.Errorf("err = %v, mau ErrInvalidKeyFormat", err)
				}
				return
			}
			if err != nil {
				t.Fatalf("err = %v, mau nil", err)
			}
			if prefix != tc.wantPrefix {
				t.Errorf("prefix = %q, mau %q", prefix, tc.wantPrefix)
			}
		})
	}
}

// Pesan error ParseAPIKey tidak boleh mengulang isi key.
func TestParseAPIKeyErrorDoesNotEchoKey(t *testing.T) {
	raw := KeyPrefixLive + strings.Repeat("Z", keyBodyLen-1) + "-"
	_, err := ParseAPIKey(raw)
	if err == nil {
		t.Fatal("seharusnya ditolak")
	}
	if strings.Contains(err.Error(), "ZZZ") {
		t.Errorf("pesan error mengulang isi key: %v", err)
	}
}

func TestMaskAPIKey(t *testing.T) {
	key, err := GenerateAPIKey(true, testPepper)
	if err != nil {
		t.Fatalf("GenerateAPIKey: %v", err)
	}
	raw := key.Raw.Reveal()

	if got := MaskAPIKey(raw); got != key.Masked {
		t.Errorf("MaskAPIKey = %q, mau %q", got, key.Masked)
	}
	// Bentuk tak dikenal disamarkan seluruhnya.
	for _, bad := range []string{"", "bukan-key", "sk_live_pendek"} {
		if got := MaskAPIKey(bad); got != "****" {
			t.Errorf("MaskAPIKey(%q) = %q, mau \"****\"", bad, got)
		}
	}
}

func TestMaskedFromParts(t *testing.T) {
	if got := MaskedFromParts(KeyPrefixLive, "9a21"); got != "sk_live_****9a21" {
		t.Errorf("MaskedFromParts = %q, mau \"sk_live_****9a21\"", got)
	}
}

// randomBase62 memakai rejection sampling; kalau diganti modulo langsung, karakter
// awal alfabet akan muncul lebih sering dan entropi key berkurang. Test ini menjaganya.
func TestRandomBase62NoModuloBias(t *testing.T) {
	const (
		samples  = 62 * 1000
		expected = samples / 62
		// Toleransi jauh di atas 6 sigma (≈31) supaya tidak flaky, tapi tetap jauh
		// di bawah simpangan yang dihasilkan bias modulo (yang membuat 4 karakter
		// pertama sekitar 1,3× lebih sering).
		tolerance = 200
	)

	s, err := randomBase62(samples)
	if err != nil {
		t.Fatalf("randomBase62: %v", err)
	}
	if len(s) != samples {
		t.Fatalf("panjang = %d, mau %d", len(s), samples)
	}

	freq := make(map[rune]int, 62)
	for _, r := range s {
		freq[r]++
	}
	if len(freq) != 62 {
		t.Errorf("hanya %d karakter berbeda yang muncul, mau 62", len(freq))
	}
	for _, r := range base62Alphabet {
		if n := freq[r]; n < expected-tolerance || n > expected+tolerance {
			t.Errorf("karakter %q muncul %d kali, diharapkan %d±%d — indikasi bias", r, n, expected, tolerance)
		}
	}
}

func TestRandomBase62Length(t *testing.T) {
	for _, n := range []int{0, 1, 7, 43, 200} {
		s, err := randomBase62(n)
		if err != nil {
			t.Fatalf("randomBase62(%d): %v", n, err)
		}
		if len(s) != n {
			t.Errorf("randomBase62(%d) panjang %d", n, len(s))
		}
		if !isBase62(s) {
			t.Errorf("randomBase62(%d) menghasilkan karakter di luar base62: %q", n, s)
		}
	}
}

func TestIsBase62(t *testing.T) {
	tests := []struct {
		s    string
		want bool
	}{
		{"", true},
		{"abcXYZ019", true},
		{"abc-def", false},
		{"abc_def", false},
		{"abc def", false},
		{"abc.def", false},
		{"abc\ndef", false},
	}
	for _, tc := range tests {
		if got := isBase62(tc.s); got != tc.want {
			t.Errorf("isBase62(%q) = %v, mau %v", tc.s, got, tc.want)
		}
	}
}
