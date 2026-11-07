package anthropic

// fase21_prc_test.go — pengunci Fase 21 PR-C (sisi Anthropic).
//
//	B-516 keputusan B-466 di jalur STREAMING: peristiwa
//	      citations_delta dibuang dengan sadar — teks utuh,
//	      atribusi tidak diteruskan dan tidak dikarang.
//
// Test ini mengalirkan peristiwa citations_delta SUNGGUHAN lewat
// ChatCompletionStream (penerjemah aliran Anthropic), bukan
// memanggil penangan delanya secara langsung.

import (
	"context"
	"errors"
	"io"
	"strings"
	"testing"
)

// TestB516StreamingCitationsDeltaDibuang mengunci keputusan B-466
// di jalur streaming: aliran yang memuat citations_delta selesai
// normal, teksnya utuh persis, dan URL sitasi tidak muncul di satu
// pun peristiwa keluaran (Raw maupun delta).
func TestB516StreamingCitationsDeltaDibuang(t *testing.T) {
	p, _ := newServer(t, sseHandler(
		"event: message_start\ndata: {\"type\":\"message_start\",\"message\":{\"id\":\"msg_b516\",\"model\":\"claude-sonnet-4-5\",\"usage\":{\"input_tokens\":10,\"output_tokens\":1}}}\n\n",
		"event: content_block_start\ndata: {\"type\":\"content_block_start\",\"index\":0,\"content_block\":{\"type\":\"text\",\"text\":\"\"}}\n\n",
		"event: content_block_delta\ndata: {\"type\":\"content_block_delta\",\"index\":0,\"delta\":{\"type\":\"text_delta\",\"text\":\"Jakarta adalah ibu kota \"}}\n\n",
		"event: content_block_delta\ndata: {\"type\":\"content_block_delta\",\"index\":0,\"delta\":{\"type\":\"citations_delta\",\"citation\":{\"type\":\"web_search_result_location\",\"url\":\"https://contoh.test/sumber-b516\",\"title\":\"Sumber B516\",\"encrypted_index\":\"abc\"}}}\n\n",
		"event: content_block_delta\ndata: {\"type\":\"content_block_delta\",\"index\":0,\"delta\":{\"type\":\"text_delta\",\"text\":\"Indonesia.\"}}\n\n",
		"event: content_block_stop\ndata: {\"type\":\"content_block_stop\",\"index\":0}\n\n",
		"event: message_delta\ndata: {\"type\":\"message_delta\",\"delta\":{\"stop_reason\":\"end_turn\"},\"usage\":{\"output_tokens\":5}}\n\n",
		"event: message_stop\ndata: {\"type\":\"message_stop\"}\n\n",
	))

	st, err := p.ChatCompletionStream(context.Background(), simpleRequest())
	if err != nil {
		t.Fatalf("ChatCompletionStream ber-sitasi: %v", err)
	}
	defer st.Close()

	got := drain(st)
	if !errors.Is(got.err, io.EOF) {
		t.Fatalf("aliran berakhir dengan %v, mau io.EOF (citations_delta tidak boleh memutus aliran)", got.err)
	}
	if got.text != "Jakarta adalah ibu kota Indonesia." {
		t.Errorf("teks tergabung = %q, mau utuh %q", got.text, "Jakarta adalah ibu kota Indonesia.")
	}
	for i, raw := range got.raws {
		if strings.Contains(string(raw), "sumber-b516") {
			t.Errorf("peristiwa keluaran %d memuat URL sitasi — keputusan B-466 di jalur streaming berubah tanpa catatan: %s", i, raw)
		}
	}
}
