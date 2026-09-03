// Package ratelimit menjawab satu pertanyaan: batas laju mana yang berlaku bagi permintaan
// ini, dan berapa angkanya.
//
// Penegakannya TIDAK ada di sini. Mesin jendela di Redis — skrip Lua yang memeriksa dan
// menaikkan seluruh penghitung dalam satu langkah — hidup di internal/apikey, dibangun di
// Fase 4 bersama autentikasi API key. Memindahkannya ke sini berarti menulis ulang satu-
// satunya bagian pembatas laju yang benar-benar sulit, atau punya dua salinannya.
//
// Yang ada di sini adalah lapisan KEBIJAKAN: membaca tabel rate_limits, menyimpannya di
// memori dengan TTL pendek, lalu menyusun daftar cakupan yang siap diserahkan ke mesin itu.
// Pembagian ini juga yang membuat internal/apikey tidak perlu tahu apa pun tentang tabel
// rate_limits maupun cache-nya.
//
// # Setiap cakupan adalah ember sendiri
//
// Batas pada cakupan berbeda TIDAK digabungkan. Batas per API key dan batas per pengguna
// adalah dua penghitung yang ditegakkan bersamaan, dan itulah maksudnya: satu pengguna bisa
// punya sepuluh key yang masing-masing 60 rpm sementara totalnya tetap dibatasi 100 rpm.
// Yang digabungkan hanyalah beberapa baris untuk cakupan DAN entitas yang sama, karena
// skemanya tidak melarang dua baris seperti itu ada.
//
// # Gagal-terbuka, dan itu keputusan sadar
//
// Kalau tabelnya tidak bisa dibaca, salinan terakhir dipertahankan; kalau belum pernah
// terbaca sama sekali, tidak ada batas yang ditegakkan. Arahnya sama dengan pembatas laju
// itu sendiri saat Redis mati, dan alasannya sama: satu tabel konfigurasi yang tidak
// terbaca tidak boleh mematikan seluruh lalu lintas inference. Yang membuatnya bisa
// dipertanggungjawabkan adalah metrik gagal-terbuka di internal/apikey — tanpa pemantauan
// atas angka itu, gateway bisa berjalan berbulan-bulan tanpa batas laju tanpa ada yang tahu.
package ratelimit

import (
	"context"
	"log/slog"
	"sync"
	"time"

	"github.com/NexGen-X/Route-X/internal/apikey"
	"github.com/NexGen-X/Route-X/internal/database/repo/policy"
)

// defaultTTL adalah umur salinan batas laju di memori.
//
// Lima detik, angka yang sama dengan cache aturan routing dan dengan alasan yang sama:
// perubahan di satu instance tidak bisa membatalkan cache instance lain tanpa pub/sub, dan
// TTL memberi batas keterlambatan yang bisa dijelaskan dalam satu kalimat kepada operator
// yang baru mengubah setelan lalu bertanya kapan berlakunya.
const defaultTTL = 5 * time.Second

// Source memasok batas laju aktif. Dipenuhi *policy.Repo.
type Source interface {
	ActiveRateLimits(ctx context.Context) ([]*policy.RateLimit, error)
}

// scopeRedis memetakan nilai kolom scope di database ke nama cakupan pada kunci Redis.
//
// Dua kosakata yang sengaja dipisah: yang di kiri terikat constraint tabel dan hanya bisa
// diubah lewat migrasi, yang di kanan terikat kunci Redis dan mengubahnya berarti kehilangan
// seluruh penghitung yang sedang berjalan. Peta ini satu-satunya tempat keduanya bertemu.
var scopeRedis = map[string]string{
	policy.ScopeGlobal:   apikey.ScopeGlobal,
	policy.ScopeAPIKey:   apikey.ScopeAPIKey,
	policy.ScopeUser:     apikey.ScopeUser,
	policy.ScopeProvider: apikey.ScopeProvider,
	policy.ScopeModel:    apikey.ScopeModel,
	policy.ScopeIP:       apikey.ScopeIP,
}

// Engine menyimpan salinan batas laju dan menyusun cakupan penegakan dari sana.
//
// Aman dipakai bersamaan oleh banyak goroutine.
type Engine struct {
	source Source
	logger *slog.Logger
	ttl    time.Duration
	now    func() time.Time

	mu     sync.RWMutex
	dimuat time.Time
	// perCakupan[scope][scopeID] adalah batas gabungan untuk satu entitas.
	perCakupan map[string]map[string]batas
	// global adalah batas cakupan global, kosong bila tidak ada.
	global batas
	// pernahTerbaca menandai apakah salinan yang ada berasal dari pembacaan yang berhasil.
	pernahTerbaca bool
}

// batas adalah batas gabungan satu entitas, dalam bentuk yang dipakai mesin penegakan.
type batas struct {
	requests apikey.Limits
	tokens   apikey.TokenLimits
}

func (b batas) kosong() bool { return b.requests.IsZero() && b.tokens.IsZero() }

// EngineOption menyetel Engine saat konstruksi.
type EngineOption func(*Engine)

// WithTTL mengubah umur salinan di memori. Nilai <= 0 diabaikan.
func WithTTL(d time.Duration) EngineOption {
	return func(e *Engine) {
		if d > 0 {
			e.ttl = d
		}
	}
}

// WithClock mengganti sumber waktu, untuk test yang memeriksa kedaluwarsa salinan tanpa
// menunggu waktu nyata berjalan.
func WithClock(now func() time.Time) EngineOption {
	return func(e *Engine) {
		if now != nil {
			e.now = now
		}
	}
}

// NewEngine membuat mesin kebijakan batas laju.
//
// source nil sah dan berarti tabel rate_limits tidak dipakai sama sekali: yang berlaku
// hanya batas yang tertulis di baris api_keys. Itu keadaan yang benar untuk pemasangan yang
// belum menyentuh tabel ini.
func NewEngine(source Source, logger *slog.Logger, opts ...EngineOption) *Engine {
	if logger == nil {
		logger = slog.New(slog.DiscardHandler)
	}
	e := &Engine{source: source, logger: logger, ttl: defaultTTL, now: time.Now}
	for _, o := range opts {
		if o != nil {
			o(e)
		}
	}
	return e
}

// Invalidate memaksa pembacaan ulang pada pemakaian berikutnya.
//
// Untuk API admin Fase 11: operator yang baru mengubah batas ingin melihat akibatnya
// sekarang, bukan setelah TTL habis. Hanya berlaku pada instance ini — lintas instance
// tetap menunggu TTL, dan tempat memperbaikinya adalah pub/sub Redis.
func (e *Engine) Invalidate() {
	e.mu.Lock()
	e.dimuat = time.Time{}
	e.mu.Unlock()
}

// Requests menyusun cakupan batas request untuk target-target ini.
//
// keyLimits adalah batas yang tertulis di baris api_keys, digabungkan PER FIELD dengan
// baris rate_limits bercakupan api_key untuk key yang sama — kolom di api_keys menang.
// Alasannya: itu angka yang ditampilkan dashboard pada key tersebut, jadi operator yang
// mengisinya berharap itulah yang berlaku, sementara baris rate_limits mengisi yang
// dibiarkan kosong.
//
// Urutan hasilnya mengikuti urutan targets, karena urutan itu menentukan batas mana yang
// dilaporkan lewat header X-RateLimit-* ketika lebih dari satu membatasi.
func (e *Engine) Requests(ctx context.Context, targets []policy.Target, keyLimits apikey.Limits) []apikey.ScopeLimits {
	e.muat(ctx)

	e.mu.RLock()
	defer e.mu.RUnlock()

	out := make([]apikey.ScopeLimits, 0, len(targets))
	for _, t := range targets {
		redisScope, ok := scopeRedis[t.Scope]
		if !ok {
			continue
		}
		b := e.cariTerkunci(t)
		limits := b.requests
		if t.Scope == policy.ScopeAPIKey {
			limits = keyLimits.WithDefaults(limits)
		}
		if limits.IsZero() {
			continue
		}
		out = append(out, apikey.ScopeLimits{Scope: redisScope, ID: idUntuk(t), Limits: limits})
	}
	return out
}

// Tokens menyusun cakupan batas token untuk target-target ini.
//
// keyLimits berperan sama seperti di Requests: kolom di api_keys menang per field.
func (e *Engine) Tokens(ctx context.Context, targets []policy.Target, keyLimits apikey.TokenLimits) []apikey.ScopeTokenLimits {
	e.muat(ctx)

	e.mu.RLock()
	defer e.mu.RUnlock()

	out := make([]apikey.ScopeTokenLimits, 0, len(targets))
	for _, t := range targets {
		redisScope, ok := scopeRedis[t.Scope]
		if !ok {
			continue
		}
		b := e.cariTerkunci(t)
		limits := b.tokens
		if t.Scope == policy.ScopeAPIKey {
			limits = keyLimits.WithDefaults(limits)
		}
		if limits.IsZero() {
			continue
		}
		out = append(out, apikey.ScopeTokenLimits{Scope: redisScope, ID: idUntuk(t), Limits: limits})
	}
	return out
}

// cariTerkunci mengambil batas satu target. Pemanggil WAJIB memegang mu (baca).
func (e *Engine) cariTerkunci(t policy.Target) batas {
	if t.Scope == policy.ScopeGlobal {
		return e.global
	}
	if per, ok := e.perCakupan[t.Scope]; ok {
		return per[t.ID]
	}
	return batas{}
}

// idUntuk mengembalikan bagian pengenal kunci Redis untuk satu target.
func idUntuk(t policy.Target) string {
	if t.Scope == policy.ScopeGlobal {
		return apikey.GlobalID
	}
	return t.ID
}

// muat memuat ulang salinan bila sudah kedaluwarsa.
//
// Kegagalan pembacaan MEMPERTAHANKAN salinan lama dan hanya dicatat. Lihat catatan paket.
func (e *Engine) muat(ctx context.Context) {
	if e.source == nil {
		return
	}

	e.mu.RLock()
	segar := !e.dimuat.IsZero() && e.now().Sub(e.dimuat) < e.ttl
	e.mu.RUnlock()
	if segar {
		return
	}

	rows, err := e.source.ActiveRateLimits(ctx)
	if err != nil {
		e.logger.WarnContext(ctx, "batas laju gagal dibaca, memakai salinan sebelumnya",
			"error", err, "pernah_terbaca", e.pernahTerbacaAman())
		// Waktu muat TIDAK diperbarui: pembacaan berikutnya mencoba lagi alih-alih menunggu
		// satu TTL penuh. Tabel yang sedang tidak bisa dibaca biasanya pulih dalam hitungan
		// detik, dan menunda percobaan berikutnya hanya memperpanjang masa tanpa penegakan.
		return
	}

	perCakupan := make(map[string]map[string]batas, len(rows))
	var global batas
	for _, row := range rows {
		b := dariBaris(row)
		if b.kosong() {
			continue
		}
		if row.Scope == policy.ScopeGlobal {
			global = gabung(global, b)
			continue
		}
		per, ok := perCakupan[row.Scope]
		if !ok {
			per = make(map[string]batas)
			perCakupan[row.Scope] = per
		}
		// Digabungkan, bukan ditimpa: skema tidak melarang dua baris untuk cakupan dan
		// entitas yang sama, dan ActiveRateLimits mengurutkannya secara tetap. Yang lebih
		// dulu menang per field, sehingga hasilnya sama di setiap instance.
		per[row.ScopeID] = gabung(per[row.ScopeID], b)
	}

	e.mu.Lock()
	e.perCakupan, e.global = perCakupan, global
	e.dimuat, e.pernahTerbaca = e.now(), true
	e.mu.Unlock()
}

// pernahTerbacaAman membaca penanda pembacaan dengan lock, untuk dipakai di pesan log.
func (e *Engine) pernahTerbacaAman() bool {
	e.mu.RLock()
	defer e.mu.RUnlock()
	return e.pernahTerbaca
}

// dariBaris menerjemahkan satu baris rate_limits menjadi batas penegakan.
//
// NULL menjadi nol, dan nol berarti "tidak ditegakkan pada cakupan ini". Perbedaan antara
// NULL dan nol memang hilang di sini, dan itu sengaja: mesin jendela memakai nol sebagai
// "tidak ada batas", sedangkan constraint rate_limits_at_least_one sudah memastikan tidak
// ada baris yang seluruh kolomnya kosong. Batas bernilai nol yang berarti "tolak semuanya"
// tidak bisa dinyatakan lewat tabel ini, dan itu adalah penolakan total yang seharusnya
// dilakukan lewat blokir, bukan lewat batas laju.
func dariBaris(row *policy.RateLimit) batas {
	return batas{
		requests: apikey.Limits{
			RPS:     deref(row.RequestsPerSecond),
			RPM:     deref(row.RequestsPerMinute),
			Daily:   deref(row.DailyRequestLimit),
			Monthly: deref(row.MonthlyRequestLimit),
		},
		tokens: apikey.TokenLimits{
			TPM:     deref(row.TokensPerMinute),
			Daily:   deref(row.DailyTokenLimit),
			Monthly: deref(row.MonthlyTokenLimit),
		},
	}
}

// gabung mengisi field yang kosong pada a dari b. a menang.
func gabung(a, b batas) batas {
	return batas{
		requests: a.requests.WithDefaults(b.requests),
		tokens:   a.tokens.WithDefaults(b.tokens),
	}
}

// deref mengembalikan nilai yang ditunjuk, atau nol bila nil.
func deref[T int | int64](v *T) T {
	if v == nil {
		return 0
	}
	return *v
}
