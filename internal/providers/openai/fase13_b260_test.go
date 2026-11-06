package openai

import (
	"context"
	"testing"

	"github.com/NexGen-X/Route-X/internal/providers"
)

// B-260: sebagian gateway berdialek OpenAI menjawab 200 dengan body
// {"error": {...}} yang kode/type-nya bergaya OpenAI sendiri
// (rate_limit_exceeded, invalid_api_key, rate_limit_error,
// authentication_error). Tabel Classify tidak memuatnya, jadi jalur
// errorFromSuccessBody menjatuhkan semuanya ke ErrKindServer: kegagalan
// auth permanen diulang dan dialihkan seperti gangguan sesaat, dan
// rate-limit tidak dikenali sebagai sinyal jeda.
func TestErrorTertanamBody200DiklasifikasikanB260(t *testing.T) {
	for _, tc := range []struct {
		nama string
		body string
		mau  providers.ErrorKind
	}{
		{"kode rate_limit_exceeded",
			`{"error":{"message":"terlalu banyak permintaan","code":"rate_limit_exceeded","type":"rate_limit_error"}}`,
			providers.ErrKindRateLimit},
		{"type rate_limit_error saja",
			`{"error":{"message":"terlalu banyak permintaan","type":"rate_limit_error"}}`,
			providers.ErrKindRateLimit},
		{"kode invalid_api_key",
			`{"error":{"message":"kunci tidak sah","code":"invalid_api_key","type":"invalid_request_error"}}`,
			providers.ErrKindAuth},
		{"type authentication_error saja",
			`{"error":{"message":"kunci tidak sah","type":"authentication_error"}}`,
			providers.ErrKindAuth},
		{"kode tak dikenal tetap server",
			`{"error":{"message":"sesuatu yang baru","code":"kode_baru_upstream"}}`,
			providers.ErrKindServer},
	} {
		t.Run(tc.nama, func(t *testing.T) {
			u := newUpstream(t, jsonHandler(200, tc.body))
			p := testProvider(t, u.url(), nil)
			_, err := p.ChatCompletion(context.Background(), &providers.ChatRequest{
				Model:    "model-uji",
				Messages: []providers.Message{{Role: "user", Content: "halo"}},
			})
			perr := providers.AsError(err)
			if perr == nil {
				t.Fatalf("error = %v, mau *providers.Error berjenis %s", err, tc.mau)
			}
			if perr.Kind != tc.mau {
				t.Errorf("jenis = %s, mau %s (kode upstream: %q)", perr.Kind, tc.mau, perr.UpstreamCode)
			}
		})
	}
}
