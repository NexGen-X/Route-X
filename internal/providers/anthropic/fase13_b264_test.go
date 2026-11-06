package anthropic

import (
	"context"
	"net/http"
	"testing"
)

// B-264 (Anthropic): Models() mengikuti kursor after_id sampai has_more
// habis dan mendeduplikasi ID lintas halaman. Sebelumnya hanya halaman
// pertama yang dibaca — katalog di atas satu halaman terpotong diam-diam.
func TestModelsMengikutiKursorDanDedupB264(t *testing.T) {
	handler := func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		switch r.URL.Query().Get("after_id") {
		case "":
			_, _ = w.Write([]byte(`{"data":[
				{"id":"claude-a","created_at":"2025-01-01T00:00:00Z"},
				{"id":"claude-b","created_at":"2025-01-02T00:00:00Z"}
			],"has_more":true,"last_id":"claude-b"}`))
		case "claude-b":
			// Halaman kedua mengulang claude-b (batas halaman bergeser):
			// duplikat harus dibuang, kemunculan pertama menang.
			_, _ = w.Write([]byte(`{"data":[
				{"id":"claude-b","created_at":"2025-01-02T00:00:00Z"},
				{"id":"claude-c","created_at":"2025-01-03T00:00:00Z"}
			],"has_more":false,"last_id":"claude-c"}`))
		default:
			http.Error(w, "after_id tak dikenal", http.StatusBadRequest)
		}
	}
	p, _ := newServer(t, handler)

	models, err := p.Models(context.Background())
	if err != nil {
		t.Fatalf("Models: %v", err)
	}
	if len(models) != 3 {
		t.Fatalf("jumlah model = %d, mau 3 (dua halaman, tanpa duplikat): %+v", len(models), models)
	}
	mau := []string{"claude-a", "claude-b", "claude-c"}
	for i, id := range mau {
		if models[i].ID != id {
			t.Errorf("model[%d] = %q, mau %q", i, models[i].ID, id)
		}
	}
}
