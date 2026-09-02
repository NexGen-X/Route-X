// Package auth memuat autentikasi dan otorisasi admin dashboard: login beserta
// perlindungan brute force, sesi berbasis cookie, perlindungan CSRF, dan pemeriksaan
// izin RBAC.
//
// Paket ini adalah satu-satunya tempat yang menerjemahkan "cookie di request" menjadi
// "siapa pengguna ini dan boleh apa". Lapisan HTTP di atasnya hanya memasang middleware;
// lapisan repository di bawahnya tidak pernah mengambil keputusan kewenangan.
//
// Empat keputusan berlaku di seluruh paket ini:
//
//  1. Kegagalan login selalu sama di mata klien. Email yang tidak terdaftar, password
//     yang salah, dan akun yang terkunci menghasilkan pesan yang identik dan status HTTP
//     yang sama — dan LoginError.Error() sendiri hanya mengembalikan pesan seragam itu,
//     sehingga pemanggil yang keliru menuliskan err.Error() ke klien pun tidak bisa
//     membocorkan perbedaannya. Alasan sebenarnya hidup di LoginError.Reason, yang hanya
//     dipakai untuk log dan audit.
//
//  2. Waktu jawaban juga tidak boleh membedakan. Email yang tidak ada tetap membayar satu
//     kali kerja argon2 lewat security.BurnVerifyTime. Tanpa itu, selisih puluhan
//     milidetik antara "akun ada" dan "akun tidak ada" menjadi alat enumerasi akun yang
//     jauh lebih murah daripada menebak password — dan enumerasi itulah langkah pertama
//     serangan password spraying.
//
//  3. Token sesi tidak pernah disimpan, di-log, atau bisa dibaca dua kali. Yang masuk
//     database hanya hash SHA-256-nya (lihat identity.Sessions), dan nilai mentahnya
//     melewati paket ini sekali saja, dalam perjalanan menjadi cookie.
//
//  4. CSRF hanya relevan untuk rute yang diautentikasi cookie; rute API yang memakai
//     Bearer API key tidak membutuhkannya sama sekali. Alasannya dijelaskan di csrf.go.
package auth

import (
	"errors"
	"fmt"
	"time"

	"github.com/NexGen-X/Route-X/internal/security"
)

// Sentinel yang dipakai pemanggil untuk memutuskan status HTTP.
var (
	// ErrLoginFailed adalah satu-satunya kegagalan login yang boleh dilihat klien.
	// Seluruh sebab — email tak terdaftar, password salah, akun terkunci, akun
	// dinonaktifkan — membungkus sentinel ini dengan pesan yang sama.
	ErrLoginFailed = errors.New(MessageLoginFailed)

	// ErrNoSession berarti request tidak membawa sesi yang berlaku. Tidak dibedakan
	// antara "tidak ada cookie", "token tidak dikenal", "sesi kedaluwarsa", dan "sesi
	// dicabut": perbedaan itu memberi tahu penyerang apakah token curiannya pernah sah.
	ErrNoSession = errors.New("sesi tidak ditemukan atau sudah tidak berlaku")

	// ErrWrongPassword berarti password lama pada permintaan ganti password tidak cocok.
	// Berbeda dari ErrLoginFailed: di sini pemanggil sudah terautentikasi, jadi
	// memberitahunya bahwa password lamanya salah tidak membocorkan apa pun.
	ErrWrongPassword = errors.New("password saat ini tidak cocok")

	// ErrWeakPassword berarti password baru tidak memenuhi kebijakan kekuatan. Error
	// yang mengembalikannya juga membungkus sentinel spesifik dari paket security
	// (security.ErrPasswordTooShort dan kawan-kawannya), sehingga pemanggil bisa
	// menyusun pesan yang menjelaskan aturan mana yang dilanggar.
	ErrWeakPassword = errors.New("password baru tidak memenuhi kebijakan")

	// ErrPasswordUnchanged berarti password baru sama dengan yang sekarang. Ditolak
	// karena permintaan seperti ini biasanya salah kirim, dan meloloskannya akan
	// mencabut seluruh sesi lain tanpa mengubah apa pun.
	ErrPasswordUnchanged = errors.New("password baru sama dengan password saat ini")
)

// Pesan yang boleh dilihat klien. Dikumpulkan di satu tempat supaya keseragamannya bisa
// diperiksa dengan mata — khususnya MessageLoginFailed, yang harus dipakai apa adanya
// untuk SETIAP kegagalan login.
const (
	// MessageLoginFailed sengaja tidak menyebut bagian mana yang salah.
	MessageLoginFailed = "email atau password salah"
	// MessageLoginRateLimited muncul saat percobaan login dari satu IP atau untuk satu
	// email terlalu sering. Ini tidak membocorkan keberadaan akun: penghitungnya naik
	// pada setiap percobaan, termasuk untuk email yang tidak terdaftar.
	MessageLoginRateLimited = "percobaan login terlalu sering, coba lagi beberapa saat lagi"
	// MessageSessionRequired dipakai untuk seluruh kegagalan sesi.
	MessageSessionRequired = "sesi tidak sah atau sudah berakhir, silakan masuk kembali"
	// MessageForbidden dipakai saat sesi sah tetapi izinnya tidak cukup. Izin yang
	// kurang tidak disebutkan di sini, hanya di log.
	MessageForbidden = "akses ke sumber daya ini tidak diizinkan"
	// MessagePasswordChangeRequired dipakai saat pengguna wajib mengganti password
	// lebih dulu.
	MessagePasswordChangeRequired = "password wajib diganti sebelum memakai fitur lain"
	// MessageCSRFInvalid dipakai untuk seluruh kegagalan pemeriksaan CSRF, tanpa
	// menyebut apakah yang bermasalah token, cookie, atau Origin.
	MessageCSRFInvalid = "permintaan ditolak: token CSRF tidak sah"
)

// Kode error yang dibaca mesin. Nilainya bagian dari kontrak dengan dashboard, jadi
// harus stabil walau teks pesannya berubah.
//
// Paket ini memakai kodenya sendiri alih-alih httpx.CodeInvalidAPIKey yang dipasang
// httpx.Unauthorized: envelope, tipe, dan status-nya sama persis, tetapi dashboard perlu
// membedakan "password salah di form login" dari "sesi habis" dan dari "wajib ganti
// password" — ketiganya 401/403 dengan tindak lanjut yang sama sekali berbeda, dan
// "invalid_api_key" tidak menjelaskan satu pun di antaranya.
const (
	// CodeLoginFailed: kredensial pada form login salah.
	CodeLoginFailed = "login_failed"
	// CodeSessionRequired: tidak ada sesi yang berlaku; dashboard harus mengarahkan ke
	// halaman login.
	CodeSessionRequired = "session_required"
	// CodePasswordChangeRequired: sesi sah, tetapi pengguna wajib mengganti password
	// lebih dulu; dashboard harus mengarahkan ke form ganti password.
	CodePasswordChangeRequired = "password_change_required"
	// CodeCSRFInvalid: pemeriksaan CSRF gagal. Dashboard sebaiknya mengambil ulang
	// token lewat GET /me lalu mencoba sekali lagi.
	CodeCSRFInvalid = "csrf_invalid"
	// CodeCurrentPasswordInvalid: password lama pada form ganti password salah.
	CodeCurrentPasswordInvalid = "current_password_invalid"
	// CodeWeakPassword: password baru ditolak kebijakan kekuatan.
	CodeWeakPassword = "weak_password"
	// CodePasswordUnchanged: password baru sama dengan yang sekarang.
	CodePasswordUnchanged = "password_unchanged"
	// CodeUnsupportedMediaType: body bukan application/json.
	CodeUnsupportedMediaType = "unsupported_media_type"
)

// FailureReason adalah sebab sebenarnya sebuah login gagal.
//
// Nilai ini TIDAK pernah dikirim ke klien. Ia ada supaya log dan audit tetap bisa
// menjawab pertanyaan operator ("apakah ada yang menebak password akun ini, atau ada
// yang mencari-cari alamat email?") tanpa memaksa pesan ke klien membocorkannya.
type FailureReason string

// Nilai FailureReason yang mungkin.
const (
	// ReasonUnknownEmail: tidak ada akun dengan email itu.
	ReasonUnknownEmail FailureReason = "unknown_email"
	// ReasonBadPassword: akun ada, password salah.
	ReasonBadPassword FailureReason = "bad_password"
	// ReasonLocked: akun sedang terkunci sementara akibat percobaan gagal berturut-turut.
	ReasonLocked FailureReason = "locked"
	// ReasonDisabled: akun tidak berstatus active, yaitu dimatikan atau dikunci admin.
	ReasonDisabled FailureReason = "disabled"
	// ReasonRateLimited: batas laju percobaan login terlampaui.
	ReasonRateLimited FailureReason = "rate_limited"
	// ReasonBrokenHash: hash password tersimpan tidak bisa diparse. Akun itu tidak akan
	// pernah bisa masuk sampai passwordnya direset, jadi kemunculannya di log adalah
	// tanda kerusakan data, bukan tanda serangan.
	ReasonBrokenHash FailureReason = "broken_hash"
)

// LoginError adalah kegagalan login.
//
// Error() sengaja hanya mengembalikan MessageLoginFailed, tanpa menyebut Reason. Itu
// bukan kelalaian: pesan error adalah nilai yang paling mudah tanpa sengaja diteruskan
// ke klien, jadi tipe ini dibuat aman bahkan ketika dipakai salah. Yang membutuhkan
// sebabnya membaca field Reason.
type LoginError struct {
	// Reason adalah sebab internal, untuk log dan audit.
	Reason FailureReason
	// RetryAfter terisi untuk ReasonRateLimited: perkiraan lama tunggu sebelum
	// percobaan berikutnya diterima. Nol untuk sebab lainnya.
	RetryAfter time.Duration
}

func (e *LoginError) Error() string { return MessageLoginFailed }

// Unwrap membuat errors.Is(err, ErrLoginFailed) bekerja untuk seluruh sebab.
func (e *LoginError) Unwrap() error { return ErrLoginFailed }

// loginFailure membentuk LoginError. Dipakai supaya setiap jalur gagal di Login pasti
// memakai bentuk error yang sama.
func loginFailure(reason FailureReason) error {
	return &LoginError{Reason: reason}
}

// weakPasswordError membungkus kegagalan validasi kekuatan password.
//
// Dua sentinel dibungkus sekaligus: ErrWeakPassword untuk pemanggil yang hanya perlu
// tahu kategorinya, dan error asli dari paket security supaya aturan spesifik yang
// dilanggar tetap bisa dikenali dengan errors.Is dan dijelaskan ke pengguna.
func weakPasswordError(op string, err error) error {
	return fmt.Errorf("%s: %w: %w", op, ErrWeakPassword, err)
}

// PasswordRuleMessage menerjemahkan kegagalan kekuatan password menjadi pesan yang aman
// dan berguna bagi pengguna.
//
// Pesan dari paket security dipakai apa adanya karena ketiganya memang ditulis untuk
// dibaca manusia dan tidak memuat nilai password. Aturan yang tidak dikenali jatuh ke
// pesan generik: menempelkan err.Error() apa adanya akan ikut membawa awalan operasi
// internal ke respons API.
func PasswordRuleMessage(err error) string {
	switch {
	case errors.Is(err, security.ErrPasswordTooShort):
		return security.ErrPasswordTooShort.Error()
	case errors.Is(err, security.ErrPasswordTooLong):
		return security.ErrPasswordTooLong.Error()
	case errors.Is(err, security.ErrPasswordCommon):
		return security.ErrPasswordCommon.Error()
	case errors.Is(err, security.ErrPasswordNoVariety):
		return security.ErrPasswordNoVariety.Error()
	default:
		return ErrWeakPassword.Error()
	}
}
