package anthropic

import (
	"context"
	"encoding/json"
	"errors"
	"io"
	"testing"

	"github.com/NexGen-X/Route-X/internal/providers"
)

// Test regresi audit3 grup 4 untuk adapter Anthropic (B-017, B-018, V-LN-42).

// --- V-GW-17: top_k dan thinking eksplisit sampai ke upstream ------------------

func TestTopKDiteruskanKeUpstream(t *testing.T) {
	p, rec := newScriptedServer(t, textResponse)
	req := simpleRequest()
	req.TopK = ptrInt(5)
	if _, err := p.ChatCompletion(context.Background(), req); err != nil {
		t.Fatalf("ChatCompletion: %v", err)
	}
	body := rec.at(t, 0)
	if body.TopK == nil || *body.TopK != 5 {
		t.Errorf("top_k = %v, mau 5 terkirim ke upstream", body.TopK)
	}
}

func TestThinkingEksplisitMenangAtasEffort(t *testing.T) {
	p, rec := newScriptedServer(t, textResponse)
	req := simpleRequest()
	req.Thinking = &providers.ThinkingParam{Type: "enabled", BudgetTokens: 2048}
	req.ReasoningEffort = "low" // akan menjadi 1024 bila effort yang menang
	req.MaxTokens = ptrInt(8192)
	if _, err := p.ChatCompletion(context.Background(), req); err != nil {
		t.Fatalf("ChatCompletion: %v", err)
	}
	th := rec.at(t, 0).Thinking
	if th["type"] != "enabled" || th["budget_tokens"] != float64(2048) {
		t.Errorf("thinking = %v, mau enabled/2048 persis seperti diminta klien", th)
	}
}

func TestThinkingDisabledEksplisitTidakDinyalakanEffort(t *testing.T) {
	p, rec := newScriptedServer(t, textResponse)
	req := simpleRequest()
	req.Thinking = &providers.ThinkingParam{Type: "disabled"}
	req.ReasoningEffort = "high"
	if _, err := p.ChatCompletion(context.Background(), req); err != nil {
		t.Fatalf("ChatCompletion: %v", err)
	}
	if th := rec.at(t, 0).Thinking; len(th) != 0 {
		t.Errorf("thinking = %v, mau tidak ada (klien mematikan secara eksplisit)", th)
	}
}

// --- V-GW-31: temperature + top_p bersamaan -------------------------------------

func TestTemperatureTopPBersamaanDitolak(t *testing.T) {
	p, rec := newScriptedServer(t, textResponse)
	req := simpleRequest()
	req.Temperature = ptrFloat(0.5)
	req.TopP = ptrFloat(0.9)
	_, err := p.ChatCompletion(context.Background(), req)
	var pe *providers.Error
	if !errors.As(err, &pe) || pe.Kind != providers.ErrKindInvalidRequest {
		t.Fatalf("err = %v, mau ErrKindInvalidRequest", err)
	}
	if rec.count() != 0 {
		t.Error("permintaan konflik tetap dikirim ke jaringan; penolakan harus terjadi sebelum upstream disentuh")
	}
}

func TestTemperatureTopPDenganThinkingTidakDitolak(t *testing.T) {
	// Saat thinking menyala, Anthropic menuntut keduanya absen — adapter
	// menstripnya, jadi kombinasi ini sah dan tidak boleh ditolak.
	p, rec := newScriptedServer(t, textResponse)
	req := simpleRequest()
	req.Temperature = ptrFloat(0.5)
	req.TopP = ptrFloat(0.9)
	req.Thinking = &providers.ThinkingParam{Type: "enabled", BudgetTokens: 1024}
	req.MaxTokens = ptrInt(8192)
	if _, err := p.ChatCompletion(context.Background(), req); err != nil {
		t.Fatalf("ChatCompletion: %v", err)
	}
	body := rec.at(t, 0)
	if body.Temperature != nil || body.TopP != nil {
		t.Errorf("temperature/top_p tetap terkirim saat thinking menyala: %v/%v", body.Temperature, body.TopP)
	}
	if len(body.Thinking) == 0 {
		t.Error("thinking tidak terkirim")
	}
}

func TestTopPSendirianDiteruskan(t *testing.T) {
	p, rec := newScriptedServer(t, textResponse)
	req := simpleRequest()
	req.TopP = ptrFloat(0.9)
	if _, err := p.ChatCompletion(context.Background(), req); err != nil {
		t.Fatalf("ChatCompletion: %v", err)
	}
	if tp := rec.at(t, 0).TopP; tp == nil || *tp != 0.9 {
		t.Errorf("top_p = %v, mau 0.9", tp)
	}
}

// --- V-GW-23: thinking riwayat hanya dengan signature ------------------------------

func TestThinkingRiwayatDikirimDenganSignature(t *testing.T) {
	p, rec := newScriptedServer(t, textResponse)
	req := &providers.ChatRequest{
		Model: "claude-sonnet-4-5",
		Messages: []providers.Message{
			{Role: providers.RoleUser, Content: "pikir keras"},
			{
				Role:               providers.RoleAssistant,
				Content:            "jawaban awal",
				ReasoningContent:   "pikir keras",
				ReasoningSignature: "SIG123",
			},
			{Role: providers.RoleUser, Content: "lanjutkan"},
		},
	}
	if _, err := p.ChatCompletion(context.Background(), req); err != nil {
		t.Fatalf("ChatCompletion: %v", err)
	}
	body := rec.at(t, 0)
	var asisten *wireMessage
	for i := range body.Messages {
		if body.Messages[i].Role == "assistant" {
			asisten = &body.Messages[i]
		}
	}
	if asisten == nil || len(asisten.Content) == 0 {
		t.Fatalf("pesan assistant hilang: %+v", body.Messages)
	}
	pertama := asisten.Content[0]
	if pertama.Type != "thinking" || pertama.Thinking != "pikir keras" || pertama.Signature != "SIG123" {
		t.Errorf("block pertama = %+v, mau thinking beserta signature-nya utuh", pertama)
	}
}

func TestThinkingRiwayatTanpaSignatureDibuang(t *testing.T) {
	p, rec := newScriptedServer(t, textResponse)
	req := &providers.ChatRequest{
		Model: "claude-sonnet-4-5",
		Messages: []providers.Message{
			{Role: providers.RoleUser, Content: "pikir keras"},
			{Role: providers.RoleAssistant, Content: "jawaban awal", ReasoningContent: "pikir keras"},
			{Role: providers.RoleUser, Content: "lanjutkan"},
		},
	}
	if _, err := p.ChatCompletion(context.Background(), req); err != nil {
		t.Fatalf("ChatCompletion: %v", err)
	}
	for _, m := range rec.at(t, 0).Messages {
		for _, b := range m.Content {
			if b.Type == "thinking" {
				t.Errorf("thinking telanjang (tanpa signature) terkirim — Anthropic akan menolaknya: %+v", b)
			}
		}
	}
}

// --- V-GW-24: is_error pada hasil tool ----------------------------------------------

func TestToolResultIsErrorDiteruskan(t *testing.T) {
	p, rec := newScriptedServer(t, textResponse)
	req := &providers.ChatRequest{
		Model: "claude-sonnet-4-5",
		Messages: []providers.Message{
			{Role: providers.RoleUser, Content: "pakai alat"},
			{
				Role:      providers.RoleAssistant,
				ToolCalls: []providers.ToolCall{{ID: "toolu_1", Type: "function", Function: providers.FunctionCall{Name: "alat", Arguments: "{}"}}},
			},
			{Role: providers.RoleTool, ToolCallID: "toolu_1", Content: "hasil gagal", IsError: true},
		},
	}
	if _, err := p.ChatCompletion(context.Background(), req); err != nil {
		t.Fatalf("ChatCompletion: %v", err)
	}
	body := rec.at(t, 0)
	terakhir := body.Messages[len(body.Messages)-1]
	if len(terakhir.Content) != 1 || terakhir.Content[0].Type != "tool_result" {
		t.Fatalf("pesan terakhir = %+v, mau satu tool_result", terakhir)
	}
	if !terakhir.Content[0].IsError {
		t.Error("is_error hilang di terjemahan adapter — model akan membaca kegagalan sebagai keberhasilan")
	}
}

// --- V-GW-28: stop_sequence utuh di respons non-stream ---------------------------------

func TestStopSequenceUtuhDiRespons(t *testing.T) {
	p, _ := newScriptedServer(t, `{
	  "id":"msg_stopseq","model":"claude-sonnet-4-5",
	  "content":[{"type":"text","text":"berhenti di urutan"}],
	  "stop_reason":"stop_sequence",
	  "stop_sequence":"\n\nHuman:",
	  "usage":{"input_tokens":10,"output_tokens":4}
	}`)
	resp, err := p.ChatCompletion(context.Background(), simpleRequest())
	if err != nil {
		t.Fatalf("ChatCompletion: %v", err)
	}
	pilihan := resp.Choices[0]
	if pilihan.FinishReason != providers.FinishStopSequence {
		t.Errorf("FinishReason = %q, mau %q (tidak dilipat ke stop)", pilihan.FinishReason, providers.FinishStopSequence)
	}
	if pilihan.StopSequence != "\n\nHuman:" {
		t.Errorf("StopSequence = %q, mau nilai urutannya terbawa", pilihan.StopSequence)
	}
	// Bentuk dialek OpenAI (Raw) tetap memakai kosakata OpenAI: "stop".
	var mentah struct {
		Choices []struct {
			FinishReason string `json:"finish_reason"`
		} `json:"choices"`
	}
	if err := json.Unmarshal(resp.Raw, &mentah); err != nil {
		t.Fatalf("Raw bukan JSON: %v", err)
	}
	if mentah.Choices[0].FinishReason != "stop" {
		t.Errorf("finish_reason dialek OpenAI = %q, mau \"stop\"", mentah.Choices[0].FinishReason)
	}
}

// --- V-GW-32: argumen utuh di content_block_start ---------------------------------------

func TestStreamInputUtuhDiBlockStart(t *testing.T) {
	p, _ := newServer(t, sseHandler(
		"event: message_start\ndata: {\"type\":\"message_start\",\"message\":{\"id\":\"msg_1\",\"model\":\"claude-sonnet-4-5\",\"usage\":{\"input_tokens\":5,\"output_tokens\":1}}}\n\n",
		"event: content_block_start\ndata: {\"type\":\"content_block_start\",\"index\":0,\"content_block\":{\"type\":\"tool_use\",\"id\":\"toolu_1\",\"name\":\"alat\",\"input\":{\"a\":1}}}\n\n",
		"event: content_block_stop\ndata: {\"type\":\"content_block_stop\",\"index\":0}\n\n",
		"event: message_delta\ndata: {\"type\":\"message_delta\",\"delta\":{\"stop_reason\":\"tool_use\"},\"usage\":{\"output_tokens\":3}}\n\n",
		"event: message_stop\ndata: {\"type\":\"message_stop\"}\n\n",
	))
	st, err := p.ChatCompletionStream(context.Background(), simpleRequest())
	if err != nil {
		t.Fatalf("ChatCompletionStream: %v", err)
	}
	defer st.Close()
	got := drain(st)
	if !errors.Is(got.err, io.EOF) {
		t.Fatalf("error akhir = %v, mau io.EOF", got.err)
	}
	if len(got.calls) != 1 {
		t.Fatalf("tool calls = %d, mau 1", len(got.calls))
	}
	if got.calls[0].Function.Arguments != `{"a":1}` {
		t.Errorf("argumen = %q, mau {\"a\":1} — argumen utuh di block_start tidak boleh diabaikan (dulu tool_use ber-input {})", got.calls[0].Function.Arguments)
	}
}

// --- V-GW-28 (stream): stop_sequence terbawa peristiwa penutup ----------------------------

func TestStreamStopSequenceTerbawa(t *testing.T) {
	p, _ := newServer(t, sseHandler(
		"event: message_start\ndata: {\"type\":\"message_start\",\"message\":{\"id\":\"msg_1\",\"model\":\"claude-sonnet-4-5\",\"usage\":{\"input_tokens\":5,\"output_tokens\":1}}}\n\n",
		"event: content_block_start\ndata: {\"type\":\"content_block_start\",\"index\":0,\"content_block\":{\"type\":\"text\",\"text\":\"\"}}\n\n",
		"event: content_block_delta\ndata: {\"type\":\"content_block_delta\",\"index\":0,\"delta\":{\"type\":\"text_delta\",\"text\":\"hai\"}}\n\n",
		"event: content_block_stop\ndata: {\"type\":\"content_block_stop\",\"index\":0}\n\n",
		"event: message_delta\ndata: {\"type\":\"message_delta\",\"delta\":{\"stop_reason\":\"stop_sequence\",\"stop_sequence\":\"TITIK\"},\"usage\":{\"output_tokens\":2}}\n\n",
		"event: message_stop\ndata: {\"type\":\"message_stop\"}\n\n",
	))
	st, err := p.ChatCompletionStream(context.Background(), simpleRequest())
	if err != nil {
		t.Fatalf("ChatCompletionStream: %v", err)
	}
	defer st.Close()
	var penutup *providers.StreamEvent
	for {
		ev, err := st.Recv()
		if errors.Is(err, io.EOF) {
			break
		}
		if err != nil {
			t.Fatalf("Recv: %v", err)
		}
		if ev.FinishReason != "" {
			penutup = ev
		}
	}
	if penutup == nil {
		t.Fatal("tidak ada peristiwa ber-finish_reason")
	}
	if penutup.FinishReason != providers.FinishStopSequence {
		t.Errorf("FinishReason = %q, mau %q", penutup.FinishReason, providers.FinishStopSequence)
	}
	if penutup.StopSequence != "TITIK" {
		t.Errorf("StopSequence = %q, mau TITIK", penutup.StopSequence)
	}
}

// --- V-LN-42: created_at model yang menyimpang ----------------------------------------------

func TestParseWaktuModel(t *testing.T) {
	if got := parseWaktuModel("2025-02-15T10:30:00Z"); got != 1739615400 {
		t.Errorf("RFC3339 = %d, mau 1739615400", got)
	}
	if got := parseWaktuModel("2025-02-15"); got != 1739577600 {
		t.Errorf("tanggal saja = %d, mau 1739577600 (dulu 0 = epoch 1970)", got)
	}
	if got := parseWaktuModel("2025-02-15T10:30:00.123456789Z"); got != 1739615400 {
		t.Errorf("pecahan detik = %d, mau 1739615400", got)
	}
	if got := parseWaktuModel("bukan-tanggal"); got != 0 {
		t.Errorf("sampah = %d, mau 0 (tidak dikenal, bukan dikarang)", got)
	}
}
