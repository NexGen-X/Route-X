package gateway

import (
	"context"
	"encoding/json"
	"net/http"
	"testing"

	"github.com/NexGen-X/Route-X/internal/database/repo/upstream"
	"github.com/NexGen-X/Route-X/internal/providers"
)

// Test pengunci B-168 dan B-169 (Fase 5 kampanye tuntas-logika).
//
// B-168: kegagalan milik klien tidak boleh tercatat sebagai kegagalan
// provider ke circuit breaker. InvalidRequest sudah dikecualikan oleh
// turunan B-161 (PR #285) dan ContentFilter oleh B-240 (PR #310);
// ModelNotFound adalah sisa terakhir dari daftar temuan ini.
// B-169: kegagalan upstream yang sampai ke klien Anthropic harus
// memakai tipe error Anthropic di SEMUA fase — dua titik pra-aliran
// sebelumnya memakai kosakata OpenAI dari httpUntuk.

// --- B-168 -------------------------------------------------------------------

func TestB168ModelNotFoundTidakMenggerusBreaker(t *testing.T) {
	g := penjagaModelBaru()
	e, _ := executorUji(g)

	out := Execute(context.Background(), e,
		rencana("model-a", 1, 0, kandidat("pa", func(c *upstream.RouteCandidate) { c.MaxRetries = 0 })),
		func(_ context.Context, c *upstream.RouteCandidate) (string, error) {
			return "", providers.Newf(providers.ErrKindModelNotFound, c.ProviderName,
				"model tidak dikenal provider ini")
		})
	if out.Err == nil || out.Err.Kind != providers.ErrKindModelNotFound {
		t.Fatalf("Err = %v, mau model_not_found", out.Err)
	}
	if len(g.catatan) != 0 {
		t.Errorf("B-168: kegagalan model_not_found tercatat ke breaker %v, mau tanpa catatan kegagalan", g.catatan)
	}
}

func TestB168KegagalanServerTetapTercatat(t *testing.T) {
	g := penjagaModelBaru()
	e, _ := executorUji(g)

	out := Execute(context.Background(), e,
		rencana("model-a", 1, 0, kandidat("pa", func(c *upstream.RouteCandidate) { c.MaxRetries = 0 })),
		func(_ context.Context, c *upstream.RouteCandidate) (string, error) {
			return "", providers.Newf(providers.ErrKindServer, c.ProviderName, "upstream 500")
		})
	if out.Err == nil {
		t.Fatal("Err = nil, mau kegagalan server")
	}
	if len(g.catatan) != 1 || g.catatan[0] != "prov-pa|model-a|false" {
		t.Errorf("catatan = %v, mau satu kegagalan server tercatat (pengecualian B-168 wajib spesifik per jenis)", g.catatan)
	}
}

// --- B-169 -------------------------------------------------------------------

type envelopeAnthropicUji struct {
	Type  string `json:"type"`
	Error struct {
		Type    string `json:"type"`
		Message string `json:"message"`
	} `json:"error"`
}

func TestB169TipeErrorAnthropicKonsistenAntarFase(t *testing.T) {
	kasus := []struct {
		nama   string
		kind   providers.ErrorKind
		status int
		tipe   string
	}{
		// Pra-fix ketiga baris ini menjawab tipe OpenAI: api_error
		// untuk auth dan kuota, invalid_request_error untuk model
		// tak dikenal — padahal kegagalan yang sama di tengah aliran
		// memakai tipe Anthropic di kolom yang sama.
		{"auth", providers.ErrKindAuth, http.StatusBadGateway, "authentication_error"},
		{"model_not_found", providers.ErrKindModelNotFound, http.StatusNotFound, "not_found_error"},
		{"kuota", providers.ErrKindQuota, http.StatusBadGateway, "billing_error"},
	}
	for _, tc := range kasus {
		for _, stream := range []bool{false, true} {
			nama := tc.nama
			if stream {
				nama += "/stream"
			} else {
				nama += "/non-stream"
			}
			t.Run(nama, func(t *testing.T) {
				gagal := &providers.Error{Kind: tc.kind, Provider: "utama", Message: "kegagalan uji " + tc.nama}
				prov := &providerTiruan{
					nama: "utama", kind: providers.KindAnthropic,
					chat: func(_ context.Context, _ *providers.ChatRequest) (*providers.ChatResponse, error) {
						return nil, gagal
					},
					alir: func(_ context.Context, _ *providers.ChatRequest) (providers.Stream, error) {
						return nil, gagal
					},
				}
				s := susun(t, prov, nil)
				body := `{"model":"claude-sonnet-4-5","max_tokens":64,"messages":[{"role":"user","content":"halo"}]}`
				if stream {
					body = `{"model":"claude-sonnet-4-5","max_tokens":64,"stream":true,"messages":[{"role":"user","content":"halo"}]}`
				}
				w := s.panggil(t, http.MethodPost, "/v1/messages", body, nil)
				if w.Code != tc.status {
					t.Fatalf("status = %d, mau %d (body: %s)", w.Code, tc.status, w.Body.String())
				}
				var env envelopeAnthropicUji
				if err := json.Unmarshal(w.Body.Bytes(), &env); err != nil {
					t.Fatalf("body bukan envelope Anthropic: %v (%s)", err, w.Body.String())
				}
				if env.Type != "error" || env.Error.Type != tc.tipe {
					t.Errorf("B-169: error.type = %q, mau %q — tipe harus dari pemeta Anthropic di semua fase (body: %s)",
						env.Error.Type, tc.tipe, w.Body.String())
				}
			})
		}
	}
}
