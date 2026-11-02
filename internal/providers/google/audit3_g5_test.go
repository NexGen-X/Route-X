package google

// Test regresi audit3 grup 5 (V-GW-40): adapter Google tidak bisa melayani
// masukan embeddings berbentuk token — penolakannya harus jelas
// (invalid_request), bukan diteruskan sebagai teks kosong atau ditebak.

import (
	"context"
	"errors"
	"testing"

	"github.com/NexGen-X/Route-X/internal/providers"
)

func TestEmbeddingsTokenInputDitolakJelas(t *testing.T) {
	p := &Provider{}
	_, err := p.Embeddings(context.Background(), &providers.EmbeddingsRequest{
		Model:      "text-embedding-004",
		TokenInput: [][]int{{123, 456}},
	})
	var pErr *providers.Error
	if !errors.As(err, &pErr) {
		t.Fatalf("err = %v, mau *providers.Error", err)
	}
	if pErr.Kind != providers.ErrKindInvalidRequest {
		t.Errorf("kind = %v, mau ErrKindInvalidRequest", pErr.Kind)
	}
}
