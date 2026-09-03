package gateway

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync"
	"testing"

	"github.com/NexGen-X/Route-X/internal/database/repo"
	"github.com/NexGen-X/Route-X/internal/database/repo/policy"
	"github.com/NexGen-X/Route-X/internal/database/repo/traffic"
	"github.com/NexGen-X/Route-X/internal/database/repo/upstream"
	"github.com/NexGen-X/Route-X/internal/providers"
	"github.com/NexGen-X/Route-X/internal/router"
	"github.com/NexGen-X/Route-X/internal/usage"
)

// pencatatTiruan menangkap catatan yang diserahkan jalur permintaan.
type pencatatTiruan struct {
	mu sync.Mutex
	ev []usage.Event
}

func (p *pencatatTiruan) Record(_ context.Context, ev usage.Event) {
	p.mu.Lock()
	defer p.mu.Unlock()
	p.ev = append(p.ev, ev)
}

// satu memastikan tepat satu permintaan tercatat, lalu mengembalikannya.
//
// Tepat satu, bukan minimal satu: satu permintaan yang tercatat dua kali berarti tokennya
// ditagih dua kali dan pemakaian anggarannya naik dua kali.
func (p *pencatatTiruan) satu(t *testing.T) usage.Event {
	t.Helper()
	p.mu.Lock()
	defer p.mu.Unlock()
	if len(p.ev) != 1 {
		t.Fatalf("catatan pemakaian = %d, mau tepat 1", len(p.ev))
	}
	return p.ev[0]
}

// susunDenganPencatat merakit Handlers beserta pencatat tiruan.
func susunDenganPencatat(t *testing.T, prov providers.Provider, ubah func(*susunan, *HandlersDeps)) (*susunan, *pencatatTiruan) {
	t.Helper()
	sink := &pencatatTiruan{}
	s := susun(t, prov, func(su *susunan, d *HandlersDeps) {
		d.Usage = sink
		if ubah != nil {
			ubah(su, d)
		}
	})
	return s, sink
}

const bodyChat = `{"model":"gpt-4o-mini","messages":[{"role":"user","content":"hai"}]}`

func TestJejakMencatatPermintaanBerhasil(t *testing.T) {
	s, sink := susunDenganPencatat(t, providerDenganUsage("utama", 40), nil)

	w := s.panggil(t, http.MethodPost, "/chat/completions", bodyChat, nil)
	if w.Code != http.StatusOK {
		t.Fatalf("status = %d, mau 200: %s", w.Code, w.Body.String())
	}

	ev := sink.satu(t)
	if ev.StatusCode != http.StatusOK {
		t.Errorf("status tercatat = %d, mau 200", ev.StatusCode)
	}
	if ev.ErrorType != "" || ev.ErrorCode != "" {
		t.Errorf("kategori kegagalan terisi pada permintaan berhasil: %q / %q", ev.ErrorType, ev.ErrorCode)
	}
	if ev.Endpoint != "/chat/completions" || ev.Method != http.MethodPost {
		t.Errorf("endpoint/metode = %q/%q", ev.Endpoint, ev.Method)
	}
	if ev.RequestedModel != "gpt-4o-mini" || ev.ModelName != "gpt-4o-mini" {
		t.Errorf("model = %q / %q", ev.RequestedModel, ev.ModelName)
	}
	if ev.ProviderName != "utama" || ev.UpstreamModel != "gpt-4o-mini-2024" {
		t.Errorf("provider/model upstream = %q/%q", ev.ProviderName, ev.UpstreamModel)
	}
	if ev.ProviderModelID != "pm-utama" {
		t.Errorf("provider_model_id = %q; tanpa ini biaya tidak bisa dihitung", ev.ProviderModelID)
	}
	if ev.TotalTokens != 40 {
		t.Errorf("total token = %d, mau 40", ev.TotalTokens)
	}
	if ev.Latency <= 0 {
		t.Error("latensi tidak terukur")
	}
	if ev.Stream {
		t.Error("permintaan non-streaming tercatat sebagai streaming")
	}
	if ev.TTFT != 0 {
		t.Errorf("TTFT = %v pada permintaan non-streaming, mau nol", ev.TTFT)
	}
	if ev.RoutingStrategy != string(router.StrategyPriority) {
		t.Errorf("strategi = %q, mau %q", ev.RoutingStrategy, router.StrategyPriority)
	}
	if len(ev.Routing.Candidates) != 1 || !ev.Routing.Candidates[0].Chosen {
		t.Errorf("keputusan routing = %+v", ev.Routing)
	}
	if len(ev.Attempts) != 1 || !ev.Attempts[0].Berhasil() {
		t.Errorf("percobaan = %+v", ev.Attempts)
	}
}

// Nama model yang diminta klien harus tercatat WALAUPUN modelnya tidak dikenal: baris itulah
// yang dicari orang ketika kliennya menerima 404.
func TestJejakMencatatModelTakDikenal(t *testing.T) {
	s, sink := susunDenganPencatat(t, providerSukses(), func(su *susunan, _ *HandlersDeps) {
		su.models.err = repo.ErrNotFound
	})

	w := s.panggil(t, http.MethodPost, "/chat/completions",
		`{"model":"model-tidak-ada","messages":[{"role":"user","content":"hai"}]}`, nil)
	if w.Code != http.StatusNotFound {
		t.Fatalf("status = %d, mau 404", w.Code)
	}

	ev := sink.satu(t)
	if ev.RequestedModel != "model-tidak-ada" {
		t.Errorf("requested_model = %q", ev.RequestedModel)
	}
	if ev.ModelName != "" || ev.ProviderName != "" {
		t.Errorf("model kanonik atau provider terisi padahal resolusinya gagal: %q / %q", ev.ModelName, ev.ProviderName)
	}
	if ev.StatusCode != http.StatusNotFound {
		t.Errorf("status = %d, mau 404", ev.StatusCode)
	}
	if ev.ErrorType != "model_not_found" {
		t.Errorf("error_type = %q, mau model_not_found", ev.ErrorType)
	}
	if ev.ErrorCode != "model_not_found" || ev.ErrorMessage == "" {
		t.Errorf("envelope tidak terbaca dari respons: kode %q, pesan %q", ev.ErrorCode, ev.ErrorMessage)
	}
}

func TestJejakMencatatPenolakanPenyaringKonten(t *testing.T) {
	src := &sumberKebijakan{filters: []*policy.ContentFilter{
		filterBaris("f1", "tanpa kata rahasia", policy.FilterBlockedPattern, func(f *policy.ContentFilter) {
			f.Pattern = "rahasia"
			f.PatternType = policy.PatternSubstring
		}),
	}}
	s, sink := susunDenganPencatat(t, providerSukses(), func(_ *susunan, d *HandlersDeps) {
		d.Guard = guardUji(t, src)
	})

	w := s.panggil(t, http.MethodPost, "/chat/completions",
		`{"model":"gpt-4o-mini","messages":[{"role":"user","content":"ini rahasia"}]}`, nil)
	if w.Code != http.StatusBadRequest {
		t.Fatalf("status = %d, mau 400: %s", w.Code, w.Body.String())
	}

	ev := sink.satu(t)
	if ev.ErrorType != "content_filter" || ev.ErrorCode != CodeContentFilter {
		t.Errorf("error_type/code = %q/%q, mau content_filter", ev.ErrorType, ev.ErrorCode)
	}
	if len(ev.Notes) != 1 || ev.Notes[0].Kind != traffic.EventContentBlocked {
		t.Errorf("peristiwa timeline = %+v, mau satu content_blocked", ev.Notes)
	}
	if len(ev.Attempts) != 0 {
		t.Errorf("permintaan yang ditolak sebelum upstream punya %d percobaan", len(ev.Attempts))
	}
	// Isi permintaan tidak boleh ikut tercatat lewat pesan error.
	if strings.Contains(ev.ErrorMessage, "ini rahasia") {
		t.Errorf("pesan error memuat isi permintaan: %q", ev.ErrorMessage)
	}
}

// Kategori kegagalan upstream diambil dari klasifikasi providers.Error, bukan dari kode
// envelope: satu kode seperti "upstream_error" mewakili beberapa kategori sekaligus, dan yang
// dipakai mengukur tingkat timeout adalah kategori aslinya.
func TestJejakMencatatKategoriKegagalanUpstream(t *testing.T) {
	prov := &providerTiruan{
		nama: "utama", kind: providers.KindOpenAI,
		chat: func(context.Context, *providers.ChatRequest) (*providers.ChatResponse, error) {
			return nil, &providers.Error{
				Kind: providers.ErrKindTimeout, Provider: "utama",
				Message: "upstream tidak menjawab sebelum batas waktu",
			}
		},
	}
	s, sink := susunDenganPencatat(t, prov, nil)

	w := s.panggil(t, http.MethodPost, "/chat/completions", bodyChat, nil)
	if w.Code != http.StatusGatewayTimeout {
		t.Fatalf("status = %d, mau 504: %s", w.Code, w.Body.String())
	}

	ev := sink.satu(t)
	if ev.ErrorType != string(providers.ErrKindTimeout) {
		t.Errorf("error_type = %q, mau timeout", ev.ErrorType)
	}
	if ev.ErrorCode != "upstream_timeout" {
		t.Errorf("error_code = %q, mau upstream_timeout", ev.ErrorCode)
	}
	// Tiga percobaan pada satu kandidat: dua di antaranya percobaan ulang, tanpa failover.
	if ev.RetryCount != 2 || ev.FailoverCount != 0 {
		t.Errorf("retry/failover = %d/%d, mau 2/0", ev.RetryCount, ev.FailoverCount)
	}
	if ev.UpstreamLatency < 0 {
		t.Error("waktu hulu negatif")
	}
}

func TestJejakMenghitungFailoverAntarKandidat(t *testing.T) {
	mati := &providerTiruan{
		nama: "mati", kind: providers.KindOpenAI,
		chat: func(context.Context, *providers.ChatRequest) (*providers.ChatResponse, error) {
			return nil, &providers.Error{Kind: providers.ErrKindNetwork, Provider: "mati", Message: "koneksi ditolak"}
		},
	}
	sehat := providerDenganUsage("sehat", 12)

	s, sink := susunDenganPencatat(t, nil, func(su *susunan, _ *HandlersDeps) {
		su.cands.cands = []*upstream.RouteCandidate{
			kandidatRute("mati", "m-1"),
			kandidatRute("sehat", "s-1"),
		}
		su.factory.perNama = map[string]providers.Provider{"mati": mati, "sehat": sehat}
	})

	w := s.panggil(t, http.MethodPost, "/chat/completions", bodyChat, nil)
	if w.Code != http.StatusOK {
		t.Fatalf("status = %d, mau 200: %s", w.Code, w.Body.String())
	}

	ev := sink.satu(t)
	if ev.ProviderName != "sehat" {
		t.Errorf("provider tercatat = %q, mau sehat", ev.ProviderName)
	}
	// Tiga percobaan ke kandidat pertama lalu satu ke kandidat kedua.
	if ev.RetryCount != 2 || ev.FailoverCount != 1 {
		t.Errorf("retry/failover = %d/%d, mau 2/1", ev.RetryCount, ev.FailoverCount)
	}
	if len(ev.Attempts) != 4 {
		t.Fatalf("percobaan = %d, mau 4", len(ev.Attempts))
	}
	if ev.Attempts[0].ErrorKind != string(providers.ErrKindNetwork) {
		t.Errorf("percobaan pertama = %+v", ev.Attempts[0])
	}
	if !ev.Attempts[3].Berhasil() {
		t.Errorf("percobaan terakhir = %+v, mau berhasil", ev.Attempts[3])
	}
	// Kandidat yang dicoba dan gagal harus tetap ada di keputusan routing, kalau tidak
	// pertanyaan "kenapa permintaan ini pergi ke sana" tidak bisa dijawab.
	if len(ev.Routing.Candidates) != 2 || ev.Routing.Candidates[0].Chosen {
		t.Errorf("keputusan routing = %+v", ev.Routing)
	}
}

func TestJejakTTFTHanyaDitandaiPotonganKonten(t *testing.T) {
	// Potongan pembuka berdialek OpenAI hanya membawa peran. Ia datang sebelum token pertama
	// dihasilkan, jadi memakainya sebagai penanda TTFT melaporkan angka yang jauh lebih cepat
	// daripada yang dialami pengguna.
	pembuka := &providers.StreamEvent{
		Raw: []byte(`{"id":"c1","object":"chat.completion.chunk","choices":[{"index":0,"delta":{"role":"assistant"}}]}`),
	}

	tests := []struct {
		nama    string
		ev      []*providers.StreamEvent
		adaTTFT bool
	}{
		{nama: "hanya potongan peran", ev: []*providers.StreamEvent{pembuka}, adaTTFT: false},
		{nama: "peran lalu konten", ev: []*providers.StreamEvent{pembuka, peristiwa("hai")}, adaTTFT: true},
	}

	for _, tc := range tests {
		t.Run(tc.nama, func(t *testing.T) {
			prov := &providerTiruan{
				nama: "utama", kind: providers.KindOpenAI,
				alir: func(context.Context, *providers.ChatRequest) (providers.Stream, error) {
					return &aliranTiruan{ev: tc.ev}, nil
				},
			}
			s, sink := susunDenganPencatat(t, prov, nil)

			w := s.panggil(t, http.MethodPost, "/chat/completions",
				`{"model":"gpt-4o-mini","stream":true,"messages":[{"role":"user","content":"hai"}]}`, nil)
			if w.Code != http.StatusOK {
				t.Fatalf("status = %d, mau 200", w.Code)
			}

			ev := sink.satu(t)
			if !ev.Stream {
				t.Error("permintaan streaming tidak tercatat sebagai streaming")
			}
			if ada := ev.TTFT > 0; ada != tc.adaTTFT {
				t.Errorf("TTFT = %v, mau terisi=%v", ev.TTFT, tc.adaTTFT)
			}
			// Waktu hulu satu aliran mencakup pembacaan, bukan hanya pembukaannya.
			if ev.UpstreamLatency <= 0 {
				t.Error("waktu hulu aliran tidak terukur")
			}
		})
	}
}

// Aliran yang terputus setelah sebagian terkirim sampai ke klien sebagai 200, dan status itu
// benar. Yang menjadikannya bisa dihitung sebagai kegagalan adalah error_type — karena itu
// agregasi memakai "error_type is not null", bukan hanya "status >= 400".
func TestJejakMencatatAliranTerputusSebagaiGagalPadaStatus200(t *testing.T) {
	prov := &providerTiruan{
		nama: "utama", kind: providers.KindOpenAI,
		alir: func(context.Context, *providers.ChatRequest) (providers.Stream, error) {
			return &aliranTiruan{
				ev: []*providers.StreamEvent{peristiwa("hai")},
				akhir: &providers.Error{
					Kind: providers.ErrKindStreamAborted, Provider: "utama",
					Message: "aliran terputus di tengah",
				},
			}, nil
		},
	}
	s, sink := susunDenganPencatat(t, prov, nil)

	w := s.panggil(t, http.MethodPost, "/chat/completions",
		`{"model":"gpt-4o-mini","stream":true,"messages":[{"role":"user","content":"hai"}]}`, nil)
	if w.Code != http.StatusOK {
		t.Fatalf("status = %d, mau 200 karena sebagian sudah terkirim", w.Code)
	}

	ev := sink.satu(t)
	if ev.StatusCode != http.StatusOK {
		t.Errorf("status tercatat = %d, mau 200", ev.StatusCode)
	}
	if ev.ErrorType != string(providers.ErrKindStreamAborted) {
		t.Fatalf("error_type = %q, mau stream_aborted; tanpa ini kegagalan streaming tidak pernah terhitung", ev.ErrorType)
	}
}

func TestJejakMencatatEmbeddings(t *testing.T) {
	prov := &providerTiruan{
		nama: "utama", kind: providers.KindOpenAI,
		embed: func(context.Context, *providers.EmbeddingsRequest) (*providers.EmbeddingsResponse, error) {
			return &providers.EmbeddingsResponse{
				Raw:   json.RawMessage(`{"object":"list","data":[]}`),
				Usage: providers.Usage{InputTokens: 9, TotalTokens: 9},
			}, nil
		},
	}
	s, sink := susunDenganPencatat(t, prov, func(su *susunan, _ *HandlersDeps) {
		su.cands.cands = []*upstream.RouteCandidate{kandidatRute("utama", "embed-1")}
	})

	w := s.panggil(t, http.MethodPost, "/embeddings", `{"model":"gpt-4o-mini","input":"hai"}`, nil)
	if w.Code != http.StatusOK {
		t.Fatalf("status = %d, mau 200: %s", w.Code, w.Body.String())
	}

	ev := sink.satu(t)
	if ev.Endpoint != "/embeddings" || ev.TotalTokens != 9 || ev.ProviderName != "utama" {
		t.Errorf("catatan embeddings = %+v", ev)
	}
	if ev.Stream {
		t.Error("embeddings tercatat sebagai streaming")
	}
}

// GET /v1/models tidak dicatat: ia tidak punya model, tidak punya provider, dan tidak
// memakai satu token pun, jadi barisnya hanya menambah derau di halaman Requests. Jejaknya
// ada di access log.
func TestDaftarModelTidakDicatatSebagaiPemakaian(t *testing.T) {
	s, sink := susunDenganPencatat(t, providerSukses(), func(su *susunan, _ *HandlersDeps) {
		su.lister.models = []*upstream.Model{modelUji()}
	})

	if w := s.panggil(t, http.MethodGet, "/models", "", nil); w.Code != http.StatusOK {
		t.Fatalf("status = %d, mau 200", w.Code)
	}

	sink.mu.Lock()
	defer sink.mu.Unlock()
	if len(sink.ev) != 0 {
		t.Fatalf("GET /models menghasilkan %d catatan pemakaian, mau 0", len(sink.ev))
	}
}

func TestPencatatResponsMenangkapStatusDanEnvelope(t *testing.T) {
	tests := []struct {
		nama       string
		tulis      func(w http.ResponseWriter)
		status     int
		kode       string
		adaEnvelop bool
	}{
		{
			nama:   "respons sukses tanpa WriteHeader",
			tulis:  func(w http.ResponseWriter) { _, _ = w.Write([]byte("halo")) },
			status: http.StatusOK,
		},
		{
			nama: "envelope error terbaca",
			tulis: func(w http.ResponseWriter) {
				w.Header().Set("Content-Type", "application/json")
				w.WriteHeader(http.StatusTooManyRequests)
				_, _ = w.Write([]byte(`{"error":{"message":"terlalu cepat","type":"rate_limit_error","code":"rate_limit_exceeded"}}`))
			},
			status: http.StatusTooManyRequests, kode: CodeRateLimited, adaEnvelop: true,
		},
		{
			nama: "badan sukses tidak ditangkap",
			tulis: func(w http.ResponseWriter) {
				w.Header().Set("Content-Type", "application/json")
				w.WriteHeader(http.StatusOK)
				_, _ = w.Write([]byte(`{"rahasia":"jawaban model"}`))
			},
			status: http.StatusOK,
		},
		{
			nama: "status pertama yang menang",
			tulis: func(w http.ResponseWriter) {
				w.WriteHeader(http.StatusServiceUnavailable)
				w.WriteHeader(http.StatusOK)
			},
			status: http.StatusServiceUnavailable,
		},
	}

	for _, tc := range tests {
		t.Run(tc.nama, func(t *testing.T) {
			p := &pencatatRespons{ResponseWriter: httptest.NewRecorder()}
			tc.tulis(p)

			if p.status != tc.status {
				t.Errorf("status = %d, mau %d", p.status, tc.status)
			}
			kode, pesan := p.envelope()
			if kode != tc.kode {
				t.Errorf("kode envelope = %q, mau %q", kode, tc.kode)
			}
			if tc.adaEnvelop && pesan == "" {
				t.Error("pesan envelope kosong")
			}
			if !tc.adaEnvelop && len(p.badan) > 0 {
				t.Errorf("badan tertangkap padahal seharusnya tidak: %q", p.badan)
			}
		})
	}
}
