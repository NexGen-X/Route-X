// Package security menyediakan primitif keamanan lintas modul: tipe rahasia yang
// tidak pernah tercetak, hashing, enkripsi kredensial, dan penjaga SSRF.
package security

import (
	"encoding/json"
	"fmt"
	"log/slog"
	"strings"
)

// redacted adalah satu-satunya bentuk yang boleh muncul di log, error, atau JSON
// untuk nilai bertipe Secret.
const redacted = "[REDACTED]"

// Secret membungkus string sensitif (password, token, API key, connection string).
//
// Nilainya hanya bisa dibaca lewat Reveal(). Semua jalur keluaran umum — fmt, slog,
// encoding/json, dan encoding/text — sengaja di-override agar mengembalikan
// "[REDACTED]", sehingga rahasia tidak bisa bocor karena seseorang lupa menyamarkan
// nilai saat menulis log atau membalas error.
type Secret string

// String memenuhi fmt.Stringer. Selalu tersamar, termasuk lewat %v dan %s.
func (s Secret) String() string { return redacted }

// GoString memenuhi fmt.GoStringer sehingga %#v juga tersamar.
func (s Secret) GoString() string { return redacted }

// LogValue memenuhi slog.LogValuer sehingga slog menyamarkannya otomatis.
func (s Secret) LogValue() slog.Value { return slog.StringValue(redacted) }

// MarshalJSON memastikan rahasia tidak pernah ikut terserialisasi ke respons API.
func (s Secret) MarshalJSON() ([]byte, error) { return json.Marshal(redacted) }

// MarshalText menutup jalur encoding/text (YAML, form, dsb).
func (s Secret) MarshalText() ([]byte, error) { return []byte(redacted), nil }

// Reveal mengembalikan nilai asli. Ini satu-satunya cara membacanya — panggil hanya
// tepat di titik pemakaian (dial database, tanda tangan HMAC, header upstream),
// jangan disimpan ke variabel string yang berkeliaran.
func (s Secret) Reveal() string { return string(s) }

// IsZero melaporkan apakah rahasia belum diisi.
func (s Secret) IsZero() bool { return len(s) == 0 }

// Len mengembalikan panjang rahasia. Aman dipakai untuk validasi kekuatan tanpa
// membuka nilainya.
func (s Secret) Len() int { return len(s) }

// Verify memastikan tipe ini benar-benar memenuhi kontrak yang diandalkan.
var (
	_ fmt.Stringer   = Secret("")
	_ slog.LogValuer = Secret("")
	_ json.Marshaler = Secret("")
)

// Mask menyamarkan pengenal yang memang perlu dikenali manusia — misalnya API key
// di dashboard — dengan menyisakan prefix dan beberapa karakter terakhir:
//
//	Mask("sk_live_0123456789abcdef9a21", "sk_live_", 4) => "sk_live_****9a21"
//
// Kalau nilainya terlalu pendek untuk disamarkan dengan aman, seluruhnya diganti
// bintang sehingga tidak ada informasi yang lolos.
func Mask(value, prefix string, tailLen int) string {
	body := strings.TrimPrefix(value, prefix)
	if tailLen < 0 {
		tailLen = 0
	}
	// Hitung per rune, bukan per byte: memotong string UTF-8 di tengah
	// rune multi-byte menghasilkan string tidak valid sehingga json.Marshal
	// gagal dan handler menjawab internal_error (ditemukan via e2e saat
	// kredensial berisi karakter non-ASCII).
	runes := []rune(body)
	if len(runes) < tailLen*2 {
		return prefix + strings.Repeat("*", max(len(runes), 4))
	}
	return prefix + "****" + string(runes[len(runes)-tailLen:])
}
