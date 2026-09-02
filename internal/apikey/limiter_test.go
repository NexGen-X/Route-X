package apikey

import (
	"context"
	"encoding/json"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"net/netip"
	"strconv"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/alicebob/miniredis/v2"
	"github.com/prometheus/client_golang/prometheus"

	"github.com/NexGen-X/Route-X/internal/cache"
	"github.com/NexGen-X/Route-X/internal/config"
	"github.com/NexGen-X/Route-X/internal/database/repo/keys"
	"github.com/NexGen-X/Route-X/internal/httpx"
	"github.com/NexGen-X/Route-X/internal/observability"
	"github.com/NexGen-X/Route-X/internal/security"
)

// --- Perkakas uji ------------------------------------------------------------

// limiterEnv adalah satu pembatas beserta Redis tiruan di belakangnya.
type limiterEnv struct {
	limiter *Limiter
	mr      *miniredis.Miniredis
	rdb     *cache.Redis
	metrics *observability.Metrics
	now     time.Time
}

// newLimiterEnv menyalakan Redis tiruan lalu menyambungkannya lewat cache.Connect,
// sehingga jalur yang diuji sama dengan jalur produksi (termasuk EVALSHA dan opsi pool).
//
// Jamnya dibekukan. Pembatas ini memasukkan nomor jendela ke dalam kunci, jadi tanpa jam
// yang dikuasai test, sebuah test yang berjalan persis di perbatasan detik akan
// menghitung dua jendela berbeda dan gagal secara acak.
func newLimiterEnv(t *testing.T, opts ...LimiterOption) *limiterEnv {
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

	metrics := observability.NewMetrics()
	env := &limiterEnv{
		limiter: NewLimiter(rdb, metrics, slog.New(slog.DiscardHandler), opts...),
		mr:      mr,
		rdb:     rdb,
		metrics: metrics,
		now:     time.Date(2026, 9, 2, 10, 30, 30, 0, time.UTC),
	}
	env.freeze(env.now)
	return env
}

// freeze memaku jam pembatas pada satu titik waktu.
func (e *limiterEnv) freeze(at time.Time) {
	e.now = at
	e.limiter.now = func() time.Time { return at }
}

// advance memindahkan jam pembatas ke depan tanpa menunggu waktu nyata.
func (e *limiterEnv) advance(d time.Duration) { e.freeze(e.now.Add(d)) }

const testKeyID = "8c6f0a0e-0f2a-4a1c-9f0e-2d0b8a7c1f11"

// counterValue membaca satu penghitung dari registry aplikasi.
//
// Registry dibaca langsung, bukan lewat prometheus/testutil: paket itu menarik
// dependensi baru ke dalam modul (promlint dan turunannya) sementara yang dibutuhkan di
// sini cuma satu angka.
func counterValue(t *testing.T, reg *prometheus.Registry, name string, labels map[string]string) float64 {
	t.Helper()
	families, err := reg.Gather()
	if err != nil {
		t.Fatalf("mengumpulkan metrik: %v", err)
	}
	for _, family := range families {
		if family.GetName() != name {
			continue
		}
		for _, metric := range family.GetMetric() {
			match := true
			for _, pair := range metric.GetLabel() {
				if want, ok := labels[pair.GetName()]; ok && want != pair.GetValue() {
					match = false
					break
				}
			}
			if match {
				return metric.GetCounter().GetValue()
			}
		}
	}
	return 0
}

var testIP = netip.MustParseAddr("203.0.113.7")

// --- Kasus tanpa Redis -------------------------------------------------------

// Pembatas nil dan pembatas tanpa Redis meloloskan semuanya, supaya deployment tanpa
// Redis memakai jalur kode yang sama.
func TestLimiterWithoutRedis(t *testing.T) {
	ctx := context.Background()
	limits := Limits{RPS: 1, RPM: 1, Daily: 1, Monthly: 1}

	var nilLimiter *Limiter
	for name, l := range map[string]*Limiter{
		"nil":         nilLimiter,
		"tanpa redis": NewLimiter(nil, nil, nil),
	} {
		for i := 0; i < 5; i++ {
			if d := l.Allow(ctx, testKeyID, limits, testIP); !d.Allowed {
				t.Fatalf("%s: request %d ditolak padahal Redis tidak dikonfigurasi", name, i)
			}
		}
		if d := l.RecordTokens(ctx, testKeyID, TokenLimits{TPM: 1}, 1000); !d.Allowed {
			t.Errorf("%s: RecordTokens melaporkan anggaran habis", name)
		}
		if d := l.TokensExceeded(ctx, testKeyID, TokenLimits{TPM: 1}); !d.Allowed {
			t.Errorf("%s: TokensExceeded melaporkan anggaran habis", name)
		}
		if err := l.Warm(ctx); err != nil {
			t.Errorf("%s: Warm = %v", name, err)
		}
	}
}

// Tanpa satu pun batas aktif, Redis tidak boleh disentuh sama sekali dan tidak ada header
// kuota yang dipasang.
func TestAllowWithoutLimitsSkipsRedis(t *testing.T) {
	env := newLimiterEnv(t)

	d := env.limiter.Allow(context.Background(), testKeyID, Limits{}, testIP)
	if !d.Allowed || d.Degraded {
		t.Fatalf("keputusan = %+v", d)
	}
	if d.LimitValue != 0 || d.Scope != "" {
		t.Errorf("keputusan melaporkan batas padahal tidak ada: %+v", d)
	}
	if keys := env.mr.Keys(); len(keys) != 0 {
		t.Errorf("kunci yang ditulis = %v, mau kosong", keys)
	}
}

// --- Penegakan batas request -------------------------------------------------

func TestAllowUnderAndOverLimit(t *testing.T) {
	env := newLimiterEnv(t)
	ctx := context.Background()
	limits := Limits{RPS: 3}

	for i := 1; i <= 3; i++ {
		d := env.limiter.Allow(ctx, testKeyID, limits, netip.Addr{})
		if !d.Allowed {
			t.Fatalf("request %d ditolak: %+v", i, d)
		}
		if want := int64(3 - i); d.Remaining != want {
			t.Errorf("request %d: Remaining = %d, mau %d", i, d.Remaining, want)
		}
		if d.LimitValue != 3 || d.Scope != ScopeAPIKey || d.Limit != LimitRPS {
			t.Errorf("request %d: laporan batas = %+v", i, d)
		}
	}

	d := env.limiter.Allow(ctx, testKeyID, limits, netip.Addr{})
	if d.Allowed {
		t.Fatal("request keempat lolos padahal batasnya tiga")
	}
	if d.Scope != ScopeAPIKey || d.Limit != LimitRPS {
		t.Errorf("penolakan dilaporkan sebagai %s/%s", d.Scope, d.Limit)
	}
	if d.Remaining != 0 {
		t.Errorf("Remaining = %d, mau 0", d.Remaining)
	}
	if d.RetryAfter <= 0 || d.RetryAfter > time.Second {
		t.Errorf("RetryAfter = %v, mau di antara 0 dan satu detik", d.RetryAfter)
	}
	if want := env.now.Truncate(time.Second).Add(time.Second); !d.ResetAt.Equal(want) {
		t.Errorf("ResetAt = %v, mau %v", d.ResetAt, want)
	}
	rejected := counterValue(t, env.metrics.Registry(), "routex_rate_limit_rejected_total",
		map[string]string{"scope": ScopeAPIKey, "limit": LimitRPS})
	if rejected != 1 {
		t.Errorf("metrik penolakan = %v, mau 1", rejected)
	}
}

// Jendela berganti berarti kuota pulih tanpa ada yang perlu membersihkan penghitung lama.
func TestAllowRecoversAfterWindow(t *testing.T) {
	env := newLimiterEnv(t)
	ctx := context.Background()
	limits := Limits{RPS: 2}

	for i := 0; i < 2; i++ {
		if d := env.limiter.Allow(ctx, testKeyID, limits, netip.Addr{}); !d.Allowed {
			t.Fatalf("request %d ditolak", i)
		}
	}
	if d := env.limiter.Allow(ctx, testKeyID, limits, netip.Addr{}); d.Allowed {
		t.Fatal("request ketiga lolos di jendela yang sama")
	}

	env.advance(time.Second)
	if d := env.limiter.Allow(ctx, testKeyID, limits, netip.Addr{}); !d.Allowed {
		t.Error("kuota tidak pulih setelah jendela berganti")
	}
}

// Request yang ditolak satu batas tidak boleh memakan kuota batas lain. Tanpa sifat ini,
// klien yang mengirim terlalu cepat akan menghabiskan kuota hariannya tanpa satu pun
// request berhasil.
func TestRejectionDoesNotConsumeOtherWindows(t *testing.T) {
	env := newLimiterEnv(t)
	ctx := context.Background()
	limits := Limits{RPS: 1, Daily: 5}

	// Detik pertama: satu lolos, dua ditolak batas per detik.
	if d := env.limiter.Allow(ctx, testKeyID, limits, netip.Addr{}); !d.Allowed {
		t.Fatal("request pertama ditolak")
	}
	for i := 0; i < 2; i++ {
		d := env.limiter.Allow(ctx, testKeyID, limits, netip.Addr{})
		if d.Allowed || d.Limit != LimitRPS {
			t.Fatalf("request tambahan di detik yang sama: %+v", d)
		}
	}

	// Empat detik berikutnya masing-masing meloloskan satu request. Kalau penolakan di
	// atas ikut menaikkan penghitung harian, kuota harian sudah habis di sini.
	for i := 0; i < 4; i++ {
		env.advance(time.Second)
		if d := env.limiter.Allow(ctx, testKeyID, limits, netip.Addr{}); !d.Allowed {
			t.Fatalf("detik ke-%d ditolak: %+v", i+2, d)
		}
	}

	// Kuota harian lima sudah terpakai habis oleh lima request yang lolos.
	env.advance(time.Second)
	d := env.limiter.Allow(ctx, testKeyID, limits, netip.Addr{})
	if d.Allowed {
		t.Fatal("request keenam lolos padahal kuota harian lima")
	}
	if d.Limit != LimitDaily {
		t.Errorf("penolakan dilaporkan sebagai %s, mau %s", d.Limit, LimitDaily)
	}
	if want := time.Date(2026, 9, 3, 0, 0, 0, 0, time.UTC); !d.ResetAt.Equal(want) {
		t.Errorf("ResetAt = %v, mau tengah malam UTC %v", d.ResetAt, want)
	}
}

// N goroutine yang berebut kuota M harus menghasilkan tepat M yang lolos. Inilah yang
// hilang kalau pemeriksaan dan penaikan penghitung tidak berada dalam satu skrip.
func TestAllowIsAtomicUnderConcurrency(t *testing.T) {
	env := newLimiterEnv(t)
	ctx := context.Background()

	const (
		goroutines = 60
		quota      = 10
	)
	// RPM, bukan RPS: jamnya dibekukan, tapi memakai jendela menit membuat test ini
	// tetap benar walau kelak jam bekunya dilepas.
	limits := Limits{RPM: quota}

	var (
		wg      sync.WaitGroup
		mu      sync.Mutex
		allowed int
	)
	start := make(chan struct{})
	for i := 0; i < goroutines; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			<-start
			if d := env.limiter.Allow(ctx, testKeyID, limits, netip.Addr{}); d.Allowed {
				mu.Lock()
				allowed++
				mu.Unlock()
			}
		}()
	}
	close(start)
	wg.Wait()

	if allowed != quota {
		t.Errorf("yang lolos = %d, mau tepat %d", allowed, quota)
	}
}

// Setiap kunci penghitung wajib punya TTL. Kunci tanpa TTL berarti batas yang macet
// selamanya, dan itu justru kegagalan yang paling sulit disadari.
func TestCounterKeysAlwaysHaveTTL(t *testing.T) {
	env := newLimiterEnv(t)
	ctx := context.Background()

	// Seluruh jenis jendela dipakai sekaligus: per detik, per menit, harian, bulanan,
	// untuk cakupan key maupun IP, plus penghitung token.
	env.limiter.ipLimits = Limits{RPS: 5, RPM: 5, Daily: 5, Monthly: 5}
	env.limiter.Allow(ctx, testKeyID, Limits{RPS: 5, RPM: 5, Daily: 5, Monthly: 5}, testIP)
	env.limiter.RecordTokens(ctx, testKeyID, TokenLimits{TPM: 100, Daily: 100, Monthly: 100}, 10)

	all := env.mr.Keys()
	if len(all) != 11 {
		t.Fatalf("jumlah kunci = %d (%v), mau 11", len(all), all)
	}
	client := env.rdb.Client()
	for _, key := range all {
		ttl, err := client.PTTL(ctx, key).Result()
		if err != nil {
			t.Fatalf("PTTL %s: %v", key, err)
		}
		if ttl <= 0 {
			t.Errorf("kunci %s tidak punya TTL (PTTL = %v)", key, ttl)
		}
	}
}

// Kunci wajib bernamespace lewat cache.RateLimitKey, dan jenis batas wajib ikut ke dalam
// kunci: nomor detik epoch dan tanggal bergaya 20260902 hidup di ruang angka yang sama.
func TestCounterKeyShape(t *testing.T) {
	env := newLimiterEnv(t)
	env.limiter.Allow(context.Background(), testKeyID, Limits{RPS: 1, Daily: 1}, netip.Addr{})

	want := map[string]bool{
		cache.RateLimitKey(ScopeAPIKey, testKeyID, LimitRPS+"-"+strconv.FormatInt(env.now.Unix(), 10)): true,
		cache.RateLimitKey(ScopeAPIKey, testKeyID, LimitDaily+"-20260902"):                             true,
	}
	for _, key := range env.mr.Keys() {
		if !want[key] {
			t.Errorf("kunci tak terduga: %s", key)
		}
		delete(want, key)
	}
	for key := range want {
		t.Errorf("kunci tidak ditulis: %s", key)
	}
}

// Redis mati harus meloloskan request, bukan menolaknya, dan itu harus terlihat di log
// dan metrik.
func TestAllowFailsOpenWhenRedisDown(t *testing.T) {
	env := newLimiterEnv(t)
	logs := &safeBuffer{}
	env.limiter.logger = slog.New(slog.NewJSONHandler(logs, nil))
	env.mr.Close()

	// Peristiwa gagal-terbuka harus bisa dikorelasikan dengan request yang mengalaminya.
	const requestID = "req-uji-9876543210"
	ctx := observability.WithRequestID(context.Background(), requestID)

	d := env.limiter.Allow(ctx, testKeyID, Limits{RPS: 1}, testIP)
	if !d.Allowed {
		t.Fatal("request ditolak padahal Redis mati")
	}
	if !d.Degraded {
		t.Error("keputusan tidak ditandai Degraded")
	}
	if got := counterValue(t, env.metrics.Registry(), "routex_rate_limit_failopen_total",
		map[string]string{"op": "enforce"}); got != 1 {
		t.Errorf("metrik gagal-terbuka = %v, mau 1", got)
	}
	out := logs.String()
	if out == "" {
		t.Fatal("gagal-terbuka tidak dicatat di log")
	}
	if !strings.Contains(out, requestID) {
		t.Errorf("log gagal-terbuka tidak memuat request_id: %s", out)
	}
}

// --- Cakupan IP dan bawaan ---------------------------------------------------

// Batas per IP ditegakkan terpisah dari batas per key, dan penolakannya dilaporkan
// dengan cakupan yang benar.
func TestAllowEnforcesIPLimits(t *testing.T) {
	env := newLimiterEnv(t, WithIPLimits(Limits{RPS: 2}))
	ctx := context.Background()

	// Key tanpa batas sendiri: yang membatasi murni cakupan IP.
	for i := 0; i < 2; i++ {
		if d := env.limiter.Allow(ctx, testKeyID, Limits{}, testIP); !d.Allowed {
			t.Fatalf("request %d ditolak", i)
		}
	}
	d := env.limiter.Allow(ctx, testKeyID, Limits{}, testIP)
	if d.Allowed {
		t.Fatal("request ketiga dari IP yang sama lolos")
	}
	if d.Scope != ScopeIP || d.Limit != LimitRPS {
		t.Errorf("penolakan dilaporkan sebagai %s/%s, mau %s/%s", d.Scope, d.Limit, ScopeIP, LimitRPS)
	}

	// IP lain punya ember sendiri.
	if d := env.limiter.Allow(ctx, testKeyID, Limits{}, netip.MustParseAddr("198.51.100.9")); !d.Allowed {
		t.Error("IP lain ikut terkena batas")
	}
	// Alamat yang tidak diketahui melewatkan cakupan IP sepenuhnya.
	if d := env.limiter.Allow(ctx, testKeyID, Limits{}, netip.Addr{}); !d.Allowed {
		t.Error("request tanpa IP ikut terkena batas per IP")
	}
}

// Batas bawaan mengisi kolom yang kosong satu per satu, bukan menggantikan seluruh
// batas key.
func TestKeyLimitDefaultsMergePerField(t *testing.T) {
	env := newLimiterEnv(t, WithKeyLimitDefaults(Limits{RPS: 100, RPM: 2}))
	ctx := context.Background()

	// Key hanya menyetel RPS; RPM diwarisi dari bawaan.
	limits := Limits{RPS: 50}
	for i := 0; i < 2; i++ {
		if d := env.limiter.Allow(ctx, testKeyID, limits, netip.Addr{}); !d.Allowed {
			t.Fatalf("request %d ditolak", i)
		}
	}
	d := env.limiter.Allow(ctx, testKeyID, limits, netip.Addr{})
	if d.Allowed {
		t.Fatal("batas RPM bawaan tidak ditegakkan")
	}
	if d.Limit != LimitRPM {
		t.Errorf("penolakan dilaporkan sebagai %s, mau %s", d.Limit, LimitRPM)
	}
	if d.LimitValue != 2 {
		t.Errorf("LimitValue = %d, mau 2 (bawaan), bukan nilai key", d.LimitValue)
	}
}

// --- Middleware --------------------------------------------------------------

func TestLimitMiddleware(t *testing.T) {
	env := newLimiterEnv(t)
	rps := 2
	key := testKey()
	key.RateLimitRPS = &rps

	handler := httpx.RealIP(nil)(env.limiter.Limit()(okHandler(nil)))
	request := func() *httptest.ResponseRecorder {
		r := httptest.NewRequest("POST", "/v1/chat/completions", nil)
		r = r.WithContext(WithPrincipal(r.Context(), &Principal{Key: key}))
		rec := httptest.NewRecorder()
		handler.ServeHTTP(rec, r)
		return rec
	}

	// Respons yang lolos pun membawa header kuota: itulah yang dipakai klien untuk
	// memperlambat diri sebelum menabrak batas.
	rec := request()
	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d, mau 200", rec.Code)
	}
	if got := rec.Header().Get(HeaderRateLimitLimit); got != "2" {
		t.Errorf("%s = %q, mau 2", HeaderRateLimitLimit, got)
	}
	if got := rec.Header().Get(HeaderRateLimitRemaining); got != "1" {
		t.Errorf("%s = %q, mau 1", HeaderRateLimitRemaining, got)
	}
	wantReset := strconv.FormatInt(env.now.Truncate(time.Second).Add(time.Second).Unix(), 10)
	if got := rec.Header().Get(HeaderRateLimitReset); got != wantReset {
		t.Errorf("%s = %q, mau %q", HeaderRateLimitReset, got, wantReset)
	}

	if rec := request(); rec.Code != http.StatusOK {
		t.Fatalf("request kedua: status = %d, mau 200", rec.Code)
	}

	rec = request()
	if rec.Code != http.StatusTooManyRequests {
		t.Fatalf("status = %d, mau 429 (body: %s)", rec.Code, rec.Body.String())
	}
	if got := rec.Header().Get("Retry-After"); got != "1" {
		t.Errorf("Retry-After = %q, mau 1", got)
	}
	if got := rec.Header().Get(HeaderRateLimitRemaining); got != "0" {
		t.Errorf("%s = %q, mau 0", HeaderRateLimitRemaining, got)
	}
	if got := rec.Header().Get(HeaderRateLimitReset); got != wantReset {
		t.Errorf("%s = %q, mau %q", HeaderRateLimitReset, got, wantReset)
	}

	var envelope httpx.ErrorResponse
	if err := json.Unmarshal(rec.Body.Bytes(), &envelope); err != nil {
		t.Fatalf("body bukan JSON: %v", err)
	}
	if envelope.Error.Type != httpx.ErrTypeRateLimit {
		t.Errorf("error.type = %q, mau %q", envelope.Error.Type, httpx.ErrTypeRateLimit)
	}
	if envelope.Error.Code == nil || *envelope.Error.Code != httpx.CodeRateLimitExceeded {
		t.Errorf("error.code = %v, mau %q", envelope.Error.Code, httpx.CodeRateLimitExceeded)
	}
	if envelope.Error.Message != MessageRateLimited {
		t.Errorf("error.message = %q", envelope.Error.Message)
	}
}

// Rute tanpa Authenticate di depannya tidak boleh diloloskan tanpa batas.
func TestLimitMiddlewareWithoutPrincipal(t *testing.T) {
	env := newLimiterEnv(t)

	rec := httptest.NewRecorder()
	next := http.HandlerFunc(func(http.ResponseWriter, *http.Request) {
		t.Error("handler dijalankan tanpa principal")
	})
	env.limiter.Limit()(next).ServeHTTP(rec, httptest.NewRequest("POST", "/v1/chat/completions", nil))

	if rec.Code != http.StatusUnauthorized {
		t.Errorf("status = %d, mau 401", rec.Code)
	}
}

// Batas token TIDAK ditegakkan middleware: jumlah token baru diketahui setelah respons
// upstream selesai. Key yang anggaran tokennya sudah habis tetap lolos di sini, dan
// penegakannya menjadi tugas jalur gateway lewat TokensExceeded.
func TestLimitMiddlewareIgnoresTokenLimits(t *testing.T) {
	env := newLimiterEnv(t)
	ctx := context.Background()

	tpm := 100
	key := testKey()
	key.RateLimitTPM = &tpm

	if d := env.limiter.RecordTokens(ctx, key.ID, TokenLimitsFromKey(key), 500); d.Allowed {
		t.Fatal("anggaran token seharusnya sudah habis")
	}

	r := httptest.NewRequest("POST", "/v1/chat/completions", nil)
	r = r.WithContext(WithPrincipal(r.Context(), &Principal{Key: key}))
	rec := httptest.NewRecorder()
	env.limiter.Limit()(okHandler(nil)).ServeHTTP(rec, r)

	if rec.Code != http.StatusOK {
		t.Errorf("status = %d, mau 200: batas token bukan urusan middleware", rec.Code)
	}
}

// --- Batas token -------------------------------------------------------------

func TestRecordTokensAndTokensExceeded(t *testing.T) {
	env := newLimiterEnv(t)
	ctx := context.Background()
	limits := TokenLimits{TPM: 100}

	d := env.limiter.RecordTokens(ctx, testKeyID, limits, 60)
	if !d.Allowed {
		t.Fatalf("anggaran dilaporkan habis setelah 60 dari 100: %+v", d)
	}
	if d.Remaining != 40 {
		t.Errorf("Remaining = %d, mau 40", d.Remaining)
	}

	d = env.limiter.RecordTokens(ctx, testKeyID, limits, 60)
	if d.Allowed {
		t.Fatal("anggaran tidak dilaporkan habis setelah 120 dari 100")
	}
	if d.Limit != LimitTPM || d.Scope != ScopeAPIKey {
		t.Errorf("laporan = %s/%s, mau %s/%s", d.Scope, d.Limit, ScopeAPIKey, LimitTPM)
	}

	// Pencatatan bukan penolakan: request-nya sudah dilayani, jadi ia tidak boleh
	// terhitung di metrik penolakan.
	if got := counterValue(t, env.metrics.Registry(), "routex_rate_limit_rejected_total",
		map[string]string{"scope": ScopeAPIKey, "limit": LimitTPM}); got != 0 {
		t.Errorf("metrik penolakan setelah RecordTokens = %v, mau 0", got)
	}

	// Peek melaporkan hal yang sama tanpa mengubah penghitung.
	key := cache.RateLimitKey(ScopeAPIKey, testKeyID, LimitTPM+"-"+strconv.FormatInt(env.now.Unix()/60, 10))
	before, err := env.rdb.Client().Get(ctx, key).Result()
	if err != nil {
		t.Fatalf("GET %s: %v", key, err)
	}
	if d := env.limiter.TokensExceeded(ctx, testKeyID, limits); d.Allowed {
		t.Error("TokensExceeded melaporkan anggaran masih ada")
	}
	after, err := env.rdb.Client().Get(ctx, key).Result()
	if err != nil {
		t.Fatalf("GET %s: %v", key, err)
	}
	if before != after {
		t.Errorf("penghitung berubah dari %s menjadi %s, padahal hanya dilihat", before, after)
	}

	// Gerbang sebelum request diteruskan: hasil "habis" ikut terhitung sebagai penolakan.
	if got := counterValue(t, env.metrics.Registry(), "routex_rate_limit_rejected_total",
		map[string]string{"scope": ScopeAPIKey, "limit": LimitTPM}); got != 1 {
		t.Errorf("metrik penolakan setelah TokensExceeded = %v, mau 1", got)
	}

	// Jendela menit berganti: anggaran pulih.
	env.advance(time.Minute)
	if d := env.limiter.TokensExceeded(ctx, testKeyID, limits); !d.Allowed {
		t.Error("anggaran token tidak pulih setelah jendela berganti")
	}
}

// Token nol tidak menulis apa pun; ia hanya melaporkan keadaan sekarang.
func TestRecordZeroTokensWritesNothing(t *testing.T) {
	env := newLimiterEnv(t)

	if d := env.limiter.RecordTokens(context.Background(), testKeyID, TokenLimits{TPM: 10}, 0); !d.Allowed {
		t.Errorf("keputusan = %+v", d)
	}
	if got := env.mr.Keys(); len(got) != 0 {
		t.Errorf("kunci yang ditulis = %v, mau kosong", got)
	}
}

// Tanpa keyID tidak ada pemilik kuota, jadi tidak ada yang bisa dicatat.
func TestRecordTokensWithoutKeyID(t *testing.T) {
	env := newLimiterEnv(t)

	if d := env.limiter.RecordTokens(context.Background(), "", TokenLimits{TPM: 10}, 5); !d.Allowed {
		t.Errorf("keputusan = %+v", d)
	}
	if got := env.mr.Keys(); len(got) != 0 {
		t.Errorf("kunci yang ditulis = %v, mau kosong", got)
	}
}

// Batas harian dan bulanan token dihitung di jendela kalender UTC.
func TestTokenDailyAndMonthlyWindows(t *testing.T) {
	env := newLimiterEnv(t)
	ctx := context.Background()
	limits := TokenLimits{Daily: 100, Monthly: 150}

	if d := env.limiter.RecordTokens(ctx, testKeyID, limits, 100); d.Allowed {
		t.Fatalf("anggaran harian seharusnya habis: %+v", d)
	}

	// Hari berganti: kuota harian pulih, kuota bulanan tidak.
	env.advance(24 * time.Hour)
	d := env.limiter.RecordTokens(ctx, testKeyID, limits, 50)
	if d.Allowed {
		t.Fatalf("anggaran bulanan seharusnya habis: %+v", d)
	}
	if d.Limit != LimitMonthlyTokens {
		t.Errorf("laporan = %s, mau %s", d.Limit, LimitMonthlyTokens)
	}
	if want := time.Date(2026, 10, 1, 0, 0, 0, 0, time.UTC); !d.ResetAt.Equal(want) {
		t.Errorf("ResetAt = %v, mau %v", d.ResetAt, want)
	}
}

// --- Pembacaan batas dan header ----------------------------------------------

func TestLimitsFromKey(t *testing.T) {
	if got := LimitsFromKey(nil); !got.IsZero() {
		t.Errorf("LimitsFromKey(nil) = %+v", got)
	}
	if got := TokenLimitsFromKey(nil); !got.IsZero() {
		t.Errorf("TokenLimitsFromKey(nil) = %+v", got)
	}

	rps, rpm, tpm := 5, 100, 1000
	daily, monthly := int64(10_000), int64(200_000)
	key := &keys.Key{
		RateLimitRPS: &rps, RateLimitRPM: &rpm, RateLimitTPM: &tpm,
		DailyRequestLimit: &daily, MonthlyRequestLimit: &monthly,
		DailyTokenLimit: &daily, MonthlyTokenLimit: &monthly,
	}
	want := Limits{RPS: 5, RPM: 100, Daily: 10_000, Monthly: 200_000}
	if got := LimitsFromKey(key); got != want {
		t.Errorf("LimitsFromKey = %+v, mau %+v", got, want)
	}
	wantTokens := TokenLimits{TPM: 1000, Daily: 10_000, Monthly: 200_000}
	if got := TokenLimitsFromKey(key); got != wantTokens {
		t.Errorf("TokenLimitsFromKey = %+v, mau %+v", got, wantTokens)
	}

	// Kolom yang NULL berarti tidak ditegakkan, bukan nol yang menolak semuanya.
	if got := LimitsFromKey(&keys.Key{}); !got.IsZero() {
		t.Errorf("LimitsFromKey(kosong) = %+v", got)
	}
}

func TestDecisionSetHeaders(t *testing.T) {
	reset := time.Date(2026, 9, 2, 10, 30, 31, 0, time.UTC)

	// Tanpa batas aktif tidak ada header sama sekali: X-RateLimit-Limit bernilai nol
	// akan dibaca klien sebagai kuota habis.
	rec := httptest.NewRecorder()
	Decision{Allowed: true}.SetHeaders(rec)
	if len(rec.Header()) != 0 {
		t.Errorf("header = %v, mau kosong", rec.Header())
	}

	rec = httptest.NewRecorder()
	Decision{LimitValue: 10, Remaining: -5, ResetAt: reset}.SetHeaders(rec)
	if got := rec.Header().Get(HeaderRateLimitLimit); got != "10" {
		t.Errorf("%s = %q", HeaderRateLimitLimit, got)
	}
	if got := rec.Header().Get(HeaderRateLimitRemaining); got != "0" {
		t.Errorf("%s = %q, sisa negatif harus dipangkas ke 0", HeaderRateLimitRemaining, got)
	}
	if got := rec.Header().Get(HeaderRateLimitReset); got != strconv.FormatInt(reset.Unix(), 10) {
		t.Errorf("%s = %q", HeaderRateLimitReset, got)
	}

	// Penerima nil pada ResponseWriter tidak boleh panik.
	Decision{LimitValue: 10}.SetHeaders(nil)
}

// Skrip harus bisa dimuat lebih dulu supaya request pertama tidak menanggung EVAL penuh.
func TestWarmLoadsScript(t *testing.T) {
	env := newLimiterEnv(t)
	if err := env.limiter.Warm(context.Background()); err != nil {
		t.Fatalf("Warm: %v", err)
	}
	if d := env.limiter.Allow(context.Background(), testKeyID, Limits{RPS: 1}, netip.Addr{}); !d.Allowed {
		t.Errorf("keputusan setelah pemanasan = %+v", d)
	}
}

// --- Bagian dalam ------------------------------------------------------------

// Dua Limiter di atas registry yang sama memakai penghitung gagal-terbuka yang sama.
func TestLimiterMetricsSharedAcrossInstances(t *testing.T) {
	metrics := observability.NewMetrics()
	first := NewLimiter(nil, metrics, nil)
	second := NewLimiter(nil, metrics, nil)
	if first.failOpen != second.failOpen {
		t.Error("Limiter kedua tidak memakai penghitung yang sudah terdaftar")
	}
	if first.rejected != metrics.RateLimitRejected {
		t.Error("penghitung penolakan tidak diambil dari observability.Metrics")
	}
}

// Kembalian skrip yang tidak dikenali harus terdeteksi, bukan ditebak: dari data yang
// tidak dipahami tidak boleh lahir keputusan penolakan.
func TestParseScriptResult(t *testing.T) {
	tests := []struct {
		name         string
		res          any
		n            int
		wantOK       bool
		wantExceeded int
		wantCounts   []int64
	}{
		{
			name: "lolos", res: []any{int64(0), int64(3), int64(7)}, n: 2,
			wantOK: true, wantExceeded: 0, wantCounts: []int64{3, 7},
		},
		{
			name: "terlampaui", res: []any{int64(2), int64(3), int64(7)}, n: 2,
			wantOK: true, wantExceeded: 2, wantCounts: []int64{3, 7},
		},
		{name: "bukan slice", res: int64(0), n: 1},
		{name: "panjang salah", res: []any{int64(0)}, n: 1},
		{name: "indeks bukan angka", res: []any{"0", int64(1)}, n: 1},
		{name: "indeks negatif", res: []any{int64(-1), int64(1)}, n: 1},
		{name: "indeks di luar jangkauan", res: []any{int64(2), int64(1)}, n: 1},
		{name: "hitungan bukan angka", res: []any{int64(0), "1"}, n: 1},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			exceeded, counts, ok := parseScriptResult(tc.res, tc.n)
			if ok != tc.wantOK {
				t.Fatalf("ok = %v, mau %v", ok, tc.wantOK)
			}
			if !ok {
				return
			}
			if exceeded != tc.wantExceeded {
				t.Errorf("exceeded = %d, mau %d", exceeded, tc.wantExceeded)
			}
			if len(counts) != len(tc.wantCounts) {
				t.Fatalf("counts = %v", counts)
			}
			for i := range counts {
				if counts[i] != tc.wantCounts[i] {
					t.Errorf("counts[%d] = %d, mau %d", i, counts[i], tc.wantCounts[i])
				}
			}
		})
	}
}

// TTL nol atau negatif akan membuat PEXPIRE justru menghapus kuncinya seketika, sehingga
// jendela yang sedang berjalan kehilangan penghitungnya.
func TestTTLMillisNeverZero(t *testing.T) {
	now := time.Date(2026, 9, 2, 10, 30, 30, 0, time.UTC)
	tests := []struct {
		name    string
		resetAt time.Time
		want    int64
	}{
		{name: "sisa satu detik", resetAt: now.Add(time.Second), want: 1000},
		{name: "sisa nol", resetAt: now, want: 1},
		{name: "sudah lewat", resetAt: now.Add(-time.Minute), want: 1},
		{name: "sisa di bawah satu milidetik", resetAt: now.Add(500 * time.Microsecond), want: 1},
	}
	for _, tc := range tests {
		if got := ttlMillis(window{resetAt: tc.resetAt}, now); got != tc.want {
			t.Errorf("%s: ttlMillis = %d, mau %d", tc.name, got, tc.want)
		}
	}
}

// Pada sisa kuota yang sama, jendela yang lebih cepat pulih yang dilaporkan: klien tidak
// boleh menahan diri lebih lama dari perlunya.
func TestDecisionFromPrefersEarliestReset(t *testing.T) {
	now := time.Date(2026, 9, 2, 10, 30, 30, 0, time.UTC)
	windows := []window{
		{scope: ScopeAPIKey, limit: LimitDaily, value: 10, resetAt: now.Add(time.Hour)},
		{scope: ScopeAPIKey, limit: LimitRPM, value: 10, resetAt: now.Add(time.Minute)},
	}
	d := decisionFrom(windows, []int64{5, 5}, 0, now)
	if !d.Allowed {
		t.Fatalf("keputusan = %+v", d)
	}
	if d.Limit != LimitRPM {
		t.Errorf("Limit = %s, mau %s", d.Limit, LimitRPM)
	}
	if d.Remaining != 5 {
		t.Errorf("Remaining = %d, mau 5", d.Remaining)
	}

	// Yang terlampaui selalu menang atas pemilihan di atas.
	d = decisionFrom(windows, []int64{10, 5}, 1, now)
	if d.Allowed || d.Limit != LimitDaily || d.Remaining != 0 {
		t.Errorf("keputusan = %+v", d)
	}
	if d.RetryAfter != time.Hour {
		t.Errorf("RetryAfter = %v, mau 1h", d.RetryAfter)
	}

	// Jendela yang sudah lewat (jam bergerak di antara penyusunan dan pembacaan) tidak
	// boleh menghasilkan RetryAfter negatif, dan hitungan yang melampaui batas tidak
	// boleh menghasilkan sisa kuota negatif.
	stale := []window{{scope: ScopeAPIKey, limit: LimitRPS, value: 2, resetAt: now.Add(-time.Second)}}
	if d := decisionFrom(stale, []int64{5}, 1, now); d.RetryAfter != 0 {
		t.Errorf("RetryAfter = %v, mau 0", d.RetryAfter)
	}
	if d := decisionFrom(stale, []int64{5}, 0, now); !d.Allowed || d.Remaining != 0 {
		t.Errorf("keputusan = %+v, mau lolos dengan sisa 0", d)
	}
}
