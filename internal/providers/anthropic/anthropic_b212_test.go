package anthropic

import (
	"context"
	"net/http"
	"testing"

	"github.com/NexGen-X/Route-X/internal/providers"
)

// TestB212BodyErrorStatus200 membuktikan B-212: body bertipe "error" yang
// tiba dengan status 200 kini menjadi error terklasifikasi — bukan
// jawaban kosong yang tampak sukses. Di kode lama ChatCompletion
// mengabaikan field type/error sepenuhnya dan toCanonicalResponse selalu
// mengarang tepat satu choice.
func TestB212BodyErrorStatus200(t *testing.T) {
	ctx := context.Background()

	kasus := []struct {
		nama string
		body string
		kind providers.ErrorKind
	}{
		{
			nama: "overloaded",
			body: `{"type":"error","error":{"type":"overloaded_error","message":"Overloaded"}}`,
			kind: providers.ErrKindOverloaded,
		},
		{
			nama: "saldo habis menjadi kuota",
			body: `{"type":"error","error":{"type":"invalid_request_error","message":"Your credit balance is too low to access the Anthropic API"}}`,
			kind: providers.ErrKindQuota,
		},
		{
			nama: "auth",
			body: `{"type":"error","error":{"type":"authentication_error","message":"invalid x-api-key"}}`,
			kind: providers.ErrKindAuth,
		},
		{
			nama: "tanpa rincian",
			body: `{"type":"error"}`,
			kind: providers.ErrKindServer,
		},
	}
	for _, k := range kasus {
		t.Run(k.nama, func(t *testing.T) {
			p, _ := newServer(t, jsonHandler(http.StatusOK, k.body))
			_, err := p.ChatCompletion(ctx, simpleRequest())
			if err == nil {
				t.Fatalf("ChatCompletion berhasil untuk body error %s, mau error berkelas %s", k.nama, k.kind)
			}
			e := providers.AsError(err)
			if e == nil {
				t.Fatalf("error bukan *providers.Error: %v", err)
			}
			if e.Kind != k.kind {
				t.Errorf("Kind = %s, mau %s", e.Kind, k.kind)
			}
		})
	}
}

// TestB212ResponsKosongDitolak membuktikan sisi kedua B-212: respons 200
// berbentuk pesan tetapi tanpa satu pun blok konten adalah kegagalan
// server, bukan jawaban kosong yang sukses.
func TestB212ResponsKosongDitolak(t *testing.T) {
	p, _ := newServer(t, jsonHandler(http.StatusOK,
		`{"id":"msg_1","type":"message","role":"assistant","model":"claude-sonnet-4-5","content":[],"stop_reason":"end_turn","usage":{"input_tokens":3,"output_tokens":0}}`))

	_, err := p.ChatCompletion(context.Background(), simpleRequest())
	if err == nil {
		t.Fatal("ChatCompletion berhasil untuk respons tanpa blok konten")
	}
	if e := providers.AsError(err); e == nil || e.Kind != providers.ErrKindServer {
		t.Errorf("error = %v, mau *providers.Error berkelas server", err)
	}
}

// TestB212ResponsSahTetapBerhasil adalah penjaga regresi: respons pesan
// yang sah tidak tersentuh pemeriksaan baru.
func TestB212ResponsSahTetapBerhasil(t *testing.T) {
	p, _ := newServer(t, jsonHandler(http.StatusOK,
		`{"id":"msg_1","type":"message","role":"assistant","model":"claude-sonnet-4-5","content":[{"type":"text","text":"halo"}],"stop_reason":"end_turn","usage":{"input_tokens":3,"output_tokens":2}}`))

	resp, err := p.ChatCompletion(context.Background(), simpleRequest())
	if err != nil {
		t.Fatalf("ChatCompletion: %v", err)
	}
	if len(resp.Choices) != 1 || resp.Choices[0].Message.Content != "halo" {
		t.Errorf("respons = %+v, ingin satu choice berisi \"halo\"", resp.Choices)
	}
}
