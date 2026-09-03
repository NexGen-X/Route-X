// Package apikey memuat autentikasi dan otorisasi jalur API /v1/* — jalur yang dipakai
// klien OpenAI-compatible, bukan dashboard. Padanannya untuk dashboard adalah paket
// auth, yang bekerja dengan cookie sesi dan RBAC; di sini kredensialnya API key dan
// kewenangannya cakupan (scope).
//
// Lima keputusan berlaku di seluruh paket ini.
//
//  1. Kredensial hanya diterima dari header, dan hanya dari dua header: Authorization
//     dengan skema Bearer, lalu X-Api-Key. Query string tidak pernah dibaca — lihat
//     Credential untuk alasannya.
//
//  2. Seluruh kegagalan autentikasi menghasilkan 401 dengan body yang identik byte per
//     byte. Tidak ada perbedaan antara key yang tidak ada, dicabut, kedaluwarsa,
//     diblokir, dan datang dari IP terlarang. Perbedaan itu hidup di log dan metrik.
//     Lihat Authenticator.Authenticate.
//
//  3. Kredensial sah tetapi kewenangannya kurang menghasilkan 403 — dan hanya di situ
//     403 dipakai, karena pada titik itu klien sudah membuktikan key-nya sah sehingga
//     tidak ada informasi baru yang bocor.
//
//  4. Nilai API key mentah tidak pernah masuk log, pesan error, maupun label metrik.
//     Yang boleh ditulis adalah bentuk tersamar dari Principal.Masked dan ID barisnya.
//
//  5. Pembatas laju gagal terbuka bila Redis mati. Lihat Limiter.
//
// Bentuk error mengikuti envelope OpenAI lewat paket httpx: klien resmi OpenAI mengurai
// error.type dan error.code, jadi tidak ada respons di paket ini yang menulis JSON-nya
// sendiri.
package apikey

import (
	"context"
	"errors"
	"fmt"
	"net/http"
	"slices"
	"strings"
	"unicode"

	"github.com/NexGen-X/Route-X/internal/database/repo/keys"
)

// Header yang dibaca untuk mencari kredensial.
const (
	// HeaderAuthorization adalah header utama: "Authorization: Bearer sk_live_...".
	HeaderAuthorization = "Authorization"
	// HeaderAPIKey adalah header alternatif. Ada karena sebagian klien memakainya —
	// SDK Anthropic mengirim "x-api-key" — dan tujuan gateway ini adalah cukup menukar
	// base URL dan API key pada aplikasi yang sudah jalan, tanpa mengubah cara ia
	// mengirim kredensial. Nama di sini bentuk kanonik Go; header dari kabel
	// dikanonikalisasi net/http, jadi "x-api-key" tetap cocok.
	HeaderAPIKey = "X-Api-Key"
)

// schemeBearer dibandingkan tanpa memandang besar kecil huruf: RFC 7235 menyatakan
// nama skema autentikasi tidak sensitif huruf, dan di lapangan "bearer" huruf kecil
// benar-benar muncul.
const schemeBearer = "bearer"

// Kegagalan pengambilan kredensial. Ketiganya berakhir sebagai 401 dengan body yang
// sama; yang membedakannya hanya label yang masuk log dan metrik.
var (
	// ErrNoCredential berarti request tidak membawa kredensial sama sekali.
	ErrNoCredential = errors.New("API key tidak disertakan")
	// ErrMalformedCredential berarti header ada tetapi bentuknya bukan yang diterima,
	// misalnya skema selain Bearer atau "Bearer" tanpa nilai.
	ErrMalformedCredential = errors.New("bentuk header kredensial tidak dikenali")
	// ErrConflictingCredential berarti request membawa lebih dari satu kredensial yang
	// nilainya berbeda.
	ErrConflictingCredential = errors.New("kredensial yang dikirim saling bertentangan")
)

// Credential mengambil API key mentah dari request.
//
// # Urutan prioritas
//
// Authorization diperiksa lebih dulu, lalu X-Api-Key. Prioritas itu hanya menentukan
// pesan kegagalan mana yang menang, bukan nilai mana yang dipakai: bila kedua header
// membawa nilai dan nilainya BERBEDA, request ditolak. Diam-diam memilih salah satu
// akan membuat klien yang salah konfigurasi — misalnya masih mengirim key lama di satu
// header setelah rotasi — tampak berhasil sesekali dan gagal di lain waktu, dan itu
// jenis kegagalan yang paling lama dicari orang. Nilai yang sama di kedua header
// diterima, karena itu kejadian yang wajar pada klien yang memasang keduanya.
//
// Header yang sama muncul berulang diperlakukan sama: semuanya harus bernilai identik.
//
// # Yang tidak diterima
//
// Authorization wajib memakai skema Bearer. Nilai tanpa skema ("Authorization:
// sk_live_...") maupun skema lain ("Basic ...") ditolak alih-alih dicoba sebagai key;
// klien yang tidak mau mengirim skema punya jalur resmi lewat X-Api-Key.
//
// Kredensial TIDAK PERNAH diambil dari query string, walau sebagian API lain
// menyediakannya. Query string ikut tercatat di log akses server dan proxy, tersimpan
// di riwayat browser, dan terkirim ke pihak ketiga lewat header Referer — tiga tempat
// yang tidak berada di bawah kendali kita dan tidak punya masa berlaku. Sekali sebuah
// key mendarat di sana, ia harus dianggap bocor. Header tidak punya satu pun dari sifat
// itu.
//
// Panjang dan bentuk key tidak diperiksa di sini; itu tugas repo keys yang memegang
// aturan formatnya (security.ParseAPIKey), sehingga hanya ada satu tempat yang tahu
// bentuk key yang sah.
func Credential(r *http.Request) (string, error) {
	if r == nil || r.Header == nil {
		return "", ErrNoCredential
	}

	var candidates []string

	for _, value := range r.Header.Values(HeaderAuthorization) {
		token, err := bearerToken(value)
		if err != nil {
			return "", err
		}
		if token != "" {
			candidates = append(candidates, token)
		}
	}

	for _, value := range r.Header.Values(HeaderAPIKey) {
		token, ok := cleanToken(value)
		if !ok {
			return "", fmt.Errorf("%w: %s memuat spasi", ErrMalformedCredential, HeaderAPIKey)
		}
		if token != "" {
			candidates = append(candidates, token)
		}
	}

	if len(candidates) == 0 {
		return "", ErrNoCredential
	}
	for _, c := range candidates[1:] {
		if c != candidates[0] {
			// Perbandingan biasa, bukan waktu konstan: kedua nilai berasal dari
			// request yang sama dan sepenuhnya dikuasai pengirimnya, jadi tidak ada
			// rahasia kami yang bisa disimpulkan dari lama perbandingan.
			return "", ErrConflictingCredential
		}
	}
	return candidates[0], nil
}

// bearerToken membaca satu nilai header Authorization.
//
// Mengembalikan ("", nil) bila header ada tetapi kosong, sehingga header kosong sama
// artinya dengan header yang tidak dikirim.
func bearerToken(value string) (string, error) {
	v := strings.TrimSpace(value)
	if v == "" {
		return "", nil
	}

	// Pemisah skema dicari sebagai spasi jenis apa pun, bukan hanya " ": klien yang
	// merakit header sendiri sesekali memakai tab.
	i := strings.IndexFunc(v, unicode.IsSpace)
	if i < 0 {
		if strings.EqualFold(v, schemeBearer) {
			return "", fmt.Errorf("%w: Bearer tanpa nilai", ErrMalformedCredential)
		}
		return "", fmt.Errorf("%w: %s tanpa skema Bearer", ErrMalformedCredential, HeaderAuthorization)
	}
	if !strings.EqualFold(v[:i], schemeBearer) {
		// Nama skema tidak dikutip ke dalam pesan error. Pesan ini tidak pernah sampai
		// ke klien — pemanggil menggantinya dengan pesan seragam — tapi ia bisa
		// mendarat di log, dan nilai header adalah tempat terakhir yang layak dipercaya.
		return "", fmt.Errorf("%w: skema %s bukan Bearer", ErrMalformedCredential, HeaderAuthorization)
	}

	// Nilai sudah dipangkas di kedua ujung, jadi bagian setelah skema pasti memuat
	// sesuatu; yang masih mungkin salah hanyalah adanya spasi di tengahnya.
	token, ok := cleanToken(v[i:])
	if !ok {
		return "", fmt.Errorf("%w: nilai Bearer memuat spasi", ErrMalformedCredential)
	}
	return token, nil
}

// cleanToken memangkas spasi di pinggir dan menolak spasi di tengah.
//
// Spasi berlebih di pinggir adalah kecelakaan yang lazim (salin-tempel, nilai dari file
// env yang belum dirapikan) dan aman dipangkas. Spasi di tengah bukan: ia berarti yang
// dikirim bukan satu nilai, dan menebak bagian mana yang dimaksud lebih buruk daripada
// menolak.
func cleanToken(value string) (string, bool) {
	v := strings.TrimSpace(value)
	if strings.IndexFunc(v, unicode.IsSpace) >= 0 {
		return "", false
	}
	return v, true
}

// --- Principal ---------------------------------------------------------------

// principalCtxKey adalah kunci context untuk principal.
//
// Tipe kosong yang tidak diekspor, bukan string: paket lain tidak bisa menuliskan nilai
// ke kunci yang sama, baik karena kebetulan memakai string yang sama maupun dengan
// sengaja. Satu-satunya jalan menaruh principal di context adalah WithPrincipal.
type principalCtxKey struct{}

// Principal adalah API key yang sudah terautentikasi.
//
// Isinya cuplikan baris api_keys pada saat kredensial diperiksa, bukan penunjuk hidup
// ke database: satu request memakai satu gambaran kewenangan yang tetap, sehingga
// pencabutan atau perubahan batas di tengah request tidak membuat separuh handler
// memakai aturan lama dan separuh lagi aturan baru. Perubahan itu berlaku pada request
// berikutnya.
//
// Nilai key mentah tidak ada di sini, karena keys.Key tidak pernah memuatnya. Yang
// tersedia untuk log dan tampilan hanyalah Masked.
type Principal struct {
	Key *keys.Key
}

// ID adalah UUID baris api_keys. Ini pengenal yang dipakai untuk kunci rate limit,
// pencatatan pemakaian, dan penautan biaya — bukan nilai key-nya.
func (p *Principal) ID() string {
	if p == nil || p.Key == nil {
		return ""
	}
	return p.Key.ID
}

// OwnerUserID adalah pemilik key, dipakai handler untuk membatasi data yang dilihat dan
// untuk menautkan pemakaian ke penagihan.
func (p *Principal) OwnerUserID() string {
	if p == nil || p.Key == nil {
		return ""
	}
	return p.Key.OwnerUserID
}

// Masked mengembalikan bentuk tersamar, mis. "sk_live_****9a21". Ini satu-satunya
// bentuk key yang boleh ditulis ke log atau ditampilkan.
func (p *Principal) Masked() string {
	if p == nil || p.Key == nil {
		return ""
	}
	return p.Key.Masked()
}

// Can melaporkan apakah key memiliki satu cakupan. Cakupan kosong selalu false, supaya
// "tanpa syarat cakupan" tidak bisa ditulis dengan memanggil ini memakai string kosong.
// RequestLimits mengembalikan batas berbasis jumlah request milik key ini.
//
// Aman pada penerima nil, seperti seluruh method Principal lainnya. Itu bukan kenyamanan:
// jalur yang tidak berautentikasi API key mendapat principal nil dari context, dan pembacaan
// p.Key di sana adalah nil-pointer yang hanya muncul di produksi pada rute yang jarang
// dilewati.
func (p *Principal) RequestLimits() Limits {
	if p == nil {
		return Limits{}
	}
	return LimitsFromKey(p.Key)
}

// TokenLimits mengembalikan batas berbasis jumlah token milik key ini. Aman pada penerima nil.
func (p *Principal) TokenLimits() TokenLimits {
	if p == nil {
		return TokenLimits{}
	}
	return TokenLimitsFromKey(p.Key)
}

func (p *Principal) Can(scope string) bool {
	if p == nil || p.Key == nil || scope == "" {
		return false
	}
	return p.Key.HasScope(scope)
}

// Scopes mengembalikan salinan daftar cakupan, selalu non-nil supaya aman
// diserialisasi sebagai [] alih-alih null.
//
// Salinan, bukan slice aslinya: handler yang mengurutkan atau memangkas hasilnya tidak
// boleh mengubah cuplikan kewenangan yang dipakai middleware di request yang sama.
func (p *Principal) Scopes() []string {
	if p == nil || p.Key == nil || len(p.Key.Scopes) == 0 {
		return []string{}
	}
	return slices.Clone(p.Key.Scopes)
}

// WithPrincipal menautkan principal ke context.
//
// Diekspor untuk dua pemakai: middleware di paket ini, dan test atau handler yang perlu
// dijalankan dengan identitas tertentu tanpa melewati seluruh jalur autentikasi.
func WithPrincipal(ctx context.Context, p *Principal) context.Context {
	return context.WithValue(ctx, principalCtxKey{}, p)
}

// PrincipalFrom mengambil principal dari context. Nilai kedua false berarti request ini
// belum melewati Authenticator.Authenticate.
func PrincipalFrom(ctx context.Context) (*Principal, bool) {
	p, ok := ctx.Value(principalCtxKey{}).(*Principal)
	return p, ok && p != nil && p.Key != nil
}
