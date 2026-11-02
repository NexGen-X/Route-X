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
	"github.com/NexGen-X/Route-X/internal/providers"
	"github.com/NexGen-X/Route-X/internal/security"
)

// Test regresi audit3 B-012 (lantai pemakaian) dan B-013 (pengembalian kuota).

func TestTerapkanLantaiPemakaian(t *testing.T) {
	req := &providers.ChatRequest{
		Model:    "gpt-4o-mini",
		Messages: []providers.Message{{Role: providers.RoleUser, Content: "halo dunia"}},
	}
	est := EstimateTokens(req)
	if est.InputTokens <= 0 {
		t.Fatalf("estimasi input = %d, mau > 0", est.InputTokens)
	}

	// Tanpa usage sama sekali: input dari estimasi, output dari byte konten.
	got := terapkanLantaiPemakaian(req, providers.Usage{}, 400)
	if got.InputTokens != est.InputTokens {
		t.Errorf("input = %d, mau estimasi %d", got.InputTokens, est.InputTokens)
	}
	if got.OutputTokens != 100 {
		t.Errorf("output = %d, mau 100 (400 byte / 4)", got.OutputTokens)
	}
	if got.TotalTokens != est.InputTokens+100 {
		t.Errorf("total = %d, mau %d", got.TotalTokens, est.InputTokens+100)
	}

	// Usage parsial: angka yang dilaporkan upstream TIDAK ditimpa, hanya
	// bagian yang kosong yang diisi lantai.
	got = terapkanLantaiPemakaian(req, providers.Usage{InputTokens: 7, TotalTokens: 7}, 400)
	if got.InputTokens != 7 || got.OutputTokens != 100 || got.TotalTokens != 107 {
		t.Errorf("parsial = %+v, mau input 7 / output 100 / total 107", got)
	}

	// Usage lengkap: utuh, berapa pun byte kontennya.
	lengkap := providers.Usage{InputTokens: 7, OutputTokens: 9, TotalTokens: 16}
	if got = terapkanLantaiPemakaian(req, lengkap, 4000); got != lengkap {
		t.Errorf("lengkap = %+v, mau utuh %+v", got, lengkap)
	}

	// Tanpa byte konten, output tetap nol — lantai tidak mengarang token
	// untuk jawaban yang memang tidak berisi apa-apa.
	got = terapkanLantaiPemakaian(req, providers.Usage{}, 0)
	if got.OutputTokens != 0 {
		t.Errorf("output tanpa konten = %d, mau 0", got.OutputTokens)
	}
}

// TestChatNonStreamTanpaUsageTetapTercatat membuktikan V-GW-10: respons
// non-streaming dari upstream yang tidak melaporkan usage tidak lagi
// tercatat 0 token — lantai estimasi mengisinya.
func TestChatNonStreamTanpaUsageTetapTercatat(t *testing.T) {
	konten := strings.Repeat("kata ", 40) // 200 byte → lantai output 50
	prov := &providerTiruan{
		nama: "utama", kind: providers.KindOpenAI,
		chat: func(_ context.Context, _ *providers.ChatRequest) (*providers.ChatResponse, error) {
			return &providers.ChatResponse{
				ID:  "chatcmpl-tanpa-usage",
				Raw: []byte(`{"id":"chatcmpl-tanpa-usage","object":"chat.completion","choices":[{"index":0,"message":{"role":"assistant","content":"` + konten + `"},"finish_reason":"stop"}]}`),
				Choices: []providers.Choice{{
					Index:        0,
					Message:      providers.Message{Role: providers.RoleAssistant, Content: konten},
					FinishReason: providers.FinishStop,
				}},
			}, nil
		},
	}
	s, sink := susunDenganPencatat(t, prov, nil)

	w := s.panggil(t, http.MethodPost, "/chat/completions", bodyChatMinimal, ctxDenganKey("k-1", "u-1"))
	if w.Code != http.StatusOK {
		t.Fatalf("status = %d, mau 200: %s", w.Code, w.Body.String())
	}

	ev := sink.satu(t)
	if ev.InputTokens <= 0 {
		t.Errorf("input tercatat = %d, mau > 0 dari lantai estimasi", ev.InputTokens)
	}
	if ev.OutputTokens != 50 {
		t.Errorf("output tercatat = %d, mau 50 (200 byte konten / 4)", ev.OutputTokens)
	}
	if ev.TotalTokens != ev.InputTokens+ev.OutputTokens {
		t.Errorf("total = %d, mau input+output = %d", ev.TotalTokens, ev.InputTokens+ev.OutputTokens)
	}
}

// TestChatStreamTanpaUsageInputTidakHilang membuktikan V-AB-14: aliran yang
// berakhir tanpa chunk usage tidak lagi kehilangan seluruh token inputnya.
func TestChatStreamTanpaUsageInputTidakHilang(t *testing.T) {
	aliran := &aliranTiruan{ev: []*providers.StreamEvent{
		peristiwa("jawabannya "),
		peristiwa("panjang sekali"),
		{FinishReason: providers.FinishStop},
	}}
	prov := &providerTiruan{
		nama: "utama", kind: providers.KindOpenAI,
		alir: func(_ context.Context, _ *providers.ChatRequest) (providers.Stream, error) {
			return aliran, nil
		},
	}
	s, sink := susunDenganPencatat(t, prov, nil)

	w := s.panggil(t, http.MethodPost, "/chat/completions",
		`{"model":"gpt-4o-mini","stream":true,"messages":[{"role":"user","content":"hai"}]}`,
		ctxDenganKey("k-1", "u-1"))
	if w.Code != http.StatusOK {
		t.Fatalf("status = %d, mau 200: %s", w.Code, w.Body.String())
	}

	ev := sink.satu(t)
	if ev.InputTokens <= 0 {
		t.Errorf("input tercatat = %d, mau > 0 dari lantai estimasi", ev.InputTokens)
	}
	if ev.OutputTokens <= 0 {
		t.Errorf("output tercatat = %d, mau > 0 dari lantai byte", ev.OutputTokens)
	}
	if ev.TotalTokens != ev.InputTokens+ev.OutputTokens {
		t.Errorf("total = %d, mau input+output = %d", ev.TotalTokens, ev.InputTokens+ev.OutputTokens)
	}
}

// TestAnthropicStreamLantaiDariKontenBukanFraming membuktikan V-AB-15:
// lantai output jalur Anthropic dihitung dari byte KONTEN, bukan byte
// framing SSE. Sebelum perbaikan, konten ±4 token tercatat 137 token
// (±34× lipat) karena yang dihitung byte peristiwa terbungkus.
func TestAnthropicStreamLantaiDariKontenBukanFraming(t *testing.T) {
	aliran := &aliranTiruan{ev: []*providers.StreamEvent{
		peristiwa("ha"),
		peristiwa("lo"),
		{FinishReason: providers.FinishStop},
	}}
	prov := &providerTiruan{
		nama: "utama", kind: providers.KindAnthropic,
		alir: func(_ context.Context, _ *providers.ChatRequest) (providers.Stream, error) {
			return aliran, nil
		},
	}
	s, sink := susunDenganPencatat(t, prov, nil)

	w := s.panggil(t, http.MethodPost, "/v1/messages", bodyMessagesStreaming, nil)
	if w.Code != http.StatusOK {
		t.Fatalf("status = %d, mau 200: %s", w.Code, w.Body.String())
	}

	ev := sink.satu(t)
	// Konten "halo" = 4 byte → lantai output tepat 1. Menghitung framing akan
	// menghasilkan puluhan.
	if ev.OutputTokens != 1 {
		t.Errorf("output tercatat = %d, mau 1 (4 byte konten / 4), bukan hitungan framing", ev.OutputTokens)
	}
	if ev.InputTokens <= 0 {
		t.Errorf("input tercatat = %d, mau > 0 dari lantai estimasi", ev.InputTokens)
	}
}

// TestPenolakanPraUpstreamMengembalikanKuota membuktikan V-AB-17 di tingkat
// handler: Limit() menaikkan counter sebelum handler berjalan; penolakan
// kewenangan yang terjadi sebelum upstream dicoba mengembalikannya, sehingga
// permintaan yang ditolak tidak membakar kuota. Permintaan yang lolos ke
// upstream TIDAK dikembalikan.
func TestPenolakanPraUpstreamMengembalikanKuota(t *testing.T) {
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
	key := &keys.Key{ID: "k-refund", OwnerUserID: "u-1", Prefix: "sk_live_", Last4: "9a21", RateLimitRPM: &rpm}
	ctxKey := apikey.WithPrincipal(context.Background(), &apikey.Principal{Key: key})

	bacaRPM := func() string {
		t.Helper()
		for _, k := range mr.Keys() {
			if strings.Contains(k, "k-refund") && strings.Contains(k, "rpm-") {
				v, _ := mr.Get(k)
				return v
			}
		}
		return ""
	}
	// Tirukan middleware Limit(): tiga permintaan terlanjur menaikkan counter.
	for i := 0; i < 3; i++ {
		if d := limiter.Allow(ctxKey, "k-refund", apikey.LimitsFromKey(key), netip.Addr{}); !d.Allowed {
			t.Fatalf("Allow %d ditolak: %+v", i+1, d)
		}
	}
	if v := bacaRPM(); v != "3" {
		t.Fatalf("counter rpm = %q, mau \"3\"", v)
	}

	// Ditolak model_not_allowed SEBELUM upstream: kuota kembali 3 → 2.
	s := susun(t, providerSukses(), func(su *susunan, d *HandlersDeps) {
		d.Guard = guard
		su.batas = &pembatasKey{model: false, provider: true}
		d.Restrict = su.batas
	})
	w := s.panggil(t, http.MethodPost, "/chat/completions", bodyChatMinimal, ctxKey)
	if w.Code != http.StatusForbidden {
		t.Fatalf("status = %d, mau 403: %s", w.Code, w.Body.String())
	}
	if v := bacaRPM(); v != "2" {
		t.Errorf("counter rpm sesudah penolakan = %q, mau \"2\" — kuota tidak kembali", v)
	}

	// Lolos ke upstream: tidak ada pengembalian.
	s2 := susun(t, providerSukses(), func(_ *susunan, d *HandlersDeps) {
		d.Guard = guard
	})
	w = s2.panggil(t, http.MethodPost, "/chat/completions", bodyChatMinimal, ctxKey)
	if w.Code != http.StatusOK {
		t.Fatalf("status = %d, mau 200: %s", w.Code, w.Body.String())
	}
	if v := bacaRPM(); v != "2" {
		t.Errorf("counter rpm sesudah permintaan berhasil = %q, mau tetap \"2\"", v)
	}
}
