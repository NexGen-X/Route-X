package gateway

import (
	"context"
	"encoding/json"
	"net/http"
	"strings"
	"testing"

	"github.com/NexGen-X/Route-X/internal/providers"
)

// ssePeristiwa adalah satu blok SSE ("event: nama\ndata: muat\n\n") yang sudah
// diurai dari body respons streaming.
type ssePeristiwa struct {
	nama string
	muat string
}

// uraikanSSE memecah respons SSE menjadi daftar peristiwa. Setiap blok dipisahkan
// baris kosong; hanya baris "event:" dan "data:" yang dibaca, sisanya diabaikan.
func uraikanSSE(t *testing.T, body string) []ssePeristiwa {
	t.Helper()
	var hasil []ssePeristiwa
	for _, blok := range strings.Split(body, "\n\n") {
		if strings.TrimSpace(blok) == "" {
			continue
		}
		var p ssePeristiwa
		for _, baris := range strings.Split(blok, "\n") {
			switch {
			case strings.HasPrefix(baris, "event:"):
				p.nama = strings.TrimSpace(strings.TrimPrefix(baris, "event:"))
			case strings.HasPrefix(baris, "data:"):
				p.muat = strings.TrimSpace(strings.TrimPrefix(baris, "data:"))
			}
		}
		if p.nama != "" {
			hasil = append(hasil, p)
		}
	}
	return hasil
}

// muatJSON mengurai muatan satu peristiwa SSE sebagai peta field.
func muatJSON(t *testing.T, p ssePeristiwa) map[string]any {
	t.Helper()
	var m map[string]any
	if err := json.Unmarshal([]byte(p.muat), &m); err != nil {
		t.Fatalf("muatan peristiwa %q bukan JSON: %v (muat: %s)", p.nama, err, p.muat)
	}
	return m
}

// contentBlock mengambil field "content_block" dari peristiwa content_block_start.
func contentBlock(t *testing.T, p ssePeristiwa) map[string]any {
	t.Helper()
	cb, ok := muatJSON(t, p)["content_block"].(map[string]any)
	if !ok {
		t.Fatalf("peristiwa %q tanpa content_block: %s", p.nama, p.muat)
	}
	return cb
}

// deltaBlok mengambil field "delta" dari peristiwa content_block_delta.
func deltaBlok(t *testing.T, p ssePeristiwa) map[string]any {
	t.Helper()
	d, ok := muatJSON(t, p)["delta"].(map[string]any)
	if !ok {
		t.Fatalf("peristiwa %q tanpa delta: %s", p.nama, p.muat)
	}
	return d
}

// panggilStreamMessages mengirim permintaan /v1/messages streaming melalui router
// sungguhan dan mengembalikan daftar peristiwa SSE yang diterima klien.
func panggilStreamMessages(t *testing.T, aliran providers.Stream) []ssePeristiwa {
	t.Helper()
	prov := &providerTiruan{
		nama: "utama", kind: providers.KindAnthropic,
		alir: func(_ context.Context, _ *providers.ChatRequest) (providers.Stream, error) {
			return aliran, nil
		},
	}
	s := susun(t, prov, nil)
	w := s.panggil(t, http.MethodPost, "/v1/messages", bodyMessagesStreaming, nil)
	if w.Code != http.StatusOK {
		t.Fatalf("status = %d, mau 200 (body: %s)", w.Code, w.Body.String())
	}
	return uraikanSSE(t, w.Body.String())
}

const bodyMessagesStreaming = `{"model":"claude-sonnet-4-5","max_tokens":1024,"messages":[{"role":"user","content":"Bagaimana cuaca di Jakarta dan Bandung?"}],"stream":true}`

// peristiwaToolMulai menyusun StreamEvent pembuka satu tool: id dan nama tanpa
// argumen, persis seperti content_block_start tool_use dari adapter Anthropic.
func peristiwaToolMulai(index int, id, nama string) *providers.StreamEvent {
	return &providers.StreamEvent{
		ToolCalls: []providers.ToolCall{{
			Index: index, ID: id, Type: "function",
			Function: providers.FunctionCall{Name: nama},
		}},
	}
}

// peristiwaToolArgumen menyusun StreamEvent satu potongan argumen tool.
func peristiwaToolArgumen(index int, argumen string) *providers.StreamEvent {
	return &providers.StreamEvent{
		ToolCalls: []providers.ToolCall{{
			Index:    index,
			Function: providers.FunctionCall{Arguments: argumen},
		}},
	}
}

func TestDecodeAnthropicMessagesRequest(t *testing.T) {
	t.Run("basic text message with system", func(t *testing.T) {
		body := []byte(`{
			"model": "claude-3-7-sonnet",
			"system": "You are a helpful assistant.",
			"max_tokens": 1024,
			"messages": [
				{"role": "user", "content": "Hello world"}
			]
		}`)

		req, err := DecodeAnthropicMessagesRequest(body)
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		if req.Model != "claude-3-7-sonnet" {
			t.Errorf("expected model claude-3-7-sonnet, got %s", req.Model)
		}
		if req.MaxTokens == nil || *req.MaxTokens != 1024 {
			t.Errorf("expected max_tokens 1024, got %v", req.MaxTokens)
		}
		if len(req.Messages) != 2 {
			t.Fatalf("expected 2 messages (system + user), got %d", len(req.Messages))
		}
		if req.Messages[0].Role != providers.RoleSystem || req.Messages[0].Content != "You are a helpful assistant." {
			t.Errorf("unexpected system message: %+v", req.Messages[0])
		}
		if req.Messages[1].Role != providers.RoleUser || req.Messages[1].Content != "Hello world" {
			t.Errorf("unexpected user message: %+v", req.Messages[1])
		}
	})

	t.Run("content blocks with text and tool use and tool result", func(t *testing.T) {
		body := []byte(`{
			"model": "claude-3-5-sonnet",
			"messages": [
				{
					"role": "user",
					"content": [
						{"type": "text", "text": "What is the weather?"}
					]
				},
				{
					"role": "assistant",
					"content": [
						{
							"type": "tool_use",
							"id": "toolu_123",
							"name": "get_weather",
							"input": {"location": "Jakarta"}
						}
					]
				},
				{
					"role": "user",
					"content": [
						{
							"type": "tool_result",
							"tool_use_id": "toolu_123",
							"content": "Sunny 30C"
						}
					]
				}
			],
			"tools": [
				{
					"name": "get_weather",
					"description": "Get weather info",
					"input_schema": {
						"type": "object",
						"properties": {
							"location": {"type": "string"}
						}
					}
				}
			]
		}`)

		req, err := DecodeAnthropicMessagesRequest(body)
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		if len(req.Messages) != 3 {
			t.Fatalf("expected 3 messages, got %d", len(req.Messages))
		}
		if req.Messages[0].Content != "What is the weather?" {
			t.Errorf("unexpected message[0]: %s", req.Messages[0].Content)
		}
		if len(req.Messages[1].ToolCalls) != 1 {
			t.Fatalf("expected 1 tool call, got %d", len(req.Messages[1].ToolCalls))
		}
		tc := req.Messages[1].ToolCalls[0]
		if tc.ID != "toolu_123" || tc.Function.Name != "get_weather" {
			t.Errorf("unexpected tool call: %+v", tc)
		}
		if req.Messages[2].Role != providers.RoleTool || req.Messages[2].ToolCallID != "toolu_123" || req.Messages[2].Content != "Sunny 30C" {
			t.Errorf("unexpected tool result message: %+v", req.Messages[2])
		}
		if len(req.Tools) != 1 {
			t.Fatalf("expected 1 tool definition, got %d", len(req.Tools))
		}
		if req.Tools[0].Function.Name != "get_weather" {
			t.Errorf("unexpected tool name: %s", req.Tools[0].Function.Name)
		}
	})

	t.Run("missing model", func(t *testing.T) {
		body := []byte(`{"messages": [{"role": "user", "content": "hi"}]}`)
		_, err := DecodeAnthropicMessagesRequest(body)
		if err == nil {
			t.Fatalf("expected error when model is missing")
		}
	})
}

// TestAnthropicStreamingToolCalls membuktikan bug utama yang diperbaiki: stream
// yang memproduksi tool calls harus mengirim blok tool_use lengkap ke klien
// (content_block_start -> input_json_delta -> content_block_stop), bukan hanya
// stop_reason "tool_use" tanpa blok sama sekali.
func TestAnthropicStreamingToolCalls(t *testing.T) {
	aliran := &aliranTiruan{ev: []*providers.StreamEvent{
		peristiwaToolMulai(0, "toolu_01", "get_weather"),
		// Argumen datang terpisah dalam beberapa potongan, seperti input_json_delta.
		peristiwaToolArgumen(0, `{"city":"`),
		peristiwaToolArgumen(0, `Jakarta"}`),
		// Tool kedua pada index kanonik baru.
		peristiwaToolMulai(1, "toolu_02", "get_weather"),
		peristiwaToolArgumen(1, `{"city":"Bandung"}`),
		{
			FinishReason: providers.FinishToolCalls,
			Usage:        &providers.Usage{InputTokens: 12, OutputTokens: 9, TotalTokens: 21},
		},
	}}
	ps := panggilStreamMessages(t, aliran)

	// Klien Anthropic menolak pesan tanpa blok padahal stop_reason tool_use.
	var mulai, delta, stop int
	for _, p := range ps {
		switch p.nama {
		case "content_block_start":
			cb := contentBlock(t, p)
			if cb["type"] != "tool_use" {
				t.Errorf("content_block_start type = %v, mau tool_use", cb["type"])
			}
			mulai++
			// id dan nama hanya dibawa peristiwa pembuka; input wajib hadir sebagai
			// objek kosong karena argumen menyusul lewat input_json_delta.
			if cb["id"] == nil || cb["name"] == nil {
				t.Errorf("content_block_start tool_use tanpa id/name: %s", p.muat)
			}
			in, ok := cb["input"].(map[string]any)
			if !ok || len(in) != 0 {
				t.Errorf("content_block_start input = %v, mau objek kosong", cb["input"])
			}
		case "content_block_delta":
			d := deltaBlok(t, p)
			if d["type"] != "input_json_delta" {
				t.Errorf("delta type = %v, mau input_json_delta", d["type"])
			}
			if d["partial_json"] == nil {
				t.Errorf("input_json_delta tanpa partial_json: %s", p.muat)
			}
			delta++
		case "content_block_stop":
			stop++
		}
	}
	if mulai != 2 || delta != 3 || stop != 2 {
		t.Fatalf("blok tool_use: start=%d delta=%d stop=%d, mau start=2 delta=3 stop=2 (events: %v)",
			mulai, delta, stop, namaPeristiwa(ps))
	}

	// Urutan blok wajib: setiap start diikuti deltanya lalu stop — blok yang tidak
	// ditutup membuat klien menunggu sampai timeout-nya sendiri. (message_start
	// mendahului seluruh blok, jadi perbandingan dimulai dari peristiwa kedua.)
	urut := namaPeristiwa(ps)
	if got := strings.Join(urut[1:7], ","); got != "content_block_start,content_block_delta,content_block_delta,content_block_stop,content_block_start,content_block_delta" {
		t.Errorf("urutan 6 peristiwa blok pertama salah: %s", got)
	}

	// stop_reason tool_use wajib sampai bersama usage penutup.
	var stopReason any
	for _, p := range ps {
		if p.nama == "message_delta" {
			stopReason = muatJSON(t, p)["delta"].(map[string]any)["stop_reason"]
		}
	}
	if stopReason != "tool_use" {
		t.Errorf("stop_reason = %v, mau tool_use", stopReason)
	}

	// Stream tool-only tidak boleh memancarkan blok text hampa di index 0.
	for _, p := range ps {
		if p.nama == "content_block_start" && contentBlock(t, p)["type"] == "text" {
			t.Errorf("stream tool-only mengirim blok text: %s", p.muat)
		}
	}
}

// TestAnthropicStreamingTeksLaluTool menjaga transisi text -> tool: blok text
// harus ditutup dengan content_block_stop sebelum blok tool_use dibuka, dan
// index blok harus berurutan tanpa celah.
func TestAnthropicStreamingTeksLaluTool(t *testing.T) {
	aliran := &aliranTiruan{ev: []*providers.StreamEvent{
		{Delta: "Memanggil "},
		{Delta: "tool..."},
		peristiwaToolMulai(0, "toolu_07", "get_weather"),
		peristiwaToolArgumen(0, `{"city":"Bandung"}`),
		{FinishReason: providers.FinishToolCalls},
	}}
	ps := panggilStreamMessages(t, aliran)

	// Urutan penuh: text dibuka, dua potongan, ditutup, lalu tool dibuka, satu
	// potongan argumen, ditutup, baru message_delta dan message_stop.
	mau := []string{
		"message_start",
		"content_block_start", "content_block_delta", "content_block_delta", "content_block_stop",
		"content_block_start", "content_block_delta", "content_block_stop",
		"message_delta", "message_stop",
	}
	if got := namaPeristiwa(ps); !samaIrisan(got, mau) {
		t.Errorf("urutan peristiwa salah:\ndapat %v\nmau   %v", got, mau)
	}

	// Index blok harus 0 untuk text lalu 1 untuk tool, tanpa melompat.
	if i := indexBlok(t, ps, "content_block_start", 0); contentBlock(t, ps[i])["type"] != "text" {
		t.Errorf("blok pertama bukan text: %s", ps[i].muat)
	}
	if i := indexBlok(t, ps, "content_block_start", 1); contentBlock(t, ps[i])["type"] != "tool_use" {
		t.Errorf("blok kedua bukan tool_use: %s", ps[i].muat)
	}

	// Potongan text hanya boleh ke index 0, potongan tool hanya ke index 1.
	for _, p := range ps {
		if p.nama != "content_block_delta" {
			continue
		}
		idx := muatJSON(t, p)["index"]
		d := deltaBlok(t, p)
		if d["type"] == "text_delta" && idx != float64(0) {
			t.Errorf("text_delta terkirim ke index %v, mau 0", idx)
		}
		if d["type"] == "input_json_delta" && idx != float64(1) {
			t.Errorf("input_json_delta terkirim ke index %v, mau 1", idx)
		}
	}
}

// TestAnthropicStreamingHanyaTeks adalah penjaga regresi: stream text-only tidak
// boleh mengandung blok tool_use, dan blok text tetap dibuka/ditutup dengan benar.
func TestAnthropicStreamingHanyaTeks(t *testing.T) {
	aliran := &aliranTiruan{ev: []*providers.StreamEvent{
		peristiwa("ha"),
		peristiwa("lo"),
		{FinishReason: providers.FinishStop, Usage: &providers.Usage{InputTokens: 2, OutputTokens: 2, TotalTokens: 4}},
	}}
	ps := panggilStreamMessages(t, aliran)

	mau := []string{
		"message_start",
		"content_block_start", "content_block_delta", "content_block_delta", "content_block_stop",
		"message_delta", "message_stop",
	}
	if got := namaPeristiwa(ps); !samaIrisan(got, mau) {
		t.Errorf("urutan peristiwa salah:\ndapat %v\nmau   %v", got, mau)
	}

	for _, p := range ps {
		switch p.nama {
		case "content_block_start":
			if cb := contentBlock(t, p); cb["type"] != "text" {
				t.Errorf("stream text-only mengirim blok %v: %s", cb["type"], p.muat)
			}
		case "content_block_delta":
			if d := deltaBlok(t, p); d["type"] != "text_delta" {
				t.Errorf("delta stream text-only bertype %v: %s", d["type"], p.muat)
			}
		}
	}

	var stopReason any
	for _, p := range ps {
		if p.nama == "message_delta" {
			stopReason = muatJSON(t, p)["delta"].(map[string]any)["stop_reason"]
		}
	}
	if stopReason != "end_turn" {
		t.Errorf("stop_reason = %v, mau end_turn", stopReason)
	}
}

// namaPeristiwa mengembalikan daftar nama peristiwa SSE secara berurutan.
func namaPeristiwa(ps []ssePeristiwa) []string {
	hasil := make([]string, len(ps))
	for i, p := range ps {
		hasil[i] = p.nama
	}
	return hasil
}

// samaIrisan membandingkan dua slice string elemen demi elemen.
func samaIrisan(got, mau []string) bool {
	if len(got) != len(mau) {
		return false
	}
	for i := range mau {
		if got[i] != mau[i] {
			return false
		}
	}
	return true
}

// indexBlok mengembalikan indeks ps dari peristiwa ke-n dengan nama yang diberikan.
func indexBlok(t *testing.T, ps []ssePeristiwa, nama string, n int) int {
	t.Helper()
	ditemukan := 0
	for i, p := range ps {
		if p.nama == nama {
			if ditemukan == n {
				return i
			}
			ditemukan++
		}
	}
	t.Fatalf("hanya %d peristiwa %q, mencari yang ke-%d", ditemukan, nama, n+1)
	return -1
}
