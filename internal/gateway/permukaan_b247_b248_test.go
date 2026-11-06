package gateway

import (
	"context"
	"encoding/json"
	"net/http"
	"strings"
	"testing"

	"github.com/NexGen-X/Route-X/internal/providers"
)

// TestPermukaanMenolakResponsKosongB247 adalah pengunci B-247: adapter
// yang "berhasil" tanpa pilihan maupun body mentah melanggar kontrak
// providers.Provider. Permukaan chat dan embeddings menjawab keadaan itu
// dengan 500, tetapi Responses mengarang 200 completed ber-output kosong
// dan Messages mengarang 200 end_turn berisi satu block teks kosong.
// Semua permukaan kini menjawab 500 untuk keadaan yang sama.
func TestPermukaanMenolakResponsKosongB247(t *testing.T) {
	prov := providerSukses()
	prov.chat = func(_ context.Context, _ *providers.ChatRequest) (*providers.ChatResponse, error) {
		return &providers.ChatResponse{}, nil
	}
	s := susun(t, prov, nil)

	wr := s.panggil(t, http.MethodPost, "/responses", `{"model":"gpt-4o-mini","input":"halo"}`, nil)
	if wr.Code != http.StatusInternalServerError {
		t.Errorf("responses kosong: status = %d, mau 500 (body: %s)", wr.Code, wr.Body.String())
	}
	if strings.Contains(wr.Body.String(), `"completed"`) {
		t.Errorf("responses kosong: body masih mengarang status completed: %s", wr.Body.String())
	}

	wa := s.panggil(t, http.MethodPost, "/messages",
		`{"model":"gpt-4o-mini","max_tokens":10,"messages":[{"role":"user","content":"halo"}]}`, nil)
	if wa.Code != http.StatusInternalServerError {
		t.Errorf("messages kosong: status = %d, mau 500 (body: %s)", wa.Code, wa.Body.String())
	}
	if !strings.Contains(wa.Body.String(), `"api_error"`) {
		t.Errorf("messages kosong: envelope bukan api_error Anthropic: %s", wa.Body.String())
	}

	// Kontrol konsistensi: permukaan chat memang sudah menjawab 500.
	wc := s.panggil(t, http.MethodPost, "/chat/completions", bodyChatMinimal, nil)
	if wc.Code != http.StatusInternalServerError {
		t.Errorf("chat kosong: status = %d, mau 500 (kontrol konsistensi)", wc.Code)
	}
}

// TestResponsesFinishAsingIncompleteB248 adalah pengunci B-248 sisi
// Responses: finish reason di luar kosakata kanonik sebelumnya dilipat
// menjadi status completed, persis seperti selesai normal. Kini ia
// incomplete dengan nilai aslinya sebagai incomplete_details.reason —
// permukaan chat meneruskan nilai yang sama apa adanya.
func TestResponsesFinishAsingIncompleteB248(t *testing.T) {
	provDenganFinish := func(fr string) *providerTiruan {
		prov := providerSukses()
		prov.chat = func(_ context.Context, _ *providers.ChatRequest) (*providers.ChatResponse, error) {
			return &providers.ChatResponse{
				ID:  "chatcmpl-x",
				Raw: json.RawMessage(`{"id":"chatcmpl-x","choices":[{"index":0,"message":{"role":"assistant","content":"hai"},"finish_reason":"` + fr + `"}]}`),
				Choices: []providers.Choice{{Index: 0,
					Message:      providers.Message{Role: providers.RoleAssistant, Content: "hai"},
					FinishReason: fr}},
			}, nil
		}
		return prov
	}
	baca := func(t *testing.T, fr string) map[string]any {
		t.Helper()
		s := susun(t, provDenganFinish(fr), nil)
		w := s.panggil(t, http.MethodPost, "/responses", `{"model":"gpt-4o-mini","input":"halo"}`, nil)
		if w.Code != http.StatusOK {
			t.Fatalf("finish %q: status = %d, mau 200: %s", fr, w.Code, w.Body.String())
		}
		var obj map[string]any
		if err := json.Unmarshal(w.Body.Bytes(), &obj); err != nil {
			t.Fatalf("finish %q: body bukan JSON: %v", fr, err)
		}
		return obj
	}

	obj := baca(t, "abort_vendor")
	if obj["status"] != "incomplete" {
		t.Errorf("finish asing: status = %v, mau incomplete", obj["status"])
	}
	det, _ := obj["incomplete_details"].(map[string]any)
	if det["reason"] != "abort_vendor" {
		t.Errorf("finish asing: incomplete_details.reason = %v, mau abort_vendor", det["reason"])
	}

	obj = baca(t, providers.FinishPauseTurn)
	if obj["status"] != "incomplete" {
		t.Errorf("pause_turn: status = %v, mau incomplete", obj["status"])
	}
	det, _ = obj["incomplete_details"].(map[string]any)
	if det["reason"] != "pause_turn" {
		t.Errorf("pause_turn: reason = %v, mau pause_turn", det["reason"])
	}

	obj = baca(t, providers.FinishStop)
	if obj["status"] != "completed" {
		t.Errorf("stop: status = %v, mau completed (kontrol)", obj["status"])
	}
}

// TestAnthropicFinishAsingDicatatDiJejakB248 adalah pengunci B-248 sisi
// Anthropic: kosakata stop_reason tertutup, jadi finish asing tetap
// dilaporkan end_turn ke klien — tetapi pelipatannya kini tercatat di
// jejak sebagai peristiwa finish_reason_unknown (migrasi 0025), di jalur
// non-streaming maupun streaming. Finish yang dikenal tidak dicatat.
func TestAnthropicFinishAsingDicatatDiJejakB248(t *testing.T) {
	prov := providerSukses()
	prov.chat = func(_ context.Context, _ *providers.ChatRequest) (*providers.ChatResponse, error) {
		return &providers.ChatResponse{
			ID:  "chatcmpl-x",
			Raw: json.RawMessage(`{"id":"chatcmpl-x","choices":[{"index":0,"message":{"role":"assistant","content":"hai"},"finish_reason":"abort_vendor"}]}`),
			Choices: []providers.Choice{{Index: 0,
				Message:      providers.Message{Role: providers.RoleAssistant, Content: "hai"},
				FinishReason: "abort_vendor"}},
		}, nil
	}
	s, sink := susunDenganPencatat(t, prov, nil)
	w := s.panggil(t, http.MethodPost, "/messages",
		`{"model":"gpt-4o-mini","max_tokens":10,"messages":[{"role":"user","content":"halo"}]}`, nil)
	if w.Code != http.StatusOK {
		t.Fatalf("messages finish asing: status = %d, mau 200: %s", w.Code, w.Body.String())
	}
	if !strings.Contains(w.Body.String(), `"end_turn"`) {
		t.Errorf("messages finish asing: stop_reason bukan end_turn: %s", w.Body.String())
	}
	ev := sink.satu(t)
	ada := false
	for _, n := range ev.Notes {
		if n.Kind == "finish_reason_unknown" && strings.Contains(n.Message, "abort_vendor") {
			ada = true
		}
	}
	if !ada {
		t.Errorf("jejak tidak memuat catatan finish_reason_unknown untuk abort_vendor; notes=%v", ev.Notes)
	}

	// Jalur streaming: peristiwa terakhir membawa finish asing.
	prov2 := providerSukses()
	prov2.alir = func(_ context.Context, _ *providers.ChatRequest) (providers.Stream, error) {
		return &aliranTiruan{ev: []*providers.StreamEvent{
			peristiwa("hai"),
			{FinishReason: "abort_vendor", Usage: &providers.Usage{InputTokens: 2, OutputTokens: 1, TotalTokens: 3}},
		}}, nil
	}
	s2, sink2 := susunDenganPencatat(t, prov2, nil)
	w2 := s2.panggil(t, http.MethodPost, "/messages",
		`{"model":"gpt-4o-mini","max_tokens":10,"stream":true,"messages":[{"role":"user","content":"halo"}]}`, nil)
	if w2.Code != http.StatusOK {
		t.Fatalf("messages stream finish asing: status = %d, mau 200: %s", w2.Code, w2.Body.String())
	}
	ev2 := sink2.satu(t)
	ada = false
	for _, n := range ev2.Notes {
		if n.Kind == "finish_reason_unknown" && strings.Contains(n.Message, "abort_vendor") {
			ada = true
		}
	}
	if !ada {
		t.Errorf("jejak stream tidak memuat catatan finish_reason_unknown; notes=%v", ev2.Notes)
	}

	// Kontrol: finish yang dikenal (stop) tidak menghasilkan catatan.
	s3, sink3 := susunDenganPencatat(t, providerSukses(), nil)
	if w := s3.panggil(t, http.MethodPost, "/messages",
		`{"model":"gpt-4o-mini","max_tokens":10,"messages":[{"role":"user","content":"halo"}]}`, nil); w.Code != http.StatusOK {
		t.Fatalf("messages normal: status = %d, mau 200: %s", w.Code, w.Body.String())
	}
	ev3 := sink3.satu(t)
	for _, n := range ev3.Notes {
		if n.Kind == "finish_reason_unknown" {
			t.Errorf("catatan finish_reason_unknown muncul untuk finish yang dikenal; notes=%v", ev3.Notes)
		}
	}
}
