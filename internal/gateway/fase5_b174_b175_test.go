package gateway

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"testing"

	"github.com/NexGen-X/Route-X/internal/database/repo"
	"github.com/NexGen-X/Route-X/internal/database/repo/upstream"
	"github.com/NexGen-X/Route-X/internal/providers"
)

// Test pengunci B-174 dan B-175 (Fase 5 kampanye tuntas-logika).
//
// B-174: embeddings tanpa lantai pemakaian — usage kosong dari
// upstream (Gemini memang tidak melaporkan token pada endpoint
// embeddings) tercatat 0 token: gratis dan senyap.
// B-175: GET /v1/models terpotong diam-diam pada 500 model karena
// kursor lanjutan repo dibuang; has_more dihitung dari irisan yang
// sudah terambil sehingga bisa berbohong.

// --- B-174 -------------------------------------------------------------------

func TestB174EmbeddingsUsageKosongDitaksirDariMasukan(t *testing.T) {
	prov := &providerTiruan{
		nama: "utama", kind: providers.KindOpenAI,
		embed: func(context.Context, *providers.EmbeddingsRequest) (*providers.EmbeddingsResponse, error) {
			// Usage nol persis seperti adapter Google pada endpoint
			// embeddings (lihat catatan di lantaiPemakaianEmbeddings).
			return &providers.EmbeddingsResponse{
				Raw: json.RawMessage(`{"object":"list","data":[]}`),
			}, nil
		},
	}
	s, sink := susunDenganPencatat(t, prov, func(su *susunan, _ *HandlersDeps) {
		su.cands.cands = []*upstream.RouteCandidate{kandidatRute("utama", "embed-1")}
		su.models.model.Capabilities = []string{upstream.CapEmbeddings}
	})

	masukan := "hai dunia ini teks masukan embeddings" // 37 byte -> ceil(37/4) = 10 token
	w := s.panggil(t, http.MethodPost, "/embeddings",
		`{"model":"gpt-4o-mini","input":"`+masukan+`"}`, nil)
	if w.Code != http.StatusOK {
		t.Fatalf("status = %d, mau 200: %s", w.Code, w.Body.String())
	}

	ev := sink.satu(t)
	if ev.TotalTokens != 10 || ev.InputTokens != 10 {
		t.Errorf("B-174: pemakaian tercatat = %d/%d token (total/input), mau 10/10 dari taksiran masukan — usage kosong upstream tidak boleh tercatat nol",
			ev.TotalTokens, ev.InputTokens)
	}
}

func TestB174LantaiEmbeddingsMenghormatiUsageUpstream(t *testing.T) {
	req := &providers.EmbeddingsRequest{Input: []string{"teks berapa pun panjangnya tidak mengubah angka upstream"}}
	u := lantaiPemakaianEmbeddings(req, providers.Usage{InputTokens: 9, TotalTokens: 9})
	if u.TotalTokens != 9 || u.InputTokens != 9 {
		t.Errorf("usage upstream ditimpa lantai: %+v, mau tetap 9/9", u)
	}
	// Total kosong tetapi input terisi: total dilengkapi dari input.
	u = lantaiPemakaianEmbeddings(req, providers.Usage{InputTokens: 7})
	if u.TotalTokens != 7 {
		t.Errorf("total = %d, mau 7 dari input yang dilaporkan", u.TotalTokens)
	}
	// Masukan berbentuk token array dihitung persis, bukan ditaksir.
	req = &providers.EmbeddingsRequest{TokenInput: [][]int{{1, 2, 3}, {4, 5}}}
	u = lantaiPemakaianEmbeddings(req, providers.Usage{})
	if u.TotalTokens != 5 || u.InputTokens != 5 {
		t.Errorf("token array: pemakaian = %+v, mau 5/5 persis", u)
	}
}

// --- B-175 -------------------------------------------------------------------

// pendaftarBerhalaman adalah ModelLister yang benar-benar memakai
// kursor: halaman berikutnya hanya keluar bila pemanggil meneruskan
// kursor yang ia terima — persis kontrak repo.
type pendaftarBerhalaman struct {
	halaman   [][]*upstream.Model
	dipanggil int
}

func (p *pendaftarBerhalaman) List(_ context.Context, _ upstream.ModelFilter, page repo.Page) ([]*upstream.Model, string, error) {
	i := 0
	if page.Cursor != "" {
		if _, err := fmt.Sscanf(page.Cursor, "kursor-%d", &i); err != nil {
			return nil, "", err
		}
	}
	p.dipanggil++
	if i >= len(p.halaman) {
		return nil, "", nil
	}
	next := ""
	if i+1 < len(p.halaman) {
		next = fmt.Sprintf("kursor-%d", i+1)
	}
	return p.halaman[i], next, nil
}

func modelKatalogB175(id, nama string) *upstream.Model {
	m := modelUji()
	m.ID = id
	m.ModelID = nama
	return m
}

func TestB175DaftarModelMengikutiKursorRepo(t *testing.T) {
	pendaftar := &pendaftarBerhalaman{halaman: [][]*upstream.Model{
		{modelKatalogB175("id-1", "model-a")},
		{modelKatalogB175("id-2", "model-b"), modelKatalogB175("id-3", "model-c")},
	}}
	s := susun(t, providerSukses(), func(_ *susunan, d *HandlersDeps) {
		d.Lister = pendaftar
	})

	w := s.panggil(t, http.MethodGet, "/models", "", nil)
	if w.Code != http.StatusOK {
		t.Fatalf("status = %d, mau 200: %s", w.Code, w.Body.String())
	}
	var daftar modelList
	if err := json.Unmarshal(w.Body.Bytes(), &daftar); err != nil {
		t.Fatalf("body bukan daftar model: %v", err)
	}
	if len(daftar.Data) != 3 {
		t.Errorf("B-175: entri = %d, mau 3 — katalog terpotong pada halaman pertama repo (kursor dibuang)", len(daftar.Data))
	}
	if daftar.HasMore {
		t.Errorf("B-175: has_more = true padahal seluruh katalog (3 model) sudah dikembalikan tanpa parameter limit")
	}
	if pendaftar.dipanggil != 2 {
		t.Errorf("List dipanggil %d kali, mau 2 (kursor diikuti sampai habis)", pendaftar.dipanggil)
	}

	// Kontrol paginasi klien di atas katalog utuh: limit=2 menyisakan
	// satu entri, dan has_more kini jujur karena dihitung dari
	// gabungan seluruh katalog.
	w = s.panggil(t, http.MethodGet, "/models?limit=2", "", nil)
	daftar = modelList{}
	if err := json.Unmarshal(w.Body.Bytes(), &daftar); err != nil {
		t.Fatal(err)
	}
	if len(daftar.Data) != 2 || !daftar.HasMore {
		t.Errorf("kontrol limit=2: entri/has_more = %d/%v, mau 2/true", len(daftar.Data), daftar.HasMore)
	}
}
