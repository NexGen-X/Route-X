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
	"io"
	"log/slog"
	"math/rand/v2"
	"net"
	"sync"
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

	// tenggatDiamAliran adalah batas maksimum aliran yang sudah terbuka boleh
	// membisu di antara dua peristiwa. Aliran terbuka tidak punya tenggat lain:
	// PrepareSSE melepas tenggat baca/tulis server, dan pengawas pembukaan
	// hanya menjaga fase membuka — upstream yang mengirim satu chunk lalu diam
	// memarkir goroutine + koneksi klien + koneksi upstream tanpa batas
	// (audit3 V-GW-14). Nilainya sengaja longgar, dua kali tenggat percobaan
	// bawaan: tujuannya hanya memanen aliran yang benar-benar mati, bukan
	// memutus model yang jeda penalarannya lama.
	tenggatDiamAliran = 120 * time.Second

	// batasTotalRencana adalah anggaran waktu TOTAL satu rencana eksekusi
	// non-streaming (audit3 V-BR-08). Tanpa anggaran ini, waktu terburuk
	// satu permintaan adalah kandidat × percobaan × tenggat percobaan —
	// pada angka bawaan ±9 menit — dan yang menahannya hanya tenggat
	// server di lapisan HTTP. Nilainya menyelaraskan WRITE_TIMEOUT bawaan
	// server (5 menit): tenggat request yang lebih pendek tetap menang
	// karena anggaran ini dipasang sebagai turunan context request.
	// Streaming dikecualikan — aliran yang sah hidup lebih lama daripada
	// anggaran membuka mana pun; fase membukanya dibatasi tenggat per
	// percobaan dan hidupnya dijaga tenggatDiamAliran.
	//
	// Nilai ini WAJIB tetap di bawah billing.HoldTTLBawaan: tahanan
	// reservasi anggaran harus hidup lebih lama dari permintaan terlama
	// yang sah, kalau tidak pagu anggaran terlampaui oleh permintaan
	// paralel yang lambat (B-189). Relasinya dikunci
	// TestBatasRencanaDiBawahTTLTahanB189.
	batasTotalRencana = 5 * time.Minute
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

// CircuitGuardTerkonfigurasi adalah penjaga arus yang menerima konfigurasi
// per-aturan routing (audit F4). Executor memakainya bila Plan membawa
// BreakerCfg dari aturan yang cocok; penjaga yang tidak menerapkannya
// (mis. ganda uji) tetap bekerja lewat CircuitGuard biasa.
type CircuitGuardTerkonfigurasi interface {
	AllowWithConfig(ctx context.Context, providerID, model string, cfg BreakerConfig) (bool, State, error)
	RecordWithConfig(ctx context.Context, providerID, model string, sukses bool, cfg BreakerConfig) error
}

// var _ memastikan *Breaker memenuhi kontrak terkonfigurasi.
var _ CircuitGuardTerkonfigurasi = (*Breaker)(nil)

// CircuitGuardHasilProbe adalah penjaga arus yang bisa membedakan hasil
// probe half_open dari hasil percobaan biasa (audit3 V-BR-03). Executor
// memakainya bila penjaga mendukungnya; penjaga yang tidak mendukungnya
// tetap bekerja lewat Record biasa, dengan konsekuensi hasil basi pada
// episode half_open tidak bisa disaring — itulah sebabnya *Breaker
// menerapkannya.
type CircuitGuardHasilProbe interface {
	// RecordHasil mencatat hasil satu percobaan; probe=true hanya bila
	// percobaan itu diizinkan saat state half_open.
	RecordHasil(ctx context.Context, providerID, model string, sukses, probe bool) error
	// RecordHasilWithConfig adalah RecordHasil dengan konfigurasi
	// per-aturan routing.
	RecordHasilWithConfig(ctx context.Context, providerID, model string, sukses, probe bool, cfg BreakerConfig) error
}

// var _ memastikan *Breaker memenuhi kontrak hasil probe.
var _ CircuitGuardHasilProbe = (*Breaker)(nil)

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

	// BreakerCfg adalah konfigurasi pemutus arus dari aturan routing yang
	// cocok (audit F4). Nilai nol berarti konfigurasi global breaker.
	// Pemetaan dari kolom routing_rules: FailureThreshold (jumlah) menjadi
	// MinSamples; OpenDuration dan HalfOpenProbes nama-sama — mengikuti
	// catatan semantik di breaker.go.
	BreakerCfg BreakerConfig

	// ModelNames memetakan identitas kanonik kandidat (RouteCandidate.
	// ModelID = models.id UUID) ke NAMA model kanoniknya, untuk plan
	// yang mencampur beberapa model (combo pipeline). Kunci pemutus
	// arus memakai nama — konsisten dengan plan satu model yang
	// kuncinya Model — jadi executor menyelesaikan kunci per kandidat
	// lewat peta ini (B-181). Kosong/nil pada plan satu model dan
	// kandidat sintetis: kunci jatuh ke Model (perilaku lama).
	ModelNames map[string]string

	// streaming menandai rencana ExecuteStream: hasil percobaan yang
	// BERHASIL membuka aliran tidak dicatat ke pemutus arus di sini,
	// melainkan oleh pembungkus aliran saat aliran benar-benar selesai
	// (audit F4 — aliran yang mati di tengah bukan keberhasilan).
	streaming bool
}

// modelKandidat menyelesaikan kunci model pemutus arus untuk SATU
// kandidat (B-181). Plan combo mencampur kandidat dari beberapa
// model kanonik, dan memakai Model plan (model jangkar yang diminta
// klien) untuk semuanya membuat kegagalan anggota model B tercatat
// ke breaker model A — mencemari lalu lintas langsung model A dan
// membiarkan model B tanpa riwayat perlindungannya sendiri. Bila
// identitas kandidat dikenal dan plan membawa nama kanoniknya, nama
// itu yang dipakai; selain itu Model plan (perilaku lama, dan satusatunya
// pilihan untuk plan satu model).
func modelKandidat(plan Plan, c *upstream.RouteCandidate) string {
	if c.ModelID != "" {
		if nama, ok := plan.ModelNames[c.ModelID]; ok && nama != "" {
			return nama
		}
	}
	return plan.Model
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
		// Konfigurasi pemutus arus per-aturan (audit F4): kolom-kolom ini
		// sebelumnya tersimpan rapi di DB dan tidak pernah dibaca jalur
		// request sama sekali.
		if rule.FailureThreshold > 0 {
			p.BreakerCfg.MinSamples = rule.FailureThreshold
		}
		if rule.OpenDuration > 0 {
			p.BreakerCfg.OpenDuration = rule.OpenDuration
		}
		if rule.HalfOpenProbes > 0 {
			p.BreakerCfg.HalfOpenProbes = rule.HalfOpenProbes
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
// Status (triage B-005): akses semantik tipe hasil eksekusi. Jalur produksi membaca field Attempt/Outcome langsung; metode ini dipertahankan sebagai API baca yang teruji.
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
// Status (triage B-005): lihat catatan pada Attempt.Berhasil — akses semantik Outcome yang dipertahankan sebagai API baca teruji.
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
// Status (triage B-005): lihat catatan pada Attempt.Berhasil — akses semantik Outcome yang dipertahankan sebagai API baca teruji.
func (o *Outcome[T]) TanpaKandidat() bool { return o.Percobaan() == 0 }

// Executor menjalankan permintaan terhadap rencana eksekusi.
//
// Hidup selama proses dan aman dipakai bersamaan: tidak ada keadaan per-permintaan yang
// disimpan di sini — semuanya ada di Plan dan di parameter Execute.
type Executor struct {
	guard  CircuitGuard
	logger *slog.Logger

	// egressHealth adalah sumber cuplikan kesehatan egress pool untuk
	// penyisihan kandidat (lihat saringKandidatEgress). nil berarti
	// penyisihan tidak berjalan — perilaku sebelum fitur ini ada.
	egressHealth EgressHealthSource

	// slotMu/slotAktif menegakkan kolom providers.max_concurrent secara
	// in-proses (audit F4): Route-X adalah satu biner, jadi penghitung
	// konkurensi per provider cukup di memori proses ini. Slot diambil
	// per percobaan dan dilepas segera setelah percobaan selesai —
	// jeda backoff antar-percobaan tidak menahan slot.
	slotMu    sync.Mutex
	slotAktif map[string]int

	// acakJitter dipisah supaya test bisa membuat backoff deterministik.
	acakJitter func(n int64) int64
	// tidur dipisah supaya test tidak perlu benar-benar menunggu.
	tidur func(ctx context.Context, d time.Duration) error
}

// EgressHealthSource membaca cuplikan kesehatan terakhir satu egress pool.
// Dipenuhi *upstream.EgressRepo lewat Get-nya.
//
// Dipasang lewat WithEgressHealth, bukan disimpulkan dari ketersediaan repo:
// memasangnya MENGUBAH pemilihan kandidat (pool yang tercatat tak sehat
// disisihkan), dan perubahan perilaku seperti itu harus terlihat di tempat
// perakitannya — pola yang sama seperti OAuthRefresher di factory.
type EgressHealthSource interface {
	Get(ctx context.Context, id string) (*upstream.EgressPool, error)
}

// ambangBasiKesehatanEgress adalah umur maksimal cuplikan kesehatan egress
// yang masih boleh dipakai menyisihkan kandidat.
//
// Probe egress berjalan bersama health checker (bawaan 60 detik), jadi
// lima menit berarti lima putaran probe terlewat berturut-turut — keadaan
// yang jauh lebih mungkin berarti pen-probe-nya sendiri yang mati daripada
// pool-nya yang masih sakit. Kesehatan yang lebih tua dari ini TIDAK
// dipakai: menyisihkan kandidat berdasar kabar lama sama buruknya dengan
// tidak memeriksanya sama sekali.
const ambangBasiKesehatanEgress = 5 * time.Minute

// ExecutorOption menyetel Executor.
type ExecutorOption func(*Executor)

// WithJitterFunc mengganti sumber jitter backoff.
// Seam pengujian yang disengaja (triage B-005): produksi memakai nilai bawaan; opsi ini ada agar test dapat menyetel perilaku secara deterministik.
func WithJitterFunc(f func(n int64) int64) ExecutorOption {
	return func(e *Executor) {
		if f != nil {
			e.acakJitter = f
		}
	}
}

// WithEgressHealth memasang penyisihan kandidat berdasar kesehatan egress
// pool: kandidat yang pool-nya tercatat tak sehat (dan catatannya segar)
// dilewati selama masih ada kandidat lain, menutup sisa grup 8 audit3 —
// kesehatan egress sudah diprobe dan dicatat berkala oleh worker, tetapi
// pemilihan rute tidak pernah membacanya. Tanpa opsi ini perilaku executor
// persis seperti sebelumnya.
func WithEgressHealth(src EgressHealthSource) ExecutorOption {
	return func(e *Executor) {
		if src != nil {
			e.egressHealth = src
		}
	}
}

// WithSleepFunc mengganti cara menunggu antar percobaan.
//
// Ada untuk test: menguji urutan backoff dengan tidur sungguhan berarti setiap test
// membayar jedanya, dan jeda yang dipercepat "sedikit saja" untuk test membuat yang diuji
// bukan lagi angka yang dipakai produksi.
// Seam pengujian yang disengaja (triage B-005): produksi memakai nilai bawaan; opsi ini ada agar test dapat menyetel perilaku secara deterministik.
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
		slotAktif:  make(map[string]int),
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

	// Penyisihan egress: kandidat yang pool egress-nya tercatat tak sehat
	// dikeluarkan dari rencana SEBELUM loop berjalan, selama masih ada
	// kandidat lain (lihat saringKandidatEgress untuk aturan fail-open-nya).
	plan.Candidates = e.saringKandidatEgress(ctx, plan.Candidates)

	// Anggaran waktu total rencana (audit3 V-BR-08) — lihat catatan
	// batasTotalRencana. Dipasang sebagai turunan context request, jadi
	// tenggat yang lebih pendek pada request tetap yang berlaku.
	if !plan.streaming {
		var batal context.CancelFunc
		ctx, batal = context.WithTimeout(ctx, batasTotalRencana)
		defer batal()
	}

	maksPercobaan := max(plan.MaxAttempts, 1)

	// sisa adalah sisa anggaran combo pipeline. Dihitung di sini, di luar loop, karena
	// anggaran dipakai BERSAMA oleh seluruh kandidat: percobaan pada kandidat pertama
	// mengurangi jatah kandidat kedua. Nol berarti tanpa anggaran (rule biasa), dan tidak
	// ada satu pun cabang di bawah yang boleh memperlakukan nol sebagai "habis".
	sisa := plan.Budget

	// modelDitolak memuat model kanonik yang sudah menjawab invalid_request
	// pada permintaan INI (B-178). invalid_request bersifat per-model —
	// penerjemah/validator tiap model berbeda (pola B-161) — jadi failover
	// lintas model masuk akal, tetapi kandidat lain dari model yang sama
	// akan menolak permintaan yang sama dan hanya membakar anggaran:
	// mereka dilewati dengan SkipReason, bukan dicoba.
	var modelDitolak map[string]bool

	// errLewatan adalah error lokal terakhir milik percobaan lewatan yang
	// membawa Err (slot konkurensi penuh, B-186). Ia HANYA cadangan:
	// menjadi kegagalan akhir bila tidak ada satu pun kegagalan upstream
	// nyata dari kandidat mana pun, dan tidak pernah menimpa kegagalan
	// nyata yang sudah tercatat.
	var errLewatan *providers.Error

	for idx, c := range plan.Candidates {
		// Context yang sudah selesai diperiksa sebelum kandidat berikutnya disentuh:
		// tanpa ini, klien yang sudah pergi tetap membuat gateway menghubungi seluruh
		// sisa daftar provider satu per satu.
		if err := ctx.Err(); err != nil {
			out.Err = providers.FromTransport(c.ProviderName, err)
			return out
		}

		// Kandidat dari model yang sudah ditolak invalid_request pada
		// permintaan ini dilewati tanpa menghubungi upstream (B-178).
		// Dilewati TETAP dicatat sebagai percobaan lewatan, supaya "kenapa
		// provider ini tidak dipakai" terjawab dari log.
		if c.ModelID != "" && modelDitolak[c.ModelID] {
			out.Attempts = append(out.Attempts, Attempt{
				ProviderID: c.ProviderID, ProviderName: c.ProviderName,
				ProviderModelID: c.ProviderModelID, UpstreamModel: c.UpstreamModelName,
				Nth: 1, Started: time.Now(),
				SkipReason: "model kandidat sudah ditolak invalid_request pada permintaan ini",
			})
			continue
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
		// Kolom providers.max_retries membatasi percobaan per kandidat ini
		// (audit F4): sebelumnya kolom itu tersimpan dan tidak ditegakkan
		// di mana pun — yang berlaku selalu MaxAttempts aturan. max_retries
		// adalah jumlah PENGULANGAN sesudah percobaan pertama.
		if c.MaxRetries >= 0 && c.MaxRetries+1 < jatah {
			jatah = c.MaxRetries + 1
		}

		hasil, att, ok := cobaKandidat(ctx, e, plan, c, jatah, jalankan)
		out.Attempts = append(out.Attempts, att...)
		// Anggaran hanya berkurang oleh percobaan yang benar-benar menghubungi
		// upstream, sama seperti Percobaan() menghitungnya. Percobaan lewatan —
		// kandidat yang disisihkan pemutus arus atau slot penuh — tidak memakai
		// anggaran: ia tidak pernah menagih upstream apa pun (audit3 V-BR-05;
		// versi lama mengurangi sebesar len(att) dan menghabiskan seluruh
		// anggaran combo pada satu kandidat yang bahkan tidak dicoba).
		if plan.Budget > 0 {
			sisa -= hitungPercobaanNyata(att)
		}
		if ok {
			out.Value = hasil
			out.Candidate = c
			out.Err = nil
			return out
		}

		// Kegagalan yang dilaporkan adalah kegagalan NYATA terakhir — dari
		// kandidat mana pun yang benar-benar menghasilkan error — sampai
		// kandidat berikutnya menimpanya. Yang dilaporkan ke klien adalah
		// kegagalan TERAKHIR, bukan yang pertama: yang pertama sudah tidak
		// menggambarkan keadaan setelah gateway mencoba tempat lain.
		// Percobaan lewatan tidak ikut memilih error ini. Lewatan pemutus
		// arus memang ber-Err nil, tetapi lewatan slot-penuh membawa Err
		// lokal (RateLimit, Local): membiarkannya ikut serta membuat error
		// lokal itu menimpa kegagalan upstream nyata kandidat sebelumnya —
		// dan bahkan menggerakkan keputusan failover/berhenti di bawah —
		// padahal kandidat yang dilewati tidak berkata apa-apa tentang
		// kesehatan kandidat sebelumnya (B-186). Error lewatan hanya
		// disimpan sebagai cadangan akhir.
		var errTerakhir *providers.Error
		for i := len(att) - 1; i >= 0; i-- {
			if att[i].Err != nil && att[i].SkipReason == "" {
				errTerakhir = att[i].Err
				break
			}
		}
		if errTerakhir == nil {
			for i := len(att) - 1; i >= 0; i-- {
				if att[i].Err != nil {
					errLewatan = att[i].Err
					break
				}
			}
			continue
		}
		out.Err = errTerakhir

		// Aliran yang sudah terkirim sebagian mengakhiri segalanya — tidak boleh diulang
		// dan tidak boleh dialihkan.
		if errTerakhir.StreamedBytes > 0 {
			return out
		}
		if errTerakhir.Failoverable() {
			continue
		}
		// invalid_request TIDAK failoverable antar-provider untuk model
		// yang sama (permintaan yang sama akan ditolak lagi), tetapi
		// failoverable ANTAR-model pada pipeline multi-model (B-178):
		// penerjemah tiap model berbeda, jadi penolakan model ini bukan
		// bukti model berikutnya menolak juga — persis skenario combo
		// [anggota Google menolak skema, anggota lain menerima]. Syaratnya
		// ketat: model kandidat ini diketahui, dan masih ada kandidat
		// bermodel BERBEDA yang belum ditolak di sisa daftar. Model ini
		// ditandai ditolak; kandidat semodel di sisa daftar dilewati di
		// kepala loop.
		if errTerakhir.Kind == providers.ErrKindInvalidRequest && c.ModelID != "" &&
			adaModelLainTersisa(plan.Candidates[idx+1:], c.ModelID, modelDitolak) {
			if modelDitolak == nil {
				modelDitolak = make(map[string]bool)
			}
			modelDitolak[c.ModelID] = true
			continue
		}
		return out
	}

	// Tidak satu pun kandidat berhasil dan tidak satu pun kegagalan nyata
	// tercatat. Jalan sampai ke sini: semua kandidat dilewati (pemutus arus
	// dan/atau slot penuh). Error lewatan cadangan — bila ada — adalah
	// jawaban yang jujur untuk keadaan itu (kandidat tunggal slot-penuh
	// memang harus menjawab dengan error lokalnya, B-186); tanpa cadangan,
	// satu-satunya lewatan yang mungkin adalah pemutus arus.
	// Outcome tanpa Err dan tanpa Value TIDAK boleh keluar dari sini:
	// pemanggil membacanya sebagai sukses lalu men-dereference Value yang nil —
	// panic 500 persis pada keadaan "semua provider bermasalah", yang seharusnya
	// dijawab 503 (audit3 V-GW-11).
	if out.Err == nil {
		if errLewatan != nil {
			out.Err = errLewatan
		} else {
			out.Err = &providers.Error{
				Kind:    providers.ErrKindOverloaded,
				Message: "semua provider untuk model ini sedang tidak tersedia karena pemutus arusnya terbuka",
			}
		}
	}
	return out
}

// adaModelLainTersisa melaporkan apakah di sisa daftar masih ada kandidat
// bermodel kanonik BERBEDA dari model yang baru ditolak — dan model itu
// sendiri belum ditolak pada permintaan ini (B-178). Kandidat tanpa
// ModelID tidak dihitung: identitasnya tidak diketahui, dan menebak
// berarti mempertaruhkan failover yang tidak terbukti aman.
func adaModelLainTersisa(sisa []*upstream.RouteCandidate, modelDitolakBaru string, modelDitolak map[string]bool) bool {
	for _, c := range sisa {
		if c.ModelID != "" && c.ModelID != modelDitolakBaru && !modelDitolak[c.ModelID] {
			return true
		}
	}
	return false
}

// saringKandidatEgress mengeluarkan kandidat yang egress pool-nya tercatat
// tak sehat dari rencana, selama masih ada kandidat yang tersisa.
//
// Kesehatannya adalah cuplikan yang ditulis probe berkala worker
// (RecordHealth grup 8 audit3): dulu cuplikan itu hanya dibaca dashboard,
// sementara pemilihan rute tetap mengirim lalu lintas ke pool yang sudah
// diketahui mati. Tiga aturan menjaga penyisihan ini tetap jujur:
//
//   - Hanya status "unhealthy" yang SEGAR yang menyisihkan (lihat
//     ambangBasiKesehatanEgress). Status sehat, belum pernah diprobe,
//     gagal dibaca, atau basi → kandidat lolos: ketidakpastian tidak
//     boleh berubah menjadi penolakan.
//   - Kandidat tanpa egress pool (koneksi langsung) selalu lolos dan
//     dihitung sebagai alternatif sehat.
//   - FAIL-OPEN: bila SEMUA kandidat tersisih, daftar dikembalikan utuh
//     dan kejadiannya dicatat sebagai Warn. Pool yang "tak sehat" menurut
//     probe bisa jadi salah vonis (jalur probe berbeda dari jalur
//     provider, target probe sedang bermasalah, dsb.); mengunci total
//     berdasar satu sinyal tidak langsung menukar gangguan sebagian
//     menjadi pemadaman penuh. Pola yang sama seperti izinkan() di
//     pemutus arus: penjaga yang bermasalah meloloskan, bukan menahan.
func (e *Executor) saringKandidatEgress(ctx context.Context, cands []*upstream.RouteCandidate) []*upstream.RouteCandidate {
	if e.egressHealth == nil || len(cands) < 2 {
		return cands
	}

	// Satu pembacaan per pool per permintaan: beberapa kandidat boleh
	// berbagi pool yang sama.
	takSehat := make(map[string]bool)
	poolTakSehat := func(id string) bool {
		if v, ada := takSehat[id]; ada {
			return v
		}
		v := e.poolEgressTakSehat(ctx, id)
		takSehat[id] = v
		return v
	}

	lolos := make([]*upstream.RouteCandidate, 0, len(cands))
	var tersisih []*upstream.RouteCandidate
	for _, c := range cands {
		if c.EgressPoolID == nil || *c.EgressPoolID == "" || !poolTakSehat(*c.EgressPoolID) {
			lolos = append(lolos, c)
			continue
		}
		tersisih = append(tersisih, c)
	}
	if len(tersisih) == 0 {
		return cands
	}
	if len(lolos) == 0 {
		e.logger.WarnContext(ctx, "semua kandidat memakai egress pool yang tercatat tak sehat; fail-open, kandidat tetap dicoba",
			"jumlah_kandidat", len(cands))
		return cands
	}
	for _, c := range tersisih {
		e.logger.InfoContext(ctx, "kandidat disisihkan karena egress pool tercatat tak sehat",
			"provider_id", c.ProviderID, "provider_name", c.ProviderName,
			"egress_pool_id", *c.EgressPoolID)
	}
	return lolos
}

// poolEgressTakSehat melaporkan apakah cuplikan kesehatan terakhir satu
// pool menyatakan tak sehat DAN masih segar. Segala bentuk ketidakpastian
// — pool tak ditemukan, status kosong, waktu periksa kosong, pembacaan
// gagal — dijawab false: yang menyisihkan kandidat hanyalah vonis tak
// sehat yang jelas dan baru.
func (e *Executor) poolEgressTakSehat(ctx context.Context, id string) bool {
	pool, err := e.egressHealth.Get(ctx, id)
	if err != nil {
		e.logger.WarnContext(ctx, "kesehatan egress pool gagal dibaca; kandidat tidak disisihkan",
			"egress_pool_id", id, "error", err)
		return false
	}
	if pool == nil || pool.LastHealthStatus == nil || *pool.LastHealthStatus != "unhealthy" {
		return false
	}
	if pool.LastHealthAt == nil {
		return false
	}
	return time.Since(*pool.LastHealthAt) <= ambangBasiKesehatanEgress
}

// izinkan menanyakan pemutus arus, dan meloloskan permintaan bila penjaganya sendiri
// bermasalah.
func (e *Executor) izinkan(ctx context.Context, c *upstream.RouteCandidate, model string, cfg BreakerConfig) (bool, State, error) {
	if e.guard == nil {
		return true, "", nil
	}
	// Jalur terkonfigurasi hanya bila rencana membawa konfigurasi aturan
	// DAN penjaga mendukungnya; selain itu jalur biasa.
	if cfg != (BreakerConfig{}) {
		if ter, ok := e.guard.(CircuitGuardTerkonfigurasi); ok {
			izin, state, err := ter.AllowWithConfig(ctx, c.ProviderID, model, cfg)
			if err != nil {
				return true, state, err
			}
			return izin, state, nil
		}
	}
	izin, state, err := e.guard.Allow(ctx, c.ProviderID, model)
	if err != nil {
		return true, state, err
	}
	return izin, state, nil
}

// ambilSlot memesan satu slot konkurensi provider. ok=false berarti batas
// max_concurrent provider sedang penuh. Fungsi lepas yang dikembalikan
// aman dipanggil tepat satu kali oleh pemesan.
func (e *Executor) ambilSlot(c *upstream.RouteCandidate) (lepas func(), ok bool) {
	if c.MaxConcurrent == nil || *c.MaxConcurrent <= 0 {
		return func() {}, true
	}
	e.slotMu.Lock()
	defer e.slotMu.Unlock()
	if e.slotAktif == nil {
		e.slotAktif = make(map[string]int)
	}
	if e.slotAktif[c.ProviderID] >= *c.MaxConcurrent {
		return nil, false
	}
	e.slotAktif[c.ProviderID]++
	var sekali sync.Once
	return func() {
		sekali.Do(func() {
			e.slotMu.Lock()
			defer e.slotMu.Unlock()
			if e.slotAktif[c.ProviderID] > 0 {
				e.slotAktif[c.ProviderID]--
			}
		})
	}, true
}

// hitungPercobaanNyata menghitung percobaan dalam satu irisan yang benar-benar
// menghubungi upstream — pasangannya Outcome.Percobaan untuk irisan per kandidat.
func hitungPercobaanNyata(att []Attempt) int {
	var n int
	for _, a := range att {
		if a.SkipReason == "" {
			n++
		}
	}
	return n
}

// kunciStatusProbe menandai context satu percobaan sebagai probe half_open.
//
// Penanda ini yang mengantar status izin ke pencatatan aliran streaming:
// hasil aliran baru dicatat saat aliran berakhir, jauh sesudah percobaan
// yang membukanya selesai, dan pada saat itu hanya context ini yang masih
// tahu apakah pembukaannya adalah sebuah probe (audit3 V-BR-03).
type kunciStatusProbe struct{}

// ctxDenganProbe menanam penanda probe pada context percobaan.
func ctxDenganProbe(ctx context.Context, probe bool) context.Context {
	if !probe {
		return ctx
	}
	return context.WithValue(ctx, kunciStatusProbe{}, true)
}

// probeDariContext membaca penanda probe dari context percobaan.
func probeDariContext(ctx context.Context) bool {
	v, _ := ctx.Value(kunciStatusProbe{}).(bool)
	return v
}

// cobaKandidat mengulang satu kandidat sampai berhasil, kehabisan percobaan, atau menemui
// kegagalan yang tidak boleh diulang.
//
// Fungsi bebas, bukan method, karena Go tidak mengizinkan method bergenerik.
//
// Selalu mengembalikan minimal satu Attempt, sehingga pemanggil boleh membaca elemen
// terakhirnya tanpa memeriksa panjang.
//
// Dua urutan di dalam loop ini adalah perbaikan audit3 dan tidak boleh
// ditukar balik:
//
//   - Slot konkurensi diambil SEBELUM izin pemutus arus diminta (V-BR-07):
//     izin probe half_open adalah kuota langka, dan mengambilnya lebih dulu
//     berarti slot yang penuh menghanguskannya tanpa satu pun kontak ke
//     upstream — breaker lalu macet di half_open walau upstream sudah pulih.
//   - Izin diminta PER PERCOBAAN, bukan sekali per kandidat (V-BR-02): pada
//     half_open satu izin adalah satu probe, dan probe yang gagal membuka
//     lagi breaker; percobaan berikutnya harus bertanya lagi dan akan
//     ditolak, bukan menembak upstream sampai MaxAttempts dari satu izin.
func cobaKandidat[T any](
	ctx context.Context,
	e *Executor,
	plan Plan,
	c *upstream.RouteCandidate,
	maksPercobaan int,
	jalankan pelaksana[T],
) (T, []Attempt, bool) {
	var kosong T
	att := make([]Attempt, 0, maksPercobaan)

	for n := 1; n <= maksPercobaan; n++ {
		// Batas konkurensi provider (audit F4): slot penuh berarti kandidat
		// dilewati TANPA menghubungi upstream dan TANPA catatan kegagalan
		// breaker (penolakan lokal) — kandidat berikutnya dicoba.
		lepas, okSlot := e.ambilSlot(c)
		if !okSlot {
			att = append(att, Attempt{
				ProviderID: c.ProviderID, ProviderName: c.ProviderName,
				ProviderModelID: c.ProviderModelID, UpstreamModel: c.UpstreamModelName,
				Nth: n, Started: time.Now(),
				SkipReason: "batas konkurensi provider penuh",
				Err: &providers.Error{
					Kind:     providers.ErrKindRateLimit,
					Provider: c.ProviderName,
					Message:  "batas konkurensi gateway ke provider ini sedang penuh",
					Local:    true,
				},
			})
			return kosong, att, false
		}

		// Kunci pemutus arus per kandidat (B-181): plan combo memakai
		// model kanonik kandidat ini, bukan model jangkar plan.
		mk := modelKandidat(plan, c)
		izin, state, gerr := e.izinkan(ctx, c, mk, plan.BreakerCfg)
		if gerr != nil {
			// Penjaga yang error tidak memblokir: lihat catatan di CircuitGuard.
			e.logger.WarnContext(ctx, "pemutus arus tidak bisa dibaca, permintaan diloloskan",
				"provider", c.ProviderName, "model", mk, "error", gerr)
		}
		if !izin {
			lepas()
			att = append(att, Attempt{
				ProviderID: c.ProviderID, ProviderName: c.ProviderName,
				ProviderModelID: c.ProviderModelID, UpstreamModel: c.UpstreamModelName,
				Nth: n, Started: time.Now(),
				BreakerState: state,
				SkipReason:   "pemutus arus terbuka",
			})
			return kosong, att, false
		}
		// Percobaan yang diizinkan saat half_open adalah sebuah probe, dan
		// status itu menentukan bagaimana hasilnya boleh ditafsirkan
		// pemutus arus (audit3 V-BR-03).
		probe := state == StateHalfOpen

		mulai := time.Now()
		nilai, perr := jalankan(ctxDenganProbe(ctx, probe), c)
		a := Attempt{
			ProviderID: c.ProviderID, ProviderName: c.ProviderName,
			ProviderModelID: c.ProviderModelID, UpstreamModel: c.UpstreamModelName,
			Nth: n, Started: mulai, Duration: time.Since(mulai),
			BreakerState: state,
		}

		if perr == nil {
			att = append(att, a)
			// Untuk streaming, keberhasilan membuka aliran BUKAN hasil akhir:
			// pencatatan ditunda ke pembungkus aliran di ExecuteStream yang
			// tahu bagaimana aliran itu berakhir (audit F4).
			if !plan.streaming {
				e.catat(ctx, c, mk, true, plan.BreakerCfg, probe)
				lepas()
			} else if aliran, ok := any(nilai).(providers.Stream); ok {
				// Slot dipegang selama aliran HIDUP, bukan sampai ia dibuka
				// (audit3 V-BR-06): kepemilikan slot berpindah ke pembungkus
				// aliran, yang melepasnya saat aliran berakhir.
				nilai = any(&aliranPemegangSlot{Stream: aliran, lepas: lepas}).(T)
			} else {
				lepas()
			}
			return nilai, att, true
		}
		lepas()

		a.Err = perr
		att = append(att, a)

		// Pembatalan oleh klien bukan kegagalan provider dan tidak boleh menghitung ke
		// pemutus arus: kalau dicatat, satu klien yang menutup koneksi berkali-kali bisa
		// memutus arus ke provider yang sebenarnya sehat.
		//
		// Hal yang sama berlaku untuk penolakan LOKAL gateway (Local): batas
		// laju/konkurensi yang diputuskan gateway sendiri sebelum upstream
		// dihubungi bukan bukti kesehatan provider (audit F4).
		//
		// Dan hal yang sama berlaku untuk ErrKindInvalidRequest (turunan B-161):
		// penolakan 400 validasi adalah kesalahan BENTUK permintaan klien — skema
		// tool yang tidak dikenal validator upstream, misalnya — bukan bukti
		// provider sakit. Pada kampanye CLI, rentetan 400 validasi dari satu tool
		// membuka pemutus arus dan tool LAIN ikut menerima 503 tanpa pernah dicoba
		// ke upstream; provider yang sehat tidak boleh diputus oleh permintaan
		// yang memang tidak sah bentuknya.
		//
		// ErrKindContentFilter ikut dikecualikan (B-240): penolakan kebijakan
		// konten upstream juga 4xx yang diputuskan oleh ISI permintaan klien,
		// bukan kesehatan provider — muatan yang sama akan ditolak berulang
		// kali, dan mencatatnya hanya membuka pemutus arus untuk trafik lain
		// yang sah. Ia sengaja TIDAK dibuat failoverable (dokumen jenisnya di
		// providers/errors.go): muatan yang sama tidak dikirim ke banyak
		// pihak. Dua keputusan itu berbeda sumbu — failover menyangkut
		// permintaan ini, pencatatan menyangkut reputasi provider — dan
		// pengecualian di sini hanya menyangkut sumbu kedua.
		//
		// ErrKindModelNotFound melengkapi daftar itu (B-168): model yang
		// tidak dikenal provider ini adalah fakta tentang PERMINTAAN (nama
		// yang diminta klien tidak ada di katalog provider itu), bukan
		// tentang kesehatan provider — provider yang sama melayani model
		// lain dengan sehat. Rentetan permintaan ke model hantu tidak boleh
		// membuka pemutus arus bagi trafik model yang sah. Seperti
		// ContentFilter, ia tetap failoverable: provider LAIN mungkin
		// mengenal modelnya, jadi permintaan ini boleh berpindah — yang
		// tidak boleh adalah kegagalannya menodai reputasi provider ini.
		if a.Err.Kind != providers.ErrKindCanceled && a.Err.Kind != providers.ErrKindInvalidRequest && a.Err.Kind != providers.ErrKindContentFilter && a.Err.Kind != providers.ErrKindModelNotFound && !a.Err.Local {
			e.catat(ctx, c, mk, false, plan.BreakerCfg, probe)
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

// aliranPemegangSlot memegang slot konkurensi provider selama aliran
// streaming hidup (audit3 V-BR-06).
//
// Sebelum pembungkus ini ada, slot dilepas tepat sesudah pembuka aliran
// kembali — yaitu saat aliran DIBUKA — sehingga max_concurrent hanya
// mengatur laju pembukaan: provider ber-max_concurrent 1 terbukti melayani
// 3 aliran 24 detik bersamaan. Slot kini dilepas pada akhir aliran yang
// sesungguhnya: peristiwa terminal dari Recv (EOF maupun error) atau
// Close, mana pun yang lebih dulu.
type aliranPemegangSlot struct {
	providers.Stream
	lepas  func()
	sekali sync.Once
}

// Recv meneruskan pembacaan; akhir aliran melepas slot.
func (a *aliranPemegangSlot) Recv() (*providers.StreamEvent, error) {
	ev, err := a.Stream.Recv()
	if err != nil {
		a.sekali.Do(a.lepas)
	}
	return ev, err
}

// Close menutup aliran lalu melepas slot.
func (a *aliranPemegangSlot) Close() error {
	err := a.Stream.Close()
	a.sekali.Do(a.lepas)
	return err
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
// probe menyampaikan apakah percobaan ini diizinkan sebagai probe
// half_open, supaya hasilnya tidak tertukar dengan hasil basi percobaan
// dari episode sebelumnya (audit3 V-BR-03). Penjaga yang tidak menerapkan
// CircuitGuardHasilProbe menerima Record biasa.
//
// Kegagalan mencatat hanya di-log, tidak dikembalikan: pada titik ini permintaan sudah
// punya jawaban, dan menggagalkannya karena Redis tidak bisa dicatat berarti menukar
// permintaan yang berhasil dengan error demi kerapian metrik.
func (e *Executor) catat(ctx context.Context, c *upstream.RouteCandidate, model string, sukses bool, cfg BreakerConfig, probe bool) {
	if e.guard == nil {
		return
	}
	var err error
	if hp, ok := e.guard.(CircuitGuardHasilProbe); ok {
		if cfg != (BreakerConfig{}) {
			err = hp.RecordHasilWithConfig(ctx, c.ProviderID, model, sukses, probe, cfg)
		} else {
			err = hp.RecordHasil(ctx, c.ProviderID, model, sukses, probe)
		}
	} else if cfg != (BreakerConfig{}) {
		if ter, ok := e.guard.(CircuitGuardTerkonfigurasi); ok {
			err = ter.RecordWithConfig(ctx, c.ProviderID, model, sukses, cfg)
		} else {
			err = e.guard.Record(ctx, c.ProviderID, model, sukses)
		}
	} else {
		err = e.guard.Record(ctx, c.ProviderID, model, sukses)
	}
	if err != nil {
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
	plan.streaming = true
	return jalankanRencana(ctx, e, plan, denganTenggatPembukaan(bungkusPencatatAliran(ctx, e, plan, open)))
}

// bungkusPencatatAliran membungkus pembuka aliran agar hasil AKHIR aliran
// — bukan sekadar keberhasilannya dibuka — yang dicatat ke pemutus arus
// (audit F4). Aliran yang dibuka lalu mati di tengah sebelumnya tercatat
// sebagai keberhasilan penuh, sehingga provider yang alirannya selalu
// putus tidak pernah terputus arusnya.
func bungkusPencatatAliran(
	ctx context.Context,
	e *Executor,
	plan Plan,
	open func(ctx context.Context, c *upstream.RouteCandidate) (providers.Stream, error),
) func(ctx context.Context, c *upstream.RouteCandidate) (providers.Stream, error) {
	return func(octx context.Context, c *upstream.RouteCandidate) (providers.Stream, error) {
		str, err := open(octx, c)
		if err != nil || str == nil {
			return str, err
		}

		// Penjaga diam dipasang paling dalam, di bawah pencatat: error tenggat
		// diam yang ia hasilkan harus mengalir LEWAT aliranPencatat supaya
		// tercatat sebagai kegagalan aliran, bukan penutupan yang sehat
		// (audit3 V-GW-14).
		dijaga := &aliranPenjagaDiam{Stream: str, batas: tenggatDiamAliran}
		dijaga.mulai()

		// Peristiwa pertama dibaca SEKARANG, selagi kegagalannya masih bisa
		// dialihkan (audit3 V-GW-12). Keputusan routing dulu ditutup saat
		// aliran berhasil DIBUKA, jadi kegagalan Recv pertama — event
		// overloaded, chunk pertama rusak, aliran langsung mati — sampai ke
		// klien sebagai satu-satunya jawaban walau kandidat sehat berikutnya
		// menganggur dan belum satu byte pun terkirim ke klien. Dengan
		// intipan ini kegagalan itu menjadi kegagalan MEMBUKA, dan executor
		// mengulang/mengalihkannya seperti kegagalan pembukaan lain.
		// Peristiwa yang berhasil dibaca ditahan dan diputar ulang sebagai
		// peristiwa pertama aliran.
		ev, err := dijaga.Recv()
		if err != nil {
			_ = dijaga.Close()
			if errors.Is(err, io.EOF) {
				// Aliran kosong: upstream menutup dengan rapi tanpa satu pun
				// peristiwa. Itu bukan jawaban — tidak ada konten, tidak ada
				// usage, tidak ada alasan berhenti — jadi ia kegagalan server
				// yang boleh dialihkan, bukan penyelesaian bersih.
				err = providers.Newf(providers.ErrKindServer, c.ProviderName,
					"upstream menutup aliran tanpa mengirim satu pun peristiwa")
			}
			return nil, err
		}

		// Status probe pembukaan ini dibaca dari context percobaan: hasil
		// aliran dicatat saat aliran berakhir, dan hanya penanda di context
		// yang masih menghubungkan akhir itu ke izin yang membukanya
		// (audit3 V-BR-03).
		probe := probeDariContext(octx)
		return &aliranPencatat{
			Stream: &aliranPeristiwaPertama{Stream: dijaga, ev: ev},
			ctx:    ctx,
			catat: func(sukses bool) {
				e.catat(ctx, c, modelKandidat(plan, c), sukses, plan.BreakerCfg, probe)
			},
		}, nil
	}
}

// aliranPeristiwaPertama memutar ulang satu peristiwa yang sudah dibaca —
// lihat intipan peristiwa pertama di bungkusPencatatAliran — sebelum
// meneruskan pembacaan ke aliran di bawahnya.
//
// Recv tidak aman untuk panggilan konkuren; sama seperti aliran lain di
// berkas ini, satu-satunya konsumennya adalah satu goroutine loop pembacaan.
type aliranPeristiwaPertama struct {
	providers.Stream
	ev    *providers.StreamEvent
	sudah bool
}

func (a *aliranPeristiwaPertama) Recv() (*providers.StreamEvent, error) {
	if !a.sudah {
		a.sudah = true
		return a.ev, nil
	}
	return a.Stream.Recv()
}

// aliranPenjagaDiam membatalkan aliran yang berhenti mengirim peristiwa
// melewati batasnya (audit3 V-GW-14).
//
// Timer di-reset setiap peristiwa berhasil diterima. Bila timer menembak,
// aliran di bawah ditutup dari goroutine timer — Close adapter memang
// dirancang aman dipanggil bersamaan dengan Recv yang menggantung — dan
// Recv yang sedang menunggu menerima error penutupan itu, yang di sini
// diterjemahkan menjadi ErrKindTimeout. Klasifikasi itu penting: error
// mentahnya terbaca "aliran ditutup/dibatalkan", dan mencatat kematian
// upstream sebagai pembatalan klien akan menyembunyikannya dari pemutus
// arus dan statistik kegagalan.
type aliranPenjagaDiam struct {
	providers.Stream

	batas time.Duration

	mu       sync.Mutex
	timer    *time.Timer
	menembak bool
	berakhir bool
	terkirim int64
}

// mulai menyalakan timer. Dipanggil sekali, tepat sesudah aliran dibuka:
// waktu menuju peristiwa pertama ikut dijaga, bukan hanya jeda antarperistiwa.
func (a *aliranPenjagaDiam) mulai() {
	a.mu.Lock()
	defer a.mu.Unlock()
	a.timer = time.AfterFunc(a.batas, a.tembak)
}

// tembak menandai tenggat diam terlampaui lalu menutup aliran di bawahnya.
func (a *aliranPenjagaDiam) tembak() {
	a.mu.Lock()
	a.menembak = true
	a.mu.Unlock()
	_ = a.Stream.Close()
}

func (a *aliranPenjagaDiam) Recv() (*providers.StreamEvent, error) {
	ev, err := a.Stream.Recv()

	a.mu.Lock()
	if err == nil {
		a.terkirim += int64(len(ev.Raw))
		if !a.berakhir && a.timer != nil {
			a.timer.Reset(a.batas)
		}
		a.mu.Unlock()
		return ev, nil
	}
	menembak := a.menembak
	a.berakhir = true
	if a.timer != nil {
		a.timer.Stop()
	}
	terkirim := a.terkirim
	a.mu.Unlock()

	if menembak && !errors.Is(err, io.EOF) {
		return nil, &providers.Error{
			Kind:          providers.ErrKindTimeout,
			Message:       "upstream berhenti mengirim aliran terlalu lama",
			StreamedBytes: terkirim,
		}
	}
	return nil, err
}

func (a *aliranPenjagaDiam) Close() error {
	a.mu.Lock()
	a.berakhir = true
	if a.timer != nil {
		a.timer.Stop()
	}
	a.mu.Unlock()
	return a.Stream.Close()
}

// aliranPencatat adalah providers.Stream yang melaporkan hasil akhirnya
// tepat satu kali. Aturan hasilnya:
//   - Recv mengembalikan io.EOF: aliran selesai bersih — sukses.
//   - Recv mengembalikan error lain: aliran mati di tengah — gagal.
//   - Close sebelum Recv terminal: sukses bila aliran sudah sempat
//     mengirim >=1 peristiwa tanpa error (konsumen menutup aliran yang
//     sehat), gagal bila belum ada satu pun peristiwa (aliran dibuka
//     lalu mati sebelum berguna).
//   - Context permintaan sudah dibatalkan (klien pergi): tidak dicatat
//     sama sekali, konsisten dengan aturan ErrKindCanceled di executor.
type aliranPencatat struct {
	providers.Stream
	ctx    context.Context
	catat  func(sukses bool)
	mu     sync.Mutex
	sudah  bool
	terima int
}

func (a *aliranPencatat) laporkan(sukses bool) {
	a.mu.Lock()
	defer a.mu.Unlock()
	if a.sudah {
		return
	}
	a.sudah = true
	if a.ctx.Err() != nil {
		// Klien sudah pergi: jangan catat apa pun (konsisten dengan aturan
		// ErrKindCanceled di cobaKandidat).
		return
	}
	a.catat(sukses)
}

func (a *aliranPencatat) Recv() (*providers.StreamEvent, error) {
	ev, err := a.Stream.Recv()
	if err != nil {
		a.laporkan(errors.Is(err, io.EOF))
	} else {
		a.mu.Lock()
		a.terima++
		a.mu.Unlock()
	}
	return ev, err
}

func (a *aliranPencatat) Close() error {
	a.mu.Lock()
	terima := a.terima
	a.mu.Unlock()
	a.laporkan(terima > 0)
	return a.Stream.Close()
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
		//
		// Nilai kembali Stop DIPERIKSA (audit3 V-GW-16): false berarti callback
		// sudah terlanjur menembak atau sedang menembak, jadi konteks aliran ini
		// sudah — atau sebentar lagi — dibatalkan berpenyebab DeadlineExceeded.
		// Aliran yang "berhasil dibuka" dalam keadaan itu tidak bisa dipakai:
		// ia akan mati pada pembacaan pertama. Buang dan laporkan sebagai
		// tenggat pembukaan habis, sama seperti pembukaan yang memang terlambat.
		berhenti := pengawas.Stop()

		if err == nil && str != nil && !berhenti {
			_ = str.Close()
			batalkan(nil)
			return nil, providers.Newf(providers.ErrKindTimeout, c.ProviderName,
				"upstream tidak mulai mengirim aliran sebelum batas waktu percobaan")
		}

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
