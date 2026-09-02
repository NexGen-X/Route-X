package upstream

import (
	"encoding/json"
	"errors"
	"fmt"
	"strconv"
	"strings"
)

// ErrInvalidAmount dikembalikan saat teks tidak bisa dibaca sebagai nilai uang.
var ErrInvalidAmount = errors.New("nilai uang tidak sah")

// Skala nilai uang di seluruh paket ini.
const (
	// USDScale adalah jumlah angka desimal yang disimpan tipe USD.
	//
	// Delapan, mengikuti kolom requests.cost_usd numeric(16,8): satu request murah
	// bisa berbiaya di bawah satu mikro-dolar, dan membulatkannya lebih awal berarti
	// selisih itu hilang seluruhnya saat ditagihkan per bulan.
	USDScale = 8

	// usdUnitsPerUSD adalah jumlah satuan terkecil dalam satu dolar, 10^USDScale.
	usdUnitsPerUSD = 100_000_000

	// priceScale adalah skala kolom harga model_pricing (numeric(14,6)).
	priceScale = 6

	// usdUnitsPerPriceUnit adalah faktor antara satuan USD dan satuan terkecil yang
	// bisa disimpan kolom harga, 10^(USDScale-priceScale).
	usdUnitsPerPriceUnit = 100

	// tokensPerMTok adalah pembagi harga: semua harga dinyatakan per satu juta token.
	tokensPerMTok = 1_000_000
)

// USD adalah nilai uang dalam dolar AS sebagai bilangan bulat satuan 10^-8 USD.
//
// float64 sengaja tidak dipakai. Angka seperti 0,1 tidak punya wujud biner yang
// tepat, sehingga penjumlahan biaya ratusan ribu request akan menyimpang dari total
// yang dihitung PostgreSQL dengan numeric — dan selisih pada nilai uang selalu
// menjadi laporan bug. Bilangan bulat bersatuan tetap tidak pernah menyimpang:
// setiap nilai punya satu wujud, penjumlahan bersifat eksak, dan pembulatan hanya
// terjadi di tempat yang kita pilih sendiri.
//
// Skala 8 dipilih karena itulah skala terlebar yang dipakai skema (requests.cost_usd).
// Harga per juta token disimpan dengan skala 6 (numeric(14,6)), yang selalu bisa
// diwakili tepat pada skala 8; IsPriceScale memeriksa arah sebaliknya.
//
// Rentangnya jauh lebih dari cukup: int64 menampung sampai sekitar 92 miliar USD.
type USD int64

// ParseUSD membaca nilai uang dari teks desimal, mis. "2.5" atau "0.00000015".
//
// Angka desimal yang lebih banyak daripada USDScale ditolak, bukan dibulatkan: kalau
// pemanggil mengirim presisi yang tidak bisa kami simpan, membulatkannya diam-diam akan
// menyembunyikan salah harga sampai muncul di tagihan. Nol di ujung dikecualikan karena
// tidak menambah presisi.
func ParseUSD(s string) (USD, error) {
	t := strings.TrimSpace(s)
	if t == "" {
		return 0, fmt.Errorf("%w: teks kosong", ErrInvalidAmount)
	}

	neg := false
	switch t[0] {
	case '+':
		t = t[1:]
	case '-':
		neg = true
		t = t[1:]
	}

	whole, frac := t, ""
	if i := strings.IndexByte(t, '.'); i >= 0 {
		whole, frac = t[:i], t[i+1:]
	}
	if whole == "" && frac == "" {
		return 0, fmt.Errorf("%w: %q tidak memuat angka", ErrInvalidAmount, s)
	}
	// Nol di ujung tidak membawa presisi apa pun, jadi "2.500000000" diterima sementara
	// "2.500000001" tetap ditolak.
	if len(frac) > USDScale {
		if trimmed := strings.TrimRight(frac, "0"); len(trimmed) <= USDScale {
			frac = trimmed
		}
	}
	if len(frac) > USDScale {
		return 0, fmt.Errorf("%w: %q memakai %d angka desimal, maksimum %d",
			ErrInvalidAmount, s, len(frac), USDScale)
	}

	// Bagian bulat dan desimal disambung lalu dipadatkan menjadi satu bilangan bulat
	// bersatuan 10^-8, sehingga tidak ada pembagian floating point sama sekali.
	digits := whole + frac + strings.Repeat("0", USDScale-len(frac))
	units, err := strconv.ParseInt(digits, 10, 64)
	if err != nil {
		if errors.Is(err, strconv.ErrRange) {
			return 0, fmt.Errorf("%w: %q di luar rentang yang bisa disimpan", ErrInvalidAmount, s)
		}
		return 0, fmt.Errorf("%w: %q bukan angka desimal", ErrInvalidAmount, s)
	}
	if neg {
		units = -units
	}
	return USD(units), nil
}

// MustParseUSD seperti ParseUSD tetapi panik bila gagal. Hanya untuk konstanta yang
// tertulis di kode dan untuk test — jangan dipakai pada masukan dari luar.
func MustParseUSD(s string) USD {
	v, err := ParseUSD(s)
	if err != nil {
		panic(err)
	}
	return v
}

// Units mengembalikan nilai dalam satuan 10^-8 USD. Dipakai saat nilai harus melewati
// batas paket sebagai bilangan bulat biasa.
func (a USD) Units() int64 { return int64(a) }

// Decimal mengembalikan wujud desimal eksak dengan USDScale angka desimal, mis.
// "2.50000000". Inilah bentuk yang dikirim ke PostgreSQL sebagai numeric.
func (a USD) Decimal() string {
	sign := ""
	// Nilai negatif dibalik lewat uint64 supaya USD(math.MinInt64) pun tidak meluap.
	u := uint64(a)
	if a < 0 {
		sign = "-"
		u = -uint64(a)
	}
	return fmt.Sprintf("%s%d.%0*d", sign, u/usdUnitsPerUSD, USDScale, u%usdUnitsPerUSD)
}

// String memenuhi fmt.Stringer dengan bentuk yang sama seperti Decimal, sehingga nilai
// uang tidak pernah tercetak sebagai bilangan bulat satuan mentah di log.
func (a USD) String() string { return a.Decimal() }

// IsPriceScale melaporkan apakah nilai ini bisa disimpan tepat pada kolom harga
// numeric(14,6).
//
// Diperiksa sebelum menulis harga: numeric(14,6) membulatkan apa pun yang lebih
// presisi, dan harga yang diam-diam berubah saat disimpan adalah kesalahan yang baru
// terlihat setelah menagih pengguna dengan angka yang salah.
func (a USD) IsPriceScale() bool { return int64(a)%usdUnitsPerPriceUnit == 0 }

// MarshalJSON menulis nilai sebagai string JSON.
//
// Angka JSON dibaca banyak klien (termasuk JavaScript) sebagai floating point ganda,
// yang justru membatalkan alasan tipe ini ada. String menjaga nilainya utuh.
func (a USD) MarshalJSON() ([]byte, error) { return json.Marshal(a.Decimal()) }

// UnmarshalJSON menerima string ("2.50") maupun angka JSON (2.50). Angka pun diurai
// dari teks aslinya, tanpa melewati float64.
func (a *USD) UnmarshalJSON(b []byte) error {
	text := strings.TrimSpace(string(b))
	if text == "null" {
		*a = 0
		return nil
	}
	if len(text) > 0 && text[0] == '"' {
		var s string
		if err := json.Unmarshal(b, &s); err != nil {
			return fmt.Errorf("%w: %v", ErrInvalidAmount, err)
		}
		text = s
	}
	v, err := ParseUSD(text)
	if err != nil {
		return err
	}
	*a = v
	return nil
}

// Verifikasi kontrak yang diandalkan lapisan HTTP dan log.
var (
	_ fmt.Stringer     = USD(0)
	_ json.Marshaler   = USD(0)
	_ json.Unmarshaler = (*USD)(nil)
)
