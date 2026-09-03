package usage

import (
	"context"
	"log/slog"
	"sync"
	"time"

	"github.com/NexGen-X/Route-X/internal/database/repo/traffic"
	"github.com/NexGen-X/Route-X/internal/observability"
	"github.com/NexGen-X/Route-X/internal/router"
)

// Nilai bawaan Index.
const (
	// defaultIndexWindow adalah lebar jendela pengukuran.
	//
	// Tiga puluh menit: cukup panjang untuk mengumpulkan sampel yang bermakna pada lalu
	// lintas sedang, cukup pendek untuk mengikuti provider yang baru melambat. Jendela satu
	// jam membuat provider yang sudah pulih tetap terlihat lambat selama setengah jam
	// berikutnya; jendela lima menit membuat urutan routing bergoyang mengikuti derau.
	defaultIndexWindow = 30 * time.Minute

	// defaultIndexInterval adalah jeda antar penyegaran.
	//
	// Kuerinya membaca baris requests mentah dan menghitung persentil per pemetaan, jadi ia
	// bukan kueri murah. Sekali per menit sudah jauh lebih cepat daripada perubahan yang
	// diukurnya — latensi p95 pada jendela 30 menit tidak bergerak berarti dalam satu menit.
	defaultIndexInterval = time.Minute

	// defaultIndexPercentile adalah persentil yang dipakai strategi lowest_latency.
	//
	// p95, bukan rata-rata: rata-rata menyembunyikan provider yang biasanya cepat tetapi
	// kadang menggantung, dan justru ekor itulah yang dirasakan pengguna.
	defaultIndexPercentile = 0.95

	// defaultIndexMinSamples adalah jumlah sampel minimum sebelum sebuah pemetaan dianggap
	// terukur.
	//
	// Sepuluh. Di bawah itu p95 praktis sama dengan nilai terbesar yang kebetulan terjadi,
	// dan memperlakukannya sebagai pengukuran akan mengalihkan seluruh lalu lintas ke
	// provider yang baru melayani satu permintaan cepat. Pemetaan yang tersaring dilaporkan
	// sebagai "belum terukur", dan paket router mengurutkannya paling belakang.
	defaultIndexMinSamples = 10

	// maxAvailabilityProviders membatasi jumlah provider yang gauge-nya diperbarui.
	//
	// Batasnya ada supaya satu pemasangan dengan provider yang sangat banyak tidak
	// meledakkan jumlah time series lewat jalur ini. Angkanya jauh di atas jumlah provider
	// yang wajar dimiliki satu pemasangan.
	maxAvailabilityProviders = 200
)

// LatencyStore memasok angka terukur dari log request. Dipenuhi *traffic.Repo.
type LatencyStore interface {
	LatencyByProviderModel(ctx context.Context, window time.Duration, percentile float64, minSamples int) (map[string]time.Duration, error)
	Breakdown(ctx context.Context, q traffic.Query, dim traffic.Dimension, limit int) ([]traffic.Slice, error)
}

// Index adalah cuplikan angka yang diturunkan dari log request dan dibaca jalur permintaan.
//
// Dua pemakainya menuntut sifat yang sama dan itulah sebabnya keduanya di satu tempat:
// strategi routing lowest_latency membaca latensi p95 per pemetaan model-provider, dan
// metrik ketersediaan membaca rasio keberhasilan per provider. Keduanya jawaban atas
// pertanyaan "apa yang benar-benar terjadi belakangan ini", keduanya mahal dihitung, dan
// keduanya tidak boleh dihitung di dalam permintaan.
//
// Karena itu pembacaannya TIDAK PERNAH menyentuh database: satu goroutine penyegar
// memperbarui cuplikan secara berkala, dan pembaca hanya melihat peta di memori. Kalau
// penyegar belum pernah berhasil, seluruh pemetaan dilaporkan "belum terukur" — yang oleh
// paket router diurutkan paling belakang, sehingga hasilnya urutan prioritas. Itu perilaku
// yang benar dan aman; yang tidak boleh terjadi adalah mengarang angka atau menahan
// permintaan sampai angkanya ada.
//
// Aman dipakai bersamaan oleh banyak goroutine.
type Index struct {
	store   LatencyStore
	metrics *observability.Metrics
	logger  *slog.Logger

	window     time.Duration
	interval   time.Duration
	percentile float64
	minSamples int

	mu       sync.RWMutex
	latensi  map[string]time.Duration
	tersedia map[string]float64
	// namaProvider memetakan id provider ke namanya, dipakai sebagai label metrik.
	//
	// Labelnya NAMA, bukan id, supaya sederet metrik gateway bisa dibaca bersama: setiap
	// metrik lain memakai nama provider, dan gauge yang memakai UUID memaksa pembaca
	// dashboard menyambungkan dua kosakata sendiri. Kuncinya tetap id di dalam Index —
	// pemanggil program memegang id, dan dua provider bisa berganti nama tanpa berganti id.
	namaProvider map[string]string
	disegarkan   time.Time
}

// IndexOption menyetel Index.
type IndexOption func(*Index)

// WithIndexWindow mengubah lebar jendela pengukuran. Nilai <= 0 diabaikan.
func WithIndexWindow(d time.Duration) IndexOption {
	return func(ix *Index) {
		if d > 0 {
			ix.window = d
		}
	}
}

// WithIndexInterval mengubah jeda penyegaran. Nilai <= 0 diabaikan.
func WithIndexInterval(d time.Duration) IndexOption {
	return func(ix *Index) {
		if d > 0 {
			ix.interval = d
		}
	}
}

// WithIndexMinSamples mengubah jumlah sampel minimum. Nilai < 1 diabaikan.
func WithIndexMinSamples(n int) IndexOption {
	return func(ix *Index) {
		if n >= 1 {
			ix.minSamples = n
		}
	}
}

// NewIndex membuat cuplikan kosong.
//
// store nil sah dan berarti tidak ada yang pernah terukur: Refresh tidak melakukan apa pun
// dan LatencyP95 selalu melapor "belum terukur".
func NewIndex(store LatencyStore, metrics *observability.Metrics, logger *slog.Logger, opts ...IndexOption) *Index {
	if logger == nil {
		logger = slog.New(slog.DiscardHandler)
	}
	ix := &Index{
		store: store, metrics: metrics, logger: logger,
		window:       defaultIndexWindow,
		interval:     defaultIndexInterval,
		percentile:   defaultIndexPercentile,
		minSamples:   defaultIndexMinSamples,
		latensi:      map[string]time.Duration{},
		tersedia:     map[string]float64{},
		namaProvider: map[string]string{},
	}
	for _, o := range opts {
		if o != nil {
			o(ix)
		}
	}
	return ix
}

// Kontrak yang diandalkan main saat memasang sumber latensi ke router.
var _ router.LatencySource = (*Index)(nil)

// LatencyP95 memenuhi router.LatencySource.
//
// ok false berarti pemetaan itu belum punya pengukuran yang cukup. Pemanggil WAJIB
// memperlakukannya sebagai "belum terukur" dan mengurutkannya paling belakang, bukan
// sebagai nol — nol adalah nilai tercepat yang bisa ada, dan menganggapnya angka sungguhan
// akan mengirim seluruh lalu lintas ke pemetaan yang paling baru ditambahkan.
func (ix *Index) LatencyP95(providerModelID string) (time.Duration, bool) {
	if ix == nil || providerModelID == "" {
		return 0, false
	}
	ix.mu.RLock()
	defer ix.mu.RUnlock()
	d, ok := ix.latensi[providerModelID]
	return d, ok
}

// Availability mengembalikan rasio keberhasilan satu provider pada jendela terakhir.
//
// ok false berarti provider itu tidak melayani satu permintaan pun di jendela ini, yang
// berbeda dari rasio nol — dan bedanya menentukan: yang pertama berarti tidak ada data,
// yang kedua berarti setiap permintaan gagal.
func (ix *Index) Availability(providerID string) (float64, bool) {
	if ix == nil || providerID == "" {
		return 0, false
	}
	ix.mu.RLock()
	defer ix.mu.RUnlock()
	v, ok := ix.tersedia[providerID]
	return v, ok
}

// RefreshedAt mengembalikan kapan cuplikan terakhir berhasil diperbarui. Waktu nol berarti
// belum pernah.
func (ix *Index) RefreshedAt() time.Time {
	if ix == nil {
		return time.Time{}
	}
	ix.mu.RLock()
	defer ix.mu.RUnlock()
	return ix.disegarkan
}

// Refresh memperbarui cuplikan sekali.
//
// Kegagalan MEMPERTAHANKAN cuplikan sebelumnya dan dikembalikan supaya pemanggil bisa
// mencatatnya. Membuang cuplikan ketika sumbernya sedang tidak bisa dibaca berarti gangguan
// sementara pada database mengubah cara seluruh lalu lintas dirutekan — perubahan perilaku
// yang jauh lebih besar akibatnya daripada angka yang berumur beberapa menit lebih tua.
func (ix *Index) Refresh(ctx context.Context) error {
	if ix == nil || ix.store == nil {
		return nil
	}

	latensi, err := ix.store.LatencyByProviderModel(ctx, ix.window, ix.percentile, ix.minSamples)
	if err != nil {
		return err
	}

	q := traffic.Query{
		From:   time.Now().Add(-ix.window),
		To:     time.Now(),
		Source: traffic.SourceRequests,
	}
	potongan, err := ix.store.Breakdown(ctx, q, traffic.DimProvider, maxAvailabilityProviders)
	if err != nil {
		return err
	}

	tersedia := make(map[string]float64, len(potongan))
	nama := make(map[string]string, len(potongan))
	for _, p := range potongan {
		// Potongan tanpa provider adalah permintaan yang ditolak sebelum satu pun provider
		// terpilih. Itu request yang benar-benar terjadi dan Summary tetap menghitungnya,
		// tetapi ia bukan ketersediaan provider mana pun.
		if p.ID == "" {
			continue
		}
		if rasio, ada := p.Availability(); ada {
			tersedia[p.ID] = rasio
			// Nama provider bisa kosong pada baris yang sangat lama, dan label kosong tidak
			// bisa dipetakan kembali ke apa pun. Id-nya lebih buruk untuk dibaca tetapi
			// selalu bisa dicari.
			nama[p.ID] = p.Name
			if p.Name == "" {
				nama[p.ID] = p.ID
			}
		}
	}

	ix.mu.Lock()
	lamaGauge := ix.labelGaugeTerkunci()
	ix.latensi, ix.tersedia, ix.namaProvider = latensi, tersedia, nama
	ix.disegarkan = time.Now()
	baruGauge := ix.labelGaugeTerkunci()
	ix.mu.Unlock()

	ix.perbaruiGauge(lamaGauge, baruGauge)
	return nil
}

// labelGaugeTerkunci menyusun peta label gauge (nama provider) ke rasionya.
// Pemanggil WAJIB memegang mu.
func (ix *Index) labelGaugeTerkunci() map[string]float64 {
	out := make(map[string]float64, len(ix.tersedia))
	for id, rasio := range ix.tersedia {
		label := ix.namaProvider[id]
		if label == "" {
			label = id
		}
		out[label] = rasio
	}
	return out
}

// perbaruiGauge memasang gauge ketersediaan dan MENGHAPUS provider yang keluar dari jendela.
//
// Penghapusannya bukan kerapian. Gauge yang tertinggal akan melaporkan angka terakhirnya
// selamanya, jadi provider yang berhenti menerima lalu lintas terlihat tetap 100% sehat —
// atau tetap 0% rusak — dan alert yang dibangun di atasnya menjadi salah dalam kedua arah.
func (ix *Index) perbaruiGauge(lama, baru map[string]float64) {
	if ix.metrics == nil {
		return
	}
	for id, rasio := range baru {
		ix.metrics.ProviderAvailability.WithLabelValues(id).Set(rasio)
	}
	for id := range lama {
		if _, masih := baru[id]; !masih {
			ix.metrics.ProviderAvailability.DeleteLabelValues(id)
		}
	}
}

// Run menyegarkan cuplikan sampai ctx selesai.
//
// Penyegaran pertama dijalankan SEKARANG, bukan setelah satu tick: kalau menunggu tick,
// seluruh permintaan pada menit pertama setelah start dirutekan tanpa satu pun angka
// latensi, dan lowest_latency berperilaku seperti priority tepat pada saat operator baru
// menyalakan gateway dan sedang memperhatikannya.
func (ix *Index) Run(ctx context.Context) {
	if ix == nil || ix.store == nil {
		return
	}
	if err := ix.Refresh(ctx); err != nil && ctx.Err() == nil {
		ix.logger.WarnContext(ctx, "penyegaran cuplikan latensi gagal", "error", err)
	}

	t := time.NewTicker(ix.interval)
	defer t.Stop()
	for {
		select {
		case <-ctx.Done():
			return
		case <-t.C:
			if err := ix.Refresh(ctx); err != nil && ctx.Err() == nil {
				ix.logger.WarnContext(ctx, "penyegaran cuplikan latensi gagal", "error", err)
			}
		}
	}
}
