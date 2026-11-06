package gateway

import (
	"context"
	"log/slog"
	"strings"
	"sync/atomic"
	"testing"
	"time"

	"github.com/alicebob/miniredis/v2"

	"github.com/NexGen-X/Route-X/internal/apikey"
	"github.com/NexGen-X/Route-X/internal/cache"
	"github.com/NexGen-X/Route-X/internal/config"
	"github.com/NexGen-X/Route-X/internal/database/repo/policy"
	"github.com/NexGen-X/Route-X/internal/observability"
	"github.com/NexGen-X/Route-X/internal/ratelimit"
	"github.com/NexGen-X/Route-X/internal/security"
)

// TestB363SatuBacaanJamUntukJangkarRefund adalah pengunci B-363:
// komentar lama mengklaim kenaikan dan jangkar memakai bacaan jam
// yang sama, tetapi kode membacanya DUA kali — AllowScopes membaca
// jam di dalam limiter, lalu TandaiKuotaModel membaca Now() lagi —
// sehingga di sekitar pergantian jendela refund menghitung dari
// jendela yang berbeda dari kenaikan dan hitungan hantu tertinggal
// di ember kenaikan. Test ini menyuntik jam yang MAJU 61 detik pada
// setiap bacaan sesudah bacaan pertama: bila penilaian dan jangkar
// tidak berbagi satu bacaan persis, refund menurunkan ember yang
// salah dan ember kenaikan tetap "1".
func TestB363SatuBacaanJamUntukJangkarRefund(t *testing.T) {
	mr := miniredis.RunT(t)
	rdb, err := cache.Connect(context.Background(), &config.Config{
		AppEnv:   config.EnvDevelopment,
		RedisURL: security.Secret("redis://" + mr.Addr()),
	}, slog.New(slog.DiscardHandler))
	if err != nil {
		t.Fatalf("cache.Connect: %v", err)
	}
	t.Cleanup(func() { _ = rdb.Close() })

	t0 := time.Date(2026, 9, 2, 10, 30, 30, 0, time.UTC)
	var bacaan atomic.Int64
	jam := func() time.Time {
		if bacaan.Add(1) == 1 {
			return t0
		}
		return t0.Add(61 * time.Second)
	}
	limiter := apikey.NewLimiter(rdb, observability.NewMetrics(), loggerSenyap(),
		apikey.WithClock(jam))
	src := &sumberKebijakan{
		limits: []*policy.RateLimit{{
			Scope: policy.ScopeModel, ScopeID: "m-b363", RequestsPerMinute: pointerG(100), Enabled: true,
		}},
	}
	guard := NewGuard(GuardDeps{
		Rates:   ratelimit.NewEngine(src, loggerSenyap()),
		Limiter: limiter,
		Logger:  loggerSenyap(),
	})

	ctx, rj := guard.PeriksaBatasModel(context.Background(), "m-b363")
	if rj != nil {
		t.Fatalf("PeriksaBatasModel menolak: %+v", rj)
	}

	kunciModel := ""
	for _, k := range mr.Keys() {
		if strings.Contains(k, "m-b363") && strings.Contains(k, "rpm") {
			kunciModel = k
		}
	}
	if kunciModel == "" {
		t.Fatal("kunci rpm cakupan model tidak ditemukan sesudah kenaikan")
	}
	if v, _ := mr.Get(kunciModel); v != "1" {
		t.Fatalf("counter %q = %q sesudah kenaikan, mau \"1\"", kunciModel, v)
	}

	guard.KembalikanKuotaModel(ctx, "m-b363")
	if v, _ := mr.Get(kunciModel); v != "0" {
		t.Errorf("counter ember kenaikan %q = %q sesudah refund, mau \"0\" — jangkar refund tidak berbagi bacaan jam dengan kenaikan", kunciModel, v)
	}
}
