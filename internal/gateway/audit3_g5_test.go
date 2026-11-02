package gateway

// Test regresi audit3 grup 5 — permukaan /v1 & kontrak API (B-019, B-020):
// daftar model yang jujur (V-GW-34/35/37), endpoint detail model (V-GW-36),
// UUID bukan nama model (V-GW-39), kontrak header/body (V-GW-41/44/45),
// pembatas laju hanya untuk inferensi (V-GW-38), 405 ber-envelope (V-EM-06),
// dan masukan token embeddings (V-GW-40).

import (
	"bytes"
	"compress/gzip"
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/NexGen-X/Route-X/internal/database/repo"
	"github.com/NexGen-X/Route-X/internal/database/repo/upstream"
	"github.com/NexGen-X/Route-X/internal/providers"
)

// --- Tiruan khusus grup 5 ----------------------------------------------------

// kandidatPerModel mengembalikan kandidat menurut model, supaya satu test bisa
// membuat model yatim (tanpa kandidat) di samping model yang terlayani.
type kandidatPerModel struct {
	per map[string][]*upstream.RouteCandidate
}

func (k *kandidatPerModel) RouteCandidates(_ context.Context, q upstream.RouteQuery) ([]*upstream.RouteCandidate, error) {
	return k.per[q.ModelID], nil
}

// listerDenganAlias adalah pendaftarModel yang juga memenuhi daftarAlias.
type listerDenganAlias struct {
	pendaftarModel
	alias map[string][]*upstream.Alias
}

func (l *listerDenganAlias) ListAliases(_ context.Context, modelID string) ([]*upstream.Alias, error) {
	return l.alias[modelID], nil
}

// resolusiPeta menyelesaikan nama dari peta; nama lain ErrNotFound.
type resolusiPeta struct {
	per map[string]*upstream.Model
}

func (r *resolusiPeta) Resolve(_ context.Context, nama string) (*upstream.Model, error) {
	if m, ok := r.per[nama]; ok {
		return m, nil
	}
	return nil, repo.ErrNotFound
}

// pembatasPerProvider menolak provider tertentu saja.
type pembatasPerProvider struct {
	ditolak map[string]bool
}

func (p *pembatasPerProvider) AllowsModel(_ context.Context, _, _ string) (bool, error) {
	return true, nil
}

func (p *pembatasPerProvider) AllowsProvider(_ context.Context, _, providerID string) (bool, error) {
	return !p.ditolak[providerID], nil
}

func modelKatalog(id, nama string) *upstream.Model {
	return &upstream.Model{
		ID: id, ModelID: nama, Enabled: true, CreatedAt: time.Unix(1700000000, 0),
	}
}

func kandidatUntuk(providerID string) *upstream.RouteCandidate {
	c := kandidatRute(providerID, "upstream-x")
	c.ProviderID = providerID
	return c
}

// susunKatalog merakit Handlers dengan lister & kandidat per model. Model
// bernama "model-a" (bila ada) otomatis memiliki alias "alias-a", meniru
// Resolve repo sungguhan yang menyelesaikan alias lewat tabel aliases.
func susunKatalog(t *testing.T, models []*upstream.Model, per map[string][]*upstream.RouteCandidate, ubah func(*HandlersDeps)) *Handlers {
	t.Helper()
	lister := &listerDenganAlias{
		pendaftarModel: pendaftarModel{models: models},
		alias:          map[string][]*upstream.Alias{},
	}
	resolver := &resolusiPeta{per: map[string]*upstream.Model{}}
	for _, m := range models {
		resolver.per[m.ModelID] = m
		if m.ModelID == "model-a" {
			lister.alias[m.ID] = []*upstream.Alias{{Alias: "alias-a", ModelID: m.ID}}
			resolver.per["alias-a"] = m
		}
	}
	deps := HandlersDeps{
		Models:     resolver,
		Lister:     lister,
		Candidates: &kandidatPerModel{per: per},
		Factory:    &pabrikTiruan{bawaan: providerSukses()},
		Executor:   NewExecutor(nil, loggerSenyap(), WithSleepFunc(func(context.Context, time.Duration) error { return nil })),
		Logger:     loggerSenyap(),
	}
	if ubah != nil {
		ubah(&deps)
	}
	h, err := NewHandlers(deps)
	if err != nil {
		t.Fatalf("NewHandlers: %v", err)
	}
	return h
}

func panggilRute(t *testing.T, h *Handlers, metode, jalur string, ctx context.Context) *httptest.ResponseRecorder {
	t.Helper()
	r := httptest.NewRequest(metode, jalur, nil)
	if ctx != nil {
		r = r.WithContext(ctx)
	}
	w := httptest.NewRecorder()
	h.Routes().ServeHTTP(w, r)
	return w
}

type entriDaftar struct {
	ID string `json:"id"`
}

type daftarModel struct {
	Object  string        `json:"object"`
	Data    []entriDaftar `json:"data"`
	FirstID *string       `json:"first_id"`
	LastID  *string       `json:"last_id"`
	HasMore bool          `json:"has_more"`
}

func uraiDaftar(t *testing.T, w *httptest.ResponseRecorder) daftarModel {
	t.Helper()
	var d daftarModel
	if err := json.Unmarshal(w.Body.Bytes(), &d); err != nil {
		t.Fatalf("body bukan daftar model: %v (body: %s)", err, w.Body.String())
	}
	return d
}

func idEntri(d daftarModel) []string {
	out := make([]string, 0, len(d.Data))
	for _, e := range d.Data {
		out = append(out, e.ID)
	}
	return out
}

// --- V-GW-34: daftar hanya memuat model yang bisa dilayani -------------------

func TestModelsMenyisihkanModelYatim(t *testing.T) {
	a := modelKatalog("id-a", "model-a")
	b := modelKatalog("id-b", "model-b") // yatim: tanpa kandidat
	c := modelKatalog("id-c", "model-c")
	h := susunKatalog(t, []*upstream.Model{a, b, c}, map[string][]*upstream.RouteCandidate{
		"id-a": {kandidatUntuk("prov-1")},
		"id-c": {kandidatUntuk("prov-2")},
	}, nil)

	w := panggilRute(t, h, http.MethodGet, "/models", nil)
	if w.Code != http.StatusOK {
		t.Fatalf("status = %d (body: %s)", w.Code, w.Body.String())
	}
	got := idEntri(uraiDaftar(t, w))
	// alias-a ikut tampil (lihat test alias); yang tidak boleh ada adalah model-b.
	for _, id := range got {
		if id == "model-b" {
			t.Errorf("model yatim tanpa pemetaan ikut terdaftar: %v", got)
		}
	}
	if len(got) != 3 { // model-a, alias-a, model-c
		t.Errorf("daftar = %v, mau [alias-a model-a model-c]", got)
	}
}

func TestModelsMenyisihkanModelTerlarangProviderKey(t *testing.T) {
	a := modelKatalog("id-a", "model-a")
	c := modelKatalog("id-c", "model-c")
	h := susunKatalog(t, []*upstream.Model{a, c}, map[string][]*upstream.RouteCandidate{
		"id-a": {kandidatUntuk("prov-1")},
		"id-c": {kandidatUntuk("prov-2")},
	}, func(d *HandlersDeps) {
		d.Restrict = &pembatasPerProvider{ditolak: map[string]bool{"prov-2": true}}
	})

	w := panggilRute(t, h, http.MethodGet, "/models", ctxDenganKey("k-1", "u-1"))
	got := idEntri(uraiDaftar(t, w))
	for _, id := range got {
		if id == "model-c" {
			t.Errorf("model yang satu-satunya provider terlarang bagi key ikut terdaftar: %v", got)
		}
	}
	if len(got) == 0 {
		t.Errorf("daftar kosong, model-a seharusnya tampil: %v", got)
	}
}

// --- V-GW-35: alias tampil di daftar -----------------------------------------

func TestModelsAliasTampilDalamDaftar(t *testing.T) {
	a := modelKatalog("id-a", "model-a")
	h := susunKatalog(t, []*upstream.Model{a}, map[string][]*upstream.RouteCandidate{
		"id-a": {kandidatUntuk("prov-1")},
	}, nil)

	got := idEntri(uraiDaftar(t, panggilRute(t, h, http.MethodGet, "/models", nil)))
	if len(got) != 2 || got[0] != "alias-a" || got[1] != "model-a" {
		t.Errorf("daftar = %v, mau [alias-a model-a]", got)
	}
}

// --- V-GW-37: paginasi bekerja ------------------------------------------------

func TestModelsPaginasiLimitAfterID(t *testing.T) {
	a := modelKatalog("id-a", "model-a")
	b := modelKatalog("id-b", "model-b")
	c := modelKatalog("id-c", "model-c")
	h := susunKatalog(t, []*upstream.Model{a, b, c}, map[string][]*upstream.RouteCandidate{
		"id-a": {kandidatUntuk("prov-1")},
		"id-b": {kandidatUntuk("prov-1")},
		"id-c": {kandidatUntuk("prov-1")},
	}, nil)
	// alias-a milik model-a ikut dalam urutan gabungan:
	// [alias-a, model-a, model-b, model-c]

	semua := uraiDaftar(t, panggilRute(t, h, http.MethodGet, "/models", nil))
	if semua.HasMore || semua.FirstID == nil || *semua.FirstID != "alias-a" ||
		semua.LastID == nil || *semua.LastID != "model-c" || len(semua.Data) != 4 {
		t.Errorf("daftar penuh = %+v", semua)
	}

	hal1 := uraiDaftar(t, panggilRute(t, h, http.MethodGet, "/models?limit=2", nil))
	if len(hal1.Data) != 2 || !hal1.HasMore || hal1.LastID == nil || *hal1.LastID != "model-a" {
		t.Fatalf("halaman 1 = %+v", hal1)
	}

	hal2 := uraiDaftar(t, panggilRute(t, h, http.MethodGet, "/models?limit=2&after_id=model-a", nil))
	if got := idEntri(hal2); len(got) != 2 || got[0] != "model-b" || got[1] != "model-c" || hal2.HasMore {
		t.Errorf("halaman 2 = %+v", hal2)
	}

	ekor := uraiDaftar(t, panggilRute(t, h, http.MethodGet, "/models?after_id=model-c", nil))
	if len(ekor.Data) != 0 || ekor.FirstID != nil || ekor.LastID != nil || ekor.HasMore {
		t.Errorf("sesudah entri terakhir = %+v, mau halaman kosong", ekor)
	}

	for _, jalur := range []string{"/models?limit=abc", "/models?limit=0", "/models?limit=-1", "/models?after_id=hantu"} {
		w := panggilRute(t, h, http.MethodGet, jalur, nil)
		if w.Code != http.StatusBadRequest {
			t.Errorf("%s: status = %d, mau 400 (body: %s)", jalur, w.Code, w.Body.String())
		}
	}
}

// --- V-GW-36: detail model -----------------------------------------------------

func TestModelDetailBerbagaiKeadaan(t *testing.T) {
	a := modelKatalog("id-a", "model-a")
	yatim := modelKatalog("id-y", "model-yatim")
	mati := modelKatalog("id-m", "model-mati")
	mati.Enabled = false
	h := susunKatalog(t, []*upstream.Model{a, yatim, mati}, map[string][]*upstream.RouteCandidate{
		"id-a": {kandidatUntuk("prov-1")},
		"id-m": {kandidatUntuk("prov-1")},
	}, nil)

	// Kanonik yang terlayani → 200 berisi entri model.
	w := panggilRute(t, h, http.MethodGet, "/models/model-a", nil)
	if w.Code != http.StatusOK || !strings.Contains(w.Body.String(), `"id":"model-a"`) {
		t.Errorf("detail kanonik: status = %d, body = %s", w.Code, w.Body.String())
	}

	// Alias menyelesaikan ke model yang sama, entri memakai nama alias.
	w = panggilRute(t, h, http.MethodGet, "/models/alias-a", nil)
	if w.Code != http.StatusOK || !strings.Contains(w.Body.String(), `"id":"alias-a"`) {
		t.Errorf("detail alias: status = %d, body = %s", w.Code, w.Body.String())
	}

	// Yatim, mati, tak dikenal, dan UUID internal → 404 model_not_found seragam.
	for _, nama := range []string{"model-yatim", "model-mati", "tidak-ada",
		"075812e2-2369-4f27-81c8-2bf58985653b"} {
		w = panggilRute(t, h, http.MethodGet, "/models/"+nama, nil)
		if w.Code != http.StatusNotFound {
			t.Errorf("detail %s: status = %d, mau 404 (body: %s)", nama, w.Code, w.Body.String())
			continue
		}
		if got := kodeGalat(galat(t, w)); got != "model_not_found" {
			t.Errorf("detail %s: code = %q, mau model_not_found", nama, got)
		}
	}
}

// --- V-GW-39: UUID bukan nama model di jalur chat ------------------------------

func TestChatUUIDSebagaiNamaModelDitolak404(t *testing.T) {
	s := susun(t, providerSukses(), nil)
	body := `{"model":"075812e2-2369-4f27-81c8-2bf58985653b","messages":[{"role":"user","content":"halo"}]}`
	w := s.panggil(t, http.MethodPost, "/chat/completions", body, nil)
	if w.Code != http.StatusNotFound {
		t.Fatalf("status = %d, mau 404 (body: %s)", w.Code, w.Body.String())
	}
	if got := kodeGalat(galat(t, w)); got != "model_not_found" {
		t.Errorf("code = %q, mau model_not_found", got)
	}
}

// --- V-GW-41/44/45: kontrak header & body --------------------------------------

func panggilDenganHeader(t *testing.T, s *susunan, jalur, body string, header map[string]string, bungkus func(*http.Request)) *httptest.ResponseRecorder {
	t.Helper()
	r := httptest.NewRequest(http.MethodPost, jalur, strings.NewReader(body))
	for k, v := range header {
		r.Header.Set(k, v)
	}
	if bungkus != nil {
		bungkus(r)
	}
	w := httptest.NewRecorder()
	s.h.Routes().ServeHTTP(w, r)
	return w
}

func TestBacaBodyContentTypeSalahDitolak415(t *testing.T) {
	s := susun(t, providerSukses(), nil)
	w := panggilDenganHeader(t, s, "/chat/completions", bodyChatMinimal,
		map[string]string{"Content-Type": "text/plain"}, nil)
	if w.Code != http.StatusUnsupportedMediaType {
		t.Fatalf("status = %d, mau 415 (body: %s)", w.Code, w.Body.String())
	}
	if got := kodeGalat(galat(t, w)); got != "unsupported_media_type" {
		t.Errorf("code = %q, mau unsupported_media_type", got)
	}
}

func TestBacaBodyGzipDibukaDanPengkodeanLainDitolak(t *testing.T) {
	s := susun(t, providerSukses(), nil)

	var buf bytes.Buffer
	gz := gzip.NewWriter(&buf)
	if _, err := gz.Write([]byte(bodyChatMinimal)); err != nil {
		t.Fatalf("gzip write: %v", err)
	}
	if err := gz.Close(); err != nil {
		t.Fatalf("gzip close: %v", err)
	}
	r := httptest.NewRequest(http.MethodPost, "/chat/completions", &buf)
	r.Header.Set("Content-Type", "application/json")
	r.Header.Set("Content-Encoding", "gzip")
	w := httptest.NewRecorder()
	s.h.Routes().ServeHTTP(w, r)
	if w.Code != http.StatusOK {
		t.Errorf("body tergzip sah: status = %d, mau 200 (body: %s)", w.Code, w.Body.String())
	}

	w = panggilDenganHeader(t, s, "/chat/completions", bodyChatMinimal,
		map[string]string{"Content-Type": "application/json", "Content-Encoding": "br"}, nil)
	if w.Code != http.StatusUnsupportedMediaType {
		t.Errorf("Content-Encoding br: status = %d, mau 415 (body: %s)", w.Code, w.Body.String())
	}
}

// TestBacaBodyChunkedMelewatiBatasJadi413 mensimulasikan jalur chunked:
// middleware MaxBytes membungkus body dengan MaxBytesReader, dan kesalahan
// bacanya tidak boleh lagi tersapu menjadi 400 (V-GW-41).
func TestBacaBodyChunkedMelewatiBatasJadi413(t *testing.T) {
	s := susun(t, providerSukses(), nil)
	besar := `{"model":"gpt-4o-mini","messages":[{"role":"user","content":"` +
		strings.Repeat("a", 4096) + `"}]}`
	w := panggilDenganHeader(t, s, "/chat/completions", besar,
		map[string]string{"Content-Type": "application/json"},
		func(r *http.Request) {
			r.Body = http.MaxBytesReader(httptest.NewRecorder(), r.Body, 1024)
		})
	if w.Code != http.StatusRequestEntityTooLarge {
		t.Fatalf("status = %d, mau 413 (body: %s)", w.Code, w.Body.String())
	}
	if got := kodeGalat(galat(t, w)); got != "payload_too_large" {
		t.Errorf("code = %q, mau payload_too_large", got)
	}
}

// --- V-EM-06: 405 ber-envelope ---------------------------------------------------

func TestMetodeSalahBerEnvelope405(t *testing.T) {
	s := susun(t, providerSukses(), nil)
	for _, jalur := range []string{"/models", "/chat/completions", "/embeddings"} {
		w := s.panggil(t, http.MethodDelete, jalur, "", nil)
		if w.Code != http.StatusMethodNotAllowed {
			t.Errorf("DELETE %s: status = %d, mau 405", jalur, w.Code)
			continue
		}
		if w.Body.Len() == 0 {
			t.Errorf("DELETE %s: body kosong, mau envelope error", jalur)
			continue
		}
		if got := kodeGalat(galat(t, w)); got != "method_not_allowed" {
			t.Errorf("DELETE %s: code = %q, mau method_not_allowed", jalur, got)
		}
	}
}

// --- V-GW-38: pembatas laju hanya membungkus inferensi ---------------------------

func TestPembatasLajuHanyaRuteInferensi(t *testing.T) {
	terpanggil := 0
	pembatas := func(_ http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
			terpanggil++
			w.Header().Set("Content-Type", "application/json")
			w.WriteHeader(http.StatusTooManyRequests)
		})
	}
	s := susun(t, providerSukses(), func(_ *susunan, d *HandlersDeps) {
		d.Lister = &pendaftarModel{models: []*upstream.Model{modelUji()}}
		d.PembatasLaju = pembatas
	})

	// Katalog tidak menyentuh pembatas sama sekali.
	if w := s.panggil(t, http.MethodGet, "/models", "", nil); w.Code != http.StatusOK {
		t.Errorf("GET /models status = %d, mau 200", w.Code)
	}
	if w := s.panggil(t, http.MethodGet, "/models/gpt-4o-mini", "", nil); w.Code != http.StatusOK {
		t.Errorf("GET /models/{model} status = %d, mau 200 (body: %s)", w.Code, w.Body.String())
	}
	if terpanggil != 0 {
		t.Fatalf("pembatas terpanggil %d kali untuk katalog, mau 0", terpanggil)
	}

	// Inferensi melewati pembatas dan ditolak di sana.
	if w := s.panggil(t, http.MethodPost, "/chat/completions", bodyChatMinimal, nil); w.Code != http.StatusTooManyRequests {
		t.Errorf("POST /chat/completions status = %d, mau 429 dari pembatas", w.Code)
	}
	if w := s.panggil(t, http.MethodPost, "/embeddings", `{"model":"gpt-4o-mini","input":"halo"}`, nil); w.Code != http.StatusTooManyRequests {
		t.Errorf("POST /embeddings status = %d, mau 429 dari pembatas", w.Code)
	}
	if terpanggil != 2 {
		t.Errorf("pembatas terpanggil %d kali, mau 2", terpanggil)
	}
}

// --- V-GW-40: masukan token embeddings sampai ke adapter --------------------------

func TestEmbeddingsInputTokenSampaiAdapter(t *testing.T) {
	var diterima *providers.EmbeddingsRequest
	prov := &providerTiruan{
		nama: "utama", kind: providers.KindOpenAI,
		embed: func(_ context.Context, req *providers.EmbeddingsRequest) (*providers.EmbeddingsResponse, error) {
			diterima = req
			return &providers.EmbeddingsResponse{Raw: json.RawMessage(`{"object":"list","data":[]}`)}, nil
		},
	}
	s := susun(t, prov, func(s *susunan, _ *HandlersDeps) {
		m := modelUji()
		m.Capabilities = []string{upstream.CapEmbeddings}
		s.models.model = m
	})

	w := s.panggil(t, http.MethodPost, "/embeddings",
		`{"model":"gpt-4o-mini","input":[[123,456],[7]]}`, nil)
	if w.Code != http.StatusOK {
		t.Fatalf("status = %d, mau 200 (body: %s)", w.Code, w.Body.String())
	}
	if diterima == nil {
		t.Fatal("adapter embeddings tidak terpanggil")
	}
	if len(diterima.Input) != 0 {
		t.Errorf("Input teks terisi untuk masukan token: %v", diterima.Input)
	}
	if len(diterima.TokenInput) != 2 || diterima.TokenInput[0][1] != 456 || diterima.TokenInput[1][0] != 7 {
		t.Errorf("TokenInput = %v, mau [[123 456] [7]]", diterima.TokenInput)
	}
}
