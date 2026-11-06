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
	"fmt"
	"log/slog"
	"net/netip"
	"sync"
	"sync/atomic"
	"time"

	"github.com/NexGen-X/Route-X/internal/apikey"
	"github.com/NexGen-X/Route-X/internal/database/repo/policy"
	"github.com/NexGen-X/Route-X/internal/database/repo/traffic"
	"github.com/NexGen-X/Route-X/internal/database/repo/upstream"
	"github.com/NexGen-X/Route-X/internal/httpx"
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
	// Kegagalan penambahan pemakaian anggaran BUKAN outcome di sini: barisnya
	// bisa tertulis (written) DAN pemakaiannya gagal pada event yang sama,
	// jadi menghitungnya sebagai outcome membuat label ini bukan partisi
	// kejadian. Ia punya counter sendiri, routex_usage_spend_failures_total
	// (audit3 V-EM-25).
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

	// CacheHit menandai permintaan yang terjawab dari response cache tanpa
	// satu pun panggilan upstream. Biayanya dipaksa nol oleh Recorder.biaya
	// (B-188): desain response cache menjanjikan HIT tetap memakan kuota
	// token tetapi tidak menagih biaya, dan sejak audit3 V-GW-09 jalur HIT
	// memasang kandidat pemenang demi atribusi provider — tanpa penanda
	// eksplisit ini pencatat menagih harga penuh seolah upstream dipanggil.
	// Tokennya tetap tercatat utuh untuk kuota dan baris requests.
	CacheHit bool

	// Usage memakai konvensi OpenAI: Input sudah memuat CachedInput, Output sudah memuat
	// Reasoning. Lihat catatan paket.
	InputTokens       int
	CachedInputTokens int
	// CacheCreationInputTokens adalah bagian dari InputTokens yang merupakan
	// token TULIS prompt cache; dipisahkan hanya untuk penagihan (tarifnya
	// berbeda), bukan kolom requests tersendiri.
	CacheCreationInputTokens int
	// CacheCreation1hInputTokens dan CacheCreation5mInputTokens adalah
	// pecahan TTL dari CacheCreationInputTokens (tulis cache 1 jam vs
	// 5 menit), diteruskan apa adanya ke TokensFor untuk tarif yang
	// berbeda. Nol bila upstream tidak melaporkannya.
	CacheCreation1hInputTokens int
	CacheCreation5mInputTokens int
	OutputTokens               int
	ReasoningTokens            int
	TotalTokens                int

	// TerjadiPada adalah saat permintaan selesai di gateway. Penagihan memakai
	// harga yang berlaku pada saat INI, bukan harga berjalan saat pencatat
	// asinkron kebetulan memproses antrean (audit3 V-AB-09): pergantian harga
	// di antara keduanya tidak boleh mengubah biaya permintaan yang sudah
	// selesai. Nol berarti pemanggil tidak tahu — pencatat jatuh ke harga
	// berjalan, perilaku lama.
	TerjadiPada time.Time

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

	antre chan Event
	mati  chan struct{}
	tutup sync.Once
	wg    sync.WaitGroup
	dalam atomic.Int64
	// kirimMu menutup jendela antara pemeriksaan sinyal tutup dan pengiriman
	// ke antrean di Record (audit3 V-LN-17): Record memegang kunci baca selama
	// periksa-dan-kirim, Close memegang kunci tulis saat menutup mati, jadi
	// tidak ada catatan yang bisa lolos pemeriksaan tepat sebelum Close lalu
	// mendarat di antrean yang sudah tidak dikuras siapa pun.
	kirimMu sync.RWMutex
	batas   time.Duration
	pekerja int
	ukuran  int
}

// Option menyetel Recorder.
type Option func(*Recorder)

// WithQueueSize mengubah panjang antrean. Nilai <= 0 diabaikan.
// Seam pengujian yang disengaja (triage B-005): produksi memakai nilai bawaan; opsi ini ada agar test dapat menyetel perilaku secara deterministik.
func WithQueueSize(n int) Option {
	return func(r *Recorder) {
		if n > 0 {
			r.ukuran = n
		}
	}
}

// WithWorkers mengubah jumlah penulis. Nilai <= 0 diabaikan.
// Seam pengujian yang disengaja (triage B-005): produksi memakai nilai bawaan; opsi ini ada agar test dapat menyetel perilaku secara deterministik.
func WithWorkers(n int) Option {
	return func(r *Recorder) {
		if n > 0 {
			r.pekerja = n
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

	// Periksa-dan-kirim berjalan di bawah kunci baca kirimMu, dan Close menutup
	// sinyal berhenti di bawah kunci tulisnya (audit3 V-LN-17). Dua select
	// terpisah tanpa kunci menyisakan jendela: pemeriksaan lolos, lalu Close
	// menutup dan penulis selesai menguras, lalu pengiriman mendarat di antrean
	// yang tak lagi dilayani — catatan hilang tanpa terhitung sebagai dropped.
	// Keputusan kirim/buang diambil di bawah kunci baca kirimMu, tetapi
	// penyelesaian akuntansi catatan yang dibuang (catatDibuang) berjalan
	// SESUDAH kunci dilepas: ia boleh menyentuh database lewat query harga,
	// dan menahannya di bawah kunci akan menahan Close yang menunggu kunci
	// tulis hanya demi catatan yang sudah pasti dibuang.
	dibuang := ""
	r.kirimMu.RLock()
	// Sinyal berhenti diperiksa SENDIRI lebih dulu, bukan sebagai satu cabang select
	// bersama pengiriman ke antrean. Kalau digabung, keduanya siap pada saat yang sama
	// setelah Close dan Go memilih salah satunya secara acak — sehingga sebagian catatan
	// masuk ke antrean yang sudah tidak dilayani siapa pun lalu hilang tanpa terhitung.
	select {
	case <-r.mati:
		dibuang = "ditutup"
	default:
		select {
		case r.antre <- ev:
			r.metrikKedalaman(r.dalam.Add(1))
		default:
			// Antrean penuh: catatan dibuang, permintaan TIDAK ditahan. Menahan permintaan
			// demi kelengkapan log adalah pertukaran yang salah arah — yang hilang di sini
			// satu baris log, yang hilang di sana jawaban bagi pengguna.
			dibuang = "penuh"
		}
	}
	r.kirimMu.RUnlock()

	switch dibuang {
	case "":
		return
	case "ditutup":
		r.hasil(OutcomeDropped)
		r.logger.WarnContext(ctx, "catatan pemakaian dibuang karena pencatat sudah ditutup",
			"request_id", ev.RequestID)
	default:
		r.hasil(OutcomeDropped)
		r.logger.WarnContext(ctx, "antrean pencatat pemakaian penuh, catatan dibuang",
			"request_id", ev.RequestID, "kapasitas", r.ukuran)
	}
	r.catatDibuang(ctx, ev)
}

// catatDibuang menyelesaikan akuntansi satu catatan yang DIBUANG (antrean
// penuh atau pencatat sudah ditutup).
//
// Baris requests-nya memang hilang — itulah arti dibuang — tetapi metrik
// requests/tokens sudah terlanjur dihitung di catatMetrik sebelum antrean,
// dan biaya upstream-nya nyata sudah keluar. Tanpa penyelesaian di sini,
// event yang dibuang terhitung di requests/tokens tetapi tidak di biaya
// dan tidak di pemakaian anggaran: total biaya dashboard lebih kecil dari
// jumlah barisnya, dan anggaran justru bocor tepat saat sistem sedang
// tertekan (audit3 V-EM-25 bagian b). Kini biayanya dihitung dengan
// kalkulator yang SAMA seperti jalur normal — biaya(), yang memakai
// PriceAt pada waktu event beserta premi cache di dalam Price.Cost — lalu
// dinaikkan ke anggaran lewat tambahPemakaian.
//
// Hitung ganda tidak mungkin: catatan yang dibuang tidak pernah masuk
// antrean, jadi tulis() tidak akan pernah melihatnya; catatan yang masuk
// antrean tidak pernah melewati fungsi ini. Kedua langkah memakai konteks
// bertenggat sendiri (konteksLangkah), jadi penyelesaian ini tidak bisa
// menggantung pemanggil tanpa batas.
func (r *Recorder) catatDibuang(ctx context.Context, ev Event) {
	ctxBiaya, batalBiaya := r.konteksLangkah()
	biaya := r.biaya(ctxBiaya, ev)
	batalBiaya()
	if biaya <= 0 {
		return
	}
	r.logger.WarnContext(ctx, "biaya catatan yang dibuang tetap dihitung dan dinaikkan ke anggaran",
		"request_id", ev.RequestID, "biaya_usd", biaya.String())

	ctxSpend, batalSpend := r.konteksLangkah()
	defer batalSpend()
	r.tambahPemakaian(ctxSpend, ev, biaya)
}

// RecordRejection mencatat penolakan middleware (401 autentikasi, 429 pembatas
// laju) sebagai baris requests, mengimplementasikan apikey.RejectionSink.
//
// Barisnya sengaja minimal dan jujur: tidak ada provider/model/token karena
// permintaan tidak pernah mencapai upstream; yang tercatat adalah persis yang
// terjadi — endpoint yang dituju, status, kategori & kode error, dan key
// pemiliknya bila kredensialnya sempat terverifikasi. Tanpa ini upaya memakai
// key mati atau dicabut tidak terlihat di mana pun (audit3 V-GW-46).
func (r *Recorder) RecordRejection(ctx context.Context, info apikey.RejectionInfo) {
	if r == nil || info.Request == nil {
		return
	}
	req := info.Request
	var ip netip.Addr
	if v, ok := httpx.ClientIPFrom(ctx); ok {
		ip = v
	}
	r.Record(ctx, Event{
		RequestID:    observability.RequestIDFrom(ctx),
		Method:       req.Method,
		Endpoint:     req.URL.Path,
		APIKeyID:     info.APIKeyID,
		APIKeyName:   info.APIKeyName,
		StatusCode:   info.StatusCode,
		ErrorType:    info.ErrorType,
		ErrorCode:    info.ErrorCode,
		ErrorMessage: info.ErrorMessage,
		ClientIP:     ip,
		UserAgent:    req.UserAgent(),
		TerjadiPada:  time.Now().UTC(),
	})
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
	r.tutup.Do(func() {
		// Kunci tulis menunggu setiap Record yang sedang memeriksa-dan-mengirim
		// selesai lebih dulu: sesudah baris ini, tidak ada pengiriman baru yang
		// bisa lolos, dan pengurasan penulis di jalan() pasti melihat seluruh
		// catatan yang pernah diterima (audit3 V-LN-17).
		r.kirimMu.Lock()
		close(r.mati)
		r.kirimMu.Unlock()
	})

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
			r.tulisAman(ev)
		case <-r.mati:
			for {
				select {
				case ev := <-r.antre:
					r.metrikKedalaman(r.dalam.Add(-1))
					r.tulisAman(ev)
				default:
					return
				}
			}
		}
	}
}

// tulisAman membungkus tulis dengan pemulihan panic (audit F5): penulis
// berjalan di goroutine tanpa supervisor, jadi SATU panic (baris cacat,
// harga ternormalisasi buruk, bug driver) sebelumnya membunuh SELURUH
// proses gateway. Kini panic hanya mengorbankan catatan itu sendiri.
func (r *Recorder) tulisAman(ev Event) {
	defer func() {
		if rec := recover(); rec != nil {
			r.hasil(OutcomeFailed)
			r.logger.Error("panic tertangkap saat menulis catatan pemakaian; hanya catatan ini yang hilang",
				"request_id", ev.RequestID, "panic", fmt.Sprint(rec))
		}
	}()
	r.tulis(ev)
}

// konteksLangkah membuat konteks BARU bertenggat untuk satu langkah penulisan.
//
// Bukan turunan context permintaan: permintaannya sudah selesai dan context-nya
// sudah dibatalkan, jadi turunan apa pun darinya gagal seketika. Dan SATU konteks
// per langkah, bukan satu untuk seluruh catatan (audit3 V-AB-22): versi lama
// membagi satu tenggat 10 detik berurutan untuk query harga + Insert +
// AppendEvents + AddSpend, sehingga database yang lambat membuat keempatnya
// gagal berkelompok dan pesan sebabnya menunjuk langkah yang salah. Kini langkah
// yang lambat hanya menggagalkan dirinya sendiri.
func (r *Recorder) konteksLangkah() (context.Context, context.CancelFunc) {
	return context.WithTimeout(context.Background(), r.batas)
}

// tulis menyimpan satu catatan.
func (r *Recorder) tulis(ev Event) {
	if statusTerpakai(ev.StatusCode) != ev.StatusCode {
		// Berarti ada jalur keluar di lapisan HTTP yang selesai tanpa melaporkan statusnya.
		// Barisnya tetap ditulis dengan 500 — lihat statusTerpakai — tetapi jalur itu perlu
		// dicari, dan hanya log ini yang menunjukkan bahwa ia ada.
		ctx, batal := r.konteksLangkah()
		r.logger.ErrorContext(ctx, "permintaan dicatat tanpa status HTTP yang sah",
			"request_id", ev.RequestID, "status", ev.StatusCode, "endpoint", ev.Endpoint)
		batal()
	}

	ctxBiaya, batalBiaya := r.konteksLangkah()
	biaya := r.biaya(ctxBiaya, ev)
	batalBiaya()

	insertGagal := false
	if r.store != nil {
		ctxTulis, batalTulis := r.konteksLangkah()
		pk, dibuat, err := r.store.Insert(ctxTulis, ev.record(biaya))
		switch {
		case err != nil:
			insertGagal = true
			r.hasil(OutcomeFailed)
			r.logger.ErrorContext(ctxTulis, "baris log request gagal ditulis",
				"request_id", ev.RequestID, "status", ev.StatusCode, "error", err)
		default:
			r.hasil(OutcomeWritten)
			if timeline := ev.timeline(); len(timeline) > 0 {
				ctxEvents, batalEvents := r.konteksLangkah()
				if err := r.store.AppendEvents(ctxEvents, pk, dibuat, timeline); err != nil {
					// Timeline hanya diagnostik: barisnya sudah tertulis, dan kehilangan
					// jejak percobaan tidak mengubah satu pun angka pemakaian. Tetapi
					// kegagalannya kini TERHITUNG, bukan hanya WARN: tanpa penghitung,
					// timeline inspector bisa hilang massal sementara indikator
					// pencatatan tampak hijau (audit3 V-AB-23).
					if r.metrics != nil {
						r.metrics.UsageEventFailures.Inc()
					}
					r.logger.WarnContext(ctxEvents, "timeline percobaan gagal ditulis",
						"request_id", ev.RequestID, "error", err)
				}
				batalEvents()
			}
		}
		batalTulis()
	}

	// Konsistensi log <-> anggaran (audit F6): AddSpend TETAP dijalankan
	// walau baris request gagal ditulis — biayanya nyata sudah keluar ke
	// upstream, dan membatalkan penegakan anggaran hanya karena log gagal
	// berarti anggaran bocor diam-diam saat insiden DB (keputusan sadar
	// ini juga dikunci oleh TestRecorderMencatatKegagalanPenulisan*).
	// Yang ditambahkan audit: PENANDA REKONSILIASI eksplisit per RequestID
	// agar pasangan "anggaran terpotong tanpa baris log" bisa ditemukan
	// dan diselaraskan kemudian, bukan tenggelam di log yang terpisah.
	ctxSpend, batalSpend := r.konteksLangkah()
	defer batalSpend()
	if insertGagal && biaya > 0 {
		r.logger.ErrorContext(ctxSpend, "REKONSILIASI: pemakaian anggaran tetap dinaikkan walau baris request gagal ditulis",
			"request_id", ev.RequestID, "biaya_usd", biaya.String())
	}

	r.tambahPemakaian(ctxSpend, ev, biaya)
}

// biaya menghitung biaya satu permintaan dari harga yang berlaku.
//
// Nol dikembalikan untuk dua keadaan yang berbeda dan keduanya benar: permintaan yang tidak
// memakai token sama sekali, dan pemetaan yang harganya belum diisi. Yang kedua dilaporkan
// ke log lewat laporTanpaHarga, karena biaya nol yang berarti "belum diisi" terlihat sama
// dengan gratis di setiap laporan.
func (r *Recorder) biaya(ctx context.Context, ev Event) upstream.USD {
	// Cache HIT tidak memanggil upstream sama sekali, jadi biayanya nol
	// secara desain (B-188) — BUKAN nol karena belum dihargai: keadaan ini
	// diperiksa paling awal supaya HIT tidak pernah terhitung sebagai
	// pemetaan tanpa harga, dan tidak bergantung pada absennya pemenang
	// (jalur HIT memang memasang pemenang demi atribusi, audit3 V-GW-09).
	if ev.CacheHit {
		return 0
	}
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
	// Harga yang dipakai adalah harga pada SAAT PERMINTAAN SELESAI (TerjadiPada),
	// bukan harga berjalan saat catatan ini kebetulan diproses: antrean asinkron
	// berarti dua waktu itu bisa berbeda, dan memakai yang kedua mengubah biaya
	// permintaan lampau setiap kali operator mengganti harga (audit3 V-AB-09).
	harga, ok := r.prices.PriceAt(ctx, ev.ProviderModelID, ev.TerjadiPada)
	if !ok {
		// Terhitung di metrik, bukan hanya WARN sekali per cuplikan: pemetaan yang
		// lupa dihargai membekukan anggaran USD tanpa gejala lain, jadi keadaannya
		// harus bisa dialarmkan (audit3 V-AB-10).
		if r.metrics != nil {
			r.metrics.UsageUnpriced.WithLabelValues(ev.ProviderName, ev.ModelName).Inc()
		}
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
	// Target provider disusun DI SINI, bukan di TargetsFor jalur request: provider
	// pemenang baru diketahui sesudah eksekusi selesai, dan tanpanya anggaran
	// cakupan provider tidak pernah terisi spent-nya (audit3 V-AB-04). Daftar
	// disalin dulu — ev.Targets milik bersama dan menempelkan di tempat akan
	// mengubah irisan milik pemanggil.
	targets := ev.Targets
	if ev.ProviderID != "" {
		targets = make([]policy.Target, 0, len(ev.Targets)+1)
		targets = append(targets, ev.Targets...)
		targets = append(targets, policy.Target{Scope: policy.ScopeProvider, ID: ev.ProviderID})
	}
	if _, err := r.spend.AddSpend(ctx, targets, biaya); err != nil {
		if r.metrics != nil {
			r.metrics.UsageSpendFailures.Inc()
		}
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
