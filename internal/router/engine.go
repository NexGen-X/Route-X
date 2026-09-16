package router

import (
	"context"
	"log/slog"
	"sync"
	"time"

	"github.com/NexGen-X/Route-X/internal/database/repo/upstream"
)

// defaultRuleTTL adalah masa berlaku cache aturan routing.
//
// Aturan routing dibaca SETIAP permintaan inference dan berubah beberapa kali sehari, jadi
// membacanya dari Postgres per permintaan adalah query yang hampir selalu mengembalikan
// jawaban yang sama.
//
// Yang dipakai TTL pendek, bukan pembatalan-saat-tulis, dan itu keputusan yang sadar:
// gateway ini berjalan multi-instance, dan perubahan aturan di satu instance tidak bisa
// membatalkan cache instance lain tanpa saluran pub/sub tersendiri. TTL pendek memberi
// batas atas keterlambatan yang bisa dijelaskan ke operator dalam satu kalimat — "aturan
// baru berlaku dalam beberapa detik" — tanpa satu pun bagian yang bisa gagal diam-diam.
// Kalau nanti keterlambatan itu terasa, tempat memperbaikinya adalah pub/sub Redis di
// Fase 11, bukan TTL yang diperpanjang.
const defaultRuleTTL = 5 * time.Second

// RuleSource memasok aturan routing yang aktif.
//
// Dipenuhi *upstream.RoutingRepo. Antarmuka supaya Engine bisa diuji tanpa Postgres.
type RuleSource interface {
	// ActiveRules mengembalikan aturan aktif terurut (priority, created_at).
	ActiveRules(ctx context.Context) ([]*upstream.RoutingRule, error)
}

// Engine menjawab satu pertanyaan: untuk permintaan ini, provider mana saja yang dicoba
// dan dalam urutan apa.
//
// Aman dipakai bersamaan oleh banyak goroutine.
type Engine struct {
	source RuleSource
	sel    *Selector
	logger *slog.Logger
	ttl    time.Duration

	// sekarang dipisah supaya test bisa memajukan waktu tanpa menunggu.
	sekarang func() time.Time

	mu      sync.RWMutex
	cache   []*Rule
	segarDi time.Time
	// muat menyatukan pemuatan ulang yang bersamaan menjadi satu query, sehingga cache
	// yang kedaluwarsa di bawah beban tidak menghasilkan satu query per permintaan.
	muat sync.Mutex
}

// EngineOption menyetel Engine.
type EngineOption func(*Engine)

// WithRuleTTL mengubah masa berlaku cache aturan.
func WithRuleTTL(d time.Duration) EngineOption {
	return func(e *Engine) {
		if d > 0 {
			e.ttl = d
		}
	}
}

// WithClock mengganti sumber waktu Engine. Ada untuk test.
func WithClock(f func() time.Time) EngineOption {
	return func(e *Engine) {
		if f != nil {
			e.sekarang = f
		}
	}
}

// NewEngine membuat Engine.
//
// source boleh nil, yang berarti tanpa aturan sama sekali: seluruh permintaan memakai
// StrategyPriority atas semua kandidat. Itu perilaku yang benar untuk pemasangan baru yang
// belum punya satu pun aturan, dan membuat jalur permintaan tidak bergantung pada tabel
// yang mungkin masih kosong.
func NewEngine(source RuleSource, sel *Selector, logger *slog.Logger, opts ...EngineOption) *Engine {
	if sel == nil {
		sel = NewSelector()
	}
	if logger == nil {
		logger = slog.New(slog.DiscardHandler)
	}
	e := &Engine{source: source, sel: sel, logger: logger, ttl: defaultRuleTTL, sekarang: time.Now}
	for _, o := range opts {
		o(e)
	}
	return e
}

// Decision adalah hasil routing satu permintaan.
type Decision struct {
	// Rule adalah aturan yang cocok, nil bila tidak ada — dan nil di sini BUKAN kegagalan:
	// artinya permintaan dirutekan dengan strategi bawaan.
	Rule *Rule
	// Candidates sudah terurut dan sudah disaring. Kosong berarti tidak ada provider yang
	// bisa melayani permintaan ini.
	Candidates []*upstream.RouteCandidate
}

// Route memilih aturan lalu menyusun urutan kandidat.
//
// cands adalah keluaran upstream.ProviderRepo.RouteCandidates untuk model yang diminta.
// Route TIDAK mengambilnya sendiri: pengambilan itu butuh model_id hasil resolusi alias dan
// pemilik BYOK, dua hal yang sudah dipegang pemanggil, dan memindahkannya ke sini berarti
// Engine harus ikut memegang repository model — dependensi yang cuma dipakai untuk
// meneruskan parameter.
//
// Kegagalan membaca aturan TIDAK menggagalkan permintaan: ia dicatat, dan permintaan
// dirutekan dengan strategi bawaan. Alternatifnya membuat satu tabel konfigurasi yang tidak
// terbaca mematikan seluruh lalu lintas inference, padahal daftar kandidat sudah ada di
// tangan dan urutan bawaan adalah urutan yang benar-benar bisa dipakai.
func (e *Engine) Route(ctx context.Context, req Request, cands []*upstream.RouteCandidate) Decision {
	rules, err := e.aturan(ctx)
	if err != nil {
		e.logger.ErrorContext(ctx, "aturan routing tidak bisa dibaca, memakai strategi bawaan",
			"model", req.ModelName, "error", err)
	}
	rule := FirstMatch(rules, req)
	return Decision{Rule: rule, Candidates: e.sel.Order(req, rule, cands)}
}

// aturan mengembalikan aturan aktif dari cache, memuat ulang bila sudah kedaluwarsa.
//
// Bila pemuatan ulang gagal, aturan LAMA tetap dipakai dan errornya dikembalikan untuk
// dicatat. Membuang cache saat sumbernya sedang tidak bisa dibaca berarti gangguan
// sementara pada database mengubah cara seluruh lalu lintas dirutekan — perubahan perilaku
// yang jauh lebih besar akibatnya daripada aturan yang berumur beberapa detik lebih tua.
func (e *Engine) aturan(ctx context.Context) ([]*Rule, error) {
	if e.source == nil {
		return nil, nil
	}

	e.mu.RLock()
	cache, segarDi := e.cache, e.segarDi
	e.mu.RUnlock()
	if e.sekarang().Before(segarDi) {
		return cache, nil
	}

	// Hanya satu goroutine memuat ulang; yang lain menunggu lalu memakai hasilnya. Tanpa
	// ini, cache yang kedaluwarsa di bawah beban menghasilkan satu query per permintaan
	// yang datang dalam jendela pemuatan — persis saat database paling tidak butuh
	// tambahan beban.
	e.muat.Lock()
	defer e.muat.Unlock()

	e.mu.RLock()
	cache, segarDi = e.cache, e.segarDi
	e.mu.RUnlock()
	if e.sekarang().Before(segarDi) {
		return cache, nil
	}

	rows, err := e.source.ActiveRules(ctx)
	if err != nil {
		return cache, err
	}

	baru := make([]*Rule, 0, len(rows))
	for _, row := range rows {
		r, err := RuleFromRow(row)
		if err != nil {
			// Satu baris aturan yang cacat tidak boleh mematikan routing seluruhnya.
			// Dicatat sebagai ERROR, bukan diam: baris seperti ini hanya bisa muncul kalau
			// check constraint dan kode ini tidak lagi sepakat, dan itu perlu diperbaiki
			// manusia — tetapi memperbaikinya tidak boleh menuntut gateway berhenti dulu.
			e.logger.ErrorContext(ctx, "aturan routing dilewati karena tidak bisa dibaca",
				"aturan", row.Name, "error", err)
			continue
		}
		baru = append(baru, r)
	}

	e.mu.Lock()
	e.cache, e.segarDi = baru, e.sekarang().Add(e.ttl)
	e.mu.Unlock()
	return baru, nil
}

// Invalidate memaksa aturan dimuat ulang pada pemanggilan berikutnya.
//
// Dipakai API admin di Fase 11 setelah mengubah aturan, supaya operator yang baru menyimpan
// perubahan langsung melihat akibatnya pada instance yang ia hubungi — instance lain
// menyusul saat TTL-nya lewat.
func (e *Engine) Invalidate() {
	e.mu.Lock()
	e.segarDi = time.Time{}
	e.mu.Unlock()
}

// FindRuleTargetModelID mencari target ModelID dari aturan routing bila nama yang diminta
// cocok dengan nama aturan aktif atau alias combo ([combo:alias=...]).
func (e *Engine) FindRuleTargetModelID(ctx context.Context, requested string) string {
	if e == nil || requested == "" {
		return ""
	}
	rules, err := e.aturan(ctx)
	if err != nil || len(rules) == 0 {
		return ""
	}
	for _, r := range rules {
		alias := ExtractComboAlias(r.Description)
		if (r.Name == requested || (alias != "" && alias == requested)) && r.MatchModelID != "" {
			return r.MatchModelID
		}
	}
	return ""
}
