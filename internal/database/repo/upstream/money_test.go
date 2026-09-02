package upstream

import (
	"encoding/json"
	"errors"
	"testing"
)

// Pembacaan teks desimal harus eksak, dan presisi yang tidak bisa disimpan harus ditolak
// alih-alih dibulatkan diam-diam.
func TestParseUSD(t *testing.T) {
	for _, tc := range []struct {
		in    string
		units int64
	}{
		{"0", 0},
		{"1", 100_000_000},
		{"2.5", 250_000_000},
		{"12.345678", 1_234_567_800},
		{"0.00000015", 15},
		{"0.00000001", 1},
		{".5", 50_000_000},
		{"1.", 100_000_000},
		{"+3.25", 325_000_000},
		{"-1.5", -150_000_000},
		{"  2.50  ", 250_000_000},
		// Nol di ujung tidak membawa presisi, jadi kelebihan angkanya diterima.
		{"0.000000000", 0},
		{"2.500000000", 250_000_000},
	} {
		t.Run(tc.in, func(t *testing.T) {
			got, err := ParseUSD(tc.in)
			if err != nil {
				t.Fatalf("ParseUSD(%q): %v", tc.in, err)
			}
			if got.Units() != tc.units {
				t.Errorf("ParseUSD(%q) = %d satuan, ingin %d", tc.in, got.Units(), tc.units)
			}
		})
	}
}

func TestParseUSDRejectsInvalidInput(t *testing.T) {
	for _, in := range []string{
		"",
		"   ",
		"abc",
		"1.2.3",
		"1,5",
		"1_000",
		// Sembilan angka desimal: lebih presisi daripada yang bisa disimpan.
		"0.000000001",
		// Melebihi rentang int64 setelah dikali 10^8.
		"999999999999.99999999",
	} {
		t.Run(in, func(t *testing.T) {
			if _, err := ParseUSD(in); !errors.Is(err, ErrInvalidAmount) {
				t.Errorf("ParseUSD(%q) = %v, ingin ErrInvalidAmount", in, err)
			}
		})
	}
}

func TestUSDDecimal(t *testing.T) {
	for _, tc := range []struct {
		units int64
		want  string
	}{
		{0, "0.00000000"},
		{1, "0.00000001"},
		{15, "0.00000015"},
		{250_000_000, "2.50000000"},
		{1_234_567_800, "12.34567800"},
		{-150_000_000, "-1.50000000"},
	} {
		t.Run(tc.want, func(t *testing.T) {
			got := USD(tc.units).Decimal()
			if got != tc.want {
				t.Errorf("USD(%d).Decimal() = %q, ingin %q", tc.units, got, tc.want)
			}
			if USD(tc.units).String() != tc.want {
				t.Errorf("String() berbeda dari Decimal()")
			}
			// Bolak-balik harus mengembalikan nilai yang sama persis.
			back, err := ParseUSD(got)
			if err != nil {
				t.Fatalf("ParseUSD(%q): %v", got, err)
			}
			if back.Units() != tc.units {
				t.Errorf("bolak-balik = %d, ingin %d", back.Units(), tc.units)
			}
		})
	}
}

// Kolom harga hanya menyimpan enam angka desimal, jadi nilai yang lebih presisi harus
// bisa dikenali sebelum ditulis.
func TestUSDIsPriceScale(t *testing.T) {
	for _, tc := range []struct {
		in   string
		want bool
	}{
		{"2.5", true},
		{"0.000001", true},
		{"0", true},
		{"12.345678", true},
		{"0.0000015", false},
		{"0.00000015", false},
		{"0.00000001", false},
	} {
		t.Run(tc.in, func(t *testing.T) {
			if got := MustParseUSD(tc.in).IsPriceScale(); got != tc.want {
				t.Errorf("MustParseUSD(%q).IsPriceScale() = %v, ingin %v", tc.in, got, tc.want)
			}
		})
	}
}

// Nilai uang keluar sebagai string JSON supaya tidak berubah saat dibaca klien yang
// memakai floating point ganda.
func TestUSDJSON(t *testing.T) {
	type wrapper struct {
		Price USD `json:"harga"`
	}

	encoded, err := json.Marshal(wrapper{Price: MustParseUSD("2.5")})
	if err != nil {
		t.Fatalf("Marshal: %v", err)
	}
	if string(encoded) != `{"harga":"2.50000000"}` {
		t.Errorf("hasil = %s", encoded)
	}

	for _, tc := range []struct {
		in    string
		units int64
	}{
		{`{"harga":"2.5"}`, 250_000_000},
		{`{"harga":2.5}`, 250_000_000},
		{`{"harga":"0.00000015"}`, 15},
		{`{"harga":null}`, 0},
	} {
		t.Run(tc.in, func(t *testing.T) {
			var got wrapper
			if err := json.Unmarshal([]byte(tc.in), &got); err != nil {
				t.Fatalf("Unmarshal(%s): %v", tc.in, err)
			}
			if got.Price.Units() != tc.units {
				t.Errorf("Unmarshal(%s) = %d satuan, ingin %d", tc.in, got.Price.Units(), tc.units)
			}
		})
	}

	t.Run("angka yang terlalu presisi ditolak", func(t *testing.T) {
		var got wrapper
		if err := json.Unmarshal([]byte(`{"harga":0.000000001}`), &got); !errors.Is(err, ErrInvalidAmount) {
			t.Errorf("Unmarshal = %v, ingin ErrInvalidAmount", err)
		}
	})
}

func TestMustParseUSDPanics(t *testing.T) {
	defer func() {
		if recover() == nil {
			t.Error("MustParseUSD tidak panik pada masukan tidak sah")
		}
	}()
	MustParseUSD("bukan angka")
}
