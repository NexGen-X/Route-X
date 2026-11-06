package gateway

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/NexGen-X/Route-X/internal/database/repo/policy"
	"github.com/NexGen-X/Route-X/internal/database/repo/upstream"
	"github.com/NexGen-X/Route-X/internal/providers"
	"github.com/NexGen-X/Route-X/internal/router"
)

// --- B-251: cascade combo fallback jalur lama di permukaan embeddings -------
//
// Tiga permukaan lain memberlakukan cascade ini sejak lama; embeddings
// adalah satu-satunya yang langsung menyerah. Test ini mengunci
// keselarasannya: kegagalan model utama pada aturan combo warisan harus
// mencoba model tier berikutnya — dengan syarat model tier benar-benar
// melayani embeddings.

func susunEmbeddingsCombo(t *testing.T, deskripsi string, modelTier2 *upstream.Model, gagal1, hasil2 func(context.Context, *providers.EmbeddingsRequest) (*providers.EmbeddingsResponse, error)) (*Handlers, *pabrikTiruan) {
	t.Helper()
	m1 := &upstream.Model{ID: "m-e1", ModelID: "emb-utama", Enabled: true, Capabilities: []string{upstream.CapEmbeddings}}

	models := map[string]*upstream.Model{"emb-utama": m1}
	cands := map[string][]*upstream.RouteCandidate{"m-e1": {kandidatRute("prov-e1", "emb-up-1")}}
	if modelTier2 != nil {
		models[modelTier2.ModelID] = modelTier2
		cands[modelTier2.ID] = []*upstream.RouteCandidate{kandidatRute("prov-e2", "emb-up-2")}
	}
	rule := &upstream.RoutingRule{
		ID: "r-emb-combo", Name: "Combo embeddings", MatchModelID: ptrString("m-e1"),
		Strategy: "priority", MaxAttempts: 1, Enabled: true,
	}
	if deskripsi != "" {
		rule.Description = ptrString(deskripsi)
	}
	engine := router.NewEngine(&sumberAturanTiruan{rules: []*upstream.RoutingRule{rule}}, nil, loggerSenyap())
	factory := &pabrikTiruan{perNama: map[string]providers.Provider{
		"prov-e1": &providerTiruan{nama: "prov-e1", kind: providers.KindOpenAI, embed: gagal1},
		"prov-e2": &providerTiruan{nama: "prov-e2", kind: providers.KindOpenAI, embed: hasil2},
	}}
	h, err := NewHandlers(HandlersDeps{
		Models:     &petaModel{models: models},
		Lister:     &pendaftarModel{},
		Candidates: &petaKandidat{cands: cands},
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

func panggilEmbeddings(t *testing.T, h *Handlers, body string) *httptest.ResponseRecorder {
	t.Helper()
	w := httptest.NewRecorder()
	r := httptest.NewRequest(http.MethodPost, "/embeddings", strings.NewReader(body))
	h.Routes().ServeHTTP(w, r)
	return w
}

func embedGagal(_ context.Context, _ *providers.EmbeddingsRequest) (*providers.EmbeddingsResponse, error) {
	return nil, providers.Newf(providers.ErrKindServer, "prov-e1", "error 500 dari provider utama")
}

func embedSuksesTier2(_ context.Context, _ *providers.EmbeddingsRequest) (*providers.EmbeddingsResponse, error) {
	return &providers.EmbeddingsResponse{
		Model: "emb-up-2",
		Data:  []providers.Embedding{{Index: 0, Vector: []float32{0.5, 0.25}}},
		Raw:   json.RawMessage(`{"object":"list","data":[{"index":0,"embedding":[0.5,0.25],"object":"embedding"}],"penanda":"tier2"}`),
	}, nil
}

func TestB251EmbeddingsCascadeComboWarisan(t *testing.T) {
	m2 := &upstream.Model{ID: "m-e2", ModelID: "emb-cadangan", Enabled: true, Capabilities: []string{upstream.CapEmbeddings}}
	h, factory := susunEmbeddingsCombo(t, "[combo:tier2=emb-cadangan]", m2, embedGagal, embedSuksesTier2)

	w := panggilEmbeddings(t, h, `{"model":"emb-utama","input":"halo"}`)
	if w.Code != http.StatusOK {
		t.Fatalf("status = %d, mau 200 dari tier 2 (body: %s)", w.Code, w.Body.String())
	}
	if !strings.Contains(w.Body.String(), `"penanda":"tier2"`) {
		t.Errorf("respons bukan dari tier 2: %s", w.Body.String())
	}
	urutan := factory.urutan()
	if len(urutan) != 2 || urutan[0] != "prov-e1" || urutan[1] != "prov-e2" {
		t.Errorf("urutan eksekusi = %v, mau [prov-e1 prov-e2]", urutan)
	}
}

func TestB251EmbeddingsTanpaComboTidakCascade(t *testing.T) {
	m2 := &upstream.Model{ID: "m-e2", ModelID: "emb-cadangan", Enabled: true, Capabilities: []string{upstream.CapEmbeddings}}
	h, factory := susunEmbeddingsCombo(t, "", m2, embedGagal, embedSuksesTier2)

	w := panggilEmbeddings(t, h, `{"model":"emb-utama","input":"halo"}`)
	if w.Code == http.StatusOK {
		t.Fatalf("tanpa tag combo respons sukses — cascade menyala tanpa dasar: %s", w.Body.String())
	}
	if urutan := factory.urutan(); len(urutan) != 1 {
		t.Errorf("urutan eksekusi = %v, mau hanya [prov-e1]", urutan)
	}
}

func TestB251EmbeddingsTierBukanEmbeddingsDilewati(t *testing.T) {
	// Model tier yang hanya melayani chat tidak boleh menggantikan model
	// embeddings: tier dilewati dan kegagalan utama yang dilaporkan.
	m2 := &upstream.Model{ID: "m-e2", ModelID: "emb-cadangan", Enabled: true, Capabilities: []string{upstream.CapText}}
	h, factory := susunEmbeddingsCombo(t, "[combo:tier2=emb-cadangan]", m2, embedGagal, embedSuksesTier2)

	w := panggilEmbeddings(t, h, `{"model":"emb-utama","input":"halo"}`)
	if w.Code == http.StatusOK {
		t.Fatalf("tier non-embeddings melayani embeddings: %s", w.Body.String())
	}
	if urutan := factory.urutan(); len(urutan) != 1 {
		t.Errorf("urutan eksekusi = %v, mau hanya [prov-e1] (tier dilewati)", urutan)
	}
}

// --- B-252: masukan embeddings berbentuk token tidak lolos penyaring --------

func TestB252EmbeddingsTokenDitolakPenyaring(t *testing.T) {
	src := &sumberKebijakan{filters: []*policy.ContentFilter{
		filterBaris("f-blok", "tanpa kata terlarang", policy.FilterBlockedPattern, func(f *policy.ContentFilter) {
			f.Pattern = "terlarang"
			f.PatternType = policy.PatternSubstring
		}),
	}}
	prov := &providerTiruan{
		nama: "utama", kind: providers.KindOpenAI,
		embed: func(context.Context, *providers.EmbeddingsRequest) (*providers.EmbeddingsResponse, error) {
			return &providers.EmbeddingsResponse{Raw: json.RawMessage(`{"object":"list","data":[]}`)}, nil
		},
	}
	s := susun(t, prov, func(su *susunan, d *HandlersDeps) {
		d.Guard = guardUji(t, src)
		su.cands.cands = []*upstream.RouteCandidate{kandidatRute("utama", "embed-1")}
		su.models.model.Capabilities = []string{upstream.CapEmbeddings}
	})

	// Masukan token: tidak ada teks untuk diperiksa, tetapi aturan pola
	// blokir aktif — permintaan harus ditolak fail-closed.
	w := s.panggil(t, http.MethodPost, "/embeddings", `{"model":"gpt-4o-mini","input":[123,456]}`, nil)
	if w.Code != http.StatusBadRequest || kodeGalat(galat(t, w)) != CodeContentFilter {
		t.Fatalf("masukan token: status/kode = %d/%s, mau 400/%s (body: %s)",
			w.Code, kodeGalat(galat(t, w)), CodeContentFilter, w.Body.String())
	}
	if !strings.Contains(galat(t, w).Message, "tidak dapat diperiksa") {
		t.Errorf("pesan penolakan = %q, mau menyebut tidak dapat diperiksa", galat(t, w).Message)
	}

	// Kontrol: masukan teks bersih melewati penyaring dan dilayani.
	w = s.panggil(t, http.MethodPost, "/embeddings", `{"model":"gpt-4o-mini","input":"halo bersih"}`, nil)
	if w.Code != http.StatusOK {
		t.Fatalf("masukan teks bersih: status = %d, mau 200 (body: %s)", w.Code, w.Body.String())
	}
	// Kontrol sebaliknya: teks yang memuat pola tetap tertangkap.
	w = s.panggil(t, http.MethodPost, "/embeddings", `{"model":"gpt-4o-mini","input":"ini terlarang"}`, nil)
	if w.Code != http.StatusBadRequest || kodeGalat(galat(t, w)) != CodeContentFilter {
		t.Fatalf("masukan teks berpola: status/kode = %d/%s, mau 400/%s",
			w.Code, kodeGalat(galat(t, w)), CodeContentFilter)
	}
}

func TestB252EmbeddingsTokenTanpaAturanPolaBerjalan(t *testing.T) {
	// Tanpa aturan pola aktif, tak terperiksa bukan pelanggaran: masukan
	// token berjalan persis seperti sebelum B-252.
	prov := &providerTiruan{
		nama: "utama", kind: providers.KindOpenAI,
		embed: func(context.Context, *providers.EmbeddingsRequest) (*providers.EmbeddingsResponse, error) {
			return &providers.EmbeddingsResponse{Raw: json.RawMessage(`{"object":"list","data":[]}`)}, nil
		},
	}
	s := susun(t, prov, func(su *susunan, d *HandlersDeps) {
		d.Guard = guardUji(t, &sumberKebijakan{})
		su.cands.cands = []*upstream.RouteCandidate{kandidatRute("utama", "embed-1")}
		su.models.model.Capabilities = []string{upstream.CapEmbeddings}
	})
	w := s.panggil(t, http.MethodPost, "/embeddings", `{"model":"gpt-4o-mini","input":[123,456]}`, nil)
	if w.Code != http.StatusOK {
		t.Fatalf("masukan token tanpa aturan pola: status = %d, mau 200 (body: %s)", w.Code, w.Body.String())
	}
}
