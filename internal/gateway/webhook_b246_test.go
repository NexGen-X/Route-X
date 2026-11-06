package gateway

import (
	"context"
	"errors"
	"net/http"
	"testing"

	"github.com/NexGen-X/Route-X/internal/providers"
)

// TestWebhookGagalMidStreamTerbitSebagaiFailedB246 adalah pengunci B-246:
// pemilihan event webhook di finalisasi jejak sebelumnya hanya melihat
// status HTTP, sehingga kegagalan mid-stream — yang tercatat berstatus 200
// dengan error_type terisi (B-172) — terbit sebagai request.completed.
// Konsumen webhook menerima "selesai" untuk aliran yang tidak pernah
// diterima klien utuh. Event harus request.failed bila status >= 400 ATAU
// error_type tidak kosong; sukses bersih tetap request.completed.
func TestWebhookGagalMidStreamTerbitSebagaiFailedB246(t *testing.T) {
	sink := &sinkWebhookPalsu{}
	prov := providerSukses()
	prov.alir = func(_ context.Context, _ *providers.ChatRequest) (providers.Stream, error) {
		return &aliranTiruan{ev: []*providers.StreamEvent{peristiwa("hai")},
			akhir: errors.New("koneksi upstream putus")}, nil
	}
	s, _ := susunDenganPencatat(t, prov, func(_ *susunan, d *HandlersDeps) {
		d.Webhooks = sink
	})

	w := s.panggil(t, http.MethodPost, "/chat/completions",
		`{"model":"gpt-4o-mini","stream":true,"messages":[{"role":"user","content":"halo"}]}`, nil)
	if w.Code != http.StatusOK {
		t.Fatalf("stream putus status = %d, mau 200 (kegagalan di dalam aliran): %s", w.Code, w.Body.String())
	}
	if p := sink.cari("request.completed"); p != nil {
		t.Errorf("event request.completed terbit untuk kegagalan mid-stream (B-246); events=%v", sink.events)
	}
	pf := sink.cari("request.failed")
	if pf == nil {
		t.Fatalf("event request.failed tidak terbit untuk kegagalan mid-stream; events=%v", sink.events)
	}
	if pf["status_code"] != http.StatusOK {
		t.Errorf("payload status_code = %v, mau 200 (status yang benar-benar terkirim)", pf["status_code"])
	}
	if pf["error_type"] == nil || pf["error_type"] == "" {
		t.Errorf("payload error_type kosong, mau kategori kegagalan mid-stream: %v", pf)
	}

	// Kontrol: sukses non-streaming bersih tetap request.completed.
	sink2 := &sinkWebhookPalsu{}
	s2, _ := susunDenganPencatat(t, providerSukses(), func(_ *susunan, d *HandlersDeps) {
		d.Webhooks = sink2
	})
	if w := s2.panggil(t, http.MethodPost, "/chat/completions", bodyChatMinimal, nil); w.Code != http.StatusOK {
		t.Fatalf("chat sukses status = %d, mau 200: %s", w.Code, w.Body.String())
	}
	if p := sink2.cari("request.completed"); p == nil {
		t.Errorf("event request.completed tidak terbit untuk sukses bersih; events=%v", sink2.events)
	}
	if p := sink2.cari("request.failed"); p != nil {
		t.Errorf("event request.failed terbit untuk sukses bersih; events=%v", sink2.events)
	}
}
