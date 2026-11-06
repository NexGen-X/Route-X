package gateway

import (
	"context"
	"log/slog"
	"strings"
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

// TestB353RefundModelBerjangkarKeJendelaKenaikan adalah pengunci
// sisi gateway dari B-353: kenaikan cakupan model dari
// PeriksaBatasModel ditandai pada tanda bersama, dan refundnya
// (KembalikanKuotaModel lewat kembalikanSatuKuotaModel) dulu
// menyusun jendela dari jam SAAT REFUND. Refund yang menyeberangi
// batas jendela menurunkan ember yang salah dan meninggalkan
// hitungan hantu di ember kenaikan. Kini tanda menyimpan jangkar
// waktu kenaikan — dibaca dari jam limiter yang sama — dan refund
// memakainya.
//
// Test ini memakai API B-363 (PeriksaBatasModel mengembalikan
// context bertanda + Limiter.RefundScopesAt) — sejak B-363 tanda
// ditanam PeriksaBatasModel sendiri dengan bacaan jam yang sama
// dengan penilaiannya, jadi langkah TandaiKuotaModel terpisah di
// versi awal test ini tidak ada lagi. Ia memang tidak
// terkompilasi di kode lama — pola yang sama seperti pengunci
// B-210; bukti runtime dua arah untuk mekanisme jangkarnya ada di
// paket apikey (TestB353RefundBerjangkarKeJendelaKenaikan), yang
// hanya memakai API lama dan gagal di sana.
func TestB353RefundModelBerjangkarKeJendelaKenaikan(t *testing.T) {
	mr := miniredis.RunT(t)
	rdb, err := cache.Connect(context.Background(), &config.Config{
		AppEnv:   config.EnvDevelopment,
		RedisURL: security.Secret("redis://" + mr.Addr()),
	}, slog.New(slog.DiscardHandler))
	if err != nil {
		t.Fatalf("cache.Connect: %v", err)
	}
	t.Cleanup(func() { _ = rdb.Close() })

	jam := time.Date(2026, 9, 2, 10, 30, 30, 0, time.UTC)
	limiter := apikey.NewLimiter(rdb, observability.NewMetrics(), loggerSenyap(),
		apikey.WithClock(func() time.Time { return jam }))
	src := &sumberKebijakan{
		limits: []*policy.RateLimit{{
			Scope: policy.ScopeModel, ScopeID: "m-b353", RequestsPerMinute: pointerG(100), Enabled: true,
		}},
	}
	guard := NewGuard(GuardDeps{
		Rates:   ratelimit.NewEngine(src, loggerSenyap()),
		Limiter: limiter,
		Logger:  loggerSenyap(),
	})

	ctx := context.Background()
	ctxBertanda, rj := guard.PeriksaBatasModel(ctx, "m-b353")
	if rj != nil {
		t.Fatalf("PeriksaBatasModel menolak: %+v", rj)
	}
	ctx = ctxBertanda

	kunciModel := func() string {
		for _, k := range mr.Keys() {
			if strings.Contains(k, "m-b353") && strings.Contains(k, "rpm") {
				return k
			}
		}
		return ""
	}
	kunci := kunciModel()
	if kunci == "" {
		t.Fatal("kunci rpm cakupan model tidak ditemukan sesudah kenaikan")
	}
	if v, _ := mr.Get(kunci); v != "1" {
		t.Fatalf("counter %q = %q sesudah kenaikan, mau \"1\"", kunci, v)
	}

	// Seberangi batas jendela menit, lalu kembalikan kuota model:
	// yang turun harus ember kenaikan, bukan ember menit berikutnya.
	jam = jam.Add(61 * time.Second)
	guard.KembalikanKuotaModel(ctx, "m-b353")
	if v, _ := mr.Get(kunci); v != "0" {
		t.Errorf("counter ember kenaikan %q = %q sesudah refund menyeberang jendela, mau \"0\" — refund model menurunkan ember yang salah", kunci, v)
	}
}
