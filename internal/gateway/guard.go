// Package gateway memuat perkakas jalur request. Berkas ini menegakkan kebijakan yang
// MEMBATASI lalu lintas — penyaring konten, batas laju, dan anggaran biaya — pada titik-titik
// yang tepat di dalam satu permintaan.
//
// # Kenapa pemeriksaannya bertahap, bukan satu panggilan
//
// Bahan yang dibutuhkan tiap kebijakan tersedia pada saat yang berbeda:
//
//	body terbaca        → ukuran permintaan, pola konten
//	model diselesaikan  → pembatasan model, batas laju cakupan model, anggaran, batas token
//	kandidat dipilih    → pembatasan provider (menyaring kandidat, bukan menolak permintaan)
//	percobaan dimulai   → batas laju cakupan provider
//	jawaban selesai     → pencatatan token, penyaring konten fase jawaban
//
// Memaksakan semuanya ke satu titik berarti salah satu dari dua hal: pemeriksaan dijalankan
// tanpa bahan yang dibutuhkannya sehingga tidak pernah memicu, atau permintaan yang jelas
// akan ditolak tetap dikirim ke provider lebih dulu. Yang kedua berbayar dalam uang.
//
// # Urutannya bukan selera
//
// Yang paling murah dan paling pasti didahulukan: ukuran body sebelum pola, pola sebelum
// perjalanan ke Redis, Redis sebelum panggilan ke upstream. Setiap tahap yang dilewati lebih
// dulu adalah biaya yang tidak dikeluarkan untuk permintaan yang toh ditolak.
package gateway

import (
	"context"
	"log/slog"
	"net/http"
	"net/netip"
	"time"

	"github.com/NexGen-X/Route-X/internal/apikey"
	"github.com/NexGen-X/Route-X/internal/billing"
	"github.com/NexGen-X/Route-X/internal/contentfilter"
	"github.com/NexGen-X/Route-X/internal/database/repo/policy"
	"github.com/NexGen-X/Route-X/internal/httpx"
	"github.com/NexGen-X/Route-X/internal/observability"
	"github.com/NexGen-X/Route-X/internal/providers"
	"github.com/NexGen-X/Route-X/internal/ratelimit"
)

// Kode penolakan kebijakan pada envelope error.
//
// Mengikuti kosakata OpenAI di mana ada padanannya (content_filter), dan menyebut sebabnya
// secara eksplisit di mana tidak ada — klien yang menerima "budget_exceeded" bisa
// membedakannya dari batas laju tanpa membaca pesannya.
const (
	CodeContentFilter  = "content_filter"
	CodePayloadTooBig  = "payload_too_large"
	CodeBudgetExceeded = "budget_exceeded"
	CodeRateLimited    = "rate_limit_exceeded"
)

// Rejection adalah penolakan kebijakan yang siap ditulis sebagai respons HTTP.
//
// Nilai, bukan penulisan langsung ke ResponseWriter: penolakan kebijakan bisa terjadi di
// jalur streaming, dan di sana pemanggil perlu memutuskan sendiri apakah masih boleh menulis
// status atau harus melaporkannya sebagai peristiwa di dalam aliran.
type Rejection struct {
	Status  int
	Type    string
	Code    string
	Message string
	// RetryAfter > 0 dipasang sebagai header Retry-After. Hanya bermakna untuk penolakan
	// yang memang pulih dengan menunggu.
	RetryAfter time.Duration
	// Headers memuat header tambahan, mis. X-RateLimit-*.
	Headers func(w http.ResponseWriter)
}

// Tulis membalas penolakan ini. Hanya boleh dipanggil sebelum satu byte respons terkirim.
func (rj *Rejection) Tulis(w http.ResponseWriter, r *http.Request) {
	if rj.Headers != nil {
		rj.Headers(w)
	}
	if rj.RetryAfter > 0 {
		w.Header().Set("Retry-After", retryAfterHeader(rj.RetryAfter))
	}
	httpx.WriteError(w, r, rj.Status, rj.Type, rj.Code, rj.Message)
}

// GuardDeps mengumpulkan dependensi Guard. Semuanya boleh nil, dan nil berarti kebijakan
// yang bersangkutan tidak ditegakkan — keadaan yang benar untuk pemasangan yang belum
// mengisi tabelnya.
type GuardDeps struct {
	Filters *contentfilter.Engine
	Budgets *billing.Enforcer
	Rates   *ratelimit.Engine
	// Limiter adalah mesin jendela di Redis. Tanpa ini, batas dari tabel rate_limits tidak
	// bisa ditegakkan sama sekali walau Rates terpasang.
	Limiter *apikey.Limiter
	// Metrics dipakai menghitung penolakan penyaring konten. nil berarti penolakan hanya
	// masuk log — dan log saja tidak bisa dijadikan alert, sehingga aturan penyaring yang
	// tiba-tiba memblokir seluruh lalu lintas satu penyewa tidak terlihat sampai penyewa itu
	// mengeluh.
	Metrics *observability.Metrics
	Logger  *slog.Logger
}

// Guard menegakkan kebijakan pembatasan lalu lintas pada jalur request.
type Guard struct {
	filters *contentfilter.Engine
	budgets *billing.Enforcer
	rates   *ratelimit.Engine
	limiter *apikey.Limiter
	metrics *observability.Metrics
	logger  *slog.Logger
}

// NewGuard membuat penegak kebijakan. Penerima nil sah dan berarti tanpa kebijakan.
func NewGuard(d GuardDeps) *Guard {
	logger := d.Logger
	if logger == nil {
		logger = slog.New(slog.DiscardHandler)
	}
	return &Guard{
		filters: d.Filters, budgets: d.Budgets, rates: d.Rates,
		limiter: d.Limiter, metrics: d.Metrics, logger: logger,
	}
}

// TargetsFor menyusun target kebijakan satu permintaan.
//
// Urutannya menentukan batas mana yang dilaporkan lewat header X-RateLimit-* ketika lebih
// dari satu membatasi: yang paling spesifik lebih dulu, karena itulah yang bisa
// ditindaklanjuti pemilik key. Cakupan global terakhir — pengguna tidak bisa berbuat apa pun
// terhadapnya, jadi melaporkannya lebih dulu hanya membingungkan.
func TargetsFor(p *apikey.Principal, modelID string, ip netip.Addr) []policy.Target {
	out := make([]policy.Target, 0, 5)
	if id := p.ID(); id != "" {
		out = append(out, policy.Target{Scope: policy.ScopeAPIKey, ID: id})
	}
	if u := p.OwnerUserID(); u != "" {
		out = append(out, policy.Target{Scope: policy.ScopeUser, ID: u})
	}
	if modelID != "" {
		out = append(out, policy.Target{Scope: policy.ScopeModel, ID: modelID})
	}
	if ip.IsValid() {
		out = append(out, policy.Target{Scope: policy.ScopeIP, ID: ip.String()})
	}
	return append(out, policy.Target{Scope: policy.ScopeGlobal})
}

// SaringPermintaan menjalankan penyaring konten atas isi permintaan.
func (g *Guard) SaringPermintaan(ctx context.Context, s contentfilter.Subject) *Rejection {
	if g == nil || g.filters == nil {
		return nil
	}
	s.Response = false
	return g.saring(ctx, s)
}

// SaringJawaban menjalankan penyaring konten atas isi jawaban.
//
// Hanya dipakai jalur non-streaming. Pada streaming, jawabannya sampai ke klien sepotong demi
// sepotong: memeriksa setiap potongan sendiri-sendiri melewatkan pola yang melintasi batas
// potongan — sehingga memberi rasa aman yang tidak benar — sementara menahan seluruh aliran
// sampai selesai demi memeriksanya membatalkan seluruh gunanya streaming. Batas ini disengaja
// dan tercatat; menutupnya butuh keputusan tentang mana yang dikorbankan.
func (g *Guard) SaringJawaban(ctx context.Context, teks, modelID, providerID string) *Rejection {
	if g == nil || g.filters == nil || teks == "" {
		return nil
	}
	return g.saring(ctx, contentfilter.Subject{
		Text: teks, ModelID: modelID, ProviderID: providerID, Response: true,
	})
}

// saring menjalankan mesin penyaring dan menerjemahkan hasilnya.
func (g *Guard) saring(ctx context.Context, s contentfilter.Subject) *Rejection {
	v := g.filters.Check(ctx, s)
	for _, w := range v.Warnings {
		// Aturan berjenis warn memang tidak menghentikan lalu lintas; log inilah gunanya,
		// dan tanpa mencatatnya aturan warn menjadi aturan yang tidak melakukan apa pun.
		g.logger.InfoContext(ctx, "penyaring konten terpicu tanpa memblokir",
			"penyaring", w.Name, "kind", w.Kind, "jawaban", s.Response)
	}
	if !v.Blocked {
		return nil
	}
	g.logger.InfoContext(ctx, "permintaan ditolak penyaring konten",
		"penyaring", v.Filter.Name, "kind", v.Filter.Kind, "jawaban", s.Response)
	if g.metrics != nil {
		// Dipecah per NAMA aturan, bukan per jenis: yang ditanyakan operator saat lalu lintas
		// mendadak ditolak adalah aturan mana, dan nama itu yang ia lihat di dashboard.
		// Namanya berasal dari baris content_filters yang dikurasi operator, jadi jumlah
		// nilainya terbatas.
		g.metrics.ContentBlocked.WithLabelValues(v.Filter.Name).Inc()
	}

	if v.Filter.Kind == policy.FilterRequestSize {
		return &Rejection{
			Status: http.StatusRequestEntityTooLarge, Type: httpx.ErrTypeInvalidRequest,
			Code: CodePayloadTooBig, Message: v.Reason,
		}
	}
	return &Rejection{
		Status: http.StatusBadRequest, Type: httpx.ErrTypeInvalidRequest,
		Code: CodeContentFilter, Message: v.Reason,
	}
}

// ProviderDilarang melaporkan apakah satu provider dilarang penyaring konten.
//
// Dipakai untuk MENYARING kandidat, bukan untuk menolak permintaan. Bedanya penting: kalau
// masih ada kandidat lain, permintaan harus tetap dilayani lewat kandidat itu — dan kalau
// tidak ada, penolakannya berbunyi "tidak ada provider yang bisa melayani model ini", yang
// bisa ditindaklanjuti, bukan 502 dari provider yang sengaja tidak dihubungi.
func (g *Guard) ProviderDilarang(ctx context.Context, providerID string) bool {
	if g == nil || g.filters == nil || providerID == "" {
		return false
	}
	return g.filters.Check(ctx, contentfilter.Subject{ProviderID: providerID}).Blocked
}

// PeriksaAnggaran menegakkan anggaran biaya.
//
// Penolakannya 402, bukan 429. Anggaran yang habis tidak pulih dengan menunggu — ia pulih
// ketika periodenya berganti — jadi 429 beserta Retry-After akan mengajak pustaka klien
// tidur sampai pergantian bulan lalu mengulang. 402 sampai ke pemanggil sebagai kegagalan
// yang jelas bukan kesalahan permintaannya dan jelas tidak akan berubah dengan mengulang.
func (g *Guard) PeriksaAnggaran(ctx context.Context, targets []policy.Target) *Rejection {
	if g == nil || g.budgets == nil {
		return nil
	}
	v := g.budgets.Check(ctx, targets)
	if len(v.Alerts) > 0 {
		g.budgets.Announce(ctx, v.Alerts)
	}
	if !v.Blocked {
		return nil
	}
	g.logger.WarnContext(ctx, "permintaan ditolak karena anggaran habis",
		"anggaran", v.Budget.Name, "cakupan", v.Budget.Scope, "periode", v.Budget.Period,
		"terpakai_usd", v.Budget.SpentUSD.String(), "batas_usd", v.Budget.LimitUSD.String())

	return &Rejection{
		Status: http.StatusPaymentRequired, Type: httpx.ErrTypeAPI, Code: CodeBudgetExceeded,
		// Nama anggaran disebut karena itu setelan operator dan justru itu yang membuat
		// pengguna tahu harus menghubungi siapa. Angkanya TIDAK disebut: batas dan pemakaian
		// satu penyewa bukan hal yang pantas dikirim ke penyewa mana pun yang memicunya.
		Message: "anggaran " + kutipKebijakan(v.Budget.Name) + " untuk periode ini sudah habis",
	}
}

// PeriksaBatasModel menegakkan batas laju cakupan MODEL.
//
// Hanya cakupan model, karena cakupan lainnya — key, pengguna, IP, global — sudah ditegakkan
// middleware apikey.Limit() sebelum body dibaca. Memeriksanya lagi di sini berarti setiap
// permintaan memakan dua jatah kuota pada penghitung yang sama, dan gejalanya hanya berupa
// "batasnya separuh dari yang saya setel".
//
// Cakupan model tidak bisa ikut ke middleware itu karena modelnya baru diketahui setelah body
// diurai dan aliasnya diselesaikan.
func (g *Guard) PeriksaBatasModel(ctx context.Context, modelID string) *Rejection {
	if g == nil || g.rates == nil || g.limiter == nil || modelID == "" {
		return nil
	}
	scopes := g.rates.Requests(ctx, []policy.Target{{Scope: policy.ScopeModel, ID: modelID}}, apikey.Limits{})
	if len(scopes) == 0 {
		return nil
	}
	d := g.limiter.AllowScopes(ctx, scopes)
	if d.Allowed {
		return nil
	}
	g.logger.InfoContext(ctx, "permintaan ditolak batas laju cakupan model",
		"model", modelID, "batas", d.Limit, "nilai", d.LimitValue)
	return rejectionBatas(d, "model "+kutipKebijakan(modelID)+" sedang melewati batas lajunya")
}

// PeriksaBatasProvider menegakkan batas laju cakupan PROVIDER untuk satu percobaan.
//
// Mengembalikan *providers.Error, bukan Rejection, dan itu yang membuatnya bekerja seperti
// yang seharusnya: mesin eksekusi memperlakukan ErrKindRateLimit sebagai kegagalan yang aman
// DIALIHKAN, sehingga provider yang sedang melewati batasnya dilewati dan kandidat berikutnya
// dicoba. Menolak seluruh permintaan di sini akan membuang kandidat lain yang sehat.
//
// Inilah batas yang dimaksud kolom rate_limit_rpm di tabel providers: dipaksakan gateway
// SEBELUM memanggil upstream, supaya kuota pihak ketiga tidak terlanggar dari sisi kami.
func (g *Guard) PeriksaBatasProvider(ctx context.Context, providerID, providerName string) *providers.Error {
	if g == nil || g.rates == nil || g.limiter == nil || providerID == "" {
		return nil
	}
	scopes := g.rates.Requests(ctx, []policy.Target{{Scope: policy.ScopeProvider, ID: providerID}}, apikey.Limits{})
	if len(scopes) == 0 {
		return nil
	}
	d := g.limiter.AllowScopes(ctx, scopes)
	if d.Allowed {
		return nil
	}
	g.logger.InfoContext(ctx, "kandidat dilewati karena batas laju cakupan provider",
		"provider", providerName, "batas", d.Limit, "nilai", d.LimitValue)

	return &providers.Error{
		Kind:     providers.ErrKindRateLimit,
		Provider: providerName,
		Message:  "batas laju gateway ke provider ini sedang penuh",
		// Sisa umur jendela, bukan lebarnya: mesin eksekusi memakainya untuk memutuskan
		// menunggu sebentar atau langsung melewati kandidat ini.
		RetryAfter: d.RetryAfter,
	}
}

// GerbangToken memeriksa apakah anggaran token sudah habis SEBELUM upstream dihubungi.
//
// Berbeda dari batas berbasis jumlah request, batas token tidak bisa ditegakkan di muka:
// besarnya sebuah permintaan baru diketahui setelah dijawab. Yang bisa dilakukan adalah
// menggerbangi permintaan berikutnya dengan pemakaian yang sudah tercatat, dan itu berarti
// satu permintaan terakhir selalu bisa melewati batas sedikit. Tidak ada pembatas token yang
// bisa menghindarinya tanpa menebak jumlah token di muka.
func (g *Guard) GerbangToken(ctx context.Context, p *apikey.Principal, targets []policy.Target) *Rejection {
	scopes := g.cakupanToken(ctx, p, targets)
	if len(scopes) == 0 {
		return nil
	}
	d := g.limiter.TokensExceededScopes(ctx, scopes)
	if d.Allowed {
		return nil
	}
	g.logger.InfoContext(ctx, "permintaan ditolak karena anggaran token habis",
		"api_key", p.Masked(), "cakupan", d.Scope, "batas", d.Limit, "nilai", d.LimitValue)
	return rejectionBatas(d, "anggaran token untuk periode ini sudah habis")
}

// CatatToken mencatat pemakaian token setelah jawaban selesai.
//
// Tidak menolak apa pun dan kegagalannya diabaikan selain dicatat: pada titik ini
// permintaannya sudah dilayani dan biayanya sudah dikeluarkan ke upstream. Yang hilang bila
// gagal adalah ketepatan penghitung yang menggerbangi permintaan berikutnya.
func (g *Guard) CatatToken(ctx context.Context, p *apikey.Principal, targets []policy.Target, tokens int64) {
	if tokens <= 0 {
		return
	}
	scopes := g.cakupanToken(ctx, p, targets)
	if len(scopes) == 0 {
		return
	}
	if d := g.limiter.RecordTokensScopes(ctx, scopes, tokens); d.Degraded {
		g.logger.DebugContext(ctx, "pencatatan token berjalan tanpa Redis", "tokens", tokens)
	}
}

// cakupanToken menyusun cakupan batas token, menggabungkan kolom di api_keys dengan tabel.
func (g *Guard) cakupanToken(ctx context.Context, p *apikey.Principal, targets []policy.Target) []apikey.ScopeTokenLimits {
	if g == nil || g.limiter == nil {
		return nil
	}
	if g.rates == nil {
		return g.limiter.TokenScopes(ctx, p)
	}
	return g.rates.Tokens(ctx, targets, p.TokenLimits())
}

// rejectionBatas menyusun penolakan 429 dari satu keputusan pembatas laju.
func rejectionBatas(d apikey.Decision, pesan string) *Rejection {
	return &Rejection{
		Status: http.StatusTooManyRequests, Type: httpx.ErrTypeRateLimit, Code: CodeRateLimited,
		Message:    pesan,
		RetryAfter: d.RetryAfter,
		// Header kuota dipasang juga pada respons yang DITOLAK: itu bagian kontrak klien
		// OpenAI-compatible, dan tanpanya klien tidak punya cara tahu kapan kuotanya pulih
		// selain mencoba lagi.
		Headers: d.SetHeaders,
	}
}

// kutipKebijakan membungkus nama kebijakan atau model untuk pesan error.
//
// Nilai yang dikutip di sini adalah nama yang ditetapkan OPERATOR (nama anggaran, nama
// kebijakan, pengenal model kanonik), bukan isi permintaan pengguna.
func kutipKebijakan(v string) string {
	const maks = 80
	if len(v) > maks {
		v = v[:maks] + "…"
	}
	return "\"" + v + "\""
}
