// Permukaan API /v1: endpoint berdialek OpenAI yang dilihat aplikasi klien.
//
// Janji fase ini satu kalimat: aplikasi yang sudah memakai klien OpenAI cukup menukar base
// URL dan API key. Itu yang menentukan setiap pilihan di file ini — bentuk body, bentuk
// error, bentuk peristiwa SSE, dan penanda penutup — semuanya mengikuti apa yang klien
// resmi mereka harapkan, bukan apa yang paling rapi bagi kami.
//
// # Batas yang tidak boleh dilewati
//
// Begitu satu byte respons terkirim, status HTTP tidak bisa diubah lagi. Untuk permintaan
// streaming itu berarti SELURUH validasi, resolusi model, pemilihan rute, dan pembukaan
// aliran upstream harus selesai SEBELUM httpx.PrepareSSE dipanggil. Kegagalan setelah titik
// itu hanya bisa dilaporkan sebagai peristiwa di dalam stream, dan klien yang tidak
// membacanya akan melihat percakapan yang berhenti di tengah tanpa penjelasan.
package gateway

import (
	"context"
	"encoding/json"
	"errors"
	"io"
	"log/slog"
	"net/http"
	"strings"
	"time"

	"github.com/go-chi/chi/v5"

	"github.com/NexGen-X/Route-X/internal/apikey"
	"github.com/NexGen-X/Route-X/internal/contentfilter"
	"github.com/NexGen-X/Route-X/internal/database/repo"
	"github.com/NexGen-X/Route-X/internal/database/repo/policy"
	"github.com/NexGen-X/Route-X/internal/database/repo/upstream"
	"github.com/NexGen-X/Route-X/internal/httpx"
	"github.com/NexGen-X/Route-X/internal/observability"
	"github.com/NexGen-X/Route-X/internal/providers"
	"github.com/NexGen-X/Route-X/internal/responsecache"
	"github.com/NexGen-X/Route-X/internal/router"
)

// maxBodyBytes adalah batas body yang dibaca handler di paket ini.
//
// Middleware httpx.MaxBytes sudah membatasi di tingkat server; batas kedua ini ada karena
// handler membaca seluruh body ke memori (perlu, karena field tak dikenal harus dipungut
// utuh untuk diteruskan), dan batas server bisa disetel jauh lebih longgar untuk keperluan
// lain seperti unggahan di dashboard.
const maxBodyBytes = 32 << 20

// ModelResolver menyelesaikan nama model yang diminta klien menjadi model kanonik.
//
// Dipenuhi *upstream.ModelRepo. Resolve menerima alias maupun nama kanonik.
type ModelResolver interface {
	Resolve(ctx context.Context, requested string) (*upstream.Model, error)
}

// ModelLister mendaftar model yang dilayani gateway ini.
type ModelLister interface {
	List(ctx context.Context, f upstream.ModelFilter, page repo.Page) ([]*upstream.Model, string, error)
}

// CandidateSource mengambil kandidat rute satu model.
type CandidateSource interface {
	RouteCandidates(ctx context.Context, q upstream.RouteQuery) ([]*upstream.RouteCandidate, error)
}

// KeyRestrictions memeriksa pembatasan model dan provider satu API key.
//
// Dipenuhi *keys.Repo. Kedua method mengembalikan true bila key TIDAK punya pembatasan,
// yang merupakan keadaan paling umum.
type KeyRestrictions interface {
	AllowsModel(ctx context.Context, keyID, modelID string) (bool, error)
	AllowsProvider(ctx context.Context, keyID, providerID string) (bool, error)
}

// ProviderFactory membuat adapter dari kandidat rute.
//
// Antarmuka, bukan *Factory langsung, supaya handler bisa diuji dengan provider tiruan
// tanpa menyentuh kredensial maupun jaringan.
type ProviderFactory interface {
	Provider(ctx context.Context, c *upstream.RouteCandidate) (providers.Provider, error)
}

// Handlers menyajikan endpoint /v1.
type Handlers struct {
	models     ModelResolver
	lister     ModelLister
	candidates CandidateSource
	restrict   KeyRestrictions
	factory    ProviderFactory
	engine     *router.Engine
	exec       *Executor
	guard      *Guard
	usage      UsageSink
	respCache  *responsecache.Engine
	logger     *slog.Logger
}

// HandlersDeps mengumpulkan dependensi Handlers.
//
// Struct alih-alih tujuh parameter berurut: konstruktor dengan banyak parameter bertipe
// antarmuka adalah tempat dua argumen tertukar tanpa satu pun keluhan dari compiler.
type HandlersDeps struct {
	Models     ModelResolver
	Lister     ModelLister
	Candidates CandidateSource
	Factory    ProviderFactory
	Engine     *router.Engine
	Executor   *Executor

	// Restrict boleh nil, yang berarti tanpa pembatasan model/provider per API key.
	Restrict KeyRestrictions
	// Guard menegakkan penyaring konten, batas laju cakupan model dan provider, serta
	// anggaran biaya. nil berarti tidak satu pun kebijakan itu ditegakkan.
	Guard *Guard
	// Usage menerima hasil setiap permintaan: token, biaya, latensi, dan jejak
	// percobaannya. nil berarti TIDAK ADA baris log request yang ditulis dan tidak ada
	// pemakaian anggaran yang naik — sah untuk test, tetapi di produksi berarti halaman
	// Requests kosong dan setiap anggaran menegakkan angka yang tidak bergerak.
	Usage UsageSink
	// ResponseCache opsional; jika terpasang, menghemat latensi dan biaya untuk prompt berulang.
	ResponseCache *responsecache.Engine
	Logger        *slog.Logger
}

// NewHandlers membuat Handlers. Mengembalikan error bila ada dependensi wajib yang kosong.
//
// Diperiksa di sini, bukan dibiarkan menjadi nil-pointer di jalur permintaan: kegagalan
// susunan seperti ini harus muncul saat start, bukan pada permintaan pertama pengguna.
func NewHandlers(d HandlersDeps) (*Handlers, error) {
	switch {
	case d.Models == nil:
		return nil, errors.New("gateway: ModelResolver wajib diisi")
	case d.Candidates == nil:
		return nil, errors.New("gateway: CandidateSource wajib diisi")
	case d.Factory == nil:
		return nil, errors.New("gateway: ProviderFactory wajib diisi")
	case d.Executor == nil:
		return nil, errors.New("gateway: Executor wajib diisi")
	}

	engine := d.Engine
	if engine == nil {
		// Tanpa mesin aturan, seluruh permintaan memakai strategi bawaan atas semua
		// kandidat. Itu perilaku yang benar untuk pemasangan yang belum punya aturan.
		engine = router.NewEngine(nil, nil, d.Logger)
	}
	logger := d.Logger
	if logger == nil {
		logger = slog.New(slog.DiscardHandler)
	}

	guard := d.Guard
	if guard == nil {
		// Guard kosong, bukan nil: setiap pemeriksaan di dalamnya sudah menangani dependensi
		// yang tidak terpasang, jadi jalur request tidak perlu memeriksa nil di enam tempat.
		guard = NewGuard(GuardDeps{Logger: logger})
	}

	return &Handlers{
		models: d.Models, lister: d.Lister, candidates: d.Candidates,
		restrict: d.Restrict, factory: d.Factory,
		engine: engine, exec: d.Executor, guard: guard, usage: d.Usage,
		respCache: d.ResponseCache, logger: logger,
	}, nil
}

// Routes mengembalikan sub-router /v1.
//
// CSRF TIDAK dipasang di sini, dan itu bukan kelalaian: rute ini berautentikasi Bearer API
// key, dan browser tidak pernah melampirkan header Authorization sendiri pada permintaan
// lintas situs. CSRF hanya bermakna untuk kredensial yang dikirim browser secara otomatis,
// yaitu cookie sesi dashboard.
//
// Autentikasi API key dipasang PEMANGGIL, bukan di sini, supaya susunan middleware seluruh
// server tetap terbaca di satu tempat.
func (h *Handlers) Routes() http.Handler {
	r := chi.NewRouter()
	r.Post("/chat/completions", h.ChatCompletions)
	// /v1/responses adalah nama baru OpenAI untuk permukaan yang sama pada pemakaian
	// percakapan biasa. Dilayani handler yang sama supaya klien yang sudah pindah ke nama
	// itu tetap bekerja; perbedaan bentuk body-nya yang belum didukung diteruskan lewat
	// ChatRequest.Extra, bukan ditelan diam-diam.
	r.Post("/responses", h.ChatCompletions)
	r.Post("/embeddings", h.Embeddings)
	r.Get("/models", h.Models)
	return r
}

// persiapan adalah hasil seluruh langkah sebelum upstream dihubungi.
type persiapan struct {
	req      *providers.ChatRequest
	model    *upstream.Model
	decision router.Decision
	plan     Plan

	// principal dan targets dibawa dari tahap persiapan supaya pencatatan token dan
	// pemeriksaan kebijakan sesudahnya tidak perlu menyusunnya ulang — dan supaya keduanya
	// pasti memakai daftar cakupan yang SAMA dengan yang dipakai saat menggerbangi
	// permintaan ini. Daftar yang disusun dua kali adalah daftar yang bisa berbeda.
	principal *apikey.Principal
	targets   []policy.Target
}

// siapkanChat menjalankan seluruh langkah yang harus selesai sebelum satu byte respons
// terkirim: mengurai body, menyelesaikan model, memeriksa kewenangan key, mengambil
// kandidat, dan memilih rute.
//
// Urutannya penting untuk permintaan streaming. Semua yang bisa menghasilkan status HTTP
// harus terjadi di sini, karena setelah PrepareSSE status tidak bisa diubah lagi — dan
// klien yang menerima 200 lalu peristiwa error di dalam stream jauh lebih sulit menangani
// kegagalan daripada klien yang menerima 400.
//
// ok false berarti jawaban sudah ditulis; pemanggil cukup return.
func (h *Handlers) siapkanChat(w http.ResponseWriter, r *http.Request, j *jejak) (*persiapan, bool) {
	body, ok := h.bacaBody(w, r)
	if !ok {
		return nil, false
	}

	req, err := DecodeChatRequest(body)
	if err != nil {
		httpx.BadRequest(w, r, httpx.CodeInvalidRequest, err.Error())
		return nil, false
	}
	j.pasangDiminta(req.Model)

	// Penyaring konten fase pertama: ukuran dan pola, keduanya bisa dijawab dari body saja.
	// Dijalankan sebelum resolusi model supaya permintaan yang jelas ditolak tidak menyentuh
	// database sama sekali.
	if rj := h.guard.SaringPermintaan(r.Context(), contentfilter.Subject{
		Text:  teksPermintaan(req),
		Bytes: int64(len(body)),
	}); rj != nil {
		rj.Tulis(w, r)
		return nil, false
	}

	model, ok := h.selesaikanModel(w, r, req.Model)
	if !ok {
		return nil, false
	}
	j.pasangModel(model)

	p, _ := apikey.PrincipalFrom(r.Context())
	if !h.izinModel(w, r, p, model) {
		return nil, false
	}

	clientIP, _ := httpx.ClientIPFrom(r.Context())
	targets := TargetsFor(p, model.ID, clientIP)
	j.pasangKebijakan(p, targets)
	if !h.kebijakan(w, r, p, model, targets) {
		return nil, false
	}

	rreq := router.Request{
		ModelID:      model.ID,
		ModelName:    req.Model,
		APIKeyID:     p.ID(),
		OwnerUserID:  p.OwnerUserID(),
		Capabilities: RequiredCapabilities(req),
		Streaming:    req.Stream,
		Tokens:       EstimateTokens(req),
	}

	cands, ok := h.kandidat(w, r, rreq, p)
	if !ok {
		return nil, false
	}

	keputusan := h.engine.Route(r.Context(), rreq, cands)
	j.pasangKeputusan(keputusan, req.Stream)
	if len(keputusan.Candidates) == 0 {
		h.tolakTanpaKandidat(w, r, rreq, model, cands)
		return nil, false
	}

	return &persiapan{
		req: req, model: model, decision: keputusan,
		principal: p, targets: targets,
		// Nama model kanonik, bukan nama upstream, menjadi bagian kunci pemutus arus:
		// satu model kanonik dipetakan ke nama berbeda di setiap provider, dan kunci yang
		// memakai nama upstream tidak bisa dibaca dashboard sebagai satu kesatuan.
		plan: PlanFromRule(keputusan.Rule, model.ModelID, keputusan.Candidates),
	}, true
}

// kebijakan menjalankan pemeriksaan yang menuntut model sudah diketahui: pembatasan model,
// batas laju cakupan model, anggaran biaya, dan gerbang anggaran token.
//
// Urutannya dari yang paling murah: pembatasan model dijawab dari memori, batas laju dan
// gerbang token satu perjalanan ke Redis, anggaran satu pembacaan dari salinan ber-TTL.
//
// ok false berarti jawaban sudah ditulis.
func (h *Handlers) kebijakan(
	w http.ResponseWriter, r *http.Request,
	p *apikey.Principal, model *upstream.Model, targets []policy.Target,
) bool {
	if rj := h.guard.SaringPermintaan(r.Context(), contentfilter.Subject{ModelID: model.ID}); rj != nil {
		rj.Tulis(w, r)
		return false
	}
	if rj := h.guard.PeriksaBatasModel(r.Context(), model.ModelID); rj != nil {
		rj.Tulis(w, r)
		return false
	}
	if rj := h.guard.PeriksaAnggaran(r.Context(), targets); rj != nil {
		rj.Tulis(w, r)
		return false
	}
	if rj := h.guard.GerbangToken(r.Context(), p, targets); rj != nil {
		rj.Tulis(w, r)
		return false
	}
	return true
}

// teksPermintaan menggabungkan seluruh teks pesan untuk diperiksa penyaring konten.
//
// Yang digabungkan hanya bagian TEKS: data URI gambar dan audio base64 bukan teks yang
// bermakna bagi pola konten, dan menyertakannya berarti setiap aturan pola dijalankan atas
// megabyte base64 pada setiap permintaan multimodal. providers.Message.Text() yang
// memutuskan bagian mana yang ikut.
func teksPermintaan(req *providers.ChatRequest) string {
	if req == nil || len(req.Messages) == 0 {
		return ""
	}
	var b strings.Builder
	for i := range req.Messages {
		t := req.Messages[i].Text()
		if t == "" {
			continue
		}
		if b.Len() > 0 {
			b.WriteByte('\n')
		}
		b.WriteString(t)
	}
	return b.String()
}

// teksJawaban menggabungkan seluruh teks jawaban untuk diperiksa penyaring konten.
func teksJawaban(resp *providers.ChatResponse) string {
	if resp == nil {
		return ""
	}
	var b strings.Builder
	for i := range resp.Choices {
		t := resp.Choices[i].Message.Text()
		if t == "" {
			continue
		}
		if b.Len() > 0 {
			b.WriteByte('\n')
		}
		b.WriteString(t)
	}
	return b.String()
}

// bacaBody membaca body permintaan dengan batas.
func (h *Handlers) bacaBody(w http.ResponseWriter, r *http.Request) ([]byte, bool) {
	if r.Body == nil {
		httpx.BadRequest(w, r, httpx.CodeInvalidRequest, "body permintaan kosong")
		return nil, false
	}
	body, err := io.ReadAll(io.LimitReader(r.Body, maxBodyBytes+1))
	if err != nil {
		// Kegagalan membaca body hampir selalu berarti klien memutus koneksi di tengah
		// pengiriman. Tidak ada yang bisa dilaporkan ke pihak yang sudah pergi, tetapi
		// status tetap ditulis supaya log mencatat sebabnya.
		httpx.BadRequest(w, r, httpx.CodeInvalidRequest, "body permintaan tidak bisa dibaca sampai selesai")
		return nil, false
	}
	if len(body) > maxBodyBytes {
		httpx.PayloadTooLarge(w, r, maxBodyBytes)
		return nil, false
	}
	if len(body) == 0 {
		httpx.BadRequest(w, r, httpx.CodeInvalidRequest, "body permintaan kosong")
		return nil, false
	}
	return body, true
}

// selesaikanModel mengubah nama yang diminta klien menjadi model kanonik.
//
// Tiga jawaban yang berbeda dibedakan di sini, dan ketiganya berarti tindak lanjut yang
// berbeda bagi pengguna: nama tidak dikenal, model dimatikan operator, dan model ditandai
// usang. Menyamakan ketiganya menjadi "model tidak ditemukan" membuat pengguna mengganti
// nama model yang sebenarnya sudah benar.
func (h *Handlers) selesaikanModel(w http.ResponseWriter, r *http.Request, diminta string) (*upstream.Model, bool) {
	model, err := h.models.Resolve(r.Context(), diminta)
	switch {
	case errors.Is(err, repo.ErrNotFound):
		httpx.WriteError(w, r, http.StatusNotFound, httpx.ErrTypeInvalidRequest, "model_not_found",
			"model "+kutip(diminta)+" tidak dikenal gateway ini; lihat GET /v1/models")
		return nil, false
	case err != nil:
		h.logger.ErrorContext(r.Context(), "resolusi model gagal", "model", diminta, "error", err)
		httpx.InternalError(w, r)
		return nil, false
	}

	// Model yang dimatikan sengaja tetap dikembalikan Resolve, supaya bisa dibedakan di
	// sini. Lihat komentar ModelRepo.Resolve.
	if !model.Enabled {
		httpx.WriteError(w, r, http.StatusNotFound, httpx.ErrTypeInvalidRequest, "model_disabled",
			"model "+kutip(diminta)+" dimatikan pada gateway ini")
		return nil, false
	}
	// Model usang MASIH DILAYANI: menandainya usang adalah pemberitahuan, bukan pemutusan,
	// dan menolaknya di sini akan mematikan lalu lintas pelanggan tanpa masa peralihan.
	// Yang dilakukan hanya mencatatnya, supaya operator punya angka nyata sebelum benar-benar
	// mematikannya.
	if model.IsDeprecated() {
		h.logger.InfoContext(r.Context(), "permintaan ke model yang sudah ditandai usang",
			"model", model.ModelID, "usang_sejak", model.DeprecatedAt)
	}
	return model, true
}

// izinModel memeriksa pembatasan model pada API key.
func (h *Handlers) izinModel(w http.ResponseWriter, r *http.Request, p *apikey.Principal, model *upstream.Model) bool {
	if h.restrict == nil || p.ID() == "" {
		return true
	}
	boleh, err := h.restrict.AllowsModel(r.Context(), p.ID(), model.ID)
	if err != nil {
		h.logger.ErrorContext(r.Context(), "pemeriksaan pembatasan model gagal",
			"api_key", p.Masked(), "model", model.ModelID, "error", err)
		// GAGAL-TERTUTUP, berbeda dari pembatas laju. Ini kontrol kewenangan: melewatkannya
		// saat database tidak bisa dibaca berarti key yang dibatasi mendapat akses ke model
		// yang justru dilarang untuknya. Batas laju yang terlewat hanya membebani kuota;
		// kewenangan yang terlewat adalah pelanggaran.
		httpx.InternalError(w, r)
		return false
	}
	if !boleh {
		httpx.WriteError(w, r, http.StatusForbidden, httpx.ErrTypePermission, "model_not_allowed",
			"API key ini tidak diizinkan memakai model "+kutip(model.ModelID))
		return false
	}
	return true
}

// kandidat mengambil kandidat rute lalu menyaringnya menurut pembatasan provider pada key.
//
// Penyaringan provider berbiaya satu query per provider YANG BERBEDA, dan hasilnya
// diingat selama permintaan ini. Pada key tanpa pembatasan — keadaan paling umum — query
// itu dijawab satu `not exists` dan tidak menyentuh baris mana pun. Bentuk yang lebih baik
// adalah satu query untuk seluruh himpunan, dan itu tempat yang tepat untuk dioptimalkan
// ketika API admin Fase 11 membuat pembatasan ini benar-benar banyak dipakai. Yang tidak
// dilakukan adalah melewatkan pemeriksaannya: key yang dibatasi ke satu provider tidak
// boleh dirutekan ke provider lain hanya karena pemeriksaannya mahal.
func (h *Handlers) kandidat(w http.ResponseWriter, r *http.Request, rreq router.Request, p *apikey.Principal) ([]*upstream.RouteCandidate, bool) {
	cands, err := h.candidates.RouteCandidates(r.Context(), upstream.RouteQuery{
		ModelID:     rreq.ModelID,
		OwnerUserID: rreq.OwnerUserID,
	})
	if err != nil {
		h.logger.ErrorContext(r.Context(), "pengambilan kandidat rute gagal",
			"model", rreq.ModelName, "error", err)
		httpx.InternalError(w, r)
		return nil, false
	}

	periksaKey := h.restrict != nil && p.ID() != ""

	diingat := make(map[string]bool, len(cands))
	out := make([]*upstream.RouteCandidate, 0, len(cands))
	for _, c := range cands {
		boleh, ada := diingat[c.ProviderID]
		if !ada {
			// Pembatasan provider dari penyaring konten MENYARING kandidat, tidak menolak
			// permintaan: selama masih ada kandidat lain, permintaan tetap dilayani lewat
			// kandidat itu. Kalau semuanya tersaring, tolakNyaTanpaKandidat yang menjelaskan
			// sebabnya — jauh lebih bisa ditindaklanjuti daripada 502 dari provider yang
			// sengaja tidak pernah dihubungi.
			boleh = !h.guard.ProviderDilarang(r.Context(), c.ProviderID)
			if boleh && periksaKey {
				var err error
				boleh, err = h.restrict.AllowsProvider(r.Context(), p.ID(), c.ProviderID)
				if err != nil {
					h.logger.ErrorContext(r.Context(), "pemeriksaan pembatasan provider gagal",
						"api_key", p.Masked(), "error", err)
					httpx.InternalError(w, r)
					return nil, false
				}
			}
			diingat[c.ProviderID] = boleh
		}
		if boleh {
			out = append(out, c)
		}
	}
	return out, true
}

// tolakTanpaKandidat menjelaskan MENGAPA tidak ada provider yang bisa melayani.
//
// Pesan tunggal "tidak ada provider" adalah jawaban yang tidak bisa ditindaklanjuti
// siapa pun: operator tidak tahu apakah harus menambah provider, menyalakan streaming pada
// pemetaan model, atau melonggarkan pembatasan API key. Empat sebab yang mungkin dibedakan
// di sini karena tindak lanjutnya berbeda, dan hanya kode ini yang tahu bedanya.
func (h *Handlers) tolakTanpaKandidat(
	w http.ResponseWriter, r *http.Request,
	rreq router.Request, model *upstream.Model, sebelumDisaring []*upstream.RouteCandidate,
) {
	nama := kutip(model.ModelID)

	// Tidak ada pemetaan provider sama sekali, atau semuanya tersaring pembatasan key.
	if len(sebelumDisaring) == 0 {
		h.logger.WarnContext(r.Context(), "tidak ada pemetaan provider untuk model yang aktif",
			"model", model.ModelID, "api_key_dibatasi", h.restrict != nil)
		httpx.WriteError(w, r, http.StatusServiceUnavailable, httpx.ErrTypeAPI, "no_provider_for_model",
			"tidak ada provider yang dikonfigurasi untuk model "+nama)
		return
	}

	// Ada kandidat, tetapi semuanya tersaring syarat kemampuan. Sebabnya bisa disebutkan
	// dengan tepat, dan itu satu-satunya bentuk pesan yang membuat operator bisa
	// memperbaikinya tanpa menebak.
	switch {
	case rreq.Streaming && tidakAdaYangMendukung(sebelumDisaring, func(c *upstream.RouteCandidate) bool { return c.SupportsStreaming }):
		httpx.WriteError(w, r, http.StatusBadRequest, httpx.ErrTypeInvalidRequest, "streaming_not_supported",
			"tidak ada provider untuk model "+nama+" yang mendukung streaming; kirim ulang dengan \"stream\": false")
	case butuhTools(rreq) && tidakAdaYangMendukung(sebelumDisaring, func(c *upstream.RouteCandidate) bool { return c.SupportsTools }):
		httpx.WriteError(w, r, http.StatusBadRequest, httpx.ErrTypeInvalidRequest, "tools_not_supported",
			"tidak ada provider untuk model "+nama+" yang mendukung pemanggilan tool")
	default:
		// Sisanya praktis selalu jendela konteks: permintaan lebih besar daripada yang
		// dideklarasikan pemetaan model mana pun.
		httpx.WriteError(w, r, http.StatusBadRequest, httpx.ErrTypeInvalidRequest, "context_length_exceeded",
			"permintaan melebihi jendela konteks setiap provider yang melayani model "+nama)
	}
}

// tidakAdaYangMendukung melaporkan bahwa tidak satu pun kandidat memenuhi syarat.
func tidakAdaYangMendukung(cands []*upstream.RouteCandidate, syarat func(*upstream.RouteCandidate) bool) bool {
	for _, c := range cands {
		if syarat(c) {
			return false
		}
	}
	return true
}

// butuhTools melaporkan apakah permintaan ini menuntut dukungan tool.
func butuhTools(rreq router.Request) bool {
	for _, c := range rreq.Capabilities {
		if c == upstream.CapTools {
			return true
		}
	}
	return false
}

// kutip membungkus nilai dari pengguna dengan tanda kutip untuk pesan error.
//
// Nilai yang dikutip di sini HANYA nama model, yang memang berasal dari klien tetapi bukan
// data sensitif — dan tanpa mengutipnya, pesan "model  tidak dikenal" pada nama kosong
// menjadi tidak bisa dibaca. Isi pesan pengguna tidak pernah masuk ke pesan error.
func kutip(v string) string {
	const maks = 100
	if len(v) > maks {
		v = v[:maks] + "…"
	}
	return "\"" + v + "\""
}

// untukKandidat menyalin permintaan dengan nama model milik kandidat ini.
//
// Wajib menyalin, bukan mengubah di tempat: satu permintaan bisa dicoba di beberapa
// provider, dan setiap provider punya nama upstream sendiri untuk model kanonik yang sama.
// Mengubah req di tempat membuat percobaan kedua mengirim nama milik provider pertama —
// yang biasanya berakhir sebagai "model tidak ditemukan" di provider yang sebenarnya
// melayaninya, yaitu kegagalan yang tampak seperti salah konfigurasi.
func untukKandidat(req *providers.ChatRequest, c *upstream.RouteCandidate) *providers.ChatRequest {
	salinan := *req
	salinan.Model = c.UpstreamModelName
	return &salinan
}

// ChatCompletions melayani POST /v1/chat/completions dan POST /v1/responses.
func (h *Handlers) ChatCompletions(w http.ResponseWriter, r *http.Request) {
	// Jejak dibuat sebelum apa pun dan diserahkan lewat defer, sehingga SETIAP jalan keluar
	// — termasuk yang belum ada saat baris ini ditulis — menghasilkan satu baris log
	// request. Writer yang dipakai selanjutnya adalah writer dari jejak, karena itulah yang
	// tahu status dan envelope apa yang benar-benar terkirim.
	j, w := h.mulaiJejak(w, r)
	defer j.selesai(r.Context())

	pr, ok := h.siapkanChat(w, r, j)
	if !ok {
		return
	}
	if pr.req.Stream {
		h.chatMengalir(w, r, pr, j)
		return
	}
	h.chatSekali(w, r, pr, j)
}

// ekstrakTier2Model membaca target fallback model dari aturan combo routing.
// Format dalam description: [combo:tier2=<model_name>]
func ekstrakTier2Model(rule *router.Rule) string {
	if rule == nil || rule.Description == "" {
		return ""
	}
	const prefix = "[combo:tier2="
	idx := strings.Index(rule.Description, prefix)
	if idx == -1 {
		return ""
	}
	sub := rule.Description[idx+len(prefix):]
	end := strings.IndexByte(sub, ']')
	if end == -1 {
		return ""
	}
	return strings.TrimSpace(sub[:end])
}

func (h *Handlers) cobaTier2Sekali(ctx context.Context, tier2ModelName string, pr *persiapan, j *jejak) *Outcome[*providers.ChatResponse] {
	t2Model, err := h.models.Resolve(ctx, tier2ModelName)
	if err != nil {
		h.logger.WarnContext(ctx, "resolusi tier2 combo model gagal", "tier2", tier2ModelName, "error", err)
		return nil
	}

	var ownerUserID string
	if pr.principal != nil {
		ownerUserID = pr.principal.OwnerUserID()
	}

	cands, err := h.candidates.RouteCandidates(ctx, upstream.RouteQuery{
		ModelID:     t2Model.ID,
		OwnerUserID: ownerUserID,
	})
	if err != nil || len(cands) == 0 {
		h.logger.WarnContext(ctx, "tidak ada kandidat untuk tier2 combo model", "tier2", tier2ModelName, "error", err)
		return nil
	}

	t2Req := *pr.req
	t2Req.Model = t2Model.ModelID

	t2Plan := Plan{
		Candidates:  cands,
		Model:       t2Model.ModelID,
		MaxAttempts: 2,
		BackoffBase: 250 * time.Millisecond,
	}

	h.logger.InfoContext(ctx, "menjalankan fallback combo routing tier 2", "tier1", pr.model.ModelID, "tier2", t2Model.ModelID)
	out := Execute(ctx, h.exec, t2Plan,
		func(c context.Context, cand *upstream.RouteCandidate) (*providers.ChatResponse, error) {
			if perr := h.guard.PeriksaBatasProvider(c, cand.ProviderID, cand.ProviderName); perr != nil {
				return nil, perr
			}
			p, err := h.adapter(c, cand)
			if err != nil {
				return nil, err
			}
			return p.ChatCompletion(c, untukKandidat(&t2Req, cand))
		})

	j.pasangPercobaan(out.Attempts, out.Candidate)
	if out.Err == nil {
		pr.model = t2Model
		j.pasangModel(t2Model)
		return out
	}
	return nil
}

func (h *Handlers) cobaTier2Mengalir(ctx context.Context, tier2ModelName string, pr *persiapan, j *jejak) *Outcome[providers.Stream] {
	t2Model, err := h.models.Resolve(ctx, tier2ModelName)
	if err != nil {
		h.logger.WarnContext(ctx, "resolusi tier2 combo model gagal", "tier2", tier2ModelName, "error", err)
		return nil
	}

	var ownerUserID string
	if pr.principal != nil {
		ownerUserID = pr.principal.OwnerUserID()
	}

	cands, err := h.candidates.RouteCandidates(ctx, upstream.RouteQuery{
		ModelID:     t2Model.ID,
		OwnerUserID: ownerUserID,
	})
	if err != nil || len(cands) == 0 {
		h.logger.WarnContext(ctx, "tidak ada kandidat untuk tier2 combo model", "tier2", tier2ModelName, "error", err)
		return nil
	}

	t2Req := *pr.req
	t2Req.Model = t2Model.ModelID

	t2Plan := Plan{
		Candidates:  cands,
		Model:       t2Model.ModelID,
		MaxAttempts: 2,
		BackoffBase: 250 * time.Millisecond,
	}

	h.logger.InfoContext(ctx, "menjalankan fallback streaming combo routing tier 2", "tier1", pr.model.ModelID, "tier2", t2Model.ModelID)
	out := ExecuteStream(ctx, h.exec, t2Plan,
		func(c context.Context, cand *upstream.RouteCandidate) (providers.Stream, error) {
			if perr := h.guard.PeriksaBatasProvider(c, cand.ProviderID, cand.ProviderName); perr != nil {
				return nil, perr
			}
			p, err := h.adapter(c, cand)
			if err != nil {
				return nil, err
			}
			return p.ChatCompletionStream(c, untukKandidat(&t2Req, cand))
		})

	j.pasangPercobaan(out.Attempts, out.Candidate)
	if out.Err == nil {
		pr.model = t2Model
		j.pasangModel(t2Model)
		return out
	}
	return nil
}

// chatSekali melayani completion non-streaming.
func (h *Handlers) chatSekali(w http.ResponseWriter, r *http.Request, pr *persiapan, j *jejak) {
	var cacheKey string
	if h.respCache != nil && h.respCache.IsEnabled() {
		cacheKey = h.respCache.ComputeKey(pr.req)
		if entry, hit, err := h.respCache.Get(r.Context(), cacheKey); err == nil && hit && entry != nil && len(entry.Raw) > 0 {
			w.Header().Set("Content-Type", "application/json")
			w.Header().Set("Cache-Control", "no-store")
			w.Header().Set("X-RouteX-Cache", "HIT")
			w.WriteHeader(http.StatusOK)
			_, _ = w.Write(entry.Raw)
			j.pasangPemakaian(entry.Usage)
			return
		}
	}

	out := Execute(r.Context(), h.exec, pr.plan,
		func(ctx context.Context, c *upstream.RouteCandidate) (*providers.ChatResponse, error) {
			if perr := h.guard.PeriksaBatasProvider(ctx, c.ProviderID, c.ProviderName); perr != nil {
				return nil, perr
			}
			p, err := h.adapter(ctx, c)
			if err != nil {
				return nil, err
			}
			return p.ChatCompletion(ctx, untukKandidat(pr.req, c))
		})

	h.catatRute(r, pr, out.Attempts, out.Candidate)
	j.pasangPercobaan(out.Attempts, out.Candidate)

	// Jika Tier 1 gagal dan aturan adalah combo routing, otomatis fallback ke Tier 2 model
	if out.Err != nil {
		if tier2 := ekstrakTier2Model(pr.decision.Rule); tier2 != "" {
			if t2Out := h.cobaTier2Sekali(r.Context(), tier2, pr, j); t2Out != nil {
				out = t2Out
			}
		}
	}

	if out.Err != nil {
		j.pasangKegagalanUpstream(out.Err)
		tulisKegagalan(w, r, out.Err)
		return
	}
	j.pasangPemakaian(out.Value.Usage)

	// Pemakaian token dicatat SEBELUM penyaring jawaban: permintaannya sudah dijalankan dan
	// tokennya sudah ditagihkan upstream, jadi jawaban yang kemudian ditolak penyaring pun
	// tetap harus memakan kuota. Kalau dibalik, klien bisa menghabiskan token tanpa batas
	// dengan mengirim permintaan yang jawabannya selalu tertahan penyaring.
	h.guard.CatatToken(r.Context(), pr.principal, pr.targets, int64(out.Value.Usage.TotalTokens))

	if rj := h.guard.SaringJawaban(r.Context(), teksJawaban(out.Value),
		pr.model.ID, out.Candidate.ProviderID); rj != nil {
		rj.Tulis(w, r)
		return
	}

	// Body upstream diteruskan APA ADANYA.
	//
	// Menyusunnya ulang dari struct kanonik akan menghapus setiap field yang belum dikenal
	// gateway — dan pada API yang bertambah setiap bulan, itu berarti klien kehilangan
	// fitur baru sampai gateway ini diperbarui. Justru itu guna ChatResponse.Raw.
	//
	// Nama model di dalamnya adalah nama UPSTREAM, bukan nama yang diminta klien. Itu
	// disengaja dan sejalan dengan perilaku OpenAI sendiri, yang membalas nama snapshot
	// yang benar-benar melayani. Jejak rutenya masuk ke log di sini, dan ke tabel requests
	// di Fase 9 — bukan ke header respons, supaya tidak ada keputusan pemaparan infrastruktur
	// yang diambil diam-diam di lapisan ini.
	if len(out.Value.Raw) > 0 {
		w.Header().Set("Content-Type", "application/json")
		w.Header().Set("Cache-Control", "no-store")
		w.Header().Set("X-RouteX-Cache", "MISS")
		w.WriteHeader(http.StatusOK)

		if cacheKey != "" && h.respCache != nil && h.respCache.IsEnabled() {
			_ = h.respCache.Set(r.Context(), cacheKey, &responsecache.Entry{
				Model:    out.Candidate.UpstreamModelName,
				Raw:      out.Value.Raw,
				Usage:    out.Value.Usage,
				CachedAt: time.Now(),
			})
		}

		if _, err := w.Write(out.Value.Raw); err != nil {
			h.logger.DebugContext(r.Context(), "penulisan respons gagal, klien kemungkinan sudah menutup koneksi",
				"error", err)
		}
		return
	}

	// Adapter yang berhasil tanpa Raw adalah pelanggaran kontrak providers.Provider, bukan
	// keadaan yang bisa dijawab dengan body kosong.
	h.logger.ErrorContext(r.Context(), "adapter mengembalikan respons tanpa body mentah",
		"provider", namaProvider(out.Candidate), "model", pr.model.ModelID)
	httpx.InternalError(w, r)
}

// adapter membuat provider untuk satu kandidat, dengan kegagalannya sudah terklasifikasi.
//
// Kegagalan penyiapan diklasifikasikan ErrKindAuth, dan itu pilihan yang sadar: kategori
// itu TIDAK boleh diulang (kredensial atau konfigurasi tidak berubah dalam hitungan
// milidetik) tetapi BOLEH dialihkan (kandidat lain punya kredensial dan konfigurasinya
// sendiri). Kedua sifat itu tepat yang dibutuhkan untuk "kandidat ini tidak bisa dipakai
// sekarang". Sebab aslinya — kredensial tidak ada, gagal didekripsi, base URL ditolak —
// hanya masuk log, karena semuanya bisa memuat nilai yang tidak boleh dilihat klien.
func (h *Handlers) adapter(ctx context.Context, c *upstream.RouteCandidate) (providers.Provider, error) {
	p, err := h.factory.Provider(ctx, c)
	if err != nil {
		h.logger.ErrorContext(ctx, "provider tidak bisa disiapkan",
			"provider", c.ProviderName, "kind", c.Kind, "error", err)
		return nil, providers.Newf(providers.ErrKindAuth, c.ProviderName,
			"provider ini tidak bisa disiapkan gateway")
	}
	return p, nil
}

// catatRute mencatat keputusan routing dan jejak percobaannya.
//
// Ini yang menjawab "kenapa permintaan ini pergi ke sana" saat operator membaca log. Fase 9
// menuliskan jejak yang sama ke requests dan request_events untuk dashboard; log tetap
// diperlukan karena ia ada bahkan ketika penulisan ke database itulah yang gagal.
func (h *Handlers) catatRute(r *http.Request, pr *persiapan, att []Attempt, menang *upstream.RouteCandidate) {
	lg := h.logger
	if len(att) <= 1 && menang != nil {
		// Jalur mulus tidak perlu menghabiskan tempat di log tingkat info.
		lg.DebugContext(r.Context(), "permintaan dirutekan",
			"model", pr.model.ModelID, "provider", namaProvider(menang), "aturan", pr.decision.Rule.String())
		return
	}
	lg.InfoContext(r.Context(), "permintaan dirutekan dengan lebih dari satu percobaan",
		"model", pr.model.ModelID,
		"aturan", pr.decision.Rule.String(),
		"kandidat", len(pr.decision.Candidates),
		"percobaan", ringkasPercobaan(att),
		"provider_menang", namaProvider(menang))
}

// ringkasPercobaan meringkas jejak percobaan menjadi satu nilai yang enak dibaca di log.
func ringkasPercobaan(att []Attempt) []string {
	out := make([]string, 0, len(att))
	for _, a := range att {
		switch {
		case a.SkipReason != "":
			out = append(out, a.ProviderName+": dilewati ("+a.SkipReason+")")
		case a.Err != nil:
			out = append(out, a.ProviderName+": "+string(a.Err.Kind))
		default:
			out = append(out, a.ProviderName+": berhasil")
		}
	}
	return out
}

// namaProvider aman dipakai pada kandidat nil.
func namaProvider(c *upstream.RouteCandidate) string {
	if c == nil {
		return ""
	}
	return c.ProviderName
}

// Framing SSE. Dibuat di sini, bukan di adapter — lihat catatan providers.StreamEvent.Raw.
var (
	sseAwalan  = []byte("data: ")
	ssePemisah = []byte("\n\n")
	sseSelesai = []byte("data: [DONE]\n\n")
)

// chatMengalir melayani completion streaming.
//
// Urutannya adalah keseluruhan isi fungsi ini: aliran upstream DIBUKA lebih dulu, dan
// PrepareSSE baru dipanggil setelah pembukaan itu berhasil. Kalau dibalik, penolakan
// upstream — 401, 429, model tidak ada — akan sampai ke klien sebagai 200 disusul peristiwa
// error di dalam stream, dan klien OpenAI mana pun akan menganggap itu percakapan yang
// berhenti tanpa sebab. Harganya: klien menunggu tanpa header sampai upstream menjawab,
// yang persis perilaku OpenAI sendiri.
func (h *Handlers) chatMengalir(w http.ResponseWriter, r *http.Request, pr *persiapan, j *jejak) {
	var cacheKey string
	if h.respCache != nil && h.respCache.IsEnabled() {
		cacheKey = h.respCache.ComputeKey(pr.req)
		if entry, hit, err := h.respCache.Get(r.Context(), cacheKey); err == nil && hit && entry != nil && len(entry.StreamChunks) > 0 {
			if err := httpx.PrepareSSE(w, r); err != nil {
				h.logger.ErrorContext(r.Context(), "penyiapan SSE gagal", "error", err)
				httpx.InternalError(w, r)
				return
			}
			rc := http.NewResponseController(w)
			for _, chunk := range entry.StreamChunks {
				if _, err := tulisPeristiwa(w, []byte(chunk)); err != nil {
					return
				}
				if err := rc.Flush(); err != nil {
					return
				}
			}
			h.tutupAliran(r, w, rc)
			j.pasangPemakaian(entry.Usage)
			return
		}
	}

	out := ExecuteStream(r.Context(), h.exec, pr.plan,
		func(ctx context.Context, c *upstream.RouteCandidate) (providers.Stream, error) {
			if perr := h.guard.PeriksaBatasProvider(ctx, c.ProviderID, c.ProviderName); perr != nil {
				return nil, perr
			}
			p, err := h.adapter(ctx, c)
			if err != nil {
				return nil, err
			}
			return p.ChatCompletionStream(ctx, untukKandidat(pr.req, c))
		})

	h.catatRute(r, pr, out.Attempts, out.Candidate)
	j.pasangPercobaan(out.Attempts, out.Candidate)

	// Jika Tier 1 gagal dan aturan adalah combo routing, otomatis fallback ke Tier 2 model
	if out.Err != nil {
		if tier2 := ekstrakTier2Model(pr.decision.Rule); tier2 != "" {
			if t2Out := h.cobaTier2Mengalir(r.Context(), tier2, pr, j); t2Out != nil {
				out = t2Out
			}
		}
	}

	if out.Err != nil {
		j.pasangKegagalanUpstream(out.Err)
		tulisKegagalan(w, r, out.Err)
		return
	}
	// Waktu hulu satu aliran adalah waktu MEMBUKA ditambah waktu membaca, dan hanya yang
	// pertama tercatat sebagai durasi percobaan. Sisanya ditambahkan lewat defer supaya ia
	// tetap terhitung ketika aliran berakhir karena klien menutup koneksi.
	mulaiAliran := time.Now()
	defer func() { j.tambahHulu(time.Since(mulaiAliran)) }()

	// Close melepas context percobaan sekaligus koneksi upstream. Tanpa ini koneksi
	// menggantung sampai tenggat permintaan, dan pada klien yang berhenti membaca lebih awal
	// — kejadian yang normal di gateway — itu berarti satu koneksi upstream tersangkut untuk
	// setiap permintaan yang dibatalkan.
	defer func() { _ = out.Value.Close() }()

	// Titik tanpa jalan kembali: setelah ini status HTTP terkunci di 200.
	if err := httpx.PrepareSSE(w, r); err != nil {
		h.logger.ErrorContext(r.Context(), "penyiapan SSE gagal", "error", err)
		httpx.InternalError(w, r)
		return
	}

	rc := http.NewResponseController(w)
	var (
		terkirim     int64
		usage        providers.Usage
		streamChunks []string
	)
	// Pencatatan token dijalankan lewat defer, bukan hanya di jalur selesai-normal: aliran
	// bisa berakhir karena klien menutup koneksi atau upstream terputus di tengah, dan token
	// yang sudah dihasilkan sampai titik itu tetap ditagihkan provider. Melewatkannya berarti
	// klien yang selalu memutus aliran lebih awal tidak pernah memakan kuota tokennya.
	defer func() {
		h.guard.CatatToken(r.Context(), pr.principal, pr.targets, int64(usage.TotalTokens))
		// Token juga diserahkan ke pencatat lewat defer yang sama, dan alasannya sama:
		// aliran bisa berakhir karena klien menutup koneksi atau upstream terputus di
		// tengah, dan token yang sudah dihasilkan sampai titik itu tetap ditagihkan
		// provider. Tanpa ini, setiap aliran yang diputus lebih awal tercatat tanpa biaya.
		j.pasangPemakaian(usage)
	}()

	for {
		ev, err := out.Value.Recv()
		if err != nil {
			if errors.Is(err, io.EOF) {
				if cacheKey != "" && h.respCache != nil && h.respCache.IsEnabled() && len(streamChunks) > 0 {
					_ = h.respCache.Set(r.Context(), cacheKey, &responsecache.Entry{
						Model:        out.Candidate.UpstreamModelName,
						Usage:        usage,
						StreamChunks: streamChunks,
						CachedAt:     time.Now(),
					})
				}
				h.tutupAliran(r, w, rc)
				return
			}
			// Kegagalan setelah header terkirim hanya bisa dilaporkan di dalam stream.
			h.kegagalanDalamAliran(r, w, rc, err, terkirim, j)
			return
		}
		if ev.Usage != nil {
			// Chunk penutup membawa jumlah token final. Ditimpa, bukan dijumlahkan: provider
			// mengirim total, dan menjumlahkan beberapa laporan total akan menagih berkali-kali.
			usage = *ev.Usage
		}
		// Peristiwa tanpa muatan tidak punya padanan yang perlu diteruskan ke klien —
		// mis. peristiwa pembuka Anthropic yang hanya membawa metadata. Diteruskan sebagai
		// "data: \n\n" ia akan diurai klien sebagai chunk kosong yang tidak sah.
		if len(ev.Raw) == 0 {
			continue
		}

		streamChunks = append(streamChunks, string(ev.Raw))

		n, err := tulisPeristiwa(w, ev.Raw)
		terkirim += n
		if err != nil {
			// Klien berhenti membaca. Tidak ada yang bisa dilaporkan kepadanya, dan
			// upstream ditutup oleh defer di atas.
			h.logger.DebugContext(r.Context(), "penulisan peristiwa SSE gagal, klien menutup koneksi",
				"terkirim_byte", terkirim, "error", err)
			return
		}
		if err := rc.Flush(); err != nil {
			h.logger.DebugContext(r.Context(), "flush SSE gagal", "error", err)
			return
		}
		// TTFT ditandai SETELAH flush berhasil, dan hanya untuk peristiwa yang benar-benar
		// membawa konten. Lihat jejak.tandaiTTFT untuk kedua alasannya.
		if adaKonten(ev) {
			j.tandaiTTFT()
		}
	}
}

// adaKonten melaporkan apakah satu peristiwa membawa keluaran model.
//
// Peristiwa pembuka aliran berdialek OpenAI hanya membawa peran, dan peristiwa penutup
// hanya membawa alasan berhenti beserta usage. Keduanya bukan token, jadi keduanya tidak
// boleh menjadi penanda "token pertama sudah sampai".
func adaKonten(ev *providers.StreamEvent) bool {
	return ev.Delta != "" || ev.ReasoningDelta != "" || len(ev.ToolCalls) > 0
}

// tulisPeristiwa menulis satu peristiwa SSE dan mengembalikan jumlah byte muatannya.
//
// Tiga penulisan terpisah, bukan satu concat: muatan chunk bisa besar, dan menyalinnya ke
// buffer baru hanya untuk menempelkan enam byte awalan berarti satu alokasi seukuran chunk
// untuk setiap potongan token yang diteruskan.
func tulisPeristiwa(w http.ResponseWriter, raw []byte) (int64, error) {
	if _, err := w.Write(sseAwalan); err != nil {
		return 0, err
	}
	n, err := w.Write(raw)
	if err != nil {
		return int64(n), err
	}
	if _, err := w.Write(ssePemisah); err != nil {
		return int64(n), err
	}
	return int64(n), nil
}

// tutupAliran mengirim penanda penutup yang diharapkan klien OpenAI.
//
// "data: [DONE]" bukan hiasan: klien resmi OpenAI memakainya untuk membedakan aliran yang
// selesai dari koneksi yang terputus. Tanpa penanda ini, setiap completion yang normal akan
// tampak seperti kegagalan jaringan bagi klien yang memeriksanya.
func (h *Handlers) tutupAliran(r *http.Request, w http.ResponseWriter, rc *http.ResponseController) {
	if _, err := w.Write(sseSelesai); err != nil {
		h.logger.DebugContext(r.Context(), "penanda penutup SSE gagal ditulis", "error", err)
		return
	}
	if err := rc.Flush(); err != nil {
		h.logger.DebugContext(r.Context(), "flush penanda penutup gagal", "error", err)
	}
}

// kegagalanDalamAliran melaporkan kegagalan yang terjadi setelah header terkirim.
//
// Bentuknya satu peristiwa SSE bermuatan envelope error yang sama seperti jalur
// non-streaming, lalu penanda penutup. Envelope yang sama disengaja: klien yang sudah punya
// kode untuk membaca error dari body 4xx bisa memakai kode yang sama di sini, dan tidak ada
// bentuk error kedua yang harus dipelajari.
//
// Penanda penutup tetap dikirim setelahnya. Tanpa itu klien menunggu sampai timeout-nya
// sendiri untuk aliran yang jelas-jelas sudah berakhir.
func (h *Handlers) kegagalanDalamAliran(
	r *http.Request, w http.ResponseWriter, rc *http.ResponseController, err error, terkirim int64, j *jejak,
) {
	perr := jadikanProviderError("", err)
	// StreamedBytes dipasang di sini supaya jelas di log bahwa kegagalan ini TIDAK bisa
	// dialihkan — dan supaya pencatatan pemakaian di Fase 9 tahu sebagian respons sudah
	// terkirim, sehingga token yang sudah dihasilkan tetap ditagihkan.
	if perr.StreamedBytes == 0 {
		perr.StreamedBytes = terkirim
	}

	// Kategori kegagalan dicatat walaupun status yang diterima klien tetap 200: klien sudah
	// menerima sebagian jawaban, jadi 200 adalah status yang benar untuk dilaporkan, dan
	// kategori inilah satu-satunya cara aliran yang putus di tengah tetap bisa dihitung
	// sebagai kegagalan. Agregasi karena itu memakai "error_type is not null", bukan hanya
	// "status >= 400".
	j.pasangKegagalanUpstream(perr)

	_, errType, code, message := httpUntuk(perr)
	h.logger.WarnContext(r.Context(), "aliran terputus setelah sebagian terkirim",
		"kind", perr.Kind, "terkirim_byte", terkirim, "error", err)

	muatan, jerr := errorEventJSON(r, errType, code, message)
	if jerr != nil {
		// Tidak bisa menyusun peristiwa error pun: yang tersisa hanya menutup aliran,
		// supaya klien tidak menunggu tanpa akhir.
		h.logger.ErrorContext(r.Context(), "peristiwa error SSE gagal disusun", "error", jerr)
		h.tutupAliran(r, w, rc)
		return
	}
	if _, werr := tulisPeristiwa(w, muatan); werr != nil {
		return
	}
	h.tutupAliran(r, w, rc)
}

// errorEventJSON menyusun muatan peristiwa error dalam envelope error OpenAI.
func errorEventJSON(r *http.Request, errType, code, message string) ([]byte, error) {
	detail := httpx.ErrorDetail{Message: message, Type: errType, RequestID: requestID(r)}
	if code != "" {
		detail.Code = &code
	}
	return json.Marshal(httpx.ErrorResponse{Error: detail})
}

// requestID mengambil ID request untuk envelope error.
func requestID(r *http.Request) string {
	if r == nil {
		return ""
	}
	return observability.RequestIDFrom(r.Context())
}

// Embeddings melayani POST /v1/embeddings.
//
// Alurnya sama dengan completion non-streaming, dengan satu perbedaan yang bukan kosmetik:
// permintaan embedding menuntut kemampuan `embeddings`, dan model chat tidak punya itu.
// Tanpa syarat itu, permintaan embedding akan dirutekan ke model chat mana pun yang cocok
// dan gagal di upstream dengan pesan yang tidak menjelaskan apa pun.
func (h *Handlers) Embeddings(w http.ResponseWriter, r *http.Request) {
	j, w := h.mulaiJejak(w, r)
	defer j.selesai(r.Context())

	body, ok := h.bacaBody(w, r)
	if !ok {
		return
	}

	req, err := DecodeEmbeddingsRequest(body)
	if err != nil {
		httpx.BadRequest(w, r, httpx.CodeInvalidRequest, err.Error())
		return
	}
	j.pasangDiminta(req.Model)

	// Penyaring konten atas masukan embedding. Sama seperti jalur chat, dijalankan sebelum
	// resolusi model supaya permintaan yang jelas ditolak tidak menyentuh database.
	if rj := h.guard.SaringPermintaan(r.Context(), contentfilter.Subject{
		Text:  strings.Join(req.Input, "\n"),
		Bytes: int64(len(body)),
	}); rj != nil {
		rj.Tulis(w, r)
		return
	}

	model, ok := h.selesaikanModel(w, r, req.Model)
	if !ok {
		return
	}
	j.pasangModel(model)

	p, _ := apikey.PrincipalFrom(r.Context())
	if !h.izinModel(w, r, p, model) {
		return
	}

	clientIP, _ := httpx.ClientIPFrom(r.Context())
	targets := TargetsFor(p, model.ID, clientIP)
	j.pasangKebijakan(p, targets)
	if !h.kebijakan(w, r, p, model, targets) {
		return
	}

	rreq := router.Request{
		ModelID:      model.ID,
		ModelName:    req.Model,
		APIKeyID:     p.ID(),
		OwnerUserID:  p.OwnerUserID(),
		Capabilities: []string{upstream.CapEmbeddings},
	}
	cands, ok := h.kandidat(w, r, rreq, p)
	if !ok {
		return
	}

	keputusan := h.engine.Route(r.Context(), rreq, cands)
	j.pasangKeputusan(keputusan, false)
	if len(keputusan.Candidates) == 0 {
		httpx.WriteError(w, r, http.StatusServiceUnavailable, httpx.ErrTypeAPI, "no_provider_for_model",
			"tidak ada provider yang dikonfigurasi untuk model embedding "+kutip(model.ModelID))
		return
	}
	plan := PlanFromRule(keputusan.Rule, model.ModelID, keputusan.Candidates)

	out := Execute(r.Context(), h.exec, plan,
		func(ctx context.Context, c *upstream.RouteCandidate) (*providers.EmbeddingsResponse, error) {
			if perr := h.guard.PeriksaBatasProvider(ctx, c.ProviderID, c.ProviderName); perr != nil {
				return nil, perr
			}
			prov, err := h.adapter(ctx, c)
			if err != nil {
				return nil, err
			}
			salinan := *req
			salinan.Model = c.UpstreamModelName
			return prov.Embeddings(ctx, &salinan)
		})

	j.pasangPercobaan(out.Attempts, out.Candidate)
	if out.Err != nil {
		j.pasangKegagalanUpstream(out.Err)
		tulisKegagalan(w, r, out.Err)
		return
	}
	j.pasangPemakaian(out.Value.Usage)
	h.guard.CatatToken(r.Context(), p, targets, int64(out.Value.Usage.TotalTokens))
	if len(out.Value.Raw) == 0 {
		h.logger.ErrorContext(r.Context(), "adapter embedding mengembalikan respons tanpa body mentah",
			"provider", namaProvider(out.Candidate))
		httpx.InternalError(w, r)
		return
	}
	w.Header().Set("Content-Type", "application/json")
	w.Header().Set("Cache-Control", "no-store")
	w.WriteHeader(http.StatusOK)
	if _, err := w.Write(out.Value.Raw); err != nil {
		h.logger.DebugContext(r.Context(), "penulisan respons embedding gagal", "error", err)
	}
}

// modelEntry adalah satu entri GET /v1/models dalam bentuk yang diharapkan klien OpenAI.
type modelEntry struct {
	ID      string `json:"id"`
	Object  string `json:"object"`
	Created int64  `json:"created"`
	OwnedBy string `json:"owned_by"`
}

// modelList adalah envelope daftar model.
type modelList struct {
	Object string       `json:"object"`
	Data   []modelEntry `json:"data"`
}

// maxModelsPerPage membatasi satu halaman daftar model.
const maxModelsPerPage = 500

// Models melayani GET /v1/models.
//
// Yang didaftar adalah model yang dilayani GATEWAY INI, dari registry-nya sendiri — bukan
// hasil meneruskan /v1/models ke provider. Bedanya penting: daftar dari provider memuat
// model yang tidak punya pemetaan di sini dan tidak menyebut alias yang justru ditambahkan
// operator, sehingga klien akan meminta nama yang gagal dan tidak menemukan nama yang
// berhasil. Daftar ini juga menghormati pembatasan model pada API key pemanggil.
func (h *Handlers) Models(w http.ResponseWriter, r *http.Request) {
	if h.lister == nil {
		// Daftar tidak bisa disusun tanpa sumbernya, dan membalas daftar KOSONG akan
		// membuat klien menyimpulkan gateway ini tidak melayani model apa pun.
		h.logger.ErrorContext(r.Context(), "GET /v1/models dipanggil tanpa ModelLister terpasang")
		httpx.InternalError(w, r)
		return
	}

	aktif := true
	models, _, err := h.lister.List(r.Context(),
		upstream.ModelFilter{Enabled: &aktif},
		repo.Page{Limit: maxModelsPerPage})
	if err != nil {
		h.logger.ErrorContext(r.Context(), "pengambilan daftar model gagal", "error", err)
		httpx.InternalError(w, r)
		return
	}

	p, _ := apikey.PrincipalFrom(r.Context())
	data := make([]modelEntry, 0, len(models))
	for _, m := range models {
		boleh, err := h.bolehLihatModel(r.Context(), p, m)
		if err != nil {
			h.logger.ErrorContext(r.Context(), "pemeriksaan pembatasan model gagal saat mendaftar",
				"api_key", p.Masked(), "model", m.ModelID, "error", err)
			httpx.InternalError(w, r)
			return
		}
		if !boleh {
			continue
		}
		data = append(data, modelEntry{
			ID:     m.ModelID,
			Object: "model",
			// Unix detik, mengikuti bentuk OpenAI. CreatedAt kami bertipe timestamptz;
			// klien yang mengubahnya menjadi tanggal mengharapkan detik, bukan milidetik.
			Created: m.CreatedAt.Unix(),
			// OwnedBy diisi keluarga model bila ada. Tanpa itu, "routex" lebih jujur
			// daripada menebak nama vendor dari nama model — tebakan seperti itu akan salah
			// tepat pada model dari penyedia yang jarang, yaitu yang paling perlu dikenali.
			OwnedBy: pemilikModel(m),
		})
	}

	if err := httpx.JSON(w, http.StatusOK, modelList{Object: "list", Data: data}); err != nil {
		h.logger.DebugContext(r.Context(), "penulisan daftar model gagal", "error", err)
	}
}

// bolehLihatModel melaporkan apakah pemanggil boleh memakai model ini.
//
// Daftar yang memuat model yang akan ditolak 403 saat dipakai adalah daftar yang menyesatkan:
// klien memilih dari daftar, jadi apa pun di dalamnya adalah janji bahwa ia bisa dipakai.
func (h *Handlers) bolehLihatModel(ctx context.Context, p *apikey.Principal, m *upstream.Model) (bool, error) {
	if h.restrict == nil || p.ID() == "" {
		return true, nil
	}
	return h.restrict.AllowsModel(ctx, p.ID(), m.ID)
}

// pemilikModel mengembalikan nilai owned_by untuk satu model.
func pemilikModel(m *upstream.Model) string {
	if m.Family != nil && *m.Family != "" {
		return *m.Family
	}
	return "routex"
}
