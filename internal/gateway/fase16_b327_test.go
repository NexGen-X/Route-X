package gateway

import (
	"context"
	"log/slog"
	"net/http"
	"strings"
	"testing"

	"github.com/alicebob/miniredis/v2"

	"github.com/NexGen-X/Route-X/internal/apikey"
	"github.com/NexGen-X/Route-X/internal/cache"
	"github.com/NexGen-X/Route-X/internal/config"
	"github.com/NexGen-X/Route-X/internal/observability"
	"github.com/NexGen-X/Route-X/internal/security"
)

// TestB327CacheHITMencatatKolomProvider adalah pengunci B-327:
// jalur MISS mencatat pemakaian ke penghitung kolom TPM provider
// (CatatTokenKolomProvider), tetapi ketiga jalur HIT permukaan
// hanya memanggil CatatToken — HIT berulang tidak pernah menyentuh
// kolom provider, jadi batas rate_limit_tpm provider bisa dilewati
// tanpa batas oleh prompt yang ter-cache (sementara jendela token
// key tetap berkurang). Kini HIT mencatat ke kolom provider dengan
// kandidat pemenang entri dan total token yang sama seperti MISS.
func TestB327CacheHITMencatatKolomProvider(t *testing.T) {
	mr := miniredis.RunT(t)
	rdb, err := cache.Connect(context.Background(), &config.Config{
		AppEnv:   config.EnvDevelopment,
		RedisURL: security.Secret("redis://" + mr.Addr()),
	}, slog.New(slog.DiscardHandler))
	if err != nil {
		t.Fatalf("cache.Connect: %v", err)
	}
	t.Cleanup(func() { _ = rdb.Close() })

	limiter := apikey.NewLimiter(rdb, observability.NewMetrics(), loggerSenyap())
	guard := NewGuard(GuardDeps{Limiter: limiter, Logger: loggerSenyap()})
	mesin := mesinCacheAktif(t)

	prov := providerDenganUsage("utama", 40)
	tpm := 1000000
	var providerID string
	s := susun(t, prov, func(s *susunan, d *HandlersDeps) {
		d.ResponseCache = mesin
		d.Guard = guard
		if len(s.cands.cands) > 0 {
			s.cands.cands[0].RateLimitTPM = &tpm
			providerID = s.cands.cands[0].ProviderID
		}
	})
	if providerID == "" {
		t.Fatal("kandidat uji tanpa ProviderID")
	}

	kunciKolom := func() string {
		for _, k := range mr.Keys() {
			if strings.Contains(k, "provider_column") && strings.Contains(k, providerID) {
				return k
			}
		}
		return ""
	}

	w := s.panggil(t, http.MethodPost, "/chat/completions", bodyChat, nil)
	if w.Code != http.StatusOK || w.Header().Get("X-RouteX-Cache") != "MISS" {
		t.Fatalf("pertama: status = %d, cache = %q, mau 200/MISS (%s)", w.Code, w.Header().Get("X-RouteX-Cache"), w.Body.String())
	}
	kunci := kunciKolom()
	if kunci == "" {
		t.Fatal("kunci kolom provider tidak ditemukan sesudah MISS — pencatatan MISS saja tidak terwakili")
	}
	if v, _ := mr.Get(kunci); v != "40" {
		t.Fatalf("counter kolom = %q sesudah MISS, mau \"40\"", v)
	}

	w = s.panggil(t, http.MethodPost, "/chat/completions", bodyChat, nil)
	if w.Code != http.StatusOK || w.Header().Get("X-RouteX-Cache") != "HIT" {
		t.Fatalf("kedua: status = %d, cache = %q, mau 200/HIT (%s)", w.Code, w.Header().Get("X-RouteX-Cache"), w.Body.String())
	}
	if v, _ := mr.Get(kunci); v != "80" {
		t.Errorf("counter kolom = %q sesudah HIT, mau \"80\" — HIT tidak mencatat ke kolom provider (B-327)", v)
	}
}
