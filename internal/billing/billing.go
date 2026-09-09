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

// Enforcer menyimpan salinan anggaran dan menjawab apakah permintaan boleh jalan.
//
// Aman dipakai bersamaan oleh banyak goroutine.
type Enforcer struct {
	source   Source
	enqueuer WebhookEnqueuer
	logger   *slog.Logger
	ttl      time.Duration
	now      func() time.Time

	mu     sync.RWMutex
	dimuat time.Time
	// global adalah anggaran bercakupan global.
	global []*policy.Budget
	// perCakupan diindeks per [scope][scope_id].
	perCakupan map[string]map[string][]*policy.Budget
	// pemuatan menyatukan pemuatan ulang yang bersamaan menjadi satu query:
	// tanpa ini, salinan yang kedaluwarsa di bawah beban menghasilkan satu
	// query ActiveBudgets per permintaan yang datang dalam jendela pemuatan —
	// persis saat database paling tidak butuh tambahan beban.
	pemuatan sync.Mutex
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
func WithTTL(d time.Duration) Option {
	return func(e *Enforcer) {
		if d > 0 {
			e.ttl = d
		}
	}
}

// WithClock mengganti sumber waktu, untuk test kedaluwarsa salinan.
func WithClock(now func() time.Time) Option {
	return func(e *Enforcer) {
		if now != nil {
			e.now = now
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
	e := &Enforcer{source: source, logger: logger, ttl: defaultTTL, now: time.Now}
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
			pertama, err := e.source.MarkThresholdAlerted(ctx, b.ID)
			if err != nil {
				e.logger.WarnContext(ctx, "penandaan peringatan ambang anggaran gagal",
					"anggaran", b.Name, "error", err)
			} else if pertama {
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

// Remaining mengembalikan sisa anggaran paling sedikit di antara yang berlaku, dan apakah
// ada anggaran yang berlaku sama sekali.
//
// Dipakai pencatat usage Fase 9 dan dashboard: "sisa 3,12 USD" jauh lebih berguna daripada
// daftar anggaran yang harus dijumlahkan sendiri pembacanya.
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

// muat memuat ulang salinan bila sudah kedaluwarsa.
//
// Kegagalan pembacaan mempertahankan salinan lama dan tidak memperbarui waktu muat, dengan
// alasan yang sama seperti di internal/ratelimit: percobaan berikutnya tidak perlu menunggu
// satu TTL penuh.
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
	e.mu.RUnlock()
	if segar {
		return
	}

	rows, err := e.source.ActiveBudgets(ctx)
	if err != nil {
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
	e.mu.Unlock()
}
