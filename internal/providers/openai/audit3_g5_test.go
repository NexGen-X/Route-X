package openai

// Test regresi audit3 grup 5 (V-GW-40): masukan embeddings berbentuk token
// harus diteruskan apa adanya ke upstream OpenAI, bukan ditolak atau
// dikonversi diam-diam.

import (
	"context"
	"testing"

	"github.com/NexGen-X/Route-X/internal/providers"
)

func TestEmbeddingsTokenInputDiteruskanApaAdanya(t *testing.T) {
	const body1 = `{"object":"list","model":"text-embedding-3-small","data":[{"object":"embedding","index":0,"embedding":[0.5]}],"usage":{"prompt_tokens":3,"total_tokens":3}}`
	const body2 = `{"object":"list","model":"text-embedding-3-small","data":[{"object":"embedding","index":0,"embedding":[0.5]},{"object":"embedding","index":1,"embedding":[0.25]}],"usage":{"prompt_tokens":3,"total_tokens":3}}`

	t.Run("tunggal", func(t *testing.T) {
		u := newUpstream(t, jsonHandler(200, body1))
		p := testProvider(t, u.url(), nil)
		if _, err := p.Embeddings(context.Background(), &providers.EmbeddingsRequest{
			Model:      "text-embedding-3-small",
			TokenInput: [][]int{{123, 456}},
		}); err != nil {
			t.Fatalf("Embeddings: %v", err)
		}
		input, ok := u.lastBody()["input"].([]any)
		if !ok || len(input) != 2 || input[0] != float64(123) || input[1] != float64(456) {
			t.Errorf("input terkirim = %v, mau [123 456]", u.lastBody()["input"])
		}
	})

	t.Run("batch", func(t *testing.T) {
		u := newUpstream(t, jsonHandler(200, body2))
		p := testProvider(t, u.url(), nil)
		if _, err := p.Embeddings(context.Background(), &providers.EmbeddingsRequest{
			Model:      "text-embedding-3-small",
			TokenInput: [][]int{{1, 2}, {3}},
		}); err != nil {
			t.Fatalf("Embeddings: %v", err)
		}
		input, ok := u.lastBody()["input"].([]any)
		if !ok || len(input) != 2 {
			t.Fatalf("input terkirim = %v, mau batch 2 urutan", u.lastBody()["input"])
		}
		pertama, ok := input[0].([]any)
		if !ok || len(pertama) != 2 || pertama[1] != float64(2) {
			t.Errorf("urutan pertama = %v, mau [1 2]", input[0])
		}
	})
}
