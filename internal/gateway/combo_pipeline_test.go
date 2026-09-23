package gateway

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strconv"
	"strings"
	"testing"
	"time"

	"github.com/NexGen-X/Route-X/internal/database/repo/upstream"
	"github.com/NexGen-X/Route-X/internal/providers"
	"github.com/NexGen-X/Route-X/internal/router"
)

// aturanPipeline membuat rule combo pipeline jsonb (jalur baru) untuk test.
func aturanPipeline(id string, strategi router.Strategy, attempts int, models []string, maks int) *upstream.RoutingRule {
	return &upstream.RoutingRule{
		ID:           id,
		Name:         "Combo Pipeline " + id,
		MatchModelID: ptrString("m-t1"),
		Strategy:     string(strategi),
		MaxAttempts:  maks,
		Enabled:      true,
		Pipeline: []byte(`{"strategy":"` + string(strategi) + `","attempts":` + strconv.Itoa(attempts) +
			`,"models":["` + strings.Join(models, `","`) + `"]}`),
	}
}

func siapCombo3Tier(t *testing.T, rule *upstream.RoutingRule,
	bangun func(nama string) *providerTiruan,
) (*Handlers, *pabrikTiruan) {
	t.Helper()
	m1 := &upstream.Model{ID: "m-t1", ModelID: "tier1-model", Enabled: true}
	m2 := &upstream.Model{ID: "m-t2", ModelID: "tier2-model", Enabled: true}
	m3 := &upstream.Model{ID: "m-t3", ModelID: "tier3-model", Enabled: true}

	cand1 := kandidatRute("prov-t1", "upstream-t1")
	cand2 := kandidatRute("prov-t2", "upstream-t2")
	cand3 := kandidatRute("prov-t3", "upstream-t3")

	models := &petaModel{models: map[string]*upstream.Model{
		"tier1-model": m1, "tier2-model": m2, "tier3-model": m3,
	}}
	cands := &petaKandidat{cands: map[string][]*upstream.RouteCandidate{
		"m-t1": {cand1}, "m-t2": {cand2}, "m-t3": {cand3},
	}}

	engine := router.NewEngine(&sumberAturanTiruan{rules: []*upstream.RoutingRule{rule}}, nil, loggerSenyap())

	perNama := map[string]providers.Provider{}
	for _, nama := range []string{"prov-t1", "prov-t2", "prov-t3"} {
		nama := nama
		perNama[nama] = bangun(nama)
	}
	factory := &pabrikTiruan{perNama: perNama}

	h, err := NewHandlers(HandlersDeps{
		Models:     models,
		Lister:     &pendaftarModel{},
		Candidates: cands,
		Factory:    factory,
		Engine:     engine,
		Executor:   NewExecutor(nil, loggerSenyap(), WithSleepFunc(func(context.Context, time.Duration) error { return nil })),
		Logger:     loggerSenyap(),
	})
	if err != nil {
		t.Fatalf("NewHandlers: %v", err)
	}
	return h, factory
}

// chatGagal membuat hook chat yang selalu gagal dengan jenis kesalahan tertentu.
func chatGagal(jenis providers.ErrorKind) func(context.Context, *providers.ChatRequest) (*providers.ChatResponse, error) {
	return func(_ context.Context, _ *providers.ChatRequest) (*providers.ChatResponse, error) {
		return nil, providers.Newf(jenis, "gagal", "gagal")
	}
}

// chatSukses membuat hook chat yang membalas teks tertentu.
func chatSukses(teks string) func(context.Context, *providers.ChatRequest) (*providers.ChatResponse, error) {
	return func(_ context.Context, _ *providers.ChatRequest) (*providers.ChatResponse, error) {
		return &providers.ChatResponse{
			ID:  "chatcmpl-combo",
			Raw: json.RawMessage(`{"id":"chatcmpl-combo","choices":[{"message":{"content":"` + teks + `"}}]}`),
		}, nil
	}
}

const bodyComboChat = `{"model":"tier1-model","messages":[{"role":"user","content":"halo"}]}`

// attempts resep adalah anggaran TOTAL lintas seluruh model: attempts:2 pada tiga model
// harus menghasilkan tepat 2 panggilan provider, bukan 3×Maks. Ini adalah inti PR: tanpa
// pembagian anggaran, satu permintaan klien menjadi perkalian biaya yang tidak pernah
// diminta resepnya.
func TestComboPipelineAnggaranTotalMembatasiPercobaan(t *testing.T) {
	h, factory := siapCombo3Tier(t,
		aturanPipeline("r-combo-budget", router.StrategyPriority, 2,
			[]string{"tier1-model", "tier2-model", "tier3-model"}, 1),
		func(nama string) *providerTiruan {
			return &providerTiruan{nama: nama, kind: providers.KindOpenAI, chat: chatGagal(providers.ErrKindOverloaded)}
		})

	w := httptest.NewRecorder()
	r := httptest.NewRequest(http.MethodPost, "/chat/completions", strings.NewReader(bodyComboChat))
	h.Routes().ServeHTTP(w, r)

	if w.Code == http.StatusOK {
		t.Fatalf("status = 200, mau kegagalan (semua provider gagal)")
	}
	urutan := factory.urutan()
	mau := []string{"prov-t1", "prov-t2"}
	if len(urutan) != len(mau) || urutan[0] != mau[0] || urutan[1] != mau[1] {
		t.Errorf("urutan panggilan = %v, mau %v — anggaran 2 harus menghentikan prov-t3", urutan, mau)
	}
}

// Anggaran dipakai bersama antar model: kegagalan pada model pertama masih menyisakan
// jatah untuk model kedua, dan model kedua itu boleh sukses menyelamatkan permintaan.
func TestComboPipelineFailoverLintasModelSampaiSukses(t *testing.T) {
	h, factory := siapCombo3Tier(t,
		aturanPipeline("r-combo-failover", router.StrategyPriority, 3,
			[]string{"tier1-model", "tier2-model", "tier3-model"}, 1),
		func(nama string) *providerTiruan {
			chat := chatGagal(providers.ErrKindOverloaded)
			if nama == "prov-t2" {
				chat = chatSukses("jawaban sukses dari tier 2")
			}
			return &providerTiruan{nama: nama, kind: providers.KindOpenAI, chat: chat}
		})

	w := httptest.NewRecorder()
	r := httptest.NewRequest(http.MethodPost, "/chat/completions", strings.NewReader(bodyComboChat))
	h.Routes().ServeHTTP(w, r)

	if w.Code != http.StatusOK {
		t.Fatalf("status = %d; ingin 200 (body: %s)", w.Code, w.Body.String())
	}
	if !strings.Contains(w.Body.String(), "jawaban sukses dari tier 2") {
		t.Errorf("respons tidak memuat jawaban tier 2: %s", w.Body.String())
	}
	urutan := factory.urutan()
	mau := []string{"prov-t1", "prov-t2"}
	if len(urutan) != len(mau) || urutan[0] != mau[0] || urutan[1] != mau[1] {
		t.Errorf("urutan panggilan = %v, mau %v — tier 3 tidak boleh disentuh", urutan, mau)
	}
}

// Jalur streaming juga memfailover antar model: kegagalan saat membuka aliran pada model
// pertama masih bisa diselamatkan oleh model kedua, dan SSE yang diterima klien utuh.
// Ini menguji penjaga Combo() pada cascade chatMengalir sekaligus anggaran di ExecuteStream.
func TestComboPipelineStreamingFailoverLintasModel(t *testing.T) {
	h, factory := siapCombo3Tier(t,
		aturanPipeline("r-combo-stream", router.StrategyPriority, 3,
			[]string{"tier1-model", "tier2-model", "tier3-model"}, 1),
		func(nama string) *providerTiruan {
			p := &providerTiruan{nama: nama, kind: providers.KindOpenAI}
			if nama == "prov-t1" {
				// Gagal saat MEMBUKA aliran: sebelum byte pertama, failover masih sah.
				p.alir = func(_ context.Context, _ *providers.ChatRequest) (providers.Stream, error) {
					return nil, providers.Newf(providers.ErrKindOverloaded, nama, "penuh")
				}
			} else {
				p.alir = func(_ context.Context, _ *providers.ChatRequest) (providers.Stream, error) {
					return &aliranTiruan{ev: []*providers.StreamEvent{
						peristiwa("chunk-dari-tier2"),
						peristiwa("[DONE]"),
					}}, nil
				}
			}
			return p
		})

	w := httptest.NewRecorder()
	r := httptest.NewRequest(http.MethodPost, "/chat/completions",
		strings.NewReader(`{"model":"tier1-model","stream":true,"messages":[{"role":"user","content":"halo"}]}`))
	h.Routes().ServeHTTP(w, r)

	if w.Code != http.StatusOK {
		t.Fatalf("status = %d; ingin 200 (body: %s)", w.Code, w.Body.String())
	}
	if !strings.Contains(w.Body.String(), "chunk-dari-tier2") {
		t.Errorf("respons SSE tidak memuat chunk tier 2: %s", w.Body.String())
	}
	if !strings.Contains(w.Body.String(), "data: ") {
		t.Errorf("respons bukan framing SSE: %s", w.Body.String()[:min(80, len(w.Body.String()))])
	}
	urutan := factory.urutan()
	mau := []string{"prov-t1", "prov-t2"}
	if len(urutan) != len(mau) || urutan[0] != mau[0] || urutan[1] != mau[1] {
		t.Errorf("urutan panggilan = %v, mau %v — tier 3 tidak boleh disentuh", urutan, mau)
	}
}

// Kompatibilitas: rule produksi lama memakai tag [combo:...] di description dengan pipeline
// NULL. Jalur itu harus tetap berfungsi tanpa perubahan — rule raute-x di produksi belum
// dimigrasikan, dan menghentikannya berarti memutus lalu lintas nyata.
func TestComboPipelineKompatibilitasTagLamaTetapBerfungsi(t *testing.T) {
	h, factory := siapCombo3Tier(t,
		&upstream.RoutingRule{
			ID:           "r-combo-tag-lama",
			Name:         "Combo Tag Lama",
			MatchModelID: ptrString("m-t1"),
			Strategy:     "priority",
			MaxAttempts:  1,
			Enabled:      true,
			// Pipeline NULL: jalur tag description, cascade per tier.
			Description: ptrString(`[combo:pipeline=[{"tier":2,"model":"tier2-model"},{"tier":3,"model":"tier3-model"}]]`),
		},
		func(nama string) *providerTiruan {
			chat := chatGagal(providers.ErrKindServer)
			if nama == "prov-t3" {
				chat = chatSukses("jawaban sukses dari tier 3")
			}
			return &providerTiruan{nama: nama, kind: providers.KindOpenAI, chat: chat}
		})

	w := httptest.NewRecorder()
	r := httptest.NewRequest(http.MethodPost, "/chat/completions", strings.NewReader(bodyComboChat))
	h.Routes().ServeHTTP(w, r)

	if w.Code != http.StatusOK {
		t.Fatalf("status = %d; ingin 200 (body: %s)", w.Code, w.Body.String())
	}
	if !strings.Contains(w.Body.String(), "jawaban sukses dari tier 3") {
		t.Errorf("respons tidak memuat jawaban tier 3: %s", w.Body.String())
	}
	urutan := factory.urutan()
	mau := []string{"prov-t1", "prov-t2", "prov-t3"}
	if len(urutan) != len(mau) {
		t.Errorf("urutan panggilan = %v, mau %v (cascade 3 tier)", urutan, mau)
	} else {
		for i := range mau {
			if urutan[i] != mau[i] {
				t.Errorf("urutan panggilan = %v, mau %v", urutan, mau)
				break
			}
		}
	}
}

// Regresi ganda: rule combo pipeline jsonb yang kebetulan juga membawa tag [combo:...] di
// description tidak boleh MENJALANKAN KEDUA jalur. Cascade tier lama harus dilewati,
// karena kandidat gabungan sudah mencakup seluruh model resep — mengeksekusinya lagi
// hanya mengulangi provider yang sudah habis anggarannya.
func TestComboPipelineJsonbTidakMenjalankanCascadeTagLama(t *testing.T) {
	h, factory := siapCombo3Tier(t,
		&upstream.RoutingRule{
			ID:           "r-combo-jsonb-tag",
			Name:         "Combo Jsonb Dengan Tag",
			MatchModelID: ptrString("m-t1"),
			Strategy:     "priority",
			MaxAttempts:  1,
			Enabled:      true,
			Pipeline:     []byte(`{"strategy":"priority","attempts":3,"models":["tier1-model","tier2-model","tier3-model"]}`),
			// Tag lama sengaja disertakan: tanpa penjaga Combo(), cascade ini akan
			// dipanggil lagi setelah loop gabungan gagal, dan tier 2 serta 3 akan
			// dipanggil dua kali.
			Description: ptrString(`[combo:pipeline=[{"tier":2,"model":"tier2-model"},{"tier":3,"model":"tier3-model"}]]`),
		},
		func(nama string) *providerTiruan {
			return &providerTiruan{nama: nama, kind: providers.KindOpenAI, chat: chatGagal(providers.ErrKindOverloaded)}
		})

	w := httptest.NewRecorder()
	r := httptest.NewRequest(http.MethodPost, "/chat/completions", strings.NewReader(bodyComboChat))
	h.Routes().ServeHTTP(w, r)

	if w.Code == http.StatusOK {
		t.Fatal("status = 200, mau kegagalan")
	}
	urutan := factory.urutan()
	// Anggaran 3 dipakai habis oleh loop gabungan. Cascade tag lama HARUS dilewati,
	// jadi tepat 3 panggilan — bukan 5 (3 dari loop + tier2 & tier3 lagi dari cascade).
	mau := []string{"prov-t1", "prov-t2", "prov-t3"}
	if len(urutan) != len(mau) {
		t.Errorf("urutan panggilan = %v, mau tepat %v — cascade tag lama ganda tidak boleh terjadi", urutan, mau)
	} else {
		for i := range mau {
			if urutan[i] != mau[i] {
				t.Errorf("urutan panggilan = %v, mau %v", urutan, mau)
				break
			}
		}
	}
}
