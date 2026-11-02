package repo

import "testing"

// TestEscapeLikePattern memastikan karakter wildcard LIKE yang diketik
// pengguna berubah menjadi harfiah (audit3 V-DT-23).
func TestEscapeLikePattern(t *testing.T) {
	for masukan, mau := range map[string]string{
		"":         "",
		"biasa":    "biasa",
		"100%":     `100\%`,
		"%":        `\%`,
		"a_b":      `a\_b`,
		"_":        `\_`,
		`a\b`:      `a\\b`,
		`50%_off\`: `50\%\_off\\`,
	} {
		if got := EscapeLikePattern(masukan); got != mau {
			t.Errorf("EscapeLikePattern(%q) = %q, mau %q", masukan, got, mau)
		}
	}
}
