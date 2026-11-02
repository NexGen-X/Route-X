package gateway

import (
	"context"
	"encoding/json"
	"net/http"
	"strings"
	"testing"

	"github.com/NexGen-X/Route-X/internal/database/repo"
	"github.com/NexGen-X/Route-X/internal/providers"
)

// Berkas ini adalah test regresi audit3 grup 4 (B-017, B-018): fidelity
// permukaan Anthropic. Setiap test menyebut butir verifikasinya; skenario dan
// angka-angkanya diambil dari bukti live di audit3/verifikasi-GW-gateway.md.

// --- B-017 / V-GW-17: parameter sah tidak lagi terbuang saat decode ---------

func TestDecodeAnthropicParameterSahTerbawa(t *testing.T) {
	body := []byte(`{
		"model": "claude-sonnet-4-5",
		"max_tokens": 100,
		"messages": [{"role": "user", "content": "pakai alat"}],
		"tool_choice": {"type": "tool", "name": "alat_x"},
		"thinking": {"type": "enabled", "budget_tokens": 1024},
		"top_k": 5,
		"metadata": {"user_id": "pengguna-42"},
		"tools": [{"name": "alat_x", "input_schema": {"type": "object"}}]
	}`)

	req, err := DecodeAnthropicMessagesRequest(body)
	if err != nil {
		t.Fatalf("DecodeAnthropicMessagesRequest: %v", err)
	}
	if req.TopK == nil || *req.TopK != 5 {
		t.Errorf("TopK = %v, mau 5 (dulu terbuang saat decode)", req.TopK)
	}
	if req.User != "pengguna-42" {
		t.Errorf("User = %q, mau pengguna-42 dari metadata.user_id", req.User)
	}
	if req.Thinking == nil || req.Thinking.Type != "enabled" || req.Thinking.BudgetTokens != 1024 {
		t.Errorf("Thinking = %+v, mau {enabled 1024}", req.Thinking)
	}
	var pilihan struct {
		Type     string `json:"type"`
		Function struct {
			Name string `json:"name"`
		} `json:"function"`
	}
	if err := json.Unmarshal(req.ToolChoice, &pilihan); err != nil {
		t.Fatalf("ToolChoice kanonik bukan JSON sah: %v (%s)", err, req.ToolChoice)
	}
	if pilihan.Type != "function" || pilihan.Function.Name != "alat_x" {
		t.Errorf("ToolChoice = %s, mau pemaksaan function alat_x", req.ToolChoice)
	}
}

func TestDecodeAnthropicToolChoiceVarian(t *testing.T) {
	kasus := map[string]string{
		`{"type":"auto"}`: `"auto"`,
		`{"type":"any"}`:  `"required"`,
		`{"type":"none"}`: `"none"`,
		`{"type":"any", "disable_parallel_tool_use": true}`: `"required"`,
	}
	for mentah, mau := range kasus {
		body := []byte(`{"model":"m","max_tokens":10,"messages":[{"role":"user","content":"x"}],"tool_choice":` + mentah + `}`)
		req, err := DecodeAnthropicMessagesRequest(body)
		if err != nil {
			t.Fatalf("tool_choice %s: %v", mentah, err)
		}
		if string(req.ToolChoice) != mau {
			t.Errorf("tool_choice %s → %s, mau %s", mentah, req.ToolChoice, mau)
		}
	}
	// disable_parallel_tool_use adalah kebalikan parallel_tool_calls.
	body := []byte(`{"model":"m","max_tokens":10,"messages":[{"role":"user","content":"x"}],"tool_choice":{"type":"any","disable_parallel_tool_use":true}}`)
	req, err := DecodeAnthropicMessagesRequest(body)
	if err != nil {
		t.Fatal(err)
	}
	if req.ParallelToolCalls == nil || *req.ParallelToolCalls {
		t.Errorf("ParallelToolCalls = %v, mau false", req.ParallelToolCalls)
	}
	// Tipe yang tidak dikenal ditolak eksplisit, bukan dibuang diam-diam.
	body = []byte(`{"model":"m","max_tokens":10,"messages":[{"role":"user","content":"x"}],"tool_choice":{"type":"kadang"}}`)
	if _, err := DecodeAnthropicMessagesRequest(body); err == nil {
		t.Error("tool_choice.type tak dikenal tidak ditolak")
	}
}

// --- B-017 / V-GW-30: max_tokens wajib ---------------------------------------

func TestDecodeAnthropicMaxTokensWajib(t *testing.T) {
	for _, body := range []string{
		`{"model":"m","messages":[{"role":"user","content":"x"}]}`,
		`{"model":"m","max_tokens":0,"messages":[{"role":"user","content":"x"}]}`,
	} {
		if _, err := DecodeAnthropicMessagesRequest([]byte(body)); err == nil {
			t.Errorf("body tanpa max_tokens sah tidak ditolak: %s", body)
		}
	}
}

// --- B-017 / V-GW-25 + V-GW-24: urutan block dan is_error --------------------

func TestDecodeAnthropicUrutanDanIsErrorToolResult(t *testing.T) {
	body := []byte(`{
		"model": "claude-sonnet-4-5",
		"max_tokens": 100,
		"messages": [
			{"role": "assistant", "content": [{"type": "tool_use", "id": "call_1", "name": "alat", "input": {}}]},
			{"role": "user", "content": [
				{"type": "tool_result", "tool_use_id": "call_1", "content": "hasil gagal", "is_error": true},
				{"type": "text", "text": "lanjutkan"}
			]}
		]
	}`)

	req, err := DecodeAnthropicMessagesRequest(body)
	if err != nil {
		t.Fatalf("DecodeAnthropicMessagesRequest: %v", err)
	}
	if len(req.Messages) != 3 {
		t.Fatalf("pesan = %d, mau 3 (assistant, tool, user): %+v", len(req.Messages), req.Messages)
	}
	hasil := req.Messages[1]
	if hasil.Role != providers.RoleTool || hasil.ToolCallID != "call_1" || hasil.Content != "hasil gagal" {
		t.Errorf("pesan[1] = %+v, mau hasil tool call_1", hasil)
	}
	if !hasil.IsError {
		t.Error("is_error pada tool_result hilang di terjemahan (V-GW-24)")
	}
	if req.Messages[2].Role != providers.RoleUser || req.Messages[2].Content != "lanjutkan" {
		t.Errorf("pesan[2] = %+v, mau teks user SESUDAH hasil tool (urutan terbalik = V-GW-25)", req.Messages[2])
	}
}

// --- B-017 / V-GW-23: thinking riwayat beserta signature ---------------------

func TestDecodeAnthropicThinkingRiwayatTerbawa(t *testing.T) {
	body := []byte(`{
		"model": "claude-sonnet-4-5",
		"max_tokens": 100,
		"messages": [
			{"role": "user", "content": "pikir keras"},
			{"role": "assistant", "content": [
				{"type": "thinking", "thinking": "pikir keras", "signature": "SIG123"},
				{"type": "text", "text": "jawaban awal"}
			]}
		]
	}`)

	req, err := DecodeAnthropicMessagesRequest(body)
	if err != nil {
		t.Fatalf("DecodeAnthropicMessagesRequest: %v", err)
	}
	if len(req.Messages) != 2 {
		t.Fatalf("pesan = %d, mau 2", len(req.Messages))
	}
	as := req.Messages[1]
	if as.ReasoningContent != "pikir keras" {
		t.Errorf("ReasoningContent = %q, mau jejak thinking terbawa", as.ReasoningContent)
	}
	if as.ReasoningSignature != "SIG123" {
		t.Errorf("ReasoningSignature = %q, mau SIG123", as.ReasoningSignature)
	}
	if as.Content != "jawaban awal" {
		t.Errorf("Content = %q", as.Content)
	}
}

// --- B-017 / V-GW-22: block document ------------------------------------------

func TestDecodeAnthropicDokumen(t *testing.T) {
	t.Run("sumber teks diterjemahkan menjadi teks", func(t *testing.T) {
		body := []byte(`{
			"model": "m", "max_tokens": 10,
			"messages": [{"role": "user", "content": [
				{"type": "document", "source": {"type": "text", "media_type": "text/plain", "data": "ISI-DOKUMEN-RAHASIA"}},
				{"type": "text", "text": "ringkas dokumen ini"}
			]}]
		}`)
		req, err := DecodeAnthropicMessagesRequest(body)
		if err != nil {
			t.Fatalf("DecodeAnthropicMessagesRequest: %v", err)
		}
		teks := req.Messages[0].Text()
		if !strings.Contains(teks, "ISI-DOKUMEN-RAHASIA") || !strings.Contains(teks, "ringkas dokumen ini") {
			t.Errorf("isi pesan = %q, mau memuat dokumen dan perintahnya", teks)
		}
	})

	// Batch C2: PDF base64 tidak lagi ditolak di decode — ia menjadi bagian
	// dokumen kanonik yang diteruskan utuh ke upstream Anthropic, dan
	// ditolak di tahap routing bila pemenangnya bukan Anthropic. Yang tetap
	// dilarang di sini adalah sumber yang tidak bisa dijaga kesetiaannya.
	t.Run("sumber base64 PDF menjadi bagian dokumen kanonik", func(t *testing.T) {
		body := []byte(`{
			"model": "m", "max_tokens": 10,
			"messages": [{"role": "user", "content": [
				{"type": "document", "source": {"type": "base64", "media_type": "application/pdf", "data": "JVBERi0="}},
				{"type": "text", "text": "ringkas"}
			]}]
		}`)
		req, err := DecodeAnthropicMessagesRequest(body)
		if err != nil {
			t.Fatalf("DecodeAnthropicMessagesRequest: %v", err)
		}
		msg := req.Messages[0]
		if len(msg.Parts) != 2 {
			t.Fatalf("bagian pesan = %d, mau 2 (dokumen + teks): %+v", len(msg.Parts), msg.Parts)
		}
		dok := msg.Parts[0]
		if dok.Type != providers.PartTypeDocument || dok.Document == nil ||
			dok.Document.MediaType != "application/pdf" || dok.Document.Data != "JVBERi0=" {
			t.Errorf("bagian[0] = %+v, mau dokumen PDF dengan data utuh", dok)
		}
		if msg.Parts[1].Type != providers.PartTypeText || msg.Parts[1].Text != "ringkas" {
			t.Errorf("bagian[1] = %+v, mau teks pendamping sesudah dokumen", msg.Parts[1])
		}
	})

	t.Run("sumber base64 text/plain didekode menjadi teks", func(t *testing.T) {
		body := []byte(`{
			"model": "m", "max_tokens": 10,
			"messages": [{"role": "user", "content": [
				{"type": "document", "source": {"type": "base64", "media_type": "text/plain", "data": "aXNpLXRla3MtZG9rdW1lbg=="}}
			]}]
		}`)
		req, err := DecodeAnthropicMessagesRequest(body)
		if err != nil {
			t.Fatalf("DecodeAnthropicMessagesRequest: %v", err)
		}
		if teks := req.Messages[0].Text(); !strings.Contains(teks, "isi-teks-dokumen") {
			t.Errorf("isi pesan = %q, mau memuat teks hasil dekode base64", teks)
		}
	})

	t.Run("sumber url ditolak eksplisit", func(t *testing.T) {
		body := []byte(`{
			"model": "m", "max_tokens": 10,
			"messages": [{"role": "user", "content": [
				{"type": "document", "source": {"type": "url", "url": "https://contoh.invalid/a.pdf"}}
			]}]
		}`)
		if _, err := DecodeAnthropicMessagesRequest(body); err == nil {
			t.Error("document sumber url tidak ditolak — gateway tidak boleh mengambil berkas dari URL klien")
		}
	})

	t.Run("base64 rusak dan media_type asing ditolak", func(t *testing.T) {
		for nama, sumber := range map[string]string{
			"base64 rusak":    `{"type": "base64", "media_type": "application/pdf", "data": "!!!bukan-base64!!!"}`,
			"media_type lain": `{"type": "base64", "media_type": "image/png", "data": "JVBERi0="}`,
			"sumber file":     `{"type": "file", "file_id": "file_123"}`,
		} {
			body := []byte(`{"model": "m", "max_tokens": 10, "messages": [{"role": "user", "content": [
				{"type": "document", "source": ` + sumber + `}]}]}`)
			if _, err := DecodeAnthropicMessagesRequest(body); err == nil {
				t.Errorf("%s: tidak ditolak", nama)
			}
		}
	})

	t.Run("block tak dikenal ditolak eksplisit", func(t *testing.T) {
		body := []byte(`{
			"model": "m", "max_tokens": 10,
			"messages": [{"role": "user", "content": [{"type": "teleportasi", "data": "x"}]}]
		}`)
		if _, err := DecodeAnthropicMessagesRequest(body); err == nil {
			t.Error("block tak dikenal tidak ditolak")
		}
	})
}

// --- B-017 / V-GW-33: system di tengah diangkat konsisten ---------------------

func TestDecodeAnthropicSystemTengahDiangkat(t *testing.T) {
	body := []byte(`{
		"model": "m", "max_tokens": 10,
		"messages": [
			{"role": "user", "content": "satu"},
			{"role": "system", "content": "instruksi-tengah"},
			{"role": "user", "content": "dua"}
		]
	}`)

	req, err := DecodeAnthropicMessagesRequest(body)
	if err != nil {
		t.Fatalf("DecodeAnthropicMessagesRequest: %v", err)
	}
	if len(req.Messages) != 3 {
		t.Fatalf("pesan = %d, mau 3 (system terangkat + 2 user): %+v", len(req.Messages), req.Messages)
	}
	if req.Messages[0].Role != providers.RoleSystem || req.Messages[0].Content != "instruksi-tengah" {
		t.Errorf("pesan[0] = %+v, mau system terangkat ke depan untuk SEMUA rute", req.Messages[0])
	}
	if req.Messages[1].Content != "satu" || req.Messages[2].Content != "dua" {
		t.Errorf("urutan user berubah: %+v", req.Messages[1:])
	}
}

// --- B-018 / V-GW-29: envelope error persiapan + count_tokens ------------------

func TestAnthropicPersiapanBerEnvelopeAnthropic(t *testing.T) {
	prov := &providerTiruan{nama: "utama", kind: providers.KindAnthropic}
	s := susun(t, prov, func(su *susunan, d *HandlersDeps) {
		su.models.err = repo.ErrNotFound
	})

	w := s.panggil(t, http.MethodPost, "/v1/messages",
		`{"model":"model-ngawur-xyz","max_tokens":10,"messages":[{"role":"user","content":"halo"}]}`, nil)
	if w.Code != http.StatusNotFound {
		t.Fatalf("status = %d, mau 404 (body: %s)", w.Code, w.Body.String())
	}
	var env struct {
		Type  string `json:"type"`
		Error struct {
			Type    string `json:"type"`
			Message string `json:"message"`
		} `json:"error"`
	}
	if err := json.Unmarshal(w.Body.Bytes(), &env); err != nil {
		t.Fatalf("body bukan JSON: %v (%s)", err, w.Body.String())
	}
	if env.Type != "error" || env.Error.Type != "not_found_error" {
		t.Errorf("envelope = %s, mau bentuk Anthropic {\"type\":\"error\",\"error\":{\"type\":\"not_found_error\"}}", w.Body.String())
	}
	if !strings.Contains(env.Error.Message, "model-ngawur-xyz") {
		t.Errorf("pesan error = %q, mau menyebut modelnya", env.Error.Message)
	}
}

func TestAnthropicCountTokens(t *testing.T) {
	prov := &providerTiruan{nama: "utama", kind: providers.KindAnthropic}
	s := susun(t, prov, nil)

	w := s.panggil(t, http.MethodPost, "/v1/messages/count_tokens",
		`{"model":"gpt-4o-mini","messages":[{"role":"user","content":"halo dunia ini prompt uji"}]}`, nil)
	if w.Code != http.StatusOK {
		t.Fatalf("status = %d, mau 200 — rute count_tokens harus ada (body: %s)", w.Code, w.Body.String())
	}
	var resp struct {
		InputTokens int `json:"input_tokens"`
	}
	if err := json.Unmarshal(w.Body.Bytes(), &resp); err != nil {
		t.Fatalf("body bukan JSON: %v", err)
	}
	if resp.InputTokens <= 0 {
		t.Errorf("input_tokens = %d, mau > 0", resp.InputTokens)
	}

	// Model hantu ditolak ber-envelope Anthropic, bukan diberi angka karangan.
	s2 := susun(t, prov, func(su *susunan, d *HandlersDeps) {
		su.models.err = repo.ErrNotFound
	})
	w = s2.panggil(t, http.MethodPost, "/v1/messages/count_tokens",
		`{"model":"hantu","messages":[{"role":"user","content":"halo"}]}`, nil)
	if w.Code != http.StatusNotFound {
		t.Fatalf("count_tokens model hantu: status = %d, mau 404 (body: %s)", w.Code, w.Body.String())
	}
	if !strings.Contains(w.Body.String(), `"type":"error"`) {
		t.Errorf("envelope = %s, mau bentuk Anthropic", w.Body.String())
	}
}

// --- B-018 / V-GW-18: teks + tool_calls dalam SATU peristiwa -------------------

func TestAnthropicStreamTeksDanToolSatuPeristiwa(t *testing.T) {
	aliran := &aliranTiruan{ev: []*providers.StreamEvent{
		{
			Delta: "teks-dan-tool",
			ToolCalls: []providers.ToolCall{{
				Index: 0, ID: "call_1", Type: "function",
				Function: providers.FunctionCall{Name: "alat", Arguments: `{"a":1}`},
			}},
		},
		{FinishReason: providers.FinishToolCalls},
	}}
	ps := panggilStreamMessages(t, aliran)

	var blokTeks, blokTool bool
	var deltaTool string
	for _, p := range ps {
		if p.nama == "content_block_start" {
			switch contentBlock(t, p)["type"] {
			case "text":
				blokTeks = true
			case "tool_use":
				blokTool = true
				if cb := contentBlock(t, p); cb["name"] != "alat" {
					t.Errorf("tool_use name = %v, mau alat", cb["name"])
				}
			}
		}
		if p.nama == "content_block_delta" && deltaBlok(t, p)["type"] == "input_json_delta" {
			deltaTool, _ = deltaBlok(t, p)["partial_json"].(string)
		}
	}
	if !blokTeks || !blokTool {
		t.Fatalf("blok teks=%v tool=%v, mau keduanya ada (tool hilang = V-GW-18); peristiwa: %v",
			blokTeks, blokTool, namaPeristiwa(ps))
	}
	if deltaTool != `{"a":1}` {
		t.Errorf("input_json_delta = %q, mau %q", deltaTool, `{"a":1}`)
	}
}

// --- B-018 / V-GW-19: pola tool→teks→tool dengan index bertabrakan ------------

func TestAnthropicStreamToolTeksTool(t *testing.T) {
	// Kedua tool tiba dengan index kanonik 0 — bentuk yang dihasilkan adapter
	// bila upstream tidak mengirim index dan tiap tool datang pada chunk-nya
	// sendiri. Identitasnya dibedakan ID.
	aliran := &aliranTiruan{ev: []*providers.StreamEvent{
		peristiwaToolMulai(0, "call_1", "alat1"),
		peristiwaToolArgumen(0, `{"x":1}`),
		{Delta: "teks-tengah"},
		peristiwaToolMulai(0, "call_2", "alat2"),
		peristiwaToolArgumen(0, `{"y":2}`),
		{FinishReason: providers.FinishToolCalls},
	}}
	ps := panggilStreamMessages(t, aliran)

	// Susun peta index block → (tipe, nama, gabungan delta).
	type ringkasan struct {
		tipe, nama, delta string
	}
	blok := map[int]*ringkasan{}
	for _, p := range ps {
		m := muatJSON(t, p)
		idx, _ := m["index"].(float64)
		switch p.nama {
		case "content_block_start":
			cb := contentBlock(t, p)
			blok[int(idx)] = &ringkasan{tipe: cb["type"].(string)}
			if cb["type"] == "tool_use" {
				blok[int(idx)].nama, _ = cb["name"].(string)
			}
		case "content_block_delta":
			r, ok := blok[int(idx)]
			if !ok {
				t.Fatalf("delta untuk block %v yang tidak pernah dibuka: %s", idx, p.muat)
			}
			d := deltaBlok(t, p)
			if d["type"] == "input_json_delta" {
				r.delta += d["partial_json"].(string)
			}
			if d["type"] == "text_delta" {
				r.delta += d["text"].(string)
			}
		}
	}

	if b := blok[0]; b == nil || b.tipe != "tool_use" || b.nama != "alat1" || b.delta != `{"x":1}` {
		t.Errorf("blok 0 = %+v, mau tool_use alat1 berargumen {\"x\":1}", b)
	}
	if b := blok[1]; b == nil || b.tipe != "text" || b.delta != "teks-tengah" {
		t.Errorf("blok 1 = %+v, mau text teks-tengah", b)
	}
	if b := blok[2]; b == nil || b.tipe != "tool_use" || b.nama != "alat2" || b.delta != `{"y":2}` {
		t.Errorf("blok 2 = %+v, mau tool_use alat2 berargumen {\"y\":2} (delta ke block tertutup = V-GW-19)", b)
	}
}

// --- B-018 / V-GW-26 + V-GW-27: usage message_delta terkoreksi ------------------

func TestAnthropicStreamUsageMessageDelta(t *testing.T) {
	aliran := &aliranTiruan{ev: []*providers.StreamEvent{
		{Delta: "jawaban"},
		{
			FinishReason: providers.FinishStop,
			// Bentuk kanonik bersarang: input 165 MEMUAT cache read 40 +
			// creation 25. Spec Anthropic menuntut input segar 100.
			Usage: &providers.Usage{
				InputTokens: 165, CachedInputTokens: 40, CacheCreationInputTokens: 25,
				OutputTokens: 10, TotalTokens: 175,
			},
		},
	}}
	ps := panggilStreamMessages(t, aliran)

	for _, p := range ps {
		if p.nama != "message_delta" {
			continue
		}
		u, ok := muatJSON(t, p)["usage"].(map[string]any)
		if !ok {
			t.Fatalf("message_delta tanpa usage: %s", p.muat)
		}
		if u["input_tokens"] != float64(100) {
			t.Errorf("input_tokens = %v, mau 100 (165 mentah = hitung ganda cache, V-GW-26)", u["input_tokens"])
		}
		if u["cache_read_input_tokens"] != float64(40) {
			t.Errorf("cache_read_input_tokens = %v, mau 40", u["cache_read_input_tokens"])
		}
		if u["cache_creation_input_tokens"] != float64(25) {
			t.Errorf("cache_creation_input_tokens = %v, mau 25 (dulu selalu 0)", u["cache_creation_input_tokens"])
		}
		if u["output_tokens"] != float64(10) {
			t.Errorf("output_tokens = %v, mau 10", u["output_tokens"])
		}
		return
	}
	t.Fatal("tidak ada peristiwa message_delta")
}

// --- B-018 / V-EM-08 + B-021 / V-GW-47: error di tengah aliran -------------------

func TestAnthropicStreamErrorTengahAliran(t *testing.T) {
	prov := &providerTiruan{
		nama: "utama", kind: providers.KindAnthropic,
		alir: func(_ context.Context, _ *providers.ChatRequest) (providers.Stream, error) {
			return &aliranTiruan{
				ev:    []*providers.StreamEvent{{Delta: "sebagian "}},
				akhir: providers.Newf(providers.ErrKindRateLimit, "utama", "upstream membatasi laju"),
			}, nil
		},
	}
	s, sink := susunDenganPencatat(t, prov, nil)
	w := s.panggil(t, http.MethodPost, "/v1/messages", bodyMessagesStreaming, nil)
	if w.Code != http.StatusOK {
		t.Fatalf("status = %d, mau 200 (header SSE sudah terkirim)", w.Code)
	}
	ps := uraikanSSE(t, w.Body.String())

	var galat *ssePeristiwa
	for i := range ps {
		if ps[i].nama == "error" {
			galat = &ps[i]
		}
		if ps[i].nama == "message_delta" || ps[i].nama == "message_stop" {
			t.Errorf("aliran gagal tetap ditutup sebagai sukses (%s) — V-GW-47", ps[i].nama)
		}
	}
	if galat == nil {
		t.Fatalf("tidak ada peristiwa error; peristiwa: %v", namaPeristiwa(ps))
	}
	e := muatJSON(t, *galat)["error"].(map[string]any)
	if e["type"] != "rate_limit_error" {
		t.Errorf("error.type = %v, mau rate_limit_error (api_error generik = V-EM-08)", e["type"])
	}

	// Catatan pemakaian harus menyebut kegagalannya: error_type terisi, bukan
	// kosong yang terbaca sebagai sukses di rollup.
	ev := sink.satu(t)
	if ev.ErrorType != string(providers.ErrKindRateLimit) {
		t.Errorf("error_type tercatat = %q, mau %q (kosong = tercatat sukses, V-GW-47)",
			ev.ErrorType, providers.ErrKindRateLimit)
	}
}

// --- B-018 / V-GW-20 + V-GW-21: non-stream Parts dan thinking ---------------------

const bodyMessagesSekali = `{"model":"claude-sonnet-4-5","max_tokens":1024,"messages":[{"role":"user","content":"halo"}]}`

func panggilMessagesSekali(t *testing.T, jawab *providers.ChatResponse) map[string]any {
	t.Helper()
	prov := &providerTiruan{
		nama: "utama", kind: providers.KindAnthropic,
		chat: func(_ context.Context, _ *providers.ChatRequest) (*providers.ChatResponse, error) {
			return jawab, nil
		},
	}
	s := susun(t, prov, nil)
	w := s.panggil(t, http.MethodPost, "/v1/messages", bodyMessagesSekali, nil)
	if w.Code != http.StatusOK {
		t.Fatalf("status = %d, mau 200 (body: %s)", w.Code, w.Body.String())
	}
	var resp map[string]any
	if err := json.Unmarshal(w.Body.Bytes(), &resp); err != nil {
		t.Fatalf("respons bukan JSON: %v", err)
	}
	return resp
}

func TestAnthropicNonStreamPartsDanThinking(t *testing.T) {
	resp := panggilMessagesSekali(t, &providers.ChatResponse{
		ID: "msg_uji",
		Choices: []providers.Choice{{
			Message: providers.Message{
				Role: providers.RoleAssistant,
				Parts: []providers.ContentPart{
					{Type: providers.PartTypeText, Text: "halo-dari-parts"},
				},
				ReasoningContent: "pikir-pikir dulu",
			},
			FinishReason: providers.FinishStop,
		}},
		Usage: providers.Usage{InputTokens: 3, OutputTokens: 5, TotalTokens: 8},
	})

	konten, ok := resp["content"].([]any)
	if !ok || len(konten) != 2 {
		t.Fatalf("content = %v, mau 2 block (thinking + text)", resp["content"])
	}
	berpikir, _ := konten[0].(map[string]any)
	if berpikir["type"] != "thinking" || berpikir["thinking"] != "pikir-pikir dulu" {
		t.Errorf("block[0] = %v, mau thinking berisi jejak penalaran (V-GW-21)", berpikir)
	}
	teks, _ := konten[1].(map[string]any)
	if teks["type"] != "text" || teks["text"] != "halo-dari-parts" {
		t.Errorf("block[1] = %v, mau text %q (kosong = V-GW-20)", teks, "halo-dari-parts")
	}
}

// --- B-018 / V-GW-26 + V-GW-28: non-stream usage dan stop_sequence -----------------

func TestAnthropicNonStreamUsageDanStopSequence(t *testing.T) {
	resp := panggilMessagesSekali(t, &providers.ChatResponse{
		ID: "msg_uji",
		Choices: []providers.Choice{{
			Message: providers.Message{
				Role:    providers.RoleAssistant,
				Content: "berhenti di urutan",
			},
			FinishReason: providers.FinishStopSequence,
			StopSequence: "\n\nHuman:",
		}},
		Usage: providers.Usage{
			InputTokens: 165, CachedInputTokens: 40, CacheCreationInputTokens: 25,
			OutputTokens: 10, TotalTokens: 175,
		},
	})

	if resp["stop_reason"] != "stop_sequence" {
		t.Errorf("stop_reason = %v, mau stop_sequence (dilipat ke end_turn = V-GW-28)", resp["stop_reason"])
	}
	if resp["stop_sequence"] != "\n\nHuman:" {
		t.Errorf("stop_sequence = %v, mau nilai urutannya", resp["stop_sequence"])
	}
	u, _ := resp["usage"].(map[string]any)
	if u["input_tokens"] != float64(100) {
		t.Errorf("input_tokens = %v, mau 100 (bukan 165 bersarang)", u["input_tokens"])
	}
	if u["cache_creation_input_tokens"] != float64(25) {
		t.Errorf("cache_creation_input_tokens = %v, mau 25", u["cache_creation_input_tokens"])
	}
	if u["cache_read_input_tokens"] != float64(40) {
		t.Errorf("cache_read_input_tokens = %v, mau 40", u["cache_read_input_tokens"])
	}
	if u["output_tokens"] != float64(10) {
		t.Errorf("output_tokens = %v, mau 10", u["output_tokens"])
	}
}
