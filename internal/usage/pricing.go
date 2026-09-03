package usage

import (
	"context"
	"log/slog"
	"sync"
	"time"

	"github.com/NexGen-X/Route-X/internal/database/repo/upstream"
	"github.com/NexGen-X/Route-X/internal/providers"
	"github.com/NexGen-X/Route-X/internal/router"
)

// defaultPriceTTL adalah umur cuplikan harga di memori.
//
// Lebih pendek daripada cache lain di proyek ini walaupun harga jauh lebih jarang berubah,
// dan alasannya justru itu: yang dibayar keterlambatan di sini adalah UANG. Setelah
// operator mengubah harga, paling lama satu TTL lalu lintas masih dihitung dengan harga
// sebelumnya, dan angka itu tersimpan permanen di requests.cost_usd. Tiga puluh detik
// adalah batas yang bisa dikatakan apa adanya kepada operator; memperpanjangnya menukar
// ketepatan tagihan dengan penghematan satu query per menit.
//
// API admin Fase 11 memanggil Invalidate setelah mengubah harga, sehingga keterlambatan itu
// hanya berlaku bagi instance LAIN — batas yang sama seperti cache aturan routing.
const defaultPriceTTL = 30 * time.Second

// PriceSource memasok seluruh harga yang sedang berlaku. Dipenuhi *upstream.PricingRepo.
type PriceSource interface {
	CurrentAll(ctx context.Context) ([]*upstream.Price, error)
}

// Pricer menjawab dua pertanyaan dari satu cuplikan harga: berapa biaya request yang sudah
// selesai, dan kandidat mana yang termurah untuk request yang belum berjalan.
//
// Keduanya dilayani cuplikan yang sama karena keduanya menanyakan hal yang sama, tetapi
// jalurnya berbeda dan itu menentukan bentuk API di sini:
//
//   - Price dipanggil pencatat pemakaian, yang berjalan SETELAH jawaban terkirim. Di sana
//     satu query ke database boleh terjadi, jadi Price memuat ulang cuplikan yang sudah
//     kedaluwarsa.
//   - Cost dipanggil router, pada setiap kandidat, SEBELUM satu byte dikirim ke provider.
//     Di sana query dilarang, jadi Cost hanya membaca cuplikan yang ada — dan bila belum
//     ada, ia melapor "tidak diketahui" alih-alih menahan permintaan. Nilai yang tidak
//     diketahui diurutkan paling belakang oleh paket router, jadi kegagalan pemuatan
//     berakibat urutan prioritas, bukan urutan yang dikarang.
//
// Warm dipanggil saat start supaya permintaan pertama sudah punya harga, mengikuti pola
// yang sama dengan pemanasan skrip pemutus arus dan pembatas laju.
//
// Aman dipakai bersamaan oleh banyak goroutine.
type Pricer struct {
	source PriceSource
	logger *slog.Logger
	ttl    time.Duration
	now    func() time.Time

	// muat menyatukan pemuatan bersamaan menjadi satu query, sehingga cuplikan yang
	// kedaluwarsa di bawah beban tidak menghasilkan satu query per permintaan.
	muat sync.Mutex

	mu       sync.RWMutex
	harga    map[string]*upstream.Price
	segarDi  time.Time
	terbaca  bool
	tanpaHrg map[string]struct{}
}

// PricerOption menyetel Pricer.
type PricerOption func(*Pricer)

// WithPriceTTL mengubah umur cuplikan harga. Nilai <= 0 diabaikan.
func WithPriceTTL(d time.Duration) PricerOption {
	return func(p *Pricer) {
		if d > 0 {
			p.ttl = d
		}
	}
}

// WithPriceClock mengganti sumber waktu, untuk test kedaluwarsa cuplikan.
func WithPriceClock(now func() time.Time) PricerOption {
	return func(p *Pricer) {
		if now != nil {
			p.now = now
		}
	}
}

// NewPricer membuat Pricer.
//
// source nil sah dan berarti tidak ada harga yang diketahui: biaya setiap request tercatat
// nol dan lowest_cost berperilaku seperti priority. Itu keadaan yang benar untuk pemasangan
// yang belum mengisi tabel harga, dan lebih baik daripada menolak permintaan karena
// harganya belum ada.
func NewPricer(source PriceSource, logger *slog.Logger, opts ...PricerOption) *Pricer {
	if logger == nil {
		logger = slog.New(slog.DiscardHandler)
	}
	p := &Pricer{
		source:   source,
		logger:   logger,
		ttl:      defaultPriceTTL,
		now:      time.Now,
		harga:    map[string]*upstream.Price{},
		tanpaHrg: map[string]struct{}{},
	}
	for _, o := range opts {
		if o != nil {
			o(p)
		}
	}
	return p
}

// Kontrak yang diandalkan main saat memasang sumber biaya ke router.
var _ router.CostSource = (*Pricer)(nil)

// Warm memuat cuplikan harga sekarang.
//
// Kegagalannya dikembalikan supaya pemanggil bisa mencatatnya, tetapi TIDAK boleh
// menghentikan start: gateway yang menolak menyala karena tabel harga belum bisa dibaca
// menukar seluruh lalu lintas dengan ketepatan laporan biaya.
func (p *Pricer) Warm(ctx context.Context) error { return p.muatUlang(ctx) }

// Invalidate memaksa pemuatan ulang pada pemakaian berikutnya.
func (p *Pricer) Invalidate() {
	p.mu.Lock()
	p.segarDi = time.Time{}
	p.mu.Unlock()
}

// Price mengembalikan harga yang berlaku untuk satu pemetaan model-provider.
//
// ok false berarti pemetaan itu belum punya harga. Pemanggil WAJIB membedakannya dari harga
// nol: biaya nol yang berarti "gratis" dan biaya nol yang berarti "belum diisi" terlihat
// sama di laporan, dan hanya yang kedua yang menuntut tindakan operator.
func (p *Pricer) Price(ctx context.Context, providerModelID string) (*upstream.Price, bool) {
	if p == nil || providerModelID == "" {
		return nil, false
	}
	p.segarkan(ctx)

	p.mu.RLock()
	defer p.mu.RUnlock()
	pr, ok := p.harga[providerModelID]
	return pr, ok
}

// Cost memenuhi router.CostSource: biaya perkiraan menjalankan est pada satu pemetaan.
//
// Tidak menyentuh database dan tidak pernah menunggu — lihat catatan tipe. Perkiraan token
// yang kosong tetap dijawab dengan membandingkan harga satuan untuk satu juta token input
// plus satu juta token output, karena "termurah" tanpa perkiraan pemakaian tetap harus
// punya jawaban, dan perbandingan pada volume yang sama untuk semua kandidat adalah jawaban
// yang paling tidak menyesatkan.
func (p *Pricer) Cost(providerModelID string, est router.TokenEstimate) (upstream.USD, bool) {
	if p == nil || providerModelID == "" {
		return 0, false
	}

	p.mu.RLock()
	pr, ok := p.harga[providerModelID]
	p.mu.RUnlock()
	if !ok {
		return 0, false
	}

	tokens := upstream.TokenUsage{
		Input:  int64(max(est.InputTokens, 0)),
		Output: int64(max(est.OutputTokens, 0)),
	}
	if est.IsZero() {
		tokens = upstream.TokenUsage{Input: 1_000_000, Output: 1_000_000}
	}

	b, err := pr.Cost(tokens)
	if err != nil {
		// Harga yang tidak bisa dipakai menghitung sama saja dengan harga yang tidak ada:
		// kandidatnya diurutkan paling belakang alih-alih dianggap termurah.
		return 0, false
	}
	return b.Total, true
}

// segarkan memuat ulang cuplikan bila sudah kedaluwarsa.
func (p *Pricer) segarkan(ctx context.Context) {
	if p.source == nil {
		return
	}

	p.mu.RLock()
	segar := p.now().Before(p.segarDi)
	p.mu.RUnlock()
	if segar {
		return
	}

	p.muat.Lock()
	defer p.muat.Unlock()

	p.mu.RLock()
	segar = p.now().Before(p.segarDi)
	p.mu.RUnlock()
	if segar {
		return
	}
	if err := p.muatUlang(ctx); err != nil {
		p.logger.WarnContext(ctx, "harga model gagal dibaca, memakai cuplikan sebelumnya",
			"error", err, "pernah_terbaca", p.pernahTerbaca())
	}
}

// muatUlang membaca seluruh harga berlaku lalu menggantikan cuplikan.
//
// Kegagalan MEMPERTAHANKAN cuplikan lama dan tidak memperbarui waktu segar, sehingga
// percobaan berikutnya tidak menunggu satu TTL penuh — alasan yang sama seperti di
// internal/ratelimit.
func (p *Pricer) muatUlang(ctx context.Context) error {
	if p.source == nil {
		return nil
	}
	rows, err := p.source.CurrentAll(ctx)
	if err != nil {
		return err
	}

	baru := make(map[string]*upstream.Price, len(rows))
	for _, row := range rows {
		baru[row.ProviderModelID] = row
	}

	p.mu.Lock()
	p.harga = baru
	p.segarDi = p.now().Add(p.ttl)
	p.terbaca = true
	// Daftar pemetaan tanpa harga dilepas bersama cuplikan: harga yang baru diisi harus
	// bisa dilaporkan lagi, kalau tidak peringatannya hilang selamanya setelah sekali muncul.
	p.tanpaHrg = map[string]struct{}{}
	p.mu.Unlock()
	return nil
}

// pernahTerbaca melaporkan apakah cuplikan yang ada berasal dari pembacaan yang berhasil.
func (p *Pricer) pernahTerbaca() bool {
	p.mu.RLock()
	defer p.mu.RUnlock()
	return p.terbaca
}

// laporTanpaHarga mencatat pemetaan yang belum punya harga, satu kali per cuplikan.
//
// Sekali per cuplikan, bukan sekali per permintaan: pemetaan yang harganya lupa diisi akan
// melayani ribuan permintaan, dan satu baris log per permintaan membuat pesan ini tenggelam
// justru karena terlalu sering muncul. Dilaporkan sebagai WARN karena akibatnya laporan
// biaya yang terlihat wajar padahal kurang — kegagalan yang tidak punya gejala lain.
func (p *Pricer) laporTanpaHarga(ctx context.Context, providerModelID, provider, model string) {
	if p == nil || providerModelID == "" {
		return
	}
	p.mu.Lock()
	_, sudah := p.tanpaHrg[providerModelID]
	if !sudah {
		p.tanpaHrg[providerModelID] = struct{}{}
	}
	p.mu.Unlock()
	if sudah {
		return
	}
	p.logger.WarnContext(ctx, "pemetaan model-provider belum punya harga, biaya tercatat nol",
		"provider", provider, "model", model, "provider_model_id", providerModelID)
}

// TokensFor menerjemahkan pemakaian token dari bentuk kanonik ke bentuk penagihan.
//
// Ini konversi yang paling mudah salah di seluruh jalur biaya, dan salahnya tidak
// menghasilkan satu pun error — hanya tagihan yang keliru.
//
// providers.Usage mengikuti konvensi OpenAI, dan konvensi itu BERSARANG: InputTokens sudah
// memuat CachedInputTokens, dan OutputTokens sudah memuat ReasoningTokens. Bentuk itu
// disengaja, karena itulah bentuk yang diteruskan apa adanya ke klien di body respons.
// upstream.TokenUsage sebaliknya menuntut keempat angkanya SALING LEPAS, karena setiap
// bagian dikalikan harganya sendiri. Menyerahkan angka bersarang apa adanya berarti token
// cache ditagih dua kali — sekali dengan harga input penuh, sekali dengan harga cache — dan
// token penalaran juga dua kali.
//
// Angka yang tidak konsisten (bagian lebih besar daripada keseluruhannya) dijepit alih-alih
// ditolak: satu laporan usage yang aneh dari satu provider tidak boleh membuat seluruh baris
// pemakaian hilang. Arah jepitannya menjaga JUMLAH token tetap sama seperti yang dilaporkan
// provider, dan sisi harga yang lebih murah yang menang — lebih baik menagih kurang karena
// laporan provider aneh daripada menagih lebih.
func TokensFor(u providers.Usage) upstream.TokenUsage {
	input, cached := int64(max(u.InputTokens, 0)), int64(max(u.CachedInputTokens, 0))
	if cached > input {
		cached = input
	}
	output, reasoning := int64(max(u.OutputTokens, 0)), int64(max(u.ReasoningTokens, 0))
	if reasoning > output {
		reasoning = output
	}
	return upstream.TokenUsage{
		Input:       input - cached,
		CachedInput: cached,
		Output:      output - reasoning,
		Reasoning:   reasoning,
	}
}
