package apikey

import (
	"context"
	"errors"
	"log/slog"
	"net/http"
	"net/netip"
	"strconv"
	"time"

	"github.com/prometheus/client_golang/prometheus"

	"github.com/NexGen-X/Route-X/internal/cache"
	"github.com/NexGen-X/Route-X/internal/database/repo/keys"
	"github.com/NexGen-X/Route-X/internal/httpx"
	"github.com/NexGen-X/Route-X/internal/observability"
)

// --- Kontrak header ----------------------------------------------------------

// Header sisa kuota yang dipasang pada setiap respons yang melewati Limit(), baik yang
// lolos maupun yang ditolak.
//
// Nilai Reset berupa detik epoch Unix, bukan lama tunggu. Itu konvensi yang dipakai
// hampir semua API yang memakai nama header X-RateLimit-* (GitHub, Twitter, draft RFC
// RateLimit), sehingga pustaka klien yang sudah ada bisa mengurainya tanpa perlakuan
// khusus. Klien yang hanya ingin tahu "berapa lama lagi" tetap terlayani lewat
// Retry-After, yang dipasang httpx.TooManyRequests pada respons 429.
const (
	HeaderRateLimitLimit     = "X-RateLimit-Limit"
	HeaderRateLimitRemaining = "X-RateLimit-Remaining"
	HeaderRateLimitReset     = "X-RateLimit-Reset"
)

// Cakupan pembatasan. Nilainya masuk ke kunci Redis dan ke label metrik, jadi tetap.
const (
	// ScopeAPIKey membatasi per baris api_keys.
	ScopeAPIKey = "apikey"
	// ScopeIP membatasi per alamat klien hasil resolusi httpx.RealIP.
	ScopeIP = "ip"
	// ScopeGlobal membatasi seluruh lalu lintas gateway ini. Tidak punya pengenal, jadi
	// pemanggil memakai satu nilai tetap sebagai ID-nya (lihat GlobalID).
	ScopeGlobal = "global"
	// ScopeUser membatasi per pemilik API key.
	ScopeUser = "user"
	// ScopeProvider membatasi lalu lintas KELUAR ke satu provider, supaya kuota pihak
	// ketiga tidak terlanggar dari sisi kami.
	ScopeProvider = "provider"
	// ScopeModel membatasi per model kanonik.
	ScopeModel = "model"
)

// GlobalID adalah pengenal yang dipakai untuk cakupan global.
//
// Cakupan global tidak punya entitas, tetapi kunci Redis butuh bagian pengenal. Nilai
// tetap ini yang mengisinya, dan ia sengaja bukan string kosong: kunci dengan bagian
// kosong menghasilkan dua pemisah berurutan yang menyulitkan pembacaan saat seseorang
// menelusuri kunci di Redis.
const GlobalID = "all"

// Jenis batas. Ikut ke dalam kunci Redis sebagai awalan label jendela, sehingga dua
// jenis batas tidak mungkin memakai kunci yang sama walau nomor jendelanya kebetulan
// bertabrakan — nomor detik epoch dan tanggal bergaya 20260902 hidup di ruang angka yang
// sama, dan tanpa awalan ini keduanya bisa bertemu.
const (
	LimitRPS     = "rps"
	LimitRPM     = "rpm"
	LimitDaily   = "daily"
	LimitMonthly = "monthly"

	LimitTPM           = "tpm"
	LimitDailyTokens   = "daily_tokens"
	LimitMonthlyTokens = "monthly_tokens"
)

// --- Batas -------------------------------------------------------------------

// Limits adalah batas berbasis jumlah request untuk satu cakupan. Nilai <= 0 berarti
// batas itu tidak ditegakkan.
type Limits struct {
	RPS     int
	RPM     int
	Daily   int64
	Monthly int64
}

// IsZero melaporkan apakah tidak ada satu pun batas yang aktif.
func (l Limits) IsZero() bool {
	return l.RPS <= 0 && l.RPM <= 0 && l.Daily <= 0 && l.Monthly <= 0
}

// WithDefaults mengisi batas yang kosong dari cakupan yang lebih luas.
//
// Penggabungan dilakukan per field, bukan "pakai default hanya bila key tidak punya
// batas apa pun". Itu mengikuti maksud skema: kolom batas di api_keys yang NULL berarti
// "warisi dari cakupan yang lebih luas", jadi key yang hanya menyetel RPS tetap tunduk
// pada batas harian bawaan.
func (l Limits) WithDefaults(def Limits) Limits {
	if l.RPS <= 0 {
		l.RPS = def.RPS
	}
	if l.RPM <= 0 {
		l.RPM = def.RPM
	}
	if l.Daily <= 0 {
		l.Daily = def.Daily
	}
	if l.Monthly <= 0 {
		l.Monthly = def.Monthly
	}
	return l
}

// LimitsFromKey membaca batas request dari baris api_keys. Kolom NULL menjadi nol, yang
// berarti "tidak ditegakkan di cakupan ini".
func LimitsFromKey(k *keys.Key) Limits {
	if k == nil {
		return Limits{}
	}
	return Limits{
		RPS:     derefInt(k.RateLimitRPS),
		RPM:     derefInt(k.RateLimitRPM),
		Daily:   derefInt64(k.DailyRequestLimit),
		Monthly: derefInt64(k.MonthlyRequestLimit),
	}
}

// TokenLimits adalah batas berbasis jumlah token.
//
// Dipisah dari Limits karena penegakannya terjadi di titik yang berbeda dalam siklus
// request — lihat Limiter.RecordTokens.
type TokenLimits struct {
	TPM     int
	Daily   int64
	Monthly int64
}

// IsZero melaporkan apakah tidak ada satu pun batas token yang aktif.
func (l TokenLimits) IsZero() bool {
	return l.TPM <= 0 && l.Daily <= 0 && l.Monthly <= 0
}

// WithDefaults mengisi batas token yang kosong dari cakupan yang lebih luas, per field,
// dengan alasan yang sama seperti Limits.WithDefaults.
func (l TokenLimits) WithDefaults(def TokenLimits) TokenLimits {
	if l.TPM <= 0 {
		l.TPM = def.TPM
	}
	if l.Daily <= 0 {
		l.Daily = def.Daily
	}
	if l.Monthly <= 0 {
		l.Monthly = def.Monthly
	}
	return l
}

// TokenLimitsFromKey membaca batas token dari baris api_keys.
func TokenLimitsFromKey(k *keys.Key) TokenLimits {
	if k == nil {
		return TokenLimits{}
	}
	return TokenLimits{
		TPM:     derefInt(k.RateLimitTPM),
		Daily:   derefInt64(k.DailyTokenLimit),
		Monthly: derefInt64(k.MonthlyTokenLimit),
	}
}

func derefInt(v *int) int {
	if v == nil {
		return 0
	}
	return *v
}

func derefInt64(v *int64) int64 {
	if v == nil {
		return 0
	}
	return *v
}

// --- Hasil pemeriksaan -------------------------------------------------------

// Decision adalah hasil satu pemeriksaan batas.
//
// Field pelaporan (Scope, Limit, LimitValue, Remaining, ResetAt) menggambarkan SATU
// batas, bukan semuanya: batas yang menolak bila ditolak, atau batas yang paling dekat
// habis bila lolos. Header X-RateLimit-* hanya punya tempat untuk satu angka, jadi yang
// dilaporkan adalah batas yang paling menentukan bagi klien — itulah yang perlu ia
// perlambat.
type Decision struct {
	// Allowed false berarti batas terlampaui.
	//
	// Untuk RecordTokens artinya sedikit berbeda: request-nya sudah terjadi dan tidak
	// bisa ditolak lagi, jadi false di situ berarti "anggaran token sudah habis setelah
	// pemakaian ini dicatat", yaitu isyarat untuk request BERIKUTNYA.
	Allowed bool

	// Degraded true berarti keputusan ini diambil tanpa Redis: batas tidak benar-benar
	// diperiksa dan request diloloskan. Lihat penjelasan gagal-terbuka di Limiter.
	Degraded bool

	// Scope dan Limit menamai batas yang dilaporkan, mis. ("apikey", "rpm"). Kosong
	// bila tidak ada batas yang aktif.
	Scope string
	Limit string

	// LimitValue adalah nilai batas itu. Nol berarti tidak ada batas yang aktif,
	// sehingga tidak ada header kuota yang perlu dipasang.
	LimitValue int64
	// Remaining adalah sisa kuota pada jendela ini setelah request ini diperhitungkan.
	Remaining int64
	// ResetAt adalah akhir jendela: saat kuota pulih penuh.
	ResetAt time.Time
	// RetryAfter adalah sisa umur jendela pada saat penolakan. Nol bila lolos.
	RetryAfter time.Duration
}

// SetHeaders memasang X-RateLimit-Limit, X-RateLimit-Remaining, dan X-RateLimit-Reset.
//
// Tidak melakukan apa pun bila tidak ada batas yang aktif: header kuota bernilai nol
// akan dibaca klien sebagai "kuota habis" dan membuatnya menahan diri tanpa alasan.
//
// Harus dipanggil sebelum status respons ditulis, seperti semua header lain.
func (d Decision) SetHeaders(w http.ResponseWriter) {
	if w == nil || d.LimitValue <= 0 {
		return
	}
	remaining := d.Remaining
	if remaining < 0 {
		remaining = 0
	}
	h := w.Header()
	h.Set(HeaderRateLimitLimit, strconv.FormatInt(d.LimitValue, 10))
	h.Set(HeaderRateLimitRemaining, strconv.FormatInt(remaining, 10))
	if !d.ResetAt.IsZero() {
		h.Set(HeaderRateLimitReset, strconv.FormatInt(d.ResetAt.Unix(), 10))
	}
}

// --- Skrip Lua ---------------------------------------------------------------

// Mode operasi skrip.
const (
	// modeEnforce: tolak tanpa menaikkan apa pun bila salah satu batas akan terlampaui.
	modeEnforce = 0
	// modeRecord: selalu naikkan, lalu laporkan batas mana yang sudah terlampaui.
	modeRecord = 1
	// modePeek: tidak menulis apa pun, hanya melaporkan keadaan sekarang.
	modePeek = 2
)

// limiterScript memeriksa dan menaikkan seluruh penghitung dalam satu langkah.
//
// # Kenapa harus satu skrip
//
// Dua alasan, dan keduanya tidak bisa ditutup dengan perintah terpisah.
//
// Pertama, TTL. INCR lalu EXPIRE sebagai dua perintah meninggalkan penghitung tanpa masa
// berlaku bila proses mati, jaringan terputus, atau context habis persis di antaranya.
// Penghitung tanpa TTL tidak pernah pulih: kunci itu akan terus tumbuh dan batasnya macet
// selamanya sampai ada yang menghapusnya dengan tangan. Di dalam skrip, Redis menjalankan
// keduanya tanpa bisa disela.
//
// Kedua, keputusan tunggal atas beberapa batas. Seluruh jendela — RPS, RPM, harian,
// bulanan, untuk cakupan key maupun IP — diperiksa lebih dulu, dan penghitung baru
// dinaikkan kalau SEMUANYA lolos. Kalau tiap batas diperiksa sendiri-sendiri, request yang
// ditolak batas per detik tetap sudah memakan satu jatah kuota harian, sehingga klien yang
// mengirim terlalu cepat akan kehabisan kuota harinya tanpa satu pun request berhasil.
// Yang mahal bukan hanya salahnya, tetapi juga betapa sulitnya menjelaskannya ke pelanggan.
//
// Pemeriksaan memakai "current + cost > limit", jadi batas 10 meloloskan tepat 10 request
// per jendela. Pada mode pencatatan token perbandingannya "count >= limit": di sana yang
// ditanyakan adalah apakah anggaran sudah habis, dan anggaran yang terpakai persis habis
// memang tidak menyisakan apa-apa untuk request berikutnya.
//
// PEXPIRE juga dipasang ulang bila PTTL melaporkan kunci tanpa masa berlaku. Itu tidak
// bisa terjadi lewat jalur ini, tetapi kunci bisa lahir dari tangan lain (operator yang
// memeriksa sesuatu dengan SET, migrasi versi skrip); memasangnya ulang membuat penghitung
// mustahil tertinggal tanpa TTL.
//
// Susunan argumen:
//
//	KEYS[i]      kunci penghitung ke-i
//	ARGV[1]      mode: 0 tegakkan, 1 catat, 2 lihat
//	ARGV[2]      cost: jumlah yang ditambahkan
//	ARGV[2i+1]   batas untuk KEYS[i]
//	ARGV[2i+2]   TTL milidetik untuk KEYS[i]
//
// Kembalian: {indeks_terlampaui, hitungan_1, ..., hitungan_n}, dengan indeks 0 berarti
// tidak ada yang terlampaui. Hitungan yang dilaporkan adalah nilai SETELAH keputusan:
// sudah naik bila diloloskan, tidak berubah bila ditolak.
var limiterScript = cache.NewScript(`
local mode = tonumber(ARGV[1])
local cost = tonumber(ARGV[2])
local n = #KEYS
local counts = {}
local exceeded = 0

-- Fase 1: baca seluruh penghitung sebelum satu pun diputuskan. Kunci yang tidak ada dan
-- kunci yang isinya bukan angka sama-sama dibaca sebagai nol; tanpa "or 0", nilai yang
-- tidak bisa diurai akan menggagalkan seluruh skrip pada operasi aritmetika di bawah.
for i = 1, n do
	counts[i] = tonumber(redis.call('GET', KEYS[i])) or 0
end

-- Fase 2: pada mode tegakkan, cari batas pertama yang akan terlampaui.
if mode == 0 then
	for i = 1, n do
		if counts[i] + cost > tonumber(ARGV[i * 2 + 1]) then
			exceeded = i
			break
		end
	end
end

-- Fase 3: naikkan hanya kalau semuanya lolos, dan selalu pastikan ada TTL.
if mode ~= 2 and exceeded == 0 then
	for i = 1, n do
		counts[i] = redis.call('INCRBY', KEYS[i], cost)
		if counts[i] == cost or redis.call('PTTL', KEYS[i]) < 0 then
			redis.call('PEXPIRE', KEYS[i], tonumber(ARGV[i * 2 + 2]))
		end
	end
end

-- Fase 4: pada mode catat dan lihat, laporkan anggaran yang sudah habis.
if mode ~= 0 then
	for i = 1, n do
		if counts[i] >= tonumber(ARGV[i * 2 + 1]) then
			exceeded = i
			break
		end
	end
end

local out = { exceeded }
for i = 1, n do
	out[i + 1] = counts[i]
end
return out
`)

// --- Limiter -----------------------------------------------------------------

// Limiter adalah pembatas laju terdistribusi untuk jalur /v1/*.
//
// # Bentuk jendela
//
// Jendela tetap dengan nomor jendela ikut di dalam kunci Redis. RPS memakai detik epoch,
// RPM memakai menit epoch, harian dan bulanan memakai tanggal UTC (20260902 dan 202609).
// Konsekuensinya diketahui: request yang menumpuk persis di perbatasan dua jendela bisa
// mencapai dua kali batas dalam rentang singkat. Itu diterima, dan bukan karena jendela
// geser sulit dibuat — ia menuntut satu sorted set per pemilik kuota berisi satu entri per
// request, yang berarti memori dan biaya per request tumbuh mengikuti batasnya, plus
// pemangkasan berkala. Untuk gateway yang tugasnya melindungi upstream dari pemakaian
// berlebih, dua kali batas selama satu detik di perbatasan tidak mengubah apa pun,
// sementara jendela tetap hanya butuh satu penghitung yang hilang sendiri lewat TTL tanpa
// pekerjaan latar apa pun.
//
// Harian dan bulanan memakai UTC, bukan zona waktu server. Kuota yang batasnya berpindah
// mengikuti zona waktu instance akan berbeda antar instance dan bergeser dua kali setahun
// di zona yang mengenal DST; pelanggan juga perlu bisa menghitung sendiri kapan kuotanya
// pulih.
//
// # Gagal terbuka
//
// Redis yang mati atau lambat membuat request DILOLOSKAN, bukan ditolak, dengan log
// peringatan dan metrik. Ini keputusan sadar, bukan kelalaian: gateway yang menolak semua
// lalu lintas karena cache-nya mati mengubah gangguan pada sistem pendukung menjadi
// pemadaman total layanan, sementara pembatas laju hanyalah pelindung dari pemakaian
// berlebih — bukan kendali keamanan. Yang benar-benar melindungi, yaitu autentikasi dan
// pemeriksaan cakupan, ada di database dan tetap berjalan. Selama Redis mati, penegakan
// batas hilang dan itu terlihat di metrik routex_rate_limit_failopen_total; sebaliknya,
// kalau ia gagal tertutup, seluruh pelanggan berhenti bekerja dan tidak ada metrik yang
// bisa memperbaiki itu.
//
// # Batas token tidak ditegakkan di sini
//
// TPM dan batas token harian/bulanan tidak bisa diperiksa di middleware: jumlah token
// baru diketahui setelah respons upstream selesai (dan untuk streaming, setelah event
// terakhir). Karena itu paket ini memisahkan keduanya. Limit() menegakkan batas berbasis
// jumlah request; jalur gateway memanggil RecordTokens setelah pemakaian diketahui, dan
// TokensExceeded sebagai gerbang sebelum request berikutnya diteruskan. Akibatnya batas
// token selalu terlampaui sedikit — satu request terakhir bisa melewatinya karena
// besarnya belum diketahui saat diizinkan — dan itu tidak bisa dihindari oleh pembatas
// mana pun yang tidak menebak jumlah token di muka.
//
// Penerima nil sah dan berarti tanpa pembatasan, begitu pula Redis nil, supaya deployment
// tanpa Redis memakai jalur kode yang sama.
type Limiter struct {
	redis  *cache.Redis
	logger *slog.Logger

	// rejected berlabel {scope, limit}, milik observability.Metrics.
	rejected *prometheus.CounterVec
	// failOpen mencatat berapa kali batas tidak bisa diperiksa karena Redis.
	failOpen *prometheus.CounterVec

	// failClosed mengubah perilaku saat Redis tidak bisa dihubungi: true = tolak
	// 429 dengan Degraded, false (default) = loloskan + catat. Fail-open menjaga
	// login tetap bisa saat Redis mati, tapi berarti pemadaman Redis mematikan
	// semua pembatasan; instalasi yang menganggap pembatasan sebagai kontrol
	// keamanan menyalakannya lewat RATE_LIMIT_FAIL_CLOSED=true.
	failClosed bool

	keyDefaults Limits
	ipLimits    Limits

	// resolver menyusun daftar cakupan lengkap dari sumber di luar paket ini (tabel
	// rate_limits). nil berarti hanya cakupan key dan IP yang ditegakkan.
	resolver ScopeResolver

	// now bisa diganti test untuk memeriksa perpindahan jendela tanpa menunggu waktu
	// nyata berjalan.
	now func() time.Time
}

// LimiterOption menyetel Limiter saat konstruksi.
type LimiterOption func(*Limiter)

// WithKeyLimitDefaults menetapkan batas yang dipakai saat kolom batas di api_keys NULL.
//
// Bawaannya kosong, yang berarti key tanpa batas tersendiri tidak dibatasi.
//
// DIABAIKAN ketika WithScopeResolver dipasang: sejak tabel rate_limits punya jalur
// pembacaannya sendiri di internal/ratelimit, batas cakupan yang lebih luas datang dari sana,
// dan resolver menyerahkan daftar cakupan yang sudah lengkap. Opsi ini tetap ada untuk
// pemasangan yang tidak memakai tabel itu sama sekali.
func WithKeyLimitDefaults(l Limits) LimiterOption {
	return func(lim *Limiter) { lim.keyDefaults = l }
}

// WithIPLimits menyalakan pembatasan per alamat klien.
//
// Bawaannya kosong — pembatasan per IP MATI kecuali dinyalakan operator, dan itu bukan
// kehati-hatian berlebih. httpx.RealIP hanya mempercayai X-Forwarded-For dari proxy yang
// terdaftar; kalau TRUSTED_PROXIES belum diisi padahal gateway berada di belakang load
// balancer, setiap request akan tampak berasal dari alamat load balancer itu. Batas per IP
// pada keadaan itu menjadi satu ember bersama untuk SELURUH lalu lintas, dan gateway akan
// menolak pelanggan yang tidak melakukan kesalahan apa pun. Nyalakan setelah resolusi IP
// klien terbukti benar.
//
// Batas per IP layak lebih longgar daripada batas per key: satu alamat bisa mewakili
// seluruh kantor di belakang NAT. Nilai seperti Limits{RPS: 20, RPM: 600} menahan satu
// klien yang lepas kendali tanpa mengganggu pemakaian normal.
func WithIPLimits(l Limits) LimiterOption {
	return func(lim *Limiter) { lim.ipLimits = l }
}

// ScopeResolver menyusun SELURUH cakupan pembatasan yang berlaku bagi satu permintaan.
//
// Dipasang oleh perakit aplikasi supaya paket ini tidak perlu tahu apa pun tentang tabel
// rate_limits maupun cache-nya: yang di sini adalah mesin jendela di Redis, dan dari mana
// angka batasnya datang bukan urusannya.
//
// Kontraknya LENGKAP, bukan tambahan: hasilnya menggantikan cakupan key dan IP bawaan, dan
// karena itu wajib menyertakan keduanya bila keduanya masih ingin ditegakkan. Sengaja
// begitu — kalau hasilnya digabungkan, cakupan key akan diperiksa dua kali dan setiap
// request memakan dua jatah kuota, yaitu kegagalan yang hanya terlihat sebagai "batasnya
// separuh dari yang saya setel".
type ScopeResolver func(ctx context.Context, p *Principal, ip netip.Addr) []ScopeLimits

// WithScopeResolver memasang penyusun cakupan pembatasan.
func WithScopeResolver(f ScopeResolver) LimiterOption {
	return func(lim *Limiter) { lim.resolver = f }
}

// WithFailClosed menyalakan mode gagal-tertutup: saat Redis tidak bisa
// dihubungi, request DITOLAK 429 (dengan Degraded true) alih-alih diloloskan.
// Dipasang dari RATE_LIMIT_FAIL_CLOSED di main.
func WithFailClosed(aktif bool) LimiterOption {
	return func(lim *Limiter) { lim.failClosed = aktif }
}

// TokenScopes menyusun cakupan batas token dari kolom di baris api_keys saja.
//
// Dipakai ketika tabel rate_limits tidak dipakai sama sekali. Tidak ada padanan
// ScopeResolver untuk sisi token, dan itu disengaja: cakupan batas token yang lengkap memuat
// cakupan MODEL, dan modelnya baru diketahui di jalur gateway — bukan di middleware, tempat
// resolver dipasang. Penyusunnya karena itu tinggal di jalur gateway (gateway.Guard), dan
// yang di sini hanyalah keadaan tanpa tabel.
func (l *Limiter) TokenScopes(_ context.Context, p *Principal) []ScopeTokenLimits {
	if l == nil {
		return nil
	}
	if limits := p.TokenLimits(); !limits.IsZero() {
		return []ScopeTokenLimits{{Scope: ScopeAPIKey, ID: p.ID(), Limits: limits}}
	}
	return nil
}

// NewLimiter membuat pembatas laju di atas Redis.
//
// rdb nil menghasilkan pembatas yang selalu meloloskan. metrics boleh nil. logger nil
// jatuh ke slog.Default().
func NewLimiter(rdb *cache.Redis, metrics *observability.Metrics, logger *slog.Logger, opts ...LimiterOption) *Limiter {
	if logger == nil {
		logger = slog.Default()
	}
	l := &Limiter{
		redis:    rdb,
		logger:   logger,
		failOpen: registerFailOpen(metrics),
		now:      time.Now,
	}
	if metrics != nil {
		l.rejected = metrics.RateLimitRejected
	}
	for _, opt := range opts {
		if opt != nil {
			opt(l)
		}
	}
	return l
}

// registerFailOpen mendaftarkan penghitung gagal-terbuka ke registry aplikasi.
//
// Metrik ini wajib ada dan wajib dipantau: ia satu-satunya tanda bahwa gateway sedang
// berjalan tanpa penegakan batas laju.
func registerFailOpen(m *observability.Metrics) *prometheus.CounterVec {
	c := prometheus.NewCounterVec(prometheus.CounterOpts{
		Name: "routex_rate_limit_failopen_total",
		Help: "Jumlah pemeriksaan batas laju yang diloloskan karena Redis tidak bisa dihubungi.",
	}, []string{"op"})
	if m == nil {
		return c
	}
	if err := m.Registry().Register(c); err != nil {
		var already prometheus.AlreadyRegisteredError
		if errors.As(err, &already) {
			if existing, ok := already.ExistingCollector.(*prometheus.CounterVec); ok {
				return existing
			}
		}
	}
	return c
}

// log mengembalikan logger pembatas yang sudah dibubuhi request_id bila context punya.
func (l *Limiter) log(ctx context.Context) *slog.Logger {
	if id := observability.RequestIDFrom(ctx); id != "" {
		return l.logger.With(slog.String("request_id", id))
	}
	return l.logger
}

// Warm memuat skrip ke cache skrip Redis supaya request pertama tidak menanggung satu
// putaran EVALSHA yang gagal ditambah EVAL berisi badan skrip. Bukan keharusan; jalur
// pemeriksaan tetap benar tanpa pemanasan.
func (l *Limiter) Warm(ctx context.Context) error {
	if l == nil || l.redis == nil {
		return nil
	}
	return l.redis.LoadScript(ctx, limiterScript)
}

// --- Jendela -----------------------------------------------------------------

// window adalah satu penghitung yang diperiksa dalam satu panggilan skrip.
type window struct {
	scope   string
	limit   string
	id      string
	bucket  string
	value   int64
	resetAt time.Time
}

// key menyusun kunci Redis lewat cache.RateLimitKey, satu-satunya pembentuk kunci yang
// boleh dipakai supaya seluruh kunci aplikasi tetap bernamespace dan bisa diaudit dari
// satu berkas.
func (w window) key() string {
	return cache.RateLimitKey(w.scope, w.id, w.limit+"-"+w.bucket)
}

// requestWindows menyusun jendela untuk batas berbasis jumlah request.
//
// Urutannya menentukan batas mana yang dilaporkan saat lebih dari satu terlampaui:
// yang paling pendek lebih dulu, karena itulah yang paling cepat pulih dan paling
// berguna diberitahukan ke klien.
func requestWindows(scope, id string, l Limits, now time.Time) []window {
	if id == "" || l.IsZero() {
		return nil
	}
	out := make([]window, 0, 4)
	if l.RPS > 0 {
		bucket, resetAt := secondWindow(now)
		out = append(out, window{scope, LimitRPS, id, bucket, int64(l.RPS), resetAt})
	}
	if l.RPM > 0 {
		bucket, resetAt := minuteWindow(now)
		out = append(out, window{scope, LimitRPM, id, bucket, int64(l.RPM), resetAt})
	}
	if l.Daily > 0 {
		bucket, resetAt := dayWindow(now)
		out = append(out, window{scope, LimitDaily, id, bucket, l.Daily, resetAt})
	}
	if l.Monthly > 0 {
		bucket, resetAt := monthWindow(now)
		out = append(out, window{scope, LimitMonthly, id, bucket, l.Monthly, resetAt})
	}
	return out
}

// tokenWindows menyusun jendela untuk batas berbasis token pada satu cakupan.
//
// Cakupannya parameter, bukan selalu key: batas token juga bisa dipasang operator pada
// cakupan yang lebih luas (global, pengguna, model) lewat tabel rate_limits, dan jumlah
// token yang dipakai satu model adalah angka yang memang ingin dibatasi terpisah dari
// jumlah requestnya.
func tokenWindows(scope, id string, l TokenLimits, now time.Time) []window {
	if id == "" || l.IsZero() {
		return nil
	}
	out := make([]window, 0, 3)
	if l.TPM > 0 {
		bucket, resetAt := minuteWindow(now)
		out = append(out, window{scope, LimitTPM, id, bucket, int64(l.TPM), resetAt})
	}
	if l.Daily > 0 {
		bucket, resetAt := dayWindow(now)
		out = append(out, window{scope, LimitDailyTokens, id, bucket, l.Daily, resetAt})
	}
	if l.Monthly > 0 {
		bucket, resetAt := monthWindow(now)
		out = append(out, window{scope, LimitMonthlyTokens, id, bucket, l.Monthly, resetAt})
	}
	return out
}

// Nomor jendela detik dan menit memakai detik epoch, jadi batasnya sama di seluruh
// instance tanpa bergantung pada zona waktu masing-masing. Truncate bekerja terhadap
// waktu absolut sejak epoch, sehingga hasilnya selalu jatuh di batas detik dan menit UTC.
func secondWindow(now time.Time) (string, time.Time) {
	start := now.Truncate(time.Second)
	return strconv.FormatInt(start.Unix(), 10), start.Add(time.Second)
}

func minuteWindow(now time.Time) (string, time.Time) {
	start := now.Truncate(time.Minute)
	return strconv.FormatInt(start.Unix()/60, 10), start.Add(time.Minute)
}

// Jendela harian dan bulanan memakai tanggal kalender UTC, bukan pembagian detik epoch:
// panjang bulan tidak tetap, dan pelanggan menghitung kuotanya dalam tanggal.
func dayWindow(now time.Time) (string, time.Time) {
	utc := now.UTC()
	start := time.Date(utc.Year(), utc.Month(), utc.Day(), 0, 0, 0, 0, time.UTC)
	return start.Format("20060102"), start.AddDate(0, 0, 1)
}

func monthWindow(now time.Time) (string, time.Time) {
	utc := now.UTC()
	start := time.Date(utc.Year(), utc.Month(), 1, 0, 0, 0, 0, time.UTC)
	return start.Format("200601"), start.AddDate(0, 1, 0)
}

// --- Pemeriksaan -------------------------------------------------------------

// Allow memeriksa seluruh batas berbasis jumlah request untuk satu request, lalu
// menghitungnya bila lolos.
//
// Batas cakupan key dan cakupan IP diperiksa dalam SATU panggilan skrip, sehingga request
// yang ditolak batas IP tidak memakan kuota key, dan sebaliknya.
//
// keyID kosong melewatkan cakupan key; ip yang tidak valid melewatkan cakupan IP. Tidak
// ada batas yang aktif berarti Redis tidak disentuh sama sekali.
func (l *Limiter) Allow(ctx context.Context, keyID string, limits Limits, ip netip.Addr) Decision {
	if l == nil || l.redis == nil {
		return Decision{Allowed: true}
	}
	now := l.now()

	// Batas key lebih dulu: itu yang tertulis pada key pelanggan, jadi itu yang paling
	// pantas dilaporkan lewat header bila keduanya sama-sama membatasi.
	scopes := []ScopeLimits{{Scope: ScopeAPIKey, ID: keyID, Limits: limits.WithDefaults(l.keyDefaults)}}
	if ip.IsValid() {
		scopes = append(scopes, ScopeLimits{Scope: ScopeIP, ID: ip.String(), Limits: l.ipLimits})
	}
	return l.allowScopesAt(ctx, scopes, now)
}

// ScopeLimits adalah satu cakupan pembatasan beserta batas request-nya.
//
// Ada karena batas laju bukan hanya milik API key. Operator memasang batas pada cakupan
// yang lebih luas lewat tabel rate_limits — global, pengguna, model, provider — dan
// semuanya harus diperiksa dalam SATU keputusan. Kalau diperiksa berurutan, request yang
// ditolak batas per detik tetap sudah memakan jatah kuota harian cakupan lain, dan klien
// bisa kehabisan kuota harinya tanpa satu pun request berhasil.
type ScopeLimits struct {
	// Scope adalah salah satu konstanta Scope di atas; ikut ke kunci Redis dan label metrik.
	Scope string
	// ID adalah pengenal entitasnya. Kosong berarti cakupan ini dilewati.
	ID     string
	Limits Limits
}

// ScopeTokenLimits adalah satu cakupan beserta batas token-nya.
type ScopeTokenLimits struct {
	Scope  string
	ID     string
	Limits TokenLimits
}

// AllowScopes memeriksa batas request pada BEBERAPA cakupan sekaligus.
//
// Seluruh jendela dari seluruh cakupan diperiksa dalam satu pemanggilan skrip, dan
// penghitung baru dinaikkan bila semuanya lolos. Urutan cakupan menentukan batas mana yang
// dilaporkan lewat header ketika lebih dari satu membatasi, jadi pemanggil menaruh yang
// paling relevan bagi klien di depan.
func (l *Limiter) AllowScopes(ctx context.Context, scopes []ScopeLimits) Decision {
	if l == nil || l.redis == nil {
		return Decision{Allowed: true}
	}
	return l.allowScopesAt(ctx, scopes, l.now())
}

// allowScopesAt adalah AllowScopes dengan waktu yang sudah ditentukan pemanggil, supaya
// Allow dan AllowScopes memakai satu jalur yang sama tanpa memanggil l.now() dua kali —
// dua pembacaan waktu dalam satu keputusan bisa jatuh di dua jendela yang berbeda.
func (l *Limiter) allowScopesAt(ctx context.Context, scopes []ScopeLimits, now time.Time) Decision {
	if l == nil || l.redis == nil {
		return Decision{Allowed: true}
	}
	var windows []window
	for _, s := range scopes {
		windows = append(windows, requestWindows(s.Scope, s.ID, s.Limits, now)...)
	}
	return l.run(ctx, modeEnforce, 1, windows, now, "enforce")
}

// TokensExceededScopes melaporkan apakah anggaran token salah satu cakupan sudah habis,
// tanpa mengubah penghitung apa pun.
//
// Gerbang sebelum request diteruskan ke upstream. Allowed false berarti request ini layak
// dibalas 429: token yang sudah terpakai melewati batas, dan menerima satu request lagi
// berarti membiarkan pemakaian melampaui batas semakin jauh.
func (l *Limiter) TokensExceededScopes(ctx context.Context, scopes []ScopeTokenLimits) Decision {
	if l == nil || l.redis == nil {
		return Decision{Allowed: true}
	}
	now := l.now()
	return l.run(ctx, modePeek, 0, tokenWindowsFor(scopes, now), now, "peek")
}

// RecordTokensScopes mencatat pemakaian token pada seluruh cakupan sekaligus.
//
// SENGAJA tidak menolak apa pun: saat jumlah token diketahui, request-nya sudah dilayani
// dan biayanya sudah dikeluarkan ke upstream. Yang bisa dilakukan hanyalah mencatatnya lalu
// memakai catatan itu untuk menggerbangi request berikutnya lewat TokensExceededScopes.
//
// Kegagalannya tidak boleh menggagalkan apa pun; yang hilang adalah ketepatan penghitung,
// bukan responsnya.
func (l *Limiter) RecordTokensScopes(ctx context.Context, scopes []ScopeTokenLimits, tokens int64) Decision {
	if l == nil || l.redis == nil {
		return Decision{Allowed: true}
	}
	if tokens <= 0 {
		return l.TokensExceededScopes(ctx, scopes)
	}
	now := l.now()
	return l.run(ctx, modeRecord, tokens, tokenWindowsFor(scopes, now), now, "record")
}

// tokenWindowsFor menggabungkan jendela token dari beberapa cakupan.
func tokenWindowsFor(scopes []ScopeTokenLimits, now time.Time) []window {
	var out []window
	for _, s := range scopes {
		out = append(out, tokenWindows(s.Scope, s.ID, s.Limits, now)...)
	}
	return out
}

// RecordTokens mencatat pemakaian token dan melaporkan apakah anggarannya sudah habis.
//
// Ini pasangan Limit() untuk sisi token, dan ia SENGAJA tidak menolak apa pun: saat
// jumlah token diketahui, request-nya sudah dilayani dan biayanya sudah dikeluarkan ke
// upstream. Yang bisa dilakukan hanyalah mencatatnya, lalu memakai catatan itu untuk
// menggerbangi request berikutnya lewat TokensExceeded.
//
// Panggil setelah respons upstream selesai — untuk streaming, setelah event terakhir,
// ketika jumlah token keluaran sudah pasti. Kegagalannya tidak boleh menggagalkan
// apa pun; yang hilang adalah ketepatan penghitung, bukan responsnya.
//
// tokens <= 0 tidak menulis apa pun dan hanya melaporkan keadaan sekarang.
func (l *Limiter) RecordTokens(ctx context.Context, keyID string, limits TokenLimits, tokens int64) Decision {
	if l == nil || l.redis == nil {
		return Decision{Allowed: true}
	}
	if tokens <= 0 {
		return l.TokensExceeded(ctx, keyID, limits)
	}
	now := l.now()
	return l.run(ctx, modeRecord, tokens, tokenWindows(ScopeAPIKey, keyID, limits, now), now, "record")
}

// TokensExceeded melaporkan apakah anggaran token key ini sudah habis, tanpa mengubah
// penghitung apa pun.
//
// Dimaksudkan sebagai gerbang sebelum request diteruskan ke upstream, jadi hasil "habis"
// ikut dihitung sebagai penolakan pada metrik. Allowed false berarti request berikutnya
// layak dibalas 429 dengan ResetAt sebagai waktu pulihnya kuota.
func (l *Limiter) TokensExceeded(ctx context.Context, keyID string, limits TokenLimits) Decision {
	if l == nil || l.redis == nil {
		return Decision{Allowed: true}
	}
	now := l.now()
	return l.run(ctx, modePeek, 0, tokenWindows(ScopeAPIKey, keyID, limits, now), now, "peek")
}

// run menjalankan skrip untuk sekumpulan jendela dan menerjemahkan hasilnya.
func (l *Limiter) run(ctx context.Context, mode int, cost int64, windows []window, now time.Time, op string) Decision {
	if len(windows) == 0 {
		// Tidak ada batas yang aktif. Redis tidak disentuh, dan tidak ada header kuota
		// yang dipasang: LimitValue nol.
		return Decision{Allowed: true}
	}

	redisKeys := make([]string, len(windows))
	args := make([]any, 0, 2+2*len(windows))
	args = append(args, mode, cost)
	for i, w := range windows {
		redisKeys[i] = w.key()
		args = append(args, w.value, ttlMillis(w, now))
	}

	res, err := l.redis.RunScript(ctx, limiterScript, redisKeys, args...)
	if err != nil {
		return l.failOpenDecision(ctx, op, err)
	}

	exceeded, counts, ok := parseScriptResult(res, len(windows))
	if !ok {
		// Bentuk kembalian yang tidak dikenali diperlakukan sama seperti Redis mati:
		// diloloskan dan dicatat. Menebak isinya berarti mengambil keputusan penolakan
		// dari data yang tidak dipahami.
		return l.failOpenDecision(ctx, op, errors.New("bentuk kembalian skrip tidak dikenali"))
	}

	decision := decisionFrom(windows, counts, exceeded, now)
	if !decision.Allowed && mode != modeRecord && l.rejected != nil {
		l.rejected.WithLabelValues(decision.Scope, decision.Limit).Inc()
	}
	return decision
}

// failOpenDecision meloloskan request saat batas tidak bisa diperiksa, sambil mencatatnya
// di log dan metrik. Lihat penjelasan gagal-terbuka di dokumentasi Limiter.
//
// Bila failClosed aktif (RATE_LIMIT_FAIL_CLOSED=true), keputusannya dibalik:
// request DITOLAK dengan Degraded true. Penolakan memakai pesan yang menjelaskan
// sebabnya supaya klien bisa membedakannya dari kuota yang benar-benar habis —
// retry dengan backoff adalah respons yang benar, bukan menghubungi operator.
func (l *Limiter) failOpenDecision(ctx context.Context, op string, err error) Decision {
	l.failOpen.WithLabelValues(op).Inc()
	if l != nil && l.failClosed {
		l.log(ctx).LogAttrs(ctx, slog.LevelError,
			"batas laju tidak bisa diperiksa, request ditolak (fail-closed)",
			slog.String("op", op),
			slog.String("error", err.Error()))
		return Decision{Allowed: false, Degraded: true, Scope: "infrastruktur", Limit: "redis-tidak-tersedia"}
	}
	l.log(ctx).LogAttrs(ctx, slog.LevelWarn,
		"batas laju tidak bisa diperiksa, request diloloskan",
		slog.String("op", op),
		slog.String("error", err.Error()))
	return Decision{Allowed: true, Degraded: true}
}

// ttlMillis adalah sisa umur jendela, yaitu tepat sampai jendela itu berakhir.
//
// Kunci sudah membawa nomor jendelanya sendiri, jadi TTL tidak perlu lebih panjang dari
// itu: penghitung jendela yang sudah lewat tidak akan pernah dipakai lagi dan lebih baik
// hilang sendiri. Minimum satu milidetik supaya PEXPIRE tidak pernah dipanggil dengan nol
// atau negatif, yang justru akan langsung menghapus kuncinya.
func ttlMillis(w window, now time.Time) int64 {
	ms := w.resetAt.Sub(now).Milliseconds()
	if ms < 1 {
		return 1
	}
	return ms
}

// parseScriptResult membaca kembalian skrip. Ditulis defensif: hasil melewati pemetaan
// tipe go-redis, dan bentuk yang tidak sesuai harus terdeteksi di sini alih-alih menjadi
// keputusan penolakan yang salah.
func parseScriptResult(res any, n int) (exceeded int, counts []int64, ok bool) {
	values, isSlice := res.([]any)
	if !isSlice || len(values) != n+1 {
		return 0, nil, false
	}
	first, isInt := values[0].(int64)
	if !isInt || first < 0 || first > int64(n) {
		return 0, nil, false
	}
	counts = make([]int64, n)
	for i := range counts {
		v, isInt := values[i+1].(int64)
		if !isInt {
			return 0, nil, false
		}
		counts[i] = v
	}
	return int(first), counts, true
}

// decisionFrom memilih satu jendela untuk dilaporkan.
//
// Bila ada yang terlampaui, itulah yang dilaporkan — klien perlu tahu batas mana yang
// menghentikannya dan kapan ia pulih. Bila semuanya lolos, yang dilaporkan adalah jendela
// dengan sisa kuota paling sedikit, yaitu batas yang akan lebih dulu mengenai klien;
// pada sisa yang sama, yang lebih cepat pulih dipilih supaya klien tidak menahan diri
// lebih lama dari perlunya.
func decisionFrom(windows []window, counts []int64, exceeded int, now time.Time) Decision {
	if exceeded > 0 {
		w := windows[exceeded-1]
		retryAfter := w.resetAt.Sub(now)
		if retryAfter < 0 {
			retryAfter = 0
		}
		return Decision{
			Scope:      w.scope,
			Limit:      w.limit,
			LimitValue: w.value,
			Remaining:  0,
			ResetAt:    w.resetAt,
			RetryAfter: retryAfter,
		}
	}

	best, bestRemaining := -1, int64(0)
	for i, w := range windows {
		remaining := w.value - counts[i]
		if remaining < 0 {
			remaining = 0
		}
		if best < 0 || remaining < bestRemaining ||
			(remaining == bestRemaining && w.resetAt.Before(windows[best].resetAt)) {
			best, bestRemaining = i, remaining
		}
	}

	w := windows[best]
	return Decision{
		Allowed:    true,
		Scope:      w.scope,
		Limit:      w.limit,
		LimitValue: w.value,
		Remaining:  bestRemaining,
		ResetAt:    w.resetAt,
	}
}

// --- Middleware --------------------------------------------------------------

// Limit menegakkan batas berbasis jumlah request pada setiap request.
//
// Pasang SETELAH Authenticate: pemilik kuota adalah key yang sudah terverifikasi, dan
// tanpa principal di context tidak ada yang bisa dibatasi. Rute tanpa Authenticate
// dijawab 401 dan dicatat sebagai kesalahan perakitan router, bukan diloloskan —
// meloloskannya berarti ada jalur /v1/* tanpa batas sama sekali tanpa ada yang tahu.
//
// Header X-RateLimit-* dipasang pada respons yang lolos maupun yang ditolak. Itu bagian
// dari kontrak klien OpenAI-compatible: pustaka klien memakainya untuk memperlambat diri
// sebelum benar-benar ditolak, dan tanpa header pada respons yang berhasil, satu-satunya
// cara mereka mengetahui batas adalah dengan menabraknya.
//
// Penolakan memakai httpx.TooManyRequests, yang memasang Retry-After dari sisa umur
// jendela — bukan dari lebar jendela penuh, yang selalu terlalu lama dan membuat klien
// menganggur lebih panjang dari perlunya.
func (l *Limiter) Limit() func(http.Handler) http.Handler {
	return func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			ctx := r.Context()

			principal, ok := PrincipalFrom(ctx)
			if !ok {
				l.log(ctx).LogAttrs(ctx, slog.LevelError,
					"rute dibatasi laju tetapi tidak dipasang di belakang Authenticate",
					slog.String("method", r.Method),
					slog.String("path", r.URL.Path))
				httpx.Unauthorized(w, r, MessageInvalidKey)
				return
			}

			clientIP, _ := httpx.ClientIPFrom(ctx)
			// Dengan resolver, seluruh cakupan datang dari sana — termasuk cakupan key dan
			// IP. Lihat kontrak ScopeResolver soal mengapa hasilnya menggantikan, bukan
			// menambah.
			var decision Decision
			if l.resolver != nil {
				decision = l.AllowScopes(ctx, l.resolver(ctx, principal, clientIP))
			} else {
				decision = l.Allow(ctx, principal.ID(), LimitsFromKey(principal.Key), clientIP)
			}
			decision.SetHeaders(w)

			if !decision.Allowed {
				// Info, bukan Warn: request yang tertahan batas adalah kejadian normal
				// pada klien yang sibuk, dan log akses sudah mencatat 429-nya. Yang
				// ditambahkan di sini adalah batas MANA yang mengenainya, satu-satunya
				// keterangan yang tidak bisa disimpulkan dari log akses.
				l.log(ctx).LogAttrs(ctx, slog.LevelInfo,
					"request ditolak pembatas laju",
					slog.String("api_key_id", principal.ID()),
					slog.String("scope", decision.Scope),
					slog.String("limit", decision.Limit),
					slog.Int64("limit_value", decision.LimitValue),
					slog.Duration("retry_after", decision.RetryAfter),
					slog.String("method", r.Method),
					slog.String("path", r.URL.Path))

				httpx.TooManyRequests(w, r, MessageRateLimited, decision.RetryAfter)
				return
			}

			next.ServeHTTP(w, r)
		})
	}
}
