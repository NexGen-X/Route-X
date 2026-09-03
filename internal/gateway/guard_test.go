package gateway

import (
	"context"
	"encoding/json"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync"
	"testing"

	"github.com/alicebob/miniredis/v2"

	"github.com/NexGen-X/Route-X/internal/apikey"
	"github.com/NexGen-X/Route-X/internal/billing"
	"github.com/NexGen-X/Route-X/internal/cache"
	"github.com/NexGen-X/Route-X/internal/config"
	"github.com/NexGen-X/Route-X/internal/contentfilter"
	"github.com/NexGen-X/Route-X/internal/database/repo/policy"
	"github.com/NexGen-X/Route-X/internal/database/repo/upstream"
	"github.com/NexGen-X/Route-X/internal/observability"
	"github.com/NexGen-X/Route-X/internal/providers"
	"github.com/NexGen-X/Route-X/internal/ratelimit"
	"github.com/NexGen-X/Route-X/internal/security"
)

// --- Sumber kebijakan tiruan -------------------------------------------------

type sumberKebijakan struct {
	mu       sync.Mutex
	limits   []*policy.RateLimit
	budgets  []*policy.Budget
	filters  []*policy.ContentFilter
	ditandai []string
}

func (s *sumberKebijakan) ActiveRateLimits(context.Context) ([]*policy.RateLimit, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	return s.limits, nil
}

func (s *sumberKebijakan) ActiveBudgets(context.Context) ([]*policy.Budget, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	return s.budgets, nil
}

func (s *sumberKebijakan) ActiveFilters(context.Context) ([]*policy.ContentFilter, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	return s.filters, nil
}

func (s *sumberKebijakan) MarkAlerted(_ context.Context, id string) (bool, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.ditandai = append(s.ditandai, id)
	return true, nil
}

func (s *sumberKebijakan) RecordEvalTimeout(context.Context, string) error { return nil }

// limiterUji menyalakan Redis tiruan lalu menyambungkannya lewat jalur produksi.
func limiterUji(t *testing.T) *apikey.Limiter {
	t.Helper()
	mr := miniredis.RunT(t)
	rdb, err := cache.Connect(context.Background(), &config.Config{
		AppEnv:   config.EnvDevelopment,
		RedisURL: security.Secret("redis://" + mr.Addr()),
	}, slog.New(slog.DiscardHandler))
	if err != nil {
		t.Fatalf("cache.Connect: %v", err)
	}
	t.Cleanup(func() { _ = rdb.Close() })
	return apikey.NewLimiter(rdb, nil, loggerSenyap())
}

// guardUji merakit Guard lengkap di atas sumber tiruan dan Redis tiruan.
func guardUji(t *testing.T, src *sumberKebijakan) *Guard {
	t.Helper()
	return NewGuard(GuardDeps{
		Filters: contentfilter.NewEngine(src, loggerSenyap()),
		Budgets: billing.NewEnforcer(src, loggerSenyap()),
		Rates:   ratelimit.NewEngine(src, loggerSenyap()),
		Limiter: limiterUji(t),
		Logger:  loggerSenyap(),
	})
}

// filterBaris menyusun satu baris content_filters dengan bawaan database.
func filterBaris(id, name, kind string, ubah func(*policy.ContentFilter)) *policy.ContentFilter {
	f := &policy.ContentFilter{
		ID: id, Name: name, Kind: kind, Priority: 100,
		AppliesTo: policy.AppliesToRequest, Action: policy.ActionBlock,
		MaxEvalMS: 50, Enabled: true,
	}
	if ubah != nil {
		ubah(f)
	}
	return f
}

// providerDenganUsage adalah provider tiruan yang melaporkan jumlah token.
func providerDenganUsage(nama string, total int) *providerTiruan {
	return &providerTiruan{
		nama: nama, kind: providers.KindOpenAI,
		chat: func(_ context.Context, _ *providers.ChatRequest) (*providers.ChatResponse, error) {
			return &providers.ChatResponse{
				ID:  "chatcmpl-1",
				Raw: json.RawMessage(rawUpstream),
				Choices: []providers.Choice{{
					Message: providers.Message{Role: providers.RoleAssistant, Content: "hai"},
				}},
				Usage: providers.Usage{InputTokens: total / 2, OutputTokens: total - total/2, TotalTokens: total},
			}, nil
		},
	}
}

// --- Penyaring konten --------------------------------------------------------

func TestPenyaringKontenMenolakPermintaan(t *testing.T) {
	src := &sumberKebijakan{filters: []*policy.ContentFilter{
		filterBaris("f1", "tanpa nomor kartu", policy.FilterBlockedPattern, func(f *policy.ContentFilter) {
			f.Pattern = `\b\d{16}\b`
			f.PatternType = policy.PatternRegex
		}),
	}}
	s := susun(t, providerSukses(), func(_ *susunan, d *HandlersDeps) { d.Guard = guardUji(t, src) })

	w := s.panggil(t, http.MethodPost, "/chat/completions",
		`{"model":"gpt-4o-mini","messages":[{"role":"user","content":"kartu saya 4111111111111111"}]}`, nil)

	if w.Code != http.StatusBadRequest {
		t.Fatalf("status = %d, mau 400 (body: %s)", w.Code, w.Body.String())
	}
	d := galat(t, w)
	if kodeGalat(d) != CodeContentFilter {
		t.Errorf("code = %q, mau %q", kodeGalat(d), CodeContentFilter)
	}
	if !strings.Contains(d.Message, "tanpa nomor kartu") {
		t.Errorf("pesan tidak menyebut nama kebijakan: %q", d.Message)
	}
	// Pesan penolakan ikut ke log bersama; isi permintaan dan pola tidak boleh ada di dalamnya.
	if strings.Contains(d.Message, "4111") || strings.Contains(d.Message, `\d{16}`) {
		t.Errorf("pesan membocorkan isi permintaan atau pola: %q", d.Message)
	}
}

func TestBatasUkuranPermintaanJadi413(t *testing.T) {
	src := &sumberKebijakan{filters: []*policy.ContentFilter{
		filterBaris("f1", "maksimal 64 byte", policy.FilterRequestSize, func(f *policy.ContentFilter) {
			f.MaxRequestBytes = pointerG(int64(64))
		}),
	}}
	s := susun(t, providerSukses(), func(_ *susunan, d *HandlersDeps) { d.Guard = guardUji(t, src) })

	w := s.panggil(t, http.MethodPost, "/chat/completions",
		`{"model":"gpt-4o-mini","messages":[{"role":"user","content":"`+strings.Repeat("a", 200)+`"}]}`, nil)

	if w.Code != http.StatusRequestEntityTooLarge {
		t.Fatalf("status = %d, mau 413 (body: %s)", w.Code, w.Body.String())
	}
	if got := kodeGalat(galat(t, w)); got != CodePayloadTooBig {
		t.Errorf("code = %q, mau %q", got, CodePayloadTooBig)
	}
}

// TestPenyaringJawabanNonStreaming menjaga sisi jawaban, dan sekaligus mencatat batas yang
// disengaja: jalur streaming TIDAK disaring, karena memeriksa per potongan melewatkan pola
// yang melintasi batas potongan sementara menahan seluruh aliran membatalkan gunanya streaming.
func TestPenyaringJawabanNonStreaming(t *testing.T) {
	src := &sumberKebijakan{filters: []*policy.ContentFilter{
		filterBaris("f1", "jawaban bersih", policy.FilterBlockedPattern, func(f *policy.ContentFilter) {
			f.Pattern = "hai"
			f.PatternType = policy.PatternSubstring
			f.AppliesTo = policy.AppliesToResponse
		}),
	}}
	// rawUpstream memuat jawaban "hai".
	prov := providerSukses()
	s := susun(t, prov, func(_ *susunan, d *HandlersDeps) { d.Guard = guardUji(t, src) })

	w := s.panggil(t, http.MethodPost, "/chat/completions", bodyChatMinimal, nil)
	if w.Code != http.StatusBadRequest {
		t.Fatalf("status = %d, mau 400 (body: %s)", w.Code, w.Body.String())
	}

	// Jalur streaming memakai aturan yang sama dan tetap lolos: itu batas yang diketahui,
	// bukan kebetulan.
	prov.alir = func(_ context.Context, _ *providers.ChatRequest) (providers.Stream, error) {
		return &aliranTiruan{ev: []*providers.StreamEvent{peristiwa("hai")}}, nil
	}
	w = s.panggil(t, http.MethodPost, "/chat/completions", bodyChatStreaming, nil)
	if w.Code != http.StatusOK {
		t.Errorf("status streaming = %d, mau 200", w.Code)
	}
}

// --- Anggaran ----------------------------------------------------------------

func TestAnggaranHabisJadi402(t *testing.T) {
	src := &sumberKebijakan{budgets: []*policy.Budget{{
		ID: "b1", Name: "anggaran bulanan", Scope: policy.ScopeGlobal, Period: policy.PeriodMonthly,
		LimitUSD: upstream.MustParseUSD("10"), SpentUSD: upstream.MustParseUSD("10"),
		ActionOnExceed: policy.ActionBlock, AlertThresholdPct: 80, Enabled: true,
	}}}
	s := susun(t, providerSukses(), func(_ *susunan, d *HandlersDeps) { d.Guard = guardUji(t, src) })

	w := s.panggil(t, http.MethodPost, "/chat/completions", bodyChatMinimal, nil)
	// 402, bukan 429: anggaran habis tidak pulih dengan menunggu, dan Retry-After yang
	// menunjuk ke pergantian bulan hanya membuat pustaka klien tidur berhari-hari.
	if w.Code != http.StatusPaymentRequired {
		t.Fatalf("status = %d, mau 402 (body: %s)", w.Code, w.Body.String())
	}
	d := galat(t, w)
	if kodeGalat(d) != CodeBudgetExceeded {
		t.Errorf("code = %q, mau %q", kodeGalat(d), CodeBudgetExceeded)
	}
	if w.Header().Get("Retry-After") != "" {
		t.Error("Retry-After dipasang pada penolakan anggaran")
	}
	// Angka batas dan pemakaian satu penyewa bukan hal yang pantas dikirim ke penyewa lain.
	if strings.Contains(d.Message, "10") {
		t.Errorf("pesan membocorkan angka anggaran: %q", d.Message)
	}
}

func TestAnggaranWarnTidakMemblokir(t *testing.T) {
	src := &sumberKebijakan{budgets: []*policy.Budget{{
		ID: "b1", Name: "pantau", Scope: policy.ScopeGlobal, Period: policy.PeriodMonthly,
		LimitUSD: upstream.MustParseUSD("10"), SpentUSD: upstream.MustParseUSD("99"),
		ActionOnExceed: policy.ActionWarn, AlertThresholdPct: 80, Enabled: true,
	}}}
	s := susun(t, providerSukses(), func(_ *susunan, d *HandlersDeps) { d.Guard = guardUji(t, src) })

	if w := s.panggil(t, http.MethodPost, "/chat/completions", bodyChatMinimal, nil); w.Code != http.StatusOK {
		t.Errorf("status = %d, mau 200", w.Code)
	}
	// Ambang peringatan ditandai supaya tidak diulang setiap permintaan.
	src.mu.Lock()
	defer src.mu.Unlock()
	if len(src.ditandai) == 0 {
		t.Error("anggaran yang melewati ambang tidak ditandai")
	}
}

// pointerG mengembalikan pointer ke nilai apa pun, untuk mengisi kolom opsional.
func pointerG[T any](v T) *T { return &v }

// --- Batas laju cakupan model ------------------------------------------------

func TestBatasLajuCakupanModel(t *testing.T) {
	src := &sumberKebijakan{limits: []*policy.RateLimit{{
		Scope: policy.ScopeModel, ScopeID: "gpt-4o-mini", RequestsPerMinute: pointerG(1), Enabled: true,
	}}}
	s := susun(t, providerSukses(), func(_ *susunan, d *HandlersDeps) { d.Guard = guardUji(t, src) })

	// Cakupan model memakai NAMA model kanonik, bukan UUID-nya: itulah nilai yang diketik
	// operator di dashboard, dan yang muncul di log bersama nama modelnya.
	if w := s.panggil(t, http.MethodPost, "/chat/completions", bodyChatMinimal, nil); w.Code != http.StatusOK {
		t.Fatalf("permintaan pertama status = %d, mau 200 (body: %s)", w.Code, w.Body.String())
	}

	w := s.panggil(t, http.MethodPost, "/chat/completions", bodyChatMinimal, nil)
	if w.Code != http.StatusTooManyRequests {
		t.Fatalf("permintaan kedua status = %d, mau 429 (body: %s)", w.Code, w.Body.String())
	}
	if got := kodeGalat(galat(t, w)); got != CodeRateLimited {
		t.Errorf("code = %q, mau %q", got, CodeRateLimited)
	}
	// Header kuota dipasang juga pada respons yang ditolak: itu bagian kontrak klien
	// OpenAI-compatible.
	if got := w.Header().Get(apikey.HeaderRateLimitLimit); got != "1" {
		t.Errorf("%s = %q, mau 1", apikey.HeaderRateLimitLimit, got)
	}
	if w.Header().Get(apikey.HeaderRateLimitReset) == "" {
		t.Errorf("%s tidak dipasang", apikey.HeaderRateLimitReset)
	}
	if w.Header().Get("Retry-After") == "" {
		t.Error("Retry-After tidak dipasang pada 429")
	}
}

// --- Batas laju cakupan provider ---------------------------------------------

// TestBatasLajuCakupanProviderDialihkan adalah keputusan bentuk yang paling penting di sisi
// provider: batas yang penuh MELEWATI kandidat itu, bukan menolak permintaan. Kandidat lain
// yang sehat tidak boleh terbuang karena kuota provider pertama sedang habis.
func TestBatasLajuCakupanProviderDialihkan(t *testing.T) {
	src := &sumberKebijakan{limits: []*policy.RateLimit{{
		Scope: policy.ScopeProvider, ScopeID: "prov-utama", RequestsPerMinute: pointerG(1), Enabled: true,
	}}}
	kedua := providerSukses()
	kedua.nama = "kedua"

	s := susun(t, nil, func(s *susunan, d *HandlersDeps) {
		s.cands.cands = []*upstream.RouteCandidate{
			kandidatRute("utama", "gpt-4o-mini-2024"),
			kandidatRute("kedua", "gpt-4o-mini-2024"),
		}
		s.factory.perNama = map[string]providers.Provider{"utama": providerSukses(), "kedua": kedua}
		d.Guard = guardUji(t, src)
	})

	if w := s.panggil(t, http.MethodPost, "/chat/completions", bodyChatMinimal, nil); w.Code != http.StatusOK {
		t.Fatalf("permintaan pertama status = %d, mau 200", w.Code)
	}
	// Permintaan kedua: kuota provider utama sudah habis, jadi jawabannya harus datang dari
	// kandidat kedua — bukan 429.
	w := s.panggil(t, http.MethodPost, "/chat/completions", bodyChatMinimal, nil)
	if w.Code != http.StatusOK {
		t.Fatalf("permintaan kedua status = %d, mau 200 lewat kandidat kedua (body: %s)", w.Code, w.Body.String())
	}
	urut := s.factory.urutan()
	if len(urut) < 2 || urut[len(urut)-1] != "kedua" {
		t.Errorf("urutan percobaan = %v, mau berakhir di kandidat kedua", urut)
	}
}

func TestBatasLajuProviderSemuaPenuhJadi429(t *testing.T) {
	src := &sumberKebijakan{limits: []*policy.RateLimit{{
		Scope: policy.ScopeProvider, ScopeID: "prov-utama", RequestsPerMinute: pointerG(1), Enabled: true,
	}}}
	s := susun(t, providerSukses(), func(_ *susunan, d *HandlersDeps) { d.Guard = guardUji(t, src) })

	if w := s.panggil(t, http.MethodPost, "/chat/completions", bodyChatMinimal, nil); w.Code != http.StatusOK {
		t.Fatalf("permintaan pertama status = %d, mau 200", w.Code)
	}
	// Satu-satunya kandidat sedang penuh: di sini 429 memang jawaban yang benar, dan ia
	// datang lewat pemetaan kegagalan upstream, bukan lewat penolakan kebijakan.
	w := s.panggil(t, http.MethodPost, "/chat/completions", bodyChatMinimal, nil)
	if w.Code != http.StatusTooManyRequests {
		t.Fatalf("status = %d, mau 429 (body: %s)", w.Code, w.Body.String())
	}
}

// --- Pembatasan provider oleh penyaring konten -------------------------------

func TestPenyaringKontenMembatasiProviderKandidat(t *testing.T) {
	src := &sumberKebijakan{filters: []*policy.ContentFilter{
		filterBaris("f1", "provider utama dilarang", policy.FilterProviderRestriction,
			func(f *policy.ContentFilter) { f.ProviderID = "prov-utama" }),
	}}
	kedua := providerSukses()
	kedua.nama = "kedua"

	s := susun(t, nil, func(s *susunan, d *HandlersDeps) {
		s.cands.cands = []*upstream.RouteCandidate{
			kandidatRute("utama", "gpt-4o-mini-2024"),
			kandidatRute("kedua", "gpt-4o-mini-2024"),
		}
		s.factory.perNama = map[string]providers.Provider{"utama": providerSukses(), "kedua": kedua}
		d.Guard = guardUji(t, src)
	})

	w := s.panggil(t, http.MethodPost, "/chat/completions", bodyChatMinimal, nil)
	if w.Code != http.StatusOK {
		t.Fatalf("status = %d, mau 200 lewat kandidat yang tidak dibatasi (body: %s)", w.Code, w.Body.String())
	}
	// Provider yang dibatasi tidak boleh dihubungi sama sekali.
	for _, nama := range s.factory.urutan() {
		if nama == "utama" {
			t.Error("provider yang dibatasi kebijakan tetap dihubungi")
		}
	}
}

func TestSemuaProviderDibatasiJadi503(t *testing.T) {
	src := &sumberKebijakan{filters: []*policy.ContentFilter{
		filterBaris("f1", "provider utama dilarang", policy.FilterProviderRestriction,
			func(f *policy.ContentFilter) { f.ProviderID = "prov-utama" }),
	}}
	s := susun(t, providerSukses(), func(_ *susunan, d *HandlersDeps) { d.Guard = guardUji(t, src) })

	w := s.panggil(t, http.MethodPost, "/chat/completions", bodyChatMinimal, nil)
	// Bukan 502 dari provider yang sengaja tidak dihubungi: penolakannya menyebut bahwa tidak
	// ada provider yang bisa melayani model ini, yang bisa ditindaklanjuti operator.
	if w.Code != http.StatusServiceUnavailable {
		t.Fatalf("status = %d, mau 503 (body: %s)", w.Code, w.Body.String())
	}
	if got := kodeGalat(galat(t, w)); got != "no_provider_for_model" {
		t.Errorf("code = %q, mau no_provider_for_model", got)
	}
}

// --- Anggaran token ----------------------------------------------------------

// TestAnggaranTokenDigerbangkanSetelahTercatat menjaga satu-satunya bentuk penegakan yang
// mungkin untuk batas token: besarnya sebuah permintaan baru diketahui setelah dijawab, jadi
// permintaan BERIKUTNYA-lah yang digerbangi pemakaian yang sudah tercatat.
func TestAnggaranTokenDigerbangkanSetelahTercatat(t *testing.T) {
	src := &sumberKebijakan{limits: []*policy.RateLimit{{
		Scope: policy.ScopeGlobal, DailyTokenLimit: pointerG(int64(100)), Enabled: true,
	}}}
	s := susun(t, providerDenganUsage("utama", 150), func(_ *susunan, d *HandlersDeps) {
		d.Guard = guardUji(t, src)
	})

	// Permintaan pertama lolos: belum ada token yang tercatat, dan tidak ada pembatas yang
	// bisa tahu permintaan ini akan memakai 150 token.
	if w := s.panggil(t, http.MethodPost, "/chat/completions", bodyChatMinimal, nil); w.Code != http.StatusOK {
		t.Fatalf("permintaan pertama status = %d, mau 200 (body: %s)", w.Code, w.Body.String())
	}

	w := s.panggil(t, http.MethodPost, "/chat/completions", bodyChatMinimal, nil)
	if w.Code != http.StatusTooManyRequests {
		t.Fatalf("permintaan kedua status = %d, mau 429 (body: %s)", w.Code, w.Body.String())
	}
	if got := kodeGalat(galat(t, w)); got != CodeRateLimited {
		t.Errorf("code = %q, mau %q", got, CodeRateLimited)
	}
}

func TestTokenAliranIkutTercatat(t *testing.T) {
	src := &sumberKebijakan{limits: []*policy.RateLimit{{
		Scope: policy.ScopeGlobal, DailyTokenLimit: pointerG(int64(10)), Enabled: true,
	}}}
	prov := providerSukses()
	prov.alir = func(_ context.Context, _ *providers.ChatRequest) (providers.Stream, error) {
		usage := providers.Usage{InputTokens: 20, OutputTokens: 30, TotalTokens: 50}
		return &aliranTiruan{ev: []*providers.StreamEvent{
			peristiwa("ha"),
			{Usage: &usage, Raw: []byte(`{"choices":[{"index":0,"delta":{}}],"usage":{"total_tokens":50}}`)},
		}}, nil
	}
	s := susun(t, prov, func(_ *susunan, d *HandlersDeps) { d.Guard = guardUji(t, src) })

	if w := s.panggil(t, http.MethodPost, "/chat/completions", bodyChatStreaming, nil); w.Code != http.StatusOK {
		t.Fatalf("aliran pertama status = %d, mau 200", w.Code)
	}
	// Token dari chunk penutup aliran harus ikut memakan kuota; tanpa itu klien yang selalu
	// memakai streaming tidak pernah dibatasi token.
	w := s.panggil(t, http.MethodPost, "/chat/completions", bodyChatStreaming, nil)
	if w.Code != http.StatusTooManyRequests {
		t.Errorf("aliran kedua status = %d, mau 429 (body: %s)", w.Code, w.Body.String())
	}
}

// --- Tanpa kebijakan ---------------------------------------------------------

// TestTanpaKebijakanSemuanyaLolos: pemasangan yang belum mengisi satu pun tabel kebijakan
// harus berjalan seperti sebelum Fase 8 ada.
func TestTanpaKebijakanSemuanyaLolos(t *testing.T) {
	s := susun(t, providerSukses(), func(_ *susunan, d *HandlersDeps) {
		d.Guard = NewGuard(GuardDeps{Logger: loggerSenyap()})
	})
	for i := 0; i < 3; i++ {
		if w := s.panggil(t, http.MethodPost, "/chat/completions", bodyChatMinimal, nil); w.Code != http.StatusOK {
			t.Fatalf("permintaan ke-%d status = %d, mau 200", i+1, w.Code)
		}
	}
}

// TestEmbeddingsIkutDijagaKebijakan memastikan permukaan kedua tidak terlewat: kebijakan yang
// hanya berlaku di satu endpoint adalah kebijakan yang bisa dilewati dengan mengganti endpoint.
func TestEmbeddingsIkutDijagaKebijakan(t *testing.T) {
	src := &sumberKebijakan{filters: []*policy.ContentFilter{
		filterBaris("f1", "tanpa rahasia", policy.FilterBlockedPattern, func(f *policy.ContentFilter) {
			f.Pattern = "rahasia"
			f.PatternType = policy.PatternSubstring
		}),
	}}
	s := susun(t, providerSukses(), func(_ *susunan, d *HandlersDeps) { d.Guard = guardUji(t, src) })

	w := s.panggil(t, http.MethodPost, "/embeddings",
		`{"model":"gpt-4o-mini","input":["ini rahasia"]}`, nil)
	if w.Code != http.StatusBadRequest {
		t.Fatalf("status = %d, mau 400 (body: %s)", w.Code, w.Body.String())
	}
	if got := kodeGalat(galat(t, w)); got != CodeContentFilter {
		t.Errorf("code = %q, mau %q", got, CodeContentFilter)
	}
}

// Penolakan penyaring konten harus terhitung di metrik, bukan hanya masuk log: log tidak
// bisa dijadikan alert, sehingga aturan yang tiba-tiba memblokir seluruh lalu lintas satu
// penyewa tidak terlihat sampai penyewa itu mengeluh.
func TestPenyaringKontenTerhitungDiMetrik(t *testing.T) {
	src := &sumberKebijakan{filters: []*policy.ContentFilter{
		filterBaris("f1", "tanpa kata rahasia", policy.FilterBlockedPattern, func(f *policy.ContentFilter) {
			f.Pattern = "rahasia"
			f.PatternType = policy.PatternSubstring
		}),
		filterBaris("f2", "hanya peringatan", policy.FilterBlockedPattern, func(f *policy.ContentFilter) {
			f.Pattern = "hati-hati"
			f.PatternType = policy.PatternSubstring
			f.Action = policy.ActionWarn
			f.Priority = 200
		}),
	}}
	metrics := observability.NewMetrics()
	g := NewGuard(GuardDeps{
		Filters: contentfilter.NewEngine(src, loggerSenyap()),
		Metrics: metrics,
		Logger:  loggerSenyap(),
	})

	if rj := g.SaringPermintaan(context.Background(), contentfilter.Subject{Text: "ini rahasia"}); rj == nil {
		t.Fatal("permintaan terlarang tidak ditolak")
	}
	// Aturan berjenis warn TIDAK boleh menaikkan penghitung blokir: ia memang tidak
	// memblokir apa pun, dan menghitungnya membuat angka blokir tidak bisa dipercaya.
	if rj := g.SaringPermintaan(context.Background(), contentfilter.Subject{Text: "hati-hati saja"}); rj != nil {
		t.Fatalf("aturan warn memblokir permintaan: %+v", rj)
	}

	rec := httptest.NewRecorder()
	metrics.Handler().ServeHTTP(rec, httptest.NewRequest(http.MethodGet, "/metrics", nil))
	body := rec.Body.String()

	if !strings.Contains(body, `routex_content_filter_blocked_total{rule="tanpa kata rahasia"} 1`) {
		t.Errorf("penolakan penyaring tidak terhitung; keluaran:\n%s", body)
	}
	if strings.Contains(body, `rule="hanya peringatan"`) {
		t.Error("aturan berjenis warn ikut dihitung sebagai blokir")
	}
}
