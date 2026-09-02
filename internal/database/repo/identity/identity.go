// Package identity memuat repository untuk identitas admin: pengguna, RBAC, sesi
// dashboard, catatan audit, dan setelan runtime.
//
// Tiga keputusan berlaku di seluruh paket ini:
//
//  1. Setiap tipe menyimpan repo.Querier, bukan *pgxpool.Pool, sehingga method yang
//     sama bisa dipakai di dalam maupun di luar transaksi. Operasi yang harus atomik
//     — membuat pengguna sekaligus memberinya peran, mengganti password sekaligus
//     mencabut seluruh sesinya — dirakit pemanggil lewat repo.InTx tanpa perlu
//     method duplikat di sini.
//
//  2. UUID dan inet dibaca sebagai teks (id::text, host(ip)). pgx tidak bisa
//     memindai kedua tipe itu ke dalam string, dan mengonversinya di SQL jauh lebih
//     murah daripada menyebarkan tipe pembungkus pgtype ke seluruh lapisan HTTP.
//     Ke arah sebaliknya string biasa bisa dikirim apa adanya sebagai parameter.
//
//  3. Semua error keluar lewat repo.Err, jadi pemanggil selalu bisa memakai
//     errors.Is dengan sentinel repo dan tidak pernah menerima pesan pgx mentah —
//     pesan pgx bisa memuat nilai baris, yang di sini berarti hash password atau
//     hash token sesi.
package identity

import (
	"encoding/base64"
	"errors"
	"fmt"
	"net/netip"
	"strconv"
	"strings"
	"time"
	"unicode/utf8"
)

// Sentinel milik paket ini, untuk kesalahan yang datang dari pemanggil dan tidak
// pernah menyentuh database. Lapisan HTTP memetakan keduanya ke 400.
var (
	// ErrInvalidCursor berarti cursor paginasi tidak bisa dibaca.
	ErrInvalidCursor = errors.New("cursor paginasi tidak valid")
	// ErrInvalidInput berarti argumen dari pemanggil tidak masuk akal, mis. status
	// pengguna di luar daftar yang sah atau masa berlaku sesi yang nol.
	ErrInvalidInput = errors.New("masukan tidak valid")
)

// --- Cursor paginasi keyset --------------------------------------------------

// cursorSeparator memisahkan penanda waktu dari pengenal di dalam cursor. Karakter
// ini tidak mungkin muncul pada UUID maupun bilangan, jadi pemisahannya tidak
// ambigu.
const cursorSeparator = "|"

// encodeCursor mengemas posisi baris terakhir sebuah halaman menjadi satu string
// buram.
//
// Waktu disimpan sebagai nanodetik Unix. Nilai yang dikemas selalu berasal dari
// database, yang presisinya mikrodetik, sehingga perjalanan bolak-baliknya tepat
// dan halaman berikutnya benar-benar bersambung dengan halaman sebelumnya.
//
// Cursor sengaja tidak ditandatangani: isinya hanya menentukan klausa WHERE atas
// baris yang memang sudah boleh dibaca pemanggil, jadi memalsukannya tidak membuka
// data apa pun.
func encodeCursor(at time.Time, id string) string {
	raw := strconv.FormatInt(at.UnixNano(), 10) + cursorSeparator + id
	return base64.RawURLEncoding.EncodeToString([]byte(raw))
}

// decodeCursor membongkar cursor menjadi posisi baris. Cursor yang rusak menghasilkan
// ErrInvalidCursor, bukan halaman pertama, supaya pemanggil tahu paginasinya patah
// alih-alih diam-diam mengulang dari awal.
func decodeCursor(cursor string) (at time.Time, id string, err error) {
	raw, err := base64.RawURLEncoding.DecodeString(cursor)
	if err != nil {
		return time.Time{}, "", fmt.Errorf("%w: bukan base64url", ErrInvalidCursor)
	}
	nanos, id, found := strings.Cut(string(raw), cursorSeparator)
	if !found || id == "" || nanos == "" {
		return time.Time{}, "", fmt.Errorf("%w: bentuknya tidak dikenali", ErrInvalidCursor)
	}
	n, err := strconv.ParseInt(nanos, 10, 64)
	if err != nil {
		return time.Time{}, "", fmt.Errorf("%w: penanda waktu tidak bisa dibaca", ErrInvalidCursor)
	}
	return time.Unix(0, n).UTC(), id, nil
}

// --- Perakit SQL -------------------------------------------------------------

// clauseList merakit potongan SQL beserta argumennya.
//
// Nilai tidak pernah masuk ke dalam string SQL: setiap potongan hanya menambah
// placeholder $N dan nilainya disimpan di args. Filter yang tidak dipakai karena itu
// tidak menyisakan jejak apa pun di query, dan itu penting untuk tabel sebesar
// audit_logs — pola "($1 is null or kolom = $1)" memaksa PostgreSQL memakai rencana
// generik yang mengabaikan indeks.
type clauseList struct {
	parts []string
	args  []any
}

// add menambahkan satu potongan. Pattern harus memuat %s tepat sebanyak values yang
// diberikan; setiap %s diganti placeholder milik nilai itu. Pattern selalu literal
// di dalam paket ini, jadi tidak ada data pemanggil yang bisa masuk ke SQL.
func (c *clauseList) add(pattern string, values ...any) {
	holders := make([]any, len(values))
	for i, v := range values {
		holders[i] = c.placeholder(v)
	}
	c.parts = append(c.parts, fmt.Sprintf(pattern, holders...))
}

// placeholder mendaftarkan satu nilai dan mengembalikan placeholder-nya.
func (c *clauseList) placeholder(v any) string {
	c.args = append(c.args, v)
	return "$" + strconv.Itoa(len(c.args))
}

// empty melaporkan apakah belum ada potongan yang ditambahkan.
func (c *clauseList) empty() bool { return len(c.parts) == 0 }

// join menggabungkan seluruh potongan dengan pemisah tertentu.
func (c *clauseList) join(sep string) string { return strings.Join(c.parts, sep) }

// where mengembalikan klausa WHERE lengkap, atau string kosong bila tanpa filter.
func (c *clauseList) where() string {
	if c.empty() {
		return ""
	}
	return " where " + c.join(" and ")
}

// --- Penyiap parameter -------------------------------------------------------

// textParam mengubah string kosong menjadi NULL. Kolom teks opsional di skema ini
// (display_name, user_agent, request_id, resource_id) memakai NULL untuk "tidak ada",
// bukan string kosong, supaya indeks dan COALESCE-nya berperilaku seragam.
func textParam(s string) any {
	if s == "" {
		return nil
	}
	return s
}

// ipParam menyiapkan nilai kolom inet dari sebuah string.
//
// Alamat yang tidak bisa diurai menjadi NULL, bukan error: nilainya berasal dari
// RemoteAddr atau header proxy yang tidak kita kendalikan, dan pencatatan login
// maupun audit tidak boleh gagal hanya karena X-Forwarded-For berisi sampah.
// Alamat IPv4-mapped dinormalkan supaya "::ffff:127.0.0.1" dan "127.0.0.1" tersimpan
// sebagai baris yang sama.
func ipParam(ip string) any {
	addr, err := netip.ParseAddr(strings.TrimSpace(ip))
	if err != nil {
		return nil
	}
	return addr.Unmap().String()
}

// seconds mengubah durasi menjadi detik pecahan untuk make_interval(secs => $n).
//
// make_interval dipakai daripada parameter bertipe interval supaya tidak bergantung
// pada dukungan encoding interval di driver, dan supaya batas waktunya dihitung dari
// jam database — bukan jam proses aplikasi, yang bisa berbeda antar replika.
func seconds(d time.Duration) float64 {
	if d < 0 {
		return 0
	}
	return d.Seconds()
}

// truncate memotong s ke paling banyak limit byte pada batas rune.
//
// Dipakai untuk nilai yang seluruhnya dikendalikan klien — User-Agent dan X-Request-ID
// — karena kolomnya bertipe text tanpa batas panjang: tanpa pemotongan ini, satu
// header raksasa cukup untuk menggelembungkan tabel sesi atau audit.
func truncate(s string, limit int) string {
	if len(s) <= limit {
		return s
	}
	cut := limit
	// Jangan meninggalkan rune yang terpotong separuh: mundur sampai awal rune.
	for cut > 0 && !utf8.RuneStart(s[cut]) {
		cut--
	}
	return s[:cut]
}

// Batas panjang nilai yang berasal dari klien.
const (
	maxUserAgentLen = 512
	maxRequestIDLen = 128
)

// validUUID melaporkan apakah s berbentuk UUID kanonik 8-4-4-4-12.
//
// Diperiksa di Go supaya ID sampah dari path URL tidak sampai ke PostgreSQL: server
// akan menjawabnya sebagai error sintaks (22P02) yang tidak punya sentinel, sehingga
// permintaan yang sebenarnya salah bentuk akan tampak sebagai kegagalan server.
// Bentuk lain yang sebenarnya diterima PostgreSQL (tanpa tanda hubung, dalam kurung
// kurawal) sengaja ditolak: seluruh ID yang kami keluarkan berbentuk kanonik.
func validUUID(s string) bool {
	if len(s) != 36 {
		return false
	}
	for i := 0; i < len(s); i++ {
		c := s[i]
		switch i {
		case 8, 13, 18, 23:
			if c != '-' {
				return false
			}
		default:
			switch {
			case c >= '0' && c <= '9', c >= 'a' && c <= 'f', c >= 'A' && c <= 'F':
			default:
				return false
			}
		}
	}
	return true
}
