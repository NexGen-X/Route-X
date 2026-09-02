package security

import (
	"crypto/rand"
	"crypto/subtle"
	"encoding/base64"
	"errors"
	"fmt"
	"strings"
	"sync"
	"unicode/utf8"

	"golang.org/x/crypto/argon2"
)

// Argon2Params memuat parameter biaya argon2id.
//
// argon2id dipilih (bukan bcrypt atau argon2i) karena tahan terhadap serangan GPU
// maupun side-channel, dan merupakan rekomendasi OWASP untuk password hashing baru.
type Argon2Params struct {
	Memory      uint32 // dalam KiB
	Iterations  uint32
	Parallelism uint8
	SaltLength  uint32
	KeyLength   uint32
}

// DefaultArgon2Params adalah biaya default: 64 MiB, 3 iterasi, 2 lane.
//
// Login admin adalah operasi jarang, jadi biaya setinggi ini tidak mengganggu
// pengalaman pakai, sementara membuat serangan offline jauh lebih mahal.
var DefaultArgon2Params = Argon2Params{
	Memory:      64 * 1024,
	Iterations:  3,
	Parallelism: 2,
	SaltLength:  16,
	KeyLength:   32,
}

// Batas panjang password. Batas atas mencegah penyerang memaksa server melakukan
// pekerjaan besar lewat input raksasa.
const (
	MinPasswordLength = 12
	MaxPasswordLength = 1024
)

var (
	// ErrPasswordTooShort dan kawan-kawannya dipakai lapisan auth untuk menyusun pesan
	// validasi yang ramah pengguna.
	ErrPasswordTooShort  = fmt.Errorf("password minimal %d karakter", MinPasswordLength)
	ErrPasswordTooLong   = fmt.Errorf("password maksimal %d karakter", MaxPasswordLength)
	ErrPasswordCommon    = errors.New("password terlalu umum dan mudah ditebak")
	ErrPasswordNoVariety = errors.New("password harus memuat minimal dua jenis karakter berbeda (huruf, angka, atau simbol)")

	// ErrInvalidHashFormat dikembalikan saat hash tersimpan tidak bisa diparse.
	ErrInvalidHashFormat = errors.New("format hash password tidak dikenal")
	// ErrIncompatibleVersion dikembalikan saat hash dibuat versi argon2 yang berbeda.
	ErrIncompatibleVersion = errors.New("versi argon2 pada hash tidak kompatibel")
)

// commonPasswords adalah daftar pendek password yang paling sering dipakai. Ini bukan
// pengganti kebijakan password menyeluruh, hanya penjaga terhadap pilihan terburuk —
// termasuk nilai default yang mungkin lupa diganti operator.
var commonPasswords = map[string]struct{}{
	"password":      {},
	"password123":   {},
	"password1234":  {},
	"123456789012":  {},
	"qwertyuiop":    {},
	"administrator": {},
	"changeme":      {},
	"changeme123":   {},
	"letmein12345":  {},
	"welcome12345":  {},
	"admin1234567":  {},
	"iloveyou1234":  {},
}

// ValidatePasswordStrength memeriksa kelayakan password sebelum di-hash.
func ValidatePasswordStrength(password string) error {
	// Daftar password umum diperiksa lebih dulu: pesan "terlalu umum" lebih
	// informatif daripada "terlalu pendek" untuk nilai seperti "changeme123", dan
	// dengan urutan ini setiap entri daftar tetap berguna walau lebih pendek dari
	// panjang minimum.
	if _, ok := commonPasswords[strings.ToLower(password)]; ok {
		return ErrPasswordCommon
	}
	if utf8.RuneCountInString(password) < MinPasswordLength {
		return ErrPasswordTooShort
	}
	if len(password) > MaxPasswordLength {
		return ErrPasswordTooLong
	}

	// Minimal dua kelas karakter: cukup untuk menyaring "aaaaaaaaaaaa" atau
	// "111111111111" tanpa memaksa aturan rumit yang justru mendorong pola buruk.
	var hasLetter, hasDigit, hasOther bool
	for _, r := range password {
		switch {
		case r >= 'a' && r <= 'z', r >= 'A' && r <= 'Z':
			hasLetter = true
		case r >= '0' && r <= '9':
			hasDigit = true
		default:
			hasOther = true
		}
	}
	classes := 0
	for _, ok := range []bool{hasLetter, hasDigit, hasOther} {
		if ok {
			classes++
		}
	}
	if classes < 2 {
		return ErrPasswordNoVariety
	}
	return nil
}

// HashPassword menghasilkan hash argon2id memakai parameter default, dalam format PHC:
//
//	$argon2id$v=19$m=65536,t=3,p=2$<salt base64>$<hash base64>
//
// Format ini menyimpan parameternya sendiri, sehingga biaya bisa ditingkatkan nanti
// tanpa membuat hash lama tidak terverifikasi.
func HashPassword(password string) (string, error) {
	return HashPasswordWith(password, DefaultArgon2Params)
}

// HashPasswordWith memakai parameter biaya khusus. Berguna di test agar cepat.
func HashPasswordWith(password string, p Argon2Params) (string, error) {
	if len(password) > MaxPasswordLength {
		return "", ErrPasswordTooLong
	}

	salt := make([]byte, p.SaltLength)
	if _, err := rand.Read(salt); err != nil {
		return "", fmt.Errorf("membuat salt: %w", err)
	}

	key := argon2.IDKey([]byte(password), salt, p.Iterations, p.Memory, p.Parallelism, p.KeyLength)

	return fmt.Sprintf("$argon2id$v=%d$m=%d,t=%d,p=%d$%s$%s",
		argon2.Version, p.Memory, p.Iterations, p.Parallelism,
		base64.RawStdEncoding.EncodeToString(salt),
		base64.RawStdEncoding.EncodeToString(key),
	), nil
}

// VerifyResult adalah hasil verifikasi password.
type VerifyResult struct {
	// Match menyatakan password cocok.
	Match bool
	// NeedsRehash menyatakan hash dibuat dengan parameter di bawah default saat ini,
	// jadi sebaiknya di-hash ulang secara transparan setelah login berhasil.
	NeedsRehash bool
}

// VerifyPassword membandingkan password dengan hash tersimpan dalam waktu konstan.
//
// Error hanya dikembalikan bila hash tersimpan tidak bisa diparse — password yang salah
// bukan error, melainkan Match=false, supaya pemanggil tidak keliru memperlakukan
// keduanya sebagai kondisi yang sama.
func VerifyPassword(password, encoded string) (VerifyResult, error) {
	p, salt, want, err := decodeHash(encoded)
	if err != nil {
		return VerifyResult{}, err
	}

	got := argon2.IDKey([]byte(password), salt, p.Iterations, p.Memory, p.Parallelism, p.KeyLength)

	return VerifyResult{
		Match:       subtle.ConstantTimeCompare(got, want) == 1,
		NeedsRehash: weakerThanDefault(p),
	}, nil
}

// weakerThanDefault melaporkan apakah parameter hash sudah di bawah standar sekarang.
func weakerThanDefault(p Argon2Params) bool {
	d := DefaultArgon2Params
	return p.Memory < d.Memory ||
		p.Iterations < d.Iterations ||
		p.KeyLength < d.KeyLength ||
		p.SaltLength < d.SaltLength
}

// decodeHash memecah string PHC menjadi parameter, salt, dan hash.
func decodeHash(encoded string) (p Argon2Params, salt, key []byte, err error) {
	parts := strings.Split(encoded, "$")
	// Format diawali "$", sehingga elemen pertama kosong: ["", "argon2id", "v=19", "m=..,t=..,p=..", salt, hash]
	if len(parts) != 6 || parts[0] != "" || parts[1] != "argon2id" {
		return p, nil, nil, ErrInvalidHashFormat
	}

	var version int
	if _, err := fmt.Sscanf(parts[2], "v=%d", &version); err != nil {
		return p, nil, nil, ErrInvalidHashFormat
	}
	if version != argon2.Version {
		return p, nil, nil, fmt.Errorf("%w: hash v=%d, dukungan v=%d", ErrIncompatibleVersion, version, argon2.Version)
	}

	if _, err := fmt.Sscanf(parts[3], "m=%d,t=%d,p=%d", &p.Memory, &p.Iterations, &p.Parallelism); err != nil {
		return p, nil, nil, ErrInvalidHashFormat
	}
	if p.Memory == 0 || p.Iterations == 0 || p.Parallelism == 0 {
		return p, nil, nil, ErrInvalidHashFormat
	}

	if salt, err = base64.RawStdEncoding.DecodeString(parts[4]); err != nil {
		return p, nil, nil, ErrInvalidHashFormat
	}
	if key, err = base64.RawStdEncoding.DecodeString(parts[5]); err != nil {
		return p, nil, nil, ErrInvalidHashFormat
	}
	if len(salt) == 0 || len(key) == 0 {
		return p, nil, nil, ErrInvalidHashFormat
	}

	p.SaltLength = uint32(len(salt))
	p.KeyLength = uint32(len(key))
	return p, salt, key, nil
}

// dummyHash adalah hash pembanding tetap yang dipakai BurnVerifyTime.
//
// Dijaga sync.Once, bukan pemeriksaan "kalau masih kosong": dua login bersamaan untuk
// email yang tidak terdaftar akan membaca dan menulis variabel ini serentak, dan itu
// data race sungguhan — terbukti dilaporkan race detector sebelum penyelarasan ini ada.
var (
	dummyHashOnce sync.Once
	dummyHash     string
)

// WarmUp menyiapkan hash pembanding yang dipakai BurnVerifyTime.
//
// Panggil sekali saat aplikasi start. Tanpa itu, panggilan BurnVerifyTime yang pertama
// harus menghitung hash penuh — jauh lebih lambat daripada satu verifikasi — sehingga
// login pertama untuk email tak terdaftar justru menonjol dari sisi waktu, kebalikan
// dari yang hendak dicapai.
func WarmUp() { dummyHashOnce.Do(buildDummyHash) }

func buildDummyHash() {
	// Kalau gagal, tidak ada yang bisa dilakukan selain melewatkannya — ini murni
	// pertahanan waktu, bukan jalur yang menentukan kebenaran.
	if h, err := HashPasswordWith("kata-sandi-pembanding-tetap", DefaultArgon2Params); err == nil {
		dummyHash = h
	}
}

// BurnVerifyTime melakukan pekerjaan argon2 setara satu verifikasi, lalu membuang
// hasilnya.
//
// Dipakai lapisan auth saat email yang dicari tidak ada: tanpa ini, permintaan untuk
// akun yang tidak terdaftar akan selesai jauh lebih cepat daripada akun yang ada, dan
// selisih waktunya menjadi alat enumerasi akun bagi penyerang.
//
// Aman dipanggil dari banyak goroutine sekaligus.
func BurnVerifyTime() {
	dummyHashOnce.Do(buildDummyHash)
	if dummyHash == "" {
		return
	}
	_, _ = VerifyPassword("kata-sandi-yang-pasti-salah", dummyHash)
}
