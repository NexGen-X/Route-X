package gateway

import (
	"context"
	"log/slog"
	"net/http"
	"net/netip"
	"strings"
	"testing"

	"github.com/alicebob/miniredis/v2"

	"github.com/NexGen-X/Route-X/internal/apikey"
	"github.com/NexGen-X/Route-X/internal/cache"
	"github.com/NexGen-X/Route-X/internal/config"
	"github.com/NexGen-X/Route-X/internal/database/repo/keys"
	"github.com/NexGen-X/Route-X/internal/observability"
	"github.com/NexGen-X/Route-X/internal/security"
)

// TestResponsesLolosTidakMengembalikanKuotaB166 adalah pengunci B-166:
// permukaan /v1/responses sebelumnya tidak pernah memanggil jaga.lolos(),
// sehingga penjaga kuota mengembalikan (refund) penghitung Limit() untuk
// SETIAP permintaan — termasuk yang berhasil dieksekusi ke upstream —
// dan RPM/RPD per key di permukaan Codex ini tidak pernah terakumulasi.
// Permintaan yang berhasil wajib mempertahankan counternya; penolakan
// pra-upstream (body tidak sah) tetap wajib mengembalikannya (V-AB-17).
func TestResponsesLolosTidakMengembalikanKuotaB166(t *testing.T) {
	mr := miniredis.RunT(t)
	rdb, err := cache.Connect(context.Background(), &config.Config{
		AppEnv:   config.EnvDevelopment,
		RedisURL: security.Secret("redis://" + mr.Addr()),
	}, slog.New(slog.DiscardHandler))
	if err != nil {
		t.Fatalf("cache.Connect: %v", err)
	}
	t.Cleanup(func() { _ = rdb.Close() })

	limiter := apikey.NewLimiter(rdb, observability.NewMetrics(), slog.New(slog.DiscardHandler))
	guard := NewGuard(GuardDeps{Limiter: limiter, Logger: loggerSenyap()})

	rpm := 100
	key := &keys.Key{ID: "k-b166", OwnerUserID: "u-1", Prefix: "sk_live_", Last4: "9a21", RateLimitRPM: &rpm}
	ctxKey := apikey.WithPrincipal(context.Background(), &apikey.Principal{Key: key})

	bacaRPM := func() string {
		t.Helper()
		for _, k := range mr.Keys() {
			if strings.Contains(k, "k-b166") && strings.Contains(k, "rpm-") {
				v, _ := mr.Get(k)
				return v
			}
		}
		return ""
	}
	naikkan := func() {
		t.Helper()
		if d := limiter.Allow(ctxKey, "k-b166", apikey.LimitsFromKey(key), netip.Addr{}); !d.Allowed {
			t.Fatalf("Allow ditolak: %+v", d)
		}
	}

	s := susun(t, providerSukses(), func(_ *susunan, d *HandlersDeps) {
		d.Guard = guard
	})

	// Permintaan BERHASIL ke upstream: counter yang dinaikkan middleware
	// Limit() wajib bertahan.
	naikkan()
	if v := bacaRPM(); v != "1" {
		t.Fatalf("counter rpm = %q, mau \"1\" sesudah Allow", v)
	}
	w := s.panggil(t, http.MethodPost, "/responses", `{"model":"gpt-4o-mini","input":"halo"}`, ctxKey)
	if w.Code != http.StatusOK {
		t.Fatalf("status = %d, mau 200: %s", w.Code, w.Body.String())
	}
	if v := bacaRPM(); v != "1" {
		t.Errorf("counter rpm sesudah permintaan BERHASIL = %q, mau tetap \"1\" — "+
			"kuota dikembalikan untuk permintaan yang sah (B-166)", v)
	}

	// Penolakan pra-upstream (body bentuk chat ditolak pengurai Responses):
	// counter tetap wajib kembali.
	naikkan()
	if v := bacaRPM(); v != "2" {
		t.Fatalf("counter rpm = %q, mau \"2\" sesudah Allow kedua", v)
	}
	w = s.panggil(t, http.MethodPost, "/responses", bodyChatMinimal, ctxKey)
	if w.Code != http.StatusBadRequest {
		t.Fatalf("status = %d, mau 400: %s", w.Code, w.Body.String())
	}
	if v := bacaRPM(); v != "1" {
		t.Errorf("counter rpm sesudah penolakan pra-upstream = %q, mau \"1\" — "+
			"penolakan tetap harus mengembalikan kuota (V-AB-17)", v)
	}
}
