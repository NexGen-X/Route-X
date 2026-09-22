// Package gateway menjalankan satu permintaan inference terhadap daftar kandidat
// provider: memutus arus ke provider yang sedang rusak, mengulang yang aman diulang,
// dan berpindah ke kandidat berikutnya ketika yang sekarang tidak menolong.
//
// Urutan kandidatnya bukan urusan paket ini — itu milik internal/router. Yang dikerjakan
// di sini adalah apa yang terjadi SETELAH urutan itu ada, dan seluruhnya digerakkan oleh
// klasifikasi error di internal/providers. Dua sifat yang dipegang paling ketat:
//
//  1. Aliran yang sudah mulai terkirim TIDAK PERNAH diulang maupun dialihkan. Begitu satu
//     byte respons sampai ke klien, percobaan kedua akan menyambung aliran baru di tengah
//     aliran pertama, dan klien menerima jawaban yang tidak koheren — lebih buruk daripada
//     kegagalan yang jujur. providers.Error.StreamedBytes yang menandainya.
//
//  2. Kegagalan yang tidak dikenal tidak diulang. Error tanpa klasifikasi bisa berarti
//     permintaannya sebenarnya sampai dan sudah menimbulkan efek di sisi provider;
//     mengulangnya berarti menagih pengguna dua kali untuk satu permintaan.
package gateway

import (
	"context"
	"errors"
	"log/slog"
	"math/rand/v2"
	"net"
	"time"

	"github.com/NexGen-X/Route-X/internal/database/repo/upstream"
	"github.com/NexGen-X/Route-X/internal/providers"
	"github.com/NexGen-X/Route-X/internal/router"
)

// Batas yang tidak boleh diambil dari konfigurasi karena bukan preferensi, melainkan
// pengaman.
const (
	// maxBackoff membatasi satu jeda backoff. routing_rules.backoff_ms sendiri boleh
	// sampai 60 detik, dan dengan backoff eksponensial percobaan keempat sudah melewati
	// batas kesabaran klien mana pun.
	maxBackoff = 10 * time.Second

	// maxRetryAfter membatasi seberapa lama header Retry-After upstream boleh dipatuhi.
	//
	// Ini bukan kehati-hatian berlebihan: provider yang membatasi laju biasa mengirim
	// "Retry-After: 3600", dan mematuhinya apa adanya berarti satu permintaan klien
	// menggantung satu jam. Bila jedanya melewati batas ini, kandidatnya DILEWATI dan
	// permintaan dialihkan ke provider berikutnya — persis guna failover.
	maxRetryAfter = 5 * time.Second

	// defaultAttemptTimeout dipakai bila kandidat tidak menyebut timeout sendiri.
	defaultAttemptTimeout = 60 * time.Second
)

// CircuitGuard adalah penjaga arus per (provider, model), dipenuhi *Breaker.
//
// Antarmuka, bukan tipe konkret, karena executor harus bisa diuji tanpa Redis: seluruh
// keputusan menarik di paket ini soal URUTAN percobaan, dan menguji itu lewat Redis
// sungguhan berarti menguji dua hal sekaligus lalu tidak tahu mana yang salah ketika
// merah. Ia juga boleh nil, dan itulah yang membuat jalur permintaan bisa dinyalakan di
// lingkungan pengembangan sebelum Redis ada.
type CircuitGuard interface {
	// Allow melaporkan apakah percobaan ke (providerID, model) boleh jalan.
	//
	// Error dari implementasi TIDAK boleh memblokir permintaan; pemanggil di paket ini
	// memperlakukannya sebagai izin. Alasannya sama seperti pembatas laju login: penjaga
	// yang gagal-tertutup membuat Redis mati mematikan seluruh gateway.
	Allow(ctx context.Context, providerID, model string) (izin bool, state State, err error)

	// Record mencatat hasil satu percobaan.
	Record(ctx context.Context, providerID, model string, sukses bool) error
}

// Breaker wajib memenuhi kontrak yang dipakai executor.
var _ CircuitGuard = (*Breaker)(nil)

// Plan adalah rencana eksekusi satu permintaan: kandidat berurut beserta kesabaran yang
// berlaku untuknya.
//
// Dipisah dari Executor karena isinya berubah setiap permintaan sementara Executor hidup
// selama proses. Menaruh keduanya di satu struct berarti Executor tidak bisa dipakai
// bersamaan oleh banyak request tanpa lock.
type Plan struct {
	// Candidates sudah terurut oleh router.Selector. Kandidat pertama dicoba lebih dulu.
	Candidates []*upstream.RouteCandidate

	// Model adalah nama model kanonik. Dipakai sebagai bagian kunci pemutus arus, bukan
	// nama model di sisi upstream: satu model kanonik bisa dipetakan ke nama berbeda di
	// setiap provider, dan pemutus arus yang memakai nama upstream tidak bisa dibaca
	// dashboard sebagai satu kesatuan.
	Model string

	// MaxAttempts adalah jumlah percobaan per kandidat, termasuk yang pertama. Nilai < 1
	// diperlakukan sebagai 1.
	MaxAttempts int

	// Budget adalah anggaran TOTAL percobaan lintas seluruh kandidat, bukan per kandidat.
	// Nol berarti tanpa anggaran: tiap kandidat memakai MaxAttempts penuhnya sendiri.
	//
	// Hanya diisi untuk combo pipeline multi-model, dan bedanya bukan gaya. Pada rule
	// biasa, satu kandidat yang sibuk boleh menghabiskan seluruh kesabarannya sendirian —
	// itulah arti "max_attempts per provider". Pada combo pipeline, resepnya adalah SATU
	// janji ke klien: "paling banyak N percobaan, di model mana pun yang sanggup".
	// Memberi tiap kandidat jatah N sendiri membuat satu permintaan klien menjadi N×M
	// request upstream: pada model berbayar itu biaya yang tidak pernah diminta operator
	// maupun pengguna, dan pada model yang dibatasi laju itu hukuman yang dikirim ke
	// provider yang sebenarnya sehat.
	Budget int

	// BackoffBase adalah jeda dasar backoff eksponensial. Nol berarti tanpa jeda.
	BackoffBase time.Duration
}

// PlanFromRule menyusun Plan dari aturan routing dan kandidat yang sudah terurut.
//
// rule boleh nil (tidak ada aturan yang cocok), dan bawaannya diambil dari nilai default
// kolom routing_rules di migrasi 0008 supaya perilaku "tanpa aturan" sama dengan perilaku
// "aturan bawaan" — kalau berbeda, operator yang membuat aturan pertamanya akan melihat
// karakter retry berubah tanpa pernah memintanya.
func PlanFromRule(rule *router.Rule, model string, cands []*upstream.RouteCandidate) Plan {
	p := Plan{Candidates: cands, Model: model, MaxAttempts: 3, BackoffBase: 250 * time.Millisecond}
	if rule != nil {
		if rule.MaxAttempts > 0 {
			p.MaxAttempts = rule.MaxAttempts
		}
		// Combo pipeline: attempts resep adalah anggaran TOTAL, bukan per kandidat.
		// Lihat catatan Plan.Budget kenapa nilainya tidak boleh dibaca sebagai
		// MaxAttempts. Syarat Combo() — lebih dari satu model — diperiksa di sini
		// supaya resep dengan satu model tetap berperilaku seperti rule biasa; resep
		// seperti itu tidak punya cascade, dan memotong anggarannya hanya mengurangi
		// kesabaran tanpa keuntungan apa pun.
		if rule.Combo() && rule.Pipeline.Attempts > 0 {
			p.Budget = rule.Pipeline.Attempts
		}
		// BackoffBase 0 berarti kolom NULL/nol, bukan permintaan "tanpa jeda":
		// menimpa apa adanya mematikan backoff dan membuat retry menghantam
		// upstream yang baru 429/5xx seketika. Hanya timpa bila > 0.
		if rule.BackoffBase > 0 {
			p.BackoffBase = rule.BackoffBase
		}
	}
	return p
}

// Attempt adalah catatan satu percobaan, sukses maupun gagal.
//
// Seluruh riwayat dikembalikan ke pemanggil, bukan hanya percobaan terakhir, karena
// itulah yang dicatat ke request_events di Fase 9: "permintaan ini berhasil" tanpa jejak
// dua provider yang gagal lebih dulu menyembunyikan tepat informasi yang dibutuhkan
// operator untuk tahu upstream mana yang sedang bermasalah.
type Attempt struct {
	ProviderID      string
	ProviderName    string
	ProviderModelID string
	// UpstreamModel adalah nama model di sisi provider ini.
	UpstreamModel string

	// Nth adalah percobaan ke-berapa pada kandidat ini, mulai dari 1.
	Nth int
	// Started dan Duration hanya mencakup percobaan ini, tanpa jeda backoff sebelumnya.
	Started  time.Time
	Duration time.Duration

	// BreakerState adalah state pemutus arus saat percobaan diputuskan.
	BreakerState State

	// Err nil berarti percobaan ini berhasil.
	Err *providers.Error

	// SkipReason terisi bila kandidat dilewati tanpa pernah dihubungi. Dicatat sebagai
	// percobaan, bukan diabaikan, supaya "kenapa provider ini tidak dipakai" bisa dijawab
	// dari log tanpa menebak.
	SkipReason string
}

// Berhasil melaporkan apakah percobaan ini berhasil.
func (a Attempt) Berhasil() bool { return a.Err == nil && a.SkipReason == "" }

// Outcome adalah hasil eksekusi beserta jejaknya.
type Outcome[T any] struct {
	// Value hanya bermakna bila Err nil.
	Value T
	// Candidate adalah kandidat yang berhasil, nil bila semuanya gagal.
	Candidate *upstream.RouteCandidate
	// Attempts adalah seluruh percobaan dalam urutan terjadinya.
	Attempts []Attempt
	// Err adalah kegagalan terakhir yang bermakna, nil bila berhasil.
	Err *providers.Error
}

// Percobaan mengembalikan jumlah percobaan yang benar-benar menghubungi upstream.
func (o *Outcome[T]) Percobaan() int {
	var n int
	for _, a := range o.Attempts {
		if a.SkipReason == "" {
			n++
		}
	}
	return n
}

// TanpaKandidat melaporkan bahwa tidak satu pun upstream sempat dihubungi.
//
// Ini yang membedakan dua kegagalan yang jawaban HTTP-nya berbeda: "tidak ada provider
// yang cocok" adalah kesalahan permintaan atau konfigurasi (dan jejaknya menjelaskan mana
// dari keduanya lewat SkipReason), sementara "semua provider dicoba dan gagal" adalah
// gangguan hulu yang pantas dijawab 502 atau 503. Menyamakan keduanya membuat operator
// mencari gangguan yang tidak ada, atau sebaliknya menganggap gangguan sebagai salah
// konfigurasi.
func (o *Outcome[T]) TanpaKandidat() bool { return o.Percobaan() == 0 }

// Executor menjalankan permintaan terhadap rencana eksekusi.
//
// Hidup selama proses dan aman dipakai bersamaan: tidak ada keadaan per-permintaan yang
// disimpan di sini — semuanya ada di Plan dan di parameter Execute.
type Executor struct {
	guard  CircuitGuard
	logger *slog.Logger

	// acakJitter dipisah supaya test bisa membuat backoff deterministik.
	acakJitter func(n int64) int64
	// tidur dipisah supaya test tidak perlu benar-benar menunggu.
	tidur func(ctx context.Context, d time.Duration) error
}

// ExecutorOption menyetel Executor.
type ExecutorOption func(*Executor)

// WithJitterFunc mengganti sumber jitter backoff.
func WithJitterFunc(f func(n int64) int64) ExecutorOption {
	return func(e *Executor) {
		if f != nil {
			e.acakJitter = f
		}
	}
}

// WithSleepFunc mengganti cara menunggu antar percobaan.
//
// Ada untuk test: menguji urutan backoff dengan tidur sungguhan berarti setiap test
// membayar jedanya, dan jeda yang dipercepat "sedikit saja" untuk test membuat yang diuji
// bukan lagi angka yang dipakai produksi.
func WithSleepFunc(f func(ctx context.Context, d time.Duration) error) ExecutorOption {
	return func(e *Executor) {
		if f != nil {
			e.tidur = f
		}
	}
}

// NewExecutor membuat Executor.
//
// guard boleh nil, yang berarti tanpa pemutus arus — retry dan failover tetap berjalan.
// Ini bukan kelonggaran untuk produksi, melainkan supaya jalur permintaan bisa dinyalakan
// sebelum Redis tersedia di lingkungan pengembangan.
func NewExecutor(guard CircuitGuard, logger *slog.Logger, opts ...ExecutorOption) *Executor {
	if logger == nil {
		logger = slog.New(slog.DiscardHandler)
	}
	e := &Executor{
		guard:      guard,
		logger:     logger,
		acakJitter: rand.Int64N,
		tidur:      tidurContext,
	}
	for _, o := range opts {
		o(e)
	}
	return e
}

// tidurContext menunggu d, atau berhenti lebih awal bila context dibatalkan.
func tidurContext(ctx context.Context, d time.Duration) error {
	if d <= 0 {
		return ctx.Err()
	}
	t := time.NewTimer(d)
	defer t.Stop()
	select {
	case <-ctx.Done():
		return ctx.Err()
	case <-t.C:
		return nil
	}
}

// Execute menjalankan op terhadap kandidat berurut sampai satu berhasil atau semuanya
// habis.
//
// Selalu mengembalikan Outcome, termasuk saat gagal — jejak percobaannya justru paling
// dibutuhkan ketika permintaan gagal, dan mengembalikannya lewat error akan membuat
// pemanggil harus membongkar tipe error hanya untuk mencatat log.
//
// op dipanggil dengan context yang SUDAH bertenggat percobaan. Ia tidak boleh memasang
// tenggatnya sendiri, dan tidak boleh menyimpan context itu melewati kembaliannya —
// kecuali untuk streaming, yang memang harus hidup lebih lama: lihat catatan di
// ExecuteStream.
//
// Generic, dan karena itu fungsi bebas alih-alih method: Go tidak mengizinkan method
// bergenerik, sementara ketiga operasi provider (completion, stream, embeddings)
// mengembalikan tipe berbeda dan menuntut logika percobaan yang identik. Menyalin logika
// ini tiga kali adalah cara paling pasti membuat aturan StreamedBytes benar di satu tempat
// dan salah di dua tempat lain.
func Execute[T any](
	ctx context.Context,
	e *Executor,
	plan Plan,
	op func(ctx context.Context, c *upstream.RouteCandidate) (T, error),
) *Outcome[T] {
	return jalankanRencana(ctx, e, plan, denganTenggat(op))
}

// jalankanRencana adalah loop kandidat yang dipakai bersama Execute dan ExecuteStream.
func jalankanRencana[T any](ctx context.Context, e *Executor, plan Plan, jalankan pelaksana[T]) *Outcome[T] {
	out := &Outcome[T]{}

	if len(plan.Candidates) == 0 {
		out.Err = providers.Newf(providers.ErrKindModelNotFound, "",
			"tidak ada provider yang bisa melayani model %q", plan.Model)
		return out
	}

	maksPercobaan := max(plan.MaxAttempts, 1)

	// sisa adalah sisa anggaran combo pipeline. Dihitung di sini, di luar loop, karena
	// anggaran dipakai BERSAMA oleh seluruh kandidat: percobaan pada kandidat pertama
	// mengurangi jatah kandidat kedua. Nol berarti tanpa anggaran (rule biasa), dan tidak
	// ada satu pun cabang di bawah yang boleh memperlakukan nol sebagai "habis".
	sisa := plan.Budget

	for _, c := range plan.Candidates {
		// Context yang sudah selesai diperiksa sebelum kandidat berikutnya disentuh:
		// tanpa ini, klien yang sudah pergi tetap membuat gateway menghubungi seluruh
		// sisa daftar provider satu per satu.
		if err := ctx.Err(); err != nil {
			out.Err = providers.FromTransport(c.ProviderName, err)
			return out
		}

		// Jatah kandidat ini. Tanpa anggaran, tiap kandidat memakai MaxAttempts penuh.
		// Dengan anggaran, jatahnya adalah yang tersisa, dan habisnya anggaran adalah
		// alasan berhenti mencoba kandidat berikutnya — bukan kegagalan, melainkan janji
		// resep combo yang sudah dipenuhi.
		jatah := maksPercobaan
		if plan.Budget > 0 {
			if sisa <= 0 {
				break
			}
			jatah = min(maksPercobaan, sisa)
		}

		izin, state, gerr := e.izinkan(ctx, c, plan.Model)
		if gerr != nil {
			// Penjaga yang error tidak memblokir: lihat catatan di CircuitGuard.
			e.logger.WarnContext(ctx, "pemutus arus tidak bisa dibaca, permintaan diloloskan",
				"provider", c.ProviderName, "model", plan.Model, "error", gerr)
		}
		if !izin {
			out.Attempts = append(out.Attempts, Attempt{
				ProviderID: c.ProviderID, ProviderName: c.ProviderName,
				ProviderModelID: c.ProviderModelID, UpstreamModel: c.UpstreamModelName,
				BreakerState: state,
				SkipReason:   "pemutus arus terbuka",
			})
			continue
		}

		hasil, att, ok := cobaKandidat(ctx, e, plan, c, state, jatah, jalankan)
		out.Attempts = append(out.Attempts, att...)
		// Anggaran hanya berkurang oleh percobaan yang benar-benar menghubungi upstream,
		// sama seperti Percobaan() menghitungnya. Kandidat yang dilewati pemutus arus di
		// atas tidak memakai anggaran: ia tidak pernah menagih upstream apa pun.
		if plan.Budget > 0 {
			sisa -= len(att)
		}
		if ok {
			out.Value = hasil
			out.Candidate = c
			out.Err = nil
			return out
		}

		// Kegagalan terakhir kandidat ini menjadi kegagalan yang dilaporkan, sampai
		// kandidat berikutnya menimpanya. Yang dilaporkan ke klien adalah kegagalan
		// TERAKHIR, bukan yang pertama: yang pertama sudah tidak menggambarkan keadaan
		// setelah gateway mencoba tempat lain.
		last := att[len(att)-1]
		out.Err = last.Err

		// Aliran yang sudah terkirim sebagian mengakhiri segalanya — tidak boleh diulang
		// dan tidak boleh dialihkan.
		if last.Err != nil && last.Err.StreamedBytes > 0 {
			return out
		}
		if last.Err != nil && !last.Err.Failoverable() {
			return out
		}
	}
	return out
}

// izinkan menanyakan pemutus arus, dan meloloskan permintaan bila penjaganya sendiri
// bermasalah.
func (e *Executor) izinkan(ctx context.Context, c *upstream.RouteCandidate, model string) (bool, State, error) {
	if e.guard == nil {
		return true, "", nil
	}
	izin, state, err := e.guard.Allow(ctx, c.ProviderID, model)
	if err != nil {
		return true, state, err
	}
	return izin, state, nil
}

// cobaKandidat mengulang satu kandidat sampai berhasil, kehabisan percobaan, atau menemui
// kegagalan yang tidak boleh diulang.
//
// Fungsi bebas, bukan method, karena Go tidak mengizinkan method bergenerik.
//
// Selalu mengembalikan minimal satu Attempt, sehingga pemanggil boleh membaca elemen
// terakhirnya tanpa memeriksa panjang.
func cobaKandidat[T any](
	ctx context.Context,
	e *Executor,
	plan Plan,
	c *upstream.RouteCandidate,
	state State,
	maksPercobaan int,
	jalankan pelaksana[T],
) (T, []Attempt, bool) {
	var kosong T
	att := make([]Attempt, 0, maksPercobaan)

	for n := 1; n <= maksPercobaan; n++ {
		mulai := time.Now()
		nilai, perr := jalankan(ctx, c)
		a := Attempt{
			ProviderID: c.ProviderID, ProviderName: c.ProviderName,
			ProviderModelID: c.ProviderModelID, UpstreamModel: c.UpstreamModelName,
			Nth: n, Started: mulai, Duration: time.Since(mulai),
			BreakerState: state,
		}

		if perr == nil {
			att = append(att, a)
			e.catat(ctx, c, plan.Model, true)
			return nilai, att, true
		}

		a.Err = perr
		att = append(att, a)

		// Pembatalan oleh klien bukan kegagalan provider dan tidak boleh menghitung ke
		// pemutus arus: kalau dicatat, satu klien yang menutup koneksi berkali-kali bisa
		// memutus arus ke provider yang sebenarnya sehat.
		if a.Err.Kind != providers.ErrKindCanceled {
			e.catat(ctx, c, plan.Model, false)
		}

		if !a.Err.Retryable() || n == maksPercobaan {
			return kosong, att, false
		}

		jeda, boleh := e.jedaSebelum(ctx, plan.BackoffBase, n+1, a.Err)
		if !boleh {
			return kosong, att, false
		}
		if err := e.tidur(ctx, jeda); err != nil {
			// Context selesai saat menunggu: percobaan berikutnya tidak akan pernah
			// sempat, dan mencatatnya sebagai percobaan yang gagal akan mengarang
			// kegagalan provider yang tidak pernah dihubungi.
			return kosong, att, false
		}
	}
	return kosong, att, false
}

// pelaksana adalah satu percobaan yang sudah mengurus umur context-nya sendiri dan
// mengembalikan kegagalan yang sudah terklasifikasi.
//
// Lapisan ini ada supaya jalur streaming dan non-streaming memakai SATU loop percobaan.
// Bedanya keduanya hanya soal umur context — non-streaming mematikannya begitu op
// kembali, streaming harus membiarkannya hidup selama aliran dibaca — dan itu perbedaan
// yang bisa dibungkus. Menyalin loop percobaannya adalah cara paling pasti membuat aturan
// StreamedBytes benar di satu jalur dan salah di jalur lain.
type pelaksana[T any] func(ctx context.Context, c *upstream.RouteCandidate) (T, *providers.Error)

// denganTenggat membungkus operasi non-streaming dengan tenggat per percobaan.
//
// Tenggatnya berlapis di atas tenggat permintaan, tidak menggantinya: yang mana pun lebih
// dulu habis, itulah yang berlaku. Ini yang membuat satu provider lambat tidak bisa
// menghabiskan seluruh anggaran waktu permintaan sendirian, sekaligus membuat klien yang
// pergi tetap menghentikan percobaan yang sedang berjalan.
func denganTenggat[T any](op func(ctx context.Context, c *upstream.RouteCandidate) (T, error)) pelaksana[T] {
	return func(ctx context.Context, c *upstream.RouteCandidate) (T, *providers.Error) {
		cctx, cancel := context.WithTimeout(ctx, tenggatPercobaan(c))
		defer cancel()

		nilai, err := op(cctx, c)
		if err != nil {
			return nilai, koreksiTenggat(ctx, cctx, jadikanProviderError(c.ProviderName, err))
		}
		return nilai, nil
	}
}

// tenggatPercobaan mengembalikan batas satu percobaan pada kandidat ini.
func tenggatPercobaan(c *upstream.RouteCandidate) time.Duration {
	if c.TimeoutMS > 0 {
		return time.Duration(c.TimeoutMS) * time.Millisecond
	}
	return defaultAttemptTimeout
}

// koreksiTenggat membedakan "tenggat PERCOBAAN habis" dari "klien membatalkan".
//
// Perlu dikoreksi karena keduanya sampai ke pemanggil sebagai context yang selesai, dan
// klasifikasinya berlawanan akibatnya: tenggat percobaan aman diulang dan dialihkan,
// pembatalan klien tidak boleh diulang ke mana pun. Tanpa koreksi ini, provider yang
// lambat akan tampak seperti klien yang pergi, dan failover tidak pernah berjalan untuk
// kegagalan paling umum yang dimiliki gateway.
func koreksiTenggat(indukCtx, cobaCtx context.Context, perr *providers.Error) *providers.Error {
	if perr == nil {
		return nil
	}
	// Induk masih hidup tetapi context percobaan sudah selesai: yang habis adalah tenggat
	// percobaan, bukan kesabaran klien.
	if indukCtx.Err() == nil && cobaCtx.Err() != nil && perr.Kind == providers.ErrKindCanceled {
		perr.Kind = providers.ErrKindTimeout
		perr.Message = "upstream tidak menjawab sebelum batas waktu percobaan"
	}
	return perr
}

// jadikanProviderError memastikan setiap kegagalan punya klasifikasi.
//
// Kegagalan yang datang tanpa *providers.Error berarti ada adapter yang mengembalikan
// error mentah. Ia diklasifikasikan lewat ClassifyTransport, dan bila itu pun tidak
// mengenalinya, hasilnya ErrKindUnknown — yang TIDAK diulang dan TIDAK dialihkan. Itu
// pilihan yang sadar: error tanpa klasifikasi bisa berarti permintaannya sebenarnya sampai
// dan sudah menimbulkan efek di sisi provider, dan mengulangnya berarti menagih pengguna
// dua kali untuk satu permintaan.
func jadikanProviderError(provider string, err error) *providers.Error {
	if perr := providers.AsError(err); perr != nil {
		if perr.Provider == "" {
			perr.Provider = provider
		}
		return perr
	}
	// ClassifyTransport hanya dipercaya untuk error yang BISA DIBUKTIKAN berasal dari
	// transport. Jaring pengamannya mengembalikan ErrKindNetwork untuk apa pun yang tidak
	// dikenalinya, dan network adalah kategori yang aman diulang MAUPUN dialihkan — jadi
	// mempercayainya di sini akan membuat setiap error mentah dari adapter diulang tiga
	// kali di setiap provider. Itu kebalikan dari yang dijanjikan doc paket ini.
	var netErr net.Error
	if errors.Is(err, context.Canceled) || errors.Is(err, context.DeadlineExceeded) ||
		errors.As(err, &netErr) {
		return providers.FromTransport(provider, err)
	}
	// Pesan aslinya tidak diteruskan: error dari adapter yang tak terklasifikasi bisa
	// memuat potongan permintaan atau URL berkredensial.
	return providers.Newf(providers.ErrKindUnknown, provider, "kegagalan upstream yang tidak terklasifikasi")
}

// catat melaporkan hasil satu percobaan ke pemutus arus.
//
// Kegagalan mencatat hanya di-log, tidak dikembalikan: pada titik ini permintaan sudah
// punya jawaban, dan menggagalkannya karena Redis tidak bisa dicatat berarti menukar
// permintaan yang berhasil dengan error demi kerapian metrik.
func (e *Executor) catat(ctx context.Context, c *upstream.RouteCandidate, model string, sukses bool) {
	if e.guard == nil {
		return
	}
	if err := e.guard.Record(ctx, c.ProviderID, model, sukses); err != nil {
		e.logger.WarnContext(ctx, "hasil percobaan gagal dicatat ke pemutus arus",
			"provider", c.ProviderName, "model", model, "sukses", sukses, "error", err)
	}
}

// jedaSebelum menghitung jeda sebelum percobaan ke-n pada kandidat yang sama.
//
// boleh false berarti percobaan berikutnya sebaiknya TIDAK dilakukan pada kandidat ini:
// entah karena upstream meminta menunggu lebih lama daripada yang pantas ditunggu satu
// permintaan, atau karena waktu yang tersisa pada permintaan ini tidak cukup.
func (e *Executor) jedaSebelum(ctx context.Context, base time.Duration, n int, perr *providers.Error) (time.Duration, bool) {
	jeda := e.backoff(base, n)

	// Retry-After dari upstream menang atas backoff hitungan sendiri: upstream tahu kapan
	// kuotanya kembali, kita hanya menduga. Tetapi tidak dipatuhi tanpa batas — provider
	// yang membatasi laju biasa mengirim jeda dalam hitungan menit, dan satu permintaan
	// klien tidak boleh menggantung selama itu. Lewat batas, kandidatnya dilewati dan
	// permintaan dialihkan ke provider berikutnya; itu persis guna failover.
	if perr != nil && perr.RetryAfter > 0 {
		if perr.RetryAfter > maxRetryAfter {
			return 0, false
		}
		jeda = perr.RetryAfter
	}

	// Menunggu melewati tenggat permintaan berarti menunggu untuk percobaan yang tidak
	// akan pernah sempat berjalan. Lebih baik langsung mencoba kandidat berikutnya.
	if tenggat, ada := ctx.Deadline(); ada && time.Until(tenggat) <= jeda {
		return 0, false
	}
	return jeda, true
}

// backoff menghitung jeda eksponensial berjitter sebelum percobaan ke-n.
//
// Jitternya "equal jitter": separuh jeda dijamin, separuhnya diundi. Dua sifat itu
// keduanya diperlukan.
//
// Bagian yang dijamin ada karena jitter penuh (mengundi dari nol) boleh menghasilkan jeda
// nol, dan mengulang seketika ke provider yang baru saja mengirim 429 hanya menghasilkan
// 429 kedua.
//
// Bagian yang diundi ada karena tanpa jitter, seluruh permintaan yang terkena satu
// pembatasan laju yang sama akan mengulang pada milidetik yang sama — dan provider
// membatasi mereka semua lagi, serentak. Gelombang itu tidak mereda sendiri; ia justru
// menguat setiap putaran.
func (e *Executor) backoff(base time.Duration, n int) time.Duration {
	if base <= 0 || n < 2 {
		return 0
	}
	// Pergeseran dibatasi supaya base yang besar dengan n yang besar tidak meluap menjadi
	// durasi negatif — dan durasi negatif membuat time.NewTimer memicu seketika, yaitu
	// backoff yang justru hilang tepat ketika paling dibutuhkan.
	geser := min(n-2, 20)
	d := base << geser
	if d <= 0 || d > maxBackoff {
		d = maxBackoff
	}
	separuh := int64(d / 2)
	if separuh <= 0 {
		return d
	}
	return time.Duration(separuh + e.acakJitter(separuh))
}

// ExecuteStream menjalankan operasi streaming terhadap kandidat berurut.
//
// Dipisah dari Execute karena umur context-nya berlawanan. Pada operasi biasa, context
// percobaan harus MATI begitu op kembali. Pada streaming, ia harus HIDUP justru setelah op
// kembali — aliran yang dikembalikan masih akan dibaca, dan context yang mati membuat
// pembacaan pertama langsung gagal. Memakai Execute untuk streaming menghasilkan aliran
// yang selalu terputus tepat sebelum byte pertama, kegagalan yang tampak seperti masalah
// upstream.
//
// Yang tetap dibatasi adalah tahap MEMBUKA aliran, dengan tenggat percobaan yang sama.
// Batas itu diperlukan karena klien streaming milik internal/providers sengaja tidak
// memasang http.Client.Timeout (batas itu akan mencakup pembacaan body, sehingga setiap
// aliran mati pada detik yang sama), jadi tanpa penjagaan di sini upstream yang menggantung
// sebelum mengirim header akan menahan permintaan sampai tenggat permintaan habis.
//
// Setelah aliran terbuka, kepemilikan context berpindah ke aliran itu: Close pada aliran
// yang dikembalikan ikut melepasnya. Pemanggil WAJIB memanggil Close — dan itu bukan
// kewajiban baru, providers.Stream sudah mensyaratkannya.
func ExecuteStream(
	ctx context.Context,
	e *Executor,
	plan Plan,
	open func(ctx context.Context, c *upstream.RouteCandidate) (providers.Stream, error),
) *Outcome[providers.Stream] {
	return jalankanRencana(ctx, e, plan, denganTenggatPembukaan(open))
}

// denganTenggatPembukaan membatasi tahap membuka aliran tanpa membatasi pembacaannya.
func denganTenggatPembukaan(
	open func(ctx context.Context, c *upstream.RouteCandidate) (providers.Stream, error),
) pelaksana[providers.Stream] {
	return func(ctx context.Context, c *upstream.RouteCandidate) (providers.Stream, *providers.Error) {
		// Pembatalan berpenyebab, bukan WithTimeout: kalau tenggat pembukaan yang habis,
		// penyebabnya harus bisa dibedakan dari klien yang pergi. Keduanya sampai ke klien
		// HTTP sebagai context yang selesai, dan klasifikasinya berlawanan akibatnya —
		// tenggat aman dialihkan ke provider lain, pembatalan klien tidak.
		cctx, batalkan := context.WithCancelCause(ctx)

		pengawas := time.AfterFunc(tenggatPercobaan(c), func() {
			batalkan(context.DeadlineExceeded)
		})

		str, err := open(cctx, c)
		// Pengawas dimatikan lebih dulu, sebelum apa pun diputuskan: kalau tidak, aliran
		// yang berhasil dibuka tepat sebelum tenggat tetap akan dibatalkan beberapa saat
		// kemudian, di tengah pembacaan klien.
		pengawas.Stop()

		if err != nil {
			perr := jadikanProviderError(c.ProviderName, err)
			if ctx.Err() == nil && errors.Is(context.Cause(cctx), context.DeadlineExceeded) {
				perr.Kind = providers.ErrKindTimeout
				perr.Message = "upstream tidak mulai mengirim aliran sebelum batas waktu percobaan"
			}
			batalkan(nil)
			return nil, perr
		}
		if str == nil {
			batalkan(nil)
			return nil, providers.Newf(providers.ErrKindUnknown, c.ProviderName,
				"adapter mengembalikan aliran kosong tanpa error")
		}

		// Sejak titik ini context milik aliran, bukan milik fungsi ini.
		return &streamTerikat{Stream: str, lepas: batalkan}, nil
	}
}

// streamTerikat menautkan umur context pembukaan ke umur aliran.
//
// Tanpa ini ada dua pilihan yang sama-sama salah: mematikan context saat pembukaan selesai
// (aliran mati sebelum byte pertama), atau membiarkannya hidup sampai tenggat permintaan
// (context bocor untuk setiap aliran yang ditutup lebih awal — dan pada gateway, "ditutup
// lebih awal" adalah kejadian yang normal, karena klien memang boleh berhenti membaca).
type streamTerikat struct {
	providers.Stream
	lepas context.CancelCauseFunc
}

// Close menutup aliran lalu melepas context-nya.
//
// Urutannya penting: melepas context lebih dulu membuat Close pada aliran di bawahnya
// membaca context yang sudah dibatalkan, dan sebagian implementasi melaporkannya sebagai
// kegagalan penutupan — kegagalan yang sepenuhnya kita sendiri yang buat.
//
// Aman dipanggil berulang, karena kedua bagiannya aman dipanggil berulang.
func (s *streamTerikat) Close() error {
	err := s.Stream.Close()
	s.lepas(nil)
	return err
}
