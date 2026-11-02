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
	"bytes"
	"compress/gzip"
	"context"
	"encoding/json"
	"errors"
	"io"
	"log/slog"
	"mime"
	"net/http"
	"slices"
	"sort"
	"strconv"
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
	"github.com/NexGen-X/Route-X/internal/webhooks"
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
	webhooks   webhooks.EnqueueSink
	respCache  *responsecache.Engine
	penaksir   router.CostSource
	// pembatasLaju adalah middleware pembatas laju inferensi; lihat
	// HandlersDeps.PembatasLaju soal mengapa ia hanya membungkus rute inferensi.
	pembatasLaju func(http.Handler) http.Handler
	logger       *slog.Logger
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
	// Penaksir adalah sumber harga untuk estimasi biaya pra-permintaan, dipakai
	// reservasi anggaran (audit3 V-AB-16). Boleh nil: tanpa penaksir, anggaran
	// ditegakkan tanpa reservasi (perilaku lama).
	Penaksir router.CostSource
	// Guard menegakkan penyaring konten, batas laju cakupan model dan provider, serta
	// anggaran biaya. nil berarti tidak satu pun kebijakan itu ditegakkan.
	Guard *Guard
	// Usage menerima hasil setiap permintaan: token, biaya, latensi, dan jejak
	// percobaannya. nil berarti TIDAK ADA baris log request yang ditulis dan tidak ada
	// pemakaian anggaran yang naik — sah untuk test, tetapi di produksi berarti halaman
	// Requests kosong dan setiap anggaran menegakkan angka yang tidak bergerak.
	Usage UsageSink
	// Webhooks adalah produser event request.completed dan request.failed,
	// dipancarkan dari jejak selesai setiap permintaan. nil berarti kedua
	// event itu tidak diproduksi (audit3 V-WH-08).
	Webhooks webhooks.EnqueueSink
	// PembatasLaju adalah middleware pembatas laju inferensi (apikey.Limiter.Limit
	// di produksi). Ia dipasang DI DALAM Routes, hanya pada rute inferensi — bukan
	// pada seluruh permukaan seperti sebelumnya. Alasannya terbukti di audit3
	// V-GW-38/V-EM-06: pembatas yang membungkus seluruh router ikut memakan kuota
	// untuk polling GET /v1/models (klien seperti Open WebUI melakukannya berkala)
	// sampai jatah chat ikut habis, dan membakar kuota untuk path yang bahkan
	// tidak dikenal (404). Katalog model adalah bacaan murah yang tidak menyentuh
	// upstream; yang dilindungi kuota adalah inferensinya. nil berarti tanpa
	// pembatas (test yang menyuntik principal langsung ke context).
	PembatasLaju func(http.Handler) http.Handler
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
		webhooks:  d.Webhooks,
		respCache: d.ResponseCache, penaksir: d.Penaksir,
		pembatasLaju: d.PembatasLaju, logger: logger,
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
	// CORS diizinkan untuk rute API /v1 agar aplikasi frontend eksternal (Open WebUI, LibreChat, dsb.)
	// dapat melakukan panggilan inferensi langsung tanpa terblokir preflight OPTIONS oleh browser.
	r.Use(httpx.CORS(nil))
	// Metode yang salah pada rute yang ada wajib menjawab envelope error, bukan
	// body kosong bawaan chi (audit3 V-EM-06).
	r.MethodNotAllowed(httpx.MethodNotAllowedHandler)

	// Rute inferensi dibungkus pembatas laju; katalog model di luar grup ini.
	// Lihat HandlersDeps.PembatasLaju untuk alasan pemisahannya (audit3 V-GW-38).
	r.Group(func(r chi.Router) {
		if h.pembatasLaju != nil {
			r.Use(h.pembatasLaju)
		}
		r.Post("/chat/completions", h.ChatCompletions)
		// /v1/responses adalah nama baru OpenAI untuk permukaan yang sama pada pemakaian
		// percakapan biasa. Dilayani handler yang sama supaya klien yang sudah pindah ke nama
		// itu tetap bekerja; perbedaan bentuk body-nya yang belum didukung diteruskan lewat
		// ChatRequest.Extra, bukan ditelan diam-diam.
		r.Post("/responses", h.ChatCompletions)
		// /v1/messages melayani Anthropic Messages API (Claude Code, dsb.)
		r.Post("/messages", h.AnthropicMessages)
		r.Post("/v1/messages", h.AnthropicMessages)
		// count_tokens adalah pendamping wajib permukaan messages pada kontrak
		// Anthropic; klien resmi memanggilnya sebelum mengirim percakapan besar.
		r.Post("/messages/count_tokens", h.AnthropicCountTokens)
		r.Post("/v1/messages/count_tokens", h.AnthropicCountTokens)
		r.Post("/embeddings", h.Embeddings)
	})
	r.Get("/models", h.Models)
	r.Get("/models/{model}", h.ModelDetail)
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
	return h.siapkanChatParsed(w, r, req, body, j)
}

func (h *Handlers) siapkanChatParsed(w http.ResponseWriter, r *http.Request, req *providers.ChatRequest, body []byte, j *jejak) (*persiapan, bool) {
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
	if !h.kebijakan(w, r, p, model, targets, h.estimasiBiaya(r.Context(), model, req)) {
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

	cands, dilarangKey, ok := h.kandidat(w, r, rreq, p)
	if !ok {
		return nil, false
	}

	keputusan := h.engine.Route(r.Context(), rreq, cands)
	j.pasangKeputusan(keputusan, req.Stream)

	// Combo pipeline (jalur baru): kandidat adalah GABUNGAN provider dari seluruh model
	// resep, terurut per pipeline.strategy, dieksekusi dalam satu loop dengan anggaran
	// total (Plan.Budget). Tidak ada cascade per model — seluruh model sudah berada di
	// satu daftar, dan urutannya ditentukan oleh resep, bukan oleh tier.
	//
	// targets dan model yang dipakai pencatatan tetap milik model yang DIMINTA klien:
	// pengguna memanggil satu model virtual, dan anggaran, penyaring jawaban, serta
	// penggunaan token tercatat untuk model itu — sama seperti klien OpenAI yang meminta
	// "gpt-4o" tetap ditagih sebagai "gpt-4o" meski dilayani snapshot tertentu.
	if keputusan.Rule.Combo() {
		tempPr := &persiapan{req: req, model: model, decision: keputusan, principal: p}
		if gabungan, ok := h.kandidatCombo(r.Context(), tempPr, rreq); ok {
			return &persiapan{
				req: req, model: model, decision: keputusan,
				principal: p, targets: targets,
				plan: PlanFromRule(keputusan.Rule, model.ModelID, gabungan),
			}, true
		}
		// Resep combo tidak menghasilkan kandidat: jatuh ke daftar kandidat model yang
		// diminta. Masih ada di keputusan.Candidates, dan kalau daftar itu juga kosong,
		// tolakTanpaKandidat di bawah yang menjelaskan sebabnya.
	}

	// Opsi C (Smart Context Bypass):
	// Jika kandidat Tier 1 kosong ATAU ukuran prompt melebihi seluruh kandidat Tier 1,
	// dan aturan memiliki pipeline combo fallback: periksa apakah ada tier lanjutan yang sanggup melayani!
	// Jalur ini memakai tag [combo:...] di description (kompatibilitas rule produksi lama
	// yang pipeline jsonb-nya masih NULL).
	kandidatTier1Kekecilan := len(keputusan.Candidates) > 0 &&
		rreq.Tokens.InputTokens > 0 &&
		semuaKandidatKekecilan(keputusan.Candidates, rreq.Tokens.InputTokens)

	if len(keputusan.Candidates) == 0 || kandidatTier1Kekecilan {
		if pipeline := ekstrakComboPipeline(keputusan.Rule); len(pipeline) > 0 {
			tempPr := &persiapan{req: req, model: model, principal: p}
			for _, tier := range pipeline {
				tModel, tCands, tTargets, rj := h.siapkanTierFallback(r.Context(), tier.Model, tier.ProviderIDs, tempPr, req.Stream)
				if rj == nil && tModel != nil && len(tCands) > 0 {
					if rreq.Tokens.InputTokens > 0 && semuaKandidatKekecilan(tCands, rreq.Tokens.InputTokens) {
						continue // Tier ini juga kekecilan, coba tier berikutnya
					}
					h.logger.InfoContext(r.Context(), "melewati Tier 1 combo karena context window tidak mencukupi, langsung ke fallback tier",
						"tier", tier.Tier, "tier1_model", model.ModelID, "fallback_model", tModel.ModelID, "tokens", rreq.Tokens.InputTokens)
					reqCopy := *req
					reqCopy.Model = tModel.ModelID
					j.pasangModel(tModel)
					j.pasangKebijakan(p, tTargets)
					return &persiapan{
						req: &reqCopy, model: tModel, decision: keputusan,
						principal: p, targets: tTargets,
						plan: PlanFromRule(keputusan.Rule, tModel.ModelID, tCands),
					}, true
				}
			}
		}
	}

	if len(keputusan.Candidates) == 0 {
		h.tolakTanpaKandidat(w, r, rreq, model, cands, dilarangKey)
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

// estimasiBiaya menaksir biaya permintaan ini dari harga pemetaan TERMAHAL
// modelnya, untuk reservasi anggaran (audit3 V-AB-16).
//
// Termahal, bukan yang akan menang: provider pemenang belum diketahui pada
// titik ini, dan reservasi yang terlalu kecil membuka kembali celah
// pelampauan, sementara reservasi yang terlalu besar hanya menolak lebih
// awal di dekat pagu. Nol berarti estimasi tidak bisa disusun — penaksir
// tidak terpasang, pemetaan tidak bisa didaftar, atau belum satu pun
// pemetaan dihargai — dan pemanggil jatuh ke pemeriksaan tanpa reservasi.
func (h *Handlers) estimasiBiaya(ctx context.Context, model *upstream.Model, req *providers.ChatRequest) upstream.USD {
	if h.penaksir == nil || model == nil {
		return 0
	}
	lister, ok := h.models.(interface {
		ListProviderModels(context.Context, upstream.ProviderModelFilter) ([]*upstream.ProviderModel, error)
	})
	if !ok {
		return 0
	}
	pemetaan, err := lister.ListProviderModels(ctx, upstream.ProviderModelFilter{ModelID: model.ID})
	if err != nil {
		h.logger.DebugContext(ctx, "estimasi biaya tidak bisa disusun, anggaran tanpa reservasi",
			"model", model.ModelID, "error", err)
		return 0
	}
	est := EstimateTokens(req)
	var maks upstream.USD
	for _, pm := range pemetaan {
		if biaya, ok := h.penaksir.Cost(pm.ID, est); ok && biaya > maks {
			maks = biaya
		}
	}
	return maks
}

// kebijakan menjalankan pemeriksaan yang menuntut model sudah diketahui: pembatasan model,
// batas laju cakupan model, anggaran biaya, dan gerbang anggaran token.
//
// Urutannya dari yang paling murah: pembatasan model dijawab dari memori, batas laju dan
// gerbang token satu perjalanan ke Redis, anggaran satu pembacaan dari salinan ber-TTL.
//
// estimasi adalah taksiran biaya permintaan ini untuk reservasi anggaran;
// nol berarti tanpa reservasi (pemeriksaan biasa).
//
// ok false berarti jawaban sudah ditulis.
func (h *Handlers) kebijakan(
	w http.ResponseWriter, r *http.Request,
	p *apikey.Principal, model *upstream.Model, targets []policy.Target, estimasi upstream.USD,
) bool {
	if rj := h.guard.SaringPermintaan(r.Context(), contentfilter.Subject{ModelID: model.ID}); rj != nil {
		rj.Tulis(w, r)
		return false
	}
	if rj := h.guard.PeriksaBatasModel(r.Context(), model.ModelID); rj != nil {
		rj.Tulis(w, r)
		return false
	}
	if rj := h.guard.PeriksaAnggaranDenganReservasi(r.Context(), targets, estimasi); rj != nil {
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
// Cakupannya adalah semua yang DITERUSKAN ke upstream sebagai teks bermakna:
// isi pesan, argumen tool_calls pada riwayat, URL pada bagian non-teks, dan
// definisi tools (nama, deskripsi, skema parameter). Versi lama hanya membaca
// Message.Text(), sehingga kata terlarang yang bersembunyi di argumen
// tool_call atau deskripsi tool lolos dari penyaring sepenuhnya walau payload
// yang sama persis diteruskan ke upstream (audit3 V-GW-49).
//
// Yang tetap TIDAK ikut adalah muatan biner mentah: data URI gambar dan audio
// base64 bukan teks yang bermakna bagi pola konten, dan menyertakannya berarti
// setiap aturan pola dijalankan atas megabyte base64 pada setiap permintaan
// multimodal. URL gambar ikut karena ia teks yang dipilih peminta dan dibaca
// upstream; datanya tidak.
func teksPermintaan(req *providers.ChatRequest) string {
	if req == nil {
		return ""
	}
	var b strings.Builder
	tambah := func(s string) {
		if s == "" {
			return
		}
		if b.Len() > 0 {
			b.WriteByte('\n')
		}
		b.WriteString(s)
	}

	for i := range req.Messages {
		m := &req.Messages[i]
		tambah(m.Text())
		// Argumen tool_call adalah teks JSON yang disusun model/peminta dan
		// dikirim apa adanya ke upstream; nama fungsinya ikut diperiksa.
		for _, tc := range m.ToolCalls {
			tambah(tc.Function.Name)
			tambah(tc.Function.Arguments)
		}
		// Bagian non-teks: yang bermakna bagi pola hanyalah URL-nya.
		for _, p := range m.Parts {
			if p.ImageURL != nil {
				tambah(p.ImageURL.URL)
			}
		}
	}

	// Definisi tools adalah teks yang ditulis peminta dan dibaca model;
	// deskripsi dan skema parameternya permukaan suntikan yang sama seperti
	// isi pesan.
	for _, t := range req.Tools {
		tambah(t.Function.Name)
		tambah(t.Function.Description)
		if len(t.Function.Parameters) > 0 {
			tambah(string(t.Function.Parameters))
		}
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

// teksDariKontenWire mengekstrak teks dari satu field "content" bentuk wire
// OpenAI: string polos, atau array bagian yang potongan teksnya digabung.
// Bentuk lain (null, angka, bagian non-teks) menghasilkan "".
func teksDariKontenWire(konten json.RawMessage) string {
	if len(konten) == 0 {
		return ""
	}
	var tunggal string
	if err := json.Unmarshal(konten, &tunggal); err == nil {
		return tunggal
	}
	var bagian []struct {
		Type string `json:"type"`
		Text string `json:"text"`
	}
	if err := json.Unmarshal(konten, &bagian); err != nil {
		return ""
	}
	var b strings.Builder
	for _, p := range bagian {
		if p.Type == "text" {
			b.WriteString(p.Text)
		}
	}
	return b.String()
}

// teksDariRawJawaban mengekstrak teks jawaban dari body wire yang tersimpan
// di entri cache (Entry.Raw selalu bentuk OpenAI — dialek lain diterjemahkan
// adapter ke bentuk itu, lihat ChatResponse.Raw).
//
// Dipakai jalur HIT untuk entri lama yang belum punya Entry.Teks: tanpa
// ekstraksi ini entri semacam itu melewati SaringJawaban sepenuhnya
// (audit3 V-GW-06). Ekstraksi yang gagal mengembalikan "" — dan teks kosong
// memang tidak punya apa-apa untuk disaring, sama seperti jalur MISS.
func teksDariRawJawaban(raw json.RawMessage) string {
	if len(raw) == 0 {
		return ""
	}
	var wire struct {
		Choices []struct {
			Message struct {
				Content json.RawMessage `json:"content"`
			} `json:"message"`
		} `json:"choices"`
	}
	if err := json.Unmarshal(raw, &wire); err != nil {
		return ""
	}
	var b strings.Builder
	for _, c := range wire.Choices {
		t := teksDariKontenWire(c.Message.Content)
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

// teksDariChunkAliran menggabungkan teks jawaban dari chunk streaming yang
// ditampung untuk cache, supaya entri streaming ikut menyimpan teks jawaban
// (audit3 V-GW-04) — sebelumnya hanya entri non-streaming yang punya Teks.
//
// Dua bentuk potongan dikenali: delta.content dialek OpenAI dan text_delta
// dialek Anthropic. Chunk yang tidak terurai dilewati; hasil gabungan ini
// bahan audit & penyaring, bukan rekonstruksi byte respons.
func teksDariChunkAliran(chunks []string) string {
	var b strings.Builder
	for _, chunk := range chunks {
		var wire struct {
			Choices []struct {
				Delta struct {
					Content json.RawMessage `json:"content"`
				} `json:"delta"`
			} `json:"choices"`
			Delta struct {
				Text string `json:"text"`
			} `json:"delta"`
		}
		if err := json.Unmarshal([]byte(chunk), &wire); err != nil {
			continue
		}
		for _, c := range wire.Choices {
			b.WriteString(teksDariKontenWire(c.Delta.Content))
		}
		b.WriteString(wire.Delta.Text)
	}
	return b.String()
}

// pemakaianCacheHIT menyusun pemakaian yang dicatat untuk satu cache HIT.
//
// Entri lama bisa menyimpan TotalTokens 0; tanpa lantai ini CatatToken
// langsung return dan HIT lama menjadi gratis tanpa batas, sementara baris
// requests mencatat 0 token untuk request yang kuotanya ditagih sebesar
// taksiran (audit3 V-GW-09). Satu angka dipakai untuk KEDUA sisi — yang
// ditagih ke kuota dan yang masuk catatan — supaya keduanya tidak pernah
// bercerita berbeda. Lantainya ditaksir dari prompt peminta, konsisten
// dengan EstimateTokens yang dipakai persiapan untuk routing.
func pemakaianCacheHIT(req *providers.ChatRequest, entry *responsecache.Entry) providers.Usage {
	u := entry.Usage
	if u.TotalTokens > 0 {
		return u
	}
	est := EstimateTokens(req).InputTokens
	if est < 1 {
		est = 1
	}
	u.InputTokens = est
	u.TotalTokens = est
	return u
}

// kandidatCacheHIT menentukan kandidat yang dicatat sebagai pelayan satu
// cache HIT (audit3 V-GW-09): kandidat dari keputusan routing request INI
// yang providernya sama dengan asal entri — nama & pemetaan modelnya masih
// yang berlaku sekarang. Bila provider asal sudah tidak termasuk kandidat
// (routing berubah sejak entri disimpan), kandidat sintetis minimal disusun
// dari identitas yang tersimpan di entri, supaya baris requests tetap
// menyebut provider & model hulu asal jawaban itu. Nil bila entri tidak
// membawa ProviderID sama sekali (entri warisan sebelum V-GW-04 diperbaiki
// di jalur streaming).
func kandidatCacheHIT(pr *persiapan, entry *responsecache.Entry) *upstream.RouteCandidate {
	if entry == nil || entry.ProviderID == "" {
		return nil
	}
	for _, c := range pr.decision.Candidates {
		if c != nil && c.ProviderID == entry.ProviderID {
			return c
		}
	}
	return &upstream.RouteCandidate{
		ProviderID:        entry.ProviderID,
		UpstreamModelName: entry.Model,
	}
}

// bacaBody membaca body permintaan dengan batas, sambil menegakkan kontrak
// header yang dijanjikan permukaan ini:
//
//   - Content-Type yang DIKIRIM klien wajib application/json. Header yang salah
//     label (text/plain, form) sebelumnya diproses penuh seolah JSON — klien
//     yang salah konfigurasi tidak pernah tahu (audit3 V-GW-45). Header yang
//     TIDAK dikirim sama sekali tetap diterima: klien non-browser yang sah ada
//     yang tidak menyetelnya, dan menolaknya tidak membeli keamanan apa pun
//     di permukaan Bearer-token ini (beda dengan permukaan auth berbasis
//     cookie, yang mewajibkannya sebagai penutup CSRF struktural).
//   - Content-Encoding gzip dibuka; pengkodean lain ditolak 415. Sebelumnya
//     header ini tidak pernah diperiksa: JSON tergzip ditolak sebagai "bukan
//     JSON yang sah" dan header gzip bohong di atas body polos diabaikan
//     diam-diam (audit3 V-GW-44).
//   - Body yang melewati batas ukuran dijawab 413 juga pada pengiriman
//     chunked. Jalur Content-Length sudah 413 dari middleware, tetapi pada
//     chunked kesalahan MaxBytesReader dari lapisan yang sama sebelumnya
//     tersapu menjadi 400 generik (audit3 V-GW-41).
func (h *Handlers) bacaBody(w http.ResponseWriter, r *http.Request) ([]byte, bool) {
	if r.Body == nil {
		httpx.BadRequest(w, r, httpx.CodeInvalidRequest, "body permintaan kosong")
		return nil, false
	}

	if ct := r.Header.Get("Content-Type"); ct != "" {
		mediaType, _, err := mime.ParseMediaType(ct)
		if err != nil || mediaType != "application/json" {
			httpx.WriteError(w, r, http.StatusUnsupportedMediaType, httpx.ErrTypeInvalidRequest,
				"unsupported_media_type", "Content-Type harus application/json")
			return nil, false
		}
	}

	body, err := io.ReadAll(io.LimitReader(r.Body, maxBodyBytes+1))
	if err != nil {
		var terlalu *http.MaxBytesError
		if errors.As(err, &terlalu) {
			httpx.WriteError(w, r, http.StatusRequestEntityTooLarge, httpx.ErrTypeInvalidRequest,
				httpx.CodePayloadTooLarge, "ukuran body request melebihi batas yang diizinkan")
			return nil, false
		}
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

	switch enc := strings.ToLower(strings.TrimSpace(r.Header.Get("Content-Encoding"))); enc {
	case "", "identity":
	case "gzip":
		gz, err := gzip.NewReader(bytes.NewReader(body))
		if err != nil {
			httpx.BadRequest(w, r, httpx.CodeInvalidRequest,
				"body dinyatakan tergzip tetapi tidak bisa dibuka")
			return nil, false
		}
		terbuka, err := io.ReadAll(io.LimitReader(gz, maxBodyBytes+1))
		_ = gz.Close()
		if err != nil {
			httpx.BadRequest(w, r, httpx.CodeInvalidRequest,
				"body tergzip tidak bisa dibaca sampai selesai")
			return nil, false
		}
		if len(terbuka) > maxBodyBytes {
			httpx.PayloadTooLarge(w, r, maxBodyBytes)
			return nil, false
		}
		body = terbuka
	default:
		httpx.WriteError(w, r, http.StatusUnsupportedMediaType, httpx.ErrTypeInvalidRequest,
			"unsupported_media_type",
			"Content-Encoding "+kutip(enc)+" tidak didukung; kirim tanpa pengkodean atau dengan gzip")
		return nil, false
	}

	if len(body) == 0 {
		httpx.BadRequest(w, r, httpx.CodeInvalidRequest, "body permintaan kosong")
		return nil, false
	}
	return body, true
}

// bentukUUID melaporkan apakah s berbentuk UUID baku (36 karakter, tanda
// hubung pada posisi baku, sisanya heksadesimal). Duplikasi sadar dari
// pemeriksaan yang sama di repo upstream (idOK): di sini ia dipakai untuk
// menolak pengenal internal di batas klien, bukan untuk validasi database.
func bentukUUID(s string) bool {
	if len(s) != 36 {
		return false
	}
	for i := 0; i < len(s); i++ {
		c := s[i]
		switch i {
		case 8, 13, 18, 23:
			if c != '-' {
				return false
			}
		default:
			if !('0' <= c && c <= '9' || 'a' <= c && c <= 'f' || 'A' <= c && c <= 'F') {
				return false
			}
		}
	}
	return true
}

// selesaikanModel mengubah nama yang diminta klien menjadi model kanonik.
//
// Tiga jawaban yang berbeda dibedakan di sini, dan ketiganya berarti tindak lanjut yang
// berbeda bagi pengguna: nama tidak dikenal, model dimatikan operator, dan model ditandai
// usang. Menyamakan ketiganya menjadi "model tidak ditemukan" membuat pengguna mengganti
// nama model yang sebenarnya sudah benar.
func (h *Handlers) selesaikanModel(w http.ResponseWriter, r *http.Request, diminta string) (*upstream.Model, bool) {
	// Nama model dari klien adalah nama kanonik atau alias — tidak pernah UUID
	// baris internal. Resolve di lapisan repo memang menerima UUID, tetapi itu
	// untuk pemanggil internal yang memegang pengenal baris (target aturan
	// routing, normalisasi cakupan admin); meneruskannya ke permukaan klien
	// membocorkan pengenal internal sebagai "nama model" yang bisa dipakai
	// siapa pun yang mengetahuinya (audit3 V-GW-39).
	if bentukUUID(diminta) {
		httpx.WriteError(w, r, http.StatusNotFound, httpx.ErrTypeInvalidRequest, "model_not_found",
			"model "+kutip(diminta)+" tidak dikenal gateway ini; lihat GET /v1/models")
		return nil, false
	}
	model, err := h.models.Resolve(r.Context(), diminta)
	if errors.Is(err, repo.ErrNotFound) && h.engine != nil {
		if targetModelID := h.engine.FindRuleTargetModelID(r.Context(), diminta); targetModelID != "" {
			model, err = h.models.Resolve(r.Context(), targetModelID)
		}
	}
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
//
// Nilai kedua, dilarangKey, melaporkan keadaan yang harus dibedakan pemanggil:
// model PUNYA pemetaan provider, tetapi semuanya gugur KHUSUS karena
// pembatasan provider pada API key (bukan penyaring kebijakan/anggaran).
// Keadaan itu milik klien — 403 — dan menyamakannya dengan "tidak ada
// provider yang dikonfigurasi" (503) mengirim operator maupun pemegang key
// mencari konfigurasi yang sebenarnya tidak rusak (audit3 V-GW-42).
func (h *Handlers) kandidat(w http.ResponseWriter, r *http.Request, rreq router.Request, p *apikey.Principal) (terpilih []*upstream.RouteCandidate, dilarangKey bool, ok bool) {
	cands, err := h.candidates.RouteCandidates(r.Context(), upstream.RouteQuery{
		ModelID:     rreq.ModelID,
		OwnerUserID: rreq.OwnerUserID,
	})
	if err != nil {
		h.logger.ErrorContext(r.Context(), "pengambilan kandidat rute gagal",
			"model", rreq.ModelName, "error", err)
		httpx.InternalError(w, r)
		return nil, false, false
	}

	periksaKey := h.restrict != nil && p.ID() != ""

	diingat := make(map[string]bool, len(cands))
	// sebabGugur mengingat MENGAPA satu provider gugur: "key" bila ia lolos
	// penyaring kebijakan/anggaran tetapi ditolak pembatasan key, "lain"
	// untuk sisanya. Tanpa beda ini dilarangKey tidak bisa ditentukan.
	sebabGugur := make(map[string]string, len(cands))
	var gugurKey, gugurLain int
	out := make([]*upstream.RouteCandidate, 0, len(cands))
	for _, c := range cands {
		boleh, ada := diingat[c.ProviderID]
		if !ada {
			// Pembatasan provider dari penyaring konten MENYARING kandidat, tidak menolak
			// permintaan: selama masih ada kandidat lain, permintaan tetap dilayani lewat
			// kandidat itu. Kalau semuanya tersaring, tolakNyaTanpaKandidat yang menjelaskan
			// sebabnya — jauh lebih bisa ditindaklanjuti daripada 502 dari provider yang
			// sengaja tidak pernah dihubungi.
			// Anggaran cakupan provider yang habis menyaring dengan cara yang sama
			// (audit3 V-AB-04): ia tidak bisa diperiksa sebelum provider dipilih.
			boleh = !h.guard.ProviderDilarang(r.Context(), c.ProviderID) &&
				!h.guard.ProviderAnggaranHabis(r.Context(), c.ProviderID)
			if !boleh {
				sebabGugur[c.ProviderID] = "lain"
			}
			if boleh && periksaKey {
				var err error
				boleh, err = h.restrict.AllowsProvider(r.Context(), p.ID(), c.ProviderID)
				if err != nil {
					h.logger.ErrorContext(r.Context(), "pemeriksaan pembatasan provider gagal",
						"api_key", p.Masked(), "error", err)
					httpx.InternalError(w, r)
					return nil, false, false
				}
				if !boleh {
					sebabGugur[c.ProviderID] = "key"
				}
			}
			diingat[c.ProviderID] = boleh
		}
		if boleh {
			out = append(out, c)
		} else if sebabGugur[c.ProviderID] == "key" {
			gugurKey++
		} else {
			gugurLain++
		}
	}
	dilarangKey = len(cands) > 0 && len(out) == 0 && gugurKey > 0 && gugurLain == 0
	return out, dilarangKey, true
}

// kandidatCombo mengumpulkan kandidat dari seluruh model dalam resep combo pipeline,
// lalu mengurutkannya menurut strategi resep.
//
// Berbeda dengan kandidat() karena resep combo sengaja mencampur provider dari beberapa
// model menjadi SATU daftar failover. Urutan antar model ditentukan oleh resep
// (pipeline.strategy), bukan oleh rule.strategy — kandidat dari model kedua boleh lebih
// dulu daripada kandidat dari model pertama bila resepnya meminta lowest_latency — dan
// anggaran total (Plan.Budget) yang membaginya, bukan cascade per model.
//
// ok false berarti resep tidak menghasilkan satu pun kandidat yang bisa dipakai. Itu
// bukan kegagalan yang menolak permintaan: pemanggil menjatuhkannya ke daftar kandidat
// model yang diminta, yang mungkin masih bisa melayani.
func (h *Handlers) kandidatCombo(ctx context.Context, pr *persiapan, rreq router.Request) ([]*upstream.RouteCandidate, bool) {
	p := pr.principal
	var ownerUserID string
	if p != nil {
		ownerUserID = p.OwnerUserID()
	}

	pernah := make(map[string]bool)
	var gabungan []*upstream.RouteCandidate
	for _, namaModel := range pr.decision.Rule.Pipeline.Models {
		// Tiap model di resep adalah model kanonik yang harus diresolve dan diperiksa
		// kewenangannya sendiri. Melewati pemeriksaan ini akan mengizinkan API key yang
		// dibatasi ke satu model memakai model lain dalam resep — pelanggaran kewenangan
		// yang dibungkam oleh daftar kandidat gabungan.
		m, err := h.models.Resolve(ctx, namaModel)
		if err != nil || m == nil {
			h.logger.WarnContext(ctx, "model combo tidak bisa diselesaikan, model dilewati",
				"model", namaModel, "error", err)
			continue
		}
		if !m.Enabled {
			h.logger.WarnContext(ctx, "model combo dimatikan, model dilewati",
				"model", namaModel)
			continue
		}
		if h.restrict != nil && p.ID() != "" {
			boleh, err := h.restrict.AllowsModel(ctx, p.ID(), m.ID)
			if err != nil {
				// GAGAL-TERTUTUP, alasan sama dengan izinModel: ini kontrol kewenangan,
				// dan melewatkannya saat database tidak bisa dibaca berarti key yang
				// dibatasi mendapat model yang justru dilarang untuknya.
				h.logger.ErrorContext(ctx, "pemeriksaan pembatasan model combo gagal",
					"api_key", p.Masked(), "model", namaModel, "error", err)
				continue
			}
			if !boleh {
				continue
			}
		}

		cands, err := h.candidates.RouteCandidates(ctx, upstream.RouteQuery{
			ModelID:     m.ID,
			OwnerUserID: ownerUserID,
		})
		if err != nil {
			h.logger.WarnContext(ctx, "kandidat model combo tidak bisa diambil, model dilewati",
				"model", namaModel, "error", err)
			continue
		}

		for _, c := range cands {
			// Dua model dalam satu resep bisa dilayani pemetaan provider+model yang sama.
			// Tanpa dedup, pemetaan itu muncul dua kali dan provider bersangkutan
			// menanggung beban ganda dari satu permintaan klien.
			if pernah[c.ProviderModelID] {
				continue
			}
			if h.guard.ProviderDilarang(ctx, c.ProviderID) {
				continue
			}
			if h.guard.ProviderAnggaranHabis(ctx, c.ProviderID) {
				continue
			}
			if h.restrict != nil && p.ID() != "" {
				boleh, err := h.restrict.AllowsProvider(ctx, p.ID(), c.ProviderID)
				if err != nil {
					h.logger.ErrorContext(ctx, "pemeriksaan pembatasan provider combo gagal",
						"api_key", p.Masked(), "provider", c.ProviderID, "error", err)
					continue
				}
				if !boleh {
					continue
				}
			}
			pernah[c.ProviderModelID] = true
			gabungan = append(gabungan, c)
		}
	}

	// Pengurutan dan penyaringan kemampuan (streaming, tools, jendela konteks) dilakukan
	// di satu tempat oleh Selector, sama seperti jalur satu model. Kandidat yang tidak
	// sanggup melayani permintaan ini harus dibuang di sini, bukan diletakkan di belakang
	// barisan failover: menunda kegagalan hanya membakar kuota.
	gabungan = h.engine.OrderCandidates(rreq, pr.decision.Rule.ID, pr.decision.Rule.Pipeline.Strategy, gabungan)
	return gabungan, len(gabungan) > 0
}

// Pesan tunggal "tidak ada provider" adalah jawaban yang tidak bisa ditindaklanjuti
// siapa pun: operator tidak tahu apakah harus menambah provider, menyalakan streaming pada
// pemetaan model, atau melonggarkan pembatasan API key. Empat sebab yang mungkin dibedakan
// di sini karena tindak lanjutnya berbeda, dan hanya kode ini yang tahu bedanya.
func (h *Handlers) tolakTanpaKandidat(
	w http.ResponseWriter, r *http.Request,
	rreq router.Request, model *upstream.Model, sebelumDisaring []*upstream.RouteCandidate,
	dilarangKey bool,
) {
	nama := kutip(model.ModelID)

	// Model punya pemetaan provider, tetapi API key ini dilarang memakai
	// semuanya. Itu penolakan kewenangan milik klien (403), bukan kekosongan
	// konfigurasi: menjawabnya 503 "tidak ada provider yang dikonfigurasi"
	// mengklaim hal yang salah tentang instalasi ini (audit3 V-GW-42).
	if dilarangKey {
		httpx.WriteError(w, r, http.StatusForbidden, httpx.ErrTypePermission, "provider_not_allowed",
			"API key ini tidak diizinkan memakai provider mana pun untuk model "+nama)
		return
	}

	// Tidak ada pemetaan provider sama sekali.
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
	case perluEmbeddings(rreq):
		// Keputusan kosong untuk permintaan embeddings padahal kandidat ada:
		// aturan routing membatasi provider untuk permintaan berkemampuan
		// ini. (Model yang sama sekali bukan model embeddings sudah ditolak
		// lebih dulu di handler Embeddings dari capabilities modelnya.)
		// Keadaan permintaan/konfigurasi yang pantas 400, bukan diratakan
		// menjadi 503 seperti model yatim (audit3 V-GW-43).
		httpx.WriteError(w, r, http.StatusBadRequest, httpx.ErrTypeInvalidRequest, "embeddings_not_supported",
			"tidak ada provider yang bisa melayani embeddings untuk model "+nama)
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

// perluEmbeddings melaporkan apakah permintaan ini menuntut kemampuan embeddings.
func perluEmbeddings(rreq router.Request) bool {
	for _, c := range rreq.Capabilities {
		if c == upstream.CapEmbeddings {
			return true
		}
	}
	return false
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
// jagaKuota adalah penjaga pengembalian kuota Limit() untuk satu permintaan
// (audit3 V-AB-17).
//
// Middleware Limit() menaikkan penghitung sebelum handler berjalan. Penjaga
// ini dipasang di awal handler dan ditutup lewat defer: bila handler keluar
// TANPA pernah memanggil lolos(), permintaan itu ditolak sebelum satu pun
// percobaan upstream — kuotanya dikembalikan. lolos() dipanggil begitu
// persiapan berhasil, karena sesudah itu eksekusi upstream pasti dicoba dan
// kuota yang terbakar memang sah.
type jagaKuota struct {
	h     *Handlers
	r     *http.Request
	aktif bool
}

// jagaKuotaPraUpstream memasang penjaga kuota pada permintaan ini.
func (h *Handlers) jagaKuotaPraUpstream(r *http.Request) *jagaKuota {
	return &jagaKuota{h: h, r: r, aktif: true}
}

// lolos menandai persiapan berhasil dan eksekusi upstream akan dicoba.
func (j *jagaKuota) lolos() { j.aktif = false }

// tutup mengembalikan kuota bila permintaan berakhir sebagai penolakan
// pra-upstream. Aman dipanggil berulang.
func (j *jagaKuota) tutup() {
	if !j.aktif {
		return
	}
	j.aktif = false
	p, ok := apikey.PrincipalFrom(j.r.Context())
	if !ok || p == nil {
		return
	}
	ip, _ := httpx.ClientIPFrom(j.r.Context())
	j.h.guard.KembalikanKuota(j.r.Context(), p, ip)
}

func (h *Handlers) ChatCompletions(w http.ResponseWriter, r *http.Request) {
	// Jejak dibuat sebelum apa pun dan diserahkan lewat defer, sehingga SETIAP jalan keluar
	// — termasuk yang belum ada saat baris ini ditulis — menghasilkan satu baris log
	// request. Writer yang dipakai selanjutnya adalah writer dari jejak, karena itulah yang
	// tahu status dan envelope apa yang benar-benar terkirim.
	j, w := h.mulaiJejak(w, r)
	defer j.selesai(r.Context())

	jaga := h.jagaKuotaPraUpstream(r)
	defer jaga.tutup()

	pr, ok := h.siapkanChat(w, r, j)
	if !ok {
		return
	}
	jaga.lolos()
	if pr.req.Stream {
		h.chatMengalir(w, r, pr, j)
		return
	}
	h.chatSekali(w, r, pr, j)
}

// ComboTier mendefinisikan satu langkah dalam cascade combo routing pipeline.
type ComboTier struct {
	Tier        int      `json:"tier"`
	Model       string   `json:"model"`
	ProviderIDs []string `json:"providers,omitempty"`
}

// ekstrakComboPipeline membaca tahapan cascade dari aturan combo routing.
// Format baru: [combo:pipeline=[{"tier":2,"model":"...","providers":["..."]},...]]
// Format lama (tetap didukung): [combo:tier2=<model_name>]
func ekstrakComboPipeline(rule *router.Rule) []ComboTier {
	if rule == nil || rule.Description == "" {
		return nil
	}
	const prefix = "[combo:pipeline="
	idx := strings.Index(rule.Description, prefix)
	if idx != -1 {
		sub := rule.Description[idx+len(prefix):]
		if strings.HasPrefix(sub, "[") {
			var tiers []ComboTier
			dec := json.NewDecoder(strings.NewReader(sub))
			if err := dec.Decode(&tiers); err == nil && len(tiers) > 0 {
				return tiers
			}
		}
	}

	// Dukungan mundur format lama: [combo:tier2=<model_name>]
	if t2 := ekstrakTier2Model(rule); t2 != "" {
		return []ComboTier{
			{
				Tier:  2,
				Model: t2,
			},
		}
	}
	return nil
}

// ekstrakTier2Model membaca target fallback model dari aturan combo routing format lama.
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

// semuaKandidatKekecilan melaporkan apakah seluruh kandidat memiliki context window
// yang diketahui dan lebih kecil dari inputTokens permintaan.
func semuaKandidatKekecilan(cands []*upstream.RouteCandidate, inputTokens int) bool {
	if len(cands) == 0 || inputTokens <= 0 {
		return false
	}
	for _, c := range cands {
		if c.MaxContextWindow == nil || *c.MaxContextWindow <= 0 || *c.MaxContextWindow >= inputTokens {
			return false
		}
	}
	return true
}

// siapkanTierFallback menyiapkan fallback combo routing untuk tier tertentu: resolusi model,
// kewenangan key, penyaringan provider khusus tier (jika ada), kebijakan, dan kandidat tersaring.
func (h *Handlers) siapkanTierFallback(ctx context.Context, tierModelName string, allowedProviderIDs []string, pr *persiapan, stream bool) (*upstream.Model, []*upstream.RouteCandidate, []policy.Target, *Rejection) {
	tModel, err := h.models.Resolve(ctx, tierModelName)
	if err != nil {
		h.logger.WarnContext(ctx, "resolusi combo fallback model gagal", "target_model", tierModelName, "error", err)
		return nil, nil, nil, nil
	}
	if !tModel.Enabled {
		h.logger.WarnContext(ctx, "combo fallback model dimatikan, fallback dilewati", "target_model", tierModelName)
		return nil, nil, nil, nil
	}
	if pr.model != nil && tModel.ID == pr.model.ID {
		h.logger.WarnContext(ctx, "combo fallback model sama dengan model saat ini, fallback dilewati", "target_model", tierModelName)
		return nil, nil, nil, nil
	}
	p := pr.principal
	if h.restrict != nil && p.ID() != "" {
		boleh, err := h.restrict.AllowsModel(ctx, p.ID(), tModel.ID)
		if err != nil {
			h.logger.ErrorContext(ctx, "pemeriksaan pembatasan model fallback gagal",
				"api_key", p.Masked(), "model", tModel.ModelID, "error", err)
			return nil, nil, nil, nil
		}
		if !boleh {
			return nil, nil, nil, &Rejection{
				Status: http.StatusForbidden, Type: httpx.ErrTypePermission, Code: "model_not_allowed",
				Message: "API key ini tidak diizinkan memakai model " + kutip(tModel.ModelID),
			}
		}
	}
	clientIP, _ := httpx.ClientIPFrom(ctx)
	targets := TargetsFor(p, tModel.ID, clientIP)
	if rj := h.guard.SaringPermintaan(ctx, contentfilter.Subject{ModelID: tModel.ID}); rj != nil {
		return nil, nil, nil, rj
	}
	if rj := h.guard.PeriksaBatasModel(ctx, tModel.ID); rj != nil {
		return nil, nil, nil, rj
	}
	if rj := h.guard.PeriksaAnggaran(ctx, targets); rj != nil {
		return nil, nil, nil, rj
	}
	if rj := h.guard.GerbangToken(ctx, p, targets); rj != nil {
		return nil, nil, nil, rj
	}
	var ownerUserID string
	if p != nil {
		ownerUserID = p.OwnerUserID()
	}
	cands, err := h.candidates.RouteCandidates(ctx, upstream.RouteQuery{
		ModelID:     tModel.ID,
		OwnerUserID: ownerUserID,
	})
	if err != nil || len(cands) == 0 {
		h.logger.WarnContext(ctx, "tidak ada kandidat untuk combo fallback model", "target_model", tierModelName, "error", err)
		return nil, nil, nil, nil
	}

	// Filter kandidat berdasarkan allowedProviderIDs jika ditentukan untuk tier ini
	if len(allowedProviderIDs) > 0 {
		filtered := make([]*upstream.RouteCandidate, 0, len(cands))
		for _, c := range cands {
			if slices.Contains(allowedProviderIDs, c.ProviderID) {
				filtered = append(filtered, c)
			}
		}
		cands = filtered
		if len(cands) == 0 {
			h.logger.WarnContext(ctx, "tidak ada kandidat lolos filter provider tier", "target_model", tierModelName, "allowed_providers", allowedProviderIDs)
			return nil, nil, nil, nil
		}
	}

	saring, ok := h.saringKandidatTier2(ctx, cands, p, stream)
	if !ok || len(saring) == 0 {
		h.logger.WarnContext(ctx, "tidak ada kandidat fallback yang lolos penyaringan provider", "target_model", tierModelName)
		return nil, nil, nil, nil
	}
	return tModel, saring, targets, nil
}

// saringKandidatTier2 menerapkan penyaringan provider yang sama seperti jalur
// tier1 di kandidat(): larangan provider dari penyaring konten dan pembatasan
// provider pada API key — tanpa akses writer. Gagal-tertutup: error pemeriksaan
// membatalkan tier2 (ok false), bukan meloloskannya.
func (h *Handlers) saringKandidatTier2(ctx context.Context, cands []*upstream.RouteCandidate, p *apikey.Principal, stream bool) ([]*upstream.RouteCandidate, bool) {
	periksaKey := h.restrict != nil && p.ID() != ""
	diingat := make(map[string]bool, len(cands))
	out := make([]*upstream.RouteCandidate, 0, len(cands))
	for _, c := range cands {
		if stream && !c.SupportsStreaming {
			continue
		}
		boleh, ada := diingat[c.ProviderID]
		if !ada {
			boleh = !h.guard.ProviderDilarang(ctx, c.ProviderID) &&
				!h.guard.ProviderAnggaranHabis(ctx, c.ProviderID)
			if boleh && periksaKey {
				var err error
				boleh, err = h.restrict.AllowsProvider(ctx, p.ID(), c.ProviderID)
				if err != nil {
					h.logger.ErrorContext(ctx, "pemeriksaan pembatasan provider tier2 gagal",
						"api_key", p.Masked(), "error", err)
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

// cobaTierFallbackSekali mencoba eksekusi satu tier fallback non-streaming.
func (h *Handlers) cobaTierFallbackSekali(ctx context.Context, tier ComboTier, pr *persiapan, j *jejak) (*Outcome[*providers.ChatResponse], *Rejection) {
	tModel, cands, targets, rj := h.siapkanTierFallback(ctx, tier.Model, tier.ProviderIDs, pr, false)
	if rj != nil {
		return nil, rj
	}
	if tModel == nil {
		return nil, nil
	}

	tReq := *pr.req
	tReq.Model = tModel.ModelID

	tPlan := PlanFromRule(pr.decision.Rule, tModel.ModelID, cands)

	h.logger.InfoContext(ctx, "menjalankan fallback combo routing tier", "tier", tier.Tier, "tier_model", tModel.ModelID)
	out := Execute(h.guard.DenganMemoBatas(ctx), h.exec, tPlan,
		func(c context.Context, cand *upstream.RouteCandidate) (*providers.ChatResponse, error) {
			if perr := h.guard.PeriksaBatasProvider(c, cand.ProviderID, cand.ProviderName); perr != nil {
				return nil, perr
			}
			if perr := h.guard.PeriksaKolomProvider(c, cand); perr != nil {
				return nil, perr
			}
			p, err := h.adapter(c, cand)
			if err != nil {
				return nil, err
			}
			return p.ChatCompletion(c, untukKandidat(&tReq, cand))
		})

	j.tambahPercobaan(out.Attempts, out.Candidate)
	if out.Err == nil {
		pr.model = tModel
		pr.targets = targets
		j.pasangModel(tModel)
		j.pasangKebijakan(pr.principal, targets)
		return out, nil
	}
	return out, nil
}

// cobaTierFallbackMengalir mencoba eksekusi satu tier fallback streaming.
func (h *Handlers) cobaTierFallbackMengalir(ctx context.Context, tier ComboTier, pr *persiapan, j *jejak) (*Outcome[providers.Stream], *Rejection) {
	tModel, cands, targets, rj := h.siapkanTierFallback(ctx, tier.Model, tier.ProviderIDs, pr, true)
	if rj != nil {
		return nil, rj
	}
	if tModel == nil {
		return nil, nil
	}

	tReq := *pr.req
	tReq.Model = tModel.ModelID

	tPlan := PlanFromRule(pr.decision.Rule, tModel.ModelID, cands)

	h.logger.InfoContext(ctx, "menjalankan fallback streaming combo routing tier", "tier", tier.Tier, "tier_model", tModel.ModelID)
	out := ExecuteStream(h.guard.DenganMemoBatas(ctx), h.exec, tPlan,
		func(c context.Context, cand *upstream.RouteCandidate) (providers.Stream, error) {
			if perr := h.guard.PeriksaBatasProvider(c, cand.ProviderID, cand.ProviderName); perr != nil {
				return nil, perr
			}
			if perr := h.guard.PeriksaKolomProvider(c, cand); perr != nil {
				return nil, perr
			}
			p, err := h.adapter(c, cand)
			if err != nil {
				return nil, err
			}
			return p.ChatCompletionStream(c, untukKandidat(&tReq, cand))
		})

	j.tambahPercobaan(out.Attempts, out.Candidate)
	if out.Err == nil {
		pr.model = tModel
		pr.targets = targets
		j.pasangModel(tModel)
		j.pasangKebijakan(pr.principal, targets)
		return out, nil
	}
	return out, nil
}

// cakupanCache menyusun konteks pemilik untuk kunci response cache.
//
// Tanpa ini dua penyewa berbagi satu entri: tenant B menerima respons milik
// tenant A, kuotanya tidak termakan, dan biayanya tercatat nol padahal
// informasinya bocor. ModelID-nya ID kanonik (models.id), bukan nama mentah
// dari klien, supaya dua alias ke model yang sama tetap berbagi entri secara
// sah — respons upstream-nya identik.
func cakupanCache(pr *persiapan) responsecache.Scope {
	var s responsecache.Scope
	if pr == nil {
		return s
	}
	if pr.model != nil {
		s.ModelID = pr.model.ID
	}
	if pr.principal != nil {
		s.OwnerUserID = pr.principal.OwnerUserID()
		s.KeyID = pr.principal.ID()
	}
	return s
}

// terapkanLantaiPemakaian mengisi sisi pemakaian yang TIDAK dilaporkan upstream.
//
// Upstream yang sah boleh menjawab tanpa objek usage sama sekali, dan aliran
// boleh berakhir (atau diputus klien) sebelum chunk usage final tiba. Tanpa
// lantai, permintaan seperti itu tercatat 0 token: kuotanya tidak berkurang
// dan biayanya nol, padahal prompt-nya nyata terkirim dan jawabannya nyata
// dihasilkan — kebocoran penagihan keluarga PR #202 (audit3 V-GW-10, V-AB-14).
//
// Dua sisi ditaksir dari bahan yang berbeda dengan sengaja:
//
//   - INPUT dari prompt peminta (EstimateTokens — taksiran yang sama yang
//     dipakai routing). Versi lama lantai aliran hanya menaksir output, jadi
//     token input hilang total pada aliran tanpa chunk usage.
//   - OUTPUT dari byte KONTEN yang benar-benar dihasilkan (byteKonten),
//     dibagi bytePerToken. Pemanggil WAJIB menyerahkan byte konten, bukan
//     byte aliran terbingkai: pada permukaan Anthropic framing SSE (baris
//     event:, envelope JSON) jauh lebih besar daripada kontennya, dan
//     menaksir dari byte terbingkai menggelembungkan taksiran belasan kali
//     lipat (audit3 V-AB-15).
//
// Angka yang SUDAH dilaporkan upstream tidak pernah ditimpa: lantai hanya
// mengisi sisi yang kosong.
func terapkanLantaiPemakaian(req *providers.ChatRequest, u providers.Usage, byteKonten int64) providers.Usage {
	if u.InputTokens <= 0 {
		if est := EstimateTokens(req).InputTokens; est > 0 {
			u.InputTokens = est
			u.TotalTokens += est
		}
	}
	if u.OutputTokens <= 0 && byteKonten > 0 {
		keluar := int(byteKonten / bytePerToken)
		if keluar < 1 {
			keluar = 1
		}
		u.OutputTokens = keluar
		u.TotalTokens += keluar
	}
	return u
}

// chatSekali melayani completion non-streaming.
func (h *Handlers) chatSekali(w http.ResponseWriter, r *http.Request, pr *persiapan, j *jejak) {
	var cacheKey string
	if h.respCache != nil && h.respCache.IsEnabled() {
		cacheKey = h.respCache.ComputeScopedKey(pr.req, cakupanCache(pr))
		if entry, hit, err := h.respCache.Get(r.Context(), cacheKey); err == nil && hit && entry != nil && len(entry.Raw) > 0 {
			// HIT tetap memakan kuota token: tanpa ini prompt yang di-cache bisa
			// diulang tanpa batas melewati token limit. Biayanya tetap nol —
			// tidak ada panggilan upstream — karena pencatat menghitung biaya
			// dari kandidat pemenang, dan jalur ini tidak punya pemenang.
			//
			// Pemakaian yang ditagih dan yang dicatat disusun sekali lewat
			// pemakaianCacheHIT (lantai taksiran untuk entri lama bertoken nol).
			pemakaian := pemakaianCacheHIT(pr.req, entry)
			j.pasangPemakaian(pemakaian)
			// Baris requests untuk HIT tetap menyebut provider & pemetaan
			// model asal jawaban (audit3 V-GW-09); tanpa ini inspektor
			// menampilkan request tanpa provider untuk semua HIT.
			if menang := kandidatCacheHIT(pr, entry); menang != nil {
				j.pasangPercobaan(nil, menang)
			}
			h.guard.CatatToken(r.Context(), pr.principal, pr.targets, int64(pemakaian.TotalTokens))
			// Kebijakan jawaban yang berlaku saat menyimpan bisa berubah sejak
			// itu; yang ditegakkan adalah kebijakan SAAT INI. Teks diambil dari
			// entri; entri lama yang belum punya teks diekstrak dari Raw-nya —
			// tanpa itu entri lama melewati penyaring sepenuhnya
			// (audit3 V-GW-06).
			teks := entry.Teks
			if teks == "" {
				teks = teksDariRawJawaban(entry.Raw)
			}
			if rj := h.guard.SaringJawaban(r.Context(), teks,
				pr.model.ID, entry.ProviderID); rj != nil {
				rj.Tulis(w, r)
				return
			}
			w.Header().Set("Content-Type", "application/json")
			w.Header().Set("Cache-Control", "no-store")
			w.Header().Set("X-RouteX-Cache", "HIT")
			w.WriteHeader(http.StatusOK)
			_, _ = w.Write(entry.Raw)
			return
		}
	}

	out := Execute(h.guard.DenganMemoBatas(r.Context()), h.exec, pr.plan,
		func(ctx context.Context, c *upstream.RouteCandidate) (*providers.ChatResponse, error) {
			if perr := h.guard.PeriksaBatasProvider(ctx, c.ProviderID, c.ProviderName); perr != nil {
				return nil, perr
			}
			if perr := h.guard.PeriksaKolomProvider(ctx, c); perr != nil {
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

	// Jika Tier 1 gagal dan aturan adalah combo routing, otomatis cascade fallback ke tier-tier berikutnya
	if out.Err != nil && !pr.decision.Rule.Combo() {
		// Cascade ini memakai tag [combo:...] di description (jalur lama). Aturan combo
		// pipeline jsonb TIDAK memakainya: kandidatnya sudah mencakup seluruh model
		// resep dalam satu loop bercadangan total, sehingga cascade di sini hanya akan
		// mencoba model-model yang sudah gagal di loop itu.
		if pipeline := ekstrakComboPipeline(pr.decision.Rule); len(pipeline) > 0 {
			for _, tier := range pipeline {
				tOut, rj := h.cobaTierFallbackSekali(r.Context(), tier, pr, j)
				switch {
				case rj != nil:
					// Tier fallback DITOLAK kebijakan: tidak ada jawaban lain yang jujur
					// selain penolakan ini — tier sebelumnya sudah gagal.
					rj.Tulis(w, r)
					return
				case tOut != nil:
					// Tier fallback berjalan (sukses atau gagal dengan sebabnya sendiri):
					// jejak percobaan tetap diakumulasikan, dan kegagalan dicatat.
					h.catatRute(r, pr, tOut.Attempts, tOut.Candidate)
					out = tOut
				}
				if out.Err == nil {
					break
				}
			}
		}
	}

	if out.Err != nil {
		j.pasangKegagalanUpstream(out.Err)
		tulisKegagalan(w, r, out.Err)
		return
	}
	// Lantai pemakaian diterapkan SEBELUM apa pun mencatat token: respons tanpa
	// objek usage dari upstream tetap harus memakan kuota dan tertagih
	// (audit3 V-GW-10). Teks jawaban dihitung sekali di sini dan dipakai ulang
	// penyaring jawaban di bawah.
	teks := teksJawaban(out.Value)
	pemakaian := terapkanLantaiPemakaian(pr.req, out.Value.Usage, int64(len(teks)))
	j.pasangPemakaian(pemakaian)

	// Pemakaian token dicatat SEBELUM penyaring jawaban: permintaannya sudah dijalankan dan
	// tokennya sudah ditagihkan upstream, jadi jawaban yang kemudian ditolak penyaring pun
	// tetap harus memakan kuota. Kalau dibalik, klien bisa menghabiskan token tanpa batas
	// dengan mengirim permintaan yang jawabannya selalu tertahan penyaring.
	h.guard.CatatToken(r.Context(), pr.principal, pr.targets, int64(pemakaian.TotalTokens))
	h.guard.CatatTokenKolomProvider(r.Context(), out.Candidate, int64(pemakaian.TotalTokens))

	if rj := h.guard.SaringJawaban(r.Context(), teks,
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
			// ProviderID dan teks ikut disimpan supaya jalur HIT bisa
			// menegakkan penyaring jawaban berlingkup provider dengan
			// kebijakan SAAT HIT, bukan saat menyimpan.
			_ = h.respCache.Set(r.Context(), cacheKey, &responsecache.Entry{
				Model:      out.Candidate.UpstreamModelName,
				ProviderID: out.Candidate.ProviderID,
				Raw:        out.Value.Raw,
				Usage:      pemakaian,
				Teks:       teks,
				CachedAt:   time.Now(),
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
		cacheKey = h.respCache.ComputeScopedKey(pr.req, cakupanCache(pr))
		if entry, hit, err := h.respCache.Get(r.Context(), cacheKey); err == nil && hit && entry != nil && len(entry.StreamChunks) > 0 {
			// Seperti jalur non-streaming: HIT tetap memakan kuota token,
			// dengan lantai taksiran yang sama untuk entri lama bertoken nol,
			// dan pemakaian yang ditagih sama dengan yang dicatat
			// (pemakaianCacheHIT, audit3 V-GW-09). Baris requests juga
			// menyebut provider asal jawaban lewat kandidatCacheHIT.
			// Penyaring jawaban TIDAK dijalankan di sini — jawaban streaming
			// memang tidak disaring (lihat Guard.SaringJawaban), dan memeriksa
			// ulang potongan yang sudah terkirim tidak mungkin.
			pemakaian := pemakaianCacheHIT(pr.req, entry)
			j.pasangPemakaian(pemakaian)
			if menang := kandidatCacheHIT(pr, entry); menang != nil {
				j.pasangPercobaan(nil, menang)
			}
			h.guard.CatatToken(r.Context(), pr.principal, pr.targets, int64(pemakaian.TotalTokens))
			if err := httpx.PrepareSSE(w, r); err != nil {
				h.gagalSiapkanSSE(w, r, j, err)
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
			return
		}
	}

	out := ExecuteStream(h.guard.DenganMemoBatas(r.Context()), h.exec, pr.plan,
		func(ctx context.Context, c *upstream.RouteCandidate) (providers.Stream, error) {
			if perr := h.guard.PeriksaBatasProvider(ctx, c.ProviderID, c.ProviderName); perr != nil {
				return nil, perr
			}
			if perr := h.guard.PeriksaKolomProvider(ctx, c); perr != nil {
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

	// Jika Tier 1 gagal dan aturan adalah combo routing, otomatis cascade fallback ke tier-tier berikutnya
	if out.Err != nil && !pr.decision.Rule.Combo() {
		// Cascade ini memakai tag [combo:...] di description (jalur lama). Aturan combo
		// pipeline jsonb TIDAK memakainya: kandidatnya sudah mencakup seluruh model
		// resep dalam satu loop bercadangan total, sehingga cascade di sini hanya akan
		// mencoba model-model yang sudah habis anggarannya di loop itu.
		if pipeline := ekstrakComboPipeline(pr.decision.Rule); len(pipeline) > 0 {
			for _, tier := range pipeline {
				tOut, rj := h.cobaTierFallbackMengalir(r.Context(), tier, pr, j)
				switch {
				case rj != nil:
					// Sebelum SSE: penolakan kebijakan masih bisa dijawab sebagai status HTTP.
					rj.Tulis(w, r)
					return
				case tOut != nil:
					h.catatRute(r, pr, tOut.Attempts, tOut.Candidate)
					out = tOut
				}
				if out.Err == nil {
					break
				}
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
		h.gagalSiapkanSSE(w, r, j, err)
		return
	}

	rc := http.NewResponseController(w)
	var (
		terkirim     int64
		usage        providers.Usage
		streamChunks []string
		// totalChunk/tampungJenuh membatasi penampungan chunk untuk cache
		// (audit F4); lihat titik append di bawah.
		totalChunk   int
		tampungJenuh bool
	)
	// Pencatatan token dijalankan lewat defer, bukan hanya di jalur selesai-normal: aliran
	// bisa berakhir karena klien menutup koneksi atau upstream terputus di tengah, dan token
	// yang sudah dihasilkan sampai titik itu tetap ditagihkan provider. Melewatkannya berarti
	// klien yang selalu memutus aliran lebih awal tidak pernah memakan kuota tokennya.
	defer func() {
		// Chunk usage final belum tentu tiba saat aliran diputus: tanpa lantai,
		// CatatToken menerima TotalTokens 0 dan langsung return, sehingga token
		// yang sudah terkirim tidak memakan kuota sama sekali. Lantainya
		// menaksir DUA sisi (audit3 V-AB-14): output dari byte yang benar-benar
		// terkirim, dan input dari prompt peminta — versi lama hanya menaksir
		// output, jadi token input hilang total pada aliran tanpa chunk usage
		// padahal prompt-nya nyata ditagihkan upstream.
		usage = terapkanLantaiPemakaian(pr.req, usage, terkirim)
		h.guard.CatatToken(r.Context(), pr.principal, pr.targets, int64(usage.TotalTokens))
		h.guard.CatatTokenKolomProvider(r.Context(), out.Candidate, int64(usage.TotalTokens))
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
					// ProviderID dan teks jawaban ikut disimpan, sejajar
					// jalur non-streaming (audit3 V-GW-04): tanpa keduanya
					// provenance entri streaming hilang dari audit.
					_ = h.respCache.Set(r.Context(), cacheKey, &responsecache.Entry{
						Model:        out.Candidate.UpstreamModelName,
						ProviderID:   out.Candidate.ProviderID,
						Usage:        usage,
						StreamChunks: streamChunks,
						Teks:         teksDariChunkAliran(streamChunks),
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

		// Chunk hanya ditampung bila response cache AKTIF untuk request
		// ini (audit F4): satu-satunya konsumen streamChunks adalah
		// pengisian cache di akhir aliran, jadi menampungnya selalu
		// berarti seluruh respons streaming diduplikasi di memori tanpa
		// batas — termasuk saat cache mati (bawaan sejak audit F3).
		// Batas total 2 MB: aliran yang melewatinya tidak di-cache.
		if !tampungJenuh && cacheKey != "" && h.respCache != nil && h.respCache.IsEnabled() {
			if totalChunk+len(ev.Raw) <= 2<<20 {
				streamChunks = append(streamChunks, string(ev.Raw))
				totalChunk += len(ev.Raw)
			} else {
				tampungJenuh = true
				streamChunks = nil
			}
		}

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

// gagalSiapkanSSE menangani kegagalan PrepareSSE di semua permukaan streaming.
//
// Dua keadaan dibedakan oleh httpx.ErrHeaderSSETerkirim (audit3 V-GW-15):
//   - Header BELUM terkirim (tenggat tidak bisa dilepas): envelope error
//     biasa masih sah dan memang itulah jawabannya.
//   - Header SUDAH terkirim (flush gagal): status 200 terkunci dan aliran
//     sudah dimulai; menulis envelope JSON di sini menempelkan JSON mentah
//     tanpa framing ke aliran SSE dan merusak parser klien. Yang dilakukan
//     hanya mencatat kegagalan di jejak — status yang terkirim dengan
//     error_type terisi, pola yang sama seperti kegagalan di tengah aliran —
//     lalu kembali supaya koneksi ditutup oleh server.
func (h *Handlers) gagalSiapkanSSE(w http.ResponseWriter, r *http.Request, j *jejak, err error) {
	h.logger.ErrorContext(r.Context(), "penyiapan SSE gagal", "error", err)
	if errors.Is(err, httpx.ErrHeaderSSETerkirim) {
		j.pasangKegagalanUpstream(&providers.Error{
			Kind:    providers.ErrKindServer,
			Message: "penyiapan aliran SSE gagal setelah header terkirim",
		})
		return
	}
	httpx.InternalError(w, r)
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

	// Penolakan pra-upstream mengembalikan kuota Limit() (audit3 V-AB-17).
	jaga := h.jagaKuotaPraUpstream(r)
	defer jaga.tutup()

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

	// Model yang capabilities-nya tidak menyatakan embeddings ditolak di
	// sini, dari data modelnya sendiri (audit3 V-GW-43): tanpa pemeriksaan
	// ini permintaan diteruskan ke provider chat dan kegagalan upstream
	// (404 model tidak tersedia) bocor ke klien sebagai jawaban gateway,
	// padahal sebabnya diketahui pasti sebelum upstream disentuh.
	if !slices.Contains(model.Capabilities, upstream.CapEmbeddings) {
		httpx.WriteError(w, r, http.StatusBadRequest, httpx.ErrTypeInvalidRequest, "embeddings_not_supported",
			"model "+kutip(model.ModelID)+" tidak melayani embeddings")
		return
	}

	clientIP, _ := httpx.ClientIPFrom(r.Context())
	targets := TargetsFor(p, model.ID, clientIP)
	j.pasangKebijakan(p, targets)
	// Embeddings tanpa reservasi (estimasi 0): taksiran token chat tidak
	// berlaku untuk masukan embedding, dan pemeriksaan biasa tetap berjalan.
	if !h.kebijakan(w, r, p, model, targets, 0) {
		return
	}

	rreq := router.Request{
		ModelID:      model.ID,
		ModelName:    req.Model,
		APIKeyID:     p.ID(),
		OwnerUserID:  p.OwnerUserID(),
		Capabilities: []string{upstream.CapEmbeddings},
	}
	cands, dilarangKey, ok := h.kandidat(w, r, rreq, p)
	if !ok {
		return
	}

	keputusan := h.engine.Route(r.Context(), rreq, cands)
	j.pasangKeputusan(keputusan, false)
	if len(keputusan.Candidates) == 0 {
		// Penolakan yang sama seperti jalur chat (audit3 V-GW-43): sebab
		// dibedakan — 403 bila key dilarang ke semua provider, 503 bila
		// memang tidak ada pemetaan, 400 bila aturan menyaring semuanya —
		// bukan satu 503 untuk segala keadaan.
		h.tolakTanpaKandidat(w, r, rreq, model, cands, dilarangKey)
		return
	}
	plan := PlanFromRule(keputusan.Rule, model.ModelID, keputusan.Candidates)

	// Persiapan lolos: eksekusi upstream akan dicoba, kuota sah terbakar.
	jaga.lolos()
	out := Execute(h.guard.DenganMemoBatas(r.Context()), h.exec, plan,
		func(ctx context.Context, c *upstream.RouteCandidate) (*providers.EmbeddingsResponse, error) {
			if perr := h.guard.PeriksaBatasProvider(ctx, c.ProviderID, c.ProviderName); perr != nil {
				return nil, perr
			}
			if perr := h.guard.PeriksaKolomProvider(ctx, c); perr != nil {
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
	h.guard.CatatTokenKolomProvider(r.Context(), out.Candidate, int64(out.Value.Usage.TotalTokens))
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

// modelList adalah envelope daftar model, mengikuti bentuk paginasi OpenAI:
// first_id/last_id menandai tepi halaman yang dikembalikan dan has_more
// menyatakan masih ada entri sesudahnya. Keduanya nil pada daftar kosong.
type modelList struct {
	Object  string       `json:"object"`
	Data    []modelEntry `json:"data"`
	FirstID *string      `json:"first_id"`
	LastID  *string      `json:"last_id"`
	HasMore bool         `json:"has_more"`
}

// maxModelsPerPage membatasi satu halaman daftar model.
const maxModelsPerPage = 500

// daftarAlias adalah kemampuan opsional ModelLister untuk menyebut alias tiap
// model. Dipenuhi *upstream.ModelRepo. Tanpa ini daftar hanya memuat nama
// kanonik — alias adalah tambahan katalog, bukan syarat daftar bisa disusun.
type daftarAlias interface {
	ListAliases(ctx context.Context, modelID string) ([]*upstream.Alias, error)
}

// Models melayani GET /v1/models.
//
// Yang didaftar adalah model yang dilayani GATEWAY INI, dari registry-nya sendiri — bukan
// hasil meneruskan /v1/models ke provider. Bedanya penting: daftar dari provider memuat
// model yang tidak punya pemetaan di sini dan tidak menyebut alias yang justru ditambahkan
// operator, sehingga klien akan meminta nama yang gagal dan tidak menemukan nama yang
// berhasil. Daftar ini juga menghormati pembatasan model pada API key pemanggil.
//
// Sejak audit3 (V-GW-34/35/37) daftar ini menepati tiga janji tambahan:
//
//   - Hanya model yang benar-benar bisa dilayani yang tampil: model tanpa satu
//     pun kandidat rute — yatim tanpa pemetaan provider — tidak lagi terdaftar
//     hanya untuk menjawab 503 saat dipakai, dan untuk key terbatas-provider,
//     model yang seluruh providernya terlarang bagi key itu ikut tersisih.
//   - Alias tampil sebagai entri sendiri, seperti yang dijanjikan dokumen:
//     alias bisa dipakai chat, jadi ia harus bisa ditemukan dari daftar.
//   - Paginasi limit/after_id OpenAI bekerja, dengan first_id/last_id/has_more
//     pada envelope. Tanpa parameter, seluruh katalog dikembalikan seperti
//     sebelumnya — klien yang sudah berjalan tidak berubah perilakunya.
func (h *Handlers) Models(w http.ResponseWriter, r *http.Request) {
	if h.lister == nil {
		// Daftar tidak bisa disusun tanpa sumbernya, dan membalas daftar KOSONG akan
		// membuat klien menyimpulkan gateway ini tidak melayani model apa pun.
		h.logger.ErrorContext(r.Context(), "GET /v1/models dipanggil tanpa ModelLister terpasang")
		httpx.InternalError(w, r)
		return
	}

	limit, afterID, ok := uraikanPaginasiModel(w, r)
	if !ok {
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
	entri, ok := h.entriModelTerlihat(w, r, p, models)
	if !ok {
		return
	}

	// Kanonik dan alias diurutkan bersama menurut id: kursor after_id menunjuk
	// id entri, jadi urutan gabungan yang stabil adalah syarat paginasi yang
	// tidak melewatkan atau mengulang entri di antara dua halaman.
	sort.Slice(entri, func(i, j int) bool { return entri[i].ID < entri[j].ID })

	halaman, hasMore, ok := potongHalamanModel(w, r, entri, limit, afterID)
	if !ok {
		return
	}
	keluaran := modelList{Object: "list", Data: halaman, HasMore: hasMore}
	if len(halaman) > 0 {
		keluaran.FirstID = &halaman[0].ID
		keluaran.LastID = &halaman[len(halaman)-1].ID
	}

	if err := httpx.JSON(w, http.StatusOK, keluaran); err != nil {
		h.logger.DebugContext(r.Context(), "penulisan daftar model gagal", "error", err)
	}
}

// ModelDetail melayani GET /v1/models/{model} (audit3 V-GW-36).
//
// Aturan tampilnya sama persis dengan daftar: nama kanonik maupun alias
// diterima, tetapi model yang tidak akan tampil di daftar — dimatikan, yatim
// tanpa kandidat, atau terlarang bagi key pemanggil — dijawab 404
// model_not_found, bukan diterangkan. Detail yang membocorkan keberadaan
// model yang tidak boleh dipakai key adalah daftar yang berbohong lewat
// pintu lain.
func (h *Handlers) ModelDetail(w http.ResponseWriter, r *http.Request) {
	diminta := chi.URLParam(r, "model")
	tidakDitemukan := func() {
		httpx.WriteError(w, r, http.StatusNotFound, httpx.ErrTypeInvalidRequest, "model_not_found",
			"model "+kutip(diminta)+" tidak dikenal gateway ini; lihat GET /v1/models")
	}

	// Konsisten dengan jalur chat: UUID baris internal bukan nama model.
	if bentukUUID(diminta) {
		tidakDitemukan()
		return
	}
	model, err := h.models.Resolve(r.Context(), diminta)
	if errors.Is(err, repo.ErrNotFound) {
		tidakDitemukan()
		return
	}
	if err != nil {
		h.logger.ErrorContext(r.Context(), "resolusi model untuk detail gagal", "model", diminta, "error", err)
		httpx.InternalError(w, r)
		return
	}
	if !model.Enabled {
		tidakDitemukan()
		return
	}

	p, _ := apikey.PrincipalFrom(r.Context())
	entri, ok := h.entriModelTerlihat(w, r, p, []*upstream.Model{model})
	if !ok {
		return
	}
	for _, e := range entri {
		if e.ID == diminta {
			if err := httpx.JSON(w, http.StatusOK, e); err != nil {
				h.logger.DebugContext(r.Context(), "penulisan detail model gagal", "error", err)
			}
			return
		}
	}
	tidakDitemukan()
}

// entriModelTerlihat menyusun entri katalog (kanonik + alias) untuk model yang
// lolos seluruh saringan keterlihatan bagi key pemanggil. ok=false berarti
// jawaban error sudah ditulis.
func (h *Handlers) entriModelTerlihat(w http.ResponseWriter, r *http.Request, p *apikey.Principal, models []*upstream.Model) ([]modelEntry, bool) {
	ctx := r.Context()
	var aliaser daftarAlias
	if la, ok := h.lister.(daftarAlias); ok {
		aliaser = la
	}

	entri := make([]modelEntry, 0, len(models))
	for _, m := range models {
		boleh, err := h.bolehLihatModel(ctx, p, m)
		if err != nil {
			h.logger.ErrorContext(ctx, "pemeriksaan pembatasan model gagal saat mendaftar",
				"api_key", p.Masked(), "model", m.ModelID, "error", err)
			httpx.InternalError(w, r)
			return nil, false
		}
		if !boleh {
			continue
		}
		bisa, err := h.modelBisaDilayani(ctx, p, m)
		if err != nil {
			h.logger.ErrorContext(ctx, "pemeriksaan kandidat model gagal saat mendaftar",
				"api_key", p.Masked(), "model", m.ModelID, "error", err)
			httpx.InternalError(w, r)
			return nil, false
		}
		if !bisa {
			continue
		}

		e := modelEntry{
			ID:     m.ModelID,
			Object: "model",
			// Unix detik, mengikuti bentuk OpenAI. CreatedAt kami bertipe timestamptz;
			// klien yang mengubahnya menjadi tanggal mengharapkan detik, bukan milidetik.
			Created: m.CreatedAt.Unix(),
			// OwnedBy diisi keluarga model bila ada. Tanpa itu, "routex" lebih jujur
			// daripada menebak nama vendor dari nama model — tebakan seperti itu akan salah
			// tepat pada model dari penyedia yang jarang, yaitu yang paling perlu dikenali.
			OwnedBy: pemilikModel(m),
		}
		entri = append(entri, e)

		if aliaser != nil {
			alias, err := aliaser.ListAliases(ctx, m.ID)
			if err != nil {
				h.logger.ErrorContext(ctx, "pengambilan alias model gagal saat mendaftar",
					"model", m.ModelID, "error", err)
				httpx.InternalError(w, r)
				return nil, false
			}
			for _, a := range alias {
				salinan := e
				salinan.ID = a.Alias
				entri = append(entri, salinan)
			}
		}
	}
	return entri, true
}

// modelBisaDilayani melaporkan apakah model punya setidaknya satu kandidat
// rute yang providernya boleh dipakai key pemanggil.
//
// Kandidat rute adalah sumber kebenaran yang sama dengan jalur chat: model
// tanpa pemetaan provider menghasilkan daftar kandidat kosong di sana dan
// 503 di sini bila tetap didaftarkan (audit3 V-GW-34). Penyaring kebijakan
// dinamis (anggaran, larangan penyaring konten) sengaja TIDAK diterapkan di
// sini — katalog menyatakan kemampuan struktural gateway, bukan keadaan
// anggaran sore ini.
func (h *Handlers) modelBisaDilayani(ctx context.Context, p *apikey.Principal, m *upstream.Model) (bool, error) {
	if h.candidates == nil {
		return true, nil
	}
	cands, err := h.candidates.RouteCandidates(ctx, upstream.RouteQuery{
		ModelID:     m.ID,
		OwnerUserID: p.OwnerUserID(),
	})
	if err != nil {
		return false, err
	}
	if len(cands) == 0 {
		return false, nil
	}
	if h.restrict == nil || p.ID() == "" {
		return true, nil
	}
	for _, c := range cands {
		boleh, err := h.restrict.AllowsProvider(ctx, p.ID(), c.ProviderID)
		if err != nil {
			return false, err
		}
		if boleh {
			return true, nil
		}
	}
	return false, nil
}

// uraikanPaginasiModel membaca limit & after_id gaya OpenAI.
//
// limit yang tidak sah ditolak 400, bukan diabaikan diam-diam: paginasi yang
// rusak tanpa suara adalah bug asal butir ini (audit3 V-GW-37) — parameter
// yang dikirim klien tetapi tidak berlaku membuat klien menyimpulkan katalog
// lebih kecil dari sebenarnya.
func uraikanPaginasiModel(w http.ResponseWriter, r *http.Request) (limit int, afterID string, ok bool) {
	q := r.URL.Query()
	afterID = q.Get("after_id")
	if mentah := q.Get("limit"); mentah != "" {
		n, err := strconv.Atoi(mentah)
		if err != nil || n <= 0 {
			httpx.BadRequest(w, r, httpx.CodeInvalidRequest,
				"parameter limit harus bilangan bulat positif")
			return 0, "", false
		}
		limit = n
	}
	return limit, afterID, true
}

// potongHalamanModel menerapkan after_id & limit pada entri yang sudah
// terurut. after_id yang tidak dikenal ditolak 400: melompat dari kursor
// yang tidak ada hanya bisa berarti halaman yang salah, dan menjawab dari
// awal akan mengulang entri yang sudah diterima klien.
func potongHalamanModel(w http.ResponseWriter, r *http.Request, entri []modelEntry, limit int, afterID string) ([]modelEntry, bool, bool) {
	mulai := 0
	if afterID != "" {
		idx := -1
		for i, e := range entri {
			if e.ID == afterID {
				idx = i
				break
			}
		}
		if idx < 0 {
			httpx.BadRequest(w, r, httpx.CodeInvalidRequest,
				"parameter after_id tidak dikenal dalam katalog model")
			return nil, false, false
		}
		mulai = idx + 1
	}
	sisa := entri[mulai:]
	hasMore := false
	if limit > 0 && len(sisa) > limit {
		sisa = sisa[:limit]
		hasMore = true
	}
	if sisa == nil {
		sisa = []modelEntry{}
	}
	return sisa, hasMore, true
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
