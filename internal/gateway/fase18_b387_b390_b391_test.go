package gateway

import (
	"context"
	"errors"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/NexGen-X/Route-X/internal/database/repo/policy"
	"github.com/NexGen-X/Route-X/internal/database/repo/upstream"
	"github.com/NexGen-X/Route-X/internal/providers"
	"github.com/NexGen-X/Route-X/internal/router"
)

// aliranNilNil adalah providers.Stream yang melanggar kontrak
// Recv: ia mengembalikan (nil, nil).
type aliranNilNil struct{ ditutup bool }

func (s *aliranNilNil) Recv() (*providers.StreamEvent, error) { return nil, nil }
func (s *aliranNilNil) Close() error                          { s.ditutup = true; return nil }

// TestB387PeristiwaNilNilTerkendali adalah pengunci B-387:
// aliranPenjagaDiam.Recv sebelumnya menghitung len(ev.Raw) tanpa
// memeriksa ev — adapter yang mengembalikan peristiwa (nil, nil)
// meledak sebagai panic di goroutine handler (dan lolos dari
// gerbang typed-nil B-358 yang memeriksa aliran, bukan peristiwa).
// Kini pelanggaran itu menjadi error berkind server.
func TestB387PeristiwaNilNilTerkendali(t *testing.T) {
	t.Run("unit penjaga", func(t *testing.T) {
		a := &aliranPenjagaDiam{Stream: &aliranNilNil{}}
		ev, err := a.Recv()
		if ev != nil {
			t.Errorf("ev = %+v, mau nil", ev)
		}
		var pe *providers.Error
		if !errors.As(err, &pe) || pe.Kind != providers.ErrKindServer {
			t.Fatalf("err = %v, mau *providers.Error berkind server", err)
		}
	})

	t.Run("permukaan chat streaming", func(t *testing.T) {
		prov := &providerTiruan{
			nama: "utama", kind: providers.KindOpenAI,
			alir: func(_ context.Context, _ *providers.ChatRequest) (providers.Stream, error) {
				return &aliranNilNil{}, nil
			},
		}
		s := susun(t, prov, nil)
		w := panggilTanpaPanic(t, s, "/chat/completions",
			`{"model":"gpt-4o-mini","stream":true,"messages":[{"role":"user","content":"hai"}]}`)
		if w.Code < 500 {
			t.Errorf("status = %d, mau 5xx terkendali (body: %s)", w.Code, w.Body.String())
		}
		if !strings.Contains(w.Body.String(), `"error"`) {
			t.Errorf("body = %q, mau envelope error", w.Body.String())
		}
	})
}

// TestB390CodecMeneruskanThoughtSignaturePesan adalah pengunci
// B-390: codec Chat sebelumnya tidak punya field thought_signature
// di tingkat pesan, jadi tanda tangan block thinking Anthropic
// yang diterbitkan adapter di tingkat pesan (encodeDelta) hilang
// saat klien mengulang riwayat — adapter yang sama lalu menolak
// mengirim balik block thinking-nya (ReasoningSignature kosong),
// persis kegagalan yang ditutup B-162 untuk per-tool-call.
func TestB390CodecMeneruskanThoughtSignaturePesan(t *testing.T) {
	req, err := DecodeChatRequest([]byte(`{
		"model": "claude-sonnet-4-5",
		"messages": [
			{"role": "user", "content": "hai"},
			{"role": "assistant", "content": "jawaban", "thought_signature": "sig-pesan-390"},
			{"role": "user", "content": "lanjutkan"}
		]
	}`))
	if err != nil {
		t.Fatalf("DecodeChatRequest: %v", err)
	}
	if got := req.Messages[1].ReasoningSignature; got != "sig-pesan-390" {
		t.Errorf("ReasoningSignature = %q, mau sig-pesan-390 — tanda tangan tingkat pesan dibuang codec", got)
	}
	// Kontrol: pesan tanpa field itu tetap kosong, bukan terisi karangan.
	if got := req.Messages[0].ReasoningSignature; got != "" {
		t.Errorf("ReasoningSignature pesan user = %q, mau kosong", got)
	}
}

// TestB391RefundTierMemakaiContextBertanda adalah pengunci B-391:
// jalur pintas Opsi C di siapkanChatParsed menyiapkan tier combo
// (PeriksaBatasModel menaikkan penghitung model tier pada pemegang
// tanda BARU di dalam siapkanTierFallback), membuangnya karena
// kandidatnya kekecilan, lalu merefund dengan context LUAR —
// pemegang tandanya berbeda, jadi refund no-op dan slot model
// tier terbakar sampai jendela berganti, walau tier tak pernah
// dieksekusi. Kini siapkanTierFallback mengembalikan context
// bertandanya dan refund di situs buang itu memakainya.
//
// Skenario: kandidat tier-1 dan tier-2 sama-sama kekecilan untuk
// prompt ini; tier-2 ber-RPM 1. Permintaan ditolak gerbang jendela
// konteks (400), tetapi sesudahnya penghitung tier-2 harus kembali
// nol — dibuktikan dengan PeriksaBatasModel baru yang DITERIMA.
func TestB391RefundTierMemakaiContextBertanda(t *testing.T) {
	m1 := &upstream.Model{ID: "m-t1", ModelID: "tier1-model", Enabled: true}
	m2 := &upstream.Model{ID: "m-t2", ModelID: "tier2-model", Enabled: true}
	models := &petaModel{models: map[string]*upstream.Model{
		"tier1-model": m1, "tier2-model": m2,
	}}
	jendelaKecil := 10
	kandKecil := func(nama, upstreamModel string) *upstream.RouteCandidate {
		c := kandidatRute(nama, upstreamModel)
		c.MaxContextWindow = &jendelaKecil
		return c
	}
	cands := &petaKandidat{cands: map[string][]*upstream.RouteCandidate{
		"m-t1": {kandKecil("utama", "upstream-t1")},
		"m-t2": {kandKecil("utama", "upstream-t2")},
	}}
	rule := &upstream.RoutingRule{
		ID:           "r-fase18-b391",
		Name:         "Aturan Tier B-391",
		MatchModelID: ptrString("m-t1"),
		Strategy:     "priority",
		MaxAttempts:  1,
		Enabled:      true,
		Description:  ptrString(`[combo:pipeline=[{"tier":2,"model":"tier2-model"}]]`),
	}
	engine := router.NewEngine(&sumberAturanTiruan{rules: []*upstream.RoutingRule{rule}}, nil, loggerSenyap())

	tiruan := &providerTiruan{
		nama: "utama", kind: providers.KindOpenAI,
		chat: chatSukses("jawaban tier 1"),
	}
	factory := &pabrikTiruan{perNama: map[string]providers.Provider{"utama": tiruan}}

	src := &sumberKebijakan{
		limits: []*policy.RateLimit{{
			Scope: policy.ScopeModel, ScopeID: "m-t2", RequestsPerMinute: pointerG(1), Enabled: true,
		}},
	}
	guard := guardUji(t, src)

	h, err := NewHandlers(HandlersDeps{
		Models:     models,
		Lister:     &pendaftarModel{},
		Candidates: cands,
		Factory:    factory,
		Engine:     engine,
		Guard:      guard,
		Executor:   NewExecutor(nil, loggerSenyap()),
		Logger:     loggerSenyap(),
	})
	if err != nil {
		t.Fatalf("NewHandlers: %v", err)
	}

	// Prompt panjang: taksiran tokennya jauh melewati jendela 10
	// milik kedua tier, jadi tier-2 disiapkan lalu dibuang, dan
	// permintaan akhirnya ditolak gerbang jendela konteks (400) —
	// hasil permukaannya sama sebelum dan sesudah fix; yang
	// membedakan adalah keadaan penghitung tier-2 sesudahnya.
	body := `{"model":"tier1-model","messages":[{"role":"user","content":"` +
		strings.Repeat("kata pembuka yang cukup panjang ", 30) + `"}]}`
	r := httptest.NewRequest(http.MethodPost, "/chat/completions", strings.NewReader(body))
	r = r.WithContext(ctxDenganKey("k-b391", "u-b391"))
	w := httptest.NewRecorder()
	h.Routes().ServeHTTP(w, r)
	if w.Code != http.StatusBadRequest || !strings.Contains(w.Body.String(), "context_length_exceeded") {
		t.Fatalf("status = %d, mau 400 context_length_exceeded (body: %s)", w.Code, w.Body.String())
	}

	// Penghitung model tier-2 (RPM 1) harus sudah kembali nol:
	// pemeriksaan baru diterima. Pra-fix ia tertolak karena slot
	// satu-satunya terbakar oleh persiapan yang dibuang.
	if _, rj := guard.PeriksaBatasModel(context.Background(), "m-t2"); rj != nil {
		t.Fatalf("PeriksaBatasModel tier-2 tertolak (%s) — slot tier yang dibuang tidak dikembalikan", rj.Message)
	}
}
