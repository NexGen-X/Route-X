package main

import (
	"context"
	"encoding/json"
	"log/slog"
	"testing"
	"time"

	"github.com/alicebob/miniredis/v2"

	"github.com/NexGen-X/Route-X/internal/cache"
	"github.com/NexGen-X/Route-X/internal/config"
	"github.com/NexGen-X/Route-X/internal/responsecache"
	"github.com/NexGen-X/Route-X/internal/security"
)

// TestB436CallbackWorkerMenyiramSaatCacheNonaktif adalah pengunci
// B-436 sisi worker: callback yang dipasang cmd untuk
// penonaktifan sesi OAuth dulu digerbang IsEnabled, jadi mutasi
// konteks jawaban yang terjadi selama response cache nonaktif
// tidak pernah menyiram dan entri basi hidup lagi saat
// re-aktivasi. Callback kini memanggil Flush tanpa syarat
// enabled (Engine.Flush sendiri memang tidak mensyaratkannya).
func TestB436CallbackWorkerMenyiramSaatCacheNonaktif(t *testing.T) {
	ctx := context.Background()

	mr := miniredis.RunT(t)
	rdb, err := cache.Connect(ctx, &config.Config{
		AppEnv:   config.EnvDevelopment,
		RedisURL: security.Secret("redis://" + mr.Addr()),
	}, slog.New(slog.DiscardHandler))
	if err != nil {
		t.Fatalf("cache.Connect: %v", err)
	}
	defer func() { _ = rdb.Close() }()
	mesin := responsecache.NewEngine(rdb, slog.New(slog.DiscardHandler))

	// Tanam entri saat aktif, lalu matikan engine: persis jendela
	// cache-mati pada mutasi worker.
	mesin.SetSettings(true, time.Minute)
	if err := mesin.Set(ctx, "kunci-b436-cmd", &responsecache.Entry{
		Model: "model-uji", Raw: json.RawMessage(`{"id":"chatcmpl-b436"}`),
	}); err != nil {
		t.Fatalf("tanam entri cache: %v", err)
	}
	mesin.SetSettings(false, time.Minute)

	callbackSiramCacheRespons(mesin, slog.New(slog.DiscardHandler))(ctx)

	mesin.SetSettings(true, time.Minute)
	if _, hit, err := mesin.Get(ctx, "kunci-b436-cmd"); err != nil || hit {
		t.Errorf("entri cache masih tersaji sesudah callback worker pada engine nonaktif (hit=%v, err=%v) — callback masih tergerbang enabled (B-436)", hit, err)
	}

	// Callback dengan engine nil harus aman (tidak panik).
	callbackSiramCacheRespons(nil, slog.New(slog.DiscardHandler))(ctx)
}
