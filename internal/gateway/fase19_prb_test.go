package gateway

import (
	"context"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/NexGen-X/Route-X/internal/database/repo/policy"
	"github.com/NexGen-X/Route-X/internal/database/repo/upstream"
	"github.com/NexGen-X/Route-X/internal/providers"
	"github.com/NexGen-X/Route-X/internal/router"
)

// TestB424SignatureNumerikPintuGateway adalah pengunci B-424:
// signature numerik pada riwayat klien (tingkat pesan & tool call
// di codec Chat, dan block thinking di permukaan Messages) dulu
// menggagalkan SELURUH decode dengan 400 generik, karena ketiga
// field itu string biasa. Kini ketiganya SignatureToleran (pola
// B-263): angka menjadi teks literalnya dan decode lolos.
func TestB424SignatureNumerikPintuGateway(t *testing.T) {
	t.Run("chat tingkat pesan dan tool call", func(t *testing.T) {
		req, err := DecodeChatRequest([]byte(`{
			"model": "gemini-2.5-pro",
			"messages": [
				{"role": "user", "content": "hai"},
				{"role": "assistant", "content": "", "thought_signature": 67890,
				 "tool_calls": [{"id": "call_1", "type": "function",
				   "thought_signature": 12345,
				   "function": {"name": "cari", "arguments": "{\"q\":\"x\"}"}}]},
				{"role": "user", "content": "lanjutkan"}
			]
		}`))
		if err != nil {
			t.Fatalf("DecodeChatRequest gagal untuk signature numerik: %v", err)
		}
		if got := req.Messages[1].ReasoningSignature; got != "67890" {
			t.Errorf("ReasoningSignature = %q, mau \"67890\" (teks literal angka)", got)
		}
		if len(req.Messages[1].ToolCalls) != 1 {
			t.Fatalf("tool calls = %d, mau 1", len(req.Messages[1].ToolCalls))
		}
		if got := req.Messages[1].ToolCalls[0].ThoughtSignature; got != "12345" {
			t.Errorf("ThoughtSignature tool call = %q, mau \"12345\"", got)
		}
	})

	t.Run("chat kontrol string tetap utuh", func(t *testing.T) {
		req, err := DecodeChatRequest([]byte(`{
			"model": "gemini-2.5-pro",
			"messages": [
				{"role": "user", "content": "hai"},
				{"role": "assistant", "content": "jawaban", "thought_signature": "sig-teks"}
			]
		}`))
		if err != nil {
			t.Fatalf("DecodeChatRequest: %v", err)
		}
		if got := req.Messages[1].ReasoningSignature; got != "sig-teks" {
			t.Errorf("ReasoningSignature = %q, mau sig-teks", got)
		}
	})

	t.Run("messages block thinking numerik", func(t *testing.T) {
		req, err := DecodeAnthropicMessagesRequest([]byte(`{
			"model": "claude-sonnet-4-5", "max_tokens": 100,
			"messages": [
				{"role": "user", "content": "hai"},
				{"role": "assistant", "content": [
					{"type": "thinking", "thinking": "jejak", "signature": 13579},
					{"type": "text", "text": "jawaban"}
				]},
				{"role": "user", "content": "lanjutkan"}
			]
		}`))
		if err != nil {
			t.Fatalf("DecodeAnthropicMessagesRequest gagal untuk signature numerik: %v", err)
		}
		var ketemu bool
		for _, m := range req.Messages {
			if m.ReasoningSignature != "" {
				ketemu = true
				if m.ReasoningSignature != "13579" {
					t.Errorf("ReasoningSignature = %q, mau \"13579\"", m.ReasoningSignature)
				}
			}
		}
		if !ketemu {
			t.Errorf("tidak ada pesan membawa ReasoningSignature — signature numerik terbuang: %+v", req.Messages)
		}
	})
}

// TestB421HttpUntukMenyaringPesanUpstream adalah pengunci B-421:
// untuk kind InvalidRequest/ModelNotFound/ContentFilter yang
// membawa status HTTP upstream, httpUntuk dulu meneruskan Message
// mentah upstream ke klien. Kini pesannya generik per kind; pesan
// buatan gateway/adapter (tanpa status upstream / Local) tetap
// diteruskan, dan kind lain tidak berubah.
func TestB421HttpUntukMenyaringPesanUpstream(t *testing.T) {
	const penanda = "PENANDA-UPSTREAM-B421 potongan prompt rahasia"
	kasus := []struct {
		kind   providers.ErrorKind
		status int
		code   string
	}{
		{providers.ErrKindInvalidRequest, http.StatusBadRequest, "invalid_request_error"},
		{providers.ErrKindModelNotFound, http.StatusNotFound, "model_not_found"},
		{providers.ErrKindContentFilter, http.StatusBadRequest, "content_filter"},
	}
	for _, k := range kasus {
		e := &providers.Error{Kind: k.kind, StatusCode: 400, Message: penanda}
		status, _, code, msg := httpUntuk(e)
		if status != k.status || code != k.code {
			t.Errorf("kind %s: status/code = %d/%q, mau %d/%q", k.kind, status, code, k.status, k.code)
		}
		if strings.Contains(msg, "PENANDA-UPSTREAM-B421") {
			t.Errorf("kind %s: pesan upstream mentah bocor ke klien: %q", k.kind, msg)
		}
		if msg == "" {
			t.Errorf("kind %s: pesan generik kosong", k.kind)
		}
	}

	// Pesan buatan adapter tanpa status upstream (penolakan
	// terjemahan bernama, pola B-388) tetap sampai ke klien.
	e := &providers.Error{Kind: providers.ErrKindInvalidRequest,
		Message: "pesan adapter bernama B421 tetap sampai"}
	if _, _, _, msg := httpUntuk(e); msg != e.Message {
		t.Errorf("pesan adapter tanpa status = %q, mau diteruskan %q", msg, e.Message)
	}
	// Kegagalan lokal gateway juga diteruskan.
	e = &providers.Error{Kind: providers.ErrKindContentFilter, Local: true, StatusCode: 400,
		Message: "penolakan lokal B421"}
	if _, _, _, msg := httpUntuk(e); msg != e.Message {
		t.Errorf("pesan lokal = %q, mau diteruskan %q", msg, e.Message)
	}
	// Kind lain tidak berubah: rate limit tetap pesan generik lamanya.
	e = &providers.Error{Kind: providers.ErrKindRateLimit, StatusCode: 429, Message: penanda}
	if _, _, _, msg := httpUntuk(e); strings.Contains(msg, "PENANDA") {
		t.Errorf("rate limit: pesan berubah tak terduga: %q", msg)
	}
}

// TestB422CabangOpsiCMenanamCtxTier adalah pengunci B-422: cabang
// sukses pintas Opsi C dulu membuang ctxTier, jadi tanda kuota
// model tier tidak terjangkau dari konteks permintaan. Skenario
// persis W1-K2: model asal TANPA batas cakupan model (tak ada
// pemegang tanda dari jalur utama), tier-2 berbatas dan diterima
// karena jendelanya cukup. Sesudah persiapan, konteks permintaan
// harus memuat tanda tier-2.
func TestB422CabangOpsiCMenanamCtxTier(t *testing.T) {
	m1 := &upstream.Model{ID: "m-t1", ModelID: "tier1-model", Enabled: true}
	m2 := &upstream.Model{ID: "m-t2", ModelID: "tier2-model", Enabled: true}
	models := &petaModel{models: map[string]*upstream.Model{
		"tier1-model": m1, "tier2-model": m2,
	}}
	jendelaKecil := 10
	jendelaBesar := 100000
	kandKecil := kandidatRute("utama", "upstream-t1")
	kandKecil.MaxContextWindow = &jendelaKecil
	kandBesar := kandidatRute("utama", "upstream-t2")
	kandBesar.MaxContextWindow = &jendelaBesar
	cands := &petaKandidat{cands: map[string][]*upstream.RouteCandidate{
		"m-t1": {kandKecil},
		"m-t2": {kandBesar},
	}}
	rule := &upstream.RoutingRule{
		ID:           "r-fase19-b422",
		Name:         "Aturan Tier B-422",
		MatchModelID: ptrString("m-t1"),
		Strategy:     "priority",
		MaxAttempts:  1,
		Enabled:      true,
		Description:  ptrString(`[combo:pipeline=[{"tier":2,"model":"tier2-model"}]]`),
	}
	engine := router.NewEngine(&sumberAturanTiruan{rules: []*upstream.RoutingRule{rule}}, nil, loggerSenyap())
	tiruan := &providerTiruan{
		nama: "utama", kind: providers.KindOpenAI,
		chat: chatSukses("jawaban tier 2"),
	}
	factory := &pabrikTiruan{perNama: map[string]providers.Provider{"utama": tiruan}}
	// Hanya tier-2 yang berbatas cakupan model: model asal sengaja
	// tanpa batas supaya tidak ada pemegang tanda dari jalur utama.
	src := &sumberKebijakan{
		limits: []*policy.RateLimit{{
			Scope: policy.ScopeModel, ScopeID: "m-t2", RequestsPerMinute: pointerG(10), Enabled: true,
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

	body := `{"model":"tier1-model","messages":[{"role":"user","content":"` +
		strings.Repeat("kata pembuka yang cukup panjang ", 30) + `"}]}`
	req, err := DecodeChatRequest([]byte(body))
	if err != nil {
		t.Fatalf("DecodeChatRequest: %v", err)
	}
	r := httptest.NewRequest(http.MethodPost, "/chat/completions", strings.NewReader(body))
	r = r.WithContext(ctxDenganKey("k-b422", "u-b422"))
	w := httptest.NewRecorder()
	pr, ok := h.siapkanChatParsed(w, r, req, []byte(body), &jejak{})
	if !ok {
		t.Fatalf("siapkanChatParsed menolak persiapan (status %d, body %s)", w.Code, w.Body.String())
	}
	if pr.model == nil || pr.model.ID != "m-t2" {
		t.Fatalf("model persiapan = %+v, mau tier-2 (m-t2) lewat Opsi C", pr.model)
	}
	terbakar := SemuaKuotaModelTerbakar(r.Context())
	ketemu := false
	for _, id := range terbakar {
		if id == "m-t2" {
			ketemu = true
		}
	}
	if !ketemu {
		t.Errorf("SemuaKuotaModelTerbakar(r.Context()) = %v, mau memuat m-t2 — ctxTier dibuang cabang Opsi C", terbakar)
	}
}

// TestB423CloseSebelumRecvMencatatSukses adalah pengunci B-423:
// aliran yang intipan peristiwa pertamanya berhasil lalu ditutup
// handler TANPA Recv (jalur gagal PrepareSSE) dulu tercatat
// sebagai KEGAGALAN breaker, karena aliranPencatat.terima tidak
// menghitung peristiwa intipan. Kini intipan dihitung sejak
// konstruksi, jadi Close mencatat sukses.
func TestB423CloseSebelumRecvMencatatSukses(t *testing.T) {
	g := penjaga()
	e, _ := executorUji(g)
	cand := kandidatRute("utama", "upstream-b423")
	plan := rencana("model-b423", 1, 0, cand)
	open := func(ctx context.Context, _ *upstream.RouteCandidate) (providers.Stream, error) {
		return &aliranUji{ctx: ctx}, nil
	}
	bungkus := bungkusPencatatAliran(context.Background(), e, plan, open)
	str, err := bungkus(context.Background(), cand)
	if err != nil {
		t.Fatalf("bungkusPencatatAliran: %v", err)
	}
	// Tanpa satu pun Recv dari konsumen: langsung Close.
	if err := str.Close(); err != nil {
		t.Fatalf("Close: %v", err)
	}
	catatan := g.catatan()
	mau := "prov-utama:true"
	if len(catatan) != 1 || catatan[0] != mau {
		t.Errorf("catatan breaker = %v, mau [%s] — aliran sehat tercatat gagal", catatan, mau)
	}
}
