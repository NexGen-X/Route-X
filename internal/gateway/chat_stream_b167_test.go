package gateway

import (
	"context"
	"net/http"
	"testing"

	"github.com/NexGen-X/Route-X/internal/providers"
)

// TestChatStreamLantaiDariKontenBukanFramingB167 adalah pengunci B-167,
// padanan permukaan chat dari TestAnthropicStreamLantaiDariKontenBukanFraming
// (V-AB-15): lantai output streaming WAJIB dihitung dari byte KONTEN, bukan
// byte envelope chat.completion.chunk. Konten "halo" = 4 byte → tepat
// 1 token; menghitung envelope (±97 byte per chunk) menghasilkan ±48.
func TestChatStreamLantaiDariKontenBukanFramingB167(t *testing.T) {
	aliran := &aliranTiruan{ev: []*providers.StreamEvent{
		peristiwa("ha"),
		peristiwa("lo"),
		{FinishReason: providers.FinishStop},
	}}
	prov := &providerTiruan{
		nama: "utama", kind: providers.KindOpenAI,
		alir: func(_ context.Context, _ *providers.ChatRequest) (providers.Stream, error) {
			return aliran, nil
		},
	}
	s, sink := susunDenganPencatat(t, prov, nil)

	w := s.panggil(t, http.MethodPost, "/chat/completions",
		`{"model":"gpt-4o-mini","stream":true,"messages":[{"role":"user","content":"hai"}]}`,
		ctxDenganKey("k-1", "u-1"))
	if w.Code != http.StatusOK {
		t.Fatalf("status = %d, mau 200: %s", w.Code, w.Body.String())
	}

	ev := sink.satu(t)
	if ev.OutputTokens != 1 {
		t.Errorf("output tercatat = %d, mau 1 (4 byte konten / 4), bukan hitungan envelope (B-167)", ev.OutputTokens)
	}
	if ev.InputTokens <= 0 {
		t.Errorf("input tercatat = %d, mau > 0 dari lantai estimasi", ev.InputTokens)
	}
	if ev.TotalTokens != ev.InputTokens+ev.OutputTokens {
		t.Errorf("total = %d, mau input+output = %d", ev.TotalTokens, ev.InputTokens+ev.OutputTokens)
	}
}

// TestChatStreamLantaiMenghitungPenalaranDanArgumenToolB167 melengkapi
// B-167: byte konten mencakup delta penalaran dan argumen tool call —
// keduanya keluaran yang dihasilkan upstream dan ditagih sebagai token
// keluaran. Total konten di sini 2 + 5 + 7 = 14 byte → 3 token.
func TestChatStreamLantaiMenghitungPenalaranDanArgumenToolB167(t *testing.T) {
	aliran := &aliranTiruan{ev: []*providers.StreamEvent{
		{Delta: "ha", ReasoningDelta: "pikir",
			Raw: []byte(`{"id":"c1","object":"chat.completion.chunk","choices":[{"index":0,"delta":{"content":"ha","reasoning_content":"pikir"}}]}`)},
		{ToolCalls: []providers.ToolCall{{Index: 0, ID: "t1", Type: "function",
			Function: providers.FunctionCall{Name: "cek", Arguments: `{"x":1}`}}},
			Raw: []byte(`{"id":"c1","object":"chat.completion.chunk","choices":[{"index":0,"delta":{"tool_calls":[{"index":0,"function":{"arguments":"{\"x\":1}"}}]}}]}`)},
		{FinishReason: providers.FinishStop},
	}}
	prov := &providerTiruan{
		nama: "utama", kind: providers.KindOpenAI,
		alir: func(_ context.Context, _ *providers.ChatRequest) (providers.Stream, error) {
			return aliran, nil
		},
	}
	s, sink := susunDenganPencatat(t, prov, nil)

	w := s.panggil(t, http.MethodPost, "/chat/completions",
		`{"model":"gpt-4o-mini","stream":true,"messages":[{"role":"user","content":"hai"}]}`,
		ctxDenganKey("k-1", "u-1"))
	if w.Code != http.StatusOK {
		t.Fatalf("status = %d, mau 200: %s", w.Code, w.Body.String())
	}

	ev := sink.satu(t)
	if ev.OutputTokens != 3 {
		t.Errorf("output tercatat = %d, mau 3 (14 byte konten: teks 2 + penalaran 5 + argumen tool 7, dibagi 4)", ev.OutputTokens)
	}
}
