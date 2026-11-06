package openai

import (
	"context"
	"encoding/json"
	"errors"
	"io"
	"strings"
	"testing"

	"github.com/NexGen-X/Route-X/internal/providers"
)

// TestB213PenolakanNonStream membuktikan B-213 di jalur non-streaming:
// field refusal upstream kini sampai ke bentuk kanonik (Message.Refusal)
// dan finish_reason "stop"-nya dipetakan ke content_filter kanonik —
// sementara Raw (yang diterima klien OpenAI) tidak berubah satu byte pun.
// Di kode lama struct wire tidak punya field refusal sama sekali.
func TestB213PenolakanNonStream(t *testing.T) {
	body := `{"id":"c1","model":"m","created":1,` +
		`"choices":[{"index":0,"message":{"role":"assistant","content":null,` +
		`"refusal":"Saya tidak bisa membantu permintaan itu."},"finish_reason":"stop"}],` +
		`"usage":{"prompt_tokens":10,"completion_tokens":5,"total_tokens":15}}`
	u := newUpstream(t, jsonHandler(200, body))
	p := testProvider(t, u.url(), nil)

	resp, err := p.ChatCompletion(context.Background(), &providers.ChatRequest{
		Model:    "m",
		Messages: []providers.Message{{Role: providers.RoleUser, Content: "halo"}},
	})
	if err != nil {
		t.Fatalf("ChatCompletion: %v", err)
	}
	pilihan := resp.Choices[0]
	if pilihan.Message.Refusal != "Saya tidak bisa membantu permintaan itu." {
		t.Errorf("Refusal = %q, ingin teks penolakan upstream", pilihan.Message.Refusal)
	}
	if pilihan.FinishReason != providers.FinishContentFilter {
		t.Errorf("FinishReason kanonik = %q, ingin %q untuk penolakan", pilihan.FinishReason, providers.FinishContentFilter)
	}
	if string(resp.Raw) != body {
		t.Errorf("Raw berubah dari body upstream:\ndapat %s\nmau   %s", resp.Raw, body)
	}
}

// TestB213AudioTranscriptNonStream membuktikan transkrip keluaran audio
// sampai ke Message.AudioTranscript; data audio mentahnya tetap urusan
// Raw permukaan OpenAI.
func TestB213AudioTranscriptNonStream(t *testing.T) {
	body := `{"id":"c1","model":"m","created":1,` +
		`"choices":[{"index":0,"message":{"role":"assistant","content":null,` +
		`"audio":{"id":"a1","data":"QUJD","expires_at":1899999999,"transcript":"halo, ini suara saya"}},` +
		`"finish_reason":"stop"}]}`
	u := newUpstream(t, jsonHandler(200, body))
	p := testProvider(t, u.url(), nil)

	resp, err := p.ChatCompletion(context.Background(), &providers.ChatRequest{
		Model:    "m",
		Messages: []providers.Message{{Role: providers.RoleUser, Content: "halo"}},
	})
	if err != nil {
		t.Fatalf("ChatCompletion: %v", err)
	}
	if got := resp.Choices[0].Message.AudioTranscript; got != "halo, ini suara saya" {
		t.Errorf("AudioTranscript = %q, ingin transkrip upstream", got)
	}
}

// TestB213PenolakanStream membuktikan B-213 di jalur streaming: potongan
// refusal menjadi RefusalDelta kanonik, dan chunk penutup ber-finish
// "stop" dipetakan ke content_filter — Raw chunk-nya tetap apa adanya.
func TestB213PenolakanStream(t *testing.T) {
	u := newUpstream(t, sseHandler(
		`{"id":"c1","model":"m","created":1,"choices":[{"index":0,"delta":{"role":"assistant","refusal":"Saya tidak "},"finish_reason":null}]}`,
		`{"id":"c1","model":"m","created":1,"choices":[{"index":0,"delta":{"refusal":"bisa."},"finish_reason":null}]}`,
		`{"id":"c1","model":"m","created":1,"choices":[{"index":0,"delta":{},"finish_reason":"stop"}]}`,
		`[DONE]`,
	))
	p := testProvider(t, u.url(), nil)

	st, err := p.ChatCompletionStream(context.Background(), &providers.ChatRequest{
		Model:    "m",
		Messages: []providers.Message{{Role: providers.RoleUser, Content: "halo"}},
	})
	if err != nil {
		t.Fatalf("ChatCompletionStream: %v", err)
	}
	defer st.Close()

	var refusal, finish string
	var rawPenutup []byte
	for {
		ev, err := st.Recv()
		if errors.Is(err, io.EOF) {
			break
		}
		if err != nil {
			t.Fatalf("Recv: %v", err)
		}
		refusal += ev.RefusalDelta
		if ev.FinishReason != "" {
			finish = ev.FinishReason
			rawPenutup = ev.Raw
		}
	}
	if refusal != "Saya tidak bisa." {
		t.Errorf("gabungan RefusalDelta = %q, ingin \"Saya tidak bisa.\"", refusal)
	}
	if finish != providers.FinishContentFilter {
		t.Errorf("FinishReason kanonik penutup = %q, ingin %q", finish, providers.FinishContentFilter)
	}
	if !strings.Contains(string(rawPenutup), `"finish_reason":"stop"`) {
		t.Errorf("Raw chunk penutup berubah: %s", rawPenutup)
	}
}

// TestB213RefusalRiwayatDikirimBalik membuktikan arah sebaliknya: pesan
// asisten ber-Refusal pada riwayat ditulis kembali ke wire sebagai field
// refusal, bukan hilang atau dilipat ke content.
func TestB213RefusalRiwayatDikirimBalik(t *testing.T) {
	wire := messagesToWire([]providers.Message{
		{Role: providers.RoleUser, Content: "halo"},
		{Role: providers.RoleAssistant, Refusal: "Saya tidak bisa membantu itu."},
	})
	raw, err := json.Marshal(wire[1])
	if err != nil {
		t.Fatalf("Marshal: %v", err)
	}
	if !strings.Contains(string(raw), `"refusal":"Saya tidak bisa membantu itu."`) {
		t.Errorf("wire pesan asisten = %s, ingin memuat field refusal", raw)
	}
}
