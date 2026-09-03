// Package policy memuat akses data untuk kebijakan yang MEMBATASI lalu lintas: batas laju,
// anggaran biaya, blokir, dan penyaring konten.
//
// Keempatnya dipisahkan dari repository lain karena punya satu sifat yang sama dan tidak
// dimiliki yang lain: isinya dibaca pada JALUR REQUEST, oleh setiap permintaan, dan
// jumlah barisnya kecil serta hanya berubah ketika operator mengubahnya.
//
// Sifat itu yang menentukan bentuk API di sini. Pembacaan jalur request mengambil SELURUH
// baris aktif sekaligus (ActiveRateLimits, ActiveBudgets, ActiveFilters), bukan satu baris
// per pemeriksaan. Pemanggil menyimpannya di memori dengan TTL pendek dan mencocokkannya
// sendiri. Alasannya bukan penghematan mikro: memeriksa enam cakupan pembatasan per
// permintaan lewat query terpisah berarti enam perjalanan ke database sebelum satu byte
// dikirim ke provider, pada tabel yang isinya sama untuk ribuan permintaan berurutan.
//
// Pembatalan cache memakai TTL, bukan invalidasi-saat-tulis, dengan alasan yang sama
// seperti cache aturan routing: perubahan di satu instance tidak bisa membatalkan cache
// instance lain tanpa pub/sub, dan TTL memberi batas keterlambatan yang bisa dijelaskan
// dalam satu kalimat.
package policy

import (
	"fmt"
	"strings"
	"time"

	"github.com/NexGen-X/Route-X/internal/database/repo"
)

// Cakupan kebijakan. Nilainya WAJIB sama dengan constraint rate_limits_scope_valid dan
// budgets_scope_valid di migrasi 0005.
//
// Perhatian: ini BUKAN kosakata yang sama dengan cakupan kunci Redis di internal/apikey
// (`apikey`, `ip`). Yang di sini adalah nilai kolom database; yang di sana adalah bagian
// kunci Redis. Menyamakan keduanya akan menyenangkan sesaat lalu mengunci keduanya pada
// satu ejaan yang tidak bisa diubah tanpa migrasi maupun kehilangan seluruh penghitung.
const (
	ScopeGlobal   = "global"
	ScopeAPIKey   = "api_key"
	ScopeUser     = "user"
	ScopeProvider = "provider"
	ScopeModel    = "model"
	ScopeIP       = "ip"
)

// Periode anggaran. Sama dengan constraint budgets_period_valid.
const (
	PeriodDaily   = "daily"
	PeriodWeekly  = "weekly"
	PeriodMonthly = "monthly"
	PeriodTotal   = "total"
)

// Tindakan saat anggaran atau penyaring terpicu. Sama dengan constraint
// budgets_action_valid dan content_filters_action_valid.
const (
	// ActionBlock menolak permintaan.
	ActionBlock = "block"
	// ActionWarn hanya mencatat dan memicu notifikasi; permintaan tetap dilayani.
	ActionWarn = "warn"
)

// Jenis subjek blokir. Sama dengan constraint bans_subject_kind_valid.
//
// Penegakannya TIDAK ada di paket ini: pemeriksaan blokir dijalankan sebagai bagian dari
// query autentikasi API key di internal/database/repo/keys, dalam satu perjalanan yang
// sama dengan pengambilan barisnya. Memindahkannya ke sini berarti satu query tambahan
// pada setiap permintaan untuk tabel yang hampir selalu kosong. Yang ada di sini adalah
// pengelolaannya: membuat, mencabut, mendaftar, dan membersihkan yang kedaluwarsa.
const (
	BanSubjectIP      = "ip"
	BanSubjectIPRange = "ip_range"
	BanSubjectAPIKey  = "api_key"
	BanSubjectUser    = "user"
)

// Jenis penyaring konten. Sama dengan constraint content_filters_kind_valid.
const (
	FilterBlockedPattern      = "blocked_pattern"
	FilterAllowedPattern      = "allowed_pattern"
	FilterModelRestriction    = "model_restriction"
	FilterProviderRestriction = "provider_restriction"
	FilterRequestSize         = "request_size"
	FilterModeration          = "moderation"
)

// Bentuk pola penyaring. Sama dengan constraint content_filters_pattern_type_valid.
const (
	PatternSubstring = "substring"
	PatternRegex     = "regex"
)

// Bagian permintaan yang diperiksa penyaring. Sama dengan constraint
// content_filters_applies_to_valid.
const (
	AppliesToRequest  = "request"
	AppliesToResponse = "response"
	AppliesToBoth     = "both"
)

// Repo adalah repository kebijakan pembatasan lalu lintas.
type Repo struct{ q repo.Querier }

// New membuat repository di atas Querier apa pun — pool maupun transaksi.
func New(q repo.Querier) *Repo { return &Repo{q: q} }

// idOK memeriksa bentuk UUID sebelum menyentuh database.
//
// Pengenal yang salah bentuk gagal di lapisan encoding pgx dan muncul sebagai kegagalan
// internal, padahal artinya sama dengan "tidak ada". Pemeriksaan ini yang membuat
// pemanggil menerima ErrNotFound, bukan error driver.
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

// nullIfEmpty mengubah string kosong menjadi NULL. Kolom scope_id memang bermakna
// "tidak ada" pada cakupan global, bukan "ada tapi kosong" — dan constraint
// rate_limits_scope_id_presence menolak yang kedua.
func nullIfEmpty(s string) any {
	if s == "" {
		return nil
	}
	return s
}

// Target adalah satu entitas yang dikenai kebijakan, mis. ("api_key", "<uuid>").
//
// Satu permintaan selalu punya BEBERAPA target sekaligus — global, API key-nya,
// penggunanya, modelnya, providernya, alamatnya — dan kebijakan yang berlaku adalah
// gabungan dari semuanya. Karena itu API di paket ini menerima daftar target, bukan satu:
// memeriksanya satu per satu berarti satu perjalanan ke database per cakupan, dan
// penolakan yang seharusnya atomik menjadi berurutan.
type Target struct {
	// Scope adalah salah satu konstanta Scope di atas.
	Scope string
	// ID kosong hanya sah untuk ScopeGlobal.
	ID string
}

// scopeArrays memecah daftar target menjadi dua array sejajar untuk dipakai sebagai
// parameter query.
//
// Dua array yang di-unnest berpasangan, bukan satu string gabungan seperti
// "api_key:<uuid>": nilai gabungan menuntut pemisah yang tidak boleh muncul di dalam
// nilainya, dan scope_id untuk cakupan ip adalah alamat yang bisa memuat ":" sebanyak
// yang ia mau.
func scopeArrays(targets []Target) (scopes, ids []string) {
	scopes = make([]string, 0, len(targets))
	ids = make([]string, 0, len(targets))
	for _, t := range targets {
		if t.Scope == "" || t.Scope == ScopeGlobal || t.ID == "" {
			continue
		}
		scopes = append(scopes, t.Scope)
		ids = append(ids, t.ID)
	}
	return scopes, ids
}

const cursorSep = "|"

// encodeCursor merakit kursor keyset dari waktu dan pengenal baris.
func encodeCursor(ts time.Time, id string) string {
	return ts.UTC().Format(time.RFC3339Nano) + cursorSep + id
}

// decodeCursor memecah kursor keyset menjadi waktu dan ID.
func decodeCursor(op, cursor string) (time.Time, string, error) {
	ts, id, ok := strings.Cut(cursor, cursorSep)
	if !ok || id == "" {
		return time.Time{}, "", fmt.Errorf("%s: %w: kursor tidak berbentuk waktu|id", op, repo.ErrConstraint)
	}
	parsed, err := time.Parse(time.RFC3339Nano, ts)
	if err != nil {
		return time.Time{}, "", fmt.Errorf("%s: %w: format waktu kursor tidak sah", op, repo.ErrConstraint)
	}
	return parsed, id, nil
}
