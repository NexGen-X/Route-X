// Package usage mencatat apa yang benar-benar terjadi pada setiap permintaan: token,
// biaya, latensi, jejak percobaan, dan pemakaian anggaran.
//
// # Pencatatan tidak boleh menggagalkan permintaan
//
// Ini sifat yang menentukan seluruh bentuk paket ini. Pada saat pencatatan berjalan,
// jawaban sudah terkirim ke klien dan biayanya sudah dikeluarkan ke provider. Yang hilang
// kalau pencatatan gagal adalah satu baris log; yang hilang kalau kegagalan itu dibiarkan
// merambat adalah jawaban bagi pengguna. Karena itu Record tidak mengembalikan error, dan
// tidak ada satu pun jalur di sini yang bisa membuat handler HTTP gagal.
//
// # Kenapa penulisannya asinkron
//
// Record hanya menghitung biaya, mencatat metrik, lalu menaruh catatan di antrean. Tiga
// alasan, dan yang pertama sendiri sudah cukup:
//
//  1. Context permintaan sudah MATI ketika pencatatan terjadi. Handler yang selesai
//     membatalkan context-nya, dan pada aliran yang diputus klien ia bahkan sudah mati
//     sebelum jawaban selesai. INSERT dengan context itu gagal setiap kali — bukan
//     kadang-kadang.
//  2. Database yang lambat tidak boleh menambah latensi pada permintaan yang jawabannya
//     sudah dikirim. Tanpa antrean, setiap hentakan Postgres menjadi hentakan gateway.
//  3. Metrik tetap tercatat walaupun penulisan ke database gagal atau dibuang, karena
//     metriknya diambil SEBELUM antrean. Kalau keduanya asinkron, hilangnya satu baris
//     database ikut menghilangkan angka yang seharusnya melaporkan hilangnya baris itu.
//
// Antrean punya batas, dan penuh berarti catatan DIBUANG, bukan permintaan yang ditahan.
// Menahan permintaan demi kelengkapan log adalah pertukaran yang salah arah. Yang membuat
// pembuangan itu bisa dipertanggungjawabkan adalah metrik routex_usage_records_total —
// tanpa memantaunya, gateway bisa berjalan berhari-hari tanpa mencatat satu pun baris
// pemakaian tanpa ada yang tahu.
//
// # Token: dua bentuk, satu sumber
//
// Kolom token di tabel requests memakai konvensi OpenAI seperti body respons yang
// diteruskan ke klien: input_tokens SUDAH memuat cached_input_tokens, dan output_tokens
// SUDAH memuat reasoning_tokens. Itu disengaja — angka di log request harus sama dengan
// angka yang dilihat klien di jawabannya, kalau tidak setiap penyelidikan tagihan dimulai
// dengan mempertanyakan mana yang benar.
//
// Perhitungan biaya menuntut bentuk yang berbeda, yaitu keempat bagian yang saling lepas,
// dan konversinya ada di TokensFor. Batas antara kedua bentuk itu hanya di satu tempat.
package usage

import (
	"context"
	"log/slog"
	"net/netip"
	"sync"
	"sync/atomic"
	"time"

	"github.com/NexGen-X/Route-X/internal/database/repo/policy"
	"github.com/NexGen-X/Route-X/internal/database/repo/traffic"
	"github.com/NexGen-X/Route-X/internal/database/repo/upstream"
	"github.com/NexGen-X/Route-X/internal/observability"
)

// Nilai bawaan pencatat.
const (
	// defaultQueueSize adalah panjang antrean catatan yang menunggu ditulis.
	//
	// Dipilih untuk menyerap hentakan, bukan pemadaman: 4.096 catatan cukup menampung
	// beberapa detik lalu lintas padat sementara Postgres menyelesaikan checkpoint, dan
	// tidak cukup untuk menyembunyikan database yang mati selama semenit — yang memang
	// tidak boleh disembunyikan.
	defaultQueueSize = 4096

	// defaultWorkers adalah jumlah penulis yang berjalan bersamaan.
	//
	// Empat, bukan satu: satu penulis membuat throughput pencatatan terikat pada latensi
	// satu round-trip database, dan pada database jarak jauh itu berarti antrean penuh
	// jauh sebelum databasenya sendiri sibuk. Bukan pula sebanyak mungkin — setiap
	// penulis memegang satu koneksi pool, dan koneksi itu diambil dari jatah yang sama
	// dengan jalur permintaan.
	defaultWorkers = 4

	// defaultWriteTimeout membatasi satu penulisan catatan.
	//
	// Batas ini yang membuat satu pernyataan yang menggantung tidak menahan penulis
	// selamanya sementara antrean di belakangnya penuh lalu mulai membuang catatan.
	defaultWriteTimeout = 10 * time.Second
)

// Hasil pemrosesan satu catatan, dipakai sebagai label metrik.
const (
	// OutcomeWritten berarti baris requests berhasil ditulis.
	OutcomeWritten = "written"
	// OutcomeFailed berarti baris requests tidak bisa ditulis.
	OutcomeFailed = "failed"
	// OutcomeDropped berarti catatan dibuang karena antrean penuh atau pencatat sudah
	// ditutup. Angka ini yang membuat batas antrean bisa dipertanggungjawabkan.
	OutcomeDropped = "dropped"
	// OutcomeSpendFailed berarti barisnya tertulis tetapi penambahan pemakaian anggaran
	// gagal. Dipisah karena akibatnya berbeda dan lebih serius: penegakan anggaran
	// berhenti bergerak tanpa satu pun permintaan yang terlihat gagal.
	OutcomeSpendFailed = "spend_failed"
)

// Attempt adalah satu percobaan ke upstream di dalam satu permintaan.
//
// Bentuknya sengaja bukan gateway.Attempt: paket ini tidak boleh bergantung pada paket
// gateway, karena arah ketergantungannya adalah gateway yang memakai paket ini.
type Attempt struct {
	ProviderID   string
	ProviderName string
	// UpstreamModel adalah nama model di sisi provider ini.
	UpstreamModel string

	// Nth adalah percobaan ke-berapa pada kandidat ini, mulai dari 1.
	Nth int
	// Duration hanya mencakup percobaan ini, tanpa jeda backoff sebelumnya.
	Duration time.Duration

	// BreakerState adalah state pemutus arus saat percobaan diputuskan.
	BreakerState string

	// StatusCode adalah status HTTP dari upstream, 0 bila kegagalannya sebelum ada respons.
	StatusCode int
	// ErrorKind kosong berarti percobaan ini berhasil.
	ErrorKind string
	// ErrorMessage WAJIB sudah disaring pemanggil.
	ErrorMessage string

	// SkipReason terisi bila kandidat dilewati tanpa pernah dihubungi.
	SkipReason string
}

// Berhasil melaporkan apakah percobaan ini benar-benar berhasil.
func (a Attempt) Berhasil() bool { return a.ErrorKind == "" && a.SkipReason == "" }

// Note adalah peristiwa timeline yang BUKAN percobaan upstream: penolakan kebijakan dan
// siklus hidup aliran.
//
// Ada karena sebagian permintaan tidak pernah menyentuh upstream sama sekali — diblokir
// penyaring konten, ditolak anggaran — dan timeline yang hanya memuat percobaan tidak bisa
// menjelaskan apa pun tentang permintaan itu. Kind wajib salah satu konstanta traffic.Event*.
type Note struct {
	Kind    string
	Message string
}

// Routing adalah ringkasan keputusan routing yang disimpan di requests.routing_decision.
//
// Inilah isi tab "Routing" di request inspector: pertanyaannya "kenapa permintaan ini
// pergi ke sana", dan jawabannya tidak bisa disusun ulang setelah kejadian karena aturan,
// harga, dan kesehatan provider semuanya berubah. Jejak percobaannya tidak diulang di sini
// — itu milik request_events.
type Routing struct {
	Strategy   string             `json:"strategy,omitempty"`
	RuleID     string             `json:"rule_id,omitempty"`
	RuleName   string             `json:"rule_name,omitempty"`
	Candidates []RoutingCandidate `json:"candidates,omitempty"`
}

// RoutingCandidate adalah satu kandidat dalam urutan yang dihasilkan strategi.
type RoutingCandidate struct {
	// Position adalah urutan kandidat ini, mulai dari 1. Disimpan eksplisit karena JSON
	// array boleh diurut ulang alat apa pun yang menyentuhnya.
	Position        int    `json:"position"`
	ProviderID      string `json:"provider_id,omitempty"`
	ProviderName    string `json:"provider_name,omitempty"`
	ProviderModelID string `json:"provider_model_id,omitempty"`
	UpstreamModel   string `json:"upstream_model,omitempty"`
	Priority        int    `json:"priority"`
	Weight          int    `json:"weight"`
	// Chosen menandai kandidat yang akhirnya melayani permintaan.
	Chosen bool `json:"chosen,omitempty"`
}

// Event adalah fakta satu permintaan yang sudah selesai.
//
// Diserahkan gateway dalam satu potong, bukan dikumpulkan bertahap oleh paket ini: yang
// tahu kapan sebuah permintaan benar-benar selesai — termasuk aliran yang diputus klien di
// tengah — hanya lapisan HTTP.
type Event struct {
	RequestID string
	Method    string
	// Endpoint adalah path LENGKAP, mis. "/v1/chat/completions".
	Endpoint string

	APIKeyID   string
	APIKeyName string
	UserID     string

	// RequestedModel adalah nama yang dikirim klien, bisa berupa alias dan bisa berupa
	// nama yang tidak dikenal. TIDAK BOLEH dipakai sebagai label metrik: nilainya berasal
	// dari klien, jadi kardinalitasnya tidak terbatas.
	RequestedModel string
	// ModelID adalah models.id hasil resolusi, kosong bila permintaan ditolak sebelum itu.
	ModelID string
	// ModelName adalah nama model KANONIK dari registry. Inilah yang aman menjadi label
	// metrik.
	ModelName string

	ProviderID string
	// ProviderName adalah nama provider yang melayani; aman menjadi label metrik karena
	// berasal dari registry yang dikurasi operator.
	ProviderName string
	// ProviderModelID adalah kunci pemetaan model-provider, dipakai mengambil harga.
	ProviderModelID string
	UpstreamModel   string

	StatusCode int
	// ErrorType adalah kategori kegagalan yang STABIL untuk diagregasi, mis. "timeout"
	// atau "content_filter" — bukan tipe envelope yang dikirim ke klien. Rollup
	// menghitung timeout_count dari kolom ini, jadi ejaan kategorinya tidak boleh berubah
	// tanpa alasan.
	ErrorType string
	// ErrorCode adalah kode pada envelope error yang diterima klien, mis.
	// "upstream_timeout". Disimpan supaya keluhan pengguna bisa dicari dari kode yang ia
	// lihat.
	ErrorCode string
	// ErrorMessage WAJIB sudah disaring.
	ErrorMessage string

	Stream bool

	// Usage memakai konvensi OpenAI: Input sudah memuat CachedInput, Output sudah memuat
	// Reasoning. Lihat catatan paket.
	InputTokens       int
	CachedInputTokens int
	OutputTokens      int
	ReasoningTokens   int
	TotalTokens       int

	// Latency adalah umur seluruh permintaan di gateway ini.
	Latency time.Duration
	// TTFT hanya terisi pada respons streaming, diukur sampai potongan konten pertama
	// benar-benar diteruskan ke klien.
	TTFT time.Duration
	// UpstreamLatency adalah waktu yang dihabiskan menunggu upstream.
	UpstreamLatency time.Duration

	RetryCount    int
	FailoverCount int

	RoutingStrategy string
	Routing         Routing

	Attempts []Attempt
	Notes    []Note

	// Targets adalah cakupan kebijakan permintaan ini, dipakai menaikkan pemakaian
	// anggaran. Harus daftar yang SAMA dengan yang dipakai saat menggerbangi permintaan;
	// daftar yang disusun dua kali adalah daftar yang bisa berbeda.
	Targets []policy.Target

	ClientIP  netip.Addr
	UserAgent string
}

// Store menulis log request. Dipenuhi *traffic.Repo.
type Store interface {
	Insert(ctx context.Context, rec traffic.Record) (id string, createdAt time.Time, err error)
	AppendEvents(ctx context.Context, requestPK string, createdAt time.Time, events []traffic.Event) error
}

// Spender menaikkan pemakaian anggaran. Dipenuhi *policy.Repo.
type Spender interface {
	AddSpend(ctx context.Context, targets []policy.Target, amount upstream.USD) (int64, error)
}

// Deps mengumpulkan dependensi Recorder. Semuanya boleh nil, dan nil berarti bagian itu
// tidak dikerjakan — bukan kegagalan.
type Deps struct {
	Store   Store
	Spend   Spender
	Prices  *Pricer
	Metrics *observability.Metrics
	Logger  *slog.Logger
}

// Recorder mencatat hasil permintaan ke database dan ke metrik.
//
// Aman dipakai bersamaan oleh banyak goroutine.
type Recorder struct {
	store   Store
	spend   Spender
	prices  *Pricer
	metrics *observability.Metrics
	logger  *slog.Logger

	antre   chan Event
	mati    chan struct{}
	tutup   sync.Once
	wg      sync.WaitGroup
	dalam   atomic.Int64
	batas   time.Duration
	pekerja int
	ukuran  int
}

// Option menyetel Recorder.
type Option func(*Recorder)

// WithQueueSize mengubah panjang antrean. Nilai <= 0 diabaikan.
func WithQueueSize(n int) Option {
	return func(r *Recorder) {
		if n > 0 {
			r.ukuran = n
		}
	}
}

// WithWorkers mengubah jumlah penulis. Nilai <= 0 diabaikan.
func WithWorkers(n int) Option {
	return func(r *Recorder) {
		if n > 0 {
			r.pekerja = n
		}
	}
}

// WithWriteTimeout mengubah batas satu penulisan. Nilai <= 0 diabaikan.
func WithWriteTimeout(d time.Duration) Option {
	return func(r *Recorder) {
		if d > 0 {
			r.batas = d
		}
	}
}

// New membuat pencatat dan menyalakan penulisnya.
//
// Pemanggil WAJIB memanggil Close saat shutdown, dan Close-nya harus dijalankan SETELAH
// server HTTP berhenti menerima permintaan: catatan yang masuk setelah antrean dikuras
// tidak akan tertulis.
func New(d Deps, opts ...Option) *Recorder {
	logger := d.Logger
	if logger == nil {
		logger = slog.New(slog.DiscardHandler)
	}
	r := &Recorder{
		store: d.Store, spend: d.Spend, prices: d.Prices, metrics: d.Metrics, logger: logger,
		mati:    make(chan struct{}),
		batas:   defaultWriteTimeout,
		pekerja: defaultWorkers,
		ukuran:  defaultQueueSize,
	}
	for _, o := range opts {
		if o != nil {
			o(r)
		}
	}
	if r.store == nil {
		// Bukan error, tetapi juga bukan keadaan yang boleh terjadi tanpa disadari:
		// gateway yang berjalan tanpa penyimpanan tidak punya satu pun baris log request,
		// dan halaman Requests akan kosong tanpa penjelasan.
		logger.Warn("pencatat pemakaian dibuat tanpa penyimpanan, tidak ada baris requests yang akan ditulis")
	}

	r.antre = make(chan Event, r.ukuran)
	for range r.pekerja {
		r.wg.Add(1)
		go r.jalan()
	}
	return r
}

// Record mencatat satu permintaan yang sudah selesai.
//
// Tidak pernah memblokir dan tidak pernah mengembalikan error. Metrik diambil di sini,
// SEBELUM antrean, supaya hilangnya satu baris database tidak ikut menghilangkan angka
// yang melaporkan hilangnya baris itu. Satu-satunya yang menunggu antrean adalah biaya,
// karena menghitungnya butuh cuplikan harga yang pemuatannya adalah pembacaan database.
func (r *Recorder) Record(ctx context.Context, ev Event) {
	if r == nil {
		return
	}
	r.catatMetrik(ev)

	// Sinyal berhenti diperiksa SENDIRI lebih dulu, bukan sebagai satu cabang select
	// bersama pengiriman ke antrean. Kalau digabung, keduanya siap pada saat yang sama
	// setelah Close dan Go memilih salah satunya secara acak — sehingga sebagian catatan
	// masuk ke antrean yang sudah tidak dilayani siapa pun lalu hilang tanpa terhitung.
	select {
	case <-r.mati:
		r.hasil(OutcomeDropped)
		r.logger.WarnContext(ctx, "catatan pemakaian dibuang karena pencatat sudah ditutup",
			"request_id", ev.RequestID)
		return
	default:
	}

	select {
	case r.antre <- ev:
		r.metrikKedalaman(r.dalam.Add(1))
	default:
		// Antrean penuh: catatan dibuang, permintaan TIDAK ditahan. Menahan permintaan
		// demi kelengkapan log adalah pertukaran yang salah arah — yang hilang di sini
		// satu baris log, yang hilang di sana jawaban bagi pengguna.
		r.hasil(OutcomeDropped)
		r.logger.WarnContext(ctx, "antrean pencatat pemakaian penuh, catatan dibuang",
			"request_id", ev.RequestID, "kapasitas", r.ukuran)
	}
}

// Close menghentikan penulis setelah antrean dikuras.
//
// Menunggu sampai ctx selesai, lalu berhenti menunggu dan melaporkan berapa catatan yang
// belum tertulis. Tidak memaksa penulis berhenti: proses yang sedang shutdown akan keluar
// sebentar lagi, dan membatalkan penulisan yang sedang berjalan hanya menukar satu baris
// yang hampir selesai dengan transaksi yang dibatalkan.
//
// Aman dipanggil berulang.
func (r *Recorder) Close(ctx context.Context) error {
	if r == nil {
		return nil
	}
	r.tutup.Do(func() { close(r.mati) })

	selesai := make(chan struct{})
	go func() {
		r.wg.Wait()
		close(selesai)
	}()

	select {
	case <-selesai:
		return nil
	case <-ctx.Done():
		sisa := r.dalam.Load()
		r.logger.Warn("pencatat pemakaian ditutup sebelum antrean habis", "sisa", sisa)
		return ctx.Err()
	}
}

// jalan adalah satu penulis.
//
// Setelah sinyal berhenti, antrean tetap DIKURAS lebih dulu. Kalau tidak, catatan yang
// sudah diterima dari permintaan yang berhasil dilayani akan hilang pada setiap restart —
// dan restart adalah kejadian rutin, bukan luar biasa.
func (r *Recorder) jalan() {
	defer r.wg.Done()
	for {
		select {
		case ev := <-r.antre:
			r.metrikKedalaman(r.dalam.Add(-1))
			r.tulis(ev)
		case <-r.mati:
			for {
				select {
				case ev := <-r.antre:
					r.metrikKedalaman(r.dalam.Add(-1))
					r.tulis(ev)
				default:
					return
				}
			}
		}
	}
}

// tulis menyimpan satu catatan.
//
// Context-nya BARU, bukan turunan context permintaan: permintaannya sudah selesai dan
// context-nya sudah dibatalkan, jadi turunan apa pun darinya gagal seketika. Tenggatnya
// ada supaya satu pernyataan yang menggantung tidak menahan penulis ini sementara antrean
// di belakangnya penuh lalu mulai membuang catatan.
func (r *Recorder) tulis(ev Event) {
	ctx, batal := context.WithTimeout(context.Background(), r.batas)
	defer batal()

	if statusTerpakai(ev.StatusCode) != ev.StatusCode {
		// Berarti ada jalur keluar di lapisan HTTP yang selesai tanpa melaporkan statusnya.
		// Barisnya tetap ditulis dengan 500 — lihat statusTerpakai — tetapi jalur itu perlu
		// dicari, dan hanya log ini yang menunjukkan bahwa ia ada.
		r.logger.ErrorContext(ctx, "permintaan dicatat tanpa status HTTP yang sah",
			"request_id", ev.RequestID, "status", ev.StatusCode, "endpoint", ev.Endpoint)
	}

	biaya := r.biaya(ctx, ev)

	if r.store != nil {
		pk, dibuat, err := r.store.Insert(ctx, ev.record(biaya))
		switch {
		case err != nil:
			r.hasil(OutcomeFailed)
			r.logger.ErrorContext(ctx, "baris log request gagal ditulis",
				"request_id", ev.RequestID, "status", ev.StatusCode, "error", err)
		default:
			r.hasil(OutcomeWritten)
			if timeline := ev.timeline(); len(timeline) > 0 {
				if err := r.store.AppendEvents(ctx, pk, dibuat, timeline); err != nil {
					// Timeline hanya diagnostik: barisnya sudah tertulis, dan kehilangan
					// jejak percobaan tidak mengubah satu pun angka pemakaian.
					r.logger.WarnContext(ctx, "timeline percobaan gagal ditulis",
						"request_id", ev.RequestID, "error", err)
				}
			}
		}
	}

	r.tambahPemakaian(ctx, ev, biaya)
}

// biaya menghitung biaya satu permintaan dari harga yang berlaku.
//
// Nol dikembalikan untuk dua keadaan yang berbeda dan keduanya benar: permintaan yang tidak
// memakai token sama sekali, dan pemetaan yang harganya belum diisi. Yang kedua dilaporkan
// ke log lewat laporTanpaHarga, karena biaya nol yang berarti "belum diisi" terlihat sama
// dengan gratis di setiap laporan.
func (r *Recorder) biaya(ctx context.Context, ev Event) upstream.USD {
	if r.prices == nil || ev.ProviderModelID == "" {
		return 0
	}
	// TotalTokens TIDAK boleh jadi gerbang: adapter yang tidak menjumlahkan
	// (atau streaming yang abort) mengisi rincian Input/Output sementara
	// TotalTokens kosong — menggerbangi dari Total membuat biaya 0 permanen
	// padahal tokennya tercatat di baris requests. Gerbangnya adalah jumlah
	// rincian yang sama dengan yang ditagih Cost di bawah.
	if ev.tokenUsage().Total() <= 0 {
		return 0
	}
	harga, ok := r.prices.Price(ctx, ev.ProviderModelID)
	if !ok {
		r.prices.laporTanpaHarga(ctx, ev.ProviderModelID, ev.ProviderName, ev.ModelName)
		return 0
	}
	rincian, err := harga.Cost(ev.tokenUsage())
	if err != nil {
		r.logger.WarnContext(ctx, "biaya permintaan tidak bisa dihitung",
			"request_id", ev.RequestID, "provider", ev.ProviderName, "model", ev.ModelName, "error", err)
		return 0
	}
	if r.metrics != nil && rincian.Total > 0 {
		// Metrik Prometheus bertipe float64, jadi angka di sini bisa menyimpang beberapa
		// satuan 10^-8 setelah jutaan penjumlahan. Yang berwenang untuk penagihan tetap
		// requests.cost_usd, yang numeric dan eksak.
		r.metrics.CostTotal.WithLabelValues(ev.ProviderName, ev.ModelName).
			Add(float64(rincian.Total) / 1e8)
	}
	return rincian.Total
}

// tambahPemakaian menaikkan pemakaian setiap anggaran yang berlaku.
//
// INI yang membuat anggaran benar-benar menahan biaya. Penegakannya sudah terpasang sejak
// Fase 8, tetapi angka yang ditegakkannya tidak bergerak sampai baris ini berjalan.
//
// Kegagalannya dicatat sebagai ERROR dan dihitung terpisah di metrik, bukan disamakan
// dengan kegagalan menulis log: log yang hilang membuat satu permintaan tidak terlihat,
// sementara pemakaian yang tidak naik membuat SELURUH anggaran berhenti menahan apa pun —
// tanpa satu pun permintaan yang terlihat gagal.
func (r *Recorder) tambahPemakaian(ctx context.Context, ev Event, biaya upstream.USD) {
	if r.spend == nil || biaya <= 0 || len(ev.Targets) == 0 {
		return
	}
	if _, err := r.spend.AddSpend(ctx, ev.Targets, biaya); err != nil {
		r.hasil(OutcomeSpendFailed)
		r.logger.ErrorContext(ctx, "pemakaian anggaran gagal dinaikkan, penegakan anggaran tidak bergerak",
			"request_id", ev.RequestID, "biaya_usd", biaya.String(), "error", err)
	}
}

// hasil menaikkan penghitung hasil pemrosesan catatan.
func (r *Recorder) hasil(outcome string) {
	if r.metrics != nil {
		r.metrics.UsageRecords.WithLabelValues(outcome).Inc()
	}
}

// metrikKedalaman melaporkan kedalaman antrean.
func (r *Recorder) metrikKedalaman(n int64) {
	if r.metrics != nil {
		r.metrics.UsageQueueDepth.Set(float64(n))
	}
}

// Depth mengembalikan jumlah catatan yang menunggu ditulis. Untuk test dan diagnostik.
func (r *Recorder) Depth() int64 {
	if r == nil {
		return 0
	}
	return r.dalam.Load()
}
