package openai

import (
	"context"
	"net/http"
	"strings"
	"testing"

	"github.com/NexGen-X/Route-X/internal/providers"
)

// TestB296KlasifikasiErrorAliranMenghormatiKode adalah pengunci B-296:
// jenisGalatAliran dulu memeriksa substring "invalid" pada TYPE lebih
// dulu daripada kode auth — error tengah-aliran
// {type:"invalid_request_error", code:"invalid_api_key"} jatuh ke
// invalid_request (tidak memicu penjaga auth B-210, tidak failoverable)
// padahal padanan non-streamingnya auth (B-260). Kini kode auth dan
// rate-limit diperiksa sebelum cabang invalid pada type.
func TestB296KlasifikasiErrorAliranMenghormatiKode(t *testing.T) {
	kasus := []struct {
		tipe, kode string
		mau        providers.ErrorKind
	}{
		{"invalid_request_error", "invalid_api_key", providers.ErrKindAuth},
		{"", "invalid_api_key", providers.ErrKindAuth},
		{"invalid_request_error", "rate_limit_exceeded", providers.ErrKindRateLimit},
		{"invalid_request_error", "sesuatu", providers.ErrKindInvalidRequest},
		{"overloaded_error", "", providers.ErrKindOverloaded},
		{"", "", providers.ErrKindServer},
	}
	for _, k := range kasus {
		if got := jenisGalatAliran(k.tipe, k.kode); got != k.mau {
			t.Errorf("jenisGalatAliran(%q, %q) = %v, mau %v", k.tipe, k.kode, got, k.mau)
		}
	}
}

// TestB297ErrorBody200TanpaMessageTerklasifikasi adalah pengunci
// B-297: body error 200 TANPA field message sebelumnya kehilangan
// klasifikasi & kode upstream sama sekali (ReadErrorBody mengembalikan
// kosong bila message kosong) sehingga jatuh ke server generik.
// Kini kode/type tetap terbaca dan pemetaan B-260 berlaku.
func TestB297ErrorBody200TanpaMessageTerklasifikasi(t *testing.T) {
	body := `{"error":{"code":"rate_limit_exceeded","type":"rate_limit_error"}}`
	u := newUpstream(t, jsonHandler(http.StatusOK, body))
	p := testProvider(t, u.url(), nil)
	_, err := p.ChatCompletion(context.Background(), &providers.ChatRequest{
		Model:    "model-uji",
		Messages: []providers.Message{{Role: "user", Content: "halo"}},
	})
	perr := providers.AsError(err)
	if perr == nil {
		t.Fatalf("mau *providers.Error, dapat %v", err)
	}
	if perr.Kind != providers.ErrKindRateLimit {
		t.Errorf("kind = %v, mau rate_limit (klasifikasi B-260 untuk code-only)", perr.Kind)
	}
	if perr.UpstreamCode != "rate_limit_exceeded" {
		t.Errorf("UpstreamCode = %q, mau rate_limit_exceeded", perr.UpstreamCode)
	}
	if !strings.Contains(perr.Message, "rate_limit_exceeded") {
		t.Errorf("pesan tidak menyebut kode upstream: %q", perr.Message)
	}

	// Kontrol: body 200 tanpa objek error sama sekali tetap fallback
	// server dengan pesan bawaan pemanggil (bukan klasifikasi karangan).
	u2 := newUpstream(t, jsonHandler(http.StatusOK, `{"id":"x","choices":[]}`))
	p2 := testProvider(t, u2.url(), nil)
	_, err2 := p2.ChatCompletion(context.Background(), &providers.ChatRequest{
		Model:    "model-uji",
		Messages: []providers.Message{{Role: "user", Content: "halo"}},
	})
	perr2 := providers.AsError(err2)
	if perr2 == nil || perr2.Kind != providers.ErrKindServer {
		t.Fatalf("kontrol: kind = %v, mau server", perr2)
	}
	if !strings.Contains(perr2.Message, "tidak memuat satu pun choice") {
		t.Errorf("kontrol: pesan fallback hilang: %q", perr2.Message)
	}
}
