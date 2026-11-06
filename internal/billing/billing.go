// Package billing menegakkan anggaran biaya: menolak permintaan baru ketika pemakaian satu
// cakupan sudah melewati batas uang yang ditetapkan operator.
//
// Perhitungan biayanya sendiri BUKAN milik paket ini di Fase 8. Yang menaikkan angka
// pemakaian adalah pencatat usage di Fase 9, lewat policy.Repo.AddSpend. Sampai itu ada,
// penegakan di sini bekerja atas kolom budgets.spent_usd yang masih nol, sehingga
// akibatnya nyata hanya untuk anggaran yang diisi dari luar. Itu disebutkan supaya tidak
// ada yang menyangka anggaran sudah menahan biaya sebelum pencatatannya terpasang.
//
// # Kenapa keterlambatan beberapa detik diterima
//
// Anggaran dibaca dari salinan ber-TTL, sama seperti batas laju dan aturan routing, dengan
// alasan yang sama: tanpa pub/sub, satu instance tidak bisa membatalkan cache instance lain.
// Akibatnya pemakaian bisa melewati batas sebesar lalu lintas selama satu TTL. Itu memang
// sifat anggaran biaya di sistem terdistribusi — angka pemakaiannya sendiri sudah dihitung
// setelah permintaan selesai, jadi batas yang benar-benar tepat tidak pernah bisa dijanjikan
// tanpa mengunci setiap permintaan pada satu penghitung pusat.
//
// # Anggaran adalah gerbang, bukan urutan
//
// Berbeda dari batas laju yang punya jendela dan waktu pulih, anggaran yang habis tidak
// pulih sampai periodenya berganti. Karena itu penolakannya BUKAN 429: klien yang mengulang
// tidak akan pernah berhasil sebelum periode baru dimulai, dan mengirim Retry-After yang
// menunjuk ke pergantian bulan hanya membuat pustaka klien tidur berhari-hari.
package billing

import (
	"context"
	"log/slog"
	"sync"
	"time"

	"github.com/NexGen-X/Route-X/internal/database/repo/policy"
	"github.com/NexGen-X/Route-X/internal/database/repo/upstream"
)

// defaultTTL adalah umur salinan anggaran di memori.
const defaultTTL = 5 * time.Second

// HoldTTLBawaan adalah umur bawaan satu tahanan reservasi anggaran
// (lihat Tahan).
//
// Tahanan adalah jembatan antara pemeriksaan pra-permintaan dan pencatatan
// pasca-bayar: ia harus hidup lebih lama dari permintaan terlama yang sah,
// tetapi cukup pendek supaya tahanan yang bocor — permintaan yang lolos
// gerbang lalu mati sebelum tercatat — tidak menahan anggaran selamanya.
//
// Batas permintaan terlama yang sah saat ini adalah anggaran waktu TOTAL
// rencana non-streaming di internal/gateway (batasTotalRencana, 5 menit);
// streaming totalnya tidak dibatasi angka itu (yang dijaga tenggat diam
// per jeda). Dua menit — nilai lama — GUGUR sebelum batas rencana itu
// tercapai: tahanan permintaan lambat hilang di tengah jalan, spent-nya
// belum mendarat, dan pemeriksaan berikutnya tidak melihatnya sama sekali,
// sehingga pagu terlampaui oleh permintaan paralel (B-189). Sepuluh menit
// adalah dua kali batas rencana: cukup longgar untuk permintaan terlama
// yang sah plus margin, dan kebocoran tahanan tetap berumur terbatas.
// Relasi "TTL tahanan > batas total rencana" dikunci oleh test di paket
// gateway (TestBatasRencanaDiBawahTTLTahanB189): menaikkan batas rencana
// melewati angka ini tanpa menaikkan TTL menghidupkan lagi B-189.
const HoldTTLBawaan = 10 * time.Minute

// Source memasok anggaran aktif dan menandai peringatan. Dipenuhi *policy.Repo.
type Source interface {
	ActiveBudgets(ctx context.Context) ([]*policy.Budget, error)
	MarkAlerted(ctx context.Context, id string) (bool, error)
	MarkThresholdAlerted(ctx context.Context, id string) (bool, error)
	MarkExceededAlerted(ctx context.Context, id string) (bool, error)
}

// Verdict adalah hasil pemeriksaan anggaran satu permintaan.
type Verdict struct {
	// Blocked true berarti permintaan harus ditolak.
	Blocked bool
	// Budget adalah anggaran yang memblokir. nil bila tidak ada yang memblokir.
	Budget *policy.Budget
	// Alerts memuat anggaran yang baru melewati ambang peringatannya dan belum pernah
	// diberitahukan. Terisi baik saat diblokir maupun tidak: ambang peringatan justru
	// gunanya untuk memberi tahu SEBELUM anggaran habis.
	Alerts []*policy.Budget
}

// WebhookEnqueuer adalah antarmuka untuk memasukkan event notifikasi ke antrean webhook.
type WebhookEnqueuer interface {
	Enqueue(ctx context.Context, event string, payload any) (int, error)
}

// tahanan adalah satu reservasi anggaran: estimasi biaya satu permintaan yang
// sudah lolos gerbang tetapi belum tercatat sebagai spent.
type tahanan struct {
	jumlah upstream.USD
	sampai time.Time
}

// Enforcer menyimpan salinan anggaran dan menjawab apakah permintaan boleh jalan.
//
// Aman dipakai bersamaan oleh banyak goroutine.
type Enforcer struct {
	source   Source
	enqueuer WebhookEnqueuer
	logger   *slog.Logger
	ttl      time.Duration
	ttlTahan time.Duration
	now      func() time.Time

	mu     sync.RWMutex
	dimuat time.Time
	// global adalah anggaran bercakupan global.
	global []*policy.Budget
	// perCakupan diindeks per [scope][scope_id].
	perCakupan map[string]map[string][]*policy.Budget
	// tahan adalah reservasi per anggaran (kunci: budget ID) dari permintaan
	// yang sedang berjalan. Dijaga mutex yang sama dengan salinan, karena
	// keputusan "boleh jalan + tahan" harus atomik terhadap keputusan
	// bersamaan lain — itulah seluruh gunanya (lihat Tahan).
	tahan map[string][]tahanan
	// pemuatan menyatukan pemuatan ulang yang bersamaan menjadi satu query:
	// tanpa ini, salinan yang kedaluwarsa di bawah beban menghasilkan satu
	// query ActiveBudgets per permintaan yang datang dalam jendela pemuatan —
	// persis saat database paling tidak butuh tambahan beban.
	pemuatan sync.Mutex
	// gagalMuat adalah waktu kegagalan pemuatan terakhir (dijaga mu), dasar
	// cache negatif di muat: sesudah gagal, percobaan berikutnya menunggu
	// jedaGagalMuat alih-alih mengantre di mutex yang sama pada SETIAP
	// request (audit3 V-LN-11).
	gagalMuat time.Time
}

// Option adalah penyetel opsional penegak anggaran.
type Option func(*Enforcer)

// WithWebhookEnqueuer memasang antrean webhook untuk notifikasi budget.threshold dan budget.exceeded.
func WithWebhookEnqueuer(enqueuer WebhookEnqueuer) Option {
	return func(e *Enforcer) {
		e.enqueuer = enqueuer
	}
}

// WithTTL mengubah umur salinan anggaran. Nilai <= 0 diabaikan.
// Seam pengujian yang disengaja (triage B-005): produksi memakai nilai bawaan; opsi ini ada agar test dapat menyetel perilaku secara deterministik.
func WithTTL(d time.Duration) Option {
	return func(e *Enforcer) {
		if d > 0 {
			e.ttl = d
		}
	}
}

// WithClock mengganti sumber waktu, untuk test kedaluwarsa salinan.
// Seam pengujian yang disengaja (triage B-005): produksi memakai nilai bawaan; opsi ini ada agar test dapat menyetel perilaku secara deterministik.
func WithClock(now func() time.Time) Option {
	return func(e *Enforcer) {
		if now != nil {
			e.now = now
		}
	}
}

// WithHoldTTL mengubah umur tahanan reservasi anggaran. Nilai <= 0 diabaikan.
// Seam pengujian yang disengaja (triage B-005): produksi memakai nilai bawaan; opsi ini ada agar test dapat menyetel perilaku secara deterministik.
func WithHoldTTL(d time.Duration) Option {
	return func(e *Enforcer) {
		if d > 0 {
			e.ttlTahan = d
		}
	}
}

// NewEnforcer membuat penegak anggaran.
//
// source nil sah dan berarti tidak ada anggaran yang ditegakkan — keadaan yang benar untuk
// pemasangan yang belum membuat satu pun anggaran.
func NewEnforcer(source Source, logger *slog.Logger, opts ...Option) *Enforcer {
	if logger == nil {
		logger = slog.New(slog.DiscardHandler)
	}
	e := &Enforcer{source: source, logger: logger, ttl: defaultTTL, ttlTahan: HoldTTLBawaan, now: time.Now}
	for _, o := range opts {
		if o != nil {
			o(e)
		}
	}
	return e
}

// Invalidate memaksa pembacaan ulang pada pemakaian berikutnya.
func (e *Enforcer) Invalidate() {
	e.mu.Lock()
	e.dimuat = time.Time{}
	e.mu.Unlock()
}

// Check memeriksa seluruh anggaran yang berlaku bagi target-target ini.
//
// Yang dilaporkan sebagai pemblokir adalah anggaran dengan SISA PALING SEDIKIT di antara
// yang memblokir, bukan yang pertama ditemukan. Alasannya bisa ditindaklanjuti: kalau
// anggaran bulanan global dan anggaran harian satu key sama-sama habis, yang perlu diketahui
// pelanggan adalah yang paling mengikat, karena melonggarkan yang lain tidak menolong.
func (e *Enforcer) Check(ctx context.Context, targets []policy.Target) Verdict {
	e.muat(ctx)

	e.mu.RLock()
	defer e.mu.RUnlock()

	var v Verdict
	periksa := func(b *policy.Budget) {
		if b.ShouldAlertThreshold() || b.ShouldAlertExceeded() {
			v.Alerts = append(v.Alerts, b)
		}
		if !b.Blocks() {
			return
		}
		if v.Budget == nil || b.LimitUSD-b.SpentUSD < v.Budget.LimitUSD-v.Budget.SpentUSD {
			v.Blocked, v.Budget = true, b
		}
	}

	for _, b := range e.global {
		periksa(b)
	}
	for _, t := range targets {
		if t.Scope == policy.ScopeGlobal || t.ID == "" {
			continue
		}
		for _, b := range e.perCakupan[t.Scope][t.ID] {
			periksa(b)
		}
	}
	return v
}

// Tahan memeriksa anggaran DAN menahan estimasi biaya permintaan ini dalam
// satu langkah atomik.
//
// Check saja tidak cukup untuk lalu lintas bersamaan (audit3 V-AB-16):
// pemeriksaan bersifat pra-permintaan atas spent yang dicatat pasca-bayar,
// jadi sepuluh permintaan paralel membaca sisa yang sama, semuanya lolos, dan
// pagu terlampaui sebesar konkurensinya (terbukti 6,3×). Tahan menutup celah
// itu dengan RESERVASI: permintaan yang lolos mengunci estimasi biayanya pada
// setiap anggaran blokir yang berlaku, dan pemeriksaan berikutnya menghitung
// spent + tahanan aktif sebagai pemakaian. Keputusan dan pemasangan tahanan
// berjalan di bawah satu kunci tulis — memeriksa lalu menahan dalam dua
// langkah terpisah akan membuka kembali celah yang sama.
//
// Tahanan berumur ttlTahan lalu gugur sendiri. Sesudah AddSpend nyata
// mendarat dan salinan menyegar, satu permintaan bisa terhitung dua kali
// (spent + tahanannya) sampai tahanannya gugur — arah salahnya konservatif
// (menolak lebih awal di dekat pagu, tidak pernah meloloskan melewatinya)
// dan jendelanya dibatasi umur tahanan.
//
// jumlah <= 0 berarti pemanggil tidak punya estimasi (mis. pemetaan belum
// dihargai — yang memang menagih nol): jawabannya persis Check, tanpa tahanan.
func (e *Enforcer) Tahan(ctx context.Context, targets []policy.Target, jumlah upstream.USD) Verdict {
	if jumlah <= 0 {
		return e.Check(ctx, targets)
	}
	e.muat(ctx)

	e.mu.Lock()
	defer e.mu.Unlock()

	sekarang := e.now()
	e.sapuTahanTerkunci(sekarang)

	var (
		v             Verdict
		sisaPemblokir upstream.USD
		berlaku       []*policy.Budget
	)
	periksa := func(b *policy.Budget) {
		berlaku = append(berlaku, b)
		if b.ShouldAlertThreshold() || b.ShouldAlertExceeded() {
			v.Alerts = append(v.Alerts, b)
		}
		if !b.Enabled || b.ActionOnExceed != policy.ActionBlock {
			return
		}
		sisa := b.LimitUSD - b.SpentUSD - e.totalTahanTerkunci(b.ID, sekarang)
		if sisa < jumlah && (v.Budget == nil || sisa < sisaPemblokir) {
			v.Blocked, v.Budget = true, b
			sisaPemblokir = sisa
		}
	}
	for _, b := range e.global {
		periksa(b)
	}
	for _, t := range targets {
		if t.Scope == policy.ScopeGlobal || t.ID == "" {
			continue
		}
		for _, b := range e.perCakupan[t.Scope][t.ID] {
			periksa(b)
		}
	}
	if v.Blocked {
		return v
	}

	t := tahanan{jumlah: jumlah, sampai: sekarang.Add(e.ttlTahan)}
	for _, b := range berlaku {
		if !b.Enabled || b.ActionOnExceed != policy.ActionBlock {
			continue
		}
		if e.tahan == nil {
			e.tahan = make(map[string][]tahanan)
		}
		e.tahan[b.ID] = append(e.tahan[b.ID], t)
	}
	return v
}

// sapuTahanTerkunci membuang tahanan yang sudah kedaluwarsa. Pemanggil WAJIB
// memegang e.mu (tulis).
func (e *Enforcer) sapuTahanTerkunci(sekarang time.Time) {
	for id, daftar := range e.tahan {
		hidup := daftar[:0]
		for _, t := range daftar {
			if t.sampai.After(sekarang) {
				hidup = append(hidup, t)
			}
		}
		if len(hidup) == 0 {
			delete(e.tahan, id)
		} else {
			e.tahan[id] = hidup
		}
	}
}

// totalTahanTerkunci menjumlahkan tahanan yang masih hidup untuk satu
// anggaran. Pemanggil WAJIB memegang e.mu dan sudah menyapu.
func (e *Enforcer) totalTahanTerkunci(id string, sekarang time.Time) upstream.USD {
	var total upstream.USD
	for _, t := range e.tahan[id] {
		if t.sampai.After(sekarang) {
			total += t.jumlah
		}
	}
	return total
}

// Announce menandai anggaran yang melewati ambang atau batas sebagai sudah diberitahukan,
// mencatat peringatan ke log, serta memproduksi webhook event budget.threshold dan budget.exceeded.
//
// Penandaan terpisah di database (alerted_at untuk ambang, exceeded_alerted_at untuk batas)
// memastikan event budget.exceeded tetap terkirim saat anggaran habis, walaupun peringatan
// budget.threshold telah terkirim sebelumnya saat menyentuh 80%.
//
// Kegagalannya tidak boleh menggagalkan permintaan: yang hilang hanyalah satu notifikasi.
func (e *Enforcer) Announce(ctx context.Context, budgets []*policy.Budget) {
	if e == nil || e.source == nil {
		return
	}
	for _, b := range budgets {
		// 1. Periksa dan kirim peringatan ambang batas (budget.threshold)
		if b.ShouldAlertThreshold() {
			e.umumkanAmbang(ctx, b)
		} else if b.ShouldAlertExceeded() && b.AlertedAt == nil && b.AlertThresholdPct > 0 {
			// Ambang TERLOMPAT (B-107): satu request bisa membawa spent dari
			// bawah ambang langsung melewati batas (biaya per request lebih
			// besar daripada lebar jendela ambang), sehingga pada setiap
			// pemeriksaan ShouldAlertThreshold sudah false — ia mensyaratkan
			// spent BELUM terlampaui. Tanpa cabang ini peringatan ambangnya
			// hilang permanen: alerted_at tidak pernah terisi dan anggaran
			// warn tampak tidak pernah memperingatkan sama sekali, padahal
			// ambangnya jelas terlewati. Ambang yang terlewati tetap
			// diumumkan — terlambat, bersamaan dengan pengumuman
			// terlampauinya, tetapi tepat satu kali seperti rancangannya.
			e.umumkanAmbang(ctx, b)
		}

		// 2. Periksa dan kirim peringatan batas anggaran terlampaui (budget.exceeded)
		if b.ShouldAlertExceeded() {
			pertama, err := e.source.MarkExceededAlerted(ctx, b.ID)
			if err != nil {
				e.logger.WarnContext(ctx, "penandaan peringatan batas terlampaui anggaran gagal",
					"anggaran", b.Name, "error", err)
			} else if pertama {
				e.logger.WarnContext(ctx, "anggaran terlampaui",
					"anggaran", b.Name,
					"cakupan", b.Scope,
					"periode", b.Period,
					"terpakai_usd", b.SpentUSD.String(),
					"batas_usd", b.LimitUSD.String(),
					"tindakan", b.ActionOnExceed)

				if e.enqueuer != nil {
					payload := map[string]any{
						"budget_id":   b.ID,
						"budget_name": b.Name,
						"scope":       b.Scope,
						"scope_id":    b.ScopeID,
						"spent_usd":   b.SpentUSD.Decimal(),
						"limit_usd":   b.LimitUSD.Decimal(),
						"timestamp":   time.Now().UTC().Format(time.RFC3339),
						"event":       "budget.exceeded",
						"action":      b.ActionOnExceed,
					}
					if _, err := e.enqueuer.Enqueue(ctx, "budget.exceeded", payload); err != nil {
						e.logger.WarnContext(ctx, "gagal memasukkan notifikasi budget.exceeded ke antrean webhook",
							"anggaran", b.Name, "error", err)
					}
				}
			}
		}
	}
	// Salinan di memori dibatalkan agar pembacaan berikutnya memuat status alerted_at
	// dan exceeded_alerted_at terbaru dari database.
	e.Invalidate()
}

// umumkanAmbang menandai dan mengumumkan SATU peringatan ambang anggaran:
// penanda alerted_at di database, baris log peringatan, dan event webhook
// budget.threshold. Penandaan bersyarat di repo (hanya yang pertama yang
// menandai yang mengumumkan), jadi memanggilnya berulang aman.
func (e *Enforcer) umumkanAmbang(ctx context.Context, b *policy.Budget) {
	pertama, err := e.source.MarkThresholdAlerted(ctx, b.ID)
	if err != nil {
		e.logger.WarnContext(ctx, "penandaan peringatan ambang anggaran gagal",
			"anggaran", b.Name, "error", err)
		return
	}
	if !pertama {
		return
	}
	e.logger.WarnContext(ctx, "anggaran melewati ambang peringatan",
		"anggaran", b.Name,
		"cakupan", b.Scope,
		"periode", b.Period,
		"ambang_persen", b.AlertThresholdPct,
		"terpakai_usd", b.SpentUSD.String(),
		"batas_usd", b.LimitUSD.String())

	if e.enqueuer != nil {
		payload := map[string]any{
			"budget_id":     b.ID,
			"budget_name":   b.Name,
			"scope":         b.Scope,
			"scope_id":      b.ScopeID,
			"spent_usd":     b.SpentUSD.Decimal(),
			"limit_usd":     b.LimitUSD.Decimal(),
			"timestamp":     time.Now().UTC().Format(time.RFC3339),
			"event":         "budget.threshold",
			"threshold_pct": b.AlertThresholdPct,
		}
		if _, err := e.enqueuer.Enqueue(ctx, "budget.threshold", payload); err != nil {
			e.logger.WarnContext(ctx, "gagal memasukkan notifikasi budget.threshold ke antrean webhook",
				"anggaran", b.Name, "error", err)
		}
	}
}

// Remaining mengembalikan sisa anggaran paling sedikit di antara yang berlaku, dan apakah
// ada anggaran yang berlaku sama sekali.
//
// Status (triage B-005): saat ini TIDAK dipanggil jalur produksi — dashboard
// menghitung sisa per anggaran dari baris repo (policy.Budget.Remaining),
// bukan dari salinan memori Enforcer. Method ini dipertahankan sebagai API
// baca yang teruji atas salinan yang sama dengan yang dipakai penegakan
// (Check/Tahan), untuk pemanggil yang membutuhkan sisa gabungan tanpa query
// baru; klaim lama bahwa ia dipakai pencatat usage dan dashboard sudah
// tidak benar dan dihapus.
func (e *Enforcer) Remaining(ctx context.Context, targets []policy.Target) (upstream.USD, bool) {
	e.muat(ctx)

	e.mu.RLock()
	defer e.mu.RUnlock()

	var (
		sisa upstream.USD
		ada  bool
	)
	catat := func(b *policy.Budget) {
		r := b.Remaining()
		if !ada || r < sisa {
			sisa, ada = r, true
		}
	}
	for _, b := range e.global {
		catat(b)
	}
	for _, t := range targets {
		if t.Scope == policy.ScopeGlobal || t.ID == "" {
			continue
		}
		for _, b := range e.perCakupan[t.Scope][t.ID] {
			catat(b)
		}
	}
	return sisa, ada
}

// tenggatMuatKebijakan membatasi satu query pemuatan ulang kebijakan, dan
// jedaGagalMuat adalah umur cache negatif sesudah kegagalan (audit3 V-LN-11).
const (
	tenggatMuatKebijakan = 5 * time.Second
	jedaGagalMuat        = 5 * time.Second
)

// muat memuat ulang salinan bila sudah kedaluwarsa.
//
// Kegagalan pembacaan mempertahankan salinan lama. Dua perbaikan atas pola
// lama (audit3 V-LN-11): (1) querynya memakai context LEPAS dari pemanggil
// pertama dengan tenggat sendiri — dulu ctx request pemanggil pertama
// dipakai di bawah mutex global, jadi disconnect satu pemuat membatalkan
// pemuatan bagi semua yang mengantre, dan query yang macet menyeret seluruh
// request gateway; (2) kegagalan dicatat sebagai cache negatif berumur
// jedaGagalMuat — dulu waktu muat sengaja tidak diperbarui sehingga SETIAP
// request berikutnya mengantre di mutex dan mengulang query yang sama ke
// database yang sedang sakit. Jedanya pendek supaya pemulihan tetap cepat.
func (e *Enforcer) muat(ctx context.Context) {
	if e.source == nil {
		return
	}

	e.mu.RLock()
	segar := !e.dimuat.IsZero() && e.now().Sub(e.dimuat) < e.ttl
	e.mu.RUnlock()
	if segar {
		return
	}

	// Hanya satu goroutine memuat ulang; yang lain menunggu lalu memakai
	// hasilnya, dengan pemeriksaan ulang karena pemuat pertama mungkin sudah
	// menyegarkan salinan selagi yang lain antre.
	e.pemuatan.Lock()
	defer e.pemuatan.Unlock()

	e.mu.RLock()
	segar = !e.dimuat.IsZero() && e.now().Sub(e.dimuat) < e.ttl
	baruGagal := !e.gagalMuat.IsZero() && e.now().Sub(e.gagalMuat) < jedaGagalMuat
	e.mu.RUnlock()
	if segar || baruGagal {
		return
	}

	qctx, batal := context.WithTimeout(context.WithoutCancel(ctx), tenggatMuatKebijakan)
	defer batal()
	rows, err := e.source.ActiveBudgets(qctx)
	if err != nil {
		e.mu.Lock()
		e.gagalMuat = e.now()
		e.mu.Unlock()
		e.logger.WarnContext(ctx, "anggaran gagal dibaca, memakai salinan sebelumnya", "error", err)
		return
	}

	var global []*policy.Budget
	perCakupan := make(map[string]map[string][]*policy.Budget)
	for _, b := range rows {
		if b.Scope == policy.ScopeGlobal {
			global = append(global, b)
			continue
		}
		per, ok := perCakupan[b.Scope]
		if !ok {
			per = make(map[string][]*policy.Budget)
			perCakupan[b.Scope] = per
		}
		per[b.ScopeID] = append(per[b.ScopeID], b)
	}

	e.mu.Lock()
	e.global, e.perCakupan, e.dimuat = global, perCakupan, e.now()
	e.gagalMuat = time.Time{}
	e.mu.Unlock()
}
