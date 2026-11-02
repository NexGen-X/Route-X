package admin

import (
	"context"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/alicebob/miniredis/v2"

	"github.com/NexGen-X/Route-X/internal/cache"
	"github.com/NexGen-X/Route-X/internal/config"
	"github.com/NexGen-X/Route-X/internal/responsecache"
	"github.com/NexGen-X/Route-X/internal/security"
)

// Test Batch C1: jalur admin kini MENERIMA aktivasi response cache —
// penolakan cache_policy_unsafe historis gugur karena kebijakan sidik
// kunci yang dituntutnya sudah diterapkan di responsecache. Kill switch
// (enabled=false) tetap berlaku seketika di engine yang sama.

func mesinCacheUntukAdmin(t *testing.T) *responsecache.Engine {
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
	return responsecache.NewEngine(rdb, slog.New(slog.DiscardHandler))
}

func panggilUpdateCache(t *testing.T, h *Handlers, body string) *httptest.ResponseRecorder {
	t.Helper()
	r := httptest.NewRequest(http.MethodPost, "/api/admin/system/cache/settings", strings.NewReader(body))
	w := httptest.NewRecorder()
	h.updateCacheSettings(w, r)
	return w
}

func TestUpdateCacheSettingsMenerimaAktivasi(t *testing.T) {
	mesin := mesinCacheUntukAdmin(t)
	h := NewHandlers(Config{ResponseCache: mesin})

	w := panggilUpdateCache(t, h, `{"enabled":true,"ttl_seconds":1800}`)
	if w.Code != http.StatusOK {
		t.Fatalf("aktivasi: status = %d, mau 200: %s", w.Code, w.Body.String())
	}
	if strings.Contains(w.Body.String(), "cache_policy_unsafe") {
		t.Fatalf("aktivasi masih ditolak cache_policy_unsafe: %s", w.Body.String())
	}
	if !mesin.IsEnabled() {
		t.Fatal("engine tidak aktif sesudah aktivasi diterima")
	}
	if mesin.TTL() != 30*time.Minute {
		t.Errorf("TTL engine = %v, mau 30 menit", mesin.TTL())
	}

	// Kill switch: menonaktifkan diterima dan berlaku seketika.
	w = panggilUpdateCache(t, h, `{"enabled":false,"ttl_seconds":1800}`)
	if w.Code != http.StatusOK {
		t.Fatalf("nonaktifkan: status = %d, mau 200: %s", w.Code, w.Body.String())
	}
	if mesin.IsEnabled() {
		t.Fatal("engine masih aktif sesudah dinonaktifkan — kill switch gagal")
	}
}

func TestUpdateCacheSettingsMenolakTTLTidakSah(t *testing.T) {
	mesin := mesinCacheUntukAdmin(t)
	h := NewHandlers(Config{ResponseCache: mesin})

	w := panggilUpdateCache(t, h, `{"enabled":true,"ttl_seconds":999999}`)
	if w.Code != http.StatusBadRequest {
		t.Fatalf("TTL raksasa: status = %d, mau 400: %s", w.Code, w.Body.String())
	}
	if mesin.IsEnabled() {
		t.Fatal("engine aktif padahal TTL ditolak")
	}
}
