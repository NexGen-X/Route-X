package anthropic

import (
	"context"
	"net/http"
	"testing"

	"github.com/NexGen-X/Route-X/internal/providers"
)

// B-261: penjaga B-212 sebelumnya hanya MENGHITUNG blok mentah
// (len(content) == 0). Penerjemah melewati blok yang tidak dikenalnya
// secara diam-diam, jadi respons 200 yang seluruh bloknya asing lolos
// sebagai jawaban kosong yang sukses — pemutus arus tidak mencatat apa
// pun, failover tidak berjalan, panggilan tercatat berhasil. Kekosongan
// kini diukur pada hasil terjemahan kanonik.
func TestB261BlokAsingSajaDitolak(t *testing.T) {
	for _, tc := range []struct {
		nama string
		body string
	}{
		{"blok tak dikenal saja",
			`{"id":"msg_1","type":"message","role":"assistant","model":"claude-sonnet-4-5","content":[{"type":"blok_masa_depan","data":"x"}],"stop_reason":"end_turn","usage":{"input_tokens":3,"output_tokens":1}}`},
		{"server_tool_use saja",
			`{"id":"msg_1","type":"message","role":"assistant","model":"claude-sonnet-4-5","content":[{"type":"server_tool_use","id":"srv_1","name":"web_search","input":{}}],"stop_reason":"end_turn","usage":{"input_tokens":3,"output_tokens":1}}`},
		{"redacted_thinking saja",
			`{"id":"msg_1","type":"message","role":"assistant","model":"claude-sonnet-4-5","content":[{"type":"redacted_thinking","data":"ZW5j"}],"stop_reason":"end_turn","usage":{"input_tokens":3,"output_tokens":1}}`},
	} {
		t.Run(tc.nama, func(t *testing.T) {
			p, _ := newServer(t, jsonHandler(http.StatusOK, tc.body))
			_, err := p.ChatCompletion(context.Background(), simpleRequest())
			if err == nil {
				t.Fatal("ChatCompletion berhasil untuk respons yang hasil terjemahannya kosong")
			}
			if e := providers.AsError(err); e == nil || e.Kind != providers.ErrKindServer {
				t.Errorf("error = %v, mau *providers.Error berkelas server", err)
			}
		})
	}
}

// Kontrol: blok asing yang MENEMANI konten nyata tidak menggagalkan
// respons, dan blok thinking sendirian adalah konten (penalaran).
func TestB261BlokAsingDenganKontenNyataLolos(t *testing.T) {
	p, _ := newServer(t, jsonHandler(http.StatusOK,
		`{"id":"msg_1","type":"message","role":"assistant","model":"claude-sonnet-4-5","content":[{"type":"blok_masa_depan","data":"x"},{"type":"text","text":"halo"}],"stop_reason":"end_turn","usage":{"input_tokens":3,"output_tokens":2}}`))
	resp, err := p.ChatCompletion(context.Background(), simpleRequest())
	if err != nil {
		t.Fatalf("ChatCompletion: %v", err)
	}
	if len(resp.Choices) != 1 || resp.Choices[0].Message.Content != "halo" {
		t.Errorf("respons = %+v, ingin satu choice berisi \"halo\"", resp.Choices)
	}

	p2, _ := newServer(t, jsonHandler(http.StatusOK,
		`{"id":"msg_2","type":"message","role":"assistant","model":"claude-sonnet-4-5","content":[{"type":"thinking","thinking":"sedang berpikir","signature":"GANTI_tanda_tangan_uji"}],"stop_reason":"end_turn","usage":{"input_tokens":3,"output_tokens":2}}`))
	resp2, err := p2.ChatCompletion(context.Background(), simpleRequest())
	if err != nil {
		t.Fatalf("ChatCompletion thinking: %v", err)
	}
	if len(resp2.Choices) != 1 || resp2.Choices[0].Message.ReasoningContent != "sedang berpikir" {
		t.Errorf("respons thinking = %+v, ingin reasoning \"sedang berpikir\"", resp2.Choices)
	}
}
