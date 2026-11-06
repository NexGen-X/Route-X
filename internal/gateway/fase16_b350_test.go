package gateway

import (
	"context"
	"errors"
	"io"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/NexGen-X/Route-X/internal/providers"
)

// TestB350ErrorAuthTengahAliranMemicuPenjaga adalah penutup lubang
// bukti B-350: pasangan B-210+B-325. B-210 membuktikan penjaga auth
// terpicu pada non-streaming dan pada streaming yang GAGAL DIBUKA;
// tak ada test yang mengirim error auth DI TENGAH Recv — padahal di
// titik itulah parseChunk aliran OpenAI mengembalikan
// *providers.Error untuk error ber-message. Kesenjangan itu menjadi
// lubang perilaku nyata lewat gerbang B-325: error code-only tanpa
// message (persis bentuk yang test ini kirim) lolos sebagai chunk
// biasa dan penjaga tak pernah terpicu. Kini: error auth
// tengah-aliran dari adapter OpenAI asli terhadap server SSE uji
// menandai MarkAuthFailure dan memicu RefreshKredensial persis satu
// kali (di luar gerbang jeda B-210, pabrik test ini segar).
func TestB350ErrorAuthTengahAliranMemicuPenjaga(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "text/event-stream")
		w.WriteHeader(http.StatusOK)
		rc := http.NewResponseController(w)
		_, _ = io.WriteString(w, "data: {\"id\":\"chatcmpl-b350\",\"object\":\"chat.completion.chunk\",\"choices\":[{\"index\":0,\"delta\":{\"role\":\"assistant\",\"content\":\"awal\"},\"finish_reason\":null}]}\n\n")
		_ = rc.Flush()
		// Error auth TANPA message — bentuk code-only B-325.
		_, _ = io.WriteString(w, "data: {\"error\":{\"code\":\"invalid_api_key\",\"type\":\"invalid_request_error\"}}\n\n")
		_ = rc.Flush()
	}))
	defer srv.Close()

	ctx := context.Background()
	pelapor := &pelaporAuthPalsu{}
	penyegar := &penyegarKredPalsu{}
	f := pabrikUji(t, kredensialUji(), nil,
		WithAuthFailureReporter(pelapor),
		WithOAuthCredentialRefresher(penyegar),
	)
	adapter, err := f.Provider(ctx, kandidatUji(providers.KindOpenAI, srv.URL))
	if err != nil {
		t.Fatalf("Provider: %v", err)
	}
	aliran, err := adapter.ChatCompletionStream(ctx, permintaanChatUji())
	if err != nil {
		t.Fatalf("ChatCompletionStream: %v", err)
	}
	defer aliran.Close()

	ev, err := aliran.Recv()
	if err != nil {
		t.Fatalf("Recv pertama: %v", err)
	}
	if ev.Delta != "awal" {
		t.Fatalf("Delta peristiwa pertama = %q, mau %q", ev.Delta, "awal")
	}
	_, err = aliran.Recv()
	var pe *providers.Error
	if !errors.As(err, &pe) {
		t.Fatalf("Recv kedua error = %v, mau *providers.Error", err)
	}
	if pe.Kind != providers.ErrKindAuth {
		t.Fatalf("Kind error tengah-aliran = %v, mau auth", pe.Kind)
	}
	if got := pelapor.jumlah.Load(); got != 1 {
		t.Errorf("MarkAuthFailure terpanggil %d kali, mau 1 — penjaga auth tak terpicu oleh error tengah-aliran", got)
	}
	if got := penyegar.jumlah.Load(); got != 1 {
		t.Errorf("RefreshKredensial terpanggil %d kali, mau 1", got)
	}
	if got := pelapor.terakhirID.Load(); got != "cred-1" {
		t.Errorf("MarkAuthFailure untuk kredensial %v, mau cred-1", got)
	}
}
