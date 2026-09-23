// Package upstream memuat akses data untuk sisi hulu gateway: provider, kredensial
// provider yang terenkripsi, jalur keluar (egress pool), registry model beserta alias
// dan pemetaannya ke provider, serta riwayat harga.
//
// Dua hal yang dijaga ketat di sini:
//
//  1. Rahasia tidak pernah menjadi teks biasa di database. Kredensial provider dan URL
//     proxy hanya masuk dan keluar lewat internal/security.Cipher, dengan AAD yang
//     mengikat ciphertext ke barisnya. Karena AAD memuat pengenal baris, pengenal itu
//     dibuat di Go sebelum enkripsi — lihat newID.
//  2. Nilai uang tidak pernah floating point. Harga dan biaya memakai tipe USD, yaitu
//     bilangan bulat bersatuan 10^-8 dolar — lihat money.go.
//
// Semua error yang keluar dari paket ini melewati repo.Err, sehingga pemanggil bisa
// memakai repo.ErrNotFound, ErrConflict, ErrInvalidReference, dan ErrConstraint tanpa
// mengenali detail driver.
package upstream

import (
	"crypto/rand"
	"errors"
	"fmt"
	"net/url"
	"strings"
	"time"

	"github.com/NexGen-X/Route-X/internal/database/repo"
	"github.com/NexGen-X/Route-X/internal/security"
)

// ErrNoChanges dikembalikan operasi pembaruan yang tidak diberi satu pun field untuk
// diubah. Dilaporkan sebagai error, bukan diam-diam sukses, karena hampir selalu berarti
// pemanggil salah merakit permintaannya.
var ErrNoChanges = errors.New("tidak ada perubahan yang diminta")

// ErrNeedsTx dikembalikan operasi yang wajib berjalan di dalam transaksi tetapi
// diberi Querier yang bukan transaksi. Lihat PricingRepo.Set.
var ErrNeedsTx = errors.New("operasi ini wajib dijalankan di dalam transaksi")

// Jenis provider. Harus sama dengan constraint providers_kind_valid di migrasi 0003.
const (
	KindOpenAI           = "openai"
	KindAnthropic        = "anthropic"
	KindGoogle           = "google"
	KindOpenAICompatible = "openai_compatible"
	KindCustom           = "custom"
)

// Status kesehatan. Sama dengan constraint providers_health_valid, egress_pool_health_valid,
// dan health_checks_status_valid.
const (
	HealthHealthy   = "healthy"
	HealthDegraded  = "degraded"
	HealthUnhealthy = "unhealthy"
)

// Kemampuan model. Sama dengan constraint models_capabilities_known di migrasi 0004.
// Kalau daftar ini berubah, migrasi baru harus mengubah constraint-nya juga.
const (
	CapText       = "text"
	CapVision     = "vision"
	CapReasoning  = "reasoning"
	CapTools      = "tools"
	CapEmbeddings = "embeddings"
)

// Strategi routing bawaan per model. Sama dengan constraint models_strategy_valid.
const (
	StrategyPriority      = "priority"
	StrategyRoundRobin    = "round_robin"
	StrategyWeighted      = "weighted"
	StrategyLowestLatency = "lowest_latency"
	StrategyLowestCost    = "lowest_cost"
	StrategyCapability    = "capability"
)

// Jenis jalur keluar. Sama dengan constraint egress_pool_kind_valid.
const (
	EgressHTTP   = "http"
	EgressHTTPS  = "https"
	EgressSOCKS5 = "socks5"
)

// Nilai bawaan yang mencerminkan default kolom di migrasi 0003.
//
// Disebut ulang di Go supaya setiap INSERT mengirim nilai konkret: dengan begitu satu
// baris punya arti yang sama entah dibuat lewat paket ini atau lewat SQL langsung, dan
// pemanggil tidak perlu menebak apa yang akan terjadi kalau field dibiarkan kosong.
const (
	DefaultPriority   = 100
	DefaultWeight     = 1
	DefaultTimeoutMS  = 120000
	DefaultMaxRetries = 2
	// DefaultCredentialLabel sama dengan default kolom provider_credentials.label.
	DefaultCredentialLabel = "primary"
	// DefaultPriceSource sama dengan default kolom model_pricing.source.
	DefaultPriceSource = "manual"
	// DefaultCurrency sama dengan default kolom model_pricing.currency.
	DefaultCurrency = "USD"
)

// pgxRow adalah antarmuka minimum yang dipenuhi pgx.Row maupun pgx.Rows, sehingga satu
// fungsi scan bisa melayani QueryRow dan iterasi Query.
type pgxRow interface {
	Scan(dest ...any) error
}

// Opt menyatakan sebuah field pembaruan sebagian.
//
// Pointer biasa tidak cukup untuk kolom yang boleh NULL: nil harus bisa berarti
// "biarkan seperti sekarang" sekaligus "kosongkan", dan keduanya adalah permintaan yang
// berbeda. Opt memisahkannya — Set menandai bahwa kolom ikut diubah, Value nil berarti
// diubah menjadi NULL.
type Opt[T any] struct {
	Set   bool
	Value *T
}

// Set menandai sebuah field untuk diubah menjadi v.
func Set[T any](v T) Opt[T] { return Opt[T]{Set: true, Value: &v} }

// Clear menandai sebuah field untuk dikosongkan (NULL).
func Clear[T any]() Opt[T] { return Opt[T]{Set: true} }

// arg mengembalikan nilai yang siap dikirim ke pgx: pointer nil menjadi NULL.
func (o Opt[T]) arg() any {
	if o.Value == nil {
		return nil
	}
	return *o.Value
}

// updateSet merakit klausa SET untuk pembaruan sebagian.
//
// Kolom yang tidak diminta berubah sengaja tidak ikut disebut, bukan ditulis ulang
// dengan nilainya sendiri lewat coalesce: dengan begitu trigger set_updated_at dan
// baris jurnal database hanya melihat kolom yang benar-benar berubah, dan tidak ada
// jalur yang bisa menimpa perubahan pengguna lain dengan nilai basi.
type updateSet struct {
	assigns []string
	args    []any
}

// newUpdateSet memulai perakitan dengan argumen pertama ($1) berisi kunci baris.
func newUpdateSet(key any) *updateSet {
	return &updateSet{args: []any{key}}
}

// add menambahkan "kolom = $n" beserta nilainya.
func (u *updateSet) add(column string, value any) {
	u.args = append(u.args, value)
	u.assigns = append(u.assigns, fmt.Sprintf("%s = $%d", column, len(u.args)))
}

// addOpt menambahkan kolom hanya bila pemanggil memintanya berubah. Fungsi bebas,
// bukan method, karena method di Go tidak boleh punya parameter tipe sendiri.
func addOpt[T any](u *updateSet, column string, o Opt[T]) {
	if o.Set {
		u.add(column, o.arg())
	}
}

// addOptJSONB menambahkan kolom jsonb opsional dengan cast eksplisit.
//
// []byte tanpa anotasi dikirim pgx dengan OID bytea, dan PostgreSQL menolaknya saat
// kolom tujuannya jsonb — cast di klausa SET adalah satu-satunya tempat memastikan biner
// mentah selalu dianggap dokumen JSON, apa pun OID yang dikirim driver. NULL tidak butuh
// cast: pgx mengirim NULL tanpa OID dan kolom jsonb menerimanya.
func addOptJSONB(u *updateSet, column string, o Opt[[]byte]) {
	if !o.Set {
		return
	}
	if o.Value == nil {
		u.add(column, nil)
		return
	}
	u.args = append(u.args, *o.Value)
	u.assigns = append(u.assigns, fmt.Sprintf("%s = $%d::jsonb", column, len(u.args)))
}

// empty melaporkan apakah tidak ada kolom yang diubah.
func (u *updateSet) empty() bool { return len(u.assigns) == 0 }

// clause mengembalikan isi klausa SET.
func (u *updateSet) clause() string { return strings.Join(u.assigns, ", ") }

// newID membuat UUID versi 4 acak.
//
// Pengenal dibuat di Go, bukan diserahkan ke gen_random_uuid() di database, karena
// ciphertext kredensial diikat ke pengenal barisnya lewat AAD: nilainya harus sudah
// diketahui sebelum enkripsi. Alternatifnya adalah menyisipkan baris dengan ciphertext
// sementara lalu memperbaruinya di transaksi yang sama, yang berarti ada saat ketika
// baris itu ada dengan isi yang salah — dan satu kegagalan di tengah jalan cukup untuk
// meninggalkannya begitu.
func newID() (string, error) {
	var b [16]byte
	if _, err := rand.Read(b[:]); err != nil {
		return "", fmt.Errorf("membuat pengenal acak: %w", err)
	}
	// Versi 4 dan varian RFC 4122, supaya nilainya sah sebagai uuid bagi PostgreSQL
	// dan tidak bisa tertukar dengan pengenal berpola lain.
	b[6] = (b[6] & 0x0f) | 0x40
	b[8] = (b[8] & 0x3f) | 0x80
	return fmt.Sprintf("%x-%x-%x-%x-%x", b[0:4], b[4:6], b[6:8], b[8:10], b[10:16]), nil
}

// idOK memeriksa bentuk UUID sebelum sebuah pengenal dikirim ke database.
//
// Pengenal yang salah bentuk (terpotong saat disalin dari dashboard, atau berasal dari
// URL yang dikarang) gagal di lapisan encoding pgx dan akan muncul sebagai kegagalan
// internal. Artinya sebenarnya sama dengan "tidak ada": tidak mungkin ada baris dengan
// pengenal seperti itu. Pemeriksaan ini yang membuat pemanggil mendapat ErrNotFound
// atau ErrInvalidReference, bukan error driver.
func idOK(id string) bool {
	if len(id) != 36 {
		return false
	}
	for i := range len(id) {
		c := id[i]
		switch i {
		case 8, 13, 18, 23:
			if c != '-' {
				return false
			}
		default:
			isHex := (c >= '0' && c <= '9') || (c >= 'a' && c <= 'f') || (c >= 'A' && c <= 'F')
			if !isHex {
				return false
			}
		}
	}
	return true
}

// maskCredential membentuk cuplikan yang aman ditampilkan di dashboard dari sebuah
// kredensial, mis. "sk-****9a21".
//
// Awalannya dipertahankan karena itulah yang membuat kredensial bisa dikenali manusia
// ("ini key OpenAI, bukan Anthropic"), sementara bagian tengahnya tidak pernah keluar.
// Kalau tidak ada awalan yang dikenali, seluruh nilai disamarkan.
func maskCredential(s security.Secret) string {
	raw := s.Reveal()
	prefix := ""
	// Awalan yang dipakai penyedia selalu pendek dan diakhiri pemisah, mis. "sk-",
	// "sk_live_", "AIza" tidak berpemisah. Hanya pemisah dalam delapan karakter pertama
	// yang dianggap awalan, supaya bagian rahasia tidak ikut terbuka.
	if i := strings.IndexAny(raw, "-_"); i >= 0 && i < 8 {
		prefix = raw[:i+1]
	}
	return security.Mask(raw, prefix, 4)
}

// maskProxyURL membentuk cuplikan URL proxy yang aman ditampilkan, mis.
// "http://user:****@host:3128".
//
// URL proxy hampir selalu memuat kredensial di userinfo, jadi hanya passwordnya yang
// ditutup: host dan port tetap terbaca supaya operator bisa mengenali jalur keluar
// mana yang dimaksud. URL yang tidak bisa diurai disamarkan seluruhnya — lebih baik
// cuplikan yang tidak berguna daripada bocor karena bentuk yang tak terduga.
func maskProxyURL(s security.Secret) string {
	u, err := url.Parse(strings.TrimSpace(s.Reveal()))
	if err != nil || u.Host == "" {
		return "****"
	}
	if u.User == nil {
		return u.Scheme + "://" + u.Host
	}
	if _, hasPassword := u.User.Password(); !hasPassword {
		return u.Scheme + "://" + u.User.Username() + "@" + u.Host
	}
	return u.Scheme + "://" + u.User.Username() + ":****@" + u.Host
}

// nullIfEmpty mengubah string kosong menjadi NULL. Kolom teks opsional di skema ini
// memang bermakna "tidak ada", bukan "ada tapi kosong".
func nullIfEmpty(s string) any {
	if strings.TrimSpace(s) == "" {
		return nil
	}
	return s
}

// intOr mengembalikan *v atau nilai bawaan bila v nil.
func intOr(v *int, def int) int {
	if v == nil {
		return def
	}
	return *v
}

// boolOr mengembalikan *v atau nilai bawaan bila v nil.
func boolOr(v *bool, def bool) bool {
	if v == nil {
		return def
	}
	return *v
}

// strOr mengembalikan v atau nilai bawaan bila v kosong.
func strOr(v, def string) string {
	if strings.TrimSpace(v) == "" {
		return def
	}
	return v
}

// refID memvalidasi pengenal referensi opsional sebelum dikirim ke database.
func refID(op, field string, id *string) error {
	if id != nil && !idOK(*id) {
		return fmt.Errorf("%s: %w: %s bukan UUID", op, repo.ErrInvalidReference, field)
	}
	return nil
}

// cursorSep memisahkan dua bagian kursor keyset.
const cursorSep = "|"

// encodeCursor merakit kursor keyset dari waktu dan pengenal baris terakhir.
//
// Waktu saja tidak cukup: now() adalah waktu transaksi, jadi beberapa baris yang
// dibuat dalam satu transaksi punya created_at yang identik. Kursor yang hanya berisi
// waktu akan melewati atau mengulang baris-baris itu, dan pengenal barislah yang
// memutus seri.
func encodeCursor(ts time.Time, id string) string {
	return ts.UTC().Format(time.RFC3339Nano) + cursorSep + id
}

// decodeCursor memecah kursor keyset. Pengenalnya dikembalikan sebagai teks karena
// tabel yang dilayani memakai uuid maupun bigint sebagai kunci.
func decodeCursor(op, cursor string) (time.Time, string, error) {
	ts, id, ok := strings.Cut(cursor, cursorSep)
	if !ok || id == "" {
		return time.Time{}, "", fmt.Errorf("%s: %w: kursor tidak berbentuk waktu|pengenal", op, repo.ErrConstraint)
	}
	parsed, err := time.Parse(time.RFC3339Nano, ts)
	if err != nil {
		return time.Time{}, "", fmt.Errorf("%s: %w: bagian waktu pada kursor tidak sah", op, repo.ErrConstraint)
	}
	return parsed, id, nil
}
