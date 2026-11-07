package admin

import (
	"context"
	"encoding/json"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/alicebob/miniredis/v2"
	"github.com/jackc/pgx/v5/pgxpool"

	"github.com/NexGen-X/Route-X/internal/cache"
	"github.com/NexGen-X/Route-X/internal/config"
	"github.com/NexGen-X/Route-X/internal/database/repo/upstream"
	"github.com/NexGen-X/Route-X/internal/responsecache"
	"github.com/NexGen-X/Route-X/internal/security"
)

// Test Fase 20 PR-A: siraman restore yang gagal harus BERNAMA
// (B-475) dan rename provider menyiram cache (B-476).
// Setiap test diskriminatif: gagal di kode basis, lulus sesudah fix.

// mesinCacheDenganRedis seperti mesinCacheUntukAdmin tetapi
// mengembalikan handle miniredis-nya juga, supaya test bisa
// menyuntikkan kegagalan Redis di tengah jalan.
func mesinCacheDenganRedis(t *testing.T) (*responsecache.Engine, *miniredis.Miniredis) {
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
	return responsecache.NewEngine(rdb, slog.New(slog.DiscardHandler)), mr
}

// metadataAuditRestoreTerbaru membaca metadata baris audit restore
// terakhir dari database hasil restore (pool baru: pool lama bisa
// memegang koneksi yang diputus penukaran database).
func metadataAuditRestoreTerbaru(t *testing.T, env *lingkunganRestore) string {
	t.Helper()
	ctx := context.Background()
	cfg := env.pool.Config().Copy()
	cfg.ConnConfig.Database = env.namaDB
	cfg.MinConns = 0
	pool, err := pgxpool.NewWithConfig(ctx, cfg)
	if err != nil {
		t.Fatalf("pool pembaca audit: %v", err)
	}
	defer pool.Close()
	var metadata string
	if err := pool.QueryRow(ctx, `SELECT metadata::text FROM audit_logs
		WHERE action = 'restore' AND resource_type = 'database_backup'
		ORDER BY id DESC LIMIT 1`).Scan(&metadata); err != nil {
		t.Fatalf("membaca audit restore: %v", err)
	}
	return metadata
}

// TestB475RestoreGagalSiramBernama adalah pengunci B-475: restore
// yang berhasil tetapi siraman cache-nya GAGAL dulu tetap menjawab
// sukses polos (best-effort kosongkanCacheRespons menelan errornya
// ke log) sementara entri pra-restore benar-benar masih hidup.
// Kini kegagalan siraman bernama di respons (peringatan) dan di
// metadata audit (cache_flush_failed), dan restore-nya sendiri
// tetap sukses — menggagalkannya jadi 500 juga bohong.
func TestB475RestoreGagalSiramBernama(t *testing.T) {
	env := siapkanLingkunganRestore(t)
	mesin, mr := mesinCacheDenganRedis(t)
	env.handler.responseCache = mesin

	env.tanamPenanda("AWAL")

	req := httptest.NewRequest(http.MethodPost, "/api/admin/system/backup/export?format=sql", nil)
	recExp := httptest.NewRecorder()
	env.handler.exportDatabaseSQL(recExp, req)
	if recExp.Code != http.StatusOK {
		t.Fatalf("ekspor status = %d. body: %s", recExp.Code, recExp.Body.String())
	}
	dump := recExp.Body.Bytes()

	tanamEntriLaluNonaktifkan(t, mesin, "kunci-b475")

	// Redis mati tepat sebelum restore: siraman wajib gagal.
	mr.SetError("ERR redis tidak tersedia (suntikan uji B-475)")

	rec := env.panggilRestore("routex_backup.sql", dump)
	if rec.Code != http.StatusOK {
		t.Fatalf("restore status = %d, mau 200 (restore-nya sendiri berhasil). body: %s",
			rec.Code, rec.Body.String())
	}
	body := rec.Body.String()
	if !strings.Contains(body, `"peringatan"`) ||
		!strings.Contains(body, "response cache gagal dikosongkan") {
		t.Errorf("respons restore tidak menamai kegagalan siraman cache. body: %s", body)
	}

	mr.SetError("")
	if metadata := metadataAuditRestoreTerbaru(t, env); !strings.Contains(metadata, `"cache_flush_failed": true`) {
		t.Errorf("metadata audit restore tidak mencatat cache_flush_failed. metadata: %s", metadata)
	}
	// Bukti dampaknya nyata: entri pra-restore memang masih hidup —
	// persis yang diperingatkan respons & audit di atas.
	if !entriMasihHidup(t, mesin, "kunci-b475") {
		t.Error("entri pra-restore ternyata sudah hilang — premis test (siraman gagal) tidak terbukti")
	}
}

// TestB475KontrolRestoreSehatTanpaPeringatan menjaga arah
// sebaliknya: restore dengan siraman sehat tidak membawa
// peringatan dan tidak mencatat cache_flush_failed.
func TestB475KontrolRestoreSehatTanpaPeringatan(t *testing.T) {
	env := siapkanLingkunganRestore(t)
	mesin := mesinCacheUntukAdmin(t)
	env.handler.responseCache = mesin

	env.tanamPenanda("AWAL")

	req := httptest.NewRequest(http.MethodPost, "/api/admin/system/backup/export?format=sql", nil)
	recExp := httptest.NewRecorder()
	env.handler.exportDatabaseSQL(recExp, req)
	if recExp.Code != http.StatusOK {
		t.Fatalf("ekspor status = %d. body: %s", recExp.Code, recExp.Body.String())
	}

	rec := env.panggilRestore("routex_backup.sql", recExp.Body.Bytes())
	if rec.Code != http.StatusOK {
		t.Fatalf("restore status = %d. body: %s", rec.Code, rec.Body.String())
	}
	if strings.Contains(rec.Body.String(), `"peringatan"`) {
		t.Errorf("restore sehat membawa peringatan. body: %s", rec.Body.String())
	}
	if metadata := metadataAuditRestoreTerbaru(t, env); strings.Contains(metadata, "cache_flush_failed") {
		t.Errorf("audit restore sehat mencatat cache_flush_failed. metadata: %s", metadata)
	}
}

// TestB476RenameProviderMenyiramCache adalah pengunci B-476: nama
// provider tidak masuk sidik rute, tetapi substring "antigravity"
// di dalamnya memilih jalur project Code Assist di factory —
// rename yang mengubah nama wajib menyiram; rename ke nama yang
// sama (sesudah normalisasi) dan update field lain tidak.
func TestB476RenameProviderMenyiramCache(t *testing.T) {
	env := setupTestEnv(t)
	ctx := context.Background()

	mesin := mesinCacheUntukAdmin(t)
	mesin.SetSettings(true, time.Minute)
	env.handlers.responseCache = mesin

	providerRepo := upstream.NewProviderRepo(env.pool)
	prov, err := providerRepo.Create(ctx, upstream.CreateProviderParams{
		Name: "prov-b476", DisplayName: "Provider B-476", Kind: "openai", BaseURL: "https://api.openai.com",
	})
	if err != nil {
		t.Fatalf("buat provider: %v", err)
	}

	tanam := func() {
		t.Helper()
		if err := mesin.Set(ctx, "kunci-b476", &responsecache.Entry{
			Model: "model-uji", Raw: json.RawMessage(`{"id":"chatcmpl-b476"}`),
		}); err != nil {
			t.Fatalf("tanam entri cache: %v", err)
		}
	}
	hit := func() bool {
		t.Helper()
		_, h, err := mesin.Get(ctx, "kunci-b476")
		if err != nil {
			t.Fatalf("baca cache: %v", err)
		}
		return h
	}

	c := env.newClient(t)
	c.login("superadmin@routex.internal", testPassword)
	url := "/api/admin/upstreams/providers/" + prov.ID

	// Kontrol 1: rename ke nama yang sama tidak menyiram.
	tanam()
	res, body := c.do(http.MethodPut, url, map[string]any{"name": "prov-b476"}, true)
	if res.StatusCode != http.StatusOK {
		t.Fatalf("PUT nama sama: status = %d, mau 200 (body: %s)", res.StatusCode, body)
	}
	if !hit() {
		t.Error("entri cache hilang sesudah rename ke nama yang sama — flush terlalu luas")
	}

	// Rename sungguhan: cache WAJIB kosong.
	tanam()
	res, body = c.do(http.MethodPut, url, map[string]any{"name": "prov-b476-lain"}, true)
	if res.StatusCode != http.StatusOK {
		t.Fatalf("PUT rename: status = %d, mau 200 (body: %s)", res.StatusCode, body)
	}
	if hit() {
		t.Error("entri cache masih tersaji sesudah rename provider — konteks nama berubah tanpa kunci baru (B-476)")
	}
}
