package openai

import (
	"context"
	"errors"
	"io"
	"net/http"
	"testing"

	"github.com/NexGen-X/Route-X/internal/providers"
)

// TestB325ErrorAliranTanpaMessageTerklasifikasi adalah pengunci B-325:
// objek error SSE OpenAI TANPA field message sebelumnya lolos sebagai
// chunk biasa (gerbang parseChunk mensyaratkan Message != ""), sehingga
// error code-only — persis bentuk yang B-297 buktikan untuk
// non-streaming — tak terklasifikasi, tak memicu penjaga auth B-210,
// dan Raw errornya diteruskan ke klien. Kini kehadiran code ATAU type
// saja cukup: peristiwa itu menjadi *providers.Error terklasifikasi.
func TestB325ErrorAliranTanpaMessageTerklasifikasi(t *testing.T) {
	kasus := []struct {
		nama    string
		muatan  string
		mauKind providers.ErrorKind
		mauKode string
	}{
		{
			nama:    "kode auth saja",
			muatan:  `{"error":{"code":"invalid_api_key","type":"invalid_request_error"}}`,
			mauKind: providers.ErrKindAuth,
			mauKode: "invalid_api_key",
		},
		{
			nama:    "type overloaded saja",
			muatan:  `{"error":{"type":"overloaded_error"}}`,
			mauKind: providers.ErrKindOverloaded,
		},
		{
			nama:    "kode rate limit saja",
			muatan:  `{"error":{"code":"rate_limit_exceeded"}}`,
			mauKind: providers.ErrKindRateLimit,
			mauKode: "rate_limit_exceeded",
		},
	}
	for _, k := range kasus {
		t.Run(k.nama, func(t *testing.T) {
			u := newUpstream(t, sseHandler(k.muatan, "[DONE]"))
			p := testProvider(t, u.url(), nil)
			s := openStream(t, p, chatRequest())
			_, err := s.Recv()
			var pe *providers.Error
			if !errors.As(err, &pe) {
				t.Fatalf("Recv error = %v, mau *providers.Error berkelas %v — objek error tanpa message lolos sebagai chunk", err, k.mauKind)
			}
			if pe.Kind != k.mauKind {
				t.Errorf("Kind = %v, mau %v", pe.Kind, k.mauKind)
			}
			if pe.UpstreamCode != k.mauKode {
				t.Errorf("UpstreamCode = %q, mau %q", pe.UpstreamCode, k.mauKode)
			}
		})
	}
}

// TestB325ErrorAliranTanpaMessageDiTengahAliran membuktikan varian
// tengah-aliran B-325: chunk sah pertama tetap sampai sebagai
// peristiwa, lalu objek error code-only mengakhiri aliran sebagai
// error auth terklasifikasi (bukan sebagai chunk kedua ber-Raw error).
func TestB325ErrorAliranTanpaMessageDiTengahAliran(t *testing.T) {
	u := newUpstream(t, sseHandler(chunkHa,
		`{"error":{"code":"invalid_api_key","type":"invalid_request_error"}}`, "[DONE]"))
	p := testProvider(t, u.url(), nil)
	s := openStream(t, p, chatRequest())

	ev, err := s.Recv()
	if err != nil {
		t.Fatalf("Recv pertama: %v", err)
	}
	if ev.Delta != "Ha" {
		t.Fatalf("Delta peristiwa pertama = %q, mau %q", ev.Delta, "Ha")
	}
	_, err = s.Recv()
	var pe *providers.Error
	if !errors.As(err, &pe) {
		t.Fatalf("Recv kedua error = %v, mau *providers.Error auth", err)
	}
	if pe.Kind != providers.ErrKindAuth {
		t.Errorf("Kind = %v, mau %v", pe.Kind, providers.ErrKindAuth)
	}
}

// TestB325ChunkSahTidakTersentuh membuktikan sisi kontrol B-325:
// aliran tanpa objek error berperilaku persis seperti sebelumnya, dan
// objek error yang sungguh kosong (tanpa message/code/type) tidak
// mengarang klasifikasi — ia tetap jatuh sebagai chunk.
func TestB325ChunkSahTidakTersentuh(t *testing.T) {
	u := newUpstream(t, sseHandler(chunkRole, chunkHa, chunkFinish, "[DONE]"))
	p := testProvider(t, u.url(), nil)
	s := openStream(t, p, chatRequest())
	events := recvAll(t, s)
	if len(events) != 3 {
		t.Fatalf("jumlah peristiwa = %d, mau 3", len(events))
	}

	u2 := newUpstream(t, sseHandler(`{"error":{}}`, "[DONE]"))
	p2 := testProvider(t, u2.url(), nil)
	s2 := openStream(t, p2, chatRequest())
	ev, err := s2.Recv()
	if err != nil {
		t.Fatalf("Recv objek error kosong: %v", err)
	}
	if ev == nil || len(ev.Raw) == 0 {
		t.Errorf("objek error kosong seharusnya tetap chunk ber-Raw, dapat %+v", ev)
	}
	_ = io.EOF
}

// TestB340ErrorBody200SelarasAliran adalah pengunci B-340:
// errorFromSuccessBody dulu memetakan type tertanam body-200 lebih
// sempit daripada jenisGalatAliran — overloaded_error/permission_error
// jatuh ke ErrKindServer (retryable) padahal padanan alirannya
// overloaded/permission. Kini cakupan keduanya selaras.
func TestB340ErrorBody200SelarasAliran(t *testing.T) {
	kasus := []struct {
		nama    string
		body    string
		mauKind providers.ErrorKind
	}{
		{"type overloaded", `{"error":{"type":"overloaded_error"}}`, providers.ErrKindOverloaded},
		{"type permission", `{"error":{"type":"permission_error"}}`, providers.ErrKindPermission},
		{"type not found", `{"error":{"type":"not_found_error"}}`, providers.ErrKindModelNotFound},
		{"kode auth (kontrol B-260)", `{"error":{"code":"invalid_api_key"}}`, providers.ErrKindAuth},
		{"kode rate limit (kontrol B-297)", `{"error":{"code":"rate_limit_exceeded","type":"rate_limit_error"}}`, providers.ErrKindRateLimit},
		{"tak dikenal tetap server (kontrol)", `{"error":{"code":"ledakan_aneh"}}`, providers.ErrKindServer},
	}
	for _, k := range kasus {
		t.Run(k.nama, func(t *testing.T) {
			u := newUpstream(t, jsonHandler(http.StatusOK, k.body))
			p := testProvider(t, u.url(), nil)
			_, err := p.ChatCompletion(context.Background(), &providers.ChatRequest{
				Model:    "model-uji",
				Messages: []providers.Message{{Role: "user", Content: "halo"}},
			})
			var pe *providers.Error
			if !errors.As(err, &pe) {
				t.Fatalf("ChatCompletion error = %v, mau *providers.Error", err)
			}
			if pe.Kind != k.mauKind {
				t.Errorf("Kind = %v, mau %v", pe.Kind, k.mauKind)
			}
		})
	}
}
