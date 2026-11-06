package providers

import (
	"encoding/json"
	"testing"
)

// B-263: signature numerik dari upstream/proksi tidak boleh menggagalkan
// unmarshal respons utuh — ia dinormalkan menjadi teks literalnya.
// Bentuk yang bukan string/angka diabaikan menjadi kosong, bukan error.
func TestSignatureToleranUnmarshal(t *testing.T) {
	for _, tc := range []struct {
		nama string
		json string
		mau  SignatureToleran
	}{
		{"string", `"abc-123"`, "abc-123"},
		{"string kosong", `""`, ""},
		{"angka bulat", `12345`, "12345"},
		{"angka negatif", `-42`, "-42"},
		{"angka desimal", `12.5`, "12.5"},
		{"null", `null`, ""},
		{"bool diabaikan", `true`, ""},
		{"objek diabaikan", `{"a":1}`, ""},
		{"array diabaikan", `[1,2]`, ""},
	} {
		t.Run(tc.nama, func(t *testing.T) {
			var s SignatureToleran
			if err := json.Unmarshal([]byte(tc.json), &s); err != nil {
				t.Fatalf("Unmarshal %s: %v", tc.json, err)
			}
			if s != tc.mau {
				t.Errorf("hasil = %q, mau %q", string(s), string(tc.mau))
			}
		})
	}
}

// Marshal selalu menulis bentuk string, dan round-trip stabil.
func TestSignatureToleranMarshal(t *testing.T) {
	raw, err := json.Marshal(SignatureToleran("12345"))
	if err != nil {
		t.Fatalf("Marshal: %v", err)
	}
	if string(raw) != `"12345"` {
		t.Errorf("marshal = %s, mau %q", raw, `"12345"`)
	}
}
