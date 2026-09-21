package security

import (
	"bytes"
	"crypto/rand"
	"errors"
	"strings"
	"testing"
)

// newTestKey menghasilkan kunci acak 32 byte.
func newTestKey(t *testing.T) []byte {
	t.Helper()
	key := make([]byte, KeyLen)
	if _, err := rand.Read(key); err != nil {
		t.Fatalf("membuat kunci: %v", err)
	}
	return key
}

func newTestCipher(t *testing.T) *Cipher {
	t.Helper()
	c, err := NewCipher(newTestKey(t))
	if err != nil {
		t.Fatalf("NewCipher: %v", err)
	}
	return c
}

func TestNewCipherRejectsBadKeyLength(t *testing.T) {
	for _, n := range []int{0, 1, 16, 24, 31, 33, 64} {
		_, err := NewCipher(make([]byte, n))
		if !errors.Is(err, ErrInvalidKeyLength) {
			t.Errorf("kunci %d byte: err = %v, mau ErrInvalidKeyLength", n, err)
		}
	}
	if _, err := NewCipher(nil); !errors.Is(err, ErrInvalidKeyLength) {
		t.Errorf("kunci nil: err = %v, mau ErrInvalidKeyLength", err)
	}
}

func TestEncryptDecryptRoundTrip(t *testing.T) {
	c := newTestCipher(t)
	aad := CredentialAAD("11111111-2222-3333-4444-555555555555")

	tests := []struct {
		name      string
		plaintext []byte
	}{
		{"api key khas", []byte("sk-proj-abcdefghijklmnopqrstuvwxyz0123456789")},
		{"kosong", []byte{}},
		{"satu byte", []byte("x")},
		{"utf-8 multibyte", []byte("kunci-räháßiá-日本語-🔐")},
		{"biner dengan byte nol", []byte{0x00, 0x01, 0xff, 0x00, 0x7f}},
		{"payload besar", bytes.Repeat([]byte("a"), 64*1024)},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			stored, err := c.Encrypt(tc.plaintext, aad)
			if err != nil {
				t.Fatalf("Encrypt: %v", err)
			}

			// Plaintext tidak boleh terlihat di bentuk tersimpan.
			if len(tc.plaintext) > 4 && strings.Contains(stored, string(tc.plaintext)) {
				t.Fatalf("plaintext terlihat di ciphertext: %s", stored)
			}

			got, err := c.Decrypt(stored, aad)
			if err != nil {
				t.Fatalf("Decrypt: %v", err)
			}
			if !bytes.Equal(got, tc.plaintext) {
				t.Errorf("hasil dekripsi berbeda dari aslinya (%d vs %d byte)", len(got), len(tc.plaintext))
			}
		})
	}
}

func TestStoredFormat(t *testing.T) {
	c := newTestCipher(t)
	stored, err := c.Encrypt([]byte("rahasia"), CredentialAAD("abc"))
	if err != nil {
		t.Fatalf("Encrypt: %v", err)
	}

	parts := strings.Split(stored, ".")
	if len(parts) != 3 {
		t.Fatalf("format tersimpan punya %d bagian, mau 3: %s", len(parts), stored)
	}
	if parts[0] != "v1" {
		t.Errorf("versi = %q, mau \"v1\"", parts[0])
	}
	if parts[1] != c.KeyID() {
		t.Errorf("key id = %q, mau %q", parts[1], c.KeyID())
	}
}

// AAD adalah pengikat konteks: ciphertext dari satu record tidak boleh bisa dibuka
// dengan konteks record lain.
func TestDecryptRejectsWrongAAD(t *testing.T) {
	c := newTestCipher(t)
	stored, err := c.Encrypt([]byte("sk-rahasia"), CredentialAAD("record-A"))
	if err != nil {
		t.Fatalf("Encrypt: %v", err)
	}

	for _, aad := range []string{
		CredentialAAD("record-B"),
		"",
		"provider_credential:record-A ",
		"record-A",
	} {
		if _, err := c.Decrypt(stored, aad); !errors.Is(err, ErrDecryptionFailed) {
			t.Errorf("aad %q: err = %v, mau ErrDecryptionFailed", aad, err)
		}
	}
}

func TestDecryptRejectsWrongKey(t *testing.T) {
	a, b := newTestCipher(t), newTestCipher(t)
	aad := CredentialAAD("record")

	stored, err := a.Encrypt([]byte("sk-rahasia"), aad)
	if err != nil {
		t.Fatalf("Encrypt: %v", err)
	}

	_, err = b.Decrypt(stored, aad)
	if !errors.Is(err, ErrKeyMismatch) {
		t.Fatalf("err = %v, mau ErrKeyMismatch", err)
	}
	// Pesan error boleh menyebut key id, tapi tidak boleh membocorkan kunci.
	if strings.Contains(err.Error(), "sk-rahasia") {
		t.Errorf("pesan error membocorkan plaintext: %v", err)
	}
}

func TestDecryptRejectsTamperedCiphertext(t *testing.T) {
	c := newTestCipher(t)
	aad := CredentialAAD("record")
	stored, err := c.Encrypt([]byte("sk-rahasia-yang-panjang"), aad)
	if err != nil {
		t.Fatalf("Encrypt: %v", err)
	}

	parts := strings.Split(stored, ".")
	payload := []byte(parts[2])

	// Ubah satu karakter di tengah payload.
	tampered := make([]byte, len(payload))
	copy(tampered, payload)
	mid := len(tampered) / 2
	if tampered[mid] == 'A' {
		tampered[mid] = 'B'
	} else {
		tampered[mid] = 'A'
	}

	broken := parts[0] + "." + parts[1] + "." + string(tampered)
	if _, err := c.Decrypt(broken, aad); err == nil {
		t.Fatal("ciphertext yang dirusak berhasil didekripsi")
	}
}

func TestDecryptRejectsMalformedInput(t *testing.T) {
	c := newTestCipher(t)
	aad := CredentialAAD("record")

	tests := []struct {
		name   string
		stored string
	}{
		{"kosong", ""},
		{"tanpa pemisah", "hanyasatubagian"},
		{"dua bagian", "v1." + c.KeyID()},
		{"empat bagian", "v1." + c.KeyID() + ".abc.def"},
		{"versi kosong", "." + c.KeyID() + ".abc"},
		{"payload bukan base64", "v1." + c.KeyID() + ".bukan base64!!"},
		{"payload terlalu pendek", "v1." + c.KeyID() + ".QQ"},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			if _, err := c.Decrypt(tc.stored, aad); err == nil {
				t.Error("input rusak seharusnya ditolak")
			}
		})
	}
}

func TestDecryptRejectsUnknownVersion(t *testing.T) {
	c := newTestCipher(t)
	aad := CredentialAAD("record")
	stored, _ := c.Encrypt([]byte("x"), aad)
	parts := strings.Split(stored, ".")

	future := "v99." + parts[1] + "." + parts[2]
	if _, err := c.Decrypt(future, aad); !errors.Is(err, ErrMalformedCiphertext) {
		t.Errorf("err = %v, mau ErrMalformedCiphertext", err)
	}
}

// Nonce harus acak per operasi: plaintext sama tidak boleh menghasilkan ciphertext sama.
func TestEncryptUsesFreshNonce(t *testing.T) {
	c := newTestCipher(t)
	aad := CredentialAAD("record")
	const plaintext = "sk-nilai-yang-sama"

	seen := make(map[string]struct{}, 200)
	for i := 0; i < 200; i++ {
		stored, err := c.Encrypt([]byte(plaintext), aad)
		if err != nil {
			t.Fatalf("Encrypt: %v", err)
		}
		if _, dup := seen[stored]; dup {
			t.Fatal("ciphertext terulang — nonce tidak acak, ini kerentanan serius pada GCM")
		}
		seen[stored] = struct{}{}
	}
}

func TestKeyIDStableAndDistinct(t *testing.T) {
	key := newTestKey(t)

	a, _ := NewCipher(key)
	b, _ := NewCipher(key)
	if a.KeyID() != b.KeyID() {
		t.Error("key id tidak stabil untuk kunci yang sama")
	}

	other := newTestCipher(t)
	if a.KeyID() == other.KeyID() {
		t.Error("dua kunci berbeda menghasilkan key id sama")
	}

	// Key id tidak boleh membocorkan kunci dalam bentuk apa pun.
	if strings.Contains(a.KeyID(), string(key)) {
		t.Error("key id memuat kunci mentah")
	}
	if len(a.KeyID()) >= KeyLen*2 {
		t.Errorf("key id terlalu panjang (%d) — jangan sampai setara hash penuh kunci", len(a.KeyID()))
	}
}

func TestCanDecrypt(t *testing.T) {
	a, b := newTestCipher(t), newTestCipher(t)
	aad := CredentialAAD("record")
	stored, _ := a.Encrypt([]byte("x"), aad)

	if !a.CanDecrypt(stored) {
		t.Error("CanDecrypt = false untuk kunci yang benar")
	}
	if b.CanDecrypt(stored) {
		t.Error("CanDecrypt = true untuk kunci yang salah")
	}
	if a.CanDecrypt("rusak") {
		t.Error("CanDecrypt = true untuk input rusak")
	}
}

func TestReencrypt(t *testing.T) {
	old, current := newTestCipher(t), newTestCipher(t)
	aad := CredentialAAD("record")
	const plaintext = "sk-kredensial-provider"

	stored, err := old.Encrypt([]byte(plaintext), aad)
	if err != nil {
		t.Fatalf("Encrypt: %v", err)
	}

	rotated, err := Reencrypt(old, current, stored, aad)
	if err != nil {
		t.Fatalf("Reencrypt: %v", err)
	}
	if !current.CanDecrypt(rotated) {
		t.Error("hasil rotasi tidak bisa dibuka kunci baru")
	}

	got, err := current.Decrypt(rotated, aad)
	if err != nil {
		t.Fatalf("Decrypt setelah rotasi: %v", err)
	}
	if string(got) != plaintext {
		t.Errorf("plaintext setelah rotasi = %q, mau %q", got, plaintext)
	}

	// Rotasi dengan kunci lama yang salah harus gagal, bukan menghasilkan data rusak.
	if _, err := Reencrypt(newTestCipher(t), current, stored, aad); err == nil {
		t.Error("Reencrypt dengan kunci lama salah seharusnya gagal")
	}
}

func TestEncryptDecryptSecret(t *testing.T) {
	c := newTestCipher(t)
	aad := CredentialAAD("record")
	const raw = "sk-rahasia-provider"

	stored, err := c.EncryptSecret(Secret(raw), aad)
	if err != nil {
		t.Fatalf("EncryptSecret: %v", err)
	}

	got, err := c.DecryptSecret(stored, aad)
	if err != nil {
		t.Fatalf("DecryptSecret: %v", err)
	}
	if got.Reveal() != raw {
		t.Errorf("Reveal() = %q, mau %q", got.Reveal(), raw)
	}
	// Hasilnya bertipe Secret, jadi tidak bisa tercetak walau di-log langsung.
	if got.String() != redacted {
		t.Errorf("hasil DecryptSecret tercetak: %s", got.String())
	}

	if _, err := c.DecryptSecret("rusak", aad); err == nil {
		t.Error("DecryptSecret menerima input rusak")
	}
}

func TestCredentialAAD(t *testing.T) {
	if got := CredentialAAD("abc"); got != "provider_credential:abc" {
		t.Errorf("CredentialAAD = %q", got)
	}
	// Dua ID berbeda wajib menghasilkan AAD berbeda.
	if CredentialAAD("a") == CredentialAAD("b") {
		t.Error("AAD tidak membedakan record")
	}
}

func TestZero(t *testing.T) {
	b := []byte("rahasia")
	zero(b)
	for i, v := range b {
		if v != 0 {
			t.Fatalf("byte %d tidak dinolkan: %d", i, v)
		}
	}
}

// TestWebhookAADDiikatKeIDWebhook memastikan format AAD webhook terikat ke ID dan membedakan tiap webhook.
func TestWebhookAADDiikatKeIDWebhook(t *testing.T) {
	if got := WebhookAAD("wh_123"); got != "webhook:wh_123" {
		t.Errorf("WebhookAAD salah: dapat %q, ingin webhook:wh_123", got)
	}
	if WebhookAAD("wh_1") == WebhookAAD("wh_2") {
		t.Error("WebhookAAD tidak boleh sama untuk ID webhook yang berbeda")
	}
	if WebhookAAD("123") == CredentialAAD("123") {
		t.Error("WebhookAAD tidak boleh bertabrakan dengan CredentialAAD untuk ID yang sama")
	}
}

// TestCLIToolAADDiikatKeIDTool memastikan AAD API key perkakas CLI terikat ke
// ID tool dan tidak dapat dipindahkan antar tool (ciphertext splicing).
func TestCLIToolAADDiikatKeIDTool(t *testing.T) {
	if got := CLIToolAAD("claude_code"); got != "cli_tool:claude_code" {
		t.Errorf("CLIToolAAD salah: dapat %q, ingin cli_tool:claude_code", got)
	}
	if CLIToolAAD("aider") == CLIToolAAD("opencode") {
		t.Error("CLIToolAAD tidak boleh sama untuk ID tool yang berbeda")
	}
	if CLIToolAAD("123") == CredentialAAD("123") {
		t.Error("CLIToolAAD tidak boleh bertabrakan dengan CredentialAAD untuk ID yang sama")
	}
	if CLIToolAAD("123") == WebhookAAD("123") {
		t.Error("CLIToolAAD tidak boleh bertabrakan dengan WebhookAAD untuk ID yang sama")
	}
}

// TestOAuthSessionAADDiikatKeIDSesi memastikan AAD refresh token OAuth terikat
// ke ID sesi dan tidak dapat dipindahkan antar sesi provider.
func TestOAuthSessionAADDiikatKeIDSesi(t *testing.T) {
	if got := OAuthSessionAAD("sess_1"); got != "provider_oauth:sess_1" {
		t.Errorf("OAuthSessionAAD salah: dapat %q, ingin provider_oauth:sess_1", got)
	}
	if OAuthSessionAAD("a") == OAuthSessionAAD("b") {
		t.Error("OAuthSessionAAD tidak boleh sama untuk ID sesi yang berbeda")
	}
	if OAuthSessionAAD("123") == CredentialAAD("123") {
		t.Error("OAuthSessionAAD tidak boleh bertabrakan dengan CredentialAAD untuk ID yang sama")
	}
	if OAuthSessionAAD("123") == CLIToolAAD("123") {
		t.Error("OAuthSessionAAD tidak boleh bertabrakan dengan CLIToolAAD untuk ID yang sama")
	}
}
