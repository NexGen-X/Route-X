package gateway

// Test regresi audit3 grup 3 — "Gateway tangguh" (B-015, B-016, B-022).
//
// Setiap test menunjuk butir verifikasinya (V-GW-*): keadaan-keadaan ini
// terbukti live di v1.5.1, jadi test di sini adalah jangkar supaya
// perbaikannya tidak mundur.

import (
	"context"
	"encoding/json"
	"errors"
	"io"
	"net/http"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/NexGen-X/Route-X/internal/database/repo/upstream"
	"github.com/NexGen-X/Route-X/internal/providers"
)

// --- B-015 / V-GW-11: semua kandidat ter-skip breaker -----------------------

// Executor tidak boleh lagi menghasilkan Outcome kosong (Err nil, Value nil)
// bila semua kandidat dilewati pemutus arus: keadaan itu dibaca pemanggil
// sebagai sukses dan berakhir panic nil-pointer.
func TestExecuteSemuaKandidatTerlewatiPemutusArus(t *testing.T) {
	g := penjaga()
	g.tolak["prov-a"] = true
	g.tolak["prov-b"] = true
	e, _ := executorUji(g)

	out := Execute(context.Background(), e, rencana("m", 3, 0, kandidat("a"), kandidat("b")),
		func(context.Context, *upstream.RouteCandidate) (string, error) {
			t.Fatal("op dipanggil padahal semua kandidat ter-skip")
			return "", nil
		})

	if out.Err == nil {
		t.Fatal("Err = nil — Outcome kosong inilah yang dulu men-panic-kan handler")
	}
	if out.Err.Kind != providers.ErrKindOverloaded {
		t.Errorf("Kind = %q, mau overloaded (dijawab 503)", out.Err.Kind)
	}
	if out.Value != "" {
		t.Errorf("Value = %q, mau kosong", out.Value)
	}
	if !out.TanpaKandidat() {
		t.Error("TanpaKandidat() = false, mau true — tidak satu pun upstream dihubungi")
	}
	status, _, _, _ := httpUntuk(out.Err)
	if status != http.StatusServiceUnavailable {
		t.Errorf("status HTTP = %d, mau 503", status)
	}
}

// Varian streaming: ExecuteStream dengan semua breaker terbuka juga harus
// menghasilkan error, bukan aliran nil yang dibaca sukses.
func TestExecuteStreamSemuaKandidatTerlewatiPemutusArus(t *testing.T) {
	g := penjaga()
	g.tolak["prov-a"] = true
	e, _ := executorUji(g)

	out := ExecuteStream(context.Background(), e, rencana("m", 1, 0, kandidat("a")),
		func(context.Context, *upstream.RouteCandidate) (providers.Stream, error) {
			t.Fatal("open dipanggil padahal kandidat ter-skip")
			return nil, nil
		})

	if out.Err == nil {
		t.Fatal("Err = nil pada ExecuteStream — handler akan men-dereference aliran nil")
	}
	if out.Err.Kind != providers.ErrKindOverloaded {
		t.Errorf("Kind = %q, mau overloaded", out.Err.Kind)
	}
	if out.Value != nil {
		t.Error("Value bukan nil pada kegagalan total")
	}
}

// Kedua permukaan menjawab 503 yang rapi — bukan panic 500 — saat semua
// pemutus arus terbuka.
func TestPermukaanMenjawab503SaatSemuaBreakerTerbuka(t *testing.T) {
	susunDenganBreakerTerbuka := func(t *testing.T, prov providers.Provider) *susunan {
		t.Helper()
		return susun(t, prov, func(_ *susunan, d *HandlersDeps) {
			g := penjaga()
			g.tolak["prov-utama"] = true
			d.Executor = NewExecutor(g, loggerSenyap(),
				WithSleepFunc(func(context.Context, time.Duration) error { return nil }))
		})
	}

	t.Run("openai", func(t *testing.T) {
		s := susunDenganBreakerTerbuka(t, providerSukses())
		w := s.panggil(t, http.MethodPost, "/chat/completions", bodyChatMinimal, nil)
		if w.Code != http.StatusServiceUnavailable {
			t.Fatalf("status = %d, mau 503 (body: %s)", w.Code, w.Body.String())
		}
		if got := kodeGalat(galat(t, w)); got != "upstream_overloaded" {
			t.Errorf("code = %q, mau upstream_overloaded", got)
		}
	})

	t.Run("anthropic", func(t *testing.T) {
		s := susunDenganBreakerTerbuka(t, providerSukses())
		w := s.panggil(t, http.MethodPost, "/v1/messages",
			`{"model":"gpt-4o-mini","max_tokens":100,"messages":[{"role":"user","content":"halo"}]}`, nil)
		if w.Code != http.StatusServiceUnavailable {
			t.Fatalf("status = %d, mau 503 (body: %s)", w.Code, w.Body.String())
		}
	})
}

// --- B-015 / V-GW-48: panic tercatat sebagai 500, bukan 499 -----------------

// Provider yang panic menembus handler; jejak yang berjalan lewat defer saat
// tumpukan mengurai harus mencatat 500 internal_error — yang benar-benar
// diterima klien dari middleware Recover — bukan 499 "klien menutup".
func TestJejakMencatatPanicSebagai500(t *testing.T) {
	prov := &providerTiruan{
		nama: "utama", kind: providers.KindOpenAI,
		chat: func(context.Context, *providers.ChatRequest) (*providers.ChatResponse, error) {
			panic("ledakan uji")
		},
	}
	s, sink := susunDenganPencatat(t, prov, nil)

	// Recover middleware di produksi menangkap panic ini dan menjawab 500;
	// di sini ia ditangkap test supaya tumpukan tidak menelan proses test.
	func() {
		defer func() { _ = recover() }()
		s.panggil(t, http.MethodPost, "/chat/completions", bodyChatMinimal, nil)
	}()

	ev := sink.satu(t)
	if ev.StatusCode != http.StatusInternalServerError {
		t.Errorf("status tercatat = %d, mau 500 (bukan 499 canceled)", ev.StatusCode)
	}
	if ev.ErrorCode != "internal_error" {
		t.Errorf("error_code tercatat = %q, mau internal_error", ev.ErrorCode)
	}
	if ev.ErrorType != "internal" {
		t.Errorf("error_type tercatat = %q, mau internal", ev.ErrorType)
	}
}

// --- B-016 / V-GW-12: kegagalan Recv pertama dialihkan ----------------------

// Aliran yang gagal pada Recv PERTAMA — belum ada byte ke klien — harus
// dialihkan ke kandidat berikutnya, bukan diserahkan ke klien sebagai
// satu-satunya jawaban.
func TestExecuteStreamKegagalanRecvPertamaDialihkan(t *testing.T) {
	e, _ := executorUji(penjaga())

	aliranGagal := &aliranTiruan{
		akhir: providers.Newf(providers.ErrKindOverloaded, "a", "penuh"),
	}
	aliranSehat := &aliranTiruan{
		ev: []*providers.StreamEvent{peristiwa("selamat")},
	}

	out := ExecuteStream(context.Background(), e, rencana("m", 1, 0, kandidat("a"), kandidat("b")),
		func(_ context.Context, c *upstream.RouteCandidate) (providers.Stream, error) {
			if c.ProviderName == "a" {
				return aliranGagal, nil
			}
			return aliranSehat, nil
		})

	if out.Err != nil {
		t.Fatalf("Err = %v, mau nil — failover Recv pertama tidak berjalan", out.Err)
	}
	if out.Candidate == nil || out.Candidate.ProviderName != "b" {
		t.Fatalf("Candidate = %v, mau b", out.Candidate)
	}
	if !aliranGagal.ditutup {
		t.Error("aliran yang gagal pada Recv pertama tidak ditutup")
	}

	// Peristiwa pertama kandidat sehat adalah yang ditahan intipan; Recv
	// pertama pemanggil harus menerimanya, lalu EOF bersih.
	ev, err := out.Value.Recv()
	if err != nil {
		t.Fatalf("Recv pertama = %v", err)
	}
	if ev.Delta != "selamat" {
		t.Errorf("Delta = %q, mau selamat", ev.Delta)
	}
	if _, err := out.Value.Recv(); !errors.Is(err, io.EOF) {
		t.Errorf("Recv kedua = %v, mau io.EOF", err)
	}
}

// Aliran yang menutup rapi tanpa satu pun peristiwa bukan jawaban: ia
// kegagalan pembukaan yang boleh dialihkan.
func TestExecuteStreamAliranKosongDialihkan(t *testing.T) {
	e, _ := executorUji(penjaga())

	out := ExecuteStream(context.Background(), e, rencana("m", 1, 0, kandidat("a"), kandidat("b")),
		func(_ context.Context, c *upstream.RouteCandidate) (providers.Stream, error) {
			if c.ProviderName == "a" {
				return &aliranTiruan{}, nil // langsung io.EOF, tanpa peristiwa
			}
			return &aliranTiruan{ev: []*providers.StreamEvent{peristiwa("isi")}}, nil
		})

	if out.Err != nil {
		t.Fatalf("Err = %v, mau nil", out.Err)
	}
	if out.Candidate == nil || out.Candidate.ProviderName != "b" {
		t.Fatalf("Candidate = %v, mau b — aliran kosong tidak dialihkan", out.Candidate)
	}
}

// Ujung-ke-ujung di permukaan OpenAI: kegagalan aliran pertama pada Recv
// pertama diselamatkan kandidat kedua, dan SSE ke klien utuh.
func TestChatMengalirKegagalanRecvPertamaDialihkan(t *testing.T) {
	provGagal := &providerTiruan{
		nama: "gagal", kind: providers.KindOpenAI,
		alir: func(context.Context, *providers.ChatRequest) (providers.Stream, error) {
			return &aliranTiruan{
				akhir: providers.Newf(providers.ErrKindOverloaded, "gagal", "penuh"),
			}, nil
		},
	}
	provSehat := &providerTiruan{
		nama: "sehat", kind: providers.KindOpenAI,
		alir: func(context.Context, *providers.ChatRequest) (providers.Stream, error) {
			return &aliranTiruan{ev: []*providers.StreamEvent{peristiwa("terselamatkan")}}, nil
		},
	}
	s := susun(t, provSehat, func(s *susunan, _ *HandlersDeps) {
		s.cands.cands = []*upstream.RouteCandidate{
			kandidatRute("gagal", "gpt-4o-mini-2024"),
			kandidatRute("sehat", "gpt-4o-mini-2024"),
		}
		s.factory.perNama = map[string]providers.Provider{
			"gagal": provGagal,
			"sehat": provSehat,
		}
	})

	w := s.panggil(t, http.MethodPost, "/chat/completions",
		`{"model":"gpt-4o-mini","stream":true,"messages":[{"role":"user","content":"halo"}]}`, nil)
	if w.Code != http.StatusOK {
		t.Fatalf("status = %d, mau 200 (body: %s)", w.Code, w.Body.String())
	}
	if !strings.Contains(w.Body.String(), "terselamatkan") {
		t.Errorf("SSE tidak memuat chunk kandidat sehat: %s", w.Body.String())
	}
	if !strings.Contains(w.Body.String(), "[DONE]") {
		t.Errorf("SSE tidak ditutup [DONE]: %s", w.Body.String())
	}
}

// --- B-016 / V-GW-16: race tenggat pembukaan ---------------------------------

// Aliran yang "berhasil dibuka" tepat saat tenggat pembukaan habis tidak
// boleh dipakai: konteksnya sudah dibatalkan pengawas. Ia harus dibuang dan
// dilaporkan sebagai timeout, lalu kandidat berikutnya dicoba.
func TestExecuteStreamPembukaanBersamaanTenggatDibuang(t *testing.T) {
	e, _ := executorUji(penjaga())

	lambat := kandidat("lambat", func(c *upstream.RouteCandidate) { c.TimeoutMS = 1 })
	out := ExecuteStream(context.Background(), e, rencana("m", 1, 0, lambat, kandidat("cepat")),
		func(cctx context.Context, c *upstream.RouteCandidate) (providers.Stream, error) {
			if c.ProviderName == "lambat" {
				// Mengabaikan konteks dan tetap kembali membawa aliran
				// SETELAH tenggat lewat — persis race yang dilaporkan.
				time.Sleep(15 * time.Millisecond)
				return &aliranTiruan{ev: []*providers.StreamEvent{peristiwa("terlambat")}}, nil
			}
			return &aliranTiruan{ev: []*providers.StreamEvent{peristiwa("cepat")}}, nil
		})

	if out.Err != nil {
		t.Fatalf("Err = %v, mau nil — kandidat cepat harus menyelamatkan", out.Err)
	}
	if out.Candidate == nil || out.Candidate.ProviderName != "cepat" {
		t.Fatalf("Candidate = %v, mau cepat — aliran kedaluwarsa tenggat tetap dipakai", out.Candidate)
	}
}

// --- B-016 / V-GW-14: penjaga diam aliran ------------------------------------

// aliranBisu menggantung pada Recv sampai ditutup, seperti upstream yang
// mengirim satu chunk lalu membisu selamanya.
type aliranBisu struct {
	tutup  chan struct{}
	sekali sync.Once
}

func (a *aliranBisu) Recv() (*providers.StreamEvent, error) {
	<-a.tutup
	return nil, providers.Newf(providers.ErrKindCanceled, "uji", "aliran ditutup")
}

func (a *aliranBisu) Close() error {
	a.sekali.Do(func() { close(a.tutup) })
	return nil
}

func TestPenjagaDiamMematikanAliranYangMembisu(t *testing.T) {
	dijaga := &aliranPenjagaDiam{Stream: &aliranBisu{tutup: make(chan struct{})}, batas: 30 * time.Millisecond}
	dijaga.mulai()

	mulai := time.Now()
	_, err := dijaga.Recv()
	if time.Since(mulai) > 5*time.Second {
		t.Fatal("Recv tidak kembali — penjaga diam tidak menembak")
	}
	var pe *providers.Error
	if !errors.As(err, &pe) {
		t.Fatalf("err = %v, mau *providers.Error", err)
	}
	if pe.Kind != providers.ErrKindTimeout {
		t.Errorf("Kind = %q, mau timeout — kematian upstream tidak boleh tercatat canceled", pe.Kind)
	}
}

func TestPenjagaDiamTidakMenggangguAliranHidup(t *testing.T) {
	// Lima peristiwa berjarak 10ms dengan batas diam 250ms: total umur
	// aliran melewati batas, tetapi tidak satu pun jeda yang melewatinya.
	lambat := &aliranBerjeda{jeda: 10 * time.Millisecond, sisa: 5}
	dijaga := &aliranPenjagaDiam{Stream: lambat, batas: 250 * time.Millisecond}
	dijaga.mulai()

	for i := 0; i < 5; i++ {
		if _, err := dijaga.Recv(); err != nil {
			t.Fatalf("Recv ke-%d = %v, mau nil", i+1, err)
		}
	}
	if _, err := dijaga.Recv(); !errors.Is(err, io.EOF) {
		t.Errorf("Recv penutup = %v, mau io.EOF bersih", err)
	}
}

// aliranBerjeda mengirim sejumlah peristiwa berjeda tetap lalu EOF.
type aliranBerjeda struct {
	jeda time.Duration
	sisa int
}

func (a *aliranBerjeda) Recv() (*providers.StreamEvent, error) {
	if a.sisa <= 0 {
		return nil, io.EOF
	}
	time.Sleep(a.jeda)
	a.sisa--
	return &providers.StreamEvent{Delta: "x", Raw: []byte(`{"x":1}`)}, nil
}

func (a *aliranBerjeda) Close() error { return nil }

// --- B-022 / V-GW-49: teksPermintaan mencakup tool & definisi ---------------

func TestTeksPermintaanMencakupToolCallsDanDefinisiTools(t *testing.T) {
	req := &providers.ChatRequest{
		Messages: []providers.Message{
			{
				Role: providers.RoleUser,
				Parts: []providers.ContentPart{
					{Type: providers.PartTypeText, Text: "tolong lihat gambar ini"},
					{Type: "image_url", ImageURL: &providers.ImageURL{URL: "https://contoh.invalid/TANDA-URL"}},
				},
			},
			{
				Role: providers.RoleAssistant,
				ToolCalls: []providers.ToolCall{{
					Type:     "function",
					Function: providers.FunctionCall{Name: "cari", Arguments: `{"q":"TANDA-ARGUMEN"}`},
				}},
			},
		},
		Tools: []providers.Tool{{
			Type: "function",
			Function: providers.FunctionDef{
				Name:        "alat_cari",
				Description: "mencari TANDA-DESKRIPSI di web",
				Parameters:  json.RawMessage(`{"type":"object","description":"TANDA-SKEMA"}`),
			},
		}},
	}

	teks := teksPermintaan(req)
	for _, tanda := range []string{"TANDA-ARGUMEN", "TANDA-DESKRIPSI", "TANDA-SKEMA", "TANDA-URL", "tolong lihat gambar ini"} {
		if !strings.Contains(teks, tanda) {
			t.Errorf("teksPermintaan tidak memuat %q — penyaring konten buta di permukaan itu\nteks: %s", tanda, teks)
		}
	}
}

// --- B-015 / V-GW-43: embeddings membedakan sebab kandidat kosong -----------

func TestEmbeddingsModelBukanEmbeddingsDitolak400(t *testing.T) {
	// Model uji bawaan harness tidak menyatakan kemampuan embeddings.
	s := susun(t, providerSukses(), nil)
	w := s.panggil(t, http.MethodPost, "/embeddings", `{"model":"gpt-4o-mini","input":"halo"}`, nil)
	if w.Code != http.StatusBadRequest {
		t.Fatalf("status = %d, mau 400 (body: %s)", w.Code, w.Body.String())
	}
	if got := kodeGalat(galat(t, w)); got != "embeddings_not_supported" {
		t.Errorf("code = %q, mau embeddings_not_supported", got)
	}
}

func TestEmbeddingsModelYatimDijawab503(t *testing.T) {
	s := susun(t, providerSukses(), func(s *susunan, _ *HandlersDeps) {
		s.models.model.Capabilities = []string{upstream.CapEmbeddings}
		s.cands.cands = nil // model embeddings tanpa satu pun pemetaan
	})
	w := s.panggil(t, http.MethodPost, "/embeddings", `{"model":"gpt-4o-mini","input":"halo"}`, nil)
	if w.Code != http.StatusServiceUnavailable {
		t.Fatalf("status = %d, mau 503 (body: %s)", w.Code, w.Body.String())
	}
	if got := kodeGalat(galat(t, w)); got != "no_provider_for_model" {
		t.Errorf("code = %q, mau no_provider_for_model", got)
	}
}

func TestEmbeddingsKeyDilarangDijawab403(t *testing.T) {
	s := susun(t, providerSukses(), func(s *susunan, d *HandlersDeps) {
		s.models.model.Capabilities = []string{upstream.CapEmbeddings}
		s.batas = &pembatasKey{model: true, provider: false}
		d.Restrict = s.batas
	})
	w := s.panggil(t, http.MethodPost, "/embeddings", `{"model":"gpt-4o-mini","input":"halo"}`, ctxDenganKey("k-1", "u-1"))
	if w.Code != http.StatusForbidden {
		t.Fatalf("status = %d, mau 403 (body: %s)", w.Code, w.Body.String())
	}
	if got := kodeGalat(galat(t, w)); got != "provider_not_allowed" {
		t.Errorf("code = %q, mau provider_not_allowed", got)
	}
}
