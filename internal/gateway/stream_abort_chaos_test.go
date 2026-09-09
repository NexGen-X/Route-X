package gateway

import (
	"context"
	"net/http"
	"strings"
	"testing"

	"github.com/NexGen-X/Route-X/internal/providers"
)

// TestStreamingAbortTengahJalanTutupUpstreamDanKirimDONE memastikan abort
// klien di tengah aliran tidak membocorkan koneksi upstream: aliran tetap
// ditutup, status tetap 200, dan penanda penutup tetap dikirim supaya klien
// yang masih membaca tidak menggantung.
func TestStreamingAbortTengahJalanTutupUpstreamDanKirimDONE(t *testing.T) {
	aliran := &aliranTiruan{
		ev: []*providers.StreamEvent{peristiwa("ha"), peristiwa("lo")},
		akhir: &providers.Error{
			Kind: providers.ErrKindStreamAborted, Provider: "utama",
			Message: "klien menutup koneksi", StreamedBytes: 12,
		},
	}
	prov := &providerTiruan{
		nama: "utama", kind: providers.KindOpenAI,
		alir: func(context.Context, *providers.ChatRequest) (providers.Stream, error) {
			return aliran, nil
		},
	}
	s := susun(t, prov, nil)
	w := s.panggil(t, http.MethodPost, "/chat/completions", bodyChatStreaming, nil)

	if w.Code != http.StatusOK {
		t.Fatalf("status = %d, mau 200", w.Code)
	}
	body := w.Body.String()
	if !strings.HasPrefix(body, "data: "+string(peristiwa("ha").Raw)) {
		t.Errorf("chunk pertama tidak terkirim sebelum abort: %q", body)
	}
	if !strings.HasSuffix(body, "data: [DONE]\n\n") {
		t.Errorf("penanda penutup tidak dikirim setelah abort: %q", body)
	}
	if !aliran.ditutup {
		t.Error("aliran upstream tidak ditutup setelah abort tengah jalan")
	}
}
