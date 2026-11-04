package gateway

import (
	"context"
	"log/slog"
	"net/http"
	"strings"
	"sync/atomic"
	"testing"
	"time"

	"github.com/alicebob/miniredis/v2"

	"github.com/NexGen-X/Route-X/internal/cache"
	"github.com/NexGen-X/Route-X/internal/config"
	"github.com/NexGen-X/Route-X/internal/database/repo/traffic"
	"github.com/NexGen-X/Route-X/internal/providers"
	"github.com/NexGen-X/Route-X/internal/responsecache"
	"github.com/NexGen-X/Route-X/internal/security"
)

// Test Batch C1: aktivasi response cache yang aman. Mesin cache di sini
// NYATA (di atas miniredis) — yang diuji adalah perilaku jalur handler
// terhadapnya: isolasi antar API key, sidik parameter, batas tahap awal
// (streaming/tool/error tidak di-cache), kill switch, dan permukaan
// Anthropic.

func mesinCacheAktif(t *testing.T) *responsecache.Engine {
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
	e := responsecache.NewEngine(rdb, slog.New(slog.DiscardHandler))
	e.SetSettings(true, time.Hour)
	return e
}

func jumlahPanggilanProvider(p *providerTiruan) int {
	p.mu.Lock()
	defer p.mu.Unlock()
	return len(p.diterima)
}

func susunDenganCache(t *testing.T, prov *providerTiruan) (*susunan, *responsecache.Engine) {
	t.Helper()
	mesin := mesinCacheAktif(t)
	s := susun(t, prov, func(_ *susunan, d *HandlersDeps) {
		d.ResponseCache = mesin
	})
	return s, mesin
}

// TestCacheIsolasiAntarKunci adalah test isolasi eksplisit syarat
// aktivasi: entri milik kunci A tidak pernah melayani kunci B, dan
// sebaliknya — kunci B membangun entrinya sendiri.
func TestCacheIsolasiAntarKunci(t *testing.T) {
	prov := providerSukses()
	s, _ := susunDenganCache(t, prov)
	ctxA := ctxDenganKey("k-a", "u-a")
	ctxB := ctxDenganKey("k-b", "u-b")

	w := s.panggil(t, http.MethodPost, "/chat/completions", bodyChatMinimal, ctxA)
	if w.Code != http.StatusOK || w.Header().Get("X-RouteX-Cache") != "MISS" {
		t.Fatalf("A pertama: status = %d, cache = %q, mau 200/MISS (%s)", w.Code, w.Header().Get("X-RouteX-Cache"), w.Body.String())
	}
	if n := jumlahPanggilanProvider(prov); n != 1 {
		t.Fatalf("sesudah A pertama provider dipanggil %d kali, mau 1", n)
	}

	w = s.panggil(t, http.MethodPost, "/chat/completions", bodyChatMinimal, ctxA)
	if w.Header().Get("X-RouteX-Cache") != "HIT" {
		t.Fatalf("A kedua: cache = %q, mau HIT", w.Header().Get("X-RouteX-Cache"))
	}
	if n := jumlahPanggilanProvider(prov); n != 1 {
		t.Fatalf("A kedua memanggil provider (%d), mau tetap 1 (HIT)", n)
	}

	// Kunci B: prompt identik, tetapi TIDAK boleh menerima entri A.
	w = s.panggil(t, http.MethodPost, "/chat/completions", bodyChatMinimal, ctxB)
	if w.Header().Get("X-RouteX-Cache") != "MISS" {
		t.Fatalf("B pertama: cache = %q, mau MISS — entri kunci A bocor ke kunci B", w.Header().Get("X-RouteX-Cache"))
	}
	if n := jumlahPanggilanProvider(prov); n != 2 {
		t.Fatalf("sesudah B pertama provider dipanggil %d kali, mau 2", n)
	}

	w = s.panggil(t, http.MethodPost, "/chat/completions", bodyChatMinimal, ctxB)
	if w.Header().Get("X-RouteX-Cache") != "HIT" {
		t.Fatalf("B kedua: cache = %q, mau HIT dari entri B sendiri", w.Header().Get("X-RouteX-Cache"))
	}
	if n := jumlahPanggilanProvider(prov); n != 2 {
		t.Fatalf("B kedua memanggil provider (%d), mau tetap 2", n)
	}
}

// TestCacheSidikParameterBerbeda: parameter yang memengaruhi keluaran
// (di sini temperature) mengubah kunci — respons untuk temperature satu
// tidak disajikan untuk temperature lain.
func TestCacheSidikParameterBerbeda(t *testing.T) {
	prov := providerSukses()
	s, _ := susunDenganCache(t, prov)
	ctx := ctxDenganKey("k-1", "u-1")
	panas := `{"model":"gpt-4o-mini","temperature":0.9,"messages":[{"role":"user","content":"halo"}]}`

	if w := s.panggil(t, http.MethodPost, "/chat/completions", bodyChatMinimal, ctx); w.Header().Get("X-RouteX-Cache") != "MISS" {
		t.Fatalf("pertama: cache = %q, mau MISS", w.Header().Get("X-RouteX-Cache"))
	}
	w := s.panggil(t, http.MethodPost, "/chat/completions", panas, ctx)
	if w.Header().Get("X-RouteX-Cache") != "MISS" {
		t.Fatalf("temperature berbeda: cache = %q, mau MISS", w.Header().Get("X-RouteX-Cache"))
	}
	if n := jumlahPanggilanProvider(prov); n != 2 {
		t.Fatalf("provider dipanggil %d kali, mau 2", n)
	}
	if w := s.panggil(t, http.MethodPost, "/chat/completions", panas, ctx); w.Header().Get("X-RouteX-Cache") != "HIT" {
		t.Fatalf("temperature sama diulang: cache = %q, mau HIT", w.Header().Get("X-RouteX-Cache"))
	}
}

// TestCacheStreamingTidakDiCache: tahap awal tidak melayani maupun
// menyimpan streaming — dua permintaan streaming identik selalu sampai
// ke provider (jawaban V-GW-01/05 laten).
func TestCacheStreamingTidakDiCache(t *testing.T) {
	prov := providerSukses()
	s, _ := susunDenganCache(t, prov)
	ctx := ctxDenganKey("k-1", "u-1")
	body := `{"model":"gpt-4o-mini","stream":true,"messages":[{"role":"user","content":"halo"}]}`

	for i := 1; i <= 2; i++ {
		w := s.panggil(t, http.MethodPost, "/chat/completions", body, ctx)
		if w.Code != http.StatusOK {
			t.Fatalf("stream %d: status = %d, mau 200: %s", i, w.Code, w.Body.String())
		}
	}
	if n := jumlahPanggilanProvider(prov); n != 2 {
		t.Fatalf("provider dipanggil %d kali untuk 2 stream, mau 2 (streaming tidak di-cache)", n)
	}
}

// TestCacheErrorTidakDiCache: respons gagal tidak disimpan — permintaan
// berikutnya mencoba upstream lagi, dan keberhasilan sesudahnya baru
// membentuk entri.
func TestCacheErrorTidakDiCache(t *testing.T) {
	var panggilan atomic.Int64
	prov := &providerTiruan{
		nama: "utama", kind: providers.KindOpenAI,
		chat: func(_ context.Context, _ *providers.ChatRequest) (*providers.ChatResponse, error) {
			if panggilan.Add(1) == 1 {
				return nil, &providers.Error{Kind: providers.ErrKindInvalidRequest, Provider: "utama", Message: "permintaan ditolak upstream"}
			}
			return providerSukses().chat(context.Background(), nil)
		},
	}
	s, _ := susunDenganCache(t, prov)
	ctx := ctxDenganKey("k-1", "u-1")

	w := s.panggil(t, http.MethodPost, "/chat/completions", bodyChatMinimal, ctx)
	if w.Code == http.StatusOK {
		t.Fatalf("pertama: status = %d, mau bukan 200 (provider gagal)", w.Code)
	}
	// Bila error ter-cache, permintaan ini akan menjawab dari cache dan
	// tidak pernah sampai provider — statusnya akan tetap gagal.
	w = s.panggil(t, http.MethodPost, "/chat/completions", bodyChatMinimal, ctx)
	if w.Code != http.StatusOK || w.Header().Get("X-RouteX-Cache") != "MISS" {
		t.Fatalf("kedua: status = %d, cache = %q, mau 200/MISS", w.Code, w.Header().Get("X-RouteX-Cache"))
	}
	w = s.panggil(t, http.MethodPost, "/chat/completions", bodyChatMinimal, ctx)
	if w.Header().Get("X-RouteX-Cache") != "HIT" {
		t.Fatalf("ketiga: cache = %q, mau HIT", w.Header().Get("X-RouteX-Cache"))
	}
}

// TestCacheRequestBertoolTidakDiCache: request yang mendeklarasikan tool
// tidak dibaca/disimpan pada tahap awal — semantik tool tidak bisa
// diverifikasi gateway, jadi bila ragu jangan cache.
func TestCacheRequestBertoolTidakDiCache(t *testing.T) {
	prov := providerSukses()
	s, _ := susunDenganCache(t, prov)
	ctx := ctxDenganKey("k-1", "u-1")
	body := `{"model":"gpt-4o-mini","messages":[{"role":"user","content":"halo"}],` +
		`"tools":[{"type":"function","function":{"name":"cek_cuaca","description":"cek cuaca","parameters":{"type":"object","properties":{}}}}]}`

	for i := 1; i <= 2; i++ {
		w := s.panggil(t, http.MethodPost, "/chat/completions", body, ctx)
		if w.Code != http.StatusOK {
			t.Fatalf("tool %d: status = %d, mau 200: %s", i, w.Code, w.Body.String())
		}
		if h := w.Header().Get("X-RouteX-Cache"); h == "HIT" {
			t.Fatalf("tool %d: cache = HIT, mau tidak pernah HIT untuk request bertool", i)
		}
	}
	if n := jumlahPanggilanProvider(prov); n != 2 {
		t.Fatalf("provider dipanggil %d kali, mau 2 (request bertool tidak di-cache)", n)
	}
}

// TestCacheKillSwitch: menonaktifkan engine menghentikan penyajian HIT
// pada request berikutnya tanpa restart; mengaktifkan lagi memulihkan.
func TestCacheKillSwitch(t *testing.T) {
	prov := providerSukses()
	s, mesin := susunDenganCache(t, prov)
	ctx := ctxDenganKey("k-1", "u-1")

	_ = s.panggil(t, http.MethodPost, "/chat/completions", bodyChatMinimal, ctx)
	if w := s.panggil(t, http.MethodPost, "/chat/completions", bodyChatMinimal, ctx); w.Header().Get("X-RouteX-Cache") != "HIT" {
		t.Fatalf("sebelum kill switch: cache = %q, mau HIT", w.Header().Get("X-RouteX-Cache"))
	}

	mesin.SetSettings(false, time.Hour)
	w := s.panggil(t, http.MethodPost, "/chat/completions", bodyChatMinimal, ctx)
	if w.Header().Get("X-RouteX-Cache") == "HIT" {
		t.Fatal("sesudah dinonaktifkan masih HIT — kill switch tidak bekerja")
	}
	if n := jumlahPanggilanProvider(prov); n != 2 {
		t.Fatalf("provider dipanggil %d kali, mau 2 (satu MISS awal + satu sesudah mati)", n)
	}

	mesin.SetSettings(true, time.Hour)
	if w := s.panggil(t, http.MethodPost, "/chat/completions", bodyChatMinimal, ctx); w.Header().Get("X-RouteX-Cache") != "HIT" {
		t.Fatalf("sesudah diaktifkan lagi: cache = %q, mau HIT", w.Header().Get("X-RouteX-Cache"))
	}
}

// TestCacheHITTercatatDanTertagih: HIT menghasilkan baris catatan
// pemakaian dengan token > 0 dan provider asal jawaban — bukan request
// gratis tanpa jejak (perilaku grup 14 yang kini hidup lewat aktivasi).
func TestCacheHITTercatatDanTertagih(t *testing.T) {
	prov := providerSukses()
	mesin := mesinCacheAktif(t)
	s, sink := susunDenganPencatat(t, prov, func(_ *susunan, d *HandlersDeps) {
		d.ResponseCache = mesin
	})
	ctx := ctxDenganKey("k-1", "u-1")

	_ = s.panggil(t, http.MethodPost, "/chat/completions", bodyChatMinimal, ctx)
	w := s.panggil(t, http.MethodPost, "/chat/completions", bodyChatMinimal, ctx)
	if w.Header().Get("X-RouteX-Cache") != "HIT" {
		t.Fatalf("kedua: cache = %q, mau HIT", w.Header().Get("X-RouteX-Cache"))
	}

	sink.mu.Lock()
	jumlah := len(sink.ev)
	var totalTerakhir int
	var statusTerakhir int
	if jumlah > 0 {
		totalTerakhir = sink.ev[jumlah-1].TotalTokens
		statusTerakhir = sink.ev[jumlah-1].StatusCode
	}
	sink.mu.Unlock()

	if jumlah != 2 {
		t.Fatalf("catatan pemakaian = %d, mau 2 (MISS + HIT sama-sama tercatat)", jumlah)
	}
	if statusTerakhir != http.StatusOK || totalTerakhir <= 0 {
		t.Errorf("catatan HIT: status = %d, total token = %d, mau 200 dan > 0 (tertagih)", statusTerakhir, totalTerakhir)
	}
}

// TestCacheSatuMissMenghitungSatuB116 adalah regresi B-116 butir 6:
// kampanye live Langkah 7 mengamati satu MISS menaikkan misses/entries
// sebesar 2. Penyelidikan di main ini membuktikan setiap permukaan hanya
// memanggil Get/Set tepat satu kali dan mesin menghitung tepat satu —
// test ini mengunci semantik itu: satu MISS = misses 1 + entries 1, dan
// satu HIT = hits 1 tanpa mengubah misses/entries.
func TestCacheSatuMissMenghitungSatuB116(t *testing.T) {
	prov := providerSukses()
	s, mesin := susunDenganCache(t, prov)
	ctx := ctxDenganKey("k-b116", "u-b116")

	w := s.panggil(t, http.MethodPost, "/chat/completions", bodyChatMinimal, ctx)
	if w.Header().Get("X-RouteX-Cache") != "MISS" {
		t.Fatalf("pertama: cache = %q, mau MISS (%s)", w.Header().Get("X-RouteX-Cache"), w.Body.String())
	}
	st, err := mesin.Stats(context.Background())
	if err != nil {
		t.Fatalf("Stats: %v", err)
	}
	if st.Misses != 1 || st.TotalEntries != 1 || st.Hits != 0 {
		t.Fatalf("sesudah 1 MISS: hits=%d misses=%d entries=%d, mau 0/1/1", st.Hits, st.Misses, st.TotalEntries)
	}

	w = s.panggil(t, http.MethodPost, "/chat/completions", bodyChatMinimal, ctx)
	if w.Header().Get("X-RouteX-Cache") != "HIT" {
		t.Fatalf("kedua: cache = %q, mau HIT", w.Header().Get("X-RouteX-Cache"))
	}
	st, err = mesin.Stats(context.Background())
	if err != nil {
		t.Fatalf("Stats: %v", err)
	}
	if st.Hits != 1 || st.Misses != 1 || st.TotalEntries != 1 {
		t.Fatalf("sesudah 1 HIT: hits=%d misses=%d entries=%d, mau 1/1/1", st.Hits, st.Misses, st.TotalEntries)
	}
}

// TestCacheHitMenghasilkanEventCacheHitB116 membuktikan B-116 butir 7:
// catatan pemakaian untuk HIT membawa peristiwa timeline cache_hit,
// sedangkan MISS tidak — supaya tab Events inspektor menjelaskan keadaan
// HIT, bukan tampil kosong tanpa sebab.
func TestCacheHitMenghasilkanEventCacheHitB116(t *testing.T) {
	prov := providerSukses()
	mesin := mesinCacheAktif(t)
	s, sink := susunDenganPencatat(t, prov, func(_ *susunan, d *HandlersDeps) {
		d.ResponseCache = mesin
	})
	ctx := ctxDenganKey("k-b116", "u-b116")

	_ = s.panggil(t, http.MethodPost, "/chat/completions", bodyChatMinimal, ctx)
	w := s.panggil(t, http.MethodPost, "/chat/completions", bodyChatMinimal, ctx)
	if w.Header().Get("X-RouteX-Cache") != "HIT" {
		t.Fatalf("kedua: cache = %q, mau HIT", w.Header().Get("X-RouteX-Cache"))
	}

	sink.mu.Lock()
	defer sink.mu.Unlock()
	if len(sink.ev) != 2 {
		t.Fatalf("catatan pemakaian = %d, mau 2", len(sink.ev))
	}
	punyaCacheHit := func(idx int) bool {
		for _, n := range sink.ev[idx].Notes {
			if n.Kind == traffic.EventCacheHit {
				return true
			}
		}
		return false
	}
	if punyaCacheHit(0) {
		t.Errorf("catatan MISS ikut membawa event cache_hit")
	}
	if !punyaCacheHit(1) {
		t.Errorf("catatan HIT tidak membawa event cache_hit; notes = %+v", sink.ev[1].Notes)
	}
}

// TestCacheAnthropicHIT: permukaan Anthropic non-streaming ikut
// dilayani cache dengan tubuh dialek Anthropic, dan entrinya tidak
// tercampur dengan permukaan OpenAI untuk prompt yang sama.
func TestCacheAnthropicHIT(t *testing.T) {
	prov := providerSukses()
	s, _ := susunDenganCache(t, prov)
	ctx := ctxDenganKey("k-1", "u-1")
	body := `{"model":"claude-sonnet-4-5","max_tokens":1024,"messages":[{"role":"user","content":"halo"}]}`

	w := s.panggil(t, http.MethodPost, "/v1/messages", body, ctx)
	if w.Code != http.StatusOK || w.Header().Get("X-RouteX-Cache") != "MISS" {
		t.Fatalf("anthropic pertama: status = %d, cache = %q, mau 200/MISS: %s", w.Code, w.Header().Get("X-RouteX-Cache"), w.Body.String())
	}
	if n := jumlahPanggilanProvider(prov); n != 1 {
		t.Fatalf("provider dipanggil %d kali, mau 1", n)
	}

	w = s.panggil(t, http.MethodPost, "/v1/messages", body, ctx)
	if w.Header().Get("X-RouteX-Cache") != "HIT" {
		t.Fatalf("anthropic kedua: cache = %q, mau HIT: %s", w.Header().Get("X-RouteX-Cache"), w.Body.String())
	}
	if n := jumlahPanggilanProvider(prov); n != 1 {
		t.Fatalf("anthropic kedua memanggil provider (%d), mau tetap 1", n)
	}
	// Tubuh HIT adalah dialek Anthropic (type message), bukan Raw OpenAI.
	if !strings.Contains(w.Body.String(), `"type":"message"`) {
		t.Errorf("tubuh HIT bukan dialek Anthropic: %s", w.Body.String())
	}
}
