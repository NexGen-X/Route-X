package security

import (
	"crypto/aes"
	"crypto/cipher"
	"crypto/hmac"
	"crypto/rand"
	"crypto/sha256"
	"encoding/base64"
	"encoding/hex"
	"errors"
	"fmt"
	"strings"
)

// Panjang kunci AES-256 dalam byte.
const KeyLen = 32

// Versi format ciphertext. Ikut disimpan agar format bisa diubah di masa depan tanpa
// membuat data lama tidak terbaca.
const cipherFormatVersion = "v1"

// keyIDInfo adalah label domain untuk turunan pengenal kunci, supaya nilai HMAC ini
// tidak bisa dipakai ulang untuk tujuan lain.
const keyIDInfo = "routex/credential-key-id/v1"

var (
	// ErrInvalidKeyLength dikembalikan saat kunci bukan 32 byte.
	ErrInvalidKeyLength = errors.New("kunci enkripsi harus tepat 32 byte untuk AES-256")
	// ErrMalformedCiphertext dikembalikan saat ciphertext tidak sesuai format.
	ErrMalformedCiphertext = errors.New("ciphertext tidak sesuai format yang dikenal")
	// ErrKeyMismatch dikembalikan saat ciphertext dienkripsi dengan kunci lain.
	ErrKeyMismatch = errors.New("ciphertext dienkripsi dengan kunci yang berbeda")
	// ErrDecryptionFailed dikembalikan saat autentikasi ciphertext gagal. Pesannya
	// sengaja tidak membedakan penyebab agar tidak menjadi oracle bagi penyerang.
	ErrDecryptionFailed = errors.New("dekripsi gagal: ciphertext atau konteksnya tidak sah")
)

// Cipher mengenkripsi kredensial provider dengan AES-256-GCM.
//
// Setiap ciphertext diikat ke konteksnya lewat Additional Authenticated Data (AAD).
// Ini penting: tanpa AAD, seseorang yang bisa menulis ke database dapat memindahkan
// blob kredensial dari satu baris provider ke baris lain dan membuat gateway memakai
// kredensial itu untuk upstream yang salah. Dengan AAD berisi identitas baris,
// pemindahan seperti itu akan gagal saat dekripsi.
type Cipher struct {
	aead  cipher.AEAD
	keyID string
}

// NewCipher membuat Cipher dari kunci 32 byte, biasanya dari config.EncryptionKey.
func NewCipher(key []byte) (*Cipher, error) {
	if len(key) != KeyLen {
		return nil, fmt.Errorf("%w: dapat %d byte", ErrInvalidKeyLength, len(key))
	}

	block, err := aes.NewCipher(key)
	if err != nil {
		// Tidak seharusnya terjadi karena panjang kunci sudah divalidasi.
		return nil, fmt.Errorf("membuat blok AES: %w", err)
	}
	aead, err := cipher.NewGCM(block)
	if err != nil {
		return nil, fmt.Errorf("membuat GCM: %w", err)
	}

	return &Cipher{aead: aead, keyID: deriveKeyID(key)}, nil
}

// deriveKeyID menghasilkan pengenal pendek untuk kunci. Dipakai untuk mengenali kunci
// mana yang mengenkripsi sebuah record sehingga rotasi kunci bisa dilakukan bertahap.
//
// Yang disimpan adalah HMAC berlabel domain, bukan hash langsung dari kunci, agar
// nilai ini tidak bisa dimanfaatkan untuk menyerang kunci aslinya.
func deriveKeyID(key []byte) string {
	mac := hmac.New(sha256.New, key)
	mac.Write([]byte(keyIDInfo))
	return hex.EncodeToString(mac.Sum(nil)[:6])
}

// KeyID mengembalikan pengenal kunci aktif.
func (c *Cipher) KeyID() string { return c.keyID }

// Encrypt mengenkripsi plaintext dan mengembalikan string siap simpan berformat
//
//	v1.<key_id>.<base64url(nonce || ciphertext || tag)>
//
// aad harus berisi identitas stabil dari record yang menyimpan hasil ini — misalnya
// "provider_credential:<uuid>". Nilai yang sama wajib diberikan saat dekripsi.
func (c *Cipher) Encrypt(plaintext []byte, aad string) (string, error) {
	nonce := make([]byte, c.aead.NonceSize())
	if _, err := rand.Read(nonce); err != nil {
		return "", fmt.Errorf("membuat nonce: %w", err)
	}

	// Nonce disimpan sebagai prefix agar bisa dipakai kembali saat dekripsi.
	sealed := c.aead.Seal(nonce, nonce, plaintext, []byte(aad))

	return strings.Join([]string{
		cipherFormatVersion,
		c.keyID,
		base64.RawURLEncoding.EncodeToString(sealed),
	}, "."), nil
}

// EncryptSecret adalah pembungkus Encrypt untuk nilai bertipe Secret.
func (c *Cipher) EncryptSecret(s Secret, aad string) (string, error) {
	return c.Encrypt([]byte(s.Reveal()), aad)
}

// Decrypt membuka ciphertext hasil Encrypt. aad harus persis sama dengan saat enkripsi.
func (c *Cipher) Decrypt(stored, aad string) ([]byte, error) {
	version, keyID, sealed, err := parseCiphertext(stored)
	if err != nil {
		return nil, err
	}
	if version != cipherFormatVersion {
		return nil, fmt.Errorf("%w: versi %q tidak didukung", ErrMalformedCiphertext, version)
	}
	if keyID != c.keyID {
		// Dilaporkan terpisah agar operator tahu ini soal rotasi kunci, bukan data rusak.
		return nil, fmt.Errorf("%w: record dienkripsi dengan kunci %s, kunci aktif %s",
			ErrKeyMismatch, keyID, c.keyID)
	}

	nonceSize := c.aead.NonceSize()
	if len(sealed) < nonceSize+c.aead.Overhead() {
		return nil, ErrMalformedCiphertext
	}

	plaintext, err := c.aead.Open(nil, sealed[:nonceSize], sealed[nonceSize:], []byte(aad))
	if err != nil {
		// Penyebab aslinya tidak diteruskan: membedakan "tag salah" dari "aad salah"
		// akan memberi penyerang informasi yang tidak perlu.
		return nil, ErrDecryptionFailed
	}
	return plaintext, nil
}

// DecryptSecret membuka ciphertext dan langsung membungkusnya sebagai Secret, sehingga
// hasilnya tidak bisa tercetak ke log karena kelalaian.
func (c *Cipher) DecryptSecret(stored, aad string) (Secret, error) {
	plaintext, err := c.Decrypt(stored, aad)
	if err != nil {
		return "", err
	}
	return Secret(plaintext), nil
}

// CanDecrypt melaporkan apakah ciphertext ini dienkripsi dengan kunci aktif. Dipakai
// worker rotasi kunci untuk memilih record yang perlu dienkripsi ulang, tanpa harus
// mencoba dekripsi.
func (c *Cipher) CanDecrypt(stored string) bool {
	version, keyID, _, err := parseCiphertext(stored)
	return err == nil && version == cipherFormatVersion && keyID == c.keyID
}

// parseCiphertext memecah format tersimpan menjadi bagian-bagiannya.
func parseCiphertext(stored string) (version, keyID string, sealed []byte, err error) {
	parts := strings.Split(stored, ".")
	if len(parts) != 3 || parts[0] == "" || parts[1] == "" || parts[2] == "" {
		return "", "", nil, ErrMalformedCiphertext
	}

	sealed, decErr := base64.RawURLEncoding.DecodeString(parts[2])
	if decErr != nil {
		return "", "", nil, fmt.Errorf("%w: payload bukan base64url", ErrMalformedCiphertext)
	}
	return parts[0], parts[1], sealed, nil
}

// Reencrypt memindahkan satu record dari kunci lama ke kunci baru. Dipakai saat rotasi
// ENCRYPTION_KEY: dekripsi dengan Cipher lama, enkripsi ulang dengan yang baru.
func Reencrypt(old, current *Cipher, stored, aad string) (string, error) {
	plaintext, err := old.Decrypt(stored, aad)
	if err != nil {
		return "", fmt.Errorf("dekripsi dengan kunci lama: %w", err)
	}
	// Bersihkan plaintext dari memori setelah dipakai; bukan jaminan mutlak di Go,
	// tapi memperkecil jendela paparan pada core dump.
	defer zero(plaintext)

	return current.Encrypt(plaintext, aad)
}

// zero menimpa isi slice dengan nol.
func zero(b []byte) {
	for i := range b {
		b[i] = 0
	}
}

// CredentialAAD membentuk AAD standar untuk kredensial provider, sehingga semua
// pemanggil memakai bentuk yang sama.
func CredentialAAD(credentialID string) string {
	return "provider_credential:" + credentialID
}

// WebhookAAD membentuk data terotentikasi tambahan (AAD) standar untuk rahasia webhook.
//
// AAD ini WAJIB diikat ke ID baris webhook: tanpa pengikatan ini, ciphertext yang sah
// dari satu webhook bisa disalin langsung ke webhook lain (ciphertext splicing) dan
// didekripsi dengan sukses oleh instance mana pun yang memegang kunci enkripsi yang sama.
func WebhookAAD(webhookID string) string {
	return "webhook:" + webhookID
}

// CLIToolAAD membentuk AAD standar untuk API key perkakas CLI yang disimpan di
// tabel settings. Kunci AAD diikat ke ID tool agar ciphertext satu tool tidak
// dapat dipindahkan ke tool lain (ciphertext splicing).
func CLIToolAAD(toolID string) string {
	return "cli_tool:" + toolID
}

// XrayStateAAD membentuk AAD tetap untuk state kredensial Xray yang disimpan
// sebagai satu record settings. Ciphertext tidak boleh dipindah ke jenis setting lain.
func XrayStateAAD() string {
	return "setting:system:xray:config"
}

// OAuthSessionAAD membentuk AAD standar untuk refresh token sesi OAuth provider.
func OAuthSessionAAD(sessionID string) string {
	return "provider_oauth:" + sessionID
}

