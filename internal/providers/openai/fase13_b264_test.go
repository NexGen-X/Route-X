package openai

import (
	"context"
	"testing"
)

// B-264 (OpenAI): daftar model dideduplikasi per ID — sebagian gateway
// berdialek OpenAI mengulang model yang sama dalam satu daftar, dan
// duplikat yang lolos menjadi baris katalog ganda di hilir. Kemunculan
// pertama menang. (API daftar model OpenAI tidak punya kursor, jadi
// tidak ada paginasi yang bisa diikuti di adapter ini.)
func TestModelsDeduplikasiB264(t *testing.T) {
	const body = `{
  "object": "list",
  "data": [
    {"id": "gpt-4o", "object": "model", "created": 1712345678, "owned_by": "openai"},
    {"id": "gpt-4o-mini", "object": "model", "created": 1712345679, "owned_by": "openai"},
    {"id": "gpt-4o", "object": "model", "created": 999, "owned_by": "duplikat"}
  ]
}`
	u := newUpstream(t, jsonHandler(200, body))
	p := testProvider(t, u.url(), nil)

	models, err := p.Models(context.Background())
	if err != nil {
		t.Fatalf("Models: %v", err)
	}
	if len(models) != 2 {
		t.Fatalf("jumlah model = %d, mau 2 (duplikat dibuang): %+v", len(models), models)
	}
	if models[0].ID != "gpt-4o" || models[0].OwnedBy != "openai" || models[0].Created != 1712345678 {
		t.Errorf("model pertama = %+v, mau kemunculan pertama gpt-4o", models[0])
	}
	if models[1].ID != "gpt-4o-mini" {
		t.Errorf("model kedua = %+v, mau gpt-4o-mini", models[1])
	}
}
