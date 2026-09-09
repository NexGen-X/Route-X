package providers

import (
	"strings"
	"testing"
	"unicode/utf8"
)

// truncate wajib memotong per-rune, bukan per-byte: memotong di tengah rune
// multibyte menghasilkan string yang tidak valid UTF-8 dan merusak log.
func TestTruncateMemotongPerRune(t *testing.T) {
	// "é" = 2 byte, "世" = 3 byte, "🌍" = 4 byte.
	s := "abé世🌍cd"
	got := truncate(s, 4)
	if !utf8.ValidString(got) {
		t.Fatalf("truncate menghasilkan string tidak valid UTF-8: %q", got)
	}
	if want := "abé世…"; got != want {
		t.Errorf("truncate = %q, mau %q", got, want)
	}
	// Tanpa pemotongan: dikembalikan utuh tanpa penanda.
	if got := truncate("héllo", 5); got != "héllo" {
		t.Errorf("truncate tanpa potong = %q, mau %q", got, "héllo")
	}
	// Batas dihitung dalam rune, bukan byte.
	if n := len([]rune(strings.TrimSuffix(truncate(s, 4), "…"))); n != 4 {
		t.Errorf("jumlah rune terpotong = %d, mau 4", n)
	}
}
