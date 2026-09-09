package security

import (
	"bytes"
	"encoding/json"
	"fmt"
	"log/slog"
	"strings"
	"testing"
	"unicode/utf8"
)

const sensitive = "sk_live_super_secret_value_9a21"

// Rahasia tidak boleh bocor lewat jalur keluaran apa pun. Satu test tabel untuk
// semua verb fmt sekaligus, karena tiap verb punya jalur berbeda di fmt.
func TestSecretNeverPrints(t *testing.T) {
	s := Secret(sensitive)

	for _, verb := range []string{"%s", "%v", "%q", "%#v", "%+v"} {
		got := fmt.Sprintf(verb, s)
		if strings.Contains(got, "super_secret") {
			t.Errorf("verb %s membocorkan rahasia: %s", verb, got)
		}
		if !strings.Contains(got, "REDACTED") {
			t.Errorf("verb %s tidak menyamarkan: %s", verb, got)
		}
	}
}

func TestSecretJSONRedacted(t *testing.T) {
	payload := struct {
		Name  string `json:"name"`
		Token Secret `json:"token"`
	}{Name: "provider-a", Token: Secret(sensitive)}

	out, err := json.Marshal(payload)
	if err != nil {
		t.Fatalf("marshal: %v", err)
	}
	if bytes.Contains(out, []byte("super_secret")) {
		t.Fatalf("JSON membocorkan rahasia: %s", out)
	}
	if !bytes.Contains(out, []byte("REDACTED")) {
		t.Fatalf("JSON tidak menyamarkan: %s", out)
	}
}

func TestSecretSlogRedacted(t *testing.T) {
	var buf bytes.Buffer
	logger := slog.New(slog.NewJSONHandler(&buf, nil))
	logger.Info("dial upstream", "credential", Secret(sensitive))

	if strings.Contains(buf.String(), "super_secret") {
		t.Fatalf("slog membocorkan rahasia: %s", buf.String())
	}
	if !strings.Contains(buf.String(), "REDACTED") {
		t.Fatalf("slog tidak menyamarkan: %s", buf.String())
	}
}

func TestSecretReveal(t *testing.T) {
	s := Secret(sensitive)
	if s.Reveal() != sensitive {
		t.Fatalf("Reveal() = %q, mau %q", s.Reveal(), sensitive)
	}
	if s.Len() != len(sensitive) {
		t.Fatalf("Len() = %d, mau %d", s.Len(), len(sensitive))
	}
	if s.IsZero() {
		t.Fatal("IsZero() = true untuk rahasia terisi")
	}
	if !Secret("").IsZero() {
		t.Fatal("IsZero() = false untuk rahasia kosong")
	}
}

func TestMask(t *testing.T) {
	tests := []struct {
		name    string
		value   string
		prefix  string
		tailLen int
		want    string
	}{
		{"key penuh", "sk_live_0123456789abcdef9a21", "sk_live_", 4, "sk_live_****9a21"},
		{"tanpa prefix", "0123456789abcdef", "", 4, "****cdef"},
		{"terlalu pendek disamarkan penuh", "sk_live_9a21", "sk_live_", 4, "sk_live_****"},
		{"kosong", "", "sk_live_", 4, "sk_live_****"},
		{"tailLen negatif diperlakukan nol", "sk_live_0123456789abcdef", "sk_live_", -1, "sk_live_****"},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			if got := Mask(tc.value, tc.prefix, tc.tailLen); got != tc.want {
				t.Errorf("Mask() = %q, mau %q", got, tc.want)
			}
		})
	}
}

// Mask tidak boleh pernah menyisakan cukup karakter untuk merekonstruksi nilai.
func TestMaskHidesMostOfValue(t *testing.T) {
	const key = "sk_live_0123456789abcdefghijklmn"
	masked := Mask(key, "sk_live_", 4)
	body := strings.TrimPrefix(key, "sk_live_")
	if strings.Contains(masked, body[:len(body)-4]) {
		t.Fatalf("Mask menyisakan badan key: %s", masked)
	}
}

// Regresi e2e sisa-kritis: Mask dengan input non-ASCII tidak boleh
// menghasilkan string UTF-8 tidak valid (json.Marshal gagal -> 500).
func TestMaskUTF8TidakPecahRune(t *testing.T) {
	for _, tc := range []struct{ value, prefix string }{
		{"sk-abcdef", "sk-"},
		{"sk-abcdef", "sk-"},
		{"kredensial-singkat", ""},
	} {
		got := Mask(tc.value, tc.prefix, 4)
		if !utf8.ValidString(got) {
			t.Fatalf("Mask(%q) tidak valid UTF-8: %q", tc.value, got)
		}
		buf, err := json.Marshal(got)
		if err != nil || !json.Valid(buf) {
			t.Fatalf("Mask(%q) gagal marshal JSON: %v", tc.value, err)
		}
	}
}
