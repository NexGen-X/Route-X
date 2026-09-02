package security

import (
	"crypto/hmac"
	"crypto/rand"
	"crypto/sha256"
	"crypto/subtle"
	"encoding/hex"
	"errors"
	"fmt"
	"strings"
)

// Prefix API key. Prefix ikut disimpan sebagai kolom tersendiri agar dashboard bisa
// menandai lingkungan sebuah key tanpa membuka nilainya.
const (
	KeyPrefixLive = "sk_live_"
	KeyPrefixTest = "sk_test_"
)

// keyBodyLen adalah jumlah karakter acak setelah prefix.
//
// 43 karakter base62 memberi sekitar 256 bit entropi — setara kunci acak 32 byte.
// Dengan entropi setinggi ini, key tidak perlu di-hash lambat: tidak ada ruang
// tebakan yang realistis, sedangkan hashing lambat akan menambah biaya pada setiap
// request yang lewat gateway.
const keyBodyLen = 43

// base62Alphabet sengaja tidak memuat '-' dan '_' agar key tetap satu kata saat
// diklik ganda di terminal atau editor, sehingga operator tidak menyalin sebagian.
const base62Alphabet = "0123456789ABCDEFGHIJKLMNOPQRSTUVWXYZabcdefghijklmnopqrstuvwxyz"

// last4Len adalah jumlah karakter terakhir yang ditampilkan di UI.
const last4Len = 4

var (
	// ErrInvalidKeyFormat dikembalikan saat string bukan API key yang dikenali.
	ErrInvalidKeyFormat = errors.New("format API key tidak dikenali")
	// ErrEmptyPepper dikembalikan saat pepper kosong. Tanpa pepper, dump database
	// sudah cukup untuk memverifikasi key hasil tebakan.
	ErrEmptyPepper = errors.New("pepper API key tidak boleh kosong")
)

// GeneratedKey adalah hasil pembuatan API key baru.
//
// Raw hanya ada di memori dan ditampilkan satu kali ke pembuatnya; yang masuk database
// adalah Hash, Prefix, dan Last4.
type GeneratedKey struct {
	// Raw adalah key utuh yang harus diberikan ke pengguna sekali saja.
	Raw Secret
	// Hash adalah HMAC-SHA256 hex dari Raw, nilai yang disimpan dan diindeks.
	Hash string
	// Prefix menandai lingkungan key, mis. "sk_live_".
	Prefix string
	// Last4 adalah empat karakter terakhir, untuk membedakan key di daftar.
	Last4 string
	// Masked adalah bentuk siap tampil, mis. "sk_live_****9a21".
	Masked string
}

// GenerateAPIKey membuat API key baru beserta hash simpanannya.
//
// live menentukan prefix: true menghasilkan sk_live_, false menghasilkan sk_test_.
func GenerateAPIKey(live bool, pepper []byte) (GeneratedKey, error) {
	if len(pepper) == 0 {
		return GeneratedKey{}, ErrEmptyPepper
	}

	prefix := KeyPrefixTest
	if live {
		prefix = KeyPrefixLive
	}

	body, err := randomBase62(keyBodyLen)
	if err != nil {
		return GeneratedKey{}, fmt.Errorf("membuat bagian acak API key: %w", err)
	}
	raw := prefix + body

	return GeneratedKey{
		Raw:    Secret(raw),
		Hash:   HashAPIKey(raw, pepper),
		Prefix: prefix,
		Last4:  body[len(body)-last4Len:],
		Masked: Mask(raw, prefix, last4Len),
	}, nil
}

// HashAPIKey menghitung HMAC-SHA256 dari key memakai pepper server.
//
// HMAC dengan pepper dipilih, bukan hash biasa: pepper hidup di environment aplikasi
// dan tidak ikut dalam dump database, sehingga penyerang yang hanya mendapat tabel
// api_keys tidak bisa memverifikasi kandidat key secara offline.
//
// Hasilnya deterministik supaya key bisa dicari lewat indeks database — inilah alasan
// primitif ini tidak boleh dipakai untuk password, yang justru wajib bersalt acak.
func HashAPIKey(raw string, pepper []byte) string {
	mac := hmac.New(sha256.New, pepper)
	mac.Write([]byte(raw))
	return hex.EncodeToString(mac.Sum(nil))
}

// VerifyAPIKey membandingkan key dari request dengan hash tersimpan dalam waktu konstan.
func VerifyAPIKey(raw, storedHash string, pepper []byte) bool {
	if len(pepper) == 0 || storedHash == "" {
		return false
	}
	computed := HashAPIKey(raw, pepper)
	return subtle.ConstantTimeCompare([]byte(computed), []byte(storedHash)) == 1
}

// ParseAPIKey memvalidasi bentuk key dan mengembalikan prefiksnya.
//
// Dipakai middleware autentikasi untuk menolak header yang jelas bukan API key sebelum
// menyentuh database, sehingga lalu lintas sampah tidak membebani query.
func ParseAPIKey(raw string) (prefix string, err error) {
	for _, p := range []string{KeyPrefixLive, KeyPrefixTest} {
		body, ok := strings.CutPrefix(raw, p)
		if !ok {
			continue
		}
		if len(body) != keyBodyLen {
			return "", fmt.Errorf("%w: panjang bagian acak %d, mau %d", ErrInvalidKeyFormat, len(body), keyBodyLen)
		}
		if !isBase62(body) {
			return "", fmt.Errorf("%w: bagian acak memuat karakter di luar base62", ErrInvalidKeyFormat)
		}
		return p, nil
	}
	return "", ErrInvalidKeyFormat
}

// MaskAPIKey membentuk tampilan tersamar dari key utuh. Kalau bentuknya tidak dikenali,
// seluruhnya disamarkan supaya tidak ada bagian yang lolos ke UI atau log.
func MaskAPIKey(raw string) string {
	prefix, err := ParseAPIKey(raw)
	if err != nil {
		return "****"
	}
	return Mask(raw, prefix, last4Len)
}

// MaskedFromParts menyusun tampilan tersamar dari data yang tersimpan di database,
// tanpa perlu key aslinya. Inilah yang dipakai dashboard.
func MaskedFromParts(prefix, last4 string) string {
	return prefix + "****" + last4
}

// randomBase62 menghasilkan string base62 acak sepanjang n karakter.
//
// Memakai rejection sampling: byte acak yang nilainya di atas kelipatan 62 terbesar
// dibuang, bukan di-modulo. Modulo langsung akan membuat karakter awal alfabet muncul
// lebih sering, mengurangi entropi efektif key.
func randomBase62(n int) (string, error) {
	const maxAcceptable = 256 - (256 % len(base62Alphabet)) // 248

	out := make([]byte, 0, n)
	buf := make([]byte, n) // dibaca ulang bila banyak byte tertolak

	for len(out) < n {
		if _, err := rand.Read(buf); err != nil {
			return "", err
		}
		for _, b := range buf {
			if int(b) >= maxAcceptable {
				continue
			}
			out = append(out, base62Alphabet[int(b)%len(base62Alphabet)])
			if len(out) == n {
				break
			}
		}
	}
	return string(out), nil
}

// isBase62 melaporkan apakah seluruh karakter s ada di alfabet base62.
func isBase62(s string) bool {
	for i := 0; i < len(s); i++ {
		c := s[i]
		switch {
		case c >= '0' && c <= '9', c >= 'A' && c <= 'Z', c >= 'a' && c <= 'z':
		default:
			return false
		}
	}
	return true
}
